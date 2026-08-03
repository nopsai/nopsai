package nopsai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"nopsai/config"
	"nopsai/pkg/correlation"
	"nopsai/pkg/serviceauth"
	"nopsai/services/nopsai/pkg/audit"
	"nopsai/services/nopsai/pkg/auth"
)

func TestAuthMiddlewarePreservesAgentServiceRoleWithSharedSigningKey(t *testing.T) {
	authService, err := auth.NewService(context.Background(), &pgxpool.Pool{}, auth.Config{
		SigningKey:  "shared-signing-key",
		JWTIssuer:   "local-issuer",
		JWTAudience: "local-audience",
		AccessTTL:   time.Minute,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	serviceAuthenticator, err := serviceauth.NewAuthenticator(serviceauth.Config{
		SigningKey: "shared-signing-key",
		Issuer:     "service-issuer",
		Audience:   "service-audience",
	})
	if err != nil {
		t.Fatalf("NewAuthenticator() error = %v", err)
	}
	credentials, err := serviceauth.NewCredentials(serviceauth.Config{
		SigningKey: "shared-signing-key",
		Issuer:     "service-issuer",
		Audience:   "service-audience",
		Role:       serviceauth.RoleAgent,
		ServiceID:  "agent",
	})
	if err != nil {
		t.Fatalf("NewCredentials() error = %v", err)
	}
	token, err := credentials.MintToken(context.Background())
	if err != nil {
		t.Fatalf("MintToken() error = %v", err)
	}

	app := &App{authService: authService, serviceAuth: serviceAuthenticator}
	handler := app.authMiddleware(app.authzMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireInternalServiceRole(w, r, serviceauth.RoleAgent) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))

	req := httptest.NewRequest(http.MethodPost, "/v1/internal/runs/00000000-0000-0000-0000-000000000001/approvals/pause", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestGitEventsEndpointParsesGitBotServiceToken(t *testing.T) {
	if isPublicPath("/v1/git/events") {
		t.Fatal("/v1/git/events must be authenticated so git-bot service tokens are parsed")
	}
	serviceAuthenticator, err := serviceauth.NewAuthenticator(serviceauth.Config{
		SigningKey: "shared-signing-key",
		Issuer:     "service-issuer",
		Audience:   "service-audience",
	})
	if err != nil {
		t.Fatalf("NewAuthenticator() error = %v", err)
	}
	credentials, err := serviceauth.NewCredentials(serviceauth.Config{
		SigningKey: "shared-signing-key",
		Issuer:     "service-issuer",
		Audience:   "service-audience",
		Role:       serviceauth.RoleGitBot,
		ServiceID:  "git-bot",
	})
	if err != nil {
		t.Fatalf("NewCredentials() error = %v", err)
	}
	token, err := credentials.MintToken(context.Background())
	if err != nil {
		t.Fatalf("MintToken() error = %v", err)
	}

	app := &App{serviceAuth: serviceAuthenticator}
	handler := app.authMiddleware(app.authzMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireInternalServiceRole(w, r, serviceauth.RoleGitBot) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))

	req := httptest.NewRequest(http.MethodPost, "/v1/git/events", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestAuditMiddlewareRecordsAuthenticatedActorFromAuthMiddleware(t *testing.T) {
	serviceAuthenticator, err := serviceauth.NewAuthenticator(serviceauth.Config{
		SigningKey: "shared-signing-key",
		Issuer:     "service-issuer",
		Audience:   "service-audience",
	})
	if err != nil {
		t.Fatalf("NewAuthenticator() error = %v", err)
	}
	credentials, err := serviceauth.NewCredentials(serviceauth.Config{
		SigningKey: "shared-signing-key",
		Issuer:     "service-issuer",
		Audience:   "service-audience",
		Role:       serviceauth.RoleAgent,
		ServiceID:  "agent",
	})
	if err != nil {
		t.Fatalf("NewCredentials() error = %v", err)
	}
	token, err := credentials.MintToken(context.Background())
	if err != nil {
		t.Fatalf("MintToken() error = %v", err)
	}

	auditLog := &recordingAuditWriter{}
	app := &App{serviceAuth: serviceAuthenticator, auditLogger: auditLog}
	handler := requestIDMiddleware(app.auditMiddleware(app.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.ClaimsFromContext(r.Context()); !ok {
			t.Fatal("expected authenticated claims in handler")
		}
		w.WriteHeader(http.StatusNoContent)
	}))))

	req := httptest.NewRequest(http.MethodPost, "/v1/internal/runs/00000000-0000-0000-0000-000000000001/approvals/pause", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set(correlation.RequestIDHeader, "audit-actor-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if len(auditLog.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(auditLog.entries))
	}
	entry := auditLog.entries[0]
	if entry.ActorSub != "agent" || entry.Provider != serviceauth.ProviderInternalService {
		t.Fatalf("audit actor = (%q, %q), want agent/internal-service", entry.ActorSub, entry.Provider)
	}
	if got := entry.Metadata["request_id"]; got != "audit-actor-1" {
		t.Fatalf("audit request_id = %#v, want audit-actor-1", got)
	}
}

func TestOIDCAuthEndpointsArePublic(t *testing.T) {
	publicPaths := []string{
		"/healthz",
		"/livez",
		"/v1/auth/providers",
		"/v1/auth/discover",
		"/v1/auth/session/exchange",
		"/v1/auth/oidc/corporate/start",
		"/v1/auth/oidc/corporate/callback",
		"/v1/auth/oauth2/github/start",
		"/v1/auth/oauth2/github/callback",
	}

	for _, path := range publicPaths {
		if !isPublicPath(path) {
			t.Fatalf("isPublicPath(%q) = false, want true", path)
		}
	}
}

func TestHealthzEndpointIsPublic(t *testing.T) {
	app := &App{}
	handler := app.buildHTTPHandler()

	for _, path := range []string{"/healthz", "/livez"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d; body: %s", path, rec.Code, http.StatusOK, rec.Body.String())
		}
		if rec.Header().Get("Cache-Control") == "" {
			t.Fatalf("%s Cache-Control header is empty, want no-store health response", path)
		}
	}
}

func TestMetricsCanRequireAuthentication(t *testing.T) {
	serviceAuthenticator, err := serviceauth.NewAuthenticator(serviceauth.Config{
		SigningKey: "shared-signing-key",
		Issuer:     "service-issuer",
		Audience:   "service-audience",
	})
	if err != nil {
		t.Fatalf("NewAuthenticator() error = %v", err)
	}
	app := &App{
		cfg:         &config.Config{MetricsRequireAuth: true},
		serviceAuth: serviceAuthenticator,
	}
	handler := app.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("metrics request without auth should not reach handler")
	}))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want unauthorized; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCorsMiddlewareAllowsConfiguredOriginsOnly(t *testing.T) {
	app := &App{cfg: &config.Config{CORSAllowedOrigins: []string{"https://app.example.com"}}}
	handler := app.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	allowedReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	allowedReq.Header.Set("Origin", "https://app.example.com")
	allowedRec := httptest.NewRecorder()
	handler.ServeHTTP(allowedRec, allowedReq)
	if got := allowedRec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("allowed origin header = %q", got)
	}

	blockedReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	blockedReq.Header.Set("Origin", "https://evil.example.com")
	blockedRec := httptest.NewRecorder()
	handler.ServeHTTP(blockedRec, blockedReq)
	if got := blockedRec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("blocked origin header = %q, want empty", got)
	}
}

func TestCorsMiddlewareDoesNotAllowWildcardWhenOriginsAreUnset(t *testing.T) {
	app := &App{cfg: &config.Config{}}
	handler := app.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty without an allow-list", got)
	}
}

func TestLocalLoginFailureReasonClassifiesDetailedErrorsForAudit(t *testing.T) {
	tests := map[string]string{
		"account disabled": "account_disabled",
		"password login is unavailable for this account":        "non_local_account",
		"account lookup is ambiguous; contact an administrator": "ambiguous_identifier",
		"too many login attempts, slow down":                    "rate_limited",
		"invalid credentials":                                   "invalid_credentials",
	}
	for message, want := range tests {
		if got := localLoginFailureReason(errors.New(message)); got != want {
			t.Fatalf("localLoginFailureReason(%q) = %q, want %q", message, got, want)
		}
	}
}

func TestFirstInstallSetupGateAllowedPaths(t *testing.T) {
	allowedPaths := []string{
		"/v1/auth/me",
		"/v1/auth/password",
		"/v1/setup/status",
		"/v1/setup/templates",
		"/v1/setup/templates.zip",
		"/v1/setup/bootstrap",
	}
	for _, path := range allowedPaths {
		if !isFirstInstallSetupAllowedPath(path) {
			t.Fatalf("isFirstInstallSetupAllowedPath(%q) = false, want true", path)
		}
	}

	blockedPaths := []string{
		"/v1/auth/email",
		"/v1/auth/personal-tokens",
		"/v1/pipelines",
		"/v1/system/config",
		"/v1/teams",
	}
	for _, path := range blockedPaths {
		if isFirstInstallSetupAllowedPath(path) {
			t.Fatalf("isFirstInstallSetupAllowedPath(%q) = true, want false", path)
		}
	}
}

type recordingAuditWriter struct {
	entries []audit.Entry
}

func (w *recordingAuditWriter) Write(_ context.Context, entry audit.Entry) error {
	w.entries = append(w.entries, entry)
	return nil
}

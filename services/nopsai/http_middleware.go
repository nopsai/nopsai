package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nopsai/pkg/correlation"
	"nopsai/pkg/serviceauth"
	"nopsai/services/nopsai/pkg/audit"
	"nopsai/services/nopsai/pkg/auth"
	"nopsai/services/nopsai/pkg/routeauthz"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/metadata"
)

func corsMiddleware(next http.Handler) http.Handler {
	return corsMiddlewareWithOrigins(next, nil)
}

func (a *App) corsMiddleware(next http.Handler) http.Handler {
	return corsMiddlewareWithOrigins(next, a.corsAllowedOrigins())
}

func corsMiddlewareWithOrigins(next http.Handler, allowedOrigins []string) http.Handler {
	allowedOrigins = normalizeAllowedOrigins(allowedOrigins)
	wildcard := containsFold(allowedOrigins, "*")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if wildcard {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if originAllowed(origin, allowedOrigins) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID, Traceparent")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func normalizeAllowedOrigins(origins []string) []string {
	if len(origins) == 0 {
		return nil
	}
	out := make([]string, 0, len(origins))
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			out = append(out, origin)
		}
	}
	return out
}

func originAllowed(origin string, allowedOrigins []string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return false
	}
	for _, allowed := range allowedOrigins {
		if strings.EqualFold(origin, allowed) {
			return true
		}
	}
	return false
}

type requestContextKey string

const (
	ctxKeyRequestID requestContextKey = "request-id"
)

func isPublicPath(path string) bool {
	switch path {
	case "/favicon.ico", "/version", "/healthz", "/livez", "/metrics", "/v1/auth/providers", "/v1/auth/discover", "/v1/auth/session/exchange", "/v1/auth/login", "/v1/auth/refresh", "/v1/auth/logout", "/v1/setup/preflight", "/v1/system/dispatcher/runner-bootstrap":
		return true
	// GitHub redirects the operator's browser back to these callbacks without a
	// bearer token; each one authorizes itself with a single-use state.
	case "/v1/git-apps/github/register/callback", "/v1/git-apps/github/install/callback":
		return true
	default:
		return strings.HasPrefix(path, "/v1/auth/oidc/") ||
			strings.HasPrefix(path, "/v1/auth/oauth2/") ||
			strings.HasPrefix(path, "/v1/git/webhooks/")
	}
}

func (a *App) isPublicPath(path string) bool {
	if strings.TrimSpace(path) == "/metrics" && a.metricsRequireAuth() {
		return false
	}
	return isPublicPath(path)
}

func (a *App) metricsRequireAuth() bool {
	if a == nil {
		return false
	}
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	if a.cfg == nil {
		return false
	}
	return a.cfg.MetricsRequireAuth
}

func (a *App) corsAllowedOrigins() []string {
	if a == nil {
		return nil
	}
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	if a.cfg == nil {
		return nil
	}
	return append([]string(nil), a.cfg.CORSAllowedOrigins...)
}

func isPasswordChangeAllowedPath(path string) bool {
	switch strings.TrimSpace(path) {
	case "/v1/auth/me", "/v1/auth/password":
		return true
	default:
		return false
	}
}

func isFirstInstallSetupAllowedPath(path string) bool {
	path = strings.TrimSpace(path)
	switch path {
	case "/v1/auth/me", "/v1/auth/password":
		return true
	// The setup wizard connects GitHub before setup is completed, so the Git
	// Apps surface has to stay reachable while the workspace is still locked.
	case "/v1/git-apps/github",
		"/v1/git-apps/github/installations",
		"/v1/git-apps/github/register/start",
		"/v1/git-apps/github/install/start":
		return true
	default:
		return strings.HasPrefix(path, "/v1/setup/")
	}
}

func isDispatcherInternalPath(path string) bool {
	switch {
	case path == "/v1/run":
		return true
	case strings.HasPrefix(path, "/v1/pipelines/"):
		return true
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/logs/ingest"):
		return true
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/finalize"):
		return true
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/status"):
		return true
	case strings.HasPrefix(path, "/v1/runs/") && strings.Contains(path, "/steps/") && strings.Contains(path, "/tasks/"):
		return true
	default:
		return false
	}
}

func isDispatcherInternalClaims(claims *auth.Claims) bool {
	if claims == nil {
		return false
	}
	return strings.TrimSpace(claims.Sub) == "dispatcher" && strings.TrimSpace(claims.Provider) == "internal-service"
}

func isLongLivedBearerProvider(provider string) bool {
	provider = strings.TrimSpace(provider)
	return strings.EqualFold(provider, auth.ProviderPersonalAccessToken) ||
		strings.EqualFold(provider, auth.ProviderServiceAccountToken)
}

func isTrustedInternalDispatcherRequest(r *http.Request) bool {
	if r == nil || !isDispatcherInternalPath(r.URL.Path) {
		return false
	}
	claims, ok := auth.ClaimsFromContext(r.Context())
	return ok && isDispatcherInternalClaims(claims)
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, reqID, traceparent := correlation.FromHTTPHeaders(r.Context(), r.Header)
		w.Header().Set(correlation.RequestIDHeader, reqID)
		if traceparent != "" {
			w.Header().Set(correlation.TraceparentHeader, traceparent)
		}
		ctx = context.WithValue(ctx, ctxKeyRequestID, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	length int
}

func (lrw *loggingResponseWriter) Unwrap() http.ResponseWriter { return lrw.ResponseWriter }

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.status = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	n, err := lrw.ResponseWriter.Write(b)
	lrw.length += n
	return n, err
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lrw, r)
		reqID := requestIDFromContext(r.Context())
		event := log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", lrw.status).
			Int("bytes", lrw.length).
			Str("request_id", reqID).
			Str("remote_ip", r.RemoteAddr).
			Str("user_agent", r.UserAgent()).
			Int64("duration_ms", time.Since(start).Milliseconds())
		if traceparent := correlation.TraceparentFromContext(r.Context()); traceparent != "" {
			event = event.Str("traceparent", traceparent)
		}
		event.Msg("http_request")
	})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error().Interface("panic", rec).Msg("panic recovered")
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *App) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if a.isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if a.authService == nil && a.serviceAuth == nil {
			http.Error(w, "authentication not configured", http.StatusServiceUnavailable)
			return
		}
		authzHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		if authzHeader == "" || !strings.HasPrefix(strings.ToLower(authzHeader), "bearer ") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		token := strings.TrimSpace(authzHeader[len("Bearer "):])
		var claims *auth.Claims
		var err error
		if a.serviceAuth != nil {
			if serviceClaims, serviceErr := a.authenticateServiceToken(r.Context(), token); serviceErr == nil {
				claims = serviceClaims
			}
		}
		if claims == nil {
			if a.authService == nil {
				http.Error(w, "authentication not configured", http.StatusServiceUnavailable)
				return
			}
			claims, err = a.authService.AuthenticateToken(r.Context(), token)
		}
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		if a.idleTimeout > 0 && !isLongLivedBearerProvider(claims.Provider) {
			now := time.Now()
			activityKey := auth.HashToken(token)
			if lastRaw, ok := a.tokenActivity.Load(activityKey); ok {
				if lastSeen, ok := lastRaw.(time.Time); ok && now.Sub(lastSeen) > a.idleTimeout {
					a.tokenActivity.Delete(activityKey)
					http.Error(w, "session expired due to inactivity", http.StatusUnauthorized)
					return
				}
			}
			a.tokenActivity.Store(activityKey, now)
		}

		mustChangePassword, err := a.passwordChangeRequired(r.Context(), claims)
		if err != nil {
			http.Error(w, "failed to validate account password state", http.StatusInternalServerError)
			return
		}
		if mustChangePassword && !isPasswordChangeAllowedPath(r.URL.Path) {
			http.Error(w, "password change required", http.StatusForbidden)
			return
		}
		setupRequired, err := a.firstInstallSetupRequired(r.Context())
		if err != nil {
			http.Error(w, "failed to validate setup state", http.StatusInternalServerError)
			return
		}
		if setupRequired && !isFirstInstallSetupAllowedPath(r.URL.Path) {
			http.Error(w, "first-install setup required", http.StatusForbidden)
			return
		}

		ctx := auth.WithClaims(r.Context(), claims)
		if state := auditRequestStateFromContext(ctx); state != nil {
			state.claims = claims
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *App) firstInstallSetupRequired(ctx context.Context) (bool, error) {
	if a == nil || a.db == nil {
		return false, nil
	}
	var completedAt string
	err := a.db.QueryRow(ctx, `
		SELECT value
		FROM setup_state
		WHERE key = $1
	`, setupStateKeyCompletedAt).Scan(&completedAt)
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(completedAt) == "", nil
}

func (a *App) authenticateServiceToken(ctx context.Context, token string) (*auth.Claims, error) {
	if a == nil || a.serviceAuth == nil {
		return nil, fmt.Errorf("service authentication not configured")
	}
	md := metadata.Pairs("authorization", "Bearer "+strings.TrimSpace(token))
	serviceClaims, err := a.serviceAuth.AuthenticateContext(metadata.NewIncomingContext(ctx, md))
	if err != nil {
		return nil, err
	}
	return &auth.Claims{
		Sub:      serviceClaims.ServiceID(),
		Provider: serviceauth.ProviderInternalService,
		Roles:    []string{serviceClaims.ServiceRole()},
	}, nil
}

func (a *App) passwordChangeRequired(ctx context.Context, claims *auth.Claims) (bool, error) {
	if a == nil || a.db == nil || claims == nil {
		return false, nil
	}
	if !strings.EqualFold(strings.TrimSpace(claims.Provider), "local") {
		return false, nil
	}
	sub := strings.TrimSpace(claims.Sub)
	if sub == "" {
		return false, nil
	}

	var required bool
	err := a.db.QueryRow(ctx, `
		SELECT must_change_password
		FROM users
		WHERE sub = $1
	`, sub).Scan(&required)
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return required, nil
}

func (a *App) authzMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if a.isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok {
			http.Error(w, "missing claims", http.StatusUnauthorized)
			return
		}

		subject := a.buildAAASubject(claims)
		ctx := withAAASubject(r.Context(), subject)
		if isAuthenticatedOnlyPath(r.URL.Path) {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if !a.aaaAvailable() {
			http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
			return
		}

		action, resource, requiresFilter, err := routeauthz.MapRequest(r)
		if err != nil {
			http.Error(w, "invalid authorization mapping", http.StatusBadRequest)
			return
		}
		if action == "" || requiresFilter {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		decision, err := a.aaaCheck(ctx, subject, action, resource, a.aaaRequestContext(r))
		if err != nil {
			http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
			return
		}
		if !decision.Allowed {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type auditRecorder struct {
	http.ResponseWriter
	status int
}

func (a *auditRecorder) Unwrap() http.ResponseWriter { return a.ResponseWriter }

func (a *auditRecorder) WriteHeader(code int) {
	a.status = code
	a.ResponseWriter.WriteHeader(code)
}

type auditRequestState struct {
	claims *auth.Claims
}

const ctxKeyAuditRequestState requestContextKey = "audit-request-state"

func auditRequestStateFromContext(ctx context.Context) *auditRequestState {
	if ctx == nil {
		return nil
	}
	state, _ := ctx.Value(ctxKeyAuditRequestState).(*auditRequestState)
	return state
}

func (a *App) auditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := &auditRequestState{}
		ctx := context.WithValue(r.Context(), ctxKeyAuditRequestState, state)
		rec := &auditRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r.WithContext(ctx))

		if a.auditLogger == nil {
			return
		}
		claims := state.claims
		if claims == nil {
			claims, _ = auth.ClaimsFromContext(ctx)
		}
		requestID := requestIDFromContext(ctx)

		entry := audit.Entry{
			ActorSub:   "",
			ActorEmail: "",
			Provider:   "",
			Action:     r.Method,
			Resource:   r.URL.Path,
			Result:     fmt.Sprintf("%d", rec.status),
			Metadata: map[string]any{
				"request_id": requestID,
				"remote_ip":  r.RemoteAddr,
				"method":     r.Method,
				"path":       r.URL.Path,
			},
		}
		if traceparent := correlation.TraceparentFromContext(ctx); traceparent != "" {
			entry.Metadata["traceparent"] = traceparent
		}
		if claims != nil {
			entry.ActorSub = claims.Sub
			entry.ActorEmail = claims.Email
			entry.Provider = claims.Provider
		}
		_ = a.auditLogger.Write(ctx, entry)
	})
}

package nopsai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"nopsai/pkg/serviceauth"
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

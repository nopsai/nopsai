package nopsai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/config"
)

func TestHandleAuthLogoutRejectsInvalidJSON(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", strings.NewReader("{"))
	rec := httptest.NewRecorder()

	app.handleAuthLogout(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("handleAuthLogout() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleAuthLogoutRejectsEmptyRefreshToken(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", strings.NewReader(`{"refresh_token":"   "}`))
	rec := httptest.NewRecorder()

	app.handleAuthLogout(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("handleAuthLogout() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestOIDCProviderIDFromUserProvider(t *testing.T) {
	if got := oidcProviderIDFromUserProvider("oidc:Local Keycloak"); got != "local-keycloak" {
		t.Fatalf("oidcProviderIDFromUserProvider() = %q, want local-keycloak", got)
	}
	if got := oidcProviderIDFromUserProvider("local"); got != "" {
		t.Fatalf("oidcProviderIDFromUserProvider(local) = %q, want empty", got)
	}
}

func TestOIDCPostLogoutRedirectURLUsesPublicURL(t *testing.T) {
	app := &App{cfg: &config.Config{PublicURL: "https://ci.example.com/"}}
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)

	if got := app.oidcPostLogoutRedirectURL(req); got != "https://ci.example.com/" {
		t.Fatalf("oidcPostLogoutRedirectURL() = %q, want public origin URL", got)
	}
}

package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"

	"nopsai/pkg/httpapi"
	"nopsai/services/nopsai/pkg/auth"
)

func (a *App) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req authLoginRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	settings, err := getOIDCSettings(r.Context(), a.db, a.cfg)
	if err != nil {
		http.Error(w, "failed to load auth settings", http.StatusInternalServerError)
		return
	}
	if !settings.LocalEnabled {
		http.Error(w, "local authentication is disabled", http.StatusUnauthorized)
		return
	}
	if a.authService != nil {
		a.authService.SetLocalEnabled(settings.LocalEnabled)
	}
	result, err := a.authService.LoginLocal(r.Context(), strings.TrimSpace(req.Identifier), strings.TrimSpace(req.Password))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	_ = httpapi.WriteJSON(w, http.StatusOK, authLoginResponse{
		AccessToken:        result.AccessToken,
		RefreshToken:       result.RefreshToken,
		ExpiresAt:          result.ExpiresAt,
		Roles:              a.resolveAAARoles(r.Context(), result.Claims),
		Provider:           result.Claims.Provider,
		Email:              result.Claims.Email,
		Sub:                result.Claims.Sub,
		MustChangePassword: result.MustChangePassword,
	})
}

func (a *App) handleAuthRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req authRefreshRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	result, err := a.authService.Refresh(r.Context(), strings.TrimSpace(req.RefreshToken))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, authLoginResponse{
		AccessToken:        result.AccessToken,
		RefreshToken:       result.RefreshToken,
		ExpiresAt:          result.ExpiresAt,
		Roles:              a.resolveAAARoles(r.Context(), result.Claims),
		Provider:           result.Claims.Provider,
		Email:              result.Claims.Email,
		Sub:                result.Claims.Sub,
		MustChangePassword: result.MustChangePassword,
	})
}

func (a *App) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req authLogoutRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	refreshToken := strings.TrimSpace(req.RefreshToken)
	if err := httpapi.ValidateRequired(httpapi.RequiredString("refresh_token", refreshToken)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	oidcProviderID, err := a.oidcProviderIDForRefreshToken(r.Context(), refreshToken)
	if err != nil {
		http.Error(w, "failed to resolve logout provider", http.StatusInternalServerError)
		return
	}
	if err := a.authService.Logout(r.Context(), refreshToken); err != nil {
		http.Error(w, "failed to logout", http.StatusInternalServerError)
		return
	}
	logoutURL := ""
	if oidcProviderID != "" {
		logoutURL, err = a.oidcProviderLogoutURL(r.Context(), r, oidcProviderID)
		if err != nil {
			http.Error(w, "failed to build identity provider logout", http.StatusInternalServerError)
			return
		}
	}
	if logoutURL != "" {
		_ = httpapi.WriteJSON(w, http.StatusOK, authLogoutResponse{LogoutURL: logoutURL})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) oidcProviderIDForRefreshToken(ctx context.Context, refreshToken string) (string, error) {
	if a == nil || a.db == nil {
		return "", nil
	}
	hash := auth.HashToken(strings.TrimSpace(refreshToken))
	if hash == "" {
		return "", nil
	}
	var provider string
	err := a.db.QueryRow(ctx, `
		SELECT u.provider
		FROM refresh_tokens rt
		JOIN users u ON u.id = rt.user_id
		WHERE rt.token_hash = $1
		LIMIT 1
	`, hash).Scan(&provider)
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return oidcProviderIDFromUserProvider(provider), nil
}

func oidcProviderIDFromUserProvider(provider string) string {
	provider = strings.TrimSpace(provider)
	if !strings.HasPrefix(strings.ToLower(provider), "oidc:") {
		return ""
	}
	return normalizeOIDCProviderID(strings.TrimSpace(provider[len("oidc:"):]))
}

func (a *App) oidcProviderLogoutURL(ctx context.Context, r *http.Request, providerID string) (string, error) {
	provider, err := getOIDCProvider(ctx, a.db, providerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	if !provider.Enabled {
		return "", nil
	}
	endSessionEndpoint, err := a.oidcEndSessionEndpoint(ctx, provider)
	if err != nil {
		return "", err
	}
	if endSessionEndpoint == "" {
		return "", nil
	}
	values := url.Values{}
	values.Set("client_id", provider.ClientID)
	values.Set("post_logout_redirect_uri", a.oidcPostLogoutRedirectURL(r))
	return appendQuery(endSessionEndpoint, values), nil
}

func (a *App) oidcEndSessionEndpoint(ctx context.Context, provider oidcProviderRecord) (string, error) {
	endpoint, err := a.discoverOIDCEndSessionEndpoint(ctx, provider)
	if err == nil && endpoint != "" {
		return endpoint, nil
	}
	issuer := strings.TrimRight(strings.TrimSpace(provider.Issuer), "/")
	if strings.Contains(strings.ToLower(issuer), "/realms/") {
		return issuer + "/protocol/openid-connect/logout", nil
	}
	if err != nil {
		return "", err
	}
	return "", nil
}

func (a *App) oidcPostLogoutRedirectURL(r *http.Request) string {
	base := ""
	if a != nil && a.cfg != nil {
		base = strings.TrimRight(strings.TrimSpace(a.cfg.PublicURL), "/")
	}
	if base == "" && r != nil {
		scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
		if scheme == "" {
			scheme = "http"
		}
		base = scheme + "://" + r.Host
	}
	if base == "" {
		return "/"
	}
	return base + "/"
}

func (a *App) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	mustChangePassword := false
	if !isDispatcherInternalClaims(claims) {
		userRecord, err := a.loadAuthenticatedUserRecord(r.Context(), claims.Sub)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "user not found", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to load user", http.StatusInternalServerError)
			return
		}
		if !userRecord.IsActive() {
			http.Error(w, "account disabled", http.StatusForbidden)
			return
		}
		mustChangePassword = userRecord.MustChangePassword
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, authLoginResponse{
		AccessToken:        "",
		Roles:              a.resolveAAARoles(r.Context(), claims),
		Provider:           claims.Provider,
		Email:              claims.Email,
		Sub:                claims.Sub,
		MustChangePassword: mustChangePassword,
		Capabilities:       a.authCapabilities(claims),
	})
}

func (a *App) resolveAAARoles(ctx context.Context, claims *auth.Claims) []string {
	if claims == nil {
		return nil
	}
	if a == nil || !a.aaaAvailable() {
		return claims.Roles
	}

	resp, err := a.aaaIntrospect(ctx, a.buildAAASubject(claims))
	if err != nil || resp == nil || len(resp.Roles) == 0 {
		return claims.Roles
	}
	return resp.Roles
}

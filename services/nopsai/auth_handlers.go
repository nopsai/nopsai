package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
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
	var req authRefreshRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	refreshToken := strings.TrimSpace(req.RefreshToken)
	if err := httpapi.ValidateRequired(httpapi.RequiredString("refresh_token", refreshToken)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.authService.Logout(r.Context(), refreshToken); err != nil {
		http.Error(w, "failed to logout", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

package nopsai

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"nopsai/pkg/httpapi"
	"nopsai/services/nopsai/pkg/auth"
)

func (a *App) handleAuthChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || strings.TrimSpace(claims.Sub) == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req authChangePasswordRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	current := strings.TrimSpace(req.CurrentPassword)
	next := strings.TrimSpace(req.NewPassword)
	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("current_password", current),
		httpapi.RequiredString("new_password", next),
	); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(next) < 8 {
		http.Error(w, "new_password must be at least 8 characters", http.StatusBadRequest)
		return
	}
	if current == next {
		http.Error(w, "new password must be different from current password", http.StatusBadRequest)
		return
	}

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
	if userRecord.Provider != "" && userRecord.Provider != "local" {
		http.Error(w, "password changes are unavailable for this account", http.StatusBadRequest)
		return
	}
	if !userRecord.PasswordHash.Valid {
		http.Error(w, "password not set for this account", http.StatusBadRequest)
		return
	}
	if err := auth.ComparePassword(userRecord.PasswordHash.String, current); err != nil {
		http.Error(w, "invalid current password", http.StatusUnauthorized)
		return
	}
	hashed, err := auth.HashPassword(next)
	if err != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	if _, err := tx.Exec(r.Context(), `UPDATE users SET password_hash = $1, must_change_password = FALSE WHERE id = $2`, hashed, userRecord.ID); err != nil {
		http.Error(w, "failed to update password", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM refresh_tokens WHERE user_id = $1`, userRecord.ID); err != nil {
		http.Error(w, "failed to revoke refresh tokens", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to save password", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleAuthUpdateEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || strings.TrimSpace(claims.Sub) == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req authUpdateEmailRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	email, err := normalizeOptionalEmail(req.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := httpapi.ValidateRequired(httpapi.RequiredString("email", email)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
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
	if userRecord.Provider != "" && userRecord.Provider != "local" {
		http.Error(w, "email changes are unavailable for this account", http.StatusBadRequest)
		return
	}
	inUse, err := a.userEmailInUse(r.Context(), email, &userRecord.ID)
	if err != nil {
		http.Error(w, "failed to validate email", http.StatusInternalServerError)
		return
	}
	if inUse {
		http.Error(w, "email already in use", http.StatusConflict)
		return
	}
	if _, err := a.db.Exec(r.Context(), `UPDATE users SET email = $1 WHERE id = $2`, email, userRecord.ID); err != nil {
		http.Error(w, "failed to update email", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, map[string]string{"email": email})
}

func (a *App) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT actor_sub, actor_email, provider, action, resource, result, created_at
		FROM audit_logs
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		http.Error(w, "failed to list audit logs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type auditRow struct {
		ActorSub   string    `json:"actor_sub"`
		ActorEmail string    `json:"actor_email"`
		Provider   string    `json:"provider"`
		Action     string    `json:"action"`
		Resource   string    `json:"resource"`
		Result     string    `json:"result"`
		CreatedAt  time.Time `json:"created_at"`
	}
	var out []auditRow
	for rows.Next() {
		var row auditRow
		if err := rows.Scan(&row.ActorSub, &row.ActorEmail, &row.Provider, &row.Action, &row.Resource, &row.Result, &row.CreatedAt); err != nil {
			http.Error(w, "failed to read audit logs", http.StatusInternalServerError)
			return
		}
		out = append(out, row)
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, out)
}

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

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
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt,
		Roles:        result.Claims.Roles,
		Provider:     result.Claims.Provider,
		Email:        result.Claims.Email,
		Sub:          result.Claims.Sub,
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
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt,
		Roles:        result.Claims.Roles,
		Provider:     result.Claims.Provider,
		Email:        result.Claims.Email,
		Sub:          result.Claims.Sub,
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
	_ = httpapi.WriteJSON(w, http.StatusOK, authLoginResponse{
		AccessToken:  "",
		Roles:        claims.Roles,
		Provider:     claims.Provider,
		Email:        claims.Email,
		Sub:          claims.Sub,
		Capabilities: a.authCapabilities(claims),
	})
}

func (a *App) authCapabilities(claims *auth.Claims) *authCapabilitiesResponse {
	if claims == nil {
		return &authCapabilitiesResponse{}
	}
	if a == nil || a.authz == nil {
		return &authCapabilitiesResponse{
			Pipelines: authResourceCapabilities{Write: true, Delete: true},
			Steps:     authResourceCapabilities{Write: true, Delete: true},
		}
	}
	return &authCapabilitiesResponse{
		Pipelines: authResourceCapabilities{
			Write:  a.authz.EnforceRoles(claims.Roles, "/v1/pipelines/__capability_probe__", http.MethodPut),
			Delete: a.authz.EnforceRoles(claims.Roles, "/v1/pipelines/__capability_probe__", http.MethodDelete),
		},
		Steps: authResourceCapabilities{
			Write:  a.authz.EnforceRoles(claims.Roles, "/v1/steps/__capability_probe__", http.MethodPut),
			Delete: a.authz.EnforceRoles(claims.Roles, "/v1/steps/__capability_probe__", http.MethodDelete),
		},
	}
}

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

	var (
		userID       uuid.UUID
		provider     string
		passwordHash sql.NullString
	)
	err := a.db.QueryRow(r.Context(), `SELECT id, provider, password_hash FROM users WHERE sub = $1`, claims.Sub).Scan(&userID, &provider, &passwordHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load user", http.StatusInternalServerError)
		return
	}
	if provider != "" && provider != "local" {
		http.Error(w, "password changes are unavailable for this account", http.StatusBadRequest)
		return
	}
	if !passwordHash.Valid {
		http.Error(w, "password not set for this account", http.StatusBadRequest)
		return
	}
	if err := auth.ComparePassword(passwordHash.String, current); err != nil {
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

	if _, err := tx.Exec(r.Context(), `UPDATE users SET password_hash = $1 WHERE id = $2`, hashed, userID); err != nil {
		http.Error(w, "failed to update password", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM refresh_tokens WHERE user_id = $1`, userID); err != nil {
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
	email := strings.TrimSpace(req.Email)
	if err := httpapi.ValidateRequired(httpapi.RequiredString("email", email)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil {
		http.Error(w, "invalid email", http.StatusBadRequest)
		return
	}
	email = strings.TrimSpace(parsed.Address)

	var (
		userID   uuid.UUID
		provider string
	)
	err = a.db.QueryRow(r.Context(), `SELECT id, provider FROM users WHERE sub = $1`, claims.Sub).Scan(&userID, &provider)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load user", http.StatusInternalServerError)
		return
	}
	if provider != "" && provider != "local" {
		http.Error(w, "email changes are unavailable for this account", http.StatusBadRequest)
		return
	}
	if _, err := a.db.Exec(r.Context(), `UPDATE users SET email = $1 WHERE id = $2`, email, userID); err != nil {
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

func ensureDefaultAdmin(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	adminID := uuid.MustParse(defaultAdminID)
	var existingID uuid.UUID
	var provider sql.NullString
	err := db.QueryRow(ctx, `SELECT id, provider FROM users WHERE sub = $1`, defaultAdminSub).Scan(&existingID, &provider)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if existingID == uuid.Nil {
		_, err = db.Exec(ctx, `
			INSERT INTO users (id, sub, email, provider, password_hash, status)
			VALUES ($1, $2, $3, 'local', $4, 'active')
			ON CONFLICT (sub) DO NOTHING
		`, adminID, defaultAdminSub, defaultAdminEmail, defaultAdminPasswordHash)
		if err != nil {
			return err
		}
		if err := db.QueryRow(ctx, `SELECT id FROM users WHERE sub = $1`, defaultAdminSub).Scan(&existingID); err != nil {
			return err
		}
	} else if provider.Valid && provider.String != "local" {
		if _, err := db.Exec(ctx, `UPDATE users SET provider = 'local' WHERE id = $1`, existingID); err != nil {
			return err
		}
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO user_roles (user_id, role)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, existingID, defaultAdminRole); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO role_permissions (role, name, obj, act)
		SELECT $1, 'All access', '/*', '.*'
		WHERE NOT EXISTS (
			SELECT 1 FROM role_permissions WHERE role = $1 AND obj = '/*' AND act = '.*'
		)
	`, defaultAdminRole); err != nil {
		return err
	}
	return nil
}

func (a *App) handleListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `
		SELECT u.id, u.sub, COALESCE(u.email, ''), u.provider, u.status, u.last_login,
		       COALESCE(json_agg(json_build_object('role', ur.role))
		                FILTER (WHERE ur.role IS NOT NULL), '[]') AS roles
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		GROUP BY u.id
		ORDER BY u.sub
	`)
	if err != nil {
		http.Error(w, "failed to list users", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var users []userSummary
	for rows.Next() {
		var u userSummary
		var rolesJSON []byte
		var lastLogin sql.NullTime
		if err := rows.Scan(&u.ID, &u.Sub, &u.Email, &u.Provider, &u.Status, &lastLogin, &rolesJSON); err != nil {
			http.Error(w, "failed to scan users", http.StatusInternalServerError)
			return
		}
		if lastLogin.Valid {
			t := lastLogin.Time
			u.LastLogin = &t
		}
		if len(rolesJSON) > 0 {
			_ = json.Unmarshal(rolesJSON, &u.Roles)
		}
		users = append(users, u)
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, users)
}

func (a *App) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req createUserRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	req.Sub = strings.TrimSpace(req.Sub)
	req.Email = strings.TrimSpace(req.Email)
	req.Role = strings.TrimSpace(req.Role)
	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("sub", req.Sub),
		httpapi.RequiredString("password", req.Password),
	); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	userID := uuid.New()
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	hashed, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}
	_, err = tx.Exec(r.Context(), `
		INSERT INTO users (id, sub, email, provider, password_hash, status)
		VALUES ($1, $2, $3, 'local', $4, 'active')
		ON CONFLICT (sub) DO UPDATE SET email = EXCLUDED.email, password_hash = EXCLUDED.password_hash, status = 'active'
	`, userID, req.Sub, req.Email, hashed)
	if err != nil {
		http.Error(w, "failed to create user", http.StatusInternalServerError)
		return
	}

	if req.Role != "" {
		_, err = tx.Exec(r.Context(), `
			INSERT INTO user_roles (user_id, role)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, userID, req.Role)
		if err != nil {
			http.Error(w, "failed to assign role", http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to save user", http.StatusInternalServerError)
		return
	}
	a.handleListUsers(w, r)
}

func (a *App) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userIDRaw := strings.TrimSpace(r.PathValue("userID"))
	if userIDRaw == "" {
		http.Error(w, "userID is required", http.StatusBadRequest)
		return
	}
	userID, err := uuid.Parse(userIDRaw)
	if err != nil {
		http.Error(w, "invalid userID", http.StatusBadRequest)
		return
	}
	var req updateUserRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Status = strings.TrimSpace(strings.ToLower(req.Status))
	req.Password = strings.TrimSpace(req.Password)

	var currentSub, currentProvider string
	err = a.db.QueryRow(r.Context(), `SELECT sub, provider FROM users WHERE id = $1`, userID).Scan(&currentSub, &currentProvider)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load user", http.StatusInternalServerError)
		return
	}

	if currentSub == defaultAdminSub && currentProvider == "local" && req.Status != "" && req.Status != "active" {
		http.Error(w, "cannot disable default admin user", http.StatusBadRequest)
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	setParts := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.Email != "" {
		setParts = append(setParts, fmt.Sprintf("email = $%d", argIdx))
		args = append(args, req.Email)
		argIdx++
	}

	if req.Status != "" {
		switch req.Status {
		case "active", "disabled":
			setParts = append(setParts, fmt.Sprintf("status = $%d", argIdx))
			args = append(args, req.Status)
			argIdx++
		default:
			http.Error(w, "invalid status", http.StatusBadRequest)
			return
		}
	}

	var hashedPassword string
	if req.Password != "" {
		if currentProvider != "local" {
			http.Error(w, "password changes are unavailable for this account", http.StatusBadRequest)
			return
		}
		hashedPassword, err = auth.HashPassword(req.Password)
		if err != nil {
			http.Error(w, "failed to hash password", http.StatusInternalServerError)
			return
		}
		setParts = append(setParts, fmt.Sprintf("password_hash = $%d", argIdx))
		args = append(args, hashedPassword)
		argIdx++
	}

	if len(setParts) == 0 {
		http.Error(w, "no fields to update", http.StatusBadRequest)
		return
	}

	args = append(args, userID)
	query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d", strings.Join(setParts, ", "), argIdx)
	if _, err := tx.Exec(r.Context(), query, args...); err != nil {
		http.Error(w, "failed to update user", http.StatusInternalServerError)
		return
	}

	if req.Password != "" {
		if _, err := tx.Exec(r.Context(), `DELETE FROM refresh_tokens WHERE user_id = $1`, userID); err != nil {
			http.Error(w, "failed to revoke refresh tokens", http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to save user", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := strings.TrimSpace(r.PathValue("userID"))
	if userID == "" {
		http.Error(w, "userID is required", http.StatusBadRequest)
		return
	}
	_, err := uuid.Parse(userID)
	if err != nil {
		http.Error(w, "invalid userID", http.StatusBadRequest)
		return
	}
	var sub, provider string
	err = a.db.QueryRow(r.Context(), `SELECT sub, provider FROM users WHERE id = $1`, userID).Scan(&sub, &provider)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load user", http.StatusInternalServerError)
		return
	}
	if sub == defaultAdminSub && provider == "local" {
		http.Error(w, "cannot delete default admin user", http.StatusBadRequest)
		return
	}
	tag, err := a.db.Exec(r.Context(), `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		http.Error(w, "failed to delete user", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleAddUserRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req userRoleRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	req.Role = strings.TrimSpace(req.Role)
	req.UserID = strings.TrimSpace(req.UserID)
	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("user_id", req.UserID),
		httpapi.RequiredString("role", req.Role),
	); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := a.db.Exec(r.Context(), `
		INSERT INTO user_roles (user_id, role)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, req.UserID, req.Role)
	if err != nil {
		http.Error(w, "failed to assign role", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeleteUserRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req userRoleRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	req.Role = strings.TrimSpace(req.Role)
	req.UserID = strings.TrimSpace(req.UserID)
	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("user_id", req.UserID),
		httpapi.RequiredString("role", req.Role),
	); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var sub, provider string
	if err := a.db.QueryRow(r.Context(), `SELECT sub, provider FROM users WHERE id = $1`, req.UserID).Scan(&sub, &provider); err == nil {
		if sub == defaultAdminSub && provider == "local" && req.Role == defaultAdminRole {
			http.Error(w, "cannot remove default admin role", http.StatusBadRequest)
			return
		}
	}
	tag, err := a.db.Exec(r.Context(), `
		DELETE FROM user_roles
		WHERE user_id = $1 AND role = $2
	`, req.UserID, req.Role)
	if err != nil {
		http.Error(w, "failed to remove role", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "role assignment not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleCreateRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req createRoleRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	req.Role = strings.TrimSpace(req.Role)
	req.Name = strings.TrimSpace(req.Name)
	req.Object = strings.TrimSpace(req.Object)
	req.Action = strings.TrimSpace(req.Action)
	if req.Role == defaultAdminRole && (req.Object != "/*" || req.Action != ".*") {
		http.Error(w, "default admin policy is fixed to /* and .*", http.StatusBadRequest)
		return
	}
	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("role", req.Role),
		httpapi.RequiredString("obj", req.Object),
		httpapi.RequiredString("act", req.Action),
	); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		req.Name = defaultPolicyName(req.Object, req.Action)
	}
	_, err := a.db.Exec(r.Context(), `
		INSERT INTO role_permissions (role, name, obj, act)
		VALUES ($1, $2, $3, $4)
	`, req.Role, req.Name, req.Object, req.Action)
	if err != nil {
		http.Error(w, "failed to create role permission", http.StatusInternalServerError)
		return
	}
	a.reloadAuthz(r.Context())
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req createRoleRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	req.Role = strings.TrimSpace(req.Role)
	req.Object = strings.TrimSpace(req.Object)
	req.Action = strings.TrimSpace(req.Action)
	if req.Role == defaultAdminRole && req.Object == "/*" && req.Action == ".*" {
		http.Error(w, "cannot delete default admin policy", http.StatusBadRequest)
		return
	}
	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("role", req.Role),
		httpapi.RequiredString("obj", req.Object),
		httpapi.RequiredString("act", req.Action),
	); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var assignments int
	if err := a.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM user_roles WHERE role = $1`, req.Role).Scan(&assignments); err != nil {
		http.Error(w, "failed to check role usage", http.StatusInternalServerError)
		return
	}
	if assignments > 0 {
		http.Error(w, "cannot delete policy from a role that is assigned to users", http.StatusBadRequest)
		return
	}
	tag, err := a.db.Exec(r.Context(), `
		DELETE FROM role_permissions
		WHERE role = $1 AND obj = $2 AND act = $3
	`, req.Role, req.Object, req.Action)
	if err != nil {
		http.Error(w, "failed to delete role permission", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "role permission not found", http.StatusNotFound)
		return
	}
	a.reloadAuthz(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleListRoles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT role, COALESCE(name, ''), obj, act
		FROM role_permissions
		ORDER BY role ASC, obj ASC, act ASC
	`)
	if err != nil {
		http.Error(w, "failed to list role permissions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type rolePerm struct {
		Role string `json:"role"`
		Name string `json:"name"`
		Obj  string `json:"obj"`
		Act  string `json:"act"`
	}
	var perms []rolePerm
	for rows.Next() {
		var p rolePerm
		if err := rows.Scan(&p.Role, &p.Name, &p.Obj, &p.Act); err != nil {
			http.Error(w, "failed to scan role permission", http.StatusInternalServerError)
			return
		}
		perms = append(perms, p)
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, perms)
}

func (a *App) reloadAuthz(ctx context.Context) {
	if a == nil || a.authz == nil {
		return
	}
	if err := a.authz.LoadPolicies(ctx, a.db); err != nil {
		log.Error().Err(err).Msg("failed to reload authz policies")
	}
}

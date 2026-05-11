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
	"nopsai/services/aaa/pkg/model"
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
		Roles:        a.resolveAAARoles(r.Context(), result.Claims),
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
		Roles:        a.resolveAAARoles(r.Context(), result.Claims),
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
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, authLoginResponse{
		AccessToken:  "",
		Roles:        a.resolveAAARoles(r.Context(), claims),
		Provider:     claims.Provider,
		Email:        claims.Email,
		Sub:          claims.Sub,
		Capabilities: a.authCapabilities(claims),
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

type authenticatedUserRecord struct {
	ID           uuid.UUID
	Provider     string
	Status       string
	PasswordHash sql.NullString
}

func (r authenticatedUserRecord) IsActive() bool {
	return strings.EqualFold(strings.TrimSpace(r.Status), "active")
}

func (a *App) loadAuthenticatedUserRecord(ctx context.Context, sub string) (authenticatedUserRecord, error) {
	var record authenticatedUserRecord
	err := a.db.QueryRow(ctx, `
		SELECT id, provider, status, password_hash
		FROM users
		WHERE sub = $1
	`, strings.TrimSpace(sub)).Scan(&record.ID, &record.Provider, &record.Status, &record.PasswordHash)
	return record, err
}

func normalizeOptionalEmail(raw string) (string, error) {
	email := strings.TrimSpace(raw)
	if email == "" {
		return "", nil
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil {
		return "", fmt.Errorf("invalid email")
	}
	return strings.TrimSpace(parsed.Address), nil
}

func (a *App) userEmailInUse(ctx context.Context, email string, excludeUserID *uuid.UUID) (bool, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return false, nil
	}

	var (
		query = `SELECT 1 FROM users WHERE LOWER(email) = LOWER($1)`
		args  = []any{email}
	)
	if excludeUserID != nil {
		query += ` AND id <> $2`
		args = append(args, *excludeUserID)
	}
	query += ` LIMIT 1`

	var exists int
	err := a.db.QueryRow(ctx, query, args...).Scan(&exists)
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, err
	default:
		return true, nil
	}
}

func (a *App) authCapabilities(claims *auth.Claims) *authCapabilitiesResponse {
	if claims == nil {
		return &authCapabilitiesResponse{}
	}
	if a == nil || !a.aaaAvailable() {
		return &authCapabilitiesResponse{}
	}

	subject := a.buildAAASubject(claims)
	ctx := context.Background()
	pipelineWrite := a.checkCapabilityOrScopedGrant(ctx, subject, "pipeline.update", model.ResourceRef{Type: "pipeline", ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "pipeline.create", model.ResourceRef{Type: "pipeline", ID: "*"})
	configRead := a.checkCapability(subject, "system.read", model.ResourceRef{Type: "system", ID: "config"}) &&
		a.checkCapability(subject, "system.read", model.ResourceRef{Type: "system", ID: "config-sync"})
	configWrite := a.checkCapability(subject, "system.update", model.ResourceRef{Type: "system", ID: "config"}) &&
		a.checkCapability(subject, "system.update", model.ResourceRef{Type: "system", ID: "config-sync"})
	dispatcherRead := a.checkCapability(subject, "system.read", model.ResourceRef{Type: "dispatcher", ID: "status"})
	dispatcherWrite := a.checkCapability(subject, "system.update", model.ResourceRef{Type: "dispatcher", ID: "runners"})
	triggerRead := a.checkCapabilityOrScopedGrant(ctx, subject, "trigger.read", model.ResourceRef{Type: "trigger", ID: "*"})
	triggerWrite := a.checkCapabilityOrScopedGrant(ctx, subject, "trigger.update", model.ResourceRef{Type: "trigger", ID: "*"})
	triggerDelete := a.checkCapabilityOrScopedGrant(ctx, subject, "trigger.delete", model.ResourceRef{Type: "trigger", ID: "*"})
	scopeRead := a.checkCapabilityOrScopedGrant(ctx, subject, "secret.list_metadata", model.ResourceRef{Type: "secret", ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "variable.list_metadata", model.ResourceRef{Type: "variable", ID: "*"}) ||
		a.hasScopedProductGrantCapability(ctx, subject, "scope.read")
	scopeWrite := a.checkCapabilityOrScopedGrant(ctx, subject, "scope.update", model.ResourceRef{Type: "scope", ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "secret.write_value", model.ResourceRef{Type: "secret", ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "variable.write_value", model.ResourceRef{Type: "variable", ID: "*"})
	scopeDelete := a.checkCapabilityOrScopedGrant(ctx, subject, "scope.delete", model.ResourceRef{Type: "scope", ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "secret.delete", model.ResourceRef{Type: "secret", ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "variable.delete", model.ResourceRef{Type: "variable", ID: "*"})

	return &authCapabilitiesResponse{
		Pipelines: authResourceCapabilities{
			Write:  pipelineWrite,
			Delete: a.checkCapabilityOrScopedGrant(ctx, subject, "pipeline.delete", model.ResourceRef{Type: "pipeline", ID: "*"}),
		},
		Steps: authResourceCapabilities{
			Write: a.checkCapabilityOrScopedGrant(ctx, subject, "step.update", model.ResourceRef{Type: "step", ID: "*"}) ||
				a.checkCapabilityOrScopedGrant(ctx, subject, "step.create", model.ResourceRef{Type: "step", ID: "*"}),
			Delete: a.checkCapabilityOrScopedGrant(ctx, subject, "step.delete", model.ResourceRef{Type: "step", ID: "*"}),
		},
		Triggers: authReadCapabilities{
			Read:   triggerRead,
			Write:  triggerWrite,
			Delete: triggerDelete,
		},
		Scopes: authReadCapabilities{
			Read:   scopeRead,
			Write:  scopeWrite,
			Delete: scopeDelete,
		},
		System: authSystemCapabilities{
			ConfigRead:      configRead,
			ConfigWrite:     configWrite,
			DispatcherRead:  dispatcherRead,
			DispatcherWrite: dispatcherWrite,
			Access:          a.checkCapability(subject, "iam.admin", model.ResourceRef{Type: "iam", ID: "admin"}),
		},
	}
}

func (a *App) checkCapabilityOrScopedGrant(ctx context.Context, subject model.Subject, action string, resource model.ResourceRef) bool {
	if a.checkCapability(subject, action, resource) {
		return true
	}
	return a.hasScopedProductGrantCapability(ctx, subject, action)
}

func (a *App) hasScopedProductGrantCapability(ctx context.Context, subject model.Subject, action string) bool {
	if a == nil || a.db == nil {
		return false
	}

	refs := a.scopedGrantSubjectRefs(ctx, subject)
	if len(refs) == 0 {
		return false
	}

	conditions := make([]string, 0, len(refs))
	args := make([]any, 0, len(refs)*2)
	for _, ref := range refs {
		if strings.TrimSpace(ref.Type) == "" || strings.TrimSpace(ref.ID) == "" {
			continue
		}
		args = append(args, ref.Type, ref.ID)
		conditions = append(conditions, fmt.Sprintf("(subject_type = $%d AND subject_id = $%d)", len(args)-1, len(args)))
	}
	if len(conditions) == 0 {
		return false
	}

	rows, err := a.db.Query(ctx, `
		SELECT role_name, resource_type
		FROM access_grants
		WHERE `+strings.Join(conditions, " OR "), args...)
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var roleName, resourceType string
		if err := rows.Scan(&roleName, &resourceType); err != nil {
			return false
		}
		if productGrantIncludesAction(roleName, resourceType, action) {
			return true
		}
	}
	return false
}

func (a *App) scopedGrantSubjectRefs(ctx context.Context, subject model.Subject) []model.SubjectRef {
	subjectType := model.NormalizeType(subject.Type)
	subjectID := strings.TrimSpace(subject.ID)

	switch subjectType {
	case model.SubjectTypeUser:
		resolvedID, err := a.lookupScopedGrantUserID(ctx, subject)
		if err != nil || resolvedID == "" {
			return nil
		}
		subjectID = resolvedID
	case model.SubjectTypeAuthGroup, model.SubjectTypeInternalService:
		if subjectID == "" {
			return nil
		}
	default:
		return nil
	}

	refs := []model.SubjectRef{{Type: subjectType, ID: subjectID}}
	rows, err := a.db.Query(ctx, `
		SELECT group_id::text
		FROM auth_group_members
		WHERE subject_type = $1 AND subject_id = $2
		ORDER BY group_id ASC
	`, subjectType, subjectID)
	if err != nil {
		return refs
	}
	defer rows.Close()

	for rows.Next() {
		var groupID string
		if err := rows.Scan(&groupID); err != nil {
			return refs
		}
		groupID = strings.TrimSpace(groupID)
		if groupID == "" {
			continue
		}
		refs = append(refs, model.SubjectRef{Type: model.SubjectTypeAuthGroup, ID: groupID})
	}
	return refs
}

func (a *App) lookupScopedGrantUserID(ctx context.Context, subject model.Subject) (string, error) {
	switch {
	case strings.TrimSpace(subject.ID) != "":
		return strings.TrimSpace(subject.ID), nil
	case strings.TrimSpace(subject.Sub) != "":
		var id string
		err := a.db.QueryRow(ctx, `SELECT id::text FROM users WHERE sub = $1 LIMIT 1`, strings.TrimSpace(subject.Sub)).Scan(&id)
		return id, err
	case strings.TrimSpace(subject.Email) != "":
		var id string
		err := a.db.QueryRow(ctx, `SELECT id::text FROM users WHERE LOWER(email) = LOWER($1) LIMIT 1`, strings.TrimSpace(subject.Email)).Scan(&id)
		return id, err
	default:
		return "", pgx.ErrNoRows
	}
}

func productGrantIncludesAction(roleName, resourceType, action string) bool {
	actions := applicableProductRoleActions(strings.TrimSpace(roleName), strings.TrimSpace(resourceType))
	for _, candidate := range actions {
		if candidate == "*" || candidate == action {
			return true
		}
	}
	return false
}

func (a *App) checkCapability(subject model.Subject, action string, resource model.ResourceRef) bool {
	if a == nil || !a.aaaAvailable() {
		return false
	}
	decision, err := a.aaaCheck(context.Background(), subject, action, resource, nil)
	return err == nil && decision.Allowed
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

	if _, err := tx.Exec(r.Context(), `UPDATE users SET password_hash = $1 WHERE id = $2`, hashed, userRecord.ID); err != nil {
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
		INSERT INTO auth_roles (name, description)
		VALUES ($1, 'Default platform administrator')
		ON CONFLICT (name) DO NOTHING
	`, defaultAdminRole); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO auth_role_bindings (role_name, subject_type, subject_id)
		VALUES ($1, 'user', $2)
		ON CONFLICT (role_name, subject_type, subject_id) DO NOTHING
	`, defaultAdminRole, existingID.String()); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO auth_role_permissions (role_name, resource_type, resource_id, action, effect)
		VALUES ($1, '*', '*', '*', 'allow')
		ON CONFLICT (role_name, resource_type, resource_id, action, effect) DO NOTHING
	`, defaultAdminRole); err != nil {
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
		SELECT
			u.id,
			u.sub,
			COALESCE(u.email, ''),
			u.provider,
			u.status,
			u.last_login,
			COALESCE((
				SELECT json_agg(json_build_object('role', roles.role_name) ORDER BY roles.role_name)
				FROM (
					SELECT DISTINCT rb.role_name
					FROM auth_role_bindings rb
					WHERE rb.subject_type = 'user' AND rb.subject_id = u.id::text
					UNION
					SELECT DISTINCT ur.role AS role_name
					FROM user_roles ur
					WHERE ur.user_id = u.id
				) roles
			), '[]'::json) AS roles
		FROM users u
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
	email, err := normalizeOptionalEmail(req.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Email = email
	req.Role = strings.TrimSpace(req.Role)
	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("sub", req.Sub),
		httpapi.RequiredString("password", req.Password),
	); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var excludeUserID *uuid.UUID
	var existingUserID uuid.UUID
	if err := a.db.QueryRow(r.Context(), `SELECT id FROM users WHERE sub = $1`, req.Sub).Scan(&existingUserID); err == nil {
		excludeUserID = &existingUserID
	} else if !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "failed to validate user", http.StatusInternalServerError)
		return
	}
	inUse, err := a.userEmailInUse(r.Context(), req.Email, excludeUserID)
	if err != nil {
		http.Error(w, "failed to validate email", http.StatusInternalServerError)
		return
	}
	if inUse {
		http.Error(w, "email already in use", http.StatusConflict)
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
	err = tx.QueryRow(r.Context(), `
		INSERT INTO users (id, sub, email, provider, password_hash, status)
		VALUES ($1, $2, $3, 'local', $4, 'active')
		ON CONFLICT (sub) DO UPDATE SET email = EXCLUDED.email, password_hash = EXCLUDED.password_hash, status = 'active'
		RETURNING id
	`, userID, req.Sub, req.Email, hashed).Scan(&userID)
	if err != nil {
		http.Error(w, "failed to create user", http.StatusInternalServerError)
		return
	}

	if req.Role != "" {
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO auth_roles (name, description)
			VALUES ($1, '')
			ON CONFLICT (name) DO NOTHING
		`, req.Role); err != nil {
			http.Error(w, "failed to prepare role", http.StatusInternalServerError)
			return
		}
		_, err = tx.Exec(r.Context(), `
			INSERT INTO user_roles (user_id, role)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, userID, req.Role)
		if err != nil {
			http.Error(w, "failed to assign role", http.StatusInternalServerError)
			return
		}
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO auth_role_bindings (role_name, subject_type, subject_id)
			VALUES ($1, 'user', $2)
			ON CONFLICT (role_name, subject_type, subject_id) DO NOTHING
		`, req.Role, userID.String()); err != nil {
			http.Error(w, "failed to assign aaa role", http.StatusInternalServerError)
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
	req.Email, err = normalizeOptionalEmail(req.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
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
	if req.Email != "" {
		inUse, err := a.userEmailInUse(r.Context(), req.Email, &userID)
		if err != nil {
			http.Error(w, "failed to validate email", http.StatusInternalServerError)
			return
		}
		if inUse {
			http.Error(w, "email already in use", http.StatusConflict)
			return
		}
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
	if _, err := uuid.Parse(req.UserID); err != nil {
		http.Error(w, "invalid user_id", http.StatusBadRequest)
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	if _, err := tx.Exec(r.Context(), `
		INSERT INTO auth_roles (name, description)
		VALUES ($1, '')
		ON CONFLICT (name) DO NOTHING
	`, req.Role); err != nil {
		http.Error(w, "failed to prepare role", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(r.Context(), `
			INSERT INTO user_roles (user_id, role)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, req.UserID, req.Role); err != nil {
		http.Error(w, "failed to assign role", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO auth_role_bindings (role_name, subject_type, subject_id)
		VALUES ($1, 'user', $2)
		ON CONFLICT (role_name, subject_type, subject_id) DO NOTHING
	`, req.Role, req.UserID); err != nil {
		http.Error(w, "failed to assign aaa role", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to save role assignment", http.StatusInternalServerError)
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
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	authTag, err := tx.Exec(r.Context(), `
		DELETE FROM auth_role_bindings
		WHERE role_name = $1 AND subject_type = 'user' AND subject_id = $2
	`, req.Role, req.UserID)
	if err != nil {
		http.Error(w, "failed to remove aaa role", http.StatusInternalServerError)
		return
	}
	legacyTag, err := tx.Exec(r.Context(), `
		DELETE FROM user_roles
		WHERE user_id = $1 AND role = $2
	`, req.UserID, req.Role)
	if err != nil {
		http.Error(w, "failed to remove role", http.StatusInternalServerError)
		return
	}
	if authTag.RowsAffected() == 0 && legacyTag.RowsAffected() == 0 {
		http.Error(w, "role assignment not found", http.StatusNotFound)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to save role removal", http.StatusInternalServerError)
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
	permission, err := parseAdminRolePermission(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := httpapi.ValidateRequired(httpapi.RequiredString("role", permission.Role)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if permission.Role == defaultAdminRole &&
		(permission.ResourceType != "*" || permission.ResourceID != "*" || permission.Action != "*" || permission.Effect != "allow") {
		http.Error(w, "default admin policy is fixed to *:* and *", http.StatusBadRequest)
		return
	}

	objectValue := formatAdminPermissionObject(permission.ResourceType, permission.ResourceID)
	actionValue := formatAdminPermissionAction(permission.Effect, permission.Action)
	displayName := adminPermissionDisplayName(permission.Name, objectValue, actionValue)

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	description := ""
	if permission.Role == policyTemplateRole {
		description = "Reusable UI policy templates"
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO auth_roles (name, description)
		VALUES ($1, $2)
		ON CONFLICT (name) DO NOTHING
	`, permission.Role, description); err != nil {
		http.Error(w, "failed to prepare aaa role", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO auth_role_permissions (role_name, resource_type, resource_id, action, effect)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (role_name, resource_type, resource_id, action, effect) DO NOTHING
	`, permission.Role, permission.ResourceType, permission.ResourceID, permission.Action, permission.Effect); err != nil {
		http.Error(w, "failed to create aaa role permission", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		DELETE FROM role_permissions
		WHERE role = $1 AND obj = $2 AND act = $3
	`, permission.Role, objectValue, actionValue); err != nil {
		http.Error(w, "failed to refresh legacy role metadata", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO role_permissions (role, name, obj, act)
		VALUES ($1, $2, $3, $4)
	`, permission.Role, displayName, objectValue, actionValue); err != nil {
		http.Error(w, "failed to create role metadata", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to save role permission", http.StatusInternalServerError)
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
	permission, err := parseAdminRolePermission(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := httpapi.ValidateRequired(httpapi.RequiredString("role", permission.Role)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if permission.Role == defaultAdminRole &&
		permission.ResourceType == "*" && permission.ResourceID == "*" && permission.Action == "*" && permission.Effect == "allow" {
		http.Error(w, "cannot delete default admin policy", http.StatusBadRequest)
		return
	}
	var assignments int
	if err := a.db.QueryRow(r.Context(), `
		SELECT
			COALESCE((SELECT COUNT(*) FROM auth_role_bindings WHERE role_name = $1), 0) +
			COALESCE((SELECT COUNT(*) FROM user_roles WHERE role = $1), 0)
	`, permission.Role).Scan(&assignments); err != nil {
		http.Error(w, "failed to check role usage", http.StatusInternalServerError)
		return
	}
	if assignments > 0 {
		http.Error(w, "cannot delete policy from a role that is assigned to users", http.StatusBadRequest)
		return
	}

	objectValue := formatAdminPermissionObject(permission.ResourceType, permission.ResourceID)
	actionValue := formatAdminPermissionAction(permission.Effect, permission.Action)

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	authTag, err := tx.Exec(r.Context(), `
		DELETE FROM auth_role_permissions
		WHERE role_name = $1 AND resource_type = $2 AND resource_id = $3 AND action = $4 AND effect = $5
	`, permission.Role, permission.ResourceType, permission.ResourceID, permission.Action, permission.Effect)
	if err != nil {
		http.Error(w, "failed to delete aaa role permission", http.StatusInternalServerError)
		return
	}
	legacyTag, err := tx.Exec(r.Context(), `
		DELETE FROM role_permissions
		WHERE role = $1 AND obj = $2 AND act = $3
	`, permission.Role, objectValue, actionValue)
	if err != nil {
		http.Error(w, "failed to delete role metadata", http.StatusInternalServerError)
		return
	}
	if authTag.RowsAffected() == 0 && legacyTag.RowsAffected() == 0 {
		http.Error(w, "role permission not found", http.StatusNotFound)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		DELETE FROM auth_roles
		WHERE name = $1
		  AND NOT EXISTS (SELECT 1 FROM auth_role_permissions WHERE role_name = $1)
		  AND NOT EXISTS (SELECT 1 FROM auth_role_bindings WHERE role_name = $1)
	`, permission.Role); err != nil {
		http.Error(w, "failed to clean up empty role", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to save role deletion", http.StatusInternalServerError)
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
	metadataRows, err := a.db.Query(r.Context(), `
		SELECT role, COALESCE(name, ''), obj, act
		FROM role_permissions
	`)
	if err != nil {
		http.Error(w, "failed to list role metadata", http.StatusInternalServerError)
		return
	}
	defer metadataRows.Close()
	nameByKey := make(map[string]string)
	for metadataRows.Next() {
		var role, name, objectValue, actionValue string
		if err := metadataRows.Scan(&role, &name, &objectValue, &actionValue); err != nil {
			http.Error(w, "failed to scan role metadata", http.StatusInternalServerError)
			return
		}
		nameByKey[adminPermissionMetadataKey(role, objectValue, actionValue)] = strings.TrimSpace(name)
	}

	rows, err := a.db.Query(r.Context(), `
		SELECT role_name, resource_type, resource_id, action, effect
		FROM auth_role_permissions
		ORDER BY role_name ASC, resource_type ASC, resource_id ASC, effect ASC, action ASC
	`)
	if err != nil {
		http.Error(w, "failed to list aaa role permissions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type rolePerm struct {
		Role   string `json:"role"`
		Name   string `json:"name"`
		Obj    string `json:"obj"`
		Act    string `json:"act"`
		Effect string `json:"effect,omitempty"`
	}
	var perms []rolePerm
	for rows.Next() {
		var roleName, resourceType, resourceID, action, effect string
		if err := rows.Scan(&roleName, &resourceType, &resourceID, &action, &effect); err != nil {
			http.Error(w, "failed to scan aaa role permission", http.StatusInternalServerError)
			return
		}
		objectValue := formatAdminPermissionObject(resourceType, resourceID)
		actionValue := formatAdminPermissionAction(effect, action)
		name := nameByKey[adminPermissionMetadataKey(roleName, objectValue, actionValue)]
		if name == "" && roleName == defaultAdminRole && objectValue == "*:*" && actionValue == "*" {
			name = nameByKey[adminPermissionMetadataKey(roleName, "/*", ".*")]
		}
		perms = append(perms, rolePerm{
			Role:   roleName,
			Name:   adminPermissionDisplayName(name, objectValue, actionValue),
			Obj:    objectValue,
			Act:    actionValue,
			Effect: effect,
		})
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

package nopsai

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"nopsai/pkg/httpapi"
	aaastore "nopsai/services/aaa/pkg/store"
	"nopsai/services/nopsai/pkg/auth"
)

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
				), '[]'::json) AS roles,
			CASE
				WHEN COALESCE(ext.external_managed, LOWER(u.provider) LIKE 'oidc:%' OR LOWER(u.provider) LIKE 'oauth2:%')
					THEN COALESCE(NULLIF(u.email, ''), NULLIF(u.sub, ''), u.id::text)
				ELSE COALESCE(NULLIF(u.sub, ''), NULLIF(u.email, ''), u.id::text)
			END AS display_name,
			COALESCE(ext.external_managed, LOWER(u.provider) LIKE 'oidc:%' OR LOWER(u.provider) LIKE 'oauth2:%') AS external_managed,
			COALESCE(ext.provider_id, '') AS external_provider_id,
				COALESCE(ext.provider_name, '') AS external_provider_name,
				COALESCE(ext.subject, '') AS external_subject,
				COALESCE(ext.external_teams, '[]'::json) AS external_teams,
				COALESCE(ext.external_auth_teams, '[]'::json) AS external_auth_teams,
				COALESCE(ext.external_roles, '[]'::json) AS external_roles
		FROM users u
		LEFT JOIN LATERAL (
			SELECT
				TRUE AS external_managed,
				ei.provider_id,
				COALESCE(NULLIF(ip.display_name, ''), ei.provider_id) AS provider_name,
				ei.subject,
					COALESCE((
						SELECT json_agg(teams.team_name ORDER BY teams.team_name)
					FROM (
						SELECT DISTINCT team_name
						FROM auth_external_team_memberships
						WHERE user_id = u.id AND provider_id = ei.provider_id
						) teams
					), '[]'::json) AS external_teams,
					COALESCE((
						SELECT json_agg(json_build_object('id', teams.id, 'name', teams.name) ORDER BY teams.name)
						FROM (
							SELECT DISTINCT g.id::text AS id, g.name
							FROM auth_team_members m
							JOIN auth_teams g ON g.id = m.team_id
							WHERE m.subject_type = 'user'
							  AND m.subject_id = u.id::text
							  AND m.managed_by_identity_provider = TRUE
							  AND m.identity_provider_id = ei.provider_id
						) teams
					), '[]'::json) AS external_auth_teams,
					COALESCE((
					SELECT json_agg(roles.role_name ORDER BY roles.role_name)
					FROM (
						SELECT DISTINCT role_name
						FROM auth_external_role_assignments
						WHERE user_id = u.id AND provider_id = ei.provider_id
					) roles
				), '[]'::json) AS external_roles
			FROM auth_external_identities ei
			LEFT JOIN auth_identity_providers ip ON ip.id = ei.provider_id
			WHERE ei.user_id = u.id
			ORDER BY ei.last_login_at DESC NULLS LAST, ei.linked_at DESC
			LIMIT 1
		) ext ON TRUE
		WHERE u.provider <> $1
		ORDER BY u.sub
	`, auth.ProviderServiceAccount)
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
		var externalTeamsJSON, externalAuthTeamsJSON, externalRolesJSON []byte
		if err := rows.Scan(
			&u.ID,
			&u.Sub,
			&u.Email,
			&u.Provider,
			&u.Status,
			&lastLogin,
			&rolesJSON,
			&u.DisplayName,
			&u.ExternalManaged,
			&u.ExternalProviderID,
			&u.ExternalProviderName,
			&u.ExternalSubject,
			&externalTeamsJSON,
			&externalAuthTeamsJSON,
			&externalRolesJSON,
		); err != nil {
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
		if len(externalTeamsJSON) > 0 {
			_ = json.Unmarshal(externalTeamsJSON, &u.ExternalTeams)
		}
		if len(externalAuthTeamsJSON) > 0 {
			_ = json.Unmarshal(externalAuthTeamsJSON, &u.ExternalAuthTeams)
		}
		if len(externalRolesJSON) > 0 {
			_ = json.Unmarshal(externalRolesJSON, &u.ExternalRoles)
		}
		normalizeUserSummaryIdentity(&u)
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
	if strings.EqualFold(req.Sub, defaultAdminSub) && req.Role != "" {
		http.Error(w, "cannot modify default admin role assignments", http.StatusBadRequest)
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
		INSERT INTO users (id, sub, email, provider, password_hash, status, must_change_password)
		VALUES ($1, $2, $3, 'local', $4, 'active', TRUE)
		ON CONFLICT (sub) DO UPDATE SET email = EXCLUDED.email, password_hash = EXCLUDED.password_hash, status = 'active', must_change_password = TRUE
		RETURNING id
	`, userID, req.Sub, req.Email, hashed).Scan(&userID)
	if err != nil {
		http.Error(w, "failed to create user", http.StatusInternalServerError)
		return
	}

	if req.Role != "" {
		if err := aaastore.EnsureRole(r.Context(), tx, req.Role, ""); err != nil {
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
		if err := aaastore.EnsureRoleBinding(r.Context(), tx, aaastore.RoleBinding{
			RoleName:    req.Role,
			SubjectType: "user",
			SubjectID:   userID.String(),
		}); err != nil {
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
		setParts = append(setParts, fmt.Sprintf("must_change_password = $%d", argIdx))
		args = append(args, true)
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
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to delete user", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	var sub, provider string
	err = tx.QueryRow(r.Context(), `SELECT sub, provider FROM users WHERE id = $1`, userID).Scan(&sub, &provider)
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
	if err := deleteUserAccessArtifacts(r.Context(), tx, userID); err != nil {
		http.Error(w, "failed to delete user access", http.StatusInternalServerError)
		return
	}
	tag, err := tx.Exec(r.Context(), `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		http.Error(w, "failed to delete user", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to delete user", http.StatusInternalServerError)
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
	var sub, provider string
	if err := a.db.QueryRow(r.Context(), `SELECT sub, provider FROM users WHERE id = $1`, req.UserID).Scan(&sub, &provider); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load user", http.StatusInternalServerError)
		return
	}
	if sub == defaultAdminSub && provider == "local" {
		http.Error(w, "cannot modify default admin role assignments", http.StatusBadRequest)
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	if err := aaastore.EnsureRole(r.Context(), tx, req.Role, ""); err != nil {
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
	if err := aaastore.EnsureRoleBinding(r.Context(), tx, aaastore.RoleBinding{
		RoleName:    req.Role,
		SubjectType: "user",
		SubjectID:   req.UserID,
	}); err != nil {
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
	if err := a.db.QueryRow(r.Context(), `SELECT sub, provider FROM users WHERE id = $1`, req.UserID).Scan(&sub, &provider); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load user", http.StatusInternalServerError)
		return
	}
	if sub == defaultAdminSub && provider == "local" {
		http.Error(w, "cannot modify default admin role assignments", http.StatusBadRequest)
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	authTag, err := aaastore.DeleteRoleBinding(r.Context(), tx, aaastore.RoleBinding{
		RoleName:    req.Role,
		SubjectType: "user",
		SubjectID:   req.UserID,
	})
	if err != nil {
		http.Error(w, "failed to remove aaa role", http.StatusInternalServerError)
		return
	}
	legacyTag, err := tx.Exec(r.Context(), `
		DELETE FROM user_roles
		WHERE user_id = $1 AND role = $2
		  AND NOT EXISTS (
			SELECT 1
			FROM auth_external_role_assignments
			WHERE user_id = $1::uuid AND role_name = $2
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM auth_role_bindings
			WHERE subject_type = 'user'
			  AND subject_id = $1
			  AND role_name = $2
			  AND source = 'idp'
		  )
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

func normalizeUserSummaryIdentity(user *userSummary) {
	if user == nil {
		return
	}
	user.Provider = strings.TrimSpace(user.Provider)
	user.Sub = strings.TrimSpace(user.Sub)
	user.Email = strings.TrimSpace(user.Email)
	user.DisplayName = strings.TrimSpace(user.DisplayName)
	user.ExternalProviderID = strings.TrimSpace(user.ExternalProviderID)
	user.ExternalProviderName = strings.TrimSpace(user.ExternalProviderName)
	user.ExternalSubject = strings.TrimSpace(user.ExternalSubject)

	providerLower := strings.ToLower(user.Provider)
	if strings.HasPrefix(providerLower, "oidc:") || strings.HasPrefix(providerLower, "oauth2:") {
		user.ExternalManaged = true
		if user.ExternalProviderID == "" {
			_, providerID, _ := strings.Cut(user.Provider, ":")
			user.ExternalProviderID = strings.TrimSpace(providerID)
		}
	}
	if user.ExternalManaged {
		user.AuthenticationSource = grantSourceIDP
		if strings.HasPrefix(providerLower, "oidc:") || strings.HasPrefix(providerLower, "oauth2:") {
			user.ProvisioningSource = grantSourceIDP
		} else {
			user.ProvisioningSource = grantSourceLocal
		}
		if user.ExternalProviderName == "" {
			user.ExternalProviderName = user.ExternalProviderID
		}
		if user.ExternalSubject == "" && user.ExternalProviderID != "" {
			for _, prefix := range []string{"oidc:" + user.ExternalProviderID + ":", "oauth2:" + user.ExternalProviderID + ":"} {
				if strings.HasPrefix(user.Sub, prefix) {
					user.ExternalSubject = strings.TrimSpace(strings.TrimPrefix(user.Sub, prefix))
					break
				}
			}
		}
		user.AuthorizationSources = appendOwnershipSource(user.AuthorizationSources, grantSourceIDP)
		if len(user.Roles) > len(user.ExternalRoles) {
			user.AuthorizationSources = appendOwnershipSource(user.AuthorizationSources, grantSourceLocal)
		}
		user.DisplayName = firstNonEmptyString(user.DisplayName, user.Email, user.Sub, user.ID)
		return
	}
	user.AuthenticationSource = grantSourceLocal
	user.ProvisioningSource = grantSourceLocal
	user.AuthorizationSources = appendOwnershipSource(user.AuthorizationSources, grantSourceLocal)
	user.DisplayName = firstNonEmptyString(user.DisplayName, user.Sub, user.Email, user.ID)
}

func appendOwnershipSource(values []string, source string) []string {
	source = strings.TrimSpace(source)
	if source == "" {
		return values
	}
	for _, existing := range values {
		if strings.TrimSpace(existing) == source {
			return values
		}
	}
	return append(values, source)
}

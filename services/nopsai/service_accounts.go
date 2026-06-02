package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"nopsai/pkg/httpapi"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/auth"
)

const (
	defaultServiceAccountTokenName = "default"
	maxServiceAccountSubLen        = 120
)

type serviceAccountIdentity struct {
	ID     uuid.UUID
	Sub    string
	Email  string
	Status string
}

func (a *App) handleListServiceAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT
			u.id::text,
			u.sub,
			COALESCE(u.email, ''),
			u.provider,
			u.status,
			COALESCE(COUNT(sat.id) FILTER (WHERE sat.revoked_at IS NULL), 0),
			MAX(sat.last_used_at) FILTER (WHERE sat.revoked_at IS NULL),
			COALESCE((
				SELECT json_agg(json_build_object('role', roles.role_name) ORDER BY roles.role_name)
				FROM (
					SELECT DISTINCT rb.role_name
					FROM auth_role_bindings rb
					WHERE rb.subject_type = $1 AND rb.subject_id = u.sub
				) roles
			), '[]'::json) AS roles
		FROM users u
		LEFT JOIN service_account_tokens sat ON sat.service_account_id = u.id
		WHERE u.provider = $2
		GROUP BY u.id, u.sub, u.email, u.provider, u.status
		ORDER BY u.sub
	`, model.SubjectTypeServiceAccount, auth.ProviderServiceAccount)
	if err != nil {
		http.Error(w, "failed to list service accounts", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	accounts := []serviceAccountSummary{}
	for rows.Next() {
		account, err := scanServiceAccountSummary(rows)
		if err != nil {
			http.Error(w, "failed to read service accounts", http.StatusInternalServerError)
			return
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read service accounts", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, accounts)
}

func (a *App) handleCreateServiceAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req createServiceAccountRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	sub, err := validateServiceAccountSub(req.Sub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	email, err := normalizeOptionalEmail(req.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	role := strings.TrimSpace(req.Role)
	tokenName := strings.TrimSpace(req.TokenName)
	if tokenName == "" {
		tokenName = defaultServiceAccountTokenName
	}

	var existing int
	err = a.db.QueryRow(r.Context(), `SELECT 1 FROM users WHERE sub = $1 LIMIT 1`, sub).Scan(&existing)
	switch {
	case err == nil:
		http.Error(w, "sub already in use", http.StatusConflict)
		return
	case !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows):
		http.Error(w, "failed to validate service account", http.StatusInternalServerError)
		return
	}
	inUse, err := a.userEmailInUse(r.Context(), email, nil)
	if err != nil {
		http.Error(w, "failed to validate email", http.StatusInternalServerError)
		return
	}
	if inUse {
		http.Error(w, "email already in use", http.StatusConflict)
		return
	}

	tokenReq := serviceAccountTokenRequest{
		Name:          tokenName,
		ExpiresInDays: req.ExpiresInDays,
		ExpiresAt:     req.ExpiresAt,
		NeverExpires:  req.NeverExpires,
	}
	if _, err := validatePersonalAccessTokenName(tokenReq.Name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := resolveServiceAccountTokenExpiry(tokenReq, time.Now()); err != nil {
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

	if _, err := tx.Exec(r.Context(), `
		INSERT INTO users (id, sub, email, provider, password_hash, status, must_change_password)
		VALUES ($1, $2, $3, $4, NULL, 'active', FALSE)
	`, userID, sub, email, auth.ProviderServiceAccount); err != nil {
		http.Error(w, "failed to create service account", http.StatusInternalServerError)
		return
	}

	if role != "" {
		if err := assignServiceAccountRole(r.Context(), tx, sub, role); err != nil {
			http.Error(w, "failed to assign role", http.StatusInternalServerError)
			return
		}
	}

	token, err := createServiceAccountToken(r.Context(), tx, userID, tokenReq)
	if err != nil {
		http.Error(w, "failed to create service account token", http.StatusInternalServerError)
		return
	}
	account, err := loadServiceAccountSummary(r.Context(), tx, userID)
	if err != nil {
		http.Error(w, "failed to load service account", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to save service account", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusCreated, createServiceAccountResponse{
		ServiceAccount: account,
		Token:          token,
	})
}

func (a *App) handleUpdateServiceAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	serviceAccountID, err := uuid.Parse(strings.TrimSpace(r.PathValue("serviceAccountID")))
	if err != nil {
		http.Error(w, "invalid serviceAccountID", http.StatusBadRequest)
		return
	}
	var req updateServiceAccountRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	email, err := normalizeOptionalEmail(req.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))

	if _, err := loadServiceAccountIdentity(r.Context(), a.db, serviceAccountID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "service account not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load service account", http.StatusInternalServerError)
		return
	}
	if email != "" {
		inUse, err := a.userEmailInUse(r.Context(), email, &serviceAccountID)
		if err != nil {
			http.Error(w, "failed to validate email", http.StatusInternalServerError)
			return
		}
		if inUse {
			http.Error(w, "email already in use", http.StatusConflict)
			return
		}
	}

	setParts := []string{}
	args := []interface{}{}
	argIdx := 1
	if email != "" {
		setParts = append(setParts, fmt.Sprintf("email = $%d", argIdx))
		args = append(args, email)
		argIdx++
	}
	if status != "" {
		switch status {
		case "active", "disabled":
			setParts = append(setParts, fmt.Sprintf("status = $%d", argIdx))
			args = append(args, status)
			argIdx++
		default:
			http.Error(w, "invalid status", http.StatusBadRequest)
			return
		}
	}
	if len(setParts) == 0 {
		http.Error(w, "no fields to update", http.StatusBadRequest)
		return
	}
	args = append(args, serviceAccountID, auth.ProviderServiceAccount)
	query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d AND provider = $%d", strings.Join(setParts, ", "), argIdx, argIdx+1)
	if _, err := a.db.Exec(r.Context(), query, args...); err != nil {
		http.Error(w, "failed to update service account", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteServiceAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	serviceAccountID, err := uuid.Parse(strings.TrimSpace(r.PathValue("serviceAccountID")))
	if err != nil {
		http.Error(w, "invalid serviceAccountID", http.StatusBadRequest)
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	identity, err := loadServiceAccountIdentity(r.Context(), tx, serviceAccountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "service account not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load service account", http.StatusInternalServerError)
		return
	}
	if err := deleteServiceAccountAccessArtifacts(r.Context(), tx, identity.Sub); err != nil {
		http.Error(w, "failed to delete service account access", http.StatusInternalServerError)
		return
	}
	tag, err := tx.Exec(r.Context(), `DELETE FROM users WHERE id = $1 AND provider = $2`, serviceAccountID, auth.ProviderServiceAccount)
	if err != nil {
		http.Error(w, "failed to delete service account", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "service account not found", http.StatusNotFound)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to delete service account", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleListServiceAccountTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	serviceAccountID, err := uuid.Parse(strings.TrimSpace(r.PathValue("serviceAccountID")))
	if err != nil {
		http.Error(w, "invalid serviceAccountID", http.StatusBadRequest)
		return
	}
	if _, err := loadServiceAccountIdentity(r.Context(), a.db, serviceAccountID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "service account not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load service account", http.StatusInternalServerError)
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT id::text, name, token_suffix, created_at, expires_at, last_used_at
		FROM service_account_tokens
		WHERE service_account_id = $1 AND revoked_at IS NULL
		ORDER BY created_at DESC
	`, serviceAccountID)
	if err != nil {
		http.Error(w, "failed to list service account tokens", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	tokens := []serviceAccountTokenResponse{}
	for rows.Next() {
		token, err := scanServiceAccountToken(rows)
		if err != nil {
			http.Error(w, "failed to read service account tokens", http.StatusInternalServerError)
			return
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read service account tokens", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, tokens)
}

func (a *App) handleCreateServiceAccountToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	serviceAccountID, err := uuid.Parse(strings.TrimSpace(r.PathValue("serviceAccountID")))
	if err != nil {
		http.Error(w, "invalid serviceAccountID", http.StatusBadRequest)
		return
	}
	var req serviceAccountTokenRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if _, err := loadServiceAccountIdentity(r.Context(), a.db, serviceAccountID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "service account not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load service account", http.StatusInternalServerError)
		return
	}
	token, err := createServiceAccountToken(r.Context(), a.db, serviceAccountID, req)
	if err != nil {
		status := http.StatusInternalServerError
		message := "failed to create service account token"
		if isServiceAccountRequestError(err) {
			status = http.StatusBadRequest
			message = err.Error()
		}
		http.Error(w, message, status)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusCreated, token)
}

func (a *App) handleRevokeServiceAccountToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	serviceAccountID, err := uuid.Parse(strings.TrimSpace(r.PathValue("serviceAccountID")))
	if err != nil {
		http.Error(w, "invalid serviceAccountID", http.StatusBadRequest)
		return
	}
	tokenID, err := uuid.Parse(strings.TrimSpace(r.PathValue("tokenID")))
	if err != nil {
		http.Error(w, "invalid token id", http.StatusBadRequest)
		return
	}
	tag, err := a.db.Exec(r.Context(), `
		UPDATE service_account_tokens
		SET revoked_at = NOW()
		WHERE id = $1 AND service_account_id = $2 AND revoked_at IS NULL
	`, tokenID, serviceAccountID)
	if err != nil {
		http.Error(w, "failed to revoke service account token", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "service account token not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleAddServiceAccountRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req serviceAccountRoleRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	serviceAccountID, role, ok := validateServiceAccountRoleRequest(w, req)
	if !ok {
		return
	}
	identity, err := loadServiceAccountIdentity(r.Context(), a.db, serviceAccountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "service account not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load service account", http.StatusInternalServerError)
		return
	}
	if err := assignServiceAccountRole(r.Context(), a.db, identity.Sub, role); err != nil {
		http.Error(w, "failed to assign role", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeleteServiceAccountRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req serviceAccountRoleRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	serviceAccountID, role, ok := validateServiceAccountRoleRequest(w, req)
	if !ok {
		return
	}
	identity, err := loadServiceAccountIdentity(r.Context(), a.db, serviceAccountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "service account not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load service account", http.StatusInternalServerError)
		return
	}
	tag, err := a.db.Exec(r.Context(), `
		DELETE FROM auth_role_bindings
		WHERE role_name = $1 AND subject_type = $2 AND subject_id = $3
	`, role, model.SubjectTypeServiceAccount, identity.Sub)
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

func validateServiceAccountRoleRequest(w http.ResponseWriter, req serviceAccountRoleRequest) (uuid.UUID, string, bool) {
	role := strings.TrimSpace(req.Role)
	rawID := strings.TrimSpace(req.ServiceAccountID)
	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("service_account_id", rawID),
		httpapi.RequiredString("role", role),
	); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return uuid.Nil, "", false
	}
	serviceAccountID, err := uuid.Parse(rawID)
	if err != nil {
		http.Error(w, "invalid service_account_id", http.StatusBadRequest)
		return uuid.Nil, "", false
	}
	return serviceAccountID, role, true
}

func validateServiceAccountSub(raw string) (string, error) {
	sub := strings.Trim(strings.TrimSpace(raw), "/")
	if sub == "" {
		return "", fmt.Errorf("sub is required")
	}
	if len(sub) > maxServiceAccountSubLen {
		return "", fmt.Errorf("sub must be 120 characters or fewer")
	}
	if strings.ContainsAny(sub, "\r\n\t ") {
		return "", fmt.Errorf("sub cannot contain whitespace or control characters")
	}
	if strings.EqualFold(sub, defaultAdminSub) {
		return "", fmt.Errorf("sub is reserved")
	}
	return sub, nil
}

func resolveServiceAccountTokenExpiry(req serviceAccountTokenRequest, now time.Time) (*time.Time, error) {
	return resolvePersonalAccessTokenExpiry(authPersonalTokenRequest{
		ExpiresInDays: req.ExpiresInDays,
		ExpiresAt:     req.ExpiresAt,
		NeverExpires:  req.NeverExpires,
	}, now)
}

func isServiceAccountRequestError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "name ") ||
		strings.Contains(msg, "name is required") ||
		strings.Contains(msg, "expires_") ||
		strings.Contains(msg, "expires_at")
}

func createServiceAccountToken(ctx context.Context, runner queryRunner, serviceAccountID uuid.UUID, req serviceAccountTokenRequest) (serviceAccountTokenResponse, error) {
	name, err := validatePersonalAccessTokenName(req.Name)
	if err != nil {
		return serviceAccountTokenResponse{}, err
	}
	expiresAt, err := resolveServiceAccountTokenExpiry(req, time.Now())
	if err != nil {
		return serviceAccountTokenResponse{}, err
	}
	rawToken, err := auth.GenerateServiceAccountToken()
	if err != nil {
		return serviceAccountTokenResponse{}, err
	}
	tokenID := uuid.New()
	token := serviceAccountTokenResponse{
		ID:          tokenID.String(),
		Name:        name,
		Token:       rawToken,
		TokenSuffix: auth.PersonalAccessTokenSuffix(rawToken),
		ExpiresAt:   expiresAt,
	}
	var expiresArg any
	if expiresAt != nil {
		expiresArg = *expiresAt
	}
	var returnedExpiresAt sql.NullTime
	err = runner.QueryRow(ctx, `
		INSERT INTO service_account_tokens (id, service_account_id, name, token_hash, token_suffix, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, expires_at
	`, tokenID, serviceAccountID, name, auth.HashToken(rawToken), token.TokenSuffix, expiresArg).Scan(&token.CreatedAt, &returnedExpiresAt)
	if err != nil {
		return serviceAccountTokenResponse{}, err
	}
	if returnedExpiresAt.Valid {
		expires := returnedExpiresAt.Time
		token.ExpiresAt = &expires
	}
	return token, nil
}

func assignServiceAccountRole(ctx context.Context, runner queryRunner, serviceAccountSub, role string) error {
	role = strings.TrimSpace(role)
	serviceAccountSub = strings.TrimSpace(serviceAccountSub)
	if role == "" || serviceAccountSub == "" {
		return nil
	}
	if _, err := runner.Exec(ctx, `
		INSERT INTO auth_roles (name, description)
		VALUES ($1, '')
		ON CONFLICT (name) DO NOTHING
	`, role); err != nil {
		return err
	}
	_, err := runner.Exec(ctx, `
		INSERT INTO auth_role_bindings (role_name, subject_type, subject_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (role_name, subject_type, subject_id) DO NOTHING
	`, role, model.SubjectTypeServiceAccount, serviceAccountSub)
	return err
}

func loadServiceAccountIdentity(ctx context.Context, runner queryRunner, serviceAccountID uuid.UUID) (serviceAccountIdentity, error) {
	var identity serviceAccountIdentity
	err := runner.QueryRow(ctx, `
		SELECT id, sub, COALESCE(email, ''), status
		FROM users
		WHERE id = $1 AND provider = $2
	`, serviceAccountID, auth.ProviderServiceAccount).Scan(&identity.ID, &identity.Sub, &identity.Email, &identity.Status)
	return identity, err
}

func loadServiceAccountSummary(ctx context.Context, runner queryRunner, serviceAccountID uuid.UUID) (serviceAccountSummary, error) {
	row := runner.QueryRow(ctx, `
		SELECT
			u.id::text,
			u.sub,
			COALESCE(u.email, ''),
			u.provider,
			u.status,
			COALESCE(COUNT(sat.id) FILTER (WHERE sat.revoked_at IS NULL), 0),
			MAX(sat.last_used_at) FILTER (WHERE sat.revoked_at IS NULL),
			COALESCE((
				SELECT json_agg(json_build_object('role', roles.role_name) ORDER BY roles.role_name)
				FROM (
					SELECT DISTINCT rb.role_name
					FROM auth_role_bindings rb
					WHERE rb.subject_type = $1 AND rb.subject_id = u.sub
				) roles
			), '[]'::json) AS roles
		FROM users u
		LEFT JOIN service_account_tokens sat ON sat.service_account_id = u.id
		WHERE u.id = $2 AND u.provider = $3
		GROUP BY u.id, u.sub, u.email, u.provider, u.status
	`, model.SubjectTypeServiceAccount, serviceAccountID, auth.ProviderServiceAccount)
	return scanServiceAccountSummary(row)
}

type serviceAccountSummaryScanner interface {
	Scan(dest ...any) error
}

func scanServiceAccountSummary(scanner serviceAccountSummaryScanner) (serviceAccountSummary, error) {
	var account serviceAccountSummary
	var rolesJSON []byte
	var lastUsed sql.NullTime
	var tokenCount int64
	if err := scanner.Scan(
		&account.ID,
		&account.Sub,
		&account.Email,
		&account.Provider,
		&account.Status,
		&tokenCount,
		&lastUsed,
		&rolesJSON,
	); err != nil {
		return serviceAccountSummary{}, err
	}
	account.TokenCount = int(tokenCount)
	if lastUsed.Valid {
		t := lastUsed.Time
		account.LastUsedAt = &t
	}
	if len(rolesJSON) > 0 {
		_ = json.Unmarshal(rolesJSON, &account.Roles)
	}
	return account, nil
}

type serviceAccountTokenScanner interface {
	Scan(dest ...any) error
}

func scanServiceAccountToken(scanner serviceAccountTokenScanner) (serviceAccountTokenResponse, error) {
	var token serviceAccountTokenResponse
	var expiresAt, lastUsed sql.NullTime
	if err := scanner.Scan(&token.ID, &token.Name, &token.TokenSuffix, &token.CreatedAt, &expiresAt, &lastUsed); err != nil {
		return serviceAccountTokenResponse{}, err
	}
	if expiresAt.Valid {
		expires := expiresAt.Time
		token.ExpiresAt = &expires
	}
	if lastUsed.Valid {
		lastUsedAt := lastUsed.Time
		token.LastUsedAt = &lastUsedAt
	}
	return token, nil
}

func deleteServiceAccountAccessArtifacts(ctx context.Context, runner execRunner, serviceAccountID string) error {
	serviceAccountID = strings.TrimSpace(serviceAccountID)
	if serviceAccountID == "" {
		return nil
	}
	statements := []string{
		`DELETE FROM access_grants WHERE subject_type = 'service_account' AND subject_id = $1`,
		`DELETE FROM resource_acl WHERE subject_type = 'service_account' AND subject_id = $1`,
		`DELETE FROM resource_ownership WHERE owner_subject_type = 'service_account' AND owner_subject_id = $1`,
		`DELETE FROM auth_group_members WHERE subject_type = 'service_account' AND subject_id = $1`,
		`DELETE FROM auth_role_bindings WHERE subject_type = 'service_account' AND subject_id = $1`,
	}
	for _, stmt := range statements {
		if _, err := runner.Exec(ctx, stmt, serviceAccountID); err != nil {
			return err
		}
	}
	return nil
}

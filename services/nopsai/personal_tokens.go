package nopsai

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"nopsai/pkg/httpapi"
	"nopsai/services/nopsai/pkg/auth"
)

const (
	defaultPersonalAccessTokenDays = 90
	maxPersonalAccessTokenDays     = 365
	maxPersonalAccessTokenNameLen  = 80
)

func (a *App) handleListPersonalAccessTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userRecord, ok := a.loadInteractiveAuthenticatedUser(w, r)
	if !ok {
		return
	}

	rows, err := a.db.Query(r.Context(), `
		SELECT id::text, name, token_suffix, created_at, expires_at, last_used_at
		FROM personal_access_tokens
		WHERE user_id = $1 AND revoked_at IS NULL
		ORDER BY created_at DESC
	`, userRecord.ID)
	if err != nil {
		http.Error(w, "failed to list personal tokens", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	tokens := []authPersonalTokenResponse{}
	for rows.Next() {
		var token authPersonalTokenResponse
		var expiresAt, lastUsed sql.NullTime
		if err := rows.Scan(&token.ID, &token.Name, &token.TokenSuffix, &token.CreatedAt, &expiresAt, &lastUsed); err != nil {
			http.Error(w, "failed to read personal tokens", http.StatusInternalServerError)
			return
		}
		if expiresAt.Valid {
			expires := expiresAt.Time
			token.ExpiresAt = &expires
		}
		if lastUsed.Valid {
			lastUsedAt := lastUsed.Time
			token.LastUsedAt = &lastUsedAt
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read personal tokens", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, tokens)
}

func (a *App) handleCreatePersonalAccessToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userRecord, ok := a.loadInteractiveAuthenticatedUser(w, r)
	if !ok {
		return
	}

	var req authPersonalTokenRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	name, err := validatePersonalAccessTokenName(req.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	expiresAt, err := resolvePersonalAccessTokenExpiry(req, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rawToken, err := auth.GeneratePersonalAccessToken()
	if err != nil {
		http.Error(w, "failed to generate personal token", http.StatusInternalServerError)
		return
	}
	tokenID := uuid.New()
	token := authPersonalTokenResponse{
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
	err = a.db.QueryRow(r.Context(), `
		INSERT INTO personal_access_tokens (id, user_id, name, token_hash, token_suffix, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, expires_at
	`, tokenID, userRecord.ID, name, auth.HashToken(rawToken), token.TokenSuffix, expiresArg).Scan(&token.CreatedAt, &returnedExpiresAt)
	if err != nil {
		http.Error(w, "failed to save personal token", http.StatusInternalServerError)
		return
	}
	if returnedExpiresAt.Valid {
		expires := returnedExpiresAt.Time
		token.ExpiresAt = &expires
	}
	_ = httpapi.WriteJSON(w, http.StatusCreated, token)
}

func (a *App) handleRevokePersonalAccessToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userRecord, ok := a.loadInteractiveAuthenticatedUser(w, r)
	if !ok {
		return
	}

	tokenID, err := uuid.Parse(strings.TrimSpace(r.PathValue("tokenID")))
	if err != nil {
		http.Error(w, "invalid token id", http.StatusBadRequest)
		return
	}
	tag, err := a.db.Exec(r.Context(), `
		UPDATE personal_access_tokens
		SET revoked_at = NOW()
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL
	`, tokenID, userRecord.ID)
	if err != nil {
		http.Error(w, "failed to revoke personal token", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "personal token not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) loadInteractiveAuthenticatedUser(w http.ResponseWriter, r *http.Request) (authenticatedUserRecord, bool) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || strings.TrimSpace(claims.Sub) == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return authenticatedUserRecord{}, false
	}
	if !isPersonalTokenManagementSessionProvider(claims.Provider) {
		http.Error(w, "personal token management requires an interactive session", http.StatusForbidden)
		return authenticatedUserRecord{}, false
	}
	userRecord, err := a.loadAuthenticatedUserRecord(r.Context(), claims.Sub)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "user not found", http.StatusNotFound)
			return authenticatedUserRecord{}, false
		}
		http.Error(w, "failed to load user", http.StatusInternalServerError)
		return authenticatedUserRecord{}, false
	}
	if !userRecord.IsActive() {
		http.Error(w, "account disabled", http.StatusForbidden)
		return authenticatedUserRecord{}, false
	}
	return userRecord, true
}

func isPersonalTokenManagementSessionProvider(provider string) bool {
	provider = strings.TrimSpace(provider)
	return strings.EqualFold(provider, "local") ||
		strings.HasPrefix(strings.ToLower(provider), "oidc:")
}

func validatePersonalAccessTokenName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", errors.New("name is required")
	}
	if len(name) > maxPersonalAccessTokenNameLen {
		return "", errors.New("name must be 80 characters or fewer")
	}
	if strings.ContainsAny(name, "\r\n\t") {
		return "", errors.New("name cannot contain control characters")
	}
	return name, nil
}

func resolvePersonalAccessTokenExpiry(req authPersonalTokenRequest, now time.Time) (*time.Time, error) {
	if req.NeverExpires {
		return nil, nil
	}

	if raw := strings.TrimSpace(req.ExpiresAt); raw != "" {
		expiresAt, err := parsePersonalAccessTokenExpiry(raw)
		if err != nil {
			return nil, err
		}
		if !expiresAt.After(now) {
			return nil, errors.New("expires_at must be in the future")
		}
		if expiresAt.After(now.AddDate(0, 0, maxPersonalAccessTokenDays)) {
			return nil, errors.New("expires_at must be within 365 days unless never_expires is true")
		}
		return &expiresAt, nil
	}

	expiresInDays := req.ExpiresInDays
	if expiresInDays == 0 {
		expiresInDays = defaultPersonalAccessTokenDays
	}
	if expiresInDays < 1 || expiresInDays > maxPersonalAccessTokenDays {
		return nil, errors.New("expires_in_days must be between 1 and 365")
	}
	expiresAt := now.AddDate(0, 0, expiresInDays)
	return &expiresAt, nil
}

func parsePersonalAccessTokenExpiry(raw string) (time.Time, error) {
	if expiresAt, err := time.Parse(time.RFC3339, raw); err == nil {
		return expiresAt, nil
	}
	if expiresDate, err := time.Parse("2006-01-02", raw); err == nil {
		return expiresDate.Add(24*time.Hour - time.Nanosecond), nil
	}
	return time.Time{}, errors.New("expires_at must be an RFC3339 timestamp or YYYY-MM-DD date")
}

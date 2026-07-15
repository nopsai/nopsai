package nopsai

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
	"github.com/rs/zerolog/log"

	"nopsai/pkg/httpapi"
	aaamodel "nopsai/services/aaa/pkg/model"
)

const (
	knowledgeConnectionProviderNotion     = "notion"
	knowledgeConnectionProviderConfluence = "confluence"
	knowledgeConnectionProviderWiki       = "wiki"

	knowledgeConnectionStatusConnected              = "connected"
	knowledgeConnectionStatusAuthenticationRequired = "authentication_required"
	knowledgeConnectionStatusPermissionDenied       = "permission_denied"
	knowledgeConnectionStatusProviderUnavailable    = "provider_unavailable"
	knowledgeConnectionStatusDisabled               = "disabled"

	knowledgeSyncModeBeforeRun = "before_run"
	knowledgeSyncModePeriodic  = "periodic"
	knowledgeSyncModeManual    = "manual"

	knowledgeFailureModeFail      = "fail"
	knowledgeFailureModeSkip      = "skip"
	knowledgeFailureModeUseCached = "use_cached"

	defaultKnowledgeSyncIntervalMinutes = 60
	minKnowledgeSyncIntervalMinutes     = 5
)

type knowledgeConnectionListItem struct {
	ID                    string         `json:"id"`
	UUID                  string         `json:"uuid,omitempty"`
	Team                  string         `json:"team"`
	Name                  string         `json:"name"`
	DisplayName           string         `json:"display_name"`
	Provider              string         `json:"provider"`
	Status                string         `json:"status"`
	Disabled              bool           `json:"disabled"`
	BaseURL               string         `json:"base_url,omitempty"`
	CredentialVisibility  string         `json:"credential_visibility"`
	Scopes                map[string]any `json:"scopes,omitempty"`
	Config                map[string]any `json:"config,omitempty"`
	LastCheckedAt         *time.Time     `json:"last_checked_at,omitempty"`
	LastError             string         `json:"last_error,omitempty"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DocumentCount         int            `json:"document_count"`
	ExternalDocumentCount int            `json:"external_document_count"`
	UsedBy                []string       `json:"used_by,omitempty"`
}

type knowledgeConnectionRecord struct {
	knowledgeConnectionListItem
	credentialRef string
}

type upsertKnowledgeConnectionRequest struct {
	Team          string         `json:"team"`
	Name          string         `json:"name"`
	DisplayName   string         `json:"display_name"`
	Provider      string         `json:"provider"`
	BaseURL       string         `json:"base_url"`
	CredentialRef string         `json:"credential_ref"`
	Scopes        map[string]any `json:"scopes"`
	Config        map[string]any `json:"config"`
	Disabled      *bool          `json:"disabled"`
}

type knowledgeConnectionTestResponse struct {
	ID            string     `json:"id"`
	Provider      string     `json:"provider"`
	Status        string     `json:"status"`
	OK            bool       `json:"ok"`
	Message       string     `json:"message"`
	CheckedAt     time.Time  `json:"checked_at"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
}

type knowledgeConnectionPageSearchResponse struct {
	Pages      []ExternalPageSummary `json:"pages"`
	NextCursor string                `json:"next_cursor,omitempty"`
}

type knowledgeConnectionResolvePageRequest struct {
	PageID  string `json:"page_id"`
	PageURL string `json:"page_url"`
	URL     string `json:"url"`
}

type knowledgeConnectionImpactResponse struct {
	Error                     string   `json:"error"`
	AffectedKnowledgeContexts []string `json:"affected_knowledge_contexts"`
}

func normalizeKnowledgeConnectionProvider(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", knowledgeConnectionProviderWiki, "external", "external_page":
		return knowledgeConnectionProviderWiki, nil
	case knowledgeConnectionProviderNotion:
		return knowledgeConnectionProviderNotion, nil
	case knowledgeConnectionProviderConfluence:
		return knowledgeConnectionProviderConfluence, nil
	default:
		return "", fmt.Errorf("provider must be notion, confluence, or wiki")
	}
}

func normalizeKnowledgeConnectionTeam(raw string) (string, error) {
	team, err := normalizeKnowledgeContextTeam(raw)
	if err != nil {
		return "", err
	}
	if team == "" {
		return "", fmt.Errorf("team is required")
	}
	return team, nil
}

func normalizeKnowledgeConnectionName(raw string) (string, error) {
	name := strings.Trim(strings.TrimSpace(raw), "/")
	if name == "" {
		return "", fmt.Errorf("connection name is required")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		return "", fmt.Errorf("connection name contains invalid path segments")
	}
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' {
			continue
		}
		return "", fmt.Errorf("connection name can only contain letters, numbers, dots, underscores, and hyphens")
	}
	return name, nil
}

func buildKnowledgeConnectionIdentifier(team, name string) string {
	return strings.Join([]string{strings.Trim(strings.TrimSpace(team), "/"), strings.Trim(strings.TrimSpace(name), "/")}, "/")
}

func splitKnowledgeConnectionIdentifier(raw string) (string, string, error) {
	value := strings.Trim(strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/")), "/")
	parts := strings.Split(value, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("knowledge connection id must use team/name")
	}
	name, err := normalizeKnowledgeConnectionName(parts[len(parts)-1])
	if err != nil {
		return "", "", err
	}
	team, err := normalizeKnowledgeConnectionTeam(strings.Join(parts[:len(parts)-1], "/"))
	if err != nil {
		return "", "", err
	}
	return team, name, nil
}

func slugifyKnowledgeConnectionName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, ch := range value {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9':
			builder.WriteRune(ch)
			lastDash = false
		case ch == '.' || ch == '_' || ch == '-':
			builder.WriteRune(ch)
			lastDash = ch == '-'
		default:
			if !lastDash && builder.Len() > 0 {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-._")
}

func normalizeKnowledgeSyncMode(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", knowledgeSyncModeManual, "manual_sync":
		return knowledgeSyncModeManual, nil
	case knowledgeSyncModeBeforeRun, "before_every_run":
		return knowledgeSyncModeBeforeRun, nil
	case knowledgeSyncModePeriodic:
		return knowledgeSyncModePeriodic, nil
	default:
		return "", fmt.Errorf("sync_mode must be before_run, periodic, or manual")
	}
}

func normalizeKnowledgeFailureMode(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", knowledgeFailureModeFail:
		return knowledgeFailureModeFail, nil
	case knowledgeFailureModeSkip:
		return knowledgeFailureModeSkip, nil
	case knowledgeFailureModeUseCached, "cached":
		return knowledgeFailureModeUseCached, nil
	default:
		return "", fmt.Errorf("failure_mode must be fail, skip, or use_cached")
	}
}

func normalizeKnowledgeSyncIntervalMinutes(raw int, syncMode string) (int, error) {
	if syncMode != knowledgeSyncModePeriodic {
		return 0, nil
	}
	if raw == 0 {
		return defaultKnowledgeSyncIntervalMinutes, nil
	}
	if raw < minKnowledgeSyncIntervalMinutes {
		return 0, fmt.Errorf("sync_interval_minutes must be at least %d for periodic sync", minKnowledgeSyncIntervalMinutes)
	}
	return raw, nil
}

func (a *App) handleListKnowledgeConnections(w http.ResponseWriter, r *http.Request) {
	teamFilter := strings.Trim(strings.TrimSpace(r.URL.Query().Get("team")), "/")
	rows, err := a.db.Query(r.Context(), `
		SELECT c.id::text, c.team_path, c.name, c.display_name, c.provider, c.status, c.disabled,
		       c.credential_ref, c.base_url, c.scopes, c.config, c.last_checked_at, c.last_error,
		       c.updated_at,
		       COUNT(k.id)::int AS document_count,
		       (COUNT(k.id) FILTER (WHERE COALESCE(k.external_page_id, '') <> ''))::int AS external_document_count
		FROM knowledge_context_connections c
		LEFT JOIN knowledge_contexts k ON k.connection_id = c.id
		WHERE ($1 = '' OR c.team_path = $1 OR c.team_path LIKE $1 || '/%')
		GROUP BY c.id, c.team_path, c.name, c.display_name, c.provider, c.status, c.disabled,
		         c.credential_ref, c.base_url, c.scopes, c.config, c.last_checked_at, c.last_error, c.updated_at
		ORDER BY c.team_path ASC, c.name ASC
	`, teamFilter)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query knowledge connections")
		http.Error(w, "Failed to retrieve knowledge connections", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var items []knowledgeConnectionListItem
	var resources []aaamodel.ResourceRef
	for rows.Next() {
		record, err := scanKnowledgeConnectionRecord(rows)
		if err != nil {
			log.Error().Err(err).Msg("Failed to scan knowledge connection")
			http.Error(w, "Failed to process knowledge connections", http.StatusInternalServerError)
			return
		}
		items = append(items, record.knowledgeConnectionListItem)
		resources = append(resources, aaamodel.ResourceRef{Type: grantResourceKnowledgeConnection, ID: record.ID})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Failed to process knowledge connections", http.StatusInternalServerError)
		return
	}

	allowedSet, err := a.allowedResourceSet(r, "knowledge_connection.read", resources)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	filtered := make([]knowledgeConnectionListItem, 0, len(items))
	for _, item := range items {
		if _, ok := allowedSet[resourceKey(aaamodel.ResourceRef{Type: grantResourceKnowledgeConnection, ID: item.ID})]; ok {
			filtered = append(filtered, item)
		}
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (a *App) handleCreateKnowledgeConnection(w http.ResponseWriter, r *http.Request) {
	var req upsertKnowledgeConnectionRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	record, status, err := a.saveKnowledgeConnection(r, "", req)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	a.auditKnowledgeConnectionAction(r.Context(), r, "knowledge_connection.created", record, "success", nil)
	writeJSON(w, http.StatusCreated, record.knowledgeConnectionListItem)
}

func (a *App) handleGetKnowledgeConnection(w http.ResponseWriter, r *http.Request) {
	identifier := strings.Trim(strings.TrimSpace(r.PathValue("connectionID")), "/")
	for _, suffix := range []string{"/pages/search", "/pages"} {
		if strings.HasSuffix(identifier, suffix) {
			a.handleSearchKnowledgeConnectionPagesByIdentifier(w, r, strings.Trim(strings.TrimSuffix(identifier, suffix), "/"))
			return
		}
	}
	record, err := a.loadKnowledgeConnectionByIdentifier(r.Context(), identifier)
	if err != nil {
		writeKnowledgeConnectionLoadError(w, identifier, err)
		return
	}
	if !a.requireAAADecision(w, r, "knowledge_connection.read", aaamodel.ResourceRef{Type: grantResourceKnowledgeConnection, ID: record.ID}) {
		return
	}
	writeJSON(w, http.StatusOK, record.knowledgeConnectionListItem)
}

func (a *App) handleUpdateKnowledgeConnection(w http.ResponseWriter, r *http.Request) {
	var req upsertKnowledgeConnectionRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	record, status, err := a.saveKnowledgeConnection(r, r.PathValue("connectionID"), req)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	a.auditKnowledgeConnectionAction(r.Context(), r, "knowledge_connection.updated", record, "success", nil)
	writeJSON(w, http.StatusOK, record.knowledgeConnectionListItem)
}

func (a *App) handleDeleteKnowledgeConnection(w http.ResponseWriter, r *http.Request) {
	record, err := a.loadKnowledgeConnectionByIdentifier(r.Context(), r.PathValue("connectionID"))
	if err != nil {
		writeKnowledgeConnectionLoadError(w, r.PathValue("connectionID"), err)
		return
	}
	if !a.requireAAADecision(w, r, "knowledge_connection.delete", aaamodel.ResourceRef{Type: grantResourceKnowledgeConnection, ID: record.ID}) {
		return
	}
	affected := a.loadKnowledgeConnectionDependentContextIDs(r.Context(), record.UUID)
	if record.DocumentCount > 0 {
		if !strings.EqualFold(r.URL.Query().Get("confirm"), "true") {
			writeJSON(w, http.StatusConflict, knowledgeConnectionImpactResponse{
				Error:                     "knowledge connection is still referenced by knowledge contexts",
				AffectedKnowledgeContexts: affected,
			})
			return
		}
	}
	tag, err := a.db.Exec(r.Context(), `DELETE FROM knowledge_context_connections WHERE id = $1`, record.UUID)
	if err != nil {
		log.Error().Err(err).Str("knowledge_connection", record.ID).Msg("Failed to delete knowledge connection")
		http.Error(w, "Failed to delete knowledge connection", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "knowledge connection not found", http.StatusNotFound)
		return
	}
	a.auditKnowledgeConnectionAction(r.Context(), r, "knowledge_connection.deleted", record, "success", map[string]any{
		"affected_knowledge_contexts": affected,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleKnowledgeConnectionPost(w http.ResponseWriter, r *http.Request) {
	identifier := strings.Trim(strings.TrimSpace(r.PathValue("connectionID")), "/")
	switch {
	case strings.HasSuffix(identifier, "/test"):
		a.handleTestKnowledgeConnectionByIdentifier(w, r, strings.Trim(strings.TrimSuffix(identifier, "/test"), "/"))
	case strings.HasSuffix(identifier, "/resolve-page"):
		a.handleResolveKnowledgeConnectionPageByIdentifier(w, r, strings.Trim(strings.TrimSuffix(identifier, "/resolve-page"), "/"))
	default:
		http.NotFound(w, r)
	}
}

func (a *App) handleTestKnowledgeConnection(w http.ResponseWriter, r *http.Request) {
	a.handleTestKnowledgeConnectionByIdentifier(w, r, r.PathValue("connectionID"))
}

func (a *App) handleTestKnowledgeConnectionByIdentifier(w http.ResponseWriter, r *http.Request, identifier string) {
	record, err := a.loadKnowledgeConnectionByIdentifier(r.Context(), identifier)
	if err != nil {
		writeKnowledgeConnectionLoadError(w, identifier, err)
		return
	}
	if !a.requireAAADecision(w, r, "knowledge_connection.test", aaamodel.ResourceRef{Type: grantResourceKnowledgeConnection, ID: record.ID}) {
		return
	}

	checkedAt := time.Now().UTC()
	status := knowledgeConnectionStatusConnected
	ok := true
	message := "Connection test succeeded."
	if record.Disabled {
		status = knowledgeConnectionStatusDisabled
		ok = false
		message = "Connection is disabled."
	} else if strings.TrimSpace(record.credentialRef) == "" {
		status = knowledgeConnectionStatusAuthenticationRequired
		ok = false
		message = "Authentication is not configured."
	} else {
		provider, err := a.knowledgePageProvider(record.Provider)
		if err != nil {
			status = knowledgeConnectionStatusProviderUnavailable
			ok = false
			message = err.Error()
		} else if err := provider.TestConnection(r.Context(), record); err != nil {
			status = knowledgeProviderErrorStatus(err)
			ok = false
			message = err.Error()
		}
	}

	_, err = a.db.Exec(r.Context(), `
		UPDATE knowledge_context_connections
		SET status = $1, last_checked_at = $2, last_error = $3, updated_at = NOW()
		WHERE id = $4
	`, status, checkedAt, map[bool]string{true: "", false: message}[ok], record.UUID)
	if err != nil {
		log.Error().Err(err).Str("knowledge_connection", record.ID).Msg("Failed to update knowledge connection health")
		http.Error(w, "Failed to test knowledge connection", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, knowledgeConnectionTestResponse{
		ID:            record.ID,
		Provider:      record.Provider,
		Status:        status,
		OK:            ok,
		Message:       message,
		CheckedAt:     checkedAt,
		LastCheckedAt: &checkedAt,
	})
}

func (a *App) handleSearchKnowledgeConnectionPages(w http.ResponseWriter, r *http.Request) {
	a.handleSearchKnowledgeConnectionPagesByIdentifier(w, r, r.PathValue("connectionID"))
}

func (a *App) handleSearchKnowledgeConnectionPagesByIdentifier(w http.ResponseWriter, r *http.Request, identifier string) {
	record, err := a.loadKnowledgeConnectionByIdentifier(r.Context(), identifier)
	if err != nil {
		writeKnowledgeConnectionLoadError(w, identifier, err)
		return
	}
	if !a.requireAAADecision(w, r, "knowledge_connection.use", aaamodel.ResourceRef{Type: grantResourceKnowledgeConnection, ID: record.ID}) {
		return
	}
	if record.Disabled {
		http.Error(w, "knowledge connection is disabled", http.StatusConflict)
		return
	}
	provider, err := a.knowledgePageProvider(record.Provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := provider.SearchPages(r.Context(), record, firstNonEmptyString(r.URL.Query().Get("query"), r.URL.Query().Get("q")), r.URL.Query().Get("cursor"))
	if err != nil {
		a.updateKnowledgeConnectionHealth(r.Context(), record, knowledgeProviderErrorStatus(err), err.Error(), nil)
		http.Error(w, err.Error(), providerHTTPStatus(err))
		return
	}
	a.updateKnowledgeConnectionHealth(r.Context(), record, knowledgeConnectionStatusConnected, "", nil)
	writeJSON(w, http.StatusOK, knowledgeConnectionPageSearchResponse{Pages: result.Pages, NextCursor: result.NextCursor})
}

func (a *App) handleResolveKnowledgeConnectionPage(w http.ResponseWriter, r *http.Request) {
	a.handleResolveKnowledgeConnectionPageByIdentifier(w, r, r.PathValue("connectionID"))
}

func (a *App) handleResolveKnowledgeConnectionPageByIdentifier(w http.ResponseWriter, r *http.Request, identifier string) {
	record, err := a.loadKnowledgeConnectionByIdentifier(r.Context(), identifier)
	if err != nil {
		writeKnowledgeConnectionLoadError(w, identifier, err)
		return
	}
	if !a.requireAAADecision(w, r, "knowledge_connection.use", aaamodel.ResourceRef{Type: grantResourceKnowledgeConnection, ID: record.ID}) {
		return
	}
	if record.Disabled {
		http.Error(w, "knowledge connection is disabled", http.StatusConflict)
		return
	}
	var req knowledgeConnectionResolvePageRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	provider, err := a.knowledgePageProvider(record.Provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var page ExternalPage
	pageID := strings.TrimSpace(req.PageID)
	pageURL := strings.TrimSpace(firstNonEmptyString(req.PageURL, req.URL))
	if pageID != "" {
		page, err = provider.GetPage(r.Context(), record, pageID)
	} else if pageURL != "" {
		page, err = provider.ResolvePage(r.Context(), record, pageURL)
	} else {
		http.Error(w, "page_id or page_url is required", http.StatusBadRequest)
		return
	}
	if err != nil {
		a.updateKnowledgeConnectionHealth(r.Context(), record, knowledgeProviderErrorStatus(err), err.Error(), nil)
		http.Error(w, err.Error(), providerHTTPStatus(err))
		return
	}
	a.updateKnowledgeConnectionHealth(r.Context(), record, knowledgeConnectionStatusConnected, "", nil)
	writeJSON(w, http.StatusOK, page)
}

func (a *App) saveKnowledgeConnection(r *http.Request, pathID string, req upsertKnowledgeConnectionRequest) (knowledgeConnectionRecord, int, error) {
	var existing *knowledgeConnectionRecord
	if pathID != "" {
		loaded, err := a.loadKnowledgeConnectionByIdentifier(r.Context(), pathID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
				return knowledgeConnectionRecord{}, http.StatusNotFound, fmt.Errorf("knowledge connection not found")
			}
			return knowledgeConnectionRecord{}, http.StatusInternalServerError, fmt.Errorf("failed to load knowledge connection")
		}
		existing = &loaded
		if strings.TrimSpace(req.Team) == "" {
			req.Team = loaded.Team
		}
		if strings.TrimSpace(req.Name) == "" {
			req.Name = loaded.Name
		}
		if strings.TrimSpace(req.DisplayName) == "" {
			req.DisplayName = loaded.DisplayName
		}
		if strings.TrimSpace(req.Provider) == "" {
			req.Provider = loaded.Provider
		}
		if strings.TrimSpace(req.BaseURL) == "" {
			req.BaseURL = loaded.BaseURL
		}
		if strings.TrimSpace(req.CredentialRef) == "" {
			req.CredentialRef = loaded.credentialRef
		}
		if req.Scopes == nil {
			req.Scopes = loaded.Scopes
		}
		if req.Config == nil {
			req.Config = loaded.Config
		}
	}
	team, err := normalizeKnowledgeConnectionTeam(req.Team)
	if err != nil {
		return knowledgeConnectionRecord{}, http.StatusBadRequest, err
	}
	provider, err := normalizeKnowledgeConnectionProvider(req.Provider)
	if err != nil {
		return knowledgeConnectionRecord{}, http.StatusBadRequest, err
	}
	nameInput := strings.TrimSpace(req.Name)
	if nameInput == "" {
		nameInput = slugifyKnowledgeConnectionName(firstNonEmptyString(req.DisplayName, provider))
	}
	name, err := normalizeKnowledgeConnectionName(nameInput)
	if err != nil {
		return knowledgeConnectionRecord{}, http.StatusBadRequest, err
	}
	if pathID != "" && existing != nil {
		if existing.Team != team || existing.Name != name {
			return knowledgeConnectionRecord{}, http.StatusBadRequest, fmt.Errorf("request body team and name must match the URL")
		}
	}
	resourceID := buildKnowledgeConnectionIdentifier(team, name)
	action := "knowledge_connection.create"
	var existingID string
	lookupErr := a.db.QueryRow(r.Context(), `
		SELECT id::text
		FROM knowledge_context_connections
		WHERE team_path = $1 AND name = $2
		LIMIT 1
	`, team, name).Scan(&existingID)
	if lookupErr == nil {
		if pathID == "" {
			return knowledgeConnectionRecord{}, http.StatusConflict, fmt.Errorf("knowledge connection already exists")
		}
		action = "knowledge_connection.update"
	} else if !errors.Is(lookupErr, pgx.ErrNoRows) && !errors.Is(lookupErr, sql.ErrNoRows) {
		log.Error().Err(lookupErr).Str("knowledge_connection", resourceID).Msg("Failed to inspect knowledge connection")
		return knowledgeConnectionRecord{}, http.StatusInternalServerError, fmt.Errorf("failed to save knowledge connection")
	}
	if !a.requireAAADecision(noopResponseWriter{}, r, action, aaamodel.ResourceRef{Type: grantResourceKnowledgeConnection, ID: resourceID}) {
		return knowledgeConnectionRecord{}, http.StatusForbidden, fmt.Errorf("forbidden")
	}

	disabled := false
	if existing != nil {
		disabled = existing.Disabled
	}
	if req.Disabled != nil {
		disabled = *req.Disabled
	}
	status := knowledgeConnectionStatusConnected
	if disabled {
		status = knowledgeConnectionStatusDisabled
	} else if strings.TrimSpace(req.CredentialRef) == "" {
		status = knowledgeConnectionStatusAuthenticationRequired
	}
	scopes, err := json.Marshal(mapOrEmpty(req.Scopes))
	if err != nil {
		return knowledgeConnectionRecord{}, http.StatusBadRequest, fmt.Errorf("invalid scopes")
	}
	config, err := json.Marshal(mapOrEmpty(req.Config))
	if err != nil {
		return knowledgeConnectionRecord{}, http.StatusBadRequest, fmt.Errorf("invalid config")
	}

	_, err = a.db.Exec(r.Context(), `
		INSERT INTO knowledge_context_connections (
			team_path, name, display_name, provider, status, disabled, credential_ref, credential_secret_ref, base_url, scopes, config, provider_config, disabled_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $8, $9::jsonb, $10::jsonb, $10::jsonb, CASE WHEN $6 THEN NOW() ELSE NULL END, NOW())
		ON CONFLICT (team_path, name) DO UPDATE SET
			display_name = EXCLUDED.display_name,
			provider = EXCLUDED.provider,
			status = EXCLUDED.status,
			disabled = EXCLUDED.disabled,
			credential_ref = EXCLUDED.credential_ref,
			credential_secret_ref = EXCLUDED.credential_secret_ref,
			base_url = EXCLUDED.base_url,
			scopes = EXCLUDED.scopes,
			config = EXCLUDED.config,
			provider_config = EXCLUDED.provider_config,
			disabled_at = CASE
				WHEN EXCLUDED.disabled THEN COALESCE(knowledge_context_connections.disabled_at, NOW())
				ELSE NULL
			END,
			last_error = CASE WHEN EXCLUDED.status = 'connected' THEN '' ELSE knowledge_context_connections.last_error END,
			updated_at = NOW()
	`, team, name, strings.TrimSpace(req.DisplayName), provider, status, disabled, strings.TrimSpace(req.CredentialRef), strings.TrimSpace(req.BaseURL), string(scopes), string(config))
	if err != nil {
		log.Error().Err(err).Str("knowledge_connection", resourceID).Msg("Failed to save knowledge connection")
		return knowledgeConnectionRecord{}, http.StatusInternalServerError, fmt.Errorf("failed to save knowledge connection")
	}
	record, err := a.loadKnowledgeConnectionByIdentifier(r.Context(), resourceID)
	if err != nil {
		return knowledgeConnectionRecord{}, http.StatusInternalServerError, fmt.Errorf("failed to load saved knowledge connection")
	}
	return record, http.StatusOK, nil
}

func mapOrEmpty(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func (a *App) loadKnowledgeConnectionByIdentifier(ctx context.Context, identifier string) (knowledgeConnectionRecord, error) {
	identifier = strings.Trim(strings.TrimSpace(identifier), "/")
	if identifier == "" {
		return knowledgeConnectionRecord{}, fmt.Errorf("knowledge connection id is required")
	}
	if _, err := uuid.Parse(identifier); err == nil {
		return a.loadKnowledgeConnectionByQuery(ctx, `c.id = $1`, identifier)
	}
	team, name, err := splitKnowledgeConnectionIdentifier(identifier)
	if err != nil {
		return knowledgeConnectionRecord{}, err
	}
	return a.loadKnowledgeConnectionByQuery(ctx, `c.team_path = $1 AND c.name = $2`, team, name)
}

func (a *App) loadKnowledgeConnectionByQuery(ctx context.Context, predicate string, args ...any) (knowledgeConnectionRecord, error) {
	query := `
		SELECT c.id::text, c.team_path, c.name, c.display_name, c.provider, c.status, c.disabled,
		       c.credential_ref, c.base_url, c.scopes, c.config, c.last_checked_at, c.last_error,
		       c.updated_at,
		       COUNT(k.id)::int AS document_count,
		       (COUNT(k.id) FILTER (WHERE COALESCE(k.external_page_id, '') <> ''))::int AS external_document_count
		FROM knowledge_context_connections c
		LEFT JOIN knowledge_contexts k ON k.connection_id = c.id
		WHERE ` + predicate + `
		GROUP BY c.id, c.team_path, c.name, c.display_name, c.provider, c.status, c.disabled,
		         c.credential_ref, c.base_url, c.scopes, c.config, c.last_checked_at, c.last_error, c.updated_at
		LIMIT 1
	`
	row := a.db.QueryRow(ctx, query, args...)
	return scanKnowledgeConnectionRecord(row)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanKnowledgeConnectionRecord(row rowScanner) (knowledgeConnectionRecord, error) {
	var record knowledgeConnectionRecord
	var scopesRaw, configRaw []byte
	var lastChecked sql.NullTime
	err := row.Scan(
		&record.UUID,
		&record.Team,
		&record.Name,
		&record.DisplayName,
		&record.Provider,
		&record.Status,
		&record.Disabled,
		&record.credentialRef,
		&record.BaseURL,
		&scopesRaw,
		&configRaw,
		&lastChecked,
		&record.LastError,
		&record.UpdatedAt,
		&record.DocumentCount,
		&record.ExternalDocumentCount,
	)
	if err != nil {
		return record, err
	}
	record.ID = buildKnowledgeConnectionIdentifier(record.Team, record.Name)
	record.CredentialVisibility = "not_configured"
	if strings.TrimSpace(record.credentialRef) != "" {
		record.CredentialVisibility = "configured"
	}
	if strings.TrimSpace(record.DisplayName) == "" {
		record.DisplayName = record.Name
	}
	if lastChecked.Valid {
		record.LastCheckedAt = &lastChecked.Time
	}
	record.Scopes = decodeKnowledgeConnectionJSONMap(scopesRaw)
	record.Config = decodeKnowledgeConnectionJSONMap(configRaw)
	return record, nil
}

func decodeKnowledgeConnectionJSONMap(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return map[string]any{}
	}
	return value
}

func writeKnowledgeConnectionLoadError(w http.ResponseWriter, identifier string, err error) {
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "knowledge connection not found", http.StatusNotFound)
		return
	}
	if strings.Contains(err.Error(), "must use team/name") || strings.Contains(err.Error(), "invalid path segments") || strings.Contains(err.Error(), "is required") {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Error().Err(err).Str("knowledge_connection", identifier).Msg("Failed to load knowledge connection")
	http.Error(w, "Failed to load knowledge connection", http.StatusInternalServerError)
}

func (a *App) updateKnowledgeConnectionHealth(ctx context.Context, record knowledgeConnectionRecord, status, message string, checkedAt *time.Time) {
	if a == nil || a.db == nil || strings.TrimSpace(record.UUID) == "" {
		return
	}
	if checkedAt == nil {
		now := time.Now().UTC()
		checkedAt = &now
	}
	_, _ = a.db.Exec(ctx, `
		UPDATE knowledge_context_connections
		SET status = $1, last_checked_at = $2, last_error = $3, updated_at = NOW()
		WHERE id = $4
	`, status, checkedAt, strings.TrimSpace(message), record.UUID)
}

func providerHTTPStatus(err error) int {
	var providerErr knowledgeProviderError
	if errors.As(err, &providerErr) {
		if providerErr.StatusCode >= 400 {
			return providerErr.StatusCode
		}
		switch providerErr.Kind {
		case knowledgeProviderErrorAuthentication:
			return http.StatusUnauthorized
		case knowledgeProviderErrorPermission:
			return http.StatusForbidden
		case knowledgeProviderErrorPageUnavailable:
			return http.StatusNotFound
		case knowledgeProviderErrorPageTooLarge:
			return http.StatusRequestEntityTooLarge
		case knowledgeProviderErrorInvalidRequest:
			return http.StatusBadRequest
		default:
			return http.StatusBadGateway
		}
	}
	return http.StatusBadGateway
}

func (a *App) loadKnowledgeConnectionDependentContextIDs(ctx context.Context, connectionUUID string) []string {
	if a == nil || a.db == nil || strings.TrimSpace(connectionUUID) == "" {
		return nil
	}
	rows, err := a.db.Query(ctx, `
		SELECT kind, team_path, name
		FROM knowledge_contexts
		WHERE connection_id = $1
		ORDER BY kind ASC, team_path ASC, name ASC
	`, connectionUUID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var kind, team, name string
		if err := rows.Scan(&kind, &team, &name); err == nil {
			ids = append(ids, buildKnowledgeContextIdentifier(kind, team, name))
		}
	}
	return ids
}

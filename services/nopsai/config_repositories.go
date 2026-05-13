package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/auth"
	configstore "nopsai/services/nopsai/pkg/store"

	"github.com/rs/zerolog/log"
)

type upsertConfigRepositoryRequest struct {
	RepoURL  string `json:"repo_url"`
	Branch   string `json:"branch"`
	BasePath string `json:"base_path"`
	Enabled  *bool  `json:"enabled"`
}

func (a *App) handleGetGlobalConfigRepository(w http.ResponseWriter, r *http.Request) {
	repo, err := a.store.GetConfigRepositoryByScope(r.Context(), models.ConfigRepositoryScopeSystem, models.ConfigRepositorySystemGlobalID)
	if err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to load global config repository")
		return
	}
	writeJSON(w, http.StatusOK, repo)
}

func (a *App) handleUpsertGlobalConfigRepository(w http.ResponseWriter, r *http.Request) {
	var req upsertConfigRepositoryRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	input, err := buildConfigRepositoryInput(req, models.ConfigRepositoryScopeSystem, models.ConfigRepositorySystemGlobalID, actorIDFromRequest(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	repo, err := a.store.CreateOrUpdateConfigRepository(r.Context(), input)
	if err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to save global config repository")
		return
	}
	writeJSON(w, http.StatusOK, repo)
}

func (a *App) handleDeleteGlobalConfigRepository(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteConfigRepositoryByScope(r.Context(), models.ConfigRepositoryScopeSystem, models.ConfigRepositorySystemGlobalID); err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to delete global config repository")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleGetGlobalConfigRepositorySyncStatus(w http.ResponseWriter, r *http.Request) {
	repo, err := a.store.GetConfigRepositoryByScope(r.Context(), models.ConfigRepositoryScopeSystem, models.ConfigRepositorySystemGlobalID)
	if err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to load global config repository")
		return
	}
	writeJSON(w, http.StatusOK, syncStatusFromConfigRepository(repo))
}

func (a *App) handleSyncGlobalConfigRepository(w http.ResponseWriter, r *http.Request) {
	a.handleSyncConfigRepositoryByScope(w, r, models.ConfigRepositoryScopeSystem, models.ConfigRepositorySystemGlobalID)
}

func (a *App) handleGetFolderConfigRepository(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireFolderConfigRepositoryDecision(w, r, "config_repo.read")
	if !ok {
		return
	}

	repo, err := a.store.GetConfigRepositoryByScope(r.Context(), models.ConfigRepositoryScopeFolder, resource.ID)
	if err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to load config repository")
		return
	}
	writeJSON(w, http.StatusOK, repo)
}

func (a *App) handleUpsertFolderConfigRepository(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireFolderConfigRepositoryDecision(w, r, "config_repo.manage")
	if !ok {
		return
	}

	var req upsertConfigRepositoryRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	input, err := buildConfigRepositoryInput(req, models.ConfigRepositoryScopeFolder, resource.ID, actorIDFromRequest(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	repo, err := a.store.CreateOrUpdateConfigRepository(r.Context(), input)
	if err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to save config repository")
		return
	}
	writeJSON(w, http.StatusOK, repo)
}

func (a *App) handleDeleteFolderConfigRepository(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireFolderConfigRepositoryDecision(w, r, "config_repo.manage")
	if !ok {
		return
	}

	if err := a.store.DeleteConfigRepositoryByScope(r.Context(), models.ConfigRepositoryScopeFolder, resource.ID); err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to delete config repository")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleGetFolderConfigRepositorySyncStatus(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireFolderConfigRepositoryDecision(w, r, "config_repo.read")
	if !ok {
		return
	}

	repo, err := a.store.GetConfigRepositoryByScope(r.Context(), models.ConfigRepositoryScopeFolder, resource.ID)
	if err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to load config repository")
		return
	}
	writeJSON(w, http.StatusOK, syncStatusFromConfigRepository(repo))
}

func (a *App) handleSyncFolderConfigRepository(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireFolderConfigRepositoryDecision(w, r, "config_repo.sync")
	if !ok {
		return
	}
	if !a.requireFolderConfigRepositoryOwner(w, r, resource.ID) {
		return
	}
	a.handleSyncConfigRepositoryByScope(w, r, models.ConfigRepositoryScopeFolder, resource.ID)
}

func (a *App) handleSyncConfigRepositoryByScope(w http.ResponseWriter, r *http.Request, scopeType, scopeID string) {
	repo, err := a.store.GetConfigRepositoryByScope(r.Context(), scopeType, scopeID)
	if err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to load config repository")
		return
	}
	if !repo.Enabled {
		http.Error(w, "config repository is disabled", http.StatusBadRequest)
		return
	}
	if strings.EqualFold(repo.LastSyncStatus, "running") {
		http.Error(w, "configuration sync is already running for this repository", http.StatusConflict)
		return
	}

	startedAt := time.Now()
	if err := a.store.UpdateConfigRepositorySyncStatus(r.Context(), repo.ID, "running", "Configuration synchronization started.", repo.LastSyncCommitSHA, &startedAt, nil); err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to update sync status")
		return
	}

	status := ConfigSyncStatus{
		Status:    "running",
		Message:   "Configuration synchronization started.",
		StartedAt: &startedAt,
	}
	writeJSON(w, http.StatusAccepted, status)

	repo.LastSyncStatus = "running"
	repo.LastSyncMessage = status.Message
	repo.LastSyncStartedAt = &startedAt
	repo.LastSyncCompletedAt = nil
	go a.syncConfigRepository(context.Background(), repo, startedAt)
}

func (a *App) handleListConfigRepositories(w http.ResponseWriter, r *http.Request) {
	repos, err := a.store.ListConfigRepositories(r.Context(), models.ConfigRepositoryFilter{})
	if err != nil {
		http.Error(w, "failed to list config repositories", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, repos)
}

func (a *App) handleSyncAllConfigRepositories(w http.ResponseWriter, r *http.Request) {
	a.handleConfigSync(w, r)
}

func (a *App) requireFolderConfigRepositoryDecision(w http.ResponseWriter, r *http.Request, action string) (accessGrantResource, bool) {
	resource, err := a.folderConfigRepositoryResource(r.Context(), r.PathValue("folderID"))
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return accessGrantResource{}, false
	}
	if !a.requireAAADecision(w, r, action, model.ResourceRef{Type: grantResourceFolder, ID: resource.ID}) {
		return accessGrantResource{}, false
	}
	return resource, true
}

func (a *App) folderConfigRepositoryResource(ctx context.Context, raw string) (accessGrantResource, error) {
	return resolveAccessGrantFolder(ctx, a.db, raw, true)
}

func (a *App) requireFolderConfigRepositoryOwner(w http.ResponseWriter, r *http.Request, folderID string) bool {
	allowed, err := a.isFolderConfigRepositoryOwner(r.Context(), r, folderID)
	if err != nil {
		log.Error().Err(err).Str("folder_id", folderID).Msg("Failed to check config repository ownership")
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return false
	}
	if !allowed {
		http.Error(w, "only group owners can sync this config repository", http.StatusForbidden)
		return false
	}
	return true
}

func (a *App) isFolderConfigRepositoryOwner(ctx context.Context, r *http.Request, folderID string) (bool, error) {
	if a == nil || a.db == nil {
		return false, fmt.Errorf("database unavailable")
	}
	subject, ok := a.currentAAASubject(r)
	if !ok {
		return false, fmt.Errorf("missing authorization subject")
	}
	introspection, err := a.aaaIntrospect(ctx, subject)
	if err != nil {
		return false, err
	}

	subjects := map[string]struct{}{}
	addSubject := func(subjectType, subjectID string) {
		subjectType = strings.TrimSpace(subjectType)
		subjectID = strings.TrimSpace(subjectID)
		if subjectType == "" || subjectID == "" {
			return
		}
		subjects[subjectType+"|"+subjectID] = struct{}{}
	}
	if introspection != nil {
		addSubject(model.SubjectTypeUser, introspection.ID)
		for _, group := range introspection.AuthGroups {
			addSubject(model.SubjectTypeAuthGroup, group.ID)
		}
	}
	addSubject(subject.Type, subject.ID)

	resourceIDs := folderOwnerGuardResourceIDs(folderID)
	if len(resourceIDs) == 0 || len(subjects) == 0 {
		return false, nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT owner_subject_type, owner_subject_id
		FROM resource_ownership
		WHERE resource_type = $1
		  AND resource_id = ANY($2)
	`, grantResourceFolder, resourceIDs)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var subjectType, subjectID string
		if err := rows.Scan(&subjectType, &subjectID); err != nil {
			return false, err
		}
		if _, ok := subjects[strings.TrimSpace(subjectType)+"|"+strings.TrimSpace(subjectID)]; ok {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func buildConfigRepositoryInput(req upsertConfigRepositoryRequest, scopeType, scopeID, actor string) (models.ConfigRepositoryInput, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.Trim(strings.TrimSpace(scopeID), "/")
	if scopeType != models.ConfigRepositoryScopeFolder && scopeType != models.ConfigRepositoryScopeSystem {
		return models.ConfigRepositoryInput{}, fmt.Errorf("scope_type must be folder or system")
	}
	if scopeID == "" {
		return models.ConfigRepositoryInput{}, fmt.Errorf("scope_id is required")
	}

	repoURL := strings.TrimSpace(req.RepoURL)
	if repoURL == "" {
		return models.ConfigRepositoryInput{}, fmt.Errorf("repo_url is required")
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = "main"
	}
	basePath, err := normalizeConfigRepositoryBasePathForRequest(req.BasePath)
	if err != nil {
		return models.ConfigRepositoryInput{}, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	return models.ConfigRepositoryInput{
		ScopeType: scopeType,
		ScopeID:   scopeID,
		RepoURL:   repoURL,
		Branch:    branch,
		BasePath:  basePath,
		Enabled:   enabled,
		Actor:     actor,
	}, nil
}

func normalizeConfigRepositoryBasePathForRequest(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	normalized = strings.ReplaceAll(normalized, "\\", "/")
	if filepath.IsAbs(normalized) {
		return "", fmt.Errorf("base_path must be relative")
	}
	normalized = strings.Trim(normalized, "/")
	if normalized == "." {
		return "", nil
	}
	if normalized == "" {
		return "", nil
	}
	for _, segment := range strings.Split(normalized, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("base_path contains invalid path segments")
		}
	}
	return normalized, nil
}

func actorIDFromRequest(r *http.Request) string {
	claims, _ := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		return ""
	}
	if strings.TrimSpace(claims.Sub) != "" {
		return strings.TrimSpace(claims.Sub)
	}
	return strings.TrimSpace(claims.Email)
}

func syncStatusFromConfigRepository(repo models.ConfigRepository) ConfigSyncStatus {
	status := strings.TrimSpace(repo.LastSyncStatus)
	if status == "" {
		status = "idle"
	}
	message := strings.TrimSpace(repo.LastSyncMessage)
	if message == "" {
		message = "No configuration sync has been requested yet."
	}
	return ConfigSyncStatus{
		Status:      status,
		Message:     message,
		StartedAt:   repo.LastSyncStartedAt,
		CompletedAt: repo.LastSyncCompletedAt,
	}
}

func writeConfigRepositoryStoreError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, configstore.ErrConfigRepositoryNotFound):
		http.Error(w, "config repository not found", http.StatusNotFound)
	case errors.Is(err, configstore.ErrConfigRepositoryConflict):
		http.Error(w, "config repository URL, branch, and base path are already bound to another scope", http.StatusConflict)
	default:
		http.Error(w, fallback, http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		logWarnEncode(err)
	}
}

func logWarnEncode(err error) {
	log.Warn().Err(err).Msg("Failed to encode JSON response")
}

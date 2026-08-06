package nopsai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/internal/credentials"
	"nopsai/services/nopsai/pkg/auth"
	configstore "nopsai/services/nopsai/pkg/store"

	"github.com/rs/zerolog/log"
)

type writeConfigRepositoryFilesRequest struct {
	Message string                      `json:"message"`
	Files   []writeConfigRepositoryFile `json:"files"`
}

type writeConfigRepositoryFile struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Delete  bool   `json:"delete,omitempty"`
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
	var req configsync.RepositoryInputRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	input, err := configsync.BuildRepositoryInput(req, models.ConfigRepositoryScopeSystem, models.ConfigRepositorySystemGlobalID, actorIDFromRequest(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.ensureConfigRepositoryCredentialReference(r.Context(), input); err != nil {
		writeConfigRepositoryCredentialError(w, err)
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

func (a *App) handleCancelGlobalConfigRepositorySync(w http.ResponseWriter, r *http.Request) {
	a.handleCancelConfigRepositorySyncByScope(w, r, models.ConfigRepositoryScopeSystem, models.ConfigRepositorySystemGlobalID)
}

func (a *App) handleWriteGlobalConfigRepository(w http.ResponseWriter, r *http.Request) {
	repo, err := a.store.GetConfigRepositoryByScope(r.Context(), models.ConfigRepositoryScopeSystem, models.ConfigRepositorySystemGlobalID)
	if err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to load global config repository")
		return
	}
	a.handleWriteConfigRepositoryFiles(w, r, repo)
}

func (a *App) handleGetTeamConfigRepository(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireTeamConfigRepositoryDecision(w, r, "config_repo.read")
	if !ok {
		return
	}

	repo, err := a.store.GetConfigRepositoryByScope(r.Context(), models.ConfigRepositoryScopeTeam, resource.ID)
	if err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to load config repository")
		return
	}
	writeJSON(w, http.StatusOK, repo)
}

func (a *App) handleUpsertTeamConfigRepository(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireTeamConfigRepositoryDecision(w, r, "config_repo.manage")
	if !ok {
		return
	}

	var req configsync.RepositoryInputRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	input, err := configsync.BuildRepositoryInput(req, models.ConfigRepositoryScopeTeam, resource.ID, actorIDFromRequest(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.ensureConfigRepositoryCredentialReference(r.Context(), input); err != nil {
		writeConfigRepositoryCredentialError(w, err)
		return
	}

	repo, err := a.store.CreateOrUpdateConfigRepository(r.Context(), input)
	if err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to save config repository")
		return
	}
	writeJSON(w, http.StatusOK, repo)
}

func (a *App) handleDeleteTeamConfigRepository(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireTeamConfigRepositoryDecision(w, r, "config_repo.manage")
	if !ok {
		return
	}

	if err := a.store.DeleteConfigRepositoryByScope(r.Context(), models.ConfigRepositoryScopeTeam, resource.ID); err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to delete config repository")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleGetTeamConfigRepositorySyncStatus(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireTeamConfigRepositoryDecision(w, r, "config_repo.read")
	if !ok {
		return
	}

	repo, err := a.store.GetConfigRepositoryByScope(r.Context(), models.ConfigRepositoryScopeTeam, resource.ID)
	if err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to load config repository")
		return
	}
	writeJSON(w, http.StatusOK, syncStatusFromConfigRepository(repo))
}

func (a *App) handleSyncTeamConfigRepository(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireTeamConfigRepositoryDecision(w, r, "config_repo.sync")
	if !ok {
		return
	}
	if !a.requireTeamConfigRepositoryOwner(w, r, resource.ID) {
		return
	}
	a.handleSyncConfigRepositoryByScope(w, r, models.ConfigRepositoryScopeTeam, resource.ID)
}

func (a *App) handleCancelTeamConfigRepositorySync(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireTeamConfigRepositoryDecision(w, r, "config_repo.sync")
	if !ok {
		return
	}
	if !a.requireTeamConfigRepositoryOwner(w, r, resource.ID) {
		return
	}
	a.handleCancelConfigRepositorySyncByScope(w, r, models.ConfigRepositoryScopeTeam, resource.ID)
}

func (a *App) handleWriteTeamConfigRepository(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireTeamConfigRepositoryDecision(w, r, "config_repo.manage")
	if !ok {
		return
	}

	repo, err := a.store.GetConfigRepositoryByScope(r.Context(), models.ConfigRepositoryScopeTeam, resource.ID)
	if err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to load config repository")
		return
	}
	a.handleWriteConfigRepositoryFiles(w, r, repo)
}

func (a *App) handleWriteConfigRepositoryFiles(w http.ResponseWriter, r *http.Request, repo models.ConfigRepository) {
	if !repo.WriteEnabled {
		http.Error(w, "config repository Git push is disabled", http.StatusBadRequest)
		return
	}
	writeBranch := strings.TrimSpace(repo.WriteBranch)
	if writeBranch == "" {
		http.Error(w, "config repository write_branch is required", http.StatusBadRequest)
		return
	}
	if err := configsync.ValidateBranchName(writeBranch, "write_branch"); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req writeConfigRepositoryFilesRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		message = "Update Nopsai config"
	}
	if len(req.Files) == 0 {
		http.Error(w, "files is required", http.StatusBadRequest)
		return
	}

	files := make([]GitCommitFile, 0, len(req.Files))
	for _, file := range req.Files {
		cleanPath, err := configsync.CleanRepositoryWritePath(repo.BasePath, file.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		files = append(files, GitCommitFile{
			Path:    cleanPath,
			Content: file.Content,
			Delete:  file.Delete,
		})
	}

	out, err := a.commitConfigRepositoryFiles(r.Context(), repo, message, files)
	if err != nil {
		log.Error().
			Err(err).
			Int64("config_repo_id", repo.ID).
			Str("git_provider", repo.Provider).
			Msg("Failed to push config repository changes")
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, out)
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
	syncCtx, run := a.registerConfigRepositorySync(context.Background(), repo.ID)

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
	go a.runRegisteredConfigRepositorySync(syncCtx, repo, startedAt, run)
}

func (a *App) handleCancelConfigRepositorySyncByScope(w http.ResponseWriter, r *http.Request, scopeType, scopeID string) {
	repo, err := a.store.GetConfigRepositoryByScope(r.Context(), scopeType, scopeID)
	if err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to load config repository")
		return
	}
	if !strings.EqualFold(repo.LastSyncStatus, "running") {
		writeJSON(w, http.StatusOK, syncStatusFromConfigRepository(repo))
		return
	}

	active := a.cancelActiveConfigRepositorySync(repo.ID)
	completedAt := time.Now()
	message := "Configuration synchronization canceled."
	if !active {
		message = "Configuration synchronization marked canceled; no active worker was registered."
	}
	if err := a.store.UpdateConfigRepositorySyncStatus(r.Context(), repo.ID, "canceled", message, repo.LastSyncCommitSHA, repo.LastSyncStartedAt, &completedAt); err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to cancel config repository sync")
		return
	}

	repo.LastSyncStatus = "canceled"
	repo.LastSyncMessage = message
	repo.LastSyncCompletedAt = &completedAt
	writeJSON(w, http.StatusOK, syncStatusFromConfigRepository(repo))
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

func (a *App) requireTeamConfigRepositoryDecision(w http.ResponseWriter, r *http.Request, action string) (accessGrantResource, bool) {
	resource, err := a.teamConfigRepositoryResource(r.Context(), r.PathValue("teamID"))
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return accessGrantResource{}, false
	}
	if !a.requireAAADecision(w, r, action, model.ResourceRef{Type: grantResourceTeam, ID: resource.ID}) {
		return accessGrantResource{}, false
	}
	return resource, true
}

func (a *App) teamConfigRepositoryResource(ctx context.Context, raw string) (accessGrantResource, error) {
	return resolveAccessGrantTeam(ctx, a.db, raw, true)
}

func (a *App) requireTeamConfigRepositoryOwner(w http.ResponseWriter, r *http.Request, teamID string) bool {
	allowed, err := a.isTeamConfigRepositoryOwner(r.Context(), r, teamID)
	if err != nil {
		log.Error().Err(err).Str("team_path", teamID).Msg("Failed to check config repository ownership")
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return false
	}
	if !allowed {
		http.Error(w, "only team owners can sync this config repository", http.StatusForbidden)
		return false
	}
	return true
}

func (a *App) isTeamConfigRepositoryOwner(ctx context.Context, r *http.Request, teamID string) (bool, error) {
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
		for _, team := range introspection.AuthTeams {
			addSubject(model.SubjectTypeAuthTeam, team.ID)
		}
	}
	addSubject(subject.Type, subject.ID)

	resourceIDs := teamOwnerGuardResourceIDs(teamID)
	if len(resourceIDs) == 0 || len(subjects) == 0 {
		return false, nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT owner_subject_type, owner_subject_id
		FROM resource_ownership
		WHERE resource_type = $1
		  AND resource_id = ANY($2)
	`, grantResourceTeam, resourceIDs)
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

func (a *App) ensureConfigRepositoryCredentialReference(ctx context.Context, input models.ConfigRepositoryInput) error {
	if strings.TrimSpace(input.CredentialRef) == "" {
		return nil
	}
	scopeID := strings.Trim(strings.TrimSpace(input.ScopeID), "/")
	description := fmt.Sprintf("Git provider token for %s config repository %s", input.ScopeType, scopeID)
	return a.ensureCredentialReference(ctx, input.CredentialRef, "bearer_token", description, input.Actor)
}

func writeConfigRepositoryCredentialError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, credentials.ErrUnavailable):
		http.Error(w, "credential service is unavailable", http.StatusServiceUnavailable)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
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

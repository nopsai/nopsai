package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nopsai/pkg/models"

	"github.com/rs/zerolog/log"
)

func (a *App) handleConfigSync(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	status, started := a.startConfigSync(startedAt)
	if !started {
		http.Error(w, "A configuration sync is already in progress", http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Warn().Err(err).Msg("Failed to encode config sync response")
	}

	go func(started time.Time) {
		log.Info().Msg("Starting synchronization for all config repositories")
		a.setConfigSyncStatus(a.syncAllConfigRepositories(context.Background(), started))
	}(startedAt)
}

func (a *App) syncAllConfigRepositories(ctx context.Context, started time.Time) ConfigSyncStatus {
	details := map[string]int{
		"repositories_synced": 0,
		"repositories_failed": 0,
	}
	var messages []string

	enabled := true
	syncedRepoIDs := map[int64]struct{}{}
	syncStore := a.configSyncStore()
	syncRepos := func(scopeType string) {
		repos, err := syncStore.ListConfigRepositories(ctx, models.ConfigRepositoryFilter{ScopeType: scopeType, Enabled: &enabled})
		if err != nil {
			details["repositories_failed"]++
			messages = append(messages, fmt.Sprintf("%s:*: %v", scopeType, err))
			return
		}
		for _, repo := range repos {
			if _, alreadySynced := syncedRepoIDs[repo.ID]; alreadySynced {
				continue
			}
			syncedRepoIDs[repo.ID] = struct{}{}
			repoStartedAt := time.Now()
			if err := syncStore.UpdateConfigRepositorySyncStatus(ctx, repo.ID, "running", "Configuration synchronization started.", repo.LastSyncCommitSHA, &repoStartedAt, nil); err != nil {
				details["repositories_failed"]++
				messages = append(messages, fmt.Sprintf("%s:%s: %v", repo.ScopeType, repo.ScopeID, err))
				continue
			}
			status := a.syncConfigRepository(ctx, repo, repoStartedAt)
			if strings.EqualFold(status.Status, "success") {
				details["repositories_synced"]++
				for key, value := range status.Details {
					details[key] += value
				}
				continue
			}
			details["repositories_failed"]++
			messages = append(messages, fmt.Sprintf("%s:%s: %s", repo.ScopeType, repo.ScopeID, status.Message))
		}
	}

	syncRepos(models.ConfigRepositoryScopeSystem)
	for {
		before := len(syncedRepoIDs)
		syncRepos(models.ConfigRepositoryScopeFolder)
		if len(syncedRepoIDs) == before {
			break
		}
	}

	completedAt := time.Now()
	if details["repositories_failed"] > 0 {
		return ConfigSyncStatus{
			Status:      "error",
			Message:     "One or more config repositories failed to synchronize: " + strings.Join(messages, "; "),
			Details:     details,
			StartedAt:   &started,
			CompletedAt: &completedAt,
		}
	}

	return ConfigSyncStatus{
		Status:      "success",
		Message:     "Configuration synchronization completed successfully.",
		Details:     details,
		StartedAt:   &started,
		CompletedAt: &completedAt,
	}
}

func (a *App) syncConfigRepository(ctx context.Context, repo models.ConfigRepository, started time.Time) ConfigSyncStatus {
	log.Info().
		Int64("config_repo_id", repo.ID).
		Str("scope_type", repo.ScopeType).
		Str("scope_id", repo.ScopeID).
		Msg("Starting configuration synchronization from Git")

	details, commitSHA, syncErr := a.syncConfigurationFromGit(ctx, repo)
	completedAt := time.Now()
	syncStore := a.configSyncStore()
	if syncErr != nil {
		message := fmt.Sprintf("Configuration synchronization failed: %v", syncErr)
		log.Error().Err(syncErr).Int64("config_repo_id", repo.ID).Msg("Configuration synchronization failed")
		if err := syncStore.UpdateConfigRepositorySyncStatus(ctx, repo.ID, "error", message, commitSHA, &started, &completedAt); err != nil {
			log.Warn().Err(err).Int64("config_repo_id", repo.ID).Msg("Failed to update config repository sync status")
		}
		return ConfigSyncStatus{
			Status:      "error",
			Message:     message,
			StartedAt:   &started,
			CompletedAt: &completedAt,
		}
	}

	message := "Configuration synchronization completed successfully."
	if err := syncStore.UpdateConfigRepositorySyncStatus(ctx, repo.ID, "success", message, commitSHA, &started, &completedAt); err != nil {
		log.Warn().Err(err).Int64("config_repo_id", repo.ID).Msg("Failed to update config repository sync status")
	}
	log.Info().Interface("details", details).Int64("config_repo_id", repo.ID).Msg("Configuration synchronization succeeded")
	return ConfigSyncStatus{
		Status:      "success",
		Message:     message,
		Details:     details,
		StartedAt:   &started,
		CompletedAt: &completedAt,
	}
}

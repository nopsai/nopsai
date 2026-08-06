package nopsai

import (
	"context"
	"encoding/json"
	"errors"
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

	syncCtx, cancel := context.WithCancel(context.Background())
	run := a.setConfigSyncCancel(cancel)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Warn().Err(err).Msg("Failed to encode config sync response")
	}

	go func(started time.Time) {
		defer a.clearConfigSyncCancel(run)
		defer cancel()
		log.Info().Msg("Starting synchronization for all config repositories")
		a.setConfigSyncStatus(a.syncAllConfigRepositories(syncCtx, started))
	}(startedAt)
}

func (a *App) handleCancelConfigSync(w http.ResponseWriter, r *http.Request) {
	completedAt := time.Now()
	active := a.cancelActiveConfigSync()
	status := ConfigSyncStatus{
		Status:      "canceled",
		Message:     "Configuration synchronization canceled.",
		CompletedAt: &completedAt,
	}
	current := a.getConfigSyncStatus()
	if current.StartedAt != nil {
		status.StartedAt = current.StartedAt
	}
	if !active && !strings.EqualFold(current.Status, "running") {
		status = current
	}
	if active || strings.EqualFold(current.Status, "running") {
		a.setConfigSyncStatus(status)
	}
	writeJSON(w, http.StatusOK, status)
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
	syncCanceled := false
	syncRepos := func(scopeType string) {
		if ctx.Err() != nil {
			return
		}
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
			if ctx.Err() != nil {
				return
			}
			syncedRepoIDs[repo.ID] = struct{}{}
			repoStartedAt := time.Now()
			if err := syncStore.UpdateConfigRepositorySyncStatus(ctx, repo.ID, "running", "Configuration synchronization started.", repo.LastSyncCommitSHA, &repoStartedAt, nil); err != nil {
				details["repositories_failed"]++
				messages = append(messages, fmt.Sprintf("%s:%s: %v", repo.ScopeType, repo.ScopeID, err))
				continue
			}
			syncCtx, run := a.registerConfigRepositorySync(ctx, repo.ID)
			status := a.runRegisteredConfigRepositorySync(syncCtx, repo, repoStartedAt, run)
			if strings.EqualFold(status.Status, "canceled") || errors.Is(ctx.Err(), context.Canceled) {
				syncCanceled = true
				return
			}
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
	if syncCanceled || errors.Is(ctx.Err(), context.Canceled) {
		completedAt := time.Now()
		return ConfigSyncStatus{
			Status:      "canceled",
			Message:     "Configuration synchronization canceled.",
			Details:     details,
			StartedAt:   &started,
			CompletedAt: &completedAt,
		}
	}
	for {
		before := len(syncedRepoIDs)
		syncRepos(models.ConfigRepositoryScopeTeam)
		if syncCanceled || errors.Is(ctx.Err(), context.Canceled) {
			completedAt := time.Now()
			return ConfigSyncStatus{
				Status:      "canceled",
				Message:     "Configuration synchronization canceled.",
				Details:     details,
				StartedAt:   &started,
				CompletedAt: &completedAt,
			}
		}
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

	commitSHA := ""
	details, syncErr := a.syncConfigurationFromGit(ctx, repo)
	completedAt := time.Now()
	syncStore := a.configSyncStore()
	if syncErr != nil {
		status := "error"
		message := fmt.Sprintf("Configuration synchronization failed: %v", syncErr)
		if errors.Is(syncErr, context.Canceled) {
			status = "canceled"
			message = "Configuration synchronization canceled."
			log.Info().Int64("config_repo_id", repo.ID).Msg("Configuration synchronization canceled")
		} else {
			log.Error().Err(syncErr).Int64("config_repo_id", repo.ID).Msg("Configuration synchronization failed")
		}
		updateCtx := ctx
		if errors.Is(ctx.Err(), context.Canceled) {
			updateCtx = context.Background()
		}
		if err := syncStore.UpdateConfigRepositorySyncStatus(updateCtx, repo.ID, status, message, commitSHA, &started, &completedAt); err != nil {
			log.Warn().Err(err).Int64("config_repo_id", repo.ID).Msg("Failed to update config repository sync status")
		}
		return ConfigSyncStatus{
			Status:      status,
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

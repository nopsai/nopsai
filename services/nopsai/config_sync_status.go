package nopsai

import (
	"context"
	"strings"
	"time"

	"nopsai/pkg/models"
)

type ConfigSyncStatus struct {
	Status      string         `json:"status"`
	Message     string         `json:"message,omitempty"`
	Details     map[string]int `json:"details,omitempty"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
}

func (a *App) setConfigSyncStatus(status ConfigSyncStatus) {
	a.configSyncMu.Lock()
	defer a.configSyncMu.Unlock()
	a.configSyncStatus = cloneConfigSyncStatus(status)
}

func (a *App) getConfigSyncStatus() ConfigSyncStatus {
	a.configSyncMu.Lock()
	defer a.configSyncMu.Unlock()
	return cloneConfigSyncStatus(a.configSyncStatus)
}

type configSyncRun struct {
	cancel context.CancelFunc
}

func (a *App) setConfigSyncCancel(cancel context.CancelFunc) *configSyncRun {
	a.configSyncMu.Lock()
	defer a.configSyncMu.Unlock()
	run := &configSyncRun{cancel: cancel}
	a.configSyncRun = run
	return run
}

func (a *App) clearConfigSyncCancel(run *configSyncRun) {
	if run == nil {
		return
	}
	a.configSyncMu.Lock()
	defer a.configSyncMu.Unlock()
	if a.configSyncRun == run {
		a.configSyncRun = nil
	}
}

func (a *App) cancelActiveConfigSync() bool {
	a.configSyncMu.Lock()
	run := a.configSyncRun
	a.configSyncMu.Unlock()
	if run == nil || run.cancel == nil {
		return false
	}
	run.cancel()
	return true
}

func cloneConfigSyncStatus(status ConfigSyncStatus) ConfigSyncStatus {
	statusCopy := status
	if status.Details != nil {
		detailsCopy := make(map[string]int, len(status.Details))
		for k, v := range status.Details {
			detailsCopy[k] = v
		}
		statusCopy.Details = detailsCopy
	}
	if status.StartedAt != nil {
		startedAt := *status.StartedAt
		statusCopy.StartedAt = &startedAt
	}
	if status.CompletedAt != nil {
		completedAt := *status.CompletedAt
		statusCopy.CompletedAt = &completedAt
	}
	return statusCopy
}

func (a *App) startConfigSync(startedAt time.Time) (ConfigSyncStatus, bool) {
	a.configSyncMu.Lock()
	defer a.configSyncMu.Unlock()

	if strings.EqualFold(a.configSyncStatus.Status, "running") {
		return cloneConfigSyncStatus(a.configSyncStatus), false
	}

	status := ConfigSyncStatus{
		Status:    "running",
		Message:   "Configuration synchronization started.",
		StartedAt: &startedAt,
	}
	a.configSyncStatus = cloneConfigSyncStatus(status)
	return cloneConfigSyncStatus(a.configSyncStatus), true
}

type configRepositorySyncRun struct {
	cancel context.CancelFunc
}

func (a *App) registerConfigRepositorySync(ctx context.Context, repoID int64) (context.Context, *configRepositorySyncRun) {
	if ctx == nil {
		ctx = context.Background()
	}
	syncCtx, cancel := context.WithCancel(ctx)
	run := &configRepositorySyncRun{cancel: cancel}
	if a != nil && repoID > 0 {
		a.configRepoSyncs.Store(repoID, run)
	}
	return syncCtx, run
}

func (a *App) unregisterConfigRepositorySync(repoID int64, run *configRepositorySyncRun) {
	if a == nil || repoID <= 0 || run == nil {
		return
	}
	current, ok := a.configRepoSyncs.Load(repoID)
	if ok && current == run {
		a.configRepoSyncs.Delete(repoID)
	}
}

func (a *App) cancelActiveConfigRepositorySync(repoID int64) bool {
	if a == nil || repoID <= 0 {
		return false
	}
	value, ok := a.configRepoSyncs.Load(repoID)
	if !ok {
		return false
	}
	run, ok := value.(*configRepositorySyncRun)
	if !ok || run.cancel == nil {
		return false
	}
	run.cancel()
	return true
}

func (a *App) runConfigRepositorySync(ctx context.Context, repo models.ConfigRepository, started time.Time) ConfigSyncStatus {
	syncCtx, run := a.registerConfigRepositorySync(ctx, repo.ID)
	return a.runRegisteredConfigRepositorySync(syncCtx, repo, started, run)
}

func (a *App) runRegisteredConfigRepositorySync(ctx context.Context, repo models.ConfigRepository, started time.Time, run *configRepositorySyncRun) ConfigSyncStatus {
	defer a.unregisterConfigRepositorySync(repo.ID, run)
	defer run.cancel()
	return a.syncConfigRepository(ctx, repo, started)
}

// This new helper function fetches and builds a RunListItem for a given run ID.

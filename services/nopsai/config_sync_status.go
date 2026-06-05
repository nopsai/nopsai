package main

import (
	"strings"
	"time"
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

// This new helper function fetches and builds a RunListItem for a given run ID.

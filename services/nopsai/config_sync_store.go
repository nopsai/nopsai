package nopsai

import (
	"context"
	"time"

	"nopsai/pkg/models"
)

type ConfigSyncStore interface {
	ListConfigRepositories(ctx context.Context, filter models.ConfigRepositoryFilter) ([]models.ConfigRepository, error)
	UpdateConfigRepositorySyncStatus(ctx context.Context, id int64, status, message, commitSHA string, startedAt, completedAt *time.Time) error
	ApplyConfigSyncPlan(ctx context.Context, binding models.ConfigRepository, plan configSyncPlan, details map[string]int, commitSHA string) error
}

type appConfigSyncStore struct {
	app *App
}

func (s appConfigSyncStore) ListConfigRepositories(ctx context.Context, filter models.ConfigRepositoryFilter) ([]models.ConfigRepository, error) {
	if s.app == nil || s.app.store == nil {
		return nil, nil
	}
	return s.app.store.ListConfigRepositories(ctx, filter)
}

func (s appConfigSyncStore) UpdateConfigRepositorySyncStatus(ctx context.Context, id int64, status, message, commitSHA string, startedAt, completedAt *time.Time) error {
	if s.app == nil || s.app.store == nil {
		return nil
	}
	return s.app.store.UpdateConfigRepositorySyncStatus(ctx, id, status, message, commitSHA, startedAt, completedAt)
}

func (s appConfigSyncStore) ApplyConfigSyncPlan(ctx context.Context, binding models.ConfigRepository, plan configSyncPlan, details map[string]int, commitSHA string) error {
	if s.app == nil {
		return nil
	}
	return s.app.applyConfigSyncPlan(ctx, binding, plan, details, commitSHA)
}

func (a *App) configSyncStore() ConfigSyncStore {
	if a == nil || a.configSync == nil {
		return appConfigSyncStore{app: a}
	}
	return a.configSync
}

package store

import (
	"context"
	"time"

	"nopsai/pkg/models"
)

type Store interface {
	GetRunListItem(ctx context.Context, runID string) (*models.RunListItem, error)
	CreateOrUpdateConfigRepository(ctx context.Context, input models.ConfigRepositoryInput) (models.ConfigRepository, error)
	GetConfigRepositoryByScope(ctx context.Context, scopeType, scopeID string) (models.ConfigRepository, error)
	DeleteConfigRepositoryByScope(ctx context.Context, scopeType, scopeID string) error
	ListConfigRepositories(ctx context.Context, filter models.ConfigRepositoryFilter) ([]models.ConfigRepository, error)
	UpdateConfigRepositorySyncStatus(ctx context.Context, id int64, status, message, commitSHA string, startedAt, completedAt *time.Time) error
	// Add other methods as they are migrated
}

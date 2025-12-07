package store

import (
	"context"
	"nopsai/pkg/models"
)

type Store interface {
	GetRunListItem(ctx context.Context, runID string) (*models.RunListItem, error)
	// Add other methods as they are migrated
}

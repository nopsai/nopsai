package nopsai

import (
	"context"
	"testing"
	"time"

	"nopsai/pkg/models"
)

func TestSyncAllConfigRepositoriesUsesInjectedStore(t *testing.T) {
	store := &recordingConfigSyncStore{}
	app := &App{configSync: store}

	status := app.syncAllConfigRepositories(context.Background(), time.Unix(1_700_000_000, 0))

	if status.Status != "success" {
		t.Fatalf("sync status = %q, want success", status.Status)
	}
	if len(store.listCalls) != 2 {
		t.Fatalf("list calls = %#v, want system and team scopes", store.listCalls)
	}
	if store.listCalls[0].ScopeType != models.ConfigRepositoryScopeSystem {
		t.Fatalf("first scope = %q, want system", store.listCalls[0].ScopeType)
	}
	if store.listCalls[1].ScopeType != models.ConfigRepositoryScopeTeam {
		t.Fatalf("second scope = %q, want team", store.listCalls[1].ScopeType)
	}
	if len(store.statusUpdates) != 0 {
		t.Fatalf("status updates = %#v, want none for empty repository list", store.statusUpdates)
	}
}

type recordingConfigSyncStore struct {
	listCalls     []models.ConfigRepositoryFilter
	statusUpdates []configSyncStatusUpdate
}

type configSyncStatusUpdate struct {
	id     int64
	status string
}

func (s *recordingConfigSyncStore) ListConfigRepositories(_ context.Context, filter models.ConfigRepositoryFilter) ([]models.ConfigRepository, error) {
	s.listCalls = append(s.listCalls, filter)
	return nil, nil
}

func (s *recordingConfigSyncStore) UpdateConfigRepositorySyncStatus(_ context.Context, id int64, status, message, commitSHA string, startedAt, completedAt *time.Time) error {
	_, _, _, _ = message, commitSHA, startedAt, completedAt
	s.statusUpdates = append(s.statusUpdates, configSyncStatusUpdate{id: id, status: status})
	return nil
}

func (s *recordingConfigSyncStore) ApplyConfigSyncPlan(context.Context, models.ConfigRepository, configSyncPlan, map[string]int, string) error {
	return nil
}

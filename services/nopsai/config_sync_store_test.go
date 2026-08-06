package nopsai

import (
	"context"
	"sync"
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

func TestSyncAllConfigRepositoriesStopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &recordingConfigSyncStore{}
	app := &App{configSync: store}
	started := time.Unix(1_700_000_000, 0)

	status := app.syncAllConfigRepositories(ctx, started)

	if status.Status != "canceled" {
		t.Fatalf("sync status = %q, want canceled", status.Status)
	}
	if len(store.listCalls) != 0 {
		t.Fatalf("list calls = %#v, want none after cancellation", store.listCalls)
	}
}

func TestRunConfigRepositorySyncCancelsActiveWorker(t *testing.T) {
	store := &blockingConfigSyncStore{applyStarted: make(chan struct{})}
	app := &App{
		configSync:  store,
		gitProvider: &fakeGitProvider{},
	}
	repo := models.ConfigRepository{
		ID:        42,
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
		Provider:  models.ConfigRepositoryProviderGitHub,
		RepoURL:   "https://github.com/acme/nopsai-config",
		Branch:    "main",
		Enabled:   true,
	}
	started := time.Unix(1_700_000_000, 0)
	done := make(chan ConfigSyncStatus, 1)

	go func() {
		done <- app.runConfigRepositorySync(context.Background(), repo, started)
	}()

	select {
	case <-store.applyStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("sync did not reach apply phase")
	}

	if !app.cancelActiveConfigRepositorySync(repo.ID) {
		t.Fatal("cancelActiveConfigRepositorySync() = false, want true")
	}

	var status ConfigSyncStatus
	select {
	case status = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sync did not finish after cancellation")
	}
	if status.Status != "canceled" {
		t.Fatalf("sync status = %q, want canceled", status.Status)
	}
	updates := store.updates()
	if len(updates) != 1 {
		t.Fatalf("status updates = %#v, want one", updates)
	}
	if updates[0].status != "canceled" {
		t.Fatalf("status update = %q, want canceled", updates[0].status)
	}
	if app.cancelActiveConfigRepositorySync(repo.ID) {
		t.Fatal("cancelActiveConfigRepositorySync() = true after run completed, want false")
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

type blockingConfigSyncStore struct {
	mu           sync.Mutex
	once         sync.Once
	applyStarted chan struct{}
	statuses     []configSyncStatusUpdate
}

func (s *blockingConfigSyncStore) ListConfigRepositories(context.Context, models.ConfigRepositoryFilter) ([]models.ConfigRepository, error) {
	return nil, nil
}

func (s *blockingConfigSyncStore) UpdateConfigRepositorySyncStatus(_ context.Context, id int64, status, message, commitSHA string, startedAt, completedAt *time.Time) error {
	_, _, _, _ = message, commitSHA, startedAt, completedAt
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statuses = append(s.statuses, configSyncStatusUpdate{id: id, status: status})
	return nil
}

func (s *blockingConfigSyncStore) ApplyConfigSyncPlan(ctx context.Context, _ models.ConfigRepository, _ configSyncPlan, _ map[string]int, _ string) error {
	s.once.Do(func() {
		close(s.applyStarted)
	})
	<-ctx.Done()
	return ctx.Err()
}

func (s *blockingConfigSyncStore) updates() []configSyncStatusUpdate {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]configSyncStatusUpdate, len(s.statuses))
	copy(out, s.statuses)
	return out
}

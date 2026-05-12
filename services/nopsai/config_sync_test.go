package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nopsai/config"
	"nopsai/pkg/models"
)

func TestStartConfigSyncAllowsOnlyOneConcurrentStart(t *testing.T) {
	var (
		app         App
		successes   atomic.Int32
		wg          sync.WaitGroup
		concurrency = 32
	)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			startedAt := time.Unix(int64(offset+1), 0)
			if _, ok := app.startConfigSync(startedAt); ok {
				successes.Add(1)
			}
		}(i)
	}

	wg.Wait()

	if got := successes.Load(); got != 1 {
		t.Fatalf("expected exactly one config sync start to succeed, got %d", got)
	}

	status := app.getConfigSyncStatus()
	if status.Status != "running" {
		t.Fatalf("expected config sync status to be running, got %q", status.Status)
	}
	if status.StartedAt == nil {
		t.Fatal("expected config sync start time to be recorded")
	}
}

func TestNormalizeConfigPathForFolder(t *testing.T) {
	tests := []struct {
		name        string
		boundFolder string
		relPath     string
		want        string
		wantErr     bool
	}{
		{
			name:        "pipeline path joins folder",
			boundFolder: "team-1",
			relPath:     "pipelines/build.yaml",
			want:        "team-1/build",
		},
		{
			name:        "duplicated bound folder is stripped",
			boundFolder: "team-1",
			relPath:     "pipelines/team-1/build.yaml",
			want:        "team-1/build",
		},
		{
			name:        "nested bound folder is stripped",
			boundFolder: "team-1/platform",
			relPath:     "steps/team-1/platform/deploy.yml",
			want:        "team-1/platform/deploy",
		},
		{
			name:        "dot dot path is rejected",
			boundFolder: "team-1",
			relPath:     "pipelines/../build.yaml",
			wantErr:     true,
		},
		{
			name:        "escaping path is rejected",
			boundFolder: "team-1",
			relPath:     "../build.yaml",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeConfigPathForFolder(tt.boundFolder, tt.relPath)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeConfigPathForFolder() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeConfigPathForFolder() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeConfigPathForFolder() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildSystemConfigResponseDoesNotExposeConfigRepoURL(t *testing.T) {
	app := App{}
	response := app.buildSystemConfigResponse(config.Config{})

	for _, key := range []string{"config_repo_url", "config_repo_configured", "config_sync_status"} {
		if _, ok := response[key]; ok {
			t.Fatalf("buildSystemConfigResponse() exposed removed key %q", key)
		}
	}
}

func TestBuildConfigRepositoryInputDefaultsAndValidation(t *testing.T) {
	input, err := buildConfigRepositoryInput(upsertConfigRepositoryRequest{
		RepoURL: " https://github.com/acme/configs.git ",
	}, models.ConfigRepositoryScopeFolder, " team-1/ ", "user-1")
	if err != nil {
		t.Fatalf("buildConfigRepositoryInput() error = %v", err)
	}
	if input.RepoURL != "https://github.com/acme/configs.git" {
		t.Fatalf("RepoURL = %q", input.RepoURL)
	}
	if input.Branch != "main" {
		t.Fatalf("Branch = %q, want main", input.Branch)
	}
	if input.ScopeID != "team-1" {
		t.Fatalf("ScopeID = %q, want team-1", input.ScopeID)
	}
	if !input.Enabled {
		t.Fatal("Enabled = false, want true")
	}

	if _, err := buildConfigRepositoryInput(upsertConfigRepositoryRequest{
		RepoURL:  "https://github.com/acme/configs.git",
		BasePath: "../outside",
	}, models.ConfigRepositoryScopeFolder, "team-1", "user-1"); err == nil {
		t.Fatal("buildConfigRepositoryInput() accepted escaping base_path")
	}
}

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

func TestParseConfigRepositoryBindingPath(t *testing.T) {
	tests := []struct {
		name          string
		relPath       string
		wantScopeType string
		wantScopeID   string
		wantErr       bool
	}{
		{
			name:          "top level group",
			relPath:       "groups/team-1.yaml",
			wantScopeType: models.ConfigRepositoryScopeFolder,
			wantScopeID:   "team-1",
		},
		{
			name:          "nested group",
			relPath:       "groups/team-1/platform.yaml",
			wantScopeType: models.ConfigRepositoryScopeFolder,
			wantScopeID:   "team-1/platform",
		},
		{
			name:    "unsupported scope",
			relPath: "systems/global.yaml",
			wantErr: true,
		},
		{
			name:    "escaping path",
			relPath: "groups/team-1/../prod.yaml",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScopeType, gotScopeID, err := parseConfigRepositoryBindingPath(tt.relPath)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseConfigRepositoryBindingPath() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseConfigRepositoryBindingPath() error = %v", err)
			}
			if gotScopeType != tt.wantScopeType || gotScopeID != tt.wantScopeID {
				t.Fatalf("parseConfigRepositoryBindingPath() = (%q, %q), want (%q, %q)", gotScopeType, gotScopeID, tt.wantScopeType, tt.wantScopeID)
			}
		})
	}
}

func TestNormalizePipelineRunStructureForFolder(t *testing.T) {
	structure := map[string]*pipelineRunStructureNode{
		"dev": {
			Description: "Development",
			Repos:       []string{"acme/app"},
			Children: map[string]*pipelineRunStructureNode{
				"services": {
					Description: "Service workloads",
					Children:    map[string]*pipelineRunStructureNode{},
				},
			},
		},
		"team-1": {
			Description: "Already scoped",
			Children: map[string]*pipelineRunStructureNode{
				"platform": {
					Description: "Platform",
					Children:    map[string]*pipelineRunStructureNode{},
				},
			},
		},
	}

	got, err := normalizePipelineRunStructureForFolder("team-1", structure)
	if err != nil {
		t.Fatalf("normalizePipelineRunStructureForFolder() error = %v", err)
	}
	team, ok := got["team-1"]
	if !ok {
		t.Fatal("expected team-1 root group")
	}
	if _, ok := team.Children["dev"]; !ok {
		t.Fatal("expected dev to be nested under team-1")
	}
	if gotRepos := team.Children["dev"].Repos; len(gotRepos) != 1 || gotRepos[0] != "acme/app" {
		t.Fatalf("dev repos = %#v, want acme/app", gotRepos)
	}
	if _, ok := team.Children["platform"]; !ok {
		t.Fatal("expected already scoped platform group to remain under team-1")
	}
	if _, duplicated := team.Children["team-1"]; duplicated {
		t.Fatal("did not expect duplicated team-1 child")
	}
}

func TestConfigRepositoryPrecedence(t *testing.T) {
	systemRepo := models.ConfigRepository{ID: 1, ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	parentRepo := models.ConfigRepository{ID: 2, ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"}
	childRepo := models.ConfigRepository{ID: 3, ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1/dev"}
	otherRepo := models.ConfigRepository{ID: 4, ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-2"}

	if !canConfigRepositoryWriteOver(parentRepo, systemRepo, "team-1/build") {
		t.Fatal("group repo should be able to take over parent-managed global resources")
	}
	if !canConfigRepositoryWriteOver(childRepo, parentRepo, "team-1/dev/deploy") {
		t.Fatal("child group repo should be able to take over parent group resources in its subtree")
	}
	if canConfigRepositoryWriteOver(parentRepo, childRepo, "team-1/dev/deploy") {
		t.Fatal("parent group repo should not take over child group resources")
	}
	if !configRepositoryShadowsCurrent(childRepo, parentRepo, "team-1/dev/deploy") {
		t.Fatal("child group repo should shadow parent group repo in its subtree")
	}
	if canConfigRepositoryWriteOver(otherRepo, parentRepo, "team-1/build") {
		t.Fatal("unrelated group repo should not take over another group")
	}
}

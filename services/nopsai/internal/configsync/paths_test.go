package configsync

import (
	"testing"

	"nopsai/pkg/models"
)

func TestNormalizePathForTeam(t *testing.T) {
	tests := []struct {
		name      string
		boundTeam string
		relPath   string
		want      string
		wantErr   bool
	}{
		{name: "joins team", boundTeam: "team-1", relPath: "pipelines/build.yaml", want: "team-1/build"},
		{name: "strips duplicated team", boundTeam: "team-1", relPath: "pipelines/team-1/build.yaml", want: "team-1/build"},
		{name: "root prefix is absolute", boundTeam: "team-1", relPath: "pipelines/root/platform/build.yaml", want: "platform/build"},
		{name: "escaping path is rejected", boundTeam: "team-1", relPath: "../build.yaml", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizePathForTeam(tt.boundTeam, tt.relPath)
			if tt.wantErr {
				if err == nil {
					t.Fatal("NormalizePathForTeam() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizePathForTeam() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizePathForTeam() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRepositoryOwnershipRules(t *testing.T) {
	systemRepo := models.ConfigRepository{ID: 1, ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	parentRepo := models.ConfigRepository{ID: 2, ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"}
	childRepo := models.ConfigRepository{ID: 3, ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1/dev"}
	otherRepo := models.ConfigRepository{ID: 4, ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-2"}

	if !CanRepositoryWriteOver(parentRepo, systemRepo, "team-1/build") {
		t.Fatal("team repo should be allowed to take over its scoped resource from system repo")
	}
	if !CanRepositoryWriteOver(childRepo, parentRepo, "team-1/dev/deploy") {
		t.Fatal("child team repo should be allowed to take over child resource from parent team repo")
	}
	if CanRepositoryWriteOver(parentRepo, childRepo, "team-1/dev/deploy") {
		t.Fatal("parent team repo should not take over a child repo resource")
	}
	if !RepositoryShadowsCurrent(childRepo, parentRepo, "team-1/dev/deploy") {
		t.Fatal("child repo should shadow parent repo for its scoped resource")
	}
	if CanRepositoryWriteOver(otherRepo, parentRepo, "team-1/build") {
		t.Fatal("unrelated team repo should not write over another team scope")
	}
}

func TestPipelineIdentifierHelpers(t *testing.T) {
	path, name, ext, err := SplitPipelineIdentifier("root/team-1/build.yml")
	if err != nil {
		t.Fatalf("SplitPipelineIdentifier() error = %v", err)
	}
	if path != "team-1" || name != "build" || ext != ".yml" {
		t.Fatalf("identifier parts = (%q, %q, %q), want (team-1, build, .yml)", path, name, ext)
	}
	if got := BuildPipelineFilePath(path, name, ext); got != "team-1/build.yml" {
		t.Fatalf("BuildPipelineFilePath() = %q, want team-1/build.yml", got)
	}
}

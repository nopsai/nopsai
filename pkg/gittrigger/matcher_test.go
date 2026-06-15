package gittrigger

import (
	"testing"

	"nopsai/pkg/models"
)

func TestFindMatchesBranchAndChangedPaths(t *testing.T) {
	manifest := models.Manifest{Triggers: []models.Trigger{
		{
			On:           "push",
			Branches:     []string{"main", "release/*"},
			IncludePaths: []string{"services/api/**", "pkg/**"},
			ExcludePaths: []string{"**/*.md", "services/api/generated/**"},
			Pipelines:    []models.PipelineSource{{Path: "api-ci"}},
			Scope:        "prod",
		},
	}}

	match := Find(manifest, Event{
		Type:              "push",
		Ref:               "refs/heads/release/1.2",
		RepositoryName:    "api",
		ChangedFiles:      []string{"README.md", "services/api/main.go"},
		ChangedFilesKnown: true,
	})
	if len(match.Pipelines) != 1 || match.Pipelines[0].Path != "api-ci" || match.Scope != "prod" {
		t.Fatalf("Find() = %#v, want api-ci in prod", match)
	}
}

func TestFindSkipsWhenOnlyExcludedPathsChanged(t *testing.T) {
	manifest := models.Manifest{Triggers: []models.Trigger{{
		On:           "push",
		Branches:     []string{"main"},
		IncludePaths: []string{"**"},
		ExcludePaths: []string{"docs/**", "**/*.md"},
		Pipelines:    []models.PipelineSource{{Path: "ci"}},
	}}}

	match := Find(manifest, Event{
		Type:              "push",
		Ref:               "refs/heads/main",
		RepositoryName:    "api",
		ChangedFiles:      []string{"docs/guide.md", "README.md"},
		ChangedFilesKnown: true,
	})
	if len(match.Pipelines) != 0 {
		t.Fatalf("Find() = %#v, want no match", match)
	}
}

func TestFindFailsOpenWhenChangedFilesAreUnavailable(t *testing.T) {
	manifest := models.Manifest{Triggers: []models.Trigger{{
		On:           "pull_request",
		Branches:     []string{"main"},
		IncludePaths: []string{"services/ui/**"},
		Pipelines:    []models.PipelineSource{{Path: "ui-ci"}},
	}}}

	match := Find(manifest, Event{
		Type:              "pull_request",
		Ref:               "refs/heads/feature/webhook-ui",
		TargetRef:         "refs/heads/main",
		RepositoryName:    "ui",
		ChangedFilesKnown: false,
	})
	if len(match.Pipelines) != 1 {
		t.Fatalf("Find() = %#v, want fail-open match", match)
	}
}

func TestFindHonorsSkipRepositoryTagAndPullRequestBranches(t *testing.T) {
	tests := []struct {
		name     string
		trigger  models.Trigger
		event    Event
		wantPath string
	}{
		{
			name: "skip repository",
			trigger: models.Trigger{
				On:        "all",
				SkipRepos: []string{"docs-*"},
				Pipelines: []models.PipelineSource{{Path: "ci"}},
			},
			event: Event{Type: "push", Ref: "refs/heads/main", RepositoryName: "docs-site"},
		},
		{
			name: "tag",
			trigger: models.Trigger{
				On:        "push",
				Tags:      []string{"v*"},
				Pipelines: []models.PipelineSource{{Path: "release"}},
			},
			event:    Event{Type: "push", Ref: "refs/tags/v1.2.0", RepositoryName: "api"},
			wantPath: "release",
		},
		{
			name: "pull request branch",
			trigger: models.Trigger{
				On:           "pull_request",
				Branches:     []string{"main"},
				SkipBranches: []string{"release/**"},
				Pipelines:    []models.PipelineSource{{Path: "pr"}},
			},
			event:    Event{Type: "pull_request", Ref: "refs/heads/feature/api", TargetRef: "refs/heads/main", RepositoryName: "api"},
			wantPath: "pr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := Find(models.Manifest{Triggers: []models.Trigger{tt.trigger}}, tt.event)
			if tt.wantPath == "" {
				if len(match.Pipelines) != 0 {
					t.Fatalf("Find() = %#v, want no match", match)
				}
				return
			}
			if len(match.Pipelines) != 1 || match.Pipelines[0].Path != tt.wantPath {
				t.Fatalf("Find() = %#v, want %s", match, tt.wantPath)
			}
		})
	}
}

func TestMatchPatternSupportsRecursiveGlob(t *testing.T) {
	tests := []struct {
		pattern string
		value   string
		want    bool
	}{
		{pattern: "services/api/**", value: "services/api/main.go", want: true},
		{pattern: "services/api/**", value: "services/api/internal/handler.go", want: true},
		{pattern: "**/*.md", value: "README.md", want: true},
		{pattern: "**/*.md", value: "docs/guide.md", want: true},
		{pattern: "release/*", value: "release/1.0", want: true},
		{pattern: "release/*", value: "release/1.0/hotfix", want: false},
	}
	for _, tt := range tests {
		if got := MatchPattern(tt.pattern, tt.value); got != tt.want {
			t.Fatalf("MatchPattern(%q, %q) = %v, want %v", tt.pattern, tt.value, got, tt.want)
		}
	}
}

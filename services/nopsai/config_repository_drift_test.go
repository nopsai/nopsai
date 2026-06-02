package main

import (
	"database/sql"
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestDiffConfigRepositoryFiles(t *testing.T) {
	items := diffConfigRepositoryFiles(
		map[string]string{
			"pipelines/removed.yaml": "old\n",
			"pipelines/same.yaml":    "same\n",
			"pipelines/changed.yaml": "old\n",
		},
		map[string]string{
			"pipelines/added.yaml":   "new\n",
			"pipelines/same.yaml":    "same\n",
			"pipelines/changed.yaml": "new\n",
		},
	)
	if got, want := len(items), 4; got != want {
		t.Fatalf("len(items) = %d, want %d", got, want)
	}
	wantStatuses := map[string]string{
		"pipelines/added.yaml":   "added",
		"pipelines/changed.yaml": "modified",
		"pipelines/removed.yaml": "deleted",
		"pipelines/same.yaml":    "unchanged",
	}
	for _, item := range items {
		if want := wantStatuses[item.Path]; item.Status != want {
			t.Fatalf("item %s status = %q, want %q", item.Path, item.Status, want)
		}
		if item.Path == "pipelines/removed.yaml" && !item.Delete {
			t.Fatal("deleted item did not set delete flag")
		}
	}
}

func TestDiffConfigRepositoryFilesIgnoresKnowledgeFrontMatterWrapper(t *testing.T) {
	gitContent := "name: runtime-output-safety\nkind: guardrail\ncontent: |\n  # Runtime Output Safety\n\n  Keep runtime values private.\n"
	desiredContent := "---\nname: runtime-output-safety\nkind: guardrail\ncontent: |\n  # Runtime Output Safety\n\n  Keep runtime values private.\n---\n"
	items := diffConfigRepositoryFiles(
		map[string]string{"knowledge/guardrail/data-team/runtime-output-safety.yaml": normalizeConfigRepositoryFileContent(gitContent)},
		map[string]string{"knowledge/guardrail/data-team/runtime-output-safety.yaml": normalizeConfigRepositoryFileContent(desiredContent)},
	)
	if got, want := len(items), 1; got != want {
		t.Fatalf("len(items) = %d, want %d", got, want)
	}
	if items[0].Status != "unchanged" {
		t.Fatalf("knowledge document status = %q, want unchanged", items[0].Status)
	}
}

func TestDiffConfigRepositoryFilesDetectsKnowledgeContentChange(t *testing.T) {
	items := diffConfigRepositoryFiles(
		map[string]string{"knowledge/guardrail/data-team/runtime-output-safety.yaml": "name: runtime-output-safety\nkind: guardrail\ncontent: old\n"},
		map[string]string{"knowledge/guardrail/data-team/runtime-output-safety.yaml": "---\nname: runtime-output-safety\nkind: guardrail\ncontent: new\n---\n"},
	)
	if got, want := len(items), 1; got != want {
		t.Fatalf("len(items) = %d, want %d", got, want)
	}
	if items[0].Status != "modified" {
		t.Fatalf("knowledge document status = %q, want modified", items[0].Status)
	}
}

func TestDiffConfigRepositoryFilesDetectsKnowledgeAccessChange(t *testing.T) {
	gitContent := `---
name: runtime-output-safety
kind: guardrail
access:
  visibility: restricted
  use_access:
    grants:
      - repository: hosein-yousefii/test-app
content: Keep runtime values private.
---
`
	desiredContent := `---
name: runtime-output-safety
kind: guardrail
access:
  visibility: workspace
content: Keep runtime values private.
---
`
	items := diffConfigRepositoryFiles(
		map[string]string{"knowledge/guardrail/data-team/runtime-output-safety.yaml": normalizeConfigRepositoryFileContent(gitContent)},
		map[string]string{"knowledge/guardrail/data-team/runtime-output-safety.yaml": normalizeConfigRepositoryFileContent(desiredContent)},
	)
	if got, want := len(items), 1; got != want {
		t.Fatalf("len(items) = %d, want %d", got, want)
	}
	if items[0].Status != "modified" {
		t.Fatalf("knowledge document status = %q, want modified", items[0].Status)
	}
}

func TestDiffConfigRepositoryFilesIgnoresKnowledgeAccessFormatting(t *testing.T) {
	gitContent := `---
name: runtime-output-safety
kind: guardrail
access:
  repositories:
    - hosein-yousefii/test-app
content: Keep runtime values private.
---
`
	desiredContent := `---
name: runtime-output-safety
kind: guardrail
access:
  visibility: restricted
  use_access:
    grants:
      - repository: hosein-yousefii/test-app
content: Keep runtime values private.
---
`
	items := diffConfigRepositoryFiles(
		map[string]string{"knowledge/guardrail/data-team/runtime-output-safety.yaml": normalizeConfigRepositoryFileContent(gitContent)},
		map[string]string{"knowledge/guardrail/data-team/runtime-output-safety.yaml": normalizeConfigRepositoryFileContent(desiredContent)},
	)
	if got, want := len(items), 1; got != want {
		t.Fatalf("len(items) = %d, want %d", got, want)
	}
	if items[0].Status != "unchanged" {
		t.Fatalf("knowledge document status = %q, want unchanged", items[0].Status)
	}
}

func TestConfigRepositoryRelativeGitPath(t *testing.T) {
	rel, ok := configRepositoryRelativeGitPath("nopsai", "nopsai/pipelines/deploy.yaml")
	if !ok || rel != "pipelines/deploy.yaml" {
		t.Fatalf("relative path = %q, %t; want pipelines/deploy.yaml, true", rel, ok)
	}
	if _, ok := configRepositoryRelativeGitPath("nopsai", "other/pipelines/deploy.yaml"); ok {
		t.Fatal("path outside base path was accepted")
	}
}

func TestConfigRepositoryExportPathForFolderScope(t *testing.T) {
	repo := models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"}
	got, ok := configRepositoryExportPath(repo, "team-1/services/api/deploy", "", "pipelines", ".yaml", false, sql.NullInt64{})
	if !ok || got != "pipelines/services/api/deploy.yaml" {
		t.Fatalf("export path = %q, %t; want pipelines/services/api/deploy.yaml, true", got, ok)
	}
	if _, ok := configRepositoryExportPath(repo, "team-2/services/api/deploy", "", "pipelines", ".yaml", false, sql.NullInt64{}); ok {
		t.Fatal("resource outside folder scope was accepted")
	}
}

func TestConfigRepositoryExportPathStripsBasePathFromManagedSource(t *testing.T) {
	repo := models.ConfigRepository{ID: 7, ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1", BasePath: "configs/team-1"}
	got, ok := configRepositoryExportPath(repo, "team-1/services/api/deploy", "configs/team-1/pipelines/services/api/deploy.yaml", "pipelines", ".yaml", true, sql.NullInt64{Int64: 7, Valid: true})
	if !ok || got != "pipelines/services/api/deploy.yaml" {
		t.Fatalf("managed export path = %q, %t; want pipelines/services/api/deploy.yaml, true", got, ok)
	}
	got, ok = configRepositoryExportPath(repo, "team-1/services/api/deploy", "pipelines/services/api/deploy.yaml", "pipelines", ".yaml", true, sql.NullInt64{Int64: 7, Valid: true})
	if !ok || got != "pipelines/services/api/deploy.yaml" {
		t.Fatalf("relative managed export path = %q, %t; want pipelines/services/api/deploy.yaml, true", got, ok)
	}
}

func TestConfigRepositoryIncludesResourceSkipsDelegatedScopes(t *testing.T) {
	systemRepo := models.ConfigRepository{ID: 1, ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	delegatedScopes := []string{"team-1"}

	if configRepositoryIncludesResource(systemRepo, "team-1/test", "database", sql.NullInt64{}, false, delegatedScopes) {
		t.Fatal("system repo should not include database resource under delegated folder")
	}
	if configRepositoryIncludesResource(systemRepo, "team-1/test", "git", sql.NullInt64{Int64: 1, Valid: true}, true, delegatedScopes) {
		t.Fatal("system repo should not keep claiming managed resource under delegated folder")
	}
	if !configRepositoryIncludesResource(systemRepo, "team-10/test", "database", sql.NullInt64{}, false, delegatedScopes) {
		t.Fatal("similarly named but unrelated folder should remain in system repo")
	}
	if !configRepositoryIncludesResource(systemRepo, "platform/test", "database", sql.NullInt64{}, false, delegatedScopes) {
		t.Fatal("unrelated system resource should remain in system repo")
	}
}

func TestConfigRepositoryIncludesResourceSkipsChildDelegatedScopesForFolderRepo(t *testing.T) {
	parentRepo := models.ConfigRepository{ID: 2, ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"}
	delegatedScopes := []string{"team-1/dev"}

	if configRepositoryIncludesResource(parentRepo, "team-1/dev/deploy", "database", sql.NullInt64{}, false, delegatedScopes) {
		t.Fatal("parent folder repo should not include database resource under child delegated folder")
	}
	if !configRepositoryIncludesResource(parentRepo, "team-1/deploy", "database", sql.NullInt64{}, false, delegatedScopes) {
		t.Fatal("parent folder repo should still include resources directly under its scope")
	}
}

func TestRenderKnowledgeContextGitOpsDocument(t *testing.T) {
	got := renderKnowledgeContextGitOpsDocument("runbook", "deploy-api", "", "# Deploy\n\nRun it.\n", "runbook/team-1/deploy-api", nil)
	if want := "---\nname: deploy-api\nkind: runbook\ncontent: |\n"; len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("knowledge document prefix = %q, want %q", got, want)
	}
	if contains := "title:"; strings.Contains(got, contains) {
		t.Fatalf("knowledge document included %q: %s", contains, got)
	}
	if contains := "description:"; strings.Contains(got, contains) {
		t.Fatalf("knowledge document included empty %q: %s", contains, got)
	}
}

func TestSyncConfigRepositoryYAMLAccessBlock(t *testing.T) {
	got, err := syncConfigRepositoryYAMLAccessBlock(`name: deploy
access:
  visibility: workspace
steps:
  - name: run
    script: echo ok
`, &configRepositoryEmbeddedAccessFile{
		Visibility: resourceVisibilityRestricted,
		UseAccess: &configRepositoryEmbeddedUseAccessFile{
			Grants: []configRepositoryEmbeddedUseGrantFile{{Repository: "hosein-yousefii/test-app"}},
		},
	})
	if err != nil {
		t.Fatalf("syncConfigRepositoryYAMLAccessBlock() error = %v", err)
	}
	if strings.Count("\n"+got, "\naccess:") != 1 {
		t.Fatalf("access block count in %q, want 1", got)
	}
	if !strings.Contains(got, "visibility: restricted") || !strings.Contains(got, "repository: hosein-yousefii/test-app") {
		t.Fatalf("updated access block missing from %q", got)
	}

	got, err = syncConfigRepositoryYAMLAccessBlock(got, nil)
	if err != nil {
		t.Fatalf("syncConfigRepositoryYAMLAccessBlock(remove) error = %v", err)
	}
	if strings.Contains("\n"+got, "\naccess:") {
		t.Fatalf("access block was not removed: %q", got)
	}
}

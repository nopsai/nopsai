package nopsai

import (
	"database/sql"
	"strings"
	"testing"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"

	"gopkg.in/yaml.v3"
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

func TestDiffConfigRepositoryFilesIgnoresTeamRepoKnowledgeFrontMatterWrapper(t *testing.T) {
	gitContent := "name: go-style\nkind: guideline\ncontent: |\n  # Go Style\n\n  Keep helpers small.\n"
	desiredContent := "---\nname: go-style\nkind: guideline\ncontent: |\n  # Go Style\n\n  Keep helpers small.\n---\n"
	items := diffConfigRepositoryFiles(
		map[string]string{"knowledge/guideline/team-1/go-style.md": normalizeConfigRepositoryFileContent(gitContent)},
		map[string]string{"knowledge/guideline/team-1/go-style.md": normalizeConfigRepositoryFileContent(desiredContent)},
	)
	if got, want := len(items), 1; got != want {
		t.Fatalf("len(items) = %d, want %d", got, want)
	}
	if items[0].Status != "unchanged" {
		t.Fatalf("team repo knowledge document status = %q, want unchanged", items[0].Status)
	}
}

func TestDiffConfigRepositoryFilesMigratesLegacyTeamKnowledgePath(t *testing.T) {
	content := normalizeConfigRepositoryFileContent("---\nname: go-style\nkind: guideline\ncontent: Keep helpers small.\n---\n")
	items := diffConfigRepositoryFiles(
		map[string]string{"knowledge/guideline/go-style.md": content},
		map[string]string{"knowledge/guideline/team-1/go-style.md": content},
	)
	if got, want := len(items), 2; got != want {
		t.Fatalf("len(items) = %d, want %d", got, want)
	}
	statuses := map[string]string{}
	for _, item := range items {
		statuses[item.Path] = item.Status
	}
	if statuses["knowledge/guideline/go-style.md"] != "deleted" {
		t.Fatalf("legacy knowledge path status = %q, want deleted", statuses["knowledge/guideline/go-style.md"])
	}
	if statuses["knowledge/guideline/team-1/go-style.md"] != "added" {
		t.Fatalf("canonical knowledge path status = %q, want added", statuses["knowledge/guideline/team-1/go-style.md"])
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
      - repository: nopsai/test-app
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
    - nopsai/test-app
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
      - repository: nopsai/test-app
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

func TestConfigRepositoryDriftPathIncludesSyncableResourceFamilies(t *testing.T) {
	if !isConfigRepositoryDriftPath("access/service-accounts.yaml") {
		t.Fatal("service account access export path should be included in drift")
	}
	for _, path := range []string{
		"access/all.yaml",
		"access/grants.yaml",
		"ai-profiles.yaml",
		"ai-profiles.yml",
		"notifications.yaml",
		"config-repositories/teams/team-1/notifications.yaml",
		"config-repositories/teams/team-1/structure.yaml",
		"setting/system/auth.yaml",
		"setting/system/credentials.yaml",
		"setting/system/github.yaml",
		"setting/system/mail.yaml",
		"setting/system/data-management.yaml",
		"setting/system/llm_profile.yaml",
		"setting/system/agent-profiles.yaml",
		"setting/system/mcp.yaml",
		"setting/system/runner.yaml",
	} {
		if !isConfigRepositoryDriftPath(path) {
			t.Fatalf("syncable path %q should be included in drift", path)
		}
	}
	if isConfigRepositoryDriftPath("access/readme.md") {
		t.Fatal("non-YAML access files should not be included in drift")
	}
	for _, path := range []string{"notifications/teams/team-1.yaml", "pipelineruns/structure.yaml", "settings/system/runner.yaml"} {
		if isConfigRepositoryDriftPath(path) {
			t.Fatalf("legacy path %q should not be included in drift", path)
		}
	}
}

func TestConfigRepositoryNotificationRoutePathUsesColocatedSystemPath(t *testing.T) {
	repo := models.ConfigRepository{ID: 7, ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	got, ok := configRepositoryNotificationRoutePath(repo, "team-1/dev")
	if !ok || got != "config-repositories/teams/team-1/dev/notifications.yaml" {
		t.Fatalf("notification route path = %q, %t; want colocated team-1/dev path", got, ok)
	}
}

func TestConfigRepositoryNotificationRoutePathUsesExplicitTeamPathForTeamRepo(t *testing.T) {
	repo := models.ConfigRepository{ID: 7, ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"}
	got, ok := configRepositoryNotificationRoutePath(repo, "team-1")
	if !ok || got != "config-repositories/teams/team-1/notifications.yaml" {
		t.Fatalf("notification route path = %q, %t; want explicit team path", got, ok)
	}
	got, ok = configRepositoryNotificationRoutePath(repo, "team-1/dev")
	if !ok || got != "config-repositories/teams/team-1/dev/notifications.yaml" {
		t.Fatalf("child notification route path = %q, %t; want explicit child path", got, ok)
	}
}

func TestConfigRepositoryTriggerExportPathUsesRepositoryPathWithTeamScope(t *testing.T) {
	repo := models.ConfigRepository{ID: 7, ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "black"}
	got, ok := configRepositoryTriggerExportPath(repo, "team-1/service-api", "", false, sql.NullInt64{})
	if !ok || got != "triggers/team-1/service-api.yaml" {
		t.Fatalf("trigger export path = %q, %t; want repository owner path", got, ok)
	}

	got, ok = configRepositoryTriggerExportPath(repo, "black/service-api", "", false, sql.NullInt64{})
	if !ok || got != "triggers/black/service-api.yaml" {
		t.Fatalf("trigger export path = %q, %t; want explicit team path", got, ok)
	}

	got, ok = configRepositoryTriggerExportPath(repo, "black/service-api", "triggers/service-api.yaml", true, sql.NullInt64{Int64: 7, Valid: true})
	if !ok || got != "triggers/black/service-api.yaml" {
		t.Fatalf("stale managed trigger export path = %q, %t; want triggers/black/service-api.yaml", got, ok)
	}
}

func TestConfigRepositoryFlatGitOpsExportPathsKeepTeamPrefix(t *testing.T) {
	repo := models.ConfigRepository{ID: 7, ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"}

	got, ok := externalTriggerExportPath(repo, externalTriggerRecord{ID: "team-1-deploy-prod"}, "", false, false, 0)
	if !ok || got != "external-triggers/team-1-deploy-prod.yaml" {
		t.Fatalf("external trigger export path = %q, %t; want external-triggers/team-1-deploy-prod.yaml, true", got, ok)
	}

	got, ok = gitWebhookSourceExportPath(repo, gitWebhookSourceRecord{ID: "team-1-gitlab-main"}, "", false, false, 0)
	if !ok || got != "git-webhook-sources/team-1-gitlab-main.yaml" {
		t.Fatalf("git webhook source export path = %q, %t; want git-webhook-sources/team-1-gitlab-main.yaml, true", got, ok)
	}

	got, ok = externalTriggerExportPath(repo, externalTriggerRecord{ID: "team-1-deploy-prod"}, "external-triggers/deploy-prod.yaml", true, true, 7)
	if !ok || got != "external-triggers/team-1-deploy-prod.yaml" {
		t.Fatalf("stale managed external trigger export path = %q, %t; want external-triggers/team-1-deploy-prod.yaml, true", got, ok)
	}

	got, ok = gitWebhookSourceExportPath(repo, gitWebhookSourceRecord{ID: "team-1-gitlab-main"}, "git-webhook-sources/gitlab-main.yaml", true, true, 7)
	if !ok || got != "git-webhook-sources/team-1-gitlab-main.yaml" {
		t.Fatalf("stale managed git webhook source export path = %q, %t; want git-webhook-sources/team-1-gitlab-main.yaml, true", got, ok)
	}
}

func TestNotificationRouteTeamPathUsesResolvedHierarchy(t *testing.T) {
	records := map[int]teamPathRecord{
		2: {ID: 2, Name: "dev", Path: "team-1/dev"},
	}
	got, err := notificationRouteTeamPath(records, 2)
	if err != nil {
		t.Fatalf("notificationRouteTeamPath() error = %v", err)
	}
	if got != "team-1/dev" {
		t.Fatalf("notificationRouteTeamPath() = %q, want team-1/dev", got)
	}
	if _, err := notificationRouteTeamPath(records, 99); err == nil {
		t.Fatal("notificationRouteTeamPath() should reject an unknown team")
	}
}

func TestConfigRepositoryTeamStructureFilesUseScopedPaths(t *testing.T) {
	structure := map[string]*configsync.TeamStructureExportNode{
		"team-1": {
			Description: "Team 1",
			Apps: []configsync.TeamStructureAppExport{
				{Name: "api", RepoURL: "https://github.com/acme/api"},
			},
			Children: map[string]*configsync.TeamStructureExportNode{},
		},
		"team-2": {
			Description: "Team 2",
			Children:    map[string]*configsync.TeamStructureExportNode{},
		},
	}

	files, err := configRepositoryTeamStructureFiles(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		structure,
	)
	if err != nil {
		t.Fatalf("configRepositoryTeamStructureFiles() error = %v", err)
	}
	if _, ok := files["config-repositories/teams/structure.yaml"]; ok {
		t.Fatalf("aggregate structure file should not be exported: %#v", files)
	}
	team1 := files["config-repositories/teams/team-1/structure.yaml"]
	if !strings.Contains(team1, "description: Team 1") || strings.Contains(team1, "team-1:") {
		t.Fatalf("team-1 scoped structure = %q, want node content without wrapper", team1)
	}
	if _, ok := files["config-repositories/teams/team-2/structure.yaml"]; !ok {
		t.Fatalf("missing team-2 scoped structure file: %#v", files)
	}
}

func TestConfigRepositoryExportPathForTeamScope(t *testing.T) {
	repo := models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"}
	got, ok := configRepositoryExportPath(repo, "team-1/services/api/deploy", "", "pipelines", false, sql.NullInt64{})
	if !ok || got != "pipelines/team-1/services/api/deploy.yaml" {
		t.Fatalf("export path = %q, %t; want pipelines/team-1/services/api/deploy.yaml, true", got, ok)
	}
	if _, ok := configRepositoryExportPath(repo, "team-2/services/api/deploy", "", "pipelines", false, sql.NullInt64{}); ok {
		t.Fatal("resource outside team scope was accepted")
	}
}

func TestConfigRepositoryExportPathStripsBasePathFromManagedSource(t *testing.T) {
	repo := models.ConfigRepository{ID: 7, ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1", BasePath: "configs/team-1"}
	got, ok := configRepositoryExportPath(repo, "team-1/services/api/deploy", "configs/team-1/pipelines/team-1/services/api/deploy.yaml", "pipelines", true, sql.NullInt64{Int64: 7, Valid: true})
	if !ok || got != "pipelines/team-1/services/api/deploy.yaml" {
		t.Fatalf("managed export path = %q, %t; want pipelines/team-1/services/api/deploy.yaml, true", got, ok)
	}
	got, ok = configRepositoryExportPath(repo, "team-1/services/api/deploy", "pipelines/team-1/services/api/deploy.yaml", "pipelines", true, sql.NullInt64{Int64: 7, Valid: true})
	if !ok || got != "pipelines/team-1/services/api/deploy.yaml" {
		t.Fatalf("relative managed export path = %q, %t; want pipelines/team-1/services/api/deploy.yaml, true", got, ok)
	}
}

func TestConfigRepositoryIncludesResourceSkipsDelegatedScopes(t *testing.T) {
	systemRepo := models.ConfigRepository{ID: 1, ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	delegatedScopes := []string{"team-1"}

	if configRepositoryIncludesResource(systemRepo, "team-1/test", "database", sql.NullInt64{}, false, delegatedScopes) {
		t.Fatal("system repo should not include database resource under delegated team")
	}
	if configRepositoryIncludesResource(systemRepo, "team-1/test", "git", sql.NullInt64{Int64: 1, Valid: true}, true, delegatedScopes) {
		t.Fatal("system repo should not keep claiming managed resource under delegated team")
	}
	if !configRepositoryIncludesResource(systemRepo, "team-10/test", "database", sql.NullInt64{}, false, delegatedScopes) {
		t.Fatal("similarly named but unrelated team should remain in system repo")
	}
	if !configRepositoryIncludesResource(systemRepo, "platform/test", "database", sql.NullInt64{}, false, delegatedScopes) {
		t.Fatal("unrelated system resource should remain in system repo")
	}
}

func TestConfigRepositoryIncludesResourceSkipsChildDelegatedScopesForTeamRepo(t *testing.T) {
	parentRepo := models.ConfigRepository{ID: 2, ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"}
	delegatedScopes := []string{"team-1/dev"}

	if configRepositoryIncludesResource(parentRepo, "team-1/dev/deploy", "database", sql.NullInt64{}, false, delegatedScopes) {
		t.Fatal("parent team repo should not include database resource under child delegated team")
	}
	if !configRepositoryIncludesResource(parentRepo, "team-1/deploy", "database", sql.NullInt64{}, false, delegatedScopes) {
		t.Fatal("parent team repo should still include resources directly under its scope")
	}
}

func TestConfigRepositoryIncludesResourceLetsTeamRepoAdoptParentManagedStep(t *testing.T) {
	teamRepo := models.ConfigRepository{ID: 2, ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"}
	parentManaged := sql.NullInt64{Int64: 1, Valid: true}

	if !configRepositoryIncludesResource(teamRepo, "team-1/shared/checkout", "git", parentManaged, true, nil) {
		t.Fatal("team repo should include scoped reusable step currently managed by the global repo")
	}
	got, ok := configRepositoryExportPath(teamRepo, "team-1/shared/checkout", "steps/team-1/shared/checkout.yaml", "steps", true, parentManaged)
	if !ok || got != "steps/team-1/shared/checkout.yaml" {
		t.Fatalf("team step export path = %q, %t; want steps/team-1/shared/checkout.yaml, true", got, ok)
	}
}

func TestConfigRepositoryScopeFilePathLetsTeamRepoAdoptOrphanedGitScope(t *testing.T) {
	teamRepo := models.ConfigRepository{ID: 2, ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"}

	if !configRepositoryIncludesResource(teamRepo, "team-1/dev", "git", sql.NullInt64{}, false, nil) {
		t.Fatal("team repo should include scoped git variable without active config repo ownership")
	}
	got, ok := configRepositoryScopeFilePath(teamRepo, "team-1/dev", "", false, sql.NullInt64{})
	if !ok || got != "scopes/team-1/dev/scope.yaml" {
		t.Fatalf("team scope file path = %q, %t; want scopes/team-1/dev/scope.yaml, true", got, ok)
	}
}

func TestConfigRepositoryIncludesTeamOwnedWebhookResourcesUsesTeamRepoOwnership(t *testing.T) {
	teamRepo := models.ConfigRepository{ID: 2, ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"}
	parentManaged := sql.NullInt64{Int64: 1, Valid: true}

	if !configRepositoryIncludesExternalTrigger(teamRepo, externalTriggerRecord{RunTeamPath: "team-1/dev"}, "git", parentManaged, true, nil) {
		t.Fatal("team repo should include team-owned external trigger currently managed by broader repo")
	}
	if !configRepositoryIncludesGitWebhookSource(teamRepo, gitWebhookSourceRecord{TeamPath: "team-1/dev"}, "git", sql.NullInt64{}, false, nil) {
		t.Fatal("team repo should include team-owned git webhook source without active config repo ownership")
	}
	if configRepositoryIncludesGitWebhookSource(teamRepo, gitWebhookSourceRecord{TeamPath: "team-1/dev"}, "git", sql.NullInt64{}, false, []string{"team-1/dev"}) {
		t.Fatal("team repo should not include git webhook source delegated to child team repo")
	}
}

func TestConfigRepositoryKnowledgeExportPathCanonicalizesLegacySourcePaths(t *testing.T) {
	systemRepo := models.ConfigRepository{ID: 7, ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	got, ok := configRepositoryKnowledgeExportPath(systemRepo, "guideline", "team-1", "go-style", "knowledge/guideline/go-style.md", true, sql.NullInt64{Int64: 7, Valid: true})
	if !ok || got != "knowledge/guideline/team-1/go-style.md" {
		t.Fatalf("system knowledge export path = %q, %t; want knowledge/guideline/team-1/go-style.md, true", got, ok)
	}

	teamRepo := models.ConfigRepository{ID: 8, ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"}
	got, ok = configRepositoryKnowledgeExportPath(teamRepo, "guideline", "team-1", "go-style", "knowledge/guideline/go-style.md", true, sql.NullInt64{Int64: 8, Valid: true})
	if !ok || got != "knowledge/guideline/team-1/go-style.md" {
		t.Fatalf("team knowledge export path = %q, %t; want knowledge/guideline/team-1/go-style.md, true", got, ok)
	}

	got, ok = configRepositoryKnowledgeExportPath(systemRepo, "guideline", "team-1", "go-style", "knowledge/guideline/team-1/go-style.yaml", true, sql.NullInt64{Int64: 7, Valid: true})
	if !ok || got != "knowledge/guideline/team-1/go-style.yaml" {
		t.Fatalf("canonical YAML knowledge export path = %q, %t; want knowledge/guideline/team-1/go-style.yaml, true", got, ok)
	}
}

func TestConfigRepositoryIncludesBasicRoleGrantKeepsSystemDelegatedScopeGrant(t *testing.T) {
	systemRepo := models.ConfigRepository{ID: 1, ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	if !configRepositoryIncludesBasicRoleGrant(systemRepo, grantResourceTeam, "team-1", []string{"team-1"}) {
		t.Fatal("system basic role grant should remain exportable even when the target team has a delegated config repo")
	}
}

func TestConfigRepositoryIncludesBasicRoleGrantSkipsChildDelegatedScopeForTeamRepo(t *testing.T) {
	parentRepo := models.ConfigRepository{ID: 2, ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"}
	delegatedScopes := []string{"team-1/dev"}

	if !configRepositoryIncludesBasicRoleGrant(parentRepo, grantResourceTeam, "team-1", delegatedScopes) {
		t.Fatal("parent team basic role grant should remain exportable")
	}
	if configRepositoryIncludesBasicRoleGrant(parentRepo, grantResourceTeam, "team-1/dev", delegatedScopes) {
		t.Fatal("child delegated team basic role grant should not be exported by parent team repo")
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
			Grants: []configRepositoryEmbeddedUseGrantFile{
				{Team: "data-team"},
				{Repository: "nopsai/test-app"},
				{ServiceAccount: "servicenow-prod"},
			},
		},
	})
	if err != nil {
		t.Fatalf("syncConfigRepositoryYAMLAccessBlock() error = %v", err)
	}
	if strings.Count("\n"+got, "\naccess:") != 1 {
		t.Fatalf("access block count in %q, want 1", got)
	}
	for _, want := range []string{
		"visibility: restricted",
		"team: data-team",
		"repository: nopsai/test-app",
		"service_account: servicenow-prod",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("updated access block missing %q from %q", want, got)
		}
	}
	for _, badIndent := range []string{
		"\n    visibility:",
		"\n    use_access:",
		"\n        grants:",
		"\n            - team:",
	} {
		if strings.Contains(got, badIndent) {
			t.Fatalf("access block used 4-space indentation %q in:\n%s", badIndent, got)
		}
	}
	for _, wantIndent := range []string{
		"\naccess:\n  visibility: restricted\n  use_access:\n    grants:\n      - team: data-team\n      - repository: nopsai/test-app\n      - service_account: servicenow-prod",
		"\nsteps:\n  - name: run\n    script: echo ok",
	} {
		if !strings.Contains("\n"+got, wantIndent) {
			t.Fatalf("rendered YAML missing 2-space indentation %q in:\n%s", wantIndent, got)
		}
	}

	got, err = syncConfigRepositoryYAMLAccessBlock(got, nil)
	if err != nil {
		t.Fatalf("syncConfigRepositoryYAMLAccessBlock(remove) error = %v", err)
	}
	if strings.Contains("\n"+got, "\naccess:") {
		t.Fatalf("access block was not removed: %q", got)
	}
}

func TestConfigRepositoryResourceAccessExportIncludesServiceAccountGrant(t *testing.T) {
	access := configRepositoryResourceAccessState{
		Visibility: resourceVisibilityRestricted,
		Grants: []configRepositoryResourceUseGrant{
			{
				ResourceType: grantResourcePipeline,
				SubjectType:  grantSubjectServiceAccount,
				SubjectID:    "webhook-deployer",
				Actions:      []string{"pipeline.use"},
			},
		},
	}.exportFile()

	if access == nil || access.UseAccess == nil || len(access.UseAccess.Grants) != 1 {
		t.Fatalf("exported access = %#v, want one grant", access)
	}
	grant := access.UseAccess.Grants[0]
	if grant.ServiceAccount != "webhook-deployer" {
		t.Fatalf("service account grant = %#v, want webhook-deployer", grant)
	}
	if grant.SubjectType != "" || grant.SubjectID != "" {
		t.Fatalf("service account grant should use shorthand fields, got %#v", grant)
	}
}

func TestConfigRepositoryResourceAccessExportUsesTeamGrantKey(t *testing.T) {
	access := configRepositoryResourceAccessState{
		Visibility: resourceVisibilityRestricted,
		Grants: []configRepositoryResourceUseGrant{
			{
				ResourceType: grantResourcePipeline,
				SubjectType:  grantSubjectTeam,
				SubjectID:    "data-team",
				Actions:      []string{"pipeline.use"},
			},
			{
				ResourceType: grantResourcePipeline,
				SubjectType:  grantSubjectTeam,
				SubjectID:    globalGrantID,
				Actions:      []string{"pipeline.use"},
			},
			{
				ResourceType: grantResourcePipeline,
				SubjectType:  grantSubjectRepository,
				SubjectID:    "nopsai/test-app",
				Actions:      []string{"pipeline.use"},
			},
			{
				ResourceType: grantResourcePipeline,
				SubjectType:  grantSubjectServiceAccount,
				SubjectID:    "servicenow-prod",
				Actions:      []string{"pipeline.use"},
			},
		},
	}.exportFile()

	content, err := marshalConfigRepositoryYAML(access)
	if err != nil {
		t.Fatalf("marshalConfigRepositoryYAML() error = %v", err)
	}
	rendered := string(content)
	for _, want := range []string{
		"visibility: restricted",
		"team: data-team",
		"team: global",
		"repository: nopsai/test-app",
		"service_account: servicenow-prod",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered access export missing %q:\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{
		"subject_type: team",
		"subject_id: data-team",
	} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("rendered access export should not contain %q:\n%s", forbidden, rendered)
		}
	}
}

func TestConfigRepositoryAccessExportDocumentRendersServiceAccounts(t *testing.T) {
	doc := configRepositoryAccessExportDocument{
		ServiceAccounts: []configRepositoryServiceAccountExport{
			{
				Sub:           "webhook-deployer",
				Email:         "webhook-deployer@example.com",
				Status:        "active",
				AdvancedRoles: []string{"viewer"},
			},
		},
		BasicRoles: []configRepositoryBasicRoleExport{
			{
				ServiceAccount: "webhook-deployer",
				Role:           productRoleDeveloper,
				Resource:       "pipeline:platform-maintenance",
			},
		},
	}

	content, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	rendered := string(content)
	for _, want := range []string{
		"service_accounts:",
		"sub: webhook-deployer",
		"advanced_roles:",
		"basic_roles:",
		"service_account: webhook-deployer",
		"resource: pipeline:platform-maintenance",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered access export missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "nopsat_") {
		t.Fatalf("rendered access export should not contain service account tokens:\n%s", rendered)
	}
	if strings.Contains(rendered, "subject_type:") || strings.Contains(rendered, "subject_id:") {
		t.Fatalf("service account basic role should use shorthand fields:\n%s", rendered)
	}
}

func TestConfigRepositoryBasicRoleExportUsesSubjectShortcuts(t *testing.T) {
	userGrant := configRepositoryBasicRoleExport{Role: productRoleOwner, Resource: configRepositoryBasicRoleResourceExport(grantResourceTeam, globalGrantID)}
	if !setConfigRepositoryBasicRoleSubjectExport(&userGrant, grantSubjectUser, "alice") {
		t.Fatal("expected user subject export to succeed")
	}
	serviceGrant := configRepositoryBasicRoleExport{Role: productRoleDeveloper, Resource: "pipeline:platform-maintenance"}
	if !setConfigRepositoryBasicRoleSubjectExport(&serviceGrant, grantSubjectServiceAccount, "webhook-deployer") {
		t.Fatal("expected service account subject export to succeed")
	}

	content, err := yaml.Marshal(configRepositoryAccessExportDocument{
		BasicRoles: []configRepositoryBasicRoleExport{userGrant, serviceGrant},
	})
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	rendered := string(content)
	for _, want := range []string{
		"user: alice",
		"resource: team:global",
		"service_account: webhook-deployer",
		"resource: pipeline:platform-maintenance",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered access export missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "subject_type:") || strings.Contains(rendered, "subject_id:") {
		t.Fatalf("basic role shortcuts should not include canonical subject fields:\n%s", rendered)
	}
}

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

func TestConfigRepositoryDriftPathIncludesSyncableResourceFamilies(t *testing.T) {
	if !isConfigRepositoryDriftPath("access/service-accounts.yaml") {
		t.Fatal("service account access export path should be included in drift")
	}
	for _, path := range []string{
		"access/all.yaml",
		"access/grants.yaml",
		"config-repositories/groups/team-1/notifications.yaml",
		"config-repositories/groups/team-1/structure.yaml",
		"setting/system/auth.yaml",
		"setting/system/mail.yaml",
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
	for _, path := range []string{"notifications/groups/team-1.yaml", "pipelineruns/structure.yaml", "settings/system/runner.yaml"} {
		if isConfigRepositoryDriftPath(path) {
			t.Fatalf("legacy path %q should not be included in drift", path)
		}
	}
}

func TestConfigRepositoryNotificationRoutePathUsesColocatedSystemPath(t *testing.T) {
	repo := models.ConfigRepository{ID: 7, ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	got, ok := configRepositoryNotificationRoutePath(repo, "team-1/dev", "config-repositories/groups/team-1/dev/notifications.yaml", true, sql.NullInt64{Int64: 7, Valid: true})
	if !ok || got != "config-repositories/groups/team-1/dev/notifications.yaml" {
		t.Fatalf("notification route path = %q, %t; want colocated team-1/dev path", got, ok)
	}
}

func TestConfigRepositoryNotificationRoutePathUsesRootFileForBoundFolder(t *testing.T) {
	repo := models.ConfigRepository{ID: 7, ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"}
	got, ok := configRepositoryNotificationRoutePath(repo, "team-1", "", false, sql.NullInt64{})
	if !ok || got != "notifications.yaml" {
		t.Fatalf("notification route path = %q, %t; want group root notifications.yaml", got, ok)
	}
	got, ok = configRepositoryNotificationRoutePath(repo, "team-1/dev", "", false, sql.NullInt64{})
	if !ok || got != "config-repositories/groups/dev/notifications.yaml" {
		t.Fatalf("child notification route path = %q, %t; want colocated child path", got, ok)
	}
}

func TestNotificationRouteGroupPathUsesResolvedHierarchy(t *testing.T) {
	records := map[int]groupPathRecord{
		2: {ID: 2, Name: "dev", Path: "team-1/dev"},
	}
	got, err := notificationRouteGroupPath(records, 2)
	if err != nil {
		t.Fatalf("notificationRouteGroupPath() error = %v", err)
	}
	if got != "team-1/dev" {
		t.Fatalf("notificationRouteGroupPath() = %q, want team-1/dev", got)
	}
	if _, err := notificationRouteGroupPath(records, 99); err == nil {
		t.Fatal("notificationRouteGroupPath() should reject an unknown group")
	}
}

func TestConfigRepositoryGroupStructureFilesUseScopedPaths(t *testing.T) {
	structure := map[string]*configsync.GroupStructureExportNode{
		"team-1": {
			Description: "Team 1",
			Apps: []configsync.GroupStructureAppExport{
				{Name: "api", RepoURL: "https://github.com/acme/api"},
			},
			Children: map[string]*configsync.GroupStructureExportNode{},
		},
		"team-2": {
			Description: "Team 2",
			Children:    map[string]*configsync.GroupStructureExportNode{},
		},
	}

	files, err := configRepositoryGroupStructureFiles(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		structure,
	)
	if err != nil {
		t.Fatalf("configRepositoryGroupStructureFiles() error = %v", err)
	}
	if _, ok := files["config-repositories/groups/structure.yaml"]; ok {
		t.Fatalf("aggregate structure file should not be exported: %#v", files)
	}
	team1 := files["config-repositories/groups/team-1/structure.yaml"]
	if !strings.Contains(team1, "description: Team 1") || strings.Contains(team1, "team-1:") {
		t.Fatalf("team-1 scoped structure = %q, want node content without wrapper", team1)
	}
	if _, ok := files["config-repositories/groups/team-2/structure.yaml"]; !ok {
		t.Fatalf("missing team-2 scoped structure file: %#v", files)
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

func TestConfigRepositoryIncludesBasicRoleGrantKeepsSystemDelegatedScopeGrant(t *testing.T) {
	systemRepo := models.ConfigRepository{ID: 1, ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	if !configRepositoryIncludesBasicRoleGrant(systemRepo, grantResourceFolder, "team-1", []string{"team-1"}) {
		t.Fatal("system basic role grant should remain exportable even when the target folder has a delegated config repo")
	}
}

func TestConfigRepositoryIncludesBasicRoleGrantSkipsChildDelegatedScopeForFolderRepo(t *testing.T) {
	parentRepo := models.ConfigRepository{ID: 2, ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"}
	delegatedScopes := []string{"team-1/dev"}

	if !configRepositoryIncludesBasicRoleGrant(parentRepo, grantResourceFolder, "team-1", delegatedScopes) {
		t.Fatal("parent folder basic role grant should remain exportable")
	}
	if configRepositoryIncludesBasicRoleGrant(parentRepo, grantResourceFolder, "team-1/dev", delegatedScopes) {
		t.Fatal("child delegated folder basic role grant should not be exported by parent folder repo")
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
	for _, badIndent := range []string{
		"\n    visibility:",
		"\n    use_access:",
		"\n        grants:",
		"\n            - repository:",
	} {
		if strings.Contains(got, badIndent) {
			t.Fatalf("access block used 4-space indentation %q in:\n%s", badIndent, got)
		}
	}
	for _, wantIndent := range []string{
		"\naccess:\n  visibility: restricted\n  use_access:\n    grants:\n      - repository: hosein-yousefii/test-app",
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
	userGrant := configRepositoryBasicRoleExport{Role: productRoleOwner, Resource: configRepositoryBasicRoleResourceExport(grantResourceFolder, generalGrantID)}
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
		"resource: folder:root",
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

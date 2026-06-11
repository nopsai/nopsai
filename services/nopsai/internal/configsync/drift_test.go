package configsync

import (
	"database/sql"
	"testing"

	"nopsai/pkg/models"
)

func testDriftOptions() DriftPathOptions {
	return DriftPathOptions{
		ExternalTriggersDirectory: "external-triggers",
		NotificationsDirectory:    "notifications",
		SettingsRelativePath: func(rel string) bool {
			switch rel {
			case "system/runner.yaml", "system/mail.yaml", "system/llm_profile.yaml", "system/agent-profiles.yaml", "system/mcp.yaml":
				return true
			default:
				return false
			}
		},
	}
}

func TestRelativeGitPath(t *testing.T) {
	rel, ok := RelativeGitPath("nopsai", "nopsai/pipelines/deploy.yaml")
	if !ok || rel != "pipelines/deploy.yaml" {
		t.Fatalf("RelativeGitPath() = %q, %t; want pipelines/deploy.yaml, true", rel, ok)
	}
	if _, ok := RelativeGitPath("nopsai", "other/pipelines/deploy.yaml"); ok {
		t.Fatal("path outside base path was accepted")
	}
}

func TestIsDriftPathIncludesSyncableResourceFamilies(t *testing.T) {
	options := testDriftOptions()
	for _, path := range []string{
		"access/service-accounts.yaml",
		"access/all.yaml",
		"access/grants.yaml",
		"config-repositories/groups/structure.yaml",
		"external-triggers/webhook.yaml",
		"notifications/groups/team-1.yaml",
		"pipelineruns/structure.yaml",
		"setting/system/llm_profile.yaml",
		"setting/system/agent-profiles.yaml",
		"setting/system/mcp.yaml",
		"settings/system/runner.yaml",
	} {
		if !IsDriftPath(path, options) {
			t.Fatalf("syncable path %q should be included in drift", path)
		}
	}
	if IsDriftPath("access/readme.md", options) {
		t.Fatal("non-YAML access files should not be included in drift")
	}
}

func TestExportPathForFolderScope(t *testing.T) {
	repo := models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"}
	got, ok := ExportPath(repo, "team-1/services/api/deploy", "", "pipelines", ".yaml", false, sql.NullInt64{}, testDriftOptions())
	if !ok || got != "pipelines/services/api/deploy.yaml" {
		t.Fatalf("ExportPath() = %q, %t; want pipelines/services/api/deploy.yaml, true", got, ok)
	}
	if _, ok := ExportPath(repo, "team-2/services/api/deploy", "", "pipelines", ".yaml", false, sql.NullInt64{}, testDriftOptions()); ok {
		t.Fatal("resource outside folder scope was accepted")
	}
}

func TestExportPathStripsBasePathFromManagedSource(t *testing.T) {
	repo := models.ConfigRepository{ID: 7, ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1", BasePath: "configs/team-1"}
	got, ok := ExportPath(repo, "team-1/services/api/deploy", "configs/team-1/pipelines/services/api/deploy.yaml", "pipelines", ".yaml", true, sql.NullInt64{Int64: 7, Valid: true}, testDriftOptions())
	if !ok || got != "pipelines/services/api/deploy.yaml" {
		t.Fatalf("managed ExportPath() = %q, %t; want pipelines/services/api/deploy.yaml, true", got, ok)
	}
	got, ok = ExportPath(repo, "team-1/services/api/deploy", "pipelines/services/api/deploy.yaml", "pipelines", ".yaml", true, sql.NullInt64{Int64: 7, Valid: true}, testDriftOptions())
	if !ok || got != "pipelines/services/api/deploy.yaml" {
		t.Fatalf("relative managed ExportPath() = %q, %t; want pipelines/services/api/deploy.yaml, true", got, ok)
	}
}

func TestIncludesResourceSkipsDelegatedScopes(t *testing.T) {
	systemRepo := models.ConfigRepository{ID: 1, ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	delegatedScopes := []string{"team-1"}

	if IncludesResource(systemRepo, "team-1/test", "database", sql.NullInt64{}, false, delegatedScopes) {
		t.Fatal("system repo should not include database resource under delegated folder")
	}
	if IncludesResource(systemRepo, "team-1/test", "git", sql.NullInt64{Int64: 1, Valid: true}, true, delegatedScopes) {
		t.Fatal("system repo should not keep claiming managed resource under delegated folder")
	}
	if !IncludesResource(systemRepo, "team-10/test", "database", sql.NullInt64{}, false, delegatedScopes) {
		t.Fatal("similarly named but unrelated folder should remain in system repo")
	}
	if !IncludesResource(systemRepo, "platform/test", "database", sql.NullInt64{}, false, delegatedScopes) {
		t.Fatal("unrelated system resource should remain in system repo")
	}
}

func TestScopeFilePathUsesDefaultScope(t *testing.T) {
	repo := models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	got, ok := ScopeFilePath(repo, "", "", false, sql.NullInt64{}, testDriftOptions())
	if !ok || got != "scopes/default/scope.yaml" {
		t.Fatalf("ScopeFilePath() = %q, %t; want scopes/default/scope.yaml, true", got, ok)
	}
}

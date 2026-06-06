package configsync

import (
	"testing"

	"nopsai/pkg/models"
)

func TestGroupStructureIncludesPathHonorsRepositoryScope(t *testing.T) {
	systemRepo := models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	if !GroupStructureIncludesPath(systemRepo, "team-1/platform") {
		t.Fatal("system config repository should include any group structure path")
	}

	folderRepo := models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"}
	if !GroupStructureIncludesPath(folderRepo, "team-1/platform") {
		t.Fatal("folder config repository should include paths under its scope")
	}
	if GroupStructureIncludesPath(folderRepo, "team-10/platform") {
		t.Fatal("folder config repository should not include similarly named sibling scopes")
	}
}

func TestBuildGroupStructureAppExportUsesRepositoryFallbacks(t *testing.T) {
	app, ok := BuildGroupStructureAppExport("", "", "acme/service-api")
	if !ok {
		t.Fatal("expected repository fallback app to be exportable")
	}
	if app.Name != "service-api" || app.RepoURL != "https://github.com/acme/service-api" {
		t.Fatalf("fallback app = %#v, want service-api and canonical URL", app)
	}

	if _, ok := BuildGroupStructureAppExport("", "", ""); ok {
		t.Fatal("empty app should not be exportable")
	}
}

func TestGroupStructureExportMapSortsAppsAndChildren(t *testing.T) {
	structure := map[string]*GroupStructureExportNode{}
	team := EnsureGroupStructureExportPath(structure, "team-1")
	team.Description = " Team "
	team.Apps = []GroupStructureAppExport{
		{Name: "worker", RepoURL: "https://github.com/acme/worker"},
		{Name: "api", RepoURL: "https://github.com/acme/api"},
	}
	EnsureGroupStructureExportPath(structure, "team-1/dev").Description = "Dev"

	out := GroupStructureExportMap(structure)
	teamMap, ok := out["team-1"].(map[string]any)
	if !ok {
		t.Fatalf("team-1 export = %#v, want map", out["team-1"])
	}
	if teamMap["description"] != "Team" {
		t.Fatalf("description = %#v, want trimmed Team", teamMap["description"])
	}
	apps, ok := teamMap["apps"].([]GroupStructureAppExport)
	if !ok || len(apps) != 2 {
		t.Fatalf("apps = %#v, want 2 exported apps", teamMap["apps"])
	}
	if apps[0].Name != "api" || apps[1].Name != "worker" {
		t.Fatalf("apps sorted order = %#v, want api then worker", apps)
	}
	if _, ok := teamMap["dev"]; !ok {
		t.Fatal("expected dev child to be exported")
	}
}

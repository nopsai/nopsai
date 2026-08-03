package nopsai

import (
	"strings"
	"testing"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
)

func TestRepositoryTriggerSchemaAddsProviderTeamAndWebhookSource(t *testing.T) {
	joined := strings.Join(repositoryTriggerSchemaStatements, "\n")
	for _, want := range []string{
		"ALTER TABLE triggers ADD COLUMN IF NOT EXISTS provider",
		"ALTER TABLE triggers ADD COLUMN IF NOT EXISTS team_path",
		"UPDATE triggers SET team_path = 'global'",
		"LOWER(BTRIM(team_path)) IN ('root', 'general', '__general__')",
		"ALTER TABLE triggers ADD COLUMN IF NOT EXISTS management",
		"ALTER TABLE triggers ADD COLUMN IF NOT EXISTS webhook_source_id",
		"FOREIGN KEY (webhook_source_id)",
		"CREATE INDEX IF NOT EXISTS idx_triggers_webhook_source",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("repository trigger schema missing %q in:\n%s", want, joined)
		}
	}
}

func TestRepositoryTriggerRecordFromManifestDefaultsMissingTeamToGlobal(t *testing.T) {
	repository := "hosein-yousefii/test-app"
	record, err := repositoryTriggerRecordFromManifest(
		repository,
		"triggers: []",
		"git",
		resourceVisibilityTeam,
		models.Manifest{},
		fallbackRepositoryTriggerTeamPath(repository),
	)
	if err != nil {
		t.Fatalf("repositoryTriggerRecordFromManifest() error = %v", err)
	}
	if record.TeamPath != globalGrantID {
		t.Fatalf("TeamPath = %q, want global", record.TeamPath)
	}
	if record.RepositoryForWebhook != "hosein-yousefii/test-app" {
		t.Fatalf("RepositoryForWebhook = %q, want full owner/repo for global trigger", record.RepositoryForWebhook)
	}
}

func TestRepositoryTriggerRecordFromManifestNormalizesMetadata(t *testing.T) {
	record, err := repositoryTriggerRecordFromManifest("team-1/acme/api", "triggers: []", "git", resourceVisibilityTeam, models.Manifest{
		Provider:      "GitLab",
		Team:          "team-1",
		WebhookSource: "corporate-gitlab",
	}, "root")
	if err != nil {
		t.Fatalf("repositoryTriggerRecordFromManifest() error = %v", err)
	}
	if record.Provider != "gitlab" || record.TeamPath != "team-1" || record.Management != repositoryTriggerManagementNopsAI {
		t.Fatalf("record metadata = %#v", record)
	}
	if record.RepositoryForWebhook != "acme/api" {
		t.Fatalf("RepositoryForWebhook = %q, want acme/api", record.RepositoryForWebhook)
	}
	if err := validateRepositoryTriggerForNopsAI(record); err != nil {
		t.Fatalf("validateRepositoryTriggerForNopsAI() error = %v", err)
	}
}

func TestRepositoryTriggerApplicationSkipsGlobalTriggers(t *testing.T) {
	_, ok, err := repositoryTriggerApplicationFromRecord(repositoryTriggerRecord{
		RepositoryName:       "hosein-yousefii/test-app",
		RepositoryForWebhook: "hosein-yousefii/test-app",
		Provider:             repositoryTriggerProviderGitHub,
		TeamPath:             globalGrantID,
	})
	if err != nil {
		t.Fatalf("repositoryTriggerApplicationFromRecord() error = %v", err)
	}
	if ok {
		t.Fatal("repositoryTriggerApplicationFromRecord() ok = true, want false for global trigger")
	}
}

func TestRepositoryTriggerScopesFromDefinition(t *testing.T) {
	scopes := repositoryTriggerScopesFromDefinition(`
triggers:
  - on: push
    pipelines: [pipelines/build.yaml]
  - on: push
    scope: /prod/
    pipelines: [pipelines/deploy.yaml]
  - on: pull_request
    scope: prod
    pipelines: [pipelines/test.yaml]
`)
	if got, want := strings.Join(scopes, ","), "default,prod"; got != want {
		t.Fatalf("repositoryTriggerScopesFromDefinition() = %q, want %q", got, want)
	}
}

func TestRepositoryTriggerValidationRequiresProviderSpecificIngress(t *testing.T) {
	gitlab := repositoryTriggerRecord{
		RepositoryName:       "acme/api",
		RepositoryForWebhook: "acme/api",
		Provider:             "gitlab",
		TeamPath:             "team-1",
		Management:           repositoryTriggerManagementNopsAI,
	}
	if err := validateRepositoryTriggerForNopsAI(gitlab); err == nil || !strings.Contains(err.Error(), "webhook_source is required") {
		t.Fatalf("validateRepositoryTriggerForNopsAI() error = %v, want webhook_source requirement", err)
	}

	github := repositoryTriggerRecord{
		RepositoryName:       "acme/api",
		RepositoryForWebhook: "acme/api",
		Provider:             "github",
		TeamPath:             "team-1",
		Management:           repositoryTriggerManagementNopsAI,
		WebhookSourceID:      "manual-source",
	}
	if err := validateRepositoryTriggerForNopsAI(github); err == nil || !strings.Contains(err.Error(), "automatic ingress") {
		t.Fatalf("validateRepositoryTriggerForNopsAI() error = %v, want automatic ingress requirement", err)
	}
}

func TestRepositoryTriggerApplicationFromRecordUsesProviderURL(t *testing.T) {
	app, ok, err := repositoryTriggerApplicationFromRecord(repositoryTriggerRecord{
		RepositoryName:       "yousefi.hosein.o/nopsai-config-gitlab",
		RepositoryForWebhook: "yousefi.hosein.o/nopsai-config-gitlab",
		Provider:             "gitlab",
		TeamPath:             "team-1",
	})
	if err != nil || !ok {
		t.Fatalf("repositoryTriggerApplicationFromRecord() = (%#v, %t, %v), want app", app, ok, err)
	}
	if app.Name != "nopsai-config-gitlab" || app.TeamPath != "team-1" {
		t.Fatalf("app identity = %#v", app)
	}
	if app.RepoURL != "https://gitlab.com/yousefi.hosein.o/nopsai-config-gitlab" {
		t.Fatalf("RepoURL = %q", app.RepoURL)
	}
}

func TestMergeRepositoryTriggerApplicationsIntoStructure(t *testing.T) {
	structure := map[string]*configsync.PipelineRunStructureNode{}
	err := mergeRepositoryTriggerApplicationsIntoStructure(structure, map[string]storedTrigger{
		"team-1/yousefi.hosein.o/nopsai-config-gitlab": {
			record: repositoryTriggerRecord{
				RepositoryName:       "team-1/yousefi.hosein.o/nopsai-config-gitlab",
				RepositoryForWebhook: "yousefi.hosein.o/nopsai-config-gitlab",
				Provider:             "gitlab",
				TeamPath:             "team-1",
			},
		},
	})
	if err != nil {
		t.Fatalf("mergeRepositoryTriggerApplicationsIntoStructure() error = %v", err)
	}
	node := structure["team-1"]
	if node == nil || len(node.Apps) != 1 {
		t.Fatalf("structure = %#v, want one app under team-1", structure)
	}
	app := node.Apps[0]
	if app.Name != "nopsai-config-gitlab" || app.RepositoryFullName != "yousefi.hosein.o/nopsai-config-gitlab" {
		t.Fatalf("app = %#v", app)
	}
}

package nopsai

import (
	"strings"
	"testing"

	"nopsai/config"
	"nopsai/pkg/models"
)

func TestParseGitOpsGitHubSettingsPlan(t *testing.T) {
	plan, err := parseGitOpsGitHubSettingsPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		gitOpsRuntimeSettingsDirectory{
			root: "setting",
			files: map[string]string{
				"setting/git-apps/github.yaml": `
provider: github
app_id: "123456"
private_key_credential_ref: credential://system/github/app-private-key
webhook_credential_ref: credential://system/github/webhook-secret
installations:
  - installation_id: "987654"
    account_login: nopsai
    account_type: organization
    enabled: true
    accessible_repositories: 99
    last_repository_refresh_at: "2026-07-25T10:00:00Z"
    last_error: should-not-be-gitops
`,
				"setting/system/runner.yaml": "github_app_id: ignored",
			},
		},
	)
	if err != nil {
		t.Fatalf("parseGitOpsGitHubSettingsPlan() error = %v", err)
	}
	if plan == nil {
		t.Fatal("parseGitOpsGitHubSettingsPlan() = nil, want plan")
	}
	if plan.sourcePath != "setting/git-apps/github.yaml" {
		t.Fatalf("sourcePath = %q", plan.sourcePath)
	}
	if plan.payload.GitBotAPIURL != nil {
		t.Fatalf("git_bot_api_url = %#v", plan.payload.GitBotAPIURL)
	}
	if plan.payload.GitHubAppID == nil || *plan.payload.GitHubAppID != "123456" {
		t.Fatalf("app_id = %#v", plan.payload.GitHubAppID)
	}
	if plan.payload.GitHubInstallationID != nil {
		t.Fatalf("github_installation_id = %#v", plan.payload.GitHubInstallationID)
	}
	if plan.payload.GitHubInstallations == nil || len(*plan.payload.GitHubInstallations) != 1 {
		t.Fatalf("installations = %#v", plan.payload.GitHubInstallations)
	}
	installation := (*plan.payload.GitHubInstallations)[0]
	if installation.InstallationID != "987654" || installation.AccountLogin != "nopsai" || installation.AccountType != "organization" || installation.Enabled == nil || !*installation.Enabled {
		t.Fatalf("installation = %#v", installation)
	}
	if installation.AccessibleRepositories != 0 || installation.LastRepositoryRefreshAt != "" || installation.LastError != "" {
		t.Fatalf("runtime metadata should be stripped from GitOps import: %#v", installation)
	}
	if plan.payload.GitHubPrivateKeyRef == nil || *plan.payload.GitHubPrivateKeyRef != "credential://system/github/app-private-key" {
		t.Fatalf("private_key_credential_ref = %#v", plan.payload.GitHubPrivateKeyRef)
	}
	if plan.payload.GitHubWebhookRef == nil || *plan.payload.GitHubWebhookRef != "credential://system/github/webhook-secret" {
		t.Fatalf("webhook_credential_ref = %#v", plan.payload.GitHubWebhookRef)
	}
}

func TestParseGitOpsGitHubSettingsPlanSupportsLegacyPathForMigration(t *testing.T) {
	plan, err := parseGitOpsGitHubSettingsPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		gitOpsRuntimeSettingsDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/github.yaml": `
git_bot_api_url: http://git-bot:8081
github_app_id: "123456"
github_installation_id: "987654"
github_private_key_credential_ref: credential://system/github/app-private-key
github_webhook_credential_ref: credential://system/github/webhook-secret
`,
			},
		},
	)
	if err != nil {
		t.Fatalf("parseGitOpsGitHubSettingsPlan() error = %v", err)
	}
	if plan == nil {
		t.Fatal("parseGitOpsGitHubSettingsPlan() = nil, want legacy plan")
	}
	if plan.sourcePath != "setting/system/github.yaml" {
		t.Fatalf("sourcePath = %q", plan.sourcePath)
	}
	if plan.payload.GitBotAPIURL == nil || *plan.payload.GitBotAPIURL != "http://git-bot:8081" {
		t.Fatalf("git_bot_api_url = %#v", plan.payload.GitBotAPIURL)
	}
	if plan.payload.GitHubInstallationID == nil || *plan.payload.GitHubInstallationID != "987654" {
		t.Fatalf("github_installation_id = %#v", plan.payload.GitHubInstallationID)
	}
}

func TestParseGitOpsGitHubSettingsPlanRejectsNonSystemRepo(t *testing.T) {
	_, err := parseGitOpsGitHubSettingsPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"},
		gitOpsRuntimeSettingsDirectory{
			root: "setting",
			files: map[string]string{
				"setting/git-apps/github.yaml": "app_id: \"123456\"",
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "system config repository") {
		t.Fatalf("expected system-scope error, got %v", err)
	}
}

func TestParseGitOpsGitHubSettingsFileRejectsInvalidYAML(t *testing.T) {
	_, err := parseGitOpsGitHubSettingsFile("app_id: [", "setting/git-apps/github.yaml")
	if err == nil || !strings.Contains(err.Error(), "failed to parse GitHub settings GitOps file") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestParseGitOpsGitHubSettingsFileRejectsRuntimeFields(t *testing.T) {
	_, err := parseGitOpsGitHubSettingsFile(`
github_app_id: "123456"
runner_id: runner-a
dispatcher_grpc_address: dispatcher:9090
`, "setting/git-apps/github.yaml")
	if err == nil || !strings.Contains(err.Error(), "setting/system/runner.yaml") {
		t.Fatalf("expected move-to-runner error, got %v", err)
	}
}

func TestParseGitOpsGitHubSettingsFileRejectsCanonicalUnsupportedFields(t *testing.T) {
	_, err := parseGitOpsGitHubSettingsFile(`
app_id: "123456"
github_installation_id: "987654"
`, "setting/git-apps/github.yaml")
	if err == nil || !strings.Contains(err.Error(), "unsupported field") {
		t.Fatalf("expected unsupported field error, got %v", err)
	}
}

func TestIsGitOpsGitHubSettingsRelativePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "git-apps/github.yaml", want: true},
		{path: "/git-apps/github.yaml", want: true},
		{path: "git-apps/github.yml", want: false},
		{path: "system/github.yaml", want: true},
		{path: "setting/system/github.yaml", want: false},
		{path: "setting/git-apps/github.yaml", want: false},
		{path: "system/runner.yaml", want: false},
		{path: "../git-apps/github.yaml", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isGitOpsGitHubSettingsRelativePath(tt.path); got != tt.want {
				t.Fatalf("isGitOpsGitHubSettingsRelativePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestBuildGitHubSettingsGitOpsFile(t *testing.T) {
	doc := buildGitHubSettingsGitOpsFile(config.Config{
		GitHubAppID: "123456",
		GitHubInstallations: []config.GitHubInstallationConfig{{
			InstallationID:          "987654",
			AccountLogin:            "nopsai",
			AccountType:             "organization",
			Enabled:                 boolPtr(true),
			AccessibleRepositories:  7,
			LastRepositoryRefreshAt: "2026-07-25T10:00:00Z",
			LastError:               "cached failure",
		}},
		GitHubPrivateKeyCredentialRef: "credential://system/github/app-private-key",
		GitHubWebhookCredentialRef:    "credential://system/github/webhook-secret",
	})
	if doc.GitBotAPIURL != nil {
		t.Fatalf("git_bot_api_url = %#v", doc.GitBotAPIURL)
	}
	if doc.Provider == nil || *doc.Provider != "github" {
		t.Fatalf("provider = %#v", doc.Provider)
	}
	if doc.AppID == nil || *doc.AppID != "123456" {
		t.Fatalf("app_id = %#v", doc.AppID)
	}
	if doc.GitHubInstallationID != nil || len(doc.GitHubInstallations) != 0 {
		t.Fatalf("legacy installation fields = (%#v, %#v)", doc.GitHubInstallationID, doc.GitHubInstallations)
	}
	if len(doc.Installations) != 1 || doc.Installations[0].InstallationID != "987654" {
		t.Fatalf("installations = %#v", doc.Installations)
	}
	if doc.Installations[0].AccessibleRepositories != 0 || doc.Installations[0].LastRepositoryRefreshAt != "" || doc.Installations[0].LastError != "" {
		t.Fatalf("runtime metadata should be stripped from GitOps export: %#v", doc.Installations[0])
	}
	if doc.PrivateKeyCredentialRef == nil || *doc.PrivateKeyCredentialRef != "credential://system/github/app-private-key" {
		t.Fatalf("private_key_credential_ref = %#v", doc.PrivateKeyCredentialRef)
	}
}

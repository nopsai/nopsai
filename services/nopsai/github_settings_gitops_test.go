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
				"setting/system/github.yaml": `
git_bot_api_url: http://git-bot:8081
github_app_id: "123456"
github_installation_id: "987654"
github_private_key_credential_ref: credential://system/github/app-private-key
github_webhook_credential_ref: credential://system/github/webhook-secret
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
	if plan.sourcePath != "setting/system/github.yaml" {
		t.Fatalf("sourcePath = %q", plan.sourcePath)
	}
	if plan.payload.GitBotAPIURL == nil || *plan.payload.GitBotAPIURL != "http://git-bot:8081" {
		t.Fatalf("git_bot_api_url = %#v", plan.payload.GitBotAPIURL)
	}
	if plan.payload.GitHubAppID == nil || *plan.payload.GitHubAppID != "123456" {
		t.Fatalf("github_app_id = %#v", plan.payload.GitHubAppID)
	}
	if plan.payload.GitHubInstallationID == nil || *plan.payload.GitHubInstallationID != "987654" {
		t.Fatalf("github_installation_id = %#v", plan.payload.GitHubInstallationID)
	}
	if plan.payload.GitHubPrivateKeyRef == nil || *plan.payload.GitHubPrivateKeyRef != "credential://system/github/app-private-key" {
		t.Fatalf("github_private_key_credential_ref = %#v", plan.payload.GitHubPrivateKeyRef)
	}
	if plan.payload.GitHubWebhookRef == nil || *plan.payload.GitHubWebhookRef != "credential://system/github/webhook-secret" {
		t.Fatalf("github_webhook_credential_ref = %#v", plan.payload.GitHubWebhookRef)
	}
}

func TestParseGitOpsGitHubSettingsPlanRejectsNonSystemRepo(t *testing.T) {
	_, err := parseGitOpsGitHubSettingsPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"},
		gitOpsRuntimeSettingsDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/github.yaml": "github_app_id: \"123456\"",
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "system config repository") {
		t.Fatalf("expected system-scope error, got %v", err)
	}
}

func TestParseGitOpsGitHubSettingsFileRejectsInvalidYAML(t *testing.T) {
	_, err := parseGitOpsGitHubSettingsFile("github_app_id: [", "setting/system/github.yaml")
	if err == nil || !strings.Contains(err.Error(), "failed to parse GitHub settings GitOps file") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestParseGitOpsGitHubSettingsFileRejectsRuntimeFields(t *testing.T) {
	_, err := parseGitOpsGitHubSettingsFile(`
github_app_id: "123456"
runner_id: runner-a
dispatcher_grpc_address: dispatcher:9090
`, "setting/system/github.yaml")
	if err == nil || !strings.Contains(err.Error(), "setting/system/runner.yaml") {
		t.Fatalf("expected move-to-runner error, got %v", err)
	}
}

func TestIsGitOpsGitHubSettingsRelativePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "system/github.yaml", want: true},
		{path: "/system/github.yaml", want: true},
		{path: "system/github.yml", want: false},
		{path: "setting/system/github.yaml", want: false},
		{path: "system/runner.yaml", want: false},
		{path: "../system/github.yaml", want: false},
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
		NopsaiGitBotAPIURL:            "http://git-bot:8081",
		GitHubAppID:                   "123456",
		GitHubInstallID:               "987654",
		GitHubPrivateKeyCredentialRef: "credential://system/github/app-private-key",
		GitHubWebhookCredentialRef:    "credential://system/github/webhook-secret",
	})
	if doc.GitBotAPIURL == nil || *doc.GitBotAPIURL != "http://git-bot:8081" {
		t.Fatalf("git_bot_api_url = %#v", doc.GitBotAPIURL)
	}
	if doc.GitHubPrivateKeyRef == nil || *doc.GitHubPrivateKeyRef != "credential://system/github/app-private-key" {
		t.Fatalf("github_private_key_credential_ref = %#v", doc.GitHubPrivateKeyRef)
	}
}

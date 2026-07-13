package nopsai

import (
	"testing"

	"github.com/google/go-github/v53/github"

	"nopsai/pkg/models"
)

func TestNormalizeGitWebhookSourceInput(t *testing.T) {
	enabled := false
	source, err := normalizeGitWebhookSourceInput(gitWebhookSourceInput{
		Name:          "GitLab Main",
		TeamPath:      "/platform/webhooks/",
		Provider:      " GITLAB ",
		Enabled:       &enabled,
		AuthMode:      "static_token",
		CredentialRef: "credential://system/webhooks/gitlab-main",
		RepositoryAllowlist: []string{
			"Acme/API",
			"acme/*",
			"acme/api",
		},
		RateLimit: map[string]any{"per_minute": 30},
	}, "")
	if err != nil {
		t.Fatalf("normalizeGitWebhookSourceInput() error = %v", err)
	}
	if source.ID != "gitlab-main" || source.Provider != "gitlab" || source.AuthMode != "static_token" {
		t.Fatalf("source identity = %#v", source)
	}
	if source.Enabled {
		t.Fatal("Enabled = true, want false")
	}
	if source.TeamPath != "platform/webhooks" {
		t.Fatalf("TeamPath = %q, want platform/webhooks", source.TeamPath)
	}
	if len(source.RepositoryAllowlist) != 2 || source.RepositoryAllowlist[0] != "acme/*" || source.RepositoryAllowlist[1] != "acme/api" {
		t.Fatalf("RepositoryAllowlist = %#v", source.RepositoryAllowlist)
	}
}

func TestNormalizeGitWebhookSourceInputValidatesSecurityConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		input gitWebhookSourceInput
	}{
		{
			name: "missing credential",
			input: gitWebhookSourceInput{
				ID:                  "gitlab",
				Provider:            "gitlab",
				AuthMode:            "hmac",
				RepositoryAllowlist: []string{"acme/*"},
			},
		},
		{
			name: "missing allowlist",
			input: gitWebhookSourceInput{
				ID:            "gitlab",
				Provider:      "gitlab",
				AuthMode:      "none",
				CredentialRef: "",
			},
		},
		{
			name: "invalid provider",
			input: gitWebhookSourceInput{
				ID:                  "unknown",
				Provider:            "unknown",
				AuthMode:            "none",
				RepositoryAllowlist: []string{"acme/*"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := normalizeGitWebhookSourceInput(tt.input, ""); err == nil {
				t.Fatal("normalizeGitWebhookSourceInput() error = nil")
			}
		})
	}
}

func TestGitWebhookRepositoryAllowed(t *testing.T) {
	allowlist := []string{"acme/api", "platform/*", "shared/**"}
	for _, repository := range []string{"acme/api", "platform/ui", "shared/team/service"} {
		if !gitWebhookRepositoryAllowed(repository, allowlist) {
			t.Fatalf("gitWebhookRepositoryAllowed(%q) = false", repository)
		}
	}
	if gitWebhookRepositoryAllowed("acme/private", allowlist) {
		t.Fatal("gitWebhookRepositoryAllowed() accepted repository outside allowlist")
	}
}

func TestParseGitOpsGitWebhookSourcesNormalizesTeamID(t *testing.T) {
	sources, err := parseGitOpsGitWebhookSources(map[string]string{
		"git-webhook-sources/gitlab-main.yaml": `
name: GitLab main
provider: gitlab
auth_mode: static_token
credential_ref: credential://system/webhooks/gitlab-main
repository_allowlist:
  - acme/*
rate_limit:
  per_minute: 50
`,
	}, "git-webhook-sources", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
		ScopeID:   "team-1",
	}, "team-1")
	if err != nil {
		t.Fatalf("parseGitOpsGitWebhookSources() error = %v", err)
	}
	source, ok := sources["team-1-gitlab-main"]
	if !ok {
		t.Fatalf("sources = %#v, want team-1-gitlab-main", sources)
	}
	if source.input.Provider != "gitlab" || source.input.CredentialRef != "credential://system/webhooks/gitlab-main" {
		t.Fatalf("source = %#v", source)
	}
	if source.input.TeamPath != "team-1" {
		t.Fatalf("TeamPath = %q, want team-1", source.input.TeamPath)
	}
}

func TestParseGitOpsGitWebhookSourcesNormalizesExplicitTeamPath(t *testing.T) {
	sources, err := parseGitOpsGitWebhookSources(map[string]string{
		"git-webhook-sources/gitlab-prod.yaml": `
name: GitLab prod
team_path: prod/webhooks
provider: gitlab
auth_mode: static_token
credential_ref: credential://system/webhooks/gitlab-prod
repository_allowlist:
  - acme/prod
`,
	}, "git-webhook-sources", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
		ScopeID:   "team-1",
	}, "team-1")
	if err != nil {
		t.Fatalf("parseGitOpsGitWebhookSources() error = %v", err)
	}
	source := sources["team-1-gitlab-prod"]
	if source.input.TeamPath != "team-1/prod/webhooks" {
		t.Fatalf("TeamPath = %q, want team-1/prod/webhooks", source.input.TeamPath)
	}
}

func TestGitHubPushChangedFilesDeduplicatesFiles(t *testing.T) {
	event := &github.PushEvent{Commits: []*github.HeadCommit{
		{
			Added:    []string{"a.go"},
			Modified: []string{"b.go", "a.go"},
			Removed:  []string{"old.go"},
		},
	}}
	files := githubPushChangedFiles(event)
	if len(files) != 3 || files[0] != "a.go" || files[1] != "b.go" || files[2] != "old.go" {
		t.Fatalf("githubPushChangedFiles() = %#v", files)
	}
}

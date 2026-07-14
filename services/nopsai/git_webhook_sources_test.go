package nopsai

import (
	"context"
	"testing"

	"github.com/google/go-github/v53/github"

	"nopsai/pkg/models"
	aaamodel "nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/credentials"
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
	if source.Visibility != gitWebhookSourceVisibilityTeam {
		t.Fatalf("Visibility = %q, want team", source.Visibility)
	}
	if len(source.RepositoryAllowlist) != 2 || source.RepositoryAllowlist[0] != "acme/*" || source.RepositoryAllowlist[1] != "acme/api" {
		t.Fatalf("RepositoryAllowlist = %#v", source.RepositoryAllowlist)
	}
}

func TestNormalizeGitWebhookSourceInputSupportsWorkspaceVisibility(t *testing.T) {
	source, err := normalizeGitWebhookSourceInput(gitWebhookSourceInput{
		ID:                  "gitlab-shared",
		Provider:            "gitlab",
		Visibility:          "workspace-shared",
		AuthMode:            "none",
		RepositoryAllowlist: []string{"acme/*"},
	}, "")
	if err != nil {
		t.Fatalf("normalizeGitWebhookSourceInput() error = %v", err)
	}
	if source.Visibility != gitWebhookSourceVisibilityWorkspace {
		t.Fatalf("Visibility = %q, want workspace", source.Visibility)
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

func TestNormalizeGitWebhookSourceInputAllowsGeneratedCredentialForCreate(t *testing.T) {
	input := gitWebhookSourceInput{
		ID:                  "gitlab",
		Provider:            "gitlab",
		AuthMode:            "hmac",
		RepositoryAllowlist: []string{"acme/*"},
	}
	if _, err := normalizeGitWebhookSourceInput(input, ""); err == nil {
		t.Fatal("normalizeGitWebhookSourceInput() error = nil, want missing credential error")
	}
	source, err := normalizeGitWebhookSourceInputWithOptions(
		input,
		"",
		gitWebhookSourceNormalizeOptions{AllowGeneratedCredential: true},
	)
	if err != nil {
		t.Fatalf("normalizeGitWebhookSourceInputWithOptions() error = %v", err)
	}
	if source.CredentialRef != "" || source.AuthMode != "hmac" {
		t.Fatalf("source = %#v, want generated credential placeholder", source)
	}
}

func TestPrepareGitWebhookSourceCredentialGeneratesTeamCredential(t *testing.T) {
	ctx := context.Background()
	app, store, service := newGitWebhookCredentialTestApp(t)
	source := gitWebhookSourceRecord{
		ID:       "GitLab-Main",
		AuthMode: "hmac",
		Provider: "gitlab",
		TeamPath: "platform",
	}
	prepared, generated, credentialID, err := app.prepareGitWebhookSourceCredential(ctx, source, "alice")
	if err != nil {
		t.Fatalf("prepareGitWebhookSourceCredential() error = %v", err)
	}
	if prepared.CredentialRef != "credential://team/platform/webhooks/gitlab-main" {
		t.Fatalf("CredentialRef = %q, want generated team reference", prepared.CredentialRef)
	}
	if generated == nil || generated.Reference != prepared.CredentialRef || generated.Value == "" {
		t.Fatalf("generated = %#v, want one-time credential value", generated)
	}
	if len(generated.Value) < len("whsec_") || generated.Value[:len("whsec_")] != "whsec_" {
		t.Fatalf("generated value = %q, want GitLab standard webhook secret prefix", generated.Value)
	}
	if credentialID == nil {
		t.Fatal("credentialID = nil, want created credential ID")
	}
	ref, _ := credentials.ParseReference(prepared.CredentialRef)
	record, err := store.GetCredentialByReference(ctx, ref)
	if err != nil {
		t.Fatalf("stored generated credential: %v", err)
	}
	if record.Kind != gitWebhookSecretCredentialKind || record.Status != credentials.StatusActive {
		t.Fatalf("generated credential = %#v, want active webhook_secret", record)
	}
	value, err := service.Resolve(ctx, ref, credentials.Purpose{
		ConsumerService: "nopsai",
		Operation:       "git_webhook_test",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if value.Text() != generated.Value {
		t.Fatalf("resolved value = %q, want generated value", value.Text())
	}

	preparedAgain, generatedAgain, credentialIDAgain, err := app.prepareGitWebhookSourceCredential(ctx, prepared, "alice")
	if err != nil {
		t.Fatalf("second prepareGitWebhookSourceCredential() error = %v", err)
	}
	if preparedAgain.CredentialRef != prepared.CredentialRef || generatedAgain != nil || credentialIDAgain != nil {
		t.Fatalf("second prepare = (%#v, %#v, %#v), want existing credential reuse", preparedAgain, generatedAgain, credentialIDAgain)
	}
}

func TestPrepareGitWebhookSourceCredentialUsesExistingCredential(t *testing.T) {
	ctx := context.Background()
	app, _, service := newGitWebhookCredentialTestApp(t)
	ref, _ := credentials.ParseReference("credential://system/webhooks/existing")
	existing, err := service.Create(ctx, createCredentialInput{
		Reference:   ref,
		Kind:        gitWebhookSecretCredentialKind,
		Description: "existing webhook secret",
		Value:       []byte("existing-secret"),
		Actor:       "admin",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	prepared, generated, credentialID, err := app.prepareGitWebhookSourceCredential(ctx, gitWebhookSourceRecord{
		ID:            "existing",
		AuthMode:      "static_token",
		CredentialRef: ref.String(),
	}, "admin")
	if err != nil {
		t.Fatalf("prepareGitWebhookSourceCredential() error = %v", err)
	}
	if prepared.CredentialRef != ref.String() || generated != nil || credentialID != nil {
		t.Fatalf("prepare = (%#v, %#v, %#v), want existing reference reuse", prepared, generated, credentialID)
	}
	stillExisting, err := app.credentialStore.GetCredentialByReference(ctx, ref)
	if err != nil {
		t.Fatalf("GetCredentialByReference() error = %v", err)
	}
	if stillExisting.ID != existing.ID || stillExisting.ActiveVersion != existing.ActiveVersion {
		t.Fatalf("credential changed from %#v to %#v", existing, stillExisting)
	}
}

func TestEnsureGitWebhookCredentialAllowedEnforcesCredentialScope(t *testing.T) {
	app, _, _ := newGitWebhookCredentialTestApp(t)
	app.aaaLocal = stubAAAAuthorizer{
		checkFn: func(_ context.Context, subject aaamodel.Subject, action string, resource aaamodel.ResourceRef, _ map[string]any) (aaamodel.Decision, error) {
			allowed := subject.Sub == "admin" && action == "iam.admin" && resource.Type == "iam" && resource.ID == "admin"
			allowed = allowed || action == "credential.create" && resource.Type == grantResourceTeam && resource.ID == "platform"
			return aaamodel.Decision{Allowed: allowed}, nil
		},
	}

	req := credentialTestRequest("POST", "/v1/git-webhook-sources", "alice", nil)
	subject, _ := app.currentAAASubject(req)
	err := app.ensureGitWebhookCredentialAllowed(req, subject, gitWebhookSourceRecord{
		ID:            "system",
		AuthMode:      "hmac",
		CredentialRef: "credential://system/webhooks/source",
	})
	if err != errGitWebhookCredentialForbidden {
		t.Fatalf("system credential err = %v, want forbidden", err)
	}

	err = app.ensureGitWebhookCredentialAllowed(req, subject, gitWebhookSourceRecord{
		ID:       "team",
		AuthMode: "hmac",
		TeamPath: "platform",
	})
	if err != nil {
		t.Fatalf("team generated credential err = %v, want nil", err)
	}

	req = credentialTestRequest("POST", "/v1/git-webhook-sources", "admin", nil)
	subject, _ = app.currentAAASubject(req)
	err = app.ensureGitWebhookCredentialAllowed(req, subject, gitWebhookSourceRecord{
		ID:            "system",
		AuthMode:      "hmac",
		CredentialRef: "credential://system/webhooks/source",
	})
	if err != nil {
		t.Fatalf("admin system credential err = %v, want nil", err)
	}

	err = app.ensureGitWebhookCredentialAllowed(req, subject, gitWebhookSourceRecord{
		ID:            "mismatch",
		AuthMode:      "hmac",
		TeamPath:      "platform",
		CredentialRef: "credential://team/other/webhooks/source",
	})
	if err == nil {
		t.Fatal("admin mismatched team credential err = nil, want validation error")
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

func newGitWebhookCredentialTestApp(t *testing.T) (*App, *memoryCredentialStore, *credentialService) {
	t.Helper()
	store := newMemoryCredentialStore()
	codec, err := credentials.NewEnvelopeCodec("01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("NewEnvelopeCodec() error = %v", err)
	}
	service, err := newCredentialService(store, codec, nil)
	if err != nil {
		t.Fatalf("newCredentialService() error = %v", err)
	}
	return &App{
		credentialStore: store,
		credentials:     service,
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ context.Context, _ aaamodel.Subject, action string, resource aaamodel.ResourceRef, _ map[string]any) (aaamodel.Decision, error) {
				allowed := action == "iam.admin" && resource.Type == "iam" && resource.ID == "admin"
				allowed = allowed || action == "credential.create" && resource.Type == grantResourceTeam
				allowed = allowed || action == "credential.use" && resource.Type == grantResourceTeam
				return aaamodel.Decision{Allowed: allowed}, nil
			},
		},
	}, store, service
}

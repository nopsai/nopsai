package nopsai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/config"

	"github.com/google/go-github/v53/github"
)

func TestBuildGitHubAppManifestPointsAtThisInstallation(t *testing.T) {
	cfg := config.Config{PublicURL: "https://nopsai.example.com"}
	manifest := buildGitHubAppManifest(cfg, "NopsAI nopsai.example.com", "https://nopsai.example.com")

	if manifest.HookAttributes.URL != "https://nopsai.example.com/webhook" {
		t.Fatalf("hook url = %q", manifest.HookAttributes.URL)
	}
	if manifest.RedirectURL != "https://nopsai.example.com"+gitHubAppRegisterCallbackPath {
		t.Fatalf("redirect url = %q", manifest.RedirectURL)
	}
	if manifest.SetupURL != "https://nopsai.example.com"+gitHubAppInstallCallbackPath {
		t.Fatalf("setup url = %q", manifest.SetupURL)
	}
	if manifest.Public || manifest.RequestOAuthOnInstall {
		t.Fatalf("generated App must stay private and App-authenticated: %#v", manifest)
	}
	for _, event := range []string{"push", "pull_request", "check_run", "check_suite", "installation"} {
		if !containsFold(manifest.DefaultEvents, event) {
			t.Fatalf("manifest is missing the %q event: %v", event, manifest.DefaultEvents)
		}
	}
	if manifest.DefaultPermissions["contents"] != "write" || manifest.DefaultPermissions["checks"] != "write" {
		t.Fatalf("permissions = %#v", manifest.DefaultPermissions)
	}
}

func TestGitHubAppPublicBaseURLRejectsUnusableValues(t *testing.T) {
	for name, publicURL := range map[string]string{
		"empty":       "",
		"no scheme":   "nopsai.example.com",
		"unsupported": "ftp://nopsai.example.com",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := gitHubAppPublicBaseURL(config.Config{PublicURL: publicURL}); err == nil {
				t.Fatalf("gitHubAppPublicBaseURL(%q) error = nil, want failure", publicURL)
			}
		})
	}
}

func TestGitHubAppManifestPostURLTargets(t *testing.T) {
	personal, err := gitHubAppManifestPostURL(gitHubAppRegistrationTargetPersonal, "", "state-value")
	if err != nil {
		t.Fatalf("personal target error = %v", err)
	}
	if personal != "https://github.com/settings/apps/new?state=state-value" {
		t.Fatalf("personal url = %q", personal)
	}

	organization, err := gitHubAppManifestPostURL(gitHubAppRegistrationTargetOrg, "acme", "state-value")
	if err != nil {
		t.Fatalf("organization target error = %v", err)
	}
	if organization != "https://github.com/organizations/acme/settings/apps/new?state=state-value" {
		t.Fatalf("organization url = %q", organization)
	}

	if _, err := gitHubAppManifestPostURL(gitHubAppRegistrationTargetOrg, "  ", "state-value"); err == nil {
		t.Fatal("organization target without an organization was accepted")
	}
}

func TestGitHubAppInstallURLRequiresSlug(t *testing.T) {
	if _, err := gitHubAppInstallURL("", "state-value"); err == nil {
		t.Fatal("gitHubAppInstallURL() accepted an unknown slug")
	}
	installURL, err := gitHubAppInstallURL("NopsAI-Example", "state-value")
	if err != nil {
		t.Fatalf("gitHubAppInstallURL() error = %v", err)
	}
	if installURL != "https://github.com/apps/nopsai-example/installations/new?state=state-value" {
		t.Fatalf("install url = %q", installURL)
	}
}

// The flow must fail before an App is created on GitHub when GitHub could never
// reach the webhook and callback URLs it would be given.
func TestHandleStartGitHubAppRegistrationRequiresPublicURL(t *testing.T) {
	app := App{cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/git-apps/github/register/start", strings.NewReader(`{"target":"personal"}`))
	rec := httptest.NewRecorder()

	app.handleStartGitHubAppRegistration(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusPreconditionFailed, rec.Body.String())
	}
}

func TestHandleStartGitHubAppRegistrationRejectsUnknownTarget(t *testing.T) {
	app := App{cfg: &config.Config{PublicURL: "https://nopsai.example.com"}}
	req := httptest.NewRequest(http.MethodPost, "/v1/git-apps/github/register/start", strings.NewReader(`{"target":"enterprise"}`))
	rec := httptest.NewRecorder()

	app.handleStartGitHubAppRegistration(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleStartGitHubAppInstallRequiresRegisteredApp(t *testing.T) {
	app := App{cfg: &config.Config{PublicURL: "https://nopsai.example.com"}}
	req := httptest.NewRequest(http.MethodPost, "/v1/git-apps/github/install/start", nil)
	rec := httptest.NewRecorder()

	app.handleStartGitHubAppInstall(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusPreconditionFailed, rec.Body.String())
	}
}

func TestHandleGitHubAppInstallCallbackRedirectsWhenInstallationIsMissing(t *testing.T) {
	app := App{cfg: &config.Config{PublicURL: "https://nopsai.example.com"}}
	req := httptest.NewRequest(http.MethodGet, "/v1/git-apps/github/install/callback", nil)
	rec := httptest.NewRecorder()

	app.handleGitHubAppInstallCallback(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "https://nopsai.example.com/system/git-apps?") ||
		!strings.Contains(location, "github_app_error=") {
		t.Fatalf("location = %q", location)
	}
}

func TestGitHubInstallationLifecycleRegistersAndRemovesInstallations(t *testing.T) {
	app := App{cfg: &config.Config{}}
	ctx := context.Background()

	created := &github.InstallationEvent{
		Action: github.String("created"),
		Installation: &github.Installation{
			ID:      github.Int64(4321),
			Account: &github.User{Login: github.String("Acme"), Type: github.String("Organization")},
		},
	}
	handled, err := app.handleGitHubInstallationLifecycleEvent(ctx, created)
	if !handled || err != nil {
		t.Fatalf("handled = %v, err = %v", handled, err)
	}
	installations := app.getConfigSnapshot().GitHubInstallations
	if len(installations) != 1 ||
		installations[0].InstallationID != "4321" ||
		installations[0].AccountLogin != "Acme" ||
		installations[0].AccountType != "organization" ||
		!config.GitHubInstallationEnabled(installations[0]) {
		t.Fatalf("installations = %#v", installations)
	}

	suspended := &github.InstallationEvent{
		Action:       github.String("suspend"),
		Installation: created.Installation,
	}
	if _, err := app.handleGitHubInstallationLifecycleEvent(ctx, suspended); err != nil {
		t.Fatalf("suspend error = %v", err)
	}
	if config.GitHubInstallationEnabled(app.getConfigSnapshot().GitHubInstallations[0]) {
		t.Fatal("suspended installation stayed enabled")
	}

	deleted := &github.InstallationEvent{
		Action:       github.String("deleted"),
		Installation: created.Installation,
	}
	if _, err := app.handleGitHubInstallationLifecycleEvent(ctx, deleted); err != nil {
		t.Fatalf("delete error = %v", err)
	}
	if len(app.getConfigSnapshot().GitHubInstallations) != 0 {
		t.Fatalf("installations after delete = %#v", app.getConfigSnapshot().GitHubInstallations)
	}
}

// A repository-selection change must never silently re-enable an installation an
// operator disabled.
func TestGitHubInstallationLifecyclePreservesOperatorDisabledState(t *testing.T) {
	app := App{cfg: &config.Config{GitHubInstallations: []config.GitHubInstallationConfig{{
		InstallationID: "4321",
		AccountLogin:   "acme",
		AccountType:    "organization",
		Enabled:        boolPtr(false),
	}}}}

	event := &github.InstallationRepositoriesEvent{
		Action: github.String("added"),
		Installation: &github.Installation{
			ID:      github.Int64(4321),
			Account: &github.User{Login: github.String("acme"), Type: github.String("Organization")},
		},
	}
	if _, err := app.handleGitHubInstallationLifecycleEvent(context.Background(), event); err != nil {
		t.Fatalf("installation_repositories error = %v", err)
	}
	installations := app.getConfigSnapshot().GitHubInstallations
	if len(installations) != 1 || config.GitHubInstallationEnabled(installations[0]) {
		t.Fatalf("installations = %#v", installations)
	}
}

func TestGitHubInstallationLifecycleIgnoresUnrelatedEvents(t *testing.T) {
	app := App{cfg: &config.Config{}}
	handled, err := app.handleGitHubInstallationLifecycleEvent(context.Background(), &github.PushEvent{})
	if handled || err != nil {
		t.Fatalf("handled = %v, err = %v", handled, err)
	}
}

func TestGitHubAppResourceExposesAppSlug(t *testing.T) {
	app := App{cfg: &config.Config{
		GitHubAppID:   "123456",
		GitHubAppSlug: "nopsai-example",
		PublicURL:     "https://nopsai.example.com",
	}}
	rec := httptest.NewRecorder()

	app.handleGetGitHubApp(rec, httptest.NewRequest(http.MethodGet, "/v1/git-apps/github", nil))

	var resource githubAppResource
	if err := json.Unmarshal(rec.Body.Bytes(), &resource); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resource.AppSlug != "nopsai-example" {
		t.Fatalf("app_slug = %q", resource.AppSlug)
	}
	if resource.WebhookEndpoint != "https://nopsai.example.com/webhook" {
		t.Fatalf("webhook_endpoint = %q", resource.WebhookEndpoint)
	}
}

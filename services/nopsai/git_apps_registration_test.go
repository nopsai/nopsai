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

// The webhook URL is fetched by GitHub, while the redirect and setup URLs are
// only opened in the operator's browser, so a tunnel that fronts git-bot alone
// is a complete setup.
func TestBuildGitHubAppManifestSeparatesWebhookFromBrowserCallbacks(t *testing.T) {
	manifest := buildGitHubAppManifest(
		"NopsAI localhost",
		"http://localhost:8080",
		"https://live-gecko-national.ngrok-free.app/webhook",
	)

	if manifest.HookAttributes.URL != "https://live-gecko-national.ngrok-free.app/webhook" {
		t.Fatalf("hook url = %q", manifest.HookAttributes.URL)
	}
	if manifest.RedirectURL != "http://localhost:8080"+gitHubAppRegisterCallbackPath {
		t.Fatalf("redirect url = %q", manifest.RedirectURL)
	}
	if manifest.SetupURL != "http://localhost:8080"+gitHubAppInstallCallbackPath {
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

func TestNormalizeGitHubWebhookURL(t *testing.T) {
	for name, testCase := range map[string]struct {
		raw     string
		want    string
		wantErr bool
	}{
		"tunnel base gets the webhook path": {
			raw:  "https://live-gecko-national.ngrok-free.app",
			want: "https://live-gecko-national.ngrok-free.app/webhook",
		},
		"explicit endpoint is kept": {
			raw:  "https://hooks.example.com/git-bot/webhook/",
			want: "https://hooks.example.com/git-bot/webhook",
		},
		"empty is rejected":    {raw: "  ", wantErr: true},
		"relative is rejected": {raw: "/webhook", wantErr: true},
		"non-http is rejected": {raw: "ftp://hooks.example.com", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := normalizeGitHubWebhookURL(testCase.raw)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("normalizeGitHubWebhookURL(%q) error = nil, want failure", testCase.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeGitHubWebhookURL(%q) error = %v", testCase.raw, err)
			}
			if got != testCase.want {
				t.Fatalf("normalizeGitHubWebhookURL(%q) = %q, want %q", testCase.raw, got, testCase.want)
			}
		})
	}
}

func TestEffectiveGitHubWebhookURLPrefersTheStoredAddress(t *testing.T) {
	stored := config.Config{
		GitHubWebhookURL: "https://live-gecko-national.ngrok-free.app/webhook",
		PublicURL:        "http://localhost:8080",
	}
	if got := effectiveGitHubWebhookURL(stored); got != "https://live-gecko-national.ngrok-free.app/webhook" {
		t.Fatalf("effectiveGitHubWebhookURL() = %q", got)
	}
	derived := config.Config{PublicURL: "https://nopsai.example.com"}
	if got := effectiveGitHubWebhookURL(derived); got != "https://nopsai.example.com/webhook" {
		t.Fatalf("derived effectiveGitHubWebhookURL() = %q", got)
	}
	if got := effectiveGitHubWebhookURL(config.Config{}); got != "" {
		t.Fatalf("unconfigured effectiveGitHubWebhookURL() = %q, want empty", got)
	}
}

// The browser origin is the accurate answer for where to send the operator back
// to, and it keeps local installs working without a public NopsAI address.
func TestGitHubAppCallbackBaseURLUsesTheBrowserOrigin(t *testing.T) {
	base, err := gitHubAppCallbackBaseURL(config.Config{}, "http://localhost:8080", "localhost:8080")
	if err != nil {
		t.Fatalf("gitHubAppCallbackBaseURL() error = %v", err)
	}
	if base != "http://localhost:8080" {
		t.Fatalf("callback base = %q", base)
	}
}

func TestGitHubAppCallbackBaseURLRejectsForeignOrigins(t *testing.T) {
	cfg := config.Config{CORSAllowedOrigins: []string{"https://ui.example.com"}}

	allowed, err := gitHubAppCallbackBaseURL(cfg, "https://ui.example.com", "api.example.com")
	if err != nil || allowed != "https://ui.example.com" {
		t.Fatalf("allowed origin = %q, err = %v", allowed, err)
	}

	if _, err := gitHubAppCallbackBaseURL(cfg, "https://attacker.example", "api.example.com"); err == nil {
		t.Fatal("gitHubAppCallbackBaseURL() accepted an origin that is not this installation")
	}
}

func TestGitHubAppCallbackBaseURLFallsBackToPublicURL(t *testing.T) {
	cfg := config.Config{PublicURL: "https://nopsai.example.com/"}
	base, err := gitHubAppCallbackBaseURL(cfg, "", "")
	if err != nil {
		t.Fatalf("gitHubAppCallbackBaseURL() error = %v", err)
	}
	if base != "https://nopsai.example.com" {
		t.Fatalf("callback base = %q", base)
	}
	if _, err := gitHubAppCallbackBaseURL(config.Config{}, "", ""); err == nil {
		t.Fatal("gitHubAppCallbackBaseURL() error = nil with no origin and no public_url")
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

// The flow must fail before an App is created on GitHub when GitHub would have
// no address to deliver webhooks to.
func TestHandleStartGitHubAppRegistrationRequiresAWebhookURL(t *testing.T) {
	app := App{cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/git-apps/github/register/start", strings.NewReader(`{"target":"personal"}`))
	req.Header.Set("Origin", "http://localhost:8080")
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()

	app.handleStartGitHubAppRegistration(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusPreconditionFailed, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "webhook URL") {
		t.Fatalf("body = %q, want the webhook URL to be named", rec.Body.String())
	}
}

// A local NopsAI reached through a tunnel that only fronts git-bot is a
// complete, supported setup.
func TestHandleStartGitHubAppRegistrationAcceptsATunnelWebhookWithoutPublicURL(t *testing.T) {
	app := App{cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/git-apps/github/register/start", strings.NewReader(
		`{"target":"organization","organization":"acme","webhook_url":"https://live-gecko-national.ngrok-free.app"}`,
	))
	req.Header.Set("Origin", "http://localhost:8080")
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()

	app.handleStartGitHubAppRegistration(rec, req)

	// Persisting the state needs a database; the precondition checks must pass
	// before that point.
	if rec.Code == http.StatusPreconditionFailed || rec.Code == http.StatusBadRequest {
		t.Fatalf("status = %d, want the tunnel webhook URL to be accepted: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleStartGitHubAppRegistrationRejectsUnknownTarget(t *testing.T) {
	app := App{cfg: &config.Config{PublicURL: "https://nopsai.example.com"}}
	req := httptest.NewRequest(http.MethodPost, "/v1/git-apps/github/register/start", strings.NewReader(`{"target":"enterprise"}`))
	req.Header.Set("Origin", "https://nopsai.example.com")
	rec := httptest.NewRecorder()

	app.handleStartGitHubAppRegistration(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleStartGitHubAppInstallRequiresRegisteredApp(t *testing.T) {
	app := App{cfg: &config.Config{PublicURL: "https://nopsai.example.com"}}
	req := httptest.NewRequest(http.MethodPost, "/v1/git-apps/github/install/start", nil)
	req.Header.Set("Origin", "https://nopsai.example.com")
	rec := httptest.NewRecorder()

	app.handleStartGitHubAppInstall(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusPreconditionFailed, rec.Body.String())
	}
}

// Without a usable state the callback cannot know the address the flow started
// from, so it stays relative. public_url must not be used here: deployments
// where it names the public git-bot ingress would send the operator to a host
// that serves no UI.
func TestHandleGitHubAppInstallCallbackRedirectsRelativeWhenTheFlowOriginIsUnknown(t *testing.T) {
	app := App{cfg: &config.Config{PublicURL: "https://git-bot.example.com"}}
	req := httptest.NewRequest(http.MethodGet, "/v1/git-apps/github/install/callback", nil)
	rec := httptest.NewRecorder()

	app.handleGitHubAppInstallCallback(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "/system/git-apps?") || !strings.Contains(location, "github_app_error=") {
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

func TestGitHubAppResourceExposesAppSlugAndWebhookURL(t *testing.T) {
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

	tunnelled := App{cfg: &config.Config{
		GitHubAppID:      "123456",
		GitHubWebhookURL: "https://live-gecko-national.ngrok-free.app/webhook",
	}}
	rec = httptest.NewRecorder()
	tunnelled.handleGetGitHubApp(rec, httptest.NewRequest(http.MethodGet, "/v1/git-apps/github", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &resource); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resource.WebhookURL != "https://live-gecko-national.ngrok-free.app/webhook" ||
		resource.WebhookEndpoint != resource.WebhookURL {
		t.Fatalf("webhook_url = %q, webhook_endpoint = %q", resource.WebhookURL, resource.WebhookEndpoint)
	}
}

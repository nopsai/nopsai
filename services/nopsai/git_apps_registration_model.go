package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"nopsai/config"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v53/github"
)

// GitHub App registration turns the manual "create the App on GitHub, then copy
// the App ID and credential references into NopsAI" procedure into a redirect
// flow. NopsAI hands GitHub an App manifest, GitHub creates the App and returns
// a one-time conversion code, and NopsAI exchanges that code for the App ID,
// private key, and webhook secret.
const (
	gitHubAppRegistrationStateTTL       = 15 * time.Minute
	gitHubAppPrivateKeyCredentialRef    = "credential://system/github/app-private-key"
	gitHubAppWebhookCredentialRef       = "credential://system/github/webhook-secret"
	gitHubAppRegisterCallbackPath       = "/v1/git-apps/github/register/callback"
	gitHubAppInstallCallbackPath        = "/v1/git-apps/github/install/callback"
	gitHubAppRegistrationTargetPersonal = "personal"
	gitHubAppRegistrationTargetOrg      = "organization"
	gitHubWebURL                        = "https://github.com"
	gitHubAPIURL                        = "https://api.github.com"
)

// gitHubAppManifestEvents and gitHubAppManifestPermissions are the single source
// for what the generated App may do. The same values are documented in
// doc/git-apps.md for operators who register an App by hand.
var gitHubAppManifestEvents = []string{
	"push",
	"pull_request",
	"check_run",
	"check_suite",
	"installation",
	"installation_repositories",
}

var gitHubAppManifestPermissions = map[string]string{
	"contents":      "write",
	"metadata":      "read",
	"pull_requests": "read",
	"checks":        "write",
}

type gitHubAppManifest struct {
	Name                  string                `json:"name"`
	URL                   string                `json:"url"`
	HookAttributes        gitHubAppManifestHook `json:"hook_attributes"`
	RedirectURL           string                `json:"redirect_url"`
	SetupURL              string                `json:"setup_url"`
	SetupOnUpdate         bool                  `json:"setup_on_update"`
	Public                bool                  `json:"public"`
	DefaultEvents         []string              `json:"default_events"`
	DefaultPermissions    map[string]string     `json:"default_permissions"`
	RequestOAuthOnInstall bool                  `json:"request_oauth_on_install"`
}

type gitHubAppManifestHook struct {
	URL    string `json:"url"`
	Active bool   `json:"active"`
}

// gitHubAppManifestConversion is the subset of the manifest conversion response
// NopsAI keeps. client_id and client_secret are deliberately ignored: NopsAI
// authenticates as the App, never as a GitHub OAuth client.
type gitHubAppManifestConversion struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	HTMLURL       string `json:"html_url"`
	PEM           string `json:"pem"`
	WebhookSecret string `json:"webhook_secret"`
	Owner         struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"owner"`
}

func normalizeGitHubAppSlug(slug string) string {
	return strings.Trim(strings.TrimSpace(strings.ToLower(slug)), "/")
}

func normalizeGitHubAppRegistrationTarget(target string) string {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "", gitHubAppRegistrationTargetPersonal, "user":
		return gitHubAppRegistrationTargetPersonal
	case gitHubAppRegistrationTargetOrg, "org":
		return gitHubAppRegistrationTargetOrg
	default:
		return ""
	}
}

// gitHubAppPublicBaseURL is the externally reachable NopsAI base URL. GitHub has
// to reach the webhook, redirect, and setup URLs built from it, so an empty or
// unparsable value fails the flow before an App is created on GitHub.
func gitHubAppPublicBaseURL(cfg config.Config) (string, error) {
	raw := strings.TrimRight(strings.TrimSpace(cfg.PublicURL), "/")
	if raw == "" {
		return "", fmt.Errorf("public_url is not configured; set it in System > Config before connecting GitHub")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("public_url %q must be an absolute URL such as https://nopsai.example.com", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("public_url %q must use http or https", raw)
	}
	return raw, nil
}

func buildGitHubAppManifest(cfg config.Config, appName, baseURL string) gitHubAppManifest {
	return gitHubAppManifest{
		Name:                  appName,
		URL:                   baseURL,
		HookAttributes:        gitHubAppManifestHook{URL: baseURL + "/webhook", Active: true},
		RedirectURL:           baseURL + gitHubAppRegisterCallbackPath,
		SetupURL:              baseURL + gitHubAppInstallCallbackPath,
		SetupOnUpdate:         true,
		Public:                false,
		DefaultEvents:         append([]string(nil), gitHubAppManifestEvents...),
		DefaultPermissions:    gitHubAppManifestPermissions,
		RequestOAuthOnInstall: false,
	}
}

// defaultGitHubAppName keeps generated App names unique per installation.
// GitHub rejects a manifest whose name is already taken, and the operator can
// override it from the UI.
func defaultGitHubAppName(cfg config.Config) string {
	host := ""
	if parsed, err := url.Parse(strings.TrimSpace(cfg.PublicURL)); err == nil {
		host = strings.TrimSpace(parsed.Hostname())
	}
	if host == "" {
		return "NopsAI"
	}
	return "NopsAI " + host
}

func gitHubAppManifestPostURL(target, organization, state string) (string, error) {
	values := url.Values{}
	values.Set("state", state)
	switch target {
	case gitHubAppRegistrationTargetOrg:
		organization = strings.Trim(strings.TrimSpace(organization), "/")
		if organization == "" {
			return "", fmt.Errorf("organization is required when target is organization")
		}
		return fmt.Sprintf(
			"%s/organizations/%s/settings/apps/new?%s",
			gitHubWebURL,
			url.PathEscape(organization),
			values.Encode(),
		), nil
	case gitHubAppRegistrationTargetPersonal:
		return fmt.Sprintf("%s/settings/apps/new?%s", gitHubWebURL, values.Encode()), nil
	default:
		return "", fmt.Errorf("target must be %q or %q", gitHubAppRegistrationTargetPersonal, gitHubAppRegistrationTargetOrg)
	}
}

func gitHubAppInstallURL(slug, state string) (string, error) {
	slug = normalizeGitHubAppSlug(slug)
	if slug == "" {
		return "", fmt.Errorf("the GitHub App slug is unknown; reconnect the App or add the installation manually")
	}
	values := url.Values{}
	values.Set("state", state)
	return fmt.Sprintf("%s/apps/%s/installations/new?%s", gitHubWebURL, url.PathEscape(slug), values.Encode()), nil
}

// convertGitHubAppManifestCode exchanges the one-time manifest code for App
// credentials. The endpoint is unauthenticated by design: possession of the
// code, which GitHub only hands to our redirect URL, is the proof.
func convertGitHubAppManifestCode(ctx context.Context, httpClient *http.Client, code string) (gitHubAppManifestConversion, error) {
	var conversion gitHubAppManifestConversion
	code = strings.TrimSpace(code)
	if code == "" {
		return conversion, fmt.Errorf("manifest code is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	endpoint := fmt.Sprintf("%s/app-manifests/%s/conversions", gitHubAPIURL, url.PathEscape(code))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return conversion, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := httpClient.Do(req)
	if err != nil {
		return conversion, fmt.Errorf("exchange GitHub App manifest code: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return conversion, fmt.Errorf("GitHub rejected the App manifest exchange with status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, &conversion); err != nil {
		return conversion, fmt.Errorf("decode GitHub App manifest response: %w", err)
	}
	if conversion.ID == 0 || strings.TrimSpace(conversion.PEM) == "" || strings.TrimSpace(conversion.WebhookSecret) == "" {
		return conversion, fmt.Errorf("GitHub returned an incomplete App manifest response")
	}
	return conversion, nil
}

// gitHubAppClient authenticates as the App itself (not an installation), which
// is what the installation lookup after an install redirect needs. git-bot owns
// installation-scoped calls; this stays inside NopsAI so a freshly registered
// App can be completed before git-bot has picked up the new credentials.
func gitHubAppClient(appID int64, privateKeyPEM string) (*github.Client, error) {
	if appID <= 0 {
		return nil, fmt.Errorf("GitHub App ID is not configured")
	}
	transport, err := ghinstallation.NewAppsTransport(http.DefaultTransport, appID, []byte(privateKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("build GitHub App transport: %w", err)
	}
	return github.NewClient(&http.Client{Transport: transport, Timeout: 20 * time.Second}), nil
}

func parseGitHubAppID(raw string) (int64, error) {
	appID, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || appID <= 0 {
		return 0, fmt.Errorf("GitHub App ID must be a positive integer")
	}
	return appID, nil
}

func gitHubInstallationConfigFromAPI(installation *github.Installation) config.GitHubInstallationConfig {
	enabled := true
	record := config.GitHubInstallationConfig{
		InstallationID: strconv.FormatInt(installation.GetID(), 10),
		Enabled:        &enabled,
	}
	if account := installation.GetAccount(); account != nil {
		record.AccountLogin = strings.TrimSpace(account.GetLogin())
		record.AccountType = config.NormalizeGitHubAccountType(account.GetType())
	}
	return record
}

// upsertGitHubInstallationConfig replaces an installation with the same ID and
// keeps the remaining records untouched, so a re-install never duplicates a row.
func upsertGitHubInstallationConfig(
	installations []config.GitHubInstallationConfig,
	next config.GitHubInstallationConfig,
) []config.GitHubInstallationConfig {
	next.InstallationID = strings.TrimSpace(next.InstallationID)
	for idx, current := range installations {
		if strings.TrimSpace(current.InstallationID) != next.InstallationID {
			continue
		}
		installations[idx] = mergeGitHubInstallationRuntimeMetadata(next, current)
		return installations
	}
	return append(installations, next)
}

func removeGitHubInstallationConfig(
	installations []config.GitHubInstallationConfig,
	installationID string,
) ([]config.GitHubInstallationConfig, bool) {
	installationID = strings.TrimSpace(installationID)
	remaining := make([]config.GitHubInstallationConfig, 0, len(installations))
	removed := false
	for _, current := range installations {
		if strings.TrimSpace(current.InstallationID) == installationID {
			removed = true
			continue
		}
		remaining = append(remaining, current)
	}
	return remaining, removed
}

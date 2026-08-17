package nopsai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/serviceauth"
	"nopsai/services/nopsai/internal/credentials"

	"github.com/google/go-github/v53/github"
	"github.com/rs/zerolog/log"
)

type gitHubAppRegistrationStartRequest struct {
	Target       string `json:"target"`
	Organization string `json:"organization"`
	AppName      string `json:"app_name"`
}

type gitHubAppRegistrationStartResponse struct {
	State           string `json:"state"`
	PostURL         string `json:"post_url"`
	Manifest        string `json:"manifest"`
	AppName         string `json:"app_name"`
	WebhookEndpoint string `json:"webhook_endpoint"`
	ExpiresAt       string `json:"expires_at"`
}

type gitHubAppInstallStartResponse struct {
	State      string `json:"state"`
	InstallURL string `json:"install_url"`
	ExpiresAt  string `json:"expires_at"`
}

// handleStartGitHubAppRegistration prepares an App manifest and a single-use
// state. The browser posts the manifest to GitHub as a form, because GitHub only
// accepts manifests through a form submission from the operator's session.
func (a *App) handleStartGitHubAppRegistration(w http.ResponseWriter, r *http.Request) {
	var req gitHubAppRegistrationStartRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	target := normalizeGitHubAppRegistrationTarget(req.Target)
	if target == "" {
		http.Error(w, "target must be personal or organization", http.StatusBadRequest)
		return
	}
	organization := strings.Trim(strings.TrimSpace(req.Organization), "/")
	if target == gitHubAppRegistrationTargetOrg && organization == "" {
		http.Error(w, "organization is required when target is organization", http.StatusBadRequest)
		return
	}

	cfg := a.getConfigSnapshot()
	baseURL, err := gitHubAppPublicBaseURL(cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusPreconditionFailed)
		return
	}
	appName := strings.TrimSpace(req.AppName)
	if appName == "" {
		appName = defaultGitHubAppName(cfg)
	}

	state, err := generateGitHubAppRegistrationState()
	if err != nil {
		http.Error(w, "failed to create registration state", http.StatusInternalServerError)
		return
	}
	// Consumed and long-expired states are only useful for replay protection
	// while they can still be presented.
	if err := purgeExpiredGitHubAppRegistrationStates(r.Context(), a.db); err != nil {
		log.Debug().Err(err).Msg("Failed to purge expired GitHub App registration states")
	}
	expiresAt := time.Now().Add(gitHubAppRegistrationStateTTL)
	if err := createGitHubAppRegistrationState(r.Context(), a.db, state, gitHubAppRegistrationState{
		Flow:         "register",
		Target:       target,
		Organization: organization,
		AppName:      appName,
		Actor:        credentialActorFromContext(r.Context()),
		ExpiresAt:    expiresAt,
	}); err != nil {
		http.Error(w, "failed to persist registration state", http.StatusInternalServerError)
		return
	}

	postURL, err := gitHubAppManifestPostURL(target, organization, state)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	manifest, err := json.Marshal(buildGitHubAppManifest(cfg, appName, baseURL))
	if err != nil {
		http.Error(w, "failed to build GitHub App manifest", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, gitHubAppRegistrationStartResponse{
		State:           state,
		PostURL:         postURL,
		Manifest:        string(manifest),
		AppName:         appName,
		WebhookEndpoint: baseURL + "/webhook",
		ExpiresAt:       expiresAt.UTC().Format(time.RFC3339),
	})
}

// handleGitHubAppRegistrationCallback receives GitHub's redirect after the App
// is created. It is reachable without a bearer token, so the single-use state
// row created by an authorized start request is the only accepted proof.
func (a *App) handleGitHubAppRegistrationCallback(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		a.redirectGitHubAppResult(w, r, "", "GitHub did not return a registration code")
		return
	}
	record, err := consumeGitHubAppRegistrationState(r.Context(), a.db, "register", state)
	if err != nil {
		a.redirectGitHubAppResult(w, r, "", "The GitHub App registration link expired or was already used")
		return
	}

	conversion, err := convertGitHubAppManifestCode(r.Context(), nil, code)
	if err != nil {
		log.Error().Err(err).Msg("Failed to convert GitHub App manifest code")
		a.redirectGitHubAppResult(w, r, "", err.Error())
		return
	}

	actor := strings.TrimSpace(record.Actor)
	if actor == "" {
		actor = "github-app-registration"
	}
	if err := a.storeGitHubAppRegistrationCredentials(r.Context(), conversion, actor); err != nil {
		log.Error().Err(err).Msg("Failed to store GitHub App credentials")
		a.redirectGitHubAppResult(w, r, "", err.Error())
		return
	}

	appID := strconv.FormatInt(conversion.ID, 10)
	slug := normalizeGitHubAppSlug(conversion.Slug)
	cfg, err := a.applySystemConfig(systemConfigPayload{
		GitHubAppID:         stringPtr(appID),
		GitHubAppSlug:       stringPtr(slug),
		GitHubPrivateKeyRef: stringPtr(gitHubAppPrivateKeyCredentialRef),
		GitHubWebhookRef:    stringPtr(gitHubAppWebhookCredentialRef),
	})
	if err != nil {
		a.redirectGitHubAppResult(w, r, "", err.Error())
		return
	}
	if err := a.persistRuntimeSettingsSnapshot(r.Context(), cfg, "database", nil, "", "", false); err != nil {
		a.redirectGitHubAppResult(w, r, "", "failed to persist GitHub App settings")
		return
	}
	log.Info().
		Str("github_app_id", appID).
		Str("github_app_slug", slug).
		Str("account", conversion.Owner.Login).
		Msg("GitHub App registered from manifest")

	// Send the operator straight into the install step: an App without an
	// installation cannot see a single repository yet.
	_, installURL, err := a.startGitHubAppInstall(r.Context(), cfg, actor)
	if err != nil {
		a.redirectGitHubAppResult(w, r, "created", "")
		return
	}
	http.Redirect(w, r, installURL, http.StatusFound)
}

// handleStartGitHubAppInstall hands the UI a one-time install URL for the
// registered App so repository selection happens on GitHub.
func (a *App) handleStartGitHubAppInstall(w http.ResponseWriter, r *http.Request) {
	cfg := a.getConfigSnapshot()
	if _, err := gitHubAppPublicBaseURL(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusPreconditionFailed)
		return
	}
	state, installURL, err := a.startGitHubAppInstall(r.Context(), cfg, credentialActorFromContext(r.Context()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusPreconditionFailed)
		return
	}
	writeJSON(w, http.StatusOK, gitHubAppInstallStartResponse{
		State:      state,
		InstallURL: installURL,
		ExpiresAt:  time.Now().Add(gitHubAppRegistrationStateTTL).UTC().Format(time.RFC3339),
	})
}

func (a *App) startGitHubAppInstall(ctx context.Context, cfg config.Config, actor string) (string, string, error) {
	slug := normalizeGitHubAppSlug(cfg.GitHubAppSlug)
	if slug == "" {
		return "", "", fmt.Errorf("no GitHub App is registered; connect a GitHub App first")
	}
	state, err := generateGitHubAppRegistrationState()
	if err != nil {
		return "", "", fmt.Errorf("failed to create install state")
	}
	if err := createGitHubAppRegistrationState(ctx, a.db, state, gitHubAppRegistrationState{
		Flow:      "install",
		Actor:     strings.TrimSpace(actor),
		ExpiresAt: time.Now().Add(gitHubAppRegistrationStateTTL),
	}); err != nil {
		return "", "", fmt.Errorf("failed to persist install state")
	}
	installURL, err := gitHubAppInstallURL(slug, state)
	if err != nil {
		return "", "", err
	}
	return state, installURL, nil
}

// handleGitHubAppInstallCallback is the App's setup URL. GitHub also calls it
// when someone installs the App directly from GitHub, without a NopsAI-issued
// state, so the installation is always verified against GitHub before it is
// registered rather than trusted from the query string.
func (a *App) handleGitHubAppInstallCallback(w http.ResponseWriter, r *http.Request) {
	rawInstallationID := strings.TrimSpace(r.URL.Query().Get("installation_id"))
	setupAction := strings.TrimSpace(r.URL.Query().Get("setup_action"))
	if state := strings.TrimSpace(r.URL.Query().Get("state")); state != "" {
		// A consumed state is not required for the callback to be trustworthy,
		// but consuming it here keeps issued links single-use.
		if _, err := consumeGitHubAppRegistrationState(r.Context(), a.db, "install", state); err != nil {
			log.Warn().Msg("GitHub App install callback carried an unknown or used state")
		}
	}
	// An organization member who cannot install apps triggers an approval
	// request instead of an installation; there is nothing to register yet.
	if strings.EqualFold(setupAction, "request") {
		a.redirectGitHubAppResult(w, r, "requested", "")
		return
	}
	if rawInstallationID == "" {
		a.redirectGitHubAppResult(w, r, "", "GitHub did not return an installation")
		return
	}
	installationID, err := strconv.ParseInt(rawInstallationID, 10, 64)
	if err != nil || installationID <= 0 {
		a.redirectGitHubAppResult(w, r, "", "GitHub returned an invalid installation id")
		return
	}

	if err := a.registerGitHubAppInstallation(r.Context(), installationID); err != nil {
		log.Error().Err(err).Int64("installation_id", installationID).Msg("Failed to register GitHub App installation")
		a.redirectGitHubAppResult(w, r, "", err.Error())
		return
	}
	a.redirectGitHubAppResult(w, r, "installed", "")
}

// registerGitHubAppInstallation verifies the installation belongs to this App
// and stores it. Verification failures never fall back to trusting the caller.
func (a *App) registerGitHubAppInstallation(ctx context.Context, installationID int64) error {
	cfg := a.getConfigSnapshot()
	client, err := a.gitHubAppAPIClient(ctx, cfg)
	if err != nil {
		return err
	}
	installation, _, err := client.Apps.GetInstallation(ctx, installationID)
	if err != nil {
		return fmt.Errorf("GitHub did not confirm installation %d for this App", installationID)
	}
	record := gitHubInstallationConfigFromAPI(installation)
	installations := upsertGitHubInstallationConfig(
		config.NormalizeGitHubInstallations(cfg.GitHubInstallations, cfg.GitHubInstallID),
		record,
	)
	return a.persistGitHubInstallations(ctx, installations)
}

func (a *App) removeGitHubAppInstallationRecord(ctx context.Context, installationID string) error {
	cfg := a.getConfigSnapshot()
	installations, removed := removeGitHubInstallationConfig(
		config.NormalizeGitHubInstallations(cfg.GitHubInstallations, cfg.GitHubInstallID),
		installationID,
	)
	if !removed {
		return nil
	}
	return a.persistGitHubInstallations(ctx, installations)
}

func (a *App) persistGitHubInstallations(ctx context.Context, installations []config.GitHubInstallationConfig) error {
	cfg, err := a.applySystemConfig(systemConfigPayload{GitHubInstallations: &installations})
	if err != nil {
		return err
	}
	if err := a.persistRuntimeSettingsSnapshot(ctx, cfg, "database", nil, "", "", false); err != nil {
		return fmt.Errorf("failed to persist GitHub App installations")
	}
	return nil
}

// gitHubAppAPIClient authenticates as the App using the stored private key.
func (a *App) gitHubAppAPIClient(ctx context.Context, cfg config.Config) (*github.Client, error) {
	appID, err := parseGitHubAppID(cfg.GitHubAppID)
	if err != nil {
		return nil, err
	}
	privateKey, err := a.resolveCredentialText(ctx, cfg.GitHubPrivateKeyCredentialRef, credentials.Purpose{
		ConsumerService: serviceauth.RoleNopsai,
		Operation:       "github.app_authenticate",
		SubjectType:     "service",
		SubjectID:       serviceauth.RoleNopsai,
	})
	if err != nil || strings.TrimSpace(privateKey) == "" {
		return nil, fmt.Errorf("the GitHub App private key credential is unavailable")
	}
	return gitHubAppClient(appID, privateKey)
}

// storeGitHubAppRegistrationCredentials writes the manifest-issued private key
// and webhook secret into the credential store, rotating them when a previous
// App was already connected.
func (a *App) storeGitHubAppRegistrationCredentials(
	ctx context.Context,
	conversion gitHubAppManifestConversion,
	actor string,
) error {
	if err := a.putGitHubAppCredential(
		ctx,
		gitHubAppPrivateKeyCredentialRef,
		"private_key",
		"GitHub App private key",
		conversion.PEM,
		actor,
	); err != nil {
		return fmt.Errorf("store GitHub App private key: %w", err)
	}
	if err := a.putGitHubAppCredential(
		ctx,
		gitHubAppWebhookCredentialRef,
		"webhook_secret",
		"GitHub App webhook verification secret",
		conversion.WebhookSecret,
		actor,
	); err != nil {
		return fmt.Errorf("store GitHub App webhook secret: %w", err)
	}
	return nil
}

func (a *App) putGitHubAppCredential(
	ctx context.Context,
	rawReference, kind, description, value, actor string,
) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("GitHub returned an empty %s", description)
	}
	if a == nil || a.credentials == nil || a.credentialStore == nil {
		return credentials.ErrUnavailable
	}
	ref, err := credentials.ParseReference(rawReference)
	if err != nil {
		return err
	}
	existing, err := a.credentialStore.GetCredentialByReference(ctx, ref)
	if err != nil {
		if !errors.Is(err, credentials.ErrNotFound) {
			return err
		}
		_, err = a.credentials.Create(ctx, createCredentialInput{
			Reference:   ref,
			Kind:        kind,
			Description: description,
			Value:       []byte(value),
			Actor:       actor,
		})
		return err
	}
	if existing.Kind != kind {
		return fmt.Errorf(
			"credential %s already exists with kind %q; expected kind %q",
			rawReference,
			existing.Kind,
			kind,
		)
	}
	_, err = a.credentials.PutValue(ctx, existing.ID, []byte(value), actor)
	return err
}

// redirectGitHubAppResult returns the operator to the Git Apps page with the
// outcome, so a failed callback never leaves them on a bare API error page.
func (a *App) redirectGitHubAppResult(w http.ResponseWriter, r *http.Request, status, message string) {
	values := url.Values{}
	if strings.TrimSpace(status) != "" {
		values.Set("github_app", status)
	}
	if strings.TrimSpace(message) != "" {
		values.Set("github_app_error", message)
	}
	base := strings.TrimRight(strings.TrimSpace(a.getConfigSnapshot().PublicURL), "/")
	target := base + "/system/git-apps"
	if encoded := values.Encode(); encoded != "" {
		target += "?" + encoded
	}
	http.Redirect(w, r, target, http.StatusFound)
}

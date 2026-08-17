package nopsai

import (
	"context"
	"strconv"
	"strings"

	"nopsai/config"

	"github.com/google/go-github/v53/github"
	"github.com/rs/zerolog/log"
)

// GitHub App installation lifecycle events keep the installation catalog in step
// with GitHub when an account is added, suspended, or removed outside NopsAI.
// git-bot has already verified the webhook signature, so the event payload is
// authoritative and no extra GitHub call is needed.
func (a *App) handleGitHubInstallationLifecycleEvent(ctx context.Context, payload any) (bool, error) {
	switch event := payload.(type) {
	case *github.InstallationEvent:
		return true, a.applyGitHubInstallationLifecycle(ctx, event.GetAction(), event.GetInstallation())
	case *github.InstallationRepositoriesEvent:
		// Repository selection changed; the installation itself stays valid and
		// only its cached repository count is now stale.
		return true, a.applyGitHubInstallationLifecycle(ctx, "updated", event.GetInstallation())
	default:
		return false, nil
	}
}

func (a *App) applyGitHubInstallationLifecycle(
	ctx context.Context,
	action string,
	installation *github.Installation,
) error {
	if installation == nil || installation.GetID() == 0 {
		return nil
	}
	installationID := strconv.FormatInt(installation.GetID(), 10)
	action = strings.ToLower(strings.TrimSpace(action))
	log.Info().
		Str("installation_id", installationID).
		Str("action", action).
		Msg("Processing GitHub App installation event")

	switch action {
	case "deleted":
		return a.removeGitHubAppInstallationRecord(ctx, installationID)
	case "suspend":
		return a.syncGitHubInstallationRecord(ctx, installation, boolPtr(false))
	case "created", "unsuspend":
		return a.syncGitHubInstallationRecord(ctx, installation, boolPtr(true))
	case "new_permissions_accepted", "updated", "added", "removed":
		// Account metadata may have changed, but whether NopsAI uses this
		// installation stays an operator decision.
		return a.syncGitHubInstallationRecord(ctx, installation, nil)
	default:
		return nil
	}
}

// syncGitHubInstallationRecord refreshes the stored account metadata for an
// installation. enabled overrides the stored flag when GitHub reports a
// suspension change; otherwise the existing value is preserved.
func (a *App) syncGitHubInstallationRecord(
	ctx context.Context,
	installation *github.Installation,
	enabled *bool,
) error {
	cfg := a.getConfigSnapshot()
	installations := config.NormalizeGitHubInstallations(cfg.GitHubInstallations, cfg.GitHubInstallID)
	record := gitHubInstallationConfigFromAPI(installation)
	if enabled != nil {
		record.Enabled = enabled
	} else if current, found := findGitHubInstallationConfig(installations, record.InstallationID); found {
		record.Enabled = current.Enabled
	}
	return a.persistGitHubInstallations(ctx, upsertGitHubInstallationConfig(installations, record))
}

func findGitHubInstallationConfig(
	installations []config.GitHubInstallationConfig,
	installationID string,
) (config.GitHubInstallationConfig, bool) {
	installationID = strings.TrimSpace(installationID)
	for _, installation := range installations {
		if strings.TrimSpace(installation.InstallationID) == installationID {
			return installation, true
		}
	}
	return config.GitHubInstallationConfig{}, false
}

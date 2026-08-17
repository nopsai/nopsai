package app

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"nopsai/config"
	"nopsai/pkg/serviceauth"
	"nopsai/services/git-bot/internal/service"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v53/github"
	"github.com/rs/zerolog/log"
)

const (
	gitHubCredentialRefreshInterval = 60 * time.Second
	gitHubInstallationCacheTTL      = 30 * time.Second
)

// githubRuntime owns the GitHub App credentials git-bot runs with. NopsAI is the
// source of truth for them, and they change whenever an operator registers or
// rotates a GitHub App, so git-bot re-reads them on an interval instead of
// freezing the values it happened to find at startup. Until a GitHub App exists,
// git-bot serves in degraded mode and recovers on its own once one is connected.
type githubRuntime struct {
	cfg         *config.Config
	httpClient  *http.Client
	credentials *serviceauth.Credentials
	resolver    *service.GitHubAppClientResolver

	mu            sync.RWMutex
	appID         int64
	webhookSecret string
	privateKey    string
}

func newGitHubRuntime(
	cfg *config.Config,
	httpClient *http.Client,
	credentials *serviceauth.Credentials,
) *githubRuntime {
	runtime := &githubRuntime{
		cfg:         cfg,
		httpClient:  httpClient,
		credentials: credentials,
	}
	runtime.resolver = service.NewGitHubClientResolver(
		newNopsaiInstallationFetcher(cfg, httpClient, credentials),
		nil,
		gitHubInstallationCacheTTL,
	)
	return runtime
}

func (r *githubRuntime) AppID() int64 {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.appID
}

func (r *githubRuntime) WebhookSecret() string {
	if r == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.webhookSecret
}

func (r *githubRuntime) configured() bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.appID > 0 && strings.TrimSpace(r.privateKey) != ""
}

// refresh reads the current App credentials from NopsAI and rebuilds the client
// factory when they changed. An unchanged response is a cheap no-op, and a
// failed read keeps the previously working credentials in place.
func (r *githubRuntime) refresh(ctx context.Context) error {
	bootstrap, err := requestGitHubBootstrap(
		ctx,
		r.cfg,
		r.httpClient,
		r.credentials,
		gitHubBootstrapURL(r.cfg),
	)
	if err != nil {
		return err
	}
	appID, err := strconv.ParseInt(strings.TrimSpace(bootstrap.GitHubAppID), 10, 64)
	if err != nil || appID <= 0 {
		return errInvalidGitHubAppID
	}

	r.mu.RLock()
	unchanged := appID == r.appID &&
		bootstrap.GitHubPrivateKey == r.privateKey &&
		bootstrap.GitHubWebhookSecret == r.webhookSecret
	r.mu.RUnlock()
	if unchanged {
		return nil
	}

	transport, err := ghinstallation.NewAppsTransport(http.DefaultTransport, appID, []byte(bootstrap.GitHubPrivateKey))
	if err != nil {
		return err
	}
	r.resolver.ReplaceClientFactory(func(installationID int64) *github.Client {
		installationTransport := ghinstallation.NewFromAppsTransport(transport, installationID)
		return github.NewClient(&http.Client{
			Transport: installationTransport,
			Timeout:   15 * time.Second,
		})
	})

	r.mu.Lock()
	previousAppID := r.appID
	r.appID = appID
	r.privateKey = bootstrap.GitHubPrivateKey
	r.webhookSecret = bootstrap.GitHubWebhookSecret
	r.mu.Unlock()

	event := log.Info().Int64("github_app_id", appID)
	if previousAppID == 0 {
		event.Msg("GitHub App credentials loaded")
	} else {
		event.Int64("previous_github_app_id", previousAppID).Msg("GitHub App credentials reloaded")
	}
	return nil
}

// watch keeps the credentials current for the lifetime of the process.
func (r *githubRuntime) watch(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = gitHubCredentialRefreshInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.refresh(ctx); err != nil {
				log.Debug().Err(err).Msg("GitHub App credentials are not available yet")
			}
		}
	}
}

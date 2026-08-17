package service

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v53/github"
)

type GitHubInstallation struct {
	InstallationID int64
	AccountLogin   string
	AccountType    string
	Enabled        bool
}

type GitHubClientResolver interface {
	ClientForRepository(ctx context.Context, owner string, repo string) (*github.Client, GitHubInstallation, error)
	ClientForInstallation(ctx context.Context, installationID int64) (*github.Client, GitHubInstallation, error)
	InstallationForID(ctx context.Context, installationID int64) (GitHubInstallation, error)
}

type GitHubInstallationFetcher func(context.Context) ([]GitHubInstallation, error)
type GitHubClientFactory func(int64) *github.Client

// GitHubAppClientResolver caches one GitHub client per installation. The client
// factory can be replaced while git-bot is running so rotated App credentials
// take effect without a restart.
type GitHubAppClientResolver struct {
	fetcher       GitHubInstallationFetcher
	clientFactory GitHubClientFactory
	ttl           time.Duration

	mu          sync.Mutex
	lastRefresh time.Time
	byOwner     map[string]GitHubInstallation
	byID        map[int64]GitHubInstallation
	clientsByID map[int64]*github.Client
}

func NewGitHubClientResolver(fetcher GitHubInstallationFetcher, clientFactory GitHubClientFactory, ttl time.Duration) *GitHubAppClientResolver {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &GitHubAppClientResolver{
		fetcher:       fetcher,
		clientFactory: clientFactory,
		ttl:           ttl,
		byOwner:       map[string]GitHubInstallation{},
		byID:          map[int64]GitHubInstallation{},
		clientsByID:   map[int64]*github.Client{},
	}
}

func StaticGitHubInstallationFetcher(installations []GitHubInstallation) GitHubInstallationFetcher {
	copied := append([]GitHubInstallation(nil), installations...)
	return func(context.Context) ([]GitHubInstallation, error) {
		return append([]GitHubInstallation(nil), copied...), nil
	}
}

// ReplaceClientFactory swaps the App credentials the resolver mints clients
// with and drops every cached client, so no request keeps using a superseded
// private key.
func (r *GitHubAppClientResolver) ReplaceClientFactory(clientFactory GitHubClientFactory) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clientFactory = clientFactory
	r.clientsByID = map[int64]*github.Client{}
}

func (r *GitHubAppClientResolver) ClientForRepository(ctx context.Context, owner string, _ string) (*github.Client, GitHubInstallation, error) {
	if r == nil {
		return nil, GitHubInstallation{}, githubIntegrationUnavailableError()
	}
	owner = strings.ToLower(strings.TrimSpace(owner))
	if owner == "" {
		return nil, GitHubInstallation{}, providerError{Status: http.StatusBadRequest, Message: "repository owner is required"}
	}
	installation, err := r.findByOwner(ctx, owner)
	if err != nil {
		return nil, GitHubInstallation{}, err
	}
	return r.clientForInstallation(installation)
}

func (r *GitHubAppClientResolver) ClientForInstallation(ctx context.Context, installationID int64) (*github.Client, GitHubInstallation, error) {
	installation, err := r.InstallationForID(ctx, installationID)
	if err != nil {
		return nil, GitHubInstallation{}, err
	}
	return r.clientForInstallation(installation)
}

func (r *GitHubAppClientResolver) InstallationForID(ctx context.Context, installationID int64) (GitHubInstallation, error) {
	if r == nil {
		return GitHubInstallation{}, githubIntegrationUnavailableError()
	}
	if installationID == 0 {
		return GitHubInstallation{}, providerError{Status: http.StatusBadRequest, Message: "installation id is required"}
	}
	if err := r.refreshIfNeeded(ctx, false); err != nil {
		return GitHubInstallation{}, err
	}
	r.mu.Lock()
	installation, ok := r.byID[installationID]
	r.mu.Unlock()
	if !ok {
		if err := r.refreshIfNeeded(ctx, true); err != nil {
			return GitHubInstallation{}, err
		}
		r.mu.Lock()
		installation, ok = r.byID[installationID]
		r.mu.Unlock()
	}
	if !ok {
		return GitHubInstallation{}, providerError{Status: http.StatusForbidden, Message: "GitHub App installation is not registered"}
	}
	if !installation.Enabled {
		return GitHubInstallation{}, providerError{Status: http.StatusForbidden, Message: "GitHub App installation is disabled"}
	}
	return installation, nil
}

func (r *GitHubAppClientResolver) findByOwner(ctx context.Context, owner string) (GitHubInstallation, error) {
	if err := r.refreshIfNeeded(ctx, false); err != nil {
		return GitHubInstallation{}, err
	}
	r.mu.Lock()
	installation, ok := r.byOwner[owner]
	fallback, hasFallback := r.singleFallbackLocked()
	r.mu.Unlock()
	if !ok {
		if err := r.refreshIfNeeded(ctx, true); err != nil {
			return GitHubInstallation{}, err
		}
		r.mu.Lock()
		installation, ok = r.byOwner[owner]
		fallback, hasFallback = r.singleFallbackLocked()
		r.mu.Unlock()
	}
	if !ok && hasFallback {
		installation = fallback
		ok = true
	}
	if !ok {
		return GitHubInstallation{}, providerError{Status: http.StatusForbidden, Message: "no registered GitHub App installation for repository owner"}
	}
	if !installation.Enabled {
		return GitHubInstallation{}, providerError{Status: http.StatusForbidden, Message: "GitHub App installation is disabled"}
	}
	return installation, nil
}

func (r *GitHubAppClientResolver) clientForInstallation(installation GitHubInstallation) (*github.Client, GitHubInstallation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if client := r.clientsByID[installation.InstallationID]; client != nil {
		return client, installation, nil
	}
	if r.clientFactory == nil {
		return nil, GitHubInstallation{}, githubIntegrationUnavailableError()
	}
	client := r.clientFactory(installation.InstallationID)
	if client == nil {
		return nil, GitHubInstallation{}, githubIntegrationUnavailableError()
	}
	r.clientsByID[installation.InstallationID] = client
	return client, installation, nil
}

func (r *GitHubAppClientResolver) refreshIfNeeded(ctx context.Context, force bool) error {
	r.mu.Lock()
	needsRefresh := force || r.lastRefresh.IsZero() || time.Since(r.lastRefresh) > r.ttl
	r.mu.Unlock()
	if !needsRefresh {
		return nil
	}
	if r.fetcher == nil {
		return githubIntegrationUnavailableError()
	}
	installations, err := r.fetcher(ctx)
	if err != nil {
		return providerError{Status: http.StatusBadGateway, Message: "failed to refresh GitHub App installations", Err: err}
	}
	byOwner := map[string]GitHubInstallation{}
	byID := map[int64]GitHubInstallation{}
	for _, installation := range installations {
		if installation.InstallationID == 0 {
			continue
		}
		accountLogin := strings.ToLower(strings.TrimSpace(installation.AccountLogin))
		if accountLogin != "" {
			byOwner[accountLogin] = installation
		}
		byID[installation.InstallationID] = installation
	}
	r.mu.Lock()
	r.byOwner = byOwner
	r.byID = byID
	r.lastRefresh = time.Now()
	r.mu.Unlock()
	return nil
}

func (r *GitHubAppClientResolver) singleFallbackLocked() (GitHubInstallation, bool) {
	if len(r.byID) != 1 {
		return GitHubInstallation{}, false
	}
	for _, installation := range r.byID {
		if strings.TrimSpace(installation.AccountLogin) == "" {
			return installation, true
		}
	}
	return GitHubInstallation{}, false
}

func ParseGitHubInstallationID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("installation id must be a positive integer")
	}
	return id, nil
}

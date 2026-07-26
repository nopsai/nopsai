package nopsai

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/config"
	"nopsai/pkg/serviceauth"
	"nopsai/services/nopsai/pkg/auth"
)

func TestHandleCreateGitHubAppInstallationRejectsOwnershipFields(t *testing.T) {
	app := App{cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/git-apps/github/installations", strings.NewReader(`{
		"installation_id": "987654",
		"account_login": "nopsai",
		"team_path": "platform"
	}`))
	rec := httptest.NewRecorder()

	app.handleCreateGitHubAppInstallation(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if len(app.getConfigSnapshot().GitHubInstallations) != 0 {
		t.Fatalf("installations mutated on invalid request: %#v", app.getConfigSnapshot().GitHubInstallations)
	}
}

func TestHandleCreateGitHubAppInstallationStoresNormalizedInstallation(t *testing.T) {
	app := App{cfg: &config.Config{GitHubInstallID: "123"}}
	req := httptest.NewRequest(http.MethodPost, "/v1/git-apps/github/installations", strings.NewReader(`{
		"installation_id": "987654",
		"account_login": " nopsai ",
		"account_type": "org",
		"enabled": true
	}`))
	rec := httptest.NewRecorder()

	app.handleCreateGitHubAppInstallation(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var resp githubAppInstallation
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.InstallationID != "987654" || resp.AccountLogin != "nopsai" || resp.AccountType != "organization" || !resp.Enabled {
		t.Fatalf("response = %#v", resp)
	}
	cfg := app.getConfigSnapshot()
	if cfg.GitHubInstallID != "" || len(cfg.GitHubInstallations) != 2 {
		t.Fatalf("config installations = scalar %q list %#v", cfg.GitHubInstallID, cfg.GitHubInstallations)
	}
}

func TestHandleInternalGitBotInstallationsRequiresGitBotServiceIdentity(t *testing.T) {
	app := App{cfg: &config.Config{
		GitHubInstallations: []config.GitHubInstallationConfig{{
			InstallationID: "987654",
			AccountLogin:   "nopsai",
			AccountType:    "organization",
			Enabled:        boolPtr(true),
		}},
	}}

	unauthorizedReq := httptest.NewRequest(http.MethodGet, "/v1/internal/git-bot/installations", nil)
	unauthorizedRec := httptest.NewRecorder()
	app.handleInternalGitBotInstallations(unauthorizedRec, unauthorizedReq)
	if unauthorizedRec.Code != http.StatusForbidden {
		t.Fatalf("unauthorized status = %d, want %d", unauthorizedRec.Code, http.StatusForbidden)
	}

	authorizedReq := httptest.NewRequest(http.MethodGet, "/v1/internal/git-bot/installations", nil)
	authorizedReq = authorizedReq.WithContext(auth.WithClaims(authorizedReq.Context(), &auth.Claims{
		Sub:      "git-bot",
		Provider: serviceauth.ProviderInternalService,
		Roles:    []string{serviceauth.RoleGitBot},
	}))
	authorizedRec := httptest.NewRecorder()
	app.handleInternalGitBotInstallations(authorizedRec, authorizedReq)
	if authorizedRec.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, want %d: %s", authorizedRec.Code, http.StatusOK, authorizedRec.Body.String())
	}
	var resp []githubAppInstallation
	if err := json.Unmarshal(authorizedRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp) != 1 || resp[0].InstallationID != "987654" {
		t.Fatalf("installations response = %#v", resp)
	}
}

func TestHandleRefreshGitHubAppInstallationPersistsRepositoryMetadataOnly(t *testing.T) {
	provider := &fakeGitProvider{
		repositories: []GitHubInstalledRepository{
			{ID: 1, FullName: "nopsai/api", Owner: "nopsai", Name: "api", Private: true, DefaultBranch: "main"},
			{ID: 2, FullName: "nopsai/worker", Owner: "nopsai", Name: "worker", Private: true, DefaultBranch: "trunk"},
		},
	}
	app := App{
		cfg: &config.Config{GitHubInstallations: []config.GitHubInstallationConfig{{
			InstallationID:          "987654",
			AccountLogin:            "nopsai",
			AccountType:             "organization",
			Enabled:                 boolPtr(true),
			AccessibleRepositories:  1,
			LastRepositoryRefreshAt: "2026-07-25T10:00:00Z",
			LastError:               "previous failure",
		}}},
		gitProvider: provider,
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/git-apps/github/installations/987654/refresh", nil)
	req.SetPathValue("installationID", "987654")
	rec := httptest.NewRecorder()

	app.handleRefreshGitHubAppInstallation(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if provider.repositoryInstallationID != "987654" {
		t.Fatalf("repository installation id = %q, want 987654", provider.repositoryInstallationID)
	}
	var resp githubAppInstallation
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.AccessibleRepositories != 2 || len(resp.Repositories) != 2 || resp.LastRepositoryRefreshAt == "" || resp.LastError != "" {
		t.Fatalf("response metadata = %#v", resp)
	}
	cfg := app.getConfigSnapshot()
	if len(cfg.GitHubInstallations) != 1 {
		t.Fatalf("config installations = %#v", cfg.GitHubInstallations)
	}
	installation := cfg.GitHubInstallations[0]
	if installation.AccessibleRepositories != 2 || installation.LastRepositoryRefreshAt == "" || installation.LastError != "" {
		t.Fatalf("stored installation metadata = %#v", installation)
	}
}

func TestHandleRefreshGitHubAppInstallationPersistsLastError(t *testing.T) {
	app := App{
		cfg: &config.Config{GitHubInstallations: []config.GitHubInstallationConfig{{
			InstallationID:          "987654",
			AccountLogin:            "nopsai",
			AccountType:             "organization",
			Enabled:                 boolPtr(true),
			AccessibleRepositories:  3,
			LastRepositoryRefreshAt: "2026-07-25T10:00:00Z",
		}}},
		gitProvider: &fakeGitProvider{repositoriesErr: errors.New("GitHub API unavailable")},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/git-apps/github/installations/987654/refresh", nil)
	req.SetPathValue("installationID", "987654")
	rec := httptest.NewRecorder()

	app.handleRefreshGitHubAppInstallation(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	cfg := app.getConfigSnapshot()
	if len(cfg.GitHubInstallations) != 1 ||
		cfg.GitHubInstallations[0].AccessibleRepositories != 3 ||
		cfg.GitHubInstallations[0].LastRepositoryRefreshAt != "2026-07-25T10:00:00Z" ||
		!strings.Contains(cfg.GitHubInstallations[0].LastError, "GitHub API unavailable") {
		t.Fatalf("stored installation metadata = %#v", cfg.GitHubInstallations)
	}
}

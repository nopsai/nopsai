package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/serviceauth"
	"nopsai/services/nopsai/pkg/auth"
)

type githubAppResource struct {
	Provider                string                  `json:"provider" yaml:"provider"`
	AppID                   string                  `json:"app_id" yaml:"app_id"`
	PrivateKeyCredentialRef string                  `json:"private_key_credential_ref" yaml:"private_key_credential_ref"`
	WebhookCredentialRef    string                  `json:"webhook_credential_ref" yaml:"webhook_credential_ref"`
	WebhookEndpoint         string                  `json:"webhook_endpoint,omitempty" yaml:"-"`
	Installations           []githubAppInstallation `json:"installations" yaml:"installations"`
}

type githubAppInstallation struct {
	InstallationID          string                      `json:"installation_id" yaml:"installation_id"`
	AccountLogin            string                      `json:"account_login,omitempty" yaml:"account_login,omitempty"`
	AccountType             string                      `json:"account_type,omitempty" yaml:"account_type,omitempty"`
	Enabled                 bool                        `json:"enabled" yaml:"enabled"`
	RepositorySelection     string                      `json:"repository_selection,omitempty" yaml:"-"`
	AccessibleRepositories  int                         `json:"accessible_repositories" yaml:"-"`
	ConnectedTriggers       int                         `json:"connected_triggers" yaml:"-"`
	LastVerifiedAt          string                      `json:"last_verified_at,omitempty" yaml:"-"`
	LastRepositoryRefreshAt string                      `json:"last_repository_refresh_at,omitempty" yaml:"-"`
	LastError               string                      `json:"last_error,omitempty" yaml:"-"`
	Status                  string                      `json:"status" yaml:"-"`
	Repositories            []githubAppInstallationRepo `json:"repositories,omitempty" yaml:"-"`
}

type githubAppInstallationRepo struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch,omitempty"`
	Access        string `json:"access,omitempty"`
	UsedByNopsAI  bool   `json:"used_by_nopsai"`
}

func (a *App) handleGetGitHubApp(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.gitHubAppResource(a.getConfigSnapshot()))
}

func (a *App) handlePutGitHubApp(w http.ResponseWriter, r *http.Request) {
	var req githubAppResource
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if provider := strings.ToLower(strings.TrimSpace(req.Provider)); provider != "" && provider != "github" {
		http.Error(w, "provider must be github", http.StatusBadRequest)
		return
	}

	currentInstallations := gitHubInstallationConfigByID(config.NormalizeGitHubInstallations(a.getConfigSnapshot().GitHubInstallations, a.getConfigSnapshot().GitHubInstallID))
	installations := make([]config.GitHubInstallationConfig, 0, len(req.Installations))
	for _, installation := range req.Installations {
		next := installation.config()
		if current, exists := currentInstallations[next.InstallationID]; exists {
			next = mergeGitHubInstallationRuntimeMetadata(next, current)
		}
		installations = append(installations, next)
	}
	payload := systemConfigPayload{
		GitHubAppID:         stringPtr(strings.TrimSpace(req.AppID)),
		GitHubPrivateKeyRef: stringPtr(strings.TrimSpace(req.PrivateKeyCredentialRef)),
		GitHubWebhookRef:    stringPtr(strings.TrimSpace(req.WebhookCredentialRef)),
		GitHubInstallations: &installations,
	}
	cfg, err := a.applySystemConfig(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.ensureGitHubAppCredentialReferences(r.Context(), cfg, credentialActorFromContext(r.Context())); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.persistRuntimeSettingsSnapshot(r.Context(), cfg, "database", nil, "", "", false); err != nil {
		http.Error(w, "failed to persist GitHub App settings", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, a.gitHubAppResource(cfg))
}

func (a *App) handleListGitHubAppInstallations(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.gitHubAppResource(a.getConfigSnapshot()).Installations)
}

func (a *App) handleCreateGitHubAppInstallation(w http.ResponseWriter, r *http.Request) {
	raw := map[string]json.RawMessage{}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	for _, unsupported := range []string{"team_path", "visibility", "repository_allowlist", "repositories"} {
		if _, exists := raw[unsupported]; exists {
			http.Error(w, fmt.Sprintf("%s does not belong on a GitHub App installation", unsupported), http.StatusBadRequest)
			return
		}
	}
	data, _ := json.Marshal(raw)
	var req githubAppInstallation
	if err := json.Unmarshal(data, &req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.InstallationID) == "" {
		http.Error(w, "installation_id is required", http.StatusBadRequest)
		return
	}

	cfg := a.getConfigSnapshot()
	installations := config.NormalizeGitHubInstallations(cfg.GitHubInstallations, cfg.GitHubInstallID)
	next := req.config()
	replaced := false
	for idx, current := range installations {
		if strings.TrimSpace(current.InstallationID) == next.InstallationID {
			installations[idx] = mergeGitHubInstallationRuntimeMetadata(next, current)
			replaced = true
			break
		}
	}
	if !replaced {
		installations = append(installations, next)
	}
	cfg, err := a.applySystemConfig(systemConfigPayload{GitHubInstallations: &installations})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.persistRuntimeSettingsSnapshot(r.Context(), cfg, "database", nil, "", "", false); err != nil {
		http.Error(w, "failed to persist GitHub App installation", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, a.gitHubInstallationResource(cfg, next.InstallationID))
}

func (a *App) handleGetGitHubAppInstallation(w http.ResponseWriter, r *http.Request) {
	installationID := r.PathValue("installationID")
	installation := a.gitHubInstallationResource(a.getConfigSnapshot(), installationID)
	if strings.TrimSpace(installation.InstallationID) == "" {
		http.Error(w, "GitHub App installation not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, installation)
}

func (a *App) handleDeleteGitHubAppInstallation(w http.ResponseWriter, r *http.Request) {
	installationID := strings.TrimSpace(r.PathValue("installationID"))
	cfg := a.getConfigSnapshot()
	installations := config.NormalizeGitHubInstallations(cfg.GitHubInstallations, cfg.GitHubInstallID)
	next := installations[:0]
	found := false
	for _, current := range installations {
		if strings.TrimSpace(current.InstallationID) == installationID {
			found = true
			continue
		}
		next = append(next, current)
	}
	if !found {
		http.Error(w, "GitHub App installation not found", http.StatusNotFound)
		return
	}
	cfg, err := a.applySystemConfig(systemConfigPayload{GitHubInstallations: &next})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.persistRuntimeSettingsSnapshot(r.Context(), cfg, "database", nil, "", "", false); err != nil {
		http.Error(w, "failed to persist GitHub App installation", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleVerifyGitHubAppInstallation(w http.ResponseWriter, r *http.Request) {
	installation := a.gitHubInstallationResource(a.getConfigSnapshot(), r.PathValue("installationID"))
	if strings.TrimSpace(installation.InstallationID) == "" {
		http.Error(w, "GitHub App installation not found", http.StatusNotFound)
		return
	}
	verifiedAt := time.Now().UTC().Format(time.RFC3339)
	updated, err := a.updateGitHubInstallationRuntimeMetadata(r.Context(), installation.InstallationID, func(current *config.GitHubInstallationConfig) {
		current.LastVerifiedAt = verifiedAt
	})
	if err != nil {
		http.Error(w, "failed to persist GitHub App verification metadata", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(updated.InstallationID) != "" {
		installation = updated
	}
	installation.LastVerifiedAt = verifiedAt
	writeJSON(w, http.StatusOK, installation)
}

func (a *App) handleRefreshGitHubAppInstallation(w http.ResponseWriter, r *http.Request) {
	installation := a.gitHubInstallationResource(a.getConfigSnapshot(), r.PathValue("installationID"))
	if strings.TrimSpace(installation.InstallationID) == "" {
		http.Error(w, "GitHub App installation not found", http.StatusNotFound)
		return
	}
	repositories, err := a.gitClient().ListInstallationRepositories(installation.InstallationID)
	if err != nil {
		lastError := err.Error()
		if updated, persistErr := a.updateGitHubInstallationRuntimeMetadata(r.Context(), installation.InstallationID, func(current *config.GitHubInstallationConfig) {
			current.LastError = lastError
		}); persistErr == nil && strings.TrimSpace(updated.InstallationID) != "" {
			installation = updated
		}
		installation.LastError = err.Error()
		installation.Status = "error"
		writeJSON(w, http.StatusBadGateway, installation)
		return
	}
	decorated := a.decorateGitHubInstallationRepositories(repositories)
	refreshedAt := time.Now().UTC().Format(time.RFC3339)
	updated, persistErr := a.updateGitHubInstallationRuntimeMetadata(r.Context(), installation.InstallationID, func(current *config.GitHubInstallationConfig) {
		current.AccessibleRepositories = len(decorated)
		current.LastRepositoryRefreshAt = refreshedAt
		current.LastError = ""
	})
	if persistErr != nil {
		http.Error(w, "failed to persist GitHub repository metadata", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(updated.InstallationID) != "" {
		installation = updated
	}
	installation.Repositories = decorated
	installation.AccessibleRepositories = len(decorated)
	installation.RepositorySelection = repositorySelectionLabel(installation.AccessibleRepositories, installation.LastRepositoryRefreshAt)
	installation.LastRepositoryRefreshAt = refreshedAt
	writeJSON(w, http.StatusOK, installation)
}

func (a *App) handleListGitHubAppInstallationRepositories(w http.ResponseWriter, r *http.Request) {
	installation := a.gitHubInstallationResource(a.getConfigSnapshot(), r.PathValue("installationID"))
	if strings.TrimSpace(installation.InstallationID) == "" {
		http.Error(w, "GitHub App installation not found", http.StatusNotFound)
		return
	}
	repositories, err := a.gitClient().ListInstallationRepositories(installation.InstallationID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, a.decorateGitHubInstallationRepositories(repositories))
}

func (a *App) handleInternalGitBotInstallations(w http.ResponseWriter, r *http.Request) {
	if !gitBotServiceRequest(r) {
		http.Error(w, "git-bot service identity required", http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, a.gitHubAppResource(a.getConfigSnapshot()).Installations)
}

func (a *App) gitHubAppResource(cfg config.Config) githubAppResource {
	installations := config.NormalizeGitHubInstallations(cfg.GitHubInstallations, cfg.GitHubInstallID)
	out := githubAppResource{
		Provider:                "github",
		AppID:                   strings.TrimSpace(cfg.GitHubAppID),
		PrivateKeyCredentialRef: strings.TrimSpace(cfg.GitHubPrivateKeyCredentialRef),
		WebhookCredentialRef:    strings.TrimSpace(cfg.GitHubWebhookCredentialRef),
		WebhookEndpoint:         strings.TrimRight(strings.TrimSpace(cfg.PublicURL), "/") + "/webhook",
		Installations:           make([]githubAppInstallation, 0, len(installations)),
	}
	if strings.TrimSpace(cfg.PublicURL) == "" {
		out.WebhookEndpoint = "/webhook"
	}
	triggerCounts := a.githubConnectedTriggerCounts()
	for _, installation := range installations {
		item := githubAppInstallationFromConfig(installation)
		item.ConnectedTriggers = triggerCounts[strings.ToLower(item.AccountLogin)]
		out.Installations = append(out.Installations, item)
	}
	sort.Slice(out.Installations, func(i, j int) bool {
		return strings.ToLower(out.Installations[i].AccountLogin+out.Installations[i].InstallationID) <
			strings.ToLower(out.Installations[j].AccountLogin+out.Installations[j].InstallationID)
	})
	return out
}

func (a *App) gitHubInstallationResource(cfg config.Config, installationID string) githubAppInstallation {
	installationID = strings.TrimSpace(installationID)
	for _, installation := range a.gitHubAppResource(cfg).Installations {
		if strings.TrimSpace(installation.InstallationID) == installationID {
			return installation
		}
	}
	return githubAppInstallation{}
}

func githubAppInstallationFromConfig(installation config.GitHubInstallationConfig) githubAppInstallation {
	enabled := config.GitHubInstallationEnabled(installation)
	lastError := strings.TrimSpace(installation.LastError)
	status := "connected"
	if !enabled {
		status = "disabled"
	} else if lastError != "" {
		status = "error"
	}
	return githubAppInstallation{
		InstallationID:          strings.TrimSpace(installation.InstallationID),
		AccountLogin:            strings.TrimSpace(installation.AccountLogin),
		AccountType:             config.NormalizeGitHubAccountType(installation.AccountType),
		Enabled:                 enabled,
		RepositorySelection:     repositorySelectionLabel(installation.AccessibleRepositories, installation.LastRepositoryRefreshAt),
		AccessibleRepositories:  max(0, installation.AccessibleRepositories),
		LastVerifiedAt:          strings.TrimSpace(installation.LastVerifiedAt),
		LastRepositoryRefreshAt: strings.TrimSpace(installation.LastRepositoryRefreshAt),
		LastError:               lastError,
		Status:                  status,
	}
}

func (installation githubAppInstallation) config() config.GitHubInstallationConfig {
	enabled := installation.Enabled
	return config.GitHubInstallationConfig{
		InstallationID:          strings.TrimSpace(installation.InstallationID),
		AccountLogin:            strings.TrimSpace(installation.AccountLogin),
		AccountType:             config.NormalizeGitHubAccountType(installation.AccountType),
		Enabled:                 &enabled,
		AccessibleRepositories:  max(0, installation.AccessibleRepositories),
		LastVerifiedAt:          strings.TrimSpace(installation.LastVerifiedAt),
		LastRepositoryRefreshAt: strings.TrimSpace(installation.LastRepositoryRefreshAt),
		LastError:               strings.TrimSpace(installation.LastError),
	}
}

func gitHubInstallationConfigByID(installations []config.GitHubInstallationConfig) map[string]config.GitHubInstallationConfig {
	byID := make(map[string]config.GitHubInstallationConfig, len(installations))
	for _, installation := range installations {
		id := strings.TrimSpace(installation.InstallationID)
		if id != "" {
			byID[id] = installation
		}
	}
	return byID
}

func mergeGitHubInstallationRuntimeMetadata(next, current config.GitHubInstallationConfig) config.GitHubInstallationConfig {
	if strings.TrimSpace(next.LastVerifiedAt) == "" {
		next.LastVerifiedAt = strings.TrimSpace(current.LastVerifiedAt)
	}
	if next.AccessibleRepositories == 0 && strings.TrimSpace(next.LastRepositoryRefreshAt) == "" {
		next.AccessibleRepositories = max(0, current.AccessibleRepositories)
		next.LastRepositoryRefreshAt = strings.TrimSpace(current.LastRepositoryRefreshAt)
	}
	if strings.TrimSpace(next.LastError) == "" {
		next.LastError = strings.TrimSpace(current.LastError)
	}
	return next
}

func (a *App) updateGitHubInstallationRuntimeMetadata(
	ctx context.Context,
	installationID string,
	mutate func(*config.GitHubInstallationConfig),
) (githubAppInstallation, error) {
	installationID = strings.TrimSpace(installationID)
	if installationID == "" {
		return githubAppInstallation{}, nil
	}
	cfg := a.getConfigSnapshot()
	installations := config.NormalizeGitHubInstallations(cfg.GitHubInstallations, cfg.GitHubInstallID)
	found := false
	var updated config.GitHubInstallationConfig
	for idx := range installations {
		if strings.TrimSpace(installations[idx].InstallationID) != installationID {
			continue
		}
		mutate(&installations[idx])
		updated = installations[idx]
		found = true
		break
	}
	if !found {
		return githubAppInstallation{}, nil
	}
	nextCfg, err := a.applySystemConfig(systemConfigPayload{GitHubInstallations: &installations})
	if err != nil {
		return githubAppInstallation{}, err
	}
	if err := a.persistRuntimeSettingsSnapshot(ctx, nextCfg, "database", nil, "", "", false); err != nil {
		return githubAppInstallation{}, err
	}
	return githubAppInstallationFromConfig(updated), nil
}

func (a *App) githubConnectedTriggerCounts() map[string]int {
	counts := map[string]int{}
	if a == nil || a.db == nil {
		return counts
	}
	rows, err := a.db.Query(contextBackground(), `
		SELECT SPLIT_PART(repository_name, '/', 1), COUNT(*)::int
		FROM triggers
		WHERE provider = 'github' AND repository_name LIKE '%/%'
		GROUP BY 1
	`)
	if err != nil {
		return counts
	}
	defer rows.Close()
	for rows.Next() {
		var owner string
		var count int
		if err := rows.Scan(&owner, &count); err == nil {
			counts[strings.ToLower(strings.TrimSpace(owner))] = count
		}
	}
	return counts
}

func (a *App) decorateGitHubInstallationRepositories(repositories []GitHubInstalledRepository) []githubAppInstallationRepo {
	triggerRepos := a.githubTriggerRepositorySet()
	out := make([]githubAppInstallationRepo, 0, len(repositories))
	for _, repo := range repositories {
		fullName := strings.TrimSpace(repo.FullName)
		out = append(out, githubAppInstallationRepo{
			ID:            repo.ID,
			FullName:      fullName,
			Owner:         repo.Owner,
			Name:          repo.Name,
			Private:       repo.Private,
			DefaultBranch: repo.DefaultBranch,
			Access:        "read/write",
			UsedByNopsAI:  triggerRepos[strings.ToLower(fullName)],
		})
	}
	return out
}

func (a *App) githubTriggerRepositorySet() map[string]bool {
	repos := map[string]bool{}
	if a == nil || a.db == nil {
		return repos
	}
	rows, err := a.db.Query(contextBackground(), `SELECT repository_name FROM triggers WHERE provider = 'github'`)
	if err != nil {
		return repos
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			repos[strings.ToLower(strings.TrimSpace(name))] = true
		}
	}
	return repos
}

func repositorySelectionLabel(count int, refreshedAt string) string {
	if count > 0 {
		return "selected"
	}
	if strings.TrimSpace(refreshedAt) != "" {
		return "none"
	}
	return "unknown"
}

func gitBotServiceRequest(r *http.Request) bool {
	claims, ok := auth.ClaimsFromContext(r.Context())
	return ok && claims != nil &&
		strings.EqualFold(strings.TrimSpace(claims.Provider), serviceauth.ProviderInternalService) &&
		containsFold(claims.Roles, serviceauth.RoleGitBot)
}

func contextBackground() context.Context {
	return context.Background()
}

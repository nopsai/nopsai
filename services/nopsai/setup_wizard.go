package main

import (
	"archive/zip"
	"context"
	crand "crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/yaml.v3"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/services/nopsai/pkg/auth"
)

const (
	setupStateKeyCompletedAt = "completed_at"
	setupStateKeyProfile     = "profile"

	setupProfileDev        = "dev"
	setupProfileTeam       = "team"
	setupProfileProduction = "production"
	setupProfileEmpty      = "empty"
)

type setupStarterProfile struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type setupCheck struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
	Blocking bool   `json:"blocking"`
}

type setupCounts struct {
	Users              int `json:"users"`
	Pipelines          int `json:"pipelines"`
	Steps              int `json:"steps"`
	Triggers           int `json:"triggers"`
	Groups             int `json:"groups"`
	AccessGrants       int `json:"access_grants"`
	LLMProfiles        int `json:"llm_profiles"`
	MCPServers         int `json:"mcp_servers"`
	MCPProfiles        int `json:"mcp_profiles"`
	KnowledgeContexts  int `json:"knowledge_contexts"`
	ConfigRepositories int `json:"config_repositories"`
}

type setupGitHubInfo struct {
	WebhookURL                 string            `json:"webhook_url"`
	GitBotServiceURL           string            `json:"git_bot_service_url,omitempty"`
	NopsaiAPIURL               string            `json:"nopsai_api_url,omitempty"`
	RequiredEvents             []string          `json:"required_events"`
	RequiredPermissions        map[string]string `json:"required_permissions"`
	AppIDConfigured            bool              `json:"app_id_configured"`
	InstallationIDConfigured   bool              `json:"installation_id_configured"`
	PrivateKeyConfigured       bool              `json:"private_key_configured"`
	WebhookSecretConfigured    bool              `json:"webhook_secret_configured"`
	GitBotURLConfigured        bool              `json:"git_bot_url_configured"`
	NopsaiForwardURLConfigured bool              `json:"nopsai_forward_url_configured"`
}

type setupStatusResponse struct {
	Completed        bool                     `json:"completed"`
	CompletedAt      string                   `json:"completed_at,omitempty"`
	Profile          string                   `json:"profile,omitempty"`
	RuntimeEnv       string                   `json:"runtime_env,omitempty"`
	EnvFilePath      string                   `json:"env_file_path,omitempty"`
	Counts           setupCounts              `json:"counts"`
	Checks           []setupCheck             `json:"checks"`
	StarterProfiles  []setupStarterProfile    `json:"starter_profiles"`
	GitHub           setupGitHubInfo          `json:"github"`
	GlobalConfigRepo *models.ConfigRepository `json:"global_config_repo,omitempty"`
}

type setupConfigRepositoryInput struct {
	RepoURL  string `json:"repo_url"`
	Branch   string `json:"branch"`
	BasePath string `json:"base_path"`
	Enabled  *bool  `json:"enabled"`
}

type setupLLMProfileInput struct {
	Name          string   `json:"name"`
	Provider      string   `json:"provider"`
	Model         string   `json:"model"`
	BaseURL       string   `json:"base_url"`
	APIKeySecret  string   `json:"api_key_secret"`
	APIKeyValue   string   `json:"api_key_value"`
	AllowedScopes []string `json:"allowed_scopes"`
}

type setupRepositoryGroupInput struct {
	Name         string   `json:"name"`
	Repositories []string `json:"repositories"`
}

type setupUserInput struct {
	Sub      string `json:"sub"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Password string `json:"password"`
	Group    string `json:"group"`
}

type setupBootstrapRequest struct {
	Profile                string                      `json:"profile"`
	GenerateSecrets        bool                        `json:"generate_secrets"`
	SeedStarterDatabase    bool                        `json:"seed_starter_database"`
	SeedLLMProfile         *bool                       `json:"seed_llm_profile"`
	MCPExamples            bool                        `json:"mcp_examples"`
	ProductionAcknowledged bool                        `json:"production_acknowledged"`
	SyncConfigRepository   bool                        `json:"sync_config_repository"`
	ConfigRepository       *setupConfigRepositoryInput `json:"config_repository"`
	RepositoryGroups       []setupRepositoryGroupInput `json:"repository_groups"`
	Repositories           []string                    `json:"repositories"`
	LLMProfile             setupLLMProfileInput        `json:"llm_profile"`
	Users                  []setupUserInput            `json:"users"`
}

type setupTemporaryCredential struct {
	Sub               string `json:"sub"`
	Email             string `json:"email,omitempty"`
	TemporaryPassword string `json:"temporary_password,omitempty"`
	Role              string `json:"role,omitempty"`
}

type setupBootstrapResponse struct {
	Status               setupStatusResponse        `json:"status"`
	Details              map[string]int             `json:"details,omitempty"`
	GeneratedSecrets     []string                   `json:"generated_secrets,omitempty"`
	RequiresRestart      bool                       `json:"requires_restart,omitempty"`
	TemporaryCredentials []setupTemporaryCredential `json:"temporary_credentials,omitempty"`
	Messages             []string                   `json:"messages,omitempty"`
}

type setupTemplatesResponse struct {
	Profile string            `json:"profile"`
	Files   map[string]string `json:"files"`
}

type setupTemplateOptions struct {
	RepositoryGroups []setupRepositoryGroupInput
	Users            []setupUserInput
	IncludeLLM       bool
	IncludeMCP       bool
	LLMProfile       setupLLMProfileInput
}

func (req setupBootstrapRequest) shouldSeedLLMProfile() bool {
	return req.SeedLLMProfile == nil || *req.SeedLLMProfile
}

func ensureSetupSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS setup_state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	return err
}

func (a *App) handleGetSetupStatus(w http.ResponseWriter, r *http.Request) {
	status, err := a.buildSetupStatus(r.Context())
	if err != nil {
		http.Error(w, "failed to build setup status", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (a *App) handleGetSetupTemplates(w http.ResponseWriter, r *http.Request) {
	profile := normalizeSetupProfile(r.URL.Query().Get("profile"))
	repositories := setupRepositoriesFromQuery(r.URL.Query())
	options := setupTemplateOptionsFromQuery(r.URL.Query())
	writeJSON(w, http.StatusOK, setupTemplatesResponse{
		Profile: profile,
		Files:   setupStarterTemplatesWithOptions(profile, repositories, options),
	})
}

func (a *App) handleDownloadSetupTemplates(w http.ResponseWriter, r *http.Request) {
	profile := normalizeSetupProfile(r.URL.Query().Get("profile"))
	repositories := setupRepositoriesFromQuery(r.URL.Query())
	options := setupTemplateOptionsFromQuery(r.URL.Query())
	files := setupStarterTemplatesWithOptions(profile, repositories, options)

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="nopsai-gitops-starter.zip"`)
	archive := zip.NewWriter(w)
	paths := make([]string, 0, len(files))
	for filePath := range files {
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	for _, filePath := range paths {
		cleanPath := strings.TrimPrefix(path.Clean("/"+filePath), "/")
		if cleanPath == "." || cleanPath == "" {
			continue
		}
		entry, err := archive.Create(cleanPath)
		if err != nil {
			_ = archive.Close()
			return
		}
		if _, err := entry.Write([]byte(files[filePath])); err != nil {
			_ = archive.Close()
			return
		}
	}
	_ = archive.Close()
}

func (a *App) handleBootstrapSetup(w http.ResponseWriter, r *http.Request) {
	var req setupBootstrapRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	req.Profile = normalizeSetupProfile(req.Profile)
	req.Repositories = normalizeSetupRepositories(req.Repositories)
	req.RepositoryGroups = normalizeSetupRepositoryGroups(req.RepositoryGroups, req.Repositories)
	req.Repositories = setupRepositoriesFromGroups(req.RepositoryGroups)

	if err := a.validateSetupBootstrapRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	actor := actorIDFromRequest(r)
	details := map[string]int{}
	var messages []string
	var generatedSecrets []string
	requiresRestart := false
	var credentials []setupTemporaryCredential

	if req.GenerateSecrets {
		names, restart, err := a.generateSetupSecrets()
		if err != nil {
			http.Error(w, "failed to generate local secrets", http.StatusInternalServerError)
			return
		}
		generatedSecrets = names
		requiresRestart = restart
		if len(names) > 0 {
			messages = append(messages, fmt.Sprintf("Generated %d service secret value(s).", len(names)))
		}
	}

	if req.ConfigRepository != nil && strings.TrimSpace(req.ConfigRepository.RepoURL) != "" {
		input, err := buildConfigRepositoryInput(upsertConfigRepositoryRequest{
			RepoURL:  req.ConfigRepository.RepoURL,
			Branch:   req.ConfigRepository.Branch,
			BasePath: req.ConfigRepository.BasePath,
			Enabled:  req.ConfigRepository.Enabled,
		}, models.ConfigRepositoryScopeSystem, models.ConfigRepositorySystemGlobalID, actor)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		repo, err := a.store.CreateOrUpdateConfigRepository(r.Context(), input)
		if err != nil {
			writeConfigRepositoryStoreError(w, err, "failed to save global config repository")
			return
		}
		details["config_repositories_saved"] = 1
		messages = append(messages, "Global config repository saved.")
		if req.SyncConfigRepository && repo.Enabled {
			startedAt := time.Now()
			if err := a.store.UpdateConfigRepositorySyncStatus(r.Context(), repo.ID, "running", "Configuration synchronization started.", repo.LastSyncCommitSHA, &startedAt, nil); err != nil {
				writeConfigRepositoryStoreError(w, err, "failed to update sync status")
				return
			}
			repo.LastSyncStatus = "running"
			repo.LastSyncStartedAt = &startedAt
			go a.syncConfigRepository(context.Background(), repo, startedAt)
			messages = append(messages, "Global config repository sync started.")
		}
	}

	if req.Profile != setupProfileEmpty && req.SeedStarterDatabase {
		seedDetails, err := a.seedStarterDatabase(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for key, value := range seedDetails {
			details[key] += value
		}
		messages = append(messages, "Starter workspace resources seeded.")
	}

	if req.Profile != setupProfileEmpty && req.Profile != setupProfileProduction && req.shouldSeedLLMProfile() {
		if count, err := a.seedSetupLLMProfile(r.Context(), req.LLMProfile); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else if count > 0 {
			details["llm_profiles_seeded"] += count
		}
	}

	if req.MCPExamples && req.Profile != setupProfileProduction && req.Profile != setupProfileEmpty {
		if count, err := a.seedSetupMCPExamples(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else if count > 0 {
			details["mcp_examples_seeded"] += count
		}
	}

	if len(req.Users) > 0 && req.Profile != setupProfileProduction && req.Profile != setupProfileEmpty {
		created, err := a.seedSetupUsers(r.Context(), req.Users, req.Profile, actor)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		credentials = created
		details["users_seeded"] += len(created)
	}

	if err := a.markSetupComplete(r.Context(), req.Profile); err != nil {
		http.Error(w, "failed to save setup completion state", http.StatusInternalServerError)
		return
	}

	status, err := a.buildSetupStatus(r.Context())
	if err != nil {
		http.Error(w, "setup completed but status could not be loaded", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, setupBootstrapResponse{
		Status:               status,
		Details:              details,
		GeneratedSecrets:     generatedSecrets,
		RequiresRestart:      requiresRestart,
		TemporaryCredentials: credentials,
		Messages:             messages,
	})
}

func (a *App) validateSetupBootstrapRequest(req setupBootstrapRequest) error {
	switch req.Profile {
	case setupProfileDev, setupProfileTeam, setupProfileProduction, setupProfileEmpty:
	default:
		return fmt.Errorf("profile must be dev, team, production, or empty")
	}
	if req.Profile == setupProfileProduction {
		if !req.ProductionAcknowledged {
			return fmt.Errorf("production setup requires acknowledgement of the guardrails")
		}
		if req.SeedStarterDatabase {
			return fmt.Errorf("production setup must seed through a GitOps config repository, not direct database starter resources")
		}
		if req.ConfigRepository == nil || strings.TrimSpace(req.ConfigRepository.RepoURL) == "" {
			return fmt.Errorf("production setup requires a global config repository")
		}
		if req.MCPExamples || len(req.Users) > 0 {
			return fmt.Errorf("production setup must manage MCP examples and users through GitOps manifests")
		}
	}
	if req.Profile == setupProfileEmpty {
		if req.SeedStarterDatabase {
			return fmt.Errorf("empty setup cannot seed starter database resources")
		}
		if req.MCPExamples || len(req.Users) > 0 {
			return fmt.Errorf("empty setup cannot seed MCP examples or users")
		}
	}
	return nil
}

func (a *App) buildSetupStatus(ctx context.Context) (setupStatusResponse, error) {
	state, err := a.loadSetupState(ctx)
	if err != nil {
		return setupStatusResponse{}, err
	}
	counts, err := a.setupCounts(ctx)
	if err != nil {
		return setupStatusResponse{}, err
	}

	var globalRepo *models.ConfigRepository
	if repo, err := a.store.GetConfigRepositoryByScope(ctx, models.ConfigRepositoryScopeSystem, models.ConfigRepositorySystemGlobalID); err == nil {
		copy := repo
		globalRepo = &copy
	} else if !errors.Is(err, pgx.ErrNoRows) {
		// store adapters may wrap not-found; the health check below keeps the user-facing status useful.
	}

	completedAt := strings.TrimSpace(state[setupStateKeyCompletedAt])
	resp := setupStatusResponse{
		Completed:        completedAt != "",
		CompletedAt:      completedAt,
		Profile:          strings.TrimSpace(state[setupStateKeyProfile]),
		RuntimeEnv:       setupRuntimeEnvironment(),
		EnvFilePath:      a.envFilePath,
		Counts:           counts,
		StarterProfiles:  setupProfiles(),
		GitHub:           a.setupGitHubInfo(),
		GlobalConfigRepo: globalRepo,
	}
	resp.Checks = a.setupHealthChecks(ctx, counts, globalRepo, resp.GitHub)
	return resp, nil
}

func (a *App) setupHealthChecks(ctx context.Context, counts setupCounts, globalRepo *models.ConfigRepository, github setupGitHubInfo) []setupCheck {
	checks := []setupCheck{}
	add := func(id, label, status, message string, blocking bool) {
		checks = append(checks, setupCheck{ID: id, Label: label, Status: status, Message: message, Blocking: blocking})
	}

	if a.db == nil {
		add("database", "Database", "error", "Database pool is unavailable.", true)
		add("admin", "Admin bootstrap", "error", "Database pool is unavailable.", true)
	} else if err := a.db.Ping(ctx); err != nil {
		add("database", "Database", "error", "Database is not reachable.", true)
		add("admin", "Admin bootstrap", "error", "Database is not reachable.", true)
	} else {
		add("database", "Database", "success", "Connected.", true)
		adminStatus, adminMessage, adminBlocking := a.setupAdminStatus(ctx)
		add("admin", "Admin bootstrap", adminStatus, adminMessage, adminBlocking)
	}

	cfg := a.getConfigSnapshot()
	secretReady := strings.TrimSpace(cfg.MasterKey) != "" && strings.TrimSpace(cfg.JWTSigningKey) != "" && strings.TrimSpace(cfg.EffectiveServiceJWTSigningKey()) != ""
	switch {
	case !secretReady:
		add("secrets", "Local secrets", "warning", "One or more local signing/encryption values are missing or only available after restart.", true)
	default:
		add("secrets", "Local secrets", "success", "Encryption and signing values are configured.", true)
	}

	if globalRepo == nil {
		add("config_repo", "Global config repo", "warning", "No global GitOps config repository is connected.", false)
	} else if !globalRepo.Enabled {
		add("config_repo", "Global config repo", "warning", "Global config repository is saved but disabled.", false)
	} else {
		add("config_repo", "Global config repo", "success", "Global config repository is connected.", false)
	}

	if github.AppIDConfigured && github.InstallationIDConfigured && github.PrivateKeyConfigured && github.WebhookSecretConfigured {
		add("github_app", "GitHub App", "success", "GitHub App settings are configured.", false)
	} else {
		add("github_app", "GitHub App", "warning", "GitHub App settings are incomplete.", false)
	}

	if github.GitBotURLConfigured && github.NopsaiForwardURLConfigured && github.WebhookSecretConfigured {
		add("git_bot", "git-bot configuration", "success", "git-bot service URL, forward URL, and webhook secret are configured.", false)
	} else {
		add("git_bot", "git-bot configuration", "warning", "git-bot URL, forward URL, or webhook secret is missing.", false)
	}

	switch {
	case counts.AccessGrants > 0:
		add("access", "Access bootstrap", "success", "Workspace access grants are present.", true)
	case counts.Users > 0:
		add("access", "Access bootstrap", "warning", "Users exist, but product access grants have not been seeded.", false)
	default:
		add("access", "Access bootstrap", "warning", "Only the default administrator is available.", false)
	}

	if counts.LLMProfiles > 0 {
		add("llm", "LLM profile", "success", "At least one LLM profile is configured.", false)
	} else {
		add("llm", "LLM profile", "warning", "No LLM profile is configured yet.", false)
	}
	if counts.MCPProfiles > 0 || counts.MCPServers > 0 {
		add("mcp", "MCP examples", "success", "MCP registry entries are present.", false)
	} else {
		add("mcp", "MCP examples", "info", "No MCP examples are enabled.", false)
	}
	if counts.Pipelines > 0 && counts.Steps > 0 {
		add("demo_pipeline", "Demo pipeline", "success", "Starter pipeline and reusable steps are available.", false)
	} else {
		add("demo_pipeline", "Demo pipeline", "warning", "No starter pipeline has been seeded.", false)
	}

	if a.dispatcher == nil {
		add("runner", "Runner health", "warning", "Dispatcher client is unavailable.", false)
	} else {
		statusCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		status, err := a.dispatcher.GetStatus(statusCtx, &emptypb.Empty{})
		if err != nil {
			add("runner", "Runner health", "warning", "Dispatcher status is unavailable.", false)
		} else if len(status.GetRunners()) == 0 {
			add("runner", "Runner health", "warning", "No runners have checked in.", false)
		} else {
			add("runner", "Runner health", "success", fmt.Sprintf("%d runner(s) are connected.", len(status.GetRunners())), false)
		}
	}

	return checks
}

func (a *App) setupAdminStatus(ctx context.Context) (status, message string, blocking bool) {
	var (
		passwordHash sql.NullString
		mustChange   bool
		statusValue  string
	)
	err := a.db.QueryRow(ctx, `
		SELECT password_hash, must_change_password, status
		FROM users
		WHERE sub = $1 AND provider = 'local'
	`, defaultAdminSub).Scan(&passwordHash, &mustChange, &statusValue)
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		return "success", "Default admin account is not present.", false
	}
	if err != nil {
		return "error", "Default admin state could not be loaded.", true
	}
	if !strings.EqualFold(statusValue, "active") {
		return "warning", "Default admin account is not active.", false
	}
	if mustChange {
		return "warning", "Default admin must change password before setup can continue.", true
	}
	if passwordHash.Valid && auth.ComparePassword(passwordHash.String, "admin") == nil {
		return "error", "Default admin still uses the insecure admin password.", true
	}
	return "success", "Default administrator is secured.", true
}

func (a *App) setupCounts(ctx context.Context) (setupCounts, error) {
	var counts setupCounts
	var err error
	if counts.Users, err = a.countRows(ctx, "SELECT COUNT(*) FROM users"); err != nil {
		return counts, err
	}
	if counts.Pipelines, err = a.countRows(ctx, "SELECT COUNT(*) FROM pipelines"); err != nil {
		return counts, err
	}
	if counts.Steps, err = a.countRows(ctx, "SELECT COUNT(*) FROM steps"); err != nil {
		return counts, err
	}
	if counts.Triggers, err = a.countRows(ctx, "SELECT COUNT(*) FROM triggers"); err != nil {
		return counts, err
	}
	if counts.Groups, err = a.countRows(ctx, "SELECT COUNT(*) FROM groups"); err != nil {
		return counts, err
	}
	if counts.AccessGrants, err = a.countRows(ctx, "SELECT COUNT(*) FROM access_grants"); err != nil {
		return counts, err
	}
	if counts.LLMProfiles, err = a.countRows(ctx, "SELECT COUNT(*) FROM llm_profiles"); err != nil {
		return counts, err
	}
	if counts.MCPServers, err = a.countRows(ctx, "SELECT COUNT(*) FROM mcp_servers"); err != nil {
		return counts, err
	}
	if counts.MCPProfiles, err = a.countRows(ctx, "SELECT COUNT(*) FROM mcp_profiles"); err != nil {
		return counts, err
	}
	if counts.KnowledgeContexts, err = a.countRows(ctx, "SELECT COUNT(*) FROM knowledge_contexts"); err != nil {
		return counts, err
	}
	if counts.ConfigRepositories, err = a.countRows(ctx, "SELECT COUNT(*) FROM config_repositories"); err != nil {
		return counts, err
	}
	return counts, nil
}

func (a *App) countRows(ctx context.Context, query string) (int, error) {
	var count int
	if a == nil || a.db == nil {
		return 0, fmt.Errorf("database unavailable")
	}
	if err := a.db.QueryRow(ctx, query).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (a *App) setupGitHubInfo() setupGitHubInfo {
	cfg := a.getConfigSnapshot()
	gitBotServiceURL := strings.TrimRight(strings.TrimSpace(cfg.NopsaiGitBotAPIURL), "/")
	nopsaiAPIURL := strings.TrimRight(strings.TrimSpace(cfg.GitBotNopsaiAPIURL), "/")
	webhookURL := ""
	if gitBotServiceURL != "" {
		if joined, err := url.JoinPath(gitBotServiceURL, "webhook"); err == nil {
			webhookURL = joined
		}
	}
	return setupGitHubInfo{
		WebhookURL:       webhookURL,
		GitBotServiceURL: gitBotServiceURL,
		NopsaiAPIURL:     nopsaiAPIURL,
		RequiredEvents: []string{
			"push",
			"pull_request",
			"check_run",
			"check_suite",
			"ping",
		},
		RequiredPermissions: map[string]string{
			"contents":      "read_and_write",
			"metadata":      "read",
			"pull_requests": "read",
			"checks":        "read_and_write",
		},
		AppIDConfigured:            strings.TrimSpace(cfg.GitHubAppID) != "",
		InstallationIDConfigured:   strings.TrimSpace(cfg.GitHubInstallID) != "",
		PrivateKeyConfigured:       strings.TrimSpace(cfg.GitHubPrivateKeyPath) != "" || strings.TrimSpace(cfg.GitHubPrivateKey) != "",
		WebhookSecretConfigured:    strings.TrimSpace(cfg.GitHubWebhookSecret) != "",
		GitBotURLConfigured:        gitBotServiceURL != "",
		NopsaiForwardURLConfigured: nopsaiAPIURL != "",
	}
}

func (a *App) loadSetupState(ctx context.Context) (map[string]string, error) {
	rows, err := a.db.Query(ctx, `SELECT key, value FROM setup_state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	state := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		state[key] = value
	}
	return state, rows.Err()
}

func (a *App) markSetupComplete(ctx context.Context, profile string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := a.db.Exec(ctx, `
		INSERT INTO setup_state (key, value, updated_at)
		VALUES ($1, $2, NOW()), ($3, $4, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`, setupStateKeyCompletedAt, now, setupStateKeyProfile, normalizeSetupProfile(profile))
	return err
}

func (a *App) generateSetupSecrets() ([]string, bool, error) {
	cfg := a.getConfigSnapshot()
	updates := map[string]string{}
	addIfEmpty := func(current, envKey string, bytes int) error {
		if strings.TrimSpace(current) != "" {
			return nil
		}
		value, err := randomSecret(bytes)
		if err != nil {
			return err
		}
		updates[envKey] = value
		return nil
	}
	for _, candidate := range []struct {
		current string
		envKey  string
		bytes   int
	}{
		{cfg.JWTSigningKey, "JWT_SIGNING_KEY", 48},
		{cfg.ServiceJWTSigningKey, "SERVICE_JWT_SIGNING_KEY", 48},
		{cfg.AAASharedToken, "AAA_SHARED_INTERNAL_TOKEN", 32},
		{cfg.GitHubWebhookSecret, "GITHUB_WEBHOOK_SECRET", 32},
		{cfg.DispatcherTLSSecret, "DISPATCHER_TLS_SECRET", 48},
	} {
		if err := addIfEmpty(candidate.current, candidate.envKey, candidate.bytes); err != nil {
			return nil, false, err
		}
	}
	if len(updates) == 0 {
		return nil, false, nil
	}
	if err := writeEnvFile(a.envFilePath, updates); err != nil {
		return nil, false, err
	}

	a.cfgMu.Lock()
	if value := updates["JWT_SIGNING_KEY"]; value != "" {
		a.cfg.JWTSigningKey = value
	}
	if value := updates["SERVICE_JWT_SIGNING_KEY"]; value != "" {
		a.cfg.ServiceJWTSigningKey = value
	}
	if value := updates["AAA_SHARED_INTERNAL_TOKEN"]; value != "" {
		a.cfg.AAASharedToken = value
	}
	if value := updates["GITHUB_WEBHOOK_SECRET"]; value != "" {
		a.cfg.GitHubWebhookSecret = value
	}
	if value := updates["DISPATCHER_TLS_SECRET"]; value != "" {
		a.cfg.DispatcherTLSSecret = value
	}
	a.cfgMu.Unlock()

	names := make([]string, 0, len(updates))
	for key := range updates {
		names = append(names, key)
	}
	sort.Strings(names)
	return names, true, nil
}

func (a *App) seedStarterDatabase(ctx context.Context, req setupBootstrapRequest) (map[string]int, error) {
	details := map[string]int{
		"run_groups_created": 0,
		"run_groups_updated": 0,
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := a.syncPipelineRunGroups(ctx, tx, setupPipelineRunStructure(req.Profile, req.RepositoryGroups, req.Repositories), details); err != nil {
		return nil, err
	}

	stepDefinition := setupReusableStepYAML()
	var step models.PipelineStep
	if err := yaml.Unmarshal([]byte(stepDefinition), &step); err != nil {
		return nil, fmt.Errorf("starter step is invalid: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO steps (path, name, definition, source, updated_at)
		VALUES ($1, $2, $3, 'setup', NOW())
		ON CONFLICT (path, name) DO UPDATE SET definition = EXCLUDED.definition, source = 'setup', updated_at = NOW()
	`, "setup", "announce", stepDefinition); err != nil {
		return nil, fmt.Errorf("seed starter step: %w", err)
	}
	details["steps_seeded"]++

	pipelineDefinition := setupFirstRunPipelineYAML(req.Profile)
	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(pipelineDefinition), &pipeline); err != nil {
		return nil, fmt.Errorf("starter pipeline is invalid: %w", err)
	}
	if err := validatePipeline(&pipeline); err != nil {
		return nil, fmt.Errorf("starter pipeline validation failed: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO pipelines (path, name, version, definition, source, updated_at)
		VALUES ($1, $2, $3, $4, 'setup', NOW())
		ON CONFLICT (path, name) DO UPDATE SET version = EXCLUDED.version, definition = EXCLUDED.definition, source = 'setup', updated_at = NOW()
	`, "setup", pipeline.Name, normalizePipelineVersion(pipeline.Version), pipelineDefinition); err != nil {
		return nil, fmt.Errorf("seed starter pipeline: %w", err)
	}
	details["pipelines_seeded"]++

	for _, repo := range req.Repositories {
		definition := setupTriggerYAML(req.Profile)
		var manifest models.Manifest
		if err := yaml.Unmarshal([]byte(definition), &manifest); err != nil {
			return nil, fmt.Errorf("starter trigger for %s is invalid: %w", repo, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO triggers (repository_name, trigger_definition, source)
			VALUES ($1, $2, 'setup')
			ON CONFLICT (repository_name) DO UPDATE SET trigger_definition = EXCLUDED.trigger_definition, source = 'setup'
		`, repo, definition); err != nil {
			return nil, fmt.Errorf("seed trigger for %s: %w", repo, err)
		}
		details["triggers_seeded"]++
	}

	for scope, values := range setupScopeVariables(req.Profile) {
		for name, value := range values {
			if _, err := tx.Exec(ctx, `
				INSERT INTO variables (name, value, repository_name, scope, source, updated_at)
				VALUES ($1, $2, NULL, $3, 'setup', NOW())
				ON CONFLICT (name, repository_name, scope) DO UPDATE SET value = EXCLUDED.value, source = 'setup', updated_at = NOW()
			`, name, value, runtimeScopeForStorage(scope)); err != nil {
				return nil, fmt.Errorf("seed variable %s: %w", name, err)
			}
			details["variables_seeded"]++
		}
	}

	knowledge := setupKnowledgeContexts(req.Profile)
	for _, item := range knowledge {
		if _, err := tx.Exec(ctx, `
			INSERT INTO knowledge_contexts (kind, group_path, name, description, content, source, updated_at)
			VALUES ($1, $2, $3, $4, $5, 'setup', NOW())
			ON CONFLICT (kind, group_path, name) DO UPDATE SET
				description = EXCLUDED.description,
				content = EXCLUDED.content,
				source = 'setup',
				updated_at = NOW()
		`, item.kind, item.group, item.name, item.description, item.content); err != nil {
			return nil, fmt.Errorf("seed knowledge context %s/%s/%s: %w", item.kind, item.group, item.name, err)
		}
		details["knowledge_contexts_seeded"]++
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return details, nil
}

func (a *App) seedSetupLLMProfile(ctx context.Context, input setupLLMProfileInput) (int, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = config.DefaultLLMProfileName
	}
	provider := strings.TrimSpace(input.Provider)
	if provider == "" {
		provider = config.LLMProviderLMStudio
	}
	modelName := strings.TrimSpace(input.Model)
	if modelName == "" {
		modelName = "qwen3-coder"
	}
	baseURL := strings.TrimSpace(input.BaseURL)
	if baseURL == "" && config.NormalizeLLMProvider(provider) == config.LLMProviderLMStudio {
		baseURL = "http://lmstudio:1234"
	}
	apiKeySecret := strings.TrimSpace(input.APIKeySecret)
	if apiKeySecret == "" && config.NormalizeLLMProvider(provider) == config.LLMProviderGemini {
		apiKeySecret = "GEMINI_API_KEY"
	}
	allowedScopes := models.NormalizeScopeList(input.AllowedScopes)
	if len(allowedScopes) == 0 {
		allowedScopes = []string{"dev", "prod"}
	}

	cfg := a.getConfigSnapshot()
	profiles := cfg.EffectiveLLMProfiles()
	if profiles == nil {
		profiles = map[string]config.LLMProfile{}
	}
	_, existed := profiles[name]
	profiles[name] = config.NormalizeLLMProfile(config.LLMProfile{
		Provider:      provider,
		Model:         modelName,
		BaseURL:       baseURL,
		APIKeySecret:  apiKeySecret,
		AllowedScopes: allowedScopes,
	})
	cfg.LLMDefaultProfile = name
	cfg.LLMProfiles = profiles
	a.cfgMu.Lock()
	a.cfg.LLMDefaultProfile = cfg.LLMDefaultProfile
	a.cfg.LLMProfiles = cfg.LLMProfiles
	a.cfgMu.Unlock()

	if err := a.persistLLMProfilesConfig(ctx, cfg); err != nil {
		return 0, err
	}
	if apiKeySecret != "" && strings.TrimSpace(input.APIKeyValue) != "" {
		encrypted, err := a.encrypt(strings.TrimSpace(input.APIKeyValue))
		if err != nil {
			return 0, err
		}
		if _, err := a.db.Exec(ctx, `
			INSERT INTO secrets (name, value, repository_name, scope)
			VALUES ($1, $2, NULL, 'default')
			ON CONFLICT (name, repository_name, scope) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
		`, apiKeySecret, encrypted); err != nil {
			return 0, err
		}
	}
	if existed {
		return 0, nil
	}
	return 1, nil
}

func (a *App) seedSetupMCPExamples(ctx context.Context) (int, error) {
	cfg := a.getConfigSnapshot()
	servers := cfg.EffectiveMCPServers()
	if servers == nil {
		servers = map[string]models.MCPServer{}
	}
	profiles := cfg.EffectiveMCPProfiles()
	if profiles == nil {
		profiles = map[string]models.MCPProfile{}
	}
	count := 0
	if _, exists := servers["github-readonly"]; !exists {
		servers["github-readonly"] = models.MCPServer{
			Name:          "github-readonly",
			DisplayName:   "GitHub MCP Read-only",
			Enabled:       false,
			Provider:      "github",
			Transport:     models.MCPTransportStreamableHTTP,
			URL:           "https://api.githubcopilot.com/mcp/x/all/readonly",
			AuthType:      models.MCPAuthBearerToken,
			AuthSecret:    "GITHUB_MCP_TOKEN",
			Timeout:       models.DefaultMCPTimeout,
			AllowedScopes: []string{"dev", "prod"},
		}
		count++
	}
	if _, exists := profiles["github-readonly"]; !exists {
		profiles["github-readonly"] = models.MCPProfile{
			Name:        "github-readonly",
			Description: "Read-only GitHub tools for setup smoke tests.",
			Enabled:     false,
			ServerRefs: []models.MCPProfileServerRef{{
				ServerName: "github-readonly",
				Tools:      []string{"*"},
			}},
			AllowedScopes: []string{"dev", "prod"},
		}
		count++
	}
	cfg.MCPServers = models.NormalizeMCPServers(servers)
	cfg.MCPProfiles = models.NormalizeMCPProfiles(profiles)
	a.cfgMu.Lock()
	a.cfg.MCPServers = cfg.MCPServers
	a.cfg.MCPProfiles = cfg.MCPProfiles
	a.cfgMu.Unlock()
	if err := a.persistMCPRegistryConfig(ctx, cfg); err != nil {
		return 0, err
	}
	return count, nil
}

func (a *App) seedSetupUsers(ctx context.Context, users []setupUserInput, profile, actor string) ([]setupTemporaryCredential, error) {
	rootFolderID := setupAccessFolder(profile)
	if err := a.ensureSetupRootFolder(ctx, rootFolderID); err != nil {
		return nil, err
	}
	created := []setupTemporaryCredential{}
	for _, input := range users {
		role, err := normalizeProductRoleName(firstNonEmptyString(input.Role, productRoleViewer))
		if err != nil {
			return nil, err
		}
		sub := strings.TrimSpace(input.Sub)
		email := strings.TrimSpace(input.Email)
		if sub == "" {
			sub = email
		}
		if sub == "" {
			continue
		}

		var userID string
		err = a.db.QueryRow(ctx, `
			SELECT id::text FROM users WHERE sub = $1 OR ($2 <> '' AND LOWER(email) = LOWER($2)) LIMIT 1
		`, sub, email).Scan(&userID)
		temporaryPassword := ""
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			temporaryPassword = strings.TrimSpace(input.Password)
			if temporaryPassword == "" {
				var genErr error
				temporaryPassword, genErr = randomSecret(18)
				if genErr != nil {
					return nil, genErr
				}
			}
			hashed, hashErr := auth.HashPassword(temporaryPassword)
			if hashErr != nil {
				return nil, hashErr
			}
			newID := uuid.NewString()
			if _, err := a.db.Exec(ctx, `
				INSERT INTO users (id, sub, email, provider, password_hash, status, must_change_password)
				VALUES ($1, $2, NULLIF($3, ''), 'local', $4, 'active', TRUE)
			`, newID, sub, email, hashed); err != nil {
				return nil, err
			}
			userID = newID
			created = append(created, setupTemporaryCredential{Sub: sub, Email: email, TemporaryPassword: temporaryPassword, Role: role})
		} else if err != nil {
			return nil, err
		}

		resourceType := grantResourceFolder
		resourceID := setupUserAccessFolder(rootFolderID, input.Group)
		if role == productRoleAdmin {
			resourceType = grantResourcePlatform
			resourceID = platformGrantID
		}
		_, err = a.GrantProductRole(ctx, GrantProductRoleInput{
			SubjectType:  grantSubjectUser,
			SubjectID:    userID,
			RoleName:     role,
			ResourceType: resourceType,
			ResourceID:   resourceID,
			Inherit:      true,
			GrantedBy:    actor,
		})
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "grant already exists") {
			return nil, err
		}
	}
	return created, nil
}

func setupUserAccessFolder(root, group string) string {
	root = strings.Trim(strings.TrimSpace(root), "/")
	group = normalizeSetupRepositoryGroupName(group)
	if root == "" {
		return group
	}
	if group == "" {
		return root
	}
	return root + "/" + group
}

func (a *App) ensureSetupRootFolder(ctx context.Context, name string) error {
	name = strings.Trim(strings.TrimSpace(name), "/")
	if name == "" {
		return nil
	}
	_, err := a.db.Exec(ctx, `
		INSERT INTO groups (name, description)
		VALUES ($1, $2)
		ON CONFLICT (name) DO NOTHING
	`, name, "Starter setup workspace")
	return err
}

func setupProfiles() []setupStarterProfile {
	return []setupStarterProfile{
		{ID: setupProfileDev, Label: "Dev", Description: "Local evaluation with starter folders, a smoke pipeline, and generated local values."},
		{ID: setupProfileTeam, Label: "Team", Description: "Shared workspace defaults for owners, developers, viewers, repository triggers, and GitOps handoff."},
		{ID: setupProfileProduction, Label: "Production", Description: "GitOps-first setup with stricter guardrails and no direct database starter seed."},
		{ID: setupProfileEmpty, Label: "Empty", Description: "Only validate prerequisites and leave resources for manual configuration."},
	}
}

func normalizeSetupProfile(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", setupProfileDev:
		return setupProfileDev
	case setupProfileTeam:
		return setupProfileTeam
	case "prod", setupProfileProduction:
		return setupProfileProduction
	case setupProfileEmpty, "blank":
		return setupProfileEmpty
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func setupGitBotURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func setupRuntimeEnvironment() string {
	for _, key := range []string{"NOPSAI_ENV", "APP_ENV", "ENVIRONMENT", "GO_ENV"} {
		if value := strings.TrimSpace(strings.ToLower(os.Getenv(key))); value != "" {
			return value
		}
	}
	return "development"
}

func setupRepositoriesFromQuery(values url.Values) []string {
	var repositories []string
	for _, raw := range values["repository"] {
		repositories = append(repositories, raw)
	}
	for _, raw := range values["repositories"] {
		for _, part := range strings.Split(raw, ",") {
			repositories = append(repositories, part)
		}
	}
	return normalizeSetupRepositories(repositories)
}

func setupTemplateOptionsFromQuery(values url.Values) setupTemplateOptions {
	includeLLM := queryBoolDefault(values, "include_llm", true)
	includeMCP := queryBoolDefault(values, "mcp_examples", true)
	return setupTemplateOptions{
		RepositoryGroups: setupRepositoryGroupsFromQuery(values),
		Users:            setupUsersFromQuery(values),
		IncludeLLM:       includeLLM,
		IncludeMCP:       includeMCP,
		LLMProfile: setupLLMProfileInput{
			Name:         "standard",
			Provider:     values.Get("llm_provider"),
			Model:        values.Get("llm_model"),
			BaseURL:      values.Get("llm_base_url"),
			APIKeySecret: values.Get("llm_api_key_secret"),
			AllowedScopes: []string{
				"dev",
				"prod",
			},
		},
	}
}

func queryBoolDefault(values url.Values, key string, defaultValue bool) bool {
	raw := strings.TrimSpace(strings.ToLower(values.Get(key)))
	if raw == "" {
		return defaultValue
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func setupRepositoryGroupsFromQuery(values url.Values) []setupRepositoryGroupInput {
	var groups []setupRepositoryGroupInput
	for _, raw := range values["repository_group"] {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		name, reposText, found := strings.Cut(raw, ":")
		if !found {
			name, reposText, found = strings.Cut(raw, "=")
		}
		if !found {
			continue
		}
		var repositories []string
		for _, repo := range strings.Split(reposText, ",") {
			repositories = append(repositories, repo)
		}
		groups = append(groups, setupRepositoryGroupInput{
			Name:         name,
			Repositories: repositories,
		})
	}
	return normalizeSetupRepositoryGroups(groups, nil)
}

func setupUsersFromQuery(values url.Values) []setupUserInput {
	var users []setupUserInput
	for _, raw := range values["setup_user"] {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		var user setupUserInput
		if err := json.Unmarshal([]byte(raw), &user); err != nil {
			continue
		}
		user.Sub = strings.TrimSpace(user.Sub)
		user.Email = strings.TrimSpace(user.Email)
		user.Role = strings.TrimSpace(user.Role)
		user.Group = normalizeSetupRepositoryGroupName(user.Group)
		if user.Sub == "" {
			user.Sub = user.Email
		}
		if user.Email == "" {
			user.Email = user.Sub
		}
		if user.Sub == "" {
			continue
		}
		users = append(users, user)
	}
	return users
}

func normalizeSetupRepositories(raw []string) []string {
	seen := map[string]bool{}
	var repositories []string
	for _, value := range raw {
		repo := strings.Trim(strings.TrimSpace(value), "/")
		if repo == "" {
			continue
		}
		repo = strings.ReplaceAll(repo, "\\", "/")
		repo = strings.TrimPrefix(repo, "git@github.com:")
		repo = strings.TrimPrefix(repo, "https://github.com/")
		repo = strings.TrimPrefix(repo, "http://github.com/")
		repo = strings.TrimPrefix(repo, "github.com/")
		repo = strings.TrimSuffix(repo, ".git")
		parts := strings.Split(repo, "/")
		if len(parts) < 2 {
			continue
		}
		repo = strings.Trim(parts[0], "/") + "/" + strings.Trim(parts[1], "/")
		if strings.Contains(repo, "..") || seen[repo] {
			continue
		}
		seen[repo] = true
		repositories = append(repositories, repo)
	}
	sort.Strings(repositories)
	return repositories
}

func normalizeSetupRepositoryGroups(raw []setupRepositoryGroupInput, legacyRepositories []string) []setupRepositoryGroupInput {
	if len(raw) == 0 && len(legacyRepositories) > 0 {
		raw = []setupRepositoryGroupInput{{
			Name:         "applications",
			Repositories: legacyRepositories,
		}}
	}

	groupsByName := map[string][]string{}
	var groupOrder []string
	for _, group := range raw {
		name := normalizeSetupRepositoryGroupName(group.Name)
		if name == "" {
			continue
		}
		if _, exists := groupsByName[name]; !exists {
			groupOrder = append(groupOrder, name)
		}
		groupsByName[name] = append(groupsByName[name], normalizeSetupRepositories(group.Repositories)...)
	}
	if len(groupsByName) == 0 {
		return nil
	}

	result := make([]setupRepositoryGroupInput, 0, len(groupOrder))
	seenRepositories := map[string]bool{}
	for _, name := range groupOrder {
		repositories := normalizeSetupRepositories(groupsByName[name])
		filtered := make([]string, 0, len(repositories))
		for _, repo := range repositories {
			key := strings.ToLower(repo)
			if seenRepositories[key] {
				continue
			}
			seenRepositories[key] = true
			filtered = append(filtered, repo)
		}
		result = append(result, setupRepositoryGroupInput{Name: name, Repositories: filtered})
	}
	return result
}

func normalizeSetupRepositoryGroupName(raw string) string {
	name := strings.Trim(strings.TrimSpace(raw), "/")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.Join(strings.Fields(name), "-")
	if name == "" || strings.Contains(name, "..") {
		return ""
	}
	return name
}

func setupRepositoriesFromGroups(groups []setupRepositoryGroupInput) []string {
	var repositories []string
	for _, group := range groups {
		repositories = append(repositories, group.Repositories...)
	}
	return normalizeSetupRepositories(repositories)
}

func setupStarterTemplates(profile string, repositories []string) map[string]string {
	return setupStarterTemplatesWithOptions(profile, repositories, setupTemplateOptions{
		IncludeLLM: true,
		IncludeMCP: true,
		LLMProfile: setupLLMProfileInput{
			Name:          "standard",
			Provider:      config.LLMProviderLMStudio,
			Model:         "qwen3-coder",
			BaseURL:       "http://lmstudio:1234",
			AllowedScopes: []string{"dev", "prod"},
		},
	})
}

func setupStarterTemplatesWithOptions(profile string, repositories []string, options setupTemplateOptions) map[string]string {
	profile = normalizeSetupProfile(profile)
	repositoryGroups := normalizeSetupRepositoryGroups(options.RepositoryGroups, repositories)
	if len(repositoryGroups) > 0 {
		repositories = setupRepositoriesFromGroups(repositoryGroups)
	} else {
		repositories = normalizeSetupRepositories(repositories)
		repositoryGroups = normalizeSetupRepositoryGroups(nil, repositories)
	}
	files := map[string]string{
		"README.md":                                 setupReadme(profile),
		"pipelines/setup/first-run.yaml":            setupFirstRunPipelineYAML(profile),
		"steps/setup/announce.yaml":                 setupReusableStepYAML(),
		"scopes/dev/scope.yaml":                     setupScopeYAML(profile, "dev"),
		"knowledge/guideline/platform/setup-run.md": setupKnowledgeMarkdown(profile),
		"access/bootstrap.yaml":                     setupAccessYAML(profile, repositoryGroups, options.Users),
		"config-repositories/groups/structure.yaml": setupConfigRepositoryStructureYAML(repositoryGroups, repositories),
	}
	if options.IncludeLLM {
		files["setting/system/llm_profile.yaml"] = setupLLMProfileYAML(options.LLMProfile)
	}
	if options.IncludeMCP {
		files["setting/system/mcp.yaml"] = setupMCPYAML()
	}
	if profile == setupProfileTeam || profile == setupProfileProduction {
		files["scopes/prod/scope.yaml"] = setupScopeYAML(profile, "prod")
	}
	for _, repo := range repositories {
		files["triggers/"+repo+".yaml"] = setupTriggerYAML(profile)
	}
	return files
}

func setupPipelineRunStructure(profile string, repositoryGroups []setupRepositoryGroupInput, repositories []string) map[string]*pipelineRunStructureNode {
	root := setupAccessFolder(profile)
	node := &pipelineRunStructureNode{
		Description: "Starter workspace",
		Children:    map[string]*pipelineRunStructureNode{},
	}
	repositoryGroups = normalizeSetupRepositoryGroups(repositoryGroups, repositories)
	for _, group := range repositoryGroups {
		child := &pipelineRunStructureNode{Description: "Repository group " + group.Name, Children: map[string]*pipelineRunStructureNode{}}
		for _, repo := range group.Repositories {
			parts := strings.Split(repo, "/")
			if len(parts) != 2 {
				continue
			}
			child.Repos = append(child.Repos, repo)
		}
		node.Children[group.Name] = child
	}
	return map[string]*pipelineRunStructureNode{root: node}
}

func setupAccessFolder(profile string) string {
	switch normalizeSetupProfile(profile) {
	case setupProfileTeam:
		return "workspace"
	case setupProfileProduction:
		return "production"
	default:
		return "sandbox"
	}
}

func setupScopeVariables(profile string) map[string]map[string]string {
	workspace := setupAccessFolder(profile)
	values := map[string]map[string]string{
		"dev": {
			"NOPSAI_SETUP_WORKSPACE": workspace,
			"NOPSAI_SETUP_SCOPE":     "dev",
		},
	}
	if profile == setupProfileTeam || profile == setupProfileProduction {
		values["prod"] = map[string]string{
			"NOPSAI_SETUP_WORKSPACE": workspace,
			"NOPSAI_SETUP_SCOPE":     "prod",
		}
	}
	return values
}

type setupKnowledgeContextSeed struct {
	kind        string
	group       string
	name        string
	description string
	content     string
}

func setupKnowledgeContexts(profile string) []setupKnowledgeContextSeed {
	return []setupKnowledgeContextSeed{{
		kind:        "guideline",
		group:       "platform",
		name:        "setup-run",
		description: "Starter pipeline run expectations",
		content:     fmt.Sprintf("Use the starter pipeline in %s to verify runner connectivity, log streaming, and optional LLM execution before attaching production repositories.", setupAccessFolder(profile)),
	}}
}

func setupReusableStepYAML() string {
	return strings.TrimSpace(`
name: announce
image: alpine:3.20
script: |
  #!/bin/sh
  set -e
  echo "Starting NopsAI setup pipeline"
`) + "\n"
}

func setupFirstRunPipelineYAML(profile string) string {
	scope := "dev"
	if normalizeSetupProfile(profile) == setupProfileProduction {
		scope = "prod"
	}
	return strings.TrimSpace(fmt.Sprintf(`
name: first-run
version: "1.0.0"
description: Verifies that NopsAI can run a starter job, stream logs, and optionally call the configured LLM profile.
container_image: alpine:3.20
working_directory: /workspace
timeout: 10m
llm_profile: standard
display_options:
  github_view: flat
variables:
  - %s:NOPSAI_SETUP_WORKSPACE
  - %s:NOPSAI_SETUP_SCOPE
steps:
  - name: announce
    include: step:setup/announce

  - name: runner-smoke
    image: alpine:3.20
    script: |
      #!/bin/sh
      set -e
      echo "NopsAI runner is executing the setup smoke test"
      echo "workspace=$NOPSAI_SETUP_WORKSPACE scope=$NOPSAI_SETUP_SCOPE"
    depends_on:
      - announce

  - name: ai-smoke
    goal: Return one short sentence confirming that the NopsAI setup smoke test reached the agent.
    ignore_failure: true
    depends_on:
      - runner-smoke
`, scope, scope)) + "\n"
}

func setupTriggerYAML(profile string) string {
	scope := "dev"
	if normalizeSetupProfile(profile) == setupProfileProduction {
		scope = "prod"
	}
	return strings.TrimSpace(fmt.Sprintf(`
triggers:
  - on: push
    branches:
      - main
    scope: %s
    pipelines:
      - setup/first-run

  - on: pull_request
    scope: %s
    pipelines:
      - setup/first-run
`, scope, scope)) + "\n"
}

func setupScopeYAML(profile, scope string) string {
	return strings.TrimSpace(fmt.Sprintf(`
variables:
  NOPSAI_SETUP_WORKSPACE: %s
  NOPSAI_SETUP_SCOPE: %s
secrets:
  GEMINI_API_KEY:
`, setupAccessFolder(profile), scope)) + "\n"
}

func setupReadme(profile string) string {
	return fmt.Sprintf("# NopsAI starter config\n\nWorkspace: `%s`\n\nThis repository contains starter resources for the first NopsAI workspace bootstrap. Keep plaintext secrets outside this repository. Scope files define plaintext scoped values under `variables:` and may define secret keys under `secrets:` with `null` placeholders or encrypted values generated by this NopsAI instance.\n", setupAccessFolder(profile))
}

func setupKnowledgeMarkdown(profile string) string {
	return fmt.Sprintf("---\ndescription: Starter setup run expectations\n---\n\nUse the %s starter workspace to prove runner connectivity, logs, repository triggers, and optional LLM execution before onboarding production automation.\n", setupAccessFolder(profile))
}

func setupAccessYAML(profile string, repositoryGroups []setupRepositoryGroupInput, users []setupUserInput) string {
	folders := setupAccessGrantFolders(profile, repositoryGroups)
	var builder strings.Builder
	if len(users) == 0 {
		builder.WriteString("users: []\n")
	} else {
		builder.WriteString("users:\n")
		for _, user := range users {
			sub := strings.TrimSpace(firstNonEmptyString(user.Sub, user.Email))
			if sub == "" {
				continue
			}
			email := strings.TrimSpace(firstNonEmptyString(user.Email, sub))
			builder.WriteString(fmt.Sprintf("  - sub: %q\n", sub))
			builder.WriteString(fmt.Sprintf("    email: %q\n", email))
			builder.WriteString("    provider: local\n")
			builder.WriteString("    status: active\n")
			builder.WriteString("    # password is intentionally not generated into GitOps; set it out of band if GitOps creates the account.\n")
		}
	}
	builder.WriteString("\nbasic_roles:\n")
	for _, folder := range folders {
		builder.WriteString("  - user: admin\n")
		builder.WriteString("    role: owner\n")
		builder.WriteString(fmt.Sprintf("    resource: folder:%s\n", folder))
	}
	for _, user := range users {
		sub := strings.TrimSpace(firstNonEmptyString(user.Sub, user.Email))
		if sub == "" {
			continue
		}
		role := strings.TrimSpace(user.Role)
		if role == "" {
			role = productRoleViewer
		}
		if normalizedRole, err := normalizeProductRoleName(role); err == nil {
			role = normalizedRole
		}
		folder := normalizeSetupRepositoryGroupName(user.Group)
		if folder == "" {
			folder = folders[0]
		}
		builder.WriteString(fmt.Sprintf("  - user: %q\n", sub))
		builder.WriteString(fmt.Sprintf("    role: %s\n", role))
		builder.WriteString(fmt.Sprintf("    resource: folder:%s\n", folder))
	}
	return builder.String()
}

func setupAccessGrantFolders(profile string, repositoryGroups []setupRepositoryGroupInput) []string {
	seen := map[string]bool{}
	var folders []string
	for _, group := range repositoryGroups {
		name := normalizeSetupRepositoryGroupName(group.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		folders = append(folders, name)
	}
	if len(folders) == 0 {
		folders = append(folders, setupAccessFolder(profile))
	}
	return folders
}

func setupLLMProfileYAML(input setupLLMProfileInput) string {
	provider := config.NormalizeLLMProvider(input.Provider)
	if provider == "" {
		provider = config.LLMProviderLMStudio
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		if provider == config.LLMProviderGemini {
			model = "gemini-2.5-flash"
		} else {
			model = "qwen3-coder"
		}
	}
	apiKeySecret := strings.TrimSpace(input.APIKeySecret)
	if apiKeySecret == "" && provider == config.LLMProviderGemini {
		apiKeySecret = "GEMINI_API_KEY"
	}
	baseURL := strings.TrimSpace(input.BaseURL)
	if baseURL == "" && provider == config.LLMProviderLMStudio {
		baseURL = "http://lmstudio:1234"
	}

	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(`
default_profile: standard

profiles:
  - name: standard
`) + "\n")
	builder.WriteString(fmt.Sprintf("    provider: %s\n", provider))
	builder.WriteString(fmt.Sprintf("    model: %s\n", model))
	if baseURL != "" {
		builder.WriteString(fmt.Sprintf("    base_url: %s\n", baseURL))
	}
	if apiKeySecret != "" {
		builder.WriteString(fmt.Sprintf("    api_key_secret: %s\n", apiKeySecret))
	}
	builder.WriteString("    allowed_scopes: [\"dev\", \"prod\"]\n")
	return builder.String()
}

func setupMCPYAML() string {
	return strings.TrimSpace(`
mcp_servers:
  github-readonly:
    display_name: GitHub MCP Read-only
    enabled: false
    provider: github
    transport: streamable_http
    url: https://api.githubcopilot.com/mcp/x/all/readonly
    auth_type: bearer_token
    auth_secret: GITHUB_MCP_TOKEN
    timeout: 30s
    allowed_scopes: ["dev", "prod"]

mcp_profiles:
  github-readonly:
    description: Read-only GitHub tools for repository-aware setup tests
    enabled: false
    servers:
      - server: github-readonly
        tools:
          - "*"
    allowed_scopes: ["dev", "prod"]
`) + "\n"
}

func setupConfigRepositoryStructureYAML(repositoryGroups []setupRepositoryGroupInput, repositories []string) string {
	var builder strings.Builder
	repositoryGroups = normalizeSetupRepositoryGroups(repositoryGroups, repositories)
	if len(repositoryGroups) == 0 {
		builder.WriteString("{}\n")
		return builder.String()
	}
	for _, group := range repositoryGroups {
		builder.WriteString(fmt.Sprintf("%s:\n", group.Name))
		builder.WriteString("  description: Repository group\n")
		if len(group.Repositories) == 0 {
			builder.WriteString("  repos: []\n")
			continue
		}
		builder.WriteString("  repos:\n")
		for _, repo := range group.Repositories {
			builder.WriteString(fmt.Sprintf("    - %s\n", repo))
		}
	}
	return builder.String()
}

func randomSecret(size int) (string, error) {
	if size <= 0 {
		size = 32
	}
	buf := make([]byte, size)
	if _, err := crand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

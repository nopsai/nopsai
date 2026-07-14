package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/systemconfig"
	"nopsai/services/nopsai/pkg/auth"
)

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

func setupBootstrapWarnings(req setupBootstrapRequest) []string {
	var warnings []string
	if warning := setupLLMSkippedWarning(req); warning != "" {
		warnings = append(warnings, warning)
	}
	return warnings
}

func setupLLMSkippedWarning(req setupBootstrapRequest) string {
	if req.Profile == setupProfileEmpty || req.Profile == setupProfileProduction || req.shouldSeedLLMProfile() {
		return ""
	}
	return "LLM profile setup was skipped. Pipelines with AI-enabled goal tasks may not work until an LLM profile is configured."
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
		status, err := a.dispatcher.GetStatus(statusCtx)
		if err != nil {
			add("runner", "Runner health", "warning", "Dispatcher status is unavailable.", false)
		} else if len(status.GetRunners()) == 0 {
			add("runner", "Runner health", "warning", "No runners have checked in.", false)
		} else if unreachable := runnerUnreachableCount(status); unreachable > 0 {
			add("runner", "Runner health", "warning", fmt.Sprintf("%d runner(s) have checked in, %d unreachable.", len(status.GetRunners()), unreachable), false)
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
	if counts.Teams, err = a.countRows(ctx, "SELECT COUNT(*) FROM teams"); err != nil {
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
	nopsaiAPIURL := strings.TrimRight(strings.TrimSpace(cfg.EffectiveNopsaiAPIURL()), "/")
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
		PrivateKeyConfigured:       strings.TrimSpace(cfg.GitHubPrivateKeyCredentialRef) != "",
		WebhookSecretConfigured:    strings.TrimSpace(cfg.GitHubWebhookCredentialRef) != "",
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
		{cfg.DispatcherTLSSecret, "DISPATCHER_TLS_SECRET", 48},
	} {
		if err := addIfEmpty(candidate.current, candidate.envKey, candidate.bytes); err != nil {
			return nil, false, err
		}
	}
	if len(updates) == 0 {
		return nil, false, nil
	}
	if err := systemconfig.WriteEnvFile(a.envFilePath, updates); err != nil {
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

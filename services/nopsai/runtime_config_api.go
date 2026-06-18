package nopsai

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/serviceauth"
	"nopsai/services/nopsai/internal/systemconfig"
	"nopsai/services/nopsai/pkg/auth"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

const (
	runtimeConfigWatchTimeout = 30 * time.Second
	runtimeConfigWatchPoll    = 500 * time.Millisecond
)

type runtimeConfigResponse struct {
	Version    int64                                 `json:"version"`
	Service    string                                `json:"service"`
	ReloadMode config.ConfigScope                    `json:"reload_mode"`
	Config     map[string]any                        `json:"config"`
	Metadata   map[string]systemconfig.FieldMetadata `json:"metadata,omitempty"`
}

type runtimeConfigSnapshot struct {
	service    string
	reloadMode config.ConfigScope
	values     map[string]any
	fields     []string
}

func (a *App) handleInternalRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	a.serveInternalRuntimeConfig(w, r, false)
}

func (a *App) handleInternalRuntimeConfigWatch(w http.ResponseWriter, r *http.Request) {
	a.serveInternalRuntimeConfig(w, r, true)
}

func (a *App) serveInternalRuntimeConfig(w http.ResponseWriter, r *http.Request, watch bool) {
	service, ok := canonicalRuntimeConfigService(r.PathValue("service"))
	if !ok {
		http.Error(w, "unknown runtime config service", http.StatusNotFound)
		return
	}
	if !a.authorizeRuntimeConfigService(r, service) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if watch {
		sinceVersion := runtimeConfigWatchVersion(r)
		if sinceVersion > 0 {
			if err := a.waitForRuntimeConfigVersionChange(r.Context(), sinceVersion); err != nil {
				if r.Context().Err() != nil {
					return
				}
				log.Warn().Err(err).Msg("runtime config watch failed")
				http.Error(w, "failed to watch runtime config", http.StatusInternalServerError)
				return
			}
		}
	}

	resp, err := a.buildRuntimeConfigResponse(r.Context(), service)
	if err != nil {
		log.Warn().Err(err).Str("service", service).Msg("failed to build runtime config response")
		http.Error(w, "failed to load runtime config", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, resp)
}

func runtimeConfigWatchVersion(r *http.Request) int64 {
	if r == nil {
		return 0
	}
	raw := strings.TrimSpace(r.URL.Query().Get("version"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("since_version"))
	}
	version, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || version < 0 {
		return 0
	}
	return version
}

func canonicalRuntimeConfigService(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "nopsai", "control-plane":
		return "nopsai", true
	case "git-bot", "gitbot":
		return "git-bot", true
	case "dispatcher":
		return "dispatcher", true
	case "runner", "docker-runner", "k8s-runner":
		return "runner", true
	case "agent":
		return "agent", true
	default:
		return "", false
	}
}

func (a *App) authorizeRuntimeConfigService(r *http.Request, service string) bool {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(claims.Provider), serviceauth.ProviderInternalService) {
		return false
	}
	if containsFold(claims.Roles, serviceauth.RoleNopsai) {
		return true
	}
	switch service {
	case "nopsai":
		return containsFold(claims.Roles, serviceauth.RoleNopsai)
	case "git-bot":
		return containsFold(claims.Roles, serviceauth.RoleGitBot)
	case "dispatcher":
		return containsFold(claims.Roles, serviceauth.RoleDispatcher)
	case "runner":
		return containsFold(claims.Roles, serviceauth.RoleRunner)
	case "agent":
		return containsFold(claims.Roles, serviceauth.RoleAgent)
	default:
		return false
	}
}

func (a *App) waitForRuntimeConfigVersionChange(ctx context.Context, sinceVersion int64) error {
	deadline := time.NewTimer(runtimeConfigWatchTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(runtimeConfigWatchPoll)
	defer ticker.Stop()

	for {
		version, err := a.currentRuntimeConfigVersion(ctx)
		if err != nil {
			return err
		}
		if version != sinceVersion {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return nil
		case <-ticker.C:
		}
	}
}

func (a *App) buildRuntimeConfigResponse(ctx context.Context, service string) (runtimeConfigResponse, error) {
	version, err := a.currentRuntimeConfigVersion(ctx)
	if err != nil {
		return runtimeConfigResponse{}, err
	}
	snapshot := buildRuntimeConfigSnapshot(a.getConfigSnapshot(), service)
	return runtimeConfigResponse{
		Version:    version,
		Service:    snapshot.service,
		ReloadMode: snapshot.reloadMode,
		Config:     snapshot.values,
		Metadata:   runtimeConfigMetadata(snapshot.fields),
	}, nil
}

func (a *App) currentRuntimeConfigVersion(ctx context.Context) (int64, error) {
	if a == nil || a.db == nil {
		return 0, nil
	}
	var version int64
	err := a.db.QueryRow(ctx, `
		SELECT COALESCE(version, 0)
		FROM runtime_settings
		WHERE id = TRUE
	`).Scan(&version)
	if errorsIsNoRows(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return version, nil
}

func errorsIsNoRows(err error) bool {
	return err == pgx.ErrNoRows || err == sql.ErrNoRows
}

func buildRuntimeConfigSnapshot(cfg config.Config, service string) runtimeConfigSnapshot {
	switch service {
	case "git-bot":
		return runtimeConfigSnapshot{
			service:    "git-bot",
			reloadMode: config.ConfigScopeRuntimeReload,
			fields: []string{
				"github_app_id",
				"github_installation_id",
				"github_private_key_credential_ref",
				"github_webhook_credential_ref",
				"git_bot_nopsai_api_url",
			},
			values: map[string]any{
				"github_app_id":                     strings.TrimSpace(cfg.GitHubAppID),
				"github_installation_id":            strings.TrimSpace(cfg.GitHubInstallID),
				"github_app_installation_id":        strings.TrimSpace(cfg.GitHubInstallID),
				"github_private_key_ref":            strings.TrimSpace(cfg.GitHubPrivateKeyCredentialRef),
				"github_webhook_secret_ref":         strings.TrimSpace(cfg.GitHubWebhookCredentialRef),
				"github_private_key_credential_ref": strings.TrimSpace(cfg.GitHubPrivateKeyCredentialRef),
				"github_webhook_credential_ref":     strings.TrimSpace(cfg.GitHubWebhookCredentialRef),
				"git_bot_nopsai_api_url":            strings.TrimSpace(cfg.GitBotNopsaiAPIURL),
			},
		}
	case "dispatcher":
		return runtimeConfigSnapshot{
			service:    "dispatcher",
			reloadMode: config.ConfigScopeRuntimeLive,
			fields: []string{
				"agent_nopsai_api_url",
				"dispatcher_routing",
			},
			values: map[string]any{
				"agent_nopsai_api_url": strings.TrimSpace(cfg.AgentNopsaiAPIURL),
				"dispatcher_routing":   systemconfig.CloneDispatcherRouting(cfg.DispatcherRouting),
			},
		}
	case "runner":
		return runtimeConfigSnapshot{
			service:    "runner",
			reloadMode: config.ConfigScopeRuntimeReload,
			fields: []string{
				"runner_id",
				"runner_scopes",
				"runner_capacity",
				"dispatcher_address",
				"docker_network_name",
				"runtime",
				"runtime_pools",
				"limits",
			},
			values: map[string]any{
				"runner_id":           strings.TrimSpace(cfg.RunnerID),
				"runner_scopes":       strings.TrimSpace(cfg.RunnerScopes),
				"runner_capacity":     cfg.RunnerCapacity,
				"dispatcher_address":  strings.TrimSpace(cfg.DispatcherAddress),
				"docker_network_name": strings.TrimSpace(cfg.DockerNetworkName),
				"runtime":             config.NormalizeRuntime(cfg.Runtime),
				"runtime_pools":       config.NormalizeRuntimePools(cfg.RuntimePools),
				"limits":              cfg.Limits,
			},
		}
	case "agent":
		return runtimeConfigSnapshot{
			service:    "agent",
			reloadMode: config.ConfigScopeNextRunOnly,
			fields: []string{
				"agent_nopsai_api_url",
				"default_pipeline_timeout",
				"llm_agent_timeout",
			},
			values: map[string]any{
				"agent_nopsai_api_url":     strings.TrimSpace(cfg.AgentNopsaiAPIURL),
				"default_pipeline_timeout": strings.TrimSpace(cfg.DefaultPipelineTimeout),
				"llm_agent_timeout":        strings.TrimSpace(cfg.LLMAgentTimeout),
			},
		}
	default:
		return runtimeConfigSnapshot{
			service:    "nopsai",
			reloadMode: config.ConfigScopeRuntimeLive,
			fields: []string{
				"log_level",
				"log_format",
				"environment",
				"public_url",
				"notification_mail_logo_url",
				"notification_mail_website_url",
				"notification_mail_support_url",
				"notification_mail_footer_address",
				"require_production_gates",
				"nopsai_git_bot_api_url",
				"runtime",
				"runtime_pools",
				"limits",
			},
			values: map[string]any{
				"log_level":                        strings.TrimSpace(cfg.LogLevel),
				"log_format":                       strings.TrimSpace(cfg.LogFormat),
				"environment":                      strings.TrimSpace(cfg.Environment),
				"public_url":                       strings.TrimSpace(cfg.PublicURL),
				"notification_mail_logo_url":       strings.TrimSpace(cfg.NotificationMailLogoURL),
				"notification_mail_website_url":    strings.TrimSpace(cfg.NotificationMailWebsiteURL),
				"notification_mail_support_url":    strings.TrimSpace(cfg.NotificationMailSupportURL),
				"notification_mail_footer_address": strings.TrimSpace(cfg.NotificationMailFooterAddress),
				"require_production_gates":         cfg.RequireProductionGates,
				"nopsai_git_bot_api_url":           strings.TrimSpace(cfg.NopsaiGitBotAPIURL),
				"runtime":                          config.NormalizeRuntime(cfg.Runtime),
				"runtime_pools":                    config.NormalizeRuntimePools(cfg.RuntimePools),
				"limits":                           cfg.Limits,
			},
		}
	}
}

func runtimeConfigMetadata(fields []string) map[string]systemconfig.FieldMetadata {
	if len(fields) == 0 {
		return nil
	}
	metadata := make(map[string]systemconfig.FieldMetadata, len(fields))
	for _, field := range fields {
		if item, ok := systemconfig.FieldMetadataByKey[field]; ok {
			metadata[field] = item
		}
	}
	return metadata
}

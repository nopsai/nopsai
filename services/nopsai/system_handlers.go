package nopsai

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	"nopsai/pkg/servicelog"
	"nopsai/services/nopsai/internal/runnerinstall"
	"nopsai/services/nopsai/internal/systemconfig"
	"nopsai/services/nopsai/pkg/auth"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

type systemConfigPayload struct {
	LogLevel                      *string                       `json:"log_level"`
	LogFormat                     *string                       `json:"log_format"`
	Environment                   *string                       `json:"environment"`
	PublicURL                     *string                       `json:"public_url"`
	NotificationMailLogoURL       *string                       `json:"notification_mail_logo_url"`
	NotificationMailWebsiteURL    *string                       `json:"notification_mail_website_url"`
	NotificationMailSupportURL    *string                       `json:"notification_mail_support_url"`
	NotificationMailFooterAddress *string                       `json:"notification_mail_footer_address"`
	RequireProductionGates        *bool                         `json:"require_production_gates"`
	AgentNopsaiAPIURL             *string                       `json:"agent_nopsai_api_url"`
	GitBotNopsaiAPIURL            *string                       `json:"git_bot_nopsai_api_url"`
	NopsaiGitBotAPIURL            *string                       `json:"nopsai_git_bot_api_url"`
	DispatcherAddress             *string                       `json:"dispatcher_address"`
	AgentImage                    *string                       `json:"agent_image"`
	DockerNetworkName             *string                       `json:"docker_network_name"`
	AutoRemovalAgentContainer     *bool                         `json:"auto_removal_agent_container"`
	DefaultPipelineTimeout        *string                       `json:"default_pipeline_timeout"`
	LLMAgentTimeout               *string                       `json:"llm_agent_timeout"`
	DispatcherRouting             map[string][]string           `json:"dispatcher_routing"`
	RunnerID                      *string                       `json:"runner_id"`
	RunnerScopes                  *string                       `json:"runner_scopes"`
	RunnerCapacity                *int                          `json:"runner_capacity"`
	GitHubAppID                   *string                       `json:"github_app_id"`
	GitHubInstallationID          *string                       `json:"github_installation_id"`
	GitHubPrivateKeyRef           *string                       `json:"github_private_key_credential_ref"`
	GitHubWebhookRef              *string                       `json:"github_webhook_credential_ref"`
	Runtime                       *string                       `json:"runtime"`
	Kubernetes                    *config.KubernetesConfig      `json:"kubernetes"`
	Limits                        *config.RunnerLimits          `json:"limits"`
	RuntimePools                  map[string]config.RuntimePool `json:"runtime_pools"`
	Assistant                     *config.AssistantConfig       `json:"assistant"`
}

func (a *App) applySystemConfig(payload systemConfigPayload) (config.Config, error) {
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()
	return applySystemConfigToConfig(a.cfg, payload)
}

func applyRuntimeProcessSettings(cfg config.Config, payload systemConfigPayload) {
	level := zerolog.GlobalLevel()
	if payload.LogLevel != nil {
		configuredLevel, err := servicelog.ParseLevel(cfg.LogLevel)
		if err != nil {
			log.Warn().Str("log_level", cfg.LogLevel).Msg("Invalid runtime log level; keeping previous global level")
			return
		}
		level = configuredLevel
	}
	if payload.LogFormat != nil || payload.LogLevel != nil {
		servicelog.ConfigureLevel(level, cfg.LogFormat)
	}
}

func (a *App) applyRuntimeSettingsGitOpsPlan(ctx context.Context, binding models.ConfigRepository, plan *gitOpsRuntimeSettingsPlan, commitSHA string) error {
	return a.applySystemSettingsGitOpsPlans(ctx, binding, plan, nil, commitSHA)
}

func (a *App) applySystemSettingsGitOpsPlans(ctx context.Context, binding models.ConfigRepository, runtimePlan *gitOpsRuntimeSettingsPlan, githubPlan *gitOpsGitHubSettingsPlan, commitSHA string) error {
	sourcePaths := make([]string, 0, 2)
	applied := false
	var cfg config.Config

	if runtimePlan != nil {
		next, err := a.applySystemConfig(runtimePlan.payload)
		if err != nil {
			return err
		}
		cfg = next
		applyRuntimeProcessSettings(cfg, runtimePlan.payload)
		sourcePaths = append(sourcePaths, runtimePlan.sourcePath)
		applied = true
	}
	if githubPlan != nil {
		next, err := a.applySystemConfig(githubPlan.payload)
		if err != nil {
			return err
		}
		cfg = next
		sourcePaths = append(sourcePaths, githubPlan.sourcePath)
		applied = true
	}
	if !applied {
		return nil
	}
	configRepoID := binding.ID
	if err := a.persistRuntimeSettingsSnapshot(ctx, cfg, "git", &configRepoID, strings.Join(sourcePaths, ", "), commitSHA, true); err != nil {
		return err
	}
	return nil
}

func (a *App) applyGitHubSettingsGitOpsPlan(ctx context.Context, binding models.ConfigRepository, plan *gitOpsGitHubSettingsPlan, commitSHA string) error {
	if plan == nil {
		return nil
	}
	return a.applySystemSettingsGitOpsPlans(ctx, binding, nil, plan, commitSHA)
}

func (a *App) handleGetSystemConfig(w http.ResponseWriter, r *http.Request) {
	cfg := a.getConfigSnapshot()
	resp := systemconfig.BuildResponse(cfg, a.envFilePath)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Warn().Err(err).Msg("Failed to encode system config response")
	}
}

func (a *App) handleGenerateRunnerCompose(w http.ResponseWriter, r *http.Request) {
	cfg := a.getConfigSnapshot()
	resp, err := runnerinstall.BuildComposeResponse(cfg, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Warn().Err(err).Msg("Failed to encode runner compose response")
	}
}

func (a *App) handleListRuntimeScopes(w http.ResponseWriter, r *http.Request) {
	scopes, err := a.listRuntimeScopes(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("Failed to list runtime scopes")
		http.Error(w, "Failed to list runtime scopes", http.StatusInternalServerError)
		return
	}

	result := make([]ScopeResponse, 0, len(scopes))
	for _, scope := range scopes {
		result = append(result, ScopeResponse{Scope: scope})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Warn().Err(err).Msg("Failed to encode runtime scopes")
	}
}

func (a *App) listRuntimeScopes(ctx context.Context) ([]string, error) {
	scopeSet := map[string]struct{}{}
	addScope := func(raw string) {
		scope := runtimeScopeForDisplay(strings.Trim(strings.TrimSpace(raw), "/"))
		if scope == "" {
			return
		}
		scopeSet[scope] = struct{}{}
	}

	cfg := a.getConfigSnapshot()
	for _, scope := range strings.Split(cfg.RunnerScopes, ",") {
		addScope(scope)
	}
	for scope := range cfg.DispatcherRouting {
		if strings.TrimSpace(scope) != "*" {
			addScope(scope)
		}
	}

	if a.db != nil {
		rows, err := a.db.Query(ctx, `
			SELECT scope FROM variables
			UNION
			SELECT scope FROM secrets
			UNION
			SELECT scope FROM external_triggers
			UNION
			SELECT COALESCE(scope, '') FROM pipeline_runs
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var scope string
			if err := rows.Scan(&scope); err != nil {
				return nil, err
			}
			addScope(scope)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	scopes := make([]string, 0, len(scopeSet))
	for scope := range scopeSet {
		scopes = append(scopes, scope)
	}
	sort.Slice(scopes, func(i, j int) bool {
		if scopes[i] == defaultRuntimeScope && scopes[j] != defaultRuntimeScope {
			return true
		}
		if scopes[j] == defaultRuntimeScope && scopes[i] != defaultRuntimeScope {
			return false
		}
		return strings.ToLower(scopes[i]) < strings.ToLower(scopes[j])
	})
	return scopes, nil
}

func (a *App) handleGenerateRunnerBootstrapCommand(w http.ResponseWriter, r *http.Request) {
	cfg := a.getConfigSnapshot()
	resp, err := runnerinstall.BuildBootstrapCommandResponse(cfg, r, a.createRunnerBootstrapToken)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Warn().Err(err).Msg("Failed to encode runner bootstrap command response")
	}
}

func (a *App) handleGenerateKubernetesRunnerManifest(w http.ResponseWriter, r *http.Request) {
	cfg := a.getConfigSnapshot()
	resp, err := runnerinstall.BuildKubernetesManifestResponse(cfg, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Warn().Err(err).Msg("Failed to encode kubernetes runner manifest response")
	}
}

func (a *App) handleGenerateKubernetesRunnerBootstrapCommand(w http.ResponseWriter, r *http.Request) {
	cfg := a.getConfigSnapshot()
	resp, err := runnerinstall.BuildKubernetesBootstrapCommandResponse(cfg, r, a.createRunnerBootstrapToken)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Warn().Err(err).Msg("Failed to encode kubernetes runner bootstrap command response")
	}
}

func (a *App) handleRunnerBootstrap(w http.ResponseWriter, r *http.Request) {
	entry, ok := a.consumeRunnerBootstrapToken(r.URL.Query().Get("token"))
	if !ok {
		http.Error(w, "runner bootstrap token not found or expired", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", entry.ContentType)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(entry.Content))
}

func (a *App) handleUpdateSystemConfig(w http.ResponseWriter, r *http.Request) {
	var payload systemConfigPayload
	if err := httpapi.DecodeJSON(r, &payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	cfg, err := a.applySystemConfig(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	applyRuntimeProcessSettings(cfg, payload)
	if err := a.ensureGitHubAppCredentialReferences(r.Context(), cfg, credentialActorFromContext(r.Context())); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.persistRuntimeSettingsSnapshot(r.Context(), cfg, "database", nil, "", "", false); err != nil {
		log.Error().Err(err).Msg("Failed to persist runtime settings")
		http.Error(w, "Failed to persist system config", http.StatusInternalServerError)
		return
	}

	resp := systemconfig.BuildResponse(cfg, a.envFilePath)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Warn().Err(err).Msg("Failed to encode updated system config response")
	}
}

func (a *App) handleGetConfigSyncStatus(w http.ResponseWriter, r *http.Request) {
	status := a.getConfigSyncStatus()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Warn().Err(err).Msg("Failed to encode config sync status")
	}
}

func (a *App) handleDispatcherStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status, dispatcherErr := a.fetchDispatcherStatus(ctx)
	if dispatcherErr != nil {
		log.Error().Err(dispatcherErr).Msg("Failed to fetch dispatcher status")
	}

	a.cfgMu.RLock()
	routing := a.cfg.DispatcherRouting
	a.cfgMu.RUnlock()

	runners := []map[string]interface{}{}
	queuedJobs := int32(0)
	if status != nil {
		queuedJobs = status.GetQueuedJobs()
		runners = make([]map[string]interface{}, 0, len(status.GetRunners()))
		for _, runner := range status.GetRunners() {
			runners = append(runners, map[string]interface{}{
				"runner_id":           runner.GetRunnerId(),
				"scopes":              runner.GetScopes(),
				"capacity":            runner.GetCapacity(),
				"active_jobs":         runner.GetActiveJobs(),
				"inflight_jobs":       runner.GetInflightJobs(),
				"last_heartbeat_unix": runner.GetLastHeartbeatUnix(),
				"metadata":            runner.GetMetadata(),
				"allow_dispatch":      runner.GetAllowDispatch(),
			})
		}
	}

	resp := map[string]interface{}{
		"queued_jobs": queuedJobs,
		"runners":     runners,
	}
	if len(routing) > 0 {
		resp["routing"] = routing
	}
	if dispatcherErr != nil {
		resp["dispatcher_error"] = "Failed to fetch dispatcher status"
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Warn().Err(err).Msg("Failed to encode dispatcher status")
	}
}

func (a *App) handleInternalDispatcherRouting(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || !isDispatcherInternalClaims(claims) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	cfg := a.getConfigSnapshot()
	resp := map[string]interface{}{
		"dispatcher_routing": systemconfig.CloneDispatcherRouting(cfg.DispatcherRouting),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Warn().Err(err).Msg("Failed to encode internal dispatcher routing response")
	}
}

func (a *App) handleUpdateRunnerDispatch(w http.ResponseWriter, r *http.Request) {
	runnerID := strings.TrimSpace(r.PathValue("runnerID"))
	if runnerID == "" {
		http.Error(w, "runner_id is required", http.StatusBadRequest)
		return
	}

	var payload struct {
		AllowDispatch *bool  `json:"allow_dispatch"`
		ConnectionID  string `json:"connection_id"`
	}
	if err := httpapi.DecodeJSON(r, &payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	if payload.AllowDispatch == nil {
		http.Error(w, "allow_dispatch is required", http.StatusBadRequest)
		return
	}

	resp, err := a.dispatcher.UpdateRunnerDispatch(r.Context(), &proto.UpdateRunnerDispatchRequest{
		RunnerId:      runnerID,
		AllowDispatch: *payload.AllowDispatch,
		ConnectionId:  strings.TrimSpace(payload.ConnectionID),
	})
	if err != nil {
		log.Error().Err(err).Str("runner_id", runnerID).Msg("Failed to update runner dispatch state")
		statusCode := http.StatusBadGateway
		if st, ok := grpcstatus.FromError(err); ok {
			switch st.Code() {
			case codes.InvalidArgument:
				statusCode = http.StatusBadRequest
			case codes.NotFound:
				statusCode = http.StatusNotFound
			case codes.Unavailable:
				statusCode = http.StatusBadGateway
			default:
				statusCode = http.StatusInternalServerError
			}
			http.Error(w, st.Message(), statusCode)
			return
		}
		http.Error(w, "Failed to update runner dispatch", statusCode)
		return
	}

	if resp == nil || resp.Runner == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp.Runner); err != nil {
		log.Warn().Err(err).Msg("Failed to encode runner dispatch response")
	}
}

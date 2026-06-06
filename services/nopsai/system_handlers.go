package nopsai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/proto"
	"nopsai/services/nopsai/internal/runnerinstall"
	"nopsai/services/nopsai/internal/systemconfig"
	"nopsai/services/nopsai/pkg/auth"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"
)

type systemConfigPayload struct {
	AgentNopsaiAPIURL         *string                       `json:"agent_nopsai_api_url"`
	GitBotNopsaiAPIURL        *string                       `json:"git_bot_nopsai_api_url"`
	NopsaiGitBotAPIURL        *string                       `json:"nopsai_git_bot_api_url"`
	DispatcherAddress         *string                       `json:"dispatcher_address"`
	AgentImage                *string                       `json:"agent_image"`
	DockerNetworkName         *string                       `json:"docker_network_name"`
	AutoRemovalAgentContainer *bool                         `json:"auto_removal_agent_container"`
	DefaultPipelineTimeout    *string                       `json:"default_pipeline_timeout"`
	LLMAgentTimeout           *string                       `json:"llm_agent_timeout"`
	DispatcherRouting         map[string][]string           `json:"dispatcher_routing"`
	RunnerID                  *string                       `json:"runner_id"`
	RunnerScopes              *string                       `json:"runner_scopes"`
	RunnerCapacity            *int                          `json:"runner_capacity"`
	Runtime                   *string                       `json:"runtime"`
	Kubernetes                *config.KubernetesConfig      `json:"kubernetes"`
	Limits                    *config.RunnerLimits          `json:"limits"`
	RuntimePools              map[string]config.RuntimePool `json:"runtime_pools"`
}

func (a *App) applySystemConfig(payload systemConfigPayload) (config.Config, error) {
	if payload.RunnerCapacity != nil && *payload.RunnerCapacity <= 0 {
		return config.Config{}, fmt.Errorf("runner_capacity must be a positive integer")
	}
	if payload.Limits != nil {
		if err := systemconfig.ValidateRunnerLimits(*payload.Limits); err != nil {
			return config.Config{}, err
		}
	}
	routing := systemconfig.NormalizeDispatcherRoutingConfig(payload.DispatcherRouting)

	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()

	if payload.AgentNopsaiAPIURL != nil {
		a.cfg.AgentNopsaiAPIURL = strings.TrimSpace(*payload.AgentNopsaiAPIURL)
	}
	if payload.GitBotNopsaiAPIURL != nil {
		a.cfg.GitBotNopsaiAPIURL = strings.TrimSpace(*payload.GitBotNopsaiAPIURL)
	}
	if payload.NopsaiGitBotAPIURL != nil {
		a.cfg.NopsaiGitBotAPIURL = strings.TrimSpace(*payload.NopsaiGitBotAPIURL)
	}
	if payload.DispatcherAddress != nil {
		a.cfg.DispatcherAddress = strings.TrimSpace(*payload.DispatcherAddress)
	}
	if payload.AgentImage != nil {
		a.cfg.AgentImage = strings.TrimSpace(*payload.AgentImage)
	}
	if payload.DockerNetworkName != nil {
		a.cfg.DockerNetworkName = strings.TrimSpace(*payload.DockerNetworkName)
	}
	if payload.AutoRemovalAgentContainer != nil {
		a.cfg.AutoRemovalAgentContainer = *payload.AutoRemovalAgentContainer
	}
	if payload.DefaultPipelineTimeout != nil {
		a.cfg.DefaultPipelineTimeout = strings.TrimSpace(*payload.DefaultPipelineTimeout)
	}
	if payload.LLMAgentTimeout != nil {
		a.cfg.LLMAgentTimeout = strings.TrimSpace(*payload.LLMAgentTimeout)
	}
	if payload.DispatcherRouting != nil {
		a.cfg.DispatcherRouting = routing
	}
	if payload.RunnerID != nil {
		a.cfg.RunnerID = strings.TrimSpace(*payload.RunnerID)
	}
	if payload.RunnerScopes != nil {
		a.cfg.RunnerScopes = systemconfig.NormalizeRunnerScopes(*payload.RunnerScopes)
	}
	if payload.RunnerCapacity != nil {
		a.cfg.RunnerCapacity = *payload.RunnerCapacity
	}
	if payload.Runtime != nil {
		a.cfg.Runtime = config.NormalizeRuntime(*payload.Runtime)
	}
	if payload.Kubernetes != nil {
		a.cfg.Kubernetes = config.NormalizeKubernetesConfig(*payload.Kubernetes)
	}
	if payload.Limits != nil {
		a.cfg.Limits = *payload.Limits
	}
	if payload.RuntimePools != nil {
		a.cfg.RuntimePools = config.NormalizeRuntimePools(payload.RuntimePools)
	}

	return *a.cfg, nil
}

func (a *App) persistSystemConfig(cfg config.Config, payload systemConfigPayload) error {
	if a.configPath == "" {
		return nil
	}

	existing := map[string]interface{}{}
	if contents, err := os.ReadFile(a.configPath); err == nil {
		if len(contents) > 0 {
			if unmarshalErr := yaml.Unmarshal(contents, &existing); unmarshalErr != nil {
				log.Warn().Err(unmarshalErr).Msg("Failed to parse existing config file; rewriting allowed fields")
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if payload.AgentImage != nil {
		existing["agent_image"] = cfg.AgentImage
	}
	if payload.AgentNopsaiAPIURL != nil {
		existing["agent_nopsai_api_url"] = cfg.AgentNopsaiAPIURL
	}
	if payload.GitBotNopsaiAPIURL != nil {
		existing["git_bot_nopsai_api_url"] = cfg.GitBotNopsaiAPIURL
	}
	if payload.NopsaiGitBotAPIURL != nil {
		existing["nopsai_git_bot_api_url"] = cfg.NopsaiGitBotAPIURL
	}
	if payload.DispatcherAddress != nil {
		existing["dispatcher_address"] = cfg.DispatcherAddress
	}
	if payload.DockerNetworkName != nil {
		existing["docker_network_name"] = cfg.DockerNetworkName
	}
	if payload.AutoRemovalAgentContainer != nil {
		existing["auto_removal_agent_container"] = cfg.AutoRemovalAgentContainer
	}
	if payload.DefaultPipelineTimeout != nil {
		existing["default_pipeline_timeout"] = cfg.DefaultPipelineTimeout
	}
	if payload.LLMAgentTimeout != nil {
		existing["llm_agent_timeout"] = cfg.LLMAgentTimeout
	}
	if payload.DispatcherRouting != nil {
		existing["dispatcher_routing"] = systemconfig.CloneDispatcherRouting(cfg.DispatcherRouting)
	}
	if payload.RunnerID != nil {
		existing["runner_id"] = cfg.RunnerID
	}
	if payload.RunnerScopes != nil {
		existing["runner_scopes"] = cfg.RunnerScopes
	}
	if payload.RunnerCapacity != nil {
		existing["runner_capacity"] = cfg.RunnerCapacity
	}
	if payload.Runtime != nil {
		existing["runtime"] = config.NormalizeRuntime(cfg.Runtime)
	}
	if payload.Kubernetes != nil {
		existing["kubernetes"] = config.NormalizeKubernetesConfig(cfg.Kubernetes)
	}
	if payload.Limits != nil {
		existing["limits"] = cfg.Limits
	}
	if payload.RuntimePools != nil {
		existing["runtime_pools"] = config.NormalizeRuntimePools(cfg.RuntimePools)
	}

	contents, err := yaml.Marshal(existing)
	if err != nil {
		return err
	}

	return os.WriteFile(a.configPath, contents, 0o644)
}

func (a *App) persistEnvOverrides(cfg config.Config, payload systemConfigPayload) error {
	if a.envFilePath == "" {
		return nil
	}

	updates := map[string]string{}

	if payload.AgentNopsaiAPIURL != nil {
		updates["AGENT_NOPSAI_API_URL"] = cfg.AgentNopsaiAPIURL
	}
	if payload.GitBotNopsaiAPIURL != nil {
		updates["GIT_BOT_NOPSAI_API_URL"] = cfg.GitBotNopsaiAPIURL
	}
	if payload.NopsaiGitBotAPIURL != nil {
		updates["NOPSAI_GIT_BOT_API_URL"] = cfg.NopsaiGitBotAPIURL
	}
	if payload.DispatcherAddress != nil {
		updates["DISPATCHER_ADDRESS"] = cfg.DispatcherAddress
	}
	if payload.AgentImage != nil {
		updates["AGENT_IMAGE"] = cfg.AgentImage
	}
	if payload.DockerNetworkName != nil {
		updates["DOCKER_NETWORK_NAME"] = cfg.DockerNetworkName
	}
	if payload.AutoRemovalAgentContainer != nil {
		updates["AUTO_REMOVAL_AGENT_CONTAINER"] = strconv.FormatBool(cfg.AutoRemovalAgentContainer)
	}
	if payload.DefaultPipelineTimeout != nil {
		updates["DEFAULT_PIPELINE_TIMEOUT"] = cfg.DefaultPipelineTimeout
	}
	if payload.LLMAgentTimeout != nil {
		updates["LLM_AGENT_TIMEOUT"] = cfg.LLMAgentTimeout
	}
	if payload.DispatcherRouting != nil {
		if encoded, err := json.Marshal(systemconfig.CloneDispatcherRouting(cfg.DispatcherRouting)); err == nil {
			updates["DISPATCHER_ROUTING"] = string(encoded)
		}
	}
	if payload.RunnerID != nil {
		updates["RUNNER_ID"] = cfg.RunnerID
	}
	if payload.RunnerScopes != nil {
		updates["RUNNER_SCOPES"] = cfg.RunnerScopes
	}
	if payload.RunnerCapacity != nil {
		updates["RUNNER_CAPACITY"] = strconv.Itoa(cfg.RunnerCapacity)
	}
	if payload.Runtime != nil {
		updates["RUNTIME"] = config.NormalizeRuntime(cfg.Runtime)
	}
	if payload.Kubernetes != nil {
		k := config.NormalizeKubernetesConfig(cfg.Kubernetes)
		updates["KUBERNETES_NAMESPACE"] = k.Namespace
		updates["KUBERNETES_SERVICE_ACCOUNT"] = k.ServiceAccount
		updates["KUBERNETES_DEFAULT_IMAGE_PULL_POLICY"] = k.DefaultImagePullPolicy
		updates["KUBERNETES_DEFAULT_WORKSPACE_SIZE"] = k.DefaultWorkspaceSize
		updates["KUBERNETES_DEFAULT_WORKSPACE_ACCESS_MODE"] = k.DefaultWorkspaceAccessMode
		updates["KUBERNETES_DEFAULT_TASK_TIMEOUT"] = k.DefaultTaskTimeout
		updates["KUBERNETES_DEFAULT_RUN_TIMEOUT"] = k.DefaultRunTimeout
		updates["KUBERNETES_WORKSPACE_VOLUME_MODE"] = k.WorkspaceVolumeMode
		updates["KUBERNETES_EXISTING_WORKSPACE_PVC"] = k.ExistingWorkspacePVC
		updates["KUBERNETES_STORAGE_CLASS"] = k.StorageClass
		if k.AffinityEnabled != nil {
			updates["KUBERNETES_AFFINITY_ENABLED"] = strconv.FormatBool(*k.AffinityEnabled)
		}
		if k.CleanupFinishedPods != nil {
			updates["KUBERNETES_CLEANUP_FINISHED_PODS"] = strconv.FormatBool(*k.CleanupFinishedPods)
		}
		if len(k.PodLabels) > 0 {
			updates["KUBERNETES_POD_LABELS"] = systemconfig.EncodeEnvJSON(k.PodLabels)
		}
		if len(k.PodAnnotations) > 0 {
			updates["KUBERNETES_POD_ANNOTATIONS"] = systemconfig.EncodeEnvJSON(k.PodAnnotations)
		}
	}
	if payload.Limits != nil {
		updates["LIMITS_MAX_CONCURRENT_RUNS"] = strconv.Itoa(cfg.Limits.MaxConcurrentRuns)
		updates["LIMITS_MAX_CONCURRENT_TASKS"] = strconv.Itoa(cfg.Limits.MaxConcurrentTasks)
		updates["LIMITS_MAX_CONCURRENT_TASKS_PER_RUN"] = strconv.Itoa(cfg.Limits.MaxConcurrentTasksPerRun)
		updates["LIMITS_MAX_PENDING_TASKS"] = strconv.Itoa(cfg.Limits.MaxPendingTasks)
	}
	if payload.RuntimePools != nil {
		updates["RUNTIME_POOLS"] = systemconfig.EncodeEnvJSON(config.NormalizeRuntimePools(cfg.RuntimePools))
	}

	if len(updates) == 0 {
		return nil
	}

	return systemconfig.WriteEnvFile(a.envFilePath, updates)
}

func (a *App) applyRuntimeSettingsGitOpsPlan(plan *gitOpsRuntimeSettingsPlan) error {
	if plan == nil {
		return nil
	}
	cfg, err := a.applySystemConfig(plan.payload)
	if err != nil {
		return err
	}
	if err := a.persistSystemConfig(cfg, plan.payload); err != nil {
		return err
	}
	return a.persistEnvOverrides(cfg, plan.payload)
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
	if err := a.persistSystemConfig(cfg, payload); err != nil {
		log.Warn().Err(err).Msg("Failed to persist system config; keeping in-memory settings only")
	}
	if err := a.persistEnvOverrides(cfg, payload); err != nil {
		log.Warn().Err(err).Msg("Failed to persist .env overrides; keeping in-memory settings only")
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

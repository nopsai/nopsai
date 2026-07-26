package nopsai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	"nopsai/pkg/registryauth"
	"nopsai/pkg/servicelog"
	"nopsai/services/nopsai/internal/credentials"
	"nopsai/services/nopsai/internal/runnerinstall"
	"nopsai/services/nopsai/internal/systemconfig"
	"nopsai/services/nopsai/pkg/auth"
	"nopsai/services/nopsai/pkg/store"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

var errRegistryCredentialForbidden = errors.New("registry credential is not visible to this subject")

type systemConfigPayload struct {
	LogLevel                      *string                            `json:"log_level"`
	LogFormat                     *string                            `json:"log_format"`
	Environment                   *string                            `json:"environment"`
	PublicURL                     *string                            `json:"public_url"`
	NotificationMailLogoURL       *string                            `json:"notification_mail_logo_url"`
	NotificationMailWebsiteURL    *string                            `json:"notification_mail_website_url"`
	NotificationMailSupportURL    *string                            `json:"notification_mail_support_url"`
	NotificationMailFooterAddress *string                            `json:"notification_mail_footer_address"`
	RequireProductionGates        *bool                              `json:"require_production_gates"`
	NopsaiAPIURL                  *string                            `json:"nopsai_api_url"`
	GitBotAPIURL                  *string                            `json:"git_bot_api_url"`
	DispatcherAddress             *string                            `json:"dispatcher_grpc_address"`
	AgentImage                    *string                            `json:"agent_image"`
	DockerNetworkName             *string                            `json:"docker_network_name"`
	AutoRemovalAgentContainer     *bool                              `json:"auto_removal_agent_container"`
	DefaultPipelineTimeout        *string                            `json:"default_pipeline_timeout"`
	RuntimeOutputMaxBytes         *int                               `json:"runtime_output_max_bytes"`
	LLMAgentTimeout               *string                            `json:"llm_agent_timeout"`
	DispatcherRouting             map[string][]string                `json:"dispatcher_routing"`
	EjectedRunnerIDs              []string                           `json:"ejected_runner_ids"`
	RunnerID                      *string                            `json:"runner_id"`
	RunnerScopes                  *string                            `json:"runner_scopes"`
	RunnerCapacity                *int                               `json:"runner_capacity"`
	GitHubAppID                   *string                            `json:"github_app_id"`
	GitHubInstallationID          *string                            `json:"github_installation_id"`
	GitHubInstallations           *[]config.GitHubInstallationConfig `json:"github_installations"`
	GitHubPrivateKeyRef           *string                            `json:"github_private_key_credential_ref"`
	GitHubWebhookRef              *string                            `json:"github_webhook_credential_ref"`
	Runtime                       *string                            `json:"runtime"`
	Kubernetes                    *config.KubernetesConfig           `json:"kubernetes"`
	Limits                        *config.RunnerLimits               `json:"limits"`
	RuntimePools                  map[string]config.RuntimePool      `json:"runtime_pools"`
	Assistant                     *config.AssistantConfig            `json:"assistant"`
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
	if runtimePlan != nil {
		if err := a.applyRunnerRegistryCredentialsGitOpsPlan(ctx, binding, runtimePlan, commitSHA); err != nil {
			return err
		}
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
	options, assignments, refs, refsProvided, err := a.runnerRegistryBootstrapOptions(r)
	if err != nil {
		if errors.Is(err, errRegistryCredentialForbidden) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	issueToken := runnerinstall.TokenIssuer(a.createRunnerBootstrapToken)
	if len(refs) > 0 {
		req := cloneBootstrapRequest(r)
		actor := credentialActor(r)
		issueToken = func(content string, ttl time.Duration, contentType string) (string, time.Time, error) {
			return a.createRunnerBootstrapTokenWithBuilder(content, ttl, contentType, func(ctx context.Context) (string, string, error) {
				authOptions, err := a.resolveRunnerRegistryAuthBootstrap(ctx, refs, actor)
				if err != nil {
					return "", "", err
				}
				script, err := runnerinstall.BuildDockerBootstrapScript(cfg, req, runnerinstall.BootstrapOptions{RegistryAuth: authOptions})
				return script, contentType, err
			})
		}
	}
	resp, err := runnerinstall.BuildBootstrapCommandResponseWithOptions(cfg, r, issueToken, options)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if refsProvided {
		if err := a.replaceRunnerRegistryCredentials(r.Context(), resp.RunnerID, assignments); err != nil {
			log.Error().Err(err).Str("runner_id", resp.RunnerID).Msg("Failed to persist runner registry credential assignments")
			http.Error(w, "failed to persist runner registry credentials", http.StatusInternalServerError)
			return
		}
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
	options, assignments, refs, refsProvided, err := a.runnerRegistryBootstrapOptions(r)
	if err != nil {
		if errors.Is(err, errRegistryCredentialForbidden) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	issueToken := runnerinstall.TokenIssuer(a.createRunnerBootstrapToken)
	if len(refs) > 0 {
		req := cloneBootstrapRequest(r)
		actor := credentialActor(r)
		issueToken = func(content string, ttl time.Duration, contentType string) (string, time.Time, error) {
			return a.createRunnerBootstrapTokenWithBuilder(content, ttl, contentType, func(ctx context.Context) (string, string, error) {
				authOptions, err := a.resolveRunnerRegistryAuthBootstrap(ctx, refs, actor)
				if err != nil {
					return "", "", err
				}
				script, _, err := runnerinstall.BuildKubernetesBootstrapScript(cfg, req, runnerinstall.BootstrapOptions{RegistryAuth: authOptions})
				return script, contentType, err
			})
		}
	}
	resp, err := runnerinstall.BuildKubernetesBootstrapCommandResponseWithOptions(cfg, r, issueToken, options)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if refsProvided {
		if err := a.replaceRunnerRegistryCredentials(r.Context(), resp.RunnerID, assignments); err != nil {
			log.Error().Err(err).Str("runner_id", resp.RunnerID).Msg("Failed to persist Kubernetes runner registry credential assignments")
			http.Error(w, "failed to persist runner registry credentials", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Warn().Err(err).Msg("Failed to encode kubernetes runner bootstrap command response")
	}
}

func (a *App) handleRunnerBootstrap(w http.ResponseWriter, r *http.Request) {
	entry, ok, err := a.consumeRunnerBootstrapToken(r.Context(), runnerBootstrapTokenFromRequest(r))
	if err != nil {
		log.Warn().Err(err).Msg("Failed to build runner bootstrap content")
		http.Error(w, "failed to build runner bootstrap content", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "runner bootstrap token not found or expired", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", entry.ContentType)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(entry.Content))
}

func (a *App) runnerRegistryBootstrapOptions(r *http.Request) (runnerinstall.BootstrapOptions, []store.RunnerRegistryCredentialInput, []credentials.Reference, bool, error) {
	refs, provided, err := parseRunnerRegistryCredentialRefs(r)
	if err != nil || len(refs) == 0 {
		return runnerinstall.BootstrapOptions{}, nil, refs, provided, err
	}
	if a == nil || a.credentialStore == nil {
		return runnerinstall.BootstrapOptions{}, nil, refs, provided, fmt.Errorf("credential store is unavailable")
	}
	actor := credentialActor(r)
	assignments := make([]store.RunnerRegistryCredentialInput, 0, len(refs))
	refStrings := make([]string, 0, len(refs))
	hosts := make([]string, 0, len(refs))
	seenHosts := map[string]struct{}{}
	for _, ref := range refs {
		record, err := a.credentialStore.GetCredentialByReference(r.Context(), ref)
		if err != nil {
			return runnerinstall.BootstrapOptions{}, nil, refs, provided, fmt.Errorf("load registry credential %s: %w", ref.String(), err)
		}
		if record.Kind != registryauth.CredentialKindDockerConfigJSON {
			return runnerinstall.BootstrapOptions{}, nil, refs, provided, fmt.Errorf("credential %s must be kind %s", ref.String(), registryauth.CredentialKindDockerConfigJSON)
		}
		if !record.HasValue() {
			return runnerinstall.BootstrapOptions{}, nil, refs, provided, fmt.Errorf("credential %s must be active and have a value", ref.String())
		}
		allowed, err := a.canReadCredentialMetadata(r, record)
		if err != nil {
			return runnerinstall.BootstrapOptions{}, nil, refs, provided, fmt.Errorf("authorization unavailable for registry credential %s: %w", ref.String(), err)
		}
		if !allowed {
			return runnerinstall.BootstrapOptions{}, nil, refs, provided, fmt.Errorf("%w: %s", errRegistryCredentialForbidden, ref.String())
		}
		credentialHosts := runnerRegistryHostsFromMetadata(record.Metadata)
		if len(credentialHosts) == 0 {
			return runnerinstall.BootstrapOptions{}, nil, refs, provided, fmt.Errorf("credential %s is missing registry host metadata; rotate the credential value", ref.String())
		}
		refStrings = append(refStrings, ref.String())
		assignments = append(assignments, store.RunnerRegistryCredentialInput{
			CredentialRef: ref,
			RegistryHosts: credentialHosts,
			Source:        "database",
			Actor:         actor,
		})
		for _, host := range credentialHosts {
			if _, exists := seenHosts[host]; exists {
				continue
			}
			seenHosts[host] = struct{}{}
			hosts = append(hosts, host)
		}
	}
	sort.Strings(hosts)
	return runnerinstall.BootstrapOptions{
		RegistryAuth: runnerinstall.RegistryAuthBootstrap{
			CredentialRefs: refStrings,
			RegistryHosts:  hosts,
		},
	}, assignments, refs, provided, nil
}

func (a *App) resolveRunnerRegistryAuthBootstrap(ctx context.Context, refs []credentials.Reference, actor string) (runnerinstall.RegistryAuthBootstrap, error) {
	if a == nil || a.credentials == nil || a.credentialStore == nil {
		return runnerinstall.RegistryAuthBootstrap{}, fmt.Errorf("credential service is unavailable")
	}
	configs := make([][]byte, 0, len(refs))
	refStrings := make([]string, 0, len(refs))
	for _, ref := range refs {
		record, err := a.credentialStore.GetCredentialByReference(ctx, ref)
		if err != nil {
			return runnerinstall.RegistryAuthBootstrap{}, fmt.Errorf("load registry credential %s: %w", ref.String(), err)
		}
		if record.Kind != registryauth.CredentialKindDockerConfigJSON {
			return runnerinstall.RegistryAuthBootstrap{}, fmt.Errorf("credential %s must be kind %s", ref.String(), registryauth.CredentialKindDockerConfigJSON)
		}
		if !record.HasValue() {
			return runnerinstall.RegistryAuthBootstrap{}, fmt.Errorf("credential %s must be active and have a value", ref.String())
		}
		value, err := a.credentials.Resolve(ctx, ref, credentials.Purpose{
			ConsumerService: "nopsai",
			Operation:       "runner.bootstrap.registry_auth",
			SubjectType:     "user",
			SubjectID:       strings.TrimSpace(actor),
			CorrelationID:   requestIDFromContext(ctx),
		})
		if err != nil {
			return runnerinstall.RegistryAuthBootstrap{}, fmt.Errorf("resolve registry credential %s: %w", ref.String(), err)
		}
		configs = append(configs, value.Bytes())
		refStrings = append(refStrings, ref.String())
	}
	merged, hosts, err := registryauth.MergeDockerConfigs(configs...)
	if err != nil {
		return runnerinstall.RegistryAuthBootstrap{}, err
	}
	return runnerinstall.RegistryAuthBootstrap{
		DockerConfigJSON: merged,
		CredentialRefs:   refStrings,
		RegistryHosts:    hosts,
	}, nil
}

func cloneBootstrapRequest(r *http.Request) *http.Request {
	if r == nil {
		return nil
	}
	return r.Clone(context.Background())
}

func (a *App) replaceRunnerRegistryCredentials(ctx context.Context, runnerID string, assignments []store.RunnerRegistryCredentialInput) error {
	if a == nil || a.db == nil {
		return nil
	}
	return store.NewPGStore(a.db).ReplaceRunnerRegistryCredentials(ctx, runnerID, assignments)
}

func (a *App) applyRunnerRegistryCredentialsGitOpsPlan(
	ctx context.Context,
	binding models.ConfigRepository,
	plan *gitOpsRuntimeSettingsPlan,
	commitSHA string,
) error {
	if a == nil || a.db == nil || plan == nil || plan.runnerRegistryCredentials == nil {
		return nil
	}
	if a.credentialStore == nil {
		return fmt.Errorf("credential store is unavailable")
	}
	configRepoID := binding.ID
	assignments := make(map[string][]store.RunnerRegistryCredentialInput, len(plan.runnerRegistryCredentials))
	for runnerID, refs := range plan.runnerRegistryCredentials {
		runnerID = strings.TrimSpace(runnerID)
		for _, ref := range refs {
			record, err := a.credentialStore.GetCredentialByReference(ctx, ref)
			if err != nil {
				return fmt.Errorf("load runner registry credential %s: %w", ref.String(), err)
			}
			if record.Kind != registryauth.CredentialKindDockerConfigJSON {
				return fmt.Errorf("credential %s must be kind %s", ref.String(), registryauth.CredentialKindDockerConfigJSON)
			}
			if !record.HasValue() {
				return fmt.Errorf("credential %s must be active and have a value", ref.String())
			}
			hosts := runnerRegistryHostsFromMetadata(record.Metadata)
			if len(hosts) == 0 {
				return fmt.Errorf("credential %s is missing registry host metadata; rotate the credential value", ref.String())
			}
			assignments[runnerID] = append(assignments[runnerID], store.RunnerRegistryCredentialInput{
				CredentialRef:         ref,
				RegistryHosts:         hosts,
				Source:                "git",
				ManagedByConfigRepo:   true,
				ConfigRepoID:          &configRepoID,
				ConfigSourcePath:      plan.sourcePath,
				ConfigSourceCommitSHA: strings.TrimSpace(commitSHA),
				Actor:                 "gitops",
			})
		}
	}
	return store.NewPGStore(a.db).ReplaceManagedRunnerRegistryCredentials(ctx, configRepoID, plan.sourcePath, assignments)
}

func parseRunnerRegistryCredentialRefs(r *http.Request) ([]credentials.Reference, bool, error) {
	if r == nil {
		return nil, false, nil
	}
	query := r.URL.Query()
	rawValues := []string{}
	provided := false
	for _, key := range []string{"registry_credential_ref", "registry_credential_refs"} {
		values, ok := query[key]
		if !ok {
			continue
		}
		provided = true
		rawValues = append(rawValues, values...)
	}
	refs := []credentials.Reference{}
	seen := map[string]struct{}{}
	for _, raw := range rawValues {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			ref, err := credentials.ParseReference(part)
			if err != nil {
				return nil, provided, err
			}
			if _, exists := seen[ref.String()]; exists {
				continue
			}
			seen[ref.String()] = struct{}{}
			refs = append(refs, ref)
		}
	}
	return refs, provided, nil
}

func runnerBootstrapTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return strings.TrimSpace(header[len("bearer "):])
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
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
	routing, _ := systemconfig.RemoveRunnersFromDispatcherRouting(a.cfg.DispatcherRouting, a.cfg.EjectedRunnerIDs)
	a.cfgMu.RUnlock()

	runners := []map[string]interface{}{}
	runnerRoutes := []dispatcherRunnerRouteInfo{}
	queuedJobs := int32(0)
	if status != nil {
		queuedJobs = status.GetQueuedJobs()
		runners = make([]map[string]interface{}, 0, len(status.GetRunners()))
		for _, runner := range status.GetRunners() {
			metadata := runner.GetMetadata()
			runnerRoutes = append(runnerRoutes, dispatcherRunnerRouteInfo{
				id:     runner.GetRunnerId(),
				scopes: runner.GetScopes(),
			})
			runners = append(runners, map[string]interface{}{
				"runner_id":           runner.GetRunnerId(),
				"scopes":              runner.GetScopes(),
				"capacity":            runner.GetCapacity(),
				"active_jobs":         runner.GetActiveJobs(),
				"inflight_jobs":       runner.GetInflightJobs(),
				"last_heartbeat_unix": runner.GetLastHeartbeatUnix(),
				"metadata":            metadata,
				"allow_dispatch":      runner.GetAllowDispatch(),
				"connection_status":   runnerConnectionStatus(metadata),
				"reachable":           runnerReachable(metadata),
			})
		}
	}
	effectiveRouting := buildEffectiveDispatcherRouting(routing, runnerRoutes)

	resp := map[string]interface{}{
		"queued_jobs": queuedJobs,
		"runners":     runners,
	}
	if len(routing) > 0 {
		resp["routing"] = routing
	}
	if len(effectiveRouting) > 0 {
		resp["effective_routing"] = effectiveRouting
	}
	if dispatcherErr != nil {
		resp["dispatcher_error"] = "Failed to fetch dispatcher status"
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Warn().Err(err).Msg("Failed to encode dispatcher status")
	}
}

type dispatcherRunnerRouteInfo struct {
	id     string
	scopes []string
}

func buildEffectiveDispatcherRouting(configured map[string][]string, runners []dispatcherRunnerRouteInfo) map[string][]string {
	effective := systemconfig.CloneDispatcherRouting(configured)
	if effective == nil {
		effective = map[string][]string{}
	}
	for _, runner := range runners {
		runnerID := strings.TrimSpace(runner.id)
		if runnerID == "" {
			continue
		}
		scopes := runner.scopes
		if len(scopes) == 0 {
			scopes = []string{"*"}
		}
		scopeSet := make(map[string]struct{}, len(scopes))
		for _, rawScope := range scopes {
			scope := strings.TrimSpace(rawScope)
			if scope == "" {
				scope = "*"
			}
			if _, seen := scopeSet[scope]; seen {
				continue
			}
			effective[scope] = appendUniqueRoutingRunner(effective[scope], runnerID)
			scopeSet[scope] = struct{}{}
		}
	}
	return effective
}

func appendUniqueRoutingRunner(existing []string, runnerID string) []string {
	runnerID = strings.TrimSpace(runnerID)
	if runnerID == "" {
		return existing
	}
	for _, existingRunnerID := range existing {
		if strings.TrimSpace(existingRunnerID) == runnerID {
			return existing
		}
	}
	return append(existing, runnerID)
}

func (a *App) handleInternalDispatcherRouting(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || !isDispatcherInternalClaims(claims) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	cfg := a.getConfigSnapshot()
	routing, _ := systemconfig.RemoveRunnersFromDispatcherRouting(cfg.DispatcherRouting, cfg.EjectedRunnerIDs)
	resp := map[string]interface{}{
		"dispatcher_routing": routing,
		"ejected_runner_ids": config.NormalizeRunnerIDs(cfg.EjectedRunnerIDs),
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
		writeDispatcherRunnerControlError(w, err, runnerID, "Failed to update runner dispatch", "Failed to update runner dispatch state")
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

func (a *App) handleEjectRunner(w http.ResponseWriter, r *http.Request) {
	runnerID := strings.TrimSpace(r.PathValue("runnerID"))
	if runnerID == "" {
		http.Error(w, "runner_id is required", http.StatusBadRequest)
		return
	}
	if err := a.recordRunnerEjection(r.Context(), runnerID); err != nil {
		log.Error().Err(err).Str("runner_id", runnerID).Msg("Failed to persist runner ejection")
		http.Error(w, "Failed to persist runner ejection", http.StatusInternalServerError)
		return
	}

	_, err := a.dispatcher.UpdateRunnerDispatch(r.Context(), &proto.UpdateRunnerDispatchRequest{
		RunnerId:      runnerID,
		AllowDispatch: false,
		ConnectionId:  proto.RunnerControlConnectionIDEject,
	})
	if err != nil {
		if st, ok := grpcstatus.FromError(err); ok && st.Code() == codes.NotFound {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeDispatcherRunnerControlError(w, err, runnerID, "Failed to eject runner", "Failed to eject runner")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeDispatcherRunnerControlError(w http.ResponseWriter, err error, runnerID, fallbackMessage, logMessage string) {
	log.Error().Err(err).Str("runner_id", runnerID).Msg(logMessage)
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
	http.Error(w, fallbackMessage, statusCode)
}

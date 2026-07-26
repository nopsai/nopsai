package nopsai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicetls"
)

type RunLauncher interface {
	LaunchAgent(ctx context.Context, req AgentRunLaunchRequest)
}

type AgentRunLaunchRequest struct {
	RunID              string
	ParentRunID        string
	ParentRunnerID     string
	Pipeline           models.Pipeline
	PipelineDefinition []byte
	Timeout            time.Duration
	GitContext         map[string]string
	ParentHistory      string
	Scope              string
	Overrides          map[string]string
	ResumeCheckpointID string
	ResumeVariables    map[string]string
	RecoveryAttempt    bool
}

type appRunLauncher struct {
	app *App
}

type agentLaunchPayload struct {
	Job             *proto.JobRequest
	InitialLogLines []string
}

type agentLaunchFailure struct {
	reason    string
	notifyGit bool
}

func (a *App) agentRunLauncher() RunLauncher {
	if a == nil || a.runLauncher == nil {
		return appRunLauncher{app: a}
	}
	return a.runLauncher
}

func (l appRunLauncher) LaunchAgent(ctx context.Context, req AgentRunLaunchRequest) {
	if l.app == nil {
		return
	}
	l.app.launchAgent(ctx, req)
}

func (a *App) launchAgent(ctx context.Context, req AgentRunLaunchRequest) {
	if ctx == nil {
		ctx = context.Background()
	}

	if !a.runCanBeDispatched(ctx, req.RunID) {
		return
	}

	payload, failure := a.buildAgentLaunchPayload(ctx, req)
	if failure != nil {
		if req.RecoveryAttempt {
			log.Warn().Str("run_id", req.RunID).Str("reason", failure.reason).Msg("Pending run recovery could not rebuild launch payload")
			return
		}
		a.failAgentLaunch(ctx, req.RunID, req.GitContext, failure.reason, failure.notifyGit)
		return
	}

	if !req.RecoveryAttempt {
		a.appendRunLogs(ctx, req.RunID, payload.InitialLogLines...)
	}

	resp, err := a.dispatcher.SubmitJob(ctx, payload.Job)
	if err != nil {
		if req.RecoveryAttempt {
			log.Warn().Err(err).Str("run_id", req.RunID).Msg("Pending run recovery could not submit job to dispatcher")
			return
		}
		log.Error().Err(err).Str("run_id", req.RunID).Msg("Failed to dispatch job to runner")
		a.failAgentLaunch(ctx, req.RunID, req.GitContext, "Failed to dispatch job to runner", true)
		a.appendRunLogs(ctx, req.RunID, "Failed to dispatch job to runner: "+err.Error())
		return
	}

	switch resp.State {
	case proto.JobState_JOB_STATE_ASSIGNED:
		if err := markRunRunning(ctx, a.db, req.RunID); err != nil {
			log.Error().Err(err).Str("run_id", req.RunID).Msg("Failed to mark run as running")
		}
		log.Info().Str("run_id", req.RunID).Str("runner_id", resp.RunnerId).Msg("Job dispatched to runner")
		if req.RecoveryAttempt {
			a.appendRunLogs(ctx, req.RunID, fmt.Sprintf("Recovered pending run and dispatched to runner %s", resp.RunnerId))
		} else {
			a.appendRunLogs(ctx, req.RunID, fmt.Sprintf("Dispatched to runner %s", resp.RunnerId))
		}
	case proto.JobState_JOB_STATE_QUEUED:
		log.Info().Str("run_id", req.RunID).Msg("No runner available; job queued")
		if !req.RecoveryAttempt {
			a.appendRunLogs(ctx, req.RunID, "No runner available; job queued by dispatcher")
		}
	default:
		var latestStatus string
		if err := a.db.QueryRow(ctx, "SELECT status FROM pipeline_runs WHERE run_id = $1", req.RunID).Scan(&latestStatus); err == nil && isTerminalRunStatus(latestStatus) {
			log.Info().Str("run_id", req.RunID).Str("status", latestStatus).Msg("Dispatcher rejected job for terminal run")
			a.appendRunLogs(ctx, req.RunID, fmt.Sprintf("Dispatcher skipped agent launch because run is %s", strings.ToLower(strings.TrimSpace(latestStatus))))
			return
		}
		if req.RecoveryAttempt {
			log.Warn().Str("run_id", req.RunID).Str("state", resp.State.String()).Msg("Pending run recovery was rejected by dispatcher")
			return
		}
		log.Error().Str("run_id", req.RunID).Str("state", resp.State.String()).Msg("Dispatcher rejected job")
		a.failAgentLaunch(ctx, req.RunID, req.GitContext, "Dispatcher rejected job", true)
		a.appendRunLogs(ctx, req.RunID, "Dispatcher rejected job")
	}
}

func (a *App) runCanBeDispatched(ctx context.Context, runID string) bool {
	var currentStatus string
	if err := a.db.QueryRow(ctx, "SELECT status FROM pipeline_runs WHERE run_id = $1", runID).Scan(&currentStatus); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to load run before agent launch")
		return false
	}

	normalizedCurrentStatus := strings.ToLower(strings.TrimSpace(currentStatus))
	if isTerminalRunStatus(normalizedCurrentStatus) || normalizedCurrentStatus == runStatusWaitingApproval {
		log.Info().Str("run_id", runID).Str("status", currentStatus).Msg("Skipping agent launch for non-dispatchable run")
		return false
	}
	return true
}

func (a *App) buildAgentLaunchPayload(ctx context.Context, req AgentRunLaunchRequest) (*agentLaunchPayload, *agentLaunchFailure) {
	cfg := a.getConfigSnapshot()

	secrets, err := a.prepareSecretsForPipeline(req.RunID, req.Pipeline, req.GitContext, req.Scope)
	if err != nil {
		log.Error().Err(err).Str("run_id", req.RunID).Msg("Failed to prepare secrets for pipeline")
		return nil, agentLaunchFailed(err.Error(), true)
	}

	finalVars := req.ResumeVariables
	if finalVars == nil {
		finalVars, err = a.prepareVariablesForPipeline(req.RunID, req.Pipeline, req.GitContext, req.Scope, req.Overrides)
		if err != nil {
			log.Error().Err(err).Str("run_id", req.RunID).Msg("Failed to prepare scope variables for pipeline")
			return nil, agentLaunchFailed(err.Error(), true)
		}
	}

	teamID, err := a.teamIDForRunProfileOwner(ctx, req.RunID)
	if err != nil {
		reason := fmt.Sprintf("Failed to resolve team profile owner: %v", err)
		log.Error().Err(err).Str("run_id", req.RunID).Msg("Failed to resolve team profile owner")
		return nil, agentLaunchFailed(reason, false)
	}

	if err := a.validatePipelineLLMProfilesForTeam(ctx, &req.Pipeline, req.Scope, teamID); err != nil {
		reason := err.Error()
		log.Error().Str("run_id", req.RunID).Msg(reason)
		return nil, agentLaunchFailed(reason, true)
	}
	if err := a.validatePipelineAgentProfilesForTeam(ctx, &req.Pipeline, teamID); err != nil {
		reason := err.Error()
		log.Error().Str("run_id", req.RunID).Msg(reason)
		return nil, agentLaunchFailed(reason, true)
	}
	if err := a.validatePipelineMCPProfilesForTeam(ctx, &req.Pipeline, req.Scope, teamID); err != nil {
		reason := err.Error()
		log.Error().Str("run_id", req.RunID).Msg(reason)
		return nil, agentLaunchFailed(reason, true)
	}

	runtimeProfiles, err := a.buildRuntimeLLMProfilesForTeam(ctx, cfg, teamID)
	if err != nil {
		reason := fmt.Sprintf("Failed to prepare LLM profiles: %v", err)
		log.Error().Err(err).Str("run_id", req.RunID).Msg("Failed to prepare LLM profiles")
		return nil, agentLaunchFailed(reason, false)
	}
	runtimeProfilesJSON, err := json.Marshal(runtimeProfiles)
	if err != nil {
		reason := "Failed to marshal LLM profiles"
		log.Error().Err(err).Str("run_id", req.RunID).Msg(reason)
		return nil, agentLaunchFailed(reason, false)
	}
	runtimeAgentProfiles, err := a.buildRuntimeAgentProfilesForTeam(ctx, teamID)
	if err != nil {
		reason := fmt.Sprintf("Failed to prepare agent profiles: %v", err)
		log.Error().Err(err).Str("run_id", req.RunID).Msg("Failed to prepare agent profiles")
		return nil, agentLaunchFailed(reason, false)
	}
	runtimeAgentProfilesJSON, err := json.Marshal(runtimeAgentProfiles)
	if err != nil {
		reason := "Failed to marshal agent profiles"
		log.Error().Err(err).Str("run_id", req.RunID).Msg(reason)
		return nil, agentLaunchFailed(reason, false)
	}

	runtimeMCPRegistry, err := a.buildRuntimeMCPRegistryForTeam(ctx, &req.Pipeline, req.Scope, teamID)
	if err != nil {
		reason := fmt.Sprintf("Failed to prepare MCP registry: %v", err)
		log.Error().Err(err).Str("run_id", req.RunID).Msg("Failed to prepare MCP registry")
		return nil, agentLaunchFailed(reason, false)
	}
	runtimeMCPRegistryJSON, err := json.Marshal(runtimeMCPRegistry)
	if err != nil {
		reason := "Failed to marshal MCP registry"
		log.Error().Err(err).Str("run_id", req.RunID).Msg(reason)
		return nil, agentLaunchFailed(reason, false)
	}

	knowledgeSnapshots, err := a.loadRunKnowledgeContextSnapshots(ctx, req.RunID)
	if err != nil {
		reason := fmt.Sprintf("Failed to load knowledge context snapshots: %v", err)
		log.Error().Err(err).Str("run_id", req.RunID).Msg("Failed to load knowledge context snapshots")
		return nil, agentLaunchFailed(reason, false)
	}
	knowledgeSnapshotsEnv, err := snapshotsJSONBase64(knowledgeSnapshots)
	if err != nil {
		reason := "Failed to marshal knowledge context snapshots"
		log.Error().Err(err).Str("run_id", req.RunID).Msg(reason)
		return nil, agentLaunchFailed(reason, false)
	}

	secretsJSON, err := json.Marshal(secrets)
	if err != nil {
		reason := "Failed to marshal secrets"
		log.Error().Err(err).Str("run_id", req.RunID).Msg(reason)
		return nil, agentLaunchFailed(reason, false)
	}
	variablesJSON, err := json.Marshal(finalVars)
	if err != nil {
		reason := "Failed to marshal variables"
		log.Error().Err(err).Str("run_id", req.RunID).Msg(reason)
		return nil, agentLaunchFailed(reason, false)
	}

	agentImageName := a.getAgentImage()
	if agentImageName == "" {
		agentImageName = "nopsai-agent:latest"
	}

	dispatcherAddr := strings.TrimSpace(cfg.DispatcherAddress)
	if dispatcherAddr == "" {
		dispatcherAddr = "localhost:9090"
	}

	sharedVolumeName := fmt.Sprintf("vol-%s", req.RunID)
	triggerEventID := strings.TrimSpace(req.GitContext["trigger_event_id"])
	agentContainerName := buildLaunchAgentContainerName(req.Pipeline.Name, req.GitContext["repo_name"], triggerEventID, req.RunID)
	preferredRunnerID := strings.TrimSpace(req.ParentRunnerID)

	envVars := buildAgentEnvironment(cfg, agentEnvironmentInput{
		RunID:                   req.RunID,
		Pipeline:                req.Pipeline,
		PipelineDefinition:      req.PipelineDefinition,
		LLMProfilesJSON:         runtimeProfilesJSON,
		AgentProfilesJSON:       runtimeAgentProfilesJSON,
		MCPRegistryJSON:         runtimeMCPRegistryJSON,
		KnowledgeContextsBase64: knowledgeSnapshotsEnv,
		SharedVolumeName:        sharedVolumeName,
		DockerNetworkName:       a.getDockerNetworkName(),
		SecretsJSON:             secretsJSON,
		VariablesJSON:           variablesJSON,
		DispatcherAddress:       dispatcherAddr,
		Timeout:                 req.Timeout,
		LLMAgentTimeout:         a.getLLMAgentTimeout(),
		RuntimeOutputMaxBytes:   a.getRuntimeOutputMaxBytes(),
		ParentHistory:           req.ParentHistory,
		Scope:                   req.Scope,
		PreferredRunnerID:       preferredRunnerID,
		ResumeCheckpointID:      req.ResumeCheckpointID,
		GitContext:              req.GitContext,
	})

	initialLines := []string{}
	if triggerEventID != "" {
		initialLines = append(initialLines, fmt.Sprintf("Trigger Event ID: %s", triggerEventID))
	} else {
		initialLines = append(initialLines, "Trigger Event ID: N/A")
	}
	initialLines = append(initialLines, fmt.Sprintf("Preparing agent container %s with image %s", agentContainerName, agentImageName))

	affinityKey := triggerEventID
	if affinityKey == "" {
		affinityKey = strings.TrimSpace(req.ParentRunID)
	}
	if affinityKey == "" {
		affinityKey = req.RunID
	}

	return &agentLaunchPayload{
		Job: &proto.JobRequest{
			RunId:              req.RunID,
			PipelineName:       req.Pipeline.Name,
			PipelineVersion:    req.Pipeline.Version,
			PipelineDefinition: req.PipelineDefinition,
			Env:                envVars,
			AgentImage:         agentImageName,
			SharedVolumeName:   sharedVolumeName,
			DockerNetwork:      a.getDockerNetworkName(),
			AutoRemove:         a.getAutoRemovalAgentContainer(),
			ContainerName:      agentContainerName,
			Scope:              req.Scope,
			NopsaiApiUrl:       strings.TrimSpace(cfg.EffectiveNopsaiAPIURL()),
			TriggerEventId:     triggerEventID,
			RunnerAffinityKey:  affinityKey,
			// Parent runner affinity is a locality hint. Scope routing remains the
			// eligibility boundary, so child runs can use any runner in the route.
			PreferredRunnerId: preferredRunnerID,
		},
		InitialLogLines: initialLines,
	}, nil
}

type agentEnvironmentInput struct {
	RunID                   string
	Pipeline                models.Pipeline
	PipelineDefinition      []byte
	LLMProfilesJSON         []byte
	AgentProfilesJSON       []byte
	MCPRegistryJSON         []byte
	KnowledgeContextsBase64 string
	SharedVolumeName        string
	DockerNetworkName       string
	SecretsJSON             []byte
	VariablesJSON           []byte
	DispatcherAddress       string
	Timeout                 time.Duration
	LLMAgentTimeout         string
	RuntimeOutputMaxBytes   int
	ParentHistory           string
	Scope                   string
	PreferredRunnerID       string
	ResumeCheckpointID      string
	GitContext              map[string]string
}

func buildAgentEnvironment(cfg config.Config, input agentEnvironmentInput) []string {
	envVars := []string{
		fmt.Sprintf("RUN_ID=%s", input.RunID),
		fmt.Sprintf("PIPELINE_NAME=%s", input.Pipeline.Name),
		fmt.Sprintf("PIPELINE_VERSION=%s", input.Pipeline.Version),
		fmt.Sprintf("%s=%s", llmProfilesRuntimeEnv, base64.StdEncoding.EncodeToString(input.LLMProfilesJSON)),
		fmt.Sprintf("%s=%s", agentProfilesRuntimeEnv, base64.StdEncoding.EncodeToString(input.AgentProfilesJSON)),
		fmt.Sprintf("%s=%s", mcpRegistryRuntimeEnv, base64.StdEncoding.EncodeToString(input.MCPRegistryJSON)),
		fmt.Sprintf("NOPSAI_KNOWLEDGE_CONTEXTS=%s", input.KnowledgeContextsBase64),
		fmt.Sprintf("NOPSAI_API_URL=%s", cfg.EffectiveNopsaiAPIURL()),
		"NOPSAI_SERVICE_NAME=agent",
		fmt.Sprintf("LOG_LEVEL=%s", cfg.LogLevel),
		fmt.Sprintf("LOG_FORMAT=%s", cfg.LogFormat),
		fmt.Sprintf("PIPELINE_DEFINITION=%s", base64.StdEncoding.EncodeToString(input.PipelineDefinition)),
		fmt.Sprintf("SHARED_VOLUME_NAME=%s", input.SharedVolumeName),
		fmt.Sprintf("DOCKER_NETWORK_NAME=%s", input.DockerNetworkName),
		fmt.Sprintf("NOPSAI_SECRETS=%s", base64.StdEncoding.EncodeToString(input.SecretsJSON)),
		fmt.Sprintf("DISPATCHER_GRPC_ADDRESS=%s", input.DispatcherAddress),
		fmt.Sprintf("%s=%s", serviceauth.EnvSigningKey, cfg.EffectiveServiceJWTSigningKey()),
		fmt.Sprintf("%s=%s", serviceauth.EnvIssuer, cfg.EffectiveServiceJWTIssuer()),
		fmt.Sprintf("%s=%s", serviceauth.EnvAudience, cfg.EffectiveServiceJWTAudience()),
		fmt.Sprintf("%s=%s", serviceauth.EnvServiceID, cfg.EffectiveAgentServiceID()),
		fmt.Sprintf("%s=%s", servicetls.EnvMode, cfg.EffectiveDispatcherTLSMode()),
		fmt.Sprintf("%s=%s", servicetls.EnvSecret, cfg.EffectiveDispatcherTLSSecret()),
		fmt.Sprintf("%s=%s", servicetls.EnvServerName, cfg.EffectiveDispatcherTLSServerName()),
	}
	if input.Timeout > 0 {
		envVars = append(envVars, fmt.Sprintf("PIPELINE_TIMEOUT=%s", input.Timeout.String()))
	}
	if input.LLMAgentTimeout != "" {
		envVars = append(envVars, fmt.Sprintf("LLM_AGENT_TIMEOUT=%s", input.LLMAgentTimeout))
	}
	if input.RuntimeOutputMaxBytes > 0 {
		envVars = append(envVars, fmt.Sprintf("NOPSAI_RUNTIME_OUTPUT_MAX_BYTES=%d", input.RuntimeOutputMaxBytes))
	}
	if input.ParentHistory != "" {
		envVars = append(envVars, fmt.Sprintf("PARENT_EXECUTION_HISTORY=%s", input.ParentHistory))
	}
	if input.Scope != "" {
		envVars = append(envVars, fmt.Sprintf("SCOPE=%s", input.Scope))
	}
	if input.PreferredRunnerID != "" {
		envVars = append(envVars, fmt.Sprintf("PARENT_RUNNER_ID=%s", input.PreferredRunnerID))
	}
	if strings.TrimSpace(input.ResumeCheckpointID) != "" {
		envVars = append(envVars, fmt.Sprintf("RESUME_CHECKPOINT_ID=%s", strings.TrimSpace(input.ResumeCheckpointID)))
	}

	envVars = append(envVars, fmt.Sprintf("NOPSAI_VARIABLES=%s", base64.StdEncoding.EncodeToString(input.VariablesJSON)))

	for key, value := range input.GitContext {
		envKey := fmt.Sprintf("GIT_%s", strings.ToUpper(key))
		envVars = append(envVars, fmt.Sprintf("%s=%s", envKey, value))
	}
	return envVars
}

func (a *App) failAgentLaunch(ctx context.Context, runID string, gitContext map[string]string, reason string, notifyGit bool) {
	a.db.Exec(ctx, "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", reason, runID)
	if notifyGit && gitContext["repo_owner"] != "" {
		a.notifyGitBotOfFinalStatus("failure", "", "", reason, gitContext)
	}
}

func (a *App) appendRunLogs(ctx context.Context, runID string, lines ...string) {
	if len(lines) == 0 {
		return
	}

	dbBatch := &pgx.Batch{}
	for _, line := range lines {
		dbBatch.Queue("INSERT INTO pipeline_run_logs (run_id, line, source, level) VALUES ($1, $2, $3, $4)", runID, line, "nopsai", "info")
	}
	if br := a.db.SendBatch(ctx, dbBatch); br != nil {
		if err := br.Close(); err != nil {
			log.Error().Err(err).Str("run_id", runID).Msg("Failed to write log lines")
		}
	}
}

func agentLaunchFailed(reason string, notifyGit bool) *agentLaunchFailure {
	return &agentLaunchFailure{
		reason:    reason,
		notifyGit: notifyGit,
	}
}

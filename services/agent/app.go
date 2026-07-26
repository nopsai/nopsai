package agent

import (
	"context"
	"encoding/base64"
	"os"
	"strings"

	"nopsai/pkg/models"
	"nopsai/pkg/registryauth"
	"nopsai/pkg/startupgates"
	agentapp "nopsai/services/agent/internal/app"
	llmruntime "nopsai/services/agent/internal/llm"
)

const agentWorkspaceDir = models.DefaultPipelineWorkingDirectory

func Run() int {
	// --- Initialization ---
	if err := agentapp.ConfigureLogging(os.Getenv("LOG_LEVEL"), os.Getenv("LOG_FORMAT")); err != nil {
		agentLog("", "").Warn().Str("log_level", os.Getenv("LOG_LEVEL")).Msg("Invalid log level; defaulting to info")
	}
	runtimeConfig, configWarnings, err := agentapp.LoadRuntimeConfig(os.Getenv)
	runID := runtimeConfig.RunID
	pipelineName := runtimeConfig.PipelineName
	for _, warning := range configWarnings {
		agentLog(runID, pipelineName).Error().Err(warning.Err).Msg(warning.LogMessage())
	}
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg(agentapp.LoadFailureLogMessage(err))
		return 1
	}
	if err := startupgates.ValidateAgentEnv(os.Getenv); err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Agent startup gates failed")
		return 1
	}
	triggerEventID := runtimeConfig.TriggerEventID
	pipelineDefBytes := runtimeConfig.PipelineDefinitionYAML
	parentHistoryBase64 := runtimeConfig.ParentHistoryBase64
	sharedVolumeName := runtimeConfig.SharedVolumeName
	pipelineTimeoutStr := runtimeConfig.PipelineTimeout
	dockerNetworkName := runtimeConfig.DockerNetworkName
	llmTimeout := runtimeConfig.LLMTimeout
	runtimeOutputMaxBytes := runtimeConfig.RuntimeOutputMaxBytes
	secrets := runtimeConfig.Secrets
	variables := runtimeConfig.Variables
	resumeCheckpointID := runtimeConfig.ResumeCheckpointID
	runScope := runtimeConfig.RunScope
	pipeline := runtimeConfig.Pipeline

	var resumeCheckpoint *agentApprovalCheckpointResponse
	if strings.TrimSpace(resumeCheckpointID) != "" {
		checkpoint, err := fetchApprovalCheckpoint(context.Background(), runID, strings.TrimSpace(resumeCheckpointID))
		if err != nil {
			agentLog(runID, pipeline.Name).Error().Err(err).Str("checkpoint_id", resumeCheckpointID).Msg("Failed to fetch approval checkpoint")
			return 1
		}
		if checkpoint.WorkspaceArchiveBase64 != "" {
			archiveBytes, err := base64.StdEncoding.DecodeString(checkpoint.WorkspaceArchiveBase64)
			if err != nil {
				agentLog(runID, pipeline.Name).Error().Err(err).Str("checkpoint_id", resumeCheckpointID).Msg("Failed to decode approval workspace archive")
				return 1
			}
			if err := restoreWorkspaceArchive(agentWorkspaceDir, archiveBytes); err != nil {
				agentLog(runID, pipeline.Name).Error().Err(err).Str("checkpoint_id", resumeCheckpointID).Msg("Failed to restore approval workspace checkpoint")
				return 1
			}
		}
		if checkpoint.Variables != nil {
			variables = checkpoint.Variables
		}
		resumeCheckpoint = &checkpoint
		agentLog(runID, pipeline.Name).Info().
			Str("checkpoint_id", checkpoint.CheckpointID).
			Str("step", checkpoint.StepName).
			Int("completed_tasks", len(checkpoint.CompletedTasks)).
			Msg("Restored approval checkpoint")
	}
	runtimeAdapters, err := newAgentRuntimeAdapters(runScope, runID, &pipeline)
	if err != nil {
		logAgentRuntimeWiringError(runID, pipelineName, err)
		return 1
	}
	registryAuthResolver, registryHosts, err := registryauth.DockerConfigResolverFromEnv(os.Getenv)
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to load local Docker registry auth config")
		return 1
	}
	if registryAuthResolver.Configured() {
		agentLog(runID, pipelineName).Info().Strs("registry_hosts", registryHosts).Msg("Loaded local Docker registry auth config")
	}

	dispatcherConn, err := connectAgentDispatcher(os.Getenv)
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to configure dispatcher client")
		return 1
	}
	defer dispatcherConn.Close()

	knowledgeSnapshots, err := loadRuntimeKnowledgeContexts()
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to load knowledge context snapshots")
		return 1
	}
	workingDirectory, err := models.NormalizePipelineWorkingDirectory(pipeline.WorkingDirectory)
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Invalid pipeline working_directory")
		return 1
	}

	agentLog(runID, pipeline.Name).Info().Str("trigger_event_id", triggerEventID).Str("working_directory", workingDirectory).Msg("Pipeline execution starting")
	logAgentRuntimeAdapters(runID, pipeline.Name, runtimeAdapters)

	executionRuntime, err := agentapp.NewExecutionRuntime(runtimeConfig.RuntimeMode, sharedVolumeName, pipeline.AffinityEnabled, agentLog(runID, pipeline.Name))
	if err != nil {
		agentLog(runID, pipeline.Name).Error().Err(err).Msg("Failed to initialize execution runtime")
		return 1
	}
	defer executionRuntime.Close()
	var resumeSnapshot *agentapp.ApprovalResumeSnapshot
	if resumeCheckpoint != nil {
		resumeSnapshot = &agentapp.ApprovalResumeSnapshot{
			ExecutionHistory: resumeCheckpoint.ExecutionHistory,
			CompletedTasks:   resumeCheckpoint.CompletedTasks,
		}
	}

	stepRuntimeOptions := agentapp.ContainerStepRuntimeOptions{
		SharedVolumeName:  sharedVolumeName,
		DockerNetworkName: dockerNetworkName,
		GitRepoName:       os.Getenv("GIT_REPO_NAME"),
	}
	if registryAuthResolver.Configured() {
		stepRuntimeOptions.RegistryAuth = registryAuthResolver
	}
	stepRuntime := agentapp.NewContainerStepRuntime(executionRuntime, stepRuntimeOptions)

	result := agentapp.RunPipeline(agentapp.PipelineRunRequest{
		RunID:                   runID,
		PipelineName:            pipelineName,
		PipelineDefinitionYAML:  pipelineDefBytes,
		ParentHistoryBase64:     parentHistoryBase64,
		PipelineTimeout:         pipelineTimeoutStr,
		SharedVolumeName:        sharedVolumeName,
		WorkspaceDir:            agentWorkspaceDir,
		WorkingDirectory:        workingDirectory,
		Pipeline:                pipeline,
		Variables:               variables,
		Secrets:                 secrets,
		ResumeCheckpoint:        resumeSnapshot,
		KnowledgeSnapshots:      knowledgeSnapshots,
		PipelineLLMEnabled:      runtimeAdapters.PipelineLLMEnabled,
		LLMTimeout:              llmTimeout,
		RuntimeOutputMaxBytes:   runtimeOutputMaxBytes,
		StepRuntime:             stepRuntime,
		ConditionClientResolver: runtimeAdapters.ConditionClientResolver,
		ActionSessionResolver:   runtimeAdapters.ActionSessionResolver,
		ApprovalPauser:          newAgentApprovalPauser(),
		IncludeRunner:           newAgentChildPipelineIncludeRunner(),
		DirectoryLister:         getDirectoryListing,
		StopRetry:               llmruntime.IsNonRetryableGoalResolutionError,
		Logger:                  agentLog,
		StepLogger:              stepLog,
		UpdateTaskStatus:        updateTaskStatus,
		ReportTaskOutputs:       reportTaskOutputs,
		NotifyFinalStatus:       notifyFinalStatus,
		WatchRunCancellation:    watchRunCancellation,
		Env:                     os.Getenv,
		Environment:             agentPipelineEnvironment,
		Exit:                    os.Exit,
		KnowledgePrompt:         buildEffectiveKnowledgeContextPrompt,
		BlockingKnowledgeKinds:  effectiveBlockingKnowledgeContextKinds,
		KnowledgeViolation:      knowledgeContextViolationFailureReason,
	})
	return result.ExitCode
}

func agentPipelineEnvironment() []string {
	return filteredAgentEnvironment(os.Environ())
}

func filteredAgentEnvironment(environ []string) []string {
	filtered := make([]string, 0, len(environ))
	for _, entry := range environ {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(key, "NOPSAI_REGISTRY_DOCKER_CONFIG_") {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

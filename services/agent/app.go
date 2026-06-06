package agent

import (
	"context"
	"encoding/base64"
	"os"
	"strings"

	appconfig "nopsai/config"
	"nopsai/pkg/models"
	agentapp "nopsai/services/agent/internal/app"
	"nopsai/services/agent/internal/resolver"
)

const agentWorkspaceDir = models.DefaultPipelineWorkingDirectory

func Run() int {
	// --- Initialization ---
	agentapp.ConfigureLogging(os.Getenv("LOG_FORMAT"))
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
	triggerEventID := runtimeConfig.TriggerEventID
	pipelineDefBytes := runtimeConfig.PipelineDefinitionYAML
	parentHistoryBase64 := runtimeConfig.ParentHistoryBase64
	sharedVolumeName := runtimeConfig.SharedVolumeName
	pipelineTimeoutStr := runtimeConfig.PipelineTimeout
	dockerNetworkName := runtimeConfig.DockerNetworkName
	llmTimeout := runtimeConfig.LLMTimeout
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
	pipelineLLMEnabled := models.PipelineLLMEnabled(&pipeline)

	var llmRegistry *LLMProfileRegistry
	if pipelineLLMEnabled {
		llmRegistry, err = NewLLMProfileRegistryFromEnv(runScope)
		if err != nil {
			agentLog(runID, pipelineName).Error().Err(err).Msg("Invalid LLM profile configuration")
			return 1
		}
	}
	mcpRegistry, err := NewMCPProfileRegistryFromEnv(runScope)
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Invalid MCP registry configuration")
		return 1
	}

	conn, dispatcher, err := agentapp.NewDispatcherClientFromEnv(os.Getenv)
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to configure dispatcher client")
		return 1
	}
	defer conn.Close()
	dispatcherClient = dispatcher

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
	if pipelineLLMEnabled {
		defaultLLMProfile, _ := llmRegistry.DefaultProfile()
		startupLog := agentLog(runID, pipeline.Name).Info().
			Str("llm_profile", llmRegistry.DefaultProfileName()).
			Str("llm_provider", defaultLLMProfile.Provider)
		switch defaultLLMProfile.Provider {
		case appconfig.LLMProviderGemini:
			startupLog.Str("llm_model", defaultLLMProfile.Model).Msg("Agent starting with embedded LLM profile registry")
		case appconfig.LLMProviderLMStudio:
			logEvent := startupLog.Str("lmstudio_base_url", defaultLLMProfile.BaseURL)
			if strings.TrimSpace(defaultLLMProfile.Model) != "" {
				logEvent = logEvent.Str("llm_model", defaultLLMProfile.Model)
			} else {
				logEvent = logEvent.Str("llm_model", "auto-discover")
			}
			if defaultLLMProfile.Reasoning != "" {
				logEvent = logEvent.Str("lmstudio_reasoning", defaultLLMProfile.Reasoning)
			}
			if defaultLLMProfile.Thinking != nil {
				logEvent = logEvent.Bool("lmstudio_thinking", *defaultLLMProfile.Thinking)
			}
			logEvent.Msg("Agent starting with embedded LLM profile registry")
		default:
			startupLog.Msg("Agent starting with embedded LLM profile registry")
		}
	} else {
		agentLog(runID, pipeline.Name).Info().Msg("LLM is disabled for this pipeline; LLM profile registry will not be loaded")
	}

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

	stepRuntime := agentapp.NewContainerStepRuntime(executionRuntime, agentapp.ContainerStepRuntimeOptions{
		SharedVolumeName:  sharedVolumeName,
		DockerNetworkName: dockerNetworkName,
		GitRepoName:       os.Getenv("GIT_REPO_NAME"),
	})

	var conditionClientResolver resolver.ConditionClientResolver
	if llmRegistry != nil {
		conditionClientResolver = func(pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task) (resolver.ConditionClient, string, error) {
			return llmRegistry.ClientFor(pipeline, step, task)
		}
	}

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
		PipelineLLMEnabled:      pipelineLLMEnabled,
		LLMTimeout:              llmTimeout,
		StepRuntime:             stepRuntime,
		ConditionClientResolver: conditionClientResolver,
		ActionSessionResolver:   newAgentActionSessionResolver(llmRegistry, mcpRegistry),
		ApprovalPauser:          newAgentApprovalPauser(),
		IncludeRunner:           newAgentChildPipelineIncludeRunner(),
		DirectoryLister:         getDirectoryListing,
		StopRetry:               isNonRetryableGoalResolutionError,
		Logger:                  agentLog,
		StepLogger:              stepLog,
		UpdateTaskStatus:        updateTaskStatus,
		NotifyFinalStatus:       notifyFinalStatus,
		WatchRunCancellation:    watchRunCancellation,
		Env:                     os.Getenv,
		Environment:             os.Environ,
		Exit:                    os.Exit,
		KnowledgePrompt:         buildEffectiveKnowledgeContextPrompt,
		BlockingKnowledgeKinds:  effectiveBlockingKnowledgeContextKinds,
		KnowledgeViolation:      knowledgeContextViolationFailureReason,
	})
	return result.ExitCode
}

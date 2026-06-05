package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	appconfig "nopsai/config"
	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	agentapp "nopsai/services/agent/internal/app"
	"nopsai/services/agent/internal/approval"
	"nopsai/services/agent/internal/dockerexec"
	"nopsai/services/agent/internal/executor"
	includeflow "nopsai/services/agent/internal/include"
	"nopsai/services/agent/internal/kubernetesexec"
	"nopsai/services/agent/internal/resolver"
	"nopsai/services/agent/internal/scheduler"

	"github.com/rs/zerolog"
)

const agentWorkspaceDir = models.DefaultPipelineWorkingDirectory

type TaskResult struct {
	Name      string
	Success   bool
	Skipped   bool
	Paused    bool
	Condition string
}

func run() int {
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
	cli := executionRuntime.Docker
	k8sRuntime := executionRuntime.Kubernetes

	sessionRegistry := agentapp.NewStepSessionRegistry()

	cleanupStepContainers := func(reason string) {
		cleanupLogger := agentLog(runID, pipeline.Name)
		agentapp.CleanupStepSessions(agentapp.StepSessionCleanupRequest{
			Sessions: sessionRegistry.Clear(),
			Reason:   reason,
			Logger:   cleanupLogger,
			Cleanup: func(session agentapp.StepSession) {
				if k8sRuntime != nil {
					k8sRuntime.CleanupPod(context.Background(), session.ID)
					return
				}
				dockerexec.Cleanup(context.Background(), cleanupLogger, cli, session.ID)
			},
		})
	}

	defer cleanupStepContainers("exit")

	activeTasks := agentapp.NewActiveTaskTracker()

	addActiveTask := func(stepName, taskName string) {
		activeTasks.Add(stepName, taskName)
	}

	removeActiveTask := func(stepName, taskName string) {
		activeTasks.Remove(stepName, taskName)
	}

	cancelActiveTasks := func(reason string) {
		for _, task := range activeTasks.Clear() {
			stepLog(runID, pipeline.Name, task.StepName, task.TaskName).Warn().Str("reason", reason).Msg("Marking task as cancelled")
			updateTaskStatus(pipeline.Name, runID, task.StepName, task.TaskName, "cancelled", 0, 0)
		}
	}

	defer cancelActiveTasks("exit")

	setTaskRunning := func(stepName, taskName string) {
		updateTaskStatus(pipeline.Name, runID, stepName, taskName, "running", 0, 0)
		addActiveTask(stepName, taskName)
	}

	finalizeTask := func(stepName, taskName, status string, exitCode int, llmDurationMs int64) {
		updateTaskStatus(pipeline.Name, runID, stepName, taskName, status, exitCode, llmDurationMs)
		removeActiveTask(stepName, taskName)
	}

	stopSignalHandler := agentapp.StartTerminationSignalHandler(agentLog(runID, pipeline.Name), func(reason string) {
		cancelActiveTasks(reason)
		cleanupStepContainers(reason)
	}, os.Exit)
	defer stopSignalHandler()

	go watchRunCancellation(pipeline.Name, runID, func() {
		cancelActiveTasks("run_cancelled")
		cleanupStepContainers("run_cancelled")
		os.Exit(0)
	})

	timeoutController := agentapp.StartPipelineTimeout(pipelineTimeoutStr, agentLog(runID, pipeline.Name), func(reason string) {
		cancelActiveTasks(reason)
		cleanupStepContainers(reason)
	})
	defer timeoutController.Stop()
	isRunStopping := func() bool {
		return timeoutController.Stopping()
	}

	totalTasks := scheduler.CountPipelineTasks(&pipeline)

	prePullCtx := timeoutController.ContextOrDefault(context.Background())
	if cli != nil {
		queue := scheduler.ImagePullQueue(&pipeline, totalTasks)
		if len(queue) == 0 {
			agentLog(runID, pipeline.Name).Debug().Msg("No images to pre-pull for pipeline")
		} else {
			prePullLogger := agentLog(runID, pipeline.Name).With().Str("component", "image-prepull").Logger()
			dockerexec.StartImagePrePull(prePullCtx, prePullLogger, cli, queue)
		}
	}

	history := new(strings.Builder)
	if resumeCheckpoint != nil {
		history.WriteString(resumeCheckpoint.ExecutionHistory)
		if resumeCheckpoint.ExecutionHistory != "" && !strings.HasSuffix(resumeCheckpoint.ExecutionHistory, "\n") {
			history.WriteString("\n")
		}
	} else if parentHistoryBase64 != "" {
		decodedHistory, err := base64.StdEncoding.DecodeString(parentHistoryBase64)
		if err != nil {
			agentLog(runID, pipeline.Name).Error().Err(err).Msg("Failed to decode parent execution history")
		} else {
			history.Write(decodedHistory)
			history.WriteString("\n--- Inherited History Above ---\n\n")
		}
	}
	completedTasks := make(map[string]bool)
	if resumeCheckpoint != nil {
		for _, key := range resumeCheckpoint.CompletedTasks {
			key = strings.TrimSpace(key)
			if key != "" {
				completedTasks[key] = true
			}
		}
	}
	pipelineFailed := false
	pipelinePaused := false
	var syncWg sync.WaitGroup
	conditionEvaluator := resolver.NewConditionEvaluator()
	taskResolver := resolver.NewTaskActionResolver()
	actionSessionResolver := newAgentActionSessionResolver(llmRegistry, mcpRegistry)
	approvalPauser := newAgentApprovalPauser()
	includeRunner := newAgentChildPipelineIncludeRunner()

	for len(completedTasks) < totalTasks {
		if timeoutController.Triggered() {
			pipelineFailed = true
			break
		}
		runnableTasks := scheduler.NextRunnableTasks(&pipeline, completedTasks)
		if len(runnableTasks) == 0 {
			if !pipelineFailed && len(completedTasks) == totalTasks {
				agentLog(runID, pipeline.Name).Info().Msg("All tasks completed successfully")
			} else if !pipelineFailed {
				agentLog(runID, pipeline.Name).Error().Msg("Stall detected: No runnable tasks found, but not all tasks are complete")
				pipelineFailed = true
			}
			break
		}
		if approvalRunnable := scheduler.FirstApprovalRunnable(runnableTasks); approvalRunnable != nil {
			runnableTasks = []*scheduler.RunnableTask{approvalRunnable}
		}

		var wg sync.WaitGroup
		results := make(chan TaskResult, len(runnableTasks))
		historyMutex := &sync.Mutex{}

		for _, runnable := range runnableTasks {
			if timeoutController.Triggered() {
				break
			}
			wg.Add(1)
			go func(runnable *scheduler.RunnableTask) {
				defer wg.Done()
				if timeoutController.Triggered() {
					if runnable.Step != nil && runnable.Task != nil {
						finalizeTask(runnable.Step.GetName(), runnable.Task.Name, "cancelled", 0, 0)
					}
					results <- TaskResult{Name: runnable.GlobalKey, Success: false}
					return
				}

				step := runnable.Step
				task := runnable.Task
				stepName := step.GetName()
				taskLogger := stepLog(runID, pipeline.Name, stepName, task.Name)
				var llmDurationMs int64
				inheritedEnv := os.Environ()
				stepContext, missingSecrets := resolver.BuildStepContext(&pipeline, step, inheritedEnv, variables, secrets)
				for _, secretName := range missingSecrets {
					taskLogger.Warn().Str("secret", secretName).Msg("Secret was requested by step but not provided")
				}

				historyMutex.Lock()
				conditionHistorySnapshot := history.String()
				historyMutex.Unlock()
				conditionResult := conditionEvaluator.Evaluate(context.Background(), resolver.ConditionRequest{
					Logger:                 taskLogger,
					Pipeline:               &pipeline,
					Step:                   step,
					Context:                stepContext,
					History:                conditionHistorySnapshot,
					Secrets:                secrets,
					KnowledgePrompt:        buildEffectiveKnowledgeContextPrompt(&pipeline, step, nil, knowledgeSnapshots),
					BlockingKnowledgeKinds: effectiveBlockingKnowledgeContextKinds(&pipeline, step, nil, knowledgeSnapshots),
					LLMTimeout:             llmTimeout,
					LLMEnabled:             pipelineLLMEnabled,
					ClientResolver: func(pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task) (resolver.ConditionClient, string, error) {
						return llmRegistry.ClientFor(pipeline, step, task)
					},
					StopRetry: isNonRetryableGoalResolutionError,
				})
				llmDurationMs = conditionResult.LLMDurationMs
				if conditionResult.Terminal {
					if conditionResult.PipelineFailed {
						pipelineFailed = true
					}
					if conditionResult.FinalizeStatus != "" {
						finalizeTask(stepName, task.Name, conditionResult.FinalizeStatus, conditionResult.FinalizeExitCode, llmDurationMs)
					}
					results <- TaskResult{Name: runnable.GlobalKey, Success: !conditionResult.Failed, Skipped: conditionResult.Skipped}
					return
				}

				if approvalStep, ok := step.AsApprovalStep(); ok {
					setTaskRunning(stepName, task.Name)

					historyMutex.Lock()
					historySnapshot := history.String()
					historyMutex.Unlock()
					completedSnapshot := scheduler.CompletedTaskKeysSnapshot(completedTasks)

					pauseResp, err := approvalPauser.Pause(context.Background(), approval.Request{
						RunID:                  runID,
						StepName:               stepName,
						TaskName:               task.Name,
						ExecutionHistory:       historySnapshot,
						CompletedTasks:         completedSnapshot,
						PipelineDefinitionYAML: string(pipelineDefBytes),
						Variables:              variables,
						WorkspaceDir:           agentWorkspaceDir,
						SharedVolumeName:       sharedVolumeName,
						RunnerID:               os.Getenv("RUNNER_ID"),
					})
					if err != nil {
						taskLogger.Error().Err(err).Msg(approval.FailureLogMessage(err))
						finalizeTask(stepName, task.Name, "failure", 1, llmDurationMs)
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}

					removeActiveTask(stepName, task.Name)
					taskLogger.Info().
						Str("approval_id", pauseResp.ApprovalID).
						Str("checkpoint_id", pauseResp.CheckpointID).
						Str("approval_type", approvalStep.Approval.Type).
						Strs("groups", approvalStep.Approval.Groups).
						Msg("Pipeline paused for approval")
					results <- TaskResult{Name: runnable.GlobalKey, Success: true, Paused: true}
					return
				}

				includeTarget := strings.TrimSpace(step.GetInclude())
				if includeTarget != "" {
					setTaskRunning(stepName, stepName)
					historyMutex.Lock()
					historySnapshot := history.String()
					historyMutex.Unlock()
					includeContext := timeoutController.ContextOrDefault(context.Background())
					includeResult := includeRunner.Run(includeContext, includeflow.Request{
						Logger:             taskLogger,
						ParentRunID:        runID,
						ParentPipelineName: pipelineName,
						StepName:           stepName,
						IncludeTarget:      includeTarget,
						History:            historySnapshot,
						Sync:               step.GetSync(),
						LLMDurationMs:      llmDurationMs,
						SyncWaitGroup:      &syncWg,
						FinalizeTask:       finalizeTask,
						MarkPipelineFailed: func() { pipelineFailed = true },
					})
					results <- TaskResult{Name: runnable.GlobalKey, Success: includeResult.Success}
					return
				}

				var stepContainerID string
				setTaskRunning(stepName, task.Name)

				stepRuntimeVars := stepContext.ContainerVariables()
				taskContext := stepContext.WithTask(task)
				taskRuntimeVars := taskContext.ContainerVariables()

				imageName := step.GetImage()
				if imageName == "" {
					imageName = pipeline.ContainerImage
				}

				stepContainerID, createdSession, err := sessionRegistry.GetOrCreate(stepName, func() (string, error) {
					if k8sRuntime != nil {
						taskLogger.Info().Str("image", imageName).Msg("Creating new Kubernetes pod for step")
						return k8sRuntime.CreateStepPod(context.Background(), kubernetesexec.StepPodRequest{
							RunID:            runID,
							PipelineName:     pipelineName,
							StepName:         stepName,
							Image:            imageName,
							WorkingDirectory: workingDirectory,
							Env:              stepRuntimeVars,
							Volumes:          step.GetVolumes(),
							RuntimePool:      firstNonEmpty(step.GetRuntimePool(), pipeline.RuntimePool),
						})
					}
					taskLogger.Info().Str("image", imageName).Msg("Creating new container for step")
					stepContainerName := dockerexec.BuildStepContainerName(os.Getenv("GIT_REPO_NAME"), pipelineName, stepName, runID)
					return dockerexec.CreateStepContainer(context.Background(), taskLogger, cli, dockerexec.StepContainerRequest{
						Image:             imageName,
						WorkingDirectory:  workingDirectory,
						Env:               stepRuntimeVars,
						Volumes:           step.GetVolumes(),
						SharedVolumeName:  sharedVolumeName,
						DockerNetworkName: dockerNetworkName,
						ContainerName:     stepContainerName,
					})
				})
				if err != nil {
					if k8sRuntime != nil {
						taskLogger.Error().Err(err).Msg("Failed to create step pod")
					} else {
						taskLogger.Error().Err(err).Msg("Failed to create step container")
					}
					finalizeTask(stepName, task.Name, "failure", 1, llmDurationMs)
					results <- TaskResult{Name: runnable.GlobalKey, Success: false}
					return
				}
				if !createdSession {
					if k8sRuntime != nil {
						taskLogger.Info().Msg("Reusing existing step pod")
					} else {
						taskLogger.Info().Msg("Reusing existing step container")
					}
				}

				var action *proto.Action
				var actionStr string
				var historyGoal string

				historyMutex.Lock()
				historySnapshot := history.String()
				historyMutex.Unlock()
				actionParentCtx := timeoutController.ContextOrDefault(context.Background())
				actionResult := taskResolver.Resolve(context.Background(), resolver.ActionRequest{
					Logger:          taskLogger,
					Pipeline:        &pipeline,
					Step:            step,
					Task:            task,
					Context:         taskContext,
					History:         historySnapshot,
					ParentContext:   actionParentCtx,
					WorkspaceDir:    agentWorkspaceDir,
					IsRunStopping:   isRunStopping,
					Secrets:         secrets,
					KnowledgePrompt: buildEffectiveKnowledgeContextPrompt(&pipeline, step, task, knowledgeSnapshots),
					LLMTimeout:      llmTimeout,
					LLMEnabled:      pipelineLLMEnabled,
					SessionResolver: actionSessionResolver,
					DirectoryLister: getDirectoryListing,
					StopRetry:       isNonRetryableGoalResolutionError,
				})
				if actionResult.LLMDurationSet {
					llmDurationMs = actionResult.LLMDurationMs
				}
				if actionResult.Failed {
					if actionResult.FinalizeStatus != "" {
						finalizeTask(stepName, task.Name, actionResult.FinalizeStatus, actionResult.FinalizeExitCode, llmDurationMs)
					}
					results <- TaskResult{Name: runnable.GlobalKey, Success: false}
					return
				}
				action = actionResult.Action
				actionStr = actionResult.ActionSummary
				goalText := actionResult.Goal

				if failureReason, blockingKinds, ok := knowledgeContextViolationFailureReason(action, &pipeline, step, task, knowledgeSnapshots); ok {
					maskedReason := taskContext.MaskText(failureReason, secrets)
					taskLogger.Error().
						Strs("knowledge_context_kinds", blockingKinds).
						Msgf("Knowledge context blocked task: %s", maskedReason)
					if zerolog.GlobalLevel() <= zerolog.InfoLevel {
						taskLogger.Info().Msgf(`status=failure action="Return answer" output="%s"`, maskedReason)
					}
					historyGoal = goalText
					if historyGoal == "" {
						historyGoal = fmt.Sprintf("Execute script for task: %s", task.Name)
					}
					historyMutex.Lock()
					history.WriteString(fmt.Sprintf("- Goal: %s\n  Action: Return answer\n  Result (Exit Code 1): %s\n", historyGoal, maskedReason))
					historyMutex.Unlock()
					finalizeTask(stepName, task.Name, "failure", 1, llmDurationMs)
					results <- TaskResult{Name: runnable.GlobalKey, Success: false}
					return
				}

				debugLogger := taskLogger.With().
					Str("action_type", action.Type).
					Logger()
				debugLogger.Debug().Msgf("Executing action: %s", actionStr)

				var stdout, stderr string
				var exitCode int

				// Retry logic for potential race conditions (e.g. filesystem locks)
				for attempt := 0; attempt < 10; attempt++ {
					if k8sRuntime != nil {
						stdout, stderr, exitCode = k8sRuntime.ExecuteAction(context.Background(), stepContainerID, action, taskRuntimeVars, workingDirectory)
					} else {
						stdout, stderr, exitCode = executor.ExecuteDockerAction(context.Background(), cli, stepContainerID, action, taskRuntimeVars, workingDirectory)
					}
					if exitCode == 0 {
						break
					}
					// Check for common race condition errors in stderr/stdout?
					// For now, retry all non-zero exits as robust fallback.
					time.Sleep(time.Duration(attempt*100) * time.Millisecond)
				}

				status := "success"
				output := stdout
				if exitCode != 0 {
					status = "failure"
					output = stderr + stdout
				}
				maskedOutput := taskContext.MaskText(output, secrets)
				if zerolog.GlobalLevel() <= zerolog.InfoLevel {
					logMsg := fmt.Sprintf(`status=%s action="%s" output="%s"`, status, actionStr, maskedOutput)
					taskLogger.Info().Msg(logMsg)
				}

				shareOutput := true
				if pipeline.LlmOutputSharing != nil {
					shareOutput = *pipeline.LlmOutputSharing
				}
				if task.LlmOutputSharing != nil {
					shareOutput = *task.LlmOutputSharing
				}
				historyGoal = goalText
				if historyGoal == "" {
					historyGoal = fmt.Sprintf("Execute script for task: %s", task.Name)
				}
				if !shareOutput {
					taskLogger.Debug().Msg("Output sharing is DISABLED for this task. Hiding output from history")
					output = "[Output was hidden by pipeline configuration]"
				} else {
					output = maskedOutput
				}

				historyMutex.Lock()
				history.WriteString(fmt.Sprintf("- Goal: %s\n  Action: %s\n  Result (Exit Code %d): %s\n", historyGoal, actionStr, exitCode, output))
				historyMutex.Unlock()

				if exitCode == 0 {
					finalizeTask(stepName, task.Name, "success", exitCode, llmDurationMs)
					results <- TaskResult{Name: runnable.GlobalKey, Success: true}
				} else {
					if task.IgnoreFailure {
						finalizeTask(stepName, task.Name, "failure (ignored)", exitCode, llmDurationMs)
						taskLogger.Warn().Msg("Task failed, but failure is ignored")
						results <- TaskResult{Name: runnable.GlobalKey, Success: true}
					} else {
						finalizeTask(stepName, task.Name, "failure", exitCode, llmDurationMs)
						taskLogger.Error().Msg("Critical task failed")
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
					}
				}
			}(runnable)
		}

		wg.Wait()
		close(results)

		for result := range results {
			if result.Paused {
				pipelinePaused = true
				continue
			}
			if !result.Skipped {
				if result.Success {
					completedTasks[result.Name] = true
				} else {
					pipelineFailed = true
				}
			} else {
				completedTasks[result.Name] = true
			}
		}

		if timeoutController.Triggered() {
			pipelineFailed = true
			break
		}

		if pipelinePaused {
			break
		}

		if pipelineFailed {
			break
		}
	}

	syncWg.Wait()

	if pipelinePaused {
		agentLog(runID, pipeline.Name).Info().Msg("Pipeline paused for approval")
		return 0
	}

	finalStatus := "success"
	if timeoutController.Triggered() {
		finalStatus = "failure"
		agentLog(runID, pipeline.Name).Error().Msg("Pipeline timed out before completion")
	} else if pipelineFailed {
		finalStatus = "failure"
		agentLog(runID, pipeline.Name).Error().Msg("Pipeline finished with failed tasks")
	} else {
		agentLog(runID, pipeline.Name).Info().Msg("Pipeline finished successfully")
	}

	notifyFinalStatus(pipeline.Name, runID, finalStatus)
	if pipelineFailed {
		return 1
	}
	return 0
}

func main() {
	os.Exit(run())
}

package app

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nopsai/services/agent/internal/approval"
	includeflow "nopsai/services/agent/internal/include"
	"nopsai/services/agent/internal/resolver"
	"nopsai/services/agent/internal/scheduler"
	workspacectx "nopsai/services/agent/internal/workspace"

	"github.com/rs/zerolog"
)

func RunPipeline(req PipelineRunRequest) PipelineRunResult {
	pipeline := req.Pipeline
	runID := req.RunID
	pipelineName := pipeline.Name
	logger := req.agentLogger()

	sessionRegistry := NewStepSessionRegistry()
	activeTasks := NewActiveTaskTracker()

	cleanupStepSessions := func(reason string) {
		cleanupLogger := req.agentLogger()
		CleanupStepSessions(StepSessionCleanupRequest{
			Sessions: sessionRegistry.Clear(),
			Reason:   reason,
			Logger:   cleanupLogger,
			Cleanup: func(session StepSession) {
				if req.StepRuntime != nil {
					req.StepRuntime.CleanupSession(context.Background(), cleanupLogger, session.ID)
				}
			},
		})
	}
	cancelActiveTasks := func(reason string) {
		for _, task := range activeTasks.Clear() {
			req.stepLogger(task.StepName, task.TaskName).Warn().Str("reason", reason).Msg("Marking task as cancelled")
			req.reportTaskStatus(task.StepName, task.TaskName, "cancelled", 0, 0)
		}
	}

	defer cleanupStepSessions("exit")
	defer cancelActiveTasks("exit")

	stopSignalHandler := StartTerminationSignalHandler(logger, func(reason string) {
		cancelActiveTasks(reason)
		cleanupStepSessions(reason)
	}, req.exit)
	defer stopSignalHandler()

	if req.WatchRunCancellation != nil {
		go req.WatchRunCancellation(pipelineName, runID, func() {
			cancelActiveTasks("run_cancelled")
			cleanupStepSessions("run_cancelled")
			req.exit(0)
		})
	}

	timeoutController := StartPipelineTimeout(req.PipelineTimeout, logger, func(reason string) {
		cancelActiveTasks(reason)
		cleanupStepSessions(reason)
	})
	defer timeoutController.Stop()
	isRunStopping := func() bool {
		return timeoutController.Stopping()
	}

	totalTasks := scheduler.CountPipelineTasks(&pipeline)
	prePullCtx := timeoutController.ContextOrDefault(context.Background())
	if req.StepRuntime != nil {
		req.StepRuntime.PrePullImages(prePullCtx, *logger, &pipeline, totalTasks)
	}

	history := new(strings.Builder)
	if req.ResumeCheckpoint != nil {
		history.WriteString(req.ResumeCheckpoint.ExecutionHistory)
		if req.ResumeCheckpoint.ExecutionHistory != "" && !strings.HasSuffix(req.ResumeCheckpoint.ExecutionHistory, "\n") {
			history.WriteString("\n")
		}
	} else if req.ParentHistoryBase64 != "" {
		decodedHistory, err := base64.StdEncoding.DecodeString(req.ParentHistoryBase64)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to decode parent execution history")
		} else {
			history.Write(decodedHistory)
			history.WriteString("\n--- Inherited History Above ---\n\n")
		}
	}

	workspaceRevision := uint64(1)
	workspaceRevisionMutex := &sync.Mutex{}
	workspaceIndex := workspacectx.NewIndex(req.WorkspaceDir, pipeline.LlmContentInclude, pipeline.LlmContentIgnore)
	if err := workspaceIndex.Refresh(logger, workspaceRevision); err != nil {
		logger.Warn().Err(err).Uint64("workspace_revision", workspaceRevision).Msg("Failed to build initial workspace index")
	}
	snapshotWorkspaceRevision := func() uint64 {
		workspaceRevisionMutex.Lock()
		defer workspaceRevisionMutex.Unlock()
		return workspaceRevision
	}
	advanceWorkspaceRevision := func() uint64 {
		workspaceRevisionMutex.Lock()
		defer workspaceRevisionMutex.Unlock()
		workspaceRevision++
		if err := workspaceIndex.Refresh(logger, workspaceRevision); err != nil {
			logger.Warn().Err(err).Uint64("workspace_revision", workspaceRevision).Msg("Failed to refresh workspace index")
		}
		return workspaceRevision
	}

	completedTasks := make(map[string]bool)
	if req.ResumeCheckpoint != nil {
		for _, key := range req.ResumeCheckpoint.CompletedTasks {
			key = strings.TrimSpace(key)
			if key != "" {
				completedTasks[key] = true
			}
		}
	}
	historyRevision := uint64(len(completedTasks))
	policyRevision := newPolicyRevisionState(req.KnowledgeSnapshots)
	ensurePolicyRevision := func(ctx context.Context, logger *zerolog.Logger, stage string) error {
		checkCtx := timeoutController.ContextOrDefault(ctx)
		return policyRevision.EnsureCurrent(checkCtx, runID, req.PolicyRevisionChecker, logger, stage)
	}

	var pipelineFailed atomic.Bool
	pipelinePaused := false
	conditionEvaluator := resolver.NewConditionEvaluator()
	taskResolver := resolver.NewTaskActionResolver()

	for len(completedTasks) < totalTasks {
		if timeoutController.Triggered() {
			pipelineFailed.Store(true)
			break
		}
		runnableTasks := scheduler.NextRunnableTasks(&pipeline, completedTasks)
		if len(runnableTasks) == 0 {
			if !pipelineFailed.Load() && len(completedTasks) == totalTasks {
				logger.Info().Msg("All tasks completed successfully")
			} else if !pipelineFailed.Load() {
				logger.Error().Msg("Stall detected: No runnable tasks found, but not all tasks are complete")
				pipelineFailed.Store(true)
			}
			break
		}
		if approvalRunnable := scheduler.FirstApprovalRunnable(runnableTasks); approvalRunnable != nil {
			runnableTasks = []*scheduler.RunnableTask{approvalRunnable}
		}

		var wg sync.WaitGroup
		results := make(chan taskResult, len(runnableTasks))
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
						req.finalizeTask(activeTasks, runnable.Step.GetName(), runnable.Task.Name, "cancelled", 0, 0)
					}
					results <- taskResult{name: runnable.GlobalKey, success: false}
					return
				}

				step := runnable.Step
				task := runnable.Task
				stepName := step.GetName()
				taskLogger := req.stepLogger(stepName, task.Name)
				var llmDurationMs int64
				stepContext, missingSecrets := resolver.BuildStepContext(&pipeline, step, req.environment(), req.Variables, req.Secrets)
				for _, secretName := range missingSecrets {
					taskLogger.Warn().Str("secret", secretName).Msg("Secret was requested by step but not provided")
				}

				if strings.TrimSpace(step.GetCondition()) != "" {
					if err := ensurePolicyRevision(context.Background(), taskLogger, "condition_evaluation"); err != nil {
						taskLogger.Error().Err(err).Msg("Blocking policy revision check failed closed before condition evaluation")
						req.finalizeTask(activeTasks, stepName, task.Name, "failure", 1, llmDurationMs)
						results <- taskResult{name: runnable.GlobalKey, success: false}
						return
					}
				}
				historyMutex.Lock()
				conditionHistorySnapshot := llmHistorySnapshotWithRevision(history.String(), historyRevision)
				historyMutex.Unlock()
				conditionResult := conditionEvaluator.Evaluate(context.Background(), resolver.ConditionRequest{
					Logger:                 taskLogger,
					Pipeline:               &pipeline,
					Step:                   step,
					Context:                stepContext,
					History:                conditionHistorySnapshot,
					Secrets:                req.Secrets,
					KnowledgePrompt:        req.knowledgePrompt(&pipeline, step, nil),
					BlockingKnowledgeKinds: req.blockingKnowledgeKinds(&pipeline, step, nil),
					LLMTimeout:             req.LLMTimeout,
					LLMEnabled:             req.PipelineLLMEnabled,
					ClientResolver:         req.ConditionClientResolver,
					StopRetry:              req.StopRetry,
				})
				llmDurationMs = conditionResult.LLMDurationMs
				if conditionResult.Terminal {
					if conditionResult.PipelineFailed {
						pipelineFailed.Store(true)
					}
					if conditionResult.FinalizeStatus != "" {
						req.finalizeTask(activeTasks, stepName, task.Name, conditionResult.FinalizeStatus, conditionResult.FinalizeExitCode, llmDurationMs)
					}
					results <- taskResult{name: runnable.GlobalKey, success: !conditionResult.Failed, skipped: conditionResult.Skipped}
					return
				}

				if approvalStep, ok := step.AsApprovalStep(); ok {
					req.setTaskRunning(activeTasks, stepName, task.Name)
					if err := ensurePolicyRevision(context.Background(), taskLogger, "approval_pause"); err != nil {
						taskLogger.Error().Err(err).Msg("Blocking policy revision check failed closed before approval pause")
						req.finalizeTask(activeTasks, stepName, task.Name, "failure", 1, llmDurationMs)
						results <- taskResult{name: runnable.GlobalKey, success: false}
						return
					}

					historyMutex.Lock()
					historySnapshot := history.String()
					historyMutex.Unlock()
					completedSnapshot := scheduler.CompletedTaskKeysSnapshot(completedTasks)

					if req.ApprovalPauser == nil {
						taskLogger.Error().Msg("Approval pauser is not configured")
						req.finalizeTask(activeTasks, stepName, task.Name, "failure", 1, llmDurationMs)
						results <- taskResult{name: runnable.GlobalKey, success: false}
						return
					}
					pauseResp, err := req.ApprovalPauser.Pause(context.Background(), approval.Request{
						RunID:                  runID,
						StepName:               stepName,
						TaskName:               task.Name,
						ExecutionHistory:       historySnapshot,
						CompletedTasks:         completedSnapshot,
						PipelineDefinitionYAML: string(req.PipelineDefinitionYAML),
						Variables:              req.Variables,
						WorkspaceDir:           req.WorkspaceDir,
						SharedVolumeName:       req.SharedVolumeName,
						RunnerID:               req.runnerID(),
					})
					if err != nil {
						taskLogger.Error().Err(err).Msg(approval.FailureLogMessage(err))
						req.finalizeTask(activeTasks, stepName, task.Name, "failure", 1, llmDurationMs)
						results <- taskResult{name: runnable.GlobalKey, success: false}
						return
					}

					activeTasks.Remove(stepName, task.Name)
					taskLogger.Info().
						Str("approval_id", pauseResp.ApprovalID).
						Str("checkpoint_id", pauseResp.CheckpointID).
						Str("approval_type", approvalStep.Approval.Type).
						Strs("teams", approvalStep.Approval.Teams).
						Msg("Pipeline paused for approval")
					results <- taskResult{name: runnable.GlobalKey, success: true, paused: true}
					return
				}

				includeTarget := strings.TrimSpace(step.GetInclude())
				if includeTarget != "" {
					req.setTaskRunning(activeTasks, stepName, stepName)
					historyMutex.Lock()
					historySnapshot := history.String()
					historyMutex.Unlock()
					if req.IncludeRunner == nil {
						taskLogger.Error().Msg("Include runner is not configured")
						req.finalizeTask(activeTasks, stepName, stepName, "failure", 1, llmDurationMs)
						results <- taskResult{name: runnable.GlobalKey, success: false}
						return
					}
					includeContext := timeoutController.ContextOrDefault(context.Background())
					includeResult := req.IncludeRunner.Run(includeContext, includeflow.Request{
						Logger:             taskLogger,
						ParentRunID:        runID,
						ParentPipelineName: req.parentPipelineName(),
						StepName:           stepName,
						IncludeTarget:      includeTarget,
						History:            historySnapshot,
						Sync:               step.GetSync(),
						LLMDurationMs:      llmDurationMs,
						FinalizeTask: func(stepName, taskName, status string, exitCode int, llmDurationMs int64) {
							req.finalizeTask(activeTasks, stepName, taskName, status, exitCode, llmDurationMs)
						},
						MarkPipelineFailed: func() { pipelineFailed.Store(true) },
					})
					results <- taskResult{name: runnable.GlobalKey, success: includeResult.Success}
					return
				}

				req.setTaskRunning(activeTasks, stepName, task.Name)
				if err := ensurePolicyRevision(context.Background(), taskLogger, "goal_resolution"); err != nil {
					taskLogger.Error().Err(err).Msg("Blocking policy revision check failed closed before goal resolution")
					req.finalizeTask(activeTasks, stepName, task.Name, "failure", 1, llmDurationMs)
					results <- taskResult{name: runnable.GlobalKey, success: false}
					return
				}

				stepRuntimeVars := stepContext.ContainerVariables()
				taskContext := stepContext.WithTask(task)
				taskRuntimeVars := taskContext.ContainerVariables()

				imageName := step.GetImage()
				if imageName == "" {
					imageName = pipeline.ContainerImage
				}

				stepSessionID, createdSession, err := sessionRegistry.GetOrCreate(stepName, func() (string, error) {
					if req.StepRuntime == nil {
						return "", fmt.Errorf("step runtime is not configured")
					}
					return req.StepRuntime.CreateSession(context.Background(), taskLogger, StepRuntimeSessionRequest{
						RunID:            runID,
						PipelineName:     pipeline.Name,
						StepName:         stepName,
						GitRepoName:      req.env("GIT_REPO_NAME"),
						Image:            imageName,
						WorkingDirectory: req.WorkingDirectory,
						Env:              stepRuntimeVars,
						Volumes:          step.GetVolumes(),
						RuntimePool:      firstNonEmpty(step.GetRuntimePool(), pipeline.RuntimePool),
					})
				})
				if err != nil {
					taskLogger.Error().Err(err).Msgf("Failed to create step %s", stepRuntimeResourceName(req.StepRuntime))
					req.finalizeTask(activeTasks, stepName, task.Name, "failure", 1, llmDurationMs)
					results <- taskResult{name: runnable.GlobalKey, success: false}
					return
				}
				if !createdSession {
					taskLogger.Info().Msgf("Reusing existing step %s", stepRuntimeResourceName(req.StepRuntime))
				}

				historyMutex.Lock()
				historySnapshot := llmHistorySnapshotWithRevision(history.String(), historyRevision)
				historyMutex.Unlock()
				workspaceRevisionSnapshot := snapshotWorkspaceRevision()
				actionParentCtx := timeoutController.ContextOrDefault(context.Background())
				actionResult := taskResolver.Resolve(context.Background(), resolver.ActionRequest{
					Logger:                 taskLogger,
					Pipeline:               &pipeline,
					Step:                   step,
					Task:                   task,
					Context:                taskContext,
					History:                historySnapshot,
					ParentContext:          actionParentCtx,
					WorkspaceDir:           req.WorkspaceDir,
					WorkspaceRevision:      workspaceRevisionSnapshot,
					WorkspaceIndex:         workspaceIndex,
					IsRunStopping:          isRunStopping,
					Secrets:                req.Secrets,
					KnowledgePrompt:        req.knowledgePrompt(&pipeline, step, task),
					BlockingKnowledgeKinds: req.blockingKnowledgeKinds(&pipeline, step, task),
					LLMTimeout:             req.LLMTimeout,
					LLMEnabled:             req.PipelineLLMEnabled,
					SessionResolver:        req.ActionSessionResolver,
					DirectoryLister:        req.DirectoryLister,
					StopRetry:              req.StopRetry,
				})
				if actionResult.LLMDurationSet {
					llmDurationMs = actionResult.LLMDurationMs
				}
				if actionResult.Failed {
					if actionResult.FinalizeStatus != "" {
						req.finalizeTask(activeTasks, stepName, task.Name, actionResult.FinalizeStatus, actionResult.FinalizeExitCode, llmDurationMs)
					}
					results <- taskResult{name: runnable.GlobalKey, success: false}
					return
				}
				action := actionResult.Action
				actionStr := actionResult.ActionSummary
				goalText := actionResult.Goal

				if failureReason, blockingKinds, ok := req.knowledgeViolation(action, &pipeline, step, task); ok {
					maskedReason := taskContext.MaskText(failureReason, req.Secrets)
					taskLogger.Error().
						Strs("knowledge_context_kinds", blockingKinds).
						Msgf("Knowledge context blocked task: %s", maskedReason)
					if zerolog.GlobalLevel() <= zerolog.InfoLevel {
						taskLogger.Info().Msgf(`status=failure action="Return answer" output="%s"`, maskedReason)
					}
					historyGoal := goalText
					if historyGoal == "" {
						historyGoal = fmt.Sprintf("Execute script for task: %s", task.Name)
					}
					historyMutex.Lock()
					history.WriteString(fmt.Sprintf("- Goal: %s\n  Action: Return answer\n  Result (Exit Code 1): %s\n", historyGoal, maskedReason))
					historyRevision++
					historyMutex.Unlock()
					req.finalizeTask(activeTasks, stepName, task.Name, "failure", 1, llmDurationMs)
					results <- taskResult{name: runnable.GlobalKey, success: false}
					return
				}

				debugLogger := taskLogger.With().
					Str("action_type", action.Type).
					Logger()
				maskedActionStr := taskContext.MaskRuntimeText(actionStr, req.Secrets)
				debugLogger.Debug().Msgf("Executing action: %s", maskedActionStr)

				var stdout, stderr string
				var exitCode int

				actionExecuted := false
				if err := ensurePolicyRevision(context.Background(), taskLogger, "action_execution"); err != nil {
					stdout, stderr, exitCode = "", err.Error(), 1
				} else if err := resolver.ValidateReplaceFilePrecondition(actionResult.FilePrecondition, req.WorkspaceDir, snapshotWorkspaceRevision()); err != nil {
					stdout, stderr, exitCode = "", err.Error(), 1
				} else {
					for attempt := 0; attempt < 10; attempt++ {
						if req.StepRuntime == nil {
							stdout, stderr, exitCode = "", "step runtime is not configured", 1
						} else {
							actionExecuted = true
							stdout, stderr, exitCode = req.StepRuntime.ExecuteAction(context.Background(), stepSessionID, action, taskRuntimeVars, req.WorkingDirectory)
						}
						if exitCode == 0 {
							break
						}
						time.Sleep(time.Duration(attempt*100) * time.Millisecond)
					}
				}
				if actionExecuted && actionMayMutateWorkspace(action) {
					newRevision := advanceWorkspaceRevision()
					taskLogger.Debug().Uint64("workspace_revision", newRevision).Msg("Advanced workspace revision after mutating action")
				}

				status := "success"
				output := stdout
				if exitCode != 0 {
					status = "failure"
					output = stderr + stdout
				}
				maskedOutput := taskContext.MaskRuntimeText(output, req.Secrets)
				if zerolog.GlobalLevel() <= zerolog.InfoLevel {
					logMsg := fmt.Sprintf(`status=%s action="%s" output="%s"`, status, maskedActionStr, maskedOutput)
					taskLogger.Info().Msg(logMsg)
				}

				shareOutput := true
				if pipeline.LlmOutputSharing != nil {
					shareOutput = *pipeline.LlmOutputSharing
				}
				if task.LlmOutputSharing != nil {
					shareOutput = *task.LlmOutputSharing
				}
				historyGoal := goalText
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
				history.WriteString(fmt.Sprintf("- Goal: %s\n  Action: %s\n  Result (Exit Code %d): %s\n", historyGoal, maskedActionStr, exitCode, output))
				historyRevision++
				historyMutex.Unlock()

				if exitCode == 0 {
					req.finalizeTask(activeTasks, stepName, task.Name, "success", exitCode, llmDurationMs)
					results <- taskResult{name: runnable.GlobalKey, success: true}
				} else {
					if task.IgnoreFailure {
						req.finalizeTask(activeTasks, stepName, task.Name, "failure (ignored)", exitCode, llmDurationMs)
						taskLogger.Warn().Msg("Task failed, but failure is ignored")
						results <- taskResult{name: runnable.GlobalKey, success: true}
					} else {
						req.finalizeTask(activeTasks, stepName, task.Name, "failure", exitCode, llmDurationMs)
						taskLogger.Error().Msg("Critical task failed")
						results <- taskResult{name: runnable.GlobalKey, success: false}
					}
				}
			}(runnable)
		}

		wg.Wait()
		close(results)

		for result := range results {
			if result.paused {
				pipelinePaused = true
				continue
			}
			if !result.skipped {
				if result.success {
					completedTasks[result.name] = true
				} else {
					pipelineFailed.Store(true)
				}
			} else {
				completedTasks[result.name] = true
			}
		}

		if timeoutController.Triggered() {
			pipelineFailed.Store(true)
			break
		}
		if pipelinePaused || pipelineFailed.Load() {
			break
		}
	}

	if pipelinePaused {
		logger.Info().Msg("Pipeline paused for approval")
		return PipelineRunResult{ExitCode: 0, Paused: true}
	}

	finalStatus := "success"
	failed := pipelineFailed.Load()
	if timeoutController.Triggered() {
		finalStatus = "failure"
		logger.Error().Msg("Pipeline timed out before completion")
	} else if failed {
		finalStatus = "failure"
		logger.Error().Msg("Pipeline finished with failed tasks")
	} else {
		logger.Info().Msg("Pipeline finished successfully")
	}

	req.notifyFinalStatus(finalStatus)
	if failed {
		return PipelineRunResult{ExitCode: 1, FinalStatus: finalStatus}
	}
	return PipelineRunResult{ExitCode: 0, FinalStatus: finalStatus}
}

package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"nopsai/services/agent/internal/approval"
	"nopsai/services/agent/internal/executor"
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
	requiredRuntimeOutputs := referencedRuntimeOutputs(&pipeline)
	runtimeOutputs := newRuntimeOutputStore()
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
				ignoreFailure := effectiveIgnoreFailure(step, task)
				finalizeFailure := func(status string, exitCode int, llmDurationMs int64, allowIgnore bool) bool {
					if strings.TrimSpace(status) == "" {
						status = "failure"
						if exitCode == 0 {
							exitCode = 1
						}
					}
					if allowIgnore {
						status = failureStatusWithTolerance(status, ignoreFailure)
					}
					req.finalizeTask(activeTasks, stepName, task.Name, status, exitCode, llmDurationMs)
					if status == "failure (ignored)" {
						taskLogger.Warn().Msg("Task failed, but failure is ignored")
						return true
					}
					return false
				}
				var llmDurationMs int64
				stepContext, missingSecrets := resolver.BuildStepContext(&pipeline, step, req.environment(), req.Variables, req.Secrets)
				for _, secretName := range missingSecrets {
					taskLogger.Warn().Str("secret", secretName).Msg("Secret was requested by step but not provided")
				}
				if resolvedOutputs, err := resolveRuntimeOutputVariables(step.GetVariables(), runtimeOutputs); err != nil {
					taskLogger.Error().Err(err).Msg("Failed to resolve step runtime output variables")
					ignored := finalizeFailure("failure", 1, llmDurationMs, true)
					results <- taskResult{name: runnable.GlobalKey, success: ignored}
					return
				} else {
					for variableName, output := range resolvedOutputs {
						stepContext.SetValue(variableName, output.Value, output.Sensitive)
					}
				}

				historyMutex.Lock()
				conditionHistorySnapshot := llmHistorySnapshotWithRevision(history.String(), historyRevision)
				historyMutex.Unlock()
				conditionBlockingKinds := req.blockingKnowledgeKinds(&pipeline, step, nil)
				conditionResult := conditionEvaluator.Evaluate(context.Background(), resolver.ConditionRequest{
					Logger:                 taskLogger,
					Pipeline:               &pipeline,
					Step:                   step,
					Context:                stepContext,
					History:                conditionHistorySnapshot,
					Secrets:                req.Secrets,
					KnowledgePrompt:        req.knowledgePrompt(&pipeline, step, nil),
					BlockingKnowledgeKinds: conditionBlockingKinds,
					LLMTimeout:             req.LLMTimeout,
					LLMEnabled:             req.PipelineLLMEnabled,
					ClientResolver:         req.ConditionClientResolver,
					StopRetry:              req.StopRetry,
				})
				llmDurationMs = conditionResult.LLMDurationMs
				if conditionResult.Terminal {
					conditionFailureIgnored := false
					conditionFailureCanBeIgnored := ignoreFailure && conditionResult.Failed && len(conditionBlockingKinds) == 0
					if conditionResult.PipelineFailed && !conditionFailureCanBeIgnored {
						pipelineFailed.Store(true)
					}
					if conditionResult.FinalizeStatus != "" || conditionFailureCanBeIgnored {
						conditionFailureIgnored = finalizeFailure(conditionResult.FinalizeStatus, conditionResult.FinalizeExitCode, llmDurationMs, conditionFailureCanBeIgnored)
					}
					results <- taskResult{name: runnable.GlobalKey, success: !conditionResult.Failed || conditionFailureIgnored, skipped: conditionResult.Skipped}
					return
				}

				if approvalStep, ok := step.AsApprovalStep(); ok {
					req.setTaskRunning(activeTasks, stepName, task.Name)

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
						ignored := finalizeFailure("failure", 1, llmDurationMs, true)
						results <- taskResult{name: runnable.GlobalKey, success: ignored}
						return
					}
					includeContext := timeoutController.ContextOrDefault(context.Background())
					includeVariables, sensitiveIncludeVariables := stepContext.SelectedVariableOverrides(step.GetVariables())
					includeResult := req.IncludeRunner.Run(includeContext, includeflow.Request{
						Logger:             taskLogger,
						ParentRunID:        runID,
						ParentPipelineName: req.parentPipelineName(),
						StepName:           stepName,
						IncludeTarget:      includeTarget,
						History:            historySnapshot,
						Variables:          includeVariables,
						SensitiveVariables: sensitiveIncludeVariables,
						Sync:               step.GetSync(),
						LLMDurationMs:      llmDurationMs,
						FinalizeTask: func(stepName, taskName, status string, exitCode int, llmDurationMs int64) {
							req.finalizeTask(activeTasks, stepName, taskName, failureStatusWithTolerance(status, ignoreFailure), exitCode, llmDurationMs)
						},
						MarkPipelineFailed: func(status string) {
							if failureStatusWithTolerance(status, ignoreFailure) != "failure (ignored)" {
								pipelineFailed.Store(true)
							}
						},
					})
					if !includeResult.Success && failureStatusWithTolerance(includeResult.Status, ignoreFailure) == "failure (ignored)" {
						taskLogger.Warn().Msg("Task failed, but failure is ignored")
						results <- taskResult{name: runnable.GlobalKey, success: true}
						return
					}
					results <- taskResult{name: runnable.GlobalKey, success: includeResult.Success}
					return
				}

				req.setTaskRunning(activeTasks, stepName, task.Name)

				stepRuntimeVars := stepContext.ContainerVariables()
				taskContext := stepContext.WithTask(task)
				if resolvedOutputs, err := resolveRuntimeOutputVariables(task.Variables, runtimeOutputs); err != nil {
					taskLogger.Error().Err(err).Msg("Failed to resolve task runtime output variables")
					ignored := finalizeFailure("failure", 1, llmDurationMs, true)
					results <- taskResult{name: runnable.GlobalKey, success: ignored}
					return
				} else {
					for variableName, output := range resolvedOutputs {
						taskContext.SetValue(variableName, output.Value, output.Sensitive)
					}
				}
				taskRuntimeVars := taskContext.ContainerVariables()

				imageName := step.GetImage()
				if imageName == "" {
					imageName = pipeline.ContainerImage
				}

				taskOutputs := task.Outputs
				outputsEnabled := len(taskOutputs) > 0
				sessionKey := stepName
				runtimeStepName := stepName
				if outputsEnabled {
					sessionKey = stepName + "/" + task.Name
					runtimeStepName = stepName + "-" + task.Name
				}
				stepSessionID, createdSession, err := sessionRegistry.GetOrCreate(sessionKey, func() (string, error) {
					if req.StepRuntime == nil {
						return "", fmt.Errorf("step runtime is not configured")
					}
					return req.StepRuntime.CreateSession(context.Background(), taskLogger, StepRuntimeSessionRequest{
						RunID:            runID,
						PipelineName:     pipeline.Name,
						StepName:         runtimeStepName,
						GitRepoName:      req.env("GIT_REPO_NAME"),
						Image:            imageName,
						WorkingDirectory: req.WorkingDirectory,
						Env:              stepRuntimeVars,
						Volumes:          step.GetVolumes(),
						OutputsEnabled:   outputsEnabled,
						RuntimePool:      firstNonEmpty(step.GetRuntimePool(), pipeline.RuntimePool),
					})
				})
				if err != nil {
					taskLogger.Error().Err(err).Msgf("Failed to create step %s", stepRuntimeResourceName(req.StepRuntime))
					ignored := finalizeFailure("failure", 1, llmDurationMs, true)
					results <- taskResult{name: runnable.GlobalKey, success: ignored}
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
				taskBlockingKinds := req.blockingKnowledgeKinds(&pipeline, step, task)
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
					BlockingKnowledgeKinds: taskBlockingKinds,
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
					actionFailureIgnored := false
					actionFailureCanBeIgnored := ignoreFailure && len(taskBlockingKinds) == 0 && !isRunStopping()
					if actionResult.FinalizeStatus != "" || actionFailureCanBeIgnored {
						actionFailureIgnored = finalizeFailure(actionResult.FinalizeStatus, actionResult.FinalizeExitCode, llmDurationMs, actionFailureCanBeIgnored)
					}
					results <- taskResult{name: runnable.GlobalKey, success: actionFailureIgnored}
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
				if err := resolver.ValidateReplaceFilePrecondition(actionResult.FilePrecondition, req.WorkspaceDir, snapshotWorkspaceRevision()); err != nil {
					stdout, stderr, exitCode = "", err.Error(), 1
				} else {
					for attempt := 0; attempt < 10; attempt++ {
						if req.StepRuntime == nil {
							stdout, stderr, exitCode = "", "step runtime is not configured", 1
						} else {
							if outputsEnabled {
								if err := req.StepRuntime.PrepareOutputDirectory(context.Background(), stepSessionID); err != nil {
									stdout, stderr, exitCode = "", err.Error(), 1
									break
								}
							}
							actionExecuted = true
							liveOutput := func(stream executor.OutputStream, line string) {
								maskedLine := taskContext.MaskRuntimeText(line, req.Secrets)
								logLiveActionOutput(taskLogger, stepName, task.Name, stream, maskedLine)
							}
							stdout, stderr, exitCode = req.StepRuntime.ExecuteAction(context.Background(), stepSessionID, action, taskRuntimeVars, req.WorkingDirectory, liveOutput)
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
				if actionExecuted && exitCode == 0 && outputsEnabled {
					requiredForTask := outputRequiredByName(requiredRuntimeOutputs, stepName, task.Name)
					collected, err := req.StepRuntime.CollectOutputs(context.Background(), stepSessionID, taskOutputs, requiredForTask, req.RuntimeOutputMaxBytes)
					if err != nil {
						stdout, stderr, exitCode = "", err.Error(), 1
					} else if len(collected) > 0 {
						runtimeOutputs.Set(stepName, task.Name, collected)
						for _, output := range collected {
							if output.Sensitive {
								taskContext.SetValue(output.Name, output.Value, true)
							}
						}
						if err := req.reportTaskOutputs(stepName, task.Name, collected); err != nil {
							taskLogger.Error().Err(err).Strs("outputs", sortedRuntimeOutputNames(collected)).Msg("Failed to persist runtime outputs")
							stdout, stderr, exitCode = "", fmt.Sprintf("persist runtime outputs: %v", err), 1
						} else {
							taskLogger.Info().Strs("outputs", sortedRuntimeOutputNames(collected)).Msg("Collected runtime outputs")
						}
					}
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
					ignored := finalizeFailure("failure", exitCode, llmDurationMs, true)
					if !ignored {
						taskLogger.Error().Msg("Critical task failed")
					}
					results <- taskResult{name: runnable.GlobalKey, success: ignored}
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

func logLiveActionOutput(logger *zerolog.Logger, stepName, taskName string, stream executor.OutputStream, line string) {
	if logger == nil || strings.TrimSpace(line) == "" {
		return
	}
	level := detectLiveActionLogLevel(stream, line)
	event := logger.Info()
	switch level {
	case "error":
		event = logger.Error()
	case "warn":
		event = logger.Warn()
	}
	event.
		Str("component", "action-output").
		Str("stream", string(stream)).
		Str("step", stepName).
		Str("task", taskName).
		Str("output_level", level).
		Msg(line)
}

func detectLiveActionLogLevel(stream executor.OutputStream, line string) string {
	if level := normalizeLiveActionLogLevel(structuredLiveActionLogLevel(line)); level != "" {
		return level
	}
	if level := inferPlainTextLiveActionLogLevel(line); level != "" {
		return level
	}
	return "info"
}

func structuredLiveActionLogLevel(line string) string {
	jsonStart := strings.Index(line, "{")
	if jsonStart == -1 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(line[jsonStart:]), &payload); err != nil {
		return ""
	}
	if level := firstLiveActionLogString(payload, "output_level", "level", "severity"); level != "" {
		return level
	}
	if meta, ok := payload["meta"].(map[string]any); ok {
		return firstLiveActionLogString(meta, "output_level", "level", "severity")
	}
	return ""
}

func firstLiveActionLogString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			if str, ok := value.(string); ok && strings.TrimSpace(str) != "" {
				return str
			}
		}
	}
	return ""
}

func normalizeLiveActionLogLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "info", "warn", "error", "debug":
		return strings.ToLower(strings.TrimSpace(level))
	case "warning":
		return "warn"
	case "fatal", "panic":
		return "error"
	case "trace":
		return "debug"
	default:
		return ""
	}
}

func inferPlainTextLiveActionLogLevel(line string) string {
	for _, field := range strings.FieldsFunc(strings.ToLower(line), func(r rune) bool {
		return !unicode.IsLetter(r)
	}) {
		switch field {
		case "info", "warn", "error", "debug":
			return field
		case "warning":
			return "warn"
		case "fatal", "panic":
			return "error"
		case "trace":
			return "debug"
		}
	}
	return ""
}

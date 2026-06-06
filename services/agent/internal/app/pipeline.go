package app

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	"nopsai/services/agent/internal/approval"
	"nopsai/services/agent/internal/dockerexec"
	"nopsai/services/agent/internal/executor"
	includeflow "nopsai/services/agent/internal/include"
	"nopsai/services/agent/internal/kubernetesexec"
	"nopsai/services/agent/internal/resolver"
	"nopsai/services/agent/internal/scheduler"

	"github.com/rs/zerolog"
)

type AgentLogger func(runID, pipeline string) *zerolog.Logger
type StepLogger func(runID, pipeline, step, task string) *zerolog.Logger
type TaskStatusReporter func(pipelineName, runID, stepName, taskName, status string, exitCode int, llmDurationMs int64)
type FinalStatusNotifier func(pipelineName, runID, status string)
type RunCancellationWatcher func(pipelineName, runID string, onCancel func())
type EnvironmentProvider func() []string
type ExitFunc func(code int)

type KnowledgePromptBuilder func(*models.Pipeline, *models.PipelineStep, *models.Task, []models.KnowledgeContextSnapshot) string
type BlockingKnowledgeKindResolver func(*models.Pipeline, *models.PipelineStep, *models.Task, []models.KnowledgeContextSnapshot) []string
type KnowledgeViolationDetector func(*proto.Action, *models.Pipeline, *models.PipelineStep, *models.Task, []models.KnowledgeContextSnapshot) (string, []string, bool)

type ApprovalPauser interface {
	Pause(context.Context, approval.Request) (approval.PauseResponse, error)
}

type IncludeRunner interface {
	Run(context.Context, includeflow.Request) includeflow.Result
}

type StepRuntime interface {
	Name() string
	PrePullImages(context.Context, zerolog.Logger, *models.Pipeline, int)
	CreateSession(context.Context, *zerolog.Logger, StepRuntimeSessionRequest) (string, error)
	ExecuteAction(context.Context, string, *proto.Action, []string, string) (string, string, int)
	CleanupSession(context.Context, *zerolog.Logger, string)
}

type StepRuntimeSessionRequest struct {
	RunID            string
	PipelineName     string
	StepName         string
	GitRepoName      string
	Image            string
	WorkingDirectory string
	Env              []string
	Volumes          []string
	RuntimePool      string
}

type ContainerStepRuntimeOptions struct {
	SharedVolumeName  string
	DockerNetworkName string
	GitRepoName       string
}

type ContainerStepRuntime struct {
	runtime           *ExecutionRuntime
	sharedVolumeName  string
	dockerNetworkName string
	gitRepoName       string
}

func NewContainerStepRuntime(runtime *ExecutionRuntime, opts ContainerStepRuntimeOptions) ContainerStepRuntime {
	return ContainerStepRuntime{
		runtime:           runtime,
		sharedVolumeName:  opts.SharedVolumeName,
		dockerNetworkName: opts.DockerNetworkName,
		gitRepoName:       opts.GitRepoName,
	}
}

func (r ContainerStepRuntime) Name() string {
	if r.runtime != nil && r.runtime.Kubernetes != nil {
		return kubernetesexec.RuntimeName
	}
	return "docker"
}

func (r ContainerStepRuntime) PrePullImages(ctx context.Context, logger zerolog.Logger, pipeline *models.Pipeline, totalTasks int) {
	if r.runtime == nil || r.runtime.Docker == nil {
		return
	}
	queue := scheduler.ImagePullQueue(pipeline, totalTasks)
	if len(queue) == 0 {
		logger.Debug().Msg("No images to pre-pull for pipeline")
		return
	}
	prePullLogger := logger.With().Str("component", "image-prepull").Logger()
	dockerexec.StartImagePrePull(ctx, prePullLogger, r.runtime.Docker, queue)
}

func (r ContainerStepRuntime) CreateSession(ctx context.Context, logger *zerolog.Logger, req StepRuntimeSessionRequest) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r.runtime == nil {
		return "", fmt.Errorf("execution runtime is not configured")
	}
	if r.runtime.Kubernetes != nil {
		if logger != nil {
			logger.Info().Str("image", req.Image).Msg("Creating new Kubernetes pod for step")
		}
		return r.runtime.Kubernetes.CreateStepPod(ctx, kubernetesexec.StepPodRequest{
			RunID:            req.RunID,
			PipelineName:     req.PipelineName,
			StepName:         req.StepName,
			Image:            req.Image,
			WorkingDirectory: req.WorkingDirectory,
			Env:              req.Env,
			Volumes:          req.Volumes,
			RuntimePool:      req.RuntimePool,
		})
	}
	if r.runtime.Docker == nil {
		return "", fmt.Errorf("Docker client is not configured")
	}
	if logger != nil {
		logger.Info().Str("image", req.Image).Msg("Creating new container for step")
	}
	stepContainerName := dockerexec.BuildStepContainerName(firstNonEmpty(req.GitRepoName, r.gitRepoName), req.PipelineName, req.StepName, req.RunID)
	return dockerexec.CreateStepContainer(ctx, logger, r.runtime.Docker, dockerexec.StepContainerRequest{
		Image:             req.Image,
		WorkingDirectory:  req.WorkingDirectory,
		Env:               req.Env,
		Volumes:           req.Volumes,
		SharedVolumeName:  r.sharedVolumeName,
		DockerNetworkName: r.dockerNetworkName,
		ContainerName:     stepContainerName,
	})
}

func (r ContainerStepRuntime) ExecuteAction(ctx context.Context, sessionID string, action *proto.Action, runtimeVars []string, workingDirectory string) (string, string, int) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r.runtime != nil && r.runtime.Kubernetes != nil {
		return r.runtime.Kubernetes.ExecuteAction(ctx, sessionID, action, runtimeVars, workingDirectory)
	}
	if r.runtime == nil || r.runtime.Docker == nil {
		return "", "execution runtime is not configured", 1
	}
	return executor.ExecuteDockerAction(ctx, r.runtime.Docker, sessionID, action, runtimeVars, workingDirectory)
}

func (r ContainerStepRuntime) CleanupSession(ctx context.Context, logger *zerolog.Logger, sessionID string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r.runtime != nil && r.runtime.Kubernetes != nil {
		r.runtime.Kubernetes.CleanupPod(ctx, sessionID)
		return
	}
	if r.runtime == nil || r.runtime.Docker == nil {
		return
	}
	dockerexec.Cleanup(ctx, logger, r.runtime.Docker, sessionID)
}

type ApprovalResumeSnapshot struct {
	ExecutionHistory string
	CompletedTasks   []string
}

type PipelineRunRequest struct {
	RunID                  string
	PipelineName           string
	PipelineDefinitionYAML []byte
	ParentHistoryBase64    string
	PipelineTimeout        string
	SharedVolumeName       string
	WorkspaceDir           string
	WorkingDirectory       string
	RunnerID               string
	Pipeline               models.Pipeline
	Variables              map[string]string
	Secrets                map[string]string
	ResumeCheckpoint       *ApprovalResumeSnapshot
	KnowledgeSnapshots     []models.KnowledgeContextSnapshot
	PipelineLLMEnabled     bool
	LLMTimeout             time.Duration

	StepRuntime             StepRuntime
	ConditionClientResolver resolver.ConditionClientResolver
	ActionSessionResolver   resolver.ActionSessionResolver
	ApprovalPauser          ApprovalPauser
	IncludeRunner           IncludeRunner
	DirectoryLister         resolver.DirectoryLister
	StopRetry               func(error) bool

	Logger                 AgentLogger
	StepLogger             StepLogger
	UpdateTaskStatus       TaskStatusReporter
	NotifyFinalStatus      FinalStatusNotifier
	WatchRunCancellation   RunCancellationWatcher
	Env                    EnvLookup
	Environment            EnvironmentProvider
	Exit                   ExitFunc
	KnowledgePrompt        KnowledgePromptBuilder
	BlockingKnowledgeKinds BlockingKnowledgeKindResolver
	KnowledgeViolation     KnowledgeViolationDetector
}

type PipelineRunResult struct {
	ExitCode    int
	FinalStatus string
	Paused      bool
}

type taskResult struct {
	name    string
	success bool
	skipped bool
	paused  bool
}

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

	completedTasks := make(map[string]bool)
	if req.ResumeCheckpoint != nil {
		for _, key := range req.ResumeCheckpoint.CompletedTasks {
			key = strings.TrimSpace(key)
			if key != "" {
				completedTasks[key] = true
			}
		}
	}

	var pipelineFailed atomic.Bool
	pipelinePaused := false
	var syncWg sync.WaitGroup
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

				historyMutex.Lock()
				conditionHistorySnapshot := history.String()
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
						Strs("groups", approvalStep.Approval.Groups).
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
						SyncWaitGroup:      &syncWg,
						FinalizeTask: func(stepName, taskName, status string, exitCode int, llmDurationMs int64) {
							req.finalizeTask(activeTasks, stepName, taskName, status, exitCode, llmDurationMs)
						},
						MarkPipelineFailed: func() { pipelineFailed.Store(true) },
					})
					results <- taskResult{name: runnable.GlobalKey, success: includeResult.Success}
					return
				}

				req.setTaskRunning(activeTasks, stepName, task.Name)

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
					WorkspaceDir:    req.WorkspaceDir,
					IsRunStopping:   isRunStopping,
					Secrets:         req.Secrets,
					KnowledgePrompt: req.knowledgePrompt(&pipeline, step, task),
					LLMTimeout:      req.LLMTimeout,
					LLMEnabled:      req.PipelineLLMEnabled,
					SessionResolver: req.ActionSessionResolver,
					DirectoryLister: req.DirectoryLister,
					StopRetry:       req.StopRetry,
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
					historyMutex.Unlock()
					req.finalizeTask(activeTasks, stepName, task.Name, "failure", 1, llmDurationMs)
					results <- taskResult{name: runnable.GlobalKey, success: false}
					return
				}

				debugLogger := taskLogger.With().
					Str("action_type", action.Type).
					Logger()
				debugLogger.Debug().Msgf("Executing action: %s", actionStr)

				var stdout, stderr string
				var exitCode int

				for attempt := 0; attempt < 10; attempt++ {
					if req.StepRuntime == nil {
						stdout, stderr, exitCode = "", "step runtime is not configured", 1
					} else {
						stdout, stderr, exitCode = req.StepRuntime.ExecuteAction(context.Background(), stepSessionID, action, taskRuntimeVars, req.WorkingDirectory)
					}
					if exitCode == 0 {
						break
					}
					time.Sleep(time.Duration(attempt*100) * time.Millisecond)
				}

				status := "success"
				output := stdout
				if exitCode != 0 {
					status = "failure"
					output = stderr + stdout
				}
				maskedOutput := taskContext.MaskText(output, req.Secrets)
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
				history.WriteString(fmt.Sprintf("- Goal: %s\n  Action: %s\n  Result (Exit Code %d): %s\n", historyGoal, actionStr, exitCode, output))
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

	syncWg.Wait()

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

func (req PipelineRunRequest) setTaskRunning(activeTasks *ActiveTaskTracker, stepName, taskName string) {
	req.reportTaskStatus(stepName, taskName, "running", 0, 0)
	activeTasks.Add(stepName, taskName)
}

func (req PipelineRunRequest) finalizeTask(activeTasks *ActiveTaskTracker, stepName, taskName, status string, exitCode int, llmDurationMs int64) {
	req.reportTaskStatus(stepName, taskName, status, exitCode, llmDurationMs)
	activeTasks.Remove(stepName, taskName)
}

func (req PipelineRunRequest) agentLogger() *zerolog.Logger {
	if req.Logger != nil {
		if logger := req.Logger(req.RunID, req.Pipeline.Name); logger != nil {
			return logger
		}
	}
	logger := zerolog.Nop()
	return &logger
}

func (req PipelineRunRequest) stepLogger(stepName, taskName string) *zerolog.Logger {
	if req.StepLogger != nil {
		if logger := req.StepLogger(req.RunID, req.Pipeline.Name, stepName, taskName); logger != nil {
			return logger
		}
	}
	return req.agentLogger()
}

func (req PipelineRunRequest) reportTaskStatus(stepName, taskName, status string, exitCode int, llmDurationMs int64) {
	if req.UpdateTaskStatus == nil {
		return
	}
	req.UpdateTaskStatus(req.Pipeline.Name, req.RunID, stepName, taskName, status, exitCode, llmDurationMs)
}

func (req PipelineRunRequest) notifyFinalStatus(status string) {
	if req.NotifyFinalStatus == nil {
		return
	}
	req.NotifyFinalStatus(req.Pipeline.Name, req.RunID, status)
}

func (req PipelineRunRequest) knowledgePrompt(pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task) string {
	if req.KnowledgePrompt == nil {
		return ""
	}
	return req.KnowledgePrompt(pipeline, step, task, req.KnowledgeSnapshots)
}

func (req PipelineRunRequest) blockingKnowledgeKinds(pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task) []string {
	if req.BlockingKnowledgeKinds == nil {
		return nil
	}
	return req.BlockingKnowledgeKinds(pipeline, step, task, req.KnowledgeSnapshots)
}

func (req PipelineRunRequest) knowledgeViolation(action *proto.Action, pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task) (string, []string, bool) {
	if req.KnowledgeViolation == nil {
		return "", nil, false
	}
	return req.KnowledgeViolation(action, pipeline, step, task, req.KnowledgeSnapshots)
}

func (req PipelineRunRequest) environment() []string {
	if req.Environment != nil {
		return req.Environment()
	}
	return os.Environ()
}

func (req PipelineRunRequest) env(key string) string {
	if req.Env != nil {
		return req.Env(key)
	}
	return os.Getenv(key)
}

func (req PipelineRunRequest) runnerID() string {
	return firstNonEmpty(req.RunnerID, req.env("RUNNER_ID"))
}

func (req PipelineRunRequest) parentPipelineName() string {
	return firstNonEmpty(req.PipelineName, req.Pipeline.Name)
}

func (req PipelineRunRequest) exit(code int) {
	if req.Exit != nil {
		req.Exit(code)
		return
	}
	os.Exit(code)
}

func stepRuntimeResourceName(runtime StepRuntime) string {
	if runtime != nil && runtime.Name() == kubernetesexec.RuntimeName {
		return "pod"
	}
	return "container"
}

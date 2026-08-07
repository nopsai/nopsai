package app

import (
	"os"
	"strings"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	"nopsai/services/agent/internal/kubernetesexec"

	"github.com/rs/zerolog"
)

func (req PipelineRunRequest) setTaskRunning(activeTasks *ActiveTaskTracker, stepName, taskName string) {
	req.reportTaskStatus(stepName, taskName, "running", 0, 0)
	activeTasks.Add(stepName, taskName)
}

func (req PipelineRunRequest) finalizeTask(activeTasks *ActiveTaskTracker, stepName, taskName, status string, exitCode int, llmDurationMs int64) {
	req.reportTaskStatus(stepName, taskName, status, exitCode, llmDurationMs)
	activeTasks.Remove(stepName, taskName)
}

func effectiveIgnoreFailure(step *models.PipelineStep, task *models.Task) bool {
	if task != nil && task.IgnoreFailure {
		return true
	}
	return step != nil && step.GetIgnoreFailure()
}

func failureStatusWithTolerance(status string, ignoreFailure bool) string {
	if !ignoreFailure || !isIgnorableFailureStatus(status) {
		return status
	}
	return "failure (ignored)"
}

func isIgnorableFailureStatus(status string) bool {
	normalized := strings.ToLower(strings.TrimSpace(status))
	if normalized == "failure" {
		return true
	}
	if strings.Contains(normalized, "ignored") {
		return false
	}
	return strings.Contains(normalized, "fail") ||
		strings.Contains(normalized, "error") ||
		strings.Contains(normalized, "not_found") ||
		normalized == "timed_out" ||
		strings.Contains(normalized, "timeout") ||
		strings.Contains(normalized, "timed out")
}

func isWarningRunStatus(status string) bool {
	normalized := strings.ToLower(strings.TrimSpace(status))
	return normalized == "warning" || normalized == "failure (ignored)"
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

func (req PipelineRunRequest) reportTaskOutputs(stepName, taskName string, outputs map[string]RuntimeOutputValue) error {
	if req.ReportTaskOutputs == nil || len(outputs) == 0 {
		return nil
	}
	return req.ReportTaskOutputs(req.Pipeline.Name, req.RunID, stepName, taskName, outputs)
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

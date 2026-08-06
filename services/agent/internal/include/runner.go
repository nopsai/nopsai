package include

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
)

type DefinitionFetcher func(context.Context, string) ([]byte, error)
type PipelineTrigger func(context.Context, string, string, string, string, []byte, string, map[string]string, []string) (string, error)
type PipelineMonitor func(context.Context, *zerolog.Logger, string) (string, error)
type RuntimeOutputFetcher func(context.Context, string, string, string, []string) (map[string]RuntimeOutput, error)
type Finalizer func(stepName, taskName, status string, exitCode int, llmDurationMs int64)
type NotFoundClassifier func(error) bool

type Config struct {
	FetchDefinition DefinitionFetcher
	TriggerPipeline PipelineTrigger
	MonitorPipeline PipelineMonitor
	FetchOutputs    RuntimeOutputFetcher
	IsNotFound      NotFoundClassifier
}

type Runner struct {
	config Config
}

type Request struct {
	Logger             *zerolog.Logger
	ParentRunID        string
	ParentPipelineName string
	StepName           string
	IncludeTarget      string
	History            string
	Variables          map[string]string
	SensitiveVariables []string
	OutputNames        []string
	Sync               bool
	LLMDurationMs      int64
	FinalizeTask       Finalizer
	MarkPipelineFailed func(string)
}

type RuntimeOutput struct {
	Name      string
	Value     string
	Sensitive bool
	SizeBytes int64
}

type Result struct {
	Handled bool
	Success bool
	Status  string
	Outputs map[string]RuntimeOutput
}

func NewRunner(config Config) Runner {
	return Runner{config: config}
}

func (r Runner) Run(ctx context.Context, req Request) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	includeTarget := strings.TrimSpace(req.IncludeTarget)
	if includeTarget == "" {
		return Result{}
	}

	parts := strings.SplitN(includeTarget, ":", 2)
	if len(parts) != 2 || parts[0] != "pipeline" {
		if req.Logger != nil {
			req.Logger.Error().Str("include", includeTarget).Msg("Invalid include format")
		}
		req.finalize(req.StepName, req.StepName, "failure", 1)
		return Result{Handled: true, Success: false, Status: "failure"}
	}
	childPipelineName := parts[1]
	outputNames := normalizedOutputNames(req.OutputNames)
	if len(outputNames) > 0 && !req.Sync {
		r.logError(req.Logger, fmt.Errorf("parent-visible child pipeline outputs require sync: true"), "Invalid child pipeline output configuration")
		req.finalize(req.StepName, req.StepName, "failure", 1)
		return Result{Handled: true, Success: false, Status: "failure"}
	}

	if r.config.FetchDefinition == nil {
		r.logError(req.Logger, fmt.Errorf("child pipeline definition fetcher is not configured"), "Failed to get child pipeline definition")
		req.finalize(req.StepName, req.StepName, "failure", 1)
		return Result{Handled: true, Success: false, Status: "failure"}
	}
	childDef, err := r.config.FetchDefinition(ctx, childPipelineName)
	if err != nil {
		if r.config.IsNotFound != nil && r.config.IsNotFound(err) {
			if req.Logger != nil {
				req.Logger.Warn().Str("child_pipeline", childPipelineName).Msg("Child pipeline not found, marking as not found")
			}
			req.finalize(req.StepName, req.StepName, "not_found", 0)
			return Result{Handled: true, Success: false, Status: "not_found"}
		}

		r.logError(req.Logger, err, "Failed to get child pipeline definition")
		req.finalize(req.StepName, req.StepName, "failure", 1)
		return Result{Handled: true, Success: false, Status: "failure"}
	}

	if r.config.TriggerPipeline == nil {
		r.logError(req.Logger, fmt.Errorf("child pipeline trigger is not configured"), "Failed to trigger child pipeline")
		req.finalize(req.StepName, req.StepName, "failure", 1)
		return Result{Handled: true, Success: false, Status: "failure"}
	}
	childRunID, err := r.config.TriggerPipeline(ctx, req.ParentRunID, req.ParentPipelineName, req.StepName, childPipelineName, childDef, req.History, req.Variables, req.SensitiveVariables)
	if err != nil {
		r.logError(req.Logger, err, "Failed to trigger child pipeline")
		req.finalize(req.StepName, req.StepName, "failure", 1)
		return Result{Handled: true, Success: false, Status: "failure"}
	}

	if req.Sync {
		finalStatus := r.waitForChild(ctx, req, childRunID)
		exitCode := 0
		outputs := map[string]RuntimeOutput(nil)
		if finalStatus == "success" && len(outputNames) > 0 {
			if r.config.FetchOutputs == nil {
				r.logError(req.Logger, fmt.Errorf("child pipeline output fetcher is not configured"), "Failed to resolve child pipeline outputs")
				finalStatus = "failure"
				exitCode = 1
			} else {
				var err error
				outputs, err = r.config.FetchOutputs(ctx, req.ParentRunID, req.StepName, childRunID, outputNames)
				if err != nil {
					r.logErrorWithRunID(req.Logger, err, childRunID, "Failed to resolve child pipeline outputs")
					finalStatus = "failure"
					exitCode = 1
				}
			}
		}
		req.finalize(req.StepName, req.StepName, finalStatus, exitCode)
		success := finalStatus == "success"
		if !success && req.MarkPipelineFailed != nil {
			req.MarkPipelineFailed(finalStatus)
		}
		return Result{Handled: true, Success: success, Status: finalStatus, Outputs: outputs}
	}

	go r.monitor(ctx, req, childRunID)
	req.finalize(req.StepName, req.StepName, "success", 0)
	return Result{Handled: true, Success: true, Status: "success"}
}

func (r Runner) monitor(ctx context.Context, req Request, childRunID string) string {
	finalStatus := r.waitForChild(ctx, req, childRunID)
	req.finalize(req.StepName, req.StepName, finalStatus, 0)
	return finalStatus
}

func (r Runner) waitForChild(ctx context.Context, req Request, childRunID string) string {
	finalStatus := "failure"
	var err error
	if r.config.MonitorPipeline == nil {
		err = fmt.Errorf("child pipeline monitor is not configured")
	} else {
		finalStatus, err = r.config.MonitorPipeline(ctx, req.Logger, childRunID)
	}
	if err != nil {
		r.logErrorWithRunID(req.Logger, err, childRunID, "Error monitoring child pipeline")
		finalStatus = "failure"
	}
	if req.Logger != nil {
		req.Logger.Info().Str("child_run_id", childRunID).Str("status", finalStatus).Msg("Child pipeline finished")
	}
	return finalStatus
}

func normalizedOutputNames(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	names := make([]string, 0, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

func (req Request) finalize(stepName, taskName, status string, exitCode int) {
	if req.FinalizeTask == nil {
		return
	}
	req.FinalizeTask(stepName, taskName, status, exitCode, req.LLMDurationMs)
}

func (Runner) logError(logger *zerolog.Logger, err error, message string) {
	if logger != nil {
		logger.Error().Err(err).Msg(message)
	}
}

func (Runner) logErrorWithRunID(logger *zerolog.Logger, err error, runID, message string) {
	if logger != nil {
		logger.Error().Err(err).Str("child_run_id", runID).Msg(message)
	}
}

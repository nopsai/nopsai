package include

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

type DefinitionFetcher func(context.Context, string) ([]byte, error)
type PipelineTrigger func(context.Context, string, string, string, string, []byte, string) (string, error)
type PipelineMonitor func(context.Context, *zerolog.Logger, string) (string, error)
type Finalizer func(stepName, taskName, status string, exitCode int, llmDurationMs int64)
type NotFoundClassifier func(error) bool

type Config struct {
	FetchDefinition DefinitionFetcher
	TriggerPipeline PipelineTrigger
	MonitorPipeline PipelineMonitor
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
	Sync               bool
	LLMDurationMs      int64
	SyncWaitGroup      *sync.WaitGroup
	FinalizeTask       Finalizer
	MarkPipelineFailed func()
}

type Result struct {
	Handled bool
	Success bool
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
		return Result{Handled: true, Success: false}
	}
	childPipelineName := parts[1]

	if r.config.FetchDefinition == nil {
		r.logError(req.Logger, fmt.Errorf("child pipeline definition fetcher is not configured"), "Failed to get child pipeline definition")
		req.finalize(req.StepName, req.StepName, "failure", 1)
		return Result{Handled: true, Success: false}
	}
	childDef, err := r.config.FetchDefinition(ctx, childPipelineName)
	if err != nil {
		if r.config.IsNotFound != nil && r.config.IsNotFound(err) {
			if req.Logger != nil {
				req.Logger.Warn().Str("child_pipeline", childPipelineName).Msg("Child pipeline not found, marking as not found")
			}
			req.finalize(req.StepName, req.StepName, "not_found", 0)
			return Result{Handled: true, Success: false}
		}

		r.logError(req.Logger, err, "Failed to get child pipeline definition")
		req.finalize(req.StepName, req.StepName, "failure", 1)
		return Result{Handled: true, Success: false}
	}

	if r.config.TriggerPipeline == nil {
		r.logError(req.Logger, fmt.Errorf("child pipeline trigger is not configured"), "Failed to trigger child pipeline")
		req.finalize(req.StepName, req.StepName, "failure", 1)
		return Result{Handled: true, Success: false}
	}
	childRunID, err := r.config.TriggerPipeline(ctx, req.ParentRunID, req.ParentPipelineName, req.StepName, childPipelineName, childDef, req.History)
	if err != nil {
		r.logError(req.Logger, err, "Failed to trigger child pipeline")
		req.finalize(req.StepName, req.StepName, "failure", 1)
		return Result{Handled: true, Success: false}
	}

	monitorFunc := r.monitorFunc(ctx, req, childRunID)
	if req.Sync {
		if req.SyncWaitGroup != nil {
			req.SyncWaitGroup.Add(1)
			go func() {
				defer req.SyncWaitGroup.Done()
				monitorFunc()
			}()
		} else {
			go monitorFunc()
		}
		return Result{Handled: true, Success: true}
	}

	go monitorFunc()
	req.finalize(req.StepName, req.StepName, "success", 0)
	return Result{Handled: true, Success: true}
}

func (r Runner) monitorFunc(ctx context.Context, req Request, childRunID string) func() {
	return func() {
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
		req.finalize(req.StepName, req.StepName, finalStatus, 0)
		if finalStatus != "success" && req.Sync && req.MarkPipelineFailed != nil {
			req.MarkPipelineFailed()
		}
	}
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

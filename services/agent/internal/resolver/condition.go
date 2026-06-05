package resolver

import (
	"context"
	"strings"
	"time"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"

	"github.com/rs/zerolog"
)

type ConditionClient interface {
	EvaluateCondition(context.Context, *proto.ConditionRequest) (*proto.ConditionResponse, error)
}

type ConditionClientResolver func(*models.Pipeline, *models.PipelineStep, *models.Task) (ConditionClient, string, error)

type ConditionRequest struct {
	Logger                 *zerolog.Logger
	Pipeline               *models.Pipeline
	Step                   *models.PipelineStep
	Context                ExecutionContext
	History                string
	Secrets                map[string]string
	KnowledgePrompt        string
	BlockingKnowledgeKinds []string
	LLMTimeout             time.Duration
	LLMEnabled             bool
	ClientResolver         ConditionClientResolver
	StopRetry              func(error) bool
}

type ConditionResult struct {
	Proceed          bool
	Terminal         bool
	Failed           bool
	Skipped          bool
	PipelineFailed   bool
	FinalizeStatus   string
	FinalizeExitCode int
	LLMDurationMs    int64
}

type ConditionEvaluator struct{}

func NewConditionEvaluator() ConditionEvaluator {
	return ConditionEvaluator{}
}

func (ConditionEvaluator) Evaluate(ctx context.Context, req ConditionRequest) ConditionResult {
	condition := ""
	stepName := ""
	if req.Step != nil {
		condition = req.Step.GetCondition()
		stepName = req.Step.GetName()
	}
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return ConditionResult{Proceed: true}
	}

	if !req.LLMEnabled || req.ClientResolver == nil {
		if req.Logger != nil {
			req.Logger.Error().Msg("Cannot evaluate condition because LLM is disabled for this pipeline")
		}
		return ConditionResult{
			Terminal:         true,
			Failed:           true,
			PipelineFailed:   true,
			FinalizeStatus:   "failure",
			FinalizeExitCode: 1,
		}
	}
	if req.Logger != nil {
		req.Logger.Info().Msgf("Evaluating condition for step '%s': \"%s\"", stepName, condition)
	}

	conditionClient, conditionProfile, profileErr := req.ClientResolver(req.Pipeline, req.Step, nil)
	if profileErr != nil {
		if req.Logger != nil {
			req.Logger.Error().Err(profileErr).Msg("Failed to resolve LLM profile for condition")
		}
		return ConditionResult{
			Terminal:       true,
			Failed:         true,
			PipelineFailed: true,
		}
	}
	if req.Logger != nil {
		req.Logger.Info().Str("llm_profile", conditionProfile).Msg("Using LLM profile for condition")
	}

	parentCtx := ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	llmTimeout := req.LLMTimeout
	if llmTimeout <= 0 {
		llmTimeout = 2 * time.Minute
	}

	conditionReq := req.Context.BuildConditionRequest(condition, req.History, req.KnowledgePrompt, req.Secrets)

	var resp *proto.ConditionResponse
	conditionStart := time.Now()
	err := withRetry(func() error {
		attemptCtx, cancel := context.WithTimeout(parentCtx, llmTimeout)
		defer cancel()
		var callErr error
		resp, callErr = conditionClient.EvaluateCondition(attemptCtx, conditionReq)
		return callErr
	}, 3, time.Second, req.StopRetry)
	llmDurationMs := time.Since(conditionStart).Milliseconds()

	if err != nil {
		if req.Logger != nil {
			req.Logger.Error().Err(err).Msg("Failed to evaluate condition from LLM. Skipping step.")
		}
		return ConditionResult{
			Terminal:       true,
			Failed:         true,
			PipelineFailed: true,
			LLMDurationMs:  llmDurationMs,
		}
	}

	if resp == nil || !resp.GetResult() {
		if len(req.BlockingKnowledgeKinds) > 0 {
			if req.Logger != nil {
				req.Logger.Error().
					Strs("knowledge_context_kinds", req.BlockingKnowledgeKinds).
					Msg("Condition evaluated to false under blocking knowledge context. Failing task.")
			}
			return ConditionResult{
				Terminal:         true,
				Failed:           true,
				FinalizeStatus:   "failure",
				FinalizeExitCode: 1,
				LLMDurationMs:    llmDurationMs,
			}
		}
		if req.Logger != nil {
			req.Logger.Info().Msg("Condition evaluated to false. Skipping task.")
		}
		return ConditionResult{
			Terminal:         true,
			Skipped:          true,
			FinalizeStatus:   "skipped",
			FinalizeExitCode: 0,
			LLMDurationMs:    llmDurationMs,
		}
	}

	if req.Logger != nil {
		req.Logger.Info().Msg("Condition evaluated to true. Proceeding with step.")
	}
	return ConditionResult{Proceed: true, LLMDurationMs: llmDurationMs}
}

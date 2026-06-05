package resolver

import (
	"context"
	"strings"
	"testing"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"

	"github.com/rs/zerolog"
)

type fakeConditionClient struct {
	response *proto.ConditionResponse
	requests []*proto.ConditionRequest
}

func (c *fakeConditionClient) EvaluateCondition(_ context.Context, req *proto.ConditionRequest) (*proto.ConditionResponse, error) {
	c.requests = append(c.requests, req)
	return c.response, nil
}

func TestConditionEvaluatorNoConditionProceedsWithoutLLM(t *testing.T) {
	logger := zerolog.Nop()
	step := &models.PipelineStep{Step: &models.GoalStep{
		BaseStep: models.BaseStep{Name: "build"},
		Goal:     "build",
	}}

	result := NewConditionEvaluator().Evaluate(context.Background(), ConditionRequest{
		Logger:     &logger,
		Pipeline:   &models.Pipeline{},
		Step:       step,
		Context:    NewExecutionContext(),
		LLMEnabled: false,
	})

	if !result.Proceed || result.Terminal {
		t.Fatalf("result = %#v, want proceed without terminal outcome", result)
	}
}

func TestConditionEvaluatorDisabledLLMReturnsFinalizableFailure(t *testing.T) {
	logger := zerolog.Nop()
	step := &models.PipelineStep{Step: &models.GoalStep{
		BaseStep: models.BaseStep{Name: "deploy", Condition: "should deploy?"},
		Goal:     "deploy",
	}}

	result := NewConditionEvaluator().Evaluate(context.Background(), ConditionRequest{
		Logger:     &logger,
		Pipeline:   &models.Pipeline{},
		Step:       step,
		Context:    NewExecutionContext(),
		LLMEnabled: false,
	})

	if !result.Terminal || !result.Failed || !result.PipelineFailed {
		t.Fatalf("result = %#v, want terminal pipeline failure", result)
	}
	if result.FinalizeStatus != "failure" || result.FinalizeExitCode != 1 {
		t.Fatalf("finalize result = %q/%d, want failure/1", result.FinalizeStatus, result.FinalizeExitCode)
	}
}

func TestConditionEvaluatorFalseConditionSkipsTask(t *testing.T) {
	logger := zerolog.Nop()
	client := &fakeConditionClient{response: &proto.ConditionResponse{Result: false}}
	step := &models.PipelineStep{Step: &models.GoalStep{
		BaseStep: models.BaseStep{Name: "deploy", Condition: "should deploy?"},
		Goal:     "deploy",
	}}
	executionContext := NewExecutionContext()
	executionContext.SetValue("API_TOKEN", "secret-token-value", true)

	result := NewConditionEvaluator().Evaluate(context.Background(), ConditionRequest{
		Logger:     &logger,
		Pipeline:   &models.Pipeline{},
		Step:       step,
		Context:    executionContext,
		History:    "token=secret-token-value",
		Secrets:    map[string]string{"API_TOKEN": "secret-token-value"},
		LLMEnabled: true,
		ClientResolver: func(*models.Pipeline, *models.PipelineStep, *models.Task) (ConditionClient, string, error) {
			return client, "unit", nil
		},
	})

	if !result.Terminal || !result.Skipped || result.Failed {
		t.Fatalf("result = %#v, want terminal skip", result)
	}
	if result.FinalizeStatus != "skipped" || result.FinalizeExitCode != 0 {
		t.Fatalf("finalize result = %q/%d, want skipped/0", result.FinalizeStatus, result.FinalizeExitCode)
	}
	if len(client.requests) != 1 {
		t.Fatalf("condition requests = %d, want 1", len(client.requests))
	}
	if strings.Contains(client.requests[0].GetHistory(), "secret-token-value") {
		t.Fatalf("condition history was not masked: %q", client.requests[0].GetHistory())
	}
}

func TestConditionEvaluatorFalseConditionFailsUnderBlockingKnowledgeContext(t *testing.T) {
	logger := zerolog.Nop()
	client := &fakeConditionClient{response: &proto.ConditionResponse{Result: false}}
	step := &models.PipelineStep{Step: &models.GoalStep{
		BaseStep: models.BaseStep{
			Name:      "deploy",
			Condition: "should deploy?",
			KnowledgeContext: []models.KnowledgeContextRef{
				{Kind: "guardrail", Ref: "team/runtime-output-safety"},
			},
		},
		Goal: "deploy",
	}}

	result := NewConditionEvaluator().Evaluate(context.Background(), ConditionRequest{
		Logger:                 &logger,
		Pipeline:               &models.Pipeline{},
		Step:                   step,
		Context:                NewExecutionContext(),
		BlockingKnowledgeKinds: []string{"guardrail"},
		LLMEnabled:             true,
		ClientResolver: func(*models.Pipeline, *models.PipelineStep, *models.Task) (ConditionClient, string, error) {
			return client, "unit", nil
		},
	})

	if !result.Terminal || !result.Failed || result.Skipped {
		t.Fatalf("result = %#v, want terminal failure", result)
	}
	if result.FinalizeStatus != "failure" || result.FinalizeExitCode != 1 {
		t.Fatalf("finalize result = %q/%d, want failure/1", result.FinalizeStatus, result.FinalizeExitCode)
	}
}

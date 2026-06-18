package agent

import (
	"strings"
	"testing"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"
)

func TestFormatKnowledgeContextPromptExplainsBlockingFailure(t *testing.T) {
	prompt := formatKnowledgeContextPrompt([]models.KnowledgeContextSnapshot{
		{
			Kind:    "guardrail",
			Name:    "runtime-output-safety",
			Ref:     "team/runtime-output-safety",
			Content: "Do not print runtime environment variables.",
		},
	})

	for _, want := range []string{
		"conflicts with guardrails or policies",
		"Before returning any action, inspect the exact structured action",
		"inspect the generated command_action.command text",
		"Guardrails and policies apply to the user's goal and to generated commands",
		"return RETURN_ANSWER instead",
		"agent will treat that response as a task failure",
		"runtime-output-safety",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want it to contain %q", prompt, want)
		}
	}
}

func TestEffectiveBlockingKnowledgeContextKindsUsesSelectedRefsOnly(t *testing.T) {
	pipeline := &models.Pipeline{
		KnowledgeContext: []models.KnowledgeContextRef{
			{Kind: "guideline", Ref: "team/report-style"},
		},
	}
	step := &models.PipelineStep{
		Step: &models.TaskStep{
			BaseStep: models.BaseStep{
				KnowledgeContext: []models.KnowledgeContextRef{
					{Kind: "guardrail", Ref: "team/runtime-output-safety"},
				},
			},
		},
	}
	task := &models.Task{
		KnowledgeContext: []models.KnowledgeContextRef{
			{Kind: "policy", Ref: "team/release-evidence"},
		},
	}
	snapshots := []models.KnowledgeContextSnapshot{
		{Kind: "guideline", Ref: "team/report-style", Content: "Write a friendly report."},
		{Kind: "guardrail", Ref: "team/runtime-output-safety", Content: "Do not print env vars."},
		{Kind: "policy", Ref: "team/release-evidence", Content: "Require release evidence."},
		{Kind: "guardrail", Ref: "team/not-selected", Content: "Not referenced."},
	}

	kinds := effectiveBlockingKnowledgeContextKinds(pipeline, step, task, snapshots)
	if got, want := strings.Join(kinds, ","), "guardrail,policy"; got != want {
		t.Fatalf("blocking kinds = %q, want %q", got, want)
	}
}

func TestKnowledgeContextViolationFailureReasonDetectsGuardrailReturnAnswer(t *testing.T) {
	pipeline := &models.Pipeline{}
	step := &models.PipelineStep{
		Step: &models.TaskStep{
			BaseStep: models.BaseStep{
				KnowledgeContext: []models.KnowledgeContextRef{
					{Kind: "guardrail", Ref: "team/runtime-output-safety"},
				},
			},
		},
	}
	task := &models.Task{Name: "blocked-env-request"}
	snapshots := []models.KnowledgeContextSnapshot{
		{Kind: "guardrail", Ref: "team/runtime-output-safety", Content: "Do not print environment variables."},
	}
	action := &proto.Action{
		Type: "RETURN_ANSWER",
		Payload: &proto.Action_AnswerAction{AnswerAction: &proto.AnswerAction{
			Answer: "I cannot run that command because it conflicts with the runtime output safety guardrail.",
		}},
	}

	reason, kinds, ok := knowledgeContextViolationFailureReason(action, pipeline, step, task, snapshots)
	if !ok {
		t.Fatal("expected guardrail conflict answer to fail the task")
	}
	if !strings.Contains(reason, "conflicts") {
		t.Fatalf("reason = %q, want conflict explanation", reason)
	}
	if got, want := strings.Join(kinds, ","), "guardrail"; got != want {
		t.Fatalf("blocking kinds = %q, want %q", got, want)
	}
}

func TestKnowledgeContextViolationFailureReasonAllowsNormalReturnAnswer(t *testing.T) {
	pipeline := &models.Pipeline{}
	step := &models.PipelineStep{
		Step: &models.TaskStep{
			BaseStep: models.BaseStep{
				KnowledgeContext: []models.KnowledgeContextRef{
					{Kind: "guardrail", Ref: "team/runtime-output-safety"},
				},
			},
		},
	}
	task := &models.Task{Name: "summarize"}
	snapshots := []models.KnowledgeContextSnapshot{
		{Kind: "guardrail", Ref: "team/runtime-output-safety", Content: "Do not print environment variables."},
	}
	action := &proto.Action{
		Type: "RETURN_ANSWER",
		Payload: &proto.Action_AnswerAction{AnswerAction: &proto.AnswerAction{
			Answer: "The build summary contains one successful test run.",
		}},
	}

	if reason, _, ok := knowledgeContextViolationFailureReason(action, pipeline, step, task, snapshots); ok {
		t.Fatalf("normal answer should not fail, got reason %q", reason)
	}
}

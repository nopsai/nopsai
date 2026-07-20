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
		"NopsAI Knowledge Snapshot",
		"knowledge_revision:",
		"policy_revision:",
		"effective_policy_snapshot_hash:",
		"policy_merge_mode: restrictive",
		"policy_precedence_version:",
		"narrower policies may add restrictions but cannot weaken broader pipeline or step policies",
		"runtime-output-safety",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want it to contain %q", prompt, want)
		}
	}
}

func TestBuildEffectiveKnowledgeContextPromptUsesScopedRestrictivePolicies(t *testing.T) {
	pipeline := &models.Pipeline{
		PolicyMergeMode: models.PolicyMergeModeRestrictive,
		KnowledgeContext: []models.KnowledgeContextRef{
			{Kind: "policy", Ref: "team/deployment-approvals"},
		},
	}
	step := &models.PipelineStep{Step: &models.TaskStep{BaseStep: models.BaseStep{
		KnowledgeContext: []models.KnowledgeContextRef{
			{Kind: "guardrail", Ref: "team/runtime-output"},
		},
	}}}
	task := &models.Task{KnowledgeContext: []models.KnowledgeContextRef{
		{Kind: "policy", Ref: "team/task-deployment-approvals"},
	}}
	snapshots := []models.KnowledgeContextSnapshot{
		{Kind: "policy", Ref: "team/deployment-approvals", Name: "deployment approvals", Content: "Production deployment requires 2 approvals."},
		{Kind: "guardrail", Ref: "team/runtime-output", Name: "runtime output", Content: "Do not print secrets."},
		{Kind: "policy", Ref: "team/task-deployment-approvals", Name: "task deployment approvals", Content: "Production deployment requires 1 approval."},
	}

	prompt := buildEffectiveKnowledgeContextPrompt(pipeline, step, task, snapshots)

	for _, want := range []string{
		"policy_merge_mode: restrictive",
		"Policy precedence is task > step > pipeline",
		"cannot weaken broader pipeline or step policies",
		"scope: pipeline",
		"scope: step",
		"scope: task",
		"Production deployment requires 2 approvals.",
		"Production deployment requires 1 approval.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want it to contain %q", prompt, want)
		}
	}
}

func TestPolicySnapshotHashChangesWhenTaskPolicyStarts(t *testing.T) {
	pipeline := &models.Pipeline{KnowledgeContext: []models.KnowledgeContextRef{{Kind: "policy", Ref: "team/release"}}}
	step := &models.PipelineStep{Step: &models.TaskStep{}}
	task := &models.Task{KnowledgeContext: []models.KnowledgeContextRef{{Kind: "policy", Ref: "team/task-release"}}}
	snapshots := []models.KnowledgeContextSnapshot{
		{Kind: "policy", Ref: "team/release", Content: "Require release evidence."},
		{Kind: "policy", Ref: "team/task-release", Content: "Require task-specific rollback evidence."},
	}

	pipelineOnly := buildEffectiveKnowledgeContextPrompt(pipeline, step, nil, snapshots)
	withTask := buildEffectiveKnowledgeContextPrompt(pipeline, step, task, snapshots)
	pipelineHash := metadataLineForTest(pipelineOnly, "effective_policy_snapshot_hash")
	taskHash := metadataLineForTest(withTask, "effective_policy_snapshot_hash")

	if pipelineHash == "" || taskHash == "" || pipelineHash == taskHash {
		t.Fatalf("effective hashes = pipeline %q task %q, want distinct populated hashes", pipelineHash, taskHash)
	}
}

func TestOverridePolicyMergeUsesNarrowestDuplicateScope(t *testing.T) {
	pipeline := &models.Pipeline{
		PolicyMergeMode: models.PolicyMergeModeOverride,
		KnowledgeContext: []models.KnowledgeContextRef{
			{Kind: "policy", Ref: "team/release"},
		},
	}
	step := &models.PipelineStep{Step: &models.TaskStep{}}
	task := &models.Task{KnowledgeContext: []models.KnowledgeContextRef{
		{Kind: "policy", Ref: "team/release"},
	}}
	snapshots := []models.KnowledgeContextSnapshot{{Kind: "policy", Ref: "team/release", Content: "Require evidence."}}

	prompt := buildEffectiveKnowledgeContextPrompt(pipeline, step, task, snapshots)

	if strings.Contains(prompt, "scope: pipeline") {
		t.Fatalf("override prompt should replace duplicate broader scope:\n%s", prompt)
	}
	if !strings.Contains(prompt, "scope: task") || !strings.Contains(prompt, "policy_merge_mode: override") {
		t.Fatalf("override prompt missing task scoped policy:\n%s", prompt)
	}
}

func TestKnowledgeContextRevisionSeparatesPolicyAndGuidelineChanges(t *testing.T) {
	snapshots := []models.KnowledgeContextSnapshot{
		{Kind: "policy", Ref: "team/release", Content: "Require evidence."},
		{Kind: "guideline", Ref: "team/style", Content: "Friendly tone."},
	}
	knowledgeRevision := knowledgeContextRevision(snapshots, false)
	policyRevision := knowledgeContextRevision(snapshots, true)
	if knowledgeRevision == "" || policyRevision == "" {
		t.Fatalf("revisions should be populated: knowledge=%q policy=%q", knowledgeRevision, policyRevision)
	}
	changedGuideline := []models.KnowledgeContextSnapshot{
		{Kind: "policy", Ref: "team/release", Content: "Require evidence."},
		{Kind: "guideline", Ref: "team/style", Content: "Concise tone."},
	}
	if got := knowledgeContextRevision(changedGuideline, false); got == knowledgeRevision {
		t.Fatal("knowledge revision did not change after guideline content changed")
	}
	if got := knowledgeContextRevision(changedGuideline, true); got != policyRevision {
		t.Fatal("policy revision changed after guideline-only content changed")
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

func metadataLineForTest(prompt, key string) string {
	prefix := key + ":"
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

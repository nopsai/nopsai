package models

import "testing"

func TestNormalizePolicyMergeModeDefaultsRestrictive(t *testing.T) {
	if got := NormalizePolicyMergeMode(""); got != PolicyMergeModeRestrictive {
		t.Fatalf("NormalizePolicyMergeMode(empty) = %q", got)
	}
	if got := NormalizePolicyMergeMode(" FAIL_ON_CONFLICT "); got != PolicyMergeModeFailOnConflict {
		t.Fatalf("NormalizePolicyMergeMode() = %q", got)
	}
	if !SupportedPolicyMergeMode(PolicyMergeModeOverride) || SupportedPolicyMergeMode("loose") {
		t.Fatal("SupportedPolicyMergeMode returned unexpected result")
	}
}

func TestEffectivePolicyMergeModeUsesNarrowestScope(t *testing.T) {
	pipeline := &Pipeline{PolicyMergeMode: PolicyMergeModeRestrictive}
	step := &PipelineStep{Step: &TaskStep{BaseStep: BaseStep{PolicyMergeMode: PolicyMergeModeOverride}}}
	task := &Task{PolicyMergeMode: PolicyMergeModeFailOnConflict}

	if got := EffectivePolicyMergeMode(pipeline, step, task); got != PolicyMergeModeFailOnConflict {
		t.Fatalf("EffectivePolicyMergeMode() = %q", got)
	}
	task.PolicyMergeMode = ""
	if got := EffectivePolicyMergeMode(pipeline, step, task); got != PolicyMergeModeOverride {
		t.Fatalf("EffectivePolicyMergeMode() without task = %q", got)
	}
}

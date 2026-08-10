package models

import "testing"

func TestNormalizeGovernanceLevelDefaultsStrict(t *testing.T) {
	if got := NormalizeGovernanceLevel(""); got != GovernanceLevelStrict {
		t.Fatalf("NormalizeGovernanceLevel(empty) = %q", got)
	}
	if got := NormalizeGovernanceLevel(" EXCEPTION_BASED "); got != GovernanceLevelExceptionBased {
		t.Fatalf("NormalizeGovernanceLevel() = %q", got)
	}
	if got := NormalizeGovernanceLevel(" FAIL_ON_CONFLICT "); got != GovernanceLevelStrict {
		t.Fatalf("legacy NormalizeGovernanceLevel() = %q", got)
	}
	if !SupportedGovernanceLevel(GovernanceLevelGuarded) || SupportedGovernanceLevel("loose") {
		t.Fatal("SupportedGovernanceLevel returned unexpected result")
	}
}

func TestEffectiveGovernanceLevelUsesNarrowestScope(t *testing.T) {
	pipeline := &Pipeline{GovernanceLevel: GovernanceLevelAdvisory}
	step := &PipelineStep{Step: &TaskStep{BaseStep: BaseStep{GovernanceLevel: GovernanceLevelGuarded}}}
	task := &Task{GovernanceLevel: GovernanceLevelExceptionBased}

	if got := EffectiveGovernanceLevel(pipeline, step, task); got != GovernanceLevelExceptionBased {
		t.Fatalf("EffectiveGovernanceLevel() = %q", got)
	}
	task.GovernanceLevel = ""
	if got := EffectiveGovernanceLevel(pipeline, step, task); got != GovernanceLevelGuarded {
		t.Fatalf("EffectiveGovernanceLevel() without task = %q", got)
	}
}

func TestInterpretPolicyReviewByGovernanceLevel(t *testing.T) {
	block := &PolicyReview{Decision: PolicyDecisionBlock, Reason: "violates policy"}
	uncertain := &PolicyReview{Decision: PolicyDecisionUncertain, Reason: "unclear"}
	conflict := &PolicyReview{Decision: PolicyDecisionConflict, Reason: "policies conflict"}

	if got := InterpretPolicyReview(GovernanceLevelAdvisory, block, false); !got.Allowed || !got.Warning {
		t.Fatalf("advisory block interpretation = %#v, want allowed warning", got)
	}
	if got := InterpretPolicyReview(GovernanceLevelGuarded, uncertain, false); !got.Allowed || !got.Warning {
		t.Fatalf("guarded uncertain interpretation = %#v, want allowed warning", got)
	}
	if got := InterpretPolicyReview(GovernanceLevelStrict, uncertain, false); got.Allowed || !got.FailClosed {
		t.Fatalf("strict uncertain interpretation = %#v, want fail closed", got)
	}
	if got := InterpretPolicyReview(GovernanceLevelExceptionBased, conflict, false); got.Allowed || !got.FailClosed {
		t.Fatalf("exception conflict interpretation = %#v, want fail closed", got)
	}
	if got := InterpretPolicyReview(GovernanceLevelExceptionBased, conflict, true); !got.Allowed || !got.Warning {
		t.Fatalf("approved exception interpretation = %#v, want allowed warning", got)
	}
}

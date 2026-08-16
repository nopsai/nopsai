package models

import "testing"

func TestNormalizeGovernanceLevelDefaultsStrict(t *testing.T) {
	if got := NormalizeGovernanceLevel(""); got != GovernanceLevelStrict {
		t.Fatalf("NormalizeGovernanceLevel(empty) = %q", got)
	}
	if got := NormalizeGovernanceLevel(" ADVISORY "); got != GovernanceLevelAdvisory {
		t.Fatalf("NormalizeGovernanceLevel(advisory) = %q", got)
	}
	if !SupportedGovernanceLevel(GovernanceLevelAdvisory) || !SupportedGovernanceLevel(GovernanceLevelStrict) {
		t.Fatal("SupportedGovernanceLevel rejected a supported level")
	}
	if SupportedGovernanceLevel("loose") {
		t.Fatal("SupportedGovernanceLevel accepted an unknown level")
	}
}

// The removed levels and legacy merge modes are not silently accepted. They are
// rejected like any other unknown value so a manifest still carrying one fails
// validation instead of running under a level nobody chose.
func TestGovernanceLevelRejectsRemovedValues(t *testing.T) {
	for _, value := range []string{
		"guarded",
		"exception_based",
		"fail_on_conflict",
		"restrictive",
		"override",
		"loose",
	} {
		if SupportedGovernanceLevel(value) {
			t.Fatalf("SupportedGovernanceLevel(%q) = true, want rejection", value)
		}
	}
}

// A removed value that somehow reaches interpretation without passing
// validation must still enforce, never fall through to advisory.
func TestRemovedGovernanceLevelStillEnforces(t *testing.T) {
	block := &PolicyReview{Decision: PolicyDecisionBlock, Reason: "violates policy"}
	if got := InterpretPolicyReview("guarded", block); got.Allowed || !got.FailClosed {
		t.Fatalf("InterpretPolicyReview(guarded, block) = %#v, want fail closed", got)
	}
}

func TestEffectiveGovernanceLevelUsesNarrowestScope(t *testing.T) {
	pipeline := &Pipeline{GovernanceLevel: GovernanceLevelAdvisory}
	step := &PipelineStep{Step: &TaskStep{BaseStep: BaseStep{GovernanceLevel: GovernanceLevelStrict}}}
	task := &Task{GovernanceLevel: GovernanceLevelAdvisory}

	if got := EffectiveGovernanceLevel(pipeline, step, task); got != GovernanceLevelAdvisory {
		t.Fatalf("EffectiveGovernanceLevel() = %q", got)
	}
	task.GovernanceLevel = ""
	if got := EffectiveGovernanceLevel(pipeline, step, task); got != GovernanceLevelStrict {
		t.Fatalf("EffectiveGovernanceLevel() without task = %q", got)
	}
}

func TestInterpretPolicyReviewByGovernanceLevel(t *testing.T) {
	block := &PolicyReview{Decision: PolicyDecisionBlock, Reason: "violates policy"}
	uncertain := &PolicyReview{Decision: PolicyDecisionUncertain, Reason: "unclear"}
	conflict := &PolicyReview{Decision: PolicyDecisionConflict, Reason: "policies conflict"}
	allow := &PolicyReview{Decision: PolicyDecisionAllow, Reason: "complies"}

	if got := InterpretPolicyReview(GovernanceLevelAdvisory, block); !got.Allowed || !got.Warning {
		t.Fatalf("advisory block interpretation = %#v, want allowed warning", got)
	}
	if got := InterpretPolicyReview(GovernanceLevelAdvisory, allow); !got.Allowed || got.Warning {
		t.Fatalf("advisory allow interpretation = %#v, want allowed without warning", got)
	}
	for name, review := range map[string]*PolicyReview{"block": block, "uncertain": uncertain, "conflict": conflict} {
		if got := InterpretPolicyReview(GovernanceLevelStrict, review); got.Allowed || !got.FailClosed {
			t.Fatalf("strict %s interpretation = %#v, want fail closed", name, got)
		}
	}
	if got := InterpretPolicyReview(GovernanceLevelStrict, allow); !got.Allowed || got.FailClosed {
		t.Fatalf("strict allow interpretation = %#v, want allowed", got)
	}
}

// An unavailable review means nothing was evaluated. Advisory downgrades a
// judgment the model made; it must not authorize skipping the evaluation, or a
// model outage would let scripts run against unchecked guardrails.
func TestUnavailablePolicyReviewFailsClosedAtEveryLevel(t *testing.T) {
	for _, level := range []string{GovernanceLevelAdvisory, GovernanceLevelStrict, "", "loose"} {
		got := InterpretUnavailablePolicyReview(level, "model unreachable")
		if got.Allowed || !got.FailClosed || got.Warning {
			t.Fatalf("InterpretUnavailablePolicyReview(%q) = %#v, want fail closed", level, got)
		}
		if got.Decision != PolicyDecisionUncertain {
			t.Fatalf("InterpretUnavailablePolicyReview(%q) decision = %q", level, got.Decision)
		}
		if got.Reason != "model unreachable" {
			t.Fatalf("InterpretUnavailablePolicyReview(%q) reason = %q", level, got.Reason)
		}
	}
	if got := InterpretUnavailablePolicyReview(GovernanceLevelAdvisory, "  "); got.Reason == "" {
		t.Fatal("InterpretUnavailablePolicyReview() dropped the fallback reason")
	}
}

// A nil review reaching InterpretPolicyReview is the same "nothing was
// evaluated" case, so it must fail closed rather than read as uncertain.
func TestInterpretPolicyReviewNilFailsClosed(t *testing.T) {
	for _, level := range []string{GovernanceLevelAdvisory, GovernanceLevelStrict} {
		if got := InterpretPolicyReview(level, nil); got.Allowed || !got.FailClosed {
			t.Fatalf("InterpretPolicyReview(%q, nil) = %#v, want fail closed", level, got)
		}
	}
}

package models

import "strings"

const (
	// Governance has exactly two levels. Advisory records the AI policy
	// judgment without stopping execution; strict requires a clear allow.
	GovernanceLevelAdvisory = "advisory"
	GovernanceLevelStrict   = "strict"

	PolicyDecisionAllow     = "allow"
	PolicyDecisionBlock     = "block"
	PolicyDecisionConflict  = "conflict"
	PolicyDecisionUncertain = "uncertain"

	GovernanceContractVersion = "2026-08-16.v2"
)

type GovernanceInterpretation struct {
	Allowed    bool
	Warning    bool
	FailClosed bool
	Decision   string
	Reason     string
}

// NormalizeGovernanceLevel resolves a configured level. An empty value defaults
// to strict; anything else is returned as-is so SupportedGovernanceLevel can
// reject it, which keeps an unrecognized level a validation error rather than a
// silent downgrade.
func NormalizeGovernanceLevel(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", GovernanceLevelStrict:
		return GovernanceLevelStrict
	case GovernanceLevelAdvisory:
		return GovernanceLevelAdvisory
	default:
		return normalized
	}
}

// EnforcedGovernanceLevel returns the level the runtime will actually apply.
// Anything that is not advisory enforces as strict, including a value that
// should have failed validation, so a prompt or a log never advertises a level
// that nothing implements.
func EnforcedGovernanceLevel(value string) string {
	if NormalizeGovernanceLevel(value) == GovernanceLevelAdvisory {
		return GovernanceLevelAdvisory
	}
	return GovernanceLevelStrict
}

func SupportedGovernanceLevel(value string) bool {
	switch NormalizeGovernanceLevel(value) {
	case GovernanceLevelAdvisory, GovernanceLevelStrict:
		return true
	default:
		return false
	}
}

func EffectiveGovernanceLevel(pipeline *Pipeline, step *PipelineStep, task *Task) string {
	level := ""
	if pipeline != nil {
		level = strings.TrimSpace(pipeline.GovernanceLevel)
	}
	if step != nil {
		if value := strings.TrimSpace(step.GetGovernanceLevel()); value != "" {
			level = value
		}
	}
	if task != nil {
		if value := strings.TrimSpace(task.GovernanceLevel); value != "" {
			level = value
		}
	}
	return NormalizeGovernanceLevel(level)
}

func NormalizePolicyDecision(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case PolicyDecisionAllow:
		return PolicyDecisionAllow
	case PolicyDecisionBlock:
		return PolicyDecisionBlock
	case PolicyDecisionConflict:
		return PolicyDecisionConflict
	case PolicyDecisionUncertain, "":
		return PolicyDecisionUncertain
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func SupportedPolicyDecision(value string) bool {
	switch NormalizePolicyDecision(value) {
	case PolicyDecisionAllow, PolicyDecisionBlock, PolicyDecisionConflict, PolicyDecisionUncertain:
		return true
	default:
		return false
	}
}

// InterpretUnavailablePolicyReview covers the case where no AI policy judgment
// could be obtained at all: the model is unavailable, the call failed, or the
// validation did not come back in a usable form.
//
// This always fails closed, at every governance level including advisory.
// Advisory downgrades a judgment the model actually made; it does not authorize
// skipping the evaluation. When blocking guardrails or policies are attached to
// a task and no review can be produced, there is no judgment to downgrade, so
// allowing the action would execute it against unchecked constraints.
func InterpretUnavailablePolicyReview(governanceLevel, reason string) GovernanceInterpretation {
	if strings.TrimSpace(reason) == "" {
		reason = "AI policy review was unavailable, so guardrails and policies could not be evaluated."
	}
	return GovernanceInterpretation{
		Allowed:    false,
		Warning:    false,
		FailClosed: true,
		Decision:   PolicyDecisionUncertain,
		Reason:     reason,
	}
}

func InterpretPolicyReview(governanceLevel string, review *PolicyReview) GovernanceInterpretation {
	level := NormalizeGovernanceLevel(governanceLevel)
	decision := PolicyDecisionUncertain
	reason := "AI policy review did not return a decision."
	if review != nil {
		decision = NormalizePolicyDecision(review.Decision)
		reason = strings.TrimSpace(review.Reason)
		if reason == "" {
			reason = "AI policy review returned " + decision + "."
		}
	}
	if !SupportedPolicyDecision(decision) {
		decision = PolicyDecisionUncertain
		if reason == "" {
			reason = "AI policy review returned an unsupported decision."
		}
	}

	result := GovernanceInterpretation{
		Allowed:  true,
		Decision: decision,
		Reason:   reason,
	}

	// A nil review means nothing was evaluated, which is never an advisory
	// warning. Callers should use InterpretUnavailablePolicyReview directly;
	// this guard keeps any missed path failing closed rather than open.
	if review == nil {
		return InterpretUnavailablePolicyReview(level, reason)
	}

	if level == GovernanceLevelAdvisory {
		if decision != PolicyDecisionAllow {
			result.Warning = true
		}
		return result
	}

	// Strict: only a clear allow proceeds.
	if decision != PolicyDecisionAllow {
		result.Allowed = false
		result.FailClosed = true
	}
	return result
}


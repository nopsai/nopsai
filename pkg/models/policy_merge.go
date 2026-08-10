package models

import "strings"

const (
	GovernanceLevelAdvisory       = "advisory"
	GovernanceLevelGuarded        = "guarded"
	GovernanceLevelStrict         = "strict"
	GovernanceLevelExceptionBased = "exception_based"

	PolicyDecisionAllow     = "allow"
	PolicyDecisionBlock     = "block"
	PolicyDecisionConflict  = "conflict"
	PolicyDecisionUncertain = "uncertain"

	GovernanceContractVersion = "2026-08-10.v1"

	// Deprecated legacy policy merge constants. Existing manifests are accepted
	// as aliases, but prompts and telemetry use governance_level.
	PolicyMergeModeRestrictive    = "restrictive"
	PolicyMergeModeOverride       = "override"
	PolicyMergeModeFailOnConflict = "fail_on_conflict"
	PolicyPrecedenceVersion       = GovernanceContractVersion
)

type GovernanceInterpretation struct {
	Allowed    bool
	Warning    bool
	FailClosed bool
	Decision   string
	Reason     string
}

func NormalizeGovernanceLevel(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", GovernanceLevelStrict:
		return GovernanceLevelStrict
	case GovernanceLevelAdvisory:
		return GovernanceLevelAdvisory
	case GovernanceLevelGuarded:
		return GovernanceLevelGuarded
	case GovernanceLevelExceptionBased:
		return GovernanceLevelExceptionBased
	case PolicyMergeModeRestrictive, PolicyMergeModeFailOnConflict:
		return GovernanceLevelStrict
	case PolicyMergeModeOverride:
		return GovernanceLevelGuarded
	default:
		return normalized
	}
}

func SupportedGovernanceLevel(value string) bool {
	switch NormalizeGovernanceLevel(value) {
	case GovernanceLevelAdvisory, GovernanceLevelGuarded, GovernanceLevelStrict, GovernanceLevelExceptionBased:
		return true
	default:
		return false
	}
}

func EffectiveGovernanceLevel(pipeline *Pipeline, step *PipelineStep, task *Task) string {
	level := ""
	if pipeline != nil {
		level = firstNonEmptyPolicyValue(pipeline.GovernanceLevel, pipeline.PolicyMergeMode)
	}
	if step != nil {
		if value := firstNonEmptyPolicyValue(step.GetGovernanceLevel(), step.GetPolicyMergeMode()); value != "" {
			level = value
		}
	}
	if task != nil {
		if value := firstNonEmptyPolicyValue(task.GovernanceLevel, task.PolicyMergeMode); value != "" {
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

func InterpretPolicyReview(governanceLevel string, review *PolicyReview, approvedException bool) GovernanceInterpretation {
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

	switch level {
	case GovernanceLevelAdvisory:
		if decision != PolicyDecisionAllow {
			result.Warning = true
		}
	case GovernanceLevelGuarded:
		if decision == PolicyDecisionBlock || decision == PolicyDecisionConflict {
			result.Allowed = false
			result.FailClosed = true
		} else if decision == PolicyDecisionUncertain {
			result.Warning = true
		}
	case GovernanceLevelExceptionBased:
		switch decision {
		case PolicyDecisionAllow:
			// Continue.
		case PolicyDecisionConflict:
			if approvedException {
				result.Warning = true
			} else {
				result.Allowed = false
				result.FailClosed = true
			}
		default:
			result.Allowed = false
			result.FailClosed = true
		}
	default:
		if decision != PolicyDecisionAllow {
			result.Allowed = false
			result.FailClosed = true
		}
	}

	return result
}

func firstNonEmptyPolicyValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// Deprecated: use NormalizeGovernanceLevel.
func NormalizePolicyMergeMode(value string) string {
	return NormalizeGovernanceLevel(value)
}

// Deprecated: use SupportedGovernanceLevel.
func SupportedPolicyMergeMode(value string) bool {
	return SupportedGovernanceLevel(value)
}

// Deprecated: use EffectiveGovernanceLevel.
func EffectivePolicyMergeMode(pipeline *Pipeline, step *PipelineStep, task *Task) string {
	return EffectiveGovernanceLevel(pipeline, step, task)
}

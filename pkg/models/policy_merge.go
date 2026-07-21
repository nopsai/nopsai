package models

import "strings"

const (
	PolicyMergeModeRestrictive    = "restrictive"
	PolicyMergeModeOverride       = "override"
	PolicyMergeModeFailOnConflict = "fail_on_conflict"

	PolicyPrecedenceVersion = "2026-07-20.v1"
)

func NormalizePolicyMergeMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", PolicyMergeModeRestrictive:
		return PolicyMergeModeRestrictive
	case PolicyMergeModeOverride:
		return PolicyMergeModeOverride
	case PolicyMergeModeFailOnConflict:
		return PolicyMergeModeFailOnConflict
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func SupportedPolicyMergeMode(value string) bool {
	switch NormalizePolicyMergeMode(value) {
	case PolicyMergeModeRestrictive, PolicyMergeModeOverride, PolicyMergeModeFailOnConflict:
		return true
	default:
		return false
	}
}

func EffectivePolicyMergeMode(pipeline *Pipeline, step *PipelineStep, task *Task) string {
	mode := ""
	if pipeline != nil {
		mode = strings.TrimSpace(pipeline.PolicyMergeMode)
	}
	if step != nil && strings.TrimSpace(step.GetPolicyMergeMode()) != "" {
		mode = strings.TrimSpace(step.GetPolicyMergeMode())
	}
	if task != nil && strings.TrimSpace(task.PolicyMergeMode) != "" {
		mode = strings.TrimSpace(task.PolicyMergeMode)
	}
	return NormalizePolicyMergeMode(mode)
}

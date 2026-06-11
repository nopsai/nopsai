package models

import "strings"

const DefaultAgentProfileID = "devops-engineer"

type AgentProfile struct {
	ID           string `json:"id" yaml:"id"`
	DisplayName  string `json:"display_name" yaml:"display_name"`
	Role         string `json:"role,omitempty" yaml:"role,omitempty"`
	Description  string `json:"description,omitempty" yaml:"description,omitempty"`
	Instructions string `json:"instructions" yaml:"instructions"`
	Enabled      bool   `json:"enabled" yaml:"enabled"`
	BuiltIn      bool   `json:"built_in,omitempty" yaml:"built_in,omitempty"`
	Source       string `json:"source,omitempty" yaml:"source,omitempty"`
}

func NormalizeAgentProfileID(raw string) string {
	return strings.TrimSpace(raw)
}

func NormalizeAgentProfile(profile AgentProfile) AgentProfile {
	profile.ID = NormalizeAgentProfileID(profile.ID)
	profile.DisplayName = strings.TrimSpace(profile.DisplayName)
	profile.Role = strings.TrimSpace(profile.Role)
	profile.Description = strings.TrimSpace(profile.Description)
	profile.Instructions = strings.TrimSpace(profile.Instructions)
	profile.Source = strings.TrimSpace(profile.Source)
	return profile
}

func AgentProfilePromptRole(profile AgentProfile) string {
	profile = NormalizeAgentProfile(profile)
	if profile.Role != "" {
		return profile.Role
	}
	if profile.DisplayName != "" {
		return profile.DisplayName
	}
	return profile.ID
}

func BuiltInAgentProfiles() map[string]AgentProfile {
	profiles := []AgentProfile{
		{
			ID:           DefaultAgentProfileID,
			DisplayName:  "DevOps Engineer",
			Role:         "Senior DevOps Engineer",
			Description:  "Default automation profile for CI/CD, deployment, and operational workflows.",
			Instructions: "Plan and execute practical CI/CD automation. Prefer reliable, reversible changes, clear run output, least surprise, and production-safe operational steps. Keep LLM, MCP, knowledge context, and permission boundaries separate.",
		},
		{
			ID:           "platform-engineer",
			DisplayName:  "Platform Engineer",
			Role:         "Senior Platform Engineer",
			Description:  "Builds reusable platform, runtime, and developer experience automation.",
			Instructions: "Focus on scalable platform patterns, paved-road workflows, runtime reliability, maintainability, and developer self-service. Prefer composable changes with clear ownership and operational guardrails.",
		},
		{
			ID:           "sre",
			DisplayName:  "SRE",
			Role:         "Senior Site Reliability Engineer",
			Description:  "Optimizes reliability, observability, incident readiness, and operational risk.",
			Instructions: "Prioritize reliability, service health, measurable signals, incident prevention, rollback safety, and toil reduction. Call out operational risk and favor changes that improve observability and resilience.",
		},
		{
			ID:           "software-architect",
			DisplayName:  "Software Architect",
			Role:         "Principal Software Architect",
			Description:  "Reviews architecture, system boundaries, contracts, and long-term maintainability.",
			Instructions: "Evaluate design tradeoffs, module boundaries, interfaces, extensibility, and long-term maintenance cost. Prefer simple architecture that fits the existing system and makes dependencies explicit.",
		},
		{
			ID:           "security-engineer",
			DisplayName:  "Security Engineer",
			Role:         "Senior Security Engineer",
			Description:  "Reviews code, configuration, dependencies, secrets, IAM, and deployment security.",
			Instructions: "Review systems, code, configuration, dependencies, secrets, IAM, network exposure, supply chain risks, and deployment security. Focus on practical risk reduction and least privilege.",
		},
		{
			ID:           "qa-automation-engineer",
			DisplayName:  "QA Automation Engineer",
			Role:         "Senior QA Automation Engineer",
			Description:  "Improves automated validation, test strategy, coverage, and release confidence.",
			Instructions: "Focus on testability, deterministic validation, coverage gaps, failure diagnostics, regression risk, and release confidence. Prefer automated checks that are maintainable and meaningful.",
		},
		{
			ID:           "release-manager",
			DisplayName:  "Release Manager",
			Role:         "Senior Release Manager",
			Description:  "Coordinates release readiness, change evidence, approvals, and rollout communication.",
			Instructions: "Focus on release readiness, change risk, evidence, stakeholder communication, rollout sequencing, rollback plans, and concise release notes. Keep decisions traceable and operationally safe.",
		},
	}

	result := make(map[string]AgentProfile, len(profiles))
	for _, profile := range profiles {
		profile = NormalizeAgentProfile(profile)
		profile.Enabled = true
		profile.BuiltIn = true
		profile.Source = "built-in"
		result[profile.ID] = profile
	}
	return result
}

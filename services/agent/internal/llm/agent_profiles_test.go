package llm

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"
)

func TestAgentProfileRegistryResolvesStepPipelineDefault(t *testing.T) {
	payload := agentRuntimeAgentProfiles{
		DefaultProfile: models.DefaultAgentProfileID,
		Profiles: map[string]agentRuntimeAgentProfile{
			models.DefaultAgentProfileID: {Role: "DevOps Engineer", Instructions: "Run safe automation.", Enabled: true},
			"sre":                        {Role: "SRE", Instructions: "Protect reliability.", Enabled: true},
			"security-engineer":          {Role: "Security Engineer", Instructions: "Reduce risk.", Enabled: true},
		},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(agentProfilesRuntimeEnv, base64.StdEncoding.EncodeToString(payloadBytes))

	registry, err := NewAgentProfileRegistryFromEnv()
	if err != nil {
		t.Fatalf("NewAgentProfileRegistryFromEnv() error = %v", err)
	}

	pipeline := &models.Pipeline{AgentProfile: "sre"}
	step := &models.PipelineStep{Step: &models.GoalStep{
		BaseStep: models.BaseStep{Name: "review", AgentProfile: "security-engineer"},
		Goal:     "Review",
	}}

	if got := registry.ProfileNameFor(pipeline, step); got != "security-engineer" {
		t.Fatalf("step profile = %q, want security-engineer", got)
	}
	if got := registry.ProfileNameFor(pipeline, nil); got != "sre" {
		t.Fatalf("pipeline profile = %q, want sre", got)
	}
	if got := registry.ProfileNameFor(&models.Pipeline{}, nil); got != models.DefaultAgentProfileID {
		t.Fatalf("default profile = %q, want %q", got, models.DefaultAgentProfileID)
	}

	promptProfile, profileName, err := registry.ProfileFor(pipeline, step)
	if err != nil {
		t.Fatalf("ProfileFor() error = %v", err)
	}
	if profileName != "security-engineer" || promptProfile.Role != "Security Engineer" {
		t.Fatalf("ProfileFor() = %#v/%q, want security-engineer", promptProfile, profileName)
	}
}

func TestAgentProfileRegistryRejectsDisabledProfile(t *testing.T) {
	registry, err := newAgentProfileRegistry(models.DefaultAgentProfileID, map[string]agentRuntimeAgentProfile{
		models.DefaultAgentProfileID: {Role: "DevOps Engineer", Instructions: "Run safe automation.", Enabled: true},
		"disabled":                   {Role: "Disabled", Instructions: "Do not use.", Enabled: false},
	})
	if err != nil {
		t.Fatalf("newAgentProfileRegistry() error = %v", err)
	}

	_, _, err = registry.ProfileFor(&models.Pipeline{AgentProfile: "disabled"}, nil)
	if err == nil {
		t.Fatal("expected disabled profile error")
	}
	if !strings.Contains(err.Error(), `agent profile "disabled" is disabled`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgentPromptProfileFallsBackToDisplayNameWhenRoleIsEmpty(t *testing.T) {
	registry, err := newAgentProfileRegistry("release-manager", map[string]agentRuntimeAgentProfile{
		"release-manager": {DisplayName: "Release Manager", Instructions: "Check rollout evidence.", Enabled: true},
	})
	if err != nil {
		t.Fatalf("newAgentProfileRegistry() error = %v", err)
	}
	profile, profileName, err := registry.ProfileFor(&models.Pipeline{}, nil)
	if err != nil {
		t.Fatalf("ProfileFor() error = %v", err)
	}
	if profileName != "release-manager" || profile.Role != "" || profile.DisplayName != "Release Manager" {
		t.Fatalf("ProfileFor() = %#v/%q, want display-name fallback profile", profile, profileName)
	}
	prompt := formatAgentPromptProfile(profile)
	if !strings.HasPrefix(prompt, "You are Release Manager.\n\nCheck rollout evidence.") {
		t.Fatalf("prompt does not use display name fallback:\n%s", prompt)
	}
}

func TestAgentPromptProfileFormatsPromptRoleAndInstructions(t *testing.T) {
	client := NewLLMClient("lmstudio", "", "model-a", "http://127.0.0.1:1234", "off")
	prompt := client.buildPromptWithMCP(&proto.GetActionRequest{Goal: "Review rollout"}, "", "", AgentPromptProfile{
		ID:           "release-manager",
		Role:         "Senior Release Manager",
		Instructions: "Check rollout evidence before acting.",
	})

	if !strings.HasPrefix(prompt, "You are Senior Release Manager.\n\nCheck rollout evidence before acting.") {
		t.Fatalf("prompt does not start with agent profile:\n%s", prompt)
	}
	if strings.Contains(prompt, "expert CI/CD automation bot") {
		t.Fatalf("prompt used fallback persona instead of selected agent profile:\n%s", prompt)
	}
}

func TestConditionPromptUsesAgentProfile(t *testing.T) {
	client := NewLLMClient("lmstudio", "", "model-a", "http://127.0.0.1:1234", "off")
	prompt := client.buildConditionPrompt(&proto.ConditionRequest{Goal: "Is production healthy?"}, AgentPromptProfile{
		ID:           "sre",
		Role:         "Senior Site Reliability Engineer",
		Instructions: "Prioritize reliability signals.",
	})

	if !strings.Contains(prompt, "You are Senior Site Reliability Engineer.") {
		t.Fatalf("condition prompt missing role:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Prioritize reliability signals.") {
		t.Fatalf("condition prompt missing instructions:\n%s", prompt)
	}
}

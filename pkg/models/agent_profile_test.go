package models

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBuiltInAgentProfilesIncludeEnabledDefault(t *testing.T) {
	profiles := BuiltInAgentProfiles()
	defaultProfile, ok := profiles[DefaultAgentProfileID]
	if !ok {
		t.Fatalf("built-in default profile %q is missing", DefaultAgentProfileID)
	}
	if !defaultProfile.Enabled || !defaultProfile.BuiltIn || defaultProfile.Source != "built-in" {
		t.Fatalf("default profile metadata = %#v, want enabled built-in", defaultProfile)
	}
	if defaultProfile.Role == "" || defaultProfile.Instructions == "" {
		t.Fatalf("default profile role/instructions must be populated: %#v", defaultProfile)
	}
}

func TestAgentProfilePromptRoleFallsBackToNameThenID(t *testing.T) {
	if got := AgentProfilePromptRole(AgentProfile{ID: "release", DisplayName: "Release Manager", Role: "Senior Release Manager"}); got != "Senior Release Manager" {
		t.Fatalf("prompt role with explicit role = %q", got)
	}
	if got := AgentProfilePromptRole(AgentProfile{ID: "release", DisplayName: "Release Manager"}); got != "Release Manager" {
		t.Fatalf("prompt role with display name = %q", got)
	}
	if got := AgentProfilePromptRole(AgentProfile{ID: "release"}); got != "release" {
		t.Fatalf("prompt role with id = %q", got)
	}
}

func TestTaskRejectsAgentProfileFromYAML(t *testing.T) {
	const raw = `
name: invalid
goal: Review release
agent_role: sre
`
	var task Task
	err := yaml.Unmarshal([]byte(raw), &task)
	if err == nil {
		t.Fatal("expected task-level agent_role YAML to be rejected")
	}
	if !strings.Contains(err.Error(), `task "invalid" cannot define agent_role`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTaskRejectsAgentProfileFromJSON(t *testing.T) {
	var task Task
	err := json.Unmarshal([]byte(`{"name":"invalid","goal":"Review release","agent_role":"sre"}`), &task)
	if err == nil {
		t.Fatal("expected task-level agent_role JSON to be rejected")
	}
	if !strings.Contains(err.Error(), `task "invalid" cannot define agent_role`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

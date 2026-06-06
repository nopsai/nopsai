package agent

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	appconfig "nopsai/config"
	"nopsai/pkg/models"
)

func TestLLMProfileRegistryResolvesTaskStepPipelineDefault(t *testing.T) {
	payload := agentRuntimeLLMProfiles{
		DefaultProfile: "standard",
		Profiles: map[string]agentRuntimeLLMProfile{
			"standard":  {Provider: appconfig.LLMProviderGemini, Model: "gemini-2.5-pro", APIKey: "key"},
			"pipeline":  {Provider: appconfig.LLMProviderGemini, Model: "gemini-2.5-flash", APIKey: "key"},
			"step":      {Provider: appconfig.LLMProviderLMStudio, BaseURL: "http://lmstudio:1234", Model: "qwen", Reasoning: "high"},
			"task":      {Provider: appconfig.LLMProviderGemini, Model: "gemini-2.5-flash", APIKey: "key"},
			"prod-only": {Provider: appconfig.LLMProviderGemini, Model: "gemini-2.5-pro", APIKey: "key", AllowedScopes: []string{"prod"}},
		},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(llmProfilesRuntimeEnv, base64.StdEncoding.EncodeToString(payloadBytes))

	registry, err := NewLLMProfileRegistryFromEnv("dev")
	if err != nil {
		t.Fatalf("NewLLMProfileRegistryFromEnv() error = %v", err)
	}

	pipeline := &models.Pipeline{LLMProfile: "pipeline"}
	step := &models.PipelineStep{Step: &models.GoalStep{BaseStep: models.BaseStep{Name: "review", LLMProfile: "step"}, Goal: "review"}}
	task := &models.Task{Name: "deep", Goal: "review", LLMProfile: "task"}

	if got := registry.ProfileNameFor(pipeline, step, task); got != "task" {
		t.Fatalf("task profile = %q, want task", got)
	}
	if got := registry.ProfileNameFor(pipeline, step, nil); got != "step" {
		t.Fatalf("step profile = %q, want step", got)
	}
	if got := registry.ProfileNameFor(pipeline, nil, nil); got != "pipeline" {
		t.Fatalf("pipeline profile = %q, want pipeline", got)
	}
	if got := registry.ProfileNameFor(&models.Pipeline{}, nil, nil); got != "standard" {
		t.Fatalf("default profile = %q, want standard", got)
	}

	client, profileName, err := registry.ClientFor(pipeline, step, task)
	if err != nil {
		t.Fatalf("ClientFor() error = %v", err)
	}
	if profileName != "task" {
		t.Fatalf("ClientFor() profile = %q, want task", profileName)
	}
	if client.profile != "task" {
		t.Fatalf("client.profile = %q, want task", client.profile)
	}
}

func TestLLMProfileRegistryRejectsDisallowedScope(t *testing.T) {
	payload := agentRuntimeLLMProfiles{
		DefaultProfile: "standard",
		Profiles: map[string]agentRuntimeLLMProfile{
			"standard": {Provider: appconfig.LLMProviderGemini, Model: "gemini", APIKey: "key"},
			"reasoning": {
				Provider:      appconfig.LLMProviderLMStudio,
				BaseURL:       "http://lmstudio:1234",
				AllowedScopes: []string{"dev", "internal"},
			},
		},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(llmProfilesRuntimeEnv, base64.StdEncoding.EncodeToString(payloadBytes))

	registry, err := NewLLMProfileRegistryFromEnv("prod")
	if err != nil {
		t.Fatalf("NewLLMProfileRegistryFromEnv() error = %v", err)
	}
	pipeline := &models.Pipeline{LLMProfile: "reasoning"}
	_, _, err = registry.ClientFor(pipeline, nil, nil)
	if err == nil {
		t.Fatal("expected scope error")
	}
	if !strings.Contains(err.Error(), `LLM profile "reasoning" is not allowed in scope "prod"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLLMProfileRegistryMapsThinkingToReasoning(t *testing.T) {
	thinking := true
	payload := agentRuntimeLLMProfiles{
		DefaultProfile: "standard",
		Profiles: map[string]agentRuntimeLLMProfile{
			"standard": {
				Provider: appconfig.LLMProviderLMStudio,
				BaseURL:  "http://lmstudio:1234",
				Model:    "qwen",
				Thinking: &thinking,
			},
		},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(llmProfilesRuntimeEnv, base64.StdEncoding.EncodeToString(payloadBytes))

	registry, err := NewLLMProfileRegistryFromEnv("dev")
	if err != nil {
		t.Fatalf("NewLLMProfileRegistryFromEnv() error = %v", err)
	}
	profile, ok := registry.DefaultProfile()
	if !ok {
		t.Fatalf("DefaultProfile() not found")
	}
	if profile.Reasoning != "on" {
		t.Fatalf("profile.Reasoning = %q, want on", profile.Reasoning)
	}
}

package llm

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

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

func TestLLMProfileRegistryRejectsUnsupportedGenerationOptions(t *testing.T) {
	thinking := true
	tests := []struct {
		name    string
		profile agentRuntimeLLMProfile
	}{
		{
			name: "generic thinking outside lm studio",
			profile: agentRuntimeLLMProfile{
				Provider: appconfig.LLMProviderOpenAI,
				Model:    "gpt-test",
				Thinking: &thinking,
			},
		},
		{
			name: "anthropic temperature above provider maximum",
			profile: agentRuntimeLLMProfile{
				Provider:    appconfig.LLMProviderAnthropic,
				Model:       "claude-test",
				Temperature: llmTestFloat64Pointer(1.1),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newLLMProfileRegistry(
				"standard",
				map[string]agentRuntimeLLMProfile{"standard": tt.profile},
				"dev",
			)
			if err == nil {
				t.Fatal("newLLMProfileRegistry() unexpectedly succeeded")
			}
		})
	}
}

func llmTestFloat64Pointer(value float64) *float64 {
	return &value
}

func TestLLMProfileRegistryPassesProviderOptionsToClient(t *testing.T) {
	temperature := 0.4
	payload := agentRuntimeLLMProfiles{
		DefaultProfile: "standard",
		Profiles: map[string]agentRuntimeLLMProfile{
			"standard": {
				Provider:       appconfig.LLMProviderOpenAI,
				Model:          "gpt-test",
				BaseURL:        "https://example.test/v1",
				APIKey:         "secret",
				TimeoutSeconds: 12,
				MaxTokens:      345,
				Temperature:    &temperature,
				Extra:          map[string]string{" project ": " project-1 "},
			},
		},
	}
	registry, err := newLLMProfileRegistry(payload.DefaultProfile, payload.Profiles, "dev")
	if err != nil {
		t.Fatalf("newLLMProfileRegistry() error = %v", err)
	}
	client, _, err := registry.ClientFor(&models.Pipeline{}, nil, nil)
	if err != nil {
		t.Fatalf("ClientFor() error = %v", err)
	}
	if client.httpClient.Timeout != 12*time.Second {
		t.Fatalf("timeout = %s", client.httpClient.Timeout)
	}
	provider, ok := client.providerClient.(*openAICompatibleClient)
	if !ok {
		t.Fatalf("provider client = %T", client.providerClient)
	}
	if provider.maxTokens != 345 || provider.temperature == nil || *provider.temperature != temperature {
		t.Fatalf("provider options = %#v", provider)
	}
	if provider.extra["project"] != "project-1" {
		t.Fatalf("extra = %#v", provider.extra)
	}
}

package nopsai

import (
	"context"
	"testing"

	"nopsai/config"
	"nopsai/pkg/models"
)

func TestValidateLLMProfileDefinitionSupportsFirstWaveProviders(t *testing.T) {
	tests := []struct {
		name    string
		profile config.LLMProfile
		valid   bool
	}{
		{name: "gemini", profile: config.LLMProfile{Provider: "gemini", Model: "gemini-test", CredentialRef: "credential://system/llm/test"}, valid: true},
		{name: "lm studio", profile: config.LLMProfile{Provider: "lmstudio", BaseURL: "http://lmstudio:1234", Reasoning: "high"}, valid: true},
		{name: "openai", profile: config.LLMProfile{Provider: "openai", Model: "gpt-test", CredentialRef: "credential://system/llm/test"}, valid: true},
		{name: "anthropic", profile: config.LLMProfile{Provider: "anthropic", Model: "claude-test", CredentialRef: "credential://system/llm/test"}, valid: true},
		{name: "groq", profile: config.LLMProfile{Provider: "groq", Model: "llama-test", CredentialRef: "credential://system/llm/test"}, valid: true},
		{name: "mistral", profile: config.LLMProfile{Provider: "mistral", Model: "mistral-test", CredentialRef: "credential://system/llm/test"}, valid: true},
		{name: "openrouter", profile: config.LLMProfile{Provider: "openrouter", Model: "openai/test", CredentialRef: "credential://system/llm/test"}, valid: true},
		{name: "ollama", profile: config.LLMProfile{Provider: "ollama", Model: "qwen", BaseURL: "http://ollama:11434/v1"}, valid: true},
		{name: "azure v1", profile: config.LLMProfile{Provider: "azure-openai", Model: "deployment", BaseURL: "https://resource.openai.azure.com/openai/v1", CredentialRef: "credential://system/llm/test"}, valid: true},
		{name: "azure legacy", profile: config.LLMProfile{Provider: "azure-openai", BaseURL: "https://resource.openai.azure.com", CredentialRef: "credential://system/llm/test", Extra: map[string]string{"deployment": "deploy"}}, valid: true},
		{name: "missing cloud key", profile: config.LLMProfile{Provider: "openai", Model: "gpt-test"}, valid: false},
		{name: "missing hosted model", profile: config.LLMProfile{Provider: "anthropic", CredentialRef: "credential://system/llm/test"}, valid: false},
		{name: "missing gemini model", profile: config.LLMProfile{Provider: "gemini", CredentialRef: "credential://system/llm/test"}, valid: false},
		{name: "missing gemini key", profile: config.LLMProfile{Provider: "gemini", Model: "gemini-test"}, valid: false},
		{name: "missing lm studio base", profile: config.LLMProfile{Provider: "lmstudio"}, valid: false},
		{name: "invalid lm studio reasoning", profile: config.LLMProfile{Provider: "lmstudio", BaseURL: "http://lmstudio:1234", Reasoning: "extreme"}, valid: false},
		{name: "lm studio temperature maximum", profile: config.LLMProfile{Provider: "lmstudio", BaseURL: "http://lmstudio:1234", Temperature: float64Ptr(1.1)}, valid: false},
		{name: "anthropic temperature maximum", profile: config.LLMProfile{Provider: "anthropic", Model: "claude-test", CredentialRef: "credential://system/llm/test", Temperature: float64Ptr(1.1)}, valid: false},
		{name: "openai temperature above one", profile: config.LLMProfile{Provider: "openai", Model: "gpt-test", CredentialRef: "credential://system/llm/test", Temperature: float64Ptr(1.1)}, valid: true},
		{name: "generic reasoning outside lm studio", profile: config.LLMProfile{Provider: "openai", Model: "gpt-test", CredentialRef: "credential://system/llm/test", Reasoning: "high"}, valid: false},
		{name: "generic thinking outside lm studio", profile: config.LLMProfile{Provider: "anthropic", Model: "claude-test", CredentialRef: "credential://system/llm/test", Thinking: boolPtr(true)}, valid: false},
		{name: "missing ollama model", profile: config.LLMProfile{Provider: "ollama", BaseURL: "http://ollama:11434/v1"}, valid: false},
		{name: "missing ollama base", profile: config.LLMProfile{Provider: "ollama", Model: "qwen"}, valid: false},
		{name: "missing azure base", profile: config.LLMProfile{Provider: "azure-openai", Model: "gpt-test", CredentialRef: "credential://system/llm/test"}, valid: false},
		{name: "missing azure model", profile: config.LLMProfile{Provider: "azure-openai", BaseURL: "https://resource.openai.azure.com", CredentialRef: "credential://system/llm/test"}, valid: false},
		{name: "missing azure key", profile: config.LLMProfile{Provider: "azure-openai", BaseURL: "https://resource.openai.azure.com", Model: "gpt-test"}, valid: false},
		{name: "invalid temperature", profile: config.LLMProfile{Provider: "openai", Model: "gpt-test", CredentialRef: "credential://system/llm/test", Temperature: float64Ptr(2.5)}, valid: false},
		{name: "negative timeout", profile: config.LLMProfile{Provider: "openai", Model: "gpt-test", CredentialRef: "credential://system/llm/test", TimeoutSeconds: -1}, valid: false},
		{name: "negative max tokens", profile: config.LLMProfile{Provider: "openai", Model: "gpt-test", CredentialRef: "credential://system/llm/test", MaxTokens: -1}, valid: false},
		{name: "unsupported", profile: config.LLMProfile{Provider: "custom"}, valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _ := validateLLMProfileDefinition("test", tt.profile)
			if got := status == "valid"; got != tt.valid {
				t.Fatalf("valid = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestValidateLLMProfileConfigurationResolvesRequiredCredentials(t *testing.T) {
	profile := config.LLMProfile{
		Provider:      config.LLMProviderOpenAI,
		Model:         "gpt-test",
		CredentialRef: "credential://system/llm/hosted",
	}
	app := &App{credentialResolver: staticCredentialResolver{}}
	if status, _ := app.validateLLMProfileConfiguration(context.Background(), "hosted", profile); status != "invalid" {
		t.Fatalf("status = %q, want invalid", status)
	}
	app.credentialResolver = staticCredentialResolver{"credential://system/llm/hosted": "secret"}
	if status, message := app.validateLLMProfileConfiguration(context.Background(), "hosted", profile); status != "valid" {
		t.Fatalf("status = %q, message = %q", status, message)
	}

	ollama := config.LLMProfile{
		Provider: config.LLMProviderOllama,
		Model:    "qwen",
		BaseURL:  "http://ollama:11434/v1",
	}
	if status, message := app.validateLLMProfileConfiguration(context.Background(), "local", ollama); status != "valid" {
		t.Fatalf("status = %q, message = %q", status, message)
	}
}

func TestBuildRuntimeLLMProfilesPreservesProviderOptions(t *testing.T) {
	temperature := 0.2
	app := &App{credentialResolver: staticCredentialResolver{
		"credential://system/llm/openrouter": "secret",
	}}
	runtime, err := app.buildRuntimeLLMProfiles(context.Background(), config.Config{
		LLMDefaultProfile: "standard",
		LLMProfiles: map[string]config.LLMProfile{
			"standard": {
				Provider:       config.LLMProviderOpenRouter,
				Model:          "openai/test",
				CredentialRef:  "credential://system/llm/openrouter",
				TimeoutSeconds: 45,
				MaxTokens:      3000,
				Temperature:    &temperature,
				PromptCache:    config.LLMFeatureConfig{Mode: "required", Scope: "run"},
				ProviderState:  config.LLMFeatureConfig{Mode: "disabled"},
				Extra:          map[string]string{"x_title": "NopsAI"},
			},
		},
	})
	if err != nil {
		t.Fatalf("buildRuntimeLLMProfiles() error = %v", err)
	}
	profile := runtime.Profiles["standard"]
	if profile.BaseURL != "https://openrouter.ai/api/v1" || profile.TimeoutSeconds != 45 || profile.MaxTokens != 3000 {
		t.Fatalf("runtime profile = %#v", profile)
	}
	if profile.Temperature == nil || *profile.Temperature != temperature || profile.Extra["x_title"] != "NopsAI" {
		t.Fatalf("runtime options = %#v", profile)
	}
	if profile.PromptCache.Mode != "required" || profile.PromptCache.Scope != "run" || profile.ProviderState.Mode != "disabled" {
		t.Fatalf("runtime feature preferences = %#v/%#v", profile.PromptCache, profile.ProviderState)
	}
}

func TestBuildRuntimeLLMProfilesForPipelineTeamSkipsUnusedProfileCredentials(t *testing.T) {
	app := &App{credentialResolver: staticCredentialResolver{
		"credential://system/llm/standard":     "standard-secret",
		"credential://system/llm/gemini-flash": "flash-secret",
		"credential://system/llm/report":       "report-secret",
	}}
	cfg := config.Config{
		LLMDefaultProfile: "reasoning",
		LLMProfiles: map[string]config.LLMProfile{
			"reasoning": {
				Provider:      config.LLMProviderOpenAI,
				Model:         "gpt-reasoning",
				CredentialRef: "credential://system/llm/reasoning",
			},
			"gemini-flash": {
				Provider:      config.LLMProviderGemini,
				Model:         "gemini-flash",
				CredentialRef: "credential://system/llm/gemini-flash",
			},
			"report": {
				Provider:      config.LLMProviderOpenAI,
				Model:         "gpt-report",
				CredentialRef: "credential://system/llm/report",
			},
		},
	}
	pipeline := &models.Pipeline{
		Name:       "python-quality",
		LLMProfile: "gemini-flash",
		Steps: []models.PipelineStep{{
			Step: &models.GoalStep{
				BaseStep: models.BaseStep{Name: "quality-evidence-review"},
				Goal:     "Review pytest and SonarQube evidence.",
			},
		}},
		Output: models.PipelineOutput{
			Items: []models.PipelineOutputItem{{
				Name:       "Python quality report",
				Type:       "markdown",
				Prompt:     "Create an engineering quality report.",
				LLMProfile: "report",
			}},
		},
	}

	runtime, err := app.buildRuntimeLLMProfilesForPipelineTeam(context.Background(), cfg, pipeline, nil)
	if err != nil {
		t.Fatalf("buildRuntimeLLMProfilesForPipelineTeam() error = %v", err)
	}
	if runtime.DefaultProfile != "gemini-flash" {
		t.Fatalf("runtime default profile = %q, want gemini-flash", runtime.DefaultProfile)
	}
	if _, ok := runtime.Profiles["reasoning"]; ok {
		t.Fatalf("unused reasoning profile was packaged: %#v", runtime.Profiles)
	}
	if got := runtime.Profiles["gemini-flash"].APIKey; got != "flash-secret" {
		t.Fatalf("gemini-flash api key = %q, want flash-secret", got)
	}
	if got := runtime.Profiles["report"].APIKey; got != "report-secret" {
		t.Fatalf("report api key = %q, want report-secret", got)
	}
}

func TestRequiredLLMProfilesForPipelineCollectsTaskOverrides(t *testing.T) {
	pipeline := &models.Pipeline{
		LLMProfile: "pipeline",
		Steps: []models.PipelineStep{{
			Step: &models.TaskStep{
				BaseStep: models.BaseStep{Name: "deep", LLMProfile: "step"},
				Tasks: []models.Task{
					{Name: "script", Script: "echo ok", LLMProfile: "unused-task"},
					{Name: "review", Goal: "Review", LLMProfile: "task"},
					{Name: "summarize", Goal: "Summarize"},
				},
			},
		}},
		Output: models.PipelineOutput{
			LLMProfile: "output",
			Items: []models.PipelineOutputItem{
				{Name: "summary", Type: "markdown", Prompt: "Summarize"},
				{Name: "dashboard", Type: "dashboard", Prompt: "Publish", LLMProfile: "dashboard"},
			},
		},
	}

	defaultProfile, required := requiredLLMProfilesForPipeline(pipeline, "reasoning")
	if defaultProfile != "pipeline" {
		t.Fatalf("runtime default = %q, want pipeline", defaultProfile)
	}
	for _, name := range []string{"pipeline", "step", "task", "output", "dashboard"} {
		if !required[name] {
			t.Fatalf("required profiles missing %q: %#v", name, required)
		}
	}
	for _, name := range []string{"reasoning", "unused-task"} {
		if required[name] {
			t.Fatalf("unexpected required profile %q: %#v", name, required)
		}
	}
}

func float64Ptr(value float64) *float64 {
	return &value
}

func TestContainerReachableLMStudioBaseURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "localhost rewritten", raw: "http://127.0.0.1:1234", want: "http://host.docker.internal:1234"},
		{name: "localhost hostname rewritten", raw: "http://localhost:1234/v1", want: "http://host.docker.internal:1234/v1"},
		{name: "remote host preserved", raw: "http://lmstudio.internal:1234", want: "http://lmstudio.internal:1234"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containerReachableLMStudioBaseURL(tt.raw); got != tt.want {
				t.Fatalf("containerReachableLMStudioBaseURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

package nopsai

import (
	"context"
	"strings"
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

func TestParseGitOpsLLMProfilePlanFromSettingDirectory(t *testing.T) {
	plan, err := parseGitOpsLLMProfilePlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		gitOpsLLMProfileDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/llm_profile.yaml": `
default_profile: reasoning
profiles:
  - name: fast
    provider: gemini
    model: gemini-2.5-flash
    credential_ref: credential://system/llm/gemini-fast
    allowed_scopes: ["dev"]
  - name: reasoning
    provider: lmstudio
    model: google/gemma-4-26b-a4b
    base_url: http://lmstudio:1234
    reasoning: high
  - name: hosted
    provider: openrouter
    model: openai/gpt-test
    credential_ref: credential://system/llm/openrouter-hosted
    timeout_seconds: 45
    max_tokens: 3000
    temperature: 0.2
    extra:
      x_title: NopsAI
`,
			},
		},
	)
	if err != nil {
		t.Fatalf("parseGitOpsLLMProfilePlan() error = %v", err)
	}
	if plan == nil {
		t.Fatal("expected GitOps LLM profile plan")
	}
	if plan.defaultProfile != "reasoning" {
		t.Fatalf("defaultProfile = %q, want reasoning", plan.defaultProfile)
	}
	if plan.sourcePath != "setting/system/llm_profile.yaml" {
		t.Fatalf("sourcePath = %q", plan.sourcePath)
	}
	if got := plan.profiles["fast"].Provider; got != config.LLMProviderGemini {
		t.Fatalf("fast provider = %q, want gemini", got)
	}
	if got := plan.profiles["reasoning"].Reasoning; got != "high" {
		t.Fatalf("reasoning profile reasoning = %q, want high", got)
	}
	hosted := plan.profiles["hosted"]
	if hosted.TimeoutSeconds != 45 || hosted.MaxTokens != 3000 || hosted.Temperature == nil || *hosted.Temperature != 0.2 {
		t.Fatalf("hosted limits = %#v", hosted)
	}
	if hosted.Extra["x_title"] != "NopsAI" {
		t.Fatalf("hosted extra = %#v", hosted.Extra)
	}
}

func TestParseGitOpsLLMProfilePlanRejectsGroupScopedRepo(t *testing.T) {
	_, err := parseGitOpsLLMProfilePlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"},
		gitOpsLLMProfileDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/llm_profile.yaml": `
llm_default_profile: standard
llm_profiles:
  standard:
    provider: lmstudio
    base_url: http://lmstudio:1234
`,
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "system config repository") {
		t.Fatalf("expected system-scope error, got %v", err)
	}
}

func TestParseGitOpsLLMProfilePlanRejectsMissingDefault(t *testing.T) {
	_, err := parseGitOpsLLMProfileFile(`
default_profile: reasoning
llm_profiles:
  fast:
    provider: gemini
    model: gemini-2.5-flash
    api_key_secret: GEMINI_API_KEY
`, "setting/system/llm_profile.yaml")
	if err == nil || !strings.Contains(err.Error(), `default profile "reasoning"`) {
		t.Fatalf("expected missing default error, got %v", err)
	}
}

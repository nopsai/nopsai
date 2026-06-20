package nopsai

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"nopsai/config"
)

func TestBuildAssistantLLMProfilesResponseFiltersForPickerContext(t *testing.T) {
	response := buildAssistantLLMProfilesResponse("standard", map[string]config.LLMProfile{
		"prod": {
			Provider:      config.LLMProviderOpenAI,
			Model:         "gpt-test",
			CredentialRef: "credential://system/llm/prod",
			AllowedScopes: []string{"platform/prod"},
		},
		"standard": {
			Provider: config.LLMProviderLMStudio,
			BaseURL:  "http://lmstudio:1234",
			Model:    "qwen",
		},
		"broken": {
			Provider: config.LLMProviderOpenAI,
			Model:    "gpt-test",
		},
	}, "platform/dev")

	if response.DefaultProfile != "standard" {
		t.Fatalf("default profile = %q, want standard", response.DefaultProfile)
	}
	if got := profileOptionNames(response.Profiles); strings.Join(got, ",") != "broken,prod,standard" {
		t.Fatalf("profile names = %#v", got)
	}

	broken := assistantProfileOptionByName(response.Profiles, "broken")
	if broken.Status != "invalid" || broken.DisabledReason == "" {
		t.Fatalf("broken profile = %#v, want invalid with disabled reason", broken)
	}

	prod := assistantProfileOptionByName(response.Profiles, "prod")
	if prod.Status != "valid" || prod.AllowedInScope || prod.Provider != config.LLMProviderOpenAI || prod.Model != "gpt-test" {
		t.Fatalf("prod profile = %#v, want valid but blocked in platform/dev", prod)
	}
	if !strings.Contains(prod.DisabledReason, "platform/dev") {
		t.Fatalf("prod disabled reason = %q, want scope", prod.DisabledReason)
	}

	standard := assistantProfileOptionByName(response.Profiles, "standard")
	if standard.Status != "valid" || !standard.AllowedInScope || standard.Provider != config.LLMProviderLMStudio {
		t.Fatalf("standard profile = %#v, want selectable local profile", standard)
	}

	unscoped := buildAssistantLLMProfilesResponse("standard", map[string]config.LLMProfile{
		"prod": {
			Provider:      config.LLMProviderOpenAI,
			Model:         "gpt-test",
			CredentialRef: "credential://system/llm/prod",
			AllowedScopes: []string{"platform/prod"},
		},
	}, "")
	if prod := assistantProfileOptionByName(unscoped.Profiles, "prod"); !prod.AllowedInScope || prod.DisabledReason != "" {
		t.Fatalf("unscoped prod profile = %#v, want selectable until a conversation scope is known", prod)
	}
}

func TestAssistantConfigResponseRedactsCredentialFields(t *testing.T) {
	response := buildAssistantConfigResponse(config.AssistantConfig{
		Enabled:            true,
		Provider:           "openai",
		Model:              "gpt-test",
		BaseURL:            "https://proxy.example/v1",
		CredentialRef:      "credential://system/assistant/api-key",
		LegacyAPIKeySecret: "NOPSAI_ASSISTANT_API_KEY",
	})

	if !response.Enabled || response.Provider != config.LLMProviderOpenAI || response.Model != "gpt-test" {
		t.Fatalf("response model fields = %#v", response)
	}
	if !response.CredentialConfigured {
		t.Fatalf("credential_configured = false, want true")
	}
	if response.DedicatedProfile != assistantDedicatedLLMProfileName {
		t.Fatalf("dedicated profile = %q", response.DedicatedProfile)
	}
}

func TestAssistantDedicatedConfigProfileWinsDefaultPickerProfile(t *testing.T) {
	defaultProfile, profiles := assistantLLMProfilesWithDedicatedConfig("standard", map[string]config.LLMProfile{
		"standard": {
			Provider: config.LLMProviderLMStudio,
			BaseURL:  "http://lmstudio:1234",
			Model:    "qwen",
		},
	}, config.AssistantConfig{
		Provider:      "gemini",
		Model:         "gemini-2.5-pro",
		CredentialRef: "credential://system/assistant/api-key",
		Timeout:       "45s",
	})

	if defaultProfile != assistantDedicatedLLMProfileName {
		t.Fatalf("default profile = %q, want dedicated assistant profile", defaultProfile)
	}
	profile := profiles[assistantDedicatedLLMProfileName]
	if profile.Provider != config.LLMProviderGemini || profile.Model != "gemini-2.5-pro" || profile.TimeoutSeconds != 45 {
		t.Fatalf("assistant profile = %#v", profile)
	}
}

func TestAssistantDefaultLLMProfileUsesConfiguredDefault(t *testing.T) {
	app := &App{cfg: &config.Config{
		LLMDefaultProfile: "standard",
		LLMProfiles: map[string]config.LLMProfile{
			"standard": {
				Provider: config.LLMProviderLMStudio,
				BaseURL:  "http://lmstudio:1234",
				Model:    "qwen",
			},
		},
	}}

	if got := app.assistantDefaultLLMProfile(context.Background()); got != "standard" {
		t.Fatalf("assistantDefaultLLMProfile() = %q, want standard", got)
	}
}

func TestAssistantHTTPClientDoesNotInheritInternalTimeout(t *testing.T) {
	transport := http.DefaultTransport
	internal := &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}

	client := assistantHTTPClient(&App{httpClient: internal})
	if client == internal {
		t.Fatal("assistantHTTPClient returned the shared internal client")
	}
	if client.Timeout != 0 {
		t.Fatalf("timeout = %s, want no inherited timeout", client.Timeout)
	}
	if client.Transport != transport {
		t.Fatal("assistantHTTPClient did not preserve the internal transport")
	}
}

func TestResolveAssistantLLMProfileAllowsScopedProfileWhenConversationScopeUnknown(t *testing.T) {
	app := &App{cfg: &config.Config{
		LLMDefaultProfile: "standard",
		LLMProfiles: map[string]config.LLMProfile{
			"standard": {
				Provider:      config.LLMProviderGemini,
				Model:         "gemini-2.5-flash",
				CredentialRef: "credential://system/llm/standard",
				AllowedScopes: []string{"prod"},
			},
		},
	}}

	name, _, ok, reason := app.resolveAssistantLLMProfile(context.Background(), assistantConversation{}, "standard")
	if !ok {
		t.Fatalf("resolveAssistantLLMProfile(%q) failed: %s", name, reason)
	}
}

func TestResolveAssistantLLMProfileRejectsScopedProfileWhenConversationScopeDiffers(t *testing.T) {
	app := &App{cfg: &config.Config{
		LLMDefaultProfile: "standard",
		LLMProfiles: map[string]config.LLMProfile{
			"standard": {
				Provider:      config.LLMProviderGemini,
				Model:         "gemini-2.5-flash",
				CredentialRef: "credential://system/llm/standard",
				AllowedScopes: []string{"prod"},
			},
		},
	}}

	_, _, ok, reason := app.resolveAssistantLLMProfile(context.Background(), assistantConversation{
		Memory: assistantConversationMemory{SelectedScope: "dev"},
	}, "standard")
	if ok || !strings.Contains(reason, `LLM profile "standard" is not allowed in scope "dev"`) {
		t.Fatalf("resolveAssistantLLMProfile() ok=%v reason=%q", ok, reason)
	}
}

func TestAssistantLLMProfilesRouteIsAuthenticatedOnly(t *testing.T) {
	if !isAuthenticatedOnlyPath("/v1/assistant/config") {
		t.Fatal("assistant config route must be authenticated-only")
	}
	if !isAuthenticatedOnlyPath("/v1/assistant/llm-profiles") {
		t.Fatal("assistant LLM profile picker route must be authenticated-only")
	}
	if !isAuthenticatedOnlyPath("/v1/assistant/conversations/00000000-0000-0000-0000-000000000001") {
		t.Fatal("assistant conversation delete/detail routes must be authenticated-only")
	}
	if isAuthenticatedOnlyPath("/v1/system/llm-profiles") {
		t.Fatal("system LLM profile management route must still require system authorization")
	}
}

func profileOptionNames(options []assistantLLMProfileOption) []string {
	names := make([]string, 0, len(options))
	for _, option := range options {
		names = append(names, option.Name)
	}
	return names
}

func assistantProfileOptionByName(options []assistantLLMProfileOption, name string) assistantLLMProfileOption {
	for _, option := range options {
		if option.Name == name {
			return option
		}
	}
	return assistantLLMProfileOption{}
}

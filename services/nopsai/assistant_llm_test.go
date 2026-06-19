package nopsai

import (
	"strings"
	"testing"

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

func TestAssistantLLMProfilesRouteIsAuthenticatedOnly(t *testing.T) {
	if !isAuthenticatedOnlyPath("/v1/assistant/llm-profiles") {
		t.Fatal("assistant LLM profile picker route must be authenticated-only")
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

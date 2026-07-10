package nopsai

import (
	"testing"

	"nopsai/config"
)

func TestParseTeamLLMProfilesPayloadAcceptsTeamDefault(t *testing.T) {
	payload := llmProfilesRequest{
		DefaultProfile: "standard",
		Profiles: []llmProfileForm{{
			Name:     "standard",
			Provider: config.LLMProviderLMStudio,
			BaseURL:  "http://lmstudio:1234",
		}},
	}

	defaultProfile, profiles, err := parseTeamLLMProfilesPayload(payload)
	if err != nil {
		t.Fatalf("parseTeamLLMProfilesPayload() error = %v", err)
	}
	if defaultProfile != "standard" {
		t.Fatalf("default profile = %q, want standard", defaultProfile)
	}
	if _, ok := profiles["standard"]; !ok {
		t.Fatalf("profiles missing standard: %#v", profiles)
	}
}

func TestParseTeamLLMProfilesPayloadRejectsUnknownDefault(t *testing.T) {
	payload := llmProfilesRequest{
		DefaultProfile: "standard",
		Profiles: []llmProfileForm{{
			Name:     "sandbox",
			Provider: config.LLMProviderLMStudio,
			BaseURL:  "http://lmstudio:1234",
		}},
	}

	_, _, err := parseTeamLLMProfilesPayload(payload)
	if err == nil {
		t.Fatal("parseTeamLLMProfilesPayload() error = nil, want error")
	}
}

func TestParseTeamLLMProfilesPayloadRejectsDuplicateProfiles(t *testing.T) {
	payload := llmProfilesRequest{
		LLMProfiles: map[string]config.LLMProfile{
			"standard": {
				Provider: config.LLMProviderLMStudio,
				BaseURL:  "http://lmstudio:1234",
			},
		},
		Profiles: []llmProfileForm{{
			Name:     "standard",
			Provider: config.LLMProviderLMStudio,
			BaseURL:  "http://lmstudio:1234",
		}},
	}

	_, _, err := parseTeamLLMProfilesPayload(payload)
	if err == nil {
		t.Fatal("parseTeamLLMProfilesPayload() error = nil, want duplicate error")
	}
}

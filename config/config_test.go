package config

import "testing"

func TestNormalizeLLMProvider(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "default to gemini", raw: "", want: LLMProviderGemini},
		{name: "gemini alias", raw: "google-gemini", want: LLMProviderGemini},
		{name: "lmstudio canonical", raw: "lmstudio", want: LLMProviderLMStudio},
		{name: "openai compatible alias", raw: "openai-compatible", want: LLMProviderLMStudio},
		{name: "unknown passes through normalized", raw: "CustomProvider", want: "customprovider"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeLLMProvider(tt.raw); got != tt.want {
				t.Fatalf("NormalizeLLMProvider(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestNormalizeLMStudioReasoning(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty preserved", raw: "", want: ""},
		{name: "off preserved", raw: "off", want: "off"},
		{name: "bool false alias", raw: "false", want: "off"},
		{name: "bool true alias", raw: "true", want: "on"},
		{name: "mixed case normalized", raw: "Medium", want: "medium"},
		{name: "unknown passes through normalized", raw: "Custom", want: "custom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeLMStudioReasoning(tt.raw); got != tt.want {
				t.Fatalf("NormalizeLMStudioReasoning(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

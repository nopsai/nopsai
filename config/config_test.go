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

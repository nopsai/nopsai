package main

import (
	"strings"
	"testing"

	appconfig "nopsai/config"
	"nopsai/pkg/proto"
)

func TestBuildLMStudioChatURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "bare host", baseURL: "http://127.0.0.1:1234", want: "http://127.0.0.1:1234/api/v1/chat"},
		{name: "api v1 base", baseURL: "http://127.0.0.1:1234/api/v1", want: "http://127.0.0.1:1234/api/v1/chat"},
		{name: "full endpoint", baseURL: "http://127.0.0.1:1234/api/v1/chat", want: "http://127.0.0.1:1234/api/v1/chat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildLMStudioChatURL(tt.baseURL); got != tt.want {
				t.Fatalf("buildLMStudioChatURL(%q) = %q, want %q", tt.baseURL, got, tt.want)
			}
		})
	}
}

func TestBuildLMStudioModelsURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "bare host", baseURL: "http://127.0.0.1:1234", want: "http://127.0.0.1:1234/api/v1/models"},
		{name: "api v1 base", baseURL: "http://127.0.0.1:1234/api/v1", want: "http://127.0.0.1:1234/api/v1/models"},
		{name: "chat endpoint", baseURL: "http://127.0.0.1:1234/api/v1/chat", want: "http://127.0.0.1:1234/api/v1/models"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildLMStudioModelsURL(tt.baseURL); got != tt.want {
				t.Fatalf("buildLMStudioModelsURL(%q) = %q, want %q", tt.baseURL, got, tt.want)
			}
		})
	}
}

func TestCleanModelTextResponse(t *testing.T) {
	raw := "```json\n{\"action\":{\"type\":\"RETURN_ANSWER\"}}\n```"
	want := "{\"action\":{\"type\":\"RETURN_ANSWER\"}}"

	if got := cleanModelTextResponse(raw); got != want {
		t.Fatalf("cleanModelTextResponse() = %q, want %q", got, want)
	}
}

func TestBuildPromptLMStudioKeepsFullContext(t *testing.T) {
	client := NewLLMClient(appconfig.LLMProviderLMStudio, "", "qwen/qwen3.6-35b-a3b", "http://127.0.0.1:1234", "off")
	req := &proto.GetActionRequest{
		Goal:    "wait for 2 seconds",
		History: strings.Repeat("history-line\n", 400),
		Variables: map[string]string{
			"SHORT": "ok",
			"LONG":  strings.Repeat("very-long-variable-value-", 40),
		},
		DirectoryListing: map[string]string{
			"pipeline.yaml": strings.Repeat("step: build-and-test\n", 300),
			"README.md":     strings.Repeat("documentation\n", 300),
			"script.sh":     strings.Repeat("echo hello\n", 300),
		},
	}

	prompt := client.buildPrompt(req)

	if strings.Contains(prompt, "[directory listing truncated") || strings.Contains(prompt, "...[truncated]...") {
		t.Fatalf("expected LM Studio prompt to keep full context, got: %s", prompt)
	}
	for _, expected := range []string{
		strings.Repeat("history-line\n", 400),
		strings.Repeat("very-long-variable-value-", 40),
		"--- File: README.md ---",
		"--- File: pipeline.yaml ---",
		"--- File: script.sh ---",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected prompt to contain full LM Studio context fragment %q", expected)
		}
	}
}

func TestBuildPromptGeminiKeepsLargeContext(t *testing.T) {
	client := NewLLMClient(appconfig.LLMProviderGemini, "", "gemini-2.5-flash", "", "")
	req := &proto.GetActionRequest{
		Goal:    "show file",
		History: strings.Repeat("history-line\n", 50),
		DirectoryListing: map[string]string{
			"pipeline.yaml": strings.Repeat("step: build-and-test\n", 120),
		},
	}

	prompt := client.buildPrompt(req)

	if strings.Contains(prompt, "...[truncated]...") {
		t.Fatalf("expected Gemini prompt to remain untruncated")
	}
}

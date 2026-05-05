package main

import (
	"strings"
	"testing"

	appconfig "nopsai/config"
	"nopsai/pkg/proto"
)

func TestBuildLMStudioChatCompletionsURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "bare host", baseURL: "http://127.0.0.1:1234", want: "http://127.0.0.1:1234/v1/chat/completions"},
		{name: "v1 base", baseURL: "http://127.0.0.1:1234/v1", want: "http://127.0.0.1:1234/v1/chat/completions"},
		{name: "full endpoint", baseURL: "http://127.0.0.1:1234/v1/chat/completions", want: "http://127.0.0.1:1234/v1/chat/completions"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildLMStudioChatCompletionsURL(tt.baseURL); got != tt.want {
				t.Fatalf("buildLMStudioChatCompletionsURL(%q) = %q, want %q", tt.baseURL, got, tt.want)
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

func TestPrepareLMStudioPromptAddsNoThinkForQwen(t *testing.T) {
	client := NewLLMClient(appconfig.LLMProviderLMStudio, "", "qwen/qwen3.6-35b-a3b", "http://127.0.0.1:1234", false)

	got := client.prepareLMStudioPrompt("Reply with JSON only.")

	if got != "Reply with JSON only.\n/no_think" {
		t.Fatalf("prepareLMStudioPrompt() = %q", got)
	}
}

func TestPrepareLMStudioPromptDoesNotDuplicateNoThink(t *testing.T) {
	client := NewLLMClient(appconfig.LLMProviderLMStudio, "", "qwen/qwen3.6-35b-a3b", "http://127.0.0.1:1234", false)

	got := client.prepareLMStudioPrompt("Reply with JSON only.\n/no_think")

	if got != "Reply with JSON only.\n/no_think" {
		t.Fatalf("prepareLMStudioPrompt() duplicated suffix: %q", got)
	}
}

func TestPrepareLMStudioPromptLeavesNonQwenModelsAlone(t *testing.T) {
	client := NewLLMClient(appconfig.LLMProviderLMStudio, "", "google/gemma-4-e4b", "http://127.0.0.1:1234", false)

	got := client.prepareLMStudioPrompt("Reply with only true.")

	if got != "Reply with only true." {
		t.Fatalf("prepareLMStudioPrompt() = %q", got)
	}
}

func TestPrepareLMStudioPromptLeavesQwenAloneWhenThinkingEnabled(t *testing.T) {
	client := NewLLMClient(appconfig.LLMProviderLMStudio, "", "qwen/qwen3.6-35b-a3b", "http://127.0.0.1:1234", true)

	got := client.prepareLMStudioPrompt("Reply with JSON only.")

	if got != "Reply with JSON only." {
		t.Fatalf("prepareLMStudioPrompt() = %q", got)
	}
}

func TestBuildPromptLMStudioCompactsLargeContext(t *testing.T) {
	client := NewLLMClient(appconfig.LLMProviderLMStudio, "", "qwen/qwen3.6-35b-a3b", "http://127.0.0.1:1234", false)
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

	if !strings.Contains(prompt, "[directory listing truncated") && !strings.Contains(prompt, "...[truncated]...") {
		t.Fatalf("expected prompt compaction markers, got: %s", prompt)
	}
	if len(prompt) > 7000 {
		t.Fatalf("expected compact LM Studio prompt, got length %d", len(prompt))
	}
}

func TestBuildPromptGeminiKeepsLargeContext(t *testing.T) {
	client := NewLLMClient(appconfig.LLMProviderGemini, "", "gemini-2.5-flash", "", false)
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

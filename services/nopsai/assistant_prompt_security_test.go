package nopsai

import (
	"strings"
	"testing"
)

func TestAssistantPromptSafeValueRedactsNestedSecrets(t *testing.T) {
	input := map[string]any{
		"log": "Authorization: Bearer bearer-secret password=db-password postgres://user:db-secret@db.internal/app",
		"nested": []any{
			map[string]any{"api_key": "api_key=provider-secret"},
		},
		"labels": map[string]string{
			"credential": "client_secret=client-secret-value",
		},
	}

	got, ok := assistantPromptSafeValue(input).(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", assistantPromptSafeValue(input))
	}

	rendered := strings.TrimSpace(strings.Join([]string{
		got["log"].(string),
		got["nested"].([]any)[0].(map[string]any)["api_key"].(string),
		got["labels"].(map[string]string)["credential"],
	}, "\n"))

	for _, secret := range []string{
		"bearer-secret",
		"db-password",
		"db-secret",
		"provider-secret",
		"client-secret-value",
	} {
		if strings.Contains(rendered, secret) {
			t.Fatalf("secret %q remained in redacted prompt value: %s", secret, rendered)
		}
	}
	if !strings.Contains(rendered, "[REDACTED]") {
		t.Fatalf("expected redaction marker, got: %s", rendered)
	}
}

func TestBuildAssistantLLMPromptRedactsAllContextSources(t *testing.T) {
	conversation := assistantConversation{
		Memory: assistantConversationMemory{
			Summary:  "token=memory-secret",
			Entities: map[string]any{"credential": "password=entity-secret"},
		},
		Messages: []assistantMessage{
			{Role: "user", Content: "api_key=history-secret"},
		},
	}
	toolCalls := []assistantToolActivity{
		{
			Name:   "pipeline.logs",
			Status: assistantToolStatusSuccess,
			Input:  map[string]any{"authorization": "Authorization: Bearer input-secret"},
			Output: map[string]any{"line": "refresh_token=output-secret"},
		},
	}

	prompt := buildAssistantLLMPrompt(
		conversation,
		"access_token=user-secret",
		assistantTurnPlan{},
		toolCalls,
		"client_secret=summary-secret",
	)

	for _, secret := range []string{
		"memory-secret",
		"entity-secret",
		"history-secret",
		"input-secret",
		"output-secret",
		"user-secret",
		"summary-secret",
	} {
		if strings.Contains(prompt, secret) {
			t.Fatalf("secret %q remained in LLM prompt", secret)
		}
	}
	if count := strings.Count(prompt, "[REDACTED]"); count < 7 {
		t.Fatalf("expected redaction markers for all prompt sources, got %d", count)
	}
}

func TestAssistantPromptRedactionPreservesUTF8Boundary(t *testing.T) {
	value := strings.Repeat("界", assistantPromptHistoryContentLimit)
	got := assistantTruncateHistoryContent(value)
	if !strings.ValidUTF8(got) {
		t.Fatal("redacted history value is not valid UTF-8")
	}
	if len(got) > assistantPromptHistoryContentLimit {
		t.Fatalf("history value exceeds byte limit: %d", len(got))
	}
}

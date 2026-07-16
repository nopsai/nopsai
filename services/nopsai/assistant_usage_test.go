package nopsai

import (
	"testing"
	"time"

	"nopsai/pkg/llmclient"
)

func TestAssistantUsageForAssistantReplySumsLLMCallsAndDuration(t *testing.T) {
	usage := assistantUsageForAssistantReply("Final answer", []assistantToolActivity{
		{
			Name: assistantLLMPlannerToolName,
			Output: map[string]any{
				"usage": llmclient.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
		},
		{
			Name: assistantLLMToolName,
			Output: map[string]any{
				"usage": map[string]any{
					"prompt_tokens":     float64(8),
					"completion_tokens": float64(4),
					"total_tokens":      float64(12),
					"estimated":         true,
				},
			},
		},
		{
			Name:   "nopsai.get_pipeline",
			Output: map[string]any{"usage": map[string]any{"total_tokens": float64(999)}},
		},
	}, 1500*time.Millisecond)

	if usage.PromptTokens != 18 || usage.CompletionTokens != 9 || usage.TotalTokens != 27 {
		t.Fatalf("usage tokens = %#v, want summed planner and synthesis tokens", usage)
	}
	if usage.LLMCalls != 2 {
		t.Fatalf("llm calls = %d, want 2", usage.LLMCalls)
	}
	if usage.DurationMS != 1500 {
		t.Fatalf("duration = %d, want 1500", usage.DurationMS)
	}
	if !usage.Estimated {
		t.Fatalf("estimated = false, want true when any LLM call reports estimated usage")
	}
	if usage.ContentTokens == 0 {
		t.Fatalf("content token estimate missing: %#v", usage)
	}
}

func TestAssistantUsageForAssistantReplyTracksDeterministicVisibleContentOnly(t *testing.T) {
	usage := assistantUsageForAssistantReply("Deterministic answer.", nil, time.Nanosecond)

	if usage.ContentTokens == 0 {
		t.Fatalf("content token estimate missing: %#v", usage)
	}
	if usage.TotalTokens != 0 || usage.CompletionTokens != 0 || usage.LLMCalls != 0 || usage.Estimated {
		t.Fatalf("usage = %#v, want visible content without provider tokens", usage)
	}
	if usage.DurationMS != 1 {
		t.Fatalf("duration = %d, want sub-millisecond duration rounded up", usage.DurationMS)
	}
}

func TestAssistantUsageForAssistantReplyEstimatesSuccessfulLLMCallWithMissingUsage(t *testing.T) {
	usage := assistantUsageForAssistantReply("LLM answer.", []assistantToolActivity{{
		Name:   assistantLLMToolName,
		Status: assistantToolStatusSuccess,
		Output: map[string]any{},
	}}, time.Second)

	if usage.TotalTokens == 0 || usage.CompletionTokens == 0 || !usage.Estimated {
		t.Fatalf("usage = %#v, want estimated provider tokens for successful LLM call without usage", usage)
	}
	if usage.LLMCalls != 1 {
		t.Fatalf("llm calls = %d, want 1", usage.LLMCalls)
	}
}

func TestAssistantUsageForUserMessageTracksVisibleContentOnly(t *testing.T) {
	usage := assistantUsageForUserMessage("set TEST_VAR = check check")

	if usage.ContentTokens == 0 || !usage.Estimated {
		t.Fatalf("usage = %#v, want estimated visible user content tokens", usage)
	}
	if usage.PromptTokens != 0 || usage.CompletionTokens != 0 || usage.TotalTokens != 0 || usage.LLMCalls != 0 {
		t.Fatalf("provider usage = %#v, want no provider prompt/completion tokens for user messages", usage)
	}
}

func TestAssistantConversationUsageFromMessagesRollsUpMessageMetrics(t *testing.T) {
	messages := []assistantMessage{
		{Usage: assistantMessageUsage{ContentTokens: 4, Estimated: true}},
		{Usage: assistantMessageUsage{ContentTokens: 3, PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15, DurationMS: 1200, LLMCalls: 1}},
	}

	usage := assistantConversationUsageFromMessages(messages)

	if usage.MessageCount != 2 || usage.ContentTokens != 7 || usage.PromptTokens != 10 || usage.CompletionTokens != 5 || usage.TotalTokens != 15 {
		t.Fatalf("usage rollup = %#v", usage)
	}
	if usage.EstimatedTokenMessages != 1 || usage.DurationMS != 1200 || usage.LLMCalls != 1 {
		t.Fatalf("usage metadata rollup = %#v", usage)
	}
}

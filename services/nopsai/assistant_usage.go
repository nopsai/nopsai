package nopsai

import (
	"strings"
	"time"

	"nopsai/pkg/llmclient"
)

func assistantUsageForUserMessage(content string) assistantMessageUsage {
	tokens := assistantEstimateTokenCount(content)
	return normalizeAssistantMessageUsage(content, assistantMessageUsage{
		ContentTokens: tokens,
		TotalTokens:   tokens,
		Estimated:     tokens > 0,
	})
}

func assistantUsageForAssistantReply(content string, toolCalls []assistantToolActivity, duration time.Duration) assistantMessageUsage {
	usage := assistantMessageUsage{
		ContentTokens: assistantEstimateTokenCount(content),
		DurationMS:    assistantDurationMilliseconds(duration),
	}
	for _, call := range toolCalls {
		callUsage, ok := assistantLLMUsageFromToolCall(call)
		if !ok {
			continue
		}
		usage.PromptTokens += callUsage.PromptTokens
		usage.CompletionTokens += callUsage.CompletionTokens
		usage.TotalTokens += callUsage.TotalTokens
		usage.LLMCalls++
		usage.Estimated = usage.Estimated || callUsage.Estimated
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 {
		usage.CompletionTokens = usage.ContentTokens
		usage.TotalTokens = usage.ContentTokens
		usage.Estimated = usage.ContentTokens > 0
	}
	return normalizeAssistantMessageUsage(content, usage)
}

func assistantLLMUsageFromToolCall(call assistantToolActivity) (assistantMessageUsage, bool) {
	if call.Name != assistantLLMPlannerToolName && call.Name != assistantLLMToolName {
		return assistantMessageUsage{}, false
	}
	switch usage := call.Output["usage"].(type) {
	case llmclient.Usage:
		return assistantMessageUsageFromLLMUsage(usage)
	case map[string]any:
		return assistantMessageUsageFromMap(usage)
	default:
		return assistantMessageUsage{}, false
	}
}

func assistantMessageUsageFromLLMUsage(usage llmclient.Usage) (assistantMessageUsage, bool) {
	out := assistantMessageUsage{
		PromptTokens:     nonNegativeInt64(usage.PromptTokens),
		CompletionTokens: nonNegativeInt64(usage.CompletionTokens),
		TotalTokens:      nonNegativeInt64(usage.TotalTokens),
		Estimated:        usage.Estimated,
	}
	if out.TotalTokens == 0 {
		out.TotalTokens = out.PromptTokens + out.CompletionTokens
	}
	return out, out.PromptTokens > 0 || out.CompletionTokens > 0 || out.TotalTokens > 0
}

func assistantMessageUsageFromMap(usage map[string]any) (assistantMessageUsage, bool) {
	out := assistantMessageUsage{
		PromptTokens:     assistantUsageInt64(usage["prompt_tokens"]),
		CompletionTokens: assistantUsageInt64(usage["completion_tokens"]),
		TotalTokens:      assistantUsageInt64(usage["total_tokens"]),
		Estimated:        assistantUsageBool(usage["estimated"]),
	}
	if out.TotalTokens == 0 {
		out.TotalTokens = out.PromptTokens + out.CompletionTokens
	}
	return out, out.PromptTokens > 0 || out.CompletionTokens > 0 || out.TotalTokens > 0
}

func normalizeAssistantMessageUsage(content string, usage assistantMessageUsage) assistantMessageUsage {
	usage.ContentTokens = nonNegativeInt64(usage.ContentTokens)
	usage.PromptTokens = nonNegativeInt64(usage.PromptTokens)
	usage.CompletionTokens = nonNegativeInt64(usage.CompletionTokens)
	usage.TotalTokens = nonNegativeInt64(usage.TotalTokens)
	usage.DurationMS = nonNegativeInt64(usage.DurationMS)
	if usage.LLMCalls < 0 {
		usage.LLMCalls = 0
	}
	if usage.ContentTokens == 0 {
		usage.ContentTokens = assistantEstimateTokenCount(content)
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return usage
}

func assistantConversationUsageFromMessages(messages []assistantMessage) assistantConversationUsage {
	usage := assistantConversationUsage{MessageCount: len(messages)}
	for _, message := range messages {
		usage.ContentTokens += message.Usage.ContentTokens
		usage.PromptTokens += message.Usage.PromptTokens
		usage.CompletionTokens += message.Usage.CompletionTokens
		usage.TotalTokens += message.Usage.TotalTokens
		usage.DurationMS += message.Usage.DurationMS
		usage.LLMCalls += message.Usage.LLMCalls
		if message.Usage.Estimated {
			usage.EstimatedTokenMessages++
		}
	}
	return usage
}

func assistantEstimateTokenCount(text string) int64 {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	runes := len([]rune(text))
	estimate := int64((runes + 3) / 4)
	if estimate < 1 {
		return 1
	}
	return estimate
}

func assistantDurationMilliseconds(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	millis := duration.Milliseconds()
	if millis == 0 {
		return 1
	}
	return millis
}

func assistantUsageInt64(value any) int64 {
	switch typed := value.(type) {
	case int:
		return nonNegativeInt64(int64(typed))
	case int64:
		return nonNegativeInt64(typed)
	case float64:
		return nonNegativeInt64(int64(typed))
	case jsonNumber:
		parsed, err := typed.Int64()
		if err != nil {
			return 0
		}
		return nonNegativeInt64(parsed)
	default:
		return 0
	}
}

func assistantUsageBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

type jsonNumber interface {
	Int64() (int64, error)
}

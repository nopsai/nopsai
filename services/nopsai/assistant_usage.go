package nopsai

import (
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/llmclient"
)

func assistantUsageForUserMessage(content string) assistantMessageUsage {
	tokens := assistantEstimateTokenCount(content)
	return normalizeAssistantMessageUsage(content, assistantMessageUsage{
		ContentTokens: tokens,
		Estimated:     tokens > 0,
	})
}

// assistantLLMPricer resolves the rate card for a model by name. It is passed in
// rather than looked up here so that usage assembly stays independent of the
// configuration snapshot.
type assistantLLMPricer func(profileName string) *config.LLMPricing

func assistantUsageForAssistantReply(content string, toolCalls []assistantToolActivity, duration time.Duration, pricer assistantLLMPricer) assistantMessageUsage {
	usage := assistantMessageUsage{
		ContentTokens: assistantEstimateTokenCount(content),
		DurationMS:    assistantDurationMilliseconds(duration),
	}
	missingSuccessfulLLMUsage := false
	// Every LLM call in a turn is priced on its own, because a turn can mix
	// models: the planner and the synthesis step need not share a profile, and
	// summing their tokens before pricing would charge both at one rate.
	priced := true
	cost := 0.0
	for _, call := range toolCalls {
		if !assistantIsLLMToolCall(call) {
			continue
		}
		callUsage, ok := assistantLLMUsageFromToolCall(call)
		if !ok {
			if call.Status == assistantToolStatusSuccess {
				missingSuccessfulLLMUsage = true
				usage.LLMCalls++
				priced = false
			}
			continue
		}
		usage.PromptTokens += callUsage.PromptTokens
		usage.CompletionTokens += callUsage.CompletionTokens
		usage.TotalTokens += callUsage.TotalTokens
		usage.LLMCalls++
		usage.Estimated = usage.Estimated || callUsage.Estimated
		if callCost, ok := assistantLLMCallCost(call, callUsage, pricer); ok {
			cost += callCost
		} else {
			priced = false
		}
	}
	if usage.LLMCalls > 0 && priced {
		usage.CostUSD = &cost
	}
	if missingSuccessfulLLMUsage && usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 {
		usage.CompletionTokens = usage.ContentTokens
		usage.TotalTokens = usage.ContentTokens
		usage.Estimated = usage.ContentTokens > 0
	}
	return normalizeAssistantMessageUsage(content, usage)
}

// assistantLLMCallCost prices one LLM call. It declines to produce a figure for
// an estimated token count, since pricing a guess yields a number that reads as
// authoritative and is not.
func assistantLLMCallCost(call assistantToolActivity, usage assistantMessageUsage, pricer assistantLLMPricer) (float64, bool) {
	if pricer == nil || usage.Estimated {
		return 0, false
	}
	pricing := pricer(assistantToolCallProfileName(call))
	if pricing == nil {
		return 0, false
	}
	cached, cacheWrite := assistantToolCallCacheTokens(call)
	input, output := pricing.CostUSD(usage.PromptTokens, usage.CompletionTokens, cached, cacheWrite)
	return input + output, true
}

func assistantToolCallProfileName(call assistantToolActivity) string {
	switch usage := call.Output["usage"].(type) {
	case llmclient.Usage:
		return usage.Profile
	case map[string]any:
		if name, ok := usage["profile"].(string); ok {
			return name
		}
	}
	if name, ok := call.Output["profile"].(string); ok {
		return name
	}
	return ""
}

func assistantToolCallCacheTokens(call assistantToolActivity) (cached int64, cacheWrite int64) {
	switch usage := call.Output["usage"].(type) {
	case llmclient.Usage:
		return usage.CachedInputTokens, usage.CacheWriteTokens
	case map[string]any:
		return assistantUsageInt64(usage["cached_input_tokens"]), assistantUsageInt64(usage["cache_write_tokens"])
	default:
		return 0, 0
	}
}

func assistantIsLLMToolCall(call assistantToolActivity) bool {
	return call.Name == assistantLLMPlannerToolName || call.Name == assistantLLMToolName
}

func assistantLLMUsageFromToolCall(call assistantToolActivity) (assistantMessageUsage, bool) {
	if !assistantIsLLMToolCall(call) {
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
		if message.Usage.LLMCalls == 0 {
			continue
		}
		if message.Usage.CostUSD == nil {
			usage.UnpricedTurns++
			continue
		}
		usage.SpendUSD += *message.Usage.CostUSD
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

// assistantLLMPricer resolves rate cards from the current configuration
// snapshot. A model that has since been removed from the configuration
// repository prices as nil, so its spend is reported as unpriced rather than as
// zero.
func (a *App) assistantLLMPricer() assistantLLMPricer {
	_, profiles := a.llmProfilesSnapshot()
	return func(profileName string) *config.LLMPricing {
		profile, ok := profiles[config.NormalizeLLMProfileName(profileName)]
		if !ok {
			return nil
		}
		return profile.Pricing
	}
}

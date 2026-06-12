package llm

import (
	"context"
	"strings"
	"sync"
)

type Usage struct {
	Provider         string
	Model            string
	Profile          string
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	InputCostUSD     float64
	OutputCostUSD    float64
	TotalCostUSD     float64
	Estimated        bool
}

type UsageCollector struct {
	mu    sync.Mutex
	items []Usage
}

type usageCollectorContextKey struct{}

func NewUsageCollector() *UsageCollector {
	return &UsageCollector{}
}

func ContextWithUsageCollector(ctx context.Context, collector *UsageCollector) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if collector == nil {
		return ctx
	}
	return context.WithValue(ctx, usageCollectorContextKey{}, collector)
}

func (c *UsageCollector) Snapshot() []Usage {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Usage, len(c.items))
	copy(out, c.items)
	return out
}

func recordUsage(ctx context.Context, usage Usage) {
	if ctx == nil {
		return
	}
	collector, ok := ctx.Value(usageCollectorContextKey{}).(*UsageCollector)
	if !ok || collector == nil {
		return
	}
	usage.Provider = strings.TrimSpace(usage.Provider)
	usage.Model = strings.TrimSpace(usage.Model)
	usage.Profile = strings.TrimSpace(usage.Profile)
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if usage.TotalCostUSD == 0 {
		usage.TotalCostUSD = usage.InputCostUSD + usage.OutputCostUSD
	}
	if usage.PromptTokens <= 0 && usage.CompletionTokens <= 0 && usage.TotalTokens <= 0 && usage.TotalCostUSD <= 0 {
		return
	}
	collector.mu.Lock()
	collector.items = append(collector.items, usage)
	collector.mu.Unlock()
}

func usageFromTokens(provider, model, profile, prompt, completion string, promptTokens, completionTokens, totalTokens int64) Usage {
	usage := Usage{
		Provider:         provider,
		Model:            model,
		Profile:          profile,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if usage.PromptTokens > 0 || usage.CompletionTokens > 0 || usage.TotalTokens > 0 {
		return usage
	}
	usage.PromptTokens = estimateTokenCount(prompt)
	usage.CompletionTokens = estimateTokenCount(completion)
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	usage.Estimated = usage.TotalTokens > 0
	return usage
}

func estimateTokenCount(text string) int64 {
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

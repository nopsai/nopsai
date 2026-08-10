package llm

import (
	"context"
	"strings"
	"sync"
)

type Usage struct {
	Provider                    string
	Model                       string
	Profile                     string
	PromptTokens                int64
	CompletionTokens            int64
	TotalTokens                 int64
	CachedInputTokens           int64
	InputCostUSD                float64
	OutputCostUSD               float64
	TotalCostUSD                float64
	PromptSHA256                string
	PromptBytes                 int
	EstimatedInputTokens        int64
	StaticContextSHA256         string
	StaticContextCacheKey       string
	HistoryRevision             uint64
	WorkspaceRevision           uint64
	KnowledgeRevision           string
	PolicyRevision              string
	GovernanceLevel             string
	GovernanceContractVersion   string
	EffectivePolicySnapshotHash string
	CacheIdentitySHA256         string
	PromptSchemaVersion         string
	ExecutionMode               string
	LogicalSessionID            string
	ProviderStateID             string
	ProviderStateUsed           bool
	ProviderStateSupported      bool
	ProviderStateSupportKnown   bool
	PromptCacheSupported        bool
	PromptCacheSupportKnown     bool
	PromptCacheHit              bool
	PromptCacheMode             string
	ProviderStateMode           string
	StablePrefixTokens          int64
	DynamicContextTokens        int64
	UncachedInputTokens         int64
	CacheWriteTokens            int64
	SharedFileCount             int
	SharedFileBytes             int
	WorkspaceToolCallCount      int
	WorkspaceToolResultBytes    int
	Estimated                   bool
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
	if metadata, ok := completionMetadataFromContext(ctx); ok {
		mergeCompletionMetadata(&usage, metadata)
	}
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
	return usageFromTokenDetails(provider, model, profile, prompt, completion, promptTokens, completionTokens, totalTokens, 0)
}

func usageFromTokenDetails(provider, model, profile, prompt, completion string, promptTokens, completionTokens, totalTokens, cachedInputTokens int64) Usage {
	return usageFromTokenDetailsWithClient(nil, provider, model, profile, prompt, completion, promptTokens, completionTokens, totalTokens, cachedInputTokens)
}

func usageFromTokenDetailsForClient(owner *LLMClient, model, prompt, completion string, promptTokens, completionTokens, totalTokens, cachedInputTokens int64) Usage {
	provider := ""
	profile := ""
	if owner != nil {
		provider = owner.provider
		profile = owner.profile
	}
	return usageFromTokenDetailsWithClient(owner, provider, model, profile, prompt, completion, promptTokens, completionTokens, totalTokens, cachedInputTokens)
}

func usageFromTokenDetailsWithClient(owner *LLMClient, provider, model, profile, prompt, completion string, promptTokens, completionTokens, totalTokens, cachedInputTokens int64) Usage {
	promptMeta := newPromptMetadata(nil, prompt)
	usage := Usage{
		Provider:                    provider,
		Model:                       model,
		Profile:                     profile,
		PromptTokens:                promptTokens,
		CompletionTokens:            completionTokens,
		TotalTokens:                 totalTokens,
		CachedInputTokens:           cachedInputTokens,
		PromptSHA256:                promptMeta.PromptSHA256,
		PromptBytes:                 promptMeta.PromptBytes,
		EstimatedInputTokens:        promptMeta.EstimatedInputTokens,
		StaticContextSHA256:         promptMeta.StaticContextSHA256,
		HistoryRevision:             promptMeta.HistoryRevision,
		WorkspaceRevision:           promptMeta.WorkspaceRevision,
		KnowledgeRevision:           promptMeta.KnowledgeRevision,
		PolicyRevision:              promptMeta.PolicyRevision,
		GovernanceLevel:             promptMeta.GovernanceLevel,
		GovernanceContractVersion:   promptMeta.GovernanceContractVersion,
		EffectivePolicySnapshotHash: promptMeta.EffectivePolicySnapshotHash,
		CacheIdentitySHA256:         promptMeta.CacheIdentitySHA256,
		SharedFileCount:             promptMeta.SharedFileCount,
		SharedFileBytes:             promptMeta.SharedFileBytes,
		WorkspaceToolCallCount:      promptMeta.WorkspaceToolCallCount,
		WorkspaceToolResultBytes:    promptMeta.WorkspaceToolResultBytes,
	}
	if promptMeta.StaticContextSHA256 != "" {
		if owner != nil {
			usage.StaticContextCacheKey = staticContextCacheKey(owner, promptMeta.StaticContextSHA256, model)
		} else {
			usage.StaticContextCacheKey = promptSHA256(strings.Join([]string{
				strings.TrimSpace(provider),
				strings.TrimSpace(model),
				strings.TrimSpace(profile),
				promptMeta.StaticContextSHA256,
			}, "\n"))
		}
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

func mergeCompletionMetadata(usage *Usage, metadata completionMetadata) {
	if usage == nil {
		return
	}
	if usage.ExecutionMode == "" {
		usage.ExecutionMode = metadata.ExecutionMode
	}
	if usage.LogicalSessionID == "" {
		usage.LogicalSessionID = metadata.LogicalSessionID
	}
	if usage.ProviderStateID == "" {
		usage.ProviderStateID = metadata.ProviderStateID
	}
	usage.ProviderStateUsed = usage.ProviderStateUsed || metadata.ProviderStateUsed
	if !usage.ProviderStateSupportKnown && metadata.ProviderStateSupportKnown {
		usage.ProviderStateSupportKnown = true
		usage.ProviderStateSupported = metadata.ProviderStateSupported
	}
	if !usage.PromptCacheSupportKnown && metadata.PromptCacheSupportKnown {
		usage.PromptCacheSupportKnown = true
		usage.PromptCacheSupported = metadata.PromptCacheSupported
	}
	if usage.PromptCacheMode == "" {
		usage.PromptCacheMode = metadata.PromptCacheMode
	}
	if usage.ProviderStateMode == "" {
		usage.ProviderStateMode = metadata.ProviderStateMode
	}
	if usage.CacheIdentitySHA256 == "" {
		usage.CacheIdentitySHA256 = metadata.CacheIdentitySHA256
	}
	if usage.PromptSchemaVersion == "" {
		usage.PromptSchemaVersion = metadata.PromptSchemaVersion
	}
	if usage.GovernanceLevel == "" {
		usage.GovernanceLevel = metadata.GovernanceLevel
	}
	if usage.GovernanceContractVersion == "" {
		usage.GovernanceContractVersion = metadata.GovernanceContractVersion
	}
	if usage.EffectivePolicySnapshotHash == "" {
		usage.EffectivePolicySnapshotHash = metadata.EffectivePolicySnapshotHash
	}
	if usage.StablePrefixTokens == 0 {
		usage.StablePrefixTokens = metadata.StablePrefixTokens
	}
	if usage.DynamicContextTokens == 0 {
		usage.DynamicContextTokens = metadata.DynamicContextTokens
	}
	if usage.CachedInputTokens > 0 {
		usage.PromptCacheHit = true
	}
	if usage.UncachedInputTokens == 0 && usage.PromptTokens > usage.CachedInputTokens {
		usage.UncachedInputTokens = usage.PromptTokens - usage.CachedInputTokens
	}
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

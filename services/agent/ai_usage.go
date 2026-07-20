package agent

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nopsai/pkg/models"
	llmruntime "nopsai/services/agent/internal/llm"
)

type aiUsageReporter interface {
	ReportAIUsage(context.Context, models.AIUsageReport) error
}

type nopsaiAIUsageReporter struct {
	runID string
}

func newNopsaiAIUsageReporter(runID string) nopsaiAIUsageReporter {
	return nopsaiAIUsageReporter{runID: strings.TrimSpace(runID)}
}

func (r nopsaiAIUsageReporter) ReportAIUsage(ctx context.Context, report models.AIUsageReport) error {
	if r.runID == "" {
		return fmt.Errorf("run ID is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return nopsaiAgentRequest(reqCtx, http.MethodPost, fmt.Sprintf("/v1/internal/runs/%s/ai-usage", r.runID), report, nil)
}

func reportCollectedAIUsage(ctx context.Context, reporter aiUsageReporter, feature, stepName, taskName, agentProfile string, usages []llmruntime.Usage) {
	if reporter == nil || len(usages) == 0 {
		return
	}
	for _, usage := range usages {
		report := aiUsageReportFromLLMUsage(feature, stepName, taskName, agentProfile, usage)
		if !aiUsageReportHasValue(report) {
			continue
		}
		if err := reporter.ReportAIUsage(ctx, report); err != nil {
			stepLog("", "", stepName, taskName).Warn().Err(err).Msg("Failed to report AI usage")
		}
	}
}

func aiUsageReportFromLLMUsage(feature, stepName, taskName, agentProfile string, usage llmruntime.Usage) models.AIUsageReport {
	metadata := map[string]any{}
	if agentProfile = strings.TrimSpace(agentProfile); agentProfile != "" {
		metadata["agent_profile"] = agentProfile
	}
	if usage.Estimated {
		metadata["estimated_tokens"] = true
	}
	if promptSHA := strings.TrimSpace(usage.PromptSHA256); promptSHA != "" {
		metadata["prompt_sha256"] = promptSHA
	}
	if usage.PromptBytes > 0 {
		metadata["prompt_bytes"] = usage.PromptBytes
	}
	if usage.EstimatedInputTokens > 0 {
		metadata["estimated_input_tokens"] = usage.EstimatedInputTokens
	}
	if usage.CachedInputTokens > 0 {
		metadata["cached_input_tokens"] = usage.CachedInputTokens
	}
	if staticSHA := strings.TrimSpace(usage.StaticContextSHA256); staticSHA != "" {
		metadata["static_context_sha256"] = staticSHA
	}
	if cacheKey := strings.TrimSpace(usage.StaticContextCacheKey); cacheKey != "" {
		metadata["static_context_cache_key"] = cacheKey
	}
	if usage.HistoryRevision > 0 {
		metadata["history_revision"] = usage.HistoryRevision
	}
	if usage.WorkspaceRevision > 0 {
		metadata["workspace_revision"] = usage.WorkspaceRevision
	}
	if knowledgeRevision := strings.TrimSpace(usage.KnowledgeRevision); knowledgeRevision != "" {
		metadata["knowledge_revision"] = knowledgeRevision
	}
	if policyRevision := strings.TrimSpace(usage.PolicyRevision); policyRevision != "" {
		metadata["policy_revision"] = policyRevision
	}
	if policyMergeMode := strings.TrimSpace(usage.PolicyMergeMode); policyMergeMode != "" {
		metadata["policy_merge_mode"] = policyMergeMode
	}
	if policyPrecedenceVersion := strings.TrimSpace(usage.PolicyPrecedenceVersion); policyPrecedenceVersion != "" {
		metadata["policy_precedence_version"] = policyPrecedenceVersion
	}
	if effectivePolicyHash := strings.TrimSpace(usage.EffectivePolicySnapshotHash); effectivePolicyHash != "" {
		metadata["effective_policy_snapshot_hash"] = effectivePolicyHash
	}
	if cacheIdentity := strings.TrimSpace(usage.CacheIdentitySHA256); cacheIdentity != "" {
		metadata["cache_identity_sha256"] = cacheIdentity
	}
	if promptSchemaVersion := strings.TrimSpace(usage.PromptSchemaVersion); promptSchemaVersion != "" {
		metadata["prompt_schema_version"] = promptSchemaVersion
	}
	if executionMode := strings.TrimSpace(usage.ExecutionMode); executionMode != "" {
		metadata["execution_mode"] = executionMode
	}
	if sessionID := strings.TrimSpace(usage.LogicalSessionID); sessionID != "" {
		metadata["logical_session_id"] = sessionID
	}
	if providerStateID := strings.TrimSpace(usage.ProviderStateID); providerStateID != "" {
		metadata["provider_state_id"] = providerStateID
	}
	if usage.ProviderStateUsed {
		metadata["provider_state_used"] = true
	}
	if usage.ProviderStateSupportKnown {
		metadata["provider_state_supported"] = usage.ProviderStateSupported
	}
	if usage.PromptCacheSupportKnown {
		metadata["prompt_cache_supported"] = usage.PromptCacheSupported
	}
	if usage.PromptCacheHit {
		metadata["prompt_cache_hit"] = true
	}
	if promptCacheMode := strings.TrimSpace(usage.PromptCacheMode); promptCacheMode != "" {
		metadata["prompt_cache_mode"] = promptCacheMode
	}
	if providerStateMode := strings.TrimSpace(usage.ProviderStateMode); providerStateMode != "" {
		metadata["provider_state_mode"] = providerStateMode
	}
	if usage.StablePrefixTokens > 0 {
		metadata["stable_prefix_tokens"] = usage.StablePrefixTokens
	}
	if usage.DynamicContextTokens > 0 {
		metadata["dynamic_context_tokens"] = usage.DynamicContextTokens
	}
	if usage.UncachedInputTokens > 0 {
		metadata["uncached_input_tokens"] = usage.UncachedInputTokens
	}
	if usage.CacheWriteTokens > 0 {
		metadata["cache_write_tokens"] = usage.CacheWriteTokens
	}
	if usage.SharedFileCount > 0 {
		metadata["shared_file_count"] = usage.SharedFileCount
	}
	if usage.SharedFileBytes > 0 {
		metadata["shared_file_bytes"] = usage.SharedFileBytes
	}
	if usage.WorkspaceToolCallCount > 0 {
		metadata["workspace_tool_call_count"] = usage.WorkspaceToolCallCount
	}
	if usage.WorkspaceToolResultBytes > 0 {
		metadata["workspace_tool_result_bytes"] = usage.WorkspaceToolResultBytes
	}
	return models.AIUsageReport{
		StepName:         strings.TrimSpace(stepName),
		TaskName:         strings.TrimSpace(taskName),
		Feature:          strings.TrimSpace(feature),
		Provider:         strings.TrimSpace(usage.Provider),
		Model:            strings.TrimSpace(usage.Model),
		LLMProfile:       strings.TrimSpace(usage.Profile),
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		InputCostUSD:     usage.InputCostUSD,
		OutputCostUSD:    usage.OutputCostUSD,
		TotalCostUSD:     usage.TotalCostUSD,
		Metadata:         metadata,
	}
}

func aiUsageReportHasValue(report models.AIUsageReport) bool {
	return report.PromptTokens > 0 || report.CompletionTokens > 0 || report.TotalTokens > 0 ||
		report.InputCostUSD > 0 || report.OutputCostUSD > 0 || report.TotalCostUSD > 0
}

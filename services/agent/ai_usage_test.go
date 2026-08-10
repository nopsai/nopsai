package agent

import (
	"testing"

	llmruntime "nopsai/services/agent/internal/llm"
)

func TestAIUsageReportFromLLMUsageMarksEstimatedTokens(t *testing.T) {
	report := aiUsageReportFromLLMUsage("goal_resolution", "build", "lint", "sre", llmruntime.Usage{
		Provider:                    "lmstudio",
		Model:                       "model-a",
		Profile:                     "local",
		PromptTokens:                12,
		TotalTokens:                 12,
		CachedInputTokens:           4,
		Estimated:                   true,
		PromptSHA256:                "abc123",
		PromptBytes:                 48,
		EstimatedInputTokens:        12,
		StaticContextSHA256:         "static123",
		StaticContextCacheKey:       "cache123",
		HistoryRevision:             7,
		WorkspaceRevision:           8,
		KnowledgeRevision:           "knowledge123",
		PolicyRevision:              "policy456",
		GovernanceLevel:             "strict",
		GovernanceContractVersion:   "2026-08-10.v1",
		EffectivePolicySnapshotHash: "effective789",
		CacheIdentitySHA256:         "cache-identity",
		PromptSchemaVersion:         "nopsai.completion.v1",
		ExecutionMode:               "stateless_prompt_cached",
		LogicalSessionID:            "session-1",
		PromptCacheSupported:        true,
		PromptCacheSupportKnown:     true,
		PromptCacheHit:              true,
		PromptCacheMode:             "auto",
		ProviderStateSupported:      false,
		ProviderStateSupportKnown:   true,
		ProviderStateMode:           "disabled",
		StablePrefixTokens:          10,
		DynamicContextTokens:        2,
		UncachedInputTokens:         8,
		SharedFileCount:             2,
		SharedFileBytes:             320,
		WorkspaceToolCallCount:      1,
		WorkspaceToolResultBytes:    96,
	})
	if report.Metadata["agent_profile"] != "sre" {
		t.Fatalf("agent profile metadata = %#v", report.Metadata)
	}
	if report.Metadata["estimated_tokens"] != true {
		t.Fatalf("estimated metadata = %#v", report.Metadata)
	}
	if report.Metadata["prompt_sha256"] != "abc123" ||
		report.Metadata["prompt_bytes"] != 48 ||
		report.Metadata["estimated_input_tokens"] != int64(12) {
		t.Fatalf("prompt metadata = %#v", report.Metadata)
	}
	if report.Metadata["static_context_sha256"] != "static123" ||
		report.Metadata["static_context_cache_key"] != "cache123" {
		t.Fatalf("static context metadata = %#v", report.Metadata)
	}
	for key, want := range map[string]any{
		"cached_input_tokens":            int64(4),
		"history_revision":               uint64(7),
		"workspace_revision":             uint64(8),
		"knowledge_revision":             "knowledge123",
		"policy_revision":                "policy456",
		"governance_level":               "strict",
		"governance_contract_version":    "2026-08-10.v1",
		"effective_policy_snapshot_hash": "effective789",
		"cache_identity_sha256":          "cache-identity",
		"prompt_schema_version":          "nopsai.completion.v1",
		"execution_mode":                 "stateless_prompt_cached",
		"logical_session_id":             "session-1",
		"prompt_cache_supported":         true,
		"prompt_cache_hit":               true,
		"prompt_cache_mode":              "auto",
		"provider_state_supported":       false,
		"provider_state_mode":            "disabled",
		"stable_prefix_tokens":           int64(10),
		"dynamic_context_tokens":         int64(2),
		"uncached_input_tokens":          int64(8),
		"shared_file_count":              2,
		"shared_file_bytes":              320,
		"workspace_tool_call_count":      1,
		"workspace_tool_result_bytes":    96,
	} {
		if got := report.Metadata[key]; got != want {
			t.Fatalf("metadata[%s] = %#v, want %#v (all metadata %#v)", key, got, want, report.Metadata)
		}
	}
	if report.Feature != "goal_resolution" || report.StepName != "build" || report.TaskName != "lint" {
		t.Fatalf("report context = %#v", report)
	}
}

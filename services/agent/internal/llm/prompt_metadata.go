package llm

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
)

type promptMetadata struct {
	Provider                    string
	Model                       string
	Profile                     string
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
	SharedFileCount             int
	SharedFileBytes             int
	WorkspaceToolCallCount      int
	WorkspaceToolResultBytes    int
}

func newPromptMetadata(owner *LLMClient, prompt string) promptMetadata {
	directorySection := promptSection(prompt, "**Working Directory Contents:**", "---\n**Workspace Tools:**")
	sharedFileCount := strings.Count(directorySection, "--- File:")
	sharedFileBytes := 0
	if sharedFileCount > 0 {
		sharedFileBytes = len([]byte(directorySection))
	}
	workspaceToolSection := promptSection(prompt, "--- Workspace Tool Results For Current Goal ---", "--- MCP Tool Results For Current Goal ---", "---\n**Current Goal:**")
	meta := promptMetadata{
		PromptSHA256:                promptSHA256(prompt),
		PromptBytes:                 len([]byte(prompt)),
		EstimatedInputTokens:        estimateTokenCount(prompt),
		StaticContextSHA256:         promptSHA256(staticPromptPrefix(prompt)),
		HistoryRevision:             maxMetadataUint64(prompt, "history_revision"),
		WorkspaceRevision:           maxMetadataUint64(prompt, "workspace_revision"),
		KnowledgeRevision:           firstMetadataLineValue(prompt, "knowledge_revision"),
		PolicyRevision:              firstMetadataLineValue(prompt, "policy_revision"),
		GovernanceLevel:             firstNonEmpty(firstMetadataLineValue(prompt, "governance_level"), firstMetadataLineValue(prompt, "policy_merge_mode")),
		GovernanceContractVersion:   firstNonEmpty(firstMetadataLineValue(prompt, "governance_contract_version"), firstMetadataLineValue(prompt, "policy_precedence_version")),
		EffectivePolicySnapshotHash: firstMetadataLineValue(prompt, "effective_policy_snapshot_hash"),
		SharedFileCount:             sharedFileCount,
		SharedFileBytes:             sharedFileBytes,
		WorkspaceToolCallCount:      strings.Count(workspaceToolSection, "Workspace tool result:"),
		WorkspaceToolResultBytes:    len([]byte(strings.TrimSpace(workspaceToolSection))),
	}
	if owner != nil {
		meta.Provider = strings.TrimSpace(owner.provider)
		meta.Model = strings.TrimSpace(owner.model)
		meta.Profile = strings.TrimSpace(owner.profile)
		meta.StaticContextCacheKey = staticContextCacheKey(owner, meta.StaticContextSHA256)
		meta.CacheIdentitySHA256 = NewCacheIdentity(owner, prompt).Hash()
	}
	return meta
}

func promptSHA256(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return fmt.Sprintf("%x", sum)
}

func addPromptMetadata(event *zerolog.Event, meta promptMetadata) *zerolog.Event {
	event = event.
		Str("provider", meta.Provider).
		Str("model", meta.Model).
		Str("prompt_sha256", meta.PromptSHA256).
		Int("prompt_bytes", meta.PromptBytes).
		Int64("estimated_input_tokens", meta.EstimatedInputTokens).
		Str("static_context_sha256", meta.StaticContextSHA256)
	if meta.StaticContextCacheKey != "" {
		event = event.Str("static_context_cache_key", meta.StaticContextCacheKey)
	}
	if meta.Profile != "" {
		event = event.Str("model", meta.Profile)
	}
	if meta.HistoryRevision > 0 {
		event = event.Uint64("history_revision", meta.HistoryRevision)
	}
	if meta.WorkspaceRevision > 0 {
		event = event.Uint64("workspace_revision", meta.WorkspaceRevision)
	}
	if meta.KnowledgeRevision != "" {
		event = event.Str("knowledge_revision", meta.KnowledgeRevision)
	}
	if meta.PolicyRevision != "" {
		event = event.Str("policy_revision", meta.PolicyRevision)
	}
	if meta.GovernanceLevel != "" {
		event = event.Str("governance_level", meta.GovernanceLevel)
	}
	if meta.GovernanceContractVersion != "" {
		event = event.Str("governance_contract_version", meta.GovernanceContractVersion)
	}
	if meta.EffectivePolicySnapshotHash != "" {
		event = event.Str("effective_policy_snapshot_hash", meta.EffectivePolicySnapshotHash)
	}
	if meta.CacheIdentitySHA256 != "" {
		event = event.Str("cache_identity_sha256", meta.CacheIdentitySHA256)
	}
	if meta.SharedFileCount > 0 {
		event = event.Int("shared_file_count", meta.SharedFileCount)
	}
	if meta.SharedFileBytes > 0 {
		event = event.Int("shared_file_bytes", meta.SharedFileBytes)
	}
	if meta.WorkspaceToolCallCount > 0 {
		event = event.Int("workspace_tool_call_count", meta.WorkspaceToolCallCount)
	}
	if meta.WorkspaceToolResultBytes > 0 {
		event = event.Int("workspace_tool_result_bytes", meta.WorkspaceToolResultBytes)
	}
	return event
}

func staticPromptPrefix(prompt string) string {
	marker := "\n---\n**Execution History"
	if idx := strings.Index(prompt, marker); idx >= 0 {
		return prompt[:idx]
	}
	return prompt
}

func staticContextCacheKey(owner *LLMClient, staticContextSHA string, modelOverride ...string) string {
	if owner == nil || staticContextSHA == "" {
		return ""
	}
	model := strings.TrimSpace(owner.model)
	if len(modelOverride) > 0 && strings.TrimSpace(modelOverride[0]) != "" {
		model = strings.TrimSpace(modelOverride[0])
	}
	parts := []string{
		strings.TrimSpace(owner.provider),
		model,
		strings.TrimSpace(owner.baseURL),
		strings.TrimSpace(owner.profile),
		staticContextSHA,
	}
	if len(owner.extra) > 0 {
		keys := make([]string, 0, len(owner.extra))
		for key := range owner.extra {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if value := strings.TrimSpace(owner.extra[key]); value != "" {
				parts = append(parts, key+"="+value)
			}
		}
	}
	return promptSHA256(strings.Join(parts, "\n"))
}

func promptSection(prompt, start string, endMarkers ...string) string {
	startIdx := strings.Index(prompt, start)
	if startIdx < 0 {
		return ""
	}
	sectionStart := startIdx + len(start)
	sectionEnd := len(prompt)
	for _, marker := range endMarkers {
		if marker = strings.TrimSpace(marker); marker == "" {
			continue
		}
		if idx := strings.Index(prompt[sectionStart:], marker); idx >= 0 && sectionStart+idx < sectionEnd {
			sectionEnd = sectionStart + idx
		}
	}
	return prompt[sectionStart:sectionEnd]
}

func firstMetadataLineValue(text, key string) string {
	prefix := key + ":"
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func maxMetadataUint64(text, key string) uint64 {
	var maxValue uint64
	prefix := key + ":"
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		value, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, prefix)), 10, 64)
		if err == nil && value > maxValue {
			maxValue = value
		}
	}

	pattern := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `"\s*:\s*(\d+)`)
	for _, match := range pattern.FindAllStringSubmatch(text, -1) {
		if len(match) < 2 {
			continue
		}
		value, err := strconv.ParseUint(match[1], 10, 64)
		if err == nil && value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}

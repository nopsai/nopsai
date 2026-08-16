package llm

import (
	"bytes"
	"strings"
	"testing"

	appconfig "nopsai/config"
	"nopsai/pkg/proto"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestBuildPromptLogsMetadataWithoutPromptText(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := log.Logger
	previousLevel := zerolog.GlobalLevel()
	t.Cleanup(func() {
		log.Logger = previousLogger
		zerolog.SetGlobalLevel(previousLevel)
	})
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log.Logger = zerolog.New(&logs)

	client := NewLLMClient(appconfig.LLMProviderOpenAI, "", "gpt-test", "", "")
	prompt := client.buildPrompt(&proto.GetActionRequest{
		Goal:    "summarize secret evidence",
		History: "secret-history-value",
		DirectoryListing: map[string]string{
			"secret.txt": "secret-file-content",
		},
	})

	if !strings.Contains(prompt, "secret-file-content") {
		t.Fatal("prompt should still contain request context before provider call")
	}
	output := logs.String()
	for _, want := range []string{
		`"prompt_kind":"action"`,
		`"provider":"openai"`,
		`"model":"gpt-test"`,
		`"prompt_sha256"`,
		`"prompt_bytes"`,
		`"estimated_input_tokens"`,
		`"static_context_sha256"`,
		`"static_context_cache_key"`,
		`"shared_file_count":1`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("log output missing %s:\n%s", want, output)
		}
	}
	for _, forbidden := range []string{"secret-file-content", "secret-history-value", "summarize secret evidence"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("log output leaked prompt text %q:\n%s", forbidden, output)
		}
	}
}

func TestPromptMetadataIncludesStableIdentityAndSize(t *testing.T) {
	client := NewLLMClient(appconfig.LLMProviderAnthropic, "", "claude-test", "", "", "reasoning")
	meta := newPromptMetadata(client, "hello world")

	if meta.Provider != appconfig.LLMProviderAnthropic || meta.Model != "claude-test" || meta.Profile != "reasoning" {
		t.Fatalf("metadata identity = %#v", meta)
	}
	if meta.PromptSHA256 != "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9" {
		t.Fatalf("prompt sha = %q", meta.PromptSHA256)
	}
	if meta.PromptBytes != len("hello world") {
		t.Fatalf("prompt bytes = %d, want %d", meta.PromptBytes, len("hello world"))
	}
	if meta.EstimatedInputTokens <= 0 {
		t.Fatalf("estimated tokens = %d, want positive", meta.EstimatedInputTokens)
	}
	if meta.StaticContextSHA256 != meta.PromptSHA256 {
		t.Fatalf("static context sha = %q, want full prompt sha when no history marker", meta.StaticContextSHA256)
	}
	if meta.StaticContextCacheKey == "" {
		t.Fatal("static context cache key is empty")
	}
}

func TestPromptMetadataExtractsContextRevisionsAndRetrievalStats(t *testing.T) {
	prompt := `NopsAI Governance Contract
knowledge_revision: knowledge123
policy_revision: policy456
effective_policy_snapshot_hash: effective789
governance_level: strict
governance_contract_version: 2026-08-10.v1
---
**Working Directory Contents:**
--- File: README.md ---
NopsAI File Identity:
path: README.md
sha256: abc
size: 5
workspace_revision: 7
--- Content ---
hello
---
**Workspace Tools:**
available
---
**Execution History (Previous Steps):**
history_revision: 12
--- Workspace Tool Results For Current Goal ---
Workspace tool result: tool=read_file arguments={"path":"README.md"} result={"workspace_revision":8,"content":"hello"}
---
**Current Goal:**
"answer"`

	meta := newPromptMetadata(nil, prompt)

	if meta.HistoryRevision != 12 || meta.WorkspaceRevision != 8 {
		t.Fatalf("revisions = history %d workspace %d", meta.HistoryRevision, meta.WorkspaceRevision)
	}
	if meta.KnowledgeRevision != "knowledge123" || meta.PolicyRevision != "policy456" {
		t.Fatalf("knowledge/policy revisions = %q/%q", meta.KnowledgeRevision, meta.PolicyRevision)
	}
	if meta.EffectivePolicySnapshotHash != "effective789" ||
		meta.GovernanceLevel != "strict" ||
		meta.GovernanceContractVersion != "2026-08-10.v1" {
		t.Fatalf("policy metadata = %#v", meta)
	}
	if meta.SharedFileCount != 1 || meta.SharedFileBytes <= 0 {
		t.Fatalf("shared file stats = %d/%d", meta.SharedFileCount, meta.SharedFileBytes)
	}
	if meta.WorkspaceToolCallCount != 1 || meta.WorkspaceToolResultBytes <= 0 {
		t.Fatalf("workspace tool stats = %d/%d", meta.WorkspaceToolCallCount, meta.WorkspaceToolResultBytes)
	}
}

func TestUsageFromTokensIncludesPromptMetadata(t *testing.T) {
	usage := usageFromTokens(appconfig.LLMProviderOpenAI, "gpt-test", "standard", "hello world", "ok", 3, 1, 4)

	if usage.PromptSHA256 != "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9" {
		t.Fatalf("prompt sha = %q", usage.PromptSHA256)
	}
	if usage.PromptBytes != len("hello world") {
		t.Fatalf("prompt bytes = %d, want %d", usage.PromptBytes, len("hello world"))
	}
	if usage.EstimatedInputTokens <= 0 {
		t.Fatalf("estimated input tokens = %d, want positive", usage.EstimatedInputTokens)
	}
	if usage.StaticContextSHA256 == "" || usage.StaticContextCacheKey == "" {
		t.Fatalf("static context metadata missing: %#v", usage)
	}
	if usage.PromptTokens != 3 || usage.CompletionTokens != 1 || usage.TotalTokens != 4 {
		t.Fatalf("provider token counts changed: %#v", usage)
	}
}

func TestUsageFromTokenDetailsForClientUsesTrustBoundaryCacheKey(t *testing.T) {
	clientA := NewLLMClientWithOptions(LLMClientOptions{
		Provider: appconfig.LLMProviderOpenAI,
		Profile:  "standard",
		Model:    "gpt-test",
		BaseURL:  "https://api-a.example.test/v1",
		Extra:    map[string]string{"project": "project-a"},
	})
	clientB := NewLLMClientWithOptions(LLMClientOptions{
		Provider: appconfig.LLMProviderOpenAI,
		Profile:  "standard",
		Model:    "gpt-test",
		BaseURL:  "https://api-b.example.test/v1",
		Extra:    map[string]string{"project": "project-a"},
	})

	usageA := usageFromTokenDetailsForClient(clientA, "gpt-test", "hello world", "ok", 3, 1, 4, 2)
	usageB := usageFromTokenDetailsForClient(clientB, "gpt-test", "hello world", "ok", 3, 1, 4, 2)

	if usageA.StaticContextCacheKey == "" || usageB.StaticContextCacheKey == "" {
		t.Fatalf("cache keys should be populated: %#v %#v", usageA, usageB)
	}
	if usageA.StaticContextCacheKey == usageB.StaticContextCacheKey {
		t.Fatal("cache key did not change across provider base URLs")
	}
	if usageA.CachedInputTokens != 2 {
		t.Fatalf("cached tokens = %d, want 2", usageA.CachedInputTokens)
	}
}

func TestStaticPromptPrefixExcludesExecutionHistoryAndGoal(t *testing.T) {
	prompt := "static tools and policy\n---\n**Execution History (Previous Steps):**\nsecret history\n---\n**Current Goal:**\nsecret goal"
	if got := staticPromptPrefix(prompt); got != "static tools and policy" {
		t.Fatalf("staticPromptPrefix() = %q", got)
	}
}

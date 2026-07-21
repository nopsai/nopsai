package llm

import (
	"context"
	"strings"
	"testing"

	appconfig "nopsai/config"
)

type fakeCompletionProvider struct {
	response string
}

func (p fakeCompletionProvider) Complete(context.Context, string) (string, error) {
	return p.response, nil
}

func (p fakeCompletionProvider) Name() string { return "fake" }

func TestCacheIdentityUsesBoundariesAndFileIdentitiesWithoutRawFileContent(t *testing.T) {
	promptA := `Agent profile

Your task is to choose an action.
---
**Variables:**
- TOKEN: [redacted]
---
**Knowledge Context:**
NopsAI Knowledge Snapshot
knowledge_revision: knowledge123
policy_revision: policy123
effective_policy_snapshot_hash: effective123
policy_merge_mode: restrictive
policy_precedence_version: 2026-07-20.v1
---
**Working Directory Contents:**
--- File: README.md ---
NopsAI File Identity:
path: README.md
sha256: abc
size: 5
workspace_revision: 9
--- Content ---
super secret file body
---
**Workspace Tools:**
tools
---
**External MCP Tools:**
mcp
---
**Execution History (Previous Steps):**
history_revision: 1`
	promptB := strings.Replace(promptA, "super secret file body", "different body", 1)
	client := &LLMClient{
		provider:           appconfig.LLMProviderOpenAI,
		model:              "gpt-test",
		baseURL:            "https://api.example.test/v1",
		profile:            "standard",
		authorizationScope: "team/platform",
		extra:              map[string]string{"project": "project-a"},
	}

	identityA := NewCacheIdentity(client, promptA)
	identityB := NewCacheIdentity(client, promptB)
	encoded := string(identityA.CanonicalBytes())

	if identityA.Hash() == "" || identityA.AuthorizationScopeHash == "" || identityA.EffectivePolicySnapshotHash != "effective123" {
		t.Fatalf("cache identity missing expected fields: %#v", identityA)
	}
	if strings.Contains(encoded, "super secret file body") {
		t.Fatalf("cache identity leaked raw file content: %s", encoded)
	}
	if identityA.Hash() != identityB.Hash() {
		t.Fatalf("cache identity changed after raw file body changed without identity change: %s vs %s", identityA.Hash(), identityB.Hash())
	}
}

func TestCompleteRequestFailsWhenRequiredProviderStateUnsupported(t *testing.T) {
	client := &LLMClient{
		provider:          appconfig.LLMProviderLMStudio,
		model:             "local",
		providerStateMode: ProviderFeatureModeRequired,
		promptCacheMode:   ProviderFeatureModeAuto,
		providerClient:    fakeCompletionProvider{response: "ok"},
	}

	_, err := client.CompleteRequest(t.Context(), CompletionRequest{Prompt: "hello"})
	if err == nil || !strings.Contains(err.Error(), "provider_state") {
		t.Fatalf("CompleteRequest() error = %v, want required provider_state failure", err)
	}
}

func TestCompleteRequestRecordsLogicalSessionMetadata(t *testing.T) {
	client := &LLMClient{
		provider:        appconfig.LLMProviderOpenAI,
		model:           "gpt-test",
		promptCacheMode: ProviderFeatureModeAuto,
		providerClient:  fakeCompletionProvider{response: "ok"},
	}
	collector := NewUsageCollector()
	ctx := ContextWithUsageCollector(t.Context(), collector)
	session := NewGoalSession("")

	resp, err := client.CompleteRequest(ctx, CompletionRequest{Prompt: "hello world", Session: session})
	if err != nil {
		t.Fatalf("CompleteRequest() error = %v", err)
	}
	if resp.Session.SessionID != session.SessionID || resp.ExecutionMode != ExecutionModeStatelessPromptCached {
		t.Fatalf("response metadata = %#v", resp)
	}
	recordUsage(ContextWithCompletionMetadata(ctx, completionMetadata{
		LogicalSessionID:        session.SessionID,
		ExecutionMode:           resp.ExecutionMode,
		CacheIdentitySHA256:     resp.CacheIdentityID,
		PromptSchemaVersion:     CompletionPromptSchemaVersion,
		PromptCacheSupported:    true,
		PromptCacheSupportKnown: true,
	}), Usage{Provider: "openai", Model: "gpt-test", PromptTokens: 3})
	usages := collector.Snapshot()
	if len(usages) != 1 || usages[0].LogicalSessionID != session.SessionID || usages[0].CacheIdentitySHA256 == "" {
		t.Fatalf("usage metadata = %#v", usages)
	}
}

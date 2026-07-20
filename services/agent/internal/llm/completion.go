package llm

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const CompletionPromptSchemaVersion = "nopsai.completion.v1"

type CompletionMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	CacheHint  string `json:"cache_hint,omitempty"`
	Provenance string `json:"provenance,omitempty"`
}

type CompletionRequest struct {
	Feature             string              `json:"feature,omitempty"`
	PromptSchemaVersion string              `json:"prompt_schema_version"`
	Session             GoalSession         `json:"session"`
	Messages            []CompletionMessage `json:"messages,omitempty"`
	Prompt              string              `json:"-"`
	CacheIdentity       CacheIdentity       `json:"cache_identity"`
}

type CompletionResponse struct {
	Text            string      `json:"text"`
	Session         GoalSession `json:"session"`
	ExecutionMode   string      `json:"execution_mode"`
	CacheIdentityID string      `json:"cache_identity_sha256,omitempty"`
}

type GoalSession struct {
	SessionID       string `json:"session_id"`
	ProviderStateID string `json:"provider_state_id,omitempty"`
}

type CacheIdentity struct {
	TenantID                    string `json:"tenant_id,omitempty"`
	AuthorizationScopeHash      string `json:"authorization_scope_hash,omitempty"`
	Provider                    string `json:"provider"`
	Model                       string `json:"model,omitempty"`
	BaseURL                     string `json:"base_url,omitempty"`
	Profile                     string `json:"profile,omitempty"`
	Deployment                  string `json:"deployment,omitempty"`
	PromptSchemaVersion         string `json:"prompt_schema_version"`
	PromptPrecedenceVersion     string `json:"prompt_precedence_version,omitempty"`
	AgentProfileHash            string `json:"agent_profile_hash,omitempty"`
	ToolSchemaHash              string `json:"tool_schema_hash,omitempty"`
	MCPSchemaHash               string `json:"mcp_schema_hash,omitempty"`
	MCPPermissionHash           string `json:"mcp_permission_hash,omitempty"`
	KnowledgeRevision           string `json:"knowledge_revision,omitempty"`
	PolicyRevision              string `json:"policy_revision,omitempty"`
	PolicyMergeMode             string `json:"policy_merge_mode,omitempty"`
	PolicyPrecedenceVersion     string `json:"policy_precedence_version,omitempty"`
	EffectivePolicySnapshotHash string `json:"effective_policy_snapshot_hash,omitempty"`
	VariablesHash               string `json:"variables_hash,omitempty"`
	PipelineFileSetHash         string `json:"pipeline_file_set_hash,omitempty"`
}

type completionMetadata struct {
	LogicalSessionID            string
	ProviderStateID             string
	ProviderStateUsed           bool
	ProviderStateSupported      bool
	ProviderStateSupportKnown   bool
	PromptCacheSupported        bool
	PromptCacheSupportKnown     bool
	PromptCacheMode             string
	ProviderStateMode           string
	ExecutionMode               string
	CacheIdentitySHA256         string
	PromptSchemaVersion         string
	PolicyMergeMode             string
	PolicyPrecedenceVersion     string
	EffectivePolicySnapshotHash string
	StablePrefixTokens          int64
	DynamicContextTokens        int64
}

type completionMetadataContextKey struct{}

func NewGoalSession(providerStateID string) GoalSession {
	sessionID := randomHexID(16)
	if sessionID == "" {
		sessionID = fmt.Sprintf("goal-%d", time.Now().UnixNano())
	}
	return GoalSession{
		SessionID:       sessionID,
		ProviderStateID: strings.TrimSpace(providerStateID),
	}
}

func (s GoalSession) normalized() GoalSession {
	if strings.TrimSpace(s.SessionID) == "" {
		s = NewGoalSession(s.ProviderStateID)
	}
	s.SessionID = strings.TrimSpace(s.SessionID)
	s.ProviderStateID = strings.TrimSpace(s.ProviderStateID)
	return s
}

func (r CompletionRequest) FlattenPrompt() string {
	if strings.TrimSpace(r.Prompt) != "" {
		return r.Prompt
	}
	var builder strings.Builder
	for _, message := range r.Messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "user"
		}
		builder.WriteString(strings.ToUpper(role))
		builder.WriteString(":\n")
		builder.WriteString(strings.TrimSpace(message.Content))
		builder.WriteString("\n\n")
	}
	return strings.TrimSpace(builder.String())
}

func (r CompletionRequest) normalized(owner *LLMClient) CompletionRequest {
	if r.PromptSchemaVersion == "" {
		r.PromptSchemaVersion = CompletionPromptSchemaVersion
	}
	r.Session = r.Session.normalized()
	prompt := r.FlattenPrompt()
	if r.Prompt == "" {
		r.Prompt = prompt
	}
	if r.CacheIdentity.Provider == "" && owner != nil {
		r.CacheIdentity = NewCacheIdentity(owner, prompt)
	}
	if r.CacheIdentity.PromptSchemaVersion == "" {
		r.CacheIdentity.PromptSchemaVersion = r.PromptSchemaVersion
	}
	return r
}

func (c *LLMClient) CompleteRequest(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	if c == nil || c.providerClient == nil {
		return CompletionResponse{}, fmt.Errorf("LLM provider client is not initialized")
	}
	req = req.normalized(c)
	capabilities := CapabilitiesForProvider(c.provider)
	executionMode := ExecutionModeStatelessPrompt
	if capabilities.PromptCacheSupported && req.CacheIdentity.Hash() != "" {
		executionMode = ExecutionModeStatelessPromptCached
	}
	meta := completionMetadataFromRequest(req, capabilities, executionMode)
	if c.promptCacheMode != "" {
		meta.PromptCacheMode = c.promptCacheMode
	}
	if c.providerStateMode != "" {
		meta.ProviderStateMode = c.providerStateMode
	}
	if c.promptCacheMode == ProviderFeatureModeDisabled {
		meta.PromptCacheSupported = false
		meta.ExecutionMode = ExecutionModeStatelessPrompt
	}
	if c.providerStateMode == ProviderFeatureModeRequired && !capabilities.ProviderStateSupported {
		return CompletionResponse{}, fmt.Errorf("provider %q does not support required provider_state mode", c.provider)
	}
	if c.promptCacheMode == ProviderFeatureModeRequired && !capabilities.PromptCacheSupported {
		return CompletionResponse{}, fmt.Errorf("provider %q does not support required prompt_cache mode", c.provider)
	}
	executionMode = meta.ExecutionMode
	callCtx := ContextWithCompletionMetadata(ctx, meta)
	text, err := c.providerClient.Complete(callCtx, req.FlattenPrompt())
	if err != nil {
		return CompletionResponse{}, err
	}
	return CompletionResponse{
		Text:            text,
		Session:         req.Session,
		ExecutionMode:   executionMode,
		CacheIdentityID: req.CacheIdentity.Hash(),
	}, nil
}

func ContextWithCompletionMetadata(ctx context.Context, metadata completionMetadata) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, completionMetadataContextKey{}, metadata)
}

func completionMetadataFromContext(ctx context.Context) (completionMetadata, bool) {
	if ctx == nil {
		return completionMetadata{}, false
	}
	metadata, ok := ctx.Value(completionMetadataContextKey{}).(completionMetadata)
	return metadata, ok
}

func completionMetadataFromRequest(req CompletionRequest, capabilities ProviderCapabilities, executionMode string) completionMetadata {
	prompt := req.FlattenPrompt()
	meta := newPromptMetadata(nil, prompt)
	stableTokens := estimateTokenCount(staticPromptPrefix(prompt))
	totalTokens := estimateTokenCount(prompt)
	dynamicTokens := totalTokens - stableTokens
	if dynamicTokens < 0 {
		dynamicTokens = 0
	}
	return completionMetadata{
		LogicalSessionID:            req.Session.SessionID,
		ProviderStateID:             req.Session.ProviderStateID,
		ProviderStateUsed:           false,
		ProviderStateSupported:      capabilities.ProviderStateSupported,
		ProviderStateSupportKnown:   true,
		PromptCacheSupported:        capabilities.PromptCacheSupported,
		PromptCacheSupportKnown:     true,
		PromptCacheMode:             ProviderFeatureModeAuto,
		ProviderStateMode:           ProviderFeatureModeDisabled,
		ExecutionMode:               executionMode,
		CacheIdentitySHA256:         req.CacheIdentity.Hash(),
		PromptSchemaVersion:         req.PromptSchemaVersion,
		PolicyMergeMode:             meta.PolicyMergeMode,
		PolicyPrecedenceVersion:     meta.PolicyPrecedenceVersion,
		EffectivePolicySnapshotHash: meta.EffectivePolicySnapshotHash,
		StablePrefixTokens:          stableTokens,
		DynamicContextTokens:        dynamicTokens,
	}
}

func NewCacheIdentity(owner *LLMClient, prompt string) CacheIdentity {
	meta := newPromptMetadata(nil, prompt)
	identity := CacheIdentity{
		PromptSchemaVersion:         CompletionPromptSchemaVersion,
		PromptPrecedenceVersion:     CompletionPromptSchemaVersion,
		KnowledgeRevision:           meta.KnowledgeRevision,
		PolicyRevision:              meta.PolicyRevision,
		PolicyMergeMode:             meta.PolicyMergeMode,
		PolicyPrecedenceVersion:     meta.PolicyPrecedenceVersion,
		EffectivePolicySnapshotHash: meta.EffectivePolicySnapshotHash,
		AgentProfileHash:            hashPromptSection(prompt, "", "\n\nYour task"),
		VariablesHash:               hashPromptSection(prompt, "**Variables:**", "---\n**Knowledge Context:**"),
		PipelineFileSetHash:         hashFileIdentities(promptSection(prompt, "**Working Directory Contents:**", "---\n**Workspace Tools:**")),
		ToolSchemaHash:              promptSHA256(strings.TrimSpace(promptSection(prompt, "**Workspace Tools:**", "---\n**External MCP Tools:**")) + "\n" + strings.TrimSpace(promptSection(prompt, "**External MCP Tools:**", "---\n**Execution History"))),
		MCPSchemaHash:               hashPromptSection(prompt, "**External MCP Tools:**", "---\n**Execution History"),
		MCPPermissionHash:           hashPromptSection(prompt, "**External MCP Tools:**", "---\n**Execution History"),
	}
	if owner != nil {
		identity.Provider = strings.TrimSpace(owner.provider)
		identity.Model = strings.TrimSpace(owner.model)
		identity.BaseURL = strings.TrimSpace(owner.baseURL)
		identity.Profile = strings.TrimSpace(owner.profile)
		if strings.TrimSpace(owner.authorizationScope) != "" {
			identity.AuthorizationScopeHash = promptSHA256(strings.TrimSpace(owner.authorizationScope))
		}
		if len(owner.extra) > 0 {
			identity.Deployment = firstNonEmpty(owner.extra["deployment"], owner.extra["azure_deployment"], owner.extra["project"], owner.extra["x_project"])
			identity.TenantID = firstNonEmpty(owner.extra["tenant_id"], owner.extra["organization"], owner.extra["org"])
		}
	}
	return identity
}

func (i CacheIdentity) Hash() string {
	encoded := i.CanonicalBytes()
	if len(encoded) == 0 {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func (i CacheIdentity) CanonicalBytes() []byte {
	encoded, err := json.Marshal(i)
	if err != nil {
		return nil
	}
	return encoded
}

func hashPromptSection(prompt, start, end string) string {
	section := ""
	if strings.TrimSpace(start) == "" {
		section = prompt
		if end = strings.TrimSpace(end); end != "" {
			if idx := strings.Index(section, end); idx >= 0 {
				section = section[:idx]
			}
		}
	} else {
		section = promptSection(prompt, start, end)
	}
	section = strings.TrimSpace(section)
	if section == "" {
		return ""
	}
	return promptSHA256(section)
}

func hashFileIdentities(directorySection string) string {
	lines := []string{}
	for _, line := range strings.Split(directorySection, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "--- File:"):
			lines = append(lines, trimmed)
		case trimmed == "NopsAI File Identity:":
			lines = append(lines, trimmed)
		case strings.HasPrefix(trimmed, "path:"):
			lines = append(lines, trimmed)
		case strings.HasPrefix(trimmed, "sha256:"):
			lines = append(lines, trimmed)
		case strings.HasPrefix(trimmed, "size:"):
			lines = append(lines, trimmed)
		case strings.HasPrefix(trimmed, "workspace_revision:"):
			lines = append(lines, trimmed)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	sort.Strings(lines)
	return promptSHA256(strings.Join(lines, "\n"))
}

func randomHexID(bytesLen int) string {
	if bytesLen <= 0 {
		bytesLen = 16
	}
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

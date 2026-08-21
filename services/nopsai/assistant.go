package nopsai

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"nopsai/config"
)

const (
	assistantRoleUser      = "user"
	assistantRoleAssistant = "assistant"
	assistantRoleSystem    = "system"
)

const assistantExecutionPlanToolName = "nopsai.assistant.execution_plan"

type assistantConversation struct {
	ID                 uuid.UUID                   `json:"id"`
	UserID             string                      `json:"user_id"`
	Title              string                      `json:"title"`
	SelectedLLMProfile string                      `json:"selected_llm_profile"`
	DocsVersion        string                      `json:"docs_version"`
	Scope              string                      `json:"scope"`
	Memory             assistantConversationMemory `json:"memory,omitempty"`
	Messages           []assistantMessage          `json:"messages,omitempty"`
	Usage              assistantConversationUsage  `json:"usage"`
	CreatedAt          time.Time                   `json:"created_at"`
	UpdatedAt          time.Time                   `json:"updated_at"`
}

type assistantMessage struct {
	ID             uuid.UUID               `json:"id"`
	ConversationID uuid.UUID               `json:"conversation_id"`
	Role           string                  `json:"role"`
	Content        string                  `json:"content"`
	ToolCalls      []assistantToolActivity `json:"tool_calls"`
	Usage          assistantMessageUsage   `json:"usage"`
	CreatedAt      time.Time               `json:"created_at"`
}

type assistantMessageUsage struct {
	ContentTokens    int64 `json:"content_tokens"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	Estimated        bool  `json:"estimated"`
	DurationMS       int64 `json:"duration_ms"`
	LLMCalls         int   `json:"llm_calls"`
	// CostUSD is nil when the turn could not be priced, which is not the same
	// as having cost nothing.
	CostUSD *float64 `json:"cost_usd,omitempty"`
}

// assistantConversationUsage reports what a conversation spent.
//
// Token counts are kept for internal use but not serialised; spend is the one
// figure the panel shows.
type assistantConversationUsage struct {
	MessageCount           int     `json:"message_count"`
	ContentTokens          int64   `json:"-"`
	PromptTokens           int64   `json:"-"`
	CompletionTokens       int64   `json:"-"`
	TotalTokens            int64   `json:"-"`
	EstimatedTokenMessages int     `json:"-"`
	SpendUSD               float64 `json:"spend_usd"`
	// UnpricedTurns counts turns whose cost could not be determined, so a
	// conversation is never shown a total that quietly omits part of itself.
	UnpricedTurns int   `json:"unpriced_turns,omitempty"`
	DurationMS    int64 `json:"duration_ms"`
	LLMCalls      int   `json:"llm_calls"`
}

type assistantToolActivity struct {
	Name         string         `json:"name"`
	Input        map[string]any `json:"input,omitempty"`
	Output       map[string]any `json:"output,omitempty"`
	Status       string         `json:"status,omitempty"`
	ResourceURIs []string       `json:"resource_uris,omitempty"`
	Source       string         `json:"source,omitempty"`
	Phase        string         `json:"phase,omitempty"`
	Confidence   string         `json:"confidence,omitempty"`
	Purpose      string         `json:"purpose,omitempty"`
}

type assistantExecutionPlan struct {
	Goal                 string                       `json:"goal"`
	Intent               string                       `json:"intent"`
	Summary              string                       `json:"summary"`
	RequiresConfirmation bool                         `json:"requires_confirmation"`
	Steps                []assistantExecutionPlanStep `json:"steps"`
}

type assistantExecutionPlanStep struct {
	Index      int    `json:"index"`
	Title      string `json:"title"`
	Source     string `json:"source"`
	Phase      string `json:"phase"`
	Confidence string `json:"confidence"`
	Tool       string `json:"tool,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Status     string `json:"status,omitempty"`
}

type assistantConversationMemory struct {
	ConversationID        uuid.UUID      `json:"conversation_id,omitempty"`
	Summary               string         `json:"summary"`
	Entities              map[string]any `json:"entities"`
	OpenTasks             []string       `json:"open_tasks"`
	PreviousProposedFixes []string       `json:"previous_proposed_fixes"`
	SelectedRun           string         `json:"selected_run"`
	SelectedPipeline      string         `json:"selected_pipeline"`
	SelectedScope         string         `json:"selected_scope"`
	SelectedDocsVersion   string         `json:"selected_docs_version"`
	UpdatedAt             time.Time      `json:"updated_at,omitempty"`
}

type assistantConversationsResponse struct {
	Conversations []assistantConversation `json:"conversations"`
}

type assistantCreateConversationRequest struct {
	Title              string `json:"title"`
	SelectedLLMProfile string `json:"selected_llm_profile"`
	DocsVersion        string `json:"docs_version"`
	Scope              string `json:"scope"`
}

type assistantCreateMessageRequest struct {
	Content            string               `json:"content"`
	SelectedLLMProfile string               `json:"selected_llm_profile"`
	PageContext        assistantPageContext `json:"page_context,omitempty"`
}

type assistantMessageResponse struct {
	Conversation assistantConversation `json:"conversation"`
	UserMessage  assistantMessage      `json:"user_message"`
	Reply        assistantMessage      `json:"reply"`
}

type assistantLLMProfilesResponse struct {
	DefaultProfile string                      `json:"default_profile"`
	Profiles       []assistantLLMProfileOption `json:"profiles"`
}

type assistantConfigResponse struct {
	Enabled                   bool                            `json:"enabled"`
	Provider                  string                          `json:"provider,omitempty"`
	Model                     string                          `json:"model,omitempty"`
	DefaultDocsVersion        string                          `json:"default_docs_version"`
	ConversationRetentionDays int                             `json:"conversation_retention_days"`
	MaxInputLogsBytes         int                             `json:"max_input_logs_bytes"`
	MaxConversationTurns      int                             `json:"max_conversation_turns"`
	DocsEnabled               bool                            `json:"docs_enabled"`
	DocsVersionAware          bool                            `json:"docs_version_aware"`
	CredentialConfigured      bool                            `json:"credential_configured"`
	DedicatedProfile          string                          `json:"dedicated_profile,omitempty"`
	Memory                    assistantMemoryConfigResponse   `json:"memory"`
	MCP                       assistantMCPConfigResponse      `json:"mcp"`
	Features                  assistantFeaturesConfigResponse `json:"features"`
	Actions                   assistantActionsConfigResponse  `json:"actions"`
}

type assistantMemoryConfigResponse struct {
	Enabled bool   `json:"enabled"`
	Scope   string `json:"scope"`
}

type assistantMCPConfigResponse struct {
	Enabled bool `json:"enabled"`
}

type assistantFeaturesConfigResponse struct {
	Docs                       bool `json:"docs"`
	PipelineDebugging          bool `json:"pipeline_debugging"`
	ConfigGeneration           bool `json:"config_generation"`
	StatisticsInsights         bool `json:"statistics_insights"`
	MaintenanceRecommendations bool `json:"maintenance_recommendations"`
	CostRecommendations        bool `json:"cost_recommendations"`
	ActionExecution            bool `json:"action_execution"`
}

type assistantActionsConfigResponse struct {
	RequireConfirmation bool `json:"require_confirmation"`
}

type assistantLLMProfileOption struct {
	Name           string `json:"name"`
	Provider       string `json:"provider,omitempty"`
	Model          string `json:"model,omitempty"`
	Status         string `json:"status"`
	Validation     string `json:"validation,omitempty"`
	AllowedInScope bool   `json:"allowed_in_scope"`
	DisabledReason string `json:"disabled_reason,omitempty"`
}

type assistantSummarizeMemoryRequest struct {
	Summary               string         `json:"summary"`
	Entities              map[string]any `json:"entities"`
	OpenTasks             []string       `json:"open_tasks"`
	PreviousProposedFixes []string       `json:"previous_proposed_fixes"`
	SelectedRun           string         `json:"selected_run"`
	SelectedPipeline      string         `json:"selected_pipeline"`
	SelectedScope         string         `json:"selected_scope"`
	SelectedDocsVersion   string         `json:"selected_docs_version"`
}

func (a *App) assistantConfig() config.AssistantConfig {
	if a == nil || a.cfg == nil {
		return config.NormalizeAssistantConfig(config.AssistantConfig{})
	}
	cfg := a.getConfigSnapshot()
	return cfg.EffectiveAssistantConfig()
}

func buildAssistantConfigResponse(cfg config.AssistantConfig) assistantConfigResponse {
	cfg = config.NormalizeAssistantConfig(cfg)
	dedicatedProfile := ""
	if assistantConfigHasDedicatedLLMProfile(cfg) {
		dedicatedProfile = assistantDedicatedLLMProfileName
	}
	return assistantConfigResponse{
		Enabled:                   cfg.Enabled,
		Provider:                  cfg.Provider,
		Model:                     cfg.Model,
		DefaultDocsVersion:        cfg.DefaultDocsVersion,
		ConversationRetentionDays: cfg.ConversationRetentionDays,
		MaxInputLogsBytes:         cfg.MaxInputLogsBytes,
		MaxConversationTurns:      cfg.MaxConversationTurns,
		DocsEnabled:               config.AssistantFeatureFlagEnabled(cfg.DocsEnabled),
		DocsVersionAware:          config.AssistantFeatureFlagEnabled(cfg.DocsVersionAware),
		CredentialConfigured:      strings.TrimSpace(cfg.CredentialRef) != "" || strings.TrimSpace(cfg.LegacyAPIKeySecret) != "",
		DedicatedProfile:          dedicatedProfile,
		Memory: assistantMemoryConfigResponse{
			Enabled: cfg.Memory.Enabled,
			Scope:   cfg.Memory.Scope,
		},
		MCP: assistantMCPConfigResponse{
			Enabled: config.AssistantMCPEnabled(cfg.MCP),
		},
		Features: assistantFeaturesConfigResponse{
			Docs:                       config.AssistantFeatureFlagEnabled(cfg.Features.Docs),
			PipelineDebugging:          config.AssistantFeatureFlagEnabled(cfg.Features.PipelineDebugging),
			ConfigGeneration:           config.AssistantFeatureFlagEnabled(cfg.Features.ConfigGeneration),
			StatisticsInsights:         config.AssistantFeatureFlagEnabled(cfg.Features.StatisticsInsights),
			MaintenanceRecommendations: config.AssistantFeatureFlagEnabled(cfg.Features.MaintenanceRecommendations),
			CostRecommendations:        config.AssistantFeatureFlagEnabled(cfg.Features.CostRecommendations),
			ActionExecution:            config.AssistantFeatureFlagEnabled(cfg.Features.ActionExecution),
		},
		Actions: assistantActionsConfigResponse{
			RequireConfirmation: config.AssistantRequireConfirmation(cfg.Actions),
		},
	}
}

func normalizeAssistantConversationRequest(req assistantCreateConversationRequest, cfg config.AssistantConfig) assistantCreateConversationRequest {
	req.Title = strings.TrimSpace(req.Title)
	req.SelectedLLMProfile = config.NormalizeLLMProfileName(req.SelectedLLMProfile)
	req.DocsVersion = strings.TrimSpace(req.DocsVersion)
	if req.DocsVersion == "" {
		req.DocsVersion = cfg.DefaultDocsVersion
	}
	if req.DocsVersion == "" {
		req.DocsVersion = "auto"
	}
	req.Scope = strings.Trim(strings.TrimSpace(req.Scope), "/")
	return req
}

func normalizeAssistantMessageRequest(req assistantCreateMessageRequest) assistantCreateMessageRequest {
	req.Content = strings.TrimSpace(req.Content)
	req.SelectedLLMProfile = config.NormalizeLLMProfileName(req.SelectedLLMProfile)
	req.PageContext = normalizeAssistantPageContext(req.PageContext)
	return req
}

func normalizeAssistantMemory(memory assistantConversationMemory) assistantConversationMemory {
	memory.Summary = strings.TrimSpace(memory.Summary)
	if memory.Entities == nil {
		memory.Entities = map[string]any{}
	}
	memory.OpenTasks = normalizeAssistantStringList(memory.OpenTasks)
	memory.PreviousProposedFixes = normalizeAssistantStringList(memory.PreviousProposedFixes)
	memory.SelectedRun = strings.TrimSpace(memory.SelectedRun)
	memory.SelectedPipeline = strings.Trim(strings.TrimSpace(memory.SelectedPipeline), "/")
	memory.SelectedScope = strings.Trim(strings.TrimSpace(memory.SelectedScope), "/")
	memory.SelectedDocsVersion = strings.TrimSpace(memory.SelectedDocsVersion)
	return memory
}

func normalizeAssistantStringList(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

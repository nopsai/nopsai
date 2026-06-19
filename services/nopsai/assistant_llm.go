package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/llmclient"
)

const assistantLLMToolName = "nopsai.llm.complete"
const assistantDedicatedLLMProfileName = "assistant"

type assistantLLMSynthesis struct {
	Reply    string
	Activity *assistantToolActivity
}

func (a *App) synthesizeAssistantReplyWithLLM(
	ctx context.Context,
	conversation assistantConversation,
	userContent string,
	selectedProfile string,
	plan assistantTurnPlan,
	toolCalls []assistantToolActivity,
	deterministicReply string,
) assistantLLMSynthesis {
	profileName, profile, ok, reason := a.resolveAssistantLLMProfile(ctx, conversation, selectedProfile)
	if !ok {
		if reason == "" {
			return assistantLLMSynthesis{Reply: deterministicReply}
		}
		return assistantLLMSynthesis{
			Reply: deterministicReply,
			Activity: assistantLLMActivity(profileName, profile, assistantToolStatusError, map[string]any{
				"fallback_reason": reason,
			}),
		}
	}

	apiKey := ""
	if config.LLMProviderRequiresAPIKey(profile.Provider) {
		value, err := a.resolveLLMProfileAPIKey(ctx, profileName, profile)
		if err != nil || strings.TrimSpace(value) == "" {
			return assistantLLMSynthesis{
				Reply: deterministicReply,
				Activity: assistantLLMActivity(profileName, profile, assistantToolStatusError, map[string]any{
					"fallback_reason": fmt.Sprintf("credential %q is unavailable", profile.CredentialRef),
				}),
			}
		}
		apiKey = value
	}

	client := llmclient.New(llmclient.Options{
		Provider:       profile.Provider,
		Profile:        profileName,
		APIKey:         apiKey,
		Model:          profile.Model,
		BaseURL:        config.EffectiveLLMProfileBaseURL(profile),
		Reasoning:      config.EffectiveLLMProfileReasoning(profile),
		TimeoutSeconds: profile.TimeoutSeconds,
		MaxTokens:      profile.MaxTokens,
		Temperature:    profile.Temperature,
		Extra:          cloneStringMap(profile.Extra),
		HTTPClient:     assistantHTTPClient(a),
	})
	prompt := buildAssistantLLMPrompt(conversation, userContent, plan, toolCalls, deterministicReply)
	completion, err := client.Complete(ctx, prompt)
	if err != nil {
		return assistantLLMSynthesis{
			Reply: deterministicReply,
			Activity: assistantLLMActivity(profileName, profile, assistantToolStatusError, map[string]any{
				"fallback_reason": err.Error(),
			}),
		}
	}
	reply := strings.TrimSpace(completion.Text)
	if reply == "" {
		reply = deterministicReply
	}
	return assistantLLMSynthesis{
		Reply: reply,
		Activity: assistantLLMActivity(profileName, profile, assistantToolStatusSuccess, map[string]any{
			"usage": completion.Usage,
		}),
	}
}

func (a *App) resolveAssistantLLMProfile(ctx context.Context, conversation assistantConversation, selectedProfile string) (string, config.LLMProfile, bool, string) {
	profileName := config.NormalizeLLMProfileName(selectedProfile)
	if profileName == "" {
		profileName = config.NormalizeLLMProfileName(conversation.SelectedLLMProfile)
	}
	defaultProfile, profiles := a.assistantLLMProfiles(ctx)
	if len(profiles) == 0 {
		return profileName, config.LLMProfile{}, false, ""
	}
	if profileName == "" {
		profileName = defaultProfile
	}
	if profileName == "" {
		return "", config.LLMProfile{}, false, ""
	}
	profile, ok := profiles[profileName]
	if !ok {
		return profileName, config.LLMProfile{}, false, fmt.Sprintf("LLM profile %q was not found", profileName)
	}
	profile = config.NormalizeLLMProfile(profile)
	if status, message := validateLLMProfileDefinition(profileName, profile); status != "valid" {
		return profileName, profile, false, message
	}
	scope := strings.Trim(strings.TrimSpace(conversation.Memory.SelectedScope), "/")
	if scope == "" {
		scope = strings.Trim(strings.TrimSpace(conversation.Scope), "/")
	}
	if !config.LLMProfileAllowedInScope(profile, scope) {
		return profileName, profile, false, fmt.Sprintf("LLM profile %q is not allowed in scope %q", profileName, scope)
	}
	return profileName, profile, true, ""
}

func (a *App) assistantLLMProfiles(ctx context.Context) (string, map[string]config.LLMProfile) {
	cfg := a.assistantConfig()
	if a != nil && a.db != nil {
		if defaultProfile, profiles, found, err := a.loadLLMProfilesFromDB(ctx); err == nil && found {
			return assistantLLMProfilesWithDedicatedConfig(defaultProfile, profiles, cfg)
		}
	}
	if a == nil || a.cfg == nil {
		return assistantLLMProfilesWithDedicatedConfig("", nil, cfg)
	}
	defaultProfile, profiles := a.llmProfilesSnapshot()
	return assistantLLMProfilesWithDedicatedConfig(defaultProfile, profiles, cfg)
}

func assistantLLMProfilesWithDedicatedConfig(defaultProfile string, profiles map[string]config.LLMProfile, cfg config.AssistantConfig) (string, map[string]config.LLMProfile) {
	profiles = config.NormalizeLLMProfiles(profiles)
	if profiles == nil {
		profiles = map[string]config.LLMProfile{}
	}
	profile, ok := assistantLLMProfileFromConfig(cfg)
	if !ok {
		return defaultProfile, profiles
	}
	profiles[assistantDedicatedLLMProfileName] = profile
	return assistantDedicatedLLMProfileName, profiles
}

func assistantConfigHasDedicatedLLMProfile(cfg config.AssistantConfig) bool {
	_, ok := assistantLLMProfileFromConfig(cfg)
	return ok
}

func assistantLLMProfileFromConfig(cfg config.AssistantConfig) (config.LLMProfile, bool) {
	cfg = config.NormalizeAssistantConfig(cfg)
	if strings.TrimSpace(cfg.Provider) == "" {
		return config.LLMProfile{}, false
	}
	timeoutSeconds := 0
	if timeout, err := time.ParseDuration(cfg.Timeout); err == nil && timeout > 0 {
		timeoutSeconds = int(timeout.Seconds())
	}
	profile := config.NormalizeLLMProfile(config.LLMProfile{
		Provider:           cfg.Provider,
		Model:              cfg.Model,
		BaseURL:            cfg.BaseURL,
		CredentialRef:      cfg.CredentialRef,
		LegacyAPIKeySecret: cfg.LegacyAPIKeySecret,
		TimeoutSeconds:     timeoutSeconds,
	})
	return profile, true
}

func buildAssistantLLMProfilesResponse(defaultProfile string, profiles map[string]config.LLMProfile, scope string) assistantLLMProfilesResponse {
	defaultProfile = config.NormalizeLLMProfileName(defaultProfile)
	scope = strings.Trim(strings.TrimSpace(scope), "/")
	profiles = config.NormalizeLLMProfiles(profiles)

	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	options := make([]assistantLLMProfileOption, 0, len(names))
	for _, name := range names {
		profile := config.NormalizeLLMProfile(profiles[name])
		status, validation := validateLLMProfileDefinition(name, profile)
		allowed := scope == "" || config.LLMProfileAllowedInScope(profile, scope)
		disabledReason := ""
		if status != "valid" {
			disabledReason = validation
		} else if !allowed {
			disabledReason = fmt.Sprintf("LLM profile %q is not allowed in scope %q", name, scope)
		}
		options = append(options, assistantLLMProfileOption{
			Name:           name,
			Provider:       profile.Provider,
			Model:          profile.Model,
			Status:         status,
			Validation:     validation,
			AllowedInScope: allowed,
			DisabledReason: disabledReason,
		})
	}

	return assistantLLMProfilesResponse{
		DefaultProfile: defaultProfile,
		Profiles:       options,
	}
}

func buildAssistantLLMPrompt(
	conversation assistantConversation,
	userContent string,
	plan assistantTurnPlan,
	toolCalls []assistantToolActivity,
	deterministicReply string,
) string {
	payload := map[string]any{
		"user_request":        strings.TrimSpace(userContent),
		"intent":              plan.Intent,
		"conversation_memory": normalizeAssistantMemory(conversation.Memory),
		"tool_calls":          assistantLLMPromptToolCalls(toolCalls),
		"tool_summary":        strings.TrimSpace(deterministicReply),
	}
	raw, _ := json.MarshalIndent(payload, "", "  ")
	return strings.TrimSpace(`You are the Nopsai AI Assistant for an enterprise CI/CD and GitOps platform.

Use only the provided JSON context and tool outputs. Do not invent pipeline runs, permissions, approvals, costs, logs, or applied changes.
Generated pipeline YAML, trigger edits, and schedule edits are proposals only. Never say a change was applied unless the tool output explicitly says it was applied.
Mention denied or unavailable tools plainly when they affect the answer. Keep the answer concise and operational.

Context:
` + string(raw))
}

func assistantLLMPromptToolCalls(toolCalls []assistantToolActivity) []map[string]any {
	items := make([]map[string]any, 0, len(toolCalls))
	for _, call := range toolCalls {
		items = append(items, map[string]any{
			"name":          call.Name,
			"status":        call.Status,
			"resource_uris": call.ResourceURIs,
			"input":         assistantPromptSafeValue(call.Input),
			"output":        assistantPromptSafeValue(call.Output),
		})
	}
	return items
}

func assistantLLMActivity(profileName string, profile config.LLMProfile, status string, output map[string]any) *assistantToolActivity {
	if output == nil {
		output = map[string]any{}
	}
	output["profile"] = profileName
	output["provider"] = profile.Provider
	output["model"] = profile.Model
	return &assistantToolActivity{
		Name: assistantLLMToolName,
		Input: map[string]any{
			"profile":  profileName,
			"provider": profile.Provider,
			"model":    profile.Model,
		},
		Output:       output,
		Status:       status,
		ResourceURIs: []string{"nopsai://system/llm-profiles"},
	}
}

func assistantPromptSafeValue(value any) any {
	switch typed := value.(type) {
	case string:
		return assistantTruncateForPrompt(typed)
	case []string:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, assistantTruncateForPrompt(item))
		}
		return values
	case []any:
		values := make([]any, 0, len(typed))
		for _, item := range typed {
			values = append(values, assistantPromptSafeValue(item))
		}
		return values
	case []map[string]any:
		values := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			mapped, _ := assistantPromptSafeValue(item).(map[string]any)
			values = append(values, mapped)
		}
		return values
	case map[string]any:
		mapped := make(map[string]any, len(typed))
		for key, item := range typed {
			mapped[key] = assistantPromptSafeValue(item)
		}
		return mapped
	default:
		return value
	}
}

func assistantTruncateForPrompt(value string) string {
	value = strings.TrimSpace(value)
	const maxPromptValueLength = 6000
	if len(value) <= maxPromptValueLength {
		return value
	}
	return value[:maxPromptValueLength] + "...[truncated]"
}

func assistantHTTPClient(a *App) *http.Client {
	if a != nil && a.httpClient != nil {
		return a.httpClient
	}
	return http.DefaultClient
}

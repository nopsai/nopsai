package llm

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	appconfig "nopsai/config"
	"nopsai/pkg/models"
)

const llmProfilesRuntimeEnv = "NOPSAI_LLM_PROFILES"

type agentRuntimeLLMProfile struct {
	Provider       string                     `json:"provider"`
	Model          string                     `json:"model,omitempty"`
	BaseURL        string                     `json:"base_url,omitempty"`
	APIKey         string                     `json:"api_key,omitempty"`
	CredentialRef  string                     `json:"credential_ref,omitempty"`
	AllowedScopes  []string                   `json:"allowed_scopes,omitempty"`
	Reasoning      string                     `json:"reasoning,omitempty"`
	Thinking       *bool                      `json:"thinking,omitempty"`
	TimeoutSeconds int                        `json:"timeout_seconds,omitempty"`
	MaxTokens      int                        `json:"max_tokens,omitempty"`
	Temperature    *float64                   `json:"temperature,omitempty"`
	PromptCache    appconfig.LLMFeatureConfig `json:"prompt_cache,omitempty"`
	ProviderState  appconfig.LLMFeatureConfig `json:"provider_state,omitempty"`
	Extra          map[string]string          `json:"extra,omitempty"`
}

type agentRuntimeLLMProfiles struct {
	DefaultProfile string                            `json:"default_profile"`
	Profiles       map[string]agentRuntimeLLMProfile `json:"profiles"`
}

type LLMProfileRegistry struct {
	defaultProfile string
	profiles       map[string]agentRuntimeLLMProfile
	scope          string

	mu      sync.Mutex
	clients map[string]*LLMClient
}

func NewLLMProfileRegistryFromEnv(scope string) (*LLMProfileRegistry, error) {
	raw := strings.TrimSpace(os.Getenv(llmProfilesRuntimeEnv))
	if raw == "" {
		return nil, fmt.Errorf("%s is required", llmProfilesRuntimeEnv)
	}
	payloadBytes, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode LLM profiles: %w", err)
	}
	var payload agentRuntimeLLMProfiles
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("parse LLM profiles: %w", err)
	}
	return newLLMProfileRegistry(payload.DefaultProfile, payload.Profiles, scope)
}

func newLLMProfileRegistry(defaultProfile string, profiles map[string]agentRuntimeLLMProfile, scope string) (*LLMProfileRegistry, error) {
	defaultProfile = strings.TrimSpace(defaultProfile)
	if defaultProfile == "" {
		defaultProfile = appconfig.DefaultLLMProfileName
	}
	if len(profiles) == 0 {
		return nil, fmt.Errorf("no LLM profiles configured")
	}

	normalized := make(map[string]agentRuntimeLLMProfile, len(profiles))
	for name, profile := range profiles {
		profileName := strings.TrimSpace(name)
		if profileName == "" {
			continue
		}
		profile.Provider = appconfig.NormalizeLLMProvider(profile.Provider)
		profile.Model = strings.TrimSpace(profile.Model)
		profile.BaseURL = strings.TrimSpace(profile.BaseURL)
		profile.APIKey = strings.TrimSpace(profile.APIKey)
		profile.CredentialRef = strings.TrimSpace(profile.CredentialRef)
		profile.Reasoning = appconfig.NormalizeLMStudioReasoning(profile.Reasoning)
		profile.PromptCache = appconfig.NormalizeLLMFeatureConfig(profile.PromptCache)
		profile.ProviderState = appconfig.NormalizeLLMFeatureConfig(profile.ProviderState)
		profile.Extra = normalizeRuntimeExtra(profile.Extra)
		if profile.MaxTokens < 0 {
			return nil, fmt.Errorf("LLM profile %q has invalid max_tokens %d", profileName, profile.MaxTokens)
		}
		if profile.Temperature != nil {
			minimum, maximum, supported := appconfig.LLMProviderTemperatureRange(profile.Provider)
			if !supported || *profile.Temperature < minimum || *profile.Temperature > maximum {
				return nil, fmt.Errorf(
					"LLM profile %q has invalid temperature %g for provider %q",
					profileName,
					*profile.Temperature,
					profile.Provider,
				)
			}
		}
		if !appconfig.LLMProviderSupportsGenericReasoning(profile.Provider) &&
			(profile.Reasoning != "" || profile.Thinking != nil) {
			return nil, fmt.Errorf(
				"LLM profile %q provider %q does not support generic reasoning or thinking",
				profileName,
				profile.Provider,
			)
		}
		if appconfig.LLMProviderSupportsGenericReasoning(profile.Provider) &&
			profile.Reasoning == "" && profile.Thinking != nil {
			if *profile.Thinking {
				profile.Reasoning = "on"
			} else {
				profile.Reasoning = "off"
			}
		}
		normalized[profileName] = profile
	}
	if _, ok := normalized[defaultProfile]; !ok {
		return nil, fmt.Errorf("default LLM profile %q is not configured", defaultProfile)
	}

	return &LLMProfileRegistry{
		defaultProfile: defaultProfile,
		profiles:       normalized,
		scope:          strings.Trim(strings.TrimSpace(scope), "/"),
		clients:        map[string]*LLMClient{},
	}, nil
}

func (r *LLMProfileRegistry) DefaultProfileName() string {
	if r == nil {
		return ""
	}
	return r.defaultProfile
}

func (r *LLMProfileRegistry) DefaultProfile() (agentRuntimeLLMProfile, bool) {
	if r == nil {
		return agentRuntimeLLMProfile{}, false
	}
	profile, ok := r.profiles[r.defaultProfile]
	return profile, ok
}

func (r *LLMProfileRegistry) ProfileNameFor(pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task) string {
	if r == nil {
		return ""
	}
	profileName := strings.TrimSpace(r.defaultProfile)
	if pipeline != nil && strings.TrimSpace(pipeline.LLMProfile) != "" {
		profileName = strings.TrimSpace(pipeline.LLMProfile)
	}
	if step != nil && strings.TrimSpace(step.GetLLMProfile()) != "" {
		profileName = strings.TrimSpace(step.GetLLMProfile())
	}
	if task != nil && strings.TrimSpace(task.LLMProfile) != "" {
		profileName = strings.TrimSpace(task.LLMProfile)
	}
	return profileName
}

func (r *LLMProfileRegistry) ClientFor(pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task) (*LLMClient, string, error) {
	if r == nil {
		return nil, "", fmt.Errorf("LLM profile registry is not initialized")
	}
	profileName := r.ProfileNameFor(pipeline, step, task)
	profile, ok := r.profiles[profileName]
	if !ok {
		return nil, profileName, fmt.Errorf("LLM profile %q is not configured", profileName)
	}
	if !agentProfileAllowedInScope(profile.AllowedScopes, r.scope) {
		return nil, profileName, fmt.Errorf("LLM profile %q is not allowed in scope %q", profileName, r.scope)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if client := r.clients[profileName]; client != nil {
		return client, profileName, nil
	}

	client := NewLLMClientWithOptions(LLMClientOptions{
		Provider:           profile.Provider,
		Profile:            profileName,
		APIKey:             profile.APIKey,
		Model:              profile.Model,
		BaseURL:            profile.BaseURL,
		Reasoning:          profile.Reasoning,
		AuthorizationScope: r.scope,
		TimeoutSeconds:     profile.TimeoutSeconds,
		MaxTokens:          profile.MaxTokens,
		Temperature:        profile.Temperature,
		PromptCacheMode:    profile.PromptCache.Mode,
		ProviderStateMode:  profile.ProviderState.Mode,
		Extra:              profile.Extra,
	})
	r.clients[profileName] = client
	return client, profileName, nil
}

func normalizeRuntimeExtra(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key != "" {
			normalized[key] = strings.TrimSpace(value)
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func agentProfileAllowedInScope(allowedScopes []string, scope string) bool {
	if len(allowedScopes) == 0 {
		return true
	}
	scope = strings.Trim(strings.TrimSpace(scope), "/")
	for _, allowed := range allowedScopes {
		if strings.EqualFold(strings.Trim(strings.TrimSpace(allowed), "/"), scope) {
			return true
		}
	}
	return false
}

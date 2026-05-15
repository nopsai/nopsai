package main

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
	Provider      string   `json:"provider"`
	Model         string   `json:"model,omitempty"`
	BaseURL       string   `json:"base_url,omitempty"`
	APIKey        string   `json:"api_key,omitempty"`
	APIKeySecret  string   `json:"api_key_secret,omitempty"`
	AllowedScopes []string `json:"allowed_scopes,omitempty"`
	Reasoning     string   `json:"reasoning,omitempty"`
	Thinking      *bool    `json:"thinking,omitempty"`
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

func NewLLMProfileRegistryFromEnv(
	llmProvider,
	geminiAPIKey,
	geminiModel,
	lmStudioBaseURL,
	lmStudioAPIKey,
	lmStudioModel,
	lmStudioReasoning,
	scope string,
) (*LLMProfileRegistry, error) {
	raw := strings.TrimSpace(os.Getenv(llmProfilesRuntimeEnv))
	if raw != "" {
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

	provider := appconfig.NormalizeLLMProvider(llmProvider)
	defaultProfile := appconfig.DefaultLLMProfileName
	profile := agentRuntimeLLMProfile{Provider: provider}
	switch provider {
	case appconfig.LLMProviderLMStudio:
		profile.Model = strings.TrimSpace(lmStudioModel)
		profile.BaseURL = strings.TrimSpace(lmStudioBaseURL)
		profile.APIKey = strings.TrimSpace(lmStudioAPIKey)
		profile.Reasoning = appconfig.NormalizeLMStudioReasoning(lmStudioReasoning)
	default:
		profile.Provider = appconfig.LLMProviderGemini
		profile.Model = strings.TrimSpace(geminiModel)
		profile.APIKey = strings.TrimSpace(geminiAPIKey)
	}

	return newLLMProfileRegistry(defaultProfile, map[string]agentRuntimeLLMProfile{defaultProfile: profile}, scope)
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
		profile.APIKeySecret = strings.TrimSpace(profile.APIKeySecret)
		profile.Reasoning = appconfig.NormalizeLMStudioReasoning(profile.Reasoning)
		if profile.Reasoning == "" && profile.Thinking != nil {
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

	client := NewLLMClient(profile.Provider, profile.APIKey, profile.Model, profile.BaseURL, profile.Reasoning)
	r.clients[profileName] = client
	return client, profileName, nil
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

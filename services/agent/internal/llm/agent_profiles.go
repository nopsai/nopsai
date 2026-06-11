package llm

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"nopsai/pkg/models"
)

const agentProfilesRuntimeEnv = "NOPSAI_AGENT_PROFILES"

type AgentPromptProfile struct {
	ID           string
	DisplayName  string
	Role         string
	Instructions string
	Source       string
}

type agentRuntimeAgentProfile struct {
	DisplayName  string `json:"display_name,omitempty"`
	Role         string `json:"role,omitempty"`
	Instructions string `json:"instructions"`
	Enabled      bool   `json:"enabled"`
	Source       string `json:"source,omitempty"`
}

type agentRuntimeAgentProfiles struct {
	DefaultProfile string                              `json:"default_profile"`
	Profiles       map[string]agentRuntimeAgentProfile `json:"profiles"`
}

type AgentProfileRegistry struct {
	defaultProfile string
	profiles       map[string]agentRuntimeAgentProfile
}

func NewAgentProfileRegistryFromEnv() (*AgentProfileRegistry, error) {
	raw := strings.TrimSpace(os.Getenv(agentProfilesRuntimeEnv))
	if raw == "" {
		return nil, fmt.Errorf("%s is required", agentProfilesRuntimeEnv)
	}
	payloadBytes, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode agent profiles: %w", err)
	}
	var payload agentRuntimeAgentProfiles
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("parse agent profiles: %w", err)
	}
	return newAgentProfileRegistry(payload.DefaultProfile, payload.Profiles)
}

func newAgentProfileRegistry(defaultProfile string, profiles map[string]agentRuntimeAgentProfile) (*AgentProfileRegistry, error) {
	defaultProfile = strings.TrimSpace(defaultProfile)
	if defaultProfile == "" {
		defaultProfile = models.DefaultAgentProfileID
	}
	if len(profiles) == 0 {
		return nil, fmt.Errorf("no agent profiles configured")
	}
	normalized := make(map[string]agentRuntimeAgentProfile, len(profiles))
	for name, profile := range profiles {
		profileName := strings.TrimSpace(name)
		if profileName == "" {
			continue
		}
		profile.DisplayName = strings.TrimSpace(profile.DisplayName)
		profile.Role = strings.TrimSpace(profile.Role)
		profile.Instructions = strings.TrimSpace(profile.Instructions)
		profile.Source = strings.TrimSpace(profile.Source)
		normalized[profileName] = profile
	}
	defaultDefinition, ok := normalized[defaultProfile]
	if !ok {
		return nil, fmt.Errorf("default agent profile %q is not configured", defaultProfile)
	}
	if !defaultDefinition.Enabled {
		return nil, fmt.Errorf("default agent profile %q is disabled", defaultProfile)
	}
	return &AgentProfileRegistry{defaultProfile: defaultProfile, profiles: normalized}, nil
}

func (r *AgentProfileRegistry) DefaultProfileName() string {
	if r == nil {
		return ""
	}
	return r.defaultProfile
}

func (r *AgentProfileRegistry) ProfileNameFor(pipeline *models.Pipeline, step *models.PipelineStep) string {
	if r == nil {
		return ""
	}
	profileName := strings.TrimSpace(r.defaultProfile)
	if pipeline != nil && strings.TrimSpace(pipeline.AgentProfile) != "" {
		profileName = strings.TrimSpace(pipeline.AgentProfile)
	}
	if step != nil && strings.TrimSpace(step.GetAgentProfile()) != "" {
		profileName = strings.TrimSpace(step.GetAgentProfile())
	}
	return profileName
}

func (r *AgentProfileRegistry) ProfileFor(pipeline *models.Pipeline, step *models.PipelineStep) (AgentPromptProfile, string, error) {
	if r == nil {
		return AgentPromptProfile{}, "", fmt.Errorf("agent profile registry is not initialized")
	}
	profileName := r.ProfileNameFor(pipeline, step)
	profile, ok := r.profiles[profileName]
	if !ok {
		return AgentPromptProfile{}, profileName, fmt.Errorf("agent profile %q is not configured", profileName)
	}
	if !profile.Enabled {
		return AgentPromptProfile{}, profileName, fmt.Errorf("agent profile %q is disabled", profileName)
	}
	return AgentPromptProfile{
		ID:           profileName,
		DisplayName:  profile.DisplayName,
		Role:         profile.Role,
		Instructions: profile.Instructions,
		Source:       profile.Source,
	}, profileName, nil
}

func defaultAgentPromptProfile() AgentPromptProfile {
	return AgentPromptProfile{
		ID:           models.DefaultAgentProfileID,
		Role:         "expert CI/CD automation bot",
		Instructions: "Achieve the user's automation goal while respecting the available tools, context, and execution permissions.",
		Source:       "fallback",
	}
}

func formatAgentPromptProfile(profile AgentPromptProfile) string {
	profile.ID = strings.TrimSpace(profile.ID)
	profile.DisplayName = strings.TrimSpace(profile.DisplayName)
	profile.Role = strings.TrimSpace(profile.Role)
	profile.Instructions = strings.TrimSpace(profile.Instructions)
	if profile.Role == "" {
		profile.Role = models.AgentProfilePromptRole(models.AgentProfile{
			ID:          profile.ID,
			DisplayName: profile.DisplayName,
			Role:        profile.Role,
		})
	}
	if profile.Role == "" && profile.Instructions == "" {
		profile = defaultAgentPromptProfile()
	} else if profile.Role == "" {
		profile.Role = defaultAgentPromptProfile().Role
	}
	var builder strings.Builder
	builder.WriteString("You are ")
	builder.WriteString(profile.Role)
	builder.WriteString(".")
	if profile.Instructions != "" {
		builder.WriteString("\n\n")
		builder.WriteString(profile.Instructions)
	}
	return builder.String()
}

package nopsai

import (
	"context"
	"testing"

	"nopsai/config"
	"nopsai/pkg/models"
)

func TestCloneLLMProfilesLetsTeamOverrideSystemProfile(t *testing.T) {
	system := map[string]config.LLMProfile{
		"review": {Provider: config.LLMProviderLMStudio, BaseURL: "http://system:1234"},
	}
	team := map[string]config.LLMProfile{
		"review": {Provider: config.LLMProviderLMStudio, BaseURL: "http://team:1234"},
	}

	merged := cloneLLMProfiles(system)
	for name, profile := range team {
		merged[name] = config.NormalizeLLMProfile(profile)
	}

	if got := merged["review"].BaseURL; got != "http://team:1234" {
		t.Fatalf("merged review base_url = %q, want team override", got)
	}
	if got := system["review"].BaseURL; got != "http://system:1234" {
		t.Fatalf("system profile mutated to %q", got)
	}
}

func TestCloneMCPProfilesLetsTeamOverrideSystemProfile(t *testing.T) {
	system := map[string]models.MCPProfile{
		"github": {Name: "github", Description: "system", Enabled: true},
	}
	team := map[string]models.MCPProfile{
		"github": {Name: "github", Description: "team", Enabled: true},
	}

	merged := cloneMCPProfiles(system)
	for name, profile := range team {
		merged[name] = models.NormalizeMCPProfile(profile)
	}

	if got := merged["github"].Description; got != "team" {
		t.Fatalf("merged github description = %q, want team override", got)
	}
	if got := system["github"].Description; got != "system" {
		t.Fatalf("system profile mutated to %q", got)
	}
}

func TestValidatePipelineLLMProfilesForTeamFallsBackToGlobalDefaultWhenTeamDefaultUnset(t *testing.T) {
	teamID := 42
	app := &App{cfg: &config.Config{
		LLMDefaultProfile: "standard",
		LLMProfiles: map[string]config.LLMProfile{
			"standard": {
				Provider: config.LLMProviderLMStudio,
				BaseURL:  "http://lmstudio:1234",
			},
		},
	}}
	pipeline := &models.Pipeline{
		Name:           "team-fallback",
		ContainerImage: "ubuntu",
		Steps: []models.PipelineStep{{
			Step: &models.GoalStep{
				BaseStep: models.BaseStep{Name: "review"},
				Goal:     "Review this change.",
			},
		}},
	}

	if err := app.validatePipelineLLMProfilesForTeam(context.Background(), pipeline, "prod", &teamID); err != nil {
		t.Fatalf("validatePipelineLLMProfilesForTeam() error = %v", err)
	}
}

func TestValidatePipelineAgentProfilesForTeamFallsBackToGlobalDefaultWhenTeamDefaultUnset(t *testing.T) {
	teamID := 42
	app := &App{}
	pipeline := &models.Pipeline{
		Name:           "team-agent-fallback",
		ContainerImage: "ubuntu",
		Steps: []models.PipelineStep{{
			Step: &models.GoalStep{
				BaseStep: models.BaseStep{Name: "review"},
				Goal:     "Review this change.",
			},
		}},
	}

	if err := app.validatePipelineAgentProfilesForTeam(context.Background(), pipeline, &teamID); err != nil {
		t.Fatalf("validatePipelineAgentProfilesForTeam() error = %v", err)
	}
}

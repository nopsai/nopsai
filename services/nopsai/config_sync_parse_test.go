package nopsai

import (
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestParseConfigSyncPlanNormalizesTeamResources(t *testing.T) {
	binding := models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
		ScopeID:   "team-1",
		RepoURL:   "https://github.com/acme/platform-config",
		BasePath:  "config",
	}
	repoCtx, err := newConfigSyncRepositoryContext(binding)
	if err != nil {
		t.Fatalf("newConfigSyncRepositoryContext() error = %v", err)
	}

	files := configSyncRepositoryFiles{
		pipelines: map[string]string{
			"config/pipelines/deploy.yaml": `
name: deploy
container_image: alpine:3.20
steps:
  - name: run
    script: echo deploy
`,
		},
		steps: map[string]string{
			"config/steps/setup.yaml": `
name: setup
script: echo setup
`,
		},
		triggers: map[string]string{
			"config/triggers/acme/api.yaml": `
triggers:
  - on: push
    pipelines:
      - deploy
`,
		},
	}

	plan, err := (&App{}).parseConfigSyncPlan(binding, repoCtx, files)
	if err != nil {
		t.Fatalf("parseConfigSyncPlan() error = %v", err)
	}

	pipeline, ok := plan.pipelines["team-1/deploy"]
	if !ok {
		t.Fatalf("pipelines = %#v, want team-1/deploy", plan.pipelines)
	}
	if pipeline.path != "team-1" || pipeline.name != "deploy" || !strings.Contains(pipeline.definition, "echo deploy") {
		t.Fatalf("pipeline = %#v, want normalized team-1/deploy with original definition", pipeline)
	}

	step, ok := plan.steps["team-1/setup"]
	if !ok {
		t.Fatalf("steps = %#v, want team-1/setup", plan.steps)
	}
	if step.path != "team-1" || step.name != "setup" || !strings.Contains(step.definition, "echo setup") {
		t.Fatalf("step = %#v, want normalized team-1/setup with original definition", step)
	}

	trigger, ok := plan.triggers["team-1/acme/api"]
	if !ok {
		t.Fatalf("triggers = %#v, want team-1/acme/api", plan.triggers)
	}
	if !strings.Contains(trigger.definition, "on: push") {
		t.Fatalf("trigger = %#v, want original manifest definition", trigger)
	}
}

func TestParseConfigSyncPlanLoadsTeamAIProfiles(t *testing.T) {
	binding := models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
		ScopeID:   "team-1",
		RepoURL:   "https://github.com/acme/platform-config",
		BasePath:  "config",
	}
	repoCtx, err := newConfigSyncRepositoryContext(binding)
	if err != nil {
		t.Fatalf("newConfigSyncRepositoryContext() error = %v", err)
	}

	files := configSyncRepositoryFiles{
		teamAIProfiles: map[string]string{
			"config/ai-profiles.yaml": `
llm_default_profile: review
llm_profiles:
  - name: review
    provider: openai
    model: gpt-4.1
    credential_ref: ref://secret/llm-openai
agent_default_profile: release-reviewer
agent_profiles:
  - id: release-reviewer
    display_name: Release Reviewer
    instructions: Review release risk.
mcp_profiles:
  - name: readonly-github
    enabled: true
    servers:
      - server: github
        tools: ["*"]
`,
		},
	}

	plan, err := (&App{}).parseConfigSyncPlan(binding, repoCtx, files)
	if err != nil {
		t.Fatalf("parseConfigSyncPlan() error = %v", err)
	}
	if plan.teamAIProfilePlan == nil {
		t.Fatalf("teamAIProfilePlan = nil, want parsed plan")
	}
	if plan.teamAIProfilePlan.teamPath != "team-1" {
		t.Fatalf("team path = %q, want team-1", plan.teamAIProfilePlan.teamPath)
	}
	if plan.teamAIProfilePlan.llmDefaultProfile == nil || *plan.teamAIProfilePlan.llmDefaultProfile != "review" {
		t.Fatalf("llm default = %#v, want review", plan.teamAIProfilePlan.llmDefaultProfile)
	}
	if _, ok := plan.teamAIProfilePlan.llmProfiles["review"]; !ok {
		t.Fatalf("llm profiles = %#v, want review", plan.teamAIProfilePlan.llmProfiles)
	}
	if plan.teamAIProfilePlan.agentDefaultProfile == nil || *plan.teamAIProfilePlan.agentDefaultProfile != "release-reviewer" {
		t.Fatalf("agent default = %#v, want release-reviewer", plan.teamAIProfilePlan.agentDefaultProfile)
	}
	if _, ok := plan.teamAIProfilePlan.mcpProfiles["readonly-github"]; !ok {
		t.Fatalf("mcp profiles = %#v, want readonly-github", plan.teamAIProfilePlan.mcpProfiles)
	}
}

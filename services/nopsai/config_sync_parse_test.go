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
	if trigger.record.TeamPath != "team-1" || trigger.record.Provider != "github" {
		t.Fatalf("trigger metadata = %#v, want team-1 GitHub defaults", trigger.record)
	}
}

func TestParseConfigSyncPlanRejectsInvalidReusableStepYAML(t *testing.T) {
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

	_, err = (&App{}).parseConfigSyncPlan(binding, repoCtx, configSyncRepositoryFiles{
		steps: map[string]string{
			"config/steps/bad-step.yaml": `
name: bad-step
variables:
  BAD/NAME: value
script: echo bad
`,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "BAD/NAME") {
		t.Fatalf("parseConfigSyncPlan() error = %v, want reusable step variable validation error", err)
	}
}

func TestParseConfigSyncPlanTriggerExplicitTeamOverridesRepositoryOwner(t *testing.T) {
	binding := models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
		RepoURL:   "https://github.com/acme/platform-config",
	}
	repoCtx, err := newConfigSyncRepositoryContext(binding)
	if err != nil {
		t.Fatalf("newConfigSyncRepositoryContext() error = %v", err)
	}

	plan, err := (&App{}).parseConfigSyncPlan(binding, repoCtx, configSyncRepositoryFiles{
		triggers: map[string]string{
			"triggers/team-1/service-api.yaml": `
team: black
triggers:
  - on: push
    pipelines:
      - platform/build
`,
		},
	})
	if err != nil {
		t.Fatalf("parseConfigSyncPlan() error = %v", err)
	}
	trigger, ok := plan.triggers["team-1/service-api"]
	if !ok {
		t.Fatalf("triggers = %#v, want team-1/service-api", plan.triggers)
	}
	if trigger.record.TeamPath != "black" {
		t.Fatalf("trigger team = %q, want explicit team black", trigger.record.TeamPath)
	}
	if trigger.record.RepositoryForWebhook != "team-1/service-api" {
		t.Fatalf("RepositoryForWebhook = %q, want team-1/service-api", trigger.record.RepositoryForWebhook)
	}
}

func assertUseGrant(t *testing.T, plan accessSyncPlan, subjectType, subjectID, resourceType, resourceID, action string) {
	t.Helper()

	key := accessGrantPlanKey{
		subjectType:  subjectType,
		subjectID:    subjectID,
		resourceType: resourceType,
		resourceID:   resourceID,
	}
	grant, ok := plan.grants[key]
	if !ok {
		t.Fatalf("missing use grant key %#v in %#v", key, plan.grants)
	}
	if grant.role != customUseGrantRole || len(grant.actions) != 1 || grant.actions[0] != action {
		t.Fatalf("grant = %#v, want %s", grant, action)
	}
}

func TestParseConfigSyncPlanAddsDashboardEmbeddedAccess(t *testing.T) {
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
		dashboards: map[string]string{
			"config/dashboards/ops-dashboard.yaml": `
title: Ops Dashboard
visibility: workspace
access:
  visibility: workspace
  use_access:
    grants:
      - team: data-team
`,
		},
	}

	plan, err := (&App{}).parseConfigSyncPlan(binding, repoCtx, files)
	if err != nil {
		t.Fatalf("parseConfigSyncPlan() error = %v", err)
	}
	resourceKey := resourceAccessPlanKey{resourceType: grantResourceDashboard, resourceID: "team-1/ops-dashboard"}
	access, ok := plan.accessPlan.resourceAccess[resourceKey]
	if !ok {
		t.Fatalf("dashboard resource access = %#v, want key %#v", plan.accessPlan.resourceAccess, resourceKey)
	}
	if !access.visibilitySet || access.visibility != resourceVisibilityWorkspace {
		t.Fatalf("dashboard visibility = (%v, %q), want workspace", access.visibilitySet, access.visibility)
	}
	grantKey := accessGrantPlanKey{
		subjectType:  grantSubjectTeam,
		subjectID:    "data-team",
		resourceType: grantResourceDashboard,
		resourceID:   "team-1/ops-dashboard",
	}
	grant, ok := plan.accessPlan.grants[grantKey]
	if !ok {
		t.Fatalf("dashboard grants = %#v, want key %#v", plan.accessPlan.grants, grantKey)
	}
	if len(grant.actions) != 1 || grant.actions[0] != "dashboard.read" {
		t.Fatalf("dashboard grant actions = %#v, want dashboard.read", grant.actions)
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

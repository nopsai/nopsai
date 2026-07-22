package nopsai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nopsai/pkg/models"
	aaamodel "nopsai/services/aaa/pkg/model"
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

func TestSampleNopsAIPlatformReleaseGitHubTriggerParses(t *testing.T) {
	binding := models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
		RepoURL:   "https://github.com/hosein-yousefii/nopsai-global-config",
	}
	repoCtx, err := newConfigSyncRepositoryContext(binding)
	if err != nil {
		t.Fatalf("newConfigSyncRepositoryContext() error = %v", err)
	}

	sampleRoot := filepath.Join("..", "..", "doc", "sample-config-repo", "global-repo")
	triggerPath := filepath.Join(sampleRoot, "triggers", "hosein-yousefii", "pre-nopsai.yaml")
	triggerRaw, err := os.ReadFile(triggerPath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", triggerPath, err)
	}
	pipelinePath := filepath.Join(sampleRoot, "pipelines", "platform", "prod", "nopsai-platform-release.yaml")
	pipelineRaw, err := os.ReadFile(pipelinePath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", pipelinePath, err)
	}
	scopePath := filepath.Join(sampleRoot, "scopes", "prod", "scope.yaml")
	scopeRaw, err := os.ReadFile(scopePath)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", scopePath, err)
	}

	plan, err := (&App{}).parseConfigSyncPlan(binding, repoCtx, configSyncRepositoryFiles{
		pipelines: map[string]string{
			"pipelines/platform/prod/nopsai-platform-release.yaml": string(pipelineRaw),
		},
		scopes: map[string]string{
			"scopes/prod/scope.yaml": string(scopeRaw),
		},
		triggers: map[string]string{
			"triggers/hosein-yousefii/pre-nopsai.yaml": string(triggerRaw),
		},
	})
	if err != nil {
		t.Fatalf("parseConfigSyncPlan() error = %v", err)
	}
	trigger, ok := plan.triggers["hosein-yousefii/pre-nopsai"]
	if !ok {
		t.Fatalf("triggers = %#v, want hosein-yousefii/pre-nopsai", plan.triggers)
	}
	if trigger.record.Provider != "github" || trigger.record.TeamPath != "platform/prod" || trigger.record.WebhookSourceID != "" {
		t.Fatalf("trigger metadata = %#v, want GitHub App trigger assigned to platform/prod with automatic ingress", trigger.record)
	}
	if trigger.record.Management != repositoryTriggerManagementNopsAI {
		t.Fatalf("Management = %q, want %q", trigger.record.Management, repositoryTriggerManagementNopsAI)
	}
	if trigger.record.RepositoryForWebhook != "hosein-yousefii/pre-nopsai" {
		t.Fatalf("RepositoryForWebhook = %q, want hosein-yousefii/pre-nopsai", trigger.record.RepositoryForWebhook)
	}
	if got := strings.Join(repositoryTriggerScopesFromDefinition(trigger.definition), ","); got != "prod" {
		t.Fatalf("trigger scopes = %q, want prod", got)
	}
	for _, want := range []string{
		"on: push",
		"branches:",
		"- main",
		"platform/prod/nopsai-platform-release",
	} {
		if !strings.Contains(trigger.definition, want) {
			t.Fatalf("trigger definition missing %q in:\n%s", want, trigger.definition)
		}
	}

	assertUseGrant(t, plan.accessPlan, aaamodel.SubjectTypeRepository, "hosein-yousefii/pre-nopsai", grantResourcePipeline, "platform/prod/nopsai-platform-release", "pipeline.use")
	assertUseGrant(t, plan.accessPlan, aaamodel.SubjectTypeRepository, "hosein-yousefii/pre-nopsai", grantResourceScope, "prod", "scope.use")
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

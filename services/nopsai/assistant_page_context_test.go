package nopsai

import (
	"strings"
	"testing"

	"nopsai/config"
)

func TestNormalizeAssistantPageContextBoundsPromptMetadata(t *testing.T) {
	context := normalizeAssistantPageContext(assistantPageContext{
		Title: "  Pipeline runs  ",
		Path:  "pipelineruns//recent/run-1",
		Area:  "Pipeline Runs!",
		Scope: " /prod/api/ ",
		Query: map[string]string{
			"status": " failure ",
			"token":  "secret",
		},
		Params: map[string]string{
			"run_id": " run-1 ",
			"token":  "secret",
		},
	})

	if context.Title != "Pipeline runs" || context.Path != "/pipelineruns/recent/run-1" || context.Area != "pipeline_runs" {
		t.Fatalf("normalized page identity = %#v", context)
	}
	if context.Scope != "prod/api" {
		t.Fatalf("scope = %q, want prod/api", context.Scope)
	}
	if _, ok := context.Query["token"]; ok {
		t.Fatalf("sensitive arbitrary query key survived normalization: %#v", context.Query)
	}
	if _, ok := context.Params["token"]; ok {
		t.Fatalf("sensitive arbitrary param key survived normalization: %#v", context.Params)
	}
	if context.Query["status"] != "failure" || context.Params["run_id"] != "run-1" {
		t.Fatalf("context maps = query %#v params %#v", context.Query, context.Params)
	}
}

func TestAssistantBaseTurnPlanUsesPageContextBeforeMemory(t *testing.T) {
	pageContext := assistantPageContext{
		ResourceType: "pipeline_run",
		ResourceID:   "00000000-0000-0000-0000-000000000123",
		PipelineID:   "platform/deploy",
		Scope:        "platform",
	}
	memory := assistantConversationMemory{
		SelectedRun:      "00000000-0000-0000-0000-000000000999",
		SelectedPipeline: "legacy/build",
		SelectedScope:    "legacy",
	}

	plan := assistantBaseTurnPlanWithPageContext("explain this failure", memory, pageContext)

	if plan.RunID != pageContext.ResourceID {
		t.Fatalf("run id = %q, want page context run", plan.RunID)
	}
	if plan.PipelineID != "platform/deploy" {
		t.Fatalf("pipeline id = %q, want page context pipeline", plan.PipelineID)
	}
	if plan.Scope != "platform" {
		t.Fatalf("scope = %q, want page context scope", plan.Scope)
	}
}

func TestAssistantBaseTurnPlanKeepsExplicitUserTargetsAheadOfPageContext(t *testing.T) {
	pageContext := assistantPageContext{
		ResourceType: "pipeline_run",
		ResourceID:   "00000000-0000-0000-0000-000000000123",
		PipelineID:   "platform/deploy",
		Scope:        "platform",
	}

	plan := assistantBaseTurnPlanWithPageContext(
		"explain run 00000000-0000-0000-0000-000000000456 for pipeline release/api in scope prod",
		assistantConversationMemory{},
		pageContext,
	)

	if plan.RunID != "00000000-0000-0000-0000-000000000456" {
		t.Fatalf("run id = %q, want explicit user run", plan.RunID)
	}
	if plan.PipelineID != "release/api" {
		t.Fatalf("pipeline id = %q, want explicit user pipeline", plan.PipelineID)
	}
	if plan.Scope != "prod" {
		t.Fatalf("scope = %q, want explicit user scope", plan.Scope)
	}
}

func TestAssistantBaseTurnPlanCombinesPipelineNameAndScopeFromPageContext(t *testing.T) {
	pageContext := assistantPageContext{
		ResourceType: "pipeline",
		ResourceName: "deploy-api",
		Scope:        "platform",
	}

	plan := assistantBaseTurnPlanWithPageContext("review this", assistantConversationMemory{}, pageContext)
	if plan.PipelineID != "platform/deploy-api" {
		t.Fatalf("pipeline id = %q, want scope/name from page context", plan.PipelineID)
	}
}

func TestNormalizeAssistantConversationRequestUsesPageContext(t *testing.T) {
	req := normalizeAssistantConversationRequest(assistantCreateConversationRequest{
		PageContext: assistantPageContext{
			ResourceType: "pipeline_run",
			ResourceID:   "00000000-0000-0000-0000-000000000123",
			PipelineID:   "platform/deploy-api",
			Scope:        "platform",
		},
	}, config.AssistantConfig{DefaultDocsVersion: "auto"})

	if req.Scope != "platform" {
		t.Fatalf("scope = %q, want page-context scope", req.Scope)
	}
	if assistantPageContextRunID(req.PageContext) != "00000000-0000-0000-0000-000000000123" {
		t.Fatalf("page context run not normalized: %#v", req.PageContext)
	}
	if assistantPageContextPipelineID(req.PageContext) != "platform/deploy-api" {
		t.Fatalf("page context pipeline not normalized: %#v", req.PageContext)
	}
}

func TestAssistantPromptsIncludePageContext(t *testing.T) {
	pageContext := assistantPageContext{
		Title:        "Pipeline runs",
		Route:        "/pipelineruns/:tab/:run_id",
		ResourceType: "pipeline_run",
		ResourceID:   "00000000-0000-0000-0000-000000000123",
		Scope:        "platform",
		Query:        map[string]string{"status": "failure"},
	}
	plan := assistantBaseTurnPlanWithPageContext("explain this", assistantConversationMemory{}, pageContext)

	synthesisPrompt := buildAssistantLLMPrompt(assistantConversation{}, "explain this", plan, nil, "Run summary")
	if !strings.Contains(synthesisPrompt, `"page_context"`) || !strings.Contains(synthesisPrompt, pageContext.ResourceID) {
		t.Fatalf("synthesis prompt missing page context:\n%s", synthesisPrompt)
	}

	schemaContext := assistantPlannerSchemaContext(assistantConversation{}, "explain this", pageContext)
	if !strings.Contains(schemaContext, pageContext.ResourceID) || !strings.Contains(schemaContext, "pipeline_run") {
		t.Fatalf("schema context missing page context: %q", schemaContext)
	}
}

// The pipeline the user is looking at has to survive planning and argument
// normalization, or an analysis step reaches the tool with no target at all.
func TestAssistantPageContextPipelineReachesAnalysisToolArguments(t *testing.T) {
	pageContext := assistantPageContext{
		Title:        "Pipelines",
		Path:         "/pipelines/nopsai/nopsai-platform-release",
		Route:        "/pipelines/:pipeline_id",
		Area:         "pipelines",
		TeamPath:     "nopsai",
		ResourceType: "pipeline",
		ResourceID:   "nopsai/nopsai-platform-release",
		PipelineID:   "nopsai/nopsai-platform-release",
	}
	base := assistantBaseTurnPlanWithPageContext("make pipeline more efficient and faster", assistantConversationMemory{}, pageContext)

	plan := assistantTurnPlanFromPlannerDecision(base, assistantPlannerDecision{
		Steps: []assistantPlannerStep{{Tool: "nopsai.analyze_pipeline", Reason: "review the selected pipeline"}},
	})
	if len(plan.Steps) != 1 {
		t.Fatalf("plan steps = %#v", plan.Steps)
	}

	args := hostedMCPMonitoringAnalyticsArgs(plan.Steps[0].ToolName, plan.Steps[0].Args)
	path, name := splitPipelineArg(args)
	if path != "nopsai" || name != "nopsai-platform-release" {
		t.Fatalf("analysis tool received (%q, %q) from args %#v", path, name, args)
	}
}

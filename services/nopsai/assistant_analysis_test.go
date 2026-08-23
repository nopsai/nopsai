package nopsai

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"nopsai/services/aaa/pkg/model"
)

func TestAssistantPlannerRoutesTeamHealthToTheAnalysisTool(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("team.read", "team.list", "system.read")}
	content := "how is the platform team doing this month and what should we fix first?"
	plan := assistantBaseTurnPlan(content)

	prompt := app.buildAssistantPlannerPrompt(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		assistantConversation{ID: uuid.New(), DocsVersion: "auto"},
		content,
		plan,
		nil,
		assistantMaxPlanToolCalls,
		1,
	)

	schemaNames := assistantPlannerSchemaToolNamesForTest(t, prompt)
	if !schemaNames["nopsai.analyze_team"] {
		t.Fatalf("team health prompt should ship the team analysis schema: %#v", schemaNames)
	}
	if !strings.Contains(prompt, "nopsai.analyze_team, nopsai.analyze_pipeline, or nopsai.analyze_run") {
		t.Fatal("planner prompt should prefer first-party analysis tools for review questions")
	}
}

func TestAssistantPlannerRoutesPipelineReviewToTheAnalysisTool(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("pipeline.read", "pipeline.list", "pipeline_run.read")}
	content := "review the platform/deploy-api pipeline, it feels slow and flaky"
	plan := assistantBaseTurnPlan(content)

	prompt := app.buildAssistantPlannerPrompt(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		assistantConversation{ID: uuid.New(), DocsVersion: "auto"},
		content,
		plan,
		nil,
		assistantMaxPlanToolCalls,
		1,
	)

	if schemaNames := assistantPlannerSchemaToolNamesForTest(t, prompt); !schemaNames["nopsai.analyze_pipeline"] {
		t.Fatalf("pipeline review prompt should ship the pipeline analysis schema: %#v", schemaNames)
	}
}

func TestAssistantPlannerRoutesPagePipelineOptimizationToAnalysisTool(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("pipeline.read", "pipeline.list", "pipeline_run.read")}
	content := "how to make this more efficient"
	pageContext := assistantPageContext{ResourceType: "pipeline", ResourceID: "nopsai/nopsai-platform-release"}
	plan := assistantBaseTurnPlanWithPageContext(content, assistantConversationMemory{}, pageContext)

	prompt := app.buildAssistantPlannerPrompt(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		assistantConversation{ID: uuid.New(), DocsVersion: "auto"},
		content,
		plan,
		nil,
		assistantMaxPlanToolCalls,
		1,
	)

	schemaNames := assistantPlannerSchemaToolNamesForTest(t, prompt)
	if !schemaNames["nopsai.analyze_pipeline"] {
		t.Fatalf("page pipeline optimization prompt should ship pipeline analysis schema: %#v", schemaNames)
	}
	if !strings.Contains(prompt, "optimization, efficiency") || !strings.Contains(prompt, "nopsai/nopsai-platform-release") {
		t.Fatalf("planner prompt missing optimization guidance or page pipeline context:\n%s", prompt)
	}
}

func TestAssistantPlannerShipsPipelineAnalysisForSelectedPipelineContext(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("pipeline.read", "pipeline.list", "pipeline_run.read")}
	content := "what should I look at next?"
	pageContext := assistantPageContext{ResourceType: "pipeline", ResourceID: "nopsai/nopsai-platform-release"}
	plan := assistantBaseTurnPlanWithPageContext(content, assistantConversationMemory{}, pageContext)

	prompt := app.buildAssistantPlannerPrompt(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		assistantConversation{ID: uuid.New(), DocsVersion: "auto"},
		content,
		plan,
		nil,
		assistantMaxPlanToolCalls,
		1,
	)

	schemaNames := assistantPlannerSchemaToolNamesForTest(t, prompt)
	if !schemaNames["nopsai.analyze_pipeline"] {
		t.Fatalf("selected pipeline context should ship pipeline analysis schema: %#v", schemaNames)
	}
}

func TestAssistantGroundsAmbiguousTeamPlanToActivePipeline(t *testing.T) {
	plan := assistantGroundAmbiguousPlanToPipelineContext(assistantTurnPlan{
		LowerContent: "how to fix queue time",
		PipelineID:   "nopsai/nopsai-platform-release",
		Steps: []assistantPlanStep{{
			ToolName: "nopsai.analyze_team",
			Args:     map[string]any{"team": "Team Root"},
		}},
	})

	if plan.Intent != "pipeline_review" {
		t.Fatalf("intent = %q, want pipeline_review", plan.Intent)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].ToolName != "nopsai.analyze_pipeline" {
		t.Fatalf("steps = %#v, want active pipeline analysis", plan.Steps)
	}
	if got := plan.Steps[0].Args["pipeline"]; got != "nopsai/nopsai-platform-release" {
		t.Fatalf("pipeline arg = %#v, want active pipeline", got)
	}
}

func TestAssistantPlannerFillsMissingPipelineArgFromPageContext(t *testing.T) {
	base := assistantBaseTurnPlanWithPageContext(
		"make the pipeline more efficient and faster",
		assistantConversationMemory{},
		assistantPageContext{ResourceType: "pipeline", ResourceID: "nopsai/nopsai-platform-release"},
	)
	plan := assistantTurnPlanFromPlannerDecision(base, assistantPlannerDecision{
		Steps: []assistantPlannerStep{{
			Tool: "nopsai.analyze_pipeline",
			Args: map[string]any{},
		}},
	})

	if len(plan.Steps) != 1 {
		t.Fatalf("steps = %#v, want one step", plan.Steps)
	}
	if got := plan.Steps[0].Args["pipeline"]; got != "nopsai/nopsai-platform-release" {
		t.Fatalf("pipeline arg = %#v, want page context pipeline", got)
	}
}

func TestAssistantPlannerFillsMonitoringPipelineArgFromPageContext(t *testing.T) {
	base := assistantBaseTurnPlanWithPageContext(
		"show the bottlenecks",
		assistantConversationMemory{},
		assistantPageContext{ResourceType: "pipeline", ResourceID: "nopsai/nopsai-platform-release"},
	)
	plan := assistantTurnPlanFromPlannerDecision(base, assistantPlannerDecision{
		Steps: []assistantPlannerStep{{
			Tool: "nopsai.get_monitoring_step_performance",
			Args: map[string]any{},
		}},
	})

	if got := plan.Steps[0].Args["pipeline"]; got != "nopsai/nopsai-platform-release" {
		t.Fatalf("pipeline arg = %#v, want page context pipeline", got)
	}
}

func TestAssistantPlannerDoesNotOverrideExplicitPipelineArg(t *testing.T) {
	base := assistantBaseTurnPlanWithPageContext(
		"compare with another pipeline",
		assistantConversationMemory{},
		assistantPageContext{ResourceType: "pipeline", ResourceID: "nopsai/nopsai-platform-release"},
	)
	plan := assistantTurnPlanFromPlannerDecision(base, assistantPlannerDecision{
		Steps: []assistantPlannerStep{{
			Tool: "nopsai.analyze_pipeline",
			Args: map[string]any{"pipeline": "workspace/platform/api"},
		}},
	})

	if got := plan.Steps[0].Args["pipeline"]; got != "workspace/platform/api" {
		t.Fatalf("pipeline arg = %#v, want explicit planner arg", got)
	}
}

func TestAssistantKeepsExplicitTeamPlan(t *testing.T) {
	plan := assistantGroundAmbiguousPlanToPipelineContext(assistantTurnPlan{
		LowerContent: "how is this team doing?",
		PipelineID:   "nopsai/nopsai-platform-release",
		Steps: []assistantPlanStep{{
			ToolName: "nopsai.analyze_team",
			Args:     map[string]any{"team": "nopsai"},
		}},
	})

	if len(plan.Steps) != 1 || plan.Steps[0].ToolName != "nopsai.analyze_team" {
		t.Fatalf("explicit team request should keep team analysis: %#v", plan.Steps)
	}
}

// A planner that picks a real, permitted tool we forgot to ship a schema for is
// right; the turn used to die on our omission. It now gets the schema.
func TestAssistantPlannerRepairsMissingSchemaForPermittedTools(t *testing.T) {
	tools := []hostedMCPTool{
		{Name: "nopsai.analyze_team"},
		{Name: "nopsai.get_team"},
	}
	decision := assistantPlannerDecision{Steps: []assistantPlannerStep{
		{Tool: "nopsai.analyze_team"},
		{Tool: "nopsai.get_team"},
		{Tool: "nopsai.analyze_team"},
	}}

	missing := assistantPlannerRepairableSchemaTools(decision, map[string]bool{"nopsai.get_team": true}, tools)
	if len(missing) != 1 || missing[0] != "nopsai.analyze_team" {
		t.Fatalf("repairable tools = %v, want only the permitted tool without a schema", missing)
	}
	if prompt := assistantPlannerSchemaRepairPrompt("BASE", missing); !strings.Contains(prompt, "nopsai.analyze_team") || !strings.HasPrefix(prompt, "BASE") {
		t.Fatalf("repair prompt = %q, want the base prompt plus the added schema names", prompt)
	}
}

func TestAssistantPlannerDoesNotRepairUnavailableOrInventedTools(t *testing.T) {
	tools := []hostedMCPTool{{Name: "nopsai.analyze_team"}}
	decision := assistantPlannerDecision{Steps: []assistantPlannerStep{
		{Tool: "nopsai.drop_database"},
		{Tool: "nopsai.delete_admin_user"},
	}}

	if missing := assistantPlannerRepairableSchemaTools(decision, map[string]bool{}, tools); len(missing) != 0 {
		t.Fatalf("repairable tools = %v, want none for tools the subject cannot call", missing)
	}
}

// Schema selection withholds proposal and mutation tools on purpose. Repair
// exists to fix our routing misses, not to hand back a guardrail.
func TestAssistantPlannerDoesNotRepairProposalOrMutatingTools(t *testing.T) {
	tools := []hostedMCPTool{
		{Name: "nopsai.propose_secret_gitops_write", Action: "secret.write_value"},
		{Name: "nopsai.update_runner_dispatch", Action: "system.update", InputSchema: objectSchema(map[string]any{"confirm": booleanSchema()})},
		{Name: "nopsai.get_monitoring_summary", Action: "system.read"},
	}
	decision := assistantPlannerDecision{Steps: []assistantPlannerStep{
		{Tool: "nopsai.propose_secret_gitops_write"},
		{Tool: "nopsai.update_runner_dispatch"},
		{Tool: "nopsai.get_monitoring_summary"},
	}}

	missing := assistantPlannerRepairableSchemaTools(decision, map[string]bool{}, tools)
	if len(missing) != 1 || missing[0] != "nopsai.get_monitoring_summary" {
		t.Fatalf("repairable tools = %v, want only the read tool", missing)
	}
}

func TestComposeAnalysisReplyRanksFindingsAndOffersTheNextStep(t *testing.T) {
	reply := composeAssistantReply(
		assistantTurnPlan{Intent: "llm_planned", Goal: "review the team"},
		"assistant",
		[]assistantToolActivity{{
			Name:   "nopsai.analyze_team",
			Status: assistantToolStatusSuccess,
			Output: analysisTestTeamOutput(),
		}},
	)

	for _, want := range []string{
		"Platform scores 60/100",
		"## Findings",
		"1. **45% of runs failed in the last 30 days** (critical · reliability)",
		"- Failure rate: 45%",
		"Recommended: Fix the pipelines that fail most first.",
		"## Next step",
		"`nopsai.analyze_pipeline`",
		"Data source: NopsAI monitoring evidence via `nopsai.analyze_team`",
		"deterministic, not model-derived",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("analysis reply missing %q:\n%s", want, reply)
		}
	}
}

func TestComposeAnalysisReplyLeadsWithTheRunDiagnosis(t *testing.T) {
	reply := composeAssistantReply(
		assistantTurnPlan{Intent: "llm_planned", Goal: "explain the failure"},
		"assistant",
		[]assistantToolActivity{{
			Name:   "nopsai.analyze_run",
			Status: assistantToolStatusSuccess,
			Output: analyzeRunEvidence(
				analysisSubject{Type: "run", ID: "run-9", Label: "platform/deploy-api"},
				analysisWindow{Days: 30},
				analysisTestFailedRunEvidence(),
			),
		}},
	)

	for _, want := range []string{
		"Run platform/deploy-api failed. Likely domain: Application tests",
		"First failure point: test / unit",
		"- Exit code: 1",
		"`nopsai.get_pipeline_run_logs`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("run analysis reply missing %q:\n%s", want, reply)
		}
	}
}

func TestComposeAnalysisReplyReadsEvidenceStoredAsJSON(t *testing.T) {
	// A conversation reloaded from the database carries []any rather than
	// []map[string]any, and the same reply has to come out of both.
	output := analysisTestTeamOutput()
	output["findings"] = []any{
		map[string]any{"title": "45% of runs failed in the last 30 days", "severity": "critical", "category": "reliability"},
	}
	output["next_actions"] = []any{
		map[string]any{"label": "Analyse platform/deploy-api", "tool": "nopsai.analyze_pipeline"},
	}

	reply := composeAnalysisReply([]assistantToolActivity{{
		Name:   "nopsai.analyze_team",
		Status: assistantToolStatusSuccess,
		Output: output,
	}})

	if !strings.Contains(reply, "45% of runs failed") || !strings.Contains(reply, "nopsai.analyze_pipeline") {
		t.Fatalf("reply did not read JSON-shaped evidence:\n%s", reply)
	}
}

func TestComposeAnalysisReplyExplainsAnUnresolvedTeam(t *testing.T) {
	reply := composeAnalysisReply([]assistantToolActivity{{
		Name:   "nopsai.analyze_team",
		Status: assistantToolStatusSuccess,
		Output: map[string]any{
			"ok":    false,
			"error": "no visible team matches \"platfrom\"",
			"available_teams": []map[string]any{
				{"id": "7", "path": "/platform", "label": "Platform"},
			},
		},
	}})

	if !strings.Contains(reply, "no visible team matches") {
		t.Fatalf("reply should name the resolution failure:\n%s", reply)
	}
	if !strings.Contains(reply, "Platform /platform") {
		t.Fatalf("reply should list the teams the user can analyse instead:\n%s", reply)
	}
}

func TestComposeAnalysisReplyIsSkippedWithoutASuccessfulAnalysis(t *testing.T) {
	if reply := composeAnalysisReply([]assistantToolActivity{{
		Name:   "nopsai.analyze_team",
		Status: assistantToolStatusError,
		Output: analysisTestTeamOutput(),
	}}); reply != "" {
		t.Fatalf("failed analysis must not compose a reply, got:\n%s", reply)
	}
}

func analysisTestTeamOutput() map[string]any {
	return map[string]any{
		"analysis":     "team",
		"summary":      "Platform scores 60/100 over the last 30 days: 3 findings, 1 critical or high.",
		"health_score": 60,
		"window":       map[string]any{"from": "2026-07-22T00:00:00Z", "to": "2026-08-21T00:00:00Z", "days": 30},
		"findings": []map[string]any{
			{
				"title":    "45% of runs failed in the last 30 days",
				"severity": "critical",
				"category": "reliability",
				"summary":  "A failure rate at this level means the pipeline result is not trustworthy.",
				"evidence": []map[string]any{
					{"label": "Failure rate", "value": "45%", "kind": "metric"},
					{"label": "Runs", "value": "18 failed of 40", "kind": "metric"},
				},
				"recommendations": []map[string]any{
					{"title": "Fix the pipelines that fail most first", "detail": "Rank failures by pipeline rather than by run."},
				},
			},
		},
		"next_actions": []map[string]any{
			{"label": "Analyse platform/deploy-api, the least reliable pipeline in this window", "tool": "nopsai.analyze_pipeline", "args": map[string]any{"pipeline": "platform/deploy-api"}},
		},
		"limitations": []string{"Only evidence the current user is allowed to read contributes to the findings."},
	}
}

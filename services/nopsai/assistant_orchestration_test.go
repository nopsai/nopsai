package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"nopsai/config"
	"nopsai/pkg/proto"
	"nopsai/services/aaa/pkg/model"
)

func TestAssistantOrchestrationGeneratesAndValidatesPipelineProposal(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("pipeline.create", "pipeline.read")}
	conversation := assistantConversation{
		ID:          uuid.New(),
		DocsVersion: "auto",
		Scope:       "platform/dev",
		Memory:      assistantConversationMemory{SelectedScope: "platform/dev"},
	}

	result := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		conversation,
		"Generate pipeline YAML named deploy-web to build and deploy the web service",
		"standard",
	)

	if len(result.ToolCalls) != 2 {
		t.Fatalf("tool calls len = %d, want 2: %#v", len(result.ToolCalls), result.ToolCalls)
	}
	if result.ToolCalls[0].Name != "nopsai.generate_pipeline" || result.ToolCalls[1].Name != "nopsai.validate_pipeline" {
		t.Fatalf("unexpected tool order: %#v", result.ToolCalls)
	}
	if result.ToolCalls[0].Status != assistantToolStatusSuccess {
		t.Fatalf("generate status = %q", result.ToolCalls[0].Status)
	}
	if yaml := assistantOutputString(result.ToolCalls[0].Output, "yaml"); !strings.Contains(yaml, "name: deploy-web") {
		t.Fatalf("generated yaml missing pipeline name: %q", yaml)
	}
	if !strings.Contains(result.Reply, "No changes were applied") {
		t.Fatalf("reply does not preserve proposal-only contract: %q", result.Reply)
	}
	if result.Memory.SelectedPipeline != "deploy-web" {
		t.Fatalf("selected pipeline = %q, want deploy-web", result.Memory.SelectedPipeline)
	}
	if len(result.Memory.PreviousProposedFixes) == 0 {
		t.Fatalf("previous proposed fixes not updated: %#v", result.Memory)
	}
}

func TestAssistantOrchestrationValidatesPastedPipelineYAML(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("pipeline.read")}
	conversation := assistantConversation{ID: uuid.New(), DocsVersion: "auto"}

	result := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		conversation,
		"Validate this:\n```yaml\nname: bad pipeline\nsteps: []\n```",
		"",
	)

	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool calls len = %d, want 1: %#v", len(result.ToolCalls), result.ToolCalls)
	}
	call := result.ToolCalls[0]
	if call.Name != "nopsai.validate_pipeline" {
		t.Fatalf("tool name = %q, want validate", call.Name)
	}
	if call.Status != assistantToolStatusSuccess {
		t.Fatalf("validate status = %q", call.Status)
	}
	if call.Output["valid"] != false {
		t.Fatalf("valid = %#v, want false", call.Output["valid"])
	}
	if !strings.Contains(result.Reply, "Validation failed") {
		t.Fatalf("reply = %q, want validation failure", result.Reply)
	}
}

func TestAssistantOrchestrationPreparesPipelineGitOpsWritePlan(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("pipeline.create")}
	conversation := assistantConversation{ID: uuid.New(), DocsVersion: "auto"}

	result := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		conversation,
		"Write this pipeline through GitOps:\n```yaml\nname: deploy-web\ncontainer_image: alpine:3.20\nsteps:\n  - name: plan\n    tasks:\n      - name: draft\n        goal: Draft deployment\n```",
		"",
	)

	if len(result.ToolCalls) != 1 {
		t.Fatalf("tool calls len = %d, want 1: %#v", len(result.ToolCalls), result.ToolCalls)
	}
	call := result.ToolCalls[0]
	if call.Name != "nopsai.propose_pipeline_create" {
		t.Fatalf("tool name = %q, want propose create", call.Name)
	}
	if call.Status != assistantToolStatusSuccess {
		t.Fatalf("tool status = %q, output = %#v", call.Status, call.Output)
	}
	if assistantOutputString(call.Output, "pipeline_id") != "deploy-web" {
		t.Fatalf("pipeline id output = %#v", call.Output)
	}
	if result.Memory.SelectedPipeline != "deploy-web" {
		t.Fatalf("selected pipeline = %q, want deploy-web", result.Memory.SelectedPipeline)
	}
	if !strings.Contains(result.Reply, "GitOps-ready pipeline write plan") || !strings.Contains(result.Reply, "No changes were applied") {
		t.Fatalf("reply does not explain write-plan contract: %q", result.Reply)
	}
}

func TestAssistantFeatureFlagsGateHostedMCPTools(t *testing.T) {
	disabled := false
	app := &App{
		cfg: &config.Config{Assistant: config.AssistantConfig{
			Enabled: true,
			Features: config.AssistantFeaturesConfig{
				PipelineDebugging: &disabled,
				ActionExecution:   &disabled,
			},
		}},
		aaaLocal: allowActionsForAssistantTest("pipeline_run.read_logs", "pipeline.execute", "pipeline.create"),
	}

	tools := app.hostedMCPToolsForSubject(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"})
	names := assistantToolNamesForTest(tools)

	if names["nopsai.analyze_pipeline_run_failure"] {
		t.Fatalf("run-analysis tool should be hidden when pipeline_debugging is disabled")
	}
	if names["nopsai.run_pipeline"] {
		t.Fatalf("execution tool should be hidden when action_execution is disabled")
	}
	if !names["nopsai.propose_pipeline_create"] {
		t.Fatalf("proposal-only config generation should remain available")
	}
}

func TestAssistantHostedMCPToolUsesJSONRPCProcessor(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("pipeline.create")}
	result, err := app.callAssistantHostedMCPTool(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		uuid.New(),
		"nopsai.generate_pipeline",
		map[string]any{"name": "deploy-web", "goal": "build and deploy"},
	)
	if err != nil {
		t.Fatalf("callAssistantHostedMCPTool() error = %v", err)
	}
	if yaml := assistantOutputString(result, "yaml"); !strings.Contains(yaml, "name: deploy-web") {
		t.Fatalf("structured MCP result missing generated yaml: %#v", result)
	}
}

func TestAssistantHostedMCPToolRespectsMCPEnabledFlag(t *testing.T) {
	disabled := false
	app := &App{
		cfg:      &config.Config{Assistant: config.AssistantConfig{MCP: config.AssistantMCPConfig{Enabled: &disabled}}},
		aaaLocal: allowActionsForAssistantTest("pipeline.create"),
	}

	call := app.runAssistantHostedMCPTool(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		uuid.New(),
		"nopsai.generate_pipeline",
		map[string]any{"name": "deploy-web"},
	)
	if call.Status != assistantToolStatusDenied || !strings.Contains(assistantOutputString(call.Output, "error"), "disabled") {
		t.Fatalf("call = %#v, want disabled denial", call)
	}
}

func TestAssistantPlanUsesRememberedRunID(t *testing.T) {
	runID := uuid.NewString()
	plan := assistantPlanFromMessage("Why did it fail?", assistantConversationMemory{SelectedRun: runID})
	if plan.Intent != "analyze_run" {
		t.Fatalf("intent = %q, want analyze_run", plan.Intent)
	}
	if plan.RunID != runID {
		t.Fatalf("run id = %q, want %q", plan.RunID, runID)
	}
}

func TestAssistantPlanDetectsPipelineSearch(t *testing.T) {
	plan := assistantPlanFromMessage("Search pipelines for deploy-api", assistantConversationMemory{})
	if plan.Intent != "search_pipelines" {
		t.Fatalf("intent = %q, want search_pipelines", plan.Intent)
	}
	if plan.SearchQuery != "deploy-api" {
		t.Fatalf("search query = %q, want deploy-api", plan.SearchQuery)
	}
}

func TestAssistantPlanRecognizesReportedMCPChatPhrases(t *testing.T) {
	tests := []struct {
		name           string
		message        string
		wantIntent     string
		wantQuery      string
		wantArea       string
		wantPipelineID string
	}{
		{
			name:       "feature discovery",
			message:    "What features can I use with the assistant right now?",
			wantIntent: "feature_capabilities",
		},
		{
			name:       "pipelineruns alias",
			message:    "list of pipelineruns",
			wantIntent: "list_runs",
		},
		{
			name:           "approval pipeline search",
			message:        "give me a pipeline that has approval step",
			wantIntent:     "search_pipelines",
			wantQuery:      "approval",
			wantPipelineID: "",
		},
		{
			name:       "env policy",
			message:    "do we have any policy to prevent showing envs?",
			wantIntent: "feature_capabilities",
			wantQuery:  "secret",
			wantArea:   "secrets",
		},
		{
			name:       "repetitive variables across scopes",
			message:    "how many repetitive variables are used in all scopes?",
			wantIntent: "variable_usage",
		},
		{
			name:       "highest llm token pipeline",
			message:    "which pipeline use the highest LLM tokens?",
			wantIntent: "ai_token_usage",
		},
		{
			name:       "plural llm usage",
			message:    "give me llm usages",
			wantIntent: "ai_token_usage",
		},
		{
			name:       "ambiguous usage asks for target",
			message:    "show usage",
			wantIntent: "clarify",
		},
		{
			name:       "scope secret counts inventory",
			message:    "how many scope do we have and for each how many secrets",
			wantIntent: "scope_secret_summary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := assistantPlanFromMessage(tt.message, assistantConversationMemory{})
			if plan.Intent != tt.wantIntent {
				t.Fatalf("intent = %q, want %q", plan.Intent, tt.wantIntent)
			}
			if plan.SearchQuery != tt.wantQuery {
				t.Fatalf("search query = %q, want %q", plan.SearchQuery, tt.wantQuery)
			}
			if plan.CapabilityArea != tt.wantArea {
				t.Fatalf("capability area = %q, want %q", plan.CapabilityArea, tt.wantArea)
			}
			if plan.PipelineID != tt.wantPipelineID {
				t.Fatalf("pipeline id = %q, want %q", plan.PipelineID, tt.wantPipelineID)
			}
		})
	}
}

func TestAssistantFeaturePlannerRoutesNopsAIFeatureSurface(t *testing.T) {
	runID := "11111111-1111-1111-1111-111111111111"
	tests := []struct {
		message string
		tool    string
		assert  func(t *testing.T, plan assistantTurnPlan)
	}{
		{message: "show setup status", tool: "nopsai.get_setup_status"},
		{
			message: "check config repo drift for folder platform/dev",
			tool:    "nopsai.get_config_repo_drift",
			assert: func(t *testing.T, plan assistantTurnPlan) {
				if plan.Steps[0].Args["folder_id"] != "platform/dev" {
					t.Fatalf("folder_id = %#v, want platform/dev", plan.Steps[0].Args["folder_id"])
				}
			},
		},
		{message: "show notification mail settings", tool: "nopsai.get_notification_mail_settings"},
		{message: "list credentials metadata", tool: "nopsai.list_credentials_metadata"},
		{
			message: "create data backup confirmed",
			tool:    "nopsai.create_data_backup",
			assert: func(t *testing.T, plan assistantTurnPlan) {
				if plan.Steps[0].Args["confirm"] != true {
					t.Fatalf("confirm = %#v, want true", plan.Steps[0].Args["confirm"])
				}
			},
		},
		{message: "show access grants", tool: "nopsai.list_access_grants"},
		{
			message: "generate kubernetes runner manifest for runner runner-a namespace nopsai",
			tool:    "nopsai.generate_kubernetes_runner_manifest",
			assert: func(t *testing.T, plan assistantTurnPlan) {
				if plan.Steps[0].Args["runner_id"] != "runner-a" || plan.Steps[0].Args["namespace"] != "nopsai" {
					t.Fatalf("runner manifest args = %#v", plan.Steps[0].Args)
				}
			},
		},
		{
			message: "list git webhook deliveries source github-main",
			tool:    "nopsai.list_git_webhook_deliveries",
			assert: func(t *testing.T, plan assistantTurnPlan) {
				if plan.Steps[0].Args["source_id"] != "github-main" {
					t.Fatalf("source_id = %#v, want github-main", plan.Steps[0].Args["source_id"])
				}
			},
		},
		{
			message: "list external trigger invocations trigger deploy-hook",
			tool:    "nopsai.list_external_trigger_invocations",
			assert: func(t *testing.T, plan assistantTurnPlan) {
				if plan.Steps[0].Args["trigger_id"] != "deploy-hook" {
					t.Fatalf("trigger_id = %#v, want deploy-hook", plan.Steps[0].Args["trigger_id"])
				}
			},
		},
		{message: "show monitoring recommendations", tool: "nopsai.list_monitoring_recommendations"},
		{
			message: "what should the UI request from MCP for monitoring",
			tool:    "nopsai.get_ui_context",
			assert: func(t *testing.T, plan assistantTurnPlan) {
				if plan.Steps[0].Args["area"] != "monitoring" {
					t.Fatalf("area = %#v, want monitoring", plan.Steps[0].Args["area"])
				}
			},
		},
		{
			message: "cancel run " + runID + " confirmed",
			tool:    "nopsai.cancel_pipeline_run",
			assert: func(t *testing.T, plan assistantTurnPlan) {
				if plan.Intent != "feature_tool" || plan.Steps[0].Args["run_id"] != runID || plan.Steps[0].Args["confirm"] != true {
					t.Fatalf("cancel run plan = %#v", plan)
				}
			},
		},
		{message: "run nopsai.list_data_backups", tool: "nopsai.list_data_backups"},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			plan := assistantPlanFromMessage(tt.message, assistantConversationMemory{})
			if plan.Intent != "feature_tool" {
				t.Fatalf("intent = %q, want feature_tool: %#v", plan.Intent, plan)
			}
			if len(plan.Steps) != 1 || plan.Steps[0].ToolName != tt.tool {
				t.Fatalf("steps = %#v, want %s", plan.Steps, tt.tool)
			}
			if tt.assert != nil {
				tt.assert(t, plan)
			}
		})
	}
}

func TestAssistantFeaturePlannerKeepsAllFeatureCoverageQuestionOnCapabilities(t *testing.T) {
	plan := assistantPlanFromMessage("it should support all nopsai features", assistantConversationMemory{})
	if plan.Intent != "feature_capabilities" {
		t.Fatalf("intent = %q, want feature_capabilities", plan.Intent)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].ToolName != "nopsai.get_feature_capabilities" {
		t.Fatalf("steps = %#v, want feature capabilities", plan.Steps)
	}
}

func TestAssistantFeaturePlanValidationBlocksUnconfirmedMutation(t *testing.T) {
	enabled := true
	app := &App{
		cfg: &config.Config{Assistant: config.AssistantConfig{
			Features: config.AssistantFeaturesConfig{ActionExecution: &enabled},
		}},
		aaaLocal: allowActionsForAssistantTest("system.update"),
	}
	plan := assistantPlanFromMessage("create data backup", assistantConversationMemory{})
	if plan.Intent != "feature_tool" || len(plan.Steps) != 1 {
		t.Fatalf("plan = %#v, want feature tool", plan)
	}

	err := app.validateAssistantToolPlan(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		plan,
	)
	if err == nil || !strings.Contains(err.Error(), "without confirm:true") {
		t.Fatalf("err = %v, want confirmation validation", err)
	}
}

func TestAssistantFeatureReplySummarizesProposalWithoutSensitiveContent(t *testing.T) {
	reply := composeFeatureToolReply([]assistantToolActivity{{
		Name:   "nopsai.propose_secret_gitops_write",
		Status: assistantToolStatusSuccess,
		Output: map[string]any{
			"proposal_type": "secrets_gitops_write",
			"applies":       false,
			"gitops": map[string]any{
				"files": []map[string]any{{
					"path":    "config-repositories/secrets/platform.yaml",
					"content": "encrypted_value: should-not-leak",
				}},
			},
		},
	}})

	if !strings.Contains(reply, "Prepared proposal") || !strings.Contains(reply, "config-repositories/secrets/platform.yaml") {
		t.Fatalf("reply = %q, want proposal and file path", reply)
	}
	if strings.Contains(reply, "should-not-leak") {
		t.Fatalf("reply leaked file content: %q", reply)
	}
}

func TestAssistantFeatureReplySurfacesConfirmationRequired(t *testing.T) {
	reply := composeFeatureToolReply([]assistantToolActivity{{
		Name:   "nopsai.create_data_backup",
		Status: assistantToolStatusSuccess,
		Output: map[string]any{
			"method":                "POST",
			"path":                  "/v1/system/data/backups",
			"requires_confirmation": true,
			"applied":               false,
		},
	}})

	if !strings.Contains(reply, "Confirmation required") || !strings.Contains(reply, "Applied: false") {
		t.Fatalf("reply = %q, want confirmation required", reply)
	}
}

func TestAssistantPlanDoesNotTreatQuestionGrammarAsScope(t *testing.T) {
	plan := assistantPlanFromMessage("how many scope do we have and for each how many secrets", assistantConversationMemory{})
	if plan.Intent != "scope_secret_summary" {
		t.Fatalf("intent = %q, want scope_secret_summary", plan.Intent)
	}
	if plan.Scope != "" {
		t.Fatalf("scope = %q, want empty question grammar ignored", plan.Scope)
	}
}

func TestAssistantRunAnalysisReplyIncludesMCPChainAndLogHint(t *testing.T) {
	runID := uuid.NewString()
	reply := composeRunAnalysisReply([]assistantToolActivity{
		{
			Name:   "nopsai.get_pipeline_run",
			Status: assistantToolStatusSuccess,
			Output: map[string]any{
				"run_id":         runID,
				"pipeline_id":    "platform/deploy-api",
				"status":         "failure",
				"failure_reason": "task failed",
			},
		},
		{
			Name:   "nopsai.get_pipeline_run_logs",
			Status: assistantToolStatusSuccess,
			Output: map[string]any{
				"logs": []map[string]any{
					{"line": "starting deploy"},
					{"line": "ERROR image tag is invalid"},
				},
				"bytes_truncated": true,
				"max_bytes":       120000,
			},
		},
		{
			Name:   "nopsai.analyze_pipeline_run_failure",
			Status: assistantToolStatusSuccess,
			Output: map[string]any{
				"root_cause_hint": "image tag is invalid",
				"suggested_next_steps": []string{
					"Check the image tag variable.",
				},
			},
		},
	})

	for _, want := range []string{"hosted Nopsai MCP chain", "platform/deploy-api", "ERROR image tag is invalid", "Log excerpt was truncated", "image tag is invalid"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantVariableUsageReplyStaysMetadataOnly(t *testing.T) {
	reply := composeVariableUsageReply([]assistantToolActivity{{
		Name:   "nopsai.analyze_variable_usage",
		Status: assistantToolStatusSuccess,
		Output: map[string]any{
			"total_visible_variables":   4,
			"unique_variable_names":     3,
			"repetitive_variable_names": 1,
			"duplicates": []map[string]any{{
				"name":         "DEPLOY_REGION",
				"occurrences":  2,
				"scopes":       []string{"dev", "prod"},
				"repositories": []string{"global"},
			}},
			"values_read": false,
		},
	}})

	if !strings.Contains(reply, "DEPLOY_REGION: 2 entries") || !strings.Contains(reply, "Values were not read") {
		t.Fatalf("reply did not summarize metadata-only variable usage:\n%s", reply)
	}
	if strings.Contains(strings.ToLower(reply), "value:") {
		t.Fatalf("reply should not include variable values:\n%s", reply)
	}
}

func TestAssistantAIUsageReplyRanksPipelinesByTokens(t *testing.T) {
	reply := composeAIUsageReply(assistantTurnPlan{}, []assistantToolActivity{{
		Name:   "nopsai.get_monitoring_ai_usage",
		Status: assistantToolStatusSuccess,
		Output: map[string]any{
			"total_tokens":            4500,
			"total_prompt_tokens":     3000,
			"total_completion_tokens": 1500,
			"by_pipeline": []map[string]any{{
				"label":  "deploy-api",
				"tokens": 3200,
				"count":  7,
			}},
			"top_token_runs": []map[string]any{{
				"label":  "run-1",
				"tokens": 2100,
			}},
		},
	}})

	for _, want := range []string{"Total tokens: 4500", "deploy-api: 3200 tokens", "run-1: 2100 tokens"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantAIUsageReplyRanksLowestTokenSchedules(t *testing.T) {
	plan := assistantPlanFromMessage("which one of the schedules run a pipeline with lower llm token required", assistantConversationMemory{})
	if plan.Intent != "ai_token_usage" {
		t.Fatalf("intent = %q, want ai_token_usage", plan.Intent)
	}
	if !assistantPlanAsksScheduleLowTokens(plan) {
		t.Fatalf("plan should detect schedule low-token ranking: %#v", plan)
	}
	reply := composeAIUsageReply(plan, []assistantToolActivity{{
		Name:   "nopsai.get_monitoring_ai_usage",
		Status: assistantToolStatusSuccess,
		Output: map[string]any{
			"total_tokens": 900,
			"lowest_token_schedules": []map[string]any{{
				"label":  "prod/nightly-smoke",
				"tokens": 120,
				"count":  2,
			}},
			"by_schedule": []map[string]any{{
				"label":  "prod/full-regression",
				"tokens": 780,
				"count":  6,
			}},
		},
	}})

	if !strings.Contains(reply, "Lowest token schedules") || !strings.Contains(reply, "prod/nightly-smoke: 120 tokens") {
		t.Fatalf("reply did not rank lowest-token schedules:\n%s", reply)
	}
	if strings.Contains(reply, "Highest token schedules") {
		t.Fatalf("reply should prefer lowest-token schedules for this prompt:\n%s", reply)
	}
}

func TestAssistantScopeSecretSummaryReplyCountsSecretsPerScope(t *testing.T) {
	reply := composeScopeSecretSummaryReply([]assistantToolActivity{
		{
			Name:   "nopsai.list_scopes",
			Status: assistantToolStatusSuccess,
			Output: map[string]any{
				"scopes": []string{"default", "prod", "stage"},
			},
		},
		{
			Name:   "nopsai.list_secret_scopes",
			Status: assistantToolStatusSuccess,
			Output: map[string]any{
				"response": []any{
					map[string]any{"scope": "default", "secret_count": float64(2)},
					map[string]any{"scope": "prod", "secret_count": float64(1)},
				},
			},
		},
	})

	for _, want := range []string{"Total visible scopes: 3", "Total visible secrets: 3", "default: 2 secrets", "prod: 1 secret", "stage: 0 secrets", "plaintext secret values were not read"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantAIUsageReplyExplainsEmptyInvestigation(t *testing.T) {
	reply := composeAIUsageReply(assistantTurnPlan{}, []assistantToolActivity{
		{
			Name:   "nopsai.get_monitoring_ai_usage",
			Status: assistantToolStatusSuccess,
			Input:  map[string]any{},
			Output: map[string]any{
				"total_tokens":            0,
				"total_prompt_tokens":     0,
				"total_completion_tokens": 0,
				"by_pipeline":             []map[string]any{},
				"top_token_runs":          []map[string]any{},
			},
		},
		{
			Name:   "nopsai.get_monitoring_ai_usage",
			Status: assistantToolStatusSuccess,
			Input:  map[string]any{"from": "2026-03-19T00:00:00Z"},
			Output: map[string]any{"total_tokens": 0},
		},
		{
			Name:   "nopsai.get_monitoring_summary",
			Status: assistantToolStatusSuccess,
			Output: map[string]any{"total_runs": 12},
		},
	})

	for _, want := range []string{"Windows checked: 2", "default monitoring window", "no visible AI usage events", "12 visible pipeline runs", "/v1/internal/runs/{runID}/ai-usage"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantAIUsageInvestigationRetriesAndCollectsContext(t *testing.T) {
	plan := assistantPlanFromMessage("give me llm usage for qwen model", assistantConversationMemory{})
	if plan.Intent != "ai_token_usage" {
		t.Fatalf("intent = %q, want ai_token_usage", plan.Intent)
	}
	if plan.AIUsageFilters["model"] != "qwen" {
		t.Fatalf("ai filters = %#v, want model qwen", plan.AIUsageFilters)
	}
	calls := []assistantToolActivity{}
	(&App{}).runAIUsageInvestigation(plan, func(name string, args map[string]any) assistantToolActivity {
		call := assistantToolActivity{Name: name, Input: args, Status: assistantToolStatusSuccess, Output: map[string]any{}}
		switch name {
		case "nopsai.get_monitoring_ai_usage":
			call.Output = map[string]any{"total_tokens": 0}
		case "nopsai.get_monitoring_summary":
			call.Output = map[string]any{"total_runs": 4}
		case "nopsai.list_pipeline_runs":
			call.Output = map[string]any{"runs": []map[string]any{{"run_id": "run-1"}}}
		}
		calls = append(calls, call)
		return call
	})

	names := []string{}
	for _, call := range calls {
		names = append(names, call.Name)
		if strings.HasPrefix(call.Name, "nopsai.get_monitoring_") && call.Input["model"] != "qwen" {
			t.Fatalf("monitoring call missing model filter: %#v", call)
		}
	}
	want := []string{
		"nopsai.get_monitoring_ai_usage",
		"nopsai.get_monitoring_ai_usage",
		"nopsai.get_monitoring_ai_usage",
		"nopsai.get_monitoring_efficiency",
		"nopsai.get_monitoring_summary",
		"nopsai.list_pipeline_runs",
		"nopsai.get_llm_profiles",
	}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %#v, want %#v", names, want)
	}
}

func TestAssistantAIUsageStepRankingDoesNotInventStepFilter(t *testing.T) {
	plan := assistantPlanFromMessage("which step used the most tokens?", assistantConversationMemory{})
	if plan.Intent != "ai_token_usage" {
		t.Fatalf("intent = %q, want ai_token_usage", plan.Intent)
	}
	if _, ok := plan.AIUsageFilters["step_name"]; ok {
		t.Fatalf("step ranking question should not add a literal step filter: %#v", plan.AIUsageFilters)
	}
}

func TestAssistantAIUsageInvestigationStopsWhenFallbackFindsEvents(t *testing.T) {
	plan := assistantPlanFromMessage("which pipeline used most llm tokens?", assistantConversationMemory{})
	usageCalls := 0
	calls := []assistantToolActivity{}
	(&App{}).runAIUsageInvestigation(plan, func(name string, args map[string]any) assistantToolActivity {
		call := assistantToolActivity{Name: name, Input: args, Status: assistantToolStatusSuccess, Output: map[string]any{}}
		if name == "nopsai.get_monitoring_ai_usage" {
			usageCalls++
			call.Output = map[string]any{"total_tokens": 0}
			if usageCalls == 2 {
				call.Output = map[string]any{"total_tokens": 100, "by_pipeline": []map[string]any{{"label": "deploy-api", "tokens": 100}}}
			}
		}
		calls = append(calls, call)
		return call
	})

	if usageCalls != 2 {
		t.Fatalf("usage calls = %d, want 2", usageCalls)
	}
	for _, call := range calls {
		if call.Name == "nopsai.list_pipeline_runs" || call.Name == "nopsai.get_llm_profiles" {
			t.Fatalf("unexpected no-event diagnostic call after usage was found: %#v", calls)
		}
	}
}

func TestAssistantOrchestrationClarifiesAmbiguousUsage(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("pipeline_run.list")}

	result := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		assistantConversation{ID: uuid.New(), DocsVersion: "auto"},
		"show usage",
		"",
	)

	if len(result.ToolCalls) != 0 {
		t.Fatalf("tool calls = %#v, want none before clarification", result.ToolCalls)
	}
	if !strings.Contains(result.Reply, "Which usage area should I check") {
		t.Fatalf("reply = %q, want clarification question", result.Reply)
	}
	if result.Memory.Entities["last_intent"] != "clarify" {
		t.Fatalf("memory last intent = %#v, want clarify", result.Memory.Entities["last_intent"])
	}
}

func TestAssistantOrchestrationAnswersSensitivePolicyFromCapabilities(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("system.read")}

	result := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		assistantConversation{ID: uuid.New(), DocsVersion: "auto"},
		"do we have any policy to prevent showing envs?",
		"",
	)

	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "nopsai.get_feature_capabilities" {
		t.Fatalf("tool calls = %#v, want feature capabilities", result.ToolCalls)
	}
	if result.ToolCalls[0].Input["area"] != "secrets" || result.ToolCalls[0].Input["query"] != "secret" {
		t.Fatalf("capability input = %#v, want secrets/secret", result.ToolCalls[0].Input)
	}
	if !strings.Contains(result.Reply, "Policy notes") || !strings.Contains(result.Reply, "Plaintext secret reads remain blocked") {
		t.Fatalf("reply = %q, want sensitive policy notes", result.Reply)
	}
}

func TestAssistantOrchestrationChecksDispatcherAndRunners(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	app := &App{
		cfg: &config.Config{
			AAAAPIURL:          server.URL,
			NopsaiGitBotAPIURL: server.URL,
		},
		httpClient: server.Client(),
		dispatcher: &fakeDispatcherClient{status: &proto.DispatcherStatus{
			QueuedJobs: 1,
			Runners: []*proto.RunnerInfo{{
				RunnerId:          "runner-a",
				Capacity:          2,
				ActiveJobs:        1,
				InflightJobs:      1,
				LastHeartbeatUnix: time.Now().Unix(),
				AllowDispatch:     true,
				Metadata:          map[string]string{"runtime": "docker"},
			}},
		}},
		aaaLocal: allowActionsForAssistantTest("system.read"),
	}

	result := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		assistantConversation{ID: uuid.New(), DocsVersion: "auto"},
		"Can you check dispatcher and runners?",
		"",
	)

	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "nopsai.get_dispatcher_status" {
		t.Fatalf("tool calls = %#v, want dispatcher status", result.ToolCalls)
	}
	if result.ToolCalls[0].Status != assistantToolStatusSuccess {
		t.Fatalf("dispatcher status = %q, output = %#v", result.ToolCalls[0].Status, result.ToolCalls[0].Output)
	}
	if !strings.Contains(result.Reply, "Dispatcher and runner status") || !strings.Contains(result.Reply, "runner-a") {
		t.Fatalf("reply = %q", result.Reply)
	}
}

func TestAssistantOrchestrationChecksMCPFeatureCapabilities(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("system.read", "pipeline.list")}

	result := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		assistantConversation{ID: uuid.New(), DocsVersion: "auto"},
		"Do we support all NopsAI features with MCP for my permissions?",
		"",
	)

	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "nopsai.get_feature_capabilities" {
		t.Fatalf("tool calls = %#v, want feature capabilities", result.ToolCalls)
	}
	if result.ToolCalls[0].Status != assistantToolStatusSuccess {
		t.Fatalf("feature capabilities status = %q, output = %#v", result.ToolCalls[0].Status, result.ToolCalls[0].Output)
	}
	if !strings.Contains(result.Reply, "MCP feature coverage for your current permissions") ||
		!strings.Contains(result.Reply, "current authenticated AAA subject") {
		t.Fatalf("reply = %q", result.Reply)
	}
}

func TestAssistantOrchestrationCallsExplicitAPIRouteThroughMCP(t *testing.T) {
	enabled := true
	app := &App{
		cfg: &config.Config{Assistant: config.AssistantConfig{
			Features: config.AssistantFeaturesConfig{ActionExecution: &enabled},
		}},
		aaaLocal: allowActionsForAssistantTest("system.update"),
	}

	result := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		assistantConversation{ID: uuid.New(), DocsVersion: "auto"},
		"Call API POST /v1/system/config/sync",
		"",
	)

	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "nopsai.call_api" {
		t.Fatalf("tool calls = %#v, want API bridge", result.ToolCalls)
	}
	if result.ToolCalls[0].Status != assistantToolStatusSuccess {
		t.Fatalf("API bridge status = %q, output = %#v", result.ToolCalls[0].Status, result.ToolCalls[0].Output)
	}
	if result.ToolCalls[0].Output["requires_confirmation"] != true {
		t.Fatalf("requires_confirmation = %#v, want true", result.ToolCalls[0].Output["requires_confirmation"])
	}
	if !strings.Contains(result.Reply, "Confirmation required") {
		t.Fatalf("reply = %q", result.Reply)
	}
}

func TestAssistantOrchestrationSynthesizesReplyWithLLMProfile(t *testing.T) {
	credentialRef := "credential://system/llm/standard"
	var capturedPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q", got)
		}
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(payload.Messages) > 0 {
			capturedPrompt = payload.Messages[0].Content
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"LLM final answer. No changes were applied."}}],"usage":{"prompt_tokens":11,"completion_tokens":4,"total_tokens":15}}`)
	}))
	defer server.Close()

	app := &App{
		cfg: &config.Config{
			LLMDefaultProfile: "standard",
			LLMProfiles: map[string]config.LLMProfile{
				"standard": {
					Provider:      config.LLMProviderOpenAI,
					Model:         "gpt-test",
					BaseURL:       server.URL + "/v1",
					CredentialRef: credentialRef,
				},
			},
		},
		httpClient:         server.Client(),
		credentialResolver: staticCredentialResolver{credentialRef: "secret"},
		aaaLocal:           allowActionsForAssistantTest("pipeline.create", "pipeline.read"),
	}
	conversation := assistantConversation{ID: uuid.New(), DocsVersion: "auto", SelectedLLMProfile: "standard"}

	result := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		conversation,
		"Generate pipeline YAML named deploy-web",
		"standard",
	)

	if result.Reply != "LLM final answer. No changes were applied." {
		t.Fatalf("reply = %q, want quality-safe LLM answer", result.Reply)
	}
	if !strings.Contains(capturedPrompt, "Generated pipeline YAML") || !strings.Contains(capturedPrompt, "No changes were applied") {
		t.Fatalf("prompt missing deterministic context: %s", capturedPrompt)
	}
	call := assistantFirstToolCall(result.ToolCalls, assistantLLMToolName)
	if call.Status != assistantToolStatusSuccess {
		t.Fatalf("llm status = %q, output = %#v", call.Status, call.Output)
	}
	if assistantOutputString(call.Output, "profile") != "standard" {
		t.Fatalf("llm profile output = %#v", call.Output)
	}
}

func TestAssistantOrchestrationFallsBackWhenLLMClaimsUnappliedChange(t *testing.T) {
	credentialRef := "credential://system/llm/standard"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"I applied the pipeline change."}}],"usage":{"prompt_tokens":9,"completion_tokens":5,"total_tokens":14}}`)
	}))
	defer server.Close()

	app := &App{
		cfg: &config.Config{
			LLMDefaultProfile: "standard",
			LLMProfiles: map[string]config.LLMProfile{
				"standard": {
					Provider:      config.LLMProviderOpenAI,
					Model:         "gpt-test",
					BaseURL:       server.URL + "/v1",
					CredentialRef: credentialRef,
				},
			},
		},
		httpClient:         server.Client(),
		credentialResolver: staticCredentialResolver{credentialRef: "secret"},
		aaaLocal:           allowActionsForAssistantTest("pipeline.create", "pipeline.read"),
	}

	result := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		assistantConversation{ID: uuid.New(), DocsVersion: "auto", SelectedLLMProfile: "standard"},
		"Generate pipeline YAML named deploy-web",
		"standard",
	)

	if !strings.Contains(result.Reply, "I drafted a GitOps-safe pipeline proposal") || !strings.Contains(result.Reply, "No changes were applied") {
		t.Fatalf("reply should fall back to deterministic proposal-safe summary: %q", result.Reply)
	}
	call := assistantFirstToolCall(result.ToolCalls, assistantLLMToolName)
	if call.Status != assistantToolStatusSuccess || !assistantOutputBool(call.Output, "quality_fallback") {
		t.Fatalf("llm quality fallback not recorded: status=%q output=%#v", call.Status, call.Output)
	}
}

func TestAssistantOrchestrationFallsBackWhenLLMProviderFails(t *testing.T) {
	credentialRef := "credential://system/llm/standard"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "provider unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	app := &App{
		cfg: &config.Config{
			LLMDefaultProfile: "standard",
			LLMProfiles: map[string]config.LLMProfile{
				"standard": {
					Provider:      config.LLMProviderOpenAI,
					Model:         "gpt-test",
					BaseURL:       server.URL + "/v1",
					CredentialRef: credentialRef,
				},
			},
		},
		httpClient:         server.Client(),
		credentialResolver: staticCredentialResolver{credentialRef: "secret"},
		aaaLocal:           allowActionsForAssistantTest("pipeline.create", "pipeline.read"),
	}

	result := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		assistantConversation{ID: uuid.New(), DocsVersion: "auto"},
		"Generate pipeline YAML named deploy-web",
		"standard",
	)

	if !strings.Contains(result.Reply, "Draft YAML") || strings.Contains(result.Reply, "LLM synthesis was unavailable") {
		t.Fatalf("reply did not fall back cleanly to deterministic summary: %q", result.Reply)
	}
	call := assistantFirstToolCall(result.ToolCalls, assistantLLMToolName)
	if call.Status != assistantToolStatusError {
		t.Fatalf("llm status = %q, want error", call.Status)
	}
	if assistantOutputString(call.Output, "fallback_reason") == "" {
		t.Fatalf("fallback reason missing: %#v", call.Output)
	}
}

func TestAssistantPlanValidationFailsClosedForUnavailableTool(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("pipeline_run.list")}
	plan := assistantTurnPlan{
		Intent: "test",
		Goal:   "List runs and then inspect LLM profiles",
		Steps: []assistantPlanStep{
			{ToolName: "nopsai.list_pipeline_runs", Args: map[string]any{"limit": 5}},
			{ToolName: "nopsai.get_llm_profiles", Args: map[string]any{}},
		},
	}

	called := false
	handled, activity := app.runAssistantValidatedToolPlan(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		plan,
		func(name string, args map[string]any) assistantToolActivity {
			called = true
			return assistantToolActivity{Name: name, Input: args, Status: assistantToolStatusSuccess}
		},
	)

	if !handled || activity == nil {
		t.Fatalf("plan should be handled by validation failure, activity = %#v", activity)
	}
	if called {
		t.Fatal("plan execution should not call any tools after validation failure")
	}
	if activity.Status != assistantToolStatusDenied || !strings.Contains(assistantOutputString(activity.Output, "error"), "unavailable tool") {
		t.Fatalf("validation activity = %#v", activity)
	}
}

func TestAssistantPlanValidationCapsToolCount(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("pipeline_run.list")}
	plan := assistantTurnPlan{Intent: "test", Goal: "too many calls"}
	for idx := 0; idx < assistantMaxPlanToolCalls+1; idx++ {
		plan.Steps = append(plan.Steps, assistantPlanStep{
			ToolName: "nopsai.list_pipeline_runs",
			Args:     map[string]any{"limit": 1},
		})
	}

	err := app.validateAssistantToolPlan(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		plan,
	)
	if err == nil || !strings.Contains(err.Error(), "max allowed") {
		t.Fatalf("err = %v, want max tool call validation", err)
	}
}

func TestAssistantPlanValidationRequiresConfirmForMutatingTools(t *testing.T) {
	enabled := true
	app := &App{
		cfg: &config.Config{Assistant: config.AssistantConfig{
			Features: config.AssistantFeaturesConfig{ActionExecution: &enabled},
		}},
		aaaLocal: allowActionsForAssistantTest("system.update"),
	}
	plan := assistantTurnPlan{
		Intent: "test",
		Goal:   "Pause a runner",
		Steps: []assistantPlanStep{{
			ToolName: "nopsai.update_runner_dispatch",
			Args: map[string]any{
				"runner_id":      "runner-a",
				"allow_dispatch": false,
			},
		}},
	}

	err := app.validateAssistantToolPlan(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		plan,
	)
	if err == nil || !strings.Contains(err.Error(), "without confirm:true") {
		t.Fatalf("err = %v, want confirmation validation", err)
	}

	plan.Steps[0].Args["confirm"] = true
	if err := app.validateAssistantToolPlan(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		plan,
	); err != nil {
		t.Fatalf("confirmed plan should pass validation: %v", err)
	}
}

func allowActionsForAssistantTest(actions ...string) stubAAAAuthorizer {
	allowedActions := map[string]struct{}{}
	for _, action := range actions {
		allowedActions[action] = struct{}{}
	}
	return stubAAAAuthorizer{
		checkFn: func(_ context.Context, _ model.Subject, action string, _ model.ResourceRef, _ map[string]any) (model.Decision, error) {
			_, ok := allowedActions[action]
			return model.Decision{Allowed: ok}, nil
		},
	}
}

func assistantToolNamesForTest(tools []hostedMCPTool) map[string]bool {
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	return names
}

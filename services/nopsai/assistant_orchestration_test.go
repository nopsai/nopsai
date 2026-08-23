package nopsai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"nopsai/config"
	"nopsai/services/aaa/pkg/model"
)

const assistantValidPipelineYAMLForTest = "name: deploy-web\ncontainer_image: alpine:3.20\nsteps:\n  - name: plan\n    script: echo ok\n"

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
	app := &App{aaaLocal: allowActionsForAssistantTest("pipeline.read")}
	result, err := app.callAssistantHostedMCPTool(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		uuid.New(),
		"nopsai.validate_pipeline",
		map[string]any{"yaml": assistantValidPipelineYAMLForTest},
	)
	if err != nil {
		t.Fatalf("callAssistantHostedMCPTool() error = %v", err)
	}
	if result["valid"] != true || result["name"] != "deploy-web" {
		t.Fatalf("structured MCP validation result = %#v, want valid deploy-web", result)
	}
}

func TestAssistantHostedMCPToolRespectsMCPEnabledFlag(t *testing.T) {
	disabled := false
	app := &App{
		cfg:      &config.Config{Assistant: config.AssistantConfig{MCP: config.AssistantMCPConfig{Enabled: &disabled}}},
		aaaLocal: allowActionsForAssistantTest("pipeline.read"),
	}

	call := app.runAssistantHostedMCPTool(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		uuid.New(),
		"nopsai.validate_pipeline",
		map[string]any{"yaml": assistantValidPipelineYAMLForTest},
	)
	if call.Status != assistantToolStatusDenied || !strings.Contains(assistantOutputString(call.Output, "error"), "disabled") {
		t.Fatalf("call = %#v, want disabled denial", call)
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
	if scope := assistantScopeFromMessage("how many scope do we have and for each how many secrets"); scope != "" {
		t.Fatalf("scope = %q, want empty question grammar ignored", scope)
	}
	if pipelineID := assistantPipelineIDFromMessage("metrics, events, and pipeline gonna build docker images"); pipelineID != "" {
		t.Fatalf("pipeline id = %q, want empty grammar ignored", pipelineID)
	}
	if pipelineID := assistantPipelineIDFromMessage("make the pipeline more efficient and faster"); pipelineID != "" {
		t.Fatalf("pipeline id = %q, want empty grammar ignored", pipelineID)
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

func TestAssistantRunAnalysisReplyUsesAnalyzerEmbeddedLogExcerpt(t *testing.T) {
	runID := uuid.NewString()
	reply := composeRunAnalysisReply([]assistantToolActivity{{
		Name:   "nopsai.analyze_pipeline_run_failure",
		Status: assistantToolStatusSuccess,
		Output: map[string]any{
			"run": map[string]any{
				"run_id":      runID,
				"pipeline_id": "platform/deploy-api",
				"status":      "failure",
			},
			"log_excerpt": []map[string]any{
				{"line": "fatal: not a git repository"},
			},
			"root_cause_hint": "fatal: not a git repository",
		},
	}})

	for _, want := range []string{"platform/deploy-api", "Log lines reviewed: 1", "fatal: not a git repository", "No changes were applied"} {
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

func TestAssistantAIUsageReplyUsesConciseCatalogOverviewWithoutRequestedDimension(t *testing.T) {
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
			"by_provider": []map[string]any{{
				"label":  "gemini",
				"tokens": 4500,
				"count":  8,
			}},
			"top_token_runs": []map[string]any{{
				"label":  "run-1",
				"tokens": 2100,
			}},
		},
	}})

	for _, want := range []string{"Total tokens: 4500", "AI usage by provider", "gemini: 4500 tokens", "AI usage by pipeline", "deploy-api: 3200 tokens"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "run-1") {
		t.Fatalf("broad overview should not dump run IDs unless run dimension is requested:\n%s", reply)
	}
}

func TestAssistantAIUsageReplySelectsDimensionFromCatalog(t *testing.T) {
	plan := assistantTurnPlan{
		Intent:       "ai_token_usage",
		LowerContent: "break down llm token usage across providers",
	}
	reply := composeAIUsageReply(plan, []assistantToolActivity{{
		Name:   "nopsai.get_monitoring_ai_usage",
		Status: assistantToolStatusSuccess,
		Output: map[string]any{
			"total_tokens": 1200,
			"by_provider": []map[string]any{{
				"label":  "lmstudio",
				"tokens": 900,
				"count":  9,
			}, {
				"label":  "gemini",
				"tokens": 300,
				"count":  3,
			}},
			"by_pipeline": []map[string]any{{
				"label":  "deploy-api",
				"tokens": 1200,
				"count":  12,
			}},
		},
	}})

	for _, want := range []string{"AI usage by provider", "Total tokens checked: 1200", "Highest token provider: lmstudio with 900 tokens", "gemini: 300 tokens"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("dimension-selected reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "AI usage by pipeline") || strings.Contains(reply, "deploy-api") {
		t.Fatalf("dimension-selected reply should not dump unrelated sections:\n%s", reply)
	}
}

func TestAssistantAIUsageReplyUsesRequestedDimensionWhenCostTermsArePresent(t *testing.T) {
	plan := assistantTurnPlan{
		Intent:       "ai_token_usage",
		LowerContent: "show cost impact grouped by pipeline",
	}
	reply := composeAIUsageReply(plan, []assistantToolActivity{{
		Name:   "nopsai.get_monitoring_ai_usage",
		Status: assistantToolStatusSuccess,
		Output: map[string]any{
			"total_tokens": 1200,
			"by_provider": []map[string]any{{
				"label":  "gemini",
				"tokens": 1200,
				"count":  12,
			}},
			"by_pipeline": []map[string]any{{
				"label":  "deploy-api",
				"tokens": 1200,
				"count":  12,
			}},
		},
	}})

	if !strings.Contains(reply, "AI usage by pipeline") || !strings.Contains(reply, "Highest token pipeline: deploy-api with 1200 tokens") {
		t.Fatalf("reply should focus the requested dimension:\n%s", reply)
	}
	if !strings.Contains(reply, "Pricing fields are not included") {
		t.Fatalf("cost wording should explain token-volume ranking when pricing fields are absent:\n%s", reply)
	}
	if strings.Contains(reply, "Provider breakdown") || strings.Contains(reply, "gemini: 1200 tokens") {
		t.Fatalf("reply should not fall back to another dimension:\n%s", reply)
	}
}

func TestAssistantAIUsageReplyUnwrapsHostedMCPResponse(t *testing.T) {
	plan := assistantTurnPlan{
		Intent:       "ai_token_usage",
		LowerContent: "compare pipeline llm token consumption",
	}
	toolCalls := []assistantToolActivity{{
		Name:   "nopsai.get_monitoring_ai_usage",
		Status: assistantToolStatusSuccess,
		Output: map[string]any{
			"status_code": 200,
			"ok":          true,
			"response": map[string]any{
				"total_tokens":            921516,
				"total_prompt_tokens":     700000,
				"total_completion_tokens": 221516,
				"exact_token_events":      314,
				"by_pipeline": []map[string]any{{
					"key":    "prod/main-test",
					"label":  "main-test",
					"tokens": 810862,
					"count":  314,
				}, {
					"key":    "prod/reference-pipeline",
					"label":  "reference-pipeline",
					"tokens": 110654,
					"count":  42,
				}},
				"by_step": []map[string]any{{
					"label":  "repository-map",
					"tokens": 441492,
					"count":  110,
				}},
			},
		},
	}}

	reply := composeAIUsageReply(plan, toolCalls)
	for _, want := range []string{"AI usage by pipeline", "Total tokens checked: 921516", "Highest token pipeline: main-test with 810862 tokens", "reference-pipeline: 110654 tokens"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
		}
	}
	if strings.Contains(reply, "repository-map") {
		t.Fatalf("pipeline-focused reply should not dump step sections:\n%s", reply)
	}
	if !assistantAnyAIUsageCallHasEvents(toolCalls) {
		t.Fatal("wrapped hosted MCP response should count as AI usage evidence")
	}
	quality := assistantAssessAnswerQuality(plan, toolCalls, "The highest LLM token pipeline is main-test with 810862 tokens.")
	if !assistantAnswerQualityPasses(quality) {
		t.Fatalf("wrapped AI usage evidence should pass answer quality: %#v", quality)
	}
}

func TestAssistantAIUsageReplyUsesLowRankFieldWhenCatalogDimensionSupportsIt(t *testing.T) {
	plan := assistantTurnPlan{
		Intent:       "ai_token_usage",
		LowerContent: "show lower token usage across schedules",
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

	if !strings.Contains(reply, "Lowest token schedules") || !strings.Contains(reply, "prod/nightly-smoke with 120 tokens") {
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

func TestAssistantDirectSecretWriteAsksForConfirmationAndStoresPending(t *testing.T) {
	enabled := true
	app := &App{
		cfg:      &config.Config{Assistant: config.AssistantConfig{Features: config.AssistantFeaturesConfig{ActionExecution: &enabled}}},
		aaaLocal: allowActionsForAssistantTest("secret.write_value", "secret.list_metadata", "scope.read"),
	}
	conversation := assistantConversation{ID: uuid.New(), DocsVersion: "auto"}

	result := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		conversation,
		"add encrypted NEW=aaasderfdfhjbd I want to add it to secret prod",
		"",
	)

	if len(result.ToolCalls) != 0 {
		t.Fatalf("direct secret write should wait for confirmation before tool calls: %#v", result.ToolCalls)
	}
	for _, want := range []string{"Please confirm", "direct MCP change", "Name: NEW", "Scope: prod", "Value: provided, not shown", "No changes were applied"} {
		if !strings.Contains(result.Reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, result.Reply)
		}
	}
	pending, ok := assistantPendingConfirmationFromMemory(result.Memory)
	if !ok {
		t.Fatalf("pending confirmation missing from memory: %#v", result.Memory.Entities)
	}
	if pending.Tool != "nopsai.write_secret_value" ||
		stringArg(pending.Args, "secret_name") != "NEW" ||
		stringArg(pending.Args, "value") != "aaasderfdfhjbd" ||
		stringArg(pending.Args, "scope") != "prod" {
		t.Fatalf("pending confirmation = %#v", pending)
	}

	conversation.Memory = result.Memory
	followUp := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		conversation,
		"NEW aaasderfdfhjbd prod",
		"",
	)
	if len(followUp.ToolCalls) != 0 {
		t.Fatalf("detail-only follow-up should still wait for explicit confirmation: %#v", followUp.ToolCalls)
	}
	if strings.Contains(followUp.Reply, "schema_tools") || strings.Contains(followUp.Reply, "planner selected") {
		t.Fatalf("follow-up leaked internal planner detail: %q", followUp.Reply)
	}
	if !strings.Contains(followUp.Reply, "Reply `confirm` to apply") {
		t.Fatalf("follow-up should ask for explicit confirmation: %q", followUp.Reply)
	}
}

func TestAssistantFallbackReplyUsesEvidenceForNovelPlannerIntent(t *testing.T) {
	reply := composeAssistantReply(assistantTurnPlan{
		Intent: "dashboard_inventory",
		Goal:   "Find dashboard data",
	}, "", []assistantToolActivity{{
		Name:   "nopsai.list_dashboards",
		Status: assistantToolStatusSuccess,
		Output: map[string]any{
			"dashboards": []map[string]any{{
				"id":        "dash-1",
				"title":     "Production deploys",
				"team_path": "platform",
			}},
		},
	}})

	if !strings.Contains(reply, "list dashboards") ||
		!strings.Contains(reply, "Dashboards: 1 item(s): dash-1") ||
		!strings.Contains(reply, "No changes were applied") {
		t.Fatalf("novel planner intent should render tool evidence:\n%s", reply)
	}
	if strings.Contains(reply, "could not search docs") {
		t.Fatalf("novel planner intent should not fall through to docs renderer:\n%s", reply)
	}
}

func TestAssistantFeatureReplySummarizesMonitoringResponseItems(t *testing.T) {
	reply := composeFeatureToolReply([]assistantToolActivity{{
		Name:   "nopsai.get_monitoring_step_performance",
		Status: assistantToolStatusSuccess,
		Output: map[string]any{
			"ok":          true,
			"status_code": 200,
			"response": map[string]any{
				"total_runs": 42,
				"items": []map[string]any{{
					"key":                  "build-image",
					"label":                "build-image",
					"p95_duration_seconds": 118,
				}, {
					"key":                  "deploy",
					"label":                "deploy",
					"p95_duration_seconds": 73,
				}},
			},
		},
	}})

	for _, want := range []string{
		"get monitoring step performance",
		"Response: total runs=42",
		"Response Items: 2 item(s): build-image, deploy",
		"No changes were applied",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("monitoring feature reply missing %q:\n%s", want, reply)
		}
	}
}

func TestAssistantOptimizationReplySummarizesCombinedMonitoringEvidence(t *testing.T) {
	reply := composeAssistantReply(assistantTurnPlan{
		Intent: "optimization_review",
		Goal:   "make this more efficient",
	}, "standard", []assistantToolActivity{{
		Name:   "nopsai.find_optimization_opportunities",
		Status: assistantToolStatusSuccess,
		Input:  map[string]any{"pipeline": "nopsai/nopsai-platform-release"},
		Output: map[string]any{
			"ok":      true,
			"applied": false,
			"source_paths": []string{
				"/v1/monitoring/efficiency?conversation_id=turn-1&pipelinePath=nopsai&pipelineName=nopsai-platform-release",
				"/v1/monitoring/recommendations?pipelinePath=nopsai&pipelineName=nopsai-platform-release",
			},
			"efficiency": map[string]any{
				"response": map[string]any{
					"total_runtime_seconds": 1200,
					"total_ai_spend_usd":    0,
					"recommendations": []string{
						"Pipeline nopsai/nopsai-platform-release has a 45% success rate across 123 runs.",
						"Pipeline main-test is the highest AI token consumer in this window.",
						"Team Root has average queue time above five minutes.",
					},
					"costly_low_success_pipelines": []map[string]any{{
						"key":          "prod/main-test",
						"success_rate": 0.05,
						"total_runs":   66,
					}},
				},
			},
			"ai_usage": map[string]any{
				"response": map[string]any{
					"spend_usd":      0,
					"unpriced_calls": 2,
				},
			},
			"pipeline_performance": map[string]any{
				"response": map[string]any{
					"items": []map[string]any{{
						"key":                      "nopsai/nopsai-platform-release",
						"total_runs":               123,
						"failed_runs":              68,
						"failure_rate":             0.55,
						"average_duration_seconds": 402.25,
						"p99_duration_seconds":     1424.52,
					}},
				},
			},
		},
	}})

	for _, want := range []string{
		"Optimization opportunities:",
		"Target: nopsai/nopsai-platform-release",
		"123 runs, 68 failed, 55% failure rate, 402s average, 1425s p99",
		"Runtime in window: 1200 runner-seconds",
		"AI spend in window: $0.00 recorded",
		"Pipeline nopsai/nopsai-platform-release has a 45% success rate across 123 runs.",
		"Data source: NopsAI monitoring evidence via `nopsai.find_optimization_opportunities`",
		"No changes were applied",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("optimization reply missing %q:\n%s", want, reply)
		}
	}
	for _, unwanted := range []string{
		"main-test",
		"Team Root",
		"conversation_id",
		"pipelinePath",
	} {
		if strings.Contains(reply, unwanted) {
			t.Fatalf("optimization reply should not contain %q:\n%s", unwanted, reply)
		}
	}
	if strings.Contains(reply, "NopsAI feature workflow") || strings.Contains(reply, "find optimization opportunities:") {
		t.Fatalf("optimization reply should not use generic feature fallback:\n%s", reply)
	}
}

func TestAssistantOptimizationReplyFallsBackWhenOnlyOtherPipelineRecommendationsExist(t *testing.T) {
	reply := composeAssistantReply(assistantTurnPlan{
		Intent: "optimization_review",
		Goal:   "make this more efficient",
	}, "standard", []assistantToolActivity{{
		Name:   "nopsai.find_optimization_opportunities",
		Status: assistantToolStatusSuccess,
		Input:  map[string]any{"pipeline": "nopsai/nopsai-platform-release"},
		Output: map[string]any{
			"ok": true,
			"efficiency": map[string]any{
				"response": map[string]any{
					"recommendations": []string{
						"Pipeline main-test is the highest AI token consumer in this window.",
						"Team Root has average queue time above five minutes.",
					},
				},
			},
			"pipeline_performance": map[string]any{
				"response": map[string]any{
					"items": []map[string]any{{
						"key":                   "nopsai/nopsai-platform-release",
						"average_queue_seconds": 420,
					}},
				},
			},
		},
	}})

	if strings.Contains(reply, "main-test") || strings.Contains(reply, "Team Root") {
		t.Fatalf("reply should filter recommendations for other pipelines:\n%s", reply)
	}
	if !strings.Contains(reply, "Reduce queue time for nopsai/nopsai-platform-release") {
		t.Fatalf("reply should fall back to a target-specific next step:\n%s", reply)
	}
}

func TestAssistantPlanDeniedReplyExplainsInvalidPriorEvidenceFinalAnswer(t *testing.T) {
	reply := composeAssistantPlanDeniedReply(assistantToolActivity{
		Output: map[string]any{
			"error": "assistant planner final answer from prior evidence must label the data source and estimate confidence",
		},
	})

	if strings.Contains(reply, "assistant planner final answer") {
		t.Fatalf("reply should not expose internal planner validation wording:\n%s", reply)
	}
	if !strings.Contains(reply, "data source and confidence") || !strings.Contains(reply, "No changes were applied") {
		t.Fatalf("reply missing user-facing source/confidence explanation:\n%s", reply)
	}
}

func TestAssistantAnswerQualityRequiresPipelineEvidenceForPipelineReplies(t *testing.T) {
	plan := assistantTurnPlan{
		Intent: "pipeline",
		Goal:   "Show me a deploy pipeline",
	}

	quality := assistantAssessAnswerQuality(plan, nil, "Here is a deploy pipeline. No changes were applied.")
	if quality.PipelineGrounded || assistantAnswerQualityPasses(quality) {
		t.Fatalf("pipeline answer without successful NopsAI evidence should fail quality: %#v", quality)
	}

	quality = assistantAssessAnswerQuality(plan, []assistantToolActivity{{
		Name:   "nopsai.get_pipeline",
		Status: assistantToolStatusSuccess,
		Output: map[string]any{"id": "platform/deploy-api"},
	}}, "I loaded pipeline platform/deploy-api from NopsAI. No changes were applied.")
	if !quality.PipelineGrounded || !assistantAnswerQualityPasses(quality) {
		t.Fatalf("pipeline answer with successful NopsAI evidence should pass quality: %#v", quality)
	}
}

func TestAssistantAnswerQualityRequiresGitOpsSafetyLanguageForPipelineProposals(t *testing.T) {
	plan := assistantTurnPlan{
		Intent: "propose_pipeline_create",
		Goal:   "Draft a deploy pipeline",
	}
	toolCalls := []assistantToolActivity{{
		Name:   "nopsai.propose_pipeline_create",
		Status: assistantToolStatusSuccess,
		Output: map[string]any{"valid": true, "pipeline_id": "deploy-web"},
	}}

	quality := assistantAssessAnswerQuality(plan, toolCalls, "The pipeline is ready. No changes were applied.")
	if quality.SuggestedNextStep || assistantAnswerQualityPasses(quality) {
		t.Fatalf("pipeline proposal without review/GitOps language should fail quality: %#v", quality)
	}

	quality = assistantAssessAnswerQuality(plan, toolCalls, "I prepared a GitOps-ready pipeline proposal for review. No changes were applied.")
	if !quality.SuggestedNextStep || !assistantAnswerQualityPasses(quality) {
		t.Fatalf("pipeline proposal with review/GitOps language should pass quality: %#v", quality)
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

func assistantPlannerPromptContextForTest(t *testing.T, prompt string) map[string]any {
	t.Helper()
	const marker = "Context:\n"
	idx := strings.LastIndex(prompt, marker)
	if idx < 0 {
		t.Fatalf("planner prompt missing context marker:\n%s", prompt)
	}
	raw := strings.TrimSpace(prompt[idx+len(marker):])
	var context map[string]any
	if err := json.Unmarshal([]byte(raw), &context); err != nil {
		t.Fatalf("planner prompt context JSON error = %v\n%s", err, raw)
	}
	return context
}

func assistantPlannerSchemaToolNamesForTest(t *testing.T, prompt string) map[string]bool {
	t.Helper()
	context := assistantPlannerPromptContextForTest(t, prompt)
	items, ok := context["schema_tools"].([]any)
	if !ok {
		t.Fatalf("planner context schema_tools has unexpected shape: %#v", context["schema_tools"])
	}
	names := map[string]bool{}
	for _, item := range items {
		schemaTool, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("planner schema tool has unexpected shape: %#v", item)
		}
		name, _ := schemaTool["name"].(string)
		if name == "" {
			t.Fatalf("planner schema tool missing name: %#v", schemaTool)
		}
		names[name] = true
	}
	return names
}

func assistantPlannerSchemaToolForTest(t *testing.T, prompt, name string) map[string]any {
	t.Helper()
	context := assistantPlannerPromptContextForTest(t, prompt)
	items, ok := context["schema_tools"].([]any)
	if !ok {
		t.Fatalf("planner context schema_tools has unexpected shape: %#v", context["schema_tools"])
	}
	for _, item := range items {
		schemaTool, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("planner schema tool has unexpected shape: %#v", item)
		}
		if schemaTool["name"] == name {
			return schemaTool
		}
	}
	t.Fatalf("planner prompt missing schema tool %q; available: %#v", name, assistantPlannerSchemaToolNamesForTest(t, prompt))
	return nil
}

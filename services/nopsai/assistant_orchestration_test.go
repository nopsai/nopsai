package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"nopsai/config"
	"nopsai/services/aaa/pkg/model"
)

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

func TestAssistantFeaturePlanValidationBlocksUnconfirmedMutation(t *testing.T) {
	enabled := true
	app := &App{
		cfg: &config.Config{Assistant: config.AssistantConfig{
			Features: config.AssistantFeaturesConfig{ActionExecution: &enabled},
		}},
		aaaLocal: allowActionsForAssistantTest("system.update"),
	}
	plan := assistantTurnPlan{
		Intent: "llm_planned",
		Goal:   "Create a data backup",
		Steps: []assistantPlanStep{{
			ToolName: "nopsai.create_data_backup",
			Args:     map[string]any{},
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
	plan := assistantTurnPlan{
		Intent:       "ai_token_usage",
		LowerContent: "which one of the schedules run a pipeline with lower llm token required",
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
	plan := assistantTurnPlan{Intent: "ai_token_usage", AIUsageFilters: map[string]any{"model": "qwen"}}
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

func TestAssistantAIUsageInvestigationStopsWhenFallbackFindsEvents(t *testing.T) {
	plan := assistantTurnPlan{Intent: "ai_token_usage", AIUsageFilters: map[string]any{}}
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

func TestAssistantOrchestrationSynthesizesReplyWithLLMProfile(t *testing.T) {
	credentialRef := "credential://system/llm/standard"
	requestCount := 0
	var plannerPrompt string
	var synthesisPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q", got)
		}
		requestCount++
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(payload.Messages) > 0 {
			if requestCount == 1 {
				plannerPrompt = payload.Messages[0].Content
			} else {
				synthesisPrompt = payload.Messages[0].Content
			}
		}
		w.Header().Set("Content-Type", "application/json")
		switch requestCount {
		case 1:
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"goal\":\"List current assistant feature coverage\",\"intent\":\"llm_planned\",\"steps\":[{\"tool\":\"nopsai.get_feature_capabilities\",\"args\":{\"query\":\"assistant\",\"include_api_routes\":false},\"reason\":\"Use the current-user capability catalog.\"}],\"success_criteria\":\"Return available feature coverage.\",\"needs_more_tools\":false,\"final_answer\":\"\",\"clarifying_question\":\"\"}"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
		default:
			fmt.Fprint(w, `{"choices":[{"message":{"content":"LLM final answer. No changes were applied."}}],"usage":{"prompt_tokens":11,"completion_tokens":4,"total_tokens":15}}`)
		}
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
		aaaLocal:           allowActionsForAssistantTest("system.read"),
	}
	conversation := assistantConversation{ID: uuid.New(), DocsVersion: "auto", SelectedLLMProfile: "standard"}

	result := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		conversation,
		"What assistant capabilities can I use?",
		"standard",
	)

	if requestCount != 2 {
		t.Fatalf("LLM requests = %d, want planner and synthesis", requestCount)
	}
	if result.Reply != "LLM final answer. No changes were applied." {
		t.Fatalf("reply = %q, want quality-safe LLM answer", result.Reply)
	}
	if !strings.Contains(plannerPrompt, "available_tools") ||
		!strings.Contains(plannerPrompt, "feature_catalog") ||
		!strings.Contains(plannerPrompt, "nopsai.get_feature_capabilities") {
		t.Fatalf("planner prompt missing tool catalog: %s", plannerPrompt)
	}
	if !strings.Contains(synthesisPrompt, "nopsai.get_feature_capabilities") || !strings.Contains(synthesisPrompt, "No changes were applied") {
		t.Fatalf("synthesis prompt missing tool evidence: %s", synthesisPrompt)
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
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		switch requestCount {
		case 1:
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"goal\":\"Generate pipeline YAML named deploy-web\",\"intent\":\"generate_pipeline\",\"steps\":[{\"tool\":\"nopsai.generate_pipeline\",\"args\":{\"name\":\"deploy-web\",\"goal\":\"Generate pipeline YAML named deploy-web\"},\"reason\":\"Draft proposal YAML only.\"}],\"success_criteria\":\"Return a GitOps-safe YAML proposal without applying it.\",\"needs_more_tools\":false,\"final_answer\":\"\",\"clarifying_question\":\"\"}"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
		default:
			fmt.Fprint(w, `{"choices":[{"message":{"content":"I applied the pipeline change."}}],"usage":{"prompt_tokens":9,"completion_tokens":5,"total_tokens":14}}`)
		}
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

	if requestCount != 2 {
		t.Fatalf("LLM requests = %d, want planner and synthesis", requestCount)
	}
	if !strings.Contains(result.Reply, "I drafted a GitOps-safe pipeline proposal") || !strings.Contains(result.Reply, "No changes were applied") {
		t.Fatalf("reply should fall back to deterministic proposal-safe summary: %q", result.Reply)
	}
	call := assistantFirstToolCall(result.ToolCalls, assistantLLMToolName)
	if call.Status != assistantToolStatusSuccess || !assistantOutputBool(call.Output, "quality_fallback") {
		t.Fatalf("llm quality fallback not recorded: status=%q output=%#v", call.Status, call.Output)
	}
}

func TestAssistantOrchestrationReportsPlannerUnavailableWhenLLMProviderFails(t *testing.T) {
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

	if !strings.Contains(result.Reply, "assistant LLM planner was unavailable") || !strings.Contains(result.Reply, "No changes were applied") {
		t.Fatalf("reply did not fail closed on planner error: %q", result.Reply)
	}
	for _, call := range result.ToolCalls {
		if call.Name == "nopsai.generate_pipeline" {
			t.Fatalf("tool should not run without a validated planner result: %#v", call)
		}
	}
	call := assistantFirstToolCall(result.ToolCalls, assistantLLMPlannerToolName)
	if call.Status != assistantToolStatusError {
		t.Fatalf("planner status = %q, want error", call.Status)
	}
	if assistantOutputString(call.Output, "fallback_reason") == "" {
		t.Fatalf("fallback reason missing: %#v", call.Output)
	}
}

func TestAssistantLLMPlannerExecutesValidatedToolPlan(t *testing.T) {
	credentialRef := "credential://system/llm/standard"
	requestCount := 0
	var plannerPrompt string
	var synthesisPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		requestCount++
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(payload.Messages) > 0 {
			if requestCount == 1 {
				plannerPrompt = payload.Messages[0].Content
			} else {
				synthesisPrompt = payload.Messages[0].Content
			}
		}
		w.Header().Set("Content-Type", "application/json")
		switch requestCount {
		case 1:
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"goal\":\"List current assistant feature coverage\",\"intent\":\"llm_planned\",\"steps\":[{\"tool\":\"nopsai.get_feature_capabilities\",\"args\":{\"query\":\"assistant\",\"include_api_routes\":false},\"reason\":\"Use the current-user capability catalog.\"}],\"success_criteria\":\"Return available feature coverage.\",\"needs_more_tools\":false,\"final_answer\":\"\",\"clarifying_question\":\"\"}"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
		default:
			fmt.Fprint(w, `{"choices":[{"message":{"content":"Feature coverage was checked through hosted MCP. No changes were applied."}}],"usage":{"prompt_tokens":11,"completion_tokens":4,"total_tokens":15}}`)
		}
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
		aaaLocal:           allowActionsForAssistantTest("system.read"),
	}

	result := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		assistantConversation{ID: uuid.New(), DocsVersion: "auto", SelectedLLMProfile: "standard"},
		"What assistant capabilities can I use?",
		"standard",
	)

	if requestCount != 2 {
		t.Fatalf("LLM requests = %d, want planner and synthesis", requestCount)
	}
	if !strings.Contains(plannerPrompt, "available_tools") ||
		!strings.Contains(plannerPrompt, "feature_catalog") ||
		!strings.Contains(plannerPrompt, "nopsai.get_feature_capabilities") {
		t.Fatalf("planner prompt missing tool catalog: %s", plannerPrompt)
	}
	if !strings.Contains(synthesisPrompt, "nopsai.get_feature_capabilities") {
		t.Fatalf("synthesis prompt missing tool evidence: %s", synthesisPrompt)
	}
	if result.Reply != "Feature coverage was checked through hosted MCP. No changes were applied." {
		t.Fatalf("reply = %q", result.Reply)
	}
	if result.ToolCalls[0].Name != assistantLLMPlannerToolName || result.ToolCalls[0].Status != assistantToolStatusSuccess {
		t.Fatalf("planner call = %#v", result.ToolCalls[0])
	}
	if call := assistantFirstToolCall(result.ToolCalls, "nopsai.get_feature_capabilities"); call.Status != assistantToolStatusSuccess {
		t.Fatalf("feature capability call = %#v", call)
	}
	if call := assistantFirstToolCall(result.ToolCalls, assistantLLMToolName); call.Status != assistantToolStatusSuccess {
		t.Fatalf("synthesis call = %#v", call)
	}
}

func TestAssistantPlannerPromptRequiresAIUsageEvidenceForRunTokenQuestion(t *testing.T) {
	runID := "e3850cec-550f-456a-bec8-e67777d71d24"
	app := &App{aaaLocal: allowActionsForAssistantTest("pipeline_run.list", "pipeline_run.read", "pipeline_run.read_logs")}
	plan := assistantBaseTurnPlan("how many token is used by "+runID+" pipelinerun", assistantConversationMemory{})

	prompt := app.buildAssistantPlannerPrompt(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		assistantConversation{ID: uuid.New(), DocsVersion: "auto"},
		plan.Goal,
		plan,
		nil,
		assistantMaxPlanToolCalls,
		1,
	)

	for _, want := range []string{
		`"ai_token_usage_evidence_required": true`,
		`"required_tool": "nopsai.get_monitoring_ai_usage"`,
		runID,
		"Pipeline run status, logs, and failure-analysis tools do not report token counts",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("planner prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAssistantPlannerPromptRequiresPipelineGenerationEvidence(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("pipeline.create")}
	content := "give me a pipeline that has 4 step and last one is approval, pipeline goal is to build and publish docker image based on DDD standards"
	plan := assistantBaseTurnPlan(content, assistantConversationMemory{})

	prompt := app.buildAssistantPlannerPrompt(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		assistantConversation{ID: uuid.New(), DocsVersion: "auto"},
		plan.Goal,
		plan,
		nil,
		assistantMaxPlanToolCalls,
		1,
	)

	for _, want := range []string{
		`"id": "pipeline_generation"`,
		`"required_any_tools": [`,
		`"nopsai.generate_pipeline"`,
		"For pipeline draft requests, use nopsai.generate_pipeline",
		"docker-ddd-publish-approval",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("planner prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAssistantRequestContractsRejectWrongFeatureEvidence(t *testing.T) {
	tests := []struct {
		name      string
		message   string
		wrongTool string
		args      map[string]any
		wantErr   string
	}{
		{
			name:      "feature policy requires capability catalog",
			message:   "do we have any policy to prevent showing envs?",
			wrongTool: "nopsai.search_docs",
			args:      map[string]any{"query": "env policy"},
			wantErr:   "nopsai.get_feature_capabilities",
		},
		{
			name:      "repetitive variables require metadata analyzer",
			message:   "how many repetitive variables are used in all scopes?",
			wrongTool: "nopsai.list_variables_metadata",
			args:      map[string]any{},
			wantErr:   "nopsai.analyze_variable_usage",
		},
		{
			name:      "scope secret counts require secret scope metadata",
			message:   "how many scope do we have and for each how many secrets",
			wrongTool: "nopsai.list_scopes",
			args:      map[string]any{"limit": 200},
			wantErr:   "nopsai.list_secret_scopes",
		},
		{
			name:      "approval pipeline question requires search",
			message:   "give me a pipeline that has approval step",
			wrongTool: "nopsai.list_pipelines",
			args:      map[string]any{"limit": 20},
			wantErr:   "nopsai.search_pipelines",
		},
		{
			name:      "pipeline generation requires generator",
			message:   "give me a pipeline that has 4 step and last one is approval, pipeline goal is to build and publish docker image based on DDD standards",
			wrongTool: "nopsai.search_pipelines",
			args:      map[string]any{"query": "approval"},
			wantErr:   "nopsai.generate_pipeline",
		},
		{
			name:      "pasted yaml validation requires validator",
			message:   "Validate this:\n```yaml\nname: deploy-web\nsteps: []\n```",
			wrongTool: "nopsai.search_docs",
			args:      map[string]any{"query": "pipeline validation"},
			wantErr:   "nopsai.validate_pipeline",
		},
		{
			name:      "explicit api call requires api bridge",
			message:   "GET /v1/system/status",
			wrongTool: "nopsai.search_docs",
			args:      map[string]any{"query": "system status"},
			wantErr:   "nopsai.call_api",
		},
	}

	app := &App{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := assistantBaseTurnPlan(tt.message, assistantConversationMemory{})
			plan.Steps = []assistantPlanStep{{ToolName: tt.wrongTool, Args: tt.args}}

			err := app.validateAssistantToolPlan(
				context.Background(),
				model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
				plan,
			)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want contract error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestAssistantRequestContractsAllowMatchingEvidence(t *testing.T) {
	runID := "e3850cec-550f-456a-bec8-e67777d71d24"
	tests := []struct {
		name    string
		message string
		steps   []assistantPlanStep
		actions []string
	}{
		{
			name:    "run token usage",
			message: "how many token is used by " + runID + " pipelinerun",
			steps: []assistantPlanStep{{
				ToolName: "nopsai.get_monitoring_ai_usage",
				Args:     map[string]any{"run_id": runID},
			}},
			actions: []string{"pipeline_run.list"},
		},
		{
			name:    "feature policy",
			message: "do we have any policy to prevent showing envs?",
			steps: []assistantPlanStep{{
				ToolName: "nopsai.get_feature_capabilities",
				Args:     map[string]any{"query": "secret", "area": "secrets"},
			}},
			actions: []string{"system.read"},
		},
		{
			name:    "scope secret inventory",
			message: "how many scope do we have and for each how many secrets",
			steps: []assistantPlanStep{
				{ToolName: "nopsai.list_scopes", Args: map[string]any{"limit": 200}},
				{ToolName: "nopsai.list_secret_scopes", Args: map[string]any{}},
			},
			actions: []string{"scope.read", "secret.list_metadata"},
		},
		{
			name:    "approval pipeline search",
			message: "give me a pipeline that has approval step",
			steps: []assistantPlanStep{{
				ToolName: "nopsai.search_pipelines",
				Args:     map[string]any{"query": "approval", "limit": 20},
			}},
			actions: []string{"pipeline.list"},
		},
		{
			name:    "pipeline generation",
			message: "give me a pipeline that has 4 step and last one is approval, pipeline goal is to build and publish docker image based on DDD standards",
			steps: []assistantPlanStep{{
				ToolName: "nopsai.generate_pipeline",
				Args: map[string]any{
					"name": "docker-ddd-image",
					"goal": "build and publish docker image based on DDD standards with 4 steps and the last one approval",
				},
			}},
			actions: []string{"pipeline.create"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{aaaLocal: allowActionsForAssistantTest(tt.actions...)}
			plan := assistantBaseTurnPlan(tt.message, assistantConversationMemory{})
			plan.Steps = tt.steps

			if err := app.validateAssistantToolPlan(
				context.Background(),
				model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
				plan,
			); err != nil {
				t.Fatalf("matching contract evidence should pass: %v", err)
			}
		})
	}
}

func TestAssistantRequestContractsRejectDirectFinalPipelineGeneration(t *testing.T) {
	plan := assistantBaseTurnPlan("give me a pipeline that has 4 step and last one is approval, pipeline goal is to build and publish docker image based on DDD standards", assistantConversationMemory{})
	plan.FinalAnswer = "name: docker-ddd-image\nsteps: []"

	err := assistantValidatePlannerFinalAnswer(plan, nil)
	if err == nil || !strings.Contains(err.Error(), "nopsai.generate_pipeline") {
		t.Fatalf("err = %v, want missing pipeline generator evidence", err)
	}
}

func TestAssistantRequestContractsRequireAllEvidenceForFinalAnswer(t *testing.T) {
	plan := assistantBaseTurnPlan("how many scope do we have and for each how many secrets", assistantConversationMemory{})
	scopeOnly := []assistantToolActivity{{
		Name:   "nopsai.list_scopes",
		Status: assistantToolStatusSuccess,
		Input:  map[string]any{"limit": 200},
		Output: map[string]any{"scopes": []string{"default", "prod"}},
	}}

	err := assistantValidatePlannerFinalAnswer(plan, scopeOnly)
	if err == nil || !strings.Contains(err.Error(), "nopsai.list_secret_scopes") {
		t.Fatalf("err = %v, want missing secret-scope evidence", err)
	}

	withSecretScopes := append(scopeOnly, assistantToolActivity{
		Name:   "nopsai.list_secret_scopes",
		Status: assistantToolStatusSuccess,
		Input:  map[string]any{},
		Output: map[string]any{"secret_scopes": []map[string]any{{"scope": "default", "secret_count": 1}}},
	})
	if err := assistantValidatePlannerFinalAnswer(plan, withSecretScopes); err != nil {
		t.Fatalf("complete evidence should pass: %v", err)
	}
}

func TestAssistantLLMPlannerRejectsRunAnalysisForRunTokenUsage(t *testing.T) {
	credentialRef := "credential://system/llm/standard"
	runID := "e3850cec-550f-456a-bec8-e67777d71d24"
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		switch requestCount {
		case 1:
			fmt.Fprintf(w, `{"choices":[{"message":{"content":"{\"goal\":\"Analyze run\",\"intent\":\"analyze_run\",\"steps\":[{\"tool\":\"nopsai.get_pipeline_run\",\"args\":{\"run_id\":\"%s\"},\"reason\":\"Read run status.\"},{\"tool\":\"nopsai.get_pipeline_run_logs\",\"args\":{\"run_id\":\"%s\",\"limit\":120},\"reason\":\"Read logs.\"},{\"tool\":\"nopsai.analyze_pipeline_run_failure\",\"args\":{\"run_id\":\"%s\"},\"reason\":\"Analyze failure.\"}],\"success_criteria\":\"Explain run status.\",\"needs_more_tools\":false,\"final_answer\":\"\",\"clarifying_question\":\"\"}"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`, runID, runID, runID)
		default:
			fmt.Fprint(w, `{"choices":[{"message":{"content":"I could not answer token usage without AI usage analytics evidence. No changes were applied."}}],"usage":{"prompt_tokens":11,"completion_tokens":4,"total_tokens":15}}`)
		}
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
		aaaLocal:           allowActionsForAssistantTest("pipeline_run.list", "pipeline_run.read", "pipeline_run.read_logs"),
	}

	result := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		assistantConversation{ID: uuid.New(), DocsVersion: "auto", SelectedLLMProfile: "standard"},
		"how many token is used by "+runID+" pipelinerun",
		"standard",
	)

	for _, call := range result.ToolCalls {
		switch call.Name {
		case "nopsai.get_pipeline_run", "nopsai.get_pipeline_run_logs", "nopsai.analyze_pipeline_run_failure":
			t.Fatalf("run-analysis tool should not execute for token usage request: %#v", call)
		}
	}
	denial, ok := assistantFirstPlanDenial(result.ToolCalls)
	if !ok {
		t.Fatalf("missing semantic plan denial: %#v", result.ToolCalls)
	}
	if !strings.Contains(assistantOutputString(denial.Output, "error"), "ai_token_usage") ||
		!strings.Contains(assistantOutputString(denial.Output, "error"), "nopsai.get_monitoring_ai_usage") {
		t.Fatalf("denial should explain token usage evidence requirement: %#v", denial.Output)
	}
	if !strings.Contains(result.Reply, "No changes were applied") {
		t.Fatalf("reply should remain fail-closed: %q", result.Reply)
	}
}

func TestAssistantLLMPlannerRejectsFinalTokenAnswerWithoutUsageEvidence(t *testing.T) {
	credentialRef := "credential://system/llm/standard"
	runID := "e3850cec-550f-456a-bec8-e67777d71d24"
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		switch requestCount {
		case 1:
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"goal\":\"Answer token usage\",\"intent\":\"ai_token_usage\",\"steps\":[],\"success_criteria\":\"Return tokens.\",\"needs_more_tools\":false,\"final_answer\":\"This run used 42 tokens.\",\"clarifying_question\":\"\"}"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
		default:
			fmt.Fprint(w, `{"choices":[{"message":{"content":"I could not answer token usage without AI usage analytics evidence. No changes were applied."}}],"usage":{"prompt_tokens":11,"completion_tokens":4,"total_tokens":15}}`)
		}
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
		aaaLocal:           allowActionsForAssistantTest("pipeline_run.list"),
	}

	result := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		assistantConversation{ID: uuid.New(), DocsVersion: "auto", SelectedLLMProfile: "standard"},
		"how many token is used by "+runID+" pipelinerun",
		"standard",
	)

	if strings.Contains(result.Reply, "42 tokens") {
		t.Fatalf("reply should not use unsupported final answer: %q", result.Reply)
	}
	denial, ok := assistantFirstPlanDenial(result.ToolCalls)
	if !ok {
		t.Fatalf("missing plan denial for unsupported final answer: %#v", result.ToolCalls)
	}
	if !strings.Contains(assistantOutputString(denial.Output, "error"), "without successful evidence from") ||
		!strings.Contains(assistantOutputString(denial.Output, "error"), "nopsai.get_monitoring_ai_usage") {
		t.Fatalf("denial output = %#v", denial.Output)
	}
}

func TestAssistantLLMPlannerBlocksUnconfirmedMutation(t *testing.T) {
	enabled := true
	credentialRef := "credential://system/llm/standard"
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		switch requestCount {
		case 1:
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"goal\":\"Pause runner dispatch\",\"intent\":\"llm_planned\",\"steps\":[{\"tool\":\"nopsai.update_runner_dispatch\",\"args\":{\"runner_id\":\"runner-a\",\"allow_dispatch\":false,\"confirm\":true},\"reason\":\"Pause dispatch for the named runner.\"}],\"success_criteria\":\"Dispatch should only pause after explicit user confirmation.\",\"needs_more_tools\":false,\"final_answer\":\"\",\"clarifying_question\":\"\"}"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
		default:
			fmt.Fprint(w, `{"choices":[{"message":{"content":"Explicit confirmation is required before pausing runner dispatch. No changes were applied."}}],"usage":{"prompt_tokens":11,"completion_tokens":4,"total_tokens":15}}`)
		}
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
			Assistant: config.AssistantConfig{
				Features: config.AssistantFeaturesConfig{ActionExecution: &enabled},
			},
		},
		httpClient:         server.Client(),
		credentialResolver: staticCredentialResolver{credentialRef: "secret"},
		aaaLocal:           allowActionsForAssistantTest("system.update"),
	}

	result := app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		assistantConversation{ID: uuid.New(), DocsVersion: "auto", SelectedLLMProfile: "standard"},
		"Pause runner runner-a",
		"standard",
	)

	for _, call := range result.ToolCalls {
		if call.Name == "nopsai.update_runner_dispatch" {
			t.Fatalf("mutation tool should not execute: %#v", call)
		}
	}
	denial, ok := assistantFirstPlanDenial(result.ToolCalls)
	if !ok {
		t.Fatalf("missing plan denial: %#v", result.ToolCalls)
	}
	if !strings.Contains(assistantOutputString(denial.Output, "error"), "user did not explicitly confirm") {
		t.Fatalf("denial output = %#v", denial.Output)
	}
	if !strings.Contains(result.Reply, "Explicit confirmation is required") || !strings.Contains(result.Reply, "No changes were applied") {
		t.Fatalf("reply = %q", result.Reply)
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

	err := app.validateAssistantToolPlan(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		plan,
	)
	if err == nil || !strings.Contains(err.Error(), "unavailable tool") {
		t.Fatalf("err = %v, want unavailable tool validation", err)
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
	plan.UserConfirmed = true
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

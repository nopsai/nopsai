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
		!strings.Contains(plannerPrompt, "input_schema") ||
		!strings.Contains(plannerPrompt, "nopsai.get_feature_capabilities") {
		t.Fatalf("planner prompt missing live tool schema catalog: %s", plannerPrompt)
	}
	if strings.Contains(plannerPrompt, "feature_catalog") || strings.Contains(plannerPrompt, "request_requirements") {
		t.Fatalf("planner prompt should not include static routing catalogs: %s", plannerPrompt)
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
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"goal\":\"Prepare pipeline YAML named deploy-web\",\"intent\":\"propose_pipeline_create\",\"steps\":[{\"tool\":\"nopsai.propose_pipeline_create\",\"args\":{\"name\":\"deploy-web\",\"yaml\":\"name: deploy-web\\ncontainer_image: alpine:3.20\\nsteps:\\n  - name: plan\\n    script: echo ok\\n\",\"message\":\"Create NopsAI pipeline deploy-web\"},\"reason\":\"Validate the LLM-drafted YAML and produce a GitOps file plan.\"}],\"success_criteria\":\"Return a GitOps-ready pipeline create plan without applying it.\",\"needs_more_tools\":false,\"final_answer\":\"\",\"clarifying_question\":\"\"}"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
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
	if !strings.Contains(result.Reply, "I prepared a GitOps-ready pipeline write plan") || !strings.Contains(result.Reply, "No changes were applied") {
		t.Fatalf("reply should fall back to deterministic proposal-safe summary: %q", result.Reply)
	}
	call := assistantFirstToolCall(result.ToolCalls, assistantLLMToolName)
	if call.Status != assistantToolStatusSuccess || !assistantOutputBool(call.Output, "quality_fallback") {
		t.Fatalf("llm quality fallback not recorded: status=%q output=%#v", call.Status, call.Output)
	}
}

func TestAssistantOrchestrationFailsClosedWhenLLMProviderFails(t *testing.T) {
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

	if !strings.Contains(result.Reply, "assistant LLM planner was unavailable or returned an invalid plan") || !strings.Contains(result.Reply, "No changes were applied") {
		t.Fatalf("reply should fail closed without a static fallback: %q", result.Reply)
	}
	call := assistantFirstToolCall(result.ToolCalls, assistantLLMPlannerToolName)
	if call.Status != assistantToolStatusError {
		t.Fatalf("planner status = %q, want error", call.Status)
	}
	if assistantOutputString(call.Output, "fallback_reason") == "" {
		t.Fatalf("fallback reason missing: %#v", call.Output)
	}
	for _, toolCall := range result.ToolCalls {
		if strings.HasPrefix(toolCall.Name, "nopsai.") && toolCall.Name != assistantLLMPlannerToolName {
			t.Fatalf("unexpected hosted MCP tool call after planner failure: %#v", toolCall)
		}
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
		!strings.Contains(plannerPrompt, "input_schema") ||
		!strings.Contains(plannerPrompt, "nopsai.get_feature_capabilities") {
		t.Fatalf("planner prompt missing live tool schema catalog: %s", plannerPrompt)
	}
	if strings.Contains(plannerPrompt, "feature_catalog") || strings.Contains(plannerPrompt, "request_requirements") {
		t.Fatalf("planner prompt should not include static routing catalogs: %s", plannerPrompt)
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

func TestAssistantPlannerPromptUsesLiveToolSchemasWithoutStaticRouting(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("pipeline.read", "pipeline.create", "system.read")}
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
		"available_tools",
		"input_schema",
		"nopsai.validate_pipeline",
		"nopsai.propose_pipeline_create",
		"Use only tool names from available_tools",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("planner prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, blocked := range []string{
		"feature_catalog",
		"request_requirements",
		"pipeline_generation_templates",
		"nopsai.generate_pipeline",
		"docker-ddd-publish-approval",
		"ai_token_usage_evidence_required",
	} {
		if strings.Contains(prompt, blocked) {
			t.Fatalf("planner prompt includes static routing artifact %q:\n%s", blocked, prompt)
		}
	}
}

func TestAssistantPlannerPromptStaysCompactForFullToolCatalog(t *testing.T) {
	app := &App{aaaLocal: stubAAAAuthorizer{
		checkFn: func(context.Context, model.Subject, string, model.ResourceRef, map[string]any) (model.Decision, error) {
			return model.Decision{Allowed: true}, nil
		},
	}}
	plan := assistantBaseTurnPlan("how many pipelines we have", assistantConversationMemory{})

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

	if len(prompt) > 60000 {
		t.Fatalf("planner prompt length = %d bytes, want <= 60000", len(prompt))
	}
	if strings.Contains(prompt, "additionalProperties") {
		t.Fatalf("planner prompt should use compact schema summaries, not full JSON schemas")
	}
}

func TestAssistantPlanValidationChecksToolInputSchema(t *testing.T) {
	app := &App{aaaLocal: allowActionsForAssistantTest("pipeline.read")}
	plan := assistantTurnPlan{
		Intent: "validate_pipeline",
		Goal:   "Validate pipeline YAML",
		Steps: []assistantPlanStep{{
			ToolName: "nopsai.validate_pipeline",
			Args:     map[string]any{"yaml": 42},
		}},
	}

	err := app.validateAssistantToolPlan(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		plan,
	)
	if err == nil || !strings.Contains(err.Error(), "yaml must be string") {
		t.Fatalf("err = %v, want schema type validation", err)
	}
}

func TestAssistantValidatePlannerFinalAnswerRequiresSuccessfulEvidence(t *testing.T) {
	plan := assistantBaseTurnPlan("What changed?", assistantConversationMemory{})

	if err := assistantValidatePlannerFinalAnswer(plan, nil); err == nil {
		t.Fatal("final answer without evidence should fail")
	}

	erroredEvidence := []assistantToolActivity{{
		Name:   "nopsai.search_docs",
		Status: assistantToolStatusError,
		Output: map[string]any{"error": "docs unavailable"},
	}}
	if err := assistantValidatePlannerFinalAnswer(plan, erroredEvidence); err == nil {
		t.Fatal("final answer with only failed evidence should fail")
	}

	successfulEvidence := []assistantToolActivity{{
		Name:   "nopsai.search_docs",
		Status: assistantToolStatusSuccess,
		Output: map[string]any{"results": []map[string]any{}},
	}}
	if err := assistantValidatePlannerFinalAnswer(plan, successfulEvidence); err != nil {
		t.Fatalf("final answer with successful MCP evidence should pass: %v", err)
	}
}

func TestAssistantLLMPlannerRejectsFinalAnswerWithoutToolEvidence(t *testing.T) {
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
	if !strings.Contains(assistantOutputString(denial.Output, "error"), "without successful hosted MCP evidence") {
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

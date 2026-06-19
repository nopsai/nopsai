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
	reply := composeAIUsageReply([]assistantToolActivity{{
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

func TestAssistantAIUsageReplyAsksFollowUpWhenNoEventsAreVisible(t *testing.T) {
	reply := composeAIUsageReply([]assistantToolActivity{{
		Name:   "nopsai.get_monitoring_ai_usage",
		Status: assistantToolStatusSuccess,
		Output: map[string]any{
			"total_tokens":            0,
			"total_prompt_tokens":     0,
			"total_completion_tokens": 0,
			"by_pipeline":             []map[string]any{},
			"top_token_runs":          []map[string]any{},
		},
	}})

	for _, want := range []string{"default monitoring window", "time range", "pipeline", "run ID"} {
		if !strings.Contains(reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, reply)
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
		fmt.Fprint(w, `{"choices":[{"message":{"content":"LLM final answer"}}],"usage":{"prompt_tokens":11,"completion_tokens":4,"total_tokens":15}}`)
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

	if result.Reply != "LLM final answer" {
		t.Fatalf("reply = %q, want LLM answer", result.Reply)
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

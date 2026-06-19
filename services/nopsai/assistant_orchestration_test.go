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
	app := &App{aaaLocal: allowActionsForAssistantTest("system.update")}

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

	if !strings.Contains(result.Reply, "Draft YAML") || !strings.Contains(result.Reply, "LLM synthesis was unavailable") {
		t.Fatalf("reply did not fall back to deterministic summary: %q", result.Reply)
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

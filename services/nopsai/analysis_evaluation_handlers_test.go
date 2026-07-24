package nopsai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/config"
	aaamodel "nopsai/services/aaa/pkg/model"
)

func TestHandleEvaluateAnalysisUsesDirectLLMProfileWithoutAssistantPlanner(t *testing.T) {
	credentialRef := "credential://system/llm/analysis"
	requestCount := 0
	var providerPrompt string
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
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(payload.Messages) != 1 || payload.Messages[0].Role != "user" {
			t.Errorf("messages = %#v, want one direct user prompt", payload.Messages)
		}
		if len(payload.Messages) > 0 {
			providerPrompt = payload.Messages[0].Content
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"summary\":\"run failed at test\",\"problem\":{\"title\":\"Tests failed\",\"detail\":\"The first failed task is unit tests.\"},\"score\":{\"detail\":\"A high finding reduced health.\",\"drivers\":[\"test failure\"]},\"fixes\":[{\"title\":\"Inspect test logs\",\"detail\":\"Open the failed task logs and fix assertions.\",\"priority\":\"now\",\"safe_action\":\"Open logs\"}],\"evidence_needed\":[],\"confidence\":0.82}"}}],"usage":{"prompt_tokens":21,"completion_tokens":9,"total_tokens":30}}`)
	}))
	defer server.Close()

	app := &App{
		cfg: &config.Config{
			Assistant:         config.AssistantConfig{Enabled: true},
			LLMDefaultProfile: "analysis",
			LLMProfiles: map[string]config.LLMProfile{
				"analysis": {
					Provider:      config.LLMProviderOpenAI,
					Model:         "gpt-test",
					BaseURL:       server.URL + "/v1",
					CredentialRef: credentialRef,
				},
			},
		},
		httpClient:         server.Client(),
		credentialResolver: staticCredentialResolver{credentialRef: "secret"},
	}

	body := bytes.NewBufferString(`{"subject_type":"run","subject_id":"run-1","scope":"prod","selected_llm_profile":"analysis","prompt":"Use this redacted structured snapshot only."}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/analysis/evaluate", body)
	req = req.WithContext(withAAASubject(req.Context(), aaamodel.Subject{Type: aaamodel.SubjectTypeUser, ID: "alice"}))
	rec := httptest.NewRecorder()

	app.handleEvaluateAnalysis(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if requestCount != 1 {
		t.Fatalf("LLM requests = %d, want one direct evaluation request", requestCount)
	}
	if !strings.Contains(providerPrompt, "redacted structured snapshot") {
		t.Fatalf("provider prompt = %q, want caller-supplied snapshot prompt", providerPrompt)
	}
	if strings.Contains(providerPrompt, "available_tools") || strings.Contains(providerPrompt, "hosted MCP") {
		t.Fatalf("provider prompt should not include assistant planner or hosted MCP tool catalog: %s", providerPrompt)
	}

	var payload analysisEvaluationResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ProfileName != "analysis" || payload.Model != "gpt-test" {
		t.Fatalf("profile/model = %q/%q, want analysis/gpt-test", payload.ProfileName, payload.Model)
	}
	if !strings.Contains(payload.Content, `"problem"`) {
		t.Fatalf("content = %q, want structured model text", payload.Content)
	}
	if payload.Usage.TotalTokens != 30 {
		t.Fatalf("total tokens = %d, want 30", payload.Usage.TotalTokens)
	}
}

func TestAnalysisEvaluationRouteIsAuthenticatedOnly(t *testing.T) {
	if !isAuthenticatedOnlyPath("/v1/analysis/evaluate") {
		t.Fatal("analysis evaluation route must be authenticated-only")
	}
}

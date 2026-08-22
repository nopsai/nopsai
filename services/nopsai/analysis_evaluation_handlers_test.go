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

func TestAnalysisEvaluationPromptCarriesServerEvidence(t *testing.T) {
	analysis := map[string]any{
		"health_score": 60,
		"data_sources": []string{"/v1/monitoring/summary?teamId=7"},
		"limitations":  []string{"Only evidence the current user is allowed to read contributes."},
		"scores":       []map[string]any{{"category": "reliability", "score": 55}},
		"findings": []map[string]any{{
			"severity":   "critical",
			"category":   "reliability",
			"title":      "45% of runs failed in the last 30 days",
			"summary":    "prose the model should not echo",
			"evidence":   []map[string]any{{"label": "Failure rate", "value": "45%"}},
			"confidence": 0.9,
		}},
	}

	prompt, sources := analysisEvaluationPromptWithEvidence("Client snapshot only.", analysis)

	if !strings.Contains(prompt, "NOPSAI_SERVER_EVIDENCE") {
		t.Fatalf("prompt is not grounded:\n%s", prompt)
	}
	if !strings.Contains(prompt, "45% of runs failed in the last 30 days") {
		t.Fatalf("prompt is missing the finding:\n%s", prompt)
	}
	if strings.Contains(prompt, "prose the model should not echo") {
		t.Fatalf("prompt carries finding prose it does not need:\n%s", prompt)
	}
	if len(sources) != 1 || sources[0] != "/v1/monitoring/summary?teamId=7" {
		t.Fatalf("sources = %v, want the evidence paths", sources)
	}
	if !strings.HasPrefix(prompt, "Client snapshot only.") {
		t.Fatal("the client prompt must stay intact ahead of the evidence")
	}
}

// An analysis that could not read anything scores nothing, and appending it would
// add tokens without adding facts.
func TestAnalysisEvaluationPromptSkipsUnscoredAnalysis(t *testing.T) {
	prompt, sources := analysisEvaluationPromptWithEvidence("Client snapshot only.", map[string]any{
		"health_score": nil,
		"limitations":  []string{"summary could not be read: status 403"},
	})

	if prompt != "Client snapshot only." || len(sources) != 0 {
		t.Fatalf("prompt = %q, sources = %v; want the ungrounded prompt", prompt, sources)
	}
}

func TestAnalysisEvaluationPromptDropsEvidenceThatWouldExceedTheInputLimit(t *testing.T) {
	huge := make([]map[string]any, 0, analysisEvaluationMaxFindings)
	for index := 0; index < analysisEvaluationMaxFindings; index++ {
		huge = append(huge, map[string]any{
			"severity": "high",
			"title":    strings.Repeat("x", 20000),
		})
	}
	prompt, sources := analysisEvaluationPromptWithEvidence("Client snapshot only.", map[string]any{
		"health_score": 40,
		"findings":     huge,
	})

	if prompt != "Client snapshot only." || len(sources) != 0 {
		t.Fatalf("oversized evidence must be dropped whole, got %d bytes", len(prompt))
	}
}

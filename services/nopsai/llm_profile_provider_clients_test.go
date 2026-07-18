package nopsai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/config"
)

func TestTestOpenAICompatibleProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("OpenAI-Organization"); got != "org-test" {
			t.Errorf("OpenAI-Organization = %q", got)
		}
		if got := r.Header.Get("OpenAI-Project"); got != "project-test" {
			t.Errorf("OpenAI-Project = %q", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload["model"] != "gpt-test" {
			t.Errorf("model = %#v", payload["model"])
		}
		if payload["max_completion_tokens"] != float64(16) {
			t.Errorf("max_completion_tokens = %#v", payload["max_completion_tokens"])
		}
		if _, ok := payload["max_tokens"]; ok {
			t.Errorf("max_tokens should be omitted: %#v", payload)
		}
		if _, ok := payload["temperature"]; ok {
			t.Errorf("temperature should be omitted: %#v", payload)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":[{"type":"text","text":"ok"},{"type":"text","text":"ready"}]}}]}`)
	}))
	defer server.Close()

	reply, err := testOpenAICompatibleProfile(t.Context(), config.LLMProfile{
		Provider: config.LLMProviderOpenAI,
		Model:    "gpt-test",
		BaseURL:  server.URL + "/v1",
		Extra: map[string]string{
			"organization": "org-test",
			"project":      "project-test",
		},
	}, "secret")
	if err != nil || reply != "ok\nready" {
		t.Fatalf("testOpenAICompatibleProfile() = %q, %v", reply, err)
	}
}

func TestTestLMStudioProfilePreservesMaxOutputTokens(t *testing.T) {
	tests := []struct {
		name             string
		maxTokens        int
		reasoning        string
		wantMaxTokens    any
		wantReasoning    any
		reasoningOmitted bool
	}{
		{name: "defaults omitted", reasoningOmitted: true},
		{name: "off reasoning omitted", reasoning: "off", reasoningOmitted: true},
		{name: "configured max tokens", maxTokens: 64, wantMaxTokens: float64(64), reasoningOmitted: true},
		{name: "enabled reasoning", reasoning: "high", wantReasoning: "high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/chat" {
					t.Errorf("path = %q", r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer secret" {
					t.Errorf("Authorization = %q", got)
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("decode request: %v", err)
				}
				if tt.wantMaxTokens == nil {
					if _, ok := payload["max_output_tokens"]; ok {
						t.Errorf("max_output_tokens should be omitted: %#v", payload)
					}
				} else if payload["max_output_tokens"] != tt.wantMaxTokens {
					t.Errorf("max_output_tokens = %#v, want %#v", payload["max_output_tokens"], tt.wantMaxTokens)
				}
				if tt.reasoningOmitted {
					if _, ok := payload["reasoning"]; ok {
						t.Errorf("reasoning should be omitted: %#v", payload)
					}
				} else if payload["reasoning"] != tt.wantReasoning {
					t.Errorf("reasoning = %#v, want %#v", payload["reasoning"], tt.wantReasoning)
				}
				fmt.Fprint(w, `{"output":[{"type":"message","content":"ok"}]}`)
			}))
			defer server.Close()

			reply, err := testLMStudioProfile(t.Context(), config.LLMProfile{
				Provider:  config.LLMProviderLMStudio,
				Model:     "local-model",
				BaseURL:   server.URL,
				Reasoning: tt.reasoning,
				MaxTokens: tt.maxTokens,
			}, "secret")
			if err != nil || reply != "ok" {
				t.Fatalf("testLMStudioProfile() = %q, %v", reply, err)
			}
		})
	}
}

func TestNopsaiOpenAIMessageTextRejectsInvalidContent(t *testing.T) {
	tests := []json.RawMessage{
		json.RawMessage(`""`),
		json.RawMessage(`[]`),
		json.RawMessage(`{"text":"unsupported"}`),
	}
	for _, raw := range tests {
		if _, err := nopsaiOpenAIMessageText(raw); err == nil {
			t.Fatalf("nopsaiOpenAIMessageText(%s) unexpectedly succeeded", raw)
		}
	}
}

func TestTestAnthropicProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "secret" {
			t.Errorf("x-api-key = %q", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if _, ok := payload["temperature"]; ok {
			t.Errorf("temperature should be omitted: %#v", payload)
		}
		fmt.Fprint(w, `{"content":[{"type":"text","text":"ok"}]}`)
	}))
	defer server.Close()

	reply, err := testAnthropicProfile(t.Context(), config.LLMProfile{
		Provider: config.LLMProviderAnthropic,
		Model:    "claude-test",
		BaseURL:  server.URL,
	}, "secret")
	if err != nil || reply != "ok" {
		t.Fatalf("testAnthropicProfile() = %q, %v", reply, err)
	}
}

func TestTestAzureOpenAIProfileUsesLegacyRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openai/deployments/deploy/chat/completions" || r.URL.Query().Get("api-version") != "2024-10-21" {
			t.Errorf("URL = %s", r.URL.String())
		}
		if got := r.Header.Get("api-key"); got != "secret" {
			t.Errorf("api-key = %q", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload["max_completion_tokens"] != float64(16) {
			t.Errorf("max_completion_tokens = %#v", payload["max_completion_tokens"])
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer server.Close()

	reply, err := testAzureOpenAIProfile(t.Context(), config.LLMProfile{
		Provider: config.LLMProviderAzureOpenAI,
		BaseURL:  server.URL,
		Extra: map[string]string{
			"deployment":  "deploy",
			"api_version": "2024-10-21",
		},
	}, "secret")
	if err != nil || reply != "ok" {
		t.Fatalf("testAzureOpenAIProfile() = %q, %v", reply, err)
	}
}

func TestProviderConnectionTestsReturnErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "denied", http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := testOpenAICompatibleProfile(t.Context(), config.LLMProfile{
		Provider: config.LLMProviderOpenAI,
		Model:    "gpt-test",
		BaseURL:  server.URL,
	}, "secret")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("error = %v", err)
	}
}

func TestAnthropicConnectionTestReturnsErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "denied", http.StatusForbidden)
	}))
	defer server.Close()

	_, err := testAnthropicProfile(t.Context(), config.LLMProfile{
		Provider: config.LLMProviderAnthropic,
		Model:    "claude-test",
		BaseURL:  server.URL + "/v1/messages",
	}, "secret")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("error = %v", err)
	}
}

func TestNopsaiProviderEndpointBuilders(t *testing.T) {
	if got := buildNopsaiOpenAIChatURL("https://example.test/v1"); got != "https://example.test/v1/chat/completions" {
		t.Fatalf("OpenAI URL = %q", got)
	}
	if got := buildNopsaiOpenAIChatURL("https://example.test/v1/chat/completions"); got != "https://example.test/v1/chat/completions" {
		t.Fatalf("OpenAI complete URL = %q", got)
	}
	if got := buildNopsaiAzureOpenAIChatURL("https://resource.openai.azure.com", "", ""); got != "https://resource.openai.azure.com/openai/v1/chat/completions" {
		t.Fatalf("Azure URL = %q", got)
	}
	if got := buildNopsaiAzureOpenAIChatURL("https://resource.openai.azure.com/openai/v1", "", ""); got != "https://resource.openai.azure.com/openai/v1/chat/completions" {
		t.Fatalf("Azure v1 URL = %q", got)
	}
	if got := buildNopsaiAzureOpenAIChatURL("https://resource.openai.azure.com", "deploy", ""); !strings.Contains(got, "/deploy/chat/completions?api-version=2024-10-21") {
		t.Fatalf("Azure legacy URL = %q", got)
	}
}

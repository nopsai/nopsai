package llmclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/config"
)

func TestOpenAICompatibleCompletion(t *testing.T) {
	var request openAIChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"answer"}}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`)
	}))
	defer server.Close()

	completion, err := New(Options{
		Provider:  config.LLMProviderOpenAI,
		Profile:   "standard",
		APIKey:    "secret",
		Model:     "gpt-test",
		BaseURL:   server.URL + "/v1",
		MaxTokens: 123,
	}).Complete(t.Context(), "hello")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if completion.Text != "answer" {
		t.Fatalf("text = %q", completion.Text)
	}
	if request.Model != "gpt-test" || request.MaxCompletionTokens != 123 || request.MaxTokens != 0 {
		t.Fatalf("request = %#v", request)
	}
	if completion.Usage.Provider != config.LLMProviderOpenAI || completion.Usage.Profile != "standard" || completion.Usage.TotalTokens != 7 {
		t.Fatalf("usage = %#v", completion.Usage)
	}
}

func TestAnthropicCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %q, want /v1/messages", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "secret" {
			t.Errorf("x-api-key = %q", got)
		}
		fmt.Fprint(w, `{"content":[{"type":"text","text":"anthropic answer"}],"usage":{"input_tokens":3,"output_tokens":2}}`)
	}))
	defer server.Close()

	completion, err := New(Options{
		Provider: config.LLMProviderAnthropic,
		APIKey:   "secret",
		Model:    "claude-test",
		BaseURL:  server.URL,
	}).Complete(t.Context(), "hello")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if completion.Text != "anthropic answer" || completion.Usage.TotalTokens != 5 {
		t.Fatalf("completion = %#v", completion)
	}
}

func TestLMStudioCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/models":
			fmt.Fprint(w, `{"models":[{"type":"llm","key":"local-model","loaded_instances":[{"id":"local-model"}]}]}`)
		case "/api/v1/chat":
			var request struct {
				Model           string `json:"model"`
				Input           string `json:"input"`
				Reasoning       string `json:"reasoning"`
				MaxOutputTokens int    `json:"max_output_tokens"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode request: %v", err)
			}
			if request.Model != "local-model" ||
				request.Input != "hello" ||
				request.Reasoning != "off" ||
				request.MaxOutputTokens != 0 {
				t.Errorf("request = %#v", request)
			}
			fmt.Fprint(w, `{"output":[{"type":"message","content":"local answer"}],"usage":{"input_tokens":4,"output_tokens":3,"total_tokens":7}}`)
		default:
			t.Errorf("unexpected path = %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	completion, err := New(Options{
		Provider:  config.LLMProviderLMStudio,
		Model:     "local-model",
		BaseURL:   server.URL,
		Reasoning: "off",
	}).Complete(t.Context(), "hello")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if completion.Text != "local answer" || completion.Usage.TotalTokens != 7 {
		t.Fatalf("completion = %#v", completion)
	}
}

func TestLMStudioCompletionUsesConfiguredMaxTokens(t *testing.T) {
	var maxOutputTokens int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/models":
			fmt.Fprint(w, `{"models":[{"type":"llm","key":"local-model","loaded_instances":[{"id":"local-model"}]}]}`)
		case "/api/v1/chat":
			var request struct {
				MaxOutputTokens int `json:"max_output_tokens"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode request: %v", err)
			}
			maxOutputTokens = request.MaxOutputTokens
			fmt.Fprint(w, `{"output":[{"type":"message","content":"local answer"}]}`)
		default:
			t.Errorf("unexpected path = %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := New(Options{
		Provider:  config.LLMProviderLMStudio,
		Model:     "local-model",
		BaseURL:   server.URL,
		MaxTokens: 128,
	}).Complete(t.Context(), "hello")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if maxOutputTokens != 128 {
		t.Fatalf("max_output_tokens = %d, want 128", maxOutputTokens)
	}
}

func TestCompletionErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "denied", http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := New(Options{Provider: config.LLMProviderMistral, Model: "mistral", BaseURL: server.URL}).Complete(t.Context(), "hello")
	if err == nil || !strings.Contains(err.Error(), "non-2xx") {
		t.Fatalf("error = %v, want non-2xx", err)
	}
}

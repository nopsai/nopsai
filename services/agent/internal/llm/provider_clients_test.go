package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appconfig "nopsai/config"
	"nopsai/pkg/proto"
)

func TestProviderRegistryDispatchesAllSupportedProviders(t *testing.T) {
	tests := []struct {
		provider string
		wantType string
	}{
		{provider: appconfig.LLMProviderGemini, wantType: "*llm.geminiClient"},
		{provider: appconfig.LLMProviderLMStudio, wantType: "*llm.lmStudioClient"},
		{provider: appconfig.LLMProviderOpenAI, wantType: "*llm.openAICompatibleClient"},
		{provider: appconfig.LLMProviderAnthropic, wantType: "*llm.anthropicClient"},
		{provider: appconfig.LLMProviderGroq, wantType: "*llm.openAICompatibleClient"},
		{provider: appconfig.LLMProviderMistral, wantType: "*llm.openAICompatibleClient"},
		{provider: appconfig.LLMProviderOllama, wantType: "*llm.openAICompatibleClient"},
		{provider: appconfig.LLMProviderOpenRouter, wantType: "*llm.openAICompatibleClient"},
		{provider: appconfig.LLMProviderAzureOpenAI, wantType: "*llm.azureOpenAIClient"},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			client := NewLLMClientWithOptions(LLMClientOptions{Provider: tt.provider})
			if got := client.providerClient.Name(); got != tt.provider {
				t.Fatalf("Name() = %q, want %q", got, tt.provider)
			}
			if got := fmt.Sprintf("%T", client.providerClient); got != tt.wantType {
				t.Fatalf("provider type = %q, want %q", got, tt.wantType)
			}
		})
	}
}

func TestLMStudioClientPreservesMaxTokens(t *testing.T) {
	client := NewLLMClientWithOptions(LLMClientOptions{Provider: appconfig.LLMProviderLMStudio})
	provider, ok := client.providerClient.(*lmStudioClient)
	if !ok {
		t.Fatalf("provider client = %T", client.providerClient)
	}
	if provider.maxTokens != 0 {
		t.Fatalf("maxTokens = %d, want 0", provider.maxTokens)
	}

	client = NewLLMClientWithOptions(LLMClientOptions{
		Provider:  appconfig.LLMProviderLMStudio,
		MaxTokens: 123,
	})
	provider, ok = client.providerClient.(*lmStudioClient)
	if !ok {
		t.Fatalf("provider client = %T", client.providerClient)
	}
	if provider.maxTokens != 123 {
		t.Fatalf("maxTokens = %d, want 123", provider.maxTokens)
	}
}

func TestOpenAICompatibleClientRequestAndUsage(t *testing.T) {
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
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"true"}}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`)
	}))
	defer server.Close()

	temperature := 0.25
	client := NewLLMClientWithOptions(LLMClientOptions{
		Provider:    appconfig.LLMProviderGroq,
		Profile:     "fast",
		APIKey:      "secret",
		Model:       "llama-test",
		BaseURL:     server.URL + "/v1",
		MaxTokens:   321,
		Temperature: &temperature,
	})
	collector := NewUsageCollector()
	response, err := client.providerClient.Complete(ContextWithUsageCollector(t.Context(), collector), "decide")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response != "true" {
		t.Fatalf("response = %q", response)
	}
	if request.Model != "llama-test" ||
		request.MaxCompletionTokens != 321 ||
		request.MaxTokens != 0 ||
		request.Temperature == nil ||
		*request.Temperature != temperature {
		t.Fatalf("request = %#v", request)
	}
	if len(request.Messages) != 1 || request.Messages[0].Content != "decide" {
		t.Fatalf("messages = %#v", request.Messages)
	}
	usages := collector.Snapshot()
	if len(usages) != 1 || usages[0].Provider != appconfig.LLMProviderGroq || usages[0].TotalTokens != 9 {
		t.Fatalf("usage = %#v", usages)
	}
}

func TestOpenRouterClientAddsAttributionHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("HTTP-Referer"); got != "https://nopsai.example.com" {
			t.Errorf("HTTP-Referer = %q", got)
		}
		if got := r.Header.Get("X-Title"); got != "NopsAI" {
			t.Errorf("X-Title = %q", got)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":[{"type":"text","text":"ok"}]}}]}`)
	}))
	defer server.Close()

	client := NewLLMClientWithOptions(LLMClientOptions{
		Provider: appconfig.LLMProviderOpenRouter,
		APIKey:   "secret",
		Model:    "openai/test",
		BaseURL:  server.URL,
		Extra: map[string]string{
			"http_referer": "https://nopsai.example.com",
			"x_title":      "NopsAI",
		},
	})
	response, err := client.providerClient.Complete(t.Context(), "hello")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response != "ok" {
		t.Fatalf("response = %q", response)
	}
}

func TestOpenAICompatibleClientUsesLegacyTokenFieldWhereRequired(t *testing.T) {
	var request openAIChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer server.Close()

	client := NewLLMClientWithOptions(LLMClientOptions{
		Provider:  appconfig.LLMProviderMistral,
		APIKey:    "secret",
		Model:     "mistral-test",
		BaseURL:   server.URL,
		MaxTokens: 123,
	})
	if response, err := client.providerClient.Complete(t.Context(), "hello"); err != nil || response != "ok" {
		t.Fatalf("Complete() = %q, %v", response, err)
	}
	if request.MaxTokens != 123 || request.MaxCompletionTokens != 0 {
		t.Fatalf("request token fields = %#v", request)
	}
	if request.Temperature != nil {
		t.Fatalf("temperature = %#v, want omitted", request.Temperature)
	}
}

func TestOpenAICompatibleClientErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		code int
		want string
	}{
		{name: "non success", body: `{"error":"denied"}`, code: http.StatusUnauthorized, want: "non-2xx"},
		{name: "invalid json", body: `{`, code: http.StatusOK, want: "unmarshal"},
		{name: "empty choices", body: `{"choices":[]}`, code: http.StatusOK, want: "empty response"},
		{name: "empty content", body: `{"choices":[{"message":{"content":""}}]}`, code: http.StatusOK, want: "content is empty"},
		{name: "invalid content", body: `{"choices":[{"message":{"content":{"text":"no"}}}]}`, code: http.StatusOK, want: "neither text nor text parts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.code)
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			client := NewLLMClient(appconfig.LLMProviderOpenAI, "key", "model", server.URL, "")
			_, err := client.providerClient.Complete(t.Context(), "hello")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestOpenAICompatibleClientUsesDefaultsAndPropagatesCancellation(t *testing.T) {
	client := NewLLMClientWithOptions(LLMClientOptions{
		Provider: appconfig.LLMProviderOllama,
		Model:    "qwen",
	})
	provider := client.providerClient.(*openAICompatibleClient)
	if provider.baseURL != "http://ollama:11434/v1" || provider.maxTokens != defaultLLMMaxTokens {
		t.Fatalf("provider defaults = %#v", provider)
	}
	if provider.temperature != nil {
		t.Fatalf("temperature = %#v, want provider default", provider.temperature)
	}

	client.httpClient.Transport = llmRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport failed")
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.providerClient.Complete(ctx, "hello"); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("error = %v", err)
	}
}

func TestAnthropicClientRequestResponseAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "secret" {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2024-01-01" {
			t.Errorf("anthropic-version = %q", got)
		}
		var payload struct {
			Model       string   `json:"model"`
			MaxTokens   int      `json:"max_tokens"`
			Temperature *float64 `json:"temperature"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload.Model != "claude-test" || payload.MaxTokens != 99 {
			t.Errorf("payload = %#v", payload)
		}
		if payload.Temperature != nil {
			t.Errorf("temperature = %#v, want omitted", payload.Temperature)
		}
		fmt.Fprint(w, `{"content":[{"type":"thinking","text":"hidden"},{"type":"text","text":"true"}],"usage":{"input_tokens":4,"output_tokens":1}}`)
	}))
	defer server.Close()

	client := NewLLMClientWithOptions(LLMClientOptions{
		Provider:  appconfig.LLMProviderAnthropic,
		Profile:   "review",
		APIKey:    "secret",
		Model:     "claude-test",
		BaseURL:   server.URL,
		MaxTokens: 99,
		Extra:     map[string]string{"anthropic_version": "2024-01-01"},
	})
	collector := NewUsageCollector()
	response, err := client.providerClient.Complete(ContextWithUsageCollector(t.Context(), collector), "decide")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if response != "true" {
		t.Fatalf("response = %q", response)
	}
	usages := collector.Snapshot()
	if len(usages) != 1 || usages[0].PromptTokens != 4 || usages[0].CompletionTokens != 1 {
		t.Fatalf("usage = %#v", usages)
	}
}

func TestAnthropicClientRejectsEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"content":[{"type":"thinking","text":"no text"}]}`)
	}))
	defer server.Close()
	client := NewLLMClient(appconfig.LLMProviderAnthropic, "key", "model", server.URL, "")
	if _, err := client.providerClient.Complete(t.Context(), "hello"); err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Fatalf("error = %v", err)
	}
}

func TestAnthropicClientErrors(t *testing.T) {
	tests := []struct {
		name string
		code int
		body string
		want string
	}{
		{name: "non success", code: http.StatusBadRequest, body: `{"error":"bad"}`, want: "non-2xx"},
		{name: "invalid json", code: http.StatusOK, body: `{`, want: "unmarshal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.code)
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			client := NewLLMClient(appconfig.LLMProviderAnthropic, "key", "model", server.URL+"/v1", "")
			if _, err := client.providerClient.Complete(t.Context(), "hello"); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestProviderEndpointBuilders(t *testing.T) {
	if got := buildOpenAIChatCompletionsURL("https://example.test/v1"); got != "https://example.test/v1/chat/completions" {
		t.Fatalf("OpenAI URL = %q", got)
	}
	if got := buildOpenAIChatCompletionsURL("https://example.test/chat/completions"); got != "https://example.test/chat/completions" {
		t.Fatalf("OpenAI complete URL = %q", got)
	}
	if got := buildAnthropicMessagesURL("https://example.test/v1"); got != "https://example.test/v1/messages" {
		t.Fatalf("Anthropic URL = %q", got)
	}
	if got := buildAnthropicMessagesURL("https://example.test/v1/messages"); got != "https://example.test/v1/messages" {
		t.Fatalf("Anthropic complete URL = %q", got)
	}
	if got := buildAzureOpenAIChatCompletionsURL("https://resource.openai.azure.com", "", ""); got != "https://resource.openai.azure.com/openai/v1/chat/completions" {
		t.Fatalf("Azure v1 URL = %q", got)
	}
	wantLegacy := "https://resource.openai.azure.com/openai/deployments/deploy/chat/completions?api-version=2024-10-21"
	if got := buildAzureOpenAIChatCompletionsURL("https://resource.openai.azure.com", "deploy", "2024-10-21"); got != wantLegacy {
		t.Fatalf("Azure legacy URL = %q, want %q", got, wantLegacy)
	}
}

func TestAzureOpenAIClientUsesModernRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openai/v1/chat/completions" || r.URL.RawQuery != "" {
			t.Errorf("URL = %s", r.URL.String())
		}
		if got := r.Header.Get("api-key"); got != "secret" {
			t.Errorf("api-key = %q", got)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer server.Close()

	client := NewLLMClientWithOptions(LLMClientOptions{
		Provider: appconfig.LLMProviderAzureOpenAI,
		APIKey:   "secret",
		Model:    "gpt-test",
		BaseURL:  server.URL,
	})
	if response, err := client.providerClient.Complete(t.Context(), "hello"); err != nil || response != "ok" {
		t.Fatalf("Complete() = %q, %v", response, err)
	}
}

func TestAzureOpenAIClientUsesAPIKeyAndDeployment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openai/deployments/deploy/chat/completions" || r.URL.Query().Get("api-version") != "2024-10-21" {
			t.Errorf("URL = %s", r.URL.String())
		}
		if got := r.Header.Get("api-key"); got != "secret" {
			t.Errorf("api-key = %q", got)
		}
		var payload openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload.Model != "deploy" {
			t.Errorf("model = %q", payload.Model)
		}
		if payload.MaxCompletionTokens != defaultLLMMaxTokens || payload.MaxTokens != 0 {
			t.Errorf("token fields = %#v", payload)
		}
		if payload.Temperature != nil {
			t.Errorf("temperature = %#v, want omitted", payload.Temperature)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer server.Close()

	client := NewLLMClientWithOptions(LLMClientOptions{
		Provider: appconfig.LLMProviderAzureOpenAI,
		APIKey:   "secret",
		Model:    "gpt-test",
		BaseURL:  server.URL,
		Extra: map[string]string{
			"deployment":  "deploy",
			"api_version": "2024-10-21",
		},
	})
	if response, err := client.providerClient.Complete(t.Context(), "hello"); err != nil || response != "ok" {
		t.Fatalf("Complete() = %q, %v", response, err)
	}
}

func TestNewLLMClientWithOptionsAppliesTimeoutAndUnsupportedProvider(t *testing.T) {
	client := NewLLMClientWithOptions(LLMClientOptions{
		Provider:       "custom",
		TimeoutSeconds: 7,
	})
	if client.httpClient.Timeout != 7*time.Second {
		t.Fatalf("timeout = %s", client.httpClient.Timeout)
	}
	if client.providerClient.Name() != "custom" {
		t.Fatalf("provider name = %q", client.providerClient.Name())
	}
	if _, err := client.providerClient.Complete(t.Context(), "hello"); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error = %v", err)
	}
}

func TestReadLLMResponseBodyRejectsOversizedResponse(t *testing.T) {
	_, err := readLLMResponseBody(io.LimitReader(strings.NewReader(strings.Repeat("x", maxLLMResponseBytes+1)), maxLLMResponseBytes+1))
	if err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("error = %v", err)
	}
}

func TestGenericProviderActionAndConditionOrchestration(t *testing.T) {
	client := NewLLMClientWithOptions(LLMClientOptions{Provider: appconfig.LLMProviderOpenAI})
	client.providerClient = staticProviderClient{
		name:     appconfig.LLMProviderOpenAI,
		response: `{"action":{"type":"RETURN_ANSWER","answer_action":{"answer":"done"}}}`,
	}
	action, err := client.getActionModel(t.Context(), "act")
	if err != nil || action.AnswerAction == nil || action.AnswerAction.Answer != "done" {
		t.Fatalf("getActionModel() = %#v, %v", action, err)
	}

	client.providerClient = staticProviderClient{name: appconfig.LLMProviderOpenAI, response: "true"}
	result, err := client.EvaluateCondition(t.Context(), &proto.ConditionRequest{Goal: "ready?"})
	if err != nil || !result.Result {
		t.Fatalf("EvaluateCondition() = %#v, %v", result, err)
	}

	client.providerClient = staticProviderClient{name: appconfig.LLMProviderOpenAI, err: errors.New("provider failed")}
	result, err = client.EvaluateCondition(t.Context(), &proto.ConditionRequest{Goal: "ready?"})
	if err == nil || result.Result {
		t.Fatalf("EvaluateCondition() = %#v, %v", result, err)
	}
}

type staticProviderClient struct {
	name     string
	response string
	err      error
}

func (c staticProviderClient) Name() string {
	return c.name
}

func (c staticProviderClient) Complete(context.Context, string) (string, error) {
	return c.response, c.err
}

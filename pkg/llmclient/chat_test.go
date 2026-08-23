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

func toolsForTest() []ToolDefinition {
	return []ToolDefinition{{
		Name:        "nopsai.get_pipeline_run_logs",
		Description: "Read run logs.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"run_id": map[string]any{"type": "string"}},
		},
	}}
}

func TestOpenAIChatSendsToolsAndReadsToolCalls(t *testing.T) {
	var request openAIToolChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		fmt.Fprint(w, `{"choices":[{"finish_reason":"tool_calls","message":{"content":null,"tool_calls":[
			{"id":"call_1","type":"function","function":{"name":"nopsai.get_pipeline_run_logs","arguments":"{\"run_id\":\"run-1\"}"}}
		]}}],"usage":{"prompt_tokens":9,"completion_tokens":3,"total_tokens":12}}`)
	}))
	defer server.Close()

	response, err := New(Options{
		Provider: config.LLMProviderOpenAI,
		APIKey:   "secret",
		Model:    "gpt-test",
		BaseURL:  server.URL + "/v1",
	}).Chat(t.Context(), ChatRequest{
		System:   "you are the assistant",
		Messages: []ChatMessage{{Role: RoleUser, Content: "why did run-1 fail"}},
		Tools:    toolsForTest(),
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if len(request.Tools) != 1 || request.Tools[0].Function.Name != "nopsai.get_pipeline_run_logs" {
		t.Fatalf("tools were not sent to the provider: %#v", request.Tools)
	}
	if request.Tools[0].Function.Parameters["type"] != "object" {
		t.Fatalf("tool parameters must stay schema-shaped: %#v", request.Tools[0].Function.Parameters)
	}
	if request.Messages[0].Role != "system" {
		t.Fatalf("system instruction should lead the messages: %#v", request.Messages)
	}
	if len(response.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %#v", response.ToolCalls)
	}
	call := response.ToolCalls[0]
	if call.ID != "call_1" || call.Name != "nopsai.get_pipeline_run_logs" || call.Arguments["run_id"] != "run-1" {
		t.Fatalf("tool call was not decoded: %#v", call)
	}
	// A tool call with no prose is a complete turn, not an empty response.
	if response.Text != "" || response.Stop != "tool_calls" {
		t.Fatalf("unexpected text or stop reason: %q / %q", response.Text, response.Stop)
	}
	if response.Usage.TotalTokens != 12 {
		t.Fatalf("usage was not recorded: %#v", response.Usage)
	}
}

func TestOpenAIChatSendsToolResultsBack(t *testing.T) {
	var request openAIToolChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"helm had no terminal to prompt on"}}]}`)
	}))
	defer server.Close()

	response, err := New(Options{Provider: config.LLMProviderOpenAI, Model: "gpt-test", BaseURL: server.URL + "/v1"}).
		Chat(t.Context(), ChatRequest{
			Messages: []ChatMessage{
				{Role: RoleUser, Content: "why did run-1 fail"},
				{Role: RoleAssistant, ToolCalls: []ToolCall{{
					ID:        "call_1",
					Name:      "nopsai.get_pipeline_run_logs",
					Arguments: map[string]any{"run_id": "run-1"},
				}}},
				{Role: RoleTool, ToolCallID: "call_1", Content: `{"logs":[{"line":"Error: inappropriate ioctl for device"}]}`},
			},
			Tools: toolsForTest(),
		})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if response.Text != "helm had no terminal to prompt on" {
		t.Fatalf("unexpected reply: %q", response.Text)
	}

	assistant := request.Messages[1]
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].Function.Arguments != `{"run_id":"run-1"}` {
		t.Fatalf("assistant tool call was not re-encoded: %#v", assistant)
	}
	result := request.Messages[2]
	if result.Role != "tool" || result.ToolCallID != "call_1" {
		t.Fatalf("tool result must answer its call: %#v", result)
	}
}

func TestAnthropicChatUsesToolUseBlocks(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		fmt.Fprint(w, `{"stop_reason":"tool_use","content":[
			{"type":"text","text":"reading the logs"},
			{"type":"tool_use","id":"toolu_1","name":"nopsai.get_pipeline_run_logs","input":{"run_id":"run-1"}}
		],"usage":{"input_tokens":11,"output_tokens":4}}`)
	}))
	defer server.Close()

	response, err := New(Options{
		Provider: config.LLMProviderAnthropic,
		APIKey:   "secret",
		Model:    "claude-test",
		BaseURL:  server.URL,
	}).Chat(t.Context(), ChatRequest{
		System: "you are the assistant",
		Messages: []ChatMessage{
			{Role: RoleUser, Content: "why did run-1 fail"},
			{Role: RoleTool, ToolCallID: "toolu_0", Content: "previous result"},
		},
		Tools: toolsForTest(),
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if response.Text != "reading the logs" || len(response.ToolCalls) != 1 {
		t.Fatalf("text and tool_use should both survive: %#v", response)
	}
	if response.ToolCalls[0].ID != "toolu_1" || response.ToolCalls[0].Arguments["run_id"] != "run-1" {
		t.Fatalf("tool_use was not decoded: %#v", response.ToolCalls[0])
	}

	tools, _ := request["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools were not sent: %#v", request["tools"])
	}
	if _, ok := tools[0].(map[string]any)["input_schema"]; !ok {
		t.Fatalf("anthropic tools carry input_schema: %#v", tools[0])
	}
	// A tool result travels as a user turn holding a tool_result block.
	messages, _ := request["messages"].([]any)
	last, _ := messages[len(messages)-1].(map[string]any)
	if last["role"] != "user" {
		t.Fatalf("anthropic tool results are user turns: %#v", last)
	}
	blocks, _ := last["content"].([]any)
	block, _ := blocks[0].(map[string]any)
	if block["type"] != "tool_result" || block["tool_use_id"] != "toolu_0" {
		t.Fatalf("tool_result block is wrong: %#v", block)
	}
}

func TestGeminiChatUsesFunctionDeclarations(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		fmt.Fprint(w, `{"candidates":[{"finishReason":"STOP","content":{"parts":[
			{"functionCall":{"name":"nopsai.get_pipeline_run_logs","args":{"run_id":"run-1"}}}
		]}}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":2,"totalTokenCount":10}}`)
	}))
	defer server.Close()

	response, err := New(Options{
		Provider: config.LLMProviderGemini,
		APIKey:   "secret",
		Model:    "gemini-test",
		BaseURL:  server.URL,
	}).Chat(t.Context(), ChatRequest{
		Messages: []ChatMessage{
			{Role: RoleUser, Content: "why did run-1 fail"},
			{Role: RoleTool, ToolName: "nopsai.get_pipeline_run_logs", Content: "log text"},
		},
		Tools: toolsForTest(),
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].Name != "nopsai.get_pipeline_run_logs" {
		t.Fatalf("functionCall was not decoded: %#v", response.ToolCalls)
	}

	tools, _ := request["tools"].([]any)
	declarations, _ := tools[0].(map[string]any)["functionDeclarations"].([]any)
	if len(declarations) != 1 {
		t.Fatalf("gemini tools carry functionDeclarations: %#v", tools)
	}
	// Gemini answers a call by name, so the result has to name its function.
	contents, _ := request["contents"].([]any)
	last, _ := contents[len(contents)-1].(map[string]any)
	parts, _ := last["parts"].([]any)
	part, _ := parts[0].(map[string]any)
	functionResponse, ok := part["functionResponse"].(map[string]any)
	if !ok || functionResponse["name"] != "nopsai.get_pipeline_run_logs" {
		t.Fatalf("gemini tool result is wrong: %#v", part)
	}
}

func TestChatRejectsAnEmptyConversation(t *testing.T) {
	if _, err := New(Options{Provider: config.LLMProviderOpenAI}).Chat(t.Context(), ChatRequest{}); err == nil {
		t.Fatal("a chat turn needs at least one message")
	}
}

func TestToolArgumentsDecodeFromTheProviderString(t *testing.T) {
	arguments, err := decodeToolArguments(`{"run_id":"run-1","limit":20}`)
	if err != nil {
		t.Fatalf("decodeToolArguments() error = %v", err)
	}
	if arguments["run_id"] != "run-1" {
		t.Fatalf("arguments were not decoded: %#v", arguments)
	}
	// A no-argument call is normal, not a parse failure.
	empty, err := decodeToolArguments("")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty arguments should decode to an empty map, got %#v / %v", empty, err)
	}
	if _, err := decodeToolArguments("not json"); err == nil {
		t.Fatal("malformed arguments must be reported")
	}
}

// A turn takes several model calls. LM Studio was asked for its model list
// before every one of them, which is that many round trips to learn the same
// answer while the model is busy generating.
func TestLMStudioChecksModelLoadOncePerClient(t *testing.T) {
	modelListCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/models/load"):
			fmt.Fprint(w, `{"ok":true}`)
		case strings.HasSuffix(r.URL.Path, "/models"):
			modelListCalls++
			fmt.Fprint(w, `{"data":[{"id":"local-model"}],"models":[{"key":"local-model","type":"llm","state":"loaded"}]}`)
		default:
			fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"answer"}}]}`)
		}
	}))
	defer server.Close()

	client := New(Options{Provider: config.LLMProviderLMStudio, Model: "local-model", BaseURL: server.URL + "/v1"})
	for turn := 0; turn < 3; turn++ {
		if _, err := client.Chat(t.Context(), ChatRequest{
			Messages: []ChatMessage{{Role: RoleUser, Content: "hello"}},
		}); err != nil {
			t.Fatalf("Chat() turn %d error = %v", turn, err)
		}
	}

	if modelListCalls != 1 {
		t.Fatalf("model list fetched %d times across three turns, want 1", modelListCalls)
	}
}

// LM Studio serves two APIs: /api/v1/chat takes a single `input` string and
// knows nothing about tools, and /v1/chat/completions is the OpenAI-compatible
// one that does. Sending a tool-calling request to the first is what produced
// "'input' is required" in the field.
func TestLMStudioChatUsesTheOpenAICompatibleEndpoint(t *testing.T) {
	for _, configured := range []string{
		"http://lmstudio:1234",
		"http://lmstudio:1234/",
		"http://lmstudio:1234/api/v1",
		"http://lmstudio:1234/api/v1/chat",
		"http://lmstudio:1234/v1",
		"http://lmstudio:1234/v1/chat/completions",
	} {
		if got := buildLMStudioOpenAIChatURL(configured); got != "http://lmstudio:1234/v1/chat/completions" {
			t.Fatalf("buildLMStudioOpenAIChatURL(%q) = %q", configured, got)
		}
	}
}

func TestLMStudioChatSendsMessagesAndTools(t *testing.T) {
	var path string
	var payload openAIToolChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			fmt.Fprint(w, `{"data":[{"id":"local-model"}],"models":[{"key":"local-model","type":"llm","state":"loaded"}]}`)
			return
		}
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"answer"}}]}`)
	}))
	defer server.Close()

	response, err := New(Options{Provider: config.LLMProviderLMStudio, Model: "local-model", BaseURL: server.URL + "/api/v1"}).
		Chat(t.Context(), ChatRequest{
			Messages: []ChatMessage{{Role: RoleUser, Content: "why did run-1 fail"}},
			Tools:    toolsForTest(),
		})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if response.Text != "answer" {
		t.Fatalf("reply = %q", response.Text)
	}
	if path != "/v1/chat/completions" {
		t.Fatalf("tool calling must go to the OpenAI-compatible path, got %q", path)
	}
	if len(payload.Messages) == 0 || len(payload.Tools) != 1 {
		t.Fatalf("messages and tools were not sent: %#v", payload)
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appconfig "nopsai/config"
	"nopsai/pkg/mcpclient"
	"nopsai/pkg/models"
	"nopsai/pkg/proto"
)

func TestBuildLMStudioChatURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "bare host", baseURL: "http://127.0.0.1:1234", want: "http://127.0.0.1:1234/api/v1/chat"},
		{name: "api v1 base", baseURL: "http://127.0.0.1:1234/api/v1", want: "http://127.0.0.1:1234/api/v1/chat"},
		{name: "full endpoint", baseURL: "http://127.0.0.1:1234/api/v1/chat", want: "http://127.0.0.1:1234/api/v1/chat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildLMStudioChatURL(tt.baseURL); got != tt.want {
				t.Fatalf("buildLMStudioChatURL(%q) = %q, want %q", tt.baseURL, got, tt.want)
			}
		})
	}
}

func TestBuildLMStudioModelsURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "bare host", baseURL: "http://127.0.0.1:1234", want: "http://127.0.0.1:1234/api/v1/models"},
		{name: "api v1 base", baseURL: "http://127.0.0.1:1234/api/v1", want: "http://127.0.0.1:1234/api/v1/models"},
		{name: "chat endpoint", baseURL: "http://127.0.0.1:1234/api/v1/chat", want: "http://127.0.0.1:1234/api/v1/models"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildLMStudioModelsURL(tt.baseURL); got != tt.want {
				t.Fatalf("buildLMStudioModelsURL(%q) = %q, want %q", tt.baseURL, got, tt.want)
			}
		})
	}
}

func TestBuildLMStudioModelLoadURL(t *testing.T) {
	if got, want := buildLMStudioModelLoadURL("http://127.0.0.1:1234"), "http://127.0.0.1:1234/api/v1/models/load"; got != want {
		t.Fatalf("buildLMStudioModelLoadURL() = %q, want %q", got, want)
	}
}

func TestCleanModelTextResponse(t *testing.T) {
	raw := "```json\n{\"action\":{\"type\":\"RETURN_ANSWER\"}}\n```"
	want := "{\"action\":{\"type\":\"RETURN_ANSWER\"}}"

	if got := cleanModelTextResponse(raw); got != want {
		t.Fatalf("cleanModelTextResponse() = %q, want %q", got, want)
	}
}

func TestDecodeActionResponseRecoversFencedJSONAfterReasoning(t *testing.T) {
	raw := "The user wants to know which files are visible based only on the shared workspace files.\n\n" +
		"Therefore, the answer should list the files present in the working directory contents.```json\n" +
		"{\n" +
		"  \"action\": {\n" +
		"    \"type\": \"RETURN_ANSWER\",\n" +
		"    \"answer_action\": {\n" +
		"      \"answer\": \"The file visible in the shared workspace is: test-app/Dockerfile.\"\n" +
		"    }\n" +
		"  }\n" +
		"}"

	action, err := decodeActionResponse(raw)
	if err != nil {
		t.Fatalf("decodeActionResponse() error = %v", err)
	}
	if action.Type != models.ActionTypeReturnAnswer {
		t.Fatalf("action.Type = %q, want %q", action.Type, models.ActionTypeReturnAnswer)
	}
	if action.AnswerAction == nil || action.AnswerAction.Answer != "The file visible in the shared workspace is: test-app/Dockerfile." {
		t.Fatalf("action.AnswerAction = %#v", action.AnswerAction)
	}
}

func TestDecodeActionResponseRecoversBalancedJSONWithBracesInString(t *testing.T) {
	raw := `I will return the action now.
{"action":{"type":"EXECUTE_COMMAND","command_action":{"command":"printf '{hello}'"}}}
Done.`

	action, err := decodeActionResponse(raw)
	if err != nil {
		t.Fatalf("decodeActionResponse() error = %v", err)
	}
	if action.Type != models.ActionTypeExecuteCommand {
		t.Fatalf("action.Type = %q, want %q", action.Type, models.ActionTypeExecuteCommand)
	}
	if action.CommandAction == nil || action.CommandAction.Command != "printf '{hello}'" {
		t.Fatalf("action.CommandAction = %#v", action.CommandAction)
	}
}

func TestDecodeActionResponseFailsWithoutValidAction(t *testing.T) {
	if _, err := decodeActionResponse("The file visible in the shared workspace is: test-app/Dockerfile."); err == nil {
		t.Fatal("expected decodeActionResponse() to fail without a valid action object")
	}
}

func TestBuildPromptLMStudioKeepsFullContext(t *testing.T) {
	client := NewLLMClient(appconfig.LLMProviderLMStudio, "", "qwen/qwen3.6-35b-a3b", "http://127.0.0.1:1234", "off")
	req := &proto.GetActionRequest{
		Goal:    "wait for 2 seconds",
		History: strings.Repeat("history-line\n", 400),
		Variables: map[string]string{
			"SHORT": "ok",
			"LONG":  strings.Repeat("very-long-variable-value-", 40),
		},
		DirectoryListing: map[string]string{
			"pipeline.yaml": strings.Repeat("step: build-and-test\n", 300),
			"README.md":     strings.Repeat("documentation\n", 300),
			"script.sh":     strings.Repeat("echo hello\n", 300),
		},
	}

	prompt := client.buildPrompt(req)

	if strings.Contains(prompt, "[directory listing truncated") || strings.Contains(prompt, "...[truncated]...") {
		t.Fatalf("expected LM Studio prompt to keep full context, got: %s", prompt)
	}
	for _, expected := range []string{
		strings.Repeat("history-line\n", 400),
		strings.Repeat("very-long-variable-value-", 40),
		"--- File: README.md ---",
		"--- File: pipeline.yaml ---",
		"--- File: script.sh ---",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected prompt to contain full LM Studio context fragment %q", expected)
		}
	}
}

func TestBuildPromptGeminiKeepsLargeContext(t *testing.T) {
	client := NewLLMClient(appconfig.LLMProviderGemini, "", "gemini-2.5-flash", "", "")
	req := &proto.GetActionRequest{
		Goal:    "show file",
		History: strings.Repeat("history-line\n", 50),
		DirectoryListing: map[string]string{
			"pipeline.yaml": strings.Repeat("step: build-and-test\n", 120),
		},
	}

	prompt := client.buildPrompt(req)

	if strings.Contains(prompt, "...[truncated]...") {
		t.Fatalf("expected Gemini prompt to remain untruncated")
	}
}

func TestGetActionWithMCPRequiresToolCallWhenRuntimeRequiresMCP(t *testing.T) {
	mcpCalls := 0
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rpcReq struct {
			ID     any             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&rpcReq); err != nil {
			t.Errorf("decode MCP request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch rpcReq.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "test-session")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      rpcReq.ID,
				"result": map[string]any{
					"protocolVersion": "2025-06-18",
					"serverInfo":      map[string]string{"name": "github", "version": "test"},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			mcpCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      rpcReq.ID,
				"result": map[string]any{
					"content": []map[string]string{{"type": "text", "text": "default_branch=main latest_commit=abc123"}},
				},
			})
		default:
			t.Errorf("unexpected MCP method %s", rpcReq.Method)
			http.NotFound(w, r)
		}
	}))
	defer mcpServer.Close()

	chatResponses := []string{
		`{"action":{"type":"EXECUTE_COMMAND","command_action":{"command":"ls -R test-app"}}}`,
		`{"action":{"type":"CALL_MCP_TOOL","mcp_tool_action":{"server":"github","tool":"issues_list","arguments":{"owner":"hosein-yousefii","repo":"test-app"}}}}`,
		`{"action":{"type":"RETURN_ANSWER","answer_action":{"answer":"Repository metadata was read through MCP."}}}`,
	}
	var chatCalls int32
	lmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/models":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"models":[%s]}`, lmStudioModelListItemForTest("model-a", true))
		case "/api/v1/chat":
			var req struct {
				Input string `json:"input"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode LM Studio request: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			callIndex := int(atomic.AddInt32(&chatCalls, 1)) - 1
			if callIndex == 1 && !strings.Contains(req.Input, "MCP tool call required before final action") {
				t.Fatalf("second prompt did not reject the premature final action:\n%s", req.Input)
			}
			if callIndex >= len(chatResponses) {
				t.Fatalf("unexpected extra LM Studio chat call %d", callIndex+1)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"output": []map[string]string{{"type": "message", "content": chatResponses[callIndex]}},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer lmServer.Close()

	spec := MCPToolSpec{Server: "github", Name: "issues_list", InputSchema: `{"type":"object"}`}
	runtime := &MCPTaskRuntime{
		registry: &MCPProfileRegistry{
			servers: map[string]agentRuntimeMCPServer{
				"github": {
					MCPServer: models.MCPServer{
						Name:      "github",
						Enabled:   true,
						Transport: models.MCPTransportStreamableHTTP,
						URL:       mcpServer.URL,
						AuthType:  models.MCPAuthNone,
						Timeout:   models.DefaultMCPTimeout,
					},
				},
			},
			clients: map[string]*mcpclient.Client{},
		},
		tools:           []MCPToolSpec{spec},
		allowed:         map[string]MCPToolSpec{mcpToolKey(spec.Server, spec.Name): spec},
		requireToolCall: true,
	}

	client := NewLLMClient(appconfig.LLMProviderLMStudio, "", "model-a", lmServer.URL, "off")
	action, err := client.GetActionWithMCP(t.Context(), &proto.GetActionRequest{
		Goal: "Read repository metadata first, then summarize the project purpose.",
	}, runtime)
	if err != nil {
		t.Fatalf("GetActionWithMCP() error = %v", err)
	}
	if action.GetType() != models.ActionTypeReturnAnswer {
		t.Fatalf("action type = %q, want RETURN_ANSWER", action.GetType())
	}
	if mcpCalls != 1 {
		t.Fatalf("MCP tools/call count = %d, want 1", mcpCalls)
	}
	if got := atomic.LoadInt32(&chatCalls); got != 3 {
		t.Fatalf("LM Studio chat calls = %d, want 3", got)
	}
}

func TestGetActionWithMCPReusesSuccessfulToolResultAfterRetry(t *testing.T) {
	var mcpCalls int32
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var rpcReq struct {
			ID     any             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&rpcReq); err != nil {
			t.Errorf("decode MCP request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch rpcReq.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "test-session")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      rpcReq.ID,
				"result": map[string]any{
					"protocolVersion": "2025-06-18",
					"serverInfo":      map[string]string{"name": "github", "version": "test"},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			atomic.AddInt32(&mcpCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      rpcReq.ID,
				"result": map[string]any{
					"content": []map[string]string{{"type": "text", "text": "commit=abc123 message=Update README.md"}},
				},
			})
		default:
			t.Errorf("unexpected MCP method %s", rpcReq.Method)
			http.NotFound(w, r)
		}
	}))
	defer mcpServer.Close()

	chatResponses := []string{
		`{"action":{"type":"CALL_MCP_TOOL","mcp_tool_action":{"server":"github","tool":"list_commits","arguments":{"owner":"hosein-yousefii","repo":"test-app"}}}}`,
		`not json`,
		`{"action":{"type":"RETURN_ANSWER","answer_action":{"answer":"abc123 Update README.md"}}}`,
	}
	var chatCalls int32
	lmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/models":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"models":[%s]}`, lmStudioModelListItemForTest("model-a", true))
		case "/api/v1/chat":
			var req struct {
				Input string `json:"input"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode LM Studio request: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			callIndex := int(atomic.AddInt32(&chatCalls, 1)) - 1
			if callIndex == 2 {
				if !strings.Contains(req.Input, "MCP tool result") || !strings.Contains(req.Input, "commit=abc123") {
					t.Fatalf("retry prompt did not include previous MCP result:\n%s", req.Input)
				}
				if strings.Contains(req.Input, "your first action must be CALL_MCP_TOOL") {
					t.Fatalf("retry prompt still forced another MCP tool call:\n%s", req.Input)
				}
			}
			if callIndex >= len(chatResponses) {
				t.Fatalf("unexpected extra LM Studio chat call %d", callIndex+1)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"output": []map[string]string{{"type": "message", "content": chatResponses[callIndex]}},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer lmServer.Close()

	spec := MCPToolSpec{Server: "github", Name: "list_commits", InputSchema: `{"type":"object"}`}
	runtime := &MCPTaskRuntime{
		registry: &MCPProfileRegistry{
			servers: map[string]agentRuntimeMCPServer{
				"github": {
					MCPServer: models.MCPServer{
						Name:      "github",
						Enabled:   true,
						Transport: models.MCPTransportStreamableHTTP,
						URL:       mcpServer.URL,
						AuthType:  models.MCPAuthNone,
						Timeout:   models.DefaultMCPTimeout,
					},
				},
			},
			clients: map[string]*mcpclient.Client{},
		},
		tools:           []MCPToolSpec{spec},
		allowed:         map[string]MCPToolSpec{mcpToolKey(spec.Server, spec.Name): spec},
		requireToolCall: true,
	}

	client := NewLLMClient(appconfig.LLMProviderLMStudio, "", "model-a", lmServer.URL, "off")
	if _, err := client.GetActionWithMCP(t.Context(), &proto.GetActionRequest{
		Goal: "List commits from the test-app repo.",
	}, runtime); err == nil {
		t.Fatalf("first GetActionWithMCP() succeeded; want parse error after tool call")
	}
	if got := runtime.SuccessfulToolCalls(); got != 1 {
		t.Fatalf("runtime successful MCP calls = %d, want 1", got)
	}

	action, err := client.GetActionWithMCP(t.Context(), &proto.GetActionRequest{
		Goal: "List commits from the test-app repo.",
	}, runtime)
	if err != nil {
		t.Fatalf("second GetActionWithMCP() error = %v", err)
	}
	if action.GetType() != models.ActionTypeReturnAnswer {
		t.Fatalf("action type = %q, want RETURN_ANSWER", action.GetType())
	}
	if got := atomic.LoadInt32(&mcpCalls); got != 1 {
		t.Fatalf("MCP tools/call count = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&chatCalls); got != 3 {
		t.Fatalf("LM Studio chat calls = %d, want 3", got)
	}
}

func TestGetActionWithMCPFailsMissingCommitToolInsteadOfGitLogFallback(t *testing.T) {
	var chatCalls int32
	lmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/models":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"models":[%s]}`, lmStudioModelListItemForTest("model-a", true))
		case "/api/v1/chat":
			atomic.AddInt32(&chatCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"output": []map[string]string{{
					"type":    "message",
					"content": `{"action":{"type":"EXECUTE_COMMAND","command_action":{"command":"git log -n 10"}}}`,
				}},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer lmServer.Close()

	runtime := &MCPTaskRuntime{
		profiles: []string{"github-readonly"},
		tools: []MCPToolSpec{
			{Server: "github", Name: "get_file_contents", InputSchema: `{"type":"object"}`},
			{Server: "github", Name: "get_repository", InputSchema: `{"type":"object"}`},
		},
		allowed:         map[string]MCPToolSpec{},
		requireToolCall: true,
	}
	runtime.recordSuccessfulToolCall(
		"github",
		"get_repository",
		json.RawMessage(`{"owner":"hosein-yousefii","repo":"test-app"}`),
		json.RawMessage(`{"default_branch":"main"}`),
	)

	client := NewLLMClient(appconfig.LLMProviderLMStudio, "", "model-a", lmServer.URL, "off")
	action, err := client.GetActionWithMCP(t.Context(), &proto.GetActionRequest{
		Goal: "List commits from the test-app repo.",
	}, runtime)
	if err == nil {
		t.Fatalf("GetActionWithMCP() succeeded with action %#v, want missing MCP permission error", action)
	}
	if action != nil {
		t.Fatalf("action = %#v, want nil on missing MCP permission", action)
	}
	if !isNonRetryableGoalResolutionError(err) {
		t.Fatalf("error = %v, want non-retryable goal resolution error", err)
	}
	answer := err.Error()
	for _, want := range []string{"Git commit history", "list_commits", "get_commit", "github-readonly", "github/get_file_contents"} {
		if !strings.Contains(answer, want) {
			t.Fatalf("answer = %q, want it to contain %q", answer, want)
		}
	}
	if strings.Contains(answer, "fatal: not a git repository") {
		t.Fatalf("answer included shell failure instead of MCP permission guidance: %q", answer)
	}
	if got := atomic.LoadInt32(&chatCalls); got != 1 {
		t.Fatalf("LM Studio chat calls = %d, want 1", got)
	}
}

func TestGetActionWithMCPFailsMissingToolReturnAnswer(t *testing.T) {
	var chatCalls int32
	lmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/models":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"models":[%s]}`, lmStudioModelListItemForTest("model-a", true))
		case "/api/v1/chat":
			atomic.AddInt32(&chatCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"output": []map[string]string{{
					"type":    "message",
					"content": `{"action":{"type":"RETURN_ANSWER","answer_action":{"answer":"I am unable to list the commits because the 'list_commits' tool is not available in the allowed MCP tools for this task. I can only access other GitHub information like branches, issues, pull requests, and file contents."}}}`,
				}},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer lmServer.Close()

	runtime := &MCPTaskRuntime{
		profiles: []string{"github-readonly"},
		tools: []MCPToolSpec{
			{Server: "github", Name: "get_file_contents", InputSchema: `{"type":"object"}`},
			{Server: "github", Name: "list_issues", InputSchema: `{"type":"object"}`},
		},
		allowed:         map[string]MCPToolSpec{},
		requireToolCall: true,
	}
	runtime.recordSuccessfulToolCall(
		"github",
		"get_file_contents",
		json.RawMessage(`{"owner":"hosein-yousefii","repo":"test-app","path":"README.md"}`),
		json.RawMessage(`{"content":"sample app"}`),
	)

	client := NewLLMClient(appconfig.LLMProviderLMStudio, "", "model-a", lmServer.URL, "off")
	action, err := client.GetActionWithMCP(t.Context(), &proto.GetActionRequest{
		Goal: "List commits from the test-app repo.",
	}, runtime)
	if err == nil {
		t.Fatalf("GetActionWithMCP() succeeded with action %#v, want missing MCP permission error", action)
	}
	if action != nil {
		t.Fatalf("action = %#v, want nil on missing MCP permission", action)
	}
	if !isNonRetryableGoalResolutionError(err) {
		t.Fatalf("error = %v, want non-retryable goal resolution error", err)
	}
	for _, want := range []string{"unable to list the commits", "list_commits", "not available", "allowed MCP tools"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want it to contain %q", err.Error(), want)
		}
	}
	if got := atomic.LoadInt32(&chatCalls); got != 1 {
		t.Fatalf("LM Studio chat calls = %d, want 1", got)
	}
}

func TestLMStudioConfiguredModelSkipsRepeatedModelDiscovery(t *testing.T) {
	var modelDiscoveryCalls int32
	var chatCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/models":
			atomic.AddInt32(&modelDiscoveryCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"models":[%s]}`, lmStudioModelListItemForTest("model-a", true))
		case "/api/v1/chat":
			atomic.AddInt32(&chatCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"output":[{"type":"message","content":"true"}]}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewLLMClient(appconfig.LLMProviderLMStudio, "", "model-a", server.URL, "")
	for i := 0; i < 2; i++ {
		if _, err := client.callLMStudioForBoolean(t.Context(), "is the cached model loaded?"); err != nil {
			t.Fatalf("callLMStudioForBoolean() error = %v", err)
		}
	}
	if got := atomic.LoadInt32(&modelDiscoveryCalls); got != 1 {
		t.Fatalf("model discovery calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&chatCalls); got != 2 {
		t.Fatalf("chat calls = %d, want 2", got)
	}
}

func TestLMStudioLoadsAreSerializedButChatsCanRunConcurrently(t *testing.T) {
	var activeLoads int32
	var maxActiveLoads int32
	var activeChats int32
	var maxActiveChats int32
	var loadedMu sync.Mutex
	loaded := map[string]bool{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/models":
			loadedMu.Lock()
			modelALoaded := loaded["model-a"]
			modelBLoaded := loaded["model-b"]
			loadedMu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"models":[%s,%s]}`,
				lmStudioModelListItemForTest("model-a", modelALoaded),
				lmStudioModelListItemForTest("model-b", modelBLoaded),
			)
		case "/api/v1/models/load":
			var req struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode load request: %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			current := atomic.AddInt32(&activeLoads, 1)
			recordMaxInt32(&maxActiveLoads, current)
			time.Sleep(25 * time.Millisecond)
			atomic.AddInt32(&activeLoads, -1)

			loadedMu.Lock()
			loaded[req.Model] = true
			loadedMu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"type":"llm","instance_id":%q,"status":"loaded"}`, req.Model)
		case "/api/v1/chat":
			current := atomic.AddInt32(&activeChats, 1)
			recordMaxInt32(&maxActiveChats, current)
			time.Sleep(150 * time.Millisecond)
			atomic.AddInt32(&activeChats, -1)

			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"output":[{"type":"message","content":"true"}]}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	clientA := NewLLMClient(appconfig.LLMProviderLMStudio, "", "model-a", server.URL, "")
	clientB := NewLLMClient(appconfig.LLMProviderLMStudio, "", "model-b", server.URL, "")

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, client := range []*LLMClient{clientA, clientB} {
		wg.Add(1)
		go func(client *LLMClient) {
			defer wg.Done()
			if _, err := client.callLMStudioForBoolean(t.Context(), "is this serialized?"); err != nil {
				errs <- err
			}
		}(client)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("callLMStudioForBoolean() error = %v", err)
	}
	if got := atomic.LoadInt32(&maxActiveLoads); got != 1 {
		t.Fatalf("max concurrent LM Studio loads = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&maxActiveChats); got != 2 {
		t.Fatalf("max concurrent LM Studio chats = %d, want 2", got)
	}
}

func lmStudioModelListItemForTest(model string, loaded bool) string {
	instances := "[]"
	if loaded {
		instances = fmt.Sprintf(`[{"id":%q}]`, model)
	}
	return fmt.Sprintf(`{"type":"llm","key":%q,"loaded_instances":%s}`, model, instances)
}

func recordMaxInt32(target *int32, current int32) {
	for {
		observed := atomic.LoadInt32(target)
		if current <= observed || atomic.CompareAndSwapInt32(target, observed, current) {
			return
		}
	}
}

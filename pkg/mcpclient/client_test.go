package mcpclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestClientInitializesListsAndCallsTool(t *testing.T) {
	var sawSession bool
	var sawProtocol bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string          `json:"method"`
			ID     any             `json:"id"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization header = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-MCP-Toolsets") != "default,actions" {
			t.Errorf("X-MCP-Toolsets header = %q", r.Header.Get("X-MCP-Toolsets"))
		}
		if req.Method != "initialize" && r.Header.Get("Mcp-Session-Id") == "session-1" {
			sawSession = true
		}
		if req.Method != "initialize" && r.Header.Get("MCP-Protocol-Version") == "2025-06-18" {
			sawProtocol = true
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "session-1")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18","serverInfo":{"name":"test-mcp","version":"1.0.0"}}}`))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"get_file","description":"Read a file","inputSchema":{"type":"object","properties":{"path":{"type":"string"}}}}]}}`))
		case "tools/call":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"ok"}]}}`))
		default:
			t.Errorf("unexpected method %s", req.Method)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(models.MCPServer{
		Name:          "github",
		Enabled:       true,
		Transport:     models.MCPTransportStreamableHTTP,
		URL:           server.URL,
		AuthType:      models.MCPAuthBearerToken,
		CredentialRef: "credential://system/mcp/github",
		Headers:       map[string]string{"X-MCP-Toolsets": "default,actions"},
		Timeout:       "5s",
	}, WithAuthValue("test-token"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result, err := client.Test(t.Context())
	if err != nil {
		t.Fatalf("Test() error = %v", err)
	}
	if result.Initialize.ServerInfo.Name != "test-mcp" {
		t.Fatalf("server name = %q", result.Initialize.ServerInfo.Name)
	}
	if len(result.Tools) != 1 || result.Tools[0].Name != "get_file" {
		t.Fatalf("tools = %#v", result.Tools)
	}
	if !sawSession {
		t.Fatalf("expected client to reuse MCP session id")
	}
	if !sawProtocol {
		t.Fatalf("expected client to send negotiated MCP protocol version")
	}
	callResult, err := client.CallTool(t.Context(), "get_file", json.RawMessage(`{"path":"README.md"}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if !json.Valid(callResult) {
		t.Fatalf("CallTool() returned invalid JSON: %s", string(callResult))
	}
}

func TestSSEJSONForIDSkipsUnrelatedMessages(t *testing.T) {
	body := []byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\"}\n\n" +
		"event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":99,\"result\":{\"ok\":false}}\n\n" +
		"event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":7,\"result\":{\"ok\":true}}\n\n")

	got, err := sseJSONForID(body, int64(7))
	if err != nil {
		t.Fatalf("sseJSONForID() error = %v", err)
	}
	if string(got) != `{"jsonrpc":"2.0","id":7,"result":{"ok":true}}` {
		t.Fatalf("sseJSONForID() = %s", string(got))
	}
}

func TestFirstSSEJSON(t *testing.T) {
	body := []byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"result\":{\"ok\":true}}\n\n")
	got, err := firstSSEJSON(body)
	if err != nil {
		t.Fatalf("firstSSEJSON() error = %v", err)
	}
	if string(got) != `{"jsonrpc":"2.0","result":{"ok":true}}` {
		t.Fatalf("firstSSEJSON() = %s", string(got))
	}
}

func TestFirstSSEJSONAllowsLongDataLines(t *testing.T) {
	payload := `{"jsonrpc":"2.0","result":{"text":"` + strings.Repeat("x", 128*1024) + `"}}`
	body := []byte("event: message\ndata: " + payload + "\n\n")
	got, err := firstSSEJSON(body)
	if err != nil {
		t.Fatalf("firstSSEJSON() error = %v", err)
	}
	if string(got) != payload {
		t.Fatalf("firstSSEJSON() returned unexpected payload length %d, want %d", len(got), len(payload))
	}
}

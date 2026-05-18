package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"nopsai/pkg/models"
)

const protocolVersion = "2025-06-18"

type Client struct {
	server          models.MCPServer
	authValue       string
	http            *http.Client
	sessionID       string
	protocolVersion string
	nextID          atomic.Int64
}

type InitializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

type Tool struct {
	Name           string          `json:"name"`
	Description    string          `json:"description,omitempty"`
	InputSchema    json.RawMessage `json:"inputSchema,omitempty"`
	InputSchemaAlt json.RawMessage `json:"input_schema,omitempty"`
}

type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type TestResult struct {
	Initialize InitializeResult
	Tools      []models.MCPTool
}

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func New(server models.MCPServer, opts ...Option) (*Client, error) {
	server = models.NormalizeMCPServer(server)
	if strings.TrimSpace(server.URL) == "" {
		return nil, fmt.Errorf("MCP server %q is missing url", server.Name)
	}
	if server.Transport != models.MCPTransportStreamableHTTP && server.Transport != models.MCPTransportHTTP {
		return nil, fmt.Errorf("MCP server %q uses unsupported transport %q", server.Name, server.Transport)
	}

	timeout := 30 * time.Second
	if parsed, err := time.ParseDuration(server.Timeout); err == nil && parsed > 0 {
		timeout = parsed
	}
	client := &Client{
		server:    server,
		authValue: resolveAuthSecret(server.AuthSecret),
		http:      &http.Client{Timeout: timeout},
	}
	for _, opt := range opts {
		opt(client)
	}
	return client, nil
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(client *Client) {
		if httpClient != nil {
			client.http = httpClient
		}
	}
}

func WithAuthValue(value string) Option {
	return func(client *Client) {
		client.authValue = strings.TrimSpace(value)
	}
}

func (c *Client) Initialize(ctx context.Context) (InitializeResult, error) {
	params := map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name":    "nopsai",
			"version": "dev",
		},
	}
	var result InitializeResult
	if err := c.call(ctx, "initialize", params, &result); err != nil {
		return InitializeResult{}, err
	}
	c.protocolVersion = strings.TrimSpace(result.ProtocolVersion)
	_ = c.notify(ctx, "notifications/initialized", map[string]any{})
	return result, nil
}

func (c *Client) ListTools(ctx context.Context) ([]models.MCPTool, error) {
	var tools []models.MCPTool
	var cursor string
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var result ListToolsResult
		if err := c.call(ctx, "tools/list", params, &result); err != nil {
			return nil, err
		}
		seenAt := time.Now().UTC()
		for _, tool := range result.Tools {
			schema := tool.InputSchema
			if len(schema) == 0 {
				schema = tool.InputSchemaAlt
			}
			inputSchema := models.CanonicalJSON(schema)
			tools = append(tools, models.MCPTool{
				ServerName:  c.server.Name,
				Name:        strings.TrimSpace(tool.Name),
				Description: strings.TrimSpace(tool.Description),
				InputSchema: inputSchema,
				SchemaHash:  models.MCPToolSchemaHash(inputSchema),
				LastSeenAt:  seenAt,
			})
		}
		cursor = strings.TrimSpace(result.NextCursor)
		if cursor == "" {
			break
		}
	}
	return tools, nil
}

func (c *Client) CallTool(ctx context.Context, toolName string, arguments json.RawMessage) (json.RawMessage, error) {
	var args any = map[string]any{}
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &args); err != nil {
			return nil, fmt.Errorf("invalid MCP tool arguments: %w", err)
		}
	}
	params := map[string]any{
		"name":      strings.TrimSpace(toolName),
		"arguments": args,
	}
	var result json.RawMessage
	if err := c.call(ctx, "tools/call", params, &result); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return json.RawMessage(`{}`), nil
	}
	return result, nil
}

func (c *Client) Test(ctx context.Context) (TestResult, error) {
	initResult, err := c.Initialize(ctx)
	if err != nil {
		return TestResult{}, err
	}
	tools, err := c.ListTools(ctx)
	if err != nil {
		return TestResult{}, err
	}
	return TestResult{Initialize: initResult, Tools: tools}, nil
}

func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	id := c.nextID.Add(1)
	payload := jsonRPCRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	body, err := c.send(ctx, payload)
	if err != nil {
		return err
	}
	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return fmt.Errorf("decode MCP response for %s: %w: %s", method, err, strings.TrimSpace(string(body)))
	}
	if rpcResp.Error != nil {
		return fmt.Errorf("MCP %s failed: %s", method, rpcResp.Error.Message)
	}
	if out == nil {
		return nil
	}
	if raw, ok := out.(*json.RawMessage); ok {
		*raw = append((*raw)[:0], rpcResp.Result...)
		return nil
	}
	if len(rpcResp.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(rpcResp.Result, out); err != nil {
		return fmt.Errorf("decode MCP result for %s: %w", method, err)
	}
	return nil
}

func (c *Client) notify(ctx context.Context, method string, params any) error {
	payload := jsonRPCRequest{JSONRPC: "2.0", Method: method, Params: params}
	_, err := c.send(ctx, payload)
	return err
}

func (c *Client) send(ctx context.Context, payload jsonRPCRequest) ([]byte, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.server.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	if c.protocolVersion != "" {
		req.Header.Set("MCP-Protocol-Version", c.protocolVersion)
	}
	for key, value := range c.server.Headers {
		if strings.TrimSpace(key) != "" {
			req.Header.Set(key, value)
		}
	}
	switch c.server.AuthType {
	case models.MCPAuthBearerToken:
		if c.authValue == "" {
			return nil, fmt.Errorf("MCP server %q auth secret %q is not set", c.server.Name, c.server.AuthSecret)
		}
		req.Header.Set("Authorization", "Bearer "+c.authValue)
	case models.MCPAuthNone:
	default:
		return nil, fmt.Errorf("MCP server %q uses unsupported auth_type %q", c.server.Name, c.server.AuthType)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call MCP server %q: %w", c.server.Name, err)
	}
	defer resp.Body.Close()

	if sessionID := strings.TrimSpace(resp.Header.Get("Mcp-Session-Id")); sessionID != "" {
		c.sessionID = sessionID
	}
	if sessionID := strings.TrimSpace(resp.Header.Get("MCP-Session-ID")); sessionID != "" {
		c.sessionID = sessionID
	}

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusNoContent {
		return []byte(`{"jsonrpc":"2.0","result":{}}`), nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("MCP server %q returned %s: %s", c.server.Name, resp.Status, strings.TrimSpace(string(body)))
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		return sseJSONForID(body, payload.ID)
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return []byte(`{"jsonrpc":"2.0","result":{}}`), nil
	}
	return trimmed, nil
}

func firstSSEJSON(body []byte) ([]byte, error) {
	return sseJSONForID(body, nil)
}

func sseJSONForID(body []byte, expectedID any) ([]byte, error) {
	var dataLines []string
	flush := func() ([]byte, bool) {
		if len(dataLines) == 0 {
			return nil, false
		}
		data := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = nil
		if data == "" || data == "[DONE]" {
			return nil, false
		}
		if !json.Valid([]byte(data)) {
			return nil, false
		}
		if expectedID == nil {
			return []byte(data), true
		}
		var response jsonRPCResponse
		if err := json.Unmarshal([]byte(data), &response); err != nil {
			return nil, false
		}
		if jsonRPCIDEqual(response.ID, expectedID) {
			return []byte(data), true
		}
		return nil, false
	}

	for _, rawLine := range bytes.Split(body, []byte{'\n'}) {
		line := strings.TrimRight(string(rawLine), "\r")
		if strings.TrimSpace(line) == "" {
			if data, ok := flush(); ok {
				return data, nil
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if data, ok := flush(); ok {
		return data, nil
	}
	if expectedID == nil {
		return nil, fmt.Errorf("SSE response did not contain a JSON-RPC message")
	}
	return nil, fmt.Errorf("SSE response did not contain JSON-RPC response id %v", expectedID)
}

func jsonRPCIDEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}
	aJSON, aErr := json.Marshal(a)
	bJSON, bErr := json.Marshal(b)
	if aErr != nil || bErr != nil {
		return fmt.Sprint(a) == fmt.Sprint(b)
	}
	return bytes.Equal(aJSON, bJSON)
}

func resolveAuthSecret(secretName string) string {
	secretName = strings.TrimSpace(secretName)
	if secretName == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(secretName))
}

func JSONString(raw json.RawMessage, maxBytes int) string {
	if maxBytes <= 0 || len(raw) <= maxBytes {
		return string(raw)
	}
	return string(raw[:maxBytes]) + "...[truncated " + strconv.Itoa(len(raw)-maxBytes) + " bytes]"
}

package nopsai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"nopsai/config"
	"nopsai/pkg/buildinfo"
	"nopsai/pkg/httpapi"
	aaamodel "nopsai/services/aaa/pkg/model"
)

const hostedMCPProtocolVersion = "2025-06-18"

// Versions this server can speak. The newest is what it offers when a client
// asks for something it does not know.
var hostedMCPSupportedProtocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// A page big enough that most clients need one request, small enough that a
// context-constrained client can ask for less. Clients must follow nextCursor.
const hostedMCPListPageSize = 100

type hostedMCPRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type hostedMCPResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *hostedMCPError `json:"error,omitempty"`
}

type hostedMCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type hostedMCPToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type hostedMCPResourceReadParams struct {
	URI string `json:"uri"`
}

func (a *App) registerHostedMCPRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/mcp", a.handleHostedMCP)
}

func (a *App) handleHostedMCP(w http.ResponseWriter, r *http.Request) {
	// After initialization a client states the version it negotiated. Accepting a
	// version we do not speak would let both sides believe they agreed, so this is
	// answered before anything else: it is a fact about the request, not about
	// whether this install has an assistant configured.
	if header := strings.TrimSpace(r.Header.Get("MCP-Protocol-Version")); header != "" && !hostedMCPProtocolVersionSupported(header) {
		http.Error(w, "unsupported MCP-Protocol-Version: "+header, http.StatusBadRequest)
		return
	}
	if !a.requireAssistantEnabled(w) {
		return
	}
	if !a.requireHostedMCPEnabled(w) {
		return
	}
	userID, ok := a.requireAssistantUserID(w, r)
	if !ok {
		return
	}
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "missing authorization subject", http.StatusUnauthorized)
		return
	}

	req, err := decodeHostedMCPRequest(r)
	if err != nil {
		writeHostedMCPError(w, nil, -32700, "invalid JSON-RPC payload")
		return
	}
	if req.ID == nil && strings.HasPrefix(req.Method, "notifications/") {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	writeHostedMCPResponse(w, a.processHostedMCPRequest(r.Context(), subject, userID, req))
}

func (a *App) requireHostedMCPEnabled(w http.ResponseWriter) bool {
	if !config.AssistantMCPEnabled(a.assistantConfig().MCP) {
		http.Error(w, "hosted MCP is disabled", http.StatusNotFound)
		return false
	}
	return true
}

func decodeHostedMCPRequest(r *http.Request) (hostedMCPRequest, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return hostedMCPRequest{}, err
	}
	if strings.TrimSpace(string(raw)) == "" {
		return hostedMCPRequest{JSONRPC: "2.0", Method: "tools/list"}, nil
	}

	var req hostedMCPRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&req); err != nil {
		return hostedMCPRequest{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return hostedMCPRequest{}, errors.New("invalid trailing JSON-RPC payload")
	} else if !errors.Is(err, io.EOF) {
		return hostedMCPRequest{}, err
	}
	req.Method = strings.TrimSpace(req.Method)
	return req, nil
}

func (a *App) processHostedMCPRequest(ctx context.Context, subject aaamodel.Subject, userID string, req hostedMCPRequest) hostedMCPResponse {
	req.Method = strings.TrimSpace(req.Method)
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		return hostedMCPErrorResponse(req.ID, -32600, "jsonrpc must be 2.0")
	}

	switch req.Method {
	case "initialize":
		return hostedMCPResultResponse(req.ID, map[string]any{
			"protocolVersion": hostedMCPNegotiatedProtocolVersion(req.Params),
			// listChanged is not advertised: this transport is request/response,
			// so there is no channel to send the notification on. Claiming it
			// would tell a client to wait for something that never arrives.
			"capabilities": map[string]any{
				"tools":       map[string]any{},
				"resources":   map[string]any{},
				"prompts":     map[string]any{},
				"completions": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "nopsai-hosted-mcp",
				"title":   "NopsAI",
				"version": buildinfo.Current().Version,
			},
			"instructions": hostedMCPServerInstructions,
		})
	case "ping":
		return hostedMCPResultResponse(req.ID, map[string]any{})
	case "tools/list":
		tools := a.hostedMCPToolsForSubject(ctx, subject)
		page, nextCursor := hostedMCPPageTools(tools, hostedMCPListCursor(req.Params))
		described := make([]map[string]any, 0, len(page))
		for _, tool := range page {
			described = append(described, hostedMCPDescribeTool(tool))
		}
		result := map[string]any{"tools": described}
		if nextCursor != "" {
			result["nextCursor"] = nextCursor
		}
		return hostedMCPResultResponse(req.ID, result)
	case "tools/call":
		var params hostedMCPToolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return hostedMCPErrorResponse(req.ID, -32602, "invalid tools/call params")
		}
		params.Name = strings.TrimSpace(params.Name)
		if params.Arguments == nil {
			params.Arguments = map[string]any{}
		}
		tool, ok := a.hostedMCPToolByName(ctx, subject, params.Name)
		if !ok {
			return hostedMCPErrorResponse(req.ID, -32601, "tool is not available")
		}
		conversationID := hostedMCPConversationID(params.Arguments)
		result, err := a.callHostedMCPTool(ctx, subject, userID, tool, params.Arguments, conversationID)
		if err != nil {
			return hostedMCPResultResponse(req.ID, hostedMCPToolResult(map[string]any{"error": err.Error()}, true))
		}
		return hostedMCPResultResponse(req.ID, hostedMCPToolResult(result, false))
	case "resources/list":
		resources := a.hostedMCPResourcesForSubject(ctx, subject)
		page, nextCursor := hostedMCPPageResources(resources, hostedMCPListCursor(req.Params))
		result := map[string]any{"resources": page}
		if nextCursor != "" {
			result["nextCursor"] = nextCursor
		}
		return hostedMCPResultResponse(req.ID, result)
	case "completion/complete":
		return hostedMCPResultResponse(req.ID, a.hostedMCPCompletion(ctx, subject, hostedMCPCompletionParamsFrom(req.Params)))
	case "prompts/list":
		prompts := a.hostedMCPPromptsForSubject(ctx, subject)
		return hostedMCPResultResponse(req.ID, map[string]any{"prompts": prompts})
	case "prompts/get":
		params := hostedMCPPromptGetParamsFrom(req.Params)
		prompt, ok := a.hostedMCPPromptByName(ctx, subject, params.Name)
		if !ok {
			return hostedMCPErrorResponse(req.ID, -32602, fmt.Sprintf("prompt %q is not available", params.Name))
		}
		if missing := hostedMCPPromptMissingArguments(prompt, params.Arguments); len(missing) > 0 {
			return hostedMCPErrorResponse(req.ID, -32602, fmt.Sprintf("prompt %q requires %s", prompt.Name, strings.Join(missing, ", ")))
		}
		return hostedMCPResultResponse(req.ID, hostedMCPPromptMessages(prompt, params.Arguments))
	case "resources/templates/list":
		return hostedMCPResultResponse(req.ID, map[string]any{
			"resourceTemplates": a.hostedMCPResourceTemplatesForSubject(ctx, subject),
		})
	case "resources/read":
		var params hostedMCPResourceReadParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return hostedMCPErrorResponse(req.ID, -32602, "invalid resources/read params")
		}
		resource, text, err := a.readHostedMCPResource(ctx, subject, params.URI)
		if err != nil {
			return hostedMCPErrorResponse(req.ID, -32000, err.Error())
		}
		return hostedMCPResultResponse(req.ID, map[string]any{
			"contents": []map[string]any{{
				"uri":      resource.URI,
				"mimeType": resource.MimeType,
				"text":     text,
			}},
		})
	default:
		return hostedMCPErrorResponse(req.ID, -32601, fmt.Sprintf("method %q is not supported", req.Method))
	}
}

func hostedMCPToolResult(payload map[string]any, isError bool) map[string]any {
	raw, err := json.MarshalIndent(payload, "", "  ")
	text := strings.TrimSpace(string(raw))
	if err != nil || text == "" {
		text = fmt.Sprint(payload)
	}
	return map[string]any{
		"content": []map[string]any{{
			"type": "text",
			"text": text,
		}},
		"structuredContent": payload,
		"isError":           isError,
	}
}

func hostedMCPConversationID(arguments map[string]any) *uuid.UUID {
	raw := stringArg(arguments, "conversation_id")
	if raw == "" {
		return nil
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return &parsed
}

func writeHostedMCPResult(w http.ResponseWriter, id any, result any) {
	writeHostedMCPResponse(w, hostedMCPResultResponse(id, result))
}

func writeHostedMCPError(w http.ResponseWriter, id any, code int, message string) {
	writeHostedMCPResponse(w, hostedMCPErrorResponse(id, code, message))
}

func writeHostedMCPResponse(w http.ResponseWriter, response hostedMCPResponse) {
	_ = httpapi.WriteJSON(w, http.StatusOK, response)
}

func hostedMCPResultResponse(id any, result any) hostedMCPResponse {
	return hostedMCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

func hostedMCPErrorResponse(id any, code int, message string) hostedMCPResponse {
	return hostedMCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &hostedMCPError{
			Code:    code,
			Message: message,
		},
	}
}

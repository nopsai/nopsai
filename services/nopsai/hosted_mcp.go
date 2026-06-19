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
	"nopsai/pkg/httpapi"
	aaamodel "nopsai/services/aaa/pkg/model"
)

const hostedMCPProtocolVersion = "2025-06-18"

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
			"protocolVersion": hostedMCPProtocolVersion,
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": true},
				"resources": map[string]any{"listChanged": true},
			},
			"serverInfo": map[string]string{
				"name":    "nopsai-hosted-mcp",
				"version": "foundation",
			},
		})
	case "tools/list":
		tools := a.hostedMCPToolsForSubject(ctx, subject)
		return hostedMCPResultResponse(req.ID, map[string]any{"tools": tools})
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
		return hostedMCPResultResponse(req.ID, map[string]any{"resources": resources})
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

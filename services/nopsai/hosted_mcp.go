package nopsai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"nopsai/pkg/httpapi"
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
	userID, ok := a.requireAssistantUserID(w, r)
	if !ok {
		return
	}
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "missing authorization subject", http.StatusUnauthorized)
		return
	}

	var req hostedMCPRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		writeHostedMCPError(w, nil, -32700, "invalid JSON-RPC payload")
		return
	}
	req.Method = strings.TrimSpace(req.Method)
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		writeHostedMCPError(w, req.ID, -32600, "jsonrpc must be 2.0")
		return
	}
	if req.ID == nil && strings.HasPrefix(req.Method, "notifications/") {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	switch req.Method {
	case "initialize":
		writeHostedMCPResult(w, req.ID, map[string]any{
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
		tools := a.hostedMCPToolsForSubject(r.Context(), subject)
		writeHostedMCPResult(w, req.ID, map[string]any{"tools": tools})
	case "tools/call":
		var params hostedMCPToolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			writeHostedMCPError(w, req.ID, -32602, "invalid tools/call params")
			return
		}
		params.Name = strings.TrimSpace(params.Name)
		if params.Arguments == nil {
			params.Arguments = map[string]any{}
		}
		tool, ok := a.hostedMCPToolByName(r.Context(), subject, params.Name)
		if !ok {
			writeHostedMCPError(w, req.ID, -32601, "tool is not available")
			return
		}
		conversationID := hostedMCPConversationID(params.Arguments)
		result, err := a.callHostedMCPTool(r.Context(), subject, userID, tool, params.Arguments, conversationID)
		if err != nil {
			writeHostedMCPResult(w, req.ID, hostedMCPToolResult(map[string]any{"error": err.Error()}, true))
			return
		}
		writeHostedMCPResult(w, req.ID, hostedMCPToolResult(result, false))
	case "resources/list":
		resources := a.hostedMCPResourcesForSubject(r.Context(), subject)
		writeHostedMCPResult(w, req.ID, map[string]any{"resources": resources})
	case "resources/read":
		var params hostedMCPResourceReadParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			writeHostedMCPError(w, req.ID, -32602, "invalid resources/read params")
			return
		}
		resource, text, err := a.readHostedMCPResource(r.Context(), subject, params.URI)
		if err != nil {
			writeHostedMCPError(w, req.ID, -32000, err.Error())
			return
		}
		writeHostedMCPResult(w, req.ID, map[string]any{
			"contents": []map[string]any{{
				"uri":      resource.URI,
				"mimeType": resource.MimeType,
				"text":     text,
			}},
		})
	default:
		writeHostedMCPError(w, req.ID, -32601, fmt.Sprintf("method %q is not supported", req.Method))
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
	_ = httpapi.WriteJSON(w, http.StatusOK, hostedMCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func writeHostedMCPError(w http.ResponseWriter, id any, code int, message string) {
	_ = httpapi.WriteJSON(w, http.StatusOK, hostedMCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &hostedMCPError{
			Code:    code,
			Message: message,
		},
	})
}

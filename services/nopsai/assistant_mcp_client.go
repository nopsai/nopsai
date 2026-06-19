package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	aaamodel "nopsai/services/aaa/pkg/model"
)

func (a *App) callAssistantHostedMCPTool(
	ctx context.Context,
	subject aaamodel.Subject,
	userID string,
	conversationID uuid.UUID,
	name string,
	args map[string]any,
) (map[string]any, error) {
	callArgs := cloneAssistantArgs(args)
	if conversationID != uuid.Nil {
		callArgs["conversation_id"] = conversationID.String()
	}
	params, err := json.Marshal(hostedMCPToolCallParams{
		Name:      strings.TrimSpace(name),
		Arguments: callArgs,
	})
	if err != nil {
		return nil, err
	}

	response := a.processHostedMCPRequest(ctx, subject, userID, hostedMCPRequest{
		JSONRPC: "2.0",
		ID:      "assistant-tool-call",
		Method:  "tools/call",
		Params:  params,
	})
	if response.Error != nil {
		return map[string]any{"error": response.Error.Message}, fmt.Errorf("%s", response.Error.Message)
	}

	result, ok := response.Result.(map[string]any)
	if !ok {
		return map[string]any{"error": "MCP tool returned an unexpected response"}, fmt.Errorf("MCP tool returned an unexpected response")
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		return map[string]any{"error": "MCP tool returned no structured content"}, fmt.Errorf("MCP tool returned no structured content")
	}
	if boolValue(result["isError"]) {
		message := assistantOutputString(structured, "error")
		if message == "" {
			message = "MCP tool returned an error"
		}
		return structured, fmt.Errorf("%s", message)
	}
	return structured, nil
}

package nopsai

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

type hostedMCPAuditRecord struct {
	UserID         string
	ConversationID *uuid.UUID
	ToolName       string
	Input          map[string]any
	Output         map[string]any
	ResourceScope  string
	Status         string
}

func (a *App) recordHostedMCPAudit(ctx context.Context, record hostedMCPAuditRecord) {
	if a == nil || a.db == nil {
		return
	}
	inputSummary := summarizeHostedMCPJSON(record.Input)
	outputSummary := summarizeHostedMCPJSON(record.Output)
	_, _ = a.db.Exec(ctx, `
		INSERT INTO hosted_mcp_audit_logs (
			user_id, conversation_id, tool_name, input_summary, output_summary, resource_scope, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, strings.TrimSpace(record.UserID), record.ConversationID, strings.TrimSpace(record.ToolName),
		inputSummary, outputSummary, strings.TrimSpace(record.ResourceScope), strings.TrimSpace(record.Status))
}

func summarizeHostedMCPJSON(value map[string]any) string {
	if len(value) == 0 {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	const maxSummaryLength = 600
	summary := strings.TrimSpace(string(raw))
	if len(summary) <= maxSummaryLength {
		return summary
	}
	return summary[:maxSummaryLength] + "..."
}

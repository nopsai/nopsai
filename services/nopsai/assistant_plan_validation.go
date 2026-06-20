package nopsai

import (
	"fmt"
	"strings"
)

func assistantValidatePlannerFinalAnswer(_ assistantTurnPlan, toolCalls []assistantToolActivity) error {
	for _, call := range assistantEvidenceToolCalls(toolCalls) {
		if call.Status == assistantToolStatusSuccess {
			return nil
		}
	}
	if len(toolCalls) == 0 {
		return fmt.Errorf("assistant planner cannot answer without successful hosted MCP evidence")
	}
	return fmt.Errorf("assistant planner cannot answer without successful hosted MCP evidence")
}

func assistantPlanIncludesTool(plan assistantTurnPlan, toolName string) bool {
	for _, step := range plan.Steps {
		if strings.TrimSpace(step.ToolName) == toolName {
			return true
		}
	}
	return false
}

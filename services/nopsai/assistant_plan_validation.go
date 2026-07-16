package nopsai

import (
	"fmt"
	"strings"
)

func assistantValidatePlannerFinalAnswer(plan assistantTurnPlan, toolCalls []assistantToolActivity, conversation assistantConversation) error {
	for _, call := range assistantEvidenceToolCalls(toolCalls) {
		if call.Status == assistantToolStatusSuccess {
			return nil
		}
	}
	if assistantConversationHasSuccessfulEvidence(conversation) {
		if assistantPlannerFinalAnswerHasDerivedProvenance(plan.FinalAnswer) {
			return nil
		}
		return fmt.Errorf("assistant planner final answer from prior evidence must label the data source and estimate confidence")
	}
	if len(toolCalls) == 0 {
		return fmt.Errorf("assistant planner cannot answer without successful hosted MCP evidence")
	}
	return fmt.Errorf("assistant planner cannot answer without successful hosted MCP evidence")
}

func assistantConversationHasSuccessfulEvidence(conversation assistantConversation) bool {
	for _, message := range conversation.Messages {
		for _, call := range assistantEvidenceToolCalls(message.ToolCalls) {
			if call.Status == assistantToolStatusSuccess {
				return true
			}
		}
	}
	return false
}

func assistantPlannerFinalAnswerHasDerivedProvenance(answer string) bool {
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "" {
		return false
	}
	hasSource := containsAny(answer,
		"source:",
		"data source",
		"mcp-backed",
		"mcp evidence",
		"previous evidence",
		"prior evidence",
		"previous mcp",
		"prior mcp",
		"same-chat",
		"conversation evidence",
	)
	hasConfidence := containsAny(answer,
		"confidence",
		"estimate",
		"estimated",
		"estimation",
		"assumption",
		"assumptions",
		"llm-derived",
		"derived",
		"calculation",
		"calculated",
		"formula",
	)
	return hasSource && hasConfidence
}

func assistantPlanIncludesTool(plan assistantTurnPlan, toolName string) bool {
	for _, step := range plan.Steps {
		if strings.TrimSpace(step.ToolName) == toolName {
			return true
		}
	}
	return false
}

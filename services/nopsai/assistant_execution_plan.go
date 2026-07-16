package nopsai

import (
	"fmt"
	"strings"
)

func assistantExecutionPlanActivity(plan assistantTurnPlan, status string) assistantToolActivity {
	executionPlan := assistantExecutionPlanFromTurnPlan(plan)
	return assistantToolActivity{
		Name:   assistantExecutionPlanToolName,
		Status: status,
		Input:  assistantPlanActivityInput(plan),
		Output: map[string]any{
			"execution_plan": executionPlan,
			"applied":        false,
		},
		ResourceURIs: []string{"nopsai://assistant/execution-plan"},
		Source:       "llm",
		Phase:        "planning",
		Confidence:   "medium",
		Purpose:      "Show the user which parts of the answer will come from MCP evidence, docs, prior evidence, and LLM synthesis.",
	}
}

func assistantExecutionPlanFromTurnPlan(plan assistantTurnPlan) assistantExecutionPlan {
	executionPlan := assistantExecutionPlan{
		Goal:                 strings.TrimSpace(plan.Goal),
		Intent:               strings.TrimSpace(plan.Intent),
		Summary:              assistantExecutionPlanSummary(plan),
		RequiresConfirmation: assistantExecutionPlanRequiresConfirmation(plan),
	}
	if executionPlan.Goal == "" {
		executionPlan.Goal = strings.TrimSpace(plan.LowerContent)
	}
	if executionPlan.Intent == "" {
		executionPlan.Intent = "llm_planned"
	}
	if len(plan.Steps) > 0 {
		for idx, step := range plan.Steps {
			executionPlan.Steps = append(executionPlan.Steps, assistantExecutionPlanStepFromPlanStep(idx+1, step))
		}
		executionPlan.Steps = append(executionPlan.Steps, assistantExecutionPlanStep{
			Index:      len(executionPlan.Steps) + 1,
			Title:      "Synthesize the answer from returned evidence",
			Source:     "llm",
			Phase:      "synthesis",
			Confidence: "medium",
			Reason:     "Use the validated tool outputs without inventing data or applied changes.",
			Status:     "planned",
		})
		return executionPlan
	}
	switch {
	case strings.TrimSpace(plan.ClarifyQuestion) != "":
		executionPlan.Steps = append(executionPlan.Steps, assistantExecutionPlanStep{
			Index:      1,
			Title:      "Ask for the missing detail",
			Source:     "llm",
			Phase:      "clarification",
			Confidence: "high",
			Reason:     "A safe tool plan needs one more user-provided constraint.",
			Status:     "planned",
		})
	case strings.TrimSpace(plan.FinalAnswer) != "":
		executionPlan.Steps = append(executionPlan.Steps, assistantExecutionPlanStep{
			Index:      1,
			Title:      "Answer from available conversation evidence",
			Source:     "llm",
			Phase:      "analysis",
			Confidence: "medium",
			Reason:     "Use prior same-chat MCP evidence and label calculations, estimates, or assumptions.",
			Status:     "planned",
		})
	default:
		executionPlan.Steps = append(executionPlan.Steps, assistantExecutionPlanStep{
			Index:      1,
			Title:      "Prepare a bounded answer",
			Source:     "llm",
			Phase:      "planning",
			Confidence: "medium",
			Reason:     "No external evidence step was selected for this turn.",
			Status:     "planned",
		})
	}
	return executionPlan
}

func assistantExecutionPlanStepFromPlanStep(index int, step assistantPlanStep) assistantExecutionPlanStep {
	tool := strings.TrimSpace(step.ToolName)
	source, phase, confidence := assistantExecutionSourceForTool(tool)
	title := assistantExecutionStepTitle(step, source)
	return assistantExecutionPlanStep{
		Index:      index,
		Title:      title,
		Source:     source,
		Phase:      phase,
		Confidence: confidence,
		Tool:       tool,
		Reason:     strings.TrimSpace(step.Thought),
		Status:     "planned",
	}
}

func assistantExecutionPlanSummary(plan assistantTurnPlan) string {
	if strings.TrimSpace(plan.FinalAnswer) != "" && len(plan.Steps) == 0 {
		return "Use prior permission-bound evidence and clearly label any LLM-derived estimate or assumption."
	}
	if strings.TrimSpace(plan.ClarifyQuestion) != "" && len(plan.Steps) == 0 {
		return "Ask a clarifying question before using tools."
	}
	if len(plan.Steps) == 0 {
		return "Prepare a bounded answer without external tool execution."
	}
	return "Use the validated plan first, then synthesize a concise answer from the returned evidence."
}

func assistantExecutionPlanRequiresConfirmation(plan assistantTurnPlan) bool {
	for _, step := range plan.Steps {
		if boolArg(step.Args, "confirm", false) || assistantExecutionToolLooksMutating(step.ToolName) {
			return true
		}
	}
	return false
}

func assistantExecutionToolLooksMutating(tool string) bool {
	tool = strings.TrimSpace(tool)
	if assistantPlannedToolIsProposal(tool) {
		return false
	}
	return strings.Contains(tool, ".create_") ||
		strings.Contains(tool, ".update_") ||
		strings.Contains(tool, ".delete_") ||
		strings.Contains(tool, ".write_") ||
		strings.Contains(tool, ".run_") ||
		strings.Contains(tool, ".sync_") ||
		strings.Contains(tool, ".cancel_") ||
		strings.Contains(tool, ".rotate_") ||
		strings.Contains(tool, ".activate_") ||
		strings.Contains(tool, ".disable_") ||
		strings.Contains(tool, ".enable_")
}

func assistantExecutionSourceForTool(tool string) (string, string, string) {
	tool = strings.TrimSpace(tool)
	switch {
	case tool == "":
		return "llm", "planning", "medium"
	case strings.Contains(tool, "_knowledge_context") || strings.Contains(tool, "knowledge"):
		return "knowledge_context", "evidence", "high"
	case strings.Contains(tool, "search_docs") || strings.Contains(tool, "read_doc"):
		return "docs", "evidence", "high"
	case strings.HasPrefix(tool, "nopsai.propose_") || strings.HasPrefix(tool, "nopsai.plan_") || strings.HasPrefix(tool, "nopsai.preview_"):
		return "gitops_proposal", "proposal", "high"
	default:
		return "mcp", "evidence", "high"
	}
}

func assistantExecutionStepTitle(step assistantPlanStep, source string) string {
	reason := strings.TrimSpace(step.Thought)
	if reason != "" {
		return assistantExecutionStepTitleFromReason(reason)
	}
	tool := strings.TrimSpace(step.ToolName)
	if tool == "" {
		return "Prepare assistant step"
	}
	switch source {
	case "docs":
		return "Read relevant documentation"
	case "knowledge_context":
		return "Read relevant knowledge context"
	case "gitops_proposal":
		return "Prepare a GitOps-safe proposal"
	default:
		return fmt.Sprintf("Run %s", tool)
	}
}

func assistantExecutionStepTitleFromReason(reason string) string {
	reason = strings.TrimSpace(strings.ReplaceAll(reason, "\n", " "))
	if reason == "" {
		return ""
	}
	if len(reason) <= 96 {
		return reason
	}
	return strings.TrimSpace(reason[:93]) + "..."
}

func assistantAnnotateToolActivityFromPlanStep(call assistantToolActivity, step assistantPlanStep, index int) assistantToolActivity {
	source, phase, confidence := assistantExecutionSourceForTool(call.Name)
	if call.Status == assistantToolStatusError || call.Status == assistantToolStatusDenied {
		confidence = "low"
	}
	call.Source = source
	call.Phase = phase
	call.Confidence = confidence
	call.Purpose = strings.TrimSpace(step.Thought)
	if call.Purpose == "" {
		call.Purpose = assistantExecutionStepTitle(step, source)
	}
	if call.Output == nil {
		call.Output = map[string]any{}
	}
	call.Output["execution_step"] = index
	call.Output["source"] = call.Source
	call.Output["phase"] = call.Phase
	call.Output["confidence"] = call.Confidence
	return call
}

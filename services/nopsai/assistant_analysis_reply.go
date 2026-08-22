package nopsai

import (
	"fmt"
	"strings"
)

const (
	assistantAnalysisMaxFindings = 5
	assistantAnalysisMaxEvidence = 3
)

// composeAnalysisReply turns an analysis tool result into the answer a reviewer
// would write: the score, what is wrong in severity order, and the one call that
// investigates the top finding. It runs before LLM synthesis so the model has a
// correct draft to improve rather than raw metrics to interpret, and it stands
// on its own when no model is available.
func composeAnalysisReply(toolCalls []assistantToolActivity) string {
	call, ok := assistantFirstAnalysisToolCall(toolCalls)
	if !ok {
		return ""
	}
	output := call.Output
	if message := assistantOutputString(output, "error"); message != "" {
		return assistantAnalysisErrorReply(output, message)
	}

	lines := []string{}
	if summary := assistantOutputString(output, "summary"); summary != "" {
		lines = append(lines, summary)
	}

	findings := analysisReplyRows(output, "findings")
	if len(findings) == 0 {
		lines = append(lines, "", "No findings were raised for this window.")
	} else {
		lines = append(lines, "", "## Findings")
		for index, finding := range findings {
			if index >= assistantAnalysisMaxFindings {
				lines = append(lines, fmt.Sprintf("- %d more finding%s are in the analysis result.", len(findings)-assistantAnalysisMaxFindings, plural(len(findings)-assistantAnalysisMaxFindings)))
				break
			}
			lines = append(lines, assistantAnalysisFindingLines(index+1, finding)...)
		}
	}

	if actions := analysisReplyRows(output, "next_actions"); len(actions) > 0 {
		lines = append(lines, "", "## Next step")
		for index, action := range actions {
			if index >= 3 {
				break
			}
			label := analysisString(action, "label")
			tool := analysisString(action, "tool")
			if label == "" && tool == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("- %s (`%s`)", label, tool))
		}
	}

	lines = append(lines, "", assistantAnalysisProvenance(call, output))
	if limitations := analysisReplyStrings(output, "limitations"); len(limitations) > 0 {
		lines = append(lines, "Limitations: "+strings.Join(limitations, " "))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func assistantAnalysisFindingLines(position int, finding map[string]any) []string {
	title := analysisString(finding, "title")
	severity := analysisString(finding, "severity")
	category := analysisString(finding, "category")
	header := fmt.Sprintf("%d. **%s**", position, title)
	if severity != "" || category != "" {
		header += fmt.Sprintf(" (%s)", strings.Trim(strings.Join([]string{severity, category}, " · "), " ·"))
	}
	lines := []string{header}
	if summary := analysisString(finding, "summary"); summary != "" {
		lines = append(lines, "   "+summary)
	}
	for index, item := range analysisReplyRows(finding, "evidence") {
		if index >= assistantAnalysisMaxEvidence {
			break
		}
		label := analysisString(item, "label")
		value := analysisString(item, "value")
		if label == "" && value == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("   - %s: %s", label, value))
	}
	for index, item := range analysisReplyRows(finding, "recommendations") {
		if index >= 1 {
			break
		}
		title := analysisString(item, "title")
		detail := analysisString(item, "detail")
		lines = append(lines, strings.TrimSpace(fmt.Sprintf("   Recommended: %s. %s", title, detail)))
	}
	return lines
}

func assistantAnalysisErrorReply(output map[string]any, message string) string {
	lines := []string{"I could not run that analysis: " + message + " No changes were applied."}
	if choices := analysisReplyRows(output, "available_teams"); len(choices) > 0 {
		lines = append(lines, "", "Teams you can analyse:")
		for index, choice := range choices {
			if index >= 10 {
				break
			}
			label := analysisString(choice, "label")
			path := analysisString(choice, "path")
			lines = append(lines, strings.TrimSpace(fmt.Sprintf("- %s %s", label, path)))
		}
	}
	return strings.Join(lines, "\n")
}

func assistantAnalysisProvenance(call assistantToolActivity, output map[string]any) string {
	window, _ := output["window"].(map[string]any)
	from := analysisString(window, "from")
	to := analysisString(window, "to")
	parts := []string{fmt.Sprintf("Data source: NopsAI monitoring evidence via `%s`", call.Name)}
	if from != "" && to != "" {
		parts = append(parts, fmt.Sprintf("window %s to %s", from, to))
	}
	return strings.Join(parts, ", ") + ". Findings and scores are deterministic, not model-derived."
}

func assistantFirstAnalysisToolCall(toolCalls []assistantToolActivity) (assistantToolActivity, bool) {
	for _, call := range toolCalls {
		if !assistantIsAnalysisTool(call.Name) {
			continue
		}
		if call.Status != assistantToolStatusSuccess || len(call.Output) == 0 {
			continue
		}
		return call, true
	}
	return assistantToolActivity{}, false
}

func assistantIsAnalysisTool(name string) bool {
	return name == "nopsai.analyze_team" || name == "nopsai.analyze_pipeline" || name == "nopsai.analyze_run"
}

// Tool output reaches this code either in-process (Go maps and slices) or after
// a database round trip (JSON types), so both shapes have to read the same.
func analysisReplyRows(container map[string]any, key string) []map[string]any {
	if len(container) == 0 {
		return nil
	}
	switch typed := container[key].(type) {
	case []map[string]any:
		return typed
	case []any:
		rows := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if row, ok := item.(map[string]any); ok {
				rows = append(rows, row)
			}
		}
		return rows
	default:
		return nil
	}
}

func analysisReplyStrings(container map[string]any, key string) []string {
	if len(container) == 0 {
		return nil
	}
	switch typed := container[key].(type) {
	case []string:
		return typed
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok && strings.TrimSpace(value) != "" {
				values = append(values, strings.TrimSpace(value))
			}
		}
		return values
	default:
		return nil
	}
}

package nopsai

import (
	"fmt"
	"sort"
	"strings"
)

func composeFeatureToolReply(toolCalls []assistantToolActivity) string {
	if len(toolCalls) == 0 {
		return "I did not find a NopsAI feature tool to call. No changes were applied."
	}
	if assistantAllToolsDenied(toolCalls) {
		return "I could not use the requested NopsAI feature tools with your current permissions. No changes were applied."
	}

	lines := []string{"NopsAI feature workflow:"}
	anyApplied := false
	for _, call := range toolCalls {
		label := assistantFeatureToolLabel(call.Name)
		if call.Status != assistantToolStatusSuccess {
			lines = append(lines, "", label+": "+assistantFeatureToolError(call))
			continue
		}
		lines = append(lines, "", label+":")
		if proposalType := assistantOutputString(call.Output, "proposal_type"); proposalType != "" {
			lines = append(lines, "- Prepared proposal: "+strings.ReplaceAll(proposalType, "_", " "))
			lines = append(lines, "- Applied: false")
		}
		if assistantOutputBool(call.Output, "requires_confirmation") {
			lines = append(lines, "- Confirmation required: rerun with confirm:true or explicit confirmation to execute this mutating action.")
			lines = append(lines, "- Applied: false")
		} else if _, ok := call.Output["applied"]; ok {
			applied := assistantOutputBool(call.Output, "applied")
			anyApplied = anyApplied || applied
			lines = append(lines, fmt.Sprintf("- Applied: %t", applied))
		} else if _, ok := call.Output["applies"]; ok {
			applies := assistantOutputBool(call.Output, "applies")
			anyApplied = anyApplied || applies
			lines = append(lines, fmt.Sprintf("- Applied: %t", applies))
		}
		if note := assistantOutputString(call.Output, "high_impact_note"); note != "" {
			lines = append(lines, "- High-impact note: "+note)
		}
		if method := assistantOutputString(call.Output, "method"); method != "" {
			route := strings.TrimSpace(method + " " + assistantOutputString(call.Output, "path"))
			lines = append(lines, "- Route: "+route)
		}
		if status := assistantOutputFloat(call.Output, "status_code"); status > 0 {
			lines = append(lines, fmt.Sprintf("- Status: %.0f", status))
		}
		lines = assistantAppendFeatureSummary(lines, call.Output)
	}
	if !anyApplied {
		lines = append(lines, "", "No changes were applied.")
	}
	return strings.Join(assistantCompactLines(lines), "\n")
}

func assistantFeatureToolLabel(name string) string {
	name = strings.TrimPrefix(strings.TrimSpace(name), "nopsai.")
	name = strings.ReplaceAll(name, "_", " ")
	if name == "" {
		return "feature tool"
	}
	return name
}

func assistantFeatureToolError(call assistantToolActivity) string {
	errText := assistantOutputString(call.Output, "error")
	if errText == "" {
		errText = strings.TrimSpace(call.Status)
	}
	if errText == "" {
		errText = "the tool did not return a successful result"
	}
	return errText + ". No changes were applied."
}

func assistantAppendFeatureSummary(lines []string, output map[string]any) []string {
	lines = assistantAppendFeatureFileSummary(lines, output)
	for _, key := range []string{
		"backups",
		"jobs",
		"schedules",
		"credentials",
		"users",
		"service_accounts",
		"roles",
		"identity_providers",
		"grants",
		"views",
		"alert_rules",
		"alert_events",
		"recommendations",
		"sources",
		"deliveries",
		"external_triggers",
		"invocations",
		"surfaces",
		"workflow",
		"profiles",
	} {
		lines = assistantAppendFeatureListSummary(lines, assistantFeatureSummaryTitle(key), output[key])
	}
	if response, ok := output["response"]; ok {
		lines = assistantAppendFeatureValueSummary(lines, "Response", response)
	}
	if text := assistantOutputString(output, "response_text"); text != "" {
		lines = append(lines, "- Response text: "+assistantTruncateForReply(text, 220))
	}
	if errText := assistantOutputString(output, "error"); errText != "" {
		lines = append(lines, "- Error: "+errText)
	}
	return lines
}

func assistantAppendFeatureFileSummary(lines []string, output map[string]any) []string {
	if paths := assistantStringSlice(output["file_paths"]); len(paths) > 0 {
		lines = append(lines, "- GitOps files: "+strings.Join(paths, ", "))
	}
	gitops, _ := output["gitops"].(map[string]any)
	if len(gitops) == 0 {
		return lines
	}
	if message := assistantOutputString(gitops, "message"); message != "" {
		lines = append(lines, "- Commit message: "+message)
	}
	files := assistantMapSlice(gitops["files"])
	if len(files) == 0 {
		return lines
	}
	paths := []string{}
	for _, file := range files {
		if path := assistantOutputString(file, "path"); path != "" {
			paths = append(paths, path)
		}
	}
	if len(paths) > 0 {
		lines = append(lines, "- GitOps files: "+strings.Join(paths, ", "))
	}
	return lines
}

func assistantAppendFeatureValueSummary(lines []string, title string, value any) []string {
	if rows := assistantMapSlice(value); len(rows) > 0 {
		return assistantAppendFeatureListSummary(lines, title, rows)
	}
	switch typed := value.(type) {
	case []any:
		return assistantAppendFeatureListSummary(lines, title, typed)
	case map[string]any:
		highSignal := assistantFeatureMapHighlights(typed)
		if len(highSignal) > 0 {
			lines = append(lines, "- "+title+": "+strings.Join(highSignal, ", "))
			return lines
		}
		keys := assistantFeatureVisibleKeys(typed)
		if len(keys) > 0 {
			lines = append(lines, "- "+title+" fields: "+strings.Join(keys, ", "))
		}
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			lines = append(lines, "- "+title+": "+assistantTruncateForReply(trimmed, 220))
		}
	default:
		if typed != nil {
			lines = append(lines, "- "+title+": "+assistantTruncateForReply(fmt.Sprint(typed), 220))
		}
	}
	return lines
}

func assistantAppendFeatureListSummary(lines []string, title string, value any) []string {
	items := assistantFeatureListItems(value)
	if len(items) == 0 {
		return lines
	}
	labels := []string{}
	for _, item := range items {
		if label := assistantFeatureItemLabel(item); label != "" {
			labels = append(labels, label)
		}
		if len(labels) >= 5 {
			break
		}
	}
	if len(labels) == 0 {
		lines = append(lines, fmt.Sprintf("- %s: %d item(s)", title, len(items)))
		return lines
	}
	lines = append(lines, fmt.Sprintf("- %s: %d item(s): %s", title, len(items), strings.Join(labels, ", ")))
	return lines
}

func assistantFeatureListItems(value any) []map[string]any {
	if rows := assistantMapSlice(value); len(rows) > 0 {
		return rows
	}
	switch typed := value.(type) {
	case []any:
		rows := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			switch mapped := item.(type) {
			case map[string]any:
				rows = append(rows, mapped)
			case string:
				rows = append(rows, map[string]any{"value": mapped})
			default:
				rows = append(rows, map[string]any{"value": fmt.Sprint(mapped)})
			}
		}
		return rows
	case []string:
		rows := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			rows = append(rows, map[string]any{"value": item})
		}
		return rows
	default:
		return nil
	}
}

func assistantFeatureItemLabel(item map[string]any) string {
	for _, key := range []string{
		"id",
		"name",
		"path",
		"reference",
		"credential_id",
		"schedule_id",
		"trigger_id",
		"source_id",
		"run_id",
		"email",
		"role",
		"area",
		"step",
		"tool",
		"value",
	} {
		if value := assistantOutputString(item, key); value != "" {
			return assistantTruncateForReply(value, 80)
		}
	}
	return ""
}

func assistantFeatureMapHighlights(value map[string]any) []string {
	highlights := []string{}
	for _, key := range []string{"status", "state", "enabled", "profile", "environment", "count", "total", "drift_status"} {
		if raw, ok := value[key]; ok && raw != nil {
			highlights = append(highlights, strings.ReplaceAll(key, "_", " ")+"="+assistantTruncateForReply(fmt.Sprint(raw), 80))
		}
	}
	return highlights
}

func assistantFeatureVisibleKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		if assistantFeatureSensitiveKey(key) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 8 {
		keys = keys[:8]
	}
	return keys
}

func assistantFeatureSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	return containsAny(normalized, "value", "secret", "token", "password", "content", "private", "credential_material")
}

func assistantFeatureSummaryTitle(key string) string {
	return strings.Title(strings.ReplaceAll(key, "_", " "))
}

func assistantTruncateForReply(value string, maxLen int) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxLen {
		return value
	}
	if maxLen <= 3 {
		return value[:maxLen]
	}
	return strings.TrimSpace(value[:maxLen-3]) + "..."
}

func assistantCompactLines(lines []string) []string {
	compacted := make([]string, 0, len(lines))
	previousBlank := false
	for _, line := range lines {
		blank := strings.TrimSpace(line) == ""
		if blank && previousBlank {
			continue
		}
		compacted = append(compacted, line)
		previousBlank = blank
	}
	return compacted
}

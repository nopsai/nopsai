package nopsai

import (
	"strings"

	"nopsai/services/nopsai/internal/systemlogs"
)

const assistantPromptValueLimit = 6000

var (
	assistantPromptValueRedactor   = systemlogs.NewRedactor(assistantPromptValueLimit)
	assistantPromptHistoryRedactor = systemlogs.NewRedactor(assistantPromptHistoryContentLimit)
)

func redactAssistantPromptValue(value string) string {
	return assistantPromptValueRedactor.Redact(strings.TrimSpace(value))
}

func redactAssistantPromptHistoryValue(value string) string {
	return assistantPromptHistoryRedactor.Redact(strings.TrimSpace(value))
}

func assistantPromptMemoryValue(memory assistantConversationMemory) map[string]any {
	memory = normalizeAssistantMemory(memory)
	return map[string]any{
		"summary":                 redactAssistantPromptValue(memory.Summary),
		"entities":                assistantPromptSafeValue(memory.Entities),
		"open_tasks":              assistantPromptSafeValue(memory.OpenTasks),
		"previous_proposed_fixes": assistantPromptSafeValue(memory.PreviousProposedFixes),
		"selected_run":            redactAssistantPromptValue(memory.SelectedRun),
		"selected_pipeline":       redactAssistantPromptValue(memory.SelectedPipeline),
		"selected_scope":          redactAssistantPromptValue(memory.SelectedScope),
		"selected_docs_version":   redactAssistantPromptValue(memory.SelectedDocsVersion),
	}
}

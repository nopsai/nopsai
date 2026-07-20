package app

import (
	"fmt"
	"strings"
)

const (
	maxLLMHistoryBytes    = 32000
	recentLLMHistoryBytes = 24000
)

func llmHistorySnapshot(history string) string {
	return llmHistorySnapshotWithRevision(history, 0)
}

func llmHistorySnapshotWithRevision(history string, revision uint64) string {
	prefix := ""
	if revision > 0 {
		prefix = fmt.Sprintf("history_revision: %d\n", revision)
	}
	if len([]byte(history)) <= maxLLMHistoryBytes {
		if strings.TrimSpace(history) == "" {
			return strings.TrimSpace(prefix)
		}
		return prefix + history
	}
	recent := strings.TrimSpace(tailBytes(history, recentLLMHistoryBytes))
	return fmt.Sprintf(
		"%sStable run summary:\nFull execution history is %d bytes and was compacted for this LLM request. Durable run history remains stored by NopsAI.\n\nRecent task events:\n%s",
		prefix,
		len([]byte(history)),
		recent,
	)
}

func tailBytes(value string, maxBytes int) string {
	if maxBytes <= 0 || len([]byte(value)) <= maxBytes {
		return value
	}
	bytes := []byte(value)
	return string(bytes[len(bytes)-maxBytes:])
}

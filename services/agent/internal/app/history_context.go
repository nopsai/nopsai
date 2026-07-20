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
	facts := compactHistoryFacts(history)
	if facts == "" {
		facts = "No structured prior-goal facts could be extracted."
	}
	return fmt.Sprintf(
		"%sStable run summary:\nFull execution history is %d bytes and was compacted for this LLM request. Durable run history remains stored by NopsAI.\n\nStructured previous-goal facts:\n%s\n\nRecent task events:\n%s",
		prefix,
		len([]byte(history)),
		facts,
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

func compactHistoryFacts(history string) string {
	blocks := strings.Split(history, "- Goal:")
	facts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		block = "Goal:" + block
		goal := firstHistoryValue(block, "Goal:")
		action := firstHistoryValue(block, "Action:")
		result := firstHistoryValue(block, "Result (Exit Code")
		exitCode := ""
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "Result (Exit Code") {
				continue
			}
			if start := strings.Index(line, "Result (Exit Code "); start >= 0 {
				rest := line[start+len("Result (Exit Code "):]
				if end := strings.Index(rest, ")"); end >= 0 {
					exitCode = strings.TrimSpace(rest[:end])
				}
			}
			break
		}
		if goal == "" && action == "" && result == "" {
			continue
		}
		result = strings.TrimSpace(result)
		if len(result) > 240 {
			result = result[:240] + "...[truncated]"
		}
		facts = append(facts, fmt.Sprintf("- goal=%q action=%q exit_code=%q result=%q", goal, action, exitCode, result))
	}
	if len(facts) == 0 {
		return ""
	}
	if len(facts) > 30 {
		facts = facts[len(facts)-30:]
	}
	return strings.Join(facts, "\n")
}

func firstHistoryValue(block, label string) string {
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, label) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, label))
		if label == "Result (Exit Code" {
			if idx := strings.Index(value, "):"); idx >= 0 {
				value = strings.TrimSpace(value[idx+len("):"):])
			}
		}
		return value
	}
	return ""
}

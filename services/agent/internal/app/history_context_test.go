package app

import (
	"strings"
	"testing"
)

func TestLLMHistorySnapshotKeepsSmallHistory(t *testing.T) {
	history := "- Goal: build\n  Result: ok\n"
	if got := llmHistorySnapshot(history); got != history {
		t.Fatalf("llmHistorySnapshot() = %q, want original history", got)
	}
}

func TestLLMHistorySnapshotWithRevisionAddsEnvelope(t *testing.T) {
	history := "- Goal: build\n  Result: ok\n"
	got := llmHistorySnapshotWithRevision(history, 12)

	if !strings.HasPrefix(got, "history_revision: 12\n") {
		t.Fatalf("snapshot missing history revision:\n%s", got)
	}
	if !strings.Contains(got, history) {
		t.Fatalf("snapshot missing original history:\n%s", got)
	}
}

func TestLLMHistorySnapshotCompactsLargeHistory(t *testing.T) {
	history := strings.Repeat("old event\n", 5000) + "recent event"
	got := llmHistorySnapshot(history)

	if !strings.Contains(got, "Stable run summary:") ||
		!strings.Contains(got, "Full execution history is") ||
		!strings.Contains(got, "Structured previous-goal facts:") ||
		!strings.Contains(got, "Recent task events:") ||
		!strings.Contains(got, "recent event") {
		t.Fatalf("compacted history missing expected summary:\n%s", got)
	}
	if len([]byte(got)) >= len([]byte(history)) {
		t.Fatalf("compacted history is not smaller: got %d original %d", len([]byte(got)), len([]byte(history)))
	}
}

func TestLLMHistorySnapshotExtractsStructuredFactsWhenCompacted(t *testing.T) {
	history := strings.Repeat("padding\n", 5000) +
		"- Goal: deploy\n  Action: run deploy\n  Result (Exit Code 0): shipped successfully\n" +
		"- Goal: verify\n  Action: curl health\n  Result (Exit Code 1): health check failed\n"
	got := llmHistorySnapshotWithRevision(history, 4)

	for _, want := range []string{
		"history_revision: 4",
		"Structured previous-goal facts:",
		`goal="deploy"`,
		`action="curl health"`,
		`exit_code="1"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("compacted history = %q, want %q", got, want)
		}
	}
}

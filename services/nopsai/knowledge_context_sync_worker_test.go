package nopsai

import (
	"testing"
	"time"
)

func TestKnowledgeSyncMetricsRecordAttemptsAndBlocks(t *testing.T) {
	var metrics knowledgeSyncMetrics

	metrics.recordAttempt("Notion", knowledgeSyncModePeriodic, "success", 1500*time.Millisecond)
	metrics.recordProviderRequest("Notion", "get_page", "success", 1500*time.Millisecond)
	metrics.recordBeforeRunBlock("Notion")

	attempts, durations, blocks := metrics.snapshot()
	attemptKey := knowledgeSyncMetricKey{Provider: "Notion", Mode: knowledgeSyncModePeriodic, Result: "success"}
	if attempts[attemptKey] != 1 {
		t.Fatalf("attempts[%#v] = %v, want 1", attemptKey, attempts[attemptKey])
	}
	if durations[attemptKey] != 1.5 {
		t.Fatalf("sync duration[%#v] = %v, want 1.5", attemptKey, durations[attemptKey])
	}
	requestKey := knowledgeSyncMetricKey{Provider: "Notion", Operation: "get_page", Result: "success"}
	if durations[requestKey] != 1.5 {
		t.Fatalf("provider duration[%#v] = %v, want 1.5", requestKey, durations[requestKey])
	}
	if blocks["Notion"] != 1 {
		t.Fatalf("before-run blocks = %v, want 1", blocks["Notion"])
	}
}

func TestKnowledgePeriodicSyncJobID(t *testing.T) {
	job := knowledgePeriodicSyncJob{Kind: "guardrail", Team: "security/platform", Name: "repo-check"}
	if got := job.ID(); got != "guardrail/security/platform/repo-check" {
		t.Fatalf("job ID = %q", got)
	}
}

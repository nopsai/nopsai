package nopsai

import (
	"strings"
	"testing"
)

func TestRunDispatchSchemaTracksRecoverableLaunchContext(t *testing.T) {
	joined := strings.Join(runDispatchSchemaStatements, "\n")
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS parent_runner_id",
		"ADD COLUMN IF NOT EXISTS parent_history",
		"ADD COLUMN IF NOT EXISTS runtime_variable_overrides JSONB",
		"ADD COLUMN IF NOT EXISTS runtime_sensitive_variable_overrides JSONB",
		"ADD COLUMN IF NOT EXISTS source TEXT",
		"ADD COLUMN IF NOT EXISTS request_id TEXT",
		"ADD COLUMN IF NOT EXISTS traceparent TEXT",
		"ADD COLUMN IF NOT EXISTS metadata JSONB",
		"idx_pipeline_runs_pending_recovery",
		"idx_pipeline_run_logs_request_id",
		"idx_pipeline_run_logs_source",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("run dispatch schema missing %q in:\n%s", want, joined)
		}
	}
}

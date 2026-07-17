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
		"idx_pipeline_runs_pending_recovery",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("run dispatch schema missing %q in:\n%s", want, joined)
		}
	}
}

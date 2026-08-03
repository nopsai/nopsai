package nopsai

import (
	"strings"
	"testing"
)

func TestScheduleSchemaSetsVisibilityDefaultBeforeCheck(t *testing.T) {
	joined := strings.Join(scheduleSchemaStatements, "\n")
	for _, statement := range []string{
		"ALTER TABLE pipeline_schedules ADD COLUMN IF NOT EXISTS run_team_path TEXT NOT NULL DEFAULT 'global'",
		"UPDATE pipeline_schedules",
		"ALTER TABLE pipeline_schedules ALTER COLUMN visibility SET DEFAULT 'team'",
	} {
		if !strings.Contains(joined, statement) {
			t.Fatalf("pipeline_schedules schema missing team statement %q", statement)
		}
	}

	drop := strings.Index(joined, "ALTER TABLE pipeline_schedules DROP CONSTRAINT IF EXISTS pipeline_schedules_visibility_check")
	defaultFix := strings.Index(joined, "ALTER TABLE pipeline_schedules ALTER COLUMN visibility SET DEFAULT 'team'")
	add := strings.Index(joined, "ALTER TABLE pipeline_schedules ADD CONSTRAINT pipeline_schedules_visibility_check")
	if defaultFix == -1 || drop == -1 || add == -1 || defaultFix > drop || drop > add {
		t.Fatalf("pipeline_schedules visibility migration order is wrong; default index %d drop index %d add index %d", defaultFix, drop, add)
	}
	assertTeamOnlySchemaVocabulary(t, joined)
}

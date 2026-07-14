package nopsai

import (
	"strings"
	"testing"
)

func TestScheduleSchemaSetsVisibilityDefaultBeforeCheck(t *testing.T) {
	joined := strings.Join(scheduleSchemaStatements, "\n")
	for _, statement := range []string{
		"SET run_team_path = run_group_path",
		"ALTER TABLE pipeline_schedules DROP COLUMN run_group_path",
	} {
		if !strings.Contains(joined, statement) {
			t.Fatalf("pipeline_schedules schema missing legacy run team migration %q", statement)
		}
	}

	drop := strings.Index(joined, "ALTER TABLE pipeline_schedules DROP CONSTRAINT IF EXISTS pipeline_schedules_visibility_check")
	defaultFix := strings.Index(joined, "ALTER TABLE pipeline_schedules ALTER COLUMN visibility SET DEFAULT 'team'")
	add := strings.Index(joined, "ALTER TABLE pipeline_schedules ADD CONSTRAINT pipeline_schedules_visibility_check")
	if defaultFix == -1 || drop == -1 || add == -1 || defaultFix > drop || drop > add {
		t.Fatalf("pipeline_schedules visibility migration order is wrong; default index %d drop index %d add index %d", defaultFix, drop, add)
	}
	for _, legacyFragment := range []string{
		"visibility = 'group'",
		"BTRIM(visibility)",
		"LOWER(BTRIM(visibility))",
	} {
		if strings.Contains(joined, legacyFragment) {
			t.Fatalf("pipeline_schedules visibility schema should not keep legacy compatibility fragment %q", legacyFragment)
		}
	}
}

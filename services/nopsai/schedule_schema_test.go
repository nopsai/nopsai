package nopsai

import (
	"strings"
	"testing"
)

func TestScheduleSchemaMigratesLegacyGroupVisibilityBeforeCheck(t *testing.T) {
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
	update := strings.Index(joined, "UPDATE pipeline_schedules SET visibility = 'team' WHERE visibility = 'group'")
	add := strings.Index(joined, "ALTER TABLE pipeline_schedules ADD CONSTRAINT pipeline_schedules_visibility_check")
	if drop == -1 || update == -1 || add == -1 || drop > update || update > add {
		t.Fatalf("pipeline_schedules visibility migration order is wrong; drop index %d update index %d add index %d", drop, update, add)
	}
}

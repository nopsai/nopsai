package nopsai

import (
	"strings"
	"testing"
)

func TestExternalTriggerSchemaMigratesLegacyRunGroupPath(t *testing.T) {
	joined := strings.Join(externalTriggerSchemaStatements, "\n")
	for _, statement := range []string{
		"SET run_team_path = run_group_path",
		"ALTER TABLE external_triggers DROP COLUMN run_group_path",
	} {
		if !strings.Contains(joined, statement) {
			t.Fatalf("external trigger schema missing legacy run team migration %q", statement)
		}
	}
}

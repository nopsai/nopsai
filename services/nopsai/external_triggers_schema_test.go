package nopsai

import (
	"strings"
	"testing"
)

func TestExternalTriggerSchemaUsesRunTeamPath(t *testing.T) {
	joined := strings.Join(externalTriggerSchemaStatements, "\n")
	for _, statement := range []string{
		"run_team_path TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE external_triggers ADD COLUMN IF NOT EXISTS run_team_path TEXT NOT NULL DEFAULT ''",
	} {
		if !strings.Contains(joined, statement) {
			t.Fatalf("external trigger schema missing run team statement %q", statement)
		}
	}
	assertTeamOnlySchemaVocabulary(t, joined)
}

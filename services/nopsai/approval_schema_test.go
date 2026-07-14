package nopsai

import (
	"strings"
	"testing"
)

func TestApprovalSchemaUsesAssignedTeams(t *testing.T) {
	joined := strings.Join(approvalSchemaStatements, "\n")
	for _, statement := range []string{
		"assigned_teams JSONB NOT NULL DEFAULT '[]'::jsonb",
		"ALTER TABLE pipeline_approvals ADD COLUMN IF NOT EXISTS assigned_teams JSONB NOT NULL DEFAULT '[]'::jsonb",
	} {
		if !strings.Contains(joined, statement) {
			t.Fatalf("approval schema missing assigned teams statement %q", statement)
		}
	}
	assertTeamOnlySchemaVocabulary(t, joined)
}

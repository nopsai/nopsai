package nopsai

import (
	"strings"
	"testing"
)

func TestApprovalSchemaMigratesLegacyAssignedGroups(t *testing.T) {
	joined := strings.Join(approvalSchemaStatements, "\n")
	for _, statement := range []string{
		"SET assigned_teams = assigned_groups",
		"ALTER TABLE pipeline_approvals DROP COLUMN assigned_groups",
	} {
		if !strings.Contains(joined, statement) {
			t.Fatalf("approval schema missing legacy assigned team migration %q", statement)
		}
	}
}

package nopsai

import (
	"strings"
	"testing"
)

func TestTeamSchemaMigratesLegacyGroupsTableAndRunColumn(t *testing.T) {
	joined := strings.Join(teamSchemaStatements, "\n")
	required := []string{
		"ALTER TABLE groups RENAME TO teams",
		"ALTER TABLE teams RENAME CONSTRAINT groups_pkey TO teams_pkey",
		"ALTER TABLE teams RENAME CONSTRAINT groups_name_key TO teams_name_key",
		"ALTER TABLE teams RENAME CONSTRAINT groups_parent_id_fkey TO teams_parent_id_fkey",
		"ALTER TABLE pipeline_runs RENAME COLUMN group_id TO team_id",
		"ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS team_id INTEGER REFERENCES teams(id) ON DELETE SET NULL",
		"ALTER TABLE teams DROP CONSTRAINT IF EXISTS groups_kind_check",
		"DROP INDEX IF EXISTS idx_groups_kind",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("team schema missing legacy team migration %q", statement)
		}
	}

	renameTable := strings.Index(joined, "ALTER TABLE groups RENAME TO teams")
	createTable := strings.Index(joined, "CREATE TABLE IF NOT EXISTS teams")
	renameRunColumn := strings.Index(joined, "ALTER TABLE pipeline_runs RENAME COLUMN group_id TO team_id")
	dropKindCheck := strings.Index(joined, "ALTER TABLE teams DROP CONSTRAINT IF EXISTS groups_kind_check")
	updateKind := strings.Index(joined, "UPDATE teams SET kind = 'team'")
	if renameTable == -1 || createTable == -1 || renameRunColumn == -1 || dropKindCheck == -1 || updateKind == -1 ||
		renameTable > createTable || createTable > renameRunColumn || renameRunColumn > dropKindCheck || dropKindCheck > updateKind {
		t.Fatalf("team schema migration order is wrong; table rename %d create %d run column rename %d drop kind %d update kind %d", renameTable, createTable, renameRunColumn, dropKindCheck, updateKind)
	}
}

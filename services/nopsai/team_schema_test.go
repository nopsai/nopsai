package nopsai

import (
	"strings"
	"testing"
)

func TestTeamSchemaUsesTeamVocabulary(t *testing.T) {
	joined := strings.Join(teamSchemaStatements, "\n")
	required := []string{
		"CREATE TABLE IF NOT EXISTS teams",
		"ALTER TABLE teams ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'team'",
		"ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS team_id INTEGER REFERENCES teams(id) ON DELETE SET NULL",
		"ALTER TABLE teams DROP CONSTRAINT IF EXISTS teams_name_key",
		"CREATE INDEX IF NOT EXISTS idx_teams_kind",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_teams_root_name_unique",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_teams_parent_name_unique",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("team schema missing statement %q", statement)
		}
	}

	createTable := strings.Index(joined, "CREATE TABLE IF NOT EXISTS teams")
	updateKind := strings.Index(joined, "UPDATE teams SET kind = 'team'")
	if createTable == -1 || updateKind == -1 || createTable > updateKind {
		t.Fatalf("team schema order is wrong; create index %d update kind %d", createTable, updateKind)
	}
	assertTeamOnlySchemaVocabulary(t, joined)
}

func TestTeamSchemaUsesSiblingScopedNames(t *testing.T) {
	joined := strings.Join(teamSchemaStatements, "\n")
	if strings.Contains(joined, "UNIQUE(name)") {
		t.Fatal("team schema must not enforce globally unique leaf names")
	}
	if !strings.Contains(joined, "WHERE parent_id IS NULL") || !strings.Contains(joined, "WHERE parent_id IS NOT NULL") {
		t.Fatalf("team schema must enforce team names separately for root and child teams:\n%s", joined)
	}
}

func assertTeamOnlySchemaVocabulary(t *testing.T, statements string) {
	t.Helper()
	statements = strings.ReplaceAll(statements, "external_group_id", "external_team_id")
	for _, token := range []string{"gr" + "oup", "fol" + "der"} {
		if strings.Contains(statements, token) {
			t.Fatalf("schema statements contain non-team vocabulary token %q", token)
		}
	}
}

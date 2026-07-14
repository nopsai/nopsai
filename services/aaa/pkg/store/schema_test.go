package store

import (
	"strings"
	"testing"
)

func TestAAASchemaUsesTeamVocabulary(t *testing.T) {
	joined := strings.Join(aaaSchemaStatements, "\n")
	required := []string{
		"CREATE TABLE IF NOT EXISTS auth_teams",
		"CREATE TABLE IF NOT EXISTS auth_team_members",
		"external_team_name TEXT NOT NULL DEFAULT ''",
		"auth_team_name TEXT NOT NULL DEFAULT ''",
		"CREATE INDEX IF NOT EXISTS idx_auth_team_members_subject",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("schema statements missing team statement %q", statement)
		}
	}

	dropIndex := strings.Index(joined, "ALTER TABLE access_grants DROP CONSTRAINT IF EXISTS access_grants_subject_type_check")
	checkIndex := strings.Index(joined, "ADD CONSTRAINT access_grants_subject_type_check")
	if dropIndex == -1 || checkIndex == -1 || dropIndex > checkIndex {
		t.Fatalf("access grant check order is wrong; drop index %d check index %d", dropIndex, checkIndex)
	}

	assertTeamOnlySchemaVocabulary(t, joined)
}

func assertTeamOnlySchemaVocabulary(t *testing.T, statements string) {
	t.Helper()
	for _, token := range []string{"gr" + "oup", "fol" + "der"} {
		if strings.Contains(statements, token) {
			t.Fatalf("schema statements contain non-team vocabulary token %q", token)
		}
	}
}

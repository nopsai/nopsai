package nopsai

import (
	"strings"
	"testing"
)

func TestAccessGrantBootstrapUsesTeamVocabulary(t *testing.T) {
	joined := strings.Join(accessGrantSchemaStatements, "\n")
	required := []string{
		"ALTER TABLE access_grants ADD COLUMN IF NOT EXISTS external_team_name TEXT NOT NULL DEFAULT ''",
		"DROP INDEX IF EXISTS idx_access_grants_identity_provider",
		"ALTER TABLE access_grants ADD CONSTRAINT access_grants_subject_type_check",
		"ALTER TABLE resource_ownership ADD CONSTRAINT resource_ownership_owner_subject_type_check",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("access grant bootstrap missing team statement %q", statement)
		}
	}

	dropIndex := strings.Index(joined, "DROP CONSTRAINT IF EXISTS access_grants_subject_type_check")
	checkIndex := strings.Index(joined, "ADD CONSTRAINT access_grants_subject_type_check")
	if dropIndex == -1 || checkIndex == -1 || dropIndex > checkIndex {
		t.Fatalf("access grant subject check order is wrong; drop index %d check index %d", dropIndex, checkIndex)
	}
	assertTeamOnlySchemaVocabulary(t, joined)
}

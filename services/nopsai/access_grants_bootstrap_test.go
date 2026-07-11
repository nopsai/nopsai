package nopsai

import (
	"strings"
	"testing"
)

func TestAccessGrantBootstrapMigratesLegacyGroupValuesBeforeChecks(t *testing.T) {
	joined := strings.Join(accessGrantSchemaStatements, "\n")
	required := []string{
		"UPDATE access_grants SET subject_type = 'auth_team' WHERE subject_type = 'auth_group'",
		"UPDATE access_grants SET subject_type = 'team' WHERE subject_type = 'group'",
		"UPDATE access_grants SET resource_type = 'team' WHERE resource_type IN ('group', 'folder')",
		"SET external_team_name = external_group_name",
		"ALTER TABLE access_grants DROP COLUMN external_group_name",
		"DROP INDEX IF EXISTS idx_access_grants_identity_provider",
		"UPDATE resource_ownership SET owner_subject_type = 'auth_team' WHERE owner_subject_type = 'auth_group'",
		"UPDATE resource_ownership SET resource_type = 'team' WHERE resource_type IN ('group', 'folder')",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("access grant bootstrap missing legacy team migration %q", statement)
		}
	}

	migrationIndex := strings.Index(joined, "UPDATE access_grants SET subject_type = 'team' WHERE subject_type = 'group'")
	checkIndex := strings.Index(joined, "ADD CONSTRAINT access_grants_subject_type_check")
	if migrationIndex == -1 || checkIndex == -1 || migrationIndex > checkIndex {
		t.Fatalf("access grant subject migration must run before the subject type check; migration index %d check index %d", migrationIndex, checkIndex)
	}
}

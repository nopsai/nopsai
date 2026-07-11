package store

import (
	"strings"
	"testing"
)

func TestAAASchemaMigratesLegacyGroupDataBeforeTeamChecks(t *testing.T) {
	joined := strings.Join(aaaSchemaStatements, "\n")
	required := []string{
		"ALTER TABLE auth_groups RENAME TO auth_teams",
		"ALTER TABLE auth_group_members RENAME TO auth_team_members",
		"RENAME COLUMN group_id TO team_id",
		"RENAME COLUMN external_group_name TO external_team_name",
		"RENAME COLUMN auth_group_name TO auth_team_name",
		"RENAME CONSTRAINT auth_groups_pkey TO auth_teams_pkey",
		"RENAME CONSTRAINT auth_groups_name_key TO auth_teams_name_key",
		"RENAME CONSTRAINT auth_group_members_pkey TO auth_team_members_pkey",
		"RENAME CONSTRAINT auth_group_members_group_id_fkey TO auth_team_members_team_id_fkey",
		"UPDATE auth_role_bindings SET subject_type = 'auth_team' WHERE subject_type = 'auth_group'",
		"UPDATE access_grants SET subject_type = 'auth_team' WHERE subject_type = 'auth_group'",
		"UPDATE access_grants SET subject_type = 'team' WHERE subject_type = 'group'",
		"UPDATE access_grants SET resource_type = 'team' WHERE resource_type IN ('group', 'folder')",
		"UPDATE resource_acl SET resource_type = 'team' WHERE resource_type IN ('group', 'folder')",
		"UPDATE resource_ownership SET resource_type = 'team' WHERE resource_type IN ('group', 'folder')",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("schema statements missing legacy team migration %q", statement)
		}
	}

	migrationIndex := strings.Index(joined, "UPDATE access_grants SET subject_type = 'team' WHERE subject_type = 'group'")
	dropIndex := strings.Index(joined, "ALTER TABLE access_grants DROP CONSTRAINT IF EXISTS access_grants_subject_type_check")
	checkIndex := strings.Index(joined, "ADD CONSTRAINT access_grants_subject_type_check")
	if dropIndex == -1 || migrationIndex == -1 || checkIndex == -1 || dropIndex > migrationIndex || migrationIndex > checkIndex {
		t.Fatalf("access grant migration order is wrong; drop index %d migration index %d check index %d", dropIndex, migrationIndex, checkIndex)
	}
}

package nopsai

import (
	"strings"
	"testing"
)

func TestAuthSchemaMigratesLegacyGroupOIDCColumns(t *testing.T) {
	joined := strings.Join(authSchemaStatements, "\n")
	required := []string{
		"ALTER TABLE auth_identity_providers ADD COLUMN IF NOT EXISTS team_claim TEXT NOT NULL DEFAULT ''",
		"SET team_claim = group_claim",
		"ALTER TABLE auth_identity_providers DROP COLUMN group_claim",
		"SET team_mapping = group_mapping",
		"ALTER TABLE auth_identity_providers DROP COLUMN group_mapping",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("auth schema missing legacy OIDC migration %q", statement)
		}
	}

	addTeamClaim := strings.Index(joined, "ADD COLUMN IF NOT EXISTS team_claim")
	copyGroupClaim := strings.Index(joined, "SET team_claim = group_claim")
	dropGroupClaim := strings.Index(joined, "DROP COLUMN group_claim")
	if addTeamClaim == -1 || copyGroupClaim == -1 || dropGroupClaim == -1 ||
		addTeamClaim > copyGroupClaim || copyGroupClaim > dropGroupClaim {
		t.Fatalf("OIDC team claim migration order is wrong; add %d copy %d drop %d", addTeamClaim, copyGroupClaim, dropGroupClaim)
	}
}

func TestAuthSchemaMigratesLegacyExternalTeamMemberships(t *testing.T) {
	joined := strings.Join(authSchemaStatements, "\n")
	required := []string{
		"ALTER TABLE auth_external_group_memberships RENAME TO auth_external_team_memberships",
		"ALTER TABLE auth_external_team_memberships RENAME COLUMN group_name TO team_name",
		"RENAME CONSTRAINT auth_external_group_memberships_pkey TO auth_external_team_memberships_pkey",
		"INSERT INTO auth_external_team_memberships (user_id, provider_id, team_name, last_seen_at)",
		"SELECT user_id, provider_id, group_name, last_seen_at",
		"DROP TABLE auth_external_group_memberships",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("auth schema missing legacy external team membership migration %q", statement)
		}
	}

	renameTable := strings.Index(joined, "ALTER TABLE auth_external_group_memberships RENAME TO auth_external_team_memberships")
	createTable := strings.Index(joined, "CREATE TABLE IF NOT EXISTS auth_external_team_memberships")
	copyOldRows := strings.Index(joined, "SELECT user_id, provider_id, group_name, last_seen_at")
	dropOldTable := strings.Index(joined, "DROP TABLE auth_external_group_memberships")
	if renameTable == -1 || createTable == -1 || copyOldRows == -1 || dropOldTable == -1 ||
		renameTable > createTable || createTable > copyOldRows || copyOldRows > dropOldTable {
		t.Fatalf("external team membership migration order is wrong; rename %d create %d copy %d drop %d", renameTable, createTable, copyOldRows, dropOldTable)
	}
}

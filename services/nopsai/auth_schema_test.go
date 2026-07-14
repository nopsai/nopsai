package nopsai

import (
	"strings"
	"testing"
)

func TestAuthSchemaUsesTeamOIDCColumns(t *testing.T) {
	joined := strings.Join(authSchemaStatements, "\n")
	required := []string{
		"ALTER TABLE auth_identity_providers ADD COLUMN IF NOT EXISTS team_claim TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE auth_identity_providers ADD COLUMN IF NOT EXISTS team_mapping JSONB NOT NULL DEFAULT '{}'::jsonb",
		"CREATE TABLE IF NOT EXISTS auth_external_team_memberships",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("auth schema missing OIDC team statement %q", statement)
		}
	}

	addTeamClaim := strings.Index(joined, "ADD COLUMN IF NOT EXISTS team_claim")
	createMemberships := strings.Index(joined, "CREATE TABLE IF NOT EXISTS auth_external_team_memberships")
	if addTeamClaim == -1 || createMemberships == -1 || addTeamClaim > createMemberships {
		t.Fatalf("OIDC schema order is wrong; add team claim %d create memberships %d", addTeamClaim, createMemberships)
	}
	assertTeamOnlySchemaVocabulary(t, joined)
}

func TestAuthSchemaDefinesExternalTeamMemberships(t *testing.T) {
	joined := strings.Join(authSchemaStatements, "\n")
	required := []string{
		"CREATE TABLE IF NOT EXISTS auth_external_team_memberships",
		"team_name TEXT NOT NULL",
		"PRIMARY KEY(user_id, provider_id, team_name)",
		"CREATE TABLE IF NOT EXISTS auth_external_role_assignments",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("auth schema missing external team membership statement %q", statement)
		}
	}
	assertTeamOnlySchemaVocabulary(t, joined)
}

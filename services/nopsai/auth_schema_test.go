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

func TestAuthSchemaTracksOIDCEmailVerificationStatus(t *testing.T) {
	joined := strings.Join(authSchemaStatements, "\n")
	required := []string{
		"email_verification_status TEXT NOT NULL DEFAULT 'unknown'",
		"ADD COLUMN IF NOT EXISTS email_verification_status",
		"auth_external_identities_email_verification_status_check",
		"'not_provided', 'unknown', 'unverified', 'verified'",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("auth schema missing OIDC email verification statement %q", statement)
		}
	}
}

func TestAuthSchemaAllowsDuplicateOrNullUserEmails(t *testing.T) {
	joined := strings.Join(authSchemaStatements, "\n")
	required := []string{
		"DROP CONSTRAINT IF EXISTS users_email_key",
		"DROP CONSTRAINT IF EXISTS users_email_unique",
		"DROP INDEX IF EXISTS idx_users_email_unique",
		"CREATE INDEX IF NOT EXISTS idx_users_email_lower_lookup",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("auth schema missing duplicate-email guardrail %q", statement)
		}
	}
	if strings.Contains(joined, "UNIQUE(email)") || strings.Contains(joined, "UNIQUE (email)") {
		t.Fatal("auth schema must not enforce unique user email addresses")
	}
}

func TestAuthSchemaDefinesPersistentLoginAttemptTracking(t *testing.T) {
	joined := strings.Join(authSchemaStatements, "\n")
	required := []string{
		"CREATE TABLE IF NOT EXISTS auth_login_attempts",
		"key_hash TEXT PRIMARY KEY",
		"attempt_window_start TIMESTAMPTZ",
		"failure_window_start TIMESTAMPTZ",
		"locked_until TIMESTAMPTZ",
		"idx_auth_login_attempts_locked",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("auth schema missing persistent login tracking statement %q", statement)
		}
	}
}

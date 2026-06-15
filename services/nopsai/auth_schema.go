package nopsai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var authSchemaStatements = []string{
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS must_change_password BOOLEAN NOT NULL DEFAULT FALSE`,
	`UPDATE users
	 SET must_change_password = TRUE
	 WHERE provider = 'local'
	   AND password_hash IS NOT NULL
	   AND last_login IS NULL
	   AND must_change_password = FALSE`,
	`CREATE TABLE IF NOT EXISTS personal_access_tokens (
		id UUID PRIMARY KEY,
		user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		token_hash TEXT UNIQUE NOT NULL,
		token_suffix TEXT NOT NULL DEFAULT '',
		expires_at TIMESTAMPTZ,
		last_used_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		revoked_at TIMESTAMPTZ
	)`,
	`ALTER TABLE personal_access_tokens ALTER COLUMN expires_at DROP NOT NULL`,
	`CREATE INDEX IF NOT EXISTS idx_personal_access_tokens_user ON personal_access_tokens(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_personal_access_tokens_expiry ON personal_access_tokens(expires_at)`,
	`CREATE TABLE IF NOT EXISTS service_account_tokens (
		id UUID PRIMARY KEY,
		service_account_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		token_hash TEXT UNIQUE NOT NULL,
		token_suffix TEXT NOT NULL DEFAULT '',
		expires_at TIMESTAMPTZ,
		last_used_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		revoked_at TIMESTAMPTZ
	)`,
	`CREATE INDEX IF NOT EXISTS idx_service_account_tokens_account ON service_account_tokens(service_account_id)`,
	`CREATE INDEX IF NOT EXISTS idx_service_account_tokens_expiry ON service_account_tokens(expires_at)`,
	`CREATE TABLE IF NOT EXISTS auth_settings (
		key TEXT PRIMARY KEY,
		value JSONB NOT NULL DEFAULT '{}'::jsonb,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE IF NOT EXISTS auth_identity_providers (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL DEFAULT 'oidc',
		display_name TEXT NOT NULL,
		issuer TEXT NOT NULL,
		authorization_endpoint TEXT NOT NULL DEFAULT '',
		token_endpoint TEXT NOT NULL DEFAULT '',
		jwks_uri TEXT NOT NULL DEFAULT '',
		userinfo_endpoint TEXT NOT NULL DEFAULT '',
		client_id TEXT NOT NULL DEFAULT '',
		client_credential_ref TEXT NOT NULL DEFAULT '',
		scopes JSONB NOT NULL DEFAULT '["openid","email","profile"]'::jsonb,
		allowed_email_domains JSONB NOT NULL DEFAULT '[]'::jsonb,
		group_claim TEXT NOT NULL DEFAULT '',
		role_mapping JSONB NOT NULL DEFAULT '{}'::jsonb,
		group_mapping JSONB NOT NULL DEFAULT '{}'::jsonb,
		basic_role_mapping JSONB NOT NULL DEFAULT '{}'::jsonb,
		entitlement_sync JSONB NOT NULL DEFAULT '{}'::jsonb,
		auto_create_users BOOLEAN,
		default_role TEXT NOT NULL DEFAULT '',
		allow_email_linking BOOLEAN,
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		config_source TEXT NOT NULL DEFAULT 'database',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`ALTER TABLE auth_identity_providers ADD COLUMN IF NOT EXISTS client_credential_ref TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE auth_identity_providers ADD COLUMN IF NOT EXISTS group_mapping JSONB NOT NULL DEFAULT '{}'::jsonb`,
	`ALTER TABLE auth_identity_providers ADD COLUMN IF NOT EXISTS basic_role_mapping JSONB NOT NULL DEFAULT '{}'::jsonb`,
	`ALTER TABLE auth_identity_providers ADD COLUMN IF NOT EXISTS entitlement_sync JSONB NOT NULL DEFAULT '{}'::jsonb`,
	`CREATE TABLE IF NOT EXISTS auth_oidc_domain_mappings (
		domain TEXT PRIMARY KEY,
		provider_id TEXT NOT NULL REFERENCES auth_identity_providers(id) ON DELETE CASCADE,
		config_source TEXT NOT NULL DEFAULT 'database',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE IF NOT EXISTS auth_external_identities (
		id UUID PRIMARY KEY,
		user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		provider_id TEXT NOT NULL REFERENCES auth_identity_providers(id) ON DELETE CASCADE,
		issuer TEXT NOT NULL,
		subject TEXT NOT NULL,
		email TEXT NOT NULL DEFAULT '',
		email_verified BOOLEAN NOT NULL DEFAULT FALSE,
		linked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		last_login_at TIMESTAMPTZ,
		UNIQUE(provider_id, issuer, subject)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_auth_external_identities_user ON auth_external_identities(user_id)`,
	`CREATE TABLE IF NOT EXISTS auth_oidc_states (
		state_hash TEXT PRIMARY KEY,
		provider_id TEXT NOT NULL REFERENCES auth_identity_providers(id) ON DELETE CASCADE,
		nonce_hash TEXT NOT NULL,
		code_verifier TEXT NOT NULL,
		return_to TEXT NOT NULL DEFAULT '',
		expires_at TIMESTAMPTZ NOT NULL,
		consumed_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_auth_oidc_states_expiry ON auth_oidc_states(expires_at)`,
	`CREATE TABLE IF NOT EXISTS auth_login_codes (
		code_hash TEXT PRIMARY KEY,
		user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		provider_id TEXT NOT NULL REFERENCES auth_identity_providers(id) ON DELETE CASCADE,
		return_to TEXT NOT NULL DEFAULT '',
		expires_at TIMESTAMPTZ NOT NULL,
		consumed_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_auth_login_codes_expiry ON auth_login_codes(expires_at)`,
	`CREATE TABLE IF NOT EXISTS auth_external_group_memberships (
		user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		provider_id TEXT NOT NULL REFERENCES auth_identity_providers(id) ON DELETE CASCADE,
		group_name TEXT NOT NULL,
		last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY(user_id, provider_id, group_name)
	)`,
	`CREATE TABLE IF NOT EXISTS auth_external_role_assignments (
		user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		provider_id TEXT NOT NULL REFERENCES auth_identity_providers(id) ON DELETE CASCADE,
		role_name TEXT NOT NULL,
		last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY(user_id, provider_id, role_name)
	)`,
}

func ensureAuthSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin auth schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range authSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply auth schema statement %d: %w", idx+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit auth schema transaction: %w", err)
	}
	return nil
}

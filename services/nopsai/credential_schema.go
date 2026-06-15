package nopsai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var credentialSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS credentials (
		id UUID PRIMARY KEY,
		namespace TEXT NOT NULL DEFAULT 'system',
		name TEXT NOT NULL,
		kind TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'disabled')),
		active_version INTEGER NOT NULL DEFAULT 0 CHECK (active_version >= 0),
		next_version INTEGER NOT NULL DEFAULT 1 CHECK (next_version > 0),
		expires_at TIMESTAMPTZ,
		last_rotated_at TIMESTAMPTZ,
		managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
		config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
		created_by TEXT NOT NULL DEFAULT '',
		updated_by TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(namespace, name)
	)`,
	`CREATE TABLE IF NOT EXISTS credential_versions (
		credential_id UUID NOT NULL REFERENCES credentials(id) ON DELETE CASCADE,
		version INTEGER NOT NULL CHECK (version > 0),
		ciphertext BYTEA NOT NULL,
		wrapped_data_key BYTEA NOT NULL,
		encryption_key_id TEXT NOT NULL,
		encryption_format_version INTEGER NOT NULL,
		created_by TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		activated_at TIMESTAMPTZ,
		revoked_at TIMESTAMPTZ,
		PRIMARY KEY (credential_id, version)
	)`,
	`CREATE TABLE IF NOT EXISTS credential_access_logs (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		credential_id UUID NOT NULL REFERENCES credentials(id) ON DELETE CASCADE,
		version INTEGER NOT NULL,
		consumer_service TEXT NOT NULL,
		purpose TEXT NOT NULL,
		subject_type TEXT NOT NULL DEFAULT '',
		subject_id TEXT NOT NULL DEFAULT '',
		correlation_id TEXT NOT NULL DEFAULT '',
		success BOOLEAN NOT NULL,
		error_code TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_credentials_status ON credentials(status)`,
	`ALTER TABLE credentials ADD COLUMN IF NOT EXISTS next_version INTEGER NOT NULL DEFAULT 1`,
	`CREATE INDEX IF NOT EXISTS idx_credentials_config_repo ON credentials(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_credential_access_logs_credential_created
		ON credential_access_logs(credential_id, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_credential_access_logs_consumer_created
		ON credential_access_logs(consumer_service, created_at DESC)`,
}

func ensureCredentialSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin credential schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	for idx, statement := range credentialSchemaStatements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("apply credential schema statement %d: %w", idx+1, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit credential schema transaction: %w", err)
	}
	return nil
}

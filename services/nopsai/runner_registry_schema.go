package nopsai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var runnerRegistryCredentialSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS runner_registry_credentials (
		runner_id TEXT NOT NULL,
		credential_ref TEXT NOT NULL,
		registry_hosts JSONB NOT NULL DEFAULT '[]'::jsonb,
		source TEXT NOT NULL DEFAULT 'database',
		managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
		config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
		created_by TEXT NOT NULL DEFAULT '',
		updated_by TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (runner_id, credential_ref)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_runner_registry_credentials_runner
		ON runner_registry_credentials(runner_id)`,
	`CREATE INDEX IF NOT EXISTS idx_runner_registry_credentials_config_repo
		ON runner_registry_credentials(config_repo_id)`,
}

func ensureRunnerRegistryCredentialSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin runner registry credential schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	for idx, statement := range runnerRegistryCredentialSchemaStatements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("apply runner registry credential schema statement %d: %w", idx+1, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit runner registry credential schema transaction: %w", err)
	}
	return nil
}

package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var configRepositorySchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS config_repositories (
		id BIGSERIAL PRIMARY KEY,
		scope_type TEXT NOT NULL CHECK (scope_type IN ('folder', 'system')),
		scope_id TEXT NOT NULL,
		repo_url TEXT NOT NULL,
		branch TEXT NOT NULL DEFAULT 'main',
		base_path TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		last_sync_status TEXT NOT NULL DEFAULT '',
		last_sync_message TEXT NOT NULL DEFAULT '',
		last_sync_started_at TIMESTAMPTZ,
		last_sync_completed_at TIMESTAMPTZ,
		last_sync_commit_sha TEXT NOT NULL DEFAULT '',
		created_by TEXT NOT NULL DEFAULT '',
		updated_by TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(scope_type, scope_id),
		UNIQUE(repo_url, branch, base_path)
	)`,
	`ALTER TABLE pipelines ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE pipelines ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipelines ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipelines ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE steps ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE steps ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE steps ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE steps ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE triggers ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE triggers ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE triggers ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE triggers ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE variables ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE variables ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE variables ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE variables ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE secrets ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE secrets ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE secrets ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE secrets ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`CREATE INDEX IF NOT EXISTS idx_config_repositories_scope ON config_repositories(scope_type, scope_id)`,
	`CREATE INDEX IF NOT EXISTS idx_pipelines_config_repo_id ON pipelines(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_steps_config_repo_id ON steps(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_triggers_config_repo_id ON triggers(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_variables_config_repo_id ON variables(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_secrets_config_repo_id ON secrets(config_repo_id)`,
}

func ensureConfigRepositorySchema(ctx context.Context, db *pgxpool.Pool) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin config repository schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range configRepositorySchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply config repository schema statement %d: %w", idx+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit config repository schema transaction: %w", err)
	}
	return nil
}

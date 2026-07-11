package nopsai

import (
	"context"
	"fmt"

	aaastore "nopsai/services/aaa/pkg/store"

	"github.com/jackc/pgx/v5/pgxpool"
)

var configRepositorySchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS config_repositories (
		id BIGSERIAL PRIMARY KEY,
		scope_type TEXT NOT NULL CHECK (scope_type IN ('team', 'system')),
		scope_id TEXT NOT NULL,
		repo_url TEXT NOT NULL,
		branch TEXT NOT NULL DEFAULT 'main',
		base_path TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		write_enabled BOOLEAN NOT NULL DEFAULT FALSE,
		write_branch TEXT NOT NULL DEFAULT '',
		config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
		managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
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
	`ALTER TABLE config_repositories ADD COLUMN IF NOT EXISTS write_enabled BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE config_repositories ADD COLUMN IF NOT EXISTS write_branch TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE config_repositories ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE config_repositories ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE config_repositories ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE config_repositories ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
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
	`ALTER TABLE secrets ADD COLUMN IF NOT EXISTS source VARCHAR(32) NOT NULL DEFAULT 'database'`,
	`ALTER TABLE secrets ALTER COLUMN value DROP NOT NULL`,
	`ALTER TABLE secrets ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE secrets ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE secrets ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE secrets ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`WITH default_scope_rows AS (
		SELECT id,
		       ROW_NUMBER() OVER (
		           PARTITION BY name, repository_name
		           ORDER BY CASE
		               WHEN scope = 'default' THEN 0
		               WHEN BTRIM(scope) = '' THEN 1
		               ELSE 2
		           END, id
		       ) AS rn
		FROM variables
		WHERE scope IS NULL OR BTRIM(scope) = '' OR scope = 'default'
	)
	DELETE FROM variables
	USING default_scope_rows
	WHERE variables.id = default_scope_rows.id
	AND default_scope_rows.rn > 1`,
	`UPDATE variables SET scope = 'default' WHERE scope IS NULL OR BTRIM(scope) = ''`,
	`ALTER TABLE variables ALTER COLUMN scope SET DEFAULT 'default'`,
	`ALTER TABLE variables ALTER COLUMN scope SET NOT NULL`,
	`DO $$
	BEGIN
		IF NOT EXISTS (
			SELECT 1 FROM pg_constraint WHERE conname = 'variables_scope_not_empty'
		) THEN
			ALTER TABLE variables ADD CONSTRAINT variables_scope_not_empty CHECK (BTRIM(scope) <> '');
		END IF;
	END $$`,
	`WITH default_scope_rows AS (
		SELECT id,
		       ROW_NUMBER() OVER (
		           PARTITION BY name, repository_name
		           ORDER BY CASE
		               WHEN scope = 'default' THEN 0
		               WHEN BTRIM(scope) = '' THEN 1
		               ELSE 2
		           END, id
		       ) AS rn
		FROM secrets
		WHERE scope IS NULL OR BTRIM(scope) = '' OR scope = 'default'
	)
	DELETE FROM secrets
	USING default_scope_rows
	WHERE secrets.id = default_scope_rows.id
	AND default_scope_rows.rn > 1`,
	`UPDATE secrets SET scope = 'default' WHERE scope IS NULL OR BTRIM(scope) = ''`,
	`ALTER TABLE secrets ALTER COLUMN scope SET DEFAULT 'default'`,
	`ALTER TABLE secrets ALTER COLUMN scope SET NOT NULL`,
	`DO $$
	BEGIN
		IF NOT EXISTS (
			SELECT 1 FROM pg_constraint WHERE conname = 'secrets_scope_not_empty'
		) THEN
			ALTER TABLE secrets ADD CONSTRAINT secrets_scope_not_empty CHECK (BTRIM(scope) <> '');
		END IF;
	END $$`,
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE user_roles ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE user_roles ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE user_roles ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE user_roles ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE role_permissions ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE role_permissions ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE role_permissions ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE role_permissions ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE access_grants ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE access_grants ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE access_grants ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE access_grants ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`CREATE INDEX IF NOT EXISTS idx_config_repositories_scope ON config_repositories(scope_type, scope_id)`,
	`CREATE INDEX IF NOT EXISTS idx_config_repositories_config_repo_id ON config_repositories(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_pipelines_config_repo_id ON pipelines(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_steps_config_repo_id ON steps(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_triggers_config_repo_id ON triggers(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_variables_config_repo_id ON variables(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_secrets_config_repo_id ON secrets(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_users_config_repo_id ON users(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_user_roles_config_repo_id ON user_roles(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_role_permissions_config_repo_id ON role_permissions(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_access_grants_config_repo_id ON access_grants(config_repo_id)`,
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
	if err := aaastore.EnsurePolicyConfigMetadataSchema(ctx, tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit config repository schema transaction: %w", err)
	}
	return nil
}

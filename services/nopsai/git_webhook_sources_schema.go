package nopsai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var gitWebhookSourceSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS git_webhook_sources (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		provider TEXT NOT NULL,
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		team_path TEXT NOT NULL DEFAULT 'global',
		visibility TEXT NOT NULL DEFAULT 'team',
		auth_mode TEXT NOT NULL,
		credential_ref TEXT NOT NULL DEFAULT '',
		repository_allowlist JSONB NOT NULL DEFAULT '[]'::jsonb,
		rate_limit JSONB NOT NULL DEFAULT '{}'::jsonb,
		created_by TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		last_used_at TIMESTAMPTZ,
		source TEXT NOT NULL DEFAULT 'database',
		config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
		managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE
	)`,
	`ALTER TABLE git_webhook_sources ADD COLUMN IF NOT EXISTS team_path TEXT NOT NULL DEFAULT 'global'`,
	`ALTER TABLE git_webhook_sources ALTER COLUMN team_path SET DEFAULT 'global'`,
	`UPDATE git_webhook_sources
		SET team_path = 'global'
		WHERE BTRIM(team_path) = '' OR LOWER(BTRIM(team_path)) IN ('root', 'general', '__general__')`,
	`ALTER TABLE git_webhook_sources ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'team'`,
	`ALTER TABLE git_webhook_sources DROP CONSTRAINT IF EXISTS git_webhook_sources_visibility_check`,
	`UPDATE git_webhook_sources SET visibility = 'workspace' WHERE visibility IN ('workspace_shared', 'shared')`,
	`UPDATE git_webhook_sources SET visibility = 'team' WHERE BTRIM(visibility) = ''`,
	`ALTER TABLE git_webhook_sources ADD CONSTRAINT git_webhook_sources_visibility_check CHECK (visibility IN ('team', 'workspace'))`,
	`CREATE INDEX IF NOT EXISTS idx_git_webhook_sources_enabled ON git_webhook_sources(enabled)`,
	`CREATE INDEX IF NOT EXISTS idx_git_webhook_sources_provider ON git_webhook_sources(provider)`,
	`CREATE INDEX IF NOT EXISTS idx_git_webhook_sources_team ON git_webhook_sources(team_path, id)`,
	`CREATE INDEX IF NOT EXISTS idx_git_webhook_sources_visibility ON git_webhook_sources(visibility)`,
	`CREATE INDEX IF NOT EXISTS idx_git_webhook_sources_config_repo ON git_webhook_sources(config_repo_id)`,
	`CREATE TABLE IF NOT EXISTS git_webhook_deliveries (
		id UUID PRIMARY KEY,
		source_id TEXT NOT NULL REFERENCES git_webhook_sources(id) ON DELETE CASCADE,
		delivery_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		event_type TEXT NOT NULL DEFAULT '',
		repository_full_name TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		run_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
		error TEXT NOT NULL DEFAULT '',
		source_ip TEXT NOT NULL DEFAULT '',
		received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		completed_at TIMESTAMPTZ,
		UNIQUE(source_id, delivery_id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_git_webhook_deliveries_source_received
		ON git_webhook_deliveries(source_id, received_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_git_webhook_deliveries_repository
		ON git_webhook_deliveries(repository_full_name, received_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_git_webhook_deliveries_status
		ON git_webhook_deliveries(status, received_at DESC)`,
}

func ensureGitWebhookSourceSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin git webhook source schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for index, statement := range gitWebhookSourceSchemaStatements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("apply git webhook source schema statement %d: %w", index+1, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit git webhook source schema transaction: %w", err)
	}
	return nil
}

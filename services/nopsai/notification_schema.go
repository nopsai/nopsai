package nopsai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var notificationSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS notification_mail_settings (
		id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
		enabled BOOLEAN NOT NULL DEFAULT FALSE,
		from_address TEXT NOT NULL DEFAULT '',
		smtp_host TEXT NOT NULL DEFAULT '',
		smtp_port INTEGER NOT NULL DEFAULT 587,
		smtp_start_tls BOOLEAN NOT NULL DEFAULT TRUE,
		smtp_username TEXT NOT NULL DEFAULT '',
		smtp_password_credential_ref TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'database',
		config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
		managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`ALTER TABLE notification_mail_settings ADD COLUMN IF NOT EXISTS smtp_password_credential_ref TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE notification_mail_settings ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'database'`,
	`ALTER TABLE notification_mail_settings ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE notification_mail_settings ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE notification_mail_settings ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE notification_mail_settings ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`CREATE TABLE IF NOT EXISTS notification_routes (
		id BIGSERIAL PRIMARY KEY,
		group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
		definition JSONB NOT NULL DEFAULT '{}'::jsonb,
		source TEXT NOT NULL DEFAULT 'database',
		config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
		managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
		updated_by TEXT NOT NULL DEFAULT '',
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(group_id)
	)`,
	`CREATE TABLE IF NOT EXISTS notification_deliveries (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		run_id UUID REFERENCES pipeline_runs(run_id) ON DELETE SET NULL,
		event_type TEXT NOT NULL,
		channel TEXT NOT NULL,
		recipient TEXT NOT NULL,
		status TEXT NOT NULL,
		error TEXT NOT NULL DEFAULT '',
		dedupe_key TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		sent_at TIMESTAMPTZ,
		UNIQUE(dedupe_key)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_notification_routes_group ON notification_routes(group_id)`,
	`CREATE INDEX IF NOT EXISTS idx_notification_routes_config_repo ON notification_routes(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_notification_deliveries_run ON notification_deliveries(run_id)`,
	`CREATE INDEX IF NOT EXISTS idx_notification_deliveries_status ON notification_deliveries(status, created_at DESC)`,
}

func ensureNotificationSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin notification schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range notificationSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply notification schema statement %d: %w", idx+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit notification schema transaction: %w", err)
	}
	return nil
}

package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var dataManagementSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS data_backups (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		backup_type TEXT NOT NULL CHECK (backup_type IN ('full', 'runs', 'logs')),
		status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'success', 'failure')),
		file_path TEXT NOT NULL DEFAULT '',
		file_name TEXT NOT NULL DEFAULT '',
		content_type TEXT NOT NULL DEFAULT 'application/gzip',
		size_bytes BIGINT NOT NULL DEFAULT 0,
		checksum_sha256 TEXT NOT NULL DEFAULT '',
		requested_by TEXT NOT NULL DEFAULT '',
		error TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		completed_at TIMESTAMPTZ
	)`,
	`CREATE TABLE IF NOT EXISTS data_cleanup_schedules (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		target TEXT NOT NULL CHECK (target IN ('runs', 'logs')),
		mode TEXT NOT NULL CHECK (mode IN ('keep_last', 'older_than_days', 'all_terminal_runs', 'all_logs')),
		keep_last INT NOT NULL DEFAULT 0,
		older_than_days INT NOT NULL DEFAULT 0,
		backup_before_cleanup BOOLEAN NOT NULL DEFAULT TRUE,
		cron_expression TEXT NOT NULL DEFAULT '0 2 * * 0',
		timezone TEXT NOT NULL DEFAULT 'UTC',
		next_run_at TIMESTAMPTZ,
		last_run_at TIMESTAMPTZ,
		last_job_id UUID,
		last_status TEXT NOT NULL DEFAULT '',
		last_deleted_counts JSONB NOT NULL DEFAULT '{}'::jsonb,
		last_error TEXT NOT NULL DEFAULT '',
		created_by TEXT NOT NULL DEFAULT '',
		updated_by TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE IF NOT EXISTS data_cleanup_jobs (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		schedule_id UUID REFERENCES data_cleanup_schedules(id) ON DELETE SET NULL,
		trigger_type TEXT NOT NULL DEFAULT 'manual' CHECK (trigger_type IN ('manual', 'scheduled')),
		status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'success', 'failure')),
		target TEXT NOT NULL CHECK (target IN ('runs', 'logs')),
		mode TEXT NOT NULL CHECK (mode IN ('keep_last', 'older_than_days', 'all_terminal_runs', 'all_logs')),
		keep_last INT NOT NULL DEFAULT 0,
		older_than_days INT NOT NULL DEFAULT 0,
		backup_before_cleanup BOOLEAN NOT NULL DEFAULT FALSE,
		backup_id UUID REFERENCES data_backups(id) ON DELETE SET NULL,
		requested_by TEXT NOT NULL DEFAULT '',
		preview_counts JSONB NOT NULL DEFAULT '{}'::jsonb,
		deleted_counts JSONB NOT NULL DEFAULT '{}'::jsonb,
		error TEXT NOT NULL DEFAULT '',
		started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		completed_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`ALTER TABLE data_cleanup_schedules ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE data_cleanup_schedules ADD COLUMN IF NOT EXISTS backup_before_cleanup BOOLEAN NOT NULL DEFAULT TRUE`,
	`ALTER TABLE data_cleanup_schedules ADD COLUMN IF NOT EXISTS last_job_id UUID`,
	`ALTER TABLE data_cleanup_schedules ADD COLUMN IF NOT EXISTS last_deleted_counts JSONB NOT NULL DEFAULT '{}'::jsonb`,
	`ALTER TABLE data_cleanup_schedules ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE data_cleanup_jobs ADD COLUMN IF NOT EXISTS preview_counts JSONB NOT NULL DEFAULT '{}'::jsonb`,
	`ALTER TABLE data_cleanup_jobs ADD COLUMN IF NOT EXISTS deleted_counts JSONB NOT NULL DEFAULT '{}'::jsonb`,
	`ALTER TABLE data_cleanup_jobs ADD COLUMN IF NOT EXISTS backup_id UUID REFERENCES data_backups(id) ON DELETE SET NULL`,
	`DO $$
	BEGIN
		IF NOT EXISTS (
			SELECT 1 FROM pg_constraint WHERE conname = 'data_cleanup_schedules_last_job_id_fkey'
		) THEN
			ALTER TABLE data_cleanup_schedules
				ADD CONSTRAINT data_cleanup_schedules_last_job_id_fkey
				FOREIGN KEY (last_job_id) REFERENCES data_cleanup_jobs(id) ON DELETE SET NULL;
		END IF;
	END $$`,
	`CREATE INDEX IF NOT EXISTS idx_data_backups_created_at ON data_backups(created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_data_backups_status ON data_backups(status, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_data_cleanup_jobs_created_at ON data_cleanup_jobs(created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_data_cleanup_jobs_schedule_id ON data_cleanup_jobs(schedule_id, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_data_cleanup_schedules_next_run ON data_cleanup_schedules(enabled, next_run_at)`,
}

func ensureDataManagementSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin data management schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range dataManagementSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply data management schema statement %d: %w", idx+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit data management schema transaction: %w", err)
	}
	return nil
}

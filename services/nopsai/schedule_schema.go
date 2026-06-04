package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var scheduleSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS pipeline_schedules (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		path TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		pipeline_path TEXT NOT NULL DEFAULT '',
		pipeline_name TEXT NOT NULL,
		pipeline_version TEXT NOT NULL DEFAULT 'latest',
		schedule_kind TEXT NOT NULL DEFAULT 'cron',
		cron_expression TEXT NOT NULL,
		run_at TIMESTAMPTZ,
		timezone TEXT NOT NULL DEFAULT 'UTC',
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		scope TEXT NOT NULL DEFAULT '',
		run_group_path TEXT NOT NULL DEFAULT '',
		variables JSONB NOT NULL DEFAULT '{}'::jsonb,
		next_run_at TIMESTAMPTZ,
		last_run_at TIMESTAMPTZ,
		last_run_id UUID REFERENCES pipeline_runs(run_id) ON DELETE SET NULL,
		last_status TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'database',
		visibility TEXT NOT NULL DEFAULT 'group' CHECK (visibility IN ('group', 'restricted', 'workspace')),
		config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
		managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
		created_by TEXT NOT NULL DEFAULT '',
		updated_by TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(path, name)
	)`,
	`ALTER TABLE pipeline_schedules ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_schedules ADD COLUMN IF NOT EXISTS pipeline_version TEXT NOT NULL DEFAULT 'latest'`,
	`ALTER TABLE pipeline_schedules ADD COLUMN IF NOT EXISTS schedule_kind TEXT NOT NULL DEFAULT 'cron'`,
	`ALTER TABLE pipeline_schedules ADD COLUMN IF NOT EXISTS run_at TIMESTAMPTZ`,
	`ALTER TABLE pipeline_schedules ADD COLUMN IF NOT EXISTS variables JSONB NOT NULL DEFAULT '{}'::jsonb`,
	`ALTER TABLE pipeline_schedules ADD COLUMN IF NOT EXISTS run_group_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_schedules ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'group'`,
	`ALTER TABLE pipeline_schedules ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE pipeline_schedules ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_schedules ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_schedules ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE pipeline_schedules ADD COLUMN IF NOT EXISTS created_by TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_schedules ADD COLUMN IF NOT EXISTS updated_by TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_schedules DROP CONSTRAINT IF EXISTS pipeline_schedules_kind_check`,
	`ALTER TABLE pipeline_schedules ADD CONSTRAINT pipeline_schedules_kind_check CHECK (schedule_kind IN ('cron', 'once'))`,
	`ALTER TABLE pipeline_schedules DROP CONSTRAINT IF EXISTS pipeline_schedules_visibility_check`,
	`ALTER TABLE pipeline_schedules ADD CONSTRAINT pipeline_schedules_visibility_check CHECK (visibility IN ('group', 'restricted', 'workspace'))`,
	`ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS schedule_id UUID REFERENCES pipeline_schedules(id) ON DELETE SET NULL`,
	`CREATE INDEX IF NOT EXISTS idx_pipeline_schedules_config_repo_id ON pipeline_schedules(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_pipeline_schedules_next_run ON pipeline_schedules(enabled, next_run_at)`,
	`CREATE INDEX IF NOT EXISTS idx_pipeline_schedules_pipeline ON pipeline_schedules(pipeline_path, pipeline_name)`,
	`CREATE INDEX IF NOT EXISTS idx_pipeline_runs_schedule_id ON pipeline_runs(schedule_id)`,
}

func ensureScheduleSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin schedule schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range scheduleSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply schedule schema statement %d: %w", idx+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit schedule schema transaction: %w", err)
	}
	return nil
}

package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var externalTriggerSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS external_triggers (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		pipeline TEXT NOT NULL,
		scope TEXT NOT NULL DEFAULT '',
		allowed_callers JSONB NOT NULL DEFAULT '[]'::jsonb,
		variable_mapping JSONB NOT NULL DEFAULT '{}'::jsonb,
		payload_schema JSONB NOT NULL DEFAULT '{}'::jsonb,
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
	`ALTER TABLE external_triggers ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'database'`,
	`ALTER TABLE external_triggers ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE external_triggers ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE external_triggers ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE external_triggers ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`CREATE INDEX IF NOT EXISTS idx_external_triggers_pipeline ON external_triggers(pipeline)`,
	`CREATE INDEX IF NOT EXISTS idx_external_triggers_enabled ON external_triggers(enabled)`,
	`CREATE INDEX IF NOT EXISTS idx_external_triggers_last_used_at ON external_triggers(last_used_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_external_triggers_config_repo ON external_triggers(config_repo_id)`,
	`CREATE TABLE IF NOT EXISTS external_trigger_invocations (
		id UUID PRIMARY KEY,
		trigger_id TEXT NOT NULL REFERENCES external_triggers(id) ON DELETE CASCADE,
		caller_type TEXT NOT NULL,
		caller_id TEXT NOT NULL,
		status TEXT NOT NULL,
		run_id UUID REFERENCES pipeline_runs(run_id) ON DELETE SET NULL,
		idempotency_key TEXT NOT NULL DEFAULT '',
		event_type TEXT NOT NULL DEFAULT '',
		source_ip TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		error TEXT NOT NULL DEFAULT ''
	)`,
	`ALTER TABLE external_trigger_invocations ADD COLUMN IF NOT EXISTS event_type TEXT NOT NULL DEFAULT ''`,
	`DROP INDEX IF EXISTS idx_external_trigger_invocations_idempotency`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_external_trigger_invocations_active_idempotency
		ON external_trigger_invocations(trigger_id, caller_type, caller_id, idempotency_key)
		WHERE idempotency_key <> '' AND status IN ('pending', 'queued')`,
	`CREATE INDEX IF NOT EXISTS idx_external_trigger_invocations_trigger_created
		ON external_trigger_invocations(trigger_id, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_external_trigger_invocations_run
		ON external_trigger_invocations(run_id)`,
}

func ensureExternalTriggerSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin external trigger schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range externalTriggerSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply external trigger schema statement %d: %w", idx+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit external trigger schema transaction: %w", err)
	}
	return nil
}

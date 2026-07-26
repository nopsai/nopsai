package nopsai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var runDispatchSchemaStatements = []string{
	`ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS parent_runner_id TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS parent_history TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS runtime_variable_overrides JSONB NOT NULL DEFAULT '{}'::jsonb`,
	`ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS runtime_sensitive_variable_overrides JSONB NOT NULL DEFAULT '{}'::jsonb`,
	`ALTER TABLE pipeline_run_logs ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_run_logs ADD COLUMN IF NOT EXISTS stream TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_run_logs ADD COLUMN IF NOT EXISTS level TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_run_logs ADD COLUMN IF NOT EXISTS step_name TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_run_logs ADD COLUMN IF NOT EXISTS task_name TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_run_logs ADD COLUMN IF NOT EXISTS runner_id TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_run_logs ADD COLUMN IF NOT EXISTS request_id TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_run_logs ADD COLUMN IF NOT EXISTS traceparent TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_run_logs ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb`,
	`CREATE TABLE IF NOT EXISTS pipeline_run_task_outputs (
		run_id UUID NOT NULL REFERENCES pipeline_runs(run_id) ON DELETE CASCADE,
		step_name TEXT NOT NULL,
		task_name TEXT NOT NULL,
		name TEXT NOT NULL,
		value TEXT NOT NULL DEFAULT '',
		sensitive BOOLEAN NOT NULL DEFAULT FALSE,
		size_bytes BIGINT NOT NULL DEFAULT 0,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (run_id, step_name, task_name, name)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_pipeline_runs_pending_recovery ON pipeline_runs(created_at) WHERE status = 'pending'`,
	`CREATE INDEX IF NOT EXISTS idx_pipeline_run_logs_run_id_id ON pipeline_run_logs(run_id, id)`,
	`CREATE INDEX IF NOT EXISTS idx_pipeline_run_task_outputs_run_id ON pipeline_run_task_outputs(run_id)`,
	`CREATE INDEX IF NOT EXISTS idx_pipeline_run_logs_request_id ON pipeline_run_logs(request_id) WHERE request_id <> ''`,
	`CREATE INDEX IF NOT EXISTS idx_pipeline_run_logs_source ON pipeline_run_logs(source) WHERE source <> ''`,
	`CREATE INDEX IF NOT EXISTS idx_pipeline_run_logs_level ON pipeline_run_logs(level) WHERE level <> ''`,
}

func ensureRunDispatchSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin run dispatch schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range runDispatchSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply run dispatch schema statement %d: %w", idx+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit run dispatch schema transaction: %w", err)
	}
	return nil
}

package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var approvalSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS pipeline_run_checkpoints (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		run_id UUID NOT NULL REFERENCES pipeline_runs(run_id) ON DELETE CASCADE,
		step_name TEXT NOT NULL,
		execution_history TEXT NOT NULL DEFAULT '',
		pipeline_definition TEXT NOT NULL DEFAULT '',
		variables JSONB NOT NULL DEFAULT '{}'::jsonb,
		workspace_archive BYTEA,
		workspace_archive_format TEXT NOT NULL DEFAULT 'tar.gz',
		shared_volume_name TEXT NOT NULL DEFAULT '',
		runner_id TEXT NOT NULL DEFAULT '',
		completed_tasks JSONB NOT NULL DEFAULT '[]'::jsonb,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_pipeline_run_checkpoints_run ON pipeline_run_checkpoints(run_id, created_at DESC)`,
	`CREATE TABLE IF NOT EXISTS pipeline_approvals (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		run_id UUID NOT NULL REFERENCES pipeline_runs(run_id) ON DELETE CASCADE,
		step_name TEXT NOT NULL,
		task_name TEXT NOT NULL,
		approval_type TEXT NOT NULL,
		assigned_groups JSONB NOT NULL DEFAULT '[]'::jsonb,
		allow_self_approval BOOLEAN NOT NULL DEFAULT FALSE,
		status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
		requested_by_type TEXT NOT NULL DEFAULT '',
		requested_by_id TEXT NOT NULL DEFAULT '',
		requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		decided_by_type TEXT NOT NULL DEFAULT '',
		decided_by_id TEXT NOT NULL DEFAULT '',
		decided_by_email TEXT NOT NULL DEFAULT '',
		decided_at TIMESTAMPTZ,
		decision_comment TEXT NOT NULL DEFAULT '',
		checkpoint_id UUID REFERENCES pipeline_run_checkpoints(id) ON DELETE SET NULL,
		UNIQUE(run_id, step_name)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_pipeline_approvals_run ON pipeline_approvals(run_id, status, requested_at DESC)`,
}

func ensureApprovalSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin approval schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range approvalSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply approval schema statement %d: %w", idx+1, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit approval schema transaction: %w", err)
	}
	return nil
}

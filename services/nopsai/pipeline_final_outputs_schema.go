package nopsai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var pipelineFinalOutputSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS pipeline_run_outputs (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		run_id UUID NOT NULL REFERENCES pipeline_runs(run_id) ON DELETE CASCADE,
		item_index INTEGER NOT NULL,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		prompt TEXT NOT NULL DEFAULT '',
		llm_profile TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		content TEXT NOT NULL DEFAULT '',
		error TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(run_id, item_index)
	)`,
	`ALTER TABLE pipeline_run_outputs ADD COLUMN IF NOT EXISTS prompt TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_run_outputs ADD COLUMN IF NOT EXISTS llm_profile TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_run_outputs ADD COLUMN IF NOT EXISTS content TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_run_outputs ADD COLUMN IF NOT EXISTS error TEXT NOT NULL DEFAULT ''`,
	`DO $$
	BEGIN
		IF NOT EXISTS (
			SELECT 1 FROM pg_constraint WHERE conname = 'pipeline_run_outputs_type_check'
		) THEN
			ALTER TABLE pipeline_run_outputs
			ADD CONSTRAINT pipeline_run_outputs_type_check
			CHECK (type IN ('markdown', 'pdf', 'excel', 'json', 'html'));
		END IF;
	END $$`,
	`DO $$
	BEGIN
		IF NOT EXISTS (
			SELECT 1 FROM pg_constraint WHERE conname = 'pipeline_run_outputs_status_check'
		) THEN
			ALTER TABLE pipeline_run_outputs
			ADD CONSTRAINT pipeline_run_outputs_status_check
			CHECK (status IN ('pending', 'generating', 'success', 'failure'));
		END IF;
	END $$`,
	`CREATE INDEX IF NOT EXISTS idx_pipeline_run_outputs_run ON pipeline_run_outputs(run_id, item_index)`,
	`CREATE INDEX IF NOT EXISTS idx_pipeline_run_outputs_status ON pipeline_run_outputs(status, updated_at DESC)`,
}

func ensurePipelineFinalOutputSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin pipeline final output schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	for idx, stmt := range pipelineFinalOutputSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply pipeline final output schema statement %d: %w", idx+1, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit pipeline final output schema transaction: %w", err)
	}
	return nil
}

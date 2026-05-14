package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var resourceAuthorizationSchemaStatements = []string{
	`ALTER TABLE pipelines ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'group'`,
	`ALTER TABLE pipelines DROP CONSTRAINT IF EXISTS pipelines_visibility_check`,
	`ALTER TABLE pipelines ADD CONSTRAINT pipelines_visibility_check CHECK (visibility IN ('group', 'restricted', 'workspace'))`,
	`ALTER TABLE steps ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'group'`,
	`ALTER TABLE steps DROP CONSTRAINT IF EXISTS steps_visibility_check`,
	`ALTER TABLE steps ADD CONSTRAINT steps_visibility_check CHECK (visibility IN ('group', 'restricted', 'workspace'))`,
	`ALTER TABLE config_repositories ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'group'`,
	`ALTER TABLE config_repositories DROP CONSTRAINT IF EXISTS config_repositories_visibility_check`,
	`ALTER TABLE config_repositories ADD CONSTRAINT config_repositories_visibility_check CHECK (visibility IN ('group', 'restricted', 'workspace'))`,
	`CREATE TABLE IF NOT EXISTS resource_visibility (
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		visibility TEXT NOT NULL DEFAULT 'group' CHECK (visibility IN ('group', 'restricted', 'workspace')),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (resource_type, resource_id)
	)`,
	`ALTER TABLE resource_visibility DROP CONSTRAINT IF EXISTS resource_visibility_visibility_check`,
	`ALTER TABLE resource_visibility ADD CONSTRAINT resource_visibility_visibility_check CHECK (visibility IN ('group', 'restricted', 'workspace'))`,
	`ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS trigger_source TEXT`,
	`ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS requested_by_type TEXT`,
	`ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS requested_by_id TEXT`,
	`ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS effective_subject_type TEXT`,
	`ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS effective_subject_id TEXT`,
	`ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS authorization_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb`,
}

func ensureResourceAuthorizationSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin resource authorization schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range resourceAuthorizationSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply resource authorization schema statement %d: %w", idx+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit resource authorization schema transaction: %w", err)
	}
	return nil
}

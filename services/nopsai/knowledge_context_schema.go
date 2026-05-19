package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var knowledgeContextSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS knowledge_contexts (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		kind TEXT NOT NULL,
		group_path TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'database',
		config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
		managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(kind, group_path, name)
	)`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'database'`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`CREATE TABLE IF NOT EXISTS resource_visibility (
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		visibility TEXT NOT NULL DEFAULT 'group' CHECK (visibility IN ('group', 'restricted', 'workspace')),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (resource_type, resource_id)
	)`,
	`DO $$
	BEGIN
		IF EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_name = 'knowledge_contexts'
			  AND column_name = 'visibility'
		) THEN
			EXECUTE 'INSERT INTO resource_visibility (resource_type, resource_id, visibility, updated_at)
				SELECT ''knowledge_context'',
				       kind || ''/'' || CASE WHEN group_path = '''' THEN '''' ELSE group_path || ''/'' END || name,
				       visibility,
				       NOW()
				FROM knowledge_contexts
				WHERE COALESCE(visibility, '''') <> ''''
				ON CONFLICT (resource_type, resource_id)
				DO UPDATE SET visibility = EXCLUDED.visibility, updated_at = NOW()';
		END IF;
	END; $$`,
	`ALTER TABLE knowledge_contexts DROP CONSTRAINT IF EXISTS knowledge_contexts_visibility_check`,
	`ALTER TABLE knowledge_contexts DROP COLUMN IF EXISTS title`,
	`ALTER TABLE knowledge_contexts DROP COLUMN IF EXISTS content_format`,
	`ALTER TABLE knowledge_contexts DROP COLUMN IF EXISTS visibility`,
	`CREATE INDEX IF NOT EXISTS idx_knowledge_contexts_kind_group ON knowledge_contexts(kind, group_path, name)`,
	`CREATE INDEX IF NOT EXISTS idx_knowledge_contexts_config_repo_id ON knowledge_contexts(config_repo_id)`,
	`CREATE TABLE IF NOT EXISTS pipeline_run_knowledge_contexts (
		id BIGSERIAL PRIMARY KEY,
		run_id UUID NOT NULL REFERENCES pipeline_runs(run_id) ON DELETE CASCADE,
		knowledge_context_id UUID REFERENCES knowledge_contexts(id) ON DELETE SET NULL,
		kind TEXT NOT NULL,
		group_path TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		ref TEXT NOT NULL DEFAULT '',
		path TEXT NOT NULL DEFAULT '',
		required BOOLEAN NOT NULL DEFAULT FALSE,
		source TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL DEFAULT '',
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
		resolved_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`ALTER TABLE pipeline_run_knowledge_contexts DROP COLUMN IF EXISTS title`,
	`ALTER TABLE pipeline_run_knowledge_contexts DROP COLUMN IF EXISTS content_format`,
	`ALTER TABLE pipeline_run_knowledge_contexts DROP COLUMN IF EXISTS visibility`,
	`CREATE INDEX IF NOT EXISTS idx_pipeline_run_knowledge_contexts_run_id ON pipeline_run_knowledge_contexts(run_id)`,
}

func ensureKnowledgeContextSchema(ctx context.Context, db *pgxpool.Pool) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin knowledge context schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range knowledgeContextSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply knowledge context schema statement %d: %w", idx+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit knowledge context schema transaction: %w", err)
	}
	return nil
}

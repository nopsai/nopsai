package nopsai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var knowledgeContextSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS knowledge_contexts (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		kind TEXT NOT NULL,
		team_path TEXT NOT NULL DEFAULT '',
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
		UNIQUE(kind, team_path, name)
	)`,
	`DO $$
	BEGIN
		IF to_regclass('knowledge_contexts') IS NOT NULL THEN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'knowledge_contexts' AND column_name = 'group_path'
			) AND NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'knowledge_contexts' AND column_name = 'team_path'
			) THEN
				ALTER TABLE knowledge_contexts RENAME COLUMN group_path TO team_path;
			END IF;
		END IF;
	END $$`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS team_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'database'`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`CREATE TABLE IF NOT EXISTS resource_visibility (
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		visibility TEXT NOT NULL DEFAULT 'team' CHECK (visibility IN ('team', 'restricted', 'workspace')),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (resource_type, resource_id)
	)`,
	`ALTER TABLE resource_visibility DROP CONSTRAINT IF EXISTS resource_visibility_visibility_check`,
	`UPDATE resource_visibility SET visibility = 'team' WHERE visibility = 'group'`,
	`ALTER TABLE resource_visibility ADD CONSTRAINT resource_visibility_visibility_check CHECK (visibility IN ('team', 'restricted', 'workspace'))`,
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
				       kind || ''/'' || CASE WHEN team_path = '''' THEN '''' ELSE team_path || ''/'' END || name,
				       CASE visibility WHEN ''group'' THEN ''team'' ELSE visibility END,
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
	`ALTER TABLE knowledge_contexts DROP CONSTRAINT IF EXISTS knowledge_contexts_kind_group_path_name_key`,
	`DO $$
	BEGIN
		IF NOT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conname = 'knowledge_contexts_kind_team_path_name_key'
		) THEN
			ALTER TABLE knowledge_contexts
			ADD CONSTRAINT knowledge_contexts_kind_team_path_name_key UNIQUE(kind, team_path, name);
		END IF;
	END $$`,
	`DROP INDEX IF EXISTS idx_knowledge_contexts_kind_group`,
	`CREATE INDEX IF NOT EXISTS idx_knowledge_contexts_kind_team ON knowledge_contexts(kind, team_path, name)`,
	`CREATE INDEX IF NOT EXISTS idx_knowledge_contexts_config_repo_id ON knowledge_contexts(config_repo_id)`,
	`CREATE TABLE IF NOT EXISTS pipeline_run_knowledge_contexts (
		id BIGSERIAL PRIMARY KEY,
		run_id UUID NOT NULL REFERENCES pipeline_runs(run_id) ON DELETE CASCADE,
		knowledge_context_id UUID REFERENCES knowledge_contexts(id) ON DELETE SET NULL,
		kind TEXT NOT NULL,
		team_path TEXT NOT NULL DEFAULT '',
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
	`DO $$
	BEGIN
		IF to_regclass('pipeline_run_knowledge_contexts') IS NOT NULL THEN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'pipeline_run_knowledge_contexts' AND column_name = 'group_path'
			) AND NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'pipeline_run_knowledge_contexts' AND column_name = 'team_path'
			) THEN
				ALTER TABLE pipeline_run_knowledge_contexts RENAME COLUMN group_path TO team_path;
			END IF;
		END IF;
	END $$`,
	`ALTER TABLE pipeline_run_knowledge_contexts ADD COLUMN IF NOT EXISTS team_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_run_knowledge_contexts DROP COLUMN IF EXISTS title`,
	`ALTER TABLE pipeline_run_knowledge_contexts DROP COLUMN IF EXISTS content_format`,
	`ALTER TABLE pipeline_run_knowledge_contexts DROP COLUMN IF EXISTS visibility`,
	`CREATE INDEX IF NOT EXISTS idx_pipeline_run_knowledge_contexts_run_id ON pipeline_run_knowledge_contexts(run_id)`,
	`DO $$
	BEGIN
		IF to_regclass('knowledge_context_legacy_metadata_backup') IS NOT NULL THEN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public'
				  AND table_name = 'knowledge_context_legacy_metadata_backup'
				  AND column_name = 'group_path'
			)
			AND NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public'
				  AND table_name = 'knowledge_context_legacy_metadata_backup'
				  AND column_name = 'team_path'
			) THEN
				ALTER TABLE knowledge_context_legacy_metadata_backup RENAME COLUMN group_path TO team_path;
			ELSIF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public'
				  AND table_name = 'knowledge_context_legacy_metadata_backup'
				  AND column_name = 'group_path'
			) THEN
				UPDATE knowledge_context_legacy_metadata_backup
				SET team_path = group_path
				WHERE team_path = ''
				  AND group_path <> '';

				ALTER TABLE knowledge_context_legacy_metadata_backup DROP COLUMN group_path;
			END IF;

			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public'
				  AND table_name = 'knowledge_context_legacy_metadata_backup'
				  AND column_name = 'visibility'
			) THEN
				UPDATE knowledge_context_legacy_metadata_backup
				SET visibility = 'team'
				WHERE visibility = 'group';
			END IF;
		END IF;
	END $$`,
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

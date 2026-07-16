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
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS team_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'database'`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`CREATE TABLE IF NOT EXISTS knowledge_context_connections (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		team_path TEXT NOT NULL,
		name TEXT NOT NULL,
		display_name TEXT NOT NULL DEFAULT '',
		provider TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'authentication_required',
		disabled BOOLEAN NOT NULL DEFAULT FALSE,
		credential_ref TEXT NOT NULL DEFAULT '',
		credential_secret_ref TEXT NOT NULL DEFAULT '',
		base_url TEXT NOT NULL DEFAULT '',
		scopes JSONB NOT NULL DEFAULT '{}'::jsonb,
		config JSONB NOT NULL DEFAULT '{}'::jsonb,
		provider_config JSONB NOT NULL DEFAULT '{}'::jsonb,
		last_checked_at TIMESTAMPTZ,
		last_error TEXT NOT NULL DEFAULT '',
		created_by TEXT NOT NULL DEFAULT '',
		updated_by TEXT NOT NULL DEFAULT '',
		disabled_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(team_path, name)
	)`,
	`ALTER TABLE knowledge_context_connections ADD COLUMN IF NOT EXISTS display_name TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_context_connections ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT 'wiki'`,
	`ALTER TABLE knowledge_context_connections ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'authentication_required'`,
	`ALTER TABLE knowledge_context_connections ADD COLUMN IF NOT EXISTS disabled BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE knowledge_context_connections ADD COLUMN IF NOT EXISTS credential_ref TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_context_connections ADD COLUMN IF NOT EXISTS credential_secret_ref TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_context_connections ADD COLUMN IF NOT EXISTS base_url TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_context_connections ADD COLUMN IF NOT EXISTS scopes JSONB NOT NULL DEFAULT '{}'::jsonb`,
	`ALTER TABLE knowledge_context_connections ADD COLUMN IF NOT EXISTS config JSONB NOT NULL DEFAULT '{}'::jsonb`,
	`ALTER TABLE knowledge_context_connections ADD COLUMN IF NOT EXISTS provider_config JSONB NOT NULL DEFAULT '{}'::jsonb`,
	`ALTER TABLE knowledge_context_connections ADD COLUMN IF NOT EXISTS last_checked_at TIMESTAMPTZ`,
	`ALTER TABLE knowledge_context_connections ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_context_connections ADD COLUMN IF NOT EXISTS created_by TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_context_connections ADD COLUMN IF NOT EXISTS updated_by TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_context_connections ADD COLUMN IF NOT EXISTS disabled_at TIMESTAMPTZ`,
	`CREATE INDEX IF NOT EXISTS idx_knowledge_context_connections_team ON knowledge_context_connections(team_path, name)`,
	`CREATE INDEX IF NOT EXISTS idx_knowledge_context_connections_provider ON knowledge_context_connections(provider)`,
	`UPDATE knowledge_context_connections SET credential_secret_ref = credential_ref WHERE credential_secret_ref = '' AND credential_ref <> ''`,
	`UPDATE knowledge_context_connections SET provider_config = config WHERE provider_config = '{}'::jsonb AND config <> '{}'::jsonb`,
	`UPDATE knowledge_context_connections SET disabled_at = updated_at WHERE disabled = TRUE AND disabled_at IS NULL`,
	`UPDATE knowledge_context_connections SET status = 'disabled' WHERE disabled = TRUE AND status <> 'disabled'`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS connection_id UUID REFERENCES knowledge_context_connections(id) ON DELETE SET NULL`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS content_source TEXT NOT NULL DEFAULT 'inline'`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS external_provider TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS external_page_id TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS external_page_url TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS external_page_title TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS sync_mode TEXT NOT NULL DEFAULT 'manual'`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS sync_interval_minutes INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS failure_mode TEXT NOT NULL DEFAULT 'fail'`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS sync_failure_mode TEXT NOT NULL DEFAULT 'fail'`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS synced_content TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS source_modified_at TIMESTAMPTZ`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS sync_status TEXT NOT NULL DEFAULT 'not_synced'`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS last_sync_status TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS last_sync_started_at TIMESTAMPTZ`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS last_synced_at TIMESTAMPTZ`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS next_sync_attempt_at TIMESTAMPTZ`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS sync_attempt_count INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS sync_error TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS last_sync_error TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS content_hash TEXT NOT NULL DEFAULT ''`,
	`CREATE TABLE IF NOT EXISTS knowledge_context_assets (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		knowledge_context_id UUID NOT NULL REFERENCES knowledge_contexts(id) ON DELETE CASCADE,
		provider TEXT NOT NULL DEFAULT '',
		external_page_id TEXT NOT NULL DEFAULT '',
		source_block_id TEXT NOT NULL DEFAULT '',
		source_block_type TEXT NOT NULL DEFAULT '',
		asset_kind TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '',
		url TEXT NOT NULL DEFAULT '',
		media_type TEXT NOT NULL DEFAULT '',
		content_hash TEXT NOT NULL DEFAULT '',
		metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(knowledge_context_id, source_block_id, asset_kind, url)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_knowledge_context_assets_context ON knowledge_context_assets(knowledge_context_id)`,
	`CREATE INDEX IF NOT EXISTS idx_knowledge_context_assets_provider_kind ON knowledge_context_assets(provider, asset_kind)`,
	`UPDATE knowledge_contexts SET content_source = 'external_page' WHERE content_source = 'inline' AND (connection_id IS NOT NULL OR external_page_id <> '' OR external_page_url <> '')`,
	`UPDATE knowledge_contexts SET synced_content = content WHERE synced_content = '' AND content <> ''`,
	`UPDATE knowledge_contexts SET sync_failure_mode = failure_mode WHERE sync_failure_mode = 'fail' AND failure_mode <> ''`,
	`UPDATE knowledge_contexts SET last_sync_status = sync_status WHERE last_sync_status = '' AND sync_status <> ''`,
	`UPDATE knowledge_contexts SET last_sync_error = sync_error WHERE last_sync_error = '' AND sync_error <> ''`,
	`CREATE INDEX IF NOT EXISTS idx_knowledge_contexts_connection_id ON knowledge_contexts(connection_id)`,
	`CREATE INDEX IF NOT EXISTS idx_knowledge_contexts_periodic_sync_due ON knowledge_contexts(sync_mode, next_sync_attempt_at, last_synced_at)
			WHERE sync_mode = 'periodic' AND (content_source = 'external_page' OR connection_id IS NOT NULL OR external_page_id <> '' OR external_page_url <> '')`,
	`CREATE TABLE IF NOT EXISTS resource_visibility (
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		visibility TEXT NOT NULL DEFAULT 'team' CHECK (visibility IN ('team', 'restricted', 'workspace')),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (resource_type, resource_id)
	)`,
	`ALTER TABLE resource_visibility DROP CONSTRAINT IF EXISTS resource_visibility_visibility_check`,
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
	`ALTER TABLE pipeline_run_knowledge_contexts ADD COLUMN IF NOT EXISTS team_path TEXT NOT NULL DEFAULT ''`,
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

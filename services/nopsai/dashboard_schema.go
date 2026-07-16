package nopsai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var dashboardSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS dashboards (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
		slug TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		visibility TEXT NOT NULL DEFAULT 'team',
		refresh_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
		source TEXT NOT NULL DEFAULT 'database',
		config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
		managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
		created_by TEXT NOT NULL DEFAULT '',
		updated_by TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		CONSTRAINT dashboards_slug_check CHECK (slug ~ '^[a-zA-Z0-9_.-]+$'),
		CONSTRAINT dashboards_visibility_check CHECK (visibility IN ('team', 'restricted', 'workspace')),
		UNIQUE(team_id, slug)
	)`,
	`ALTER TABLE dashboards ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'database'`,
	`ALTER TABLE dashboards ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE dashboards ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE dashboards ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE dashboards ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`CREATE TABLE IF NOT EXISTS dashboard_sections (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		dashboard_id UUID NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
		section_key TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		layout JSONB NOT NULL DEFAULT '{}'::jsonb,
		display_order INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		CONSTRAINT dashboard_sections_key_check CHECK (section_key ~ '^[a-zA-Z0-9_.-]+$'),
		UNIQUE(dashboard_id, section_key)
	)`,
	`CREATE TABLE IF NOT EXISTS dashboard_source_bindings (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		dashboard_id UUID NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
		section_key TEXT NOT NULL,
		pipeline_id TEXT NOT NULL,
		output_name TEXT NOT NULL,
		entry_key TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		required_for_refresh BOOLEAN NOT NULL DEFAULT TRUE,
		refresh_order INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		CONSTRAINT dashboard_source_bindings_section_check CHECK (section_key ~ '^[a-zA-Z0-9_.-]+$'),
		UNIQUE(dashboard_id, section_key, pipeline_id, output_name, entry_key)
	)`,
	`CREATE TABLE IF NOT EXISTS dashboard_publications (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		dashboard_id UUID NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
		section_key TEXT NOT NULL,
		entry_key TEXT NOT NULL,
		mode TEXT NOT NULL,
		content JSONB NOT NULL,
		revision INTEGER NOT NULL DEFAULT 1,
		run_id UUID REFERENCES pipeline_runs(run_id) ON DELETE SET NULL,
		run_output_id UUID REFERENCES pipeline_run_outputs(id) ON DELETE SET NULL,
		pipeline_id TEXT NOT NULL DEFAULT '',
		output_name TEXT NOT NULL DEFAULT '',
		refresh_id UUID,
		source_finished_at TIMESTAMPTZ,
		published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		expires_at TIMESTAMPTZ,
		status TEXT NOT NULL DEFAULT 'current',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		CONSTRAINT dashboard_publications_mode_check CHECK (mode IN ('replace', 'append')),
		CONSTRAINT dashboard_publications_status_check CHECK (status IN ('current', 'archived')),
		CONSTRAINT dashboard_publications_revision_check CHECK (revision >= 1),
		CONSTRAINT dashboard_publications_section_check CHECK (section_key ~ '^[a-zA-Z0-9_.-]+$')
	)`,
	`CREATE TABLE IF NOT EXISTS dashboard_publication_events (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		dashboard_id UUID NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
		section_key TEXT NOT NULL,
		entry_key TEXT NOT NULL,
		publication_id UUID REFERENCES dashboard_publications(id) ON DELETE SET NULL,
		revision INTEGER NOT NULL DEFAULT 0,
		event_type TEXT NOT NULL,
		content JSONB NOT NULL DEFAULT '{}'::jsonb,
		run_id UUID REFERENCES pipeline_runs(run_id) ON DELETE SET NULL,
		refresh_id UUID,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE IF NOT EXISTS dashboard_refreshes (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		dashboard_id UUID NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
		requested_by_type TEXT NOT NULL DEFAULT '',
		requested_by_id TEXT NOT NULL DEFAULT '',
		trigger_type TEXT NOT NULL DEFAULT 'manual',
		scope_type TEXT NOT NULL DEFAULT 'dashboard',
		scope JSONB NOT NULL DEFAULT '{}'::jsonb,
		mode TEXT NOT NULL DEFAULT 'strict',
		status TEXT NOT NULL DEFAULT 'running',
		total_sources INTEGER NOT NULL DEFAULT 0,
		required_sources INTEGER NOT NULL DEFAULT 0,
		queued_sources INTEGER NOT NULL DEFAULT 0,
		running_sources INTEGER NOT NULL DEFAULT 0,
		successful_sources INTEGER NOT NULL DEFAULT 0,
		failed_sources INTEGER NOT NULL DEFAULT 0,
		skipped_sources INTEGER NOT NULL DEFAULT 0,
		max_concurrency INTEGER NOT NULL DEFAULT 4,
		timeout_seconds INTEGER NOT NULL DEFAULT 2700,
		idempotency_key TEXT NOT NULL DEFAULT '',
		error TEXT NOT NULL DEFAULT '',
		started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		finished_at TIMESTAMPTZ,
		timeout_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		CONSTRAINT dashboard_refreshes_scope_type_check CHECK (scope_type IN ('dashboard', 'section', 'source')),
		CONSTRAINT dashboard_refreshes_mode_check CHECK (mode IN ('strict', 'best_effort')),
		CONSTRAINT dashboard_refreshes_status_check CHECK (status IN ('running', 'complete', 'partial', 'failed', 'cancelled', 'timed_out')),
		CONSTRAINT dashboard_refreshes_source_counts_check CHECK (
			total_sources >= 0 AND required_sources >= 0 AND queued_sources >= 0 AND running_sources >= 0
			AND successful_sources >= 0 AND failed_sources >= 0 AND skipped_sources >= 0
		),
		CONSTRAINT dashboard_refreshes_concurrency_check CHECK (max_concurrency >= 1),
		CONSTRAINT dashboard_refreshes_timeout_check CHECK (timeout_seconds >= 0)
	)`,
	`CREATE TABLE IF NOT EXISTS dashboard_refresh_pipeline_runs (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		refresh_id UUID NOT NULL REFERENCES dashboard_refreshes(id) ON DELETE CASCADE,
		dashboard_id UUID NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
		source_binding_id UUID REFERENCES dashboard_source_bindings(id) ON DELETE SET NULL,
		pipeline_id TEXT NOT NULL DEFAULT '',
		output_name TEXT NOT NULL DEFAULT '',
		section_key TEXT NOT NULL DEFAULT '',
		entry_key TEXT NOT NULL DEFAULT '',
		run_id UUID REFERENCES pipeline_runs(run_id) ON DELETE SET NULL,
		required BOOLEAN NOT NULL DEFAULT TRUE,
		status TEXT NOT NULL DEFAULT 'queued',
		error TEXT NOT NULL DEFAULT '',
		started_at TIMESTAMPTZ,
		finished_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		CONSTRAINT dashboard_refresh_pipeline_runs_status_check CHECK (status IN ('queued', 'running', 'success', 'failed', 'skipped', 'cancelled', 'timed_out')),
		CONSTRAINT dashboard_refresh_pipeline_runs_section_check CHECK (section_key = '' OR section_key ~ '^[a-zA-Z0-9_.-]+$'),
		UNIQUE(refresh_id, source_binding_id)
	)`,
	`CREATE TABLE IF NOT EXISTS dashboard_refresh_schedules (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		dashboard_id UUID NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		cron_expression TEXT NOT NULL,
		timezone TEXT NOT NULL DEFAULT 'UTC',
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		scope_type TEXT NOT NULL DEFAULT 'dashboard',
		scope JSONB NOT NULL DEFAULT '{}'::jsonb,
		mode TEXT NOT NULL DEFAULT 'strict',
		run_scope TEXT NOT NULL DEFAULT '',
		variables JSONB NOT NULL DEFAULT '{}'::jsonb,
		max_concurrency INTEGER NOT NULL DEFAULT 4,
		timeout_seconds INTEGER NOT NULL DEFAULT 2700,
		next_run_at TIMESTAMPTZ,
		last_refresh_id UUID REFERENCES dashboard_refreshes(id) ON DELETE SET NULL,
		last_status TEXT NOT NULL DEFAULT '',
		service_account_id TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'database',
		config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
		managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
		created_by TEXT NOT NULL DEFAULT '',
		updated_by TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		CONSTRAINT dashboard_refresh_schedules_name_check CHECK (name ~ '^[a-zA-Z0-9_.-]+$'),
		CONSTRAINT dashboard_refresh_schedules_scope_type_check CHECK (scope_type IN ('dashboard', 'section', 'source')),
		CONSTRAINT dashboard_refresh_schedules_mode_check CHECK (mode IN ('strict', 'best_effort')),
		CONSTRAINT dashboard_refresh_schedules_concurrency_check CHECK (max_concurrency >= 1 AND max_concurrency <= 16),
		CONSTRAINT dashboard_refresh_schedules_timeout_check CHECK (timeout_seconds >= 0 AND timeout_seconds <= 43200),
		UNIQUE(dashboard_id, name)
	)`,
	`ALTER TABLE dashboard_publications DROP CONSTRAINT IF EXISTS dashboard_publications_mode_check`,
	`ALTER TABLE dashboard_publications ADD CONSTRAINT dashboard_publications_mode_check CHECK (mode IN ('replace', 'append', 'snapshot', 'series'))`,
	`CREATE INDEX IF NOT EXISTS idx_dashboards_team ON dashboards(team_id, slug)`,
	`CREATE INDEX IF NOT EXISTS idx_dashboard_sections_dashboard ON dashboard_sections(dashboard_id, display_order, section_key)`,
	`CREATE INDEX IF NOT EXISTS idx_dashboard_sources_dashboard ON dashboard_source_bindings(dashboard_id, section_key, refresh_order)`,
	`CREATE INDEX IF NOT EXISTS idx_dashboard_publications_dashboard ON dashboard_publications(dashboard_id, section_key, status, published_at DESC)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_dashboard_publications_replace_current ON dashboard_publications(dashboard_id, section_key, entry_key) WHERE mode = 'replace' AND status = 'current'`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_dashboard_publications_series_current ON dashboard_publications(dashboard_id, section_key, entry_key) WHERE mode = 'series' AND status = 'current'`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_dashboard_publications_run_output ON dashboard_publications(run_output_id) WHERE run_output_id IS NOT NULL`,
	`CREATE INDEX IF NOT EXISTS idx_dashboard_publication_events_dashboard ON dashboard_publication_events(dashboard_id, section_key, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_dashboard_refreshes_dashboard ON dashboard_refreshes(dashboard_id, created_at DESC)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_dashboard_refreshes_active ON dashboard_refreshes(dashboard_id) WHERE status = 'running'`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_dashboard_refreshes_idempotency ON dashboard_refreshes(dashboard_id, idempotency_key) WHERE idempotency_key <> ''`,
	`CREATE INDEX IF NOT EXISTS idx_dashboard_refresh_pipeline_runs_refresh ON dashboard_refresh_pipeline_runs(refresh_id, status, created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_dashboard_refresh_pipeline_runs_run ON dashboard_refresh_pipeline_runs(run_id) WHERE run_id IS NOT NULL`,
	`CREATE INDEX IF NOT EXISTS idx_dashboard_refresh_schedules_dashboard ON dashboard_refresh_schedules(dashboard_id, name)`,
	`CREATE INDEX IF NOT EXISTS idx_dashboard_refresh_schedules_next_run ON dashboard_refresh_schedules(enabled, next_run_at)`,
	`CREATE INDEX IF NOT EXISTS idx_dashboard_refresh_schedules_config_repo_id ON dashboard_refresh_schedules(config_repo_id)`,
}

func ensureDashboardSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin dashboard schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	for idx, stmt := range dashboardSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply dashboard schema statement %d: %w", idx+1, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit dashboard schema transaction: %w", err)
	}
	return nil
}

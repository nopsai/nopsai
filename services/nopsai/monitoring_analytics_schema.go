package nopsai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var monitoringAnalyticsSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS ai_usage_events (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		run_id UUID REFERENCES pipeline_runs(run_id) ON DELETE SET NULL,
		step_name TEXT NOT NULL DEFAULT '',
		task_name TEXT NOT NULL DEFAULT '',
		pipeline_path TEXT NOT NULL DEFAULT '',
		pipeline_name TEXT NOT NULL DEFAULT '',
		team_id INTEGER REFERENCES teams(id) ON DELETE SET NULL,
		feature TEXT NOT NULL DEFAULT '',
		provider TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		prompt_tokens BIGINT NOT NULL DEFAULT 0,
		completion_tokens BIGINT NOT NULL DEFAULT 0,
		total_tokens BIGINT NOT NULL DEFAULT 0,
		input_cost_usd NUMERIC(18, 8) NOT NULL DEFAULT 0,
		output_cost_usd NUMERIC(18, 8) NOT NULL DEFAULT 0,
		total_cost_usd NUMERIC(18, 8) NOT NULL DEFAULT 0,
		requested_by_type TEXT NOT NULL DEFAULT '',
		requested_by_id TEXT NOT NULL DEFAULT '',
		effective_subject_type TEXT NOT NULL DEFAULT '',
		effective_subject_id TEXT NOT NULL DEFAULT '',
			metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	`ALTER TABLE ai_usage_events ADD COLUMN IF NOT EXISTS team_id INTEGER REFERENCES teams(id) ON DELETE SET NULL`,
	`CREATE INDEX IF NOT EXISTS idx_ai_usage_events_run ON ai_usage_events(run_id)`,
	`CREATE INDEX IF NOT EXISTS idx_ai_usage_events_pipeline_created ON ai_usage_events(pipeline_path, pipeline_name, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_ai_usage_events_team_created ON ai_usage_events(team_id, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_ai_usage_events_feature_created ON ai_usage_events(feature, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_ai_usage_events_subject_created ON ai_usage_events(effective_subject_type, effective_subject_id, created_at DESC)`,

	`CREATE TABLE IF NOT EXISTS runner_metric_snapshots (
		id BIGSERIAL PRIMARY KEY,
		runner_id TEXT NOT NULL,
		runtime TEXT NOT NULL DEFAULT '',
		namespace TEXT NOT NULL DEFAULT '',
		node TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT '',
		capacity INTEGER NOT NULL DEFAULT 0,
		active_jobs INTEGER NOT NULL DEFAULT 0,
		inflight_jobs INTEGER NOT NULL DEFAULT 0,
		queued_jobs INTEGER NOT NULL DEFAULT 0,
		allow_dispatch BOOLEAN NOT NULL DEFAULT TRUE,
		sampled_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_runner_metric_snapshots_sampled ON runner_metric_snapshots(sampled_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_runner_metric_snapshots_runner_sampled ON runner_metric_snapshots(runner_id, sampled_at DESC)`,

	`CREATE TABLE IF NOT EXISTS pipeline_run_usage_summary (
		run_id UUID PRIMARY KEY REFERENCES pipeline_runs(run_id) ON DELETE CASCADE,
		total_runtime_seconds BIGINT NOT NULL DEFAULT 0,
		runner_cost_usd NUMERIC(18, 8) NOT NULL DEFAULT 0,
		ai_prompt_tokens BIGINT NOT NULL DEFAULT 0,
		ai_completion_tokens BIGINT NOT NULL DEFAULT 0,
		ai_total_tokens BIGINT NOT NULL DEFAULT 0,
		ai_cost_usd NUMERIC(18, 8) NOT NULL DEFAULT 0,
		total_cost_usd NUMERIC(18, 8) NOT NULL DEFAULT 0,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_pipeline_run_usage_summary_total_cost ON pipeline_run_usage_summary(total_cost_usd DESC)`,

	`CREATE TABLE IF NOT EXISTS monitoring_saved_views (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name TEXT NOT NULL,
		owner_subject_type TEXT NOT NULL DEFAULT '',
		owner_subject_id TEXT NOT NULL DEFAULT '',
		visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'team', 'workspace')),
		team_id INTEGER REFERENCES teams(id) ON DELETE SET NULL,
		filters JSONB NOT NULL DEFAULT '{}'::jsonb,
		columns JSONB NOT NULL DEFAULT '[]'::jsonb,
		source TEXT NOT NULL DEFAULT 'database',
		config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
			managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	`ALTER TABLE monitoring_saved_views ADD COLUMN IF NOT EXISTS team_id INTEGER REFERENCES teams(id) ON DELETE SET NULL`,
	`ALTER TABLE monitoring_saved_views DROP CONSTRAINT IF EXISTS monitoring_saved_views_visibility_check`,
	`ALTER TABLE monitoring_saved_views ADD CONSTRAINT monitoring_saved_views_visibility_check CHECK (visibility IN ('private', 'team', 'workspace'))`,
	`CREATE INDEX IF NOT EXISTS idx_monitoring_saved_views_owner ON monitoring_saved_views(owner_subject_type, owner_subject_id)`,
	`CREATE INDEX IF NOT EXISTS idx_monitoring_saved_views_config_repo ON monitoring_saved_views(config_repo_id)`,

	`CREATE TABLE IF NOT EXISTS monitoring_alert_rules (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		owner_subject_type TEXT NOT NULL DEFAULT '',
		owner_subject_id TEXT NOT NULL DEFAULT '',
		visibility TEXT NOT NULL DEFAULT 'workspace' CHECK (visibility IN ('private', 'team', 'workspace')),
		severity TEXT NOT NULL DEFAULT 'warning',
		metric TEXT NOT NULL,
		comparator TEXT NOT NULL DEFAULT 'gt',
		threshold NUMERIC(18, 8) NOT NULL DEFAULT 0,
		window_seconds INTEGER NOT NULL DEFAULT 3600,
		filters JSONB NOT NULL DEFAULT '{}'::jsonb,
		notification_route_id BIGINT REFERENCES notification_routes(id) ON DELETE SET NULL,
		source TEXT NOT NULL DEFAULT 'database',
		config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
		managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`ALTER TABLE monitoring_alert_rules ADD COLUMN IF NOT EXISTS owner_subject_type TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE monitoring_alert_rules ADD COLUMN IF NOT EXISTS owner_subject_id TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE monitoring_alert_rules ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'workspace'`,
	`ALTER TABLE monitoring_alert_rules DROP CONSTRAINT IF EXISTS monitoring_alert_rules_visibility_check`,
	`ALTER TABLE monitoring_alert_rules ADD CONSTRAINT monitoring_alert_rules_visibility_check CHECK (visibility IN ('private', 'team', 'workspace'))`,
	`CREATE INDEX IF NOT EXISTS idx_monitoring_alert_rules_enabled ON monitoring_alert_rules(enabled)`,
	`CREATE INDEX IF NOT EXISTS idx_monitoring_alert_rules_owner ON monitoring_alert_rules(owner_subject_type, owner_subject_id)`,
	`CREATE INDEX IF NOT EXISTS idx_monitoring_alert_rules_config_repo ON monitoring_alert_rules(config_repo_id)`,

	`CREATE TABLE IF NOT EXISTS monitoring_alert_events (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		rule_id UUID REFERENCES monitoring_alert_rules(id) ON DELETE SET NULL,
		status TEXT NOT NULL DEFAULT 'firing',
		value NUMERIC(18, 8) NOT NULL DEFAULT 0,
		message TEXT NOT NULL DEFAULT '',
		started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		resolved_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_monitoring_alert_events_rule_created ON monitoring_alert_events(rule_id, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_monitoring_alert_events_status_created ON monitoring_alert_events(status, created_at DESC)`,

	`CREATE TABLE IF NOT EXISTS monitoring_recommendations (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		fingerprint TEXT NOT NULL UNIQUE,
		category TEXT NOT NULL DEFAULT '',
		severity TEXT NOT NULL DEFAULT 'info',
		status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'acknowledged', 'resolved')),
		message TEXT NOT NULL,
		metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
		first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		resolved_at TIMESTAMPTZ
	)`,
	`CREATE INDEX IF NOT EXISTS idx_monitoring_recommendations_status_seen ON monitoring_recommendations(status, last_seen_at DESC)`,
}

func ensureMonitoringAnalyticsSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin monitoring analytics schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range monitoringAnalyticsSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply monitoring analytics schema statement %d: %w", idx+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit monitoring analytics schema transaction: %w", err)
	}
	return nil
}

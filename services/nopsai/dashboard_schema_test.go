package nopsai

import (
	"strings"
	"testing"
)

func TestDashboardSchemaCoversStepOnePersistence(t *testing.T) {
	combined := strings.Join(dashboardSchemaStatements, "\n")
	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS dashboards",
		"CREATE TABLE IF NOT EXISTS dashboard_sections",
		"CREATE TABLE IF NOT EXISTS dashboard_source_bindings",
		"CREATE TABLE IF NOT EXISTS dashboard_publications",
		"CREATE TABLE IF NOT EXISTS dashboard_publication_events",
		"CREATE TABLE IF NOT EXISTS dashboard_refreshes",
		"CREATE TABLE IF NOT EXISTS dashboard_refresh_pipeline_runs",
		"CREATE TABLE IF NOT EXISTS dashboard_refresh_schedules",
		"refresh_policy JSONB NOT NULL DEFAULT '{}'::jsonb",
		"managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE",
		"required_for_refresh BOOLEAN NOT NULL DEFAULT TRUE",
		"run_output_id UUID REFERENCES pipeline_run_outputs(id) ON DELETE SET NULL",
		"expires_at TIMESTAMPTZ",
		"mode IN ('replace', 'append', 'snapshot', 'series')",
		"status IN ('running', 'complete', 'partial', 'failed', 'cancelled', 'timed_out')",
		"idx_dashboard_publications_replace_current",
		"idx_dashboard_publications_series_current",
		"idx_dashboard_publications_run_output",
		"idx_dashboard_refreshes_active",
		"idx_dashboard_refreshes_idempotency",
		"idx_dashboard_refresh_pipeline_runs_run",
		"idx_dashboard_refresh_schedules_next_run",
	} {
		if !strings.Contains(combined, expected) {
			t.Fatalf("dashboard schema missing %q", expected)
		}
	}
}

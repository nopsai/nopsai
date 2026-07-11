package nopsai

import (
	"strings"
	"testing"
)

func TestMonitoringAnalyticsSchemaMigratesLegacyGroupColumns(t *testing.T) {
	joined := strings.Join(monitoringAnalyticsSchemaStatements, "\n")
	required := []string{
		"ALTER TABLE ai_usage_events RENAME COLUMN group_id TO team_id",
		"ALTER TABLE ai_usage_events ADD COLUMN IF NOT EXISTS team_id INTEGER REFERENCES teams(id) ON DELETE SET NULL",
		"ALTER TABLE ai_usage_events RENAME CONSTRAINT ai_usage_events_group_id_fkey TO ai_usage_events_team_id_fkey",
		"DROP INDEX IF EXISTS idx_ai_usage_events_group_created",
		"ALTER TABLE monitoring_saved_views RENAME COLUMN group_id TO team_id",
		"ALTER TABLE monitoring_saved_views ADD COLUMN IF NOT EXISTS team_id INTEGER REFERENCES teams(id) ON DELETE SET NULL",
		"ALTER TABLE monitoring_saved_views RENAME CONSTRAINT monitoring_saved_views_group_id_fkey TO monitoring_saved_views_team_id_fkey",
		"UPDATE monitoring_saved_views SET visibility = 'team' WHERE visibility = 'group'",
		"ALTER TABLE monitoring_saved_views ADD CONSTRAINT monitoring_saved_views_visibility_check CHECK (visibility IN ('private', 'team', 'workspace'))",
		"UPDATE monitoring_alert_rules SET visibility = 'team' WHERE visibility = 'group'",
		"ALTER TABLE monitoring_alert_rules ADD CONSTRAINT monitoring_alert_rules_visibility_check CHECK (visibility IN ('private', 'team', 'workspace'))",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("monitoring analytics schema missing legacy team migration %q", statement)
		}
	}
}

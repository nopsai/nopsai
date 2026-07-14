package nopsai

import (
	"strings"
	"testing"
)

func TestMonitoringAnalyticsSchemaUsesTeamColumns(t *testing.T) {
	joined := strings.Join(monitoringAnalyticsSchemaStatements, "\n")
	required := []string{
		"ALTER TABLE ai_usage_events ADD COLUMN IF NOT EXISTS team_id INTEGER REFERENCES teams(id) ON DELETE SET NULL",
		"CREATE INDEX IF NOT EXISTS idx_ai_usage_events_team_created ON ai_usage_events(team_id, created_at DESC)",
		"ALTER TABLE monitoring_saved_views ADD COLUMN IF NOT EXISTS team_id INTEGER REFERENCES teams(id) ON DELETE SET NULL",
		"ALTER TABLE monitoring_saved_views ADD CONSTRAINT monitoring_saved_views_visibility_check CHECK (visibility IN ('private', 'team', 'workspace'))",
		"ALTER TABLE monitoring_alert_rules ADD CONSTRAINT monitoring_alert_rules_visibility_check CHECK (visibility IN ('private', 'team', 'workspace'))",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("monitoring analytics schema missing team statement %q", statement)
		}
	}
	assertTeamOnlySchemaVocabulary(t, joined)
}

package nopsai

import (
	"strings"
	"testing"
)

func TestNotificationSchemaUsesTeamRouteColumn(t *testing.T) {
	joined := strings.Join(notificationSchemaStatements, "\n")
	required := []string{
		"CREATE TABLE IF NOT EXISTS notification_routes",
		"team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE",
		"ADD CONSTRAINT notification_routes_team_id_fkey FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE",
		"ADD CONSTRAINT notification_routes_team_id_key UNIQUE(team_id)",
		"CREATE INDEX IF NOT EXISTS idx_notification_routes_team ON notification_routes(team_id)",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("notification schema missing team route statement %q", statement)
		}
	}
	assertTeamOnlySchemaVocabulary(t, joined)
}

package nopsai

import (
	"strings"
	"testing"
)

func TestNotificationSchemaMigratesLegacyGroupRouteColumn(t *testing.T) {
	joined := strings.Join(notificationSchemaStatements, "\n")
	required := []string{
		"ALTER TABLE notification_routes RENAME COLUMN group_id TO team_id",
		"ALTER TABLE notification_routes DROP CONSTRAINT IF EXISTS notification_routes_group_id_fkey",
		"ALTER TABLE notification_routes DROP CONSTRAINT IF EXISTS notification_routes_group_id_key",
		"ADD CONSTRAINT notification_routes_team_id_fkey FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE",
		"ADD CONSTRAINT notification_routes_team_id_key UNIQUE(team_id)",
		"DROP INDEX IF EXISTS idx_notification_routes_group",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("notification schema missing legacy route migration %q", statement)
		}
	}
}

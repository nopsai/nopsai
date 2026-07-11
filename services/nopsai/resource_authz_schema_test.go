package nopsai

import (
	"strings"
	"testing"
)

func TestResourceAuthorizationSchemaMigratesLegacyGroupVisibilityBeforeChecks(t *testing.T) {
	joined := strings.Join(resourceAuthorizationSchemaStatements, "\n")
	for _, table := range []string{"pipelines", "steps", "triggers", "config_repositories", "resource_visibility"} {
		update := "UPDATE " + table + " SET visibility = 'team' WHERE visibility = 'group'"
		if !strings.Contains(joined, update) {
			t.Fatalf("resource authorization schema missing legacy visibility migration %q", update)
		}
	}

	assertVisibilityMigrationOrder(t, joined, "pipelines")
	assertVisibilityMigrationOrder(t, joined, "steps")
	assertVisibilityMigrationOrder(t, joined, "triggers")
	assertVisibilityMigrationOrder(t, joined, "config_repositories")
	assertVisibilityMigrationOrder(t, joined, "resource_visibility")
}

func assertVisibilityMigrationOrder(t *testing.T, statements, table string) {
	t.Helper()
	drop := strings.Index(statements, "ALTER TABLE "+table+" DROP CONSTRAINT IF EXISTS "+table+"_visibility_check")
	update := strings.Index(statements, "UPDATE "+table+" SET visibility = 'team' WHERE visibility = 'group'")
	add := strings.Index(statements, "ALTER TABLE "+table+" ADD CONSTRAINT "+table+"_visibility_check")
	if drop == -1 || update == -1 || add == -1 || drop > update || update > add {
		t.Fatalf("%s visibility migration order is wrong; drop index %d update index %d add index %d", table, drop, update, add)
	}
}

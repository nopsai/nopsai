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
		defaultRepair := "ALTER TABLE " + table + " ALTER COLUMN visibility SET DEFAULT 'team'"
		if !strings.Contains(joined, defaultRepair) {
			t.Fatalf("resource authorization schema missing legacy visibility default repair %q", defaultRepair)
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
	defaultRepair := strings.Index(statements, "ALTER TABLE "+table+" ALTER COLUMN visibility SET DEFAULT 'team'")
	notNull := strings.Index(statements, "ALTER TABLE "+table+" ALTER COLUMN visibility SET NOT NULL")
	add := strings.Index(statements, "ALTER TABLE "+table+" ADD CONSTRAINT "+table+"_visibility_check")
	if drop == -1 || update == -1 || defaultRepair == -1 || notNull == -1 || add == -1 ||
		drop > update || update > defaultRepair || defaultRepair > notNull || notNull > add {
		t.Fatalf(
			"%s visibility migration order is wrong; drop index %d update index %d default index %d not-null index %d add index %d",
			table,
			drop,
			update,
			defaultRepair,
			notNull,
			add,
		)
	}
}

package nopsai

import (
	"strings"
	"testing"
)

func TestConfigRepositorySchemaMigratesScopeTypeConstraint(t *testing.T) {
	joined := strings.Join(configRepositorySchemaStatements, "\n")
	drop := strings.Index(joined, "ALTER TABLE config_repositories DROP CONSTRAINT IF EXISTS config_repositories_scope_type_check")
	add := strings.Index(joined, "ALTER TABLE config_repositories ADD CONSTRAINT config_repositories_scope_type_check CHECK (scope_type IN ('team', 'system'))")
	if drop < 0 {
		t.Fatal("config repository schema does not drop the legacy scope_type check constraint")
	}
	if add < 0 {
		t.Fatal("config repository schema does not recreate the scope_type check constraint with team support")
	}
	if drop > add {
		t.Fatal("config repository scope_type check must be dropped before it is recreated")
	}
}

package nopsai

import (
	"testing"

	"nopsai/config"
)

func TestDatabaseBootstrapBackfillsTeamAuthSubjectsAfterTeamSchema(t *testing.T) {
	steps := databaseBootstrapSteps(&config.Config{})
	positions := map[string]int{}
	for index, step := range steps {
		positions[step.name] = index
	}

	authSchema, hasAuthSchema := positions["auth schema"]
	teamSchema, hasTeamSchema := positions["team schema"]
	teamAuthSubjects, hasTeamAuthSubjects := positions["team auth subjects"]
	if !hasAuthSchema || !hasTeamSchema || !hasTeamAuthSubjects {
		t.Fatalf("bootstrap steps missing auth/team subject reconciliation: %#v", positions)
	}
	if !(authSchema < teamSchema && teamSchema < teamAuthSubjects) {
		t.Fatalf("team auth subject reconciliation order invalid: auth=%d team=%d subjects=%d", authSchema, teamSchema, teamAuthSubjects)
	}
}

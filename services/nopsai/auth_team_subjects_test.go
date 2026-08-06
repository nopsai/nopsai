package nopsai

import (
	"context"
	"testing"
)

func TestEnsureAuthTeamsForNamesCreatesUniqueSubjects(t *testing.T) {
	ctx := context.Background()
	tx := &recordingOIDCTx{}

	created, err := ensureAuthTeamsForNames(ctx, tx, []string{
		" engineering/platform ",
		"/engineering/platform/",
		"platform/prod",
		"",
	}, "Created from NopsAI team sync")
	if err != nil {
		t.Fatalf("ensureAuthTeamsForNames() error = %v", err)
	}
	if created != 2 {
		t.Fatalf("created = %d, want 2", created)
	}
	if !recordedOIDCExecContains(tx.execs, "INSERT INTO auth_teams", "engineering/platform", "Created from NopsAI team sync") {
		t.Fatalf("execs missing engineering/platform auth team insert: %#v", tx.execs)
	}
	if !recordedOIDCExecContains(tx.execs, "INSERT INTO auth_teams", "platform/prod", "Created from NopsAI team sync") {
		t.Fatalf("execs missing platform/prod auth team insert: %#v", tx.execs)
	}
}

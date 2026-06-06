package app

import "testing"

func TestCleanupStepSessionsCallsCleanupForEachSession(t *testing.T) {
	var cleaned []StepSession
	CleanupStepSessions(StepSessionCleanupRequest{
		Sessions: []StepSession{
			{Name: "build", ID: "container-1"},
			{Name: "deploy", ID: "container-2"},
		},
		Reason: "test",
		Cleanup: func(session StepSession) {
			cleaned = append(cleaned, session)
		},
	})

	if len(cleaned) != 2 {
		t.Fatalf("cleaned %d sessions, want 2", len(cleaned))
	}
	if cleaned[0].Name != "build" || cleaned[1].Name != "deploy" {
		t.Fatalf("cleaned sessions = %#v", cleaned)
	}
}

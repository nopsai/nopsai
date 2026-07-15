package nopsai

import "testing"

func TestKnowledgeConnectionIdentifierRoundTrip(t *testing.T) {
	id := buildKnowledgeConnectionIdentifier("platform/security", "notion-main")
	if id != "platform/security/notion-main" {
		t.Fatalf("id = %q", id)
	}
	team, name, err := splitKnowledgeConnectionIdentifier(id)
	if err != nil {
		t.Fatalf("splitKnowledgeConnectionIdentifier() error = %v", err)
	}
	if team != "platform/security" || name != "notion-main" {
		t.Fatalf("split = %q/%q", team, name)
	}
}

func TestKnowledgeConnectionNormalization(t *testing.T) {
	provider, err := normalizeKnowledgeConnectionProvider("external_page")
	if err != nil {
		t.Fatalf("normalize provider: %v", err)
	}
	if provider != knowledgeConnectionProviderWiki {
		t.Fatalf("provider = %q", provider)
	}
	if got := slugifyKnowledgeConnectionName("Data Team Notion!"); got != "data-team-notion" {
		t.Fatalf("slug = %q", got)
	}
	if _, err := normalizeKnowledgeConnectionName("../bad"); err == nil {
		t.Fatal("expected invalid connection name error")
	}
	if _, err := normalizeKnowledgeConnectionTeam(""); err == nil {
		t.Fatal("expected empty team error")
	}
}

func TestKnowledgeSyncAndFailureModeNormalization(t *testing.T) {
	syncMode, err := normalizeKnowledgeSyncMode("before_every_run")
	if err != nil {
		t.Fatalf("normalize sync mode: %v", err)
	}
	if syncMode != knowledgeSyncModeBeforeRun {
		t.Fatalf("sync mode = %q", syncMode)
	}
	failureMode, err := normalizeKnowledgeFailureMode("cached")
	if err != nil {
		t.Fatalf("normalize failure mode: %v", err)
	}
	if failureMode != knowledgeFailureModeUseCached {
		t.Fatalf("failure mode = %q", failureMode)
	}
}

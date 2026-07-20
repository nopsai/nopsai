package models

import "testing"

func TestKnowledgeContextRevisionTracksBlockingSubset(t *testing.T) {
	snapshots := []KnowledgeContextSnapshot{
		{Kind: "policy", Ref: "team/release", Content: "Require evidence."},
		{Kind: "guideline", Ref: "team/style", Content: "Friendly tone."},
	}
	knowledgeRevision := KnowledgeContextRevision(snapshots, false)
	policyRevision := KnowledgeContextRevision(snapshots, true)
	if knowledgeRevision == "" || policyRevision == "" {
		t.Fatalf("revisions should be populated: knowledge=%q policy=%q", knowledgeRevision, policyRevision)
	}

	changedGuideline := []KnowledgeContextSnapshot{
		{Kind: "policy", Ref: "team/release", Content: "Require evidence."},
		{Kind: "guideline", Ref: "team/style", Content: "Concise tone."},
	}
	if got := KnowledgeContextRevision(changedGuideline, false); got == knowledgeRevision {
		t.Fatal("knowledge revision did not change after guideline content changed")
	}
	if got := KnowledgeContextRevision(changedGuideline, true); got != policyRevision {
		t.Fatal("policy revision changed after guideline-only content changed")
	}
}

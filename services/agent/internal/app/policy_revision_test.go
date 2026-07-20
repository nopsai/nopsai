package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestPolicyRevisionStateAllowsUnchangedBlockingPolicy(t *testing.T) {
	snapshots := []models.KnowledgeContextSnapshot{
		{Kind: "policy", Ref: "team/release", Content: "Require evidence."},
	}
	state := newPolicyRevisionState(snapshots)
	revision := models.KnowledgeContextRevision(snapshots, true)

	err := state.EnsureCurrent(t.Context(), "run-1", func(context.Context, string) (models.PolicyRevisionResponse, error) {
		return models.PolicyRevisionResponse{
			RunStartPolicyRevision: revision,
			CurrentPolicyRevision:  revision,
			BlockingContextCount:   1,
		}, nil
	}, nil, "goal_resolution")
	if err != nil {
		t.Fatalf("EnsureCurrent() error = %v", err)
	}
}

func TestPolicyRevisionStateFailsClosedOnPolicyChange(t *testing.T) {
	snapshots := []models.KnowledgeContextSnapshot{
		{Kind: "policy", Ref: "team/release", Content: "Require evidence."},
	}
	changed := []models.KnowledgeContextSnapshot{
		{Kind: "policy", Ref: "team/release", Content: "Require approval and evidence."},
	}
	state := newPolicyRevisionState(snapshots)

	err := state.EnsureCurrent(t.Context(), "run-1", func(context.Context, string) (models.PolicyRevisionResponse, error) {
		return models.PolicyRevisionResponse{
			RunStartPolicyRevision: models.KnowledgeContextRevision(snapshots, true),
			CurrentPolicyRevision:  models.KnowledgeContextRevision(changed, true),
			BlockingContextCount:   1,
		}, nil
	}, nil, "action_execution")
	if err == nil || !strings.Contains(err.Error(), "blocking policy revision changed") {
		t.Fatalf("EnsureCurrent() error = %v, want policy change failure", err)
	}
}

func TestPolicyRevisionStateFailsClosedWhenCheckerUnavailable(t *testing.T) {
	state := newPolicyRevisionState([]models.KnowledgeContextSnapshot{
		{Kind: "guardrail", Ref: "team/runtime", Content: "Do not print env vars."},
	})

	err := state.EnsureCurrent(t.Context(), "run-1", func(context.Context, string) (models.PolicyRevisionResponse, error) {
		return models.PolicyRevisionResponse{}, errors.New("service unavailable")
	}, nil, "condition_evaluation")
	if err == nil || !strings.Contains(err.Error(), "service unavailable") {
		t.Fatalf("EnsureCurrent() error = %v, want service failure", err)
	}
}

func TestPolicyRevisionStateSkipsWhenNoBlockingPolicy(t *testing.T) {
	state := newPolicyRevisionState([]models.KnowledgeContextSnapshot{
		{Kind: "guideline", Ref: "team/style", Content: "Use clear prose."},
	})
	called := false
	err := state.EnsureCurrent(t.Context(), "run-1", func(context.Context, string) (models.PolicyRevisionResponse, error) {
		called = true
		return models.PolicyRevisionResponse{}, nil
	}, nil, "goal_resolution")
	if err != nil {
		t.Fatalf("EnsureCurrent() error = %v", err)
	}
	if called {
		t.Fatal("checker was called for non-blocking knowledge context")
	}
}

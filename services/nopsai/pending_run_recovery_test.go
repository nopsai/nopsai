package nopsai

import (
	"testing"
	"time"
)

func TestPendingRunRecoveryLaunchRequestPreservesLaunchContext(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	createdAt := now.Add(-10 * time.Minute)
	timeoutAt := now.Add(20 * time.Minute)

	req, err := pendingRunRecoveryLaunchRequest(pendingRunRecoveryRecord{
		RunID:              "run-1",
		ParentRunID:        "parent-1",
		ParentRunnerID:     "runner-parent",
		ParentHistory:      "parent history",
		PipelineName:       "fallback-name",
		PipelineVersion:    "v9",
		PipelineDefinition: "name: queued pipeline\nversion: v2\nsteps: []\n",
		Scope:              "prod",
		CreatedAt:          createdAt,
		TimeoutAt:          &timeoutAt,
		GitContext: map[string]string{
			"repo_owner":       "acme",
			"repo_name":        "api",
			"check_run_id":     "12345",
			"trigger_event_id": "trigger-1",
		},
		VariableOverrides: map[string]string{"RELEASE_CHANNEL": "nightly"},
	}, now)
	if err != nil {
		t.Fatalf("pendingRunRecoveryLaunchRequest() error = %v", err)
	}

	if !req.RecoveryAttempt {
		t.Fatal("RecoveryAttempt = false, want true")
	}
	if req.RunID != "run-1" || req.ParentRunID != "parent-1" || req.ParentRunnerID != "runner-parent" {
		t.Fatalf("run identity context = %#v", req)
	}
	if req.ParentHistory != "parent history" {
		t.Fatalf("ParentHistory = %q", req.ParentHistory)
	}
	if req.Pipeline.Name != "queued-pipeline" || req.Pipeline.Version != "v2" {
		t.Fatalf("pipeline = %s/%s, want queued-pipeline/v2", req.Pipeline.Name, req.Pipeline.Version)
	}
	if req.Scope != "prod" {
		t.Fatalf("Scope = %q, want prod", req.Scope)
	}
	if req.Timeout != 30*time.Minute {
		t.Fatalf("Timeout = %s, want 30m", req.Timeout)
	}
	if got := req.GitContext["check_run_id"]; got != "12345" {
		t.Fatalf("check_run_id = %q, want 12345", got)
	}
	if got := req.Overrides["RELEASE_CHANNEL"]; got != "nightly" {
		t.Fatalf("override RELEASE_CHANNEL = %q, want nightly", got)
	}
}

func TestPendingRunRecoveryLaunchRequestPreservesOriginalTimeoutAfterQueueWait(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	createdAt := now.Add(-40 * time.Minute)
	timeoutAt := createdAt.Add(30 * time.Minute)

	req, err := pendingRunRecoveryLaunchRequest(pendingRunRecoveryRecord{
		RunID:              "run-expired",
		PipelineDefinition: "name: expired\nsteps: []\n",
		CreatedAt:          createdAt,
		TimeoutAt:          &timeoutAt,
	}, now)
	if err != nil {
		t.Fatalf("pendingRunRecoveryLaunchRequest() error = %v", err)
	}
	if req.Timeout != 30*time.Minute {
		t.Fatalf("Timeout = %s, want original 30m duration", req.Timeout)
	}
}

func TestPendingRunRecoveryGitContextFiltersInvalidCheckRunID(t *testing.T) {
	got := pendingRunRecoveryGitContext(map[string]string{
		"repo_owner":   " acme ",
		"check_run_id": "not-a-number",
	})
	if got["repo_owner"] != "acme" {
		t.Fatalf("repo_owner = %q, want acme", got["repo_owner"])
	}
	if _, ok := got["check_run_id"]; ok {
		t.Fatal("invalid check_run_id should be filtered")
	}
}

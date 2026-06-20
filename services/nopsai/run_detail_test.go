package nopsai

import (
	"testing"
	"time"

	runquery "nopsai/services/nopsai/internal/runs"
)

func TestDeriveRunDetailStepStatusPrefersChildRunProgress(t *testing.T) {
	tasks := []TaskDetail{
		{TaskID: "task-1", StepName: "included", TaskName: "included", Status: "success"},
	}
	childRuns := []RunListItem{
		{RunID: "child-1", ParentStepName: "included", Status: "running"},
	}

	got := runquery.DeriveRunDetailStepStatus(tasks, childRuns)
	if got != "running" {
		t.Fatalf("deriveRunDetailStepStatus() = %q, want %q", got, "running")
	}
}

func TestDeriveRunDetailStepStatusUsesChildTerminalStateWhenPlaceholderLags(t *testing.T) {
	tasks := []TaskDetail{
		{TaskID: "task-1", StepName: "included", TaskName: "included", Status: "pending"},
	}
	childRuns := []RunListItem{
		{RunID: "child-1", ParentStepName: "included", Status: "success", IsComplete: true},
	}

	got := runquery.DeriveRunDetailStepStatus(tasks, childRuns)
	if got != "success" {
		t.Fatalf("deriveRunDetailStepStatus() = %q, want %q", got, "success")
	}
}

func TestDeriveRunDetailStepStatusPreservesCancellation(t *testing.T) {
	tasks := []TaskDetail{
		{TaskID: "task-1", StepName: "deploy", TaskName: "build", Status: "success"},
		{TaskID: "task-2", StepName: "deploy", TaskName: "release", Status: "cancelled"},
	}

	got := runquery.DeriveRunDetailStepStatus(tasks, nil)
	if got != "cancelled" {
		t.Fatalf("deriveRunDetailStepStatus() = %q, want %q", got, "cancelled")
	}
}

func TestFinalizeRunDetailStepStatusMarksStaleRunningStepFailed(t *testing.T) {
	start := time.Unix(1_700_000_000, 0).UTC()
	tasks := []TaskDetail{
		{TaskID: "task-1", StepName: "deploy", TaskName: "release", Status: "running", StartedAt: start},
	}

	got := runquery.FinalizeRunDetailStepStatus("running", tasks, "failure")
	if got != "failure" {
		t.Fatalf("finalizeRunDetailStepStatus() = %q, want %q", got, "failure")
	}
}

func TestFinalizeRunDetailStepStatusMarksPendingStepSkippedOnFailedRun(t *testing.T) {
	got := runquery.FinalizeRunDetailStepStatus("pending", nil, "failure")
	if got != "skipped" {
		t.Fatalf("finalizeRunDetailStepStatus() = %q, want %q", got, "skipped")
	}
}

func TestFinalizeRunDetailStepStatusPreservesTerminalStepStatus(t *testing.T) {
	got := runquery.FinalizeRunDetailStepStatus("failure (ignored)", nil, "success")
	if got != "failure (ignored)" {
		t.Fatalf("finalizeRunDetailStepStatus() = %q, want %q", got, "failure (ignored)")
	}
}

func TestFinalizeRunDetailStepStatusPreservesIgnoredFailureRunStatus(t *testing.T) {
	start := time.Unix(1_700_000_000, 0).UTC()
	tasks := []TaskDetail{
		{TaskID: "task-1", StepName: "test", TaskName: "lint", Status: "running", StartedAt: start},
	}

	got := runquery.FinalizeRunDetailStepStatus("running", tasks, "failure (ignored)")
	if got != "failure (ignored)" {
		t.Fatalf("finalizeRunDetailStepStatus() = %q, want %q", got, "failure (ignored)")
	}
}

func TestBuildRunDetailETagChangesWhenTaskOrChildStatusChanges(t *testing.T) {
	start := time.Unix(1_700_000_000, 0).UTC()
	run := RunListItem{
		RunID:      "run-1",
		Status:     "running",
		StartedAt:  start,
		IsComplete: false,
	}
	exitCodeZero := 0
	baseTasks := map[string][]TaskDetail{
		"prepare": {
			{
				TaskID:     "task-1",
				StepName:   "prepare",
				TaskName:   "prepare",
				Status:     "pending",
				TaskIndex:  1,
				StartedAt:  start,
				ExitCode:   &exitCodeZero,
				FinishedAt: time.Time{},
			},
		},
	}
	baseChildRuns := []RunListItem{
		{
			RunID:          "child-1",
			ParentStepName: "prepare",
			Status:         "pending",
		},
	}

	baseETag := runquery.BuildRunDetailETag(run, baseChildRuns, baseTasks, nil, nil, nil)

	taskUpdated := map[string][]TaskDetail{
		"prepare": {
			{
				TaskID:     "task-1",
				StepName:   "prepare",
				TaskName:   "prepare",
				Status:     "running",
				TaskIndex:  1,
				StartedAt:  start,
				ExitCode:   &exitCodeZero,
				FinishedAt: time.Time{},
			},
		},
	}
	taskETag := runquery.BuildRunDetailETag(run, baseChildRuns, taskUpdated, nil, nil, nil)
	if taskETag == baseETag {
		t.Fatalf("expected task status change to alter ETag, but both were %q", taskETag)
	}

	childUpdated := []RunListItem{
		{
			RunID:          "child-1",
			ParentStepName: "prepare",
			Status:         "running",
			StartedAt:      start,
		},
	}
	childETag := runquery.BuildRunDetailETag(run, childUpdated, baseTasks, nil, nil, nil)
	if childETag == baseETag {
		t.Fatalf("expected child run status change to alter ETag, but both were %q", childETag)
	}
}

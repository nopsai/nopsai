package main

import (
	"testing"
	"time"
)

func TestDeriveRunDetailStepStatusPrefersChildRunProgress(t *testing.T) {
	tasks := []TaskDetail{
		{TaskID: "task-1", StepName: "included", TaskName: "included", Status: "success"},
	}
	childRuns := []RunListItem{
		{RunID: "child-1", ParentStepName: "included", Status: "running"},
	}

	got := deriveRunDetailStepStatus(tasks, childRuns)
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

	got := deriveRunDetailStepStatus(tasks, childRuns)
	if got != "success" {
		t.Fatalf("deriveRunDetailStepStatus() = %q, want %q", got, "success")
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

	baseETag := buildRunDetailETag(run, baseChildRuns, baseTasks)

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
	taskETag := buildRunDetailETag(run, baseChildRuns, taskUpdated)
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
	childETag := buildRunDetailETag(run, childUpdated, baseTasks)
	if childETag == baseETag {
		t.Fatalf("expected child run status change to alter ETag, but both were %q", childETag)
	}
}

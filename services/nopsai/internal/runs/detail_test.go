package runs

import (
	"testing"
	"time"

	"nopsai/pkg/models"
)

func TestDeriveRunDetailStepStatusPrefersChildRunProgress(t *testing.T) {
	tasks := []models.TaskDetail{
		{TaskID: "task-1", StepName: "included", TaskName: "included", Status: "success"},
	}
	childRuns := []models.RunListItem{
		{RunID: "child-1", ParentStepName: "included", Status: "running"},
	}

	got := DeriveRunDetailStepStatus(tasks, childRuns)
	if got != "running" {
		t.Fatalf("DeriveRunDetailStepStatus() = %q, want %q", got, "running")
	}
}

func TestFinalizeRunDetailStepStatusMarksPendingStepSkippedOnFailedRun(t *testing.T) {
	got := FinalizeRunDetailStepStatus("pending", nil, "failure")
	if got != "skipped" {
		t.Fatalf("FinalizeRunDetailStepStatus() = %q, want %q", got, "skipped")
	}
}

func TestBuildRunDetailETagChangesWhenTaskStatusChanges(t *testing.T) {
	start := time.Unix(1_700_000_000, 0).UTC()
	run := models.RunListItem{
		RunID:      "run-1",
		Status:     "running",
		StartedAt:  start,
		IsComplete: false,
	}
	exitCodeZero := 0
	baseTasks := map[string][]models.TaskDetail{
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
	taskUpdated := map[string][]models.TaskDetail{
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

	baseETag := BuildRunDetailETag(run, nil, baseTasks)
	taskETag := BuildRunDetailETag(run, nil, taskUpdated)
	if taskETag == baseETag {
		t.Fatalf("expected task status change to alter ETag, but both were %q", taskETag)
	}
}

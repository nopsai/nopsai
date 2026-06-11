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

func TestDeriveRunDetailStepDurationCapsUnfinishedWorkAtRunFinish(t *testing.T) {
	start := time.Unix(1_700_000_000, 0).UTC()
	finished := start.Add(45 * time.Second)
	tasks := []models.TaskDetail{
		{
			TaskID:    "task-1",
			StepName:  "prepare",
			TaskName:  "clone",
			Status:    "running",
			StartedAt: start,
		},
	}

	got := DeriveRunDetailStepDurationUntil(tasks, nil, finished)
	if got != 45*time.Second {
		t.Fatalf("DeriveRunDetailStepDurationUntil() = %s, want 45s", got)
	}
}

func TestFinalizeRunDetailTasksForDisplaySkipsNeverStartedWorkAndBoundsFailedWork(t *testing.T) {
	start := time.Unix(1_700_000_000, 0).UTC()
	finished := start.Add(30 * time.Second)
	tasks := []models.TaskDetail{
		{
			TaskID:    "task-1",
			StepName:  "prepare",
			TaskName:  "clone",
			Status:    "running",
			StartedAt: start,
		},
		{
			TaskID:   "task-2",
			StepName: "prepare",
			TaskName: "branch",
			Status:   "pending",
		},
	}

	got := FinalizeRunDetailTasksForDisplay(tasks, "failure", "failure", finished)
	if got[0].Status != "failure" {
		t.Fatalf("first task status = %q, want failure", got[0].Status)
	}
	if got[0].FinishedAt != finished {
		t.Fatalf("first task finished_at = %s, want %s", got[0].FinishedAt, finished)
	}
	if got[0].ExitCode == nil || *got[0].ExitCode != 1 {
		t.Fatalf("first task exit code = %v, want 1", got[0].ExitCode)
	}
	if got[1].Status != "skipped" {
		t.Fatalf("second task status = %q, want skipped", got[1].Status)
	}
	if !got[1].FinishedAt.IsZero() {
		t.Fatalf("second task finished_at = %s, want zero", got[1].FinishedAt)
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

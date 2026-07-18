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

func TestApplyChildRunStatusKeepsParentRunningWhileChildRuns(t *testing.T) {
	start := time.Unix(1_700_000_000, 0).UTC()
	finished := start.Add(20 * time.Second)
	parent := models.RunListItem{
		RunID:      "parent-1",
		Status:     "success",
		StartedAt:  start,
		FinishedAt: finished,
		Duration:   "20s",
		IsComplete: true,
	}

	got := ApplyChildRunStatus(parent, []models.RunListItem{{
		RunID:     "child-1",
		Status:    "running",
		StartedAt: start.Add(5 * time.Second),
	}})

	if got.Status != "running" {
		t.Fatalf("status = %q, want running", got.Status)
	}
	if got.IsComplete {
		t.Fatal("parent should not be complete while a child run is active")
	}
	if !got.FinishedAt.IsZero() {
		t.Fatalf("finished_at = %s, want zero while child is active", got.FinishedAt)
	}
}

func TestApplyChildRunStatusFailsSuccessfulParentWhenChildFails(t *testing.T) {
	start := time.Unix(1_700_000_000, 0).UTC()
	parentFinished := start.Add(20 * time.Second)
	childFinished := start.Add(45 * time.Second)
	parent := models.RunListItem{
		RunID:      "parent-1",
		Status:     "success",
		StartedAt:  start,
		FinishedAt: parentFinished,
		Duration:   "20s",
		IsComplete: true,
	}

	got := ApplyChildRunStatus(parent, []models.RunListItem{{
		RunID:      "child-1",
		Status:     "failure",
		StartedAt:  start.Add(5 * time.Second),
		FinishedAt: childFinished,
		IsComplete: true,
	}})

	if got.Status != "failure" {
		t.Fatalf("status = %q, want failure", got.Status)
	}
	if !got.IsComplete {
		t.Fatal("parent should stay complete after terminal child failure")
	}
	if got.FinishedAt != childFinished {
		t.Fatalf("finished_at = %s, want child finish %s", got.FinishedAt, childFinished)
	}
	if got.Duration != "45s" {
		t.Fatalf("duration = %q, want 45s", got.Duration)
	}
}

func TestBuildDetailUsesChildRunStatusForParentAndStep(t *testing.T) {
	start := time.Unix(1_700_000_000, 0).UTC()
	pipeline := models.Pipeline{
		Steps: []models.PipelineStep{{
			Step: &models.IncludeStep{
				BaseStep: models.BaseStep{Name: "included"},
				Include:  "pipeline:child",
			},
		}},
	}
	detail := BuildDetail(DetailBuildInput{
		Run: models.RunListItem{
			RunID:      "parent-1",
			Status:     "success",
			StartedAt:  start,
			FinishedAt: start.Add(10 * time.Second),
			IsComplete: true,
		},
		OriginalPipeline: pipeline,
		ResolvedPipeline: pipeline,
		ChildRuns: []models.RunListItem{{
			RunID:          "child-1",
			ParentStepName: "included",
			Status:         "running",
			StartedAt:      start.Add(5 * time.Second),
		}},
		TasksByStep: map[string][]models.TaskDetail{
			"included": {{
				TaskID:    "task-1",
				StepName:  "included",
				TaskName:  "included",
				Status:    "success",
				StartedAt: start,
			}},
		},
	})

	if detail.RunInfo.Status != "running" {
		t.Fatalf("run status = %q, want running", detail.RunInfo.Status)
	}
	if detail.RunInfo.IsComplete {
		t.Fatal("run_info should be incomplete while child run is active")
	}
	if len(detail.Steps) != 1 || detail.Steps[0].Status != "running" {
		t.Fatalf("step status = %#v, want running", detail.Steps)
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

func TestFinalizeRunDetailTasksForDisplayClosesSuccessfulRunWork(t *testing.T) {
	start := time.Unix(1_700_000_000, 0).UTC()
	finished := start.Add(30 * time.Second)
	tasks := []models.TaskDetail{
		{
			TaskID:    "task-1",
			StepName:  "build",
			TaskName:  "compile",
			Status:    "running",
			StartedAt: start,
		},
		{
			TaskID:   "task-2",
			StepName: "build",
			TaskName: "package",
			Status:   "pending",
		},
	}

	got := FinalizeRunDetailTasksForDisplay(tasks, "success", "success", finished)
	if got[0].Status != "success" || got[1].Status != "success" {
		t.Fatalf("task statuses = %q/%q, want success/success", got[0].Status, got[1].Status)
	}
	if got[0].FinishedAt != finished {
		t.Fatalf("first task finished_at = %s, want %s", got[0].FinishedAt, finished)
	}
}

func TestBuildStepDetailsForRunAttachesAIUsageByStepAndTask(t *testing.T) {
	pipeline := models.Pipeline{
		Steps: []models.PipelineStep{{
			Step: &models.TaskStep{
				BaseStep: models.BaseStep{Name: "plan"},
				Tasks: []models.Task{{
					Name: "summarize",
				}},
			},
		}},
	}
	tasksByStep := map[string][]models.TaskDetail{
		"plan": {{
			TaskID:    "task-1",
			StepName:  "plan",
			TaskName:  "summarize",
			Status:    "success",
			TaskIndex: 1,
		}},
	}

	steps := BuildStepDetailsForRun(
		models.RunListItem{Status: "success", IsComplete: true},
		pipeline,
		pipeline,
		tasksByStep,
		nil,
		map[string]models.AIUsageSummary{"plan": {TotalTokens: 75, PromptTokens: 50, CompletionTokens: 25}},
		map[string]models.AIUsageSummary{taskUsageKey("plan", "summarize"): {TotalTokens: 60}},
	)
	if len(steps) != 1 {
		t.Fatalf("steps len = %d, want 1", len(steps))
	}
	if steps[0].AIUsage.TotalTokens != 75 {
		t.Fatalf("step tokens = %d, want 75", steps[0].AIUsage.TotalTokens)
	}
	if len(steps[0].Tasks) != 1 || steps[0].Tasks[0].AIUsage.TotalTokens != 60 {
		t.Fatalf("task usage = %#v, want 60 tokens", steps[0].Tasks)
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

	baseETag := BuildRunDetailETag(run, nil, baseTasks, nil, nil, nil)
	taskETag := BuildRunDetailETag(run, nil, taskUpdated, nil, nil, nil)
	if taskETag == baseETag {
		t.Fatalf("expected task status change to alter ETag, but both were %q", taskETag)
	}
}

func TestBuildRunDetailETagChangesWhenAIUsageChanges(t *testing.T) {
	run := models.RunListItem{RunID: "run-1", Status: "success", IsComplete: true}
	tasks := map[string][]models.TaskDetail{
		"plan": {{
			TaskID:    "task-1",
			StepName:  "plan",
			TaskName:  "summarize",
			Status:    "success",
			TaskIndex: 1,
		}},
	}

	baseETag := BuildRunDetailETag(run, nil, tasks, map[string]models.AIUsageSummary{"plan": {TotalTokens: 10}}, nil, nil)
	updatedETag := BuildRunDetailETag(run, nil, tasks, map[string]models.AIUsageSummary{"plan": {TotalTokens: 11}}, nil, nil)
	if updatedETag == baseETag {
		t.Fatalf("expected AI usage change to alter ETag, but both were %q", updatedETag)
	}
}

func TestBuildRunDetailETagChangesWhenFinalOutputContentChanges(t *testing.T) {
	run := models.RunListItem{RunID: "run-1", Status: "success", IsComplete: true}
	tasks := map[string][]models.TaskDetail{}

	baseETag := BuildRunDetailETag(run, nil, tasks, nil, nil, []models.PipelineRunFinalOutput{{
		ID:      "output-1",
		Name:    "Executive summary",
		Type:    "markdown",
		Status:  "generating",
		Content: "",
	}})
	updatedETag := BuildRunDetailETag(run, nil, tasks, nil, nil, []models.PipelineRunFinalOutput{{
		ID:      "output-1",
		Name:    "Executive summary",
		Type:    "markdown",
		Status:  "success",
		Content: "Done",
	}})
	if updatedETag == baseETag {
		t.Fatalf("expected final output change to alter ETag, but both were %q", updatedETag)
	}
}

func TestBuildRunDetailETagChangesWhenFinalOutputAuditChanges(t *testing.T) {
	run := models.RunListItem{RunID: "run-1", Status: "success", IsComplete: true}
	tasks := map[string][]models.TaskDetail{}
	base := models.PipelineRunFinalOutput{
		ID:                 "output-1",
		Name:               "Executive summary",
		Type:               "markdown",
		Status:             "success",
		Content:            "Done",
		GenerationAttempts: 1,
	}
	updated := base
	updated.GenerationAttempts = 2
	updated.ContractViolations = 1

	baseETag := BuildRunDetailETag(run, nil, tasks, nil, nil, []models.PipelineRunFinalOutput{base})
	updatedETag := BuildRunDetailETag(run, nil, tasks, nil, nil, []models.PipelineRunFinalOutput{updated})
	if updatedETag == baseETag {
		t.Fatalf("expected final output audit change to alter ETag, but both were %q", updatedETag)
	}
}

func TestBuildRunDetailETagChangesWhenFinalOutputRenderAuditChanges(t *testing.T) {
	run := models.RunListItem{RunID: "run-1", Status: "success", IsComplete: true}
	base := models.PipelineRunFinalOutput{ID: "output-1", Name: "Report", Type: "pdf", Status: "success", Content: "{}"}
	updated := base
	updated.RenderAttempts = 2
	updated.RenderFailures = 1
	baseETag := BuildRunDetailETag(run, nil, nil, nil, nil, []models.PipelineRunFinalOutput{base})
	updatedETag := BuildRunDetailETag(run, nil, nil, nil, nil, []models.PipelineRunFinalOutput{updated})
	if updatedETag == baseETag {
		t.Fatalf("expected render audit change to alter ETag, but both were %q", updatedETag)
	}
}

func TestFinalOutputGenerationTiming(t *testing.T) {
	createdAt := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(2*time.Minute + 30*time.Second)
	duration, seconds := FinalOutputGenerationTiming(createdAt, updatedAt)
	if duration != "2m30s" || seconds != 150 {
		t.Fatalf("FinalOutputGenerationTiming() = %q, %.0f; want 2m30s, 150", duration, seconds)
	}

	duration, seconds = FinalOutputGenerationTiming(updatedAt, createdAt)
	if duration != "" || seconds != 0 {
		t.Fatalf("FinalOutputGenerationTiming(inverted) = %q, %.0f; want empty, 0", duration, seconds)
	}
}

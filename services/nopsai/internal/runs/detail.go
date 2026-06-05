package runs

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"nopsai/pkg/models"
)

type RunRecord struct {
	Run                    models.RunListItem
	PipelineDefinitionYAML string
}

type DetailBuildInput struct {
	Run                    models.RunListItem
	PipelineDefinitionYAML string
	OriginalPipeline       models.Pipeline
	ResolvedPipeline       models.Pipeline
	ChildRuns              []models.RunListItem
	TasksByStep            map[string][]models.TaskDetail
	KnowledgeContexts      []models.KnowledgeContextSnapshot
	ParentRunInfo          *models.ParentRunInfo
}

func LoadRunRecord(ctx context.Context, db Queryer, runID string) (RunRecord, error) {
	row := db.QueryRow(ctx, `
		SELECT
			pr.run_id, pr.pipeline_name, pr.pipeline_path, pr.pipeline_version, pr.status, COALESCE(pr.git_commit_sha, ''),
			COALESCE(pr.git_repo_owner, ''), COALESCE(pr.git_repo_name, ''),
			pr.started_at, pr.finished_at, pr.parent_run_id,
			COALESCE(pr.git_pusher_name, ''), pr.pipeline_definition, COALESCE(pr.git_ref, ''), COALESCE(pr.git_target_ref, ''),
			pr.failure_reason, COALESCE(pr.pipeline_source, ''), COALESCE(pr.trigger_event_id, ''),
			COALESCE(pr.trigger_source, ''), COALESCE(pr.schedule_id::text, ''), COALESCE(ps.name, ''), COALESCE(ps.path, ''),
			COALESCE(eti.trigger_id, ''), COALESCE(et.name, ''), COALESCE(eti.event_type, ''),
			COALESCE(eti.caller_type, ''), COALESCE(eti.caller_id, ''), COALESCE(eti.idempotency_key, '')
		FROM pipeline_runs pr
		LEFT JOIN pipeline_schedules ps ON ps.id = pr.schedule_id
		LEFT JOIN external_trigger_invocations eti ON eti.id::text = pr.trigger_event_id
		LEFT JOIN external_triggers et ON et.id = eti.trigger_id
		WHERE pr.run_id = $1
	`, runID)

	var record RunRecord
	var startedAt, finishedAt sql.NullTime
	var commitSHA, repoOwner, repoName, pusherName, gitRef, gitTargetRef, failureReason, pipelineSource, pipelineVersion, pipelinePath, triggerEventID, triggerSource, scheduleID, scheduleName, schedulePath sql.NullString
	var externalTriggerID, externalTriggerName, externalTriggerEventType, externalTriggerCallerType, externalTriggerCallerID, externalTriggerIdempotency sql.NullString
	err := row.Scan(
		&record.Run.RunID, &record.Run.PipelineName, &pipelinePath, &pipelineVersion, &record.Run.Status, &commitSHA,
		&repoOwner, &repoName, &startedAt, &finishedAt,
		&record.Run.ParentRunID, &pusherName, &record.PipelineDefinitionYAML, &gitRef, &gitTargetRef,
		&failureReason, &pipelineSource, &triggerEventID,
		&triggerSource, &scheduleID, &scheduleName, &schedulePath, &externalTriggerID, &externalTriggerName,
		&externalTriggerEventType, &externalTriggerCallerType, &externalTriggerCallerID, &externalTriggerIdempotency,
	)
	if err != nil {
		return RunRecord{}, err
	}

	run := &record.Run
	run.PipelinePath = pipelinePath.String
	run.GitCommitSHA = commitSHA.String
	run.GitRepoOwner = repoOwner.String
	run.GitRepoName = repoName.String
	run.GitPusherName = pusherName.String
	run.GitRef = gitRef.String
	run.GitTargetRef = gitTargetRef.String
	run.FailureReason = failureReason.String
	run.PipelineSource = pipelineSource.String
	run.PipelineVersion = NormalizePipelineVersion(pipelineVersion.String)
	run.TriggerEventID = triggerEventID.String
	run.TriggerSource = triggerSource.String
	run.ScheduleID = scheduleID.String
	run.ScheduleName = scheduleName.String
	run.SchedulePath = schedulePath.String
	run.ExternalTriggerID = externalTriggerID.String
	run.ExternalTriggerName = externalTriggerName.String
	run.ExternalTriggerEventType = externalTriggerEventType.String
	run.ExternalTriggerCallerType = externalTriggerCallerType.String
	run.ExternalTriggerCallerID = externalTriggerCallerID.String
	run.ExternalTriggerIdempotency = externalTriggerIdempotency.String
	if startedAt.Valid {
		run.StartedAt = startedAt.Time
		if finishedAt.Valid {
			run.FinishedAt = finishedAt.Time
			run.Duration = run.FinishedAt.Sub(run.StartedAt).Round(time.Second).String()
			run.IsComplete = true
		} else {
			run.Duration = time.Since(run.StartedAt).Round(time.Second).String()
			run.IsComplete = IsTerminalRunStatus(run.Status)
		}
	} else {
		run.Duration = "0s"
		run.IsComplete = IsTerminalRunStatus(run.Status)
	}

	return record, nil
}

func LoadParentRunInfo(ctx context.Context, db Queryer, parentRunID *string) (*models.ParentRunInfo, error) {
	if parentRunID == nil || strings.TrimSpace(*parentRunID) == "" {
		return nil, nil
	}

	var parentPipelineName, parentPipelineVersion, parentPipelinePath string
	err := db.QueryRow(ctx, `
		SELECT pipeline_name, pipeline_path, pipeline_version FROM pipeline_runs WHERE run_id = $1
	`, *parentRunID).Scan(&parentPipelineName, &parentPipelinePath, &parentPipelineVersion)
	if err != nil {
		return nil, err
	}
	return &models.ParentRunInfo{
		RunID:           *parentRunID,
		PipelineName:    parentPipelineName,
		PipelinePath:    parentPipelinePath,
		PipelineVersion: NormalizePipelineVersion(parentPipelineVersion),
	}, nil
}

func LoadChildRuns(ctx context.Context, db Queryer, runID string) ([]models.RunListItem, error) {
	rows, err := db.Query(ctx, `
		SELECT run_id, pipeline_name, pipeline_path, pipeline_version, status, started_at, finished_at, parent_step_name, COALESCE(trigger_event_id, '')
		FROM pipeline_runs
		WHERE parent_run_id = $1
		ORDER BY created_at ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	childRuns := make([]models.RunListItem, 0)
	for rows.Next() {
		var childRun models.RunListItem
		var childStartedAt, childFinishedAt sql.NullTime
		var parentStepName, childPipelineVersion, childPipelinePath, childTriggerEventID sql.NullString
		if err := rows.Scan(&childRun.RunID, &childRun.PipelineName, &childPipelinePath, &childPipelineVersion, &childRun.Status, &childStartedAt, &childFinishedAt, &parentStepName, &childTriggerEventID); err != nil {
			return nil, err
		}
		childRun.PipelinePath = childPipelinePath.String
		childRun.PipelineVersion = NormalizePipelineVersion(childPipelineVersion.String)
		childRun.TriggerEventID = childTriggerEventID.String
		if childStartedAt.Valid {
			childRun.StartedAt = childStartedAt.Time
		}
		if childFinishedAt.Valid {
			childRun.FinishedAt = childFinishedAt.Time
			childRun.Duration = childRun.FinishedAt.Sub(childRun.StartedAt).Round(time.Second).String()
			childRun.IsComplete = true
		} else if childStartedAt.Valid {
			childRun.Duration = time.Since(childRun.StartedAt).Round(time.Second).String()
		}
		childRun.ParentStepName = parentStepName.String
		childRuns = append(childRuns, childRun)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return childRuns, nil
}

func LoadTaskDetailsByStep(ctx context.Context, db Queryer, runID string) (map[string][]models.TaskDetail, error) {
	rows, err := db.Query(ctx, `
		SELECT task_id, step_name, task_name, status, exit_code, started_at, finished_at, task_index
		FROM task_runs
		WHERE run_id = $1
		ORDER BY task_index ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasksByStep := make(map[string][]models.TaskDetail)
	for rows.Next() {
		var task models.TaskDetail
		var startedAt, finishedAt sql.NullTime
		if err := rows.Scan(&task.TaskID, &task.StepName, &task.TaskName, &task.Status, &task.ExitCode, &startedAt, &finishedAt, &task.TaskIndex); err != nil {
			return nil, err
		}
		if startedAt.Valid {
			task.StartedAt = startedAt.Time
		}
		if finishedAt.Valid {
			task.FinishedAt = finishedAt.Time
		}
		tasksByStep[task.StepName] = append(tasksByStep[task.StepName], task)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasksByStep, nil
}

func BuildDetail(input DetailBuildInput) models.RunDetail {
	return models.RunDetail{
		RunInfo:                input.Run,
		Steps:                  BuildStepDetailsForRun(input.Run.Status, input.OriginalPipeline, input.ResolvedPipeline, input.TasksByStep, input.ChildRuns),
		PipelineDefinition:     input.OriginalPipeline,
		PipelineDefinitionYAML: input.PipelineDefinitionYAML,
		KnowledgeContexts:      input.KnowledgeContexts,
		ChildRuns:              input.ChildRuns,
		ParentRunInfo:          input.ParentRunInfo,
	}
}

func BuildStepDetailsForRun(runStatus string, originalPipeline, resolvedPipeline models.Pipeline, tasksByStep map[string][]models.TaskDetail, childRuns []models.RunListItem) []models.StepDetail {
	childRunsByStep := make(map[string][]models.RunListItem)
	for _, childRun := range childRuns {
		if childRun.ParentStepName == "" {
			continue
		}
		childRunsByStep[childRun.ParentStepName] = append(childRunsByStep[childRun.ParentStepName], childRun)
	}

	steps := make([]models.StepDetail, 0, len(resolvedPipeline.Steps))
	for _, pStep := range resolvedPipeline.Steps {
		stepName := pStep.GetName()
		stepTasks := tasksByStep[stepName]
		stepChildRuns := childRunsByStep[stepName]
		status := FinalizeRunDetailStepStatus(DeriveRunDetailStepStatus(stepTasks, stepChildRuns), stepTasks, runStatus)
		stepDuration := DeriveRunDetailStepDuration(stepTasks, stepChildRuns)

		originalPStep, _ := FindStepByName(originalPipeline.Steps, stepName)
		config := models.StepConfiguration{
			Image:            pStep.GetImage(),
			Include:          originalPStep.GetInclude(),
			Sync:             pStep.GetSync(),
			Approval:         pStep.GetApproval(),
			Secrets:          pStep.GetSecrets(),
			Volumes:          pStep.GetVolumes(),
			Variables:        pStep.GetVariables(),
			IgnoreFailure:    pStep.GetIgnoreFailure(),
			LlmOutputSharing: pStep.GetLlmOutputSharing(),
			LLMProfile:       pStep.GetLLMProfile(),
			MCPProfiles:      pStep.GetMCPProfiles(),
			RuntimePool:      pStep.GetRuntimePool(),
			KnowledgeContext: pStep.GetKnowledgeContext(),
			Tasks:            pStep.GetTasks(),
		}
		steps = append(steps, models.StepDetail{
			Name:          stepName,
			Status:        status,
			DependsOn:     pStep.GetDependsOn(),
			Tasks:         stepTasks,
			Duration:      stepDuration.Round(time.Second).String(),
			Configuration: config,
		})
	}
	return steps
}

func BuildRunDetailETag(run models.RunListItem, childRuns []models.RunListItem, tasksByStep map[string][]models.TaskDetail) string {
	hasher := sha256.New()
	fmt.Fprintf(
		hasher,
		"run|%s|%s|%t|%d|%d|%s|%s|%s|",
		run.RunID,
		NormalizeRunDetailStatus(run.Status),
		run.IsComplete,
		run.StartedAt.UnixNano(),
		run.FinishedAt.UnixNano(),
		strings.TrimSpace(run.FailureReason),
		strings.TrimSpace(run.PipelineSource),
		strings.TrimSpace(run.TriggerEventID),
	)

	for _, childRun := range childRuns {
		fmt.Fprintf(
			hasher,
			"child|%s|%s|%t|%d|%d|%s|",
			childRun.RunID,
			NormalizeRunDetailStatus(childRun.Status),
			childRun.IsComplete,
			childRun.StartedAt.UnixNano(),
			childRun.FinishedAt.UnixNano(),
			strings.TrimSpace(childRun.ParentStepName),
		)
	}

	stepNames := make([]string, 0, len(tasksByStep))
	for stepName := range tasksByStep {
		stepNames = append(stepNames, stepName)
	}
	sort.Strings(stepNames)

	for _, stepName := range stepNames {
		fmt.Fprintf(hasher, "step|%s|", stepName)
		for _, task := range tasksByStep[stepName] {
			exitCode := ""
			if task.ExitCode != nil {
				exitCode = strconv.Itoa(*task.ExitCode)
			}
			fmt.Fprintf(
				hasher,
				"task|%s|%s|%s|%s|%d|%d|%d|",
				task.TaskID,
				task.StepName,
				task.TaskName,
				NormalizeRunDetailStatus(task.Status),
				task.TaskIndex,
				task.StartedAt.UnixNano(),
				task.FinishedAt.UnixNano(),
			)
			fmt.Fprintf(hasher, "exit|%s|", exitCode)
		}
	}

	return fmt.Sprintf(`W/"%x"`, hasher.Sum(nil))
}

func NormalizeRunDetailStatus(status string) string {
	raw := strings.ToLower(strings.TrimSpace(status))
	switch {
	case raw == "":
		return "pending"
	case raw == "success" || raw == "running" || raw == "pending" || raw == "skipped" || raw == "cancelled" || raw == "waiting_approval" || raw == "rejected":
		return raw
	case raw == "failure" || strings.Contains(raw, "not_found") || strings.Contains(raw, "timeout"):
		return "failure"
	case strings.Contains(raw, "ignored"):
		return "failure (ignored)"
	case strings.Contains(raw, "fail") || strings.Contains(raw, "error"):
		return "failure"
	default:
		return raw
	}
}

func SummarizeRunDetailStatuses(statuses []string) string {
	if len(statuses) == 0 {
		return "pending"
	}
	priority := map[string]int{
		"failure":           0,
		"rejected":          1,
		"failure (ignored)": 2,
		"cancelled":         3,
		"waiting_approval":  4,
		"running":           5,
		"pending":           6,
		"skipped":           7,
		"success":           8,
	}
	best := "pending"
	bestPriority := len(priority) + 1
	for _, status := range statuses {
		normalized := NormalizeRunDetailStatus(status)
		rank, ok := priority[normalized]
		if !ok {
			normalized = "failure"
			rank = priority[normalized]
		}
		if rank < bestPriority {
			best = normalized
			bestPriority = rank
		}
	}
	return best
}

func DeriveRunDetailStepStatus(tasks []models.TaskDetail, childRuns []models.RunListItem) string {
	taskStatuses := make([]string, 0, len(tasks))
	for _, task := range tasks {
		taskStatuses = append(taskStatuses, task.Status)
	}
	taskStatus := SummarizeRunDetailStatuses(taskStatuses)

	if len(childRuns) == 0 {
		return taskStatus
	}

	childStatuses := make([]string, 0, len(childRuns))
	for _, childRun := range childRuns {
		childStatuses = append(childStatuses, childRun.Status)
	}
	childStatus := SummarizeRunDetailStatuses(childStatuses)

	if taskStatus == "failure" || taskStatus == "failure (ignored)" || taskStatus == "cancelled" {
		return SummarizeRunDetailStatuses([]string{taskStatus, childStatus})
	}
	if childStatus != "pending" {
		return childStatus
	}
	return SummarizeRunDetailStatuses([]string{taskStatus, childStatus})
}

func FinalizeRunDetailStepStatus(stepStatus string, tasks []models.TaskDetail, runStatus string) string {
	normalizedStep := NormalizeRunDetailStatus(stepStatus)
	normalizedRun := NormalizeRunDetailStatus(runStatus)
	if normalizedStep != "running" && normalizedStep != "pending" {
		return normalizedStep
	}

	switch normalizedRun {
	case "success":
		return "success"
	case "failure", "failure (ignored)":
		if normalizedStep == "running" || HasInFlightRunDetailTask(tasks) {
			return normalizedRun
		}
		return "skipped"
	case "rejected":
		if normalizedStep == "waiting_approval" || normalizedStep == "running" || HasInFlightRunDetailTask(tasks) {
			return "rejected"
		}
		return "skipped"
	case "waiting_approval":
		return normalizedStep
	case "cancelled":
		if normalizedStep == "running" || HasInFlightRunDetailTask(tasks) {
			return "cancelled"
		}
		return "skipped"
	default:
		return normalizedStep
	}
}

func HasInFlightRunDetailTask(tasks []models.TaskDetail) bool {
	for _, task := range tasks {
		if !task.StartedAt.IsZero() && task.FinishedAt.IsZero() {
			return true
		}
	}
	return false
}

func DeriveRunDetailStepDuration(tasks []models.TaskDetail, childRuns []models.RunListItem) time.Duration {
	var earliestStart time.Time
	var latestFinish time.Time
	hasActiveWork := false

	for _, task := range tasks {
		if !task.StartedAt.IsZero() && (earliestStart.IsZero() || task.StartedAt.Before(earliestStart)) {
			earliestStart = task.StartedAt
		}
		if !task.FinishedAt.IsZero() && task.FinishedAt.After(latestFinish) {
			latestFinish = task.FinishedAt
		}
		if !task.StartedAt.IsZero() && task.FinishedAt.IsZero() {
			hasActiveWork = true
		}
	}

	for _, childRun := range childRuns {
		if !childRun.StartedAt.IsZero() && (earliestStart.IsZero() || childRun.StartedAt.Before(earliestStart)) {
			earliestStart = childRun.StartedAt
		}
		if !childRun.FinishedAt.IsZero() && childRun.FinishedAt.After(latestFinish) {
			latestFinish = childRun.FinishedAt
		}
		if !childRun.StartedAt.IsZero() && childRun.FinishedAt.IsZero() {
			hasActiveWork = true
		}
	}

	if earliestStart.IsZero() {
		return 0
	}
	if !latestFinish.IsZero() && !hasActiveWork {
		return latestFinish.Sub(earliestStart)
	}
	return time.Since(earliestStart)
}

func AllTasksDone(tasks []models.TaskDetail) bool {
	if len(tasks) == 0 {
		return false
	}
	for _, t := range tasks {
		if t.Status != "success" && !strings.Contains(t.Status, "ignore") {
			return false
		}
	}
	return true
}

func IsTerminalRunStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "failure", "failure (ignored)", "cancelled", "timed_out", "rejected":
		return true
	default:
		return false
	}
}

func FindStepByName(steps []models.PipelineStep, name string) (models.PipelineStep, bool) {
	for _, step := range steps {
		if step.GetName() == name {
			return step, true
		}
	}
	return models.PipelineStep{}, false
}

func IsNotFound(err error) bool {
	return err == pgx.ErrNoRows
}

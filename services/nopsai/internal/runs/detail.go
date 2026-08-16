package runs

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
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
	StepAIUsage            map[string]models.AIUsageSummary
	TaskAIUsage            map[string]models.AIUsageSummary
	KnowledgeContexts      []models.KnowledgeContextSnapshot
	FinalOutputs           []models.PipelineRunFinalOutput
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
			COALESCE(eti.caller_type, ''), COALESCE(eti.caller_id, ''), COALESCE(eti.idempotency_key, ''),
			pr.created_at, pr.timeout_at, COALESCE(pr.scope, ''), COALESCE(pr.team_id, 0)::int,
			COALESCE(pr.git_clone_url, ''), COALESCE(pr.git_ssh_url, ''), COALESCE(pr.git_commit_url, ''),
			COALESCE(pr.git_commit_message, ''), COALESCE(pr.git_commit_author_name, ''),
			COALESCE(pr.git_commit_author_email, ''), COALESCE(pr.git_commit_author_username, ''),
			COALESCE(pr.git_pusher_email, ''), COALESCE(pr.git_check_run_id, 0)::bigint,
			COALESCE(pr.requested_by_type, ''), COALESCE(pr.requested_by_id, ''),
			COALESCE(pr.effective_subject_type, ''), COALESCE(pr.effective_subject_id, ''),
			COALESCE(pr.runtime_variable_overrides::text, '{}'),
			COALESCE(prus.ai_prompt_tokens, 0)::bigint, COALESCE(prus.ai_completion_tokens, 0)::bigint,
			COALESCE(prus.ai_total_tokens, 0)::bigint, COALESCE(prus.ai_cost_usd, 0)::float8
		FROM pipeline_runs pr
		LEFT JOIN pipeline_schedules ps ON ps.id = pr.schedule_id
		LEFT JOIN external_trigger_invocations eti ON eti.id::text = pr.trigger_event_id
		LEFT JOIN external_triggers et ON et.id = eti.trigger_id
		LEFT JOIN pipeline_run_usage_summary prus ON prus.run_id = pr.run_id
		WHERE pr.run_id = $1
	`, runID)

	var record RunRecord
	var startedAt, finishedAt, timeoutAt sql.NullTime
	var commitSHA, repoOwner, repoName, pusherName, gitRef, gitTargetRef, failureReason, pipelineSource, pipelineVersion, pipelinePath, triggerEventID, triggerSource, scheduleID, scheduleName, schedulePath sql.NullString
	var externalTriggerID, externalTriggerName, externalTriggerEventType, externalTriggerCallerType, externalTriggerCallerID, externalTriggerIdempotency sql.NullString
	var scope, gitCloneURL, gitSSHURL, gitCommitURL, gitCommitMessage, gitCommitAuthorName, gitCommitAuthorEmail, gitCommitAuthorUsername, gitPusherEmail sql.NullString
	var requestedByType, requestedByID, effectiveSubjectType, effectiveSubjectID, runtimeVariableOverridesRaw sql.NullString
	err := row.Scan(
		&record.Run.RunID, &record.Run.PipelineName, &pipelinePath, &pipelineVersion, &record.Run.Status, &commitSHA,
		&repoOwner, &repoName, &startedAt, &finishedAt,
		&record.Run.ParentRunID, &pusherName, &record.PipelineDefinitionYAML, &gitRef, &gitTargetRef,
		&failureReason, &pipelineSource, &triggerEventID,
		&triggerSource, &scheduleID, &scheduleName, &schedulePath, &externalTriggerID, &externalTriggerName,
		&externalTriggerEventType, &externalTriggerCallerType, &externalTriggerCallerID, &externalTriggerIdempotency,
		&record.Run.CreatedAt, &timeoutAt, &scope, &record.Run.TeamID,
		&gitCloneURL, &gitSSHURL, &gitCommitURL, &gitCommitMessage, &gitCommitAuthorName,
		&gitCommitAuthorEmail, &gitCommitAuthorUsername, &gitPusherEmail, &record.Run.GitCheckRunID,
		&requestedByType, &requestedByID, &effectiveSubjectType, &effectiveSubjectID, &runtimeVariableOverridesRaw,
		&record.Run.AIUsage.PromptTokens, &record.Run.AIUsage.CompletionTokens, &record.Run.AIUsage.TotalTokens, &record.Run.AIUsage.TotalCostUSD,
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
	run.Scope = scope.String
	run.GitCloneURL = gitCloneURL.String
	run.GitSSHURL = gitSSHURL.String
	run.GitCommitURL = gitCommitURL.String
	run.GitCommitMessage = gitCommitMessage.String
	run.GitCommitAuthorName = gitCommitAuthorName.String
	run.GitCommitAuthorEmail = gitCommitAuthorEmail.String
	run.GitCommitAuthorUsername = gitCommitAuthorUsername.String
	run.GitPusherEmail = gitPusherEmail.String
	run.RequestedByType = requestedByType.String
	run.RequestedByID = requestedByID.String
	run.EffectiveSubjectType = effectiveSubjectType.String
	run.EffectiveSubjectID = effectiveSubjectID.String
	run.RuntimeVariableOverrides = parseRuntimeVariableOverrides(runtimeVariableOverridesRaw.String)
	if timeoutAt.Valid {
		run.TimeoutAt = timeoutAt.Time
	}
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
		SELECT pr.run_id, pr.pipeline_name, pr.pipeline_path, pr.pipeline_version, pr.status, pr.started_at, pr.finished_at,
		       pr.parent_step_name, COALESCE(pr.trigger_event_id, ''),
		       COALESCE(prus.ai_prompt_tokens, 0)::bigint, COALESCE(prus.ai_completion_tokens, 0)::bigint,
		       COALESCE(prus.ai_total_tokens, 0)::bigint, COALESCE(prus.ai_cost_usd, 0)::float8
		FROM pipeline_runs pr
		LEFT JOIN pipeline_run_usage_summary prus ON prus.run_id = pr.run_id
		WHERE pr.parent_run_id = $1
		ORDER BY pr.created_at ASC
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
		if err := rows.Scan(
			&childRun.RunID, &childRun.PipelineName, &childPipelinePath, &childPipelineVersion, &childRun.Status, &childStartedAt, &childFinishedAt,
			&parentStepName, &childTriggerEventID,
			&childRun.AIUsage.PromptTokens, &childRun.AIUsage.CompletionTokens, &childRun.AIUsage.TotalTokens, &childRun.AIUsage.TotalCostUSD,
		); err != nil {
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

func LoadAIUsageByStep(ctx context.Context, db Queryer, runID string) (map[string]models.AIUsageSummary, error) {
	rows, err := db.Query(ctx, `
		SELECT COALESCE(step_name, ''),
		       COALESCE(SUM(prompt_tokens), 0)::bigint,
		       COALESCE(SUM(completion_tokens), 0)::bigint,
		       COALESCE(SUM(total_tokens), 0)::bigint,
		       COALESCE(SUM(total_cost_usd), 0)::float8
		FROM ai_usage_events
		WHERE run_id = $1
		GROUP BY COALESCE(step_name, '')
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	usageByStep := make(map[string]models.AIUsageSummary)
	for rows.Next() {
		var stepName string
		var usage models.AIUsageSummary
		if err := rows.Scan(&stepName, &usage.PromptTokens, &usage.CompletionTokens, &usage.TotalTokens, &usage.TotalCostUSD); err != nil {
			return nil, err
		}
		usageByStep[stepName] = usage
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return usageByStep, nil
}

func LoadAIUsageByTask(ctx context.Context, db Queryer, runID string) (map[string]models.AIUsageSummary, error) {
	rows, err := db.Query(ctx, `
		SELECT COALESCE(step_name, ''), COALESCE(task_name, ''),
		       COALESCE(SUM(prompt_tokens), 0)::bigint,
		       COALESCE(SUM(completion_tokens), 0)::bigint,
		       COALESCE(SUM(total_tokens), 0)::bigint,
		       COALESCE(SUM(total_cost_usd), 0)::float8
		FROM ai_usage_events
		WHERE run_id = $1
		  AND COALESCE(task_name, '') <> ''
		GROUP BY COALESCE(step_name, ''), COALESCE(task_name, '')
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	usageByTask := make(map[string]models.AIUsageSummary)
	for rows.Next() {
		var stepName, taskName string
		var usage models.AIUsageSummary
		if err := rows.Scan(&stepName, &taskName, &usage.PromptTokens, &usage.CompletionTokens, &usage.TotalTokens, &usage.TotalCostUSD); err != nil {
			return nil, err
		}
		usageByTask[taskUsageKey(stepName, taskName)] = usage
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return usageByTask, nil
}

func LoadFinalOutputs(ctx context.Context, db Queryer, runID string) ([]models.PipelineRunFinalOutput, error) {
	rows, err := db.Query(ctx, `
		SELECT id::text, name, type, status, content, error, model,
		       generation_attempts, contract_violations, render_attempts, render_failures,
		       created_at, generation_started_at, updated_at, COALESCE(dashboard_target::text, '{}')
		FROM pipeline_run_outputs
		WHERE run_id = $1
		ORDER BY item_index ASC, created_at ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	outputs := []models.PipelineRunFinalOutput{}
	for rows.Next() {
		var output models.PipelineRunFinalOutput
		var dashboardTargetRaw string
		var generationStartedAt sql.NullTime
		if err := rows.Scan(
			&output.ID,
			&output.Name,
			&output.Type,
			&output.Status,
			&output.Content,
			&output.Error,
			&output.LLMProfile,
			&output.GenerationAttempts,
			&output.ContractViolations,
			&output.RenderAttempts,
			&output.RenderFailures,
			&output.CreatedAt,
			&generationStartedAt,
			&output.UpdatedAt,
			&dashboardTargetRaw,
		); err != nil {
			return nil, err
		}
		output.GenerationStartedAt = nullTimePtr(generationStartedAt)
		output.DashboardTarget = parseFinalOutputDashboardTarget(dashboardTargetRaw)
		outputs = append(outputs, output)
		output.GenerationDuration, output.GenerationSeconds = FinalOutputGenerationTiming(output.GenerationStartedAt, output.UpdatedAt)
		outputs[len(outputs)-1] = output
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return outputs, nil
}

func parseFinalOutputDashboardTarget(raw string) *models.DashboardOutputTarget {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return nil
	}
	var target models.DashboardOutputTarget
	if err := json.Unmarshal([]byte(raw), &target); err != nil {
		return nil
	}
	if strings.TrimSpace(target.Ref) == "" &&
		strings.TrimSpace(target.Section) == "" &&
		strings.TrimSpace(target.EntryKey) == "" &&
		strings.TrimSpace(target.Mode) == "" &&
		strings.TrimSpace(target.Preset) == "" &&
		strings.TrimSpace(target.TTL) == "" {
		return nil
	}
	return &target
}

func parseRuntimeVariableOverrides(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return nil
	}
	var overrides map[string]any
	if err := json.Unmarshal([]byte(raw), &overrides); err != nil || len(overrides) == 0 {
		return nil
	}
	return overrides
}

func FinalOutputGenerationTiming(startedAt *time.Time, updatedAt time.Time) (string, float64) {
	if startedAt == nil || startedAt.IsZero() || updatedAt.IsZero() || updatedAt.Before(*startedAt) {
		return "", 0
	}
	duration := updatedAt.Sub(*startedAt).Round(time.Second)
	return duration.String(), duration.Seconds()
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}

func BuildDetail(input DetailBuildInput) models.RunDetail {
	run := ApplyChildRunStatus(input.Run, input.ChildRuns)
	return models.RunDetail{
		RunInfo:                run,
		Steps:                  BuildStepDetailsForRun(run, input.OriginalPipeline, input.ResolvedPipeline, input.TasksByStep, input.ChildRuns, input.StepAIUsage, input.TaskAIUsage),
		PipelineDefinition:     input.OriginalPipeline,
		PipelineDefinitionYAML: input.PipelineDefinitionYAML,
		KnowledgeContexts:      input.KnowledgeContexts,
		FinalOutputs:           input.FinalOutputs,
		ChildRuns:              input.ChildRuns,
		ParentRunInfo:          input.ParentRunInfo,
	}
}

func ApplyChildRunStatus(run models.RunListItem, childRuns []models.RunListItem) models.RunListItem {
	if len(childRuns) == 0 {
		run.Status = NormalizeRunDetailStatus(run.Status)
		return run
	}

	statuses := make([]string, 0, len(childRuns)+1)
	statuses = append(statuses, run.Status)
	for _, childRun := range childRuns {
		statuses = append(statuses, childRun.Status)
	}
	effectiveStatus := SummarizeRunDetailStatuses(statuses)
	run.Status = effectiveStatus

	if IsTerminalRunStatus(effectiveStatus) {
		run.IsComplete = true
		if latestChildFinish := latestFinishedAt(childRuns); !latestChildFinish.IsZero() && latestChildFinish.After(run.FinishedAt) {
			run.FinishedAt = latestChildFinish
		}
		if !run.StartedAt.IsZero() && !run.FinishedAt.IsZero() {
			run.Duration = run.FinishedAt.Sub(run.StartedAt).Round(time.Second).String()
		}
		return run
	}

	run.IsComplete = false
	run.FinishedAt = time.Time{}
	if !run.StartedAt.IsZero() {
		run.Duration = time.Since(run.StartedAt).Round(time.Second).String()
	}
	return run
}

func latestFinishedAt(runs []models.RunListItem) time.Time {
	var latest time.Time
	for _, run := range runs {
		if run.FinishedAt.After(latest) {
			latest = run.FinishedAt
		}
	}
	return latest
}

func BuildStepDetailsForRun(run models.RunListItem, originalPipeline, resolvedPipeline models.Pipeline, tasksByStep map[string][]models.TaskDetail, childRuns []models.RunListItem, stepAIUsage map[string]models.AIUsageSummary, taskAIUsage map[string]models.AIUsageSummary) []models.StepDetail {
	childRunsByStep := make(map[string][]models.RunListItem)
	for _, childRun := range childRuns {
		if childRun.ParentStepName == "" {
			continue
		}
		childRunsByStep[childRun.ParentStepName] = append(childRunsByStep[childRun.ParentStepName], childRun)
	}

	runStatus := run.Status
	runFinishedAt := time.Time{}
	if IsTerminalRunStatus(runStatus) && !run.FinishedAt.IsZero() {
		runFinishedAt = run.FinishedAt
	}

	steps := make([]models.StepDetail, 0, len(resolvedPipeline.Steps))
	for _, pStep := range resolvedPipeline.Steps {
		stepName := pStep.GetName()
		rawStepTasks := tasksByStep[stepName]
		stepChildRuns := childRunsByStep[stepName]
		status := FinalizeRunDetailStepStatus(DeriveRunDetailStepStatus(rawStepTasks, stepChildRuns), rawStepTasks, runStatus)
		stepTasks := FinalizeRunDetailTasksForDisplay(rawStepTasks, runStatus, status, runFinishedAt)
		for i := range stepTasks {
			stepTasks[i].AIUsage = taskAIUsage[taskUsageKey(stepTasks[i].StepName, stepTasks[i].TaskName)]
		}
		stepDuration := DeriveRunDetailStepDurationUntil(stepTasks, stepChildRuns, runFinishedAt)

		originalPStep, _ := FindStepByName(originalPipeline.Steps, stepName)
		config := models.StepConfiguration{
			Image:            pStep.GetImage(),
			Include:          originalPStep.GetInclude(),
			Sync:             pStep.GetSync(),
			Approval:         pStep.GetApproval(),
			Secrets:          pStep.GetSecrets(),
			Volumes:          pStep.GetVolumes(),
			Variables:        pStep.GetVariables(),
			Outputs:          pStep.GetOutputs(),
			IgnoreFailure:    pStep.GetIgnoreFailure(),
			AgentProfile:     pStep.GetAgentProfile(),
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
			AIUsage:       stepAIUsage[stepName],
		})
	}
	return steps
}

func taskUsageKey(stepName, taskName string) string {
	return strings.TrimSpace(stepName) + "\x00" + strings.TrimSpace(taskName)
}

func FinalizeRunDetailTasksForDisplay(tasks []models.TaskDetail, runStatus, stepStatus string, runFinishedAt time.Time) []models.TaskDetail {
	normalizedRun := NormalizeRunDetailStatus(runStatus)
	if !IsTerminalRunStatus(normalizedRun) {
		return tasks
	}

	normalizedStep := NormalizeRunDetailStatus(stepStatus)
	finalized := make([]models.TaskDetail, len(tasks))
	copy(finalized, tasks)

	for i := range finalized {
		taskStatus := NormalizeRunDetailStatus(finalized[i].Status)
		if IsTerminalRunDetailTaskStatus(taskStatus) {
			continue
		}

		if finalized[i].StartedAt.IsZero() {
			if normalizedRun == "success" || normalizedRun == "warning" {
				finalized[i].Status = "success"
			}
			if normalizedRun == "failure" || normalizedRun == "cancelled" || normalizedRun == "rejected" {
				finalized[i].Status = "skipped"
			}
			continue
		}

		switch normalizedRun {
		case "success", "warning":
			finalized[i].Status = "success"
		case "cancelled", "rejected":
			finalized[i].Status = "cancelled"
		case "failure":
			if normalizedStep == "failure" {
				finalized[i].Status = "failure"
				if finalized[i].ExitCode == nil {
					exitCode := 1
					finalized[i].ExitCode = &exitCode
				}
			} else {
				finalized[i].Status = "cancelled"
			}
		}
		if finalized[i].FinishedAt.IsZero() && !runFinishedAt.IsZero() {
			finalized[i].FinishedAt = runFinishedAt
		}
	}

	return finalized
}

func IsTerminalRunDetailTaskStatus(status string) bool {
	switch NormalizeRunDetailStatus(status) {
	case "success", "warning", "failure", "cancelled", "skipped", "rejected":
		return true
	default:
		return false
	}
}

func BuildRunDetailETag(run models.RunListItem, childRuns []models.RunListItem, tasksByStep map[string][]models.TaskDetail, stepAIUsage map[string]models.AIUsageSummary, taskAIUsage map[string]models.AIUsageSummary, finalOutputs []models.PipelineRunFinalOutput) string {
	hasher := sha256.New()
	fmt.Fprintf(
		hasher,
		"run|%s|%s|%t|%d|%d|%s|%s|%s|%d|%d|%d|%.8f|",
		run.RunID,
		NormalizeRunDetailStatus(run.Status),
		run.IsComplete,
		run.StartedAt.UnixNano(),
		run.FinishedAt.UnixNano(),
		strings.TrimSpace(run.FailureReason),
		strings.TrimSpace(run.PipelineSource),
		strings.TrimSpace(run.TriggerEventID),
		run.AIUsage.PromptTokens,
		run.AIUsage.CompletionTokens,
		run.AIUsage.TotalTokens,
		run.AIUsage.TotalCostUSD,
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
			taskOutputs := append([]models.TaskRuntimeOutput(nil), task.Outputs...)
			sort.Slice(taskOutputs, func(i, j int) bool {
				if taskOutputs[i].StepName != taskOutputs[j].StepName {
					return taskOutputs[i].StepName < taskOutputs[j].StepName
				}
				if taskOutputs[i].TaskName != taskOutputs[j].TaskName {
					return taskOutputs[i].TaskName < taskOutputs[j].TaskName
				}
				return taskOutputs[i].Name < taskOutputs[j].Name
			})
			for _, output := range taskOutputs {
				fmt.Fprintf(
					hasher,
					"task_output|%s|%s|%s|%t|%d|",
					strings.TrimSpace(output.StepName),
					strings.TrimSpace(output.TaskName),
					strings.TrimSpace(output.Name),
					output.Sensitive,
					output.SizeBytes,
				)
			}
		}
	}

	writeAIUsageMapToHash(hasher, "step_usage", stepAIUsage)
	writeAIUsageMapToHash(hasher, "task_usage", taskAIUsage)
	for _, output := range finalOutputs {
		fmt.Fprintf(
			hasher,
			"output|%s|%s|%s|%s|%s|%s|%s|%s|%d|%d|%d|%d|%d|%d|",
			output.ID,
			strings.TrimSpace(output.Name),
			strings.TrimSpace(output.Type),
			strings.TrimSpace(output.Status),
			strings.TrimSpace(output.Content),
			strings.TrimSpace(output.Error),
			strings.TrimSpace(output.LLMProfile),
			finalOutputDashboardTargetHash(output.DashboardTarget),
			output.GenerationAttempts,
			output.ContractViolations,
			output.RenderAttempts,
			output.RenderFailures,
			output.CreatedAt.UnixNano(),
			output.UpdatedAt.UnixNano(),
		)
	}

	return fmt.Sprintf(`W/"%x"`, hasher.Sum(nil))
}

func finalOutputDashboardTargetHash(target *models.DashboardOutputTarget) string {
	if target == nil {
		return ""
	}
	return strings.Join([]string{
		strings.TrimSpace(target.Ref),
		strings.TrimSpace(target.Section),
		strings.TrimSpace(target.EntryKey),
		strings.TrimSpace(target.Mode),
		strings.TrimSpace(target.Preset),
		strings.TrimSpace(target.TTL),
	}, "|")
}

func writeAIUsageMapToHash(hasher interface {
	Write([]byte) (int, error)
}, prefix string, usageMap map[string]models.AIUsageSummary) {
	keys := make([]string, 0, len(usageMap))
	for key := range usageMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		usage := usageMap[key]
		fmt.Fprintf(
			hasher,
			"%s|%s|%d|%d|%d|%.8f|",
			prefix,
			key,
			usage.PromptTokens,
			usage.CompletionTokens,
			usage.TotalTokens,
			usage.TotalCostUSD,
		)
	}
}

func NormalizeRunDetailStatus(status string) string {
	raw := strings.ToLower(strings.TrimSpace(status))
	switch {
	case raw == "":
		return "pending"
	case raw == "success" || raw == "warning" || raw == "running" || raw == "pending" || raw == "skipped" || raw == "cancelled" || raw == "waiting_approval" || raw == "rejected" || raw == "timed_out":
		return raw
	case strings.Contains(raw, "ignored"):
		return "warning"
	case raw == "failure" || strings.Contains(raw, "not_found") || strings.Contains(raw, "timeout"):
		return "failure"
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
		"failure":          0,
		"rejected":         1,
		"timed_out":        2,
		"cancelled":        3,
		"waiting_approval": 4,
		"running":          5,
		"pending":          6,
		"warning":          7,
		"skipped":          8,
		"success":          9,
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

	if taskStatus == "failure" || taskStatus == "warning" || taskStatus == "cancelled" {
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
	case "success", "warning":
		return "success"
	case "failure", "timed_out":
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
	return DeriveRunDetailStepDurationUntil(tasks, childRuns, time.Time{})
}

func DeriveRunDetailStepDurationUntil(tasks []models.TaskDetail, childRuns []models.RunListItem, upperBound time.Time) time.Duration {
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
			if !upperBound.IsZero() && upperBound.After(latestFinish) {
				latestFinish = upperBound
			}
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
			if !upperBound.IsZero() && upperBound.After(latestFinish) {
				latestFinish = upperBound
			}
		}
	}

	if earliestStart.IsZero() {
		return 0
	}
	if !latestFinish.IsZero() && (!hasActiveWork || !upperBound.IsZero()) {
		if latestFinish.Before(earliestStart) {
			return 0
		}
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
	case "success", "warning", "failure", "failure (ignored)", "cancelled", "timed_out", "rejected":
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

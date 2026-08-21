package runs

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
)

type Queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type ListFilter struct {
	TeamID   *int
	RootTeam bool
	Branch   string
	Limit    int
	Offset   int
}

func List(ctx context.Context, db Queryer, filter ListFilter) ([]models.RunListItem, error) {
	query, args := BuildListRunsQuery(filter.TeamID, filter.RootTeam, filter.Branch, filter.Limit, filter.Offset)
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.RunListItem
	for rows.Next() {
		run, err := scanRunListItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ApplyDirectChildRunStatuses(ctx, db, out)
}

func ApplyDirectChildRunStatuses(ctx context.Context, db Queryer, runs []models.RunListItem) ([]models.RunListItem, error) {
	if len(runs) == 0 {
		return runs, nil
	}

	parentIDs := make([]string, 0, len(runs))
	runIndexes := make(map[string]int, len(runs))
	for index, run := range runs {
		runID := strings.TrimSpace(run.RunID)
		if runID == "" {
			continue
		}
		parentIDs = append(parentIDs, runID)
		runIndexes[runID] = index
	}
	if len(parentIDs) == 0 {
		return runs, nil
	}

	rows, err := db.Query(ctx, `
		SELECT parent_run_id::text, run_id::text, status, started_at, finished_at
		FROM pipeline_runs
		WHERE parent_run_id::text = ANY($1::text[])
	`, parentIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	childRunsByParent := make(map[string][]models.RunListItem)
	for rows.Next() {
		var parentRunID, childRunID, status string
		var startedAt, finishedAt sql.NullTime
		if err := rows.Scan(&parentRunID, &childRunID, &status, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		childRun := models.RunListItem{
			RunID:  childRunID,
			Status: status,
		}
		if startedAt.Valid {
			childRun.StartedAt = startedAt.Time
		}
		if finishedAt.Valid {
			childRun.FinishedAt = finishedAt.Time
			childRun.IsComplete = true
		} else {
			childRun.IsComplete = IsTerminalRunStatus(status)
		}
		childRunsByParent[parentRunID] = append(childRunsByParent[parentRunID], childRun)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for runID, childRuns := range childRunsByParent {
		index, ok := runIndexes[runID]
		if !ok {
			continue
		}
		runs[index] = ApplyChildRunStatus(runs[index], childRuns)
		refreshRunFinalOutputStatus(&runs[index])
	}
	return runs, nil
}

func refreshRunFinalOutputStatus(run *models.RunListItem) {
	if run == nil || run.FinalOutputStatus == nil {
		return
	}
	status := run.FinalOutputStatus
	status.Status = FinalOutputAggregateStatus(
		status.Configured,
		run.Status,
		status.Total,
		status.Pending,
		status.Generating,
		status.Generated,
		status.Failed,
		status.Cancelled,
	)
}

func BuildListRunsQuery(teamID *int, rootTeam bool, branchName string, limit, offset int) (string, []any) {
	query := `
		SELECT
		    pr.run_id, pr.pipeline_name, pr.pipeline_path, pr.pipeline_version, pr.status, pr.created_at, COALESCE(pr.git_commit_sha, ''),
		    COALESCE(pr.git_repo_owner, ''), COALESCE(pr.git_repo_name, ''), pr.started_at, pr.finished_at, pr.parent_run_id,
			COALESCE(pr.git_pusher_name, ''), COALESCE(pr.git_ref, ''), COALESCE(pr.git_target_ref, ''),
			COALESCE(pr.pipeline_source, ''), COALESCE(pr.trigger_event_id, ''), COALESCE(pr.failure_reason, ''),
			COALESCE(pr.trigger_source, ''), COALESCE(pr.schedule_id::text, ''), COALESCE(ps.name, ''), COALESCE(ps.path, ''),
			COALESCE(eti.trigger_id, ''), COALESCE(et.name, ''), COALESCE(eti.event_type, ''), COALESCE(eti.caller_type, ''),
			COALESCE(eti.caller_id, ''), COALESCE(eti.idempotency_key, ''),
			COALESCE(pr.pipeline_definition, ''),
			COALESCE(prus.ai_prompt_tokens, 0)::bigint, COALESCE(prus.ai_completion_tokens, 0)::bigint,
			COALESCE(prus.ai_total_tokens, 0)::bigint, COALESCE(prus.ai_cost_usd, 0)::float8, COALESCE(prus.ai_unpriced_calls, 0)::bigint,
			COALESCE(outputs.total, 0)::int,
			COALESCE(outputs.pending, 0)::int,
			COALESCE(outputs.generating, 0)::int,
			COALESCE(outputs.generated, 0)::int,
			COALESCE(outputs.failed, 0)::int,
			COALESCE(outputs.cancelled, 0)::int,
			outputs.updated_at
			FROM pipeline_runs pr
		LEFT JOIN pipeline_schedules ps ON ps.id = pr.schedule_id
		LEFT JOIN external_trigger_invocations eti ON eti.id::text = pr.trigger_event_id
		LEFT JOIN external_triggers et ON et.id = eti.trigger_id
		LEFT JOIN pipeline_run_usage_summary prus ON prus.run_id = pr.run_id
		LEFT JOIN LATERAL (
			SELECT COUNT(*)::int AS total,
			       COUNT(*) FILTER (WHERE status = 'pending')::int AS pending,
			       COUNT(*) FILTER (WHERE status = 'generating')::int AS generating,
			       COUNT(*) FILTER (WHERE status = 'success')::int AS generated,
			       COUNT(*) FILTER (WHERE status = 'failure')::int AS failed,
			       COUNT(*) FILTER (WHERE status = 'cancelled')::int AS cancelled,
			       MAX(updated_at) AS updated_at
			FROM pipeline_run_outputs
			WHERE run_id = pr.run_id
		) outputs ON true
	`
	args := []any{}
	var conditions []string
	withClause := ""

	if teamID != nil {
		args = append(args, *teamID)
		withClause = fmt.Sprintf(`
			WITH RECURSIVE selected_teams AS (
				SELECT id FROM teams WHERE id = $%d
				UNION ALL
				SELECT g.id
				FROM teams g
				JOIN selected_teams sg ON g.parent_id = sg.id
			)
		`, len(args))
		conditions = append(conditions, "pr.team_id IN (SELECT id FROM selected_teams)")
	} else if rootTeam {
		conditions = append(conditions, "pr.team_id IS NULL")
	}

	if branchName != "" {
		conditions = append(conditions, fmt.Sprintf("pr.git_ref = $%d", len(args)+1))
		args = append(args, "refs/heads/"+branchName)
	}

	query = withClause + query
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY pr.created_at DESC LIMIT %d OFFSET %d", limit, offset)
	return query, args
}

func TeamByBranch(items []models.RunListItem) map[string][]models.RunListItem {
	out := make(map[string][]models.RunListItem)
	for _, run := range items {
		branch := "Others"
		if run.GitRef != "" {
			branch = strings.TrimPrefix(run.GitRef, "refs/heads/")
		}
		out[branch] = append(out[branch], run)
	}
	return out
}

func scanRunListItem(scanner interface {
	Scan(dest ...any) error
}) (models.RunListItem, error) {
	var run models.RunListItem
	var startedAt, finishedAt sql.NullTime
	var commitSHA, repoOwner, repoName, pusherName, gitRef, gitTargetRef, pipelineSource, pipelineVersion, pipelinePath, triggerEventID, failureReason, triggerSource, scheduleID, scheduleName, schedulePath sql.NullString
	var externalTriggerID, externalTriggerName, externalTriggerEventType, externalTriggerCallerType, externalTriggerCallerID, externalTriggerIdempotency sql.NullString
	var pipelineDefinition string
	var outputsUpdatedAt sql.NullTime
	var outputTotal, outputPending, outputGenerating, outputGenerated, outputFailed, outputCancelled int
	err := scanner.Scan(
		&run.RunID, &run.PipelineName, &pipelinePath, &pipelineVersion, &run.Status, &run.CreatedAt, &commitSHA,
		&repoOwner, &repoName, &startedAt, &finishedAt, &run.ParentRunID, &pusherName, &gitRef, &gitTargetRef, &pipelineSource, &triggerEventID, &failureReason,
		&triggerSource, &scheduleID, &scheduleName, &schedulePath, &externalTriggerID, &externalTriggerName, &externalTriggerEventType,
		&externalTriggerCallerType, &externalTriggerCallerID, &externalTriggerIdempotency,
		&pipelineDefinition,
		&run.AIUsage.PromptTokens, &run.AIUsage.CompletionTokens, &run.AIUsage.TotalTokens, &run.AIUsage.SpendUSD, &run.AIUsage.UnpricedCalls,
		&outputTotal, &outputPending, &outputGenerating, &outputGenerated, &outputFailed, &outputCancelled, &outputsUpdatedAt,
	)
	if err != nil {
		return models.RunListItem{}, err
	}

	run.PipelinePath = pipelinePath.String
	run.GitCommitSHA = commitSHA.String
	run.PipelineVersion = NormalizePipelineVersion(pipelineVersion.String)
	run.GitRepoOwner = repoOwner.String
	run.GitRepoName = repoName.String
	run.GitPusherName = pusherName.String
	run.GitRef = gitRef.String
	run.GitTargetRef = gitTargetRef.String
	run.PipelineSource = pipelineSource.String
	run.TriggerEventID = triggerEventID.String
	run.FailureReason = failureReason.String
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
	run.FinalOutputStatus = SummarizeFinalOutputStatus(
		CountConfiguredFinalOutputs(pipelineDefinition),
		run.Status,
		outputTotal,
		outputPending,
		outputGenerating,
		outputGenerated,
		outputFailed,
		outputCancelled,
		outputsUpdatedAt,
	)
	if startedAt.Valid {
		run.StartedAt = startedAt.Time
		if finishedAt.Valid {
			run.FinishedAt = finishedAt.Time
			run.Duration = run.FinishedAt.Sub(run.StartedAt).Round(time.Second).String()
			run.IsComplete = true
		} else {
			run.Duration = time.Since(run.StartedAt).Round(time.Second).String()
			run.IsComplete = false
		}
	} else {
		run.IsComplete = true
	}
	return run, nil
}

func CountConfiguredFinalOutputs(definition string) int {
	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(definition), &pipeline); err != nil {
		return 0
	}
	return len(pipeline.Output.Items)
}

func SummarizeFinalOutputStatus(
	configured int,
	runStatus string,
	total int,
	pending int,
	generating int,
	generated int,
	failed int,
	cancelled int,
	updatedAt sql.NullTime,
) *models.FinalOutputStatusSummary {
	if configured <= 0 && total <= 0 {
		return nil
	}
	if configured <= 0 {
		configured = total
	}
	summary := &models.FinalOutputStatusSummary{
		Status:     FinalOutputAggregateStatus(configured, runStatus, total, pending, generating, generated, failed, cancelled),
		Configured: configured,
		Total:      total,
		Pending:    pending,
		Generating: generating,
		Generated:  generated,
		Failed:     failed,
		Cancelled:  cancelled,
	}
	if updatedAt.Valid {
		summary.UpdatedAt = &updatedAt.Time
	}
	return summary
}

func FinalOutputAggregateStatus(
	configured int,
	runStatus string,
	total int,
	pending int,
	generating int,
	generated int,
	failed int,
	cancelled int,
) string {
	if configured <= 0 && total <= 0 {
		return ""
	}
	if total <= 0 {
		if !IsTerminalRunStatus(NormalizeRunDetailStatus(runStatus)) {
			return "waiting"
		}
		return "not_generated"
	}
	if generating > 0 {
		return "generating"
	}
	if pending > 0 {
		return "pending"
	}
	if failed > 0 {
		if generated > 0 || cancelled > 0 {
			return "partial_failure"
		}
		return "failure"
	}
	if cancelled > 0 {
		if generated > 0 {
			return "partial_cancelled"
		}
		return "cancelled"
	}
	if generated >= total {
		return "success"
	}
	return "pending"
}

func NormalizePipelineVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "latest"
	}
	return version
}

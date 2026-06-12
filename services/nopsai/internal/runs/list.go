package runs

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"nopsai/pkg/models"
)

type Queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type ListFilter struct {
	GroupID   *int
	RootGroup bool
	Branch    string
	Limit     int
	Offset    int
}

func List(ctx context.Context, db Queryer, filter ListFilter) ([]models.RunListItem, error) {
	query, args := BuildListRunsQuery(filter.GroupID, filter.RootGroup, filter.Branch, filter.Limit, filter.Offset)
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
	return out, nil
}

func BuildListRunsQuery(groupID *int, rootGroup bool, branchName string, limit, offset int) (string, []any) {
	query := `
		SELECT
		    pr.run_id, pr.pipeline_name, pr.pipeline_path, pr.pipeline_version, pr.status, COALESCE(pr.git_commit_sha, ''),
		    COALESCE(pr.git_repo_owner, ''), COALESCE(pr.git_repo_name, ''), pr.started_at, pr.finished_at, pr.parent_run_id,
			COALESCE(pr.git_pusher_name, ''), COALESCE(pr.git_ref, ''), COALESCE(pr.git_target_ref, ''),
			COALESCE(pr.pipeline_source, ''), COALESCE(pr.trigger_event_id, ''), COALESCE(pr.failure_reason, ''),
			COALESCE(pr.trigger_source, ''), COALESCE(pr.schedule_id::text, ''), COALESCE(ps.name, ''), COALESCE(ps.path, ''),
			COALESCE(eti.trigger_id, ''), COALESCE(et.name, ''), COALESCE(eti.event_type, ''), COALESCE(eti.caller_type, ''),
			COALESCE(eti.caller_id, ''), COALESCE(eti.idempotency_key, ''),
			COALESCE(prus.ai_prompt_tokens, 0)::bigint, COALESCE(prus.ai_completion_tokens, 0)::bigint,
			COALESCE(prus.ai_total_tokens, 0)::bigint, COALESCE(prus.ai_cost_usd, 0)::float8
			FROM pipeline_runs pr
		LEFT JOIN pipeline_schedules ps ON ps.id = pr.schedule_id
		LEFT JOIN external_trigger_invocations eti ON eti.id::text = pr.trigger_event_id
		LEFT JOIN external_triggers et ON et.id = eti.trigger_id
		LEFT JOIN pipeline_run_usage_summary prus ON prus.run_id = pr.run_id
	`
	args := []any{}
	var conditions []string
	withClause := ""

	if groupID != nil {
		args = append(args, *groupID)
		withClause = fmt.Sprintf(`
			WITH RECURSIVE selected_groups AS (
				SELECT id FROM groups WHERE id = $%d
				UNION ALL
				SELECT g.id
				FROM groups g
				JOIN selected_groups sg ON g.parent_id = sg.id
			)
		`, len(args))
		conditions = append(conditions, "pr.group_id IN (SELECT id FROM selected_groups)")
	} else if rootGroup {
		conditions = append(conditions, "pr.group_id IS NULL")
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

func GroupByBranch(items []models.RunListItem) map[string][]models.RunListItem {
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
	err := scanner.Scan(
		&run.RunID, &run.PipelineName, &pipelinePath, &pipelineVersion, &run.Status, &commitSHA,
		&repoOwner, &repoName, &startedAt, &finishedAt, &run.ParentRunID, &pusherName, &gitRef, &gitTargetRef, &pipelineSource, &triggerEventID, &failureReason,
		&triggerSource, &scheduleID, &scheduleName, &schedulePath, &externalTriggerID, &externalTriggerName, &externalTriggerEventType,
		&externalTriggerCallerType, &externalTriggerCallerID, &externalTriggerIdempotency,
		&run.AIUsage.PromptTokens, &run.AIUsage.CompletionTokens, &run.AIUsage.TotalTokens, &run.AIUsage.TotalCostUSD,
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

func NormalizePipelineVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "latest"
	}
	return version
}

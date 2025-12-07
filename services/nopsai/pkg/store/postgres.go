package store

import (
	"context"
	"database/sql"
	"time"

	"nopsai/pkg/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PGStore struct {
	db *pgxpool.Pool
}

func NewPGStore(db *pgxpool.Pool) *PGStore {
	return &PGStore{db: db}
}

func (s *PGStore) GetRunListItem(ctx context.Context, runID string) (*models.RunListItem, error) {
	var run models.RunListItem
	var startedAt, finishedAt sql.NullTime
	var commitSHA, repoOwner, repoName, pusherName, gitRef, gitTargetRef, pipelineSource, pipelineVersion, pipelinePath, triggerEventID sql.NullString

	query := `
        SELECT
		    run_id, pipeline_name, pipeline_path, pipeline_version, status, COALESCE(git_commit_sha, ''),
            COALESCE(git_repo_owner, ''), COALESCE(git_repo_name, ''), started_at, finished_at,
			parent_run_id, COALESCE(git_pusher_name, ''), COALESCE(git_ref, ''), COALESCE(git_target_ref, ''),
			COALESCE(pipeline_source, ''), COALESCE(trigger_event_id, '')
        FROM pipeline_runs
        WHERE run_id = $1
    `
	err := s.db.QueryRow(ctx, query, runID).Scan(
		&run.RunID, &run.PipelineName, &pipelinePath, &pipelineVersion, &run.Status, &commitSHA,
		&repoOwner, &repoName, &startedAt, &finishedAt, &run.ParentRunID, &pusherName, &gitRef, &gitTargetRef, &pipelineSource, &triggerEventID,
	)

	if err != nil {
		return nil, err
	}

	run.PipelinePath = pipelinePath.String
	run.GitCommitSHA = commitSHA.String
	run.PipelineVersion = normalizePipelineVersion(pipelineVersion.String)
	run.GitRepoOwner = repoOwner.String
	run.GitRepoName = repoName.String
	run.GitPusherName = pusherName.String
	run.GitRef = gitRef.String
	run.GitTargetRef = gitTargetRef.String
	run.PipelineSource = pipelineSource.String
	run.TriggerEventID = triggerEventID.String

	if startedAt.Valid {
		run.StartedAt = startedAt.Time
		if finishedAt.Valid {
			run.FinishedAt = finishedAt.Time
			run.Duration = run.FinishedAt.Sub(run.StartedAt).Round(time.Second).String()
			run.IsComplete = true
		} else {
			run.Duration = time.Since(run.StartedAt).Round(time.Second).String()
			run.IsComplete = isTerminalRunStatus(run.Status)
		}
	} else {
		run.Duration = "0s"
		run.IsComplete = isTerminalRunStatus(run.Status)
	}

	return &run, nil
}

// Helpers needed for GetRunListItem
// In a full refactor, these might be in a utils package or shared method
func normalizePipelineVersion(version string) string {
	if version == "" {
		return "latest"
	}
	return version
}

func isTerminalRunStatus(status string) bool {
	return status == "success" || status == "failure" || status == "cancelled" || status == "timed_out"
}

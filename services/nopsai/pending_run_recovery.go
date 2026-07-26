package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
)

const (
	pendingRunRecoveryPollInterval = 15 * time.Second
	pendingRunRecoveryMinAge       = 10 * time.Second
	pendingRunRecoveryBatchSize    = 25
)

type pendingRunRecoveryRecord struct {
	RunID              string
	ParentRunID        string
	ParentRunnerID     string
	ParentHistory      string
	PipelineName       string
	PipelineVersion    string
	PipelineDefinition string
	Scope              string
	CreatedAt          time.Time
	TimeoutAt          *time.Time
	GitContext         map[string]string
	VariableOverrides  map[string]string
}

func (a *App) runPendingRunRecoveryWorker(ctx context.Context) {
	ticker := time.NewTicker(pendingRunRecoveryPollInterval)
	defer ticker.Stop()

	a.recoverPendingPipelineRuns(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.recoverPendingPipelineRuns(ctx)
		}
	}
}

func (a *App) recoverPendingPipelineRuns(ctx context.Context) {
	if a == nil || a.db == nil || a.dispatcher == nil {
		return
	}
	cutoff := time.Now().Add(-pendingRunRecoveryMinAge)
	records, err := a.listPendingRunsForRecovery(ctx, cutoff, pendingRunRecoveryBatchSize)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to list pending pipeline runs for recovery")
		return
	}
	for _, record := range records {
		if err := a.recoverPendingPipelineRun(ctx, record); err != nil {
			log.Warn().Err(err).Str("run_id", record.RunID).Msg("Failed to recover pending pipeline run")
		}
	}
}

func (a *App) listPendingRunsForRecovery(ctx context.Context, cutoff time.Time, limit int) ([]pendingRunRecoveryRecord, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := a.db.Query(ctx, `
		SELECT run_id::text,
		       COALESCE(parent_run_id::text, ''),
		       COALESCE(parent_runner_id, ''),
		       COALESCE(parent_history, ''),
		       COALESCE(pipeline_name, ''),
		       COALESCE(pipeline_version, ''),
		       COALESCE(pipeline_definition, ''),
		       COALESCE(scope, ''),
		       created_at,
		       timeout_at,
		       COALESCE(runtime_variable_overrides, '{}'::jsonb),
		       COALESCE(git_repo_owner, ''),
		       COALESCE(git_repo_name, ''),
		       COALESCE(git_clone_url, ''),
		       COALESCE(git_ssh_url, ''),
		       COALESCE(git_ref, ''),
		       COALESCE(git_target_ref, ''),
		       COALESCE(git_commit_sha, ''),
		       COALESCE(git_commit_url, ''),
		       COALESCE(git_commit_message, ''),
		       COALESCE(git_commit_author_name, ''),
		       COALESCE(git_commit_author_email, ''),
		       COALESCE(git_commit_author_username, ''),
		       COALESCE(git_pusher_name, ''),
		       COALESCE(git_pusher_email, ''),
		       COALESCE(git_check_run_id::text, ''),
		       COALESCE(trigger_event_id, '')
		FROM pipeline_runs
		WHERE status = 'pending'
		  AND created_at <= $1
		ORDER BY created_at ASC
		LIMIT $2
	`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []pendingRunRecoveryRecord
	for rows.Next() {
		var record pendingRunRecoveryRecord
		var timeoutAt sql.NullTime
		var overrides []byte
		var repoOwner, repoName, cloneURL, sshURL, ref, targetRef, commitSHA, commitURL, commitMessage string
		var commitAuthorName, commitAuthorEmail, commitAuthorUsername, pusherName, pusherEmail, checkRunID, triggerEventID string
		if err := rows.Scan(
			&record.RunID,
			&record.ParentRunID,
			&record.ParentRunnerID,
			&record.ParentHistory,
			&record.PipelineName,
			&record.PipelineVersion,
			&record.PipelineDefinition,
			&record.Scope,
			&record.CreatedAt,
			&timeoutAt,
			&overrides,
			&repoOwner,
			&repoName,
			&cloneURL,
			&sshURL,
			&ref,
			&targetRef,
			&commitSHA,
			&commitURL,
			&commitMessage,
			&commitAuthorName,
			&commitAuthorEmail,
			&commitAuthorUsername,
			&pusherName,
			&pusherEmail,
			&checkRunID,
			&triggerEventID,
		); err != nil {
			return nil, err
		}
		if timeoutAt.Valid {
			record.TimeoutAt = &timeoutAt.Time
		}
		record.VariableOverrides = map[string]string{}
		if len(overrides) > 0 {
			if err := json.Unmarshal(overrides, &record.VariableOverrides); err != nil {
				return nil, fmt.Errorf("decode variable overrides for run %s: %w", record.RunID, err)
			}
		}
		record.GitContext = pendingRunRecoveryGitContext(map[string]string{
			"repo_owner":             repoOwner,
			"repo_name":              repoName,
			"clone_url":              cloneURL,
			"ssh_url":                sshURL,
			"ref":                    ref,
			"target_ref":             targetRef,
			"commit_sha":             commitSHA,
			"commit_url":             commitURL,
			"commit_message":         commitMessage,
			"commit_author_name":     commitAuthorName,
			"commit_author_email":    commitAuthorEmail,
			"commit_author_username": commitAuthorUsername,
			"pusher_name":            pusherName,
			"pusher_email":           pusherEmail,
			"check_run_id":           checkRunID,
			"trigger_event_id":       triggerEventID,
		})
		records = append(records, record)
	}
	return records, rows.Err()
}

func (a *App) recoverPendingPipelineRun(ctx context.Context, record pendingRunRecoveryRecord) error {
	req, err := pendingRunRecoveryLaunchRequest(record)
	if err != nil {
		return err
	}
	a.agentRunLauncher().LaunchAgent(ctx, req)
	return nil
}

func pendingRunRecoveryLaunchRequest(record pendingRunRecoveryRecord) (AgentRunLaunchRequest, error) {
	runID := strings.TrimSpace(record.RunID)
	if runID == "" {
		return AgentRunLaunchRequest{}, fmt.Errorf("run id is required")
	}
	pipelineDefinition := []byte(strings.TrimSpace(record.PipelineDefinition))
	if len(pipelineDefinition) == 0 {
		return AgentRunLaunchRequest{}, fmt.Errorf("pipeline definition is empty")
	}

	var pipeline models.Pipeline
	if err := yaml.Unmarshal(pipelineDefinition, &pipeline); err != nil {
		return AgentRunLaunchRequest{}, fmt.Errorf("parse pipeline definition: %w", err)
	}
	pipeline.Name = sanitizeInput(firstNonEmptyString(pipeline.Name, record.PipelineName))
	pipeline.Version = normalizePipelineVersion(firstNonEmptyString(pipeline.Version, record.PipelineVersion))
	timeout := pendingRunRecoveryOriginalTimeout(record.CreatedAt, record.TimeoutAt)

	return AgentRunLaunchRequest{
		RunID:              runID,
		ParentRunID:        strings.TrimSpace(record.ParentRunID),
		ParentRunnerID:     strings.TrimSpace(record.ParentRunnerID),
		Pipeline:           pipeline,
		PipelineDefinition: pipelineDefinition,
		Timeout:            timeout,
		GitContext:         pendingRunRecoveryGitContext(record.GitContext),
		ParentHistory:      record.ParentHistory,
		Scope:              strings.TrimSpace(record.Scope),
		Overrides:          cloneStringMap(record.VariableOverrides),
		RecoveryAttempt:    true,
	}, nil
}

func pendingRunRecoveryOriginalTimeout(createdAt time.Time, timeoutAt *time.Time) time.Duration {
	if timeoutAt == nil || timeoutAt.IsZero() || createdAt.IsZero() {
		return 0
	}
	timeout := timeoutAt.Sub(createdAt)
	if timeout <= 0 {
		return 0
	}
	return timeout
}

func pendingRunRecoveryGitContext(input map[string]string) map[string]string {
	context := make(map[string]string, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		if key == "check_run_id" {
			if _, err := strconv.ParseInt(value, 10, 64); err != nil {
				continue
			}
		}
		context[key] = value
	}
	return context
}

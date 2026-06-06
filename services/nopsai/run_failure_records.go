package nopsai

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"nopsai/services/nopsai/internal/configsync"
)

func (a *App) recordMissingPipelineRun(identifier string, pipelineVersion string, pipelineDef []byte, gitContext map[string]string, scopeValue, pipelineSource, summary string) {
	runID := uuid.New()
	pathPart, namePart, _, err := configsync.SplitPipelineIdentifier(identifier)
	if err != nil {
		namePart = sanitizeInput(identifier)
		pathPart = ""
	}
	namePart = sanitizeInput(namePart)
	if namePart == "" {
		namePart = "missing-pipeline"
	}

	groupID, groupErr := a.resolveGroupIDForRun(context.Background(), "", pathPart, gitContext)
	if groupErr != nil {
		log.Error().Err(groupErr).Str("pipeline", identifier).Msg("Failed to resolve group for missing pipeline run")
	}

	var triggerEventIDSQL sql.NullString
	if gitContext != nil {
		id := strings.TrimSpace(gitContext["trigger_event_id"])
		if id == "" {
			id = deriveTriggerEventID(gitContext)
		}
		if id == "" {
			id = runID.String()
		}
		if id != "" {
			triggerEventIDSQL.String = id
			triggerEventIDSQL.Valid = true
			gitContext["trigger_event_id"] = id
		}
	}
	checkRunIDSQL := nullableGitCheckRunID(gitContext)

	now := time.Now()
	_, err = a.db.Exec(context.Background(), `
		INSERT INTO pipeline_runs (
			run_id, pipeline_name, pipeline_path, pipeline_version, status,
			pipeline_definition, git_repo_owner, git_repo_name, git_clone_url, git_ssh_url,
			git_ref, git_target_ref, git_commit_sha, git_commit_url, git_commit_message,
			git_commit_author_name, git_commit_author_email, git_commit_author_username,
			git_pusher_name, git_pusher_email, git_check_run_id, group_id, trigger_event_id,
			scope, pipeline_source, started_at, finished_at, failure_reason
		) VALUES (
			$1, $2, $3, $4, 'failure', $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27
		)`,
		runID,
		namePart,
		pathPart,
		normalizePipelineVersion(pipelineVersion),
		string(pipelineDef),
		gitContext["repo_owner"],
		gitContext["repo_name"],
		gitContext["clone_url"],
		gitContext["ssh_url"],
		gitContext["ref"],
		gitContext["target_ref"],
		gitContext["commit_sha"],
		gitContext["commit_url"],
		gitContext["commit_message"],
		gitContext["commit_author_name"],
		gitContext["commit_author_email"],
		gitContext["commit_author_username"],
		gitContext["pusher_name"],
		gitContext["pusher_email"],
		checkRunIDSQL,
		groupID,
		triggerEventIDSQL,
		scopeValue,
		pipelineSource,
		now,
		now,
		summary,
	)
	if err != nil {
		log.Error().Err(err).Str("pipeline", identifier).Msg("Failed to record missing pipeline run")
		return
	}
}

func (a *App) recordAuthorizationDeniedPipelineRun(identifier string, pipelineVersion string, pipelineDef []byte, gitContext map[string]string, scopeValue, pipelineSource, triggerSource, callerType, callerID, summary string, authChecks []ResourceUseAuthResult) {
	if gitContext == nil {
		gitContext = map[string]string{}
	}
	runID := uuid.New()
	pathPart, namePart, _, err := configsync.SplitPipelineIdentifier(identifier)
	if err != nil {
		namePart = sanitizeInput(identifier)
		pathPart = ""
	}
	namePart = sanitizeInput(namePart)
	if namePart == "" {
		namePart = "authorization-denied"
	}

	groupID, groupErr := a.resolveGroupIDForRun(context.Background(), "", pathPart, gitContext)
	if groupErr != nil {
		log.Error().Err(groupErr).Str("pipeline", identifier).Msg("Failed to resolve group for authorization denied pipeline run")
	}

	var triggerEventIDSQL sql.NullString
	id := strings.TrimSpace(gitContext["trigger_event_id"])
	if id == "" {
		id = deriveTriggerEventID(gitContext)
	}
	if id == "" {
		id = runID.String()
	}
	if id != "" {
		triggerEventIDSQL.String = id
		triggerEventIDSQL.Valid = true
		gitContext["trigger_event_id"] = id
	}

	checkRunIDSQL := nullableGitCheckRunID(gitContext)

	authSnapshot, snapshotErr := buildRunAuthorizationSnapshot(triggerSource, callerType, callerID, authChecks)
	if snapshotErr != nil {
		log.Error().Err(snapshotErr).Str("pipeline", identifier).Msg("Failed to build authorization denied run snapshot")
		authSnapshot = []byte(`{}`)
	}

	now := time.Now()
	_, err = a.db.Exec(context.Background(), `
		INSERT INTO pipeline_runs (
			run_id, pipeline_name, pipeline_path, pipeline_version, status,
			pipeline_definition, git_repo_owner, git_repo_name, git_clone_url, git_ssh_url,
			git_ref, git_target_ref, git_commit_sha, git_commit_url, git_commit_message,
			git_commit_author_name, git_commit_author_email, git_commit_author_username,
			git_pusher_name, git_pusher_email, git_check_run_id, group_id, trigger_event_id,
			scope, pipeline_source, trigger_source, requested_by_type, requested_by_id,
			effective_subject_type, effective_subject_id, authorization_snapshot, started_at,
			finished_at, failure_reason
		) VALUES (
			$1, $2, $3, $4, 'failure', $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28,
			$29, $30::jsonb, $31, $32, $33
		)`,
		runID,
		namePart,
		pathPart,
		normalizePipelineVersion(pipelineVersion),
		string(pipelineDef),
		gitContext["repo_owner"],
		gitContext["repo_name"],
		gitContext["clone_url"],
		gitContext["ssh_url"],
		gitContext["ref"],
		gitContext["target_ref"],
		gitContext["commit_sha"],
		gitContext["commit_url"],
		gitContext["commit_message"],
		gitContext["commit_author_name"],
		gitContext["commit_author_email"],
		gitContext["commit_author_username"],
		gitContext["pusher_name"],
		gitContext["pusher_email"],
		checkRunIDSQL,
		groupID,
		triggerEventIDSQL,
		scopeValue,
		pipelineSource,
		triggerSource,
		callerType,
		callerID,
		callerType,
		callerID,
		string(authSnapshot),
		now,
		now,
		summary,
	)
	if err != nil {
		log.Error().Err(err).Str("pipeline", identifier).Msg("Failed to record authorization denied pipeline run")
		return
	}
}

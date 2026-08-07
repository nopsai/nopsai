package nopsai

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/pkg/serviceauth"
	aaamodel "nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/auth"
	"nopsai/services/nopsai/pkg/routeauthz"
)

const (
	runStatusWaitingApproval  = "waiting_approval"
	runStatusRejected         = "rejected"
	runStatusTimedOut         = "timed_out"
	taskStatusWaitingApproval = "waiting_approval"
	taskStatusTimedOut        = "timed_out"
	approvalStatusPending     = "pending"
	approvalStatusApproved    = "approved"
	approvalStatusRejected    = "rejected"
	approvalStatusTimedOut    = "timed_out"
	approvalActionApprove     = "approval.approve"
	approvalRejectCommentText = "comment is required when rejecting an approval"
	approvalTimeoutComment    = "Approval timed out"
	approvalTimeoutBatchSize  = 50
)

type approvalPauseRequest struct {
	StepName               string            `json:"step_name"`
	TaskName               string            `json:"task_name"`
	ExecutionHistory       string            `json:"execution_history"`
	CompletedTasks         []string          `json:"completed_tasks"`
	PipelineDefinitionYAML string            `json:"pipeline_definition_yaml"`
	Variables              map[string]string `json:"variables,omitempty"`
	WorkspaceArchiveBase64 string            `json:"workspace_archive_base64"`
	SharedVolumeName       string            `json:"shared_volume_name,omitempty"`
	RunnerID               string            `json:"runner_id,omitempty"`
}

type approvalPauseResponse struct {
	ApprovalID   string `json:"approval_id"`
	CheckpointID string `json:"checkpoint_id"`
	Status       string `json:"status"`
}

type approvalCheckpointResponse struct {
	CheckpointID           string            `json:"checkpoint_id"`
	RunID                  string            `json:"run_id"`
	StepName               string            `json:"step_name"`
	ExecutionHistory       string            `json:"execution_history"`
	CompletedTasks         []string          `json:"completed_tasks"`
	PipelineDefinitionYAML string            `json:"pipeline_definition_yaml"`
	Variables              map[string]string `json:"variables,omitempty"`
	WorkspaceArchiveBase64 string            `json:"workspace_archive_base64,omitempty"`
	WorkspaceArchiveFormat string            `json:"workspace_archive_format,omitempty"`
}

type pipelineApprovalResponse struct {
	ID                string     `json:"id"`
	RunID             string     `json:"run_id"`
	StepName          string     `json:"step_name"`
	TaskName          string     `json:"task_name"`
	ApprovalType      string     `json:"approval_type"`
	AssignedTeams     []string   `json:"assigned_teams"`
	AllowSelfApproval bool       `json:"allow_self_approval"`
	Status            string     `json:"status"`
	RequestedByType   string     `json:"requested_by_type,omitempty"`
	RequestedByID     string     `json:"requested_by_id,omitempty"`
	RequestedAt       time.Time  `json:"requested_at"`
	DecidedByType     string     `json:"decided_by_type,omitempty"`
	DecidedByID       string     `json:"decided_by_id,omitempty"`
	DecidedByEmail    string     `json:"decided_by_email,omitempty"`
	DecidedAt         *time.Time `json:"decided_at,omitempty"`
	DecisionComment   string     `json:"decision_comment,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	CheckpointID      string     `json:"checkpoint_id,omitempty"`
}

type approvalDecisionRequest struct {
	Comment string `json:"comment"`
}

func normalizeApprovalDecisionComment(approved bool, raw string) (string, error) {
	comment := strings.TrimSpace(raw)
	if !approved && comment == "" {
		return "", errors.New(approvalRejectCommentText)
	}
	return comment, nil
}

type approvalDecisionRecord struct {
	ID                string
	RunID             string
	StepName          string
	TaskName          string
	ApprovalType      string
	AssignedTeams     []string
	AllowSelfApproval bool
	Status            string
	RequestedByType   string
	RequestedByID     string
	CheckpointID      string
	ExpiresAt         *time.Time
	RunStatus         string
	PipelineName      string
	PipelinePath      string
	PipelineVersion   string
	ParentRunID       string
	Scope             string
	PipelineDef       []byte
	Variables         map[string]string
	RunnerID          string
	GitContext        map[string]string
}

type pendingApprovalVisibility struct {
	RunID             string
	AssignedTeams     []string
	AllowSelfApproval bool
	RequestedByType   string
	RequestedByID     string
}

func (a *App) handlePauseRunForApproval(w http.ResponseWriter, r *http.Request) {
	if !requireInternalServiceRole(w, r, serviceauth.RoleAgent) {
		return
	}
	runID := strings.TrimSpace(r.PathValue("runID"))
	if _, err := uuid.Parse(runID); err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}
	var req approvalPauseRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid approval pause request", http.StatusBadRequest)
		return
	}
	req.StepName = strings.TrimSpace(req.StepName)
	if req.TaskName = strings.TrimSpace(req.TaskName); req.TaskName == "" {
		req.TaskName = req.StepName
	}
	if req.StepName == "" {
		http.Error(w, "step_name is required", http.StatusBadRequest)
		return
	}

	var storedPipelineDef, runStatus, requestedByType, requestedByID sql.NullString
	err := a.db.QueryRow(r.Context(), `
		SELECT pipeline_definition, status, COALESCE(requested_by_type, ''), COALESCE(requested_by_id, '')
		FROM pipeline_runs
		WHERE run_id = $1
	`, runID).Scan(&storedPipelineDef, &runStatus, &requestedByType, &requestedByID)
	if err != nil {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}
	if isTerminalRunStatus(runStatus.String) {
		http.Error(w, "Cannot pause a terminal run", http.StatusConflict)
		return
	}

	pipelineDef := strings.TrimSpace(req.PipelineDefinitionYAML)
	if pipelineDef == "" && storedPipelineDef.Valid {
		pipelineDef = storedPipelineDef.String
	}
	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(pipelineDef), &pipeline); err != nil {
		http.Error(w, fmt.Sprintf("Checkpoint pipeline definition is malformed: %v", err), http.StatusBadRequest)
		return
	}
	approvalConfig, ok := approvalDefinitionForStep(&pipeline, req.StepName)
	if !ok {
		http.Error(w, "Approval step not found in pipeline definition", http.StatusBadRequest)
		return
	}
	assignedTeams := normalizeApprovalTeams(approvalConfig.Teams)
	if len(assignedTeams) == 0 {
		http.Error(w, "Approval step must assign at least one team", http.StatusBadRequest)
		return
	}
	var expiresAt any
	if timeout := strings.TrimSpace(approvalConfig.Timeout); timeout != "" {
		duration, err := time.ParseDuration(timeout)
		if err != nil || duration <= 0 {
			http.Error(w, "Approval step timeout is invalid", http.StatusBadRequest)
			return
		}
		expiresAt = time.Now().UTC().Add(duration)
	}

	workspaceArchive, err := decodeOptionalBase64(req.WorkspaceArchiveBase64)
	if err != nil {
		http.Error(w, "workspace_archive_base64 is invalid", http.StatusBadRequest)
		return
	}
	completedTasks := normalizeCompletedTaskKeys(req.CompletedTasks)
	completedTasksJSON, _ := json.Marshal(completedTasks)
	assignedTeamsJSON, _ := json.Marshal(assignedTeams)
	variables := req.Variables
	if variables == nil {
		variables = map[string]string{}
	}
	variablesJSON, _ := json.Marshal(variables)

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "Failed to start approval transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	var existingStatus string
	existingErr := tx.QueryRow(r.Context(), `
		SELECT status
		FROM pipeline_approvals
		WHERE run_id = $1 AND step_name = $2
		FOR UPDATE
	`, runID, req.StepName).Scan(&existingStatus)
	if existingErr != nil && existingErr != pgx.ErrNoRows {
		http.Error(w, "Failed to inspect existing approval", http.StatusInternalServerError)
		return
	}
	if existingErr == nil && existingStatus != approvalStatusPending {
		http.Error(w, "Approval has already been decided", http.StatusConflict)
		return
	}

	var checkpointID uuid.UUID
	err = tx.QueryRow(r.Context(), `
		INSERT INTO pipeline_run_checkpoints (
			run_id, step_name, execution_history, pipeline_definition, variables,
			workspace_archive, shared_volume_name, runner_id, completed_tasks
		)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9::jsonb)
		RETURNING id
	`, runID, req.StepName, req.ExecutionHistory, pipelineDef, string(variablesJSON), workspaceArchive, strings.TrimSpace(req.SharedVolumeName), strings.TrimSpace(req.RunnerID), string(completedTasksJSON)).Scan(&checkpointID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to store approval checkpoint")
		http.Error(w, "Failed to store approval checkpoint", http.StatusInternalServerError)
		return
	}

	var approvalID uuid.UUID
	if existingErr == pgx.ErrNoRows {
		err = tx.QueryRow(r.Context(), `
			INSERT INTO pipeline_approvals (
				run_id, step_name, task_name, approval_type, assigned_teams,
				allow_self_approval, requested_by_type, requested_by_id, checkpoint_id, expires_at
			)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10)
			RETURNING id
		`, runID, req.StepName, req.TaskName, strings.TrimSpace(approvalConfig.Type), string(assignedTeamsJSON), approvalConfig.AllowSelfApproval, requestedByType.String, requestedByID.String, checkpointID, expiresAt).Scan(&approvalID)
	} else {
		err = tx.QueryRow(r.Context(), `
			UPDATE pipeline_approvals
			SET task_name = $3,
			    approval_type = $4,
			    assigned_teams = $5::jsonb,
			    allow_self_approval = $6,
			    requested_by_type = $7,
			    requested_by_id = $8,
			    requested_at = NOW(),
			    checkpoint_id = $9,
			    expires_at = $10
			WHERE run_id = $1 AND step_name = $2
			RETURNING id
		`, runID, req.StepName, req.TaskName, strings.TrimSpace(approvalConfig.Type), string(assignedTeamsJSON), approvalConfig.AllowSelfApproval, requestedByType.String, requestedByID.String, checkpointID, expiresAt).Scan(&approvalID)
	}
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to upsert approval")
		http.Error(w, "Failed to create approval", http.StatusInternalServerError)
		return
	}

	tag, err := tx.Exec(r.Context(), `
		UPDATE task_runs
		SET status = $1, started_at = COALESCE(started_at, NOW()), finished_at = NULL, exit_code = NULL
		WHERE run_id = $2 AND step_name = $3 AND task_name = $4
	`, taskStatusWaitingApproval, runID, req.StepName, req.TaskName)
	if err != nil {
		http.Error(w, "Failed to update approval task status", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "Approval task row not found", http.StatusPreconditionFailed)
		return
	}

	if _, err := tx.Exec(r.Context(), `
		UPDATE pipeline_runs
		SET status = $1, started_at = COALESCE(started_at, NOW()), finished_at = NULL
		WHERE run_id = $2
		  AND status NOT IN ('success', 'failure', 'failure (ignored)', 'cancelled', 'timed_out', 'rejected')
	`, runStatusWaitingApproval, runID); err != nil {
		http.Error(w, "Failed to update run status", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "Failed to commit approval pause", http.StatusInternalServerError)
		return
	}

	go a.notifyGitBotOfTaskStatus(runID, req.StepName, req.TaskName, taskStatusWaitingApproval)
	go a.dispatchPipelineRunNotification(runID, "approval_requested")
	_ = httpapi.WriteJSON(w, http.StatusOK, approvalPauseResponse{
		ApprovalID:   approvalID.String(),
		CheckpointID: checkpointID.String(),
		Status:       runStatusWaitingApproval,
	})
}

func (a *App) handleGetRunCheckpoint(w http.ResponseWriter, r *http.Request) {
	if !requireInternalServiceRole(w, r, serviceauth.RoleAgent) {
		return
	}
	runID := strings.TrimSpace(r.PathValue("runID"))
	checkpointID := strings.TrimSpace(r.PathValue("checkpointID"))
	if _, err := uuid.Parse(runID); err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}
	if _, err := uuid.Parse(checkpointID); err != nil {
		http.Error(w, "Invalid checkpoint ID", http.StatusBadRequest)
		return
	}

	var stepName, history, pipelineDef, archiveFormat string
	var variablesBytes []byte
	var archive []byte
	err := a.db.QueryRow(r.Context(), `
		SELECT step_name, execution_history, pipeline_definition, variables, workspace_archive, workspace_archive_format
		FROM pipeline_run_checkpoints
		WHERE id = $1 AND run_id = $2
	`, checkpointID, runID).Scan(&stepName, &history, &pipelineDef, &variablesBytes, &archive, &archiveFormat)
	if err != nil {
		http.Error(w, "Checkpoint not found", http.StatusNotFound)
		return
	}

	completedTasks, err := a.completedTaskKeysForRun(r.Context(), runID)
	if err != nil {
		http.Error(w, "Failed to load completed task keys", http.StatusInternalServerError)
		return
	}
	variables := map[string]string{}
	if len(variablesBytes) > 0 {
		_ = json.Unmarshal(variablesBytes, &variables)
	}

	resp := approvalCheckpointResponse{
		CheckpointID:           checkpointID,
		RunID:                  runID,
		StepName:               stepName,
		ExecutionHistory:       history,
		CompletedTasks:         completedTasks,
		PipelineDefinitionYAML: pipelineDef,
		Variables:              variables,
		WorkspaceArchiveFormat: archiveFormat,
	}
	if len(archive) > 0 {
		resp.WorkspaceArchiveBase64 = base64.StdEncoding.EncodeToString(archive)
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleListRunApprovals(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("runID"))
	authorized, err := a.canReadRunOrApprove(r, runID)
	if err != nil {
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	if !authorized {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	a.expireApprovalTimeouts(r.Context())
	rows, err := a.db.Query(r.Context(), `
		SELECT id::text, run_id::text, step_name, task_name, approval_type, assigned_teams,
		       allow_self_approval, status, requested_by_type, requested_by_id, requested_at,
		       decided_by_type, decided_by_id, decided_by_email, decided_at, decision_comment,
		       expires_at, COALESCE(checkpoint_id::text, '')
		FROM pipeline_approvals
		WHERE run_id = $1
		ORDER BY requested_at ASC
	`, runID)
	if err != nil {
		http.Error(w, "Failed to load approvals", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var approvals []pipelineApprovalResponse
	for rows.Next() {
		approval, err := scanApprovalResponse(rows)
		if err != nil {
			http.Error(w, "Failed to read approvals", http.StatusInternalServerError)
			return
		}
		approvals = append(approvals, approval)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Failed to read approvals", http.StatusInternalServerError)
		return
	}

	_ = httpapi.WriteJSON(w, http.StatusOK, approvals)
}

func (a *App) handleApproveRunApproval(w http.ResponseWriter, r *http.Request) {
	a.handleApprovalDecision(w, r, true)
}

func (a *App) handleRejectRunApproval(w http.ResponseWriter, r *http.Request) {
	a.handleApprovalDecision(w, r, false)
}

func (a *App) handleApprovalDecision(w http.ResponseWriter, r *http.Request, approved bool) {
	runID := strings.TrimSpace(r.PathValue("runID"))
	approvalID := strings.TrimSpace(r.PathValue("approvalID"))
	if _, err := uuid.Parse(runID); err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}
	if _, err := uuid.Parse(approvalID); err != nil {
		http.Error(w, "Invalid approval ID", http.StatusBadRequest)
		return
	}
	var req approvalDecisionRequest
	if err := httpapi.DecodeOptionalJSON(r, &req); err != nil {
		http.Error(w, "Invalid approval decision request", http.StatusBadRequest)
		return
	}
	comment, err := normalizeApprovalDecisionComment(approved, req.Comment)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Comment = comment

	record, err := a.loadApprovalForDecision(r.Context(), runID, approvalID)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "Approval not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to load approval", http.StatusInternalServerError)
		return
	}
	if record.Status != approvalStatusPending {
		http.Error(w, "Approval has already been decided", http.StatusConflict)
		return
	}
	if record.RunStatus != runStatusWaitingApproval {
		http.Error(w, "Run is not waiting for approval", http.StatusConflict)
		return
	}
	if record.ExpiresAt != nil && !time.Now().UTC().Before(record.ExpiresAt.UTC()) {
		if !a.authorizeApprovalDecision(w, r, record) {
			return
		}
		a.expireApprovalTimeouts(r.Context())
		http.Error(w, "Approval has timed out", http.StatusConflict)
		return
	}
	if strings.TrimSpace(record.CheckpointID) == "" || len(record.PipelineDef) == 0 {
		http.Error(w, "Approval checkpoint is missing", http.StatusConflict)
		return
	}
	if !a.authorizeApprovalDecision(w, r, record) {
		return
	}

	subject, _ := a.currentAAASubject(r)
	deciderType := strings.TrimSpace(subject.Type)
	deciderID := approvalSubjectID(subject)
	deciderEmail := strings.TrimSpace(subject.Email)

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "Failed to start approval decision", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	nextApprovalStatus := approvalStatusApproved
	nextRunStatus := "pending"
	taskStatus := "success"
	exitCode := 0
	failureReason := ""
	if !approved {
		nextApprovalStatus = approvalStatusRejected
		nextRunStatus = runStatusRejected
		taskStatus = "failure"
		exitCode = 1
		failureReason = firstNonEmptyString(req.Comment, "Approval rejected")
	}

	tag, err := tx.Exec(r.Context(), `
		UPDATE pipeline_approvals
		SET status = $1,
		    decided_by_type = $2,
		    decided_by_id = $3,
		    decided_by_email = $4,
		    decided_at = NOW(),
		    decision_comment = $5
		WHERE id = $6 AND run_id = $7 AND status = $8
	`, nextApprovalStatus, deciderType, deciderID, deciderEmail, req.Comment, approvalID, runID, approvalStatusPending)
	if err != nil {
		http.Error(w, "Failed to update approval", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "Approval has already been decided", http.StatusConflict)
		return
	}

	if _, err := tx.Exec(r.Context(), `
		UPDATE task_runs
		SET status = $1, exit_code = $2, started_at = COALESCE(started_at, NOW()), finished_at = NOW()
		WHERE run_id = $3 AND step_name = $4 AND task_name = $5
	`, taskStatus, exitCode, runID, record.StepName, record.TaskName); err != nil {
		http.Error(w, "Failed to update approval task", http.StatusInternalServerError)
		return
	}

	if approved {
		if _, err := tx.Exec(r.Context(), `
			UPDATE pipeline_runs
			SET status = $1, finished_at = NULL, failure_reason = NULL
			WHERE run_id = $2 AND status = $3
		`, nextRunStatus, runID, runStatusWaitingApproval); err != nil {
			http.Error(w, "Failed to update run status", http.StatusInternalServerError)
			return
		}
	} else {
		if _, err := tx.Exec(r.Context(), `
			UPDATE pipeline_runs
			SET status = $1, finished_at = NOW(), failure_reason = $2
			WHERE run_id = $3 AND status = $4
		`, nextRunStatus, failureReason, runID, runStatusWaitingApproval); err != nil {
			http.Error(w, "Failed to reject run", http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "Failed to commit approval decision", http.StatusInternalServerError)
		return
	}

	go a.notifyGitBotOfTaskStatus(runID, record.StepName, record.TaskName, taskStatus)
	if !approved {
		go a.dispatchPipelineRunNotification(runID, "approval_rejected")
		go a.notifyGitBotOfFinalStatus("failure", record.StepName, record.TaskName, failureReason, record.GitContext)
		_ = httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": nextRunStatus})
		return
	}
	go a.dispatchPipelineRunNotification(runID, "approval_approved")
	go a.dispatchPipelineRunNotification(runID, "pending")

	var pipeline models.Pipeline
	if err := yaml.Unmarshal(record.PipelineDef, &pipeline); err != nil {
		a.updateRunRecordWithFailure(uuid.MustParse(runID), "Failed to parse approval checkpoint pipeline definition", record.GitContext)
		http.Error(w, "Approval was recorded, but checkpoint pipeline definition is invalid", http.StatusInternalServerError)
		return
	}
	timeoutDuration := a.pipelineTimeoutDurationForResume(&pipeline)
	if timeoutDuration > 0 {
		if _, err := a.db.Exec(context.Background(), "UPDATE pipeline_runs SET timeout_at = $1 WHERE run_id = $2", time.Now().Add(timeoutDuration), runID); err != nil {
			log.Warn().Err(err).Str("run_id", runID).Msg("Failed to refresh timeout for approved run")
		}
	}

	go a.agentRunLauncher().LaunchAgent(context.Background(), AgentRunLaunchRequest{
		RunID:              runID,
		ParentRunID:        record.ParentRunID,
		ParentRunnerID:     record.RunnerID,
		Pipeline:           pipeline,
		PipelineDefinition: record.PipelineDef,
		Timeout:            timeoutDuration,
		GitContext:         record.GitContext,
		Scope:              record.Scope,
		ResumeCheckpointID: record.CheckpointID,
		ResumeVariables:    record.Variables,
	})
	_ = httpapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "approved", "run_status": "pending"})
}

func (a *App) loadApprovalForDecision(ctx context.Context, runID, approvalID string) (approvalDecisionRecord, error) {
	var record approvalDecisionRecord
	var teamsBytes, variablesBytes []byte
	var pipelineDefCheckpoint, pipelineDefRun sql.NullString
	var checkpointID, parentRunID, runnerID, scope sql.NullString
	var expiresAt sql.NullTime
	var repoOwner, repoName, cloneURL, sshURL, ref, targetRef, commitSHA, commitURL, commitMessage sql.NullString
	var commitAuthorName, commitAuthorEmail, commitAuthorUsername, pusherName, pusherEmail, checkRunID, triggerEventID sql.NullString
	err := a.db.QueryRow(ctx, `
		SELECT
			pa.id::text, pa.run_id::text, pa.step_name, pa.task_name, pa.approval_type,
			pa.assigned_teams, pa.allow_self_approval, pa.status, pa.requested_by_type,
			pa.requested_by_id, pa.expires_at, COALESCE(pa.checkpoint_id::text, ''),
			pr.status, pr.pipeline_name, pr.pipeline_path, pr.pipeline_version,
			COALESCE(pr.parent_run_id::text, ''), COALESCE(pr.scope, ''),
			pr.pipeline_definition, COALESCE(cp.pipeline_definition, ''), COALESCE(cp.variables, '{}'::jsonb), COALESCE(cp.runner_id, ''),
			COALESCE(pr.git_repo_owner, ''), COALESCE(pr.git_repo_name, ''),
			COALESCE(pr.git_clone_url, ''), COALESCE(pr.git_ssh_url, ''),
			COALESCE(pr.git_ref, ''), COALESCE(pr.git_target_ref, ''),
			COALESCE(pr.git_commit_sha, ''), COALESCE(pr.git_commit_url, ''),
			COALESCE(pr.git_commit_message, ''), COALESCE(pr.git_commit_author_name, ''),
			COALESCE(pr.git_commit_author_email, ''), COALESCE(pr.git_commit_author_username, ''),
			COALESCE(pr.git_pusher_name, ''), COALESCE(pr.git_pusher_email, ''),
			COALESCE(pr.git_check_run_id::text, ''), COALESCE(pr.trigger_event_id, '')
		FROM pipeline_approvals pa
		JOIN pipeline_runs pr ON pr.run_id = pa.run_id
		LEFT JOIN pipeline_run_checkpoints cp ON cp.id = pa.checkpoint_id
		WHERE pa.run_id = $1 AND pa.id = $2
	`, runID, approvalID).Scan(
		&record.ID, &record.RunID, &record.StepName, &record.TaskName, &record.ApprovalType,
		&teamsBytes, &record.AllowSelfApproval, &record.Status, &record.RequestedByType,
		&record.RequestedByID, &expiresAt, &checkpointID,
		&record.RunStatus, &record.PipelineName, &record.PipelinePath, &record.PipelineVersion,
		&parentRunID, &scope,
		&pipelineDefRun, &pipelineDefCheckpoint, &variablesBytes, &runnerID,
		&repoOwner, &repoName, &cloneURL, &sshURL, &ref, &targetRef, &commitSHA, &commitURL,
		&commitMessage, &commitAuthorName, &commitAuthorEmail, &commitAuthorUsername,
		&pusherName, &pusherEmail, &checkRunID, &triggerEventID,
	)
	if err != nil {
		return record, err
	}
	if expiresAt.Valid {
		record.ExpiresAt = &expiresAt.Time
	}
	record.CheckpointID = checkpointID.String
	record.ParentRunID = parentRunID.String
	record.Scope = scope.String
	record.RunnerID = runnerID.String
	record.PipelineDef = []byte(firstNonEmptyString(pipelineDefCheckpoint.String, pipelineDefRun.String))
	record.AssignedTeams = decodeApprovalTeams(teamsBytes)
	record.Variables = map[string]string{}
	if len(variablesBytes) > 0 {
		_ = json.Unmarshal(variablesBytes, &record.Variables)
	}
	record.GitContext = map[string]string{
		"repo_owner":             repoOwner.String,
		"repo_name":              repoName.String,
		"clone_url":              cloneURL.String,
		"ssh_url":                sshURL.String,
		"ref":                    ref.String,
		"target_ref":             targetRef.String,
		"commit_sha":             commitSHA.String,
		"commit_url":             commitURL.String,
		"commit_message":         commitMessage.String,
		"commit_author_name":     commitAuthorName.String,
		"commit_author_email":    commitAuthorEmail.String,
		"commit_author_username": commitAuthorUsername.String,
		"pusher_name":            pusherName.String,
		"pusher_email":           pusherEmail.String,
		"check_run_id":           checkRunID.String,
		"trigger_event_id":       triggerEventID.String,
	}
	return record, nil
}

type expiredApprovalRecord struct {
	RunID      string
	StepName   string
	TaskName   string
	GitContext map[string]string
}

func (a *App) expireApprovalTimeouts(ctx context.Context) {
	if a == nil || a.db == nil {
		return
	}
	records, err := a.expireApprovalTimeoutsNow(ctx, approvalTimeoutBatchSize)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to expire approval timeouts")
		return
	}
	for _, record := range records {
		go a.notifyGitBotOfTaskStatus(record.RunID, record.StepName, record.TaskName, taskStatusTimedOut)
		go a.dispatchPipelineRunNotification(record.RunID, runStatusTimedOut)
		go a.notifyGitBotOfFinalStatus(runStatusTimedOut, record.StepName, record.TaskName, approvalTimeoutComment, record.GitContext)
	}
}

func (a *App) expireApprovalTimeoutsNow(ctx context.Context, limit int) ([]expiredApprovalRecord, error) {
	if limit <= 0 {
		return nil, nil
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT
			pa.run_id::text, pa.step_name, pa.task_name,
			COALESCE(pr.git_repo_owner, ''), COALESCE(pr.git_repo_name, ''),
			COALESCE(pr.git_clone_url, ''), COALESCE(pr.git_ssh_url, ''),
			COALESCE(pr.git_ref, ''), COALESCE(pr.git_target_ref, ''),
			COALESCE(pr.git_commit_sha, ''), COALESCE(pr.git_commit_url, ''),
			COALESCE(pr.git_commit_message, ''), COALESCE(pr.git_commit_author_name, ''),
			COALESCE(pr.git_commit_author_email, ''), COALESCE(pr.git_commit_author_username, ''),
			COALESCE(pr.git_pusher_name, ''), COALESCE(pr.git_pusher_email, ''),
			COALESCE(pr.git_check_run_id::text, ''), COALESCE(pr.trigger_event_id, '')
		FROM pipeline_approvals pa
		JOIN pipeline_runs pr ON pr.run_id = pa.run_id
		WHERE pa.status = $1
		  AND pa.expires_at IS NOT NULL
		  AND pa.expires_at <= NOW()
		  AND pr.status = $2
		ORDER BY pa.expires_at ASC
		LIMIT $3
		FOR UPDATE OF pa SKIP LOCKED
	`, approvalStatusPending, runStatusWaitingApproval, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []expiredApprovalRecord
	for rows.Next() {
		var record expiredApprovalRecord
		var repoOwner, repoName, cloneURL, sshURL, ref, targetRef, commitSHA, commitURL, commitMessage string
		var commitAuthorName, commitAuthorEmail, commitAuthorUsername, pusherName, pusherEmail, checkRunID, triggerEventID string
		if err := rows.Scan(
			&record.RunID, &record.StepName, &record.TaskName,
			&repoOwner, &repoName, &cloneURL, &sshURL, &ref, &targetRef, &commitSHA, &commitURL,
			&commitMessage, &commitAuthorName, &commitAuthorEmail, &commitAuthorUsername,
			&pusherName, &pusherEmail, &checkRunID, &triggerEventID,
		); err != nil {
			return nil, err
		}
		record.GitContext = pendingRunRecoveryGitContext(map[string]string{
			"run_id":                 record.RunID,
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for _, record := range records {
		if _, err := tx.Exec(ctx, `
			UPDATE pipeline_approvals
			SET status = $1,
			    decided_at = NOW(),
			    decision_comment = $2
			WHERE run_id = $3 AND step_name = $4 AND status = $5
		`, approvalStatusTimedOut, approvalTimeoutComment, record.RunID, record.StepName, approvalStatusPending); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE task_runs
			SET status = $1,
			    exit_code = 1,
			    started_at = COALESCE(started_at, NOW()),
			    finished_at = NOW()
			WHERE run_id = $2 AND step_name = $3 AND task_name = $4
		`, taskStatusTimedOut, record.RunID, record.StepName, record.TaskName); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE pipeline_runs
			SET status = $1,
			    finished_at = NOW(),
			    failure_reason = $2
			WHERE run_id = $3 AND status = $4
		`, runStatusTimedOut, approvalTimeoutComment, record.RunID, runStatusWaitingApproval); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return records, nil
}

func (a *App) authorizeApprovalDecision(w http.ResponseWriter, r *http.Request, record approvalDecisionRecord) bool {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "missing authorization subject", http.StatusUnauthorized)
		return false
	}
	subjectID := approvalSubjectID(subject)
	if !record.AllowSelfApproval &&
		strings.EqualFold(record.RequestedByType, subject.Type) &&
		strings.EqualFold(record.RequestedByID, subjectID) {
		http.Error(w, "self approval is not allowed for this step", http.StatusForbidden)
		return false
	}
	if !a.aaaAvailable() {
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return false
	}
	for _, team := range record.AssignedTeams {
		resource := aaamodel.ResourceRef{Type: grantResourceTeam, ID: team}
		decision, err := a.aaaCheck(r.Context(), subject, approvalActionApprove, resource, a.aaaRequestContext(r))
		if err != nil {
			http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
			return false
		}
		if decision.Allowed {
			return true
		}
	}
	http.Error(w, "forbidden", http.StatusForbidden)
	return false
}

func (a *App) canReadRunOrApprove(r *http.Request, runID string) (bool, error) {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		return false, fmt.Errorf("missing authorization subject")
	}
	if !a.aaaAvailable() {
		return false, fmt.Errorf("authorization unavailable")
	}

	resource := routeauthz.RunResource(runID)
	decision, err := a.aaaCheck(r.Context(), subject, "pipeline_run.read", resource, a.aaaRequestContext(r))
	if err != nil {
		return false, err
	}
	if decision.Allowed {
		return true, nil
	}

	approvableSet, err := a.approvableRunSetForSubject(r.Context(), subject, a.aaaRequestContext(r), []aaamodel.ResourceRef{resource})
	if err != nil {
		return false, err
	}
	_, ok = approvableSet[resourceKey(resource)]
	return ok, nil
}

func (a *App) approvableRunSet(r *http.Request, runResources []aaamodel.ResourceRef) (map[string]struct{}, error) {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		return nil, fmt.Errorf("missing authorization subject")
	}
	if !a.aaaAvailable() {
		return nil, fmt.Errorf("authorization unavailable")
	}
	return a.approvableRunSetForSubject(r.Context(), subject, a.aaaRequestContext(r), runResources)
}

func (a *App) approvableRunSetForSubject(ctx context.Context, subject aaamodel.Subject, requestContext map[string]any, runResources []aaamodel.ResourceRef) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	if len(runResources) == 0 {
		return out, nil
	}

	approvals, err := a.loadPendingApprovalVisibility(ctx, runResources)
	if err != nil {
		return nil, err
	}
	return a.approvableRunSetFromApprovals(ctx, subject, requestContext, approvals)
}

func (a *App) approvableRunSetFromApprovals(ctx context.Context, subject aaamodel.Subject, requestContext map[string]any, approvals []pendingApprovalVisibility) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	teamDecisionCache := map[string]bool{}
	subjectID := approvalSubjectID(subject)
	for _, approval := range approvals {
		if !approval.AllowSelfApproval &&
			strings.EqualFold(approval.RequestedByType, subject.Type) &&
			strings.EqualFold(approval.RequestedByID, subjectID) {
			continue
		}
		for _, team := range approval.AssignedTeams {
			cacheKey := strings.ToLower(strings.TrimSpace(team))
			allowed, ok := teamDecisionCache[cacheKey]
			if !ok {
				decision, err := a.aaaCheck(ctx, subject, approvalActionApprove, aaamodel.ResourceRef{Type: grantResourceTeam, ID: team}, requestContext)
				if err != nil {
					return nil, err
				}
				allowed = decision.Allowed
				teamDecisionCache[cacheKey] = allowed
			}
			if allowed {
				out[resourceKey(routeauthz.RunResource(approval.RunID))] = struct{}{}
				break
			}
		}
	}
	return out, nil
}

func (a *App) loadPendingApprovalVisibility(ctx context.Context, runResources []aaamodel.ResourceRef) ([]pendingApprovalVisibility, error) {
	runIDs := make([]string, 0, len(runResources))
	seen := map[string]struct{}{}
	for _, resource := range runResources {
		if strings.TrimSpace(resource.Type) != "pipeline_run" {
			continue
		}
		runID := strings.TrimSpace(resource.ID)
		if runID == "" {
			continue
		}
		if _, ok := seen[runID]; ok {
			continue
		}
		seen[runID] = struct{}{}
		runIDs = append(runIDs, runID)
	}
	if len(runIDs) == 0 {
		return nil, nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT pa.run_id::text, pa.assigned_teams, pa.allow_self_approval,
		       pa.requested_by_type, pa.requested_by_id
		FROM pipeline_approvals pa
		JOIN pipeline_runs pr ON pr.run_id = pa.run_id
		WHERE pa.status = 'pending'
		  AND pr.status = $1
		  AND pa.run_id::text = ANY($2)
	`, runStatusWaitingApproval, runIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var approvals []pendingApprovalVisibility
	for rows.Next() {
		var approval pendingApprovalVisibility
		var teamsBytes []byte
		if err := rows.Scan(
			&approval.RunID,
			&teamsBytes,
			&approval.AllowSelfApproval,
			&approval.RequestedByType,
			&approval.RequestedByID,
		); err != nil {
			return nil, err
		}
		approval.AssignedTeams = decodeApprovalTeams(teamsBytes)
		approvals = append(approvals, approval)
	}
	return approvals, rows.Err()
}

func (a *App) pipelineTimeoutDurationForResume(pipeline *models.Pipeline) time.Duration {
	timeoutStr := ""
	if pipeline != nil {
		timeoutStr = strings.TrimSpace(pipeline.Timeout)
	}
	if timeoutStr == "" {
		timeoutStr = a.getDefaultPipelineTimeout()
	}
	if timeoutStr == "" {
		return 0
	}
	duration, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return 0
	}
	return duration
}

func (a *App) completedTaskKeysForRun(ctx context.Context, runID string) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT step_name, task_name
		FROM task_runs
		WHERE run_id = $1
		  AND status IN ('success', 'skipped', 'failure (ignored)')
		ORDER BY task_index ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var stepName, taskName string
		if err := rows.Scan(&stepName, &taskName); err != nil {
			return nil, err
		}
		keys = append(keys, fmt.Sprintf("%s/%s", stepName, taskName))
	}
	return keys, rows.Err()
}

func scanApprovalResponse(rows pgx.Rows) (pipelineApprovalResponse, error) {
	var approval pipelineApprovalResponse
	var teamsBytes []byte
	var decidedAt sql.NullTime
	var expiresAt sql.NullTime
	err := rows.Scan(
		&approval.ID, &approval.RunID, &approval.StepName, &approval.TaskName, &approval.ApprovalType,
		&teamsBytes, &approval.AllowSelfApproval, &approval.Status, &approval.RequestedByType,
		&approval.RequestedByID, &approval.RequestedAt, &approval.DecidedByType, &approval.DecidedByID,
		&approval.DecidedByEmail, &decidedAt, &approval.DecisionComment, &expiresAt, &approval.CheckpointID,
	)
	if err != nil {
		return approval, err
	}
	approval.AssignedTeams = decodeApprovalTeams(teamsBytes)
	if decidedAt.Valid {
		approval.DecidedAt = &decidedAt.Time
	}
	if expiresAt.Valid {
		approval.ExpiresAt = &expiresAt.Time
	}
	return approval, nil
}

func approvalDefinitionForStep(pipeline *models.Pipeline, stepName string) (models.ApprovalDefinition, bool) {
	if pipeline == nil {
		return models.ApprovalDefinition{}, false
	}
	for _, step := range pipeline.Steps {
		if step.GetName() != stepName {
			continue
		}
		if approvalStep, ok := step.AsApprovalStep(); ok {
			approval := approvalStep.Approval
			approval.Teams = normalizeApprovalTeams(approval.Teams)
			return approval, true
		}
	}
	return models.ApprovalDefinition{}, false
}

func requireInternalServiceRole(w http.ResponseWriter, r *http.Request, role string) bool {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		http.Error(w, "missing claims", http.StatusUnauthorized)
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(claims.Provider), serviceauth.ProviderInternalService) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	for _, claimRole := range claims.Roles {
		if strings.EqualFold(strings.TrimSpace(claimRole), role) {
			return true
		}
	}
	http.Error(w, "forbidden", http.StatusForbidden)
	return false
}

func normalizeApprovalTeams(teams []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(teams))
	for _, team := range teams {
		normalized := strings.Trim(strings.ReplaceAll(strings.TrimSpace(team), "\\", "/"), "/")
		if normalized == "" {
			continue
		}
		key := strings.ToLower(normalized)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func decodeApprovalTeams(raw []byte) []string {
	var teams []string
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &teams)
	}
	return normalizeApprovalTeams(teams)
}

func normalizeCompletedTaskKeys(keys []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" || !strings.Contains(trimmed, "/") {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func approvalSubjectID(subject aaamodel.Subject) string {
	return firstNonEmptyString(subject.ID, subject.Sub, subject.Email)
}

func decodeOptionalBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(value)
}

package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"

	"nopsai/pkg/correlation"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/serviceauth"
	runquery "nopsai/services/nopsai/internal/runs"
	"nopsai/services/nopsai/pkg/auth"
	"nopsai/services/nopsai/pkg/routeauthz"
)

func (a *App) handleTaskUpdate(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	stepName := r.PathValue("stepName")
	taskName := r.PathValue("taskName")

	var update StepStatusUpdate
	if err := httpapi.DecodeJSON(r, &update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var runStatus string
	if err := a.db.QueryRow(context.Background(), "SELECT status FROM pipeline_runs WHERE run_id = $1", runID).Scan(&runStatus); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to load run status for task update")
		http.Error(w, "Failed to update task status", http.StatusInternalServerError)
		return
	}

	runStatus = strings.ToLower(strings.TrimSpace(runStatus))
	if isTerminalRunStatus(runStatus) {
		if runStatus == "cancelled" && strings.EqualFold(update.Status, "cancelled") {
			query := `
				UPDATE task_runs
				SET status = 'cancelled', finished_at = COALESCE(finished_at, NOW())
				WHERE run_id = $1 AND step_name = $2 AND task_name = $3
				  AND status NOT IN ('success', 'warning', 'failure', 'failure (ignored)', 'skipped', 'cancelled')`
			if _, err := a.db.Exec(context.Background(), query, runID, stepName, taskName); err != nil {
				log.Error().Err(err).Str("run_id", runID).Str("step", stepName).Str("task", taskName).Msg("Failed to preserve cancelled task status")
				http.Error(w, "Failed to update task status", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		log.Info().
			Str("run_id", runID).
			Str("step", stepName).
			Str("task", taskName).
			Str("status", update.Status).
			Str("run_status", runStatus).
			Msg("Ignoring late task status update for terminal run")
		w.WriteHeader(http.StatusOK)
		return
	}

	normalizedUpdateStatus := runquery.NormalizeRunDetailStatus(update.Status)
	if normalizedUpdateStatus == taskStatusWaitingApproval {
		query := `
			UPDATE task_runs
			SET status = $1, started_at = COALESCE(started_at, NOW()), finished_at = NULL, exit_code = NULL
			WHERE run_id = $2 AND step_name = $3 AND task_name = $4`
		if _, err := a.db.Exec(context.Background(), query, taskStatusWaitingApproval, runID, stepName, taskName); err != nil {
			log.Error().Err(err).Str("run_id", runID).Str("step", stepName).Str("task", taskName).Msg("Failed to update task waiting approval status")
			http.Error(w, "Failed to update task status", http.StatusInternalServerError)
			return
		}
	} else if update.Status == "running" {
		tx, err := a.db.Begin(context.Background())
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Msg("Failed to start transaction for task update")
			http.Error(w, "Failed to update task status", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(context.Background())

		_, err = tx.Exec(context.Background(), "UPDATE task_runs SET status = 'running', started_at = NOW() WHERE run_id = $1 AND step_name = $2 AND task_name = $3", runID, stepName, taskName)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Str("step", stepName).Str("task", taskName).Msg("Failed to update task start time")
			http.Error(w, "Failed to update task status", http.StatusInternalServerError)
			return
		}

		err = markRunRunning(context.Background(), tx, runID)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Msg("Failed to update run start time")
			http.Error(w, "Failed to update task status", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(context.Background()); err != nil {
			log.Error().Err(err).Str("run_id", runID).Msg("Failed to commit transaction for task update")
			http.Error(w, "Failed to update task status", http.StatusInternalServerError)
			return
		}
		go a.dispatchPipelineRunNotification(runID, "running")
	} else {
		query := "UPDATE task_runs SET status = $1, exit_code = $2, started_at = COALESCE(started_at, NOW()), finished_at = NOW() WHERE run_id = $3 AND step_name = $4 AND task_name = $5"
		_, err := a.db.Exec(context.Background(), query, update.Status, update.ExitCode, runID, stepName, taskName)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Str("step", stepName).Str("task", taskName).Msg("Failed to update task finish status")
			http.Error(w, "Failed to update task status", http.StatusInternalServerError)
			return
		}
	}

	log.Info().Str("run_id", runID).Str("step", stepName).Str("task", taskName).Str("status", update.Status).Msg("Updated task status")

	go a.notifyGitBotOfTaskStatus(runID, stepName, taskName, update.Status)

	w.WriteHeader(http.StatusOK)
}

func (a *App) handleFinalizeRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	var req FinalizeRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Info().Str("run_id", runID).Str("status", req.Status).Msg("Received final status from agent")

	var currentStatus string
	if err := a.db.QueryRow(context.Background(), "SELECT status FROM pipeline_runs WHERE run_id = $1", runID).Scan(&currentStatus); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to load run before finalization")
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}

	currentStatus = strings.ToLower(strings.TrimSpace(currentStatus))
	if currentStatus == "cancelled" || currentStatus == runStatusWaitingApproval {
		log.Info().Str("run_id", runID).Str("status", req.Status).Str("current_status", currentStatus).Msg("Ignoring final status because run is not finalizable")
		w.WriteHeader(http.StatusOK)
		return
	}
	if isTerminalRunStatus(currentStatus) {
		log.Info().Str("run_id", runID).Str("status", req.Status).Str("current_status", currentStatus).Msg("Ignoring final status for completed run")
		w.WriteHeader(http.StatusOK)
		return
	}

	finalStatus := normalizeFinalizeRunStatus(req.Status)
	var failedStep, failedTask string
	if finalStatus == "failure" {
		err := a.db.QueryRow(context.Background(), "SELECT step_name, task_name FROM task_runs WHERE run_id = $1 AND status NOT IN ('success', 'warning', 'pending', 'skipped', 'failure (ignored)', 'running') ORDER BY finished_at ASC, started_at ASC LIMIT 1", runID).Scan(&failedStep, &failedTask)
		if err != nil {
			log.Warn().Err(err).Str("run_id", runID).Msg("Could not determine the exact failed task for final status notification.")
		}
	}

	var gitContext = make(map[string]string)
	var repoOwner, repoName, commitSHA sql.NullString
	var checkRunID sql.NullInt64
	query := `SELECT git_repo_owner, git_repo_name, git_commit_sha, git_check_run_id FROM pipeline_runs WHERE run_id = $1`
	err := a.db.QueryRow(context.Background(), query, runID).Scan(&repoOwner, &repoName, &commitSHA, &checkRunID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to retrieve git context for final notification")
	} else {
		if repoOwner.Valid {
			gitContext["repo_owner"] = repoOwner.String
		}
		if repoName.Valid {
			gitContext["repo_name"] = repoName.String
		}
		if commitSHA.Valid {
			gitContext["commit_sha"] = commitSHA.String
		}
		if checkRunID.Valid {
			gitContext["check_run_id"] = strconv.FormatInt(checkRunID.Int64, 10)
		}
	}

	failureReason := ""
	if finalStatus == "failure" {
		failureReason = strings.TrimSpace(req.FailureReason)
	}

	tx, err := a.db.Begin(context.Background())
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to begin final run status transaction")
		http.Error(w, "Failed to update run status", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(context.Background())

	if err := closeIncompleteTasksForFinalStatus(context.Background(), tx, runID, finalStatus); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to close incomplete tasks for final run status")
		http.Error(w, "Failed to update run status", http.StatusInternalServerError)
		return
	}

	_, err = tx.Exec(context.Background(), `
		UPDATE pipeline_runs
		SET status = $1::varchar,
		    finished_at = COALESCE(finished_at, NOW()),
		    failure_reason = CASE
		        WHEN $1::varchar = 'failure' THEN COALESCE(NULLIF($3::text, ''), failure_reason)
		        ELSE NULL
		    END
		WHERE run_id = $2::uuid AND status != 'cancelled'
	`, finalStatus, runID, failureReason)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to update final run status in DB from agent notification")
		http.Error(w, "Failed to update run status", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(context.Background()); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to commit final run status transaction")
		http.Error(w, "Failed to update run status", http.StatusInternalServerError)
		return
	}

	if err := a.preparePipelineFinalOutputRecords(context.Background(), runID); err != nil {
		log.Warn().Err(err).Str("run_id", runID).Msg("Failed to prepare final outputs after run finalization")
	} else {
		go a.generatePipelineFinalOutputs(context.Background(), runID)
	}

	if gitContext["repo_owner"] != "" {
		// Run git-bot notification in background to prevent agent hang
		go a.notifyGitBotOfFinalStatus(finalStatus, failedStep, failedTask, failureReason, gitContext)
	}
	go a.dispatchPipelineRunNotification(runID, finalStatus)

	w.WriteHeader(http.StatusOK)
}

func closeIncompleteTasksForFinalStatus(ctx context.Context, exec interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, runID, finalStatus string) error {
	switch runquery.NormalizeRunDetailStatus(finalStatus) {
	case "failure":
		_, err := exec.Exec(ctx, `
			UPDATE task_runs
			SET status = CASE
			        WHEN started_at IS NULL THEN 'skipped'
			        ELSE 'failure'
			    END,
			    exit_code = CASE
			        WHEN started_at IS NULL THEN exit_code
			        ELSE COALESCE(exit_code, 1)
			    END,
			    finished_at = CASE
			        WHEN started_at IS NULL THEN finished_at
			        ELSE COALESCE(finished_at, NOW())
			    END
			WHERE run_id = $1
			  AND status NOT IN ('success', 'warning', 'failure', 'failure (ignored)', 'skipped', 'cancelled', 'rejected')`, runID)
		return err
	case "cancelled", "rejected":
		_, err := exec.Exec(ctx, `
			UPDATE task_runs
			SET status = CASE
			        WHEN started_at IS NULL THEN 'skipped'
			        ELSE 'cancelled'
			    END,
			    finished_at = CASE
			        WHEN started_at IS NULL THEN finished_at
			        ELSE COALESCE(finished_at, NOW())
			    END
			WHERE run_id = $1
			  AND status NOT IN ('success', 'warning', 'failure', 'failure (ignored)', 'skipped', 'cancelled', 'rejected')`, runID)
		return err
	default:
		return nil
	}
}

func (a *App) handleGetRunStatus(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	var status string
	err := a.db.QueryRow(context.Background(), "SELECT status FROM pipeline_runs WHERE run_id = $1", runID).Scan(&status)
	if err != nil {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": status})
}

func (a *App) handleGetRunByCheckID(w http.ResponseWriter, r *http.Request) {
	checkRunID := r.PathValue("checkRunID")
	var runID string
	// Find the latest run for this check_run_id, as there could be multiple re-runs
	err := a.db.QueryRow(context.Background(),
		"SELECT run_id FROM pipeline_runs WHERE git_check_run_id = $1 ORDER BY created_at DESC LIMIT 1",
		checkRunID).Scan(&runID)
	if err != nil {
		http.Error(w, "Run not found for this check run ID", http.StatusNotFound)
		return
	}
	if !a.requireAAADecision(w, r, "pipeline_run.read", routeauthz.RunResource(runID)) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"run_id": runID})
}

func (a *App) handleIngestLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	runID := strings.TrimSpace(r.PathValue("runID"))
	if runID == "" {
		http.Error(w, "Run ID is required", http.StatusBadRequest)
		return
	}
	if _, err := uuid.Parse(runID); err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	var payload runLogIngestPayload
	if err := httpapi.DecodeJSON(r, &payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(payload.Lines) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	payload = normalizeRunLogIngestPayload(r, payload)
	payload.Lines = filterRunLogIngestLines(payload)
	if len(payload.Lines) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	metadataJSON, err := json.Marshal(payload.Metadata)
	if err != nil {
		http.Error(w, "Invalid log metadata", http.StatusBadRequest)
		return
	}

	batch := &pgx.Batch{}
	for _, line := range payload.Lines {
		line = redactRunLogLine(line)
		lineFields := runLogFieldsForLine(payload, line)
		batch.Queue(`
			INSERT INTO pipeline_run_logs (
				run_id, line, source, stream, level, step_name, task_name, runner_id,
				request_id, traceparent, metadata
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb)
		`, runID, line, payload.Source, payload.Stream, lineFields.Level, lineFields.StepName, lineFields.TaskName, payload.RunnerID, payload.RequestID, payload.Traceparent, metadataJSON)
	}
	br := a.db.SendBatch(r.Context(), batch)
	if err := br.Close(); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to ingest log batch")
		http.Error(w, "Failed to persist logs", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleGetRunLogs(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	sinceLineStr := r.URL.Query().Get("since_line")
	var lastID int64 = 0
	if sinceLineStr != "" {
		if parsed, err := strconv.ParseInt(sinceLineStr, 10, 64); err == nil {
			lastID = parsed
		}
	}
	includeChildren := includeChildRunLogs(r)

	rows, err := a.db.Query(r.Context(), `
		WITH RECURSIVE visible_runs AS (
			SELECT run_id, pipeline_name, parent_run_id, parent_step_name, 0 AS depth
			FROM pipeline_runs
			WHERE run_id::text = $1
			UNION ALL
			SELECT child.run_id, child.pipeline_name, child.parent_run_id, child.parent_step_name, parent.depth + 1
			FROM pipeline_runs child
			JOIN visible_runs parent ON child.parent_run_id = parent.run_id
			WHERE $3::boolean AND parent.depth < 8
		)
		SELECT logs.id, logs.timestamp, logs.line, logs.source, logs.stream, logs.level,
		       logs.step_name, logs.task_name, logs.runner_id, logs.request_id, logs.traceparent, logs.metadata,
		       visible_runs.run_id::text, COALESCE(visible_runs.pipeline_name, ''),
		       COALESCE(visible_runs.parent_run_id::text, ''), COALESCE(visible_runs.parent_step_name, '')
		FROM pipeline_run_logs logs
		JOIN visible_runs ON visible_runs.run_id = logs.run_id
		WHERE logs.id > $2
		ORDER BY logs.id ASC
	`, runID, lastID, includeChildren)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to query logs for run")
		http.Error(w, "Failed to retrieve logs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var logs []LogLine
	for rows.Next() {
		var logLine LogLine
		var metadataJSON []byte
		if err := rows.Scan(
			&logLine.ID,
			&logLine.Timestamp,
			&logLine.Line,
			&logLine.Source,
			&logLine.Stream,
			&logLine.Level,
			&logLine.StepName,
			&logLine.TaskName,
			&logLine.RunnerID,
			&logLine.RequestID,
			&logLine.Traceparent,
			&metadataJSON,
			&logLine.RunID,
			&logLine.PipelineName,
			&logLine.ParentRunID,
			&logLine.ParentStepName,
		); err != nil {
			log.Error().Err(err).Msg("Failed to scan log line")
			continue
		}
		if len(metadataJSON) > 0 && string(metadataJSON) != "{}" {
			_ = json.Unmarshal(metadataJSON, &logLine.Metadata)
		}
		logs = append(logs, logLine)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func includeChildRunLogs(r *http.Request) bool {
	if r == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("include_children"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

type runLogIngestPayload struct {
	Lines       []string       `json:"lines"`
	Source      string         `json:"source,omitempty"`
	Stream      string         `json:"stream,omitempty"`
	Level       string         `json:"level,omitempty"`
	StepName    string         `json:"step_name,omitempty"`
	TaskName    string         `json:"task_name,omitempty"`
	RunnerID    string         `json:"runner_id,omitempty"`
	RequestID   string         `json:"request_id,omitempty"`
	Traceparent string         `json:"traceparent,omitempty"`
	ServiceID   string         `json:"service_id,omitempty"`
	ServiceRole string         `json:"service_role,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

func normalizeRunLogIngestPayload(r *http.Request, payload runLogIngestPayload) runLogIngestPayload {
	payload.Source = strings.TrimSpace(payload.Source)
	payload.Stream = strings.ToLower(strings.TrimSpace(payload.Stream))
	payload.Level = normalizeRunLogLevelValue(payload.Level)
	payload.StepName = strings.TrimSpace(payload.StepName)
	payload.TaskName = strings.TrimSpace(payload.TaskName)
	payload.RunnerID = strings.TrimSpace(payload.RunnerID)
	payload.RequestID = strings.TrimSpace(payload.RequestID)
	payload.Traceparent = strings.TrimSpace(payload.Traceparent)
	payload.ServiceID = strings.TrimSpace(payload.ServiceID)
	payload.ServiceRole = strings.TrimSpace(payload.ServiceRole)
	if payload.Metadata == nil {
		payload.Metadata = map[string]any{}
	}
	if r != nil {
		if payload.RequestID == "" {
			payload.RequestID = requestIDFromContext(r.Context())
		}
		if payload.Traceparent == "" {
			payload.Traceparent = correlation.TraceparentFromContext(r.Context())
		}
		if claims, ok := auth.ClaimsFromContext(r.Context()); ok && claims != nil {
			if payload.ServiceID == "" {
				payload.ServiceID = claims.Sub
			}
			if payload.ServiceRole == "" && len(claims.Roles) > 0 {
				payload.ServiceRole = claims.Roles[0]
			}
			if payload.Source == "" {
				payload.Source = payload.ServiceRole
				if payload.Source == "" {
					payload.Source = claims.Sub
				}
			}
			if payload.Metadata["ingested_by"] == nil {
				payload.Metadata["ingested_by"] = claims.Sub
			}
			if payload.Metadata["ingested_by_provider"] == nil {
				payload.Metadata["ingested_by_provider"] = claims.Provider
			}
		}
	}
	if payload.Source == "" {
		payload.Source = "unknown"
	}
	if payload.ServiceID != "" && payload.Metadata["service_id"] == nil {
		payload.Metadata["service_id"] = payload.ServiceID
	}
	if payload.ServiceRole != "" && payload.Metadata["service_role"] == nil {
		payload.Metadata["service_role"] = payload.ServiceRole
	}
	return payload
}

func filterRunLogIngestLines(payload runLogIngestPayload) []string {
	if len(payload.Lines) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(payload.Lines))
	for _, line := range payload.Lines {
		if shouldDropRunLogIngestLine(payload, line) {
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
}

func shouldDropRunLogIngestLine(payload runLogIngestPayload, line string) bool {
	if !isAgentRunLogPayload(payload) || !strings.Contains(line, "grpc_client_request") {
		return false
	}

	parsed := parseStructuredRunLogFields(line)
	if len(parsed) == 0 {
		return true
	}
	message := strings.TrimSpace(structuredRunLogString(parsed, "message", "msg"))
	if message != "" && message != "grpc_client_request" {
		return false
	}
	level := normalizeRunLogLevelValue(structuredRunLogString(parsed, "output_level", "severity", "level"))
	if level == "" {
		level = payload.Level
	}
	if level == "warn" || level == "error" {
		return false
	}
	code := strings.ToUpper(strings.TrimSpace(structuredRunLogString(parsed, "grpc_code")))
	return code == "" || code == "OK"
}

func isAgentRunLogPayload(payload runLogIngestPayload) bool {
	source := strings.ToLower(strings.TrimSpace(payload.Source))
	role := strings.ToLower(strings.TrimSpace(payload.ServiceRole))
	return source == serviceauth.RoleAgent || role == serviceauth.RoleAgent
}

type inferredRunLogFields struct {
	Level    string
	StepName string
	TaskName string
}

func runLogFieldsForLine(payload runLogIngestPayload, line string) inferredRunLogFields {
	fields := inferredRunLogFields{
		Level:    payload.Level,
		StepName: payload.StepName,
		TaskName: payload.TaskName,
	}
	parsed := parseStructuredRunLogFields(line)
	if fields.Level == "" {
		fields.Level = normalizeRunLogLevelValue(structuredRunLogString(parsed, "output_level", "severity", "level"))
	}
	if fields.StepName == "" {
		fields.StepName = structuredRunLogString(parsed, "step", "step_name")
	}
	if fields.TaskName == "" {
		fields.TaskName = structuredRunLogString(parsed, "task", "task_name")
	}
	if fields.Level == "" {
		fields.Level = inferPlainTextRunLogLevel(line)
	}
	if fields.Level == "" {
		fields.Level = "info"
	}
	return fields
}

func parseStructuredRunLogFields(line string) map[string]any {
	jsonStart := strings.Index(line, "{")
	if jsonStart == -1 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(line[jsonStart:]), &payload); err != nil {
		return nil
	}
	return payload
}

func structuredRunLogString(payload map[string]any, keys ...string) string {
	if len(payload) == 0 {
		return ""
	}
	if value := firstRunLogString(payload, keys...); value != "" {
		return value
	}
	if meta, ok := payload["meta"].(map[string]any); ok {
		if value := firstRunLogString(meta, keys...); value != "" {
			return value
		}
	}
	if message, ok := payload["message"].(string); ok {
		if nested := parseStructuredRunLogFields(message); len(nested) > 0 {
			if value := firstRunLogString(nested, keys...); value != "" {
				return value
			}
			if meta, ok := nested["meta"].(map[string]any); ok {
				return firstRunLogString(meta, keys...)
			}
		}
	}
	return ""
}

func firstRunLogString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			if str, ok := value.(string); ok && strings.TrimSpace(str) != "" {
				return str
			}
		}
	}
	return ""
}

func normalizeRunLogLevelValue(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "info", "warn", "error", "debug":
		return strings.ToLower(strings.TrimSpace(level))
	case "warning":
		return "warn"
	case "fatal", "panic":
		return "error"
	case "trace":
		return "debug"
	default:
		return ""
	}
}

func inferPlainTextRunLogLevel(line string) string {
	for _, field := range strings.FieldsFunc(strings.ToLower(line), func(r rune) bool {
		return !unicode.IsLetter(r)
	}) {
		switch field {
		case "info", "warn", "error", "debug":
			return field
		case "warning":
			return "warn"
		case "fatal", "panic":
			return "error"
		case "trace":
			return "debug"
		}
	}
	return ""
}

package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"

	"nopsai/pkg/httpapi"
	runquery "nopsai/services/nopsai/internal/runs"
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
				  AND status NOT IN ('success', 'failure', 'failure (ignored)', 'skipped', 'cancelled')`
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
		err := a.db.QueryRow(context.Background(), "SELECT step_name, task_name FROM task_runs WHERE run_id = $1 AND status NOT IN ('success', 'pending', 'skipped', 'failure (ignored)', 'running') ORDER BY finished_at ASC, started_at ASC LIMIT 1", runID).Scan(&failedStep, &failedTask)
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
	case "failure", "failure (ignored)":
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
			  AND status NOT IN ('success', 'failure', 'failure (ignored)', 'skipped', 'cancelled', 'rejected')`, runID)
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
			  AND status NOT IN ('success', 'failure', 'failure (ignored)', 'skipped', 'cancelled', 'rejected')`, runID)
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

	var payload struct {
		Lines []string `json:"lines"`
	}
	if err := httpapi.DecodeJSON(r, &payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(payload.Lines) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	batch := &pgx.Batch{}
	for _, line := range payload.Lines {
		batch.Queue("INSERT INTO pipeline_run_logs (run_id, line) VALUES ($1, $2)", runID, line)
	}
	br := a.db.SendBatch(context.Background(), batch)
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

	rows, err := a.db.Query(context.Background(), "SELECT id, timestamp, line FROM pipeline_run_logs WHERE run_id = $1 AND id > $2 ORDER BY id ASC", runID, lastID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to query logs for run")
		http.Error(w, "Failed to retrieve logs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var logs []LogLine
	for rows.Next() {
		var logLine LogLine
		if err := rows.Scan(&logLine.ID, &logLine.Timestamp, &logLine.Line); err != nil {
			log.Error().Err(err).Msg("Failed to scan log line")
			continue
		}
		logs = append(logs, logLine)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

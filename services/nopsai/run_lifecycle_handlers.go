package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"
)

var errRunAlreadyCompleted = errors.New("run has already completed")

func (a *App) cancelRunHierarchy(ctx context.Context, runUUID uuid.UUID, reason, childReason string) error {
	nextReason := strings.TrimSpace(childReason)
	if nextReason == "" {
		nextReason = reason
	}
	_, err := a.markRunCancelled(ctx, runUUID, reason)
	if err != nil && !errors.Is(err, errRunAlreadyCompleted) {
		return err
	}
	if errors.Is(err, errRunAlreadyCompleted) {
		return err
	}

	rows, queryErr := a.db.Query(ctx, "SELECT run_id FROM pipeline_runs WHERE parent_run_id = $1", runUUID)
	if queryErr != nil {
		return queryErr
	}
	defer rows.Close()

	var childRunIDs []uuid.UUID
	for rows.Next() {
		var childRunID uuid.UUID
		if scanErr := rows.Scan(&childRunID); scanErr != nil {
			return scanErr
		}
		childRunIDs = append(childRunIDs, childRunID)
	}
	if rows.Err() != nil {
		return rows.Err()
	}

	for _, childRunID := range childRunIDs {
		if childErr := a.cancelRunHierarchy(ctx, childRunID, nextReason, nextReason); childErr != nil && !errors.Is(childErr, errRunAlreadyCompleted) {
			return childErr
		}
	}

	return nil
}

func (a *App) markRunCancelled(ctx context.Context, runUUID uuid.UUID, reason string) (bool, error) {
	var status string
	var repoName, repoOwner, commitSHA sql.NullString
	var checkRunID sql.NullInt64

	err := a.db.QueryRow(ctx, `
		SELECT status, git_repo_name, git_repo_owner, git_commit_sha, git_check_run_id
		FROM pipeline_runs
		WHERE run_id = $1`, runUUID).Scan(&status, &repoName, &repoOwner, &commitSHA, &checkRunID)
	if err != nil {
		return false, err
	}

	statusLower := strings.ToLower(strings.TrimSpace(status))
	if isCompletedRunStatus(statusLower) {
		return false, errRunAlreadyCompleted
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	changed := statusLower != "cancelled"
	if changed {
		if _, err := tx.Exec(ctx, "UPDATE pipeline_runs SET status = 'cancelled', finished_at = COALESCE(finished_at, NOW()) WHERE run_id = $1", runUUID); err != nil {
			return false, err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO pipeline_run_logs (run_id, line, source, level) VALUES ($1, $2, $3, $4)", runUUID, reason, "nopsai", "warn"); err != nil {
			log.Warn().Err(err).Str("run_id", runUUID.String()).Msg("Failed to record cancellation log line")
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE task_runs
		SET status = 'cancelled', finished_at = COALESCE(finished_at, NOW())
		WHERE run_id = $1
		  AND status NOT IN ('success', 'warning', 'failure', 'failure (ignored)', 'skipped', 'cancelled')`, runUUID); err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}

	if changed && checkRunID.Valid {
		gitContext := map[string]string{
			"repo_owner":   repoOwner.String,
			"repo_name":    repoName.String,
			"commit_sha":   commitSHA.String,
			"check_run_id": strconv.FormatInt(checkRunID.Int64, 10),
		}
		a.notifyGitBotOfFinalStatus("cancelled", "", "", reason, gitContext)
	}
	if changed {
		go a.dispatchPipelineRunNotification(runUUID.String(), "cancelled")
	}

	return changed, nil
}

func (a *App) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	runIDStr := r.PathValue("runID")
	runUUID, err := uuid.Parse(runIDStr)
	if err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	if err := a.cancelRunHierarchy(context.Background(), runUUID, "Run cancelled by user.", "Run cancelled because parent run was cancelled."); err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			http.Error(w, "Pipeline run not found", http.StatusNotFound)
		case errors.Is(err, errRunAlreadyCompleted):
			http.Error(w, "Run has already completed", http.StatusBadRequest)
		default:
			log.Error().Err(err).Str("run_id", runUUID.String()).Msg("Failed to cancel run")
			http.Error(w, "Failed to cancel run", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
}

func (a *App) handleDeleteRun(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("runID"))
	if runID == "" {
		http.Error(w, "Run ID is required", http.StatusBadRequest)
		return
	}

	if _, err := uuid.Parse(runID); err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	commandTag, err := a.db.Exec(context.Background(), "DELETE FROM pipeline_runs WHERE run_id = $1", runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to delete pipeline run")
		http.Error(w, "Failed to delete pipeline run", http.StatusInternalServerError)
		return
	}

	if commandTag.RowsAffected() == 0 {
		http.Error(w, "Pipeline run not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteRepoBranchRuns(w http.ResponseWriter, r *http.Request) {
	repoOwner := strings.TrimSpace(r.PathValue("repoOwner"))
	repoName := strings.TrimSpace(r.PathValue("repoName"))
	branchParam := strings.TrimSpace(r.PathValue("branch"))

	if repoOwner == "" || repoName == "" {
		http.Error(w, "Repository owner and name are required", http.StatusBadRequest)
		return
	}
	if branchParam == "" {
		http.Error(w, "Branch name is required", http.StatusBadRequest)
		return
	}

	branch := strings.Trim(branchParam, " ")
	branch = strings.Trim(branch, "/")

	var commandTag pgconn.CommandTag
	var err error
	branchLower := strings.ToLower(branch)

	ctx := context.Background()

	if branchLower == "others" {
		commandTag, err = a.db.Exec(ctx,
			"DELETE FROM pipeline_runs WHERE git_repo_owner = $1 AND git_repo_name = $2 AND (git_ref IS NULL OR git_ref = '')",
			repoOwner, repoName,
		)
	} else {
		normalized := branch
		if strings.HasPrefix(normalized, "refs/") {
			normalized = strings.TrimPrefix(normalized, "refs/heads/")
		}
		refWithPrefix := "refs/heads/" + normalized

		commandTag, err = a.db.Exec(ctx,
			"DELETE FROM pipeline_runs WHERE git_repo_owner = $1 AND git_repo_name = $2 AND (git_ref = $3 OR git_ref = $4)",
			repoOwner, repoName, refWithPrefix, normalized,
		)
	}

	if err != nil {
		log.Error().Err(err).
			Str("repo_owner", repoOwner).
			Str("repo_name", repoName).
			Str("branch", branch).Msg("Failed to delete pipeline runs for branch")
		http.Error(w, "Failed to delete runs for branch", http.StatusInternalServerError)
		return
	}

	if commandTag.RowsAffected() == 0 {
		http.Error(w, "No pipeline runs found for the specified branch", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

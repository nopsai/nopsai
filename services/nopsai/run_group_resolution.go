package nopsai

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"nopsai/services/nopsai/internal/configsync"
	runquery "nopsai/services/nopsai/internal/runs"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

func setNoStoreHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
}

func markRunRunning(ctx context.Context, runner execRunner, runID string) error {
	_, err := runner.Exec(ctx, `
		UPDATE pipeline_runs
		SET status = 'running', started_at = COALESCE(started_at, NOW())
		WHERE run_id = $1
		  AND status NOT IN ('success', 'failure', 'failure (ignored)', 'cancelled', 'timed_out', 'waiting_approval', 'rejected')`, runID)
	return err
}

func (a *App) resolveGroupIDForRepo(repoOwner, repoName string) (sql.NullInt32, error) {
	var groupID sql.NullInt32
	repoName = strings.TrimSpace(repoName)
	if repoName == "" {
		return groupID, nil
	}

	repoOwner = strings.TrimSpace(repoOwner)
	fullRepoName := repositoryFullName(repoOwner, repoName)
	matches, err := a.repositoryGroupMatches(context.Background(), repoOwner, repoName)
	if err != nil {
		return groupID, err
	}
	if len(matches) > 0 {
		groupID.Int32 = int32(matches[0].ID)
		groupID.Valid = true
		return groupID, nil
	}

	var existingID int32
	repoURL := configsync.CanonicalRepositoryURL(fullRepoName)
	err = a.db.QueryRow(context.Background(), "SELECT id FROM groups WHERE repository_full_name = $1 OR name = $1", fullRepoName).Scan(&existingID)
	if err == pgx.ErrNoRows {
		if repoOwner != "" {
			err = a.db.QueryRow(context.Background(), "SELECT id FROM groups WHERE name = $1", repoName).Scan(&existingID)
			if err == nil {
				log.Info().Str("old_name", repoName).Str("repository", fullRepoName).Msg("Found matching app folder, attaching repository metadata.")
				if _, updateErr := a.db.Exec(context.Background(), "UPDATE groups SET kind = 'app', repo_url = $1, repository_full_name = $2, updated_at = NOW() WHERE id = $3", repoURL, fullRepoName, existingID); updateErr != nil {
					log.Error().Err(updateErr).Msg("Failed to rename existing folder to claim it.")
					existingID = 0
				}
			} else if err == pgx.ErrNoRows {
				log.Info().Str("repo", fullRepoName).Msg("No existing app found. Creating a new one at the root.")
				err = a.db.QueryRow(context.Background(), `INSERT INTO groups (name, kind, parent_id, repo_url, repository_full_name) VALUES ($1, 'app', NULL, $2, $3) RETURNING id`, fullRepoName, repoURL, fullRepoName).Scan(&existingID)
			}
		} else {
			log.Info().Str("repo", repoName).Msg("No existing app found. Creating a new one at the root.")
			err = a.db.QueryRow(context.Background(), `INSERT INTO groups (name, kind, parent_id, repo_url, repository_full_name) VALUES ($1, 'app', NULL, $2, $3) RETURNING id`, repoName, configsync.CanonicalRepositoryURL(repoName), repoName).Scan(&existingID)
		}
	}
	if err == nil && existingID != 0 && fullRepoName != "" {
		_, _ = a.db.Exec(context.Background(), "UPDATE groups SET kind = 'app', repo_url = CASE WHEN repo_url = '' THEN $1 ELSE repo_url END, repository_full_name = $2, updated_at = NOW() WHERE id = $3", repoURL, fullRepoName, existingID)
	}
	if err != nil && err != pgx.ErrNoRows {
		return groupID, err
	}
	if existingID != 0 {
		groupID.Int32 = existingID
		groupID.Valid = true
	}
	return groupID, nil
}

func (a *App) resolveGroupIDForRun(ctx context.Context, explicitGroupPath, pipelinePath string, gitContext map[string]string) (sql.NullInt32, error) {
	for _, candidate := range runquery.GroupResolutionCandidates(explicitGroupPath, pipelinePath, gitContext) {
		switch candidate.Kind {
		case runquery.GroupResolutionPath:
			groupID, err := a.resolveGroupIDForPath(ctx, candidate.Value)
			if err != nil || groupID.Valid || candidate.Required {
				return groupID, err
			}
		case runquery.GroupResolutionRepo:
			repoOwner, repoName := splitRepositoryFullName(candidate.Value)
			return a.resolveGroupIDForRepo(repoOwner, repoName)
		}
	}
	return sql.NullInt32{}, nil
}

func splitRepositoryFullName(fullName string) (string, string) {
	fullName = strings.Trim(strings.TrimSpace(fullName), "/")
	owner, repo, ok := strings.Cut(fullName, "/")
	if !ok {
		return "", fullName
	}
	return owner, repo
}

func nullableGitCheckRunID(gitContext map[string]string) sql.NullInt64 {
	if gitContext == nil {
		return sql.NullInt64{}
	}
	value := strings.TrimSpace(gitContext["check_run_id"])
	if value == "" {
		return sql.NullInt64{}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: parsed, Valid: true}
}

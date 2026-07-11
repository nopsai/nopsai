package nopsai

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	runquery "nopsai/services/nopsai/internal/runs"

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

func (a *App) resolveTeamIDForRepo(repoOwner, repoName string) (sql.NullInt32, error) {
	var teamID sql.NullInt32
	repoName = strings.TrimSpace(repoName)
	if repoName == "" {
		return teamID, nil
	}

	repoOwner = strings.TrimSpace(repoOwner)
	fullRepoName := repositoryFullName(repoOwner, repoName)
	matches, err := a.repositoryTeamMatches(context.Background(), repoOwner, repoName)
	if err != nil {
		return teamID, err
	}
	if len(matches) > 0 {
		return repositoryTeamIDFromMatches(matches), nil
	}

	log.Debug().Str("repo", fullRepoName).Msg("No existing app/team found for repository run; leaving run unteamed.")
	return teamID, nil
}

func repositoryTeamIDFromMatches(matches []repositoryTeamMatch) sql.NullInt32 {
	if len(matches) == 0 {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(matches[0].ID), Valid: true}
}

func (a *App) resolveTeamIDForRun(ctx context.Context, explicitTeamPath, pipelinePath string, gitContext map[string]string) (sql.NullInt32, error) {
	for _, candidate := range runquery.TeamResolutionCandidates(explicitTeamPath, pipelinePath, gitContext) {
		switch candidate.Kind {
		case runquery.TeamResolutionPath:
			teamID, err := a.resolveTeamIDForPath(ctx, candidate.Value)
			if err != nil || teamID.Valid || candidate.Required {
				return teamID, err
			}
		case runquery.TeamResolutionRepo:
			repoOwner, repoName := splitRepositoryFullName(candidate.Value)
			return a.resolveTeamIDForRepo(repoOwner, repoName)
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

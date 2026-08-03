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

func repositoryTeamIDFromMatchesUnderPath(matches []repositoryTeamMatch, teamPath string) (sql.NullInt32, bool) {
	teamPath = strings.Trim(strings.TrimSpace(teamPath), "/")
	if teamPath == "" || isGlobalGrantResourceID(teamPath) {
		return repositoryTeamIDFromMatches(matches), len(matches) > 0
	}
	for _, match := range matches {
		path := strings.Trim(strings.TrimSpace(match.Path), "/")
		if path == teamPath || strings.HasPrefix(path, teamPath+"/") {
			return sql.NullInt32{Int32: int32(match.ID), Valid: true}, true
		}
	}
	return sql.NullInt32{}, false
}

func (a *App) resolveRepositoryTeamIDUnderPath(ctx context.Context, teamPath string, gitContext map[string]string) (sql.NullInt32, bool, error) {
	if gitContext == nil {
		return sql.NullInt32{}, false, nil
	}
	repoName := strings.TrimSpace(gitContext["repo_name"])
	if repoName == "" {
		return sql.NullInt32{}, false, nil
	}
	matches, err := a.repositoryTeamMatches(ctx, gitContext["repo_owner"], repoName)
	if err != nil {
		return sql.NullInt32{}, false, err
	}
	teamID, found := repositoryTeamIDFromMatchesUnderPath(matches, teamPath)
	return teamID, found, nil
}

func (a *App) resolveTeamIDForRun(ctx context.Context, explicitTeamPath, pipelinePath string, gitContext map[string]string) (sql.NullInt32, error) {
	explicitTeamPath = strings.Trim(strings.TrimSpace(explicitTeamPath), "/")
	if isGlobalGrantResourceID(explicitTeamPath) {
		explicitTeamPath = ""
	}
	if explicitTeamPath != "" {
		if teamID, found, err := a.resolveRepositoryTeamIDUnderPath(ctx, explicitTeamPath, gitContext); err != nil || found {
			return teamID, err
		}
		return a.resolveTeamIDForPath(ctx, explicitTeamPath)
	}
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

func hasGitHubCheckRunContext(gitContext map[string]string) bool {
	if gitContext == nil {
		return false
	}
	return strings.TrimSpace(gitContext["repo_owner"]) != "" &&
		strings.TrimSpace(gitContext["repo_name"]) != "" &&
		strings.TrimSpace(gitContext["commit_sha"]) != ""
}

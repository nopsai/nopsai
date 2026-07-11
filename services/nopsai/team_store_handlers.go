package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"

	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
)

func normalizeTeamForWrite(team *Team) error {
	team.Name = strings.TrimSpace(team.Name)
	team.Description = strings.TrimSpace(team.Description)
	team.Kind = strings.TrimSpace(strings.ToLower(team.Kind))
	team.RepoURL = strings.TrimSpace(team.RepoURL)
	team.RepositoryFullName = strings.Trim(strings.TrimSpace(team.RepositoryFullName), "/")
	if team.Kind == "" {
		if team.RepoURL != "" || team.RepositoryFullName != "" {
			team.Kind = "app"
		} else {
			team.Kind = "team"
		}
	}
	switch team.Kind {
	case "team":
		if team.Name == "" {
			return fmt.Errorf("team name is required")
		}
		if configsync.IsReservedRootTeamName(team.Name) {
			return fmt.Errorf("root is reserved and cannot be used as a team name")
		}
		team.RepoURL = ""
		team.RepositoryFullName = ""
	case "app":
		if team.RepoURL == "" && team.RepositoryFullName != "" {
			team.RepoURL = configsync.CanonicalRepositoryURL(team.RepositoryFullName)
		}
		if team.RepoURL == "" {
			return fmt.Errorf("repository URL is required")
		}
		fullName, err := configsync.RepositoryFullNameFromURL(team.RepoURL)
		if err != nil {
			return err
		}
		team.RepositoryFullName = fullName
		if team.Name == "" {
			team.Name = configsync.RepositoryDisplayNameFromFullName(fullName)
		}
		if _, err := configsync.NormalizeStructureName(team.Name); err != nil {
			return err
		}
		team.Description = ""
	default:
		return fmt.Errorf("kind must be team or app")
	}
	return nil
}

type teamListError struct {
	status  int
	message string
	err     error
}

func (e *teamListError) Error() string {
	if e == nil {
		return ""
	}
	if e.err != nil {
		return e.err.Error()
	}
	return e.message
}

func teamListStatusError(status int, message string, err error) *teamListError {
	return &teamListError{status: status, message: message, err: err}
}

func (a *App) visibleTeamsForRequest(r *http.Request) ([]Team, map[int]teamPathRecord, *teamListError) {
	rows, err := a.db.Query(r.Context(), "SELECT id, name, COALESCE(kind, 'team'), parent_id, description, COALESCE(repo_url, ''), COALESCE(repository_full_name, '') FROM teams")
	if err != nil {
		log.Error().Err(err).Msg("Failed to query teams from database")
		return nil, nil, teamListStatusError(http.StatusInternalServerError, "Failed to retrieve teams", err)
	}
	defer rows.Close()

	var allTeams []Team
	teamMap := make(map[int]*Team)

	for rows.Next() {
		var g Team
		var parentID sql.NullInt32
		var description sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &g.Kind, &parentID, &description, &g.RepoURL, &g.RepositoryFullName); err != nil {
			log.Error().Err(err).Msg("Failed to scan team row")
			return nil, nil, teamListStatusError(http.StatusInternalServerError, "Error processing teams", err)
		}
		if parentID.Valid {
			pid := int(parentID.Int32)
			g.ParentID = &pid
		}
		if description.Valid {
			g.Description = description.String
		}
		if g.Kind == "" {
			g.Kind = "team"
		}
		if g.Kind == "team" && (g.RepoURL != "" || g.RepositoryFullName != "" || strings.Contains(strings.Trim(g.Name, "/"), "/")) {
			g.Kind = "app"
		}
		if g.Kind == "app" && g.RepositoryFullName == "" {
			if fullName, err := configsync.RepositoryFullNameFromURL(g.RepoURL); err == nil {
				g.RepositoryFullName = fullName
			} else if strings.Contains(g.Name, "/") {
				g.RepositoryFullName = strings.Trim(g.Name, "/")
			}
		}
		if g.Kind == "app" && g.RepoURL == "" && g.RepositoryFullName != "" {
			g.RepoURL = configsync.CanonicalRepositoryURL(g.RepositoryFullName)
		}
		allTeams = append(allTeams, g)
	}
	if rows.Err() != nil {
		log.Error().Err(rows.Err()).Msg("Error iterating over team rows")
		return nil, nil, teamListStatusError(http.StatusInternalServerError, "Error retrieving teams", rows.Err())
	}

	for i := range allTeams {
		teamMap[allTeams[i].ID] = &allTeams[i]
	}

	pathRecords, err := a.teamPathRecords(r.Context())
	if err != nil {
		return nil, nil, teamListStatusError(http.StatusInternalServerError, "Failed to resolve team paths", err)
	}

	resources := make([]model.ResourceRef, 0, len(allTeams))
	for _, team := range allTeams {
		record, ok := pathRecords[team.ID]
		if !ok || strings.TrimSpace(record.Path) == "" {
			continue
		}
		resource := model.ResourceRef{Type: grantResourceTeam, ID: record.Path}
		resources = append(resources, resource)
	}

	allowedSet, err := a.allowedResourceSet(r, "team.list", resources)
	if err != nil {
		return nil, nil, teamListStatusError(http.StatusServiceUnavailable, "Authorization unavailable", err)
	}
	visibleTeamIDs, directAllowedTeamIDs := visibleTeamTeamIDs(allTeams, pathRecords, allowedSet)

	query := `
        SELECT g.id, MAX(r.started_at)
        FROM teams g
        JOIN pipeline_runs r ON g.id = r.team_id
        GROUP BY g.id
    `
	runRows, err := a.db.Query(r.Context(), query)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query last run times for teams")
	} else {
		defer runRows.Close()
		for runRows.Next() {
			var teamID int
			var lastRunAt sql.NullTime
			if err := runRows.Scan(&teamID, &lastRunAt); err == nil {
				if lastRunAt.Valid {
					if team, ok := teamMap[teamID]; ok {
						team.LastRunAt = &lastRunAt.Time
					}
				}
			}
		}
	}

	filtered := make([]Team, 0, len(allTeams))
	for _, team := range allTeams {
		if _, ok := visibleTeamIDs[team.ID]; !ok {
			continue
		}
		if _, ok := directAllowedTeamIDs[team.ID]; !ok {
			team.Description = ""
			team.LastRunAt = nil
			team.NavigationOnly = true
		}
		filtered = append(filtered, team)
	}

	return filtered, pathRecords, nil
}

func visibleTeamTeamIDs(allTeams []Team, pathRecords map[int]teamPathRecord, allowedSet map[string]struct{}) (map[int]struct{}, map[int]struct{}) {
	visible := make(map[int]struct{})
	directAllowed := make(map[int]struct{})
	teamByID := make(map[int]Team, len(allTeams))
	for _, team := range allTeams {
		teamByID[team.ID] = team
	}

	for _, team := range allTeams {
		record, ok := pathRecords[team.ID]
		if !ok || strings.TrimSpace(record.Path) == "" {
			continue
		}
		resource := model.ResourceRef{Type: grantResourceTeam, ID: record.Path}
		if _, ok := allowedSet[resourceKey(resource)]; !ok {
			continue
		}

		directAllowed[team.ID] = struct{}{}
		for currentID := team.ID; ; {
			if _, ok := visible[currentID]; ok {
				break
			}
			visible[currentID] = struct{}{}
			current, ok := teamByID[currentID]
			if !ok || current.ParentID == nil {
				break
			}
			currentID = *current.ParentID
		}
	}

	return visible, directAllowed
}

func (a *App) teamDeleteAuthorizationTarget(ctx context.Context, teamID int) (string, model.ResourceRef, error) {
	if a == nil || a.db == nil {
		return "", model.ResourceRef{}, fmt.Errorf("database unavailable")
	}

	var teamName, kind, repositoryFullName string
	if err := a.db.QueryRow(ctx, `SELECT name, COALESCE(kind, 'team'), COALESCE(repository_full_name, '') FROM teams WHERE id = $1`, teamID).Scan(&teamName, &kind, &repositoryFullName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return "", model.ResourceRef{}, fmt.Errorf("resource not found")
		}
		return "", model.ResourceRef{}, err
	}

	resource, err := a.teamGrantResourceByTeamID(ctx, teamID)
	if err != nil {
		return "", model.ResourceRef{}, err
	}
	action, resourceRef := teamDeleteAuthorizationTargetFromName(teamName, kind, repositoryFullName, resource)
	return action, resourceRef, nil
}

func teamDeleteAuthorizationTargetFromName(teamName, kind, repositoryFullName string, teamResource accessGrantResource) (string, model.ResourceRef) {
	repositoryID := strings.Trim(strings.TrimSpace(repositoryFullName), "/")
	if repositoryID == "" {
		repositoryID = strings.Trim(strings.TrimSpace(teamName), "/")
	}
	if kind == "app" || strings.Contains(repositoryID, "/") {
		return "repository.delete", model.ResourceRef{Type: grantResourceRepo, ID: repositoryID}
	}
	return "team.delete", model.ResourceRef{Type: grantResourceTeam, ID: teamResource.ID}
}

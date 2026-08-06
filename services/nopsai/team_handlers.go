package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"

	"nopsai/pkg/httpapi"
	"nopsai/services/aaa/pkg/model"
)

type teamListResponse struct {
	Teams        []teamResponse        `json:"teams"`
	Applications []applicationResponse `json:"applications,omitempty"`
}

type teamResponse struct {
	ID             int        `json:"id"`
	Kind           string     `json:"kind"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	DisplayName    string     `json:"display_name"`
	Description    string     `json:"description,omitempty"`
	ParentTeamID   *int       `json:"parent_team_id"`
	ParentID       *int       `json:"parent_id"`
	Path           string     `json:"path"`
	Source         string     `json:"source"`
	LastRunAt      *time.Time `json:"last_run_at,omitempty"`
	NavigationOnly bool       `json:"navigation_only,omitempty"`
	Applications   []int      `json:"application_ids,omitempty"`
}

type applicationResponse struct {
	ID                 int        `json:"id"`
	Kind               string     `json:"kind"`
	Name               string     `json:"name"`
	Slug               string     `json:"slug"`
	DisplayName        string     `json:"display_name"`
	TeamID             *int       `json:"team_id"`
	ParentID           *int       `json:"parent_id"`
	Path               string     `json:"path"`
	TeamPath           string     `json:"team_path,omitempty"`
	RepoURL            string     `json:"repo_url"`
	RepositoryFullName string     `json:"repository_full_name"`
	Source             string     `json:"source"`
	LastRunAt          *time.Time `json:"last_run_at,omitempty"`
	NavigationOnly     bool       `json:"navigation_only,omitempty"`
}

type teamWriteRequest struct {
	Name         string           `json:"name"`
	Slug         string           `json:"slug"`
	DisplayName  string           `json:"display_name"`
	Description  string           `json:"description"`
	ParentTeamID optionalIntField `json:"parent_team_id"`
	ParentID     optionalIntField `json:"parent_id"`
}

type applicationWriteRequest struct {
	Name               string `json:"name"`
	Slug               string `json:"slug"`
	DisplayName        string `json:"display_name"`
	RepoURL            string `json:"repo_url"`
	RepositoryFullName string `json:"repository_full_name"`
}

type optionalIntField struct {
	Set   bool
	Value *int
}

func (field *optionalIntField) UnmarshalJSON(data []byte) error {
	field.Set = true
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		field.Value = nil
		return nil
	}
	var value int
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	field.Value = &value
	return nil
}

func (a *App) handleListTeams(w http.ResponseWriter, r *http.Request) {
	teams, records, listErr := a.visibleTeamsForRequest(r)
	if listErr != nil {
		http.Error(w, listErr.message, listErr.status)
		return
	}

	includeApplications := strings.EqualFold(r.URL.Query().Get("include"), "applications") ||
		strings.EqualFold(r.URL.Query().Get("include_applications"), "true")
	response := teamListResponse{Teams: []teamResponse{}}
	if includeApplications {
		response.Applications = []applicationResponse{}
	}

	appIDsByTeam := map[int][]int{}
	for _, team := range teams {
		if team.Kind == "app" {
			if team.ParentID != nil {
				appIDsByTeam[*team.ParentID] = append(appIDsByTeam[*team.ParentID], team.ID)
			}
			if includeApplications {
				response.Applications = append(response.Applications, applicationResponseFromTeam(team, records))
			}
			continue
		}
		response.Teams = append(response.Teams, teamResponseFromTeam(team, records, nil))
	}
	for index := range response.Teams {
		if ids := appIDsByTeam[response.Teams[index].ID]; len(ids) > 0 {
			response.Teams[index].Applications = ids
		}
	}

	_ = httpapi.WriteJSON(w, http.StatusOK, response)
}

func (a *App) handleGetTeam(w http.ResponseWriter, r *http.Request) {
	record, status, err := a.resolveTeamRecord(r.Context(), r.PathValue("teamID"), false)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	team, err := a.teamForRecord(r.Context(), record)
	if err != nil {
		http.Error(w, "Failed to load team", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, teamResponseFromTeam(team, map[int]teamPathRecord{record.ID: record}, nil))
}

func (a *App) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	var input teamWriteRequest
	if err := httpapi.DecodeJSON(r, &input); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	team := Team{
		Kind:        "team",
		Name:        firstNonEmptyString(input.Name, input.Slug, input.DisplayName),
		Description: input.Description,
		ParentID:    parentIDFromTeamWriteRequest(input, nil),
	}
	if err := normalizeTeamForWrite(&team); err != nil {
		http.Error(w, strings.ReplaceAll(err.Error(), "team", "team"), http.StatusBadRequest)
		return
	}
	if err := a.validateTeamParentForWrite(r.Context(), team.ID, team.ParentID); err != nil {
		writeTeamParentValidationError(w, err)
		return
	}
	if !a.authorizeTeamCreate(w, r, team.ParentID) {
		return
	}
	created, err := a.insertTeamRecord(r.Context(), team)
	if err != nil {
		writeTeamMutationError(w, err, "Failed to create team")
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusCreated, teamResponseFromTeam(created, nil, nil))
}

func (a *App) handleUpdateTeam(w http.ResponseWriter, r *http.Request) {
	record, status, err := a.resolveTeamRecord(r.Context(), r.PathValue("teamID"), false)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	var input teamWriteRequest
	if err := httpapi.DecodeJSON(r, &input); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	team := Team{
		ID:          record.ID,
		Kind:        "team",
		Name:        firstNonEmptyString(input.Name, input.Slug, input.DisplayName, record.Name),
		Description: input.Description,
		ParentID:    parentIDFromTeamWriteRequest(input, record.ParentID),
	}
	if err := normalizeTeamForWrite(&team); err != nil {
		http.Error(w, strings.ReplaceAll(err.Error(), "team", "team"), http.StatusBadRequest)
		return
	}
	if err := a.validateTeamParentForWrite(r.Context(), team.ID, team.ParentID); err != nil {
		writeTeamParentValidationError(w, err)
		return
	}
	if !a.authorizeTeamUpdate(w, r, record.ID) {
		return
	}
	if !sameOptionalInt(record.ParentID, team.ParentID) && !a.authorizeTeamCreate(w, r, team.ParentID) {
		return
	}
	if err := a.updateTeamRecord(r.Context(), team); err != nil {
		writeTeamMutationError(w, err, "Failed to update team")
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *App) handleDeleteTeam(w http.ResponseWriter, r *http.Request) {
	record, status, err := a.resolveTeamRecord(r.Context(), r.PathValue("teamID"), false)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	if !a.authorizeTeamDelete(w, r, record.ID) {
		return
	}
	if _, err := a.db.Exec(r.Context(), "DELETE FROM teams WHERE id = $1", record.ID); err != nil {
		log.Error().Err(err).Msg("Failed to delete team")
		http.Error(w, "Failed to delete team", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleListTeamApplications(w http.ResponseWriter, r *http.Request) {
	parentID, records, status, err := a.resolveApplicationParent(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	teams, _, listErr := a.visibleTeamsForRequest(r)
	if listErr != nil {
		http.Error(w, listErr.message, listErr.status)
		return
	}
	out := []applicationResponse{}
	for _, team := range teams {
		if team.Kind != "app" {
			continue
		}
		if (team.ParentID == nil && parentID == nil) || (team.ParentID != nil && parentID != nil && *team.ParentID == *parentID) {
			out = append(out, applicationResponseFromTeam(team, records))
		}
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, out)
}

func (a *App) handleCreateTeamApplication(w http.ResponseWriter, r *http.Request) {
	parentID, _, status, err := a.resolveApplicationParent(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	var input applicationWriteRequest
	if err := httpapi.DecodeJSON(r, &input); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	team := Team{
		Kind:               "app",
		Name:               firstNonEmptyString(input.Name, input.Slug, input.DisplayName),
		RepoURL:            input.RepoURL,
		RepositoryFullName: input.RepositoryFullName,
		ParentID:           parentID,
	}
	if err := normalizeTeamForWrite(&team); err != nil {
		http.Error(w, strings.ReplaceAll(err.Error(), "team", "application"), http.StatusBadRequest)
		return
	}
	if !a.authorizeApplicationCreate(w, r, parentID) {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "Failed to create application", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())
	created, err := insertTeamRecordWithRunner(r.Context(), tx, team)
	if err != nil {
		writeTeamMutationError(w, err, "Failed to create application")
		return
	}
	if err := reassignRepositoryRunsToApplication(r.Context(), tx, created.ID, created.ParentID, created.RepositoryFullName); err != nil {
		writeTeamMutationError(w, err, "Failed to assign existing runs to application")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeTeamMutationError(w, err, "Failed to create application")
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusCreated, applicationResponseFromTeam(created, nil))
}

func (a *App) handleUpdateTeamApplication(w http.ResponseWriter, r *http.Request) {
	parentID, _, status, err := a.resolveApplicationParent(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	appRecord, status, err := a.resolveTeamRecord(r.Context(), r.PathValue("applicationID"), true)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	var input applicationWriteRequest
	if err := httpapi.DecodeJSON(r, &input); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	team := Team{
		ID:                 appRecord.ID,
		Kind:               "app",
		Name:               firstNonEmptyString(input.Name, input.Slug, input.DisplayName, appRecord.Name),
		RepoURL:            input.RepoURL,
		RepositoryFullName: input.RepositoryFullName,
		ParentID:           parentID,
	}
	if err := normalizeTeamForWrite(&team); err != nil {
		http.Error(w, strings.ReplaceAll(err.Error(), "team", "application"), http.StatusBadRequest)
		return
	}
	if !a.authorizeApplicationUpdate(w, r, appRecord.ID) {
		return
	}
	if !sameOptionalInt(appRecord.ParentID, parentID) && !a.authorizeApplicationCreate(w, r, parentID) {
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "Failed to update application", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())
	if err := updateTeamRecordWithRunner(r.Context(), tx, team); err != nil {
		writeTeamMutationError(w, err, "Failed to update application")
		return
	}
	if err := reassignRepositoryRunsToApplication(r.Context(), tx, team.ID, team.ParentID, team.RepositoryFullName); err != nil {
		writeTeamMutationError(w, err, "Failed to assign existing runs to application")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeTeamMutationError(w, err, "Failed to update application")
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *App) handleDeleteTeamApplication(w http.ResponseWriter, r *http.Request) {
	parentID, _, status, err := a.resolveApplicationParent(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	appRecord, status, err := a.resolveTeamRecord(r.Context(), r.PathValue("applicationID"), true)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	if !sameOptionalInt(appRecord.ParentID, parentID) {
		http.Error(w, "application does not belong to this team", http.StatusNotFound)
		return
	}
	if !a.authorizeApplicationDelete(w, r, appRecord.ID) {
		return
	}
	if _, err := a.db.Exec(r.Context(), "DELETE FROM teams WHERE id = $1", appRecord.ID); err != nil {
		log.Error().Err(err).Msg("Failed to delete application")
		http.Error(w, "Failed to delete application", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleTeamConfigRepository(w http.ResponseWriter, r *http.Request) {
	if !a.setTeamHierarchyPathValue(w, r) {
		return
	}
	a.handleTeamConfigRepositoryRoute(w, r)
}

func (a *App) handleTeamNotificationRoute(w http.ResponseWriter, r *http.Request) {
	if !a.setTeamHierarchyPathValue(w, r) {
		return
	}
	a.handleTeamNotificationRouteByMethod(w, r)
}

func (a *App) handleTeamNotificationRouteByMethod(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.handleGetTeamNotificationRoute(w, r)
	case http.MethodPut:
		a.handleUpsertTeamNotificationRoute(w, r)
	case http.MethodDelete:
		a.handleDeleteTeamNotificationRoute(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleTeamConfigRepositoryRoute(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/sync") && r.Method == http.MethodGet:
		a.handleGetTeamConfigRepositorySyncStatus(w, r)
	case strings.HasSuffix(r.URL.Path, "/sync") && r.Method == http.MethodPost:
		a.handleSyncTeamConfigRepository(w, r)
	case strings.HasSuffix(r.URL.Path, "/drift") && r.Method == http.MethodGet:
		a.handleGetTeamConfigRepositoryDrift(w, r)
	case strings.HasSuffix(r.URL.Path, "/write") && r.Method == http.MethodPost:
		a.handleWriteTeamConfigRepository(w, r)
	case strings.HasSuffix(r.URL.Path, "/validate") && r.Method == http.MethodPost:
		a.handleValidateTeamConfigRepository(w, r)
	case r.Method == http.MethodGet:
		a.handleGetTeamConfigRepository(w, r)
	case r.Method == http.MethodPut:
		a.handleUpsertTeamConfigRepository(w, r)
	case r.Method == http.MethodDelete:
		a.handleDeleteTeamConfigRepository(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) setTeamHierarchyPathValue(w http.ResponseWriter, r *http.Request) bool {
	record, status, err := a.resolveTeamRecord(r.Context(), r.PathValue("teamID"), false)
	if err != nil {
		http.Error(w, err.Error(), status)
		return false
	}
	path := strings.TrimSpace(record.Path)
	if path == "" {
		path = record.Name
	}
	r.SetPathValue("teamID", path)
	return true
}

func (a *App) authorizeTeamCreate(w http.ResponseWriter, r *http.Request, parentID *int) bool {
	if parentID != nil {
		parentResource, err := a.teamGrantResourceByTeamID(r.Context(), *parentID)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return false
		}
		return a.requireAAADecision(w, r, "team.create", model.ResourceRef{Type: grantResourceTeam, ID: parentResource.ID})
	}
	return a.requireAAADecision(w, r, "team.create", model.ResourceRef{Type: grantResourceTeam, ID: "*"})
}

func (a *App) authorizeTeamUpdate(w http.ResponseWriter, r *http.Request, teamID int) bool {
	resource, err := a.teamGrantResourceByTeamID(r.Context(), teamID)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return false
	}
	return a.requireAAADecision(w, r, "team.update", model.ResourceRef{Type: grantResourceTeam, ID: resource.ID})
}

func (a *App) authorizeTeamDelete(w http.ResponseWriter, r *http.Request, teamID int) bool {
	action, resource, err := a.teamDeleteAuthorizationTarget(r.Context(), teamID)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return false
	}
	return a.requireAAADecision(w, r, action, resource)
}

func (a *App) authorizeApplicationCreate(w http.ResponseWriter, r *http.Request, parentID *int) bool {
	return a.authorizeTeamCreate(w, r, parentID)
}

func (a *App) authorizeApplicationUpdate(w http.ResponseWriter, r *http.Request, teamID int) bool {
	return a.authorizeTeamUpdate(w, r, teamID)
}

func (a *App) authorizeApplicationDelete(w http.ResponseWriter, r *http.Request, teamID int) bool {
	return a.authorizeTeamDelete(w, r, teamID)
}

func (a *App) validateTeamParentForWrite(ctx context.Context, teamID int, parentID *int) error {
	if parentID == nil {
		return nil
	}
	records, err := a.teamPathRecords(ctx)
	if err != nil {
		return fmt.Errorf("failed to resolve teams")
	}
	return validateTeamParentID(records, teamID, parentID)
}

func validateTeamParentID(records map[int]teamPathRecord, teamID int, parentID *int) error {
	if parentID == nil {
		return nil
	}
	if *parentID == teamID && teamID != 0 {
		return fmt.Errorf("team cannot be its own parent")
	}
	visited := map[int]struct{}{}
	if teamID != 0 {
		visited[teamID] = struct{}{}
	}
	currentID := *parentID
	for {
		record, ok := records[currentID]
		if !ok {
			return fmt.Errorf("parent team not found")
		}
		if record.Kind == "app" || record.RepoURL != "" || record.RepositoryFullName != "" {
			return fmt.Errorf("parent must be a team")
		}
		if _, seen := visited[currentID]; seen {
			return fmt.Errorf("team parent hierarchy would contain a cycle")
		}
		visited[currentID] = struct{}{}
		if record.ParentID == nil {
			return nil
		}
		currentID = *record.ParentID
	}
}

func (a *App) resolveApplicationParent(ctx context.Context, raw string) (*int, map[int]teamPathRecord, int, error) {
	records, err := a.teamPathRecords(ctx)
	if err != nil {
		return nil, nil, http.StatusInternalServerError, fmt.Errorf("failed to resolve teams")
	}
	teamID := strings.Trim(strings.TrimSpace(raw), "/")
	if teamID == "" || strings.EqualFold(teamID, "root") {
		return nil, records, http.StatusOK, nil
	}
	record, status, err := a.resolveTeamRecord(ctx, teamID, false)
	if err != nil {
		return nil, nil, status, err
	}
	records[record.ID] = record
	return &record.ID, records, http.StatusOK, nil
}

func (a *App) resolveTeamRecord(ctx context.Context, raw string, wantApplication bool) (teamPathRecord, int, error) {
	raw = strings.Trim(strings.TrimSpace(raw), "/")
	if raw == "" || strings.EqualFold(raw, "root") {
		return teamPathRecord{}, http.StatusNotFound, fmt.Errorf("team not found")
	}
	records, err := a.teamPathRecords(ctx)
	if err != nil {
		return teamPathRecord{}, http.StatusInternalServerError, fmt.Errorf("failed to resolve teams")
	}
	var record teamPathRecord
	found := false
	if id, parseErr := strconv.Atoi(raw); parseErr == nil {
		record, found = records[id]
	} else {
		if decoded, decodeErr := url.PathUnescape(raw); decodeErr == nil {
			raw = decoded
		}
		raw = strings.Trim(strings.TrimSpace(raw), "/")
		for _, candidate := range records {
			if candidate.Path == raw {
				record = candidate
				found = true
				break
			}
		}
	}
	if !found {
		return teamPathRecord{}, http.StatusNotFound, fmt.Errorf("team not found")
	}
	isApplication := record.Kind == "app" || record.RepositoryFullName != "" || record.RepoURL != ""
	if wantApplication && !isApplication {
		return teamPathRecord{}, http.StatusNotFound, fmt.Errorf("application not found")
	}
	if !wantApplication && isApplication {
		return teamPathRecord{}, http.StatusNotFound, fmt.Errorf("team not found")
	}
	return record, http.StatusOK, nil
}

func (a *App) teamForRecord(ctx context.Context, record teamPathRecord) (Team, error) {
	var team Team
	var parentID sql.NullInt32
	var description sql.NullString
	if err := a.db.QueryRow(ctx, "SELECT id, name, COALESCE(kind, 'team'), parent_id, description, COALESCE(repo_url, ''), COALESCE(repository_full_name, '') FROM teams WHERE id = $1", record.ID).Scan(
		&team.ID,
		&team.Name,
		&team.Kind,
		&parentID,
		&description,
		&team.RepoURL,
		&team.RepositoryFullName,
	); err != nil {
		return Team{}, err
	}
	if parentID.Valid {
		parent := int(parentID.Int32)
		team.ParentID = &parent
	}
	if description.Valid {
		team.Description = description.String
	}
	return team, nil
}

func (a *App) insertTeamRecord(ctx context.Context, team Team) (Team, error) {
	return insertTeamRecordWithRunner(ctx, a.db, team)
}

func insertTeamRecordWithRunner(ctx context.Context, runner queryRunner, team Team) (Team, error) {
	query := `INSERT INTO teams (name, kind, parent_id, description, repo_url, repository_full_name) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	if err := runner.QueryRow(ctx, query, team.Name, team.Kind, team.ParentID, team.Description, team.RepoURL, team.RepositoryFullName).Scan(&team.ID); err != nil {
		return Team{}, err
	}
	return team, nil
}

func (a *App) updateTeamRecord(ctx context.Context, team Team) error {
	return updateTeamRecordWithRunner(ctx, a.db, team)
}

func updateTeamRecordWithRunner(ctx context.Context, runner queryRunner, team Team) error {
	query := `UPDATE teams SET name = $1, kind = $2, parent_id = $3, description = $4, repo_url = $5, repository_full_name = $6, updated_at = NOW() WHERE id = $7`
	_, err := runner.Exec(ctx, query, team.Name, team.Kind, team.ParentID, team.Description, team.RepoURL, team.RepositoryFullName, team.ID)
	return err
}

func writeTeamMutationError(w http.ResponseWriter, err error, fallback string) {
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), "unique constraint") {
		http.Error(w, "A team or application with this name or repository already exists.", http.StatusConflict)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "resource not found", http.StatusNotFound)
		return
	}
	log.Error().Err(err).Str("fallback", fallback).Msg("Team mutation failed")
	http.Error(w, fallback, http.StatusInternalServerError)
}

func teamResponseFromTeam(team Team, records map[int]teamPathRecord, applicationIDs []int) teamResponse {
	path := teamPathForResponse(team.ID, team.Name, records)
	lastRunAt := nullableSQLTime(team.LastRunAt)
	return teamResponse{
		ID:             team.ID,
		Kind:           "team",
		Name:           team.Name,
		Slug:           team.Name,
		DisplayName:    team.Name,
		Description:    team.Description,
		ParentTeamID:   team.ParentID,
		ParentID:       team.ParentID,
		Path:           path,
		Source:         "database",
		LastRunAt:      lastRunAt,
		NavigationOnly: team.NavigationOnly,
		Applications:   applicationIDs,
	}
}

func applicationResponseFromTeam(team Team, records map[int]teamPathRecord) applicationResponse {
	path := teamPathForResponse(team.ID, team.Name, records)
	teamPath := ""
	if team.ParentID != nil && records != nil {
		teamPath = records[*team.ParentID].Path
	}
	lastRunAt := nullableSQLTime(team.LastRunAt)
	return applicationResponse{
		ID:                 team.ID,
		Kind:               "application",
		Name:               team.Name,
		Slug:               team.Name,
		DisplayName:        teamDisplayNameForApplication(team),
		TeamID:             team.ParentID,
		ParentID:           team.ParentID,
		Path:               path,
		TeamPath:           teamPath,
		RepoURL:            team.RepoURL,
		RepositoryFullName: team.RepositoryFullName,
		Source:             "database",
		LastRunAt:          lastRunAt,
		NavigationOnly:     team.NavigationOnly,
	}
}

func teamPathForResponse(teamID int, fallback string, records map[int]teamPathRecord) string {
	if records != nil {
		if record, ok := records[teamID]; ok && strings.TrimSpace(record.Path) != "" {
			return record.Path
		}
	}
	return strings.Trim(strings.TrimSpace(fallback), "/")
}

func teamDisplayNameForApplication(team Team) string {
	if strings.TrimSpace(team.Name) != "" && !strings.Contains(team.Name, "/") {
		return team.Name
	}
	fullName := strings.Trim(strings.TrimSpace(team.RepositoryFullName), "/")
	if fullName == "" {
		fullName = strings.Trim(strings.TrimSpace(team.Name), "/")
	}
	parts := strings.Split(fullName, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if strings.TrimSpace(parts[i]) != "" {
			return strings.TrimSpace(parts[i])
		}
	}
	return team.Name
}

func nullableSQLTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	return value
}

func parentIDFromTeamWriteRequest(input teamWriteRequest, fallback *int) *int {
	if input.ParentTeamID.Set {
		return input.ParentTeamID.Value
	}
	if input.ParentID.Set {
		return input.ParentID.Value
	}
	return fallback
}

func writeTeamParentValidationError(w http.ResponseWriter, err error) {
	if strings.Contains(err.Error(), "not found") {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}

func sameOptionalInt(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

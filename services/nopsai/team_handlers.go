package nopsai

import (
	"context"
	"database/sql"
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
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
	ParentTeamID *int   `json:"parent_team_id"`
	ParentID     *int   `json:"parent_id"`
}

type applicationWriteRequest struct {
	Name               string `json:"name"`
	Slug               string `json:"slug"`
	DisplayName        string `json:"display_name"`
	RepoURL            string `json:"repo_url"`
	RepositoryFullName string `json:"repository_full_name"`
}

func (a *App) handleListTeams(w http.ResponseWriter, r *http.Request) {
	groups, records, listErr := a.visibleGroupsForRequest(r)
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
	for _, group := range groups {
		if group.Kind == "app" {
			if group.ParentID != nil {
				appIDsByTeam[*group.ParentID] = append(appIDsByTeam[*group.ParentID], group.ID)
			}
			if includeApplications {
				response.Applications = append(response.Applications, applicationResponseFromGroup(group, records))
			}
			continue
		}
		response.Teams = append(response.Teams, teamResponseFromGroup(group, records, nil))
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
	group, err := a.groupForRecord(r.Context(), record)
	if err != nil {
		http.Error(w, "Failed to load team", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, teamResponseFromGroup(group, map[int]groupPathRecord{record.ID: record}, nil))
}

func (a *App) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	var input teamWriteRequest
	if err := httpapi.DecodeJSON(r, &input); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	group := Group{
		Kind:        "group",
		Name:        firstNonEmptyString(input.Name, input.Slug, input.DisplayName),
		Description: input.Description,
		ParentID:    firstNonNilInt(input.ParentTeamID, input.ParentID),
	}
	if err := normalizeGroupForWrite(&group); err != nil {
		http.Error(w, strings.ReplaceAll(err.Error(), "group", "team"), http.StatusBadRequest)
		return
	}
	if !a.authorizeTeamCreate(w, r, group.ParentID) {
		return
	}
	created, err := a.insertGroupRecord(r.Context(), group)
	if err != nil {
		writeGroupMutationError(w, err, "Failed to create team")
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusCreated, teamResponseFromGroup(created, nil, nil))
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
	group := Group{
		ID:          record.ID,
		Kind:        "group",
		Name:        firstNonEmptyString(input.Name, input.Slug, input.DisplayName, record.Name),
		Description: input.Description,
		ParentID:    firstNonNilInt(input.ParentTeamID, input.ParentID),
	}
	if err := normalizeGroupForWrite(&group); err != nil {
		http.Error(w, strings.ReplaceAll(err.Error(), "group", "team"), http.StatusBadRequest)
		return
	}
	if !a.authorizeTeamUpdate(w, r, record.ID) {
		return
	}
	if err := a.updateGroupRecord(r.Context(), group); err != nil {
		writeGroupMutationError(w, err, "Failed to update team")
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
	if _, err := a.db.Exec(r.Context(), "DELETE FROM groups WHERE id = $1", record.ID); err != nil {
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
	groups, _, listErr := a.visibleGroupsForRequest(r)
	if listErr != nil {
		http.Error(w, listErr.message, listErr.status)
		return
	}
	out := []applicationResponse{}
	for _, group := range groups {
		if group.Kind != "app" {
			continue
		}
		if (group.ParentID == nil && parentID == nil) || (group.ParentID != nil && parentID != nil && *group.ParentID == *parentID) {
			out = append(out, applicationResponseFromGroup(group, records))
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
	group := Group{
		Kind:               "app",
		Name:               firstNonEmptyString(input.Name, input.Slug, input.DisplayName),
		RepoURL:            input.RepoURL,
		RepositoryFullName: input.RepositoryFullName,
		ParentID:           parentID,
	}
	if err := normalizeGroupForWrite(&group); err != nil {
		http.Error(w, strings.ReplaceAll(err.Error(), "group", "application"), http.StatusBadRequest)
		return
	}
	if !a.authorizeApplicationCreate(w, r, parentID) {
		return
	}
	created, err := a.insertGroupRecord(r.Context(), group)
	if err != nil {
		writeGroupMutationError(w, err, "Failed to create application")
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusCreated, applicationResponseFromGroup(created, nil))
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
	if !sameOptionalInt(appRecord.ParentID, parentID) {
		http.Error(w, "application does not belong to this team", http.StatusNotFound)
		return
	}
	var input applicationWriteRequest
	if err := httpapi.DecodeJSON(r, &input); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	group := Group{
		ID:                 appRecord.ID,
		Kind:               "app",
		Name:               firstNonEmptyString(input.Name, input.Slug, input.DisplayName, appRecord.Name),
		RepoURL:            input.RepoURL,
		RepositoryFullName: input.RepositoryFullName,
		ParentID:           parentID,
	}
	if err := normalizeGroupForWrite(&group); err != nil {
		http.Error(w, strings.ReplaceAll(err.Error(), "group", "application"), http.StatusBadRequest)
		return
	}
	if !a.authorizeApplicationUpdate(w, r, appRecord.ID) {
		return
	}
	if err := a.updateGroupRecord(r.Context(), group); err != nil {
		writeGroupMutationError(w, err, "Failed to update application")
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
	if _, err := a.db.Exec(r.Context(), "DELETE FROM groups WHERE id = $1", appRecord.ID); err != nil {
		log.Error().Err(err).Msg("Failed to delete application")
		http.Error(w, "Failed to delete application", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleTeamConfigRepository(w http.ResponseWriter, r *http.Request) {
	if !a.setTeamFolderPathValue(w, r) {
		return
	}
	a.handleFolderConfigRepositoryRoute(w, r)
}

func (a *App) handleTeamNotificationRoute(w http.ResponseWriter, r *http.Request) {
	if !a.setTeamFolderPathValue(w, r) {
		return
	}
	a.handleTeamNotificationRouteByMethod(w, r)
}

func (a *App) handleTeamNotificationRouteByMethod(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.handleGetFolderNotificationRoute(w, r)
	case http.MethodPut:
		a.handleUpsertFolderNotificationRoute(w, r)
	case http.MethodDelete:
		a.handleDeleteFolderNotificationRoute(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleFolderConfigRepositoryRoute(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/sync") && r.Method == http.MethodGet:
		a.handleGetFolderConfigRepositorySyncStatus(w, r)
	case strings.HasSuffix(r.URL.Path, "/sync") && r.Method == http.MethodPost:
		a.handleSyncFolderConfigRepository(w, r)
	case strings.HasSuffix(r.URL.Path, "/drift") && r.Method == http.MethodGet:
		a.handleGetFolderConfigRepositoryDrift(w, r)
	case strings.HasSuffix(r.URL.Path, "/write") && r.Method == http.MethodPost:
		a.handleWriteFolderConfigRepository(w, r)
	case r.Method == http.MethodGet:
		a.handleGetFolderConfigRepository(w, r)
	case r.Method == http.MethodPut:
		a.handleUpsertFolderConfigRepository(w, r)
	case r.Method == http.MethodDelete:
		a.handleDeleteFolderConfigRepository(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) setTeamFolderPathValue(w http.ResponseWriter, r *http.Request) bool {
	record, status, err := a.resolveTeamRecord(r.Context(), r.PathValue("teamID"), false)
	if err != nil {
		http.Error(w, err.Error(), status)
		return false
	}
	path := strings.TrimSpace(record.Path)
	if path == "" {
		path = record.Name
	}
	r.SetPathValue("folderID", path)
	return true
}

func (a *App) authorizeTeamCreate(w http.ResponseWriter, r *http.Request, parentID *int) bool {
	if parentID != nil {
		parentResource, err := a.folderGrantResourceByGroupID(r.Context(), *parentID)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return false
		}
		return a.requireAAADecision(w, r, "folder.create", model.ResourceRef{Type: grantResourceFolder, ID: parentResource.ID})
	}
	return a.requireAAADecision(w, r, "folder.create", model.ResourceRef{Type: grantResourceFolder, ID: "*"})
}

func (a *App) authorizeTeamUpdate(w http.ResponseWriter, r *http.Request, groupID int) bool {
	resource, err := a.folderGrantResourceByGroupID(r.Context(), groupID)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return false
	}
	return a.requireAAADecision(w, r, "folder.update", model.ResourceRef{Type: grantResourceFolder, ID: resource.ID})
}

func (a *App) authorizeTeamDelete(w http.ResponseWriter, r *http.Request, groupID int) bool {
	action, resource, err := a.groupDeleteAuthorizationTarget(r.Context(), groupID)
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

func (a *App) authorizeApplicationUpdate(w http.ResponseWriter, r *http.Request, groupID int) bool {
	return a.authorizeTeamUpdate(w, r, groupID)
}

func (a *App) authorizeApplicationDelete(w http.ResponseWriter, r *http.Request, groupID int) bool {
	return a.authorizeTeamDelete(w, r, groupID)
}

func (a *App) resolveApplicationParent(ctx context.Context, raw string) (*int, map[int]groupPathRecord, int, error) {
	records, err := a.folderPathRecords(ctx)
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

func (a *App) resolveTeamRecord(ctx context.Context, raw string, wantApplication bool) (groupPathRecord, int, error) {
	raw = strings.Trim(strings.TrimSpace(raw), "/")
	if raw == "" || strings.EqualFold(raw, "root") {
		return groupPathRecord{}, http.StatusNotFound, fmt.Errorf("team not found")
	}
	records, err := a.folderPathRecords(ctx)
	if err != nil {
		return groupPathRecord{}, http.StatusInternalServerError, fmt.Errorf("failed to resolve teams")
	}
	var record groupPathRecord
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
		return groupPathRecord{}, http.StatusNotFound, fmt.Errorf("team not found")
	}
	isApplication := record.Kind == "app" || record.RepositoryFullName != "" || record.RepoURL != ""
	if wantApplication && !isApplication {
		return groupPathRecord{}, http.StatusNotFound, fmt.Errorf("application not found")
	}
	if !wantApplication && isApplication {
		return groupPathRecord{}, http.StatusNotFound, fmt.Errorf("team not found")
	}
	return record, http.StatusOK, nil
}

func (a *App) groupForRecord(ctx context.Context, record groupPathRecord) (Group, error) {
	var group Group
	var parentID sql.NullInt32
	var description sql.NullString
	if err := a.db.QueryRow(ctx, "SELECT id, name, COALESCE(kind, 'group'), parent_id, description, COALESCE(repo_url, ''), COALESCE(repository_full_name, '') FROM groups WHERE id = $1", record.ID).Scan(
		&group.ID,
		&group.Name,
		&group.Kind,
		&parentID,
		&description,
		&group.RepoURL,
		&group.RepositoryFullName,
	); err != nil {
		return Group{}, err
	}
	if parentID.Valid {
		parent := int(parentID.Int32)
		group.ParentID = &parent
	}
	if description.Valid {
		group.Description = description.String
	}
	return group, nil
}

func (a *App) insertGroupRecord(ctx context.Context, group Group) (Group, error) {
	query := `INSERT INTO groups (name, kind, parent_id, description, repo_url, repository_full_name) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	if err := a.db.QueryRow(ctx, query, group.Name, group.Kind, group.ParentID, group.Description, group.RepoURL, group.RepositoryFullName).Scan(&group.ID); err != nil {
		return Group{}, err
	}
	return group, nil
}

func (a *App) updateGroupRecord(ctx context.Context, group Group) error {
	query := `UPDATE groups SET name = $1, kind = $2, parent_id = $3, description = $4, repo_url = $5, repository_full_name = $6, updated_at = NOW() WHERE id = $7`
	_, err := a.db.Exec(ctx, query, group.Name, group.Kind, group.ParentID, group.Description, group.RepoURL, group.RepositoryFullName, group.ID)
	return err
}

func writeGroupMutationError(w http.ResponseWriter, err error, fallback string) {
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

func teamResponseFromGroup(group Group, records map[int]groupPathRecord, applicationIDs []int) teamResponse {
	path := groupPathForResponse(group.ID, group.Name, records)
	lastRunAt := nullableSQLTime(group.LastRunAt)
	return teamResponse{
		ID:             group.ID,
		Kind:           "team",
		Name:           group.Name,
		Slug:           group.Name,
		DisplayName:    group.Name,
		Description:    group.Description,
		ParentTeamID:   group.ParentID,
		ParentID:       group.ParentID,
		Path:           path,
		Source:         "database",
		LastRunAt:      lastRunAt,
		NavigationOnly: group.NavigationOnly,
		Applications:   applicationIDs,
	}
}

func applicationResponseFromGroup(group Group, records map[int]groupPathRecord) applicationResponse {
	path := groupPathForResponse(group.ID, group.Name, records)
	teamPath := ""
	if group.ParentID != nil && records != nil {
		teamPath = records[*group.ParentID].Path
	}
	lastRunAt := nullableSQLTime(group.LastRunAt)
	return applicationResponse{
		ID:                 group.ID,
		Kind:               "application",
		Name:               group.Name,
		Slug:               group.Name,
		DisplayName:        groupDisplayNameForApplication(group),
		TeamID:             group.ParentID,
		ParentID:           group.ParentID,
		Path:               path,
		TeamPath:           teamPath,
		RepoURL:            group.RepoURL,
		RepositoryFullName: group.RepositoryFullName,
		Source:             "database",
		LastRunAt:          lastRunAt,
		NavigationOnly:     group.NavigationOnly,
	}
}

func groupPathForResponse(groupID int, fallback string, records map[int]groupPathRecord) string {
	if records != nil {
		if record, ok := records[groupID]; ok && strings.TrimSpace(record.Path) != "" {
			return record.Path
		}
	}
	return strings.Trim(strings.TrimSpace(fallback), "/")
}

func groupDisplayNameForApplication(group Group) string {
	if strings.TrimSpace(group.Name) != "" && !strings.Contains(group.Name, "/") {
		return group.Name
	}
	fullName := strings.Trim(strings.TrimSpace(group.RepositoryFullName), "/")
	if fullName == "" {
		fullName = strings.Trim(strings.TrimSpace(group.Name), "/")
	}
	parts := strings.Split(fullName, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if strings.TrimSpace(parts[i]) != "" {
			return strings.TrimSpace(parts[i])
		}
	}
	return group.Name
}

func nullableSQLTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	return value
}

func firstNonNilInt(values ...*int) *int {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func sameOptionalInt(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"

	"nopsai/pkg/httpapi"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
)

func normalizeGroupForWrite(group *Group) error {
	group.Name = strings.TrimSpace(group.Name)
	group.Description = strings.TrimSpace(group.Description)
	group.Kind = strings.TrimSpace(strings.ToLower(group.Kind))
	group.RepoURL = strings.TrimSpace(group.RepoURL)
	group.RepositoryFullName = strings.Trim(strings.TrimSpace(group.RepositoryFullName), "/")
	if group.Kind == "" {
		if group.RepoURL != "" || group.RepositoryFullName != "" {
			group.Kind = "app"
		} else {
			group.Kind = "group"
		}
	}
	switch group.Kind {
	case "group":
		if group.Name == "" {
			return fmt.Errorf("group name is required")
		}
		if configsync.IsReservedRootGroupName(group.Name) {
			return fmt.Errorf("root is reserved and cannot be used as a group name")
		}
		group.RepoURL = ""
		group.RepositoryFullName = ""
	case "app":
		if group.RepoURL == "" && group.RepositoryFullName != "" {
			group.RepoURL = configsync.CanonicalRepositoryURL(group.RepositoryFullName)
		}
		if group.RepoURL == "" {
			return fmt.Errorf("repository URL is required")
		}
		fullName, err := configsync.RepositoryFullNameFromURL(group.RepoURL)
		if err != nil {
			return err
		}
		group.RepositoryFullName = fullName
		if group.Name == "" {
			group.Name = configsync.RepositoryDisplayNameFromFullName(fullName)
		}
		if _, err := configsync.NormalizeStructureName(group.Name); err != nil {
			return err
		}
		group.Description = ""
	default:
		return fmt.Errorf("kind must be group or app")
	}
	return nil
}

func (a *App) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var group Group
	if err := httpapi.DecodeJSON(r, &group); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := normalizeGroupForWrite(&group); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if group.ParentID != nil {
		parentResource, err := a.folderGrantResourceByGroupID(r.Context(), *group.ParentID)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		if !a.requireAAADecision(w, r, "folder.create", model.ResourceRef{Type: grantResourceFolder, ID: parentResource.ID}) {
			return
		}
	} else if !a.requireAAADecision(w, r, "folder.create", model.ResourceRef{Type: grantResourceFolder, ID: "*"}) {
		return
	}

	query := `INSERT INTO groups (name, kind, parent_id, description, repo_url, repository_full_name) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	err := a.db.QueryRow(context.Background(), query, group.Name, group.Kind, group.ParentID, group.Description, group.RepoURL, group.RepositoryFullName).Scan(&group.ID)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") {
			http.Error(w, "A group or app with this name or repository already exists.", http.StatusConflict)
			return
		}
		log.Error().Err(err).Msg("Failed to create group")
		http.Error(w, "Failed to create group", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(group)
}

func (a *App) handleGetGroups(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(context.Background(), "SELECT id, name, COALESCE(kind, 'group'), parent_id, description, COALESCE(repo_url, ''), COALESCE(repository_full_name, '') FROM groups")
	if err != nil {
		log.Error().Err(err).Msg("Failed to query groups from database")
		http.Error(w, "Failed to retrieve groups", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var allGroups []Group
	groupMap := make(map[int]*Group)

	for rows.Next() {
		var g Group
		var parentID sql.NullInt32
		var description sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &g.Kind, &parentID, &description, &g.RepoURL, &g.RepositoryFullName); err != nil {
			log.Error().Err(err).Msg("Failed to scan group row")
			http.Error(w, "Error processing groups", http.StatusInternalServerError)
			return
		}
		if parentID.Valid {
			pid := int(parentID.Int32)
			g.ParentID = &pid
		}
		if description.Valid {
			g.Description = description.String
		}
		if g.Kind == "" {
			g.Kind = "group"
		}
		if g.Kind == "group" && (g.RepoURL != "" || g.RepositoryFullName != "" || strings.Contains(strings.Trim(g.Name, "/"), "/")) {
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
		allGroups = append(allGroups, g)
	}
	if rows.Err() != nil {
		log.Error().Err(rows.Err()).Msg("Error iterating over group rows")
		http.Error(w, "Error retrieving groups", http.StatusInternalServerError)
		return
	}

	for i := range allGroups {
		groupMap[allGroups[i].ID] = &allGroups[i]
	}

	pathRecords, err := a.folderPathRecords(r.Context())
	if err != nil {
		http.Error(w, "Failed to resolve folder paths", http.StatusInternalServerError)
		return
	}

	resources := make([]model.ResourceRef, 0, len(allGroups))
	resourceByGroupID := make(map[int]model.ResourceRef, len(allGroups))
	for _, group := range allGroups {
		record, ok := pathRecords[group.ID]
		if !ok || strings.TrimSpace(record.Path) == "" {
			continue
		}
		resource := model.ResourceRef{Type: grantResourceFolder, ID: record.Path}
		resources = append(resources, resource)
		resourceByGroupID[group.ID] = resource
	}

	allowedSet, err := a.allowedResourceSet(r, "folder.list", resources)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}

	query := `
        SELECT g.id, MAX(r.started_at)
        FROM groups g
        JOIN pipeline_runs r ON g.id = r.group_id
        GROUP BY g.id
    `
	runRows, err := a.db.Query(context.Background(), query)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query last run times for groups")
	} else {
		defer runRows.Close()
		for runRows.Next() {
			var groupID int
			var lastRunAt sql.NullTime
			if err := runRows.Scan(&groupID, &lastRunAt); err == nil {
				if lastRunAt.Valid {
					if group, ok := groupMap[groupID]; ok {
						group.LastRunAt = &lastRunAt.Time
					}
				}
			}
		}
	}

	filtered := make([]Group, 0, len(allGroups))
	for _, group := range allGroups {
		resource, ok := resourceByGroupID[group.ID]
		if !ok {
			continue
		}
		if _, ok := allowedSet[resourceKey(resource)]; !ok {
			continue
		}
		filtered = append(filtered, group)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filtered)
}

func (a *App) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.Atoi(r.PathValue("groupID"))
	if err != nil {
		http.Error(w, "Invalid group ID", http.StatusBadRequest)
		return
	}

	var group Group
	if err := httpapi.DecodeJSON(r, &group); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := normalizeGroupForWrite(&group); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resource, err := a.folderGrantResourceByGroupID(r.Context(), groupID)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	if !a.requireAAADecision(w, r, "folder.update", model.ResourceRef{Type: grantResourceFolder, ID: resource.ID}) {
		return
	}

	query := `UPDATE groups SET name = $1, kind = $2, parent_id = $3, description = $4, repo_url = $5, repository_full_name = $6, updated_at = NOW() WHERE id = $7`
	_, err = a.db.Exec(context.Background(), query, group.Name, group.Kind, group.ParentID, group.Description, group.RepoURL, group.RepositoryFullName, groupID)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") {
			http.Error(w, "A group or app with this name or repository already exists.", http.StatusConflict)
			return
		}
		log.Error().Err(err).Msg("Failed to update group")
		http.Error(w, "Failed to update group", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *App) handleMoveGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.Atoi(r.PathValue("groupID"))
	if err != nil {
		http.Error(w, "Invalid group ID", http.StatusBadRequest)
		return
	}

	var payload struct {
		ParentID *int `json:"parent_id"`
	}
	if err := httpapi.DecodeJSON(r, &payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	resource, err := a.folderGrantResourceByGroupID(r.Context(), groupID)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	if !a.requireAAADecision(w, r, "folder.move", model.ResourceRef{Type: grantResourceFolder, ID: resource.ID}) {
		return
	}
	if payload.ParentID != nil {
		parentResource, err := a.folderGrantResourceByGroupID(r.Context(), *payload.ParentID)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		if !a.requireAAADecision(w, r, "folder.create", model.ResourceRef{Type: grantResourceFolder, ID: parentResource.ID}) {
			return
		}
	} else if !a.requireAAADecision(w, r, "folder.create", model.ResourceRef{Type: grantResourceFolder, ID: "*"}) {
		return
	}

	// Validation: Prevent moving a group into itself or its own children.
	if payload.ParentID != nil {
		if groupID == *payload.ParentID {
			http.Error(w, "Cannot move a folder into itself.", http.StatusBadRequest)
			return
		}

		var isChild bool
		query := `
			WITH RECURSIVE Descendants AS (
				SELECT id, parent_id FROM groups WHERE id = $1
				UNION ALL
				SELECT g.id, g.parent_id FROM groups g
				INNER JOIN Descendants d ON g.id = d.parent_id
			)
			SELECT EXISTS (SELECT 1 FROM Descendants WHERE id = $2)
		`
		err := a.db.QueryRow(context.Background(), query, *payload.ParentID, groupID).Scan(&isChild)
		if err != nil {
			log.Error().Err(err).Msg("Failed during ancestry check for group move")
			http.Error(w, "Server error during validation.", http.StatusInternalServerError)
			return
		}
		if isChild {
			http.Error(w, "Cannot move a folder into one of its own subfolders.", http.StatusBadRequest)
			return
		}
	}

	_, err = a.db.Exec(context.Background(), "UPDATE groups SET parent_id = $1, updated_at = NOW() WHERE id = $2", payload.ParentID, groupID)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") {
			http.Error(w, "A folder or repository with the same name already exists in the target location.", http.StatusConflict)
			return
		}
		log.Error().Err(err).Msg("Failed to update group parent")
		http.Error(w, "Failed to move group", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.Atoi(r.PathValue("groupID"))
	if err != nil {
		http.Error(w, "Invalid group ID", http.StatusBadRequest)
		return
	}

	action, resource, err := a.groupDeleteAuthorizationTarget(r.Context(), groupID)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	if !a.requireAAADecision(w, r, action, resource) {
		return
	}

	_, err = a.db.Exec(context.Background(), "DELETE FROM groups WHERE id = $1", groupID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete group")
		http.Error(w, "Failed to delete group", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) groupDeleteAuthorizationTarget(ctx context.Context, groupID int) (string, model.ResourceRef, error) {
	if a == nil || a.db == nil {
		return "", model.ResourceRef{}, fmt.Errorf("database unavailable")
	}

	var groupName, kind, repositoryFullName string
	if err := a.db.QueryRow(ctx, `SELECT name, COALESCE(kind, 'group'), COALESCE(repository_full_name, '') FROM groups WHERE id = $1`, groupID).Scan(&groupName, &kind, &repositoryFullName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return "", model.ResourceRef{}, fmt.Errorf("resource not found")
		}
		return "", model.ResourceRef{}, err
	}

	resource, err := a.folderGrantResourceByGroupID(ctx, groupID)
	if err != nil {
		return "", model.ResourceRef{}, err
	}
	action, resourceRef := groupDeleteAuthorizationTargetFromName(groupName, kind, repositoryFullName, resource)
	return action, resourceRef, nil
}

func groupDeleteAuthorizationTargetFromName(groupName, kind, repositoryFullName string, folderResource accessGrantResource) (string, model.ResourceRef) {
	repositoryID := strings.Trim(strings.TrimSpace(repositoryFullName), "/")
	if repositoryID == "" {
		repositoryID = strings.Trim(strings.TrimSpace(groupName), "/")
	}
	if kind == "app" || strings.Contains(repositoryID, "/") {
		return "repository.delete", model.ResourceRef{Type: grantResourceRepo, ID: repositoryID}
	}
	return "folder.delete", model.ResourceRef{Type: grantResourceFolder, ID: folderResource.ID}
}

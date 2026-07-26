package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"nopsai/pkg/httpapi"
	"nopsai/services/aaa/pkg/model"
	aaastore "nopsai/services/aaa/pkg/store"
)

const customUseGrantRole = "use"

type resourceAccessResponse struct {
	Resource         string                     `json:"resource"`
	ResourceType     string                     `json:"resource_type"`
	ResourceID       string                     `json:"resource_id"`
	Visibility       string                     `json:"visibility"`
	UseAccess        resourceAccessUseAccess    `json:"use_access"`
	ManageAccess     resourceAccessManageAccess `json:"manage_access"`
	AccessOverridden bool                       `json:"access_overridden"`
	OverriddenBy     string                     `json:"overridden_by,omitempty"`
	OverriddenAt     *time.Time                 `json:"overridden_at,omitempty"`
}

type resourceAccessUseAccess struct {
	Mode   string                `json:"mode"`
	Grants []accessGrantResponse `json:"grants"`
}

type resourceAccessManageAccess struct {
	Mode string `json:"mode"`
}

type updateResourceAccessRequest struct {
	Visibility string                    `json:"visibility"`
	UseAccess  *updateUseAccessModeInput `json:"use_access"`
}

type updateUseAccessModeInput struct {
	Mode string `json:"mode"`
}

type createResourceGrantRequest struct {
	SubjectType string         `json:"subject_type"`
	SubjectID   string         `json:"subject_id"`
	Actions     []string       `json:"actions"`
	Conditions  map[string]any `json:"conditions"`
}

type createResourceUseGrantInput struct {
	SubjectType  string
	SubjectID    string
	ResourceType string
	ResourceID   string
	Actions      []string
	GrantedBy    string
}

type parsedResourceAccessPath struct {
	Operation    string
	ResourceType string
	ResourceID   string
	GrantID      string
}

type accessTeamResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type accessAuthTeamResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (a *App) handleListAccessAuthTeams(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := a.currentAAASubject(r); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	teams, err := listAccessAuthTeams(r.Context(), a.db)
	if err != nil {
		http.Error(w, "failed to list auth teams", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, teams)
}

func listAccessAuthTeams(ctx context.Context, runner queryRunner) ([]accessAuthTeamResponse, error) {
	rows, err := runner.Query(ctx, `
		SELECT id::text, name
		FROM auth_teams
		ORDER BY LOWER(name), name, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	teams := make([]accessAuthTeamResponse, 0)
	for rows.Next() {
		var team accessAuthTeamResponse
		if err := rows.Scan(&team.ID, &team.Name); err != nil {
			return nil, err
		}
		team.ID = strings.TrimSpace(team.ID)
		team.Name = strings.TrimSpace(team.Name)
		if team.ID == "" || team.Name == "" {
			continue
		}
		teams = append(teams, team)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return teams, nil
}

func (a *App) handleListAccessTeams(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := a.currentAAASubject(r); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	records, err := loadTeamPathRecords(r.Context(), a.db)
	if err != nil {
		http.Error(w, "failed to list teams", http.StatusInternalServerError)
		return
	}
	resources := make([]model.ResourceRef, 0, len(records))
	for _, record := range records {
		if !isSelectableAccessTeamRecord(record) {
			continue
		}
		if strings.TrimSpace(record.Path) == "" {
			continue
		}
		resources = append(resources, model.ResourceRef{Type: grantResourceTeam, ID: record.Path})
	}
	allowedSet, err := a.allowedResourceSet(r, "team.list", resources)
	if err != nil {
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	teams := make([]accessTeamResponse, 0, len(records))
	for _, record := range records {
		if !isSelectableAccessTeamRecord(record) {
			continue
		}
		path := strings.Trim(strings.TrimSpace(record.Path), "/")
		if path == "" {
			continue
		}
		resource := model.ResourceRef{Type: grantResourceTeam, ID: path}
		if _, ok := allowedSet[resourceKey(resource)]; !ok {
			continue
		}
		teams = append(teams, accessTeamResponse{ID: path, Name: "/" + path})
	}
	sort.Slice(teams, func(i, j int) bool {
		return strings.ToLower(teams[i].Name) < strings.ToLower(teams[j].Name)
	})
	_ = httpapi.WriteJSON(w, http.StatusOK, teams)
}

func isSelectableAccessTeamRecord(record teamPathRecord) bool {
	return !strings.EqualFold(strings.TrimSpace(record.Kind), "app") &&
		strings.TrimSpace(record.RepoURL) == "" &&
		strings.TrimSpace(record.RepositoryFullName) == ""
}

func (a *App) handleResourceAccessRoute(w http.ResponseWriter, r *http.Request) {
	parsed, err := parseResourceAccessPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	switch parsed.Operation {
	case "access":
		switch r.Method {
		case http.MethodGet:
			a.handleGetResourceAccess(w, r, parsed.ResourceType, parsed.ResourceID)
		case http.MethodPut:
			a.handleUpdateResourceAccess(w, r, parsed.ResourceType, parsed.ResourceID)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case "grants":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.handleCreateResourceUseGrant(w, r, parsed.ResourceType, parsed.ResourceID)
	case "grant":
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.handleDeleteResourceAccessGrant(w, r, parsed.ResourceType, parsed.ResourceID, parsed.GrantID)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func parseResourceAccessPath(path string) (parsedResourceAccessPath, error) {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/v1/resources/"), "/")
	if trimmed == "" || trimmed == path {
		return parsedResourceAccessPath{}, fmt.Errorf("resource path is required")
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) < 3 {
		return parsedResourceAccessPath{}, fmt.Errorf("invalid resource access path")
	}
	resourceType, err := url.PathUnescape(parts[0])
	if err != nil {
		return parsedResourceAccessPath{}, fmt.Errorf("invalid resource type")
	}
	decodeID := func(values []string) (string, error) {
		if len(values) == 0 {
			return "", fmt.Errorf("resource id is required")
		}
		decoded := make([]string, 0, len(values))
		for _, value := range values {
			part, err := url.PathUnescape(value)
			if err != nil {
				return "", fmt.Errorf("invalid resource id")
			}
			decoded = append(decoded, part)
		}
		return strings.Trim(strings.Join(decoded, "/"), "/"), nil
	}

	last := parts[len(parts)-1]
	switch last {
	case "access":
		resourceID, err := decodeID(parts[1 : len(parts)-1])
		if err != nil {
			return parsedResourceAccessPath{}, err
		}
		return parsedResourceAccessPath{Operation: "access", ResourceType: resourceType, ResourceID: resourceID}, nil
	case "grants":
		resourceID, err := decodeID(parts[1 : len(parts)-1])
		if err != nil {
			return parsedResourceAccessPath{}, err
		}
		return parsedResourceAccessPath{Operation: "grants", ResourceType: resourceType, ResourceID: resourceID}, nil
	default:
		if len(parts) >= 4 && parts[len(parts)-2] == "grants" {
			resourceID, err := decodeID(parts[1 : len(parts)-2])
			if err != nil {
				return parsedResourceAccessPath{}, err
			}
			grantID, err := url.PathUnescape(last)
			if err != nil || strings.TrimSpace(grantID) == "" {
				return parsedResourceAccessPath{}, fmt.Errorf("invalid grant id")
			}
			return parsedResourceAccessPath{Operation: "grant", ResourceType: resourceType, ResourceID: resourceID, GrantID: grantID}, nil
		}
	}
	return parsedResourceAccessPath{}, fmt.Errorf("invalid resource access path")
}

func (a *App) handleGetResourceAccess(w http.ResponseWriter, r *http.Request, resourceType, resourceID string) {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	resource, err := resolveAccessGrantResource(r.Context(), a.db, resourceType, resourceID, true)
	if err != nil {
		writeResourceAccessResolveError(w, err)
		return
	}
	if err := authorizeGrantOperation(r.Context(), subject, resource, productRoleOwner, a.aaaCheck, a.aaaRequestContext(r)); err != nil {
		writeGrantAuthorizationError(w, err)
		return
	}

	resp, err := a.resourceAccessResponse(r.Context(), resource)
	if err != nil {
		http.Error(w, "failed to load resource access", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleUpdateResourceAccess(w http.ResponseWriter, r *http.Request, resourceType, resourceID string) {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	resource, err := resolveAccessGrantResource(r.Context(), a.db, resourceType, resourceID, true)
	if err != nil {
		writeResourceAccessResolveError(w, err)
		return
	}
	if err := authorizeGrantOperation(r.Context(), subject, resource, productRoleOwner, a.aaaCheck, a.aaaRequestContext(r)); err != nil {
		writeGrantAuthorizationError(w, err)
		return
	}

	var req updateResourceAccessRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	visibility := strings.TrimSpace(req.Visibility)
	if visibility == "" && req.UseAccess != nil {
		visibility = req.UseAccess.Mode
	}
	visibility, err = normalizeResourceVisibilityUpdate(visibility)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateResourceVisibilityPolicy(resource.Type, visibility); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.setResourceVisibility(r.Context(), resource, visibility); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	if err := a.markResourceAccessOverride(r.Context(), resource, overrideActorFromSubject(subject)); err != nil {
		http.Error(w, "failed to mark access override", http.StatusInternalServerError)
		return
	}

	resp, err := a.resourceAccessResponse(r.Context(), resource)
	if err != nil {
		http.Error(w, "failed to load resource access", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleCreateResourceUseGrant(w http.ResponseWriter, r *http.Request, resourceType, resourceID string) {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	resource, err := resolveAccessGrantResource(r.Context(), a.db, resourceType, resourceID, true)
	if err != nil {
		writeResourceAccessResolveError(w, err)
		return
	}
	if err := authorizeGrantOperation(r.Context(), subject, resource, productRoleOwner, a.aaaCheck, a.aaaRequestContext(r)); err != nil {
		writeGrantAuthorizationError(w, err)
		return
	}

	var req createResourceGrantRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := validateResourceGrantConditions(req.Conditions); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	record, err := a.CreateResourceUseGrant(r.Context(), createResourceUseGrantInput{
		SubjectType:  req.SubjectType,
		SubjectID:    req.SubjectID,
		ResourceType: resource.Type,
		ResourceID:   resource.ID,
		Actions:      req.Actions,
		GrantedBy:    firstNonEmptyString(subject.Sub, subject.ID),
	})
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case strings.Contains(err.Error(), "not found"):
			status = http.StatusNotFound
		case strings.Contains(err.Error(), "already exists"):
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	if err := a.markResourceAccessOverride(r.Context(), resource, overrideActorFromSubject(subject)); err != nil {
		http.Error(w, "failed to mark access override", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusCreated, accessGrantResponseFromRecord(record))
}

func (a *App) handleDeleteResourceAccessGrant(w http.ResponseWriter, r *http.Request, resourceType, resourceID, grantIDValue string) {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	resource, err := resolveAccessGrantResource(r.Context(), a.db, resourceType, resourceID, true)
	if err != nil {
		writeResourceAccessResolveError(w, err)
		return
	}
	if err := authorizeGrantOperation(r.Context(), subject, resource, productRoleOwner, a.aaaCheck, a.aaaRequestContext(r)); err != nil {
		writeGrantAuthorizationError(w, err)
		return
	}

	grantID, err := parseAccessGrantID(grantIDValue)
	if err != nil {
		http.Error(w, "invalid grant id", http.StatusBadRequest)
		return
	}
	record, err := loadAccessGrantRecord(r.Context(), a.db, grantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if record.ResourceType != resource.Type || record.ResourceID != resource.ID {
		http.Error(w, "grant does not belong to resource", http.StatusBadRequest)
		return
	}
	if err := a.deleteProductRoleGrant(r.Context(), grantID); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	if err := a.markResourceAccessOverride(r.Context(), resource, overrideActorFromSubject(subject)); err != nil {
		http.Error(w, "failed to mark access override", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) resourceAccessResponse(ctx context.Context, resource accessGrantResource) (resourceAccessResponse, error) {
	visibility, err := a.resourceVisibility(ctx, resource.Type, resource.ID)
	if err != nil {
		return resourceAccessResponse{}, err
	}
	grants, err := a.listResourceAccessGrants(ctx, resource)
	if err != nil {
		return resourceAccessResponse{}, err
	}
	override, err := a.resourceAccessOverride(ctx, resource)
	if err != nil {
		return resourceAccessResponse{}, err
	}
	return resourceAccessResponse{
		Resource:     formatResourceLabel(resource.Type, resource.ID),
		ResourceType: resource.Type,
		ResourceID:   externalGrantResourceID(resource.Type, resource.Display, resource.ID),
		Visibility:   visibility,
		UseAccess: resourceAccessUseAccess{
			Mode:   resourceAccessModeForVisibility(visibility),
			Grants: grants,
		},
		ManageAccess:     resourceAccessManageAccess{Mode: "owners"},
		AccessOverridden: override.overridden,
		OverriddenBy:     override.by,
		OverriddenAt:     override.at,
	}, nil
}

func (a *App) listResourceAccessGrants(ctx context.Context, resource accessGrantResource) ([]accessGrantResponse, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			subject_type,
			subject_id,
			subject_display,
			role_name,
			resource_type,
			resource_id,
			resource_display,
			inherit,
			granted_by,
			created_at,
			managed_by_config_repo,
			config_source_path,
			config_source_commit_sha,
			managed_by_identity_provider,
			identity_provider_id,
			external_team_name
		FROM access_grants
		WHERE resource_type = $1 AND resource_id = $2
		ORDER BY role_name ASC, subject_type ASC, subject_display ASC, subject_id ASC
	`, resource.Type, resource.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grants := make([]accessGrantResponse, 0)
	for rows.Next() {
		var record accessGrantRecord
		if err := rows.Scan(
			&record.ID,
			&record.SubjectType,
			&record.SubjectID,
			&record.SubjectDisplay,
			&record.RoleName,
			&record.ResourceType,
			&record.ResourceID,
			&record.ResourceDisplay,
			&record.Inherit,
			&record.GrantedBy,
			&record.CreatedAt,
			&record.ManagedByConfig,
			&record.ConfigSourcePath,
			&record.ConfigSourceCommitSHA,
			&record.ManagedByIdentityProvider,
			&record.IdentityProviderID,
			&record.ExternalTeamName,
		); err != nil {
			return nil, err
		}
		grants = append(grants, accessGrantResponseFromRecord(record))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	inherited, err := a.listInheritedResourceAccessGrants(ctx, resource)
	if err != nil {
		return nil, err
	}
	grants = append(grants, inherited...)
	return grants, nil
}

func (a *App) listInheritedResourceAccessGrants(ctx context.Context, resource accessGrantResource) ([]accessGrantResponse, error) {
	parentTeams := inheritedAccessParentTeams(resource)
	if len(parentTeams) == 0 {
		return nil, nil
	}
	rows, err := a.db.Query(ctx, `
		SELECT
			id,
			subject_type,
			subject_id,
			subject_display,
			role_name,
			resource_type,
			resource_id,
			resource_display,
			inherit,
			granted_by,
			created_at,
			managed_by_config_repo,
			config_source_path,
			config_source_commit_sha,
			managed_by_identity_provider,
			identity_provider_id,
			external_team_name
		FROM access_grants
		WHERE resource_type = $1
		  AND resource_id = ANY($2)
		  AND inherit = TRUE
		ORDER BY resource_id ASC, role_name ASC, subject_type ASC, subject_display ASC, subject_id ASC
	`, grantResourceTeam, parentTeams)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grants := make([]accessGrantResponse, 0)
	for rows.Next() {
		var record accessGrantRecord
		if err := rows.Scan(
			&record.ID,
			&record.SubjectType,
			&record.SubjectID,
			&record.SubjectDisplay,
			&record.RoleName,
			&record.ResourceType,
			&record.ResourceID,
			&record.ResourceDisplay,
			&record.Inherit,
			&record.GrantedBy,
			&record.CreatedAt,
			&record.ManagedByConfig,
			&record.ConfigSourcePath,
			&record.ConfigSourceCommitSHA,
			&record.ManagedByIdentityProvider,
			&record.IdentityProviderID,
			&record.ExternalTeamName,
		); err != nil {
			return nil, err
		}
		if !inheritedGrantAppliesToResource(record, resource.Type) {
			continue
		}
		record.InheritedFromResourceType = record.ResourceType
		record.InheritedFromResourceID = record.ResourceID
		record.InheritedFromResourceDisplay = record.ResourceDisplay
		grants = append(grants, accessGrantResponseFromRecord(record))
	}
	return grants, rows.Err()
}

func inheritedAccessParentTeams(resource accessGrantResource) []string {
	switch resource.Type {
	case grantResourcePipeline, grantResourceStep:
		path, _ := model.SplitPipelineID(resource.ID)
		return teamPathPrefixes(path)
	case grantResourceScope:
		return teamPathPrefixes(resource.ID)
	case grantResourceTeam:
		prefixes := teamPathPrefixes(resource.ID)
		if len(prefixes) == 0 {
			return nil
		}
		return prefixes[:len(prefixes)-1]
	case grantResourceRepo, grantResourceTrigger:
		return teamPathPrefixes(repositoryParentPath(resource.ID))
	case grantResourceKnowledgeContext:
		_, team, _, _ := splitKnowledgeContextIdentifier(resource.ID)
		return teamPathPrefixes(team)
	case grantResourceKnowledgeConnection:
		team, _, _ := splitKnowledgeConnectionIdentifier(resource.ID)
		return teamPathPrefixes(team)
	case grantResourceLLMProfile, grantResourceAgentProfile, grantResourceMCPServer, grantResourceMCPProfile:
		path, _ := model.SplitPipelineID(resource.ID)
		return teamPathPrefixes(path)
	case grantResourceSecret, grantResourceVariable:
		repoName, scope, _ := model.ParseNamedResourceID(resource.ID)
		if scope != "" {
			return teamPathPrefixes(scope)
		}
		return teamPathPrefixes(repositoryParentPath(repoName))
	default:
		return nil
	}
}

func teamPathPrefixes(path string) []string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" || path == generalGrantID {
		return nil
	}
	parts := strings.Split(path, "/")
	prefixes := make([]string, 0, len(parts))
	for idx := range parts {
		prefix := strings.Trim(strings.Join(parts[:idx+1], "/"), "/")
		if prefix != "" {
			prefixes = append(prefixes, prefix)
		}
	}
	return prefixes
}

func inheritedGrantAppliesToResource(record accessGrantRecord, targetResourceType string) bool {
	if record.RoleName == productRoleAdmin {
		return true
	}
	if record.RoleName == customUseGrantRole {
		action, err := defaultUseActionForResource(targetResourceType)
		return err == nil && actionAppliesToGrantResource(action, targetResourceType)
	}
	return len(applicableProductRoleActions(record.RoleName, targetResourceType)) > 0
}

type resourceAccessOverrideRecord struct {
	overridden bool
	by         string
	at         *time.Time
}

func (a *App) resourceAccessOverride(ctx context.Context, resource accessGrantResource) (resourceAccessOverrideRecord, error) {
	var record resourceAccessOverrideRecord
	var updatedAt time.Time
	err := a.db.QueryRow(ctx, `
		SELECT overridden_by, updated_at
		FROM resource_access_overrides
		WHERE resource_type = $1 AND resource_id = $2
	`, resource.Type, resource.ID).Scan(&record.by, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return resourceAccessOverrideRecord{}, nil
		}
		return resourceAccessOverrideRecord{}, err
	}
	record.overridden = true
	record.at = &updatedAt
	return record, nil
}

func (a *App) markResourceAccessOverride(ctx context.Context, resource accessGrantResource, actor string) error {
	if a == nil || a.db == nil {
		return fmt.Errorf("database unavailable")
	}
	_, err := a.db.Exec(ctx, `
		INSERT INTO resource_access_overrides (resource_type, resource_id, overridden_by, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (resource_type, resource_id)
		DO UPDATE SET overridden_by = EXCLUDED.overridden_by, updated_at = NOW()
	`, resource.Type, resource.ID, strings.TrimSpace(actor))
	return err
}

func overrideActorFromSubject(subject model.Subject) string {
	return firstNonEmptyString(subject.Sub, subject.Email, subject.ID, subject.Type)
}

func (a *App) setResourceVisibility(ctx context.Context, resource accessGrantResource, visibility string) error {
	if a == nil || a.db == nil {
		return fmt.Errorf("database unavailable")
	}
	return setResourceVisibilityWithRunner(ctx, a.db, resource, visibility)
}

func setResourceVisibilityWithRunner(ctx context.Context, runner execRunner, resource accessGrantResource, visibility string) error {
	if runner == nil {
		return fmt.Errorf("database unavailable")
	}
	visibility = normalizeResourceVisibility(visibility)
	var tag pgconn.CommandTag
	var err error
	switch resource.Type {
	case grantResourcePipeline:
		path, name := model.SplitPipelineID(resource.ID)
		tag, err = runner.Exec(ctx, `UPDATE pipelines SET visibility = $1 WHERE path = $2 AND name = $3`, visibility, path, name)
	case grantResourceStep:
		path, name := model.SplitPipelineID(resource.ID)
		tag, err = runner.Exec(ctx, `UPDATE steps SET visibility = $1 WHERE path = $2 AND name = $3`, visibility, path, name)
	case grantResourceConfig:
		tag, err = runner.Exec(ctx, `UPDATE config_repositories SET visibility = $1 WHERE id::text = $2 OR scope_id = $2`, visibility, resource.ID)
	case grantResourceDashboard:
		teamPath, slug, splitErr := splitDashboardRef(resource.ID)
		if splitErr != nil {
			return splitErr
		}
		tag, err = runner.Exec(ctx, `
			WITH RECURSIVE team_paths AS (
				SELECT id, name::text AS path
				FROM teams
				WHERE parent_id IS NULL
				UNION ALL
				SELECT child.id, team_paths.path || '/' || child.name
				FROM teams child
				JOIN team_paths ON child.parent_id = team_paths.id
			)
			UPDATE dashboards d
			SET visibility = $1
			FROM team_paths tp
			WHERE d.team_id = tp.id
			  AND tp.path = $2
			  AND d.slug = $3
		`, visibility, teamPath, slug)
	default:
		_, err = runner.Exec(ctx, `
			INSERT INTO resource_visibility (resource_type, resource_id, visibility, updated_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (resource_type, resource_id)
			DO UPDATE SET visibility = EXCLUDED.visibility, updated_at = NOW()
		`, resource.Type, resource.ID, visibility)
		return err
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("resource not found")
	}
	return nil
}

func normalizeResourceVisibilityUpdate(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", resourceVisibilityTeam, "only_this_team", "team_only":
		return resourceVisibilityTeam, nil
	case resourceVisibilityRestricted, "specific", "specific_teams_or_repositories":
		return resourceVisibilityRestricted, nil
	case resourceVisibilityWorkspace, "everyone_in_workspace", "public":
		return resourceVisibilityWorkspace, nil
	default:
		return "", fmt.Errorf("visibility must be team, restricted, or workspace")
	}
}

func resourceAccessModeForVisibility(visibility string) string {
	switch normalizeResourceVisibility(visibility) {
	case resourceVisibilityRestricted:
		return "specific_teams_or_repositories"
	case resourceVisibilityWorkspace:
		return "everyone_in_workspace"
	default:
		return "only_this_team"
	}
}

func validateResourceVisibilityPolicy(resourceType, visibility string) error {
	if visibility != resourceVisibilityWorkspace {
		return nil
	}
	switch resourceType {
	case grantResourcePipeline, grantResourceStep, grantResourceConfig, grantResourceKnowledgeContext, grantResourceKnowledgeConnection,
		grantResourceLLMProfile, grantResourceAgentProfile, grantResourceMCPServer, grantResourceMCPProfile, grantResourceDashboard:
		return nil
	case grantResourceScope, grantResourceSecret, grantResourceVariable, grantResourceRunner:
		return fmt.Errorf("workspace visibility is not available for this resource yet")
	default:
		return fmt.Errorf("workspace visibility is not available for this resource")
	}
}

func validateResourceGrantConditions(conditions map[string]any) error {
	for key, value := range conditions {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "branches", "events":
			if isEmptyConditionValue(value) {
				continue
			}
			return fmt.Errorf("branch and event conditions are not enforced yet")
		default:
			if !isEmptyConditionValue(value) {
				return fmt.Errorf("conditions are not supported yet")
			}
		}
	}
	return nil
}

func isEmptyConditionValue(value any) bool {
	if value == nil {
		return true
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case []string:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		raw, _ := json.Marshal(typed)
		return string(raw) == "null" || string(raw) == `""` || string(raw) == "[]" || string(raw) == "{}"
	}
}

func defaultUseActionForResource(resourceType string) (string, error) {
	switch resourceType {
	case grantResourcePipeline:
		return "pipeline.use", nil
	case grantResourceScope:
		return "scope.use", nil
	case grantResourceStep:
		return "step.use", nil
	case grantResourceSecret:
		return "secret.use", nil
	case grantResourceVariable:
		return "variable.use", nil
	case grantResourceRunner:
		return "runner.use", nil
	case grantResourceConfig:
		return "config_repo.use", nil
	case grantResourceKnowledgeContext:
		return "knowledge_context.use", nil
	case grantResourceKnowledgeConnection:
		return "knowledge_connection.use", nil
	case grantResourceDashboard:
		return "dashboard.read", nil
	case grantResourceLLMProfile:
		return "llm_profile.use", nil
	case grantResourceAgentProfile:
		return "agent_profile.use", nil
	case grantResourceMCPServer:
		return "mcp_server.use", nil
	case grantResourceMCPProfile:
		return "mcp_profile.use", nil
	default:
		return "", fmt.Errorf("use grants are not supported for %s", resourceType)
	}
}

func normalizeUseGrantActions(resourceType string, actions []string) ([]string, error) {
	if len(actions) == 0 {
		action, err := defaultUseActionForResource(resourceType)
		if err != nil {
			return nil, err
		}
		return []string{action}, nil
	}
	seen := make(map[string]struct{}, len(actions))
	normalized := make([]string, 0, len(actions))
	for _, raw := range actions {
		action := strings.TrimSpace(raw)
		if action == "" {
			continue
		}
		if !isResourceUseGrantAction(resourceType, action) {
			return nil, fmt.Errorf("only use actions can be granted from this endpoint")
		}
		if !actionAppliesToGrantResource(action, resourceType) {
			return nil, fmt.Errorf("%s does not apply to %s", action, resourceType)
		}
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		normalized = append(normalized, action)
	}
	if len(normalized) == 0 {
		return nil, fmt.Errorf("at least one action is required")
	}
	return normalized, nil
}

func isResourceUseGrantAction(resourceType, action string) bool {
	if resourceType == grantResourceDashboard {
		return action == "dashboard.read"
	}
	return strings.HasSuffix(action, ".use")
}

func (a *App) CreateResourceUseGrant(ctx context.Context, input createResourceUseGrantInput) (accessGrantRecord, error) {
	var record accessGrantRecord
	if a == nil || a.db == nil {
		return record, fmt.Errorf("database unavailable")
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return record, err
	}
	defer tx.Rollback(ctx)

	subject, err := resolveResourceUseGrantSubject(ctx, tx, input.SubjectType, input.SubjectID)
	if err != nil {
		return record, err
	}
	if locked, err := isDefaultAdminGrantSubject(ctx, tx, subject.Type, subject.ID); err != nil {
		return record, err
	} else if locked {
		return record, fmt.Errorf("cannot modify default admin role assignments")
	}
	resource, err := resolveAccessGrantResource(ctx, tx, input.ResourceType, input.ResourceID, true)
	if err != nil {
		return record, err
	}
	actions, err := normalizeUseGrantActions(resource.Type, input.Actions)
	if err != nil {
		return record, err
	}

	var existingID int64
	err = tx.QueryRow(ctx, `
		SELECT id
		FROM access_grants
		WHERE subject_type = $1 AND subject_id = $2 AND resource_type = $3 AND resource_id = $4
	`, subject.Type, subject.ID, resource.Type, resource.ID).Scan(&existingID)
	switch {
	case err == nil:
		record, err = loadAccessGrantRecord(ctx, tx, existingID)
		if err != nil {
			return accessGrantRecord{}, err
		}
		if _, ok := productRoleDefinitions[record.RoleName]; ok {
			roleActions := applicableProductRoleActions(record.RoleName, resource.Type)
			if !actionsContainAll(roleActions, actions) {
				return accessGrantRecord{}, fmt.Errorf("grant already exists with role %s", record.RoleName)
			}
			if err := tx.Commit(ctx); err != nil {
				return accessGrantRecord{}, err
			}
			return record, nil
		}
	case errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows):
		grantedBy := strings.TrimSpace(input.GrantedBy)
		err = tx.QueryRow(ctx, `
			INSERT INTO access_grants (
				subject_type, subject_id, subject_display, role_name,
				resource_type, resource_id, resource_display, inherit, granted_by
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, FALSE, $8)
			RETURNING id, created_at
		`, subject.Type, subject.ID, subject.Display, customUseGrantRole, resource.Type, resource.ID, resource.Display, grantedBy).Scan(&record.ID, &record.CreatedAt)
		if err != nil {
			return accessGrantRecord{}, err
		}
		record.SubjectType = subject.Type
		record.SubjectID = subject.ID
		record.SubjectDisplay = subject.Display
		record.RoleName = customUseGrantRole
		record.ResourceType = resource.Type
		record.ResourceID = resource.ID
		record.ResourceDisplay = resource.Display
		record.Inherit = false
		record.GrantedBy = grantedBy
	default:
		return accessGrantRecord{}, err
	}

	if subject.Type != grantSubjectTeam {
		for _, action := range actions {
			if err := aaastore.UpsertResourceACL(ctx, tx, aaastore.ResourceACL{
				ResourceType:  resource.Type,
				ResourceID:    resource.ID,
				SubjectType:   subject.Type,
				SubjectID:     subject.ID,
				AccessGrantID: &record.ID,
				Action:        action,
				Effect:        "allow",
			}); err != nil {
				return accessGrantRecord{}, err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return accessGrantRecord{}, err
	}
	return record, nil
}

func resolveResourceUseGrantSubject(ctx context.Context, runner queryRunner, rawType, rawID string) (accessGrantSubject, error) {
	switch strings.ToLower(strings.TrimSpace(rawType)) {
	case grantSubjectTeam:
		team, err := resolveAccessGrantTeam(ctx, runner, rawID, true)
		if err != nil {
			return accessGrantSubject{}, err
		}
		if team.ID == generalGrantID {
			return accessGrantSubject{}, fmt.Errorf("team grants require a concrete team")
		}
		return accessGrantSubject{Type: grantSubjectTeam, ID: team.ID, Display: team.Display}, nil
	default:
		return resolveAccessGrantSubject(ctx, runner, rawType, rawID)
	}
}

func actionsContainAll(available, required []string) bool {
	if len(required) == 0 {
		return true
	}
	seen := make(map[string]struct{}, len(available))
	for _, action := range available {
		seen[action] = struct{}{}
		if action == "*" {
			return true
		}
	}
	for _, action := range required {
		if _, ok := seen[action]; !ok {
			return false
		}
	}
	return true
}

func writeResourceAccessResolveError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if strings.Contains(err.Error(), "not found") {
		status = http.StatusNotFound
	}
	http.Error(w, err.Error(), status)
}

func writeGrantAuthorizationError(w http.ResponseWriter, err error) {
	if err.Error() == "forbidden" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
}

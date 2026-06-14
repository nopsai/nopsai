package nopsai

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"nopsai/pkg/httpapi"
	"nopsai/services/aaa/pkg/model"
)

func (a *App) handleCreateAccessGrant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req createAccessGrantRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	roleName, err := normalizeProductRoleName(req.Role)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resource, err := resolveAccessGrantResource(r.Context(), a.db, req.ResourceType, req.ResourceID, true)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}

	inherit := resource.Type == grantResourceFolder
	if req.Inherit != nil {
		inherit = *req.Inherit
	}
	if err := authorizeGrantOperation(r.Context(), subject, resource, roleName, a.aaaCheck, a.aaaRequestContext(r)); err != nil {
		if err.Error() == "forbidden" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return
	}

	record, err := a.GrantProductRole(r.Context(), GrantProductRoleInput{
		SubjectType:  req.SubjectType,
		SubjectID:    req.SubjectID,
		RoleName:     roleName,
		ResourceType: resource.Type,
		ResourceID:   resource.ID,
		Inherit:      inherit,
		GrantedBy:    firstNonEmptyString(subject.Sub, subject.ID),
	})
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case strings.Contains(err.Error(), "already exists"):
			status = http.StatusConflict
		case strings.Contains(err.Error(), "not found"):
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}

	_ = httpapi.WriteJSON(w, http.StatusCreated, accessGrantResponseFromRecord(record))
}

func (a *App) handleListAccessGrants(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	resourceType := strings.TrimSpace(r.URL.Query().Get("resource_type"))
	resourceID := strings.TrimSpace(r.URL.Query().Get("resource_id"))
	if resourceType == "" && resourceID == "" {
		decision, err := a.aaaCheck(r.Context(), subject, "iam.admin", model.ResourceRef{Type: "iam", ID: "admin"}, a.aaaRequestContext(r))
		if err != nil {
			http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
			return
		}
		if !decision.Allowed {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	} else {
		resource, err := resolveAccessGrantResource(r.Context(), a.db, resourceType, resourceID, true)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		if err := authorizeGrantOperation(r.Context(), subject, resource, productRoleOwner, a.aaaCheck, a.aaaRequestContext(r)); err != nil {
			if err.Error() == "forbidden" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
			return
		}
	}

	query := `
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
			external_group_name
		FROM access_grants
		WHERE ($1 = '' OR resource_type = $1)
		  AND ($2 = '' OR resource_id = $2)
		  AND ($3 = '' OR role_name = $3)
		ORDER BY resource_type ASC, resource_id ASC, subject_type ASC, subject_display ASC
	`

	roleFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("role")))
	rows, err := a.db.Query(r.Context(), query, normalizedOrEmpty(resourceType, resourceID), normalizedResourceIDOrEmpty(resourceType, resourceID), roleFilter)
	if err != nil {
		http.Error(w, "failed to list grants", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	records := make([]accessGrantResponse, 0)
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
			&record.ExternalGroupName,
		); err != nil {
			http.Error(w, "failed to read grants", http.StatusInternalServerError)
			return
		}
		records = append(records, accessGrantResponseFromRecord(record))
	}

	_ = httpapi.WriteJSON(w, http.StatusOK, records)
}

func normalizedOrEmpty(resourceType, resourceID string) string {
	if strings.TrimSpace(resourceType) == "" || strings.TrimSpace(resourceID) == "" {
		return ""
	}
	value, err := normalizeAccessGrantResourceType(resourceType)
	if err != nil {
		return ""
	}
	return value
}

func normalizedResourceIDOrEmpty(resourceType, resourceID string) string {
	if strings.TrimSpace(resourceType) == "" || strings.TrimSpace(resourceID) == "" {
		return ""
	}
	resource, err := resolveAccessGrantResource(context.Background(), &noopQueryRunner{}, resourceType, resourceID, false)
	if err != nil {
		return ""
	}
	return resource.ID
}

type noopQueryRunner struct{}

func (n *noopQueryRunner) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, fmt.Errorf("unsupported")
}
func (n *noopQueryRunner) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, fmt.Errorf("unsupported")
}
func (n *noopQueryRunner) QueryRow(context.Context, string, ...any) pgx.Row { return nil }

func (a *App) handleDeleteAccessGrant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	grantID, err := parseAccessGrantID(r.PathValue("grantID"))
	if err != nil {
		http.Error(w, "invalid grant id", http.StatusBadRequest)
		return
	}

	record, err := loadAccessGrantRecord(r.Context(), a.db, grantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	resource := accessGrantResource{
		Type:    record.ResourceType,
		ID:      record.ResourceID,
		Display: record.ResourceDisplay,
	}
	if err := authorizeGrantOperation(r.Context(), subject, resource, record.RoleName, a.aaaCheck, a.aaaRequestContext(r)); err != nil {
		if err.Error() == "forbidden" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return
	}

	if _, err := a.deleteProductRoleGrant(r.Context(), grantID); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleGetEffectivePermissions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	action := strings.TrimSpace(r.URL.Query().Get("action"))
	resourceType := strings.TrimSpace(r.URL.Query().Get("resource_type"))
	resourceID := strings.TrimSpace(r.URL.Query().Get("resource_id"))
	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("action", action),
		httpapi.RequiredString("resource_type", resourceType),
		httpapi.RequiredString("resource_id", resourceID),
	); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resource, err := resolveAccessGrantResource(r.Context(), a.db, resourceType, resourceID, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	decision, err := a.aaaCheck(r.Context(), subject, action, model.ResourceRef{Type: resource.Type, ID: resource.ID}, a.aaaRequestContext(r))
	if err != nil {
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return
	}

	response := buildEffectivePermissionResponse(action, resource, decision)
	_ = httpapi.WriteJSON(w, http.StatusOK, response)
}

func buildEffectivePermissionResponse(action string, resource accessGrantResource, decision model.Decision) effectivePermissionResponse {
	resp := effectivePermissionResponse{
		Allowed:       decision.Allowed,
		Action:        action,
		Resource:      formatResourceLabel(resource.Type, resource.DisplayOrID()),
		Reason:        strings.TrimSpace(decision.Reason),
		Inherited:     strings.Contains(decision.Reason, "inheritance"),
		MatchedPolicy: decision.MatchedPolicy,
	}

	if decision.MatchedPolicy == nil {
		return resp
	}

	if roleName, _ := decision.MatchedPolicy["role_name"].(string); strings.TrimSpace(roleName) != "" {
		resp.MatchedRole = strings.TrimSpace(roleName)
	}
	if subjectType, _ := decision.MatchedPolicy["subject_type"].(string); strings.TrimSpace(subjectType) != "" {
		subjectID, _ := decision.MatchedPolicy["subject_id"].(string)
		resp.MatchedSubject = formatSubjectLabel(subjectType, subjectID)
	}
	if matchedResourceType, _ := decision.MatchedPolicy["resource_type"].(string); strings.TrimSpace(matchedResourceType) != "" {
		matchedResourceID, _ := decision.MatchedPolicy["resource_id"].(string)
		resp.MatchedResource = formatResourceLabel(matchedResourceType, matchedResourceID)
		if resp.Inherited {
			resp.SourceParentResource = resp.MatchedResource
		}
	}
	if matchedAction, _ := decision.MatchedPolicy["action"].(string); strings.TrimSpace(matchedAction) != "" {
		resp.LowLevelPermission = matchedAction
	}

	resp.Reason = buildHumanReadableDecisionReason(resp, decision)
	return resp
}

func buildHumanReadableDecisionReason(resp effectivePermissionResponse, decision model.Decision) string {
	if resp.MatchedRole == "" || resp.MatchedSubject == "" || resp.MatchedResource == "" {
		return strings.TrimSpace(decision.Reason)
	}
	if resp.Inherited {
		return fmt.Sprintf("%s has %s on %s, inherited by %s", resp.MatchedSubject, resp.MatchedRole, resp.MatchedResource, resp.Resource)
	}
	return fmt.Sprintf("%s has %s on %s", resp.MatchedSubject, resp.MatchedRole, resp.MatchedResource)
}

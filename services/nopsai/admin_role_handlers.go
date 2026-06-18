package nopsai

import (
	"context"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	"nopsai/pkg/httpapi"
	aaastore "nopsai/services/aaa/pkg/store"
)

func (a *App) handleCreateRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req createRoleRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	req.Role = strings.TrimSpace(req.Role)
	req.Name = strings.TrimSpace(req.Name)
	req.Object = strings.TrimSpace(req.Object)
	req.Action = strings.TrimSpace(req.Action)
	permission, err := parseAdminRolePermission(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := httpapi.ValidateRequired(httpapi.RequiredString("role", permission.Role)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if isProtectedAdminRoleName(permission.Role) {
		http.Error(w, "default roles cannot be modified", http.StatusBadRequest)
		return
	}

	objectValue := formatAdminPermissionObject(permission.ResourceType, permission.ResourceID)
	actionValue := formatAdminPermissionAction(permission.Effect, permission.Action)
	displayName := adminPermissionDisplayName(permission.Name, objectValue, actionValue)

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	description := ""
	if permission.Role == policyTemplateRole {
		description = "Reusable UI policy templates"
	}
	if err := aaastore.EnsureRole(r.Context(), tx, permission.Role, description); err != nil {
		http.Error(w, "failed to prepare aaa role", http.StatusInternalServerError)
		return
	}
	if err := aaastore.EnsureRolePermission(r.Context(), tx, aaastore.RolePermission{
		RoleName:     permission.Role,
		ResourceType: permission.ResourceType,
		ResourceID:   permission.ResourceID,
		Action:       permission.Action,
		Effect:       permission.Effect,
	}); err != nil {
		http.Error(w, "failed to create aaa role permission", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		DELETE FROM role_permissions
		WHERE role = $1 AND obj = $2 AND act = $3
	`, permission.Role, objectValue, actionValue); err != nil {
		http.Error(w, "failed to refresh legacy role metadata", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO role_permissions (role, name, obj, act)
		VALUES ($1, $2, $3, $4)
	`, permission.Role, displayName, objectValue, actionValue); err != nil {
		http.Error(w, "failed to create role metadata", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to save role permission", http.StatusInternalServerError)
		return
	}
	a.reloadAuthz(r.Context())
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req createRoleRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	req.Role = strings.TrimSpace(req.Role)
	req.Object = strings.TrimSpace(req.Object)
	req.Action = strings.TrimSpace(req.Action)
	permission, err := parseAdminRolePermission(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := httpapi.ValidateRequired(httpapi.RequiredString("role", permission.Role)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if isProtectedAdminRoleName(permission.Role) {
		http.Error(w, "default roles cannot be deleted", http.StatusBadRequest)
		return
	}
	var assignments int
	if err := a.db.QueryRow(r.Context(), `
		SELECT
			COALESCE((SELECT COUNT(*) FROM auth_role_bindings WHERE role_name = $1), 0) +
			COALESCE((SELECT COUNT(*) FROM user_roles WHERE role = $1), 0)
	`, permission.Role).Scan(&assignments); err != nil {
		http.Error(w, "failed to check role usage", http.StatusInternalServerError)
		return
	}
	if assignments > 0 {
		http.Error(w, "cannot delete policy from a role that is assigned to users", http.StatusBadRequest)
		return
	}

	objectValue := formatAdminPermissionObject(permission.ResourceType, permission.ResourceID)
	actionValue := formatAdminPermissionAction(permission.Effect, permission.Action)

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	authTag, err := aaastore.DeleteRolePermission(r.Context(), tx, aaastore.RolePermission{
		RoleName:     permission.Role,
		ResourceType: permission.ResourceType,
		ResourceID:   permission.ResourceID,
		Action:       permission.Action,
		Effect:       permission.Effect,
	})
	if err != nil {
		http.Error(w, "failed to delete aaa role permission", http.StatusInternalServerError)
		return
	}
	legacyTag, err := tx.Exec(r.Context(), `
		DELETE FROM role_permissions
		WHERE role = $1 AND obj = $2 AND act = $3
	`, permission.Role, objectValue, actionValue)
	if err != nil {
		http.Error(w, "failed to delete role metadata", http.StatusInternalServerError)
		return
	}
	if authTag.RowsAffected() == 0 && legacyTag.RowsAffected() == 0 {
		http.Error(w, "role permission not found", http.StatusNotFound)
		return
	}
	if err := aaastore.DeleteEmptyRole(r.Context(), tx, permission.Role); err != nil {
		http.Error(w, "failed to clean up empty role", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to save role deletion", http.StatusInternalServerError)
		return
	}
	a.reloadAuthz(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleListRoles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	metadataRows, err := a.db.Query(r.Context(), `
		SELECT role, COALESCE(name, ''), obj, act
		FROM role_permissions
	`)
	if err != nil {
		http.Error(w, "failed to list role metadata", http.StatusInternalServerError)
		return
	}
	defer metadataRows.Close()
	nameByKey := make(map[string]string)
	for metadataRows.Next() {
		var role, name, objectValue, actionValue string
		if err := metadataRows.Scan(&role, &name, &objectValue, &actionValue); err != nil {
			http.Error(w, "failed to scan role metadata", http.StatusInternalServerError)
			return
		}
		nameByKey[adminPermissionMetadataKey(role, objectValue, actionValue)] = strings.TrimSpace(name)
	}

	rows, err := a.db.Query(r.Context(), `
		SELECT role_name, resource_type, resource_id, action, effect
		FROM auth_role_permissions
		ORDER BY role_name ASC, resource_type ASC, resource_id ASC, effect ASC, action ASC
	`)
	if err != nil {
		http.Error(w, "failed to list aaa role permissions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type rolePerm struct {
		Role   string `json:"role"`
		Name   string `json:"name"`
		Obj    string `json:"obj"`
		Act    string `json:"act"`
		Effect string `json:"effect,omitempty"`
	}
	perms := make([]rolePerm, 0)
	for rows.Next() {
		var roleName, resourceType, resourceID, action, effect string
		if err := rows.Scan(&roleName, &resourceType, &resourceID, &action, &effect); err != nil {
			http.Error(w, "failed to scan aaa role permission", http.StatusInternalServerError)
			return
		}
		objectValue := formatAdminPermissionObject(resourceType, resourceID)
		actionValue := formatAdminPermissionAction(effect, action)
		name := nameByKey[adminPermissionMetadataKey(roleName, objectValue, actionValue)]
		if name == "" && roleName == defaultAdminRole && objectValue == "*:*" && actionValue == "*" {
			name = nameByKey[adminPermissionMetadataKey(roleName, "/*", ".*")]
		}
		perms = append(perms, rolePerm{
			Role:   roleName,
			Name:   adminPermissionDisplayName(name, objectValue, actionValue),
			Obj:    objectValue,
			Act:    actionValue,
			Effect: effect,
		})
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, perms)
}

func (a *App) reloadAuthz(ctx context.Context) {
	if a == nil || a.authz == nil {
		return
	}
	if err := a.authz.LoadPolicies(ctx, a.db); err != nil {
		log.Error().Err(err).Msg("failed to reload authz policies")
	}
}

package main

import (
	"fmt"
	"strings"
)

const policyTemplateRole = "__policy_template__"

type adminRolePermission struct {
	Role         string
	Name         string
	ResourceType string
	ResourceID   string
	Action       string
	Effect       string
}

func parseAdminRolePermission(req createRoleRequest) (adminRolePermission, error) {
	objectValue := strings.TrimSpace(req.Object)
	if objectValue == "" && (strings.TrimSpace(req.ResourceType) != "" || strings.TrimSpace(req.ResourceID) != "") {
		objectValue = formatAdminPermissionObject(req.ResourceType, req.ResourceID)
	}

	resourceType, resourceID, err := parseAdminPermissionObject(objectValue)
	if err != nil {
		return adminRolePermission{}, err
	}

	effect, action, err := parseAdminPermissionAction(req.Effect, req.Action)
	if err != nil {
		return adminRolePermission{}, err
	}

	return adminRolePermission{
		Role:         strings.TrimSpace(req.Role),
		Name:         strings.TrimSpace(req.Name),
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       action,
		Effect:       effect,
	}, nil
}

func parseAdminPermissionObject(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	switch raw {
	case "":
		return "", "", fmt.Errorf("resource selector is required")
	case "*", "*:*", "/*":
		return "*", "*", nil
	}
	if strings.HasPrefix(raw, "/v1/") || strings.HasPrefix(raw, "/") {
		return "", "", fmt.Errorf("legacy path policies are not supported; use resource_type:resource_id such as pipeline:*")
	}

	resourceType, resourceID, hasColon := strings.Cut(raw, ":")
	resourceType = strings.TrimSpace(resourceType)
	resourceID = strings.TrimSpace(resourceID)
	if !hasColon {
		resourceID = "*"
	}
	if resourceType == "" {
		return "", "", fmt.Errorf("resource_type is required")
	}
	if resourceID == "" {
		resourceID = "*"
	}
	return resourceType, resourceID, nil
}

func parseAdminPermissionAction(explicitEffect, raw string) (string, string, error) {
	effect := strings.ToLower(strings.TrimSpace(explicitEffect))
	action := strings.TrimSpace(raw)

	if effect == "" {
		fields := strings.Fields(action)
		if len(fields) > 1 {
			switch strings.ToLower(fields[0]) {
			case "allow", "deny":
				effect = strings.ToLower(fields[0])
				action = strings.TrimSpace(strings.TrimPrefix(action, fields[0]))
			}
		}
	}
	if effect == "" {
		effect = "allow"
	}
	switch effect {
	case "allow", "deny":
	default:
		return "", "", fmt.Errorf("effect must be allow or deny")
	}

	switch action {
	case "", ".*":
		action = "*"
	}
	if action == "" {
		return "", "", fmt.Errorf("action is required")
	}
	return effect, action, nil
}

func formatAdminPermissionObject(resourceType, resourceID string) string {
	resourceType = strings.TrimSpace(resourceType)
	resourceID = strings.TrimSpace(resourceID)
	if resourceType == "" || resourceType == "*" {
		resourceType = "*"
	}
	if resourceID == "" || resourceID == "*" {
		resourceID = "*"
	}
	return resourceType + ":" + resourceID
}

func formatAdminPermissionAction(effect, action string) string {
	effect = strings.ToLower(strings.TrimSpace(effect))
	action = strings.TrimSpace(action)
	if action == "" {
		action = "*"
	}
	if effect == "deny" {
		return "deny " + action
	}
	return action
}

func adminPermissionMetadataKey(role, object, action string) string {
	return strings.TrimSpace(role) + "::" + strings.TrimSpace(object) + "::" + strings.TrimSpace(action)
}

func adminPermissionDisplayName(fallback, object, action string) string {
	if fallback = strings.TrimSpace(fallback); fallback != "" {
		return fallback
	}
	return defaultPolicyName(object, action)
}

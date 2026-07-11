package nopsai

import (
	"fmt"
	"strconv"
	"strings"

	"nopsai/services/aaa/pkg/model"
)

func accessGrantResponseFromRecord(record accessGrantRecord) accessGrantResponse {
	source := "ui"
	if record.ManagedByConfig {
		source = "gitops"
	} else if record.ManagedByIdentityProvider {
		source = "sso"
	}
	inheritedFromResourceID := externalGrantResourceID(record.InheritedFromResourceType, record.InheritedFromResourceDisplay, record.InheritedFromResourceID)
	inheritedFromResource := ""
	if record.InheritedFromResourceType != "" && inheritedFromResourceID != "" {
		inheritedFromResource = formatResourceLabel(record.InheritedFromResourceType, inheritedFromResourceID)
	}
	return accessGrantResponse{
		ID:                        formatAccessGrantID(record.ID),
		SubjectType:               record.SubjectType,
		SubjectID:                 record.SubjectID,
		SubjectDisplay:            record.SubjectDisplay,
		Role:                      record.RoleName,
		ResourceType:              record.ResourceType,
		ResourceID:                externalGrantResourceID(record.ResourceType, record.ResourceDisplay, record.ResourceID),
		Inherit:                   record.Inherit,
		GrantedBy:                 record.GrantedBy,
		CreatedAt:                 record.CreatedAt,
		ManagedByConfigRepo:       record.ManagedByConfig,
		ConfigSourcePath:          record.ConfigSourcePath,
		ConfigSourceCommitSHA:     record.ConfigSourceCommitSHA,
		ManagedByIdentityProvider: record.ManagedByIdentityProvider,
		IdentityProviderID:        record.IdentityProviderID,
		ExternalTeamName:          record.ExternalTeamName,
		Source:                    source,
		InheritedFromResourceType: record.InheritedFromResourceType,
		InheritedFromResourceID:   inheritedFromResourceID,
		InheritedFromResource:     inheritedFromResource,
	}
}

func externalGrantResourceID(resourceType, display, internalID string) string {
	if resourceType == grantResourceTeam && internalID == generalGrantID {
		return rootGrantID
	}
	if strings.TrimSpace(display) != "" {
		return display
	}
	if resourceType == grantResourceTeam {
		return "/" + strings.Trim(strings.TrimSpace(internalID), "/")
	}
	if resourceType == grantResourcePlatform {
		return "platform"
	}
	return internalID
}

func formatAccessGrantID(id int64) string {
	return fmt.Sprintf("grant_%d", id)
}

func parseAccessGrantID(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "grant_")
	return strconv.ParseInt(raw, 10, 64)
}

func formatSubjectLabel(subjectType, subjectID string) string {
	subjectID = strings.TrimSpace(subjectID)
	switch subjectType {
	case grantSubjectTeam:
		return "team " + subjectID
	case model.SubjectTypeAuthTeam:
		return "team " + subjectID
	case model.SubjectTypeRepository:
		return "repository " + subjectID
	case model.SubjectTypeTrigger:
		return "trigger " + subjectID
	case model.SubjectTypeServiceAccount:
		return "service account " + subjectID
	case model.SubjectTypeInternalService:
		return "service " + subjectID
	default:
		return "user " + subjectID
	}
}

func formatResourceLabel(resourceType, resourceID string) string {
	resourceType = strings.TrimSpace(resourceType)
	resourceID = strings.TrimSpace(resourceID)
	if resourceType == grantResourceSecret || resourceType == grantResourceVariable {
		if label := formatNamedResourceLabel(resourceType, resourceID); label != "" {
			return label
		}
	}
	if resourceType == grantResourceScope && resourceID == "" {
		resourceID = "default"
	}
	if resourceType == grantResourceTeam && resourceID == generalGrantID {
		resourceID = rootGrantID
	}
	if resourceType == grantResourceTeam && resourceID != "" && resourceID != rootGrantID && !strings.HasPrefix(resourceID, "/") {
		resourceID = "/" + strings.Trim(resourceID, "/")
	}
	return resourceType + ":" + resourceID
}

func formatNamedResourceLabel(resourceType, resourceID string) string {
	repoName, scope, name := model.ParseNamedResourceID(resourceID)
	if strings.TrimSpace(name) == "" {
		return ""
	}
	if strings.TrimSpace(scope) == "" {
		scope = "default"
	}
	parts := []string{
		"name=" + strings.TrimSpace(name),
		"scope=" + strings.Trim(strings.TrimSpace(scope), "/"),
	}
	if repoName = strings.TrimSpace(repoName); repoName != "" {
		parts = append(parts, "repo="+repoName)
	}
	return strings.TrimSpace(resourceType) + ":" + strings.Join(parts, " ")
}

func isRootGrantResourceID(raw string) bool {
	switch strings.ToLower(strings.Trim(strings.TrimSpace(raw), "/")) {
	case rootGrantID:
		return true
	default:
		return false
	}
}

func isRootPathAlias(raw string) bool {
	switch strings.ToLower(strings.Trim(strings.TrimSpace(raw), "/")) {
	case rootGrantID:
		return true
	default:
		return false
	}
}

func stripRootPathPrefix(raw string) (string, bool) {
	path := strings.Trim(strings.TrimSpace(raw), "/")
	if isRootPathAlias(path) {
		return "", true
	}
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return "", true
	}
	if !isRootPathAlias(parts[0]) {
		return path, false
	}
	parts = parts[1:]
	if len(parts) == 0 {
		return "", true
	}
	return strings.Join(parts, "/"), false
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func (r accessGrantResource) DisplayOrID() string {
	return firstNonEmptyString(r.Display, r.ID)
}

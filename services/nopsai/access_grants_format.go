package nopsai

import (
	"fmt"
	"strconv"
	"strings"

	"nopsai/services/aaa/pkg/model"
)

func accessGrantResponseFromRecord(record accessGrantRecord) accessGrantResponse {
	ownershipSource := strings.TrimSpace(record.Source)
	if ownershipSource == "" {
		ownershipSource = grantSourceLocal
	}
	providerID := firstNonEmptyString(record.ProviderID, record.IdentityProviderID)
	externalGroupID := firstNonEmptyString(record.ExternalGroupID, record.ExternalTeamName)
	inheritedFromResourceID := externalGrantResourceID(record.InheritedFromResourceType, record.InheritedFromResourceDisplay, record.InheritedFromResourceID)
	inheritedFromResource := ""
	if record.InheritedFromResourceType != "" && inheritedFromResourceID != "" {
		inheritedFromResource = formatResourceLabel(record.InheritedFromResourceType, inheritedFromResourceID)
	}
	return accessGrantResponse{
		ID:                        formatAccessGrantID(record.ID),
		SubjectType:               record.SubjectType,
		SubjectID:                 externalGrantSubjectID(record.SubjectType, record.SubjectID),
		SubjectDisplay:            externalGrantSubjectDisplay(record.SubjectType, record.SubjectID, record.SubjectDisplay),
		Role:                      record.RoleName,
		ResourceType:              record.ResourceType,
		ResourceID:                externalGrantResourceID(record.ResourceType, record.ResourceDisplay, record.ResourceID),
		Inherit:                   record.Inherit,
		GrantedBy:                 record.GrantedBy,
		CreatedAt:                 record.CreatedAt,
		ManagedByConfigRepo:       record.ManagedByConfig,
		ConfigSourcePath:          record.ConfigSourcePath,
		ConfigSourceCommitSHA:     record.ConfigSourceCommitSHA,
		ManagedByIdentityProvider: record.ManagedByIdentityProvider || ownershipSource == grantSourceIDP,
		IdentityProviderID:        providerID,
		ExternalTeamName:          externalGroupID,
		ProviderID:                providerID,
		ExternalGroupID:           externalGroupID,
		ExternalRoleID:            record.ExternalRoleID,
		Source:                    ownershipSource,
		InheritedFromResourceType: record.InheritedFromResourceType,
		InheritedFromResourceID:   inheritedFromResourceID,
		InheritedFromResource:     inheritedFromResource,
	}
}

func externalGrantResourceID(resourceType, display, internalID string) string {
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

func externalGrantSubjectID(subjectType, internalID string) string {
	return internalID
}

func externalGrantSubjectDisplay(subjectType, internalID, display string) string {
	if subjectType == grantSubjectTeam && internalID == globalGrantID {
		return globalGrantID
	}
	return display
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
	if resourceType == grantResourceTeam && resourceID != "" && !isGlobalGrantResourceID(resourceID) && !strings.HasPrefix(resourceID, "/") {
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

func isGlobalGrantResourceID(raw string) bool {
	return strings.EqualFold(strings.Trim(strings.TrimSpace(raw), "/"), globalGrantID)
}

func isRetiredGlobalTeamAlias(raw string) bool {
	normalized := strings.ToLower(strings.Trim(strings.TrimSpace(raw), "/"))
	return normalized == rootGrantID || normalized == "general" || normalized == "__general__"
}

func isGlobalPathAlias(raw string) bool {
	return strings.EqualFold(strings.Trim(strings.TrimSpace(raw), "/"), globalGrantID)
}

func stripGlobalPathPrefix(raw string) (string, bool) {
	path := strings.Trim(strings.TrimSpace(raw), "/")
	if isGlobalPathAlias(path) {
		return "", true
	}
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return "", true
	}
	if !isGlobalPathAlias(parts[0]) {
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

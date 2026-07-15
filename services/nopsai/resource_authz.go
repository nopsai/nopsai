package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
)

const (
	resourceVisibilityTeam       = "team"
	resourceVisibilityRestricted = "restricted"
	resourceVisibilityWorkspace  = "workspace"

	resourceUseReasonDirectGrant     = "explicit_grant"
	resourceUseReasonSameTeam        = "same_team"
	resourceUseReasonWorkspacePublic = "workspace_public"
	resourceUseReasonScopeAccess     = "scope_access"
	resourceUseReasonAuthError       = "authorization_error"
	resourceUseReasonDenied          = "denied"
)

type TeamRef struct {
	ID    int    `json:"id,omitempty"`
	Path  string `json:"path,omitempty"`
	Valid bool   `json:"valid"`
}

type ResourceUseAuthInput struct {
	CallerType string `json:"caller_type"`
	CallerID   string `json:"caller_id"`

	Action       string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`

	EventType string `json:"event_type,omitempty"`
	Ref       string `json:"ref,omitempty"`
	Repo      string `json:"repo,omitempty"`
}

type ResourceUseAuthResult struct {
	Allowed         bool   `json:"allowed"`
	Reason          string `json:"reason"`
	Action          string `json:"action,omitempty"`
	ResourceType    string `json:"resource_type,omitempty"`
	ResourceID      string `json:"resource_id,omitempty"`
	MatchedGrantID  string `json:"matched_grant_id,omitempty"`
	MatchedResource string `json:"matched_resource,omitempty"`
	CallerTeam      string `json:"caller_team,omitempty"`
	ResourceTeam    string `json:"resource_team,omitempty"`
	Visibility      string `json:"visibility,omitempty"`
	EventType       string `json:"event_type,omitempty"`
	Ref             string `json:"ref,omitempty"`
	Repo            string `json:"repo,omitempty"`
}

type resourceUseBatchCheckRequest struct {
	CallerType string                 `json:"caller_type"`
	CallerID   string                 `json:"caller_id"`
	Checks     []ResourceUseAuthInput `json:"checks"`
}

type runAuthorizationSnapshot struct {
	TriggerSource string                  `json:"trigger_source,omitempty"`
	Caller        string                  `json:"caller,omitempty"`
	Checks        []ResourceUseAuthResult `json:"checks"`
}

func (a *App) AuthorizeResourceUse(ctx context.Context, input ResourceUseAuthInput) (ResourceUseAuthResult, error) {
	result := ResourceUseAuthResult{
		Allowed:      false,
		Reason:       resourceUseReasonDenied,
		Action:       strings.TrimSpace(input.Action),
		ResourceType: strings.TrimSpace(input.ResourceType),
		ResourceID:   strings.Trim(strings.TrimSpace(input.ResourceID), "/"),
		EventType:    strings.TrimSpace(input.EventType),
		Ref:          strings.TrimSpace(input.Ref),
		Repo:         strings.TrimSpace(input.Repo),
	}
	if a == nil || !a.aaaAvailable() {
		return result, fmt.Errorf("authorization unavailable")
	}

	callerType, err := normalizeResourceUseCallerType(input.CallerType)
	if err != nil {
		return result, err
	}
	callerID := normalizeResourceUseCallerID(callerType, input.CallerID)
	if callerID == "" {
		return result, fmt.Errorf("caller_id is required")
	}
	resourceType, err := normalizeAccessGrantResourceType(input.ResourceType)
	if err != nil {
		return result, err
	}
	resourceID, err := normalizeResourceUseResourceID(resourceType, input.ResourceID)
	if err != nil {
		return result, fmt.Errorf("resource_id is required")
	}
	action := strings.TrimSpace(input.Action)
	if action == "" {
		return result, fmt.Errorf("action is required")
	}

	result.Action = action
	result.ResourceType = resourceType
	result.ResourceID = resourceID

	subject := subjectForResourceUse(callerType, callerID)
	resource := model.ResourceRef{Type: resourceType, ID: resourceID}
	requestContext := map[string]any{
		"event_type": strings.TrimSpace(input.EventType),
		"ref":        strings.TrimSpace(input.Ref),
		"repo":       strings.TrimSpace(input.Repo),
		"caller":     formatSubjectLabel(callerType, callerID),
	}
	visibility, err := a.resourceVisibility(ctx, resourceType, resourceID)
	if err != nil {
		return result, err
	}
	result.Visibility = visibility

	decision, err := a.aaaCheck(ctx, subject, action, resource, requestContext)
	if err != nil {
		return result, err
	}
	if decision.Allowed {
		allowReason := a.resourceUseAllowReason(ctx, callerType, callerID, resource, decision)
		result.Allowed = true
		result.Reason = allowReason
		result.MatchedGrantID = a.matchedGrantIDFromDecision(ctx, decision)
		result.MatchedResource = matchedResourceLabel(decision, resource)
		return result, nil
	}
	if isExplicitResourceUseDeny(decision) {
		result.MatchedResource = matchedResourceLabel(decision, resource)
		return result, nil
	}

	callerTeam, _ := a.ResolveCallerTeam(ctx, callerType, callerID)
	resourceTeam, _ := a.ResolveResourceTeam(ctx, resourceType, resourceID)
	if callerTeam.Valid {
		result.CallerTeam = callerTeam.Path
	}
	if resourceTeam.Valid {
		result.ResourceTeam = resourceTeam.Path
	}
	if callerTeam.Valid && resourceTeam.Valid && IsSameTeamBoundary(callerTeam.Path, resourceTeam.Path) {
		if sameTeamResourceUseAllowed(resourceType, visibility) {
			result.Allowed = true
			result.Reason = resourceUseReasonSameTeam
			result.MatchedResource = formatResourceLabel(grantResourceTeam, resourceTeam.Path)
			return result, nil
		}
		if teamAllowed, teamDecision, checkErr := a.callerHasTeamAction(ctx, subject, action, resourceTeam.Path); checkErr != nil {
			return result, checkErr
		} else if teamAllowed {
			result.Allowed = true
			result.Reason = resourceUseReasonSameTeam
			result.MatchedGrantID = a.matchedGrantIDFromDecision(ctx, teamDecision)
			result.MatchedResource = matchedResourceLabel(teamDecision, model.ResourceRef{Type: grantResourceTeam, ID: resourceTeam.Path})
			if result.MatchedResource == "" {
				result.MatchedResource = formatResourceLabel(grantResourceTeam, resourceTeam.Path)
			}
			return result, nil
		}
	}

	if teamAllowed, grantID, grantTeam, checkErr := a.callerHasExplicitTeamUseGrant(ctx, subject, action, resourceType, resourceID, callerTeam); checkErr != nil {
		return result, checkErr
	} else if teamAllowed {
		result.Allowed = true
		result.Reason = resourceUseReasonDirectGrant
		result.MatchedGrantID = formatAccessGrantID(grantID)
		result.MatchedResource = formatResourceLabel(grantResourceTeam, grantTeam)
		return result, nil
	}

	if scopeID, ok := parentScopeForRuntimeResourceUse(action, resourceType, resourceID); ok {
		scopeResult, scopeErr := a.AuthorizeResourceUse(ctx, ResourceUseAuthInput{
			CallerType:   callerType,
			CallerID:     callerID,
			Action:       "scope.use",
			ResourceType: grantResourceScope,
			ResourceID:   scopeID,
			EventType:    input.EventType,
			Ref:          input.Ref,
			Repo:         input.Repo,
		})
		if scopeErr != nil {
			return result, scopeErr
		}
		if scopeResult.Allowed {
			result.Allowed = true
			result.Reason = resourceUseReasonScopeAccess
			result.MatchedGrantID = scopeResult.MatchedGrantID
			result.MatchedResource = scopeResult.MatchedResource
			if result.MatchedResource == "" {
				result.MatchedResource = formatResourceLabel(grantResourceScope, scopeID)
			}
			if result.CallerTeam == "" {
				result.CallerTeam = scopeResult.CallerTeam
			}
			if result.ResourceTeam == "" {
				result.ResourceTeam = scopeResult.ResourceTeam
			}
			return result, nil
		}
	}

	if visibility == resourceVisibilityWorkspace {
		result.Allowed = true
		result.Reason = resourceUseReasonWorkspacePublic
		result.MatchedResource = formatResourceLabel(resourceType, resourceID)
		return result, nil
	}

	return result, nil
}

func sameTeamResourceUseAllowed(resourceType, visibility string) bool {
	switch normalizeResourceVisibility(visibility) {
	case resourceVisibilityTeam, resourceVisibilityRestricted:
	default:
		return false
	}

	switch strings.TrimSpace(resourceType) {
	case grantResourcePipeline, grantResourceScope, grantResourceStep, grantResourceKnowledgeContext, grantResourceKnowledgeConnection,
		grantResourceLLMProfile, grantResourceAgentProfile, grantResourceMCPServer, grantResourceMCPProfile:
		return true
	default:
		return false
	}
}

func isExplicitResourceUseDeny(decision model.Decision) bool {
	if decision.Allowed {
		return false
	}
	reason := strings.ToLower(strings.TrimSpace(decision.Reason))
	if reason == "" || reason == "default_deny" || !strings.Contains(reason, "deny") {
		return false
	}
	return len(decision.MatchedPolicy) > 0
}

func parentScopeForRuntimeResourceUse(action, resourceType, resourceID string) (string, bool) {
	action = strings.TrimSpace(action)
	resourceType = strings.TrimSpace(resourceType)
	if resourceType != grantResourceSecret && resourceType != grantResourceVariable {
		return "", false
	}
	if action != "secret.use" && action != "variable.use" {
		return "", false
	}
	_, scope, _ := model.ParseNamedResourceID(resourceID)
	scope = strings.Trim(strings.TrimSpace(scope), "/")
	if scope == "" {
		return "default", true
	}
	return scope, true
}

func normalizeResourceUseResourceID(resourceType, raw string) (string, error) {
	resourceID := strings.Trim(strings.TrimSpace(raw), "/")
	if resourceType == grantResourceScope {
		scopeID, _, scopeDisplay := normalizeScopeGrantResourceID(resourceID)
		if strings.TrimSpace(scopeDisplay) == "" {
			return "", fmt.Errorf("resource_id is required")
		}
		return scopeID, nil
	}
	if resourceType == grantResourceSecret || resourceType == grantResourceVariable {
		resourceID = runtimeNamedResourceIDForResource(resourceID)
	}
	if resourceType == grantResourceKnowledgeContext {
		kind, team, name, err := splitKnowledgeContextIdentifier(resourceID)
		if err != nil {
			return "", err
		}
		resourceID = buildKnowledgeContextIdentifier(kind, team, name)
	}
	if resourceType == grantResourceKnowledgeConnection {
		team, name, err := splitKnowledgeConnectionIdentifier(resourceID)
		if err != nil {
			return "", err
		}
		resourceID = buildKnowledgeConnectionIdentifier(team, name)
	}
	if resourceID == "" {
		return "", fmt.Errorf("resource_id is required")
	}
	return resourceID, nil
}

func normalizeResourceUseCallerType(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case model.SubjectTypeUser:
		return model.SubjectTypeUser, nil
	case model.SubjectTypeAuthTeam:
		return model.SubjectTypeAuthTeam, nil
	case grantSubjectRepository:
		return model.SubjectTypeRepository, nil
	case grantSubjectTrigger:
		return model.SubjectTypeTrigger, nil
	case grantSubjectServiceAccount:
		return model.SubjectTypeServiceAccount, nil
	case grantSubjectService, model.SubjectTypeInternalService:
		return model.SubjectTypeInternalService, nil
	default:
		return "", fmt.Errorf("caller_type must be user, auth_team, repository, trigger, service_account, or internal_service")
	}
}

func normalizeResourceUseCallerID(callerType, raw string) string {
	value := strings.Trim(strings.TrimSpace(raw), "/")
	for _, prefix := range []string{
		callerType + ":",
		model.SubjectTypeRepository + ":",
		model.SubjectTypeTrigger + ":",
		model.SubjectTypeServiceAccount + ":",
		"service:",
	} {
		if strings.HasPrefix(strings.ToLower(value), prefix) {
			value = strings.TrimSpace(value[len(prefix):])
			break
		}
	}
	return strings.Trim(strings.TrimSpace(value), "/")
}

func subjectForResourceUse(callerType, callerID string) model.Subject {
	callerType = strings.TrimSpace(callerType)
	callerID = strings.TrimSpace(callerID)
	if callerType == model.SubjectTypeUser {
		if _, err := uuidParse(callerID); err == nil {
			return model.Subject{Type: model.SubjectTypeUser, ID: callerID}
		}
		if strings.Contains(callerID, "@") {
			return model.Subject{Type: model.SubjectTypeUser, Email: callerID}
		}
		return model.Subject{Type: model.SubjectTypeUser, Sub: callerID}
	}
	return model.Subject{Type: callerType, ID: callerID}
}

func uuidParse(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) != 36 {
		return "", fmt.Errorf("not a uuid")
	}
	for idx, ch := range value {
		switch idx {
		case 8, 13, 18, 23:
			if ch != '-' {
				return "", fmt.Errorf("not a uuid")
			}
		default:
			if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
				return "", fmt.Errorf("not a uuid")
			}
		}
	}
	return value, nil
}

func (a *App) resourceUseAllowReason(ctx context.Context, callerType, callerID string, resource model.ResourceRef, decision model.Decision) string {
	matchedType, _ := decision.MatchedPolicy["resource_type"].(string)
	matchedID, _ := decision.MatchedPolicy["resource_id"].(string)
	if strings.TrimSpace(matchedType) != grantResourceTeam {
		return resourceUseReasonDirectGrant
	}

	callerTeam, _ := a.ResolveCallerTeam(ctx, callerType, callerID)
	resourceTeam, _ := a.ResolveResourceTeam(ctx, resource.Type, resource.ID)
	if callerTeam.Valid && resourceTeam.Valid && IsSameTeamBoundary(callerTeam.Path, resourceTeam.Path) {
		return resourceUseReasonSameTeam
	}
	if matchedID != "" && resourceTeam.Valid && IsSameTeamBoundary(matchedID, resourceTeam.Path) {
		return resourceUseReasonSameTeam
	}
	return resourceUseReasonDirectGrant
}

func decisionMatchedTeam(decision model.Decision) bool {
	if decision.MatchedPolicy == nil {
		return false
	}
	matchedType, _ := decision.MatchedPolicy["resource_type"].(string)
	return strings.TrimSpace(matchedType) == grantResourceTeam
}

func (a *App) ResolveCallerTeam(ctx context.Context, callerType, callerID string) (TeamRef, error) {
	callerType = strings.TrimSpace(callerType)
	callerID = strings.Trim(strings.TrimSpace(callerID), "/")
	if callerID == "" || a == nil || a.db == nil {
		return TeamRef{}, nil
	}
	switch callerType {
	case model.SubjectTypeRepository:
		return a.resolveRepositoryTeamRef(ctx, callerID)
	case model.SubjectTypeTrigger:
		var repositoryName string
		err := a.db.QueryRow(ctx, `SELECT repository_name FROM triggers WHERE repository_name = $1 LIMIT 1`, callerID).Scan(&repositoryName)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
			return TeamRef{}, err
		}
		if repositoryName == "" {
			repositoryName = callerID
		}
		return a.resolveRepositoryTeamRef(ctx, repositoryName)
	default:
		return TeamRef{}, nil
	}
}

func (a *App) ResolveResourceTeam(ctx context.Context, resourceType, resourceID string) (TeamRef, error) {
	resourceType = strings.TrimSpace(resourceType)
	resourceID = strings.Trim(strings.TrimSpace(resourceID), "/")
	switch resourceType {
	case grantResourcePipeline, grantResourceStep:
		path, _ := model.SplitPipelineID(resourceID)
		return teamRefFromPath(path), nil
	case grantResourceScope:
		return teamRefFromPath(resourceID), nil
	case grantResourceTeam:
		return teamRefFromPath(resourceID), nil
	case grantResourceRepo, grantResourceTrigger:
		if a == nil || a.db == nil {
			return teamRefFromPath(repositoryParentPath(resourceID)), nil
		}
		ref, err := a.resolveRepositoryTeamRef(ctx, resourceID)
		if err != nil || ref.Valid {
			return ref, err
		}
		return teamRefFromPath(repositoryParentPath(resourceID)), nil
	case grantResourceSecret, grantResourceVariable:
		repoName, scope, _ := model.ParseNamedResourceID(resourceID)
		if scope != "" {
			return teamRefFromPath(scope), nil
		}
		return teamRefFromPath(repositoryParentPath(repoName)), nil
	case grantResourceKnowledgeContext:
		_, team, _, err := splitKnowledgeContextIdentifier(resourceID)
		if err != nil {
			return TeamRef{}, err
		}
		return teamRefFromPath(team), nil
	case grantResourceKnowledgeConnection:
		team, _, err := splitKnowledgeConnectionIdentifier(resourceID)
		if err != nil {
			return TeamRef{}, err
		}
		return teamRefFromPath(team), nil
	case grantResourceLLMProfile, grantResourceAgentProfile, grantResourceMCPServer, grantResourceMCPProfile:
		path, _ := model.SplitPipelineID(resourceID)
		return teamRefFromPath(path), nil
	default:
		return TeamRef{}, nil
	}
}

func (a *App) resolveRepositoryTeamRef(ctx context.Context, repositoryID string) (TeamRef, error) {
	if teamPath, found, err := a.repositoryTriggerTeamPathForRepository(ctx, repositoryID); err != nil || found {
		if err != nil {
			return TeamRef{}, err
		}
		if strings.Trim(strings.TrimSpace(teamPath), "/") != rootGrantID {
			return teamRefFromPath(teamPath), nil
		}
	}
	owner, repo := splitRepositoryID(repositoryID)
	matches, err := a.repositoryTeamMatches(ctx, owner, repo)
	if err != nil {
		return TeamRef{}, err
	}
	if len(matches) == 0 {
		return teamRefFromPath(repositoryID), nil
	}
	return TeamRef{ID: matches[0].ID, Path: strings.Trim(matches[0].Path, "/"), Valid: true}, nil
}

func teamRefFromPath(path string) TeamRef {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" || path == generalGrantID {
		return TeamRef{Path: generalGrantID, Valid: path == generalGrantID}
	}
	return TeamRef{Path: path, Valid: true}
}

func splitRepositoryID(repositoryID string) (string, string) {
	repositoryID = strings.Trim(strings.TrimSpace(repositoryID), "/")
	if repositoryID == "" {
		return "", ""
	}
	parts := strings.Split(repositoryID, "/")
	if len(parts) < 2 {
		return "", repositoryID
	}
	return parts[len(parts)-2], parts[len(parts)-1]
}

func repositoryParentPath(repositoryID string) string {
	repositoryID = strings.Trim(strings.TrimSpace(repositoryID), "/")
	parts := strings.Split(repositoryID, "/")
	if len(parts) <= 2 {
		return ""
	}
	return strings.Join(parts[:len(parts)-2], "/")
}

func IsSameTeamBoundary(callerTeam, resourceTeam string) bool {
	callerTeam = strings.Trim(strings.TrimSpace(callerTeam), "/")
	resourceTeam = strings.Trim(strings.TrimSpace(resourceTeam), "/")
	if callerTeam == "" || resourceTeam == "" || callerTeam == generalGrantID || resourceTeam == generalGrantID {
		return false
	}
	if callerTeam == resourceTeam {
		return true
	}
	if strings.HasPrefix(resourceTeam, callerTeam+"/") || strings.HasPrefix(callerTeam, resourceTeam+"/") {
		return true
	}
	return firstPathSegment(callerTeam) == firstPathSegment(resourceTeam)
}

func firstPathSegment(path string) string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return ""
	}
	if idx := strings.Index(path, "/"); idx >= 0 {
		return path[:idx]
	}
	return path
}

func (a *App) callerHasTeamAction(ctx context.Context, subject model.Subject, action, teamPath string) (bool, model.Decision, error) {
	teamPath = strings.Trim(strings.TrimSpace(teamPath), "/")
	if teamPath == "" {
		teamPath = generalGrantID
	}
	decision, err := a.aaaCheck(ctx, subject, action, model.ResourceRef{Type: grantResourceTeam, ID: teamPath}, map[string]any{"resource_use_check": "same_team"})
	if err != nil {
		return false, model.Decision{}, err
	}
	return decision.Allowed, decision, nil
}

func (a *App) callerHasExplicitTeamUseGrant(ctx context.Context, subject model.Subject, action, resourceType, resourceID string, callerTeam TeamRef) (bool, int64, string, error) {
	if a == nil || a.db == nil || !callerTeam.Valid {
		return false, 0, "", nil
	}
	callerTeam.Path = strings.Trim(strings.TrimSpace(callerTeam.Path), "/")
	if callerTeam.Path == "" || callerTeam.Path == generalGrantID {
		return false, 0, "", nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT id, subject_id
		FROM access_grants
		WHERE subject_type = $1
		  AND resource_type = $2
		  AND resource_id = $3
		  AND role_name = $4
		ORDER BY id ASC
	`, grantSubjectTeam, resourceType, resourceID, customUseGrantRole)
	if err != nil {
		return false, 0, "", err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			grantID   int64
			grantTeam string
		)
		if err := rows.Scan(&grantID, &grantTeam); err != nil {
			return false, 0, "", err
		}
		grantTeam = strings.Trim(strings.TrimSpace(grantTeam), "/")
		if !teamGrantIncludesCallerTeam(grantTeam, callerTeam.Path) {
			continue
		}
		return true, grantID, grantTeam, nil
	}
	if err := rows.Err(); err != nil {
		return false, 0, "", err
	}
	return false, 0, "", nil
}

func teamGrantIncludesCallerTeam(grantTeam, callerTeam string) bool {
	grantTeam = strings.Trim(strings.TrimSpace(grantTeam), "/")
	callerTeam = strings.Trim(strings.TrimSpace(callerTeam), "/")
	if grantTeam == "" || callerTeam == "" || grantTeam == generalGrantID || callerTeam == generalGrantID {
		return false
	}
	return callerTeam == grantTeam || strings.HasPrefix(callerTeam, grantTeam+"/")
}

func (a *App) resourceVisibility(ctx context.Context, resourceType, resourceID string) (string, error) {
	if a == nil || a.db == nil {
		return resourceVisibilityTeam, nil
	}
	resourceType = strings.TrimSpace(resourceType)
	resourceID = strings.Trim(strings.TrimSpace(resourceID), "/")
	switch resourceType {
	case grantResourcePipeline:
		path, name := model.SplitPipelineID(resourceID)
		var visibility string
		err := a.db.QueryRow(ctx, `SELECT visibility FROM pipelines WHERE path = $1 AND name = $2`, path, name).Scan(&visibility)
		if err == nil {
			return normalizeResourceVisibility(visibility), nil
		}
		if !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	case grantResourceStep:
		path, name := model.SplitPipelineID(resourceID)
		var visibility string
		err := a.db.QueryRow(ctx, `SELECT visibility FROM steps WHERE path = $1 AND name = $2`, path, name).Scan(&visibility)
		if err == nil {
			return normalizeResourceVisibility(visibility), nil
		}
		if !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	case grantResourceConfig:
		var visibility string
		err := a.db.QueryRow(ctx, `
			SELECT visibility
			FROM config_repositories
			WHERE id::text = $1 OR scope_id = $1
			ORDER BY id ASC
			LIMIT 1
		`, resourceID).Scan(&visibility)
		if err == nil {
			return normalizeResourceVisibility(visibility), nil
		}
		if !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	case grantResourceTrigger:
		var visibility string
		err := a.db.QueryRow(ctx, `SELECT visibility FROM triggers WHERE repository_name = $1`, resourceID).Scan(&visibility)
		if err == nil {
			return normalizeResourceVisibility(visibility), nil
		}
		if !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}

	var visibility string
	err := a.db.QueryRow(ctx, `
		SELECT visibility
		FROM resource_visibility
		WHERE resource_type = $1 AND resource_id = $2
	`, resourceType, resourceID).Scan(&visibility)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return resourceVisibilityTeam, nil
		}
		return "", err
	}
	return normalizeResourceVisibility(visibility), nil
}

func normalizeResourceVisibility(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case resourceVisibilityTeam:
		return resourceVisibilityTeam
	case resourceVisibilityRestricted:
		return resourceVisibilityRestricted
	case resourceVisibilityWorkspace:
		return resourceVisibilityWorkspace
	default:
		return resourceVisibilityTeam
	}
}

func matchedResourceLabel(decision model.Decision, fallback model.ResourceRef) string {
	if decision.MatchedPolicy != nil {
		resourceType, _ := decision.MatchedPolicy["resource_type"].(string)
		resourceID, _ := decision.MatchedPolicy["resource_id"].(string)
		if strings.TrimSpace(resourceType) != "" && strings.TrimSpace(resourceID) != "" {
			return formatResourceLabel(resourceType, resourceID)
		}
	}
	if strings.TrimSpace(fallback.Type) == "" || strings.TrimSpace(fallback.ID) == "" {
		return ""
	}
	return formatResourceLabel(fallback.Type, fallback.ID)
}

func (a *App) matchedGrantIDFromDecision(ctx context.Context, decision model.Decision) string {
	if a == nil || a.db == nil || decision.MatchedPolicy == nil {
		return ""
	}
	subjectType, _ := decision.MatchedPolicy["subject_type"].(string)
	subjectID, _ := decision.MatchedPolicy["subject_id"].(string)
	resourceType, _ := decision.MatchedPolicy["resource_type"].(string)
	resourceID, _ := decision.MatchedPolicy["resource_id"].(string)
	action, _ := decision.MatchedPolicy["action"].(string)
	effect, _ := decision.MatchedPolicy["effect"].(string)
	if subjectType == "" || subjectID == "" || resourceType == "" || resourceID == "" || action == "" || effect == "" {
		return ""
	}
	var grantID sql.NullInt64
	err := a.db.QueryRow(ctx, `
		SELECT access_grant_id
		FROM resource_acl
		WHERE subject_type = $1
		  AND subject_id = $2
		  AND resource_type = $3
		  AND resource_id = $4
		  AND action = $5
		  AND effect = $6
		  AND access_grant_id IS NOT NULL
		ORDER BY id ASC
		LIMIT 1
	`, subjectType, subjectID, resourceType, resourceID, action, effect).Scan(&grantID)
	if err != nil || !grantID.Valid {
		return ""
	}
	return formatAccessGrantID(grantID.Int64)
}

func resourceUseCallerFromRequest(a *App, r *http.Request, gitContext map[string]string) (string, string) {
	if r != nil {
		callerType := strings.TrimSpace(r.Header.Get("X-Nopsai-Caller-Type"))
		callerID := strings.TrimSpace(r.Header.Get("X-Nopsai-Caller-ID"))
		if callerType != "" && callerID != "" {
			if normalized, err := normalizeResourceUseCallerType(callerType); err == nil {
				return normalized, normalizeResourceUseCallerID(normalized, callerID)
			}
		}
	}
	if repoID := repositoryFullName(gitContext["repo_owner"], gitContext["repo_name"]); repoID != "" {
		return model.SubjectTypeRepository, repoID
	}
	if a != nil && a.db != nil && r != nil {
		parentRunID := strings.TrimSpace(r.Header.Get("X-Nopsai-Parent-Run-ID"))
		if parentRunID != "" {
			var requestedByType, requestedByID, effectiveType, effectiveID sql.NullString
			err := a.db.QueryRow(r.Context(), `
				SELECT requested_by_type, requested_by_id, effective_subject_type, effective_subject_id
				FROM pipeline_runs
				WHERE run_id::text = $1
			`, parentRunID).Scan(&requestedByType, &requestedByID, &effectiveType, &effectiveID)
			if err == nil {
				if effectiveType.Valid && effectiveID.Valid && strings.TrimSpace(effectiveType.String) != "" && strings.TrimSpace(effectiveID.String) != "" {
					return strings.TrimSpace(effectiveType.String), strings.TrimSpace(effectiveID.String)
				}
				if requestedByType.Valid && requestedByID.Valid && strings.TrimSpace(requestedByType.String) != "" && strings.TrimSpace(requestedByID.String) != "" {
					return strings.TrimSpace(requestedByType.String), strings.TrimSpace(requestedByID.String)
				}
			}
		}
	}
	if a != nil && r != nil {
		if subject, ok := a.currentAAASubject(r); ok {
			return subject.Type, firstNonEmptyString(subject.ID, subject.Sub, subject.Email)
		}
	}
	return model.SubjectTypeInternalService, "dispatcher"
}

func (a *App) authorizeRunResourceUses(
	ctx context.Context,
	callerType,
	callerID,
	triggerSource string,
	gitContext map[string]string,
	pipelinePath string,
	pipelineSource string,
	pipeline models.Pipeline,
	scope string,
) ([]ResourceUseAuthResult, error) {
	repoID := repositoryFullName(gitContext["repo_owner"], gitContext["repo_name"])
	eventType := strings.TrimSpace(gitContext["event_type"])
	if eventType == "" {
		eventType = strings.TrimPrefix(strings.TrimSpace(triggerSource), "github_")
	}
	ref := strings.TrimSpace(gitContext["ref"])

	type checkSpec struct {
		action       string
		resourceType string
		resourceID   string
	}
	specs := make([]checkSpec, 0, 1)
	if shouldAuthorizeTopLevelPipelineUse(pipelineSource) {
		specs = append(specs, checkSpec{
			action:       "pipeline.use",
			resourceType: grantResourcePipeline,
			resourceID:   model.BuildPipelineID(pipelinePath, pipeline.Name),
		})
	}
	if strings.TrimSpace(scope) != "" {
		specs = append(specs, checkSpec{
			action:       "scope.use",
			resourceType: grantResourceScope,
			resourceID:   strings.Trim(strings.TrimSpace(scope), "/"),
		})
	}
	for _, stepID := range collectReferencedStepIdentifiers(&pipeline) {
		specs = append(specs, checkSpec{
			action:       "step.use",
			resourceType: grantResourceStep,
			resourceID:   stepID,
		})
	}
	for _, pipelineID := range collectReferencedPipelineIdentifiers(&pipeline) {
		specs = append(specs, checkSpec{
			action:       "pipeline.use",
			resourceType: grantResourcePipeline,
			resourceID:   pipelineID,
		})
	}

	results := make([]ResourceUseAuthResult, 0, len(specs))
	for _, spec := range specs {
		result, err := a.AuthorizeResourceUse(ctx, ResourceUseAuthInput{
			CallerType:   callerType,
			CallerID:     callerID,
			Action:       spec.action,
			ResourceType: spec.resourceType,
			ResourceID:   spec.resourceID,
			EventType:    eventType,
			Ref:          ref,
			Repo:         repoID,
		})
		if err != nil {
			return results, err
		}
		results = append(results, result)
		if !result.Allowed {
			return results, errors.New(resourceUseDeniedMessage(callerType, callerID, result))
		}
	}
	return results, nil
}

func shouldAuthorizeTopLevelPipelineUse(pipelineSource string) bool {
	switch strings.ToLower(strings.TrimSpace(pipelineSource)) {
	case "repository", "git":
		return false
	default:
		return true
	}
}

func collectReferencedPipelineIdentifiers(pipeline *models.Pipeline) []string {
	if pipeline == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var identifiers []string
	for _, step := range pipeline.Steps {
		includeValue := strings.TrimSpace(step.GetInclude())
		if includeValue == "" {
			continue
		}
		lower := strings.ToLower(includeValue)
		if !strings.HasPrefix(lower, "pipeline:") {
			continue
		}
		includeValue = strings.TrimSpace(includeValue[len("pipeline:"):])
		includeValue = strings.Trim(includeValue, "/")
		if includeValue == "" {
			continue
		}
		if _, ok := seen[includeValue]; ok {
			continue
		}
		seen[includeValue] = struct{}{}
		identifiers = append(identifiers, includeValue)
	}
	return identifiers
}

func resourceUseDeniedMessage(callerType, callerID string, result ResourceUseAuthResult) string {
	subject := formatResourceUseCaller(callerType, callerID)
	resource := formatResourceLabel(result.ResourceType, result.ResourceID)
	switch result.ResourceType {
	case grantResourcePipeline:
		return fmt.Sprintf("%s is not allowed to use pipeline %s. Ask the pipeline owner to share it with this repository or team.", subject, result.ResourceID)
	case grantResourceScope:
		return fmt.Sprintf("%s is not allowed to use %s. Ask the scope owner to share it with this repository or team.", subject, resource)
	case grantResourceStep:
		return fmt.Sprintf("%s is not allowed to use step %s. Ask the step owner to share it with this repository or team.", subject, result.ResourceID)
	default:
		return fmt.Sprintf("%s is not allowed to use %s.", subject, resource)
	}
}

func resourceUseFailureSummary(callerType, callerID string, result ResourceUseAuthResult, authErr error) string {
	result = normalizeResourceUseFailureResult(result, authErr)
	base := resourceUseDeniedMessage(callerType, callerID, result)
	if authErr != nil {
		base = fmt.Sprintf("Authorization unavailable for %s: %v", formatResourceLabel(result.ResourceType, result.ResourceID), authErr)
	}
	details := resourceUseDecisionDetails(callerType, callerID, result, authErr)
	if details == "" {
		return base
	}
	return base + "\n\n" + details
}

func normalizeResourceUseFailureResult(result ResourceUseAuthResult, authErr error) ResourceUseAuthResult {
	result.Allowed = false
	if authErr != nil {
		result.Reason = resourceUseReasonAuthError
	} else if strings.TrimSpace(result.Reason) == "" {
		result.Reason = resourceUseReasonDenied
	}
	return result
}

func resourceUseDecisionDetails(callerType, callerID string, result ResourceUseAuthResult, authErr error) string {
	var lines []string
	if caller := formatResourceUseCaller(callerType, callerID); caller != "" {
		lines = append(lines, "Caller: "+caller)
	}
	if repo := strings.TrimSpace(result.Repo); repo != "" {
		lines = append(lines, "Repository: "+repo)
	}
	if action := strings.TrimSpace(result.Action); action != "" {
		lines = append(lines, "Action: "+action)
	}
	if result.ResourceType != "" || result.ResourceID != "" {
		lines = append(lines, "Resource: "+formatResourceLabel(result.ResourceType, result.ResourceID))
	}
	if eventType := strings.TrimSpace(result.EventType); eventType != "" {
		lines = append(lines, "Event: "+eventType)
	}
	if ref := strings.TrimSpace(result.Ref); ref != "" {
		lines = append(lines, "Ref: "+ref)
	}
	if callerTeam := strings.Trim(strings.TrimSpace(result.CallerTeam), "/"); callerTeam != "" {
		lines = append(lines, "Caller team: "+callerTeam)
	}
	if resourceTeam := strings.Trim(strings.TrimSpace(result.ResourceTeam), "/"); resourceTeam != "" {
		lines = append(lines, "Resource team: "+resourceTeam)
	}
	if visibility := strings.TrimSpace(result.Visibility); visibility != "" {
		lines = append(lines, "Visibility: "+resourceUseVisibilityLabel(visibility))
	}
	reason := strings.TrimSpace(result.Reason)
	if authErr != nil {
		reason = resourceUseReasonAuthError
	}
	if reason == "" {
		reason = resourceUseReasonDenied
	}
	lines = append(lines, "Decision reason: "+reason)
	if explanation := resourceUseDenialExplanation(result, authErr); explanation != "" {
		lines = append(lines, "Why: "+explanation)
	}
	if matched := strings.TrimSpace(result.MatchedResource); matched != "" {
		lines = append(lines, "Matched resource: "+matched)
	}
	if grantID := strings.TrimSpace(result.MatchedGrantID); grantID != "" {
		lines = append(lines, "Matched grant: "+grantID)
	}
	if authErr != nil {
		lines = append(lines, "Error: "+authErr.Error())
	}
	return strings.Join(lines, "\n")
}

func resourceUseVisibilityLabel(visibility string) string {
	switch strings.TrimSpace(visibility) {
	case resourceVisibilityWorkspace:
		return "public"
	case resourceVisibilityRestricted:
		return "restricted"
	case resourceVisibilityTeam:
		return "team"
	default:
		return strings.TrimSpace(visibility)
	}
}

func resourceUseDenialExplanation(result ResourceUseAuthResult, authErr error) string {
	if authErr != nil {
		return "the authorization service could not complete the check"
	}
	if result.Allowed {
		return ""
	}
	visibility := strings.TrimSpace(result.Visibility)
	callerTeam := strings.Trim(strings.TrimSpace(result.CallerTeam), "/")
	resourceTeam := strings.Trim(strings.TrimSpace(result.ResourceTeam), "/")
	action := strings.TrimSpace(result.Action)
	if action == "" {
		action = "this action"
	}
	switch {
	case visibility == resourceVisibilityRestricted:
		return "restricted resources require an explicit grant, and no matching grant was found"
	case visibility == resourceVisibilityWorkspace:
		return "an explicit deny policy blocked this public resource"
	case callerTeam != "" && resourceTeam != "" && IsSameTeamBoundary(callerTeam, resourceTeam):
		return fmt.Sprintf("same-team availability still requires %s, and no matching role or grant was found", action)
	case callerTeam != "" && resourceTeam != "":
		return fmt.Sprintf("cross-team use from %s to %s requires an explicit grant or public visibility", callerTeam, resourceTeam)
	default:
		return "no direct permission, same-team permission, explicit grant, or public visibility matched this request"
	}
}

func mergeResourceUseAuthResults(base, additions []ResourceUseAuthResult) []ResourceUseAuthResult {
	seen := make(map[string]struct{}, len(base)+len(additions))
	out := make([]ResourceUseAuthResult, 0, len(base)+len(additions))
	add := func(result ResourceUseAuthResult) {
		key := strings.Join([]string{
			result.Action,
			result.ResourceType,
			result.ResourceID,
			fmt.Sprint(result.Allowed),
			result.Reason,
		}, "\x00")
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, result)
	}
	for _, result := range base {
		add(result)
	}
	for _, result := range additions {
		add(result)
	}
	return out
}

func runTriggerSourceFromRequest(r *http.Request, gitContext map[string]string) string {
	if r != nil {
		if value := strings.TrimSpace(r.Header.Get("X-Nopsai-Trigger-Source")); value != "" {
			return value
		}
		if strings.TrimSpace(r.Header.Get("X-Nopsai-Parent-Run-ID")) != "" {
			return "child_pipeline"
		}
	}
	if repositoryFullName(gitContext["repo_owner"], gitContext["repo_name"]) != "" {
		return "github"
	}
	return "manual"
}

func buildRunAuthorizationSnapshot(triggerSource, callerType, callerID string, checks []ResourceUseAuthResult) ([]byte, error) {
	snapshot := runAuthorizationSnapshot{
		TriggerSource: strings.TrimSpace(triggerSource),
		Caller:        formatResourceUseCaller(callerType, callerID),
		Checks:        checks,
	}
	return json.Marshal(snapshot)
}

func formatResourceUseCaller(callerType, callerID string) string {
	callerType = strings.TrimSpace(callerType)
	callerID = strings.TrimSpace(callerID)
	if callerType == "" || callerID == "" {
		return ""
	}
	return callerType + ":" + callerID
}

func (a *App) runResourceUseCaller(ctx context.Context, runID string, gitContext map[string]string) (callerType, callerID, triggerSource string, err error) {
	runID = strings.TrimSpace(runID)
	if runID == "" || a == nil || a.db == nil {
		return model.SubjectTypeInternalService, "dispatcher", "", nil
	}
	var requestedType, requestedID, effectiveType, effectiveID, source sql.NullString
	err = a.db.QueryRow(ctx, `
		SELECT requested_by_type, requested_by_id, effective_subject_type, effective_subject_id, trigger_source
		FROM pipeline_runs
		WHERE run_id::text = $1
	`, runID).Scan(&requestedType, &requestedID, &effectiveType, &effectiveID, &source)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return "", "", "", fmt.Errorf("run not found")
		}
		return "", "", "", err
	}
	triggerSource = strings.TrimSpace(source.String)
	if effectiveType.Valid && effectiveID.Valid && strings.TrimSpace(effectiveType.String) != "" && strings.TrimSpace(effectiveID.String) != "" {
		return strings.TrimSpace(effectiveType.String), strings.TrimSpace(effectiveID.String), triggerSource, nil
	}
	if requestedType.Valid && requestedID.Valid && strings.TrimSpace(requestedType.String) != "" && strings.TrimSpace(requestedID.String) != "" {
		return strings.TrimSpace(requestedType.String), strings.TrimSpace(requestedID.String), triggerSource, nil
	}
	if repoID := repositoryFullName(gitContext["repo_owner"], gitContext["repo_name"]); repoID != "" {
		return model.SubjectTypeRepository, repoID, triggerSource, nil
	}
	return model.SubjectTypeInternalService, "dispatcher", triggerSource, nil
}

func (a *App) authorizeRunRuntimeResourceUse(ctx context.Context, runID string, gitContext map[string]string, action, resourceType, resourceID string) (ResourceUseAuthResult, error) {
	callerType, callerID, triggerSource, err := a.runResourceUseCaller(ctx, runID, gitContext)
	if err != nil {
		return ResourceUseAuthResult{}, err
	}
	result, err := a.AuthorizeResourceUse(ctx, ResourceUseAuthInput{
		CallerType:   callerType,
		CallerID:     callerID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		EventType:    strings.TrimPrefix(strings.TrimSpace(triggerSource), "github_"),
		Ref:          strings.TrimSpace(gitContext["ref"]),
		Repo:         repositoryFullName(gitContext["repo_owner"], gitContext["repo_name"]),
	})
	if err != nil {
		return result, err
	}
	_ = a.appendRunAuthorizationChecks(ctx, runID, triggerSource, callerType, callerID, []ResourceUseAuthResult{result})
	if !result.Allowed {
		return result, errors.New(resourceUseDeniedMessage(callerType, callerID, result))
	}
	return result, nil
}

func (a *App) appendRunAuthorizationChecks(ctx context.Context, runID, triggerSource, callerType, callerID string, checks []ResourceUseAuthResult) error {
	runID = strings.TrimSpace(runID)
	if runID == "" || len(checks) == 0 || a == nil || a.db == nil {
		return nil
	}
	var raw string
	err := a.db.QueryRow(ctx, `SELECT COALESCE(authorization_snapshot::text, '{}') FROM pipeline_runs WHERE run_id::text = $1`, runID).Scan(&raw)
	if err != nil {
		return err
	}
	var snapshot runAuthorizationSnapshot
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &snapshot)
	}
	if strings.TrimSpace(snapshot.TriggerSource) == "" {
		snapshot.TriggerSource = strings.TrimSpace(triggerSource)
	}
	if strings.TrimSpace(snapshot.Caller) == "" {
		snapshot.Caller = formatResourceUseCaller(callerType, callerID)
	}
	snapshot.Checks = mergeResourceUseAuthResults(snapshot.Checks, checks)
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	_, err = a.db.Exec(ctx, `UPDATE pipeline_runs SET authorization_snapshot = $1::jsonb WHERE run_id::text = $2`, string(encoded), runID)
	return err
}

func (a *App) handleResourceUseCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var input ResourceUseAuthInput
	if err := httpapi.DecodeJSON(r, &input); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := a.applyDefaultResourceUseCaller(r, &input); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	result, err := a.AuthorizeResourceUse(r.Context(), input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, result)
}

func (a *App) handleResourceUseBatchCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req resourceUseBatchCheckRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	results := make([]ResourceUseAuthResult, 0, len(req.Checks))
	for _, check := range req.Checks {
		if check.CallerType == "" {
			check.CallerType = req.CallerType
		}
		if check.CallerID == "" {
			check.CallerID = req.CallerID
		}
		if err := a.applyDefaultResourceUseCaller(r, &check); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		result, err := a.AuthorizeResourceUse(r.Context(), check)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		results = append(results, result)
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (a *App) applyDefaultResourceUseCaller(r *http.Request, input *ResourceUseAuthInput) error {
	if input == nil {
		return fmt.Errorf("invalid request")
	}
	current, ok := a.currentAAASubject(r)
	if !ok {
		return fmt.Errorf("unauthorized")
	}
	if strings.TrimSpace(input.CallerType) == "" && strings.TrimSpace(input.CallerID) == "" {
		input.CallerType = current.Type
		input.CallerID = firstNonEmptyString(current.ID, current.Sub, current.Email)
		return nil
	}
	if resourceUseCallerMatchesSubject(*input, current) {
		return nil
	}
	decision, err := a.aaaCheck(r.Context(), current, "iam.admin", model.ResourceRef{Type: "iam", ID: "admin"}, a.aaaRequestContext(r))
	if err != nil {
		return fmt.Errorf("authorization unavailable")
	}
	if !decision.Allowed {
		return fmt.Errorf("forbidden")
	}
	return nil
}

func resourceUseCallerMatchesSubject(input ResourceUseAuthInput, subject model.Subject) bool {
	inputType, err := normalizeResourceUseCallerType(input.CallerType)
	if err != nil {
		return false
	}
	if inputType != subject.Type {
		return false
	}
	callerID := normalizeResourceUseCallerID(inputType, input.CallerID)
	for _, value := range []string{subject.ID, subject.Sub, subject.Email} {
		if strings.TrimSpace(value) != "" && strings.EqualFold(callerID, strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}

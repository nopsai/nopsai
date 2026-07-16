package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"nopsai/services/aaa/pkg/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PGStore struct {
	db *pgxpool.Pool
}

func NewPGStore(db *pgxpool.Pool) *PGStore {
	return &PGStore{db: db}
}

func (s *PGStore) ResolveSubject(ctx context.Context, subject model.Subject) (*model.ResolvedSubject, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not configured")
	}

	switch model.NormalizeType(subject.Type) {
	case model.SubjectTypeInternalService:
		subjectID := strings.TrimSpace(subject.ID)
		resolved := &model.ResolvedSubject{
			Subject: model.Subject{
				Type: model.SubjectTypeInternalService,
				ID:   subjectID,
			},
			Provider:    "internal-service",
			Status:      "active",
			DirectRoles: s.fetchBindingRoles(ctx, model.SubjectTypeInternalService, subjectID),
			AuthTeams:   s.fetchSubjectTeams(ctx, model.SubjectTypeInternalService, subjectID),
		}
		return resolved, nil
	case model.SubjectTypeRepository, model.SubjectTypeTrigger, model.SubjectTypeServiceAccount:
		subjectType := model.NormalizeType(subject.Type)
		subjectID := strings.Trim(strings.TrimSpace(subject.ID), "/")
		if subjectID == "" {
			return nil, ErrSubjectNotFound
		}
		resolved := &model.ResolvedSubject{
			Subject: model.Subject{
				Type: subjectType,
				ID:   subjectID,
			},
			Provider:    "nopsai",
			Status:      "active",
			DirectRoles: s.fetchBindingRoles(ctx, subjectType, subjectID),
			AuthTeams:   s.fetchSubjectTeams(ctx, subjectType, subjectID),
		}
		return resolved, nil
	case model.SubjectTypeAuthTeam:
		teamID := strings.TrimSpace(subject.ID)
		if teamID == "" {
			return nil, ErrSubjectNotFound
		}
		resolved := &model.ResolvedSubject{
			Subject: model.Subject{
				Type: model.SubjectTypeAuthTeam,
				ID:   teamID,
			},
			Provider:    "aaa",
			Status:      "active",
			DirectRoles: s.fetchBindingRoles(ctx, model.SubjectTypeAuthTeam, teamID),
		}
		return resolved, nil
	case model.SubjectTypeRole:
		roleName := strings.TrimSpace(subject.ID)
		if roleName == "" {
			return nil, ErrSubjectNotFound
		}
		return &model.ResolvedSubject{
			Subject: model.Subject{
				Type: model.SubjectTypeRole,
				ID:   roleName,
			},
			Provider:    "aaa",
			Status:      "active",
			DirectRoles: []string{roleName},
		}, nil
	default:
		return s.resolveUserSubject(ctx, subject)
	}
}

func (s *PGStore) resolveUserSubject(ctx context.Context, subject model.Subject) (*model.ResolvedSubject, error) {
	const baseQuery = `
		SELECT id::text, sub, COALESCE(email, ''), provider, status
		FROM users
		WHERE %s
		LIMIT 1
	`

	query, args, err := buildUserSubjectLookup(baseQuery, subject)
	if err != nil {
		return nil, err
	}

	resolved := &model.ResolvedSubject{}
	err = s.db.QueryRow(ctx, query, args...).Scan(
		&resolved.Subject.ID,
		&resolved.Subject.Sub,
		&resolved.Subject.Email,
		&resolved.Provider,
		&resolved.Status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSubjectNotFound
		}
		return nil, err
	}
	resolved.Subject.Type = model.SubjectTypeUser
	resolved.DirectRoles = s.fetchUserRoles(ctx, resolved.Subject.ID)
	resolved.AuthTeams = s.fetchSubjectTeams(ctx, model.SubjectTypeUser, resolved.Subject.ID)

	if !strings.EqualFold(resolved.Status, "active") {
		return resolved, ErrSubjectInactive
	}
	return resolved, nil
}

func buildUserSubjectLookup(baseQuery string, subject model.Subject) (string, []any, error) {
	userID := strings.TrimSpace(subject.ID)
	sub := strings.TrimSpace(subject.Sub)
	email := strings.TrimSpace(subject.Email)

	switch {
	case userID != "":
		return formatQuery(baseQuery, "id::text = $1"), []any{userID}, nil
	case sub != "":
		// Prefer the stable subject identifier over email to avoid resolving the wrong user
		// when an email is stale, duplicated, or shared across records.
		return formatQuery(baseQuery, "sub = $1"), []any{sub}, nil
	case email != "":
		return formatQuery(baseQuery, "email = $1"), []any{email}, nil
	default:
		return "", nil, ErrSubjectNotFound
	}
}

func formatQuery(base, predicate string) string {
	return strings.Replace(base, "%s", predicate, 1)
}

func (s *PGStore) fetchBindingRoles(ctx context.Context, subjectType, subjectID string) []string {
	if strings.TrimSpace(subjectID) == "" {
		return nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT role_name
		FROM auth_role_bindings
		WHERE subject_type = $1 AND subject_id = $2
		ORDER BY role_name ASC
	`, subjectType, subjectID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanRoleList(rows)
}

func (s *PGStore) fetchUserRoles(ctx context.Context, userID string) []string {
	rows, err := s.db.Query(ctx, `
		SELECT role_name
		FROM auth_role_bindings
		WHERE subject_type = 'user' AND subject_id = $1
		UNION
		SELECT role
		FROM user_roles
		WHERE user_id::text = $1
		ORDER BY 1 ASC
	`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanRoleList(rows)
}

func scanRoleList(rows pgx.Rows) []string {
	seen := make(map[string]struct{})
	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return roles
		}
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	return roles
}

func (s *PGStore) fetchSubjectTeams(ctx context.Context, subjectType, subjectID string) []model.AuthTeamInfo {
	if strings.TrimSpace(subjectType) == "" || strings.TrimSpace(subjectID) == "" {
		return nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT g.id::text, g.name, COALESCE(rb.role_name, '')
		FROM auth_team_members m
		JOIN auth_teams g ON g.id = m.team_id
		LEFT JOIN auth_role_bindings rb
			ON rb.subject_type = 'auth_team' AND rb.subject_id = g.id::text
		WHERE m.subject_type = $1 AND m.subject_id = $2
		ORDER BY g.name ASC, rb.role_name ASC
	`, subjectType, subjectID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	teamByID := make(map[string]*model.AuthTeamInfo)
	var teamOrder []string
	for rows.Next() {
		var teamID, teamName, roleName string
		if err := rows.Scan(&teamID, &teamName, &roleName); err != nil {
			return nil
		}
		team := teamByID[teamID]
		if team == nil {
			team = &model.AuthTeamInfo{ID: teamID, Name: teamName}
			teamByID[teamID] = team
			teamOrder = append(teamOrder, teamID)
		}
		roleName = strings.TrimSpace(roleName)
		if roleName != "" && !containsString(team.Roles, roleName) {
			team.Roles = append(team.Roles, roleName)
		}
	}

	teams := make([]model.AuthTeamInfo, 0, len(teamOrder))
	for _, teamID := range teamOrder {
		team := teamByID[teamID]
		teams = append(teams, *team)
	}
	return teams
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func (s *PGStore) FindRolePermissionMatch(ctx context.Context, roleNames []string, resource model.ResourceRef, action, effect string) (*model.MatchedPolicy, error) {
	if len(roleNames) == 0 {
		return nil, nil
	}

	if supportsNamedResourceSubsetMatch(resource.Type) {
		return s.findNamedResourceRolePermissionMatch(ctx, roleNames, resource, action, effect)
	}

	row := s.db.QueryRow(ctx, `
		SELECT role_name, resource_type, resource_id, action, effect
		FROM auth_role_permissions
		WHERE role_name = ANY($1)
			AND effect = $2
			AND (resource_type = $3 OR resource_type = '*')
			AND (resource_id = $4 OR resource_id = '*')
			AND (action = $5 OR action = '*')
		ORDER BY
			CASE WHEN resource_type = $3 THEN 0 ELSE 1 END,
			CASE WHEN resource_id = $4 THEN 0 ELSE 1 END,
			CASE WHEN action = $5 THEN 0 ELSE 1 END,
			role_name ASC
		LIMIT 1
	`, roleNames, effect, resource.Type, resource.ID, action)

	var policy model.MatchedPolicy
	if err := row.Scan(&policy.RoleName, &policy.ResourceType, &policy.ResourceID, &policy.Action, &policy.Effect); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	policy.Source = "role_permission"
	return &policy, nil
}

type rolePermissionMatchSpecificity struct {
	exactResourceType bool
	resourceIDScore   int
	exactAction       bool
	roleName          string
}

func supportsNamedResourceSubsetMatch(resourceType string) bool {
	switch strings.TrimSpace(resourceType) {
	case "secret", "variable":
		return true
	default:
		return false
	}
}

func namedResourceQueryValues(raw string) (url.Values, error) {
	values, err := url.ParseQuery(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	return values, nil
}

func namedResourceSubsetMatch(policyID, resourceID string) bool {
	policyID = strings.TrimSpace(policyID)
	resourceID = strings.TrimSpace(resourceID)
	if policyID == "" {
		return false
	}
	if policyID == "*" || policyID == resourceID {
		return true
	}

	policyValues, err := namedResourceQueryValues(policyID)
	if err != nil || len(policyValues) == 0 {
		return false
	}
	resourceValues, err := namedResourceQueryValues(resourceID)
	if err != nil {
		return false
	}

	for key, values := range policyValues {
		policyValue := ""
		if len(values) > 0 {
			policyValue = strings.TrimSpace(values[0])
		}
		resourceValue := strings.TrimSpace(resourceValues.Get(key))
		if policyValue != resourceValue {
			return false
		}
	}

	return true
}

func namedResourceMatchScore(policyID, resourceID string) int {
	policyID = strings.TrimSpace(policyID)
	resourceID = strings.TrimSpace(resourceID)
	switch {
	case policyID == "*" || policyID == "":
		return 0
	case policyID == resourceID:
		return 1000
	}

	values, err := namedResourceQueryValues(policyID)
	if err != nil {
		return -1
	}
	score := 0
	for range values {
		score += 10
	}
	return score
}

func rolePermissionSpecificity(policy model.MatchedPolicy, resource model.ResourceRef, action string) rolePermissionMatchSpecificity {
	score := 0
	switch {
	case policy.ResourceID == "*":
		score = 0
	case supportsNamedResourceSubsetMatch(resource.Type) && policy.ResourceType == resource.Type:
		score = namedResourceMatchScore(policy.ResourceID, resource.ID)
	default:
		score = 1000
	}

	return rolePermissionMatchSpecificity{
		exactResourceType: policy.ResourceType == resource.Type,
		resourceIDScore:   score,
		exactAction:       policy.Action == action,
		roleName:          policy.RoleName,
	}
}

func (m rolePermissionMatchSpecificity) betterThan(other rolePermissionMatchSpecificity) bool {
	switch {
	case m.exactResourceType != other.exactResourceType:
		return m.exactResourceType
	case m.resourceIDScore != other.resourceIDScore:
		return m.resourceIDScore > other.resourceIDScore
	case m.exactAction != other.exactAction:
		return m.exactAction
	default:
		return m.roleName < other.roleName
	}
}

func rolePermissionResourceMatches(policyType, policyID string, resource model.ResourceRef) bool {
	policyType = strings.TrimSpace(policyType)
	policyID = strings.TrimSpace(policyID)
	resourceType := strings.TrimSpace(resource.Type)
	resourceID := strings.TrimSpace(resource.ID)

	switch {
	case policyType == "*":
		return policyID == "*" || policyID == resourceID
	case policyType != resourceType:
		return false
	case policyID == "*" || policyID == resourceID:
		return true
	case supportsNamedResourceSubsetMatch(resourceType):
		return namedResourceSubsetMatch(policyID, resourceID)
	default:
		return false
	}
}

func (s *PGStore) findNamedResourceRolePermissionMatch(ctx context.Context, roleNames []string, resource model.ResourceRef, action, effect string) (*model.MatchedPolicy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT role_name, resource_type, resource_id, action, effect
		FROM auth_role_permissions
		WHERE role_name = ANY($1)
			AND effect = $2
			AND (resource_type = $3 OR resource_type = '*')
			AND (action = $4 OR action = '*')
	`, roleNames, effect, resource.Type, action)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var (
		bestMatch       *model.MatchedPolicy
		bestSpecificity rolePermissionMatchSpecificity
	)

	for rows.Next() {
		var policy model.MatchedPolicy
		if err := rows.Scan(&policy.RoleName, &policy.ResourceType, &policy.ResourceID, &policy.Action, &policy.Effect); err != nil {
			return nil, err
		}
		if !rolePermissionResourceMatches(policy.ResourceType, policy.ResourceID, resource) {
			continue
		}
		policy.Source = "role_permission"
		specificity := rolePermissionSpecificity(policy, resource, action)
		if bestMatch == nil || specificity.betterThan(bestSpecificity) {
			policyCopy := policy
			bestMatch = &policyCopy
			bestSpecificity = specificity
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return bestMatch, nil
}

func (s *PGStore) FindACLMatch(ctx context.Context, subject model.SubjectRef, resource model.ResourceRef, action, effect string) (*model.MatchedPolicy, error) {
	row := s.db.QueryRow(ctx, `
		SELECT
			ra.resource_type,
			COALESCE(NULLIF(ag.resource_display, ''), ra.resource_id),
			ra.subject_type,
			COALESCE(NULLIF(ag.subject_display, ''), ra.subject_id),
			ra.action,
			ra.effect,
			COALESCE(ag.role_name, '')
		FROM resource_acl ra
		LEFT JOIN access_grants ag ON ag.id = ra.access_grant_id
		WHERE ra.subject_type = $1
			AND ra.subject_id = $2
			AND ra.effect = $3
			AND (ra.resource_type = $4 OR ra.resource_type = '*')
			AND (ra.resource_id = $5 OR ra.resource_id = '*')
			AND (ra.action = $6 OR ra.action = '*')
		ORDER BY
			CASE WHEN ra.resource_type = $4 THEN 0 ELSE 1 END,
			CASE WHEN ra.resource_id = $5 THEN 0 ELSE 1 END,
			CASE WHEN ra.action = $6 THEN 0 ELSE 1 END
		LIMIT 1
	`, subject.Type, subject.ID, effect, resource.Type, resource.ID, action)

	var policy model.MatchedPolicy
	if err := row.Scan(
		&policy.ResourceType,
		&policy.ResourceID,
		&policy.SubjectType,
		&policy.SubjectID,
		&policy.Action,
		&policy.Effect,
		&policy.RoleName,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	policy.Source = "resource_acl"
	return &policy, nil
}

func (s *PGStore) ResolveResourceInheritance(ctx context.Context, resource model.ResourceRef) ([]model.InheritedResource, error) {
	switch strings.TrimSpace(resource.Type) {
	case "pipeline_run":
		var pipelinePath, pipelineName, repoOwner, repoName, scope string
		var teamID sql.NullInt64
		err := s.db.QueryRow(ctx, `
			SELECT
				COALESCE(pipeline_path, ''),
				COALESCE(pipeline_name, ''),
				COALESCE(git_repo_owner, ''),
				COALESCE(git_repo_name, ''),
				COALESCE(scope, ''),
				team_id
			FROM pipeline_runs
			WHERE run_id::text = $1
		`, resource.ID).Scan(&pipelinePath, &pipelineName, &repoOwner, &repoName, &scope, &teamID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrResourceNotFound
			}
			return nil, err
		}
		var out []model.InheritedResource
		if pipelineName != "" {
			out = append(out, model.InheritedResource{
				Resource: model.ResourceRef{Type: "pipeline", ID: model.BuildPipelineID(pipelinePath, pipelineName)},
				Reason:   "pipeline_inheritance",
			})
		}

		if teamID.Valid {
			teamAncestors, err := s.teamHierarchyAncestors(ctx, int(teamID.Int64))
			if err != nil {
				return nil, err
			}
			out = appendInheritedResources(out, teamAncestors)
		}

		if repoID := repositoryResourceID(repoOwner, repoName); repoID != "" {
			out = appendInheritedResource(out, model.InheritedResource{
				Resource: model.ResourceRef{Type: "repository", ID: repoID},
				Reason:   "repository_inheritance",
			})
			teamAncestors, err := s.repositoryTeamAncestors(ctx, repoID)
			if err != nil {
				return nil, err
			}
			out = appendInheritedResources(out, teamAncestors)
		}

		if strings.TrimSpace(scope) != "" {
			out = appendInheritedResource(out, model.InheritedResource{
				Resource: model.ResourceRef{Type: "scope", ID: strings.Trim(strings.TrimSpace(scope), "/")},
				Reason:   "scope_inheritance",
			})
			out = appendInheritedResources(out, scopeTeamAncestors(scope))
		}

		if strings.TrimSpace(pipelinePath) == "" {
			return appendInheritedResources(out, generalTeamAncestors()), nil
		}
		teamAncestors, err := s.containingTeamAncestors(ctx, pipelinePath)
		if err != nil {
			return nil, err
		}
		return appendInheritedResources(out, teamAncestors), nil
	case "pipeline":
		pipelinePath, _ := model.SplitPipelineID(resource.ID)
		if strings.TrimSpace(pipelinePath) == "" {
			return generalTeamAncestors(), nil
		}
		return s.containingTeamAncestors(ctx, pipelinePath)
	case "pipeline_schedule":
		var schedulePath string
		err := s.db.QueryRow(ctx, `
			SELECT COALESCE(path, '')
			FROM pipeline_schedules
			WHERE id::text = $1 OR CONCAT(NULLIF(path, ''), CASE WHEN path = '' THEN '' ELSE '/' END, name) = $1
			LIMIT 1
		`, strings.Trim(strings.TrimSpace(resource.ID), "/")).Scan(&schedulePath)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrResourceNotFound
			}
			return nil, err
		}
		if strings.TrimSpace(schedulePath) == "" {
			return generalTeamAncestors(), nil
		}
		return s.containingTeamAncestors(ctx, schedulePath)
	case "dashboard":
		resourceID := strings.Trim(strings.TrimSpace(resource.ID), "/")
		if resourceID == "" || resourceID == "*" {
			return nil, nil
		}
		if looksLikeStoreUUID(resourceID) {
			var teamID int
			err := s.db.QueryRow(ctx, `
				SELECT team_id
				FROM dashboards
				WHERE id::text = $1
				LIMIT 1
			`, resourceID).Scan(&teamID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil, ErrResourceNotFound
				}
				return nil, err
			}
			return s.teamHierarchyAncestors(ctx, teamID)
		}
		parts := strings.Split(resourceID, "/")
		if len(parts) < 2 {
			return nil, nil
		}
		teamPath := strings.Trim(strings.Join(parts[:len(parts)-1], "/"), "/")
		if teamPath == "" {
			return generalTeamAncestors(), nil
		}
		return s.containingTeamAncestors(ctx, teamPath)
	case "team":
		if strings.TrimSpace(resource.ID) == model.TeamGeneralID {
			return nil, nil
		}
		return s.teamAncestors(ctx, resource.ID)
	case "step":
		stepPath, _ := model.SplitPipelineID(resource.ID)
		if strings.TrimSpace(stepPath) == "" {
			return generalTeamAncestors(), nil
		}
		return s.containingTeamAncestors(ctx, stepPath)
	case "scope":
		return scopeTeamAncestors(resource.ID), nil
	case "repository":
		return s.repositoryTeamAncestors(ctx, resource.ID)
	case "trigger":
		repoID := strings.TrimSpace(resource.ID)
		if repoID == "" {
			return nil, nil
		}
		out := []model.InheritedResource{{
			Resource: model.ResourceRef{Type: "repository", ID: repoID},
			Reason:   "repository_inheritance",
		}}
		teamAncestors, err := s.repositoryTeamAncestors(ctx, repoID)
		if err != nil {
			return nil, err
		}
		return append(out, teamAncestors...), nil
	case "external_trigger":
		return s.externalTriggerTeamAncestors(ctx, resource.ID)
	case "git_webhook_source":
		return s.gitWebhookSourceTeamAncestors(ctx, resource.ID)
	case "knowledge_context":
		parts := strings.Split(strings.Trim(strings.TrimSpace(resource.ID), "/"), "/")
		if len(parts) < 3 {
			return nil, nil
		}
		teamPath := strings.Trim(strings.Join(parts[1:len(parts)-1], "/"), "/")
		if teamPath == "" {
			return generalTeamAncestors(), nil
		}
		return s.containingTeamAncestors(ctx, teamPath)
	case "secret", "variable":
		repoName, scope, _ := model.ParseNamedResourceID(resource.ID)
		var out []model.InheritedResource
		if repoName != "" {
			out = append(out, model.InheritedResource{
				Resource: model.ResourceRef{Type: "repository", ID: repoName},
				Reason:   "repository_inheritance",
			})
			teamAncestors, err := s.repositoryTeamAncestors(ctx, repoName)
			if err != nil {
				return nil, err
			}
			out = append(out, teamAncestors...)
		}
		if scope != "" {
			out = append(out, model.InheritedResource{
				Resource: model.ResourceRef{Type: "scope", ID: scope},
				Reason:   "scope_inheritance",
			})
			out = append(out, scopeTeamAncestors(scope)...)
		}
		if repoName == "" {
			if scope == "" {
				out = append(out, generalTeamAncestors()...)
			}
		}
		return out, nil
	default:
		return nil, nil
	}
}

func looksLikeStoreUUID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 36 {
		return false
	}
	for idx, ch := range value {
		switch idx {
		case 8, 13, 18, 23:
			if ch != '-' {
				return false
			}
		default:
			if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
				return false
			}
		}
	}
	return true
}

func repositoryResourceID(owner, name string) string {
	owner = strings.Trim(strings.TrimSpace(owner), "/")
	name = strings.Trim(strings.TrimSpace(name), "/")
	if name == "" {
		return ""
	}
	if owner == "" {
		return name
	}
	return owner + "/" + name
}

func appendInheritedResources(out []model.InheritedResource, resources []model.InheritedResource) []model.InheritedResource {
	for _, resource := range resources {
		out = appendInheritedResource(out, resource)
	}
	return out
}

func appendInheritedResource(out []model.InheritedResource, resource model.InheritedResource) []model.InheritedResource {
	resource.Resource.Type = strings.TrimSpace(resource.Resource.Type)
	resource.Resource.ID = strings.TrimSpace(resource.Resource.ID)
	resource.Reason = strings.TrimSpace(resource.Reason)
	if resource.Resource.Type == "" || resource.Resource.ID == "" {
		return out
	}
	for _, existing := range out {
		if existing.Resource.Type == resource.Resource.Type && existing.Resource.ID == resource.Resource.ID && existing.Reason == resource.Reason {
			return out
		}
	}
	return append(out, resource)
}

func scopeTeamAncestors(scope string) []model.InheritedResource {
	scope = strings.Trim(strings.TrimSpace(scope), "/")
	if scope == "" {
		return generalTeamAncestors()
	}
	return prefixTeamResources(strings.Split(scope, "/"), true)
}

func generalTeamAncestors() []model.InheritedResource {
	return []model.InheritedResource{{
		Resource: model.ResourceRef{Type: "team", ID: model.TeamGeneralID},
		Reason:   "team_inheritance",
	}}
}

func (s *PGStore) teamAncestors(ctx context.Context, path string) ([]model.InheritedResource, error) {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return nil, nil
	}
	segments := strings.Split(path, "/")
	if len(segments) <= 1 {
		return nil, nil
	}

	if ok, err := s.teamPathExists(ctx, segments); err != nil {
		return nil, err
	} else if !ok {
		return prefixTeamAncestors(segments), nil
	}

	return prefixTeamAncestors(segments), nil
}

func (s *PGStore) containingTeamAncestors(ctx context.Context, path string) ([]model.InheritedResource, error) {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return nil, nil
	}
	segments := strings.Split(path, "/")

	if _, err := s.teamPathExists(ctx, segments); err != nil {
		return nil, err
	}
	return prefixTeamResources(segments, true), nil
}

func (s *PGStore) teamPathExists(ctx context.Context, segments []string) (bool, error) {
	if len(segments) == 0 {
		return false, nil
	}

	var (
		currentID int
		parentID  *int
		name      string
	)
	if err := s.db.QueryRow(ctx, `
		SELECT id, parent_id, name
		FROM teams
		WHERE name = $1
	`, segments[len(segments)-1]).Scan(&currentID, &parentID, &name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if name != segments[len(segments)-1] {
		return false, nil
	}

	for idx := len(segments) - 2; idx >= 0; idx-- {
		if parentID == nil {
			return false, nil
		}
		var nextParentID *int
		if err := s.db.QueryRow(ctx, `
			SELECT parent_id, name
			FROM teams
			WHERE id = $1
		`, *parentID).Scan(&nextParentID, &name); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return false, nil
			}
			return false, err
		}
		if name != segments[idx] {
			return false, nil
		}
		parentID = nextParentID
	}
	return true, nil
}

func prefixTeamAncestors(segments []string) []model.InheritedResource {
	return prefixTeamResources(segments, false)
}

func prefixTeamResources(segments []string, includeSelf bool) []model.InheritedResource {
	start := len(segments) - 1
	if includeSelf {
		start = len(segments)
	}
	if start <= 0 {
		return nil
	}

	out := make([]model.InheritedResource, 0, start)
	for i := start; i > 0; i-- {
		out = append(out, model.InheritedResource{
			Resource: model.ResourceRef{Type: "team", ID: strings.Join(segments[:i], "/")},
			Reason:   "team_inheritance",
		})
	}
	return out
}

func (s *PGStore) repositoryTeamAncestors(ctx context.Context, repoID string) ([]model.InheritedResource, error) {
	repoID = strings.TrimSpace(repoID)
	if repoID == "" {
		return nil, nil
	}

	parentID, name, ok, err := s.repositoryTeamByMetadata(ctx, repoID)
	if err != nil {
		return nil, err
	}
	if !ok {
		parentID, name, ok, err = s.repositoryTeamByPathSuffix(ctx, repoID)
		if err != nil {
			return nil, err
		}
	}
	if !ok {
		return repositoryIDTeamAncestors(repoID), nil
	}

	parentAncestors, err := s.teamParentTeamAncestors(ctx, parentID)
	if err != nil {
		return nil, err
	}
	return teamSelfAndParentTeamAncestors(name, parentAncestors), nil
}

func (s *PGStore) repositoryTeamByMetadata(ctx context.Context, repoID string) (*int, string, bool, error) {
	var parentID *int
	var name string
	if err := s.db.QueryRow(ctx, `
		SELECT parent_id, name
		FROM teams
		WHERE name = $1 OR LOWER(repository_full_name) = LOWER($1)
		ORDER BY
			CASE
				WHEN LOWER(repository_full_name) = LOWER($1) THEN 0
				WHEN name = $1 THEN 1
				ELSE 2
			END
		LIMIT 1
	`, repoID).Scan(&parentID, &name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", false, nil
		}
		return nil, "", false, err
	}
	return parentID, name, true, nil
}

func (s *PGStore) repositoryTeamByPathSuffix(ctx context.Context, repoID string) (*int, string, bool, error) {
	var parentID *int
	var name string
	if err := s.db.QueryRow(ctx, `
		WITH RECURSIVE team_paths AS (
			SELECT
				id,
				parent_id,
				name,
				TRIM(BOTH '/' FROM name)::text AS path
			FROM teams
			WHERE parent_id IS NULL
			UNION ALL
			SELECT
				g.id,
				g.parent_id,
				g.name,
				TRIM(BOTH '/' FROM
					CASE
						WHEN gp.path = '' THEN g.name
						ELSE gp.path || '/' || g.name
					END
				)::text AS path
			FROM teams g
			JOIN team_paths gp ON g.parent_id = gp.id
		)
		SELECT parent_id, name
		FROM team_paths
		WHERE path = $1 OR RIGHT(path, LENGTH($1) + 1) = '/' || $1
		ORDER BY LENGTH(path) DESC, path ASC
		LIMIT 1
	`, repoID).Scan(&parentID, &name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", false, nil
		}
		return nil, "", false, err
	}
	return parentID, name, true, nil
}

func (s *PGStore) teamHierarchyAncestors(ctx context.Context, teamID int) ([]model.InheritedResource, error) {
	if teamID <= 0 {
		return nil, nil
	}

	var parentID *int
	var name string
	if err := s.db.QueryRow(ctx, `
		SELECT parent_id, name
		FROM teams
		WHERE id = $1
	`, teamID).Scan(&parentID, &name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	parentAncestors, err := s.teamParentTeamAncestors(ctx, parentID)
	if err != nil {
		return nil, err
	}
	return teamSelfAndParentTeamAncestors(name, parentAncestors), nil
}

func repositoryIDTeamAncestors(repoID string) []model.InheritedResource {
	repoID = strings.Trim(strings.TrimSpace(repoID), "/")
	if repoID == "" {
		return generalTeamAncestors()
	}
	segments := strings.Split(repoID, "/")
	if len(segments) <= 1 {
		return generalTeamAncestors()
	}
	return prefixTeamResources(segments[:len(segments)-1], true)
}

func (s *PGStore) externalTriggerTeamAncestors(ctx context.Context, triggerID string) ([]model.InheritedResource, error) {
	triggerID = strings.Trim(strings.TrimSpace(triggerID), "/")
	if triggerID == "" {
		return nil, nil
	}

	var teamPath string
	if err := s.db.QueryRow(ctx, `
		SELECT COALESCE(run_team_path, '')
		FROM external_triggers
		WHERE id = $1
	`, triggerID).Scan(&teamPath); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		return s.automationResourceTeamAncestors(ctx, triggerID, "")
	}
	return s.automationResourceTeamAncestors(ctx, triggerID, teamPath)
}

func (s *PGStore) gitWebhookSourceTeamAncestors(ctx context.Context, sourceID string) ([]model.InheritedResource, error) {
	sourceID = strings.Trim(strings.TrimSpace(sourceID), "/")
	if sourceID == "" {
		return nil, nil
	}

	var teamPath string
	if err := s.db.QueryRow(ctx, `
		SELECT COALESCE(team_path, '')
		FROM git_webhook_sources
		WHERE id = $1
	`, sourceID).Scan(&teamPath); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		return s.automationResourceTeamAncestors(ctx, sourceID, "")
	}
	return s.automationResourceTeamAncestors(ctx, sourceID, teamPath)
}

func (s *PGStore) automationResourceTeamAncestors(ctx context.Context, resourceID, teamPath string) ([]model.InheritedResource, error) {
	teamPath = strings.Trim(strings.TrimSpace(teamPath), "/")
	if teamPath != "" {
		return s.containingTeamAncestors(ctx, teamPath)
	}

	resourceID = strings.Trim(strings.TrimSpace(resourceID), "/")
	if resourceID == "" {
		return nil, nil
	}
	resourcePath, _ := model.SplitPipelineID(resourceID)
	if strings.TrimSpace(resourcePath) == "" {
		return generalTeamAncestors(), nil
	}
	return s.containingTeamAncestors(ctx, resourcePath)
}

func (s *PGStore) teamParentTeamAncestors(ctx context.Context, parentID *int) ([]model.InheritedResource, error) {
	if parentID == nil {
		return nil, nil
	}

	var names []string
	currentID := *parentID
	for {
		var (
			name         string
			nextParentID *int
		)
		if err := s.db.QueryRow(ctx, `
			SELECT name, parent_id
			FROM teams
			WHERE id = $1
		`, currentID).Scan(&name, &nextParentID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil
			}
			return nil, err
		}

		name = strings.Trim(strings.TrimSpace(name), "/")
		if name != "" {
			names = append(names, name)
		}
		if nextParentID == nil {
			break
		}
		currentID = *nextParentID
	}

	if len(names) == 0 {
		return nil, nil
	}

	for left, right := 0, len(names)-1; left < right; left, right = left+1, right-1 {
		names[left], names[right] = names[right], names[left]
	}

	out := make([]model.InheritedResource, 0, len(names))
	for i := len(names); i > 0; i-- {
		out = append(out, model.InheritedResource{
			Resource: model.ResourceRef{Type: "team", ID: strings.Join(names[:i], "/")},
			Reason:   "team_inheritance",
		})
	}
	return out, nil
}

func teamSelfAndParentTeamAncestors(name string, parentAncestors []model.InheritedResource) []model.InheritedResource {
	name = strings.Trim(strings.TrimSpace(name), "/")
	if name == "" {
		return parentAncestors
	}
	selfPath := name
	if len(parentAncestors) > 0 {
		parentPath := strings.Trim(strings.TrimSpace(parentAncestors[0].Resource.ID), "/")
		if parentPath != "" {
			selfPath = strings.Trim(parentPath+"/"+name, "/")
		}
	}
	out := []model.InheritedResource{{
		Resource: model.ResourceRef{Type: "team", ID: selfPath},
		Reason:   "team_inheritance",
	}}
	return appendInheritedResources(out, parentAncestors)
}

func (s *PGStore) WriteDecisionLog(ctx context.Context, entry model.DecisionLogEntry) error {
	return s.insertDecisionLog(ctx, entry)
}

func (s *PGStore) RecordAudit(ctx context.Context, entry model.DecisionLogEntry) error {
	return s.insertDecisionLog(ctx, entry)
}

func (s *PGStore) insertDecisionLog(ctx context.Context, entry model.DecisionLogEntry) error {
	var matchedPolicyJSON, contextJSON []byte
	var err error
	if entry.MatchedPolicy != nil {
		matchedPolicyJSON, err = json.Marshal(entry.MatchedPolicy)
		if err != nil {
			return err
		}
	}
	if entry.Context != nil {
		contextJSON, err = json.Marshal(entry.Context)
		if err != nil {
			return err
		}
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO authz_decision_logs (
			request_id, subject_type, subject_id, action, resource_type, resource_id,
			allowed, reason, matched_policy, sensitive, context
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`,
		entry.RequestID,
		entry.SubjectType,
		entry.SubjectID,
		entry.Action,
		entry.ResourceType,
		entry.ResourceID,
		entry.Allowed,
		entry.Reason,
		matchedPolicyJSON,
		entry.Sensitive,
		contextJSON,
	)
	return err
}

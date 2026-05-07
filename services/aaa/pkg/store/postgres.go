package store

import (
	"context"
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
			AuthGroups:  s.fetchSubjectGroups(ctx, model.SubjectTypeInternalService, subjectID),
		}
		return resolved, nil
	case model.SubjectTypeAuthGroup:
		groupID := strings.TrimSpace(subject.ID)
		if groupID == "" {
			return nil, ErrSubjectNotFound
		}
		resolved := &model.ResolvedSubject{
			Subject: model.Subject{
				Type: model.SubjectTypeAuthGroup,
				ID:   groupID,
			},
			Provider:    "aaa",
			Status:      "active",
			DirectRoles: s.fetchBindingRoles(ctx, model.SubjectTypeAuthGroup, groupID),
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
	resolved.AuthGroups = s.fetchSubjectGroups(ctx, model.SubjectTypeUser, resolved.Subject.ID)

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

func (s *PGStore) fetchSubjectGroups(ctx context.Context, subjectType, subjectID string) []model.AuthGroupInfo {
	if strings.TrimSpace(subjectType) == "" || strings.TrimSpace(subjectID) == "" {
		return nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT g.id::text, g.name, COALESCE(rb.role_name, '')
		FROM auth_group_members m
		JOIN auth_groups g ON g.id = m.group_id
		LEFT JOIN auth_role_bindings rb
			ON rb.subject_type = 'auth_group' AND rb.subject_id = g.id::text
		WHERE m.subject_type = $1 AND m.subject_id = $2
		ORDER BY g.name ASC, rb.role_name ASC
	`, subjectType, subjectID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	groupByID := make(map[string]*model.AuthGroupInfo)
	var groupOrder []string
	for rows.Next() {
		var groupID, groupName, roleName string
		if err := rows.Scan(&groupID, &groupName, &roleName); err != nil {
			return nil
		}
		group := groupByID[groupID]
		if group == nil {
			group = &model.AuthGroupInfo{ID: groupID, Name: groupName}
			groupByID[groupID] = group
			groupOrder = append(groupOrder, groupID)
		}
		roleName = strings.TrimSpace(roleName)
		if roleName != "" && !containsString(group.Roles, roleName) {
			group.Roles = append(group.Roles, roleName)
		}
	}

	groups := make([]model.AuthGroupInfo, 0, len(groupOrder))
	for _, groupID := range groupOrder {
		group := groupByID[groupID]
		groups = append(groups, *group)
	}
	return groups
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
		SELECT resource_type, resource_id, subject_type, subject_id, action, effect
		FROM resource_acl
		WHERE subject_type = $1
			AND subject_id = $2
			AND effect = $3
			AND (resource_type = $4 OR resource_type = '*')
			AND (resource_id = $5 OR resource_id = '*')
			AND (action = $6 OR action = '*')
		ORDER BY
			CASE WHEN resource_type = $4 THEN 0 ELSE 1 END,
			CASE WHEN resource_id = $5 THEN 0 ELSE 1 END,
			CASE WHEN action = $6 THEN 0 ELSE 1 END
		LIMIT 1
	`, subject.Type, subject.ID, effect, resource.Type, resource.ID, action)

	var policy model.MatchedPolicy
	if err := row.Scan(&policy.ResourceType, &policy.ResourceID, &policy.SubjectType, &policy.SubjectID, &policy.Action, &policy.Effect); err != nil {
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
		var pipelinePath, pipelineName string
		err := s.db.QueryRow(ctx, `
			SELECT COALESCE(pipeline_path, ''), COALESCE(pipeline_name, '')
			FROM pipeline_runs
			WHERE run_id::text = $1
		`, resource.ID).Scan(&pipelinePath, &pipelineName)
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
		folderAncestors, err := s.folderAncestors(ctx, pipelinePath)
		if err != nil {
			return nil, err
		}
		return append(out, folderAncestors...), nil
	case "pipeline":
		pipelinePath, _ := model.SplitPipelineID(resource.ID)
		return s.folderAncestors(ctx, pipelinePath)
	case "folder":
		return s.folderAncestors(ctx, resource.ID)
	case "trigger":
		repoID := strings.TrimSpace(resource.ID)
		if repoID == "" {
			return nil, nil
		}
		return []model.InheritedResource{{
			Resource: model.ResourceRef{Type: "repository", ID: repoID},
			Reason:   "repository_inheritance",
		}}, nil
	case "secret", "variable":
		repoName, scope, _ := model.ParseNamedResourceID(resource.ID)
		var out []model.InheritedResource
		if repoName != "" {
			out = append(out, model.InheritedResource{
				Resource: model.ResourceRef{Type: "repository", ID: repoName},
				Reason:   "repository_inheritance",
			})
		}
		if scope != "" {
			out = append(out, model.InheritedResource{
				Resource: model.ResourceRef{Type: "scope", ID: scope},
				Reason:   "scope_inheritance",
			})
		}
		return out, nil
	default:
		return nil, nil
	}
}

func (s *PGStore) folderAncestors(ctx context.Context, path string) ([]model.InheritedResource, error) {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return nil, nil
	}
	segments := strings.Split(path, "/")
	if len(segments) <= 1 {
		return nil, nil
	}

	if ok, err := s.folderPathExists(ctx, segments); err != nil {
		return nil, err
	} else if !ok {
		return prefixFolderAncestors(segments), nil
	}

	return prefixFolderAncestors(segments), nil
}

func (s *PGStore) folderPathExists(ctx context.Context, segments []string) (bool, error) {
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
		FROM groups
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
			FROM groups
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

func prefixFolderAncestors(segments []string) []model.InheritedResource {
	out := make([]model.InheritedResource, 0, len(segments)-1)
	for i := len(segments) - 1; i > 0; i-- {
		out = append(out, model.InheritedResource{
			Resource: model.ResourceRef{Type: "folder", ID: strings.Join(segments[:i], "/")},
			Reason:   "folder_inheritance",
		})
	}
	return out
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

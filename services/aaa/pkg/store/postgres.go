package store

import (
	"context"
	"encoding/json"
	"errors"
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

	var (
		query string
		args  []any
	)

	userID := strings.TrimSpace(subject.ID)
	sub := strings.TrimSpace(subject.Sub)
	email := strings.TrimSpace(subject.Email)

	switch {
	case userID != "":
		query = baseQuery
		args = []any{userID}
		query = formatQuery(query, "id::text = $1")
	case sub != "" && email != "":
		query = formatQuery(baseQuery, "(sub = $1 OR email = $2)")
		args = []any{sub, email}
	case sub != "":
		query = formatQuery(baseQuery, "sub = $1")
		args = []any{sub}
	case email != "":
		query = formatQuery(baseQuery, "email = $1")
		args = []any{email}
	default:
		return nil, ErrSubjectNotFound
	}

	resolved := &model.ResolvedSubject{}
	err := s.db.QueryRow(ctx, query, args...).Scan(
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

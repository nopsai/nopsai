package authz

import (
	"context"
	"errors"
	"strings"

	"nopsai/services/aaa/pkg/model"
	"nopsai/services/aaa/pkg/store"
)

const productAdminRoleName = "admin"

type Backend interface {
	ResolveSubject(ctx context.Context, subject model.Subject) (*model.ResolvedSubject, error)
	FindRolePermissionMatch(ctx context.Context, roleNames []string, resource model.ResourceRef, action, effect string) (*model.MatchedPolicy, error)
	FindACLMatch(ctx context.Context, subject model.SubjectRef, resource model.ResourceRef, action, effect string) (*model.MatchedPolicy, error)
	ResolveResourceInheritance(ctx context.Context, resource model.ResourceRef) ([]model.InheritedResource, error)
	WriteDecisionLog(ctx context.Context, entry model.DecisionLogEntry) error
	RecordAudit(ctx context.Context, entry model.DecisionLogEntry) error
}

type Evaluator struct {
	backend Backend
}

func NewEvaluator(backend Backend) *Evaluator {
	return &Evaluator{backend: backend}
}

func (e *Evaluator) Introspect(ctx context.Context, subject model.Subject) (*model.IntrospectResponse, error) {
	resolved, err := e.backend.ResolveSubject(ctx, subject)
	if err != nil {
		return nil, err
	}
	return &model.IntrospectResponse{
		ID:        resolved.Subject.ID,
		Sub:       resolved.Subject.Sub,
		Email:     resolved.Subject.Email,
		Provider:  resolved.Provider,
		Status:    resolved.Status,
		Roles:     resolved.EffectiveRoles(),
		AuthTeams: resolved.AuthTeams,
	}, nil
}

func (e *Evaluator) Check(ctx context.Context, subject model.Subject, action string, resource model.ResourceRef, requestContext map[string]any) (model.Decision, error) {
	action = model.NormalizeAction(action)
	resource.Type = strings.TrimSpace(resource.Type)
	resource.ID = strings.TrimSpace(resource.ID)

	resolved, err := e.backend.ResolveSubject(ctx, subject)
	if err != nil {
		return e.handleSubjectResolutionError(ctx, err, subject, action, resource, requestContext, resolved)
	}

	if decision, ok, err := e.findAdminAllow(ctx, resolved, action, resource); err != nil {
		return model.Decision{}, err
	} else if ok {
		return decision, e.logDecision(ctx, resolved, decision, action, resource, requestContext)
	}

	inherited, err := e.backend.ResolveResourceInheritance(ctx, resource)
	if err != nil {
		if errors.Is(err, store.ErrResourceNotFound) {
			decision := model.Decision{Allowed: false, Reason: "resource_not_found"}
			return decision, e.logDecision(ctx, resolved, decision, action, resource, requestContext)
		}
		return model.Decision{}, err
	}

	if decision, ok, err := e.findDeny(ctx, resolved, action, resource, inherited); err != nil {
		return model.Decision{}, err
	} else if ok {
		return decision, e.logDecision(ctx, resolved, decision, action, resource, requestContext)
	}

	if decision, ok, err := e.findAllow(ctx, resolved, action, resource, inherited); err != nil {
		return model.Decision{}, err
	} else if ok {
		return decision, e.logDecision(ctx, resolved, decision, action, resource, requestContext)
	}

	decision := model.Decision{Allowed: false, Reason: "default_deny"}
	return decision, e.logDecision(ctx, resolved, decision, action, resource, requestContext)
}

func (e *Evaluator) BatchCheck(ctx context.Context, subject model.Subject, checks []model.BatchCheckItem, requestContext map[string]any) ([]model.Decision, error) {
	decisions := make([]model.Decision, 0, len(checks))
	for _, check := range checks {
		decision, err := e.Check(ctx, subject, check.Action, check.Resource, requestContext)
		if err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

func (e *Evaluator) Filter(ctx context.Context, subject model.Subject, action string, resources []model.ResourceRef, requestContext map[string]any) ([]model.ResourceRef, error) {
	allowed := make([]model.ResourceRef, 0, len(resources))
	for _, resource := range resources {
		decision, err := e.Check(ctx, subject, action, resource, requestContext)
		if err != nil {
			return nil, err
		}
		if decision.Allowed {
			allowed = append(allowed, resource)
		}
	}
	return allowed, nil
}

func (e *Evaluator) RecordAudit(ctx context.Context, req model.AuditRecordRequest) error {
	return e.backend.RecordAudit(ctx, model.DecisionLogEntry{
		RequestID:     strings.TrimSpace(req.RequestID),
		SubjectType:   strings.TrimSpace(req.Subject.Type),
		SubjectID:     firstNonEmpty(req.Subject.ID, req.Subject.Sub, req.Subject.Email),
		Action:        strings.TrimSpace(req.Action),
		ResourceType:  strings.TrimSpace(req.Resource.Type),
		ResourceID:    strings.TrimSpace(req.Resource.ID),
		Allowed:       req.Allowed,
		Reason:        strings.TrimSpace(req.Reason),
		MatchedPolicy: req.MatchedPolicy,
		Sensitive:     req.Sensitive,
		Context:       req.Context,
	})
}

func (e *Evaluator) handleSubjectResolutionError(
	ctx context.Context,
	err error,
	subject model.Subject,
	action string,
	resource model.ResourceRef,
	requestContext map[string]any,
	resolved *model.ResolvedSubject,
) (model.Decision, error) {
	switch {
	case errors.Is(err, store.ErrSubjectInactive):
		decision := model.Decision{Allowed: false, Reason: "subject_inactive"}
		return decision, e.logDecision(ctx, resolved, decision, action, resource, requestContext)
	case errors.Is(err, store.ErrSubjectNotFound):
		decision := model.Decision{Allowed: false, Reason: "subject_not_found"}
		fallback := resolved
		if fallback == nil {
			fallback = &model.ResolvedSubject{
				Subject: model.Subject{
					Type:  model.NormalizeType(subject.Type),
					ID:    firstNonEmpty(subject.ID, subject.Sub, subject.Email),
					Sub:   strings.TrimSpace(subject.Sub),
					Email: strings.TrimSpace(subject.Email),
				},
				Status: "missing",
			}
		}
		return decision, e.logDecision(ctx, fallback, decision, action, resource, requestContext)
	default:
		return model.Decision{}, err
	}
}

func (e *Evaluator) findDeny(ctx context.Context, resolved *model.ResolvedSubject, action string, resource model.ResourceRef, inherited []model.InheritedResource) (model.Decision, bool, error) {
	if resolved == nil {
		return model.Decision{}, false, nil
	}
	if match, err := e.backend.FindRolePermissionMatch(ctx, resolved.DirectRoles, resource, action, "deny"); err != nil {
		return model.Decision{}, false, err
	} else if match != nil {
		return model.Decision{Allowed: false, Reason: "direct_role_deny", MatchedPolicy: match.AsMap()}, true, nil
	}
	for _, team := range resolved.AuthTeams {
		if match, err := e.backend.FindRolePermissionMatch(ctx, team.Roles, resource, action, "deny"); err != nil {
			return model.Decision{}, false, err
		} else if match != nil {
			return model.Decision{Allowed: false, Reason: "auth_team_role_deny", MatchedPolicy: match.AsMap()}, true, nil
		}
	}

	directSubject := model.SubjectRef{Type: resolved.Subject.Type, ID: resolved.Subject.ID}
	if match, err := e.backend.FindACLMatch(ctx, directSubject, resource, action, "deny"); err != nil {
		return model.Decision{}, false, err
	} else if match != nil {
		return model.Decision{Allowed: false, Reason: "direct_acl_deny", MatchedPolicy: match.AsMap()}, true, nil
	}
	for _, team := range resolved.AuthTeams {
		if match, err := e.backend.FindACLMatch(ctx, model.SubjectRef{Type: model.SubjectTypeAuthTeam, ID: team.ID}, resource, action, "deny"); err != nil {
			return model.Decision{}, false, err
		} else if match != nil {
			return model.Decision{Allowed: false, Reason: "auth_team_acl_deny", MatchedPolicy: match.AsMap()}, true, nil
		}
	}

	for _, inheritedResource := range inherited {
		if match, err := e.backend.FindACLMatch(ctx, directSubject, inheritedResource.Resource, action, "deny"); err != nil {
			return model.Decision{}, false, err
		} else if match != nil {
			return model.Decision{Allowed: false, Reason: inheritedReason(inheritedResource.Reason, false), MatchedPolicy: match.AsMap()}, true, nil
		}
		for _, team := range resolved.AuthTeams {
			if match, err := e.backend.FindACLMatch(ctx, model.SubjectRef{Type: model.SubjectTypeAuthTeam, ID: team.ID}, inheritedResource.Resource, action, "deny"); err != nil {
				return model.Decision{}, false, err
			} else if match != nil {
				return model.Decision{Allowed: false, Reason: inheritedReason(inheritedResource.Reason, false), MatchedPolicy: match.AsMap()}, true, nil
			}
		}
	}

	return model.Decision{}, false, nil
}

func (e *Evaluator) findAllow(ctx context.Context, resolved *model.ResolvedSubject, action string, resource model.ResourceRef, inherited []model.InheritedResource) (model.Decision, bool, error) {
	if resolved == nil {
		return model.Decision{}, false, nil
	}

	if match, err := e.backend.FindRolePermissionMatch(ctx, resolved.DirectRoles, resource, action, "allow"); err != nil {
		return model.Decision{}, false, err
	} else if match != nil {
		return model.Decision{Allowed: true, Reason: "direct_role_allow", MatchedPolicy: match.AsMap()}, true, nil
	}
	for _, team := range resolved.AuthTeams {
		if match, err := e.backend.FindRolePermissionMatch(ctx, team.Roles, resource, action, "allow"); err != nil {
			return model.Decision{}, false, err
		} else if match != nil {
			return model.Decision{Allowed: true, Reason: "auth_team_role_allow", MatchedPolicy: match.AsMap()}, true, nil
		}
	}

	directSubject := model.SubjectRef{Type: resolved.Subject.Type, ID: resolved.Subject.ID}
	if match, err := e.backend.FindACLMatch(ctx, directSubject, resource, action, "allow"); err != nil {
		return model.Decision{}, false, err
	} else if match != nil {
		return model.Decision{Allowed: true, Reason: "direct_acl_allow", MatchedPolicy: match.AsMap()}, true, nil
	}
	for _, team := range resolved.AuthTeams {
		if match, err := e.backend.FindACLMatch(ctx, model.SubjectRef{Type: model.SubjectTypeAuthTeam, ID: team.ID}, resource, action, "allow"); err != nil {
			return model.Decision{}, false, err
		} else if match != nil {
			return model.Decision{Allowed: true, Reason: "auth_team_acl_allow", MatchedPolicy: match.AsMap()}, true, nil
		}
	}

	for _, inheritedResource := range inherited {
		if match, err := e.backend.FindACLMatch(ctx, directSubject, inheritedResource.Resource, action, "allow"); err != nil {
			return model.Decision{}, false, err
		} else if match != nil {
			return model.Decision{Allowed: true, Reason: inheritedReason(inheritedResource.Reason, true), MatchedPolicy: match.AsMap()}, true, nil
		}
		for _, team := range resolved.AuthTeams {
			if match, err := e.backend.FindACLMatch(ctx, model.SubjectRef{Type: model.SubjectTypeAuthTeam, ID: team.ID}, inheritedResource.Resource, action, "allow"); err != nil {
				return model.Decision{}, false, err
			} else if match != nil {
				return model.Decision{Allowed: true, Reason: inheritedReason(inheritedResource.Reason, true), MatchedPolicy: match.AsMap()}, true, nil
			}
		}
	}

	return model.Decision{}, false, nil
}

func (e *Evaluator) findAdminAllow(ctx context.Context, resolved *model.ResolvedSubject, action string, resource model.ResourceRef) (model.Decision, bool, error) {
	adminRoles := effectiveAdminRoles(resolved)
	if len(adminRoles) == 0 {
		return model.Decision{}, false, nil
	}
	if match, err := e.backend.FindRolePermissionMatch(ctx, adminRoles, resource, action, "allow"); err != nil {
		return model.Decision{}, false, err
	} else if match != nil {
		return model.Decision{Allowed: true, Reason: "admin_role_allow", MatchedPolicy: match.AsMap()}, true, nil
	}
	return model.Decision{Allowed: true, Reason: "admin_role_allow"}, true, nil
}

func effectiveAdminRoles(resolved *model.ResolvedSubject) []string {
	if resolved == nil {
		return nil
	}
	roles := resolved.EffectiveRoles()
	adminRoles := make([]string, 0, 2)
	for _, role := range roles {
		switch {
		case strings.EqualFold(strings.TrimSpace(role), model.RoleNameAdmin):
			adminRoles = appendUniqueRole(adminRoles, model.RoleNameAdmin)
		case strings.EqualFold(strings.TrimSpace(role), productAdminRoleName):
			adminRoles = appendUniqueRole(adminRoles, productAdminRoleName)
		}
	}
	return adminRoles
}

func appendUniqueRole(roles []string, role string) []string {
	for _, existing := range roles {
		if existing == role {
			return roles
		}
	}
	return append(roles, role)
}

func inheritedReason(reason string, allowed bool) string {
	switch strings.TrimSpace(reason) {
	case "pipeline_inheritance":
		if allowed {
			return "pipeline_acl_inheritance_allow"
		}
		return "pipeline_acl_inheritance_deny"
	case "repository_inheritance":
		if allowed {
			return "repository_acl_inheritance_allow"
		}
		return "repository_acl_inheritance_deny"
	case "scope_inheritance":
		if allowed {
			return "scope_acl_inheritance_allow"
		}
		return "scope_acl_inheritance_deny"
	default:
		if allowed {
			return "inherited_team_acl_allow"
		}
		return "inherited_team_acl_deny"
	}
}

func (e *Evaluator) logDecision(ctx context.Context, resolved *model.ResolvedSubject, decision model.Decision, action string, resource model.ResourceRef, requestContext map[string]any) error {
	if e == nil || e.backend == nil {
		return nil
	}
	shouldLog := !decision.Allowed || (decision.Allowed && model.IsSensitiveAction(action))
	if !shouldLog {
		return nil
	}

	subjectType := ""
	subjectID := ""
	if resolved != nil {
		subjectType = resolved.Subject.Type
		subjectID = firstNonEmpty(resolved.Subject.ID, resolved.Subject.Sub, resolved.Subject.Email)
	}

	return e.backend.WriteDecisionLog(ctx, model.DecisionLogEntry{
		RequestID:     requestIDFromContext(requestContext),
		SubjectType:   subjectType,
		SubjectID:     subjectID,
		Action:        action,
		ResourceType:  resource.Type,
		ResourceID:    resource.ID,
		Allowed:       decision.Allowed,
		Reason:        decision.Reason,
		MatchedPolicy: decision.MatchedPolicy,
		Sensitive:     decision.Allowed && model.IsSensitiveAction(action),
		Context:       requestContext,
	})
}

func requestIDFromContext(values map[string]any) string {
	if values == nil {
		return ""
	}
	if raw, ok := values["request_id"].(string); ok {
		return strings.TrimSpace(raw)
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

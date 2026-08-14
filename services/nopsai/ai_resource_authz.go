package nopsai

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"nopsai/services/aaa/pkg/model"
)

type aiResourceAccessSpec struct {
	resourceType string
	systemID     string
	readAction   string
	useAction    string
	manageAction string
}

var (
	llmProfileAccessSpec = aiResourceAccessSpec{
		resourceType: grantResourceLLMProfile,
		systemID:     "models",
		readAction:   "model.read",
		useAction:    "model.use",
		manageAction: "model.manage_acl",
	}
	agentProfileAccessSpec = aiResourceAccessSpec{
		resourceType: grantResourceAgentProfile,
		systemID:     "agent-roles",
		readAction:   "agent_role.read",
		useAction:    "agent_role.use",
		manageAction: "agent_role.manage_acl",
	}
	mcpServerAccessSpec = aiResourceAccessSpec{
		resourceType: grantResourceMCPServer,
		systemID:     "mcp",
		readAction:   "mcp_server.read",
		useAction:    "mcp_server.use",
		manageAction: "mcp_server.manage_acl",
	}
	mcpProfileAccessSpec = aiResourceAccessSpec{
		resourceType: grantResourceMCPProfile,
		systemID:     "mcp",
		readAction:   "mcp_profile.read",
		useAction:    "mcp_profile.use",
		manageAction: "mcp_profile.manage_acl",
	}
)

func aiResourceTeamPath(resourceID string) string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(resourceID), "/"), "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			clean = append(clean, part)
		}
	}
	if len(clean) <= 1 {
		return ""
	}
	return strings.Join(clean[:len(clean)-1], "/")
}

func aiResourceLocalName(resourceID string) string {
	if strings.HasSuffix(strings.TrimSpace(resourceID), "/") {
		return ""
	}
	parts := strings.Split(strings.Trim(strings.TrimSpace(resourceID), "/"), "/")
	for index := len(parts) - 1; index >= 0; index-- {
		part := strings.TrimSpace(parts[index])
		if part != "" {
			return part
		}
	}
	return ""
}

func aiResourceVisibleDefault(defaultID string, visibleIDs []string) string {
	defaultID = strings.TrimSpace(defaultID)
	for _, id := range visibleIDs {
		if strings.EqualFold(id, defaultID) {
			return id
		}
	}
	return ""
}

func (a *App) aiResourceTopicReadAllowed(ctx context.Context, subject model.Subject, spec aiResourceAccessSpec) bool {
	systemResource := model.ResourceRef{Type: "system", ID: spec.systemID}
	return a.checkCapability(subject, "system.read", systemResource) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, spec.readAction, model.ResourceRef{Type: spec.resourceType, ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, spec.useAction, model.ResourceRef{Type: spec.resourceType, ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, spec.manageAction, model.ResourceRef{Type: spec.resourceType, ID: "*"})
}

func (a *App) aiResourceTopicWriteAllowed(ctx context.Context, subject model.Subject, spec aiResourceAccessSpec) bool {
	return a.checkCapability(subject, "system.update", model.ResourceRef{Type: "system", ID: spec.systemID}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, spec.manageAction, model.ResourceRef{Type: spec.resourceType, ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "team.update", model.ResourceRef{Type: grantResourceTeam, ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "team.manage_acl", model.ResourceRef{Type: grantResourceTeam, ID: "*"})
}

func (a *App) aiResourceVisible(r *http.Request, spec aiResourceAccessSpec, resourceID string) (bool, error) {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		return false, fmt.Errorf("missing authorization subject")
	}
	if !a.aaaAvailable() {
		return false, fmt.Errorf("authorization unavailable")
	}
	if allowed, err := a.aiResourceSystemAllowed(r, subject, "system.read", spec.systemID); err != nil || allowed {
		return allowed, err
	}
	return a.aiResourceResourceAllowed(r, subject, spec, resourceID, []string{spec.readAction, spec.useAction, spec.manageAction})
}

func (a *App) aiResourceUsable(r *http.Request, spec aiResourceAccessSpec, resourceID string) (bool, error) {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		return false, fmt.Errorf("missing authorization subject")
	}
	if !a.aaaAvailable() {
		return false, fmt.Errorf("authorization unavailable")
	}
	if allowed, err := a.aiResourceSystemAllowed(r, subject, "system.update", spec.systemID); err != nil || allowed {
		return allowed, err
	}
	return a.aiResourceResourceAllowed(r, subject, spec, resourceID, []string{spec.useAction, spec.manageAction})
}

func (a *App) aiResourceWriteAllowed(r *http.Request, spec aiResourceAccessSpec, resourceID string) (bool, error) {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		return false, fmt.Errorf("missing authorization subject")
	}
	if !a.aaaAvailable() {
		return false, fmt.Errorf("authorization unavailable")
	}
	if allowed, err := a.aiResourceSystemAllowed(r, subject, "system.update", spec.systemID); err != nil || allowed {
		return allowed, err
	}
	if allowed, err := a.aiResourceDirectAllowed(r, subject, spec, resourceID, []string{spec.manageAction}); err != nil || allowed {
		return allowed, err
	}
	teamPath := aiResourceTeamPath(resourceID)
	if teamPath == "" {
		return false, nil
	}
	return a.aiResourceActionsAllowed(r, subject, model.ResourceRef{Type: grantResourceTeam, ID: teamPath}, []string{
		spec.manageAction,
		"team.update",
		"team.manage_acl",
	})
}

func (a *App) aiResourceSystemAllowed(r *http.Request, subject model.Subject, action string, systemID string) (bool, error) {
	return a.aiResourceActionsAllowed(r, subject, model.ResourceRef{Type: "system", ID: systemID}, []string{action})
}

func (a *App) aiResourceResourceAllowed(r *http.Request, subject model.Subject, spec aiResourceAccessSpec, resourceID string, actions []string) (bool, error) {
	if allowed, err := a.aiResourceDirectAllowed(r, subject, spec, resourceID, actions); err != nil || allowed {
		return allowed, err
	}
	teamPath := aiResourceTeamPath(resourceID)
	if teamPath == "" {
		return false, nil
	}
	return a.aiResourceActionsAllowed(r, subject, model.ResourceRef{Type: grantResourceTeam, ID: teamPath}, actions)
}

func (a *App) aiResourceDirectAllowed(r *http.Request, subject model.Subject, spec aiResourceAccessSpec, resourceID string, actions []string) (bool, error) {
	return a.aiResourceActionsAllowed(r, subject, model.ResourceRef{Type: spec.resourceType, ID: strings.TrimSpace(resourceID)}, actions)
}

func (a *App) aiResourceActionsAllowed(r *http.Request, subject model.Subject, resource model.ResourceRef, actions []string) (bool, error) {
	for _, action := range actions {
		action = strings.TrimSpace(action)
		if action == "" {
			continue
		}
		decision, err := a.aaaCheck(r.Context(), subject, action, resource, a.aaaRequestContext(r))
		if err != nil {
			return false, err
		}
		if decision.Allowed {
			return true, nil
		}
	}
	return false, nil
}

func (a *App) requireAIResourceVisible(w http.ResponseWriter, r *http.Request, spec aiResourceAccessSpec, resourceID string) bool {
	allowed, err := a.aiResourceVisible(r, spec, resourceID)
	return a.writeAIResourceAuthzResult(w, allowed, err)
}

func (a *App) requireAIResourceUse(w http.ResponseWriter, r *http.Request, spec aiResourceAccessSpec, resourceID string) bool {
	allowed, err := a.aiResourceUsable(r, spec, resourceID)
	return a.writeAIResourceAuthzResult(w, allowed, err)
}

func (a *App) requireAIResourceWrite(w http.ResponseWriter, r *http.Request, spec aiResourceAccessSpec, resourceID string) bool {
	allowed, err := a.aiResourceWriteAllowed(r, spec, resourceID)
	return a.writeAIResourceAuthzResult(w, allowed, err)
}

func (a *App) writeAIResourceAuthzResult(w http.ResponseWriter, allowed bool, err error) bool {
	if err != nil {
		if strings.Contains(err.Error(), "missing authorization subject") {
			http.Error(w, "missing authorization subject", http.StatusUnauthorized)
			return false
		}
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return false
	}
	if !allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

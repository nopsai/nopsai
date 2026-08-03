package nopsai

import (
	"fmt"
	"net/http"
	"strings"

	aaamodel "nopsai/services/aaa/pkg/model"
)

func teamGrantResourceForPath(rawPath string) (aaamodel.ResourceRef, bool, error) {
	if strings.TrimSpace(rawPath) == "" {
		return aaamodel.ResourceRef{}, false, nil
	}
	teamPath, err := normalizeRunTeamPath(rawPath)
	if err != nil {
		return aaamodel.ResourceRef{}, false, err
	}
	return aaamodel.ResourceRef{Type: grantResourceTeam, ID: teamPath}, true, nil
}

func teamOwnedAuthorizationResource(teamPath string, fallback aaamodel.ResourceRef) (aaamodel.ResourceRef, error) {
	resource, ok, err := teamGrantResourceForPath(teamPath)
	if err != nil {
		return aaamodel.ResourceRef{}, err
	}
	if !ok {
		return fallback, nil
	}
	return resource, nil
}

func (a *App) requireTeamOwnedCreateDecision(w http.ResponseWriter, r *http.Request, action string, fallback aaamodel.ResourceRef, teamPath string) bool {
	resource, err := teamOwnedAuthorizationResource(teamPath, fallback)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid team path: %v", err), http.StatusBadRequest)
		return false
	}
	return a.requireAAADecision(w, r, action, resource)
}

func teamOwnedEffectivePermissionResource(action string, resource accessGrantResource, teamPath string) (aaamodel.ResourceRef, bool, error) {
	if !supportsTeamOwnedEffectivePermission(action, resource.Type) {
		return aaamodel.ResourceRef{}, false, nil
	}
	return teamGrantResourceForPath(teamPath)
}

func supportsTeamOwnedEffectivePermission(action, resourceType string) bool {
	switch resourceType {
	case grantResourcePipeline:
		return action == "pipeline.create"
	case grantResourceSchedule:
		return action == "pipeline_schedule.create"
	case grantResourceTrigger:
		return action == "trigger.update"
	case grantResourceExternalTrigger:
		return action == "external_trigger.create"
	case grantResourceGitWebhookSource:
		return action == "git_webhook_source.create"
	case grantResourceScope:
		return action == "scope.update"
	case grantResourceStep:
		return action == "step.create"
	default:
		return false
	}
}

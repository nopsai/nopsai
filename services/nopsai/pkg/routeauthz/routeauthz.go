package routeauthz

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"nopsai/services/aaa/pkg/model"
)

func MapRequest(r *http.Request) (action string, resource model.ResourceRef, requiresFilter bool, err error) {
	if r == nil {
		return "", model.ResourceRef{}, false, nil
	}

	path := strings.TrimSpace(r.URL.Path)
	switch {
	case path == "/v1/auth/me", path == "/v1/auth/password", path == "/v1/auth/email",
		path == "/v1/auth/personal-tokens", strings.HasPrefix(path, "/v1/auth/personal-tokens/"):
		return "", model.ResourceRef{}, false, nil
	case strings.HasPrefix(path, "/v1/admin/"):
		return "iam.admin", model.ResourceRef{Type: "iam", ID: "admin"}, false, nil
	case strings.HasPrefix(path, "/v1/audit"):
		return "audit.read", model.ResourceRef{Type: "audit", ID: "authz"}, false, nil
	case path == "/v1/system/config":
		if r.Method == http.MethodGet {
			return "system.read", model.ResourceRef{Type: "system", ID: "config"}, false, nil
		}
		return "system.update", model.ResourceRef{Type: "system", ID: "config"}, false, nil
	case path == "/v1/system/notifications/mail" || path == "/v1/system/notifications/mail/test":
		if r.Method == http.MethodGet {
			return "system.read", model.ResourceRef{Type: "system", ID: "notifications"}, false, nil
		}
		return "system.update", model.ResourceRef{Type: "system", ID: "notifications"}, false, nil
	case path == "/v1/system/llm-profiles" || strings.HasPrefix(path, "/v1/system/llm-profiles/"):
		if r.Method == http.MethodGet {
			return "system.read", model.ResourceRef{Type: "system", ID: "llm-profiles"}, false, nil
		}
		return "system.update", model.ResourceRef{Type: "system", ID: "llm-profiles"}, false, nil
	case path == "/v1/system/mcp" || strings.HasPrefix(path, "/v1/system/mcp/"):
		if r.Method == http.MethodGet {
			return "system.read", model.ResourceRef{Type: "system", ID: "mcp"}, false, nil
		}
		return "system.update", model.ResourceRef{Type: "system", ID: "mcp"}, false, nil
	case path == "/v1/system/config/sync":
		if r.Method == http.MethodGet {
			return "system.read", model.ResourceRef{Type: "system", ID: "config-sync"}, false, nil
		}
		return "system.update", model.ResourceRef{Type: "system", ID: "config-sync"}, false, nil
	case path == "/v1/system/config-repo":
		if r.Method == http.MethodGet {
			return "system.read", model.ResourceRef{Type: "system", ID: "config-repos"}, false, nil
		}
		return "system.update", model.ResourceRef{Type: "system", ID: "config-repos"}, false, nil
	case path == "/v1/system/config-repo/sync":
		if r.Method == http.MethodGet {
			return "system.read", model.ResourceRef{Type: "system", ID: "config-repos"}, false, nil
		}
		return "system.update", model.ResourceRef{Type: "system", ID: "config-repos"}, false, nil
	case path == "/v1/system/config-repo/write":
		return "system.update", model.ResourceRef{Type: "system", ID: "config-repos"}, false, nil
	case path == "/v1/system/config-repo/drift":
		return "system.read", model.ResourceRef{Type: "system", ID: "config-repos"}, false, nil
	case path == "/v1/system/config-repos":
		return "system.read", model.ResourceRef{Type: "system", ID: "config-repos"}, false, nil
	case path == "/v1/system/config-repos/sync":
		return "system.update", model.ResourceRef{Type: "system", ID: "config-repos"}, false, nil
	case strings.HasPrefix(path, "/v1/system/data/"):
		if r.Method == http.MethodGet || path == "/v1/system/data/cleanup/preview" {
			return "system.read", model.ResourceRef{Type: "system", ID: "config"}, false, nil
		}
		return "system.update", model.ResourceRef{Type: "system", ID: "config"}, false, nil
	case path == "/v1/internal/config/sync":
		return "system.update", model.ResourceRef{Type: "system", ID: "config-sync"}, false, nil
	case strings.HasPrefix(path, "/v1/internal/runs/"):
		return "", model.ResourceRef{}, false, nil
	case strings.HasPrefix(path, "/v1/setup/"):
		if r.Method == http.MethodGet {
			return "system.read", model.ResourceRef{Type: "system", ID: "config"}, false, nil
		}
		return "system.update", model.ResourceRef{Type: "system", ID: "config"}, false, nil
	case path == "/v1/system/dispatcher":
		return "system.read", model.ResourceRef{Type: "dispatcher", ID: "status"}, false, nil
	case path == "/v1/system/dispatcher/scopes":
		return "system.read", model.ResourceRef{Type: "dispatcher", ID: "scopes"}, false, nil
	case path == "/v1/system/dispatcher/runner-compose":
		return "system.update", model.ResourceRef{Type: "dispatcher", ID: "runners"}, false, nil
	case path == "/v1/system/dispatcher/runner-bootstrap-command":
		return "system.update", model.ResourceRef{Type: "dispatcher", ID: "runners"}, false, nil
	case path == "/v1/system/dispatcher/kubernetes-runner-bootstrap-command":
		return "system.update", model.ResourceRef{Type: "dispatcher", ID: "runners"}, false, nil
	case path == "/v1/system/dispatcher/kubernetes-runner-manifest":
		return "system.update", model.ResourceRef{Type: "dispatcher", ID: "runners"}, false, nil
	case strings.HasPrefix(path, "/v1/system/dispatcher/runners/"):
		return "system.update", model.ResourceRef{Type: "dispatcher", ID: "runners"}, false, nil
	case strings.HasPrefix(path, "/v1/groups/") && strings.HasSuffix(path, "/config-repo/sync"):
		resource = model.ResourceRef{Type: "folder", ID: folderIDFromConfigRepoPath(path, "/config-repo/sync")}
		if r.Method == http.MethodGet {
			return "config_repo.read", resource, false, nil
		}
		return "config_repo.sync", resource, false, nil
	case strings.HasPrefix(path, "/v1/groups/") && strings.HasSuffix(path, "/config-repo/write"):
		return "config_repo.manage", model.ResourceRef{Type: "folder", ID: folderIDFromConfigRepoPath(path, "/config-repo/write")}, false, nil
	case strings.HasPrefix(path, "/v1/groups/") && strings.HasSuffix(path, "/config-repo/drift"):
		return "config_repo.read", model.ResourceRef{Type: "folder", ID: folderIDFromConfigRepoPath(path, "/config-repo/drift")}, false, nil
	case strings.HasPrefix(path, "/v1/groups/") && strings.HasSuffix(path, "/config-repo"):
		resource = model.ResourceRef{Type: "folder", ID: folderIDFromConfigRepoPath(path, "/config-repo")}
		switch r.Method {
		case http.MethodGet:
			return "config_repo.read", resource, false, nil
		case http.MethodPut, http.MethodPatch, http.MethodDelete:
			return "config_repo.manage", resource, false, nil
		}
	case strings.HasPrefix(path, "/v1/groups/") && strings.HasSuffix(path, "/notifications"):
		resource = model.ResourceRef{Type: "folder", ID: folderIDFromConfigRepoPath(path, "/notifications")}
		if r.Method == http.MethodGet {
			return "config_repo.read", resource, false, nil
		}
		return "config_repo.manage", resource, false, nil
	case path == "/v1/groups":
		switch r.Method {
		case http.MethodGet:
			return "folder.list", model.ResourceRef{Type: "folder", ID: "*"}, true, nil
		case http.MethodPost:
			return "", model.ResourceRef{}, false, nil
		}
	case strings.HasPrefix(path, "/v1/groups/") && strings.HasSuffix(path, "/move"):
		return "", model.ResourceRef{}, false, nil
	case strings.HasPrefix(path, "/v1/groups/"):
		switch r.Method {
		case http.MethodPut, http.MethodPatch:
			return "", model.ResourceRef{}, false, nil
		case http.MethodDelete:
			return "", model.ResourceRef{}, false, nil
		}
	case path == "/v1/pipelines" && r.Method == http.MethodGet:
		return "pipeline.list", model.ResourceRef{Type: "pipeline", ID: "*"}, true, nil
	case path == "/v1/schedules" && r.Method == http.MethodGet:
		return "pipeline_schedule.list", model.ResourceRef{Type: "pipeline_schedule", ID: "*"}, true, nil
	case path == "/v1/schedules" && r.Method == http.MethodPost:
		return "", model.ResourceRef{}, false, nil
	case strings.HasPrefix(path, "/v1/schedules/"):
		return "", model.ResourceRef{}, false, nil
	case path == "/v1/external-triggers" && r.Method == http.MethodGet:
		return "external_trigger.read", model.ResourceRef{Type: "external_trigger", ID: "*"}, true, nil
	case path == "/v1/external-triggers" && r.Method == http.MethodPost:
		return "", model.ResourceRef{}, false, nil
	case strings.HasPrefix(path, "/v1/external-triggers/") && strings.HasSuffix(path, "/invoke"):
		return "", model.ResourceRef{}, false, nil
	case strings.HasPrefix(path, "/v1/external-triggers/") && strings.HasSuffix(path, "/invocations"):
		return "external_trigger.read", model.ResourceRef{Type: "external_trigger", ID: externalTriggerIDFromRequest(r)}, false, nil
	case strings.HasPrefix(path, "/v1/external-triggers/"):
		resource = model.ResourceRef{Type: "external_trigger", ID: externalTriggerIDFromRequest(r)}
		switch r.Method {
		case http.MethodGet:
			return "external_trigger.read", resource, false, nil
		case http.MethodPut, http.MethodPatch:
			return "external_trigger.update", resource, false, nil
		case http.MethodDelete:
			return "external_trigger.delete", resource, false, nil
		}
	case strings.HasPrefix(path, "/v1/pipelines/"):
		pipelineID := normalizePathIdentifier(pathValueOrTail(r, "pipelineName", "/v1/pipelines/"))
		switch r.Method {
		case http.MethodGet:
			return "pipeline.read", model.ResourceRef{Type: "pipeline", ID: pipelineID}, false, nil
		case http.MethodPut:
			return "", model.ResourceRef{Type: "pipeline", ID: pipelineID}, false, nil
		case http.MethodDelete:
			return "pipeline.delete", model.ResourceRef{Type: "pipeline", ID: pipelineID}, false, nil
		}
	case path == "/v1/run" && r.Method == http.MethodPost:
		return "", model.ResourceRef{}, false, nil
	case strings.HasPrefix(path, "/v1/run/") && r.Method == http.MethodPost:
		return "pipeline.execute", model.ResourceRef{Type: "pipeline", ID: normalizePathIdentifier(pathValueOrTail(r, "pipelineName", "/v1/run/"))}, false, nil
	case path == "/v1/runs" && r.Method == http.MethodGet:
		return "pipeline_run.list", model.ResourceRef{Type: "pipeline_run", ID: "*"}, true, nil
	case strings.HasPrefix(path, "/v1/runs/") && strings.Contains(path, "/approvals/") &&
		(strings.HasSuffix(path, "/approve") || strings.HasSuffix(path, "/reject")):
		return "", model.ResourceRef{}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/approvals"):
		return "", model.ResourceRef{Type: "pipeline_run", ID: runIDFromRequest(r)}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/rerun"):
		return "pipeline_run.rerun", model.ResourceRef{Type: "pipeline_run", ID: runIDFromRequest(r)}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/cancel"):
		return "pipeline_run.cancel", model.ResourceRef{Type: "pipeline_run", ID: runIDFromRequest(r)}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/finalize"):
		return "pipeline_run.finalize", model.ResourceRef{Type: "pipeline_run", ID: runIDFromRequest(r)}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/logs/ingest"):
		return "pipeline_run.write_logs", model.ResourceRef{Type: "pipeline_run", ID: runIDFromRequest(r)}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && strings.Contains(path, "/steps/") && strings.Contains(path, "/tasks/"):
		return "pipeline_run.task_update", model.ResourceRef{Type: "pipeline_run", ID: runIDFromRequest(r)}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/status"):
		return "pipeline_run.read", model.ResourceRef{Type: "pipeline_run", ID: runIDFromRequest(r)}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/logs"):
		return "pipeline_run.read_logs", model.ResourceRef{Type: "pipeline_run", ID: runIDFromRequest(r)}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && r.Method == http.MethodGet:
		return "", model.ResourceRef{Type: "pipeline_run", ID: runIDFromRequest(r)}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && r.Method == http.MethodDelete:
		return "pipeline_run.delete", model.ResourceRef{Type: "pipeline_run", ID: runIDFromRequest(r)}, false, nil
	case strings.HasPrefix(path, "/v1/runs-by-check/"):
		return "", model.ResourceRef{}, false, nil
	case path == "/v1/overrides" && r.Method == http.MethodGet:
		return "trigger.read", model.ResourceRef{Type: "trigger", ID: "*"}, true, nil
	case strings.HasPrefix(path, "/v1/overrides/"):
		resource = model.ResourceRef{Type: "trigger", ID: triggerIDFromOverrideRequest(r)}
		switch r.Method {
		case http.MethodGet:
			return "trigger.read", resource, false, nil
		case http.MethodPut:
			return "trigger.update", resource, false, nil
		case http.MethodDelete:
			return "trigger.delete", resource, false, nil
		}
	case path == "/v1/secrets" && r.Method == http.MethodGet:
		return "secret.list_metadata", model.ResourceRef{Type: "secret", ID: "*"}, true, nil
	case path == "/v1/secrets/scopes" && r.Method == http.MethodGet:
		return "secret.list_metadata", model.ResourceRef{Type: "secret", ID: "*"}, true, nil
	case path == "/v1/secrets/encrypt" && r.Method == http.MethodPost:
		return "", model.ResourceRef{}, false, nil
	case strings.HasPrefix(path, "/v1/secrets/"):
		resource = BuildSecretResource("", r.URL.Query().Get("scope"), pathValueOrSegment(r, "secretName", 2))
		switch r.Method {
		case http.MethodGet:
			return "secret.read_value", resource, false, nil
		case http.MethodPut:
			return "secret.write_value", resource, false, nil
		case http.MethodDelete:
			return "secret.delete", resource, false, nil
		}
	case strings.HasPrefix(path, "/v1/repositories/") && strings.HasSuffix(path, "/secrets") && r.Method == http.MethodGet:
		return "secret.list_metadata", model.ResourceRef{Type: "secret", ID: "*"}, true, nil
	case strings.Contains(path, "/secrets/"):
		resource = BuildSecretResource(repositoryIDFromRequest(r), r.URL.Query().Get("scope"), pathValueOrSegment(r, "secretName", 5))
		switch r.Method {
		case http.MethodPut:
			return "secret.write_value", resource, false, nil
		case http.MethodDelete:
			return "secret.delete", resource, false, nil
		}
	case path == "/v1/variables" && r.Method == http.MethodGet:
		return "variable.list_metadata", model.ResourceRef{Type: "variable", ID: "*"}, true, nil
	case path == "/v1/variables/scopes" && r.Method == http.MethodGet:
		return "variable.list_metadata", model.ResourceRef{Type: "variable", ID: "*"}, true, nil
	case strings.HasPrefix(path, "/v1/variables/"):
		resource = BuildVariableResource("", r.URL.Query().Get("scope"), pathValueOrSegment(r, "variableName", 2))
		switch r.Method {
		case http.MethodGet:
			return "variable.read_value", resource, false, nil
		case http.MethodPut:
			return "variable.write_value", resource, false, nil
		case http.MethodDelete:
			return "variable.delete", resource, false, nil
		}
	case strings.HasPrefix(path, "/v1/repositories/") && strings.HasSuffix(path, "/variables") && r.Method == http.MethodGet:
		return "variable.list_metadata", model.ResourceRef{Type: "variable", ID: "*"}, true, nil
	case strings.Contains(path, "/variables/"):
		resource = BuildVariableResource(repositoryIDFromRequest(r), r.URL.Query().Get("scope"), pathValueOrSegment(r, "variableName", 5))
		switch r.Method {
		case http.MethodGet:
			return "variable.read_value", resource, false, nil
		case http.MethodPut:
			return "variable.write_value", resource, false, nil
		case http.MethodDelete:
			return "variable.delete", resource, false, nil
		}
	case strings.HasPrefix(path, "/v1/repositories/") && strings.Contains(path, "/branches/") && r.Method == http.MethodDelete:
		return "pipeline_run.delete", model.ResourceRef{
			Type: "repository",
			ID:   repositoryIDFromRequest(r),
		}, false, nil
	case strings.HasPrefix(path, "/v1/repositories/") && strings.HasSuffix(path, "/branches") && r.Method == http.MethodGet:
		return "repository.read", model.ResourceRef{Type: "repository", ID: repositoryIDFromRequest(r)}, false, nil
	case path == "/v1/steps" && r.Method == http.MethodGet:
		return "step.read", model.ResourceRef{Type: "step", ID: "*"}, true, nil
	case strings.HasPrefix(path, "/v1/steps/"):
		resource = StepResource(stepIdentifierFromPathValue(pathValueOrTail(r, "stepPath", "/v1/steps/"), pathValueOrTail(r, "stepName", "/v1/steps/")))
		switch r.Method {
		case http.MethodGet:
			return "step.read", resource, false, nil
		case http.MethodPut:
			return "", resource, false, nil
		case http.MethodDelete:
			return "step.delete", resource, false, nil
		}
	case path == "/v1/knowledge-contexts" && r.Method == http.MethodGet:
		return "knowledge_context.read", model.ResourceRef{Type: "knowledge_context", ID: "*"}, true, nil
	case strings.HasPrefix(path, "/v1/knowledge-contexts/"):
		resourceID := normalizePathIdentifier(pathValueOrTail(r, "knowledgeID", "/v1/knowledge-contexts/"))
		resource = model.ResourceRef{Type: "knowledge_context", ID: resourceID}
		switch r.Method {
		case http.MethodGet:
			return "knowledge_context.read", resource, false, nil
		case http.MethodPut, http.MethodPatch:
			return "", resource, false, nil
		case http.MethodDelete:
			return "knowledge_context.delete", resource, false, nil
		}
	case strings.HasPrefix(path, "/v1/access/"):
		return "", model.ResourceRef{}, false, nil
	}

	return "", model.ResourceRef{}, false, nil
}

func PipelineResource(path, name string) model.ResourceRef {
	return model.ResourceRef{
		Type: "pipeline",
		ID:   model.BuildPipelineID(path, name),
	}
}

func RunResource(runID string) model.ResourceRef {
	return model.ResourceRef{
		Type: "pipeline_run",
		ID:   strings.TrimSpace(runID),
	}
}

func BuildSecretResource(repoName, scope, name string) model.ResourceRef {
	return model.ResourceRef{
		Type: "secret",
		ID:   model.BuildNamedResourceID(repoName, normalizeRuntimeScopeForResource(scope), name),
	}
}

func BuildVariableResource(repoName, scope, name string) model.ResourceRef {
	return model.ResourceRef{
		Type: "variable",
		ID:   model.BuildNamedResourceID(repoName, normalizeRuntimeScopeForResource(scope), name),
	}
}

func normalizeRuntimeScopeForResource(scope string) string {
	scope = strings.Trim(strings.TrimSpace(scope), "/")
	if strings.EqualFold(scope, "default") {
		return ""
	}
	return scope
}

func BuildTriggerResource(repoOwner, repoName string) model.ResourceRef {
	return model.ResourceRef{
		Type: "trigger",
		ID:   buildRepositoryID(repoOwner, repoName),
	}
}

func ExternalTriggerResource(id string) model.ResourceRef {
	return model.ResourceRef{
		Type: "external_trigger",
		ID:   normalizePathIdentifier(id),
	}
}

func StepResource(identifier string) model.ResourceRef {
	return model.ResourceRef{
		Type: "step",
		ID:   normalizeStepIdentifier(identifier),
	}
}

func buildRepositoryID(repoOwner, repoName string) string {
	repoOwner = strings.Trim(strings.TrimSpace(repoOwner), "/")
	repoName = strings.Trim(strings.TrimSpace(repoName), "/")
	if repoOwner == "" {
		return repoName
	}
	return fmt.Sprintf("%s/%s", repoOwner, repoName)
}

func normalizePathIdentifier(value string) string {
	return strings.Trim(strings.TrimSpace(value), "/")
}

func pathValueOrSegment(r *http.Request, name string, index int) string {
	if r == nil {
		return ""
	}
	if value := strings.TrimSpace(r.PathValue(name)); value != "" {
		return value
	}
	return pathSegment(r.URL.Path, index)
}

func pathValueOrTail(r *http.Request, name, prefix string) string {
	if r == nil {
		return ""
	}
	if value := strings.TrimSpace(r.PathValue(name)); value != "" {
		return value
	}
	return pathTail(r.URL.Path, prefix)
}

func runIDFromRequest(r *http.Request) string {
	return pathValueOrSegment(r, "runID", 2)
}

func repositoryIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	return buildRepositoryID(pathValueOrSegment(r, "repoOwner", 2), pathValueOrSegment(r, "repoName", 3))
}

func triggerIDFromOverrideRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if owner := strings.TrimSpace(r.PathValue("repoOwner")); owner != "" {
		return buildRepositoryID(owner, r.PathValue("repoName"))
	}
	return normalizePathIdentifier(pathTail(r.URL.Path, "/v1/overrides/"))
}

func externalTriggerIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if value := strings.TrimSpace(r.PathValue("id")); value != "" {
		return normalizePathIdentifier(value)
	}
	value := pathTail(r.URL.Path, "/v1/external-triggers/")
	value = strings.TrimSuffix(value, "/invoke")
	value = strings.TrimSuffix(value, "/invocations")
	return normalizePathIdentifier(value)
}

func pathSegment(path string, index int) string {
	segments := pathSegments(path)
	if index < 0 || index >= len(segments) {
		return ""
	}
	return segments[index]
}

func pathTail(path, prefix string) string {
	path = strings.TrimSpace(path)
	prefix = strings.TrimSpace(prefix)
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	value := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if decoded, err := url.PathUnescape(value); err == nil {
		value = decoded
	}
	return normalizePathIdentifier(value)
}

func pathSegments(path string) []string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return nil
	}
	raw := strings.Split(path, "/")
	segments := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if decoded, err := url.PathUnescape(part); err == nil {
			part = decoded
		}
		segments = append(segments, part)
	}
	return segments
}

func folderIDFromConfigRepoPath(path, suffix string) string {
	folderID := strings.TrimSpace(path)
	folderID = strings.TrimPrefix(folderID, "/v1/groups/")
	folderID = strings.TrimSuffix(folderID, suffix)
	if decoded, err := url.PathUnescape(folderID); err == nil {
		folderID = decoded
	}
	return normalizePathIdentifier(folderID)
}

func normalizeStepIdentifier(value string) string {
	value = normalizePathIdentifier(value)
	if strings.HasSuffix(value, "/usage") {
		value = strings.TrimSuffix(value, "/usage")
		value = normalizePathIdentifier(value)
	}
	return value
}

func stepIdentifierFromPathValue(stepPath, stepName string) string {
	if strings.TrimSpace(stepPath) != "" {
		return stepPath
	}
	return stepName
}

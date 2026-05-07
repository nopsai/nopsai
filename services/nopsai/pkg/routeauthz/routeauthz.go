package routeauthz

import (
	"fmt"
	"net/http"
	"strings"

	"nopsai/services/aaa/pkg/model"
)

func MapRequest(r *http.Request) (action string, resource model.ResourceRef, requiresFilter bool, err error) {
	if r == nil {
		return "", model.ResourceRef{}, false, nil
	}

	path := strings.TrimSpace(r.URL.Path)
	switch {
	case path == "/v1/auth/me", path == "/v1/auth/password", path == "/v1/auth/email":
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
	case path == "/v1/system/config/sync":
		if r.Method == http.MethodGet {
			return "system.read", model.ResourceRef{Type: "system", ID: "config-sync"}, false, nil
		}
		return "system.update", model.ResourceRef{Type: "system", ID: "config-sync"}, false, nil
	case path == "/v1/internal/config/sync":
		return "system.update", model.ResourceRef{Type: "system", ID: "config-sync"}, false, nil
	case path == "/v1/system/dispatcher":
		return "system.read", model.ResourceRef{Type: "dispatcher", ID: "status"}, false, nil
	case strings.HasPrefix(path, "/v1/system/dispatcher/runners/"):
		return "system.update", model.ResourceRef{Type: "dispatcher", ID: "runners"}, false, nil
	case path == "/v1/groups":
		switch r.Method {
		case http.MethodGet:
			return "folder.list", model.ResourceRef{Type: "folder", ID: "*"}, false, nil
		case http.MethodPost:
			return "folder.create", model.ResourceRef{Type: "folder", ID: "*"}, false, nil
		}
	case strings.HasPrefix(path, "/v1/groups/") && strings.HasSuffix(path, "/move"):
		return "folder.move", model.ResourceRef{Type: "folder", ID: strings.TrimSpace(r.PathValue("groupID"))}, false, nil
	case strings.HasPrefix(path, "/v1/groups/"):
		switch r.Method {
		case http.MethodPut, http.MethodPatch:
			return "folder.update", model.ResourceRef{Type: "folder", ID: strings.TrimSpace(r.PathValue("groupID"))}, false, nil
		case http.MethodDelete:
			return "folder.delete", model.ResourceRef{Type: "folder", ID: strings.TrimSpace(r.PathValue("groupID"))}, false, nil
		}
	case path == "/v1/pipelines" && r.Method == http.MethodGet:
		return "pipeline.list", model.ResourceRef{Type: "pipeline", ID: "*"}, true, nil
	case strings.HasPrefix(path, "/v1/pipelines/"):
		pipelineID := normalizePathIdentifier(r.PathValue("pipelineName"))
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
		return "pipeline.execute", model.ResourceRef{Type: "pipeline", ID: normalizePathIdentifier(r.PathValue("pipelineName"))}, false, nil
	case path == "/v1/runs" && r.Method == http.MethodGet:
		return "pipeline_run.list", model.ResourceRef{Type: "pipeline_run", ID: "*"}, true, nil
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/rerun"):
		return "pipeline_run.rerun", model.ResourceRef{Type: "pipeline_run", ID: strings.TrimSpace(r.PathValue("runID"))}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/cancel"):
		return "pipeline_run.cancel", model.ResourceRef{Type: "pipeline_run", ID: strings.TrimSpace(r.PathValue("runID"))}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/finalize"):
		return "pipeline_run.finalize", model.ResourceRef{Type: "pipeline_run", ID: strings.TrimSpace(r.PathValue("runID"))}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/logs/ingest"):
		return "pipeline_run.write_logs", model.ResourceRef{Type: "pipeline_run", ID: strings.TrimSpace(r.PathValue("runID"))}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && strings.Contains(path, "/steps/") && strings.Contains(path, "/tasks/"):
		return "pipeline_run.task_update", model.ResourceRef{Type: "pipeline_run", ID: strings.TrimSpace(r.PathValue("runID"))}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/status"):
		return "pipeline_run.read", model.ResourceRef{Type: "pipeline_run", ID: strings.TrimSpace(r.PathValue("runID"))}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/logs"):
		return "pipeline_run.read", model.ResourceRef{Type: "pipeline_run", ID: strings.TrimSpace(r.PathValue("runID"))}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && r.Method == http.MethodGet:
		return "pipeline_run.read", model.ResourceRef{Type: "pipeline_run", ID: strings.TrimSpace(r.PathValue("runID"))}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && r.Method == http.MethodDelete:
		return "pipeline_run.delete", model.ResourceRef{Type: "pipeline_run", ID: strings.TrimSpace(r.PathValue("runID"))}, false, nil
	case strings.HasPrefix(path, "/v1/runs-by-check/"):
		return "pipeline_run.read", model.ResourceRef{Type: "pipeline_run", ID: "*"}, false, nil
	case path == "/v1/overrides" && r.Method == http.MethodGet:
		return "trigger.read", model.ResourceRef{Type: "trigger", ID: "*"}, false, nil
	case strings.HasPrefix(path, "/v1/overrides/"):
		resource = model.ResourceRef{Type: "trigger", ID: buildRepositoryID(r.PathValue("repoOwner"), r.PathValue("repoName"))}
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
		return "secret.list_metadata", model.ResourceRef{Type: "secret", ID: "*"}, false, nil
	case strings.HasPrefix(path, "/v1/secrets/"):
		resource = BuildSecretResource("", r.URL.Query().Get("env"), r.PathValue("secretName"))
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
		resource = BuildSecretResource(buildRepositoryID(r.PathValue("repoOwner"), r.PathValue("repoName")), r.URL.Query().Get("env"), r.PathValue("secretName"))
		switch r.Method {
		case http.MethodPut:
			return "secret.write_value", resource, false, nil
		case http.MethodDelete:
			return "secret.delete", resource, false, nil
		}
	case path == "/v1/variables" && r.Method == http.MethodGet:
		return "variable.list_metadata", model.ResourceRef{Type: "variable", ID: "*"}, true, nil
	case path == "/v1/variables/scopes" && r.Method == http.MethodGet:
		return "variable.list_metadata", model.ResourceRef{Type: "variable", ID: "*"}, false, nil
	case strings.HasPrefix(path, "/v1/variables/"):
		resource = BuildVariableResource("", r.URL.Query().Get("env"), r.PathValue("variableName"))
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
		resource = BuildVariableResource(buildRepositoryID(r.PathValue("repoOwner"), r.PathValue("repoName")), r.URL.Query().Get("env"), r.PathValue("variableName"))
		switch r.Method {
		case http.MethodGet:
			return "variable.read_value", resource, false, nil
		case http.MethodPut:
			return "variable.write_value", resource, false, nil
		case http.MethodDelete:
			return "variable.delete", resource, false, nil
		}
	case strings.HasPrefix(path, "/v1/repositories/") && strings.HasSuffix(path, "/branches") && r.Method == http.MethodGet:
		return "system.read", model.ResourceRef{Type: "repository", ID: buildRepositoryID(r.PathValue("repoOwner"), r.PathValue("repoName"))}, false, nil
	case path == "/v1/steps" && r.Method == http.MethodGet:
		return "system.read", model.ResourceRef{Type: "system", ID: "steps"}, false, nil
	case strings.HasPrefix(path, "/v1/steps/"):
		switch r.Method {
		case http.MethodGet:
			return "system.read", model.ResourceRef{Type: "system", ID: "steps"}, false, nil
		case http.MethodPut, http.MethodDelete:
			return "system.update", model.ResourceRef{Type: "system", ID: "steps"}, false, nil
		}
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
		ID:   model.BuildNamedResourceID(repoName, scope, name),
	}
}

func BuildVariableResource(repoName, scope, name string) model.ResourceRef {
	return model.ResourceRef{
		Type: "variable",
		ID:   model.BuildNamedResourceID(repoName, scope, name),
	}
}

func BuildTriggerResource(repoOwner, repoName string) model.ResourceRef {
	return model.ResourceRef{
		Type: "trigger",
		ID:   buildRepositoryID(repoOwner, repoName),
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

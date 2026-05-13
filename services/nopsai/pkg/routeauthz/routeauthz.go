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
	case path == "/v1/system/config-repos":
		return "system.read", model.ResourceRef{Type: "system", ID: "config-repos"}, false, nil
	case path == "/v1/system/config-repos/sync":
		return "system.update", model.ResourceRef{Type: "system", ID: "config-repos"}, false, nil
	case path == "/v1/internal/config/sync":
		return "system.update", model.ResourceRef{Type: "system", ID: "config-sync"}, false, nil
	case path == "/v1/system/dispatcher":
		return "system.read", model.ResourceRef{Type: "dispatcher", ID: "status"}, false, nil
	case strings.HasPrefix(path, "/v1/system/dispatcher/runners/"):
		return "system.update", model.ResourceRef{Type: "dispatcher", ID: "runners"}, false, nil
	case strings.HasPrefix(path, "/v1/groups/") && strings.HasSuffix(path, "/config-repo/sync"):
		resource = model.ResourceRef{Type: "folder", ID: folderIDFromConfigRepoPath(path, "/config-repo/sync")}
		if r.Method == http.MethodGet {
			return "config_repo.read", resource, false, nil
		}
		return "config_repo.sync", resource, false, nil
	case strings.HasPrefix(path, "/v1/groups/") && strings.HasSuffix(path, "/config-repo"):
		resource = model.ResourceRef{Type: "folder", ID: folderIDFromConfigRepoPath(path, "/config-repo")}
		switch r.Method {
		case http.MethodGet:
			return "config_repo.read", resource, false, nil
		case http.MethodPut, http.MethodPatch, http.MethodDelete:
			return "config_repo.manage", resource, false, nil
		}
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
		return "pipeline_run.read_logs", model.ResourceRef{Type: "pipeline_run", ID: strings.TrimSpace(r.PathValue("runID"))}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && r.Method == http.MethodGet:
		return "pipeline_run.read", model.ResourceRef{Type: "pipeline_run", ID: strings.TrimSpace(r.PathValue("runID"))}, false, nil
	case strings.HasPrefix(path, "/v1/runs/") && r.Method == http.MethodDelete:
		return "pipeline_run.delete", model.ResourceRef{Type: "pipeline_run", ID: strings.TrimSpace(r.PathValue("runID"))}, false, nil
	case strings.HasPrefix(path, "/v1/runs-by-check/"):
		return "", model.ResourceRef{}, false, nil
	case path == "/v1/overrides" && r.Method == http.MethodGet:
		return "trigger.read", model.ResourceRef{Type: "trigger", ID: "*"}, true, nil
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
		return "secret.list_metadata", model.ResourceRef{Type: "secret", ID: "*"}, true, nil
	case strings.HasPrefix(path, "/v1/secrets/"):
		resource = BuildSecretResource("", r.URL.Query().Get("scope"), r.PathValue("secretName"))
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
		resource = BuildSecretResource(buildRepositoryID(r.PathValue("repoOwner"), r.PathValue("repoName")), r.URL.Query().Get("scope"), r.PathValue("secretName"))
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
		resource = BuildVariableResource("", r.URL.Query().Get("scope"), r.PathValue("variableName"))
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
		resource = BuildVariableResource(buildRepositoryID(r.PathValue("repoOwner"), r.PathValue("repoName")), r.URL.Query().Get("scope"), r.PathValue("variableName"))
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
			ID:   buildRepositoryID(r.PathValue("repoOwner"), r.PathValue("repoName")),
		}, false, nil
	case strings.HasPrefix(path, "/v1/repositories/") && strings.HasSuffix(path, "/branches") && r.Method == http.MethodGet:
		return "repository.read", model.ResourceRef{Type: "repository", ID: buildRepositoryID(r.PathValue("repoOwner"), r.PathValue("repoName"))}, false, nil
	case path == "/v1/steps" && r.Method == http.MethodGet:
		return "step.read", model.ResourceRef{Type: "step", ID: "*"}, true, nil
	case strings.HasPrefix(path, "/v1/steps/"):
		resource = StepResource(stepIdentifierFromPathValue(r.PathValue("stepPath"), r.PathValue("stepName")))
		switch r.Method {
		case http.MethodGet:
			return "step.read", resource, false, nil
		case http.MethodPut:
			return "", resource, false, nil
		case http.MethodDelete:
			return "step.delete", resource, false, nil
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

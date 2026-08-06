package nopsai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/pkg/routeauthz"
	configstore "nopsai/services/nopsai/pkg/store"
	"nopsai/services/nopsai/pkg/validation"
)

type yamlValidationRequest struct {
	YAML       string `json:"yaml"`
	Content    string `json:"content"`
	ResourceID string `json:"resource_id"`
	Path       string `json:"path"`
	Name       string `json:"name"`
	Repository string `json:"repository"`
	RepoOwner  string `json:"repo_owner"`
	RepoName   string `json:"repo_name"`
	TeamPath   string `json:"team_path"`
}

type configRepositoryValidationRequest struct {
	BasePath string                         `json:"base_path"`
	Files    []configRepositoryValidateFile `json:"files"`
}

type configRepositoryValidateFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Delete  bool   `json:"delete,omitempty"`
}

func (a *App) handleValidatePipeline(w http.ResponseWriter, r *http.Request) {
	req, raw, ok := decodeYAMLValidationRequest(w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(raw) == "" {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(validationIssue{
			Message: "pipeline YAML is required",
			Path:    "yaml",
			Code:    "required",
		}))
		return
	}
	if yamlRootHasKey(raw, "triggers") && !yamlRootHasKey(raw, "steps") {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(validationIssue{
			Message: "The provided file appears to be a trigger manifest, not a pipeline. A pipeline must contain 'steps', not 'triggers'.",
			Path:    "triggers",
			Line:    yamlMappingKeyLine(decodeValidationYAMLRoot(raw), "triggers"),
			Code:    "resource_kind_mismatch",
		}))
		return
	}

	var pipeline models.Pipeline
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(raw)))
	if err := decoder.Decode(&pipeline); err != nil {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(validationParseIssue(raw, "Pipeline YAML is malformed", err)))
		return
	}
	if issue, invalid := pipelineNameMismatchIssue(raw, req, pipeline.Name); invalid {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(issue))
		return
	}
	if !a.requireValidationStepUseDecisions(w, r, &pipeline) {
		return
	}
	if err := a.validatePipelineWithStoredStepIncludes(r.Context(), &pipeline); err != nil {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(validationIssueFromError(raw, "pipeline", err)))
		return
	}
	writeValidationResponse(w, http.StatusOK, validValidationResponse())
}

func (a *App) handleValidateReusableStep(w http.ResponseWriter, r *http.Request) {
	req, raw, ok := decodeYAMLValidationRequest(w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(raw) == "" {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(validationIssue{
			Message: "reusable step YAML is required",
			Path:    "yaml",
			Code:    "required",
		}))
		return
	}

	var step models.PipelineStep
	if err := yaml.Unmarshal([]byte(raw), &step); err != nil {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(validationParseIssue(raw, "Reusable step YAML is malformed", err)))
		return
	}
	stepName := step.GetName()
	if stepName == "" {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(validationIssue{
			Message: "a reusable step must have a 'name' field in its definition",
			Path:    "name",
			Line:    yamlMappingKeyLine(decodeValidationYAMLRoot(raw), "name"),
			Code:    "required",
		}))
		return
	}
	if issue, invalid := stepNameMismatchIssue(raw, req, stepName); invalid {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(issue))
		return
	}
	if err := validation.ValidateReusableStep(&step); err != nil {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(validationIssueFromError(raw, "step", err)))
		return
	}
	writeValidationResponse(w, http.StatusOK, validValidationResponse())
}

func (a *App) handleValidateTriggerOverride(w http.ResponseWriter, r *http.Request) {
	req, raw, ok := decodeYAMLValidationRequest(w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(raw) == "" {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(validationIssue{
			Message: "trigger manifest YAML is required",
			Path:    "yaml",
			Code:    "required",
		}))
		return
	}
	if yamlRootHasKey(raw, "steps") {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(validationIssue{
			Message: "The provided file appears to be a pipeline, not a trigger manifest. A trigger must contain 'triggers', not 'steps'.",
			Path:    "steps",
			Line:    yamlMappingKeyLine(decodeValidationYAMLRoot(raw), "steps"),
			Code:    "resource_kind_mismatch",
		}))
		return
	}

	repository := triggerValidationRepository(req)
	if repository == "" {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(validationIssue{
			Message: "repository is required for trigger validation",
			Path:    "repository",
			Code:    "required",
		}))
		return
	}

	var manifest models.Manifest
	if err := yaml.Unmarshal([]byte(raw), &manifest); err != nil {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(validationParseIssue(raw, "Trigger manifest YAML is malformed", err)))
		return
	}
	fallbackTeam := fallbackRepositoryTriggerTeamPath(repository)
	if strings.TrimSpace(req.TeamPath) != "" {
		fallbackTeam = req.TeamPath
	}
	record, err := repositoryTriggerRecordFromManifest(repository, raw, "database", resourceVisibilityTeam, manifest, fallbackTeam)
	if err != nil {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(validationIssueFromError(raw, "trigger", err)))
		return
	}
	if a != nil && a.db != nil {
		err = validateRepositoryTriggerWebhookSource(r.Context(), a.db, record)
	} else {
		err = validateRepositoryTriggerForNopsAI(record)
	}
	if err != nil {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(validationIssueFromError(raw, "trigger", err)))
		return
	}
	writeValidationResponse(w, http.StatusOK, validValidationResponse())
}

func (a *App) handleValidateGlobalConfigRepository(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeConfigRepositoryValidationRequest(w, r)
	if !ok {
		return
	}
	repo, err := a.validationConfigRepository(r.Context(), models.ConfigRepositoryScopeSystem, models.ConfigRepositorySystemGlobalID, req.BasePath)
	if err != nil {
		validationBadRequest(w, err.Error(), "invalid_config_repository")
		return
	}
	a.handleValidateConfigRepositoryBundle(w, r, repo, req)
}

func (a *App) handleValidateTeamConfigRepository(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireTeamConfigRepositoryDecision(w, r, "config_repo.read")
	if !ok {
		return
	}
	req, decoded := decodeConfigRepositoryValidationRequest(w, r)
	if !decoded {
		return
	}
	repo, err := a.validationConfigRepository(r.Context(), models.ConfigRepositoryScopeTeam, resource.ID, req.BasePath)
	if err != nil {
		validationBadRequest(w, err.Error(), "invalid_config_repository")
		return
	}
	a.handleValidateConfigRepositoryBundle(w, r, repo, req)
}

func (a *App) handleValidateConfigRepositoryBundle(w http.ResponseWriter, _ *http.Request, repo models.ConfigRepository, req configRepositoryValidationRequest) {
	if len(req.Files) == 0 {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(validationIssue{
			Message: "files is required",
			Path:    "files",
			Code:    "required",
		}))
		return
	}
	repoCtx, err := newConfigSyncRepositoryContext(repo)
	if err != nil {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(validationIssue{
			Message: err.Error(),
			Path:    "base_path",
			Code:    "invalid_config_repository",
		}))
		return
	}
	files, err := configSyncRepositoryFilesFromValidationRequest(repoCtx, req)
	if err != nil {
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(validationIssue{
			Message: err.Error(),
			Path:    "files",
			Code:    "invalid_file_path",
		}))
		return
	}
	if _, err := a.parseConfigSyncPlan(repo, repoCtx, files); err != nil {
		issue := validationIssue{
			Message: err.Error(),
			File:    validationFileFromError(err, req.Files),
			Line:    yamlErrorLine(err),
			Code:    validationCodeForMessage(err.Error()),
		}
		writeValidationResponse(w, http.StatusOK, invalidValidationResponse(issue))
		return
	}
	writeValidationResponse(w, http.StatusOK, validValidationResponse())
}

func decodeYAMLValidationRequest(w http.ResponseWriter, r *http.Request) (yamlValidationRequest, string, bool) {
	defer r.Body.Close()
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(contentType, "json") {
		var req yamlValidationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			validationBadRequest(w, "invalid JSON request payload", "invalid_json")
			return yamlValidationRequest{}, "", false
		}
		raw := firstNonEmptyString(req.YAML, req.Content)
		return req, raw, true
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		validationBadRequest(w, "error reading request body", "request_read_failed")
		return yamlValidationRequest{}, "", false
	}
	return yamlValidationRequest{}, string(body), true
}

func decodeConfigRepositoryValidationRequest(w http.ResponseWriter, r *http.Request) (configRepositoryValidationRequest, bool) {
	defer r.Body.Close()
	var req configRepositoryValidationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validationBadRequest(w, "invalid JSON request payload", "invalid_json")
		return configRepositoryValidationRequest{}, false
	}
	basePath, err := configsync.NormalizeRepositoryBasePathForRequest(req.BasePath)
	if err != nil {
		validationBadRequest(w, err.Error(), "invalid_base_path")
		return configRepositoryValidationRequest{}, false
	}
	req.BasePath = basePath
	return req, true
}

func pipelineNameMismatchIssue(raw string, req yamlValidationRequest, yamlName string) (validationIssue, bool) {
	resourceID := strings.Trim(strings.TrimSpace(firstNonEmptyString(req.ResourceID, req.Path)), "/")
	if resourceID == "" && strings.TrimSpace(req.Name) == "" {
		return validationIssue{}, false
	}
	expected := strings.TrimSpace(req.Name)
	if expected == "" {
		_, expectedName, _, err := configsync.SplitPipelineIdentifier(resourceID)
		if err != nil {
			return validationIssue{Message: err.Error(), Path: "resource_id", Code: "invalid_resource_id"}, true
		}
		expected = expectedName
	}
	if expected != "" && expected != yamlName {
		return validationIssue{
			Message: fmt.Sprintf("the pipeline name in the target ('%s') must match the 'name' field in the YAML ('%s')", expected, yamlName),
			Path:    "name",
			Line:    yamlMappingKeyLine(decodeValidationYAMLRoot(raw), "name"),
			Code:    "name_mismatch",
		}, true
	}
	return validationIssue{}, false
}

func stepNameMismatchIssue(raw string, req yamlValidationRequest, yamlName string) (validationIssue, bool) {
	resourceID := strings.Trim(strings.TrimSpace(firstNonEmptyString(req.ResourceID, req.Path)), "/")
	if resourceID == "" && strings.TrimSpace(req.Name) == "" {
		return validationIssue{}, false
	}
	expected := strings.TrimSpace(req.Name)
	if expected == "" {
		_, expectedName, _, err := configsync.SplitStepIdentifier(resourceID)
		if err != nil {
			return validationIssue{Message: err.Error(), Path: "resource_id", Code: "invalid_resource_id"}, true
		}
		expected = expectedName
	}
	if expected != "" && expected != yamlName {
		return validationIssue{
			Message: fmt.Sprintf("the reusable step name in the target ('%s') must match the 'name' field in the YAML ('%s')", expected, yamlName),
			Path:    "name",
			Line:    yamlMappingKeyLine(decodeValidationYAMLRoot(raw), "name"),
			Code:    "name_mismatch",
		}, true
	}
	return validationIssue{}, false
}

func triggerValidationRepository(req yamlValidationRequest) string {
	repository := strings.Trim(strings.TrimSpace(req.Repository), "/")
	if repository != "" {
		return repository
	}
	owner := strings.Trim(strings.TrimSpace(req.RepoOwner), "/")
	name := strings.Trim(strings.TrimSpace(req.RepoName), "/")
	if owner != "" && name != "" {
		return owner + "/" + name
	}
	return ""
}

func yamlRootHasKey(raw, key string) bool {
	return yamlMappingKeyLine(decodeValidationYAMLRoot(raw), key) > 0
}

func (a *App) requireValidationStepUseDecisions(w http.ResponseWriter, r *http.Request, pipeline *models.Pipeline) bool {
	if pipeline == nil {
		return true
	}
	if a == nil {
		return true
	}
	if _, ok := a.currentAAASubject(r); !ok || !a.aaaAvailable() {
		return true
	}
	for _, stepIdentifier := range collectReferencedStepIdentifiers(pipeline) {
		if !a.requireAAADecision(w, r, "step.use", routeauthz.StepResource(stepIdentifier)) {
			return false
		}
	}
	return true
}

func (a *App) validationConfigRepository(ctx context.Context, scopeType, scopeID, basePath string) (models.ConfigRepository, error) {
	repo := models.ConfigRepository{}
	if a != nil && a.store != nil {
		stored, err := a.store.GetConfigRepositoryByScope(ctx, scopeType, scopeID)
		if err == nil {
			repo = stored
		} else if !errors.Is(err, configstore.ErrConfigRepositoryNotFound) {
			return models.ConfigRepository{}, err
		}
	}
	if strings.TrimSpace(repo.ScopeType) == "" {
		repo = models.ConfigRepository{
			ScopeType: scopeType,
			ScopeID:   scopeID,
			Provider:  models.ConfigRepositoryProviderGitHub,
			RepoURL:   "https://github.com/nopsai/config.git",
			Branch:    "main",
			Enabled:   true,
		}
	}
	if strings.TrimSpace(basePath) != "" || strings.TrimSpace(repo.BasePath) == "" {
		repo.BasePath = strings.TrimSpace(basePath)
	}
	if strings.TrimSpace(repo.Provider) == "" {
		repo.Provider = models.ConfigRepositoryProviderGitHub
	}
	if strings.TrimSpace(repo.RepoURL) == "" {
		repo.RepoURL = "https://github.com/nopsai/config.git"
	}
	if strings.TrimSpace(repo.Branch) == "" {
		repo.Branch = "main"
	}
	return repo, nil
}

func configSyncRepositoryFilesFromValidationRequest(repoCtx configSyncRepositoryContext, req configRepositoryValidationRequest) (configSyncRepositoryFiles, error) {
	files := emptyConfigSyncRepositoryFiles()
	for _, file := range req.Files {
		if file.Delete {
			continue
		}
		normalized, err := normalizeValidationDraftPath(req.BasePath, file.Path)
		if err != nil {
			return configSyncRepositoryFiles{}, err
		}
		if normalized == "" {
			continue
		}
		addConfigValidationFile(&files, repoCtx, normalized, file.Content)
	}
	return files, nil
}

func emptyConfigSyncRepositoryFiles() configSyncRepositoryFiles {
	return configSyncRepositoryFiles{
		pipelines:          map[string]string{},
		steps:              map[string]string{},
		triggers:           map[string]string{},
		externalTriggers:   map[string]string{},
		gitWebhookSources:  map[string]string{},
		dashboards:         map[string]string{},
		dashboardTemplates: map[string]string{},
		schedules:          map[string]string{},
		scopes:             map[string]string{},
		configRepositories: map[string]string{},
		access:             map[string]string{},
		knowledge:          map[string]string{},
		notifications:      map[string]string{},
		teamAIProfiles:     map[string]string{},
		setting:            map[string]string{},
	}
}

func normalizeValidationDraftPath(basePath, rawPath string) (string, error) {
	rawPath = strings.TrimSpace(strings.ReplaceAll(rawPath, "\\", "/"))
	if rawPath == "" {
		return "", fmt.Errorf("file path is required")
	}
	if filepath.IsAbs(rawPath) {
		return "", fmt.Errorf("file path must be relative: %s", rawPath)
	}
	rawPath = strings.Trim(rawPath, "/")
	cleaned := path.Clean(rawPath)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("invalid file path: %s", rawPath)
	}
	for _, segment := range strings.Split(cleaned, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("invalid file path: %s", rawPath)
		}
	}
	basePath = strings.Trim(strings.TrimSpace(basePath), "/")
	if basePath != "" {
		if rel, ok := configsync.RelativePath(cleaned, basePath); ok && rel != "" {
			return cleaned, nil
		}
		return configsync.RepoJoinPath(basePath, cleaned), nil
	}
	return cleaned, nil
}

func addConfigValidationFile(files *configSyncRepositoryFiles, repoCtx configSyncRepositoryContext, filePath, content string) {
	dirs := repoCtx.dirs
	addIfRelative := func(target map[string]string, root string) bool {
		if _, ok := configsync.RelativePath(filePath, root); ok {
			target[filePath] = content
			return true
		}
		return false
	}
	switch {
	case addIfRelative(files.pipelines, dirs.pipeline):
	case addIfRelative(files.steps, dirs.step):
	case addIfRelative(files.triggers, dirs.trigger):
	case addIfRelative(files.externalTriggers, dirs.externalTrigger):
	case addIfRelative(files.gitWebhookSources, dirs.gitWebhookSource):
	case addIfRelative(files.dashboards, dirs.dashboard):
	case addIfRelative(files.dashboardTemplates, dirs.dashboardTemplate):
	case addIfRelative(files.schedules, dirs.schedule):
	case addIfRelative(files.scopes, dirs.scope):
	case addIfRelative(files.configRepositories, dirs.configRepository):
	case addIfRelative(files.access, dirs.access):
	case addIfRelative(files.knowledge, dirs.knowledge):
	case addIfRelative(files.setting, dirs.setting):
	default:
		if repoCtx.boundTeam != "" {
			rel, ok := configsync.RelativePath(filePath, repoCtx.basePath)
			if ok && strings.EqualFold(rel, "notifications.yaml") {
				files.notifications[filePath] = content
			}
			if ok && (strings.EqualFold(rel, "ai-profiles.yaml") || strings.EqualFold(rel, "ai-profiles.yml")) {
				files.teamAIProfiles[filePath] = content
			}
		}
	}
}

func validationFileFromError(err error, files []configRepositoryValidateFile) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	for _, file := range files {
		path := strings.TrimSpace(file.Path)
		if path != "" && strings.Contains(message, path) {
			return path
		}
		normalized := filepath.ToSlash(path)
		if normalized != "" && strings.Contains(message, normalized) {
			return path
		}
	}
	return ""
}

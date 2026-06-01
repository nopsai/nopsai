package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nopsai/pkg/models"

	"gopkg.in/yaml.v3"
)

type configRepositoryDriftResponse struct {
	BaseBranch  string                      `json:"base_branch"`
	PushBranch  string                      `json:"push_branch"`
	Items       []configRepositoryDriftItem `json:"items"`
	Summary     map[string]int              `json:"summary"`
	CanPush     bool                        `json:"can_push"`
	PushMessage string                      `json:"push_message"`
}

type configRepositoryDriftItem struct {
	Path           string  `json:"path"`
	Status         string  `json:"status"`
	GitContent     *string `json:"git_content,omitempty"`
	DesiredContent *string `json:"desired_content,omitempty"`
	Delete         bool    `json:"delete,omitempty"`
}

func (a *App) handleGetGlobalConfigRepositoryDrift(w http.ResponseWriter, r *http.Request) {
	repo, err := a.store.GetConfigRepositoryByScope(r.Context(), models.ConfigRepositoryScopeSystem, models.ConfigRepositorySystemGlobalID)
	if err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to load global config repository")
		return
	}
	a.handleGetConfigRepositoryDrift(w, r, repo)
}

func (a *App) handleGetFolderConfigRepositoryDrift(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireFolderConfigRepositoryDecision(w, r, "config_repo.read")
	if !ok {
		return
	}

	repo, err := a.store.GetConfigRepositoryByScope(r.Context(), models.ConfigRepositoryScopeFolder, resource.ID)
	if err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to load config repository")
		return
	}
	a.handleGetConfigRepositoryDrift(w, r, repo)
}

func (a *App) handleGetConfigRepositoryDrift(w http.ResponseWriter, r *http.Request, repo models.ConfigRepository) {
	desired, err := a.exportConfigRepositoryFiles(r.Context(), repo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	gitFiles, err := a.loadConfigRepositoryGitFiles(repo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	items := diffConfigRepositoryFiles(gitFiles, desired)
	summary := map[string]int{"added": 0, "modified": 0, "deleted": 0, "unchanged": 0}
	for _, item := range items {
		summary[item.Status]++
	}

	writeJSON(w, http.StatusOK, configRepositoryDriftResponse{
		BaseBranch:  repo.Branch,
		PushBranch:  strings.TrimSpace(repo.WriteBranch),
		Items:       items,
		Summary:     summary,
		CanPush:     repo.WriteEnabled && strings.TrimSpace(repo.WriteBranch) != "",
		PushMessage: defaultConfigRepositoryPushMessage(repo),
	})
}

func (a *App) loadConfigRepositoryGitFiles(repo models.ConfigRepository) (map[string]string, error) {
	owner, name, err := parseGitHubRepoURL(repo.RepoURL)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, directory := range []string{"pipelines", "steps", "triggers", "schedules", "scopes", "knowledge", "setting", "settings"} {
		directoryPath := filepath.ToSlash(filepath.Join(strings.Trim(strings.TrimSpace(repo.BasePath), "/"), directory))
		files, err := a.requestGitBotDirectory(owner, name, repo.Branch, directoryPath)
		if err != nil {
			return nil, err
		}
		for filePath, content := range files {
			rel, ok := configRepositoryRelativeGitPath(repo.BasePath, filePath)
			if !ok || !isConfigRepositoryDriftPath(rel) {
				continue
			}
			result[rel] = normalizeConfigRepositoryFileContent(content)
		}
	}
	return result, nil
}

func diffConfigRepositoryFiles(gitFiles, desiredFiles map[string]string) []configRepositoryDriftItem {
	pathSet := make(map[string]struct{}, len(gitFiles)+len(desiredFiles))
	for filePath := range gitFiles {
		pathSet[filePath] = struct{}{}
	}
	for filePath := range desiredFiles {
		pathSet[filePath] = struct{}{}
	}

	paths := make([]string, 0, len(pathSet))
	for filePath := range pathSet {
		paths = append(paths, filePath)
	}
	sort.Strings(paths)

	items := make([]configRepositoryDriftItem, 0, len(paths))
	for _, filePath := range paths {
		gitContent, inGit := gitFiles[filePath]
		desiredContent, inDesired := desiredFiles[filePath]
		switch {
		case !inGit && inDesired:
			content := desiredContent
			items = append(items, configRepositoryDriftItem{Path: filePath, Status: "added", DesiredContent: &content})
		case inGit && !inDesired:
			content := gitContent
			items = append(items, configRepositoryDriftItem{Path: filePath, Status: "deleted", GitContent: &content, Delete: true})
		case inGit && inDesired && !configRepositoryFileContentsEqual(filePath, gitContent, desiredContent):
			before, after := gitContent, desiredContent
			items = append(items, configRepositoryDriftItem{Path: filePath, Status: "modified", GitContent: &before, DesiredContent: &after})
		default:
			before, after := gitContent, desiredContent
			items = append(items, configRepositoryDriftItem{Path: filePath, Status: "unchanged", GitContent: &before, DesiredContent: &after})
		}
	}
	return items
}

func configRepositoryFileContentsEqual(filePath, gitContent, desiredContent string) bool {
	if gitContent == desiredContent {
		return true
	}
	if !strings.HasPrefix(strings.Trim(strings.TrimSpace(filepath.ToSlash(filePath)), "/"), "knowledge/") {
		return false
	}
	gitDoc, ok := canonicalKnowledgeContextDriftDocument(filePath, gitContent)
	if !ok {
		return false
	}
	desiredDoc, ok := canonicalKnowledgeContextDriftDocument(filePath, desiredContent)
	if !ok {
		return false
	}
	return gitDoc == desiredDoc
}

type configRepositoryKnowledgeDriftDocument struct {
	Name        string
	Kind        string
	Description string
	Content     string
}

func canonicalKnowledgeContextDriftDocument(filePath, content string) (configRepositoryKnowledgeDriftDocument, bool) {
	doc, body, err := parseKnowledgeContextDocument(content)
	if err != nil {
		return configRepositoryKnowledgeDriftDocument{}, false
	}
	pathKind, _, pathName, err := parseKnowledgeContextDriftPath(filePath)
	if err != nil {
		return configRepositoryKnowledgeDriftDocument{}, false
	}
	kind := strings.TrimSpace(doc.Kind)
	if kind == "" {
		kind = pathKind
	}
	name := strings.TrimSpace(doc.Name)
	if name == "" {
		name = pathName
	}
	kind, err = normalizeKnowledgeContextKind(kind)
	if err != nil {
		return configRepositoryKnowledgeDriftDocument{}, false
	}
	name, err = normalizeKnowledgeContextName(name)
	if err != nil {
		return configRepositoryKnowledgeDriftDocument{}, false
	}
	return configRepositoryKnowledgeDriftDocument{
		Name:        name,
		Kind:        kind,
		Description: strings.TrimSpace(doc.Description),
		Content:     strings.TrimSpace(body),
	}, true
}

func parseKnowledgeContextDriftPath(filePath string) (string, string, string, error) {
	rel := strings.Trim(strings.TrimSpace(filepath.ToSlash(filePath)), "/")
	rel = strings.TrimPrefix(rel, "knowledge/")
	parts := strings.Split(rel, "/")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("knowledge document path must use kind/group/document")
	}
	kind, err := normalizeKnowledgeContextKind(parts[0])
	if err != nil {
		return "", "", "", err
	}
	name, err := normalizeKnowledgeContextName(parts[len(parts)-1])
	if err != nil {
		return "", "", "", err
	}
	group, err := normalizeKnowledgeContextGroup(strings.Join(parts[1:len(parts)-1], "/"))
	if err != nil {
		return "", "", "", err
	}
	return kind, group, name, nil
}

func (a *App) exportConfigRepositoryFiles(ctx context.Context, repo models.ConfigRepository) (map[string]string, error) {
	delegatedScopes, err := a.configRepositoryDelegatedScopes(ctx, repo)
	if err != nil {
		return nil, err
	}

	files := map[string]string{}
	if err := a.exportConfigRepositoryPipelines(ctx, repo, delegatedScopes, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositorySteps(ctx, repo, delegatedScopes, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryTriggers(ctx, repo, delegatedScopes, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositorySchedules(ctx, repo, delegatedScopes, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryScopes(ctx, repo, delegatedScopes, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryKnowledge(ctx, repo, delegatedScopes, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryRuntimeSettings(repo, files); err != nil {
		return nil, err
	}
	for filePath, content := range files {
		files[filePath] = normalizeConfigRepositoryFileContent(content)
	}
	return files, nil
}

func (a *App) configRepositoryDelegatedScopes(ctx context.Context, repo models.ConfigRepository) ([]string, error) {
	scopeSet := map[string]struct{}{}
	addScope := func(scope string) {
		scope = strings.Trim(strings.TrimSpace(scope), "/")
		if scope == "" {
			return
		}
		if repo.ScopeType == models.ConfigRepositoryScopeFolder {
			boundScope := strings.Trim(strings.TrimSpace(repo.ScopeID), "/")
			if scope == boundScope || !configResourceUnderScope(scope, boundScope) {
				return
			}
		}
		scopeSet[scope] = struct{}{}
	}

	rows, err := a.db.Query(ctx, `
		SELECT scope_id
		FROM config_repositories
		WHERE scope_type = $1
		  AND enabled = TRUE
		  AND id <> $2
	`, models.ConfigRepositoryScopeFolder, repo.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load delegated config repository scopes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var scopeID string
		if err := rows.Scan(&scopeID); err != nil {
			return nil, err
		}
		addScope(scopeID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	scopes := make([]string, 0, len(scopeSet))
	for scope := range scopeSet {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	return scopes, nil
}

func (a *App) exportConfigRepositoryPipelines(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	rows, err := a.db.Query(ctx, `
		SELECT path, name, definition, COALESCE(source, 'database'), config_repo_id, managed_by_config_repo, config_source_path
		FROM pipelines
		ORDER BY path ASC, name ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var pathPart, name, definition, source, sourcePath string
		var configRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(&pathPart, &name, &definition, &source, &configRepoID, &managed, &sourcePath); err != nil {
			return err
		}
		identifier := buildPipelineIdentifier(pathPart, name)
		if !configRepositoryIncludesResource(repo, identifier, source, configRepoID, managed, delegatedScopes) {
			continue
		}
		filePath, ok := configRepositoryExportPath(repo, identifier, sourcePath, "pipelines", ".yaml", managed, configRepoID)
		if !ok {
			continue
		}
		files[filePath] = definition
	}
	return rows.Err()
}

func (a *App) exportConfigRepositorySteps(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	rows, err := a.db.Query(ctx, `
		SELECT path, name, definition, COALESCE(source, 'database'), config_repo_id, managed_by_config_repo, config_source_path
		FROM steps
		ORDER BY path ASC, name ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var pathPart, name, definition, source, sourcePath string
		var configRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(&pathPart, &name, &definition, &source, &configRepoID, &managed, &sourcePath); err != nil {
			return err
		}
		identifier := buildStepIdentifier(pathPart, name)
		if !configRepositoryIncludesResource(repo, identifier, source, configRepoID, managed, delegatedScopes) {
			continue
		}
		filePath, ok := configRepositoryExportPath(repo, identifier, sourcePath, "steps", ".yaml", managed, configRepoID)
		if !ok {
			continue
		}
		files[filePath] = definition
	}
	return rows.Err()
}

func (a *App) exportConfigRepositoryTriggers(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	rows, err := a.db.Query(ctx, `
		SELECT repository_name, trigger_definition, COALESCE(source, 'database'), config_repo_id, managed_by_config_repo, config_source_path
		FROM triggers
		ORDER BY repository_name ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var identifier, definition, source, sourcePath string
		var configRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(&identifier, &definition, &source, &configRepoID, &managed, &sourcePath); err != nil {
			return err
		}
		if !configRepositoryIncludesResource(repo, identifier, source, configRepoID, managed, delegatedScopes) {
			continue
		}
		filePath, ok := configRepositoryExportPath(repo, identifier, sourcePath, "triggers", ".yaml", managed, configRepoID)
		if !ok {
			continue
		}
		files[filePath] = definition
	}
	return rows.Err()
}

type configRepositoryScheduleDocument struct {
	Name           string            `yaml:"name"`
	Description    string            `yaml:"description,omitempty"`
	Pipeline       string            `yaml:"pipeline"`
	ScheduleKind   string            `yaml:"schedule_kind,omitempty"`
	CronExpression string            `yaml:"cron_expression,omitempty"`
	RunAt          string            `yaml:"run_at,omitempty"`
	Timezone       string            `yaml:"timezone,omitempty"`
	Enabled        bool              `yaml:"enabled"`
	Scope          string            `yaml:"scope,omitempty"`
	Variables      map[string]string `yaml:"variables,omitempty"`
}

func (a *App) exportConfigRepositorySchedules(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	rows, err := a.db.Query(ctx, `
		SELECT path, name, description, pipeline_path, pipeline_name,
		       COALESCE(schedule_kind, 'cron'), cron_expression, run_at, timezone, enabled, scope, variables::text,
		       COALESCE(source, 'database'), config_repo_id, managed_by_config_repo, config_source_path
		FROM pipeline_schedules
		ORDER BY path ASC, name ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var pathPart, name, description, pipelinePath, pipelineName, scheduleKind, cronExpression, timezone, scope, variablesRaw, source, sourcePath string
		var runAt sql.NullTime
		var enabled bool
		var configRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(&pathPart, &name, &description, &pipelinePath, &pipelineName, &scheduleKind, &cronExpression, &runAt, &timezone, &enabled, &scope, &variablesRaw, &source, &configRepoID, &managed, &sourcePath); err != nil {
			return err
		}
		identifier := buildPipelineIdentifier(pathPart, name)
		if !configRepositoryIncludesResource(repo, identifier, source, configRepoID, managed, delegatedScopes) {
			continue
		}
		filePath, ok := configRepositoryExportPath(repo, identifier, sourcePath, "schedules", ".yaml", managed, configRepoID)
		if !ok {
			continue
		}
		var variables map[string]string
		if strings.TrimSpace(variablesRaw) != "" {
			_ = json.Unmarshal([]byte(variablesRaw), &variables)
		}
		if len(variables) == 0 {
			variables = nil
		}
		doc := configRepositoryScheduleDocument{
			Name:        name,
			Description: strings.TrimSpace(description),
			Pipeline:    buildPipelineIdentifier(pipelinePath, pipelineName),
			Timezone:    timezone,
			Enabled:     enabled,
			Scope:       scope,
			Variables:   variables,
		}
		if normalizeScheduleKindValue(scheduleKind) == scheduleKindOnce {
			doc.ScheduleKind = scheduleKindOnce
			if runAt.Valid {
				doc.RunAt = runAt.Time.UTC().Format(time.RFC3339)
			}
		} else {
			doc.CronExpression = cronExpression
		}
		content, err := yaml.Marshal(doc)
		if err != nil {
			return err
		}
		files[filePath] = string(content)
	}
	return rows.Err()
}

type configRepositoryScopeExport struct {
	Variables map[string]string
	Secrets   map[string]any
}

type configRepositoryScopeDocument struct {
	Variables map[string]string `yaml:"variables,omitempty"`
	Secrets   map[string]any    `yaml:"secrets,omitempty"`
}

func (a *App) exportConfigRepositoryScopes(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	scopeFiles := map[string]*configRepositoryScopeExport{}
	addFile := func(filePath string) *configRepositoryScopeExport {
		if _, ok := scopeFiles[filePath]; !ok {
			scopeFiles[filePath] = &configRepositoryScopeExport{
				Variables: map[string]string{},
				Secrets:   map[string]any{},
			}
		}
		return scopeFiles[filePath]
	}

	if err := a.exportConfigRepositoryScopeVariables(ctx, repo, delegatedScopes, addFile); err != nil {
		return err
	}
	if err := a.exportConfigRepositoryScopeSecrets(ctx, repo, delegatedScopes, addFile); err != nil {
		return err
	}

	for filePath, payload := range scopeFiles {
		doc := configRepositoryScopeDocument{}
		if len(payload.Variables) > 0 {
			doc.Variables = payload.Variables
		}
		if len(payload.Secrets) > 0 {
			doc.Secrets = payload.Secrets
		}
		content, err := yaml.Marshal(doc)
		if err != nil {
			return err
		}
		files[filePath] = string(content)
	}
	return nil
}

func (a *App) exportConfigRepositoryScopeVariables(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, addFile func(string) *configRepositoryScopeExport) error {
	rows, err := a.db.Query(ctx, `
		SELECT name, value, COALESCE(repository_name, ''), scope, COALESCE(source, 'database'), config_repo_id, managed_by_config_repo, config_source_path
		FROM variables
		ORDER BY scope ASC, repository_name ASC NULLS FIRST, name ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name, value, repositoryName, scope, source, sourcePath string
		var configRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(&name, &value, &repositoryName, &scope, &source, &configRepoID, &managed, &sourcePath); err != nil {
			return err
		}
		if !configRepositoryIncludesResource(repo, scope, source, configRepoID, managed, delegatedScopes) {
			continue
		}
		filePath, ok := configRepositoryScopeFilePath(repo, scope, sourcePath, managed, configRepoID)
		if !ok {
			continue
		}
		key := strings.TrimSpace(name)
		if repositoryName != "" {
			key = strings.Trim(strings.TrimSpace(repositoryName)+"/"+key, "/")
		}
		addFile(filePath).Variables[key] = value
	}
	return rows.Err()
}

func (a *App) exportConfigRepositoryScopeSecrets(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, addFile func(string) *configRepositoryScopeExport) error {
	rows, err := a.db.Query(ctx, `
		SELECT name, value, COALESCE(repository_name, ''), scope, COALESCE(source, 'database'), config_repo_id, managed_by_config_repo, config_source_path
		FROM secrets
		ORDER BY scope ASC, repository_name ASC NULLS FIRST, name ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name, repositoryName, scope, source, sourcePath string
		var value sql.NullString
		var configRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(&name, &value, &repositoryName, &scope, &source, &configRepoID, &managed, &sourcePath); err != nil {
			return err
		}
		if !configRepositoryIncludesResource(repo, scope, source, configRepoID, managed, delegatedScopes) {
			continue
		}
		filePath, ok := configRepositoryScopeFilePath(repo, scope, sourcePath, managed, configRepoID)
		if !ok {
			continue
		}
		key := strings.TrimSpace(name)
		if repositoryName != "" {
			key = strings.Trim(strings.TrimSpace(repositoryName)+"/"+key, "/")
		}
		if value.Valid && strings.TrimSpace(value.String) != "" {
			addFile(filePath).Secrets[key] = value.String
		} else {
			addFile(filePath).Secrets[key] = nil
		}
	}
	return rows.Err()
}

func (a *App) exportConfigRepositoryKnowledge(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	rows, err := a.db.Query(ctx, `
		SELECT kind, group_path, name, description, content, COALESCE(source, 'database'), config_repo_id, managed_by_config_repo, config_source_path
		FROM knowledge_contexts
		ORDER BY kind ASC, group_path ASC, name ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var kind, groupPath, name, description, content, source, sourcePath string
		var configRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(&kind, &groupPath, &name, &description, &content, &source, &configRepoID, &managed, &sourcePath); err != nil {
			return err
		}
		identifier := buildKnowledgeContextIdentifier(kind, groupPath, name)
		if !configRepositoryIncludesResource(repo, groupPath, source, configRepoID, managed, delegatedScopes) {
			continue
		}
		filePath := strings.TrimSpace(sourcePath)
		if managed && configRepoID.Valid && configRepoID.Int64 == repo.ID && filePath != "" {
			var ok bool
			filePath, ok = configRepositoryManagedSourcePath(repo, filePath)
			if !ok {
				continue
			}
		} else {
			relGroup, ok := configRepositoryRelativeResourceIdentifier(repo, groupPath)
			if !ok {
				continue
			}
			relID := strings.Trim(strings.Trim(relGroup, "/")+"/"+strings.Trim(name, "/"), "/")
			if relID == "" {
				continue
			}
			filePath = filepath.ToSlash(filepath.Join("knowledge", kind, relID+".md"))
		}
		if !isConfigRepositoryDriftPath(filePath) {
			continue
		}
		files[filePath] = renderKnowledgeContextGitOpsDocument(kind, name, description, content, identifier)
	}
	return rows.Err()
}

func (a *App) exportConfigRepositoryRuntimeSettings(repo models.ConfigRepository, files map[string]string) error {
	if repo.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil
	}
	doc := buildRuntimeSettingsGitOpsFile(a.getConfigSnapshot())
	content, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	files["setting/system/runner.yaml"] = string(content)
	return nil
}

type configRepositoryKnowledgeDocument struct {
	Name        string `yaml:"name"`
	Kind        string `yaml:"kind"`
	Description string `yaml:"description,omitempty"`
	Content     string `yaml:"content"`
}

func renderKnowledgeContextGitOpsDocument(kind, name, description, content, fallbackName string) string {
	doc := configRepositoryKnowledgeDocument{
		Name:        name,
		Kind:        kind,
		Description: strings.TrimSpace(description),
		Content:     content,
	}
	if strings.TrimSpace(doc.Name) == "" {
		doc.Name = fallbackName
	}
	encoded, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Sprintf("---\nname: %s\nkind: %s\ncontent: |\n%s\n---\n", name, kind, indentConfigRepositoryBlock(content))
	}
	return "---\n" + string(encoded) + "---\n"
}

func indentConfigRepositoryBlock(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = "  " + line
	}
	return strings.Join(lines, "\n")
}

func configRepositoryIncludesResource(repo models.ConfigRepository, identifier, source string, configRepoID sql.NullInt64, managed bool, delegatedScopes []string) bool {
	if configResourceUnderAnyScope(identifier, delegatedScopes) {
		return false
	}
	if managed && configRepoID.Valid {
		return configRepoID.Int64 == repo.ID
	}
	if !strings.EqualFold(strings.TrimSpace(source), "database") {
		return false
	}
	if repo.ScopeType == models.ConfigRepositoryScopeFolder {
		_, ok := configRepositoryRelativeResourceIdentifier(repo, identifier)
		return ok
	}
	return repo.ScopeType == models.ConfigRepositoryScopeSystem
}

func configRepositoryExportPath(repo models.ConfigRepository, identifier, sourcePath, directory, extension string, managed bool, configRepoID sql.NullInt64) (string, bool) {
	if managed && configRepoID.Valid && configRepoID.Int64 == repo.ID && strings.TrimSpace(sourcePath) != "" {
		return configRepositoryManagedSourcePath(repo, sourcePath)
	}
	relID, ok := configRepositoryRelativeResourceIdentifier(repo, identifier)
	if !ok || relID == "" {
		return "", false
	}
	return filepath.ToSlash(filepath.Join(directory, relID+extension)), true
}

func configRepositoryScopeFilePath(repo models.ConfigRepository, scope, sourcePath string, managed bool, configRepoID sql.NullInt64) (string, bool) {
	if managed && configRepoID.Valid && configRepoID.Int64 == repo.ID && strings.TrimSpace(sourcePath) != "" {
		return configRepositoryManagedSourcePath(repo, sourcePath)
	}
	relScope, ok := configRepositoryRelativeResourceIdentifier(repo, runtimeScopeForDisplay(scope))
	if !ok {
		return "", false
	}
	if relScope == "" || relScope == defaultRuntimeScope {
		relScope = "default"
	}
	return filepath.ToSlash(filepath.Join("scopes", relScope, "scope.yaml")), true
}

func configRepositoryManagedSourcePath(repo models.ConfigRepository, sourcePath string) (string, bool) {
	cleaned := strings.Trim(strings.TrimSpace(filepath.ToSlash(sourcePath)), "/")
	if cleaned == "" {
		return "", false
	}
	if rel, ok := configRepositoryRelativeGitPath(repo.BasePath, cleaned); ok && isConfigRepositoryDriftPath(rel) {
		return rel, true
	}
	if isConfigRepositoryDriftPath(cleaned) {
		return cleaned, true
	}
	return "", false
}

func configRepositoryRelativeResourceIdentifier(repo models.ConfigRepository, identifier string) (string, bool) {
	identifier = strings.Trim(strings.TrimSpace(strings.ReplaceAll(identifier, "\\", "/")), "/")
	if repo.ScopeType != models.ConfigRepositoryScopeFolder {
		return identifier, identifier != ""
	}
	scopeID := strings.Trim(strings.TrimSpace(repo.ScopeID), "/")
	if scopeID == "" {
		return "", false
	}
	if identifier == scopeID {
		return "", true
	}
	prefix := scopeID + "/"
	if strings.HasPrefix(identifier, prefix) {
		return strings.TrimPrefix(identifier, prefix), true
	}
	return "", false
}

func configRepositoryRelativeGitPath(basePath, filePath string) (string, bool) {
	basePath = strings.Trim(strings.TrimSpace(filepath.ToSlash(basePath)), "/")
	filePath = strings.Trim(strings.TrimSpace(filepath.ToSlash(filePath)), "/")
	if filePath == "" {
		return "", false
	}
	if basePath == "" {
		return filePath, true
	}
	if filePath == basePath {
		return "", false
	}
	prefix := basePath + "/"
	if strings.HasPrefix(filePath, prefix) {
		return strings.TrimPrefix(filePath, prefix), true
	}
	return "", false
}

func isConfigRepositoryDriftPath(filePath string) bool {
	filePath = strings.Trim(strings.TrimSpace(filepath.ToSlash(filePath)), "/")
	for _, prefix := range []string{"pipelines/", "steps/", "triggers/", "schedules/", "scopes/", "knowledge/"} {
		if strings.HasPrefix(filePath, prefix) {
			return true
		}
	}
	if rel, ok := strings.CutPrefix(filePath, "settings/"); ok {
		return isConfigRepositorySettingsDriftPath(rel)
	}
	if rel, ok := strings.CutPrefix(filePath, "setting/"); ok {
		return isConfigRepositorySettingsDriftPath(rel)
	}
	return false
}

func isConfigRepositorySettingsDriftPath(rel string) bool {
	return isGitOpsRuntimeSettingsRelativePath(rel)
}

func normalizeConfigRepositoryFileContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimRight(content, "\n") + "\n"
	return content
}

func defaultConfigRepositoryPushMessage(repo models.ConfigRepository) string {
	scope := strings.Trim(repo.ScopeType+"/"+repo.ScopeID, "/")
	if scope == "" {
		scope = "config"
	}
	return "Update Nopsai config for " + scope
}

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/auth"

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

type configRepositoryResourceAccessState struct {
	Visibility string
	Grants     []configRepositoryResourceUseGrant
	Override   bool
}

type configRepositoryResourceUseGrant struct {
	ResourceType string
	SubjectType  string
	SubjectID    string
	Actions      []string
}

type configRepositoryEmbeddedAccessFile struct {
	Visibility string                                 `yaml:"visibility,omitempty"`
	UseAccess  *configRepositoryEmbeddedUseAccessFile `yaml:"use_access,omitempty"`
}

type configRepositoryEmbeddedUseAccessFile struct {
	Grants []configRepositoryEmbeddedUseGrantFile `yaml:"grants,omitempty"`
}

type configRepositoryEmbeddedUseGrantFile struct {
	SubjectType    string   `yaml:"subject_type,omitempty"`
	SubjectID      string   `yaml:"subject_id,omitempty"`
	Group          string   `yaml:"group,omitempty"`
	Repository     string   `yaml:"repository,omitempty"`
	User           string   `yaml:"user,omitempty"`
	Trigger        string   `yaml:"trigger,omitempty"`
	ServiceAccount string   `yaml:"service_account,omitempty"`
	Service        string   `yaml:"service,omitempty"`
	Actions        []string `yaml:"actions,omitempty"`
}

func marshalConfigRepositoryYAML(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(value); err != nil {
		_ = encoder.Close()
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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
	for _, directory := range []string{"pipelines", "steps", "triggers", externalTriggersGitOpsDirectory, "schedules", "scopes", "knowledge", "pipelineruns", "config-repositories", "access", "setting", "settings"} {
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
	Access      string
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
		Access:      canonicalConfigRepositoryAccessString(doc.Access),
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

	resourceAccess, err := a.configRepositoryResourceAccess(ctx, repo, delegatedScopes)
	if err != nil {
		return nil, err
	}

	files := map[string]string{}
	if err := a.exportConfigRepositoryPipelines(ctx, repo, delegatedScopes, resourceAccess, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositorySteps(ctx, repo, delegatedScopes, resourceAccess, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryTriggers(ctx, repo, delegatedScopes, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryExternalTriggers(ctx, repo, delegatedScopes, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositorySchedules(ctx, repo, delegatedScopes, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryScopes(ctx, repo, delegatedScopes, resourceAccess, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryKnowledge(ctx, repo, delegatedScopes, resourceAccess, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryGroupStructure(ctx, repo, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryAccess(ctx, repo, delegatedScopes, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryLLMProfiles(ctx, repo, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryMCPRegistry(ctx, repo, files); err != nil {
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

func (a *App) configRepositoryResourceAccess(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string) (map[resourceAccessPlanKey]configRepositoryResourceAccessState, error) {
	states := map[resourceAccessPlanKey]configRepositoryResourceAccessState{}
	include := func(resourceType, resourceID string) bool {
		return configRepositoryIncludesAccessResource(repo, resourceType, resourceID, delegatedScopes)
	}
	setState := func(key resourceAccessPlanKey, update func(*configRepositoryResourceAccessState)) {
		state := states[key]
		if strings.TrimSpace(state.Visibility) == "" {
			state.Visibility = resourceVisibilityGroup
		}
		update(&state)
		states[key] = state
	}

	rows, err := a.db.Query(ctx, `
		SELECT resource_type, resource_id
		FROM resource_access_overrides
		ORDER BY resource_type ASC, resource_id ASC
	`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var resourceType, resourceID string
		if err := rows.Scan(&resourceType, &resourceID); err != nil {
			rows.Close()
			return nil, err
		}
		key, ok := configRepositoryAccessResourceKey(resourceType, resourceID)
		if !ok || !include(key.resourceType, key.resourceID) {
			continue
		}
		setState(key, func(state *configRepositoryResourceAccessState) {
			state.Override = true
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if err := a.addConfigRepositoryTableVisibilities(ctx, grantResourcePipeline, "pipelines", include, setState); err != nil {
		return nil, err
	}
	if err := a.addConfigRepositoryTableVisibilities(ctx, grantResourceStep, "steps", include, setState); err != nil {
		return nil, err
	}
	if err := a.addConfigRepositoryResourceVisibilityRows(ctx, include, setState); err != nil {
		return nil, err
	}
	if err := a.addConfigRepositoryResourceUseGrants(ctx, include, setState); err != nil {
		return nil, err
	}

	for key, state := range states {
		state.Visibility = normalizeResourceVisibility(state.Visibility)
		if !state.Override && len(state.Grants) == 0 && state.Visibility == resourceVisibilityGroup {
			delete(states, key)
			continue
		}
		sort.Slice(state.Grants, func(i, j int) bool {
			if state.Grants[i].SubjectType != state.Grants[j].SubjectType {
				return state.Grants[i].SubjectType < state.Grants[j].SubjectType
			}
			return state.Grants[i].SubjectID < state.Grants[j].SubjectID
		})
		states[key] = state
	}
	return states, nil
}

func (a *App) addConfigRepositoryTableVisibilities(ctx context.Context, resourceType, tableName string, include func(string, string) bool, setState func(resourceAccessPlanKey, func(*configRepositoryResourceAccessState))) error {
	rows, err := a.db.Query(ctx, fmt.Sprintf(`
		SELECT path, name, visibility
		FROM %s
		ORDER BY path ASC, name ASC
	`, tableName))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var pathPart, name, visibility string
		if err := rows.Scan(&pathPart, &name, &visibility); err != nil {
			return err
		}
		resourceID := buildPipelineIdentifier(pathPart, name)
		key, ok := configRepositoryAccessResourceKey(resourceType, resourceID)
		if !ok || !include(key.resourceType, key.resourceID) {
			continue
		}
		visibility = normalizeResourceVisibility(visibility)
		setState(key, func(state *configRepositoryResourceAccessState) {
			state.Visibility = visibility
		})
	}
	return rows.Err()
}

func (a *App) addConfigRepositoryResourceVisibilityRows(ctx context.Context, include func(string, string) bool, setState func(resourceAccessPlanKey, func(*configRepositoryResourceAccessState))) error {
	rows, err := a.db.Query(ctx, `
		SELECT resource_type, resource_id, visibility
		FROM resource_visibility
		WHERE resource_type = ANY($1)
		ORDER BY resource_type ASC, resource_id ASC
	`, []string{grantResourceScope, grantResourceKnowledgeContext})
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var resourceType, resourceID, visibility string
		if err := rows.Scan(&resourceType, &resourceID, &visibility); err != nil {
			return err
		}
		key, ok := configRepositoryAccessResourceKey(resourceType, resourceID)
		if !ok || !include(key.resourceType, key.resourceID) {
			continue
		}
		visibility = normalizeResourceVisibility(visibility)
		setState(key, func(state *configRepositoryResourceAccessState) {
			state.Visibility = visibility
		})
	}
	return rows.Err()
}

func (a *App) addConfigRepositoryResourceUseGrants(ctx context.Context, include func(string, string) bool, setState func(resourceAccessPlanKey, func(*configRepositoryResourceAccessState))) error {
	rows, err := a.db.Query(ctx, `
		SELECT ag.id, ag.subject_type, ag.subject_id, ag.resource_type, ag.resource_id, COALESCE(ra.action, '')
		FROM access_grants ag
		LEFT JOIN resource_acl ra ON ra.access_grant_id = ag.id AND ra.effect = 'allow'
		WHERE ag.role_name = $1
		  AND ag.resource_type = ANY($2)
		ORDER BY ag.resource_type ASC, ag.resource_id ASC, ag.subject_type ASC, ag.subject_id ASC, ag.id ASC, ra.action ASC
	`, customUseGrantRole, []string{grantResourcePipeline, grantResourceStep, grantResourceScope, grantResourceKnowledgeContext})
	if err != nil {
		return err
	}
	defer rows.Close()

	var currentID int64
	var currentKey resourceAccessPlanKey
	var currentGrant configRepositoryResourceUseGrant
	var currentActions []string
	flush := func() error {
		if currentID == 0 {
			return nil
		}
		if !include(currentKey.resourceType, currentKey.resourceID) {
			return nil
		}
		actions := uniqueSortedStrings(currentActions)
		if len(actions) == 0 {
			action, err := defaultUseActionForResource(currentKey.resourceType)
			if err != nil {
				return err
			}
			actions = []string{action}
		}
		currentGrant.Actions = actions
		setState(currentKey, func(state *configRepositoryResourceAccessState) {
			state.Grants = append(state.Grants, currentGrant)
		})
		return nil
	}

	for rows.Next() {
		var grantID int64
		var subjectType, subjectID, resourceType, resourceID, action string
		if err := rows.Scan(&grantID, &subjectType, &subjectID, &resourceType, &resourceID, &action); err != nil {
			return err
		}
		key, ok := configRepositoryAccessResourceKey(resourceType, resourceID)
		if !ok {
			continue
		}
		if grantID != currentID {
			if err := flush(); err != nil {
				return err
			}
			currentID = grantID
			currentKey = key
			currentGrant = configRepositoryResourceUseGrant{
				ResourceType: currentKey.resourceType,
				SubjectType:  strings.TrimSpace(subjectType),
				SubjectID:    strings.Trim(strings.TrimSpace(subjectID), "/"),
			}
			currentActions = nil
		}
		if strings.TrimSpace(action) != "" {
			currentActions = append(currentActions, strings.TrimSpace(action))
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return flush()
}

func configRepositoryIncludesAccessResource(repo models.ConfigRepository, resourceType, resourceID string, delegatedScopes []string) bool {
	if !isConfigRepositoryEmbeddedAccessResourceType(resourceType) {
		return false
	}
	if accessGrantResourceIntersectsAnyScope(resourceType, resourceID, delegatedScopes) {
		return false
	}
	return accessGrantResourceInConfigBindingScope(resourceType, resourceID, repo)
}

func configRepositoryIncludesBasicRoleGrant(repo models.ConfigRepository, resourceType, resourceID string, delegatedScopes []string) bool {
	if repo.ScopeType == models.ConfigRepositoryScopeFolder &&
		accessGrantResourceIntersectsAnyScope(resourceType, resourceID, delegatedScopes) {
		return false
	}
	return accessGrantResourceInConfigBindingScope(resourceType, resourceID, repo)
}

func isConfigRepositoryEmbeddedAccessResourceType(resourceType string) bool {
	switch strings.TrimSpace(resourceType) {
	case grantResourcePipeline, grantResourceStep, grantResourceScope, grantResourceKnowledgeContext:
		return true
	default:
		return false
	}
}

func configRepositoryAccessResourceKey(resourceType, resourceID string) (resourceAccessPlanKey, bool) {
	resourceType = strings.TrimSpace(resourceType)
	resourceID = strings.Trim(strings.TrimSpace(filepath.ToSlash(resourceID)), "/")
	if resourceType == grantResourceScope {
		resourceID = runtimeScopeForResource(resourceID)
	}
	if !isConfigRepositoryEmbeddedAccessResourceType(resourceType) {
		return resourceAccessPlanKey{}, false
	}
	if resourceType != grantResourceScope && resourceID == "" {
		return resourceAccessPlanKey{}, false
	}
	return resourceAccessPlanKey{resourceType: resourceType, resourceID: resourceID}, true
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func (state configRepositoryResourceAccessState) exportFile() *configRepositoryEmbeddedAccessFile {
	visibility := normalizeResourceVisibility(state.Visibility)
	grants := make([]configRepositoryEmbeddedUseGrantFile, 0, len(state.Grants))
	for _, grant := range state.Grants {
		subjectID := strings.Trim(strings.TrimSpace(grant.SubjectID), "/")
		if subjectID == "" {
			continue
		}
		exportGrant := configRepositoryEmbeddedUseGrantFile{}
		switch strings.TrimSpace(grant.SubjectType) {
		case grantSubjectGroup:
			exportGrant.Group = subjectID
		case grantSubjectRepository:
			exportGrant.Repository = subjectID
		case grantSubjectUser:
			exportGrant.User = subjectID
		case grantSubjectTrigger:
			exportGrant.Trigger = subjectID
		case grantSubjectServiceAccount:
			exportGrant.ServiceAccount = subjectID
		case grantSubjectService, "internal_service":
			exportGrant.Service = subjectID
		default:
			exportGrant.SubjectType = strings.TrimSpace(grant.SubjectType)
			exportGrant.SubjectID = subjectID
		}
		actions := uniqueSortedStrings(grant.Actions)
		resourceType := strings.TrimSpace(grant.ResourceType)
		if resourceType == "" {
			resourceType = resourceTypeForUseActions(actions)
		}
		if defaultAction, err := defaultUseActionForResource(resourceType); err != nil || len(actions) != 1 || actions[0] != defaultAction {
			exportGrant.Actions = actions
		}
		grants = append(grants, exportGrant)
	}
	if visibility == resourceVisibilityGroup && len(grants) == 0 {
		return nil
	}
	file := &configRepositoryEmbeddedAccessFile{Visibility: visibility}
	if len(grants) > 0 {
		file.UseAccess = &configRepositoryEmbeddedUseAccessFile{Grants: grants}
	}
	return file
}

func resourceTypeForUseActions(actions []string) string {
	for _, action := range actions {
		switch {
		case strings.HasPrefix(action, "pipeline."):
			return grantResourcePipeline
		case strings.HasPrefix(action, "step."):
			return grantResourceStep
		case strings.HasPrefix(action, "scope."):
			return grantResourceScope
		case strings.HasPrefix(action, "knowledge_context."):
			return grantResourceKnowledgeContext
		}
	}
	return ""
}

func syncConfigRepositoryYAMLAccessBlock(content string, access *configRepositoryEmbeddedAccessFile) (string, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return "", err
	}
	if len(root.Content) == 0 {
		root.Kind = yaml.DocumentNode
		root.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return "", fmt.Errorf("document must be a YAML mapping")
	}
	removeTopLevelYAMLKey(doc, "access")
	if access != nil {
		var accessRoot yaml.Node
		encoded, err := marshalConfigRepositoryYAML(access)
		if err != nil {
			return "", err
		}
		if err := yaml.Unmarshal(encoded, &accessRoot); err != nil {
			return "", err
		}
		if len(accessRoot.Content) > 0 {
			doc.Content = append(doc.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "access"},
				accessRoot.Content[0],
			)
		}
	}
	encoded, err := marshalConfigRepositoryYAML(&root)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func removeTopLevelYAMLKey(doc *yaml.Node, key string) {
	if doc == nil || doc.Kind != yaml.MappingNode {
		return
	}
	next := doc.Content[:0]
	for i := 0; i+1 < len(doc.Content); i += 2 {
		keyNode := doc.Content[i]
		valueNode := doc.Content[i+1]
		if keyNode != nil && keyNode.Kind == yaml.ScalarNode && keyNode.Value == key {
			continue
		}
		next = append(next, keyNode, valueNode)
	}
	doc.Content = next
}

func canonicalConfigRepositoryAccessString(access *embeddedResourceAccessFile) string {
	if access == nil {
		return ""
	}
	rawGrants := embeddedResourceAccessGrants(*access)
	visibility := firstNonEmptyString(access.Visibility, embeddedResourceUseAccessMode(access.UseAccess))
	if visibility == "" && len(rawGrants) > 0 {
		visibility = resourceVisibilityRestricted
	}
	if visibility == "" {
		visibility = resourceVisibilityGroup
	}
	normalizedVisibility, err := normalizeResourceVisibilityUpdate(visibility)
	if err != nil {
		return ""
	}
	state := configRepositoryResourceAccessState{Visibility: normalizedVisibility}
	for _, rawGrant := range rawGrants {
		subjectType, subjectID, err := normalizeEmbeddedResourceUseGrantSubject(rawGrant)
		if err != nil {
			return ""
		}
		actions, err := normalizeUseGrantActions(grantResourceKnowledgeContext, rawGrant.Actions.values())
		if err != nil {
			return ""
		}
		state.Grants = append(state.Grants, configRepositoryResourceUseGrant{
			ResourceType: grantResourceKnowledgeContext,
			SubjectType:  subjectType,
			SubjectID:    subjectID,
			Actions:      actions,
		})
	}
	sort.Slice(state.Grants, func(i, j int) bool {
		if state.Grants[i].SubjectType != state.Grants[j].SubjectType {
			return state.Grants[i].SubjectType < state.Grants[j].SubjectType
		}
		return state.Grants[i].SubjectID < state.Grants[j].SubjectID
	})
	export := state.exportFile()
	if export == nil {
		return ""
	}
	encoded, err := marshalConfigRepositoryYAML(export)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(encoded))
}

func (a *App) exportConfigRepositoryPipelines(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, resourceAccess map[resourceAccessPlanKey]configRepositoryResourceAccessState, files map[string]string) error {
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
		if access, ok := resourceAccess[resourceAccessPlanKey{resourceType: grantResourcePipeline, resourceID: identifier}]; ok && (access.Override || !managed) {
			definition, err = syncConfigRepositoryYAMLAccessBlock(definition, access.exportFile())
			if err != nil {
				return fmt.Errorf("failed to render pipeline access for %s: %w", identifier, err)
			}
		}
		files[filePath] = definition
	}
	return rows.Err()
}

func (a *App) exportConfigRepositorySteps(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, resourceAccess map[resourceAccessPlanKey]configRepositoryResourceAccessState, files map[string]string) error {
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
		if access, ok := resourceAccess[resourceAccessPlanKey{resourceType: grantResourceStep, resourceID: identifier}]; ok && (access.Override || !managed) {
			definition, err = syncConfigRepositoryYAMLAccessBlock(definition, access.exportFile())
			if err != nil {
				return fmt.Errorf("failed to render step access for %s: %w", identifier, err)
			}
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

func (a *App) exportConfigRepositoryExternalTriggers(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	rows, err := a.db.Query(ctx, `
		SELECT id, name, description, enabled, pipeline, scope, allowed_callers, variable_mapping,
		       payload_schema, rate_limit, COALESCE(source, 'database'), config_repo_id,
		       managed_by_config_repo, COALESCE(config_source_path, '')
		FROM external_triggers
		ORDER BY id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var trigger externalTriggerRecord
		var allowedJSON, mappingJSON, schemaJSON, rateLimitJSON []byte
		var source, sourcePath string
		var configRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(
			&trigger.ID,
			&trigger.Name,
			&trigger.Description,
			&trigger.Enabled,
			&trigger.Pipeline,
			&trigger.Scope,
			&allowedJSON,
			&mappingJSON,
			&schemaJSON,
			&rateLimitJSON,
			&source,
			&configRepoID,
			&managed,
			&sourcePath,
		); err != nil {
			return err
		}
		_ = decodeJSONWithDefault(allowedJSON, &trigger.AllowedCallers, []externalTriggerAllowedCaller{})
		_ = decodeJSONWithDefault(mappingJSON, &trigger.VariableMapping, map[string]string{})
		_ = decodeJSONWithDefault(schemaJSON, &trigger.PayloadSchema, map[string]any{})
		_ = decodeJSONWithDefault(rateLimitJSON, &trigger.RateLimit, map[string]any{})
		if !configRepositoryIncludesExternalTrigger(repo, trigger, source, configRepoID, managed, delegatedScopes) {
			continue
		}
		filePath, ok := externalTriggerExportPath(repo, trigger, sourcePath, managed, configRepoID.Valid, configRepoID.Int64)
		if !ok {
			continue
		}
		enabled := trigger.Enabled
		doc := externalTriggerGitOpsDocument{
			ID:              trigger.ID,
			Name:            trigger.Name,
			Description:     strings.TrimSpace(trigger.Description),
			Enabled:         &enabled,
			Pipeline:        trigger.Pipeline,
			Scope:           trigger.Scope,
			AllowedCallers:  trigger.AllowedCallers,
			VariableMapping: nilIfEmptyStringMap(trigger.VariableMapping),
			PayloadSchema:   nilIfEmptyAnyMap(trigger.PayloadSchema),
			RateLimit:       nilIfEmptyAnyMap(trigger.RateLimit),
		}
		content, err := marshalConfigRepositoryYAML(doc)
		if err != nil {
			return err
		}
		files[filePath] = string(content)
	}
	return rows.Err()
}

func configRepositoryIncludesExternalTrigger(repo models.ConfigRepository, trigger externalTriggerRecord, source string, configRepoID sql.NullInt64, managed bool, delegatedScopes []string) bool {
	resourceScope := externalTriggerConfigScope(trigger)
	if configResourceUnderAnyScope(resourceScope, delegatedScopes) {
		return false
	}
	if managed && configRepoID.Valid {
		return configRepoID.Int64 == repo.ID
	}
	if !strings.EqualFold(strings.TrimSpace(source), "database") {
		return false
	}
	return configResourceUnderBindingScope(resourceScope, repo)
}

func nilIfEmptyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	return values
}

func nilIfEmptyAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	return values
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
		content, err := marshalConfigRepositoryYAML(doc)
		if err != nil {
			return err
		}
		files[filePath] = string(content)
	}
	return rows.Err()
}

type configRepositoryScopeExport struct {
	Access    *configRepositoryEmbeddedAccessFile
	Variables map[string]string
	Secrets   map[string]any
}

type configRepositoryScopeDocument struct {
	Access    *configRepositoryEmbeddedAccessFile `yaml:"access,omitempty"`
	Variables map[string]string                   `yaml:"variables,omitempty"`
	Secrets   map[string]any                      `yaml:"secrets,omitempty"`
}

func (a *App) exportConfigRepositoryScopes(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, resourceAccess map[resourceAccessPlanKey]configRepositoryResourceAccessState, files map[string]string) error {
	scopeFiles := map[string]*configRepositoryScopeExport{}
	scopeAccessAdded := map[resourceAccessPlanKey]struct{}{}
	addFile := func(filePath string) *configRepositoryScopeExport {
		if _, ok := scopeFiles[filePath]; !ok {
			scopeFiles[filePath] = &configRepositoryScopeExport{
				Variables: map[string]string{},
				Secrets:   map[string]any{},
			}
		}
		return scopeFiles[filePath]
	}
	addScopedResourceAccess := func(scope string, sourcePath string, managed bool, configRepoID sql.NullInt64) {
		key := resourceAccessPlanKey{resourceType: grantResourceScope, resourceID: runtimeScopeForResource(scope)}
		access, ok := resourceAccess[key]
		if !ok {
			return
		}
		filePath, ok := configRepositoryScopeFilePath(repo, scope, sourcePath, managed, configRepoID)
		if !ok {
			return
		}
		addFile(filePath).Access = access.exportFile()
		scopeAccessAdded[key] = struct{}{}
	}

	if err := a.exportConfigRepositoryScopeVariables(ctx, repo, delegatedScopes, addFile, addScopedResourceAccess); err != nil {
		return err
	}
	if err := a.exportConfigRepositoryScopeSecrets(ctx, repo, delegatedScopes, addFile, addScopedResourceAccess); err != nil {
		return err
	}
	for key := range resourceAccess {
		if key.resourceType != grantResourceScope {
			continue
		}
		if _, ok := scopeAccessAdded[key]; ok {
			continue
		}
		addScopedResourceAccess(runtimeScopeForDisplay(key.resourceID), "", false, sql.NullInt64{})
	}

	for filePath, payload := range scopeFiles {
		doc := configRepositoryScopeDocument{}
		if payload.Access != nil {
			doc.Access = payload.Access
		}
		if len(payload.Variables) > 0 {
			doc.Variables = payload.Variables
		}
		if len(payload.Secrets) > 0 {
			doc.Secrets = payload.Secrets
		}
		content, err := marshalConfigRepositoryYAML(doc)
		if err != nil {
			return err
		}
		files[filePath] = string(content)
	}
	return nil
}

func (a *App) exportConfigRepositoryScopeVariables(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, addFile func(string) *configRepositoryScopeExport, addAccess func(string, string, bool, sql.NullInt64)) error {
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
		if addAccess != nil {
			addAccess(scope, sourcePath, managed, configRepoID)
		}
	}
	return rows.Err()
}

func (a *App) exportConfigRepositoryScopeSecrets(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, addFile func(string) *configRepositoryScopeExport, addAccess func(string, string, bool, sql.NullInt64)) error {
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
		if addAccess != nil {
			addAccess(scope, sourcePath, managed, configRepoID)
		}
	}
	return rows.Err()
}

func (a *App) exportConfigRepositoryKnowledge(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, resourceAccess map[resourceAccessPlanKey]configRepositoryResourceAccessState, files map[string]string) error {
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
		var access *configRepositoryEmbeddedAccessFile
		if currentAccess, ok := resourceAccess[resourceAccessPlanKey{resourceType: grantResourceKnowledgeContext, resourceID: identifier}]; ok {
			access = currentAccess.exportFile()
		}
		files[filePath] = renderKnowledgeContextGitOpsDocument(kind, name, description, content, identifier, access)
	}
	return rows.Err()
}

type configRepositoryGroupStructureExportNode struct {
	Description string
	Config      *configRepositoryBindingExport
	Apps        []configRepositoryGroupStructureAppExport
	Children    map[string]*configRepositoryGroupStructureExportNode
}

type configRepositoryGroupStructureAppExport struct {
	Name    string `yaml:"name"`
	RepoURL string `yaml:"repo_url"`
}

type configRepositoryBindingExport struct {
	RepoURL      string `yaml:"repo_url"`
	Branch       string `yaml:"branch,omitempty"`
	BasePath     string `yaml:"base_path,omitempty"`
	Enabled      *bool  `yaml:"enabled,omitempty"`
	WriteEnabled *bool  `yaml:"write_enabled,omitempty"`
	WriteBranch  string `yaml:"write_branch,omitempty"`
}

func (a *App) exportConfigRepositoryGroupStructure(ctx context.Context, repo models.ConfigRepository, files map[string]string) error {
	records, err := loadGroupPathRecords(ctx, a.db)
	if err != nil {
		return err
	}
	structure := map[string]*configRepositoryGroupStructureExportNode{}
	for _, record := range records {
		path := strings.Trim(strings.TrimSpace(record.Path), "/")
		if path == "" || !configRepositoryGroupStructureIncludesPath(repo, path) {
			continue
		}
		if record.Kind == "app" || record.RepositoryFullName != "" {
			parentPath := groupItemParentPath(path)
			if parentPath == "" || !configRepositoryGroupStructureIncludesPath(repo, parentPath) {
				continue
			}
			parent := ensureConfigRepositoryGroupStructureExportPath(structure, parentPath)
			app := configRepositoryGroupStructureAppExport{
				Name:    strings.TrimSpace(record.Name),
				RepoURL: strings.TrimSpace(record.RepoURL),
			}
			if app.Name == "" {
				app.Name = repositoryDisplayNameFromFullName(record.RepositoryFullName)
			}
			if app.RepoURL == "" && strings.TrimSpace(record.RepositoryFullName) != "" {
				app.RepoURL = canonicalRepositoryURL(record.RepositoryFullName)
			}
			if app.Name != "" && app.RepoURL != "" {
				parent.Apps = append(parent.Apps, app)
			}
			continue
		}
		node := ensureConfigRepositoryGroupStructureExportPath(structure, path)
		node.Description = strings.TrimSpace(record.Description)
	}

	rows, err := a.db.Query(ctx, `
		SELECT scope_id, repo_url, branch, base_path, enabled, write_enabled, write_branch,
		       config_repo_id, managed_by_config_repo
		FROM config_repositories
		WHERE scope_type = $1
		  AND id <> $2
		ORDER BY scope_id ASC
	`, models.ConfigRepositoryScopeFolder, repo.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var scopeID, repoURL, branch, basePath, writeBranch string
		var enabled, writeEnabled, managed bool
		var configRepoID sql.NullInt64
		if err := rows.Scan(&scopeID, &repoURL, &branch, &basePath, &enabled, &writeEnabled, &writeBranch, &configRepoID, &managed); err != nil {
			return err
		}
		scopeID = strings.Trim(strings.TrimSpace(scopeID), "/")
		if scopeID == "" || !configRepositoryGroupStructureIncludesPath(repo, scopeID) {
			continue
		}
		if managed && (!configRepoID.Valid || configRepoID.Int64 != repo.ID) {
			continue
		}
		if !managed && repo.ScopeType != models.ConfigRepositoryScopeSystem && !configResourceUnderScope(scopeID, repo.ScopeID) {
			continue
		}
		enabledValue := enabled
		writeEnabledValue := writeEnabled
		node := ensureConfigRepositoryGroupStructureExportPath(structure, scopeID)
		node.Config = &configRepositoryBindingExport{
			RepoURL:      strings.TrimSpace(repoURL),
			Branch:       strings.TrimSpace(branch),
			BasePath:     strings.TrimSpace(basePath),
			Enabled:      &enabledValue,
			WriteEnabled: &writeEnabledValue,
			WriteBranch:  strings.TrimSpace(writeBranch),
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(structure) == 0 {
		return nil
	}
	content, err := marshalConfigRepositoryYAML(configRepositoryGroupStructureExportMap(structure))
	if err != nil {
		return err
	}
	files[configRepositoryGroupStructurePath] = string(content)
	return nil
}

func configRepositoryGroupStructureIncludesPath(repo models.ConfigRepository, path string) bool {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return false
	}
	if repo.ScopeType == models.ConfigRepositoryScopeSystem {
		return true
	}
	return configResourceUnderScope(path, repo.ScopeID)
}

func ensureConfigRepositoryGroupStructureExportPath(structure map[string]*configRepositoryGroupStructureExportNode, path string) *configRepositoryGroupStructureExportNode {
	parts := strings.Split(strings.Trim(strings.TrimSpace(path), "/"), "/")
	children := structure
	var current *configRepositoryGroupStructureExportNode
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		current = children[part]
		if current == nil {
			current = &configRepositoryGroupStructureExportNode{Children: map[string]*configRepositoryGroupStructureExportNode{}}
			children[part] = current
		}
		if current.Children == nil {
			current.Children = map[string]*configRepositoryGroupStructureExportNode{}
		}
		children = current.Children
	}
	return current
}

func configRepositoryGroupStructureExportMap(structure map[string]*configRepositoryGroupStructureExportNode) map[string]any {
	out := map[string]any{}
	names := make([]string, 0, len(structure))
	for name := range structure {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out[name] = configRepositoryGroupStructureNodeExportMap(structure[name])
	}
	return out
}

func configRepositoryGroupStructureNodeExportMap(node *configRepositoryGroupStructureExportNode) map[string]any {
	out := map[string]any{}
	if node == nil {
		return out
	}
	if strings.TrimSpace(node.Description) != "" {
		out["description"] = strings.TrimSpace(node.Description)
	}
	if node.Config != nil {
		out["config"] = node.Config
	}
	if len(node.Apps) > 0 {
		sort.Slice(node.Apps, func(i, j int) bool {
			return node.Apps[i].Name < node.Apps[j].Name
		})
		out["apps"] = node.Apps
	}
	for name, child := range configRepositoryGroupStructureExportMap(node.Children) {
		out[name] = child
	}
	return out
}

const (
	configRepositoryAccessAllPath             = "access/all.yaml"
	configRepositoryAccessGrantsPath          = "access/grants.yaml"
	configRepositoryServiceAccountsAccessPath = "access/service-accounts.yaml"
	configRepositoryGroupStructurePath        = "config-repositories/groups/structure.yaml"
	configRepositoryLLMProfilesPath           = "setting/system/llm_profile.yaml"
	configRepositoryMCPRegistryPath           = "setting/system/mcp.yaml"
)

type configRepositoryAccessExportDocument struct {
	Users                []configRepositoryUserExport                `yaml:"users,omitempty"`
	ServiceAccounts      []configRepositoryServiceAccountExport      `yaml:"service_accounts,omitempty"`
	AdvancedRoles        []configRepositoryAdvancedRoleExport        `yaml:"advanced_roles,omitempty"`
	Policies             []configRepositoryAccessPolicyExport        `yaml:"policies,omitempty"`
	AdvancedRoleBindings []configRepositoryAdvancedRoleBindingExport `yaml:"advanced_role_bindings,omitempty"`
	BasicRoles           []configRepositoryBasicRoleExport           `yaml:"basic_roles,omitempty"`
}

type configRepositoryUserExport struct {
	Sub           string   `yaml:"sub"`
	Email         string   `yaml:"email,omitempty"`
	Provider      string   `yaml:"provider,omitempty"`
	Status        string   `yaml:"status"`
	AdvancedRoles []string `yaml:"advanced_roles,omitempty"`
}

type configRepositoryServiceAccountExport struct {
	Sub           string   `yaml:"sub"`
	Email         string   `yaml:"email,omitempty"`
	Status        string   `yaml:"status"`
	AdvancedRoles []string `yaml:"advanced_roles,omitempty"`
}

type configRepositoryBasicRoleExport struct {
	User           string `yaml:"user,omitempty"`
	ServiceAccount string `yaml:"service_account,omitempty"`
	SubjectType    string `yaml:"subject_type,omitempty"`
	SubjectID      string `yaml:"subject_id,omitempty"`
	Role           string `yaml:"role"`
	Resource       string `yaml:"resource"`
	Inherit        *bool  `yaml:"inherit,omitempty"`
}

type configRepositoryAdvancedRoleExport struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

type configRepositoryAccessPolicyExport struct {
	Role     string `yaml:"role"`
	Name     string `yaml:"name,omitempty"`
	Resource string `yaml:"resource"`
	Action   string `yaml:"action"`
	Effect   string `yaml:"effect,omitempty"`
}

type configRepositoryAdvancedRoleBindingExport struct {
	Role        string `yaml:"role"`
	SubjectType string `yaml:"subject_type"`
	SubjectID   string `yaml:"subject_id"`
}

func (a *App) exportConfigRepositoryAccess(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	if repo.ScopeType == models.ConfigRepositoryScopeSystem {
		allDoc := configRepositoryAccessExportDocument{}
		users, err := a.configRepositoryUserExports(ctx, repo)
		if err != nil {
			return err
		}
		allDoc.Users = users
		roles, err := a.configRepositoryAdvancedRoleExports(ctx, repo)
		if err != nil {
			return err
		}
		allDoc.AdvancedRoles = roles
		policies, err := a.configRepositoryAccessPolicyExports(ctx, repo)
		if err != nil {
			return err
		}
		allDoc.Policies = policies
		roleBindings, err := a.configRepositoryAdvancedRoleBindingExports(ctx, repo)
		if err != nil {
			return err
		}
		allDoc.AdvancedRoleBindings = roleBindings
		grants, err := a.configRepositoryBasicRoleGrantExports(ctx, repo, delegatedScopes, func(subjectType string) bool {
			return subjectType != grantSubjectServiceAccount
		})
		if err != nil {
			return err
		}
		allDoc.BasicRoles = grants
		if !allDoc.empty() {
			content, err := marshalConfigRepositoryYAML(allDoc)
			if err != nil {
				return err
			}
			files[configRepositoryAccessAllPath] = string(content)
		}

		serviceDoc := configRepositoryAccessExportDocument{}
		serviceAccounts, err := a.configRepositoryServiceAccountExports(ctx, repo)
		if err != nil {
			return err
		}
		serviceDoc.ServiceAccounts = serviceAccounts
		serviceGrants, err := a.configRepositoryBasicRoleGrantExports(ctx, repo, delegatedScopes, func(subjectType string) bool {
			return subjectType == grantSubjectServiceAccount
		})
		if err != nil {
			return err
		}
		serviceDoc.BasicRoles = serviceGrants
		if !serviceDoc.empty() {
			content, err := marshalConfigRepositoryYAML(serviceDoc)
			if err != nil {
				return err
			}
			files[configRepositoryServiceAccountsAccessPath] = string(content)
		}
		return nil
	}

	grants, err := a.configRepositoryBasicRoleGrantExports(ctx, repo, delegatedScopes, func(subjectType string) bool {
		return true
	})
	if err != nil {
		return err
	}
	if len(grants) == 0 {
		return nil
	}
	doc := configRepositoryAccessExportDocument{BasicRoles: grants}
	content, err := marshalConfigRepositoryYAML(doc)
	if err != nil {
		return err
	}
	files[configRepositoryAccessGrantsPath] = string(content)
	return nil
}

func (doc configRepositoryAccessExportDocument) empty() bool {
	return len(doc.Users) == 0 &&
		len(doc.ServiceAccounts) == 0 &&
		len(doc.AdvancedRoles) == 0 &&
		len(doc.Policies) == 0 &&
		len(doc.AdvancedRoleBindings) == 0 &&
		len(doc.BasicRoles) == 0
}

func (a *App) configRepositoryUserExports(ctx context.Context, repo models.ConfigRepository) ([]configRepositoryUserExport, error) {
	rows, err := a.db.Query(ctx, `
		SELECT sub, COALESCE(email, ''), provider, status
		FROM users
		WHERE provider <> $1
		  AND sub <> $2
		  AND (managed_by_config_repo = FALSE OR config_repo_id = $3)
		ORDER BY sub ASC
	`, auth.ProviderServiceAccount, defaultAdminSub, repo.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []configRepositoryUserExport{}
	userIndex := map[string]int{}
	for rows.Next() {
		var user configRepositoryUserExport
		if err := rows.Scan(&user.Sub, &user.Email, &user.Provider, &user.Status); err != nil {
			return nil, err
		}
		user.Sub = strings.TrimSpace(user.Sub)
		user.Email = strings.TrimSpace(user.Email)
		user.Provider = strings.TrimSpace(user.Provider)
		user.Status = strings.TrimSpace(user.Status)
		if user.Sub == "" {
			continue
		}
		if user.Status == "" {
			user.Status = "active"
		}
		if user.Provider == "" || user.Provider == "local" {
			user.Provider = ""
		}
		userIndex[user.Sub] = len(users)
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	roleRows, err := a.db.Query(ctx, `
		SELECT subject_id, role_name
		FROM auth_role_bindings
		WHERE subject_type = $1
		  AND (managed_by_config_repo = FALSE OR config_repo_id = $2)
		ORDER BY subject_id ASC, role_name ASC
	`, grantSubjectUser, repo.ID)
	if err != nil {
		return nil, err
	}
	defer roleRows.Close()
	for roleRows.Next() {
		var subjectID, roleName string
		if err := roleRows.Scan(&subjectID, &roleName); err != nil {
			return nil, err
		}
		idx, ok := userIndex[strings.TrimSpace(subjectID)]
		if !ok {
			continue
		}
		roleName = strings.TrimSpace(roleName)
		if roleName == "" {
			continue
		}
		users[idx].AdvancedRoles = append(users[idx].AdvancedRoles, roleName)
	}
	if err := roleRows.Err(); err != nil {
		return nil, err
	}
	for idx := range users {
		users[idx].AdvancedRoles = uniqueSortedStrings(users[idx].AdvancedRoles)
	}
	return users, nil
}

func (a *App) configRepositoryServiceAccountExports(ctx context.Context, repo models.ConfigRepository) ([]configRepositoryServiceAccountExport, error) {
	rows, err := a.db.Query(ctx, `
		SELECT sub, COALESCE(email, ''), status
		FROM users
		WHERE provider = $1
		  AND (managed_by_config_repo = FALSE OR config_repo_id = $2)
		ORDER BY sub ASC
	`, auth.ProviderServiceAccount, repo.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := []configRepositoryServiceAccountExport{}
	accountIndex := map[string]int{}
	for rows.Next() {
		var account configRepositoryServiceAccountExport
		if err := rows.Scan(&account.Sub, &account.Email, &account.Status); err != nil {
			return nil, err
		}
		account.Sub = strings.TrimSpace(account.Sub)
		account.Email = strings.TrimSpace(account.Email)
		account.Status = strings.TrimSpace(account.Status)
		if account.Sub == "" {
			continue
		}
		if account.Status == "" {
			account.Status = "active"
		}
		accountIndex[account.Sub] = len(accounts)
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	roleRows, err := a.db.Query(ctx, `
		SELECT subject_id, role_name
		FROM auth_role_bindings arb
		WHERE subject_type = $1
		  AND (managed_by_config_repo = FALSE OR config_repo_id = $2)
		  AND NOT EXISTS (
			SELECT 1
			FROM access_grants ag
			WHERE ag.subject_type = arb.subject_type
			  AND ag.subject_id = arb.subject_id
			  AND ag.role_name = arb.role_name
			  AND ag.role_name = $3
		  )
		ORDER BY subject_id ASC, role_name ASC
	`, grantSubjectServiceAccount, repo.ID, productRoleAdmin)
	if err != nil {
		return nil, err
	}
	defer roleRows.Close()

	for roleRows.Next() {
		var subjectID, roleName string
		if err := roleRows.Scan(&subjectID, &roleName); err != nil {
			return nil, err
		}
		idx, ok := accountIndex[strings.TrimSpace(subjectID)]
		if !ok {
			continue
		}
		roleName = strings.TrimSpace(roleName)
		if roleName == "" {
			continue
		}
		accounts[idx].AdvancedRoles = append(accounts[idx].AdvancedRoles, roleName)
	}
	if err := roleRows.Err(); err != nil {
		return nil, err
	}
	for idx := range accounts {
		accounts[idx].AdvancedRoles = uniqueSortedStrings(accounts[idx].AdvancedRoles)
	}
	return accounts, nil
}

func (a *App) configRepositoryAdvancedRoleExports(ctx context.Context, repo models.ConfigRepository) ([]configRepositoryAdvancedRoleExport, error) {
	rows, err := a.db.Query(ctx, `
		SELECT name, COALESCE(description, '')
		FROM auth_roles
		WHERE managed_by_config_repo = FALSE OR config_repo_id = $1
		ORDER BY name ASC
	`, repo.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roles := []configRepositoryAdvancedRoleExport{}
	for rows.Next() {
		var role configRepositoryAdvancedRoleExport
		if err := rows.Scan(&role.Name, &role.Description); err != nil {
			return nil, err
		}
		role.Name = strings.TrimSpace(role.Name)
		role.Description = strings.TrimSpace(role.Description)
		if role.Name == "" || isProtectedAdminRoleName(role.Name) {
			continue
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return roles, nil
}

func (a *App) configRepositoryAccessPolicyExports(ctx context.Context, repo models.ConfigRepository) ([]configRepositoryAccessPolicyExport, error) {
	rows, err := a.db.Query(ctx, `
		SELECT role_name, resource_type, resource_id, action, effect
		FROM auth_role_permissions
		WHERE managed_by_config_repo = FALSE OR config_repo_id = $1
		ORDER BY role_name ASC, resource_type ASC, resource_id ASC, action ASC, effect ASC
	`, repo.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	policies := []configRepositoryAccessPolicyExport{}
	for rows.Next() {
		var policy configRepositoryAccessPolicyExport
		var resourceType, resourceID, effect string
		if err := rows.Scan(&policy.Role, &resourceType, &resourceID, &policy.Action, &effect); err != nil {
			return nil, err
		}
		policy.Role = strings.TrimSpace(policy.Role)
		resourceType = strings.TrimSpace(resourceType)
		resourceID = strings.TrimSpace(resourceID)
		policy.Action = strings.TrimSpace(policy.Action)
		effect = strings.TrimSpace(effect)
		if policy.Role == "" || isProtectedAdminRoleName(policy.Role) || resourceType == "" || policy.Action == "" {
			continue
		}
		policy.Resource = resourceType + ":" + resourceID
		if effect != "" && effect != "allow" {
			policy.Effect = effect
		}
		policies = append(policies, policy)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return policies, nil
}

func (a *App) configRepositoryAdvancedRoleBindingExports(ctx context.Context, repo models.ConfigRepository) ([]configRepositoryAdvancedRoleBindingExport, error) {
	rows, err := a.db.Query(ctx, `
		SELECT role_name, subject_type, subject_id
		FROM auth_role_bindings
		WHERE subject_type = ANY($1)
		  AND (managed_by_config_repo = FALSE OR config_repo_id = $2)
		ORDER BY role_name ASC, subject_type ASC, subject_id ASC
	`, []string{model.SubjectTypeRepository, model.SubjectTypeTrigger, model.SubjectTypeInternalService}, repo.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bindings := []configRepositoryAdvancedRoleBindingExport{}
	for rows.Next() {
		var binding configRepositoryAdvancedRoleBindingExport
		if err := rows.Scan(&binding.Role, &binding.SubjectType, &binding.SubjectID); err != nil {
			return nil, err
		}
		binding.Role = strings.TrimSpace(binding.Role)
		binding.SubjectType = exportAccessSubjectType(binding.SubjectType)
		binding.SubjectID = strings.Trim(strings.TrimSpace(binding.SubjectID), "/")
		if binding.Role == "" || isProtectedAdminRoleName(binding.Role) || binding.SubjectType == "" || binding.SubjectID == "" {
			continue
		}
		bindings = append(bindings, binding)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return bindings, nil
}

func (a *App) configRepositoryBasicRoleGrantExports(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, includeSubject func(string) bool) ([]configRepositoryBasicRoleExport, error) {
	rows, err := a.db.Query(ctx, `
		SELECT
			ag.subject_type,
			CASE
				WHEN ag.subject_type = $2 THEN COALESCE(NULLIF(u.sub, ''), ag.subject_id)
				WHEN ag.subject_type = $4 THEN COALESCE(NULLIF(sa.sub, ''), ag.subject_id)
				ELSE ag.subject_id
			END AS subject_id,
			ag.role_name,
			ag.resource_type,
			ag.resource_id,
			ag.inherit,
			ag.config_repo_id,
			ag.managed_by_config_repo
		FROM access_grants ag
		LEFT JOIN users u
		  ON ag.subject_type = $2
		 AND (ag.subject_id = u.id::text OR ag.subject_id = u.sub)
		 AND u.provider <> $3
		LEFT JOIN users sa
		  ON ag.subject_type = $4
		 AND (ag.subject_id = sa.sub OR ag.subject_id = sa.id::text)
		 AND sa.provider = $3
		WHERE ag.role_name <> $1
		ORDER BY ag.subject_type ASC, subject_id ASC, ag.resource_type ASC, ag.resource_id ASC, ag.role_name ASC
	`, customUseGrantRole, model.SubjectTypeUser, auth.ProviderServiceAccount, model.SubjectTypeServiceAccount)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grants := []configRepositoryBasicRoleExport{}
	for rows.Next() {
		var subjectType, subjectID, roleName, resourceType, resourceID string
		var inherit, managed bool
		var configRepoID sql.NullInt64
		if err := rows.Scan(&subjectType, &subjectID, &roleName, &resourceType, &resourceID, &inherit, &configRepoID, &managed); err != nil {
			return nil, err
		}
		subjectType = exportAccessSubjectType(subjectType)
		subjectID = strings.Trim(strings.TrimSpace(subjectID), "/")
		roleName = strings.TrimSpace(roleName)
		resourceType = strings.TrimSpace(resourceType)
		resourceID = strings.Trim(strings.TrimSpace(resourceID), "/")
		if subjectType == "" || subjectID == "" || roleName == "" || resourceType == "" {
			continue
		}
		if includeSubject != nil && !includeSubject(subjectType) {
			continue
		}
		if _, ok := productRoleDefinitions[roleName]; !ok {
			continue
		}
		if managed {
			if !configRepoID.Valid || configRepoID.Int64 != repo.ID {
				continue
			}
		}
		if !configRepositoryIncludesBasicRoleGrant(repo, resourceType, resourceID, delegatedScopes) {
			continue
		}
		grant := configRepositoryBasicRoleExport{
			Role:     roleName,
			Resource: configRepositoryBasicRoleResourceExport(resourceType, resourceID),
		}
		if !setConfigRepositoryBasicRoleSubjectExport(&grant, subjectType, subjectID) {
			continue
		}
		defaultInherit := resourceType == grantResourceFolder
		if inherit != defaultInherit {
			next := inherit
			grant.Inherit = &next
		}
		grants = append(grants, grant)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return grants, nil
}

func setConfigRepositoryBasicRoleSubjectExport(grant *configRepositoryBasicRoleExport, subjectType, subjectID string) bool {
	if grant == nil {
		return false
	}
	subjectID = strings.Trim(strings.TrimSpace(subjectID), "/")
	if subjectID == "" {
		return false
	}
	switch strings.TrimSpace(subjectType) {
	case grantSubjectUser:
		grant.User = subjectID
	case grantSubjectServiceAccount:
		grant.ServiceAccount = subjectID
	default:
		grant.SubjectType = strings.TrimSpace(subjectType)
		grant.SubjectID = subjectID
	}
	return grant.User != "" || grant.ServiceAccount != "" || (grant.SubjectType != "" && grant.SubjectID != "")
}

func configRepositoryBasicRoleResourceExport(resourceType, resourceID string) string {
	resourceType = strings.TrimSpace(resourceType)
	resourceID = strings.Trim(strings.TrimSpace(resourceID), "/")
	if resourceType == grantResourceFolder && resourceID == generalGrantID {
		resourceID = "general"
	}
	return resourceType + ":" + resourceID
}

func exportAccessSubjectType(subjectType string) string {
	switch strings.TrimSpace(subjectType) {
	case model.SubjectTypeUser:
		return grantSubjectUser
	case model.SubjectTypeRepository:
		return grantSubjectRepository
	case model.SubjectTypeTrigger:
		return grantSubjectTrigger
	case model.SubjectTypeServiceAccount:
		return grantSubjectServiceAccount
	case model.SubjectTypeInternalService, grantSubjectService:
		return model.SubjectTypeInternalService
	default:
		return ""
	}
}

type configRepositoryLLMProfilesExportDocument struct {
	DefaultProfile string           `yaml:"default_profile"`
	Profiles       []llmProfileForm `yaml:"profiles"`
}

func (a *App) exportConfigRepositoryLLMProfiles(ctx context.Context, repo models.ConfigRepository, files map[string]string) error {
	if repo.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil
	}

	defaultProfile, profiles, found, err := a.loadLLMProfilesFromDB(ctx)
	if err != nil {
		return err
	}
	if !found {
		cfg := a.getConfigSnapshot()
		defaultProfile = cfg.EffectiveLLMDefaultProfile()
		profiles = cfg.EffectiveLLMProfiles()
	}
	profiles = config.NormalizeLLMProfiles(profiles)
	if len(profiles) == 0 {
		return nil
	}
	defaultProfile = config.NormalizeLLMProfileName(defaultProfile)
	if defaultProfile == "" {
		defaultProfile = config.DefaultLLMProfileName
	}

	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	doc := configRepositoryLLMProfilesExportDocument{DefaultProfile: defaultProfile}
	for _, name := range names {
		doc.Profiles = append(doc.Profiles, profileFormFromConfig(name, profiles[name]))
	}
	content, err := marshalConfigRepositoryYAML(doc)
	if err != nil {
		return err
	}
	files[configRepositoryLLMProfilesPath] = string(content)
	return nil
}

func (a *App) exportConfigRepositoryMCPRegistry(ctx context.Context, repo models.ConfigRepository, files map[string]string) error {
	if repo.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil
	}

	servers, profiles, found, err := a.loadMCPRegistryFromDB(ctx)
	if err != nil {
		return err
	}
	if !found {
		cfg := a.getConfigSnapshot()
		servers = cfg.EffectiveMCPServers()
		profiles = cfg.EffectiveMCPProfiles()
	}
	servers = models.NormalizeMCPServers(servers)
	profiles = models.NormalizeMCPProfiles(profiles)
	if len(servers) == 0 && len(profiles) == 0 {
		return nil
	}

	exportServers := map[string]models.MCPServer{}
	for name, server := range servers {
		server.Name = ""
		exportServers[name] = server
	}
	exportProfiles := map[string]models.MCPProfile{}
	for name, profile := range profiles {
		profile.Name = ""
		exportProfiles[name] = profile
	}

	content, err := marshalConfigRepositoryYAML(mcpRegistryRequest{
		MCPServers:  exportServers,
		MCPProfiles: exportProfiles,
	})
	if err != nil {
		return err
	}
	files[configRepositoryMCPRegistryPath] = string(content)
	return nil
}

func (a *App) exportConfigRepositoryRuntimeSettings(repo models.ConfigRepository, files map[string]string) error {
	if repo.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil
	}
	doc := buildRuntimeSettingsGitOpsFile(a.getConfigSnapshot())
	content, err := marshalConfigRepositoryYAML(doc)
	if err != nil {
		return err
	}
	files["setting/system/runner.yaml"] = string(content)
	return nil
}

type configRepositoryKnowledgeDocument struct {
	Name        string                              `yaml:"name"`
	Kind        string                              `yaml:"kind"`
	Description string                              `yaml:"description,omitempty"`
	Access      *configRepositoryEmbeddedAccessFile `yaml:"access,omitempty"`
	Content     string                              `yaml:"content"`
}

func renderKnowledgeContextGitOpsDocument(kind, name, description, content, fallbackName string, access *configRepositoryEmbeddedAccessFile) string {
	doc := configRepositoryKnowledgeDocument{
		Name:        name,
		Kind:        kind,
		Description: strings.TrimSpace(description),
		Access:      access,
		Content:     content,
	}
	if strings.TrimSpace(doc.Name) == "" {
		doc.Name = fallbackName
	}
	encoded, err := marshalConfigRepositoryYAML(doc)
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
	for _, prefix := range []string{"pipelines/", "steps/", "triggers/", externalTriggersGitOpsDirectory + "/", "schedules/", "scopes/", "knowledge/", "pipelineruns/", "config-repositories/"} {
		if strings.HasPrefix(filePath, prefix) {
			return true
		}
	}
	if strings.HasPrefix(filePath, "access/") && isYAMLFile(filePath) {
		return true
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
	return isGitOpsRuntimeSettingsRelativePath(rel) ||
		isGitOpsLLMProfileRelativePath(rel) ||
		isGitOpsMCPRegistryRelativePath(rel)
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

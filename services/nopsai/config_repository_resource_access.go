package nopsai

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"

	"gopkg.in/yaml.v3"
)

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
	Team           string   `yaml:"team,omitempty"`
	Repository     string   `yaml:"repository,omitempty"`
	User           string   `yaml:"user,omitempty"`
	Trigger        string   `yaml:"trigger,omitempty"`
	ServiceAccount string   `yaml:"service_account,omitempty"`
	Service        string   `yaml:"service,omitempty"`
	Actions        []string `yaml:"actions,omitempty"`
}

func (a *App) configRepositoryResourceAccess(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string) (map[resourceAccessPlanKey]configRepositoryResourceAccessState, error) {
	states := map[resourceAccessPlanKey]configRepositoryResourceAccessState{}
	include := func(resourceType, resourceID string) bool {
		return configRepositoryIncludesAccessResource(repo, resourceType, resourceID, delegatedScopes)
	}
	setState := func(key resourceAccessPlanKey, update func(*configRepositoryResourceAccessState)) {
		state := states[key]
		if strings.TrimSpace(state.Visibility) == "" {
			state.Visibility = resourceVisibilityTeam
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
		if !state.Override && len(state.Grants) == 0 && state.Visibility == resourceVisibilityTeam {
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
		resourceID := configsync.BuildPipelineIdentifier(pathPart, name)
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
	if repo.ScopeType == models.ConfigRepositoryScopeTeam &&
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
		case grantSubjectTeam:
			exportGrant.Team = subjectID
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
	if visibility == resourceVisibilityTeam && len(grants) == 0 {
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
		visibility = resourceVisibilityTeam
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

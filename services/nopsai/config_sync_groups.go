package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func configRepositoryOverrideScopes(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, parsed map[string]storedConfigRepository) ([]string, error) {
	scopeSet := map[string]struct{}{}
	addScope := func(scope string) {
		scope = strings.Trim(strings.TrimSpace(scope), "/")
		if scope == "" {
			return
		}
		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			boundScope := strings.Trim(strings.TrimSpace(binding.ScopeID), "/")
			if scope == boundScope || !configResourceUnderScope(scope, boundScope) {
				return
			}
		}
		scopeSet[scope] = struct{}{}
	}

	for _, repo := range parsed {
		if repo.enabled && repo.scopeType == models.ConfigRepositoryScopeFolder {
			addScope(repo.scopeID)
		}
	}

	rows, err := tx.Query(ctx, `
		SELECT scope_id
		FROM config_repositories
		WHERE scope_type = $1
		  AND enabled = TRUE
		  AND id <> $2
	`, models.ConfigRepositoryScopeFolder, binding.ID)
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
	return scopes, nil
}

func effectivePipelineRunStructureForConfigSync(
	binding models.ConfigRepository,
	configRepositories map[string]storedConfigRepository,
	pipelineRunStructure map[string]*pipelineRunStructureNode,
	configRepositoryPipelineRunStructure map[string]*pipelineRunStructureNode,
	overrideScopes []string,
) (map[string]*pipelineRunStructureNode, error) {
	effective, err := configRepositoryGroupStructure(binding, configRepositories)
	if err != nil {
		return nil, err
	}

	structure := pipelineRunStructure
	if binding.ScopeType == models.ConfigRepositoryScopeSystem && containsGroupConfigRepository(configRepositories) {
		structure = nil
	} else {
		structure = filterPipelineRunStructureByScopes(structure, configRepositoryStructureFilterScopes(binding, configRepositories, overrideScopes))
	}

	mergePipelineRunStructure(effective, structure)
	mergePipelineRunStructure(effective, configRepositoryPipelineRunStructure)
	return effective, nil
}

func containsGroupConfigRepository(configRepositories map[string]storedConfigRepository) bool {
	for _, repo := range configRepositories {
		if repo.scopeType == models.ConfigRepositoryScopeFolder {
			return true
		}
	}
	return false
}

func configRepositoryStructureFilterScopes(binding models.ConfigRepository, configRepositories map[string]storedConfigRepository, overrideScopes []string) []string {
	scopeSet := map[string]struct{}{}
	addScope := func(scope string) {
		scope = strings.Trim(strings.TrimSpace(scope), "/")
		if scope == "" {
			return
		}
		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			boundScope := strings.Trim(strings.TrimSpace(binding.ScopeID), "/")
			if scope == boundScope || !configResourceUnderScope(scope, boundScope) {
				return
			}
		}
		scopeSet[scope] = struct{}{}
	}

	for _, scope := range overrideScopes {
		addScope(scope)
	}
	for _, repo := range configRepositories {
		if repo.scopeType == models.ConfigRepositoryScopeFolder {
			addScope(repo.scopeID)
		}
	}

	scopes := make([]string, 0, len(scopeSet))
	for scope := range scopeSet {
		scopes = append(scopes, scope)
	}
	return scopes
}

func configRepositoryGroupStructure(binding models.ConfigRepository, configRepositories map[string]storedConfigRepository) (map[string]*pipelineRunStructureNode, error) {
	result := map[string]*pipelineRunStructureNode{}
	addPath := func(path string) error {
		segments, err := cleanConfigPathSegments(path, false)
		if err != nil {
			return err
		}
		ensurePipelineRunStructurePath(result, segments)
		return nil
	}

	if binding.ScopeType == models.ConfigRepositoryScopeFolder {
		if err := addPath(binding.ScopeID); err != nil {
			return nil, fmt.Errorf("invalid group-scoped config repository group path %q: %w", binding.ScopeID, err)
		}
	}
	for _, repo := range configRepositories {
		if repo.scopeType != models.ConfigRepositoryScopeFolder {
			continue
		}
		if err := addPath(repo.scopeID); err != nil {
			return nil, fmt.Errorf("invalid config repository group path %q: %w", repo.scopeID, err)
		}
	}
	return result, nil
}

func filterDelegatedConfigResources(
	binding models.ConfigRepository,
	overrideScopes []string,
	pipelines map[string]storedPipeline,
	steps map[string]storedStep,
	schedules map[string]storedSchedule,
	externalTriggers map[string]storedExternalTrigger,
	notificationRoutes map[string]storedNotificationRoute,
	knowledgeContexts map[string]storedKnowledgeContext,
	generalScopeVars map[generalScopeVarKey]storedScopeVar,
	repoScopeVars map[repoScopeVarKey]storedScopeVar,
	generalScopeSecrets map[generalScopeSecretKey]storedScopeSecret,
	repoScopeSecrets map[repoScopeSecretKey]storedScopeSecret,
	triggers map[string]storedTrigger,
) {
	if len(overrideScopes) == 0 {
		return
	}

	for key := range pipelines {
		if configResourceUnderAnyScope(key, overrideScopes) {
			delete(pipelines, key)
		}
	}
	for key := range steps {
		if configResourceUnderAnyScope(key, overrideScopes) {
			delete(steps, key)
		}
	}
	for key, schedule := range schedules {
		scope := schedule.input.Path
		if scope == "" {
			scope = key
		}
		if configResourceUnderAnyScope(scope, overrideScopes) {
			delete(schedules, key)
		}
	}
	for key, trigger := range externalTriggers {
		if configResourceUnderAnyScope(externalTriggerConfigScope(trigger.input), overrideScopes) {
			delete(externalTriggers, key)
		}
	}
	for key, route := range notificationRoutes {
		if configResourceUnderAnyScope(notificationRouteResourceScope(route), overrideScopes) {
			delete(notificationRoutes, key)
		}
	}
	for key, knowledge := range knowledgeContexts {
		scope := knowledge.group
		if scope == "" {
			scope = key
		}
		if configResourceUnderAnyScope(scope, overrideScopes) {
			delete(knowledgeContexts, key)
		}
	}
	for key := range generalScopeVars {
		if configResourceUnderAnyScope(key.scopePath, overrideScopes) {
			delete(generalScopeVars, key)
		}
	}
	for key := range repoScopeVars {
		if configResourceUnderAnyScope(key.repo, overrideScopes) || configResourceUnderAnyScope(key.scopePath, overrideScopes) {
			delete(repoScopeVars, key)
		}
	}
	for key := range generalScopeSecrets {
		if configResourceUnderAnyScope(key.scopePath, overrideScopes) {
			delete(generalScopeSecrets, key)
		}
	}
	for key := range repoScopeSecrets {
		if configResourceUnderAnyScope(key.repo, overrideScopes) || configResourceUnderAnyScope(key.scopePath, overrideScopes) {
			delete(repoScopeSecrets, key)
		}
	}
	for key := range triggers {
		if configResourceUnderAnyScope(key, overrideScopes) {
			delete(triggers, key)
		}
	}
}

func loadConfigRepositoryByID(ctx context.Context, tx pgx.Tx, id int64) (models.ConfigRepository, error) {
	var repo models.ConfigRepository
	err := tx.QueryRow(ctx, `
		SELECT id, scope_type, scope_id
		FROM config_repositories
		WHERE id = $1
	`, id).Scan(&repo.ID, &repo.ScopeType, &repo.ScopeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ConfigRepository{}, fmt.Errorf("config repository %d not found", id)
		}
		return models.ConfigRepository{}, err
	}
	return repo, nil
}

func ensureConfigResourceWritable(ctx context.Context, tx pgx.Tx, tableName, resourceKind, resourceID string, binding models.ConfigRepository, resourceScope string, whereClause string, args ...any) (bool, error) {
	query := fmt.Sprintf("SELECT config_repo_id, managed_by_config_repo FROM %s WHERE %s LIMIT 1", tableName, whereClause)
	var existingRepoID sql.NullInt64
	var managed bool
	if err := tx.QueryRow(ctx, query, args...).Scan(&existingRepoID, &managed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil
		}
		return false, err
	}
	if !managed {
		if canConfigRepositoryAdoptUnmanagedResource(binding, resourceScope) {
			return true, nil
		}
		return false, fmt.Errorf("%s %s is outside the config repository scope", resourceKind, resourceID)
	}
	if !existingRepoID.Valid {
		return false, fmt.Errorf("%s %s is already managed by an unknown config repository", resourceKind, resourceID)
	}
	if existingRepoID.Int64 == binding.ID {
		return true, nil
	}

	existing, err := loadConfigRepositoryByID(ctx, tx, existingRepoID.Int64)
	if err != nil {
		return false, err
	}
	if canConfigRepositoryWriteOver(binding, existing, resourceScope) {
		return true, nil
	}
	if configRepositoryShadowsCurrent(existing, binding, resourceScope) {
		return false, nil
	}

	owner := strconv.FormatInt(existingRepoID.Int64, 10)
	return false, fmt.Errorf("%s %s is already managed by config repository %s", resourceKind, resourceID, owner)
}

type groupRecord struct {
	ID                 int
	Name               string
	Kind               string
	ParentID           *int
	Description        string
	RepoURL            string
	RepositoryFullName string
}

type groupRecordSet struct {
	byName map[string]*groupRecord
	byRepo map[string]*groupRecord
}

func loadExistingGroupRecords(ctx context.Context, tx pgx.Tx) (groupRecordSet, error) {
	rows, err := tx.Query(ctx, "SELECT id, name, COALESCE(kind, 'group'), parent_id, description, COALESCE(repo_url, ''), COALESCE(repository_full_name, '') FROM groups")
	if err != nil {
		return groupRecordSet{}, err
	}
	defer rows.Close()

	result := groupRecordSet{
		byName: make(map[string]*groupRecord),
		byRepo: make(map[string]*groupRecord),
	}
	for rows.Next() {
		var (
			record      groupRecord
			parentID    sql.NullInt32
			description sql.NullString
		)
		if err := rows.Scan(&record.ID, &record.Name, &record.Kind, &parentID, &description, &record.RepoURL, &record.RepositoryFullName); err != nil {
			return groupRecordSet{}, err
		}
		key, err := normalizeStructureName(record.Name)
		if err != nil {
			return groupRecordSet{}, err
		}
		if _, exists := result.byName[key]; exists {
			return groupRecordSet{}, fmt.Errorf("duplicate group name '%s' detected in database", key)
		}
		record.Name = key
		record.ParentID = pointerFromNullInt(parentID)
		record.Description = strings.TrimSpace(description.String)
		record.RepoURL = strings.TrimSpace(record.RepoURL)
		record.RepositoryFullName = strings.Trim(strings.TrimSpace(record.RepositoryFullName), "/")
		result.byName[key] = &record
		if record.RepositoryFullName != "" {
			repoKey := strings.ToLower(record.RepositoryFullName)
			if _, exists := result.byRepo[repoKey]; exists {
				return groupRecordSet{}, fmt.Errorf("duplicate repository app '%s' detected in database", record.RepositoryFullName)
			}
			result.byRepo[repoKey] = &record
		}
	}
	if err := rows.Err(); err != nil {
		return groupRecordSet{}, err
	}

	return result, nil
}

func pointerFromNullInt(value sql.NullInt32) *int {
	if !value.Valid {
		return nil
	}
	v := int(value.Int32)
	return &v
}

func copyIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	v := *value
	return &v
}

func parentPointersEqual(a, b *int) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a != nil && b != nil:
		return *a == *b
	default:
		return false
	}
}

func normalizePipelineRunStructureApp(app pipelineRunStructureApp) (pipelineRunStructureApp, error) {
	repoURL := strings.TrimSpace(app.RepoURL)
	fullName := strings.Trim(strings.TrimSpace(app.RepositoryFullName), "/")
	var err error
	if fullName == "" {
		if repoURL == "" {
			repoURL = strings.TrimSpace(app.Name)
		}
		fullName, err = configsync.RepositoryFullNameFromURL(repoURL)
		if err != nil {
			return pipelineRunStructureApp{}, err
		}
	}
	if repoURL == "" {
		repoURL = configsync.CanonicalRepositoryURL(fullName)
	}
	name := strings.TrimSpace(app.Name)
	if name == "" || strings.EqualFold(name, repoURL) {
		name = configsync.RepositoryDisplayNameFromFullName(fullName)
	}
	name, err = normalizeStructureName(name)
	if err != nil {
		return pipelineRunStructureApp{}, err
	}
	return pipelineRunStructureApp{
		Name:               name,
		RepoURL:            repoURL,
		RepositoryFullName: fullName,
	}, nil
}

func (a *App) syncPipelineRunGroups(ctx context.Context, tx pgx.Tx, structure map[string]*pipelineRunStructureNode, details map[string]int) error {
	if len(structure) == 0 {
		return nil
	}

	existingGroups, err := loadExistingGroupRecords(ctx, tx)
	if err != nil {
		return fmt.Errorf("failed to load existing pipeline run folders: %w", err)
	}

	registerGroupRecord := func(record *groupRecord) {
		if record == nil {
			return
		}
		existingGroups.byName[record.Name] = record
		if record.RepositoryFullName != "" {
			existingGroups.byRepo[strings.ToLower(record.RepositoryFullName)] = record
		}
	}

	var ensureFolder func(name string, parentID *int, description string) (int, error)
	ensureFolder = func(name string, parentID *int, description string) (int, error) {
		normalized, err := normalizeStructureName(name)
		if err != nil {
			return 0, err
		}
		description = strings.TrimSpace(description)
		if record, ok := existingGroups.byName[normalized]; ok {
			if record.Kind == "app" || record.RepositoryFullName != "" {
				return 0, fmt.Errorf("folder '%s' conflicts with an existing app", normalized)
			}
			parentChanged := !parentPointersEqual(record.ParentID, parentID)
			descChanged := strings.TrimSpace(record.Description) != description
			kindChanged := record.Kind != "group" || record.RepoURL != "" || record.RepositoryFullName != ""
			if parentChanged || descChanged || kindChanged {
				if _, err := tx.Exec(ctx, "UPDATE groups SET kind = 'group', parent_id = $1, description = $2, repo_url = '', repository_full_name = '', updated_at = NOW() WHERE id = $3", parentID, description, record.ID); err != nil {
					return 0, fmt.Errorf("failed to update folder '%s': %w", normalized, err)
				}
				delete(existingGroups.byRepo, strings.ToLower(record.RepositoryFullName))
				record.Kind = "group"
				record.ParentID = copyIntPointer(parentID)
				record.Description = description
				record.RepoURL = ""
				record.RepositoryFullName = ""
				details["run_groups_updated"]++
			}
			return record.ID, nil
		}

		var newID int
		if err := tx.QueryRow(ctx, "INSERT INTO groups (name, kind, parent_id, description) VALUES ($1, 'group', $2, $3) RETURNING id", normalized, parentID, description).Scan(&newID); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				refreshed, loadErr := loadExistingGroupRecords(ctx, tx)
				if loadErr != nil {
					return 0, fmt.Errorf("failed to reload folders after conflict: %w", loadErr)
				}
				existingGroups = refreshed
				if _, ok := existingGroups.byName[normalized]; ok {
					return ensureFolder(normalized, parentID, description)
				}
			}
			return 0, fmt.Errorf("failed to create folder '%s': %w", normalized, err)
		}
		registerGroupRecord(&groupRecord{ID: newID, Name: normalized, Kind: "group", ParentID: copyIntPointer(parentID), Description: description})
		details["run_groups_created"]++
		return newID, nil
	}

	var ensureApp func(app pipelineRunStructureApp, parentID *int) (int, error)
	ensureApp = func(app pipelineRunStructureApp, parentID *int) (int, error) {
		normalizedApp, err := normalizePipelineRunStructureApp(app)
		if err != nil {
			return 0, err
		}
		name := normalizedApp.Name
		repoURL := normalizedApp.RepoURL
		fullName := normalizedApp.RepositoryFullName
		repoKey := strings.ToLower(fullName)

		record := existingGroups.byRepo[repoKey]
		if record == nil {
			if existingByName, ok := existingGroups.byName[name]; ok {
				if existingByName.Kind != "app" && existingByName.RepositoryFullName == "" {
					return 0, fmt.Errorf("app '%s' conflicts with an existing folder", name)
				}
				if existingByName.RepositoryFullName != "" && !strings.EqualFold(existingByName.RepositoryFullName, fullName) {
					return 0, fmt.Errorf("app '%s' conflicts with repository '%s'", name, existingByName.RepositoryFullName)
				}
				record = existingByName
			}
		}
		if record != nil {
			parentChanged := !parentPointersEqual(record.ParentID, parentID)
			nameChanged := record.Name != name
			kindChanged := record.Kind != "app"
			descChanged := strings.TrimSpace(record.Description) != ""
			repoChanged := strings.TrimSpace(record.RepoURL) != repoURL || !strings.EqualFold(record.RepositoryFullName, fullName)
			if parentChanged || nameChanged || kindChanged || descChanged || repoChanged {
				if _, err := tx.Exec(ctx, "UPDATE groups SET name = $1, kind = 'app', parent_id = $2, description = '', repo_url = $3, repository_full_name = $4, updated_at = NOW() WHERE id = $5", name, parentID, repoURL, fullName, record.ID); err != nil {
					return 0, fmt.Errorf("failed to update app '%s': %w", name, err)
				}
				delete(existingGroups.byName, record.Name)
				delete(existingGroups.byRepo, strings.ToLower(record.RepositoryFullName))
				record.Name = name
				record.Kind = "app"
				record.ParentID = copyIntPointer(parentID)
				record.Description = ""
				record.RepoURL = repoURL
				record.RepositoryFullName = fullName
				registerGroupRecord(record)
				details["run_groups_updated"]++
			}
			return record.ID, nil
		}

		var newID int
		if err := tx.QueryRow(ctx, "INSERT INTO groups (name, kind, parent_id, description, repo_url, repository_full_name) VALUES ($1, 'app', $2, '', $3, $4) RETURNING id", name, parentID, repoURL, fullName).Scan(&newID); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				refreshed, loadErr := loadExistingGroupRecords(ctx, tx)
				if loadErr != nil {
					return 0, fmt.Errorf("failed to reload apps after conflict: %w", loadErr)
				}
				existingGroups = refreshed
				return ensureApp(normalizedApp, parentID)
			}
			return 0, fmt.Errorf("failed to create app '%s': %w", name, err)
		}
		registerGroupRecord(&groupRecord{ID: newID, Name: name, Kind: "app", ParentID: copyIntPointer(parentID), RepoURL: repoURL, RepositoryFullName: fullName})
		details["run_groups_created"]++
		return newID, nil
	}

	var applyNode func(name string, node *pipelineRunStructureNode, parentID *int) error
	applyNode = func(name string, node *pipelineRunStructureNode, parentID *int) error {
		groupID, err := ensureFolder(name, parentID, node.Description)
		if err != nil {
			return err
		}
		apps := node.Apps
		if len(apps) == 0 {
			for _, repoName := range node.Repos {
				apps = append(apps, pipelineRunStructureApp{Name: repoName, RepoURL: repoName})
			}
		}
		for _, app := range apps {
			if _, err := ensureApp(app, &groupID); err != nil {
				return err
			}
		}
		for childName, childNode := range node.Children {
			if err := applyNode(childName, childNode, &groupID); err != nil {
				return err
			}
		}
		return nil
	}

	for name, node := range structure {
		if node == nil {
			node = &pipelineRunStructureNode{Children: map[string]*pipelineRunStructureNode{}}
		}
		if err := applyNode(name, node, nil); err != nil {
			return fmt.Errorf("failed to sync folder '%s': %w", name, err)
		}
	}

	return nil
}

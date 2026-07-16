package nopsai

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
		if binding.ScopeType == models.ConfigRepositoryScopeTeam {
			boundScope := strings.Trim(strings.TrimSpace(binding.ScopeID), "/")
			if scope == boundScope || !configsync.ResourceUnderScope(scope, boundScope) {
				return
			}
		}
		scopeSet[scope] = struct{}{}
	}

	for _, repo := range parsed {
		if repo.enabled && repo.scopeType == models.ConfigRepositoryScopeTeam {
			addScope(repo.scopeID)
		}
	}

	rows, err := tx.Query(ctx, `
		SELECT scope_id
		FROM config_repositories
		WHERE scope_type = $1
		  AND enabled = TRUE
		  AND id <> $2
	`, models.ConfigRepositoryScopeTeam, binding.ID)
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
	configRepositoryPipelineRunStructure map[string]*configsync.PipelineRunStructureNode,
) (map[string]*configsync.PipelineRunStructureNode, error) {
	effective, err := configRepositoryTeamStructure(binding, configRepositories)
	if err != nil {
		return nil, err
	}
	configsync.MergePipelineRunStructure(effective, configRepositoryPipelineRunStructure)
	return effective, nil
}

func configRepositoryTeamStructure(binding models.ConfigRepository, configRepositories map[string]storedConfigRepository) (map[string]*configsync.PipelineRunStructureNode, error) {
	result := map[string]*configsync.PipelineRunStructureNode{}
	addPath := func(path string) error {
		segments, err := configsync.CleanPathSegments(path, false)
		if err != nil {
			return err
		}
		configsync.EnsurePipelineRunStructurePath(result, segments)
		return nil
	}

	if binding.ScopeType == models.ConfigRepositoryScopeTeam {
		if err := addPath(binding.ScopeID); err != nil {
			return nil, fmt.Errorf("invalid team-scoped config repository team path %q: %w", binding.ScopeID, err)
		}
	}
	for _, repo := range configRepositories {
		if repo.scopeType != models.ConfigRepositoryScopeTeam {
			continue
		}
		if err := addPath(repo.scopeID); err != nil {
			return nil, fmt.Errorf("invalid config repository team path %q: %w", repo.scopeID, err)
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
	dashboards map[string]storedDashboard,
	externalTriggers map[string]storedExternalTrigger,
	gitWebhookSources map[string]storedGitWebhookSource,
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
		if configsync.ResourceUnderAnyScope(key, overrideScopes) {
			delete(pipelines, key)
		}
	}
	for key := range steps {
		if configsync.ResourceUnderAnyScope(key, overrideScopes) {
			delete(steps, key)
		}
	}
	for key, schedule := range schedules {
		scope := schedule.input.Path
		if scope == "" {
			scope = key
		}
		if configsync.ResourceUnderAnyScope(scope, overrideScopes) {
			delete(schedules, key)
		}
	}
	for key, dashboard := range dashboards {
		scope := dashboard.teamPath
		if scope == "" {
			scope = key
		}
		if configsync.ResourceUnderAnyScope(scope, overrideScopes) {
			delete(dashboards, key)
		}
	}
	for key, trigger := range externalTriggers {
		if configsync.ResourceUnderAnyScope(externalTriggerConfigScope(trigger.input), overrideScopes) {
			delete(externalTriggers, key)
		}
	}
	for key, source := range gitWebhookSources {
		if configsync.ResourceUnderAnyScope(effectiveGitWebhookSourceTeamPath(source.input), overrideScopes) {
			delete(gitWebhookSources, key)
		}
	}
	for key, route := range notificationRoutes {
		if configsync.ResourceUnderAnyScope(notificationRouteResourceScope(route), overrideScopes) {
			delete(notificationRoutes, key)
		}
	}
	for key, knowledge := range knowledgeContexts {
		scope := knowledge.team
		if scope == "" {
			scope = key
		}
		if configsync.ResourceUnderAnyScope(scope, overrideScopes) {
			delete(knowledgeContexts, key)
		}
	}
	for key := range generalScopeVars {
		if configsync.ResourceUnderAnyScope(key.scopePath, overrideScopes) {
			delete(generalScopeVars, key)
		}
	}
	for key := range repoScopeVars {
		if configsync.ResourceUnderAnyScope(key.repo, overrideScopes) || configsync.ResourceUnderAnyScope(key.scopePath, overrideScopes) {
			delete(repoScopeVars, key)
		}
	}
	for key := range generalScopeSecrets {
		if configsync.ResourceUnderAnyScope(key.scopePath, overrideScopes) {
			delete(generalScopeSecrets, key)
		}
	}
	for key := range repoScopeSecrets {
		if configsync.ResourceUnderAnyScope(key.repo, overrideScopes) || configsync.ResourceUnderAnyScope(key.scopePath, overrideScopes) {
			delete(repoScopeSecrets, key)
		}
	}
	for key := range triggers {
		if configsync.ResourceUnderAnyScope(key, overrideScopes) {
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
		if configsync.CanRepositoryAdoptUnmanagedResource(binding, resourceScope) {
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
	if configsync.CanRepositoryWriteOver(binding, existing, resourceScope) {
		return true, nil
	}
	if configsync.RepositoryShadowsCurrent(existing, binding, resourceScope) {
		return false, nil
	}

	owner := strconv.FormatInt(existingRepoID.Int64, 10)
	return false, fmt.Errorf("%s %s is already managed by config repository %s", resourceKind, resourceID, owner)
}

type teamRecord struct {
	ID                 int
	Name               string
	Kind               string
	ParentID           *int
	Description        string
	RepoURL            string
	RepositoryFullName string
}

type teamRecordSet struct {
	byName map[string]*teamRecord
	byRepo map[string]*teamRecord
}

func loadExistingTeamRecords(ctx context.Context, runner queryRunner) (teamRecordSet, error) {
	rows, err := runner.Query(ctx, "SELECT id, name, COALESCE(kind, 'team'), parent_id, description, COALESCE(repo_url, ''), COALESCE(repository_full_name, '') FROM teams")
	if err != nil {
		return teamRecordSet{}, err
	}
	defer rows.Close()

	result := teamRecordSet{
		byName: make(map[string]*teamRecord),
		byRepo: make(map[string]*teamRecord),
	}
	for rows.Next() {
		var (
			record      teamRecord
			parentID    sql.NullInt32
			description sql.NullString
		)
		if err := rows.Scan(&record.ID, &record.Name, &record.Kind, &parentID, &description, &record.RepoURL, &record.RepositoryFullName); err != nil {
			return teamRecordSet{}, err
		}
		key, err := configsync.NormalizeStructureName(record.Name)
		if err != nil {
			return teamRecordSet{}, err
		}
		if _, exists := result.byName[key]; exists {
			return teamRecordSet{}, fmt.Errorf("duplicate team name '%s' detected in database", key)
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
				return teamRecordSet{}, fmt.Errorf("duplicate repository app '%s' detected in database", record.RepositoryFullName)
			}
			result.byRepo[repoKey] = &record
		}
	}
	if err := rows.Err(); err != nil {
		return teamRecordSet{}, err
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

func normalizePipelineRunStructureApp(app configsync.PipelineRunStructureApp) (configsync.PipelineRunStructureApp, error) {
	repoURL := strings.TrimSpace(app.RepoURL)
	fullName := strings.Trim(strings.TrimSpace(app.RepositoryFullName), "/")
	var err error
	if fullName == "" {
		if repoURL == "" {
			repoURL = strings.TrimSpace(app.Name)
		}
		fullName, err = configsync.RepositoryFullNameFromURL(repoURL)
		if err != nil {
			return configsync.PipelineRunStructureApp{}, err
		}
	}
	if repoURL == "" {
		repoURL = configsync.CanonicalRepositoryURL(fullName)
	}
	name := strings.TrimSpace(app.Name)
	if name == "" || strings.EqualFold(name, repoURL) {
		name = configsync.RepositoryDisplayNameFromFullName(fullName)
	}
	name, err = configsync.NormalizeStructureName(name)
	if err != nil {
		return configsync.PipelineRunStructureApp{}, err
	}
	return configsync.PipelineRunStructureApp{
		Name:               name,
		RepoURL:            repoURL,
		RepositoryFullName: fullName,
	}, nil
}

func (a *App) syncPipelineRunTeams(ctx context.Context, tx pgx.Tx, structure map[string]*configsync.PipelineRunStructureNode, details map[string]int) error {
	if len(structure) == 0 {
		return nil
	}

	existingTeams, err := loadExistingTeamRecords(ctx, tx)
	if err != nil {
		return fmt.Errorf("failed to load existing pipeline run teams: %w", err)
	}

	registerTeamRecord := func(record *teamRecord) {
		if record == nil {
			return
		}
		existingTeams.byName[record.Name] = record
		if record.RepositoryFullName != "" {
			existingTeams.byRepo[strings.ToLower(record.RepositoryFullName)] = record
		}
	}

	var ensureTeam func(name string, parentID *int, description string) (int, error)
	ensureTeam = func(name string, parentID *int, description string) (int, error) {
		normalized, err := configsync.NormalizeStructureName(name)
		if err != nil {
			return 0, err
		}
		description = strings.TrimSpace(description)
		if record, ok := existingTeams.byName[normalized]; ok {
			if record.Kind == "app" || record.RepositoryFullName != "" {
				return 0, fmt.Errorf("team '%s' conflicts with an existing app", normalized)
			}
			parentChanged := !parentPointersEqual(record.ParentID, parentID)
			descChanged := strings.TrimSpace(record.Description) != description
			kindChanged := record.Kind != "team" || record.RepoURL != "" || record.RepositoryFullName != ""
			if parentChanged || descChanged || kindChanged {
				if _, err := tx.Exec(ctx, "UPDATE teams SET kind = 'team', parent_id = $1, description = $2, repo_url = '', repository_full_name = '', updated_at = NOW() WHERE id = $3", parentID, description, record.ID); err != nil {
					return 0, fmt.Errorf("failed to update team '%s': %w", normalized, err)
				}
				delete(existingTeams.byRepo, strings.ToLower(record.RepositoryFullName))
				record.Kind = "team"
				record.ParentID = copyIntPointer(parentID)
				record.Description = description
				record.RepoURL = ""
				record.RepositoryFullName = ""
				details["run_teams_updated"]++
			}
			return record.ID, nil
		}

		var newID int
		if err := tx.QueryRow(ctx, "INSERT INTO teams (name, kind, parent_id, description) VALUES ($1, 'team', $2, $3) RETURNING id", normalized, parentID, description).Scan(&newID); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				refreshed, loadErr := loadExistingTeamRecords(ctx, tx)
				if loadErr != nil {
					return 0, fmt.Errorf("failed to reload teams after conflict: %w", loadErr)
				}
				existingTeams = refreshed
				if _, ok := existingTeams.byName[normalized]; ok {
					return ensureTeam(normalized, parentID, description)
				}
			}
			return 0, fmt.Errorf("failed to create team '%s': %w", normalized, err)
		}
		registerTeamRecord(&teamRecord{ID: newID, Name: normalized, Kind: "team", ParentID: copyIntPointer(parentID), Description: description})
		details["run_teams_created"]++
		return newID, nil
	}

	var ensureApp func(app configsync.PipelineRunStructureApp, parentID *int) (int, error)
	ensureApp = func(app configsync.PipelineRunStructureApp, parentID *int) (int, error) {
		normalizedApp, err := normalizePipelineRunStructureApp(app)
		if err != nil {
			return 0, err
		}
		name := normalizedApp.Name
		repoURL := normalizedApp.RepoURL
		fullName := normalizedApp.RepositoryFullName
		repoKey := strings.ToLower(fullName)

		record := existingTeams.byRepo[repoKey]
		if record == nil {
			if existingByName, ok := existingTeams.byName[name]; ok {
				if existingByName.Kind != "app" && existingByName.RepositoryFullName == "" {
					return 0, fmt.Errorf("app '%s' conflicts with an existing team", name)
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
				if _, err := tx.Exec(ctx, "UPDATE teams SET name = $1, kind = 'app', parent_id = $2, description = '', repo_url = $3, repository_full_name = $4, updated_at = NOW() WHERE id = $5", name, parentID, repoURL, fullName, record.ID); err != nil {
					return 0, fmt.Errorf("failed to update app '%s': %w", name, err)
				}
				delete(existingTeams.byName, record.Name)
				delete(existingTeams.byRepo, strings.ToLower(record.RepositoryFullName))
				record.Name = name
				record.Kind = "app"
				record.ParentID = copyIntPointer(parentID)
				record.Description = ""
				record.RepoURL = repoURL
				record.RepositoryFullName = fullName
				registerTeamRecord(record)
				details["run_teams_updated"]++
			}
			if _, err := reassignRepositoryRunsToApplication(ctx, tx, record.ID, parentID, fullName); err != nil {
				return 0, fmt.Errorf("failed to assign existing runs to app '%s': %w", name, err)
			}
			return record.ID, nil
		}

		var newID int
		if err := tx.QueryRow(ctx, "INSERT INTO teams (name, kind, parent_id, description, repo_url, repository_full_name) VALUES ($1, 'app', $2, '', $3, $4) RETURNING id", name, parentID, repoURL, fullName).Scan(&newID); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				refreshed, loadErr := loadExistingTeamRecords(ctx, tx)
				if loadErr != nil {
					return 0, fmt.Errorf("failed to reload apps after conflict: %w", loadErr)
				}
				existingTeams = refreshed
				return ensureApp(normalizedApp, parentID)
			}
			return 0, fmt.Errorf("failed to create app '%s': %w", name, err)
		}
		registerTeamRecord(&teamRecord{ID: newID, Name: name, Kind: "app", ParentID: copyIntPointer(parentID), RepoURL: repoURL, RepositoryFullName: fullName})
		if _, err := reassignRepositoryRunsToApplication(ctx, tx, newID, parentID, fullName); err != nil {
			return 0, fmt.Errorf("failed to assign existing runs to app '%s': %w", name, err)
		}
		details["run_teams_created"]++
		return newID, nil
	}

	var applyNode func(name string, node *configsync.PipelineRunStructureNode, parentID *int) error
	applyNode = func(name string, node *configsync.PipelineRunStructureNode, parentID *int) error {
		teamID, err := ensureTeam(name, parentID, node.Description)
		if err != nil {
			return err
		}
		apps := node.Apps
		if len(apps) == 0 {
			for _, repoName := range node.Repos {
				apps = append(apps, configsync.PipelineRunStructureApp{Name: repoName, RepoURL: repoName})
			}
		}
		for _, app := range apps {
			if _, err := ensureApp(app, &teamID); err != nil {
				return err
			}
		}
		for childName, childNode := range node.Children {
			if err := applyNode(childName, childNode, &teamID); err != nil {
				return err
			}
		}
		return nil
	}

	for name, node := range structure {
		if node == nil {
			node = &configsync.PipelineRunStructureNode{Children: map[string]*configsync.PipelineRunStructureNode{}}
		}
		if err := applyNode(name, node, nil); err != nil {
			return fmt.Errorf("failed to sync team '%s': %w", name, err)
		}
	}

	return nil
}

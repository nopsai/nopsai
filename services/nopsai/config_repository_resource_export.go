package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
)

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
		identifier := configsync.BuildPipelineIdentifier(pathPart, name)
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
		identifier := configsync.BuildStepIdentifier(pathPart, name)
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
		SELECT repository_name, trigger_definition, COALESCE(source, 'database'), COALESCE(team_path, ''), config_repo_id, managed_by_config_repo, config_source_path
		FROM triggers
		ORDER BY repository_name ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var identifier, definition, source, teamPath, sourcePath string
		var configRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(&identifier, &definition, &source, &teamPath, &configRepoID, &managed, &sourcePath); err != nil {
			return err
		}
		scope := repositoryTriggerConfigScope(repositoryTriggerRecord{RepositoryName: identifier, TeamPath: teamPath})
		if !configRepositoryIncludesResource(repo, scope, source, configRepoID, managed, delegatedScopes) {
			continue
		}
		filePath, ok := configRepositoryTriggerExportPath(repo, identifier, sourcePath, managed, configRepoID)
		if !ok {
			continue
		}
		files[filePath] = definition
	}
	return rows.Err()
}

func configRepositoryTriggerExportPath(repo models.ConfigRepository, repositoryName, sourcePath string, managed bool, configRepoID sql.NullInt64) (string, bool) {
	if managed && configRepoID.Valid && configRepoID.Int64 == repo.ID && strings.TrimSpace(sourcePath) != "" {
		return configRepositoryManagedSourcePath(repo, sourcePath)
	}
	identifier := strings.Trim(strings.TrimSpace(repositoryName), "/")
	if identifier == "" {
		return "", false
	}
	if repo.ScopeType == models.ConfigRepositoryScopeTeam {
		if rel, ok := configRepositoryRelativeResourceIdentifier(repo, identifier); ok && rel != "" {
			identifier = rel
		}
	}
	return filepath.ToSlash(filepath.Join("triggers", identifier+".yaml")), true
}

func (a *App) exportConfigRepositoryExternalTriggers(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	rows, err := a.db.Query(ctx, `
		SELECT id, name, description, enabled, pipeline, scope, COALESCE(run_team_path, ''), allowed_callers, variable_mapping,
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
			&trigger.RunTeamPath,
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
			RunTeamPath:     trigger.RunTeamPath,
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
	if configsync.ResourceUnderAnyScope(resourceScope, delegatedScopes) {
		return false
	}
	if managed && configRepoID.Valid {
		return configRepoID.Int64 == repo.ID
	}
	if !strings.EqualFold(strings.TrimSpace(source), "database") {
		return false
	}
	return configsync.ResourceUnderBindingScope(resourceScope, repo)
}

func configRepositoryIncludesGitWebhookSource(repo models.ConfigRepository, source gitWebhookSourceRecord, sourceType string, configRepoID sql.NullInt64, managed bool, delegatedScopes []string) bool {
	resourceScope := effectiveGitWebhookSourceTeamPath(source)
	if configsync.ResourceUnderAnyScope(resourceScope, delegatedScopes) {
		return false
	}
	if managed && configRepoID.Valid {
		return configRepoID.Int64 == repo.ID
	}
	if !strings.EqualFold(strings.TrimSpace(sourceType), "database") {
		return false
	}
	return configsync.ResourceUnderBindingScope(resourceScope, repo)
}

func (a *App) exportConfigRepositoryGitWebhookSources(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	rows, err := a.db.Query(ctx, `
		SELECT id, name, description, provider, enabled, COALESCE(team_path, ''), COALESCE(visibility, 'team'), auth_mode, credential_ref,
		       repository_allowlist, rate_limit, COALESCE(source, 'database'), config_repo_id,
		       managed_by_config_repo, COALESCE(config_source_path, '')
		FROM git_webhook_sources
		ORDER BY id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var source gitWebhookSourceRecord
		var allowlistJSON, rateLimitJSON []byte
		var sourceType, sourcePath string
		var configRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(
			&source.ID,
			&source.Name,
			&source.Description,
			&source.Provider,
			&source.Enabled,
			&source.TeamPath,
			&source.Visibility,
			&source.AuthMode,
			&source.CredentialRef,
			&allowlistJSON,
			&rateLimitJSON,
			&sourceType,
			&configRepoID,
			&managed,
			&sourcePath,
		); err != nil {
			return err
		}
		if !configRepositoryIncludesGitWebhookSource(repo, source, sourceType, configRepoID, managed, delegatedScopes) {
			continue
		}
		_ = decodeJSONWithDefault(allowlistJSON, &source.RepositoryAllowlist, []string{})
		_ = decodeJSONWithDefault(rateLimitJSON, &source.RateLimit, map[string]any{})
		filePath, ok := gitWebhookSourceExportPath(repo, source, sourcePath, managed, configRepoID.Valid, configRepoID.Int64)
		if !ok {
			continue
		}
		enabled := source.Enabled
		doc := gitWebhookSourceGitOpsDocument{
			ID:                  source.ID,
			Name:                source.Name,
			Description:         source.Description,
			Provider:            source.Provider,
			Enabled:             &enabled,
			TeamPath:            source.TeamPath,
			Visibility:          source.Visibility,
			AuthMode:            source.AuthMode,
			CredentialRef:       source.CredentialRef,
			RepositoryAllowlist: source.RepositoryAllowlist,
			RateLimit:           nilIfEmptyAnyMap(source.RateLimit),
		}
		content, err := marshalConfigRepositoryYAML(doc)
		if err != nil {
			return err
		}
		files[filePath] = string(content)
	}
	return rows.Err()
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
	RunTeamPath    string            `yaml:"run_team_path,omitempty"`
	Variables      map[string]string `yaml:"variables,omitempty"`
}

func (a *App) exportConfigRepositorySchedules(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	rows, err := a.db.Query(ctx, `
		SELECT path, name, description, pipeline_path, pipeline_name,
		       COALESCE(schedule_kind, 'cron'), cron_expression, run_at, timezone, enabled, scope, COALESCE(run_team_path, ''), variables::text,
		       COALESCE(source, 'database'), config_repo_id, managed_by_config_repo, config_source_path
		FROM pipeline_schedules
		ORDER BY path ASC, name ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var pathPart, name, description, pipelinePath, pipelineName, scheduleKind, cronExpression, timezone, scope, runTeamPath, variablesRaw, source, sourcePath string
		var runAt sql.NullTime
		var enabled bool
		var configRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(&pathPart, &name, &description, &pipelinePath, &pipelineName, &scheduleKind, &cronExpression, &runAt, &timezone, &enabled, &scope, &runTeamPath, &variablesRaw, &source, &configRepoID, &managed, &sourcePath); err != nil {
			return err
		}
		identifier := configsync.BuildPipelineIdentifier(pathPart, name)
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
			Pipeline:    configsync.BuildPipelineIdentifier(pipelinePath, pipelineName),
			Timezone:    timezone,
			Enabled:     enabled,
			Scope:       scope,
			RunTeamPath: runTeamPath,
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

func (a *App) exportConfigRepositoryNotificationRoutes(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	teamRecords, err := loadTeamPathRecords(ctx, a.db)
	if err != nil {
		return fmt.Errorf("failed to resolve notification team paths: %w", err)
	}

	rows, err := a.db.Query(ctx, `
		SELECT nr.definition::text, COALESCE(nr.source, 'database'), nr.config_repo_id,
		       nr.managed_by_config_repo, COALESCE(nr.config_source_path, ''), nr.team_id
		FROM notification_routes nr
		ORDER BY nr.team_id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var definitionRaw, source, sourcePath string
		var configRepoID sql.NullInt64
		var managed bool
		var teamID int
		if err := rows.Scan(&definitionRaw, &source, &configRepoID, &managed, &sourcePath, &teamID); err != nil {
			return err
		}
		teamPath, err := notificationRouteTeamPath(teamRecords, teamID)
		if err != nil {
			return err
		}
		if !configRepositoryIncludesResource(repo, teamPath, source, configRepoID, managed, delegatedScopes) {
			continue
		}
		filePath, ok := configRepositoryNotificationRoutePath(repo, teamPath, sourcePath, managed, configRepoID)
		if !ok {
			continue
		}
		var definition notificationRouteDefinition
		if err := json.Unmarshal([]byte(definitionRaw), &definition); err != nil {
			return err
		}
		normalized, err := normalizeNotificationRouteDefinition(notificationRouteDefinitionFileFromDefinition(definition))
		if err != nil {
			return err
		}
		content, err := marshalConfigRepositoryYAML(normalized)
		if err != nil {
			return err
		}
		files[filePath] = string(content)
	}
	return rows.Err()
}

func notificationRouteTeamPath(records map[int]teamPathRecord, teamID int) (string, error) {
	record, ok := records[teamID]
	if !ok {
		return "", fmt.Errorf("notification route references unknown team %d", teamID)
	}
	teamPath := strings.Trim(strings.TrimSpace(record.Path), "/")
	if teamPath == "" {
		return "", fmt.Errorf("notification route team %d has no resolved path", teamID)
	}
	return teamPath, nil
}

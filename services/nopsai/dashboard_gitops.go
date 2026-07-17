package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
)

type dashboardGitOpsDocument struct {
	Title            string                           `yaml:"title"`
	Description      string                           `yaml:"description,omitempty"`
	Visibility       string                           `yaml:"visibility,omitempty"`
	RefreshPolicy    map[string]any                   `yaml:"refresh_policy,omitempty"`
	Sections         []dashboardGitOpsSection         `yaml:"sections,omitempty"`
	Sources          []dashboardGitOpsSource          `yaml:"sources,omitempty"`
	RefreshSchedules []dashboardGitOpsRefreshSchedule `yaml:"refresh_schedules,omitempty"`
}

type dashboardGitOpsSection struct {
	SectionKey   string         `yaml:"section_key"`
	Title        string         `yaml:"title,omitempty"`
	Description  string         `yaml:"description,omitempty"`
	Layout       map[string]any `yaml:"layout,omitempty"`
	DisplayOrder int            `yaml:"display_order,omitempty"`
}

type dashboardGitOpsSource struct {
	SectionKey         string `yaml:"section_key"`
	PipelineID         string `yaml:"pipeline_id"`
	OutputName         string `yaml:"output_name"`
	EntryKey           string `yaml:"entry_key,omitempty"`
	Enabled            *bool  `yaml:"enabled,omitempty"`
	RequiredForRefresh *bool  `yaml:"required_for_refresh,omitempty"`
	RefreshOrder       int    `yaml:"refresh_order,omitempty"`
}

type dashboardGitOpsRefreshSchedule struct {
	Name           string                       `yaml:"name"`
	Description    string                       `yaml:"description,omitempty"`
	CronExpression string                       `yaml:"cron_expression"`
	Timezone       string                       `yaml:"timezone,omitempty"`
	Enabled        *bool                        `yaml:"enabled,omitempty"`
	Scope          dashboardRefreshScopeRequest `yaml:"scope,omitempty"`
	Mode           string                       `yaml:"mode,omitempty"`
	RunScope       string                       `yaml:"run_scope,omitempty"`
	Variables      map[string]string            `yaml:"variables,omitempty"`
	MaxConcurrency int                          `yaml:"max_concurrency,omitempty"`
	Timeout        string                       `yaml:"timeout,omitempty"`
}

type storedDashboard struct {
	teamPath   string
	slug       string
	input      dashboardInput
	sources    []dashboardSourceInput
	schedules  []dashboardRefreshScheduleInput
	sourcePath string
}

func parseGitOpsDashboards(files map[string]string, dashboardDir string, binding models.ConfigRepository, boundTeam string) (map[string]storedDashboard, error) {
	dashboards := map[string]storedDashboard{}
	for path, content := range files {
		normalized := filepath.ToSlash(path)
		rel, ok := configsync.RelativePath(normalized, dashboardDir)
		if !ok {
			continue
		}
		if rel == "" || strings.HasSuffix(rel, "/") || !isYAMLFile(rel) {
			continue
		}
		identifier := strings.TrimSuffix(rel, filepath.Ext(rel))
		if binding.ScopeType == models.ConfigRepositoryScopeTeam {
			targetID, err := configsync.NormalizePathForTeam(boundTeam, rel)
			if err != nil {
				return nil, fmt.Errorf("invalid team-scoped dashboard path '%s': %w", normalized, err)
			}
			identifier = strings.TrimSuffix(targetID, filepath.Ext(targetID))
		}
		teamPath, slug, err := splitDashboardRef(identifier)
		if err != nil {
			return nil, fmt.Errorf("invalid dashboard path '%s': %w", normalized, err)
		}
		var doc dashboardGitOpsDocument
		if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
			return nil, fmt.Errorf("failed to parse dashboard '%s': %w", normalized, err)
		}
		sections := make([]dashboardSectionInput, 0, len(doc.Sections))
		for _, section := range doc.Sections {
			input, err := normalizeDashboardSectionInput(dashboardSectionRequest{
				SectionKey:   section.SectionKey,
				Title:        section.Title,
				Description:  section.Description,
				Layout:       section.Layout,
				DisplayOrder: section.DisplayOrder,
			})
			if err != nil {
				return nil, fmt.Errorf("invalid dashboard section '%s': %w", normalized, err)
			}
			sections = append(sections, input)
		}
		if len(sections) == 0 {
			sections = []dashboardSectionInput{{SectionKey: "overview", Title: "Overview", Layout: map[string]any{}}}
		}
		sources := make([]dashboardSourceInput, 0, len(doc.Sources))
		for _, source := range doc.Sources {
			input, err := normalizeDashboardSourceInput(dashboardSourceRequest{
				SectionKey:         source.SectionKey,
				PipelineID:         source.PipelineID,
				OutputName:         source.OutputName,
				EntryKey:           source.EntryKey,
				Enabled:            source.Enabled,
				RequiredForRefresh: source.RequiredForRefresh,
				RefreshOrder:       source.RefreshOrder,
			})
			if err != nil {
				return nil, fmt.Errorf("invalid dashboard source '%s': %w", normalized, err)
			}
			sources = append(sources, input)
		}
		request := dashboardRequest{
			TeamPath:      teamPath,
			Slug:          slug,
			Title:         doc.Title,
			Description:   doc.Description,
			Visibility:    doc.Visibility,
			RefreshPolicy: doc.RefreshPolicy,
		}
		if strings.TrimSpace(request.Title) == "" {
			return nil, fmt.Errorf("dashboard '%s' title is required", normalized)
		}
		preview, err := normalizeDashboardInput(request, 1, teamPath)
		if err != nil {
			return nil, fmt.Errorf("invalid dashboard '%s': %w", normalized, err)
		}
		schedules := make([]dashboardRefreshScheduleInput, 0, len(doc.RefreshSchedules))
		for _, schedule := range doc.RefreshSchedules {
			input, err := normalizeDashboardRefreshScheduleInput(dashboardRefreshScheduleRequest{
				Name:           schedule.Name,
				Description:    schedule.Description,
				CronExpression: schedule.CronExpression,
				Timezone:       schedule.Timezone,
				Enabled:        schedule.Enabled,
				Scope:          schedule.Scope,
				Mode:           schedule.Mode,
				RunScope:       schedule.RunScope,
				Variables:      schedule.Variables,
				MaxConcurrency: schedule.MaxConcurrency,
				Timeout:        schedule.Timeout,
			}, preview.RefreshPolicy)
			if err != nil {
				return nil, fmt.Errorf("invalid dashboard refresh schedule '%s': %w", normalized, err)
			}
			schedules = append(schedules, input)
		}
		key := dashboardResourceID(teamPath, slug)
		if _, exists := dashboards[key]; exists {
			return nil, fmt.Errorf("duplicate dashboard '%s' detected in config repository", key)
		}
		preview.Sections = sections
		dashboards[key] = storedDashboard{
			teamPath:   teamPath,
			slug:       slug,
			input:      preview,
			sources:    sources,
			schedules:  schedules,
			sourcePath: normalized,
		}
	}
	return dashboards, nil
}

func (a *App) applyGitOpsDashboards(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, dashboards map[string]storedDashboard, commitSHA string, details map[string]int) error {
	if len(dashboards) == 0 {
		return nil
	}
	teamRecords, err := loadTeamPathRecords(ctx, tx)
	if err != nil {
		return err
	}
	teamIDs := map[string]int{}
	for _, record := range teamRecords {
		teamIDs[record.Path] = record.ID
	}
	for key, stored := range dashboards {
		teamID := teamIDs[stored.teamPath]
		if teamID == 0 {
			return fmt.Errorf("dashboard '%s' references missing team '%s'", key, stored.teamPath)
		}
		writable, err := ensureConfigResourceWritable(ctx, tx, "dashboards", "dashboard", key, binding, stored.teamPath, "team_id = $1 AND slug = $2", teamID, stored.slug)
		if err != nil {
			return err
		}
		if !writable {
			continue
		}
		refreshPolicyJSON, err := json.Marshal(stored.input.RefreshPolicy)
		if err != nil {
			return fmt.Errorf("failed to marshal dashboard refresh policy '%s': %w", key, err)
		}
		var dashboardID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO dashboards (
				team_id, slug, title, description, visibility, refresh_policy, source,
				config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo,
				created_by, updated_by, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6::jsonb, 'git',
				$7, $8, $9, TRUE,
				'gitops', 'gitops', NOW()
			)
			ON CONFLICT (team_id, slug) DO UPDATE SET
				title = EXCLUDED.title,
				description = EXCLUDED.description,
				visibility = EXCLUDED.visibility,
				refresh_policy = EXCLUDED.refresh_policy,
				source = 'git',
				config_repo_id = EXCLUDED.config_repo_id,
				config_source_path = EXCLUDED.config_source_path,
				config_source_commit_sha = EXCLUDED.config_source_commit_sha,
				managed_by_config_repo = TRUE,
				updated_by = EXCLUDED.updated_by,
				updated_at = NOW()
			RETURNING id::text
		`, teamID, stored.slug, stored.input.Title, stored.input.Description, stored.input.Visibility, string(refreshPolicyJSON),
			binding.ID, stored.sourcePath, commitSHA).Scan(&dashboardID); err != nil {
			return fmt.Errorf("failed to upsert dashboard '%s': %w", key, err)
		}
		for _, section := range stored.input.Sections {
			if err := upsertDashboardSection(ctx, tx, dashboardID, section); err != nil {
				return fmt.Errorf("failed to upsert dashboard section '%s': %w", key, err)
			}
		}
		for _, source := range stored.sources {
			if err := upsertDashboardSourceBinding(ctx, tx, dashboardID, source); err != nil {
				return fmt.Errorf("failed to upsert dashboard source '%s': %w", key, err)
			}
		}
		dashboard := dashboardRecord{ID: dashboardID, TeamID: teamID, TeamPath: stored.teamPath, Slug: stored.slug}
		if err := upsertGitOpsDashboardRefreshSchedules(ctx, tx, binding, dashboard, stored.schedules, stored.sourcePath, commitSHA); err != nil {
			return fmt.Errorf("failed to upsert dashboard refresh schedules '%s': %w", key, err)
		}
		details["dashboards_synced"]++
	}
	return nil
}

func upsertGitOpsDashboardRefreshSchedules(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, dashboard dashboardRecord, schedules []dashboardRefreshScheduleInput, sourcePath, commitSHA string) error {
	seen := make([]string, 0, len(schedules))
	for _, schedule := range schedules {
		scopeJSON, err := json.Marshal(schedule.Refresh.Scope)
		if err != nil {
			return err
		}
		variablesJSON, err := json.Marshal(schedule.Refresh.Variables)
		if err != nil {
			return err
		}
		var scheduleID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO dashboard_refresh_schedules (
				dashboard_id, name, description, cron_expression, timezone, enabled,
				scope_type, scope, mode, run_scope, variables, max_concurrency, timeout_seconds,
				next_run_at, source, config_repo_id, config_source_path, config_source_commit_sha,
				managed_by_config_repo, created_by, updated_by, updated_at
			) VALUES (
				$1::uuid, $2, $3, $4, $5, $6,
				$7, $8::jsonb, $9, $10, $11::jsonb, $12, $13,
				$14, 'git', $15, $16, $17,
				TRUE, 'gitops', 'gitops', NOW()
			)
			ON CONFLICT (dashboard_id, name) DO UPDATE SET
				description = EXCLUDED.description,
				cron_expression = EXCLUDED.cron_expression,
				timezone = EXCLUDED.timezone,
				enabled = EXCLUDED.enabled,
				scope_type = EXCLUDED.scope_type,
				scope = EXCLUDED.scope,
				mode = EXCLUDED.mode,
				run_scope = EXCLUDED.run_scope,
				variables = EXCLUDED.variables,
				max_concurrency = EXCLUDED.max_concurrency,
				timeout_seconds = EXCLUDED.timeout_seconds,
				next_run_at = EXCLUDED.next_run_at,
				source = 'git',
				config_repo_id = EXCLUDED.config_repo_id,
				config_source_path = EXCLUDED.config_source_path,
				config_source_commit_sha = EXCLUDED.config_source_commit_sha,
				managed_by_config_repo = TRUE,
				updated_by = EXCLUDED.updated_by,
				updated_at = NOW()
			RETURNING id::text
		`, dashboard.ID, schedule.Name, schedule.Description, schedule.CronExpression, schedule.Timezone, schedule.Enabled,
			schedule.Refresh.ScopeType, string(scopeJSON), schedule.Refresh.Mode, schedule.Refresh.RunScope, string(variablesJSON),
			schedule.Refresh.MaxConcurrency, int(schedule.Refresh.Timeout.Seconds()), schedule.NextRunAt,
			binding.ID, sourcePath, commitSHA).Scan(&scheduleID); err != nil {
			return err
		}
		serviceAccountID := dashboardRefreshScheduleServiceAccountID(scheduleID)
		if _, err := tx.Exec(ctx, `
			UPDATE dashboard_refresh_schedules
			SET service_account_id = CASE WHEN service_account_id = '' THEN $2 ELSE service_account_id END
			WHERE id::text = $1
		`, scheduleID, serviceAccountID); err != nil {
			return err
		}
		if err := ensureDashboardRefreshScheduleACLs(ctx, tx, dashboard, serviceAccountID, schedule.Refresh); err != nil {
			return err
		}
		seen = append(seen, schedule.Name)
	}
	if len(schedules) > 0 {
		if _, err := tx.Exec(ctx, `
			DELETE FROM dashboard_refresh_schedules
			WHERE dashboard_id::text = $1
			  AND managed_by_config_repo = TRUE
			  AND config_repo_id = $2
			  AND name != ALL($3)
		`, dashboard.ID, binding.ID, seen); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) exportConfigRepositoryDashboards(
	ctx context.Context,
	repo models.ConfigRepository,
	delegatedScopes []string,
	resourceAccess map[resourceAccessPlanKey]configRepositoryResourceAccessState,
	files map[string]string,
) error {
	records, err := a.listDashboardRecords(ctx, "", "")
	if err != nil {
		return err
	}
	for _, record := range records {
		if !configRepositoryIncludesResource(repo, record.ref(), record.Source, record.ConfigRepoID, record.ManagedByConfigRepo, delegatedScopes) {
			continue
		}
		filePath, ok := configRepositoryExportPath(repo, record.ref(), record.ConfigSourcePath, "dashboards", ".yaml", record.ManagedByConfigRepo, record.ConfigRepoID)
		if !ok {
			continue
		}
		sections, err := a.listDashboardSections(ctx, record.ID)
		if err != nil {
			return err
		}
		sources, err := a.listDashboardSources(ctx, record.ID)
		if err != nil {
			return err
		}
		schedules, err := a.listDashboardRefreshScheduleRecords(ctx, record)
		if err != nil {
			return err
		}
		resourceID := record.ref()
		doc := dashboardGitOpsDocument{
			Title:            record.Title,
			Description:      record.Description,
			Visibility:       record.Visibility,
			RefreshPolicy:    nilIfEmptyAnyMap(record.RefreshPolicy),
			Sections:         make([]dashboardGitOpsSection, 0, len(sections)),
			Sources:          make([]dashboardGitOpsSource, 0, len(sources)),
			RefreshSchedules: make([]dashboardGitOpsRefreshSchedule, 0, len(schedules)),
		}
		for _, section := range sections {
			doc.Sections = append(doc.Sections, dashboardGitOpsSection{
				SectionKey:   section.SectionKey,
				Title:        section.Title,
				Description:  section.Description,
				Layout:       nilIfEmptyAnyMap(section.Layout),
				DisplayOrder: section.DisplayOrder,
			})
		}
		for _, source := range sources {
			enabled := source.Enabled
			required := source.RequiredForRefresh
			doc.Sources = append(doc.Sources, dashboardGitOpsSource{
				SectionKey:         source.SectionKey,
				PipelineID:         source.PipelineID,
				OutputName:         source.OutputName,
				EntryKey:           source.EntryKey,
				Enabled:            &enabled,
				RequiredForRefresh: &required,
				RefreshOrder:       source.RefreshOrder,
			})
		}
		for _, schedule := range schedules {
			enabled := schedule.Enabled
			doc.RefreshSchedules = append(doc.RefreshSchedules, dashboardGitOpsRefreshSchedule{
				Name:           schedule.Name,
				Description:    schedule.Description,
				CronExpression: schedule.CronExpression,
				Timezone:       schedule.Timezone,
				Enabled:        &enabled,
				Scope: dashboardRefreshScopeRequest{
					Type:        schedule.ScopeType,
					SectionKeys: stringSliceFromAny(schedule.Scope["section_keys"]),
					SourceIDs:   stringSliceFromAny(schedule.Scope["source_ids"]),
				},
				Mode:           schedule.Mode,
				RunScope:       schedule.RunScope,
				Variables:      nilIfEmptyStringMap(schedule.Variables),
				MaxConcurrency: schedule.MaxConcurrency,
				Timeout:        fmt.Sprintf("%ds", schedule.TimeoutSeconds),
			})
		}
		content, err := marshalConfigRepositoryYAML(doc)
		if err != nil {
			return err
		}
		definition := string(content)
		if access, ok := resourceAccess[resourceAccessPlanKey{resourceType: grantResourceDashboard, resourceID: resourceID}]; ok && (access.Override || !record.ManagedByConfigRepo) {
			definition, err = syncConfigRepositoryYAMLAccessBlock(definition, access.exportFile())
			if err != nil {
				return fmt.Errorf("failed to render dashboard access for %s: %w", resourceID, err)
			}
		}
		files[filePath] = definition
	}
	return nil
}

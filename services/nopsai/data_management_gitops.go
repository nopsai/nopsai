package nopsai

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
)

const dataManagementGitOpsPath = "system/data-management.yaml"

type gitOpsDataManagementPlan struct {
	schedules  map[string]storedDataCleanupSchedule
	sourcePath string
}

type storedDataCleanupSchedule struct {
	input      dataCleanupScheduleInput
	sourcePath string
}

type dataManagementGitOpsFile struct {
	CleanupSchedules []dataCleanupScheduleGitOpsFile `json:"cleanup_schedules" yaml:"cleanup_schedules"`
}

type dataCleanupScheduleGitOpsFile struct {
	Name                string `json:"name" yaml:"name"`
	Description         string `json:"description,omitempty" yaml:"description,omitempty"`
	Enabled             *bool  `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Target              string `json:"target" yaml:"target"`
	Mode                string `json:"mode" yaml:"mode"`
	KeepLast            int    `json:"keep_last,omitempty" yaml:"keep_last,omitempty"`
	OlderThanDays       int    `json:"older_than_days,omitempty" yaml:"older_than_days,omitempty"`
	BackupBeforeCleanup *bool  `json:"backup_before_cleanup,omitempty" yaml:"backup_before_cleanup,omitempty"`
	CronExpression      string `json:"cron_expression" yaml:"cron_expression"`
	Timezone            string `json:"timezone,omitempty" yaml:"timezone,omitempty"`
}

func parseGitOpsDataManagementPlan(binding models.ConfigRepository, directories ...gitOpsRuntimeSettingsDirectory) (*gitOpsDataManagementPlan, error) {
	var candidates []gitOpsRuntimeSettingsFileCandidate
	for _, directory := range directories {
		root := filepath.ToSlash(strings.Trim(directory.root, "/"))
		for path, content := range directory.files {
			normalized := filepath.ToSlash(path)
			rel, ok := configsync.RelativePath(normalized, root)
			if !ok || !isGitOpsDataManagementRelativePath(rel) {
				continue
			}
			candidates = append(candidates, gitOpsRuntimeSettingsFileCandidate{sourcePath: normalized, content: content})
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	if binding.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil, fmt.Errorf("data management settings can only be configured from a system config repository")
	}
	if len(candidates) > 1 {
		paths := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			paths = append(paths, candidate.sourcePath)
		}
		sort.Strings(paths)
		return nil, fmt.Errorf("multiple data management GitOps files found: %s", strings.Join(paths, ", "))
	}
	return parseGitOpsDataManagementFile(candidates[0].content, candidates[0].sourcePath)
}

func isGitOpsDataManagementRelativePath(rel string) bool {
	return strings.Trim(filepath.ToSlash(rel), "/") == dataManagementGitOpsPath
}

func parseGitOpsDataManagementFile(content, sourcePath string) (*gitOpsDataManagementPlan, error) {
	var file dataManagementGitOpsFile
	if err := yaml.Unmarshal([]byte(content), &file); err != nil {
		return nil, fmt.Errorf("failed to parse data management GitOps file '%s': %w", sourcePath, err)
	}
	plan := &gitOpsDataManagementPlan{
		schedules:  map[string]storedDataCleanupSchedule{},
		sourcePath: sourcePath,
	}
	for _, schedule := range file.CleanupSchedules {
		if strings.TrimSpace(schedule.Name) == "" {
			return nil, fmt.Errorf("data cleanup schedule name is required in %s", sourcePath)
		}
		input, err := normalizeDataCleanupScheduleInput(dataCleanupScheduleRequest{
			Name:                schedule.Name,
			Description:         schedule.Description,
			Enabled:             schedule.Enabled,
			Target:              schedule.Target,
			Mode:                schedule.Mode,
			KeepLast:            schedule.KeepLast,
			OlderThanDays:       schedule.OlderThanDays,
			BackupBeforeCleanup: schedule.BackupBeforeCleanup,
			CronExpression:      schedule.CronExpression,
			Timezone:            schedule.Timezone,
		})
		if err != nil {
			return nil, fmt.Errorf("invalid data cleanup schedule %q in '%s': %w", schedule.Name, sourcePath, err)
		}
		key := input.Name
		if _, exists := plan.schedules[key]; exists {
			return nil, fmt.Errorf("duplicate data cleanup schedule %q detected in %s", key, sourcePath)
		}
		plan.schedules[key] = storedDataCleanupSchedule{input: input, sourcePath: sourcePath}
	}
	return plan, nil
}

func buildDataManagementGitOpsFile(records []dataCleanupScheduleRecord) dataManagementGitOpsFile {
	sort.Slice(records, func(i, j int) bool {
		return records[i].Name < records[j].Name
	})
	doc := dataManagementGitOpsFile{CleanupSchedules: []dataCleanupScheduleGitOpsFile{}}
	for _, record := range records {
		enabled := record.Enabled
		backup := record.BackupBeforeCleanup
		doc.CleanupSchedules = append(doc.CleanupSchedules, dataCleanupScheduleGitOpsFile{
			Name:                record.Name,
			Description:         strings.TrimSpace(record.Description),
			Enabled:             &enabled,
			Target:              record.Target,
			Mode:                record.Mode,
			KeepLast:            record.KeepLast,
			OlderThanDays:       record.OlderThanDays,
			BackupBeforeCleanup: &backup,
			CronExpression:      record.CronExpression,
			Timezone:            record.Timezone,
		})
	}
	return doc
}

func sortedDataCleanupSchedules(schedules map[string]storedDataCleanupSchedule) []storedDataCleanupSchedule {
	keys := make([]string, 0, len(schedules))
	for key := range schedules {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]storedDataCleanupSchedule, 0, len(keys))
	for _, key := range keys {
		result = append(result, schedules[key])
	}
	return result
}

func upsertDataCleanupScheduleFromGitOps(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, stored storedDataCleanupSchedule, commitSHA string) error {
	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM data_cleanup_schedules WHERE name = $1`, stored.input.Name).Scan(&count); err != nil {
		return fmt.Errorf("failed to inspect data cleanup schedule %q: %w", stored.input.Name, err)
	}
	if count > 1 {
		return fmt.Errorf("data cleanup schedule %q matches multiple database rows; rename duplicates before GitOps can manage it", stored.input.Name)
	}
	writable, err := ensureConfigResourceWritable(ctx, tx, "data_cleanup_schedules", "data cleanup schedule", stored.input.Name, binding, models.ConfigRepositorySystemGlobalID, "name = $1", stored.input.Name)
	if err != nil {
		return err
	}
	if !writable {
		return nil
	}
	if count == 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO data_cleanup_schedules (
				name, description, enabled, target, mode, keep_last, older_than_days,
				backup_before_cleanup, cron_expression, timezone, next_run_at, source,
				config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo,
				created_by, updated_by, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7,
				$8, $9, $10, $11, 'git',
				$12, $13, $14, TRUE,
				'config-repo', 'config-repo', NOW()
			)
		`, stored.input.Name, stored.input.Description, stored.input.Enabled, stored.input.Plan.Target, stored.input.Plan.Mode,
			stored.input.Plan.KeepLast, stored.input.Plan.OlderThanDays, stored.input.Plan.BackupBeforeCleanup,
			stored.input.CronExpression, stored.input.Timezone, stored.input.NextRunAt,
			binding.ID, stored.sourcePath, commitSHA); err != nil {
			return fmt.Errorf("failed to insert data cleanup schedule %q: %w", stored.input.Name, err)
		}
		return nil
	}
	tag, err := tx.Exec(ctx, `
		UPDATE data_cleanup_schedules
		SET description = $2,
			enabled = $3,
			target = $4,
			mode = $5,
			keep_last = $6,
			older_than_days = $7,
			backup_before_cleanup = $8,
			cron_expression = $9,
			timezone = $10,
			next_run_at = $11,
			source = 'git',
			config_repo_id = $12,
			config_source_path = $13,
			config_source_commit_sha = $14,
			managed_by_config_repo = TRUE,
			updated_by = 'config-repo',
			updated_at = NOW()
		WHERE name = $1
	`, stored.input.Name, stored.input.Description, stored.input.Enabled, stored.input.Plan.Target, stored.input.Plan.Mode,
		stored.input.Plan.KeepLast, stored.input.Plan.OlderThanDays, stored.input.Plan.BackupBeforeCleanup,
		stored.input.CronExpression, stored.input.Timezone, stored.input.NextRunAt,
		binding.ID, stored.sourcePath, commitSHA)
	if err != nil {
		return fmt.Errorf("failed to update data cleanup schedule %q: %w", stored.input.Name, err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func pruneGitOpsDataCleanupSchedules(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, schedules map[string]storedDataCleanupSchedule) error {
	names := make([]string, 0, len(schedules))
	for name := range schedules {
		names = append(names, name)
	}
	if len(names) == 0 {
		if _, err := tx.Exec(ctx, "DELETE FROM data_cleanup_schedules WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID); err != nil {
			return fmt.Errorf("failed to prune data cleanup schedules: %w", err)
		}
		return nil
	}
	if _, err := tx.Exec(ctx, "DELETE FROM data_cleanup_schedules WHERE managed_by_config_repo = TRUE AND config_repo_id = $2 AND name != ALL($1)", names, binding.ID); err != nil {
		return fmt.Errorf("failed to prune data cleanup schedules: %w", err)
	}
	return nil
}

func (a *App) exportConfigRepositoryDataManagement(ctx context.Context, repo models.ConfigRepository, files map[string]string) error {
	if repo.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil
	}
	records, err := a.listDataCleanupSchedules(ctx)
	if err != nil {
		return err
	}
	filtered := make([]dataCleanupScheduleRecord, 0, len(records))
	for _, record := range records {
		if dataCleanupScheduleIncludedInConfigRepository(repo, record) {
			filtered = append(filtered, record)
		}
	}
	content, err := marshalConfigRepositoryYAML(buildDataManagementGitOpsFile(filtered))
	if err != nil {
		return err
	}
	files[configRepositoryDataManagementPath] = string(content)
	return nil
}

func dataCleanupScheduleIncludedInConfigRepository(repo models.ConfigRepository, record dataCleanupScheduleRecord) bool {
	configRepoID := sql.NullInt64{}
	if record.ConfigRepoID != nil {
		configRepoID = sql.NullInt64{Int64: *record.ConfigRepoID, Valid: true}
	}
	return configRepositoryIncludesResource(repo, models.ConfigRepositorySystemGlobalID, record.Source, configRepoID, record.ManagedByConfigRepo, nil)
}

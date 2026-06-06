package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
	aaamodel "nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
)

func (a *App) loadSchedulePipeline(ctx context.Context, pipelinePath, pipelineName string) (models.Pipeline, []byte, error) {
	var definition string
	if err := a.db.QueryRow(ctx, `SELECT definition FROM pipelines WHERE path = $1 AND name = $2`, pipelinePath, pipelineName).Scan(&definition); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return models.Pipeline{}, nil, fmt.Errorf("pipeline not found")
		}
		return models.Pipeline{}, nil, fmt.Errorf("failed to load pipeline: %w", err)
	}
	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(definition), &pipeline); err != nil {
		return models.Pipeline{}, nil, fmt.Errorf("pipeline YAML is malformed: %w", err)
	}
	pipeline.Name = sanitizeInput(pipeline.Name)
	pipeline.Version = normalizePipelineVersion(pipeline.Version)
	if pipeline.Name == "" {
		pipeline.Name = pipelineName
	}
	return pipeline, []byte(definition), nil
}

func loadSchedulePipelineFromSync(ctx context.Context, runner queryRunner, input scheduleInput, pipelines map[string]storedPipeline) (models.Pipeline, error) {
	pipelineID := configsync.BuildPipelineIdentifier(input.PipelinePath, input.PipelineName)
	if stored, ok := pipelines[pipelineID]; ok {
		var pipeline models.Pipeline
		if err := yaml.Unmarshal([]byte(stored.definition), &pipeline); err != nil {
			return models.Pipeline{}, err
		}
		pipeline.Name = sanitizeInput(pipeline.Name)
		pipeline.Version = normalizePipelineVersion(pipeline.Version)
		if pipeline.Name == "" {
			pipeline.Name = input.PipelineName
		}
		return pipeline, nil
	}
	var definition string
	if err := runner.QueryRow(ctx, `SELECT definition FROM pipelines WHERE path = $1 AND name = $2`, input.PipelinePath, input.PipelineName).Scan(&definition); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return models.Pipeline{}, fmt.Errorf("pipeline %q not found", pipelineID)
		}
		return models.Pipeline{}, err
	}
	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(definition), &pipeline); err != nil {
		return models.Pipeline{}, err
	}
	pipeline.Name = sanitizeInput(pipeline.Name)
	pipeline.Version = normalizePipelineVersion(pipeline.Version)
	if pipeline.Name == "" {
		pipeline.Name = input.PipelineName
	}
	return pipeline, nil
}

func (a *App) createSchedule(ctx context.Context, input scheduleInput, pipeline models.Pipeline, actor, source, sourcePath, commitSHA string) (scheduleRecord, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return scheduleRecord{}, err
	}
	defer tx.Rollback(ctx)

	variablesJSON, err := json.Marshal(input.Variables)
	if err != nil {
		return scheduleRecord{}, err
	}
	var scheduleID string
	err = tx.QueryRow(ctx, `
		INSERT INTO pipeline_schedules (
			path, name, description, pipeline_path, pipeline_name, pipeline_version,
			schedule_kind, cron_expression, run_at, timezone, enabled, scope, variables, next_run_at,
			run_group_path, source, config_source_path, config_source_commit_sha, managed_by_config_repo,
			created_by, updated_by
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12, $13::jsonb, $14,
			$15, $16, $17, $18, $19,
			$20, $20
		)
		RETURNING id::text
	`, input.Path, input.Name, input.Description, input.PipelinePath, input.PipelineName, input.PipelineVersion,
		input.ScheduleKind, input.CronExpression, input.RunAt, input.Timezone, input.Enabled, input.Scope, string(variablesJSON), input.NextRunAt,
		input.RunGroupPath, source, sourcePath, commitSHA, strings.EqualFold(source, "git"), actor).Scan(&scheduleID)
	if err != nil {
		return scheduleRecord{}, err
	}
	if err := ensureScheduleExecutionACLs(ctx, tx, scheduleID, input, pipeline); err != nil {
		return scheduleRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return scheduleRecord{}, err
	}
	return a.getScheduleRecord(ctx, scheduleID)
}

func (a *App) updateSchedule(ctx context.Context, scheduleID string, input scheduleInput, pipeline models.Pipeline, actor string) (scheduleRecord, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return scheduleRecord{}, err
	}
	defer tx.Rollback(ctx)

	variablesJSON, err := json.Marshal(input.Variables)
	if err != nil {
		return scheduleRecord{}, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE pipeline_schedules
		SET path = $2,
			name = $3,
			description = $4,
			pipeline_path = $5,
			pipeline_name = $6,
			pipeline_version = $7,
			schedule_kind = $8,
			cron_expression = $9,
			run_at = $10,
			timezone = $11,
			enabled = $12,
			scope = $13,
			variables = $14::jsonb,
			next_run_at = $15,
			run_group_path = $16,
			updated_by = $17,
			updated_at = NOW()
		WHERE id::text = $1
	`, scheduleID, input.Path, input.Name, input.Description, input.PipelinePath, input.PipelineName, input.PipelineVersion,
		input.ScheduleKind, input.CronExpression, input.RunAt, input.Timezone, input.Enabled, input.Scope, string(variablesJSON), input.NextRunAt, input.RunGroupPath, actor)
	if err != nil {
		return scheduleRecord{}, err
	}
	if tag.RowsAffected() == 0 {
		return scheduleRecord{}, pgx.ErrNoRows
	}
	if err := ensureScheduleExecutionACLs(ctx, tx, scheduleID, input, pipeline); err != nil {
		return scheduleRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return scheduleRecord{}, err
	}
	return a.getScheduleRecord(ctx, scheduleID)
}

func (a *App) setScheduleEnabled(ctx context.Context, scheduleID string, enabled bool, nextRunAt *time.Time, actor string) (scheduleRecord, error) {
	tag, err := a.db.Exec(ctx, `
		UPDATE pipeline_schedules
		SET enabled = $2,
			next_run_at = $3,
			updated_by = $4,
			updated_at = NOW()
		WHERE id::text = $1
	`, scheduleID, enabled, nextRunAt, actor)
	if err != nil {
		return scheduleRecord{}, err
	}
	if tag.RowsAffected() == 0 {
		return scheduleRecord{}, pgx.ErrNoRows
	}
	return a.getScheduleRecord(ctx, scheduleID)
}

func (a *App) deleteSchedule(ctx context.Context, scheduleID string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, _ = tx.Exec(ctx, `DELETE FROM resource_acl WHERE subject_type = $1 AND subject_id = $2`, aaamodel.SubjectTypeServiceAccount, scheduleServiceAccountID(scheduleID))
	tag, err := tx.Exec(ctx, `DELETE FROM pipeline_schedules WHERE id::text = $1`, scheduleID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return tx.Commit(ctx)
}

func ensureScheduleExecutionACLs(ctx context.Context, runner queryRunner, scheduleID string, input scheduleInput, pipeline models.Pipeline) error {
	serviceAccountID := scheduleServiceAccountID(scheduleID)
	if _, err := runner.Exec(ctx, `DELETE FROM resource_acl WHERE subject_type = $1 AND subject_id = $2`, aaamodel.SubjectTypeServiceAccount, serviceAccountID); err != nil {
		return err
	}
	type acl struct {
		resourceType string
		resourceID   string
		action       string
	}
	var grants []acl
	pipelineID := aaamodel.BuildPipelineID(input.PipelinePath, input.PipelineName)
	grants = append(grants,
		acl{resourceType: grantResourcePipeline, resourceID: pipelineID, action: "pipeline.execute"},
		acl{resourceType: grantResourcePipeline, resourceID: pipelineID, action: "pipeline.use"},
	)
	scopeID := strings.Trim(strings.TrimSpace(input.Scope), "/")
	if scopeID == "" {
		scopeID = "default"
	}
	normalizedScopeID, _, _ := normalizeScopeGrantResourceID(scopeID)
	grants = append(grants, acl{resourceType: grantResourceScope, resourceID: normalizedScopeID, action: "scope.use"})
	for _, stepID := range collectReferencedStepIdentifiers(&pipeline) {
		grants = append(grants, acl{resourceType: grantResourceStep, resourceID: stepID, action: "step.use"})
	}
	for _, childPipelineID := range collectReferencedPipelineIdentifiers(&pipeline) {
		grants = append(grants, acl{resourceType: grantResourcePipeline, resourceID: childPipelineID, action: "pipeline.use"})
	}

	seen := map[string]struct{}{}
	for _, grant := range grants {
		key := grant.resourceType + "\x00" + grant.resourceID + "\x00" + grant.action
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if _, err := runner.Exec(ctx, `
			INSERT INTO resource_acl (resource_type, resource_id, subject_type, subject_id, action, effect)
			VALUES ($1, $2, $3, $4, $5, 'allow')
			ON CONFLICT (resource_type, resource_id, subject_type, subject_id, action, effect)
			DO UPDATE SET access_grant_id = NULL
		`, grant.resourceType, grant.resourceID, aaamodel.SubjectTypeServiceAccount, serviceAccountID, grant.action); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) listScheduleRecords(ctx context.Context, pipelinePath, pipelineName string) ([]scheduleRecord, error) {
	query := baseScheduleSelect()
	args := []any{}
	if strings.TrimSpace(pipelineName) != "" {
		query += " WHERE s.pipeline_path = $1 AND s.pipeline_name = $2"
		args = append(args, pipelinePath, pipelineName)
	}
	query += " ORDER BY s.path ASC, s.name ASC"
	rows, err := a.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []scheduleRecord
	for rows.Next() {
		record, err := scanScheduleRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (a *App) getScheduleRecord(ctx context.Context, scheduleID string) (scheduleRecord, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return scheduleRecord{}, pgx.ErrNoRows
	}
	return scanScheduleRecord(a.db.QueryRow(ctx, baseScheduleSelect()+" WHERE s.id::text = $1 OR "+scheduleIdentifierSQL("s")+" = $1 LIMIT 1", scheduleID))
}

func baseScheduleSelect() string {
	return `
		SELECT
			s.id::text, s.path, s.name, s.description,
			s.pipeline_path, s.pipeline_name, s.pipeline_version,
			COALESCE(s.schedule_kind, 'cron'), s.cron_expression, s.run_at,
			s.timezone, s.enabled, s.scope, COALESCE(s.run_group_path, ''), s.variables::text,
			s.next_run_at, s.last_run_at, COALESCE(s.last_run_id::text, ''),
			COALESCE(s.last_status, ''), COALESCE(s.source, 'database'), COALESCE(s.visibility, 'group'),
			s.config_repo_id, COALESCE(s.config_source_path, ''), COALESCE(s.config_source_commit_sha, ''),
			s.managed_by_config_repo, COALESCE(s.created_by, ''), COALESCE(s.updated_by, ''),
			s.created_at, s.updated_at,
			COALESCE(r.run_id::text, ''), COALESCE(r.status, ''), r.started_at, r.finished_at
		FROM pipeline_schedules s
		LEFT JOIN pipeline_runs r ON r.run_id = s.last_run_id
	`
}

func scheduleIdentifierSQL(alias string) string {
	alias = strings.TrimSpace(alias)
	if alias != "" {
		alias += "."
	}
	return "CASE WHEN " + alias + "path = '' THEN " + alias + "name ELSE " + alias + "path || '/' || " + alias + "name END"
}

type scheduleScanner interface {
	Scan(dest ...any) error
}

func scanScheduleRecord(scanner scheduleScanner) (scheduleRecord, error) {
	var record scheduleRecord
	var variablesRaw string
	var runAt, nextRunAt, lastRunAt, latestStartedAt, latestFinishedAt sql.NullTime
	var latestRunID, latestStatus sql.NullString
	if err := scanner.Scan(
		&record.ID, &record.Path, &record.Name, &record.Description,
		&record.PipelinePath, &record.PipelineName, &record.PipelineVersion,
		&record.ScheduleKind, &record.CronExpression, &runAt,
		&record.Timezone, &record.Enabled, &record.Scope, &record.RunGroupPath, &variablesRaw,
		&nextRunAt, &lastRunAt, &record.LastRunID, &record.LastStatus, &record.Source, &record.Visibility,
		&record.ConfigRepoID, &record.ConfigSourcePath, &record.ConfigSourceCommitSHA,
		&record.ManagedByConfigRepo, &record.CreatedBy, &record.UpdatedBy, &record.CreatedAt, &record.UpdatedAt,
		&latestRunID, &latestStatus, &latestStartedAt, &latestFinishedAt,
	); err != nil {
		return record, err
	}
	record.ScheduleKind = normalizeScheduleKindValue(record.ScheduleKind)
	if strings.TrimSpace(variablesRaw) != "" {
		_ = json.Unmarshal([]byte(variablesRaw), &record.Variables)
	}
	if record.Variables == nil {
		record.Variables = map[string]string{}
	}
	if runAt.Valid {
		t := runAt.Time
		record.RunAt = &t
	}
	if nextRunAt.Valid {
		t := nextRunAt.Time
		record.NextRunAt = &t
	}
	if lastRunAt.Valid {
		t := lastRunAt.Time
		record.LastRunAt = &t
	}
	if latestRunID.Valid && strings.TrimSpace(latestRunID.String) != "" {
		summary := &scheduleRunSummary{
			RunID:  latestRunID.String,
			Status: latestStatus.String,
		}
		if latestStartedAt.Valid {
			t := latestStartedAt.Time
			summary.StartedAt = &t
		}
		if latestFinishedAt.Valid {
			t := latestFinishedAt.Time
			summary.FinishedAt = &t
		}
		if latestStartedAt.Valid {
			if latestFinishedAt.Valid {
				summary.Duration = latestFinishedAt.Time.Sub(latestStartedAt.Time).Round(time.Second).String()
			} else {
				summary.Duration = time.Since(latestStartedAt.Time).Round(time.Second).String()
			}
		}
		record.LatestRun = summary
	}
	return record, nil
}

func resolveScheduleGrantResource(ctx context.Context, runner queryRunner, rawID string, requireExists bool) (accessGrantResource, error) {
	rawID = strings.Trim(strings.TrimSpace(rawID), "/")
	if rawID == "" {
		return accessGrantResource{}, fmt.Errorf("resource_id is required")
	}
	if requireExists || looksLikeUUID(rawID) {
		var path, name string
		err := runner.QueryRow(ctx, `
			SELECT path, name
			FROM pipeline_schedules
			WHERE id::text = $1 OR `+scheduleIdentifierSQL("")+` = $1
			LIMIT 1
		`, rawID).Scan(&path, &name)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
				return accessGrantResource{}, fmt.Errorf("resource not found")
			}
			return accessGrantResource{}, err
		}
		identifier := configsync.BuildPipelineIdentifier(path, name)
		return accessGrantResource{Type: grantResourceSchedule, ID: identifier, Display: identifier}, nil
	}
	path, name, _, err := configsync.SplitPipelineIdentifier(rawID)
	if err != nil {
		return accessGrantResource{}, err
	}
	identifier := configsync.BuildPipelineIdentifier(path, name)
	return accessGrantResource{Type: grantResourceSchedule, ID: identifier, Display: identifier}, nil
}

func looksLikeUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}

func (a *App) resolveGroupIDForPath(ctx context.Context, groupPath string) (sql.NullInt32, error) {
	var out sql.NullInt32
	groupPath = strings.Trim(strings.TrimSpace(groupPath), "/")
	groupPath, rootOnly := stripRootPathPrefix(groupPath)
	if rootOnly || groupPath == "" {
		return out, nil
	}
	records, err := loadGroupPathRecords(ctx, a.db)
	if err != nil {
		return out, err
	}
	for _, record := range records {
		if record.Path == groupPath {
			out.Int32 = int32(record.ID)
			out.Valid = true
			return out, nil
		}
	}
	return out, nil
}

package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"

	aaamodel "nopsai/services/aaa/pkg/model"
	aaastore "nopsai/services/aaa/pkg/store"
)

const (
	dashboardRefreshScheduleWorkerPollInterval = time.Minute
	dashboardRefreshScheduleWorkerBatchSize    = 10
	dashboardRefreshScheduleServiceAccountPref = "dashboard-schedule:"
)

func dashboardRefreshScheduleServiceAccountID(scheduleID string) string {
	return dashboardRefreshScheduleServiceAccountPref + strings.TrimSpace(scheduleID)
}

func (a *App) listDashboardRefreshScheduleRecords(ctx context.Context, dashboard dashboardRecord) ([]dashboardRefreshScheduleRecord, error) {
	rows, err := a.db.Query(ctx, baseDashboardRefreshScheduleSelect()+`
		WHERE dashboard_id::text = $1
		ORDER BY name ASC
	`, dashboard.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []dashboardRefreshScheduleRecord
	for rows.Next() {
		record, err := scanDashboardRefreshScheduleRecord(rows)
		if err != nil {
			return nil, err
		}
		record.DashboardRef = dashboard.ref()
		records = append(records, record)
	}
	return records, rows.Err()
}

func (a *App) getDashboardRefreshScheduleRecord(ctx context.Context, dashboard dashboardRecord, scheduleID string) (dashboardRefreshScheduleRecord, error) {
	record, err := scanDashboardRefreshScheduleRecord(a.db.QueryRow(ctx, baseDashboardRefreshScheduleSelect()+`
		WHERE dashboard_id::text = $1
		  AND (id::text = $2 OR name = $2)
		LIMIT 1
	`, dashboard.ID, strings.TrimSpace(scheduleID)))
	if err != nil {
		return record, err
	}
	record.DashboardRef = dashboard.ref()
	return record, nil
}

func (a *App) createDashboardRefreshSchedule(ctx context.Context, dashboard dashboardRecord, input dashboardRefreshScheduleInput, actor, source, sourcePath, commitSHA string, configRepoID *int64) (dashboardRefreshScheduleRecord, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return dashboardRefreshScheduleRecord{}, err
	}
	defer tx.Rollback(ctx)

	scopeJSON, err := json.Marshal(input.Refresh.Scope)
	if err != nil {
		return dashboardRefreshScheduleRecord{}, err
	}
	variablesJSON, err := json.Marshal(input.Refresh.Variables)
	if err != nil {
		return dashboardRefreshScheduleRecord{}, err
	}
	var scheduleID string
	err = tx.QueryRow(ctx, `
		INSERT INTO dashboard_refresh_schedules (
			dashboard_id, name, description, cron_expression, timezone, enabled,
			scope_type, scope, mode, run_scope, variables, max_concurrency, timeout_seconds,
			next_run_at, source, config_repo_id, config_source_path, config_source_commit_sha,
			managed_by_config_repo, created_by, updated_by, updated_at
		) VALUES (
			$1::uuid, $2, $3, $4, $5, $6,
			$7, $8::jsonb, $9, $10, $11::jsonb, $12, $13,
			$14, $15, $16, $17, $18,
			$19, $20, $20, NOW()
		)
		RETURNING id::text
	`, dashboard.ID, input.Name, input.Description, input.CronExpression, input.Timezone, input.Enabled,
		input.Refresh.ScopeType, string(scopeJSON), input.Refresh.Mode, input.Refresh.RunScope, string(variablesJSON),
		input.Refresh.MaxConcurrency, int(input.Refresh.Timeout.Seconds()), input.NextRunAt, source, configRepoID,
		sourcePath, commitSHA, strings.EqualFold(source, "git"), actor).Scan(&scheduleID)
	if err != nil {
		return dashboardRefreshScheduleRecord{}, err
	}
	serviceAccountID := dashboardRefreshScheduleServiceAccountID(scheduleID)
	if _, err := tx.Exec(ctx, `
		UPDATE dashboard_refresh_schedules
		SET service_account_id = $2,
			updated_at = NOW()
		WHERE id::text = $1
	`, scheduleID, serviceAccountID); err != nil {
		return dashboardRefreshScheduleRecord{}, err
	}
	if err := ensureDashboardRefreshScheduleACLs(ctx, tx, dashboard, serviceAccountID, input.Refresh); err != nil {
		return dashboardRefreshScheduleRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return dashboardRefreshScheduleRecord{}, err
	}
	return a.getDashboardRefreshScheduleRecord(ctx, dashboard, scheduleID)
}

func (a *App) updateDashboardRefreshSchedule(ctx context.Context, dashboard dashboardRecord, scheduleID string, input dashboardRefreshScheduleInput, actor string) (dashboardRefreshScheduleRecord, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return dashboardRefreshScheduleRecord{}, err
	}
	defer tx.Rollback(ctx)

	scopeJSON, err := json.Marshal(input.Refresh.Scope)
	if err != nil {
		return dashboardRefreshScheduleRecord{}, err
	}
	variablesJSON, err := json.Marshal(input.Refresh.Variables)
	if err != nil {
		return dashboardRefreshScheduleRecord{}, err
	}
	serviceAccountID := dashboardRefreshScheduleServiceAccountID(scheduleID)
	tag, err := tx.Exec(ctx, `
		UPDATE dashboard_refresh_schedules
		SET name = $3,
			description = $4,
			cron_expression = $5,
			timezone = $6,
			enabled = $7,
			scope_type = $8,
			scope = $9::jsonb,
			mode = $10,
			run_scope = $11,
			variables = $12::jsonb,
			max_concurrency = $13,
			timeout_seconds = $14,
			next_run_at = $15,
			service_account_id = CASE WHEN service_account_id = '' THEN $16 ELSE service_account_id END,
			source = 'database',
			config_repo_id = NULL,
			config_source_path = '',
			config_source_commit_sha = '',
			managed_by_config_repo = FALSE,
			updated_by = $17,
			updated_at = NOW()
		WHERE dashboard_id::text = $1 AND id::text = $2
	`, dashboard.ID, scheduleID, input.Name, input.Description, input.CronExpression, input.Timezone, input.Enabled,
		input.Refresh.ScopeType, string(scopeJSON), input.Refresh.Mode, input.Refresh.RunScope, string(variablesJSON),
		input.Refresh.MaxConcurrency, int(input.Refresh.Timeout.Seconds()), input.NextRunAt, serviceAccountID, actor)
	if err != nil {
		return dashboardRefreshScheduleRecord{}, err
	}
	if tag.RowsAffected() == 0 {
		return dashboardRefreshScheduleRecord{}, pgx.ErrNoRows
	}
	if err := ensureDashboardRefreshScheduleACLs(ctx, tx, dashboard, serviceAccountID, input.Refresh); err != nil {
		return dashboardRefreshScheduleRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return dashboardRefreshScheduleRecord{}, err
	}
	return a.getDashboardRefreshScheduleRecord(ctx, dashboard, scheduleID)
}

func (a *App) deleteDashboardRefreshSchedule(ctx context.Context, dashboard dashboardRecord, scheduleID string) error {
	record, err := a.getDashboardRefreshScheduleRecord(ctx, dashboard, scheduleID)
	if err != nil {
		return err
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := aaastore.DeleteResourceACLBySubject(ctx, tx, aaamodel.SubjectTypeServiceAccount, record.ServiceAccountID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM dashboard_refresh_schedules WHERE dashboard_id::text = $1 AND id::text = $2`, dashboard.ID, record.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return tx.Commit(ctx)
}

func (a *App) setDashboardRefreshScheduleEnabled(ctx context.Context, dashboard dashboardRecord, scheduleID string, enabled bool, actor string) (dashboardRefreshScheduleRecord, error) {
	record, err := a.getDashboardRefreshScheduleRecord(ctx, dashboard, scheduleID)
	if err != nil {
		return dashboardRefreshScheduleRecord{}, err
	}
	var nextRunAt *time.Time
	if enabled {
		next, err := nextScheduleRunAt(record.CronExpression, record.Timezone, time.Now())
		if err != nil {
			return dashboardRefreshScheduleRecord{}, err
		}
		nextRunAt = &next
	}
	if _, err := a.db.Exec(ctx, `
		UPDATE dashboard_refresh_schedules
		SET enabled = $3,
			next_run_at = $4,
			updated_by = $5,
			updated_at = NOW()
		WHERE dashboard_id::text = $1 AND id::text = $2
	`, dashboard.ID, record.ID, enabled, nextRunAt, actor); err != nil {
		return dashboardRefreshScheduleRecord{}, err
	}
	return a.getDashboardRefreshScheduleRecord(ctx, dashboard, record.ID)
}

func ensureDashboardRefreshScheduleACLs(ctx context.Context, runner queryRunner, dashboard dashboardRecord, serviceAccountID string, refresh dashboardRefreshInput) error {
	if err := aaastore.DeleteResourceACLBySubject(ctx, runner, aaamodel.SubjectTypeServiceAccount, serviceAccountID); err != nil {
		return err
	}
	type acl struct {
		resourceType string
		resourceID   string
		action       string
	}
	grants := []acl{
		{resourceType: grantResourceDashboard, resourceID: dashboard.ref(), action: "dashboard.refresh"},
		{resourceType: grantResourceDashboard, resourceID: dashboard.ref(), action: "dashboard.publish"},
	}
	sources, err := listDashboardSourcesForACL(ctx, runner, dashboard.ID)
	if err != nil {
		return err
	}
	for _, source := range selectDashboardRefreshSources(refresh, sources) {
		grants = append(grants,
			acl{resourceType: grantResourcePipeline, resourceID: source.PipelineID, action: "pipeline.execute"},
			acl{resourceType: grantResourcePipeline, resourceID: source.PipelineID, action: "pipeline.use"},
		)
	}
	scopeIDs := map[string]struct{}{}
	if strings.TrimSpace(refresh.RunScope) != "" {
		scopeIDs[refresh.RunScope] = struct{}{}
	} else {
		for _, source := range selectDashboardRefreshSources(refresh, sources) {
			scopeIDs[source.RunScope] = struct{}{}
		}
	}
	if len(scopeIDs) == 0 {
		scopeIDs[""] = struct{}{}
	}
	for scopeID := range scopeIDs {
		if strings.TrimSpace(scopeID) == "" {
			scopeID = "default"
		}
		normalizedScopeID, _, _ := normalizeScopeGrantResourceID(scopeID)
		grants = append(grants, acl{resourceType: grantResourceScope, resourceID: normalizedScopeID, action: "scope.use"})
	}

	seen := map[string]struct{}{}
	for _, grant := range grants {
		key := grant.resourceType + "\x00" + grant.resourceID + "\x00" + grant.action
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if err := aaastore.UpsertResourceACL(ctx, runner, aaastore.ResourceACL{
			ResourceType: grant.resourceType,
			ResourceID:   grant.resourceID,
			SubjectType:  aaamodel.SubjectTypeServiceAccount,
			SubjectID:    serviceAccountID,
			Action:       grant.action,
			Effect:       "allow",
		}); err != nil {
			return err
		}
	}
	return nil
}

func listDashboardSourcesForACL(ctx context.Context, runner queryRunner, dashboardID string) ([]dashboardSourceRecord, error) {
	rows, err := runner.Query(ctx, `
		SELECT id::text, dashboard_id::text, section_key, pipeline_id, output_name, entry_key, run_scope,
		       enabled, required_for_refresh, refresh_order, created_at, updated_at
		FROM dashboard_source_bindings
		WHERE dashboard_id::text = $1
		ORDER BY section_key ASC, refresh_order ASC, pipeline_id ASC, output_name ASC, entry_key ASC, run_scope ASC
	`, dashboardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []dashboardSourceRecord
	for rows.Next() {
		record, err := scanDashboardSourceRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (a *App) runDashboardRefreshScheduleWorker(ctx context.Context) {
	ticker := time.NewTicker(dashboardRefreshScheduleWorkerPollInterval)
	defer ticker.Stop()

	a.dispatchDueDashboardRefreshSchedules(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.dispatchDueDashboardRefreshSchedules(ctx)
		}
	}
}

func (a *App) dispatchDueDashboardRefreshSchedules(ctx context.Context) {
	if a == nil || a.db == nil {
		return
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to begin scheduled dashboard refresh dispatch")
		return
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, baseDashboardRefreshScheduleSelect()+`
		WHERE enabled = TRUE
		  AND next_run_at IS NOT NULL
		  AND next_run_at <= NOW()
		ORDER BY next_run_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, dashboardRefreshScheduleWorkerBatchSize)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to claim due dashboard refresh schedules")
		return
	}
	var due []dashboardRefreshScheduleRecord
	for rows.Next() {
		record, err := scanDashboardRefreshScheduleRecord(rows)
		if err != nil {
			rows.Close()
			log.Warn().Err(err).Msg("Failed to scan due dashboard refresh schedule")
			return
		}
		due = append(due, record)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		log.Warn().Err(err).Msg("Failed to read due dashboard refresh schedules")
		return
	}
	rows.Close()

	now := time.Now()
	for _, record := range due {
		nextRunAt, err := nextScheduleRunAt(record.CronExpression, record.Timezone, now)
		if err != nil {
			log.Warn().Err(err).Str("dashboard_refresh_schedule_id", record.ID).Msg("Failed to calculate next dashboard refresh schedule time")
			_, _ = tx.Exec(ctx, `
				UPDATE dashboard_refresh_schedules
				SET next_run_at = NULL,
					last_status = 'failure',
					updated_at = NOW()
				WHERE id::text = $1
			`, record.ID)
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE dashboard_refresh_schedules
			SET next_run_at = $2,
				updated_at = NOW()
			WHERE id::text = $1
		`, record.ID, nextRunAt); err != nil {
			log.Warn().Err(err).Str("dashboard_refresh_schedule_id", record.ID).Msg("Failed to advance dashboard refresh schedule")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to commit dashboard refresh schedule claims")
		return
	}

	for _, record := range due {
		if _, err := a.executeDashboardRefreshSchedule(ctx, record); err != nil {
			log.Warn().Err(err).Str("dashboard_refresh_schedule_id", record.ID).Msg("Scheduled dashboard refresh failed")
		}
	}
}

func (a *App) executeDashboardRefreshSchedule(ctx context.Context, record dashboardRefreshScheduleRecord) (dashboardRefreshResponse, error) {
	dashboard, err := a.getDashboardRecord(ctx, record.DashboardID)
	if err != nil {
		return dashboardRefreshResponse{}, err
	}
	serviceAccountID := strings.TrimSpace(record.ServiceAccountID)
	if serviceAccountID == "" {
		serviceAccountID = dashboardRefreshScheduleServiceAccountID(record.ID)
	}
	subject := aaamodel.Subject{Type: aaamodel.SubjectTypeServiceAccount, ID: serviceAccountID}
	if a.aaaAvailable() {
		decision, err := a.aaaCheck(ctx, subject, "dashboard.refresh", dashboard.resourceRef(), map[string]any{
			"dashboard_schedule_id": record.ID,
			"background":            true,
		})
		if err != nil {
			return dashboardRefreshResponse{}, err
		}
		if !decision.Allowed {
			return dashboardRefreshResponse{}, fmt.Errorf("dashboard refresh schedule service account is not authorized")
		}
	}
	input := dashboardRefreshInput{
		ScopeType:      record.ScopeType,
		Scope:          record.Scope,
		SectionKeys:    stringSliceFromAny(record.Scope["section_keys"]),
		SourceIDs:      stringSliceFromAny(record.Scope["source_ids"]),
		TriggerType:    dashboardRefreshTriggerScheduled,
		Mode:           record.Mode,
		RunScope:       record.RunScope,
		Variables:      record.Variables,
		MaxConcurrency: record.MaxConcurrency,
		Timeout:        time.Duration(record.TimeoutSeconds) * time.Second,
		IdempotencyKey: fmt.Sprintf("dashboard-schedule:%s:%d", record.ID, time.Now().Unix()),
	}
	response, err := a.startDashboardRefresh(ctx, dashboard, input, subject)
	if err != nil {
		_, _ = a.db.Exec(ctx, `
			UPDATE dashboard_refresh_schedules
			SET last_status = 'failure',
				updated_at = NOW()
			WHERE id::text = $1
		`, record.ID)
		return dashboardRefreshResponse{}, err
	}
	_, _ = a.db.Exec(ctx, `
		UPDATE dashboard_refresh_schedules
		SET last_refresh_id = $2::uuid,
			last_status = $3,
			updated_at = NOW()
		WHERE id::text = $1
	`, record.ID, response.ID, response.Status)
	a.auditDashboardAction(ctx, nil, "dashboard.refreshed", dashboard, "success", map[string]any{
		"refresh_id":            response.ID,
		"trigger_type":          response.TriggerType,
		"dashboard_schedule_id": record.ID,
	})
	return response, nil
}

func baseDashboardRefreshScheduleSelect() string {
	return `
		SELECT id::text, dashboard_id::text, name, description, cron_expression, timezone,
		       enabled, scope_type, scope::text, mode, run_scope, variables::text,
		       max_concurrency, timeout_seconds, next_run_at, COALESCE(last_refresh_id::text, ''),
		       last_status, service_account_id, COALESCE(source, 'database'), config_repo_id,
		       COALESCE(config_source_path, ''), COALESCE(config_source_commit_sha, ''),
		       managed_by_config_repo, created_by, updated_by, created_at, updated_at
		FROM dashboard_refresh_schedules
	`
}

func scanDashboardRefreshScheduleRecord(scanner dashboardScanner) (dashboardRefreshScheduleRecord, error) {
	var record dashboardRefreshScheduleRecord
	var scopeRaw, variablesRaw string
	var nextRunAt sql.NullTime
	if err := scanner.Scan(
		&record.ID,
		&record.DashboardID,
		&record.Name,
		&record.Description,
		&record.CronExpression,
		&record.Timezone,
		&record.Enabled,
		&record.ScopeType,
		&scopeRaw,
		&record.Mode,
		&record.RunScope,
		&variablesRaw,
		&record.MaxConcurrency,
		&record.TimeoutSeconds,
		&nextRunAt,
		&record.LastRefreshID,
		&record.LastStatus,
		&record.ServiceAccountID,
		&record.Source,
		&record.ConfigRepoID,
		&record.ConfigSourcePath,
		&record.ConfigSourceCommitSHA,
		&record.ManagedByConfigRepo,
		&record.CreatedBy,
		&record.UpdatedBy,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return record, err
	}
	record.Scope = scanJSONMap(scopeRaw)
	record.Variables = scanJSONStringMap(variablesRaw)
	record.NextRunAt = nullTimePtr(nextRunAt)
	if strings.TrimSpace(record.ServiceAccountID) == "" && strings.TrimSpace(record.ID) != "" {
		record.ServiceAccountID = dashboardRefreshScheduleServiceAccountID(record.ID)
	}
	record.Mode = normalizeDashboardRefreshMode(record.Mode)
	record.ScopeType = firstNonEmptyString(record.ScopeType, dashboardRefreshScopeDashboard)
	return record, nil
}

func scanJSONStringMap(raw string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		return map[string]string{}
	}
	return out
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return dedupeStrings(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				values = append(values, strings.TrimSpace(text))
			}
		}
		return dedupeStrings(values)
	default:
		return nil
	}
}

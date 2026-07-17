package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"nopsai/pkg/models"
	aaamodel "nopsai/services/aaa/pkg/model"
)

type dashboardPublicationTarget struct {
	Ref       string
	Section   string
	EntryKey  string
	RunScope  string
	Mode      string
	Preset    string
	TTL       string
	RefreshID string
	ExpiresAt *time.Time
}

type dashboardPublicationRunRecord struct {
	PipelinePath string
	PipelineName string
	FinishedAt   *time.Time
	SubjectType  string
	SubjectID    string
	Scope        string
}

func (a *App) publishDashboardFinalOutput(ctx context.Context, runID string, output pipelineFinalOutputRecord, content string) error {
	spec, err := parseDashboardSpec(content)
	if err != nil {
		return err
	}
	contentJSON, err := marshalFinalOutputSpec(spec)
	if err != nil {
		return err
	}
	target, err := normalizeDashboardPublicationTarget(output)
	if err != nil {
		return err
	}
	run, err := a.loadDashboardPublicationRun(ctx, runID)
	if err != nil {
		return err
	}
	run.Scope, err = normalizeDashboardRunScope(run.Scope)
	if err != nil {
		return err
	}
	if refreshID, err := a.dashboardRefreshIDForRun(ctx, runID); err == nil {
		target.RefreshID = refreshID
	} else {
		return err
	}
	dashboard, err := a.getDashboardRecord(ctx, target.Ref)
	if err != nil {
		return fmt.Errorf("dashboard target %q not found: %w", target.Ref, err)
	}
	if err := a.authorizeDashboardPublication(ctx, run, dashboard); err != nil {
		return err
	}

	pipelineID := aaamodel.BuildPipelineID(run.PipelinePath, run.PipelineName)
	source, eventType, skipReason, err := a.matchDashboardPublicationSource(ctx, dashboard.ID, target, pipelineID, output.Name, run.Scope)
	if err != nil {
		return err
	}
	if eventType != "" {
		return a.insertDashboardPublicationSkipEvent(ctx, dashboard.ID, target, eventType, skipReason, runID, output, run)
	}
	target.RunScope = source.RunScope

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	switch target.Mode {
	case dashboardPublishModeAppend:
		err = insertAppendDashboardPublication(ctx, tx, dashboard.ID, target, runID, output, run, contentJSON)
	case dashboardPublishModeSnapshot:
		err = snapshotDashboardPublication(ctx, tx, dashboard.ID, target, runID, output, run, contentJSON)
	case dashboardPublishModeSeries:
		err = seriesDashboardPublication(ctx, tx, dashboard.ID, target, runID, output, run, spec)
	default:
		err = replaceDashboardPublication(ctx, tx, dashboard.ID, target, runID, output, run, contentJSON)
	}
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	a.auditDashboardAction(ctx, nil, "dashboard.published", dashboard, "success", map[string]any{
		"run_id":      runID,
		"output_id":   output.ID,
		"output_name": output.Name,
		"section":     target.Section,
		"entry_key":   target.EntryKey,
		"mode":        target.Mode,
	})
	return nil
}

func normalizeDashboardPublicationTarget(output pipelineFinalOutputRecord) (dashboardPublicationTarget, error) {
	target := output.Dashboard
	ref := strings.Trim(strings.TrimSpace(target.Ref), "/")
	if ref == "" {
		return dashboardPublicationTarget{}, fmt.Errorf("dashboard.ref is required")
	}
	section := strings.TrimSpace(target.Section)
	if section == "" {
		return dashboardPublicationTarget{}, fmt.Errorf("dashboard.section is required")
	}
	if !dashboardSectionKeyPattern.MatchString(section) {
		return dashboardPublicationTarget{}, fmt.Errorf("dashboard.section is invalid")
	}
	entryKey := strings.TrimSpace(target.EntryKey)
	if entryKey == "" {
		entryKey = strings.TrimSpace(output.Name)
	}
	if entryKey == "" || !dashboardEntryKeyPattern.MatchString(entryKey) {
		return dashboardPublicationTarget{}, fmt.Errorf("dashboard.entry_key is invalid")
	}
	mode := normalizeDashboardPublishMode(target.Mode)
	var expiresAt *time.Time
	if ttl := strings.TrimSpace(target.TTL); ttl != "" {
		duration, err := parseDashboardTTL(ttl)
		if err != nil {
			return dashboardPublicationTarget{}, err
		}
		t := time.Now().UTC().Add(duration)
		expiresAt = &t
	}
	return dashboardPublicationTarget{
		Ref:       ref,
		Section:   section,
		EntryKey:  entryKey,
		Mode:      mode,
		Preset:    strings.TrimSpace(target.Preset),
		TTL:       strings.TrimSpace(target.TTL),
		ExpiresAt: expiresAt,
	}, nil
}

func normalizeDashboardPublishMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case dashboardPublishModeAppend:
		return dashboardPublishModeAppend
	case dashboardPublishModeSnapshot:
		return dashboardPublishModeSnapshot
	case dashboardPublishModeSeries:
		return dashboardPublishModeSeries
	default:
		return dashboardPublishModeReplace
	}
}

func parseDashboardTTL(raw string) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, nil
	}
	duration, err := parseDashboardDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("dashboard.ttl must be a positive duration")
	}
	if duration > dashboardPublicationMaxTTL {
		return 0, fmt.Errorf("dashboard.ttl cannot exceed %s", dashboardPublicationMaxTTL)
	}
	return duration, nil
}

func (a *App) loadDashboardPublicationRun(ctx context.Context, runID string) (dashboardPublicationRunRecord, error) {
	var record dashboardPublicationRunRecord
	var finishedAt sql.NullTime
	var requestedType, requestedID, effectiveType, effectiveID sql.NullString
	err := a.db.QueryRow(ctx, `
		SELECT COALESCE(pipeline_path, ''), COALESCE(pipeline_name, ''), finished_at,
		       COALESCE(scope, ''),
		       requested_by_type, requested_by_id, effective_subject_type, effective_subject_id
		FROM pipeline_runs
		WHERE run_id::text = $1
	`, runID).Scan(
		&record.PipelinePath,
		&record.PipelineName,
		&finishedAt,
		&record.Scope,
		&requestedType,
		&requestedID,
		&effectiveType,
		&effectiveID,
	)
	if err != nil {
		return record, err
	}
	record.FinishedAt = nullTimePtr(finishedAt)
	record.SubjectType = strings.TrimSpace(effectiveType.String)
	record.SubjectID = strings.TrimSpace(effectiveID.String)
	if record.SubjectType == "" || record.SubjectID == "" {
		record.SubjectType = strings.TrimSpace(requestedType.String)
		record.SubjectID = strings.TrimSpace(requestedID.String)
	}
	if record.SubjectType == "" || record.SubjectID == "" {
		record.SubjectType = aaamodel.SubjectTypeInternalService
		record.SubjectID = "dispatcher"
	}
	return record, nil
}

func (a *App) authorizeDashboardPublication(ctx context.Context, run dashboardPublicationRunRecord, dashboard dashboardRecord) error {
	if !a.aaaAvailable() {
		return fmt.Errorf("authorization unavailable")
	}
	subject := subjectForResourceUse(run.SubjectType, run.SubjectID)
	decision, err := a.aaaCheck(ctx, subject, "dashboard.publish", dashboard.resourceRef(), map[string]any{
		"pipeline_id": aaamodel.BuildPipelineID(run.PipelinePath, run.PipelineName),
		"dashboard":   dashboard.ref(),
		"background":  true,
	})
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return fmt.Errorf("dashboard publication forbidden")
	}
	return nil
}

func (a *App) dashboardRefreshIDForRun(ctx context.Context, runID string) (string, error) {
	var refreshID string
	err := a.db.QueryRow(ctx, `
		SELECT refresh_id::text
		FROM dashboard_refresh_pipeline_runs
		WHERE run_id::text = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, strings.TrimSpace(runID)).Scan(&refreshID)
	if err != nil {
		if dashboardNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return refreshID, nil
}

func (a *App) matchDashboardPublicationSource(
	ctx context.Context,
	dashboardID string,
	target dashboardPublicationTarget,
	pipelineID string,
	outputName string,
	runScope string,
) (dashboardSourceRecord, string, string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id::text, dashboard_id::text, section_key, pipeline_id, output_name, entry_key, run_scope,
		       enabled, required_for_refresh, refresh_order, created_at, updated_at
		FROM dashboard_source_bindings
		WHERE dashboard_id::text = $1
		  AND section_key = $2
		  AND pipeline_id = $3
		  AND output_name = $4
		  AND (entry_key = $5 OR entry_key = '')
		ORDER BY CASE WHEN entry_key = $5 THEN 0 ELSE 1 END,
		         CASE WHEN run_scope = $6 THEN 0 ELSE 1 END,
		         run_scope ASC
	`, dashboardID, target.Section, pipelineID, outputName, target.EntryKey, runScope)
	if err != nil {
		return dashboardSourceRecord{}, "", "", err
	}
	defer rows.Close()

	var disabledExact bool
	availableScopes := []string{}
	for rows.Next() {
		source, scanErr := scanDashboardSourceRecord(rows)
		if scanErr != nil {
			return dashboardSourceRecord{}, "", "", scanErr
		}
		if source.RunScope == runScope {
			if source.Enabled {
				return source, "", "", nil
			}
			disabledExact = true
			continue
		}
		availableScopes = append(availableScopes, dashboardRunScopeLabel(source.RunScope))
	}
	if err := rows.Err(); err != nil {
		return dashboardSourceRecord{}, "", "", err
	}
	if disabledExact {
		return dashboardSourceRecord{}, "skipped_disabled_source", fmt.Sprintf("matching dashboard source for run scope %s is disabled", dashboardRunScopeLabel(runScope)), nil
	}
	if len(availableScopes) > 0 {
		sort.Strings(availableScopes)
		return dashboardSourceRecord{}, "skipped_scope_mismatch", fmt.Sprintf(
			"run scope %s does not match configured source scope(s): %s",
			dashboardRunScopeLabel(runScope),
			strings.Join(uniqueStrings(availableScopes), ", "),
		), nil
	}
	return dashboardSourceRecord{}, "skipped_missing_source", fmt.Sprintf(
		"no dashboard source binding matches pipeline %s output %s entry %s scope %s",
		pipelineID,
		outputName,
		target.EntryKey,
		dashboardRunScopeLabel(runScope),
	), nil
}

func (a *App) insertDashboardPublicationSkipEvent(
	ctx context.Context,
	dashboardID string,
	target dashboardPublicationTarget,
	eventType string,
	reason string,
	runID string,
	output pipelineFinalOutputRecord,
	run dashboardPublicationRunRecord,
) error {
	content, err := json.Marshal(map[string]any{
		"reason":      reason,
		"pipeline_id": aaamodel.BuildPipelineID(run.PipelinePath, run.PipelineName),
		"output_name": output.Name,
		"run_scope":   run.Scope,
	})
	if err != nil {
		return err
	}
	_, err = a.db.Exec(ctx, `
		INSERT INTO dashboard_publication_events (
			dashboard_id, section_key, entry_key, revision, event_type, content, run_id, refresh_id
		) VALUES (
			$1::uuid, $2, $3, 0, $4, $5::jsonb, $6::uuid, NULLIF($7, '')::uuid
		)
	`, dashboardID, target.Section, target.EntryKey, eventType, string(content), runID, target.RefreshID)
	return err
}

func dashboardRunScopeLabel(scope string) string {
	if strings.TrimSpace(scope) == "" {
		return "default"
	}
	return scope
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
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
	return result
}

func upsertDashboardSourceBinding(ctx context.Context, runner queryRunner, dashboardID string, input dashboardSourceInput) error {
	_, err := runner.Exec(ctx, `
		INSERT INTO dashboard_source_bindings (
			dashboard_id, section_key, pipeline_id, output_name, entry_key, run_scope,
			enabled, required_for_refresh, refresh_order, updated_at
		) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (dashboard_id, section_key, pipeline_id, output_name, entry_key, run_scope) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			required_for_refresh = EXCLUDED.required_for_refresh,
			refresh_order = EXCLUDED.refresh_order,
			updated_at = NOW()
	`, dashboardID, input.SectionKey, input.PipelineID, input.OutputName, input.EntryKey, input.RunScope, input.Enabled, input.RequiredForRefresh, input.RefreshOrder)
	return err
}

func insertAppendDashboardPublication(
	ctx context.Context,
	tx pgx.Tx,
	dashboardID string,
	target dashboardPublicationTarget,
	runID string,
	output pipelineFinalOutputRecord,
	run dashboardPublicationRunRecord,
	content string,
) error {
	var existing string
	err := tx.QueryRow(ctx, `
		SELECT id::text
		FROM dashboard_publications
		WHERE run_output_id::text = $1
		LIMIT 1
	`, output.ID).Scan(&existing)
	if err == nil {
		return nil
	}
	if err != nil && !dashboardNotFound(err) {
		return err
	}
	var revision int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(revision), 0) + 1
		FROM dashboard_publications
		WHERE dashboard_id::text = $1 AND section_key = $2 AND entry_key = $3 AND run_scope = $4
	`, dashboardID, target.Section, target.EntryKey, target.RunScope).Scan(&revision); err != nil {
		return err
	}
	var publicationID string
	err = tx.QueryRow(ctx, `
		INSERT INTO dashboard_publications (
			dashboard_id, section_key, entry_key, mode, content, revision,
			run_id, run_output_id, pipeline_id, output_name, run_scope, refresh_id, source_finished_at, expires_at, status, updated_at
		) VALUES (
			$1::uuid, $2, $3, 'append', $4::jsonb, $5,
			$6::uuid, $7::uuid, $8, $9, $10, NULLIF($11, '')::uuid, $12, $13, 'current', NOW()
		)
		RETURNING id::text
	`, dashboardID, target.Section, target.EntryKey, content, revision, runID, output.ID,
		aaamodel.BuildPipelineID(run.PipelinePath, run.PipelineName), output.Name, target.RunScope, target.RefreshID, run.FinishedAt, target.ExpiresAt).Scan(&publicationID)
	if err != nil {
		return err
	}
	return insertDashboardPublicationEvent(ctx, tx, dashboardID, target, publicationID, revision, "published", content, runID)
}

func replaceDashboardPublication(
	ctx context.Context,
	tx pgx.Tx,
	dashboardID string,
	target dashboardPublicationTarget,
	runID string,
	output pipelineFinalOutputRecord,
	run dashboardPublicationRunRecord,
	content string,
) error {
	var existingID, existingRunOutputID string
	var existingRevision int
	var existingFinishedAt sql.NullTime
	err := tx.QueryRow(ctx, `
		SELECT id::text, revision, source_finished_at, COALESCE(run_output_id::text, '')
		FROM dashboard_publications
		WHERE dashboard_id::text = $1
		  AND section_key = $2
		  AND entry_key = $3
		  AND run_scope = $4
		  AND mode = 'replace'
		  AND status = 'current'
		FOR UPDATE
	`, dashboardID, target.Section, target.EntryKey, target.RunScope).Scan(&existingID, &existingRevision, &existingFinishedAt, &existingRunOutputID)
	if err == nil {
		if strings.TrimSpace(existingRunOutputID) == output.ID {
			return nil
		}
		if existingFinishedAt.Valid && run.FinishedAt != nil && existingFinishedAt.Time.After(*run.FinishedAt) {
			return insertDashboardPublicationEvent(ctx, tx, dashboardID, target, existingID, existingRevision, "skipped_older", `{}`, runID)
		}
		revision := existingRevision + 1
		_, err = tx.Exec(ctx, `
			UPDATE dashboard_publications
			SET content = $2::jsonb,
				revision = $3,
				run_id = $4::uuid,
				run_output_id = $5::uuid,
				pipeline_id = $6,
				output_name = $7,
				run_scope = $8,
				refresh_id = NULLIF($9, '')::uuid,
				source_finished_at = $10,
				expires_at = $11,
				published_at = NOW(),
				updated_at = NOW()
			WHERE id::text = $1
		`, existingID, content, revision, runID, output.ID,
			aaamodel.BuildPipelineID(run.PipelinePath, run.PipelineName), output.Name, target.RunScope, target.RefreshID, run.FinishedAt, target.ExpiresAt)
		if err != nil {
			return err
		}
		return insertDashboardPublicationEvent(ctx, tx, dashboardID, target, existingID, revision, "published", content, runID)
	}
	if err != nil && !dashboardNotFound(err) {
		return err
	}

	var publicationID string
	err = tx.QueryRow(ctx, `
		INSERT INTO dashboard_publications (
			dashboard_id, section_key, entry_key, mode, content, revision,
			run_id, run_output_id, pipeline_id, output_name, run_scope, refresh_id, source_finished_at, expires_at, status, updated_at
		) VALUES (
			$1::uuid, $2, $3, 'replace', $4::jsonb, 1,
			$5::uuid, $6::uuid, $7, $8, $9, NULLIF($10, '')::uuid, $11, $12, 'current', NOW()
		)
		RETURNING id::text
	`, dashboardID, target.Section, target.EntryKey, content, runID, output.ID,
		aaamodel.BuildPipelineID(run.PipelinePath, run.PipelineName), output.Name, target.RunScope, target.RefreshID, run.FinishedAt, target.ExpiresAt).Scan(&publicationID)
	if err != nil {
		return err
	}
	return insertDashboardPublicationEvent(ctx, tx, dashboardID, target, publicationID, 1, "published", content, runID)
}

func snapshotDashboardPublication(
	ctx context.Context,
	tx pgx.Tx,
	dashboardID string,
	target dashboardPublicationTarget,
	runID string,
	output pipelineFinalOutputRecord,
	run dashboardPublicationRunRecord,
	content string,
) error {
	var existing string
	err := tx.QueryRow(ctx, `
		SELECT id::text
		FROM dashboard_publications
		WHERE run_output_id::text = $1
		LIMIT 1
	`, output.ID).Scan(&existing)
	if err == nil {
		return nil
	}
	if err != nil && !dashboardNotFound(err) {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE dashboard_publications
		SET status = 'archived',
			updated_at = NOW()
		WHERE dashboard_id::text = $1
		  AND section_key = $2
		  AND run_scope = $3
		  AND status = 'current'
	`, dashboardID, target.Section, target.RunScope); err != nil {
		return err
	}
	var revision int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(revision), 0) + 1
		FROM dashboard_publications
		WHERE dashboard_id::text = $1 AND section_key = $2 AND run_scope = $3
	`, dashboardID, target.Section, target.RunScope).Scan(&revision); err != nil {
		return err
	}
	var publicationID string
	err = tx.QueryRow(ctx, `
		INSERT INTO dashboard_publications (
			dashboard_id, section_key, entry_key, mode, content, revision,
			run_id, run_output_id, pipeline_id, output_name, run_scope, refresh_id, source_finished_at, expires_at, status, updated_at
		) VALUES (
			$1::uuid, $2, $3, 'snapshot', $4::jsonb, $5,
			$6::uuid, $7::uuid, $8, $9, $10, NULLIF($11, '')::uuid, $12, $13, 'current', NOW()
		)
		RETURNING id::text
	`, dashboardID, target.Section, target.EntryKey, content, revision, runID, output.ID,
		aaamodel.BuildPipelineID(run.PipelinePath, run.PipelineName), output.Name, target.RunScope, target.RefreshID, run.FinishedAt, target.ExpiresAt).Scan(&publicationID)
	if err != nil {
		return err
	}
	return insertDashboardPublicationEvent(ctx, tx, dashboardID, target, publicationID, revision, "published", content, runID)
}

func seriesDashboardPublication(
	ctx context.Context,
	tx pgx.Tx,
	dashboardID string,
	target dashboardPublicationTarget,
	runID string,
	output pipelineFinalOutputRecord,
	run dashboardPublicationRunRecord,
	incoming models.DashboardSpec,
) error {
	if err := validateDashboardSeriesPublicationSpec(incoming); err != nil {
		return err
	}
	var existing string
	err := tx.QueryRow(ctx, `
		SELECT id::text
		FROM dashboard_publications
		WHERE run_output_id::text = $1
		LIMIT 1
	`, output.ID).Scan(&existing)
	if err == nil {
		return nil
	}
	if err != nil && !dashboardNotFound(err) {
		return err
	}

	content := ""
	var existingID, existingContent string
	var existingRevision int
	err = tx.QueryRow(ctx, `
		SELECT id::text, revision, content::text
		FROM dashboard_publications
		WHERE dashboard_id::text = $1
		  AND section_key = $2
		  AND entry_key = $3
		  AND run_scope = $4
		  AND mode = 'series'
		  AND status = 'current'
		FOR UPDATE
	`, dashboardID, target.Section, target.EntryKey, target.RunScope).Scan(&existingID, &existingRevision, &existingContent)
	if err == nil {
		merged, mergeErr := mergeDashboardSeriesSpec(existingContent, incoming)
		if mergeErr != nil {
			return mergeErr
		}
		content, mergeErr = marshalFinalOutputSpec(merged)
		if mergeErr != nil {
			return mergeErr
		}
		revision := existingRevision + 1
		if _, err := tx.Exec(ctx, `
			UPDATE dashboard_publications
			SET content = $2::jsonb,
				revision = $3,
				run_id = $4::uuid,
				run_output_id = $5::uuid,
				pipeline_id = $6,
				output_name = $7,
				run_scope = $8,
				refresh_id = NULLIF($9, '')::uuid,
				source_finished_at = $10,
				expires_at = $11,
				published_at = NOW(),
				updated_at = NOW()
			WHERE id::text = $1
		`, existingID, content, revision, runID, output.ID,
			aaamodel.BuildPipelineID(run.PipelinePath, run.PipelineName), output.Name, target.RunScope, target.RefreshID, run.FinishedAt, target.ExpiresAt); err != nil {
			return err
		}
		return insertDashboardPublicationEvent(ctx, tx, dashboardID, target, existingID, revision, "published", content, runID)
	}
	if err != nil && !dashboardNotFound(err) {
		return err
	}

	var revision int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(revision), 0) + 1
		FROM dashboard_publications
		WHERE dashboard_id::text = $1 AND section_key = $2 AND entry_key = $3 AND run_scope = $4
	`, dashboardID, target.Section, target.EntryKey, target.RunScope).Scan(&revision); err != nil {
		return err
	}
	var marshalErr error
	content, marshalErr = marshalFinalOutputSpec(trimDashboardSeriesSpec(incoming))
	if marshalErr != nil {
		return marshalErr
	}
	var publicationID string
	err = tx.QueryRow(ctx, `
		INSERT INTO dashboard_publications (
			dashboard_id, section_key, entry_key, mode, content, revision,
			run_id, run_output_id, pipeline_id, output_name, run_scope, refresh_id, source_finished_at, expires_at, status, updated_at
		) VALUES (
			$1::uuid, $2, $3, 'series', $4::jsonb, $5,
			$6::uuid, $7::uuid, $8, $9, $10, NULLIF($11, '')::uuid, $12, $13, 'current', NOW()
		)
		RETURNING id::text
	`, dashboardID, target.Section, target.EntryKey, content, revision, runID, output.ID,
		aaamodel.BuildPipelineID(run.PipelinePath, run.PipelineName), output.Name, target.RunScope, target.RefreshID, run.FinishedAt, target.ExpiresAt).Scan(&publicationID)
	if err != nil {
		return err
	}
	return insertDashboardPublicationEvent(ctx, tx, dashboardID, target, publicationID, revision, "published", content, runID)
}

func validateDashboardSeriesPublicationSpec(spec models.DashboardSpec) error {
	for _, block := range spec.Blocks {
		if block.Chart != nil && (block.Type == "chart" || block.Type == "series") {
			return nil
		}
	}
	return fmt.Errorf("series publication requires at least one chart or series block")
}

func mergeDashboardSeriesSpec(existingContent string, incoming models.DashboardSpec) (models.DashboardSpec, error) {
	existing, err := parseDashboardSpec(existingContent)
	if err != nil {
		return models.DashboardSpec{}, err
	}
	existingBlocks := map[string]models.DashboardBlock{}
	for _, block := range existing.Blocks {
		if block.Chart == nil {
			continue
		}
		existingBlocks[dashboardSeriesBlockIdentity(block)] = block
	}
	merged := incoming
	for index, block := range merged.Blocks {
		if block.Chart == nil {
			continue
		}
		existingBlock, ok := existingBlocks[dashboardSeriesBlockIdentity(block)]
		if !ok || existingBlock.Chart == nil {
			block.Chart = trimDashboardChart(block.Chart)
			merged.Blocks[index] = block
			continue
		}
		block.Chart = mergeDashboardCharts(existingBlock.Chart, block.Chart)
		merged.Blocks[index] = block
	}
	return trimDashboardSeriesSpec(merged), nil
}

func dashboardSeriesBlockIdentity(block models.DashboardBlock) string {
	chartType := ""
	if block.Chart != nil {
		chartType = strings.ToLower(strings.TrimSpace(block.Chart.Type))
	}
	label := firstNonEmptyString(block.Title, block.Label)
	return strings.ToLower(strings.TrimSpace(block.Type)) + "\x00" + strings.ToLower(strings.TrimSpace(label)) + "\x00" + chartType
}

func mergeDashboardCharts(existing, incoming *models.DashboardChart) *models.DashboardChart {
	if incoming == nil {
		return trimDashboardChart(existing)
	}
	if existing == nil {
		return trimDashboardChart(incoming)
	}
	merged := *incoming
	seriesByKey := map[string]models.DashboardChartSeries{}
	for _, series := range existing.Series {
		seriesByKey[strings.TrimSpace(series.Key)] = series
	}
	merged.Series = make([]models.DashboardChartSeries, 0, len(incoming.Series))
	for _, series := range incoming.Series {
		key := strings.TrimSpace(series.Key)
		if current, ok := seriesByKey[key]; ok {
			series.Points = mergeDashboardSeriesPoints(current.Points, series.Points)
		}
		series.Points = trimDashboardSeriesPoints(series.Points)
		merged.Series = append(merged.Series, series)
		delete(seriesByKey, key)
	}
	for _, series := range seriesByKey {
		series.Points = trimDashboardSeriesPoints(series.Points)
		merged.Series = append(merged.Series, series)
	}
	sort.SliceStable(merged.Series, func(i, j int) bool {
		return strings.TrimSpace(merged.Series[i].Key) < strings.TrimSpace(merged.Series[j].Key)
	})
	return trimDashboardChart(&merged)
}

func mergeDashboardSeriesPoints(existing, incoming []models.DashboardSeriesPoint) []models.DashboardSeriesPoint {
	merged := make([]models.DashboardSeriesPoint, 0, len(existing)+len(incoming))
	indexByKey := map[string]int{}
	for _, point := range existing {
		key := dashboardSeriesPointKey(point)
		indexByKey[key] = len(merged)
		merged = append(merged, point)
	}
	for _, point := range incoming {
		key := dashboardSeriesPointKey(point)
		if idx, ok := indexByKey[key]; ok {
			merged[idx] = point
			continue
		}
		indexByKey[key] = len(merged)
		merged = append(merged, point)
	}
	return trimDashboardSeriesPoints(merged)
}

func dashboardSeriesPointKey(point models.DashboardSeriesPoint) string {
	if timestamp := strings.TrimSpace(point.Timestamp); timestamp != "" {
		return "t:" + timestamp
	}
	return "l:" + strings.TrimSpace(point.Label)
}

func trimDashboardSeriesSpec(spec models.DashboardSpec) models.DashboardSpec {
	for idx := range spec.Blocks {
		if spec.Blocks[idx].Chart != nil {
			spec.Blocks[idx].Chart = trimDashboardChart(spec.Blocks[idx].Chart)
		}
	}
	return spec
}

func trimDashboardChart(chart *models.DashboardChart) *models.DashboardChart {
	if chart == nil {
		return nil
	}
	trimmed := *chart
	trimmed.Series = append([]models.DashboardChartSeries(nil), chart.Series...)
	for idx := range trimmed.Series {
		trimmed.Series[idx].Points = trimDashboardSeriesPoints(trimmed.Series[idx].Points)
	}
	return &trimmed
}

func trimDashboardSeriesPoints(points []models.DashboardSeriesPoint) []models.DashboardSeriesPoint {
	if len(points) <= dashboardSeriesRetentionPoints {
		return points
	}
	ordered := append([]models.DashboardSeriesPoint(nil), points...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return dashboardSeriesPointSortKey(ordered[i]) < dashboardSeriesPointSortKey(ordered[j])
	})
	return ordered[len(ordered)-dashboardSeriesRetentionPoints:]
}

func dashboardSeriesPointSortKey(point models.DashboardSeriesPoint) string {
	if timestamp := strings.TrimSpace(point.Timestamp); timestamp != "" {
		return timestamp
	}
	return strings.TrimSpace(point.Label)
}

func insertDashboardPublicationEvent(
	ctx context.Context,
	runner queryRunner,
	dashboardID string,
	target dashboardPublicationTarget,
	publicationID string,
	revision int,
	eventType string,
	content string,
	runID string,
) error {
	if strings.TrimSpace(content) == "" {
		content = `{}`
	}
	var payload json.RawMessage = []byte(content)
	if !json.Valid(payload) {
		payload = []byte(`{}`)
	}
	_, err := runner.Exec(ctx, `
		INSERT INTO dashboard_publication_events (
			dashboard_id, section_key, entry_key, publication_id, revision, event_type, content, run_id, refresh_id
		) VALUES ($1::uuid, $2, $3, $4::uuid, $5, $6, $7::jsonb, $8::uuid, NULLIF($9, '')::uuid)
	`, dashboardID, target.Section, target.EntryKey, publicationID, revision, eventType, string(payload), runID, target.RefreshID)
	return err
}

func dashboardTargetFromOutputItem(item models.PipelineOutputItem) dashboardPublicationTarget {
	entryKey := strings.TrimSpace(item.Dashboard.EntryKey)
	if entryKey == "" {
		entryKey = strings.TrimSpace(item.Name)
	}
	return dashboardPublicationTarget{
		Ref:      strings.Trim(strings.TrimSpace(item.Dashboard.Ref), "/"),
		Section:  strings.TrimSpace(item.Dashboard.Section),
		EntryKey: entryKey,
		Mode:     normalizeDashboardPublishMode(item.Dashboard.Mode),
		Preset:   strings.TrimSpace(item.Dashboard.Preset),
		TTL:      strings.TrimSpace(item.Dashboard.TTL),
	}
}

func syncDashboardSourceBindingsForPipeline(ctx context.Context, runner queryRunner, pipelinePath, pipelineName string, pipeline models.Pipeline) error {
	if len(pipeline.Output.Items) == 0 {
		return nil
	}
	teamPaths, err := loadTeamPathRecords(ctx, runner)
	if err != nil {
		return err
	}
	for _, item := range pipeline.Output.Items {
		if normalizePipelineFinalOutputType(item.Type) != "dashboard" {
			continue
		}
		target := dashboardTargetFromOutputItem(item)
		if target.Ref == "" || target.Section == "" || target.EntryKey == "" {
			continue
		}
		teamPath, slug, err := splitDashboardRef(target.Ref)
		if err != nil {
			continue
		}
		teamID := 0
		for _, record := range teamPaths {
			if record.Path == teamPath {
				teamID = record.ID
				break
			}
		}
		if teamID == 0 {
			continue
		}
		var dashboardID string
		err = runner.QueryRow(ctx, `
			SELECT id::text
			FROM dashboards
			WHERE team_id = $1 AND slug = $2
			LIMIT 1
		`, teamID, slug).Scan(&dashboardID)
		if err != nil {
			if dashboardNotFound(err) {
				continue
			}
			return err
		}
		if err := upsertDashboardSection(ctx, runner, dashboardID, dashboardSectionInput{
			SectionKey: target.Section,
			Title:      titleFromKey(target.Section),
			Layout:     map[string]any{},
		}); err != nil {
			return err
		}
		if err := upsertDashboardSourceBinding(ctx, runner, dashboardID, dashboardSourceInput{
			SectionKey:         target.Section,
			PipelineID:         aaamodel.BuildPipelineID(pipelinePath, pipelineName),
			OutputName:         item.Name,
			EntryKey:           target.EntryKey,
			Enabled:            true,
			RequiredForRefresh: true,
		}); err != nil {
			return err
		}
	}
	return nil
}

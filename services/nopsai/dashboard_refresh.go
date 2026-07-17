package nopsai

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"

	aaamodel "nopsai/services/aaa/pkg/model"
)

const dashboardRefreshTriggerSource = "dashboard_refresh"

type dashboardRefreshHTTPError struct {
	StatusCode int
	Message    string
}

func (e dashboardRefreshHTTPError) Error() string {
	return e.Message
}

type dashboardRefreshSourcePlan struct {
	Source dashboardSourceRecord
	Status string
	Error  string
	Launch bool
}

func (a *App) startDashboardRefresh(ctx context.Context, dashboard dashboardRecord, input dashboardRefreshInput, subject aaamodel.Subject) (dashboardRefreshResponse, error) {
	if a == nil || a.db == nil {
		return dashboardRefreshResponse{}, dashboardRefreshHTTPError{StatusCode: http.StatusServiceUnavailable, Message: "database unavailable"}
	}
	sources, err := a.listDashboardSources(ctx, dashboard.ID)
	if err != nil {
		return dashboardRefreshResponse{}, err
	}
	plans, err := a.planDashboardRefreshSources(ctx, input, subject, sources)
	if err != nil {
		return dashboardRefreshResponse{}, err
	}
	if len(plans) == 0 {
		return dashboardRefreshResponse{}, dashboardRefreshHTTPError{StatusCode: http.StatusBadRequest, Message: "dashboard refresh has no matching sources"}
	}

	refreshID, existing, err := a.createDashboardRefreshRecord(ctx, dashboard, input, subject, plans)
	if err != nil {
		return dashboardRefreshResponse{}, err
	}
	if existing {
		return a.getDashboardRefreshResponse(ctx, dashboard, refreshID)
	}

	a.launchDashboardRefreshSources(ctx, dashboard, refreshID, input, subject, plans)
	return a.reconcileDashboardRefresh(ctx, dashboard, refreshID)
}

func (a *App) planDashboardRefreshSources(ctx context.Context, input dashboardRefreshInput, subject aaamodel.Subject, sources []dashboardSourceRecord) ([]dashboardRefreshSourcePlan, error) {
	selected := selectDashboardRefreshSources(input, sources)
	if len(selected) == 0 {
		return nil, nil
	}
	if len(selected) > dashboardRefreshMaxSources {
		return nil, dashboardRefreshHTTPError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("dashboard refresh cannot include more than %d sources", dashboardRefreshMaxSources),
		}
	}
	plans := make([]dashboardRefreshSourcePlan, 0, len(selected))
	var strictErrors []string
	for _, source := range selected {
		plan := dashboardRefreshSourcePlan{
			Source: source,
			Status: dashboardRefreshRunStatusQueued,
			Launch: true,
		}
		if !source.Enabled {
			plan.Status = dashboardRefreshRunStatusSkipped
			plan.Error = "source is disabled"
			plan.Launch = false
		} else if err := a.validateDashboardRefreshSource(ctx, subject, source); err != nil {
			plan.Status = dashboardRefreshRunStatusSkipped
			plan.Error = err.Error()
			plan.Launch = false
		}
		if !plan.Launch && input.Mode == dashboardRefreshModeStrict && source.RequiredForRefresh {
			strictErrors = append(strictErrors, fmt.Sprintf("%s/%s: %s", source.PipelineID, source.OutputName, plan.Error))
		}
		plans = append(plans, plan)
	}
	if len(strictErrors) > 0 {
		return nil, dashboardRefreshHTTPError{
			StatusCode: http.StatusBadRequest,
			Message:    "dashboard refresh strict mode blocked by required source: " + strings.Join(strictErrors, "; "),
		}
	}
	return plans, nil
}

func selectDashboardRefreshSources(input dashboardRefreshInput, sources []dashboardSourceRecord) []dashboardSourceRecord {
	sectionSet := make(map[string]struct{}, len(input.SectionKeys))
	for _, section := range input.SectionKeys {
		sectionSet[section] = struct{}{}
	}
	sourceSet := make(map[string]struct{}, len(input.SourceIDs))
	for _, sourceID := range input.SourceIDs {
		sourceSet[sourceID] = struct{}{}
	}
	selected := make([]dashboardSourceRecord, 0, len(sources))
	for _, source := range sources {
		switch input.ScopeType {
		case dashboardRefreshScopeSource:
			if _, ok := sourceSet[source.ID]; ok {
				selected = append(selected, source)
			}
		case dashboardRefreshScopeSection:
			if _, ok := sectionSet[source.SectionKey]; ok && source.Enabled {
				selected = append(selected, source)
			}
		default:
			if source.Enabled {
				selected = append(selected, source)
			}
		}
	}
	return selected
}

func (a *App) validateDashboardRefreshSource(ctx context.Context, subject aaamodel.Subject, source dashboardSourceRecord) error {
	pipelinePath, pipelineName := aaamodel.SplitPipelineID(source.PipelineID)
	if strings.TrimSpace(pipelineName) == "" {
		return fmt.Errorf("pipeline_id is invalid")
	}
	var exists int
	err := a.db.QueryRow(ctx, `
		SELECT 1
		FROM pipelines
		WHERE path = $1 AND name = $2
		LIMIT 1
	`, pipelinePath, pipelineName).Scan(&exists)
	if err != nil {
		if dashboardNotFound(err) {
			return fmt.Errorf("pipeline not found")
		}
		return err
	}
	if a.aaaAvailable() {
		decision, err := a.aaaCheck(ctx, subject, "pipeline.execute", aaamodel.ResourceRef{Type: grantResourcePipeline, ID: source.PipelineID}, map[string]any{
			"dashboard_refresh": true,
			"source_id":         source.ID,
		})
		if err != nil {
			return fmt.Errorf("authorization unavailable")
		}
		if !decision.Allowed {
			return fmt.Errorf("pipeline execute forbidden")
		}
	}
	return nil
}

func (a *App) createDashboardRefreshRecord(ctx context.Context, dashboard dashboardRecord, input dashboardRefreshInput, subject aaamodel.Subject, plans []dashboardRefreshSourcePlan) (string, bool, error) {
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey != "" {
		if existingID, err := a.findDashboardRefreshByIdempotency(ctx, dashboard.ID, idempotencyKey); err == nil && existingID != "" {
			return existingID, true, nil
		} else if err != nil && !dashboardNotFound(err) {
			return "", false, err
		}
	}
	var activeID string
	err := a.db.QueryRow(ctx, `
		SELECT id::text
		FROM dashboard_refreshes
		WHERE dashboard_id::text = $1 AND status = 'running'
		ORDER BY created_at DESC
		LIMIT 1
	`, dashboard.ID).Scan(&activeID)
	if err == nil && activeID != "" {
		return "", false, dashboardRefreshHTTPError{StatusCode: http.StatusConflict, Message: "dashboard refresh already running"}
	}
	if err != nil && !dashboardNotFound(err) {
		return "", false, err
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback(ctx)

	scopeJSON, err := json.Marshal(input.Scope)
	if err != nil {
		return "", false, err
	}
	triggerType := normalizeDashboardRefreshTrigger(input.TriggerType)
	timeoutSeconds := int(input.Timeout.Seconds())
	var timeoutAt *time.Time
	if input.Timeout > 0 {
		value := time.Now().UTC().Add(input.Timeout)
		timeoutAt = &value
	}
	total, required, queued, skipped := dashboardRefreshPlanCounts(plans)
	subjectType, subjectID := dashboardRefreshSubject(subject)
	var refreshID string
	err = tx.QueryRow(ctx, `
		INSERT INTO dashboard_refreshes (
			dashboard_id, requested_by_type, requested_by_id, trigger_type, scope_type, scope,
			mode, status, total_sources, required_sources, queued_sources, running_sources,
			successful_sources, failed_sources, skipped_sources, max_concurrency, timeout_seconds,
			idempotency_key, timeout_at, updated_at
		) VALUES (
			$1::uuid, $2, $3, $4, $5, $6::jsonb,
			$7, 'running', $8, $9, $10, 0,
			0, 0, $11, $12, $13,
			$14, $15, NOW()
		)
		RETURNING id::text
	`, dashboard.ID, subjectType, subjectID, triggerType, input.ScopeType, string(scopeJSON),
		input.Mode, total, required, queued, skipped, input.MaxConcurrency, timeoutSeconds,
		idempotencyKey, timeoutAt).Scan(&refreshID)
	if err != nil {
		if isUniqueViolation(err) {
			if idempotencyKey != "" {
				if existingID, findErr := a.findDashboardRefreshByIdempotency(ctx, dashboard.ID, idempotencyKey); findErr == nil && existingID != "" {
					return existingID, true, nil
				}
			}
			return "", false, dashboardRefreshHTTPError{StatusCode: http.StatusConflict, Message: "dashboard refresh already running"}
		}
		return "", false, err
	}
	for _, plan := range plans {
		runScope := plan.Source.RunScope
		if strings.TrimSpace(input.RunScope) != "" {
			runScope = input.RunScope
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO dashboard_refresh_pipeline_runs (
				refresh_id, dashboard_id, source_binding_id, pipeline_id, output_name,
				section_key, entry_key, run_scope, required, status, error, updated_at
			) VALUES (
				$1::uuid, $2::uuid, $3::uuid, $4, $5,
				$6, $7, $8, $9, $10, $11, NOW()
			)
		`, refreshID, dashboard.ID, plan.Source.ID, plan.Source.PipelineID, plan.Source.OutputName,
			plan.Source.SectionKey, plan.Source.EntryKey, runScope, plan.Source.RequiredForRefresh, plan.Status, plan.Error); err != nil {
			return "", false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", false, err
	}
	if queued == 0 {
		if _, err := a.updateDashboardRefreshRollup(ctx, refreshID); err != nil {
			return "", false, err
		}
	}
	return refreshID, false, nil
}

func dashboardRefreshPlanCounts(plans []dashboardRefreshSourcePlan) (total, required, queued, skipped int) {
	for _, plan := range plans {
		total++
		if plan.Source.RequiredForRefresh {
			required++
		}
		switch plan.Status {
		case dashboardRefreshRunStatusQueued:
			queued++
		case dashboardRefreshRunStatusSkipped:
			skipped++
		}
	}
	return total, required, queued, skipped
}

func dashboardRefreshSubject(subject aaamodel.Subject) (string, string) {
	subjectType := strings.TrimSpace(subject.Type)
	subjectID := firstNonEmptyString(subject.ID, subject.Sub, subject.Email)
	if subjectType == "" {
		subjectType = aaamodel.SubjectTypeInternalService
	}
	if subjectID == "" {
		subjectID = "dashboard-refresh"
	}
	return subjectType, subjectID
}

func (a *App) findDashboardRefreshByIdempotency(ctx context.Context, dashboardID, idempotencyKey string) (string, error) {
	var refreshID string
	err := a.db.QueryRow(ctx, `
		SELECT id::text
		FROM dashboard_refreshes
		WHERE dashboard_id::text = $1 AND idempotency_key = $2
		LIMIT 1
	`, dashboardID, strings.TrimSpace(idempotencyKey)).Scan(&refreshID)
	return refreshID, err
}

func (a *App) launchDashboardRefreshSources(ctx context.Context, dashboard dashboardRecord, refreshID string, input dashboardRefreshInput, subject aaamodel.Subject, plans []dashboardRefreshSourcePlan) {
	concurrency := input.MaxConcurrency
	if concurrency <= 0 {
		concurrency = dashboardRefreshDefaultConcurrency
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, plan := range plans {
		if !plan.Launch {
			continue
		}
		plan := plan
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			runID, err := a.launchDashboardRefreshSource(ctx, dashboard, refreshID, input, subject, plan.Source)
			if err != nil {
				if _, updateErr := a.db.Exec(ctx, `
					UPDATE dashboard_refresh_pipeline_runs
					SET status = 'failed',
						error = $3,
						finished_at = NOW(),
						updated_at = NOW()
					WHERE refresh_id::text = $1 AND source_binding_id::text = $2
				`, refreshID, plan.Source.ID, err.Error()); updateErr != nil {
					log.Warn().Err(updateErr).Str("refresh_id", refreshID).Str("source_id", plan.Source.ID).Msg("Failed to mark dashboard refresh source failed")
				}
				return
			}
			if _, err := a.db.Exec(ctx, `
				UPDATE dashboard_refresh_pipeline_runs
				SET status = 'running',
					run_id = $3::uuid,
					started_at = COALESCE(started_at, NOW()),
					error = '',
					updated_at = NOW()
				WHERE refresh_id::text = $1 AND source_binding_id::text = $2
			`, refreshID, plan.Source.ID, runID); err != nil {
				log.Warn().Err(err).Str("refresh_id", refreshID).Str("source_id", plan.Source.ID).Str("run_id", runID).Msg("Failed to link dashboard refresh run")
			}
		}()
	}
	wg.Wait()
}

func (a *App) launchDashboardRefreshSource(ctx context.Context, dashboard dashboardRecord, refreshID string, input dashboardRefreshInput, subject aaamodel.Subject, source dashboardSourceRecord) (string, error) {
	runScope := source.RunScope
	if strings.TrimSpace(input.RunScope) != "" {
		runScope = input.RunScope
	}
	payload := runRequestPayload{
		Pipeline:  source.PipelineID,
		Scope:     runScope,
		Variables: input.Variables,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/run", bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	subjectType, subjectID := dashboardRefreshSubject(subject)
	req.Header.Set("X-Nopsai-Caller-Type", subjectType)
	req.Header.Set("X-Nopsai-Caller-ID", subjectID)
	req.Header.Set("X-Nopsai-Trigger-Source", dashboardRefreshTriggerSource)
	req.Header.Set("X-Nopsai-Pipeline-Source", dashboardRefreshTriggerSource)
	req.Header.Set("X-Nopsai-Trigger-Event-ID", fmt.Sprintf("dashboard-refresh:%s:%s", refreshID, source.ID))
	req.Header.Set("X-Nopsai-Dashboard-Refresh-ID", refreshID)
	if strings.TrimSpace(runScope) != "" {
		req.Header.Set("X-Nopsai-Scope", runScope)
	}
	if strings.TrimSpace(dashboard.TeamPath) != "" {
		req.Header.Set("X-Nopsai-Team-Path", dashboard.TeamPath)
	}
	req = req.WithContext(withAAASubject(req.Context(), subject))

	recorder := httptest.NewRecorder()
	a.handleRunPipeline(recorder, req)
	if recorder.Code < 200 || recorder.Code >= 300 {
		message := strings.TrimSpace(recorder.Body.String())
		if message == "" {
			message = fmt.Sprintf("dashboard refresh source launch failed with status %d", recorder.Code)
		}
		return "", fmt.Errorf("%s", message)
	}
	var response struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || strings.TrimSpace(response.RunID) == "" {
		response.RunID = parseRunIDFromCreatedMessage(recorder.Body.String())
	}
	if strings.TrimSpace(response.RunID) == "" {
		return "", fmt.Errorf("dashboard refresh source launch did not return a run id")
	}
	return response.RunID, nil
}

func (a *App) getDashboardRefreshResponse(ctx context.Context, dashboard dashboardRecord, refreshID string) (dashboardRefreshResponse, error) {
	record, err := a.getDashboardRefreshRecord(ctx, dashboard, refreshID)
	if err != nil {
		return dashboardRefreshResponse{}, err
	}
	runs, err := a.listDashboardRefreshRunRecords(ctx, refreshID)
	if err != nil {
		return dashboardRefreshResponse{}, err
	}
	return dashboardRefreshResponseFromRecord(record, runs), nil
}

func (a *App) listDashboardRefreshResponses(ctx context.Context, dashboard dashboardRecord, limit int) ([]dashboardRefreshResponse, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	refreshes, err := a.listDashboardRefreshRecords(ctx, dashboard, limit)
	if err != nil {
		return nil, err
	}
	responses := make([]dashboardRefreshResponse, 0, len(refreshes))
	for _, refresh := range refreshes {
		record := refresh
		if record.Status == dashboardRefreshStatusRunning {
			reconciled, err := a.reconcileDashboardRefresh(ctx, dashboard, record.ID)
			if err == nil {
				responses = append(responses, reconciled)
				continue
			}
		}
		runs, err := a.listDashboardRefreshRunRecords(ctx, record.ID)
		if err != nil {
			return nil, err
		}
		responses = append(responses, dashboardRefreshResponseFromRecord(record, runs))
	}
	return responses, nil
}

func (a *App) reconcileDashboardRefresh(ctx context.Context, dashboard dashboardRecord, refreshID string) (dashboardRefreshResponse, error) {
	record, err := a.getDashboardRefreshRecord(ctx, dashboard, refreshID)
	if err != nil {
		return dashboardRefreshResponse{}, err
	}
	if record.Status != dashboardRefreshStatusRunning {
		return a.getDashboardRefreshResponse(ctx, dashboard, refreshID)
	}
	if record.TimeoutAt != nil && time.Now().After(*record.TimeoutAt) {
		if _, err := a.db.Exec(ctx, `
			UPDATE dashboard_refresh_pipeline_runs
			SET status = 'timed_out',
				error = CASE WHEN error = '' THEN 'dashboard refresh timed out' ELSE error END,
				finished_at = COALESCE(finished_at, NOW()),
				updated_at = NOW()
			WHERE refresh_id::text = $1 AND status IN ('queued', 'running')
		`, refreshID); err != nil {
			return dashboardRefreshResponse{}, err
		}
	}
	if err := a.reconcileDashboardRefreshRunRows(ctx, refreshID); err != nil {
		return dashboardRefreshResponse{}, err
	}
	if _, err := a.updateDashboardRefreshRollup(ctx, refreshID); err != nil {
		return dashboardRefreshResponse{}, err
	}
	return a.getDashboardRefreshResponse(ctx, dashboard, refreshID)
}

func (a *App) reconcileDashboardRefreshRunRows(ctx context.Context, refreshID string) error {
	rows, err := a.db.Query(ctx, `
		SELECT drr.id::text, COALESCE(drr.run_id::text, ''), drr.status, COALESCE(pr.status, '')
		FROM dashboard_refresh_pipeline_runs drr
		LEFT JOIN pipeline_runs pr ON pr.run_id = drr.run_id
		WHERE drr.refresh_id::text = $1
	`, refreshID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type update struct {
		id     string
		status string
		error  string
	}
	var updates []update
	for rows.Next() {
		var id, runID, currentStatus, runStatus string
		if err := rows.Scan(&id, &runID, &currentStatus, &runStatus); err != nil {
			return err
		}
		if dashboardRefreshRunTerminal(currentStatus) || runID == "" || runStatus == "" {
			continue
		}
		nextStatus := dashboardRefreshRunStatusFromPipelineStatus(runStatus)
		if nextStatus == "" || nextStatus == currentStatus {
			continue
		}
		message := ""
		if nextStatus == dashboardRefreshRunStatusFailed {
			message = "pipeline run failed"
		}
		updates = append(updates, update{id: id, status: nextStatus, error: message})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range updates {
		finishedExpr := "finished_at"
		if dashboardRefreshRunTerminal(item.status) {
			finishedExpr = "COALESCE(finished_at, NOW())"
		}
		_, err := a.db.Exec(ctx, fmt.Sprintf(`
			UPDATE dashboard_refresh_pipeline_runs
			SET status = $2,
				error = CASE WHEN $3 = '' THEN error ELSE $3 END,
				finished_at = %s,
				updated_at = NOW()
			WHERE id::text = $1
		`, finishedExpr), item.id, item.status, item.error)
		if err != nil {
			return err
		}
	}
	return nil
}

func dashboardRefreshRunStatusFromPipelineStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success":
		return dashboardRefreshRunStatusSuccess
	case "failure", "failure (ignored)", "rejected":
		return dashboardRefreshRunStatusFailed
	case "cancelled":
		return dashboardRefreshRunStatusCancelled
	case "timed_out":
		return dashboardRefreshRunStatusTimedOut
	case "skipped":
		return dashboardRefreshRunStatusSkipped
	case "pending", "running", "waiting_approval":
		return dashboardRefreshRunStatusRunning
	default:
		return ""
	}
}

func dashboardRefreshRunTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case dashboardRefreshRunStatusSuccess, dashboardRefreshRunStatusFailed, dashboardRefreshRunStatusSkipped, dashboardRefreshRunStatusCancelled, dashboardRefreshRunStatusTimedOut:
		return true
	default:
		return false
	}
}

func (a *App) updateDashboardRefreshRollup(ctx context.Context, refreshID string) (dashboardRefreshRecord, error) {
	var total, required, queued, running, successful, failed, skipped int
	err := a.db.QueryRow(ctx, `
		SELECT COUNT(*)::int,
		       COUNT(*) FILTER (WHERE required)::int,
		       COUNT(*) FILTER (WHERE status = 'queued')::int,
		       COUNT(*) FILTER (WHERE status = 'running')::int,
		       COUNT(*) FILTER (WHERE status = 'success')::int,
		       COUNT(*) FILTER (WHERE status IN ('failed', 'timed_out', 'cancelled'))::int,
		       COUNT(*) FILTER (WHERE status = 'skipped')::int
		FROM dashboard_refresh_pipeline_runs
		WHERE refresh_id::text = $1
	`, refreshID).Scan(&total, &required, &queued, &running, &successful, &failed, &skipped)
	if err != nil {
		return dashboardRefreshRecord{}, err
	}
	var requiredFailed, requiredSkipped, requiredSuccessful int
	if err := a.db.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE required AND status IN ('failed', 'timed_out', 'cancelled'))::int,
		       COUNT(*) FILTER (WHERE required AND status = 'skipped')::int,
		       COUNT(*) FILTER (WHERE required AND status = 'success')::int
		FROM dashboard_refresh_pipeline_runs
		WHERE refresh_id::text = $1
	`, refreshID).Scan(&requiredFailed, &requiredSkipped, &requiredSuccessful); err != nil {
		return dashboardRefreshRecord{}, err
	}
	status := dashboardRefreshStatusRunning
	finishedExpr := "finished_at"
	if queued+running == 0 {
		finishedExpr = "COALESCE(finished_at, NOW())"
		switch {
		case total == 0:
			status = dashboardRefreshStatusFailed
		case requiredFailed > 0 && successful == 0:
			status = dashboardRefreshStatusFailed
		case requiredFailed > 0 || requiredSkipped > 0 || requiredSuccessful < required:
			status = dashboardRefreshStatusPartial
		default:
			status = dashboardRefreshStatusComplete
		}
	}
	if _, err := a.db.Exec(ctx, fmt.Sprintf(`
		UPDATE dashboard_refreshes
		SET status = $2,
			total_sources = $3,
			required_sources = $4,
			queued_sources = $5,
			running_sources = $6,
			successful_sources = $7,
			failed_sources = $8,
			skipped_sources = $9,
			finished_at = %s,
			updated_at = NOW()
		WHERE id::text = $1
	`, finishedExpr), refreshID, status, total, required, queued, running, successful, failed, skipped); err != nil {
		return dashboardRefreshRecord{}, err
	}
	return a.getDashboardRefreshRecordByID(ctx, refreshID)
}

func (a *App) cancelDashboardRefresh(ctx context.Context, dashboard dashboardRecord, refreshID string) (dashboardRefreshResponse, error) {
	record, err := a.getDashboardRefreshRecord(ctx, dashboard, refreshID)
	if err != nil {
		return dashboardRefreshResponse{}, err
	}
	if record.Status != dashboardRefreshStatusRunning {
		return a.getDashboardRefreshResponse(ctx, dashboard, refreshID)
	}
	runIDs, err := a.activeDashboardRefreshRunIDs(ctx, refreshID)
	if err != nil {
		return dashboardRefreshResponse{}, err
	}
	if _, err := a.db.Exec(ctx, `
		UPDATE dashboard_refresh_pipeline_runs
		SET status = 'cancelled',
			error = CASE WHEN error = '' THEN 'dashboard refresh cancelled' ELSE error END,
			finished_at = COALESCE(finished_at, NOW()),
			updated_at = NOW()
		WHERE refresh_id::text = $1 AND status IN ('queued', 'running')
	`, refreshID); err != nil {
		return dashboardRefreshResponse{}, err
	}
	for _, runID := range runIDs {
		runUUID, parseErr := uuid.Parse(runID)
		if parseErr != nil {
			continue
		}
		if err := a.cancelRunHierarchy(ctx, runUUID, "Dashboard refresh cancelled.", "Dashboard refresh cancelled."); err != nil && !errors.Is(err, errRunAlreadyCompleted) && !dashboardNotFound(err) {
			log.Warn().Err(err).Str("run_id", runID).Str("refresh_id", refreshID).Msg("Failed to cancel dashboard refresh pipeline run")
		}
	}
	if _, err := a.db.Exec(ctx, `
		UPDATE dashboard_refreshes
		SET status = 'cancelled',
			queued_sources = 0,
			running_sources = 0,
			failed_sources = failed_sources + queued_sources + running_sources,
			finished_at = COALESCE(finished_at, NOW()),
			updated_at = NOW()
		WHERE id::text = $1
	`, refreshID); err != nil {
		return dashboardRefreshResponse{}, err
	}
	return a.getDashboardRefreshResponse(ctx, dashboard, refreshID)
}

func (a *App) activeDashboardRefreshRunIDs(ctx context.Context, refreshID string) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT COALESCE(run_id::text, '')
		FROM dashboard_refresh_pipeline_runs
		WHERE refresh_id::text = $1 AND status IN ('queued', 'running') AND run_id IS NOT NULL
	`, refreshID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runIDs []string
	for rows.Next() {
		var runID string
		if err := rows.Scan(&runID); err != nil {
			return nil, err
		}
		if runID != "" {
			runIDs = append(runIDs, runID)
		}
	}
	return runIDs, rows.Err()
}

func (a *App) retryFailedDashboardRefreshSources(ctx context.Context, dashboard dashboardRecord, refreshID string, subject aaamodel.Subject) (dashboardRefreshResponse, error) {
	if _, err := a.getDashboardRefreshRecord(ctx, dashboard, refreshID); err != nil {
		return dashboardRefreshResponse{}, err
	}
	rows, err := a.db.Query(ctx, `
		SELECT DISTINCT COALESCE(source_binding_id::text, '')
		FROM dashboard_refresh_pipeline_runs
		WHERE refresh_id::text = $1
		  AND status IN ('failed', 'skipped', 'cancelled', 'timed_out')
		  AND source_binding_id IS NOT NULL
	`, refreshID)
	if err != nil {
		return dashboardRefreshResponse{}, err
	}
	defer rows.Close()
	var sourceIDs []string
	for rows.Next() {
		var sourceID string
		if err := rows.Scan(&sourceID); err != nil {
			return dashboardRefreshResponse{}, err
		}
		if sourceID != "" {
			sourceIDs = append(sourceIDs, sourceID)
		}
	}
	if err := rows.Err(); err != nil {
		return dashboardRefreshResponse{}, err
	}
	if len(sourceIDs) == 0 {
		return dashboardRefreshResponse{}, dashboardRefreshHTTPError{StatusCode: http.StatusBadRequest, Message: "refresh has no failed sources to retry"}
	}
	input := dashboardRefreshInput{
		ScopeType:      dashboardRefreshScopeSource,
		Scope:          map[string]any{"type": dashboardRefreshScopeSource, "source_ids": sourceIDs, "retry_of": refreshID},
		SourceIDs:      sourceIDs,
		Mode:           dashboardRefreshModeBestEffort,
		Variables:      map[string]string{},
		MaxConcurrency: dashboardRefreshDefaultConcurrency,
		Timeout:        dashboardRefreshDefaultTimeout,
	}
	return a.startDashboardRefresh(ctx, dashboard, input, subject)
}

func (a *App) getDashboardRefreshRecord(ctx context.Context, dashboard dashboardRecord, refreshID string) (dashboardRefreshRecord, error) {
	record, err := a.getDashboardRefreshRecordByID(ctx, refreshID)
	if err != nil {
		return record, err
	}
	if record.DashboardID != dashboard.ID {
		return dashboardRefreshRecord{}, pgx.ErrNoRows
	}
	record.DashboardRef = dashboard.ref()
	return record, nil
}

func (a *App) getDashboardRefreshRecordByID(ctx context.Context, refreshID string) (dashboardRefreshRecord, error) {
	return scanDashboardRefreshRecord(a.db.QueryRow(ctx, `
		SELECT id::text, dashboard_id::text, requested_by_type, requested_by_id, trigger_type,
		       scope_type, scope::text, mode, status, total_sources, required_sources,
		       queued_sources, running_sources, successful_sources, failed_sources, skipped_sources,
		       max_concurrency, timeout_seconds, idempotency_key, error, started_at, finished_at,
		       timeout_at, created_at, updated_at
		FROM dashboard_refreshes
		WHERE id::text = $1
	`, refreshID))
}

func (a *App) listDashboardRefreshRecords(ctx context.Context, dashboard dashboardRecord, limit int) ([]dashboardRefreshRecord, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id::text, dashboard_id::text, requested_by_type, requested_by_id, trigger_type,
		       scope_type, scope::text, mode, status, total_sources, required_sources,
		       queued_sources, running_sources, successful_sources, failed_sources, skipped_sources,
		       max_concurrency, timeout_seconds, idempotency_key, error, started_at, finished_at,
		       timeout_at, created_at, updated_at
		FROM dashboard_refreshes
		WHERE dashboard_id::text = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, dashboard.ID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []dashboardRefreshRecord{}
	for rows.Next() {
		record, err := scanDashboardRefreshRecord(rows)
		if err != nil {
			return nil, err
		}
		record.DashboardRef = dashboard.ref()
		records = append(records, record)
	}
	return records, rows.Err()
}

func (a *App) listDashboardRefreshRunRecords(ctx context.Context, refreshID string) ([]dashboardRefreshRunRecord, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id::text, refresh_id::text, dashboard_id::text, COALESCE(source_binding_id::text, ''),
		       pipeline_id, output_name, section_key, entry_key, run_scope, COALESCE(run_id::text, ''),
		       required, status, error, started_at, finished_at, created_at, updated_at
		FROM dashboard_refresh_pipeline_runs
		WHERE refresh_id::text = $1
		ORDER BY section_key ASC, created_at ASC, pipeline_id ASC, output_name ASC
	`, refreshID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []dashboardRefreshRunRecord{}
	for rows.Next() {
		record, err := scanDashboardRefreshRunRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func scanDashboardRefreshRecord(scanner dashboardScanner) (dashboardRefreshRecord, error) {
	var record dashboardRefreshRecord
	var scopeRaw string
	var finishedAt, timeoutAt sql.NullTime
	if err := scanner.Scan(
		&record.ID,
		&record.DashboardID,
		&record.RequestedByType,
		&record.RequestedByID,
		&record.TriggerType,
		&record.ScopeType,
		&scopeRaw,
		&record.Mode,
		&record.Status,
		&record.TotalSources,
		&record.RequiredSources,
		&record.QueuedSources,
		&record.RunningSources,
		&record.SuccessfulSources,
		&record.FailedSources,
		&record.SkippedSources,
		&record.MaxConcurrency,
		&record.TimeoutSeconds,
		&record.IdempotencyKey,
		&record.Error,
		&record.StartedAt,
		&finishedAt,
		&timeoutAt,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return record, err
	}
	record.Scope = scanJSONMap(scopeRaw)
	record.FinishedAt = nullTimePtr(finishedAt)
	record.TimeoutAt = nullTimePtr(timeoutAt)
	return record, nil
}

func scanDashboardRefreshRunRecord(scanner dashboardScanner) (dashboardRefreshRunRecord, error) {
	var record dashboardRefreshRunRecord
	var startedAt, finishedAt sql.NullTime
	if err := scanner.Scan(
		&record.ID,
		&record.RefreshID,
		&record.DashboardID,
		&record.SourceBindingID,
		&record.PipelineID,
		&record.OutputName,
		&record.SectionKey,
		&record.EntryKey,
		&record.RunScope,
		&record.RunID,
		&record.Required,
		&record.Status,
		&record.Error,
		&startedAt,
		&finishedAt,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return record, err
	}
	record.StartedAt = nullTimePtr(startedAt)
	record.FinishedAt = nullTimePtr(finishedAt)
	return record, nil
}

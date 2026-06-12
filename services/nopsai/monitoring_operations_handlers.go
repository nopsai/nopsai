package nopsai

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"

	"nopsai/services/aaa/pkg/model"
)

type monitoringSavedViewRecord struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	OwnerSubjectType    string         `json:"owner_subject_type"`
	OwnerSubjectID      string         `json:"owner_subject_id"`
	Visibility          string         `json:"visibility"`
	GroupID             *int           `json:"group_id,omitempty"`
	Filters             map[string]any `json:"filters"`
	Columns             []string       `json:"columns"`
	Source              string         `json:"source"`
	ManagedByConfigRepo bool           `json:"managed_by_config_repo"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

type monitoringSavedViewInput struct {
	Name       string         `json:"name"`
	Visibility string         `json:"visibility"`
	GroupID    *int           `json:"group_id"`
	Filters    map[string]any `json:"filters"`
	Columns    []string       `json:"columns"`
}

type monitoringAlertRuleRecord struct {
	ID                  string                `json:"id"`
	Name                string                `json:"name"`
	Description         string                `json:"description"`
	Enabled             bool                  `json:"enabled"`
	OwnerSubjectType    string                `json:"owner_subject_type"`
	OwnerSubjectID      string                `json:"owner_subject_id"`
	Visibility          string                `json:"visibility"`
	Severity            string                `json:"severity"`
	Metric              string                `json:"metric"`
	Comparator          string                `json:"comparator"`
	Threshold           float64               `json:"threshold"`
	WindowSeconds       int                   `json:"window_seconds"`
	Filters             map[string]any        `json:"filters"`
	Source              string                `json:"source"`
	ManagedByConfigRepo bool                  `json:"managed_by_config_repo"`
	CreatedAt           time.Time             `json:"created_at"`
	UpdatedAt           time.Time             `json:"updated_at"`
	LastEvent           *monitoringAlertEvent `json:"last_event,omitempty"`
}

type monitoringAlertRuleInput struct {
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Enabled       *bool          `json:"enabled"`
	Visibility    string         `json:"visibility"`
	Severity      string         `json:"severity"`
	Metric        string         `json:"metric"`
	Comparator    string         `json:"comparator"`
	Threshold     float64        `json:"threshold"`
	WindowSeconds int            `json:"window_seconds"`
	Filters       map[string]any `json:"filters"`
}

type monitoringAlertEvent struct {
	ID         string     `json:"id"`
	RuleID     string     `json:"rule_id,omitempty"`
	Status     string     `json:"status"`
	Value      float64    `json:"value"`
	Message    string     `json:"message"`
	StartedAt  time.Time  `json:"started_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type monitoringRecommendationRecord struct {
	ID          string         `json:"id"`
	Fingerprint string         `json:"fingerprint"`
	Category    string         `json:"category"`
	Severity    string         `json:"severity"`
	Status      string         `json:"status"`
	Message     string         `json:"message"`
	Metadata    map[string]any `json:"metadata"`
	FirstSeenAt time.Time      `json:"first_seen_at"`
	LastSeenAt  time.Time      `json:"last_seen_at"`
	ResolvedAt  *time.Time     `json:"resolved_at,omitempty"`
}

func (a *App) handleListMonitoringSavedViews(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	subject, ok := a.requireMonitoringSubject(w, r)
	if !ok {
		return
	}
	ownerType, ownerID := monitoringSubjectOwner(subject)
	rows, err := a.db.Query(r.Context(), `
		SELECT id::text, name, owner_subject_type, owner_subject_id, visibility, group_id,
		       filters, columns, source, managed_by_config_repo, created_at, updated_at
		FROM monitoring_saved_views
		WHERE visibility = 'workspace'
		   OR (owner_subject_type = $1 AND owner_subject_id = $2)
		   OR managed_by_config_repo
		ORDER BY updated_at DESC, name ASC
		LIMIT 100
	`, ownerType, ownerID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list monitoring saved views")
		http.Error(w, "failed to list monitoring saved views", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	views := []monitoringSavedViewRecord{}
	for rows.Next() {
		item, err := scanMonitoringSavedView(rows)
		if err != nil {
			http.Error(w, "failed to read monitoring saved views", http.StatusInternalServerError)
			return
		}
		views = append(views, item)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read monitoring saved views", http.StatusInternalServerError)
		return
	}
	writeMonitoringJSON(w, views)
}

func (a *App) handleCreateMonitoringSavedView(w http.ResponseWriter, r *http.Request) {
	subject, ok := a.requireMonitoringSubject(w, r)
	if !ok {
		return
	}
	var input monitoringSavedViewInput
	if !decodeMonitoringJSON(w, r, &input) {
		return
	}
	if err := normalizeMonitoringSavedViewInput(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ownerType, ownerID := monitoringSubjectOwner(subject)
	filtersJSON, _ := json.Marshal(input.Filters)
	columnsJSON, _ := json.Marshal(input.Columns)
	item, err := a.insertMonitoringSavedView(r.Context(), input, ownerType, ownerID, filtersJSON, columnsJSON)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create monitoring saved view")
		http.Error(w, "failed to create monitoring saved view", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	writeMonitoringJSON(w, item)
}

func (a *App) handleUpdateMonitoringSavedView(w http.ResponseWriter, r *http.Request) {
	subject, ok := a.requireMonitoringSubject(w, r)
	if !ok {
		return
	}
	viewID := strings.TrimSpace(r.PathValue("viewID"))
	if viewID == "" {
		http.Error(w, "viewID is required", http.StatusBadRequest)
		return
	}
	var input monitoringSavedViewInput
	if !decodeMonitoringJSON(w, r, &input) {
		return
	}
	if err := normalizeMonitoringSavedViewInput(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ownerType, ownerID := monitoringSubjectOwner(subject)
	filtersJSON, _ := json.Marshal(input.Filters)
	columnsJSON, _ := json.Marshal(input.Columns)
	item, err := a.updateMonitoringSavedView(r.Context(), viewID, input, ownerType, ownerID, filtersJSON, columnsJSON)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "monitoring saved view not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Msg("Failed to update monitoring saved view")
		http.Error(w, "failed to update monitoring saved view", http.StatusInternalServerError)
		return
	}
	writeMonitoringJSON(w, item)
}

func (a *App) handleDeleteMonitoringSavedView(w http.ResponseWriter, r *http.Request) {
	subject, ok := a.requireMonitoringSubject(w, r)
	if !ok {
		return
	}
	viewID := strings.TrimSpace(r.PathValue("viewID"))
	if viewID == "" {
		http.Error(w, "viewID is required", http.StatusBadRequest)
		return
	}
	ownerType, ownerID := monitoringSubjectOwner(subject)
	tag, err := a.db.Exec(r.Context(), `
		DELETE FROM monitoring_saved_views
		WHERE id::text = $1
		  AND owner_subject_type = $2
		  AND owner_subject_id = $3
		  AND managed_by_config_repo = FALSE
	`, viewID, ownerType, ownerID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete monitoring saved view")
		http.Error(w, "failed to delete monitoring saved view", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "monitoring saved view not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleListMonitoringAlertRules(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	subject, ok := a.requireMonitoringSubject(w, r)
	if !ok {
		return
	}
	ownerType, ownerID := monitoringSubjectOwner(subject)
	rules, err := a.listMonitoringAlertRules(r.Context(), ownerType, ownerID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list monitoring alert rules")
		http.Error(w, "failed to list monitoring alert rules", http.StatusInternalServerError)
		return
	}
	writeMonitoringJSON(w, rules)
}

func (a *App) handleCreateMonitoringAlertRule(w http.ResponseWriter, r *http.Request) {
	subject, ok := a.requireMonitoringSubject(w, r)
	if !ok {
		return
	}
	var input monitoringAlertRuleInput
	if !decodeMonitoringJSON(w, r, &input) {
		return
	}
	if err := normalizeMonitoringAlertRuleInput(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ownerType, ownerID := monitoringSubjectOwner(subject)
	filtersJSON, _ := json.Marshal(input.Filters)
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	item, err := a.insertMonitoringAlertRule(r.Context(), input, enabled, ownerType, ownerID, filtersJSON)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create monitoring alert rule")
		http.Error(w, "failed to create monitoring alert rule", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	writeMonitoringJSON(w, item)
}

func (a *App) handleUpdateMonitoringAlertRule(w http.ResponseWriter, r *http.Request) {
	subject, ok := a.requireMonitoringSubject(w, r)
	if !ok {
		return
	}
	ruleID := strings.TrimSpace(r.PathValue("ruleID"))
	if ruleID == "" {
		http.Error(w, "ruleID is required", http.StatusBadRequest)
		return
	}
	var input monitoringAlertRuleInput
	if !decodeMonitoringJSON(w, r, &input) {
		return
	}
	if err := normalizeMonitoringAlertRuleInput(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ownerType, ownerID := monitoringSubjectOwner(subject)
	filtersJSON, _ := json.Marshal(input.Filters)
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	item, err := a.updateMonitoringAlertRule(r.Context(), ruleID, input, enabled, ownerType, ownerID, filtersJSON)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "monitoring alert rule not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Msg("Failed to update monitoring alert rule")
		http.Error(w, "failed to update monitoring alert rule", http.StatusInternalServerError)
		return
	}
	writeMonitoringJSON(w, item)
}

func (a *App) handleDeleteMonitoringAlertRule(w http.ResponseWriter, r *http.Request) {
	subject, ok := a.requireMonitoringSubject(w, r)
	if !ok {
		return
	}
	ruleID := strings.TrimSpace(r.PathValue("ruleID"))
	if ruleID == "" {
		http.Error(w, "ruleID is required", http.StatusBadRequest)
		return
	}
	ownerType, ownerID := monitoringSubjectOwner(subject)
	tag, err := a.db.Exec(r.Context(), `
		DELETE FROM monitoring_alert_rules
		WHERE id::text = $1
		  AND owner_subject_type = $2
		  AND owner_subject_id = $3
		  AND managed_by_config_repo = FALSE
	`, ruleID, ownerType, ownerID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete monitoring alert rule")
		http.Error(w, "failed to delete monitoring alert rule", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "monitoring alert rule not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleEvaluateMonitoringAlertRule(w http.ResponseWriter, r *http.Request) {
	subject, ok := a.requireMonitoringSubject(w, r)
	if !ok {
		return
	}
	ruleID := strings.TrimSpace(r.PathValue("ruleID"))
	if ruleID == "" {
		http.Error(w, "ruleID is required", http.StatusBadRequest)
		return
	}
	ownerType, ownerID := monitoringSubjectOwner(subject)
	rule, err := a.loadMonitoringAlertRule(r.Context(), ruleID, ownerType, ownerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "monitoring alert rule not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load monitoring alert rule", http.StatusInternalServerError)
		return
	}
	event, err := a.evaluateMonitoringAlertRule(r, rule)
	if err != nil {
		log.Error().Err(err).Str("rule_id", ruleID).Msg("Failed to evaluate monitoring alert rule")
		http.Error(w, "failed to evaluate monitoring alert rule", http.StatusInternalServerError)
		return
	}
	writeMonitoringJSON(w, event)
}

func (a *App) handleListMonitoringAlertEvents(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	subject, ok := a.requireMonitoringSubject(w, r)
	if !ok {
		return
	}
	ownerType, ownerID := monitoringSubjectOwner(subject)
	rows, err := a.db.Query(r.Context(), `
		SELECT e.id::text, COALESCE(e.rule_id::text, ''), e.status, e.value::float8, e.message,
		       e.started_at, e.resolved_at, e.created_at
		FROM monitoring_alert_events e
		LEFT JOIN monitoring_alert_rules r ON r.id = e.rule_id
		WHERE r.id IS NULL
		   OR r.visibility = 'workspace'
		   OR r.managed_by_config_repo
		   OR (r.owner_subject_type = $1 AND r.owner_subject_id = $2)
		ORDER BY e.created_at DESC
		LIMIT 100
	`, ownerType, ownerID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list monitoring alert events")
		http.Error(w, "failed to list monitoring alert events", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	events := []monitoringAlertEvent{}
	for rows.Next() {
		event, err := scanMonitoringAlertEvent(rows)
		if err != nil {
			http.Error(w, "failed to read monitoring alert events", http.StatusInternalServerError)
			return
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read monitoring alert events", http.StatusInternalServerError)
		return
	}
	writeMonitoringJSON(w, events)
}

func (a *App) handleListMonitoringRecommendations(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	if _, ok := a.requireMonitoringSubject(w, r); !ok {
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	args := []any{}
	condition := "TRUE"
	if status != "" {
		args = append(args, status)
		condition = "status = $1"
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT id::text, fingerprint, category, severity, status, message, metadata,
		       first_seen_at, last_seen_at, resolved_at
		FROM monitoring_recommendations
		WHERE `+condition+`
		ORDER BY CASE status WHEN 'open' THEN 0 WHEN 'acknowledged' THEN 1 ELSE 2 END, last_seen_at DESC
		LIMIT 100
	`, args...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list monitoring recommendations")
		http.Error(w, "failed to list monitoring recommendations", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	recommendations := []monitoringRecommendationRecord{}
	for rows.Next() {
		item, err := scanMonitoringRecommendation(rows)
		if err != nil {
			http.Error(w, "failed to read monitoring recommendations", http.StatusInternalServerError)
			return
		}
		recommendations = append(recommendations, item)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read monitoring recommendations", http.StatusInternalServerError)
		return
	}
	writeMonitoringJSON(w, recommendations)
}

func (a *App) handleAcknowledgeMonitoringRecommendation(w http.ResponseWriter, r *http.Request) {
	a.updateMonitoringRecommendationStatus(w, r, "acknowledged")
}

func (a *App) handleResolveMonitoringRecommendation(w http.ResponseWriter, r *http.Request) {
	a.updateMonitoringRecommendationStatus(w, r, "resolved")
}

func (a *App) updateMonitoringRecommendationStatus(w http.ResponseWriter, r *http.Request, status string) {
	if _, ok := a.requireMonitoringSubject(w, r); !ok {
		return
	}
	recommendationID := strings.TrimSpace(r.PathValue("recommendationID"))
	if recommendationID == "" {
		http.Error(w, "recommendationID is required", http.StatusBadRequest)
		return
	}
	row := a.db.QueryRow(r.Context(), `
		UPDATE monitoring_recommendations
		SET status = $2,
		    resolved_at = CASE WHEN $2 = 'resolved' THEN NOW() ELSE NULL END,
		    last_seen_at = CASE WHEN $2 = 'resolved' THEN last_seen_at ELSE NOW() END
		WHERE id::text = $1
		RETURNING id::text, fingerprint, category, severity, status, message, metadata,
		          first_seen_at, last_seen_at, resolved_at
	`, recommendationID, status)
	item, err := scanMonitoringRecommendation(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "monitoring recommendation not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to update monitoring recommendation", http.StatusInternalServerError)
		return
	}
	writeMonitoringJSON(w, item)
}

func (a *App) insertMonitoringSavedView(ctx context.Context, input monitoringSavedViewInput, ownerType, ownerID string, filtersJSON, columnsJSON []byte) (monitoringSavedViewRecord, error) {
	row := a.db.QueryRow(ctx, `
		INSERT INTO monitoring_saved_views (
			name, owner_subject_type, owner_subject_id, visibility, group_id,
			filters, columns, source, managed_by_config_repo
		)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb,'database',FALSE)
		RETURNING id::text, name, owner_subject_type, owner_subject_id, visibility, group_id,
		          filters, columns, source, managed_by_config_repo, created_at, updated_at
	`, input.Name, ownerType, ownerID, input.Visibility, input.GroupID, string(filtersJSON), string(columnsJSON))
	return scanMonitoringSavedView(row)
}

func (a *App) updateMonitoringSavedView(ctx context.Context, viewID string, input monitoringSavedViewInput, ownerType, ownerID string, filtersJSON, columnsJSON []byte) (monitoringSavedViewRecord, error) {
	row := a.db.QueryRow(ctx, `
		UPDATE monitoring_saved_views
		SET name = $2,
		    visibility = $3,
		    group_id = $4,
		    filters = $5::jsonb,
		    columns = $6::jsonb,
		    updated_at = NOW()
		WHERE id::text = $1
		  AND owner_subject_type = $7
		  AND owner_subject_id = $8
		  AND managed_by_config_repo = FALSE
		RETURNING id::text, name, owner_subject_type, owner_subject_id, visibility, group_id,
		          filters, columns, source, managed_by_config_repo, created_at, updated_at
	`, viewID, input.Name, input.Visibility, input.GroupID, string(filtersJSON), string(columnsJSON), ownerType, ownerID)
	return scanMonitoringSavedView(row)
}

func (a *App) listMonitoringAlertRules(ctx context.Context, ownerType, ownerID string) ([]monitoringAlertRuleRecord, error) {
	rows, err := a.db.Query(ctx, monitoringAlertRuleSelectQuery(`
		WHERE r.visibility = 'workspace'
		   OR (r.owner_subject_type = $1 AND r.owner_subject_id = $2)
		   OR r.managed_by_config_repo
		ORDER BY r.enabled DESC, r.updated_at DESC, r.name ASC
		LIMIT 100
	`), ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := []monitoringAlertRuleRecord{}
	for rows.Next() {
		item, err := scanMonitoringAlertRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, item)
	}
	return rules, rows.Err()
}

func (a *App) loadMonitoringAlertRule(ctx context.Context, ruleID, ownerType, ownerID string) (monitoringAlertRuleRecord, error) {
	row := a.db.QueryRow(ctx, monitoringAlertRuleSelectQuery(`
		WHERE r.id::text = $1
		  AND (
		      r.visibility = 'workspace'
		      OR (r.owner_subject_type = $2 AND r.owner_subject_id = $3)
		      OR r.managed_by_config_repo
		  )
	`), ruleID, ownerType, ownerID)
	return scanMonitoringAlertRule(row)
}

func (a *App) insertMonitoringAlertRule(ctx context.Context, input monitoringAlertRuleInput, enabled bool, ownerType, ownerID string, filtersJSON []byte) (monitoringAlertRuleRecord, error) {
	row := a.db.QueryRow(ctx, `
		INSERT INTO monitoring_alert_rules (
			name, description, enabled, owner_subject_type, owner_subject_id, visibility,
			severity, metric, comparator, threshold, window_seconds, filters,
			source, managed_by_config_repo
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,'database',FALSE)
		RETURNING id::text, name, description, enabled, owner_subject_type, owner_subject_id,
		          visibility, severity, metric, comparator, threshold::float8, window_seconds,
		          filters, source, managed_by_config_repo, created_at, updated_at,
		          NULL::text, NULL::text, NULL::float8, NULL::text, NULL::timestamptz,
		          NULL::timestamptz, NULL::timestamptz
	`, input.Name, input.Description, enabled, ownerType, ownerID, input.Visibility, input.Severity, input.Metric, input.Comparator, input.Threshold, input.WindowSeconds, string(filtersJSON))
	return scanMonitoringAlertRule(row)
}

func (a *App) updateMonitoringAlertRule(ctx context.Context, ruleID string, input monitoringAlertRuleInput, enabled bool, ownerType, ownerID string, filtersJSON []byte) (monitoringAlertRuleRecord, error) {
	row := a.db.QueryRow(ctx, `
		UPDATE monitoring_alert_rules
		SET name = $2,
		    description = $3,
		    enabled = $4,
		    visibility = $5,
		    severity = $6,
		    metric = $7,
		    comparator = $8,
		    threshold = $9,
		    window_seconds = $10,
		    filters = $11::jsonb,
		    updated_at = NOW()
		WHERE id::text = $1
		  AND owner_subject_type = $12
		  AND owner_subject_id = $13
		  AND managed_by_config_repo = FALSE
		RETURNING id::text, name, description, enabled, owner_subject_type, owner_subject_id,
		          visibility, severity, metric, comparator, threshold::float8, window_seconds,
		          filters, source, managed_by_config_repo, created_at, updated_at,
		          NULL::text, NULL::text, NULL::float8, NULL::text, NULL::timestamptz,
		          NULL::timestamptz, NULL::timestamptz
	`, ruleID, input.Name, input.Description, enabled, input.Visibility, input.Severity, input.Metric, input.Comparator, input.Threshold, input.WindowSeconds, string(filtersJSON), ownerType, ownerID)
	return scanMonitoringAlertRule(row)
}

func (a *App) evaluateMonitoringAlertRule(r *http.Request, rule monitoringAlertRuleRecord) (monitoringAlertEvent, error) {
	filters, err := monitoringAlertRuleFilters(rule, time.Now().UTC())
	if err != nil {
		return monitoringAlertEvent{}, err
	}
	value, err := a.monitoringAlertMetricValue(r, rule.Metric, filters)
	if err != nil {
		return monitoringAlertEvent{}, err
	}
	firing := rule.Enabled && monitoringAlertComparatorMatched(rule.Comparator, value, rule.Threshold)
	status := "resolved"
	if firing {
		status = "firing"
	}
	message := fmt.Sprintf("Alert rule %q evaluated %s: %.2f %s %.2f.", rule.Name, status, value, rule.Comparator, rule.Threshold)
	return a.insertMonitoringAlertEvent(r.Context(), rule.ID, status, value, message)
}

func (a *App) monitoringAlertMetricValue(r *http.Request, metric string, filters monitoringAnalyticsFilters) (float64, error) {
	switch metric {
	case "failure_rate":
		runIDs, err := a.visibleMonitoringRunIDs(r, filters)
		if err != nil || len(runIDs) == 0 {
			return 0, err
		}
		var failed, completed int64
		err = a.db.QueryRow(r.Context(), `
			SELECT COUNT(*) FILTER (WHERE LOWER(status) IN ('failure', 'failed')),
			       COUNT(*) FILTER (WHERE LOWER(status) IN ('success', 'succeeded', 'failure', 'failed', 'cancelled'))
			FROM pipeline_runs
			WHERE run_id::text = ANY($1)
		`, runIDs).Scan(&failed, &completed)
		if err != nil || completed == 0 {
			return 0, err
		}
		return float64(failed) / float64(completed), nil
	case "p95_duration_seconds":
		runIDs, err := a.visibleMonitoringRunIDs(r, filters)
		if err != nil || len(runIDs) == 0 {
			return 0, err
		}
		var value float64
		err = a.db.QueryRow(r.Context(), `
			SELECT COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (
				ORDER BY EXTRACT(EPOCH FROM finished_at - started_at)
			), 0)::float8
			FROM pipeline_runs
			WHERE run_id::text = ANY($1)
			  AND started_at IS NOT NULL
			  AND finished_at IS NOT NULL
			  AND finished_at >= started_at
		`, runIDs).Scan(&value)
		return value, err
	case "ai_tokens":
		runIDs, err := a.visibleMonitoringRunIDs(r, filters)
		if err != nil || len(runIDs) == 0 {
			return 0, err
		}
		var value float64
		err = a.db.QueryRow(r.Context(), `
			SELECT COALESCE(SUM(total_tokens), 0)::float8
			FROM ai_usage_events
			WHERE run_id::text = ANY($1)
			  AND created_at >= $2 AND created_at <= $3
		`, runIDs, filters.From, filters.To).Scan(&value)
		return value, err
	case "external_trigger_failures":
		triggerIDs, err := a.visibleMonitoringExternalTriggerIDs(r, filters)
		if err != nil || len(triggerIDs) == 0 {
			return 0, err
		}
		var value float64
		err = a.db.QueryRow(r.Context(), `
			SELECT COUNT(*)::float8
			FROM external_trigger_invocations
			WHERE trigger_id = ANY($1)
			  AND created_at >= $2 AND created_at <= $3
			  AND LOWER(status) IN ('failed', 'failure')
		`, triggerIDs, filters.From, filters.To).Scan(&value)
		return value, err
	case "queued_jobs":
		status, err := a.fetchDispatcherStatus(r.Context())
		if err != nil {
			return 0, err
		}
		a.sampleMonitoringRunnerSnapshots(r.Context(), status)
		return float64(status.GetQueuedJobs()), nil
	case "runner_utilization":
		status, err := a.fetchDispatcherStatus(r.Context())
		if err != nil {
			return 0, err
		}
		a.sampleMonitoringRunnerSnapshots(r.Context(), status)
		_, summary := monitoringRunnersFromDispatcherStatus(status, nil)
		if summary.Capacity == 0 {
			return 0, nil
		}
		return float64(summary.ActiveJobs) / float64(summary.Capacity), nil
	default:
		return 0, fmt.Errorf("unsupported metric %q", metric)
	}
}

func (a *App) insertMonitoringAlertEvent(ctx context.Context, ruleID, status string, value float64, message string) (monitoringAlertEvent, error) {
	row := a.db.QueryRow(ctx, `
		INSERT INTO monitoring_alert_events (rule_id, status, value, message, started_at, resolved_at)
		VALUES ($1,$2,$3,$4,NOW(),CASE WHEN $2 = 'resolved' THEN NOW() ELSE NULL END)
		RETURNING id::text, COALESCE(rule_id::text, ''), status, value::float8, message,
		          started_at, resolved_at, created_at
	`, ruleID, status, value, message)
	return scanMonitoringAlertEvent(row)
}

func (a *App) persistMonitoringRecommendations(ctx context.Context, recommendations []string) error {
	if a == nil || a.db == nil || len(recommendations) == 0 {
		return nil
	}
	metadataJSON, _ := json.Marshal(map[string]any{"source": "monitoring_efficiency"})
	for _, recommendation := range recommendations {
		message := strings.TrimSpace(recommendation)
		if message == "" {
			continue
		}
		fingerprint := monitoringRecommendationFingerprint("efficiency", message)
		if _, err := a.db.Exec(ctx, `
			INSERT INTO monitoring_recommendations (fingerprint, category, severity, status, message, metadata)
			VALUES ($1,'efficiency','info','open',$2,$3::jsonb)
			ON CONFLICT (fingerprint) DO UPDATE
			SET category = EXCLUDED.category,
			    severity = EXCLUDED.severity,
			    status = 'open',
			    message = EXCLUDED.message,
			    metadata = EXCLUDED.metadata,
			    last_seen_at = NOW(),
			    resolved_at = NULL
		`, fingerprint, message, string(metadataJSON)); err != nil {
			return err
		}
	}
	return nil
}

func monitoringAlertRuleSelectQuery(suffix string) string {
	return `
		SELECT r.id::text, r.name, r.description, r.enabled, r.owner_subject_type, r.owner_subject_id,
		       r.visibility, r.severity, r.metric, r.comparator, r.threshold::float8, r.window_seconds,
		       r.filters, r.source, r.managed_by_config_repo, r.created_at, r.updated_at,
		       e.id::text, e.status, e.value::float8, e.message, e.started_at, e.resolved_at, e.created_at
		FROM monitoring_alert_rules r
		LEFT JOIN LATERAL (
			SELECT *
			FROM monitoring_alert_events
			WHERE rule_id = r.id
			ORDER BY created_at DESC
			LIMIT 1
		) e ON TRUE
	` + suffix
}

type monitoringScanner interface {
	Scan(dest ...any) error
}

func scanMonitoringSavedView(row monitoringScanner) (monitoringSavedViewRecord, error) {
	var item monitoringSavedViewRecord
	var groupID sql.NullInt64
	var filtersJSON, columnsJSON []byte
	if err := row.Scan(&item.ID, &item.Name, &item.OwnerSubjectType, &item.OwnerSubjectID, &item.Visibility, &groupID,
		&filtersJSON, &columnsJSON, &item.Source, &item.ManagedByConfigRepo, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return item, err
	}
	if groupID.Valid {
		id := int(groupID.Int64)
		item.GroupID = &id
	}
	item.Filters = map[string]any{}
	item.Columns = []string{}
	_ = json.Unmarshal(filtersJSON, &item.Filters)
	_ = json.Unmarshal(columnsJSON, &item.Columns)
	return item, nil
}

func scanMonitoringAlertRule(row monitoringScanner) (monitoringAlertRuleRecord, error) {
	var item monitoringAlertRuleRecord
	var filtersJSON []byte
	var eventID, eventStatus, eventMessage sql.NullString
	var eventValue sql.NullFloat64
	var eventStartedAt, eventResolvedAt, eventCreatedAt sql.NullTime
	if err := row.Scan(&item.ID, &item.Name, &item.Description, &item.Enabled, &item.OwnerSubjectType, &item.OwnerSubjectID,
		&item.Visibility, &item.Severity, &item.Metric, &item.Comparator, &item.Threshold, &item.WindowSeconds,
		&filtersJSON, &item.Source, &item.ManagedByConfigRepo, &item.CreatedAt, &item.UpdatedAt,
		&eventID, &eventStatus, &eventValue, &eventMessage, &eventStartedAt, &eventResolvedAt, &eventCreatedAt); err != nil {
		return item, err
	}
	item.Filters = map[string]any{}
	_ = json.Unmarshal(filtersJSON, &item.Filters)
	if eventID.Valid {
		item.LastEvent = &monitoringAlertEvent{
			ID:        eventID.String,
			RuleID:    item.ID,
			Status:    eventStatus.String,
			Value:     eventValue.Float64,
			Message:   eventMessage.String,
			StartedAt: eventStartedAt.Time,
			CreatedAt: eventCreatedAt.Time,
		}
		if eventResolvedAt.Valid {
			item.LastEvent.ResolvedAt = &eventResolvedAt.Time
		}
	}
	return item, nil
}

func scanMonitoringAlertEvent(row monitoringScanner) (monitoringAlertEvent, error) {
	var item monitoringAlertEvent
	var ruleID sql.NullString
	var resolvedAt sql.NullTime
	if err := row.Scan(&item.ID, &ruleID, &item.Status, &item.Value, &item.Message, &item.StartedAt, &resolvedAt, &item.CreatedAt); err != nil {
		return item, err
	}
	if ruleID.Valid {
		item.RuleID = ruleID.String
	}
	if resolvedAt.Valid {
		item.ResolvedAt = &resolvedAt.Time
	}
	return item, nil
}

func scanMonitoringRecommendation(row monitoringScanner) (monitoringRecommendationRecord, error) {
	var item monitoringRecommendationRecord
	var metadataJSON []byte
	var resolvedAt sql.NullTime
	if err := row.Scan(&item.ID, &item.Fingerprint, &item.Category, &item.Severity, &item.Status, &item.Message,
		&metadataJSON, &item.FirstSeenAt, &item.LastSeenAt, &resolvedAt); err != nil {
		return item, err
	}
	item.Metadata = map[string]any{}
	_ = json.Unmarshal(metadataJSON, &item.Metadata)
	if resolvedAt.Valid {
		item.ResolvedAt = &resolvedAt.Time
	}
	return item, nil
}

func (a *App) requireMonitoringSubject(w http.ResponseWriter, r *http.Request) (model.Subject, bool) {
	if a == nil || a.db == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return model.Subject{}, false
	}
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "missing authorization subject", http.StatusUnauthorized)
		return model.Subject{}, false
	}
	return subject, true
}

func monitoringSubjectOwner(subject model.Subject) (string, string) {
	subjectType := strings.TrimSpace(subject.Type)
	if subjectType == "" {
		subjectType = model.SubjectTypeUser
	}
	subjectID := strings.TrimSpace(subject.ID)
	if subjectID == "" {
		subjectID = strings.TrimSpace(subject.Sub)
	}
	if subjectID == "" {
		subjectID = strings.TrimSpace(subject.Email)
	}
	return subjectType, subjectID
}

func normalizeMonitoringSavedViewInput(input *monitoringSavedViewInput) error {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(input.Name) > 120 {
		return fmt.Errorf("name must be 120 characters or fewer")
	}
	input.Visibility = normalizeMonitoringVisibility(input.Visibility, "private")
	if input.Filters == nil {
		input.Filters = map[string]any{}
	}
	if input.Columns == nil {
		input.Columns = []string{}
	}
	return nil
}

func normalizeMonitoringAlertRuleInput(input *monitoringAlertRuleInput) error {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(input.Name) > 120 {
		return fmt.Errorf("name must be 120 characters or fewer")
	}
	input.Description = strings.TrimSpace(input.Description)
	input.Visibility = normalizeMonitoringVisibility(input.Visibility, "workspace")
	input.Severity = normalizeMonitoringSeverity(input.Severity)
	input.Metric = strings.TrimSpace(input.Metric)
	if !supportedMonitoringAlertMetrics()[input.Metric] {
		return fmt.Errorf("unsupported metric")
	}
	input.Comparator = strings.ToLower(strings.TrimSpace(input.Comparator))
	if input.Comparator == "" {
		input.Comparator = "gt"
	}
	if !supportedMonitoringAlertComparators()[input.Comparator] {
		return fmt.Errorf("unsupported comparator")
	}
	if math.IsNaN(input.Threshold) || math.IsInf(input.Threshold, 0) {
		return fmt.Errorf("threshold is invalid")
	}
	if input.WindowSeconds == 0 {
		input.WindowSeconds = 3600
	}
	if input.WindowSeconds < 60 || input.WindowSeconds > int((30*24*time.Hour)/time.Second) {
		return fmt.Errorf("window_seconds must be between 60 and 2592000")
	}
	if input.Filters == nil {
		input.Filters = map[string]any{}
	}
	return nil
}

func normalizeMonitoringVisibility(value, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "private", "group", "workspace":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return fallback
	}
}

func normalizeMonitoringSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "info", "warning", "critical":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "warning"
	}
}

func supportedMonitoringAlertMetrics() map[string]bool {
	return map[string]bool{
		"failure_rate":              true,
		"p95_duration_seconds":      true,
		"queued_jobs":               true,
		"runner_utilization":        true,
		"ai_tokens":                 true,
		"external_trigger_failures": true,
	}
}

func supportedMonitoringAlertComparators() map[string]bool {
	return map[string]bool{
		"gt":  true,
		"gte": true,
		"lt":  true,
		"lte": true,
		"eq":  true,
	}
}

func monitoringAlertComparatorMatched(comparator string, value, threshold float64) bool {
	switch strings.ToLower(strings.TrimSpace(comparator)) {
	case "gt":
		return value > threshold
	case "gte":
		return value >= threshold
	case "lt":
		return value < threshold
	case "lte":
		return value <= threshold
	case "eq":
		return math.Abs(value-threshold) < 0.000001
	default:
		return false
	}
}

func monitoringAlertRuleFilters(rule monitoringAlertRuleRecord, now time.Time) (monitoringAnalyticsFilters, error) {
	window := time.Duration(rule.WindowSeconds) * time.Second
	if window <= 0 {
		window = time.Hour
	}
	filters := monitoringAnalyticsFilters{
		From: now.Add(-window),
		To:   now,
	}
	for key, value := range rule.Filters {
		if value == nil {
			continue
		}
		text := strings.TrimSpace(monitoringStringFromAny(value))
		switch key {
		case "groupId":
			if strings.EqualFold(text, rootGrantID) {
				filters.RootGroup = true
				continue
			}
			parsed, err := monitoringIntFromAny(value)
			if err != nil {
				return filters, fmt.Errorf("invalid groupId")
			}
			filters.GroupID = &parsed
		case "pipelinePath":
			filters.PipelinePath = text
		case "pipelineName":
			filters.PipelineName = text
		case "repo":
			filters.Repo = text
		case "ref", "branch":
			filters.Ref = text
			if key == "branch" && text != "" && !strings.HasPrefix(text, "refs/") {
				filters.Ref = "refs/heads/" + text
			}
		case "commitSHA", "commitSha", "commit":
			filters.CommitSHA = text
		case "triggerSource":
			filters.TriggerSource = text
		case "status":
			filters.Status = text
		case "requestedByType":
			filters.RequestedByType = text
		case "requestedById":
			filters.RequestedByID = text
		case "effectiveSubjectType":
			filters.EffectiveSubjectType = text
		case "effectiveSubjectId":
			filters.EffectiveSubjectID = text
		case "externalTriggerId":
			filters.ExternalTriggerID = text
		case "scheduleId":
			filters.ScheduleID = text
		case "minDurationSeconds":
			parsed, err := monitoringFloatFromAny(value)
			if err != nil || parsed < 0 {
				return filters, fmt.Errorf("invalid minDurationSeconds")
			}
			filters.MinDurationSeconds = &parsed
		case "maxDurationSeconds":
			parsed, err := monitoringFloatFromAny(value)
			if err != nil || parsed < 0 {
				return filters, fmt.Errorf("invalid maxDurationSeconds")
			}
			filters.MaxDurationSeconds = &parsed
		}
	}
	return filters, nil
}

func monitoringStringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		if math.Trunc(typed) == typed {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return fmt.Sprint(value)
	}
}

func monitoringIntFromAny(value any) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case float64:
		if math.Trunc(typed) != typed {
			return 0, fmt.Errorf("not an integer")
		}
		return int(typed), nil
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err
	default:
		return strconv.Atoi(strings.TrimSpace(monitoringStringFromAny(value)))
	}
}

func monitoringFloatFromAny(value any) (float64, error) {
	switch typed := value.(type) {
	case float64:
		return typed, nil
	case int:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case json.Number:
		return typed.Float64()
	default:
		return strconv.ParseFloat(strings.TrimSpace(monitoringStringFromAny(value)), 64)
	}
}

func monitoringRecommendationFingerprint(category, message string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(category) + "\x00" + strings.TrimSpace(message)))
	return hex.EncodeToString(sum[:])
}

func decodeMonitoringJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return false
	}
	return true
}

func writeMonitoringJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

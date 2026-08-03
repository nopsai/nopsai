package nopsai

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"

	"nopsai/pkg/httpapi"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/pkg/routeauthz"
)

const (
	externalTriggerStatusPending = "pending"
	externalTriggerStatusQueued  = "queued"
	externalTriggerStatusFailed  = "failed"
)

var externalTriggerIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,160}$`)

type externalTriggerAllowedCaller struct {
	Type string `json:"type" yaml:"type"`
	ID   string `json:"id" yaml:"id"`
}

type externalTriggerRecord struct {
	ID              string                         `json:"id"`
	Name            string                         `json:"name"`
	Description     string                         `json:"description"`
	Enabled         bool                           `json:"enabled"`
	Pipeline        string                         `json:"pipeline"`
	Scope           string                         `json:"scope"`
	RunTeamPath     string                         `json:"run_team_path,omitempty"`
	AllowedCallers  []externalTriggerAllowedCaller `json:"allowed_callers"`
	VariableMapping map[string]string              `json:"variable_mapping"`
	PayloadSchema   map[string]any                 `json:"payload_schema"`
	RateLimit       map[string]any                 `json:"rate_limit"`
	CreatedBy       string                         `json:"created_by"`
	CreatedAt       time.Time                      `json:"created_at"`
	UpdatedAt       time.Time                      `json:"updated_at"`
	LastUsedAt      *time.Time                     `json:"last_used_at,omitempty"`
	Source          string                         `json:"source,omitempty"`
	ConfigRepoID    *int64                         `json:"config_repo_id,omitempty"`
	ConfigSource    string                         `json:"config_source_path,omitempty"`
	ManagedByGitOps bool                           `json:"managed_by_config_repo,omitempty"`
}

type externalTriggerInput struct {
	ID              string                         `json:"id"`
	Name            string                         `json:"name"`
	Description     string                         `json:"description"`
	Enabled         *bool                          `json:"enabled"`
	Pipeline        string                         `json:"pipeline"`
	Scope           string                         `json:"scope"`
	RunTeamPath     string                         `json:"run_team_path"`
	AllowedCallers  []externalTriggerAllowedCaller `json:"allowed_callers"`
	VariableMapping map[string]string              `json:"variable_mapping"`
	PayloadSchema   map[string]any                 `json:"payload_schema"`
	RateLimit       map[string]any                 `json:"rate_limit"`
}

type externalTriggerInvokeRequest struct {
	EventType      string            `json:"event_type"`
	IdempotencyKey string            `json:"idempotency_key"`
	Variables      map[string]string `json:"variables"`
	Payload        map[string]any    `json:"payload"`
}

type externalTriggerInvokeResponse struct {
	RunID          string `json:"run_id"`
	TriggerEventID string `json:"trigger_event_id"`
	Status         string `json:"status"`
}

type externalTriggerInvocationRecord struct {
	ID             string    `json:"id"`
	TriggerID      string    `json:"trigger_id"`
	CallerType     string    `json:"caller_type"`
	CallerID       string    `json:"caller_id"`
	Status         string    `json:"status"`
	RunID          string    `json:"run_id,omitempty"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	EventType      string    `json:"event_type,omitempty"`
	SourceIP       string    `json:"source_ip,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	Error          string    `json:"error,omitempty"`
}

func (a *App) handleListExternalTriggers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT id, name, description, enabled, pipeline, scope, COALESCE(run_team_path, ''), allowed_callers, variable_mapping,
		       payload_schema, rate_limit, created_by, created_at, updated_at, last_used_at,
		       COALESCE(source, 'database'), config_repo_id, COALESCE(config_source_path, ''), managed_by_config_repo
		FROM external_triggers
		ORDER BY updated_at DESC, id ASC
	`)
	if err != nil {
		http.Error(w, "failed to list external triggers", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	triggers := []externalTriggerRecord{}
	resources := []model.ResourceRef{}
	for rows.Next() {
		trigger, err := scanExternalTrigger(rows)
		if err != nil {
			http.Error(w, "failed to read external triggers", http.StatusInternalServerError)
			return
		}
		triggers = append(triggers, trigger)
		resources = append(resources, routeauthz.ExternalTriggerResource(trigger.ID))
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read external triggers", http.StatusInternalServerError)
		return
	}

	allowedSet, err := a.allowedResourceSet(r, "external_trigger.read", resources)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	filtered := make([]externalTriggerRecord, 0, len(triggers))
	for _, trigger := range triggers {
		if _, ok := allowedSet[resourceKey(routeauthz.ExternalTriggerResource(trigger.ID))]; ok {
			filtered = append(filtered, trigger)
		}
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, filtered)
}

func (a *App) handleCreateExternalTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req externalTriggerInput
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "missing authorization subject", http.StatusUnauthorized)
		return
	}
	trigger, err := normalizeExternalTriggerInput(req, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !a.requireTeamOwnedCreateDecision(w, r, "external_trigger.create", routeauthz.ExternalTriggerResource(trigger.ID), trigger.RunTeamPath) {
		return
	}
	if err := a.validateExternalTriggerPipeline(r.Context(), trigger.Pipeline); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	allowedJSON, _ := json.Marshal(trigger.AllowedCallers)
	mappingJSON, _ := json.Marshal(trigger.VariableMapping)
	schemaJSON, _ := json.Marshal(trigger.PayloadSchema)
	rateLimitJSON, _ := json.Marshal(trigger.RateLimit)
	createdBy := formatSubjectLabel(subject.Type, firstNonEmptyString(subject.ID, subject.Sub, subject.Email))

	if _, err := a.db.Exec(r.Context(), `
		INSERT INTO external_triggers
			(id, name, description, enabled, pipeline, scope, run_team_path, allowed_callers, variable_mapping,
			 payload_schema, rate_limit, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::jsonb, $10::jsonb, $11::jsonb, $12)
	`, trigger.ID, trigger.Name, trigger.Description, trigger.Enabled, trigger.Pipeline, trigger.Scope,
		trigger.RunTeamPath, string(allowedJSON), string(mappingJSON), string(schemaJSON), string(rateLimitJSON), createdBy); err != nil {
		http.Error(w, "failed to create external trigger", http.StatusInternalServerError)
		return
	}
	created, err := a.loadExternalTrigger(r.Context(), trigger.ID)
	if err != nil {
		http.Error(w, "failed to load external trigger", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusCreated, created)
}

func (a *App) handleGetExternalTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	trigger, err := a.loadExternalTrigger(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "external trigger not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load external trigger", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, trigger)
}

func (a *App) handleUpdateExternalTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	var req externalTriggerInput
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	trigger, err := normalizeExternalTriggerInput(req, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err = a.loadExternalTrigger(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "external trigger not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load external trigger", http.StatusInternalServerError)
		return
	}
	if err := a.validateExternalTriggerPipeline(r.Context(), trigger.Pipeline); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	allowedJSON, _ := json.Marshal(trigger.AllowedCallers)
	mappingJSON, _ := json.Marshal(trigger.VariableMapping)
	schemaJSON, _ := json.Marshal(trigger.PayloadSchema)
	rateLimitJSON, _ := json.Marshal(trigger.RateLimit)

	tag, err := a.db.Exec(r.Context(), `
		UPDATE external_triggers
		SET name = $2,
		    description = $3,
		    enabled = $4,
		    pipeline = $5,
		    scope = $6,
		    run_team_path = $7,
		    allowed_callers = $8::jsonb,
		    variable_mapping = $9::jsonb,
		    payload_schema = $10::jsonb,
		    rate_limit = $11::jsonb,
		    source = 'database',
		    config_repo_id = NULL,
		    config_source_path = '',
		    config_source_commit_sha = '',
		    managed_by_config_repo = FALSE,
		    updated_at = NOW()
		WHERE id = $1
	`, id, trigger.Name, trigger.Description, trigger.Enabled, trigger.Pipeline, trigger.Scope,
		trigger.RunTeamPath, string(allowedJSON), string(mappingJSON), string(schemaJSON), string(rateLimitJSON))
	if err != nil {
		http.Error(w, "failed to update external trigger", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "external trigger not found", http.StatusNotFound)
		return
	}
	updated, err := a.loadExternalTrigger(r.Context(), id)
	if err != nil {
		http.Error(w, "failed to load external trigger", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, updated)
}

func (a *App) handleDeleteExternalTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	_, err := a.loadExternalTrigger(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "external trigger not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load external trigger", http.StatusInternalServerError)
		return
	}
	tag, err := a.db.Exec(r.Context(), `DELETE FROM external_triggers WHERE id = $1`, id)
	if err != nil {
		http.Error(w, "failed to delete external trigger", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "external trigger not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleListExternalTriggerInvocations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = min(parsed, 200)
		}
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT id::text, trigger_id, caller_type, caller_id, status, COALESCE(run_id::text, ''),
		       idempotency_key, event_type, source_ip, created_at, error
		FROM external_trigger_invocations
		WHERE trigger_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, strings.TrimSpace(r.PathValue("id")), limit)
	if err != nil {
		http.Error(w, "failed to list external trigger invocations", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	records := []externalTriggerInvocationRecord{}
	for rows.Next() {
		var record externalTriggerInvocationRecord
		if err := rows.Scan(&record.ID, &record.TriggerID, &record.CallerType, &record.CallerID, &record.Status,
			&record.RunID, &record.IdempotencyKey, &record.EventType, &record.SourceIP, &record.CreatedAt, &record.Error); err != nil {
			http.Error(w, "failed to read external trigger invocations", http.StatusInternalServerError)
			return
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read external trigger invocations", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, records)
}

func (a *App) handleInvokeExternalTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	trigger, err := a.loadExternalTrigger(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "external trigger not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load external trigger", http.StatusInternalServerError)
		return
	}
	var payload externalTriggerInvokeRequest
	if err := httpapi.DecodeOptionalJSON(r, &payload); err != nil {
		http.Error(w, "invalid invoke payload", http.StatusBadRequest)
		return
	}
	callerType, callerID, subject, err := externalTriggerCallerFromRequest(a, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	idempotencyKey := strings.TrimSpace(payload.IdempotencyKey)
	if idempotencyKey != "" {
		existing, found, err := a.findExternalTriggerInvocationByIdempotency(r.Context(), trigger.ID, callerType, callerID, idempotencyKey)
		if err != nil {
			http.Error(w, "failed to validate idempotency key", http.StatusInternalServerError)
			return
		}
		if found {
			if existing.RunID != "" && existing.Status == externalTriggerStatusQueued {
				_ = httpapi.WriteJSON(w, http.StatusOK, externalTriggerInvokeResponse{
					RunID:          existing.RunID,
					TriggerEventID: existing.ID,
					Status:         externalTriggerStatusQueued,
				})
				return
			}
			http.Error(w, "idempotency key is already being processed", http.StatusConflict)
			return
		}
	}

	invocationID := uuid.New()
	sourceIP := externalTriggerSourceIP(r)
	eventType := strings.TrimSpace(payload.EventType)
	if err := a.insertExternalTriggerInvocation(r.Context(), invocationID, trigger.ID, callerType, callerID, externalTriggerStatusPending, "", idempotencyKey, eventType, sourceIP, ""); err != nil {
		http.Error(w, "failed to record external trigger invocation", http.StatusInternalServerError)
		return
	}
	failInvocation := func(status int, msg string) {
		_ = a.updateExternalTriggerInvocation(r.Context(), invocationID, externalTriggerStatusFailed, "", msg)
		http.Error(w, msg, status)
	}

	if !trigger.Enabled {
		failInvocation(http.StatusForbidden, "external trigger is disabled")
		return
	}
	if !a.externalTriggerCallerAllowed(r.Context(), trigger.AllowedCallers, subject, callerType, callerID) {
		failInvocation(http.StatusForbidden, "caller is not configured as an allowed caller for this external trigger")
		return
	}
	decision, err := a.aaaCheck(r.Context(), subject, "external_trigger.invoke", routeauthz.ExternalTriggerResource(trigger.ID), a.aaaRequestContext(r))
	if err != nil {
		failInvocation(http.StatusServiceUnavailable, "authorization unavailable")
		return
	}
	if !decision.Allowed {
		failInvocation(http.StatusForbidden, "caller is missing external_trigger.invoke permission for this external trigger")
		return
	}
	if exceeded, message, err := a.externalTriggerRateLimitExceeded(r.Context(), trigger.ID, trigger.RateLimit); err != nil {
		failInvocation(http.StatusInternalServerError, "failed to validate external trigger rate limit")
		return
	} else if exceeded {
		failInvocation(http.StatusTooManyRequests, message)
		return
	}
	if err := validateExternalTriggerPayloadSchema(trigger.PayloadSchema, payload.Payload); err != nil {
		failInvocation(http.StatusBadRequest, err.Error())
		return
	}
	variables, err := applyExternalTriggerVariableMapping(payload, trigger.VariableMapping)
	if err != nil {
		failInvocation(http.StatusBadRequest, err.Error())
		return
	}

	runID, triggerEventID, runErr := a.startExternalTriggerRun(r, trigger, invocationID.String(), callerType, callerID, eventType, variables)
	if runErr != nil {
		failInvocation(runErr.status, runErr.message)
		return
	}

	if err := a.updateExternalTriggerInvocation(r.Context(), invocationID, externalTriggerStatusQueued, runID, ""); err != nil {
		log.Warn().Err(err).Str("invocation_id", invocationID.String()).Msg("failed to update external trigger invocation")
	}
	_, _ = a.db.Exec(r.Context(), `UPDATE external_triggers SET last_used_at = NOW() WHERE id = $1`, trigger.ID)
	_ = httpapi.WriteJSON(w, http.StatusAccepted, externalTriggerInvokeResponse{
		RunID:          runID,
		TriggerEventID: triggerEventID,
		Status:         externalTriggerStatusQueued,
	})
}

type externalTriggerRunError struct {
	status  int
	message string
}

func (a *App) startExternalTriggerRun(
	original *http.Request,
	trigger externalTriggerRecord,
	triggerEventID,
	callerType,
	callerID,
	eventType string,
	variables map[string]string,
) (string, string, *externalTriggerRunError) {
	body, _ := json.Marshal(runRequestPayload{
		Pipeline:  trigger.Pipeline,
		Scope:     trigger.Scope,
		Variables: variables,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/run", bytes.NewReader(body)).WithContext(original.Context())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Nopsai-Trigger-Source", "external_trigger")
	req.Header.Set("X-Nopsai-Trigger-Event-ID", triggerEventID)
	req.Header.Set("X-Nopsai-Caller-Type", callerType)
	req.Header.Set("X-Nopsai-Caller-ID", callerID)
	req.Header.Set("X-Nopsai-Pipeline-Source", "external_trigger")
	req.Header.Set("X-Nopsai-External-Trigger-ID", trigger.ID)
	req.Header.Set("X-Nopsai-External-Trigger-Name", trigger.Name)
	if scope := strings.TrimSpace(trigger.Scope); scope != "" {
		req.Header.Set("X-Nopsai-Scope", scope)
	}
	if teamPath := effectiveExternalTriggerRunTeamPath(trigger); teamPath != "" {
		req.Header.Set("X-Nopsai-Team-Path", teamPath)
	}
	if event := strings.TrimSpace(eventType); event != "" {
		req.Header.Set("X-Nopsai-External-Event-Type", event)
	}
	rec := httptest.NewRecorder()
	a.handleRunPipeline(rec, req)
	if rec.Code < 200 || rec.Code >= 300 {
		message := strings.TrimSpace(rec.Body.String())
		if message == "" {
			message = "failed to start external trigger run"
		}
		status := rec.Code
		if status < 400 {
			status = http.StatusInternalServerError
		}
		return "", "", &externalTriggerRunError{status: status, message: message}
	}
	var response struct {
		RunID          string `json:"run_id"`
		TriggerEventID string `json:"trigger_event_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil || strings.TrimSpace(response.RunID) == "" {
		return "", "", &externalTriggerRunError{status: http.StatusInternalServerError, message: "run started but response was invalid"}
	}
	if strings.TrimSpace(response.TriggerEventID) == "" {
		response.TriggerEventID = triggerEventID
	}
	return response.RunID, response.TriggerEventID, nil
}

func scanExternalTrigger(scanner interface{ Scan(...any) error }) (externalTriggerRecord, error) {
	var trigger externalTriggerRecord
	var allowedJSON, mappingJSON, schemaJSON, rateLimitJSON []byte
	var lastUsed sql.NullTime
	var configRepoID sql.NullInt64
	if err := scanner.Scan(&trigger.ID, &trigger.Name, &trigger.Description, &trigger.Enabled, &trigger.Pipeline,
		&trigger.Scope, &trigger.RunTeamPath, &allowedJSON, &mappingJSON, &schemaJSON, &rateLimitJSON, &trigger.CreatedBy,
		&trigger.CreatedAt, &trigger.UpdatedAt, &lastUsed, &trigger.Source, &configRepoID, &trigger.ConfigSource,
		&trigger.ManagedByGitOps); err != nil {
		return trigger, err
	}
	if lastUsed.Valid {
		trigger.LastUsedAt = &lastUsed.Time
	}
	if configRepoID.Valid {
		id := configRepoID.Int64
		trigger.ConfigRepoID = &id
	}
	_ = decodeJSONWithDefault(allowedJSON, &trigger.AllowedCallers, []externalTriggerAllowedCaller{})
	_ = decodeJSONWithDefault(mappingJSON, &trigger.VariableMapping, map[string]string{})
	_ = decodeJSONWithDefault(schemaJSON, &trigger.PayloadSchema, map[string]any{})
	_ = decodeJSONWithDefault(rateLimitJSON, &trigger.RateLimit, map[string]any{})
	return trigger, nil
}

func decodeJSONWithDefault[T any](raw []byte, target *T, fallback T) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		*target = fallback
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		*target = fallback
		return err
	}
	return nil
}

func (a *App) loadExternalTrigger(ctx context.Context, id string) (externalTriggerRecord, error) {
	return scanExternalTrigger(a.db.QueryRow(ctx, `
		SELECT id, name, description, enabled, pipeline, scope, COALESCE(run_team_path, ''), allowed_callers, variable_mapping,
		       payload_schema, rate_limit, created_by, created_at, updated_at, last_used_at,
		       COALESCE(source, 'database'), config_repo_id, COALESCE(config_source_path, ''), managed_by_config_repo
		FROM external_triggers
		WHERE id = $1
	`, strings.TrimSpace(id)))
}

func normalizeExternalTriggerInput(req externalTriggerInput, pathID string) (externalTriggerRecord, error) {
	id := strings.TrimSpace(firstNonEmptyString(pathID, req.ID))
	name := strings.TrimSpace(req.Name)
	if id == "" {
		id = slugifyExternalTriggerID(name)
	}
	if !externalTriggerIDPattern.MatchString(id) {
		return externalTriggerRecord{}, fmt.Errorf("id must be 1-160 characters using letters, numbers, dots, underscores, or hyphens")
	}
	if name == "" {
		name = id
	}
	pipeline := normalizeExternalTriggerPipeline(req.Pipeline)
	if pipeline == "" {
		return externalTriggerRecord{}, fmt.Errorf("pipeline is required")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	allowed, err := normalizeExternalTriggerAllowedCallers(req.AllowedCallers)
	if err != nil {
		return externalTriggerRecord{}, err
	}
	mapping, err := normalizeExternalTriggerVariableMapping(req.VariableMapping)
	if err != nil {
		return externalTriggerRecord{}, err
	}
	runTeamPath, err := normalizeRunTeamPath(req.RunTeamPath)
	if err != nil {
		return externalTriggerRecord{}, fmt.Errorf("invalid run_team_path: %w", err)
	}
	scope := strings.Trim(strings.TrimSpace(req.Scope), "/")
	if normalizedScope, globalOnly := stripGlobalPathPrefix(scope); globalOnly {
		scope = ""
	} else {
		scope = normalizedScope
	}
	return externalTriggerRecord{
		ID:              id,
		Name:            name,
		Description:     strings.TrimSpace(req.Description),
		Enabled:         enabled,
		Pipeline:        pipeline,
		Scope:           scope,
		RunTeamPath:     runTeamPath,
		AllowedCallers:  allowed,
		VariableMapping: mapping,
		PayloadSchema:   normalizeObjectMap(req.PayloadSchema),
		RateLimit:       normalizeObjectMap(req.RateLimit),
	}, nil
}

func effectiveExternalTriggerRunTeamPath(trigger externalTriggerRecord) string {
	if teamPath := strings.Trim(strings.TrimSpace(trigger.RunTeamPath), "/"); teamPath != "" {
		return teamPath
	}
	return globalGrantID
}

func normalizeExternalTriggerPipeline(value string) string {
	pipeline, _ := normalizeExternalTriggerPipelineReference(value)
	return pipeline
}

func normalizeExternalTriggerPipelineReference(value string) (string, bool) {
	value = strings.Trim(strings.TrimSpace(value), "/")
	value = strings.TrimPrefix(value, ".nopsai/")
	value = strings.TrimPrefix(value, "pipelines/")
	value = strings.TrimSuffix(strings.TrimSuffix(value, ".yaml"), ".yml")
	value = strings.Trim(value, "/")
	if value == "" {
		return "", false
	}
	pipeline, globalQualified, err := configsync.NormalizePipelineIdentifierReference(value)
	if err != nil {
		return value, globalQualified
	}
	return pipeline, globalQualified
}

func slugifyExternalTriggerID(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = regexp.MustCompile(`[^a-z0-9_.-]+`).ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		return "external-trigger"
	}
	return name
}

func normalizeExternalTriggerAllowedCallers(callers []externalTriggerAllowedCaller) ([]externalTriggerAllowedCaller, error) {
	out := make([]externalTriggerAllowedCaller, 0, len(callers))
	seen := map[string]struct{}{}
	for _, caller := range callers {
		callerType := strings.ToLower(strings.TrimSpace(caller.Type))
		switch callerType {
		case "team":
			callerType = model.SubjectTypeAuthTeam
		case model.SubjectTypeUser, model.SubjectTypeServiceAccount, model.SubjectTypeAuthTeam:
		default:
			return nil, fmt.Errorf("allowed_callers type must be user, service_account, or auth_team")
		}
		callerID := strings.Trim(strings.TrimSpace(caller.ID), "/")
		if callerID == "" {
			return nil, fmt.Errorf("allowed_callers id is required")
		}
		key := callerType + "\x00" + callerID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, externalTriggerAllowedCaller{Type: callerType, ID: callerID})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type == out[j].Type {
			return out[i].ID < out[j].ID
		}
		return out[i].Type < out[j].Type
	})
	return out, nil
}

func normalizeExternalTriggerVariableMapping(mapping map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for key, value := range mapping {
		name := strings.TrimSpace(key)
		if name == "" {
			continue
		}
		if !envKeyPattern.MatchString(name) {
			return nil, fmt.Errorf("variable_mapping key %q is invalid", name)
		}
		source := strings.TrimSpace(value)
		if source == "" {
			return nil, fmt.Errorf("variable_mapping source for %q is required", name)
		}
		out[name] = source
	}
	return out, nil
}

func normalizeObjectMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func (a *App) validateExternalTriggerPipeline(ctx context.Context, pipeline string) error {
	path, name, _, err := configsync.SplitPipelineIdentifier(pipeline)
	if err != nil {
		return err
	}
	var exists int
	err = a.db.QueryRow(ctx, `SELECT 1 FROM pipelines WHERE path = $1 AND name = $2 LIMIT 1`, path, name).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("pipeline not found")
		}
		return fmt.Errorf("failed to validate pipeline")
	}
	return nil
}

func externalTriggerCallerFromRequest(a *App, r *http.Request) (string, string, model.Subject, error) {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		return "", "", model.Subject{}, fmt.Errorf("missing authorization subject")
	}
	callerType := strings.TrimSpace(subject.Type)
	callerID := strings.Trim(strings.TrimSpace(firstNonEmptyString(subject.ID, subject.Sub, subject.Email)), "/")
	if callerType == "" || callerID == "" {
		return "", "", subject, fmt.Errorf("caller identity is incomplete")
	}
	return callerType, callerID, subject, nil
}

func (a *App) externalTriggerCallerAllowed(ctx context.Context, allowed []externalTriggerAllowedCaller, subject model.Subject, callerType, callerID string) bool {
	if len(allowed) == 0 {
		return false
	}
	teamIDs := map[string]struct{}{}
	for _, caller := range allowed {
		if strings.TrimSpace(caller.Type) != model.SubjectTypeAuthTeam {
			continue
		}
		if len(teamIDs) == 0 {
			resp, err := a.aaaIntrospect(ctx, subject)
			if err == nil && resp != nil {
				for _, team := range resp.AuthTeams {
					teamIDs[strings.TrimSpace(team.ID)] = struct{}{}
					teamIDs[strings.TrimSpace(team.Name)] = struct{}{}
				}
			}
		}
	}
	for _, caller := range allowed {
		allowedType := strings.TrimSpace(caller.Type)
		allowedID := strings.Trim(strings.TrimSpace(caller.ID), "/")
		if allowedType == callerType && (allowedID == "*" || allowedID == callerID) {
			return true
		}
		if allowedType == model.SubjectTypeAuthTeam {
			if _, ok := teamIDs[allowedID]; ok {
				return true
			}
		}
	}
	return false
}

func (a *App) externalTriggerRateLimitExceeded(ctx context.Context, triggerID string, rateLimit map[string]any) (bool, string, error) {
	perMinute := intFromMap(rateLimit, "per_minute", "requests_per_minute", "invocations_per_minute")
	if perMinute <= 0 {
		return false, "", nil
	}
	var count int
	if err := a.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM external_trigger_invocations
		WHERE trigger_id = $1
		  AND created_at > NOW() - INTERVAL '1 minute'
	`, triggerID).Scan(&count); err != nil {
		return false, "", err
	}
	if externalTriggerRateLimitCountExceeded(count, perMinute) {
		return true, fmt.Sprintf("external trigger rate limit exceeded: %d per minute", perMinute), nil
	}
	return false, "", nil
}

func externalTriggerRateLimitCountExceeded(count, perMinute int) bool {
	return perMinute > 0 && count > perMinute
}

func intFromMap(values map[string]any, keys ...string) int {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch v := value.(type) {
		case float64:
			return int(v)
		case int:
			return v
		case int64:
			return int(v)
		case string:
			parsed, _ := strconv.Atoi(strings.TrimSpace(v))
			return parsed
		}
	}
	return 0
}

func validateExternalTriggerPayloadSchema(schema map[string]any, payload map[string]any) error {
	if len(schema) == 0 {
		return nil
	}
	schemaType, _ := schema["type"].(string)
	if schemaType != "" && schemaType != "object" {
		return fmt.Errorf("payload_schema only supports object schemas")
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if required, ok := schema["required"].([]any); ok {
		for _, item := range required {
			name := strings.TrimSpace(fmt.Sprint(item))
			if name == "" {
				continue
			}
			if _, exists := payload[name]; !exists {
				return fmt.Errorf("payload is missing required field %q", name)
			}
		}
	}
	properties, _ := schema["properties"].(map[string]any)
	for name, rawSpec := range properties {
		value, exists := payload[name]
		if !exists || value == nil {
			continue
		}
		spec, _ := rawSpec.(map[string]any)
		expected, _ := spec["type"].(string)
		if expected == "" {
			continue
		}
		if !jsonSchemaTypeMatches(expected, value) {
			return fmt.Errorf("payload field %q must be %s", name, expected)
		}
	}
	return nil
}

func jsonSchemaTypeMatches(expected string, value any) bool {
	switch strings.TrimSpace(expected) {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		number, ok := value.(float64)
		return ok && number == float64(int64(number))
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	default:
		return true
	}
}

func applyExternalTriggerVariableMapping(payload externalTriggerInvokeRequest, mapping map[string]string) (map[string]string, error) {
	variables := map[string]string{}
	for key, value := range payload.Variables {
		name := strings.TrimSpace(key)
		if name == "" {
			continue
		}
		if !envKeyPattern.MatchString(name) {
			return nil, fmt.Errorf("variable name %q is invalid", name)
		}
		variables[name] = value
	}
	for key, source := range mapping {
		value, ok := externalTriggerMappingValue(payload, source)
		if !ok {
			continue
		}
		variables[key] = value
	}
	return variables, nil
}

func externalTriggerMappingValue(payload externalTriggerInvokeRequest, source string) (string, bool) {
	source = strings.TrimSpace(source)
	switch {
	case source == "event_type":
		return strings.TrimSpace(payload.EventType), strings.TrimSpace(payload.EventType) != ""
	case strings.HasPrefix(source, "payload."):
		return stringifyExternalTriggerValue(resolveExternalTriggerPath(payload.Payload, strings.TrimPrefix(source, "payload.")))
	case strings.HasPrefix(source, "variables."):
		key := strings.TrimSpace(strings.TrimPrefix(source, "variables."))
		value, ok := payload.Variables[key]
		return value, ok
	default:
		if strings.HasPrefix(source, "literal:") {
			return strings.TrimPrefix(source, "literal:"), true
		}
		return stringifyExternalTriggerValue(resolveExternalTriggerPath(payload.Payload, source))
	}
}

func resolveExternalTriggerPath(root map[string]any, path string) (any, bool) {
	current := any(root)
	for _, part := range strings.Split(path, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func stringifyExternalTriggerValue(value any, ok bool) (string, bool) {
	if !ok || value == nil {
		return "", false
	}
	switch v := value.(type) {
	case string:
		return v, true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(v), true
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return "", false
		}
		return string(encoded), true
	}
}

func (a *App) findExternalTriggerInvocationByIdempotency(ctx context.Context, triggerID, callerType, callerID, key string) (externalTriggerInvocationRecord, bool, error) {
	var record externalTriggerInvocationRecord
	err := a.db.QueryRow(ctx, `
		SELECT id::text, trigger_id, caller_type, caller_id, status, COALESCE(run_id::text, ''),
		       idempotency_key, event_type, source_ip, created_at, error
		FROM external_trigger_invocations
		WHERE trigger_id = $1
		  AND caller_type = $2
		  AND caller_id = $3
		  AND idempotency_key = $4
		  AND status IN ($5, $6)
		LIMIT 1
	`, triggerID, callerType, callerID, key, externalTriggerStatusPending, externalTriggerStatusQueued).Scan(&record.ID, &record.TriggerID, &record.CallerType, &record.CallerID,
		&record.Status, &record.RunID, &record.IdempotencyKey, &record.EventType, &record.SourceIP, &record.CreatedAt, &record.Error)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return record, false, nil
		}
		return record, false, err
	}
	return record, true, nil
}

func (a *App) insertExternalTriggerInvocation(ctx context.Context, id uuid.UUID, triggerID, callerType, callerID, status, runID, idempotencyKey, eventType, sourceIP, errText string) error {
	runIDValue := nullableUUIDParam(runID)
	_, err := a.db.Exec(ctx, `
		INSERT INTO external_trigger_invocations
			(id, trigger_id, caller_type, caller_id, status, run_id, idempotency_key, event_type, source_ip, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, id, triggerID, callerType, callerID, status, runIDValue, idempotencyKey, eventType, sourceIP, errText)
	return err
}

func (a *App) updateExternalTriggerInvocation(ctx context.Context, id uuid.UUID, status, runID, errText string) error {
	runIDValue := nullableUUIDParam(runID)
	_, err := a.db.Exec(ctx, `
		UPDATE external_trigger_invocations
		SET status = $2,
		    run_id = COALESCE($3::uuid, run_id),
		    error = $4
		WHERE id = $1
	`, id, status, runIDValue, errText)
	return err
}

func nullableUUIDParam(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil
	}
	return parsed
}

func externalTriggerSourceIP(r *http.Request) string {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value == "" {
			continue
		}
		if header == "X-Forwarded-For" {
			value = strings.TrimSpace(strings.Split(value, ",")[0])
		}
		if value != "" {
			return value
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

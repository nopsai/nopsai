package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"

	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	aaamodel "nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/pkg/routeauthz"
)

func (a *App) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	pipelineFilter := strings.TrimSpace(r.URL.Query().Get("pipeline"))
	pathFilter, nameFilter, _, filterErr := configsync.SplitPipelineIdentifier(pipelineFilter)
	if pipelineFilter != "" && filterErr != nil {
		http.Error(w, filterErr.Error(), http.StatusBadRequest)
		return
	}

	records, err := a.listScheduleRecords(r.Context(), pathFilter, nameFilter)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list schedules")
		http.Error(w, "Failed to retrieve schedules", http.StatusInternalServerError)
		return
	}
	resources := make([]aaamodel.ResourceRef, 0, len(records))
	for _, record := range records {
		resources = append(resources, record.resourceRef())
	}
	allowedSet, err := a.allowedResourceSet(r, "pipeline_schedule.list", resources)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	responses := make([]scheduleResponse, 0, len(records))
	for _, record := range records {
		if _, ok := allowedSet[resourceKey(record.resourceRef())]; !ok {
			continue
		}
		responses = append(responses, scheduleResponseFromRecord(record))
	}
	writeJSON(w, http.StatusOK, responses)
}

func (a *App) handleGetSchedule(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireScheduleDecision(w, r, "pipeline_schedule.read")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, scheduleResponseFromRecord(record))
}

func (a *App) handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	var req scheduleRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid schedule payload", http.StatusBadRequest)
		return
	}
	input, err := normalizeScheduleInput(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	folderID := input.Path
	if folderID == "" {
		folderID = generalGrantID
	}
	if !a.requireAAADecision(w, r, "pipeline_schedule.create", aaamodel.ResourceRef{Type: grantResourceFolder, ID: folderID}) {
		return
	}
	pipeline, _, err := a.validateScheduleRuntimeAccess(r.Context(), r, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	record, err := a.createSchedule(r.Context(), input, pipeline, actorIDFromRequest(r), "database", "", "")
	if err != nil {
		if isUniqueViolation(err) {
			http.Error(w, "schedule already exists", http.StatusConflict)
			return
		}
		log.Error().Err(err).Msg("Failed to create schedule")
		http.Error(w, "Failed to create schedule", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, scheduleResponseFromRecord(record))
}

func (a *App) handleUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	existing, ok := a.requireScheduleDecision(w, r, "pipeline_schedule.update")
	if !ok {
		return
	}
	if existing.ManagedByConfigRepo {
		http.Error(w, "GitOps-managed schedules must be changed in the config repository", http.StatusConflict)
		return
	}
	var req scheduleRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid schedule payload", http.StatusBadRequest)
		return
	}
	input, err := normalizeScheduleInput(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	pipeline, _, err := a.validateScheduleRuntimeAccess(r.Context(), r, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	record, err := a.updateSchedule(r.Context(), existing.ID, input, pipeline, actorIDFromRequest(r))
	if err != nil {
		if isUniqueViolation(err) {
			http.Error(w, "schedule already exists", http.StatusConflict)
			return
		}
		log.Error().Err(err).Str("schedule_id", existing.ID).Msg("Failed to update schedule")
		http.Error(w, "Failed to update schedule", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, scheduleResponseFromRecord(record))
}

func (a *App) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireScheduleDecision(w, r, "pipeline_schedule.delete")
	if !ok {
		return
	}
	if record.ManagedByConfigRepo {
		http.Error(w, "GitOps-managed schedules must be changed in the config repository", http.StatusConflict)
		return
	}
	if err := a.deleteSchedule(r.Context(), record.ID); err != nil {
		log.Error().Err(err).Str("schedule_id", record.ID).Msg("Failed to delete schedule")
		http.Error(w, "Failed to delete schedule", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleEnableSchedule(w http.ResponseWriter, r *http.Request) {
	a.handleSetScheduleEnabled(w, r, true)
}

func (a *App) handleDisableSchedule(w http.ResponseWriter, r *http.Request) {
	a.handleSetScheduleEnabled(w, r, false)
}

func (a *App) handleSetScheduleEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	record, ok := a.requireScheduleDecision(w, r, "pipeline_schedule.update")
	if !ok {
		return
	}
	if record.ManagedByConfigRepo {
		http.Error(w, "GitOps-managed schedules must be changed in the config repository", http.StatusConflict)
		return
	}
	nextRunAt := record.NextRunAt
	if enabled {
		switch normalizeScheduleKindValue(record.ScheduleKind) {
		case scheduleKindOnce:
			if record.RunAt == nil {
				http.Error(w, "run_at is required for one-time schedules", http.StatusBadRequest)
				return
			}
			if !record.RunAt.After(time.Now()) {
				http.Error(w, "run_at must be in the future before enabling this one-time schedule", http.StatusBadRequest)
				return
			}
			nextRunAt = record.RunAt
		default:
			next, err := nextScheduleRunAt(record.CronExpression, record.Timezone, time.Now())
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			nextRunAt = &next
		}
	}
	updated, err := a.setScheduleEnabled(r.Context(), record.ID, enabled, nextRunAt, actorIDFromRequest(r))
	if err != nil {
		log.Error().Err(err).Str("schedule_id", record.ID).Msg("Failed to update schedule enabled state")
		http.Error(w, "Failed to update schedule", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, scheduleResponseFromRecord(updated))
}

func (a *App) handleRunScheduleNow(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireScheduleDecision(w, r, "pipeline_schedule.execute")
	if !ok {
		return
	}
	runID, err := a.executeSchedule(r.Context(), record)
	if err != nil {
		log.Error().Err(err).Str("schedule_id", record.ID).Msg("Failed to execute schedule")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"run_id": runID})
}

func (a *App) requireScheduleDecision(w http.ResponseWriter, r *http.Request, action string) (scheduleRecord, bool) {
	record, err := a.getScheduleRecord(r.Context(), r.PathValue("scheduleID"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "schedule not found", http.StatusNotFound)
			return scheduleRecord{}, false
		}
		log.Error().Err(err).Msg("Failed to load schedule")
		http.Error(w, "Failed to load schedule", http.StatusInternalServerError)
		return scheduleRecord{}, false
	}
	if !a.requireAAADecision(w, r, action, record.resourceRef()) {
		return scheduleRecord{}, false
	}
	return record, true
}

func (a *App) validateScheduleRuntimeAccess(ctx context.Context, r *http.Request, input scheduleInput) (models.Pipeline, []byte, error) {
	if !a.requireAAADecision(noopResponseWriter{}, r, "pipeline.execute", routeauthz.PipelineResource(input.PipelinePath, input.PipelineName)) {
		return models.Pipeline{}, nil, fmt.Errorf("forbidden")
	}
	pipeline, definition, err := a.loadSchedulePipeline(ctx, input.PipelinePath, input.PipelineName)
	if err != nil {
		return models.Pipeline{}, nil, err
	}
	subject, ok := a.currentAAASubject(r)
	if !ok {
		return models.Pipeline{}, nil, fmt.Errorf("unauthorized")
	}
	callerID := firstNonEmptyString(subject.ID, subject.Sub, subject.Email)
	if _, err := a.authorizeRunResourceUses(ctx, subject.Type, callerID, scheduleTriggerSource, map[string]string{}, input.PipelinePath, scheduleTriggerSource, pipeline, input.Scope); err != nil {
		return models.Pipeline{}, nil, err
	}
	return pipeline, definition, nil
}

type noopResponseWriter struct{}

func (noopResponseWriter) Header() http.Header       { return http.Header{} }
func (noopResponseWriter) Write([]byte) (int, error) { return 0, nil }
func (noopResponseWriter) WriteHeader(int)           {}

package nopsai

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"

	"nopsai/pkg/httpapi"
	aaamodel "nopsai/services/aaa/pkg/model"
)

func (a *App) handleListDashboards(w http.ResponseWriter, r *http.Request) {
	records, err := a.listDashboardRecords(r.Context(), r.URL.Query().Get("team"), r.URL.Query().Get("q"))
	if err != nil {
		log.Error().Err(err).Msg("Failed to list dashboards")
		http.Error(w, "Failed to retrieve dashboards", http.StatusInternalServerError)
		return
	}
	resources := make([]aaamodel.ResourceRef, 0, len(records))
	for _, record := range records {
		resources = append(resources, record.resourceRef())
	}
	allowedSet, err := a.allowedResourceSet(r, "dashboard.list", resources)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	responses := make([]dashboardResponse, 0, len(records))
	for _, record := range records {
		if _, ok := allowedSet[resourceKey(record.resourceRef())]; !ok {
			continue
		}
		responses = append(responses, dashboardResponseFromRecord(record))
	}
	writeJSON(w, http.StatusOK, responses)
}

func (a *App) handleCreateDashboard(w http.ResponseWriter, r *http.Request) {
	var req dashboardRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid dashboard payload", http.StatusBadRequest)
		return
	}
	teamID, teamPath, err := a.resolveDashboardTeam(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !a.requireAAADecision(w, r, "dashboard.create", aaamodel.ResourceRef{Type: grantResourceTeam, ID: teamPath}) {
		return
	}
	input, err := normalizeDashboardInput(req, teamID, teamPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	record, err := a.createDashboard(r.Context(), input, actorIDFromRequest(r))
	if err != nil {
		if isUniqueViolation(err) {
			http.Error(w, "dashboard already exists", http.StatusConflict)
			return
		}
		log.Error().Err(err).Msg("Failed to create dashboard")
		http.Error(w, "Failed to create dashboard", http.StatusInternalServerError)
		return
	}
	a.auditDashboardAction(r.Context(), r, "dashboard.created", record, "success", nil)
	writeJSON(w, http.StatusCreated, dashboardResponseFromRecord(record))
}

func (a *App) handleGetDashboard(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.read")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, dashboardResponseFromRecord(record))
}

func (a *App) handleUpdateDashboard(w http.ResponseWriter, r *http.Request) {
	existing, ok := a.requireDashboardDecision(w, r, "dashboard.update")
	if !ok {
		return
	}
	var req dashboardRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid dashboard payload", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.TeamPath) == "" && req.TeamID == 0 {
		req.TeamPath = existing.TeamPath
		req.TeamID = existing.TeamID
	}
	if strings.TrimSpace(req.Slug) == "" {
		req.Slug = existing.Slug
	}
	if strings.TrimSpace(req.Title) == "" {
		req.Title = existing.Title
	}
	if strings.TrimSpace(req.Visibility) == "" {
		req.Visibility = existing.Visibility
	}
	if req.RefreshPolicy == nil {
		req.RefreshPolicy = existing.RefreshPolicy
	}
	teamID, teamPath, err := a.resolveDashboardTeam(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	input, err := normalizeDashboardInput(req, teamID, teamPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	record, err := a.updateDashboard(r.Context(), existing.ID, input, actorIDFromRequest(r))
	if err != nil {
		if isUniqueViolation(err) {
			http.Error(w, "dashboard already exists", http.StatusConflict)
			return
		}
		log.Error().Err(err).Str("dashboard_id", existing.ID).Msg("Failed to update dashboard")
		http.Error(w, "Failed to update dashboard", http.StatusInternalServerError)
		return
	}
	a.auditDashboardAction(r.Context(), r, "dashboard.updated", record, "success", nil)
	writeJSON(w, http.StatusOK, dashboardResponseFromRecord(record))
}

func (a *App) handleDeleteDashboard(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.delete")
	if !ok {
		return
	}
	if err := a.deleteDashboard(r.Context(), record.ID); err != nil {
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to delete dashboard")
		http.Error(w, "Failed to delete dashboard", http.StatusInternalServerError)
		return
	}
	a.auditDashboardAction(r.Context(), r, "dashboard.deleted", record, "success", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleGetDashboardView(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.read")
	if !ok {
		return
	}
	sections, err := a.listDashboardSections(r.Context(), record.ID)
	if err != nil {
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to list dashboard sections")
		http.Error(w, "Failed to retrieve dashboard", http.StatusInternalServerError)
		return
	}
	publications, err := a.listDashboardPublications(r.Context(), record.ID, true)
	if err != nil {
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to list dashboard publications")
		http.Error(w, "Failed to retrieve dashboard", http.StatusInternalServerError)
		return
	}
	sources, err := a.listDashboardSources(r.Context(), record.ID)
	if err != nil {
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to list dashboard sources")
		http.Error(w, "Failed to retrieve dashboard", http.StatusInternalServerError)
		return
	}
	response := dashboardViewResponse{
		Dashboard:    dashboardResponseFromRecord(record),
		Sections:     make([]dashboardSectionResponse, 0, len(sections)),
		Publications: make([]dashboardPublicationResponse, 0, len(publications)),
		Sources:      make([]dashboardSourceResponse, 0, len(sources)),
	}
	for _, section := range sections {
		response.Sections = append(response.Sections, dashboardSectionResponseFromRecord(section))
	}
	for _, publication := range publications {
		response.Publications = append(response.Publications, dashboardPublicationResponseFromRecord(publication))
	}
	for _, source := range sources {
		response.Sources = append(response.Sources, dashboardSourceResponseFromRecord(source))
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) handleGetDashboardHistory(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.read")
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events, err := a.listDashboardEvents(r.Context(), record.ID, limit)
	if err != nil {
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to list dashboard history")
		http.Error(w, "Failed to retrieve dashboard history", http.StatusInternalServerError)
		return
	}
	responses := make([]dashboardEventResponse, 0, len(events))
	for _, event := range events {
		responses = append(responses, dashboardEventResponseFromRecord(event))
	}
	writeJSON(w, http.StatusOK, responses)
}

func (a *App) handleListDashboardSections(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.read")
	if !ok {
		return
	}
	sections, err := a.listDashboardSections(r.Context(), record.ID)
	if err != nil {
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to list dashboard sections")
		http.Error(w, "Failed to retrieve dashboard sections", http.StatusInternalServerError)
		return
	}
	responses := make([]dashboardSectionResponse, 0, len(sections))
	for _, section := range sections {
		responses = append(responses, dashboardSectionResponseFromRecord(section))
	}
	writeJSON(w, http.StatusOK, responses)
}

func (a *App) handleCreateDashboardSection(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.update")
	if !ok {
		return
	}
	var req dashboardSectionRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid dashboard section payload", http.StatusBadRequest)
		return
	}
	input, err := normalizeDashboardSectionInput(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	section, err := a.createDashboardSection(r.Context(), record.ID, input)
	if err != nil {
		if isUniqueViolation(err) {
			http.Error(w, "dashboard section already exists", http.StatusConflict)
			return
		}
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to create dashboard section")
		http.Error(w, "Failed to create dashboard section", http.StatusInternalServerError)
		return
	}
	a.auditDashboardAction(r.Context(), r, "dashboard.section_created", record, "success", map[string]any{"section_id": section.ID, "section_key": section.SectionKey})
	writeJSON(w, http.StatusCreated, dashboardSectionResponseFromRecord(section))
}

func (a *App) handleUpdateDashboardSection(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.update")
	if !ok {
		return
	}
	existing, err := a.getDashboardSection(r.Context(), record.ID, strings.TrimSpace(r.PathValue("sectionID")))
	if err != nil {
		if dashboardNotFound(err) {
			http.Error(w, "dashboard section not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to load dashboard section")
		http.Error(w, "Failed to load dashboard section", http.StatusInternalServerError)
		return
	}
	var req dashboardSectionRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid dashboard section payload", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.SectionKey) == "" {
		req.SectionKey = existing.SectionKey
	}
	if strings.TrimSpace(req.SectionKey) != existing.SectionKey {
		http.Error(w, "section_key cannot be changed after creation", http.StatusBadRequest)
		return
	}
	input, err := normalizeDashboardSectionInput(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	section, err := a.updateDashboardSection(r.Context(), record.ID, existing.ID, input)
	if err != nil {
		if dashboardNotFound(err) {
			http.Error(w, "dashboard section not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("dashboard_id", record.ID).Str("section_id", existing.ID).Msg("Failed to update dashboard section")
		http.Error(w, "Failed to update dashboard section", http.StatusInternalServerError)
		return
	}
	a.auditDashboardAction(r.Context(), r, "dashboard.section_updated", record, "success", map[string]any{"section_id": section.ID, "section_key": section.SectionKey})
	writeJSON(w, http.StatusOK, dashboardSectionResponseFromRecord(section))
}

func (a *App) handleDeleteDashboardSection(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.update")
	if !ok {
		return
	}
	sectionID := strings.TrimSpace(r.PathValue("sectionID"))
	section, err := a.getDashboardSection(r.Context(), record.ID, sectionID)
	if err != nil {
		if dashboardNotFound(err) {
			http.Error(w, "dashboard section not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to load dashboard section")
		http.Error(w, "Failed to delete dashboard section", http.StatusInternalServerError)
		return
	}
	if err := a.deleteDashboardSection(r.Context(), record.ID, sectionID); err != nil {
		if dashboardNotFound(err) {
			http.Error(w, "dashboard section not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("dashboard_id", record.ID).Str("section_id", sectionID).Msg("Failed to delete dashboard section")
		http.Error(w, "Failed to delete dashboard section", http.StatusInternalServerError)
		return
	}
	a.auditDashboardAction(r.Context(), r, "dashboard.section_deleted", record, "success", map[string]any{"section_id": section.ID, "section_key": section.SectionKey})
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleStartDashboardRefresh(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.refresh")
	if !ok {
		return
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Nopsai-Trigger-Source")), dashboardRefreshTriggerSource) {
		http.Error(w, "recursive dashboard refresh is not allowed", http.StatusConflict)
		return
	}
	var req dashboardRefreshRequest
	if err := httpapi.DecodeOptionalJSON(r, &req); err != nil {
		http.Error(w, "Invalid dashboard refresh payload", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		req.IdempotencyKey = firstNonEmptyString(r.Header.Get("Idempotency-Key"), r.Header.Get("X-Idempotency-Key"))
	}
	input, err := normalizeDashboardRefreshRequest(req, record.RefreshPolicy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	response, err := a.startDashboardRefresh(r.Context(), record, input, subject)
	if err != nil {
		writeDashboardRefreshError(w, err)
		return
	}
	a.auditDashboardAction(r.Context(), r, "dashboard.refreshed", record, "success", map[string]any{
		"refresh_id":   response.ID,
		"trigger_type": response.TriggerType,
		"scope_type":   response.ScopeType,
	})
	writeJSON(w, http.StatusAccepted, response)
}

func (a *App) handleListDashboardRefreshes(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.read")
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	responses, err := a.listDashboardRefreshResponses(r.Context(), record, limit)
	if err != nil {
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to list dashboard refreshes")
		http.Error(w, "Failed to retrieve dashboard refreshes", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, responses)
}

func (a *App) handleGetDashboardRefresh(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.read")
	if !ok {
		return
	}
	refreshID := strings.TrimSpace(r.PathValue("refreshID"))
	response, err := a.reconcileDashboardRefresh(r.Context(), record, refreshID)
	if err != nil {
		if dashboardNotFound(err) {
			http.Error(w, "dashboard refresh not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("dashboard_id", record.ID).Str("refresh_id", refreshID).Msg("Failed to retrieve dashboard refresh")
		http.Error(w, "Failed to retrieve dashboard refresh", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) handleCancelDashboardRefresh(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.refresh")
	if !ok {
		return
	}
	refreshID := strings.TrimSpace(r.PathValue("refreshID"))
	response, err := a.cancelDashboardRefresh(r.Context(), record, refreshID)
	if err != nil {
		writeDashboardRefreshError(w, err)
		return
	}
	a.auditDashboardAction(r.Context(), r, "dashboard.refresh_cancelled", record, "success", map[string]any{"refresh_id": response.ID})
	writeJSON(w, http.StatusOK, response)
}

func (a *App) handleRetryDashboardRefreshFailed(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.refresh")
	if !ok {
		return
	}
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	refreshID := strings.TrimSpace(r.PathValue("refreshID"))
	response, err := a.retryFailedDashboardRefreshSources(r.Context(), record, refreshID, subject)
	if err != nil {
		writeDashboardRefreshError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, response)
}

func (a *App) handleListDashboardRefreshSchedules(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.read")
	if !ok {
		return
	}
	schedules, err := a.listDashboardRefreshScheduleRecords(r.Context(), record)
	if err != nil {
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to list dashboard refresh schedules")
		http.Error(w, "Failed to retrieve dashboard refresh schedules", http.StatusInternalServerError)
		return
	}
	responses := make([]dashboardRefreshScheduleResponse, 0, len(schedules))
	for _, schedule := range schedules {
		responses = append(responses, dashboardRefreshScheduleResponseFromRecord(schedule))
	}
	writeJSON(w, http.StatusOK, responses)
}

func (a *App) handleCreateDashboardRefreshSchedule(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.update")
	if !ok {
		return
	}
	var req dashboardRefreshScheduleRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid dashboard refresh schedule payload", http.StatusBadRequest)
		return
	}
	input, err := normalizeDashboardRefreshScheduleInput(req, record.RefreshPolicy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	schedule, err := a.createDashboardRefreshSchedule(r.Context(), record, input, actorIDFromRequest(r), "database", "", "", nil)
	if err != nil {
		if isUniqueViolation(err) {
			http.Error(w, "dashboard refresh schedule already exists", http.StatusConflict)
			return
		}
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to create dashboard refresh schedule")
		http.Error(w, "Failed to create dashboard refresh schedule", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, dashboardRefreshScheduleResponseFromRecord(schedule))
}

func (a *App) handleUpdateDashboardRefreshSchedule(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.update")
	if !ok {
		return
	}
	var req dashboardRefreshScheduleRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid dashboard refresh schedule payload", http.StatusBadRequest)
		return
	}
	input, err := normalizeDashboardRefreshScheduleInput(req, record.RefreshPolicy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	schedule, err := a.updateDashboardRefreshSchedule(r.Context(), record, strings.TrimSpace(r.PathValue("scheduleID")), input, actorIDFromRequest(r))
	if err != nil {
		if dashboardNotFound(err) {
			http.Error(w, "dashboard refresh schedule not found", http.StatusNotFound)
			return
		}
		if isUniqueViolation(err) {
			http.Error(w, "dashboard refresh schedule already exists", http.StatusConflict)
			return
		}
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to update dashboard refresh schedule")
		http.Error(w, "Failed to update dashboard refresh schedule", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, dashboardRefreshScheduleResponseFromRecord(schedule))
}

func (a *App) handleDeleteDashboardRefreshSchedule(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.update")
	if !ok {
		return
	}
	if err := a.deleteDashboardRefreshSchedule(r.Context(), record, strings.TrimSpace(r.PathValue("scheduleID"))); err != nil {
		if dashboardNotFound(err) {
			http.Error(w, "dashboard refresh schedule not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to delete dashboard refresh schedule")
		http.Error(w, "Failed to delete dashboard refresh schedule", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleEnableDashboardRefreshSchedule(w http.ResponseWriter, r *http.Request) {
	a.handleSetDashboardRefreshScheduleEnabled(w, r, true)
}

func (a *App) handleDisableDashboardRefreshSchedule(w http.ResponseWriter, r *http.Request) {
	a.handleSetDashboardRefreshScheduleEnabled(w, r, false)
}

func (a *App) handleSetDashboardRefreshScheduleEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.refresh")
	if !ok {
		return
	}
	schedule, err := a.setDashboardRefreshScheduleEnabled(r.Context(), record, strings.TrimSpace(r.PathValue("scheduleID")), enabled, actorIDFromRequest(r))
	if err != nil {
		if dashboardNotFound(err) {
			http.Error(w, "dashboard refresh schedule not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to update dashboard refresh schedule")
		http.Error(w, "Failed to update dashboard refresh schedule", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, dashboardRefreshScheduleResponseFromRecord(schedule))
}

func (a *App) handleRunDashboardRefreshScheduleNow(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.refresh")
	if !ok {
		return
	}
	schedule, err := a.getDashboardRefreshScheduleRecord(r.Context(), record, strings.TrimSpace(r.PathValue("scheduleID")))
	if err != nil {
		if dashboardNotFound(err) {
			http.Error(w, "dashboard refresh schedule not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to load dashboard refresh schedule")
		http.Error(w, "Failed to load dashboard refresh schedule", http.StatusInternalServerError)
		return
	}
	response, err := a.executeDashboardRefreshSchedule(r.Context(), schedule)
	if err != nil {
		writeDashboardRefreshError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, response)
}

func (a *App) handleListDashboardSources(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.read")
	if !ok {
		return
	}
	sources, err := a.listDashboardSources(r.Context(), record.ID)
	if err != nil {
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to list dashboard sources")
		http.Error(w, "Failed to retrieve dashboard sources", http.StatusInternalServerError)
		return
	}
	responses := make([]dashboardSourceResponse, 0, len(sources))
	for _, source := range sources {
		responses = append(responses, dashboardSourceResponseFromRecord(source))
	}
	writeJSON(w, http.StatusOK, responses)
}

func (a *App) handleCreateDashboardSource(w http.ResponseWriter, r *http.Request) {
	a.handleSaveDashboardSource(w, r, "")
}

func (a *App) handleUpdateDashboardSource(w http.ResponseWriter, r *http.Request) {
	a.handleSaveDashboardSource(w, r, r.PathValue("sourceID"))
}

func (a *App) handleSaveDashboardSource(w http.ResponseWriter, r *http.Request, sourceID string) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.manage_sources")
	if !ok {
		return
	}
	var req dashboardSourceRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid dashboard source payload", http.StatusBadRequest)
		return
	}
	input, err := normalizeDashboardSourceInput(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	section := dashboardSectionInput{SectionKey: input.SectionKey, Title: titleFromKey(input.SectionKey), Layout: map[string]any{}}
	if err := upsertDashboardSection(r.Context(), a.db, record.ID, section); err != nil {
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to ensure dashboard section")
		http.Error(w, "Failed to save dashboard source", http.StatusInternalServerError)
		return
	}
	source, err := a.saveDashboardSource(r.Context(), record.ID, sourceID, input)
	if err != nil {
		if dashboardNotFound(err) {
			http.Error(w, "dashboard source not found", http.StatusNotFound)
			return
		}
		if isUniqueViolation(err) {
			http.Error(w, "dashboard source already exists", http.StatusConflict)
			return
		}
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to save dashboard source")
		http.Error(w, "Failed to save dashboard source", http.StatusInternalServerError)
		return
	}
	status := http.StatusOK
	if sourceID == "" {
		status = http.StatusCreated
	}
	if source.Enabled {
		a.auditDashboardAction(r.Context(), r, "dashboard.source_enabled", record, "success", map[string]any{"source_id": source.ID})
	} else {
		a.auditDashboardAction(r.Context(), r, "dashboard.source_disabled", record, "success", map[string]any{"source_id": source.ID})
	}
	writeJSON(w, status, dashboardSourceResponseFromRecord(source))
}

func (a *App) handleDeleteDashboardSource(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireDashboardDecision(w, r, "dashboard.manage_sources")
	if !ok {
		return
	}
	if err := a.deleteDashboardSource(r.Context(), record.ID, r.PathValue("sourceID")); err != nil {
		if dashboardNotFound(err) {
			http.Error(w, "dashboard source not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("dashboard_id", record.ID).Msg("Failed to delete dashboard source")
		http.Error(w, "Failed to delete dashboard source", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) requireDashboardDecision(w http.ResponseWriter, r *http.Request, action string) (dashboardRecord, bool) {
	record, err := a.getDashboardRecord(r.Context(), r.PathValue("dashboardID"))
	if err != nil {
		if dashboardNotFound(err) {
			http.Error(w, "dashboard not found", http.StatusNotFound)
			return dashboardRecord{}, false
		}
		log.Error().Err(err).Msg("Failed to load dashboard")
		http.Error(w, "Failed to load dashboard", http.StatusInternalServerError)
		return dashboardRecord{}, false
	}
	if !a.requireAAADecision(w, r, action, record.resourceRef()) {
		return dashboardRecord{}, false
	}
	return record, true
}

func writeDashboardRefreshError(w http.ResponseWriter, err error) {
	var refreshErr dashboardRefreshHTTPError
	if errors.As(err, &refreshErr) {
		http.Error(w, refreshErr.Message, refreshErr.StatusCode)
		return
	}
	if dashboardNotFound(err) {
		http.Error(w, "dashboard refresh not found", http.StatusNotFound)
		return
	}
	log.Error().Err(err).Msg("Dashboard refresh request failed")
	http.Error(w, "Failed to process dashboard refresh", http.StatusInternalServerError)
}

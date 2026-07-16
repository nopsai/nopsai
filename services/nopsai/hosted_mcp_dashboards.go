package nopsai

import (
	"context"
	"fmt"

	aaamodel "nopsai/services/aaa/pkg/model"
)

func (a *App) hostedMCPListDashboards(ctx context.Context, subject aaamodel.Subject, args map[string]any) (map[string]any, error) {
	records, err := a.listDashboardRecords(ctx, firstNonEmptyString(stringArg(args, "team_path"), stringArg(args, "team")), firstNonEmptyString(stringArg(args, "q"), stringArg(args, "query")))
	if err != nil {
		return nil, err
	}
	resources := make([]aaamodel.ResourceRef, 0, len(records))
	for _, record := range records {
		resources = append(resources, record.resourceRef())
	}
	allowed, err := a.aaaFilter(ctx, subject, "dashboard.list", resources, map[string]any{"tool": "nopsai.list_dashboards"})
	if err != nil {
		return nil, err
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, resource := range allowed {
		allowedSet[resourceKey(resource)] = struct{}{}
	}
	limit := limitArg(args, 50, 200)
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		if _, ok := allowedSet[resourceKey(record.resourceRef())]; !ok {
			continue
		}
		response := dashboardResponseFromRecord(record)
		items = append(items, map[string]any{
			"id":                        response.ID,
			"ref":                       response.Ref,
			"team_path":                 response.TeamPath,
			"slug":                      response.Slug,
			"title":                     response.Title,
			"description":               response.Description,
			"visibility":                response.Visibility,
			"current_publication_count": response.CurrentPublicationCount,
			"last_published_at":         response.LastPublishedAt,
			"updated_at":                response.UpdatedAt,
		})
		if len(items) >= limit {
			break
		}
	}
	return map[string]any{"dashboards": items}, nil
}

func (a *App) hostedMCPGetDashboard(ctx context.Context, args map[string]any) (map[string]any, error) {
	dashboardID := firstNonEmptyString(stringArg(args, "dashboard_id"), stringArg(args, "id"), stringArg(args, "ref"))
	record, err := a.getDashboardRecord(ctx, dashboardID)
	if err != nil {
		return nil, err
	}
	sections, err := a.listDashboardSections(ctx, record.ID)
	if err != nil {
		return nil, err
	}
	publications, err := a.listDashboardPublications(ctx, record.ID, true)
	if err != nil {
		return nil, err
	}
	sources, err := a.listDashboardSources(ctx, record.ID)
	if err != nil {
		return nil, err
	}
	sectionResponses := make([]dashboardSectionResponse, 0, len(sections))
	for _, section := range sections {
		sectionResponses = append(sectionResponses, dashboardSectionResponseFromRecord(section))
	}
	publicationResponses := make([]dashboardPublicationResponse, 0, len(publications))
	for _, publication := range publications {
		publicationResponses = append(publicationResponses, dashboardPublicationResponseFromRecord(publication))
	}
	sourceResponses := make([]dashboardSourceResponse, 0, len(sources))
	for _, source := range sources {
		sourceResponses = append(sourceResponses, dashboardSourceResponseFromRecord(source))
	}
	result := map[string]any{
		"dashboard":    dashboardResponseFromRecord(record),
		"sections":     sectionResponses,
		"publications": publicationResponses,
		"sources":      sourceResponses,
	}
	if boolArg(args, "include_history", false) {
		events, err := a.listDashboardEvents(ctx, record.ID, 100)
		if err != nil {
			return nil, err
		}
		eventResponses := make([]dashboardEventResponse, 0, len(events))
		for _, event := range events {
			eventResponses = append(eventResponses, dashboardEventResponseFromRecord(event))
		}
		result["history"] = eventResponses
	}
	return result, nil
}

func (a *App) hostedMCPListDashboardRefreshes(ctx context.Context, args map[string]any) (map[string]any, error) {
	dashboardID := firstNonEmptyString(stringArg(args, "dashboard_id"), stringArg(args, "id"), stringArg(args, "ref"))
	record, err := a.getDashboardRecord(ctx, dashboardID)
	if err != nil {
		return nil, err
	}
	responses, err := a.listDashboardRefreshResponses(ctx, record, limitArg(args, 50, 200))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"dashboard": dashboardResponseFromRecord(record),
		"refreshes": responses,
	}, nil
}

func (a *App) hostedMCPListDashboardRefreshSchedules(ctx context.Context, args map[string]any) (map[string]any, error) {
	dashboardID := firstNonEmptyString(stringArg(args, "dashboard_id"), stringArg(args, "id"), stringArg(args, "ref"))
	record, err := a.getDashboardRecord(ctx, dashboardID)
	if err != nil {
		return nil, err
	}
	schedules, err := a.listDashboardRefreshScheduleRecords(ctx, record)
	if err != nil {
		return nil, err
	}
	responses := make([]dashboardRefreshScheduleResponse, 0, len(schedules))
	for _, schedule := range schedules {
		responses = append(responses, dashboardRefreshScheduleResponseFromRecord(schedule))
	}
	return map[string]any{
		"dashboard": dashboardResponseFromRecord(record),
		"schedules": responses,
	}, nil
}

func (a *App) hostedMCPRefreshDashboard(ctx context.Context, subject aaamodel.Subject, args map[string]any) (map[string]any, error) {
	if !boolArg(args, "confirm", false) {
		return nil, fmt.Errorf("confirm=true is required to start a dashboard refresh")
	}
	dashboardID := firstNonEmptyString(stringArg(args, "dashboard_id"), stringArg(args, "id"), stringArg(args, "ref"))
	record, err := a.getDashboardRecord(ctx, dashboardID)
	if err != nil {
		return nil, err
	}
	req := dashboardRefreshRequest{
		Scope: dashboardRefreshScopeRequest{
			Type:       firstNonEmptyString(stringArg(args, "scope_type"), stringArg(args, "type")),
			SectionKey: stringArg(args, "section_key"),
			SourceID:   stringArg(args, "source_id"),
		},
		TriggerType:    dashboardRefreshTriggerAssistant,
		Mode:           stringArg(args, "mode"),
		RunScope:       stringArg(args, "run_scope"),
		Variables:      hostedMCPStringMapArg(args, "variables"),
		MaxConcurrency: intArg(args, "max_concurrency", 0, dashboardRefreshMaxConcurrency),
		Timeout:        stringArg(args, "timeout"),
		IdempotencyKey: stringArg(args, "idempotency_key"),
	}
	input, err := normalizeDashboardRefreshRequest(req, record.RefreshPolicy)
	if err != nil {
		return nil, err
	}
	response, err := a.startDashboardRefresh(ctx, record, input, subject)
	if err != nil {
		return nil, err
	}
	return map[string]any{"refresh": response}, nil
}

func (a *App) hostedMCPRunDashboardRefreshSchedule(ctx context.Context, args map[string]any) (map[string]any, error) {
	if !boolArg(args, "confirm", false) {
		return nil, fmt.Errorf("confirm=true is required to run a dashboard refresh schedule")
	}
	dashboardID := firstNonEmptyString(stringArg(args, "dashboard_id"), stringArg(args, "id"), stringArg(args, "ref"))
	record, err := a.getDashboardRecord(ctx, dashboardID)
	if err != nil {
		return nil, err
	}
	scheduleID := firstNonEmptyString(stringArg(args, "schedule_id"), stringArg(args, "name"))
	schedule, err := a.getDashboardRefreshScheduleRecord(ctx, record, scheduleID)
	if err != nil {
		return nil, err
	}
	response, err := a.executeDashboardRefreshSchedule(ctx, schedule)
	if err != nil {
		return nil, err
	}
	return map[string]any{"refresh": response}, nil
}

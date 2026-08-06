package routeauthz

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"nopsai/internal/cli/apicatalog"
)

func TestMapRequestUsesFilterForTriggerAndScopeLists(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "trigger overrides", path: "/v1/overrides"},
		{name: "secret scopes", path: "/v1/secrets/scopes"},
		{name: "variable scopes", path: "/v1/variables/scopes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			action, _, requiresFilter, err := MapRequest(req)
			if err != nil {
				t.Fatalf("MapRequest() error = %v", err)
			}
			if action == "" {
				t.Fatal("MapRequest() action = empty, want authz action")
			}
			if !requiresFilter {
				t.Fatal("MapRequest() requiresFilter = false, want true")
			}
		})
	}
}

func TestMutatingCatalogRoutesRequireAuthzOrExplicitHandlerAuthorization(t *testing.T) {
	deferrals := map[string]string{
		"POST /v1/access/grants":                                                   "Access grant writes load the target principal/resource and authorize in the access handler.",
		"DELETE /v1/access/grants/{grantID}":                                       "Access grant deletion resolves the grant before authorizing the target resource.",
		"POST /v1/analysis/evaluate":                                               "Analysis evaluation is an authenticated subject-scoped utility endpoint.",
		"POST /v1/assistant/conversations":                                         "Assistant conversations are scoped to the authenticated subject in the handler.",
		"DELETE /v1/assistant/conversations/{id}":                                  "Assistant conversations are scoped to the authenticated subject in the handler.",
		"POST /v1/assistant/conversations/{id}/messages":                           "Assistant conversations are scoped to the authenticated subject in the handler.",
		"POST /v1/assistant/conversations/{id}/summarize-memory":                   "Assistant conversations are scoped to the authenticated subject in the handler.",
		"POST /v1/auth/discover":                                                   "Authentication discovery is public by design.",
		"POST /v1/auth/email":                                                      "Authenticated users update their own email address in the handler.",
		"POST /v1/auth/login":                                                      "Login is public by design.",
		"POST /v1/auth/logout":                                                     "Logout is public by design.",
		"POST /v1/auth/password":                                                   "Authenticated users update their own password in the handler.",
		"POST /v1/auth/personal-tokens":                                            "Authenticated users manage their own personal tokens in the handler.",
		"DELETE /v1/auth/personal-tokens/{tokenID}":                                "Authenticated users manage their own personal tokens in the handler.",
		"POST /v1/auth/refresh":                                                    "Token refresh is public by design and validated by refresh token state.",
		"POST /v1/auth/session/exchange":                                           "Session exchange is public by design and validates the exchanged session.",
		"POST /v1/authz/resource-use/batch-check":                                  "Resource-use checks answer visibility questions for the current subject.",
		"POST /v1/authz/resource-use/check":                                        "Resource-use checks answer visibility questions for the current subject.",
		"POST /v1/dashboards":                                                      "Dashboard creation resolves the requested team and authorizes dashboard.create in the handler.",
		"POST /v1/external-triggers":                                               "External trigger creation derives the concrete trigger resource from the request body.",
		"POST /v1/external-triggers/{id}/invoke":                                   "External trigger invocation validates trigger secrets and idempotency in the handler.",
		"POST /v1/git-webhook-sources":                                             "Git webhook source creation derives the concrete source resource from the request body.",
		"POST /v1/git/events":                                                      "Git event ingestion is authenticated service flow handled outside AAA route mapping.",
		"POST /v1/git/webhooks/{sourceID}":                                         "Git webhooks are public by design and validate source-specific secrets.",
		"POST /v1/internal/runs/{runID}/ai-usage":                                  "Run AI usage ingestion is an internal service-auth endpoint.",
		"POST /v1/internal/runs/{runID}/approvals/pause":                           "Run approval pause is an internal service-auth endpoint.",
		"POST /v1/internal/runs/{runID}/task-outputs/resolve":                      "Child runtime output resolution is an internal service-auth endpoint.",
		"POST /v1/internal/runs/{runID}/steps/{stepName}/tasks/{taskName}/outputs": "Task output ingestion is an internal service-auth endpoint.",
		"POST /v1/knowledge-connections":                                           "Knowledge connection creation derives the concrete resource from the request body.",
		"DELETE /v1/knowledge-connections/{connectionID...}":                       "Knowledge connection writes authorize the concrete resource in the handler.",
		"PATCH /v1/knowledge-connections/{connectionID...}":                        "Knowledge connection writes authorize the concrete resource in the handler.",
		"POST /v1/knowledge-connections/{connectionID...}":                         "Knowledge connection writes authorize the concrete resource in the handler.",
		"PUT /v1/knowledge-connections/{connectionID...}":                          "Knowledge connection writes authorize the concrete resource in the handler.",
		"POST /v1/knowledge-connections/{connectionID}/resolve-page":               "Connection page resolution checks connection use permission in the handler.",
		"POST /v1/knowledge-connections/{connectionID}/test":                       "Connection tests check connection use permission in the handler.",
		"POST /v1/knowledge-context-connections":                                   "Knowledge connection creation derives the concrete resource from the request body.",
		"DELETE /v1/knowledge-context-connections/{connectionID...}":               "Knowledge connection writes authorize the concrete resource in the handler.",
		"PATCH /v1/knowledge-context-connections/{connectionID...}":                "Knowledge connection writes authorize the concrete resource in the handler.",
		"POST /v1/knowledge-context-connections/{connectionID...}":                 "Knowledge connection writes authorize the concrete resource in the handler.",
		"PUT /v1/knowledge-context-connections/{connectionID...}":                  "Knowledge connection writes authorize the concrete resource in the handler.",
		"POST /v1/knowledge-context-connections/{connectionID}/resolve-page":       "Connection page resolution checks connection use permission in the handler.",
		"POST /v1/knowledge-context-connections/{connectionID}/test":               "Connection tests check connection use permission in the handler.",
		"POST /v1/knowledge-contexts/{knowledgeID...}":                             "Knowledge context creation derives the concrete resource from the request body.",
		"PUT /v1/knowledge-contexts/{knowledgeID...}":                              "Knowledge context writes authorize the concrete resource in the handler.",
		"POST /v1/mcp":                                                       "Hosted MCP applies tool-level AAA before executing API-backed actions.",
		"POST /v1/monitoring/alert-rules":                                    "Monitoring operations are scoped to the authenticated subject in the handler.",
		"DELETE /v1/monitoring/alert-rules/{ruleID}":                         "Monitoring operations are scoped to the authenticated subject in the handler.",
		"PUT /v1/monitoring/alert-rules/{ruleID}":                            "Monitoring operations are scoped to the authenticated subject in the handler.",
		"POST /v1/monitoring/alert-rules/{ruleID}/evaluate":                  "Monitoring operations are scoped to the authenticated subject in the handler.",
		"POST /v1/monitoring/recommendations/{recommendationID}/acknowledge": "Monitoring recommendation workflow is authenticated and persisted by ID in the handler.",
		"POST /v1/monitoring/recommendations/{recommendationID}/resolve":     "Monitoring recommendation workflow is authenticated and persisted by ID in the handler.",
		"POST /v1/monitoring/views":                                          "Monitoring operations are scoped to the authenticated subject in the handler.",
		"DELETE /v1/monitoring/views/{viewID}":                               "Monitoring operations are scoped to the authenticated subject in the handler.",
		"PUT /v1/monitoring/views/{viewID}":                                  "Monitoring operations are scoped to the authenticated subject in the handler.",
		"PUT /v1/overrides/{repoOwner}/{repoName}":                           "Trigger override writes derive the concrete trigger and authorize in the handler.",
		"PUT /v1/pipelines/{pipelineName...}":                                "Pipeline upserts derive create/update authorization from the request body and existing state.",
		"POST /v1/run":                                                       "Ad-hoc run requests derive the pipeline resource from the submitted definition.",
		"POST /v1/runs/{runID}/approvals/{approvalID}/approve":               "Approval decisions authorize against the concrete approval and run in the handler.",
		"POST /v1/runs/{runID}/approvals/{approvalID}/reject":                "Approval decisions authorize against the concrete approval and run in the handler.",
		"POST /v1/schedules":                                                 "Schedule creation derives the concrete pipeline schedule from the request body.",
		"DELETE /v1/schedules/{scheduleID}":                                  "Schedule writes load the concrete schedule resource before authorizing.",
		"PATCH /v1/schedules/{scheduleID}":                                   "Schedule writes load the concrete schedule resource before authorizing.",
		"PUT /v1/schedules/{scheduleID}":                                     "Schedule writes load the concrete schedule resource before authorizing.",
		"POST /v1/schedules/{scheduleID}/disable":                            "Schedule writes load the concrete schedule resource before authorizing.",
		"POST /v1/schedules/{scheduleID}/enable":                             "Schedule writes load the concrete schedule resource before authorizing.",
		"POST /v1/schedules/{scheduleID}/run":                                "Schedule run-now authorizes the loaded schedule in the handler.",
		"PUT /v1/steps/{stepName...}":                                        "Step upserts derive create/update authorization from the submitted definition.",
		"POST /v1/teams":                                                     "Team creation derives parent/team authorization from the request body.",
		"DELETE /v1/teams/{teamID}":                                          "Team writes load and authorize the concrete team in the handler.",
		"PUT /v1/teams/{teamID}":                                             "Team writes load and authorize the concrete team in the handler.",
		"POST /v1/teams/{teamID}/applications":                               "Team application writes load and authorize the concrete team/application in the handler.",
		"DELETE /v1/teams/{teamID}/applications/{applicationID}":             "Team application writes load and authorize the concrete team/application in the handler.",
		"PUT /v1/teams/{teamID}/applications/{applicationID}":                "Team application writes load and authorize the concrete team/application in the handler.",
	}

	var missing []string
	for _, route := range apicatalog.Routes() {
		if !mutatingRouteMethod(route.Method) {
			continue
		}
		pathValues := routeAuthzPathValues(route)
		path, err := route.Expand(pathValues)
		if err != nil {
			t.Fatalf("expand %s %s: %v", route.Method, route.Path, err)
		}
		req := httptest.NewRequest(route.Method, path, nil)
		for name, value := range pathValues {
			req.SetPathValue(name, value)
		}
		action, _, _, err := MapRequest(req)
		if err != nil {
			t.Fatalf("MapRequest(%s %s) error = %v", route.Method, route.Path, err)
		}
		if action != "" {
			continue
		}
		key := route.Method + " " + route.Path
		if _, ok := deferrals[key]; !ok {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("mutating routes without AAA mapping or explicit handler-authz deferral:\n%s", strings.Join(missing, "\n"))
	}
}

func TestMapRequestRejectsUnknownMutatingRoutes(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/new-unmapped-route", nil)

	action, resource, requiresFilter, err := MapRequest(req)

	if err == nil {
		t.Fatal("MapRequest() error = nil, want unmapped mutating route error")
	}
	if action != "" || resource.Type != "" || resource.ID != "" || requiresFilter {
		t.Fatalf("MapRequest() = action %q resource %#v filter %v, want empty denied mapping", action, resource, requiresFilter)
	}
}

func TestMapRequestRejectsRemovedRegistryAuthBroker(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/internal/registry-auth/docker", nil)

	_, _, _, err := MapRequest(req)

	if err == nil {
		t.Fatal("MapRequest() error = nil, want removed registry-auth broker to be rejected")
	}
}

func TestMapRequestDefersResourceAccessRoutesToHandlers(t *testing.T) {
	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/v1/resources/pipeline/team-1/build/access"},
		{method: http.MethodPut, path: "/v1/resources/pipeline/team-1/build/access"},
		{method: http.MethodPost, path: "/v1/resources/pipeline/team-1/build/grants"},
		{method: http.MethodDelete, path: "/v1/resources/pipeline/team-1/build/grants/grant_123"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			action, resource, requiresFilter, err := MapRequest(req)
			if err != nil {
				t.Fatalf("MapRequest() error = %v", err)
			}
			if action != "" || resource.Type != "" || resource.ID != "" || requiresFilter {
				t.Fatalf("MapRequest() = action %q resource %#v filter %v, want handler deferral", action, resource, requiresFilter)
			}
		})
	}
}

func mutatingRouteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func routeAuthzPathValues(route apicatalog.Route) map[string]string {
	values := make(map[string]string, len(route.PathParameters))
	for _, parameter := range route.PathParameters {
		value := strings.TrimSpace(parameter.Example)
		if value == "" {
			value = strings.TrimSpace(parameter.Name) + "-value"
		}
		if !parameter.CatchAll && strings.Contains(value, "/") {
			value = strings.TrimSpace(parameter.Name) + "-value"
		}
		values[parameter.Name] = value
	}
	return values
}

func TestMapRequestDefersRunByCheckAuthorizationToConcreteRun(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/runs-by-check/check-123", nil)
	action, resource, requiresFilter, err := MapRequest(req)
	if err != nil {
		t.Fatalf("MapRequest() error = %v", err)
	}
	if action != "" {
		t.Fatalf("MapRequest() action = %q, want empty", action)
	}
	if requiresFilter {
		t.Fatal("MapRequest() requiresFilter = true, want false")
	}
	if resource.Type != "" || resource.ID != "" {
		t.Fatalf("MapRequest() resource = %#v, want empty", resource)
	}
}

func TestMapRequestDefersApprovalAwareRunReadsToHandler(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "run detail", path: "/v1/runs/run-123"},
		{name: "run approvals", path: "/v1/runs/run-123/approvals"},
		{name: "run final output download", path: "/v1/runs/run-123/outputs/output-1/download"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			action, resource, requiresFilter, err := MapRequest(req)
			if err != nil {
				t.Fatalf("MapRequest() error = %v", err)
			}
			if action != "" {
				t.Fatalf("MapRequest() action = %q, want empty", action)
			}
			if requiresFilter {
				t.Fatal("MapRequest() requiresFilter = true, want false")
			}
			if resource.Type != "pipeline_run" || resource.ID != "run-123" {
				t.Fatalf("MapRequest() resource = %#v, want pipeline_run:run-123", resource)
			}
		})
	}
}

func TestMapRequestMapsFinalOutputRetryToRunRerun(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/run-123/outputs/output-1/retry", nil)
	req.SetPathValue("runID", "run-123")

	action, resource, requiresFilter, err := MapRequest(req)

	if err != nil {
		t.Fatalf("MapRequest() error = %v", err)
	}
	if action != "pipeline_run.rerun" || resource.Type != "pipeline_run" || resource.ID != "run-123" || requiresFilter {
		t.Fatalf("MapRequest() = action %q resource %#v filter %v, want pipeline_run.rerun on run-123", action, resource, requiresFilter)
	}
}

func TestMapRequestTreatsPersonalTokenRoutesAsAuthenticatedOnly(t *testing.T) {
	for _, tt := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/v1/auth/personal-tokens"},
		{method: http.MethodPost, path: "/v1/auth/personal-tokens"},
		{method: http.MethodDelete, path: "/v1/auth/personal-tokens/00000000-0000-0000-0000-000000000001"},
	} {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		action, resource, requiresFilter, err := MapRequest(req)
		if err != nil {
			t.Fatalf("MapRequest() error = %v", err)
		}
		if action != "" || requiresFilter || resource.Type != "" || resource.ID != "" {
			t.Fatalf("MapRequest(%s %s) = action %q resource %#v filter %v, want authenticated-only", tt.method, tt.path, action, resource, requiresFilter)
		}
	}
}

func TestMapRequestUsesUpdatedLowLevelActions(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		pathValues map[string]string
		wantAction string
		wantType   string
		wantID     string
		wantFilter bool
	}{
		{
			name:       "team list uses filter",
			method:     http.MethodGet,
			path:       "/v1/teams",
			wantAction: "team.list",
			wantType:   "team",
			wantID:     "*",
			wantFilter: true,
		},
		{
			name:       "secret gitops encryption requires write permission",
			method:     http.MethodPost,
			path:       "/v1/secrets/encrypt",
			wantAction: "secret.write_value",
			wantType:   "secret",
			wantID:     "*",
		},
		{
			name:       "run logs use read logs action",
			method:     http.MethodGet,
			path:       "/v1/runs/run-123/logs",
			pathValues: map[string]string{"runID": "run-123"},
			wantAction: "pipeline_run.read_logs",
			wantType:   "pipeline_run",
			wantID:     "run-123",
		},
		{
			name:       "run status maps run id before mux path values",
			method:     http.MethodGet,
			path:       "/v1/runs/run-123/status",
			wantAction: "pipeline_run.read",
			wantType:   "pipeline_run",
			wantID:     "run-123",
		},
		{
			name:       "run log ingest maps run id before mux path values",
			method:     http.MethodPost,
			path:       "/v1/runs/run-123/logs/ingest",
			wantAction: "pipeline_run.write_logs",
			wantType:   "pipeline_run",
			wantID:     "run-123",
		},
		{
			name:       "run finalize maps run id before mux path values",
			method:     http.MethodPost,
			path:       "/v1/runs/run-123/finalize",
			wantAction: "pipeline_run.finalize",
			wantType:   "pipeline_run",
			wantID:     "run-123",
		},
		{
			name:       "run final output cancel uses run cancel action",
			method:     http.MethodPost,
			path:       "/v1/runs/run-123/outputs/output-1/cancel",
			wantAction: "pipeline_run.cancel",
			wantType:   "pipeline_run",
			wantID:     "run-123",
		},
		{
			name:       "run task update maps run id before mux path values",
			method:     http.MethodPost,
			path:       "/v1/runs/run-123/steps/build/tasks/test",
			wantAction: "pipeline_run.task_update",
			wantType:   "pipeline_run",
			wantID:     "run-123",
		},
		{
			name:       "pipeline detail maps nested id before mux path values",
			method:     http.MethodGet,
			path:       "/v1/pipelines/team/build",
			wantAction: "pipeline.read",
			wantType:   "pipeline",
			wantID:     "team/build",
		},
		{
			name:       "pipeline validation uses list permission",
			method:     http.MethodPost,
			path:       "/v1/pipelines/validate",
			wantAction: "pipeline.list",
			wantType:   "pipeline",
			wantID:     "*",
		},
		{
			name:       "trigger override maps nested id before mux path values",
			method:     http.MethodGet,
			path:       "/v1/overrides/team-1/dev/nopsai/test-app",
			wantAction: "trigger.read",
			wantType:   "trigger",
			wantID:     "team-1/dev/nopsai/test-app",
		},
		{
			name:       "trigger validation uses trigger read",
			method:     http.MethodPost,
			path:       "/v1/overrides/validate",
			wantAction: "trigger.read",
			wantType:   "trigger",
			wantID:     "*",
		},
		{
			name:       "repository branches use repository read",
			method:     http.MethodGet,
			path:       "/v1/repositories/acme/widgets/branches",
			pathValues: map[string]string{"repoOwner": "acme", "repoName": "widgets"},
			wantAction: "repository.read",
			wantType:   "repository",
			wantID:     "acme/widgets",
		},
		{
			name:       "step list uses filter",
			method:     http.MethodGet,
			path:       "/v1/steps",
			wantAction: "step.read",
			wantType:   "step",
			wantID:     "*",
			wantFilter: true,
		},
		{
			name:       "step validation uses step read",
			method:     http.MethodPost,
			path:       "/v1/steps/validate",
			wantAction: "step.read",
			wantType:   "step",
			wantID:     "*",
		},
		{
			name:       "schedule list uses filter",
			method:     http.MethodGet,
			path:       "/v1/schedules",
			wantAction: "pipeline_schedule.list",
			wantType:   "pipeline_schedule",
			wantID:     "*",
			wantFilter: true,
		},
		{
			name:       "dashboard list uses filter",
			method:     http.MethodGet,
			path:       "/v1/dashboards",
			wantAction: "dashboard.list",
			wantType:   "dashboard",
			wantID:     "*",
			wantFilter: true,
		},
		{
			name:       "dashboard view uses dashboard read",
			method:     http.MethodGet,
			path:       "/v1/dashboards/00000000-0000-0000-0000-000000000001/view",
			wantAction: "dashboard.read",
			wantType:   "dashboard",
			wantID:     "00000000-0000-0000-0000-000000000001",
		},
		{
			name:       "dashboard source mutation uses manage sources",
			method:     http.MethodPut,
			path:       "/v1/dashboards/00000000-0000-0000-0000-000000000001/sources/00000000-0000-0000-0000-000000000002",
			pathValues: map[string]string{"dashboardID": "00000000-0000-0000-0000-000000000001", "sourceID": "00000000-0000-0000-0000-000000000002"},
			wantAction: "dashboard.manage_sources",
			wantType:   "dashboard",
			wantID:     "00000000-0000-0000-0000-000000000001",
		},
		{
			name:       "dashboard section mutation uses dashboard update",
			method:     http.MethodPatch,
			path:       "/v1/dashboards/00000000-0000-0000-0000-000000000001/sections/00000000-0000-0000-0000-000000000002",
			pathValues: map[string]string{"dashboardID": "00000000-0000-0000-0000-000000000001", "sectionID": "00000000-0000-0000-0000-000000000002"},
			wantAction: "dashboard.update",
			wantType:   "dashboard",
			wantID:     "00000000-0000-0000-0000-000000000001",
		},
		{
			name:       "dashboard publication removal uses dashboard update",
			method:     http.MethodDelete,
			path:       "/v1/dashboards/00000000-0000-0000-0000-000000000001/publications/00000000-0000-0000-0000-000000000002",
			pathValues: map[string]string{"dashboardID": "00000000-0000-0000-0000-000000000001", "publicationID": "00000000-0000-0000-0000-000000000002"},
			wantAction: "dashboard.update",
			wantType:   "dashboard",
			wantID:     "00000000-0000-0000-0000-000000000001",
		},
		{
			name:       "dashboard refresh start uses refresh",
			method:     http.MethodPost,
			path:       "/v1/dashboards/00000000-0000-0000-0000-000000000001/refresh",
			pathValues: map[string]string{"dashboardID": "00000000-0000-0000-0000-000000000001"},
			wantAction: "dashboard.refresh",
			wantType:   "dashboard",
			wantID:     "00000000-0000-0000-0000-000000000001",
		},
		{
			name:       "dashboard refresh history uses read",
			method:     http.MethodGet,
			path:       "/v1/dashboards/00000000-0000-0000-0000-000000000001/refreshes/00000000-0000-0000-0000-000000000002",
			pathValues: map[string]string{"dashboardID": "00000000-0000-0000-0000-000000000001", "refreshID": "00000000-0000-0000-0000-000000000002"},
			wantAction: "dashboard.read",
			wantType:   "dashboard",
			wantID:     "00000000-0000-0000-0000-000000000001",
		},
		{
			name:       "dashboard refresh cancel uses refresh",
			method:     http.MethodPost,
			path:       "/v1/dashboards/00000000-0000-0000-0000-000000000001/refreshes/00000000-0000-0000-0000-000000000002/cancel",
			pathValues: map[string]string{"dashboardID": "00000000-0000-0000-0000-000000000001", "refreshID": "00000000-0000-0000-0000-000000000002"},
			wantAction: "dashboard.refresh",
			wantType:   "dashboard",
			wantID:     "00000000-0000-0000-0000-000000000001",
		},
		{
			name:       "dashboard refresh schedule list uses read",
			method:     http.MethodGet,
			path:       "/v1/dashboards/00000000-0000-0000-0000-000000000001/refresh-schedules",
			pathValues: map[string]string{"dashboardID": "00000000-0000-0000-0000-000000000001"},
			wantAction: "dashboard.read",
			wantType:   "dashboard",
			wantID:     "00000000-0000-0000-0000-000000000001",
		},
		{
			name:       "dashboard refresh schedule mutation uses update",
			method:     http.MethodPut,
			path:       "/v1/dashboards/00000000-0000-0000-0000-000000000001/refresh-schedules/00000000-0000-0000-0000-000000000003",
			pathValues: map[string]string{"dashboardID": "00000000-0000-0000-0000-000000000001", "scheduleID": "00000000-0000-0000-0000-000000000003"},
			wantAction: "dashboard.update",
			wantType:   "dashboard",
			wantID:     "00000000-0000-0000-0000-000000000001",
		},
		{
			name:       "dashboard refresh schedule run uses refresh",
			method:     http.MethodPost,
			path:       "/v1/dashboards/00000000-0000-0000-0000-000000000001/refresh-schedules/00000000-0000-0000-0000-000000000003/run",
			pathValues: map[string]string{"dashboardID": "00000000-0000-0000-0000-000000000001", "scheduleID": "00000000-0000-0000-0000-000000000003"},
			wantAction: "dashboard.refresh",
			wantType:   "dashboard",
			wantID:     "00000000-0000-0000-0000-000000000001",
		},
		{
			name:       "git webhook source list uses filter",
			method:     http.MethodGet,
			path:       "/v1/git-webhook-sources",
			wantAction: "git_webhook_source.read",
			wantType:   "git_webhook_source",
			wantID:     "*",
			wantFilter: true,
		},
		{
			name:       "git webhook source update",
			method:     http.MethodPut,
			path:       "/v1/git-webhook-sources/gitlab-main",
			pathValues: map[string]string{"sourceID": "gitlab-main"},
			wantAction: "git_webhook_source.update",
			wantType:   "git_webhook_source",
			wantID:     "gitlab-main",
		},
		{
			name:       "git webhook source deliveries",
			method:     http.MethodGet,
			path:       "/v1/git-webhook-sources/gitlab-main/deliveries",
			pathValues: map[string]string{"sourceID": "gitlab-main"},
			wantAction: "git_webhook_source.read",
			wantType:   "git_webhook_source",
			wantID:     "gitlab-main",
		},
		{
			name:       "git app read uses system config",
			method:     http.MethodGet,
			path:       "/v1/git-apps/github",
			wantAction: "system.read",
			wantType:   "system",
			wantID:     "config",
		},
		{
			name:       "git app installation create uses system config update",
			method:     http.MethodPost,
			path:       "/v1/git-apps/github/installations",
			wantAction: "system.update",
			wantType:   "system",
			wantID:     "config",
		},
		{
			name:       "git app installation repositories use system config read",
			method:     http.MethodGet,
			path:       "/v1/git-apps/github/installations/987654/repositories",
			wantAction: "system.read",
			wantType:   "system",
			wantID:     "config",
		},
		{
			name:       "step detail trims usage suffix",
			method:     http.MethodGet,
			path:       "/v1/steps/shared/util/archive/usage",
			pathValues: map[string]string{"stepPath": "shared/util/archive/usage"},
			wantAction: "step.read",
			wantType:   "step",
			wantID:     "shared/util/archive",
		},
		{
			name:       "step delete uses delete action",
			method:     http.MethodDelete,
			path:       "/v1/steps/shared/util/archive",
			pathValues: map[string]string{"stepPath": "shared/util/archive"},
			wantAction: "step.delete",
			wantType:   "step",
			wantID:     "shared/util/archive",
		},
		{
			name:       "system log source list uses resource filtering",
			method:     http.MethodGet,
			path:       "/v1/system/logs/sources",
			wantAction: "system_log.read",
			wantType:   "system_log",
			wantID:     "*",
			wantFilter: true,
		},
		{
			name:       "system log stream uses selected source",
			method:     http.MethodGet,
			path:       "/v1/system/logs/sources/dispatcher/stream",
			wantAction: "system_log.read",
			wantType:   "system_log",
			wantID:     "dispatcher",
		},
		{
			name:       "team config repository read uses team path",
			method:     http.MethodGet,
			path:       "/v1/teams/team-1/config-repository",
			wantAction: "config_repo.read",
			wantType:   "team",
			wantID:     "team-1",
		},
		{
			name:       "team config repository validation uses team path",
			method:     http.MethodPost,
			path:       "/v1/teams/team-1/config-repository/validate",
			wantAction: "config_repo.read",
			wantType:   "team",
			wantID:     "team-1",
		},
		{
			name:       "global config repo read uses system resource",
			method:     http.MethodGet,
			path:       "/v1/system/config-repo",
			wantAction: "system.read",
			wantType:   "system",
			wantID:     "config-repos",
		},
		{
			name:       "global config repo validation uses system resource",
			method:     http.MethodPost,
			path:       "/v1/system/config-repo/validate",
			wantAction: "system.read",
			wantType:   "system",
			wantID:     "config-repos",
		},
		{
			name:       "global config repo update uses system resource",
			method:     http.MethodPut,
			path:       "/v1/system/config-repo",
			wantAction: "system.update",
			wantType:   "system",
			wantID:     "config-repos",
		},
		{
			name:       "global config repo write uses system resource",
			method:     http.MethodPost,
			path:       "/v1/system/config-repo/write",
			wantAction: "system.update",
			wantType:   "system",
			wantID:     "config-repos",
		},
		{
			name:       "global config repo drift uses system resource",
			method:     http.MethodGet,
			path:       "/v1/system/config-repo/drift",
			wantAction: "system.read",
			wantType:   "system",
			wantID:     "config-repos",
		},
		{
			name:       "mail notification settings read uses notification system resource",
			method:     http.MethodGet,
			path:       "/v1/system/notifications/mail",
			wantAction: "system.read",
			wantType:   "system",
			wantID:     "notifications",
		},
		{
			name:       "mail notification test uses notification system update",
			method:     http.MethodPost,
			path:       "/v1/system/notifications/mail/test",
			wantAction: "system.update",
			wantType:   "system",
			wantID:     "notifications",
		},
		{
			name:       "data backups list uses system config read",
			method:     http.MethodGet,
			path:       "/v1/system/data/backups",
			wantAction: "system.read",
			wantType:   "system",
			wantID:     "config",
		},
		{
			name:       "data backup create uses system config update",
			method:     http.MethodPost,
			path:       "/v1/system/data/backups",
			wantAction: "system.update",
			wantType:   "system",
			wantID:     "config",
		},
		{
			name:       "data cleanup preview uses system config read",
			method:     http.MethodPost,
			path:       "/v1/system/data/cleanup/preview",
			wantAction: "system.read",
			wantType:   "system",
			wantID:     "config",
		},
		{
			name:       "data cleanup run uses system config update",
			method:     http.MethodPost,
			path:       "/v1/system/data/cleanup/run",
			wantAction: "system.update",
			wantType:   "system",
			wantID:     "config",
		},
		{
			name:       "llm profiles read defers to handler resource filtering",
			method:     http.MethodGet,
			path:       "/v1/system/llm-profiles",
			wantAction: "system.read",
			wantType:   "system",
			wantID:     "llm-profiles",
			wantFilter: true,
		},
		{
			name:       "credential list uses metadata action",
			method:     http.MethodGet,
			path:       "/v1/system/credentials",
			wantAction: "credential.list_metadata",
			wantType:   "credential",
			wantID:     "*",
			wantFilter: true,
		},
		{
			name:       "credential create uses create action",
			method:     http.MethodPost,
			path:       "/v1/system/credentials",
			wantAction: "credential.create",
			wantType:   "credential",
			wantID:     "*",
			wantFilter: true,
		},
		{
			name:       "credential rotation uses write value action",
			method:     http.MethodPut,
			path:       "/v1/system/credentials/00000000-0000-0000-0000-000000000001/value",
			wantAction: "credential.write_value",
			wantType:   "credential",
			wantID:     "00000000-0000-0000-0000-000000000001",
			wantFilter: true,
		},
		{
			name:       "credential version activation uses rotate action",
			method:     http.MethodPost,
			path:       "/v1/system/credentials/00000000-0000-0000-0000-000000000001/versions/2/activate",
			wantAction: "credential.rotate",
			wantType:   "credential",
			wantID:     "00000000-0000-0000-0000-000000000001",
			wantFilter: true,
		},
		{
			name:       "credential disable uses disable action",
			method:     http.MethodPost,
			path:       "/v1/system/credentials/00000000-0000-0000-0000-000000000001/disable",
			wantAction: "credential.disable",
			wantType:   "credential",
			wantID:     "00000000-0000-0000-0000-000000000001",
			wantFilter: true,
		},
		{
			name:       "credential enable uses enable action",
			method:     http.MethodPost,
			path:       "/v1/system/credentials/00000000-0000-0000-0000-000000000001/enable",
			wantAction: "credential.enable",
			wantType:   "credential",
			wantID:     "00000000-0000-0000-0000-000000000001",
			wantFilter: true,
		},
		{
			name:       "credential version delete uses delete version action",
			method:     http.MethodDelete,
			path:       "/v1/system/credentials/00000000-0000-0000-0000-000000000001/versions/2",
			wantAction: "credential.delete_version",
			wantType:   "credential",
			wantID:     "00000000-0000-0000-0000-000000000001",
			wantFilter: true,
		},
		{
			name:       "credential delete uses delete action",
			method:     http.MethodDelete,
			path:       "/v1/system/credentials/00000000-0000-0000-0000-000000000001",
			wantAction: "credential.delete",
			wantType:   "credential",
			wantID:     "00000000-0000-0000-0000-000000000001",
			wantFilter: true,
		},
		{
			name:       "runner compose template uses dispatcher runner update",
			method:     http.MethodGet,
			path:       "/v1/system/dispatcher/runner-compose",
			wantAction: "system.update",
			wantType:   "dispatcher",
			wantID:     "runners",
		},
		{
			name:       "runner bootstrap command uses dispatcher runner update",
			method:     http.MethodGet,
			path:       "/v1/system/dispatcher/runner-bootstrap-command",
			wantAction: "system.update",
			wantType:   "dispatcher",
			wantID:     "runners",
		},
		{
			name:       "kubernetes runner bootstrap command uses dispatcher runner update",
			method:     http.MethodGet,
			path:       "/v1/system/dispatcher/kubernetes-runner-bootstrap-command",
			wantAction: "system.update",
			wantType:   "dispatcher",
			wantID:     "runners",
		},
		{
			name:       "kubernetes runner manifest uses dispatcher runner update",
			method:     http.MethodGet,
			path:       "/v1/system/dispatcher/kubernetes-runner-manifest",
			wantAction: "system.update",
			wantType:   "dispatcher",
			wantID:     "runners",
		},
		{
			name:       "runner eject uses dispatcher runner update",
			method:     http.MethodDelete,
			path:       "/v1/system/dispatcher/runners/runner-prod-5",
			wantAction: "system.update",
			wantType:   "dispatcher",
			wantID:     "runners",
		},
		{
			name:       "llm profile delete defers to handler resource filtering",
			method:     http.MethodDelete,
			path:       "/v1/system/llm-profiles/fast",
			wantAction: "system.read",
			wantType:   "system",
			wantID:     "llm-profiles",
			wantFilter: true,
		},
		{
			name:       "agent profiles read defers to handler resource filtering",
			method:     http.MethodGet,
			path:       "/v1/system/agent-profiles",
			wantAction: "system.read",
			wantType:   "system",
			wantID:     "agent-profiles",
			wantFilter: true,
		},
		{
			name:       "agent profile delete defers to handler resource filtering",
			method:     http.MethodDelete,
			path:       "/v1/system/agent-profiles/sre",
			wantAction: "system.read",
			wantType:   "system",
			wantID:     "agent-profiles",
			wantFilter: true,
		},
		{
			name:       "agent profile default update defers to handler resource filtering",
			method:     http.MethodPut,
			path:       "/v1/system/agent-profiles/default",
			wantAction: "system.read",
			wantType:   "system",
			wantID:     "agent-profiles",
			wantFilter: true,
		},
		{
			name:       "mcp servers read defers to handler resource filtering",
			method:     http.MethodGet,
			path:       "/v1/system/mcp/servers",
			wantAction: "system.read",
			wantType:   "system",
			wantID:     "mcp",
			wantFilter: true,
		},
		{
			name:       "mcp profile test defers to handler resource filtering",
			method:     http.MethodPost,
			path:       "/v1/system/mcp/profiles/github-pr-review/test",
			wantAction: "system.read",
			wantType:   "system",
			wantID:     "mcp",
			wantFilter: true,
		},
		{
			name:       "team config repository manage uses team path",
			method:     http.MethodPut,
			path:       "/v1/teams/team-1/config-repository",
			wantAction: "config_repo.manage",
			wantType:   "team",
			wantID:     "team-1",
		},
		{
			name:       "team config repository sync uses team path",
			method:     http.MethodPost,
			path:       "/v1/teams/team-1/config-repository/sync",
			wantAction: "config_repo.sync",
			wantType:   "team",
			wantID:     "team-1",
		},
		{
			name:       "team config repository write uses team path",
			method:     http.MethodPost,
			path:       "/v1/teams/team-1/config-repository/write",
			wantAction: "config_repo.manage",
			wantType:   "team",
			wantID:     "team-1",
		},
		{
			name:       "team config repository drift uses team path",
			method:     http.MethodGet,
			path:       "/v1/teams/team-1/config-repository/drift",
			wantAction: "config_repo.read",
			wantType:   "team",
			wantID:     "team-1",
		},
		{
			name:       "team notifications read uses team path",
			method:     http.MethodGet,
			path:       "/v1/teams/team-1/notifications",
			wantAction: "config_repo.read",
			wantType:   "team",
			wantID:     "team-1",
		},
		{
			name:       "team notifications manage uses team path",
			method:     http.MethodPut,
			path:       "/v1/teams/team-1/notifications",
			wantAction: "config_repo.manage",
			wantType:   "team",
			wantID:     "team-1",
		},
		{
			name:       "team config repository decodes slash in team path",
			method:     http.MethodGet,
			path:       "/v1/teams/team-1%2Fplatform/config-repository",
			wantAction: "config_repo.read",
			wantType:   "team",
			wantID:     "team-1/platform",
		},
		{
			name:       "team list uses team filter",
			method:     http.MethodGet,
			path:       "/v1/teams",
			wantAction: "team.list",
			wantType:   "team",
			wantID:     "*",
			wantFilter: true,
		},
		{
			name:       "team detail uses team read",
			method:     http.MethodGet,
			path:       "/v1/teams/team-1",
			wantAction: "team.read",
			wantType:   "team",
			wantID:     "team-1",
		},
		{
			name:       "team applications use team read",
			method:     http.MethodGet,
			path:       "/v1/teams/team-1/applications",
			wantAction: "team.read",
			wantType:   "team",
			wantID:     "team-1",
		},
		{
			name:       "team config repository manage uses team path",
			method:     http.MethodPut,
			path:       "/v1/teams/team-1/config-repository",
			wantAction: "config_repo.manage",
			wantType:   "team",
			wantID:     "team-1",
		},
		{
			name:       "team notification read decodes slash in team path",
			method:     http.MethodGet,
			path:       "/v1/teams/team-1%2Fplatform/notifications",
			wantAction: "config_repo.read",
			wantType:   "team",
			wantID:     "team-1/platform",
		},
		{
			name:       "team LLM profile read uses team read",
			method:     http.MethodGet,
			path:       "/v1/teams/team-1/llm-profiles",
			wantAction: "team.read",
			wantType:   "team",
			wantID:     "team-1",
		},
		{
			name:       "team LLM profile write uses team update",
			method:     http.MethodPut,
			path:       "/v1/teams/team-1/llm-profiles/standard",
			wantAction: "team.update",
			wantType:   "team",
			wantID:     "team-1",
		},
		{
			name:       "team LLM default update decodes slash in team path",
			method:     http.MethodPut,
			path:       "/v1/teams/team-1%2Fplatform/llm-profiles/default",
			wantAction: "team.update",
			wantType:   "team",
			wantID:     "team-1/platform",
		},
		{
			name:       "team agent default update decodes slash in team path",
			method:     http.MethodPut,
			path:       "/v1/teams/team-1%2Fplatform/agent-profiles/default",
			wantAction: "team.update",
			wantType:   "team",
			wantID:     "team-1/platform",
		},
		{
			name:       "team MCP profile read uses team read",
			method:     http.MethodGet,
			path:       "/v1/teams/team-1/mcp-profiles/github-readonly",
			wantAction: "team.read",
			wantType:   "team",
			wantID:     "team-1",
		},
		{
			name:       "team MCP profile alias write uses team update",
			method:     http.MethodPost,
			path:       "/v1/teams/team-1/mcp/profiles",
			wantAction: "team.update",
			wantType:   "team",
			wantID:     "team-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			for key, value := range tt.pathValues {
				req.SetPathValue(key, value)
			}
			action, resource, requiresFilter, err := MapRequest(req)
			if err != nil {
				t.Fatalf("MapRequest() error = %v", err)
			}
			if action != tt.wantAction {
				t.Fatalf("MapRequest() action = %q, want %q", action, tt.wantAction)
			}
			if resource.Type != tt.wantType || resource.ID != tt.wantID {
				t.Fatalf("MapRequest() resource = %#v, want %s:%s", resource, tt.wantType, tt.wantID)
			}
			if requiresFilter != tt.wantFilter {
				t.Fatalf("MapRequest() requiresFilter = %t, want %t", requiresFilter, tt.wantFilter)
			}
		})
	}
}

func TestMapRequestDefersStepPutAuthorizationToHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/v1/steps/shared/util/archive", nil)
	req.SetPathValue("stepName", "shared/util/archive")
	action, resource, requiresFilter, err := MapRequest(req)
	if err != nil {
		t.Fatalf("MapRequest() error = %v", err)
	}
	if action != "" {
		t.Fatalf("action = %q, want empty", action)
	}
	if resource.Type != "step" || resource.ID != "shared/util/archive" {
		t.Fatalf("resource = %#v, want step:shared/util/archive", resource)
	}
	if requiresFilter {
		t.Fatal("requiresFilter = true, want false")
	}
}

func TestMapRequestDefersTriggerOverridePutAuthorizationToHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/v1/overrides/acme/widgets", nil)
	req.SetPathValue("repoOwner", "acme")
	req.SetPathValue("repoName", "widgets")
	action, resource, requiresFilter, err := MapRequest(req)
	if err != nil {
		t.Fatalf("MapRequest() error = %v", err)
	}
	if action != "" {
		t.Fatalf("action = %q, want empty", action)
	}
	if resource.Type != "trigger" || resource.ID != "acme/widgets" {
		t.Fatalf("resource = %#v, want trigger:acme/widgets", resource)
	}
	if requiresFilter {
		t.Fatal("requiresFilter = true, want false")
	}
}

func TestMapRequestDefersKnowledgeContextPutAuthorizationToHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/v1/knowledge-contexts/guardrail/team-1/repo-check", nil)
	req.SetPathValue("knowledgeID", "guardrail/team-1/repo-check")
	action, resource, requiresFilter, err := MapRequest(req)
	if err != nil {
		t.Fatalf("MapRequest() error = %v", err)
	}
	if action != "" {
		t.Fatalf("action = %q, want empty", action)
	}
	if resource.Type != "knowledge_context" || resource.ID != "guardrail/team-1/repo-check" {
		t.Fatalf("resource = %#v, want knowledge_context:guardrail/team-1/repo-check", resource)
	}
	if requiresFilter {
		t.Fatal("requiresFilter = true, want false")
	}
}

func TestRuntimeResourceBuildersNormalizeDefaultScope(t *testing.T) {
	if got, want := BuildSecretResource("", "default", "TOKEN"), BuildSecretResource("", "", "TOKEN"); got != want {
		t.Fatalf("BuildSecretResource(default) = %#v, want %#v", got, want)
	}
	if got, want := BuildVariableResource("owner/repo", "/default/", "API_URL"), BuildVariableResource("owner/repo", "", "API_URL"); got != want {
		t.Fatalf("BuildVariableResource(default) = %#v, want %#v", got, want)
	}
}

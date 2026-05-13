package routeauthz

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
			name:       "group list uses filter",
			method:     http.MethodGet,
			path:       "/v1/groups",
			wantAction: "folder.list",
			wantType:   "folder",
			wantID:     "*",
			wantFilter: true,
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
			name:       "trigger override maps nested id before mux path values",
			method:     http.MethodGet,
			path:       "/v1/overrides/team-1/dev/hosein-yousefii/test-app",
			wantAction: "trigger.read",
			wantType:   "trigger",
			wantID:     "team-1/dev/hosein-yousefii/test-app",
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
			name:       "group config repo read uses group path",
			method:     http.MethodGet,
			path:       "/v1/groups/team-1/config-repo",
			wantAction: "config_repo.read",
			wantType:   "folder",
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
			name:       "global config repo update uses system resource",
			method:     http.MethodPut,
			path:       "/v1/system/config-repo",
			wantAction: "system.update",
			wantType:   "system",
			wantID:     "config-repos",
		},
		{
			name:       "group config repo manage uses group path",
			method:     http.MethodPut,
			path:       "/v1/groups/team-1/config-repo",
			wantAction: "config_repo.manage",
			wantType:   "folder",
			wantID:     "team-1",
		},
		{
			name:       "group config repo sync uses group path",
			method:     http.MethodPost,
			path:       "/v1/groups/team-1/config-repo/sync",
			wantAction: "config_repo.sync",
			wantType:   "folder",
			wantID:     "team-1",
		},
		{
			name:       "group config repo decodes slash in group path",
			method:     http.MethodGet,
			path:       "/v1/groups/team-1%2Fplatform/config-repo",
			wantAction: "config_repo.read",
			wantType:   "folder",
			wantID:     "team-1/platform",
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

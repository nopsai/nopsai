package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/aaaclient"
	"nopsai/services/nopsai/pkg/auth"
	"nopsai/services/nopsai/pkg/routeauthz"
)

type mockAAACalls struct {
	checks  int
	filters int
}

func TestAuthzMiddlewareDeniesNonAdminAdminEndpoint(t *testing.T) {
	calls := &mockAAACalls{}
	client := newInMemoryAAAClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/authz/check":
			calls.checks++
			_ = json.NewEncoder(w).Encode(model.Decision{Allowed: false, Reason: "default_deny"})
		default:
			http.NotFound(w, r)
		}
	}))

	app := &App{aaaClient: client}
	nextCalled := false
	handler := app.authzMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := withClaimsRequest(http.MethodGet, "/v1/admin/users", ``, &auth.Claims{Sub: "viewer", Email: "viewer@example.com", Provider: "local"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if nextCalled {
		t.Fatal("next handler was called for denied admin request")
	}
	if calls.checks != 1 {
		t.Fatalf("AAA check calls = %d, want 1", calls.checks)
	}
}

func TestAuthzMiddlewareFailsClosedWhenAAAUnavailable(t *testing.T) {
	app := &App{aaaClient: aaaclient.New("http://127.0.0.1:1", "dev-default-for-local-only")}
	handler := app.authzMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := withClaimsRequest(http.MethodGet, "/v1/admin/users", ``, &auth.Claims{Sub: "viewer", Provider: "local"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestPipelineListFilteringDoesNotLeakUnauthorizedNames(t *testing.T) {
	calls := &mockAAACalls{}
	allowedPipeline := routeauthz.PipelineResource("team", "alpha")

	client := newInMemoryAAAClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/authz/filter":
			calls.filters++
			var req model.FilterRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode filter request: %v", err)
			}
			_ = json.NewEncoder(w).Encode(model.FilterResponse{Resources: []model.ResourceRef{allowedPipeline}})
		case "/v1/authz/check":
			calls.checks++
			t.Fatalf("unexpected coarse authz check for filtered list endpoint")
		default:
			http.NotFound(w, r)
		}
	}))

	app := &App{aaaClient: client}
	handler := app.authzMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resources := []model.ResourceRef{
			routeauthz.PipelineResource("team", "alpha"),
			routeauthz.PipelineResource("team", "beta"),
		}
		allowed, err := app.allowedResourceSet(r, "pipeline.list", resources)
		if err != nil {
			http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
			return
		}
		names := make([]string, 0, len(resources))
		for _, resource := range resources {
			if _, ok := allowed[resourceKey(resource)]; ok {
				names = append(names, resource.ID)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(names)
	}))

	req := withClaimsRequest(http.MethodGet, "/v1/pipelines", ``, &auth.Claims{Sub: "viewer", Email: "viewer@example.com", Provider: "local"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var names []string
	if err := json.Unmarshal(rec.Body.Bytes(), &names); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(names) != 1 || names[0] != allowedPipeline.ID {
		t.Fatalf("visible pipelines = %#v, want [%q]", names, allowedPipeline.ID)
	}
	if calls.filters != 1 || calls.checks != 0 {
		t.Fatalf("AAA calls = %#v, want filter-only", calls)
	}
}

func TestRunnerExecuteAllowedButUpdateDenied(t *testing.T) {
	client := newInMemoryAAAClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/authz/check":
			var req model.CheckRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode check request: %v", err)
			}
			allowed := req.Action == "pipeline.execute"
			_ = json.NewEncoder(w).Encode(model.Decision{Allowed: allowed, Reason: "mock"})
		default:
			http.NotFound(w, r)
		}
	}))

	app := &App{aaaClient: client}

	executeHandler := app.authzMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	executeReq := withClaimsRequest(http.MethodPost, "/v1/run/foo", ``, &auth.Claims{Sub: "runner", Email: "runner@example.com", Provider: "local"})
	executeReq.SetPathValue("pipelineName", "foo")
	executeRec := httptest.NewRecorder()
	executeHandler.ServeHTTP(executeRec, executeReq)
	if executeRec.Code != http.StatusNoContent {
		t.Fatalf("execute status = %d, want %d", executeRec.Code, http.StatusNoContent)
	}

	updateHandler := app.authzMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !app.requireAAADecision(w, r, "pipeline.update", routeauthz.PipelineResource("", "foo")) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	updateReq := withClaimsRequest(http.MethodPut, "/v1/pipelines/foo", "name: foo", &auth.Claims{Sub: "runner", Email: "runner@example.com", Provider: "local"})
	updateReq.SetPathValue("pipelineName", "foo")
	updateRec := httptest.NewRecorder()
	updateHandler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusForbidden {
		t.Fatalf("update status = %d, want %d", updateRec.Code, http.StatusForbidden)
	}
}

func TestDispatcherRequestsUseInternalServiceSubject(t *testing.T) {
	client := newInMemoryAAAClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/authz/check":
			var req model.CheckRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode check request: %v", err)
			}
			if req.Subject.Type != model.SubjectTypeInternalService {
				t.Fatalf("subject type = %q, want %q", req.Subject.Type, model.SubjectTypeInternalService)
			}
			if req.Subject.ID != "dispatcher" {
				t.Fatalf("subject id = %q, want dispatcher", req.Subject.ID)
			}
			if req.Action != "pipeline.read" {
				t.Fatalf("action = %q, want pipeline.read", req.Action)
			}
			if req.Resource != routeauthz.PipelineResource("team", "build") {
				t.Fatalf("resource = %#v, want %#v", req.Resource, routeauthz.PipelineResource("team", "build"))
			}
			_ = json.NewEncoder(w).Encode(model.Decision{Allowed: true, Reason: "mock"})
		default:
			http.NotFound(w, r)
		}
	}))

	app := &App{aaaClient: client}
	handler := app.authzMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := withClaimsRequest(http.MethodGet, "/v1/pipelines/team/build", ``, &auth.Claims{Sub: "dispatcher", Provider: "internal-service"})
	req.SetPathValue("pipelineName", "team/build")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestBranchRunDeletionRouteUsesAAA(t *testing.T) {
	calls := &mockAAACalls{}
	client := newInMemoryAAAClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/authz/check":
			calls.checks++
			var req model.CheckRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode check request: %v", err)
			}
			if req.Action != "pipeline_run.delete" {
				t.Fatalf("action = %q, want pipeline_run.delete", req.Action)
			}
			if req.Resource.Type != "repository" || req.Resource.ID != "acme/widgets" {
				t.Fatalf("resource = %#v, want repository acme/widgets", req.Resource)
			}
			_ = json.NewEncoder(w).Encode(model.Decision{Allowed: false, Reason: "default_deny"})
		default:
			http.NotFound(w, r)
		}
	}))

	app := &App{aaaClient: client}
	nextCalled := false
	handler := app.authzMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := withClaimsRequest(http.MethodDelete, "/v1/repositories/acme/widgets/branches/main", ``, &auth.Claims{Sub: "viewer", Email: "viewer@example.com", Provider: "local"})
	req.SetPathValue("repoOwner", "acme")
	req.SetPathValue("repoName", "widgets")
	req.SetPathValue("branch", "main")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if nextCalled {
		t.Fatal("next handler was called for denied branch deletion request")
	}
	if calls.checks != 1 {
		t.Fatalf("AAA check calls = %d, want 1", calls.checks)
	}
}

func TestDefaultAdminAllowedThroughMiddleware(t *testing.T) {
	client := newInMemoryAAAClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/authz/check":
			var req model.CheckRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode check request: %v", err)
			}
			allowed := req.Subject.Sub == "admin" && req.Action == "iam.admin"
			_ = json.NewEncoder(w).Encode(model.Decision{Allowed: allowed, Reason: "mock"})
		default:
			http.NotFound(w, r)
		}
	}))

	app := &App{aaaClient: client}
	handler := app.authzMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := withClaimsRequest(http.MethodGet, "/v1/admin/users", ``, &auth.Claims{Sub: "admin", Email: "admin@example.com", Provider: "local"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func withClaimsRequest(method, target, body string, claims *auth.Claims) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := context.WithValue(req.Context(), ctxKeyRequestID, "test-request")
	if claims != nil {
		ctx = auth.WithClaims(ctx, claims)
	}
	return req.WithContext(ctx)
}

func newInMemoryAAAClient(t *testing.T, handler http.Handler) *aaaclient.Client {
	t.Helper()
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			return rec.Result(), nil
		}),
	}
	return aaaclient.NewWithHTTPClient("http://aaa", "dev-default-for-local-only", httpClient)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

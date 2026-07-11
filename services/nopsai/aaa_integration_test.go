package nopsai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type stubAAAAuthorizer struct {
	introspectFn  func(context.Context, model.Subject) (*model.IntrospectResponse, error)
	checkFn       func(context.Context, model.Subject, string, model.ResourceRef, map[string]any) (model.Decision, error)
	batchCheckFn  func(context.Context, model.Subject, []model.BatchCheckItem, map[string]any) ([]model.Decision, error)
	filterFn      func(context.Context, model.Subject, string, []model.ResourceRef, map[string]any) ([]model.ResourceRef, error)
	recordAuditFn func(context.Context, model.AuditRecordRequest) error
}

func (s stubAAAAuthorizer) Introspect(ctx context.Context, subject model.Subject) (*model.IntrospectResponse, error) {
	if s.introspectFn == nil {
		return nil, fmt.Errorf("unexpected introspect call")
	}
	return s.introspectFn(ctx, subject)
}

func (s stubAAAAuthorizer) Check(ctx context.Context, subject model.Subject, action string, resource model.ResourceRef, requestContext map[string]any) (model.Decision, error) {
	if s.checkFn == nil {
		return model.Decision{}, fmt.Errorf("unexpected check call")
	}
	return s.checkFn(ctx, subject, action, resource, requestContext)
}

func (s stubAAAAuthorizer) BatchCheck(ctx context.Context, subject model.Subject, checks []model.BatchCheckItem, requestContext map[string]any) ([]model.Decision, error) {
	if s.batchCheckFn != nil {
		return s.batchCheckFn(ctx, subject, checks, requestContext)
	}
	decisions := make([]model.Decision, 0, len(checks))
	for _, check := range checks {
		decision, err := s.Check(ctx, subject, check.Action, check.Resource, requestContext)
		if err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

func (s stubAAAAuthorizer) Filter(ctx context.Context, subject model.Subject, action string, resources []model.ResourceRef, requestContext map[string]any) ([]model.ResourceRef, error) {
	if s.filterFn == nil {
		return nil, fmt.Errorf("unexpected filter call")
	}
	return s.filterFn(ctx, subject, action, resources, requestContext)
}

func (s stubAAAAuthorizer) RecordAudit(ctx context.Context, req model.AuditRecordRequest) error {
	if s.recordAuditFn == nil {
		return fmt.Errorf("unexpected record audit call")
	}
	return s.recordAuditFn(ctx, req)
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

func TestAuthzMiddlewareDefersTeamListAuthorizationToHandlerFiltering(t *testing.T) {
	remoteCalls := 0
	localCalls := 0
	app := &App{
		aaaClient: stubAAAAuthorizer{
			checkFn: func(context.Context, model.Subject, string, model.ResourceRef, map[string]any) (model.Decision, error) {
				remoteCalls++
				return model.Decision{}, errors.New("aaa unavailable")
			},
		},
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ context.Context, subject model.Subject, action string, resource model.ResourceRef, _ map[string]any) (model.Decision, error) {
				localCalls++
				if subject.Sub != "admin" {
					t.Fatalf("subject sub = %q, want admin", subject.Sub)
				}
				if action != "team.list" {
					t.Fatalf("action = %q, want team.list", action)
				}
				if resource.Type != "team" || resource.ID != "*" {
					t.Fatalf("resource = %#v, want team:*", resource)
				}
				return model.Decision{Allowed: true, Reason: "fallback"}, nil
			},
		},
	}

	nextCalled := false
	handler := app.authzMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := withClaimsRequest(http.MethodGet, "/v1/teams", ``, &auth.Claims{Sub: "admin", Email: "admin@example.com", Provider: "local"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if !nextCalled {
		t.Fatal("next handler was not called for filtered team list request")
	}
	if remoteCalls != 0 || localCalls != 0 {
		t.Fatalf("remote/local calls = %d/%d, want 0/0 until handler filtering", remoteCalls, localCalls)
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

func TestAllowedResourceSetFallsBackToLocalEvaluator(t *testing.T) {
	allowedPipeline := routeauthz.PipelineResource("team", "alpha")
	app := &App{
		aaaClient: stubAAAAuthorizer{
			filterFn: func(context.Context, model.Subject, string, []model.ResourceRef, map[string]any) ([]model.ResourceRef, error) {
				return nil, errors.New("aaa unavailable")
			},
		},
		aaaLocal: stubAAAAuthorizer{
			filterFn: func(_ context.Context, subject model.Subject, action string, resources []model.ResourceRef, _ map[string]any) ([]model.ResourceRef, error) {
				if subject.Sub != "viewer" {
					t.Fatalf("subject sub = %q, want viewer", subject.Sub)
				}
				if action != "pipeline.list" {
					t.Fatalf("action = %q, want pipeline.list", action)
				}
				if len(resources) != 2 {
					t.Fatalf("resources = %d, want 2", len(resources))
				}
				return []model.ResourceRef{allowedPipeline}, nil
			},
		},
	}

	req := withClaimsRequest(http.MethodGet, "/v1/pipelines", ``, &auth.Claims{Sub: "viewer", Email: "viewer@example.com", Provider: "local"})
	allowed, err := app.allowedResourceSet(req, "pipeline.list", []model.ResourceRef{
		allowedPipeline,
		routeauthz.PipelineResource("team", "beta"),
	})
	if err != nil {
		t.Fatalf("allowedResourceSet() error = %v", err)
	}
	if len(allowed) != 1 {
		t.Fatalf("allowed set size = %d, want 1", len(allowed))
	}
	if _, ok := allowed[resourceKey(allowedPipeline)]; !ok {
		t.Fatalf("allowed set = %#v, want %q", allowed, resourceKey(allowedPipeline))
	}
}

func TestAAABatchCheckFallsBackToLocalEvaluator(t *testing.T) {
	remoteCalls := 0
	localCalls := 0
	app := &App{
		aaaClient: stubAAAAuthorizer{
			batchCheckFn: func(context.Context, model.Subject, []model.BatchCheckItem, map[string]any) ([]model.Decision, error) {
				remoteCalls++
				return nil, errors.New("aaa unavailable")
			},
		},
		aaaLocal: stubAAAAuthorizer{
			batchCheckFn: func(_ context.Context, subject model.Subject, checks []model.BatchCheckItem, requestContext map[string]any) ([]model.Decision, error) {
				localCalls++
				if subject.Sub != "viewer" {
					t.Fatalf("subject sub = %q, want viewer", subject.Sub)
				}
				if got := requestContext["request_id"]; got != "batch-1" {
					t.Fatalf("request_id = %#v, want batch-1", got)
				}
				decisions := make([]model.Decision, 0, len(checks))
				for range checks {
					decisions = append(decisions, model.Decision{Allowed: true, Reason: "fallback"})
				}
				return decisions, nil
			},
		},
	}

	decisions, err := app.aaaBatchCheck(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"}, []model.BatchCheckItem{{
		Action:   "pipeline.read",
		Resource: routeauthz.PipelineResource("team", "alpha"),
	}}, map[string]any{"request_id": "batch-1"})
	if err != nil {
		t.Fatalf("aaaBatchCheck() error = %v", err)
	}
	if len(decisions) != 1 || !decisions[0].Allowed {
		t.Fatalf("decisions = %#v, want one allowed decision", decisions)
	}
	if remoteCalls != 1 || localCalls != 1 {
		t.Fatalf("remote/local batch calls = %d/%d, want 1/1", remoteCalls, localCalls)
	}
}

func TestAAARecordAuditFallsBackToLocalEvaluator(t *testing.T) {
	remoteCalls := 0
	localCalls := 0
	app := &App{
		aaaClient: stubAAAAuthorizer{
			recordAuditFn: func(context.Context, model.AuditRecordRequest) error {
				remoteCalls++
				return errors.New("aaa unavailable")
			},
		},
		aaaLocal: stubAAAAuthorizer{
			recordAuditFn: func(_ context.Context, req model.AuditRecordRequest) error {
				localCalls++
				if req.RequestID != "audit-1" {
					t.Fatalf("request id = %q, want audit-1", req.RequestID)
				}
				if req.Action != "pipeline.execute" {
					t.Fatalf("action = %q, want pipeline.execute", req.Action)
				}
				return nil
			},
		},
	}

	err := app.aaaRecordAudit(context.Background(), model.AuditRecordRequest{
		RequestID: "audit-1",
		Subject:   model.Subject{Type: model.SubjectTypeUser, ID: "viewer"},
		Action:    "pipeline.execute",
		Resource:  routeauthz.PipelineResource("team", "alpha"),
		Allowed:   true,
		Reason:    "manual_record",
	})
	if err != nil {
		t.Fatalf("aaaRecordAudit() error = %v", err)
	}
	if remoteCalls != 1 || localCalls != 1 {
		t.Fatalf("remote/local audit calls = %d/%d, want 1/1", remoteCalls, localCalls)
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

func TestSyntheticGitRunRequestsUseDispatcherInternalSubject(t *testing.T) {
	calls := 0
	client := newInMemoryAAAClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/authz/check":
			calls++
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
			if req.Action != "pipeline.execute" {
				t.Fatalf("action = %q, want pipeline.execute", req.Action)
			}
			if req.Resource != routeauthz.PipelineResource("", "reference-pipeline") {
				t.Fatalf("resource = %#v, want %#v", req.Resource, routeauthz.PipelineResource("", "reference-pipeline"))
			}
			_ = json.NewEncoder(w).Encode(model.Decision{Allowed: true, Reason: "mock"})
		default:
			http.NotFound(w, r)
		}
	}))

	app := &App{aaaClient: client}
	req := httptest.NewRequest(http.MethodPost, "/v1/run/reference-pipeline", nil)
	req.SetPathValue("pipelineName", "reference-pipeline")
	req = app.withDispatcherInternalSubject(req)

	rec := httptest.NewRecorder()
	if !app.requireAAADecision(rec, req, "pipeline.execute", routeauthz.PipelineResource("", "reference-pipeline")) {
		t.Fatalf("expected synthetic git run request to be authorized, got status %d", rec.Code)
	}
	if calls != 1 {
		t.Fatalf("AAA check calls = %d, want 1", calls)
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

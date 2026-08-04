package nopsai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nopsai/config"
	"nopsai/pkg/proto"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/systemlogs"
)

type systemLogTestProvider struct {
	sources []systemlogs.SourceStatus
	tail    []systemlogs.Entry
}

type systemLogFlushRecorder struct {
	*httptest.ResponseRecorder
	flushed chan struct{}
}

func (r *systemLogFlushRecorder) Flush() {
	r.ResponseRecorder.Flush()
	select {
	case r.flushed <- struct{}{}:
	default:
	}
}

func (p *systemLogTestProvider) ListSources(context.Context) ([]systemlogs.SourceStatus, error) {
	if len(p.sources) > 0 {
		return append([]systemlogs.SourceStatus(nil), p.sources...), nil
	}
	return []systemlogs.SourceStatus{{ID: "dispatcher", Available: true, State: "running"}}, nil
}
func (p *systemLogTestProvider) Tail(context.Context, string, int) ([]systemlogs.Entry, error) {
	return append([]systemlogs.Entry(nil), p.tail...), nil
}
func (p *systemLogTestProvider) Follow(ctx context.Context, _ string, _ systemlogs.Cursor, _ func(systemlogs.Entry)) error {
	<-ctx.Done()
	return ctx.Err()
}

func newSystemLogHandlerTestApp(provider systemlogs.Provider) *App {
	registry := systemlogs.NewRegistry([]systemlogs.Source{{ID: "dispatcher", ContainerName: "nopsai-dispatcher"}})
	return &App{
		systemLogs:       systemlogs.NewBroker(provider, registry, systemlogs.NewRedactor(1024), systemlogs.NewCursorCodec([]byte("test")), systemlogs.BrokerOptions{RetryDelay: time.Millisecond}),
		systemLogLimiter: newSystemLogRateLimiter(10, time.Minute),
	}
}

func TestHandleTailSystemLogsReturnsRedactedEntries(t *testing.T) {
	app := newSystemLogHandlerTestApp(&systemLogTestProvider{tail: []systemlogs.Entry{{ContainerInstance: "one", Stream: systemlogs.StreamStdout, Line: "token=secret"}}})
	req := httptest.NewRequest(http.MethodGet, "/v1/system/logs/sources/dispatcher/tail?lines=20", nil)
	req.SetPathValue("sourceID", "dispatcher")
	recorder := httptest.NewRecorder()
	app.handleTailSystemLogs(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "token=secret") || !strings.Contains(recorder.Body.String(), "[REDACTED]") {
		t.Fatalf("body = %s, want redacted entry", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "best effort") {
		t.Fatalf("body = %s, want warning", recorder.Body.String())
	}
}

func TestHandleListSystemLogSourcesFiltersWithAAA(t *testing.T) {
	app := newSystemLogHandlerTestApp(&systemLogTestProvider{})
	app.aaaLocal = stubAAAAuthorizer{filterFn: func(_ context.Context, _ model.Subject, action string, resources []model.ResourceRef, _ map[string]any) ([]model.ResourceRef, error) {
		if action != "system_log.read" || len(resources) != 1 || resources[0].ID != "dispatcher" {
			t.Fatalf("filter request = %q %#v", action, resources)
		}
		return resources, nil
	}}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/logs/sources", nil)
	req = req.WithContext(withAAASubject(req.Context(), model.Subject{Type: model.SubjectTypeUser, Sub: "operator"}))
	recorder := httptest.NewRecorder()
	app.handleListSystemLogSources(recorder, req)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"id":"dispatcher"`) {
		t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestHandleListSystemLogSourcesHidesUnregisteredRunnerSources(t *testing.T) {
	app := newSystemLogHandlerTestApp(&systemLogTestProvider{sources: []systemlogs.SourceStatus{
		{ID: "dispatcher", Available: true, State: "running"},
		{ID: systemlogs.RunnerSourceID("runner-ejected"), Available: true, State: "running"},
		{ID: systemlogs.RunnerSourceID("runner-active"), Available: true, State: "running"},
	}})
	app.dispatcher = &fakeDispatcherClient{status: &proto.DispatcherStatus{Runners: []*proto.RunnerInfo{{
		RunnerId: "runner-active",
		Metadata: map[string]string{"log_source_id": systemlogs.RunnerSourceID("runner-active")},
	}}}}
	app.aaaLocal = stubAAAAuthorizer{filterFn: func(_ context.Context, _ model.Subject, _ string, resources []model.ResourceRef, _ map[string]any) ([]model.ResourceRef, error) {
		return resources, nil
	}}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/logs/sources", nil)
	req = req.WithContext(withAAASubject(req.Context(), model.Subject{Type: model.SubjectTypeUser, Sub: "operator"}))
	recorder := httptest.NewRecorder()
	app.handleListSystemLogSources(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if strings.Contains(body, "runner-ejected") {
		t.Fatalf("body = %s, want unregistered runner hidden", body)
	}
	if !strings.Contains(body, "runner-active") {
		t.Fatalf("body = %s, want registered runner visible", body)
	}
}

func TestHandleListSystemLogSourcesIncludesRegisteredRunnerWithoutProviderSource(t *testing.T) {
	sourceID := systemlogs.RunnerSourceID("runner-k8s")
	app := newSystemLogHandlerTestApp(&systemLogTestProvider{sources: []systemlogs.SourceStatus{
		{ID: "dispatcher", Available: true, State: "running"},
	}})
	app.dispatcher = &fakeDispatcherClient{status: &proto.DispatcherStatus{Runners: []*proto.RunnerInfo{{
		RunnerId: "runner-k8s",
		Metadata: map[string]string{
			"runtime":                   "kubernetes",
			"kubernetes_namespace":      "runner-ns",
			"kubernetes_label_selector": "app.kubernetes.io/name=nopsai-k8s-runner,nopsai.io/runner-id=runner-k8s",
			"log_source_id":             sourceID,
		},
	}}}}
	app.aaaLocal = stubAAAAuthorizer{filterFn: func(_ context.Context, _ model.Subject, _ string, resources []model.ResourceRef, _ map[string]any) ([]model.ResourceRef, error) {
		return resources, nil
	}}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/logs/sources", nil)
	req = req.WithContext(withAAASubject(req.Context(), model.Subject{Type: model.SubjectTypeUser, Sub: "operator"}))
	recorder := httptest.NewRecorder()

	app.handleListSystemLogSources(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, want := range []string{`"id":"` + sourceID + `"`, `"display_name":"Runner runner-k8s"`, `"state":"unavailable"`, "Kubernetes pod not discovered"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %s, want %q", body, want)
		}
	}
}

func TestHandleListSystemLogSourcesAnnotatesRecentlyReconnectedRunner(t *testing.T) {
	sourceID := systemlogs.RunnerSourceID("runner-k8s")
	disconnectedAt := time.Now().Add(-5 * time.Minute).UTC().Format(time.RFC3339)
	app := newSystemLogHandlerTestApp(&systemLogTestProvider{sources: []systemlogs.SourceStatus{
		{ID: sourceID, DisplayName: "Runner runner-k8s", ContainerName: "runner", Available: true, State: "running", Health: "ready"},
	}})
	app.dispatcher = &fakeDispatcherClient{status: &proto.DispatcherStatus{Runners: []*proto.RunnerInfo{{
		RunnerId: "runner-k8s",
		Metadata: map[string]string{
			"runtime":              "kubernetes",
			"log_source_id":        sourceID,
			"reachable":            "true",
			"connection_status":    "online",
			"last_disconnected_at": disconnectedAt,
		},
	}}}}
	app.aaaLocal = stubAAAAuthorizer{filterFn: func(_ context.Context, _ model.Subject, _ string, resources []model.ResourceRef, _ map[string]any) ([]model.ResourceRef, error) {
		return resources, nil
	}}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/logs/sources", nil)
	req = req.WithContext(withAAASubject(req.Context(), model.Subject{Type: model.SubjectTypeUser, Sub: "operator"}))
	recorder := httptest.NewRecorder()

	app.handleListSystemLogSources(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, want := range []string{`"id":"` + sourceID + `"`, `"health":"recently reconnected"`, "dispatcher stream reconnected after disconnect"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %s, want %q", body, want)
		}
	}
}

func TestHandleListSystemLogSourcesAnnotatesUnreachableRegisteredRunner(t *testing.T) {
	sourceID := systemlogs.RunnerSourceID("runner-k8s")
	disconnectedAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	app := newSystemLogHandlerTestApp(&systemLogTestProvider{sources: []systemlogs.SourceStatus{
		{ID: sourceID, DisplayName: "Runner runner-k8s", ContainerName: "runner", Available: true, State: "running", Health: "ready"},
	}})
	app.dispatcher = &fakeDispatcherClient{status: &proto.DispatcherStatus{Runners: []*proto.RunnerInfo{{
		RunnerId: "runner-k8s",
		Metadata: map[string]string{
			"runtime":              "kubernetes",
			"log_source_id":        sourceID,
			"reachable":            "false",
			"connection_status":    "unreachable",
			"last_disconnected_at": disconnectedAt,
		},
	}}}}
	app.aaaLocal = stubAAAAuthorizer{filterFn: func(_ context.Context, _ model.Subject, _ string, resources []model.ResourceRef, _ map[string]any) ([]model.ResourceRef, error) {
		return resources, nil
	}}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/logs/sources", nil)
	req = req.WithContext(withAAASubject(req.Context(), model.Subject{Type: model.SubjectTypeUser, Sub: "operator"}))
	recorder := httptest.NewRecorder()

	app.handleListSystemLogSources(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, want := range []string{`"id":"` + sourceID + `"`, `"health":"dispatcher unreachable"`, "dispatcher connection unreachable"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %s, want %q", body, want)
		}
	}
}

func TestHandleTailSystemLogsValidatesLineCountAndSource(t *testing.T) {
	app := newSystemLogHandlerTestApp(&systemLogTestProvider{})
	for _, tt := range []struct {
		path, source string
		status       int
	}{
		{path: "/v1/system/logs/sources/dispatcher/tail?lines=-1", source: "dispatcher", status: http.StatusBadRequest},
		{path: "/v1/system/logs/sources/base/tail", source: "base", status: http.StatusNotFound},
	} {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		req.SetPathValue("sourceID", tt.source)
		recorder := httptest.NewRecorder()
		app.handleTailSystemLogs(recorder, req)
		if recorder.Code != tt.status {
			t.Fatalf("%s status = %d, want %d", tt.path, recorder.Code, tt.status)
		}
	}
}

func TestHandleStreamSystemLogsWritesAuthenticatedFetchSSEContract(t *testing.T) {
	app := newSystemLogHandlerTestApp(&systemLogTestProvider{tail: []systemlogs.Entry{{ContainerInstance: "one", Stream: systemlogs.StreamStderr, Line: "failed"}}})
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/v1/system/logs/sources/dispatcher/stream?tail=1", nil).WithContext(ctx)
	req.SetPathValue("sourceID", "dispatcher")
	recorder := &systemLogFlushRecorder{ResponseRecorder: httptest.NewRecorder(), flushed: make(chan struct{}, 1)}
	done := make(chan struct{})
	go func() { defer close(done); app.handleStreamSystemLogs(recorder, req) }()
	select {
	case <-recorder.flushed:
	case <-time.After(time.Second):
		t.Fatal("stream did not flush initial events")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not close after cancellation")
	}
	if recorder.Header().Get("Content-Type") != "text/event-stream; charset=utf-8" {
		t.Fatalf("Content-Type = %q", recorder.Header().Get("Content-Type"))
	}
	body := recorder.Body.String()
	for _, value := range []string{"event: status", `"state":"connected"`, "event: log", `"stream":"stderr"`} {
		if !strings.Contains(body, value) {
			t.Fatalf("SSE body = %q, want %q", body, value)
		}
	}
}

func TestHandleStreamSystemLogsAcceptsEncodedDynamicRunnerSource(t *testing.T) {
	sourceID := systemlogs.RunnerSourceID("runner-general2")
	app := newSystemLogHandlerTestApp(&systemLogTestProvider{tail: []systemlogs.Entry{{ContainerInstance: "runner-general2", Stream: systemlogs.StreamStdout, Line: "runner ready"}}})
	app.dispatcher = &fakeDispatcherClient{status: &proto.DispatcherStatus{Runners: []*proto.RunnerInfo{{
		RunnerId: "runner-general2",
		Metadata: map[string]string{"log_source_id": sourceID},
	}}}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/system/logs/sources/{sourceID}/stream", app.handleStreamSystemLogs)
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/v1/system/logs/sources/runner%3Arunner-general2/stream?tail=1", nil).WithContext(ctx)
	recorder := &systemLogFlushRecorder{ResponseRecorder: httptest.NewRecorder(), flushed: make(chan struct{}, 1)}
	done := make(chan struct{})
	go func() { defer close(done); mux.ServeHTTP(recorder, req) }()
	select {
	case <-recorder.flushed:
	case <-time.After(time.Second):
		t.Fatal("stream did not flush initial events")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not close after cancellation")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, value := range []string{"event: status", `"source_id":"` + sourceID + `"`, "event: log", `"runner ready"`} {
		if !strings.Contains(body, value) {
			t.Fatalf("SSE body = %q, want %q", body, value)
		}
	}
}

func TestHandleTailSystemLogsRejectsUnregisteredRunnerSource(t *testing.T) {
	sourceID := systemlogs.RunnerSourceID("runner-ejected")
	app := newSystemLogHandlerTestApp(&systemLogTestProvider{tail: []systemlogs.Entry{{ContainerInstance: "runner-ejected", Stream: systemlogs.StreamStdout, Line: "still running"}}})
	app.dispatcher = &fakeDispatcherClient{status: &proto.DispatcherStatus{}}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/logs/sources/runner%3Arunner-ejected/tail?lines=20", nil)
	req.SetPathValue("sourceID", sourceID)
	recorder := httptest.NewRecorder()

	app.handleTailSystemLogs(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestRunnerSourceHintsUseRegisteredKubernetesRunnerMetadata(t *testing.T) {
	app := &App{
		cfg: &config.Config{EjectedRunnerIDs: []string{"runner-revoked"}},
		dispatcher: &fakeDispatcherClient{status: &proto.DispatcherStatus{Runners: []*proto.RunnerInfo{
			{
				RunnerId: "runner-k8s",
				Metadata: map[string]string{
					"runtime":                   "kubernetes",
					"kubernetes_namespace":      "runner-ns",
					"kubernetes_label_selector": "app.kubernetes.io/name=nopsai-k8s-runner,nopsai.io/runner-id=runner-k8s",
					"log_source_id":             systemlogs.RunnerSourceID("runner-k8s"),
					"nopsai_platform_id":        "platform-a",
				},
			},
			{
				RunnerId: "runner-revoked",
				Metadata: map[string]string{
					"runtime":                   "kubernetes",
					"kubernetes_namespace":      "runner-ns",
					"kubernetes_label_selector": "nopsai.io/runner-id=runner-revoked",
				},
			},
			{
				RunnerId: "runner-docker",
				Metadata: map[string]string{"runtime": "docker"},
			},
		}}},
	}

	hints, err := app.RunnerSourceHints(context.Background())
	if err != nil {
		t.Fatalf("RunnerSourceHints() error = %v", err)
	}
	if len(hints) != 1 {
		t.Fatalf("hints = %#v, want one Kubernetes runner hint", hints)
	}
	hint := hints[0]
	if hint.RunnerID != "runner-k8s" || hint.SourceID != systemlogs.RunnerSourceID("runner-k8s") || hint.PlatformID != "platform-a" || hint.Namespace != "runner-ns" || hint.ContainerName != "runner" {
		t.Fatalf("hint = %#v", hint)
	}
}

func TestSystemLogRateLimiterUsesSlidingWindow(t *testing.T) {
	limiter := newSystemLogRateLimiter(2, time.Minute)
	now := time.Unix(100, 0)
	if !limiter.Allow("user", now) || !limiter.Allow("user", now.Add(time.Second)) {
		t.Fatal("initial requests should be allowed")
	}
	if limiter.Allow("user", now.Add(2*time.Second)) {
		t.Fatal("third request should be limited")
	}
	if !limiter.Allow("user", now.Add(2*time.Minute)) {
		t.Fatal("request after window should be allowed")
	}
}

func TestResponseWriterWrappersExposeFlush(t *testing.T) {
	recorder := httptest.NewRecorder()
	wrapped := &loggingResponseWriter{ResponseWriter: &auditRecorder{ResponseWriter: recorder}}
	if err := http.NewResponseController(wrapped).Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if !recorder.Flushed {
		t.Fatal("underlying recorder was not flushed")
	}
}

func TestAppendSystemLogMetricsIncludesOperationalCounters(t *testing.T) {
	app := newSystemLogHandlerTestApp(&systemLogTestProvider{})
	var output strings.Builder
	app.appendSystemLogMetrics(&output)
	for _, name := range []string{
		"nopsai_system_log_streams_active",
		"nopsai_system_log_reconnects_total",
		"nopsai_system_log_redacted_lines_total",
		"nopsai_system_log_provider_errors_total",
	} {
		if !strings.Contains(output.String(), name) {
			t.Fatalf("metrics missing %s: %s", name, output.String())
		}
	}
}

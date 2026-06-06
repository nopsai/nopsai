package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nopsai/pkg/proto"
	"nopsai/pkg/serviceauth"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestQueuedJobsDispatchAfterResume(t *testing.T) {
	d := newDispatcherServer(nil, "http://example")

	rc := &runnerConn{
		connectionID:  "conn-1",
		id:            "runner-1",
		scopes:        map[string]struct{}{},
		capacity:      1,
		lastHeartbeat: time.Now(),
		inflight:      make(map[string]*proto.JobRequest),
		sendCh:        make(chan *proto.DispatcherMessage, 4),
		metadata:      map[string]string{},
		allowDispatch: false,
	}
	d.addRunner(rc)

	job := &proto.JobRequest{RunId: "run-1"}
	if _, err := d.SubmitJob(context.Background(), job); err != nil {
		t.Fatalf("submit job: %v", err)
	}

	if got := len(d.queue); got != 1 {
		t.Fatalf("expected 1 queued job, got %d", got)
	}

	if _, err := d.UpdateRunnerDispatch(context.Background(), &proto.UpdateRunnerDispatchRequest{
		RunnerId:      rc.id,
		ConnectionId:  rc.connectionID,
		AllowDispatch: true,
	}); err != nil {
		t.Fatalf("resume runner: %v", err)
	}

	select {
	case msg := <-rc.sendCh:
		if msg.GetJob() == nil {
			t.Fatalf("expected job dispatch message, got %T", msg.Message)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for dispatch after resume")
	}
}

func TestRemoveRunnerCancelsConnection(t *testing.T) {
	d := newDispatcherServer(nil, "http://example")

	cancelled := false
	rc := &runnerConn{
		connectionID:  "conn-cancel",
		id:            "runner-cancel",
		scopes:        map[string]struct{}{},
		capacity:      1,
		lastHeartbeat: time.Now(),
		inflight:      make(map[string]*proto.JobRequest),
		sendCh:        make(chan *proto.DispatcherMessage, 1),
		metadata:      map[string]string{},
		allowDispatch: true,
		cancel: func() {
			cancelled = true
		},
	}

	d.addRunner(rc)
	d.removeRunner(rc.connectionID)

	if !cancelled {
		t.Fatal("expected cancel to be invoked when runner is removed")
	}
}

func TestRunnerJobFailureFinalizesRun(t *testing.T) {
	type requestRecord struct {
		path string
		body string
	}
	seen := make(chan requestRecord, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Fatalf("missing Authorization header for %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		seen <- requestRecord{path: r.URL.Path, body: string(body)}
		switch r.URL.Path {
		case "/v1/runs/run-1/logs/ingest":
			w.WriteHeader(http.StatusNoContent)
		case "/v1/runs/run-1/finalize":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	d := newDispatcherServer(nil, server.URL, newTestDispatcherCredentials(t))
	d.nopsai.(*nopsaiHTTPClient).setHTTPClient(server.Client())
	rc := &runnerConn{
		connectionID:  "conn-failed",
		id:            "runner-failed",
		scopes:        map[string]struct{}{},
		capacity:      1,
		active:        1,
		lastHeartbeat: time.Now(),
		inflight: map[string]*proto.JobRequest{
			"run-1": {RunId: "run-1"},
		},
		sendCh:        make(chan *proto.DispatcherMessage, 1),
		metadata:      map[string]string{},
		allowDispatch: true,
	}
	d.addRunner(rc)

	d.handleJobResult(rc.connectionID, &proto.JobResult{
		RunId:  "run-1",
		Status: "failed",
		Error:  "container create: name conflict",
	})

	records := make(map[string]string)
	for len(records) < 2 {
		select {
		case record := <-seen:
			records[record.path] = record.body
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for nopsai failure reports; got %#v", records)
		}
	}
	if body := records["/v1/runs/run-1/logs/ingest"]; !strings.Contains(body, "container create: name conflict") {
		t.Fatalf("log body = %q, want failure detail", body)
	}
	finalizeBody := records["/v1/runs/run-1/finalize"]
	if !strings.Contains(finalizeBody, `"status":"failure"`) || !strings.Contains(finalizeBody, "container create: name conflict") {
		t.Fatalf("finalize body = %q, want failure status and detail", finalizeBody)
	}
	if _, ok := rc.inflight["run-1"]; ok {
		t.Fatal("run remained inflight after failed start")
	}
	if rc.active != 0 {
		t.Fatalf("runner active = %d, want 0", rc.active)
	}
}

func TestSubmitJobRejectsTerminalRun(t *testing.T) {
	server := newRunStatusServer(map[string]string{
		"run-cancelled": "cancelled",
	})
	defer server.Close()

	d := newDispatcherServer(nil, server.URL, newTestDispatcherCredentials(t))
	d.nopsai.(*nopsaiHTTPClient).setHTTPClient(server.Client())

	resp, err := d.SubmitJob(context.Background(), &proto.JobRequest{RunId: "run-cancelled"})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}
	if resp.State != proto.JobState_JOB_STATE_REJECTED {
		t.Fatalf("SubmitJob() state = %s, want %s", resp.State, proto.JobState_JOB_STATE_REJECTED)
	}
	if len(d.queue) != 0 {
		t.Fatalf("expected cancelled run to stay out of queue, found %d queued jobs", len(d.queue))
	}
}

func TestRunStatusAllowsDispatchUsesAllowList(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{status: "", want: true},
		{status: "pending", want: true},
		{status: "running", want: true},
		{status: "waiting_approval", want: false},
		{status: "rejected", want: false},
		{status: "success", want: false},
		{status: "unknown", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := runStatusAllowsDispatch(tt.status); got != tt.want {
				t.Fatalf("runStatusAllowsDispatch(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestPumpQueueDropsCancelledRunAndDispatchesRunnableJob(t *testing.T) {
	server := newRunStatusServer(map[string]string{
		"run-cancelled": "cancelled",
		"run-ready":     "running",
	})
	defer server.Close()

	d := newDispatcherServer(nil, server.URL, newTestDispatcherCredentials(t))
	d.nopsai.(*nopsaiHTTPClient).setHTTPClient(server.Client())

	rc := &runnerConn{
		connectionID:  "conn-queue",
		id:            "runner-queue",
		scopes:        map[string]struct{}{},
		capacity:      1,
		lastHeartbeat: time.Now(),
		inflight:      make(map[string]*proto.JobRequest),
		sendCh:        make(chan *proto.DispatcherMessage, 4),
		metadata:      map[string]string{},
		allowDispatch: true,
	}
	d.addRunner(rc)

	d.queue = []*proto.JobRequest{
		{RunId: "run-cancelled"},
		{RunId: "run-ready"},
	}

	d.pumpQueue()

	if got := len(d.queue); got != 0 {
		t.Fatalf("expected queue to be drained, got %d jobs", got)
	}

	select {
	case msg := <-rc.sendCh:
		job := msg.GetJob()
		if job == nil {
			t.Fatalf("expected job dispatch message, got %T", msg.Message)
		}
		if job.RunId != "run-ready" {
			t.Fatalf("expected runnable job to dispatch, got %q", job.RunId)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for queue dispatch")
	}

	if _, exists := rc.inflight["run-cancelled"]; exists {
		t.Fatal("cancelled run should not be tracked as inflight")
	}
	if _, exists := rc.inflight["run-ready"]; !exists {
		t.Fatal("runnable run should be tracked as inflight")
	}
}

func TestPrepareJobForRunnerAppliesRunnerOverridesWithoutMutatingInput(t *testing.T) {
	d := newDispatcherServer(nil, "http://example")
	job := &proto.JobRequest{
		RunId:         "run-prepare",
		Env:           []string{"KEEP=1"},
		DockerNetwork: "original-network",
	}
	runner := &runnerConn{
		id:       "runner-prepare",
		metadata: map[string]string{"docker_network": "runner-network", "dispatcher_addr": "dispatcher:7443"},
	}

	prepared := d.prepareJobForRunner(job, runner)
	if prepared == nil {
		t.Fatal("prepareJobForRunner() returned nil")
	}
	if prepared == job {
		t.Fatal("prepareJobForRunner() returned the original job pointer")
	}
	if job.DockerNetwork != "original-network" {
		t.Fatalf("original job docker network = %q, want %q", job.DockerNetwork, "original-network")
	}
	if prepared.DockerNetwork != "runner-network" {
		t.Fatalf("prepared job docker network = %q, want %q", prepared.DockerNetwork, "runner-network")
	}
	if strings.Join(job.Env, ",") != "KEEP=1" {
		t.Fatalf("original job runtime variables mutated to %v", job.Env)
	}
	gotRuntimeVars := strings.Join(prepared.Env, ",")
	for _, want := range []string{"KEEP=1", "DOCKER_NETWORK_NAME=runner-network", "DISPATCHER_ADDRESS=dispatcher:7443", "RUNNER_ID=runner-prepare"} {
		if !strings.Contains(gotRuntimeVars, want) {
			t.Fatalf("prepared runtime variables = %v, want entry %q", prepared.Env, want)
		}
	}
}

func TestPickRunnerFallsBackWhenPreferredRunnerAtCapacity(t *testing.T) {
	d := newDispatcherServer(map[string][]string{"prod": {"runner-a", "runner-b"}}, "http://example")
	runnerA := newTestRunnerConn("runner-a", "prod")
	runnerA.capacity = 1
	runnerA.active = 1
	runnerB := newTestRunnerConn("runner-b", "prod")
	runnerB.capacity = 1
	d.addRunner(runnerA)
	d.addRunner(runnerB)

	d.mu.Lock()
	got := d.pickRunnerForJobLocked(&proto.JobRequest{
		RunId:             "run-preferred-fallback",
		Scope:             "prod",
		PreferredRunnerId: "runner-a",
	})
	d.mu.Unlock()

	if got == nil || got.id != "runner-b" {
		t.Fatalf("pickRunnerForJobLocked() = %v, want runner-b", runnerIDForTest(got))
	}
}

func TestPickRunnerFallsBackWhenAffinityRunnerAtCapacity(t *testing.T) {
	d := newDispatcherServer(map[string][]string{"prod": {"runner-a", "runner-b"}}, "http://example")
	runnerA := newTestRunnerConn("runner-a", "prod")
	runnerA.capacity = 1
	runnerA.active = 1
	runnerB := newTestRunnerConn("runner-b", "prod")
	runnerB.capacity = 1
	d.addRunner(runnerA)
	d.addRunner(runnerB)
	d.triggerAssignments["event-1"] = "runner-a"

	d.mu.Lock()
	got := d.pickRunnerForJobLocked(&proto.JobRequest{
		RunId:             "run-affinity-fallback",
		Scope:             "prod",
		RunnerAffinityKey: "event-1",
	})
	assigned := d.triggerAssignments["event-1"]
	d.mu.Unlock()

	if got == nil || got.id != "runner-b" {
		t.Fatalf("pickRunnerForJobLocked() = %v, want runner-b", runnerIDForTest(got))
	}
	if assigned != "runner-b" {
		t.Fatalf("trigger assignment = %q, want runner-b", assigned)
	}
}

func TestApplyRoutingNormalizesAndClearsAssignments(t *testing.T) {
	d := newDispatcherServer(map[string][]string{"prod": {"runner-old"}}, "http://example")
	d.triggerAssignments["event-1"] = "runner-old"

	changed := d.applyRouting(map[string][]string{
		" prod ": {" runner-prod ", ""},
		"":       {" runner-default "},
	})
	if !changed {
		t.Fatal("applyRouting() changed = false, want true")
	}
	if got := d.routing["prod"]; len(got) != 1 || got[0] != "runner-prod" {
		t.Fatalf("prod routing = %#v, want runner-prod", d.routing)
	}
	if got := d.routing["*"]; len(got) != 1 || got[0] != "runner-default" {
		t.Fatalf("default routing = %#v, want runner-default", d.routing)
	}
	if len(d.triggerAssignments) != 0 {
		t.Fatalf("trigger assignments were not cleared: %#v", d.triggerAssignments)
	}

	changed = d.applyRouting(map[string][]string{
		"prod": {"runner-prod"},
		"*":    {"runner-default"},
	})
	if changed {
		t.Fatal("applyRouting() changed = true for identical routing")
	}
}

func TestSyncRoutingFromNopsaiUsesInternalEndpoint(t *testing.T) {
	tokenSeen := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/internal/dispatcher/routing" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			tokenSeen = true
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]map[string][]string{
			"dispatcher_routing": {
				"prod": {"runner-prod"},
			},
		})
	}))
	defer server.Close()

	d := newDispatcherServer(nil, server.URL, newTestDispatcherCredentials(t))
	d.nopsai.(*nopsaiHTTPClient).setHTTPClient(server.Client())

	if err := d.syncRoutingFromNopsai(context.Background()); err != nil {
		t.Fatalf("syncRoutingFromNopsai() error = %v", err)
	}
	if !tokenSeen {
		t.Fatal("expected dispatcher internal bearer token")
	}
	if got := d.routing["prod"]; len(got) != 1 || got[0] != "runner-prod" {
		t.Fatalf("routing = %#v, want prod runner", d.routing)
	}
}

func newTestRunnerConn(id string, scopes ...string) *runnerConn {
	scopeSet := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope != "" {
			scopeSet[scope] = struct{}{}
		}
	}
	return &runnerConn{
		connectionID:  "conn-" + id,
		id:            id,
		scopes:        scopeSet,
		capacity:      1,
		lastHeartbeat: time.Now(),
		inflight:      make(map[string]*proto.JobRequest),
		sendCh:        make(chan *proto.DispatcherMessage, 4),
		metadata:      map[string]string{},
		allowDispatch: true,
	}
}

func runnerIDForTest(r *runnerConn) string {
	if r == nil {
		return "<nil>"
	}
	return r.id
}

func TestPumpQueueDoesNotHoldDispatcherLockWhileFetchingRunStatus(t *testing.T) {
	requestStarted := make(chan struct{}, 1)
	releaseResponse := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestStarted <- struct{}{}
		<-releaseResponse
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "pending"})
	}))
	defer server.Close()

	d := newDispatcherServer(nil, server.URL, newTestDispatcherCredentials(t))
	d.nopsai.(*nopsaiHTTPClient).setHTTPClient(server.Client())
	d.queue = []*proto.JobRequest{{RunId: "run-blocked"}}

	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		d.pumpQueue()
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for run-status fetch to start")
	}

	addRunnerDone := make(chan struct{})
	go func() {
		d.addRunner(&runnerConn{
			connectionID:  "conn-unblocked",
			id:            "runner-unblocked",
			scopes:        map[string]struct{}{},
			capacity:      1,
			lastHeartbeat: time.Now(),
			inflight:      make(map[string]*proto.JobRequest),
			sendCh:        make(chan *proto.DispatcherMessage, 1),
			metadata:      map[string]string{},
			allowDispatch: true,
		})
		close(addRunnerDone)
	}()

	select {
	case <-addRunnerDone:
	case <-time.After(200 * time.Millisecond):
		close(releaseResponse)
		t.Fatal("dispatcher lock remained blocked during queued run status fetch")
	}

	close(releaseResponse)

	select {
	case <-pumpDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for pumpQueue to finish")
	}
}

func TestDispatcherAuthRequiresBearerToken(t *testing.T) {
	auth := newTestDispatcherAuth(t)

	_, err := auth.unaryInterceptor(context.Background(), nil, &grpc.UnaryServerInfo{
		FullMethod: "/proto.DispatcherService/GetStatus",
	}, func(ctx context.Context, req any) (any, error) {
		t.Fatal("handler should not be called without credentials")
		return nil, nil
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("status.Code(err) = %s, want %s", status.Code(err), codes.Unauthenticated)
	}
}

func TestDispatcherAuthAllowsExpectedRole(t *testing.T) {
	auth := newTestDispatcherAuth(t)
	ctx := contextWithServiceToken(t, serviceauth.RoleNopsai, "control-plane")

	called := false
	_, err := auth.unaryInterceptor(ctx, nil, &grpc.UnaryServerInfo{
		FullMethod: "/proto.DispatcherService/SubmitJob",
	}, func(ctx context.Context, req any) (any, error) {
		called = true
		claims, ok := serviceauth.ClaimsFromContext(ctx)
		if !ok {
			t.Fatal("expected service claims in handler context")
		}
		if claims.ServiceRole() != serviceauth.RoleNopsai {
			t.Fatalf("claims role = %q, want %q", claims.ServiceRole(), serviceauth.RoleNopsai)
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("unaryInterceptor() error = %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
}

func TestDispatcherAuthRejectsUnexpectedRole(t *testing.T) {
	auth := newTestDispatcherAuth(t)
	ctx := contextWithServiceToken(t, serviceauth.RoleAgent, "agent")

	_, err := auth.unaryInterceptor(ctx, nil, &grpc.UnaryServerInfo{
		FullMethod: "/proto.DispatcherService/SubmitJob",
	}, func(ctx context.Context, req any) (any, error) {
		t.Fatal("handler should not be called for wrong service role")
		return nil, nil
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("status.Code(err) = %s, want %s", status.Code(err), codes.PermissionDenied)
	}
}

func TestDispatcherAuthRejectsUnexpectedServiceID(t *testing.T) {
	auth := newTestDispatcherAuth(t)
	ctx := contextWithServiceToken(t, serviceauth.RoleAgent, "different-agent")

	_, err := auth.unaryInterceptor(ctx, nil, &grpc.UnaryServerInfo{
		FullMethod: "/proto.DispatcherService/FinalizeRun",
	}, func(ctx context.Context, req any) (any, error) {
		t.Fatal("handler should not be called for wrong service identity")
		return nil, nil
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("status.Code(err) = %s, want %s", status.Code(err), codes.PermissionDenied)
	}
}

func newTestDispatcherCredentials(t *testing.T) *serviceauth.Credentials {
	t.Helper()
	credentials, err := serviceauth.NewCredentials(serviceauth.Config{
		SigningKey: "test-service-key",
		Issuer:     serviceauth.DefaultIssuer,
		Audience:   serviceauth.DefaultAudience,
		Role:       serviceauth.RoleDispatcher,
		ServiceID:  "dispatcher",
	})
	if err != nil {
		t.Fatalf("NewCredentials() error = %v", err)
	}
	return credentials
}

func newTestDispatcherAuth(t *testing.T) *dispatcherAuth {
	t.Helper()
	authenticator, err := serviceauth.NewAuthenticator(serviceauth.Config{
		SigningKey: "test-service-key",
		Issuer:     serviceauth.DefaultIssuer,
		Audience:   serviceauth.DefaultAudience,
	})
	if err != nil {
		t.Fatalf("NewAuthenticator() error = %v", err)
	}
	return newDispatcherAuth(authenticator, map[string]string{
		serviceauth.RoleNopsai: "control-plane",
		serviceauth.RoleRunner: "runner",
		serviceauth.RoleAgent:  "agent",
	})
}

func contextWithServiceToken(t *testing.T, role, serviceID string) context.Context {
	t.Helper()
	creds, err := serviceauth.NewCredentials(serviceauth.Config{
		SigningKey: "test-service-key",
		Issuer:     serviceauth.DefaultIssuer,
		Audience:   serviceauth.DefaultAudience,
		Role:       role,
		ServiceID:  serviceID,
	})
	if err != nil {
		t.Fatalf("NewCredentials() error = %v", err)
	}
	md, err := creds.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("GetRequestMetadata() error = %v", err)
	}
	return metadata.NewIncomingContext(context.Background(), metadata.New(md))
}

func newRunStatusServer(statuses map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runID := strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, "/status"), "/v1/runs/")
		status := statuses[runID]
		if status == "" {
			status = "pending"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": status})
	}))
}

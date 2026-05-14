package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nopsai/pkg/proto"
	"nopsai/pkg/serviceauth"
	nopsaiAuth "nopsai/services/nopsai/pkg/auth"

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

func TestSubmitJobRejectsTerminalRun(t *testing.T) {
	server := newRunStatusServer(map[string]string{
		"run-cancelled": "cancelled",
	})
	defer server.Close()

	d := newDispatcherServer(nil, server.URL, newTestJWTSigner())
	d.httpClient = server.Client()

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

func TestPumpQueueDropsCancelledRunAndDispatchesRunnableJob(t *testing.T) {
	server := newRunStatusServer(map[string]string{
		"run-cancelled": "cancelled",
		"run-ready":     "running",
	})
	defer server.Close()

	d := newDispatcherServer(nil, server.URL, newTestJWTSigner())
	d.httpClient = server.Client()

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

	d := newDispatcherServer(nil, server.URL, newTestJWTSigner())
	d.httpClient = server.Client()
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

func newTestJWTSigner() *nopsaiAuth.LocalJWTService {
	return nopsaiAuth.NewLocalJWTService([]byte("test-signing-key"), "test-issuer", "test-audience", time.Minute)
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

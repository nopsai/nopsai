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

	"nopsai/pkg/correlation"
	"nopsai/pkg/proto"
	"nopsai/pkg/runmetadata"
	"nopsai/pkg/serviceauth"

	"github.com/golang-jwt/jwt/v5"
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

func TestDisconnectedRunnerRemainsRegisteredAsUnreachable(t *testing.T) {
	d := newDispatcherServer(nil, "http://example")
	rc := newTestRunnerConn("runner-offline", "prod")
	rc.lastHeartbeat = time.Unix(1_783_000_000, 0)
	d.addRunner(rc)

	d.removeRunner(rc.connectionID)

	status, err := d.GetStatus(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if len(status.GetRunners()) != 1 {
		t.Fatalf("runners len = %d, want registered runner", len(status.GetRunners()))
	}
	info := status.GetRunners()[0]
	if info.GetRunnerId() != "runner-offline" {
		t.Fatalf("runner id = %q, want runner-offline", info.GetRunnerId())
	}
	if info.GetActiveJobs() != 0 || info.GetInflightJobs() != 0 {
		t.Fatalf("disconnected runner load = active %d inflight %d, want 0/0", info.GetActiveJobs(), info.GetInflightJobs())
	}
	metadata := info.GetMetadata()
	if metadata["reachable"] != "false" || metadata["connection_status"] != "unreachable" {
		t.Fatalf("metadata = %#v, want unreachable registered runner", metadata)
	}
}

func TestEjectRunnerDeletesRegistrationAndCancelsConnection(t *testing.T) {
	d := newDispatcherServer(nil, "http://example")
	cancelled := false
	rc := newTestRunnerConn("runner-eject", "prod")
	rc.cancel = func() {
		cancelled = true
	}
	d.triggerAssignments["trigger-1"] = "runner-eject"
	d.addRunner(rc)

	_, err := d.UpdateRunnerDispatch(context.Background(), &proto.UpdateRunnerDispatchRequest{
		RunnerId:     "runner-eject",
		ConnectionId: proto.RunnerControlConnectionIDEject,
	})
	if err != nil {
		t.Fatalf("eject runner: %v", err)
	}
	if !cancelled {
		t.Fatal("expected connected runner stream cancel to be invoked")
	}

	status, err := d.GetStatus(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if len(status.GetRunners()) != 0 {
		t.Fatalf("runners len = %d, want no registered runner after eject", len(status.GetRunners()))
	}

	d.mu.Lock()
	_, connected := d.runners[rc.connectionID]
	_, registered := d.registeredRunners["runner-eject"]
	_, assigned := d.triggerAssignments["trigger-1"]
	d.mu.Unlock()
	if connected {
		t.Fatal("runner connection remained registered after eject")
	}
	if registered {
		t.Fatal("registered runner record remained after eject")
	}
	if assigned {
		t.Fatal("trigger assignment remained after runner eject")
	}
}

func TestEjectRunnerBlocksSameRunnerIDReconnect(t *testing.T) {
	d := newDispatcherServer(nil, "http://example")
	rc := newTestRunnerConn("runner-eject-blocked", "prod")
	d.addRunner(rc)

	if _, err := d.UpdateRunnerDispatch(context.Background(), &proto.UpdateRunnerDispatchRequest{
		RunnerId:     "runner-eject-blocked",
		ConnectionId: proto.RunnerControlConnectionIDEject,
	}); err != nil {
		t.Fatalf("eject runner: %v", err)
	}

	replacement := newTestRunnerConn("runner-eject-blocked", "prod")
	replacement.connectionID = "conn-replacement"
	if d.addRunner(replacement) {
		t.Fatal("addRunner() accepted a runner ID that had been ejected")
	}

	status, err := d.GetStatus(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if len(status.GetRunners()) != 0 {
		t.Fatalf("runners len = %d, want ejected runner ID blocked", len(status.GetRunners()))
	}
}

func TestAddRunnerRejectsDuplicateLiveRunnerID(t *testing.T) {
	d := newDispatcherServer(nil, "http://example")
	first := newTestRunnerConn("runner-duplicate", "prod")
	if !d.addRunner(first) {
		t.Fatal("addRunner() rejected first runner")
	}

	duplicate := newTestRunnerConn("runner-duplicate", "prod")
	duplicate.connectionID = "conn-runner-duplicate-2"
	if d.addRunner(duplicate) {
		t.Fatal("addRunner() accepted a duplicate live runner ID")
	}

	d.mu.Lock()
	_, firstConnected := d.runners[first.connectionID]
	_, duplicateConnected := d.runners[duplicate.connectionID]
	d.mu.Unlock()
	if !firstConnected {
		t.Fatal("original runner connection was removed")
	}
	if duplicateConnected {
		t.Fatal("duplicate runner connection was registered")
	}
}

func TestRegisterRejectsDuplicateLiveRunnerID(t *testing.T) {
	d := newDispatcherServer(nil, "http://example")
	first := newTestRunnerConn("runner-register-duplicate", "prod")
	if !d.addRunner(first) {
		t.Fatal("addRunner() rejected first runner")
	}

	stream := &fakeRegisterStream{
		ctx: context.Background(),
		recv: []*proto.RunnerMessage{{
			Message: &proto.RunnerMessage_Register{
				Register: &proto.RunnerRegistration{
					RunnerId: "runner-register-duplicate",
				},
			},
		}},
	}

	err := d.Register(stream)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("status.Code(err) = %s, want %s", status.Code(err), codes.PermissionDenied)
	}
	if !strings.Contains(err.Error(), "already connected") {
		t.Fatalf("Register() error = %v, want duplicate runner message", err)
	}
}

func TestEjectDisconnectedRunnerDeletesRegistration(t *testing.T) {
	d := newDispatcherServer(nil, "http://example")
	rc := newTestRunnerConn("runner-offline-eject", "prod")
	d.addRunner(rc)
	d.removeRunner(rc.connectionID)

	_, err := d.UpdateRunnerDispatch(context.Background(), &proto.UpdateRunnerDispatchRequest{
		RunnerId:     "runner-offline-eject",
		ConnectionId: proto.RunnerControlConnectionIDEject,
	})
	if err != nil {
		t.Fatalf("eject disconnected runner: %v", err)
	}

	status, err := d.GetStatus(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if len(status.GetRunners()) != 0 {
		t.Fatalf("runners len = %d, want disconnected registration removed", len(status.GetRunners()))
	}
}

func TestEjectRunnerLockedRequeuesInflightJobs(t *testing.T) {
	d := newDispatcherServer(nil, "http://example")
	rc := newTestRunnerConn("runner-eject-requeue", "prod")
	rc.active = 1
	rc.inflight["run-1"] = &proto.JobRequest{RunId: "run-1", Scope: "prod"}
	d.addRunner(rc)

	d.mu.Lock()
	_, requeuedJobs, ok := d.ejectRunnerLocked("runner-eject-requeue")
	queueLen := len(d.queue)
	d.mu.Unlock()

	if !ok {
		t.Fatal("ejectRunnerLocked() ok = false, want true")
	}
	if requeuedJobs != 1 {
		t.Fatalf("requeued jobs = %d, want 1", requeuedJobs)
	}
	if queueLen != 1 {
		t.Fatalf("queue len = %d, want inflight job requeued", queueLen)
	}
}

func TestRegisteredUnreachableRunnerDoesNotReceiveJobs(t *testing.T) {
	server := newRunStatusServer(map[string]string{"run-queued": "pending"})
	defer server.Close()

	d := newDispatcherServer(nil, server.URL, newTestDispatcherCredentials(t))
	d.nopsai.(*nopsaiHTTPClient).setHTTPClient(server.Client())
	rc := newTestRunnerConn("runner-offline", "prod")
	d.addRunner(rc)
	d.removeRunner(rc.connectionID)

	resp, err := d.SubmitJob(context.Background(), &proto.JobRequest{RunId: "run-queued", Scope: "prod"})
	if err != nil {
		t.Fatalf("SubmitJob() error = %v", err)
	}
	if resp.State != proto.JobState_JOB_STATE_QUEUED {
		t.Fatalf("SubmitJob() state = %s, want queued", resp.State)
	}
	if got := len(d.queue); got != 1 {
		t.Fatalf("queue len = %d, want job to wait for a reachable runner", got)
	}
}

func TestRunnerDispatchFlagPersistsAcrossReconnect(t *testing.T) {
	d := newDispatcherServer(nil, "http://example")
	first := newTestRunnerConn("runner-paused", "prod")
	d.addRunner(first)
	d.removeRunner(first.connectionID)

	if _, err := d.UpdateRunnerDispatch(context.Background(), &proto.UpdateRunnerDispatchRequest{
		RunnerId:      "runner-paused",
		AllowDispatch: false,
	}); err != nil {
		t.Fatalf("pause registered runner: %v", err)
	}

	second := newTestRunnerConn("runner-paused", "prod")
	second.connectionID = "conn-runner-paused-2"
	d.addRunner(second)
	if second.allowDispatch {
		t.Fatal("reconnected runner allowDispatch = true, want persisted pause")
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

func TestSubmitJobDeduplicatesQueuedRun(t *testing.T) {
	server := newRunStatusServer(map[string]string{"run-queued": "pending"})
	defer server.Close()

	d := newDispatcherServer(nil, server.URL, newTestDispatcherCredentials(t))
	d.nopsai.(*nopsaiHTTPClient).setHTTPClient(server.Client())

	first, err := d.SubmitJob(context.Background(), &proto.JobRequest{RunId: "run-queued", Scope: "prod"})
	if err != nil {
		t.Fatalf("first SubmitJob() error = %v", err)
	}
	if first.State != proto.JobState_JOB_STATE_QUEUED {
		t.Fatalf("first SubmitJob() state = %s, want queued", first.State)
	}

	second, err := d.SubmitJob(context.Background(), &proto.JobRequest{RunId: "run-queued", Scope: "prod"})
	if err != nil {
		t.Fatalf("second SubmitJob() error = %v", err)
	}
	if second.State != proto.JobState_JOB_STATE_QUEUED || second.Message != "already queued" {
		t.Fatalf("second SubmitJob() = (%s, %q), want queued/already queued", second.State, second.Message)
	}
	if got := len(d.queue); got != 1 {
		t.Fatalf("queue len = %d, want one queued job", got)
	}
}

func TestSubmitJobDeduplicatesInflightRun(t *testing.T) {
	server := newRunStatusServer(map[string]string{"run-active": "running"})
	defer server.Close()

	d := newDispatcherServer(nil, server.URL, newTestDispatcherCredentials(t))
	d.nopsai.(*nopsaiHTTPClient).setHTTPClient(server.Client())
	rc := newTestRunnerConn("runner-active", "prod")
	d.addRunner(rc)

	first, err := d.SubmitJob(context.Background(), &proto.JobRequest{RunId: "run-active", Scope: "prod"})
	if err != nil {
		t.Fatalf("first SubmitJob() error = %v", err)
	}
	if first.State != proto.JobState_JOB_STATE_ASSIGNED {
		t.Fatalf("first SubmitJob() state = %s, want assigned", first.State)
	}

	second, err := d.SubmitJob(context.Background(), &proto.JobRequest{RunId: "run-active", Scope: "prod"})
	if err != nil {
		t.Fatalf("second SubmitJob() error = %v", err)
	}
	if second.State != proto.JobState_JOB_STATE_ASSIGNED || second.RunnerId != "runner-active" || second.Message != "already dispatched" {
		t.Fatalf("second SubmitJob() = (%s, %q, %q), want assigned/runner-active/already dispatched", second.State, second.RunnerId, second.Message)
	}
	if got := len(rc.sendCh); got != 1 {
		t.Fatalf("runner send queue len = %d, want one dispatch message", got)
	}
	if rc.active != 1 {
		t.Fatalf("runner active = %d, want 1", rc.active)
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

func TestRegisterRecordsAuthenticatedRunnerServiceMetadata(t *testing.T) {
	d := newDispatcherServer(nil, "http://example")
	ctx := serviceauth.WithClaims(context.Background(), &serviceauth.Claims{
		Role: serviceauth.RoleRunner,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "runner-secure",
		},
	})
	stream := &fakeRegisterStream{
		ctx: ctx,
		recv: []*proto.RunnerMessage{{
			Message: &proto.RunnerMessage_Register{
				Register: &proto.RunnerRegistration{
					RunnerId: "runner-secure",
					Metadata: map[string]string{
						"service_id":   "spoofed",
						"service_role": "spoofed",
					},
				},
			},
		}},
	}

	err := d.Register(stream)
	if err != io.EOF {
		t.Fatalf("Register() error = %v, want EOF after test stream closes", err)
	}

	status, err := d.GetStatus(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if len(status.GetRunners()) != 1 {
		t.Fatalf("runners len = %d, want registered runner snapshot", len(status.GetRunners()))
	}
	metadata := status.GetRunners()[0].GetMetadata()
	if metadata["service_id"] != "runner-secure" || metadata["service_role"] != serviceauth.RoleRunner {
		t.Fatalf("service metadata = %#v, want authenticated runner identity", metadata)
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
	for _, want := range []string{"KEEP=1", "DISPATCHER_GRPC_ADDRESS=dispatcher:7443", "RUNNER_ID=runner-prepare"} {
		if !strings.Contains(gotRuntimeVars, want) {
			t.Fatalf("prepared runtime variables = %v, want entry %q", prepared.Env, want)
		}
	}
	if strings.Contains(gotRuntimeVars, "DOCKER_NETWORK_NAME=") {
		t.Fatalf("prepared runtime variables leaked runner network to agent env: %v", prepared.Env)
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

func TestScopedRegisteredRunnerExtendsConfiguredRouting(t *testing.T) {
	d := newDispatcherServer(map[string][]string{"prod": {"runner-static"}}, "http://example")
	runner := newTestRunnerConn("runner-dynamic", "prod")
	d.addRunner(runner)

	d.mu.Lock()
	got := d.pickRunnerForJobLocked(&proto.JobRequest{RunId: "run-prod", Scope: "prod"})
	configured := append([]string(nil), d.routing["prod"]...)
	d.mu.Unlock()

	if got == nil || got.id != "runner-dynamic" {
		t.Fatalf("pickRunnerForJobLocked() = %v, want runner-dynamic", runnerIDForTest(got))
	}
	if len(configured) != 1 || configured[0] != "runner-static" {
		t.Fatalf("configured routing mutated = %#v, want runner-static only", configured)
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
		_ = json.NewEncoder(w).Encode(map[string]any{
			"dispatcher_routing": map[string][]string{
				"prod": {"runner-prod"},
			},
			"ejected_runner_ids": []string{"runner-blocked"},
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
	if d.addRunner(newTestRunnerConn("runner-blocked", "prod")) {
		t.Fatal("synced ejected runner ID should block registration")
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

type fakeRegisterStream struct {
	grpc.ServerStream
	ctx  context.Context
	recv []*proto.RunnerMessage
	sent []*proto.DispatcherMessage
}

func (s *fakeRegisterStream) Context() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

func (s *fakeRegisterStream) Recv() (*proto.RunnerMessage, error) {
	if len(s.recv) == 0 {
		return nil, io.EOF
	}
	msg := s.recv[0]
	s.recv = s.recv[1:]
	return msg, nil
}

func (s *fakeRegisterStream) Send(msg *proto.DispatcherMessage) error {
	s.sent = append(s.sent, msg)
	return nil
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

func TestNopsaiHTTPClientIngestLogsSendsCorrelationMetadata(t *testing.T) {
	var capturedHeader string
	var capturedPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get(correlation.RequestIDHeader)
		if err := json.NewDecoder(r.Body).Decode(&capturedPayload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := newNopsaiHTTPClient(server.URL, newTestDispatcherCredentials(t))
	client.setHTTPClient(server.Client())
	ctx := correlation.WithTraceparent(correlation.WithRequestID(context.Background(), "req-ingest"), "trace-ingest")
	err := client.IngestLogs(ctx, "00000000-0000-0000-0000-000000000123", []string{"line one"}, LogIngestMetadata{
		Source:      serviceauth.RoleRunner,
		ServiceID:   "runner-1",
		ServiceRole: serviceauth.RoleRunner,
	})
	if err != nil {
		t.Fatalf("IngestLogs() error = %v", err)
	}
	if capturedHeader != "req-ingest" {
		t.Fatalf("X-Request-ID = %q, want req-ingest", capturedHeader)
	}
	if capturedPayload["source"] != serviceauth.RoleRunner || capturedPayload["service_id"] != "runner-1" {
		t.Fatalf("payload metadata = %#v, want runner source/service", capturedPayload)
	}
	if capturedPayload["request_id"] != "req-ingest" || capturedPayload["traceparent"] != "trace-ingest" {
		t.Fatalf("payload correlation = %#v, want req/trace", capturedPayload)
	}
}

func TestNopsaiHTTPClientTriggerPipelineSendsIncludeVariablesAsJSON(t *testing.T) {
	var capturedContentType string
	var capturedPayload struct {
		Definition         string            `json:"definition"`
		Variables          map[string]string `json:"variables"`
		SensitiveVariables []string          `json:"sensitive_variables"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&capturedPayload); err != nil {
			t.Fatalf("decode trigger payload: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("Pipeline run created successfully with ID: child-run-1"))
	}))
	defer server.Close()

	encoded, err := runmetadata.EncodeVariableOverrides(runmetadata.VariableOverrides{
		Variables: map[string]string{
			"CHANNEL": "stable",
			"TOKEN":   "secret",
		},
		SensitiveVariables: []string{"TOKEN"},
	})
	if err != nil {
		t.Fatalf("EncodeVariableOverrides() error = %v", err)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(runmetadata.VariableOverridesMetadataKey, encoded))

	client := newNopsaiHTTPClient(server.URL, newTestDispatcherCredentials(t))
	client.setHTTPClient(server.Client())
	resp, err := client.TriggerPipeline(ctx, &proto.TriggerPipelineRequest{
		PipelineDefinition: []byte("name: child\nsteps: []\n"),
	})
	if err != nil {
		t.Fatalf("TriggerPipeline() error = %v", err)
	}
	if resp.GetRunId() != "child-run-1" {
		t.Fatalf("run id = %q, want child-run-1", resp.GetRunId())
	}
	if capturedContentType != "application/json" {
		t.Fatalf("content type = %q, want application/json", capturedContentType)
	}
	if capturedPayload.Definition != "name: child\nsteps: []\n" {
		t.Fatalf("definition = %q, want child definition", capturedPayload.Definition)
	}
	if capturedPayload.Variables["CHANNEL"] != "stable" || capturedPayload.Variables["TOKEN"] != "secret" {
		t.Fatalf("variables = %#v, want CHANNEL and TOKEN", capturedPayload.Variables)
	}
	if len(capturedPayload.SensitiveVariables) != 1 || capturedPayload.SensitiveVariables[0] != "TOKEN" {
		t.Fatalf("sensitive variables = %#v, want TOKEN", capturedPayload.SensitiveVariables)
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

func TestDispatcherAuthAllowsRunnerScopedServiceIdentity(t *testing.T) {
	auth := newTestDispatcherAuth(t)
	ctx := contextWithServiceToken(t, serviceauth.RoleRunner, "k8s-runner-1")

	claims, err := auth.authenticate(ctx, "/proto.DispatcherService/Register")
	if err != nil {
		t.Fatalf("authenticate() runner error = %v", err)
	}
	if claims.ServiceRole() != serviceauth.RoleRunner || claims.ServiceID() != "k8s-runner-1" {
		t.Fatalf("claims = role %q service %q, want runner k8s-runner-1", claims.ServiceRole(), claims.ServiceID())
	}
}

func TestDispatcherAuthRejectsRunnerServiceIdentityWhenExpectedIDConfigured(t *testing.T) {
	authenticator := newTestServiceAuthenticator(t)
	auth := newDispatcherAuth(authenticator, map[string]string{
		serviceauth.RoleRunner: "shared-runner-service",
	})
	ctx := contextWithServiceToken(t, serviceauth.RoleRunner, "k8s-runner-1")

	_, err := auth.authenticate(ctx, "/proto.DispatcherService/Register")
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
	authenticator := newTestServiceAuthenticator(t)
	return newDispatcherAuth(authenticator, map[string]string{
		serviceauth.RoleNopsai: "control-plane",
		serviceauth.RoleRunner: "runner",
		serviceauth.RoleAgent:  "agent",
	})
}

func newTestServiceAuthenticator(t *testing.T) *serviceauth.Authenticator {
	t.Helper()
	authenticator, err := serviceauth.NewAuthenticator(serviceauth.Config{
		SigningKey: "test-service-key",
		Issuer:     serviceauth.DefaultIssuer,
		Audience:   serviceauth.DefaultAudience,
	})
	if err != nil {
		t.Fatalf("NewAuthenticator() error = %v", err)
	}
	return authenticator
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

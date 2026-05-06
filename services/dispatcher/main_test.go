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
	nopsaiAuth "nopsai/services/nopsai/pkg/auth"
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

func newTestJWTSigner() *nopsaiAuth.LocalJWTService {
	return nopsaiAuth.NewLocalJWTService([]byte("test-signing-key"), "test-issuer", "test-audience", time.Minute)
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

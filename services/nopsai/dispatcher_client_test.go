package nopsai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"nopsai/config"
	"nopsai/pkg/proto"
)

func TestFetchDispatcherStatusUsesInjectedDispatcherClient(t *testing.T) {
	dispatcher := &fakeDispatcherClient{
		status: &proto.DispatcherStatus{QueuedJobs: 7},
	}
	app := &App{dispatcher: dispatcher}

	got, err := app.fetchDispatcherStatus(context.Background())
	if err != nil {
		t.Fatalf("fetchDispatcherStatus() error = %v", err)
	}
	if got.GetQueuedJobs() != 7 {
		t.Fatalf("queued jobs = %d, want 7", got.GetQueuedJobs())
	}
	if !dispatcher.statusCalled {
		t.Fatal("dispatcher GetStatus was not called")
	}
}

func TestBuildEffectiveDispatcherRoutingIncludesRegisteredRunnerScopes(t *testing.T) {
	effective := buildEffectiveDispatcherRouting(
		map[string][]string{"prod": {"runner-static"}},
		[]dispatcherRunnerRouteInfo{
			{id: "runner-dynamic", scopes: []string{"prod", "team-1"}},
			{id: "runner-default"},
		},
	)

	if got := effective["prod"]; len(got) != 2 || got[0] != "runner-static" || got[1] != "runner-dynamic" {
		t.Fatalf("prod effective routing = %#v, want configured plus dynamic runner", got)
	}
	if got := effective["team-1"]; len(got) != 1 || got[0] != "runner-dynamic" {
		t.Fatalf("team effective routing = %#v, want dynamic runner", got)
	}
	if got := effective["*"]; len(got) != 1 || got[0] != "runner-default" {
		t.Fatalf("default effective routing = %#v, want runner-default", got)
	}
}

type fakeDispatcherClient struct {
	status           *proto.DispatcherStatus
	statusCalled     bool
	updateRunnerReq  *proto.UpdateRunnerDispatchRequest
	updateRunnerResp *proto.UpdateRunnerDispatchResponse
	updateRunnerErr  error
}

func (f *fakeDispatcherClient) SubmitJob(ctx context.Context, job *proto.JobRequest) (*proto.SubmitJobResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDispatcherClient) GetStatus(ctx context.Context) (*proto.DispatcherStatus, error) {
	f.statusCalled = true
	return f.status, nil
}

func (f *fakeDispatcherClient) UpdateRunnerDispatch(ctx context.Context, req *proto.UpdateRunnerDispatchRequest) (*proto.UpdateRunnerDispatchResponse, error) {
	f.updateRunnerReq = req
	if f.updateRunnerErr != nil {
		return nil, f.updateRunnerErr
	}
	if f.updateRunnerResp != nil {
		return f.updateRunnerResp, nil
	}
	return &proto.UpdateRunnerDispatchResponse{}, nil
}

func TestHandleEjectRunnerUsesDispatcherControlDelete(t *testing.T) {
	dispatcher := &fakeDispatcherClient{}
	app := &App{
		dispatcher: dispatcher,
		cfg: &config.Config{DispatcherRouting: map[string][]string{
			"*":    {"runner-general", "runner-prod-5"},
			"prod": {"runner-prod-5"},
			"dev":  {"runner-dev"},
		}},
	}
	req := httptest.NewRequest(http.MethodDelete, "/v1/system/dispatcher/runners/runner-prod-5", nil)
	req.SetPathValue("runnerID", "runner-prod-5")
	rec := httptest.NewRecorder()

	app.handleEjectRunner(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if dispatcher.updateRunnerReq == nil {
		t.Fatal("dispatcher UpdateRunnerDispatch was not called")
	}
	if dispatcher.updateRunnerReq.GetRunnerId() != "runner-prod-5" {
		t.Fatalf("runner_id = %q, want runner-prod-5", dispatcher.updateRunnerReq.GetRunnerId())
	}
	if dispatcher.updateRunnerReq.GetConnectionId() != proto.RunnerControlConnectionIDEject {
		t.Fatalf("connection_id = %q, want eject sentinel", dispatcher.updateRunnerReq.GetConnectionId())
	}
	if dispatcher.updateRunnerReq.GetAllowDispatch() {
		t.Fatal("allow_dispatch = true, want false for eject control")
	}
	if got := app.getConfigSnapshot().EjectedRunnerIDs; len(got) != 1 || got[0] != "runner-prod-5" {
		t.Fatalf("ejected runner ids = %#v, want runner-prod-5", got)
	}
	cfg := app.getConfigSnapshot()
	if got := cfg.DispatcherRouting["*"]; len(got) != 1 || got[0] != "runner-general" {
		t.Fatalf("default routing = %#v, want runner-general only", got)
	}
	if _, exists := cfg.DispatcherRouting["prod"]; exists {
		t.Fatalf("prod route remained after ejecting its only runner: %#v", cfg.DispatcherRouting)
	}
	if got := cfg.DispatcherRouting["dev"]; len(got) != 1 || got[0] != "runner-dev" {
		t.Fatalf("dev routing = %#v, want runner-dev unchanged", got)
	}
}

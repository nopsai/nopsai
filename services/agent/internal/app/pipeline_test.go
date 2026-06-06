package app

import (
	"context"
	"io"
	"sync"
	"testing"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	"nopsai/services/agent/internal/approval"

	"github.com/rs/zerolog"
)

func TestRunPipelineExecutesDirectScriptAndReportsSuccess(t *testing.T) {
	runtime := &fakeStepRuntime{stdout: "ok"}
	statuses := &statusRecorder{}
	finalStatuses := &finalStatusRecorder{}

	result := RunPipeline(testPipelineRunRequest(models.Pipeline{
		Name:           "pipeline",
		ContainerImage: "alpine:latest",
		Steps: []models.PipelineStep{
			{Step: &models.ScriptStep{
				BaseStep: models.BaseStep{Name: "build"},
				Script:   "echo ok",
			}},
		},
	}, runtime, statuses, finalStatuses))

	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.FinalStatus != "success" {
		t.Fatalf("FinalStatus = %q, want success", result.FinalStatus)
	}
	if result.Paused {
		t.Fatal("Paused = true, want false")
	}

	if got := finalStatuses.snapshot(); len(got) != 1 || got[0] != "success" {
		t.Fatalf("final statuses = %#v, want [success]", got)
	}
	if got := statuses.snapshot(); !sameTaskStatuses(got, []taskStatus{
		{stepName: "build", taskName: "build", status: "running"},
		{stepName: "build", taskName: "build", status: "success"},
	}) {
		t.Fatalf("task statuses = %#v, want running then success", got)
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if len(runtime.createRequests) != 1 {
		t.Fatalf("create requests = %d, want 1", len(runtime.createRequests))
	}
	createReq := runtime.createRequests[0]
	if createReq.Image != "alpine:latest" {
		t.Fatalf("create image = %q, want alpine:latest", createReq.Image)
	}
	if createReq.WorkingDirectory != models.DefaultPipelineWorkingDirectory {
		t.Fatalf("working directory = %q, want default", createReq.WorkingDirectory)
	}
	if len(runtime.actions) != 1 || runtime.actions[0].GetCommandAction().Command != "echo ok" {
		t.Fatalf("actions = %#v, want direct script command", runtime.actions)
	}
	if len(runtime.cleanupSessions) != 1 || runtime.cleanupSessions[0] != "session-build" {
		t.Fatalf("cleanup sessions = %#v, want [session-build]", runtime.cleanupSessions)
	}
}

func TestRunPipelinePausesForApprovalWithoutFinalStatus(t *testing.T) {
	runtime := &fakeStepRuntime{}
	statuses := &statusRecorder{}
	finalStatuses := &finalStatusRecorder{}
	pauser := &fakeApprovalPauser{}

	req := testPipelineRunRequest(models.Pipeline{
		Name:           "pipeline",
		ContainerImage: "alpine:latest",
		Steps: []models.PipelineStep{
			{Step: &models.ApprovalStep{
				BaseStep: models.BaseStep{Name: "approve"},
				Approval: models.ApprovalDefinition{
					Type:   "manual",
					Groups: []string{"platform"},
				},
			}},
		},
	}, runtime, statuses, finalStatuses)
	req.ApprovalPauser = pauser
	req.PipelineDefinitionYAML = []byte("name: pipeline")
	req.SharedVolumeName = "workspace"
	req.RunnerID = "runner-1"

	result := RunPipeline(req)

	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
	if !result.Paused {
		t.Fatal("Paused = false, want true")
	}
	if result.FinalStatus != "" {
		t.Fatalf("FinalStatus = %q, want empty while paused", result.FinalStatus)
	}
	if got := finalStatuses.snapshot(); len(got) != 0 {
		t.Fatalf("final statuses = %#v, want none", got)
	}
	if got := statuses.snapshot(); !sameTaskStatuses(got, []taskStatus{
		{stepName: "approve", taskName: "approve", status: "running"},
	}) {
		t.Fatalf("task statuses = %#v, want only running approval status", got)
	}

	if !pauser.called {
		t.Fatal("approval pauser was not called")
	}
	if pauser.request.PipelineDefinitionYAML != "name: pipeline" {
		t.Fatalf("approval pipeline YAML = %q, want request payload", pauser.request.PipelineDefinitionYAML)
	}
	if pauser.request.SharedVolumeName != "workspace" {
		t.Fatalf("shared volume = %q, want workspace", pauser.request.SharedVolumeName)
	}
	if pauser.request.RunnerID != "runner-1" {
		t.Fatalf("runner ID = %q, want runner-1", pauser.request.RunnerID)
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if len(runtime.createRequests) != 0 || len(runtime.actions) != 0 {
		t.Fatalf("runtime was used for approval: creates=%d actions=%d", len(runtime.createRequests), len(runtime.actions))
	}
}

func testPipelineRunRequest(pipeline models.Pipeline, runtime StepRuntime, statuses *statusRecorder, finalStatuses *finalStatusRecorder) PipelineRunRequest {
	logger := zerolog.New(io.Discard)
	return PipelineRunRequest{
		RunID:              "run-1",
		PipelineName:       pipeline.Name,
		WorkspaceDir:       models.DefaultPipelineWorkingDirectory,
		WorkingDirectory:   models.DefaultPipelineWorkingDirectory,
		Pipeline:           pipeline,
		PipelineLLMEnabled: false,
		StepRuntime:        runtime,
		Logger: func(_, _ string) *zerolog.Logger {
			return &logger
		},
		StepLogger: func(_, _, _, _ string) *zerolog.Logger {
			return &logger
		},
		UpdateTaskStatus:  statuses.report,
		NotifyFinalStatus: finalStatuses.report,
		Env: func(string) string {
			return ""
		},
		Environment: func() []string {
			return []string{"PATH=/usr/bin"}
		},
		Exit: func(int) {},
	}
}

type fakeStepRuntime struct {
	mu              sync.Mutex
	createRequests  []StepRuntimeSessionRequest
	actions         []*proto.Action
	cleanupSessions []string
	stdout          string
	stderr          string
	exitCode        int
}

func (r *fakeStepRuntime) Name() string {
	return "docker"
}

func (r *fakeStepRuntime) PrePullImages(context.Context, zerolog.Logger, *models.Pipeline, int) {}

func (r *fakeStepRuntime) CreateSession(_ context.Context, _ *zerolog.Logger, req StepRuntimeSessionRequest) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createRequests = append(r.createRequests, req)
	return "session-" + req.StepName, nil
}

func (r *fakeStepRuntime) ExecuteAction(_ context.Context, _ string, action *proto.Action, _ []string, _ string) (string, string, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions = append(r.actions, action)
	return r.stdout, r.stderr, r.exitCode
}

func (r *fakeStepRuntime) CleanupSession(_ context.Context, _ *zerolog.Logger, sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupSessions = append(r.cleanupSessions, sessionID)
}

type fakeApprovalPauser struct {
	called  bool
	request approval.Request
}

func (p *fakeApprovalPauser) Pause(_ context.Context, req approval.Request) (approval.PauseResponse, error) {
	p.called = true
	p.request = req
	return approval.PauseResponse{ApprovalID: "approval-1", CheckpointID: "checkpoint-1"}, nil
}

type taskStatus struct {
	stepName string
	taskName string
	status   string
}

type statusRecorder struct {
	mu       sync.Mutex
	statuses []taskStatus
}

func (r *statusRecorder) report(_, _, stepName, taskName, status string, _ int, _ int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses = append(r.statuses, taskStatus{stepName: stepName, taskName: taskName, status: status})
}

func (r *statusRecorder) snapshot() []taskStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]taskStatus(nil), r.statuses...)
}

type finalStatusRecorder struct {
	mu       sync.Mutex
	statuses []string
}

func (r *finalStatusRecorder) report(_, _, status string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses = append(r.statuses, status)
}

func (r *finalStatusRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.statuses...)
}

func sameTaskStatuses(got, want []taskStatus) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

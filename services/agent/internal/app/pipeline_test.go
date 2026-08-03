package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	"nopsai/services/agent/internal/approval"
	"nopsai/services/agent/internal/executor"
	includeflow "nopsai/services/agent/internal/include"
	"nopsai/services/agent/internal/resolver"

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

func TestRunPipelineHonorsStepIgnoreFailureForScriptStartupFailure(t *testing.T) {
	runtime := &fakeStepRuntime{
		stdout:       "ok",
		createErrors: map[string]error{"lint": errors.New("image pull failed")},
	}
	statuses := &statusRecorder{}
	finalStatuses := &finalStatusRecorder{}

	result := RunPipeline(testPipelineRunRequest(models.Pipeline{
		Name:           "pipeline",
		ContainerImage: "alpine:latest",
		Steps: []models.PipelineStep{
			{Step: &models.ScriptStep{
				BaseStep: models.BaseStep{
					Name:          "lint",
					IgnoreFailure: true,
				},
				Script: "npm run lint",
			}},
			{Step: &models.ScriptStep{
				BaseStep: models.BaseStep{
					Name:      "deploy",
					DependsOn: []string{"lint"},
				},
				Script: "echo deploy",
			}},
		},
	}, runtime, statuses, finalStatuses))

	if result.ExitCode != 0 || result.FinalStatus != "success" {
		t.Fatalf("result = %#v, want successful pipeline after ignored step failure", result)
	}
	if got := finalStatuses.snapshot(); len(got) != 1 || got[0] != "success" {
		t.Fatalf("final statuses = %#v, want [success]", got)
	}
	if got := statuses.snapshot(); !sameTaskStatuses(got, []taskStatus{
		{stepName: "lint", taskName: "lint", status: "running"},
		{stepName: "lint", taskName: "lint", status: "failure (ignored)"},
		{stepName: "deploy", taskName: "deploy", status: "running"},
		{stepName: "deploy", taskName: "deploy", status: "success"},
	}) {
		t.Fatalf("task statuses = %#v, want ignored lint failure then deploy success", got)
	}
}

func TestRunPipelineHonorsStepIgnoreFailureForSyncIncludeFailure(t *testing.T) {
	runtime := &fakeStepRuntime{stdout: "ok"}
	statuses := &statusRecorder{}
	finalStatuses := &finalStatusRecorder{}

	req := testPipelineRunRequest(models.Pipeline{
		Name:           "pipeline",
		ContainerImage: "alpine:latest",
		Steps: []models.PipelineStep{
			{Step: &models.IncludeStep{
				BaseStep: models.BaseStep{
					Name:          "child",
					IgnoreFailure: true,
				},
				Include: "pipeline:child",
				Sync:    true,
			}},
			{Step: &models.ScriptStep{
				BaseStep: models.BaseStep{
					Name:      "deploy",
					DependsOn: []string{"child"},
				},
				Script: "echo deploy",
			}},
		},
	}, runtime, statuses, finalStatuses)
	req.IncludeRunner = &fakeIncludeRunner{
		status:             "failure",
		exitCode:           1,
		markPipelineFailed: true,
		result:             includeflow.Result{Handled: true, Success: false, Status: "failure"},
	}

	result := RunPipeline(req)

	if result.ExitCode != 0 || result.FinalStatus != "success" {
		t.Fatalf("result = %#v, want successful pipeline after ignored include failure", result)
	}
	if got := finalStatuses.snapshot(); len(got) != 1 || got[0] != "success" {
		t.Fatalf("final statuses = %#v, want [success]", got)
	}
	if got := statuses.snapshot(); !sameTaskStatuses(got, []taskStatus{
		{stepName: "child", taskName: "child", status: "running"},
		{stepName: "child", taskName: "child", status: "failure (ignored)"},
		{stepName: "deploy", taskName: "deploy", status: "running"},
		{stepName: "deploy", taskName: "deploy", status: "success"},
	}) {
		t.Fatalf("task statuses = %#v, want ignored child failure then deploy success", got)
	}
}

func TestRunPipelineResolvesRuntimeOutputVariablesForDependentTask(t *testing.T) {
	runtime := &fakeStepRuntime{
		outputs: map[string]RuntimeOutputValue{
			"image_tag": {Name: "image_tag", Value: "v1.2.3"},
		},
	}
	statuses := &statusRecorder{}
	finalStatuses := &finalStatusRecorder{}

	result := RunPipeline(testPipelineRunRequest(models.Pipeline{
		Name:           "pipeline",
		ContainerImage: "alpine:latest",
		Steps: []models.PipelineStep{
			{Step: &models.TaskStep{
				BaseStep: models.BaseStep{Name: "prepare"},
				Tasks: []models.Task{{
					Name:    "generate",
					Script:  "printf %s v1.2.3 > /nopsai/outputs/image_tag",
					Outputs: []models.TaskOutput{{Name: "image_tag"}},
				}},
			}},
			{Step: &models.TaskStep{
				BaseStep: models.BaseStep{Name: "build"},
				Tasks: []models.Task{{
					Name:      "image",
					DependsOn: []string{"prepare.generate"},
					Variables: map[string]string{
						"IMAGE_TAG": "$steps.prepare.generate.outputs.image_tag",
					},
					Script: "echo $IMAGE_TAG",
				}},
			}},
		},
	}, runtime, statuses, finalStatuses))

	if result.ExitCode != 0 || result.FinalStatus != "success" {
		t.Fatalf("result = %#v, want successful runtime output pipeline", result)
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if len(runtime.createRequests) != 2 {
		t.Fatalf("create requests = %d, want 2", len(runtime.createRequests))
	}
	if !runtime.createRequests[0].OutputsEnabled {
		t.Fatalf("producer OutputsEnabled = false, want true")
	}
	if runtime.createRequests[1].OutputsEnabled {
		t.Fatalf("consumer OutputsEnabled = true, want false")
	}
	if len(runtime.runtimeVars) != 2 {
		t.Fatalf("runtime vars calls = %d, want 2", len(runtime.runtimeVars))
	}
	if runtimeVarsContain(runtime.runtimeVars[0], "IMAGE_TAG=v1.2.3") {
		t.Fatalf("producer runtime vars unexpectedly contain resolved output: %s", strings.Join(runtime.runtimeVars[0], "\n"))
	}
	if !runtimeVarsContain(runtime.runtimeVars[1], "IMAGE_TAG=v1.2.3") {
		t.Fatalf("consumer runtime vars missing resolved output: %s", strings.Join(runtime.runtimeVars[1], "\n"))
	}
}

func TestRunPipelineLogsNonSensitiveRuntimeValuesButMasksSensitiveValues(t *testing.T) {
	runtime := &fakeStepRuntime{
		stdout: `{"change_id":"Nothing-so-far","application":"dont ask me","image_reference":"not me","environment":"production","strategy":"rolling-update","api_token":"super-secret-token"}`,
	}
	statuses := &statusRecorder{}
	finalStatuses := &finalStatusRecorder{}
	var logOutput bytes.Buffer
	logger := zerolog.New(&logOutput)

	req := testPipelineRunRequest(models.Pipeline{
		Name:           "pipeline",
		ContainerImage: "alpine:latest",
		Steps: []models.PipelineStep{
			{Step: &models.ScriptStep{
				BaseStep: models.BaseStep{
					Name: "prepare",
					Variables: map[string]string{
						"ENVIRONMENT":      "production",
						"CHANGE_ID":        "Nothing-so-far",
						"APPLICATION_NAME": "dont ask me",
						"IMAGE_REFERENCE":  "not me",
						"API_TOKEN":        "super-secret-token",
					},
				},
				Script: "cat /nopsai/outputs/production_request_json",
			}},
		},
	}, runtime, statuses, finalStatuses)
	req.Logger = func(_, _ string) *zerolog.Logger {
		return &logger
	}
	req.StepLogger = func(_, _, _, _ string) *zerolog.Logger {
		return &logger
	}

	result := RunPipeline(req)
	if result.ExitCode != 0 || result.FinalStatus != "success" {
		t.Fatalf("result = %#v, want successful pipeline", result)
	}

	logs := logOutput.String()
	for _, want := range []string{"Nothing-so-far", "dont ask me", "not me", "production", "rolling-update"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("expected run logs to include non-sensitive value %q, got %s", want, logs)
		}
	}
	if strings.Contains(logs, "super-secret-token") {
		t.Fatalf("run logs leaked sensitive value: %s", logs)
	}
	if !strings.Contains(logs, "*****") {
		t.Fatalf("run logs did not include masking marker for sensitive value: %s", logs)
	}
}

func TestRunPipelinePassesResolvedIncludeVariablesToChildPipeline(t *testing.T) {
	runtime := &fakeStepRuntime{
		outputs: map[string]RuntimeOutputValue{
			"image_tag": {Name: "image_tag", Value: "v1.2.3"},
			"token":     {Name: "token", Value: "secret-token", Sensitive: true},
		},
	}
	statuses := &statusRecorder{}
	finalStatuses := &finalStatusRecorder{}
	includeRunner := &fakeIncludeRunner{
		status: "success",
		result: includeflow.Result{Handled: true, Success: true, Status: "success"},
	}
	req := testPipelineRunRequest(models.Pipeline{
		Name:           "pipeline",
		ContainerImage: "alpine:latest",
		Steps: []models.PipelineStep{
			{Step: &models.TaskStep{
				BaseStep: models.BaseStep{Name: "prepare"},
				Tasks: []models.Task{{
					Name: "generate",
					Outputs: []models.TaskOutput{
						{Name: "image_tag"},
						{Name: "token", Sensitive: true},
					},
					Script: "generate outputs",
				}},
			}},
			{Step: &models.IncludeStep{
				BaseStep: models.BaseStep{
					Name:      "child",
					DependsOn: []string{"prepare"},
					Variables: map[string]string{
						"CHANNEL":   "stable",
						"IMAGE_TAG": "$steps.prepare.generate.outputs.image_tag",
						"TOKEN":     "$steps.prepare.generate.outputs.token",
					},
				},
				Include: "pipeline:child",
			}},
		},
	}, runtime, statuses, finalStatuses)
	req.IncludeRunner = includeRunner

	result := RunPipeline(req)

	if result.ExitCode != 0 || result.FinalStatus != "success" {
		t.Fatalf("result = %#v, want successful include pipeline", result)
	}
	if len(includeRunner.requests) != 1 {
		t.Fatalf("include requests = %d, want 1", len(includeRunner.requests))
	}
	includeReq := includeRunner.requests[0]
	if includeReq.Variables["CHANNEL"] != "stable" || includeReq.Variables["IMAGE_TAG"] != "v1.2.3" || includeReq.Variables["TOKEN"] != "secret-token" {
		t.Fatalf("include variables = %#v, want resolved values", includeReq.Variables)
	}
	if len(includeReq.SensitiveVariables) != 1 || includeReq.SensitiveVariables[0] != "TOKEN" {
		t.Fatalf("include sensitive variables = %#v, want TOKEN", includeReq.SensitiveVariables)
	}
}

func TestRunPipelineDoesNotIgnoreBlockingConditionFailure(t *testing.T) {
	runtime := &fakeStepRuntime{stdout: "ok"}
	statuses := &statusRecorder{}
	finalStatuses := &finalStatusRecorder{}

	req := testPipelineRunRequest(models.Pipeline{
		Name:           "pipeline",
		ContainerImage: "alpine:latest",
		Steps: []models.PipelineStep{
			{Step: &models.ScriptStep{
				BaseStep: models.BaseStep{
					Name:          "release",
					Condition:     "only when the guardrail allows release",
					IgnoreFailure: true,
				},
				Script: "./release.sh",
			}},
		},
	}, runtime, statuses, finalStatuses)
	req.PipelineLLMEnabled = true
	req.ConditionClientResolver = func(*models.Pipeline, *models.PipelineStep, *models.Task) (resolver.ConditionClient, string, error) {
		return &fakeConditionClient{response: &proto.ConditionResponse{Result: false}}, "guardrail", nil
	}
	req.BlockingKnowledgeKinds = func(*models.Pipeline, *models.PipelineStep, *models.Task, []models.KnowledgeContextSnapshot) []string {
		return []string{"guardrail"}
	}

	result := RunPipeline(req)

	if result.ExitCode != 1 || result.FinalStatus != "failure" {
		t.Fatalf("result = %#v, want fail-closed pipeline", result)
	}
	if got := finalStatuses.snapshot(); len(got) != 1 || got[0] != "failure" {
		t.Fatalf("final statuses = %#v, want [failure]", got)
	}
	if got := statuses.snapshot(); !sameTaskStatuses(got, []taskStatus{
		{stepName: "release", taskName: "release", status: "failure"},
	}) {
		t.Fatalf("task statuses = %#v, want blocking condition failure", got)
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
					Type:  "manual",
					Teams: []string{"platform"},
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

func TestDetectLiveActionLogLevelNormalizesStructuredAndStreamLevels(t *testing.T) {
	cases := []struct {
		name   string
		stream executor.OutputStream
		line   string
		want   string
	}{
		{name: "structured warning", stream: executor.OutputStreamStdout, line: `{"level":"warning","message":"slow"}`, want: "warn"},
		{name: "structured trace", stream: executor.OutputStreamStdout, line: `{"output_level":"trace","message":"verbose"}`, want: "debug"},
		{name: "structured stderr error", stream: executor.OutputStreamStderr, line: `{"level":"error","message":"failed"}`, want: "error"},
		{name: "stderr fallback", stream: executor.OutputStreamStderr, line: "plain progress", want: "info"},
		{name: "stderr buildkit progress", stream: executor.OutputStreamStderr, line: "#15 sha256:abcd 14.81MB / 14.81MB 4.5s done", want: "info"},
		{name: "plain text error", stream: executor.OutputStreamStderr, line: "ERROR request failed", want: "error"},
		{name: "stdout fallback", stream: executor.OutputStreamStdout, line: "plain output", want: "info"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectLiveActionLogLevel(tc.stream, tc.line); got != tc.want {
				t.Fatalf("detectLiveActionLogLevel() = %q, want %q", got, tc.want)
			}
		})
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
	runtimeVars     [][]string
	cleanupSessions []string
	stdout          string
	stderr          string
	exitCode        int
	createErrors    map[string]error
	outputs         map[string]RuntimeOutputValue
	outputErr       error
}

func (r *fakeStepRuntime) Name() string {
	return "docker"
}

func (r *fakeStepRuntime) PrePullImages(context.Context, zerolog.Logger, *models.Pipeline, int) {}

func (r *fakeStepRuntime) CreateSession(_ context.Context, _ *zerolog.Logger, req StepRuntimeSessionRequest) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createRequests = append(r.createRequests, req)
	if err := r.createErrors[req.StepName]; err != nil {
		return "", err
	}
	return "session-" + req.StepName, nil
}

func (r *fakeStepRuntime) ExecuteAction(_ context.Context, _ string, action *proto.Action, runtimeVars []string, _ string, onLine executor.OutputLineHandler) (string, string, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions = append(r.actions, action)
	r.runtimeVars = append(r.runtimeVars, append([]string(nil), runtimeVars...))
	if onLine != nil && r.stdout != "" {
		onLine(executor.OutputStreamStdout, r.stdout)
	}
	if onLine != nil && r.stderr != "" {
		onLine(executor.OutputStreamStderr, r.stderr)
	}
	return r.stdout, r.stderr, r.exitCode
}

func runtimeVarsContain(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func (r *fakeStepRuntime) PrepareOutputDirectory(context.Context, string) error {
	return nil
}

func (r *fakeStepRuntime) CollectOutputs(context.Context, string, []models.TaskOutput, map[string]bool, int64) (map[string]RuntimeOutputValue, error) {
	if r.outputErr != nil {
		return nil, r.outputErr
	}
	if len(r.outputs) == 0 {
		return nil, nil
	}
	out := make(map[string]RuntimeOutputValue, len(r.outputs))
	for key, value := range r.outputs {
		out[key] = value
	}
	return out, nil
}

func (r *fakeStepRuntime) CleanupSession(_ context.Context, _ *zerolog.Logger, sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupSessions = append(r.cleanupSessions, sessionID)
}

type fakeIncludeRunner struct {
	status             string
	exitCode           int
	markPipelineFailed bool
	result             includeflow.Result
	requests           []includeflow.Request
}

func (r *fakeIncludeRunner) Run(_ context.Context, req includeflow.Request) includeflow.Result {
	r.requests = append(r.requests, req)
	if req.FinalizeTask != nil && r.status != "" {
		req.FinalizeTask(req.StepName, req.StepName, r.status, r.exitCode, req.LLMDurationMs)
	}
	if req.MarkPipelineFailed != nil && r.markPipelineFailed {
		req.MarkPipelineFailed(r.status)
	}
	return r.result
}

type fakeConditionClient struct {
	response *proto.ConditionResponse
	err      error
}

func (c *fakeConditionClient) EvaluateCondition(context.Context, *proto.ConditionRequest) (*proto.ConditionResponse, error) {
	return c.response, c.err
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

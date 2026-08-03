package nopsai

import (
	"context"
	"strings"
	"testing"

	"nopsai/config"
	"nopsai/pkg/models"
)

func TestAgentRunLauncherUsesInjectedLauncher(t *testing.T) {
	launcher := &recordingRunLauncher{}
	app := &App{runLauncher: launcher}

	app.agentRunLauncher().LaunchAgent(context.Background(), AgentRunLaunchRequest{
		RunID:          "run-1",
		ParentRunID:    "parent-1",
		ParentRunnerID: "runner-1",
		Pipeline: models.Pipeline{
			Name:    "deploy",
			Version: "v1",
		},
		PipelineDefinition: []byte("name: deploy"),
		GitContext:         map[string]string{"trigger_event_id": "trigger-1"},
		Scope:              "prod",
		ResumeCheckpointID: "checkpoint-1",
		ResumeVariables:    map[string]string{"ENV": "prod"},
	})

	if len(launcher.calls) != 1 {
		t.Fatalf("launcher calls = %d, want 1", len(launcher.calls))
	}
	call := launcher.calls[0]
	if call.RunID != "run-1" || call.ParentRunID != "parent-1" || call.ParentRunnerID != "runner-1" {
		t.Fatalf("launcher call ids = %#v", call)
	}
	if call.Pipeline.Name != "deploy" || string(call.PipelineDefinition) != "name: deploy" {
		t.Fatalf("launcher pipeline payload = %#v", call)
	}
	if call.GitContext["trigger_event_id"] != "trigger-1" || call.ResumeVariables["ENV"] != "prod" {
		t.Fatalf("launcher runtime context = %#v", call)
	}
}

func TestBuildAgentEnvironmentDoesNotExposeDockerNetworkName(t *testing.T) {
	env := buildAgentEnvironment(config.Config{}, agentEnvironmentInput{
		RunID:            "run-1",
		Pipeline:         models.Pipeline{Name: "deploy", Version: "v1"},
		SharedVolumeName: "vol-run-1",
		SecretsJSON:      []byte("{}"),
		VariablesJSON:    []byte("{}"),
	})

	for _, entry := range env {
		if strings.HasPrefix(entry, "DOCKER_NETWORK_NAME=") {
			t.Fatalf("agent environment leaked Docker network: %v", env)
		}
	}
}

type recordingRunLauncher struct {
	calls []AgentRunLaunchRequest
}

func (r *recordingRunLauncher) LaunchAgent(_ context.Context, req AgentRunLaunchRequest) {
	r.calls = append(r.calls, req)
}

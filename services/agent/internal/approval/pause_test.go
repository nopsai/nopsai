package approval

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
)

func TestPauserBuildsCheckpointPayload(t *testing.T) {
	var capturedRunID string
	var capturedPayload PausePayload
	pauser := NewPauser(Config{
		DefaultWorkspaceDir: "/workspace",
		CheckpointMaxBytes:  func() int64 { return 1024 },
		ArchiveWorkspace: func(root string, maxBytes int64) ([]byte, error) {
			if root != "/workspace" || maxBytes != 1024 {
				t.Fatalf("archive args = %q/%d, want /workspace/1024", root, maxBytes)
			}
			return []byte("workspace archive"), nil
		},
		RequestPause: func(_ context.Context, runID string, payload PausePayload) (PauseResponse, error) {
			capturedRunID = runID
			capturedPayload = payload
			return PauseResponse{ApprovalID: "approval-1", CheckpointID: "checkpoint-1", Status: "paused"}, nil
		},
	})

	resp, err := pauser.Pause(context.Background(), Request{
		RunID:                  "run-1",
		StepName:               "approve",
		TaskName:               "approve",
		ExecutionHistory:       "- Goal: deploy",
		CompletedTasks:         []string{"build/build"},
		PipelineDefinitionYAML: "name: release",
		Variables:              map[string]string{"ENV": "prod"},
		SharedVolumeName:       "workspace-run-1",
		RunnerID:               "runner-1",
	})
	if err != nil {
		t.Fatalf("Pause() error = %v", err)
	}
	if resp.CheckpointID != "checkpoint-1" {
		t.Fatalf("response = %#v, want checkpoint-1", resp)
	}
	if capturedRunID != "run-1" {
		t.Fatalf("captured run ID = %q, want run-1", capturedRunID)
	}
	if capturedPayload.StepName != "approve" || capturedPayload.TaskName != "approve" {
		t.Fatalf("captured step/task = %q/%q, want approve/approve", capturedPayload.StepName, capturedPayload.TaskName)
	}
	if capturedPayload.PipelineDefinitionYAML != "name: release" || capturedPayload.SharedVolumeName != "workspace-run-1" || capturedPayload.RunnerID != "runner-1" {
		t.Fatalf("captured payload = %#v", capturedPayload)
	}
	if got := capturedPayload.Variables["ENV"]; got != "prod" {
		t.Fatalf("captured variable ENV = %q, want prod", got)
	}
	archive, err := base64.StdEncoding.DecodeString(capturedPayload.WorkspaceArchiveBase64)
	if err != nil {
		t.Fatalf("decode workspace archive: %v", err)
	}
	if string(archive) != "workspace archive" {
		t.Fatalf("archive = %q, want workspace archive", archive)
	}
}

func TestPauserArchiveFailureSkipsPauseRequest(t *testing.T) {
	archiveErr := errors.New("archive failed")
	requestCalled := false
	pauser := NewPauser(Config{
		ArchiveWorkspace: func(string, int64) ([]byte, error) {
			return nil, archiveErr
		},
		RequestPause: func(context.Context, string, PausePayload) (PauseResponse, error) {
			requestCalled = true
			return PauseResponse{}, nil
		},
	})

	_, err := pauser.Pause(context.Background(), Request{RunID: "run-1", WorkspaceDir: "/workspace"})
	if err == nil {
		t.Fatal("Pause() succeeded; want archive failure")
	}
	if !errors.Is(err, archiveErr) {
		t.Fatalf("Pause() error = %v, want wrapped archive error", err)
	}
	if requestCalled {
		t.Fatal("pause request was called after archive failure")
	}
	if got := FailureLogMessage(err); got != "Failed to archive workspace for approval checkpoint" {
		t.Fatalf("failure log message = %q", got)
	}
}

func TestPauserRequestFailureUsesRequestStageMessage(t *testing.T) {
	requestErr := errors.New("request failed")
	pauser := NewPauser(Config{
		ArchiveWorkspace: func(string, int64) ([]byte, error) {
			return []byte("archive"), nil
		},
		RequestPause: func(context.Context, string, PausePayload) (PauseResponse, error) {
			return PauseResponse{}, requestErr
		},
	})

	_, err := pauser.Pause(context.Background(), Request{RunID: "run-1", WorkspaceDir: "/workspace"})
	if !errors.Is(err, requestErr) {
		t.Fatalf("Pause() error = %v, want wrapped request error", err)
	}
	if got := FailureLogMessage(err); got != "Failed to request approval pause" {
		t.Fatalf("failure log message = %q", got)
	}
}

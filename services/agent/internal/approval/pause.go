package approval

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

type failureStage string

const (
	failureArchive failureStage = "archive"
	failureRequest failureStage = "request"
)

type pauseError struct {
	stage failureStage
	err   error
}

func (e pauseError) Error() string {
	if e.err == nil {
		return "approval pause failed"
	}
	return e.err.Error()
}

func (e pauseError) Unwrap() error {
	return e.err
}

type PausePayload struct {
	StepName               string            `json:"step_name"`
	TaskName               string            `json:"task_name"`
	ExecutionHistory       string            `json:"execution_history"`
	CompletedTasks         []string          `json:"completed_tasks"`
	PipelineDefinitionYAML string            `json:"pipeline_definition_yaml"`
	Variables              map[string]string `json:"variables,omitempty"`
	WorkspaceArchiveBase64 string            `json:"workspace_archive_base64"`
	SharedVolumeName       string            `json:"shared_volume_name,omitempty"`
	RunnerID               string            `json:"runner_id,omitempty"`
}

type PauseResponse struct {
	ApprovalID   string `json:"approval_id"`
	CheckpointID string `json:"checkpoint_id"`
	Status       string `json:"status"`
}

type WorkspaceArchiver func(root string, maxBytes int64) ([]byte, error)
type PauseRequester func(ctx context.Context, runID string, req PausePayload) (PauseResponse, error)
type CheckpointLimitProvider func() int64

type Config struct {
	ArchiveWorkspace    WorkspaceArchiver
	RequestPause        PauseRequester
	CheckpointMaxBytes  CheckpointLimitProvider
	DefaultWorkspaceDir string
}

type Pauser struct {
	config Config
}

type Request struct {
	RunID                  string
	StepName               string
	TaskName               string
	ExecutionHistory       string
	CompletedTasks         []string
	PipelineDefinitionYAML string
	Variables              map[string]string
	WorkspaceDir           string
	SharedVolumeName       string
	RunnerID               string
}

func NewPauser(config Config) Pauser {
	return Pauser{config: config}
}

func (p Pauser) Pause(ctx context.Context, req Request) (PauseResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if p.config.ArchiveWorkspace == nil {
		return PauseResponse{}, pauseError{
			stage: failureArchive,
			err:   fmt.Errorf("archive workspace for approval checkpoint: workspace archiver is not configured"),
		}
	}
	if p.config.RequestPause == nil {
		return PauseResponse{}, pauseError{
			stage: failureRequest,
			err:   fmt.Errorf("request approval pause: pause requester is not configured"),
		}
	}
	maxBytes := int64(0)
	if p.config.CheckpointMaxBytes != nil {
		maxBytes = p.config.CheckpointMaxBytes()
	}

	workspaceDir := firstNonEmpty(req.WorkspaceDir, p.config.DefaultWorkspaceDir)
	workspaceArchive, err := p.config.ArchiveWorkspace(workspaceDir, maxBytes)
	if err != nil {
		return PauseResponse{}, pauseError{
			stage: failureArchive,
			err:   fmt.Errorf("archive workspace for approval checkpoint: %w", err),
		}
	}

	resp, err := p.config.RequestPause(ctx, req.RunID, PausePayload{
		StepName:               req.StepName,
		TaskName:               req.TaskName,
		ExecutionHistory:       req.ExecutionHistory,
		CompletedTasks:         req.CompletedTasks,
		PipelineDefinitionYAML: req.PipelineDefinitionYAML,
		Variables:              req.Variables,
		WorkspaceArchiveBase64: base64.StdEncoding.EncodeToString(workspaceArchive),
		SharedVolumeName:       req.SharedVolumeName,
		RunnerID:               req.RunnerID,
	})
	if err != nil {
		return PauseResponse{}, pauseError{
			stage: failureRequest,
			err:   fmt.Errorf("request approval pause: %w", err),
		}
	}
	return resp, nil
}

func FailureLogMessage(err error) string {
	var pauseErr pauseError
	if !errors.As(err, &pauseErr) {
		return "Failed to pause pipeline for approval"
	}
	switch pauseErr.stage {
	case failureArchive:
		return "Failed to archive workspace for approval checkpoint"
	case failureRequest:
		return "Failed to request approval pause"
	default:
		return "Failed to pause pipeline for approval"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

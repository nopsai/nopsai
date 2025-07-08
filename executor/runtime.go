package executor

import (
	"context"
	"io"
)

type PipelineContext struct {
	PipelineName           string
	ImageName              string
	WorkspacePath          string
	ContainerWorkspacePath string
	Environment            map[string]string
}

type StepContext struct {
	Name              string
	StepScriptContent string
}

type ExecutionResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Error    error
}

type AgentIO struct {
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser
}

// Executor defines the interface for different pipeline execution strategies.
type Executor interface {
	GetType() string
	PrepareEnvironment(ctx PipelineContext, verbose bool) error
	ExecuteStep(ctx StepContext, verbose bool) ExecutionResult
	CleanupEnvironment(verbose bool) error
}

type ContainerRuntime interface {
	ImageExists(ctx context.Context, imageName string) (bool, error)
	PullImage(ctx context.Context, imageName string) error
	CreateAndStartContainer(ctx context.Context, config ContainerConfig) (string, error)
	CopyToContainer(ctx context.Context, containerID, hostPath, containerPath string) error
	StartAgentExec(ctx context.Context, containerID string, agentPath string) (AgentIO, error)
	StopAndRemoveContainer(ctx context.Context, containerID string) error
}

type ContainerConfig struct {
	Name             string
	Image            string
	WorkspaceMount   HostMount
	AgentScriptMount HostMount
	EntrypointCmd    []string
	WorkingDir       string
	Environment      map[string]string
}

type HostMount struct {
	HostPath      string
	ContainerPath string
}

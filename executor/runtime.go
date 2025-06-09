package executor

import (
	"context"
	"io"
)

// PipelineContext holds information needed to prepare an environment.
type PipelineContext struct {
	PipelineName           string
	ImageName              string
	HostWorkspacePath      string
	ContainerWorkspacePath string
	Environment            map[string]string // Added to pass env vars
}

// ActionContext holds information for executing a single planned action.
type ActionContext struct {
	ActionName          string
	ActionScriptContent string
}

// ExecutionResult holds the outcome of a command/script execution.
type ExecutionResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Error    error
}

// AgentIO holds the streams for communicating with a running agent.
type AgentIO struct {
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser
}

// ContainerRuntime defines the contract for any container engine.
type ContainerRuntime interface {
	ImageExists(ctx context.Context, imageName string) (bool, error)
	PullImage(ctx context.Context, imageName string) error
	CreateAndStartContainer(ctx context.Context, config ContainerConfig) (string, error)
	CopyToContainer(ctx context.Context, containerID, hostPath, containerPath string) error
	StartAgentExec(ctx context.Context, containerID string, agentPath string) (AgentIO, error)
	StopAndRemoveContainer(ctx context.Context, containerID string) error
}

// ContainerConfig defines the configuration for creating a new container.
type ContainerConfig struct {
	Name             string
	Image            string
	WorkspaceMount   HostMount
	AgentScriptMount HostMount
	EntrypointCmd    []string
	WorkingDir       string
	Environment      map[string]string // Added to pass env vars to the runtime
}

// HostMount defines a volume mount from the host to the container.
type HostMount struct {
	HostPath      string
	ContainerPath string
}

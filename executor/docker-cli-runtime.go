package executor

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// DockerCLIRuntime is a concrete implementation for the Docker CLI.
type DockerCLIRuntime struct{}

func NewDockerCLIRuntime() *DockerCLIRuntime {
	return &DockerCLIRuntime{}
}

// ImageExists checks if an image is present locally using 'docker image inspect'.
func (d *DockerCLIRuntime) ImageExists(ctx context.Context, imageName string) (bool, error) {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", imageName)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			// Command ran and exited with an error code, which means the image does not exist.
			return false, nil
		}
		// A different kind of error occurred (e.g., docker daemon not running). Propagate it.
		return false, fmt.Errorf("failed to run 'docker image inspect' for '%s': %w", imageName, err)
	}
	// Command succeeded, so the image exists.
	return true, nil
}

func (d *DockerCLIRuntime) PullImage(ctx context.Context, imageName string) error {
	cmd := exec.CommandContext(ctx, "docker", "pull", imageName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to pull image '%s': %w. output: %s", imageName, err, string(output))
	}
	return nil
}

func (d *DockerCLIRuntime) CreateAndStartContainer(ctx context.Context, config ContainerConfig) (string, error) {
	args := []string{
		"run", "-d",
		"--name", config.Name,
		// Mount the project workspace
		"-v", fmt.Sprintf("%s:%s", config.WorkspaceMount.HostPath, config.WorkspaceMount.ContainerPath),
		// Mount the agent/scripts directory
		"-v", fmt.Sprintf("%s:%s", config.AgentScriptMount.HostPath, config.AgentScriptMount.ContainerPath),
		// Set the working directory
		"-w", config.WorkingDir,
	}

	// Add environment variables passed from the pipeline config
	if config.Environment != nil {
		for key, value := range config.Environment {
			args = append(args, "--env", fmt.Sprintf("%s=%s", key, value))
		}
	}

	// The logic for conditionally mounting the Docker socket has been removed.
	// The container will rely on the DOCKER_HOST environment variable if set.

	args = append(args, config.Image)
	args = append(args, config.EntrypointCmd...)

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to start container: %w. output: %s", err, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func (d *DockerCLIRuntime) CopyToContainer(ctx context.Context, containerID, hostPath, containerPath string) error {
	cmd := exec.CommandContext(ctx, "docker", "cp", hostPath, fmt.Sprintf("%s:%s", containerID, containerPath))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to copy file to container: %w. output: %s", err, string(output))
	}
	return nil
}

func (d *DockerCLIRuntime) StartAgentExec(ctx context.Context, containerID string, agentPath string) (AgentIO, error) {
	// Execute the agent binary directly
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerID, agentPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return AgentIO{}, fmt.Errorf("failed to get stdin pipe for agent: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return AgentIO{}, fmt.Errorf("failed to get stdout pipe for agent: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return AgentIO{}, fmt.Errorf("failed to get stderr pipe for agent: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return AgentIO{}, fmt.Errorf("failed to start agent exec process: %w", err)
	}

	return AgentIO{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}, nil
}

func (d *DockerCLIRuntime) StopAndRemoveContainer(ctx context.Context, containerID string) error {
	stopCmd := exec.CommandContext(ctx, "docker", "stop", containerID)
	if err := stopCmd.Run(); err != nil {
		fmt.Printf("warning: failed to stop container %s: %v\n", containerID, err)
	}

	rmCmd := exec.CommandContext(ctx, "docker", "rm", containerID)
	if err := rmCmd.Run(); err != nil {
		return fmt.Errorf("failed to remove container %s: %w", containerID, err)
	}
	return nil
}

package executor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"nopsai/sharedtypes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type DockerExecutor struct {
	runtime            ContainerRuntime
	containerID        string
	hostAgentDir       string
	containerAgentPath string
	agentIO            AgentIO
	agentResultScanner *bufio.Scanner
	verbose            bool
}

func NewDockerExecutor(runtime ContainerRuntime) *DockerExecutor {
	return &DockerExecutor{
		runtime:            runtime,
		containerAgentPath: "/nopsai_agent/nopsai-agent",
	}
}

func (de *DockerExecutor) GetType() string {
	return "docker"
}

func (de *DockerExecutor) PrepareEnvironment(ctx PipelineContext, verbose bool) error {
	de.verbose = verbose
	if ctx.ImageName == "" {
		return fmt.Errorf("docker executor: image name is required")
	}

	exists, err := de.runtime.ImageExists(context.Background(), ctx.ImageName)
	if err != nil {
		return fmt.Errorf("could not check for image existence: %w", err)
	}
	if !exists {
		if verbose {
			log.Printf("image '%s' not found locally, pulling...", ctx.ImageName)
		}
		if err := de.runtime.PullImage(context.Background(), ctx.ImageName); err != nil {
			return fmt.Errorf("failed to pull image '%s': %w", ctx.ImageName, err)
		}
	} else {
		if verbose {
			log.Printf("image '%s' found locally, skipping pull.", ctx.ImageName)
		}
	}

	hostAgentDir, err := os.MkdirTemp("", "nopsai_agent_*")
	if err != nil {
		return fmt.Errorf("failed to create temp agent directory: %w", err)
	}
	de.hostAgentDir = hostAgentDir

	hostWorkspace, err := filepath.Abs(ctx.HostWorkspacePath)
	if err != nil {
		return fmt.Errorf("failed to resolve host workspace path: %w", err)
	}
	if _, err := os.Stat(hostWorkspace); os.IsNotExist(err) {
		if err := os.MkdirAll(hostWorkspace, 0755); err != nil {
			return fmt.Errorf("failed to create host workspace path '%s': %w", hostWorkspace, err)
		}
	}

	containerWorkspace := ctx.ContainerWorkspacePath
	if containerWorkspace == "" {
		containerWorkspace = "/workspace"
	}

	containerCfg := ContainerConfig{
		Name:             fmt.Sprintf("nopsai_exec_%s_%d", strings.ReplaceAll(ctx.PipelineName, " ", "_"), time.Now().UnixNano()),
		Image:            ctx.ImageName,
		WorkspaceMount:   HostMount{HostPath: hostWorkspace, ContainerPath: containerWorkspace},
		AgentScriptMount: HostMount{HostPath: de.hostAgentDir, ContainerPath: "/nopsai_agent"},
		WorkingDir:       containerWorkspace,
		EntrypointCmd:    []string{"tail", "-f", "/dev/null"},
		Environment:      ctx.Environment,
	}

	if verbose {
		log.Println("creating and starting container...")
	}
	containerID, err := de.runtime.CreateAndStartContainer(context.Background(), containerCfg)
	if err != nil {
		os.RemoveAll(de.hostAgentDir)
		return err
	}
	de.containerID = containerID
	if verbose {
		log.Printf("container '%s' started.", de.containerID)
	}

	agentHostPath := "./agent/nopsai-agent"
	if err := de.runtime.CopyToContainer(context.Background(), de.containerID, agentHostPath, de.containerAgentPath); err != nil {
		de.CleanupEnvironment(verbose)
		return fmt.Errorf("failed to copy agent to container: %w", err)
	}

	_, err = exec.Command("docker", "exec", de.containerID, "chmod", "+x", de.containerAgentPath).CombinedOutput()
	if err != nil {
		de.CleanupEnvironment(verbose)
		return fmt.Errorf("failed to make agent executable in container: %w", err)
	}

	if verbose {
		log.Println("starting agent process inside container...")
	}
	agentIO, err := de.runtime.StartAgentExec(context.Background(), de.containerID, de.containerAgentPath)
	if err != nil {
		de.CleanupEnvironment(verbose)
		return fmt.Errorf("failed to start agent process: %w", err)
	}
	de.agentIO = agentIO
	de.agentResultScanner = bufio.NewScanner(de.agentIO.Stdout)

	go func() {
		scanner := bufio.NewScanner(de.agentIO.Stderr)
		for scanner.Scan() {
			log.Printf("AGENT_STDERR: %s", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			log.Printf("warning: error reading from agent stderr: %v", err)
		}
	}()

	return nil
}

func (de *DockerExecutor) ExecuteStep(ctx StepContext, verbose bool) ExecutionResult {
	if de.containerID == "" {
		return ExecutionResult{Error: fmt.Errorf("docker executor: environment not prepared")}
	}

	agentCmd := sharedtypes.AgentCommand{
		StepName: ctx.Name,
		Script:   ctx.StepScriptContent,
	}

	cmdBytes, err := json.Marshal(agentCmd)
	if err != nil {
		return ExecutionResult{Error: fmt.Errorf("failed to marshal agent command: %w", err)}
	}

	if _, err := fmt.Fprintln(de.agentIO.Stdin, string(cmdBytes)); err != nil {
		return ExecutionResult{Error: fmt.Errorf("failed to send command to agent: %w", err)}
	}

	if de.agentResultScanner.Scan() {
		line := de.agentResultScanner.Text()
		var agentResult sharedtypes.AgentExecResult
		if err := json.Unmarshal([]byte(line), &agentResult); err != nil {
			return ExecutionResult{Error: fmt.Errorf("failed to unmarshal result from agent: %w. raw line: %s", err, line)}
		}

		return ExecutionResult{
			Stdout:   agentResult.Stdout,
			Stderr:   agentResult.Stderr,
			ExitCode: agentResult.ExitCode,
			Error:    nil,
		}
	}

	if err := de.agentResultScanner.Err(); err != nil {
		return ExecutionResult{Error: fmt.Errorf("error reading from agent stdout: %w", err)}
	}

	return ExecutionResult{Error: fmt.Errorf("did not receive result from agent; agent may have terminated unexpectedly")}
}

func (de *DockerExecutor) CleanupEnvironment(verbose bool) error {
	if de.agentIO.Stdin != nil {
		de.agentIO.Stdin.Close()
	}

	if de.containerID != "" {
		if err := de.runtime.StopAndRemoveContainer(context.Background(), de.containerID); err != nil {
			log.Printf("warning: error during container cleanup: %v", err)
		}
	}

	if de.hostAgentDir != "" {
		if err := os.RemoveAll(de.hostAgentDir); err != nil {
			return fmt.Errorf("failed to remove host agent directory %s: %w", de.hostAgentDir, err)
		}
	}
	return nil
}

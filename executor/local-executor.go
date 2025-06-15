package executor

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type LocalExecutorT struct {
	hostWorkspacePath string
	environment       map[string]string
	verbose           bool
}

func LocalExecutor() *LocalExecutorT {
	return &LocalExecutorT{}
}

func (le *LocalExecutorT) GetType() string {
	return "local"
}

func (le *LocalExecutorT) PrepareEnvironment(ctx PipelineContext, verbose bool) error {
	le.verbose = verbose
	le.environment = ctx.Environment

	if ctx.HostWorkspacePath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("local executor: HostWorkspacePath is empty and failed to get current working directory: %w", err)
		}
		le.hostWorkspacePath = cwd
		if le.verbose {
			log.Printf("LocalExecutor: HostWorkspacePath not set, using current directory: %s", le.hostWorkspacePath)
		}
	} else {
		absPath, err := filepath.Abs(ctx.HostWorkspacePath)
		if err != nil {
			return fmt.Errorf("local executor: failed to get absolute path for HostWorkspacePath '%s': %w", ctx.HostWorkspacePath, err)
		}
		le.hostWorkspacePath = absPath
	}

	if le.verbose {
		log.Printf("LocalExecutor: Ensuring workspace directory exists: %s", le.hostWorkspacePath)
	}
	if err := os.MkdirAll(le.hostWorkspacePath, 0755); err != nil {
		return fmt.Errorf("local executor: failed to create workspace directory '%s': %w", le.hostWorkspacePath, err)
	}
	if le.verbose {
		log.Printf("LocalExecutor: Environment prepared. Workspace: %s", le.hostWorkspacePath)
	}
	return nil
}

func (le *LocalExecutorT) ExecuteStep(ctx StepContext, verbose bool) ExecutionResult {
	if le.hostWorkspacePath == "" {
		return ExecutionResult{Error: fmt.Errorf("local executor: host workspace path not set, PrepareEnvironment may not have been called")}
	}

	scriptFileNamePattern := fmt.Sprintf("nopsai_step_%s_*.sh", strings.ReplaceAll(ctx.Name, " ", "_"))
	tempScriptFile, err := os.CreateTemp(le.hostWorkspacePath, scriptFileNamePattern)
	if err != nil {
		return ExecutionResult{Error: fmt.Errorf("local executor: failed to create temporary script file in '%s': %w", le.hostWorkspacePath, err)}
	}
	defer os.Remove(tempScriptFile.Name())

	scriptContent := ctx.StepScriptContent
	if !strings.HasPrefix(scriptContent, "#!") {
		scriptContent = fmt.Sprintf("#!/bin/bash\nset -e\n%s", scriptContent)
	}

	if _, err := tempScriptFile.WriteString(scriptContent); err != nil {
		tempScriptFile.Close()
		return ExecutionResult{Error: fmt.Errorf("local executor: failed to write to temporary script file '%s': %w", tempScriptFile.Name(), err)}
	}
	if err := tempScriptFile.Close(); err != nil {
		return ExecutionResult{Error: fmt.Errorf("local executor: failed to close temporary script file '%s': %w", tempScriptFile.Name(), err)}
	}

	if err := os.Chmod(tempScriptFile.Name(), 0755); err != nil {
		return ExecutionResult{Error: fmt.Errorf("local executor: failed to make temporary script '%s' executable: %w", tempScriptFile.Name(), err)}
	}

	if le.verbose {
		log.Printf("LocalExecutor: Executing script for step '%s': %s in dir '%s'", ctx.Name, tempScriptFile.Name(), le.hostWorkspacePath)
		log.Printf("LocalExecutor: Final script content for step '%s':\n---\n%s\n---", ctx.Name, scriptContent)
	}

	cmd := exec.Command(tempScriptFile.Name())
	cmd.Dir = le.hostWorkspacePath

	cmdEnv := os.Environ()
	if le.environment != nil {
		for key, value := range le.environment {
			cmdEnv = append(cmdEnv, fmt.Sprintf("%s=%s", key, value))
		}
	}
	cmd.Env = cmdEnv

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	if le.verbose {
		log.Printf("LocalExecutor: Step '%s' completed. ExitCode: %d", ctx.Name, exitCode)
		if stdout.Len() > 0 {
			log.Printf("LocalExecutor: Step '%s' STDOUT:\n%s", ctx.Name, stdout.String())
		}
		if stderr.Len() > 0 {
			log.Printf("LocalExecutor: Step '%s' STDERR:\n%s", ctx.Name, stderr.String())
		}
	}

	return ExecutionResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Error:    runErr,
	}
}

func (le *LocalExecutorT) CleanupEnvironment(verbose bool) error {
	if le.verbose {
		log.Printf("LocalExecutor: Cleaning up environment. Workspace was: %s", le.hostWorkspacePath)
	}
	return nil
}

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
	WorkspacePath string
	environment   map[string]string
	verbose       bool
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

	if ctx.WorkspacePath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("local executor: WorkspacePath is empty and failed to get current working directory: %w", err)
		}
		le.WorkspacePath = cwd
		if le.verbose {
			log.Printf("LocalExecutor: WorkspacePath not set, using current directory: %s", le.WorkspacePath)
		}
	} else {
		absPath, err := filepath.Abs(ctx.WorkspacePath)
		if err != nil {
			return fmt.Errorf("local executor: failed to get absolute path for WorkspacePath '%s': %w", ctx.WorkspacePath, err)
		}
		le.WorkspacePath = absPath
	}

	if le.verbose {
		log.Printf("LocalExecutor: Ensuring workspace directory exists: %s", le.WorkspacePath)
	}
	if err := os.MkdirAll(le.WorkspacePath, 0755); err != nil {
		return fmt.Errorf("local executor: failed to create workspace directory '%s': %w", le.WorkspacePath, err)
	}

	// Ensure the outputs file exists
	outputsFile := filepath.Join(le.WorkspacePath, "nopsai_outputs.env")
	if _, err := os.Stat(outputsFile); os.IsNotExist(err) {
		if err := os.WriteFile(outputsFile, []byte{}, 0644); err != nil {
			return fmt.Errorf("local executor: failed to create outputs file '%s': %w", outputsFile, err)
		}
	}

	if le.verbose {
		log.Printf("LocalExecutor: Environment prepared. Workspace: %s", le.WorkspacePath)
	}
	return nil
}

func (le *LocalExecutorT) ExecuteStep(ctx StepContext, verbose bool) ExecutionResult {
	if le.WorkspacePath == "" {
		return ExecutionResult{Error: fmt.Errorf("local executor: host workspace path not set, PrepareEnvironment may not have been called")}
	}

	scriptFileNamePattern := fmt.Sprintf("nopsai_step_%s_*.sh", strings.ReplaceAll(ctx.Name, " ", "_"))
	tempScriptFile, err := os.CreateTemp(le.WorkspacePath, scriptFileNamePattern)
	if err != nil {
		return ExecutionResult{Error: fmt.Errorf("local executor: failed to create temporary script file in '%s': %w", le.WorkspacePath, err)}
	}
	defer os.Remove(tempScriptFile.Name())

	outputsFile := filepath.Join(le.WorkspacePath, "nopsai_outputs.env")

	// Create the wrapper script content
	wrapperScript := fmt.Sprintf(`#!/bin/bash
set -eo pipefail

export WORKSPACE="%s"
source "%s"

%s
`, le.WorkspacePath, outputsFile, ctx.StepScriptContent)

	if _, err := tempScriptFile.WriteString(wrapperScript); err != nil {
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
		log.Printf("LocalExecutor: Executing script for step '%s': %s in dir '%s'", ctx.Name, tempScriptFile.Name(), le.WorkspacePath)
		log.Printf("LocalExecutor: Final script content for step '%s':\n---\n%s\n---", ctx.Name, wrapperScript)
	}

	cmd := exec.Command(tempScriptFile.Name())
	cmd.Dir = le.WorkspacePath

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
		log.Printf("LocalExecutor: Cleaning up environment. Workspace was: %s", le.WorkspacePath)
	}
	return nil
}

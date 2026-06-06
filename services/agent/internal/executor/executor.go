package executor

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"path"
	"strings"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"
)

type PreparedAction struct {
	Command    string
	Stdout     string
	ReturnOnly bool
}

func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func ResolveActionFilePath(workingDirectory, actionPath string) (string, error) {
	resolvedWorkingDirectory, err := models.NormalizePipelineWorkingDirectory(workingDirectory)
	if err != nil {
		return "", err
	}

	trimmed := strings.TrimSpace(actionPath)
	if trimmed == "" {
		return "", fmt.Errorf("file action path cannot be empty")
	}

	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	cleaned := strings.TrimPrefix(path.Clean(normalized), "/")
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("file action path cannot be empty")
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("file action path cannot escape working directory")
	}

	return path.Join(resolvedWorkingDirectory, cleaned), nil
}

func PrepareAction(action *proto.Action, workingDirectory string) (PreparedAction, error) {
	switch action.Type {
	case "EXECUTE_COMMAND":
		return PreparedAction{Command: action.GetCommandAction().Command}, nil
	case "REPLACE_FILE":
		content := action.GetFileAction().Content
		encodedContent := base64.StdEncoding.EncodeToString([]byte(content))
		filePath, err := ResolveActionFilePath(workingDirectory, action.GetFileAction().Path)
		if err != nil {
			return PreparedAction{}, err
		}
		return PreparedAction{
			Command: fmt.Sprintf("printf %%s %s | base64 -d > %s", ShellQuote(encodedContent), ShellQuote(filePath)),
		}, nil
	case "RETURN_ANSWER":
		ansAction := action.GetAnswerAction()
		if ansAction == nil {
			return PreparedAction{}, fmt.Errorf("Invalid answer action payload")
		}
		return PreparedAction{Stdout: ansAction.Answer, ReturnOnly: true}, nil
	default:
		return PreparedAction{}, fmt.Errorf("Unknown action type")
	}
}

func ExecuteDockerAction(ctx context.Context, cli *client.Client, containerID string, action *proto.Action, runtimeVars []string, workingDirectory string) (string, string, int) {
	resolvedWorkingDirectory, err := models.NormalizePipelineWorkingDirectory(workingDirectory)
	if err != nil {
		return "", err.Error(), 1
	}

	prepared, err := PrepareAction(action, resolvedWorkingDirectory)
	if err != nil {
		return "", err.Error(), 1
	}
	if prepared.ReturnOnly {
		return prepared.Stdout, "", 0
	}

	execConfig := client.ExecCreateOptions{
		Cmd:          []string{"sh", "-c", prepared.Command},
		Env:          runtimeVars,
		WorkingDir:   resolvedWorkingDirectory,
		AttachStdout: true,
		AttachStderr: true,
		TTY:          false,
	}

	execID, err := cli.ExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Sprintf("failed to create exec: %v", err), 1
	}

	resp, err := cli.ExecAttach(ctx, execID.ID, client.ExecAttachOptions{})
	if err != nil {
		return "", fmt.Sprintf("failed to attach to exec: %v", err), 1
	}
	defer resp.Close()

	var stdout, stderr bytes.Buffer
	_, err = stdcopy.StdCopy(&stdout, &stderr, resp.Reader)
	if err != nil {
		return "", fmt.Sprintf("failed to read output: %v", err), 1
	}

	inspect, err := cli.ExecInspect(ctx, execID.ID, client.ExecInspectOptions{})
	if err != nil {
		return "", fmt.Sprintf("failed to inspect exec: %v", err), 1
	}

	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), inspect.ExitCode
}

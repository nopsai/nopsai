package models

import (
	"fmt"
	"path"
	"strings"
)

const DefaultPipelineWorkingDirectory = "/workspace"

func NormalizePipelineWorkingDirectory(workingDirectory string) (string, error) {
	trimmed := strings.TrimSpace(workingDirectory)
	if trimmed == "" || trimmed == "." {
		return DefaultPipelineWorkingDirectory, nil
	}

	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	if strings.ContainsRune(normalized, '\x00') {
		return "", fmt.Errorf("working_directory cannot contain NUL bytes")
	}

	cleaned := path.Clean(normalized)
	if cleaned == "." {
		return DefaultPipelineWorkingDirectory, nil
	}
	if strings.Contains(cleaned, ":") {
		return "", fmt.Errorf("working_directory cannot contain ':'")
	}

	if !strings.HasPrefix(cleaned, "/") {
		if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			return "", fmt.Errorf("relative working_directory cannot escape %s", DefaultPipelineWorkingDirectory)
		}
		cleaned = path.Join(DefaultPipelineWorkingDirectory, cleaned)
	}

	if cleaned == "/" {
		return "", fmt.Errorf("working_directory cannot be the container root")
	}

	return cleaned, nil
}

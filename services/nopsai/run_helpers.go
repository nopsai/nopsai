package nopsai

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)
var envKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func defaultPolicyName(obj, act string) string {
	cleanedObj := strings.Trim(strings.TrimSpace(obj), "/")
	base := cleanedObj
	if base == "" {
		base = "policy"
	} else {
		parts := strings.Split(base, "/")
		base = parts[len(parts)-1]
		if base == "" {
			base = cleanedObj
		}
	}
	action := strings.TrimSpace(act)
	if action == "" {
		action = "ANY"
	}
	return fmt.Sprintf("%s • %s", base, action)
}

func deriveTriggerEventID(gitContext map[string]string) string {
	if gitContext == nil {
		return ""
	}
	owner := strings.ToLower(strings.TrimSpace(gitContext["repo_owner"]))
	name := strings.ToLower(strings.TrimSpace(gitContext["repo_name"]))
	ref := strings.ToLower(strings.TrimSpace(gitContext["ref"]))
	sha := strings.ToLower(strings.TrimSpace(gitContext["commit_sha"]))
	if owner == "" && name == "" && ref == "" && sha == "" {
		return ""
	}
	return fmt.Sprintf("%s|%s|%s|%s", owner, name, ref, sha)
}

func (a *App) getRunListItem(runID string) (*RunListItem, error) {
	return a.store.GetRunListItem(context.Background(), runID)
}

func matchBranchPattern(pattern, name string) bool {
	if pattern == "" {
		return false
	}
	var builder strings.Builder
	builder.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				builder.WriteString(".*")
				i++
			} else {
				builder.WriteString("[^/]*")
			}
		case '?':
			builder.WriteString(".")
		case '.', '(', ')', '+', '|', '^', '$', '{', '}', '[', ']', '\\':
			builder.WriteByte('\\')
			builder.WriteByte(pattern[i])
		default:
			builder.WriteByte(pattern[i])
		}
	}
	builder.WriteString("$")
	re, err := regexp.Compile(builder.String())
	if err != nil {
		return pattern == name
	}
	return re.MatchString(name)
}

var (
	errManifestNotFound = errors.New("manifest not found")
	errPipelineNotFound = errors.New("pipeline not found")
)

func sanitizeInput(name string) string {
	sanitized := strings.ReplaceAll(name, " ", "-")
	return nonAlphanumericRegex.ReplaceAllString(sanitized, "")
}

func isTerminalRunStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "warning", "failure", "cancelled", "timed_out", "failure (ignored)", "rejected":
		return true
	default:
		return false
	}
}

func isCompletedRunStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "warning", "failure", "timed_out", "rejected":
		return true
	default:
		return false
	}
}

func normalizeFinalizeRunStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success":
		return "success"
	case "warning":
		return "warning"
	case "cancelled":
		return "cancelled"
	default:
		return "failure"
	}
}

func buildAgentContainerName(pipelineName, repoName, triggerEventID, runID string) string {
	sanitizedPipelineName := sanitizeInput(pipelineName)
	sanitizedTriggerID := sanitizeInput(strings.TrimSpace(triggerEventID))
	if sanitizedTriggerID == "" {
		sanitizedTriggerID = "no-trigger"
	} else if len(sanitizedTriggerID) > 8 {
		sanitizedTriggerID = sanitizedTriggerID[:8]
	}

	shortRunID := runID
	if len(shortRunID) > 8 {
		shortRunID = shortRunID[:8]
	}

	if strings.TrimSpace(repoName) != "" {
		sanitizedRepoName := sanitizeInput(repoName)
		return fmt.Sprintf("agent-%s-%s-%s-%s", sanitizedRepoName, sanitizedPipelineName, sanitizedTriggerID, shortRunID)
	}

	return fmt.Sprintf("agent-%s-%s-%s", sanitizedPipelineName, sanitizedTriggerID, shortRunID)
}

func buildLaunchAgentContainerName(pipelineName, repoName, triggerEventID, runID string) string {
	baseName := buildAgentContainerName(pipelineName, repoName, triggerEventID, runID)
	launchID := strings.ReplaceAll(uuid.NewString(), "-", "")
	if len(launchID) > 8 {
		launchID = launchID[:8]
	}
	suffix := "-" + launchID
	if maxBaseLen := dockerContainerNameMax - len(suffix); len(baseName) > maxBaseLen {
		baseName = baseName[:maxBaseLen]
	}
	return baseName + suffix
}

func normalizePipelineVersion(version string) string {
	sanitized := sanitizeInput(version)
	if sanitized == "" {
		return "latest"
	}
	return sanitized
}

package agent

import (
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func agentLog(runID, pipeline string) *zerolog.Logger {
	logger := log.With().
		Str("run_id", runID).
		Str("pipeline", pipeline).
		Str("component", "agent").
		Logger()
	return &logger
}

func splitPipelineIdentifier(identifier string) (string, string) {
	trimmed := strings.TrimSpace(identifier)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "", ""
	}

	normalized := filepath.ToSlash(trimmed)
	lower := strings.ToLower(normalized)
	if strings.HasSuffix(lower, ".yaml") {
		normalized = normalized[:len(normalized)-len(".yaml")]
	} else if strings.HasSuffix(lower, ".yml") {
		normalized = normalized[:len(normalized)-len(".yml")]
	}

	parts := strings.Split(normalized, "/")
	name := parts[len(parts)-1]
	var path string
	if len(parts) > 1 {
		path = strings.Join(parts[:len(parts)-1], "/")
	}
	return path, name
}

func stepLog(runID, pipeline, step, task string) *zerolog.Logger {
	logger := log.With().
		Str("run_id", runID).
		Str("pipeline", pipeline).
		Str("step", step)
	if task != "" {
		logger = logger.Str("task", task)
	}
	result := logger.Logger()
	return &result
}

package runnerinstall

import (
	"strings"

	"nopsai/config"
)

func ShellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func shellDoubleQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, `$`, `\$`)
	value = strings.ReplaceAll(value, "`", "\\`")
	return value
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func runnerServiceIDForInstall(cfg config.Config, runnerID string) string {
	if strings.TrimSpace(cfg.RunnerServiceID) != "" {
		return cfg.EffectiveRunnerServiceID()
	}
	return firstNonEmptyString(runnerID, cfg.EffectiveRunnerServiceID())
}

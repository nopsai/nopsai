package command

import (
	"fmt"
	"strings"

	"nopsai/internal/cli/interactive"
)

func sessionHeaderLines(state homeState) []string {
	contextLabel := valueOrDefault(state.ContextName, "not selected")
	if state.ContextCount > 0 {
		contextLabel = fmt.Sprintf("%s (%d configured)", contextLabel, state.ContextCount)
	}
	header := []string{
		fmt.Sprintf("Version: %s | Context: %s | API: %s", valueOrDefault(state.Version, "dev"), contextLabel, valueOrDefault(state.API, "not configured")),
		fmt.Sprintf("User: %s | Token: %s | Health: %s", valueOrDefault(state.User, "not authenticated"), state.Token, homeHealthSummary(state.Checks)),
	}
	if strings.TrimSpace(state.Warning) != "" {
		header = append(header, "Warning: "+strings.TrimSpace(state.Warning))
	}
	return header
}

func sessionFooterLines() []string {
	return []string{
		"Keys: type filter | Up/Down move | PgUp/PgDn jump | Enter select | Esc quit | Ctrl+C quit",
		"Quick: nopsai api call --interactive | nopsai api request GET /v1/auth/me | nopsai platform release --interactive",
	}
}

func commandOutputScreenOptions(title string, state homeState, footer []string) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Title:      title,
		Breadcrumb: []string{"Home", title},
		Header:     sessionHeaderLines(state),
		Footer:     sessionFooterLines(),
	}
}

func requireChoiceValue(raw, field string, supported ...string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" && len(supported) > 0 {
		value = supported[0]
	}
	for _, option := range supported {
		if value == option {
			return value, nil
		}
	}
	return "", fmt.Errorf("%s must be one of: %s", field, strings.Join(supported, ", "))
}

func splitPromptLines(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			values = append(values, line)
		}
	}
	return values
}

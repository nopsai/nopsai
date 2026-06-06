package nopsai

import "strings"

func normalizeVariableSourceKey(value string) string {
	key := strings.TrimSpace(strings.ToLower(value))
	switch {
	case strings.Contains(key, "git"):
		return "git"
	case strings.Contains(key, "draft"):
		return "draft"
	case strings.Contains(key, "local"):
		return "local"
	default:
		return "database"
	}
}

func isYAMLFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}

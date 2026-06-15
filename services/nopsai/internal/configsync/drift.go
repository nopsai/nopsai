package configsync

import (
	"database/sql"
	"path/filepath"
	"strings"

	"nopsai/pkg/models"
)

type DriftPathOptions struct {
	ExternalTriggersDirectory string
	SettingsRelativePath      func(string) bool
}

func IncludesResource(repo models.ConfigRepository, identifier, source string, configRepoID sql.NullInt64, managed bool, delegatedScopes []string) bool {
	if ResourceUnderAnyScope(identifier, delegatedScopes) {
		return false
	}
	if managed && configRepoID.Valid {
		return configRepoID.Int64 == repo.ID
	}
	if !strings.EqualFold(strings.TrimSpace(source), "database") {
		return false
	}
	if repo.ScopeType == models.ConfigRepositoryScopeFolder {
		_, ok := RelativeResourceIdentifier(repo, identifier)
		return ok
	}
	return repo.ScopeType == models.ConfigRepositoryScopeSystem
}

func ExportPath(repo models.ConfigRepository, identifier, sourcePath, directory, extension string, managed bool, configRepoID sql.NullInt64, options DriftPathOptions) (string, bool) {
	if managed && configRepoID.Valid && configRepoID.Int64 == repo.ID && strings.TrimSpace(sourcePath) != "" {
		return ManagedSourcePath(repo, sourcePath, options)
	}
	relID, ok := RelativeResourceIdentifier(repo, identifier)
	if !ok || relID == "" {
		return "", false
	}
	return filepath.ToSlash(filepath.Join(directory, relID+extension)), true
}

func ScopeFilePath(repo models.ConfigRepository, scope, sourcePath string, managed bool, configRepoID sql.NullInt64, options DriftPathOptions) (string, bool) {
	if managed && configRepoID.Valid && configRepoID.Int64 == repo.ID && strings.TrimSpace(sourcePath) != "" {
		return ManagedSourcePath(repo, sourcePath, options)
	}
	relScope, ok := RelativeResourceIdentifier(repo, RuntimeScopeForDisplay(scope))
	if !ok {
		return "", false
	}
	if relScope == "" || relScope == "default" {
		relScope = "default"
	}
	return filepath.ToSlash(filepath.Join("scopes", relScope, "scope.yaml")), true
}

func ManagedSourcePath(repo models.ConfigRepository, sourcePath string, options DriftPathOptions) (string, bool) {
	cleaned := strings.Trim(strings.TrimSpace(filepath.ToSlash(sourcePath)), "/")
	if cleaned == "" {
		return "", false
	}
	if rel, ok := RelativeGitPath(repo.BasePath, cleaned); ok && IsDriftPath(rel, options) {
		return rel, true
	}
	if IsDriftPath(cleaned, options) {
		return cleaned, true
	}
	return "", false
}

func RelativeResourceIdentifier(repo models.ConfigRepository, identifier string) (string, bool) {
	identifier = strings.Trim(strings.TrimSpace(strings.ReplaceAll(identifier, "\\", "/")), "/")
	if repo.ScopeType != models.ConfigRepositoryScopeFolder {
		return identifier, identifier != ""
	}
	scopeID := strings.Trim(strings.TrimSpace(repo.ScopeID), "/")
	if scopeID == "" {
		return "", false
	}
	if identifier == scopeID {
		return "", true
	}
	prefix := scopeID + "/"
	if strings.HasPrefix(identifier, prefix) {
		return strings.TrimPrefix(identifier, prefix), true
	}
	return "", false
}

func RelativeGitPath(basePath, filePath string) (string, bool) {
	basePath = strings.Trim(strings.TrimSpace(filepath.ToSlash(basePath)), "/")
	filePath = strings.Trim(strings.TrimSpace(filepath.ToSlash(filePath)), "/")
	if filePath == "" {
		return "", false
	}
	if basePath == "" {
		return filePath, true
	}
	if filePath == basePath {
		return "", false
	}
	prefix := basePath + "/"
	if strings.HasPrefix(filePath, prefix) {
		return strings.TrimPrefix(filePath, prefix), true
	}
	return "", false
}

func IsDriftPath(filePath string, options DriftPathOptions) bool {
	options = normalizeDriftPathOptions(options)
	filePath = strings.Trim(strings.TrimSpace(filepath.ToSlash(filePath)), "/")
	if filePath == "notifications.yaml" {
		return true
	}
	for _, prefix := range []string{
		"pipelines/",
		"steps/",
		"triggers/",
		options.ExternalTriggersDirectory + "/",
		"schedules/",
		"scopes/",
		"knowledge/",
		"config-repositories/",
	} {
		if strings.HasPrefix(filePath, prefix) {
			return true
		}
	}
	if strings.HasPrefix(filePath, "access/") && isYAMLFile(filePath) {
		return true
	}
	if rel, ok := strings.CutPrefix(filePath, "setting/"); ok {
		return options.SettingsRelativePath(rel)
	}
	return false
}

func RuntimeScopeForDisplay(scope string) string {
	scope = strings.Trim(strings.TrimSpace(scope), "/")
	if scope == "" || strings.EqualFold(scope, "default") {
		return "default"
	}
	return scope
}

func normalizeDriftPathOptions(options DriftPathOptions) DriftPathOptions {
	options.ExternalTriggersDirectory = strings.Trim(strings.TrimSpace(options.ExternalTriggersDirectory), "/")
	if options.ExternalTriggersDirectory == "" {
		options.ExternalTriggersDirectory = "external-triggers"
	}
	if options.SettingsRelativePath == nil {
		options.SettingsRelativePath = func(string) bool { return false }
	}
	return options
}

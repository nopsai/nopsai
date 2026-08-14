package configsync

import (
	"database/sql"
	"path/filepath"
	"strings"

	"nopsai/pkg/models"
)

type DriftPathOptions struct {
	ExternalTriggersDirectory  string
	GitWebhookSourcesDirectory string
	DashboardDirectory         string
	DashboardTemplateDirectory string
	// RegistryDirectories holds the per-resource registry directories, such as
	// models and agent roles, that own one file per resource.
	RegistryDirectories  []string
	SettingsRelativePath func(string) bool
}

func IncludesResource(repo models.ConfigRepository, identifier, source string, configRepoID sql.NullInt64, managed bool, delegatedScopes []string) bool {
	if ResourceUnderAnyScope(identifier, delegatedScopes) {
		return false
	}
	underBindingScope := ResourceUnderBindingScope(identifier, repo)
	if repo.ScopeType == models.ConfigRepositoryScopeTeam {
		_, underBindingScope = RelativeResourceIdentifier(repo, identifier)
	}
	if !underBindingScope {
		return false
	}
	if managed && configRepoID.Valid {
		if configRepoID.Int64 == repo.ID {
			return true
		}
		return repo.ScopeType == models.ConfigRepositoryScopeTeam
	}
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "database", "":
		return true
	case "git":
		return true
	default:
		return false
	}
}

func ExportPath(repo models.ConfigRepository, identifier, sourcePath, directory, extension string, managed bool, configRepoID sql.NullInt64, options DriftPathOptions) (string, bool) {
	relID, ok := RelativeResourceIdentifier(repo, identifier)
	if !ok || relID == "" {
		return "", false
	}
	canonicalID := strings.Trim(strings.TrimSpace(strings.ReplaceAll(identifier, "\\", "/")), "/")
	if canonicalID == "" {
		return "", false
	}
	canonicalPath := filepath.ToSlash(filepath.Join(directory, canonicalID+extension))
	if managed && configRepoID.Valid && configRepoID.Int64 == repo.ID && strings.TrimSpace(sourcePath) != "" {
		if managedPath, ok := ManagedSourcePathForCanonical(repo, sourcePath, canonicalPath, options); ok {
			return managedPath, true
		}
	}
	return canonicalPath, true
}

func ScopeFilePath(repo models.ConfigRepository, scope, sourcePath string, managed bool, configRepoID sql.NullInt64, options DriftPathOptions) (string, bool) {
	displayScope := RuntimeScopeForDisplay(scope)
	_, ok := RelativeResourceIdentifier(repo, displayScope)
	if !ok {
		return "", false
	}
	canonicalScope := strings.Trim(strings.TrimSpace(displayScope), "/")
	if canonicalScope == "" || canonicalScope == "default" {
		canonicalScope = "default"
	}
	canonicalPath := filepath.ToSlash(filepath.Join("scopes", canonicalScope, "scope.yaml"))
	if managed && configRepoID.Valid && configRepoID.Int64 == repo.ID && strings.TrimSpace(sourcePath) != "" {
		if managedPath, ok := ManagedSourcePathForCanonical(repo, sourcePath, canonicalPath, options); ok {
			return managedPath, true
		}
	}
	return canonicalPath, true
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

func ManagedSourcePathForCanonical(repo models.ConfigRepository, sourcePath, canonicalPath string, options DriftPathOptions) (string, bool) {
	managedPath, ok := ManagedSourcePath(repo, sourcePath, options)
	if !ok {
		return "", false
	}
	if !EquivalentExportPath(managedPath, canonicalPath) {
		return "", false
	}
	return managedPath, true
}

func EquivalentExportPath(path, canonicalPath string) bool {
	path = strings.Trim(strings.TrimSpace(filepath.ToSlash(path)), "/")
	canonicalPath = strings.Trim(strings.TrimSpace(filepath.ToSlash(canonicalPath)), "/")
	if path == "" || canonicalPath == "" {
		return false
	}
	return strings.TrimSuffix(path, filepath.Ext(path)) == strings.TrimSuffix(canonicalPath, filepath.Ext(canonicalPath))
}

func RelativeResourceIdentifier(repo models.ConfigRepository, identifier string) (string, bool) {
	identifier = strings.Trim(strings.TrimSpace(strings.ReplaceAll(identifier, "\\", "/")), "/")
	if repo.ScopeType != models.ConfigRepositoryScopeTeam {
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
		options.GitWebhookSourcesDirectory + "/",
		options.DashboardDirectory + "/",
		options.DashboardTemplateDirectory + "/",
		"schedules/",
		"scopes/",
		"knowledge/",
		"config-repositories/",
	} {
		if strings.HasPrefix(filePath, prefix) {
			return true
		}
	}
	for _, directory := range options.RegistryDirectories {
		directory = strings.Trim(strings.TrimSpace(directory), "/")
		if directory != "" && strings.HasPrefix(filePath, directory+"/") {
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
	options.GitWebhookSourcesDirectory = strings.Trim(strings.TrimSpace(options.GitWebhookSourcesDirectory), "/")
	if options.GitWebhookSourcesDirectory == "" {
		options.GitWebhookSourcesDirectory = "git-webhook-sources"
	}
	options.DashboardDirectory = strings.Trim(strings.TrimSpace(options.DashboardDirectory), "/")
	if options.DashboardDirectory == "" {
		options.DashboardDirectory = "dashboards"
	}
	options.DashboardTemplateDirectory = strings.Trim(strings.TrimSpace(options.DashboardTemplateDirectory), "/")
	if options.DashboardTemplateDirectory == "" {
		options.DashboardTemplateDirectory = "dashboard-templates"
	}
	if options.SettingsRelativePath == nil {
		options.SettingsRelativePath = func(string) bool { return false }
	}
	return options
}

package nopsai

import (
	"database/sql"
	"path/filepath"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/internal/mcpregistry"
)

const (
	configRepositoryAccessAllPath             = "access/all.yaml"
	configRepositoryAccessGrantsPath          = "access/grants.yaml"
	configRepositoryServiceAccountsAccessPath = "access/service-accounts.yaml"
	configRepositoryLLMProfilesPath           = "setting/system/llm_profile.yaml"
	configRepositoryAgentProfilesPath         = "setting/system/agent-profiles.yaml"
	configRepositoryMCPRegistryPath           = "setting/system/mcp.yaml"
	configRepositoryMailSettingsPath          = "setting/system/mail.yaml"
	configRepositoryCredentialsPath           = "setting/system/credentials.yaml"
)

func configRepositoryIncludesResource(repo models.ConfigRepository, identifier, source string, configRepoID sql.NullInt64, managed bool, delegatedScopes []string) bool {
	return configsync.IncludesResource(repo, identifier, source, configRepoID, managed, delegatedScopes)
}

func configRepositoryExportPath(repo models.ConfigRepository, identifier, sourcePath, directory, extension string, managed bool, configRepoID sql.NullInt64) (string, bool) {
	return configsync.ExportPath(repo, identifier, sourcePath, directory, extension, managed, configRepoID, configRepositoryDriftPathOptions())
}

func configRepositoryNotificationRoutePath(repo models.ConfigRepository, groupPath, sourcePath string, managed bool, configRepoID sql.NullInt64) (string, bool) {
	relID, ok := configsync.RelativeResourceIdentifier(repo, groupPath)
	if !ok {
		return "", false
	}
	if repo.ScopeType == models.ConfigRepositoryScopeFolder && relID == "" {
		return "notifications.yaml", true
	}
	if relID == "" {
		return "", false
	}
	return filepath.ToSlash(filepath.Join("config-repositories", "groups", relID, "notifications.yaml")), true
}

func configRepositoryScopeFilePath(repo models.ConfigRepository, scope, sourcePath string, managed bool, configRepoID sql.NullInt64) (string, bool) {
	return configsync.ScopeFilePath(repo, scope, sourcePath, managed, configRepoID, configRepositoryDriftPathOptions())
}

func configRepositoryManagedSourcePath(repo models.ConfigRepository, sourcePath string) (string, bool) {
	return configsync.ManagedSourcePath(repo, sourcePath, configRepositoryDriftPathOptions())
}

func configRepositoryRelativeResourceIdentifier(repo models.ConfigRepository, identifier string) (string, bool) {
	return configsync.RelativeResourceIdentifier(repo, identifier)
}

func configRepositoryRelativeGitPath(basePath, filePath string) (string, bool) {
	return configsync.RelativeGitPath(basePath, filePath)
}

func isConfigRepositoryDriftPath(filePath string) bool {
	if isGitOpsTeamAIProfilesPath(filePath) {
		return true
	}
	return configsync.IsDriftPath(filePath, configRepositoryDriftPathOptions())
}

func isConfigRepositorySettingsDriftPath(rel string) bool {
	return isGitOpsAuthSettingsRelativePath(rel) ||
		isGitOpsRuntimeSettingsRelativePath(rel) ||
		isGitOpsGitHubSettingsRelativePath(rel) ||
		isGitOpsMailSettingsRelativePath(rel) ||
		isGitOpsCredentialsRelativePath(rel) ||
		isGitOpsLLMProfileRelativePath(rel) ||
		isGitOpsAgentProfileRelativePath(rel) ||
		mcpregistry.IsGitOpsRelativePath(rel)
}

func configRepositoryDriftPathOptions() configsync.DriftPathOptions {
	return configsync.DriftPathOptions{
		ExternalTriggersDirectory:  externalTriggersGitOpsDirectory,
		GitWebhookSourcesDirectory: gitWebhookSourcesGitOpsDirectory,
		SettingsRelativePath:       isConfigRepositorySettingsDriftPath,
	}
}

func normalizeConfigRepositoryFileContent(content string) string {
	return configsync.NormalizeFileContent(content)
}

func defaultConfigRepositoryPushMessage(repo models.ConfigRepository) string {
	scope := strings.Trim(repo.ScopeType+"/"+repo.ScopeID, "/")
	if scope == "" {
		scope = "config"
	}
	return "Update Nopsai config for " + scope
}

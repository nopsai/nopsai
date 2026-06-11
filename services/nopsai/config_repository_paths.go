package nopsai

import (
	"database/sql"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/internal/mcpregistry"
)

const (
	configRepositoryAccessAllPath             = "access/all.yaml"
	configRepositoryAccessGrantsPath          = "access/grants.yaml"
	configRepositoryServiceAccountsAccessPath = "access/service-accounts.yaml"
	configRepositoryGroupStructurePath        = "config-repositories/groups/structure.yaml"
	configRepositoryLLMProfilesPath           = "setting/system/llm_profile.yaml"
	configRepositoryAgentProfilesPath         = "setting/system/agent-profiles.yaml"
	configRepositoryMCPRegistryPath           = "setting/system/mcp.yaml"
)

func configRepositoryIncludesResource(repo models.ConfigRepository, identifier, source string, configRepoID sql.NullInt64, managed bool, delegatedScopes []string) bool {
	return configsync.IncludesResource(repo, identifier, source, configRepoID, managed, delegatedScopes)
}

func configRepositoryExportPath(repo models.ConfigRepository, identifier, sourcePath, directory, extension string, managed bool, configRepoID sql.NullInt64) (string, bool) {
	return configsync.ExportPath(repo, identifier, sourcePath, directory, extension, managed, configRepoID, configRepositoryDriftPathOptions())
}

func configRepositoryNotificationRoutePath(repo models.ConfigRepository, groupPath, sourcePath string, managed bool, configRepoID sql.NullInt64) (string, bool) {
	return configsync.NotificationRoutePath(repo, groupPath, sourcePath, managed, configRepoID, configRepositoryDriftPathOptions())
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
	return configsync.IsDriftPath(filePath, configRepositoryDriftPathOptions())
}

func isConfigRepositorySettingsDriftPath(rel string) bool {
	return isGitOpsRuntimeSettingsRelativePath(rel) ||
		isGitOpsMailSettingsRelativePath(rel) ||
		isGitOpsLLMProfileRelativePath(rel) ||
		isGitOpsAgentProfileRelativePath(rel) ||
		mcpregistry.IsGitOpsRelativePath(rel)
}

func configRepositoryDriftPathOptions() configsync.DriftPathOptions {
	return configsync.DriftPathOptions{
		ExternalTriggersDirectory: externalTriggersGitOpsDirectory,
		NotificationsDirectory:    notificationGitOpsDirectory,
		SettingsRelativePath:      isConfigRepositorySettingsDriftPath,
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

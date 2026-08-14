package nopsai

import (
	"context"
	"database/sql"
	"sort"
	"strings"

	"nopsai/config"
	"nopsai/pkg/models"
)

// exportConfigRepositoryModels writes one file per model: models/<name>.yaml for
// the workspace registry and models/<team>/<name>.yaml for team-owned models.
func (a *App) exportConfigRepositoryModels(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	if repo.ScopeType == models.ConfigRepositoryScopeSystem {
		defaultProfile, profiles, found, err := a.loadLLMProfilesFromDB(ctx)
		if err != nil {
			return err
		}
		if !found {
			cfg := a.getConfigSnapshot()
			defaultProfile = cfg.EffectiveLLMDefaultProfile()
			profiles = cfg.EffectiveLLMProfiles()
		}
		profiles = config.NormalizeLLMProfiles(profiles)
		defaultProfile = config.NormalizeLLMProfileName(defaultProfile)
		for _, name := range sortedProfileNames(profiles) {
			filePath, ok := registryGitOpsExportPath(repo, modelsGitOpsDirectory, "", name)
			if !ok {
				continue
			}
			content, err := marshalConfigRepositoryYAML(buildModelGitOpsFile(name, profiles[name], name == defaultProfile))
			if err != nil {
				return err
			}
			files[filePath] = string(content)
		}
	}

	teamModels, teamDefaults, err := a.loadTeamModelsForExport(ctx)
	if err != nil {
		return err
	}
	for _, teamPath := range sortedRegistryTeamPaths(teamModels) {
		if !configRepositoryIncludesResource(repo, teamPath, "database", sql.NullInt64{}, false, delegatedScopes) {
			continue
		}
		profiles := teamModels[teamPath]
		for _, name := range sortedProfileNames(profiles) {
			filePath, ok := registryGitOpsExportPath(repo, modelsGitOpsDirectory, teamPath, name)
			if !ok {
				continue
			}
			content, err := marshalConfigRepositoryYAML(buildModelGitOpsFile(name, profiles[name], name == teamDefaults[teamPath]))
			if err != nil {
				return err
			}
			files[filePath] = string(content)
		}
	}
	return nil
}

// exportConfigRepositoryAgentRoles writes one file per agent role.
func (a *App) exportConfigRepositoryAgentRoles(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	if repo.ScopeType == models.ConfigRepositoryScopeSystem {
		stored, err := a.loadStoredAgentProfilesFromDB(ctx)
		if err != nil {
			return err
		}
		defaultProfile, err := a.effectiveAgentProfileDefault(ctx, nil)
		if err != nil {
			return err
		}
		names := make([]string, 0, len(stored))
		for name := range stored {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			filePath, ok := registryGitOpsExportPath(repo, agentRolesGitOpsDirectory, "", name)
			if !ok {
				continue
			}
			content, err := marshalConfigRepositoryYAML(buildAgentRoleGitOpsFile(stored[name].AgentProfile, name == defaultProfile))
			if err != nil {
				return err
			}
			files[filePath] = string(content)
		}
	}

	teamRoles, teamDefaults, err := a.loadTeamAgentRolesForExport(ctx)
	if err != nil {
		return err
	}
	for _, teamPath := range sortedRegistryTeamPaths(teamRoles) {
		if !configRepositoryIncludesResource(repo, teamPath, "database", sql.NullInt64{}, false, delegatedScopes) {
			continue
		}
		roles := teamRoles[teamPath]
		names := make([]string, 0, len(roles))
		for name := range roles {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			filePath, ok := registryGitOpsExportPath(repo, agentRolesGitOpsDirectory, teamPath, name)
			if !ok {
				continue
			}
			content, err := marshalConfigRepositoryYAML(buildAgentRoleGitOpsFile(roles[name], name == teamDefaults[teamPath]))
			if err != nil {
				return err
			}
			files[filePath] = string(content)
		}
	}
	return nil
}

// exportConfigRepositoryMCPRegistry writes one file per MCP server and profile.
func (a *App) exportConfigRepositoryMCPRegistry(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	if repo.ScopeType == models.ConfigRepositoryScopeSystem {
		servers, profiles, found, err := a.loadMCPRegistryFromDB(ctx)
		if err != nil {
			return err
		}
		if !found {
			cfg := a.getConfigSnapshot()
			servers = cfg.EffectiveMCPServers()
			profiles = cfg.EffectiveMCPProfiles()
		}
		servers = models.NormalizeMCPServers(servers)
		profiles = models.NormalizeMCPProfiles(profiles)
		for _, name := range sortedMCPServerKeys(servers) {
			filePath, ok := registryGitOpsExportPath(repo, mcpServersGitOpsDirectory, "", name)
			if !ok {
				continue
			}
			content, err := marshalConfigRepositoryYAML(buildMCPServerGitOpsFile(name, servers[name]))
			if err != nil {
				return err
			}
			files[filePath] = string(content)
		}
		for _, name := range sortedMCPProfileKeys(profiles) {
			filePath, ok := registryGitOpsExportPath(repo, mcpProfilesGitOpsDirectory, "", name)
			if !ok {
				continue
			}
			content, err := marshalConfigRepositoryYAML(buildMCPProfileGitOpsFile(name, profiles[name]))
			if err != nil {
				return err
			}
			files[filePath] = string(content)
		}
	}

	teamProfiles, err := a.loadTeamMCPProfilesForExport(ctx)
	if err != nil {
		return err
	}
	for _, teamPath := range sortedRegistryTeamPaths(teamProfiles) {
		if !configRepositoryIncludesResource(repo, teamPath, "database", sql.NullInt64{}, false, delegatedScopes) {
			continue
		}
		profiles := teamProfiles[teamPath]
		for _, name := range sortedMCPProfileKeys(profiles) {
			filePath, ok := registryGitOpsExportPath(repo, mcpProfilesGitOpsDirectory, teamPath, name)
			if !ok {
				continue
			}
			content, err := marshalConfigRepositoryYAML(buildMCPProfileGitOpsFile(name, profiles[name]))
			if err != nil {
				return err
			}
			files[filePath] = string(content)
		}
	}
	return nil
}

func (a *App) loadTeamModelsForExport(ctx context.Context) (map[string]map[string]config.LLMProfile, map[string]string, error) {
	profiles := map[string]map[string]config.LLMProfile{}
	defaults := map[string]string{}
	if a == nil || a.db == nil {
		return profiles, defaults, nil
	}
	records, err := loadTeamPathRecords(ctx, a.db)
	if err != nil {
		return nil, nil, err
	}
	for _, record := range records {
		teamPath := strings.Trim(strings.TrimSpace(record.Path), "/")
		if teamPath == "" {
			continue
		}
		storedDefault, teamProfiles, err := a.loadTeamLLMProfilesFromDB(ctx, record.ID)
		if err != nil {
			return nil, nil, err
		}
		if len(teamProfiles) == 0 {
			continue
		}
		profiles[teamPath] = config.NormalizeLLMProfiles(teamProfiles)
		defaults[teamPath] = config.NormalizeLLMProfileName(storedDefault)
	}
	return profiles, defaults, nil
}

func (a *App) loadTeamAgentRolesForExport(ctx context.Context) (map[string]map[string]models.AgentProfile, map[string]string, error) {
	roles := map[string]map[string]models.AgentProfile{}
	defaults := map[string]string{}
	if a == nil || a.db == nil {
		return roles, defaults, nil
	}
	records, err := loadTeamPathRecords(ctx, a.db)
	if err != nil {
		return nil, nil, err
	}
	for _, record := range records {
		teamPath := strings.Trim(strings.TrimSpace(record.Path), "/")
		if teamPath == "" {
			continue
		}
		teamRoles, err := a.loadTeamAgentProfilesFromDB(ctx, record.ID)
		if err != nil {
			return nil, nil, err
		}
		if len(teamRoles) == 0 {
			continue
		}
		roles[teamPath] = teamRoles
		defaultRole, err := a.loadTeamProfileSetting(ctx, record.ID, teamAgentDefaultProfileSetting)
		if err != nil {
			return nil, nil, err
		}
		defaults[teamPath] = normalizeAgentProfileDefault(defaultRole)
	}
	return roles, defaults, nil
}

func (a *App) loadTeamMCPProfilesForExport(ctx context.Context) (map[string]map[string]models.MCPProfile, error) {
	profiles := map[string]map[string]models.MCPProfile{}
	if a == nil || a.db == nil {
		return profiles, nil
	}
	records, err := loadTeamPathRecords(ctx, a.db)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		teamPath := strings.Trim(strings.TrimSpace(record.Path), "/")
		if teamPath == "" {
			continue
		}
		teamProfiles, err := a.loadTeamMCPProfilesFromDB(ctx, record.ID)
		if err != nil {
			return nil, err
		}
		if len(teamProfiles) == 0 {
			continue
		}
		profiles[teamPath] = teamProfiles
	}
	return profiles, nil
}

func sortedProfileNames(profiles map[string]config.LLMProfile) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedMCPServerKeys(servers map[string]models.MCPServer) []string {
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedMCPProfileKeys(profiles map[string]models.MCPProfile) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedRegistryTeamPaths[T any](byTeam map[string]T) []string {
	teams := make([]string, 0, len(byTeam))
	for team := range byTeam {
		teams = append(teams, team)
	}
	sort.Strings(teams)
	return teams
}

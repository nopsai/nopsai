package nopsai

import (
	"context"
	"sort"

	"nopsai/config"
	"nopsai/pkg/models"
)

// exportConfigRepositoryModels writes one file per model at
// models/<name>.yaml, so a team-scoped model name lands under its team path in
// the same shape as pipelines.
func (a *App) exportConfigRepositoryModels(ctx context.Context, repo models.ConfigRepository, files map[string]string) error {
	if repo.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil
	}
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
		filePath, ok := registryGitOpsExportPath(repo, modelsGitOpsDirectory, name)
		if !ok || !isConfigRepositoryDriftPath(filePath) {
			continue
		}
		content, err := marshalConfigRepositoryYAML(buildModelGitOpsFile(name, profiles[name], name == defaultProfile))
		if err != nil {
			return err
		}
		files[filePath] = string(content)
	}
	return nil
}

// exportConfigRepositoryAgentRoles writes one file per agent role.
func (a *App) exportConfigRepositoryAgentRoles(ctx context.Context, repo models.ConfigRepository, files map[string]string) error {
	if repo.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil
	}
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
		filePath, ok := registryGitOpsExportPath(repo, agentRolesGitOpsDirectory, name)
		if !ok || !isConfigRepositoryDriftPath(filePath) {
			continue
		}
		content, err := marshalConfigRepositoryYAML(buildAgentRoleGitOpsFile(stored[name].AgentProfile, name == defaultProfile))
		if err != nil {
			return err
		}
		files[filePath] = string(content)
	}
	return nil
}

// exportConfigRepositoryMCPRegistry writes one file per MCP server and profile.
func (a *App) exportConfigRepositoryMCPRegistry(ctx context.Context, repo models.ConfigRepository, files map[string]string) error {
	if repo.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil
	}
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
		filePath, ok := registryGitOpsExportPath(repo, mcpServersGitOpsDirectory, name)
		if !ok || !isConfigRepositoryDriftPath(filePath) {
			continue
		}
		content, err := marshalConfigRepositoryYAML(buildMCPServerGitOpsFile(name, servers[name]))
		if err != nil {
			return err
		}
		files[filePath] = string(content)
	}
	for _, name := range sortedMCPProfileKeys(profiles) {
		filePath, ok := registryGitOpsExportPath(repo, mcpProfilesGitOpsDirectory, name)
		if !ok || !isConfigRepositoryDriftPath(filePath) {
			continue
		}
		content, err := marshalConfigRepositoryYAML(buildMCPProfileGitOpsFile(name, profiles[name]))
		if err != nil {
			return err
		}
		files[filePath] = string(content)
	}
	return nil
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

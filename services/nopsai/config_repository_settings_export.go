package main

import (
	"context"
	"sort"
	"strings"

	"nopsai/config"
	"nopsai/pkg/models"
)

type configRepositoryLLMProfilesExportDocument struct {
	DefaultProfile string           `yaml:"default_profile"`
	Profiles       []llmProfileForm `yaml:"profiles"`
}

func (a *App) exportConfigRepositoryLLMProfiles(ctx context.Context, repo models.ConfigRepository, files map[string]string) error {
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
	if len(profiles) == 0 {
		return nil
	}
	defaultProfile = config.NormalizeLLMProfileName(defaultProfile)
	if defaultProfile == "" {
		defaultProfile = config.DefaultLLMProfileName
	}

	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	doc := configRepositoryLLMProfilesExportDocument{DefaultProfile: defaultProfile}
	for _, name := range names {
		doc.Profiles = append(doc.Profiles, profileFormFromConfig(name, profiles[name]))
	}
	content, err := marshalConfigRepositoryYAML(doc)
	if err != nil {
		return err
	}
	files[configRepositoryLLMProfilesPath] = string(content)
	return nil
}

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
	if len(servers) == 0 && len(profiles) == 0 {
		return nil
	}

	exportServers := map[string]models.MCPServer{}
	for name, server := range servers {
		server.Name = ""
		exportServers[name] = server
	}
	exportProfiles := map[string]models.MCPProfile{}
	for name, profile := range profiles {
		profile.Name = ""
		exportProfiles[name] = profile
	}

	content, err := marshalConfigRepositoryYAML(mcpRegistryRequest{
		MCPServers:  exportServers,
		MCPProfiles: exportProfiles,
	})
	if err != nil {
		return err
	}
	files[configRepositoryMCPRegistryPath] = string(content)
	return nil
}

func (a *App) exportConfigRepositoryRuntimeSettings(repo models.ConfigRepository, files map[string]string) error {
	if repo.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil
	}
	doc := buildRuntimeSettingsGitOpsFile(a.getConfigSnapshot())
	content, err := marshalConfigRepositoryYAML(doc)
	if err != nil {
		return err
	}
	files["setting/system/runner.yaml"] = string(content)
	return nil
}

func (a *App) exportConfigRepositoryMailSettings(ctx context.Context, repo models.ConfigRepository, files map[string]string) error {
	if repo.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil
	}
	record, err := a.loadNotificationMailSettings(ctx)
	if err != nil {
		return err
	}
	filePath := "settings/system/mail.yaml"
	if record.ManagedByConfigRepo && record.ConfigRepoID != nil && *record.ConfigRepoID == repo.ID && strings.TrimSpace(record.ConfigSourcePath) != "" {
		if managedPath, ok := configRepositoryManagedSourcePath(repo, record.ConfigSourcePath); ok {
			filePath = managedPath
		}
	}
	content, err := marshalConfigRepositoryYAML(record.notificationMailSettingsFile)
	if err != nil {
		return err
	}
	files[filePath] = string(content)
	return nil
}

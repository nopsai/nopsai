package nopsai

import (
	"context"
	"sort"
	"strings"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/mcpregistry"
)

type configRepositoryLLMProfilesExportDocument struct {
	DefaultProfile string           `yaml:"default_profile"`
	Profiles       []llmProfileForm `yaml:"profiles"`
}

type configRepositoryAgentProfilesExportDocument struct {
	DefaultProfile string             `yaml:"default_profile"`
	AgentProfiles  []agentProfileForm `yaml:"agent_profiles"`
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

func (a *App) exportConfigRepositoryAgentProfiles(ctx context.Context, repo models.ConfigRepository, files map[string]string) error {
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
	if len(stored) == 0 && defaultProfile == models.DefaultAgentProfileID {
		return nil
	}
	names := make([]string, 0, len(stored))
	for name := range stored {
		names = append(names, name)
	}
	sort.Strings(names)
	doc := configRepositoryAgentProfilesExportDocument{DefaultProfile: defaultProfile}
	for _, name := range names {
		profile := stored[name].AgentProfile
		doc.AgentProfiles = append(doc.AgentProfiles, agentProfileFormFromModel(profile))
	}
	content, err := marshalConfigRepositoryYAML(doc)
	if err != nil {
		return err
	}
	files[configRepositoryAgentProfilesPath] = string(content)
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

	content, err := marshalConfigRepositoryYAML(mcpregistry.RegistryRequest{
		MCPServers:  exportServers,
		MCPProfiles: exportProfiles,
	})
	if err != nil {
		return err
	}
	files[configRepositoryMCPRegistryPath] = string(content)
	return nil
}

func (a *App) exportConfigRepositoryAuthSettings(ctx context.Context, repo models.ConfigRepository, files map[string]string) error {
	if repo.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil
	}

	settings, providers, mappings, err := a.loadAuthSettingsGitOpsState(ctx)
	if err != nil {
		return err
	}
	doc := buildAuthSettingsGitOpsFile(settings, providers, mappings)
	if !authSettingsGitOpsFileHasState(doc) {
		return nil
	}
	content, err := marshalConfigRepositoryYAML(doc)
	if err != nil {
		return err
	}
	files["setting/system/auth.yaml"] = string(content)
	return nil
}

func (a *App) loadAuthSettingsGitOpsState(ctx context.Context) (oidcSettings, []oidcProviderRecord, map[string]string, error) {
	cfg := config.Config{AuthProviderLocalEnabled: true}
	if a != nil && a.cfg != nil {
		cfg = a.getConfigSnapshot()
	}

	settings := oidcSettings{
		LocalEnabled:      cfg.EffectiveAuthProviderLocalEnabled(),
		OIDCEnabled:       cfg.EffectiveOIDCAuth().Enabled,
		AutoCreateUsers:   cfg.EffectiveOIDCAuth().AutoCreateUsers,
		DefaultRole:       strings.TrimSpace(cfg.EffectiveOIDCAuth().DefaultRole),
		AllowEmailLinking: cfg.EffectiveOIDCAuth().AllowEmailLinking,
	}
	var providers []oidcProviderRecord
	mappings := normalizeOIDCDomainMappings(cfg.EffectiveOIDCAuth().DomainMapping)
	for id, providerCfg := range cfg.EffectiveOIDCAuth().Providers {
		provider := oidcProviderRecordFromConfig(id, providerCfg, authProviderSourceConfig)
		if provider.ID != "" {
			providers = append(providers, provider)
		}
	}

	if a == nil || a.db == nil {
		return settings, providers, mappings, nil
	}

	dbSettings, err := getOIDCSettings(ctx, a.db, a.cfg)
	if err != nil {
		return oidcSettings{}, nil, nil, err
	}
	dbProviders, err := listOIDCProviders(ctx, a.db, false)
	if err != nil {
		return oidcSettings{}, nil, nil, err
	}
	dbMappings, err := listOIDCDomainMappings(ctx, a.db)
	if err != nil {
		return oidcSettings{}, nil, nil, err
	}
	if len(dbProviders) > 0 || len(dbMappings) > 0 || dbSettings != settings {
		return dbSettings, dbProviders, dbMappings, nil
	}
	return settings, providers, mappings, nil
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

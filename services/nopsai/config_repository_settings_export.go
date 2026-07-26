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

type configRepositoryTeamAIProfilesExportDocument = teamAIProfilesGitOpsFile

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

func (a *App) exportConfigRepositoryTeamAIProfiles(ctx context.Context, repo models.ConfigRepository, files map[string]string) error {
	if repo.ScopeType != models.ConfigRepositoryScopeTeam {
		return nil
	}
	record, status, err := a.resolveTeamRecord(ctx, repo.ScopeID, false)
	if err != nil {
		if status == 404 {
			return nil
		}
		return err
	}

	llmDefault, err := a.loadTeamProfileSetting(ctx, record.ID, teamLLMDefaultProfileSetting)
	if err != nil {
		return err
	}
	_, llmProfiles, err := a.loadTeamLLMProfilesFromDB(ctx, record.ID)
	if err != nil {
		return err
	}
	agentDefault, err := a.loadTeamProfileSetting(ctx, record.ID, teamAgentDefaultProfileSetting)
	if err != nil {
		return err
	}
	agentProfiles, err := a.loadTeamAgentProfilesFromDB(ctx, record.ID)
	if err != nil {
		return err
	}
	mcpProfiles, err := a.loadTeamMCPProfilesFromDB(ctx, record.ID)
	if err != nil {
		return err
	}
	if len(llmProfiles) == 0 && len(agentProfiles) == 0 && len(mcpProfiles) == 0 && strings.TrimSpace(llmDefault) == "" && strings.TrimSpace(agentDefault) == "" {
		return nil
	}

	doc := configRepositoryTeamAIProfilesExportDocument{}
	llmDefault = config.NormalizeLLMProfileName(llmDefault)
	if llmDefault != "" {
		doc.LLMDefaultProfile = &llmDefault
	}
	llmNames := make([]string, 0, len(llmProfiles))
	for name := range llmProfiles {
		llmNames = append(llmNames, name)
	}
	sort.Strings(llmNames)
	for _, name := range llmNames {
		doc.LLMProfiles = append(doc.LLMProfiles, profileFormFromConfig(name, llmProfiles[name]))
	}

	agentDefault = normalizeAgentProfileDefault(agentDefault)
	if agentDefault != "" {
		doc.AgentDefaultProfile = &agentDefault
	}
	agentNames := make([]string, 0, len(agentProfiles))
	for name := range agentProfiles {
		agentNames = append(agentNames, name)
	}
	sort.Strings(agentNames)
	for _, name := range agentNames {
		doc.AgentProfiles = append(doc.AgentProfiles, agentProfileFormFromModel(agentProfiles[name]))
	}

	mcpNames := make([]string, 0, len(mcpProfiles))
	for name := range mcpProfiles {
		mcpNames = append(mcpNames, name)
	}
	sort.Strings(mcpNames)
	for _, name := range mcpNames {
		profile := models.NormalizeMCPProfile(mcpProfiles[name])
		profile.Name = name
		doc.MCPProfiles = append(doc.MCPProfiles, profile)
	}

	content, err := marshalConfigRepositoryYAML(doc)
	if err != nil {
		return err
	}
	files[configRepositoryTeamAIProfilesPath] = string(content)
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
	cfg := a.getConfigSnapshot()
	runtimeDoc := buildRuntimeSettingsGitOpsFile(cfg)
	runtimeContent, err := marshalConfigRepositoryYAML(runtimeDoc)
	if err != nil {
		return err
	}
	files["setting/system/runner.yaml"] = string(runtimeContent)

	githubDoc := buildGitHubSettingsGitOpsFile(cfg)
	githubContent, err := marshalConfigRepositoryYAML(githubDoc)
	if err != nil {
		return err
	}
	files["setting/git-apps/github.yaml"] = string(githubContent)
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
	content, err := marshalConfigRepositoryYAML(record.notificationMailSettingsFile)
	if err != nil {
		return err
	}
	files[configRepositoryMailSettingsPath] = string(content)
	return nil
}

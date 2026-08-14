package nopsai

import (
	"context"
	"database/sql"
	"sort"
	"strings"

	"nopsai/config"
	"nopsai/pkg/models"
)

type configRepositoryTeamDefaultsExportDocument = teamDefaultsGitOpsFile

func (a *App) exportConfigRepositoryTeamDefaults(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	records, err := loadTeamPathRecords(ctx, a.db)
	if err != nil {
		return err
	}
	teamPaths := make([]string, 0, len(records))
	recordsByPath := map[string]teamPathRecord{}
	for _, record := range records {
		teamPath := strings.Trim(strings.TrimSpace(record.Path), "/")
		if teamPath == "" || strings.EqualFold(record.Kind, "app") || record.RepositoryFullName != "" || record.RepoURL != "" {
			continue
		}
		if !configRepositoryIncludesResource(repo, teamPath, "database", sql.NullInt64{}, false, delegatedScopes) {
			continue
		}
		teamPaths = append(teamPaths, teamPath)
		recordsByPath[teamPath] = record
	}
	sort.Strings(teamPaths)
	for _, teamPath := range teamPaths {
		record := recordsByPath[teamPath]
		llmDefault, err := a.loadTeamProfileSetting(ctx, record.ID, teamLLMDefaultProfileSetting)
		if err != nil {
			return err
		}
		agentDefault, err := a.loadTeamProfileSetting(ctx, record.ID, teamAgentDefaultProfileSetting)
		if err != nil {
			return err
		}
		knowledgeDefaults, err := a.loadTeamKnowledgeDefaults(ctx, record)
		if err != nil {
			return err
		}
		doc, ok := buildConfigRepositoryTeamDefaultsExportDocument(llmDefault, agentDefault, knowledgeDefaults)
		if !ok {
			continue
		}
		filePath, ok := configRepositoryTeamDefaultsFilePath(repo, teamPath)
		if !ok {
			continue
		}
		content, err := marshalConfigRepositoryYAML(doc)
		if err != nil {
			return err
		}
		files[filePath] = string(content)
	}
	return nil
}

func buildConfigRepositoryTeamDefaultsExportDocument(
	llmDefault string,
	agentDefault string,
	knowledgeDefaults map[string]string,
) (configRepositoryTeamDefaultsExportDocument, bool) {
	doc := configRepositoryTeamDefaultsExportDocument{}
	hasDefaults := false
	llmDefault = config.NormalizeLLMProfileName(llmDefault)
	if llmDefault != "" {
		doc.LLMProfile = &llmDefault
		hasDefaults = true
	}
	agentDefault = normalizeAgentProfileDefault(agentDefault)
	if agentDefault != "" {
		doc.AgentProfile = &agentDefault
		hasDefaults = true
	}
	if len(knowledgeDefaults) > 0 {
		doc.KnowledgeContext = knowledgeDefaults
		hasDefaults = true
	}
	return doc, hasDefaults
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

	assistantDoc := buildAssistantSettingsGitOpsFile(cfg)
	assistantContent, err := marshalConfigRepositoryYAML(assistantDoc)
	if err != nil {
		return err
	}
	files[configRepositoryAssistantSettingsPath] = string(assistantContent)
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

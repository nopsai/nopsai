package nopsai

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/internal/mcpregistry"
	"nopsai/services/nopsai/pkg/validation"

	"gopkg.in/yaml.v3"
)

type configSyncPlan struct {
	configRepositoryPipelineRunStructure map[string]*configsync.PipelineRunStructureNode
	configRepositories                   map[string]storedConfigRepository
	accessPlan                           accessSyncPlan
	knowledgeContexts                    map[string]storedKnowledgeContext
	llmProfilePlan                       *gitOpsLLMProfilePlan
	agentProfilePlan                     *gitOpsAgentProfilePlan
	mcpRegistryPlan                      *mcpregistry.GitOpsPlan
	teamDefaultsPlans                    map[string]*gitOpsTeamDefaultsPlan
	authSettingsPlan                     *gitOpsAuthSettingsPlan
	credentialPlan                       *gitOpsCredentialPlan
	runtimeSettingsPlan                  *gitOpsRuntimeSettingsPlan
	githubSettingsPlan                   *gitOpsGitHubSettingsPlan
	assistantSettingsPlan                *gitOpsAssistantSettingsPlan
	mailSettingsPlan                     *gitOpsMailSettingsPlan
	dataManagementPlan                   *gitOpsDataManagementPlan
	schedules                            map[string]storedSchedule
	dashboards                           map[string]storedDashboard
	externalTriggers                     map[string]storedExternalTrigger
	gitWebhookSources                    map[string]storedGitWebhookSource
	notificationRoutes                   map[string]storedNotificationRoute
	pipelines                            map[string]storedPipeline
	steps                                map[string]storedStep
	generalScopeVars                     map[generalScopeVarKey]storedScopeVar
	repoScopeVars                        map[repoScopeVarKey]storedScopeVar
	generalScopeSecrets                  map[generalScopeSecretKey]storedScopeSecret
	repoScopeSecrets                     map[repoScopeSecretKey]storedScopeSecret
	triggers                             map[string]storedTrigger
}

func (a *App) parseConfigSyncPlan(binding models.ConfigRepository, repoCtx configSyncRepositoryContext, files configSyncRepositoryFiles) (configSyncPlan, error) {
	basePath := repoCtx.basePath
	boundTeam := repoCtx.boundTeam
	pipelineDir := repoCtx.dirs.pipeline
	stepDir := repoCtx.dirs.step
	triggerDir := repoCtx.dirs.trigger
	externalTriggerDir := repoCtx.dirs.externalTrigger
	scheduleDir := repoCtx.dirs.schedule
	dashboardDir := repoCtx.dirs.dashboard
	scopeDir := repoCtx.dirs.scope
	configRepositoryDir := repoCtx.dirs.configRepository
	accessDir := repoCtx.dirs.access
	knowledgeDir := repoCtx.dirs.knowledge
	settingDir := repoCtx.dirs.setting

	plan := configSyncPlan{
		configRepositoryPipelineRunStructure: map[string]*configsync.PipelineRunStructureNode{},
		configRepositories:                   map[string]storedConfigRepository{},
		pipelines:                            map[string]storedPipeline{},
		steps:                                map[string]storedStep{},
		generalScopeVars:                     map[generalScopeVarKey]storedScopeVar{},
		repoScopeVars:                        map[repoScopeVarKey]storedScopeVar{},
		generalScopeSecrets:                  map[generalScopeSecretKey]storedScopeSecret{},
		repoScopeSecrets:                     map[repoScopeSecretKey]storedScopeSecret{},
		triggers:                             map[string]storedTrigger{},
	}

	for path, content := range files.configRepositories {
		normalized := filepath.ToSlash(path)
		rel, ok := configsync.RelativePath(normalized, configRepositoryDir)
		if !ok {
			continue
		}
		if rel == "" || strings.HasSuffix(rel, "/") || !isYAMLFile(rel) {
			continue
		}
		if _, ok, err := configRepositoryTeamNotificationRoutePath(rel); err != nil {
			return configSyncPlan{}, fmt.Errorf("invalid notification route path '%s': %w", normalized, err)
		} else if ok {
			continue
		}
		if _, ok, err := configRepositoryTeamDefaultsFileScope(rel); err != nil {
			return configSyncPlan{}, fmt.Errorf("invalid team defaults path '%s': %w", normalized, err)
		} else if ok {
			continue
		}
		structure, isStructureFile, err := configsync.ParseConfigRepositoryTeamPipelineRunStructure(rel, content)
		if err != nil {
			return configSyncPlan{}, fmt.Errorf("failed to parse config repository team structure '%s': %w", normalized, err)
		}
		if isStructureFile {
			if binding.ScopeType == models.ConfigRepositoryScopeTeam {
				structure, err = configsync.NormalizePipelineRunStructureForTeam(boundTeam, structure)
				if err != nil {
					return configSyncPlan{}, fmt.Errorf("failed to normalize config repository team structure '%s': %w", normalized, err)
				}
			}
			inlineConfigRepositories, err := configRepositoryBindingsFromPipelineRunStructure(structure, normalized)
			if err != nil {
				return configSyncPlan{}, err
			}
			for key, stored := range inlineConfigRepositories {
				if _, exists := plan.configRepositories[key]; exists {
					return configSyncPlan{}, fmt.Errorf("duplicate config repository binding for '%s' detected", key)
				}
				plan.configRepositories[key] = stored
			}
			configsync.MergePipelineRunStructure(plan.configRepositoryPipelineRunStructure, structure)
			continue
		}

		scopeType, scopeID, err := configsync.ParseBindingPath(rel)
		if err != nil {
			return configSyncPlan{}, fmt.Errorf("invalid config repository binding '%s': %w", normalized, err)
		}

		var file configsync.BindingFile
		if err := yaml.Unmarshal([]byte(content), &file); err != nil {
			return configSyncPlan{}, fmt.Errorf("failed to parse config repository binding '%s': %w", normalized, err)
		}
		if err := configsync.ValidateBindingFile(file, scopeType, scopeID, normalized); err != nil {
			return configSyncPlan{}, err
		}
		if binding.ScopeType == models.ConfigRepositoryScopeTeam {
			scopeID, err = configsync.NormalizePathForTeam(boundTeam, scopeID)
			if err != nil {
				return configSyncPlan{}, fmt.Errorf("invalid team-scoped config repository binding '%s': %w", normalized, err)
			}
		}
		basePath, err := configsync.NormalizeRepositoryBasePathForRequest(file.BasePath)
		if err != nil {
			return configSyncPlan{}, fmt.Errorf("invalid base_path in config repository binding '%s': %w", normalized, err)
		}
		enabled := true
		if file.Enabled != nil {
			enabled = *file.Enabled
		}
		writeEnabled, writeBranch := configsync.BindingWriteSettings(file)
		branch := strings.TrimSpace(file.Branch)
		if branch == "" {
			branch = "main"
		}
		provider, err := configsync.NormalizeRepositoryProvider(file.Provider, file.RepoURL)
		if err != nil {
			return configSyncPlan{}, err
		}

		key := scopeType + "/" + scopeID
		if _, exists := plan.configRepositories[key]; exists {
			return configSyncPlan{}, fmt.Errorf("duplicate config repository binding for '%s' detected", key)
		}
		plan.configRepositories[key] = storedConfigRepository{
			scopeType:     scopeType,
			scopeID:       scopeID,
			provider:      provider,
			repoURL:       strings.TrimSpace(file.RepoURL),
			branch:        branch,
			basePath:      basePath,
			credentialRef: strings.TrimSpace(file.CredentialRef),
			enabled:       enabled,
			writeEnabled:  writeEnabled,
			writeBranch:   writeBranch,
			sourcePath:    normalized,
		}
	}

	accessPlan, err := parseAccessSyncPlan(files.access, accessDir, binding, boundTeam)
	if err != nil {
		return configSyncPlan{}, err
	}
	knowledgeContexts, err := parseGitOpsKnowledgeContexts(files.knowledge, knowledgeDir, binding, boundTeam, accessPlan)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.knowledgeContexts = knowledgeContexts
	plan.llmProfilePlan, err = parseGitOpsLLMProfilePlan(
		binding,
		gitOpsLLMProfileDirectory{root: settingDir, files: files.setting},
	)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.agentProfilePlan, err = parseGitOpsAgentProfilePlan(
		binding,
		gitOpsAgentProfileDirectory{root: settingDir, files: files.setting},
	)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.mcpRegistryPlan, err = mcpregistry.ParseGitOpsPlan(
		binding,
		mcpregistry.GitOpsDirectory{Root: settingDir, Files: files.setting},
	)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.teamDefaultsPlans, err = parseGitOpsTeamDefaultsPlans(binding, repoCtx, files.configRepositories)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.authSettingsPlan, err = parseGitOpsAuthSettingsPlan(
		binding,
		gitOpsRuntimeSettingsDirectory{root: settingDir, files: files.setting},
	)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.credentialPlan, err = parseGitOpsCredentialPlan(
		binding,
		gitOpsCredentialDirectory{root: settingDir, files: files.setting},
	)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.runtimeSettingsPlan, err = parseGitOpsRuntimeSettingsPlan(
		binding,
		gitOpsRuntimeSettingsDirectory{root: settingDir, files: files.setting},
	)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.githubSettingsPlan, err = parseGitOpsGitHubSettingsPlan(
		binding,
		gitOpsRuntimeSettingsDirectory{root: settingDir, files: files.setting},
	)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.assistantSettingsPlan, err = parseGitOpsAssistantSettingsPlan(
		binding,
		gitOpsRuntimeSettingsDirectory{root: settingDir, files: files.setting},
	)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.mailSettingsPlan, err = parseGitOpsMailSettingsPlan(
		binding,
		gitOpsRuntimeSettingsDirectory{root: settingDir, files: files.setting},
	)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.dataManagementPlan, err = parseGitOpsDataManagementPlan(
		binding,
		gitOpsRuntimeSettingsDirectory{root: settingDir, files: files.setting},
	)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.schedules, err = parseGitOpsSchedules(files.schedules, scheduleDir, binding, boundTeam)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.dashboards, err = parseGitOpsDashboards(files.dashboards, dashboardDir, binding, boundTeam)
	if err != nil {
		return configSyncPlan{}, err
	}
	for path, content := range files.dashboards {
		normalized := filepath.ToSlash(path)
		rel, ok := configsync.RelativePath(normalized, dashboardDir)
		if !ok {
			continue
		}
		if rel == "" || strings.HasSuffix(rel, "/") || !isYAMLFile(rel) {
			continue
		}
		identifier := strings.TrimSuffix(rel, filepath.Ext(rel))
		if binding.ScopeType == models.ConfigRepositoryScopeTeam {
			targetID, err := configsync.NormalizePathForTeam(boundTeam, rel)
			if err != nil {
				return configSyncPlan{}, fmt.Errorf("invalid team-scoped dashboard path '%s': %w", normalized, err)
			}
			identifier = strings.TrimSuffix(targetID, filepath.Ext(targetID))
		}
		teamPath, slug, err := splitDashboardRef(identifier)
		if err != nil {
			return configSyncPlan{}, fmt.Errorf("invalid dashboard path '%s': %w", normalized, err)
		}
		key := dashboardResourceID(teamPath, slug)
		if err := accessPlan.addEmbeddedResourceAccess(content, normalized, grantResourceDashboard, key, binding, boundTeam); err != nil {
			return configSyncPlan{}, fmt.Errorf("invalid dashboard access '%s': %w", normalized, err)
		}
	}
	plan.externalTriggers, err = parseGitOpsExternalTriggers(files.externalTriggers, externalTriggerDir, binding, boundTeam)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.gitWebhookSources, err = parseGitOpsGitWebhookSources(files.gitWebhookSources, repoCtx.dirs.gitWebhookSource, binding, boundTeam)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.notificationRoutes, err = parseGitOpsNotificationRoutes(files.notifications, basePath, binding, boundTeam)
	if err != nil {
		return configSyncPlan{}, err
	}
	colocatedNotificationRoutes, err := parseGitOpsConfigRepositoryNotificationRoutes(files.configRepositories, configRepositoryDir, binding, boundTeam)
	if err != nil {
		return configSyncPlan{}, err
	}
	for key, route := range colocatedNotificationRoutes {
		if _, exists := plan.notificationRoutes[key]; exists {
			return configSyncPlan{}, fmt.Errorf("duplicate notification route for team '%s' detected", route.teamPath)
		}
		plan.notificationRoutes[key] = route
	}

	for path, content := range files.steps {
		normalized := filepath.ToSlash(path)
		rel, ok := configsync.RelativePath(normalized, stepDir)
		if !ok {
			continue
		}
		if rel == "" || strings.HasSuffix(rel, "/") || !isYAMLFile(rel) {
			continue
		}

		var step models.PipelineStep
		if err := yaml.Unmarshal([]byte(content), &step); err != nil {
			return configSyncPlan{}, fmt.Errorf("failed to parse reusable step '%s': %w", normalized, err)
		}
		stepName := step.GetName()
		if stepName == "" {
			return configSyncPlan{}, fmt.Errorf("reusable step '%s' is missing the required 'name' field", normalized)
		}
		if err := validation.ValidateReusableStep(&step); err != nil {
			return configSyncPlan{}, fmt.Errorf("invalid reusable step '%s': %w", normalized, err)
		}

		stepPath, fileBase, _, err := configsync.SplitStepIdentifier(rel)
		if err != nil {
			return configSyncPlan{}, fmt.Errorf("invalid reusable step path '%s': %w", normalized, err)
		}
		if stepName != fileBase {
			return configSyncPlan{}, fmt.Errorf("reusable step '%s' name '%s' must match file name '%s'", normalized, stepName, fileBase)
		}

		if binding.ScopeType == models.ConfigRepositoryScopeTeam {
			targetID, err := configsync.NormalizePathForTeam(boundTeam, rel)
			if err != nil {
				return configSyncPlan{}, fmt.Errorf("invalid team-scoped reusable step path '%s': %w", normalized, err)
			}
			stepPath, fileBase, _, err = configsync.SplitStepIdentifier(targetID)
			if err != nil {
				return configSyncPlan{}, fmt.Errorf("invalid normalized reusable step path '%s': %w", targetID, err)
			}
		}

		key := configsync.BuildStepIdentifier(stepPath, fileBase)
		if _, exists := plan.steps[key]; exists {
			return configSyncPlan{}, fmt.Errorf("duplicate reusable step '%s' detected in config repository", key)
		}
		if err := accessPlan.addEmbeddedResourceAccess(content, normalized, grantResourceStep, key, binding, boundTeam); err != nil {
			return configSyncPlan{}, fmt.Errorf("invalid reusable step access '%s': %w", normalized, err)
		}

		plan.steps[key] = storedStep{
			definition: content,
			path:       stepPath,
			name:       fileBase,
			sourcePath: normalized,
		}
	}

	for path, content := range files.pipelines {
		normalized := filepath.ToSlash(path)
		rel, ok := configsync.RelativePath(normalized, pipelineDir)
		if !ok {
			continue
		}
		if rel == "" || strings.HasSuffix(rel, "/") || !isYAMLFile(rel) {
			continue
		}

		var pipeline models.Pipeline
		if err := yaml.Unmarshal([]byte(content), &pipeline); err != nil {
			return configSyncPlan{}, fmt.Errorf("failed to parse pipeline '%s': %w", normalized, err)
		}
		if err := a.validatePipelineWithConfigSyncStepIncludes(&pipeline, plan.steps, binding, boundTeam); err != nil {
			return configSyncPlan{}, fmt.Errorf("pipeline validation failed for '%s': %w", normalized, err)
		}

		pipelinePath, fileBase, _, err := configsync.SplitPipelineIdentifier(rel)
		if err != nil {
			return configSyncPlan{}, fmt.Errorf("invalid pipeline path '%s': %w", normalized, err)
		}
		if pipeline.Name != fileBase {
			return configSyncPlan{}, fmt.Errorf("pipeline '%s' name '%s' must match file name '%s'", normalized, pipeline.Name, fileBase)
		}

		if binding.ScopeType == models.ConfigRepositoryScopeTeam {
			targetID, err := configsync.NormalizePathForTeam(boundTeam, rel)
			if err != nil {
				return configSyncPlan{}, fmt.Errorf("invalid team-scoped pipeline path '%s': %w", normalized, err)
			}
			pipelinePath, fileBase, _, err = configsync.SplitPipelineIdentifier(targetID)
			if err != nil {
				return configSyncPlan{}, fmt.Errorf("invalid normalized pipeline path '%s': %w", targetID, err)
			}
		}

		key := configsync.BuildPipelineIdentifier(pipelinePath, fileBase)
		if _, exists := plan.pipelines[key]; exists {
			return configSyncPlan{}, fmt.Errorf("duplicate pipeline '%s' detected in config repository", key)
		}
		if err := accessPlan.addEmbeddedResourceAccess(content, normalized, grantResourcePipeline, key, binding, boundTeam); err != nil {
			return configSyncPlan{}, fmt.Errorf("invalid pipeline access '%s': %w", normalized, err)
		}

		plan.pipelines[key] = storedPipeline{
			definition: content,
			version:    normalizePipelineVersion(pipeline.Version),
			path:       pipelinePath,
			name:       fileBase,
			sourcePath: normalized,
		}
	}

	for path, content := range files.scopes {
		normalized := filepath.ToSlash(path)
		rel, ok := configsync.RelativePath(normalized, scopeDir)
		if !ok {
			continue
		}
		if rel == "" || strings.HasSuffix(rel, "/") {
			continue
		}

		scopePath, ok, err := configsync.ParseScopeFilePath(rel)
		if err != nil {
			return configSyncPlan{}, fmt.Errorf("invalid scope file '%s': %w", normalized, err)
		}
		if !ok {
			continue
		}
		if binding.ScopeType == models.ConfigRepositoryScopeTeam {
			scopePath, err = configsync.NormalizePathForTeam(boundTeam, scopePath)
			if err != nil {
				return configSyncPlan{}, fmt.Errorf("invalid team-scoped scope path '%s': %w", normalized, err)
			}
		}

		var raw map[string]interface{}
		if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
			return configSyncPlan{}, fmt.Errorf("failed to parse scope file '%s': %w", normalized, err)
		}

		hasEmbeddedScopeAccess, err := a.addScopeConfigEntries(
			raw,
			plan.generalScopeVars,
			plan.repoScopeVars,
			plan.generalScopeSecrets,
			plan.repoScopeSecrets,
			scopePath,
			normalized,
			binding,
			boundTeam,
		)
		if err != nil {
			return configSyncPlan{}, err
		}
		if hasEmbeddedScopeAccess {
			if err := accessPlan.addEmbeddedResourceAccess(content, normalized, grantResourceScope, scopePath, binding, boundTeam); err != nil {
				return configSyncPlan{}, fmt.Errorf("invalid scope access '%s': %w", normalized, err)
			}
		}
	}

	for path, content := range files.triggers {
		normalized := filepath.ToSlash(path)
		rel, ok := configsync.RelativePath(normalized, triggerDir)
		if !ok {
			continue
		}
		if rel == "" || strings.HasSuffix(rel, "/") || !isYAMLFile(rel) {
			continue
		}

		repoKey := strings.TrimSuffix(rel, filepath.Ext(rel))
		repoKey = strings.Trim(repoKey, "/")
		if repoKey == "" {
			return configSyncPlan{}, fmt.Errorf("trigger file '%s' does not specify a repository", normalized)
		}
		if strings.Contains(repoKey, "..") {
			return configSyncPlan{}, fmt.Errorf("trigger file '%s' contains invalid path segments", normalized)
		}
		repoKey = filepath.ToSlash(repoKey)
		if binding.ScopeType == models.ConfigRepositoryScopeTeam {
			repoKey, err = configsync.NormalizePathForTeam(boundTeam, repoKey)
			if err != nil {
				return configSyncPlan{}, fmt.Errorf("invalid team-scoped trigger path '%s': %w", normalized, err)
			}
		}

		var manifest models.Manifest
		if err := yaml.Unmarshal([]byte(content), &manifest); err != nil {
			return configSyncPlan{}, fmt.Errorf("failed to parse trigger manifest '%s': %w", normalized, err)
		}
		fallbackTeamPath := fallbackRepositoryTriggerTeamPath(repoKey)
		if binding.ScopeType == models.ConfigRepositoryScopeTeam {
			fallbackTeamPath = boundTeam
		}
		record, err := repositoryTriggerRecordFromManifest(repoKey, content, "git", resourceVisibilityTeam, manifest, fallbackTeamPath)
		if err != nil {
			return configSyncPlan{}, fmt.Errorf("invalid trigger manifest metadata '%s': %w", normalized, err)
		}
		if err := validateRepositoryTriggerForNopsAI(record); err != nil {
			return configSyncPlan{}, fmt.Errorf("invalid trigger manifest metadata '%s': %w", normalized, err)
		}

		if _, exists := plan.triggers[repoKey]; exists {
			return configSyncPlan{}, fmt.Errorf("duplicate trigger manifest for repository '%s' detected", repoKey)
		}

		plan.triggers[repoKey] = storedTrigger{definition: content, sourcePath: normalized, record: record}
	}

	plan.accessPlan = accessPlan
	return plan, nil
}

func (a *App) validatePipelineWithConfigSyncStepIncludes(pipeline *models.Pipeline, steps map[string]storedStep, binding models.ConfigRepository, boundTeam string) error {
	resolver := func(ctx context.Context, includeIdentifier, stepPath, includeName string) (string, error) {
		for _, key := range configSyncStepIncludeLookupKeys(includeIdentifier, stepPath, includeName, binding, boundTeam) {
			if step, ok := steps[key]; ok {
				return step.definition, nil
			}
		}
		return a.resolveStoredStepIncludeDefinition(ctx, includeIdentifier, stepPath, includeName)
	}
	resolved, err := a.resolveStepIncludesWithResolver(context.Background(), pipeline, resolver)
	if err != nil {
		return err
	}
	return validatePipeline(resolved)
}

func configSyncStepIncludeLookupKeys(includeIdentifier, stepPath, includeName string, binding models.ConfigRepository, boundTeam string) []string {
	seen := map[string]bool{}
	keys := []string{}
	add := func(path, name string) {
		key := configsync.BuildStepIdentifier(path, name)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		keys = append(keys, key)
	}

	add(stepPath, includeName)
	if binding.ScopeType != models.ConfigRepositoryScopeTeam {
		return keys
	}
	targetID, err := configsync.NormalizePathForTeam(boundTeam, includeIdentifier)
	if err != nil || strings.TrimSpace(targetID) == "" {
		return keys
	}
	normalizedPath, normalizedName, _, err := configsync.SplitStepIdentifier(targetID)
	if err != nil {
		return keys
	}
	add(normalizedPath, normalizedName)
	return keys
}

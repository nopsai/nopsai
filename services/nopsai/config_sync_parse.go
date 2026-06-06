package nopsai

import (
	"fmt"
	"path/filepath"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/internal/mcpregistry"

	"gopkg.in/yaml.v3"
)

type configSyncPlan struct {
	pipelineRunStructure                 map[string]*configsync.PipelineRunStructureNode
	configRepositoryPipelineRunStructure map[string]*configsync.PipelineRunStructureNode
	configRepositories                   map[string]storedConfigRepository
	accessPlan                           accessSyncPlan
	knowledgeContexts                    map[string]storedKnowledgeContext
	llmProfilePlan                       *gitOpsLLMProfilePlan
	mcpRegistryPlan                      *mcpregistry.GitOpsPlan
	runtimeSettingsPlan                  *gitOpsRuntimeSettingsPlan
	mailSettingsPlan                     *gitOpsMailSettingsPlan
	schedules                            map[string]storedSchedule
	externalTriggers                     map[string]storedExternalTrigger
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
	boundFolder := repoCtx.boundFolder
	pipelineDir := repoCtx.dirs.pipeline
	stepDir := repoCtx.dirs.step
	triggerDir := repoCtx.dirs.trigger
	externalTriggerDir := repoCtx.dirs.externalTrigger
	scheduleDir := repoCtx.dirs.schedule
	scopeDir := repoCtx.dirs.scope
	pipelineRunDir := repoCtx.dirs.pipelineRun
	configRepositoryDir := repoCtx.dirs.configRepository
	accessDir := repoCtx.dirs.access
	knowledgeDir := repoCtx.dirs.knowledge
	notificationDir := repoCtx.dirs.notification
	settingDir := repoCtx.dirs.setting
	settingsDir := repoCtx.dirs.settings

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

	for path, content := range files.pipelineRuns {
		normalized := filepath.ToSlash(path)
		rel, ok := configsync.RelativePath(normalized, pipelineRunDir)
		if !ok {
			continue
		}
		if rel == "structure.yaml" || rel == "structure.yml" {
			parsed, err := configsync.ParsePipelineRunStructure(content)
			if err != nil {
				return configSyncPlan{}, fmt.Errorf("failed to parse pipeline run structure '%s': %w", normalized, err)
			}
			if binding.ScopeType == models.ConfigRepositoryScopeFolder {
				parsed, err = configsync.NormalizePipelineRunStructureForFolder(boundFolder, parsed)
				if err != nil {
					return configSyncPlan{}, fmt.Errorf("failed to normalize pipeline run structure '%s': %w", normalized, err)
				}
			}
			plan.pipelineRunStructure = parsed
			break
		}
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
		structure, isStructureFile, err := configsync.ParseConfigRepositoryGroupPipelineRunStructure(rel, content)
		if err != nil {
			return configSyncPlan{}, fmt.Errorf("failed to parse config repository group structure '%s': %w", normalized, err)
		}
		if isStructureFile {
			if binding.ScopeType == models.ConfigRepositoryScopeFolder {
				structure, err = configsync.NormalizePipelineRunStructureForFolder(boundFolder, structure)
				if err != nil {
					return configSyncPlan{}, fmt.Errorf("failed to normalize config repository group structure '%s': %w", normalized, err)
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
		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			scopeID, err = configsync.NormalizePathForFolder(boundFolder, scopeID)
			if err != nil {
				return configSyncPlan{}, fmt.Errorf("invalid group-scoped config repository binding '%s': %w", normalized, err)
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

		key := scopeType + "/" + scopeID
		if _, exists := plan.configRepositories[key]; exists {
			return configSyncPlan{}, fmt.Errorf("duplicate config repository binding for '%s' detected", key)
		}
		plan.configRepositories[key] = storedConfigRepository{
			scopeType:    scopeType,
			scopeID:      scopeID,
			repoURL:      strings.TrimSpace(file.RepoURL),
			branch:       branch,
			basePath:     basePath,
			enabled:      enabled,
			writeEnabled: writeEnabled,
			writeBranch:  writeBranch,
			sourcePath:   normalized,
		}
	}

	accessPlan, err := parseAccessSyncPlan(files.access, accessDir, binding, boundFolder)
	if err != nil {
		return configSyncPlan{}, err
	}
	knowledgeContexts, err := parseGitOpsKnowledgeContexts(files.knowledge, knowledgeDir, binding, boundFolder, accessPlan)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.knowledgeContexts = knowledgeContexts
	plan.llmProfilePlan, err = parseGitOpsLLMProfilePlan(
		binding,
		gitOpsLLMProfileDirectory{root: settingDir, files: files.setting},
		gitOpsLLMProfileDirectory{root: settingsDir, files: files.settings},
	)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.mcpRegistryPlan, err = mcpregistry.ParseGitOpsPlan(
		binding,
		mcpregistry.GitOpsDirectory{Root: settingDir, Files: files.setting},
		mcpregistry.GitOpsDirectory{Root: settingsDir, Files: files.settings},
	)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.runtimeSettingsPlan, err = parseGitOpsRuntimeSettingsPlan(
		binding,
		gitOpsRuntimeSettingsDirectory{root: settingDir, files: files.setting},
		gitOpsRuntimeSettingsDirectory{root: settingsDir, files: files.settings},
	)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.mailSettingsPlan, err = parseGitOpsMailSettingsPlan(
		binding,
		gitOpsRuntimeSettingsDirectory{root: settingDir, files: files.setting},
		gitOpsRuntimeSettingsDirectory{root: settingsDir, files: files.settings},
	)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.schedules, err = parseGitOpsSchedules(files.schedules, scheduleDir, binding, boundFolder)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.externalTriggers, err = parseGitOpsExternalTriggers(files.externalTriggers, externalTriggerDir, binding, boundFolder)
	if err != nil {
		return configSyncPlan{}, err
	}
	plan.notificationRoutes, err = parseGitOpsNotificationRoutes(files.notifications, notificationDir, basePath, binding, boundFolder)
	if err != nil {
		return configSyncPlan{}, err
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
		if err := validatePipeline(&pipeline); err != nil {
			return configSyncPlan{}, fmt.Errorf("pipeline validation failed for '%s': %w", normalized, err)
		}

		pipelinePath, fileBase, _, err := configsync.SplitPipelineIdentifier(rel)
		if err != nil {
			return configSyncPlan{}, fmt.Errorf("invalid pipeline path '%s': %w", normalized, err)
		}
		if pipeline.Name != fileBase {
			return configSyncPlan{}, fmt.Errorf("pipeline '%s' name '%s' must match file name '%s'", normalized, pipeline.Name, fileBase)
		}

		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			targetID, err := configsync.NormalizePathForFolder(boundFolder, rel)
			if err != nil {
				return configSyncPlan{}, fmt.Errorf("invalid group-scoped pipeline path '%s': %w", normalized, err)
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
		if err := accessPlan.addEmbeddedResourceAccess(content, normalized, grantResourcePipeline, key, binding, boundFolder); err != nil {
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

		stepPath, fileBase, _, err := configsync.SplitStepIdentifier(rel)
		if err != nil {
			return configSyncPlan{}, fmt.Errorf("invalid reusable step path '%s': %w", normalized, err)
		}
		if stepName != fileBase {
			return configSyncPlan{}, fmt.Errorf("reusable step '%s' name '%s' must match file name '%s'", normalized, stepName, fileBase)
		}

		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			targetID, err := configsync.NormalizePathForFolder(boundFolder, rel)
			if err != nil {
				return configSyncPlan{}, fmt.Errorf("invalid group-scoped reusable step path '%s': %w", normalized, err)
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
		if err := accessPlan.addEmbeddedResourceAccess(content, normalized, grantResourceStep, key, binding, boundFolder); err != nil {
			return configSyncPlan{}, fmt.Errorf("invalid reusable step access '%s': %w", normalized, err)
		}

		plan.steps[key] = storedStep{
			definition: content,
			path:       stepPath,
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
		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			scopePath, err = configsync.NormalizePathForFolder(boundFolder, scopePath)
			if err != nil {
				return configSyncPlan{}, fmt.Errorf("invalid group-scoped scope path '%s': %w", normalized, err)
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
			boundFolder,
		)
		if err != nil {
			return configSyncPlan{}, err
		}
		if hasEmbeddedScopeAccess {
			if err := accessPlan.addEmbeddedResourceAccess(content, normalized, grantResourceScope, scopePath, binding, boundFolder); err != nil {
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
		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			repoKey, err = configsync.NormalizePathForFolder(boundFolder, repoKey)
			if err != nil {
				return configSyncPlan{}, fmt.Errorf("invalid group-scoped trigger path '%s': %w", normalized, err)
			}
		}

		if err := yaml.Unmarshal([]byte(content), &models.Manifest{}); err != nil {
			return configSyncPlan{}, fmt.Errorf("failed to parse trigger manifest '%s': %w", normalized, err)
		}

		if _, exists := plan.triggers[repoKey]; exists {
			return configSyncPlan{}, fmt.Errorf("duplicate trigger manifest for repository '%s' detected", repoKey)
		}

		plan.triggers[repoKey] = storedTrigger{definition: content, sourcePath: normalized}
	}

	plan.accessPlan = accessPlan
	return plan, nil
}

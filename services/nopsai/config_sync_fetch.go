package nopsai

import (
	"context"
	"fmt"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
)

type configSyncGitReader interface {
	EnsureAccessible(ctx context.Context) error
	Directory(ctx context.Context, ref, path string) (map[string]string, error)
	File(ctx context.Context, ref, path string, notFoundErr error) (string, error)
}

type configSyncRepositoryContext struct {
	provider  string
	host      string
	owner     string
	repo      string
	project   string
	branch    string
	basePath  string
	boundTeam string
	dirs      configRepositoryGitDirs
}

type configSyncRepositoryFiles struct {
	pipelines          map[string]string
	steps              map[string]string
	triggers           map[string]string
	externalTriggers   map[string]string
	gitWebhookSources  map[string]string
	dashboards         map[string]string
	dashboardTemplates map[string]string
	schedules          map[string]string
	scopes             map[string]string
	configRepositories map[string]string
	access             map[string]string
	knowledge          map[string]string
	models             map[string]string
	agentRoles         map[string]string
	mcp                map[string]string
	notifications      map[string]string
	setting            map[string]string
}

func newConfigSyncRepositoryContext(binding models.ConfigRepository) (configSyncRepositoryContext, error) {
	repoURL := strings.TrimSpace(binding.RepoURL)
	if repoURL == "" {
		return configSyncRepositoryContext{}, fmt.Errorf("config repository URL is not configured")
	}
	branch := strings.TrimSpace(binding.Branch)
	if branch == "" {
		branch = "main"
	}
	basePath := configsync.NormalizeRepositoryBasePathValue(binding.BasePath)
	boundTeam := strings.Trim(strings.TrimSpace(binding.ScopeID), "/")
	if binding.ScopeType == models.ConfigRepositoryScopeTeam && boundTeam == "" {
		return configSyncRepositoryContext{}, fmt.Errorf("team-scoped config repository is missing its scope_id")
	}

	identity, err := configsync.ParseRepositoryIdentity(repoURL, binding.Provider)
	if err != nil {
		return configSyncRepositoryContext{}, fmt.Errorf("failed to parse config repository URL: %w", err)
	}

	return configSyncRepositoryContext{
		provider:  identity.Provider,
		host:      identity.Host,
		owner:     identity.Owner,
		repo:      identity.Repo,
		project:   identity.ProjectPath,
		branch:    branch,
		basePath:  basePath,
		boundTeam: boundTeam,
		dirs:      configRepositoryGitDirsForBasePath(basePath),
	}, nil
}

func fetchConfigSyncRepositoryFiles(ctx context.Context, reader configSyncGitReader, repoCtx configSyncRepositoryContext, binding models.ConfigRepository) (configSyncRepositoryFiles, error) {
	if err := reader.EnsureAccessible(ctx); err != nil {
		return configSyncRepositoryFiles{}, err
	}

	files := configSyncRepositoryFiles{
		notifications: map[string]string{},
	}
	directoryResults, err := fetchConfigRepositoryDirectories(ctx, reader, repoCtx.branch, []configRepositoryDirectoryRequest{
		{path: repoCtx.dirs.pipeline, resource: "pipeline definitions"},
		{path: repoCtx.dirs.step, resource: "reusable steps"},
		{path: repoCtx.dirs.trigger, resource: "trigger manifests"},
		{path: repoCtx.dirs.externalTrigger, resource: "external trigger manifests"},
		{path: repoCtx.dirs.gitWebhookSource, resource: "git webhook source manifests"},
		{path: repoCtx.dirs.dashboard, resource: "dashboard manifests"},
		{path: repoCtx.dirs.dashboardTemplate, resource: "dashboard template manifests"},
		{path: repoCtx.dirs.schedule, resource: "schedule manifests"},
		{path: repoCtx.dirs.scope, resource: "scope definitions"},
		{path: repoCtx.dirs.configRepository, resource: "config repository bindings"},
		{path: repoCtx.dirs.access, resource: "access manifests"},
		{path: repoCtx.dirs.knowledge, resource: "knowledge contexts"},
		{path: repoCtx.dirs.model, resource: "model definitions"},
		{path: repoCtx.dirs.agentRole, resource: "agent role definitions"},
		{path: repoCtx.dirs.mcp, resource: "MCP server and profile definitions"},
		{path: repoCtx.dirs.setting, resource: "system settings"},
	})
	if err != nil {
		return configSyncRepositoryFiles{}, err
	}
	files.pipelines = directoryResults[0]
	files.steps = directoryResults[1]
	files.triggers = directoryResults[2]
	files.externalTriggers = directoryResults[3]
	files.gitWebhookSources = directoryResults[4]
	files.dashboards = directoryResults[5]
	files.dashboardTemplates = directoryResults[6]
	files.schedules = directoryResults[7]
	files.scopes = directoryResults[8]
	files.configRepositories = directoryResults[9]
	files.access = directoryResults[10]
	files.knowledge = directoryResults[11]
	files.models = directoryResults[12]
	files.agentRoles = directoryResults[13]
	files.mcp = directoryResults[14]
	files.setting = directoryResults[15]

	if binding.ScopeType == models.ConfigRepositoryScopeTeam {
		rootRoutePath := configsync.RepoJoinPath(repoCtx.basePath, "notifications.yaml")
		optionalResults, err := fetchConfigRepositoryOptionalFiles(ctx, reader, repoCtx.branch, []configRepositoryOptionalFileRequest{
			{path: rootRoutePath, resource: "notification route", notFoundErr: errNotificationGitOpsNotFound},
		})
		if err != nil {
			return configSyncRepositoryFiles{}, err
		}
		for idx, result := range optionalResults {
			if !result.found {
				continue
			}
			if idx == 0 {
				files.notifications[result.path] = result.content
			}
		}
	}
	return files, nil
}

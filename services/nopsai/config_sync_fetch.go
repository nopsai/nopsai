package nopsai

import (
	"context"
	"errors"
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
	notifications      map[string]string
	teamAIProfiles     map[string]string
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

	fetchDir := func(path, resource string) (map[string]string, error) {
		files, err := reader.Directory(ctx, repoCtx.branch, path)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch %s: %w", resource, err)
		}
		return files, nil
	}

	var (
		files configSyncRepositoryFiles
		err   error
	)
	if files.pipelines, err = fetchDir(repoCtx.dirs.pipeline, "pipeline definitions"); err != nil {
		return configSyncRepositoryFiles{}, err
	}
	if files.steps, err = fetchDir(repoCtx.dirs.step, "reusable steps"); err != nil {
		return configSyncRepositoryFiles{}, err
	}
	if files.triggers, err = fetchDir(repoCtx.dirs.trigger, "trigger manifests"); err != nil {
		return configSyncRepositoryFiles{}, err
	}
	if files.externalTriggers, err = fetchDir(repoCtx.dirs.externalTrigger, "external trigger manifests"); err != nil {
		return configSyncRepositoryFiles{}, err
	}
	if files.gitWebhookSources, err = fetchDir(repoCtx.dirs.gitWebhookSource, "git webhook source manifests"); err != nil {
		return configSyncRepositoryFiles{}, err
	}
	if files.dashboards, err = fetchDir(repoCtx.dirs.dashboard, "dashboard manifests"); err != nil {
		return configSyncRepositoryFiles{}, err
	}
	if files.dashboardTemplates, err = fetchDir(repoCtx.dirs.dashboardTemplate, "dashboard template manifests"); err != nil {
		return configSyncRepositoryFiles{}, err
	}
	if files.schedules, err = fetchDir(repoCtx.dirs.schedule, "schedule manifests"); err != nil {
		return configSyncRepositoryFiles{}, err
	}
	if files.scopes, err = fetchDir(repoCtx.dirs.scope, "scope definitions"); err != nil {
		return configSyncRepositoryFiles{}, err
	}
	if files.configRepositories, err = fetchDir(repoCtx.dirs.configRepository, "config repository bindings"); err != nil {
		return configSyncRepositoryFiles{}, err
	}
	if files.access, err = fetchDir(repoCtx.dirs.access, "access manifests"); err != nil {
		return configSyncRepositoryFiles{}, err
	}
	if files.knowledge, err = fetchDir(repoCtx.dirs.knowledge, "knowledge contexts"); err != nil {
		return configSyncRepositoryFiles{}, err
	}
	files.notifications = map[string]string{}
	files.teamAIProfiles = map[string]string{}
	if binding.ScopeType == models.ConfigRepositoryScopeTeam {
		rootRoutePath := configsync.RepoJoinPath(repoCtx.basePath, "notifications.yaml")
		content, err := reader.File(ctx, repoCtx.branch, rootRoutePath, errNotificationGitOpsNotFound)
		if err == nil {
			files.notifications[rootRoutePath] = content
		} else if !errors.Is(err, errNotificationGitOpsNotFound) {
			return configSyncRepositoryFiles{}, fmt.Errorf("failed to fetch notification route '%s': %w", rootRoutePath, err)
		}
		for _, rootProfilePath := range []string{
			configsync.RepoJoinPath(repoCtx.basePath, "ai-profiles.yaml"),
			configsync.RepoJoinPath(repoCtx.basePath, "ai-profiles.yml"),
		} {
			content, err := reader.File(ctx, repoCtx.branch, rootProfilePath, errTeamAIProfilesGitOpsNotFound)
			if err == nil {
				files.teamAIProfiles[rootProfilePath] = content
				continue
			}
			if !errors.Is(err, errTeamAIProfilesGitOpsNotFound) {
				return configSyncRepositoryFiles{}, fmt.Errorf("failed to fetch team AI profiles '%s': %w", rootProfilePath, err)
			}
		}
	}
	if files.setting, err = fetchDir(repoCtx.dirs.setting, "system settings"); err != nil {
		return configSyncRepositoryFiles{}, err
	}
	return files, nil
}

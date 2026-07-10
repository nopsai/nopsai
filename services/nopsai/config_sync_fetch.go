package nopsai

import (
	"errors"
	"fmt"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
)

type configSyncGitReader interface {
	ensureConfigRepoAccessible(owner, repo string) error
	requestGitBotDirectory(owner, repo, ref, path string) (map[string]string, error)
	requestGitBotFile(owner, repo, ref, path string, notFoundErr error) (string, error)
}

type configSyncRepositoryContext struct {
	owner       string
	repo        string
	branch      string
	basePath    string
	boundFolder string
	dirs        configRepositoryGitDirs
}

type configSyncRepositoryFiles struct {
	pipelines          map[string]string
	steps              map[string]string
	triggers           map[string]string
	externalTriggers   map[string]string
	gitWebhookSources  map[string]string
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
	boundFolder := strings.Trim(strings.TrimSpace(binding.ScopeID), "/")
	if binding.ScopeType == models.ConfigRepositoryScopeFolder && boundFolder == "" {
		return configSyncRepositoryContext{}, fmt.Errorf("group-scoped config repository is missing its scope_id")
	}

	owner, repo, err := configsync.ParseGitHubRepoURL(repoURL)
	if err != nil {
		return configSyncRepositoryContext{}, fmt.Errorf("failed to parse config repository URL: %w", err)
	}

	return configSyncRepositoryContext{
		owner:       owner,
		repo:        repo,
		branch:      branch,
		basePath:    basePath,
		boundFolder: boundFolder,
		dirs:        configRepositoryGitDirsForBasePath(basePath),
	}, nil
}

func fetchConfigSyncRepositoryFiles(reader configSyncGitReader, repoCtx configSyncRepositoryContext, binding models.ConfigRepository) (configSyncRepositoryFiles, error) {
	if err := reader.ensureConfigRepoAccessible(repoCtx.owner, repoCtx.repo); err != nil {
		return configSyncRepositoryFiles{}, err
	}

	fetchDir := func(path, resource string) (map[string]string, error) {
		files, err := reader.requestGitBotDirectory(repoCtx.owner, repoCtx.repo, repoCtx.branch, path)
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
	if binding.ScopeType == models.ConfigRepositoryScopeFolder {
		rootRoutePath := configsync.RepoJoinPath(repoCtx.basePath, "notifications.yaml")
		content, err := reader.requestGitBotFile(repoCtx.owner, repoCtx.repo, repoCtx.branch, rootRoutePath, errNotificationGitOpsNotFound)
		if err == nil {
			files.notifications[rootRoutePath] = content
		} else if !errors.Is(err, errNotificationGitOpsNotFound) {
			return configSyncRepositoryFiles{}, fmt.Errorf("failed to fetch notification route '%s': %w", rootRoutePath, err)
		}
		for _, rootProfilePath := range []string{
			configsync.RepoJoinPath(repoCtx.basePath, "ai-profiles.yaml"),
			configsync.RepoJoinPath(repoCtx.basePath, "ai-profiles.yml"),
		} {
			content, err := reader.requestGitBotFile(repoCtx.owner, repoCtx.repo, repoCtx.branch, rootProfilePath, errTeamAIProfilesGitOpsNotFound)
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

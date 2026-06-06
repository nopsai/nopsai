package nopsai

import "nopsai/services/nopsai/internal/configsync"

type configRepositoryGitDirs struct {
	pipeline         string
	step             string
	trigger          string
	externalTrigger  string
	schedule         string
	scope            string
	pipelineRun      string
	configRepository string
	access           string
	knowledge        string
	notification     string
	setting          string
	settings         string
}

func configRepositoryGitDirsForBasePath(basePath string) configRepositoryGitDirs {
	return configRepositoryGitDirs{
		pipeline:         configsync.RepoJoinPath(basePath, "pipelines"),
		step:             configsync.RepoJoinPath(basePath, "steps"),
		trigger:          configsync.RepoJoinPath(basePath, "triggers"),
		externalTrigger:  configsync.RepoJoinPath(basePath, externalTriggersGitOpsDirectory),
		schedule:         configsync.RepoJoinPath(basePath, "schedules"),
		scope:            configsync.RepoJoinPath(basePath, "scopes"),
		pipelineRun:      configsync.RepoJoinPath(basePath, "pipelineruns"),
		configRepository: configsync.RepoJoinPath(basePath, "config-repositories"),
		access:           configsync.RepoJoinPath(basePath, "access"),
		knowledge:        configsync.RepoJoinPath(basePath, "knowledge"),
		notification:     configsync.RepoJoinPath(basePath, notificationGitOpsDirectory),
		setting:          configsync.RepoJoinPath(basePath, "setting"),
		settings:         configsync.RepoJoinPath(basePath, "settings"),
	}
}

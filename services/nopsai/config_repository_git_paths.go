package nopsai

import "nopsai/services/nopsai/internal/configsync"

type configRepositoryGitDirs struct {
	pipeline          string
	step              string
	trigger           string
	externalTrigger   string
	gitWebhookSource  string
	dashboard         string
	dashboardTemplate string
	schedule          string
	scope             string
	configRepository  string
	access            string
	knowledge         string
	setting           string
}

func configRepositoryGitDirsForBasePath(basePath string) configRepositoryGitDirs {
	return configRepositoryGitDirs{
		pipeline:          configsync.RepoJoinPath(basePath, "pipelines"),
		step:              configsync.RepoJoinPath(basePath, "steps"),
		trigger:           configsync.RepoJoinPath(basePath, "triggers"),
		externalTrigger:   configsync.RepoJoinPath(basePath, externalTriggersGitOpsDirectory),
		gitWebhookSource:  configsync.RepoJoinPath(basePath, gitWebhookSourcesGitOpsDirectory),
		dashboard:         configsync.RepoJoinPath(basePath, "dashboards"),
		dashboardTemplate: configsync.RepoJoinPath(basePath, "dashboard-templates"),
		schedule:          configsync.RepoJoinPath(basePath, "schedules"),
		scope:             configsync.RepoJoinPath(basePath, "scopes"),
		configRepository:  configsync.RepoJoinPath(basePath, "config-repositories"),
		access:            configsync.RepoJoinPath(basePath, "access"),
		knowledge:         configsync.RepoJoinPath(basePath, "knowledge"),
		setting:           configsync.RepoJoinPath(basePath, "setting"),
	}
}

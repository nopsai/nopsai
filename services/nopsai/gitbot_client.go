package main

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/gitbot"
)

type suiteCheckRunResponse = gitbot.SuiteCheckRunResponse
type gitBotCommitFile = gitbot.CommitFile
type gitBotCommitFilesResponse = gitbot.CommitFilesResponse

func (a *App) gitBotClient() gitbot.Client {
	return gitbot.Client{
		BaseURL:    a.cfg.NopsaiGitBotAPIURL,
		HTTPClient: a.httpClient,
	}
}

func (a *App) requestGitBotFile(owner, repo, ref, path string, notFoundErr error) (string, error) {
	return a.gitBotClient().File(owner, repo, ref, path, notFoundErr)
}

func (a *App) requestGitBotDirectory(owner, repo, ref, path string) (map[string]string, error) {
	return a.gitBotClient().Directory(owner, repo, ref, path)
}

func (a *App) requestGitBotCommitFiles(owner, repo, baseRef, branch, message string, files []gitBotCommitFile) (gitBotCommitFilesResponse, error) {
	return a.gitBotClient().CommitFiles(owner, repo, baseRef, branch, message, files)
}

func (a *App) branchHasOpenPullRequest(owner, repo, branch string) (bool, error) {
	return a.gitBotClient().BranchHasOpenPullRequest(owner, repo, branch)
}

func (a *App) ensureConfigRepoAccessible(owner, repo string) error {
	return a.gitBotClient().EnsureRepoAccessible(owner, repo)
}

func (a *App) requestGitBotPipeline(owner, repo, ref string, source models.PipelineSource) ([]byte, error) {
	return a.gitBotClient().Pipeline(owner, repo, ref, source, errPipelineNotFound)
}

func (a *App) findSuiteCheckRun(owner, repo string, suiteID int64, commitSHA string) (*suiteCheckRunResponse, error) {
	return a.gitBotClient().FindSuiteCheckRun(owner, repo, suiteID, commitSHA)
}

func (a *App) createGitHubCheckRun(owner, repo, ref string, pipelineDef []byte, pipelineSource string) (int64, error) {
	return a.gitBotClient().CreateCheckRun(owner, repo, ref, pipelineDef, pipelineSource)
}

func (a *App) createChildGitHubCheckRun(owner, repo, ref, parentName, includeName string, pipelineDef []byte) (int64, error) {
	return a.gitBotClient().CreateChildCheckRun(owner, repo, ref, parentName, includeName, pipelineDef)
}

func (a *App) initializeGitHubCheckRun(owner, repo string, checkRunID int64, pipelineDef []byte, pipelineName string) error {
	return a.gitBotClient().InitializeCheckRun(owner, repo, checkRunID, pipelineDef, pipelineName)
}

func (a *App) cancelStaleCheckRuns(owner, repo, beforeSHA string) {
	if err := a.gitBotClient().CancelStaleCheckRuns(owner, repo, beforeSHA); err != nil {
		log.Error().Err(err).Msg("Failed to request stale check run cancellation")
	}
}

func (a *App) notifyGitBotOfFinalStatus(status, failedStep, failedTask, summary string, gitContext map[string]string) {
	checkRunID, _ := strconv.ParseInt(gitContext["check_run_id"], 10, 64)
	if checkRunID == 0 {
		if runID := strings.TrimSpace(gitContext["run_id"]); runID != "" {
			_ = a.db.QueryRow(context.Background(), "SELECT git_check_run_id FROM pipeline_runs WHERE run_id = $1", runID).Scan(&checkRunID)
		}
	}
	if checkRunID == 0 {
		return
	}

	err := a.gitBotClient().NotifyFinalStatus(gitbot.FinalStatusRequest{
		Status:     status,
		FailedStep: failedStep,
		FailedTask: failedTask,
		CheckRunID: checkRunID,
		RepoOwner:  gitContext["repo_owner"],
		RepoName:   gitContext["repo_name"],
		CommitSHA:  gitContext["commit_sha"],
		Summary:    summary,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to notify git-bot of final status")
		return
	}
	log.Info().Msg("Successfully notified git-bot of final pipeline status.")
}

func (a *App) notifyGitBotOfTaskStatus(runID, stepName, taskName, taskStatus string) {
	var repoOwner, repoName, commitSHA, pipelineDef sql.NullString
	var checkRunID sql.NullInt64
	var taskIndex, totalTasks int
	var startedAt, finishedAt sql.NullTime

	query := `
		SELECT
			r.git_repo_owner, r.git_repo_name, r.git_commit_sha, r.git_check_run_id, r.pipeline_definition,
			t.task_index, (SELECT COUNT(*) FROM task_runs WHERE run_id = r.run_id),
			t.started_at, t.finished_at
		FROM pipeline_runs r JOIN task_runs t ON r.run_id = t.run_id
		WHERE r.run_id = $1 AND t.step_name = $2 AND t.task_name = $3`

	err := a.db.QueryRow(context.Background(), query, runID, stepName, taskName).Scan(&repoOwner, &repoName, &commitSHA, &checkRunID, &pipelineDef, &taskIndex, &totalTasks, &startedAt, &finishedAt)
	if err != nil || !repoOwner.Valid || !checkRunID.Valid {
		log.Warn().Str("run_id", runID).Err(err).Msg("Not a Git-triggered run with a check ID, skipping task status update.")
		return
	}

	var pipeline models.Pipeline
	var dependsOn []string
	if pipelineDef.Valid {
		if err := yaml.Unmarshal([]byte(pipelineDef.String), &pipeline); err == nil {
			for _, step := range pipeline.Steps {
				if step.GetName() == stepName {
					for _, task := range step.GetTasks() {
						if task.Name == taskName {
							dependsOn = task.DependsOn
							break
						}
					}
					break
				}
			}
		}
	}

	err = a.gitBotClient().NotifyTaskStatus(gitbot.TaskStatusRequest{
		RunID:      runID,
		RepoOwner:  repoOwner.String,
		RepoName:   repoName.String,
		CheckRunID: checkRunID.Int64,
		CommitSHA:  commitSHA.String,
		StepName:   stepName,
		TaskName:   taskName,
		TaskStatus: taskStatus,
		TaskIndex:  taskIndex,
		TotalTasks: totalTasks,
		DependsOn:  dependsOn,
		StartedAt:  startedAt.Time,
		FinishedAt: finishedAt.Time,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to notify git-bot of task status")
	}
}

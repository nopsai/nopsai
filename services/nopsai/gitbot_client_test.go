package nopsai

import (
	"errors"
	"testing"

	"nopsai/pkg/models"
)

func TestRequestGitBotFileUsesInjectedGitProvider(t *testing.T) {
	provider := &fakeGitProvider{
		fileContent: "name: injected",
	}
	app := &App{gitProvider: provider}

	got, err := app.requestGitBotFile("acme", "service", "main", "pipelines/build.yaml", errPipelineNotFound)
	if err != nil {
		t.Fatalf("requestGitBotFile() error = %v", err)
	}
	if got != "name: injected" {
		t.Fatalf("requestGitBotFile() = %q, want injected content", got)
	}
	if provider.fileOwner != "acme" || provider.fileRepo != "service" || provider.fileRef != "main" || provider.filePath != "pipelines/build.yaml" {
		t.Fatalf("provider file request = %s/%s@%s:%s", provider.fileOwner, provider.fileRepo, provider.fileRef, provider.filePath)
	}
}

type fakeGitProvider struct {
	fileOwner                string
	fileRepo                 string
	fileRef                  string
	filePath                 string
	fileContent              string
	directories              map[string]map[string]string
	directoryErr             error
	ensureRepoAccessibleErr  error
	repositoryInstallationID string
	repositories             []GitHubInstalledRepository
	repositoriesErr          error
}

func (f *fakeGitProvider) File(owner, repo, ref, path string, notFoundErr error) (string, error) {
	f.fileOwner = owner
	f.fileRepo = repo
	f.fileRef = ref
	f.filePath = path
	if f.fileContent == "" {
		return "", notFoundErr
	}
	return f.fileContent, nil
}

func (f *fakeGitProvider) Directory(owner, repo, ref, path string) (map[string]string, error) {
	if f.directoryErr != nil {
		return nil, f.directoryErr
	}
	if f.directories == nil {
		return map[string]string{}, nil
	}
	files := f.directories[path]
	if files == nil {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(files))
	for filePath, content := range files {
		out[filePath] = content
	}
	return out, nil
}

func (f *fakeGitProvider) CommitFiles(owner, repo, baseRef, branch, message string, files []GitCommitFile) (GitCommitFilesResponse, error) {
	return GitCommitFilesResponse{}, errors.New("not implemented")
}

func (f *fakeGitProvider) BranchHasOpenPullRequest(owner, repo, branch string) (bool, error) {
	return false, errors.New("not implemented")
}

func (f *fakeGitProvider) EnsureRepoAccessible(owner, repo string) error {
	return f.ensureRepoAccessibleErr
}

func (f *fakeGitProvider) ListInstallationRepositories(installationID string) ([]GitHubInstalledRepository, error) {
	f.repositoryInstallationID = installationID
	if f.repositoriesErr != nil {
		return nil, f.repositoriesErr
	}
	return append([]GitHubInstalledRepository(nil), f.repositories...), nil
}

func (f *fakeGitProvider) Pipeline(owner, repo, ref string, source models.PipelineSource, notFoundErr error) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeGitProvider) FindSuiteCheckRun(owner, repo string, suiteID int64, commitSHA string) (*SuiteCheckRunResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeGitProvider) CreateCheckRun(owner, repo, ref string, pipelineDef []byte, pipelineSource string) (int64, error) {
	return 0, errors.New("not implemented")
}

func (f *fakeGitProvider) CreateChildCheckRun(owner, repo, ref, parentName, includeName string, pipelineDef []byte) (int64, error) {
	return 0, errors.New("not implemented")
}

func (f *fakeGitProvider) InitializeCheckRun(owner, repo string, checkRunID int64, pipelineDef []byte, pipelineName string) error {
	return errors.New("not implemented")
}

func (f *fakeGitProvider) CancelStaleCheckRuns(owner, repo, beforeSHA string) error {
	return errors.New("not implemented")
}

func (f *fakeGitProvider) NotifyFinalStatus(req GitFinalStatusRequest) error {
	return errors.New("not implemented")
}

func (f *fakeGitProvider) NotifyTaskStatus(req GitTaskStatusRequest) error {
	return errors.New("not implemented")
}

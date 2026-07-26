package service

import (
	"context"
	"net/http"
)

const githubIntegrationUnavailableMessage = "GitHub integration is not configured"

type unavailableRepositoryProvider struct{}

func (unavailableRepositoryProvider) FetchFile(context.Context, FileContentRequest) (string, error) {
	return "", githubIntegrationUnavailableError()
}

func (unavailableRepositoryProvider) FetchDirectory(context.Context, DirectoryContentsRequest) (map[string]string, error) {
	return nil, githubIntegrationUnavailableError()
}

func (unavailableRepositoryProvider) CheckAccess(context.Context, RepositoryAccessRequest) (RepositoryAccessResponse, error) {
	return RepositoryAccessResponse{}, githubIntegrationUnavailableError()
}

func (unavailableRepositoryProvider) BranchHasOpenPR(context.Context, BranchPROpenRequest) (BranchPROpenResponse, error) {
	return BranchPROpenResponse{}, githubIntegrationUnavailableError()
}

func (unavailableRepositoryProvider) ListInstalled(context.Context) ([]InstalledRepository, error) {
	return nil, githubIntegrationUnavailableError()
}

func (unavailableRepositoryProvider) ListInstalledForInstallation(context.Context, int64) ([]InstalledRepository, error) {
	return nil, githubIntegrationUnavailableError()
}

func (unavailableRepositoryProvider) FetchPipeline(context.Context, PipelineContentRequest) (string, error) {
	return "", githubIntegrationUnavailableError()
}

type unavailableChecksProvider struct{}

func (unavailableChecksProvider) CreateQueued(context.Context, createQueuedCheckRunRequest) (int64, error) {
	return 0, githubIntegrationUnavailableError()
}

func (unavailableChecksProvider) MarkInProgress(context.Context, checkRunProgressUpdate) error {
	return githubIntegrationUnavailableError()
}

func (unavailableChecksProvider) Conclude(context.Context, checkRunConclusionUpdate) error {
	return githubIntegrationUnavailableError()
}

func (unavailableChecksProvider) ListForRef(context.Context, string, string, string) ([]checkRunSummary, error) {
	return nil, githubIntegrationUnavailableError()
}

func githubIntegrationUnavailableError() error {
	return providerError{
		Status:  http.StatusServiceUnavailable,
		Message: githubIntegrationUnavailableMessage,
	}
}

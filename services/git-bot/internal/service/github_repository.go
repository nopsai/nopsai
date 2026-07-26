package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"nopsai/pkg/httpapi"

	"github.com/google/go-github/v53/github"
)

type repositoryProvider interface {
	FetchFile(context.Context, FileContentRequest) (string, error)
	FetchDirectory(context.Context, DirectoryContentsRequest) (map[string]string, error)
	CheckAccess(context.Context, RepositoryAccessRequest) (RepositoryAccessResponse, error)
	BranchHasOpenPR(context.Context, BranchPROpenRequest) (BranchPROpenResponse, error)
	ListInstalled(context.Context) ([]InstalledRepository, error)
	ListInstalledForInstallation(context.Context, int64) ([]InstalledRepository, error)
	FetchPipeline(context.Context, PipelineContentRequest) (string, error)
}

type providerError struct {
	Status  int
	Message string
	Err     error
}

func (e providerError) Error() string {
	if e.Err == nil {
		return e.Message
	}
	return e.Message + ": " + e.Err.Error()
}

func (e providerError) Unwrap() error {
	return e.Err
}

type githubRepositoryProvider struct {
	resolver GitHubClientResolver
}

func newGitHubRepositoryProvider(resolver GitHubClientResolver) repositoryProvider {
	return githubRepositoryProvider{resolver: resolver}
}

func (p githubRepositoryProvider) FetchFile(ctx context.Context, req FileContentRequest) (string, error) {
	client, _, err := p.resolver.ClientForRepository(ctx, req.Owner, req.Repo)
	if err != nil {
		return "", err
	}
	fileContent, _, _, err := client.Repositories.GetContents(
		ctx,
		req.Owner,
		req.Repo,
		req.Path,
		&github.RepositoryContentGetOptions{Ref: req.Ref},
	)
	if err != nil {
		if isGitHubStatus(err, http.StatusNotFound) {
			return "", providerError{Status: http.StatusNotFound, Message: "file not found", Err: err}
		}
		return "", providerError{Status: http.StatusInternalServerError, Message: "failed to fetch file", Err: err}
	}
	if fileContent == nil {
		return "", providerError{Status: http.StatusNotFound, Message: "file not found"}
	}
	content, err := fileContent.GetContent()
	if err != nil {
		return "", providerError{Status: http.StatusInternalServerError, Message: "failed to decode file", Err: err}
	}
	return content, nil
}

func (p githubRepositoryProvider) FetchDirectory(ctx context.Context, req DirectoryContentsRequest) (map[string]string, error) {
	client, _, err := p.resolver.ClientForRepository(ctx, req.Owner, req.Repo)
	if err != nil {
		return nil, err
	}
	files := make(map[string]string)
	if err := p.collectRepositoryContents(ctx, client, req.Owner, req.Repo, strings.TrimPrefix(req.Path, "/"), req.Ref, files); err != nil {
		if isGitHubStatus(err, http.StatusNotFound) {
			return nil, providerError{Status: http.StatusNotFound, Message: "path not found", Err: err}
		}
		return nil, providerError{Status: http.StatusInternalServerError, Message: "failed to fetch repository contents", Err: err}
	}
	return files, nil
}

func (p githubRepositoryProvider) CheckAccess(ctx context.Context, req RepositoryAccessRequest) (RepositoryAccessResponse, error) {
	client, _, err := p.resolver.ClientForRepository(ctx, req.Owner, req.Repo)
	if err != nil {
		return RepositoryAccessResponse{}, err
	}
	repo, resp, err := client.Repositories.Get(ctx, req.Owner, req.Repo)
	if err != nil {
		var ghErr *github.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Response != nil {
			switch ghErr.Response.StatusCode {
			case http.StatusNotFound:
				return RepositoryAccessResponse{}, providerError{Status: http.StatusNotFound, Message: "repository not found or Git Bot not installed", Err: err}
			case http.StatusForbidden:
				return RepositoryAccessResponse{}, providerError{Status: http.StatusForbidden, Message: "access to repository forbidden for Git Bot", Err: err}
			}
		}
		statusCode := http.StatusInternalServerError
		if resp != nil {
			statusCode = resp.StatusCode
		}
		message := "failed to verify repository access"
		if ghErr != nil && ghErr.Message != "" {
			message = fmt.Sprintf("%s: %s", message, ghErr.Message)
		} else {
			message = fmt.Sprintf("%s: %v", message, err)
		}
		return RepositoryAccessResponse{}, providerError{Status: statusCode, Message: message, Err: err}
	}

	defaultBranch := ""
	if repo != nil && repo.DefaultBranch != nil {
		defaultBranch = repo.GetDefaultBranch()
	}
	return RepositoryAccessResponse{Accessible: true, DefaultBranch: defaultBranch}, nil
}

func (p githubRepositoryProvider) BranchHasOpenPR(ctx context.Context, req BranchPROpenRequest) (BranchPROpenResponse, error) {
	client, _, err := p.resolver.ClientForRepository(ctx, req.Owner, req.Repo)
	if err != nil {
		return BranchPROpenResponse{}, err
	}
	options := &github.PullRequestListOptions{
		State: "open",
		Head:  fmt.Sprintf("%s:%s", req.Owner, req.Branch),
		ListOptions: github.ListOptions{
			PerPage: 1,
		},
	}
	prs, _, err := client.PullRequests.List(ctx, req.Owner, req.Repo, options)
	if err != nil {
		return BranchPROpenResponse{}, providerError{Status: http.StatusInternalServerError, Message: "failed to check pull requests", Err: err}
	}
	return BranchPROpenResponse{HasOpenPR: len(prs) > 0}, nil
}

func (p githubRepositoryProvider) ListInstalled(ctx context.Context) ([]InstalledRepository, error) {
	// Preserve the legacy endpoint by aggregating repositories from all enabled
	// registered installations visible to the resolver.
	installations, err := p.installations(ctx)
	if err != nil {
		return nil, err
	}
	var repositories []InstalledRepository
	for _, installation := range installations {
		if !installation.Enabled {
			continue
		}
		next, err := p.ListInstalledForInstallation(ctx, installation.InstallationID)
		if err != nil {
			return nil, err
		}
		repositories = append(repositories, next...)
	}
	return repositories, nil
}

func (p githubRepositoryProvider) ListInstalledForInstallation(ctx context.Context, installationID int64) ([]InstalledRepository, error) {
	client, _, err := p.resolver.ClientForInstallation(ctx, installationID)
	if err != nil {
		return nil, err
	}
	var repositories []InstalledRepository
	opts := &github.ListOptions{PerPage: 100}
	for {
		result, resp, err := client.Apps.ListRepos(ctx, opts)
		if err != nil {
			return nil, providerError{Status: http.StatusBadGateway, Message: "failed to list installation repositories", Err: err}
		}
		for _, repo := range result.Repositories {
			owner := ""
			if repo.Owner != nil {
				owner = repo.Owner.GetLogin()
			}
			repositories = append(repositories, InstalledRepository{
				ID:            repo.GetID(),
				FullName:      repo.GetFullName(),
				Owner:         owner,
				Name:          repo.GetName(),
				Private:       repo.GetPrivate(),
				DefaultBranch: repo.GetDefaultBranch(),
			})
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return repositories, nil
}

func (p githubRepositoryProvider) FetchPipeline(ctx context.Context, req PipelineContentRequest) (string, error) {
	if strings.HasPrefix(req.Source.Path, "http://") || strings.HasPrefix(req.Source.Path, "https://") {
		return "", providerError{Status: http.StatusBadRequest, Message: "remote pipeline URLs are no longer supported"}
	}
	if req.Source.Path == "" {
		return "", providerError{Status: http.StatusBadRequest, Message: "pipeline source must include a path"}
	}
	content, err := p.FetchFile(ctx, FileContentRequest{
		Owner: req.Owner,
		Repo:  req.Repo,
		Ref:   req.Ref,
		Path:  req.Source.Path,
	})
	if err != nil {
		var providerErr providerError
		if errors.As(err, &providerErr) && providerErr.Message == "failed to decode file" {
			return "", providerError{Status: http.StatusInternalServerError, Message: "failed to decode pipeline file", Err: err}
		}
		if isGitHubStatus(err, http.StatusNotFound) {
			return "", providerError{Status: http.StatusNotFound, Message: "pipeline file not found", Err: err}
		}
		return "", providerError{Status: http.StatusInternalServerError, Message: "failed to fetch pipeline file", Err: err}
	}
	return content, nil
}

func (p githubRepositoryProvider) collectRepositoryContents(ctx context.Context, client *github.Client, owner, repo, path, ref string, results map[string]string) error {
	fileContent, dirContents, _, err := client.Repositories.GetContents(
		ctx,
		owner,
		repo,
		path,
		&github.RepositoryContentGetOptions{Ref: ref},
	)
	if err != nil {
		return err
	}

	if fileContent != nil {
		content, err := fileContent.GetContent()
		if err != nil {
			return err
		}
		results[fileContent.GetPath()] = content
		return nil
	}

	for _, entry := range dirContents {
		entryPath := entry.GetPath()

		switch entry.GetType() {
		case "dir":
			if err := p.collectRepositoryContents(ctx, client, owner, repo, entryPath, ref, results); err != nil {
				return err
			}
		case "file":
			fileContent, _, _, err := client.Repositories.GetContents(
				ctx,
				owner,
				repo,
				entryPath,
				&github.RepositoryContentGetOptions{Ref: ref},
			)
			if err != nil {
				return err
			}
			if fileContent == nil {
				continue
			}
			content, err := fileContent.GetContent()
			if err != nil {
				return err
			}
			results[entryPath] = content
		}
	}
	return nil
}

func (p githubRepositoryProvider) installations(ctx context.Context) ([]GitHubInstallation, error) {
	resolver, ok := p.resolver.(*githubClientResolver)
	if !ok {
		return nil, githubIntegrationUnavailableError()
	}
	if err := resolver.refreshIfNeeded(ctx, true); err != nil {
		return nil, err
	}
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	out := make([]GitHubInstallation, 0, len(resolver.byID))
	for _, installation := range resolver.byID {
		out = append(out, installation)
	}
	return out, nil
}

func writeProviderError(w http.ResponseWriter, err error, fallbackMessage string) {
	var providerErr providerError
	if errors.As(err, &providerErr) {
		_ = httpapi.WriteJSONError(w, providerErr.Status, providerErr.Message)
		return
	}
	_ = httpapi.WriteJSONError(w, http.StatusInternalServerError, fallbackMessage)
}

func isGitHubStatus(err error, statusCode int) bool {
	var providerErr providerError
	if errors.As(err, &providerErr) {
		return providerErr.Status == statusCode
	}
	var ghErr *github.ErrorResponse
	return errors.As(err, &ghErr) && ghErr.Response != nil && ghErr.Response.StatusCode == statusCode
}

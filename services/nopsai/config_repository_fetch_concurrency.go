package nopsai

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"
)

const (
	configRepositoryDirectoryFetchConcurrency = 4
	configRepositoryProviderFileConcurrency   = 6
)

type configRepositoryDirectoryRequest struct {
	path     string
	resource string
}

func fetchConfigRepositoryDirectories(ctx context.Context, reader configSyncGitReader, branch string, requests []configRepositoryDirectoryRequest) ([]map[string]string, error) {
	results := make([]map[string]string, len(requests))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(configRepositoryDirectoryFetchConcurrency)
	for idx, request := range requests {
		idx, request := idx, request
		group.Go(func() error {
			files, err := reader.Directory(groupCtx, branch, request.path)
			if err != nil {
				return fmt.Errorf("failed to fetch %s: %w", request.resource, err)
			}
			if files == nil {
				files = map[string]string{}
			}
			results[idx] = files
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}

type configRepositoryOptionalFileRequest struct {
	path        string
	resource    string
	notFoundErr error
}

type configRepositoryOptionalFileResult struct {
	path    string
	content string
	found   bool
}

func fetchConfigRepositoryOptionalFiles(ctx context.Context, reader configSyncGitReader, branch string, requests []configRepositoryOptionalFileRequest) ([]configRepositoryOptionalFileResult, error) {
	results := make([]configRepositoryOptionalFileResult, len(requests))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(configRepositoryDirectoryFetchConcurrency)
	for idx, request := range requests {
		idx, request := idx, request
		group.Go(func() error {
			content, err := reader.File(groupCtx, branch, request.path, request.notFoundErr)
			if err != nil {
				if request.notFoundErr != nil && errors.Is(err, request.notFoundErr) {
					results[idx] = configRepositoryOptionalFileResult{path: request.path}
					return nil
				}
				return fmt.Errorf("failed to fetch %s '%s': %w", request.resource, request.path, err)
			}
			results[idx] = configRepositoryOptionalFileResult{
				path:    request.path,
				content: content,
				found:   true,
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}

type configRepositoryProviderFileRequest struct {
	path  string
	fetch func(context.Context) (string, error)
}

func fetchConfigRepositoryProviderFiles(ctx context.Context, requests []configRepositoryProviderFileRequest) (map[string]string, error) {
	files := make(map[string]string, len(requests))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(configRepositoryProviderFileConcurrency)
	var mu sync.Mutex
	for _, request := range requests {
		request := request
		group.Go(func() error {
			content, err := request.fetch(groupCtx)
			if err != nil {
				return err
			}
			mu.Lock()
			files[request.path] = content
			mu.Unlock()
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return files, nil
}

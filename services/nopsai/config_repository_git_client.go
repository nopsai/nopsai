package nopsai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/internal/credentials"
)

type configRepositoryGitContentClient interface {
	EnsureAccessible(ctx context.Context) error
	Directory(ctx context.Context, ref, path string) (map[string]string, error)
	File(ctx context.Context, ref, path string, notFoundErr error) (string, error)
	CommitFiles(ctx context.Context, baseRef, branch, message string, files []GitCommitFile) (GitCommitFilesResponse, error)
}

type gitBotConfigRepositoryClient struct {
	app      *App
	identity configsync.RepositoryIdentity
}

func (c gitBotConfigRepositoryClient) EnsureAccessible(_ context.Context) error {
	return c.app.ensureConfigRepoAccessible(c.identity.Owner, c.identity.Repo)
}

func (c gitBotConfigRepositoryClient) Directory(_ context.Context, ref, path string) (map[string]string, error) {
	return c.app.requestGitBotDirectory(c.identity.Owner, c.identity.Repo, ref, path)
}

func (c gitBotConfigRepositoryClient) File(_ context.Context, ref, path string, notFoundErr error) (string, error) {
	return c.app.requestGitBotFile(c.identity.Owner, c.identity.Repo, ref, path, notFoundErr)
}

func (c gitBotConfigRepositoryClient) CommitFiles(_ context.Context, baseRef, branch, message string, files []GitCommitFile) (GitCommitFilesResponse, error) {
	return c.app.requestGitBotCommitFiles(c.identity.Owner, c.identity.Repo, baseRef, branch, message, files)
}

type tokenConfigRepositoryClient struct {
	provider   string
	identity   configsync.RepositoryIdentity
	token      string
	httpClient *http.Client
}

func (a *App) newConfigRepositoryGitContentClient(ctx context.Context, repo models.ConfigRepository) (configRepositoryGitContentClient, configsync.RepositoryIdentity, error) {
	provider, err := configsync.NormalizeRepositoryProvider(repo.Provider, repo.RepoURL)
	if err != nil {
		return nil, configsync.RepositoryIdentity{}, err
	}
	identity, err := configsync.ParseRepositoryIdentity(repo.RepoURL, provider)
	if err != nil {
		return nil, configsync.RepositoryIdentity{}, err
	}
	if provider == models.ConfigRepositoryProviderGitHub && strings.TrimSpace(repo.CredentialRef) == "" {
		return gitBotConfigRepositoryClient{app: a, identity: identity}, identity, nil
	}
	token, err := a.resolveConfigRepositoryGitToken(ctx, repo)
	if err != nil {
		return nil, identity, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, identity, fmt.Errorf("credential_ref is required for %s config repository access", provider)
	}
	client := a.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	return tokenConfigRepositoryClient{
		provider:   provider,
		identity:   identity,
		token:      token,
		httpClient: client,
	}, identity, nil
}

func (a *App) commitConfigRepositoryFiles(ctx context.Context, repo models.ConfigRepository, message string, files []GitCommitFile) (GitCommitFilesResponse, error) {
	client, _, err := a.newConfigRepositoryGitContentClient(ctx, repo)
	if err != nil {
		return GitCommitFilesResponse{}, err
	}
	return client.CommitFiles(ctx, configRepositoryBranch(repo.Branch), strings.TrimSpace(repo.WriteBranch), message, files)
}

func (a *App) resolveConfigRepositoryGitToken(ctx context.Context, repo models.ConfigRepository) (string, error) {
	ref := strings.TrimSpace(repo.CredentialRef)
	if ref == "" {
		return "", nil
	}
	subjectID := fmt.Sprintf("%s/%s", strings.TrimSpace(repo.ScopeType), strings.Trim(strings.TrimSpace(repo.ScopeID), "/"))
	if repo.ID > 0 {
		subjectID = fmt.Sprintf("%d", repo.ID)
	}
	return a.resolveCredentialText(ctx, ref, credentials.Purpose{
		ConsumerService: "nopsai",
		Operation:       "config_repository.git",
		SubjectType:     "config_repository",
		SubjectID:       subjectID,
	})
}

func configRepositoryBranch(branch string) string {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "main"
	}
	return branch
}

func (c tokenConfigRepositoryClient) EnsureAccessible(ctx context.Context) error {
	var endpoint string
	switch c.provider {
	case models.ConfigRepositoryProviderGitHub:
		endpoint = c.githubRepoEndpoint()
	case models.ConfigRepositoryProviderGitLab:
		endpoint = c.gitlabProjectEndpoint()
	case models.ConfigRepositoryProviderBitbucket:
		endpoint = c.bitbucketRepoEndpoint()
	case models.ConfigRepositoryProviderGitea:
		endpoint = c.giteaRepoEndpoint()
	default:
		return fmt.Errorf("unsupported config repository provider %q", c.provider)
	}
	status, _, body, err := c.do(ctx, http.MethodGet, endpoint, nil, "")
	if err != nil {
		return err
	}
	if statusOK(status, http.StatusOK) {
		return nil
	}
	return c.statusError("verify repository access", status, body)
}

func (c tokenConfigRepositoryClient) Directory(ctx context.Context, ref, path string) (map[string]string, error) {
	ref = configRepositoryBranch(ref)
	switch c.provider {
	case models.ConfigRepositoryProviderGitHub:
		return c.githubDirectory(ctx, ref, path)
	case models.ConfigRepositoryProviderGitLab:
		return c.gitlabDirectory(ctx, ref, path)
	case models.ConfigRepositoryProviderBitbucket:
		return c.bitbucketDirectory(ctx, ref, path)
	case models.ConfigRepositoryProviderGitea:
		return c.giteaDirectory(ctx, ref, path)
	default:
		return nil, fmt.Errorf("unsupported config repository provider %q", c.provider)
	}
}

func (c tokenConfigRepositoryClient) File(ctx context.Context, ref, path string, notFoundErr error) (string, error) {
	ref = configRepositoryBranch(ref)
	switch c.provider {
	case models.ConfigRepositoryProviderGitHub:
		return c.githubFile(ctx, ref, path, notFoundErr)
	case models.ConfigRepositoryProviderGitLab:
		return c.gitlabFile(ctx, ref, path, notFoundErr)
	case models.ConfigRepositoryProviderBitbucket:
		return c.bitbucketFile(ctx, ref, path, notFoundErr)
	case models.ConfigRepositoryProviderGitea:
		return c.giteaFile(ctx, ref, path, notFoundErr)
	default:
		return "", fmt.Errorf("unsupported config repository provider %q", c.provider)
	}
}

func (c tokenConfigRepositoryClient) CommitFiles(ctx context.Context, baseRef, branch, message string, files []GitCommitFile) (GitCommitFilesResponse, error) {
	baseRef = configRepositoryBranch(baseRef)
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return GitCommitFilesResponse{}, fmt.Errorf("write_branch is required")
	}
	switch c.provider {
	case models.ConfigRepositoryProviderGitHub:
		return c.githubCommitFiles(ctx, baseRef, branch, message, files)
	case models.ConfigRepositoryProviderGitLab:
		return c.gitlabCommitFiles(ctx, baseRef, branch, message, files)
	case models.ConfigRepositoryProviderBitbucket:
		return c.bitbucketCommitFiles(ctx, baseRef, branch, message, files)
	case models.ConfigRepositoryProviderGitea:
		return c.giteaCommitFiles(ctx, baseRef, branch, message, files)
	default:
		return GitCommitFilesResponse{}, fmt.Errorf("unsupported config repository provider %q", c.provider)
	}
}

func (c tokenConfigRepositoryClient) githubDirectory(ctx context.Context, ref, dir string) (map[string]string, error) {
	files := map[string]string{}
	if err := c.githubWalkDirectory(ctx, ref, dir, files); err != nil {
		return nil, err
	}
	return files, nil
}

func (c tokenConfigRepositoryClient) githubWalkDirectory(ctx context.Context, ref, dir string, files map[string]string) error {
	status, _, body, err := c.do(ctx, http.MethodGet, c.githubContentsEndpoint(dir, ref), nil, "")
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if !statusOK(status, http.StatusOK) {
		return c.statusError("fetch GitHub contents", status, body)
	}
	var entries []githubContentEntry
	if isJSONArray(body) {
		if err := json.Unmarshal(body, &entries); err != nil {
			return err
		}
	} else {
		var entry githubContentEntry
		if err := json.Unmarshal(body, &entry); err != nil {
			return err
		}
		entries = []githubContentEntry{entry}
	}
	for _, entry := range entries {
		switch entry.Type {
		case "dir":
			if err := c.githubWalkDirectory(ctx, ref, entry.Path, files); err != nil {
				return err
			}
		case "file":
			content, err := c.githubFile(ctx, ref, entry.Path, fmt.Errorf("file not found"))
			if err != nil {
				return err
			}
			files[entry.Path] = content
		}
	}
	return nil
}

func (c tokenConfigRepositoryClient) githubFile(ctx context.Context, ref, filePath string, notFoundErr error) (string, error) {
	status, _, body, err := c.do(ctx, http.MethodGet, c.githubContentsEndpoint(filePath, ref), nil, "")
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound {
		return "", notFoundErr
	}
	if !statusOK(status, http.StatusOK) {
		return "", c.statusError("fetch GitHub file", status, body)
	}
	var entry githubContentEntry
	if err := json.Unmarshal(body, &entry); err != nil {
		return "", err
	}
	if entry.Type != "" && entry.Type != "file" {
		return "", fmt.Errorf("GitHub path %q is not a file", filePath)
	}
	return decodeProviderContent(entry.Content, entry.Encoding)
}

func (c tokenConfigRepositoryClient) githubCommitFiles(ctx context.Context, baseRef, branch, message string, files []GitCommitFile) (GitCommitFilesResponse, error) {
	headSHA, branchExists, err := c.githubBranchHead(ctx, branch)
	if err != nil {
		return GitCommitFilesResponse{}, err
	}
	if !branchExists {
		var baseExists bool
		headSHA, baseExists, err = c.githubBranchHead(ctx, baseRef)
		if err != nil {
			return GitCommitFilesResponse{}, err
		}
		if !baseExists {
			return GitCommitFilesResponse{}, fmt.Errorf("base branch %q was not found", baseRef)
		}
	}
	var commit struct {
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	if err := c.doJSON(ctx, http.MethodGet, c.githubAPIBase()+"/repos/"+escapePathSegments(c.identity.Owner)+"/"+escapePathSegments(c.identity.Repo)+"/git/commits/"+url.PathEscape(headSHA), nil, &commit, http.StatusOK); err != nil {
		return GitCommitFilesResponse{}, err
	}
	tree := make([]githubTreeEntry, 0, len(files))
	for _, file := range files {
		if file.Delete {
			tree = append(tree, githubTreeEntry{Path: file.Path, Mode: "100644", Type: "blob", SHA: nil})
			continue
		}
		var blob struct {
			SHA string `json:"sha"`
		}
		if err := c.doJSON(ctx, http.MethodPost, c.githubAPIBase()+"/repos/"+escapePathSegments(c.identity.Owner)+"/"+escapePathSegments(c.identity.Repo)+"/git/blobs", map[string]string{
			"content":  file.Content,
			"encoding": "utf-8",
		}, &blob, http.StatusCreated); err != nil {
			return GitCommitFilesResponse{}, err
		}
		sha := blob.SHA
		tree = append(tree, githubTreeEntry{Path: file.Path, Mode: "100644", Type: "blob", SHA: &sha})
	}
	var treeOut struct {
		SHA string `json:"sha"`
	}
	if err := c.doJSON(ctx, http.MethodPost, c.githubAPIBase()+"/repos/"+escapePathSegments(c.identity.Owner)+"/"+escapePathSegments(c.identity.Repo)+"/git/trees", map[string]any{
		"base_tree": commit.Tree.SHA,
		"tree":      tree,
	}, &treeOut, http.StatusCreated); err != nil {
		return GitCommitFilesResponse{}, err
	}
	var commitOut struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
	}
	if err := c.doJSON(ctx, http.MethodPost, c.githubAPIBase()+"/repos/"+escapePathSegments(c.identity.Owner)+"/"+escapePathSegments(c.identity.Repo)+"/git/commits", map[string]any{
		"message": message,
		"tree":    treeOut.SHA,
		"parents": []string{headSHA},
	}, &commitOut, http.StatusCreated); err != nil {
		return GitCommitFilesResponse{}, err
	}
	if branchExists {
		if err := c.doJSON(ctx, http.MethodPatch, c.githubAPIBase()+"/repos/"+escapePathSegments(c.identity.Owner)+"/"+escapePathSegments(c.identity.Repo)+"/git/refs/heads/"+escapePathSegments(branch), map[string]any{
			"sha":   commitOut.SHA,
			"force": false,
		}, nil, http.StatusOK); err != nil {
			return GitCommitFilesResponse{}, err
		}
	} else {
		if err := c.doJSON(ctx, http.MethodPost, c.githubAPIBase()+"/repos/"+escapePathSegments(c.identity.Owner)+"/"+escapePathSegments(c.identity.Repo)+"/git/refs", map[string]string{
			"ref": "refs/heads/" + branch,
			"sha": commitOut.SHA,
		}, nil, http.StatusCreated); err != nil {
			return GitCommitFilesResponse{}, err
		}
	}
	commitURL := strings.TrimSpace(commitOut.HTMLURL)
	if commitURL == "" && c.identity.WebURL != "" {
		commitURL = strings.TrimRight(c.identity.WebURL, "/") + "/commit/" + commitOut.SHA
	}
	return GitCommitFilesResponse{Branch: branch, CommitSHA: commitOut.SHA, CommitURL: commitURL, FilesChanged: len(files)}, nil
}

func (c tokenConfigRepositoryClient) githubBranchHead(ctx context.Context, branch string) (string, bool, error) {
	var out struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	status, _, body, err := c.do(ctx, http.MethodGet, c.githubAPIBase()+"/repos/"+escapePathSegments(c.identity.Owner)+"/"+escapePathSegments(c.identity.Repo)+"/git/ref/heads/"+escapePathSegments(branch), nil, "")
	if err != nil {
		return "", false, err
	}
	if status == http.StatusNotFound {
		return "", false, nil
	}
	if !statusOK(status, http.StatusOK) {
		return "", false, c.statusError("fetch GitHub branch", status, body)
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", false, err
	}
	if out.Object.SHA == "" {
		return "", false, fmt.Errorf("GitHub branch %q response did not include a commit SHA", branch)
	}
	return out.Object.SHA, true, nil
}

func (c tokenConfigRepositoryClient) gitlabDirectory(ctx context.Context, ref, dir string) (map[string]string, error) {
	result := map[string]string{}
	page := "1"
	for {
		values := url.Values{}
		values.Set("ref", ref)
		values.Set("path", strings.Trim(dir, "/"))
		values.Set("recursive", "true")
		values.Set("per_page", "100")
		values.Set("page", page)
		status, headers, body, err := c.do(ctx, http.MethodGet, c.gitlabProjectEndpoint()+"/repository/tree?"+values.Encode(), nil, "")
		if err != nil {
			return nil, err
		}
		if status == http.StatusNotFound {
			return result, nil
		}
		if !statusOK(status, http.StatusOK) {
			return nil, c.statusError("fetch GitLab repository tree", status, body)
		}
		var entries []gitlabTreeEntry
		if err := json.Unmarshal(body, &entries); err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.Type != "blob" {
				continue
			}
			content, err := c.gitlabFile(ctx, ref, entry.Path, fmt.Errorf("file not found"))
			if err != nil {
				return nil, err
			}
			result[entry.Path] = content
		}
		next := strings.TrimSpace(headers.Get("X-Next-Page"))
		if next == "" {
			break
		}
		page = next
	}
	return result, nil
}

func (c tokenConfigRepositoryClient) gitlabFile(ctx context.Context, ref, filePath string, notFoundErr error) (string, error) {
	values := url.Values{}
	values.Set("ref", ref)
	endpoint := c.gitlabProjectEndpoint() + "/repository/files/" + url.PathEscape(strings.Trim(filePath, "/")) + "/raw?" + values.Encode()
	status, _, body, err := c.do(ctx, http.MethodGet, endpoint, nil, "")
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound {
		return "", notFoundErr
	}
	if !statusOK(status, http.StatusOK) {
		return "", c.statusError("fetch GitLab file", status, body)
	}
	return string(body), nil
}

func (c tokenConfigRepositoryClient) gitlabCommitFiles(ctx context.Context, baseRef, branch, message string, files []GitCommitFile) (GitCommitFilesResponse, error) {
	branchExists, err := c.gitlabBranchExists(ctx, branch)
	if err != nil {
		return GitCommitFilesResponse{}, err
	}
	checkRef := branch
	if !branchExists {
		checkRef = baseRef
	}
	actions := make([]gitlabCommitAction, 0, len(files))
	for _, file := range files {
		action := "create"
		if file.Delete {
			action = "delete"
		} else {
			exists, err := c.gitlabFileExists(ctx, checkRef, file.Path)
			if err != nil {
				return GitCommitFilesResponse{}, err
			}
			if exists {
				action = "update"
			}
		}
		actions = append(actions, gitlabCommitAction{
			Action:   action,
			FilePath: file.Path,
			Content:  file.Content,
		})
	}
	payload := gitlabCommitRequest{
		Branch:        branch,
		CommitMessage: message,
		Actions:       actions,
	}
	if !branchExists && branch != baseRef {
		payload.StartBranch = baseRef
	}
	var out struct {
		ID     string `json:"id"`
		WebURL string `json:"web_url"`
	}
	if err := c.doJSON(ctx, http.MethodPost, c.gitlabProjectEndpoint()+"/repository/commits", payload, &out, http.StatusCreated, http.StatusOK); err != nil {
		return GitCommitFilesResponse{}, err
	}
	return GitCommitFilesResponse{Branch: branch, CommitSHA: out.ID, CommitURL: out.WebURL, FilesChanged: len(files)}, nil
}

func (c tokenConfigRepositoryClient) gitlabBranchExists(ctx context.Context, branch string) (bool, error) {
	status, _, body, err := c.do(ctx, http.MethodGet, c.gitlabProjectEndpoint()+"/repository/branches/"+url.PathEscape(branch), nil, "")
	if err != nil {
		return false, err
	}
	switch status {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, c.statusError("fetch GitLab branch", status, body)
	}
}

func (c tokenConfigRepositoryClient) gitlabFileExists(ctx context.Context, ref, filePath string) (bool, error) {
	_, err := c.gitlabFile(ctx, ref, filePath, errConfigRepositoryProviderFileNotFound)
	if err == nil {
		return true, nil
	}
	if err == errConfigRepositoryProviderFileNotFound {
		return false, nil
	}
	return false, err
}

func (c tokenConfigRepositoryClient) bitbucketDirectory(ctx context.Context, ref, dir string) (map[string]string, error) {
	files := map[string]string{}
	if err := c.bitbucketWalkDirectory(ctx, ref, dir, files); err != nil {
		return nil, err
	}
	return files, nil
}

func (c tokenConfigRepositoryClient) bitbucketWalkDirectory(ctx context.Context, ref, dir string, files map[string]string) error {
	endpoint := c.bitbucketSourceEndpoint(ref, dir)
	for endpoint != "" {
		status, _, body, err := c.do(ctx, http.MethodGet, endpoint, nil, "")
		if err != nil {
			return err
		}
		if status == http.StatusNotFound {
			return nil
		}
		if !statusOK(status, http.StatusOK) {
			return c.statusError("fetch Bitbucket source", status, body)
		}
		listing, ok := parseBitbucketDirectoryListing(body)
		if !ok {
			if strings.TrimSpace(dir) != "" {
				files[strings.Trim(dir, "/")] = string(body)
			}
			return nil
		}
		for _, entry := range listing.Values {
			switch entry.Type {
			case "commit_directory":
				if err := c.bitbucketWalkDirectory(ctx, ref, entry.Path, files); err != nil {
					return err
				}
			case "commit_file":
				content, err := c.bitbucketFile(ctx, ref, entry.Path, fmt.Errorf("file not found"))
				if err != nil {
					return err
				}
				files[entry.Path] = content
			}
		}
		endpoint = strings.TrimSpace(listing.Next)
	}
	return nil
}

func (c tokenConfigRepositoryClient) bitbucketFile(ctx context.Context, ref, filePath string, notFoundErr error) (string, error) {
	status, _, body, err := c.do(ctx, http.MethodGet, c.bitbucketSourceEndpoint(ref, filePath), nil, "")
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound {
		return "", notFoundErr
	}
	if !statusOK(status, http.StatusOK) {
		return "", c.statusError("fetch Bitbucket file", status, body)
	}
	return string(body), nil
}

func (c tokenConfigRepositoryClient) bitbucketCommitFiles(ctx context.Context, baseRef, branch, message string, files []GitCommitFile) (GitCommitFilesResponse, error) {
	parentHash := ""
	if baseRef != "" && branch != baseRef {
		branchHead, branchExists, err := c.bitbucketBranchHead(ctx, branch)
		if err != nil {
			return GitCommitFilesResponse{}, err
		}
		if !branchExists {
			baseHead, baseExists, err := c.bitbucketBranchHead(ctx, baseRef)
			if err != nil {
				return GitCommitFilesResponse{}, err
			}
			if !baseExists {
				return GitCommitFilesResponse{}, fmt.Errorf("base branch %q was not found", baseRef)
			}
			parentHash = baseHead
		} else {
			parentHash = branchHead
		}
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("message", message); err != nil {
		return GitCommitFilesResponse{}, err
	}
	if err := writer.WriteField("branch", branch); err != nil {
		return GitCommitFilesResponse{}, err
	}
	if parentHash != "" {
		_ = writer.WriteField("parents", parentHash)
	}
	for _, file := range files {
		if file.Delete {
			if err := writer.WriteField("files", file.Path); err != nil {
				return GitCommitFilesResponse{}, err
			}
			continue
		}
		part, err := writer.CreateFormFile(file.Path, filepath.Base(file.Path))
		if err != nil {
			return GitCommitFilesResponse{}, err
		}
		if _, err := io.WriteString(part, file.Content); err != nil {
			return GitCommitFilesResponse{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return GitCommitFilesResponse{}, err
	}
	status, _, responseBody, err := c.do(ctx, http.MethodPost, c.bitbucketRepoEndpoint()+"/src", body.Bytes(), writer.FormDataContentType())
	if err != nil {
		return GitCommitFilesResponse{}, err
	}
	if !statusOK(status, http.StatusOK, http.StatusCreated) {
		return GitCommitFilesResponse{}, c.statusError("commit Bitbucket files", status, responseBody)
	}
	var out struct {
		Hash  string `json:"hash"`
		Links struct {
			HTML struct {
				Href string `json:"href"`
			} `json:"html"`
		} `json:"links"`
	}
	if len(bytes.TrimSpace(responseBody)) > 0 {
		_ = json.Unmarshal(responseBody, &out)
	}
	return GitCommitFilesResponse{Branch: branch, CommitSHA: out.Hash, CommitURL: out.Links.HTML.Href, FilesChanged: len(files)}, nil
}

func (c tokenConfigRepositoryClient) bitbucketBranchHead(ctx context.Context, branch string) (string, bool, error) {
	endpoint := c.bitbucketRepoEndpoint() + "/refs/branches/" + url.PathEscape(branch)
	status, _, body, err := c.do(ctx, http.MethodGet, endpoint, nil, "")
	if err != nil {
		return "", false, err
	}
	if status == http.StatusNotFound {
		return "", false, nil
	}
	if !statusOK(status, http.StatusOK) {
		return "", false, c.statusError("fetch Bitbucket branch", status, body)
	}
	var out struct {
		Target struct {
			Hash string `json:"hash"`
		} `json:"target"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", false, err
	}
	return out.Target.Hash, out.Target.Hash != "", nil
}

func (c tokenConfigRepositoryClient) giteaDirectory(ctx context.Context, ref, dir string) (map[string]string, error) {
	files := map[string]string{}
	if err := c.giteaWalkDirectory(ctx, ref, dir, files); err != nil {
		return nil, err
	}
	return files, nil
}

func (c tokenConfigRepositoryClient) giteaWalkDirectory(ctx context.Context, ref, dir string, files map[string]string) error {
	status, _, body, err := c.do(ctx, http.MethodGet, c.giteaContentsEndpoint(dir, ref), nil, "")
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if !statusOK(status, http.StatusOK) {
		return c.statusError("fetch Gitea contents", status, body)
	}
	var entries []giteaContentEntry
	if isJSONArray(body) {
		if err := json.Unmarshal(body, &entries); err != nil {
			return err
		}
	} else {
		var entry giteaContentEntry
		if err := json.Unmarshal(body, &entry); err != nil {
			return err
		}
		entries = []giteaContentEntry{entry}
	}
	for _, entry := range entries {
		switch entry.Type {
		case "dir":
			if err := c.giteaWalkDirectory(ctx, ref, entry.Path, files); err != nil {
				return err
			}
		case "file":
			content, err := c.giteaFile(ctx, ref, entry.Path, fmt.Errorf("file not found"))
			if err != nil {
				return err
			}
			files[entry.Path] = content
		}
	}
	return nil
}

func (c tokenConfigRepositoryClient) giteaFile(ctx context.Context, ref, filePath string, notFoundErr error) (string, error) {
	status, _, body, err := c.do(ctx, http.MethodGet, c.giteaContentsEndpoint(filePath, ref), nil, "")
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound {
		return "", notFoundErr
	}
	if !statusOK(status, http.StatusOK) {
		return "", c.statusError("fetch Gitea file", status, body)
	}
	var entry giteaContentEntry
	if err := json.Unmarshal(body, &entry); err != nil {
		return "", err
	}
	if entry.Type != "" && entry.Type != "file" {
		return "", fmt.Errorf("Gitea path %q is not a file", filePath)
	}
	return decodeProviderContent(entry.Content, "base64")
}

func (c tokenConfigRepositoryClient) giteaCommitFiles(ctx context.Context, baseRef, branch, message string, files []GitCommitFile) (GitCommitFilesResponse, error) {
	branchExists, err := c.giteaBranchExists(ctx, branch)
	if err != nil {
		return GitCommitFilesResponse{}, err
	}
	operationBranch := branch
	if !branchExists {
		operationBranch = baseRef
	}
	changed := 0
	lastCommitSHA := ""
	lastCommitURL := ""
	for _, file := range files {
		entry, exists, err := c.giteaContentMetadata(ctx, operationBranch, file.Path)
		if err != nil {
			return GitCommitFilesResponse{}, err
		}
		if file.Delete && !exists {
			continue
		}
		payload := giteaContentWriteRequest{
			Message: message,
			Branch:  operationBranch,
		}
		if !branchExists && branch != baseRef {
			payload.NewBranch = branch
		}
		if exists {
			payload.SHA = entry.SHA
		}
		method := http.MethodPost
		if file.Delete {
			method = http.MethodDelete
		} else {
			payload.Content = base64.StdEncoding.EncodeToString([]byte(file.Content))
			if exists {
				method = http.MethodPut
			}
		}
		var out giteaContentWriteResponse
		if err := c.doJSON(ctx, method, c.giteaContentsEndpoint(file.Path, ""), payload, &out, http.StatusOK, http.StatusCreated); err != nil {
			return GitCommitFilesResponse{}, err
		}
		changed++
		if out.Commit.SHA != "" {
			lastCommitSHA = out.Commit.SHA
		}
		if out.Commit.HTMLURL != "" {
			lastCommitURL = out.Commit.HTMLURL
		}
		branchExists = true
		operationBranch = branch
	}
	return GitCommitFilesResponse{Branch: branch, CommitSHA: lastCommitSHA, CommitURL: lastCommitURL, FilesChanged: changed}, nil
}

func (c tokenConfigRepositoryClient) giteaBranchExists(ctx context.Context, branch string) (bool, error) {
	status, _, body, err := c.do(ctx, http.MethodGet, c.giteaRepoEndpoint()+"/branches/"+url.PathEscape(branch), nil, "")
	if err != nil {
		return false, err
	}
	switch status {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, c.statusError("fetch Gitea branch", status, body)
	}
}

func (c tokenConfigRepositoryClient) giteaContentMetadata(ctx context.Context, ref, filePath string) (giteaContentEntry, bool, error) {
	status, _, body, err := c.do(ctx, http.MethodGet, c.giteaContentsEndpoint(filePath, ref), nil, "")
	if err != nil {
		return giteaContentEntry{}, false, err
	}
	if status == http.StatusNotFound {
		return giteaContentEntry{}, false, nil
	}
	if !statusOK(status, http.StatusOK) {
		return giteaContentEntry{}, false, c.statusError("fetch Gitea file metadata", status, body)
	}
	var entry giteaContentEntry
	if err := json.Unmarshal(body, &entry); err != nil {
		return giteaContentEntry{}, false, err
	}
	return entry, true, nil
}

func (c tokenConfigRepositoryClient) doJSON(ctx context.Context, method, endpoint string, payload any, out any, okStatuses ...int) error {
	var body []byte
	var contentType string
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = encoded
		contentType = "application/json"
	}
	status, _, responseBody, err := c.do(ctx, method, endpoint, body, contentType)
	if err != nil {
		return err
	}
	if !statusOK(status, okStatuses...) {
		return c.statusError(method+" "+endpoint, status, responseBody)
	}
	if out != nil && len(bytes.TrimSpace(responseBody)) > 0 {
		if err := json.Unmarshal(responseBody, out); err != nil {
			return err
		}
	}
	return nil
}

func (c tokenConfigRepositoryClient) do(ctx context.Context, method, endpoint string, body []byte, contentType string) (int, http.Header, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "nopsai-config-sync")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	switch c.provider {
	case models.ConfigRepositoryProviderGitLab:
		req.Header.Set("PRIVATE-TOKEN", c.token)
	case models.ConfigRepositoryProviderGitea:
		req.Header.Set("Authorization", "token "+c.token)
	default:
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return 0, resp.Header, nil, err
	}
	return resp.StatusCode, resp.Header, responseBody, nil
}

func (c tokenConfigRepositoryClient) statusError(operation string, status int, body []byte) error {
	message := strings.TrimSpace(string(body))
	var payload struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if strings.TrimSpace(payload.Message) != "" {
			message = strings.TrimSpace(payload.Message)
		} else if strings.TrimSpace(payload.Error) != "" {
			message = strings.TrimSpace(payload.Error)
		}
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return fmt.Errorf("%s failed for %s repository %s (status %d): %s", operation, c.provider, c.identity.ProjectPath, status, message)
}

func (c tokenConfigRepositoryClient) scheme() string {
	if c.identity.Scheme == "http" {
		return "http"
	}
	return "https"
}

func (c tokenConfigRepositoryClient) githubAPIBase() string {
	host := strings.TrimSpace(c.identity.Host)
	if host == "" || strings.EqualFold(host, "github.com") {
		return c.scheme() + "://api.github.com"
	}
	if strings.EqualFold(host, "api.github.com") {
		return c.scheme() + "://" + host
	}
	return c.scheme() + "://" + host + "/api/v3"
}

func (c tokenConfigRepositoryClient) githubRepoEndpoint() string {
	return c.githubAPIBase() + "/repos/" + escapePathSegments(c.identity.Owner) + "/" + escapePathSegments(c.identity.Repo)
}

func (c tokenConfigRepositoryClient) githubContentsEndpoint(filePath, ref string) string {
	endpoint := c.githubRepoEndpoint() + "/contents"
	if strings.Trim(filePath, "/") != "" {
		endpoint += "/" + escapePathSegments(filePath)
	}
	values := url.Values{}
	if strings.TrimSpace(ref) != "" {
		values.Set("ref", ref)
	}
	if encoded := values.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	return endpoint
}

func (c tokenConfigRepositoryClient) gitlabAPIBase() string {
	host := strings.TrimSpace(c.identity.Host)
	if host == "" {
		host = "gitlab.com"
	}
	return c.scheme() + "://" + host + "/api/v4"
}

func (c tokenConfigRepositoryClient) gitlabProjectEndpoint() string {
	return c.gitlabAPIBase() + "/projects/" + url.PathEscape(c.identity.ProjectPath)
}

func (c tokenConfigRepositoryClient) bitbucketAPIBase() string {
	host := strings.TrimSpace(c.identity.Host)
	if host == "" {
		host = "bitbucket.org"
	}
	if strings.EqualFold(host, "bitbucket.org") {
		return c.scheme() + "://api.bitbucket.org/2.0"
	}
	return c.scheme() + "://" + host + "/2.0"
}

func (c tokenConfigRepositoryClient) bitbucketRepoEndpoint() string {
	return c.bitbucketAPIBase() + "/repositories/" + escapePathSegments(c.identity.Owner) + "/" + escapePathSegments(c.identity.Repo)
}

func (c tokenConfigRepositoryClient) bitbucketSourceEndpoint(ref, filePath string) string {
	endpoint := c.bitbucketRepoEndpoint() + "/src/" + url.PathEscape(ref)
	if strings.Trim(filePath, "/") != "" {
		endpoint += "/" + escapePathSegments(filePath)
	}
	return endpoint
}

func (c tokenConfigRepositoryClient) giteaAPIBase() string {
	host := strings.TrimSpace(c.identity.Host)
	if host == "" {
		host = "gitea.com"
	}
	return c.scheme() + "://" + host + "/api/v1"
}

func (c tokenConfigRepositoryClient) giteaRepoEndpoint() string {
	return c.giteaAPIBase() + "/repos/" + escapePathSegments(c.identity.Owner) + "/" + escapePathSegments(c.identity.Repo)
}

func (c tokenConfigRepositoryClient) giteaContentsEndpoint(filePath, ref string) string {
	endpoint := c.giteaRepoEndpoint() + "/contents"
	if strings.Trim(filePath, "/") != "" {
		endpoint += "/" + escapePathSegments(filePath)
	}
	values := url.Values{}
	if strings.TrimSpace(ref) != "" {
		values.Set("ref", ref)
	}
	if encoded := values.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	return endpoint
}

type githubContentEntry struct {
	Type     string `json:"type"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

type githubTreeEntry struct {
	Path string  `json:"path"`
	Mode string  `json:"mode"`
	Type string  `json:"type"`
	SHA  *string `json:"sha"`
}

type gitlabTreeEntry struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

type gitlabCommitRequest struct {
	Branch        string               `json:"branch"`
	StartBranch   string               `json:"start_branch,omitempty"`
	CommitMessage string               `json:"commit_message"`
	Actions       []gitlabCommitAction `json:"actions"`
}

type gitlabCommitAction struct {
	Action   string `json:"action"`
	FilePath string `json:"file_path"`
	Content  string `json:"content,omitempty"`
}

type bitbucketDirectoryListing struct {
	Values []struct {
		Type string `json:"type"`
		Path string `json:"path"`
	} `json:"values"`
	Next string `json:"next"`
}

type giteaContentEntry struct {
	Type    string `json:"type"`
	Path    string `json:"path"`
	Content string `json:"content"`
	SHA     string `json:"sha"`
}

type giteaContentWriteRequest struct {
	Message   string `json:"message"`
	Content   string `json:"content,omitempty"`
	Branch    string `json:"branch,omitempty"`
	NewBranch string `json:"new_branch,omitempty"`
	SHA       string `json:"sha,omitempty"`
}

type giteaContentWriteResponse struct {
	Commit struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
	} `json:"commit"`
}

var errConfigRepositoryProviderFileNotFound = fmt.Errorf("config repository provider file not found")

func decodeProviderContent(raw, encoding string) (string, error) {
	if !strings.EqualFold(strings.TrimSpace(encoding), "base64") {
		return raw, nil
	}
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t', ' ':
			return -1
		default:
			return r
		}
	}, raw)
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(cleaned)
	}
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func escapePathSegments(value string) string {
	value = strings.Trim(value, "/")
	if value == "" {
		return ""
	}
	segments := strings.Split(value, "/")
	for idx, segment := range segments {
		segments[idx] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}

func statusOK(status int, okStatuses ...int) bool {
	for _, okStatus := range okStatuses {
		if status == okStatus {
			return true
		}
	}
	return false
}

func isJSONArray(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) > 0 && trimmed[0] == '['
}

func isJSONObject(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func parseBitbucketDirectoryListing(body []byte) (bitbucketDirectoryListing, bool) {
	if !isJSONObject(body) {
		return bitbucketDirectoryListing{}, false
	}
	var listing bitbucketDirectoryListing
	if err := json.Unmarshal(body, &listing); err != nil {
		return bitbucketDirectoryListing{}, false
	}
	if listing.Values == nil && strings.TrimSpace(listing.Next) == "" {
		return bitbucketDirectoryListing{}, false
	}
	return listing, true
}

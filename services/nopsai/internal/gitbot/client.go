package gitbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nopsai/pkg/correlation"
	"nopsai/pkg/models"
	"nopsai/pkg/serviceauth"
)

type Client struct {
	BaseURL     string
	HTTPClient  *http.Client
	Credentials *serviceauth.Credentials
}

type CommitFile struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Delete  bool   `json:"delete,omitempty"`
}

type CommitFilesResponse struct {
	Branch       string `json:"branch"`
	CommitSHA    string `json:"commit_sha"`
	CommitURL    string `json:"commit_url,omitempty"`
	FilesChanged int    `json:"files_changed"`
}

type InstalledRepository struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

type SuiteCheckRunResponse struct {
	CheckRunID         int64  `json:"check_run_id"`
	HeadSHA            string `json:"head_sha"`
	PullRequestHeadRef string `json:"pull_request_head_ref,omitempty"`
	HeadBranch         string `json:"head_branch,omitempty"`
}

type FinalStatusRequest struct {
	Status     string
	FailedStep string
	FailedTask string
	CheckRunID int64
	RepoOwner  string
	RepoName   string
	CommitSHA  string
	Summary    string
}

type TaskStatusRequest struct {
	RunID      string
	RepoOwner  string
	RepoName   string
	CheckRunID int64
	CommitSHA  string
	StepName   string
	TaskName   string
	TaskStatus string
	TaskIndex  int
	TotalTasks int
	DependsOn  []string
	StartedAt  time.Time
	FinishedAt time.Time
}

func (c Client) File(owner, repo, ref, path string, notFoundErr error) (string, error) {
	resp, err := c.postJSON("/v1/github/file", map[string]string{
		"owner": owner,
		"repo":  repo,
		"ref":   ref,
		"path":  path,
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var out struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return "", err
		}
		return out.Content, nil
	case http.StatusNotFound:
		return "", notFoundErr
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("git-bot file request failed with status %d: %s", resp.StatusCode, string(respBody))
	}
}

func (c Client) Directory(owner, repo, ref, path string) (map[string]string, error) {
	resp, err := c.postJSON("/v1/github/contents", map[string]string{
		"owner": owner,
		"repo":  repo,
		"ref":   ref,
		"path":  path,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var out struct {
			Files map[string]string `json:"files"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, err
		}
		if out.Files == nil {
			out.Files = map[string]string{}
		}
		return out.Files, nil
	case http.StatusNotFound:
		return map[string]string{}, nil
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("git-bot contents request for '%s' failed with status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
}

func (c Client) CommitFiles(owner, repo, baseRef, branch, message string, files []CommitFile) (CommitFilesResponse, error) {
	resp, err := c.postJSON("/v1/github/commit", struct {
		Owner   string       `json:"owner"`
		Repo    string       `json:"repo"`
		BaseRef string       `json:"base_ref"`
		Branch  string       `json:"branch"`
		Message string       `json:"message"`
		Files   []CommitFile `json:"files"`
	}{
		Owner:   owner,
		Repo:    repo,
		BaseRef: baseRef,
		Branch:  branch,
		Message: message,
		Files:   files,
	})
	if err != nil {
		return CommitFilesResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var out CommitFilesResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return CommitFilesResponse{}, err
		}
		return out, nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return CommitFilesResponse{}, fmt.Errorf("git-bot commit request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
}

func (c Client) BranchHasOpenPullRequest(owner, repo, branch string) (bool, error) {
	resp, err := c.postJSON("/v1/github/branch/has-open-pr", map[string]string{
		"owner":  owner,
		"repo":   repo,
		"branch": branch,
	})
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var out struct {
			HasOpenPR bool `json:"has_open_pr"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return false, err
		}
		return out.HasOpenPR, nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return false, fmt.Errorf("branch open PR check failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
}

func (c Client) EnsureRepoAccessible(owner, repo string) error {
	resp, err := c.postJSON("/v1/github/repo/access", map[string]string{
		"owner": owner,
		"repo":  repo,
	})
	if err != nil {
		return fmt.Errorf("failed to verify config repository access: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	message := strings.TrimSpace(string(respBody))
	var errPayload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &errPayload); err == nil && errPayload.Error != "" {
		message = errPayload.Error
	}

	switch resp.StatusCode {
	case http.StatusNotFound:
		return fmt.Errorf("config repository '%s/%s' could not be found or Git Bot is not installed", owner, repo)
	case http.StatusForbidden:
		return fmt.Errorf("nopsai git-bot does not have permission to access config repository '%s/%s'", owner, repo)
	default:
		return fmt.Errorf("failed to verify config repository access for %s/%s (status %d): %s", owner, repo, resp.StatusCode, message)
	}
}

func (c Client) ListInstallationRepositories(installationID string) ([]InstalledRepository, error) {
	resp, err := c.getJSON("/v1/github/installations/" + urlPathEscape(installationID) + "/repositories")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("git-bot installation repository list failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var out struct {
		Repositories []InstalledRepository `json:"repositories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Repositories == nil {
		out.Repositories = []InstalledRepository{}
	}
	return out.Repositories, nil
}

func (c Client) Pipeline(owner, repo, ref string, source models.PipelineSource, notFoundErr error) ([]byte, error) {
	if source.Path != "" {
		content, err := c.File(owner, repo, ref, source.Path, notFoundErr)
		return []byte(content), err
	}

	resp, err := c.postJSON("/v1/github/pipeline", struct {
		Owner  string                `json:"owner"`
		Repo   string                `json:"repo"`
		Ref    string                `json:"ref"`
		Source models.PipelineSource `json:"source"`
	}{
		Owner:  owner,
		Repo:   repo,
		Ref:    ref,
		Source: source,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var out struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, err
		}
		return []byte(out.Content), nil
	case http.StatusNotFound:
		return nil, notFoundErr
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("git-bot pipeline request failed with status %d: %s", resp.StatusCode, string(respBody))
	}
}

func (c Client) FindSuiteCheckRun(owner, repo string, suiteID int64, commitSHA string) (*SuiteCheckRunResponse, error) {
	resp, err := c.postJSON("/v1/checks/find-suite-run", map[string]interface{}{
		"owner":      owner,
		"repo":       repo,
		"suite_id":   suiteID,
		"commit_sha": commitSHA,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to request suite check run from git-bot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("git-bot suite lookup failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out SuiteCheckRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("failed to decode git-bot suite lookup response: %w", err)
	}
	if out.CheckRunID == 0 || out.HeadSHA == "" {
		return nil, fmt.Errorf("git-bot returned incomplete suite check run data")
	}
	return &out, nil
}

func (c Client) CreateCheckRun(owner, repo, ref string, pipelineDef []byte, pipelineSource string) (int64, error) {
	resp, err := c.postJSON("/v1/checks/create", map[string]interface{}{
		"owner":               owner,
		"repo":                repo,
		"ref":                 ref,
		"pipeline_definition": string(pipelineDef),
		"pipeline_source":     pipelineSource,
	})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("git-bot check run creation failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var out struct {
		CheckRunID int64 `json:"check_run_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.CheckRunID, nil
}

func (c Client) CreateChildCheckRun(owner, repo, ref, parentName, includeName string, pipelineDef []byte) (int64, error) {
	resp, err := c.postJSON("/v1/checks/create-child", map[string]string{
		"owner":               owner,
		"repo":                repo,
		"ref":                 ref,
		"parent_name":         parentName,
		"include_name":        includeName,
		"pipeline_definition": string(pipelineDef),
	})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("git-bot returned status %d for child check run", resp.StatusCode)
	}

	var out struct {
		CheckRunID int64 `json:"check_run_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.CheckRunID, nil
}

func (c Client) InitializeCheckRun(owner, repo string, checkRunID int64, pipelineDef []byte, pipelineName string) error {
	resp, err := c.postJSON("/v1/checks/initialize", map[string]interface{}{
		"owner":               owner,
		"repo":                repo,
		"check_run_id":        checkRunID,
		"pipeline_definition": string(pipelineDef),
		"pipeline_name":       pipelineName,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("git-bot check run initialization failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (c Client) NotifyFinalStatus(req FinalStatusRequest) error {
	resp, err := c.postJSON("/v1/run/status", map[string]interface{}{
		"status":       req.Status,
		"failed_step":  req.FailedStep,
		"failed_task":  req.FailedTask,
		"check_run_id": req.CheckRunID,
		"repo_owner":   req.RepoOwner,
		"repo_name":    req.RepoName,
		"commit_sha":   req.CommitSHA,
		"summary":      req.Summary,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("git-bot final status update failed with status %d", resp.StatusCode)
	}
	return nil
}

func (c Client) NotifyTaskStatus(req TaskStatusRequest) error {
	resp, err := c.postJSON("/v1/task/status", map[string]interface{}{
		"run_id":       req.RunID,
		"repo_owner":   req.RepoOwner,
		"repo_name":    req.RepoName,
		"check_run_id": req.CheckRunID,
		"commit_sha":   req.CommitSHA,
		"step_name":    req.StepName,
		"task_name":    req.TaskName,
		"task_status":  req.TaskStatus,
		"task_index":   req.TaskIndex,
		"total_tasks":  req.TotalTasks,
		"depends_on":   req.DependsOn,
		"started_at":   req.StartedAt,
		"finished_at":  req.FinishedAt,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("git-bot task status update failed with status %d", resp.StatusCode)
	}
	return nil
}

func (c Client) getJSON(path string) (*http.Response, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	ctx, _ := correlation.EnsureRequestID(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.Credentials == nil {
		return nil, fmt.Errorf("nopsai service credentials are not configured")
	}
	token, err := c.Credentials.MintToken(req.Context())
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	correlation.SetHTTPHeaders(ctx, req.Header)
	return client.Do(req)
}

func (c Client) postJSON(path string, payload interface{}) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	ctx, _ := correlation.EnsureRequestID(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(path), bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Credentials == nil {
		return nil, fmt.Errorf("nopsai service credentials are not configured")
	}
	token, err := c.Credentials.MintToken(req.Context())
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	correlation.SetHTTPHeaders(ctx, req.Header)
	return client.Do(req)
}

func (c Client) url(path string) string {
	return strings.TrimRight(c.BaseURL, "/") + path
}

func urlPathEscape(value string) string {
	return strings.NewReplacer("/", "%2F", " ", "%20").Replace(strings.TrimSpace(value))
}

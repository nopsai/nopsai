package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/pkg/proxyhttp"
	"nopsai/services/git-bot/internal/checkrender"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v53/github"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

type GitBotApp struct {
	cfg            *config.Config
	ghClient       *github.Client
	httpClient     *http.Client
	webhookSecret  string
	checkRunStates map[int64]*CheckRunState
	stateLock      sync.Mutex
	githubAppID    int64
}

type RunStatusUpdate struct {
	Status     string `json:"status"`
	FailedStep string `json:"failed_step"`
	FailedTask string `json:"failed_task"`
	CheckRunID int64  `json:"check_run_id"`
	RepoOwner  string `json:"repo_owner"`
	RepoName   string `json:"repo_name"`
	Summary    string `json:"summary,omitempty"`
}

type CreateChildCheckRunRequest struct {
	Owner              string `json:"owner"`
	Repo               string `json:"repo"`
	Ref                string `json:"ref"`
	ParentName         string `json:"parent_name"`
	IncludeName        string `json:"include_name"`
	PipelineDefinition string `json:"pipeline_definition"`
}

type FileContentRequest struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	Ref   string `json:"ref"`
	Path  string `json:"path"`
}

type FileContentResponse struct {
	Content string `json:"content"`
}

type DirectoryContentsRequest struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	Path  string `json:"path"`
	Ref   string `json:"ref,omitempty"`
}

type DirectoryContentsResponse struct {
	Files map[string]string `json:"files"`
}

type CommitFile struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Delete  bool   `json:"delete,omitempty"`
}

type CommitFilesRequest struct {
	Owner   string       `json:"owner"`
	Repo    string       `json:"repo"`
	BaseRef string       `json:"base_ref"`
	Branch  string       `json:"branch"`
	Message string       `json:"message"`
	Files   []CommitFile `json:"files"`
}

type CommitFilesResponse struct {
	Branch       string `json:"branch"`
	CommitSHA    string `json:"commit_sha"`
	CommitURL    string `json:"commit_url,omitempty"`
	FilesChanged int    `json:"files_changed"`
}

type RepositoryAccessRequest struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

type RepositoryAccessResponse struct {
	Accessible    bool   `json:"accessible"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

type BranchPROpenRequest struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
}

type BranchPROpenResponse struct {
	HasOpenPR bool `json:"has_open_pr"`
}

type InstalledRepository struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

type InstalledRepositoriesResponse struct {
	Repositories []InstalledRepository `json:"repositories"`
}

type PipelineContentRequest struct {
	Owner  string                `json:"owner"`
	Repo   string                `json:"repo"`
	Ref    string                `json:"ref"`
	Source models.PipelineSource `json:"source"`
}

type PipelineContentResponse struct {
	Content string `json:"content"`
}

type CreateCheckRunRequest struct {
	Owner              string `json:"owner"`
	Repo               string `json:"repo"`
	Ref                string `json:"ref"`
	PipelineDefinition string `json:"pipeline_definition"`
	PipelineSource     string `json:"pipeline_source"`
}

type CreateCheckRunResponse struct {
	CheckRunID int64 `json:"check_run_id"`
}

type InitializeCheckRunRequest struct {
	Owner              string `json:"owner"`
	Repo               string `json:"repo"`
	CheckRunID         int64  `json:"check_run_id"`
	PipelineDefinition string `json:"pipeline_definition"`
	PipelineName       string `json:"pipeline_name"`
}

type CancelStaleCheckRunsRequest struct {
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	BeforeSHA string `json:"before_sha"`
}

type CancelStaleCheckRunsResponse struct {
	Cancelled int `json:"cancelled"`
}

type FindSuiteCheckRunRequest struct {
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	SuiteID   int64  `json:"suite_id"`
	CommitSHA string `json:"commit_sha"`
}

type FindSuiteCheckRunResponse struct {
	CheckRunID         int64  `json:"check_run_id"`
	HeadSHA            string `json:"head_sha"`
	PullRequestHeadRef string `json:"pull_request_head_ref,omitempty"`
	HeadBranch         string `json:"head_branch,omitempty"`
}

func (a *GitBotApp) verifySignature(r *http.Request, body []byte) bool {
	signature := r.Header.Get("X-Hub-Signature-256")
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	actualSignature := strings.TrimPrefix(signature, "sha256=")

	mac := hmac.New(sha256.New, []byte(a.webhookSecret))
	mac.Write(body)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(actualSignature), []byte(expectedMAC))
}

func (a *GitBotApp) createCheckRun(owner, repo, ref, pipelineDef, pipelineSource string) int64 {
	var pipeline models.Pipeline
	_ = yaml.Unmarshal([]byte(pipelineDef), &pipeline)

	checkName := pipeline.Name
	if checkName == "" {
		checkName = "Nopsai Pipeline"
	}
	if pipelineSource == "database override" {
		checkName = fmt.Sprintf("%s-overridden", checkName)
	}

	opts := github.CreateCheckRunOptions{
		Name:    checkName,
		HeadSHA: ref,
		Status:  github.String("queued"),
	}
	checkRun, _, err := a.ghClient.Checks.CreateCheckRun(context.Background(), owner, repo, opts)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create check run")
		return 0
	}

	// Use the centralized helper to initialize the state
	if err := a.initializeCheckRunState(*checkRun.ID, owner, repo, pipelineDef, checkName); err != nil {
		log.Error().Err(err).Msg("Failed to initialize check run state")
		// Conclude the check run as a failure since we can't track it
		a.concludeCheckRun(owner, repo, *checkRun.ID, "failure", "Failed to initialize internal tracking state for this pipeline.")
		return 0
	}

	inProgressOpts := github.UpdateCheckRunOptions{
		Name:   checkName,
		Status: github.String("in_progress"),
		Output: &github.CheckRunOutput{
			Title:   github.String(checkName),
			Summary: github.String("Pipeline is starting..."),
		},
	}
	a.ghClient.Checks.UpdateCheckRun(context.Background(), owner, repo, *checkRun.ID, inProgressOpts)

	return *checkRun.ID
}

func (a *GitBotApp) handleCreateChildCheckRun(w http.ResponseWriter, r *http.Request) {
	var req CreateChildCheckRunRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("owner", req.Owner),
		httpapi.RequiredString("repo", req.Repo),
		httpapi.RequiredString("ref", req.Ref),
		httpapi.RequiredString("parent_name", req.ParentName),
		httpapi.RequiredString("include_name", req.IncludeName),
		httpapi.RequiredString("pipeline_definition", req.PipelineDefinition),
	); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	checkName := fmt.Sprintf("%s / Included: %s", req.ParentName, req.IncludeName)

	opts := github.CreateCheckRunOptions{
		Name:    checkName,
		HeadSHA: req.Ref,
		Status:  github.String("queued"),
	}
	checkRun, _, err := a.ghClient.Checks.CreateCheckRun(context.Background(), req.Owner, req.Repo, opts)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create child check run")
		_ = httpapi.WriteJSONError(w, http.StatusInternalServerError, "failed to create child check run")
		return
	}

	// Use the centralized helper to initialize the state
	if err := a.initializeCheckRunState(*checkRun.ID, req.Owner, req.Repo, req.PipelineDefinition, checkName); err != nil {
		log.Error().Err(err).Msg("Failed to initialize state for child check run")
		a.concludeCheckRun(req.Owner, req.Repo, *checkRun.ID, "failure", "Failed to initialize internal tracking state for this included pipeline.")
		_ = httpapi.WriteJSONError(w, http.StatusInternalServerError, "failed to initialize internal state")
		return
	}

	inProgressOpts := github.UpdateCheckRunOptions{
		Name:   checkName,
		Status: github.String("in_progress"),
	}
	a.ghClient.Checks.UpdateCheckRun(context.Background(), req.Owner, req.Repo, *checkRun.ID, inProgressOpts)

	_ = httpapi.WriteJSON(w, http.StatusOK, map[string]int64{"check_run_id": *checkRun.ID})
}

func (a *GitBotApp) initializeCheckRunState(checkRunID int64, owner, repo, pipelineDef, checkName string) error {
	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(pipelineDef), &pipeline); err != nil {
		return fmt.Errorf("invalid pipeline definition: %w", err)
	}

	view := "flat"
	if pipeline.DisplayOptions.GitHubView == "mermaid" {
		view = "mermaid"
	} else if pipeline.DisplayOptions.GitHubView == "tree" {
		view = "tree"
	}

	a.stateLock.Lock()
	defer a.stateLock.Unlock()

	initialState := &CheckRunState{
		Steps:              make(map[string]map[string]TaskStatusUpdate),
		StepOrder:          []string{},
		GitHubView:         view,
		PipelineName:       checkName,
		PipelineDefinition: pipelineDef, // Store the pipeline definition
	}

	totalTasks := 0
	for _, step := range pipeline.Steps {
		if step.GetInclude() != "" {
			totalTasks++
		} else if tasks := step.GetTasks(); len(tasks) > 0 {
			totalTasks += len(tasks)
		} else {
			totalTasks++
		}
	}

	taskIndex := 1
	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		initialState.StepOrder = append(initialState.StepOrder, stepName)
		initialState.Steps[stepName] = make(map[string]TaskStatusUpdate)

		if step.GetInclude() != "" {
			initialState.Steps[stepName][stepName] = TaskStatusUpdate{
				StepName:   stepName,
				TaskName:   stepName,
				TaskStatus: "pending",
				TaskIndex:  taskIndex,
				TotalTasks: totalTasks,
				DependsOn:  step.GetDependsOn(),
				RepoOwner:  owner,
				RepoName:   repo,
			}
			taskIndex++
			continue
		}

		if tasks := step.GetTasks(); len(tasks) > 0 {
			for _, task := range tasks {
				initialState.Steps[stepName][task.Name] = TaskStatusUpdate{
					StepName:   stepName,
					TaskName:   task.Name,
					TaskStatus: "pending",
					TaskIndex:  taskIndex,
					TotalTasks: totalTasks,
					DependsOn:  task.DependsOn,
					RepoOwner:  owner,
					RepoName:   repo,
				}
				taskIndex++
			}
		} else {
			initialState.Steps[stepName][stepName] = TaskStatusUpdate{
				StepName:   stepName,
				TaskName:   stepName,
				TaskStatus: "pending",
				TaskIndex:  taskIndex,
				TotalTasks: totalTasks,
				DependsOn:  step.GetDependsOn(),
				RepoOwner:  owner,
				RepoName:   repo,
			}
			taskIndex++
		}
	}

	a.checkRunStates[checkRunID] = initialState
	log.Info().Int64("check_run_id", checkRunID).Int("steps", len(initialState.Steps)).Msg("Successfully initialized check run state.")
	return nil
}

func (a *GitBotApp) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Could not read request body", http.StatusInternalServerError)
		return
	}

	if !a.verifySignature(r, body) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	forwardURL := fmt.Sprintf("%s/v1/git/events", a.cfg.GitBotNopsaiAPIURL)
	req, err := http.NewRequest(http.MethodPost, forwardURL, bytes.NewReader(body))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create request to nopsai event endpoint")
		http.Error(w, "Failed to forward event", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	for _, header := range []string{"X-GitHub-Event", "X-GitHub-Delivery", "X-GitHub-Enterprise-Host", "X-GitHub-Enterprise-Version"} {
		if value := r.Header.Get(header); value != "" {
			req.Header.Set(header, value)
		}
	}
	req.Header.Set("X-Nopsai-Forwarded-By", "git-bot")

	client := a.httpClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to forward event to nopsai")
		http.Error(w, "Failed to forward event", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Error().Err(err).Msg("Failed to proxy response body")
	}
}

func (a *GitBotApp) handleFetchFile(w http.ResponseWriter, r *http.Request) {
	var req FileContentRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("owner", req.Owner),
		httpapi.RequiredString("repo", req.Repo),
		httpapi.RequiredString("ref", req.Ref),
		httpapi.RequiredString("path", req.Path),
	); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	fileContent, _, _, err := a.ghClient.Repositories.GetContents(
		context.Background(),
		req.Owner,
		req.Repo,
		req.Path,
		&github.RepositoryContentGetOptions{Ref: req.Ref},
	)
	if err != nil {
		var respErr *github.ErrorResponse
		if errors.As(err, &respErr) && respErr.Response != nil && respErr.Response.StatusCode == http.StatusNotFound {
			_ = httpapi.WriteJSONError(w, http.StatusNotFound, "file not found")
			return
		}
		log.Error().Err(err).Str("owner", req.Owner).Str("repo", req.Repo).Str("path", req.Path).Msg("Failed to fetch repository file")
		_ = httpapi.WriteJSONError(w, http.StatusInternalServerError, "failed to fetch file")
		return
	}
	if fileContent == nil {
		_ = httpapi.WriteJSONError(w, http.StatusNotFound, "file not found")
		return
	}

	content, err := fileContent.GetContent()
	if err != nil {
		log.Error().Err(err).Str("owner", req.Owner).Str("repo", req.Repo).Str("path", req.Path).Msg("Failed to decode repository file")
		_ = httpapi.WriteJSONError(w, http.StatusInternalServerError, "failed to decode file")
		return
	}

	_ = httpapi.WriteJSON(w, http.StatusOK, FileContentResponse{Content: content})
}

func (a *GitBotApp) handleFetchDirectoryContents(w http.ResponseWriter, r *http.Request) {
	var req DirectoryContentsRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("owner", req.Owner),
		httpapi.RequiredString("repo", req.Repo),
		httpapi.RequiredString("path", req.Path),
	); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	files := make(map[string]string)

	if err := a.collectRepositoryContents(context.Background(), req.Owner, req.Repo, strings.TrimPrefix(req.Path, "/"), req.Ref, files); err != nil {
		var respErr *github.ErrorResponse
		if errors.As(err, &respErr) && respErr.Response != nil && respErr.Response.StatusCode == http.StatusNotFound {
			_ = httpapi.WriteJSONError(w, http.StatusNotFound, "path not found")
			return
		}
		log.Error().Err(err).Str("owner", req.Owner).Str("repo", req.Repo).Str("path", req.Path).Msg("Failed to fetch repository contents")
		_ = httpapi.WriteJSONError(w, http.StatusInternalServerError, "failed to fetch repository contents")
		return
	}

	_ = httpapi.WriteJSON(w, http.StatusOK, DirectoryContentsResponse{Files: files})
}

func (a *GitBotApp) handleCommitFiles(w http.ResponseWriter, r *http.Request) {
	var req CommitFilesRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("owner", req.Owner),
		httpapi.RequiredString("repo", req.Repo),
		httpapi.RequiredString("base_ref", req.BaseRef),
		httpapi.RequiredString("branch", req.Branch),
		httpapi.RequiredString("message", req.Message),
	); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Files) == 0 {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "files is required")
		return
	}

	baseRefName, err := normalizeGitHubBranchRef(req.BaseRef)
	if err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "invalid base_ref: "+err.Error())
		return
	}
	branchName, err := normalizeGitHubBranchRef(req.Branch)
	if err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "invalid branch: "+err.Error())
		return
	}

	entries := make([]*github.TreeEntry, 0, len(req.Files))
	seen := make(map[string]struct{}, len(req.Files))
	for _, file := range req.Files {
		cleanPath, err := cleanCommitFilePath(file.Path)
		if err != nil {
			_ = httpapi.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if _, exists := seen[cleanPath]; exists {
			_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "duplicate file path: "+cleanPath)
			return
		}
		seen[cleanPath] = struct{}{}
		entry := &github.TreeEntry{
			Path: github.String(cleanPath),
			Mode: github.String("100644"),
			Type: github.String("blob"),
		}
		if !file.Delete {
			entry.Content = github.String(file.Content)
		}
		entries = append(entries, entry)
	}

	ctx := r.Context()
	baseRef, _, err := a.ghClient.Git.GetRef(ctx, req.Owner, req.Repo, baseRefName)
	if err != nil {
		writeGitHubCommitError(w, err, http.StatusNotFound, "base ref not found")
		return
	}
	baseSHA := ""
	if baseRef != nil && baseRef.Object != nil {
		baseSHA = baseRef.Object.GetSHA()
	}
	if baseSHA == "" {
		_ = httpapi.WriteJSONError(w, http.StatusBadGateway, "base ref did not include a commit sha")
		return
	}

	targetRef, _, err := a.ghClient.Git.GetRef(ctx, req.Owner, req.Repo, branchName)
	if err != nil {
		var ghErr *github.ErrorResponse
		if !errors.As(err, &ghErr) || ghErr.Response == nil || ghErr.Response.StatusCode != http.StatusNotFound {
			writeGitHubCommitError(w, err, http.StatusBadGateway, "failed to read push branch")
			return
		}
		targetRef, _, err = a.ghClient.Git.CreateRef(ctx, req.Owner, req.Repo, &github.Reference{
			Ref: github.String("refs/" + branchName),
			Object: &github.GitObject{
				SHA: github.String(baseSHA),
			},
		})
		if err != nil {
			writeGitHubCommitError(w, err, http.StatusBadGateway, "failed to create push branch")
			return
		}
	}

	parentSHA := ""
	if targetRef != nil && targetRef.Object != nil {
		parentSHA = targetRef.Object.GetSHA()
	}
	if parentSHA == "" {
		_ = httpapi.WriteJSONError(w, http.StatusBadGateway, "push branch did not include a commit sha")
		return
	}
	parentCommit, _, err := a.ghClient.Git.GetCommit(ctx, req.Owner, req.Repo, parentSHA)
	if err != nil {
		writeGitHubCommitError(w, err, http.StatusBadGateway, "failed to read push branch commit")
		return
	}
	baseTreeSHA := ""
	if parentCommit != nil && parentCommit.Tree != nil {
		baseTreeSHA = parentCommit.Tree.GetSHA()
	}
	if baseTreeSHA == "" {
		_ = httpapi.WriteJSONError(w, http.StatusBadGateway, "push branch commit did not include a tree sha")
		return
	}

	tree, _, err := a.ghClient.Git.CreateTree(ctx, req.Owner, req.Repo, baseTreeSHA, entries)
	if err != nil {
		writeGitHubCommitError(w, err, http.StatusBadGateway, "failed to create push tree")
		return
	}
	commit, _, err := a.ghClient.Git.CreateCommit(ctx, req.Owner, req.Repo, &github.Commit{
		Message: github.String(strings.TrimSpace(req.Message)),
		Tree:    tree,
		Parents: []*github.Commit{{SHA: github.String(parentSHA)}},
	})
	if err != nil {
		writeGitHubCommitError(w, err, http.StatusBadGateway, "failed to create push commit")
		return
	}
	commitSHA := commit.GetSHA()
	if commitSHA == "" {
		_ = httpapi.WriteJSONError(w, http.StatusBadGateway, "created push commit did not include a sha")
		return
	}

	_, _, err = a.ghClient.Git.UpdateRef(ctx, req.Owner, req.Repo, &github.Reference{
		Ref: github.String("refs/" + branchName),
		Object: &github.GitObject{
			SHA: github.String(commitSHA),
		},
	}, false)
	if err != nil {
		writeGitHubCommitError(w, err, http.StatusBadGateway, "failed to update push branch")
		return
	}

	_ = httpapi.WriteJSON(w, http.StatusOK, CommitFilesResponse{
		Branch:       strings.TrimPrefix(branchName, "heads/"),
		CommitSHA:    commitSHA,
		CommitURL:    commit.GetHTMLURL(),
		FilesChanged: len(entries),
	})
}

func normalizeGitHubBranchRef(value string) (string, error) {
	branch := strings.TrimSpace(value)
	branch = strings.TrimPrefix(branch, "refs/heads/")
	branch = strings.TrimPrefix(branch, "heads/")
	if branch == "" {
		return "", fmt.Errorf("branch is required")
	}
	if strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") || strings.HasSuffix(branch, ".") || branch == "@" {
		return "", fmt.Errorf("branch name is invalid")
	}
	invalidFragments := []string{"..", "//", "@{", "\\", ":", "?", "*", "[", "^", "~", " "}
	for _, fragment := range invalidFragments {
		if strings.Contains(branch, fragment) {
			return "", fmt.Errorf("branch contains invalid characters")
		}
	}
	for _, segment := range strings.Split(branch, "/") {
		if segment == "" || strings.HasPrefix(segment, ".") || strings.HasSuffix(segment, ".lock") {
			return "", fmt.Errorf("branch contains invalid path segments")
		}
	}
	return "heads/" + branch, nil
}

func cleanCommitFilePath(raw string) (string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	normalized = strings.TrimPrefix(normalized, "/")
	cleaned := path.Clean(normalized)
	if cleaned == "." || cleaned == "" || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("invalid file path: %s", raw)
	}
	return cleaned, nil
}

func writeGitHubCommitError(w http.ResponseWriter, err error, fallbackStatus int, fallbackMessage string) {
	var ghErr *github.ErrorResponse
	if errors.As(err, &ghErr) && ghErr.Response != nil {
		message := fallbackMessage
		if strings.TrimSpace(ghErr.Message) != "" {
			message = strings.TrimSpace(ghErr.Message)
		}
		_ = httpapi.WriteJSONError(w, ghErr.Response.StatusCode, message)
		return
	}
	log.Error().Err(err).Msg(fallbackMessage)
	_ = httpapi.WriteJSONError(w, fallbackStatus, fallbackMessage)
}

func (a *GitBotApp) handleCheckRepoAccess(w http.ResponseWriter, r *http.Request) {
	var req RepositoryAccessRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("owner", req.Owner),
		httpapi.RequiredString("repo", req.Repo),
	); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	repo, resp, err := a.ghClient.Repositories.Get(context.Background(), req.Owner, req.Repo)
	if err != nil {
		var ghErr *github.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Response != nil {
			switch ghErr.Response.StatusCode {
			case http.StatusNotFound:
				_ = httpapi.WriteJSONError(w, http.StatusNotFound, "repository not found or Git Bot not installed")
				return
			case http.StatusForbidden:
				_ = httpapi.WriteJSONError(w, http.StatusForbidden, "access to repository forbidden for Git Bot")
				return
			}
		}
		status := http.StatusInternalServerError
		if resp != nil {
			status = resp.StatusCode
		}
		message := "failed to verify repository access"
		if ghErr != nil && ghErr.Message != "" {
			message = fmt.Sprintf("%s: %s", message, ghErr.Message)
		} else {
			message = fmt.Sprintf("%s: %v", message, err)
		}
		log.Error().Err(err).Str("owner", req.Owner).Str("repo", req.Repo).Int("status", status).Msg("Failed to verify repository access")
		_ = httpapi.WriteJSONError(w, status, message)
		return
	}

	defaultBranch := ""
	if repo != nil && repo.DefaultBranch != nil {
		defaultBranch = repo.GetDefaultBranch()
	}

	_ = httpapi.WriteJSON(w, http.StatusOK, RepositoryAccessResponse{Accessible: true, DefaultBranch: defaultBranch})
}

func (a *GitBotApp) handleCheckBranchHasOpenPR(w http.ResponseWriter, r *http.Request) {
	var req BranchPROpenRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("owner", req.Owner),
		httpapi.RequiredString("repo", req.Repo),
		httpapi.RequiredString("branch", req.Branch),
	); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	options := &github.PullRequestListOptions{
		State: "open",
		Head:  fmt.Sprintf("%s:%s", req.Owner, req.Branch),
		ListOptions: github.ListOptions{
			PerPage: 1,
		},
	}

	prs, _, err := a.ghClient.PullRequests.List(context.Background(), req.Owner, req.Repo, options)
	if err != nil {
		log.Error().Err(err).Str("owner", req.Owner).Str("repo", req.Repo).Str("branch", req.Branch).Msg("Failed to check open pull requests for branch")
		_ = httpapi.WriteJSONError(w, http.StatusInternalServerError, "failed to check pull requests")
		return
	}

	response := BranchPROpenResponse{HasOpenPR: len(prs) > 0}

	if err := httpapi.WriteJSON(w, http.StatusOK, response); err != nil {
		log.Error().Err(err).Msg("Failed to encode branch PR response")
	}
}

func (a *GitBotApp) collectRepositoryContents(ctx context.Context, owner, repo, path, ref string, results map[string]string) error {
	fileContent, dirContents, _, err := a.ghClient.Repositories.GetContents(
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
			if err := a.collectRepositoryContents(ctx, owner, repo, entryPath, ref, results); err != nil {
				return err
			}
		case "file":
			fileContent, _, _, err := a.ghClient.Repositories.GetContents(
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

func (a *GitBotApp) handleListInstalledRepositories(w http.ResponseWriter, r *http.Request) {
	var repositories []InstalledRepository
	opts := &github.ListOptions{PerPage: 100}
	for {
		result, resp, err := a.ghClient.Apps.ListRepos(r.Context(), opts)
		if err != nil {
			log.Error().Err(err).Msg("Failed to list GitHub App installation repositories")
			_ = httpapi.WriteJSONError(w, http.StatusBadGateway, "failed to list installation repositories")
			return
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
	_ = httpapi.WriteJSON(w, http.StatusOK, InstalledRepositoriesResponse{Repositories: repositories})
}

func (a *GitBotApp) handleFetchPipeline(w http.ResponseWriter, r *http.Request) {
	var req PipelineContentRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("owner", req.Owner),
		httpapi.RequiredString("repo", req.Repo),
		httpapi.RequiredString("ref", req.Ref),
	); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	var pipelineYAML []byte

	if req.Source.Path != "" {
		if strings.HasPrefix(req.Source.Path, "http://") || strings.HasPrefix(req.Source.Path, "https://") {
			_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "remote pipeline URLs are no longer supported")
			return
		}
		fileContent, _, _, fetchErr := a.ghClient.Repositories.GetContents(
			context.Background(),
			req.Owner,
			req.Repo,
			req.Source.Path,
			&github.RepositoryContentGetOptions{Ref: req.Ref},
		)
		if fetchErr != nil {
			var respErr *github.ErrorResponse
			if errors.As(fetchErr, &respErr) && respErr.Response != nil && respErr.Response.StatusCode == http.StatusNotFound {
				_ = httpapi.WriteJSONError(w, http.StatusNotFound, "pipeline file not found")
				return
			}
			log.Error().Err(fetchErr).Str("owner", req.Owner).Str("repo", req.Repo).Str("path", req.Source.Path).Msg("Failed to fetch pipeline file")
			_ = httpapi.WriteJSONError(w, http.StatusInternalServerError, "failed to fetch pipeline file")
			return
		}
		if fileContent == nil {
			_ = httpapi.WriteJSONError(w, http.StatusNotFound, "pipeline file not found")
			return
		}
		content, decodeErr := fileContent.GetContent()
		if decodeErr != nil {
			log.Error().Err(decodeErr).Str("owner", req.Owner).Str("repo", req.Repo).Str("path", req.Source.Path).Msg("Failed to decode pipeline file content")
			_ = httpapi.WriteJSONError(w, http.StatusInternalServerError, "failed to decode pipeline file")
			return
		}
		pipelineYAML = []byte(content)
	} else {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "pipeline source must include a path")
		return
	}

	_ = httpapi.WriteJSON(w, http.StatusOK, PipelineContentResponse{Content: string(pipelineYAML)})
}

func (a *GitBotApp) handleCreateCheckRun(w http.ResponseWriter, r *http.Request) {
	var req CreateCheckRunRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("owner", req.Owner),
		httpapi.RequiredString("repo", req.Repo),
		httpapi.RequiredString("ref", req.Ref),
		httpapi.RequiredString("pipeline_definition", req.PipelineDefinition),
	); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.PipelineSource == "" {
		req.PipelineSource = "repository"
	}

	checkRunID := a.createCheckRun(req.Owner, req.Repo, req.Ref, req.PipelineDefinition, req.PipelineSource)
	if checkRunID == 0 {
		_ = httpapi.WriteJSONError(w, http.StatusInternalServerError, "failed to create check run")
		return
	}

	_ = httpapi.WriteJSON(w, http.StatusOK, CreateCheckRunResponse{CheckRunID: checkRunID})
}

func (a *GitBotApp) handleInitializeCheckRun(w http.ResponseWriter, r *http.Request) {
	var req InitializeCheckRunRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("owner", req.Owner),
		httpapi.RequiredString("repo", req.Repo),
		httpapi.RequiredInt64("check_run_id", req.CheckRunID),
		httpapi.RequiredString("pipeline_definition", req.PipelineDefinition),
		httpapi.RequiredString("pipeline_name", req.PipelineName),
	); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.initializeCheckRunState(req.CheckRunID, req.Owner, req.Repo, req.PipelineDefinition, req.PipelineName); err != nil {
		log.Error().Err(err).Int64("check_run_id", req.CheckRunID).Msg("Failed to initialize check run state")
		_ = httpapi.WriteJSONError(w, http.StatusInternalServerError, "failed to initialize check run state")
		return
	}

	opts := github.UpdateCheckRunOptions{
		Name:   req.PipelineName,
		Status: github.String("in_progress"),
		Output: &github.CheckRunOutput{
			Title:   github.String(req.PipelineName),
			Summary: github.String("Pipeline is starting..."),
		},
	}
	if _, _, err := a.ghClient.Checks.UpdateCheckRun(context.Background(), req.Owner, req.Repo, req.CheckRunID, opts); err != nil {
		log.Error().Err(err).Int64("check_run_id", req.CheckRunID).Msg("Failed to mark check run in progress")
		_ = httpapi.WriteJSONError(w, http.StatusInternalServerError, "failed to update check run")
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (a *GitBotApp) handleCancelStaleCheckRuns(w http.ResponseWriter, r *http.Request) {
	var req CancelStaleCheckRunsRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("owner", req.Owner),
		httpapi.RequiredString("repo", req.Repo),
		httpapi.RequiredString("before_sha", req.BeforeSHA),
	); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	opts := &github.ListCheckRunsOptions{}
	checkRuns, _, err := a.ghClient.Checks.ListCheckRunsForRef(context.Background(), req.Owner, req.Repo, req.BeforeSHA, opts)
	if err != nil {
		log.Error().Err(err).Str("owner", req.Owner).Str("repo", req.Repo).Str("sha", req.BeforeSHA).Msg("Failed to list check runs for stale commit")
		_ = httpapi.WriteJSONError(w, http.StatusInternalServerError, "failed to list check runs")
		return
	}

	cancelled := 0
	for _, cr := range checkRuns.CheckRuns {
		isOurApp := cr.GetApp() != nil && cr.GetApp().GetID() == a.githubAppID
		isRunning := cr.GetStatus() == "queued" || cr.GetStatus() == "in_progress"
		if !isOurApp || !isRunning {
			continue
		}

		updateOpts := github.UpdateCheckRunOptions{
			Name:        cr.GetName(),
			Status:      github.String("completed"),
			Conclusion:  github.String("cancelled"),
			CompletedAt: &github.Timestamp{Time: time.Now()},
			Output: &github.CheckRunOutput{
				Title:   github.String(cr.GetName() + " - Cancelled"),
				Summary: github.String("This run was cancelled because a new commit was pushed to the branch."),
			},
		}
		if _, _, err := a.ghClient.Checks.UpdateCheckRun(context.Background(), req.Owner, req.Repo, cr.GetID(), updateOpts); err != nil {
			log.Error().Err(err).Int64("check_run_id", cr.GetID()).Msg("Failed to cancel stale check run")
			continue
		}
		cancelled++
	}

	_ = httpapi.WriteJSON(w, http.StatusOK, CancelStaleCheckRunsResponse{Cancelled: cancelled})
}

func (a *GitBotApp) handleFindSuiteCheckRun(w http.ResponseWriter, r *http.Request) {
	var req FindSuiteCheckRunRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("owner", req.Owner),
		httpapi.RequiredString("repo", req.Repo),
		httpapi.RequiredInt64("suite_id", req.SuiteID),
		httpapi.RequiredString("commit_sha", req.CommitSHA),
	); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	runsResp, _, err := a.ghClient.Checks.ListCheckRunsForRef(context.Background(), req.Owner, req.Repo, req.CommitSHA, &github.ListCheckRunsOptions{})
	if err != nil {
		log.Error().Err(err).Msg("Failed to list check runs for commit")
		_ = httpapi.WriteJSONError(w, http.StatusInternalServerError, "failed to list check runs for commit")
		return
	}
	if len(runsResp.CheckRuns) == 0 {
		_ = httpapi.WriteJSONError(w, http.StatusNotFound, "no check runs found for suite")
		return
	}

	var target *github.CheckRun
	for _, cr := range runsResp.CheckRuns {
		if cr.CheckSuite != nil && cr.CheckSuite.ID != nil && *cr.CheckSuite.ID == req.SuiteID {
			target = cr
			break
		}
		if cr.GetApp() != nil && cr.GetApp().GetID() == a.githubAppID {
			target = cr
			break
		}
	}
	if target == nil {
		target = runsResp.CheckRuns[0]
	}

	response := FindSuiteCheckRunResponse{
		CheckRunID: target.GetID(),
		HeadSHA:    target.GetHeadSHA(),
	}
	if target.CheckSuite != nil && target.CheckSuite.HeadBranch != nil {
		response.HeadBranch = target.CheckSuite.GetHeadBranch()
	}
	if len(target.PullRequests) > 0 && target.PullRequests[0] != nil && target.PullRequests[0].Head != nil && target.PullRequests[0].Head.Ref != nil {
		response.PullRequestHeadRef = target.PullRequests[0].GetHead().GetRef()
	}

	_ = httpapi.WriteJSON(w, http.StatusOK, response)
}

func (a *GitBotApp) handleTaskStatusUpdate(w http.ResponseWriter, r *http.Request) {
	var update TaskStatusUpdate
	if err := httpapi.DecodeJSON(r, &update); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := httpapi.ValidateRequired(
		httpapi.RequiredInt64("check_run_id", update.CheckRunID),
		httpapi.RequiredString("repo_owner", update.RepoOwner),
		httpapi.RequiredString("repo_name", update.RepoName),
		httpapi.RequiredString("step_name", update.StepName),
		httpapi.RequiredString("task_name", update.TaskName),
	); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	log.Info().Int64("check_run_id", update.CheckRunID).Str("step", update.StepName).Str("task", update.TaskName).Msg("Received task status update")

	a.stateLock.Lock()
	defer a.stateLock.Unlock()

	state, ok := a.checkRunStates[update.CheckRunID]
	if !ok {
		log.Error().Int64("check_run_id", update.CheckRunID).Msg("Received task update for unknown check run")
		return
	}

	// Store the RunID from the first task update that contains it
	if state.RunID == "" && update.RunID != "" {
		state.RunID = update.RunID
	}

	// --- NEW: Clean up placeholder tasks for included steps ---
	// If this update is for a task within a step (e.g., "overwrite/write-secret-2")
	// and a placeholder for the step itself exists (e.g., "overwrite/overwrite"), remove the placeholder.
	if update.StepName != update.TaskName {
		if stepTasks, ok := state.Steps[update.StepName]; ok {
			if _, placeholderExists := stepTasks[update.StepName]; placeholderExists {
				log.Info().Str("step", update.StepName).Msg("Removing placeholder task for included step")
				delete(state.Steps[update.StepName], update.StepName)
			}
		}
	}
	// --- End of new logic ---

	if existingTask, ok := state.Steps[update.StepName][update.TaskName]; ok {
		existingTask.TaskStatus = update.TaskStatus
		if !update.StartedAt.IsZero() {
			existingTask.StartedAt = update.StartedAt
		}
		if !update.FinishedAt.IsZero() {
			existingTask.FinishedAt = update.FinishedAt
		}
		if len(update.DependsOn) > 0 {
			existingTask.DependsOn = update.DependsOn
		}
		state.Steps[update.StepName][update.TaskName] = existingTask
	} else {
		if _, ok := state.Steps[update.StepName]; !ok {
			state.Steps[update.StepName] = make(map[string]TaskStatusUpdate)
		}
		state.Steps[update.StepName][update.TaskName] = update
	}

	if update.GitHubView != "" {
		state.GitHubView = update.GitHubView
	}

	summary := checkrender.Render(state)

	completedTasks := 0
	totalTasks := 0
	for _, stepTasks := range state.Steps {
		for _, task := range stepTasks {
			totalTasks++
			if task.TaskStatus != "pending" && task.TaskStatus != "running" && task.TaskStatus != "skipped" {
				completedTasks++
			}
		}
	}

	newTitle := fmt.Sprintf("Running... (%d/%d tasks)", completedTasks, totalTasks)
	if state.RunID != "" {
		shortRunID := state.RunID[:8]
		newTitle = fmt.Sprintf("Running...(%s) (%d/%d tasks)", shortRunID, completedTasks, totalTasks)
	}

	opts := github.UpdateCheckRunOptions{
		Name:   state.PipelineName,
		Status: github.String("in_progress"),
		Output: &github.CheckRunOutput{
			Title:   github.String(newTitle),
			Summary: github.String(summary),
		},
	}
	_, _, err := a.ghClient.Checks.UpdateCheckRun(context.Background(), update.RepoOwner, update.RepoName, update.CheckRunID, opts)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update check run")
	}

	w.WriteHeader(http.StatusOK)
}

// handleRunStatusUpdate now handles skipping dependent tasks.
func (a *GitBotApp) handleRunStatusUpdate(w http.ResponseWriter, r *http.Request) {
	var update RunStatusUpdate
	if err := httpapi.DecodeJSON(r, &update); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("status", update.Status),
		httpapi.RequiredInt64("check_run_id", update.CheckRunID),
		httpapi.RequiredString("repo_owner", update.RepoOwner),
		httpapi.RequiredString("repo_name", update.RepoName),
	); err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	log.Info().Int64("check_run_id", update.CheckRunID).Str("status", update.Status).Msg("Received final pipeline status")

	a.stateLock.Lock()
	state, ok := a.checkRunStates[update.CheckRunID]

	if ok && update.Status == "failure" && update.FailedTask != "" {
		// Basic logic to skip subsequent tasks in the same step
		failedStepFound := false
		for _, stepName := range state.StepOrder {
			if stepName == update.FailedStep {
				failedStepFound = true
			}
			if failedStepFound {
				for taskName, task := range state.Steps[stepName] {
					if task.TaskStatus == "pending" {
						task.TaskStatus = "skipped"
						state.Steps[stepName][taskName] = task
					}
				}
			}
		}
	}

	summary := ""
	if ok {
		summary = checkrender.Render(state)
	}
	if update.Status == "failure" && update.Summary != "" {
		summary = fmt.Sprintf("❌ **Pipeline Failed:**\n\n%s", update.Summary)
	}
	a.stateLock.Unlock()

	a.concludeCheckRun(update.RepoOwner, update.RepoName, update.CheckRunID, update.Status, summary)
	w.WriteHeader(http.StatusOK)
}

func (a *GitBotApp) concludeCheckRun(owner, repo string, checkRunID int64, conclusion, summary string) {
	if checkRunID == 0 {
		log.Warn().Msg("Invalid checkRunID (0), skipping conclusion.")
		return
	}

	a.stateLock.Lock()
	defer a.stateLock.Unlock()

	state, ok := a.checkRunStates[checkRunID]
	if !ok {
		log.Warn().Int64("check_run_id", checkRunID).Msg("State not found for check run, cannot conclude with final name.")
		opts := github.UpdateCheckRunOptions{
			Name:        "Nopsai Pipeline",
			Status:      github.String("completed"),
			Conclusion:  github.String(conclusion),
			CompletedAt: &github.Timestamp{Time: time.Now()},
			Output: &github.CheckRunOutput{
				Title:   github.String("Nopsai Pipeline - " + strings.Title(conclusion)),
				Summary: github.String(summary),
			},
		}
		_, _, err := a.ghClient.Checks.UpdateCheckRun(context.Background(), owner, repo, checkRunID, opts)
		if err != nil {
			log.Error().Err(err).Msg("Failed to conclude check run with fallback name")
		}
		return
	}

	// The check run name remains clean and unchanged.
	finalName := state.PipelineName

	// Construct the final title with the ID and conclusion.
	finalTitle := fmt.Sprintf("%s - %s", state.PipelineName, strings.Title(conclusion))
	if state.RunID != "" {
		shortRunID := state.RunID[:8]
		finalTitle = fmt.Sprintf("%s (%s) - %s", state.PipelineName, shortRunID, strings.Title(conclusion))
	}

	opts := github.UpdateCheckRunOptions{
		Name:        finalName,
		Status:      github.String("completed"),
		Conclusion:  github.String(conclusion),
		CompletedAt: &github.Timestamp{Time: time.Now()},
		Output: &github.CheckRunOutput{
			Title:   github.String(finalTitle),
			Summary: github.String(summary),
		},
	}

	_, _, err := a.ghClient.Checks.UpdateCheckRun(context.Background(), owner, repo, checkRunID, opts)
	if err != nil {
		log.Error().Err(err).Msg("Failed to conclude check run")
	}

	delete(a.checkRunStates, checkRunID)
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func main() {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yml"
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to load config from %s", configPath)
	}

	if cfg.GitHubPrivateKey != "" {
		correctedKey := strings.ReplaceAll(cfg.GitHubPrivateKey, "\n", "\n")

		// ** FIXED **: Ensure parent directory exists before writing the file.
		if err := os.MkdirAll(filepath.Dir(cfg.GitHubPrivateKeyPath), 0700); err != nil {
			log.Fatal().Err(err).Msgf("Failed to create directory for private key: %s", cfg.GitHubPrivateKeyPath)
		}

		log.Info().Msgf("Writing GITHUB_PRIVATE_KEY to file: %s", cfg.GitHubPrivateKeyPath)
		err = os.WriteFile(cfg.GitHubPrivateKeyPath, []byte(correctedKey), 0600)
		if err != nil {
			log.Fatal().Err(err).Msgf("Failed to write private key to file: %s", cfg.GitHubPrivateKeyPath)
		}
	}
	logLevel, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.Warn().Msgf("Invalid log level '%s', defaulting to 'info'", cfg.LogLevel)
		logLevel = zerolog.InfoLevel
	}
	if cfg.LogFormat == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen})
	}
	zerolog.SetGlobalLevel(logLevel)

	appID, err := strconv.ParseInt(cfg.GitHubAppID, 10, 64)
	if err != nil {
		log.Fatal().Err(err).Msg("Invalid GitHub App ID in configuration")
	}
	installationID, err := strconv.ParseInt(cfg.GitHubInstallID, 10, 64)
	if err != nil {
		log.Fatal().Err(err).Msg("Invalid GitHub Installation ID in configuration")
	}

	if cfg.GitHubPrivateKeyPath == "" {
		log.Fatal().Msg("github_private_key_path must be set in the configuration.")
	}

	log.Info().Msgf("Loading GitHub private key from file path: %s", cfg.GitHubPrivateKeyPath)
	privateKeyBytes, err := os.ReadFile(cfg.GitHubPrivateKeyPath)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to read private key from path: %s", cfg.GitHubPrivateKeyPath)
	}

	itr, err := ghinstallation.NewAppsTransport(http.DefaultTransport, appID, privateKeyBytes)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create GitHub App transport")
	}

	installationTransport := ghinstallation.NewFromAppsTransport(itr, installationID)
	githubHTTPClient := &http.Client{
		Transport: installationTransport,
		Timeout:   15 * time.Second,
	}
	ghClient := github.NewClient(githubHTTPClient)
	httpClient := proxyhttp.NewInternalAwareClient(10 * time.Second)

	app := &GitBotApp{
		cfg:            cfg,
		ghClient:       ghClient,
		httpClient:     httpClient,
		webhookSecret:  cfg.GitHubWebhookSecret,
		checkRunStates: make(map[int64]*CheckRunState),
		githubAppID:    appID,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("/webhook", app.handleWebhook)
	mux.HandleFunc("POST /v1/github/file", app.handleFetchFile)
	mux.HandleFunc("POST /v1/github/contents", app.handleFetchDirectoryContents)
	mux.HandleFunc("POST /v1/github/commit", app.handleCommitFiles)
	mux.HandleFunc("POST /v1/github/repo/access", app.handleCheckRepoAccess)
	mux.HandleFunc("POST /v1/github/branch/has-open-pr", app.handleCheckBranchHasOpenPR)
	mux.HandleFunc("GET /v1/github/installation/repositories", app.handleListInstalledRepositories)
	mux.HandleFunc("POST /v1/github/pipeline", app.handleFetchPipeline)
	mux.HandleFunc("POST /v1/checks/create", app.handleCreateCheckRun)
	mux.HandleFunc("POST /v1/checks/initialize", app.handleInitializeCheckRun)
	mux.HandleFunc("POST /v1/checks/find-suite-run", app.handleFindSuiteCheckRun)
	mux.HandleFunc("POST /v1/checks/cancel-stale", app.handleCancelStaleCheckRuns)
	mux.HandleFunc("/v1/run/status", app.handleRunStatusUpdate)
	mux.HandleFunc("/v1/task/status", app.handleTaskStatusUpdate)
	mux.HandleFunc("/v1/checks/create-child", app.handleCreateChildCheckRun)

	log.Info().Msgf("Nopsai Git Bot server listening on %s", cfg.GitBotListenAddress)
	if err := http.ListenAndServe(cfg.GitBotListenAddress, mux); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}

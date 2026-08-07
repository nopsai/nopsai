package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicelog"
	"nopsai/services/git-bot/internal/checkrender"

	"github.com/google/go-github/v53/github"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

type GitBotApp struct {
	clientResolver     GitHubClientResolver
	webhookSecret      string
	checkRunStates     map[int64]*checkrender.State
	stateLock          sync.Mutex
	githubAppID        int64
	repositoryProvider repositoryProvider
	checksProvider     checksProvider
	webhookForwarder   nopsaiWebhookForwarder
	serviceAuth        *serviceauth.Authenticator
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

func NewGitBotApp(
	cfg *config.Config,
	resolver GitHubClientResolver,
	httpClient *http.Client,
	githubAppID int64,
	webhookSecret string,
	serviceAuthenticator *serviceauth.Authenticator,
	serviceCredentials *serviceauth.Credentials,
) *GitBotApp {
	repositoryProvider := repositoryProvider(unavailableRepositoryProvider{})
	checksProvider := checksProvider(unavailableChecksProvider{})
	if resolver != nil {
		repositoryProvider = newGitHubRepositoryProvider(resolver)
		checksProvider = newGitHubChecksProvider(resolver)
	}
	return &GitBotApp{
		clientResolver:     resolver,
		webhookSecret:      webhookSecret,
		checkRunStates:     make(map[int64]*checkrender.State),
		githubAppID:        githubAppID,
		repositoryProvider: repositoryProvider,
		checksProvider:     checksProvider,
		webhookForwarder:   newNopsaiWebhookForwarder(cfg, httpClient, serviceCredentials),
		serviceAuth:        serviceAuthenticator,
	}
}

func (a *GitBotApp) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("/webhook", a.handleWebhook)
	mux.HandleFunc("POST /v1/github/file", a.handleFetchFile)
	mux.HandleFunc("POST /v1/github/contents", a.handleFetchDirectoryContents)
	mux.HandleFunc("POST /v1/github/commit", a.handleCommitFiles)
	mux.HandleFunc("POST /v1/github/repo/access", a.handleCheckRepoAccess)
	mux.HandleFunc("POST /v1/github/branch/has-open-pr", a.handleCheckBranchHasOpenPR)
	mux.HandleFunc("GET /v1/github/installation/repositories", a.handleListInstalledRepositories)
	mux.HandleFunc("GET /v1/github/installations/{installationID}/repositories", a.handleListInstallationRepositories)
	mux.HandleFunc("POST /v1/github/pipeline", a.handleFetchPipeline)
	mux.HandleFunc("POST /v1/checks/create", a.handleCreateCheckRun)
	mux.HandleFunc("POST /v1/checks/initialize", a.handleInitializeCheckRun)
	mux.HandleFunc("POST /v1/checks/find-suite-run", a.handleFindSuiteCheckRun)
	mux.HandleFunc("POST /v1/checks/cancel-stale", a.handleCancelStaleCheckRuns)
	mux.HandleFunc("/v1/run/status", a.handleRunStatusUpdate)
	mux.HandleFunc("/v1/task/status", a.handleTaskStatusUpdate)
	mux.HandleFunc("/v1/checks/create-child", a.handleCreateChildCheckRun)
	return servicelog.HTTPMiddleware(httpapi.LimitRequestBody(a.authenticateInternalRoutes(mux), httpapi.DefaultMaxRequestBodyBytes))
}

func (a *GitBotApp) verifySignature(r *http.Request, body []byte) bool {
	if a == nil || strings.TrimSpace(a.webhookSecret) == "" {
		return false
	}
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

func (a *GitBotApp) createCheckRun(owner, repo, ref, pipelineDef, pipelineSource string) (int64, error) {
	var pipeline models.Pipeline
	_ = yaml.Unmarshal([]byte(pipelineDef), &pipeline)

	checkName := pipeline.Name
	if checkName == "" {
		checkName = "Nopsai Pipeline"
	}
	if pipelineSource == "database override" {
		checkName = fmt.Sprintf("%s-overridden", checkName)
	}

	checkRunID, err := a.checksProvider.CreateQueued(context.Background(), createQueuedCheckRunRequest{
		Owner: owner,
		Repo:  repo,
		Ref:   ref,
		Name:  checkName,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to create check run")
		return 0, err
	}

	// Use the centralized helper to initialize the state
	if err := a.initializeCheckRunState(checkRunID, owner, repo, pipelineDef, checkName); err != nil {
		log.Error().Err(err).Msg("Failed to initialize check run state")
		// Conclude the check run as a failure since we can't track it
		a.concludeCheckRun(owner, repo, checkRunID, "failure", "Failed to initialize internal tracking state for this pipeline.")
		return 0, err
	}

	if err := a.checksProvider.MarkInProgress(context.Background(), checkRunProgressUpdate{
		Owner:      owner,
		Repo:       repo,
		CheckRunID: checkRunID,
		Name:       checkName,
		Title:      checkName,
		Summary:    "Pipeline is starting...",
	}); err != nil {
		log.Error().Err(err).Int64("check_run_id", checkRunID).Msg("Failed to mark check run in progress")
	}

	return checkRunID, nil
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

	checkRunID, err := a.checksProvider.CreateQueued(context.Background(), createQueuedCheckRunRequest{
		Owner: req.Owner,
		Repo:  req.Repo,
		Ref:   req.Ref,
		Name:  checkName,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to create child check run")
		writeProviderError(w, err, "failed to create child check run")
		return
	}

	// Use the centralized helper to initialize the state
	if err := a.initializeCheckRunState(checkRunID, req.Owner, req.Repo, req.PipelineDefinition, checkName); err != nil {
		log.Error().Err(err).Msg("Failed to initialize state for child check run")
		a.concludeCheckRun(req.Owner, req.Repo, checkRunID, "failure", "Failed to initialize internal tracking state for this included pipeline.")
		_ = httpapi.WriteJSONError(w, http.StatusInternalServerError, "failed to initialize internal state")
		return
	}

	if err := a.checksProvider.MarkInProgress(context.Background(), checkRunProgressUpdate{
		Owner:      req.Owner,
		Repo:       req.Repo,
		CheckRunID: checkRunID,
		Name:       checkName,
	}); err != nil {
		log.Error().Err(err).Int64("check_run_id", checkRunID).Msg("Failed to mark child check run in progress")
	}

	_ = httpapi.WriteJSON(w, http.StatusOK, map[string]int64{"check_run_id": checkRunID})
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

	initialState := &checkrender.State{
		Steps:              make(map[string]map[string]checkrender.TaskStatusUpdate),
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
		initialState.Steps[stepName] = make(map[string]checkrender.TaskStatusUpdate)

		if step.GetInclude() != "" {
			initialState.Steps[stepName][stepName] = checkrender.TaskStatusUpdate{
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
				initialState.Steps[stepName][task.Name] = checkrender.TaskStatusUpdate{
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
			initialState.Steps[stepName][stepName] = checkrender.TaskStatusUpdate{
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
	body, err := httpapi.ReadRequestBody(w, r, httpapi.DefaultMaxWebhookBodyBytes)
	if err != nil {
		if httpapi.IsRequestBodyTooLarge(err) {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Could not read request body", http.StatusInternalServerError)
		return
	}

	if a == nil || strings.TrimSpace(a.webhookSecret) == "" || a.webhookForwarder == nil {
		http.Error(w, githubIntegrationUnavailableMessage, http.StatusServiceUnavailable)
		return
	}
	if !a.verifySignature(r, body) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}
	installationID, ok := githubWebhookInstallationID(body)
	if !ok {
		http.Error(w, "Missing GitHub App installation", http.StatusBadRequest)
		return
	}
	if a.clientResolver == nil {
		http.Error(w, githubIntegrationUnavailableMessage, http.StatusServiceUnavailable)
		return
	}
	if _, err := a.clientResolver.InstallationForID(r.Context(), installationID); err != nil {
		writeProviderError(w, err, "GitHub App installation is not registered")
		return
	}
	r.Header.Set("X-GitHub-Installation-ID", fmt.Sprintf("%d", installationID))

	a.webhookForwarder.ForwardWebhook(w, r, body)
}

func githubWebhookInstallationID(body []byte) (int64, bool) {
	var envelope struct {
		Installation *struct {
			ID int64 `json:"id"`
		} `json:"installation"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return 0, false
	}
	if envelope.Installation == nil || envelope.Installation.ID == 0 {
		return 0, false
	}
	return envelope.Installation.ID, true
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

	content, err := a.repositoryProvider.FetchFile(r.Context(), req)
	if err != nil {
		log.Error().Err(err).Str("owner", req.Owner).Str("repo", req.Repo).Str("path", req.Path).Msg("Failed to fetch repository file")
		writeProviderError(w, err, "failed to fetch file")
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

	files, err := a.repositoryProvider.FetchDirectory(r.Context(), req)
	if err != nil {
		log.Error().Err(err).Str("owner", req.Owner).Str("repo", req.Repo).Str("path", req.Path).Msg("Failed to fetch repository contents")
		writeProviderError(w, err, "failed to fetch repository contents")
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
	if a == nil || a.clientResolver == nil {
		_ = httpapi.WriteJSONError(w, http.StatusServiceUnavailable, githubIntegrationUnavailableMessage)
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
	client, _, err := a.clientResolver.ClientForRepository(ctx, req.Owner, req.Repo)
	if err != nil {
		writeProviderError(w, err, "failed to resolve GitHub App installation")
		return
	}
	baseRef, _, err := client.Git.GetRef(ctx, req.Owner, req.Repo, baseRefName)
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

	targetRef, _, err := client.Git.GetRef(ctx, req.Owner, req.Repo, branchName)
	if err != nil {
		var ghErr *github.ErrorResponse
		if !errors.As(err, &ghErr) || ghErr.Response == nil || ghErr.Response.StatusCode != http.StatusNotFound {
			writeGitHubCommitError(w, err, http.StatusBadGateway, "failed to read push branch")
			return
		}
		targetRef, _, err = client.Git.CreateRef(ctx, req.Owner, req.Repo, &github.Reference{
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
	parentCommit, _, err := client.Git.GetCommit(ctx, req.Owner, req.Repo, parentSHA)
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

	tree, _, err := client.Git.CreateTree(ctx, req.Owner, req.Repo, baseTreeSHA, entries)
	if err != nil {
		writeGitHubCommitError(w, err, http.StatusBadGateway, "failed to create push tree")
		return
	}
	commit, _, err := client.Git.CreateCommit(ctx, req.Owner, req.Repo, &github.Commit{
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

	_, _, err = client.Git.UpdateRef(ctx, req.Owner, req.Repo, &github.Reference{
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

	response, err := a.repositoryProvider.CheckAccess(r.Context(), req)
	if err != nil {
		log.Error().Err(err).Str("owner", req.Owner).Str("repo", req.Repo).Msg("Failed to verify repository access")
		writeProviderError(w, err, "failed to verify repository access")
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, response)
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

	response, err := a.repositoryProvider.BranchHasOpenPR(r.Context(), req)
	if err != nil {
		log.Error().Err(err).Str("owner", req.Owner).Str("repo", req.Repo).Str("branch", req.Branch).Msg("Failed to check open pull requests for branch")
		writeProviderError(w, err, "failed to check pull requests")
		return
	}

	if err := httpapi.WriteJSON(w, http.StatusOK, response); err != nil {
		log.Error().Err(err).Msg("Failed to encode branch PR response")
	}
}

func (a *GitBotApp) handleListInstalledRepositories(w http.ResponseWriter, r *http.Request) {
	repositories, err := a.repositoryProvider.ListInstalled(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("Failed to list GitHub App installation repositories")
		writeProviderError(w, err, "failed to list installation repositories")
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, InstalledRepositoriesResponse{Repositories: repositories})
}

func (a *GitBotApp) handleListInstallationRepositories(w http.ResponseWriter, r *http.Request) {
	installationID, err := ParseGitHubInstallationID(r.PathValue("installationID"))
	if err != nil {
		_ = httpapi.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	repositories, err := a.repositoryProvider.ListInstalledForInstallation(r.Context(), installationID)
	if err != nil {
		log.Error().Err(err).Int64("installation_id", installationID).Msg("Failed to list GitHub App installation repositories")
		writeProviderError(w, err, "failed to list installation repositories")
		return
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

	content, err := a.repositoryProvider.FetchPipeline(r.Context(), req)
	if err != nil {
		log.Error().Err(err).Str("owner", req.Owner).Str("repo", req.Repo).Str("path", req.Source.Path).Msg("Failed to fetch pipeline file")
		writeProviderError(w, err, "failed to fetch pipeline file")
		return
	}

	_ = httpapi.WriteJSON(w, http.StatusOK, PipelineContentResponse{Content: content})
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

	checkRunID, err := a.createCheckRun(req.Owner, req.Repo, req.Ref, req.PipelineDefinition, req.PipelineSource)
	if err != nil {
		writeProviderError(w, err, "failed to create check run")
		return
	}
	if checkRunID == 0 {
		_ = httpapi.WriteJSONError(w, http.StatusBadGateway, "created check run did not include an id")
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

	if err := a.checksProvider.MarkInProgress(context.Background(), checkRunProgressUpdate{
		Owner:      req.Owner,
		Repo:       req.Repo,
		CheckRunID: req.CheckRunID,
		Name:       req.PipelineName,
		Title:      req.PipelineName,
		Summary:    "Pipeline is starting...",
	}); err != nil {
		log.Error().Err(err).Int64("check_run_id", req.CheckRunID).Msg("Failed to mark check run in progress")
		writeProviderError(w, err, "failed to update check run")
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

	checkRuns, err := a.checksProvider.ListForRef(context.Background(), req.Owner, req.Repo, req.BeforeSHA)
	if err != nil {
		log.Error().Err(err).Str("owner", req.Owner).Str("repo", req.Repo).Str("sha", req.BeforeSHA).Msg("Failed to list check runs for stale commit")
		writeProviderError(w, err, "failed to list check runs")
		return
	}

	cancelled := 0
	for _, cr := range checkRuns {
		isOurApp := cr.HasApp && cr.AppID == a.githubAppID
		isRunning := cr.Status == "queued" || cr.Status == "in_progress"
		if !isOurApp || !isRunning {
			continue
		}

		if err := a.checksProvider.Conclude(context.Background(), checkRunConclusionUpdate{
			Owner:       req.Owner,
			Repo:        req.Repo,
			CheckRunID:  cr.ID,
			Name:        cr.Name,
			Conclusion:  "cancelled",
			Title:       cr.Name + " - Cancelled",
			Summary:     "This run was cancelled because a new commit was pushed to the branch.",
			CompletedAt: time.Now(),
		}); err != nil {
			log.Error().Err(err).Int64("check_run_id", cr.ID).Msg("Failed to cancel stale check run")
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

	checkRuns, err := a.checksProvider.ListForRef(context.Background(), req.Owner, req.Repo, req.CommitSHA)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list check runs for commit")
		writeProviderError(w, err, "failed to list check runs for commit")
		return
	}
	if len(checkRuns) == 0 {
		_ = httpapi.WriteJSONError(w, http.StatusNotFound, "no check runs found for suite")
		return
	}

	var target *checkRunSummary
	for i := range checkRuns {
		cr := &checkRuns[i]
		if cr.HasCheckSuite && cr.CheckSuiteID == req.SuiteID {
			target = cr
			break
		}
		if cr.HasApp && cr.AppID == a.githubAppID {
			target = cr
			break
		}
	}
	if target == nil {
		target = &checkRuns[0]
	}

	response := FindSuiteCheckRunResponse{
		CheckRunID:         target.ID,
		HeadSHA:            target.HeadSHA,
		HeadBranch:         target.HeadBranch,
		PullRequestHeadRef: target.PullRequestHeadRef,
	}

	_ = httpapi.WriteJSON(w, http.StatusOK, response)
}

func (a *GitBotApp) handleTaskStatusUpdate(w http.ResponseWriter, r *http.Request) {
	var update checkrender.TaskStatusUpdate
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
			state.Steps[update.StepName] = make(map[string]checkrender.TaskStatusUpdate)
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

	if err := a.checksProvider.MarkInProgress(context.Background(), checkRunProgressUpdate{
		Owner:      update.RepoOwner,
		Repo:       update.RepoName,
		CheckRunID: update.CheckRunID,
		Name:       state.PipelineName,
		Title:      newTitle,
		Summary:    summary,
	}); err != nil {
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

	displayConclusion := checkRunConclusionTitle(conclusion)
	githubConclusion := githubCheckRunConclusion(conclusion)
	state, ok := a.checkRunStates[checkRunID]
	if !ok {
		log.Warn().Int64("check_run_id", checkRunID).Msg("State not found for check run, cannot conclude with final name.")
		if err := a.checksProvider.Conclude(context.Background(), checkRunConclusionUpdate{
			Owner:       owner,
			Repo:        repo,
			CheckRunID:  checkRunID,
			Name:        "Nopsai Pipeline",
			Conclusion:  githubConclusion,
			Title:       "Nopsai Pipeline - " + displayConclusion,
			Summary:     summary,
			CompletedAt: time.Now(),
		}); err != nil {
			log.Error().Err(err).Msg("Failed to conclude check run with fallback name")
		}
		return
	}

	// The check run name remains clean and unchanged.
	finalName := state.PipelineName

	// Construct the final title with the ID and conclusion.
	finalTitle := fmt.Sprintf("%s - %s", state.PipelineName, displayConclusion)
	if state.RunID != "" {
		shortRunID := state.RunID[:8]
		finalTitle = fmt.Sprintf("%s (%s) - %s", state.PipelineName, shortRunID, displayConclusion)
	}

	if err := a.checksProvider.Conclude(context.Background(), checkRunConclusionUpdate{
		Owner:       owner,
		Repo:        repo,
		CheckRunID:  checkRunID,
		Name:        finalName,
		Conclusion:  githubConclusion,
		Title:       finalTitle,
		Summary:     summary,
		CompletedAt: time.Now(),
	}); err != nil {
		log.Error().Err(err).Msg("Failed to conclude check run")
	}

	delete(a.checkRunStates, checkRunID)
}

func githubCheckRunConclusion(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success":
		return "success"
	case "warning":
		return "neutral"
	case "cancelled":
		return "cancelled"
	case "timed_out":
		return "timed_out"
	case "skipped":
		return "skipped"
	default:
		return "failure"
	}
}

func checkRunConclusionTitle(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "warning":
		return "Warning"
	case "timed_out":
		return "Timed Out"
	case "":
		return "Failure"
	default:
		return strings.Title(strings.ReplaceAll(status, "_", " "))
	}
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

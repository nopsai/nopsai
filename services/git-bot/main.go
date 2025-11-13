package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"nopsai/config"
	"nopsai/pkg/models"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v53/github"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

type GitBotApp struct {
	cfg            *config.Config
	ghClient       *github.Client
	webhookSecret  string
	checkRunStates map[int64]*CheckRunState
	stateLock      sync.Mutex
	githubAppID    int64
}

// TaskStatusUpdate reflects the new granular status updates.
type TaskStatusUpdate struct {
	RunID      string    `json:"run_id"`
	RepoOwner  string    `json:"repo_owner"`
	RepoName   string    `json:"repo_name"`
	CheckRunID int64     `json:"check_run_id"`
	StepName   string    `json:"step_name"`
	TaskName   string    `json:"task_name"`
	TaskStatus string    `json:"task_status"`
	TaskIndex  int       `json:"task_index"`
	TotalTasks int       `json:"total_tasks"`
	DependsOn  []string  `json:"depends_on"`
	GitHubView string    `json:"github_view"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

// CheckRunState now stores tasks nested within steps.
type CheckRunState struct {
	RunID              string
	Steps              map[string]map[string]TaskStatusUpdate
	StepOrder          []string
	GitHubView         string
	PipelineName       string
	PipelineDefinition string
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

type errorResponse struct {
	Error string `json:"error"`
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

func (a *GitBotApp) renderMarkdownTree(state *CheckRunState) string {
	var builder strings.Builder
	allTasks := make(map[string]TaskStatusUpdate)

	// Maps a task to the list of tasks that depend on it.
	children := make(map[string][]string)
	isChild := make(map[string]bool)

	// Consolidate all tasks into a single map and initialize helper maps
	for _, stepTasks := range state.Steps {
		for taskName, task := range stepTasks {
			allTasks[taskName] = task
			children[taskName] = []string{}
			isChild[taskName] = false
		}
	}

	// Build the dependency graph
	for taskName, task := range allTasks {
		if len(task.DependsOn) > 0 {
			isChild[taskName] = true
			for _, depName := range task.DependsOn {
				if _, ok := allTasks[depName]; ok {
					children[depName] = append(children[depName], taskName)
				}
			}
		}
	}

	// ** NEW **: Keep track of rendered tasks to avoid duplicates
	renderedTasks := make(map[string]bool)

	// A recursive function to render a task and its descendants
	var renderNode func(taskName string, level int)
	renderNode = func(taskName string, level int) {
		// ** FIXED **: Check if the task has already been rendered
		if renderedTasks[taskName] {
			indentation := strings.Repeat("  ", level)
			builder.WriteString(fmt.Sprintf("%s- `%s` *(already shown above)*\n", indentation, taskName))
			return
		}
		renderedTasks[taskName] = true

		task := allTasks[taskName]

		icon := "⏳"
		duration := ""
		if !task.StartedAt.IsZero() && !task.FinishedAt.IsZero() {
			duration = fmt.Sprintf(" (took %s)", task.FinishedAt.Sub(task.StartedAt).Round(time.Second))
		}
		switch {
		case task.TaskStatus == "success":
			icon = "✅"
		case strings.Contains(strings.ToLower(task.TaskStatus), "failure (ignored)"):
			icon = "⚠️"
		case strings.Contains(strings.ToLower(task.TaskStatus), "fail"):
			icon = "❌"
		case task.TaskStatus == "skipped":
			icon = "⚪️"
		case task.TaskStatus == "not_found":
			icon = "❓"
		}

		indentation := strings.Repeat("  ", level)
		builder.WriteString(fmt.Sprintf("%s- %s **%s**: `%s` - %s%s\n", indentation, icon, task.StepName, task.TaskName, task.TaskStatus, duration))

		// Sort children by their original task index for deterministic order
		taskChildren := children[taskName]
		sort.SliceStable(taskChildren, func(i, j int) bool {
			return allTasks[taskChildren[i]].TaskIndex < allTasks[taskChildren[j]].TaskIndex
		})

		// Recursively render children
		for _, childName := range taskChildren {
			renderNode(childName, level+1)
		}
	}

	// Find and render root nodes (tasks that are not children of any other task)
	var rootTasks []string
	for taskName := range allTasks {
		if !isChild[taskName] {
			rootTasks = append(rootTasks, taskName)
		}
	}

	// Sort root tasks by their original task index for deterministic order
	sort.SliceStable(rootTasks, func(i, j int) bool {
		return allTasks[rootTasks[i]].TaskIndex < allTasks[rootTasks[j]].TaskIndex
	})

	for _, taskName := range rootTasks {
		renderNode(taskName, 0)
	}

	return builder.String()
}

func (a *GitBotApp) renderMermaidGraph(state *CheckRunState) string {
	var builder strings.Builder
	builder.WriteString("```mermaid\n")
	builder.WriteString("graph TD\n")

	// Style Definitions
	builder.WriteString("\n    %% --- Style Definitions ---\n")
	builder.WriteString("    classDef success fill:#1a3021,stroke:#3fb950,color:#c9d1d9\n")
	builder.WriteString("    classDef failure fill:#38191c,stroke:#f85149,color:#c9d1d9\n")
	builder.WriteString("    classDef ignored fill:#34291a,stroke:#d29922,color:#c9d1d9\n")
	builder.WriteString("    classDef pending fill:#242930,stroke:#6e7681,color:#c9d1d9\n")
	builder.WriteString("    classDef skipped fill:#242930,stroke:#6e7681,color:#c9d1d9\n")
	builder.WriteString("    linkStyle default stroke:#6e7681,stroke-width:1px\n")

	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(state.PipelineDefinition), &pipeline); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal pipeline definition for Mermaid graph")
		return "Error: Could not render dependency graph."
	}

	taskToNodeID := make(map[string]string)
	stepStartNodes := make(map[string]string)
	stepEndNodes := make(map[string]string)

	// 1. Define all task nodes and invisible step boundary nodes
	builder.WriteString("\n    %% --- Node Definitions ---\n")
	nodeCounter := 0
	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		tasksInStep := state.Steps[stepName]

		// Create invisible start and end nodes for each step to act as hubs
		stepStartNodes[stepName] = fmt.Sprintf("S%d_start", nodeCounter)
		stepEndNodes[stepName] = fmt.Sprintf("S%d_end", nodeCounter)
		builder.WriteString(fmt.Sprintf("    %s(( )); style %s fill:none,stroke:none,width:0,height:0\n", stepStartNodes[stepName], stepStartNodes[stepName]))
		builder.WriteString(fmt.Sprintf("    %s(( )); style %s fill:none,stroke:none,width:0,height:0\n", stepEndNodes[stepName], stepEndNodes[stepName]))

		for taskName, task := range tasksInStep {
			nodeID := fmt.Sprintf("T%d", nodeCounter)
			taskToNodeID[taskName] = nodeID
			nodeCounter++

			var statusIcon, styleClass string
			var duration string
			if !task.StartedAt.IsZero() && !task.FinishedAt.IsZero() {
				duration = fmt.Sprintf("<br/>%s", task.FinishedAt.Sub(task.StartedAt).Round(time.Second))
			}

			switch {
			case task.TaskStatus == "success":
				statusIcon, styleClass = "✅", "success"
			case strings.Contains(task.TaskStatus, "failure (ignored)"):
				statusIcon, styleClass = "⚠️", "ignored"
			case strings.Contains(task.TaskStatus, "fail"):
				statusIcon, styleClass = "❌", "failure"
			case task.TaskStatus == "skipped", task.TaskStatus == "not_found":
				statusIcon, styleClass = "⚪️", "skipped"
			default:
				statusIcon, styleClass = "⏳", "pending"
			}

			nodeText := fmt.Sprintf("%s %s:<br/>%s%s", statusIcon, task.StepName, task.TaskName, duration)
			builder.WriteString(fmt.Sprintf("    %s(\"`%s`\"):::%s\n", nodeID, nodeText, styleClass))
		}
	}

	// 2. Define all dependency links
	builder.WriteString("\n    %% --- Link Definitions ---\n")
	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		tasksInStep := state.Steps[stepName]
		internalDependencies := make(map[string]bool)

		// 2a. Link internal tasks (tasks that depend on other tasks in the same step)
		for taskName, task := range tasksInStep {
			toNode := taskToNodeID[taskName]
			if len(task.DependsOn) > 0 {
				for _, depName := range task.DependsOn {
					fromNode := taskToNodeID[depName]
					builder.WriteString(fmt.Sprintf("    %s --> %s\n", fromNode, toNode))
					internalDependencies[taskName] = true
				}
			}
		}

		// 2b. Link the step's invisible start node to all of its initial tasks
		for taskName := range tasksInStep {
			if !internalDependencies[taskName] {
				builder.WriteString(fmt.Sprintf("    %s --> %s\n", stepStartNodes[stepName], taskToNodeID[taskName]))
			}
		}

		// 2c. Link all of the step's terminal tasks to its invisible end node
		allTaskDeps := make(map[string]bool)
		for _, task := range tasksInStep {
			for _, dep := range task.DependsOn {
				allTaskDeps[dep] = true
			}
		}
		for taskName := range tasksInStep {
			if !allTaskDeps[taskName] {
				builder.WriteString(fmt.Sprintf("    %s --> %s\n", taskToNodeID[taskName], stepEndNodes[stepName]))
			}
		}

		// 2d. Link the invisible end node of dependency steps to the invisible start node of this step
		if len(step.GetDependsOn()) > 0 {
			for _, depStepName := range step.GetDependsOn() {
				builder.WriteString(fmt.Sprintf("    %s --> %s\n", stepEndNodes[depStepName], stepStartNodes[stepName]))
			}
		}
	}

	builder.WriteString("```\n")
	return builder.String()
}

// Renders a flat list of tasks, prefixed with their step name.
func (a *GitBotApp) renderMarkdownFlatList(state *CheckRunState) string {
	var builder strings.Builder
	allTasks := []TaskStatusUpdate{}

	for _, stepName := range state.StepOrder {
		tasks := state.Steps[stepName]
		for _, task := range tasks {
			allTasks = append(allTasks, task)
		}
	}

	// Sort all tasks globally by their index
	sort.SliceStable(allTasks, func(i, j int) bool {
		return allTasks[i].TaskIndex < allTasks[j].TaskIndex
	})

	for _, task := range allTasks {
		icon := "⏳"
		duration := ""
		if !task.StartedAt.IsZero() && !task.FinishedAt.IsZero() {
			duration = fmt.Sprintf(" (took %s)", task.FinishedAt.Sub(task.StartedAt).Round(time.Second))
		}
		if task.TaskStatus == "success" {
			icon = "✅"
		} else if strings.Contains(strings.ToLower(task.TaskStatus), "failure (ignored)") {
			icon = "⚠️"
		} else if strings.Contains(strings.ToLower(task.TaskStatus), "fail") {
			icon = "❌"
		} else if task.TaskStatus == "skipped" {
			icon = "⚪️"
		} else if task.TaskStatus == "not_found" {
			icon = "❓"
		}
		builder.WriteString(fmt.Sprintf("- %s **%s**: `%s` - %s%s\n", icon, task.StepName, task.TaskName, task.TaskStatus, duration))
	}
	return builder.String()
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
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
		http.Error(w, "Failed to create child check run", http.StatusInternalServerError)
		return
	}

	// Use the centralized helper to initialize the state
	if err := a.initializeCheckRunState(*checkRun.ID, req.Owner, req.Repo, req.PipelineDefinition, checkName); err != nil {
		log.Error().Err(err).Msg("Failed to initialize state for child check run")
		a.concludeCheckRun(req.Owner, req.Repo, *checkRun.ID, "failure", "Failed to initialize internal tracking state for this included pipeline.")
		http.Error(w, "Failed to initialize internal state", http.StatusInternalServerError)
		return
	}

	inProgressOpts := github.UpdateCheckRunOptions{
		Name:   checkName,
		Status: github.String("in_progress"),
	}
	a.ghClient.Checks.UpdateCheckRun(context.Background(), req.Owner, req.Repo, *checkRun.ID, inProgressOpts)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"check_run_id": *checkRun.ID})
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

	resp, err := http.DefaultClient.Do(req)
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Owner == "" || req.Repo == "" || req.Path == "" || req.Ref == "" {
		http.Error(w, "owner, repo, ref, and path are required", http.StatusBadRequest)
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
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("owner", req.Owner).Str("repo", req.Repo).Str("path", req.Path).Msg("Failed to fetch repository file")
		http.Error(w, "Failed to fetch file", http.StatusInternalServerError)
		return
	}
	if fileContent == nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	content, err := fileContent.GetContent()
	if err != nil {
		log.Error().Err(err).Str("owner", req.Owner).Str("repo", req.Repo).Str("path", req.Path).Msg("Failed to decode repository file")
		http.Error(w, "Failed to decode file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(FileContentResponse{Content: content})
}

func (a *GitBotApp) handleFetchDirectoryContents(w http.ResponseWriter, r *http.Request) {
	var req DirectoryContentsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Owner == "" || req.Repo == "" || req.Path == "" {
		http.Error(w, "owner, repo, and path are required", http.StatusBadRequest)
		return
	}

	files := make(map[string]string)

	if err := a.collectRepositoryContents(context.Background(), req.Owner, req.Repo, strings.TrimPrefix(req.Path, "/"), req.Ref, files); err != nil {
		var respErr *github.ErrorResponse
		if errors.As(err, &respErr) && respErr.Response != nil && respErr.Response.StatusCode == http.StatusNotFound {
			http.Error(w, "path not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("owner", req.Owner).Str("repo", req.Repo).Str("path", req.Path).Msg("Failed to fetch repository contents")
		http.Error(w, "Failed to fetch repository contents", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(DirectoryContentsResponse{Files: files})
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: message})
}

func (a *GitBotApp) handleCheckRepoAccess(w http.ResponseWriter, r *http.Request) {
	var req RepositoryAccessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Owner == "" || req.Repo == "" {
		writeJSONError(w, http.StatusBadRequest, "owner and repo are required")
		return
	}

	repo, resp, err := a.ghClient.Repositories.Get(context.Background(), req.Owner, req.Repo)
	if err != nil {
		var ghErr *github.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Response != nil {
			switch ghErr.Response.StatusCode {
			case http.StatusNotFound:
				writeJSONError(w, http.StatusNotFound, "repository not found or Git Bot not installed")
				return
			case http.StatusForbidden:
				writeJSONError(w, http.StatusForbidden, "access to repository forbidden for Git Bot")
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
		writeJSONError(w, status, message)
		return
	}

	defaultBranch := ""
	if repo != nil && repo.DefaultBranch != nil {
		defaultBranch = repo.GetDefaultBranch()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RepositoryAccessResponse{Accessible: true, DefaultBranch: defaultBranch})
}

func (a *GitBotApp) handleCheckBranchHasOpenPR(w http.ResponseWriter, r *http.Request) {
	var req BranchPROpenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Owner == "" || req.Repo == "" || req.Branch == "" {
		writeJSONError(w, http.StatusBadRequest, "owner, repo, and branch are required")
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
		writeJSONError(w, http.StatusInternalServerError, "failed to check pull requests")
		return
	}

	response := BranchPROpenResponse{HasOpenPR: len(prs) > 0}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
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

func (a *GitBotApp) handleFetchPipeline(w http.ResponseWriter, r *http.Request) {
	var req PipelineContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Owner == "" || req.Repo == "" || req.Ref == "" {
		http.Error(w, "owner, repo, and ref are required", http.StatusBadRequest)
		return
	}

	var pipelineYAML []byte

	if req.Source.Path != "" {
		if strings.HasPrefix(req.Source.Path, "http://") || strings.HasPrefix(req.Source.Path, "https://") {
			http.Error(w, "Remote pipeline URLs are no longer supported", http.StatusBadRequest)
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
				http.Error(w, "pipeline file not found", http.StatusNotFound)
				return
			}
			log.Error().Err(fetchErr).Str("owner", req.Owner).Str("repo", req.Repo).Str("path", req.Source.Path).Msg("Failed to fetch pipeline file")
			http.Error(w, "Failed to fetch pipeline file", http.StatusInternalServerError)
			return
		}
		if fileContent == nil {
			http.Error(w, "pipeline file not found", http.StatusNotFound)
			return
		}
		content, decodeErr := fileContent.GetContent()
		if decodeErr != nil {
			log.Error().Err(decodeErr).Str("owner", req.Owner).Str("repo", req.Repo).Str("path", req.Source.Path).Msg("Failed to decode pipeline file content")
			http.Error(w, "Failed to decode pipeline file", http.StatusInternalServerError)
			return
		}
		pipelineYAML = []byte(content)
	} else {
		http.Error(w, "pipeline source must include a path", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(PipelineContentResponse{Content: string(pipelineYAML)})
}

func (a *GitBotApp) handleCreateCheckRun(w http.ResponseWriter, r *http.Request) {
	var req CreateCheckRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Owner == "" || req.Repo == "" || req.Ref == "" || req.PipelineDefinition == "" {
		http.Error(w, "owner, repo, ref, and pipeline_definition are required", http.StatusBadRequest)
		return
	}

	if req.PipelineSource == "" {
		req.PipelineSource = "repository"
	}

	checkRunID := a.createCheckRun(req.Owner, req.Repo, req.Ref, req.PipelineDefinition, req.PipelineSource)
	if checkRunID == 0 {
		http.Error(w, "Failed to create check run", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CreateCheckRunResponse{CheckRunID: checkRunID})
}

func (a *GitBotApp) handleInitializeCheckRun(w http.ResponseWriter, r *http.Request) {
	var req InitializeCheckRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Owner == "" || req.Repo == "" || req.CheckRunID == 0 || req.PipelineDefinition == "" || req.PipelineName == "" {
		http.Error(w, "owner, repo, check_run_id, pipeline_definition, and pipeline_name are required", http.StatusBadRequest)
		return
	}

	if err := a.initializeCheckRunState(req.CheckRunID, req.Owner, req.Repo, req.PipelineDefinition, req.PipelineName); err != nil {
		log.Error().Err(err).Int64("check_run_id", req.CheckRunID).Msg("Failed to initialize check run state")
		http.Error(w, "Failed to initialize check run state", http.StatusInternalServerError)
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
		http.Error(w, "Failed to update check run", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (a *GitBotApp) handleCancelStaleCheckRuns(w http.ResponseWriter, r *http.Request) {
	var req CancelStaleCheckRunsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Owner == "" || req.Repo == "" || req.BeforeSHA == "" {
		http.Error(w, "owner, repo, and before_sha are required", http.StatusBadRequest)
		return
	}

	opts := &github.ListCheckRunsOptions{}
	checkRuns, _, err := a.ghClient.Checks.ListCheckRunsForRef(context.Background(), req.Owner, req.Repo, req.BeforeSHA, opts)
	if err != nil {
		log.Error().Err(err).Str("owner", req.Owner).Str("repo", req.Repo).Str("sha", req.BeforeSHA).Msg("Failed to list check runs for stale commit")
		http.Error(w, "Failed to list check runs", http.StatusInternalServerError)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CancelStaleCheckRunsResponse{Cancelled: cancelled})
}

func (a *GitBotApp) handleFindSuiteCheckRun(w http.ResponseWriter, r *http.Request) {
	var req FindSuiteCheckRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Owner == "" || req.Repo == "" || req.SuiteID == 0 {
		http.Error(w, "owner, repo, and suite_id are required", http.StatusBadRequest)
		return
	}

	if req.CommitSHA == "" {
		http.Error(w, "commit_sha is required", http.StatusBadRequest)
		return
	}

	runsResp, _, err := a.ghClient.Checks.ListCheckRunsForRef(context.Background(), req.Owner, req.Repo, req.CommitSHA, &github.ListCheckRunsOptions{})
	if err != nil {
		log.Error().Err(err).Msg("Failed to list check runs for commit")
		http.Error(w, "Failed to list check runs for commit", http.StatusInternalServerError)
		return
	}
	if len(runsResp.CheckRuns) == 0 {
		http.Error(w, "No check runs found for suite", http.StatusNotFound)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (a *GitBotApp) handleTaskStatusUpdate(w http.ResponseWriter, r *http.Request) {
	var update TaskStatusUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
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

	var summary string
	switch state.GitHubView {
	case "mermaid":
		summary = a.renderMermaidGraph(state)
	case "tree":
		summary = a.renderMarkdownTree(state)
	default:
		summary = a.renderMarkdownFlatList(state)
	}

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
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
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
		switch state.GitHubView {
		case "mermaid":
			summary = a.renderMermaidGraph(state)
		case "tree":
			summary = a.renderMarkdownTree(state)
		default:
			summary = a.renderMarkdownFlatList(state)
		}
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
	ghClient := github.NewClient(&http.Client{Transport: installationTransport})

	app := &GitBotApp{
		cfg:            cfg,
		ghClient:       ghClient,
		webhookSecret:  cfg.GitHubWebhookSecret,
		checkRunStates: make(map[int64]*CheckRunState),
		githubAppID:    appID,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", app.handleWebhook)
	mux.HandleFunc("POST /v1/github/file", app.handleFetchFile)
	mux.HandleFunc("POST /v1/github/contents", app.handleFetchDirectoryContents)
	mux.HandleFunc("POST /v1/github/repo/access", app.handleCheckRepoAccess)
	mux.HandleFunc("POST /v1/github/branch/has-open-pr", app.handleCheckBranchHasOpenPR)
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

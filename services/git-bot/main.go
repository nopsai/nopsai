package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
}

// TaskStatusUpdate reflects the new granular status updates.
type TaskStatusUpdate struct {
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
	Steps        map[string]map[string]TaskStatusUpdate
	StepOrder    []string
	GitHubView   string
	PipelineName string
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

// Renders a nested tree view of steps and their tasks.
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
		case task.TaskStatus == "completed":
			icon = "✅"
		case strings.Contains(strings.ToLower(task.TaskStatus), "failed (ignored)"):
			icon = "⚠️"
		case strings.Contains(strings.ToLower(task.TaskStatus), "fail"):
			icon = "❌"
		case task.TaskStatus == "skipped":
			icon = "⚪️"
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

	builder.WriteString("\n    %% Style Definitions\n")
	builder.WriteString("    classDef success fill:#d4edda,stroke:#c3e6cb,color:#155724\n")
	builder.WriteString("    classDef failure fill:#f8d7da,stroke:#f5c6cb,color:#721c24\n")
	builder.WriteString("    classDef ignored fill:#fff3cd,stroke:#ffeeba,color:#856404\n")
	builder.WriteString("    classDef pending fill:#e2e3e5,stroke:#d6d8db,color:#383d41\n")
	builder.WriteString("    classDef skipped fill:#f8f9fa,stroke:#ced4da,color:#6c757d\n")
	builder.WriteString("    classDef root fill:#d1e7dd,stroke:#a3cfbb,color:#0a3622,font-weight:bold\n")

	taskToNodeID := make(map[string]string)
	allTasks := make(map[string]TaskStatusUpdate)
	var repoName string

	// Consolidate all tasks into a single map and get repo name
	for _, tasks := range state.Steps {
		for taskName, task := range tasks {
			allTasks[taskName] = task
			if repoName == "" && task.RepoOwner != "" && task.RepoName != "" {
				repoName = fmt.Sprintf("%s/%s", task.RepoOwner, task.RepoName)
			}
		}
	}

	// Define the root repository node
	if repoName != "" {
		builder.WriteString(fmt.Sprintf("\n    root(\"%s\"):::root\n", repoName))
	}

	// Use the original step order to define nodes for consistent layout
	nodeCounter := 0
	for _, stepName := range state.StepOrder {
		// Create a temporary sorted list of tasks within the step for deterministic node ID assignment
		tasksInStep := []TaskStatusUpdate{}
		for _, task := range state.Steps[stepName] {
			tasksInStep = append(tasksInStep, task)
		}
		sort.SliceStable(tasksInStep, func(i, j int) bool {
			return tasksInStep[i].TaskIndex < tasksInStep[j].TaskIndex
		})

		for _, task := range tasksInStep {
			nodeID := fmt.Sprintf("T%d", nodeCounter)
			taskToNodeID[task.TaskName] = nodeID
			nodeCounter++

			var statusIcon, styleClass string
			var duration string
			if !task.StartedAt.IsZero() && !task.FinishedAt.IsZero() {
				duration = fmt.Sprintf("<br/>%s", task.FinishedAt.Sub(task.StartedAt).Round(time.Second))
			}

			switch {
			case task.TaskStatus == "completed":
				statusIcon, styleClass = "✅", "success"
			case strings.Contains(task.TaskStatus, "failed (ignored)"):
				statusIcon, styleClass = "⚠️", "ignored"
			case strings.Contains(task.TaskStatus, "fail"):
				statusIcon, styleClass = "❌", "failure"
			case task.TaskStatus == "skipped":
				statusIcon, styleClass = "⚪️", "skipped"
			default:
				statusIcon, styleClass = "⏳", "pending"
			}

			// ** FIXED **: Applying the new user-requested format
			nodeText := fmt.Sprintf("%s %s:<br/>%s%s", statusIcon, task.StepName, task.TaskName, duration)
			builder.WriteString(fmt.Sprintf("    %s(\"`%s`\"):::%s\n", nodeID, nodeText, styleClass))
		}
	}

	// Define links between tasks after all nodes are declared
	builder.WriteString("\n    %% Link Definitions\n")
	for taskName, task := range allTasks {
		toNodeID := taskToNodeID[taskName]

		// ** FIXED **: Corrected dependency logic
		if len(task.DependsOn) == 0 {
			// Only link to the root if the task has NO dependencies
			if repoName != "" {
				builder.WriteString(fmt.Sprintf("    root --> %s\n", toNodeID))
			}
		} else {
			// Link from each dependency to this task
			for _, depName := range task.DependsOn {
				if fromNodeID, ok := taskToNodeID[depName]; ok {
					builder.WriteString(fmt.Sprintf("    %s --> %s\n", fromNodeID, toNodeID))
				}
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
		if task.TaskStatus == "completed" {
			icon = "✅"
		} else if strings.Contains(strings.ToLower(task.TaskStatus), "failed (ignored)") {
			icon = "⚠️"
		} else if strings.Contains(strings.ToLower(task.TaskStatus), "fail") {
			icon = "❌"
		} else if task.TaskStatus == "skipped" {
			icon = "⚪️"
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
		checkName = "Nopsai"
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

	inProgressOpts := github.UpdateCheckRunOptions{
		Name:   checkName,
		Status: github.String("in_progress"),
		Output: &github.CheckRunOutput{
			Title:   github.String(repo),
			Summary: github.String("Pipeline is starting..."),
		},
	}
	a.ghClient.Checks.UpdateCheckRun(context.Background(), owner, repo, *checkRun.ID, inProgressOpts)

	view := "flat"
	if pipeline.DisplayOptions.GitHubView == "mermaid" {
		view = "mermaid"
	} else if pipeline.DisplayOptions.GitHubView == "tree" {
		view = "tree"
	}

	a.stateLock.Lock()
	defer a.stateLock.Unlock()

	initialState := &CheckRunState{
		Steps:        make(map[string]map[string]TaskStatusUpdate),
		StepOrder:    []string{},
		GitHubView:   view,
		PipelineName: checkName,
	}

	totalTasks := 0
	for _, step := range pipeline.Steps {
		if len(step.Tasks) > 0 {
			totalTasks += len(step.Tasks)
		} else {
			totalTasks++
		}
	}

	taskIndex := 1
	for _, step := range pipeline.Steps {
		initialState.StepOrder = append(initialState.StepOrder, step.Name)
		initialState.Steps[step.Name] = make(map[string]TaskStatusUpdate)
		if len(step.Tasks) > 0 {
			for _, task := range step.Tasks {
				initialState.Steps[step.Name][task.Name] = TaskStatusUpdate{
					StepName:   step.Name,
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
			// Handle legacy step as a single task
			initialState.Steps[step.Name][step.Name] = TaskStatusUpdate{
				StepName:   step.Name,
				TaskName:   step.Name,
				TaskStatus: "pending",
				TaskIndex:  taskIndex,
				TotalTasks: totalTasks,
				DependsOn:  step.DependsOn,
				RepoOwner:  owner,
				RepoName:   repo,
			}
			taskIndex++
		}
	}

	a.checkRunStates[*checkRun.ID] = initialState

	return *checkRun.ID
}

func (a *GitBotApp) findPipelineForEvent(manifest models.Manifest, eventType, ref string) (string, string) {
	for _, trigger := range manifest.Triggers {
		if trigger.On != eventType {
			continue
		}

		// Handle push events (branches and tags)
		if eventType == "push" {
			if strings.HasPrefix(ref, "refs/heads/") { // It's a branch
				branchName := strings.TrimPrefix(ref, "refs/heads/")
				for _, pattern := range trigger.Branches {
					if matched, _ := filepath.Match(pattern, branchName); matched {
						return trigger.Path, trigger.Environment
					}
				}
			} else if strings.HasPrefix(ref, "refs/tags/") { // It's a tag
				tagName := strings.TrimPrefix(ref, "refs/tags/")
				for _, pattern := range trigger.Tags {
					if matched, _ := filepath.Match(pattern, tagName); matched {
						return trigger.Path, trigger.Environment
					}
				}
			}
		}

		// Handle pull_request events
		if eventType == "pull_request" {
			return trigger.Path, trigger.Environment
		}
	}
	return "", ""
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

	eventType := github.WebHookType(r)
	payload, err := github.ParseWebHook(eventType, body)
	if err != nil {
		http.Error(w, "Could not parse webhook", http.StatusBadRequest)
		return
	}

	var repoName, commitSHA, owner, repo, ref string
	var headCommit *github.HeadCommit
	var pusher *github.User

	// This entire switch statement for parsing the event remains unchanged.
	switch event := payload.(type) {
	case *github.PushEvent:
		if event.GetAfter() == "0000000000000000000000000000000000000000" {
			log.Info().Msg("Ignoring push event for branch deletion.")
			w.WriteHeader(http.StatusOK)
			return
		}
		if event.Repo == nil || event.Repo.Owner == nil || event.Repo.Owner.Login == nil ||
			event.Repo.Name == nil || event.Repo.FullName == nil || event.After == nil {
			log.Warn().Msg("Received push event with missing essential repository, owner, or commit SHA. Ignoring.")
			w.WriteHeader(http.StatusOK)
			return
		}
		repoName = *event.Repo.FullName
		commitSHA = *event.After
		owner = *event.Repo.Owner.Login
		repo = *event.Repo.Name
		ref = *event.Ref
		headCommit = event.HeadCommit
		pusher = event.Pusher
		log.Info().Str("repo", repoName).Str("commit", commitSHA).Msg("Processing push event")

	case *github.PullRequestEvent:
		if event.GetAction() == "closed" {
			log.Info().Msg("Ignoring pull_request event with 'closed' action to prevent duplicate runs on merge.")
			w.WriteHeader(http.StatusOK)
			return
		}
		eventType = "pull_request" // Normalize event type
		if event.Repo == nil || event.Repo.Owner == nil || event.Repo.Owner.Login == nil ||
			event.Repo.Name == nil || event.Repo.FullName == nil || event.PullRequest == nil || event.PullRequest.Head == nil || event.PullRequest.Head.SHA == nil {
			log.Warn().Msg("Received pull_request event with missing essential data. Ignoring.")
			w.WriteHeader(http.StatusOK)
			return
		}
		repoName = *event.Repo.FullName
		commitSHA = *event.PullRequest.Head.SHA
		owner = *event.Repo.Owner.Login
		repo = *event.Repo.Name
		ref = *event.PullRequest.Head.Ref
		log.Info().Str("repo", repoName).Str("commit", commitSHA).Msg("Processing pull_request event")
	case *github.CreateEvent:
		log.Info().Msg("Ignoring 'create' event as per configuration.")
		w.WriteHeader(http.StatusOK)
		return
	case *github.CheckRunEvent:
		if event.GetAction() == "rerequested" {
			if event.Repo == nil || event.Repo.Owner == nil || event.Repo.Owner.Login == nil ||
				event.Repo.Name == nil || event.Repo.FullName == nil || event.CheckRun == nil || event.CheckRun.HeadSHA == nil {
				log.Warn().Msg("Received rerequested check_run event with missing essential data. Ignoring.")
				w.WriteHeader(http.StatusOK)
				return
			}
			repoName = *event.Repo.FullName
			commitSHA = *event.CheckRun.HeadSHA
			owner = *event.Repo.Owner.Login
			repo = *event.Repo.Name

			if len(event.CheckRun.PullRequests) > 0 {
				eventType = "pull_request"
				ref = *event.CheckRun.PullRequests[0].Head.Ref
			} else {
				eventType = "push"
				if event.CheckRun.CheckSuite != nil && event.CheckRun.CheckSuite.HeadBranch != nil {
					ref = "refs/heads/" + *event.CheckRun.CheckSuite.HeadBranch
				} else {
					ref = commitSHA
				}
			}
			log.Info().Str("repo", repoName).Str("commit", commitSHA).Str("event_type", eventType).Msg("Processing rerun request from check_run event")
		} else {
			log.Info().Msgf("Received check_run event with action '%s', ignoring.", event.GetAction())
			w.WriteHeader(http.StatusOK)
			return
		}

	case *github.CheckSuiteEvent:
		if event.GetAction() == "rerequested" {
			if event.Repo == nil || event.Repo.Owner == nil || event.Repo.Owner.Login == nil ||
				event.Repo.Name == nil || event.Repo.FullName == nil || event.CheckSuite == nil || event.CheckSuite.HeadSHA == nil {
				log.Warn().Msg("Received rerequested check_suite event with missing essential data. Ignoring.")
				w.WriteHeader(http.StatusOK)
				return
			}
			repoName = *event.Repo.FullName
			commitSHA = *event.CheckSuite.HeadSHA
			owner = *event.Repo.Owner.Login
			repo = *event.Repo.Name

			if len(event.CheckSuite.PullRequests) > 0 {
				eventType = "pull_request"
				ref = *event.CheckSuite.PullRequests[0].Head.Ref
			} else {
				eventType = "push"
				if event.CheckSuite.HeadBranch != nil {
					ref = "refs/heads/" + *event.CheckSuite.HeadBranch
				} else {
					ref = commitSHA
				}
			}
			log.Info().Str("repo", repoName).Str("commit", commitSHA).Str("event_type", eventType).Msg("Processing rerun request from check_suite event")
		} else {
			log.Info().Msgf("Received check_suite event with action '%s', ignoring.", event.GetAction())
			w.WriteHeader(http.StatusOK)
			return
		}

	default:
		log.Info().Msgf("Received unhandled event type '%s', ignoring.", eventType)
		w.WriteHeader(http.StatusOK)
		return
	}

	var manifest models.Manifest
	var pipelineYAML []byte
	var pipelineSource string

	overrideURL := fmt.Sprintf("%s/v1/overrides/%s/%s", a.cfg.GitBotNopsaiAPIURL, owner, repo)
	resp, err := http.Get(overrideURL)
	if err == nil && resp.StatusCode == http.StatusOK {
		pipelineSource = "database override"
		log.Info().Str("repo", repoName).Msg("Found trigger override from nopsai service.")
		overrideBody, _ := io.ReadAll(resp.Body)
		if err := yaml.Unmarshal(overrideBody, &manifest); err != nil {
			log.Error().Err(err).Msg("Failed to parse trigger override manifest")
			checkRunID := a.createCheckRun(owner, repo, commitSHA, "", pipelineSource)
			a.concludeCheckRun(owner, repo, checkRunID, "failure", "Could not parse the trigger override manifest from the database.")
			http.Error(w, "Could not parse trigger override manifest", http.StatusInternalServerError)
			return
		}
		resp.Body.Close()
	} else {
		pipelineSource = "repository"
		log.Info().Str("repo", repoName).Msg("No override found, fetching .nopsai/triggers.yaml from repository.")
		manifestPath := ".nopsai/triggers.yaml"
		fileContent, _, _, err := a.ghClient.Repositories.GetContents(context.Background(), owner, repo, manifestPath, &github.RepositoryContentGetOptions{Ref: commitSHA})
		if err != nil || fileContent == nil {
			log.Error().Err(err).Msg("Failed to fetch .nopsai/triggers.yaml from repository")
			checkRunID := a.createCheckRun(owner, repo, commitSHA, "", pipelineSource)
			a.concludeCheckRun(owner, repo, checkRunID, "failure", "Could not find .nopsai/triggers.yaml in the repository.")
			http.Error(w, "Could not fetch pipeline file", http.StatusNotFound)
			return
		}
		content, err := fileContent.GetContent()
		if err != nil {
			log.Error().Err(err).Msg("Failed to decode file content")
			checkRunID := a.createCheckRun(owner, repo, commitSHA, "", pipelineSource)
			a.concludeCheckRun(owner, repo, checkRunID, "failure", "Could not decode the .nopsai/triggers.yaml file content.")
			http.Error(w, "Could not decode file content", http.StatusInternalServerError)
			return
		}
		if err := yaml.Unmarshal([]byte(content), &manifest); err != nil {
			log.Error().Err(err).Msg("Failed to parse .nopsai/triggers.yaml manifest")
			checkRunID := a.createCheckRun(owner, repo, commitSHA, "", pipelineSource)
			a.concludeCheckRun(owner, repo, checkRunID, "failure", "Could not parse the .nopsai/triggers.yaml manifest file.")
			http.Error(w, "Could not parse manifest file", http.StatusBadRequest)
			return
		}
	}

	pipelinePath, environment := a.findPipelineForEvent(manifest, eventType, ref)
	if pipelinePath == "" {
		log.Info().Msgf("No pipeline found for event '%s' and ref '%s'.", eventType, ref)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("No pipeline found for this event."))
		return
	}

	if pipelineSource == "repository" {
		fullPipelinePath := filepath.Join(".nopsai", pipelinePath)
		log.Info().Msgf("Found pipeline '%s' for event '%s' and ref '%s'.", fullPipelinePath, eventType, ref)
		pipelineFileContent, _, _, err := a.ghClient.Repositories.GetContents(context.Background(), owner, repo, fullPipelinePath, &github.RepositoryContentGetOptions{Ref: commitSHA})
		if err != nil || pipelineFileContent == nil {
			log.Error().Err(err).Msgf("Failed to fetch pipeline file '%s' from repository", fullPipelinePath)
			checkRunID := a.createCheckRun(owner, repo, commitSHA, "", pipelineSource)
			a.concludeCheckRun(owner, repo, checkRunID, "failure", fmt.Sprintf("Could not find pipeline file '%s' in the repository.", fullPipelinePath))
			http.Error(w, "Could not fetch pipeline file", http.StatusNotFound)
			return
		}
		pipelineContent, err := pipelineFileContent.GetContent()
		if err != nil {
			log.Error().Err(err).Msg("Failed to decode pipeline file content")
			checkRunID := a.createCheckRun(owner, repo, commitSHA, "", pipelineSource)
			a.concludeCheckRun(owner, repo, checkRunID, "failure", "Could not decode the pipeline file content.")
			http.Error(w, "Could not decode pipeline file content", http.StatusInternalServerError)
			return
		}
		pipelineYAML = []byte(pipelineContent)
	} else {
		log.Info().Str("pipeline_name", pipelinePath).Msg("Fetching central pipeline definition for override")
		pipelineURL := fmt.Sprintf("%s/v1/pipelines/%s", a.cfg.GitBotNopsaiAPIURL, pipelinePath)
		resp, err := http.Get(pipelineURL)
		if err != nil || resp.StatusCode != http.StatusOK {
			log.Error().Err(err).Msg("Failed to fetch central pipeline definition")
			checkRunID := a.createCheckRun(owner, repo, commitSHA, "", pipelineSource)
			a.concludeCheckRun(owner, repo, checkRunID, "failure", fmt.Sprintf("Could not fetch the central pipeline named '%s'.", pipelinePath))
			http.Error(w, "Could not fetch central pipeline", http.StatusInternalServerError)
			return
		}
		pipelineYAML, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
	}

	checkRunID := a.createCheckRun(owner, repo, commitSHA, string(pipelineYAML), pipelineSource)
	if checkRunID == 0 {
		http.Error(w, "Failed to create check run", http.StatusInternalServerError)
		return
	}

	var runURL string
	var req *http.Request

	if pipelineSource == "repository" {
		runURL = fmt.Sprintf("%s/v1/run", a.cfg.GitBotNopsaiAPIURL)
		req, _ = http.NewRequest("POST", runURL, bytes.NewBuffer(pipelineYAML))
		req.Header.Set("Content-Type", "application/x-yaml")
	} else {
		runURL = fmt.Sprintf("%s/v1/run/%s", a.cfg.GitBotNopsaiAPIURL, pipelinePath)
		req, _ = http.NewRequest("POST", runURL, nil)
	}

	req.Header.Set("X-Git-Repo-Owner", owner)
	req.Header.Set("X-Git-Repo-Name", repo)
	req.Header.Set("X-Git-Commit-SHA", commitSHA)
	req.Header.Set("X-Git-Check-Run-ID", strconv.FormatInt(checkRunID, 10))
	req.Header.Set("X-Nopsai-Environment", environment)

	if pushEvent, ok := payload.(*github.PushEvent); ok {
		if pushEvent.Repo.CloneURL != nil {
			req.Header.Set("X-Git-Clone-URL", *pushEvent.Repo.CloneURL)
		}
		if pushEvent.Repo.SSHURL != nil {
			req.Header.Set("X-Git-SSH-URL", *pushEvent.Repo.SSHURL)
		}
		if pushEvent.Ref != nil {
			req.Header.Set("X-Git-Ref", *pushEvent.Ref)
		}
		if headCommit != nil {
			if headCommit.URL != nil {
				req.Header.Set("X-Git-Commit-URL", *headCommit.URL)
			}
			if headCommit.Message != nil {
				firstLine := strings.Split(*headCommit.Message, "\n")[0]
				req.Header.Set("X-Git-Commit-Message", firstLine)
			}
			if headCommit.Author != nil {
				if headCommit.Author.Name != nil {
					req.Header.Set("X-Git-Commit-Author-Name", *headCommit.Author.Name)
				}
				if headCommit.Author.Email != nil {
					req.Header.Set("X-Git-Commit-Author-Email", *headCommit.Author.Email)
				}
				if headCommit.Author.Login != nil {
					req.Header.Set("X-Git-Commit-Author-Username", *headCommit.Author.Login)
				}
			}
		}
		if pusher != nil {
			if pusher.Name != nil {
				req.Header.Set("X-Git-Pusher-Name", *pusher.Name)
			}
			if pusher.Email != nil {
				req.Header.Set("X-Git-Pusher-Email", *pusher.Email)
			}
		}
	} else if checkRunEvent, ok := payload.(*github.CheckRunEvent); ok {
		if checkRunEvent.Repo.CloneURL != nil {
			req.Header.Set("X-Git-Clone-URL", *checkRunEvent.Repo.CloneURL)
		}
		if checkRunEvent.Repo.SSHURL != nil {
			req.Header.Set("X-Git-SSH-URL", *checkRunEvent.Repo.SSHURL)
		}
		if checkRunEvent.CheckRun.HeadSHA != nil {
			req.Header.Set("X-Git-Ref", *checkRunEvent.CheckRun.HeadSHA)
		}
	} else if checkSuiteEvent, ok := payload.(*github.CheckSuiteEvent); ok {
		if checkSuiteEvent.Repo.CloneURL != nil {
			req.Header.Set("X-Git-Clone-URL", *checkSuiteEvent.Repo.CloneURL)
		}
		if checkSuiteEvent.Repo.SSHURL != nil {
			req.Header.Set("X-Git-SSH-URL", *checkSuiteEvent.Repo.SSHURL)
		}
		if checkSuiteEvent.CheckSuite.HeadSHA != nil {
			req.Header.Set("X-Git-Ref", *checkSuiteEvent.CheckSuite.HeadSHA)
		}
	}

	client := &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		log.Error().Err(err).Int("status_code", 0).Msg("Failed to trigger nopsai pipeline")
		summary := fmt.Sprintf("Failed to trigger Nopsai pipeline.\n\nError: %s", err.Error())
		a.concludeCheckRun(owner, repo, checkRunID, "failure", summary)
		http.Error(w, "Failed to trigger pipeline", http.StatusInternalServerError)
		return
	}

	if resp.StatusCode != http.StatusCreated {
		statusCode := resp.StatusCode
		errorBody, _ := io.ReadAll(resp.Body)
		log.Error().Int("status_code", statusCode).Msg("Nopsai service returned non-OK status")
		summary := fmt.Sprintf("Failed to trigger Nopsai pipeline. The nopsai service responded with status %d.\n\nError: %s", statusCode, string(errorBody))
		a.concludeCheckRun(owner, repo, checkRunID, "failure", summary)
		http.Error(w, "Failed to trigger pipeline", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("Pipeline triggered."))
}

// handleTaskStatusUpdate processes updates for individual tasks.
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

	// ** FIXED **: Safely merge the update instead of overwriting the state.
	// This preserves the original DependsOn array from being erased.
	if existingTask, ok := state.Steps[update.StepName][update.TaskName]; ok {
		existingTask.TaskStatus = update.TaskStatus
		if !update.StartedAt.IsZero() {
			existingTask.StartedAt = update.StartedAt
		}
		if !update.FinishedAt.IsZero() {
			existingTask.FinishedAt = update.FinishedAt
		}
		// Only update dependencies if the incoming update actually provides them.
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
	for _, tasks := range state.Steps {
		for _, task := range tasks {
			totalTasks++
			if task.TaskStatus != "pending" && task.TaskStatus != "skipped" {
				completedTasks++
			}
		}
	}
	newTitle := fmt.Sprintf("In progress... (%d/%d tasks)", completedTasks, totalTasks)

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
	checkName := "Nopsai"
	if ok {
		checkName = state.PipelineName
	}

	finalTitle := checkName + " - " + strings.ToUpper(string(conclusion[0])) + conclusion[1:]

	opts := github.UpdateCheckRunOptions{
		Name:        checkName,
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
		err := os.WriteFile(cfg.GitHubPrivateKeyPath, []byte(correctedKey), 0600)
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
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", app.handleWebhook)
	mux.HandleFunc("/v1/run/status", app.handleRunStatusUpdate)
	mux.HandleFunc("/v1/task/status", app.handleTaskStatusUpdate)

	log.Info().Msgf("Nopsai Git Bot server listening on %s", cfg.GitBotListenAddress)
	if err := http.ListenAndServe(cfg.GitBotListenAddress, mux); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}

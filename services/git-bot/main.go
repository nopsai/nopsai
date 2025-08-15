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

type StepStatusUpdate struct {
	RepoOwner  string   `json:"repo_owner"`
	RepoName   string   `json:"repo_name"`
	CheckRunID int64    `json:"check_run_id"`
	StepName   string   `json:"step_name"`
	StepStatus string   `json:"step_status"`
	StepIndex  int      `json:"step_index"`
	TotalSteps int      `json:"total_steps"`
	DependsOn  []string `json:"depends_on"`
	GitHubView string   `json:"github_view"`
}

type CheckRunState struct {
	Steps      map[string]StepStatusUpdate
	GitHubView string
}

type RunStatusUpdate struct {
	Status     string `json:"status"`
	FailedStep string `json:"failed_step"`
	CheckRunID int64  `json:"check_run_id"`
	RepoOwner  string `json:"repo_owner"`
	RepoName   string `json:"repo_name"`
}

// in services/git-bot/main.go

func (a *GitBotApp) renderMarkdownTree(state *CheckRunState) string {
	var builder strings.Builder
	childrenMap := make(map[string][]string)

	// Create a map of all steps by name for easy lookup
	stepsByName := make(map[string]StepStatusUpdate)
	for _, step := range state.Steps {
		stepsByName[step.StepName] = step
	}

	// Build the dependency tree (childrenMap) from a sorted list of all steps
	allStepNames := make([]string, 0, len(stepsByName))
	for name := range stepsByName {
		allStepNames = append(allStepNames, name)
	}
	// Sort all steps by their original index for consistent processing
	sort.SliceStable(allStepNames, func(i, j int) bool {
		return stepsByName[allStepNames[i]].StepIndex < stepsByName[allStepNames[j]].StepIndex
	})

	for _, stepName := range allStepNames {
		step := stepsByName[stepName]
		if len(step.DependsOn) == 0 {
			// This is a root node
			childrenMap[""] = append(childrenMap[""], step.StepName)
		} else {
			for _, parent := range step.DependsOn {
				childrenMap[parent] = append(childrenMap[parent], step.StepName)
			}
		}
	}

	// This map tracks which steps have been fully rendered (with their children)
	visited := make(map[string]bool)

	var buildTree func(parent string, depth int)
	buildTree = func(parent string, depth int) {
		children, ok := childrenMap[parent]
		if !ok {
			return
		}

		// Sort children to maintain a consistent order
		sort.SliceStable(children, func(i, j int) bool {
			return stepsByName[children[i]].StepIndex < stepsByName[children[j]].StepIndex
		})

		for _, childName := range children {
			step := stepsByName[childName]
			icon := "⏳"
			if step.StepStatus == "completed" {
				icon = "✅"
			} else if strings.Contains(strings.ToLower(step.StepStatus), "failed (ignored)") {
				icon = "⚠️"
			} else if strings.Contains(strings.ToLower(step.StepStatus), "fail") {
				icon = "❌"
			}

			// If we've already rendered this step fully, just show a reference and stop.
			if visited[childName] {
				builder.WriteString(strings.Repeat("  ", depth))
				builder.WriteString(fmt.Sprintf("- %s `%s` (dependency already shown)\n", icon, step.StepName))
				continue // Do not recurse further down this branch
			}

			// Mark the step as visited before rendering it
			visited[childName] = true

			builder.WriteString(strings.Repeat("  ", depth))
			builder.WriteString(fmt.Sprintf("- %s `%s` - %s\n", icon, step.StepName, step.StepStatus))

			// Recursively build the tree for the children of this step
			buildTree(childName, depth+1)
		}
	}

	// Start building the tree from the root nodes
	rootNodes := childrenMap[""]
	sort.SliceStable(rootNodes, func(i, j int) bool {
		return stepsByName[rootNodes[i]].StepIndex < stepsByName[rootNodes[j]].StepIndex
	})

	for _, rootNodeName := range rootNodes {
		// A root node could theoretically be a dependency of another root, so check if visited.
		if visited[rootNodeName] {
			continue
		}
		visited[rootNodeName] = true

		step := stepsByName[rootNodeName]
		icon := "⏳"
		if step.StepStatus == "completed" {
			icon = "✅"
		} else if strings.Contains(strings.ToLower(step.StepStatus), "failed (ignored)") {
			icon = "⚠️"
		} else if strings.Contains(strings.ToLower(step.StepStatus), "fail") {
			icon = "❌"
		}

		builder.WriteString(fmt.Sprintf("- %s `%s` - %s\n", icon, step.StepName, step.StepStatus))
		buildTree(rootNodeName, 1)
	}

	return builder.String()
}

// renderMermaidGraph automatically groups steps into visual stages and adds status icons.
func (a *GitBotApp) renderMermaidGraph(state *CheckRunState) string {
	var builder strings.Builder
	builder.WriteString("```mermaid\n")
	builder.WriteString("graph LR\n")

	// --- 1. Setup ---
	stepsByName := make(map[string]StepStatusUpdate)
	for _, step := range state.Steps {
		stepsByName[step.StepName] = step
	}

	allStepNames := make([]string, 0, len(stepsByName))
	for name := range stepsByName {
		allStepNames = append(allStepNames, name)
	}
	sort.SliceStable(allStepNames, func(i, j int) bool {
		return stepsByName[allStepNames[i]].StepIndex < stepsByName[allStepNames[j]].StepIndex
	})

	// --- 2. Style Class Definitions ---
	builder.WriteString("\n    %% Style Definitions\n")
	builder.WriteString("    classDef success fill:#d4edda,stroke:#c3e6cb,color:#155724\n")
	builder.WriteString("    classDef failure fill:#f8d7da,stroke:#f5c6cb,color:#721c24\n")
	builder.WriteString("    classDef ignored fill:#fff3cd,stroke:#ffeeba,color:#856404\n")
	builder.WriteString("    classDef pending fill:#e2e3e5,stroke:#d6d8db,color:#383d41\n")
	builder.WriteString("    classDef skipped fill:#f8f9fa,stroke:#ced4da,color:#6c757d\n")

	// --- 3. Calculate Stages ---
	stages := make([][]string, 0)
	processedSteps := make(map[string]bool)
	for len(processedSteps) < len(stepsByName) {
		currentStage := make([]string, 0)
		for _, name := range allStepNames {
			if _, ok := processedSteps[name]; ok {
				continue
			}
			step := stepsByName[name]
			dependenciesMet := true
			for _, dep := range step.DependsOn {
				if _, ok := processedSteps[dep]; !ok {
					dependenciesMet = false
					break
				}
			}
			if dependenciesMet {
				currentStage = append(currentStage, name)
			}
		}
		if len(currentStage) == 0 {
			break
		}
		for _, name := range currentStage {
			processedSteps[name] = true
		}
		stages = append(stages, currentStage)
	}

	// --- 4. Define Nodes within Dynamically Named Subgraphs ---
	for i, stage := range stages {
		// **This is the key change: Dynamically create the stage name.**
		stageName := fmt.Sprintf("Dependency Layer - %d", i+1)

		builder.WriteString(fmt.Sprintf("\n    subgraph \"%s\"\n", stageName))
		for _, stepName := range stage {
			step := stepsByName[stepName]
			var statusIcon, styleClass string
			switch {
			case step.StepStatus == "completed":
				statusIcon, styleClass = "✅", "success"
			case strings.Contains(step.StepStatus, "failed (ignored)"):
				statusIcon, styleClass = "⚠️", "ignored"
			case strings.Contains(step.StepStatus, "fail"):
				statusIcon, styleClass = "❌", "failure"
			case step.StepStatus == "skipped":
				statusIcon, styleClass = "⚪", "skipped"
			default:
				statusIcon, styleClass = "⏳", "pending"
			}
			nodeText := fmt.Sprintf("%s %s", statusIcon, step.StepName)
			builder.WriteString(fmt.Sprintf("        %s(\"`%s`\"):::%s\n", stepName, nodeText, styleClass))
		}
		builder.WriteString("    end\n")
	}

	// --- 5. Define Links Between Nodes ---
	builder.WriteString("\n    %% Link Definitions\n")
	for _, stepName := range allStepNames {
		step := stepsByName[stepName]
		for _, parent := range step.DependsOn {
			builder.WriteString(fmt.Sprintf("    %s --> %s\n", parent, stepName))
		}
	}

	builder.WriteString("```\n")
	return builder.String()
}

func (a *GitBotApp) renderMarkdownFlatList(state *CheckRunState) string {
	var builder strings.Builder
	steps := make([]StepStatusUpdate, 0, len(state.Steps))
	for _, step := range state.Steps {
		steps = append(steps, step)
	}

	// Sort steps by their index for a consistent, ordered list
	sort.SliceStable(steps, func(i, j int) bool {
		return steps[i].StepIndex < steps[j].StepIndex
	})

	for _, step := range steps {
		icon := "⏳"
		if step.StepStatus == "completed" {
			icon = "✅"
		} else if strings.Contains(strings.ToLower(step.StepStatus), "failed (ignored)") {
			icon = "⚠️"
		} else if strings.Contains(strings.ToLower(step.StepStatus), "fail") {
			icon = "❌"
		}
		builder.WriteString(fmt.Sprintf("- %s Step %d: `%s` - %s\n", icon, step.StepIndex, step.StepName, step.StepStatus))
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

func (a *GitBotApp) createCheckRun(owner, repo, ref, pipelineDef string) int64 {
	opts := github.CreateCheckRunOptions{
		Name:    "Nopsai", // Use "Nopsai" instead of "Nopsai CI"
		HeadSHA: ref,
		Status:  github.String("queued"),
	}
	checkRun, _, err := a.ghClient.Checks.CreateCheckRun(context.Background(), owner, repo, opts)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create check run")
		return 0
	}

	inProgressOpts := github.UpdateCheckRunOptions{
		Name:   "Nopsai", // Use "Nopsai" here as well
		Status: github.String("in_progress"),
		Output: &github.CheckRunOutput{
			Title:   github.String(repo),
			Summary: github.String("Pipeline is starting..."),
		},
	}
	a.ghClient.Checks.UpdateCheckRun(context.Background(), owner, repo, *checkRun.ID, inProgressOpts)

	var pipeline models.Pipeline
	_ = yaml.Unmarshal([]byte(pipelineDef), &pipeline)
	view := "mermaid"
	if pipeline.DisplayOptions.GitHubView == "flat" {
		view = "flat"
	} else if pipeline.DisplayOptions.GitHubView == "tree" {
		view = "tree"
	}

	a.stateLock.Lock()
	defer a.stateLock.Unlock()

	initialSteps := make(map[string]StepStatusUpdate)
	for i, step := range pipeline.Steps {
		initialSteps[step.Name] = StepStatusUpdate{
			StepName:   step.Name,
			StepStatus: "pending",
			StepIndex:  i + 1,
			TotalSteps: len(pipeline.Steps),
			DependsOn:  step.DependsOn,
			RepoOwner:  owner,
			RepoName:   repo,
		}
	}

	// **FIX**: Use the correct struct name `CheckRunState`.
	a.checkRunStates[*checkRun.ID] = &CheckRunState{
		Steps:      initialSteps,
		GitHubView: view,
	}

	return *checkRun.ID
}
func (a *GitBotApp) findPipelineForEvent(manifest models.Manifest, eventType, ref string) string {
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
						return trigger.Path
					}
				}
			} else if strings.HasPrefix(ref, "refs/tags/") { // It's a tag
				tagName := strings.TrimPrefix(ref, "refs/tags/")
				for _, pattern := range trigger.Tags {
					if matched, _ := filepath.Match(pattern, tagName); matched {
						return trigger.Path
					}
				}
			}
		}

		// Handle pull_request events
		if eventType == "pull_request" {
			return trigger.Path
		}
	}
	return ""
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

			// Logic to determine if it's a PR or push rerun
			if len(event.CheckRun.PullRequests) > 0 {
				eventType = "pull_request"
				ref = *event.CheckRun.PullRequests[0].Head.Ref
			} else {
				eventType = "push"
				if event.CheckRun.CheckSuite != nil && event.CheckRun.CheckSuite.HeadBranch != nil {
					ref = "refs/heads/" + *event.CheckRun.CheckSuite.HeadBranch
				} else {
					ref = commitSHA // Fallback
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

			// Logic to determine if it's a PR or push rerun
			if len(event.CheckSuite.PullRequests) > 0 {
				eventType = "pull_request"
				ref = *event.CheckSuite.PullRequests[0].Head.Ref
			} else {
				eventType = "push"
				if event.CheckSuite.HeadBranch != nil {
					ref = "refs/heads/" + *event.CheckSuite.HeadBranch
				} else {
					ref = commitSHA // Fallback
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

	var pipelineYAML []byte

	overrideURL := fmt.Sprintf("%s/v1/overrides/%s/%s", a.cfg.GitBotNopsaiAPIURL, owner, repo)
	resp, err := http.Get(overrideURL)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		pipelineYAML, _ = io.ReadAll(resp.Body)
		log.Info().Str("repo", repoName).Msg("Found and using pipeline override from nopsai service.")
	} else {
		log.Info().Str("repo", repoName).Msg("No override found, fetching .nopsai/triggers.yaml from repository.")
		manifestPath := ".nopsai/triggers.yaml"
		fileContent, _, _, err := a.ghClient.Repositories.GetContents(context.Background(), owner, repo, manifestPath, &github.RepositoryContentGetOptions{Ref: commitSHA})
		if err != nil || fileContent == nil {
			log.Error().Err(err).Msg("Failed to fetch .nopsai/triggers.yaml from repository")
			checkRunID := a.createCheckRun(owner, repo, commitSHA, "")
			a.concludeCheckRun(owner, repo, checkRunID, "failure", "Could not find .nopsai/triggers.yaml in the repository.")
			http.Error(w, "Could not fetch pipeline file", http.StatusNotFound)
			return
		}
		content, err := fileContent.GetContent()
		if err != nil {
			log.Error().Err(err).Msg("Failed to decode file content")
			checkRunID := a.createCheckRun(owner, repo, commitSHA, "")
			a.concludeCheckRun(owner, repo, checkRunID, "failure", "Could not decode the .nopsai/triggers.yaml file content.")
			http.Error(w, "Could not decode file content", http.StatusInternalServerError)
			return
		}
		var manifest models.Manifest
		if err := yaml.Unmarshal([]byte(content), &manifest); err != nil {
			log.Error().Err(err).Msg("Failed to parse .nopsai/triggers.yaml manifest")
			checkRunID := a.createCheckRun(owner, repo, commitSHA, "")
			a.concludeCheckRun(owner, repo, checkRunID, "failure", "Could not parse the .nopsai/triggers.yaml manifest file.")
			http.Error(w, "Could not parse manifest file", http.StatusBadRequest)
			return
		}

		pipelinePath := a.findPipelineForEvent(manifest, eventType, ref)
		if pipelinePath == "" {
			log.Info().Msgf("No pipeline found for event '%s' and ref '%s'.", eventType, ref)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("No pipeline found for this event."))
			return
		}

		fullPipelinePath := filepath.Join(".nopsai", pipelinePath)

		log.Info().Msgf("Found pipeline '%s' for event '%s' and ref '%s'.", fullPipelinePath, eventType, ref)
		pipelineFileContent, _, _, err := a.ghClient.Repositories.GetContents(context.Background(), owner, repo, fullPipelinePath, &github.RepositoryContentGetOptions{Ref: commitSHA})

		if err != nil || pipelineFileContent == nil {
			log.Error().Err(err).Msgf("Failed to fetch pipeline file '%s' from repository", fullPipelinePath)
			checkRunID := a.createCheckRun(owner, repo, commitSHA, "")
			a.concludeCheckRun(owner, repo, checkRunID, "failure", fmt.Sprintf("Could not find pipeline file '%s' in the repository.", fullPipelinePath))
			http.Error(w, "Could not fetch pipeline file", http.StatusNotFound)
			return
		}
		pipelineContent, err := pipelineFileContent.GetContent()
		if err != nil {
			log.Error().Err(err).Msg("Failed to decode pipeline file content")
			checkRunID := a.createCheckRun(owner, repo, commitSHA, "")
			a.concludeCheckRun(owner, repo, checkRunID, "failure", "Could not decode the pipeline file content.")
			http.Error(w, "Could not decode pipeline file content", http.StatusInternalServerError)
			return
		}
		pipelineYAML = []byte(pipelineContent)
	}

	checkRunID := a.createCheckRun(owner, repo, commitSHA, string(pipelineYAML))
	if checkRunID == 0 {
		http.Error(w, "Failed to create check run", http.StatusInternalServerError)
		return
	}

	runURL := fmt.Sprintf("%s/v1/run", a.cfg.GitBotNopsaiAPIURL)
	req, _ := http.NewRequest("POST", runURL, bytes.NewBuffer(pipelineYAML))
	req.Header.Set("Content-Type", "application/x-yaml")

	req.Header.Set("X-Git-Repo-Owner", owner)
	req.Header.Set("X-Git-Repo-Name", repo)
	req.Header.Set("X-Git-Commit-SHA", commitSHA)
	req.Header.Set("X-Git-Check-Run-ID", strconv.FormatInt(checkRunID, 10))

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
				// Sanitize the commit message for the header
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

func (a *GitBotApp) handleStepStatusUpdate(w http.ResponseWriter, r *http.Request) {
	var update StepStatusUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	log.Info().Int64("check_run_id", update.CheckRunID).Str("step", update.StepName).Msg("Received step status update")

	a.stateLock.Lock()
	defer a.stateLock.Unlock()

	state, ok := a.checkRunStates[update.CheckRunID]
	if !ok {
		log.Error().Int64("check_run_id", update.CheckRunID).Msg("Received step update for unknown check run")
		return
	}
	state.Steps[update.StepName] = update
	if update.GitHubView != "" {
		state.GitHubView = update.GitHubView
	}

	var summary string
	if state.GitHubView == "flat" {
		summary = a.renderMarkdownFlatList(state)
	} else if state.GitHubView == "tree" {
		summary = a.renderMarkdownTree(state)
	} else {
		summary = a.renderMermaidGraph(state)
	}
	newTitle := fmt.Sprintf("In progress... (%d/%d steps)", len(state.Steps), update.TotalSteps)

	opts := github.UpdateCheckRunOptions{
		Name:   "Nopsai",
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

func (a *GitBotApp) handleRunStatusUpdate(w http.ResponseWriter, r *http.Request) {
	var update RunStatusUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Info().Int64("check_run_id", update.CheckRunID).Str("status", update.Status).Msg("Received final pipeline status")

	a.stateLock.Lock()
	state, ok := a.checkRunStates[update.CheckRunID]

	if ok && update.Status == "failure" && update.FailedStep != "" {
		dependents := make(map[string][]string)
		for _, step := range state.Steps {
			for _, dep := range step.DependsOn {
				dependents[dep] = append(dependents[dep], step.StepName)
			}
		}

		queue := []string{update.FailedStep}
		processedForSkip := make(map[string]bool)
		processedForSkip[update.FailedStep] = true

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]

			for _, dependentName := range dependents[current] {
				if !processedForSkip[dependentName] {
					processedForSkip[dependentName] = true
					if step, stepOk := state.Steps[dependentName]; stepOk && step.StepStatus == "pending" {
						step.StepStatus = "skipped"
						state.Steps[dependentName] = step
					}
					queue = append(queue, dependentName)
				}
			}
		}
	}

	summary := ""
	if ok {
		if state.GitHubView == "flat" {
			summary = a.renderMarkdownFlatList(state)
		} else if state.GitHubView == "tree" {
			summary = a.renderMarkdownTree(state)
		} else {
			summary = a.renderMermaidGraph(state)
		}
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

	finalTitle := "Nopsai - " + strings.ToUpper(string(conclusion[0])) + conclusion[1:]

	opts := github.UpdateCheckRunOptions{
		Name:        "Nopsai",
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

	a.stateLock.Lock()
	defer a.stateLock.Unlock()
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
	mux.HandleFunc("/v1/step/status", app.handleStepStatusUpdate)

	log.Info().Msgf("Nopsai Git Bot server listening on %s", cfg.GitBotListenAddress)
	if err := http.ListenAndServe(cfg.GitBotListenAddress, mux); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}

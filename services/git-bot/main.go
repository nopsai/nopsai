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
	CheckRunID int64  `json:"check_run_id"`
	RepoOwner  string `json:"repo_owner"`
	RepoName   string `json:"repo_name"`
}

func (a *GitBotApp) renderMarkdownTree(state *CheckRunState) string {
	var builder strings.Builder
	childrenMap := make(map[string][]string)
	allSteps := make([]StepStatusUpdate, 0, len(state.Steps))

	for _, step := range state.Steps {
		allSteps = append(allSteps, step)
		if len(step.DependsOn) == 0 {
			childrenMap[""] = append(childrenMap[""], step.StepName)
		} else {
			for _, parent := range step.DependsOn {
				childrenMap[parent] = append(childrenMap[parent], step.StepName)
			}
		}
	}

	sort.SliceStable(allSteps, func(i, j int) bool {
		return allSteps[i].StepIndex < allSteps[j].StepIndex
	})

	var buildTree func(parent string, depth int)
	visited := make(map[string]bool)
	buildTree = func(parent string, depth int) {
		children, ok := childrenMap[parent]
		if !ok {
			return
		}

		// Sort children based on their original step index to maintain order
		sort.SliceStable(children, func(i, j int) bool {
			return state.Steps[children[i]].StepIndex < state.Steps[children[j]].StepIndex
		})

		for _, childName := range children {
			if visited[childName] {
				continue
			}
			visited[childName] = true
			step := state.Steps[childName]
			icon := "⏳"
			if step.StepStatus == "completed" {
				icon = "✅"
			} else if strings.Contains(strings.ToLower(step.StepStatus), "fail") {
				icon = "❌"
			}

			builder.WriteString(strings.Repeat("  ", depth))
			builder.WriteString(fmt.Sprintf("- %s `%s` - %s\n", icon, step.StepName, step.StepStatus))
			buildTree(childName, depth+1)
		}
	}

	// This ensures we start with all root nodes (those with no dependencies)
	rootNodes := childrenMap[""]
	sort.SliceStable(rootNodes, func(i, j int) bool {
		return state.Steps[rootNodes[i]].StepIndex < state.Steps[rootNodes[j]].StepIndex
	})
	for _, rootNode := range rootNodes {
		if !visited[rootNode] {
			step := state.Steps[rootNode]
			icon := "⏳"
			if step.StepStatus == "completed" {
				icon = "✅"
			} else if strings.Contains(strings.ToLower(step.StepStatus), "fail") {
				icon = "❌"
			}

			builder.WriteString(fmt.Sprintf("- %s `%s` - %s\n", icon, step.StepName, step.StepStatus))
			visited[rootNode] = true
			buildTree(rootNode, 1)
		}
	}
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
		Name:    "Nopsai CI",
		HeadSHA: ref,
		Status:  github.String("queued"),
	}
	checkRun, _, err := a.ghClient.Checks.CreateCheckRun(context.Background(), owner, repo, opts)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create check run")
		return 0
	}

	inProgressOpts := github.UpdateCheckRunOptions{
		Name:   "Nopsai CI",
		Status: github.String("in_progress"),
		Output: &github.CheckRunOutput{
			Title:   github.String(repo),
			Summary: github.String("Pipeline is starting..."),
		},
	}
	a.ghClient.Checks.UpdateCheckRun(context.Background(), owner, repo, *checkRun.ID, inProgressOpts)

	var pipeline models.Pipeline
	_ = yaml.Unmarshal([]byte(pipelineDef), &pipeline)
	view := "flat"
	if pipeline.DisplayOptions.GitHubView == "tree" {
		view = "tree"
	}

	a.stateLock.Lock()
	defer a.stateLock.Unlock()
	a.checkRunStates[*checkRun.ID] = &CheckRunState{
		Steps:      make(map[string]StepStatusUpdate),
		GitHubView: view,
	}

	return *checkRun.ID
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

	var repoName, commitSHA, owner, repo string
	var headCommit *github.HeadCommit
	var pusher *github.User

	switch event := payload.(type) {
	case *github.PushEvent:
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
		headCommit = event.HeadCommit
		pusher = event.Pusher
		log.Info().Str("repo", repoName).Str("commit", commitSHA).Msg("Processing push event")

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
			log.Info().Str("repo", repoName).Str("commit", commitSHA).Msg("Processing rerun request from check_run event")
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
			log.Info().Str("repo", repoName).Str("commit", commitSHA).Msg("Processing rerun request from check_suite event")
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
		log.Info().Str("repo", repoName).Msg("No override found, fetching .nopsai.yaml from repository.")
		fileContent, _, _, err := a.ghClient.Repositories.GetContents(context.Background(), owner, repo, ".nopsai.yaml", &github.RepositoryContentGetOptions{Ref: commitSHA})
		if err != nil || fileContent == nil {
			log.Error().Err(err).Msg("Failed to fetch .nopsai.yaml from repository")
			checkRunID := a.createCheckRun(owner, repo, commitSHA, "")
			a.concludeCheckRun(owner, repo, checkRunID, "failure", "Could not find .nopsai.yaml in the repository.")
			http.Error(w, "Could not fetch pipeline file", http.StatusNotFound)
			return
		}
		content, err := fileContent.GetContent()
		if err != nil {
			log.Error().Err(err).Msg("Failed to decode file content")
			checkRunID := a.createCheckRun(owner, repo, commitSHA, "")
			a.concludeCheckRun(owner, repo, checkRunID, "failure", "Could not decode the .nopsai.yaml file content.")
			http.Error(w, "Could not decode file content", http.StatusInternalServerError)
			return
		}
		pipelineYAML = []byte(content)
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
				req.Header.Set("X-Git-Commit-Message", *headCommit.Message)
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
	if err != nil || resp.StatusCode != http.StatusCreated {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		log.Error().Err(err).Int("status_code", statusCode).Msg("Failed to trigger nopsai pipeline")
		errorBody, _ := io.ReadAll(resp.Body)
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
	if state.GitHubView == "tree" {
		summary = a.renderMarkdownTree(state)
	} else {
		summary = a.renderMarkdownFlatList(state)
	}
	newTitle := fmt.Sprintf("In progress... (%d/%d steps)", len(state.Steps), update.TotalSteps)

	opts := github.UpdateCheckRunOptions{
		Name:   "Nopsai CI",
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
	summary := ""
	if ok {
		if state.GitHubView == "tree" {
			summary = a.renderMarkdownTree(state)
		} else {
			summary = a.renderMarkdownFlatList(state)
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

	finalTitle := "Nopsai CI - " + strings.ToUpper(string(conclusion[0])) + conclusion[1:]

	opts := github.UpdateCheckRunOptions{
		Name:        "Nopsai CI",
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

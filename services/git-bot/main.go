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
	"strconv"
	"strings"
	"time"

	"nopsai/config"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v53/github"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type GitBotApp struct {
	cfg           *config.Config
	ghClient      *github.Client
	webhookSecret string
}

// StepStatusUpdate is the structure nopsai uses to report per-step progress.
type StepStatusUpdate struct {
	RepoOwner  string `json:"repo_owner"`
	RepoName   string `json:"repo_name"`
	CheckRunID int64  `json:"check_run_id"`
	StepName   string `json:"step_name"`
	StepStatus string `json:"step_status"`
	StepIndex  int    `json:"step_index"`
	TotalSteps int    `json:"total_steps"`
}

// RunStatusUpdate is the structure nopsai uses to report the final status of a run.
type RunStatusUpdate struct {
	Status     string `json:"status"`
	CheckRunID int64  `json:"check_run_id"`
	RepoOwner  string `json:"repo_owner"`
	RepoName   string `json:"repo_name"`
}

// verifySignature checks the GitHub webhook signature to ensure the request is legitimate.
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

// createCheckRun creates a new check run on GitHub and immediately updates it to "in_progress".
func (a *GitBotApp) createCheckRun(owner, repo, ref, pipelineName string) int64 {
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
			Title:   github.String(pipelineName),
			Summary: github.String("Pipeline is starting..."),
		},
	}
	_, _, err = a.ghClient.Checks.UpdateCheckRun(context.Background(), owner, repo, *checkRun.ID, inProgressOpts)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update check run to in_progress")
		// Continue anyway, as the run is still created
	}
	return *checkRun.ID
}

// handleWebhook is the main entry point for incoming webhooks from GitHub.
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

	event, err := github.ParseWebHook(github.WebHookType(r), body)
	if err != nil {
		http.Error(w, "Could not parse webhook", http.StatusBadRequest)
		return
	}

	pushEvent, ok := event.(*github.PushEvent)
	if !ok || pushEvent.GetRef() == "" {
		log.Info().Msg("Received non-push event or push with no ref, ignoring.")
		w.WriteHeader(http.StatusOK)
		return
	}

	repoName := *pushEvent.Repo.FullName
	commitSHA := *pushEvent.After
	owner := *pushEvent.Repo.Owner.Login
	repo := *pushEvent.Repo.Name

	log.Info().Str("repo", repoName).Str("commit", commitSHA).Msg("Processing push event")
	checkRunID := a.createCheckRun(owner, repo, commitSHA, *pushEvent.Repo.Name)
	if checkRunID == 0 {
		http.Error(w, "Failed to create check run", http.StatusInternalServerError)
		return
	}

	var pipelineYAML []byte
	overrideURL := fmt.Sprintf("%s/v1/overrides/%s/%s", a.cfg.GitBotNopsaiAPIURL, owner, repo)
	resp, err := http.Get(overrideURL)
	if err == nil && resp.StatusCode == http.StatusOK {
		pipelineYAML, _ = io.ReadAll(resp.Body)
		log.Info().Str("repo", repoName).Msg("Found and using pipeline override from nopsai service.")
		resp.Body.Close()
	} else {
		log.Info().Str("repo", repoName).Msg("No override found, fetching .nopsai.yaml from repository.")
		fileContent, _, _, err := a.ghClient.Repositories.GetContents(context.Background(), owner, repo, ".nopsai.yaml", &github.RepositoryContentGetOptions{Ref: commitSHA})
		if err != nil {
			log.Error().Err(err).Msg("Failed to fetch .nopsai.yaml from repository")
			a.concludeCheckRun(owner, repo, checkRunID, "failure", "Could not find .nopsai.yaml in repository.")
			http.Error(w, "Could not fetch pipeline file", http.StatusNotFound)
			return
		}
		content, err := fileContent.GetContent()
		if err != nil {
			log.Error().Err(err).Msg("Failed to decode file content")
			http.Error(w, "Could not decode file content", http.StatusInternalServerError)
			return
		}
		pipelineYAML = []byte(content)
	}

	runURL := fmt.Sprintf("%s/v1/run", a.cfg.GitBotNopsaiAPIURL)
	req, _ := http.NewRequest("POST", runURL, bytes.NewBuffer(pipelineYAML))
	req.Header.Set("Content-Type", "application/x-yaml")
	req.Header.Set("X-Git-Repo-Owner", owner)
	req.Header.Set("X-Git-Repo-Name", repo)
	req.Header.Set("X-Git-Commit-SHA", commitSHA)
	req.Header.Set("X-Git-Check-Run-ID", strconv.FormatInt(checkRunID, 10))

	client := &http.Client{}
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		log.Error().Err(err).Int("status_code", statusCode).Msg("Failed to trigger nopsai pipeline")
		a.concludeCheckRun(owner, repo, checkRunID, "failure", "Failed to trigger Nopsai pipeline.")
		http.Error(w, "Failed to trigger pipeline", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("Pipeline triggered."))
}

// handleStepStatusUpdate is the internal endpoint for nopsai to report per-step progress.
func (a *GitBotApp) handleStepStatusUpdate(w http.ResponseWriter, r *http.Request) {
	var update StepStatusUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	log.Info().Int64("check_run_id", update.CheckRunID).Str("step", update.StepName).Msg("Received step status update")

	summary := fmt.Sprintf("Step %d/%d: '%s' %s.", update.StepIndex, update.TotalSteps, update.StepName, update.StepStatus)
	opts := github.UpdateCheckRunOptions{
		Name:   "Nopsai CI",
		Status: github.String("in_progress"),
		Output: &github.CheckRunOutput{
			Title:   github.String("Pipeline in progress..."),
			Summary: github.String(summary),
		},
	}
	_, _, err := a.ghClient.Checks.UpdateCheckRun(context.Background(), update.RepoOwner, update.RepoName, update.CheckRunID, opts)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update check run")
	}

	w.WriteHeader(http.StatusOK)
}

// handleRunStatusUpdate is the internal endpoint for nopsai to report the final pipeline status.
func (a *GitBotApp) handleRunStatusUpdate(w http.ResponseWriter, r *http.Request) {
	var update RunStatusUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Info().Int64("check_run_id", update.CheckRunID).Str("status", update.Status).Msg("Received final pipeline status")

	description := "Pipeline completed successfully."
	if update.Status == "failure" {
		description = "Pipeline failed."
	}

	a.concludeCheckRun(update.RepoOwner, update.RepoName, update.CheckRunID, update.Status, description)
	w.WriteHeader(http.StatusOK)
}

func (a *GitBotApp) concludeCheckRun(owner, repo string, checkRunID int64, conclusion, summary string) {
	opts := github.UpdateCheckRunOptions{
		Name:        "Nopsai CI",
		Status:      github.String("completed"),
		Conclusion:  github.String(conclusion),
		CompletedAt: &github.Timestamp{Time: time.Now()},
		Output: &github.CheckRunOutput{
			Title:   github.String("Pipeline Finished"),
			Summary: github.String(summary),
		},
	}
	_, _, err := a.ghClient.Checks.UpdateCheckRun(context.Background(), owner, repo, checkRunID, opts)
	if err != nil {
		log.Error().Err(err).Msg("Failed to conclude check run")
	}
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
		cfg:           cfg,
		ghClient:      ghClient,
		webhookSecret: cfg.GitHubWebhookSecret,
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

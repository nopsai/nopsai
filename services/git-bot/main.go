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
	"sync"
	"time"

	"nopsai/config"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v53/github"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type GitBotApp struct {
	cfg               *config.Config
	ghClient          *github.Client
	webhookSecret     string
	checkRunSummaries map[int64]string
	summaryLock       sync.Mutex
}

type StepStatusUpdate struct {
	RepoOwner  string `json:"repo_owner"`
	RepoName   string `json:"repo_name"`
	CheckRunID int64  `json:"check_run_id"`
	StepName   string `json:"step_name"`
	StepStatus string `json:"step_status"`
	StepIndex  int    `json:"step_index"`
	TotalSteps int    `json:"total_steps"`
}

type RunStatusUpdate struct {
	Status     string `json:"status"`
	CheckRunID int64  `json:"check_run_id"`
	RepoOwner  string `json:"repo_owner"`
	RepoName   string `json:"repo_name"`
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

func (a *GitBotApp) createCheckRun(owner, repo, ref string) int64 {
	a.summaryLock.Lock()
	defer a.summaryLock.Unlock()

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

	checkRunID := a.createCheckRun(owner, repo, commitSHA)
	delete(a.checkRunSummaries, checkRunID)
	if checkRunID == 0 {
		http.Error(w, "Failed to create check run", http.StatusInternalServerError)
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
			// Corrected call with a valid conclusion and a summary message
			a.concludeCheckRun(owner, repo, checkRunID, "failure", "Could not find .nopsai.yaml in the repository.")
			http.Error(w, "Could not fetch pipeline file", http.StatusNotFound)
			return
		}
		content, err := fileContent.GetContent()
		if err != nil {
			log.Error().Err(err).Msg("Failed to decode file content")
			// Corrected call
			a.concludeCheckRun(owner, repo, checkRunID, "failure", "Could not decode the .nopsai.yaml file content.")
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
		// Corrected call with a valid conclusion and a summary message
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

	a.summaryLock.Lock()
	defer a.summaryLock.Unlock()

	summaryLine := ""
	if strings.Contains(strings.ToLower(update.StepStatus), "fail") {
		summaryLine = fmt.Sprintf("❌ Step %d: '%s' - %s\n", update.StepIndex, update.StepName, update.StepStatus)
	} else {
		summaryLine = fmt.Sprintf("✅ Step %d: '%s' - %s\n", update.StepIndex, update.StepName, update.StepStatus)
	}
	a.checkRunSummaries[update.CheckRunID] += summaryLine

	newTitle := fmt.Sprintf("In progress... (%d/%d steps)", update.StepIndex, update.TotalSteps)

	opts := github.UpdateCheckRunOptions{
		Name:   "Nopsai CI",
		Status: github.String("in_progress"),
		Output: &github.CheckRunOutput{
			Title:   github.String(newTitle),
			Summary: github.String(a.checkRunSummaries[update.CheckRunID]),
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

	// Get the accumulated summary before concluding
	a.summaryLock.Lock()
	summary := a.checkRunSummaries[update.CheckRunID]
	a.summaryLock.Unlock()

	a.concludeCheckRun(update.RepoOwner, update.RepoName, update.CheckRunID, update.Status, summary)
	w.WriteHeader(http.StatusOK)
}

// Updated function signature to accept a separate summary
func (a *GitBotApp) concludeCheckRun(owner, repo string, checkRunID int64, conclusion, summary string) {
	if checkRunID == 0 {
		log.Warn().Msg("Invalid checkRunID (0), skipping conclusion.")
		return
	}

	a.summaryLock.Lock()
	defer a.summaryLock.Unlock()

	finalTitle := "Nopsai CI - " + strings.ToUpper(string(conclusion[0])) + conclusion[1:]

	// Use the provided summary. If it's empty, use the one from the map.
	finalSummary := summary
	if finalSummary == "" {
		finalSummary = a.checkRunSummaries[checkRunID]
	}

	opts := github.UpdateCheckRunOptions{
		Name:        "Nopsai CI",
		Status:      github.String("completed"),
		Conclusion:  github.String(conclusion),
		CompletedAt: &github.Timestamp{Time: time.Now()},
		Output: &github.CheckRunOutput{
			Title:   github.String(finalTitle),
			Summary: github.String(finalSummary),
		},
	}

	_, _, err := a.ghClient.Checks.UpdateCheckRun(context.Background(), owner, repo, checkRunID, opts)
	if err != nil {
		log.Error().Err(err).Msg("Failed to conclude check run")
	}

	delete(a.checkRunSummaries, checkRunID)
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
		cfg:               cfg,
		ghClient:          ghClient,
		webhookSecret:     cfg.GitHubWebhookSecret,
		checkRunSummaries: make(map[int64]string),
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

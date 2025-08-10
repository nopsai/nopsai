package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/models"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

type App struct {
	db  *pgxpool.Pool
	cfg *config.Config
}

type StepStatusUpdate struct {
	Status   string `json:"status"`
	ExitCode int    `json:"exit_code"`
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

func sanitizeContainerName(name string) string {
	sanitized := strings.ReplaceAll(name, " ", "-")
	return nonAlphanumericRegex.ReplaceAllString(sanitized, "")
}

func (a *App) handleRunPipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	var pipeline models.Pipeline
	if err := yaml.Unmarshal(body, &pipeline); err != nil {
		http.Error(w, "Error parsing YAML pipeline", http.StatusBadRequest)
		return
	}

	runID := uuid.New()
	log.Info().Str("run_id", runID.String()).Msgf("Received pipeline: %s", pipeline.Name)

	repoOwner := r.Header.Get("X-Git-Repo-Owner")
	repoName := r.Header.Get("X-Git-Repo-Name")
	commitSHA := r.Header.Get("X-Git-Commit-SHA")
	checkRunIDStr := r.Header.Get("X-Git-Check-Run-ID")
	checkRunID, _ := strconv.ParseInt(checkRunIDStr, 10, 64)

	timeoutStr := pipeline.Timeout
	if timeoutStr == "" {
		timeoutStr = a.cfg.DefaultPipelineTimeout
	}

	var timeoutAt sql.NullTime
	var timeoutDuration time.Duration
	if timeoutStr != "" {
		duration, err := time.ParseDuration(timeoutStr)
		if err != nil {
			log.Error().Err(err).Msgf("Invalid timeout duration format: %s", timeoutStr)
			http.Error(w, "Invalid timeout duration format", http.StatusBadRequest)
			return
		}
		timeoutAt.Time = time.Now().Add(duration)
		timeoutAt.Valid = true
		timeoutDuration = duration
	}

	tx, err := a.db.Begin(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Failed to start database transaction")
		http.Error(w, "Failed to start database transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(context.Background())

	_, err = tx.Exec(context.Background(),
		"INSERT INTO runs (run_id, pipeline_name, status, timeout_at, pipeline_definition, git_repo_owner, git_repo_name, git_commit_sha, git_check_run_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)",
		runID, pipeline.Name, "pending", timeoutAt, string(body), repoOwner, repoName, commitSHA, checkRunID,
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to insert run record")
		http.Error(w, "Failed to create run record", http.StatusInternalServerError)
		return
	}

	for i, step := range pipeline.Steps {
		_, err := tx.Exec(context.Background(),
			"INSERT INTO steps (step_id, run_id, name, status, step_index) VALUES (gen_random_uuid(), $1, $2, 'pending', $3)",
			runID, step.Name, i+1,
		)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to insert step %s", step.Name)
			http.Error(w, "Failed to create step records", http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(context.Background()); err != nil {
		log.Error().Err(err).Msg("Failed to commit transaction")
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	go a.launchAgent(runID.String(), pipeline, body, timeoutDuration, repoOwner, repoName, commitSHA, checkRunID)

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Pipeline run created successfully with ID: " + runID.String()))
}

func (a *App) handleStepUpdate(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	stepName := r.PathValue("stepName")

	var update StepStatusUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	_, err := a.db.Exec(context.Background(),
		"UPDATE steps SET status = $1, exit_code = $2, finished_at = NOW() WHERE run_id = $3 AND name = $4",
		update.Status, update.ExitCode, runID, stepName,
	)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Str("step", stepName).Msg("Failed to update step status")
		http.Error(w, "Failed to update step status", http.StatusInternalServerError)
		return
	}

	log.Info().Str("run_id", runID).Str("step", stepName).Str("status", update.Status).Msg("Updated step status")

	go a.notifyGitBotOfStepStatus(runID, stepName, update.Status)

	w.WriteHeader(http.StatusOK)
}

func (a *App) handleOverrideCheck(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)

	var pipelineDef string
	err := a.db.QueryRow(context.Background(), "SELECT pipeline_definition FROM pipeline_overrides WHERE repository_name = $1", fullName).Scan(&pipelineDef)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(pipelineDef))
}

func (a *App) launchAgent(runID string, pipeline models.Pipeline, pipelineDef []byte, timeout time.Duration, repoOwner, repoName, commitSHA string, checkRunID int64) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Error creating Docker client")
		return
	}
	defer cli.Close()

	imageName := "nopsai-agent:latest"
	imageFilters := filters.NewArgs(filters.Arg("reference", imageName))
	images, err := cli.ImageList(ctx, image.ListOptions{Filters: imageFilters})
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msgf("Failed to list images to check for %s", imageName)
		return
	}
	if len(images) == 0 {
		log.Info().Msgf("Image %s not found locally, pulling...", imageName)
		out, err := cli.ImagePull(ctx, imageName, image.PullOptions{})
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Msgf("Failed to pull image %s", imageName)
			return
		}
		io.ReadAll(out)
		out.Close()
	} else {
		log.Info().Msgf("Image %s found locally.", imageName)
	}

	sharedVolumeName := fmt.Sprintf("vol-%s", runID)
	_, err = cli.VolumeCreate(context.Background(), volume.CreateOptions{Name: sharedVolumeName})
	if err != nil {
		log.Error().Err(err).Msg("Failed to create shared volume")
		return
	}
	defer cli.VolumeRemove(context.Background(), sharedVolumeName, true)

	sanitizedPipelineName := sanitizeContainerName(pipeline.Name)
	agentContainerName := fmt.Sprintf("agent-%s-%s", sanitizedPipelineName, runID)

	envVars := []string{
		fmt.Sprintf("RUN_ID=%s", runID),
		fmt.Sprintf("PIPELINE_NAME=%s", pipeline.Name),
		fmt.Sprintf("PIPELINE_TIMEOUT=%s", timeout.String()),
		fmt.Sprintf("LLM_AGENT_ADDRESS=%s", a.cfg.AgentLlmAgentAddress),
		fmt.Sprintf("NOPSAI_API_URL=%s", a.cfg.AgentNopsaiAPIURL),
		fmt.Sprintf("LOG_LEVEL=%s", a.cfg.LogLevel),
		fmt.Sprintf("LOG_FORMAT=%s", a.cfg.LogFormat),
		fmt.Sprintf("PIPELINE_DEFINITION=%s", base64.StdEncoding.EncodeToString(pipelineDef)),
		fmt.Sprintf("SHARED_VOLUME_NAME=%s", sharedVolumeName),
		fmt.Sprintf("DOCKER_NETWORK_NAME=%s", a.cfg.DockerNetworkName),
	}

	hostConfig := &container.HostConfig{
		Binds: []string{
			"/var/run/docker.sock:/var/run/docker.sock",
			fmt.Sprintf("%s:/workspace", sharedVolumeName),
		},
		AutoRemove: a.cfg.AutoRemovalAgentContainer,
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: imageName,
		Env:   envVars,
	}, hostConfig, &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			a.cfg.DockerNetworkName: {},
		},
	}, nil, agentContainerName)

	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to create agent container")
		return
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		log.Error().Err(err).Str("container_id", resp.ID).Msg("Failed to start agent container")
		return
	}

	log.Info().Str("run_id", runID).Str("container_name", agentContainerName).Msg("Successfully started agent container")

	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Msg("Error waiting for agent container")
		}
	case status := <-statusCh:
		log.Info().Str("run_id", runID).Int64("status_code", status.StatusCode).Msg("Agent container finished.")
		finalStatus := "success"
		if status.StatusCode != 0 {
			finalStatus = "failure"
		}
		a.db.Exec(context.Background(), "UPDATE runs SET status = $1, finished_at = NOW() WHERE run_id = $2", finalStatus, runID)
		if repoOwner != "" && repoName != "" && commitSHA != "" {
			a.notifyGitBotOfFinalStatus(finalStatus, checkRunID, repoOwner, repoName, commitSHA)
		}
	}
}

func (a *App) notifyGitBotOfFinalStatus(status string, checkRunID int64, repoOwner, repoName, commitSHA string) {
	gitBotURL := fmt.Sprintf("%s/v1/run/status", a.cfg.NopsaiGitBotAPIURL)
	payload := map[string]interface{}{
		"status":       status,
		"check_run_id": checkRunID,
		"repo_owner":   repoOwner,
		"repo_name":    repoName,
		"commit_sha":   commitSHA,
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(gitBotURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Error().Err(err).Msg("Failed to notify git-bot of final status")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status_code", resp.StatusCode).Msg("Received non-OK status from git-bot")
	} else {
		log.Info().Msg("Successfully notified git-bot of final pipeline status.")
	}
}

func (a *App) notifyGitBotOfStepStatus(runID, stepName, stepStatus string) {
	var repoOwner, repoName, commitSHA sql.NullString
	var checkRunID sql.NullInt64
	var stepIndex, totalSteps int

	query := `
		SELECT
			r.git_repo_owner, r.git_repo_name, r.git_commit_sha, r.git_check_run_id,
			s.step_index, (SELECT COUNT(*) FROM steps WHERE run_id = r.run_id)
		FROM runs r JOIN steps s ON r.run_id = s.run_id
		WHERE r.run_id = $1 AND s.name = $2`

	err := a.db.QueryRow(context.Background(), query, runID, stepName).Scan(&repoOwner, &repoName, &commitSHA, &checkRunID, &stepIndex, &totalSteps)
	if err != nil || !repoOwner.Valid || !checkRunID.Valid {
		log.Warn().Str("run_id", runID).Msg("Not a Git-triggered run with a check ID, skipping step status update.")
		return
	}

	gitBotURL := fmt.Sprintf("%s/v1/step/status", a.cfg.NopsaiGitBotAPIURL)
	payload := map[string]interface{}{
		"repo_owner":   repoOwner.String,
		"repo_name":    repoName.String,
		"check_run_id": checkRunID.Int64,
		"commit_sha":   commitSHA.String,
		"step_name":    stepName,
		"step_status":  stepStatus,
		"step_index":   stepIndex,
		"total_steps":  totalSteps,
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(gitBotURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Error().Err(err).Msg("Failed to notify git-bot of step status")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status_code", resp.StatusCode).Msg("Received non-OK status from git-bot for step update")
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

	var dbpool *pgxpool.Pool
	for i := 0; i < 5; i++ {
		dbpool, err = pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err == nil {
			if err = dbpool.Ping(context.Background()); err == nil {
				log.Info().Msg("Successfully connected to the database.")
				break
			}
		}
		log.Warn().Err(err).Msgf("Unable to connect to database. Retrying in 3 seconds...")
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database after multiple retries")
	}
	defer dbpool.Close()

	app := &App{db: dbpool, cfg: cfg}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/run", app.handleRunPipeline)
	mux.HandleFunc("POST /v1/runs/{runID}/steps/{stepName}", app.handleStepUpdate)
	mux.HandleFunc("GET /v1/overrides/{repoOwner}/{repoName}", app.handleOverrideCheck)

	log.Info().Msgf("Nopsai API server listening on %s", cfg.NopsaiListenAddress)
	if err := http.ListenAndServe(cfg.NopsaiListenAddress, mux); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}

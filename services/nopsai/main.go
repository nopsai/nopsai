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
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
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

// App now holds the shared Docker client
type App struct {
	db  *pgxpool.Pool
	cfg *config.Config
	cli *client.Client
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

	if pipeline.Name == "" {
		http.Error(w, "Pipeline validation failed: 'name' is a required field.", http.StatusBadRequest)
		return
	}
	if pipeline.ContainerImage == "" {
		http.Error(w, "Pipeline validation failed: 'container_image' is a required field.", http.StatusBadRequest)
		return
	}
	if len(pipeline.Steps) == 0 {
		http.Error(w, "Pipeline validation failed: at least one step is required.", http.StatusBadRequest)
		return
	}
	for i, step := range pipeline.Steps {
		if step.Name == "" {
			http.Error(w, fmt.Sprintf("Pipeline validation failed: 'name' is a required field for step %d.", i+1), http.StatusBadRequest)
			return
		}
		if step.Goal == "" {
			http.Error(w, fmt.Sprintf("Pipeline validation failed: 'goal' is a required field for step '%s'.", step.Name), http.StatusBadRequest)
			return
		}
	}

	runID := uuid.New()
	log.Info().Str("run_id", runID.String()).Msgf("Received pipeline: %s", pipeline.Name)

	gitContext := map[string]string{
		"repo_owner":             r.Header.Get("X-Git-Repo-Owner"),
		"repo_name":              r.Header.Get("X-Git-Repo-Name"),
		"clone_url":              r.Header.Get("X-Git-Clone-URL"),
		"ssh_url":                r.Header.Get("X-Git-SSH-URL"),
		"ref":                    r.Header.Get("X-Git-Ref"),
		"commit_sha":             r.Header.Get("X-Git-Commit-SHA"),
		"commit_url":             r.Header.Get("X-Git-Commit-URL"),
		"commit_message":         r.Header.Get("X-Git-Commit-Message"),
		"commit_author_name":     r.Header.Get("X-Git-Commit-Author-Name"),
		"commit_author_email":    r.Header.Get("X-Git-Commit-Author-Email"),
		"commit_author_username": r.Header.Get("X-Git-Commit-Author-Username"),
		"pusher_name":            r.Header.Get("X-Git-Pusher-Name"),
		"pusher_email":           r.Header.Get("X-Git-Pusher-Email"),
		"check_run_id":           r.Header.Get("X-Git-Check-Run-ID"),
	}
	checkRunID, _ := strconv.ParseInt(gitContext["check_run_id"], 10, 64)

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

	// This is the corrected INSERT statement.
	_, err = tx.Exec(context.Background(),
		`INSERT INTO runs (run_id, pipeline_name, status, timeout_at, pipeline_definition, 
			git_repo_owner, git_repo_name, git_clone_url, git_ssh_url, git_ref, 
			git_commit_sha, git_commit_url, git_commit_message, git_commit_author_name, 
			git_commit_author_email, git_commit_author_username, git_pusher_name, 
			git_pusher_email, git_check_run_id) 
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
		runID, pipeline.Name, "pending", timeoutAt, string(body),
		gitContext["repo_owner"], gitContext["repo_name"], gitContext["clone_url"], gitContext["ssh_url"], gitContext["ref"],
		gitContext["commit_sha"], gitContext["commit_url"], gitContext["commit_message"], gitContext["commit_author_name"],
		gitContext["commit_author_email"], gitContext["commit_author_username"], gitContext["pusher_name"],
		gitContext["pusher_email"], checkRunID,
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

	// This is the corrected call, passing the gitContext map.
	go a.launchAgent(runID.String(), pipeline, body, timeoutDuration, gitContext)

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Pipeline run created successfully with ID: " + runID.String()))
}

func (a *App) handleRerunPipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	originalRunID := r.PathValue("runID")

	// 1. Fetch the original run's data from the database.
	var pipelineDef, pipelineName sql.NullString
	var gitContext = make(map[string]string)
	var timeoutAt sql.NullTime

	query := `SELECT 
				pipeline_definition, pipeline_name, timeout_at,
				git_repo_owner, git_repo_name, git_clone_url, git_ssh_url, git_ref, 
				git_commit_sha, git_commit_url, git_commit_message, git_commit_author_name, 
				git_commit_author_email, git_commit_author_username, git_pusher_name, 
				git_pusher_email, git_check_run_id
			  FROM runs WHERE run_id = $1`

	var repoOwner, repoName, cloneURL, sshURL, ref, commitSHA, commitURL, commitMessage,
		commitAuthorName, commitAuthorEmail, commitAuthorUsername, pusherName, pusherEmail sql.NullString
	var checkRunID sql.NullInt64

	err := a.db.QueryRow(context.Background(), query, originalRunID).Scan(
		&pipelineDef, &pipelineName, &timeoutAt,
		&repoOwner, &repoName, &cloneURL, &sshURL, &ref, &commitSHA, &commitURL, &commitMessage,
		&commitAuthorName, &commitAuthorEmail, &commitAuthorUsername, &pusherName, &pusherEmail, &checkRunID,
	)

	if err != nil {
		log.Error().Err(err).Str("original_run_id", originalRunID).Msg("Failed to find original run to rerun")
		http.Error(w, "Original pipeline run not found", http.StatusNotFound)
		return
	}

	if !pipelineDef.Valid {
		http.Error(w, "Original pipeline definition is missing, cannot rerun", http.StatusInternalServerError)
		return
	}

	// 2. Assemble the gitContext map, matching the launchAgent signature.
	if repoOwner.Valid {
		gitContext["repo_owner"] = repoOwner.String
	}
	if repoName.Valid {
		gitContext["repo_name"] = repoName.String
	}
	if cloneURL.Valid {
		gitContext["clone_url"] = cloneURL.String
	}
	if sshURL.Valid {
		gitContext["ssh_url"] = sshURL.String
	}
	if ref.Valid {
		gitContext["ref"] = ref.String
	}
	if commitSHA.Valid {
		gitContext["commit_sha"] = commitSHA.String
	}
	if commitURL.Valid {
		gitContext["commit_url"] = commitURL.String
	}
	if commitMessage.Valid {
		gitContext["commit_message"] = commitMessage.String
	}
	if commitAuthorName.Valid {
		gitContext["commit_author_name"] = commitAuthorName.String
	}
	if commitAuthorEmail.Valid {
		gitContext["commit_author_email"] = commitAuthorEmail.String
	}
	if commitAuthorUsername.Valid {
		gitContext["commit_author_username"] = commitAuthorUsername.String
	}
	if pusherName.Valid {
		gitContext["pusher_name"] = pusherName.String
	}
	if pusherEmail.Valid {
		gitContext["pusher_email"] = pusherEmail.String
	}
	if checkRunID.Valid {
		gitContext["check_run_id"] = strconv.FormatInt(checkRunID.Int64, 10)
	}

	// 3. Create a new run using the old run's data.
	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(pipelineDef.String), &pipeline); err != nil {
		http.Error(w, "Could not parse original pipeline definition", http.StatusInternalServerError)
		return
	}

	newRunID := uuid.New()
	log.Info().Str("new_run_id", newRunID.String()).Str("original_run_id", originalRunID).Msg("Rerunning pipeline")

	var timeoutDuration time.Duration
	if timeoutAt.Valid {
		var originalCreatedAt time.Time
		// This is the corrected block
		err := a.db.QueryRow(context.Background(), "SELECT created_at FROM runs WHERE run_id = $1", originalRunID).Scan(&originalCreatedAt)
		if err != nil {
			log.Error().Err(err).Str("original_run_id", originalRunID).Msg("Failed to get original run creation time for timeout calculation")
			http.Error(w, "Could not calculate rerun timeout", http.StatusInternalServerError)
			return
		}
		originalDuration := timeoutAt.Time.Sub(originalCreatedAt)
		timeoutAt.Time = time.Now().Add(originalDuration)
		timeoutDuration = originalDuration
	}

	// 4. Insert the new run and steps into the database.
	tx, err := a.db.Begin(context.Background())
	if err != nil {
		http.Error(w, "Failed to start database transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(context.Background())

	_, err = tx.Exec(context.Background(),
		`INSERT INTO runs (run_id, pipeline_name, status, timeout_at, pipeline_definition, 
			git_repo_owner, git_repo_name, git_clone_url, git_ssh_url, git_ref, 
			git_commit_sha, git_commit_url, git_commit_message, git_commit_author_name, 
			git_commit_author_email, git_commit_author_username, git_pusher_name, 
			git_pusher_email, git_check_run_id) 
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
		newRunID, pipelineName, "pending", timeoutAt, pipelineDef,
		repoOwner, repoName, cloneURL, sshURL, ref, commitSHA, commitURL, commitMessage,
		commitAuthorName, commitAuthorEmail, commitAuthorUsername, pusherName, pusherEmail, checkRunID,
	)
	if err != nil {
		http.Error(w, "Failed to create new run record for rerun", http.StatusInternalServerError)
		return
	}

	for i, step := range pipeline.Steps {
		_, err := tx.Exec(context.Background(),
			"INSERT INTO steps (step_id, run_id, name, status, step_index) VALUES (gen_random_uuid(), $1, $2, 'pending', $3)",
			newRunID, step.Name, i+1,
		)
		if err != nil {
			http.Error(w, "Failed to create step records for rerun", http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(context.Background()); err != nil {
		http.Error(w, "Failed to commit transaction for rerun", http.StatusInternalServerError)
		return
	}

	// 5. Launch a new agent for the new run with the correct signature.
	go a.launchAgent(newRunID.String(), pipeline, []byte(pipelineDef.String), timeoutDuration, gitContext)

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("Pipeline rerun initiated with new ID: " + newRunID.String()))
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

	w.Header().Set("Content-Type", "application/x-yaml") // Add this line
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(pipelineDef))
}

// launchAgent now uses the shared Docker client from a.cli
func (a *App) launchAgent(runID string, pipeline models.Pipeline, pipelineDef []byte, timeout time.Duration, gitContext map[string]string) {
	ctx := context.Background()

	agentImageName := a.cfg.AgentImage
	if agentImageName == "" {
		agentImageName = "nopsai-agent:latest"
	}

	if err := ensureImageExists(ctx, a.cli, agentImageName); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to ensure agent image exists")
		return
	}

	sharedVolumeName := fmt.Sprintf("vol-%s", runID)
	_, err := a.cli.VolumeCreate(context.Background(), volume.CreateOptions{Name: sharedVolumeName})
	if err != nil {
		log.Error().Err(err).Msg("Failed to create shared volume")
		return
	}
	defer a.cli.VolumeRemove(context.Background(), sharedVolumeName, true)

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
	for key, value := range gitContext {
		envKey := fmt.Sprintf("GIT_%s", strings.ToUpper(key))
		envVars = append(envVars, fmt.Sprintf("%s=%s", envKey, value))
	}

	hostConfig := &container.HostConfig{
		Binds: []string{
			"/var/run/docker.sock:/var/run/docker.sock",
			fmt.Sprintf("%s:/workspace", sharedVolumeName),
		},
		AutoRemove: a.cfg.AutoRemovalAgentContainer,
	}

	resp, err := a.cli.ContainerCreate(ctx, &container.Config{
		Image: agentImageName,
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

	if err := a.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		log.Error().Err(err).Str("container_id", resp.ID).Msg("Failed to start agent container")
		return
	}

	log.Info().Str("run_id", runID).Str("container_name", agentContainerName).Msg("Successfully started agent container")

	statusCh, errCh := a.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
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
		if gitContext["repo_owner"] != "" {
			a.notifyGitBotOfFinalStatus(finalStatus, gitContext)
		}
	}
}

func (a *App) notifyGitBotOfFinalStatus(status string, gitContext map[string]string) {
	checkRunID, _ := strconv.ParseInt(gitContext["check_run_id"], 10, 64)
	gitBotURL := fmt.Sprintf("%s/v1/run/status", a.cfg.NopsaiGitBotAPIURL)
	payload := map[string]interface{}{
		"status":       status,
		"check_run_id": checkRunID,
		"repo_owner":   gitContext["repo_owner"],
		"repo_name":    gitContext["repo_name"],
		"commit_sha":   gitContext["commit_sha"],
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

func ensureImageExists(ctx context.Context, cli *client.Client, imageName string) error {
	imageFilters := filters.NewArgs(filters.Arg("reference", imageName))
	images, err := cli.ImageList(ctx, image.ListOptions{Filters: imageFilters})
	if err != nil {
		return fmt.Errorf("failed to list images to check for %s: %w", imageName, err)
	}

	if len(images) == 0 {
		log.Info().Msgf("Image %s not found locally, pulling...", imageName)
		out, err := cli.ImagePull(ctx, imageName, image.PullOptions{})
		if err != nil {
			return fmt.Errorf("failed to pull image %s: %w", imageName, err)
		}
		defer out.Close()
		// Read the output to ensure the pull is complete
		io.Copy(io.Discard, out)
	} else {
		log.Info().Msgf("Image %s found locally.", imageName)
	}
	return nil
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

	// Create the Docker client ONCE here
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create Docker client")
	}
	defer cli.Close()

	// Pass the shared client to the App instance
	app := &App{db: dbpool, cfg: cfg, cli: cli}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/run", app.handleRunPipeline)
	mux.HandleFunc("POST /v1/runs/{runID}/steps/{stepName}", app.handleStepUpdate)
	mux.HandleFunc("GET /v1/overrides/{repoOwner}/{repoName}", app.handleOverrideCheck)
	mux.HandleFunc("POST /v1/runs/{runID}/rerun", app.handleRerunPipeline)

	server := &http.Server{
		Addr:    cfg.NopsaiListenAddress,
		Handler: mux,
	}

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Info().Msgf("Nopsai API server listening on %s", cfg.NopsaiListenAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	<-stop

	log.Info().Msg("Shutting down the server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	app.db.Close()
	log.Info().Msg("Server exiting")
}

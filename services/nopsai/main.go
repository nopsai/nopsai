package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
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

// StepStatusUpdate is the structure agents use to report the status of a step.
type StepStatusUpdate struct {
	Status   string `json:"status"`
	ExitCode int    `json:"exit_code"`
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

// sanitizeContainerName ensures the pipeline name is a valid string for a container name.
func sanitizeContainerName(name string) string {
	sanitized := strings.ReplaceAll(name, " ", "-")
	return nonAlphanumericRegex.ReplaceAllString(sanitized, "")
}

// handleRunPipeline is the main public endpoint. It receives a pipeline YAML,
// creates the initial database records, and launches an agent container to run it.
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

	// Determine the timeout duration, prioritizing the pipeline-specific setting.
	timeoutStr := pipeline.Timeout
	if timeoutStr == "" {
		timeoutStr = a.cfg.DefaultPipelineTimeout
	}

	var timeoutAt sql.NullTime
	if timeoutStr != "" {
		duration, err := time.ParseDuration(timeoutStr)
		if err != nil {
			log.Error().Err(err).Msgf("Invalid timeout duration format: %s", timeoutStr)
			http.Error(w, "Invalid timeout duration format", http.StatusBadRequest)
			return
		}
		timeoutAt.Time = time.Now().Add(duration)
		timeoutAt.Valid = true
	}

	tx, err := a.db.Begin(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Failed to start database transaction")
		http.Error(w, "Failed to start database transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(context.Background())

	_, err = tx.Exec(context.Background(),
		"INSERT INTO runs (run_id, pipeline_name, status, timeout_at, pipeline_definition) VALUES ($1, $2, $3, $4, $5)",
		runID, pipeline.Name, "pending", timeoutAt, string(body),
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to insert run record")
		http.Error(w, "Failed to create run record", http.StatusInternalServerError)
		return
	}

	for _, step := range pipeline.Steps {
		_, err := tx.Exec(context.Background(),
			"INSERT INTO steps (step_id, run_id, name, status) VALUES (gen_random_uuid(), $1, $2, 'pending')",
			runID, step.Name,
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

	// Launch the agent in a goroutine so the HTTP request can return immediately.
	go a.launchAgent(runID.String(), pipeline, body, timeoutStr)

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Pipeline run created successfully with ID: " + runID.String()))
}

// handleStepUpdate is an internal endpoint for agents to report back their status.
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
	w.WriteHeader(http.StatusOK)
}

// launchAgent handles the interaction with the Docker API to start a new agent container.
func (a *App) launchAgent(runID string, pipeline models.Pipeline, pipelineDef []byte, timeout string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Error creating Docker client")
		return
	}
	defer cli.Close()

	// Ensure the agent image exists locally before trying to run it.
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

	// Create a dedicated volume for this run.
	sharedVolumeName := fmt.Sprintf("vol-%s", runID)
	_, err = cli.VolumeCreate(context.Background(), volume.CreateOptions{Name: sharedVolumeName})
	if err != nil {
		log.Error().Err(err).Msg("Failed to create shared volume")
		return
	}
	defer cli.VolumeRemove(context.Background(), sharedVolumeName, true)

	sanitizedPipelineName := sanitizeContainerName(pipeline.Name)
	agentContainerName := fmt.Sprintf("agent-%s-%s", sanitizedPipelineName, runID)

	// Inject all necessary configuration for the agent to operate independently.
	envVars := []string{
		fmt.Sprintf("RUN_ID=%s", runID),
		fmt.Sprintf("PIPELINE_NAME=%s", pipeline.Name),
		fmt.Sprintf("PIPELINE_TIMEOUT=%s", timeout),
		fmt.Sprintf("LLM_AGENT_ADDRESS=%s", a.cfg.AgentLlmAgentAddress),
		fmt.Sprintf("NOPSAI_API_URL=%s", a.cfg.AgentNopsaiAPIURL),
		fmt.Sprintf("LOG_LEVEL=%s", a.cfg.LogLevel),
		fmt.Sprintf("LOG_FORMAT=%s", a.cfg.LogFormat),
		fmt.Sprintf("PIPELINE_DEFINITION=%s", base64.StdEncoding.EncodeToString(pipelineDef)),
		fmt.Sprintf("SHARED_VOLUME_NAME=%s", sharedVolumeName),
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

	// Wait for the agent container to finish its execution.
	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Msg("Error waiting for agent container")
		}
	case status := <-statusCh:
		log.Info().Str("run_id", runID).Int64("status_code", status.StatusCode).Msg("Agent container finished.")
		finalStatus := "succeeded"
		if status.StatusCode != 0 {
			finalStatus = "failed"
		}
		a.db.Exec(context.Background(), "UPDATE runs SET status = $1, finished_at = NOW() WHERE run_id = $2", finalStatus, runID)
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

	log.Info().Msgf("Nopsai API server listening on %s", cfg.NopsaiListenAddress)
	if err := http.ListenAndServe(cfg.NopsaiListenAddress, mux); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}

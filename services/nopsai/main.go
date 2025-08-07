package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"nopsai/config"
	"nopsai/pkg/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

type App struct {
	db              *pgxpool.Pool
	executorAddress string
}

// This request now only contains information for the whole run.
type ExecutionRequest struct {
	RunID            string            `json:"run_id"`
	PipelineName     string            `json:"pipeline_name"`
	ContainerImage   string            `json:"container_image"`
	WorkingDirectory string            `json:"working_directory"`
	Environment      map[string]string `json:"environment"`
}

func (a *App) handleRunPipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Error().Msgf("Invalid request method: %s", r.Method)
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("Error reading request body")
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	var pipeline models.Pipeline
	if err := yaml.Unmarshal(body, &pipeline); err != nil {
		log.Error().Err(err).Msg("Error parsing YAML pipeline")
		http.Error(w, "Error parsing YAML pipeline", http.StatusBadRequest)
		return
	}

	log.Info().Msgf("Received pipeline: %s", pipeline.Name)
	runID := uuid.New()

	tx, err := a.db.Begin(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Failed to start database transaction")
		http.Error(w, "Failed to start database transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(context.Background())

	envBytes, err := json.Marshal(pipeline.Environment)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal environment variables")
		http.Error(w, "Failed to marshal environment variables", http.StatusInternalServerError)
		return
	}
	_, err = tx.Exec(context.Background(),
		"INSERT INTO runs (run_id, pipeline_name, pipeline_definition, status, environment) VALUES ($1, $2, $3, $4, $5)",
		runID, pipeline.Name, string(body), "running", string(envBytes),
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to insert run record")
		http.Error(w, "Failed to create run record", http.StatusInternalServerError)
		return
	}

	stepNameToID := make(map[string]uuid.UUID)
	for i, step := range pipeline.Steps {
		stepID := uuid.New()
		stepNameToID[step.Name] = stepID
		_, err := tx.Exec(context.Background(),
			"INSERT INTO steps (step_id, run_id, step_index, name, goal, status, ignore_failure) VALUES ($1, $2, $3, $4, $5, $6, $7)",
			stepID, runID, i, step.Name, step.Goal, "pending", step.IgnoreFailure,
		)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to insert step %d", i)
			http.Error(w, "Failed to create step records", http.StatusInternalServerError)
			return
		}
	}

	for _, step := range pipeline.Steps {
		stepID := stepNameToID[step.Name]
		for _, depName := range step.DependsOn {
			depID, ok := stepNameToID[depName]
			if !ok {
				log.Fatal().Msgf("Error: dependency '%s' for step '%s' not found", depName, step.Name)
				return
			}
			_, err := tx.Exec(context.Background(),
				"INSERT INTO step_dependencies (step_id, depends_on_step_id) VALUES ($1, $2)",
				stepID, depID,
			)
			if err != nil {
				log.Fatal().Err(err).Msgf("Failed to insert dependency for step %s", step.Name)
				return
			}
		}
	}

	if err := tx.Commit(context.Background()); err != nil {
		log.Error().Err(err).Msg("Failed to commit transaction")
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	log.Info().Str("run_id", runID.String()).Msg("Successfully created run. Requesting container from executor...")
	a.callExecutor(runID.String(), pipeline)

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Pipeline run created successfully with ID: " + runID.String()))
	log.Info().Str("run_id", runID.String()).Msg("Pipeline run creation response sent.")
}

func (a *App) callExecutor(runID string, pipeline models.Pipeline) {
	execReq := ExecutionRequest{
		RunID:            runID,
		PipelineName:     pipeline.Name,
		ContainerImage:   pipeline.ContainerImage,
		WorkingDirectory: pipeline.WorkingDirectory,
		Environment:      pipeline.Environment,
	}
	reqBytes, err := json.Marshal(execReq)
	if err != nil {
		log.Fatal().Err(err).Msgf("Executor call failed for run %s: could not marshal request", runID)
		return
	}

	resp, err := http.Post(a.executorAddress+"/execute", "application/json", bytes.NewBuffer(reqBytes))
	if err != nil {
		log.Fatal().Err(err).Msgf("Executor call failed for run %s", runID)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Fatal().Msgf("Executor service returned error for run %s: %s", runID, string(body))
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

	app := &App{
		db:              dbpool,
		executorAddress: cfg.ExecutorAddress,
	}

	http.HandleFunc("/v1/run", app.handleRunPipeline)
	log.Info().Msgf("Nopsai orchestrator listening on %s", cfg.NopsaiListenAddress)
	if err := http.ListenAndServe(cfg.NopsaiListenAddress, nil); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}

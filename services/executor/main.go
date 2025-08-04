package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"nopsai/config"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type ExecutorApp struct {
	cfg *config.Config
}

type ExecutionRequest struct {
	RunID            string `json:"run_id"`
	ContainerImage   string `json:"container_image"`
	WorkingDirectory string `json:"working_directory"`
}

func (app *ExecutorApp) handleExecute(w http.ResponseWriter, r *http.Request) {
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

	var req ExecutionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log.Error().Err(err).Msg("Error parsing request JSON")
		http.Error(w, "Error parsing request JSON", http.StatusBadRequest)
		return
	}

	log.Info().Str("run_id", req.RunID).Msgf("Received execution request with image: %s", req.ContainerImage)

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Error().Err(err).Msg("Error creating Docker client")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer cli.Close()

	llmAgentAddress := app.cfg.AgentLlmAgentAddress
	networkName := app.cfg.DockerNetworkName

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:      req.ContainerImage,
		WorkingDir: req.WorkingDirectory,
		Env: []string{
			fmt.Sprintf("RUN_ID=%s", req.RunID),
			fmt.Sprintf("LLM_AGENT_ADDRESS=%s", llmAgentAddress),
		},
		Tty: false,
	}, &container.HostConfig{}, &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			networkName: {},
		},
	}, nil, "")
	if err != nil {
		log.Error().Err(err).Str("run_id", req.RunID).Msg("Failed to create container")
		http.Error(w, "Failed to create container", http.StatusInternalServerError)
		return
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		log.Error().Err(err).Str("container_id", resp.ID).Msg("Failed to start container")
		http.Error(w, "Failed to start container", http.StatusInternalServerError)
		return
	}

	log.Info().Str("run_id", req.RunID).Str("container_id", resp.ID[:12]).Msg("Successfully started container")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Container started successfully: %s", resp.ID)
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen})

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
	zerolog.SetGlobalLevel(logLevel)

	app := &ExecutorApp{cfg: cfg}

	http.HandleFunc("/execute", app.handleExecute)
	log.Info().Msgf("Executor service listening on %s", cfg.ExecutorListenAddress)
	if err := http.ListenAndServe(cfg.ExecutorListenAddress, nil); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}

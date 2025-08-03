package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"nopsai/config"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
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
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	var req ExecutionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Error parsing request JSON", http.StatusBadRequest)
		return
	}

	log.Printf("Received execution request for Run ID: %s, with image: %s", req.RunID, req.ContainerImage)

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Printf("Error creating Docker client: %v", err)
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
		log.Printf("Error creating container: %v", err)
		http.Error(w, "Failed to create container", http.StatusInternalServerError)
		return
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		log.Printf("Error starting container: %v", err)
		http.Error(w, "Failed to start container", http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully started container %s for Run ID %s", resp.ID[:12], req.RunID)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Container started successfully: %s", resp.ID)
}

func main() {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yml"
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config from %s: %v", configPath, err)
	}

	app := &ExecutorApp{cfg: cfg}

	http.HandleFunc("/execute", app.handleExecute)
	log.Printf("Executor service listening on %s", cfg.ExecutorListenAddress)
	if err := http.ListenAndServe(cfg.ExecutorListenAddress, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

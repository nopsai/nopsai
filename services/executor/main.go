package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

type ExecutionRequest struct {
	RunID          string `json:"run_id"`
	ContainerImage string `json:"container_image"`
}

func handleExecute(w http.ResponseWriter, r *http.Request) {
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

	log.Printf("Received execution request for Run ID: %s with image: %s", req.RunID, req.ContainerImage)

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Printf("Error creating Docker client: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer cli.Close()

	llmAgentAddress := "nopsai-llm-agent:50051"
	// Docker Compose v2 uses projectname-networkname as the default network name.
	// Assuming the project directory is named 'nopsai' or similar.
	// You may need to adjust this if your project directory name is different.
	networkName := "nopsai-net"

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: req.ContainerImage,
		Env: []string{
			fmt.Sprintf("RUN_ID=%s", req.RunID),
			fmt.Sprintf("LLM_AGENT_ADDRESS=%s", llmAgentAddress),
		},
		Tty: false,
	}, &container.HostConfig{}, &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			// Explicitly connect the new container to our shared network.
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
	http.HandleFunc("/execute", handleExecute)
	log.Println("Executor service listening on :8081")
	if err := http.ListenAndServe(":8081", nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"nopsai/config"
	"nopsai/pkg/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"
)

type App struct {
	db              *pgxpool.Pool
	executorAddress string
}

type ExecutionRequest struct {
	RunID          string `json:"run_id"`
	ContainerImage string `json:"container_image"`
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

	log.Printf("Received pipeline: %s", pipeline.Name)

	runID := uuid.New()

	tx, err := a.db.Begin(context.Background())
	if err != nil {
		log.Printf("Failed to start database transaction: %v", err)
		http.Error(w, "Failed to start database transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(context.Background())

	_, err = tx.Exec(context.Background(),
		"INSERT INTO runs (run_id, pipeline_definition, status) VALUES ($1, $2, $3)",
		runID, string(body), "pending",
	)
	if err != nil {
		http.Error(w, "Failed to create run record", http.StatusInternalServerError)
		return
	}

	for i, step := range pipeline.Steps {
		_, err := tx.Exec(context.Background(),
			"INSERT INTO steps (step_id, run_id, step_index, name, goal, status) VALUES ($1, $2, $3, $4, $5, $6)",
			uuid.New(), runID, i, step.Name, step.Goal, "pending",
		)
		if err != nil {
			log.Printf("Failed to insert step %d: %v", i, err)
			http.Error(w, "Failed to create step records", http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(context.Background()); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully created run with ID: %s. Forwarding to executor...", runID.String())

	execReq := ExecutionRequest{
		RunID:          runID.String(),
		ContainerImage: pipeline.ContainerImage,
	}
	reqBytes, err := json.Marshal(execReq)
	if err != nil {
		http.Error(w, "Failed to create executor request", http.StatusInternalServerError)
		return
	}

	resp, err := http.Post(a.executorAddress+"/execute", "application/json", bytes.NewBuffer(reqBytes))
	if err != nil {
		log.Printf("Failed to call executor service: %v", err)
		http.Error(w, "Failed to call executor service", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Executor service returned error: %s", string(body))
		http.Error(w, "Executor service failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "Pipeline run created and execution started with ID: %s\n", runID.String())
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

	var dbpool *pgxpool.Pool
	for i := 0; i < 5; i++ {
		dbpool, err = pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err == nil {
			if err = dbpool.Ping(context.Background()); err == nil {
				log.Println("Successfully connected to the database.")
				break
			}
		}
		log.Printf("Unable to connect to database: %v. Retrying in 3 seconds...", err)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatalf("Failed to connect to database after multiple retries: %v", err)
	}
	defer dbpool.Close()

	app := &App{
		db:              dbpool,
		executorAddress: cfg.ExecutorAddress,
	}

	http.HandleFunc("/v1/run", app.handleRunPipeline)
	log.Printf("Nopsai orchestrator listening on %s", cfg.NopsaiListenAddress)
	if err := http.ListenAndServe(cfg.NopsaiListenAddress, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

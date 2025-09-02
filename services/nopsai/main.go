package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"slices"
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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

type App struct {
	db     *pgxpool.Pool
	cfg    *config.Config
	cli    *client.Client
	encKey []byte
}

type RunListItem struct {
	RunID         string    `json:"run_id"`
	PipelineName  string    `json:"pipeline_name"`
	Status        string    `json:"status"`
	GitCommitSHA  string    `json:"git_commit_sha"`
	GitRepoName   string    `json:"git_repo_name"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	Duration      string    `json:"duration"`
	IsComplete    bool      `json:"is_complete"`
	ParentRunID   *string   `json:"parent_run_id"`
	GitPusherName string    `json:"git_pusher_name"`
}

type StepDetail struct {
	Name      string       `json:"name"`
	Status    string       `json:"status"`
	DependsOn []string     `json:"depends_on"`
	Tasks     []TaskDetail `json:"tasks"`
	Duration  string       `json:"duration"`
}

type TaskDetail struct {
	TaskID     string    `json:"task_id"`
	StepName   string    `json:"step_name"`
	TaskName   string    `json:"task_name"`
	Status     string    `json:"status"`
	ExitCode   *int      `json:"exit_code"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	TaskIndex  int       `json:"task_index"`
}

type RunDetail struct {
	RunInfo            RunListItem     `json:"run_info"`
	Steps              []StepDetail    `json:"steps"`
	PipelineDefinition models.Pipeline `json:"pipeline_definition"`
}

type StepStatusUpdate struct {
	Status   string `json:"status"`
	ExitCode int    `json:"exit_code"`
}

type SecretRequest struct {
	Value string `json:"value"`
}

type EnvRequest struct {
	Value string `json:"value"`
}

type PipelineRequest struct {
	Definition string `json:"definition"`
}

type TriggerOverrideRequest struct {
	TriggerDefinition string `json:"trigger_definition"`
}

type FinalizeRequest struct {
	Status string `json:"status"`
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

// corsMiddleware allows cross-origin requests from the UI development server.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*") // Allow any origin for simplicity in POC
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func validatePipeline(pipeline *models.Pipeline) error {
	if pipeline.Name == "" {
		return fmt.Errorf("'name' is a required field")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`).MatchString(pipeline.Name) {
		return fmt.Errorf("pipeline name can only contain alphanumeric characters, underscores, dots, and hyphens")
	}
	if pipeline.ContainerImage == "" && len(pipeline.Steps) > 0 && pipeline.Steps[0].Image == "" {
		return fmt.Errorf("'container_image' is a required field if steps don't have their own image")
	}
	if len(pipeline.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}

	allStepNames := make(map[string]bool)
	stepToTaskNames := make(map[string]map[string]bool)

	// First pass: Collect all step and task names
	for _, step := range pipeline.Steps {
		if step.Name == "" {
			return fmt.Errorf("a step is missing its required 'name' field")
		}
		if allStepNames[step.Name] {
			return fmt.Errorf("duplicate step name '%s' found. Step names must be unique", step.Name)
		}
		allStepNames[step.Name] = true
		stepToTaskNames[step.Name] = make(map[string]bool)

		isIncludeStep := step.Include != ""
		isTaskStep := len(step.Tasks) > 0
		isLegacyStep := step.Goal != "" || step.Script != ""

		if isIncludeStep {
			if isTaskStep || isLegacyStep {
				return fmt.Errorf("step '%s' is an 'include' step and cannot also contain 'tasks', 'goal', or 'script'", step.Name)
			}
		} else if isTaskStep {
			if isLegacyStep {
				return fmt.Errorf("step '%s' has tasks and should not also contain 'goal' or 'script'", step.Name)
			}
			for _, task := range step.Tasks {
				if task.Name == "" {
					return fmt.Errorf("a task in step '%s' is missing its required 'name' field", step.Name)
				}
				if stepToTaskNames[step.Name][task.Name] {
					return fmt.Errorf("duplicate task name '%s' found within step '%s'. Task names must be unique within a step", task.Name, step.Name)
				}
				stepToTaskNames[step.Name][task.Name] = true
			}
		} else if !isLegacyStep {
			return fmt.Errorf("step '%s' must contain 'include', 'tasks', 'goal', or 'script'", step.Name)
		}
	}

	// Second pass: Validate dependencies based on the corrected rules
	for _, step := range pipeline.Steps {
		// Rule: A step can only depend on other steps.
		for _, depName := range step.DependsOn {
			if !allStepNames[depName] {
				return fmt.Errorf("step '%s' has an undefined dependency: '%s'", step.Name, depName)
			}
		}

		// Rule: If a step has tasks, those tasks can only depend on other tasks within the SAME step.
		if len(step.Tasks) > 0 {
			for _, task := range step.Tasks {
				for _, depName := range task.DependsOn {
					if !stepToTaskNames[step.Name][depName] {
						return fmt.Errorf("task '%s' in step '%s' has an invalid dependency: '%s'. Tasks can only depend on other tasks within the same step", task.Name, step.Name, depName)
					}
				}
			}
		}
	}

	return nil
}

func (a *App) encrypt(text string) (string, error) {
	block, err := aes.NewCipher(a.encKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(text), nil)
	return hex.EncodeToString(ciphertext), nil
}

func (a *App) decrypt(text string) (string, error) {
	ciphertext, err := hex.DecodeString(text)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(a.encKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (a *App) handleListGeneralEnvs(w http.ResponseWriter, r *http.Request) {
	env := r.URL.Query().Get("env")
	var rows pgx.Rows
	var err error

	if env != "" {
		rows, err = a.db.Query(context.Background(), "SELECT name FROM environments WHERE repository_name IS NULL AND environment = $1 ORDER BY name ASC", env)
	} else {
		rows, err = a.db.Query(context.Background(), "SELECT name FROM environments WHERE repository_name IS NULL AND environment IS NULL ORDER BY name ASC")
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to query general environments from database")
		http.Error(w, "Failed to retrieve environments", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Error().Err(err).Msg("Failed to scan environment name")
			http.Error(w, "Failed to process environments", http.StatusInternalServerError)
			return
		}
		names = append(names, name)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(names)
}

func (a *App) handleCreateOrUpdateGeneralEnv(w http.ResponseWriter, r *http.Request) {
	envName := r.PathValue("envName")
	env := r.URL.Query().Get("env")
	var req EnvRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var err error
	if env != "" {
		query := `INSERT INTO environments (name, value, repository_name, environment, updated_at) VALUES ($1, $2, NULL, $3, NOW())
				  ON CONFLICT (name, repository_name, environment) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`
		_, err = a.db.Exec(context.Background(), query, envName, req.Value, env)
	} else {
		query := `INSERT INTO environments (name, value, repository_name, environment, updated_at) VALUES ($1, $2, NULL, NULL, NOW())
				  ON CONFLICT (name, repository_name, environment) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`
		_, err = a.db.Exec(context.Background(), query, envName, req.Value)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to save general environment to database")
		http.Error(w, "Failed to save environment", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeleteGeneralEnv(w http.ResponseWriter, r *http.Request) {
	envName := r.PathValue("envName")
	env := r.URL.Query().Get("env")
	var err error

	if env != "" {
		_, err = a.db.Exec(context.Background(), "DELETE FROM environments WHERE name = $1 AND repository_name IS NULL AND environment = $2", envName, env)
	} else {
		_, err = a.db.Exec(context.Background(), "DELETE FROM environments WHERE name = $1 AND repository_name IS NULL AND environment IS NULL", envName)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to delete general environment from database")
		http.Error(w, "Failed to delete environment", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleListRepoEnvs(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)
	env := r.URL.Query().Get("env")
	var rows pgx.Rows
	var err error

	if env != "" {
		rows, err = a.db.Query(context.Background(), "SELECT name FROM environments WHERE repository_name = $1 AND environment = $2 ORDER BY name ASC", fullName, env)
	} else {
		rows, err = a.db.Query(context.Background(), "SELECT name FROM environments WHERE repository_name = $1 AND environment IS NULL ORDER BY name ASC", fullName)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to query repo environments from database")
		http.Error(w, "Failed to retrieve environments", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Error().Err(err).Msg("Failed to scan environment name")
			http.Error(w, "Failed to process environments", http.StatusInternalServerError)
			return
		}
		names = append(names, name)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(names)
}

func (a *App) handleCreateOrUpdateRepoEnv(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)
	envName := r.PathValue("envName")
	env := r.URL.Query().Get("env")
	var req EnvRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var err error
	if env != "" {
		query := `INSERT INTO environments (name, value, repository_name, environment, updated_at) VALUES ($1, $2, $3, $4, NOW())
				  ON CONFLICT (name, repository_name, environment) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`
		_, err = a.db.Exec(context.Background(), query, envName, req.Value, fullName, env)
	} else {
		query := `INSERT INTO environments (name, value, repository_name, environment, updated_at) VALUES ($1, $2, $3, NULL, NOW())
				  ON CONFLICT (name, repository_name, environment) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`
		_, err = a.db.Exec(context.Background(), query, envName, req.Value, fullName)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to save repo environment to database")
		http.Error(w, "Failed to save environment", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeleteRepoEnv(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)
	envName := r.PathValue("envName")
	env := r.URL.Query().Get("env")
	var err error

	if env != "" {
		_, err = a.db.Exec(context.Background(), "DELETE FROM environments WHERE name = $1 AND repository_name = $2 AND environment = $3", envName, fullName, env)
	} else {
		_, err = a.db.Exec(context.Background(), "DELETE FROM environments WHERE name = $1 AND repository_name = $2 AND environment IS NULL", envName, fullName)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to delete repo environment from database")
		http.Error(w, "Failed to delete environment", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) prepareEnvironmentForPipeline(pipeline models.Pipeline, gitContext map[string]string, environment string) (map[string]string, error) {
	finalEnv := make(map[string]string)
	repoFullName := fmt.Sprintf("%s/%s", gitContext["repo_owner"], gitContext["repo_name"])

	// The 'environment' block from the YAML is now a list of required variable names.
	requiredVars := pipeline.Environment

	for _, varName := range requiredVars {
		var value string
		var err error
		found := false

		if environment != "" {
			// Precedence: 1. Repo/Env -> 2. General/Env
			err = a.db.QueryRow(context.Background(), "SELECT value FROM environments WHERE name = $1 AND repository_name = $2 AND environment = $3", varName, repoFullName, environment).Scan(&value)
			if err == nil {
				found = true
			}
			if !found {
				err = a.db.QueryRow(context.Background(), "SELECT value FROM environments WHERE name = $1 AND repository_name IS NULL AND environment = $2", varName, environment).Scan(&value)
				if err == nil {
					found = true
				}
			}

			if !found {
				return nil, fmt.Errorf("pipeline aborted: required environment variable '%s' not found for environment '%s'", varName, environment)
			}

		} else {
			// Precedence: 1. Repo/No-Env -> 2. General/No-Env
			err = a.db.QueryRow(context.Background(), "SELECT value FROM environments WHERE name = $1 AND repository_name = $2 AND environment IS NULL", varName, repoFullName).Scan(&value)
			if err == nil {
				found = true
			}
			if !found {
				err = a.db.QueryRow(context.Background(), "SELECT value FROM environments WHERE name = $1 AND repository_name IS NULL AND environment IS NULL", varName).Scan(&value)
				if err == nil {
					found = true
				}
			}
		}

		if !found {
			return nil, fmt.Errorf("pipeline aborted: required environment variable '%s' not found in the default scope", varName)
		}

		finalEnv[varName] = value
	}

	return finalEnv, nil
}

func (a *App) handleListGeneralSecrets(w http.ResponseWriter, r *http.Request) {
	env := r.URL.Query().Get("env")
	var rows pgx.Rows
	var err error

	if env != "" {
		query := "SELECT name FROM secrets WHERE repository_name IS NULL AND environment = $1 ORDER BY name ASC"
		rows, err = a.db.Query(context.Background(), query, env)
	} else {
		query := "SELECT name FROM secrets WHERE repository_name IS NULL AND environment IS NULL ORDER BY name ASC"
		rows, err = a.db.Query(context.Background(), query)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to query general secrets from database")
		http.Error(w, "Failed to retrieve secrets", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var secretNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Error().Err(err).Msg("Failed to scan secret name")
			http.Error(w, "Failed to process secrets", http.StatusInternalServerError)
			return
		}
		secretNames = append(secretNames, name)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(secretNames)
}

func (a *App) handleCreateOrUpdateGeneralSecret(w http.ResponseWriter, r *http.Request) {
	secretName := r.PathValue("secretName")
	env := r.URL.Query().Get("env")
	var req SecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	encryptedValue, err := a.encrypt(req.Value)
	if err != nil {
		log.Error().Err(err).Msg("Failed to encrypt secret")
		http.Error(w, "Failed to encrypt secret", http.StatusInternalServerError)
		return
	}

	if env != "" {
		query := `INSERT INTO secrets (name, value, repository_name, environment, updated_at) VALUES ($1, $2, NULL, $3, NOW())
				  ON CONFLICT (name, repository_name, environment) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`
		_, err = a.db.Exec(context.Background(), query, secretName, encryptedValue, env)
	} else {
		query := `INSERT INTO secrets (name, value, repository_name, environment, updated_at) VALUES ($1, $2, NULL, NULL, NOW())
				  ON CONFLICT (name, repository_name, environment) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`
		_, err = a.db.Exec(context.Background(), query, secretName, encryptedValue)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to save general secret to database")
		http.Error(w, "Failed to save secret", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeleteGeneralSecret(w http.ResponseWriter, r *http.Request) {
	secretName := r.PathValue("secretName")
	env := r.URL.Query().Get("env")
	var err error

	if env != "" {
		_, err = a.db.Exec(context.Background(), "DELETE FROM secrets WHERE name = $1 AND repository_name IS NULL AND environment = $2", secretName, env)
	} else {
		_, err = a.db.Exec(context.Background(), "DELETE FROM secrets WHERE name = $1 AND repository_name IS NULL AND environment IS NULL", secretName)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to delete general secret from database")
		http.Error(w, "Failed to delete secret", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleListRepoSecrets(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)
	env := r.URL.Query().Get("env")
	var rows pgx.Rows
	var err error

	if env != "" {
		rows, err = a.db.Query(context.Background(), "SELECT name FROM secrets WHERE repository_name = $1 AND environment = $2 ORDER BY name ASC", fullName, env)
	} else {
		rows, err = a.db.Query(context.Background(), "SELECT name FROM secrets WHERE repository_name = $1 AND environment IS NULL ORDER BY name ASC", fullName)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to query repo secrets from database")
		http.Error(w, "Failed to retrieve secrets", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var secretNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Error().Err(err).Msg("Failed to scan secret name")
			http.Error(w, "Failed to process secrets", http.StatusInternalServerError)
			return
		}
		secretNames = append(secretNames, name)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(secretNames)
}

func (a *App) handleCreateOrUpdateRepoSecret(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)
	secretName := r.PathValue("secretName")
	env := r.URL.Query().Get("env")
	var req SecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	encryptedValue, err := a.encrypt(req.Value)
	if err != nil {
		log.Error().Err(err).Msg("Failed to encrypt secret")
		http.Error(w, "Failed to encrypt secret", http.StatusInternalServerError)
		return
	}

	if env != "" {
		query := `INSERT INTO secrets (name, value, repository_name, environment, updated_at) VALUES ($1, $2, $3, $4, NOW())
				  ON CONFLICT (name, repository_name, environment) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`
		_, err = a.db.Exec(context.Background(), query, secretName, encryptedValue, fullName, env)
	} else {
		query := `INSERT INTO secrets (name, value, repository_name, environment, updated_at) VALUES ($1, $2, $3, NULL, NOW())
				  ON CONFLICT (name, repository_name, environment) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`
		_, err = a.db.Exec(context.Background(), query, secretName, encryptedValue, fullName)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to save repo secret to database")
		http.Error(w, "Failed to save secret", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeleteRepoSecret(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)
	secretName := r.PathValue("secretName")
	env := r.URL.Query().Get("env")
	var err error

	if env != "" {
		_, err = a.db.Exec(context.Background(), "DELETE FROM secrets WHERE name = $1 AND repository_name = $2 AND environment = $3", secretName, fullName, env)
	} else {
		_, err = a.db.Exec(context.Background(), "DELETE FROM secrets WHERE name = $1 AND repository_name = $2 AND environment IS NULL", secretName, fullName)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to delete repo secret from database")
		http.Error(w, "Failed to delete secret", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) prepareSecretsForPipeline(pipeline models.Pipeline, gitContext map[string]string, environment string) (map[string]string, error) {
	requiredSecrets := make(map[string]struct{})
	for _, step := range pipeline.Steps {
		for _, secretName := range step.Secrets {
			requiredSecrets[secretName] = struct{}{}
		}
	}

	if len(requiredSecrets) == 0 {
		return nil, nil
	}

	finalSecrets := make(map[string]string)
	repoFullName := fmt.Sprintf("%s/%s", gitContext["repo_owner"], gitContext["repo_name"])

	for secretName := range requiredSecrets {
		var encryptedValue string
		var err error
		found := false

		if environment != "" {
			// --- STRICT ENVIRONMENT MODE ---
			// Precedence: 1. Repo/Env -> 2. General/Env

			// 1. Check for repo-specific, environment-specific secret
			err = a.db.QueryRow(context.Background(), "SELECT value FROM secrets WHERE name = $1 AND repository_name = $2 AND environment = $3", secretName, repoFullName, environment).Scan(&encryptedValue)
			if err == nil {
				found = true
			}

			// 2. If not found, check for general, environment-specific secret
			if !found {
				err = a.db.QueryRow(context.Background(), "SELECT value FROM secrets WHERE name = $1 AND repository_name IS NULL AND environment = $2", secretName, environment).Scan(&encryptedValue)
				if err == nil {
					found = true
				}
			}

			if !found {
				// FAIL FAST: If an environment is specified, we do not fall back to non-environmental secrets.
				return nil, fmt.Errorf("pipeline aborted: required secret '%s' not found for environment '%s'", secretName, environment)
			}

		} else {
			// --- DEFAULT (NO ENVIRONMENT) MODE ---
			// Precedence: 1. Repo/No-Env -> 2. General/No-Env

			// 1. Check for repo-specific, non-environment secret
			err = a.db.QueryRow(context.Background(), "SELECT value FROM secrets WHERE name = $1 AND repository_name = $2 AND environment IS NULL", secretName, repoFullName).Scan(&encryptedValue)
			if err == nil {
				found = true
			}

			// 2. If not found, check for general, non-environment secret
			if !found {
				err = a.db.QueryRow(context.Background(), "SELECT value FROM secrets WHERE name = $1 AND repository_name IS NULL AND environment IS NULL", secretName).Scan(&encryptedValue)
				if err == nil {
					found = true
				}
			}
		}

		if !found {
			// This will catch the failure for the non-environment mode.
			return nil, fmt.Errorf("pipeline aborted: required secret '%s' not found in the default scope", secretName)
		}

		decryptedValue, decryptErr := a.decrypt(encryptedValue)
		if decryptErr != nil {
			log.Error().Err(decryptErr).Str("secret_name", secretName).Msg("Failed to decrypt secret; this will cause a failure.")
			return nil, fmt.Errorf("pipeline aborted: failed to decrypt secret '%s'", secretName)
		}
		finalSecrets[secretName] = decryptedValue
	}

	return finalSecrets, nil
}

func (a *App) handleListPipelines(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(context.Background(), "SELECT name FROM pipelines ORDER BY name ASC")
	if err != nil {
		log.Error().Err(err).Msg("Failed to query pipelines from database")
		http.Error(w, "Failed to retrieve pipelines", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var pipelineNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Error().Err(err).Msg("Failed to scan pipeline name")
			http.Error(w, "Failed to process pipelines", http.StatusInternalServerError)
			return
		}
		pipelineNames = append(pipelineNames, name)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(pipelineNames)
}

// handleListTriggerOverrides retrieves the names of all repositories with active trigger overrides.
func (a *App) handleListTriggerOverrides(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(context.Background(), "SELECT repository_name FROM trigger_overrides ORDER BY repository_name ASC")
	if err != nil {
		log.Error().Err(err).Msg("Failed to query trigger overrides from database")
		http.Error(w, "Failed to retrieve trigger overrides", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var repoNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Error().Err(err).Msg("Failed to scan repository name")
			http.Error(w, "Failed to process trigger overrides", http.StatusInternalServerError)
			return
		}
		repoNames = append(repoNames, name)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(repoNames)
}

func (a *App) handleGetPipeline(w http.ResponseWriter, r *http.Request) {
	pipelineName := r.PathValue("pipelineName")

	var pipelineDef string
	err := a.db.QueryRow(context.Background(), "SELECT definition FROM pipelines WHERE name = $1", pipelineName).Scan(&pipelineDef)
	if err != nil {
		log.Error().Err(err).Str("pipeline_name", pipelineName).Msg("Pipeline not found in database")
		http.Error(w, "Pipeline not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(pipelineDef))
}

func (a *App) handleGetTriggerOverride(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)

	var triggerDef string
	err := a.db.QueryRow(context.Background(), "SELECT trigger_definition FROM trigger_overrides WHERE repository_name = $1", fullName).Scan(&triggerDef)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(triggerDef))
}

func (a *App) handleCreateOrUpdateTriggerOverride(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)

	triggerDef, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	var genericYAML map[string]interface{}
	if err := yaml.Unmarshal(triggerDef, &genericYAML); err != nil {
		http.Error(w, fmt.Sprintf("Invalid YAML format: %v", err), http.StatusBadRequest)
		return
	}

	if _, hasStepsKey := genericYAML["steps"]; hasStepsKey {
		http.Error(w, "Validation failed: The provided file appears to be a pipeline, not a trigger manifest. A trigger must contain 'triggers', not 'steps'.", http.StatusBadRequest)
		return
	}

	var manifest models.Manifest
	if err := yaml.Unmarshal(triggerDef, &manifest); err != nil {
		errorMsg := fmt.Sprintf("Trigger validation failed: %v", err)
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	query := `INSERT INTO trigger_overrides (repository_name, trigger_definition) VALUES ($1, $2)
			  ON CONFLICT (repository_name) DO UPDATE SET trigger_definition = $2`
	_, err = a.db.Exec(context.Background(), query, fullName, string(triggerDef))
	if err != nil {
		log.Error().Err(err).Msg("Failed to save trigger override")
		http.Error(w, "Failed to save trigger override", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeleteTriggerOverride(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)

	_, err := a.db.Exec(context.Background(), "DELETE FROM trigger_overrides WHERE repository_name = $1", fullName)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete trigger override from database")
		http.Error(w, "Failed to delete trigger override", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleCreateOrUpdatePipeline(w http.ResponseWriter, r *http.Request) {
	pipelineName := r.PathValue("pipelineName")

	pipelineDef, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	var genericYAML map[string]interface{}
	if err := yaml.Unmarshal(pipelineDef, &genericYAML); err != nil {
		http.Error(w, fmt.Sprintf("Invalid YAML format: %v", err), http.StatusBadRequest)
		return
	}

	if _, hasTriggersKey := genericYAML["triggers"]; hasTriggersKey {
		http.Error(w, "Validation failed: The provided file appears to be a trigger manifest, not a pipeline. A pipeline must contain 'steps', not 'triggers'.", http.StatusBadRequest)
		return
	}

	var pipeline models.Pipeline
	if err := yaml.Unmarshal(pipelineDef, &pipeline); err != nil {
		http.Error(w, fmt.Sprintf("Pipeline YAML is malformed: %v", err), http.StatusBadRequest)
		return
	}

	if err := validatePipeline(&pipeline); err != nil {
		http.Error(w, fmt.Sprintf("Pipeline validation failed: %v", err), http.StatusBadRequest)
		return
	}

	query := `INSERT INTO pipelines (name, definition, updated_at) VALUES ($1, $2, NOW())
			  ON CONFLICT (name) DO UPDATE SET definition = $2, updated_at = NOW()`
	_, err = a.db.Exec(context.Background(), query, pipelineName, string(pipelineDef))
	if err != nil {
		log.Error().Err(err).Msg("Failed to save pipeline to database")
		http.Error(w, "Failed to save pipeline", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeletePipeline(w http.ResponseWriter, r *http.Request) {
	pipelineName := r.PathValue("pipelineName")
	_, err := a.db.Exec(context.Background(), "DELETE FROM pipelines WHERE name = $1", pipelineName)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete pipeline from database")
		http.Error(w, "Failed to delete pipeline", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func sanitizeInput(name string) string {
	sanitized := strings.ReplaceAll(name, " ", "-")
	return nonAlphanumericRegex.ReplaceAllString(sanitized, "")
}

func (a *App) handleListRuns(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(context.Background(), `
		SELECT
			run_id, pipeline_name, status, COALESCE(git_commit_sha, ''),
			COALESCE(git_repo_name, ''), started_at, finished_at, parent_run_id,
			COALESCE(git_pusher_name, '')
		FROM runs
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query runs from database")
		http.Error(w, "Failed to retrieve runs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var runs []RunListItem
	for rows.Next() {
		var run RunListItem
		var startedAt, finishedAt sql.NullTime
		var commitSHA, repoName, pusherName sql.NullString
		err := rows.Scan(
			&run.RunID, &run.PipelineName, &run.Status, &commitSHA,
			&repoName, &startedAt, &finishedAt, &run.ParentRunID, &pusherName,
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to scan run row")
			continue
		}
		run.GitCommitSHA = commitSHA.String
		run.GitRepoName = repoName.String
		run.GitPusherName = pusherName.String
		if startedAt.Valid {
			run.StartedAt = startedAt.Time
		}
		if finishedAt.Valid {
			run.FinishedAt = finishedAt.Time
			run.Duration = run.FinishedAt.Sub(run.StartedAt).Round(time.Second).String()
			run.IsComplete = true
		} else if startedAt.Valid {
			run.Duration = time.Since(run.StartedAt).Round(time.Second).String()
			run.IsComplete = false
		} else {
			run.IsComplete = true // If it hasn't even started, it's not "running"
		}
		runs = append(runs, run)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runs)
}

func (a *App) handleGetRunDetails(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")

	// Get main run info
	var run RunListItem
	var pipelineDefinition string
	var startedAt, finishedAt sql.NullTime
	var commitSHA, repoName, pusherName sql.NullString
	err := a.db.QueryRow(context.Background(), `
		SELECT
			run_id, pipeline_name, status, COALESCE(git_commit_sha, ''),
			COALESCE(git_repo_name, ''), started_at, finished_at, parent_run_id,
			COALESCE(git_pusher_name, ''), pipeline_definition
		FROM runs
		WHERE run_id = $1
	`, runID).Scan(
		&run.RunID, &run.PipelineName, &run.Status, &commitSHA,
		&repoName, &startedAt, &finishedAt, &run.ParentRunID, &pusherName, &pipelineDefinition,
	)

	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to query run details from database")
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}
	run.GitCommitSHA = commitSHA.String
	run.GitRepoName = repoName.String
	run.GitPusherName = pusherName.String
	if startedAt.Valid {
		run.StartedAt = startedAt.Time
	}
	if finishedAt.Valid {
		run.FinishedAt = finishedAt.Time
		run.Duration = run.FinishedAt.Sub(run.StartedAt).Round(time.Second).String()
		run.IsComplete = true
	} else if startedAt.Valid {
		run.Duration = time.Since(run.StartedAt).Round(time.Second).String()
	}

	// Get all tasks for the run
	taskRows, err := a.db.Query(context.Background(), `
		SELECT task_id, step_name, task_name, status, exit_code, started_at, finished_at, task_index
		FROM tasks
		WHERE run_id = $1
		ORDER BY task_index ASC
	`, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to query tasks for run")
		http.Error(w, "Failed to retrieve tasks", http.StatusInternalServerError)
		return
	}
	defer taskRows.Close()

	tasksByStep := make(map[string][]TaskDetail)
	for taskRows.Next() {
		var task TaskDetail
		var startedAt, finishedAt sql.NullTime
		if err := taskRows.Scan(&task.TaskID, &task.StepName, &task.TaskName, &task.Status, &task.ExitCode, &startedAt, &finishedAt, &task.TaskIndex); err != nil {
			log.Error().Err(err).Msg("Failed to scan task row")
			continue
		}
		if startedAt.Valid {
			task.StartedAt = startedAt.Time
		}
		if finishedAt.Valid {
			task.FinishedAt = finishedAt.Time
		}
		tasksByStep[task.StepName] = append(tasksByStep[task.StepName], task)
	}

	// Parse pipeline definition to get step dependencies
	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(pipelineDefinition), &pipeline); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to parse pipeline definition for dependencies")
		http.Error(w, "Failed to parse pipeline definition", http.StatusInternalServerError)
		return
	}

	var steps []StepDetail
	for _, pStep := range pipeline.Steps {
		stepTasks := tasksByStep[pStep.Name]

		status := "pending"
		var stepDuration time.Duration
		var firstTaskStart, lastTaskFinish time.Time

		if len(stepTasks) > 0 {
			for _, t := range stepTasks {
				if firstTaskStart.IsZero() || (!t.StartedAt.IsZero() && t.StartedAt.Before(firstTaskStart)) {
					firstTaskStart = t.StartedAt
				}
				if !t.FinishedAt.IsZero() && t.FinishedAt.After(lastTaskFinish) {
					lastTaskFinish = t.FinishedAt
				}
			}

			if !firstTaskStart.IsZero() {
				if !lastTaskFinish.IsZero() {
					stepDuration = lastTaskFinish.Sub(firstTaskStart)
				} else {
					stepDuration = time.Since(firstTaskStart)
				}
			}

			if slices.ContainsFunc(stepTasks, func(t TaskDetail) bool {
				return strings.Contains(t.Status, "fail") && !strings.Contains(t.Status, "ignore")
			}) {
				status = "failure"
			} else if slices.ContainsFunc(stepTasks, func(t TaskDetail) bool { return t.Status == "started" }) {
				status = "running"
			} else if allTasksDone(stepTasks) {
				if slices.ContainsFunc(stepTasks, func(t TaskDetail) bool { return strings.Contains(t.Status, "ignore") }) {
					status = "failed (ignored)"
				} else {
					status = "success"
				}
			} else if slices.ContainsFunc(stepTasks, func(t TaskDetail) bool { return t.Status == "skipped" }) {
				status = "skipped"
			}
		}

		steps = append(steps, StepDetail{
			Name:      pStep.Name,
			Status:    status,
			DependsOn: pStep.DependsOn,
			Tasks:     stepTasks,
			Duration:  stepDuration.Round(time.Second).String(),
		})
	}

	response := RunDetail{
		RunInfo:            run,
		Steps:              steps,
		PipelineDefinition: pipeline,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func allTasksDone(tasks []TaskDetail) bool {
	if len(tasks) == 0 {
		return false
	}
	for _, t := range tasks {
		if t.Status != "completed" && !strings.Contains(t.Status, "ignore") {
			return false
		}
	}
	return true
}

func (a *App) launchAndRunPipeline(
	w http.ResponseWriter,
	pipeline models.Pipeline,
	pipelineDef []byte,
	gitContext map[string]string,
	timeoutDuration time.Duration,
	parentRunID string,
	parentHistory string,
	environment string, // Add environment to the function signature
) {
	runID := uuid.New()
	log.Info().Str("run_id", runID.String()).Str("parent_run_id", parentRunID).Msgf("Launching pipeline: %s", pipeline.Name)

	var timeoutAt sql.NullTime
	if timeoutDuration > 0 {
		timeoutAt.Time = time.Now().Add(timeoutDuration)
		timeoutAt.Valid = true
	}

	tx, err := a.db.Begin(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Failed to start database transaction")
		http.Error(w, "Failed to start database transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(context.Background())

	var parentRunIDSQL sql.NullString
	if parentRunID != "" {
		parentRunIDSQL.String = parentRunID
		parentRunIDSQL.Valid = true
	}

	_, err = tx.Exec(context.Background(),
		`INSERT INTO runs (run_id, parent_run_id, pipeline_name, status, started_at, timeout_at, pipeline_definition,
			git_repo_owner, git_repo_name, git_clone_url, git_ssh_url, git_ref,
			git_commit_sha, git_commit_url, git_commit_message, git_commit_author_name,
			git_commit_author_email, git_commit_author_username, git_pusher_name,
			git_pusher_email, git_check_run_id)
			VALUES ($1, $2, $3, 'pending', NOW(), $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
		runID, parentRunIDSQL, pipeline.Name, timeoutAt, string(pipelineDef),
		gitContext["repo_owner"], gitContext["repo_name"], gitContext["clone_url"], gitContext["ssh_url"], gitContext["ref"],
		gitContext["commit_sha"], gitContext["commit_url"], gitContext["commit_message"], gitContext["commit_author_name"],
		gitContext["commit_author_email"], gitContext["commit_author_username"], gitContext["pusher_name"],
		gitContext["pusher_email"], gitContext["check_run_id"],
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to insert run record")
		http.Error(w, "Failed to create run record", http.StatusInternalServerError)
		return
	}

	for _, step := range pipeline.Steps {
		if step.Include != "" {
			_, err := tx.Exec(context.Background(),
				"INSERT INTO tasks (task_id, run_id, step_name, task_name, status, task_index) VALUES (gen_random_uuid(), $1, $2, $3, 'pending', $4)",
				runID, step.Name, step.Name, 1,
			)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to insert 'include' step %s as a task", step.Name)
				http.Error(w, "Failed to create task records", http.StatusInternalServerError)
				return
			}
		} else if len(step.Tasks) > 0 {
			for i, task := range step.Tasks {
				_, err := tx.Exec(context.Background(),
					"INSERT INTO tasks (task_id, run_id, step_name, task_name, status, task_index) VALUES (gen_random_uuid(), $1, $2, $3, 'pending', $4)",
					runID, step.Name, task.Name, i+1,
				)
				if err != nil {
					log.Error().Err(err).Msgf("Failed to insert task %s for step %s", task.Name, step.Name)
					http.Error(w, "Failed to create task records", http.StatusInternalServerError)
					return
				}
			}
		} else { // Legacy step
			_, err := tx.Exec(context.Background(),
				"INSERT INTO tasks (task_id, run_id, step_name, task_name, status, task_index) VALUES (gen_random_uuid(), $1, $2, $3, 'pending', $4)",
				runID, step.Name, step.Name, 1,
			)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to insert step %s as a task", step.Name)
				http.Error(w, "Failed to create task records", http.StatusInternalServerError)
				return
			}
		}
	}

	if err := tx.Commit(context.Background()); err != nil {
		log.Error().Err(err).Msg("Failed to commit transaction")
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	go a.launchAgent(runID.String(), pipeline, pipelineDef, timeoutDuration, gitContext, parentHistory, environment)

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Pipeline run created successfully with ID: " + runID.String()))
}

func (a *App) handleRunPipeline(w http.ResponseWriter, r *http.Request) {
	var pipeline models.Pipeline
	var pipelineDef []byte
	var err error

	parentRunID := r.Header.Get("X-Nopsai-Parent-Run-ID")
	parentHistory := r.Header.Get("X-Nopsai-Parent-History")
	environment := r.Header.Get("X-Nopsai-Environment")
	pipelineNameFromPath := r.PathValue("pipelineName")

	if pipelineNameFromPath != "" {
		var pipelineDefStr string
		err = a.db.QueryRow(context.Background(), "SELECT definition FROM pipelines WHERE name = $1", pipelineNameFromPath).Scan(&pipelineDefStr)
		if err != nil {
			log.Error().Err(err).Str("pipeline_name", pipelineNameFromPath).Msg("Pipeline not found in database")
			http.Error(w, "Pipeline not found", http.StatusNotFound)
			return
		}
		pipelineDef = []byte(pipelineDefStr)

		if err = yaml.Unmarshal(pipelineDef, &pipeline); err != nil {
			http.Error(w, "Error parsing stored YAML pipeline", http.StatusInternalServerError)
			return
		}
	} else {
		pipelineDef, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusInternalServerError)
			return
		}
		if err = yaml.Unmarshal(pipelineDef, &pipeline); err != nil {
			http.Error(w, fmt.Sprintf("Pipeline YAML is malformed: %v", err), http.StatusBadRequest)
			return
		}
	}

	pipeline.Name = sanitizeInput(pipeline.Name)

	// Resolve step includes before validation and launch
	resolvedPipeline, err := a.resolveStepIncludes(&pipeline)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to resolve step includes: %v", err), http.StatusBadRequest)
		return
	}

	if err := validatePipeline(resolvedPipeline); err != nil {
		http.Error(w, fmt.Sprintf("Pipeline validation failed: %v", err), http.StatusBadRequest)
		return
	}

	// Marshal the resolved pipeline to pass to the agent
	resolvedPipelineDef, err := yaml.Marshal(resolvedPipeline)
	if err != nil {
		http.Error(w, "Failed to marshal resolved pipeline", http.StatusInternalServerError)
		return
	}

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

	if parentRunID != "" {
		parentPipelineName := r.Header.Get("X-Nopsai-Parent-Pipeline-Name")
		gitbotURL := fmt.Sprintf("%s/v1/checks/create-child", a.cfg.NopsaiGitBotAPIURL)

		payload := map[string]string{
			"owner":               gitContext["repo_owner"],
			"repo":                gitContext["repo_name"],
			"ref":                 gitContext["commit_sha"],
			"parent_name":         parentPipelineName,
			"include_name":        pipeline.Name,
			"pipeline_definition": string(resolvedPipelineDef),
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(gitbotURL, "application/json", bytes.NewBuffer(body))
		if err != nil || resp.StatusCode != http.StatusOK {
			log.Error().Err(err).Msg("Failed to request new check run from git-bot")
			http.Error(w, "Failed to create GitHub check for included pipeline", http.StatusInternalServerError)
			return
		}

		var respData map[string]int64
		if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
			log.Error().Err(err).Msg("Failed to decode git-bot response for new check run")
			http.Error(w, "Failed to decode git-bot response", http.StatusInternalServerError)
			return
		}
		resp.Body.Close()

		gitContext["check_run_id"] = strconv.FormatInt(respData["check_run_id"], 10)
	}

	timeoutStr := resolvedPipeline.Timeout
	if timeoutStr == "" {
		timeoutStr = a.cfg.DefaultPipelineTimeout
	}

	var timeoutDuration time.Duration
	if timeoutStr != "" {
		duration, err := time.ParseDuration(timeoutStr)
		if err != nil {
			http.Error(w, "Invalid timeout duration format", http.StatusBadRequest)
			return
		}
		timeoutDuration = duration
	}

	a.launchAndRunPipeline(w, *resolvedPipeline, resolvedPipelineDef, gitContext, timeoutDuration, parentRunID, parentHistory, environment)
}

func (a *App) handleRerunPipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	originalRunID := r.PathValue("runID")

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

	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(pipelineDef.String), &pipeline); err != nil {
		http.Error(w, "Could not parse original pipeline definition", http.StatusInternalServerError)
		return
	}

	var timeoutDuration time.Duration
	if timeoutAt.Valid {
		var originalCreatedAt time.Time
		err := a.db.QueryRow(context.Background(), "SELECT created_at FROM runs WHERE run_id = $1", originalRunID).Scan(&originalCreatedAt)
		if err != nil {
			log.Error().Err(err).Str("original_run_id", originalRunID).Msg("Failed to get original run creation time for timeout calculation")
			http.Error(w, "Could not calculate rerun timeout", http.StatusInternalServerError)
			return
		}
		originalDuration := timeoutAt.Time.Sub(originalCreatedAt)
		timeoutDuration = originalDuration
	}

	a.launchAndRunPipeline(w, pipeline, []byte(pipelineDef.String), gitContext, timeoutDuration, "", "", "")
}

func (a *App) handleTaskUpdate(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	stepName := r.PathValue("stepName")
	taskName := r.PathValue("taskName")

	var update StepStatusUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var query string
	if update.Status == "started" {
		query = "UPDATE tasks SET status = 'started', started_at = NOW() WHERE run_id = $1 AND step_name = $2 AND task_name = $3"
		_, err := a.db.Exec(context.Background(), query, runID, stepName, taskName)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Str("step", stepName).Str("task", taskName).Msg("Failed to update task start time")
			http.Error(w, "Failed to update task status", http.StatusInternalServerError)
			return
		}
	} else {
		query = "UPDATE tasks SET status = $1, exit_code = $2, finished_at = NOW() WHERE run_id = $3 AND step_name = $4 AND task_name = $5"
		_, err := a.db.Exec(context.Background(), query, update.Status, update.ExitCode, runID, stepName, taskName)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Str("step", stepName).Str("task", taskName).Msg("Failed to update task finish status")
			http.Error(w, "Failed to update task status", http.StatusInternalServerError)
			return
		}
	}

	log.Info().Str("run_id", runID).Str("step", stepName).Str("task", taskName).Str("status", update.Status).Msg("Updated task status")

	go a.notifyGitBotOfTaskStatus(runID, stepName, taskName, update.Status)

	w.WriteHeader(http.StatusOK)
}

func (a *App) resolveStepIncludes(pipeline *models.Pipeline) (*models.Pipeline, error) {
	var finalSteps []models.PipelineStep
	for _, step := range pipeline.Steps {
		if !strings.HasPrefix(step.Include, "step:") {
			finalSteps = append(finalSteps, step)
			continue
		}

		// Handle step include
		includeName := strings.TrimPrefix(step.Include, "step:")
		var stepDefStr string
		err := a.db.QueryRow(context.Background(), "SELECT definition FROM reusable_steps WHERE name = $1", includeName).Scan(&stepDefStr)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch included step '%s': %w", includeName, err)
		}

		var includedStep models.PipelineStep
		if err := yaml.Unmarshal([]byte(stepDefStr), &includedStep); err != nil {
			return nil, fmt.Errorf("failed to parse included step '%s': %w", includeName, err)
		}

		// 1. Overwrite the name (for UI consistency)
		includedStep.Name = step.Name

		// 2. Transfer metadata from the placeholder
		includedStep.DependsOn = step.DependsOn
		includedStep.IgnoreFailure = step.IgnoreFailure
		if step.LlmOutputSharing != nil {
			includedStep.LlmOutputSharing = step.LlmOutputSharing
		}

		// 3. Overwrite specific fields if they are defined in the pipeline
		if len(step.Volumes) > 0 {
			includedStep.Volumes = step.Volumes
		}
		if len(step.Secrets) > 0 {
			includedStep.Secrets = step.Secrets
		}
		if len(step.Environment) > 0 {
			includedStep.Environment = step.Environment
		}

		finalSteps = append(finalSteps, includedStep)
	}

	pipeline.Steps = finalSteps
	return pipeline, nil
}

func (a *App) handleListReusableSteps(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(context.Background(), "SELECT name FROM reusable_steps ORDER BY name ASC")
	if err != nil {
		log.Error().Err(err).Msg("Failed to query reusable steps from database")
		http.Error(w, "Failed to retrieve reusable steps", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var stepNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Error().Err(err).Msg("Failed to scan reusable step name")
			http.Error(w, "Failed to process reusable steps", http.StatusInternalServerError)
			return
		}
		stepNames = append(stepNames, name)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(stepNames)
}

func (a *App) handleGetReusableStep(w http.ResponseWriter, r *http.Request) {
	stepName := r.PathValue("stepName")

	var stepDef string
	err := a.db.QueryRow(context.Background(), "SELECT definition FROM reusable_steps WHERE name = $1", stepName).Scan(&stepDef)
	if err != nil {
		log.Error().Err(err).Str("step_name", stepName).Msg("Reusable step not found in database")
		http.Error(w, "Reusable step not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(stepDef))
}

func (a *App) handleCreateOrUpdateReusableStep(w http.ResponseWriter, r *http.Request) {
	stepName := r.PathValue("stepName")

	stepDef, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	var step models.PipelineStep
	if err := yaml.Unmarshal(stepDef, &step); err != nil {
		http.Error(w, fmt.Sprintf("Reusable step YAML is malformed: %v", err), http.StatusBadRequest)
		return
	}

	// Basic validation for a reusable step
	if step.Name == "" {
		http.Error(w, "Validation failed: a reusable step must have a 'name' field in its definition.", http.StatusBadRequest)
		return
	}
	if step.Name != stepName {
		http.Error(w, fmt.Sprintf("Validation failed: the step name in the URL ('%s') must match the 'name' field in the YAML ('%s').", stepName, step.Name), http.StatusBadRequest)
		return
	}

	query := `INSERT INTO reusable_steps (name, definition, updated_at) VALUES ($1, $2, NOW())
			  ON CONFLICT (name) DO UPDATE SET definition = $2, updated_at = NOW()`
	_, err = a.db.Exec(context.Background(), query, stepName, string(stepDef))
	if err != nil {
		log.Error().Err(err).Msg("Failed to save reusable step to database")
		http.Error(w, "Failed to save reusable step", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeleteReusableStep(w http.ResponseWriter, r *http.Request) {
	stepName := r.PathValue("stepName")
	_, err := a.db.Exec(context.Background(), "DELETE FROM reusable_steps WHERE name = $1", stepName)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete reusable step from database")
		http.Error(w, "Failed to delete reusable step", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleFinalizeRun receives the final status directly from the agent.
func (a *App) handleFinalizeRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	var req FinalizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Info().Str("run_id", runID).Str("status", req.Status).Msg("Received final status from agent")

	finalStatus := req.Status
	var failedStep, failedTask string
	if finalStatus != "success" {
		finalStatus = "failure" // Normalize status
		// This query now finds the first task that is not in a successful or pending state.
		err := a.db.QueryRow(context.Background(), "SELECT step_name, task_name FROM tasks WHERE run_id = $1 AND status NOT IN ('completed', 'pending', 'skipped', 'failed (ignored)', 'started') ORDER BY finished_at ASC, started_at ASC LIMIT 1", runID).Scan(&failedStep, &failedTask)
		if err != nil {
			log.Warn().Err(err).Str("run_id", runID).Msg("Could not determine the exact failed task for final status notification.")
		}
	}

	var gitContext = make(map[string]string)
	var repoOwner, repoName, commitSHA sql.NullString
	var checkRunID sql.NullInt64
	query := `SELECT git_repo_owner, git_repo_name, git_commit_sha, git_check_run_id FROM runs WHERE run_id = $1`
	err := a.db.QueryRow(context.Background(), query, runID).Scan(&repoOwner, &repoName, &commitSHA, &checkRunID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to retrieve git context for final notification")
	} else {
		if repoOwner.Valid {
			gitContext["repo_owner"] = repoOwner.String
		}
		if repoName.Valid {
			gitContext["repo_name"] = repoName.String
		}
		if commitSHA.Valid {
			gitContext["commit_sha"] = commitSHA.String
		}
		if checkRunID.Valid {
			gitContext["check_run_id"] = strconv.FormatInt(checkRunID.Int64, 10)
		}
	}

	_, err = a.db.Exec(context.Background(), "UPDATE runs SET status = $1, finished_at = NOW() WHERE run_id = $2 AND finished_at IS NULL", finalStatus, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to update final run status in DB from agent notification")
	}

	if gitContext["repo_owner"] != "" {
		a.notifyGitBotOfFinalStatus(finalStatus, failedStep, failedTask, "", gitContext)
	}

	w.WriteHeader(http.StatusOK)
}

func (a *App) handleGetRunStatus(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	var status string
	err := a.db.QueryRow(context.Background(), "SELECT status FROM runs WHERE run_id = $1", runID).Scan(&status)
	if err != nil {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": status})
}

func (a *App) launchAgent(runID string, pipeline models.Pipeline, pipelineDef []byte, timeout time.Duration, gitContext map[string]string, parentHistory string, environment string) {
	ctx := context.Background()

	secrets, err := a.prepareSecretsForPipeline(pipeline, gitContext, environment)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to prepare secrets for pipeline")
		a.db.Exec(context.Background(), "UPDATE runs SET status = $1, finished_at = NOW() WHERE run_id = $2", "failed", runID)
		if gitContext["repo_owner"] != "" {
			a.notifyGitBotOfFinalStatus("failure", "", "", err.Error(), gitContext)
		}
		return
	}

	// --- MODIFICATION START ---
	// Prepare the final environment variables for the pipeline.
	finalEnv, err := a.prepareEnvironmentForPipeline(pipeline, gitContext, environment)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to prepare environment variables for pipeline")
		a.db.Exec(context.Background(), "UPDATE runs SET status = $1, finished_at = NOW() WHERE run_id = $2", "failed", runID)
		if gitContext["repo_owner"] != "" {
			a.notifyGitBotOfFinalStatus("failure", "", "", err.Error(), gitContext)
		}
		return
	}
	// --- MODIFICATION END ---

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Error creating Docker client")
		return
	}
	defer cli.Close()

	agentImageName := a.cfg.AgentImage
	if agentImageName == "" {
		agentImageName = "nopsai-agent:latest"
	}

	if err := ensureImageExists(ctx, cli, agentImageName); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to ensure agent image exists")
		return
	}

	sharedVolumeName := fmt.Sprintf("vol-%s", runID)
	_, err = cli.VolumeCreate(context.Background(), volume.CreateOptions{Name: sharedVolumeName})
	if err != nil {
		log.Error().Err(err).Msg("Failed to create shared volume")
		return
	}
	defer cli.VolumeRemove(context.Background(), sharedVolumeName, true)

	repoName := gitContext["repo_name"]
	sanitizedPipelineName := sanitizeInput(pipeline.Name)
	shortRunID := runID[:8]

	var agentContainerName string
	if repoName != "" {
		sanitizedRepoName := sanitizeInput(repoName)
		agentContainerName = fmt.Sprintf("agent-%s-%s-%s", sanitizedRepoName, sanitizedPipelineName, shortRunID)
	} else {
		agentContainerName = fmt.Sprintf("agent-%s-%s", sanitizedPipelineName, shortRunID)
	}

	secretsJSON, err := json.Marshal(secrets)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to marshal secrets")
		a.db.Exec(context.Background(), "UPDATE runs SET status = $1, finished_at = NOW() WHERE run_id = $2", "failed", runID)
		return
	}

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
		fmt.Sprintf("NOPSAI_SECRETS=%s", base64.StdEncoding.EncodeToString(secretsJSON)),
	}
	if parentHistory != "" {
		envVars = append(envVars, fmt.Sprintf("PARENT_EXECUTION_HISTORY=%s", parentHistory))
	}
	if environment != "" {
		envVars = append(envVars, fmt.Sprintf("ENVIRONMENT=%s", environment))
	}
	// --- MODIFICATION START ---
	// Add the fully resolved environment variables.
	for key, value := range finalEnv {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
	}
	// --- MODIFICATION END ---
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

	resp, err := cli.ContainerCreate(ctx, &container.Config{
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
			a.db.Exec(context.Background(), "UPDATE runs SET status = 'failed', finished_at = NOW() WHERE run_id = $1 AND finished_at IS NULL", runID)
		}
	case status := <-statusCh:
		log.Info().Str("run_id", runID).Int64("status_code", status.StatusCode).Msg("Agent container finished.")
		finalStatus := "success"
		if status.StatusCode != 0 {
			finalStatus = "failure"
		}
		a.db.Exec(context.Background(), "UPDATE runs SET status = $1, finished_at = NOW() WHERE run_id = $2 AND finished_at IS NULL", finalStatus, runID)
	}
}

func (a *App) notifyGitBotOfFinalStatus(status, failedStep, failedTask, summary string, gitContext map[string]string) {
	checkRunID, _ := strconv.ParseInt(gitContext["check_run_id"], 10, 64)
	gitBotURL := fmt.Sprintf("%s/v1/run/status", a.cfg.NopsaiGitBotAPIURL)
	payload := map[string]interface{}{
		"status":       status,
		"failed_step":  failedStep,
		"failed_task":  failedTask,
		"check_run_id": checkRunID,
		"repo_owner":   gitContext["repo_owner"],
		"repo_name":    gitContext["repo_name"],
		"commit_sha":   gitContext["commit_sha"],
		"summary":      summary,
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

func (a *App) notifyGitBotOfTaskStatus(runID, stepName, taskName, taskStatus string) {
	var repoOwner, repoName, commitSHA, pipelineDef sql.NullString
	var checkRunID sql.NullInt64
	var taskIndex, totalTasks int
	var startedAt, finishedAt sql.NullTime

	query := `
		SELECT
			r.git_repo_owner, r.git_repo_name, r.git_commit_sha, r.git_check_run_id, r.pipeline_definition,
			t.task_index, (SELECT COUNT(*) FROM tasks WHERE run_id = r.run_id),
			t.started_at, t.finished_at
		FROM runs r JOIN tasks t ON r.run_id = t.run_id
		WHERE r.run_id = $1 AND t.step_name = $2 AND t.task_name = $3`

	err := a.db.QueryRow(context.Background(), query, runID, stepName, taskName).Scan(&repoOwner, &repoName, &commitSHA, &checkRunID, &pipelineDef, &taskIndex, &totalTasks, &startedAt, &finishedAt)
	if err != nil || !repoOwner.Valid || !checkRunID.Valid {
		log.Warn().Str("run_id", runID).Err(err).Msg("Not a Git-triggered run with a check ID, skipping task status update.")
		return
	}

	var pipeline models.Pipeline
	var dependsOn []string
	if pipelineDef.Valid {
		if err := yaml.Unmarshal([]byte(pipelineDef.String), &pipeline); err == nil {
			for _, step := range pipeline.Steps {
				if step.Name == stepName {
					for _, task := range step.Tasks {
						if task.Name == taskName {
							dependsOn = task.DependsOn
							break
						}
					}
					break
				}
			}
		}
	}

	gitBotURL := fmt.Sprintf("%s/v1/task/status", a.cfg.NopsaiGitBotAPIURL)
	payload := map[string]interface{}{
		"run_id":       runID,
		"repo_owner":   repoOwner.String,
		"repo_name":    repoName.String,
		"check_run_id": checkRunID.Int64,
		"commit_sha":   commitSHA.String,
		"step_name":    stepName,
		"task_name":    taskName,
		"task_status":  taskStatus,
		"task_index":   taskIndex,
		"total_tasks":  totalTasks,
		"depends_on":   dependsOn,
		"started_at":   startedAt.Time,
		"finished_at":  finishedAt.Time,
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(gitBotURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Error().Err(err).Msg("Failed to notify git-bot of task status")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status_code", resp.StatusCode).Msg("Received non-OK status from git-bot for task update")
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

	if cfg.MasterKey == "" {
		log.Fatal().Msg("NOPSAI_MASTER_KEY environment variable is not set. This is required for secret encryption.")
	}
	key := sha256.Sum256([]byte(cfg.MasterKey))

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

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create Docker client")
	}
	defer cli.Close()

	app := &App{db: dbpool, cfg: cfg, cli: cli, encKey: key[:]}

	mux := http.NewServeMux()

	// Pipeline Management
	mux.HandleFunc("GET /v1/pipelines", app.handleListPipelines)
	mux.HandleFunc("GET /v1/pipelines/{pipelineName}", app.handleGetPipeline)
	mux.HandleFunc("GET /v1/runs/{runID}/status", app.handleGetRunStatus)
	mux.HandleFunc("PUT /v1/pipelines/{pipelineName}", app.handleCreateOrUpdatePipeline)
	mux.HandleFunc("DELETE /v1/pipelines/{pipelineName}", app.handleDeletePipeline)

	// Reusable Step Management
	mux.HandleFunc("GET /v1/steps", app.handleListReusableSteps)
	mux.HandleFunc("GET /v1/steps/{stepName}", app.handleGetReusableStep)
	mux.HandleFunc("PUT /v1/steps/{stepName}", app.handleCreateOrUpdateReusableStep)
	mux.HandleFunc("DELETE /v1/steps/{stepName}", app.handleDeleteReusableStep)

	// Trigger Override Management
	mux.HandleFunc("GET /v1/overrides", app.handleListTriggerOverrides)
	mux.HandleFunc("GET /v1/overrides/{repoOwner}/{repoName}", app.handleGetTriggerOverride)
	mux.HandleFunc("PUT /v1/overrides/{repoOwner}/{repoName}", app.handleCreateOrUpdateTriggerOverride)
	mux.HandleFunc("DELETE /v1/overrides/{repoOwner}/{repoName}", app.handleDeleteTriggerOverride)

	// General Secret Management
	mux.HandleFunc("GET /v1/secrets", app.handleListGeneralSecrets)
	mux.HandleFunc("PUT /v1/secrets/{secretName}", app.handleCreateOrUpdateGeneralSecret)
	mux.HandleFunc("DELETE /v1/secrets/{secretName}", app.handleDeleteGeneralSecret)

	// Repository-level Secret Management
	mux.HandleFunc("GET /v1/repositories/{repoOwner}/{repoName}/secrets", app.handleListRepoSecrets)
	mux.HandleFunc("PUT /v1/repositories/{repoOwner}/{repoName}/secrets/{secretName}", app.handleCreateOrUpdateRepoSecret)
	mux.HandleFunc("DELETE /v1/repositories/{repoOwner}/{repoName}/secrets/{secretName}", app.handleDeleteRepoSecret)

	// General Environment Variable Management
	mux.HandleFunc("GET /v1/environments", app.handleListGeneralEnvs)
	mux.HandleFunc("PUT /v1/environments/{envName}", app.handleCreateOrUpdateGeneralEnv)
	mux.HandleFunc("DELETE /v1/environments/{envName}", app.handleDeleteGeneralEnv)

	// Repository-level Environment Variable Management
	mux.HandleFunc("GET /v1/repositories/{repoOwner}/{repoName}/environments", app.handleListRepoEnvs)
	mux.HandleFunc("PUT /v1/repositories/{repoOwner}/{repoName}/environments/{envName}", app.handleCreateOrUpdateRepoEnv)
	mux.HandleFunc("DELETE /v1/repositories/{repoOwner}/{repoName}/environments/{envName}", app.handleDeleteRepoEnv)

	// Pipeline Execution
	mux.HandleFunc("POST /v1/run", app.handleRunPipeline)
	mux.HandleFunc("POST /v1/run/{pipelineName}", app.handleRunPipeline)
	mux.HandleFunc("GET /v1/runs", app.handleListRuns)              // New endpoint for UI
	mux.HandleFunc("GET /v1/runs/{runID}", app.handleGetRunDetails) // New endpoint for UI
	mux.HandleFunc("POST /v1/runs/{runID}/rerun", app.handleRerunPipeline)
	mux.HandleFunc("POST /v1/runs/{runID}/finalize", app.handleFinalizeRun)
	mux.HandleFunc("POST /v1/runs/{runID}/steps/{stepName}/tasks/{taskName}", app.handleTaskUpdate)

	server := &http.Server{
		Addr:    cfg.NopsaiListenAddress,
		Handler: corsMiddleware(mux), // Important: Wrap mux with CORS middleware
	}

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

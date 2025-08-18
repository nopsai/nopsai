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

type App struct {
	db     *pgxpool.Pool
	cfg    *config.Config
	cli    *client.Client
	encKey []byte
}

type StepStatusUpdate struct {
	Status   string `json:"status"`
	ExitCode int    `json:"exit_code"`
}

type SecretRequest struct {
	Value string `json:"value"`
}

type PipelineRequest struct {
	Definition string `json:"definition"`
}

type TriggerOverrideRequest struct {
	TriggerDefinition string `json:"trigger_definition"`
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

func validatePipeline(pipeline *models.Pipeline) error {
	if pipeline.Name == "" {
		return fmt.Errorf("'name' is a required field")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`).MatchString(pipeline.Name) {
		return fmt.Errorf("pipeline name can only contain alphanumeric characters, underscores, dots, and hyphens")
	}
	if pipeline.ContainerImage == "" {
		return fmt.Errorf("'container_image' is a required field")
	}
	if len(pipeline.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}

	stepNames := make(map[string]struct{})
	for _, step := range pipeline.Steps {
		if step.Name == "" {
			return fmt.Errorf("a step is missing its required 'name' field")
		}
		stepNames[step.Name] = struct{}{}
	}

	for _, step := range pipeline.Steps {
		for _, depName := range step.DependsOn {
			if _, exists := stepNames[depName]; !exists {
				return fmt.Errorf("step '%s' has an undefined dependency: '%s'", step.Name, depName)
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

func (a *App) handleListGeneralSecrets(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(context.Background(), "SELECT name FROM secrets WHERE repository_name IS NULL ORDER BY name ASC")
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

	query := `INSERT INTO secrets (name, value, repository_name, updated_at) VALUES ($1, $2, NULL, NOW())
			  ON CONFLICT (name, repository_name) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`
	_, err = a.db.Exec(context.Background(), query, secretName, encryptedValue)
	if err != nil {
		log.Error().Err(err).Msg("Failed to save general secret to database")
		http.Error(w, "Failed to save secret", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeleteGeneralSecret(w http.ResponseWriter, r *http.Request) {
	secretName := r.PathValue("secretName")
	_, err := a.db.Exec(context.Background(), "DELETE FROM secrets WHERE name = $1 AND repository_name IS NULL", secretName)
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
	rows, err := a.db.Query(context.Background(), "SELECT name FROM secrets WHERE repository_name = $1 ORDER BY name ASC", fullName)
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

	query := `INSERT INTO secrets (name, value, repository_name, updated_at) VALUES ($1, $2, $3, NOW())
			  ON CONFLICT (name, repository_name) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`
	_, err = a.db.Exec(context.Background(), query, secretName, encryptedValue, fullName)
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
	_, err := a.db.Exec(context.Background(), "DELETE FROM secrets WHERE name = $1 AND repository_name = $2", secretName, fullName)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete repo secret from database")
		http.Error(w, "Failed to delete secret", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

func (a *App) launchAndRunPipeline(
	w http.ResponseWriter,
	pipeline models.Pipeline,
	pipelineDef []byte,
	gitContext map[string]string,
	timeoutDuration time.Duration,
) {
	runID := uuid.New()
	log.Info().Str("run_id", runID.String()).Msgf("Received pipeline: %s", pipeline.Name)

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

	_, err = tx.Exec(context.Background(),
		`INSERT INTO runs (run_id, pipeline_name, status, started_at, timeout_at, pipeline_definition,
			git_repo_owner, git_repo_name, git_clone_url, git_ssh_url, git_ref,
			git_commit_sha, git_commit_url, git_commit_message, git_commit_author_name,
			git_commit_author_email, git_commit_author_username, git_pusher_name,
			git_pusher_email, git_check_run_id)
			VALUES ($1, $2, $3, NOW(), $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
		runID, pipeline.Name, "pending", timeoutAt, string(pipelineDef),
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

	go a.launchAgent(runID.String(), pipeline, pipelineDef, timeoutDuration, gitContext)

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Pipeline run created successfully with ID: " + runID.String()))
}

func (a *App) handleRunPipeline(w http.ResponseWriter, r *http.Request) {
	var pipeline models.Pipeline
	var pipelineDef []byte
	var err error

	pipelineNameFromPath := r.PathValue("pipelineName")

	if pipelineNameFromPath != "" {
		// --- Path 1: Run by name (Override) ---
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
		// Improve the name for clarity in logs
		pipeline.Name = fmt.Sprintf("%s-overridden", pipeline.Name)

	} else {
		// --- Path 2: Run from request body (Repository) ---
		pipelineDef, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusInternalServerError)
			return
		}
		if err = yaml.Unmarshal(pipelineDef, &pipeline); err != nil {
			http.Error(w, fmt.Sprintf("Pipeline YAML is malformed: %v", err), http.StatusBadRequest)
			return
		}
		pipeline.Name = sanitizeInput(pipeline.Name)
	}

	// --- Common Logic for both paths ---
	if err := validatePipeline(&pipeline); err != nil {
		http.Error(w, fmt.Sprintf("Pipeline validation failed: %v", err), http.StatusBadRequest)
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

	timeoutStr := pipeline.Timeout
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

	a.launchAndRunPipeline(w, pipeline, pipelineDef, gitContext, timeoutDuration)
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

	a.launchAndRunPipeline(w, pipeline, []byte(pipelineDef.String), gitContext, timeoutDuration)
}

func (a *App) handleStepUpdate(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	stepName := r.PathValue("stepName")

	var update StepStatusUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var query string
	if update.Status == "started" {
		query = "UPDATE steps SET status = 'started', started_at = NOW() WHERE run_id = $1 AND name = $2"
		_, err := a.db.Exec(context.Background(), query, runID, stepName)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Str("step", stepName).Msg("Failed to update step start time")
			http.Error(w, "Failed to update step status", http.StatusInternalServerError)
			return
		}
	} else {
		query = "UPDATE steps SET status = $1, exit_code = $2, finished_at = NOW() WHERE run_id = $3 AND name = $4"
		_, err := a.db.Exec(context.Background(), query, update.Status, update.ExitCode, runID, stepName)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Str("step", stepName).Msg("Failed to update step finish status")
			http.Error(w, "Failed to update step status", http.StatusInternalServerError)
			return
		}
	}

	log.Info().Str("run_id", runID).Str("step", stepName).Str("status", update.Status).Msg("Updated step status")

	go a.notifyGitBotOfStepStatus(runID, stepName, update.Status)

	w.WriteHeader(http.StatusOK)
}

func (a *App) launchAgent(runID string, pipeline models.Pipeline, pipelineDef []byte, timeout time.Duration, gitContext map[string]string) {
	ctx := context.Background()

	secrets, err := a.prepareSecretsForPipeline(pipeline, gitContext)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to prepare secrets for pipeline")
		a.db.Exec(context.Background(), "UPDATE runs SET status = $1, finished_at = NOW() WHERE run_id = $2", "failed", runID)
		if gitContext["repo_owner"] != "" {
			a.notifyGitBotOfFinalStatus("failure", "secret_preparation", err.Error(), gitContext)
		}
		return
	}

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

	agentContainerName := fmt.Sprintf("agent-%s-%s", sanitizeInput(pipeline.Name), runID)

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
		}
	case status := <-statusCh:
		log.Info().Str("run_id", runID).Int64("status_code", status.StatusCode).Msg("Agent container finished.")
		finalStatus := "success"
		var failedStep string
		if status.StatusCode != 0 {
			finalStatus = "failure"
			err := a.db.QueryRow(context.Background(), "SELECT name FROM steps WHERE run_id = $1 AND status = 'failed' ORDER BY finished_at ASC LIMIT 1", runID).Scan(&failedStep)
			if err != nil {
				log.Warn().Err(err).Str("run_id", runID).Msg("Could not determine the exact failed step.")
			}
		}
		a.db.Exec(context.Background(), "UPDATE runs SET status = $1, finished_at = NOW() WHERE run_id = $2", finalStatus, runID)
		if gitContext["repo_owner"] != "" {
			a.notifyGitBotOfFinalStatus(finalStatus, failedStep, "", gitContext)
		}
	}
}

func (a *App) prepareSecretsForPipeline(pipeline models.Pipeline, gitContext map[string]string) (map[string]string, error) {
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

	// 1. Fetch General Secrets First
	for secretName := range requiredSecrets {
		var encryptedValue string
		err := a.db.QueryRow(context.Background(), "SELECT value FROM secrets WHERE name = $1 AND repository_name IS NULL", secretName).Scan(&encryptedValue)
		if err == nil {
			decryptedValue, err := a.decrypt(encryptedValue)
			if err == nil {
				finalSecrets[secretName] = decryptedValue
			} else {
				log.Error().Err(err).Str("secret_name", secretName).Msg("Failed to decrypt general secret, it will be ignored")
			}
		}
	}

	// 2. Fetch Repository-level Secrets and Override if context exists
	if repoOwner, ok := gitContext["repo_owner"]; ok {
		if repoName, ok := gitContext["repo_name"]; ok {
			repoFullName := fmt.Sprintf("%s/%s", repoOwner, repoName)
			for secretName := range requiredSecrets {
				var encryptedValue string
				err := a.db.QueryRow(context.Background(), "SELECT value FROM secrets WHERE name = $1 AND repository_name = $2", secretName, repoFullName).Scan(&encryptedValue)
				if err == nil {
					decryptedValue, err := a.decrypt(encryptedValue)
					if err == nil {
						finalSecrets[secretName] = decryptedValue // This overwrites the general secret
					} else {
						log.Error().Err(err).Str("secret_name", secretName).Str("repo", repoFullName).Msg("Failed to decrypt repo secret, it will be ignored")
					}
				}
			}
		}
	}

	// 3. Final Check: Ensure all required secrets were found
	for secretName := range requiredSecrets {
		if _, ok := finalSecrets[secretName]; !ok {
			return nil, fmt.Errorf("pipeline aborted: required secret '%s' not found in general or repository scope", secretName)
		}
	}

	return finalSecrets, nil
}

func (a *App) notifyGitBotOfFinalStatus(status, failedStep, summary string, gitContext map[string]string) {
	checkRunID, _ := strconv.ParseInt(gitContext["check_run_id"], 10, 64)
	gitBotURL := fmt.Sprintf("%s/v1/run/status", a.cfg.NopsaiGitBotAPIURL)
	payload := map[string]interface{}{
		"status":       status,
		"failed_step":  failedStep,
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

func (a *App) notifyGitBotOfStepStatus(runID, stepName, stepStatus string) {
	var repoOwner, repoName, commitSHA, pipelineDef sql.NullString
	var checkRunID sql.NullInt64
	var stepIndex, totalSteps int
	var startedAt, finishedAt sql.NullTime

	query := `
		SELECT
			r.git_repo_owner, r.git_repo_name, r.git_commit_sha, r.git_check_run_id, r.pipeline_definition,
			s.step_index, (SELECT COUNT(*) FROM steps WHERE run_id = r.run_id),
			s.started_at, s.finished_at
		FROM runs r JOIN steps s ON r.run_id = s.run_id
		WHERE r.run_id = $1 AND s.name = $2`

	err := a.db.QueryRow(context.Background(), query, runID, stepName).Scan(&repoOwner, &repoName, &commitSHA, &checkRunID, &pipelineDef, &stepIndex, &totalSteps, &startedAt, &finishedAt)
	if err != nil || !repoOwner.Valid || !checkRunID.Valid {
		log.Warn().Str("run_id", runID).Msg("Not a Git-triggered run with a check ID, skipping step status update.")
		return
	}

	var pipeline models.Pipeline
	var dependsOn []string
	if pipelineDef.Valid {
		if err := yaml.Unmarshal([]byte(pipelineDef.String), &pipeline); err == nil {
			for _, step := range pipeline.Steps {
				if step.Name == stepName {
					dependsOn = step.DependsOn
					break
				}
			}
		}
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
		"depends_on":   dependsOn,
		"started_at":   startedAt.Time,
		"finished_at":  finishedAt.Time,
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
	mux.HandleFunc("PUT /v1/pipelines/{pipelineName}", app.handleCreateOrUpdatePipeline)
	mux.HandleFunc("DELETE /v1/pipelines/{pipelineName}", app.handleDeletePipeline)

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

	// Pipeline Execution
	mux.HandleFunc("POST /v1/run", app.handleRunPipeline)
	mux.HandleFunc("POST /v1/run/{pipelineName}", app.handleRunPipeline)
	mux.HandleFunc("POST /v1/runs/{runID}/rerun", app.handleRerunPipeline)
	mux.HandleFunc("POST /v1/runs/{runID}/steps/{stepName}", app.handleStepUpdate)

	server := &http.Server{
		Addr:    cfg.NopsaiListenAddress,
		Handler: mux,
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

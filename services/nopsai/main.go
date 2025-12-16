package main

import (
	"bufio"
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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/pkg/proto"

	"github.com/google/go-github/v53/github"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/yaml.v3"

	"nopsai/services/nopsai/pkg/store"
	"nopsai/services/nopsai/pkg/validation"
)

// WebSocket Hub implementation

type App struct {
	db         *pgxpool.Pool
	cfg        *config.Config
	dispatcher proto.DispatcherServiceClient
	encKey     []byte
	httpClient *http.Client
	store      store.Store
	configPath string
	cfgMu      sync.RWMutex

	configSyncMu     sync.Mutex
	configSyncStatus ConfigSyncStatus
	envFilePath      string
}

type LogLine = models.LogLine
type RunListItem = models.RunListItem
type StepConfiguration = models.StepConfiguration
type StepDetail = models.StepDetail
type TaskDetail = models.TaskDetail
type ParentRunInfo = models.ParentRunInfo
type RunDetail = models.RunDetail
type StepStatusUpdate = models.StepStatusUpdate
type SecretRequest = models.SecretRequest
type VariableRequest = models.VariableRequest
type ScopeResponse = models.ScopeResponse
type VariableValueResponse = models.VariableValueResponse
type PipelineRequest = models.PipelineRequest
type TriggerOverrideRequest = models.TriggerOverrideRequest
type FinalizeRequest = models.FinalizeRequest
type Group = models.Group

type ConfigSyncStatus struct {
	Status      string         `json:"status"`
	Message     string         `json:"message,omitempty"`
	Details     map[string]int `json:"details,omitempty"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
}

// Keep these local for now if not in models
type suiteCheckRunResponse struct {
	CheckRunID         int64  `json:"check_run_id"`
	HeadSHA            string `json:"head_sha"`
	PullRequestHeadRef string `json:"pull_request_head_ref,omitempty"`
	HeadBranch         string `json:"head_branch,omitempty"`
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)
var envKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func deriveTriggerEventID(gitContext map[string]string) string {
	if gitContext == nil {
		return ""
	}
	owner := strings.ToLower(strings.TrimSpace(gitContext["repo_owner"]))
	name := strings.ToLower(strings.TrimSpace(gitContext["repo_name"]))
	ref := strings.ToLower(strings.TrimSpace(gitContext["ref"]))
	sha := strings.ToLower(strings.TrimSpace(gitContext["commit_sha"]))
	if owner == "" && name == "" && ref == "" && sha == "" {
		return ""
	}
	return fmt.Sprintf("%s|%s|%s|%s", owner, name, ref, sha)
}

func (a *App) getConfigSnapshot() config.Config {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return *a.cfg
}

func (a *App) getConfigRepoURL() string {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return strings.TrimSpace(a.cfg.ConfigRepoURL)
}

func (a *App) getAutoRemovalAgentContainer() bool {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return a.cfg.AutoRemovalAgentContainer
}

func (a *App) getDefaultPipelineTimeout() string {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return a.cfg.DefaultPipelineTimeout
}

func (a *App) getAgentImage() string {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return strings.TrimSpace(a.cfg.AgentImage)
}

func (a *App) getDockerNetworkName() string {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return strings.TrimSpace(a.cfg.DockerNetworkName)
}

func (a *App) getLLMAgentTimeout() string {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return strings.TrimSpace(a.cfg.LLMAgentTimeout)
}

func (a *App) setConfigSyncStatus(status ConfigSyncStatus) {
	a.configSyncMu.Lock()
	defer a.configSyncMu.Unlock()
	statusCopy := status
	if status.Details != nil {
		detailsCopy := make(map[string]int, len(status.Details))
		for k, v := range status.Details {
			detailsCopy[k] = v
		}
		statusCopy.Details = detailsCopy
	}
	a.configSyncStatus = statusCopy
}

func (a *App) getConfigSyncStatus() ConfigSyncStatus {
	a.configSyncMu.Lock()
	defer a.configSyncMu.Unlock()
	statusCopy := a.configSyncStatus
	if statusCopy.Details != nil {
		detailsCopy := make(map[string]int, len(statusCopy.Details))
		for k, v := range statusCopy.Details {
			detailsCopy[k] = v
		}
		statusCopy.Details = detailsCopy
	}
	return statusCopy
}

func (a *App) isConfigSyncRunning() bool {
	return strings.EqualFold(a.getConfigSyncStatus().Status, "running")
}

// This new helper function fetches and builds a RunListItem for a given run ID.
func (a *App) getRunListItem(runID string) (*RunListItem, error) {
	return a.store.GetRunListItem(context.Background(), runID)
}

// The broadcast function is updated to send a more specific 'run_summary_update' message
// with the full RunListItem as the payload.

func matchBranchPattern(pattern, name string) bool {
	if pattern == "" {
		return false
	}
	var builder strings.Builder
	builder.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				builder.WriteString(".*")
				i++
			} else {
				builder.WriteString("[^/]*")
			}
		case '?':
			builder.WriteString(".")
		case '.', '(', ')', '+', '|', '^', '$', '{', '}', '[', ']', '\\':
			builder.WriteByte('\\')
			builder.WriteByte(pattern[i])
		default:
			builder.WriteByte(pattern[i])
		}
	}
	builder.WriteString("$")
	re, err := regexp.Compile(builder.String())
	if err != nil {
		return pattern == name
	}
	return re.MatchString(name)
}

var (
	errManifestNotFound = errors.New("manifest not found")
	errPipelineNotFound = errors.New("pipeline not found")
)

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
	return validation.ValidatePipeline(pipeline)
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

func (a *App) handleListGeneralVariables(w http.ResponseWriter, r *http.Request) {
	scope := r.URL.Query().Get("env")
	includeSource := strings.EqualFold(r.URL.Query().Get("include_source"), "true")
	var rows pgx.Rows
	var err error

	queryGeneral := "SELECT name, COALESCE(source, 'database'), created_at, updated_at FROM variables WHERE repository_name IS NULL AND %s ORDER BY name ASC"
	queryRepo := "SELECT repository_name, name, COALESCE(source, 'database'), created_at, updated_at FROM variables WHERE repository_name IS NOT NULL AND %s ORDER BY repository_name ASC, name ASC"

	ctx := context.Background()
	condition := "scope IS NULL"
	args := []interface{}{}
	if scope != "" {
		condition = "scope = $1"
		args = append(args, scope)
	}

	rows, err = a.db.Query(ctx, fmt.Sprintf(queryGeneral, condition), args...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query general variables from database")
		http.Error(w, "Failed to retrieve variables", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type variableListItem struct {
		Name      string `json:"name"`
		Source    string `json:"source"`
		CreatedAt string `json:"created_at,omitempty"`
		UpdatedAt string `json:"updated_at,omitempty"`
	}

	nameSet := make(map[string]struct{})
	var names []string
	var items []variableListItem

	addEntry := func(name, source string, createdAt, updatedAt time.Time) {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return
		}
		if _, exists := nameSet[trimmed]; exists {
			return
		}
		nameSet[trimmed] = struct{}{}
		if includeSource {
			items = append(items, variableListItem{
				Name:      trimmed,
				Source:    normalizeVariableSourceKey(source),
				CreatedAt: createdAt.Format(time.RFC3339),
				UpdatedAt: updatedAt.Format(time.RFC3339),
			})
		} else {
			names = append(names, trimmed)
		}
	}

	for rows.Next() {
		var name, source string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&name, &source, &createdAt, &updatedAt); err != nil {
			log.Error().Err(err).Msg("Failed to scan variable name")
			http.Error(w, "Failed to process variables", http.StatusInternalServerError)
			return
		}
		addEntry(name, source, createdAt, updatedAt)
	}

	rows.Close()
	repoCondition := "scope IS NULL"
	repoArgs := []interface{}{}
	if scope != "" {
		repoCondition = "scope = $1"
		repoArgs = append(repoArgs, scope)
	}

	rows, err = a.db.Query(ctx, fmt.Sprintf(queryRepo, repoCondition), repoArgs...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query repository variables from database")
		http.Error(w, "Failed to retrieve variables", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var repoName, varName, source string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&repoName, &varName, &source, &createdAt, &updatedAt); err != nil {
			log.Error().Err(err).Msg("Failed to scan repository variable name")
			http.Error(w, "Failed to process variables", http.StatusInternalServerError)
			return
		}
		repo := strings.TrimSpace(repoName)
		name := strings.TrimSpace(varName)
		if repo == "" || name == "" {
			continue
		}
		addEntry(repo+"/"+name, source, createdAt, updatedAt)
	}

	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if includeSource {
		json.NewEncoder(w).Encode(items)
		return
	}
	json.NewEncoder(w).Encode(names)
}

func (a *App) handleListVariableScopes(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	rows, err := a.db.Query(ctx, "SELECT DISTINCT scope FROM variables WHERE repository_name IS NULL")
	if err != nil {
		log.Error().Err(err).Msg("Failed to query scope list from database")
		http.Error(w, "Failed to retrieve scopes", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	scopeSet := make(map[string]struct{})
	scopeSet[""] = struct{}{}

	for rows.Next() {
		var env sql.NullString
		if err := rows.Scan(&env); err != nil {
			log.Error().Err(err).Msg("Failed to scan scope name")
			http.Error(w, "Failed to process scopes", http.StatusInternalServerError)
			return
		}
		value := ""
		if env.Valid {
			value = strings.TrimSpace(env.String)
		}
		scopeSet[value] = struct{}{}
	}

	if err := rows.Err(); err != nil {
		log.Error().Err(err).Msg("Failed during scope iteration")
		http.Error(w, "Failed to process scopes", http.StatusInternalServerError)
		return
	}

	scopes := make([]string, 0, len(scopeSet))
	for value := range scopeSet {
		scopes = append(scopes, value)
	}

	sort.Slice(scopes, func(i, j int) bool {
		ai, aj := scopes[i], scopes[j]
		if ai == "" && aj == "" {
			return false
		}
		if ai == "" {
			return true
		}
		if aj == "" {
			return false
		}
		return strings.ToLower(ai) < strings.ToLower(aj)
	})

	result := make([]ScopeResponse, 0, len(scopes))
	for _, value := range scopes {
		result = append(result, ScopeResponse{Scope: value})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Error().Err(err).Msg("Failed to encode scope response")
	}
}

func (a *App) handleCreateOrUpdateGeneralVariable(w http.ResponseWriter, r *http.Request) {
	variableName := r.PathValue("variableName")
	scope := r.URL.Query().Get("env")
	var req VariableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var err error
	if scope != "" {
		query := `INSERT INTO variables (name, value, repository_name, scope, source, updated_at) VALUES ($1, $2, NULL, $3, 'database', NOW())
				  ON CONFLICT (name, repository_name, scope) DO UPDATE SET value = EXCLUDED.value, source = 'database', updated_at = NOW()`
		_, err = a.db.Exec(context.Background(), query, variableName, req.Value, scope)
	} else {
		query := `INSERT INTO variables (name, value, repository_name, scope, source, updated_at) VALUES ($1, $2, NULL, NULL, 'database', NOW())
				  ON CONFLICT (name, repository_name, scope) DO UPDATE SET value = EXCLUDED.value, source = 'database', updated_at = NOW()`
		_, err = a.db.Exec(context.Background(), query, variableName, req.Value)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to save variable to database")
		http.Error(w, "Failed to save variable", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeleteGeneralVariable(w http.ResponseWriter, r *http.Request) {
	variableName := r.PathValue("variableName")
	scope := r.URL.Query().Get("env")
	var err error

	if scope != "" {
		_, err = a.db.Exec(context.Background(), "DELETE FROM variables WHERE name = $1 AND repository_name IS NULL AND scope = $2", variableName, scope)
	} else {
		_, err = a.db.Exec(context.Background(), "DELETE FROM variables WHERE name = $1 AND repository_name IS NULL AND scope IS NULL", variableName)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to delete variable from database")
		http.Error(w, "Failed to delete variable", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleGetGeneralVariableValue(w http.ResponseWriter, r *http.Request) {
	variableName := r.PathValue("variableName")
	scope := r.URL.Query().Get("env")
	var value string
	var err error

	if scope != "" {
		err = a.db.QueryRow(context.Background(), "SELECT value FROM variables WHERE name = $1 AND repository_name IS NULL AND scope = $2", variableName, scope).Scan(&value)
	} else {
		err = a.db.QueryRow(context.Background(), "SELECT value FROM variables WHERE name = $1 AND repository_name IS NULL AND scope IS NULL", variableName).Scan(&value)
	}

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Variable not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Msg("Failed to fetch variable value")
		http.Error(w, "Failed to retrieve variable", http.StatusInternalServerError)
		return
	}

	response := VariableValueResponse{Name: variableName, Value: value}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (a *App) handleListRepoVariables(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)
	scope := r.URL.Query().Get("env")
	var rows pgx.Rows
	var err error

	if scope != "" {
		rows, err = a.db.Query(context.Background(), "SELECT name FROM variables WHERE repository_name = $1 AND scope = $2 ORDER BY name ASC", fullName, scope)
	} else {
		rows, err = a.db.Query(context.Background(), "SELECT name FROM variables WHERE repository_name = $1 AND scope IS NULL ORDER BY name ASC", fullName)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to query repository variables from database")
		http.Error(w, "Failed to retrieve variables", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Error().Err(err).Msg("Failed to scan variable name")
			http.Error(w, "Failed to process variables", http.StatusInternalServerError)
			return
		}
		names = append(names, name)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(names)
}

func (a *App) handleCreateOrUpdateRepoVariable(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)
	variableName := r.PathValue("variableName")
	scope := r.URL.Query().Get("env")
	var req VariableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var err error
	if scope != "" {
		query := `INSERT INTO variables (name, value, repository_name, scope, source, updated_at) VALUES ($1, $2, $3, $4, 'database', NOW())
				  ON CONFLICT (name, repository_name, scope) DO UPDATE SET value = EXCLUDED.value, source = 'database', updated_at = NOW()`
		_, err = a.db.Exec(context.Background(), query, variableName, req.Value, fullName, scope)
	} else {
		query := `INSERT INTO variables (name, value, repository_name, scope, source, updated_at) VALUES ($1, $2, $3, NULL, 'database', NOW())
				  ON CONFLICT (name, repository_name, scope) DO UPDATE SET value = EXCLUDED.value, source = 'database', updated_at = NOW()`
		_, err = a.db.Exec(context.Background(), query, variableName, req.Value, fullName)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to save repository variable to database")
		http.Error(w, "Failed to save variable", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeleteRepoVariable(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)
	variableName := r.PathValue("variableName")
	scope := r.URL.Query().Get("env")
	var err error

	if scope != "" {
		_, err = a.db.Exec(context.Background(), "DELETE FROM variables WHERE name = $1 AND repository_name = $2 AND scope = $3", variableName, fullName, scope)
	} else {
		_, err = a.db.Exec(context.Background(), "DELETE FROM variables WHERE name = $1 AND repository_name = $2 AND scope IS NULL", variableName, fullName)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to delete repository variable from database")
		http.Error(w, "Failed to delete variable", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleGetRepoVariableValue(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)
	variableName := r.PathValue("variableName")
	scope := r.URL.Query().Get("env")
	var value string
	var err error

	if scope != "" {
		err = a.db.QueryRow(context.Background(), "SELECT value FROM variables WHERE name = $1 AND repository_name = $2 AND scope = $3", variableName, fullName, scope).Scan(&value)
	} else {
		err = a.db.QueryRow(context.Background(), "SELECT value FROM variables WHERE name = $1 AND repository_name = $2 AND scope IS NULL", variableName, fullName).Scan(&value)
	}

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Variable not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Msg("Failed to fetch repository variable value")
		http.Error(w, "Failed to retrieve variable", http.StatusInternalServerError)
		return
	}

	response := VariableValueResponse{Name: variableName, Value: value}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (a *App) prepareVariablesForPipeline(pipeline models.Pipeline, gitContext map[string]string, scope string, overrides map[string]string) (map[string]string, error) {
	finalVars := make(map[string]string)
	cleanOverrides := make(map[string]string)
	for key, value := range overrides {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		cleanOverrides[trimmedKey] = value
	}

	repoFullName := fmt.Sprintf("%s/%s", gitContext["repo_owner"], gitContext["repo_name"])

	// The 'variables' block in the YAML is a list of required variable names.
	requiredVars := pipeline.Variables

	for _, rawName := range requiredVars {
		varName := strings.TrimSpace(rawName)
		if varName == "" {
			continue
		}

		// Allow ad-hoc overrides to satisfy or replace scoped values.
		if val, ok := cleanOverrides[varName]; ok {
			finalVars[varName] = val
			continue
		}

		var value string
		var err error
		found := false

		if scope != "" {
			// Precedence: 1. Repo/Env -> 2. General/Env
			err = a.db.QueryRow(context.Background(), "SELECT value FROM variables WHERE name = $1 AND repository_name = $2 AND scope = $3", varName, repoFullName, scope).Scan(&value)
			if err == nil {
				found = true
			}
			if !found {
				err = a.db.QueryRow(context.Background(), "SELECT value FROM variables WHERE name = $1 AND repository_name IS NULL AND scope = $2", varName, scope).Scan(&value)
				if err == nil {
					found = true
				}
			}

			if !found {
				return nil, fmt.Errorf("pipeline aborted: required scope variable '%s' not found for scope '%s'", varName, scope)
			}

		} else {
			// Precedence: 1. Repo/No-Env -> 2. General/No-Env
			err = a.db.QueryRow(context.Background(), "SELECT value FROM variables WHERE name = $1 AND repository_name = $2 AND scope IS NULL", varName, repoFullName).Scan(&value)
			if err == nil {
				found = true
			}
			if !found {
				err = a.db.QueryRow(context.Background(), "SELECT value FROM variables WHERE name = $1 AND repository_name IS NULL AND scope IS NULL", varName).Scan(&value)
				if err == nil {
					found = true
				}
			}
		}

		if !found {
			return nil, fmt.Errorf("pipeline aborted: required scope variable '%s' not found in the default scope", varName)
		}

		finalVars[varName] = value
	}

	// Append any ad-hoc overrides that are not declared as required variables.
	for key, value := range cleanOverrides {
		if _, exists := finalVars[key]; !exists {
			finalVars[key] = value
		}
	}

	return finalVars, nil
}

func (a *App) handleListGeneralSecrets(w http.ResponseWriter, r *http.Request) {
	env := r.URL.Query().Get("env")
	includeSource := strings.EqualFold(r.URL.Query().Get("include_source"), "true")
	ctx := context.Background()

	type secretListItem struct {
		Name      string `json:"name"`
		Source    string `json:"source"`
		CreatedAt string `json:"created_at,omitempty"`
		UpdatedAt string `json:"updated_at,omitempty"`
	}

	const defaultSecretSource = "database"
	nameSet := make(map[string]struct{})
	var names []string
	var items []secretListItem

	addEntry := func(name string, createdAt, updatedAt time.Time) {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return
		}
		if _, exists := nameSet[trimmed]; exists {
			return
		}
		nameSet[trimmed] = struct{}{}
		if includeSource {
			items = append(items, secretListItem{
				Name:      trimmed,
				Source:    defaultSecretSource,
				CreatedAt: createdAt.Format(time.RFC3339),
				UpdatedAt: updatedAt.Format(time.RFC3339),
			})
		} else {
			names = append(names, trimmed)
		}
	}

	condition := "scope IS NULL"
	args := []interface{}{}
	if env != "" {
		condition = "scope = $1"
		args = append(args, env)
	}

	generalQuery := fmt.Sprintf("SELECT name, created_at, updated_at FROM secrets WHERE repository_name IS NULL AND %s ORDER BY name ASC", condition)
	rows, err := a.db.Query(ctx, generalQuery, args...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query general secrets from database")
		http.Error(w, "Failed to retrieve secrets", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var name string
		var createdAt, updatedAt time.Time
		if scanErr := rows.Scan(&name, &createdAt, &updatedAt); scanErr != nil {
			rows.Close()
			log.Error().Err(scanErr).Msg("Failed to scan secret name")
			http.Error(w, "Failed to process secrets", http.StatusInternalServerError)
			return
		}
		addEntry(name, createdAt, updatedAt)
	}
	rows.Close()

	repoCondition := "scope IS NULL"
	repoArgs := []interface{}{}
	if env != "" {
		repoCondition = "scope = $1"
		repoArgs = append(repoArgs, env)
	}
	repoQuery := fmt.Sprintf("SELECT repository_name, name, created_at, updated_at FROM secrets WHERE repository_name IS NOT NULL AND %s ORDER BY repository_name ASC, name ASC", repoCondition)
	rows, err = a.db.Query(ctx, repoQuery, repoArgs...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query repository secrets from database")
		http.Error(w, "Failed to retrieve secrets", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var repoName, secretName string
		var createdAt, updatedAt time.Time
		if scanErr := rows.Scan(&repoName, &secretName, &createdAt, &updatedAt); scanErr != nil {
			rows.Close()
			log.Error().Err(scanErr).Msg("Failed to scan repository secret")
			http.Error(w, "Failed to process secrets", http.StatusInternalServerError)
			return
		}
		repo := strings.TrimSpace(repoName)
		name := strings.TrimSpace(secretName)
		if repo == "" || name == "" {
			continue
		}
		addEntry(repo+"/"+name, createdAt, updatedAt)
	}
	rows.Close()

	if includeSource {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(items); err != nil {
			log.Warn().Err(err).Msg("Failed to encode secret response")
		}
		return
	}

	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(names); err != nil {
		log.Warn().Err(err).Msg("Failed to encode secret response")
	}
}

type SecretScopeSummary struct {
	Scope       string `json:"scope"`
	SecretCount int    `json:"secret_count"`
}

func (a *App) handleListSecretScopes(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(context.Background(), `
		SELECT COALESCE(scope, '') AS scope_value, COUNT(*) AS secret_count
		FROM secrets
		GROUP BY scope_value
		ORDER BY scope_value NULLS FIRST`)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query secret scopes from database")
		http.Error(w, "Failed to retrieve secret scopes", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	scopeCounts := make(map[string]int)
	scopeCounts[""] = 0

	for rows.Next() {
		var env sql.NullString
		var count int
		if scanErr := rows.Scan(&env, &count); scanErr != nil {
			log.Error().Err(scanErr).Msg("Failed to scan secret scope row")
			http.Error(w, "Failed to process secret scopes", http.StatusInternalServerError)
			return
		}
		envValue := ""
		if env.Valid {
			envValue = strings.TrimSpace(env.String)
		}
		scopeCounts[envValue] += count
	}

	if err := rows.Err(); err != nil {
		log.Error().Err(err).Msg("Failed during secret scope iteration")
		http.Error(w, "Failed to process secret scopes", http.StatusInternalServerError)
		return
	}

	scopes := make([]SecretScopeSummary, 0, len(scopeCounts))
	for envValue, count := range scopeCounts {
		scopes = append(scopes, SecretScopeSummary{Scope: envValue, SecretCount: count})
	}

	sort.Slice(scopes, func(i, j int) bool {
		aEnv := scopes[i].Scope
		bEnv := scopes[j].Scope
		if aEnv == "" && bEnv == "" {
			return false
		}
		if aEnv == "" {
			return true
		}
		if bEnv == "" {
			return false
		}
		return strings.ToLower(aEnv) < strings.ToLower(bEnv)
	})

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(scopes); err != nil {
		log.Warn().Err(err).Msg("Failed to encode secret scopes response")
	}
}

func (a *App) handleGetGeneralSecretValue(w http.ResponseWriter, r *http.Request) {
	secretName := r.PathValue("secretName")
	if secretName == "" {
		http.Error(w, "Secret name is required", http.StatusBadRequest)
		return
	}

	env := r.URL.Query().Get("env")
	var query string
	var args []any
	if env != "" {
		query = "SELECT value FROM secrets WHERE name = $1 AND repository_name IS NULL AND scope = $2"
		args = []any{secretName, env}
	} else {
		query = "SELECT value FROM secrets WHERE name = $1 AND repository_name IS NULL AND scope IS NULL"
		args = []any{secretName}
	}

	var encryptedValue string
	err := a.db.QueryRow(context.Background(), query, args...).Scan(&encryptedValue)
	if err == pgx.ErrNoRows {
		http.Error(w, "Secret not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Error().Err(err).Str("secret", secretName).Msg("Failed to fetch secret value")
		http.Error(w, "Failed to fetch secret", http.StatusInternalServerError)
		return
	}

	value, err := a.decrypt(encryptedValue)
	if err != nil {
		log.Error().Err(err).Str("secret", secretName).Msg("Failed to decrypt secret value")
		http.Error(w, "Failed to decrypt secret", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"value": value})
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
		query := `INSERT INTO secrets (name, value, repository_name, scope, updated_at) VALUES ($1, $2, NULL, $3, NOW())
				  ON CONFLICT (name, repository_name, scope) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`
		_, err = a.db.Exec(context.Background(), query, secretName, encryptedValue, env)
	} else {
		query := `INSERT INTO secrets (name, value, repository_name, scope, updated_at) VALUES ($1, $2, NULL, NULL, NOW())
				  ON CONFLICT (name, repository_name, scope) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`
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
		_, err = a.db.Exec(context.Background(), "DELETE FROM secrets WHERE name = $1 AND repository_name IS NULL AND scope = $2", secretName, env)
	} else {
		_, err = a.db.Exec(context.Background(), "DELETE FROM secrets WHERE name = $1 AND repository_name IS NULL AND scope IS NULL", secretName)
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
		rows, err = a.db.Query(context.Background(), "SELECT name FROM secrets WHERE repository_name = $1 AND scope = $2 ORDER BY name ASC", fullName, env)
	} else {
		rows, err = a.db.Query(context.Background(), "SELECT name FROM secrets WHERE repository_name = $1 AND scope IS NULL ORDER BY name ASC", fullName)
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
		query := `INSERT INTO secrets (name, value, repository_name, scope, updated_at) VALUES ($1, $2, $3, $4, NOW())
				  ON CONFLICT (name, repository_name, scope) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`
		_, err = a.db.Exec(context.Background(), query, secretName, encryptedValue, fullName, env)
	} else {
		query := `INSERT INTO secrets (name, value, repository_name, scope, updated_at) VALUES ($1, $2, $3, NULL, NOW())
				  ON CONFLICT (name, repository_name, scope) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`
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
		_, err = a.db.Exec(context.Background(), "DELETE FROM secrets WHERE name = $1 AND repository_name = $2 AND scope = $3", secretName, fullName, env)
	} else {
		_, err = a.db.Exec(context.Background(), "DELETE FROM secrets WHERE name = $1 AND repository_name = $2 AND scope IS NULL", secretName, fullName)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to delete repo secret from database")
		http.Error(w, "Failed to delete secret", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type systemConfigPayload struct {
	ConfigRepoURL             *string `json:"config_repo_url"`
	AgentNopsaiAPIURL         *string `json:"agent_nopsai_api_url"`
	GitBotNopsaiAPIURL        *string `json:"git_bot_nopsai_api_url"`
	NopsaiGitBotAPIURL        *string `json:"nopsai_git_bot_api_url"`
	AgentImage                *string `json:"agent_image"`
	DockerNetworkName         *string `json:"docker_network_name"`
	AutoRemovalAgentContainer *bool   `json:"auto_removal_agent_container"`
	DefaultPipelineTimeout    *string `json:"default_pipeline_timeout"`
	LLMAgentTimeout           *string `json:"llm_agent_timeout"`
}

func (a *App) buildSystemConfigResponse(cfg config.Config) map[string]interface{} {
	return map[string]interface{}{
		"config_repo_url":              cfg.ConfigRepoURL,
		"agent_nopsai_api_url":         cfg.AgentNopsaiAPIURL,
		"git_bot_nopsai_api_url":       cfg.GitBotNopsaiAPIURL,
		"nopsai_git_bot_api_url":       cfg.NopsaiGitBotAPIURL,
		"agent_image":                  cfg.AgentImage,
		"docker_network_name":          cfg.DockerNetworkName,
		"auto_removal_agent_container": cfg.AutoRemovalAgentContainer,
		"default_pipeline_timeout":     cfg.DefaultPipelineTimeout,
		"llm_agent_timeout":            cfg.LLMAgentTimeout,
		"config_repo_configured":       strings.TrimSpace(cfg.ConfigRepoURL) != "",
		"config_sync_status":           a.getConfigSyncStatus(),
		"env_file_path":                a.envFilePath,
	}
}

func (a *App) applySystemConfig(payload systemConfigPayload) config.Config {
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()

	if payload.ConfigRepoURL != nil {
		a.cfg.ConfigRepoURL = strings.TrimSpace(*payload.ConfigRepoURL)
	}
	if payload.AgentNopsaiAPIURL != nil {
		a.cfg.AgentNopsaiAPIURL = strings.TrimSpace(*payload.AgentNopsaiAPIURL)
	}
	if payload.GitBotNopsaiAPIURL != nil {
		a.cfg.GitBotNopsaiAPIURL = strings.TrimSpace(*payload.GitBotNopsaiAPIURL)
	}
	if payload.NopsaiGitBotAPIURL != nil {
		a.cfg.NopsaiGitBotAPIURL = strings.TrimSpace(*payload.NopsaiGitBotAPIURL)
	}
	if payload.AgentImage != nil {
		a.cfg.AgentImage = strings.TrimSpace(*payload.AgentImage)
	}
	if payload.DockerNetworkName != nil {
		a.cfg.DockerNetworkName = strings.TrimSpace(*payload.DockerNetworkName)
	}
	if payload.AutoRemovalAgentContainer != nil {
		a.cfg.AutoRemovalAgentContainer = *payload.AutoRemovalAgentContainer
	}
	if payload.DefaultPipelineTimeout != nil {
		a.cfg.DefaultPipelineTimeout = strings.TrimSpace(*payload.DefaultPipelineTimeout)
	}
	if payload.LLMAgentTimeout != nil {
		a.cfg.LLMAgentTimeout = strings.TrimSpace(*payload.LLMAgentTimeout)
	}

	return *a.cfg
}

func (a *App) persistSystemConfig(cfg config.Config, payload systemConfigPayload) error {
	if a.configPath == "" {
		return nil
	}

	existing := map[string]interface{}{}
	if contents, err := os.ReadFile(a.configPath); err == nil {
		if len(contents) > 0 {
			if unmarshalErr := yaml.Unmarshal(contents, &existing); unmarshalErr != nil {
				log.Warn().Err(unmarshalErr).Msg("Failed to parse existing config file; rewriting allowed fields")
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if payload.ConfigRepoURL != nil {
		existing["config_repo_url"] = cfg.ConfigRepoURL
	}
	if payload.AgentImage != nil {
		existing["agent_image"] = cfg.AgentImage
	}
	if payload.AgentNopsaiAPIURL != nil {
		existing["agent_nopsai_api_url"] = cfg.AgentNopsaiAPIURL
	}
	if payload.GitBotNopsaiAPIURL != nil {
		existing["git_bot_nopsai_api_url"] = cfg.GitBotNopsaiAPIURL
	}
	if payload.NopsaiGitBotAPIURL != nil {
		existing["nopsai_git_bot_api_url"] = cfg.NopsaiGitBotAPIURL
	}
	if payload.DockerNetworkName != nil {
		existing["docker_network_name"] = cfg.DockerNetworkName
	}
	if payload.AutoRemovalAgentContainer != nil {
		existing["auto_removal_agent_container"] = cfg.AutoRemovalAgentContainer
	}
	if payload.DefaultPipelineTimeout != nil {
		existing["default_pipeline_timeout"] = cfg.DefaultPipelineTimeout
	}
	if payload.LLMAgentTimeout != nil {
		existing["llm_agent_timeout"] = cfg.LLMAgentTimeout
	}

	contents, err := yaml.Marshal(existing)
	if err != nil {
		return err
	}

	return os.WriteFile(a.configPath, contents, 0o644)
}

func (a *App) persistEnvOverrides(cfg config.Config, payload systemConfigPayload) error {
	if a.envFilePath == "" {
		return nil
	}

	updates := map[string]string{}

	if payload.ConfigRepoURL != nil {
		updates["CONFIG_REPO_URL"] = cfg.ConfigRepoURL
	}
	if payload.AgentNopsaiAPIURL != nil {
		updates["AGENT_NOPSAI_API_URL"] = cfg.AgentNopsaiAPIURL
	}
	if payload.GitBotNopsaiAPIURL != nil {
		updates["GIT_BOT_NOPSAI_API_URL"] = cfg.GitBotNopsaiAPIURL
	}
	if payload.NopsaiGitBotAPIURL != nil {
		updates["NOPSAI_GIT_BOT_API_URL"] = cfg.NopsaiGitBotAPIURL
	}
	if payload.AgentImage != nil {
		updates["AGENT_IMAGE"] = cfg.AgentImage
	}
	if payload.DockerNetworkName != nil {
		updates["DOCKER_NETWORK_NAME"] = cfg.DockerNetworkName
	}
	if payload.AutoRemovalAgentContainer != nil {
		updates["AUTO_REMOVAL_AGENT_CONTAINER"] = strconv.FormatBool(cfg.AutoRemovalAgentContainer)
	}
	if payload.DefaultPipelineTimeout != nil {
		updates["DEFAULT_PIPELINE_TIMEOUT"] = cfg.DefaultPipelineTimeout
	}
	if payload.LLMAgentTimeout != nil {
		updates["LLM_AGENT_TIMEOUT"] = cfg.LLMAgentTimeout
	}

	if len(updates) == 0 {
		return nil
	}

	return writeEnvFile(a.envFilePath, updates)
}

func writeEnvFile(path string, updates map[string]string) error {
	var lines []string
	used := make(map[string]bool, len(updates))

	if data, err := os.ReadFile(path); err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			line := scanner.Text()
			if key, ok := parseEnvKey(line); ok {
				if value, shouldReplace := updates[key]; shouldReplace {
					line = formatEnvLine(key, value)
					used[key] = true
				}
			}
			lines = append(lines, line)
		}
		if scanErr := scanner.Err(); scanErr != nil {
			return scanErr
		}
	}

	for key, value := range updates {
		if used[key] {
			continue
		}
		lines = append(lines, formatEnvLine(key, value))
	}

	output := strings.Join(lines, "\n")
	return os.WriteFile(path, []byte(output), 0o644)
}

func parseEnvKey(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	parts := strings.SplitN(trimmed, "=", 2)
	if len(parts) != 2 {
		return "", false
	}
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return "", false
	}
	return key, true
}

func formatEnvLine(key, value string) string {
	escaped := strings.ReplaceAll(value, `"`, `\"`)
	return fmt.Sprintf(`%s="%s"`, key, escaped)
}

func (a *App) handleGetSystemConfig(w http.ResponseWriter, r *http.Request) {
	cfg := a.getConfigSnapshot()
	resp := a.buildSystemConfigResponse(cfg)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Warn().Err(err).Msg("Failed to encode system config response")
	}
}

func (a *App) handleUpdateSystemConfig(w http.ResponseWriter, r *http.Request) {
	var payload systemConfigPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	cfg := a.applySystemConfig(payload)
	if err := a.persistSystemConfig(cfg, payload); err != nil {
		log.Warn().Err(err).Msg("Failed to persist system config; keeping in-memory settings only")
	}
	if err := a.persistEnvOverrides(cfg, payload); err != nil {
		log.Warn().Err(err).Msg("Failed to persist .env overrides; keeping in-memory settings only")
	}

	resp := a.buildSystemConfigResponse(cfg)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Warn().Err(err).Msg("Failed to encode updated system config response")
	}
}

func (a *App) handleGetConfigSyncStatus(w http.ResponseWriter, r *http.Request) {
	status := a.getConfigSyncStatus()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Warn().Err(err).Msg("Failed to encode config sync status")
	}
}

func (a *App) handleDispatcherStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status, err := a.dispatcher.GetStatus(ctx, &emptypb.Empty{})
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch dispatcher status")
		http.Error(w, "Failed to fetch dispatcher status", http.StatusBadGateway)
		return
	}

	a.cfgMu.RLock()
	routing := a.cfg.DispatcherRouting
	a.cfgMu.RUnlock()

	runners := make([]map[string]interface{}, 0, len(status.GetRunners()))
	for _, runner := range status.GetRunners() {
		runners = append(runners, map[string]interface{}{
			"runner_id":           runner.GetRunnerId(),
			"scopes":              runner.GetScopes(),
			"capacity":            runner.GetCapacity(),
			"active_jobs":         runner.GetActiveJobs(),
			"inflight_jobs":       runner.GetInflightJobs(),
			"last_heartbeat_unix": runner.GetLastHeartbeatUnix(),
			"metadata":            runner.GetMetadata(),
			"allow_dispatch":      runner.GetAllowDispatch(),
		})
	}

	resp := map[string]interface{}{
		"queued_jobs": status.GetQueuedJobs(),
		"runners":     runners,
	}
	if len(routing) > 0 {
		resp["routing"] = routing
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Warn().Err(err).Msg("Failed to encode dispatcher status")
	}
}

func (a *App) handleUpdateRunnerDispatch(w http.ResponseWriter, r *http.Request) {
	runnerID := strings.TrimSpace(r.PathValue("runnerID"))
	if runnerID == "" {
		http.Error(w, "runner_id is required", http.StatusBadRequest)
		return
	}

	var payload struct {
		AllowDispatch *bool  `json:"allow_dispatch"`
		ConnectionID  string `json:"connection_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	if payload.AllowDispatch == nil {
		http.Error(w, "allow_dispatch is required", http.StatusBadRequest)
		return
	}

	resp, err := a.dispatcher.UpdateRunnerDispatch(r.Context(), &proto.UpdateRunnerDispatchRequest{
		RunnerId:      runnerID,
		AllowDispatch: *payload.AllowDispatch,
		ConnectionId:  strings.TrimSpace(payload.ConnectionID),
	})
	if err != nil {
		log.Error().Err(err).Str("runner_id", runnerID).Msg("Failed to update runner dispatch state")
		statusCode := http.StatusBadGateway
		if st, ok := grpcstatus.FromError(err); ok {
			switch st.Code() {
			case codes.InvalidArgument:
				statusCode = http.StatusBadRequest
			case codes.NotFound:
				statusCode = http.StatusNotFound
			case codes.Unavailable:
				statusCode = http.StatusBadGateway
			default:
				statusCode = http.StatusInternalServerError
			}
			http.Error(w, st.Message(), statusCode)
			return
		}
		http.Error(w, "Failed to update runner dispatch", statusCode)
		return
	}

	if resp == nil || resp.Runner == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp.Runner); err != nil {
		log.Warn().Err(err).Msg("Failed to encode runner dispatch response")
	}
}

func (a *App) prepareSecretsForPipeline(pipeline models.Pipeline, gitContext map[string]string, scope string) (map[string]string, error) {
	requiredSecrets := make(map[string]struct{})
	for _, step := range pipeline.Steps {
		for _, secretName := range step.GetSecrets() {
			requiredSecrets[secretName] = struct{}{}
		}
	}

	if len(requiredSecrets) == 0 {
		return nil, nil
	}

	finalSecrets := make(map[string]string)
	repoFullName := fmt.Sprintf("%s/%s", gitContext["repo_owner"], gitContext["repo_name"])

	for secretName := range requiredSecrets {
		encryptedValue, found, err := a.findEncryptedSecret(secretName, repoFullName, scope)
		if err != nil {
			return nil, fmt.Errorf("pipeline aborted: failed to resolve secret '%s': %w", secretName, err)
		}
		if !found {
			if scope != "" {
				return nil, fmt.Errorf("pipeline aborted: required secret '%s' not found for scope '%s'", secretName, scope)
			}
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

func (a *App) handleConfigSync(w http.ResponseWriter, r *http.Request) {
	repoURL := a.getConfigRepoURL()
	if repoURL == "" {
		http.Error(w, "CONFIG_REPO_URL is not configured", http.StatusBadRequest)
		return
	}

	if a.isConfigSyncRunning() {
		http.Error(w, "A configuration sync is already in progress", http.StatusConflict)
		return
	}

	startedAt := time.Now()
	a.setConfigSyncStatus(ConfigSyncStatus{
		Status:    "running",
		Message:   "Configuration synchronization started.",
		StartedAt: &startedAt,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(a.getConfigSyncStatus()); err != nil {
		log.Warn().Err(err).Msg("Failed to encode config sync response")
	}

	go func(started time.Time) {
		log.Info().Msg("Starting configuration synchronization from Git")
		details, syncErr := a.syncConfigurationFromGit(context.Background())

		completedAt := time.Now()
		if syncErr != nil {
			log.Error().Err(syncErr).Msg("Configuration synchronization failed")
			a.setConfigSyncStatus(ConfigSyncStatus{
				Status:      "error",
				Message:     fmt.Sprintf("Configuration synchronization failed: %v", syncErr),
				StartedAt:   &started,
				CompletedAt: &completedAt,
			})
			return
		}
		log.Info().Interface("details", details).Msg("Configuration synchronization succeeded")
		a.setConfigSyncStatus(ConfigSyncStatus{
			Status:      "success",
			Message:     "Configuration synchronization completed successfully.",
			Details:     details,
			StartedAt:   &started,
			CompletedAt: &completedAt,
		})
	}(startedAt)
}

func (a *App) syncConfigurationFromGit(ctx context.Context) (map[string]int, error) {
	details := map[string]int{
		"pipelines_synced":    0,
		"steps_synced":        0,
		"general_vars_synced": 0,
		"repo_vars_synced":    0,
		"triggers_synced":     0,
		"run_groups_created":  0,
		"run_groups_updated":  0,
	}

	repoURL := a.getConfigRepoURL()
	if repoURL == "" {
		return nil, fmt.Errorf("CONFIG_REPO_URL is not configured")
	}

	owner, repo, err := parseGitHubRepoURL(repoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CONFIG_REPO_URL: %w", err)
	}
	if err := a.ensureConfigRepoAccessible(owner, repo); err != nil {
		return nil, err
	}

	// --- 1. Fetch all configurations from Git ---

	pipelineFiles, err := a.requestGitBotDirectory(owner, repo, "pipelines")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pipeline definitions: %w", err)
	}
	stepFiles, err := a.requestGitBotDirectory(owner, repo, "steps")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch reusable steps: %w", err)
	}
	triggerFiles, err := a.requestGitBotDirectory(owner, repo, "triggers")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch trigger manifests: %w", err)
	}
	environmentFiles, err := a.requestGitBotDirectory(owner, repo, "environments")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch environment definitions: %w", err)
	}
	pipelineRunFiles, err := a.requestGitBotDirectory(owner, repo, "pipelineruns")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pipeline run structure definitions: %w", err)
	}

	var pipelineRunStructure map[string]*pipelineRunStructureNode
	for path, content := range pipelineRunFiles {
		normalized := filepath.ToSlash(path)
		rel := strings.TrimPrefix(normalized, "pipelineruns/")
		if rel == "structure.yaml" || rel == "structure.yml" {
			parsed, err := parsePipelineRunStructure(content)
			if err != nil {
				return nil, fmt.Errorf("failed to parse pipeline run structure '%s': %w", normalized, err)
			}
			pipelineRunStructure = parsed
			break
		}
	}

	type storedPipeline struct {
		definition string
		version    string
		path       string
		name       string
	}
	type storedStep struct {
		definition string
		path       string
		name       string
	}

	// --- 2. Parse Files ---

	pipelines := make(map[string]storedPipeline)
	for path, content := range pipelineFiles {
		normalized := filepath.ToSlash(path)
		rel := strings.TrimPrefix(normalized, "pipelines/")
		if rel == "" || strings.HasSuffix(rel, "/") || !isYAMLFile(rel) {
			continue
		}

		var pipeline models.Pipeline
		if err := yaml.Unmarshal([]byte(content), &pipeline); err != nil {
			return nil, fmt.Errorf("failed to parse pipeline '%s': %w", normalized, err)
		}
		if err := validatePipeline(&pipeline); err != nil {
			return nil, fmt.Errorf("pipeline validation failed for '%s': %w", normalized, err)
		}

		pipelinePath, fileBase, _, err := splitPipelineIdentifier(rel)
		if err != nil {
			return nil, fmt.Errorf("invalid pipeline path '%s': %w", normalized, err)
		}
		if pipeline.Name != fileBase {
			return nil, fmt.Errorf("pipeline '%s' name '%s' must match file name '%s'", normalized, pipeline.Name, fileBase)
		}

		key := buildPipelineIdentifier(pipelinePath, pipeline.Name)
		if _, exists := pipelines[key]; exists {
			return nil, fmt.Errorf("duplicate pipeline '%s' detected in config repository", key)
		}

		pipelines[key] = storedPipeline{
			definition: content,
			version:    normalizePipelineVersion(pipeline.Version),
			path:       pipelinePath,
			name:       pipeline.Name,
		}
	}

	steps := make(map[string]storedStep)
	for path, content := range stepFiles {
		normalized := filepath.ToSlash(path)
		rel := strings.TrimPrefix(normalized, "steps/")
		if rel == "" || strings.HasSuffix(rel, "/") || !isYAMLFile(rel) {
			continue
		}

		var step models.PipelineStep
		if err := yaml.Unmarshal([]byte(content), &step); err != nil {
			return nil, fmt.Errorf("failed to parse reusable step '%s': %w", normalized, err)
		}
		stepName := step.GetName()
		if stepName == "" {
			return nil, fmt.Errorf("reusable step '%s' is missing the required 'name' field", normalized)
		}

		stepPath, fileBase, _, err := splitStepIdentifier(rel)
		if err != nil {
			return nil, fmt.Errorf("invalid reusable step path '%s': %w", normalized, err)
		}
		if stepName != fileBase {
			return nil, fmt.Errorf("reusable step '%s' name '%s' must match file name '%s'", normalized, stepName, fileBase)
		}

		key := buildStepIdentifier(stepPath, stepName)
		if _, exists := steps[key]; exists {
			return nil, fmt.Errorf("duplicate reusable step '%s' detected in config repository", key)
		}

		steps[key] = storedStep{
			definition: content,
			path:       stepPath,
			name:       stepName,
		}
	}

	generalEnvs := make(map[generalEnvKey]string)
	repoEnvs := make(map[repoEnvKey]string)

	for path, content := range environmentFiles {
		normalized := filepath.ToSlash(path)
		rel := strings.TrimPrefix(normalized, "environments/")
		if rel == "" || strings.HasSuffix(rel, "/") {
			continue
		}

		envPath, ok, err := parseEnvironmentFilePath(rel)
		if err != nil {
			return nil, fmt.Errorf("invalid environment file '%s': %w", normalized, err)
		}
		if !ok {
			continue
		}

		var raw map[string]interface{}
		if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
			return nil, fmt.Errorf("failed to parse environment file '%s': %w", normalized, err)
		}

		for key, value := range raw {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				return nil, fmt.Errorf("environment file '%s' contains an empty key", normalized)
			}

			strValue, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("environment entry '%s' in '%s' must be a string", trimmedKey, normalized)
			}

			parts := strings.Split(trimmedKey, "/")
			switch len(parts) {
			case 1:
				gKey := generalEnvKey{envPath: envPath, name: trimmedKey}
				if _, exists := generalEnvs[gKey]; exists {
					return nil, fmt.Errorf("duplicate environment variable '%s' for '%s' detected", trimmedKey, envPath)
				}
				generalEnvs[gKey] = strValue
			case 3:
				repoName := fmt.Sprintf("%s/%s", strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
				varName := strings.TrimSpace(parts[2])
				if repoName == "" || varName == "" {
					return nil, fmt.Errorf("invalid repository-scoped environment key '%s' in '%s'", trimmedKey, normalized)
				}
				rKey := repoEnvKey{repo: repoName, envPath: envPath, name: varName}
				if _, exists := repoEnvs[rKey]; exists {
					return nil, fmt.Errorf("duplicate repository environment variable '%s' for '%s' detected", trimmedKey, envPath)
				}
				repoEnvs[rKey] = strValue
			default:
				return nil, fmt.Errorf("environment key '%s' in '%s' has an unsupported format", trimmedKey, normalized)
			}
		}
	}

	triggers := make(map[string]string)
	for path, content := range triggerFiles {
		normalized := filepath.ToSlash(path)
		rel := strings.TrimPrefix(normalized, "triggers/")
		if rel == "" || strings.HasSuffix(rel, "/") || !isYAMLFile(rel) {
			continue
		}

		repoKey := strings.TrimSuffix(rel, filepath.Ext(rel))
		repoKey = strings.Trim(repoKey, "/")
		if repoKey == "" {
			return nil, fmt.Errorf("trigger file '%s' does not specify a repository", normalized)
		}
		if strings.Contains(repoKey, "..") {
			return nil, fmt.Errorf("trigger file '%s' contains invalid path segments", normalized)
		}
		repoKey = filepath.ToSlash(repoKey)

		if err := yaml.Unmarshal([]byte(content), &models.Manifest{}); err != nil {
			return nil, fmt.Errorf("failed to parse trigger manifest '%s': %w", normalized, err)
		}

		if _, exists := triggers[repoKey]; exists {
			return nil, fmt.Errorf("duplicate trigger manifest for repository '%s' detected", repoKey)
		}

		triggers[repoKey] = content
	}

	// --- 3. Database Transaction (Upsert + Prune) ---
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	const pipelineUpsert = `INSERT INTO pipelines (path, name, version, definition, source, updated_at) VALUES ($1, $2, $3, $4, 'git', NOW())
		ON CONFLICT (path, name) DO UPDATE SET version = EXCLUDED.version, definition = EXCLUDED.definition, source = 'git', updated_at = NOW()`
	const stepUpsert = `INSERT INTO steps (path, name, definition, source, updated_at) VALUES ($1, $2, $3, 'git', NOW())
		ON CONFLICT (path, name) DO UPDATE SET definition = EXCLUDED.definition, source = 'git', updated_at = NOW()`
	const envUpsert = `INSERT INTO variables (name, value, repository_name, scope, source, updated_at) VALUES ($1, $2, $3, $4, 'git', NOW())
		ON CONFLICT (name, repository_name, scope) DO UPDATE SET value = EXCLUDED.value, source = 'git', updated_at = NOW()`
	const triggerUpsert = `INSERT INTO triggers (repository_name, trigger_definition, source) VALUES ($1, $2, 'git')
		ON CONFLICT (repository_name) DO UPDATE SET trigger_definition = EXCLUDED.trigger_definition, source = 'git'`

	// A. Upsert Pipelines
	for key, stored := range pipelines {
		if _, err := tx.Exec(ctx, pipelineUpsert, stored.path, stored.name, stored.version, stored.definition); err != nil {
			return nil, fmt.Errorf("failed to upsert pipeline '%s': %w", key, err)
		}
		details["pipelines_synced"]++
	}

	// B. Upsert Steps
	for key, stored := range steps {
		if _, err := tx.Exec(ctx, stepUpsert, stored.path, stored.name, stored.definition); err != nil {
			return nil, fmt.Errorf("failed to upsert reusable step '%s': %w", key, err)
		}
		details["steps_synced"]++
	}

	// C. Upsert General Envs
	for key, value := range generalEnvs {
		var envParam interface{}
		if key.envPath != "" {
			envParam = key.envPath
		}
		if _, err := tx.Exec(ctx, envUpsert, key.name, value, nil, envParam); err != nil {
			return nil, fmt.Errorf("failed to upsert variable '%s' for scope '%s': %w", key.name, key.envPath, err)
		}
		details["general_vars_synced"]++
	}

	// D. Upsert Repo Envs
	for key, value := range repoEnvs {
		var envParam interface{}
		if key.envPath != "" {
			envParam = key.envPath
		}
		if _, err := tx.Exec(ctx, envUpsert, key.name, value, key.repo, envParam); err != nil {
			return nil, fmt.Errorf("failed to upsert repository variable '%s' for repo '%s' scope '%s': %w", key.name, key.repo, key.envPath, err)
		}
		details["repo_vars_synced"]++
	}

	// E. Upsert Triggers
	for repoName, definition := range triggers {
		if _, err := tx.Exec(ctx, triggerUpsert, repoName, definition); err != nil {
			return nil, fmt.Errorf("failed to upsert trigger override '%s': %w", repoName, err)
		}
		details["triggers_synced"]++
	}

	// --- PRUNING PHASE: Remove items that exist in DB as source='git' but were not in the Git payload ---

	// 1. Prune Pipelines
	{
		var paths, names []string
		for _, p := range pipelines {
			paths = append(paths, p.path)
			names = append(names, p.name)
		}
		if len(paths) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM pipelines WHERE source = 'git'"); err != nil {
				return nil, fmt.Errorf("failed to prune pipelines: %w", err)
			}
		} else {
			// Delete where source='git' AND (path, name) NOT IN the lists we just processed
			if _, err := tx.Exec(ctx, `
				DELETE FROM pipelines 
				WHERE source = 'git' 
				AND NOT EXISTS (
					SELECT 1 FROM unnest($1::text[], $2::text[]) AS t(p, n) 
					WHERE pipelines.path = t.p AND pipelines.name = t.n
				)`, paths, names); err != nil {
				return nil, fmt.Errorf("failed to prune pipelines: %w", err)
			}
		}
	}

	// 2. Prune Steps
	{
		var paths, names []string
		for _, s := range steps {
			paths = append(paths, s.path)
			names = append(names, s.name)
		}
		if len(paths) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM steps WHERE source = 'git'"); err != nil {
				return nil, fmt.Errorf("failed to prune steps: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, `
				DELETE FROM steps 
				WHERE source = 'git' 
				AND NOT EXISTS (
					SELECT 1 FROM unnest($1::text[], $2::text[]) AS t(p, n) 
					WHERE steps.path = t.p AND steps.name = t.n
				)`, paths, names); err != nil {
				return nil, fmt.Errorf("failed to prune steps: %w", err)
			}
		}
	}

	// 3. Prune Triggers
	{
		var repos []string
		for repo := range triggers {
			repos = append(repos, repo)
		}
		if len(repos) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM triggers WHERE source = 'git'"); err != nil {
				return nil, fmt.Errorf("failed to prune triggers: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, "DELETE FROM triggers WHERE source = 'git' AND repository_name != ALL($1)", repos); err != nil {
				return nil, fmt.Errorf("failed to prune triggers: %w", err)
			}
		}
	}

	// 4. Prune Variables (Environment Variables)
	{
		var names []string
		var repos []*string
		var scopes []*string

		// Helper to collect all valid (name, repo, scope) tuples
		addVar := func(n string, r *string, s string) {
			names = append(names, n)
			repos = append(repos, r)
			if s == "" {
				scopes = append(scopes, nil)
			} else {
				scopes = append(scopes, &s)
			}
		}

		for key := range generalEnvs {
			addVar(key.name, nil, key.envPath)
		}
		for key := range repoEnvs {
			r := key.repo // copy loop variable
			addVar(key.name, &r, key.envPath)
		}

		if len(names) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM variables WHERE source = 'git'"); err != nil {
				return nil, fmt.Errorf("failed to prune variables: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, `
				DELETE FROM variables 
				WHERE source = 'git' 
				AND NOT EXISTS (
					SELECT 1 FROM unnest($1::text[], $2::text[], $3::text[]) AS t(n, r, s) 
					WHERE variables.name = t.n 
					AND variables.repository_name IS NOT DISTINCT FROM t.r 
					AND variables.scope IS NOT DISTINCT FROM t.s
				)`, names, repos, scopes); err != nil {
				return nil, fmt.Errorf("failed to prune variables: %w", err)
			}
		}
	}

	// Sync groups (UI folders) - Note: Groups do not have a 'source' column, so we do not prune them to avoid deleting user-created folders.
	if len(pipelineRunStructure) > 0 {
		if err := a.syncPipelineRunGroups(ctx, tx, pipelineRunStructure, details); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit configuration synchronization transaction: %w", err)
	}

	log.Info().
		Str("repo_owner", owner).
		Str("repo_name", repo).
		Int("pipelines_synced", details["pipelines_synced"]).
		Int("steps_synced", details["steps_synced"]).
		Int("general_vars_synced", details["general_vars_synced"]).
		Int("repo_vars_synced", details["repo_vars_synced"]).
		Int("triggers_synced", details["triggers_synced"]).
		Int("run_groups_created", details["run_groups_created"]).
		Int("run_groups_updated", details["run_groups_updated"]).
		Msg("Configuration synchronization from Git completed")

	return details, nil
}

type generalEnvKey struct {
	envPath string
	name    string
}

type repoEnvKey struct {
	repo    string
	envPath string
	name    string
}

type pipelineRunStructureNode struct {
	Description string
	Repos       []string
	Children    map[string]*pipelineRunStructureNode
}

type groupRecord struct {
	ID          int
	ParentID    *int
	Description string
}

func parsePipelineRunStructure(content string) (map[string]*pipelineRunStructureNode, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return map[string]*pipelineRunStructureNode{}, nil
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
		return nil, err
	}
	result := make(map[string]*pipelineRunStructureNode, len(raw))
	for name, value := range raw {
		normalized, err := normalizeStructureName(name)
		if err != nil {
			return nil, err
		}
		if _, exists := result[normalized]; exists {
			return nil, fmt.Errorf("duplicate folder '%s' in pipelinerun structure", normalized)
		}
		node, err := decodePipelineRunStructureNode(value)
		if err != nil {
			return nil, fmt.Errorf("folder '%s': %w", normalized, err)
		}
		result[normalized] = node
	}
	return result, nil
}

func decodePipelineRunStructureNode(value interface{}) (*pipelineRunStructureNode, error) {
	node := &pipelineRunStructureNode{Children: map[string]*pipelineRunStructureNode{}}
	if value == nil {
		return node, nil
	}

	switch typed := value.(type) {
	case string:
		node.Description = strings.TrimSpace(typed)
		return node, nil
	case map[string]interface{}:
		return decodePipelineRunStructureMap(node, typed)
	default:
		return nil, fmt.Errorf("expected mapping or description for folder, got %T", value)
	}
}

func decodePipelineRunStructureMap(node *pipelineRunStructureNode, childMap map[string]interface{}) (*pipelineRunStructureNode, error) {

	for key, raw := range childMap {
		switch key {
		case "repos":
			repos, err := parseStructureRepoList(raw)
			if err != nil {
				return nil, err
			}
			node.Repos = repos
		case "description":
			if raw == nil {
				node.Description = ""
				continue
			}
			text, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("description must be a string, got %T", raw)
			}
			node.Description = strings.TrimSpace(text)
		default:
			childName, err := normalizeStructureName(key)
			if err != nil {
				return nil, err
			}
			if _, exists := node.Children[childName]; exists {
				return nil, fmt.Errorf("duplicate folder '%s' detected", childName)
			}
			childNode, err := decodePipelineRunStructureNode(raw)
			if err != nil {
				return nil, fmt.Errorf("folder '%s': %w", childName, err)
			}
			node.Children[childName] = childNode
		}
	}

	return node, nil
}

func parseStructureRepoList(value interface{}) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("repos must be defined as a list, got %T", value)
	}
	var repos []string
	for idx, raw := range items {
		if raw == nil {
			continue
		}
		text, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("repos entry %d must be a string, got %T", idx, raw)
		}
		repo := strings.TrimSpace(text)
		if repo == "" {
			continue
		}
		repos = append(repos, repo)
	}
	return repos, nil
}

func normalizeStructureName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("pipelinerun structure contains an empty folder or repository name")
	}
	return trimmed, nil
}

func loadExistingGroupRecords(ctx context.Context, tx pgx.Tx) (map[string]*groupRecord, error) {
	rows, err := tx.Query(ctx, "SELECT id, name, parent_id, description FROM groups")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*groupRecord)
	for rows.Next() {
		var (
			id          int
			name        string
			parentID    sql.NullInt32
			description sql.NullString
		)
		if err := rows.Scan(&id, &name, &parentID, &description); err != nil {
			return nil, err
		}
		key, err := normalizeStructureName(name)
		if err != nil {
			return nil, err
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate group name '%s' detected in database", key)
		}
		result[key] = &groupRecord{
			ID:          id,
			ParentID:    pointerFromNullInt(parentID),
			Description: strings.TrimSpace(description.String),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func pointerFromNullInt(value sql.NullInt32) *int {
	if !value.Valid {
		return nil
	}
	v := int(value.Int32)
	return &v
}

func copyIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	v := *value
	return &v
}

func parentPointersEqual(a, b *int) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a != nil && b != nil:
		return *a == *b
	default:
		return false
	}
}

func (a *App) syncPipelineRunGroups(ctx context.Context, tx pgx.Tx, structure map[string]*pipelineRunStructureNode, details map[string]int) error {
	if len(structure) == 0 {
		return nil
	}

	existingGroups, err := loadExistingGroupRecords(ctx, tx)
	if err != nil {
		return fmt.Errorf("failed to load existing pipeline run folders: %w", err)
	}

	var ensureGroup func(name string, parentID *int, description string) (int, error)
	ensureGroup = func(name string, parentID *int, description string) (int, error) {
		normalized, err := normalizeStructureName(name)
		if err != nil {
			return 0, err
		}
		description = strings.TrimSpace(description)
		if record, ok := existingGroups[normalized]; ok {
			parentChanged := !parentPointersEqual(record.ParentID, parentID)
			descChanged := strings.TrimSpace(record.Description) != description
			if parentChanged || descChanged {
				if _, err := tx.Exec(ctx, "UPDATE groups SET parent_id = $1, description = $2, updated_at = NOW() WHERE id = $3", parentID, description, record.ID); err != nil {
					return 0, fmt.Errorf("failed to update folder '%s': %w", normalized, err)
				}
				record.ParentID = copyIntPointer(parentID)
				record.Description = description
				details["run_groups_updated"]++
			}
			return record.ID, nil
		}

		var newID int
		if err := tx.QueryRow(ctx, "INSERT INTO groups (name, parent_id, description) VALUES ($1, $2, $3) RETURNING id", normalized, parentID, description).Scan(&newID); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				refreshed, loadErr := loadExistingGroupRecords(ctx, tx)
				if loadErr != nil {
					return 0, fmt.Errorf("failed to reload folders after conflict: %w", loadErr)
				}
				existingGroups = refreshed
				if _, ok := existingGroups[normalized]; ok {
					return ensureGroup(normalized, parentID, description)
				}
			}
			return 0, fmt.Errorf("failed to create folder '%s': %w", normalized, err)
		}
		existingGroups[normalized] = &groupRecord{ID: newID, ParentID: copyIntPointer(parentID), Description: description}
		details["run_groups_created"]++
		return newID, nil
	}

	var applyNode func(name string, node *pipelineRunStructureNode, parentID *int) error
	applyNode = func(name string, node *pipelineRunStructureNode, parentID *int) error {
		groupID, err := ensureGroup(name, parentID, node.Description)
		if err != nil {
			return err
		}
		for _, repoName := range node.Repos {
			if _, err := ensureGroup(repoName, &groupID, ""); err != nil {
				return err
			}
		}
		for childName, childNode := range node.Children {
			if err := applyNode(childName, childNode, &groupID); err != nil {
				return err
			}
		}
		return nil
	}

	for name, node := range structure {
		if node == nil {
			node = &pipelineRunStructureNode{Children: map[string]*pipelineRunStructureNode{}}
		}
		if err := applyNode(name, node, nil); err != nil {
			return fmt.Errorf("failed to sync folder '%s': %w", name, err)
		}
	}

	return nil
}

func normalizeVariableSourceKey(value string) string {
	key := strings.TrimSpace(strings.ToLower(value))
	switch {
	case strings.Contains(key, "git"):
		return "git"
	case strings.Contains(key, "draft"):
		return "draft"
	case strings.Contains(key, "local"):
		return "local"
	default:
		return "database"
	}
}

func isYAMLFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}

func (a *App) handleListPipelines(w http.ResponseWriter, r *http.Request) {
	includeSource := strings.EqualFold(r.URL.Query().Get("include_source"), "true")
	rows, err := a.db.Query(context.Background(), "SELECT path, name, source FROM pipelines ORDER BY path ASC, name ASC")
	if err != nil {
		log.Error().Err(err).Msg("Failed to query pipelines from database")
		http.Error(w, "Failed to retrieve pipelines", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type pipelineListItem struct {
		ID     string `json:"id"`
		Source string `json:"source"`
	}

	var (
		pipelineNames []string
		pipelineItems []pipelineListItem
	)

	for rows.Next() {
		var path, name, source string
		if err := rows.Scan(&path, &name, &source); err != nil {
			log.Error().Err(err).Msg("Failed to scan pipeline entry")
			http.Error(w, "Failed to process pipelines", http.StatusInternalServerError)
			return
		}
		identifier := buildPipelineIdentifier(path, name)
		if includeSource {
			pipelineItems = append(pipelineItems, pipelineListItem{ID: identifier, Source: source})
		} else {
			pipelineNames = append(pipelineNames, identifier)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if includeSource {
		json.NewEncoder(w).Encode(pipelineItems)
		return
	}
	json.NewEncoder(w).Encode(pipelineNames)
}

// handleListTriggerOverrides retrieves the names of all repositories with active trigger overrides.
func (a *App) handleListTriggerOverrides(w http.ResponseWriter, r *http.Request) {
	includeSource := strings.EqualFold(r.URL.Query().Get("include_source"), "true")

	var (
		rows pgx.Rows
		err  error
	)

	if includeSource {
		rows, err = a.db.Query(context.Background(), "SELECT repository_name, source FROM triggers ORDER BY repository_name ASC")
	} else {
		rows, err = a.db.Query(context.Background(), "SELECT repository_name FROM triggers ORDER BY repository_name ASC")
	}
	if err != nil {
		log.Error().Err(err).Msg("Failed to query trigger overrides from database")
		http.Error(w, "Failed to retrieve trigger overrides", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type triggerOverrideItem struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	}

	var (
		repoNames []string
		items     []triggerOverrideItem
	)

	for rows.Next() {
		var name string
		var source string
		if includeSource {
			if err := rows.Scan(&name, &source); err != nil {
				log.Error().Err(err).Msg("Failed to scan trigger override entry")
				http.Error(w, "Failed to process trigger overrides", http.StatusInternalServerError)
				return
			}
			source = strings.TrimSpace(strings.ToLower(source))
			switch {
			case source == "" || source == "db" || source == "database":
				source = "database"
			case strings.Contains(source, "git"):
				source = "git"
			case strings.Contains(source, "draft"):
				source = "draft"
			case strings.Contains(source, "local") || strings.Contains(source, "repo file") || strings.Contains(source, "repository file"):
				source = "local"
			default:
				source = "database"
			}
			items = append(items, triggerOverrideItem{Name: name, Source: source})
		} else {
			if err := rows.Scan(&name); err != nil {
				log.Error().Err(err).Msg("Failed to scan repository name")
				http.Error(w, "Failed to process trigger overrides", http.StatusInternalServerError)
				return
			}
			repoNames = append(repoNames, name)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if includeSource {
		json.NewEncoder(w).Encode(items)
		return
	}
	json.NewEncoder(w).Encode(repoNames)
}

func (a *App) handleGetPipeline(w http.ResponseWriter, r *http.Request) {
	pipelineIdentifier := r.PathValue("pipelineName")
	pathPart, namePart, extPart, err := splitPipelineIdentifier(pipelineIdentifier)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rows, err := a.db.Query(context.Background(), "SELECT definition FROM pipelines WHERE path = $1 AND name = $2", pathPart, namePart)
	if err != nil {
		log.Error().Err(err).Str("pipeline", pipelineIdentifier).Msg("Database query failed for pipeline")
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var pipelineDef string
	if rows.Next() {
		err = rows.Scan(&pipelineDef)
		if err != nil {
			log.Error().Err(err).Str("pipeline", pipelineIdentifier).Msg("Database scan failed for pipeline definition")
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
	} else {
		err = pgx.ErrNoRows
	}

	if err == nil {
		w.Header().Set("Content-Type", "application/x-yaml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(pipelineDef))
		return
	}

	if err != pgx.ErrNoRows {
		log.Error().Err(err).Str("pipeline", pipelineIdentifier).Msg("Database error while fetching pipeline")
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	repoOwner := r.URL.Query().Get("repoOwner")
	repoName := r.URL.Query().Get("repoName")
	commitSHA := r.URL.Query().Get("commitSHA")

	if repoOwner != "" && repoName != "" && commitSHA != "" {
		log.Info().Str("pipeline", pipelineIdentifier).Msg("Pipeline not in DB, attempting to fetch from repository as fallback")

		fetchWithExtension := func(extension string) ([]byte, error) {
			pipelinePath := buildPipelineFilePath(pathPart, namePart, extension)
			if !strings.HasPrefix(pipelinePath, ".nopsai/") {
				pipelinePath = ".nopsai/" + pipelinePath
			}
			return a.requestGitBotPipeline(repoOwner, repoName, commitSHA, models.PipelineSource{Path: pipelinePath})
		}

		extensions := []string{}
		if extPart != "" {
			extensions = append(extensions, extPart)
		} else {
			extensions = append(extensions, ".yaml", ".yml")
		}

		var pipelineYAML []byte
		var fetchErr error
		for _, extension := range extensions {
			pipelineYAML, fetchErr = fetchWithExtension(extension)
			if fetchErr == nil {
				break
			}
		}
		if fetchErr != nil {
			log.Error().Err(fetchErr).Str("pipeline", pipelineIdentifier).Msg("Failed to fetch pipeline from repository as fallback")
			http.Error(w, "Pipeline not found in database or repository", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/x-yaml")
		w.WriteHeader(http.StatusOK)
		w.Write(pipelineYAML)
		return
	}

	log.Error().Err(err).Str("pipeline", pipelineIdentifier).Msg("Pipeline not found in database and no git context for fallback")
	http.Error(w, "Pipeline not found", http.StatusNotFound)
}

func (a *App) handleGetTriggerOverride(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)

	var triggerDef string
	err := a.db.QueryRow(context.Background(), "SELECT trigger_definition FROM triggers WHERE repository_name = $1", fullName).Scan(&triggerDef)
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

	var existingSource string
	lookupErr := a.db.QueryRow(context.Background(), "SELECT source FROM triggers WHERE repository_name = $1", fullName).Scan(&existingSource)
	if lookupErr != nil && lookupErr != pgx.ErrNoRows {
		log.Error().Err(lookupErr).Msg("Failed to inspect existing trigger source")
		http.Error(w, "Failed to save trigger override", http.StatusInternalServerError)
		return
	}

	desiredSource := "database"
	if strings.EqualFold(existingSource, "git") {
		desiredSource = existingSource
	}

	query := `INSERT INTO triggers (repository_name, trigger_definition, source) VALUES ($1, $2, $3)
			  ON CONFLICT (repository_name) DO UPDATE SET trigger_definition = $2, source = $3`
	_, err = a.db.Exec(context.Background(), query, fullName, string(triggerDef), desiredSource)
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

	_, err := a.db.Exec(context.Background(), "DELETE FROM triggers WHERE repository_name = $1", fullName)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete trigger override from database")
		http.Error(w, "Failed to delete trigger override", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleCreateOrUpdatePipeline(w http.ResponseWriter, r *http.Request) {
	identifier := r.PathValue("pipelineName")
	var (
		dbPath       string
		expectedName string
	)
	if identifier != "" {
		pathPart, namePart, _, err := splitPipelineIdentifier(identifier)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		dbPath = pathPart
		expectedName = namePart
	}

	pipelineDef, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	var pipeline models.Pipeline
	decoder := yaml.NewDecoder(bytes.NewReader(pipelineDef))

	if err := decoder.Decode(&pipeline); err != nil {
		var genericYAML map[string]interface{}
		if err := yaml.Unmarshal(pipelineDef, &genericYAML); err == nil {
			if _, hasTriggersKey := genericYAML["triggers"]; hasTriggersKey {
				http.Error(w, "Validation failed: The provided file appears to be a trigger manifest, not a pipeline. A pipeline must contain 'steps', not 'triggers'.", http.StatusBadRequest)
				return
			}
		}
		http.Error(w, fmt.Sprintf("Pipeline YAML is malformed or contains unknown fields: %v", err), http.StatusBadRequest)
		return
	}

	if err := validatePipeline(&pipeline); err != nil {
		http.Error(w, fmt.Sprintf("Pipeline validation failed: %v", err), http.StatusBadRequest)
		return
	}

	pipeline.Version = normalizePipelineVersion(pipeline.Version)

	if expectedName != "" && expectedName != pipeline.Name {
		errorMsg := fmt.Sprintf("Validation failed: the pipeline name in the URL ('%s') must match the 'name' field in the YAML ('%s').", expectedName, pipeline.Name)
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	storedName := pipeline.Name
	storedVersion := pipeline.Version

	var existingSource string
	lookupErr := a.db.QueryRow(context.Background(), "SELECT source FROM pipelines WHERE path = $1 AND name = $2", dbPath, storedName).Scan(&existingSource)
	if lookupErr != nil && lookupErr != pgx.ErrNoRows {
		log.Error().Err(lookupErr).Msg("Failed to inspect existing pipeline source")
		http.Error(w, "Failed to save pipeline", http.StatusInternalServerError)
		return
	}

	desiredSource := "database"
	if strings.EqualFold(existingSource, "git") {
		desiredSource = existingSource
	}

	query := `INSERT INTO pipelines (path, name, version, definition, source, updated_at) VALUES ($1, $2, $3, $4, $5, NOW())
			  ON CONFLICT (path, name) DO UPDATE SET version = $3, definition = $4, source = $5, updated_at = NOW()`
	_, err = a.db.Exec(context.Background(), query, dbPath, storedName, storedVersion, string(pipelineDef), desiredSource)
	if err != nil {
		log.Error().Err(err).Msg("Failed to save pipeline to database")
		http.Error(w, "Failed to save pipeline", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeletePipeline(w http.ResponseWriter, r *http.Request) {
	pipelineIdentifier := r.PathValue("pipelineName")
	pathPart, namePart, _, err := splitPipelineIdentifier(pipelineIdentifier)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, err = a.db.Exec(context.Background(), "DELETE FROM pipelines WHERE path = $1 AND name = $2", pathPart, namePart)
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

func isTerminalRunStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "failure", "cancelled", "failure (ignored)":
		return true
	default:
		return false
	}
}

func buildAgentContainerName(pipelineName, repoName, triggerEventID, runID string) string {
	sanitizedPipelineName := sanitizeInput(pipelineName)
	sanitizedTriggerID := sanitizeInput(strings.TrimSpace(triggerEventID))
	if sanitizedTriggerID == "" {
		sanitizedTriggerID = "no-trigger"
	} else if len(sanitizedTriggerID) > 8 {
		sanitizedTriggerID = sanitizedTriggerID[:8]
	}

	shortRunID := runID
	if len(shortRunID) > 8 {
		shortRunID = shortRunID[:8]
	}

	if strings.TrimSpace(repoName) != "" {
		sanitizedRepoName := sanitizeInput(repoName)
		return fmt.Sprintf("agent-%s-%s-%s-%s", sanitizedRepoName, sanitizedPipelineName, sanitizedTriggerID, shortRunID)
	}

	return fmt.Sprintf("agent-%s-%s-%s", sanitizedPipelineName, sanitizedTriggerID, shortRunID)
}

func normalizePipelineVersion(version string) string {
	sanitized := sanitizeInput(version)
	if sanitized == "" {
		return "latest"
	}
	return sanitized
}

func splitYAMLIdentifier(identifier string) (string, string, string, error) {
	trimmed := strings.TrimSpace(identifier)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "", "", "", fmt.Errorf("identifier cannot be empty")
	}

	normalized := filepath.ToSlash(trimmed)
	lower := strings.ToLower(normalized)
	var ext string
	switch {
	case strings.HasSuffix(lower, ".yaml"):
		ext = normalized[len(normalized)-len(".yaml"):]
		normalized = normalized[:len(normalized)-len(".yaml")]
	case strings.HasSuffix(lower, ".yml"):
		ext = normalized[len(normalized)-len(".yml"):]
		normalized = normalized[:len(normalized)-len(".yml")]
	}

	parts := strings.Split(normalized, "/")
	name := parts[len(parts)-1]
	if name == "" {
		return "", "", "", fmt.Errorf("identifier missing name")
	}

	var path string
	if len(parts) > 1 {
		path = strings.Join(parts[:len(parts)-1], "/")
	}
	if strings.Contains(path, "..") {
		return "", "", "", fmt.Errorf("identifier contains invalid path segments")
	}

	return path, name, ext, nil
}

func splitPipelineIdentifier(identifier string) (string, string, string, error) {
	return splitYAMLIdentifier(identifier)
}

func buildPipelineIdentifier(path, name string) string {
	if path == "" {
		return name
	}
	return path + "/" + name
}

func buildPipelineFilePath(path, name, ext string) string {
	if ext == "" {
		ext = ".yaml"
	}
	if path == "" {
		return name + ext
	}
	return path + "/" + name + ext
}

func splitStepIdentifier(identifier string) (string, string, string, error) {
	return splitYAMLIdentifier(identifier)
}

func buildStepIdentifier(path, name string) string {
	return buildPipelineIdentifier(path, name)
}

func parseEnvironmentFilePath(rel string) (string, bool, error) {
	lower := strings.ToLower(rel)
	if !strings.HasSuffix(lower, "env.yaml") && !strings.HasSuffix(lower, "env.yml") {
		return "", false, nil
	}

	base := filepath.Base(rel)
	if !strings.EqualFold(base, "env.yaml") && !strings.EqualFold(base, "env.yml") {
		return "", false, nil
	}

	envPath := strings.TrimSuffix(rel[:len(rel)-len(base)], "/")
	envPath = strings.Trim(envPath, "/")
	if envPath != "" {
		if strings.Contains(envPath, "..") {
			return "", false, fmt.Errorf("environment path contains invalid segments")
		}
		segments := strings.Split(envPath, "/")
		for _, segment := range segments {
			if segment == "" {
				return "", false, fmt.Errorf("environment path contains empty segments")
			}
		}
		envPath = filepath.ToSlash(envPath)
	}

	return envPath, true, nil
}

func parseGitHubRepoURL(raw string) (string, string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", fmt.Errorf("config repository URL is empty")
	}

	trimmed = strings.TrimSuffix(trimmed, ".git")

	if strings.HasPrefix(trimmed, "git@") {
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid config repository URL: %s", raw)
		}
		trimmed = parts[1]
	}

	if strings.Contains(trimmed, "://") {
		u, err := url.Parse(trimmed)
		if err != nil {
			return "", "", fmt.Errorf("invalid config repository URL: %w", err)
		}
		path := strings.Trim(u.Path, "/")
		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			return "", "", fmt.Errorf("invalid config repository URL: %s", raw)
		}
		return parts[len(parts)-2], parts[len(parts)-1], nil
	}

	trimmed = strings.Trim(trimmed, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid config repository URL: %s", raw)
	}

	return parts[len(parts)-2], parts[len(parts)-1], nil
}

func (a *App) handleListRuns(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT
		    run_id, pipeline_name, pipeline_path, pipeline_version, status, COALESCE(git_commit_sha, ''),
		    COALESCE(git_repo_owner, ''), COALESCE(git_repo_name, ''), started_at, finished_at, parent_run_id,
		    COALESCE(git_pusher_name, ''), COALESCE(git_ref, ''), COALESCE(git_target_ref, ''),
			COALESCE(pipeline_source, ''), COALESCE(trigger_event_id, '')
    	FROM pipeline_runs
	`
	args := []interface{}{}
	var conditions []string

	if groupIDStr := r.URL.Query().Get("groupId"); groupIDStr != "" {
		groupID, err := strconv.Atoi(groupIDStr)
		if err == nil {
			conditions = append(conditions, fmt.Sprintf("group_id = $%d", len(args)+1))
			args = append(args, groupID)
		}
	}

	if branchName := r.URL.Query().Get("branch"); branchName != "" {
		conditions = append(conditions, fmt.Sprintf("git_ref = $%d", len(args)+1))
		args = append(args, "refs/heads/"+branchName)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC LIMIT 300"

	rows, err := a.db.Query(context.Background(), query, args...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query runs from database")
		http.Error(w, "Failed to retrieve runs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	runsByBranch := make(map[string][]RunListItem)
	var allRuns []RunListItem
	for rows.Next() {
		var run RunListItem
		var startedAt, finishedAt sql.NullTime
		var commitSHA, repoOwner, repoName, pusherName, gitRef, gitTargetRef, pipelineSource, pipelineVersion, pipelinePath, triggerEventID sql.NullString
		err := rows.Scan(
			&run.RunID, &run.PipelineName, &pipelinePath, &pipelineVersion, &run.Status, &commitSHA,
			&repoOwner, &repoName, &startedAt, &finishedAt, &run.ParentRunID, &pusherName, &gitRef, &gitTargetRef, &pipelineSource, &triggerEventID,
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to scan run row")
			continue
		}
		run.PipelinePath = pipelinePath.String
		run.GitCommitSHA = commitSHA.String
		run.PipelineVersion = normalizePipelineVersion(pipelineVersion.String)
		run.GitRepoOwner = repoOwner.String
		run.GitRepoName = repoName.String
		run.GitPusherName = pusherName.String
		run.GitRef = gitRef.String
		run.GitTargetRef = gitTargetRef.String
		run.PipelineSource = pipelineSource.String
		run.TriggerEventID = triggerEventID.String
		if startedAt.Valid {
			run.StartedAt = startedAt.Time
			if finishedAt.Valid {
				run.FinishedAt = finishedAt.Time
				run.Duration = run.FinishedAt.Sub(run.StartedAt).Round(time.Second).String()
				run.IsComplete = true
			} else {
				run.Duration = time.Since(run.StartedAt).Round(time.Second).String()
				run.IsComplete = false
			}
		} else {
			run.IsComplete = true // If it hasn't even started, it's not "running"
		}
		allRuns = append(allRuns, run)
	}

	if r.URL.Query().Get("groupId") != "" {
		for _, run := range allRuns {
			branch := "Others"
			if run.GitRef != "" {
				branch = strings.TrimPrefix(run.GitRef, "refs/heads/")
			}
			runsByBranch[branch] = append(runsByBranch[branch], run)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(runsByBranch)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(allRuns)
	}
}

func (a *App) handleGetRunDetails(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")

	var run RunListItem
	var pipelineDefinition string
	var startedAt, finishedAt sql.NullTime
	var commitSHA, repoOwner, repoName, pusherName, gitRef, gitTargetRef, failureReason, pipelineSource, pipelineVersion, pipelinePath, triggerEventID sql.NullString
	err := a.db.QueryRow(context.Background(), `
		SELECT
			run_id, pipeline_name, pipeline_path, pipeline_version, status, COALESCE(git_commit_sha, ''),
			COALESCE(git_repo_owner, ''), COALESCE(git_repo_name, ''),
			started_at, finished_at, parent_run_id,
			COALESCE(git_pusher_name, ''), pipeline_definition, COALESCE(git_ref, ''), COALESCE(git_target_ref, ''),
			failure_reason, COALESCE(pipeline_source, ''), COALESCE(trigger_event_id, '')
		FROM pipeline_runs
		WHERE run_id = $1
	`, runID).Scan(
		&run.RunID, &run.PipelineName, &pipelinePath, &pipelineVersion, &run.Status, &commitSHA,
		&repoOwner, &repoName, &startedAt, &finishedAt,
		&run.ParentRunID, &pusherName, &pipelineDefinition, &gitRef, &gitTargetRef,
		&failureReason, &pipelineSource, &triggerEventID,
	)

	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to query run details from database")
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}
	run.PipelinePath = pipelinePath.String
	run.GitCommitSHA = commitSHA.String
	run.GitRepoOwner = repoOwner.String
	run.GitRepoName = repoName.String
	run.GitPusherName = pusherName.String
	run.GitRef = gitRef.String
	run.GitTargetRef = gitTargetRef.String
	run.FailureReason = failureReason.String
	run.PipelineSource = pipelineSource.String
	run.PipelineVersion = normalizePipelineVersion(pipelineVersion.String)
	run.TriggerEventID = triggerEventID.String
	if startedAt.Valid {
		run.StartedAt = startedAt.Time
		if finishedAt.Valid {
			run.FinishedAt = finishedAt.Time
			run.Duration = run.FinishedAt.Sub(run.StartedAt).Round(time.Second).String()
			run.IsComplete = true
		} else {
			run.Duration = time.Since(run.StartedAt).Round(time.Second).String()
			run.IsComplete = isTerminalRunStatus(run.Status)
		}
	} else {
		run.Duration = "0s"
		run.IsComplete = isTerminalRunStatus(run.Status)
	}

	// Calculate ETag based on RunID, Status, and timestamps
	etag := fmt.Sprintf(`"%s-%s-%d-%d"`, run.RunID, run.Status, run.StartedAt.Unix(), run.FinishedAt.Unix())
	w.Header().Set("ETag", etag)

	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	var parentRunInfo *ParentRunInfo
	if run.ParentRunID != nil && *run.ParentRunID != "" {
		var parentPipelineName, parentPipelineVersion, parentPipelinePath string
		err := a.db.QueryRow(context.Background(), `
            SELECT pipeline_name, pipeline_path, pipeline_version FROM pipeline_runs WHERE run_id = $1
        `, *run.ParentRunID).Scan(&parentPipelineName, &parentPipelinePath, &parentPipelineVersion)
		if err != nil {
			log.Error().Err(err).Str("parent_run_id", *run.ParentRunID).Msg("Failed to query parent pipeline name")
		} else {
			parentRunInfo = &ParentRunInfo{
				RunID:           *run.ParentRunID,
				PipelineName:    parentPipelineName,
				PipelinePath:    parentPipelinePath,
				PipelineVersion: normalizePipelineVersion(parentPipelineVersion),
			}
		}
	}

	childRuns := make([]RunListItem, 0)
	childRows, err := a.db.Query(context.Background(), `
		SELECT run_id, pipeline_name, pipeline_path, pipeline_version, status, started_at, finished_at, parent_step_name, COALESCE(trigger_event_id, '')
		FROM pipeline_runs
		WHERE parent_run_id = $1
		ORDER BY created_at ASC
	`, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to query child runs for details view")
	} else {
		defer childRows.Close()
		for childRows.Next() {
			var childRun RunListItem
			var childStartedAt, childFinishedAt sql.NullTime
			var parentStepName, childPipelineVersion, childPipelinePath, childTriggerEventID sql.NullString
			if err := childRows.Scan(&childRun.RunID, &childRun.PipelineName, &childPipelinePath, &childPipelineVersion, &childRun.Status, &childStartedAt, &childFinishedAt, &parentStepName, &childTriggerEventID); err != nil {
				log.Error().Err(err).Msg("Failed to scan child run row")
				continue
			}
			childRun.PipelinePath = childPipelinePath.String
			childRun.PipelineVersion = normalizePipelineVersion(childPipelineVersion.String)
			childRun.TriggerEventID = childTriggerEventID.String
			if childStartedAt.Valid {
				childRun.StartedAt = childStartedAt.Time
			}
			if childFinishedAt.Valid {
				childRun.FinishedAt = childFinishedAt.Time
				childRun.Duration = childRun.FinishedAt.Sub(childRun.StartedAt).Round(time.Second).String()
				childRun.IsComplete = true
			} else if childStartedAt.Valid {
				childRun.Duration = time.Since(childRun.StartedAt).Round(time.Second).String()
			}
			childRun.ParentStepName = parentStepName.String
			childRuns = append(childRuns, childRun)
		}
	}

	taskRows, err := a.db.Query(context.Background(), `
		SELECT task_id, step_name, task_name, status, exit_code, started_at, finished_at, task_index
		FROM task_runs
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

	var originalPipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(pipelineDefinition), &originalPipeline); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to parse original pipeline definition")
		http.Error(w, "Failed to parse pipeline definition", http.StatusInternalServerError)
		return
	}

	tempPipelineForResolving := originalPipeline
	resolvedPipeline, err := a.resolveStepIncludes(&tempPipelineForResolving)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to resolve includes for details view")
		resolvedPipeline = &originalPipeline
	}

	steps := make([]StepDetail, 0) // Initialize as an empty slice
	for _, pStep := range resolvedPipeline.Steps {
		stepName := pStep.GetName()
		stepTasks := tasksByStep[stepName]

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

			if slices.ContainsFunc(stepTasks, func(t TaskDetail) bool { return t.Status == "failure" && !strings.Contains(t.Status, "ignored") }) {
				status = "failure"
			} else if slices.ContainsFunc(stepTasks, func(t TaskDetail) bool { return t.Status == "running" }) {
				status = "running"
			} else if allTasksDone(stepTasks) {
				if slices.ContainsFunc(stepTasks, func(t TaskDetail) bool { return strings.Contains(t.Status, "ignored") }) {
					status = "failure (ignored)"
				} else {
					status = "success"
				}
			} else if slices.ContainsFunc(stepTasks, func(t TaskDetail) bool { return t.Status == "skipped" }) {
				status = "skipped"
			}
		}

		originalPStep, _ := findStepByName(originalPipeline.Steps, stepName)

		config := StepConfiguration{
			Image:            pStep.GetImage(),
			Include:          originalPStep.GetInclude(),
			Sync:             pStep.GetSync(),
			Secrets:          pStep.GetSecrets(),
			Volumes:          pStep.GetVolumes(),
			Variables:        pStep.GetVariables(),
			IgnoreFailure:    pStep.GetIgnoreFailure(),
			LlmOutputSharing: pStep.GetLlmOutputSharing(),
			Tasks:            pStep.GetTasks(),
		}

		steps = append(steps, StepDetail{
			Name:          stepName,
			Status:        status,
			DependsOn:     pStep.GetDependsOn(),
			Tasks:         stepTasks,
			Duration:      stepDuration.Round(time.Second).String(),
			Configuration: config,
		})
	}

	response := RunDetail{
		RunInfo:                run,
		Steps:                  steps,
		PipelineDefinition:     originalPipeline,
		PipelineDefinitionYAML: pipelineDefinition,
		ChildRuns:              childRuns,
		ParentRunInfo:          parentRunInfo,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func allTasksDone(tasks []TaskDetail) bool {
	if len(tasks) == 0 {
		return false
	}
	for _, t := range tasks {
		if t.Status != "success" && !strings.Contains(t.Status, "ignore") {
			return false
		}
	}
	return true
}

func (a *App) updateRunRecordWithFailure(runID uuid.UUID, reason string, gitContext map[string]string) {
	log.Error().Str("run_id", runID.String()).Msg(reason)
	_, err := a.db.Exec(context.Background(),
		"UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2",
		reason, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID.String()).Msg("Failed to update run record with failure reason")
	}
	if gitContext["check_run_id"] != "" {
		a.notifyGitBotOfFinalStatus("failure", "", "", reason, gitContext)
	}
}

func (a *App) resolveGroupIDForRepo(repoOwner, repoName string) (sql.NullInt32, error) {
	var groupID sql.NullInt32
	repoName = strings.TrimSpace(repoName)
	if repoName == "" {
		return groupID, nil
	}

	repoOwner = strings.TrimSpace(repoOwner)
	fullRepoName := repoName
	if repoOwner != "" {
		fullRepoName = fmt.Sprintf("%s/%s", repoOwner, repoName)
	}

	var existingID int32
	err := a.db.QueryRow(context.Background(), "SELECT id FROM groups WHERE name = $1", fullRepoName).Scan(&existingID)
	if err == pgx.ErrNoRows {
		if repoOwner != "" {
			err = a.db.QueryRow(context.Background(), "SELECT id FROM groups WHERE name = $1", repoName).Scan(&existingID)
			if err == nil {
				log.Info().Str("old_name", repoName).Str("new_name", fullRepoName).Msg("Found matching folder, renaming to claim it for the repository.")
				if _, updateErr := a.db.Exec(context.Background(), "UPDATE groups SET name = $1 WHERE id = $2", fullRepoName, existingID); updateErr != nil {
					log.Error().Err(updateErr).Msg("Failed to rename existing folder to claim it.")
					existingID = 0
				}
			} else if err == pgx.ErrNoRows {
				log.Info().Str("repo", fullRepoName).Msg("No existing folder found. Creating a new one at the root.")
				err = a.db.QueryRow(context.Background(), `INSERT INTO groups (name, parent_id) VALUES ($1, NULL) RETURNING id`, fullRepoName).Scan(&existingID)
			}
		} else {
			log.Info().Str("repo", repoName).Msg("No existing folder found. Creating a new one at the root.")
			err = a.db.QueryRow(context.Background(), `INSERT INTO groups (name, parent_id) VALUES ($1, NULL) RETURNING id`, repoName).Scan(&existingID)
		}
	}
	if err != nil && err != pgx.ErrNoRows {
		return groupID, err
	}
	if existingID != 0 {
		groupID.Int32 = existingID
		groupID.Valid = true
	}
	return groupID, nil
}

func (a *App) recordMissingPipelineRun(identifier string, pipelineVersion string, pipelineDef []byte, gitContext map[string]string, scopeValue, pipelineSource, summary string) {
	runID := uuid.New()
	pathPart, namePart, _, err := splitPipelineIdentifier(identifier)
	if err != nil {
		namePart = sanitizeInput(identifier)
		pathPart = ""
	}
	namePart = sanitizeInput(namePart)
	if namePart == "" {
		namePart = "missing-pipeline"
	}

	groupID, groupErr := a.resolveGroupIDForRepo(gitContext["repo_owner"], gitContext["repo_name"])
	if groupErr != nil {
		log.Error().Err(groupErr).Str("pipeline", identifier).Msg("Failed to resolve group for missing pipeline run")
	}

	var triggerEventIDSQL sql.NullString
	if gitContext != nil {
		id := strings.TrimSpace(gitContext["trigger_event_id"])
		if id == "" {
			id = deriveTriggerEventID(gitContext)
		}
		if id == "" {
			id = runID.String()
		}
		if id != "" {
			triggerEventIDSQL.String = id
			triggerEventIDSQL.Valid = true
			gitContext["trigger_event_id"] = id
		}
	}

	now := time.Now()
	_, err = a.db.Exec(context.Background(), `
		INSERT INTO pipeline_runs (
			run_id, pipeline_name, pipeline_path, pipeline_version, status,
			pipeline_definition, git_repo_owner, git_repo_name, git_clone_url, git_ssh_url,
			git_ref, git_target_ref, git_commit_sha, git_commit_url, git_commit_message,
			git_commit_author_name, git_commit_author_email, git_commit_author_username,
			git_pusher_name, git_pusher_email, git_check_run_id, group_id, trigger_event_id,
			scope, pipeline_source, started_at, finished_at, failure_reason
		) VALUES (
			$1, $2, $3, $4, 'failure', $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27
		)`,
		runID,
		namePart,
		pathPart,
		normalizePipelineVersion(pipelineVersion),
		string(pipelineDef),
		gitContext["repo_owner"],
		gitContext["repo_name"],
		gitContext["clone_url"],
		gitContext["ssh_url"],
		gitContext["ref"],
		gitContext["target_ref"],
		gitContext["commit_sha"],
		gitContext["commit_url"],
		gitContext["commit_message"],
		gitContext["commit_author_name"],
		gitContext["commit_author_email"],
		gitContext["commit_author_username"],
		gitContext["pusher_name"],
		gitContext["pusher_email"],
		gitContext["check_run_id"],
		groupID,
		triggerEventIDSQL,
		scopeValue,
		pipelineSource,
		now,
		now,
		summary,
	)
	if err != nil {
		log.Error().Err(err).Str("pipeline", identifier).Msg("Failed to record missing pipeline run")
		return
	}
}

func (a *App) launchAndRunPipeline(
	runID uuid.UUID,
	parentRunID string,
	parentRunnerID string,
	pipeline models.Pipeline,
	pipelineDef []byte,
	timeoutDuration time.Duration,
	gitContext map[string]string,
	parentHistory string,
	scope string,
	overrides map[string]string,
) {
	tx, err := a.db.Begin(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Failed to start database transaction for tasks")
		a.updateRunRecordWithFailure(runID, "Failed to start database transaction for tasks", gitContext)
		return
	}
	defer tx.Rollback(context.Background())

	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		if step.GetInclude() != "" {
			_, err := tx.Exec(context.Background(),
				"INSERT INTO task_runs (task_id, run_id, step_name, task_name, status, task_index) VALUES (gen_random_uuid(), $1, $2, $3, 'pending', $4)",
				runID, stepName, stepName, 1,
			)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to insert 'include' step %s as a task", stepName)
				// Don't need to call updateRunRecordWithFailure here as the transaction will be rolled back
				return
			}
		} else if tasks := step.GetTasks(); len(tasks) > 0 {
			for i, task := range tasks {
				_, err := tx.Exec(context.Background(),
					"INSERT INTO task_runs (task_id, run_id, step_name, task_name, status, task_index) VALUES (gen_random_uuid(), $1, $2, $3, 'pending', $4)",
					runID, stepName, task.Name, i+1,
				)
				if err != nil {
					log.Error().Err(err).Msgf("Failed to insert task %s for step %s", task.Name, stepName)
					return
				}
			}
		} else { // Legacy step
			_, err := tx.Exec(context.Background(),
				"INSERT INTO task_runs (task_id, run_id, step_name, task_name, status, task_index) VALUES (gen_random_uuid(), $1, $2, $3, 'pending', $4)",
				runID, stepName, stepName, 1,
			)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to insert step %s as a task", stepName)
				return
			}
		}
	}

	if err := tx.Commit(context.Background()); err != nil {
		log.Error().Err(err).Msg("Failed to commit tasks transaction")
		a.updateRunRecordWithFailure(runID, "Failed to commit tasks transaction", gitContext)
		return
	}

	go a.launchAgent(runID.String(), parentRunID, parentRunnerID, pipeline, pipelineDef, timeoutDuration, gitContext, parentHistory, scope, overrides)
}

func findStepByName(steps []models.PipelineStep, name string) (models.PipelineStep, bool) {
	for _, step := range steps {
		if step.GetName() == name {
			return step, true
		}
	}
	return models.PipelineStep{}, false
}

func (a *App) handleGitEvent(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Could not read request body", http.StatusInternalServerError)
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		http.Error(w, "Missing X-GitHub-Event header", http.StatusBadRequest)
		return
	}

	payload, err := github.ParseWebHook(eventType, body)
	if err != nil {
		http.Error(w, "Could not parse webhook", http.StatusBadRequest)
		return
	}

	var (
		owner, repo, ref, commitSHA string
		repoFullName                string
		headCommit                  *github.HeadCommit
		pusher                      *github.User
		cloneURL                    string
		sshURL                      string
		branchName                  string
		targetRef                   string
		commitURL                   string
		commitMessage               string
		commitAuthorName            string
		commitAuthorEmail           string
		commitAuthorUsername        string
		pusherName                  string
		pusherEmail                 string
		beforeSHA                   string
		isRerun                     bool
		rerunCheckRun               *github.CheckRun
	)
	deliveryID := strings.TrimSpace(r.Header.Get("X-GitHub-Delivery"))
	triggerEventID := deliveryID

	switch event := payload.(type) {
	case *github.PushEvent:
		if event.GetAfter() == "0000000000000000000000000000000000000000" {
			log.Info().Msg("Ignoring push event for branch deletion.")
			w.WriteHeader(http.StatusOK)
			return
		}
		if event.Repo == nil || event.Repo.Owner == nil || event.Repo.Owner.Login == nil ||
			event.Repo.Name == nil || event.Repo.FullName == nil || event.After == nil {
			log.Warn().Msg("Received push event with missing essential repository or commit data. Ignoring.")
			w.WriteHeader(http.StatusOK)
			return
		}
		repoFullName = event.GetRepo().GetFullName()
		owner = event.GetRepo().GetOwner().GetLogin()
		repo = event.GetRepo().GetName()
		commitSHA = event.GetAfter()
		ref = event.GetRef()
		if strings.HasPrefix(ref, "refs/heads/") {
			branchName = strings.TrimPrefix(ref, "refs/heads/")
		}
		headCommit = event.HeadCommit
		pusher = event.Pusher
		beforeSHA = event.GetBefore()

		if event.Repo.CloneURL != nil {
			cloneURL = event.Repo.GetCloneURL()
		}
		if event.Repo.SSHURL != nil {
			sshURL = event.Repo.GetSSHURL()
		}
		if headCommit != nil {
			if headCommit.URL != nil {
				commitURL = headCommit.GetURL()
			}
			if headCommit.Message != nil {
				message := headCommit.GetMessage()
				if idx := strings.Index(message, "\n"); idx >= 0 {
					commitMessage = message[:idx]
				} else {
					commitMessage = message
				}
			}
			if headCommit.Author != nil {
				if headCommit.Author.Name != nil {
					commitAuthorName = headCommit.Author.GetName()
				}
				if headCommit.Author.Email != nil {
					commitAuthorEmail = headCommit.Author.GetEmail()
				}
				if headCommit.Author.Login != nil {
					commitAuthorUsername = headCommit.Author.GetLogin()
				}
			}
		}
		if pusher != nil {
			if pusher.Name != nil {
				pusherName = pusher.GetName()
			}
			if pusher.Email != nil {
				pusherEmail = pusher.GetEmail()
			}
		}

		log.Info().Str("repo", repoFullName).Str("commit", commitSHA).Msg("Processing push event")
	case *github.PullRequestEvent:
		if event.GetAction() == "closed" {
			log.Info().Msg("Ignoring pull_request event with 'closed' action to prevent duplicate runs on merge.")
			w.WriteHeader(http.StatusOK)
			return
		}
		if event.Repo == nil || event.Repo.Owner == nil || event.Repo.Owner.Login == nil ||
			event.Repo.Name == nil || event.Repo.FullName == nil || event.PullRequest == nil || event.PullRequest.Head == nil || event.PullRequest.Head.SHA == nil {
			log.Warn().Msg("Received pull_request event with missing essential data. Ignoring.")
			w.WriteHeader(http.StatusOK)
			return
		}
		eventType = "pull_request"
		repoFullName = event.GetRepo().GetFullName()
		owner = event.GetRepo().GetOwner().GetLogin()
		repo = event.GetRepo().GetName()
		commitSHA = event.GetPullRequest().GetHead().GetSHA()
		ref = event.GetPullRequest().GetHead().GetRef()
		branchName = ref
		if strings.HasPrefix(branchName, "refs/heads/") {
			branchName = strings.TrimPrefix(branchName, "refs/heads/")
		} else if branchName != "" {
			ref = fmt.Sprintf("refs/heads/%s", branchName)
		}
		if prBase := event.GetPullRequest().GetBase(); prBase != nil {
			targetRef = prBase.GetRef()
			if targetRef != "" && !strings.HasPrefix(targetRef, "refs/") {
				targetRef = fmt.Sprintf("refs/heads/%s", targetRef)
			}
		}
		if prUser := event.GetPullRequest().GetUser(); prUser != nil {
			if name := prUser.GetName(); name != "" {
				pusherName = name
			} else {
				pusherName = prUser.GetLogin()
			}
			pusherEmail = prUser.GetEmail()
		}
		if pusherName == "" && commitAuthorName != "" {
			pusherName = commitAuthorName
		}
		log.Info().Str("repo", repoFullName).Str("commit", commitSHA).Msg("Processing pull_request event")
	case *github.CheckRunEvent:
		if event.GetAction() != "rerequested" {
			log.Info().Msgf("Received check_run event with action '%s', ignoring.", event.GetAction())
			w.WriteHeader(http.StatusOK)
			return
		}
		if event.Repo == nil || event.Repo.Owner == nil || event.Repo.Owner.Login == nil ||
			event.Repo.Name == nil || event.Repo.FullName == nil || event.CheckRun == nil || event.CheckRun.HeadSHA == nil {
			log.Warn().Msg("Received rerequested check_run event with missing essential data. Ignoring.")
			w.WriteHeader(http.StatusOK)
			return
		}
		repoFullName = event.GetRepo().GetFullName()
		owner = event.GetRepo().GetOwner().GetLogin()
		repo = event.GetRepo().GetName()
		commitSHA = event.GetCheckRun().GetHeadSHA()
		rerunCheckRun = event.GetCheckRun()
		isRerun = true
		if len(event.CheckRun.PullRequests) > 0 {
			eventType = "pull_request"
			pr := event.CheckRun.PullRequests[0]
			if pr != nil {
				if head := pr.GetHead(); head != nil {
					ref = head.GetRef()
				}
				if base := pr.GetBase(); base != nil {
					targetRef = base.GetRef()
					if targetRef != "" && !strings.HasPrefix(targetRef, "refs/") {
						targetRef = fmt.Sprintf("refs/heads/%s", targetRef)
					}
				}
			}
		} else {
			eventType = "push"
			if event.CheckRun.CheckSuite != nil && event.CheckRun.CheckSuite.HeadBranch != nil {
				ref = "refs/heads/" + event.CheckRun.CheckSuite.GetHeadBranch()
			} else {
				ref = commitSHA
			}
		}
	case *github.CheckSuiteEvent:
		if event.GetAction() != "rerequested" {
			log.Info().Msgf("Received check_suite event with action '%s', ignoring.", event.GetAction())
			w.WriteHeader(http.StatusOK)
			return
		}
		if event.Repo == nil || event.Repo.Owner == nil || event.Repo.Owner.Login == nil ||
			event.Repo.Name == nil || event.Repo.FullName == nil || event.CheckSuite == nil || event.CheckSuite.HeadSHA == nil {
			log.Warn().Msg("Received rerequested check_suite event with missing essential data. Ignoring.")
			w.WriteHeader(http.StatusOK)
			return
		}
		repoFullName = event.GetRepo().GetFullName()
		owner = event.GetRepo().GetOwner().GetLogin()
		repo = event.GetRepo().GetName()
		isRerun = true

		suiteInfo, err := a.findSuiteCheckRun(owner, repo, event.GetCheckSuite().GetID(), event.GetCheckSuite().GetHeadSHA())
		if err != nil {
			log.Error().Err(err).Msg("Failed to resolve check run for rerequested suite")
			http.Error(w, "Could not find check run for this suite.", http.StatusInternalServerError)
			return
		}
		rerunCheckRun = &github.CheckRun{
			ID:      github.Int64(suiteInfo.CheckRunID),
			HeadSHA: github.String(suiteInfo.HeadSHA),
		}
		commitSHA = suiteInfo.HeadSHA
		if suiteInfo.PullRequestHeadRef != "" {
			rerunCheckRun.PullRequests = []*github.PullRequest{{
				Head: &github.PullRequestBranch{Ref: github.String(suiteInfo.PullRequestHeadRef)},
			}}
		}

		if len(event.CheckSuite.PullRequests) > 0 {
			eventType = "pull_request"
			pr := event.CheckSuite.PullRequests[0]
			if pr != nil {
				if head := pr.GetHead(); head != nil {
					ref = head.GetRef()
				}
				if base := pr.GetBase(); base != nil {
					targetRef = base.GetRef()
					if targetRef != "" && !strings.HasPrefix(targetRef, "refs/") {
						targetRef = fmt.Sprintf("refs/heads/%s", targetRef)
					}
				}
			}
		} else if suiteInfo.PullRequestHeadRef != "" {
			eventType = "pull_request"
			ref = suiteInfo.PullRequestHeadRef
		} else {
			eventType = "push"
			if event.CheckSuite.HeadBranch != nil {
				ref = "refs/heads/" + event.CheckSuite.GetHeadBranch()
			} else if suiteInfo.HeadBranch != "" {
				ref = "refs/heads/" + suiteInfo.HeadBranch
			} else {
				ref = commitSHA
			}
		}
	default:
		log.Info().Msgf("Received unhandled event type '%s', ignoring.", eventType)
		w.WriteHeader(http.StatusOK)
		return
	}

	if eventType == "push" && branchName != "" {
		hasOpenPR, err := a.branchHasOpenPullRequest(owner, repo, branchName)
		if err != nil {
			log.Error().Err(err).Str("repo", repoFullName).Str("branch", branchName).Msg("Failed to check for open pull requests; proceeding with push event")
		} else if hasOpenPR {
			log.Info().Str("repo", repoFullName).Str("branch", branchName).Msg("Skipping push event because branch has open pull request")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Push event skipped: branch has open pull request."))
			return
		}
	}

	if owner == "" || repo == "" || commitSHA == "" {
		log.Warn().Msg("Skipping event due to missing owner, repo, or commit SHA")
		w.WriteHeader(http.StatusOK)
		return
	}
	if triggerEventID == "" {
		triggerEventID = deriveTriggerEventID(map[string]string{
			"repo_owner": owner,
			"repo_name":  repo,
			"ref":        ref,
			"commit_sha": commitSHA,
		})
	}
	if triggerEventID == "" {
		triggerEventID = uuid.NewString()
	}

	if eventType == "push" && !strings.HasPrefix(ref, "refs/") {
		var storedRef sql.NullString
		err := a.db.QueryRow(
			context.Background(),
			"SELECT git_ref FROM pipeline_runs WHERE git_repo_owner = $1 AND git_repo_name = $2 AND git_commit_sha = $3 ORDER BY created_at DESC LIMIT 1",
			owner, repo, commitSHA,
		).Scan(&storedRef)
		if err == nil && storedRef.Valid && strings.HasPrefix(storedRef.String, "refs/") {
			log.Info().Str("commit", commitSHA).Str("ref", storedRef.String).Msg("Recovered original ref for rerun event")
			ref = storedRef.String
		} else if err != nil && err != sql.ErrNoRows {
			log.Warn().Err(err).Str("commit", commitSHA).Msg("Failed to recover original ref for rerun event")
		}
	}

	if beforeSHA != "" && beforeSHA != "0000000000000000000000000000000000000000" {
		a.cancelStaleCheckRuns(owner, repo, beforeSHA)
	}

	manifest, pipelineSource, err := a.fetchTriggerManifest(owner, repo, commitSHA)
	if err != nil {
		if errors.Is(err, errManifestNotFound) {
			log.Info().Str("repo", repoFullName).Msg("No trigger manifest found; skipping event.")
			w.WriteHeader(http.StatusOK)
			return
		}
		log.Error().Err(err).Str("repo", repoFullName).Msg("Failed to load trigger manifest")
		http.Error(w, "Failed to load trigger manifest", http.StatusInternalServerError)
		return
	}

	pipelines, baseScope := findPipelinesForEvent(manifest, eventType, ref, repo)
	if len(pipelines) == 0 {
		log.Info().Str("repo", repoFullName).Str("ref", ref).Msg("No pipelines matched event.")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("No pipeline found for this event."))
		return
	}

	anyTriggered := false
	for _, p := range pipelines {
		originalPath := p.Path
		effectiveScope := baseScope

		if strings.HasPrefix(p.Path, "http://") || strings.HasPrefix(p.Path, "https://") {
			errMsg := fmt.Sprintf("Remote pipeline URLs are not supported (entry: %s)", p.Path)
			fallbackDef := fmt.Sprintf("name: remote-url\nsteps: []\n# %s", p.Path)
			checkRunID, createErr := a.createGitHubCheckRun(owner, repo, commitSHA, []byte(fallbackDef), pipelineSource)
			if createErr == nil {
				a.notifyImmediateCheckFailure(owner, repo, checkRunID, commitSHA, errMsg)
			}
			log.Warn().Str("repo", repoFullName).Msg(errMsg)
			continue
		}
		if p.Path == "" {
			log.Warn().Str("repo", repoFullName).Msg("Pipeline entry missing path; skipping.")
			continue
		}

		var pipelineYAML []byte
		pipelineSourceForCheck := pipelineSource // Start with the trigger's source
		var err error

		dbPath, dbName, _, parseErr := splitPipelineIdentifier(p.Path)
		if parseErr != nil {
			log.Warn().Err(parseErr).Str("pipeline", p.Path).Msg("Skipping pipeline due to invalid identifier")
			continue
		}

		// Attempt to fetch the pipeline from the database first to check for an override.
		pipelineYAML, err = a.fetchPipelineFromDB(dbPath, dbName)
		if err == nil {
			// Success: An override exists in the database.
			pipelineSourceForCheck = "database override"
			log.Info().Str("pipeline", p.Path).Msg("Using overridden pipeline definition from database.")
		} else if errors.Is(err, errPipelineNotFound) {
			if pipelineSource == "database override" {
				log.Warn().Str("pipeline", p.Path).Msg("Pipeline not found in database; falling back to repository definition.")
			}
			// Not found in DB, so fetch from the repository.
			repoPath := originalPath
			if !strings.HasPrefix(repoPath, ".nopsai/") {
				repoPath = ".nopsai/" + repoPath
			}
			repoSource := p
			repoSource.Path = repoPath
			pipelineYAML, err = a.requestGitBotPipeline(owner, repo, commitSHA, repoSource)
			if err == nil {
				pipelineSourceForCheck = "repository"
				p.Path = originalPath
			}
		}

		if err != nil {
			identifier := originalPath
			if errors.Is(err, errPipelineNotFound) && !strings.HasPrefix(identifier, ".nopsai/") {
				identifier = ".nopsai/" + identifier
			}
			summary := ""
			switch {
			case errors.Is(err, errPipelineNotFound):
				summary = fmt.Sprintf("Error: Could not locate pipeline `%s`.", identifier)
			default:
				summary = fmt.Sprintf("Error: %v", err)
			}

			fallbackDef := fmt.Sprintf("name: %s\nsteps: []", identifier)
			var createErr error
			checkRunID, createErr := a.createGitHubCheckRun(owner, repo, commitSHA, []byte(fallbackDef), pipelineSourceForCheck)
			if createErr != nil {
				log.Error().Err(createErr).Str("pipeline", identifier).Msg("Failed to create check run after pipeline retrieval error")
				http.Error(w, "Failed to create check run", http.StatusInternalServerError)
				return
			}
			a.notifyImmediateCheckFailure(owner, repo, checkRunID, commitSHA, summary)

			if errors.Is(err, errPipelineNotFound) {
				gitContextForRun := map[string]string{
					"repo_owner":             owner,
					"repo_name":              repo,
					"clone_url":              cloneURL,
					"ssh_url":                sshURL,
					"ref":                    ref,
					"target_ref":             targetRef,
					"commit_sha":             commitSHA,
					"commit_url":             commitURL,
					"commit_message":         commitMessage,
					"commit_author_name":     commitAuthorName,
					"commit_author_email":    commitAuthorEmail,
					"commit_author_username": commitAuthorUsername,
					"pusher_name":            pusherName,
					"pusher_email":           pusherEmail,
					"check_run_id":           strconv.FormatInt(checkRunID, 10),
					"trigger_event_id":       triggerEventID,
				}
				placeholderDef := fallbackDef
				if !strings.HasSuffix(placeholderDef, "\n") {
					placeholderDef += "\n"
				}
				placeholderDef += fmt.Sprintf("# %s\n", summary)
				a.recordMissingPipelineRun(originalPath, "", []byte(placeholderDef), gitContextForRun, effectiveScope, pipelineSourceForCheck, summary)
			}
			continue
		}

		var pipeline models.Pipeline
		if err := yaml.Unmarshal(pipelineYAML, &pipeline); err != nil {
			log.Error().Err(err).Msg("Failed to parse pipeline YAML")
			summary := fmt.Sprintf("Error: Pipeline definition is invalid. %v", err)
			checkRunID, createErr := a.createGitHubCheckRun(owner, repo, commitSHA, pipelineYAML, pipelineSourceForCheck)
			if createErr != nil {
				http.Error(w, "Failed to create check run", http.StatusInternalServerError)
				return
			}
			a.notifyImmediateCheckFailure(owner, repo, checkRunID, commitSHA, summary)
			continue
		}

		var checkRunIDStr string
		if isRerun && rerunCheckRun != nil {
			checkRunIDStr = strconv.FormatInt(rerunCheckRun.GetID(), 10)
		}

		headers := map[string]string{
			"X-Git-Repo-Owner":             owner,
			"X-Git-Repo-Name":              repo,
			"X-Git-Commit-SHA":             commitSHA,
			"X-Git-Check-Run-ID":           checkRunIDStr,
			"X-Git-Ref":                    ref,
			"X-Git-Target-Ref":             targetRef,
			"X-Nopsai-Scope":               effectiveScope,
			"X-Nopsai-Pipeline-Path":       dbPath,
			"X-Git-Clone-URL":              cloneURL,
			"X-Git-SSH-URL":                sshURL,
			"X-Git-Commit-URL":             commitURL,
			"X-Git-Commit-Message":         commitMessage,
			"X-Git-Commit-Author-Name":     commitAuthorName,
			"X-Git-Commit-Author-Email":    commitAuthorEmail,
			"X-Git-Commit-Author-Username": commitAuthorUsername,
			"X-Git-Pusher-Name":            pusherName,
			"X-Git-Pusher-Email":           pusherEmail,
			"X-Nopsai-Pipeline-Source":     pipelineSourceForCheck,
			"X-Nopsai-Trigger-Event-ID":    triggerEventID,
		}
		if isRerun {
			headers["X-Git-Rerun-Commit-SHA"] = commitSHA
		}

		var req *http.Request
		if pipelineSourceForCheck == "database override" {
			req = httptest.NewRequest(http.MethodPost, "/v1/run/"+originalPath, nil)
			req.SetPathValue("pipelineName", originalPath)
		} else {
			req = httptest.NewRequest(http.MethodPost, "/v1/run", bytes.NewReader(pipelineYAML))
			req.Header.Set("Content-Type", "application/x-yaml")
		}
		for key, value := range headers {
			if value != "" {
				req.Header.Set(key, value)
			}
		}

		recorder := httptest.NewRecorder()
		a.handleRunPipeline(recorder, req)
		result := recorder.Result()
		responseBody, _ := io.ReadAll(result.Body)
		result.Body.Close()

		if result.StatusCode != http.StatusCreated {
			summary := fmt.Sprintf("Failed to trigger Nopsai pipeline. The nopsai service responded with status %d.\n\nError: %s", result.StatusCode, strings.TrimSpace(string(responseBody)))
			if checkRunIDStr != "" {
				if parsedID, err := strconv.ParseInt(checkRunIDStr, 10, 64); err == nil {
					a.notifyImmediateCheckFailure(owner, repo, parsedID, commitSHA, summary)
				}
			}
			continue
		}

		anyTriggered = true
	}

	if anyTriggered {
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("Pipelines triggered."))
	} else {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("No pipelines triggered."))
	}
}

type runRequestPayload struct {
	Pipeline   string            `json:"pipeline"`
	Scope      string            `json:"scope"`
	Variables  map[string]string `json:"variables"`
	Definition string            `json:"definition"`
}

func (a *App) handleRunPipeline(w http.ResponseWriter, r *http.Request) {
	var pipeline models.Pipeline
	var pipelineDef []byte
	var err error

	parentRunID := r.Header.Get("X-Nopsai-Parent-Run-ID")
	parentRunnerID := r.Header.Get("X-Nopsai-Parent-Runner-ID")
	parentHistory := r.Header.Get("X-Nopsai-Parent-History")
	scope := strings.TrimSpace(r.Header.Get("X-Nopsai-Scope"))
	parentStepName := r.Header.Get("X-Nopsai-Parent-Step-Name")
	pipelineSource := r.Header.Get("X-Nopsai-Pipeline-Source")
	pipelineNameFromPath := r.PathValue("pipelineName")
	pipelinePathForRun := strings.TrimSpace(r.Header.Get("X-Nopsai-Pipeline-Path"))
	pipelinePathForRun = filepath.ToSlash(strings.Trim(pipelinePathForRun, "/"))
	if pipelinePathForRun == "." {
		pipelinePathForRun = ""
	}

	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	var payload runRequestPayload
	if strings.Contains(contentType, "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "Invalid JSON payload for run request", http.StatusBadRequest)
			return
		}
	}

	if scope == "" {
		scope = strings.TrimSpace(payload.Scope)
	}

	if pipelineNameFromPath == "" {
		pipelineNameFromPath = strings.TrimSpace(payload.Pipeline)
	}

	overrideVars := make(map[string]string)
	if len(payload.Variables) > 0 {
		var invalidKeys []string
		for key, value := range payload.Variables {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				continue
			}
			if !envKeyPattern.MatchString(trimmedKey) {
				invalidKeys = append(invalidKeys, trimmedKey)
				continue
			}
			overrideVars[trimmedKey] = value
		}
		if len(invalidKeys) > 0 {
			http.Error(w, fmt.Sprintf("Invalid variable override name(s): %s. Allowed characters: letters, numbers, underscores, dots, and hyphens.", strings.Join(invalidKeys, ", ")), http.StatusBadRequest)
			return
		}
	}

	rawDefinition := strings.TrimSpace(payload.Definition)
	usePayloadDefinition := rawDefinition != ""

	if strings.Contains(contentType, "application/json") && pipelineNameFromPath == "" && !usePayloadDefinition {
		http.Error(w, "Pipeline identifier or definition is required for JSON run requests", http.StatusBadRequest)
		return
	}

	if pipelineNameFromPath != "" {
		pathPart, namePart, _, parseErr := splitPipelineIdentifier(pipelineNameFromPath)
		if parseErr != nil {
			http.Error(w, parseErr.Error(), http.StatusBadRequest)
			return
		}
		pipelinePathForRun = pathPart
		if !usePayloadDefinition {
			var pipelineDefStr string
			err = a.db.QueryRow(context.Background(), "SELECT definition FROM pipelines WHERE path = $1 AND name = $2", pathPart, namePart).Scan(&pipelineDefStr)
			if err != nil {
				log.Error().Err(err).Str("pipeline", pipelineNameFromPath).Msg("Pipeline not found in database")
				http.Error(w, "Pipeline not found", http.StatusNotFound)
				return
			}
			pipelineDef = []byte(pipelineDefStr)
		}
	}

	if usePayloadDefinition {
		pipelineDef = []byte(rawDefinition)
	} else if pipelineNameFromPath == "" {
		pipelineDef, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusInternalServerError)
			return
		}
	}

	if len(pipelineDef) == 0 {
		http.Error(w, "Pipeline definition is required to start a run", http.StatusBadRequest)
		return
	}

	if err = yaml.Unmarshal(pipelineDef, &pipeline); err != nil {
		http.Error(w, fmt.Sprintf("Pipeline YAML is malformed: %v", err), http.StatusBadRequest)
		return
	}

	if pipelinePathForRun != "" && strings.Contains(pipelinePathForRun, "..") {
		http.Error(w, "Invalid pipeline path", http.StatusBadRequest)
		return
	}

	pipeline.Name = sanitizeInput(pipeline.Name)
	pipeline.Version = normalizePipelineVersion(pipeline.Version)

	gitContext := map[string]string{
		"repo_owner":             r.Header.Get("X-Git-Repo-Owner"),
		"repo_name":              r.Header.Get("X-Git-Repo-Name"),
		"clone_url":              r.Header.Get("X-Git-Clone-URL"),
		"ssh_url":                r.Header.Get("X-Git-SSH-URL"),
		"ref":                    r.Header.Get("X-Git-Ref"),
		"target_ref":             r.Header.Get("X-Git-Target-Ref"),
		"commit_sha":             r.Header.Get("X-Git-Commit-SHA"),
		"commit_url":             r.Header.Get("X-Git-Commit-URL"),
		"commit_message":         r.Header.Get("X-Git-Commit-Message"),
		"commit_author_name":     r.Header.Get("X-Git-Commit-Author-Name"),
		"commit_author_email":    r.Header.Get("X-Git-Commit-Author-Email"),
		"commit_author_username": r.Header.Get("X-Git-Commit-Author-Username"),
		"pusher_name":            r.Header.Get("X-Git-Pusher-Name"),
		"pusher_email":           r.Header.Get("X-Git-Pusher-Email"),
		"check_run_id":           r.Header.Get("X-Git-Check-Run-ID"),
		"trigger_event_id":       r.Header.Get("X-Nopsai-Trigger-Event-ID"),
	}

	rerunCommitSHA := r.Header.Get("X-Git-Rerun-Commit-SHA")
	if rerunCommitSHA != "" {
		log.Info().Str("commit_sha", rerunCommitSHA).Msg("Handling as a re-run: looking for original context.")
		var originalPusherName sql.NullString

		err := a.db.QueryRow(context.Background(),
			`SELECT git_pusher_name FROM pipeline_runs 
			 WHERE git_commit_sha = $1 AND git_repo_owner = $2 AND git_repo_name = $3
			 ORDER BY created_at DESC LIMIT 1`,
			rerunCommitSHA, gitContext["repo_owner"], gitContext["repo_name"]).Scan(&originalPusherName)

		if err != nil {
			log.Warn().Err(err).Str("commit_sha", rerunCommitSHA).Msg("Could not find original run to copy context from.")
		} else {
			if gitContext["pusher_name"] == "" && originalPusherName.Valid {
				gitContext["pusher_name"] = originalPusherName.String
				log.Info().Str("pusher_name", originalPusherName.String).Msg("Copied pusher name from original run.")
			}
		}
	}

	runID := uuid.New()

	var parentRunIDSQL sql.NullString
	if parentRunID != "" {
		parentRunIDSQL.String = parentRunID
		parentRunIDSQL.Valid = true
	}
	var parentStepNameSQL sql.NullString
	if parentStepName != "" {
		parentStepNameSQL.String = parentStepName
		parentStepNameSQL.Valid = true
	}
	var triggerEventIDSQL sql.NullString
	id := strings.TrimSpace(gitContext["trigger_event_id"])
	if id == "" {
		id = deriveTriggerEventID(gitContext)
	}
	if id == "" {
		id = runID.String()
	}
	if id != "" {
		triggerEventIDSQL.String = id
		triggerEventIDSQL.Valid = true
		gitContext["trigger_event_id"] = id
	}

	groupID, err := a.resolveGroupIDForRepo(gitContext["repo_owner"], gitContext["repo_name"])
	if err != nil {
		repoOwner := strings.TrimSpace(gitContext["repo_owner"])
		repoName := strings.TrimSpace(gitContext["repo_name"])
		repoFullName := repoName
		if repoOwner != "" {
			repoFullName = fmt.Sprintf("%s/%s", repoOwner, repoName)
		}
		log.Error().Err(err).Str("repo", repoFullName).Msg("Failed to find or create group for repository")
	}

	var checkRunIDSQL sql.NullInt64
	if val := gitContext["check_run_id"]; val != "" {
		if parsed, err := strconv.ParseInt(val, 10, 64); err == nil {
			checkRunIDSQL.Int64 = parsed
			checkRunIDSQL.Valid = true
		}
	}

	_, err = a.db.Exec(context.Background(),
		`INSERT INTO pipeline_runs (run_id, parent_run_id, pipeline_name, pipeline_path, pipeline_version, status, pipeline_definition,
			git_repo_owner, git_repo_name, git_clone_url, git_ssh_url, git_ref, git_target_ref,
			git_commit_sha, git_commit_url, git_commit_message, git_commit_author_name,
			git_commit_author_email, git_commit_author_username, git_pusher_name,
			git_pusher_email, git_check_run_id, group_id, parent_step_name, trigger_event_id, scope, pipeline_source)
			VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26)`,
		runID, parentRunIDSQL, pipeline.Name, pipelinePathForRun, pipeline.Version, string(pipelineDef),
		gitContext["repo_owner"], gitContext["repo_name"], gitContext["clone_url"], gitContext["ssh_url"], gitContext["ref"], gitContext["target_ref"],
		gitContext["commit_sha"], gitContext["commit_url"], gitContext["commit_message"], gitContext["commit_author_name"],
		gitContext["commit_author_email"], gitContext["commit_author_username"], gitContext["pusher_name"],
		gitContext["pusher_email"], checkRunIDSQL, groupID, parentStepNameSQL, triggerEventIDSQL, scope, pipelineSource,
	)

	if err != nil {
		log.Error().Err(err).Msg("Failed to insert initial run record")
		http.Error(w, "Failed to create run record", http.StatusInternalServerError)
		a.notifyGitBotOfFinalStatus("failure", "", "", "Failed to create initial run record in DB", gitContext)
		return
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
			"pipeline_definition": string(pipelineDef),
		}

		// Run git-bot notification in background and update DB with the new check_run_id
		go func(rID string) {
			body, _ := json.Marshal(payload)
			resp, err := a.postJSON(gitbotURL, body)
			if err != nil {
				log.Error().Err(err).Msg("Failed to request new check run from git-bot (async)")
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				log.Error().Int("status", resp.StatusCode).Msg("Git-bot returned non-OK status for child check run (async)")
				return
			}

			var respData map[string]int64
			if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
				log.Error().Err(err).Msg("Failed to decode git-bot response (async)")
				return
			}

			checkRunID := respData["check_run_id"]
			// Update the record with the obtained check_run_id
			_, err = a.db.Exec(context.Background(), "UPDATE pipeline_runs SET git_check_run_id = $1 WHERE run_id = $2", checkRunID, rID)
			if err != nil {
				log.Error().Err(err).Str("run_id", rID).Int64("check_run_id", checkRunID).Msg("Failed to update pipeline run with check_run_id (async)")
			}
		}(runID.String())
	}

	resolvedPipeline, err := a.resolveStepIncludes(&pipeline)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to resolve step includes: %v", err)
		http.Error(w, errMsg, http.StatusBadRequest)
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		return
	}

	if err := validatePipeline(resolvedPipeline); err != nil {
		errMsg := fmt.Sprintf("Pipeline validation failed: %v", err)
		http.Error(w, errMsg, http.StatusBadRequest)
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		return
	}

	resolvedPipelineDef, err := yaml.Marshal(resolvedPipeline)
	if err != nil {
		errMsg := "Failed to marshal resolved pipeline"
		http.Error(w, errMsg, http.StatusInternalServerError)
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		return
	}

	// Create or initialize GitHub check run without blocking the trigger path.
	a.ensureCheckRunAsync(runID, *resolvedPipeline, resolvedPipelineDef, gitContext, pipelineSource, rerunCommitSHA != "")

	timeoutStr := resolvedPipeline.Timeout
	if timeoutStr == "" {
		timeoutStr = a.getDefaultPipelineTimeout()
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

	if timeoutDuration > 0 {
		timeoutAt := time.Now().Add(timeoutDuration)
		_, err := a.db.Exec(context.Background(), "UPDATE pipeline_runs SET timeout_at = $1 WHERE run_id = $2", timeoutAt, runID)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID.String()).Msg("Failed to update run timeout")
		}
	}

	a.launchAndRunPipeline(runID, parentRunID, parentRunnerID, *resolvedPipeline, resolvedPipelineDef, timeoutDuration, gitContext, parentHistory, scope, overrideVars)

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Pipeline run created successfully with ID: " + runID.String()))
}

func (a *App) handleRerunPipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	originalRunID := r.PathValue("runID")

	var pipelineDef, pipelineName, pipelinePathDB, pipelineVersionDB, scope sql.NullString
	var gitContext = make(map[string]string)
	var timeoutAt sql.NullTime

	query := `SELECT
				pipeline_definition, pipeline_name, pipeline_path, pipeline_version, timeout_at, scope,
				git_repo_owner, git_repo_name, git_clone_url, git_ssh_url, git_ref, git_target_ref,
				git_commit_sha, git_commit_url, git_commit_message, git_commit_author_name,
				git_commit_author_email, git_commit_author_username, git_pusher_name,
				git_pusher_email, git_check_run_id, trigger_event_id, status
			  FROM pipeline_runs WHERE run_id = $1`

	var repoOwner, repoName, cloneURL, sshURL, ref, targetRef, commitSHA, commitURL, commitMessage,
		commitAuthorName, commitAuthorEmail, commitAuthorUsername, pusherName, pusherEmail, triggerEventID sql.NullString
	var originalStatus string
	var checkRunID sql.NullInt64

	err := a.db.QueryRow(context.Background(), query, originalRunID).Scan(
		&pipelineDef, &pipelineName, &pipelinePathDB, &pipelineVersionDB, &timeoutAt, &scope,
		&repoOwner, &repoName, &cloneURL, &sshURL, &ref, &targetRef, &commitSHA, &commitURL, &commitMessage,
		&commitAuthorName, &commitAuthorEmail, &commitAuthorUsername, &pusherName, &pusherEmail, &checkRunID, &triggerEventID, &originalStatus,
	)

	if err != nil {
		log.Error().Err(err).Str("original_run_id", originalRunID).Msg("Failed to find original run to rerun")
		http.Error(w, "Original pipeline run not found", http.StatusNotFound)
		return
	}

	if !isTerminalRunStatus(originalStatus) {
		http.Error(w, "Original pipeline run is still in progress; wait until it finishes before rerunning.", http.StatusConflict)
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
	if targetRef.Valid {
		gitContext["target_ref"] = targetRef.String
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
	if triggerEventID.Valid {
		gitContext["trigger_event_id"] = triggerEventID.String
	}

	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(pipelineDef.String), &pipeline); err != nil {
		http.Error(w, "Could not parse original pipeline definition", http.StatusInternalServerError)
		return
	}
	pipeline.Version = normalizePipelineVersion(pipeline.Version)
	if pipeline.Version == "latest" && pipelineVersionDB.Valid {
		pipeline.Version = normalizePipelineVersion(pipelineVersionDB.String)
	}

	var timeoutDuration time.Duration
	if timeoutAt.Valid {
		var originalCreatedAt time.Time
		err := a.db.QueryRow(context.Background(), "SELECT created_at FROM pipeline_runs WHERE run_id = $1", originalRunID).Scan(&originalCreatedAt)
		if err == nil {
			timeoutDuration = timeoutAt.Time.Sub(originalCreatedAt)
		}
	}

	runID := uuid.New()

	var groupID sql.NullInt32
	if name, ok := gitContext["repo_name"]; ok && name != "" {
		owner := gitContext["repo_owner"]
		fullRepoName := fmt.Sprintf("%s/%s", owner, name)
		var existingID int32
		err := a.db.QueryRow(context.Background(), "SELECT id FROM groups WHERE name = $1", fullRepoName).Scan(&existingID)
		if err == nil {
			groupID.Int32 = existingID
			groupID.Valid = true
		}
	}
	var triggerEventIDSQL sql.NullString
	newTriggerID := runID.String()
	if newTriggerID == "" {
		newTriggerID = uuid.New().String()
	}
	triggerEventIDSQL.String = newTriggerID
	triggerEventIDSQL.Valid = true
	gitContext["trigger_event_id"] = newTriggerID

	_, err = a.db.Exec(context.Background(),
		`INSERT INTO pipeline_runs (run_id, pipeline_name, pipeline_path, pipeline_version, status, pipeline_definition,
			git_repo_owner, git_repo_name, git_clone_url, git_ssh_url, git_ref, git_target_ref,
			git_commit_sha, git_commit_url, git_commit_message, git_commit_author_name,
			git_commit_author_email, git_commit_author_username, git_pusher_name,
			git_pusher_email, git_check_run_id, group_id, trigger_event_id, scope)
			VALUES ($1, $2, $3, $4, 'pending', $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)`,
		runID, pipeline.Name, pipelinePathDB.String, pipeline.Version, pipelineDef.String,
		gitContext["repo_owner"], gitContext["repo_name"], gitContext["clone_url"], gitContext["ssh_url"], gitContext["ref"], gitContext["target_ref"],
		gitContext["commit_sha"], gitContext["commit_url"], gitContext["commit_message"], gitContext["commit_author_name"],
		gitContext["commit_author_email"], gitContext["commit_author_username"], gitContext["pusher_name"],
		gitContext["pusher_email"], gitContext["check_run_id"], groupID, triggerEventIDSQL, scope.String,
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to insert initial record for rerun")
		http.Error(w, "Failed to create rerun record", http.StatusInternalServerError)
		return
	}

	resolvedPipeline, err := a.resolveStepIncludes(&pipeline)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to resolve step includes on rerun: %v", err)
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	if err := validatePipeline(resolvedPipeline); err != nil {
		errMsg := fmt.Sprintf("Pipeline validation failed on rerun: %v", err)
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	resolvedPipelineDef, err := yaml.Marshal(resolvedPipeline)
	if err != nil {
		errMsg := "Failed to marshal resolved pipeline on rerun"
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	a.launchAndRunPipeline(runID, "", "", *resolvedPipeline, resolvedPipelineDef, timeoutDuration, gitContext, "", scope.String, nil)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"runId":          runID.String(),
		"triggerEventId": newTriggerID,
	})
}

func (a *App) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	runIDStr := r.PathValue("runID")
	runUUID, err := uuid.Parse(runIDStr)
	if err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	var status string
	var pipelineName, repoName, triggerEventID, repoOwner, commitSHA sql.NullString
	var checkRunID sql.NullInt64

	err = a.db.QueryRow(context.Background(), `
		SELECT status, pipeline_name, git_repo_name, trigger_event_id, git_repo_owner, git_commit_sha, git_check_run_id
		FROM pipeline_runs
		WHERE run_id = $1`, runUUID).Scan(&status, &pipelineName, &repoName, &triggerEventID, &repoOwner, &commitSHA, &checkRunID)
	if err == pgx.ErrNoRows {
		http.Error(w, "Pipeline run not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Error().Err(err).Str("run_id", runUUID.String()).Msg("Failed to load run for cancellation")
		http.Error(w, "Failed to cancel run", http.StatusInternalServerError)
		return
	}

	statusLower := strings.ToLower(strings.TrimSpace(status))
	if statusLower == "success" || statusLower == "failure" || statusLower == "cancelled" {
		http.Error(w, "Run has already completed", http.StatusBadRequest)
		return
	}

	if _, err := a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'cancelled', finished_at = NOW() WHERE run_id = $1", runUUID); err != nil {
		log.Error().Err(err).Str("run_id", runUUID.String()).Msg("Failed to mark run as cancelled")
		http.Error(w, "Failed to cancel run", http.StatusInternalServerError)
		return
	}

	if _, err := a.db.Exec(context.Background(), "INSERT INTO pipeline_run_logs (run_id, line) VALUES ($1, $2)", runUUID, "Run cancelled by user."); err != nil {
		log.Warn().Err(err).Str("run_id", runUUID.String()).Msg("Failed to record cancellation log line")
	}

	if checkRunID.Valid {
		gitContext := map[string]string{
			"repo_owner":   repoOwner.String,
			"repo_name":    repoName.String,
			"commit_sha":   commitSHA.String,
			"check_run_id": strconv.FormatInt(checkRunID.Int64, 10),
		}
		a.notifyGitBotOfFinalStatus("cancelled", "", "", "Run cancelled by user.", gitContext)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
}

func (a *App) handleDeleteRun(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("runID"))
	if runID == "" {
		http.Error(w, "Run ID is required", http.StatusBadRequest)
		return
	}

	if _, err := uuid.Parse(runID); err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	commandTag, err := a.db.Exec(context.Background(), "DELETE FROM pipeline_runs WHERE run_id = $1", runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to delete pipeline run")
		http.Error(w, "Failed to delete pipeline run", http.StatusInternalServerError)
		return
	}

	if commandTag.RowsAffected() == 0 {
		http.Error(w, "Pipeline run not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteRepoBranchRuns(w http.ResponseWriter, r *http.Request) {
	repoOwner := strings.TrimSpace(r.PathValue("repoOwner"))
	repoName := strings.TrimSpace(r.PathValue("repoName"))
	branchParam := strings.TrimSpace(r.PathValue("branch"))

	if repoOwner == "" || repoName == "" {
		http.Error(w, "Repository owner and name are required", http.StatusBadRequest)
		return
	}
	if branchParam == "" {
		http.Error(w, "Branch name is required", http.StatusBadRequest)
		return
	}

	branch := strings.Trim(branchParam, " ")
	branch = strings.Trim(branch, "/")

	var commandTag pgconn.CommandTag
	var err error
	branchLower := strings.ToLower(branch)

	ctx := context.Background()

	if branchLower == "others" {
		commandTag, err = a.db.Exec(ctx,
			"DELETE FROM pipeline_runs WHERE git_repo_owner = $1 AND git_repo_name = $2 AND (git_ref IS NULL OR git_ref = '')",
			repoOwner, repoName,
		)
	} else {
		normalized := branch
		if strings.HasPrefix(normalized, "refs/") {
			normalized = strings.TrimPrefix(normalized, "refs/heads/")
		}
		refWithPrefix := "refs/heads/" + normalized

		commandTag, err = a.db.Exec(ctx,
			"DELETE FROM pipeline_runs WHERE git_repo_owner = $1 AND git_repo_name = $2 AND (git_ref = $3 OR git_ref = $4)",
			repoOwner, repoName, refWithPrefix, normalized,
		)
	}

	if err != nil {
		log.Error().Err(err).
			Str("repo_owner", repoOwner).
			Str("repo_name", repoName).
			Str("branch", branch).Msg("Failed to delete pipeline runs for branch")
		http.Error(w, "Failed to delete runs for branch", http.StatusInternalServerError)
		return
	}

	if commandTag.RowsAffected() == 0 {
		http.Error(w, "No pipeline runs found for the specified branch", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

	if update.Status == "running" {
		tx, err := a.db.Begin(context.Background())
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Msg("Failed to start transaction for task update")
			http.Error(w, "Failed to update task status", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(context.Background())

		_, err = tx.Exec(context.Background(), "UPDATE task_runs SET status = 'running', started_at = NOW() WHERE run_id = $1 AND step_name = $2 AND task_name = $3", runID, stepName, taskName)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Str("step", stepName).Str("task", taskName).Msg("Failed to update task start time")
			http.Error(w, "Failed to update task status", http.StatusInternalServerError)
			return
		}

		_, err = tx.Exec(context.Background(), "UPDATE pipeline_runs SET started_at = NOW() WHERE run_id = $1 AND started_at IS NULL", runID)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Msg("Failed to update run start time")
			http.Error(w, "Failed to update task status", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(context.Background()); err != nil {
			log.Error().Err(err).Str("run_id", runID).Msg("Failed to commit transaction for task update")
			http.Error(w, "Failed to update task status", http.StatusInternalServerError)
			return
		}
	} else {
		query := "UPDATE task_runs SET status = $1, exit_code = $2, finished_at = NOW() WHERE run_id = $3 AND step_name = $4 AND task_name = $5"
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
		includeValue := step.GetInclude()
		if !strings.HasPrefix(includeValue, "step:") {
			finalSteps = append(finalSteps, step)
			continue
		}

		// Handle step include
		includeIdentifier := strings.TrimPrefix(includeValue, "step:")
		stepPath, includeName, _, err := splitStepIdentifier(includeIdentifier)
		if err != nil {
			return nil, fmt.Errorf("invalid reusable step identifier '%s': %w", includeIdentifier, err)
		}

		var stepDefStr string
		err = a.db.QueryRow(context.Background(), "SELECT definition FROM steps WHERE path = $1 AND name = $2", stepPath, includeName).Scan(&stepDefStr)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch included step '%s': %w", includeIdentifier, err)
		}

		var includedStep models.PipelineStep
		if err := yaml.Unmarshal([]byte(stepDefStr), &includedStep); err != nil {
			return nil, fmt.Errorf("failed to parse included step '%s': %w", includeName, err)
		}

		// 1. Overwrite the name (for UI consistency)
		includedStep.SetName(step.GetName())

		// 2. Transfer metadata from the placeholder
		includedStep.SetDependsOn(step.GetDependsOn())
		includedStep.SetIgnoreFailure(step.GetIgnoreFailure())
		if llm := step.GetLlmOutputSharing(); llm != nil {
			includedStep.SetLlmOutputSharing(llm)
		}

		// 3. Overwrite specific fields if they are defined in the pipeline
		if vols := step.GetVolumes(); len(vols) > 0 {
			includedStep.SetVolumes(vols)
		}
		if secrets := step.GetSecrets(); len(secrets) > 0 {
			includedStep.SetSecrets(secrets)
		}
		if vars := step.GetVariables(); len(vars) > 0 {
			includedStep.SetVariables(vars)
		}

		finalSteps = append(finalSteps, includedStep)
	}

	pipeline.Steps = finalSteps
	return pipeline, nil
}

func (a *App) handleListReusableSteps(w http.ResponseWriter, r *http.Request) {
	includeSource := strings.EqualFold(r.URL.Query().Get("include_source"), "true")
	ctx := context.Background()

	var (
		rows pgx.Rows
		err  error
	)

	if includeSource {
		rows, err = a.db.Query(ctx, "SELECT path, name, COALESCE(source, 'database'), updated_at FROM steps ORDER BY path ASC, name ASC")
	} else {
		rows, err = a.db.Query(ctx, "SELECT path, name FROM steps ORDER BY path ASC, name ASC")
	}
	if err != nil {
		log.Error().Err(err).Msg("Failed to query reusable steps from database")
		http.Error(w, "Failed to retrieve reusable steps", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "application/json")

	if includeSource {
		type stepListItem struct {
			Identifier string    `json:"identifier"`
			Path       string    `json:"path"`
			Name       string    `json:"name"`
			Source     string    `json:"source"`
			UpdatedAt  time.Time `json:"updated_at"`
		}

		var items []stepListItem
		for rows.Next() {
			var path, name, source string
			var updatedAt time.Time
			if err := rows.Scan(&path, &name, &source, &updatedAt); err != nil {
				log.Error().Err(err).Msg("Failed to scan reusable step entry")
				http.Error(w, "Failed to process reusable steps", http.StatusInternalServerError)
				return
			}
			items = append(items, stepListItem{
				Identifier: buildStepIdentifier(path, name),
				Path:       path,
				Name:       name,
				Source:     source,
				UpdatedAt:  updatedAt,
			})
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(items)
		return
	}

	var stepNames []string
	for rows.Next() {
		var path, name string
		if err := rows.Scan(&path, &name); err != nil {
			log.Error().Err(err).Msg("Failed to scan reusable step entry")
			http.Error(w, "Failed to process reusable steps", http.StatusInternalServerError)
			return
		}
		stepNames = append(stepNames, buildStepIdentifier(path, name))
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(stepNames)
}

func (a *App) handleGetStepRoute(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.PathValue("stepPath"))
	if raw == "" {
		http.Error(w, "Step identifier is required", http.StatusBadRequest)
		return
	}

	normalized := strings.Trim(raw, "/")
	if strings.HasSuffix(normalized, "/usage") {
		trimmed := strings.TrimSuffix(normalized, "/usage")
		trimmed = strings.Trim(trimmed, "/")
		if trimmed == "" {
			http.Error(w, "Invalid step identifier", http.StatusBadRequest)
			return
		}
		a.respondStepUsage(w, trimmed)
		return
	}

	a.respondStepDefinition(w, normalized)
}

func (a *App) respondStepDefinition(w http.ResponseWriter, identifier string) {
	pathPart, namePart, _, err := splitStepIdentifier(identifier)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var stepDef string
	err = a.db.QueryRow(context.Background(), "SELECT definition FROM steps WHERE path = $1 AND name = $2", pathPart, namePart).Scan(&stepDef)
	if err != nil {
		log.Error().Err(err).Str("step", identifier).Msg("Reusable step not found in database")
		http.Error(w, "Reusable step not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(stepDef))
}

func (a *App) respondStepUsage(w http.ResponseWriter, identifier string) {
	pathPart, namePart, _, err := splitStepIdentifier(identifier)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	targetKey := buildStepIdentifier(pathPart, namePart)
	ctx := context.Background()
	rows, err := a.db.Query(ctx, "SELECT path, name, definition, COALESCE(source, 'database') FROM pipelines")
	if err != nil {
		log.Error().Err(err).Msg("Failed to query pipelines for step usage")
		http.Error(w, "Failed to load step usage", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type usageItem struct {
		Identifier  string `json:"identifier"`
		Name        string `json:"name"`
		Path        string `json:"path"`
		Source      string `json:"source"`
		Description string `json:"description,omitempty"`
	}

	var usage []usageItem
	for rows.Next() {
		var (
			pipelinePath string
			pipelineName string
			definition   string
			source       string
		)
		if err := rows.Scan(&pipelinePath, &pipelineName, &definition, &source); err != nil {
			log.Error().Err(err).Msg("Failed to scan pipeline row for step usage")
			continue
		}

		var pipeline models.Pipeline
		if err := yaml.Unmarshal([]byte(definition), &pipeline); err != nil {
			log.Error().Err(err).Str("pipeline", buildPipelineIdentifier(pipelinePath, pipelineName)).Msg("Failed to parse pipeline definition for usage lookup")
			continue
		}

		if pipelineIncludesStep(&pipeline, targetKey, namePart) {
			usage = append(usage, usageItem{
				Identifier:  buildPipelineIdentifier(pipelinePath, pipelineName),
				Name:        pipelineName,
				Path:        pipelinePath,
				Source:      source,
				Description: pipeline.Description,
			})
		}
	}

	if err := rows.Err(); err != nil {
		log.Error().Err(err).Msg("Pipeline iteration failed for step usage")
		// continue with whatever we collected
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(usage)
}

func pipelineIncludesStep(pipeline *models.Pipeline, targetIdentifier, targetName string) bool {
	if pipeline == nil {
		return false
	}
	targetKey := strings.TrimSpace(targetIdentifier)
	targetName = strings.TrimSpace(targetName)
	for _, step := range pipeline.Steps {
		includeValue := strings.TrimSpace(step.GetInclude())
		if includeValue == "" {
			continue
		}
		lower := strings.ToLower(includeValue)
		if strings.HasPrefix(lower, "pipeline:") {
			continue
		}
		if strings.HasPrefix(lower, "step:") {
			includeValue = strings.TrimSpace(includeValue[len("step:"):])
		}
		if strings.EqualFold(includeValue, targetKey) {
			return true
		}
		if targetName != "" && strings.EqualFold(includeValue, targetName) {
			return true
		}
	}
	return false
}

func (a *App) handleCreateOrUpdateReusableStep(w http.ResponseWriter, r *http.Request) {
	identifier := r.PathValue("stepName")
	var (
		dbPath       string
		expectedName string
	)
	if identifier != "" {
		pathPart, namePart, _, err := splitStepIdentifier(identifier)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		dbPath = pathPart
		expectedName = namePart
	}

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
	stepName := step.GetName()
	if stepName == "" {
		http.Error(w, "Validation failed: a reusable step must have a 'name' field in its definition.", http.StatusBadRequest)
		return
	}

	storedName := stepName
	if expectedName != "" && expectedName != storedName {
		errorMsg := fmt.Sprintf("Validation failed: the step name in the URL ('%s') must match the 'name' field in the YAML ('%s').", expectedName, storedName)
		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	query := `INSERT INTO steps (path, name, definition, source, updated_at) VALUES ($1, $2, $3, 'database', NOW())
			  ON CONFLICT (path, name) DO UPDATE SET definition = $3, source = 'database', updated_at = NOW()`
	_, err = a.db.Exec(context.Background(), query, dbPath, storedName, string(stepDef))
	if err != nil {
		log.Error().Err(err).Msg("Failed to save reusable step to database")
		http.Error(w, "Failed to save reusable step", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeleteReusableStep(w http.ResponseWriter, r *http.Request) {
	identifier := r.PathValue("stepName")
	pathPart, namePart, _, err := splitStepIdentifier(identifier)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, err = a.db.Exec(context.Background(), "DELETE FROM steps WHERE path = $1 AND name = $2", pathPart, namePart)
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
		err := a.db.QueryRow(context.Background(), "SELECT step_name, task_name FROM task_runs WHERE run_id = $1 AND status NOT IN ('success', 'pending', 'skipped', 'failure (ignored)', 'running') ORDER BY finished_at ASC, started_at ASC LIMIT 1", runID).Scan(&failedStep, &failedTask)
		if err != nil {
			log.Warn().Err(err).Str("run_id", runID).Msg("Could not determine the exact failed task for final status notification.")
		}
	}

	var gitContext = make(map[string]string)
	var repoOwner, repoName, commitSHA sql.NullString
	var checkRunID sql.NullInt64
	query := `SELECT git_repo_owner, git_repo_name, git_commit_sha, git_check_run_id FROM pipeline_runs WHERE run_id = $1`
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

	_, err = a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = $1, finished_at = NOW() WHERE run_id = $2 AND finished_at IS NULL", finalStatus, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to update final run status in DB from agent notification")
	}

	if gitContext["repo_owner"] != "" {
		// Run git-bot notification in background to prevent agent hang
		go a.notifyGitBotOfFinalStatus(finalStatus, failedStep, failedTask, "", gitContext)
	}

	w.WriteHeader(http.StatusOK)
}

func (a *App) handleGetRunStatus(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	var status string
	err := a.db.QueryRow(context.Background(), "SELECT status FROM pipeline_runs WHERE run_id = $1", runID).Scan(&status)
	if err != nil {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": status})
}

func (a *App) handleGetRunByCheckID(w http.ResponseWriter, r *http.Request) {
	checkRunID := r.PathValue("checkRunID")
	var runID string
	// Find the latest run for this check_run_id, as there could be multiple re-runs
	err := a.db.QueryRow(context.Background(),
		"SELECT run_id FROM pipeline_runs WHERE git_check_run_id = $1 ORDER BY created_at DESC LIMIT 1",
		checkRunID).Scan(&runID)
	if err != nil {
		http.Error(w, "Run not found for this check run ID", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"run_id": runID})
}

func (a *App) launchAgent(runID string, parentRunID string, parentRunnerID string, pipeline models.Pipeline, pipelineDef []byte, timeout time.Duration, gitContext map[string]string, parentHistory string, scope string, overrides map[string]string) {
	ctx := context.Background()

	secrets, err := a.prepareSecretsForPipeline(pipeline, gitContext, scope)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to prepare secrets for pipeline")
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", err.Error(), runID)
		if gitContext["repo_owner"] != "" {
			a.notifyGitBotOfFinalStatus("failure", "", "", err.Error(), gitContext)
		}
		return
	}

	finalVars, err := a.prepareVariablesForPipeline(pipeline, gitContext, scope, overrides)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to prepare scope variables for pipeline")
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", err.Error(), runID)
		if gitContext["repo_owner"] != "" {
			a.notifyGitBotOfFinalStatus("failure", "", "", err.Error(), gitContext)
		}
		return
	}

	if strings.TrimSpace(a.cfg.GeminiAPIKey) == "" || strings.TrimSpace(a.cfg.GeminiModel) == "" {
		reason := "Gemini configuration missing (GEMINI_API_KEY / GEMINI_MODEL)"
		log.Error().Str("run_id", runID).Msg(reason)
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", reason, runID)
		if gitContext["repo_owner"] != "" {
			a.notifyGitBotOfFinalStatus("failure", "", "", reason, gitContext)
		}
		return
	}

	agentImageName := a.getAgentImage()
	if agentImageName == "" {
		agentImageName = "nopsai-agent:latest"
	}

	dispatcherAddr := strings.TrimSpace(a.cfg.DispatcherAddress)
	if dispatcherAddr == "" {
		dispatcherAddr = "localhost:9090"
	}

	sharedVolumeName := fmt.Sprintf("vol-%s", runID)

	repoName := gitContext["repo_name"]
	triggerEventID := strings.TrimSpace(gitContext["trigger_event_id"])
	agentContainerName := buildAgentContainerName(pipeline.Name, repoName, triggerEventID, runID)
	preferredRunnerID := strings.TrimSpace(parentRunnerID)

	secretsJSON, err := json.Marshal(secrets)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to marshal secrets")
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", "Failed to marshal secrets", runID)
		return
	}

	envVars := []string{
		fmt.Sprintf("RUN_ID=%s", runID),
		fmt.Sprintf("PIPELINE_NAME=%s", pipeline.Name),
		fmt.Sprintf("PIPELINE_VERSION=%s", pipeline.Version),
		fmt.Sprintf("GEMINI_API_KEY=%s", a.cfg.GeminiAPIKey),
		fmt.Sprintf("GEMINI_MODEL=%s", a.cfg.GeminiModel),
		fmt.Sprintf("NOPSAI_API_URL=%s", a.cfg.AgentNopsaiAPIURL),
		fmt.Sprintf("LOG_LEVEL=%s", a.cfg.LogLevel),
		fmt.Sprintf("LOG_FORMAT=%s", a.cfg.LogFormat),
		fmt.Sprintf("PIPELINE_DEFINITION=%s", base64.StdEncoding.EncodeToString(pipelineDef)),
		fmt.Sprintf("SHARED_VOLUME_NAME=%s", sharedVolumeName),
		fmt.Sprintf("DOCKER_NETWORK_NAME=%s", a.getDockerNetworkName()),
		fmt.Sprintf("NOPSAI_SECRETS=%s", base64.StdEncoding.EncodeToString(secretsJSON)),
		fmt.Sprintf("DISPATCHER_ADDRESS=%s", dispatcherAddr),
	}
	if timeout > 0 {
		envVars = append(envVars, fmt.Sprintf("PIPELINE_TIMEOUT=%s", timeout.String()))
	}
	if a.getLLMAgentTimeout() != "" {
		envVars = append(envVars, fmt.Sprintf("LLM_AGENT_TIMEOUT=%s", a.getLLMAgentTimeout()))
	}
	if parentHistory != "" {
		envVars = append(envVars, fmt.Sprintf("PARENT_EXECUTION_HISTORY=%s", parentHistory))
	}
	if scope != "" {
		envVars = append(envVars, fmt.Sprintf("SCOPE=%s", scope))
	}
	if preferredRunnerID != "" {
		envVars = append(envVars, fmt.Sprintf("PARENT_RUNNER_ID=%s", preferredRunnerID))
	}

	variablesJSON, err := json.Marshal(finalVars)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to marshal variables")
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", "Failed to marshal variables", runID)
		return
	}

	envVars = append(envVars, fmt.Sprintf("NOPSAI_VARIABLES=%s", base64.StdEncoding.EncodeToString(variablesJSON)))

	for key, value := range gitContext {
		envKey := fmt.Sprintf("GIT_%s", strings.ToUpper(key))
		envVars = append(envVars, fmt.Sprintf("%s=%s", envKey, value))
	}

	initialLines := []string{}
	if triggerEventID != "" {
		initialLines = append(initialLines, fmt.Sprintf("Trigger Event ID: %s", triggerEventID))
	} else {
		initialLines = append(initialLines, "Trigger Event ID: N/A")
	}
	initialLines = append(initialLines, fmt.Sprintf("Preparing agent container %s with image %s", agentContainerName, agentImageName))

	appendLogs := func(lines ...string) {
		if len(lines) == 0 {
			return
		}
		dbBatch := &pgx.Batch{}
		for _, line := range lines {
			dbBatch.Queue("INSERT INTO pipeline_run_logs (run_id, line) VALUES ($1, $2)", runID, line)
		}
		if br := a.db.SendBatch(context.Background(), dbBatch); br != nil {
			if err := br.Close(); err != nil {
				log.Error().Err(err).Str("run_id", runID).Msg("Failed to write log lines")
			}
		}
	}

	appendLogs(initialLines...)

	affinityKey := triggerEventID
	if affinityKey == "" {
		affinityKey = strings.TrimSpace(parentRunID)
	}
	if affinityKey == "" {
		affinityKey = runID
	}

	job := &proto.JobRequest{
		RunId:              runID,
		PipelineName:       pipeline.Name,
		PipelineVersion:    pipeline.Version,
		PipelineDefinition: pipelineDef,
		Env:                envVars,
		AgentImage:         agentImageName,
		SharedVolumeName:   sharedVolumeName,
		DockerNetwork:      a.getDockerNetworkName(),
		AutoRemove:         a.getAutoRemovalAgentContainer(),
		ContainerName:      agentContainerName,
		Scope:              scope,
		NopsaiApiUrl:       strings.TrimSpace(a.cfg.AgentNopsaiAPIURL),
		TriggerEventId:     triggerEventID,
		RunnerAffinityKey:  affinityKey,
		PreferredRunnerId:  preferredRunnerID,
	}

	resp, err := a.dispatcher.SubmitJob(ctx, job)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to dispatch job to runner")
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", "Failed to dispatch job to runner", runID)
		if gitContext["repo_owner"] != "" {
			a.notifyGitBotOfFinalStatus("failure", "", "", "Failed to dispatch job to runner", gitContext)
		}
		appendLogs("Failed to dispatch job to runner: " + err.Error())
		return
	}

	switch resp.State {
	case proto.JobState_JOB_STATE_ASSIGNED:
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'running', started_at = COALESCE(started_at, NOW()) WHERE run_id = $1", runID)
		log.Info().Str("run_id", runID).Str("runner_id", resp.RunnerId).Msg("Job dispatched to runner")
		appendLogs(fmt.Sprintf("Dispatched to runner %s", resp.RunnerId))
	case proto.JobState_JOB_STATE_QUEUED:
		log.Info().Str("run_id", runID).Msg("No runner available; job queued")
		appendLogs("No runner available; job queued by dispatcher")
	default:
		log.Error().Str("run_id", runID).Str("state", resp.State.String()).Msg("Dispatcher rejected job")
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", "Dispatcher rejected job", runID)
		if gitContext["repo_owner"] != "" {
			a.notifyGitBotOfFinalStatus("failure", "", "", "Dispatcher rejected job", gitContext)
		}
		appendLogs("Dispatcher rejected job")
	}
}

func (a *App) notifyGitBotOfFinalStatus(status, failedStep, failedTask, summary string, gitContext map[string]string) {
	checkRunID, _ := strconv.ParseInt(gitContext["check_run_id"], 10, 64)
	if checkRunID == 0 {
		if runID := strings.TrimSpace(gitContext["run_id"]); runID != "" {
			_ = a.db.QueryRow(context.Background(), "SELECT git_check_run_id FROM pipeline_runs WHERE run_id = $1", runID).Scan(&checkRunID)
		}
	}
	if checkRunID == 0 {
		return
	}
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

	resp, err := a.postJSON(gitBotURL, body)
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
			t.task_index, (SELECT COUNT(*) FROM task_runs WHERE run_id = r.run_id),
			t.started_at, t.finished_at
		FROM pipeline_runs r JOIN task_runs t ON r.run_id = t.run_id
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
				if step.GetName() == stepName {
					for _, task := range step.GetTasks() {
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

	resp, err := a.postJSON(gitBotURL, body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to notify git-bot of task status")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status_code", resp.StatusCode).Msg("Received non-OK status from git-bot for task update")
	}
}

func (a *App) findEncryptedSecret(secretName, repoFullName, scope string) (string, bool, error) {
	var encryptedValue string

	if scope != "" {
		err := a.db.QueryRow(context.Background(), "SELECT value FROM secrets WHERE name = $1 AND repository_name = $2 AND scope = $3", secretName, repoFullName, scope).Scan(&encryptedValue)
		if err == nil {
			return encryptedValue, true, nil
		}
		if err != pgx.ErrNoRows {
			return "", false, err
		}

		err = a.db.QueryRow(context.Background(), "SELECT value FROM secrets WHERE name = $1 AND repository_name IS NULL AND scope = $2", secretName, scope).Scan(&encryptedValue)
		if err == nil {
			return encryptedValue, true, nil
		}
		if err == pgx.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}

	err := a.db.QueryRow(context.Background(), "SELECT value FROM secrets WHERE name = $1 AND repository_name = $2 AND scope IS NULL", secretName, repoFullName).Scan(&encryptedValue)
	if err == nil {
		return encryptedValue, true, nil
	}
	if err != pgx.ErrNoRows {
		return "", false, err
	}

	err = a.db.QueryRow(context.Background(), "SELECT value FROM secrets WHERE name = $1 AND repository_name IS NULL AND scope IS NULL", secretName).Scan(&encryptedValue)
	if err == nil {
		return encryptedValue, true, nil
	}
	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	return "", false, err
}

func findPipelinesForEvent(manifest models.Manifest, eventType, ref, repoName string) ([]models.PipelineSource, string) {
	for _, trigger := range manifest.Triggers {
		// Support "all" event type or specific match
		if trigger.On != eventType && trigger.On != "all" {
			continue
		}

		// Check for repo exceptions
		if len(trigger.SkipRepos) > 0 && isRepoSkipped(repoName, trigger.SkipRepos) {
			continue
		}

		if eventType == "push" {
			if strings.HasPrefix(ref, "refs/heads/") {
				branchName := strings.TrimPrefix(ref, "refs/heads/")
				branchIncluded := false
				if len(trigger.Branches) > 0 {
					branchIncluded = branchMatchesAnyPattern(branchName, trigger.Branches)
				} else if len(trigger.SkipBranches) > 0 {
					branchIncluded = true
				}
				// If "on: all", treat empty branches as "all branches"
				if trigger.On == "all" && len(trigger.Branches) == 0 {
					branchIncluded = true
				}

				if branchIncluded {
					if branchMatchesAnyPattern(branchName, trigger.SkipBranches) {
						continue
					}
					return trigger.Pipelines, trigger.Scope
				}
			} else if strings.HasPrefix(ref, "refs/tags/") {
				tagName := strings.TrimPrefix(ref, "refs/tags/")
				for _, pattern := range trigger.Tags {
					if matchBranchPattern(pattern, tagName) {
						return trigger.Pipelines, trigger.Scope
					}
				}
			}
		}

		if eventType == "pull_request" {
			return trigger.Pipelines, trigger.Scope
		}
	}
	return nil, ""
}

func branchMatchesAnyPattern(branchName string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchBranchPattern(pattern, branchName) {
			return true
		}
	}
	return false
}

func (a *App) getTriggerOverride(fullName string) (string, error) {
	var triggerDef string
	err := a.db.QueryRow(context.Background(), "SELECT trigger_definition FROM triggers WHERE repository_name = $1", fullName).Scan(&triggerDef)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return triggerDef, nil
}

func isRepoSkipped(repoName string, skipList []string) bool {
	for _, pattern := range skipList {
		if matchBranchPattern(pattern, repoName) {
			return true
		}
	}
	return false
}

func (a *App) fetchTriggerManifest(owner, repo, commitSHA string) (models.Manifest, string, error) {
	fullName := fmt.Sprintf("%s/%s", owner, repo)
	var manifest models.Manifest

	// 1. Try Specific Repo Override
	if overrideDef, err := a.getTriggerOverride(fullName); err != nil {
		return manifest, "", err
	} else if overrideDef != "" {
		if err := yaml.Unmarshal([]byte(overrideDef), &manifest); err != nil {
			return manifest, "", err
		}
		log.Info().Str("repository", fullName).Msg("Using trigger override from database")
		return manifest, "database override", nil
	}

	// 2. Try Owner-Wide "all" Override
	ownerAll := fmt.Sprintf("%s/all", owner)
	if overrideDef, err := a.getTriggerOverride(ownerAll); err != nil {
		return manifest, "", err
	} else if overrideDef != "" {
		if err := yaml.Unmarshal([]byte(overrideDef), &manifest); err != nil {
			return manifest, "", err
		}
		log.Info().Str("repository", fullName).Str("owner_trigger", ownerAll).Msg("Using owner-wide trigger override from database")
		return manifest, "database owner override", nil
	}

	// 3. Fallback to Git
	content, err := a.requestGitBotFile(owner, repo, commitSHA, ".nopsai/triggers.yaml", errManifestNotFound)
	if err != nil {
		return manifest, "", err
	}
	if err := yaml.Unmarshal([]byte(content), &manifest); err != nil {
		return manifest, "", err
	}
	return manifest, "git", nil
}

func (a *App) postJSON(url string, body []byte) (*http.Response, error) {
	client := a.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	return client.Post(url, "application/json", bytes.NewBuffer(body))
}

func (a *App) requestGitBotFile(owner, repo, ref, path string, notFoundErr error) (string, error) {
	payload := map[string]string{
		"owner": owner,
		"repo":  repo,
		"ref":   ref,
		"path":  path,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/github/file", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var out struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return "", err
		}
		return out.Content, nil
	case http.StatusNotFound:
		return "", notFoundErr
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("git-bot file request failed with status %d: %s", resp.StatusCode, string(respBody))
	}
}

func (a *App) requestGitBotDirectory(owner, repo, path string) (map[string]string, error) {
	payload := map[string]string{
		"owner": owner,
		"repo":  repo,
		"path":  path,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/github/contents", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var out struct {
			Files map[string]string `json:"files"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, err
		}
		if out.Files == nil {
			out.Files = map[string]string{}
		}
		return out.Files, nil
	case http.StatusNotFound:
		return map[string]string{}, nil
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("git-bot contents request for '%s' failed with status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
}

func (a *App) branchHasOpenPullRequest(owner, repo, branch string) (bool, error) {
	payload := map[string]string{
		"owner":  owner,
		"repo":   repo,
		"branch": branch,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/github/branch/has-open-pr", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var out struct {
			HasOpenPR bool `json:"has_open_pr"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return false, err
		}
		return out.HasOpenPR, nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return false, fmt.Errorf("branch open PR check failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
}

func (a *App) ensureConfigRepoAccessible(owner, repo string) error {
	payload := map[string]string{
		"owner": owner,
		"repo":  repo,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/github/repo/access", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		return fmt.Errorf("failed to verify config repository access: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	message := strings.TrimSpace(string(respBody))
	var errPayload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &errPayload); err == nil && errPayload.Error != "" {
		message = errPayload.Error
	}

	switch resp.StatusCode {
	case http.StatusNotFound:
		return fmt.Errorf("config repository '%s/%s' could not be found or Git Bot is not installed", owner, repo)
	case http.StatusForbidden:
		return fmt.Errorf("nopsai git-bot does not have permission to access config repository '%s/%s'", owner, repo)
	default:
		return fmt.Errorf("failed to verify config repository access for %s/%s (status %d): %s", owner, repo, resp.StatusCode, message)
	}
}

func (a *App) requestGitBotPipeline(owner, repo, ref string, source models.PipelineSource) ([]byte, error) {
	if source.Path != "" {
		content, err := a.requestGitBotFile(owner, repo, ref, source.Path, errPipelineNotFound)
		return []byte(content), err
	}

	payload := struct {
		Owner  string                `json:"owner"`
		Repo   string                `json:"repo"`
		Ref    string                `json:"ref"`
		Source models.PipelineSource `json:"source"`
	}{
		Owner:  owner,
		Repo:   repo,
		Ref:    ref,
		Source: source,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/github/pipeline", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var out struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, err
		}
		return []byte(out.Content), nil
	case http.StatusNotFound:
		return nil, errPipelineNotFound
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("git-bot pipeline request failed with status %d: %s", resp.StatusCode, string(respBody))
	}
}

func (a *App) findSuiteCheckRun(owner, repo string, suiteID int64, commitSHA string) (*suiteCheckRunResponse, error) {
	payload := map[string]interface{}{
		"owner":      owner,
		"repo":       repo,
		"suite_id":   suiteID,
		"commit_sha": commitSHA,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/checks/find-suite-run", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to request suite check run from git-bot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("git-bot suite lookup failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out suiteCheckRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("failed to decode git-bot suite lookup response: %w", err)
	}
	if out.CheckRunID == 0 || out.HeadSHA == "" {
		return nil, fmt.Errorf("git-bot returned incomplete suite check run data")
	}
	return &out, nil
}

func (a *App) createGitHubCheckRun(owner, repo, ref string, pipelineDef []byte, pipelineSource string) (int64, error) {
	payload := map[string]interface{}{
		"owner":               owner,
		"repo":                repo,
		"ref":                 ref,
		"pipeline_definition": string(pipelineDef),
		"pipeline_source":     pipelineSource,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/checks/create", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("git-bot check run creation failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var out struct {
		CheckRunID int64 `json:"check_run_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.CheckRunID, nil
}

func (a *App) ensureCheckRunAsync(runID uuid.UUID, pipeline models.Pipeline, resolvedPipelineDef []byte, gitCtx map[string]string, pipelineSource string, isRerun bool) {
	owner := strings.TrimSpace(gitCtx["repo_owner"])
	repo := strings.TrimSpace(gitCtx["repo_name"])
	commitSHA := strings.TrimSpace(gitCtx["commit_sha"])
	checkRunIDStr := strings.TrimSpace(gitCtx["check_run_id"])
	if owner == "" || repo == "" || commitSHA == "" {
		return
	}

	go func() {
		ctx := context.Background()
		if checkRunIDStr != "" {
			checkRunID, err := strconv.ParseInt(checkRunIDStr, 10, 64)
			if err != nil {
				log.Warn().Err(err).Str("check_run_id", checkRunIDStr).Msg("Invalid check run ID provided; skipping initialization")
				return
			}
			if isRerun {
				if err := a.initializeGitHubCheckRun(owner, repo, checkRunID, resolvedPipelineDef, pipeline.Name); err != nil {
					log.Error().Err(err).Int64("check_run_id", checkRunID).Msg("Failed to initialize rerun check run (async)")
				}
			}
			if _, err := a.db.Exec(ctx, "UPDATE pipeline_runs SET git_check_run_id = $1 WHERE run_id = $2", checkRunID, runID); err != nil {
				log.Error().Err(err).Str("run_id", runID.String()).Int64("check_run_id", checkRunID).Msg("Failed to persist provided check run ID (async)")
			}
			return
		}

		checkRunID, err := a.createGitHubCheckRun(owner, repo, commitSHA, resolvedPipelineDef, pipelineSource)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID.String()).Msg("Failed to create check run (async)")
			return
		}

		if _, err := a.db.Exec(ctx, "UPDATE pipeline_runs SET git_check_run_id = $1 WHERE run_id = $2", checkRunID, runID); err != nil {
			log.Error().Err(err).Str("run_id", runID.String()).Int64("check_run_id", checkRunID).Msg("Failed to persist check run ID (async)")
		} else {
			log.Info().Str("run_id", runID.String()).Int64("check_run_id", checkRunID).Msg("Attached check run to pipeline run (async)")
		}
	}()
}

func (a *App) initializeGitHubCheckRun(owner, repo string, checkRunID int64, pipelineDef []byte, pipelineName string) error {
	payload := map[string]interface{}{
		"owner":               owner,
		"repo":                repo,
		"check_run_id":        checkRunID,
		"pipeline_definition": string(pipelineDef),
		"pipeline_name":       pipelineName,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/checks/initialize", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("git-bot check run initialization failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (a *App) cancelStaleCheckRuns(owner, repo, beforeSHA string) {
	if beforeSHA == "" {
		return
	}
	payload := map[string]string{
		"owner":      owner,
		"repo":       repo,
		"before_sha": beforeSHA,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/v1/checks/cancel-stale", a.cfg.NopsaiGitBotAPIURL)
	resp, err := a.postJSON(url, body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to request stale check run cancellation")
		return
	}
	resp.Body.Close()
}

func (a *App) fetchPipelineFromDB(path, name string) ([]byte, error) {
	var pipelineDef string
	err := a.db.QueryRow(context.Background(), "SELECT definition FROM pipelines WHERE path = $1 AND name = $2", path, name).Scan(&pipelineDef)
	if err == pgx.ErrNoRows {
		return nil, errPipelineNotFound
	}
	if err != nil {
		return nil, err
	}
	return []byte(pipelineDef), nil
}

func (a *App) notifyImmediateCheckFailure(owner, repo string, checkRunID int64, commitSHA, summary string) {
	gitContext := map[string]string{
		"repo_owner":   owner,
		"repo_name":    repo,
		"check_run_id": strconv.FormatInt(checkRunID, 10),
		"commit_sha":   commitSHA,
	}
	a.notifyGitBotOfFinalStatus("failure", "", "", summary, gitContext)
}

func (a *App) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var group Group
	if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	group.Description = strings.TrimSpace(group.Description)

	query := `INSERT INTO groups (name, parent_id, description) VALUES ($1, $2, $3) RETURNING id`
	err := a.db.QueryRow(context.Background(), query, group.Name, group.ParentID, group.Description).Scan(&group.ID)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") {
			http.Error(w, "A folder or repository with this name already exists.", http.StatusConflict)
			return
		}
		log.Error().Err(err).Msg("Failed to create group")
		http.Error(w, "Failed to create group", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(group)
}

func (a *App) handleGetGroups(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(context.Background(), "SELECT id, name, parent_id, description FROM groups")
	if err != nil {
		log.Error().Err(err).Msg("Failed to query groups from database")
		http.Error(w, "Failed to retrieve groups", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var allGroups []Group
	groupMap := make(map[int]*Group)

	for rows.Next() {
		var g Group
		var parentID sql.NullInt32
		var description sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &parentID, &description); err != nil {
			log.Error().Err(err).Msg("Failed to scan group row")
			http.Error(w, "Error processing groups", http.StatusInternalServerError)
			return
		}
		if parentID.Valid {
			pid := int(parentID.Int32)
			g.ParentID = &pid
		}
		if description.Valid {
			g.Description = description.String
		}
		allGroups = append(allGroups, g)
	}
	if rows.Err() != nil {
		log.Error().Err(rows.Err()).Msg("Error iterating over group rows")
		http.Error(w, "Error retrieving groups", http.StatusInternalServerError)
		return
	}

	for i := range allGroups {
		groupMap[allGroups[i].ID] = &allGroups[i]
	}

	query := `
        SELECT g.id, MAX(r.started_at)
        FROM groups g
        JOIN pipeline_runs r ON g.id = r.group_id
        GROUP BY g.id
    `
	runRows, err := a.db.Query(context.Background(), query)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query last run times for groups")
	} else {
		defer runRows.Close()
		for runRows.Next() {
			var groupID int
			var lastRunAt sql.NullTime
			if err := runRows.Scan(&groupID, &lastRunAt); err == nil {
				if lastRunAt.Valid {
					if group, ok := groupMap[groupID]; ok {
						group.LastRunAt = &lastRunAt.Time
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(allGroups)
}

func (a *App) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.Atoi(r.PathValue("groupID"))
	if err != nil {
		http.Error(w, "Invalid group ID", http.StatusBadRequest)
		return
	}

	var group Group
	if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	group.Description = strings.TrimSpace(group.Description)

	query := `UPDATE groups SET name = $1, parent_id = $2, description = $3, updated_at = NOW() WHERE id = $4`
	_, err = a.db.Exec(context.Background(), query, group.Name, group.ParentID, group.Description, groupID)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") {
			http.Error(w, "A folder or repository with this name already exists.", http.StatusConflict)
			return
		}
		log.Error().Err(err).Msg("Failed to update group")
		http.Error(w, "Failed to update group", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *App) handleMoveGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.Atoi(r.PathValue("groupID"))
	if err != nil {
		http.Error(w, "Invalid group ID", http.StatusBadRequest)
		return
	}

	var payload struct {
		ParentID *int `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validation: Prevent moving a group into itself or its own children.
	if payload.ParentID != nil {
		if groupID == *payload.ParentID {
			http.Error(w, "Cannot move a folder into itself.", http.StatusBadRequest)
			return
		}

		var isChild bool
		query := `
			WITH RECURSIVE Descendants AS (
				SELECT id, parent_id FROM groups WHERE id = $1
				UNION ALL
				SELECT g.id, g.parent_id FROM groups g
				INNER JOIN Descendants d ON g.id = d.parent_id
			)
			SELECT EXISTS (SELECT 1 FROM Descendants WHERE id = $2)
		`
		err := a.db.QueryRow(context.Background(), query, *payload.ParentID, groupID).Scan(&isChild)
		if err != nil {
			log.Error().Err(err).Msg("Failed during ancestry check for group move")
			http.Error(w, "Server error during validation.", http.StatusInternalServerError)
			return
		}
		if isChild {
			http.Error(w, "Cannot move a folder into one of its own subfolders.", http.StatusBadRequest)
			return
		}
	}

	_, err = a.db.Exec(context.Background(), "UPDATE groups SET parent_id = $1, updated_at = NOW() WHERE id = $2", payload.ParentID, groupID)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") {
			http.Error(w, "A folder or repository with the same name already exists in the target location.", http.StatusConflict)
			return
		}
		log.Error().Err(err).Msg("Failed to update group parent")
		http.Error(w, "Failed to move group", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.Atoi(r.PathValue("groupID"))
	if err != nil {
		http.Error(w, "Invalid group ID", http.StatusBadRequest)
		return
	}

	_, err = a.db.Exec(context.Background(), "DELETE FROM groups WHERE id = $1", groupID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete group")
		http.Error(w, "Failed to delete group", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleListRepoBranches(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)

	query := `
		SELECT DISTINCT git_ref
		FROM pipeline_runs
		WHERE git_repo_name = $1 AND git_ref IS NOT NULL
		ORDER BY git_ref ASC
	`

	rows, err := a.db.Query(context.Background(), query, fullName)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query branches from database")
		http.Error(w, "Failed to retrieve branches", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var branches []string
	for rows.Next() {
		var branch sql.NullString
		if err := rows.Scan(&branch); err != nil {
			log.Error().Err(err).Msg("Failed to scan branch name")
			http.Error(w, "Failed to process branches", http.StatusInternalServerError)
			return
		}
		if branch.Valid {
			branches = append(branches, strings.TrimPrefix(branch.String, "refs/heads/"))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(branches)
}

func (a *App) handleIngestLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	runID := strings.TrimSpace(r.PathValue("runID"))
	if runID == "" {
		http.Error(w, "Run ID is required", http.StatusBadRequest)
		return
	}
	if _, err := uuid.Parse(runID); err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	var payload struct {
		Lines []string `json:"lines"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(payload.Lines) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	batch := &pgx.Batch{}
	for _, line := range payload.Lines {
		batch.Queue("INSERT INTO pipeline_run_logs (run_id, line) VALUES ($1, $2)", runID, line)
	}
	br := a.db.SendBatch(context.Background(), batch)
	if err := br.Close(); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to ingest log batch")
		http.Error(w, "Failed to persist logs", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleGetRunLogs(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	sinceLineStr := r.URL.Query().Get("since_line")
	var lastID int64 = 0
	if sinceLineStr != "" {
		if parsed, err := strconv.ParseInt(sinceLineStr, 10, 64); err == nil {
			lastID = parsed
		}
	}

	rows, err := a.db.Query(context.Background(), "SELECT id, timestamp, line FROM pipeline_run_logs WHERE run_id = $1 AND id > $2 ORDER BY id ASC", runID, lastID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to query logs for run")
		http.Error(w, "Failed to retrieve logs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var logs []LogLine
	for rows.Next() {
		var logLine LogLine
		if err := rows.Scan(&logLine.ID, &logLine.Timestamp, &logLine.Line); err != nil {
			log.Error().Err(err).Msg("Failed to scan log line")
			continue
		}
		logs = append(logs, logLine)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
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

	envFilePath := os.Getenv("ENV_FILE_PATH")
	if envFilePath == "" {
		envFilePath = filepath.Join(filepath.Dir(configPath), ".env")
	}

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

	dispatcherAddr := strings.TrimSpace(cfg.DispatcherAddress)
	if dispatcherAddr == "" {
		dispatcherAddr = "localhost:9090"
	}
	dispatcherConn, err := grpc.Dial(dispatcherAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal().Err(err).Str("addr", dispatcherAddr).Msg("Failed to connect to dispatcher")
	}
	defer dispatcherConn.Close()

	app := &App{
		db:          dbpool,
		cfg:         cfg,
		dispatcher:  proto.NewDispatcherServiceClient(dispatcherConn),
		encKey:      key[:],
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		store:       store.NewPGStore(dbpool),
		configPath:  configPath,
		envFilePath: envFilePath,
		configSyncStatus: ConfigSyncStatus{
			Status:  "idle",
			Message: "No configuration sync has been requested yet.",
		},
	}

	mux := http.NewServeMux()

	// Group Management
	mux.HandleFunc("POST /v1/git/events", app.handleGitEvent)
	mux.HandleFunc("POST /v1/groups", app.handleCreateGroup)
	mux.HandleFunc("GET /v1/groups", app.handleGetGroups)
	mux.HandleFunc("PUT /v1/groups/{groupID}", app.handleUpdateGroup)
	mux.HandleFunc("DELETE /v1/groups/{groupID}", app.handleDeleteGroup)
	mux.HandleFunc("PUT /v1/groups/{groupID}/move", app.handleMoveGroup)

	// Configuration Synchronization
	mux.HandleFunc("GET /v1/system/config", app.handleGetSystemConfig)
	mux.HandleFunc("PUT /v1/system/config", app.handleUpdateSystemConfig)
	mux.HandleFunc("GET /v1/system/config/sync", app.handleGetConfigSyncStatus)
	mux.HandleFunc("POST /v1/system/config/sync", app.handleConfigSync)
	mux.HandleFunc("POST /v1/internal/config/sync", app.handleConfigSync)
	mux.HandleFunc("GET /v1/system/dispatcher", app.handleDispatcherStatus)
	mux.HandleFunc("POST /v1/system/dispatcher/runners/{runnerID}/dispatch", app.handleUpdateRunnerDispatch)

	// Pipeline Management
	mux.HandleFunc("GET /v1/pipelines", app.handleListPipelines)
	mux.HandleFunc("GET /v1/pipelines/{pipelineName...}", app.handleGetPipeline)
	mux.HandleFunc("GET /v1/runs/{runID}/status", app.handleGetRunStatus)
	mux.HandleFunc("PUT /v1/pipelines/{pipelineName...}", app.handleCreateOrUpdatePipeline)
	mux.HandleFunc("DELETE /v1/pipelines/{pipelineName...}", app.handleDeletePipeline)
	mux.HandleFunc("GET /v1/steps", app.handleListReusableSteps)
	mux.HandleFunc("GET /v1/steps/{stepPath...}", app.handleGetStepRoute)
	mux.HandleFunc("PUT /v1/steps/{stepName...}", app.handleCreateOrUpdateReusableStep)
	mux.HandleFunc("DELETE /v1/steps/{stepName...}", app.handleDeleteReusableStep)
	mux.HandleFunc("GET /v1/overrides", app.handleListTriggerOverrides)
	mux.HandleFunc("GET /v1/overrides/{repoOwner}/{repoName}", app.handleGetTriggerOverride)
	mux.HandleFunc("PUT /v1/overrides/{repoOwner}/{repoName}", app.handleCreateOrUpdateTriggerOverride)
	mux.HandleFunc("DELETE /v1/overrides/{repoOwner}/{repoName}", app.handleDeleteTriggerOverride)
	mux.HandleFunc("GET /v1/secrets", app.handleListGeneralSecrets)
	mux.HandleFunc("GET /v1/secrets/scopes", app.handleListSecretScopes)
	mux.HandleFunc("GET /v1/secrets/{secretName}", app.handleGetGeneralSecretValue)
	mux.HandleFunc("PUT /v1/secrets/{secretName}", app.handleCreateOrUpdateGeneralSecret)
	mux.HandleFunc("DELETE /v1/secrets/{secretName}", app.handleDeleteGeneralSecret)
	mux.HandleFunc("GET /v1/repositories/{repoOwner}/{repoName}/secrets", app.handleListRepoSecrets)
	mux.HandleFunc("PUT /v1/repositories/{repoOwner}/{repoName}/secrets/{secretName}", app.handleCreateOrUpdateRepoSecret)
	mux.HandleFunc("DELETE /v1/repositories/{repoOwner}/{repoName}/secrets/{secretName}", app.handleDeleteRepoSecret)
	mux.HandleFunc("GET /v1/repositories/{repoOwner}/{repoName}/branches", app.handleListRepoBranches)
	mux.HandleFunc("GET /v1/variables", app.handleListGeneralVariables)
	mux.HandleFunc("GET /v1/variables/scopes", app.handleListVariableScopes)
	mux.HandleFunc("GET /v1/variables/{variableName}", app.handleGetGeneralVariableValue)
	mux.HandleFunc("PUT /v1/variables/{variableName}", app.handleCreateOrUpdateGeneralVariable)
	mux.HandleFunc("DELETE /v1/variables/{variableName}", app.handleDeleteGeneralVariable)
	mux.HandleFunc("GET /v1/repositories/{repoOwner}/{repoName}/variables", app.handleListRepoVariables)
	mux.HandleFunc("GET /v1/repositories/{repoOwner}/{repoName}/variables/{variableName}", app.handleGetRepoVariableValue)
	mux.HandleFunc("PUT /v1/repositories/{repoOwner}/{repoName}/variables/{variableName}", app.handleCreateOrUpdateRepoVariable)
	mux.HandleFunc("DELETE /v1/repositories/{repoOwner}/{repoName}/variables/{variableName}", app.handleDeleteRepoVariable)
	mux.HandleFunc("POST /v1/run", app.handleRunPipeline)
	mux.HandleFunc("POST /v1/run/{pipelineName...}", app.handleRunPipeline)
	mux.HandleFunc("GET /v1/runs", app.handleListRuns)
	mux.HandleFunc("GET /v1/runs/{runID}", app.handleGetRunDetails)
	mux.HandleFunc("DELETE /v1/runs/{runID}", app.handleDeleteRun)
	mux.HandleFunc("GET /v1/runs-by-check/{checkRunID}", app.handleGetRunByCheckID)
	mux.HandleFunc("POST /v1/runs/{runID}/rerun", app.handleRerunPipeline)
	mux.HandleFunc("POST /v1/runs/{runID}/cancel", app.handleCancelRun)
	mux.HandleFunc("POST /v1/runs/{runID}/finalize", app.handleFinalizeRun)
	mux.HandleFunc("POST /v1/runs/{runID}/steps/{stepName}/tasks/{taskName}", app.handleTaskUpdate)
	mux.HandleFunc("POST /v1/runs/{runID}/logs/ingest", app.handleIngestLogs)
	mux.HandleFunc("GET /v1/runs/{runID}/logs", app.handleGetRunLogs)
	mux.HandleFunc("DELETE /v1/repositories/{repoOwner}/{repoName}/branches/{branch...}", app.handleDeleteRepoBranchRuns)

	server := &http.Server{
		Addr:    cfg.NopsaiListenAddress,
		Handler: corsMiddleware(mux),
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

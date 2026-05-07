package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"

	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/routeauthz"
)

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

	type variableCandidate struct {
		repoName  string
		name      string
		source    string
		createdAt time.Time
		updatedAt time.Time
		resource  model.ResourceRef
	}

	var candidates []variableCandidate

	for rows.Next() {
		var name, source string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&name, &source, &createdAt, &updatedAt); err != nil {
			log.Error().Err(err).Msg("Failed to scan variable name")
			http.Error(w, "Failed to process variables", http.StatusInternalServerError)
			return
		}
		candidates = append(candidates, variableCandidate{
			name:      name,
			source:    source,
			createdAt: createdAt,
			updatedAt: updatedAt,
			resource:  routeauthz.BuildVariableResource("", scope, name),
		})
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
		varName = strings.TrimSpace(varName)
		if repo == "" || varName == "" {
			continue
		}
		candidates = append(candidates, variableCandidate{
			repoName:  repo,
			name:      varName,
			source:    source,
			createdAt: createdAt,
			updatedAt: updatedAt,
			resource:  routeauthz.BuildVariableResource(repo, scope, varName),
		})
	}

	resources := make([]model.ResourceRef, 0, len(candidates))
	for _, candidate := range candidates {
		resources = append(resources, candidate.resource)
	}
	allowedSet, err := a.allowedResourceSet(r, "variable.list_metadata", resources)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}

	nameSet := make(map[string]struct{})
	var names []string
	var items []variableListItem
	for _, candidate := range candidates {
		if _, ok := allowedSet[resourceKey(candidate.resource)]; !ok {
			continue
		}
		displayName := strings.TrimSpace(candidate.name)
		if candidate.repoName != "" {
			displayName = candidate.repoName + "/" + displayName
		}
		if displayName == "" {
			continue
		}
		if _, exists := nameSet[displayName]; exists {
			continue
		}
		nameSet[displayName] = struct{}{}
		if includeSource {
			items = append(items, variableListItem{
				Name:      displayName,
				Source:    normalizeVariableSourceKey(candidate.source),
				CreatedAt: candidate.createdAt.Format(time.RFC3339),
				UpdatedAt: candidate.updatedAt.Format(time.RFC3339),
			})
		} else {
			names = append(names, displayName)
		}
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
	if err := httpapi.DecodeJSON(r, &req); err != nil {
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
	var resources []model.ResourceRef
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Error().Err(err).Msg("Failed to scan variable name")
			http.Error(w, "Failed to process variables", http.StatusInternalServerError)
			return
		}
		names = append(names, name)
		resources = append(resources, routeauthz.BuildVariableResource(fullName, scope, name))
	}

	allowedSet, err := a.allowedResourceSet(r, "variable.list_metadata", resources)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}

	filtered := make([]string, 0, len(names))
	for idx, name := range names {
		if _, ok := allowedSet[resourceKey(resources[idx])]; !ok {
			continue
		}
		filtered = append(filtered, name)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(filtered)
}

func (a *App) handleCreateOrUpdateRepoVariable(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)
	variableName := r.PathValue("variableName")
	scope := r.URL.Query().Get("env")
	var req VariableRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
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

	type secretCandidate struct {
		repoName  string
		name      string
		createdAt time.Time
		updatedAt time.Time
		resource  model.ResourceRef
	}

	var candidates []secretCandidate

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
		candidates = append(candidates, secretCandidate{
			name:      name,
			createdAt: createdAt,
			updatedAt: updatedAt,
			resource:  routeauthz.BuildSecretResource("", env, name),
		})
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
		secretName = strings.TrimSpace(secretName)
		if repo == "" || secretName == "" {
			continue
		}
		candidates = append(candidates, secretCandidate{
			repoName:  repo,
			name:      secretName,
			createdAt: createdAt,
			updatedAt: updatedAt,
			resource:  routeauthz.BuildSecretResource(repo, env, secretName),
		})
	}
	rows.Close()

	resources := make([]model.ResourceRef, 0, len(candidates))
	for _, candidate := range candidates {
		resources = append(resources, candidate.resource)
	}
	allowedSet, err := a.allowedResourceSet(r, "secret.list_metadata", resources)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}

	const defaultSecretSource = "database"
	nameSet := make(map[string]struct{})
	var names []string
	var items []secretListItem
	for _, candidate := range candidates {
		if _, ok := allowedSet[resourceKey(candidate.resource)]; !ok {
			continue
		}
		displayName := strings.TrimSpace(candidate.name)
		if candidate.repoName != "" {
			displayName = candidate.repoName + "/" + displayName
		}
		if displayName == "" {
			continue
		}
		if _, exists := nameSet[displayName]; exists {
			continue
		}
		nameSet[displayName] = struct{}{}
		if includeSource {
			items = append(items, secretListItem{
				Name:      displayName,
				Source:    defaultSecretSource,
				CreatedAt: candidate.createdAt.Format(time.RFC3339),
				UpdatedAt: candidate.updatedAt.Format(time.RFC3339),
			})
		} else {
			names = append(names, displayName)
		}
	}

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
	if err := httpapi.DecodeJSON(r, &req); err != nil {
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
	var resources []model.ResourceRef
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Error().Err(err).Msg("Failed to scan secret name")
			http.Error(w, "Failed to process secrets", http.StatusInternalServerError)
			return
		}
		secretNames = append(secretNames, name)
		resources = append(resources, routeauthz.BuildSecretResource(fullName, env, name))
	}

	allowedSet, err := a.allowedResourceSet(r, "secret.list_metadata", resources)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}

	filtered := make([]string, 0, len(secretNames))
	for idx, name := range secretNames {
		if _, ok := allowedSet[resourceKey(resources[idx])]; !ok {
			continue
		}
		filtered = append(filtered, name)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(filtered)
}

func (a *App) handleCreateOrUpdateRepoSecret(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)
	secretName := r.PathValue("secretName")
	env := r.URL.Query().Get("env")
	var req SecretRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
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

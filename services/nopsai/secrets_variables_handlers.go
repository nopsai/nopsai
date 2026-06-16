package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	return a.secretCodec().Encrypt(text)
}

func (a *App) decrypt(text string) (string, error) {
	return a.secretCodec().Decrypt(text)
}

func (a *App) handleEncryptSecretForGitOps(w http.ResponseWriter, r *http.Request) {
	var req SecretRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	encryptedValue, err := a.encrypt(req.Value)
	if err != nil {
		log.Error().Err(err).Msg("Failed to encrypt GitOps secret value")
		http.Error(w, "Failed to encrypt secret", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"encrypted_value": encryptedValue,
		"algorithm":       "aes-256-gcm",
		"encoding":        "hex",
	}); err != nil {
		log.Warn().Err(err).Msg("Failed to encode GitOps secret encryption response")
	}
}

func (a *App) handleListGeneralVariables(w http.ResponseWriter, r *http.Request) {
	scope := runtimeScopeForStorage(r.URL.Query().Get("scope"))
	resourceScope := runtimeScopeForResource(scope)
	includeSource := strings.EqualFold(r.URL.Query().Get("include_source"), "true")
	var rows pgx.Rows
	var err error

	queryGeneral := "SELECT name, COALESCE(source, 'database'), created_at, updated_at FROM variables WHERE repository_name IS NULL AND %s ORDER BY name ASC"
	queryRepo := "SELECT repository_name, name, COALESCE(source, 'database'), created_at, updated_at FROM variables WHERE repository_name IS NOT NULL AND %s ORDER BY repository_name ASC, name ASC"

	ctx := context.Background()
	condition := runtimeScopeEqualsSQL("scope", 1, scope)
	args := []interface{}{scope}

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
			resource:  routeauthz.BuildVariableResource("", resourceScope, name),
		})
	}

	rows.Close()
	repoCondition := runtimeScopeEqualsSQL("scope", 1, scope)
	repoArgs := []interface{}{scope}

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
			resource:  routeauthz.BuildVariableResource(repo, resourceScope, varName),
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
	rows, err := a.db.Query(ctx, `
		SELECT COALESCE(repository_name, ''), scope, name
		FROM variables
		ORDER BY repository_name ASC NULLS FIRST, scope ASC, name ASC
	`)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query scope list from database")
		http.Error(w, "Failed to retrieve scopes", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type variableScopeCandidate struct {
		scope    string
		resource model.ResourceRef
	}

	var candidates []variableScopeCandidate

	for rows.Next() {
		var repoName, scope, name string
		if err := rows.Scan(&repoName, &scope, &name); err != nil {
			log.Error().Err(err).Msg("Failed to scan scope name")
			http.Error(w, "Failed to process scopes", http.StatusInternalServerError)
			return
		}
		repoName = strings.TrimSpace(repoName)
		scope = runtimeScopeForDisplay(scope)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		candidates = append(candidates, variableScopeCandidate{
			scope:    scope,
			resource: routeauthz.BuildVariableResource(repoName, runtimeScopeForResource(scope), name),
		})
	}

	if err := rows.Err(); err != nil {
		log.Error().Err(err).Msg("Failed during scope iteration")
		http.Error(w, "Failed to process scopes", http.StatusInternalServerError)
		return
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

	scopeSet := make(map[string]struct{})
	for _, candidate := range candidates {
		if _, ok := allowedSet[resourceKey(candidate.resource)]; !ok {
			continue
		}
		scopeSet[candidate.scope] = struct{}{}
	}

	scopes := make([]string, 0, len(scopeSet))
	for value := range scopeSet {
		scopes = append(scopes, value)
	}

	sort.Slice(scopes, func(i, j int) bool {
		ai, aj := scopes[i], scopes[j]
		if ai == defaultRuntimeScope && aj == defaultRuntimeScope {
			return false
		}
		if ai == defaultRuntimeScope {
			return true
		}
		if aj == defaultRuntimeScope {
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
	scope := runtimeScopeForStorage(r.URL.Query().Get("scope"))
	var req VariableRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	query := `INSERT INTO variables (name, value, repository_name, scope, source, config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at)
			  VALUES ($1, $2, NULL, $3, 'database', NULL, '', '', FALSE, NOW())
			  ON CONFLICT (name, repository_name, scope) DO UPDATE SET
			    value = EXCLUDED.value,
			    source = 'database',
			    config_repo_id = NULL,
			    config_source_path = '',
			    config_source_commit_sha = '',
			    managed_by_config_repo = FALSE,
			    updated_at = NOW()`
	_, err := a.db.Exec(context.Background(), query, variableName, req.Value, scope)

	if err != nil {
		log.Error().Err(err).Msg("Failed to save variable to database")
		http.Error(w, "Failed to save variable", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeleteGeneralVariable(w http.ResponseWriter, r *http.Request) {
	variableName := r.PathValue("variableName")
	scope := runtimeScopeForStorage(r.URL.Query().Get("scope"))
	_, err := a.db.Exec(context.Background(), "DELETE FROM variables WHERE name = $1 AND repository_name IS NULL AND "+runtimeScopeEqualsSQL("scope", 2, scope), variableName, scope)

	if err != nil {
		log.Error().Err(err).Msg("Failed to delete variable from database")
		http.Error(w, "Failed to delete variable", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleGetGeneralVariableValue(w http.ResponseWriter, r *http.Request) {
	variableName := r.PathValue("variableName")
	scope := runtimeScopeForStorage(r.URL.Query().Get("scope"))
	var value string
	err := a.db.QueryRow(context.Background(), "SELECT value FROM variables WHERE name = $1 AND repository_name IS NULL AND "+runtimeScopeEqualsSQL("scope", 2, scope)+" LIMIT 1", variableName, scope).Scan(&value)

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
	scope := runtimeScopeForStorage(r.URL.Query().Get("scope"))
	resourceScope := runtimeScopeForResource(scope)
	rows, err := a.db.Query(context.Background(), "SELECT name FROM variables WHERE repository_name = $1 AND "+runtimeScopeEqualsSQL("scope", 2, scope)+" ORDER BY name ASC", fullName, scope)

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
		resources = append(resources, routeauthz.BuildVariableResource(fullName, resourceScope, name))
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
	scope := runtimeScopeForStorage(r.URL.Query().Get("scope"))
	var req VariableRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	query := `INSERT INTO variables (name, value, repository_name, scope, source, config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at)
			  VALUES ($1, $2, $3, $4, 'database', NULL, '', '', FALSE, NOW())
			  ON CONFLICT (name, repository_name, scope) DO UPDATE SET
			    value = EXCLUDED.value,
			    source = 'database',
			    config_repo_id = NULL,
			    config_source_path = '',
			    config_source_commit_sha = '',
			    managed_by_config_repo = FALSE,
			    updated_at = NOW()`
	_, err := a.db.Exec(context.Background(), query, variableName, req.Value, fullName, scope)

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
	scope := runtimeScopeForStorage(r.URL.Query().Get("scope"))
	_, err := a.db.Exec(context.Background(), "DELETE FROM variables WHERE name = $1 AND repository_name = $2 AND "+runtimeScopeEqualsSQL("scope", 3, scope), variableName, fullName, scope)

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
	scope := runtimeScopeForStorage(r.URL.Query().Get("scope"))
	var value string
	err := a.db.QueryRow(context.Background(), "SELECT value FROM variables WHERE name = $1 AND repository_name = $2 AND "+runtimeScopeEqualsSQL("scope", 3, scope)+" LIMIT 1", variableName, fullName, scope).Scan(&value)

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

func (a *App) findVariableValue(varName, repoFullName, scope string) (string, string, bool, error) {
	var value string
	storageScope := runtimeScopeForStorage(scope)
	resourceScope := runtimeScopeForResource(storageScope)

	err := a.db.QueryRow(context.Background(), "SELECT value FROM variables WHERE name = $1 AND repository_name = $2 AND "+runtimeScopeEqualsSQL("scope", 3, storageScope)+" LIMIT 1", varName, repoFullName, storageScope).Scan(&value)
	if err == nil {
		return value, model.BuildNamedResourceID(repoFullName, resourceScope, varName), true, nil
	}
	if err != pgx.ErrNoRows {
		return "", "", false, err
	}

	err = a.db.QueryRow(context.Background(), "SELECT value FROM variables WHERE name = $1 AND repository_name IS NULL AND "+runtimeScopeEqualsSQL("scope", 2, storageScope)+" LIMIT 1", varName, storageScope).Scan(&value)
	if err == nil {
		return value, model.BuildNamedResourceID("", resourceScope, varName), true, nil
	}
	if err == pgx.ErrNoRows {
		return "", "", false, nil
	}
	return "", "", false, err
}

func (a *App) prepareVariablesForPipeline(runID string, pipeline models.Pipeline, gitContext map[string]string, scope string, overrides map[string]string) (map[string]string, error) {
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
	declaredRuntimeNames := make(map[string]string)
	requiredRefs := make([]models.ScopedRuntimeRef, 0, len(requiredVars))

	for _, rawName := range requiredVars {
		if strings.TrimSpace(rawName) == "" {
			continue
		}
		varRef, err := models.ParseScopedRuntimeRef(rawName, scope)
		if err != nil {
			return nil, fmt.Errorf("pipeline aborted: invalid variable reference '%s': %w", rawName, err)
		}
		if previousLookup, ok := declaredRuntimeNames[varRef.Name]; ok && previousLookup != varRef.LookupKey() {
			return nil, fmt.Errorf("pipeline aborted: variable references resolve to multiple values for runtime name '%s'", varRef.Name)
		}
		declaredRuntimeNames[varRef.Name] = varRef.LookupKey()
		requiredRefs = append(requiredRefs, varRef)
	}

	for _, varRef := range requiredRefs {
		// Allow ad-hoc overrides to satisfy or replace scoped values.
		if val, ok := cleanOverrides[varRef.Name]; ok {
			finalVars[varRef.Key()] = val
			continue
		}

		value, resourceID, found, err := a.findVariableValue(varRef.Name, repoFullName, varRef.Scope)
		if err != nil {
			if varRef.Scope != "" {
				return nil, fmt.Errorf("pipeline aborted: failed to resolve scope variable '%s': %w", varRef.Key(), err)
			}
			return nil, fmt.Errorf("pipeline aborted: failed to resolve variable '%s': %w", varRef.Key(), err)
		}

		if !found {
			if varRef.Scope != "" {
				return nil, fmt.Errorf("pipeline aborted: required scope variable '%s' not found for scope '%s'", varRef.Key(), varRef.DisplayScope())
			}
			return nil, fmt.Errorf("pipeline aborted: required scope variable '%s' not found in the default scope", varRef.Key())
		}

		if strings.TrimSpace(runID) != "" {
			if _, err := a.authorizeRunRuntimeResourceUse(context.Background(), runID, gitContext, "variable.use", grantResourceVariable, resourceID); err != nil {
				return nil, fmt.Errorf("pipeline aborted: %w", err)
			}
		}

		finalVars[varRef.Key()] = value
	}

	// Append any ad-hoc overrides that are not declared as required variables.
	for key, value := range cleanOverrides {
		if _, declared := declaredRuntimeNames[key]; declared {
			continue
		}
		if _, exists := finalVars[key]; !exists {
			finalVars[key] = value
		}
	}

	return finalVars, nil
}

func (a *App) handleListGeneralSecrets(w http.ResponseWriter, r *http.Request) {
	scope := runtimeScopeForStorage(r.URL.Query().Get("scope"))
	resourceScope := runtimeScopeForResource(scope)
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
		source    string
		createdAt time.Time
		updatedAt time.Time
		resource  model.ResourceRef
	}

	var candidates []secretCandidate

	condition := runtimeScopeEqualsSQL("scope", 1, scope)
	args := []interface{}{scope}

	generalQuery := fmt.Sprintf("SELECT name, COALESCE(source, 'database'), created_at, updated_at FROM secrets WHERE repository_name IS NULL AND %s ORDER BY name ASC", condition)
	rows, err := a.db.Query(ctx, generalQuery, args...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query general secrets from database")
		http.Error(w, "Failed to retrieve secrets", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var name, source string
		var createdAt, updatedAt time.Time
		if scanErr := rows.Scan(&name, &source, &createdAt, &updatedAt); scanErr != nil {
			rows.Close()
			log.Error().Err(scanErr).Msg("Failed to scan secret name")
			http.Error(w, "Failed to process secrets", http.StatusInternalServerError)
			return
		}
		candidates = append(candidates, secretCandidate{
			name:      name,
			source:    source,
			createdAt: createdAt,
			updatedAt: updatedAt,
			resource:  routeauthz.BuildSecretResource("", resourceScope, name),
		})
	}
	rows.Close()

	repoCondition := runtimeScopeEqualsSQL("scope", 1, scope)
	repoArgs := []interface{}{scope}
	repoQuery := fmt.Sprintf("SELECT repository_name, name, COALESCE(source, 'database'), created_at, updated_at FROM secrets WHERE repository_name IS NOT NULL AND %s ORDER BY repository_name ASC, name ASC", repoCondition)
	rows, err = a.db.Query(ctx, repoQuery, repoArgs...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query repository secrets from database")
		http.Error(w, "Failed to retrieve secrets", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var repoName, secretName, source string
		var createdAt, updatedAt time.Time
		if scanErr := rows.Scan(&repoName, &secretName, &source, &createdAt, &updatedAt); scanErr != nil {
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
			source:    source,
			createdAt: createdAt,
			updatedAt: updatedAt,
			resource:  routeauthz.BuildSecretResource(repo, resourceScope, secretName),
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
				Source:    normalizeVariableSourceKey(candidate.source),
				CreatedAt: candidate.createdAt.Format(time.RFC3339),
				UpdatedAt: candidate.updatedAt.Format(time.RFC3339),
			})
		} else {
			names = append(names, displayName)
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

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
		SELECT COALESCE(repository_name, ''), scope, name
		FROM secrets
		ORDER BY repository_name ASC NULLS FIRST, scope ASC, name ASC`)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query secret scopes from database")
		http.Error(w, "Failed to retrieve secret scopes", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type secretScopeCandidate struct {
		scope    string
		resource model.ResourceRef
	}

	var candidates []secretScopeCandidate

	for rows.Next() {
		var repoName, scope, name string
		if scanErr := rows.Scan(&repoName, &scope, &name); scanErr != nil {
			log.Error().Err(scanErr).Msg("Failed to scan secret scope row")
			http.Error(w, "Failed to process secret scopes", http.StatusInternalServerError)
			return
		}
		repoName = strings.TrimSpace(repoName)
		scope = runtimeScopeForDisplay(scope)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		candidates = append(candidates, secretScopeCandidate{
			scope:    scope,
			resource: routeauthz.BuildSecretResource(repoName, runtimeScopeForResource(scope), name),
		})
	}

	if err := rows.Err(); err != nil {
		log.Error().Err(err).Msg("Failed during secret scope iteration")
		http.Error(w, "Failed to process secret scopes", http.StatusInternalServerError)
		return
	}

	resources := make([]model.ResourceRef, 0, len(candidates))
	for _, candidate := range candidates {
		resources = append(resources, candidate.resource)
	}
	allowedSet, err := a.allowedResourceSet(r, "secret.list_metadata", resources)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}

	scopeCounts := make(map[string]int)
	for _, candidate := range candidates {
		if _, ok := allowedSet[resourceKey(candidate.resource)]; !ok {
			continue
		}
		scopeCounts[candidate.scope]++
	}

	scopes := make([]SecretScopeSummary, 0, len(scopeCounts))
	for envValue, count := range scopeCounts {
		scopes = append(scopes, SecretScopeSummary{Scope: envValue, SecretCount: count})
	}

	sort.Slice(scopes, func(i, j int) bool {
		aEnv := scopes[i].Scope
		bEnv := scopes[j].Scope
		if aEnv == defaultRuntimeScope && bEnv == defaultRuntimeScope {
			return false
		}
		if aEnv == defaultRuntimeScope {
			return true
		}
		if bEnv == defaultRuntimeScope {
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

	scope := runtimeScopeForStorage(r.URL.Query().Get("scope"))
	var query string
	var args []any
	query = "SELECT value FROM secrets WHERE name = $1 AND repository_name IS NULL AND " + runtimeScopeEqualsSQL("scope", 2, scope) + " LIMIT 1"
	args = []any{secretName, scope}

	var encryptedValue sql.NullString
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
	if !encryptedValue.Valid || strings.TrimSpace(encryptedValue.String) == "" {
		http.Error(w, "Secret value is not set", http.StatusConflict)
		return
	}

	value, err := a.decrypt(encryptedValue.String)
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
	scope := runtimeScopeForStorage(r.URL.Query().Get("scope"))
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

	query := `INSERT INTO secrets (name, value, repository_name, scope, source, config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at)
			  VALUES ($1, $2, NULL, $3, 'database', NULL, '', '', FALSE, NOW())
			  ON CONFLICT (name, repository_name, scope) DO UPDATE SET
			    value = EXCLUDED.value,
			    source = 'database',
			    config_repo_id = NULL,
			    config_source_path = '',
			    config_source_commit_sha = '',
			    managed_by_config_repo = FALSE,
			    updated_at = NOW()`
	_, err = a.db.Exec(context.Background(), query, secretName, encryptedValue, scope)

	if err != nil {
		log.Error().Err(err).Msg("Failed to save general secret to database")
		http.Error(w, "Failed to save secret", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *App) handleDeleteGeneralSecret(w http.ResponseWriter, r *http.Request) {
	secretName := r.PathValue("secretName")
	scope := runtimeScopeForStorage(r.URL.Query().Get("scope"))
	_, err := a.db.Exec(context.Background(), "DELETE FROM secrets WHERE name = $1 AND repository_name IS NULL AND "+runtimeScopeEqualsSQL("scope", 2, scope), secretName, scope)

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
	scope := runtimeScopeForStorage(r.URL.Query().Get("scope"))
	resourceScope := runtimeScopeForResource(scope)
	rows, err := a.db.Query(context.Background(), "SELECT name FROM secrets WHERE repository_name = $1 AND "+runtimeScopeEqualsSQL("scope", 2, scope)+" ORDER BY name ASC", fullName, scope)

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
		resources = append(resources, routeauthz.BuildSecretResource(fullName, resourceScope, name))
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
	scope := runtimeScopeForStorage(r.URL.Query().Get("scope"))
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

	query := `INSERT INTO secrets (name, value, repository_name, scope, source, config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at)
			  VALUES ($1, $2, $3, $4, 'database', NULL, '', '', FALSE, NOW())
			  ON CONFLICT (name, repository_name, scope) DO UPDATE SET
			    value = EXCLUDED.value,
			    source = 'database',
			    config_repo_id = NULL,
			    config_source_path = '',
			    config_source_commit_sha = '',
			    managed_by_config_repo = FALSE,
			    updated_at = NOW()`
	_, err = a.db.Exec(context.Background(), query, secretName, encryptedValue, fullName, scope)

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
	scope := runtimeScopeForStorage(r.URL.Query().Get("scope"))
	_, err := a.db.Exec(context.Background(), "DELETE FROM secrets WHERE name = $1 AND repository_name = $2 AND "+runtimeScopeEqualsSQL("scope", 3, scope), secretName, fullName, scope)

	if err != nil {
		log.Error().Err(err).Msg("Failed to delete repo secret from database")
		http.Error(w, "Failed to delete secret", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

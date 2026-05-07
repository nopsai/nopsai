package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/routeauthz"
)

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

	type pipelineEntry struct {
		identifier string
		source     string
		resource   model.ResourceRef
	}

	var entries []pipelineEntry

	for rows.Next() {
		var path, name, source string
		if err := rows.Scan(&path, &name, &source); err != nil {
			log.Error().Err(err).Msg("Failed to scan pipeline entry")
			http.Error(w, "Failed to process pipelines", http.StatusInternalServerError)
			return
		}
		entries = append(entries, pipelineEntry{
			identifier: buildPipelineIdentifier(path, name),
			source:     source,
			resource:   routeauthz.PipelineResource(path, name),
		})
	}

	resources := make([]model.ResourceRef, 0, len(entries))
	for _, entry := range entries {
		resources = append(resources, entry.resource)
	}
	allowedSet, err := a.allowedResourceSet(r, "pipeline.list", resources)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}

	var (
		pipelineNames []string
		pipelineItems []pipelineListItem
	)
	for _, entry := range entries {
		if _, ok := allowedSet[resourceKey(entry.resource)]; !ok {
			continue
		}
		if includeSource {
			pipelineItems = append(pipelineItems, pipelineListItem{ID: entry.identifier, Source: entry.source})
		} else {
			pipelineNames = append(pipelineNames, entry.identifier)
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

	type triggerEntry struct {
		name     string
		source   string
		resource model.ResourceRef
	}

	var (
		repoNames []string
		items     []triggerOverrideItem
		entries   []triggerEntry
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
			entries = append(entries, triggerEntry{
				name:     name,
				source:   source,
				resource: routeauthz.BuildTriggerResource("", name),
			})
		} else {
			if err := rows.Scan(&name); err != nil {
				log.Error().Err(err).Msg("Failed to scan repository name")
				http.Error(w, "Failed to process trigger overrides", http.StatusInternalServerError)
				return
			}
			entries = append(entries, triggerEntry{
				name:     name,
				resource: routeauthz.BuildTriggerResource("", name),
			})
		}
	}

	resources := make([]model.ResourceRef, 0, len(entries))
	for _, entry := range entries {
		resources = append(resources, entry.resource)
	}
	allowedSet, err := a.allowedResourceSet(r, "trigger.read", resources)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}

	for _, entry := range entries {
		if _, ok := allowedSet[resourceKey(entry.resource)]; !ok {
			continue
		}
		if includeSource {
			items = append(items, triggerOverrideItem{Name: entry.name, Source: entry.source})
			continue
		}
		repoNames = append(repoNames, entry.name)
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

	pipelineYAML, err := a.fetchPipelineFromDB(pathPart, namePart)
	if err == nil {
		w.Header().Set("Content-Type", "application/x-yaml")
		w.WriteHeader(http.StatusOK)
		w.Write(pipelineYAML)
		return
	}

	if !errors.Is(err, errPipelineNotFound) {
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

		var fetchErr error
		for _, extension := range extensions {
			pipelineYAML, fetchErr = fetchWithExtension(extension)
			if fetchErr == nil {
				break
			}
			if !errors.Is(fetchErr, errPipelineNotFound) {
				log.Error().Err(fetchErr).Str("pipeline", pipelineIdentifier).Msg("Failed to fetch pipeline from repository as fallback")
				http.Error(w, "Failed to fetch pipeline from repository", http.StatusBadGateway)
				return
			}
		}
		if fetchErr != nil {
			http.Error(w, "Pipeline not found in database or repository", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/x-yaml")
		w.WriteHeader(http.StatusOK)
		w.Write(pipelineYAML)
		return
	}

	log.Info().Str("pipeline", pipelineIdentifier).Msg("Pipeline not found in database and no git context for fallback")
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
	action := "pipeline.update"
	if lookupErr == pgx.ErrNoRows {
		action = "pipeline.create"
	}
	if !a.requireAAADecision(w, r, action, routeauthz.PipelineResource(dbPath, storedName)) {
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

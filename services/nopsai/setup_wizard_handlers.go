package nopsai

import (
	"archive/zip"
	"context"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
)

func (a *App) handleGetSetupStatus(w http.ResponseWriter, r *http.Request) {
	status, err := a.buildSetupStatus(r.Context())
	if err != nil {
		http.Error(w, "failed to build setup status", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (a *App) handleGetSetupTemplates(w http.ResponseWriter, r *http.Request) {
	profile := normalizeSetupProfile(r.URL.Query().Get("profile"))
	repositories := setupRepositoriesFromQuery(r.URL.Query())
	options := setupTemplateOptionsFromQuery(r.URL.Query())
	writeJSON(w, http.StatusOK, setupTemplatesResponse{
		Profile: profile,
		Files:   setupStarterTemplatesWithOptions(profile, repositories, options),
	})
}

func (a *App) handleDownloadSetupTemplates(w http.ResponseWriter, r *http.Request) {
	profile := normalizeSetupProfile(r.URL.Query().Get("profile"))
	repositories := setupRepositoriesFromQuery(r.URL.Query())
	options := setupTemplateOptionsFromQuery(r.URL.Query())
	files := setupStarterTemplatesWithOptions(profile, repositories, options)

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="nopsai-gitops-starter.zip"`)
	archive := zip.NewWriter(w)
	paths := make([]string, 0, len(files))
	for filePath := range files {
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	for _, filePath := range paths {
		cleanPath := strings.TrimPrefix(path.Clean("/"+filePath), "/")
		if cleanPath == "." || cleanPath == "" {
			continue
		}
		entry, err := archive.Create(cleanPath)
		if err != nil {
			_ = archive.Close()
			return
		}
		if _, err := entry.Write([]byte(files[filePath])); err != nil {
			_ = archive.Close()
			return
		}
	}
	_ = archive.Close()
}

func (a *App) handleBootstrapSetup(w http.ResponseWriter, r *http.Request) {
	var req setupBootstrapRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	req.Profile = normalizeSetupProfile(req.Profile)
	req.Repositories = normalizeSetupRepositories(req.Repositories)
	req.RepositoryTeams = normalizeSetupRepositoryTeams(req.RepositoryTeams, req.Repositories)
	req.Repositories = setupRepositoriesFromTeams(req.RepositoryTeams)
	req.Users = normalizeSetupUsers(req.Users)

	if err := a.validateSetupBootstrapRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	actor := actorIDFromRequest(r)
	details := map[string]int{}
	var messages []string
	warnings := setupBootstrapWarnings(req)
	var generatedSecrets []string
	requiresRestart := false
	var credentials []setupTemporaryCredential

	if req.GenerateSecrets {
		names, restart, err := a.generateSetupSecrets()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to generate local secrets: %v", err), http.StatusInternalServerError)
			return
		}
		generatedSecrets = names
		requiresRestart = restart
		if len(names) > 0 {
			messages = append(messages, fmt.Sprintf("Generated %d service secret value(s).", len(names)))
		}
	}

	if req.ConfigRepository != nil && strings.TrimSpace(req.ConfigRepository.RepoURL) != "" {
		input, err := configsync.BuildRepositoryInput(configsync.RepositoryInputRequest{
			RepoURL:  req.ConfigRepository.RepoURL,
			Branch:   req.ConfigRepository.Branch,
			BasePath: req.ConfigRepository.BasePath,
			Enabled:  req.ConfigRepository.Enabled,
		}, models.ConfigRepositoryScopeSystem, models.ConfigRepositorySystemGlobalID, actor)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		repo, err := a.store.CreateOrUpdateConfigRepository(r.Context(), input)
		if err != nil {
			writeConfigRepositoryStoreError(w, err, "failed to save global config repository")
			return
		}
		details["config_repositories_saved"] = 1
		messages = append(messages, "Global config repository saved.")
		if req.SyncConfigRepository && repo.Enabled {
			startedAt := time.Now()
			if err := a.store.UpdateConfigRepositorySyncStatus(r.Context(), repo.ID, "running", "Configuration synchronization started.", repo.LastSyncCommitSHA, &startedAt, nil); err != nil {
				writeConfigRepositoryStoreError(w, err, "failed to update sync status")
				return
			}
			repo.LastSyncStatus = "running"
			repo.LastSyncStartedAt = &startedAt
			go a.syncConfigRepository(context.Background(), repo, startedAt)
			messages = append(messages, "Global config repository sync started.")
		}
	}

	if req.Profile != setupProfileEmpty && req.SeedStarterDatabase {
		seedDetails, err := a.seedStarterDatabase(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for key, value := range seedDetails {
			details[key] += value
		}
		messages = append(messages, "Starter workspace resources seeded.")
	}

	if req.Profile != setupProfileEmpty && req.shouldSeedStarterLLMProfile() {
		if count, err := a.seedSetupLLMProfile(r.Context(), req.LLMProfile); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else if count > 0 {
			details["llm_profiles_seeded"] += count
		}
	}

	if req.MCPExamples && req.Profile != setupProfileProduction && req.Profile != setupProfileEmpty {
		if count, err := a.seedSetupMCPExamples(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else if count > 0 {
			details["mcp_examples_seeded"] += count
		}
	}

	if len(req.Users) > 0 && req.Profile != setupProfileProduction && req.Profile != setupProfileEmpty {
		created, err := a.seedSetupUsers(r.Context(), req.Users, req.RepositoryTeams, actor)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		credentials = created
		details["users_seeded"] += len(created)
	}

	if err := a.markSetupComplete(r.Context(), req.Profile); err != nil {
		http.Error(w, "failed to save setup completion state", http.StatusInternalServerError)
		return
	}

	status, err := a.buildSetupStatus(r.Context())
	if err != nil {
		http.Error(w, "setup completed but status could not be loaded", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, setupBootstrapResponse{
		Status:               status,
		Details:              details,
		GeneratedSecrets:     generatedSecrets,
		RequiresRestart:      requiresRestart,
		TemporaryCredentials: credentials,
		Messages:             messages,
		Warnings:             warnings,
	})
}

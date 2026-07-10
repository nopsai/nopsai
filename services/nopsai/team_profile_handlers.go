package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/mcpregistry"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	teamLLMDefaultProfileSetting   = "llm_default_profile"
	teamAgentDefaultProfileSetting = "agent_default_profile"
)

var teamProfileSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS team_profile_settings (
		team_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
		key TEXT NOT NULL,
		value TEXT NOT NULL DEFAULT '',
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (team_id, key)
	)`,
	`CREATE TABLE IF NOT EXISTS team_llm_profiles (
		team_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		provider TEXT NOT NULL,
		model TEXT NOT NULL DEFAULT '',
		base_url TEXT NOT NULL DEFAULT '',
		credential_ref TEXT NOT NULL DEFAULT '',
		allowed_scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
		reasoning TEXT NOT NULL DEFAULT '',
		thinking BOOLEAN,
		timeout_seconds INTEGER NOT NULL DEFAULT 0,
		max_tokens INTEGER NOT NULL DEFAULT 0,
		temperature DOUBLE PRECISION,
		extra JSONB NOT NULL DEFAULT '{}'::jsonb,
		source TEXT NOT NULL DEFAULT 'ui',
		config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
		managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (team_id, name)
	)`,
	`CREATE TABLE IF NOT EXISTS team_agent_profiles (
		team_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
		id TEXT NOT NULL,
		display_name TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		instructions TEXT NOT NULL,
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		source TEXT NOT NULL DEFAULT 'ui',
		config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
		managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (team_id, id)
	)`,
	`CREATE TABLE IF NOT EXISTS team_mcp_profiles (
		team_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		server_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
		allowed_scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
		source TEXT NOT NULL DEFAULT 'ui',
		config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
		managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (team_id, name)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_team_llm_profiles_config_repo ON team_llm_profiles(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_team_agent_profiles_config_repo ON team_agent_profiles(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_team_mcp_profiles_config_repo ON team_mcp_profiles(config_repo_id)`,
}

type teamLLMProfileView struct {
	llmProfileView
	Scope    string `json:"scope"`
	TeamID   int    `json:"team_id"`
	TeamPath string `json:"team_path"`
}

type teamLLMProfilesResponse struct {
	TeamID         int                  `json:"team_id"`
	TeamPath       string               `json:"team_path"`
	DefaultProfile string               `json:"default_profile"`
	Profiles       []teamLLMProfileView `json:"profiles"`
}

type teamAgentProfileView struct {
	models.AgentProfile
	Scope    string `json:"scope"`
	TeamID   int    `json:"team_id"`
	TeamPath string `json:"team_path"`
}

type teamAgentProfilesResponse struct {
	TeamID         int                    `json:"team_id"`
	TeamPath       string                 `json:"team_path"`
	DefaultProfile string                 `json:"default_profile"`
	Profiles       []teamAgentProfileView `json:"profiles"`
}

type teamMCPProfileView struct {
	models.MCPProfile
	Scope    string `json:"scope"`
	TeamID   int    `json:"team_id"`
	TeamPath string `json:"team_path"`
}

type teamMCPProfilesResponse struct {
	TeamID   int                  `json:"team_id"`
	TeamPath string               `json:"team_path"`
	Profiles []teamMCPProfileView `json:"profiles"`
}

func ensureTeamProfileSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin team profile schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	for idx, stmt := range teamProfileSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply team profile schema statement %d: %w", idx+1, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit team profile schema transaction: %w", err)
	}
	return nil
}

func (a *App) handleListTeamLLMProfiles(w http.ResponseWriter, r *http.Request) {
	record, ok := a.resolveAuthorizedTeamProfile(w, r, false)
	if !ok {
		return
	}
	response, err := a.buildTeamLLMProfilesResponse(r.Context(), record)
	if err != nil {
		http.Error(w, "failed to load team LLM profiles", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, response)
}

func (a *App) handleReplaceTeamLLMProfiles(w http.ResponseWriter, r *http.Request) {
	record, ok := a.resolveAuthorizedTeamProfile(w, r, true)
	if !ok {
		return
	}
	var payload llmProfilesRequest
	if err := httpapi.DecodeJSON(r, &payload); err != nil {
		http.Error(w, "invalid LLM profiles payload", http.StatusBadRequest)
		return
	}
	defaultProfile, profiles, err := parseTeamLLMProfilesPayload(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.ensureLLMProfileCredentialReferences(r.Context(), profiles, credentialActorFromContext(r.Context())); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.persistTeamLLMProfiles(r.Context(), record.ID, defaultProfile, profiles); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	response, err := a.buildTeamLLMProfilesResponse(r.Context(), record)
	if err != nil {
		http.Error(w, "failed to load team LLM profiles", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, response)
}

func (a *App) handleUpsertTeamLLMProfile(w http.ResponseWriter, r *http.Request) {
	record, ok := a.resolveAuthorizedTeamProfile(w, r, true)
	if !ok {
		return
	}
	profileName := config.NormalizeLLMProfileName(r.PathValue("profileName"))
	if profileName == "" {
		http.Error(w, "LLM profile name is required", http.StatusBadRequest)
		return
	}
	var payload llmProfileForm
	if err := httpapi.DecodeJSON(r, &payload); err != nil {
		http.Error(w, "invalid LLM profile payload", http.StatusBadRequest)
		return
	}
	if payload.Name != "" && !strings.EqualFold(config.NormalizeLLMProfileName(payload.Name), profileName) {
		http.Error(w, "LLM profile name in path and payload must match", http.StatusBadRequest)
		return
	}
	payload.Name = profileName
	profile := profileConfigFromForm(payload)
	if status, message := validateLLMProfileDefinition(profileName, profile); status != "valid" {
		http.Error(w, message, http.StatusBadRequest)
		return
	}
	if err := a.ensureLLMProfileCredentialReferences(r.Context(), map[string]config.LLMProfile{profileName: profile}, credentialActorFromContext(r.Context())); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.upsertTeamLLMProfile(r.Context(), record.ID, profileName, profile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	response, err := a.buildTeamLLMProfilesResponse(r.Context(), record)
	if err != nil {
		http.Error(w, "failed to load team LLM profiles", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, response)
}

func (a *App) handleDeleteTeamLLMProfile(w http.ResponseWriter, r *http.Request) {
	record, ok := a.resolveAuthorizedTeamProfile(w, r, true)
	if !ok {
		return
	}
	profileName := config.NormalizeLLMProfileName(r.PathValue("profileName"))
	if profileName == "" {
		http.Error(w, "LLM profile name is required", http.StatusBadRequest)
		return
	}
	if _, err := a.db.Exec(r.Context(), `
		DELETE FROM team_llm_profiles WHERE team_id = $1 AND name = $2
	`, record.ID, profileName); err != nil {
		http.Error(w, "failed to delete team LLM profile", http.StatusInternalServerError)
		return
	}
	if _, err := a.db.Exec(r.Context(), `
		DELETE FROM team_profile_settings WHERE team_id = $1 AND key = $2 AND value = $3
	`, record.ID, teamLLMDefaultProfileSetting, profileName); err != nil {
		http.Error(w, "failed to clear team LLM default profile", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleSetTeamDefaultLLMProfile(w http.ResponseWriter, r *http.Request) {
	record, ok := a.resolveAuthorizedTeamProfile(w, r, true)
	if !ok {
		return
	}
	var payload struct {
		DefaultProfile string `json:"default_profile"`
	}
	if err := httpapi.DecodeJSON(r, &payload); err != nil {
		http.Error(w, "invalid default LLM profile payload", http.StatusBadRequest)
		return
	}
	defaultProfile := config.NormalizeLLMProfileName(payload.DefaultProfile)
	if defaultProfile != "" {
		_, profiles, err := a.loadTeamLLMProfilesFromDB(r.Context(), record.ID)
		if err != nil {
			http.Error(w, "failed to load team LLM profiles", http.StatusInternalServerError)
			return
		}
		if _, ok := profiles[defaultProfile]; !ok {
			http.Error(w, "default LLM profile must be a team profile", http.StatusBadRequest)
			return
		}
	}
	if err := a.persistTeamProfileSetting(r.Context(), record.ID, teamLLMDefaultProfileSetting, defaultProfile); err != nil {
		http.Error(w, "failed to save team LLM default profile", http.StatusInternalServerError)
		return
	}
	response, err := a.buildTeamLLMProfilesResponse(r.Context(), record)
	if err != nil {
		http.Error(w, "failed to load team LLM profiles", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, response)
}

func (a *App) handleListTeamAgentProfiles(w http.ResponseWriter, r *http.Request) {
	record, ok := a.resolveAuthorizedTeamProfile(w, r, false)
	if !ok {
		return
	}
	response, err := a.buildTeamAgentProfilesResponse(r.Context(), record)
	if err != nil {
		http.Error(w, "failed to load team agent profiles", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, response)
}

func (a *App) handleCreateTeamAgentProfile(w http.ResponseWriter, r *http.Request) {
	record, ok := a.resolveAuthorizedTeamProfile(w, r, true)
	if !ok {
		return
	}
	var payload agentProfileForm
	if err := httpapi.DecodeJSON(r, &payload); err != nil {
		http.Error(w, "invalid agent profile payload", http.StatusBadRequest)
		return
	}
	profile := agentProfileFromForm(payload, "team")
	if err := validateAgentProfileDefinition(profile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.upsertTeamAgentProfile(r.Context(), record.ID, profile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	response, err := a.buildTeamAgentProfilesResponse(r.Context(), record)
	if err != nil {
		http.Error(w, "failed to load team agent profiles", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusCreated, response)
}

func (a *App) handleGetTeamAgentProfile(w http.ResponseWriter, r *http.Request) {
	record, ok := a.resolveAuthorizedTeamProfile(w, r, false)
	if !ok {
		return
	}
	profileID := models.NormalizeAgentProfileID(r.PathValue("profileID"))
	profiles, err := a.loadTeamAgentProfilesFromDB(r.Context(), record.ID)
	if err != nil {
		http.Error(w, "failed to load team agent profiles", http.StatusInternalServerError)
		return
	}
	profile, ok := profiles[profileID]
	if !ok {
		http.Error(w, "team agent profile not found", http.StatusNotFound)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, teamAgentProfileView{
		AgentProfile: profile,
		Scope:        "team",
		TeamID:       record.ID,
		TeamPath:     record.Path,
	})
}

func (a *App) handleUpsertTeamAgentProfile(w http.ResponseWriter, r *http.Request) {
	record, ok := a.resolveAuthorizedTeamProfile(w, r, true)
	if !ok {
		return
	}
	profileID := models.NormalizeAgentProfileID(r.PathValue("profileID"))
	if profileID == "" {
		http.Error(w, "agent profile id is required", http.StatusBadRequest)
		return
	}
	var payload agentProfileForm
	if err := httpapi.DecodeJSON(r, &payload); err != nil {
		http.Error(w, "invalid agent profile payload", http.StatusBadRequest)
		return
	}
	if payload.ID != "" && !strings.EqualFold(models.NormalizeAgentProfileID(payload.ID), profileID) {
		http.Error(w, "agent profile id in path and payload must match", http.StatusBadRequest)
		return
	}
	payload.ID = profileID
	profile := agentProfileFromForm(payload, "team")
	if err := validateAgentProfileDefinition(profile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.upsertTeamAgentProfile(r.Context(), record.ID, profile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	response, err := a.buildTeamAgentProfilesResponse(r.Context(), record)
	if err != nil {
		http.Error(w, "failed to load team agent profiles", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, response)
}

func (a *App) handleSetTeamDefaultAgentProfile(w http.ResponseWriter, r *http.Request) {
	record, ok := a.resolveAuthorizedTeamProfile(w, r, true)
	if !ok {
		return
	}
	var payload agentProfileDefaultRequest
	if err := httpapi.DecodeJSON(r, &payload); err != nil {
		http.Error(w, "invalid default agent profile payload", http.StatusBadRequest)
		return
	}
	defaultProfile := normalizeAgentProfileDefault(payload.DefaultProfile)
	profiles, err := a.loadTeamAgentProfilesFromDB(r.Context(), record.ID)
	if err != nil {
		http.Error(w, "failed to load team agent profiles", http.StatusInternalServerError)
		return
	}
	if defaultProfile != "" {
		if profile, ok := profiles[defaultProfile]; !ok || !profile.Enabled {
			http.Error(w, "default agent profile must be an enabled team profile", http.StatusBadRequest)
			return
		}
	}
	if err := a.persistTeamProfileSetting(r.Context(), record.ID, teamAgentDefaultProfileSetting, defaultProfile); err != nil {
		http.Error(w, "failed to save team default agent profile", http.StatusInternalServerError)
		return
	}
	response, err := a.buildTeamAgentProfilesResponse(r.Context(), record)
	if err != nil {
		http.Error(w, "failed to load team agent profiles", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, response)
}

func (a *App) handleDeleteTeamAgentProfile(w http.ResponseWriter, r *http.Request) {
	record, ok := a.resolveAuthorizedTeamProfile(w, r, true)
	if !ok {
		return
	}
	profileID := models.NormalizeAgentProfileID(r.PathValue("profileID"))
	if profileID == "" {
		http.Error(w, "agent profile id is required", http.StatusBadRequest)
		return
	}
	if _, err := a.db.Exec(r.Context(), `
		DELETE FROM team_agent_profiles WHERE team_id = $1 AND id = $2
	`, record.ID, profileID); err != nil {
		http.Error(w, "failed to delete team agent profile", http.StatusInternalServerError)
		return
	}
	if _, err := a.db.Exec(r.Context(), `
		DELETE FROM team_profile_settings WHERE team_id = $1 AND key = $2 AND value = $3
	`, record.ID, teamAgentDefaultProfileSetting, profileID); err != nil {
		http.Error(w, "failed to clear team agent default profile", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleListTeamMCPProfiles(w http.ResponseWriter, r *http.Request) {
	record, ok := a.resolveAuthorizedTeamProfile(w, r, false)
	if !ok {
		return
	}
	response, err := a.buildTeamMCPProfilesResponse(r.Context(), record)
	if err != nil {
		http.Error(w, "failed to load team MCP profiles", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, response)
}

func (a *App) handleCreateTeamMCPProfile(w http.ResponseWriter, r *http.Request) {
	record, ok := a.resolveAuthorizedTeamProfile(w, r, true)
	if !ok {
		return
	}
	profile, err := a.decodeValidatedTeamMCPProfile(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.upsertTeamMCPProfile(r.Context(), record.ID, profile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	response, err := a.buildTeamMCPProfilesResponse(r.Context(), record)
	if err != nil {
		http.Error(w, "failed to load team MCP profiles", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusCreated, response)
}

func (a *App) handleGetTeamMCPProfile(w http.ResponseWriter, r *http.Request) {
	record, ok := a.resolveAuthorizedTeamProfile(w, r, false)
	if !ok {
		return
	}
	profileName := models.NormalizeMCPProfileName(r.PathValue("profileName"))
	profiles, err := a.loadTeamMCPProfilesFromDB(r.Context(), record.ID)
	if err != nil {
		http.Error(w, "failed to load team MCP profiles", http.StatusInternalServerError)
		return
	}
	profile, ok := profiles[profileName]
	if !ok {
		http.Error(w, "team MCP profile not found", http.StatusNotFound)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, teamMCPProfileView{
		MCPProfile: profile,
		Scope:      "team",
		TeamID:     record.ID,
		TeamPath:   record.Path,
	})
}

func (a *App) handleUpsertTeamMCPProfile(w http.ResponseWriter, r *http.Request) {
	record, ok := a.resolveAuthorizedTeamProfile(w, r, true)
	if !ok {
		return
	}
	profile, err := a.decodeValidatedTeamMCPProfile(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.upsertTeamMCPProfile(r.Context(), record.ID, profile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	response, err := a.buildTeamMCPProfilesResponse(r.Context(), record)
	if err != nil {
		http.Error(w, "failed to load team MCP profiles", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, response)
}

func (a *App) handleDeleteTeamMCPProfile(w http.ResponseWriter, r *http.Request) {
	record, ok := a.resolveAuthorizedTeamProfile(w, r, true)
	if !ok {
		return
	}
	profileName := models.NormalizeMCPProfileName(r.PathValue("profileName"))
	if profileName == "" {
		http.Error(w, "MCP profile name is required", http.StatusBadRequest)
		return
	}
	if _, err := a.db.Exec(r.Context(), `
		DELETE FROM team_mcp_profiles WHERE team_id = $1 AND name = $2
	`, record.ID, profileName); err != nil {
		http.Error(w, "failed to delete team MCP profile", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) resolveAuthorizedTeamProfile(w http.ResponseWriter, r *http.Request, write bool) (groupPathRecord, bool) {
	record, status, err := a.resolveTeamRecord(r.Context(), r.PathValue("teamID"), false)
	if err != nil {
		http.Error(w, err.Error(), status)
		return groupPathRecord{}, false
	}
	action := "folder.read"
	if write {
		action = "folder.update"
	}
	if !a.authorizeTeamProfileAccess(w, r, record.ID, action) {
		return groupPathRecord{}, false
	}
	return record, true
}

func (a *App) authorizeTeamProfileAccess(w http.ResponseWriter, r *http.Request, teamID int, action string) bool {
	resource, err := a.folderGrantResourceByGroupID(r.Context(), teamID)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return false
	}
	return a.requireAAADecision(w, r, action, model.ResourceRef{Type: grantResourceFolder, ID: resource.ID})
}

func parseTeamLLMProfilesPayload(payload llmProfilesRequest) (string, map[string]config.LLMProfile, error) {
	defaultProfile := config.NormalizeLLMProfileName(firstNonEmptyString(payload.DefaultProfile, payload.LLMDefaultProfile))
	profiles := map[string]config.LLMProfile{}
	for name, profile := range payload.LLMProfiles {
		profileName := config.NormalizeLLMProfileName(name)
		if profileName == "" {
			return "", nil, fmt.Errorf("LLM profile name is required")
		}
		profile = config.NormalizeLLMProfile(profile)
		if status, message := validateLLMProfileDefinition(profileName, profile); status != "valid" {
			return "", nil, fmt.Errorf("%s", message)
		}
		profiles[profileName] = profile
	}
	for _, form := range payload.Profiles {
		profileName := config.NormalizeLLMProfileName(form.Name)
		if profileName == "" {
			return "", nil, fmt.Errorf("LLM profile name is required")
		}
		profile := profileConfigFromForm(form)
		if status, message := validateLLMProfileDefinition(profileName, profile); status != "valid" {
			return "", nil, fmt.Errorf("%s", message)
		}
		if _, exists := profiles[profileName]; exists {
			return "", nil, fmt.Errorf("duplicate LLM profile %q", profileName)
		}
		profiles[profileName] = profile
	}
	if defaultProfile != "" {
		if _, ok := profiles[defaultProfile]; !ok {
			return "", nil, fmt.Errorf("default LLM profile %q is not defined for this team", defaultProfile)
		}
	}
	return defaultProfile, profiles, nil
}

func (a *App) buildTeamLLMProfilesResponse(ctx context.Context, record groupPathRecord) (teamLLMProfilesResponse, error) {
	defaultProfile, profiles, err := a.loadTeamLLMProfilesFromDB(ctx, record.ID)
	if err != nil {
		return teamLLMProfilesResponse{}, err
	}
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	views := make([]teamLLMProfileView, 0, len(names))
	scope := strings.TrimSpace(record.Path)
	for _, name := range names {
		profile := config.NormalizeLLMProfile(profiles[name])
		status, message := validateLLMProfileDefinition(name, profile)
		views = append(views, teamLLMProfileView{
			llmProfileView: llmProfileView{
				llmProfileForm: profileFormFromConfig(name, profile),
				Status:         status,
				Validation:     message,
				AllowedInScope: config.LLMProfileAllowedInScope(profile, scope),
			},
			Scope:    "team",
			TeamID:   record.ID,
			TeamPath: record.Path,
		})
	}
	return teamLLMProfilesResponse{
		TeamID:         record.ID,
		TeamPath:       record.Path,
		DefaultProfile: defaultProfile,
		Profiles:       views,
	}, nil
}

func (a *App) loadTeamLLMProfilesFromDB(ctx context.Context, teamID int) (string, map[string]config.LLMProfile, error) {
	defaultProfile, err := a.loadTeamProfileSetting(ctx, teamID, teamLLMDefaultProfileSetting)
	if err != nil {
		return "", nil, err
	}
	rows, err := a.db.Query(ctx, `
		SELECT name, provider, model, base_url, credential_ref, allowed_scopes, reasoning, thinking, timeout_seconds, max_tokens, temperature, extra
		FROM team_llm_profiles
		WHERE team_id = $1
		ORDER BY name ASC
	`, teamID)
	if err != nil {
		return "", nil, fmt.Errorf("load team LLM profiles: %w", err)
	}
	defer rows.Close()

	profiles := map[string]config.LLMProfile{}
	for rows.Next() {
		var (
			name       string
			profile    config.LLMProfile
			allowedRaw []byte
			extraRaw   []byte
			thinking   sql.NullBool
			temp       sql.NullFloat64
		)
		if err := rows.Scan(
			&name,
			&profile.Provider,
			&profile.Model,
			&profile.BaseURL,
			&profile.CredentialRef,
			&allowedRaw,
			&profile.Reasoning,
			&thinking,
			&profile.TimeoutSeconds,
			&profile.MaxTokens,
			&temp,
			&extraRaw,
		); err != nil {
			return "", nil, fmt.Errorf("scan team LLM profile: %w", err)
		}
		if len(allowedRaw) > 0 {
			_ = json.Unmarshal(allowedRaw, &profile.AllowedScopes)
		}
		if thinking.Valid {
			value := thinking.Bool
			profile.Thinking = &value
		}
		if temp.Valid {
			value := temp.Float64
			profile.Temperature = &value
		}
		if len(extraRaw) > 0 {
			_ = json.Unmarshal(extraRaw, &profile.Extra)
		}
		profiles[config.NormalizeLLMProfileName(name)] = config.NormalizeLLMProfile(profile)
	}
	if err := rows.Err(); err != nil {
		return "", nil, fmt.Errorf("iterate team LLM profiles: %w", err)
	}
	defaultProfile = config.NormalizeLLMProfileName(defaultProfile)
	if defaultProfile != "" {
		if _, ok := profiles[defaultProfile]; !ok {
			defaultProfile = ""
		}
	}
	return defaultProfile, profiles, nil
}

func (a *App) persistTeamLLMProfiles(ctx context.Context, teamID int, defaultProfile string, profiles map[string]config.LLMProfile) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin team LLM profile persistence: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM team_llm_profiles WHERE team_id = $1`, teamID); err != nil {
		return fmt.Errorf("clear team LLM profiles: %w", err)
	}
	for name, profile := range config.NormalizeLLMProfiles(profiles) {
		if err := upsertTeamLLMProfileTx(ctx, tx, teamID, name, profile); err != nil {
			return err
		}
	}
	if err := persistTeamProfileSettingTx(ctx, tx, teamID, teamLLMDefaultProfileSetting, defaultProfile); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit team LLM profiles: %w", err)
	}
	return nil
}

func (a *App) upsertTeamLLMProfile(ctx context.Context, teamID int, name string, profile config.LLMProfile) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin team LLM profile persistence: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := upsertTeamLLMProfileTx(ctx, tx, teamID, name, profile); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit team LLM profile: %w", err)
	}
	return nil
}

func upsertTeamLLMProfileTx(ctx context.Context, tx pgx.Tx, teamID int, name string, profile config.LLMProfile) error {
	return upsertTeamLLMProfileTxWithSource(ctx, tx, teamID, name, profile, "ui", "", "", nil, false)
}

func upsertTeamLLMProfileTxWithSource(ctx context.Context, tx pgx.Tx, teamID int, name string, profile config.LLMProfile, source, sourcePath, commitSHA string, configRepoID any, managed bool) error {
	name = config.NormalizeLLMProfileName(name)
	profile = config.NormalizeLLMProfile(profile)
	if source == "" {
		source = "ui"
	}
	if status, message := validateLLMProfileDefinition(name, profile); status != "valid" {
		return fmt.Errorf("%s", message)
	}
	allowedJSON, err := json.Marshal(profile.AllowedScopes)
	if err != nil {
		return fmt.Errorf("encode team LLM profile allowed scopes: %w", err)
	}
	extraJSON, err := json.Marshal(profile.Extra)
	if err != nil {
		return fmt.Errorf("encode team LLM profile extra: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO team_llm_profiles (
			team_id, name, provider, model, base_url, credential_ref, allowed_scopes,
			reasoning, thinking, timeout_seconds, max_tokens, temperature, extra, source,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $11, $12, $13::jsonb, $14, $15, $16, $17, $18, NOW())
		ON CONFLICT (team_id, name) DO UPDATE SET
			provider = EXCLUDED.provider,
			model = EXCLUDED.model,
			base_url = EXCLUDED.base_url,
			credential_ref = EXCLUDED.credential_ref,
			allowed_scopes = EXCLUDED.allowed_scopes,
			reasoning = EXCLUDED.reasoning,
			thinking = EXCLUDED.thinking,
			timeout_seconds = EXCLUDED.timeout_seconds,
			max_tokens = EXCLUDED.max_tokens,
			temperature = EXCLUDED.temperature,
			extra = EXCLUDED.extra,
			source = EXCLUDED.source,
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = EXCLUDED.managed_by_config_repo,
			updated_at = NOW()
	`, teamID, name, profile.Provider, profile.Model, profile.BaseURL, profile.CredentialRef, string(allowedJSON), profile.Reasoning, profile.Thinking, profile.TimeoutSeconds, profile.MaxTokens, profile.Temperature, string(extraJSON), source, configRepoID, sourcePath, commitSHA, managed)
	if err != nil {
		return fmt.Errorf("persist team LLM profile %q: %w", name, err)
	}
	return nil
}

func (a *App) buildTeamAgentProfilesResponse(ctx context.Context, record groupPathRecord) (teamAgentProfilesResponse, error) {
	defaultProfile, err := a.loadTeamProfileSetting(ctx, record.ID, teamAgentDefaultProfileSetting)
	if err != nil {
		return teamAgentProfilesResponse{}, err
	}
	profiles, err := a.loadTeamAgentProfilesFromDB(ctx, record.ID)
	if err != nil {
		return teamAgentProfilesResponse{}, err
	}
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	views := make([]teamAgentProfileView, 0, len(names))
	for _, name := range names {
		views = append(views, teamAgentProfileView{
			AgentProfile: profiles[name],
			Scope:        "team",
			TeamID:       record.ID,
			TeamPath:     record.Path,
		})
	}
	defaultProfile = normalizeAgentProfileDefault(defaultProfile)
	if defaultProfile != "" {
		if _, ok := profiles[defaultProfile]; !ok {
			defaultProfile = ""
		}
	}
	return teamAgentProfilesResponse{
		TeamID:         record.ID,
		TeamPath:       record.Path,
		DefaultProfile: defaultProfile,
		Profiles:       views,
	}, nil
}

func (a *App) loadTeamAgentProfilesFromDB(ctx context.Context, teamID int) (map[string]models.AgentProfile, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id, display_name, role, description, instructions, enabled, source
		FROM team_agent_profiles
		WHERE team_id = $1
		ORDER BY id ASC
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("load team agent profiles: %w", err)
	}
	defer rows.Close()

	profiles := map[string]models.AgentProfile{}
	for rows.Next() {
		var profile models.AgentProfile
		if err := rows.Scan(&profile.ID, &profile.DisplayName, &profile.Role, &profile.Description, &profile.Instructions, &profile.Enabled, &profile.Source); err != nil {
			return nil, fmt.Errorf("scan team agent profile: %w", err)
		}
		profile = models.NormalizeAgentProfile(profile)
		profiles[profile.ID] = profile
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate team agent profiles: %w", err)
	}
	return profiles, nil
}

func (a *App) upsertTeamAgentProfile(ctx context.Context, teamID int, profile models.AgentProfile) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin team agent profile persistence: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := upsertTeamAgentProfileTx(ctx, tx, teamID, profile, "team", "", "", nil, false); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit team agent profile: %w", err)
	}
	return nil
}

func upsertTeamAgentProfileTx(ctx context.Context, tx pgx.Tx, teamID int, profile models.AgentProfile, source, sourcePath, commitSHA string, configRepoID any, managed bool) error {
	profile = models.NormalizeAgentProfile(profile)
	if err := validateAgentProfileDefinition(profile); err != nil {
		return err
	}
	if profile.Source == "" {
		profile.Source = source
	}
	if profile.Source == "" {
		profile.Source = "team"
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO team_agent_profiles (
			team_id, id, display_name, role, description, instructions, enabled, source,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
		ON CONFLICT (team_id, id) DO UPDATE SET
			display_name = EXCLUDED.display_name,
			role = EXCLUDED.role,
			description = EXCLUDED.description,
			instructions = EXCLUDED.instructions,
			enabled = EXCLUDED.enabled,
			source = EXCLUDED.source,
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = EXCLUDED.managed_by_config_repo,
			updated_at = NOW()
	`, teamID, profile.ID, profile.DisplayName, profile.Role, profile.Description, profile.Instructions, profile.Enabled, profile.Source, configRepoID, sourcePath, commitSHA, managed)
	if err != nil {
		return fmt.Errorf("persist team agent profile %q: %w", profile.ID, err)
	}
	return nil
}

func (a *App) buildTeamMCPProfilesResponse(ctx context.Context, record groupPathRecord) (teamMCPProfilesResponse, error) {
	profiles, err := a.loadTeamMCPProfilesFromDB(ctx, record.ID)
	if err != nil {
		return teamMCPProfilesResponse{}, err
	}
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	views := make([]teamMCPProfileView, 0, len(names))
	for _, name := range names {
		views = append(views, teamMCPProfileView{
			MCPProfile: profiles[name],
			Scope:      "team",
			TeamID:     record.ID,
			TeamPath:   record.Path,
		})
	}
	return teamMCPProfilesResponse{
		TeamID:   record.ID,
		TeamPath: record.Path,
		Profiles: views,
	}, nil
}

func (a *App) loadTeamMCPProfilesFromDB(ctx context.Context, teamID int) (map[string]models.MCPProfile, error) {
	rows, err := a.db.Query(ctx, `
		SELECT name, description, enabled, server_refs, allowed_scopes
		FROM team_mcp_profiles
		WHERE team_id = $1
		ORDER BY name ASC
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("load team MCP profiles: %w", err)
	}
	defer rows.Close()

	profiles := map[string]models.MCPProfile{}
	for rows.Next() {
		var (
			profile    models.MCPProfile
			refsRaw    []byte
			allowedRaw []byte
		)
		if err := rows.Scan(&profile.Name, &profile.Description, &profile.Enabled, &refsRaw, &allowedRaw); err != nil {
			return nil, fmt.Errorf("scan team MCP profile: %w", err)
		}
		if len(refsRaw) > 0 {
			_ = json.Unmarshal(refsRaw, &profile.ServerRefs)
		}
		if len(allowedRaw) > 0 {
			_ = json.Unmarshal(allowedRaw, &profile.AllowedScopes)
		}
		profile = models.NormalizeMCPProfile(profile)
		profiles[profile.Name] = profile
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate team MCP profiles: %w", err)
	}
	return profiles, nil
}

func (a *App) decodeValidatedTeamMCPProfile(r *http.Request) (models.MCPProfile, error) {
	var profile models.MCPProfile
	if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
		return models.MCPProfile{}, fmt.Errorf("invalid MCP profile payload")
	}
	pathName := models.NormalizeMCPProfileName(r.PathValue("profileName"))
	if pathName != "" {
		if profile.Name != "" && !strings.EqualFold(models.NormalizeMCPProfileName(profile.Name), pathName) {
			return models.MCPProfile{}, fmt.Errorf("MCP profile name in path and payload must match")
		}
		profile.Name = pathName
	}
	profile = models.NormalizeMCPProfile(profile)
	servers, err := a.loadMCPServersFromDB(r.Context())
	if err != nil {
		return models.MCPProfile{}, fmt.Errorf("failed to load MCP servers")
	}
	toolsByServer, err := a.loadMCPToolsByServer(r.Context())
	if err != nil {
		return models.MCPProfile{}, fmt.Errorf("failed to load MCP tools")
	}
	if err := mcpregistry.ValidateProfileDefinition(profile, servers, toolsByServer); err != nil {
		return models.MCPProfile{}, err
	}
	return profile, nil
}

func (a *App) upsertTeamMCPProfile(ctx context.Context, teamID int, profile models.MCPProfile) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin team MCP profile persistence: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := upsertTeamMCPProfileTx(ctx, tx, teamID, profile, "team", "", "", nil, false); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit team MCP profile: %w", err)
	}
	return nil
}

func upsertTeamMCPProfileTx(ctx context.Context, tx pgx.Tx, teamID int, profile models.MCPProfile, source, sourcePath, commitSHA string, configRepoID any, managed bool) error {
	profile = models.NormalizeMCPProfile(profile)
	if source == "" {
		source = "team"
	}
	refsJSON, err := json.Marshal(profile.ServerRefs)
	if err != nil {
		return fmt.Errorf("encode team MCP profile server refs: %w", err)
	}
	allowedJSON, err := json.Marshal(profile.AllowedScopes)
	if err != nil {
		return fmt.Errorf("encode team MCP profile allowed scopes: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO team_mcp_profiles (
			team_id, name, description, enabled, server_refs, allowed_scopes, source,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at
		)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7, $8, $9, $10, $11, NOW())
		ON CONFLICT (team_id, name) DO UPDATE SET
			description = EXCLUDED.description,
			enabled = EXCLUDED.enabled,
			server_refs = EXCLUDED.server_refs,
			allowed_scopes = EXCLUDED.allowed_scopes,
			source = EXCLUDED.source,
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = EXCLUDED.managed_by_config_repo,
			updated_at = NOW()
	`, teamID, profile.Name, profile.Description, profile.Enabled, string(refsJSON), string(allowedJSON), source, configRepoID, sourcePath, commitSHA, managed)
	if err != nil {
		return fmt.Errorf("persist team MCP profile %q: %w", profile.Name, err)
	}
	return nil
}

func (a *App) loadTeamProfileSetting(ctx context.Context, teamID int, key string) (string, error) {
	var value string
	if err := a.db.QueryRow(ctx, `
		SELECT value FROM team_profile_settings WHERE team_id = $1 AND key = $2
	`, teamID, key).Scan(&value); err != nil {
		if errorsIsNoRows(err) {
			return "", nil
		}
		return "", fmt.Errorf("load team profile setting %q: %w", key, err)
	}
	return strings.TrimSpace(value), nil
}

func (a *App) persistTeamProfileSetting(ctx context.Context, teamID int, key, value string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin team profile setting persistence: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := persistTeamProfileSettingTx(ctx, tx, teamID, key, value); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit team profile setting: %w", err)
	}
	return nil
}

func persistTeamProfileSettingTx(ctx context.Context, tx pgx.Tx, teamID int, key, value string) error {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" {
		return fmt.Errorf("team profile setting key is required")
	}
	if value == "" {
		_, err := tx.Exec(ctx, `DELETE FROM team_profile_settings WHERE team_id = $1 AND key = $2`, teamID, key)
		if err != nil {
			return fmt.Errorf("clear team profile setting %q: %w", key, err)
		}
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO team_profile_settings (team_id, key, value, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (team_id, key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`, teamID, key, value)
	if err != nil {
		return fmt.Errorf("persist team profile setting %q: %w", key, err)
	}
	return nil
}

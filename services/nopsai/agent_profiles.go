package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/pkg/validation"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"
)

const agentProfilesRuntimeEnv = "NOPSAI_AGENT_PROFILES"

var agentProfileIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+(?:/[a-zA-Z0-9_.-]+)*$`)

var agentProfileSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS agent_profile_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT '',
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE IF NOT EXISTS agent_profiles (
		id TEXT PRIMARY KEY,
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
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`ALTER TABLE agent_profiles ALTER COLUMN role SET DEFAULT ''`,
	`CREATE INDEX IF NOT EXISTS idx_agent_profiles_source ON agent_profiles(source)`,
	`CREATE INDEX IF NOT EXISTS idx_agent_profiles_config_repo ON agent_profiles(config_repo_id)`,
}

type agentProfileForm struct {
	ID           string `json:"id" yaml:"id"`
	DisplayName  string `json:"display_name" yaml:"display_name"`
	Role         string `json:"role,omitempty" yaml:"role,omitempty"`
	Description  string `json:"description,omitempty" yaml:"description,omitempty"`
	Instructions string `json:"instructions" yaml:"instructions"`
	Enabled      *bool  `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}

type agentProfileView struct {
	models.AgentProfile
	UsageCount  int      `json:"usage_count"`
	References  []string `json:"references,omitempty"`
	LastUpdated string   `json:"last_updated,omitempty"`
	ReadOnly    bool     `json:"read_only"`
	SourcePath  string   `json:"source_path,omitempty"`
}

type agentProfilesResponse struct {
	DefaultProfile string             `json:"default_profile"`
	Profiles       []agentProfileView `json:"profiles"`
}

type agentProfileUsageResponse struct {
	ProfileID  string   `json:"profile_id"`
	UsageCount int      `json:"usage_count"`
	References []string `json:"references"`
}

type agentProfilesRequest struct {
	DefaultProfile      string             `json:"default_profile" yaml:"default_profile"`
	AgentDefaultProfile string             `json:"agent_default_profile" yaml:"agent_default_profile"`
	AgentProfiles       []agentProfileForm `json:"agent_profiles" yaml:"agent_profiles"`
	Profiles            []agentProfileForm `json:"profiles" yaml:"profiles"`
}

type agentProfileDefaultRequest struct {
	DefaultProfile string `json:"default_profile"`
}

type storedAgentProfile struct {
	models.AgentProfile
	ConfigSourcePath      string
	ConfigSourceCommitSHA string
	ManagedByConfigRepo   bool
	UpdatedAt             time.Time
}

type gitOpsAgentProfileDirectory struct {
	root  string
	files map[string]string
}

type gitOpsAgentProfileFileCandidate struct {
	sourcePath string
	content    string
}

type gitOpsAgentProfilePlan struct {
	defaultProfile string
	profiles       map[string]models.AgentProfile
	sourcePath     string
}

type runtimeAgentProfile struct {
	DisplayName  string `json:"display_name,omitempty"`
	Role         string `json:"role,omitempty"`
	Instructions string `json:"instructions"`
	Enabled      bool   `json:"enabled"`
	Source       string `json:"source,omitempty"`
}

type runtimeAgentProfiles struct {
	DefaultProfile string                         `json:"default_profile"`
	Profiles       map[string]runtimeAgentProfile `json:"profiles"`
}

func agentProfileFromForm(form agentProfileForm, source string) models.AgentProfile {
	enabled := true
	if form.Enabled != nil {
		enabled = *form.Enabled
	}
	return models.NormalizeAgentProfile(models.AgentProfile{
		ID:           form.ID,
		DisplayName:  form.DisplayName,
		Role:         form.Role,
		Description:  form.Description,
		Instructions: form.Instructions,
		Enabled:      enabled,
		Source:       source,
	})
}

func agentProfileFormFromModel(profile models.AgentProfile) agentProfileForm {
	enabled := profile.Enabled
	return agentProfileForm{
		ID:           profile.ID,
		DisplayName:  profile.DisplayName,
		Role:         profile.Role,
		Description:  profile.Description,
		Instructions: profile.Instructions,
		Enabled:      &enabled,
	}
}

func ensureAgentProfileSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin agent profile schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	for idx, stmt := range agentProfileSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply agent profile schema statement %d: %w", idx+1, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit agent profile schema transaction: %w", err)
	}
	return nil
}

func validateAgentProfileDefinition(profile models.AgentProfile) error {
	profile = models.NormalizeAgentProfile(profile)
	if profile.ID == "" {
		return fmt.Errorf("agent profile id is required")
	}
	if !agentProfileIDPattern.MatchString(profile.ID) {
		return fmt.Errorf("agent profile %q can only contain slash-separated letters, numbers, dots, underscores, and hyphens", profile.ID)
	}
	if profile.DisplayName == "" {
		return fmt.Errorf("agent profile %q is missing display_name", profile.ID)
	}
	if profile.Instructions == "" {
		return fmt.Errorf("agent profile %q is missing instructions", profile.ID)
	}
	return nil
}

func normalizeAgentProfileDefault(raw string) string {
	return models.NormalizeAgentProfileID(raw)
}

func requestedAgentProfileDefault(file agentProfilesRequest) string {
	defaultProfile := normalizeAgentProfileDefault(file.DefaultProfile)
	if defaultProfile == "" {
		defaultProfile = normalizeAgentProfileDefault(file.AgentDefaultProfile)
	}
	return defaultProfile
}

func effectiveDefaultAgentProfile(configured string, profiles map[string]models.AgentProfile) string {
	configured = normalizeAgentProfileDefault(configured)
	if configured != "" {
		if profile, ok := profiles[configured]; ok && profile.Enabled {
			return configured
		}
	}
	if profile, ok := profiles[models.DefaultAgentProfileID]; ok && profile.Enabled {
		return models.DefaultAgentProfileID
	}
	names := make([]string, 0, len(profiles))
	for name, profile := range profiles {
		if profile.Enabled {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) > 0 {
		return names[0]
	}
	return models.DefaultAgentProfileID
}

func validateDefaultAgentProfile(defaultProfile string, profiles map[string]models.AgentProfile) error {
	defaultProfile = normalizeAgentProfileDefault(defaultProfile)
	if defaultProfile == "" {
		defaultProfile = models.DefaultAgentProfileID
	}
	profile, ok := profiles[defaultProfile]
	if !ok {
		return fmt.Errorf("default agent profile %q is not configured", defaultProfile)
	}
	if !profile.Enabled {
		return fmt.Errorf("default agent profile %q is disabled", defaultProfile)
	}
	return nil
}

func (a *App) loadStoredAgentProfilesFromDB(ctx context.Context) (map[string]storedAgentProfile, error) {
	if a == nil || a.db == nil {
		return nil, nil
	}
	rows, err := a.db.Query(ctx, `
		SELECT id, display_name, role, description, instructions, enabled, source,
			config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at
		FROM agent_profiles
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("load agent profiles: %w", err)
	}
	defer rows.Close()

	profiles := map[string]storedAgentProfile{}
	for rows.Next() {
		var stored storedAgentProfile
		if err := rows.Scan(
			&stored.ID,
			&stored.DisplayName,
			&stored.Role,
			&stored.Description,
			&stored.Instructions,
			&stored.Enabled,
			&stored.Source,
			&stored.ConfigSourcePath,
			&stored.ConfigSourceCommitSHA,
			&stored.ManagedByConfigRepo,
			&stored.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan agent profile: %w", err)
		}
		stored.AgentProfile = models.NormalizeAgentProfile(stored.AgentProfile)
		if stored.Source == "" {
			stored.Source = "ui"
		}
		profiles[stored.ID] = stored
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent profiles: %w", err)
	}
	return profiles, nil
}

func (a *App) effectiveAgentProfiles(ctx context.Context) (map[string]models.AgentProfile, map[string]storedAgentProfile, error) {
	profiles := models.BuiltInAgentProfiles()
	stored, err := a.loadStoredAgentProfilesFromDB(ctx)
	if err != nil {
		return nil, nil, err
	}
	for id, record := range stored {
		profile := models.NormalizeAgentProfile(record.AgentProfile)
		profile.BuiltIn = false
		if profile.Source == "" {
			profile.Source = "ui"
		}
		profiles[id] = profile
	}
	return profiles, stored, nil
}

func (a *App) loadAgentProfileDefaultFromDB(ctx context.Context) (string, error) {
	if a == nil || a.db == nil {
		return models.DefaultAgentProfileID, nil
	}
	var defaultProfile string
	err := a.db.QueryRow(ctx, `
		SELECT value
		FROM agent_profile_settings
		WHERE key = 'default_profile'
	`).Scan(&defaultProfile)
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, sql.ErrNoRows):
		return models.DefaultAgentProfileID, nil
	case err != nil:
		return "", fmt.Errorf("load default agent profile: %w", err)
	default:
		defaultProfile = normalizeAgentProfileDefault(defaultProfile)
		if defaultProfile == "" {
			defaultProfile = models.DefaultAgentProfileID
		}
		return defaultProfile, nil
	}
}

func (a *App) effectiveAgentProfileDefault(ctx context.Context, profiles map[string]models.AgentProfile) (string, error) {
	if profiles == nil {
		var err error
		profiles, _, err = a.effectiveAgentProfiles(ctx)
		if err != nil {
			return "", err
		}
	}
	configured, err := a.loadAgentProfileDefaultFromDB(ctx)
	if err != nil {
		return "", err
	}
	return effectiveDefaultAgentProfile(configured, profiles), nil
}

func persistAgentProfileDefaultToTx(ctx context.Context, tx pgx.Tx, defaultProfile string) error {
	defaultProfile = normalizeAgentProfileDefault(defaultProfile)
	if defaultProfile == "" {
		defaultProfile = models.DefaultAgentProfileID
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO agent_profile_settings (key, value, updated_at)
		VALUES ('default_profile', $1, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`, defaultProfile); err != nil {
		return fmt.Errorf("persist default agent profile: %w", err)
	}
	return nil
}

func (a *App) persistAgentProfileDefaultToDB(ctx context.Context, defaultProfile string, profiles map[string]models.AgentProfile) error {
	if a == nil || a.db == nil {
		return nil
	}
	if profiles == nil {
		var err error
		profiles, _, err = a.effectiveAgentProfiles(ctx)
		if err != nil {
			return err
		}
	}
	if err := validateDefaultAgentProfile(defaultProfile, profiles); err != nil {
		return err
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin default agent profile persistence: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := persistAgentProfileDefaultToTx(ctx, tx, defaultProfile); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit default agent profile persistence: %w", err)
	}
	return nil
}

func (a *App) agentProfileCatalogForValidation(ctx context.Context) (string, map[string]validation.AgentProfileDefinition, error) {
	return a.agentProfileCatalogForValidationForTeam(ctx, nil)
}

func (a *App) agentProfileCatalogForValidationForTeam(ctx context.Context, teamID *int) (string, map[string]validation.AgentProfileDefinition, error) {
	profiles, defaultProfile, err := a.effectiveAgentProfilesForTeam(ctx, teamID)
	if err != nil {
		return "", nil, err
	}
	catalog := make(map[string]validation.AgentProfileDefinition, len(profiles))
	for id, profile := range profiles {
		catalog[id] = validation.AgentProfileDefinition{Enabled: profile.Enabled}
	}
	return defaultProfile, catalog, nil
}

func (a *App) validatePipelineAgentProfiles(pipeline *models.Pipeline) error {
	return a.validatePipelineAgentProfilesForTeam(context.Background(), pipeline, nil)
}

func (a *App) validatePipelineAgentProfilesForTeam(ctx context.Context, pipeline *models.Pipeline, teamID *int) error {
	defaultProfile, catalog, err := a.agentProfileCatalogForValidationForTeam(ctx, teamID)
	if err != nil {
		return err
	}
	requireTeamDefault := teamID != nil
	if requireTeamDefault {
		teamDefault, err := a.loadTeamProfileSetting(ctx, *teamID, teamAgentDefaultProfileSetting)
		if err != nil {
			return err
		}
		defaultProfile = normalizeAgentProfileDefault(teamDefault)
	}
	return validation.ValidatePipelineAgentProfiles(pipeline, validation.AgentProfileValidationOptions{
		DefaultProfile:        defaultProfile,
		RequireDefaultProfile: requireTeamDefault,
		Profiles:              catalog,
	})
}

func collectExplicitAgentProfileReferencesFromPipeline(pipeline *models.Pipeline, profileID, prefix string) []string {
	var refs []string
	if pipeline == nil {
		return refs
	}
	if strings.EqualFold(strings.TrimSpace(pipeline.AgentProfile), profileID) {
		refs = append(refs, prefix)
	}
	for _, step := range pipeline.Steps {
		stepName := strings.TrimSpace(step.GetName())
		if stepName == "" {
			stepName = "unknown"
		}
		if strings.EqualFold(strings.TrimSpace(step.GetAgentProfile()), profileID) {
			refs = append(refs, fmt.Sprintf("%s step %q", prefix, stepName))
		}
	}
	return refs
}

func (a *App) findAgentProfileReferences(profileID string) ([]string, error) {
	if a == nil || a.db == nil {
		return nil, nil
	}
	profileID = models.NormalizeAgentProfileID(profileID)
	var refs []string
	rows, err := a.db.Query(context.Background(), "SELECT path, name, definition FROM pipelines ORDER BY path ASC, name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pathPart, namePart, definition string
		if err := rows.Scan(&pathPart, &namePart, &definition); err != nil {
			return nil, err
		}
		var pipeline models.Pipeline
		pipelineID := configsync.BuildPipelineIdentifier(pathPart, namePart)
		if err := yaml.Unmarshal([]byte(definition), &pipeline); err != nil {
			refs = append(refs, fmt.Sprintf("pipeline %s (unreadable YAML)", pipelineID))
			continue
		}
		refs = append(refs, collectExplicitAgentProfileReferencesFromPipeline(&pipeline, profileID, "pipeline "+pipelineID)...)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	stepRows, err := a.db.Query(context.Background(), "SELECT path, name, definition FROM steps ORDER BY path ASC, name ASC")
	if err != nil {
		return nil, err
	}
	defer stepRows.Close()
	for stepRows.Next() {
		var pathPart, namePart, definition string
		if err := stepRows.Scan(&pathPart, &namePart, &definition); err != nil {
			return nil, err
		}
		var step models.PipelineStep
		stepID := configsync.BuildPipelineIdentifier(pathPart, namePart)
		if err := yaml.Unmarshal([]byte(definition), &step); err != nil {
			refs = append(refs, fmt.Sprintf("step %s (unreadable YAML)", stepID))
			continue
		}
		if strings.EqualFold(strings.TrimSpace(step.GetAgentProfile()), profileID) {
			refs = append(refs, "step "+stepID)
		}
	}
	if err := stepRows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(refs)
	return refs, nil
}

func persistAgentProfileToTx(ctx context.Context, tx pgx.Tx, profile models.AgentProfile, source, sourcePath, commitSHA string, configRepoID any, managed bool) error {
	profile = models.NormalizeAgentProfile(profile)
	if err := validateAgentProfileDefinition(profile); err != nil {
		return err
	}
	if source == "" {
		source = "ui"
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO agent_profiles (
			id, display_name, role, description, instructions, enabled, source,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
		ON CONFLICT (id) DO UPDATE SET
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
	`, profile.ID, profile.DisplayName, profile.Role, profile.Description, profile.Instructions, profile.Enabled, source, configRepoID, sourcePath, commitSHA, managed)
	if err != nil {
		return fmt.Errorf("persist agent profile %q: %w", profile.ID, err)
	}
	return nil
}

func (a *App) persistAgentProfileToDB(ctx context.Context, profile models.AgentProfile) error {
	if a == nil || a.db == nil {
		return nil
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin agent profile persistence: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := persistAgentProfileToTx(ctx, tx, profile, "ui", "", "", nil, false); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit agent profile persistence: %w", err)
	}
	return nil
}

func persistGitOpsAgentProfilesToTx(ctx context.Context, tx pgx.Tx, plan *gitOpsAgentProfilePlan, configRepoID int64, commitSHA string) error {
	if plan == nil {
		return nil
	}
	ids := make([]string, 0, len(plan.profiles))
	for id := range plan.profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM agent_profiles WHERE source = 'gitops'`); err != nil {
			return fmt.Errorf("clear GitOps agent profiles: %w", err)
		}
	} else if _, err := tx.Exec(ctx, `DELETE FROM agent_profiles WHERE source = 'gitops' AND id != ALL($1::text[])`, ids); err != nil {
		return fmt.Errorf("prune GitOps agent profiles: %w", err)
	}
	for _, id := range ids {
		profile := plan.profiles[id]
		if err := persistAgentProfileToTx(ctx, tx, profile, "gitops", plan.sourcePath, commitSHA, configRepoID, true); err != nil {
			return err
		}
	}
	if err := persistAgentProfileDefaultToTx(ctx, tx, plan.defaultProfile); err != nil {
		return err
	}
	return nil
}

func parseGitOpsAgentProfilePlan(binding models.ConfigRepository, directories ...gitOpsAgentProfileDirectory) (*gitOpsAgentProfilePlan, error) {
	candidates := []gitOpsAgentProfileFileCandidate{}
	for _, directory := range directories {
		root := filepath.ToSlash(strings.Trim(directory.root, "/"))
		for path, content := range directory.files {
			normalized := filepath.ToSlash(path)
			rel, ok := configsync.RelativePath(normalized, root)
			if !ok || !isGitOpsAgentProfileRelativePath(rel) {
				continue
			}
			candidates = append(candidates, gitOpsAgentProfileFileCandidate{sourcePath: normalized, content: content})
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	if binding.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil, fmt.Errorf("agent profiles can only be configured from a system config repository")
	}
	if len(candidates) > 1 {
		paths := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			paths = append(paths, candidate.sourcePath)
		}
		sort.Strings(paths)
		return nil, fmt.Errorf("multiple agent profile GitOps files found: %s", strings.Join(paths, ", "))
	}
	return parseGitOpsAgentProfileFile(candidates[0].content, candidates[0].sourcePath)
}

func isGitOpsAgentProfileRelativePath(rel string) bool {
	switch strings.Trim(filepath.ToSlash(rel), "/") {
	case "system/agent-profiles.yaml", "system/agent-profiles.yml", "system/agent_profiles.yaml", "system/agent_profiles.yml":
		return true
	default:
		return false
	}
}

func parseGitOpsAgentProfileFile(content, sourcePath string) (*gitOpsAgentProfilePlan, error) {
	var file agentProfilesRequest
	if err := yaml.Unmarshal([]byte(content), &file); err != nil {
		return nil, fmt.Errorf("failed to parse agent profile GitOps file '%s': %w", sourcePath, err)
	}
	requestedDefault := requestedAgentProfileDefault(file)
	defaultProfile := requestedDefault
	if defaultProfile == "" {
		defaultProfile = models.DefaultAgentProfileID
	}
	forms := append([]agentProfileForm{}, file.AgentProfiles...)
	forms = append(forms, file.Profiles...)
	if len(forms) == 0 && requestedDefault == "" {
		return nil, fmt.Errorf("agent profile GitOps file '%s' must define at least one profile or default_profile", sourcePath)
	}
	profiles := map[string]models.AgentProfile{}
	for _, form := range forms {
		profile := agentProfileFromForm(form, "gitops")
		profile.ID = models.NormalizeAgentProfileID(profile.ID)
		if profile.ID == "" {
			return nil, fmt.Errorf("agent profile GitOps file '%s' contains a profile without an id", sourcePath)
		}
		if _, exists := profiles[profile.ID]; exists {
			return nil, fmt.Errorf("agent profile GitOps file '%s' defines profile %q more than once", sourcePath, profile.ID)
		}
		if err := validateAgentProfileDefinition(profile); err != nil {
			return nil, fmt.Errorf("invalid agent profile in GitOps file '%s': %w", sourcePath, err)
		}
		profiles[profile.ID] = profile
	}
	defaultDefinition, ok := profiles[defaultProfile]
	if !ok {
		defaultDefinition, ok = models.BuiltInAgentProfiles()[defaultProfile]
	}
	if !ok {
		return nil, fmt.Errorf("agent profile GitOps file '%s' sets default profile %q but does not define it or reference a built-in profile", sourcePath, defaultProfile)
	}
	if !defaultDefinition.Enabled {
		return nil, fmt.Errorf("agent profile GitOps file '%s' sets disabled profile %q as default", sourcePath, defaultProfile)
	}
	return &gitOpsAgentProfilePlan{defaultProfile: defaultProfile, profiles: profiles, sourcePath: sourcePath}, nil
}

func (a *App) buildRuntimeAgentProfiles(ctx context.Context) (runtimeAgentProfiles, error) {
	return a.buildRuntimeAgentProfilesForTeam(ctx, nil)
}

func (a *App) buildRuntimeAgentProfilesForTeam(ctx context.Context, teamID *int) (runtimeAgentProfiles, error) {
	effectiveProfiles, defaultProfile, err := a.effectiveAgentProfilesForTeam(ctx, teamID)
	if err != nil {
		return runtimeAgentProfiles{}, err
	}
	profiles := make(map[string]runtimeAgentProfile, len(effectiveProfiles))
	for id, profile := range effectiveProfiles {
		profile = models.NormalizeAgentProfile(profile)
		profiles[id] = runtimeAgentProfile{
			DisplayName:  profile.DisplayName,
			Role:         profile.Role,
			Instructions: profile.Instructions,
			Enabled:      profile.Enabled,
			Source:       profile.Source,
		}
	}
	return runtimeAgentProfiles{DefaultProfile: defaultProfile, Profiles: profiles}, nil
}

func (a *App) buildAgentProfileViews(ctx context.Context) ([]agentProfileView, error) {
	profiles, stored, err := a.effectiveAgentProfiles(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	views := make([]agentProfileView, 0, len(names))
	for _, name := range names {
		profile := models.NormalizeAgentProfile(profiles[name])
		refs, _ := a.findAgentProfileReferences(name)
		record, hasStored := stored[name]
		view := agentProfileView{
			AgentProfile: profile,
			UsageCount:   len(refs),
			References:   refs,
			ReadOnly:     profile.BuiltIn || strings.EqualFold(profile.Source, "gitops"),
		}
		if hasStored {
			view.SourcePath = record.ConfigSourcePath
			if !record.UpdatedAt.IsZero() {
				view.LastUpdated = record.UpdatedAt.Format(time.RFC3339)
			}
		}
		views = append(views, view)
	}
	return views, nil
}

func (a *App) agentProfileViewVisible(r *http.Request, view agentProfileView) (bool, error) {
	if view.BuiltIn {
		return true, nil
	}
	return a.aiResourceVisible(r, agentProfileAccessSpec, view.ID)
}

func builtInAgentProfileID(profileID string) bool {
	_, ok := models.BuiltInAgentProfiles()[models.NormalizeAgentProfileID(profileID)]
	return ok
}

func (a *App) handleListAgentProfiles(w http.ResponseWriter, r *http.Request) {
	views, err := a.buildAgentProfileViews(r.Context())
	if err != nil {
		http.Error(w, "failed to load agent profiles", http.StatusInternalServerError)
		return
	}
	filtered := make([]agentProfileView, 0, len(views))
	visibleIDs := make([]string, 0, len(views))
	for _, view := range views {
		visible, err := a.agentProfileViewVisible(r, view)
		if err != nil {
			http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
			return
		}
		if !visible {
			continue
		}
		filtered = append(filtered, view)
		visibleIDs = append(visibleIDs, view.ID)
	}
	defaultProfile, err := a.effectiveAgentProfileDefault(r.Context(), nil)
	if err != nil {
		http.Error(w, "failed to load default agent profile", http.StatusInternalServerError)
		return
	}
	defaultProfile = aiResourceVisibleDefault(defaultProfile, visibleIDs)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(agentProfilesResponse{DefaultProfile: defaultProfile, Profiles: filtered})
}

func (a *App) handleGetAgentProfile(w http.ResponseWriter, r *http.Request) {
	profileID := models.NormalizeAgentProfileID(r.PathValue("profileID"))
	views, err := a.buildAgentProfileViews(r.Context())
	if err != nil {
		http.Error(w, "failed to load agent profiles", http.StatusInternalServerError)
		return
	}
	for _, view := range views {
		if view.ID == profileID {
			if !view.BuiltIn && !a.requireAIResourceVisible(w, r, agentProfileAccessSpec, profileID) {
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(view)
			return
		}
	}
	http.Error(w, "agent profile not found", http.StatusNotFound)
}

func (a *App) handleCreateAgentProfile(w http.ResponseWriter, r *http.Request) {
	var payload agentProfileForm
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid agent profile payload", http.StatusBadRequest)
		return
	}
	profile := agentProfileFromForm(payload, "ui")
	if err := validateAgentProfileDefinition(profile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !a.requireAIResourceWrite(w, r, agentProfileAccessSpec, profile.ID) {
		return
	}
	profiles, _, err := a.effectiveAgentProfiles(r.Context())
	if err != nil {
		http.Error(w, "failed to inspect agent profiles", http.StatusInternalServerError)
		return
	}
	if existing, ok := profiles[profile.ID]; ok {
		if existing.BuiltIn {
			http.Error(w, "built-in agent profiles cannot be overwritten; duplicate with a new id", http.StatusConflict)
			return
		}
		http.Error(w, "agent profile already exists", http.StatusConflict)
		return
	}
	if err := a.persistAgentProfileToDB(r.Context(), profile); err != nil {
		http.Error(w, "failed to persist agent profile", http.StatusInternalServerError)
		return
	}
	views, err := a.buildAgentProfileViews(r.Context())
	if err != nil {
		http.Error(w, "failed to load agent profiles", http.StatusInternalServerError)
		return
	}
	filtered := make([]agentProfileView, 0, len(views))
	visibleIDs := make([]string, 0, len(views))
	for _, view := range views {
		visible, err := a.agentProfileViewVisible(r, view)
		if err != nil {
			http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
			return
		}
		if !visible {
			continue
		}
		filtered = append(filtered, view)
		visibleIDs = append(visibleIDs, view.ID)
	}
	defaultProfile, err := a.effectiveAgentProfileDefault(r.Context(), nil)
	if err != nil {
		http.Error(w, "failed to load default agent profile", http.StatusInternalServerError)
		return
	}
	defaultProfile = aiResourceVisibleDefault(defaultProfile, visibleIDs)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(agentProfilesResponse{DefaultProfile: defaultProfile, Profiles: filtered})
}

func (a *App) handleUpsertAgentProfile(w http.ResponseWriter, r *http.Request) {
	profileID := models.NormalizeAgentProfileID(r.PathValue("profileID"))
	if profileID == "" {
		http.Error(w, "agent profile id is required", http.StatusBadRequest)
		return
	}
	var payload agentProfileForm
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid agent profile payload", http.StatusBadRequest)
		return
	}
	if payload.ID != "" && !strings.EqualFold(models.NormalizeAgentProfileID(payload.ID), profileID) {
		http.Error(w, "agent profile id in path and payload must match", http.StatusBadRequest)
		return
	}
	payload.ID = profileID
	profile := agentProfileFromForm(payload, "ui")
	if err := validateAgentProfileDefinition(profile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !a.requireAIResourceWrite(w, r, agentProfileAccessSpec, profileID) {
		return
	}
	profiles, _, err := a.effectiveAgentProfiles(r.Context())
	if err != nil {
		http.Error(w, "failed to inspect agent profiles", http.StatusInternalServerError)
		return
	}
	if existing, ok := profiles[profileID]; ok && (existing.BuiltIn || strings.EqualFold(existing.Source, "gitops")) {
		http.Error(w, "agent profile is read-only", http.StatusForbidden)
		return
	}
	defaultProfile, err := a.effectiveAgentProfileDefault(r.Context(), profiles)
	if err != nil {
		http.Error(w, "failed to inspect default agent profile", http.StatusInternalServerError)
		return
	}
	if strings.EqualFold(defaultProfile, profileID) && !profile.Enabled {
		http.Error(w, "default agent profile cannot be disabled", http.StatusBadRequest)
		return
	}
	if err := a.persistAgentProfileToDB(r.Context(), profile); err != nil {
		http.Error(w, "failed to persist agent profile", http.StatusInternalServerError)
		return
	}
	a.handleListAgentProfiles(w, r)
}

func (a *App) handleDeleteAgentProfile(w http.ResponseWriter, r *http.Request) {
	profileID := models.NormalizeAgentProfileID(r.PathValue("profileID"))
	if profileID == "" {
		http.Error(w, "agent profile id is required", http.StatusBadRequest)
		return
	}
	profiles, _, err := a.effectiveAgentProfiles(r.Context())
	if err != nil {
		http.Error(w, "failed to inspect agent profiles", http.StatusInternalServerError)
		return
	}
	profile, ok := profiles[profileID]
	if !ok {
		http.Error(w, "agent profile not found", http.StatusNotFound)
		return
	}
	if profile.BuiltIn || strings.EqualFold(profile.Source, "gitops") {
		http.Error(w, "agent profile is read-only", http.StatusForbidden)
		return
	}
	if !a.requireAIResourceWrite(w, r, agentProfileAccessSpec, profileID) {
		return
	}
	defaultProfile, err := a.effectiveAgentProfileDefault(r.Context(), profiles)
	if err != nil {
		http.Error(w, "failed to inspect default agent profile", http.StatusInternalServerError)
		return
	}
	if strings.EqualFold(defaultProfile, profileID) {
		http.Error(w, "default agent profile cannot be deleted", http.StatusBadRequest)
		return
	}
	refs, err := a.findAgentProfileReferences(profileID)
	if err != nil {
		http.Error(w, "failed to inspect agent profile usage", http.StatusInternalServerError)
		return
	}
	if len(refs) > 0 && !strings.EqualFold(r.URL.Query().Get("force"), "true") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":      "agent profile is still referenced",
			"references": refs,
		})
		return
	}
	if a.db != nil {
		if _, err := a.db.Exec(r.Context(), `DELETE FROM agent_profiles WHERE id = $1 AND source <> 'gitops'`, profileID); err != nil {
			http.Error(w, "failed to delete agent profile", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleSetDefaultAgentProfile(w http.ResponseWriter, r *http.Request) {
	if !a.requireAAADecision(w, r, "system.update", model.ResourceRef{Type: "system", ID: "agent-profiles"}) {
		return
	}
	var payload agentProfileDefaultRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid default agent profile payload", http.StatusBadRequest)
		return
	}
	defaultProfile := normalizeAgentProfileDefault(payload.DefaultProfile)
	if defaultProfile == "" {
		defaultProfile = models.DefaultAgentProfileID
	}
	profiles, _, err := a.effectiveAgentProfiles(r.Context())
	if err != nil {
		http.Error(w, "failed to inspect agent profiles", http.StatusInternalServerError)
		return
	}
	if err := validateDefaultAgentProfile(defaultProfile, profiles); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.persistAgentProfileDefaultToDB(r.Context(), defaultProfile, profiles); err != nil {
		http.Error(w, "failed to persist default agent profile", http.StatusInternalServerError)
		return
	}
	a.handleListAgentProfiles(w, r)
}

func (a *App) handleGetAgentProfileUsage(w http.ResponseWriter, r *http.Request) {
	profileID := models.NormalizeAgentProfileID(r.PathValue("profileID"))
	if profileID == "" {
		http.Error(w, "agent profile id is required", http.StatusBadRequest)
		return
	}
	if !builtInAgentProfileID(profileID) && !a.requireAIResourceVisible(w, r, agentProfileAccessSpec, profileID) {
		return
	}
	refs, err := a.findAgentProfileReferences(profileID)
	if err != nil {
		http.Error(w, "failed to inspect agent profile usage", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(agentProfileUsageResponse{
		ProfileID:  profileID,
		UsageCount: len(refs),
		References: refs,
	})
}

func (a *App) handleValidateAgentProfile(w http.ResponseWriter, r *http.Request) {
	var payload agentProfileForm
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid agent profile payload", http.StatusBadRequest)
		return
	}
	profile := agentProfileFromForm(payload, "ui")
	err := validateAgentProfileDefinition(profile)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"valid": false,
			"error": err.Error(),
		})
		return
	}
	if !a.requireAIResourceWrite(w, r, agentProfileAccessSpec, profile.ID) {
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"valid": true})
}

func writeAgentProfileStoreError(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, sql.ErrNoRows):
		http.Error(w, "agent profile not found", http.StatusNotFound)
	default:
		http.Error(w, "agent profile request failed", http.StatusInternalServerError)
	}
}

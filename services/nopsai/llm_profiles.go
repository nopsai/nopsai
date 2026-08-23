package nopsai

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/internal/credentials"
	"nopsai/services/nopsai/pkg/validation"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

const llmProfilesRuntimeEnv = "NOPSAI_LLM_PROFILES"

var llmProfileSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS llm_profile_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT '',
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE IF NOT EXISTS llm_profiles (
		name TEXT PRIMARY KEY,
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
		prompt_cache JSONB NOT NULL DEFAULT '{}'::jsonb,
		provider_state JSONB NOT NULL DEFAULT '{}'::jsonb,
		extra JSONB NOT NULL DEFAULT '{}'::jsonb,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`ALTER TABLE llm_profiles ADD COLUMN IF NOT EXISTS credential_ref TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE llm_profiles ADD COLUMN IF NOT EXISTS timeout_seconds INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE llm_profiles ADD COLUMN IF NOT EXISTS max_tokens INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE llm_profiles ADD COLUMN IF NOT EXISTS temperature DOUBLE PRECISION`,
	`ALTER TABLE llm_profiles ADD COLUMN IF NOT EXISTS prompt_cache JSONB NOT NULL DEFAULT '{}'::jsonb`,
	`ALTER TABLE llm_profiles ADD COLUMN IF NOT EXISTS provider_state JSONB NOT NULL DEFAULT '{}'::jsonb`,
	`ALTER TABLE llm_profiles ADD COLUMN IF NOT EXISTS extra JSONB NOT NULL DEFAULT '{}'::jsonb`,
}

type llmProfileForm struct {
	Name           string                  `json:"name" yaml:"name"`
	Provider       string                  `json:"provider" yaml:"provider"`
	Model          string                  `json:"model" yaml:"model"`
	BaseURL        string                  `json:"base_url" yaml:"base_url"`
	CredentialRef  string                  `json:"credential_ref" yaml:"credential_ref"`
	AllowedScopes  []string                `json:"allowed_scopes" yaml:"allowed_scopes"`
	Reasoning      string                  `json:"reasoning" yaml:"reasoning"`
	Thinking       *bool                   `json:"thinking,omitempty" yaml:"thinking,omitempty"`
	TimeoutSeconds int                     `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
	MaxTokens      int                     `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`
	Temperature    *float64                `json:"temperature,omitempty" yaml:"temperature,omitempty"`
	PromptCache    config.LLMFeatureConfig `json:"prompt_cache,omitempty" yaml:"prompt_cache,omitempty"`
	ProviderState  config.LLMFeatureConfig `json:"provider_state,omitempty" yaml:"provider_state,omitempty"`
	Pricing        *config.LLMPricing      `json:"pricing,omitempty" yaml:"pricing,omitempty"`
	Extra          map[string]string       `json:"extra,omitempty" yaml:"extra,omitempty"`

	// pricingDeclared records whether the request actually carried a "pricing"
	// key. An omitted key and an explicit null both decode to a nil pointer, and
	// a client that does not know about rate cards would otherwise delete one the
	// operator declared just by saving an unrelated field.
	pricingDeclared bool
}

// UnmarshalJSON decodes the form and remembers whether pricing was stated, so a
// save can tell "leave the rate card alone" apart from "this model has none".
func (f *llmProfileForm) UnmarshalJSON(data []byte) error {
	type formFields llmProfileForm
	var decoded formFields
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var declared map[string]json.RawMessage
	if err := json.Unmarshal(data, &declared); err != nil {
		return err
	}
	*f = llmProfileForm(decoded)
	_, f.pricingDeclared = declared["pricing"]
	return nil
}

// llmProfilePricingForSave resolves the rate card a save should persist. A form
// that states pricing wins outright. A form that stays silent keeps whatever the
// profile already had, and a brand new model falls back to the provider default,
// which is explicit zeroes for a self-hosted provider and nothing for a hosted
// one whose rates only the operator knows.
func llmProfilePricingForSave(form llmProfileForm, existing map[string]config.LLMProfile, name string) *config.LLMPricing {
	if form.pricingDeclared {
		return clonePricing(form.Pricing)
	}
	if current, ok := existing[config.NormalizeLLMProfileName(name)]; ok && current.Pricing != nil {
		return clonePricing(current.Pricing)
	}
	return config.DefaultLLMPricingForProvider(form.Provider)
}

type llmProfileView struct {
	llmProfileForm
	Status         string   `json:"status"`
	Validation     string   `json:"validation,omitempty"`
	References     []string `json:"references,omitempty"`
	AllowedInScope bool     `json:"allowed_in_scope"`
	DisabledReason string   `json:"disabled_reason,omitempty"`
}

type llmProfilesResponse struct {
	DefaultProfile string           `json:"default_profile"`
	Profiles       []llmProfileView `json:"profiles"`
}

type llmProfilesRequest struct {
	DefaultProfile    string                       `json:"default_profile" yaml:"default_profile"`
	LLMDefaultProfile string                       `json:"llm_default_profile" yaml:"llm_default_profile"`
	Profiles          []llmProfileForm             `json:"profiles" yaml:"profiles"`
	LLMProfiles       map[string]config.LLMProfile `json:"llm_profiles" yaml:"llm_profiles"`
}

type runtimeLLMProfile struct {
	Provider       string                  `json:"provider"`
	Model          string                  `json:"model,omitempty"`
	BaseURL        string                  `json:"base_url,omitempty"`
	APIKey         string                  `json:"api_key,omitempty"`
	CredentialRef  string                  `json:"credential_ref,omitempty"`
	AllowedScopes  []string                `json:"allowed_scopes,omitempty"`
	Reasoning      string                  `json:"reasoning,omitempty"`
	Thinking       *bool                   `json:"thinking,omitempty"`
	TimeoutSeconds int                     `json:"timeout_seconds,omitempty"`
	MaxTokens      int                     `json:"max_tokens,omitempty"`
	Temperature    *float64                `json:"temperature,omitempty"`
	PromptCache    config.LLMFeatureConfig `json:"prompt_cache,omitempty"`
	ProviderState  config.LLMFeatureConfig `json:"provider_state,omitempty"`
	Extra          map[string]string       `json:"extra,omitempty"`
}

type runtimeLLMProfiles struct {
	DefaultProfile string                       `json:"default_profile"`
	Profiles       map[string]runtimeLLMProfile `json:"profiles"`
}

func profileFormFromConfig(name string, profile config.LLMProfile) llmProfileForm {
	return llmProfileForm{
		Name:           name,
		Provider:       profile.Provider,
		Model:          profile.Model,
		BaseURL:        profile.BaseURL,
		CredentialRef:  profile.CredentialRef,
		AllowedScopes:  append([]string(nil), profile.AllowedScopes...),
		Reasoning:      profile.Reasoning,
		Thinking:       profile.Thinking,
		TimeoutSeconds: profile.TimeoutSeconds,
		MaxTokens:      profile.MaxTokens,
		Temperature:    profile.Temperature,
		PromptCache:    profile.PromptCache,
		ProviderState:  profile.ProviderState,
		Pricing:        clonePricing(profile.Pricing),
		Extra:          cloneStringMap(profile.Extra),
	}
}

func clonePricing(pricing *config.LLMPricing) *config.LLMPricing {
	if pricing == nil {
		return nil
	}
	cloned := *pricing
	if pricing.CachedInputPerMillionUSD != nil {
		rate := *pricing.CachedInputPerMillionUSD
		cloned.CachedInputPerMillionUSD = &rate
	}
	if pricing.CacheWritePerMillionUSD != nil {
		rate := *pricing.CacheWritePerMillionUSD
		cloned.CacheWritePerMillionUSD = &rate
	}
	return &cloned
}

func profileConfigFromForm(form llmProfileForm) config.LLMProfile {
	return config.NormalizeLLMProfile(config.LLMProfile{
		Provider:       form.Provider,
		Model:          form.Model,
		BaseURL:        form.BaseURL,
		CredentialRef:  form.CredentialRef,
		AllowedScopes:  form.AllowedScopes,
		Reasoning:      form.Reasoning,
		Thinking:       form.Thinking,
		TimeoutSeconds: form.TimeoutSeconds,
		MaxTokens:      form.MaxTokens,
		Temperature:    form.Temperature,
		PromptCache:    form.PromptCache,
		ProviderState:  form.ProviderState,
		Pricing:        clonePricing(form.Pricing),
		Extra:          form.Extra,
	})
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func (a *App) llmProfilesSnapshot() (string, map[string]config.LLMProfile) {
	cfg := a.getConfigSnapshot()
	return cfg.EffectiveLLMDefaultProfile(), cfg.EffectiveLLMProfiles()
}

func (a *App) llmProfileCatalogForValidation(cfg config.Config) (string, map[string]validation.LLMProfileDefinition) {
	defaultProfile := cfg.EffectiveLLMDefaultProfile()
	effectiveProfiles := cfg.EffectiveLLMProfiles()
	profiles := make(map[string]validation.LLMProfileDefinition, len(effectiveProfiles))
	for name, profile := range effectiveProfiles {
		profiles[name] = validation.LLMProfileDefinition{AllowedScopes: append([]string(nil), profile.AllowedScopes...)}
	}
	return defaultProfile, profiles
}

func (a *App) validatePipelineLLMProfiles(pipeline *models.Pipeline, scope string) error {
	return a.validatePipelineLLMProfilesForTeam(context.Background(), pipeline, scope, nil)
}

func (a *App) validatePipelineLLMProfilesForTeam(ctx context.Context, pipeline *models.Pipeline, scope string, teamID *int) error {
	if !models.PipelineRequiresLLMProfiles(pipeline) {
		return nil
	}
	cfg := a.getConfigSnapshot()
	defaultProfile, effectiveProfiles, err := a.effectiveLLMProfilesForTeam(ctx, cfg, teamID)
	if err != nil {
		return err
	}
	profiles := make(map[string]validation.LLMProfileDefinition, len(effectiveProfiles))
	for name, profile := range effectiveProfiles {
		profiles[name] = validation.LLMProfileDefinition{AllowedScopes: append([]string(nil), profile.AllowedScopes...)}
	}
	if err := validation.ValidatePipelineLLMProfiles(pipeline, validation.LLMProfileValidationOptions{
		DefaultProfile:        defaultProfile,
		RequireDefaultProfile: false,
		Profiles:              profiles,
		Scope:                 scope,
	}); err != nil {
		return err
	}

	for name := range collectResolvedLLMProfiles(pipeline, defaultProfile) {
		profile := effectiveProfiles[name]
		status, message := a.validateLLMProfileConfiguration(ctx, name, profile)
		if status != "valid" {
			if message == "" {
				message = fmt.Sprintf("LLM profile %q is invalid", name)
			}
			return errors.New(message)
		}
	}
	return nil
}

func collectResolvedLLMProfiles(pipeline *models.Pipeline, defaultProfile string) map[string]bool {
	used := map[string]bool{}
	if pipeline == nil {
		return used
	}
	pipelineProfile := strings.TrimSpace(pipeline.LLMProfile)
	if pipelineProfile == "" {
		pipelineProfile = strings.TrimSpace(defaultProfile)
	}
	if pipelineProfile != "" {
		used[pipelineProfile] = true
	}
	outputProfile := strings.TrimSpace(pipeline.Output.LLMProfile)
	if outputProfile == "" {
		outputProfile = pipelineProfile
	}
	if outputProfile != "" && len(pipeline.Output.Items) > 0 {
		used[outputProfile] = true
	}
	for _, item := range pipeline.Output.Items {
		itemProfile := strings.TrimSpace(item.LLMProfile)
		if itemProfile == "" {
			itemProfile = outputProfile
		}
		if itemProfile != "" {
			used[itemProfile] = true
		}
	}
	for _, step := range pipeline.Steps {
		stepProfile := strings.TrimSpace(step.GetLLMProfile())
		if stepProfile == "" {
			stepProfile = pipelineProfile
		}
		if stepProfile != "" {
			used[stepProfile] = true
		}
		for _, task := range step.GetTasks() {
			taskProfile := strings.TrimSpace(task.LLMProfile)
			if taskProfile == "" {
				taskProfile = stepProfile
			}
			if taskProfile != "" {
				used[taskProfile] = true
			}
		}
	}
	return used
}

func collectExplicitLLMProfileReferencesFromPipeline(pipeline *models.Pipeline, profileName, prefix string) []string {
	var refs []string
	if pipeline == nil {
		return refs
	}
	if strings.EqualFold(strings.TrimSpace(pipeline.LLMProfile), profileName) {
		refs = append(refs, prefix)
	}
	if strings.EqualFold(strings.TrimSpace(pipeline.Output.LLMProfile), profileName) {
		refs = append(refs, prefix+" output")
	}
	for _, item := range pipeline.Output.Items {
		itemName := strings.TrimSpace(item.Name)
		if itemName == "" {
			itemName = "unknown"
		}
		if strings.EqualFold(strings.TrimSpace(item.LLMProfile), profileName) {
			refs = append(refs, fmt.Sprintf("%s output %q", prefix, itemName))
		}
	}
	for _, step := range pipeline.Steps {
		stepName := strings.TrimSpace(step.GetName())
		if stepName == "" {
			stepName = "unknown"
		}
		stepRef := fmt.Sprintf("%s step %q", prefix, stepName)
		if strings.EqualFold(strings.TrimSpace(step.GetLLMProfile()), profileName) {
			refs = append(refs, stepRef)
		}
		for _, task := range step.GetTasks() {
			taskName := strings.TrimSpace(task.Name)
			if taskName == "" {
				taskName = "unknown"
			}
			if strings.EqualFold(strings.TrimSpace(task.LLMProfile), profileName) {
				refs = append(refs, fmt.Sprintf("%s task %q", stepRef, taskName))
			}
		}
	}
	return refs
}

func replaceLLMProfileInPipeline(pipeline *models.Pipeline, oldProfile, newProfile string) bool {
	changed := false
	if pipeline == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(pipeline.LLMProfile), oldProfile) {
		pipeline.LLMProfile = newProfile
		changed = true
	}
	for i := range pipeline.Steps {
		if strings.EqualFold(strings.TrimSpace(pipeline.Steps[i].GetLLMProfile()), oldProfile) {
			pipeline.Steps[i].SetLLMProfile(newProfile)
			changed = true
		}
		if taskStep, ok := pipeline.Steps[i].AsTaskStep(); ok {
			for j := range taskStep.Tasks {
				if strings.EqualFold(strings.TrimSpace(taskStep.Tasks[j].LLMProfile), oldProfile) {
					taskStep.Tasks[j].LLMProfile = newProfile
					changed = true
				}
			}
		}
	}
	return changed
}

func (a *App) findLLMProfileReferences(profileName string) ([]string, error) {
	if a.db == nil {
		return nil, nil
	}

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
		if err := yaml.Unmarshal([]byte(definition), &pipeline); err != nil {
			refs = append(refs, fmt.Sprintf("pipeline %s (unreadable YAML)", configsync.BuildPipelineIdentifier(pathPart, namePart)))
			continue
		}
		refs = append(refs, collectExplicitLLMProfileReferencesFromPipeline(&pipeline, profileName, "pipeline "+configsync.BuildPipelineIdentifier(pathPart, namePart))...)
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
		if strings.EqualFold(strings.TrimSpace(step.GetLLMProfile()), profileName) {
			refs = append(refs, "step "+stepID)
		}
		for _, task := range step.GetTasks() {
			if strings.EqualFold(strings.TrimSpace(task.LLMProfile), profileName) {
				refs = append(refs, fmt.Sprintf("step %s task %q", stepID, task.Name))
			}
		}
	}
	if err := stepRows.Err(); err != nil {
		return nil, err
	}

	sort.Strings(refs)
	return refs, nil
}

func (a *App) migrateLLMProfileReferences(oldProfile, newProfile string) error {
	if a.db == nil {
		return nil
	}

	ctx := context.Background()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	pipelineRows, err := tx.Query(ctx, "SELECT path, name, definition FROM pipelines")
	if err != nil {
		return err
	}
	type pipelineUpdate struct {
		path       string
		name       string
		definition string
	}
	var pipelineUpdates []pipelineUpdate
	for pipelineRows.Next() {
		var pathPart, namePart, definition string
		if err := pipelineRows.Scan(&pathPart, &namePart, &definition); err != nil {
			pipelineRows.Close()
			return err
		}
		var pipeline models.Pipeline
		if err := yaml.Unmarshal([]byte(definition), &pipeline); err != nil {
			pipelineRows.Close()
			return err
		}
		if replaceLLMProfileInPipeline(&pipeline, oldProfile, newProfile) {
			nextDef, err := yaml.Marshal(&pipeline)
			if err != nil {
				pipelineRows.Close()
				return err
			}
			pipelineUpdates = append(pipelineUpdates, pipelineUpdate{path: pathPart, name: namePart, definition: string(nextDef)})
		}
	}
	if err := pipelineRows.Err(); err != nil {
		pipelineRows.Close()
		return err
	}
	pipelineRows.Close()

	for _, update := range pipelineUpdates {
		if _, err := tx.Exec(ctx, "UPDATE pipelines SET definition = $1, updated_at = NOW() WHERE path = $2 AND name = $3", update.definition, update.path, update.name); err != nil {
			return err
		}
	}

	stepRows, err := tx.Query(ctx, "SELECT path, name, definition FROM steps")
	if err != nil {
		return err
	}
	type stepUpdate struct {
		path       string
		name       string
		definition string
	}
	var stepUpdates []stepUpdate
	for stepRows.Next() {
		var pathPart, namePart, definition string
		if err := stepRows.Scan(&pathPart, &namePart, &definition); err != nil {
			stepRows.Close()
			return err
		}
		var step models.PipelineStep
		if err := yaml.Unmarshal([]byte(definition), &step); err != nil {
			stepRows.Close()
			return err
		}
		wrapper := models.Pipeline{Steps: []models.PipelineStep{step}}
		if replaceLLMProfileInPipeline(&wrapper, oldProfile, newProfile) {
			nextDef, err := yaml.Marshal(wrapper.Steps[0])
			if err != nil {
				stepRows.Close()
				return err
			}
			stepUpdates = append(stepUpdates, stepUpdate{path: pathPart, name: namePart, definition: string(nextDef)})
		}
	}
	if err := stepRows.Err(); err != nil {
		stepRows.Close()
		return err
	}
	stepRows.Close()

	for _, update := range stepUpdates {
		if _, err := tx.Exec(ctx, "UPDATE steps SET definition = $1, updated_at = NOW() WHERE path = $2 AND name = $3", update.definition, update.path, update.name); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (a *App) setLLMProfiles(defaultProfile string, profiles map[string]config.LLMProfile) {
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()

	next := configWithLLMProfiles(*a.cfg, defaultProfile, profiles)
	a.cfg.LLMDefaultProfile = next.LLMDefaultProfile
	a.cfg.LLMProfiles = next.LLMProfiles
}

func configWithLLMProfiles(base config.Config, defaultProfile string, profiles map[string]config.LLMProfile) config.Config {
	base.LLMDefaultProfile = config.NormalizeLLMProfileName(defaultProfile)
	if base.LLMDefaultProfile == "" {
		base.LLMDefaultProfile = config.DefaultLLMProfileName
	}
	base.LLMProfiles = config.NormalizeLLMProfiles(profiles)
	return base
}

func ensureLLMProfileSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin LLM profile schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range llmProfileSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply LLM profile schema statement %d: %w", idx+1, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit LLM profile schema transaction: %w", err)
	}
	return nil
}

func (a *App) loadOrSeedLLMProfilesConfig(ctx context.Context) error {
	if a == nil || a.db == nil {
		return nil
	}

	defaultProfile, profiles, found, err := a.loadLLMProfilesFromDB(ctx)
	if err != nil {
		return err
	}
	if found {
		a.setLLMProfiles(defaultProfile, profiles)
		return nil
	}

	cfg := a.getConfigSnapshot()
	profiles = cfg.EffectiveLLMProfiles()
	if len(profiles) == 0 {
		return nil
	}
	return a.persistLLMProfilesToDB(ctx, cfg.EffectiveLLMDefaultProfile(), profiles)
}

func (a *App) loadLLMProfilesFromDB(ctx context.Context) (string, map[string]config.LLMProfile, bool, error) {
	if a == nil || a.db == nil {
		return "", nil, false, nil
	}

	defaultProfile := config.DefaultLLMProfileName
	var storedDefault string
	err := a.db.QueryRow(ctx, `SELECT value FROM llm_profile_settings WHERE key = 'default_profile'`).Scan(&storedDefault)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", nil, false, fmt.Errorf("load default LLM profile from database: %w", err)
	}
	if strings.TrimSpace(storedDefault) != "" {
		defaultProfile = config.NormalizeLLMProfileName(storedDefault)
	}

	rows, err := a.db.Query(ctx, `
		SELECT name, provider, model, base_url, credential_ref, allowed_scopes, reasoning, thinking,
		       timeout_seconds, max_tokens, temperature, prompt_cache, provider_state, extra
		FROM llm_profiles
		ORDER BY name ASC
	`)
	if err != nil {
		return "", nil, false, fmt.Errorf("load LLM profiles from database: %w", err)
	}
	defer rows.Close()

	profiles := map[string]config.LLMProfile{}
	for rows.Next() {
		var (
			name             string
			profile          config.LLMProfile
			allowedScopesRaw []byte
			thinking         sql.NullBool
			temperature      sql.NullFloat64
			promptCacheRaw   []byte
			providerStateRaw []byte
			extraRaw         []byte
		)
		if err := rows.Scan(
			&name,
			&profile.Provider,
			&profile.Model,
			&profile.BaseURL,
			&profile.CredentialRef,
			&allowedScopesRaw,
			&profile.Reasoning,
			&thinking,
			&profile.TimeoutSeconds,
			&profile.MaxTokens,
			&temperature,
			&promptCacheRaw,
			&providerStateRaw,
			&extraRaw,
		); err != nil {
			return "", nil, false, fmt.Errorf("scan LLM profile from database: %w", err)
		}
		if len(allowedScopesRaw) > 0 {
			if err := json.Unmarshal(allowedScopesRaw, &profile.AllowedScopes); err != nil {
				return "", nil, false, fmt.Errorf("parse allowed scopes for LLM profile %q: %w", name, err)
			}
		}
		if thinking.Valid {
			value := thinking.Bool
			profile.Thinking = &value
		}
		if temperature.Valid {
			value := temperature.Float64
			profile.Temperature = &value
		}
		if len(promptCacheRaw) > 0 {
			if err := json.Unmarshal(promptCacheRaw, &profile.PromptCache); err != nil {
				return "", nil, false, fmt.Errorf("parse prompt cache settings for LLM profile %q: %w", name, err)
			}
		}
		if len(providerStateRaw) > 0 {
			if err := json.Unmarshal(providerStateRaw, &profile.ProviderState); err != nil {
				return "", nil, false, fmt.Errorf("parse provider state settings for LLM profile %q: %w", name, err)
			}
		}
		if len(extraRaw) > 0 {
			if err := json.Unmarshal(extraRaw, &profile.Extra); err != nil {
				return "", nil, false, fmt.Errorf("parse extra options for LLM profile %q: %w", name, err)
			}
		}
		profiles[config.NormalizeLLMProfileName(name)] = config.NormalizeLLMProfile(profile)
	}
	if err := rows.Err(); err != nil {
		return "", nil, false, fmt.Errorf("iterate LLM profiles from database: %w", err)
	}
	if len(profiles) == 0 {
		return "", nil, false, nil
	}
	if _, ok := profiles[defaultProfile]; !ok {
		defaultProfile = config.DefaultLLMProfileName
		if _, ok := profiles[defaultProfile]; !ok {
			names := make([]string, 0, len(profiles))
			for name := range profiles {
				names = append(names, name)
			}
			sort.Strings(names)
			defaultProfile = names[0]
		}
	}
	return defaultProfile, profiles, true, nil
}

func (a *App) persistLLMProfilesToDB(ctx context.Context, defaultProfile string, profiles map[string]config.LLMProfile) error {
	if a == nil || a.db == nil {
		return nil
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin LLM profile persistence transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := persistLLMProfilesToTx(ctx, tx, defaultProfile, profiles); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit LLM profile persistence transaction: %w", err)
	}
	return nil
}

func persistLLMProfilesToTx(ctx context.Context, tx pgx.Tx, defaultProfile string, profiles map[string]config.LLMProfile) error {
	defaultProfile = config.NormalizeLLMProfileName(defaultProfile)
	if defaultProfile == "" {
		defaultProfile = config.DefaultLLMProfileName
	}
	profiles = config.NormalizeLLMProfiles(profiles)

	if _, err := tx.Exec(ctx, `DELETE FROM llm_profiles`); err != nil {
		return fmt.Errorf("clear LLM profiles: %w", err)
	}

	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		profile := config.NormalizeLLMProfile(profiles[name])
		allowedScopesJSON, err := json.Marshal(profile.AllowedScopes)
		if err != nil {
			return fmt.Errorf("encode allowed scopes for LLM profile %q: %w", name, err)
		}
		var thinking any
		if profile.Thinking != nil {
			thinking = *profile.Thinking
		}
		var temperature any
		if profile.Temperature != nil {
			temperature = *profile.Temperature
		}
		promptCacheJSON, err := json.Marshal(profile.PromptCache)
		if err != nil {
			return fmt.Errorf("encode prompt cache settings for LLM profile %q: %w", name, err)
		}
		providerStateJSON, err := json.Marshal(profile.ProviderState)
		if err != nil {
			return fmt.Errorf("encode provider state settings for LLM profile %q: %w", name, err)
		}
		extraJSON, err := json.Marshal(profile.Extra)
		if err != nil {
			return fmt.Errorf("encode extra options for LLM profile %q: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO llm_profiles (
				name, provider, model, base_url, credential_ref, allowed_scopes, reasoning, thinking,
				timeout_seconds, max_tokens, temperature, prompt_cache, provider_state, extra, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11, $12::jsonb, $13::jsonb, $14::jsonb, NOW())
		`,
			name,
			profile.Provider,
			profile.Model,
			profile.BaseURL,
			profile.CredentialRef,
			string(allowedScopesJSON),
			profile.Reasoning,
			thinking,
			profile.TimeoutSeconds,
			profile.MaxTokens,
			temperature,
			string(promptCacheJSON),
			string(providerStateJSON),
			string(extraJSON),
		); err != nil {
			return fmt.Errorf("persist LLM profile %q: %w", name, err)
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO llm_profile_settings (key, value, updated_at)
		VALUES ('default_profile', $1, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`, defaultProfile); err != nil {
		return fmt.Errorf("persist default LLM profile: %w", err)
	}
	return nil
}

func (a *App) persistLLMProfilesConfig(ctx context.Context, cfg config.Config) error {
	if err := a.ensureLLMProfileCredentialReferences(ctx, cfg.EffectiveLLMProfiles(), credentialActorFromContext(ctx)); err != nil {
		return err
	}
	dbBacked := a != nil && a.db != nil
	if err := a.persistLLMProfilesToDB(ctx, cfg.EffectiveLLMDefaultProfile(), cfg.EffectiveLLMProfiles()); err != nil {
		return err
	}
	return a.persistLLMProfilesBootstrapConfig(cfg, !dbBacked)
}

func (a *App) persistLLMProfilesBootstrapConfig(cfg config.Config, required bool) error {
	if a == nil || a.configPath == "" {
		return nil
	}

	existing := map[string]interface{}{}
	if contents, err := os.ReadFile(a.configPath); err == nil {
		if len(contents) > 0 {
			_ = yaml.Unmarshal(contents, &existing)
		}
	} else if !os.IsNotExist(err) {
		if !required {
			log.Warn().Err(err).Str("config_path", a.configPath).Msg("Failed to sync LLM profiles to bootstrap config after database persistence")
			return nil
		}
		return err
	}

	existing["llm_default_profile"] = cfg.EffectiveLLMDefaultProfile()
	existing["llm_profiles"] = cfg.LLMProfiles
	for _, legacyKey := range []string{
		"llm_provider",
		"gemini_api_key",
		"gemini_model",
		"lmstudio_base_url",
		"lmstudio_api_key",
		"lmstudio_model",
		"lmstudio_reasoning",
	} {
		delete(existing, legacyKey)
	}

	contents, err := yaml.Marshal(existing)
	if err != nil {
		return err
	}
	if err := os.WriteFile(a.configPath, contents, 0o644); err != nil {
		if !required {
			log.Warn().Err(err).Str("config_path", a.configPath).Msg("Failed to sync LLM profiles to bootstrap config after database persistence")
			return nil
		}
		return err
	}
	return nil
}

func validateLLMProfileDefinition(name string, profile config.LLMProfile) (string, string) {
	profile = config.NormalizeLLMProfile(profile)
	if profile.TimeoutSeconds < 0 {
		return "invalid", fmt.Sprintf("LLM profile %q has invalid timeout_seconds %d", name, profile.TimeoutSeconds)
	}
	if profile.MaxTokens < 0 {
		return "invalid", fmt.Sprintf("LLM profile %q has invalid max_tokens %d", name, profile.MaxTokens)
	}
	if profile.MaxTokens > 0 && !config.LLMProviderSupportsMaxTokens(profile.Provider) {
		return "invalid", fmt.Sprintf("LLM profile %q provider %q does not support max_tokens", name, profile.Provider)
	}
	if status, message := validateLLMProfilePricing(name, profile.Pricing); status != "valid" {
		return status, message
	}
	if !config.SupportedLLMFeatureMode(profile.PromptCache.Mode) {
		return "invalid", fmt.Sprintf("LLM profile %q has invalid prompt_cache.mode %q", name, profile.PromptCache.Mode)
	}
	if !config.SupportedLLMFeatureMode(profile.ProviderState.Mode) {
		return "invalid", fmt.Sprintf("LLM profile %q has invalid provider_state.mode %q", name, profile.ProviderState.Mode)
	}
	if profile.Temperature != nil {
		minimum, maximum, supported := config.LLMProviderTemperatureRange(profile.Provider)
		if !supported {
			return "invalid", fmt.Sprintf("LLM profile %q provider %q does not support temperature", name, profile.Provider)
		}
		if *profile.Temperature < minimum || *profile.Temperature > maximum {
			return "invalid", fmt.Sprintf(
				"LLM profile %q has invalid temperature %g for provider %q; expected %g to %g",
				name,
				*profile.Temperature,
				profile.Provider,
				minimum,
				maximum,
			)
		}
	}
	if !config.LLMProviderSupportsGenericReasoning(profile.Provider) {
		if strings.TrimSpace(profile.Reasoning) != "" {
			return "invalid", fmt.Sprintf("LLM profile %q provider %q does not support the generic reasoning setting", name, profile.Provider)
		}
		if profile.Thinking != nil {
			return "invalid", fmt.Sprintf("LLM profile %q provider %q does not support the generic thinking setting", name, profile.Provider)
		}
	}
	switch profile.Provider {
	case config.LLMProviderGemini:
		if strings.TrimSpace(profile.Model) == "" {
			return "invalid", fmt.Sprintf("LLM profile %q is missing model", name)
		}
		if !llmProfileHasCredential(profile) {
			return "invalid", fmt.Sprintf("LLM profile %q is missing credential_ref", name)
		}
	case config.LLMProviderLMStudio:
		if strings.TrimSpace(profile.BaseURL) == "" {
			return "invalid", fmt.Sprintf("LLM profile %q is missing base_url", name)
		}
		if !config.IsValidLMStudioReasoning(profile.Reasoning) {
			return "invalid", fmt.Sprintf("LLM profile %q has invalid reasoning setting %q", name, profile.Reasoning)
		}
	case config.LLMProviderOpenAI,
		config.LLMProviderAnthropic,
		config.LLMProviderGroq,
		config.LLMProviderMistral,
		config.LLMProviderOpenRouter:
		if strings.TrimSpace(profile.Model) == "" {
			return "invalid", fmt.Sprintf("LLM profile %q is missing model", name)
		}
		if !llmProfileHasCredential(profile) {
			return "invalid", fmt.Sprintf("LLM profile %q is missing credential_ref", name)
		}
	case config.LLMProviderOllama:
		if strings.TrimSpace(profile.Model) == "" {
			return "invalid", fmt.Sprintf("LLM profile %q is missing model", name)
		}
		if strings.TrimSpace(profile.BaseURL) == "" {
			return "invalid", fmt.Sprintf("LLM profile %q is missing base_url", name)
		}
	case config.LLMProviderAzureOpenAI:
		if strings.TrimSpace(profile.BaseURL) == "" {
			return "invalid", fmt.Sprintf("LLM profile %q is missing base_url", name)
		}
		if strings.TrimSpace(profile.Model) == "" && strings.TrimSpace(profile.Extra["deployment"]) == "" {
			return "invalid", fmt.Sprintf("LLM profile %q is missing model or extra.deployment", name)
		}
		if !llmProfileHasCredential(profile) {
			return "invalid", fmt.Sprintf("LLM profile %q is missing credential_ref", name)
		}
	default:
		return "invalid", fmt.Sprintf("LLM profile %q uses unsupported provider %q", name, profile.Provider)
	}
	return "valid", ""
}

func (a *App) validateLLMProfileConfiguration(ctx context.Context, name string, profile config.LLMProfile) (string, string) {
	if status, message := validateLLMProfileDefinition(name, profile); status != "valid" {
		return status, message
	}
	profile = config.NormalizeLLMProfile(profile)
	if config.LLMProviderRequiresAPIKey(profile.Provider) {
		value, err := a.resolveLLMProfileAPIKey(ctx, name, profile)
		if err != nil || strings.TrimSpace(value) == "" {
			return "invalid", fmt.Sprintf("LLM profile %q credential %q is unavailable", name, profile.CredentialRef)
		}
	}
	return "valid", ""
}

func (a *App) resolveLLMProfileAPIKey(ctx context.Context, name string, profile config.LLMProfile) (string, error) {
	if strings.TrimSpace(profile.CredentialRef) == "" && strings.TrimSpace(profile.LegacyAPIKeySecret) != "" {
		return strings.TrimSpace(os.Getenv(strings.TrimSpace(profile.LegacyAPIKeySecret))), nil
	}
	return a.resolveCredentialText(ctx, profile.CredentialRef, credentials.Purpose{
		ConsumerService: "nopsai",
		Operation:       "llm.authenticate",
		SubjectType:     "model",
		SubjectID:       name,
	})
}

func llmProfileHasCredential(profile config.LLMProfile) bool {
	return strings.TrimSpace(profile.CredentialRef) != "" || strings.TrimSpace(profile.LegacyAPIKeySecret) != ""
}

func (a *App) buildRuntimeLLMProfiles(ctx context.Context, cfg config.Config) (runtimeLLMProfiles, error) {
	return a.buildRuntimeLLMProfilesForTeam(ctx, cfg, nil)
}

func (a *App) buildRuntimeLLMProfilesForTeam(ctx context.Context, cfg config.Config, teamID *int) (runtimeLLMProfiles, error) {
	defaultProfile, effectiveProfiles, err := a.effectiveLLMProfilesForTeam(ctx, cfg, teamID)
	if err != nil {
		return runtimeLLMProfiles{}, err
	}
	profiles := make(map[string]runtimeLLMProfile, len(effectiveProfiles))
	for name, profile := range effectiveProfiles {
		runtimeProfile, err := a.runtimeLLMProfileForConfig(ctx, name, profile)
		if err != nil {
			return runtimeLLMProfiles{}, err
		}
		profiles[name] = runtimeProfile
	}
	return runtimeLLMProfiles{DefaultProfile: defaultProfile, Profiles: profiles}, nil
}

func (a *App) buildRuntimeLLMProfilesForPipelineTeam(
	ctx context.Context,
	cfg config.Config,
	pipeline *models.Pipeline,
	teamID *int,
) (runtimeLLMProfiles, error) {
	defaultProfile, effectiveProfiles, err := a.effectiveLLMProfilesForTeam(ctx, cfg, teamID)
	if err != nil {
		return runtimeLLMProfiles{}, err
	}
	runtimeDefault, requiredProfiles := requiredLLMProfilesForPipeline(pipeline, defaultProfile)
	if runtimeDefault == "" {
		runtimeDefault = defaultProfile
	}
	profiles := make(map[string]runtimeLLMProfile, len(requiredProfiles))
	names := make([]string, 0, len(requiredProfiles))
	for name := range requiredProfiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		profile, ok := effectiveProfiles[name]
		if !ok {
			return runtimeLLMProfiles{}, fmt.Errorf("LLM profile %q is not configured", name)
		}
		runtimeProfile, err := a.runtimeLLMProfileForConfig(ctx, name, profile)
		if err != nil {
			return runtimeLLMProfiles{}, err
		}
		profiles[name] = runtimeProfile
	}
	return runtimeLLMProfiles{DefaultProfile: runtimeDefault, Profiles: profiles}, nil
}

func (a *App) runtimeLLMProfileForConfig(ctx context.Context, name string, profile config.LLMProfile) (runtimeLLMProfile, error) {
	normalized := config.NormalizeLLMProfile(profile)
	baseURL := config.EffectiveLLMProfileBaseURL(normalized)
	if normalized.Provider == config.LLMProviderLMStudio {
		baseURL = containerReachableLMStudioBaseURL(baseURL)
	}
	apiKey, err := a.resolveLLMProfileAPIKey(ctx, name, normalized)
	if err != nil {
		return runtimeLLMProfile{}, fmt.Errorf("resolve credential for LLM profile %q: %w", name, err)
	}
	return runtimeLLMProfile{
		Provider:       normalized.Provider,
		Model:          normalized.Model,
		BaseURL:        baseURL,
		APIKey:         apiKey,
		CredentialRef:  normalized.CredentialRef,
		AllowedScopes:  append([]string(nil), normalized.AllowedScopes...),
		Reasoning:      config.EffectiveLLMProfileReasoning(normalized),
		Thinking:       normalized.Thinking,
		TimeoutSeconds: normalized.TimeoutSeconds,
		MaxTokens:      normalized.MaxTokens,
		Temperature:    normalized.Temperature,
		PromptCache:    normalized.PromptCache,
		ProviderState:  normalized.ProviderState,
		Extra:          cloneStringMap(normalized.Extra),
	}, nil
}

func requiredLLMProfilesForPipeline(pipeline *models.Pipeline, defaultProfile string) (string, map[string]bool) {
	required := map[string]bool{}
	defaultProfile = config.NormalizeLLMProfileName(defaultProfile)
	if defaultProfile == "" {
		defaultProfile = config.DefaultLLMProfileName
	}
	if pipeline == nil {
		required[defaultProfile] = true
		return defaultProfile, required
	}

	pipelineProfile := config.NormalizeLLMProfileName(pipeline.LLMProfile)
	if pipelineProfile == "" {
		pipelineProfile = defaultProfile
	}
	runtimeDefault := pipelineProfile
	required[runtimeDefault] = true

	if len(pipeline.Output.Items) > 0 {
		outputProfile := firstNonEmpty(pipeline.Output.LLMProfile, pipelineProfile)
		outputProfile = config.NormalizeLLMProfileName(outputProfile)
		for _, item := range pipeline.Output.Items {
			itemProfile := firstNonEmpty(item.LLMProfile, outputProfile)
			itemProfile = config.NormalizeLLMProfileName(itemProfile)
			if itemProfile != "" {
				required[itemProfile] = true
			}
		}
	}

	for _, step := range pipeline.Steps {
		stepProfile := firstNonEmpty(step.GetLLMProfile(), pipelineProfile)
		stepProfile = config.NormalizeLLMProfileName(stepProfile)
		if strings.TrimSpace(step.GetCondition()) != "" || strings.TrimSpace(step.GetGoal()) != "" {
			if stepProfile != "" {
				required[stepProfile] = true
			}
		}
		for _, task := range step.GetTasks() {
			if strings.TrimSpace(task.Goal) == "" {
				continue
			}
			taskProfile := firstNonEmpty(task.LLMProfile, stepProfile)
			taskProfile = config.NormalizeLLMProfileName(taskProfile)
			if taskProfile != "" {
				required[taskProfile] = true
			}
		}
	}
	return runtimeDefault, required
}

func (a *App) handleListLLMProfiles(w http.ResponseWriter, r *http.Request) {
	cfg := a.getConfigSnapshot()
	defaultProfile := cfg.EffectiveLLMDefaultProfile()
	profiles := cfg.EffectiveLLMProfiles()
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))

	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	views := make([]llmProfileView, 0, len(names))
	visibleNames := make([]string, 0, len(names))
	for _, name := range names {
		visible, err := a.aiResourceVisible(r, llmProfileAccessSpec, name)
		if err != nil {
			http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
			return
		}
		if !visible {
			continue
		}
		profile := config.NormalizeLLMProfile(profiles[name])
		status, message := a.validateLLMProfileConfiguration(r.Context(), name, profile)
		refs, _ := a.findLLMProfileReferences(name)
		allowed := config.LLMProfileAllowedInScope(profile, scope)
		disabledReason := ""
		if scope != "" && !allowed {
			disabledReason = fmt.Sprintf("LLM profile %q is not allowed in scope %q", name, scope)
		}
		views = append(views, llmProfileView{
			llmProfileForm: profileFormFromConfig(name, profile),
			Status:         status,
			Validation:     message,
			References:     refs,
			AllowedInScope: allowed,
			DisabledReason: disabledReason,
		})
		visibleNames = append(visibleNames, name)
	}
	defaultProfile = aiResourceVisibleDefault(defaultProfile, visibleNames)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(llmProfilesResponse{DefaultProfile: defaultProfile, Profiles: views})
}

func (a *App) handleReplaceLLMProfiles(w http.ResponseWriter, r *http.Request) {
	if !a.requireAAADecision(w, r, "system.update", model.ResourceRef{Type: "system", ID: "models"}) {
		return
	}
	var payload llmProfilesRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid LLM profile payload", http.StatusBadRequest)
		return
	}

	defaultProfile := strings.TrimSpace(payload.DefaultProfile)
	if defaultProfile == "" {
		defaultProfile = strings.TrimSpace(payload.LLMDefaultProfile)
	}
	if defaultProfile == "" {
		defaultProfile = config.DefaultLLMProfileName
	}

	cfg := a.getConfigSnapshot()
	existing := cfg.EffectiveLLMProfiles()
	profiles := map[string]config.LLMProfile{}
	for name, profile := range payload.LLMProfiles {
		profileName := config.NormalizeLLMProfileName(name)
		if profileName == "" {
			continue
		}
		profiles[profileName] = config.NormalizeLLMProfile(profile)
	}
	for _, form := range payload.Profiles {
		profileName := config.NormalizeLLMProfileName(form.Name)
		if profileName == "" {
			http.Error(w, "profile name is required", http.StatusBadRequest)
			return
		}
		form.Pricing = llmProfilePricingForSave(form, existing, profileName)
		profiles[profileName] = profileConfigFromForm(form)
	}
	if len(profiles) == 0 {
		http.Error(w, "at least one LLM profile is required", http.StatusBadRequest)
		return
	}
	if _, ok := profiles[defaultProfile]; !ok {
		http.Error(w, fmt.Sprintf("default LLM profile %q is not configured", defaultProfile), http.StatusBadRequest)
		return
	}

	cfg = configWithLLMProfiles(cfg, defaultProfile, profiles)
	if err := a.persistLLMProfilesConfig(r.Context(), cfg); err != nil {
		http.Error(w, "failed to persist LLM profiles", http.StatusInternalServerError)
		return
	}
	a.setLLMProfiles(defaultProfile, profiles)
	a.handleListLLMProfiles(w, r)
}

func (a *App) handleSetDefaultLLMProfile(w http.ResponseWriter, r *http.Request) {
	if !a.requireAAADecision(w, r, "system.update", model.ResourceRef{Type: "system", ID: "models"}) {
		return
	}
	var payload struct {
		DefaultProfile string `json:"default_profile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid default LLM profile payload", http.StatusBadRequest)
		return
	}
	defaultProfile := config.NormalizeLLMProfileName(payload.DefaultProfile)
	if defaultProfile == "" {
		http.Error(w, "default LLM profile is required", http.StatusBadRequest)
		return
	}

	cfg := a.getConfigSnapshot()
	profiles := cfg.EffectiveLLMProfiles()
	if _, ok := profiles[defaultProfile]; !ok {
		http.Error(w, fmt.Sprintf("default LLM profile %q is not configured", defaultProfile), http.StatusBadRequest)
		return
	}

	cfg = configWithLLMProfiles(cfg, defaultProfile, profiles)
	if err := a.persistLLMProfilesConfig(r.Context(), cfg); err != nil {
		http.Error(w, "failed to persist default LLM profile", http.StatusInternalServerError)
		return
	}
	a.setLLMProfiles(defaultProfile, profiles)
	a.handleListLLMProfiles(w, r)
}

func (a *App) handleUpsertLLMProfile(w http.ResponseWriter, r *http.Request) {
	profileName := config.NormalizeLLMProfileName(r.PathValue("profileName"))
	if profileName == "" {
		http.Error(w, "profile name is required", http.StatusBadRequest)
		return
	}

	var payload llmProfileForm
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid LLM profile payload", http.StatusBadRequest)
		return
	}
	if payload.Name != "" && !strings.EqualFold(strings.TrimSpace(payload.Name), profileName) {
		http.Error(w, "profile name in path and payload must match", http.StatusBadRequest)
		return
	}
	if !a.requireAIResourceWrite(w, r, llmProfileAccessSpec, profileName) {
		return
	}

	cfg := a.getConfigSnapshot()
	defaultProfile := cfg.EffectiveLLMDefaultProfile()
	profiles := cfg.EffectiveLLMProfiles()
	if profiles == nil {
		profiles = map[string]config.LLMProfile{}
	}
	payload.Pricing = llmProfilePricingForSave(payload, profiles, profileName)
	profiles[profileName] = profileConfigFromForm(payload)
	if _, ok := profiles[defaultProfile]; !ok {
		defaultProfile = profileName
	}

	cfg = configWithLLMProfiles(cfg, defaultProfile, profiles)
	if err := a.persistLLMProfilesConfig(r.Context(), cfg); err != nil {
		http.Error(w, "failed to persist LLM profile", http.StatusInternalServerError)
		return
	}
	a.setLLMProfiles(defaultProfile, profiles)
	a.handleListLLMProfiles(w, r)
}

func (a *App) handleDeleteLLMProfile(w http.ResponseWriter, r *http.Request) {
	profileName := config.NormalizeLLMProfileName(r.PathValue("profileName"))
	if profileName == "" {
		http.Error(w, "profile name is required", http.StatusBadRequest)
		return
	}

	cfg := a.getConfigSnapshot()
	defaultProfile := cfg.EffectiveLLMDefaultProfile()
	if strings.EqualFold(profileName, defaultProfile) {
		http.Error(w, "default LLM profile cannot be deleted while active", http.StatusBadRequest)
		return
	}

	profiles := cfg.EffectiveLLMProfiles()
	if _, ok := profiles[profileName]; !ok {
		http.Error(w, "LLM profile not found", http.StatusNotFound)
		return
	}
	if !a.requireAIResourceWrite(w, r, llmProfileAccessSpec, profileName) {
		return
	}

	refs, err := a.findLLMProfileReferences(profileName)
	if err != nil {
		http.Error(w, "failed to inspect LLM profile references", http.StatusInternalServerError)
		return
	}
	force := strings.EqualFold(r.URL.Query().Get("force"), "true")
	migrateTo := config.NormalizeLLMProfileName(r.URL.Query().Get("migrate_to"))
	if len(refs) > 0 {
		if !force {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":      "LLM profile is still referenced",
				"references": refs,
			})
			return
		}
		if migrateTo == "" {
			http.Error(w, "migrate_to is required when forcing deletion of a used LLM profile", http.StatusBadRequest)
			return
		}
		if strings.EqualFold(migrateTo, profileName) {
			http.Error(w, "migrate_to must be a different LLM profile", http.StatusBadRequest)
			return
		}
		if _, ok := profiles[migrateTo]; !ok {
			http.Error(w, fmt.Sprintf("migration target LLM profile %q is not configured", migrateTo), http.StatusBadRequest)
			return
		}
		if err := a.migrateLLMProfileReferences(profileName, migrateTo); err != nil {
			http.Error(w, "failed to migrate LLM profile references", http.StatusInternalServerError)
			return
		}
	}

	delete(profiles, profileName)
	cfg = configWithLLMProfiles(cfg, defaultProfile, profiles)
	if err := a.persistLLMProfilesConfig(r.Context(), cfg); err != nil {
		http.Error(w, "failed to persist LLM profile deletion", http.StatusInternalServerError)
		return
	}
	a.setLLMProfiles(defaultProfile, profiles)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleTestLLMProfile(w http.ResponseWriter, r *http.Request) {
	profileName := config.NormalizeLLMProfileName(r.PathValue("profileName"))
	if profileName == "" {
		http.Error(w, "profile name is required", http.StatusBadRequest)
		return
	}
	cfg := a.getConfigSnapshot()
	profiles := cfg.EffectiveLLMProfiles()
	profile, ok := profiles[profileName]
	if !ok {
		http.Error(w, "LLM profile not found", http.StatusNotFound)
		return
	}
	if !a.requireAIResourceUse(w, r, llmProfileAccessSpec, profileName) {
		return
	}
	if status, message := a.validateLLMProfileConfiguration(r.Context(), profileName, profile); status != "valid" {
		http.Error(w, message, http.StatusBadRequest)
		return
	}

	timeout := 20 * time.Second
	if profile.TimeoutSeconds > 0 {
		timeout = time.Duration(profile.TimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	apiKey, err := a.resolveLLMProfileAPIKey(ctx, profileName, profile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	reply, err := testLLMProfile(ctx, profile, apiKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"reply":  reply,
	})
}

func testLLMProfile(ctx context.Context, profile config.LLMProfile, apiKey string) (string, error) {
	profile = config.NormalizeLLMProfile(profile)
	switch profile.Provider {
	case config.LLMProviderGemini:
		return testGeminiProfile(ctx, profile, apiKey)
	case config.LLMProviderLMStudio:
		return testLMStudioProfile(ctx, profile, apiKey)
	case config.LLMProviderOpenAI,
		config.LLMProviderGroq,
		config.LLMProviderMistral,
		config.LLMProviderOllama,
		config.LLMProviderOpenRouter:
		return testOpenAICompatibleProfile(ctx, profile, apiKey)
	case config.LLMProviderAnthropic:
		return testAnthropicProfile(ctx, profile, apiKey)
	case config.LLMProviderAzureOpenAI:
		return testAzureOpenAIProfile(ctx, profile, apiKey)
	default:
		return "", fmt.Errorf("unsupported LLM provider: %s", profile.Provider)
	}
}

func testGeminiProfile(ctx context.Context, profile config.LLMProfile, apiKey string) (string, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", profile.Model, apiKey)
	payload := models.GeminiRequest{
		Contents: []models.Content{{Parts: []models.Part{{Text: "reply ok"}}}},
	}
	if profile.MaxTokens > 0 || profile.Temperature != nil {
		payload.GenerationConfig = &models.GeminiGenerationConfig{
			MaxOutputTokens: profile.MaxTokens,
			Temperature:     profile.Temperature,
		}
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini api returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var geminiResp models.GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", err
	}
	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned an empty response")
	}
	return strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text), nil
}

func testLMStudioProfile(ctx context.Context, profile config.LLMProfile, apiKey string) (string, error) {
	payload := struct {
		Model           string   `json:"model,omitempty"`
		Input           string   `json:"input"`
		Reasoning       string   `json:"reasoning,omitempty"`
		MaxOutputTokens int      `json:"max_output_tokens,omitempty"`
		Temperature     *float64 `json:"temperature,omitempty"`
		Store           bool     `json:"store"`
	}{
		Model:           profile.Model,
		Input:           "reply ok",
		Reasoning:       config.LMStudioReasoningRequestValue(config.EffectiveLLMProfileReasoning(profile)),
		MaxOutputTokens: profile.MaxTokens,
		Temperature:     profile.Temperature,
		Store:           false,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, buildNopsaiLMStudioChatURL(profile.BaseURL), bytes.NewReader(payloadBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lm studio api returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var lmStudioResp struct {
		Output []struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &lmStudioResp); err != nil {
		return "", err
	}
	for _, item := range lmStudioResp.Output {
		if item.Type == "message" && strings.TrimSpace(item.Content) != "" {
			return strings.TrimSpace(item.Content), nil
		}
	}
	return "", fmt.Errorf("lm studio returned an empty response")
}

func buildNopsaiLMStudioChatURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasSuffix(lower, "/api/v1/chat"):
		return trimmed
	case strings.HasSuffix(lower, "/api/v1"):
		return trimmed + "/chat"
	default:
		return trimmed + "/api/v1/chat"
	}
}

func writeLLMProfileStoreError(w http.ResponseWriter, err error) {
	switch err {
	case nil:
		return
	case pgx.ErrNoRows:
		http.Error(w, "LLM profile not found", http.StatusNotFound)
	default:
		http.Error(w, "LLM profile request failed", http.StatusInternalServerError)
	}
}

// validateLLMProfilePricing checks a rate card if one is present.
//
// Pricing is optional. Local models have no per-token price, and many providers
// publish none that nopsai can know, so requiring a rate card would block
// configuration that is otherwise correct. A model without one still runs; its
// usage records with no cost and is reported as unpriced rather than as free.
//
// Only nonsensical values are rejected, since a negative rate cannot describe
// any real price and would quietly subtract from a spend total.
func validateLLMProfilePricing(name string, pricing *config.LLMPricing) (string, string) {
	if pricing == nil {
		return "valid", ""
	}
	if pricing.InputPerMillionUSD < 0 {
		return "invalid", fmt.Sprintf("LLM profile %q has negative pricing.input_per_million_usd %g", name, pricing.InputPerMillionUSD)
	}
	if pricing.OutputPerMillionUSD < 0 {
		return "invalid", fmt.Sprintf("LLM profile %q has negative pricing.output_per_million_usd %g", name, pricing.OutputPerMillionUSD)
	}
	if pricing.CachedInputPerMillionUSD != nil && *pricing.CachedInputPerMillionUSD < 0 {
		return "invalid", fmt.Sprintf("LLM profile %q has negative pricing.cached_input_per_million_usd %g", name, *pricing.CachedInputPerMillionUSD)
	}
	if pricing.CacheWritePerMillionUSD != nil && *pricing.CacheWritePerMillionUSD < 0 {
		return "invalid", fmt.Sprintf("LLM profile %q has negative pricing.cache_write_per_million_usd %g", name, *pricing.CacheWritePerMillionUSD)
	}
	return "valid", ""
}

package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/credentials"
	"nopsai/services/nopsai/pkg/auth"
)

func (a *App) seedStarterDatabase(ctx context.Context, req setupBootstrapRequest) (map[string]int, error) {
	details := map[string]int{
		"run_teams_created": 0,
		"run_teams_updated": 0,
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := a.syncPipelineRunTeams(ctx, tx, setupPipelineRunStructure(req.Profile, req.RepositoryTeams, req.Repositories), details); err != nil {
		return nil, err
	}

	stepDefinition := setupReusableStepYAML()
	var step models.PipelineStep
	if err := yaml.Unmarshal([]byte(stepDefinition), &step); err != nil {
		return nil, fmt.Errorf("starter step is invalid: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO steps (path, name, definition, source, updated_at)
		VALUES ($1, $2, $3, 'setup', NOW())
		ON CONFLICT (path, name) DO UPDATE SET definition = EXCLUDED.definition, source = 'setup', updated_at = NOW()
	`, "setup", "announce", stepDefinition); err != nil {
		return nil, fmt.Errorf("seed starter step: %w", err)
	}
	details["steps_seeded"]++

	pipelineDefinition := setupFirstRunPipelineYAML(req.Profile)
	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(pipelineDefinition), &pipeline); err != nil {
		return nil, fmt.Errorf("starter pipeline is invalid: %w", err)
	}
	if err := validatePipeline(&pipeline); err != nil {
		return nil, fmt.Errorf("starter pipeline validation failed: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO pipelines (path, name, version, definition, source, updated_at)
		VALUES ($1, $2, $3, $4, 'setup', NOW())
		ON CONFLICT (path, name) DO UPDATE SET version = EXCLUDED.version, definition = EXCLUDED.definition, source = 'setup', updated_at = NOW()
	`, "setup", pipeline.Name, normalizePipelineVersion(pipeline.Version), pipelineDefinition); err != nil {
		return nil, fmt.Errorf("seed starter pipeline: %w", err)
	}
	details["pipelines_seeded"]++

	for _, repo := range req.Repositories {
		definition := setupTriggerYAML(req.Profile)
		var manifest models.Manifest
		if err := yaml.Unmarshal([]byte(definition), &manifest); err != nil {
			return nil, fmt.Errorf("starter trigger for %s is invalid: %w", repo, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO triggers (repository_name, trigger_definition, source, visibility)
			VALUES ($1, $2, 'setup', 'team')
			ON CONFLICT (repository_name) DO UPDATE SET trigger_definition = EXCLUDED.trigger_definition, source = 'setup', visibility = EXCLUDED.visibility
		`, repo, definition); err != nil {
			return nil, fmt.Errorf("seed trigger for %s: %w", repo, err)
		}
		details["triggers_seeded"]++
	}

	for scope, values := range setupScopeVariables(req.Profile, req.RepositoryTeams, req.Repositories) {
		for name, value := range values {
			if _, err := tx.Exec(ctx, `
				INSERT INTO variables (name, value, repository_name, scope, source, updated_at)
				VALUES ($1, $2, NULL, $3, 'setup', NOW())
				ON CONFLICT (name, repository_name, scope) DO UPDATE SET value = EXCLUDED.value, source = 'setup', updated_at = NOW()
			`, name, value, runtimeScopeForStorage(scope)); err != nil {
				return nil, fmt.Errorf("seed variable %s: %w", name, err)
			}
			details["variables_seeded"]++
		}
	}

	knowledge := setupKnowledgeContexts(req.Profile, req.RepositoryTeams, req.Repositories)
	for _, item := range knowledge {
		if _, err := tx.Exec(ctx, `
			INSERT INTO knowledge_contexts (kind, team_path, name, description, content, source, updated_at)
			VALUES ($1, $2, $3, $4, $5, 'setup', NOW())
			ON CONFLICT (kind, team_path, name) DO UPDATE SET
				description = EXCLUDED.description,
				content = EXCLUDED.content,
				source = 'setup',
				updated_at = NOW()
		`, item.kind, item.team, item.name, item.description, item.content); err != nil {
			return nil, fmt.Errorf("seed knowledge context %s/%s/%s: %w", item.kind, item.team, item.name, err)
		}
		details["knowledge_contexts_seeded"]++
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return details, nil
}

func (a *App) seedSetupLLMProfile(ctx context.Context, input setupLLMProfileInput) (int, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = config.DefaultLLMProfileName
	}
	provider := strings.TrimSpace(input.Provider)
	if provider == "" {
		provider = config.LLMProviderLMStudio
	}
	provider = config.NormalizeLLMProvider(provider)
	modelName := strings.TrimSpace(input.Model)
	if modelName == "" {
		modelName = config.DefaultLLMProviderModel(provider)
	}
	baseURL := strings.TrimSpace(input.BaseURL)
	if baseURL == "" {
		baseURL = config.DefaultLLMProviderBaseURL(provider)
	}
	credentialRef := strings.TrimSpace(input.CredentialRef)
	if credentialRef == "" && config.LLMProviderRequiresAPIKey(provider) {
		credentialRef = "credential://system/llm/" + credentialReferenceSegment(name)
	}
	allowedScopes := models.NormalizeScopeList(input.AllowedScopes)
	if len(allowedScopes) == 0 {
		allowedScopes = []string{"dev", "prod"}
	}

	cfg := a.getConfigSnapshot()
	profiles := cfg.EffectiveLLMProfiles()
	if profiles == nil {
		profiles = map[string]config.LLMProfile{}
	}
	_, existed := profiles[name]
	profiles[name] = config.NormalizeLLMProfile(config.LLMProfile{
		Provider:       provider,
		Model:          modelName,
		BaseURL:        baseURL,
		CredentialRef:  credentialRef,
		AllowedScopes:  allowedScopes,
		TimeoutSeconds: input.TimeoutSeconds,
		MaxTokens:      input.MaxTokens,
		Temperature:    input.Temperature,
		Extra:          input.Extra,
	})
	cfg.LLMDefaultProfile = name
	cfg.LLMProfiles = profiles
	a.cfgMu.Lock()
	a.cfg.LLMDefaultProfile = cfg.LLMDefaultProfile
	a.cfg.LLMProfiles = cfg.LLMProfiles
	a.cfgMu.Unlock()

	if err := a.persistLLMProfilesConfig(ctx, cfg); err != nil {
		return 0, err
	}
	if credentialRef != "" {
		ref, err := credentials.ParseReference(credentialRef)
		if err != nil {
			return 0, err
		}
		credential, err := a.credentials.EnsureMetadata(ctx, createCredentialInput{
			Reference:   ref,
			Kind:        "api_key",
			Description: "LLM API key for " + name,
			Actor:       credentialActorFromContext(ctx),
		})
		if err != nil {
			return 0, err
		}
		if value := strings.TrimSpace(input.APIKeyValue); value != "" {
			if _, err := a.credentials.PutValue(ctx, credential.ID, []byte(value), credentialActorFromContext(ctx)); err != nil {
				return 0, err
			}
		}
	}
	if existed {
		return 0, nil
	}
	return 1, nil
}

func (a *App) seedSetupMCPExamples(ctx context.Context) (int, error) {
	cfg := a.getConfigSnapshot()
	servers := cfg.EffectiveMCPServers()
	if servers == nil {
		servers = map[string]models.MCPServer{}
	}
	profiles := cfg.EffectiveMCPProfiles()
	if profiles == nil {
		profiles = map[string]models.MCPProfile{}
	}
	count := 0
	if _, exists := servers["github-readonly"]; !exists {
		servers["github-readonly"] = models.MCPServer{
			Name:          "github-readonly",
			DisplayName:   "GitHub MCP Read-only",
			Enabled:       false,
			Provider:      "github",
			Transport:     models.MCPTransportStreamableHTTP,
			URL:           "https://api.githubcopilot.com/mcp/x/all/readonly",
			AuthType:      models.MCPAuthBearerToken,
			CredentialRef: "credential://system/mcp/github-readonly",
			Timeout:       models.DefaultMCPTimeout,
			AllowedScopes: []string{"dev", "prod"},
		}
		count++
	}
	if _, exists := profiles["github-readonly"]; !exists {
		profiles["github-readonly"] = models.MCPProfile{
			Name:        "github-readonly",
			Description: "Read-only GitHub tools for setup smoke tests.",
			Enabled:     false,
			ServerRefs: []models.MCPProfileServerRef{{
				ServerName: "github-readonly",
				Tools:      []string{"*"},
			}},
			AllowedScopes: []string{"dev", "prod"},
		}
		count++
	}
	cfg.MCPServers = models.NormalizeMCPServers(servers)
	cfg.MCPProfiles = models.NormalizeMCPProfiles(profiles)
	a.cfgMu.Lock()
	a.cfg.MCPServers = cfg.MCPServers
	a.cfg.MCPProfiles = cfg.MCPProfiles
	a.cfgMu.Unlock()
	if err := a.persistMCPRegistryConfig(ctx, cfg); err != nil {
		return 0, err
	}
	return count, nil
}

func (a *App) seedSetupUsers(ctx context.Context, users []setupUserInput, profile string, repositoryTeams []setupRepositoryTeamInput, actor string) ([]setupTemporaryCredential, error) {
	grantTeams := setupAccessGrantTeams(profile, repositoryTeams)
	if err := a.ensureSetupRootTeams(ctx, grantTeams); err != nil {
		return nil, err
	}
	created := []setupTemporaryCredential{}
	for _, input := range users {
		role, err := normalizeProductRoleName(firstNonEmptyString(input.Role, productRoleViewer))
		if err != nil {
			return nil, err
		}
		sub := strings.TrimSpace(input.Sub)
		email := strings.TrimSpace(input.Email)
		if sub == "" {
			sub = email
		}
		if sub == "" {
			continue
		}

		var userID string
		err = a.db.QueryRow(ctx, `
			SELECT id::text FROM users WHERE sub = $1 OR ($2 <> '' AND LOWER(email) = LOWER($2)) LIMIT 1
		`, sub, email).Scan(&userID)
		temporaryPassword := ""
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			temporaryPassword = strings.TrimSpace(input.Password)
			if temporaryPassword == "" {
				var genErr error
				temporaryPassword, genErr = randomSecret(18)
				if genErr != nil {
					return nil, genErr
				}
			}
			hashed, hashErr := auth.HashPassword(temporaryPassword)
			if hashErr != nil {
				return nil, hashErr
			}
			newID := uuid.NewString()
			if _, err := a.db.Exec(ctx, `
				INSERT INTO users (id, sub, email, provider, password_hash, status, must_change_password)
				VALUES ($1, $2, NULLIF($3, ''), 'local', $4, 'active', TRUE)
			`, newID, sub, email, hashed); err != nil {
				return nil, err
			}
			userID = newID
			created = append(created, setupTemporaryCredential{Sub: sub, Email: email, TemporaryPassword: temporaryPassword, Role: role})
		} else if err != nil {
			return nil, err
		}

		resourceType := grantResourceTeam
		resourceID := setupAccessUserTeam(grantTeams, input.Team)
		if role == productRoleAdmin {
			resourceType = grantResourcePlatform
			resourceID = platformGrantID
		} else if resourceID == "" {
			return nil, fmt.Errorf("setup user %s requires a selected team for role %s", sub, role)
		}
		_, err = a.GrantProductRole(ctx, GrantProductRoleInput{
			SubjectType:  grantSubjectUser,
			SubjectID:    userID,
			RoleName:     role,
			ResourceType: resourceType,
			ResourceID:   resourceID,
			Inherit:      true,
			GrantedBy:    actor,
		})
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "grant already exists") {
			return nil, err
		}
	}
	return created, nil
}

func (a *App) ensureSetupRootTeams(ctx context.Context, names []string) error {
	for _, name := range names {
		name = strings.Trim(strings.TrimSpace(name), "/")
		if name == "" {
			continue
		}
		if _, err := a.db.Exec(ctx, `
			INSERT INTO teams (name, kind, description)
			VALUES ($1, 'team', $2)
			ON CONFLICT (name) DO NOTHING
		`, name, "Starter setup team"); err != nil {
			return err
		}
	}
	return nil
}

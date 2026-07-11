package nopsai

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/internal/mcpregistry"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"
)

const configRepositoryTeamAIProfilesPath = "ai-profiles.yaml"

var errTeamAIProfilesGitOpsNotFound = errors.New("team AI profile GitOps file not found")

type teamAIProfilesGitOpsFile struct {
	LLMDefaultProfile   *string             `json:"llm_default_profile,omitempty" yaml:"llm_default_profile,omitempty"`
	LLMProfiles         []llmProfileForm    `json:"llm_profiles,omitempty" yaml:"llm_profiles,omitempty"`
	AgentDefaultProfile *string             `json:"agent_default_profile,omitempty" yaml:"agent_default_profile,omitempty"`
	AgentProfiles       []agentProfileForm  `json:"agent_profiles,omitempty" yaml:"agent_profiles,omitempty"`
	MCPProfiles         []models.MCPProfile `json:"mcp_profiles,omitempty" yaml:"mcp_profiles,omitempty"`
}

type gitOpsTeamAIProfilePlan struct {
	teamPath            string
	sourcePath          string
	llmDefaultProfile   *string
	llmProfiles         map[string]config.LLMProfile
	agentDefaultProfile *string
	agentProfiles       map[string]models.AgentProfile
	mcpProfiles         map[string]models.MCPProfile
}

type gitOpsTeamAIProfileFileCandidate struct {
	sourcePath string
	content    string
}

func parseGitOpsTeamAIProfilePlan(binding models.ConfigRepository, repoCtx configSyncRepositoryContext, files map[string]string) (*gitOpsTeamAIProfilePlan, error) {
	candidates := []gitOpsTeamAIProfileFileCandidate{}
	for path, content := range files {
		normalized := filepath.ToSlash(path)
		rel, ok := configRepositoryRelativeGitPath(repoCtx.basePath, normalized)
		if !ok || !isGitOpsTeamAIProfilesPath(rel) {
			continue
		}
		candidates = append(candidates, gitOpsTeamAIProfileFileCandidate{
			sourcePath: normalized,
			content:    content,
		})
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	if binding.ScopeType != models.ConfigRepositoryScopeTeam {
		return nil, fmt.Errorf("team AI profiles can only be configured from a team config repository")
	}
	if len(candidates) > 1 {
		paths := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			paths = append(paths, candidate.sourcePath)
		}
		sort.Strings(paths)
		return nil, fmt.Errorf("multiple team AI profile GitOps files found: %s", strings.Join(paths, ", "))
	}
	teamPath := strings.Trim(strings.TrimSpace(repoCtx.boundTeam), "/")
	if teamPath == "" {
		teamPath = strings.Trim(strings.TrimSpace(binding.ScopeID), "/")
	}
	return parseGitOpsTeamAIProfileFile(candidates[0].content, candidates[0].sourcePath, teamPath)
}

func parseGitOpsTeamAIProfileFile(content, sourcePath, teamPath string) (*gitOpsTeamAIProfilePlan, error) {
	var file teamAIProfilesGitOpsFile
	if err := yaml.Unmarshal([]byte(content), &file); err != nil {
		return nil, fmt.Errorf("failed to parse team AI profile GitOps file '%s': %w", sourcePath, err)
	}
	teamPath = strings.Trim(strings.TrimSpace(teamPath), "/")
	if teamPath == "" {
		return nil, fmt.Errorf("team AI profile GitOps file '%s' is missing a team scope", sourcePath)
	}

	llmProfiles := map[string]config.LLMProfile{}
	for _, form := range file.LLMProfiles {
		name := config.NormalizeLLMProfileName(form.Name)
		if name == "" {
			return nil, fmt.Errorf("team AI profile GitOps file '%s' contains an LLM profile without a name", sourcePath)
		}
		if _, exists := llmProfiles[name]; exists {
			return nil, fmt.Errorf("team AI profile GitOps file '%s' defines LLM profile %q more than once", sourcePath, name)
		}
		profile := profileConfigFromForm(form)
		if status, message := validateLLMProfileDefinition(name, profile); status != "valid" {
			return nil, fmt.Errorf("invalid LLM profile in team AI profile GitOps file '%s': %s", sourcePath, message)
		}
		for _, scope := range profile.AllowedScopes {
			if strings.TrimSpace(scope) == "" {
				continue
			}
			if _, err := configsync.CleanPathSegments(scope, false); err != nil {
				return nil, fmt.Errorf("invalid allowed scope %q for LLM profile %q in team AI profile GitOps file '%s': %w", scope, name, sourcePath, err)
			}
		}
		llmProfiles[name] = profile
	}

	agentProfiles := map[string]models.AgentProfile{}
	for _, form := range file.AgentProfiles {
		profile := agentProfileFromForm(form, "gitops")
		profile.ID = models.NormalizeAgentProfileID(profile.ID)
		if profile.ID == "" {
			return nil, fmt.Errorf("team AI profile GitOps file '%s' contains an agent profile without an id", sourcePath)
		}
		if _, exists := agentProfiles[profile.ID]; exists {
			return nil, fmt.Errorf("team AI profile GitOps file '%s' defines agent profile %q more than once", sourcePath, profile.ID)
		}
		if err := validateAgentProfileDefinition(profile); err != nil {
			return nil, fmt.Errorf("invalid agent profile in team AI profile GitOps file '%s': %w", sourcePath, err)
		}
		agentProfiles[profile.ID] = profile
	}

	mcpProfiles := map[string]models.MCPProfile{}
	for _, profile := range file.MCPProfiles {
		name := models.NormalizeMCPProfileName(profile.Name)
		if name == "" {
			return nil, fmt.Errorf("team AI profile GitOps file '%s' contains an MCP profile without a name", sourcePath)
		}
		if _, exists := mcpProfiles[name]; exists {
			return nil, fmt.Errorf("team AI profile GitOps file '%s' defines MCP profile %q more than once", sourcePath, name)
		}
		profile.Name = name
		mcpProfiles[name] = models.NormalizeMCPProfile(profile)
	}

	llmDefault := normalizeOptionalLLMDefault(file.LLMDefaultProfile)
	if llmDefault != nil && *llmDefault != "" {
		if _, ok := llmProfiles[*llmDefault]; !ok {
			return nil, fmt.Errorf("team AI profile GitOps file '%s' sets LLM default profile %q but does not define it", sourcePath, *llmDefault)
		}
	}
	agentDefault := normalizeOptionalAgentDefault(file.AgentDefaultProfile)
	if agentDefault != nil && *agentDefault != "" {
		if profile, ok := agentProfiles[*agentDefault]; ok && !profile.Enabled {
			return nil, fmt.Errorf("team AI profile GitOps file '%s' sets disabled agent profile %q as default", sourcePath, *agentDefault)
		}
	}
	if len(llmProfiles) == 0 && len(agentProfiles) == 0 && len(mcpProfiles) == 0 && llmDefault == nil && agentDefault == nil {
		return nil, fmt.Errorf("team AI profile GitOps file '%s' must define at least one profile or default", sourcePath)
	}
	return &gitOpsTeamAIProfilePlan{
		teamPath:            teamPath,
		sourcePath:          sourcePath,
		llmDefaultProfile:   llmDefault,
		llmProfiles:         llmProfiles,
		agentDefaultProfile: agentDefault,
		agentProfiles:       agentProfiles,
		mcpProfiles:         mcpProfiles,
	}, nil
}

func normalizeOptionalLLMDefault(raw *string) *string {
	if raw == nil {
		return nil
	}
	value := config.NormalizeLLMProfileName(*raw)
	return &value
}

func normalizeOptionalAgentDefault(raw *string) *string {
	if raw == nil {
		return nil
	}
	value := normalizeAgentProfileDefault(*raw)
	return &value
}

func isGitOpsTeamAIProfilesPath(path string) bool {
	switch strings.Trim(filepath.ToSlash(path), "/") {
	case "ai-profiles.yaml", "ai-profiles.yml":
		return true
	default:
		return false
	}
}

func (a *App) persistTeamAIProfilesToTx(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, plan *gitOpsTeamAIProfilePlan, commitSHA string) error {
	if plan == nil {
		return nil
	}
	teamID, err := resolveTeamAIProfileTeamID(ctx, tx, plan.teamPath)
	if err != nil {
		return err
	}
	if err := a.persistGitOpsTeamLLMProfilesToTx(ctx, tx, teamID, binding.ID, plan, commitSHA); err != nil {
		return err
	}
	if err := persistGitOpsTeamAgentProfilesToTx(ctx, tx, teamID, binding.ID, plan, commitSHA); err != nil {
		return err
	}
	if err := a.persistGitOpsTeamMCPProfilesToTx(ctx, tx, teamID, binding.ID, plan, commitSHA); err != nil {
		return err
	}
	return nil
}

func resolveTeamAIProfileTeamID(ctx context.Context, tx pgx.Tx, teamPath string) (int, error) {
	teamPath = strings.Trim(strings.TrimSpace(teamPath), "/")
	if teamPath == "" {
		return 0, fmt.Errorf("team AI profile scope is required")
	}
	var (
		teamID int
		kind   string
	)
	if err := tx.QueryRow(ctx, `
		SELECT id, COALESCE(kind, 'team') FROM teams WHERE name = $1
	`, teamPath).Scan(&teamID, &kind); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("team AI profiles reference missing team '%s'", teamPath)
		}
		return 0, fmt.Errorf("failed to resolve team AI profile owner '%s': %w", teamPath, err)
	}
	if strings.EqualFold(kind, "app") {
		return 0, fmt.Errorf("team AI profiles must be attached to a team, not application '%s'", teamPath)
	}
	return teamID, nil
}

func (a *App) persistGitOpsTeamLLMProfilesToTx(ctx context.Context, tx pgx.Tx, teamID int, configRepoID int64, plan *gitOpsTeamAIProfilePlan, commitSHA string) error {
	names := make([]string, 0, len(plan.llmProfiles))
	for name := range plan.llmProfiles {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		if _, err := tx.Exec(ctx, `
			DELETE FROM team_llm_profiles
			WHERE team_id = $1 AND managed_by_config_repo = TRUE AND config_repo_id = $2
		`, teamID, configRepoID); err != nil {
			return fmt.Errorf("clear GitOps team LLM profiles: %w", err)
		}
	} else if _, err := tx.Exec(ctx, `
		DELETE FROM team_llm_profiles
		WHERE team_id = $1 AND managed_by_config_repo = TRUE AND config_repo_id = $2 AND name != ALL($3::text[])
	`, teamID, configRepoID, names); err != nil {
		return fmt.Errorf("prune GitOps team LLM profiles: %w", err)
	}
	for _, name := range names {
		if err := upsertTeamLLMProfileTxWithSource(ctx, tx, teamID, name, plan.llmProfiles[name], "gitops", plan.sourcePath, commitSHA, configRepoID, true); err != nil {
			return err
		}
	}
	if plan.llmDefaultProfile != nil {
		if err := persistTeamProfileSettingTx(ctx, tx, teamID, teamLLMDefaultProfileSetting, *plan.llmDefaultProfile); err != nil {
			return err
		}
	}
	return nil
}

func persistGitOpsTeamAgentProfilesToTx(ctx context.Context, tx pgx.Tx, teamID int, configRepoID int64, plan *gitOpsTeamAIProfilePlan, commitSHA string) error {
	ids := make([]string, 0, len(plan.agentProfiles))
	for id := range plan.agentProfiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		if _, err := tx.Exec(ctx, `
			DELETE FROM team_agent_profiles
			WHERE team_id = $1 AND managed_by_config_repo = TRUE AND config_repo_id = $2
		`, teamID, configRepoID); err != nil {
			return fmt.Errorf("clear GitOps team agent profiles: %w", err)
		}
	} else if _, err := tx.Exec(ctx, `
		DELETE FROM team_agent_profiles
		WHERE team_id = $1 AND managed_by_config_repo = TRUE AND config_repo_id = $2 AND id != ALL($3::text[])
	`, teamID, configRepoID, ids); err != nil {
		return fmt.Errorf("prune GitOps team agent profiles: %w", err)
	}
	for _, id := range ids {
		if err := upsertTeamAgentProfileTx(ctx, tx, teamID, plan.agentProfiles[id], "gitops", plan.sourcePath, commitSHA, configRepoID, true); err != nil {
			return err
		}
	}
	if plan.agentDefaultProfile != nil {
		if err := persistTeamProfileSettingTx(ctx, tx, teamID, teamAgentDefaultProfileSetting, *plan.agentDefaultProfile); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) persistGitOpsTeamMCPProfilesToTx(ctx context.Context, tx pgx.Tx, teamID int, configRepoID int64, plan *gitOpsTeamAIProfilePlan, commitSHA string) error {
	servers, err := a.loadMCPServersFromDB(ctx)
	if err != nil {
		return fmt.Errorf("load MCP servers for team profile validation: %w", err)
	}
	if len(servers) == 0 {
		servers = a.getConfigSnapshot().EffectiveMCPServers()
	}
	toolsByServer, err := a.loadMCPToolsByServer(ctx)
	if err != nil {
		return fmt.Errorf("load MCP tools for team profile validation: %w", err)
	}
	names := make([]string, 0, len(plan.mcpProfiles))
	for name, profile := range plan.mcpProfiles {
		profile.Name = name
		if err := mcpregistry.ValidateProfileDefinition(profile, servers, toolsByServer); err != nil {
			return fmt.Errorf("invalid team MCP profile in '%s': %w", plan.sourcePath, err)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		if _, err := tx.Exec(ctx, `
			DELETE FROM team_mcp_profiles
			WHERE team_id = $1 AND managed_by_config_repo = TRUE AND config_repo_id = $2
		`, teamID, configRepoID); err != nil {
			return fmt.Errorf("clear GitOps team MCP profiles: %w", err)
		}
	} else if _, err := tx.Exec(ctx, `
		DELETE FROM team_mcp_profiles
		WHERE team_id = $1 AND managed_by_config_repo = TRUE AND config_repo_id = $2 AND name != ALL($3::text[])
	`, teamID, configRepoID, names); err != nil {
		return fmt.Errorf("prune GitOps team MCP profiles: %w", err)
	}
	for _, name := range names {
		if err := upsertTeamMCPProfileTx(ctx, tx, teamID, plan.mcpProfiles[name], "gitops", plan.sourcePath, commitSHA, configRepoID, true); err != nil {
			return err
		}
	}
	return nil
}

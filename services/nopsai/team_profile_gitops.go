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

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"
)

const configRepositoryTeamDefaultsPath = "defaults.yaml"

type teamDefaultsGitOpsFile struct {
	LLMProfile       *string           `json:"model,omitempty" yaml:"model,omitempty"`
	AgentProfile     *string           `json:"agent_role,omitempty" yaml:"agent_role,omitempty"`
	KnowledgeContext map[string]string `json:"knowledge_context,omitempty" yaml:"knowledge_context,omitempty"`
}

type gitOpsTeamDefaultsPlan struct {
	teamPath            string
	sourcePath          string
	llmDefaultProfile   *string
	agentDefaultProfile *string
	knowledgeDefaults   map[string]string
}

func parseGitOpsTeamDefaultsPlans(binding models.ConfigRepository, repoCtx configSyncRepositoryContext, configRepositoryFiles map[string]string) (map[string]*gitOpsTeamDefaultsPlan, error) {
	plans := map[string]*gitOpsTeamDefaultsPlan{}
	addPlan := func(plan *gitOpsTeamDefaultsPlan) error {
		key := strings.ToLower(strings.Trim(plan.teamPath, "/"))
		if existing, exists := plans[key]; exists {
			return fmt.Errorf("duplicate team defaults for team '%s' in '%s' and '%s'", plan.teamPath, existing.sourcePath, plan.sourcePath)
		}
		plans[key] = plan
		return nil
	}

	configRepositoryDir := filepath.ToSlash(strings.Trim(repoCtx.dirs.configRepository, "/"))
	for rawPath, content := range configRepositoryFiles {
		normalized := filepath.ToSlash(rawPath)
		rel, ok := configsync.RelativePath(normalized, configRepositoryDir)
		if !ok {
			continue
		}
		teamPath, ok, err := configRepositoryTeamDefaultsFileScope(rel)
		if err != nil {
			return nil, fmt.Errorf("invalid team defaults path '%s': %w", normalized, err)
		}
		if !ok {
			continue
		}
		if binding.ScopeType == models.ConfigRepositoryScopeTeam {
			teamPath, err = configsync.NormalizePathForTeam(repoCtx.boundTeam, teamPath)
			if err != nil {
				return nil, fmt.Errorf("invalid team defaults path '%s': %w", normalized, err)
			}
		}
		teamPath = strings.Trim(strings.TrimSpace(teamPath), "/")
		if binding.ScopeType == models.ConfigRepositoryScopeTeam && !configsync.ResourceUnderScope(teamPath, repoCtx.boundTeam) {
			return nil, fmt.Errorf("team defaults '%s' targets team '%s' outside bound team '%s'", normalized, teamPath, repoCtx.boundTeam)
		}
		plan, err := parseGitOpsTeamDefaultsFile(content, normalized, teamPath)
		if err != nil {
			return nil, err
		}
		if err := addPlan(plan); err != nil {
			return nil, err
		}
	}
	return plans, nil
}

func parseGitOpsTeamDefaultsFile(content, sourcePath, teamPath string) (*gitOpsTeamDefaultsPlan, error) {
	var file teamDefaultsGitOpsFile
	if err := yaml.Unmarshal([]byte(content), &file); err != nil {
		return nil, fmt.Errorf("failed to parse team defaults GitOps file '%s': %w", sourcePath, err)
	}
	sourceKind := "team defaults GitOps file"
	teamPath = strings.Trim(strings.TrimSpace(teamPath), "/")
	if teamPath == "" {
		return nil, fmt.Errorf("team defaults GitOps file '%s' is missing a team scope", sourcePath)
	}

	llmDefault, _, err := mergeGitOpsDefaultValue(nil, "", file.LLMProfile, "model", sourcePath, sourceKind, normalizeOptionalLLMDefault)
	if err != nil {
		return nil, err
	}
	agentDefault, _, err := mergeGitOpsDefaultValue(nil, "", file.AgentProfile, "agent_role", sourcePath, sourceKind, normalizeOptionalAgentDefault)
	if err != nil {
		return nil, err
	}
	knowledgeDefaults := map[string]string(nil)
	knowledgeDefaults, err = mergeGitOpsKnowledgeDefaults(knowledgeDefaults, file.KnowledgeContext, "knowledge_context", sourcePath, sourceKind, teamPath)
	if err != nil {
		return nil, err
	}
	if llmDefault != nil && *llmDefault != "" {
		defaultTeamPath := aiResourceTeamPath(*llmDefault)
		if defaultTeamPath != "" && !strings.EqualFold(defaultTeamPath, teamPath) {
			return nil, fmt.Errorf("team defaults GitOps file '%s' sets LLM default profile %q outside team %q", sourcePath, *llmDefault, teamPath)
		}
	}
	if agentDefault != nil && *agentDefault != "" {
		defaultTeamPath := aiResourceTeamPath(*agentDefault)
		if defaultTeamPath != "" && !strings.EqualFold(defaultTeamPath, teamPath) {
			return nil, fmt.Errorf("team defaults GitOps file '%s' sets agent default profile %q outside team %q", sourcePath, *agentDefault, teamPath)
		}
	}
	if llmDefault == nil && agentDefault == nil && len(knowledgeDefaults) == 0 {
		return nil, fmt.Errorf("team defaults GitOps file '%s' must define at least one default", sourcePath)
	}
	return &gitOpsTeamDefaultsPlan{
		teamPath:            teamPath,
		sourcePath:          sourcePath,
		llmDefaultProfile:   llmDefault,
		agentDefaultProfile: agentDefault,
		knowledgeDefaults:   knowledgeDefaults,
	}, nil
}

func mergeGitOpsDefaultValue(current *string, currentLabel string, incoming *string, incomingLabel string, sourcePath string, sourceKind string, normalize func(*string) *string) (*string, string, error) {
	normalized := normalize(incoming)
	if normalized == nil {
		return current, currentLabel, nil
	}
	if current != nil && *current != *normalized {
		return nil, "", fmt.Errorf("%s '%s' sets conflicting defaults in %s and %s", sourceKind, sourcePath, currentLabel, incomingLabel)
	}
	return normalized, incomingLabel, nil
}

func mergeGitOpsKnowledgeDefaults(current map[string]string, incoming map[string]string, incomingLabel string, sourcePath string, sourceKind string, teamPath string) (map[string]string, error) {
	if incoming == nil {
		return current, nil
	}
	if current == nil {
		current = map[string]string{}
	}
	for rawKind, rawRef := range incoming {
		kind, err := normalizeKnowledgeContextKind(rawKind)
		if err != nil {
			return nil, fmt.Errorf("%s '%s' has invalid %s kind %q: %w", sourceKind, sourcePath, incomingLabel, rawKind, err)
		}
		canonical, err := normalizeGitOpsKnowledgeDefaultRef(kind, teamPath, rawRef)
		if err != nil {
			return nil, fmt.Errorf("%s '%s' has invalid %s.%s default %q: %w", sourceKind, sourcePath, incomingLabel, kind, rawRef, err)
		}
		if existing, ok := current[kind]; ok && existing != canonical {
			return nil, fmt.Errorf("%s '%s' sets conflicting %s knowledge defaults in multiple locations", sourceKind, sourcePath, kind)
		}
		current[kind] = canonical
	}
	return current, nil
}

func normalizeGitOpsKnowledgeDefaultRef(kind, teamPath, rawRef string) (string, error) {
	rawRef = strings.TrimSpace(rawRef)
	if rawRef == "" {
		return "", nil
	}
	_, refTeam, name, err := teamKnowledgeDefaultRefParts(kind, teamPath, rawRef)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(refTeam, teamPath) {
		return "", fmt.Errorf("knowledge default must reference a %s document owned by %s", kind, teamPath)
	}
	return buildKnowledgeDocumentIdentifier(refTeam, name), nil
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

func (a *App) persistTeamDefaultsToTx(ctx context.Context, tx pgx.Tx, plan *gitOpsTeamDefaultsPlan) error {
	if plan == nil {
		return nil
	}
	teamID, err := resolveTeamDefaultsTeamID(ctx, tx, plan.teamPath)
	if err != nil {
		return err
	}
	record := teamPathRecord{ID: teamID, Path: plan.teamPath}
	if plan.llmDefaultProfile != nil {
		profiles, err := loadTeamLLMProfilesForDefaultWithRunner(ctx, tx, teamID)
		if err != nil {
			return fmt.Errorf("load team LLM profiles for defaults: %w", err)
		}
		canonical, ok := canonicalTeamLLMDefaultProfileValue(record, *plan.llmDefaultProfile, profiles, a.getConfigSnapshot().EffectiveLLMProfiles())
		if !ok {
			return fmt.Errorf("team defaults GitOps file '%s' sets unknown LLM default profile %q", plan.sourcePath, *plan.llmDefaultProfile)
		}
		if err := persistTeamProfileSettingTx(ctx, tx, teamID, teamLLMDefaultProfileSetting, canonical); err != nil {
			return err
		}
	}
	if plan.agentDefaultProfile != nil {
		profiles, err := loadTeamAgentProfilesForDefaultWithRunner(ctx, tx, teamID)
		if err != nil {
			return fmt.Errorf("load team agent profiles for defaults: %w", err)
		}
		effectiveProfiles, _, err := a.effectiveAgentProfiles(ctx)
		if err != nil {
			return fmt.Errorf("load agent profiles for team default validation: %w", err)
		}
		canonical, ok := canonicalTeamAgentDefaultProfileValue(record, *plan.agentDefaultProfile, profiles, effectiveProfiles)
		if !ok {
			return fmt.Errorf("team defaults GitOps file '%s' sets unknown or disabled agent default profile %q", plan.sourcePath, *plan.agentDefaultProfile)
		}
		if err := persistTeamProfileSettingTx(ctx, tx, teamID, teamAgentDefaultProfileSetting, canonical); err != nil {
			return err
		}
	}
	if len(plan.knowledgeDefaults) > 0 {
		kinds := make([]string, 0, len(plan.knowledgeDefaults))
		for kind := range plan.knowledgeDefaults {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		for _, kind := range kinds {
			rawRef := plan.knowledgeDefaults[kind]
			canonical, ok, err := canonicalTeamKnowledgeDefaultWithRunner(ctx, tx, record, kind, rawRef)
			if err != nil {
				return fmt.Errorf("validate %s knowledge default from '%s': %w", kind, plan.sourcePath, err)
			}
			if !ok {
				return fmt.Errorf("team defaults GitOps file '%s' sets unknown %s knowledge default %q", plan.sourcePath, kind, rawRef)
			}
			if err := persistTeamProfileSettingTx(ctx, tx, teamID, teamKnowledgeDefaultSettingKey(kind), canonical); err != nil {
				return err
			}
		}
	}
	return nil
}

func loadTeamLLMProfilesForDefaultWithRunner(ctx context.Context, runner queryRunner, teamID int) (map[string]config.LLMProfile, error) {
	rows, err := runner.Query(ctx, `
		SELECT name
		FROM team_llm_profiles
		WHERE team_id = $1
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := map[string]config.LLMProfile{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		name = config.NormalizeLLMProfileName(name)
		if name != "" {
			profiles[name] = config.LLMProfile{}
		}
	}
	return profiles, rows.Err()
}

func loadTeamAgentProfilesForDefaultWithRunner(ctx context.Context, runner queryRunner, teamID int) (map[string]models.AgentProfile, error) {
	rows, err := runner.Query(ctx, `
		SELECT id, enabled
		FROM team_agent_profiles
		WHERE team_id = $1
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := map[string]models.AgentProfile{}
	for rows.Next() {
		var profile models.AgentProfile
		if err := rows.Scan(&profile.ID, &profile.Enabled); err != nil {
			return nil, err
		}
		profile.ID = models.NormalizeAgentProfileID(profile.ID)
		if profile.ID != "" {
			profiles[profile.ID] = profile
		}
	}
	return profiles, rows.Err()
}

func sortedTeamDefaultsPlans(plans map[string]*gitOpsTeamDefaultsPlan) []*gitOpsTeamDefaultsPlan {
	out := make([]*gitOpsTeamDefaultsPlan, 0, len(plans))
	for _, plan := range plans {
		if plan != nil {
			out = append(out, plan)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].teamPath) < strings.ToLower(out[j].teamPath)
	})
	return out
}

func resolveTeamDefaultsTeamID(ctx context.Context, tx pgx.Tx, teamPath string) (int, error) {
	teamPath = strings.Trim(strings.TrimSpace(teamPath), "/")
	if teamPath == "" {
		return 0, fmt.Errorf("team defaults scope is required")
	}
	var (
		teamID int
		kind   string
	)
	if err := tx.QueryRow(ctx, `
		SELECT id, COALESCE(kind, 'team') FROM teams WHERE name = $1
	`, teamPath).Scan(&teamID, &kind); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			records, recordsErr := loadTeamPathRecords(ctx, tx)
			if recordsErr != nil {
				return 0, fmt.Errorf("failed to resolve team defaults owner '%s': %w", teamPath, recordsErr)
			}
			for _, record := range records {
				if record.Path != teamPath {
					continue
				}
				if strings.EqualFold(record.Kind, "app") {
					return 0, fmt.Errorf("team defaults must be attached to a team, not application '%s'", teamPath)
				}
				return record.ID, nil
			}
			return 0, fmt.Errorf("team defaults reference missing team '%s'", teamPath)
		}
		return 0, fmt.Errorf("failed to resolve team defaults owner '%s': %w", teamPath, err)
	}
	if strings.EqualFold(kind, "app") {
		return 0, fmt.Errorf("team defaults must be attached to a team, not application '%s'", teamPath)
	}
	return teamID, nil
}

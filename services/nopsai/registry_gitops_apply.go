package nopsai

import (
	"context"
	"fmt"
	"sort"

	"nopsai/pkg/models"

	"github.com/jackc/pgx/v5"
)

// persistGitOpsAgentRolesToTx replaces the Git-owned global agent role registry
// and its default with the roles this repository declares.
func persistGitOpsAgentRolesToTx(
	ctx context.Context,
	tx pgx.Tx,
	plan *gitOpsAgentRolePlan,
	defaultRole string,
	roles map[string]models.AgentProfile,
	configRepoID int64,
	commitSHA string,
) error {
	if plan == nil {
		return nil
	}
	ids := make([]string, 0, len(roles))
	for id := range roles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM agent_profiles WHERE source = 'gitops'`); err != nil {
			return fmt.Errorf("clear GitOps agent roles: %w", err)
		}
	} else if _, err := tx.Exec(ctx, `DELETE FROM agent_profiles WHERE source = 'gitops' AND id != ALL($1::text[])`, ids); err != nil {
		return fmt.Errorf("prune GitOps agent roles: %w", err)
	}
	for _, id := range ids {
		sourcePath := plan.roles[id].sourcePath
		if err := persistAgentProfileToTx(ctx, tx, roles[id], "gitops", sourcePath, commitSHA, configRepoID, true); err != nil {
			return err
		}
	}
	return persistAgentProfileDefaultToTx(ctx, tx, defaultRole)
}

// persistGitOpsTeamModels writes the team-owned model registry for every team
// this repository declares models for, prunes the models it no longer declares,
// and applies the team default when a file claims it.
func (a *App) persistGitOpsTeamModels(
	ctx context.Context,
	tx pgx.Tx,
	binding models.ConfigRepository,
	plan *gitOpsModelPlan,
	commitSHA string,
) (int, error) {
	synced := 0
	byTeam := plan.teamModels()
	for _, teamPath := range sortedRegistryTeams(byTeam) {
		teamID, err := resolveTeamDefaultsTeamID(ctx, tx, teamPath)
		if err != nil {
			return 0, fmt.Errorf("resolve team %q for models: %w", teamPath, err)
		}
		names := sortedModelNames(byTeam[teamPath])
		if _, err := tx.Exec(ctx, `
			DELETE FROM team_llm_profiles
			WHERE team_id = $1 AND managed_by_config_repo = TRUE AND config_repo_id = $2 AND name != ALL($3::text[])
		`, teamID, binding.ID, names); err != nil {
			return 0, fmt.Errorf("prune team models for %q: %w", teamPath, err)
		}
		for _, name := range names {
			stored := byTeam[teamPath][name]
			if err := upsertTeamLLMProfileTxWithSource(ctx, tx, teamID, name, stored.profile, "git", stored.sourcePath, commitSHA, binding.ID, true); err != nil {
				return 0, fmt.Errorf("sync team model %q: %w", teamPath+"/"+name, err)
			}
			synced++
		}
		if defaultName, ok := plan.defaults[teamPath]; ok {
			if err := persistTeamProfileSettingTx(ctx, tx, teamID, teamLLMDefaultProfileSetting, defaultName); err != nil {
				return 0, fmt.Errorf("set team model default for %q: %w", teamPath, err)
			}
		}
	}
	return synced, nil
}

func (a *App) persistGitOpsTeamAgentRoles(
	ctx context.Context,
	tx pgx.Tx,
	binding models.ConfigRepository,
	plan *gitOpsAgentRolePlan,
	commitSHA string,
) (int, error) {
	synced := 0
	byTeam := plan.teamRoles()
	for _, teamPath := range sortedRegistryTeams(byTeam) {
		teamID, err := resolveTeamDefaultsTeamID(ctx, tx, teamPath)
		if err != nil {
			return 0, fmt.Errorf("resolve team %q for agent roles: %w", teamPath, err)
		}
		names := sortedAgentRoleNames(byTeam[teamPath])
		if _, err := tx.Exec(ctx, `
			DELETE FROM team_agent_profiles
			WHERE team_id = $1 AND managed_by_config_repo = TRUE AND config_repo_id = $2 AND id != ALL($3::text[])
		`, teamID, binding.ID, names); err != nil {
			return 0, fmt.Errorf("prune team agent roles for %q: %w", teamPath, err)
		}
		for _, name := range names {
			stored := byTeam[teamPath][name]
			if err := upsertTeamAgentProfileTx(ctx, tx, teamID, stored.profile, "git", stored.sourcePath, commitSHA, binding.ID, true); err != nil {
				return 0, fmt.Errorf("sync team agent role %q: %w", teamPath+"/"+name, err)
			}
			synced++
		}
		if defaultName, ok := plan.defaults[teamPath]; ok {
			if err := persistTeamProfileSettingTx(ctx, tx, teamID, teamAgentDefaultProfileSetting, defaultName); err != nil {
				return 0, fmt.Errorf("set team agent role default for %q: %w", teamPath, err)
			}
		}
	}
	return synced, nil
}

func (a *App) persistGitOpsTeamMCPProfiles(
	ctx context.Context,
	tx pgx.Tx,
	binding models.ConfigRepository,
	plan *gitOpsMCPRegistryPlan,
	commitSHA string,
) (int, error) {
	synced := 0
	byTeam := plan.teamProfiles()
	for _, teamPath := range sortedRegistryTeams(byTeam) {
		teamID, err := resolveTeamDefaultsTeamID(ctx, tx, teamPath)
		if err != nil {
			return 0, fmt.Errorf("resolve team %q for MCP profiles: %w", teamPath, err)
		}
		names := sortedMCPProfileNames(byTeam[teamPath])
		if _, err := tx.Exec(ctx, `
			DELETE FROM team_mcp_profiles
			WHERE team_id = $1 AND managed_by_config_repo = TRUE AND config_repo_id = $2 AND name != ALL($3::text[])
		`, teamID, binding.ID, names); err != nil {
			return 0, fmt.Errorf("prune team MCP profiles for %q: %w", teamPath, err)
		}
		for _, name := range names {
			stored := byTeam[teamPath][name]
			if err := upsertTeamMCPProfileTx(ctx, tx, teamID, stored.profile, "git", stored.sourcePath, commitSHA, binding.ID, true); err != nil {
				return 0, fmt.Errorf("sync team MCP profile %q: %w", teamPath+"/"+name, err)
			}
			synced++
		}
	}
	return synced, nil
}

func sortedRegistryTeams[T any](byTeam map[string]map[string]T) []string {
	teams := make([]string, 0, len(byTeam))
	for team := range byTeam {
		teams = append(teams, team)
	}
	sort.Strings(teams)
	return teams
}

func sortedModelNames(models map[string]storedModel) []string {
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedAgentRoleNames(roles map[string]storedAgentRole) []string {
	names := make([]string, 0, len(roles))
	for name := range roles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedMCPProfileNames(profiles map[string]storedMCPProfile) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

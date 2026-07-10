package nopsai

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"nopsai/config"
	"nopsai/pkg/models"
)

func (a *App) teamIDForRunProfileOwner(ctx context.Context, runID string) (*int, error) {
	if a == nil || a.db == nil {
		return nil, nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, nil
	}
	var groupID sql.NullInt32
	err := a.db.QueryRow(ctx, `
		SELECT group_id FROM pipeline_runs WHERE run_id::text = $1
	`, runID).Scan(&groupID)
	if errorsIsNoRows(err) || !groupID.Valid {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load run profile owner: %w", err)
	}
	return a.teamIDForGroupProfileOwner(ctx, int(groupID.Int32))
}

func (a *App) teamIDForGroupProfileOwner(ctx context.Context, groupID int) (*int, error) {
	var (
		kind     string
		parentID sql.NullInt32
	)
	err := a.db.QueryRow(ctx, `
		SELECT COALESCE(kind, 'group'), parent_id FROM groups WHERE id = $1
	`, groupID).Scan(&kind, &parentID)
	if errorsIsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load group profile owner: %w", err)
	}
	if strings.EqualFold(kind, "app") {
		if !parentID.Valid {
			return nil, nil
		}
		teamID := int(parentID.Int32)
		return &teamID, nil
	}
	teamID := groupID
	return &teamID, nil
}

func (a *App) effectiveLLMProfilesForTeam(ctx context.Context, cfg config.Config, teamID *int) (string, map[string]config.LLMProfile, error) {
	defaultProfile := cfg.EffectiveLLMDefaultProfile()
	profiles := cloneLLMProfiles(cfg.EffectiveLLMProfiles())
	if teamID == nil || a == nil || a.db == nil {
		return defaultProfile, profiles, nil
	}
	teamDefault, teamProfiles, err := a.loadTeamLLMProfilesFromDB(ctx, *teamID)
	if err != nil {
		return "", nil, err
	}
	for name, profile := range teamProfiles {
		profiles[name] = config.NormalizeLLMProfile(profile)
	}
	teamDefault = config.NormalizeLLMProfileName(teamDefault)
	if teamDefault != "" {
		if _, ok := profiles[teamDefault]; ok {
			defaultProfile = teamDefault
		}
	}
	return defaultProfile, profiles, nil
}

func cloneLLMProfiles(raw map[string]config.LLMProfile) map[string]config.LLMProfile {
	if len(raw) == 0 {
		return map[string]config.LLMProfile{}
	}
	cloned := make(map[string]config.LLMProfile, len(raw))
	for name, profile := range raw {
		cloned[name] = config.NormalizeLLMProfile(profile)
	}
	return cloned
}

func (a *App) effectiveAgentProfilesForTeam(ctx context.Context, teamID *int) (map[string]models.AgentProfile, string, error) {
	profiles, _, err := a.effectiveAgentProfiles(ctx)
	if err != nil {
		return nil, "", err
	}
	defaultProfile, err := a.effectiveAgentProfileDefault(ctx, profiles)
	if err != nil {
		return nil, "", err
	}
	if teamID == nil || a == nil || a.db == nil {
		return profiles, defaultProfile, nil
	}
	teamProfiles, err := a.loadTeamAgentProfilesFromDB(ctx, *teamID)
	if err != nil {
		return nil, "", err
	}
	for id, profile := range teamProfiles {
		profile = models.NormalizeAgentProfile(profile)
		if profile.Source == "" {
			profile.Source = "team"
		}
		profile.BuiltIn = false
		profiles[id] = profile
	}
	teamDefault, err := a.loadTeamProfileSetting(ctx, *teamID, teamAgentDefaultProfileSetting)
	if err != nil {
		return nil, "", err
	}
	teamDefault = normalizeAgentProfileDefault(teamDefault)
	if profile, ok := profiles[teamDefault]; ok && profile.Enabled {
		defaultProfile = teamDefault
	}
	return profiles, defaultProfile, nil
}

func (a *App) effectiveMCPProfilesForTeam(ctx context.Context, cfg config.Config, teamID *int) (map[string]models.MCPProfile, error) {
	profiles := cloneMCPProfiles(cfg.EffectiveMCPProfiles())
	if teamID == nil || a == nil || a.db == nil {
		return profiles, nil
	}
	teamProfiles, err := a.loadTeamMCPProfilesFromDB(ctx, *teamID)
	if err != nil {
		return nil, err
	}
	for name, profile := range teamProfiles {
		profiles[name] = models.NormalizeMCPProfile(profile)
	}
	return profiles, nil
}

func cloneMCPProfiles(raw map[string]models.MCPProfile) map[string]models.MCPProfile {
	if len(raw) == 0 {
		return map[string]models.MCPProfile{}
	}
	cloned := make(map[string]models.MCPProfile, len(raw))
	for name, profile := range raw {
		cloned[name] = models.NormalizeMCPProfile(profile)
	}
	return cloned
}

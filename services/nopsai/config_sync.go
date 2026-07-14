package nopsai

import (
	"context"

	"nopsai/pkg/models"

	"github.com/rs/zerolog/log"
)

func (a *App) syncConfigurationFromGit(ctx context.Context, binding models.ConfigRepository) (map[string]int, string, error) {
	details := map[string]int{
		"pipelines_synced":               0,
		"steps_synced":                   0,
		"general_vars_synced":            0,
		"repo_vars_synced":               0,
		"triggers_synced":                0,
		"external_triggers_synced":       0,
		"git_webhook_sources_synced":     0,
		"schedules_synced":               0,
		"secrets_synced":                 0,
		"config_repositories_synced":     0,
		"run_teams_created":              0,
		"run_teams_updated":              0,
		"access_users_synced":            0,
		"access_service_accounts_synced": 0,
		"access_roles_synced":            0,
		"access_policies_synced":         0,
		"access_role_bindings_synced":    0,
		"access_grants_synced":           0,
		"resource_access_synced":         0,
		"llm_profiles_synced":            0,
		"agent_profiles_synced":          0,
		"mcp_servers_synced":             0,
		"mcp_profiles_synced":            0,
		"knowledge_contexts_synced":      0,
		"auth_settings_synced":           0,
		"credentials_synced":             0,
		"runtime_settings_synced":        0,
		"mail_settings_synced":           0,
		"notification_routes_synced":     0,
	}

	repoCtx, err := newConfigSyncRepositoryContext(binding)
	if err != nil {
		return nil, "", err
	}
	commitSHA := ""

	client, _, err := a.newConfigRepositoryGitContentClient(ctx, binding)
	if err != nil {
		return nil, commitSHA, err
	}

	files, err := fetchConfigSyncRepositoryFiles(ctx, client, repoCtx, binding)
	if err != nil {
		return nil, commitSHA, err
	}

	plan, err := a.parseConfigSyncPlan(binding, repoCtx, files)
	if err != nil {
		return nil, commitSHA, err
	}

	if err := a.configSyncStore().ApplyConfigSyncPlan(ctx, binding, plan, details, commitSHA); err != nil {
		return nil, commitSHA, err
	}

	log.Info().
		Str("git_provider", repoCtx.provider).
		Str("git_host", repoCtx.host).
		Str("git_project", repoCtx.project).
		Str("repo_owner", repoCtx.owner).
		Str("repo_name", repoCtx.repo).
		Int("pipelines_synced", details["pipelines_synced"]).
		Int("steps_synced", details["steps_synced"]).
		Int("knowledge_contexts_synced", details["knowledge_contexts_synced"]).
		Int("general_vars_synced", details["general_vars_synced"]).
		Int("repo_vars_synced", details["repo_vars_synced"]).
		Int("secrets_synced", details["secrets_synced"]).
		Int("triggers_synced", details["triggers_synced"]).
		Int("external_triggers_synced", details["external_triggers_synced"]).
		Int("git_webhook_sources_synced", details["git_webhook_sources_synced"]).
		Int("config_repositories_synced", details["config_repositories_synced"]).
		Int("run_teams_created", details["run_teams_created"]).
		Int("run_teams_updated", details["run_teams_updated"]).
		Int("access_users_synced", details["access_users_synced"]).
		Int("access_service_accounts_synced", details["access_service_accounts_synced"]).
		Int("access_roles_synced", details["access_roles_synced"]).
		Int("access_policies_synced", details["access_policies_synced"]).
		Int("access_role_bindings_synced", details["access_role_bindings_synced"]).
		Int("access_grants_synced", details["access_grants_synced"]).
		Int("resource_access_synced", details["resource_access_synced"]).
		Int("llm_profiles_synced", details["llm_profiles_synced"]).
		Int("agent_profiles_synced", details["agent_profiles_synced"]).
		Int("mcp_servers_synced", details["mcp_servers_synced"]).
		Int("mcp_profiles_synced", details["mcp_profiles_synced"]).
		Int("auth_settings_synced", details["auth_settings_synced"]).
		Int("credentials_synced", details["credentials_synced"]).
		Int("mail_settings_synced", details["mail_settings_synced"]).
		Int("notification_routes_synced", details["notification_routes_synced"]).
		Msg("Configuration synchronization from Git completed")

	return details, commitSHA, nil
}

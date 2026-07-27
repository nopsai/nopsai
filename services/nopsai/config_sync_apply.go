package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"
)

const configSyncScheduleUpsertSQL = `INSERT INTO pipeline_schedules (
		path, name, description, pipeline_path, pipeline_name, pipeline_version,
		schedule_kind, cron_expression, run_at, timezone, enabled, scope, variables, next_run_at, source,
		visibility, run_team_path, config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at
	) VALUES (
		$1, $2, $3, $4, $5, $6,
		$7, $8, $9, $10, $11, $12, $13::jsonb, $14, 'git',
		$15, $16, $17, $18, $19, TRUE, NOW()
	)
	ON CONFLICT (path, name) DO UPDATE SET
		description = EXCLUDED.description,
		pipeline_path = EXCLUDED.pipeline_path,
		pipeline_name = EXCLUDED.pipeline_name,
		pipeline_version = EXCLUDED.pipeline_version,
		schedule_kind = EXCLUDED.schedule_kind,
		cron_expression = EXCLUDED.cron_expression,
		run_at = EXCLUDED.run_at,
		timezone = EXCLUDED.timezone,
		enabled = EXCLUDED.enabled,
		scope = EXCLUDED.scope,
		variables = EXCLUDED.variables,
		next_run_at = EXCLUDED.next_run_at,
		run_team_path = EXCLUDED.run_team_path,
		source = 'git',
		config_repo_id = EXCLUDED.config_repo_id,
		config_source_path = EXCLUDED.config_source_path,
		config_source_commit_sha = EXCLUDED.config_source_commit_sha,
		managed_by_config_repo = TRUE,
		updated_at = NOW()
	RETURNING id::text`

func (a *App) applyConfigSyncPlan(ctx context.Context, binding models.ConfigRepository, plan configSyncPlan, details map[string]int, commitSHA string) error {
	configRepositoryPipelineRunStructure := plan.configRepositoryPipelineRunStructure
	configRepositories := plan.configRepositories
	accessPlan := plan.accessPlan
	knowledgeContexts := plan.knowledgeContexts
	llmProfilePlan := plan.llmProfilePlan
	agentProfilePlan := plan.agentProfilePlan
	mcpRegistryPlan := plan.mcpRegistryPlan
	teamAIProfilePlan := plan.teamAIProfilePlan
	authSettingsPlan := plan.authSettingsPlan
	credentialPlan := plan.credentialPlan
	runtimeSettingsPlan := plan.runtimeSettingsPlan
	githubSettingsPlan := plan.githubSettingsPlan
	mailSettingsPlan := plan.mailSettingsPlan
	dataManagementPlan := plan.dataManagementPlan
	schedules := plan.schedules
	dashboards := plan.dashboards
	externalTriggers := plan.externalTriggers
	gitWebhookSources := plan.gitWebhookSources
	notificationRoutes := plan.notificationRoutes
	pipelines := plan.pipelines
	steps := plan.steps
	generalScopeVars := plan.generalScopeVars
	repoScopeVars := plan.repoScopeVars
	generalScopeSecrets := plan.generalScopeSecrets
	repoScopeSecrets := plan.repoScopeSecrets
	triggers := plan.triggers

	credentialActor := fmt.Sprintf("gitops:config-repository:%d", binding.ID)
	credentialRepoID := binding.ID
	credentialMetadata := func(kind, description, sourcePath string) createCredentialInput {
		return createCredentialInput{
			Kind:                  kind,
			Description:           description,
			Actor:                 credentialActor,
			ManagedByConfigRepo:   true,
			ConfigRepoID:          &credentialRepoID,
			ConfigSourcePath:      sourcePath,
			ConfigSourceCommitSHA: commitSHA,
		}
	}
	if llmProfilePlan != nil {
		for name, profile := range llmProfilePlan.profiles {
			if err := a.ensureCredentialReferenceMetadata(
				ctx,
				profile.CredentialRef,
				credentialMetadata("api_key", "LLM API key for "+name, llmProfilePlan.sourcePath),
			); err != nil {
				return fmt.Errorf("prepare LLM credential metadata for %q: %w", name, err)
			}
		}
	}
	if teamAIProfilePlan != nil {
		for name, profile := range teamAIProfilePlan.llmProfiles {
			if err := a.ensureCredentialReferenceMetadata(
				ctx,
				profile.CredentialRef,
				credentialMetadata("api_key", "Team LLM API key for "+name, teamAIProfilePlan.sourcePath),
			); err != nil {
				return fmt.Errorf("prepare team LLM credential metadata for %q: %w", name, err)
			}
		}
	}
	if mcpRegistryPlan != nil {
		for name, server := range mcpRegistryPlan.Servers {
			if err := a.ensureCredentialReferenceMetadata(
				ctx,
				server.CredentialRef,
				credentialMetadata("bearer_token", "MCP bearer token for "+name, mcpRegistryPlan.SourcePath),
			); err != nil {
				return fmt.Errorf("prepare MCP credential metadata for %q: %w", name, err)
			}
		}
	}
	if mailSettingsPlan != nil {
		if err := a.ensureCredentialReferenceMetadata(
			ctx,
			mailSettingsPlan.settings.SMTP.PasswordCredentialRef,
			credentialMetadata("password", "SMTP authentication password", mailSettingsPlan.sourcePath),
		); err != nil {
			return fmt.Errorf("prepare mail credential metadata: %w", err)
		}
	}
	if githubSettingsPlan != nil {
		if githubSettingsPlan.payload.GitHubPrivateKeyRef != nil {
			if err := a.ensureCredentialReferenceMetadata(
				ctx,
				*githubSettingsPlan.payload.GitHubPrivateKeyRef,
				credentialMetadata("private_key", "GitHub App private key", githubSettingsPlan.sourcePath),
			); err != nil {
				return fmt.Errorf("prepare GitHub App private key credential metadata: %w", err)
			}
		}
		if githubSettingsPlan.payload.GitHubWebhookRef != nil {
			if err := a.ensureCredentialReferenceMetadata(
				ctx,
				*githubSettingsPlan.payload.GitHubWebhookRef,
				credentialMetadata("webhook_secret", "GitHub App webhook verification secret", githubSettingsPlan.sourcePath),
			); err != nil {
				return fmt.Errorf("prepare GitHub App webhook credential metadata: %w", err)
			}
		}
	}
	for id, source := range gitWebhookSources {
		if err := a.ensureCredentialReferenceMetadata(
			ctx,
			source.input.CredentialRef,
			credentialMetadata(
				gitWebhookSecretCredentialKind,
				"Webhook secret for Git source "+id,
				source.sourcePath,
			),
		); err != nil {
			return fmt.Errorf("prepare git webhook source credential metadata for %q: %w", id, err)
		}
	}
	if authSettingsPlan != nil {
		for _, provider := range authSettingsPlan.providers {
			if err := a.ensureCredentialReferenceMetadata(
				ctx,
				provider.ClientCredentialRef,
				credentialMetadata("client_secret", "OIDC client secret for "+provider.ID, authSettingsPlan.sourcePath),
			); err != nil {
				return fmt.Errorf("prepare OIDC client credential metadata for %q: %w", provider.ID, err)
			}
			if err := a.ensureCredentialReferenceMetadata(
				ctx,
				provider.EntitlementSync.AdminClientCredentialRef,
				credentialMetadata("client_secret", "OIDC entitlement admin client secret for "+provider.ID, authSettingsPlan.sourcePath),
			); err != nil {
				return fmt.Errorf("prepare OIDC admin client credential metadata for %q: %w", provider.ID, err)
			}
			if err := a.ensureCredentialReferenceMetadata(
				ctx,
				provider.EntitlementSync.AdminPasswordCredentialRef,
				credentialMetadata("password", "OIDC entitlement admin password for "+provider.ID, authSettingsPlan.sourcePath),
			); err != nil {
				return fmt.Errorf("prepare OIDC credential metadata for %q: %w", provider.ID, err)
			}
		}
	}
	for key, repo := range configRepositories {
		if err := a.ensureCredentialReferenceMetadata(
			ctx,
			repo.credentialRef,
			credentialMetadata("bearer_token", "Git provider token for config repository "+key, repo.sourcePath),
		); err != nil {
			return fmt.Errorf("prepare config repository credential metadata for %q: %w", key, err)
		}
	}

	// --- 3. Database Transaction (Upsert + Prune) ---
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	overrideScopes, err := configRepositoryOverrideScopes(ctx, tx, binding, configRepositories)
	if err != nil {
		return err
	}
	filterDelegatedConfigResources(overrideScopes, pipelines, steps, schedules, dashboards, externalTriggers, gitWebhookSources, notificationRoutes, knowledgeContexts, generalScopeVars, repoScopeVars, generalScopeSecrets, repoScopeSecrets, triggers)
	filterDelegatedAccessResources(accessPlan, binding, overrideScopes)
	if err := mergeRepositoryTriggerApplicationsIntoStructure(configRepositoryPipelineRunStructure, triggers); err != nil {
		return fmt.Errorf("failed to prepare repository trigger applications: %w", err)
	}
	effectivePipelineRunStructure, err := effectivePipelineRunStructureForConfigSync(binding, configRepositories, configRepositoryPipelineRunStructure)
	if err != nil {
		return err
	}

	const pipelineUpsert = `INSERT INTO pipelines (
			path, name, version, definition, source,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at
		) VALUES ($1, $2, $3, $4, 'git', $5, $6, $7, TRUE, NOW())
		ON CONFLICT (path, name) DO UPDATE SET
			version = EXCLUDED.version,
			definition = EXCLUDED.definition,
			source = 'git',
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE,
			updated_at = NOW()`
	const stepUpsert = `INSERT INTO steps (
			path, name, definition, source,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at
		) VALUES ($1, $2, $3, 'git', $4, $5, $6, TRUE, NOW())
		ON CONFLICT (path, name) DO UPDATE SET
			definition = EXCLUDED.definition,
			source = 'git',
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE,
			updated_at = NOW()`
	const knowledgeContextUpsert = `INSERT INTO knowledge_contexts (
			kind, team_path, name, description, content,
			source, config_repo_id, config_source_path, config_source_commit_sha,
			managed_by_config_repo, updated_at
		) VALUES ($1, $2, $3, $4, $5, 'git', $6, $7, $8, TRUE, NOW())
		ON CONFLICT (kind, team_path, name) DO UPDATE SET
			description = EXCLUDED.description,
			content = EXCLUDED.content,
			source = 'git',
			content_source = 'inline',
			connection_id = NULL,
			external_provider = '',
			external_page_id = '',
			external_page_url = '',
			external_page_title = '',
			sync_mode = 'manual',
			failure_mode = 'fail',
			sync_failure_mode = 'fail',
			sync_status = 'not_synced',
			last_sync_status = '',
			sync_error = '',
			last_sync_error = '',
			source_modified_at = NULL,
			content_hash = '',
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE,
			updated_at = NOW()`
	const envUpsert = `INSERT INTO variables (
			name, value, repository_name, scope, source,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at
		) VALUES ($1, $2, $3, $4, 'git', $5, $6, $7, TRUE, NOW())
		ON CONFLICT (name, repository_name, scope) DO UPDATE SET
			value = EXCLUDED.value,
			source = 'git',
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE,
			updated_at = NOW()`
	const secretUpsert = `INSERT INTO secrets (
			name, value, repository_name, scope, source,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at
		) VALUES ($1, $2, $3, $4, 'git', $5, $6, $7, TRUE, NOW())
		ON CONFLICT (name, repository_name, scope) DO UPDATE SET
			value = EXCLUDED.value,
			source = 'git',
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE,
			updated_at = NOW()`
	const triggerUpsert = `INSERT INTO triggers (
			repository_name, trigger_definition, source, visibility, provider, team_path, management, webhook_source_id,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
		) VALUES ($1, $2, 'git', $3, $4, $5, $6, NULLIF($7, ''), $8, $9, $10, TRUE)
		ON CONFLICT (repository_name) DO UPDATE SET
			trigger_definition = EXCLUDED.trigger_definition,
			source = 'git',
			visibility = EXCLUDED.visibility,
			provider = EXCLUDED.provider,
			team_path = EXCLUDED.team_path,
			management = EXCLUDED.management,
			webhook_source_id = EXCLUDED.webhook_source_id,
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE`
	const externalTriggerUpsert = `INSERT INTO external_triggers (
			id, name, description, enabled, pipeline, scope, run_team_path, allowed_callers, variable_mapping,
			payload_schema, rate_limit, created_by, source,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::jsonb,
			$10::jsonb, $11::jsonb, 'config-repo', 'git',
			$12, $13, $14, TRUE, NOW()
		)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			enabled = EXCLUDED.enabled,
			pipeline = EXCLUDED.pipeline,
			scope = EXCLUDED.scope,
			run_team_path = EXCLUDED.run_team_path,
			allowed_callers = EXCLUDED.allowed_callers,
			variable_mapping = EXCLUDED.variable_mapping,
			payload_schema = EXCLUDED.payload_schema,
			rate_limit = EXCLUDED.rate_limit,
			source = 'git',
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE,
			updated_at = NOW()`
	const gitWebhookSourceUpsert = `INSERT INTO git_webhook_sources (
			id, name, description, provider, enabled, team_path, visibility, auth_mode, credential_ref,
			repository_allowlist, rate_limit, created_by, source,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9,
			$10::jsonb, $11::jsonb, 'config-repo', 'git',
			$12, $13, $14, TRUE, NOW()
		)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			provider = EXCLUDED.provider,
			enabled = EXCLUDED.enabled,
			team_path = EXCLUDED.team_path,
			visibility = EXCLUDED.visibility,
			auth_mode = EXCLUDED.auth_mode,
			credential_ref = EXCLUDED.credential_ref,
			repository_allowlist = EXCLUDED.repository_allowlist,
			rate_limit = EXCLUDED.rate_limit,
			source = 'git',
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE,
			updated_at = NOW()`
	const notificationRouteUpsert = `INSERT INTO notification_routes (
			team_id, definition, source,
			config_repo_id, config_source_path, config_source_commit_sha,
			managed_by_config_repo, updated_by, updated_at
		) VALUES (
			$1, $2::jsonb, 'git',
			$3, $4, $5,
			TRUE, 'config-repo', NOW()
		)
		ON CONFLICT (team_id) DO UPDATE SET
			definition = EXCLUDED.definition,
			source = 'git',
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE,
			updated_by = EXCLUDED.updated_by,
			updated_at = NOW()`
	const configRepositoryUpsert = `INSERT INTO config_repositories (
			scope_type, scope_id, provider, repo_url, branch, base_path, credential_ref, enabled, write_enabled, write_branch,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo,
			created_by, updated_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, TRUE, 'config-repo', 'config-repo')
		ON CONFLICT (scope_type, scope_id) DO UPDATE SET
			provider = EXCLUDED.provider,
			repo_url = EXCLUDED.repo_url,
			branch = EXCLUDED.branch,
			base_path = EXCLUDED.base_path,
			credential_ref = EXCLUDED.credential_ref,
			enabled = EXCLUDED.enabled,
			write_enabled = EXCLUDED.write_enabled,
			write_branch = EXCLUDED.write_branch,
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE,
			updated_by = EXCLUDED.updated_by,
			updated_at = NOW()`

	// A. Upsert Config Repository Bindings
	for key, stored := range configRepositories {
		writable, err := ensureConfigResourceWritable(ctx, tx, "config_repositories", "config repository", key, binding, stored.scopeID, "scope_type = $1 AND scope_id = $2", stored.scopeType, stored.scopeID)
		if err != nil {
			return err
		}
		if !writable {
			continue
		}
		if _, err := tx.Exec(ctx, configRepositoryUpsert, stored.scopeType, stored.scopeID, stored.provider, stored.repoURL, stored.branch, stored.basePath, stored.credentialRef, stored.enabled, stored.writeEnabled, stored.writeBranch, binding.ID, stored.sourcePath, commitSHA); err != nil {
			return fmt.Errorf("failed to upsert config repository binding '%s': %w", key, err)
		}
		details["config_repositories_synced"]++
	}

	if err := a.applyGitOpsDashboards(ctx, tx, binding, dashboards, commitSHA, details); err != nil {
		return err
	}

	// B. Upsert Pipelines
	for key, stored := range pipelines {
		writable, err := ensureConfigResourceWritable(ctx, tx, "pipelines", "pipeline", key, binding, key, "path = $1 AND name = $2", stored.path, stored.name)
		if err != nil {
			return err
		}
		if !writable {
			continue
		}
		if _, err := tx.Exec(ctx, pipelineUpsert, stored.path, stored.name, stored.version, stored.definition, binding.ID, stored.sourcePath, commitSHA); err != nil {
			return fmt.Errorf("failed to upsert pipeline '%s': %w", key, err)
		}
		var pipeline models.Pipeline
		if err := yaml.Unmarshal([]byte(stored.definition), &pipeline); err != nil {
			return fmt.Errorf("failed to parse synced pipeline '%s' for dashboard source bindings: %w", key, err)
		}
		if err := syncDashboardSourceBindingsForPipeline(ctx, tx, stored.path, stored.name, pipeline); err != nil {
			return fmt.Errorf("failed to sync dashboard source bindings for pipeline '%s': %w", key, err)
		}
		details["pipelines_synced"]++
	}

	// C. Upsert Steps
	for key, stored := range steps {
		writable, err := ensureConfigResourceWritable(ctx, tx, "steps", "reusable step", key, binding, key, "path = $1 AND name = $2", stored.path, stored.name)
		if err != nil {
			return err
		}
		if !writable {
			continue
		}
		if _, err := tx.Exec(ctx, stepUpsert, stored.path, stored.name, stored.definition, binding.ID, stored.sourcePath, commitSHA); err != nil {
			return fmt.Errorf("failed to upsert reusable step '%s': %w", key, err)
		}
		details["steps_synced"]++
	}

	// D. Upsert Schedules
	for key, stored := range schedules {
		writable, err := ensureConfigResourceWritable(ctx, tx, "pipeline_schedules", "schedule", key, binding, stored.input.Path, "path = $1 AND name = $2", stored.input.Path, stored.input.Name)
		if err != nil {
			return err
		}
		if !writable {
			continue
		}
		variablesJSON, err := json.Marshal(stored.input.Variables)
		if err != nil {
			return fmt.Errorf("failed to marshal schedule variables '%s': %w", key, err)
		}
		var scheduleID string
		if err := tx.QueryRow(ctx, configSyncScheduleUpsertSQL,
			stored.input.Path,
			stored.input.Name,
			stored.input.Description,
			stored.input.PipelinePath,
			stored.input.PipelineName,
			stored.input.PipelineVersion,
			stored.input.ScheduleKind,
			stored.input.CronExpression,
			stored.input.RunAt,
			stored.input.Timezone,
			stored.input.Enabled,
			stored.input.Scope,
			string(variablesJSON),
			stored.input.NextRunAt,
			scheduleDefaultVisibility,
			stored.input.RunTeamPath,
			binding.ID,
			stored.sourcePath,
			commitSHA,
		).Scan(&scheduleID); err != nil {
			return fmt.Errorf("failed to upsert schedule '%s': %w", key, err)
		}
		pipeline, err := loadSchedulePipelineFromSync(ctx, tx, stored.input, pipelines)
		if err != nil {
			return fmt.Errorf("failed to prepare schedule execution access '%s': %w", key, err)
		}
		if err := ensureScheduleExecutionACLs(ctx, tx, scheduleID, stored.input, pipeline); err != nil {
			return fmt.Errorf("failed to sync schedule execution access '%s': %w", key, err)
		}
		details["schedules_synced"]++
	}

	// E. Upsert Knowledge Contexts
	for key, stored := range knowledgeContexts {
		writable, err := ensureConfigResourceWritable(ctx, tx, "knowledge_contexts", "knowledge context", key, binding, stored.team, "kind = $1 AND team_path = $2 AND name = $3", stored.kind, stored.team, stored.name)
		if err != nil {
			return err
		}
		if !writable {
			continue
		}
		if _, err := tx.Exec(ctx, knowledgeContextUpsert, stored.kind, stored.team, stored.name, stored.description, stored.content, binding.ID, stored.sourcePath, commitSHA); err != nil {
			return fmt.Errorf("failed to upsert knowledge context '%s': %w", key, err)
		}
		details["knowledge_contexts_synced"]++
	}

	// F. Upsert General Scope Vars
	for key, stored := range generalScopeVars {
		scopeParam := runtimeScopeForStorage(key.scopePath)
		resourceID := fmt.Sprintf("scope=%s name=%s", runtimeScopeForDisplay(key.scopePath), key.name)
		writable, err := ensureConfigResourceWritable(ctx, tx, "variables", "variable", resourceID, binding, key.scopePath, "name = $1 AND repository_name IS NULL AND "+runtimeScopeEqualsSQL("scope", 2), key.name, scopeParam)
		if err != nil {
			return err
		}
		if !writable {
			continue
		}
		if _, err := tx.Exec(ctx, envUpsert, key.name, stored.value, nil, scopeParam, binding.ID, stored.sourcePath, commitSHA); err != nil {
			return fmt.Errorf("failed to upsert variable '%s' for scope '%s': %w", key.name, key.scopePath, err)
		}
		details["general_vars_synced"]++
	}

	// G. Upsert Repo Scope Vars
	for key, stored := range repoScopeVars {
		scopeParam := runtimeScopeForStorage(key.scopePath)
		resourceID := fmt.Sprintf("repo=%s scope=%s name=%s", key.repo, runtimeScopeForDisplay(key.scopePath), key.name)
		writable, err := ensureConfigResourceWritable(ctx, tx, "variables", "variable", resourceID, binding, key.repo, "name = $1 AND repository_name = $2 AND "+runtimeScopeEqualsSQL("scope", 3), key.name, key.repo, scopeParam)
		if err != nil {
			return err
		}
		if !writable {
			continue
		}
		if _, err := tx.Exec(ctx, envUpsert, key.name, stored.value, key.repo, scopeParam, binding.ID, stored.sourcePath, commitSHA); err != nil {
			return fmt.Errorf("failed to upsert repository variable '%s' for repo '%s' scope '%s': %w", key.name, key.repo, key.scopePath, err)
		}
		details["repo_vars_synced"]++
	}

	// H. Upsert Scope Secrets
	for key, stored := range generalScopeSecrets {
		scopeParam := runtimeScopeForStorage(key.scopePath)
		resourceID := fmt.Sprintf("scope=%s name=%s", runtimeScopeForDisplay(key.scopePath), key.name)
		writable, err := ensureConfigResourceWritable(ctx, tx, "secrets", "secret", resourceID, binding, key.scopePath, "name = $1 AND repository_name IS NULL AND "+runtimeScopeEqualsSQL("scope", 2), key.name, scopeParam)
		if err != nil {
			return err
		}
		if !writable {
			continue
		}
		var encryptedValue any
		if stored.encryptedValue != nil {
			encryptedValue = *stored.encryptedValue
		}
		if _, err := tx.Exec(ctx, secretUpsert, key.name, encryptedValue, nil, scopeParam, binding.ID, stored.sourcePath, commitSHA); err != nil {
			return fmt.Errorf("failed to upsert secret '%s' for scope '%s': %w", key.name, key.scopePath, err)
		}
		details["secrets_synced"]++
	}
	for key, stored := range repoScopeSecrets {
		scopeParam := runtimeScopeForStorage(key.scopePath)
		resourceID := fmt.Sprintf("repo=%s scope=%s name=%s", key.repo, runtimeScopeForDisplay(key.scopePath), key.name)
		writable, err := ensureConfigResourceWritable(ctx, tx, "secrets", "secret", resourceID, binding, key.repo, "name = $1 AND repository_name = $2 AND "+runtimeScopeEqualsSQL("scope", 3), key.name, key.repo, scopeParam)
		if err != nil {
			return err
		}
		if !writable {
			continue
		}
		var encryptedValue any
		if stored.encryptedValue != nil {
			encryptedValue = *stored.encryptedValue
		}
		if _, err := tx.Exec(ctx, secretUpsert, key.name, encryptedValue, key.repo, scopeParam, binding.ID, stored.sourcePath, commitSHA); err != nil {
			return fmt.Errorf("failed to upsert repository secret '%s' for repo '%s' scope '%s': %w", key.name, key.repo, key.scopePath, err)
		}
		details["secrets_synced"]++
	}

	// I. Upsert Git Webhook Sources
	for key, stored := range gitWebhookSources {
		resourceScope := effectiveGitWebhookSourceTeamPath(stored.input)
		writable, err := ensureConfigResourceWritable(
			ctx,
			tx,
			"git_webhook_sources",
			"git webhook source",
			key,
			binding,
			resourceScope,
			"id = $1",
			key,
		)
		if err != nil {
			return err
		}
		if !writable {
			continue
		}
		allowlistJSON, err := json.Marshal(stored.input.RepositoryAllowlist)
		if err != nil {
			return fmt.Errorf("failed to marshal git webhook source allowlist %q: %w", key, err)
		}
		rateLimitJSON, err := json.Marshal(stored.input.RateLimit)
		if err != nil {
			return fmt.Errorf("failed to marshal git webhook source rate limit %q: %w", key, err)
		}
		if _, err := tx.Exec(ctx, gitWebhookSourceUpsert,
			stored.input.ID,
			stored.input.Name,
			stored.input.Description,
			stored.input.Provider,
			stored.input.Enabled,
			stored.input.TeamPath,
			stored.input.Visibility,
			stored.input.AuthMode,
			stored.input.CredentialRef,
			string(allowlistJSON),
			string(rateLimitJSON),
			binding.ID,
			stored.sourcePath,
			commitSHA,
		); err != nil {
			return fmt.Errorf("failed to upsert git webhook source %q: %w", key, err)
		}
		details["git_webhook_sources_synced"]++
	}

	// J. Upsert Triggers
	for repoName, stored := range triggers {
		writable, err := ensureConfigResourceWritable(ctx, tx, "triggers", "trigger", repoName, binding, repositoryTriggerConfigScope(stored.record), "repository_name = $1", repoName)
		if err != nil {
			return err
		}
		if !writable {
			continue
		}
		if err := validateRepositoryTriggerWebhookSource(ctx, tx, stored.record); err != nil {
			return fmt.Errorf("invalid trigger override '%s': %w", repoName, err)
		}
		if _, err := tx.Exec(ctx, triggerUpsert,
			repoName,
			stored.definition,
			stored.record.Visibility,
			stored.record.Provider,
			stored.record.TeamPath,
			stored.record.Management,
			stored.record.WebhookSourceID,
			binding.ID,
			stored.sourcePath,
			commitSHA,
		); err != nil {
			return fmt.Errorf("failed to upsert trigger override '%s': %w", repoName, err)
		}
		details["triggers_synced"]++
	}

	// K. Upsert External Triggers
	for key, stored := range externalTriggers {
		resourceScope := externalTriggerConfigScope(stored.input)
		writable, err := ensureConfigResourceWritable(ctx, tx, "external_triggers", "external trigger", key, binding, resourceScope, "id = $1", key)
		if err != nil {
			return err
		}
		if !writable {
			continue
		}
		pipelinePath, pipelineName, _, err := configsync.SplitPipelineIdentifier(stored.input.Pipeline)
		if err != nil {
			return fmt.Errorf("invalid external trigger pipeline '%s': %w", key, err)
		}
		var exists int
		if err := tx.QueryRow(ctx, `SELECT 1 FROM pipelines WHERE path = $1 AND name = $2 LIMIT 1`, pipelinePath, pipelineName).Scan(&exists); err != nil {
			if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("external trigger '%s' references missing pipeline '%s'", key, stored.input.Pipeline)
			}
			return fmt.Errorf("failed to validate external trigger pipeline '%s': %w", key, err)
		}
		allowedJSON, err := json.Marshal(stored.input.AllowedCallers)
		if err != nil {
			return fmt.Errorf("failed to marshal external trigger callers '%s': %w", key, err)
		}
		mappingJSON, err := json.Marshal(stored.input.VariableMapping)
		if err != nil {
			return fmt.Errorf("failed to marshal external trigger variable mapping '%s': %w", key, err)
		}
		schemaJSON, err := json.Marshal(stored.input.PayloadSchema)
		if err != nil {
			return fmt.Errorf("failed to marshal external trigger payload schema '%s': %w", key, err)
		}
		rateLimitJSON, err := json.Marshal(stored.input.RateLimit)
		if err != nil {
			return fmt.Errorf("failed to marshal external trigger rate limit '%s': %w", key, err)
		}
		if _, err := tx.Exec(ctx, externalTriggerUpsert,
			stored.input.ID,
			stored.input.Name,
			stored.input.Description,
			stored.input.Enabled,
			stored.input.Pipeline,
			stored.input.Scope,
			stored.input.RunTeamPath,
			string(allowedJSON),
			string(mappingJSON),
			string(schemaJSON),
			string(rateLimitJSON),
			binding.ID,
			stored.sourcePath,
			commitSHA,
		); err != nil {
			return fmt.Errorf("failed to upsert external trigger '%s': %w", key, err)
		}
		details["external_triggers_synced"]++
	}

	// --- PRUNING PHASE: Remove items that exist in DB as source='git' but were not in the Git payload ---

	// 0. Prune Config Repository Bindings
	if binding.ScopeType == models.ConfigRepositoryScopeSystem {
		var scopeTypes, scopeIDs []string
		for _, cfgRepo := range configRepositories {
			scopeTypes = append(scopeTypes, cfgRepo.scopeType)
			scopeIDs = append(scopeIDs, cfgRepo.scopeID)
		}
		prunedRepoIDs := []int64{}
		if len(scopeTypes) == 0 {
			rows, err := tx.Query(ctx, "SELECT id FROM config_repositories WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID)
			if err != nil {
				return fmt.Errorf("failed to prune config repository bindings: %w", err)
			}
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return fmt.Errorf("failed to scan pruned config repository binding: %w", err)
				}
				prunedRepoIDs = append(prunedRepoIDs, id)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return fmt.Errorf("failed to read pruned config repository bindings: %w", err)
			}
			rows.Close()
		} else {
			rows, err := tx.Query(ctx, `
				SELECT id FROM config_repositories
				WHERE managed_by_config_repo = TRUE
				AND config_repo_id = $3
				AND NOT EXISTS (
					SELECT 1 FROM unnest($1::text[], $2::text[]) AS t(scope_type, scope_id)
					WHERE config_repositories.scope_type = t.scope_type
					AND config_repositories.scope_id = t.scope_id
				)`, scopeTypes, scopeIDs, binding.ID)
			if err != nil {
				return fmt.Errorf("failed to prune config repository bindings: %w", err)
			}
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return fmt.Errorf("failed to scan pruned config repository binding: %w", err)
				}
				prunedRepoIDs = append(prunedRepoIDs, id)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return fmt.Errorf("failed to read pruned config repository bindings: %w", err)
			}
			rows.Close()
		}
		if len(prunedRepoIDs) > 0 {
			for _, tableName := range []string{"config_repositories", "pipelines", "steps", "pipeline_schedules", "data_cleanup_schedules", "dashboards", "dashboard_refresh_schedules", "triggers", "external_triggers", "git_webhook_sources", "variables", "secrets", "knowledge_contexts", "agent_profiles", "notification_routes", "notification_mail_settings", "runtime_settings", "credentials"} {
				if _, err := tx.Exec(ctx, fmt.Sprintf(`
					UPDATE %s
					SET config_repo_id = NULL,
						config_source_path = '',
						config_source_commit_sha = '',
						managed_by_config_repo = FALSE
					WHERE config_repo_id = ANY($1)
				`, tableName), prunedRepoIDs); err != nil {
					return fmt.Errorf("failed to detach resources from pruned config repository bindings: %w", err)
				}
			}
			if _, err := tx.Exec(ctx, "DELETE FROM config_repositories WHERE id = ANY($1)", prunedRepoIDs); err != nil {
				return fmt.Errorf("failed to prune config repository bindings: %w", err)
			}
		}
	}

	// 1. Prune Pipelines
	{
		var paths, names []string
		for _, p := range pipelines {
			paths = append(paths, p.path)
			names = append(names, p.name)
		}
		if len(paths) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM pipelines WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID); err != nil {
				return fmt.Errorf("failed to prune pipelines: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, `
				DELETE FROM pipelines 
				WHERE managed_by_config_repo = TRUE
				AND config_repo_id = $3
				AND NOT EXISTS (
					SELECT 1 FROM unnest($1::text[], $2::text[]) AS t(p, n) 
					WHERE pipelines.path = t.p AND pipelines.name = t.n
				)`, paths, names, binding.ID); err != nil {
				return fmt.Errorf("failed to prune pipelines: %w", err)
			}
		}
	}

	// 2. Prune Steps
	{
		var paths, names []string
		for _, s := range steps {
			paths = append(paths, s.path)
			names = append(names, s.name)
		}
		if len(paths) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM steps WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID); err != nil {
				return fmt.Errorf("failed to prune steps: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, `
				DELETE FROM steps 
				WHERE managed_by_config_repo = TRUE
				AND config_repo_id = $3
				AND NOT EXISTS (
					SELECT 1 FROM unnest($1::text[], $2::text[]) AS t(p, n) 
					WHERE steps.path = t.p AND steps.name = t.n
				)`, paths, names, binding.ID); err != nil {
				return fmt.Errorf("failed to prune steps: %w", err)
			}
		}
	}

	// 3. Prune Schedules
	{
		var paths, names []string
		for _, s := range schedules {
			paths = append(paths, s.input.Path)
			names = append(names, s.input.Name)
		}
		if len(paths) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM pipeline_schedules WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID); err != nil {
				return fmt.Errorf("failed to prune schedules: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, `
				DELETE FROM pipeline_schedules
				WHERE managed_by_config_repo = TRUE
				AND config_repo_id = $3
				AND NOT EXISTS (
					SELECT 1 FROM unnest($1::text[], $2::text[]) AS t(p, n)
					WHERE pipeline_schedules.path = t.p AND pipeline_schedules.name = t.n
				)`, paths, names, binding.ID); err != nil {
				return fmt.Errorf("failed to prune schedules: %w", err)
			}
		}
	}

	// 4. Prune Dashboards
	{
		var teamIDs []int
		var slugs []string
		if len(dashboards) > 0 {
			teamRecords, err := loadTeamPathRecords(ctx, tx)
			if err != nil {
				return fmt.Errorf("failed to resolve dashboard teams for pruning: %w", err)
			}
			teamIDs, slugs, err = dashboardPruneTargetsFromTeamRecords(dashboards, teamRecords)
			if err != nil {
				return fmt.Errorf("failed to resolve dashboard teams for pruning: %w", err)
			}
		}
		if len(teamIDs) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM dashboards WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID); err != nil {
				return fmt.Errorf("failed to prune dashboards: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, `
				DELETE FROM dashboards d
				WHERE d.managed_by_config_repo = TRUE
				  AND d.config_repo_id = $3
				  AND NOT EXISTS (
					SELECT 1
					FROM unnest($1::int[], $2::text[]) AS wanted(team_id, slug)
					WHERE d.team_id = wanted.team_id AND d.slug = wanted.slug
				  )
			`, teamIDs, slugs, binding.ID); err != nil {
				return fmt.Errorf("failed to prune dashboards: %w", err)
			}
		}
	}

	// 5. Prune Knowledge Contexts
	{
		var kinds, teams, names []string
		for _, knowledge := range knowledgeContexts {
			kinds = append(kinds, knowledge.kind)
			teams = append(teams, knowledge.team)
			names = append(names, knowledge.name)
		}
		if len(kinds) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM knowledge_contexts WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID); err != nil {
				return fmt.Errorf("failed to prune knowledge contexts: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, `
				DELETE FROM knowledge_contexts
				WHERE managed_by_config_repo = TRUE
				AND config_repo_id = $4
				AND NOT EXISTS (
					SELECT 1 FROM unnest($1::text[], $2::text[], $3::text[]) AS t(k, g, n)
					WHERE knowledge_contexts.kind = t.k
					  AND knowledge_contexts.team_path = t.g
					  AND knowledge_contexts.name = t.n
				)`, kinds, teams, names, binding.ID); err != nil {
				return fmt.Errorf("failed to prune knowledge contexts: %w", err)
			}
		}
	}

	// 5. Prune Triggers
	{
		var repos []string
		for repo := range triggers {
			repos = append(repos, repo)
		}
		if len(repos) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM triggers WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID); err != nil {
				return fmt.Errorf("failed to prune triggers: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, "DELETE FROM triggers WHERE managed_by_config_repo = TRUE AND config_repo_id = $2 AND repository_name != ALL($1)", repos, binding.ID); err != nil {
				return fmt.Errorf("failed to prune triggers: %w", err)
			}
		}
	}

	// 6. Prune External Triggers
	{
		var ids []string
		for id := range externalTriggers {
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM external_triggers WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID); err != nil {
				return fmt.Errorf("failed to prune external triggers: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, "DELETE FROM external_triggers WHERE managed_by_config_repo = TRUE AND config_repo_id = $2 AND id != ALL($1)", ids, binding.ID); err != nil {
				return fmt.Errorf("failed to prune external triggers: %w", err)
			}
		}
	}

	// 6b. Prune Git Webhook Sources
	{
		var ids []string
		for id := range gitWebhookSources {
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM git_webhook_sources WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID); err != nil {
				return fmt.Errorf("failed to prune git webhook sources: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, "DELETE FROM git_webhook_sources WHERE managed_by_config_repo = TRUE AND config_repo_id = $2 AND id != ALL($1)", ids, binding.ID); err != nil {
				return fmt.Errorf("failed to prune git webhook sources: %w", err)
			}
		}
	}

	// 7. Prune Variables (Scope Variables)
	{
		var names []string
		var repos []*string
		var scopes []*string

		// Helper to collect all valid (name, repo, scope) tuples
		addVar := func(n string, r *string, s string) {
			names = append(names, n)
			repos = append(repos, r)
			storedScope := runtimeScopeForStorage(s)
			scopes = append(scopes, &storedScope)
		}

		for key := range generalScopeVars {
			addVar(key.name, nil, key.scopePath)
		}
		for key := range repoScopeVars {
			r := key.repo // copy loop variable
			addVar(key.name, &r, key.scopePath)
		}

		if len(names) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM variables WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID); err != nil {
				return fmt.Errorf("failed to prune variables: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, `
				DELETE FROM variables 
				WHERE managed_by_config_repo = TRUE
				AND config_repo_id = $4
				AND NOT EXISTS (
					SELECT 1 FROM unnest($1::text[], $2::text[], $3::text[]) AS t(n, r, s) 
					WHERE variables.name = t.n 
					AND variables.repository_name IS NOT DISTINCT FROM t.r 
					AND variables.scope IS NOT DISTINCT FROM t.s
				)`, names, repos, scopes, binding.ID); err != nil {
				return fmt.Errorf("failed to prune variables: %w", err)
			}
		}
	}

	// 8. Prune Secrets (Scope Secrets)
	{
		var names []string
		var repos []*string
		var scopes []*string

		addSecret := func(n string, r *string, s string) {
			names = append(names, n)
			repos = append(repos, r)
			storedScope := runtimeScopeForStorage(s)
			scopes = append(scopes, &storedScope)
		}

		for key := range generalScopeSecrets {
			addSecret(key.name, nil, key.scopePath)
		}
		for key := range repoScopeSecrets {
			r := key.repo
			addSecret(key.name, &r, key.scopePath)
		}

		if len(names) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM secrets WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID); err != nil {
				return fmt.Errorf("failed to prune secrets: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, `
				DELETE FROM secrets
				WHERE managed_by_config_repo = TRUE
				AND config_repo_id = $4
				AND NOT EXISTS (
					SELECT 1 FROM unnest($1::text[], $2::text[], $3::text[]) AS t(n, r, s)
					WHERE secrets.name = t.n
					AND secrets.repository_name IS NOT DISTINCT FROM t.r
					AND secrets.scope IS NOT DISTINCT FROM t.s
				)`, names, repos, scopes, binding.ID); err != nil {
				return fmt.Errorf("failed to prune secrets: %w", err)
			}
		}
	}

	// Sync UI teams. Teams do not have a source column, so we do not prune them to avoid deleting user-created teams.
	if len(effectivePipelineRunStructure) > 0 {
		if err := a.syncPipelineRunTeams(ctx, tx, effectivePipelineRunStructure, details); err != nil {
			return err
		}
	}

	for _, stored := range sortedNotificationRoutes(notificationRoutes) {
		teamID, err := notificationRouteTeamIDForPath(ctx, tx, stored.teamPath)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("notification route '%s' references missing team '%s'", stored.sourcePath, stored.teamPath)
			}
			return fmt.Errorf("failed to resolve notification route team '%s': %w", stored.teamPath, err)
		}
		definitionJSON, err := json.Marshal(stored.definition)
		if err != nil {
			return fmt.Errorf("failed to marshal notification route '%s': %w", stored.teamPath, err)
		}
		if _, err := tx.Exec(ctx, notificationRouteUpsert, teamID, string(definitionJSON), binding.ID, stored.sourcePath, commitSHA); err != nil {
			return fmt.Errorf("failed to upsert notification route '%s': %w", stored.teamPath, err)
		}
		details["notification_routes_synced"]++
	}
	{
		var teamIDs []int
		for _, stored := range notificationRoutes {
			teamID, err := notificationRouteTeamIDForPath(ctx, tx, stored.teamPath)
			if err != nil {
				return fmt.Errorf("failed to resolve notification route team '%s' for pruning: %w", stored.teamPath, err)
			}
			teamIDs = append(teamIDs, teamID)
		}
		if len(teamIDs) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM notification_routes WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID); err != nil {
				return fmt.Errorf("failed to prune notification routes: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, "DELETE FROM notification_routes WHERE managed_by_config_repo = TRUE AND config_repo_id = $2 AND team_id != ALL($1)", teamIDs, binding.ID); err != nil {
				return fmt.Errorf("failed to prune notification routes: %w", err)
			}
		}
	}
	if mailSettingsPlan != nil {
		if _, err := scanNotificationMailSettings(tx.QueryRow(ctx, notificationMailSettingsUpsertSQL,
			mailSettingsPlan.settings.Enabled,
			mailSettingsPlan.settings.From,
			mailSettingsPlan.settings.SMTP.Host,
			mailSettingsPlan.settings.SMTP.Port,
			mailSettingsPlan.settings.SMTP.StartTLS,
			mailSettingsPlan.settings.SMTP.Username,
			mailSettingsPlan.settings.SMTP.PasswordCredentialRef,
			"git",
			binding.ID,
			mailSettingsPlan.sourcePath,
			commitSHA,
			true,
		)); err != nil {
			return fmt.Errorf("failed to sync mail settings from '%s': %w", mailSettingsPlan.sourcePath, err)
		}
		details["mail_settings_synced"] = 1
	}
	if dataManagementPlan != nil {
		for _, stored := range sortedDataCleanupSchedules(dataManagementPlan.schedules) {
			if err := upsertDataCleanupScheduleFromGitOps(ctx, tx, binding, stored, commitSHA); err != nil {
				return fmt.Errorf("failed to sync data cleanup schedule %q from '%s': %w", stored.input.Name, stored.sourcePath, err)
			}
			details["data_cleanup_schedules_synced"]++
		}
		if err := pruneGitOpsDataCleanupSchedules(ctx, tx, binding, dataManagementPlan.schedules); err != nil {
			return err
		}
	}
	if credentialPlan != nil {
		if err := syncCredentialsFromGitOps(ctx, tx, binding, credentialPlan, commitSHA); err != nil {
			return fmt.Errorf("failed to sync credentials from '%s': %w", credentialPlan.sourcePath, err)
		}
		details["credentials_synced"] = len(credentialPlan.credentials)
	}

	if err := a.syncAccessConfiguration(ctx, tx, binding, accessPlan, commitSHA, details); err != nil {
		return err
	}
	if teamAIProfilePlan != nil {
		if err := a.persistTeamAIProfilesToTx(ctx, tx, binding, teamAIProfilePlan, commitSHA); err != nil {
			return fmt.Errorf("failed to sync team AI profiles from '%s': %w", teamAIProfilePlan.sourcePath, err)
		}
		details["team_llm_profiles_synced"] = len(teamAIProfilePlan.llmProfiles)
		details["team_agent_profiles_synced"] = len(teamAIProfilePlan.agentProfiles)
		details["team_mcp_profiles_synced"] = len(teamAIProfilePlan.mcpProfiles)
	}
	if llmProfilePlan != nil {
		if err := persistLLMProfilesToTx(ctx, tx, llmProfilePlan.defaultProfile, llmProfilePlan.profiles); err != nil {
			return fmt.Errorf("failed to sync LLM profiles from '%s': %w", llmProfilePlan.sourcePath, err)
		}
		details["llm_profiles_synced"] = len(llmProfilePlan.profiles)
	}
	if agentProfilePlan != nil {
		if err := persistGitOpsAgentProfilesToTx(ctx, tx, agentProfilePlan, binding.ID, commitSHA); err != nil {
			return fmt.Errorf("failed to sync agent profiles from '%s': %w", agentProfilePlan.sourcePath, err)
		}
		details["agent_profiles_synced"] = len(agentProfilePlan.profiles)
	}
	if mcpRegistryPlan != nil {
		if err := persistMCPRegistryToTx(ctx, tx, mcpRegistryPlan.Servers, mcpRegistryPlan.Profiles); err != nil {
			return fmt.Errorf("failed to sync MCP registry from '%s': %w", mcpRegistryPlan.SourcePath, err)
		}
		details["mcp_servers_synced"] = len(mcpRegistryPlan.Servers)
		details["mcp_profiles_synced"] = len(mcpRegistryPlan.Profiles)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit configuration synchronization transaction: %w", err)
	}
	if authSettingsPlan != nil {
		if err := a.applyAuthSettingsGitOpsPlan(ctx, authSettingsPlan); err != nil {
			return fmt.Errorf("failed to sync auth settings from '%s': %w", authSettingsPlan.sourcePath, err)
		}
		details["auth_settings_synced"] = 1
	}
	if runtimeSettingsPlan != nil || githubSettingsPlan != nil {
		if err := a.applySystemSettingsGitOpsPlans(ctx, binding, runtimeSettingsPlan, githubSettingsPlan, commitSHA); err != nil {
			return fmt.Errorf("failed to sync system settings: %w", err)
		}
		if runtimeSettingsPlan != nil {
			details["runtime_settings_synced"] = 1
		}
		if githubSettingsPlan != nil {
			details["github_settings_synced"] = 1
		}
	}
	if llmProfilePlan != nil {
		a.setLLMProfiles(llmProfilePlan.defaultProfile, llmProfilePlan.profiles)
	}
	if mcpRegistryPlan != nil {
		a.setMCPRegistry(mcpRegistryPlan.Servers, mcpRegistryPlan.Profiles)
	}
	return nil
}

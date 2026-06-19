package nopsai

import (
	"context"
	"sort"
	"strings"

	aaamodel "nopsai/services/aaa/pkg/model"
)

type hostedMCPFeatureCapability struct {
	Area        string
	Features    []string
	Coverage    string
	Mode        string
	Tools       []string
	Resources   []string
	APIRoutes   []string
	Permissions []hostedMCPPermission
	Notes       []string
}

func (a *App) hostedMCPGetFeatureCapabilities(ctx context.Context, subject aaamodel.Subject, args map[string]any) (map[string]any, error) {
	areaFilter := strings.ToLower(strings.TrimSpace(stringArg(args, "area")))
	query := strings.ToLower(strings.TrimSpace(stringArg(args, "query")))
	includeAPIRoutes := boolArg(args, "include_api_routes", true)
	areaFilter, query = hostedMCPNormalizeFeatureCapabilityFilters(areaFilter, query)

	availableTools := hostedMCPToolNames(a.hostedMCPToolsForSubject(ctx, subject))
	availableToolSet := hostedMCPNameSet(availableTools)
	availableResources := hostedMCPResourceURIs(a.hostedMCPResourcesForSubject(ctx, subject))
	availableResourceSet := hostedMCPNameSet(availableResources)

	areas := []map[string]any{}
	featureCount := 0
	for _, capability := range hostedMCPFeatureCapabilityCatalog() {
		if areaFilter != "" && !strings.Contains(strings.ToLower(capability.Area), areaFilter) {
			continue
		}
		if query != "" && !hostedMCPFeatureCapabilityMatches(capability, query) {
			continue
		}
		featureCount += len(capability.Features)
		areas = append(areas, a.hostedMCPFeatureCapabilityResponse(ctx, subject, capability, availableToolSet, availableResourceSet, includeAPIRoutes))
	}

	return map[string]any{
		"permission_model": map[string]any{
			"mode":                                  "current_authenticated_subject",
			"assistant_and_mcp_share_subject":       true,
			"tools_list_filtered_by_aaa":            true,
			"resources_list_filtered_by_aaa":        true,
			"tool_calls_recheck_specific_resources": true,
			"scoped_product_grants_supported":       true,
			"write_contract":                        "First-class hosted MCP write tools return proposals unless the tool explicitly documents a direct mutating operation.",
			"secret_contract":                       "Secret and credential domains should use metadata, encrypted GitOps payloads, or explicit write flows; plaintext secret retrieval is not exposed as a default assistant context.",
			"api_bridge":                            "nopsai.call_api can call guarded /v1 REST API routes as the current subject; mutating calls require confirm:true and route/resource AAA checks still apply.",
		},
		"subject":             hostedMCPSubjectSummary(subject),
		"available_tools":     availableTools,
		"available_resources": availableResources,
		"areas":               areas,
		"area_count":          len(areas),
		"feature_count":       featureCount,
	}, nil
}

func (a *App) hostedMCPFeatureCapabilityResponse(ctx context.Context, subject aaamodel.Subject, capability hostedMCPFeatureCapability, availableToolSet, availableResourceSet map[string]struct{}, includeAPIRoutes bool) map[string]any {
	availableTools, missingTools := hostedMCPPartitionNames(capability.Tools, availableToolSet)
	availableResources, missingResources := hostedMCPPartitionNames(capability.Resources, availableResourceSet)
	permissionChecks := a.hostedMCPFeaturePermissionChecks(ctx, subject, capability.Permissions)

	response := map[string]any{
		"area":      capability.Area,
		"features":  capability.Features,
		"coverage":  capability.Coverage,
		"mode":      capability.Mode,
		"tools":     capability.Tools,
		"resources": capability.Resources,
		"user_access": map[string]any{
			"tools":               hostedMCPAccessState(len(availableTools), len(capability.Tools)),
			"resources":           hostedMCPAccessState(len(availableResources), len(capability.Resources)),
			"permissions":         hostedMCPPermissionAccessState(permissionChecks),
			"available_tools":     availableTools,
			"missing_tools":       missingTools,
			"available_resources": availableResources,
			"missing_resources":   missingResources,
			"permission_checks":   permissionChecks,
		},
		"notes": capability.Notes,
	}
	if includeAPIRoutes {
		response["api_routes"] = capability.APIRoutes
	}
	return response
}

func (a *App) hostedMCPFeaturePermissionChecks(ctx context.Context, subject aaamodel.Subject, permissions []hostedMCPPermission) []map[string]any {
	checks := make([]map[string]any, 0, len(permissions))
	for _, permission := range permissions {
		if !permission.valid() {
			continue
		}
		checks = append(checks, map[string]any{
			"action":        permission.Action,
			"resource_type": permission.Resource.Type,
			"resource_id":   permission.Resource.ID,
			"allowed":       a.hostedMCPAllowed(ctx, subject, permission),
		})
	}
	return checks
}

func hostedMCPFeatureCapabilityCatalog() []hostedMCPFeatureCapability {
	return []hostedMCPFeatureCapability{
		{
			Area: "First-install setup",
			Features: []string{
				"Setup status and health checks",
				"Public setup preflight",
				"Step-by-step setup modal",
				"Persistent setup reference page",
				"Local secret generation",
				"Global GitOps config repository creation",
				"Runtime variable output generation",
				"Starter GitOps template preview",
				"Starter database seeding",
				"Starter repository groups",
				"Starter users",
				"Setup guardrails",
			},
			Coverage: "first_class",
			Mode:     "guided setup status/preflight/templates/plan plus confirmed bootstrap",
			Tools: []string{
				"nopsai.call_api",
				"nopsai.get_system_status",
				"nopsai.get_setup_status",
				"nopsai.get_setup_preflight",
				"nopsai.get_setup_templates",
				"nopsai.plan_first_install_setup",
				"nopsai.bootstrap_first_install_setup",
			},
			Resources: []string{
				"nopsai://system/status",
			},
			APIRoutes: []string{
				"GET /v1/setup/status",
				"GET /v1/setup/preflight",
				"GET /v1/setup/templates",
				"GET /v1/setup/templates.zip",
				"POST /v1/setup/bootstrap",
			},
			Permissions: []hostedMCPPermission{
				hostedMCPReadPermission("system.read", "system", "config"),
				hostedMCPReadPermission("system.update", "system", "config"),
			},
			Notes: []string{
				"Public preflight remains public outside MCP; hosted MCP reports setup through the authenticated subject.",
				"Bootstrap writes are high-impact, confirmed, and auditable because they create initial enterprise state.",
			},
		},
		{
			Area: "Pipeline authoring",
			Features: []string{
				"YAML-defined pipelines",
				"Step-level images",
				"Required scope variables",
				"Step dependencies",
				"Multi-task steps",
				"Goal steps",
				"Script steps",
				"Approval steps",
				"Reusable step inclusion",
				"Child pipeline inclusion",
				"Conditional execution",
				"Docker volume mounting",
				"Variable overrides",
				"Secret declaration",
				"Ignored failures",
				"LLM content sharing controls",
				"Agent Profile selection",
				"Knowledge context references",
				"GitHub display options",
			},
			Coverage: "first_class",
			Mode:     "read/proposal; GitOps-ready pipeline create/update plans",
			Tools: []string{
				"nopsai.call_api",
				"nopsai.list_pipelines",
				"nopsai.search_pipelines",
				"nopsai.get_pipeline",
				"nopsai.get_pipeline_knowledge_context",
				"nopsai.generate_pipeline",
				"nopsai.validate_pipeline",
				"nopsai.propose_pipeline_create",
				"nopsai.propose_pipeline_update",
				"nopsai.propose_reusable_step_create",
				"nopsai.propose_reusable_step_update",
				"nopsai.propose_reusable_step_delete",
				"nopsai.suggest_design_improvements",
			},
			Resources: []string{
				"nopsai://pipelines",
				"nopsai://knowledge-contexts",
			},
			APIRoutes: []string{
				"GET /v1/pipelines",
				"GET /v1/pipelines/{pipelineName...}",
				"PUT /v1/pipelines/{pipelineName...}",
				"DELETE /v1/pipelines/{pipelineName...}",
				"GET /v1/steps",
				"GET /v1/steps/{stepPath...}",
				"PUT /v1/steps/{stepName...}",
				"DELETE /v1/steps/{stepName...}",
			},
			Permissions: []hostedMCPPermission{
				hostedMCPReadPermission("pipeline.list", "pipeline", "*"),
				hostedMCPReadPermission("pipeline.read", "pipeline", "*"),
				hostedMCPReadPermission("pipeline.create", "pipeline", "*"),
				hostedMCPReadPermission("pipeline.update", "pipeline", "*"),
				hostedMCPReadPermission("pipeline.delete", "pipeline", "*"),
				hostedMCPReadPermission("step.read", "step", "*"),
				hostedMCPReadPermission("step.create", "step", "*"),
				hostedMCPReadPermission("step.update", "step", "*"),
				hostedMCPReadPermission("step.delete", "step", "*"),
			},
			Notes: []string{
				"Pipeline MCP writes are proposal-only and return commit-ready GitOps file payloads.",
				"Reusable step create/update/delete tools validate step YAML and return GitOps file plans under steps/.",
			},
		},
		{
			Area: "Execution semantics and run operations",
			Features: []string{
				"Parallel execution",
				"Dependency-aware execution",
				"Shared workspace volume",
				"Reusable container session",
				"Child pipeline triggering",
				"Asynchronous child pipeline monitoring",
				"Parent/child run status aggregation",
				"Approval checkpoints",
				"Approval resume",
				"Pipeline timeout handling",
				"Secret masking",
				"Knowledge context run snapshots",
				"Run creation",
				"Run listing",
				"Group/folder run listing",
				"Root run listing",
				"Run details",
				"Run status",
				"Run approvals",
				"Approval approve/reject",
				"Run rerun",
				"Run cancel",
				"Run delete",
				"Branch run delete",
				"Log ingestion",
				"Task status update",
				"Run finalization",
				"GitHub check run lookup",
			},
			Coverage: "partial",
			Mode:     "read/analysis and user run mutations first-class; internal service mutations excluded",
			Tools: []string{
				"nopsai.call_api",
				"nopsai.run_pipeline",
				"nopsai.list_pipeline_runs",
				"nopsai.get_pipeline_run",
				"nopsai.get_pipeline_run_logs",
				"nopsai.analyze_pipeline_run_failure",
				"nopsai.list_run_approvals",
				"nopsai.approve_run_approval",
				"nopsai.reject_run_approval",
				"nopsai.rerun_pipeline_run",
				"nopsai.cancel_pipeline_run",
				"nopsai.delete_pipeline_run",
				"nopsai.explain_internal_run_operations",
				"nopsai.list_lab_items",
				"nopsai.get_lab_item",
				"nopsai.explain_lab_result",
			},
			Resources: []string{
				"nopsai://pipeline-runs",
				"nopsai://lab",
			},
			APIRoutes: []string{
				"POST /v1/run",
				"POST /v1/run/{pipelineName...}",
				"GET /v1/runs",
				"GET /v1/runs/{runID}",
				"GET /v1/runs/{runID}/status",
				"GET /v1/runs/{runID}/logs",
				"GET /v1/runs/{runID}/approvals",
				"POST /v1/runs/{runID}/approvals/{approvalID}/approve",
				"POST /v1/runs/{runID}/approvals/{approvalID}/reject",
				"POST /v1/runs/{runID}/rerun",
				"POST /v1/runs/{runID}/cancel",
				"DELETE /v1/runs/{runID}",
				"DELETE /v1/repositories/{repoOwner}/{repoName}/branches/{branch...}",
				"GET /v1/runs-by-check/{checkRunID}",
			},
			Permissions: []hostedMCPPermission{
				hostedMCPReadPermission("pipeline.execute", "pipeline", "*"),
				hostedMCPReadPermission("pipeline_run.list", "pipeline_run", "*"),
				hostedMCPReadPermission("pipeline_run.read", "pipeline_run", "*"),
				hostedMCPReadPermission("pipeline_run.read_logs", "pipeline_run", "*"),
				hostedMCPReadPermission("approval.approve", "folder", "*"),
				hostedMCPReadPermission("pipeline_run.rerun", "pipeline_run", "*"),
				hostedMCPReadPermission("pipeline_run.cancel", "pipeline_run", "*"),
				hostedMCPReadPermission("pipeline_run.delete", "pipeline_run", "*"),
			},
			Notes: []string{
				"Dispatcher/internal log ingestion, task update, and finalization are service paths, not normal assistant tools.",
				"Run creation, approval decisions, rerun, cancel, and delete require explicit confirmation before execution.",
			},
		},
		{
			Area: "Pipeline scheduling",
			Features: []string{
				"Schedule resources",
				"Cron schedules",
				"One-time schedules",
				"Timezone scheduling",
				"Schedule enable/disable",
				"Schedule run now",
				"Schedule edit/delete",
				"Schedule-owned service account",
				"Schedule run group routing",
			},
			Coverage: "first_class",
			Mode:     "read and GitOps-ready write plans first-class; run-now requires confirmation",
			Tools: []string{
				"nopsai.call_api",
				"nopsai.list_schedules",
				"nopsai.get_schedule",
				"nopsai.propose_schedule_change",
				"nopsai.propose_schedule_create",
				"nopsai.propose_schedule_update",
				"nopsai.propose_schedule_delete",
				"nopsai.propose_schedule_enable",
				"nopsai.propose_schedule_disable",
				"nopsai.run_schedule_now",
			},
			Resources: []string{
				"nopsai://schedules",
			},
			APIRoutes: []string{
				"GET /v1/schedules",
				"POST /v1/schedules",
				"GET /v1/schedules/{scheduleID}",
				"PUT /v1/schedules/{scheduleID}",
				"PATCH /v1/schedules/{scheduleID}",
				"DELETE /v1/schedules/{scheduleID}",
				"POST /v1/schedules/{scheduleID}/enable",
				"POST /v1/schedules/{scheduleID}/disable",
				"POST /v1/schedules/{scheduleID}/run",
			},
			Permissions: []hostedMCPPermission{
				hostedMCPReadPermission("pipeline_schedule.list", "pipeline_schedule", "*"),
				hostedMCPReadPermission("pipeline_schedule.read", "pipeline_schedule", "*"),
				hostedMCPReadPermission("pipeline_schedule.create", "pipeline_schedule", "*"),
				hostedMCPReadPermission("pipeline_schedule.update", "pipeline_schedule", "*"),
				hostedMCPReadPermission("pipeline_schedule.delete", "pipeline_schedule", "*"),
				hostedMCPReadPermission("pipeline.execute", "pipeline", "*"),
			},
			Notes: []string{
				"Schedule create/update/delete/enable/disable tools return GitOps file plans; run-now is a confirmed runtime action.",
			},
		},
		{
			Area: "Knowledge context",
			Features: []string{
				"Managed knowledge documents",
				"Repo-local markdown documents",
				"Run-resolved context storage",
				"Knowledge context CRUD",
			},
			Coverage: "first_class",
			Mode:     "read/traversal and GitOps-ready write plans first-class",
			Tools: []string{
				"nopsai.call_api",
				"nopsai.search_docs",
				"nopsai.read_doc",
				"nopsai.list_knowledge_contexts",
				"nopsai.get_knowledge_context",
				"nopsai.get_pipeline_knowledge_context",
				"nopsai.propose_knowledge_context_create",
				"nopsai.propose_knowledge_context_update",
				"nopsai.propose_knowledge_context_delete",
			},
			Resources: []string{
				"nopsai://docs",
				"nopsai://knowledge-contexts",
			},
			APIRoutes: []string{
				"GET /v1/knowledge-contexts",
				"GET /v1/knowledge-contexts/{knowledgeID...}",
				"PUT /v1/knowledge-contexts/{knowledgeID...}",
				"DELETE /v1/knowledge-contexts/{knowledgeID...}",
			},
			Permissions: []hostedMCPPermission{
				hostedMCPReadPermission("knowledge_context.read", "knowledge_context", "*"),
				hostedMCPReadPermission("knowledge_context.create", "knowledge_context", "*"),
				hostedMCPReadPermission("knowledge_context.update", "knowledge_context", "*"),
				hostedMCPReadPermission("knowledge_context.delete", "knowledge_context", "*"),
			},
			Notes: []string{
				"Repo-local markdown is intentionally reported as run-time-only because it resolves from repository commit state.",
				"Managed knowledge context writes return GitOps file plans and do not mutate database state directly.",
			},
		},
		{
			Area: "Secrets, variables, and scopes",
			Features: []string{
				"Secrets",
				"Variables",
				"Scoped runs",
				"Scope CRUD",
				"Variable CRUD",
				"Secret CRUD",
			},
			Coverage: "first_class",
			Mode:     "scope reads, metadata, encryption, explicit writes, deletes, and scoped GitOps value plans first-class",
			Tools: []string{
				"nopsai.call_api",
				"nopsai.list_scopes",
				"nopsai.get_scope",
				"nopsai.explain_scope_permissions",
				"nopsai.list_secrets_metadata",
				"nopsai.list_secret_scopes",
				"nopsai.encrypt_secret_for_gitops",
				"nopsai.write_secret_value",
				"nopsai.delete_secret_value",
				"nopsai.propose_secret_gitops_write",
				"nopsai.propose_secret_gitops_delete",
				"nopsai.list_variables_metadata",
				"nopsai.list_variable_scopes",
				"nopsai.get_variable_value",
				"nopsai.write_variable_value",
				"nopsai.delete_variable_value",
				"nopsai.propose_variable_gitops_write",
				"nopsai.propose_variable_gitops_delete",
			},
			Resources: []string{
				"nopsai://scopes",
			},
			APIRoutes: []string{
				"GET /v1/secrets",
				"GET /v1/secrets/scopes",
				"POST /v1/secrets/encrypt",
				"GET /v1/secrets/{secretName}",
				"PUT /v1/secrets/{secretName}",
				"DELETE /v1/secrets/{secretName}",
				"GET /v1/repositories/{repoOwner}/{repoName}/secrets",
				"PUT /v1/repositories/{repoOwner}/{repoName}/secrets/{secretName}",
				"DELETE /v1/repositories/{repoOwner}/{repoName}/secrets/{secretName}",
				"GET /v1/variables",
				"GET /v1/variables/scopes",
				"GET /v1/variables/{variableName}",
				"PUT /v1/variables/{variableName}",
				"DELETE /v1/variables/{variableName}",
				"GET /v1/repositories/{repoOwner}/{repoName}/variables",
				"GET /v1/repositories/{repoOwner}/{repoName}/variables/{variableName}",
				"PUT /v1/repositories/{repoOwner}/{repoName}/variables/{variableName}",
				"DELETE /v1/repositories/{repoOwner}/{repoName}/variables/{variableName}",
			},
			Permissions: []hostedMCPPermission{
				hostedMCPReadPermission("scope.use", "scope", "*"),
				hostedMCPReadPermission("scope.update", "scope", "*"),
				hostedMCPReadPermission("scope.delete", "scope", "*"),
				hostedMCPReadPermission("secret.list_metadata", "secret", "*"),
				hostedMCPReadPermission("secret.read_value", "secret", "*"),
				hostedMCPReadPermission("secret.write_value", "secret", "*"),
				hostedMCPReadPermission("secret.delete", "secret", "*"),
				hostedMCPReadPermission("variable.list_metadata", "variable", "*"),
				hostedMCPReadPermission("variable.read_value", "variable", "*"),
				hostedMCPReadPermission("variable.write_value", "variable", "*"),
				hostedMCPReadPermission("variable.delete", "variable", "*"),
			},
			Notes: []string{
				"Plaintext secret reads remain blocked from default assistant context; use metadata, encryption, explicit writes, or GitOps plans.",
				"Secret/credential values and encrypted payloads are redacted from hosted MCP audit input/output summaries.",
			},
		},
		{
			Area: "Git providers, webhooks, and triggers",
			Features: []string{
				"GitHub webhook ingestion",
				"GitLab webhook ingestion",
				"Bitbucket webhook ingestion",
				"Gitea webhook ingestion",
				"Generic webhook ingestion",
				"Webhook authentication",
				"Repository allowlists",
				"Delivery idempotency",
				"Source rate limits",
				"Delivery audit records",
				"Trigger manifests",
				"Event matching",
				"Branch globs",
				"Tag globs",
				"Skipped branches",
				"Skipped repositories",
				"Changed-file include globs",
				"Changed-file exclude globs",
				"Pipeline lists",
				"Scope selection",
				"Repository trigger overrides",
				"Owner-wide trigger overrides",
				"GitHub check runs",
				"Check reruns",
				"Stale check cancellation",
				"Branch open-PR checks",
				"Repository access verification",
				"Config Git push",
				"GitOps-managed Git Webhook Sources",
				"External trigger run group routing",
				"Branch listing",
			},
			Coverage: "partial",
			Mode:     "trigger read/proposal plus webhook source and external trigger tools first-class; public ingress excluded",
			Tools: []string{
				"nopsai.call_api",
				"nopsai.list_triggers",
				"nopsai.get_trigger",
				"nopsai.propose_trigger_change",
				"nopsai.list_external_triggers",
				"nopsai.get_external_trigger",
				"nopsai.list_external_trigger_invocations",
				"nopsai.propose_external_trigger_create",
				"nopsai.propose_external_trigger_update",
				"nopsai.propose_external_trigger_delete",
				"nopsai.invoke_external_trigger",
				"nopsai.list_git_webhook_sources",
				"nopsai.get_git_webhook_source",
				"nopsai.list_git_webhook_deliveries",
				"nopsai.propose_git_webhook_source_create",
				"nopsai.propose_git_webhook_source_update",
				"nopsai.propose_git_webhook_source_delete",
				"nopsai.explain_webhook_ingress_policy",
			},
			Resources: []string{
				"nopsai://triggers",
			},
			APIRoutes: []string{
				"GET /v1/overrides",
				"GET /v1/overrides/{repoOwner}/{repoName}",
				"PUT /v1/overrides/{repoOwner}/{repoName}",
				"DELETE /v1/overrides/{repoOwner}/{repoName}",
				"GET /v1/external-triggers",
				"POST /v1/external-triggers",
				"GET /v1/external-triggers/{id}",
				"PUT /v1/external-triggers/{id}",
				"PATCH /v1/external-triggers/{id}",
				"DELETE /v1/external-triggers/{id}",
				"GET /v1/external-triggers/{id}/invocations",
				"POST /v1/external-triggers/{id}/invoke",
				"GET /v1/git-webhook-sources",
				"POST /v1/git-webhook-sources",
				"GET /v1/git-webhook-sources/{sourceID}",
				"PUT /v1/git-webhook-sources/{sourceID}",
				"PATCH /v1/git-webhook-sources/{sourceID}",
				"DELETE /v1/git-webhook-sources/{sourceID}",
				"GET /v1/git-webhook-sources/{sourceID}/deliveries",
				"GET /v1/repositories/{repoOwner}/{repoName}/branches",
			},
			Permissions: []hostedMCPPermission{
				hostedMCPReadPermission("trigger.read", "trigger", "*"),
				hostedMCPReadPermission("trigger.update", "trigger", "*"),
				hostedMCPReadPermission("trigger.delete", "trigger", "*"),
				hostedMCPReadPermission("external_trigger.read", "external_trigger", "*"),
				hostedMCPReadPermission("external_trigger.create", "external_trigger", "*"),
				hostedMCPReadPermission("external_trigger.update", "external_trigger", "*"),
				hostedMCPReadPermission("external_trigger.delete", "external_trigger", "*"),
				hostedMCPReadPermission("git_webhook_source.read", "git_webhook_source", "*"),
				hostedMCPReadPermission("git_webhook_source.create", "git_webhook_source", "*"),
				hostedMCPReadPermission("git_webhook_source.update", "git_webhook_source", "*"),
				hostedMCPReadPermission("git_webhook_source.delete", "git_webhook_source", "*"),
				hostedMCPReadPermission("repository.read", "repository", "*"),
			},
			Notes: []string{
				"Webhook delivery ingestion remains public/provider-facing and should not be exposed as a general assistant mutation.",
				"Webhook source and external trigger writes return GitOps file plans; external trigger invocation requires confirmation.",
				"The webhook ingress policy tool gives assistants an auditable way to explain this boundary.",
			},
		},
		{
			Area: "Configuration sync and GitOps",
			Features: []string{
				"Pipeline sync",
				"Reusable step sync",
				"Schedule sync",
				"Trigger sync",
				"Git webhook source sync",
				"Scope sync",
				"Knowledge sync",
				"Config repository sync",
				"Auth settings sync",
				"Mail settings sync",
				"LLM profile sync",
				"Agent profile sync",
				"MCP registry sync",
				"GitHub settings sync",
				"Runner settings sync",
				"Credential sync",
				"Config repository drift",
				"Config repository adoption",
				"Config sync trigger",
				"Config sync status",
			},
			Coverage: "first_class",
			Mode:     "GitOps write plans plus config repo sync/drift/write workflows first-class",
			Tools: []string{
				"nopsai.call_api",
				"nopsai.propose_pipeline_create",
				"nopsai.propose_pipeline_update",
				"nopsai.get_config_sync_status",
				"nopsai.sync_system_config",
				"nopsai.get_config_repo",
				"nopsai.get_config_repo_drift",
				"nopsai.sync_config_repo",
				"nopsai.write_config_repo",
				"nopsai.list_config_repos",
				"nopsai.sync_all_config_repos",
				"nopsai.get_system_status",
			},
			Resources: []string{
				"nopsai://system/status",
			},
			APIRoutes: []string{
				"GET /v1/system/config/sync",
				"POST /v1/system/config/sync",
				"GET /v1/system/config-repo",
				"PUT /v1/system/config-repo",
				"DELETE /v1/system/config-repo",
				"GET /v1/system/config-repo/sync",
				"POST /v1/system/config-repo/sync",
				"GET /v1/system/config-repo/drift",
				"POST /v1/system/config-repo/write",
				"GET /v1/system/config-repos",
				"POST /v1/system/config-repos/sync",
				"GET /v1/groups/{folderID}/config-repo",
				"PUT /v1/groups/{folderID}/config-repo",
				"DELETE /v1/groups/{folderID}/config-repo",
				"GET /v1/groups/{folderID}/config-repo/sync",
				"POST /v1/groups/{folderID}/config-repo/sync",
				"GET /v1/groups/{folderID}/config-repo/drift",
				"POST /v1/groups/{folderID}/config-repo/write",
			},
			Permissions: []hostedMCPPermission{
				hostedMCPReadPermission("system.read", "system", "config-sync"),
				hostedMCPReadPermission("system.update", "system", "config-sync"),
				hostedMCPReadPermission("system.read", "system", "config-repos"),
				hostedMCPReadPermission("system.update", "system", "config-repos"),
				hostedMCPReadPermission("config_repo.read", "folder", "*"),
				hostedMCPReadPermission("config_repo.sync", "folder", "*"),
				hostedMCPReadPermission("config_repo.manage", "folder", "*"),
			},
			Notes: []string{
				"Config repo sync and write tools use the existing GitOps API workflow and require explicit confirmation for mutating operations.",
			},
		},
		{
			Area: "Notifications",
			Features: []string{
				"Notification mail settings",
				"SMTP test",
				"Pipeline mail notifications",
				"Group notification routing",
				"GitOps notification routes",
				"Named notification routes",
				"Asynchronous mail delivery",
			},
			Coverage: "first_class",
			Mode:     "mail reads, GitOps proposals, route proposals, and SMTP test first-class",
			Tools: []string{
				"nopsai.call_api",
				"nopsai.get_notification_mail_settings",
				"nopsai.propose_notification_mail_settings",
				"nopsai.test_notification_mail_settings",
				"nopsai.get_notification_route",
				"nopsai.propose_notification_route_update",
				"nopsai.propose_notification_route_delete",
			},
			APIRoutes: []string{
				"GET /v1/system/notifications/mail",
				"PUT /v1/system/notifications/mail",
				"POST /v1/system/notifications/mail/test",
				"GET /v1/groups/{folderID}/notifications",
				"PUT /v1/groups/{folderID}/notifications",
				"DELETE /v1/groups/{folderID}/notifications",
			},
			Permissions: []hostedMCPPermission{
				hostedMCPReadPermission("system.read", "system", "notifications"),
				hostedMCPReadPermission("system.update", "system", "notifications"),
				hostedMCPReadPermission("config_repo.read", "folder", "*"),
				hostedMCPReadPermission("config_repo.manage", "folder", "*"),
			},
			Notes: []string{
				"SMTP test is an external side effect and requires explicit confirmation.",
				"Mail and route updates return GitOps file plans by default.",
			},
		},
		{
			Area: "Metrics and monitoring",
			Features: []string{
				"Metrics",
				"Monitoring",
				"Monitoring analytics",
				"Monitoring filters",
				"LLM usage accounting",
				"Runner trend sampling",
				"Monitoring saved views",
				"Monitoring alert rules",
				"Monitoring alert events",
				"Monitoring recommendations",
				"Dispatcher queue visibility",
				"Runner metadata display",
				"Active-run inspection",
			},
			Coverage: "first_class",
			Mode:     "analytics, cost/statistics/dispatcher, saved views, alerts, and recommendation operations first-class",
			Tools: []string{
				"nopsai.call_api",
				"nopsai.get_statistics",
				"nopsai.get_cost_summary",
				"nopsai.suggest_cost_improvements",
				"nopsai.get_dispatcher_status",
				"nopsai.get_monitoring_summary",
				"nopsai.get_monitoring_run_analytics",
				"nopsai.get_monitoring_pipeline_performance",
				"nopsai.get_monitoring_step_performance",
				"nopsai.get_monitoring_task_performance",
				"nopsai.get_monitoring_trigger_analytics",
				"nopsai.get_monitoring_external_trigger_analytics",
				"nopsai.get_monitoring_ai_usage",
				"nopsai.get_monitoring_reliability",
				"nopsai.get_monitoring_efficiency",
				"nopsai.get_monitoring_security",
				"nopsai.get_monitoring_runner_history",
				"nopsai.list_monitoring_views",
				"nopsai.create_monitoring_view",
				"nopsai.update_monitoring_view",
				"nopsai.delete_monitoring_view",
				"nopsai.list_monitoring_alert_rules",
				"nopsai.create_monitoring_alert_rule",
				"nopsai.update_monitoring_alert_rule",
				"nopsai.delete_monitoring_alert_rule",
				"nopsai.evaluate_monitoring_alert_rule",
				"nopsai.list_monitoring_alert_events",
				"nopsai.list_monitoring_recommendations",
				"nopsai.acknowledge_monitoring_recommendation",
				"nopsai.resolve_monitoring_recommendation",
			},
			Resources: []string{
				"nopsai://statistics",
				"nopsai://costs",
				"nopsai://system/dispatcher",
			},
			APIRoutes: []string{
				"GET /metrics",
				"GET /v1/monitoring/dispatcher",
				"GET /v1/monitoring/runners/history",
				"GET /v1/monitoring/summary",
				"GET /v1/monitoring/runs/analytics",
				"GET /v1/monitoring/pipelines/performance",
				"GET /v1/monitoring/steps/performance",
				"GET /v1/monitoring/tasks/performance",
				"GET /v1/monitoring/triggers/analytics",
				"GET /v1/monitoring/external-triggers/analytics",
				"GET /v1/monitoring/ai-usage",
				"GET /v1/monitoring/reliability",
				"GET /v1/monitoring/efficiency",
				"GET /v1/monitoring/security",
				"GET /v1/monitoring/views",
				"POST /v1/monitoring/views",
				"PUT /v1/monitoring/views/{viewID}",
				"DELETE /v1/monitoring/views/{viewID}",
				"GET /v1/monitoring/alert-rules",
				"POST /v1/monitoring/alert-rules",
				"PUT /v1/monitoring/alert-rules/{ruleID}",
				"DELETE /v1/monitoring/alert-rules/{ruleID}",
				"POST /v1/monitoring/alert-rules/{ruleID}/evaluate",
				"GET /v1/monitoring/alert-events",
				"GET /v1/monitoring/recommendations",
				"POST /v1/monitoring/recommendations/{recommendationID}/acknowledge",
				"POST /v1/monitoring/recommendations/{recommendationID}/resolve",
			},
			Permissions: []hostedMCPPermission{
				hostedMCPReadPermission("pipeline_run.list", "pipeline_run", "*"),
				hostedMCPReadPermission("pipeline_run.read", "pipeline_run", "*"),
				hostedMCPReadPermission("system.read", "dispatcher", "status"),
			},
			Notes: []string{
				"Monitoring handlers already filter active runs and analytics by the current subject.",
				"Saved views, alert rules, and recommendations are owner-scoped by the existing monitoring handlers.",
				"Dedicated analytics tools map one-to-one to the monitoring aggregate endpoints and accept the shared monitoring filter set.",
			},
		},
		{
			Area: "System, profiles, credentials, and runners",
			Features: []string{
				"Docker Compose stack",
				"AAA service",
				"Runner capacities",
				"Kubernetes runner install",
				"Runtime pools",
				"Dispatcher status",
				"Runner status",
				"Runner dispatch pause/resume",
				"Local login",
				"Access token flow",
				"Refresh token flow",
				"Personal access tokens",
				"Password changes",
				"Email updates",
				"Login rate limiting",
				"Login lockout",
				"LLM profile sync",
				"Agent profile sync",
				"MCP registry sync",
				"Credential sync",
			},
			Coverage: "first_class",
			Mode:     "profile/status reads, credential metadata/mutations/GitOps plans, and runner install/dispatch workflows first-class",
			Tools: []string{
				"nopsai.call_api",
				"nopsai.get_llm_profiles",
				"nopsai.get_mcp_profiles",
				"nopsai.get_system_status",
				"nopsai.get_dispatcher_status",
				"nopsai.list_credentials_metadata",
				"nopsai.get_credential_metadata",
				"nopsai.create_credential",
				"nopsai.rotate_credential_value",
				"nopsai.activate_credential_version",
				"nopsai.disable_credential",
				"nopsai.enable_credential",
				"nopsai.delete_credential_version",
				"nopsai.delete_credential",
				"nopsai.propose_credential_gitops",
				"nopsai.generate_runner_compose",
				"nopsai.generate_kubernetes_runner_manifest",
				"nopsai.generate_runner_bootstrap_command",
				"nopsai.generate_kubernetes_runner_bootstrap_command",
				"nopsai.update_runner_dispatch",
			},
			Resources: []string{
				"nopsai://system/llm-profiles",
				"nopsai://system/mcp-profiles",
				"nopsai://system/status",
				"nopsai://system/dispatcher",
			},
			APIRoutes: []string{
				"GET /v1/auth/me",
				"POST /v1/auth/password",
				"POST /v1/auth/email",
				"GET /v1/auth/personal-tokens",
				"POST /v1/auth/personal-tokens",
				"DELETE /v1/auth/personal-tokens/{tokenID}",
				"GET /v1/system/config",
				"PUT /v1/system/config",
				"GET /v1/system/credentials",
				"POST /v1/system/credentials",
				"GET /v1/system/credentials/{credentialID}",
				"PUT /v1/system/credentials/{credentialID}/value",
				"POST /v1/system/credentials/{credentialID}/versions/{version}/activate",
				"DELETE /v1/system/credentials/{credentialID}/versions/{version}",
				"POST /v1/system/credentials/{credentialID}/disable",
				"POST /v1/system/credentials/{credentialID}/enable",
				"DELETE /v1/system/credentials/{credentialID}",
				"GET /v1/system/llm-profiles",
				"GET /v1/system/agent-profiles",
				"GET /v1/system/mcp/servers",
				"GET /v1/system/mcp/profiles",
				"GET /v1/system/dispatcher",
				"GET /v1/system/dispatcher/scopes",
				"GET /v1/system/dispatcher/runner-compose",
				"GET /v1/system/dispatcher/runner-bootstrap-command",
				"GET /v1/system/dispatcher/kubernetes-runner-bootstrap-command",
				"GET /v1/system/dispatcher/kubernetes-runner-manifest",
				"POST /v1/system/dispatcher/runners/{runnerID}/dispatch",
			},
			Permissions: []hostedMCPPermission{
				hostedMCPReadPermission("system.read", "system", "config"),
				hostedMCPReadPermission("system.update", "system", "config"),
				hostedMCPReadPermission("system.read", "system", "llm-profiles"),
				hostedMCPReadPermission("system.update", "system", "llm-profiles"),
				hostedMCPReadPermission("system.read", "system", "agent-profiles"),
				hostedMCPReadPermission("system.update", "system", "agent-profiles"),
				hostedMCPReadPermission("system.read", "system", "mcp"),
				hostedMCPReadPermission("system.update", "system", "mcp"),
				hostedMCPReadPermission("credential.list_metadata", "credential", "*"),
				hostedMCPReadPermission("credential.create", "credential", "*"),
				hostedMCPReadPermission("credential.write_value", "credential", "*"),
				hostedMCPReadPermission("credential.rotate", "credential", "*"),
				hostedMCPReadPermission("credential.disable", "credential", "*"),
				hostedMCPReadPermission("credential.enable", "credential", "*"),
				hostedMCPReadPermission("credential.delete_version", "credential", "*"),
				hostedMCPReadPermission("credential.delete", "credential", "*"),
				hostedMCPReadPermission("system.read", "dispatcher", "status"),
				hostedMCPReadPermission("system.read", "dispatcher", "scopes"),
				hostedMCPReadPermission("system.update", "dispatcher", "runners"),
			},
			Notes: []string{
				"Auth self-service endpoints stay authenticated-user operations rather than AAA capability tools.",
				"Credential values should be rotated or referenced, not echoed into assistant context; hosted MCP redacts sensitive credential inputs and outputs from audit.",
				"Runner bootstrap command tools require include_sensitive_response:true because responses contain one-time bootstrap tokens.",
			},
		},
		{
			Area: "AAA, access, and audit",
			Features: []string{
				"AAA service",
				"AAA fallback",
				"Policy schema",
				"Route-level protection",
				"Product roles",
				"Access-grant management",
				"Per-resource access controls",
				"Runtime use checks",
				"Resource visibility modes",
				"Group-path inheritance",
				"Deny-before-allow evaluation",
				"Effective-permission introspection",
				"Casbin metadata compatibility",
				"Admin bootstrap",
				"Audit logging",
			},
			Coverage: "first_class",
			Mode:     "permission-bound MCP foundation plus dedicated access, audit, resource-use, and admin workflow tools",
			Tools: []string{
				"nopsai.call_api",
				"nopsai.get_feature_capabilities",
				"nopsai.list_access_grants",
				"nopsai.create_access_grant",
				"nopsai.delete_access_grant",
				"nopsai.get_effective_permissions",
				"nopsai.check_resource_use",
				"nopsai.batch_check_resource_use",
				"nopsai.get_resource_access",
				"nopsai.update_resource_access",
				"nopsai.create_resource_use_grant",
				"nopsai.delete_resource_access_grant",
				"nopsai.list_audit_logs",
				"nopsai.list_admin_users",
				"nopsai.create_admin_user",
				"nopsai.update_admin_user",
				"nopsai.delete_admin_user",
				"nopsai.list_admin_service_accounts",
				"nopsai.create_admin_service_account",
				"nopsai.update_admin_service_account",
				"nopsai.delete_admin_service_account",
				"nopsai.list_admin_roles",
				"nopsai.create_admin_role",
				"nopsai.delete_admin_role",
				"nopsai.list_admin_identity_providers",
				"nopsai.update_admin_identity_provider",
			},
			Resources: []string{
				"nopsai://features",
			},
			APIRoutes: []string{
				"GET /v1/access/grants",
				"POST /v1/access/grants",
				"DELETE /v1/access/grants/{grantID}",
				"GET /v1/access/groups",
				"GET /v1/access/auth-groups",
				"GET /v1/access/effective-permissions",
				"POST /v1/authz/resource-use/check",
				"POST /v1/authz/resource-use/batch-check",
				"/v1/resources/{resourceType}/{resourceID...}",
				"GET /v1/audit",
				"GET /v1/admin/users",
				"GET /v1/admin/service-accounts",
				"GET /v1/admin/roles",
				"GET /v1/admin/identity-providers",
			},
			Permissions: []hostedMCPPermission{
				hostedMCPReadPermission("iam.admin", "iam", "admin"),
				hostedMCPReadPermission("audit.read", "audit", "authz"),
			},
			Notes: []string{
				"Hosted MCP tools and resources are listed and executed as the current authenticated subject.",
				"Tool calls perform an initial tool permission check and then re-check specific resources when arguments identify one.",
				"Dedicated admin tools call existing protected routes; resource-scoped grant handlers still enforce owner/admin rules.",
			},
		},
		{
			Area: "Data management and runtime operations",
			Features: []string{
				"Data backups",
				"Data cleanup",
				"Scheduled cleanup",
				"Log batching",
				"REST polling",
				"Image pre-pull",
				"Container naming",
			},
			Coverage: "first_class",
			Mode:     "backup and cleanup operations first-class with high-impact confirmation",
			Tools: []string{
				"nopsai.call_api",
				"nopsai.list_data_backups",
				"nopsai.create_data_backup",
				"nopsai.delete_data_backup",
				"nopsai.preview_data_cleanup",
				"nopsai.run_data_cleanup",
				"nopsai.list_data_cleanup_jobs",
				"nopsai.list_data_cleanup_schedules",
				"nopsai.create_data_cleanup_schedule",
				"nopsai.update_data_cleanup_schedule",
				"nopsai.delete_data_cleanup_schedule",
				"nopsai.run_data_cleanup_schedule",
				"nopsai.enable_data_cleanup_schedule",
				"nopsai.disable_data_cleanup_schedule",
			},
			APIRoutes: []string{
				"GET /v1/system/data/backups",
				"POST /v1/system/data/backups",
				"GET /v1/system/data/backups/{backupID}/download",
				"DELETE /v1/system/data/backups/{backupID}",
				"POST /v1/system/data/cleanup/preview",
				"POST /v1/system/data/cleanup/run",
				"GET /v1/system/data/cleanup/jobs",
				"GET /v1/system/data/cleanup/schedules",
				"POST /v1/system/data/cleanup/schedules",
				"PUT /v1/system/data/cleanup/schedules/{scheduleID}",
				"DELETE /v1/system/data/cleanup/schedules/{scheduleID}",
				"POST /v1/system/data/cleanup/schedules/{scheduleID}/run",
				"POST /v1/system/data/cleanup/schedules/{scheduleID}/enable",
				"POST /v1/system/data/cleanup/schedules/{scheduleID}/disable",
			},
			Permissions: []hostedMCPPermission{
				hostedMCPReadPermission("system.read", "system", "config"),
				hostedMCPReadPermission("system.update", "system", "config"),
			},
			Notes: []string{
				"Backup creation/deletion and cleanup execution are marked high-impact and require explicit confirmation.",
			},
		},
		{
			Area: "UI surfaces",
			Features: []string{
				"Pipeline Runs UI",
				"Pipelines UI",
				"Schedules UI",
				"Triggers UI",
				"Scopes UI",
				"Lab UI",
				"Steps UI",
				"Knowledge Context UI",
				"System UI",
				"Profile UI",
				"Login UI",
			},
			Coverage: "contextual",
			Mode:     "MCP supports backing product data; rendering remains UI-owned",
			Tools: []string{
				"nopsai.call_api",
				"nopsai.get_feature_capabilities",
				"nopsai.get_ui_context",
				"nopsai.list_pipelines",
				"nopsai.list_pipeline_runs",
				"nopsai.list_schedules",
				"nopsai.list_triggers",
				"nopsai.list_scopes",
				"nopsai.list_knowledge_contexts",
				"nopsai.get_system_status",
			},
			Resources: []string{
				"nopsai://features",
				"nopsai://pipelines",
				"nopsai://pipeline-runs",
				"nopsai://schedules",
				"nopsai://triggers",
				"nopsai://scopes",
				"nopsai://knowledge-contexts",
				"nopsai://system/status",
			},
			Notes: []string{
				"MCP should not own UI rendering; it should provide permission-bound operational context for assistant workflows.",
			},
		},
	}
}

func hostedMCPFeatureCapabilityMatches(capability hostedMCPFeatureCapability, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	values := []string{capability.Area, capability.Coverage, capability.Mode}
	values = append(values, capability.Features...)
	values = append(values, capability.Tools...)
	values = append(values, capability.Resources...)
	values = append(values, capability.APIRoutes...)
	values = append(values, capability.Notes...)
	for _, value := range values {
		if hostedMCPTextMatchesSearch(value, query) {
			return true
		}
	}
	for _, permission := range capability.Permissions {
		if hostedMCPTextMatchesSearch(permission.Action+" "+permission.Resource.Type+" "+permission.Resource.ID, query) {
			return true
		}
	}
	return false
}

func hostedMCPNormalizeFeatureCapabilityFilters(areaFilter, query string) (string, string) {
	areaFilter = strings.ToLower(strings.TrimSpace(areaFilter))
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return areaFilter, query
	}
	if containsAny(query, "features can i use", "what features", "assistant capabilities", "capabilities do i have", "available tools", "available resources", "feature coverage", "mcp coverage") {
		return areaFilter, ""
	}
	if containsAny(query, "policy", "prevent", "block", "hide", "showing", "show", "expose", "read") &&
		containsAny(query, "env", "envs", "environment variable", "environment variables", "secret", "secrets", "credential", "credentials", "token", "password") {
		if areaFilter == "" {
			if containsAny(query, "credential", "credentials") && !containsAny(query, "env", "envs", "secret", "secrets", "environment") {
				areaFilter = "system"
			} else {
				areaFilter = "secrets"
			}
		}
		return areaFilter, ""
	}
	return areaFilter, query
}

func hostedMCPToolNames(tools []hostedMCPTool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

func hostedMCPResourceURIs(resources []hostedMCPResource) []string {
	uris := make([]string, 0, len(resources))
	for _, resource := range resources {
		uris = append(uris, resource.URI)
	}
	sort.Strings(uris)
	return uris
}

func hostedMCPNameSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	return set
}

func hostedMCPPartitionNames(values []string, available map[string]struct{}) ([]string, []string) {
	have := []string{}
	missing := []string{}
	for _, value := range values {
		if _, ok := available[value]; ok {
			have = append(have, value)
		} else {
			missing = append(missing, value)
		}
	}
	sort.Strings(have)
	sort.Strings(missing)
	return have, missing
}

func hostedMCPAccessState(available, total int) string {
	switch {
	case total == 0:
		return "not_applicable"
	case available == total:
		return "available"
	case available > 0:
		return "partial"
	default:
		return "unavailable"
	}
}

func hostedMCPPermissionAccessState(checks []map[string]any) string {
	if len(checks) == 0 {
		return "not_applicable"
	}
	allowed := 0
	for _, check := range checks {
		if value, _ := check["allowed"].(bool); value {
			allowed++
		}
	}
	return hostedMCPAccessState(allowed, len(checks))
}

func hostedMCPSubjectSummary(subject aaamodel.Subject) map[string]any {
	return map[string]any{
		"type":  strings.TrimSpace(subject.Type),
		"id":    strings.TrimSpace(subject.ID),
		"sub":   strings.TrimSpace(subject.Sub),
		"email": strings.TrimSpace(subject.Email),
	}
}

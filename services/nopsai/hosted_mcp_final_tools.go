package nopsai

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
	aaamodel "nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/internal/credentials"
)

func hostedMCPFinalTools() []hostedMCPTool {
	monitoringSchema := objectSchema(map[string]any{
		"from":                   stringSchema(),
		"to":                     stringSchema(),
		"groupId":                stringSchema(),
		"group_id":               stringSchema(),
		"pipelinePath":           stringSchema(),
		"pipeline_path":          stringSchema(),
		"pipelineName":           stringSchema(),
		"pipeline_name":          stringSchema(),
		"repo":                   stringSchema(),
		"runId":                  stringSchema(),
		"run_id":                 stringSchema(),
		"branch":                 stringSchema(),
		"ref":                    stringSchema(),
		"commitSHA":              stringSchema(),
		"commit":                 stringSchema(),
		"triggerSource":          stringSchema(),
		"trigger_source":         stringSchema(),
		"status":                 stringSchema(),
		"requestedByType":        stringSchema(),
		"requested_by_type":      stringSchema(),
		"requestedById":          stringSchema(),
		"requested_by_id":        stringSchema(),
		"effectiveSubjectType":   stringSchema(),
		"effective_subject_type": stringSchema(),
		"effectiveSubjectId":     stringSchema(),
		"effective_subject_id":   stringSchema(),
		"externalTriggerId":      stringSchema(),
		"external_trigger_id":    stringSchema(),
		"scheduleId":             stringSchema(),
		"schedule_id":            stringSchema(),
		"provider":               stringSchema(),
		"model":                  stringSchema(),
		"llmProfile":             stringSchema(),
		"llm_profile":            stringSchema(),
		"profile":                stringSchema(),
		"feature":                stringSchema(),
		"stepName":               stringSchema(),
		"step_name":              stringSchema(),
		"taskName":               stringSchema(),
		"task_name":              stringSchema(),
		"compare":                stringSchema(),
		"minDurationSeconds":     numberSchema(),
		"min_duration_seconds":   numberSchema(),
		"maxDurationSeconds":     numberSchema(),
		"max_duration_seconds":   numberSchema(),
		"query":                  objectSchema(map[string]any{}),
	})

	return []hostedMCPTool{
		toolDef("nopsai.list_system_log_sources", "List allow-listed platform log sources visible to the current subject.", "system_log.read", "system_log", "*", objectSchema(map[string]any{})),
		toolDef("nopsai.tail_system_logs", "Read a bounded, server-redacted tail of one allow-listed platform log source.", "system_log.read", "system_log", "*", objectSchema(map[string]any{"source_id": stringSchema(), "lines": numberSchema()})),
		toolDef("nopsai.get_setup_status", "Read first-install setup status, starter profile state, counts, and health checks.", "system.read", "system", "config", objectSchema(map[string]any{})),
		toolDef("nopsai.get_setup_preflight", "Run public first-install setup preflight checks.", "system.read", "system", "config", objectSchema(map[string]any{})),
		toolDef("nopsai.get_setup_templates", "Generate starter GitOps setup templates for a setup profile.", "system.read", "system", "config", objectSchema(map[string]any{"profile": stringSchema(), "repositories": objectSchema(map[string]any{}), "repository_groups": objectSchema(map[string]any{}), "include_llm": booleanSchema(), "include_mcp": booleanSchema(), "llm_profile": objectSchema(map[string]any{}), "users": objectSchema(map[string]any{})})),
		toolDef("nopsai.plan_first_install_setup", "Return a guided first-install setup plan and GitOps starter template bundle without applying changes.", "system.read", "system", "config", objectSchema(map[string]any{"profile": stringSchema(), "repositories": objectSchema(map[string]any{}), "repository_groups": objectSchema(map[string]any{}), "include_llm": booleanSchema(), "include_mcp": booleanSchema(), "llm_profile": objectSchema(map[string]any{}), "users": objectSchema(map[string]any{})})),
		toolDef("nopsai.bootstrap_first_install_setup", "Run first-install bootstrap as the current subject. High-impact operation requiring confirm:true.", "system.update", "system", "config", objectSchema(map[string]any{"profile": stringSchema(), "generate_secrets": booleanSchema(), "seed_starter_database": booleanSchema(), "seed_llm_profile": booleanSchema(), "mcp_examples": booleanSchema(), "production_acknowledged": booleanSchema(), "sync_config_repository": booleanSchema(), "config_repository": objectSchema(map[string]any{}), "repository_groups": objectSchema(map[string]any{}), "repositories": objectSchema(map[string]any{}), "llm_profile": objectSchema(map[string]any{}), "users": objectSchema(map[string]any{}), "confirm": booleanSchema()})),

		toolDef("nopsai.propose_reusable_step_create", "Validate reusable step YAML and return a GitOps-ready create file plan without applying changes.", "step.create", "step", "*", objectSchema(map[string]any{"step": stringSchema(), "path": stringSchema(), "name": stringSchema(), "yaml": stringSchema(), "definition": stringSchema(), "message": stringSchema()})),
		toolDef("nopsai.propose_reusable_step_update", "Validate reusable step YAML and return a GitOps-ready update file plan without applying changes.", "step.update", "step", "*", objectSchema(map[string]any{"step": stringSchema(), "path": stringSchema(), "name": stringSchema(), "yaml": stringSchema(), "definition": stringSchema(), "message": stringSchema()})),
		toolDef("nopsai.propose_reusable_step_delete", "Return a GitOps-ready reusable step delete file plan without applying changes.", "step.delete", "step", "*", objectSchema(map[string]any{"step": stringSchema(), "path": stringSchema(), "name": stringSchema(), "message": stringSchema()})),

		toolDef("nopsai.list_secrets_metadata", "List secret metadata only; plaintext secret values are never returned.", "secret.list_metadata", "secret", "*", objectSchema(map[string]any{"repository": stringSchema(), "repo_owner": stringSchema(), "repo_name": stringSchema(), "scope": stringSchema()})),
		toolDef("nopsai.list_secret_scopes", "List scopes that contain visible secrets.", "secret.list_metadata", "secret", "*", objectSchema(map[string]any{})),
		toolDef("nopsai.encrypt_secret_for_gitops", "Encrypt a secret value for GitOps. The value and encrypted output are redacted from hosted MCP audit.", "secret.write_value", "secret", "*", objectSchema(map[string]any{"value": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.write_secret_value", "Create or update a secret value through the API as the current subject. Requires confirm:true and never reads plaintext back.", "secret.write_value", "secret", "*", objectSchema(map[string]any{"name": stringSchema(), "secret_name": stringSchema(), "value": stringSchema(), "repository": stringSchema(), "repo_owner": stringSchema(), "repo_name": stringSchema(), "scope": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.delete_secret_value", "Delete a secret value through the API as the current subject. Requires confirm:true.", "secret.delete", "secret", "*", objectSchema(map[string]any{"name": stringSchema(), "secret_name": stringSchema(), "repository": stringSchema(), "repo_owner": stringSchema(), "repo_name": stringSchema(), "scope": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.propose_secret_gitops_write", "Return a scoped GitOps file plan for an encrypted secret value without applying changes.", "secret.write_value", "secret", "*", objectSchema(map[string]any{"name": stringSchema(), "secret_name": stringSchema(), "encrypted_value": stringSchema(), "repository": stringSchema(), "repo_owner": stringSchema(), "repo_name": stringSchema(), "scope": stringSchema(), "message": stringSchema()})),
		toolDef("nopsai.propose_secret_gitops_delete", "Return a scoped GitOps edit plan to remove a secret entry without applying changes.", "secret.delete", "secret", "*", objectSchema(map[string]any{"name": stringSchema(), "secret_name": stringSchema(), "repository": stringSchema(), "repo_owner": stringSchema(), "repo_name": stringSchema(), "scope": stringSchema(), "message": stringSchema()})),
		toolDef("nopsai.list_variables_metadata", "List variable metadata visible to the current subject.", "variable.list_metadata", "variable", "*", objectSchema(map[string]any{"repository": stringSchema(), "repo_owner": stringSchema(), "repo_name": stringSchema(), "scope": stringSchema()})),
		toolDef("nopsai.list_variable_scopes", "List scopes that contain visible variables.", "variable.list_metadata", "variable", "*", objectSchema(map[string]any{})),
		toolDef("nopsai.analyze_variable_usage", "Analyze visible variable metadata for repeated names across scopes and repositories without reading values.", "variable.list_metadata", "variable", "*", objectSchema(map[string]any{"scope": stringSchema(), "repository": stringSchema(), "repo_owner": stringSchema(), "repo_name": stringSchema(), "limit": numberSchema()})),
		toolDef("nopsai.get_variable_value", "Read a variable value as the current subject when allowed.", "variable.read_value", "variable", "*", objectSchema(map[string]any{"name": stringSchema(), "variable_name": stringSchema(), "repository": stringSchema(), "repo_owner": stringSchema(), "repo_name": stringSchema(), "scope": stringSchema()})),
		toolDef("nopsai.write_variable_value", "Create or update a variable value through the API as the current subject. Requires confirm:true.", "variable.write_value", "variable", "*", objectSchema(map[string]any{"name": stringSchema(), "variable_name": stringSchema(), "value": stringSchema(), "repository": stringSchema(), "repo_owner": stringSchema(), "repo_name": stringSchema(), "scope": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.delete_variable_value", "Delete a variable value through the API as the current subject. Requires confirm:true.", "variable.delete", "variable", "*", objectSchema(map[string]any{"name": stringSchema(), "variable_name": stringSchema(), "repository": stringSchema(), "repo_owner": stringSchema(), "repo_name": stringSchema(), "scope": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.propose_variable_gitops_write", "Return a scoped GitOps file plan for a variable value without applying changes.", "variable.write_value", "variable", "*", objectSchema(map[string]any{"name": stringSchema(), "variable_name": stringSchema(), "value": stringSchema(), "repository": stringSchema(), "repo_owner": stringSchema(), "repo_name": stringSchema(), "scope": stringSchema(), "message": stringSchema()})),
		toolDef("nopsai.propose_variable_gitops_delete", "Return a scoped GitOps edit plan to remove a variable entry without applying changes.", "variable.delete", "variable", "*", objectSchema(map[string]any{"name": stringSchema(), "variable_name": stringSchema(), "repository": stringSchema(), "repo_owner": stringSchema(), "repo_name": stringSchema(), "scope": stringSchema(), "message": stringSchema()})),

		toolDef("nopsai.explain_internal_run_operations", "Explain why internal run service callbacks are not assistant mutation tools and show safe alternatives.", "pipeline_run.read", "pipeline_run", "*", objectSchema(map[string]any{"run_id": stringSchema()})),
		toolDef("nopsai.explain_webhook_ingress_policy", "Explain why public webhook delivery ingress is not exposed as an assistant mutation tool.", "trigger.read", "trigger", "*", objectSchema(map[string]any{"source_id": stringSchema(), "repository": stringSchema()})),

		toolDef("nopsai.get_monitoring_summary", "Read monitoring summary analytics with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.get_monitoring_run_analytics", "Read run analytics with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.get_monitoring_pipeline_performance", "Read pipeline performance analytics with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.get_monitoring_step_performance", "Read step performance analytics with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.get_monitoring_task_performance", "Read task performance analytics with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.get_monitoring_trigger_analytics", "Read trigger analytics with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.get_monitoring_external_trigger_analytics", "Read external trigger analytics with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.get_monitoring_ai_usage", "Read AI usage analytics with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.get_monitoring_reliability", "Read reliability analytics with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.get_monitoring_efficiency", "Read efficiency analytics with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.get_monitoring_security", "Read security analytics with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.get_monitoring_runner_history", "Read runner history analytics with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.get_monitoring_schedule_ai_usage", "Read schedule-level AI usage analytics with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.get_monitoring_schedule_performance", "Read combined schedule performance, AI usage, and efficiency analytics with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.get_monitoring_trigger_performance", "Read combined trigger and external-trigger performance analytics with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.get_pipeline_efficiency", "Read combined pipeline performance, efficiency, and AI usage analytics with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.compare_pipelines", "Compare pipeline performance, reliability, efficiency, and AI usage with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.compare_schedules", "Compare schedule AI usage, run performance, and efficiency with filters.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.explain_pipeline_health", "Explain pipeline health from performance, reliability, efficiency, and security analytics.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),
		toolDef("nopsai.find_optimization_opportunities", "Find optimization opportunities from efficiency, recommendation, AI usage, and pipeline performance analytics.", "pipeline_run.list", "pipeline_run", "*", monitoringSchema),

		toolDef("nopsai.list_credentials_metadata", "List credential metadata only; credential values are never returned.", "credential.list_metadata", "credential", "*", objectSchema(map[string]any{})),
		toolDef("nopsai.get_credential_metadata", "Read credential metadata and version metadata by credential id.", "credential.list_metadata", "credential", "*", objectSchema(map[string]any{"credential_id": stringSchema()})),
		toolDef("nopsai.create_credential", "Create credential metadata and optional initial value. Requires confirm:true and redacts values from hosted MCP audit.", "credential.create", "credential", "*", objectSchema(map[string]any{"reference": stringSchema(), "kind": stringSchema(), "description": stringSchema(), "value": stringSchema(), "expires_at": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.rotate_credential_value", "Rotate a credential value. Requires confirm:true and redacts the value from hosted MCP audit.", "credential.write_value", "credential", "*", objectSchema(map[string]any{"credential_id": stringSchema(), "value": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.activate_credential_version", "Activate a credential version. Requires confirm:true.", "credential.rotate", "credential", "*", objectSchema(map[string]any{"credential_id": stringSchema(), "version": numberSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.disable_credential", "Disable a credential. Requires confirm:true.", "credential.disable", "credential", "*", objectSchema(map[string]any{"credential_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.enable_credential", "Enable a credential. Requires confirm:true.", "credential.enable", "credential", "*", objectSchema(map[string]any{"credential_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.delete_credential_version", "Delete a credential version. High-impact operation requiring confirm:true.", "credential.delete_version", "credential", "*", objectSchema(map[string]any{"credential_id": stringSchema(), "version": numberSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.delete_credential", "Delete credential metadata and value history. High-impact operation requiring confirm:true.", "credential.delete", "credential", "*", objectSchema(map[string]any{"credential_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.propose_credential_gitops", "Return a GitOps-ready credentials.yaml plan with encrypted credential version material only.", "credential.create", "credential", "*", objectSchema(map[string]any{"credential": objectSchema(map[string]any{}), "reference": stringSchema(), "kind": stringSchema(), "description": stringSchema(), "status": stringSchema(), "active_version": numberSchema(), "expires_at": stringSchema(), "versions": objectSchema(map[string]any{}), "message": stringSchema()})),

		toolDef("nopsai.generate_runner_compose", "Generate Docker Compose runner installation artifacts.", "system.update", "dispatcher", "runners", objectSchema(map[string]any{"runner_id": stringSchema(), "runner_scopes": stringSchema(), "runner_capacity": numberSchema(), "query": objectSchema(map[string]any{})})),
		toolDef("nopsai.generate_kubernetes_runner_manifest", "Generate Kubernetes runner installation manifest.", "system.update", "dispatcher", "runners", objectSchema(map[string]any{"runner_id": stringSchema(), "runner_scopes": stringSchema(), "runner_capacity": numberSchema(), "namespace": stringSchema(), "service_account": stringSchema(), "storage_class": stringSchema(), "query": objectSchema(map[string]any{})})),
		toolDef("nopsai.generate_runner_bootstrap_command", "Generate a runner bootstrap command. Sensitive response requires include_sensitive_response:true.", "system.update", "dispatcher", "runners", objectSchema(map[string]any{"runner_id": stringSchema(), "runner_scopes": stringSchema(), "runner_capacity": numberSchema(), "include_sensitive_response": booleanSchema(), "query": objectSchema(map[string]any{})})),
		toolDef("nopsai.generate_kubernetes_runner_bootstrap_command", "Generate a Kubernetes runner bootstrap command. Sensitive response requires include_sensitive_response:true.", "system.update", "dispatcher", "runners", objectSchema(map[string]any{"runner_id": stringSchema(), "runner_scopes": stringSchema(), "runner_capacity": numberSchema(), "namespace": stringSchema(), "service_account": stringSchema(), "storage_class": stringSchema(), "include_sensitive_response": booleanSchema(), "query": objectSchema(map[string]any{})})),
		toolDef("nopsai.update_runner_dispatch", "Pause or resume dispatcher work assignment for a runner. Requires confirm:true.", "system.update", "dispatcher", "runners", objectSchema(map[string]any{"runner_id": stringSchema(), "allow_dispatch": booleanSchema(), "connection_id": stringSchema(), "confirm": booleanSchema()})),

		toolDef("nopsai.list_access_grants", "List access grants. Global listing requires iam.admin; resource-scoped listing is still enforced by the API.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"resource_type": stringSchema(), "resource_id": stringSchema(), "role": stringSchema()})),
		toolDef("nopsai.create_access_grant", "Create an access grant. Requires confirm:true and API-level grant authorization.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"subject_type": stringSchema(), "subject_id": stringSchema(), "role": stringSchema(), "resource_type": stringSchema(), "resource_id": stringSchema(), "inherit": booleanSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.delete_access_grant", "Delete an access grant. Requires confirm:true and API-level grant authorization.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"grant_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.get_effective_permissions", "Check the current subject's effective permission for an action and resource.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"action": stringSchema(), "resource_type": stringSchema(), "resource_id": stringSchema()})),
		toolDef("nopsai.check_resource_use", "Run a resource-use authorization check for the current subject or an admin-specified caller.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"body": objectSchema(map[string]any{}), "caller_type": stringSchema(), "caller_id": stringSchema(), "resource_type": stringSchema(), "resource_id": stringSchema(), "action": stringSchema()})),
		toolDef("nopsai.batch_check_resource_use", "Run batch resource-use authorization checks.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"body": objectSchema(map[string]any{}), "caller_type": stringSchema(), "caller_id": stringSchema(), "checks": objectSchema(map[string]any{})})),
		toolDef("nopsai.get_resource_access", "Read resource access settings.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"resource_type": stringSchema(), "resource_id": stringSchema()})),
		toolDef("nopsai.update_resource_access", "Update resource access settings. Requires confirm:true.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"resource_type": stringSchema(), "resource_id": stringSchema(), "visibility": stringSchema(), "use_access": objectSchema(map[string]any{}), "body": objectSchema(map[string]any{}), "confirm": booleanSchema()})),
		toolDef("nopsai.create_resource_use_grant", "Create a resource-use grant. Requires confirm:true.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"resource_type": stringSchema(), "resource_id": stringSchema(), "subject_type": stringSchema(), "subject_id": stringSchema(), "actions": objectSchema(map[string]any{}), "conditions": objectSchema(map[string]any{}), "body": objectSchema(map[string]any{}), "confirm": booleanSchema()})),
		toolDef("nopsai.delete_resource_access_grant", "Delete a resource access grant. Requires confirm:true.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"resource_type": stringSchema(), "resource_id": stringSchema(), "grant_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.list_audit_logs", "List AAA audit log entries.", "audit.read", "audit", "authz", objectSchema(map[string]any{"limit": numberSchema()})),
		toolDef("nopsai.list_admin_users", "List users through the admin API.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"query": objectSchema(map[string]any{})})),
		toolDef("nopsai.create_admin_user", "Create a user through the admin API. Requires confirm:true.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"body": objectSchema(map[string]any{}), "confirm": booleanSchema()})),
		toolDef("nopsai.update_admin_user", "Update a user through the admin API. Requires confirm:true.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"user_id": stringSchema(), "body": objectSchema(map[string]any{}), "confirm": booleanSchema()})),
		toolDef("nopsai.delete_admin_user", "Delete a user through the admin API. Requires confirm:true.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"user_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.list_admin_service_accounts", "List service accounts through the admin API.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"query": objectSchema(map[string]any{})})),
		toolDef("nopsai.create_admin_service_account", "Create a service account through the admin API. Requires confirm:true.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"body": objectSchema(map[string]any{}), "confirm": booleanSchema()})),
		toolDef("nopsai.update_admin_service_account", "Update a service account through the admin API. Requires confirm:true.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"service_account_id": stringSchema(), "body": objectSchema(map[string]any{}), "confirm": booleanSchema()})),
		toolDef("nopsai.delete_admin_service_account", "Delete a service account through the admin API. Requires confirm:true.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"service_account_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.list_admin_roles", "List roles through the admin API.", "iam.admin", "iam", "admin", objectSchema(map[string]any{})),
		toolDef("nopsai.create_admin_role", "Create a role through the admin API. Requires confirm:true.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"body": objectSchema(map[string]any{}), "confirm": booleanSchema()})),
		toolDef("nopsai.delete_admin_role", "Delete a role through the admin API. Requires confirm:true.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"body": objectSchema(map[string]any{}), "confirm": booleanSchema()})),
		toolDef("nopsai.list_admin_identity_providers", "List identity provider admin configuration.", "iam.admin", "iam", "admin", objectSchema(map[string]any{})),
		toolDef("nopsai.update_admin_identity_provider", "Update identity provider admin configuration. Requires confirm:true.", "iam.admin", "iam", "admin", objectSchema(map[string]any{"provider": stringSchema(), "body": objectSchema(map[string]any{}), "confirm": booleanSchema()})),

		toolDef("nopsai.get_ui_context", "Return contextual UI ownership hints; MCP provides data and plans while the UI owns rendering.", "system.read", "system", "mcp", objectSchema(map[string]any{"area": stringSchema()})),
	}
}

func (a *App) authorizeHostedMCPFinalToolCall(ctx context.Context, subject aaamodel.Subject, tool hostedMCPTool, args map[string]any) (bool, error) {
	permission := hostedMCPToolPermission(tool)
	if tool.Name == "nopsai.list_system_log_sources" && a.hostedMCPAnySystemLogAllowed(ctx, subject) {
		return true, nil
	}
	switch tool.Name {
	case "nopsai.propose_reusable_step_create", "nopsai.propose_reusable_step_update", "nopsai.propose_reusable_step_delete":
		permission.Resource.ID = hostedMCPReusableStepArgID(args)
	case "nopsai.list_secrets_metadata", "nopsai.list_secret_scopes":
		permission.Resource.ID = "*"
	case "nopsai.encrypt_secret_for_gitops", "nopsai.write_secret_value", "nopsai.delete_secret_value", "nopsai.propose_secret_gitops_write", "nopsai.propose_secret_gitops_delete":
		permission.Resource.ID = hostedMCPNamedValueResourceID(args, "secret")
	case "nopsai.list_variables_metadata", "nopsai.list_variable_scopes", "nopsai.analyze_variable_usage":
		permission.Resource.ID = "*"
	case "nopsai.get_variable_value", "nopsai.write_variable_value", "nopsai.delete_variable_value", "nopsai.propose_variable_gitops_write", "nopsai.propose_variable_gitops_delete":
		permission.Resource.ID = hostedMCPNamedValueResourceID(args, "variable")
	case "nopsai.get_credential_metadata", "nopsai.rotate_credential_value", "nopsai.activate_credential_version", "nopsai.disable_credential", "nopsai.enable_credential", "nopsai.delete_credential_version", "nopsai.delete_credential":
		permission.Resource.ID = stringArg(args, "credential_id")
	case "nopsai.propose_credential_gitops":
		permission.Resource.ID = firstNonEmptyString(hostedMCPCredentialReferenceResourceID(args), "*")
	case "nopsai.update_runner_dispatch":
		permission.Resource.ID = firstNonEmptyString(stringArg(args, "runner_id"), "runners")
	case "nopsai.explain_internal_run_operations":
		permission.Resource.ID = firstNonEmptyString(stringArg(args, "run_id"), "*")
	case "nopsai.explain_webhook_ingress_policy":
		permission.Resource.ID = firstNonEmptyString(stringArg(args, "source_id"), stringArg(args, "repository"), "*")
	case "nopsai.tail_system_logs":
		permission.Resource.ID = stringArg(args, "source_id")
	default:
		if !hostedMCPFinalToolName(tool.Name) {
			return false, nil
		}
	}
	if strings.TrimSpace(permission.Resource.ID) == "" {
		permission.Resource.ID = tool.Resource.ID
	}
	if tool.AuthenticatedOnly || a.hostedMCPAllowed(ctx, subject, permission) {
		return true, nil
	}
	return true, fmt.Errorf("tool %s is not allowed for %s:%s", tool.Name, permission.Resource.Type, permission.Resource.ID)
}

func (a *App) executeHostedMCPFinalTool(ctx context.Context, subject aaamodel.Subject, name string, args map[string]any) (map[string]any, bool, error) {
	switch name {
	case "nopsai.list_system_log_sources":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, "/v1/system/logs/sources", nil, false, false, false, ""), true, nil
	case "nopsai.tail_system_logs":
		sourceID := hostedMCPPathTail(stringArg(args, "source_id"))
		lines := intArg(args, "lines", 500, 2000)
		path := fmt.Sprintf("/v1/system/logs/sources/%s/tail?lines=%d", sourceID, lines)
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, path, nil, false, false, false, ""), true, nil
	case "nopsai.get_setup_status":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, "/v1/setup/status", nil, false, false, false, ""), true, nil
	case "nopsai.get_setup_preflight":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, "/v1/setup/preflight", nil, false, false, false, ""), true, nil
	case "nopsai.get_setup_templates":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, hostedMCPSetupTemplatesPath(args), nil, false, false, false, ""), true, nil
	case "nopsai.plan_first_install_setup":
		result, err := hostedMCPPlanFirstInstallSetup(args)
		return result, true, err
	case "nopsai.bootstrap_first_install_setup":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPost, "/v1/setup/bootstrap", hostedMCPBodyWithout(args, "confirm"), boolArg(args, "confirm", false), false, true, "First-install bootstrap can create users, credentials, repositories, groups, example resources, and local secrets."), true, nil
	case "nopsai.propose_reusable_step_create":
		result, err := hostedMCPProposeReusableStep(args, "create")
		return result, true, err
	case "nopsai.propose_reusable_step_update":
		result, err := hostedMCPProposeReusableStep(args, "update")
		return result, true, err
	case "nopsai.propose_reusable_step_delete":
		result, err := hostedMCPProposeReusableStep(args, "delete")
		return result, true, err
	case "nopsai.list_secrets_metadata":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, hostedMCPSecretVariablePath(args, "secrets", "list"), nil, false, false, false, ""), true, nil
	case "nopsai.list_secret_scopes":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, "/v1/secrets/scopes", nil, false, false, false, ""), true, nil
	case "nopsai.encrypt_secret_for_gitops":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPost, "/v1/secrets/encrypt", map[string]any{"value": stringArg(args, "value")}, true, false, true, "Secret encryption handles plaintext; hosted MCP audit redacts inputs and outputs."), true, nil
	case "nopsai.write_secret_value":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPut, hostedMCPSecretVariablePath(args, "secrets", "item"), map[string]any{"value": stringArg(args, "value")}, boolArg(args, "confirm", false), false, true, "Writing a secret stores encrypted value material and cannot be previewed as plaintext."), true, nil
	case "nopsai.delete_secret_value":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodDelete, hostedMCPSecretVariablePath(args, "secrets", "item"), nil, boolArg(args, "confirm", false), false, true, "Deleting a secret can break pipelines, triggers, or runtime integrations that use it."), true, nil
	case "nopsai.propose_secret_gitops_write":
		result, err := hostedMCPProposeScopedValueGitOps(args, "secrets", "write")
		return result, true, err
	case "nopsai.propose_secret_gitops_delete":
		result, err := hostedMCPProposeScopedValueGitOps(args, "secrets", "delete")
		return result, true, err
	case "nopsai.list_variables_metadata":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, hostedMCPSecretVariablePath(args, "variables", "list"), nil, false, false, false, ""), true, nil
	case "nopsai.list_variable_scopes":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, "/v1/variables/scopes", nil, false, false, false, ""), true, nil
	case "nopsai.analyze_variable_usage":
		result, err := a.hostedMCPAnalyzeVariableUsage(ctx, subject, args)
		return result, true, err
	case "nopsai.get_variable_value":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, hostedMCPSecretVariablePath(args, "variables", "item"), nil, false, false, false, ""), true, nil
	case "nopsai.write_variable_value":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPut, hostedMCPSecretVariablePath(args, "variables", "item"), map[string]any{"value": stringArg(args, "value")}, boolArg(args, "confirm", false), false, false, ""), true, nil
	case "nopsai.delete_variable_value":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodDelete, hostedMCPSecretVariablePath(args, "variables", "item"), nil, boolArg(args, "confirm", false), false, true, "Deleting a variable can break pipelines, triggers, or runtime integrations that use it."), true, nil
	case "nopsai.propose_variable_gitops_write":
		result, err := hostedMCPProposeScopedValueGitOps(args, "variables", "write")
		return result, true, err
	case "nopsai.propose_variable_gitops_delete":
		result, err := hostedMCPProposeScopedValueGitOps(args, "variables", "delete")
		return result, true, err
	case "nopsai.explain_internal_run_operations":
		return hostedMCPExplainInternalRunOperations(args), true, nil
	case "nopsai.explain_webhook_ingress_policy":
		return hostedMCPExplainWebhookIngressPolicy(args), true, nil
	case "nopsai.get_monitoring_summary", "nopsai.get_monitoring_run_analytics", "nopsai.get_monitoring_pipeline_performance", "nopsai.get_monitoring_step_performance", "nopsai.get_monitoring_task_performance", "nopsai.get_monitoring_trigger_analytics", "nopsai.get_monitoring_external_trigger_analytics", "nopsai.get_monitoring_ai_usage", "nopsai.get_monitoring_reliability", "nopsai.get_monitoring_efficiency", "nopsai.get_monitoring_security", "nopsai.get_monitoring_runner_history", "nopsai.get_monitoring_schedule_ai_usage":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, hostedMCPMonitoringAnalyticsPath(name, args), nil, false, false, false, ""), true, nil
	case "nopsai.get_monitoring_schedule_performance", "nopsai.get_monitoring_trigger_performance", "nopsai.get_pipeline_efficiency", "nopsai.compare_pipelines", "nopsai.compare_schedules", "nopsai.explain_pipeline_health", "nopsai.find_optimization_opportunities":
		return a.hostedMCPMonitoringInsightTool(ctx, subject, name, args), true, nil
	case "nopsai.list_credentials_metadata":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, "/v1/system/credentials", nil, false, false, false, ""), true, nil
	case "nopsai.get_credential_metadata":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, "/v1/system/credentials/"+hostedMCPPathTail(stringArg(args, "credential_id")), nil, false, false, false, ""), true, nil
	case "nopsai.create_credential":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPost, "/v1/system/credentials", hostedMCPCredentialCreateBody(args), boolArg(args, "confirm", false), false, true, "Creating credentials can introduce new secret material and access paths."), true, nil
	case "nopsai.rotate_credential_value":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPut, "/v1/system/credentials/"+hostedMCPPathTail(stringArg(args, "credential_id"))+"/value", map[string]any{"value": stringArg(args, "value")}, boolArg(args, "confirm", false), false, true, "Rotating credentials can immediately affect integrations that use the credential."), true, nil
	case "nopsai.activate_credential_version":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPost, "/v1/system/credentials/"+hostedMCPPathTail(stringArg(args, "credential_id"))+"/versions/"+hostedMCPPathTail(hostedMCPVersionArg(args))+"/activate", nil, boolArg(args, "confirm", false), false, true, "Activating a credential version changes which value integrations use."), true, nil
	case "nopsai.disable_credential":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPost, "/v1/system/credentials/"+hostedMCPPathTail(stringArg(args, "credential_id"))+"/disable", nil, boolArg(args, "confirm", false), false, true, "Disabling a credential can stop dependent integrations."), true, nil
	case "nopsai.enable_credential":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPost, "/v1/system/credentials/"+hostedMCPPathTail(stringArg(args, "credential_id"))+"/enable", nil, boolArg(args, "confirm", false), false, false, ""), true, nil
	case "nopsai.delete_credential_version":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodDelete, "/v1/system/credentials/"+hostedMCPPathTail(stringArg(args, "credential_id"))+"/versions/"+hostedMCPPathTail(hostedMCPVersionArg(args)), nil, boolArg(args, "confirm", false), false, true, "Deleting credential versions permanently removes recovery material."), true, nil
	case "nopsai.delete_credential":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodDelete, "/v1/system/credentials/"+hostedMCPPathTail(stringArg(args, "credential_id")), nil, boolArg(args, "confirm", false), false, true, "Deleting a credential removes its metadata and value history when no references remain."), true, nil
	case "nopsai.propose_credential_gitops":
		result, err := hostedMCPProposeCredentialGitOps(args)
		return result, true, err
	case "nopsai.generate_runner_compose":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, hostedMCPRunnerPath("/v1/system/dispatcher/runner-compose", args), nil, false, false, false, ""), true, nil
	case "nopsai.generate_kubernetes_runner_manifest":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, hostedMCPRunnerPath("/v1/system/dispatcher/kubernetes-runner-manifest", args), nil, false, false, false, ""), true, nil
	case "nopsai.generate_runner_bootstrap_command":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, hostedMCPRunnerPath("/v1/system/dispatcher/runner-bootstrap-command", args), nil, false, boolArg(args, "include_sensitive_response", false), true, "Runner bootstrap commands contain one-time bootstrap tokens."), true, nil
	case "nopsai.generate_kubernetes_runner_bootstrap_command":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, hostedMCPRunnerPath("/v1/system/dispatcher/kubernetes-runner-bootstrap-command", args), nil, false, boolArg(args, "include_sensitive_response", false), true, "Runner bootstrap commands contain one-time bootstrap tokens."), true, nil
	case "nopsai.update_runner_dispatch":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPost, "/v1/system/dispatcher/runners/"+hostedMCPPathTail(stringArg(args, "runner_id"))+"/dispatch", hostedMCPRunnerDispatchBody(args), boolArg(args, "confirm", false), false, true, "Changing runner dispatch can pause or resume work assignment for active capacity."), true, nil
	case "nopsai.list_access_grants":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, hostedMCPPathWithQuery("/v1/access/grants", args, map[string]string{}), nil, false, false, false, ""), true, nil
	case "nopsai.create_access_grant":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPost, "/v1/access/grants", hostedMCPBodyWithout(args, "confirm"), boolArg(args, "confirm", false), false, true, "Access grants change who can use or administer resources."), true, nil
	case "nopsai.delete_access_grant":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodDelete, "/v1/access/grants/"+hostedMCPPathTail(stringArg(args, "grant_id")), nil, boolArg(args, "confirm", false), false, true, "Deleting access grants can remove user or service access."), true, nil
	case "nopsai.get_effective_permissions":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, hostedMCPPathWithQuery("/v1/access/effective-permissions", args, map[string]string{}), nil, false, false, false, ""), true, nil
	case "nopsai.check_resource_use":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPost, "/v1/authz/resource-use/check", hostedMCPResourceUseBody(args), true, false, false, ""), true, nil
	case "nopsai.batch_check_resource_use":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPost, "/v1/authz/resource-use/batch-check", hostedMCPResourceUseBody(args), true, false, false, ""), true, nil
	case "nopsai.get_resource_access":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, hostedMCPResourceAccessPath(args, "/access"), nil, false, false, false, ""), true, nil
	case "nopsai.update_resource_access":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPut, hostedMCPResourceAccessPath(args, "/access"), hostedMCPResourceAccessUpdateBody(args), boolArg(args, "confirm", false), false, true, "Resource access settings affect visibility and use grants."), true, nil
	case "nopsai.create_resource_use_grant":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPost, hostedMCPResourceAccessPath(args, "/grants"), hostedMCPResourceUseGrantBody(args), boolArg(args, "confirm", false), false, true, "Resource-use grants allow callers to use protected resources."), true, nil
	case "nopsai.delete_resource_access_grant":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodDelete, hostedMCPResourceAccessPath(args, "/grants/"+hostedMCPPathTail(stringArg(args, "grant_id"))), nil, boolArg(args, "confirm", false), false, true, "Deleting a resource access grant can remove runtime access."), true, nil
	case "nopsai.list_audit_logs":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, hostedMCPPathWithQuery("/v1/audit", args, map[string]string{}), nil, false, false, false, ""), true, nil
	case "nopsai.list_admin_users":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, hostedMCPPathWithQuery("/v1/admin/users", args, map[string]string{}), nil, false, false, false, ""), true, nil
	case "nopsai.create_admin_user":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPost, "/v1/admin/users", firstNonNil(args["body"], hostedMCPBodyWithout(args, "confirm")), boolArg(args, "confirm", false), false, true, "Creating users changes tenant access."), true, nil
	case "nopsai.update_admin_user":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPatch, "/v1/admin/users/"+hostedMCPPathTail(stringArg(args, "user_id")), firstNonNil(args["body"], hostedMCPBodyWithout(args, "confirm", "user_id")), boolArg(args, "confirm", false), false, true, "Updating users can change identity, status, or access."), true, nil
	case "nopsai.delete_admin_user":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodDelete, "/v1/admin/users/"+hostedMCPPathTail(stringArg(args, "user_id")), nil, boolArg(args, "confirm", false), false, true, "Deleting users removes access and may affect audit context."), true, nil
	case "nopsai.list_admin_service_accounts":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, hostedMCPPathWithQuery("/v1/admin/service-accounts", args, map[string]string{}), nil, false, false, false, ""), true, nil
	case "nopsai.create_admin_service_account":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPost, "/v1/admin/service-accounts", firstNonNil(args["body"], hostedMCPBodyWithout(args, "confirm")), boolArg(args, "confirm", false), false, true, "Creating service accounts creates machine access paths."), true, nil
	case "nopsai.update_admin_service_account":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPatch, "/v1/admin/service-accounts/"+hostedMCPPathTail(stringArg(args, "service_account_id")), firstNonNil(args["body"], hostedMCPBodyWithout(args, "confirm", "service_account_id")), boolArg(args, "confirm", false), false, true, "Updating service accounts can change machine access."), true, nil
	case "nopsai.delete_admin_service_account":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodDelete, "/v1/admin/service-accounts/"+hostedMCPPathTail(stringArg(args, "service_account_id")), nil, boolArg(args, "confirm", false), false, true, "Deleting service accounts removes machine access."), true, nil
	case "nopsai.list_admin_roles":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, "/v1/admin/roles", nil, false, false, false, ""), true, nil
	case "nopsai.create_admin_role":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPost, "/v1/admin/roles", firstNonNil(args["body"], hostedMCPBodyWithout(args, "confirm")), boolArg(args, "confirm", false), false, true, "Creating roles changes platform authorization behavior."), true, nil
	case "nopsai.delete_admin_role":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodDelete, "/v1/admin/roles", firstNonNil(args["body"], hostedMCPBodyWithout(args, "confirm")), boolArg(args, "confirm", false), false, true, "Deleting roles can remove permissions from users or services."), true, nil
	case "nopsai.list_admin_identity_providers":
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, "/v1/admin/identity-providers", nil, false, false, false, ""), true, nil
	case "nopsai.update_admin_identity_provider":
		path := "/v1/admin/identity-providers"
		if provider := stringArg(args, "provider"); provider != "" {
			path += "/" + hostedMCPPathTail(provider)
		}
		return a.hostedMCPFinalAPITool(ctx, subject, http.MethodPut, path, firstNonNil(args["body"], hostedMCPBodyWithout(args, "confirm", "provider")), boolArg(args, "confirm", false), false, true, "Identity provider updates can alter login and entitlement sync behavior."), true, nil
	case "nopsai.get_ui_context":
		return hostedMCPUIContext(args), true, nil
	default:
		return nil, false, nil
	}
}

func hostedMCPFinalToolName(name string) bool {
	for _, tool := range hostedMCPFinalTools() {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func (a *App) hostedMCPFinalAPITool(ctx context.Context, subject aaamodel.Subject, method, path string, body any, confirm, includeSensitive, highImpact bool, highImpactNote string) map[string]any {
	callArgs := map[string]any{
		"method":                     method,
		"path":                       path,
		"confirm":                    confirm,
		"include_sensitive_response": includeSensitive,
	}
	if body != nil {
		callArgs["body"] = body
	}
	result, err := a.hostedMCPCallAPI(ctx, subject, callArgs)
	if err != nil {
		return map[string]any{"error": err.Error(), "method": method, "path": path, "applied": false}
	}
	if highImpact {
		result["high_impact"] = true
		if highImpactNote != "" {
			result["high_impact_note"] = highImpactNote
		}
	}
	return result
}

func hostedMCPSetupTemplatesPath(args map[string]any) string {
	query := map[string]string{"profile": stringArg(args, "profile")}
	if boolArg(args, "include_llm", false) {
		query["include_llm"] = "true"
	}
	if boolArg(args, "include_mcp", false) {
		query["include_mcp"] = "true"
	}
	return hostedMCPPathWithQuery("/v1/setup/templates", mergeStringQuery(args, query), map[string]string{})
}

func hostedMCPPlanFirstInstallSetup(args map[string]any) (map[string]any, error) {
	profile := normalizeSetupProfile(stringArg(args, "profile"))
	repositories := normalizeSetupRepositories(hostedMCPStringSliceArg(args, "repositories"))
	var groups []setupRepositoryGroupInput
	if err := hostedMCPDecodeObject(args["repository_groups"], &groups); err != nil {
		return nil, err
	}
	groups = normalizeSetupRepositoryGroups(groups, repositories)
	repositories = setupRepositoriesFromGroups(groups)

	var users []setupUserInput
	if err := hostedMCPDecodeObject(args["users"], &users); err != nil {
		return nil, err
	}
	var llm setupLLMProfileInput
	if err := hostedMCPDecodeObject(args["llm_profile"], &llm); err != nil {
		return nil, err
	}
	options := setupTemplateOptions{
		RepositoryGroups: groups,
		Users:            users,
		IncludeLLM:       boolArg(args, "include_llm", true),
		IncludeMCP:       boolArg(args, "include_mcp", profile != setupProfileProduction && profile != setupProfileEmpty),
		LLMProfile:       llm,
	}
	files := setupStarterTemplatesWithOptions(profile, repositories, options)
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return map[string]any{
		"proposal_type": "first_install_setup",
		"applies":       false,
		"profile":       profile,
		"workflow": []map[string]any{
			{"step": "get_setup_status", "tool": "nopsai.get_setup_status"},
			{"step": "get_setup_preflight", "tool": "nopsai.get_setup_preflight"},
			{"step": "review_gitops_templates", "tool": "nopsai.get_setup_templates"},
			{"step": "bootstrap_when_ready", "tool": "nopsai.bootstrap_first_install_setup", "requires_confirmation": true},
			{"step": "sync_config_repository", "tool": "nopsai.sync_config_repo", "when": "config_repository.enabled && sync_config_repository"},
		},
		"gitops": map[string]any{
			"files":       hostedMCPFileListFromMap(files),
			"file_paths":  paths,
			"review_note": "Review starter templates, commit them through the config repository, then run bootstrap with confirm:true when ready.",
		},
	}, nil
}

func hostedMCPFileListFromMap(files map[string]string) []map[string]any {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	out := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		out = append(out, map[string]any{"path": path, "content": files[path], "delete": false})
	}
	return out
}

func hostedMCPProposeReusableStep(args map[string]any, mode string) (map[string]any, error) {
	pathPart, namePart := hostedMCPReusableStepParts(args)
	if mode == "delete" {
		if namePart == "" {
			return nil, fmt.Errorf("step, name, or reusable step YAML name is required")
		}
		id := configsync.BuildStepIdentifier(pathPart, namePart)
		return hostedMCPDeleteFilePlan("reusable_step_delete", id, hostedMCPReusableStepGitOpsPath(pathPart, namePart), stringArg(args, "message")), nil
	}

	raw := firstNonEmptyString(stringArg(args, "yaml"), stringArg(args, "definition"))
	if raw == "" {
		return map[string]any{"proposal_type": "reusable_step_" + mode, "applies": false, "valid": false, "error": "yaml is required"}, nil
	}
	var step models.PipelineStep
	if err := yaml.Unmarshal([]byte(raw), &step); err != nil {
		return map[string]any{"proposal_type": "reusable_step_" + mode, "applies": false, "valid": false, "error": err.Error()}, nil
	}
	stepName := strings.TrimSpace(step.GetName())
	if stepName == "" {
		return map[string]any{"proposal_type": "reusable_step_" + mode, "applies": false, "valid": false, "error": "a reusable step must have a name field"}, nil
	}
	if namePart != "" && namePart != stepName {
		return map[string]any{"proposal_type": "reusable_step_" + mode, "applies": false, "valid": false, "error": fmt.Sprintf("target name %q must match YAML name %q", namePart, stepName)}, nil
	}
	id := configsync.BuildStepIdentifier(pathPart, stepName)
	return hostedMCPFilePlan("reusable_step_"+mode, id, hostedMCPReusableStepGitOpsPath(pathPart, stepName), raw, false, stringArg(args, "message")), nil
}

func hostedMCPReusableStepArgID(args map[string]any) string {
	pathPart, namePart := hostedMCPReusableStepParts(args)
	if namePart != "" {
		return configsync.BuildStepIdentifier(pathPart, namePart)
	}
	raw := firstNonEmptyString(stringArg(args, "yaml"), stringArg(args, "definition"))
	var step models.PipelineStep
	if raw != "" && yaml.Unmarshal([]byte(raw), &step) == nil && strings.TrimSpace(step.GetName()) != "" {
		return configsync.BuildStepIdentifier(pathPart, step.GetName())
	}
	return ""
}

func hostedMCPReusableStepParts(args map[string]any) (string, string) {
	if id := strings.Trim(strings.TrimSpace(firstNonEmptyString(stringArg(args, "step"), stringArg(args, "step_id"))), "/"); id != "" {
		pathPart, namePart, _, err := configsync.SplitStepIdentifier(id)
		if err == nil {
			return pathPart, namePart
		}
		parts := strings.Split(id, "/")
		if len(parts) == 1 {
			return "", parts[0]
		}
		return strings.Join(parts[:len(parts)-1], "/"), parts[len(parts)-1]
	}
	return strings.Trim(strings.TrimSpace(stringArg(args, "path")), "/"), strings.TrimSpace(stringArg(args, "name"))
}

func hostedMCPReusableStepGitOpsPath(pathPart, name string) string {
	return filepath.ToSlash(filepath.Join("steps", strings.Trim(pathPart, "/"), strings.TrimSpace(name)+".yaml"))
}

func hostedMCPNamedValueResourceID(args map[string]any, kind string) string {
	name := hostedMCPNamedValueName(args, kind)
	repo := hostedMCPRepositoryFullName(args)
	scope := stringArg(args, "scope")
	if name == "" && kind == "secret" {
		return "*"
	}
	if name == "" && kind == "variable" {
		return "*"
	}
	return aaamodel.BuildNamedResourceID(repo, scope, name)
}

func hostedMCPNamedValueName(args map[string]any, kind string) string {
	if kind == "secret" {
		return firstNonEmptyString(stringArg(args, "secret_name"), stringArg(args, "name"))
	}
	return firstNonEmptyString(stringArg(args, "variable_name"), stringArg(args, "name"))
}

func hostedMCPRepositoryFullName(args map[string]any) string {
	if repo := strings.Trim(strings.TrimSpace(stringArg(args, "repository")), "/"); repo != "" {
		return repo
	}
	owner := strings.Trim(strings.TrimSpace(firstNonEmptyString(stringArg(args, "repo_owner"), stringArg(args, "owner"))), "/")
	name := strings.Trim(strings.TrimSpace(firstNonEmptyString(stringArg(args, "repo_name"), stringArg(args, "repository_name"))), "/")
	if owner == "" || name == "" {
		return ""
	}
	return owner + "/" + name
}

func hostedMCPSecretVariablePath(args map[string]any, section, mode string) string {
	nameKind := "variable"
	if section == "secrets" {
		nameKind = "secret"
	}
	name := hostedMCPNamedValueName(args, nameKind)
	repo := hostedMCPRepositoryFullName(args)
	base := "/v1/" + section
	if repo != "" {
		parts := strings.SplitN(repo, "/", 2)
		if len(parts) == 2 {
			base = "/v1/repositories/" + hostedMCPPathTail(parts[0]) + "/" + hostedMCPPathTail(parts[1]) + "/" + section
		}
	}
	if mode == "item" {
		base += "/" + hostedMCPPathTail(name)
	}
	query := map[string]string{"include_source": "true"}
	if scope := stringArg(args, "scope"); scope != "" {
		query["scope"] = scope
	}
	return hostedMCPPathWithQuery(base, mergeStringQuery(nil, query), map[string]string{})
}

func hostedMCPProposeScopedValueGitOps(args map[string]any, section, mode string) (map[string]any, error) {
	valueKind := "variable"
	valueKey := "value"
	if section == "secrets" {
		valueKind = "secret"
		valueKey = "encrypted_value"
	}
	name := hostedMCPNamedValueName(args, valueKind)
	if name == "" {
		return nil, fmt.Errorf("%s name is required", valueKind)
	}
	scope := strings.Trim(strings.TrimSpace(stringArg(args, "scope")), "/")
	if scope == "" {
		scope = defaultRuntimeScope
	}
	repo := hostedMCPRepositoryFullName(args)
	entryKey := name
	if repo != "" {
		entryKey = strings.Trim(repo, "/") + "/" + name
	}
	path := filepath.ToSlash(filepath.Join("scopes", scope, "scope.yaml"))
	id := aaamodel.BuildNamedResourceID(repo, scope, name)
	if mode == "delete" {
		return map[string]any{
			"proposal_type": section + "_gitops_delete",
			"applies":       false,
			"valid":         true,
			"id":            id,
			"gitops": map[string]any{
				"message": firstNonEmptyString(stringArg(args, "message"), "Remove NopsAI "+valueKind+" "+name),
				"files": []map[string]any{{
					"path":       path,
					"delete":     false,
					"remove_key": []string{section, entryKey},
				}},
				"review_note": "Remove this key from the existing scope file, then sync GitOps. The assistant does not delete the entire scope file for a single value.",
			},
		}, nil
	}
	value := stringArg(args, valueKey)
	if value == "" {
		return map[string]any{"proposal_type": section + "_gitops_write", "applies": false, "valid": false, "error": valueKey + " is required"}, nil
	}
	doc := map[string]any{section: map[string]any{entryKey: value}}
	content, err := marshalConfigRepositoryYAML(doc)
	if err != nil {
		return nil, err
	}
	return hostedMCPFilePlan(section+"_gitops_write", id, path, string(content), false, stringArg(args, "message")), nil
}

func hostedMCPExplainInternalRunOperations(args map[string]any) map[string]any {
	return map[string]any{
		"applies": false,
		"policy":  "internal_run_service_operations_are_not_assistant_mutations",
		"run_id":  stringArg(args, "run_id"),
		"excluded_routes": []string{
			"POST /v1/runs/{runID}/logs/ingest",
			"POST /v1/runs/{runID}/steps/{stepName}/tasks/{taskName}",
			"POST /v1/runs/{runID}/finalize",
			"POST /v1/internal/runs/{runID}/approvals/pause",
		},
		"safe_assistant_tools": []string{
			"nopsai.get_pipeline_run",
			"nopsai.get_pipeline_run_logs",
			"nopsai.analyze_pipeline_run_failure",
			"nopsai.cancel_pipeline_run",
			"nopsai.rerun_pipeline_run",
		},
		"reason": "These callbacks are owned by runners and internal services. Exposing them as assistant tools would allow fabricated run state, logs, or finalization.",
	}
}

func hostedMCPExplainWebhookIngressPolicy(args map[string]any) map[string]any {
	return map[string]any{
		"applies":    false,
		"policy":     "public_webhook_delivery_ingress_is_not_an_assistant_mutation",
		"source_id":  stringArg(args, "source_id"),
		"repository": stringArg(args, "repository"),
		"excluded_routes": []string{
			"POST /v1/git/events",
			"POST /v1/git/webhooks/{sourceID}",
		},
		"safe_assistant_tools": []string{
			"nopsai.list_git_webhook_sources",
			"nopsai.get_git_webhook_source",
			"nopsai.list_git_webhook_deliveries",
			"nopsai.propose_git_webhook_source_create",
			"nopsai.propose_git_webhook_source_update",
			"nopsai.propose_git_webhook_source_delete",
			"nopsai.invoke_external_trigger",
		},
		"reason": "Webhook ingress verifies external provider signatures and delivery semantics. Assistant-triggered work should use external trigger invocation or GitOps source configuration instead.",
	}
}

func hostedMCPMonitoringAnalyticsPath(toolName string, args map[string]any) string {
	base := hostedMCPMonitoringAnalyticsBasePath(toolName)
	return hostedMCPPathWithQuery(base, args, hostedMCPMonitoringAnalyticsAliases())
}

func hostedMCPMonitoringAnalyticsBasePath(toolName string) string {
	return map[string]string{
		"nopsai.get_monitoring_summary":                    "/v1/monitoring/summary",
		"nopsai.get_monitoring_run_analytics":              "/v1/monitoring/runs/analytics",
		"nopsai.get_monitoring_pipeline_performance":       "/v1/monitoring/pipelines/performance",
		"nopsai.get_monitoring_step_performance":           "/v1/monitoring/steps/performance",
		"nopsai.get_monitoring_task_performance":           "/v1/monitoring/tasks/performance",
		"nopsai.get_monitoring_trigger_analytics":          "/v1/monitoring/triggers/analytics",
		"nopsai.get_monitoring_external_trigger_analytics": "/v1/monitoring/external-triggers/analytics",
		"nopsai.get_monitoring_ai_usage":                   "/v1/monitoring/ai-usage",
		"nopsai.get_monitoring_reliability":                "/v1/monitoring/reliability",
		"nopsai.get_monitoring_efficiency":                 "/v1/monitoring/efficiency",
		"nopsai.get_monitoring_security":                   "/v1/monitoring/security",
		"nopsai.get_monitoring_runner_history":             "/v1/monitoring/runners/history",
		"nopsai.get_monitoring_schedule_ai_usage":          "/v1/monitoring/ai-usage",
	}[toolName]
}

func hostedMCPMonitoringAnalyticsAliases() map[string]string {
	return map[string]string{
		"group_id":               "groupId",
		"pipeline_path":          "pipelinePath",
		"pipeline_name":          "pipelineName",
		"run_id":                 "runId",
		"commit_sha":             "commitSHA",
		"trigger_source":         "triggerSource",
		"requested_by_type":      "requestedByType",
		"requested_by_id":        "requestedById",
		"effective_subject_type": "effectiveSubjectType",
		"effective_subject_id":   "effectiveSubjectId",
		"external_trigger_id":    "externalTriggerId",
		"schedule_id":            "scheduleId",
		"llm_profile":            "llmProfile",
		"step_name":              "stepName",
		"task_name":              "taskName",
		"min_duration_seconds":   "minDurationSeconds",
		"max_duration_seconds":   "maxDurationSeconds",
	}
}

func (a *App) hostedMCPMonitoringInsightTool(ctx context.Context, subject aaamodel.Subject, toolName string, args map[string]any) map[string]any {
	paths := hostedMCPMonitoringInsightPaths(toolName)
	result := map[string]any{
		"analysis":     strings.TrimPrefix(toolName, "nopsai."),
		"applied":      false,
		"combined":     true,
		"source_count": len(paths),
		"ok":           true,
	}
	sourcePaths := make([]string, 0, len(paths))
	for _, path := range paths {
		requestPath := hostedMCPPathWithQuery(path.Path, args, hostedMCPMonitoringAnalyticsAliases())
		sourcePaths = append(sourcePaths, requestPath)
		part := a.hostedMCPFinalAPITool(ctx, subject, http.MethodGet, requestPath, nil, false, false, false, "")
		if _, hasError := part["error"]; hasError {
			result["ok"] = false
		}
		if ok, hasOK := part["ok"].(bool); hasOK && !ok {
			result["ok"] = false
		}
		result[path.Key] = part
	}
	result["source_paths"] = sourcePaths
	return result
}

type hostedMCPMonitoringInsightPath struct {
	Key  string
	Path string
}

func hostedMCPMonitoringInsightPaths(toolName string) []hostedMCPMonitoringInsightPath {
	switch toolName {
	case "nopsai.get_monitoring_schedule_performance":
		return []hostedMCPMonitoringInsightPath{
			{Key: "summary", Path: "/v1/monitoring/summary"},
			{Key: "ai_usage", Path: "/v1/monitoring/ai-usage"},
			{Key: "efficiency", Path: "/v1/monitoring/efficiency"},
		}
	case "nopsai.get_monitoring_trigger_performance":
		return []hostedMCPMonitoringInsightPath{
			{Key: "trigger_analytics", Path: "/v1/monitoring/triggers/analytics"},
			{Key: "external_trigger_analytics", Path: "/v1/monitoring/external-triggers/analytics"},
			{Key: "summary", Path: "/v1/monitoring/summary"},
		}
	case "nopsai.get_pipeline_efficiency":
		return []hostedMCPMonitoringInsightPath{
			{Key: "pipeline_performance", Path: "/v1/monitoring/pipelines/performance"},
			{Key: "efficiency", Path: "/v1/monitoring/efficiency"},
			{Key: "ai_usage", Path: "/v1/monitoring/ai-usage"},
		}
	case "nopsai.compare_pipelines":
		return []hostedMCPMonitoringInsightPath{
			{Key: "pipeline_performance", Path: "/v1/monitoring/pipelines/performance"},
			{Key: "efficiency", Path: "/v1/monitoring/efficiency"},
			{Key: "reliability", Path: "/v1/monitoring/reliability"},
			{Key: "ai_usage", Path: "/v1/monitoring/ai-usage"},
		}
	case "nopsai.compare_schedules":
		return []hostedMCPMonitoringInsightPath{
			{Key: "ai_usage", Path: "/v1/monitoring/ai-usage"},
			{Key: "efficiency", Path: "/v1/monitoring/efficiency"},
			{Key: "summary", Path: "/v1/monitoring/summary"},
		}
	case "nopsai.explain_pipeline_health":
		return []hostedMCPMonitoringInsightPath{
			{Key: "pipeline_performance", Path: "/v1/monitoring/pipelines/performance"},
			{Key: "reliability", Path: "/v1/monitoring/reliability"},
			{Key: "efficiency", Path: "/v1/monitoring/efficiency"},
			{Key: "security", Path: "/v1/monitoring/security"},
		}
	case "nopsai.find_optimization_opportunities":
		return []hostedMCPMonitoringInsightPath{
			{Key: "efficiency", Path: "/v1/monitoring/efficiency"},
			{Key: "recommendations", Path: "/v1/monitoring/recommendations"},
			{Key: "ai_usage", Path: "/v1/monitoring/ai-usage"},
			{Key: "pipeline_performance", Path: "/v1/monitoring/pipelines/performance"},
		}
	default:
		return nil
	}
}

func hostedMCPCredentialCreateBody(args map[string]any) map[string]any {
	body := map[string]any{
		"reference":   stringArg(args, "reference"),
		"kind":        stringArg(args, "kind"),
		"description": stringArg(args, "description"),
	}
	if value := stringArg(args, "value"); value != "" {
		body["value"] = value
	}
	if expiresAt := stringArg(args, "expires_at"); expiresAt != "" {
		body["expires_at"] = expiresAt
	}
	return body
}

func hostedMCPProposeCredentialGitOps(args map[string]any) (map[string]any, error) {
	var item gitOpsCredential
	if err := hostedMCPDecodeObject(firstNonNil(args["credential"], args["body"]), &item); err != nil {
		return nil, err
	}
	if value := stringArg(args, "reference"); value != "" {
		item.Reference = value
	}
	if value := stringArg(args, "kind"); value != "" {
		item.Kind = value
	}
	if value := stringArg(args, "description"); value != "" {
		item.Description = value
	}
	if value := stringArg(args, "status"); value != "" {
		item.Status = value
	}
	if value := intArg(args, "active_version", 0, 0); value > 0 {
		item.ActiveVersion = value
	}
	if value := stringArg(args, "expires_at"); value != "" {
		parsed, err := hostedMCPParseTime(value)
		if err != nil {
			return map[string]any{"proposal_type": "credential_gitops", "applies": false, "valid": false, "error": err.Error()}, nil
		}
		item.ExpiresAt = &parsed
	}
	if rawVersions := args["versions"]; rawVersions != nil {
		var versions []gitOpsCredentialValue
		if err := hostedMCPDecodeObject(rawVersions, &versions); err != nil {
			return nil, err
		}
		item.Versions = versions
	}
	ref, err := credentials.ParseReference(item.Reference)
	if err != nil {
		return map[string]any{"proposal_type": "credential_gitops", "applies": false, "valid": false, "error": err.Error()}, nil
	}
	kind, err := normalizeCredentialKind(item.Kind)
	if err != nil {
		return map[string]any{"proposal_type": "credential_gitops", "applies": false, "valid": false, "error": err.Error()}, nil
	}
	item.Reference = ref.String()
	item.Kind = kind
	item.Status = normalizeCredentialGitOpsStatus(item.Status, item.ActiveVersion)
	if item.ActiveVersion == 0 && len(item.Versions) > 0 {
		item.ActiveVersion = item.Versions[0].Version
		item.Status = normalizeCredentialGitOpsStatus(item.Status, item.ActiveVersion)
	}
	doc := gitOpsCredentialFile{Credentials: []gitOpsCredential{item}}
	content, err := marshalConfigRepositoryYAML(doc)
	if err != nil {
		return nil, err
	}
	if _, err := parseGitOpsCredentialFile(string(content), configRepositoryCredentialsPath); err != nil {
		return map[string]any{"proposal_type": "credential_gitops", "applies": false, "valid": false, "error": err.Error()}, nil
	}
	return hostedMCPFilePlan("credential_gitops", ref.String(), configRepositoryCredentialsPath, string(content), false, stringArg(args, "message")), nil
}

func hostedMCPParseTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed, nil
	}
	if parsed, err := time.Parse("2006-01-02", raw); err == nil {
		return parsed, nil
	}
	return time.Time{}, fmt.Errorf("invalid expires_at %q", raw)
}

func hostedMCPCredentialReferenceResourceID(args map[string]any) string {
	ref := stringArg(args, "reference")
	if ref == "" {
		var item gitOpsCredential
		_ = hostedMCPDecodeObject(firstNonNil(args["credential"], args["body"]), &item)
		ref = item.Reference
	}
	parsed, err := credentials.ParseReference(ref)
	if err != nil {
		return ""
	}
	return parsed.ResourceID()
}

func hostedMCPVersionArg(args map[string]any) string {
	if value := stringArg(args, "version"); value != "" {
		return value
	}
	if value := intArg(args, "version", 0, 0); value > 0 {
		return fmt.Sprint(value)
	}
	return ""
}

func hostedMCPRunnerPath(base string, args map[string]any) string {
	aliases := map[string]string{}
	return hostedMCPPathWithQuery(base, args, aliases)
}

func hostedMCPRunnerDispatchBody(args map[string]any) map[string]any {
	return map[string]any{
		"allow_dispatch": boolArg(args, "allow_dispatch", false),
		"connection_id":  stringArg(args, "connection_id"),
	}
}

func hostedMCPResourceUseBody(args map[string]any) any {
	if body := args["body"]; body != nil {
		return body
	}
	return hostedMCPBodyWithout(args, "confirm")
}

func hostedMCPResourceAccessPath(args map[string]any, suffix string) string {
	return "/v1/resources/" + hostedMCPPathTail(stringArg(args, "resource_type")) + "/" + hostedMCPPathTail(stringArg(args, "resource_id")) + suffix
}

func hostedMCPResourceAccessUpdateBody(args map[string]any) any {
	if body := args["body"]; body != nil {
		return body
	}
	body := map[string]any{}
	if value := stringArg(args, "visibility"); value != "" {
		body["visibility"] = value
	}
	if value := args["use_access"]; value != nil {
		body["use_access"] = value
	}
	return body
}

func hostedMCPResourceUseGrantBody(args map[string]any) any {
	if body := args["body"]; body != nil {
		return body
	}
	body := map[string]any{
		"subject_type": stringArg(args, "subject_type"),
		"subject_id":   stringArg(args, "subject_id"),
	}
	if actions := args["actions"]; actions != nil {
		body["actions"] = actions
	}
	if conditions := args["conditions"]; conditions != nil {
		body["conditions"] = conditions
	}
	return body
}

func hostedMCPUIContext(args map[string]any) map[string]any {
	area := strings.ToLower(strings.TrimSpace(stringArg(args, "area")))
	surfaces := []map[string]any{
		{"area": "assistant", "owner": "ui", "mcp_role": "data, proposals, and tool execution results", "routes": []string{"/assistant", "/v1/assistant/*", "/v1/mcp"}},
		{"area": "pipelines", "owner": "ui", "mcp_role": "pipeline definitions, run state, GitOps plans, and knowledge context references", "routes": []string{"/pipelines", "/runs"}},
		{"area": "monitoring", "owner": "ui", "mcp_role": "saved views, alert rules, recommendations, and analytics payloads", "routes": []string{"/monitoring"}},
		{"area": "setup", "owner": "ui", "mcp_role": "guided workflow data and confirmed bootstrap execution", "routes": []string{"/setup"}},
		{"area": "admin", "owner": "ui", "mcp_role": "AAA, audit, credential, and system data with confirmation gates", "routes": []string{"/admin", "/settings"}},
	}
	if area != "" {
		filtered := surfaces[:0]
		for _, surface := range surfaces {
			if surface["area"] == area {
				filtered = append(filtered, surface)
			}
		}
		surfaces = filtered
	}
	return map[string]any{
		"applies":      false,
		"policy":       "mcp_provides_context_data_and_plans_ui_owns_rendering",
		"rendering":    "intentionally_excluded_from_mcp_mutation",
		"surfaces":     surfaces,
		"review_note":  "Frontend route composition and rendering remain in the UI. MCP tools expose data, proposals, and confirmed operations for the current subject.",
		"gitops_ready": true,
		"current_user": "authorization is evaluated through the current hosted MCP subject",
	}
}

func hostedMCPBodyWithout(args map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	exclude := map[string]struct{}{}
	for _, key := range keys {
		exclude[key] = struct{}{}
	}
	for key, value := range args {
		if _, skip := exclude[key]; skip {
			continue
		}
		out[key] = value
	}
	return out
}

func mergeStringQuery(args map[string]any, values map[string]string) map[string]any {
	out := map[string]any{}
	for key, value := range args {
		out[key] = value
	}
	for key, value := range values {
		if strings.TrimSpace(value) != "" {
			out[key] = value
		}
	}
	return out
}

func hostedMCPPathWithQuery(base string, args map[string]any, aliases map[string]string) string {
	values := url.Values{}
	if query := hostedMCPMapArg(args, "query"); len(query) > 0 {
		for key, value := range query {
			hostedMCPAddQueryValue(values, key, value)
		}
	}
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key == "query" || key == "body" || key == "confirm" || key == "include_sensitive_response" {
			continue
		}
		value := args[key]
		if value == nil {
			continue
		}
		queryKey := key
		if alias := aliases[key]; alias != "" {
			queryKey = alias
		}
		hostedMCPAddQueryValue(values, queryKey, value)
	}
	if len(values) == 0 {
		return base
	}
	return base + "?" + values.Encode()
}

func hostedMCPAddQueryValue(values url.Values, key string, value any) {
	key = strings.TrimSpace(key)
	if key == "" || value == nil {
		return
	}
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				values.Add(key, trimmed)
			}
		}
	case []any:
		for _, item := range typed {
			if trimmed := strings.TrimSpace(fmt.Sprint(item)); trimmed != "" {
				values.Add(key, trimmed)
			}
		}
	case map[string]any:
		return
	default:
		if trimmed := strings.TrimSpace(fmt.Sprint(typed)); trimmed != "" {
			values.Set(key, trimmed)
		}
	}
}

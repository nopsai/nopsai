package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	aaamodel "nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
)

func hostedMCPDedicatedTools() []hostedMCPTool {
	return []hostedMCPTool{
		toolDef("nopsai.run_pipeline", "Start a pipeline run as the current subject. Requires confirm:true.", "pipeline.execute", "pipeline", "*", objectSchema(map[string]any{"pipeline": stringSchema(), "path": stringSchema(), "name": stringSchema(), "scope": stringSchema(), "variables": objectSchema(map[string]any{}), "definition": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.list_run_approvals", "List approvals for a pipeline run when the current subject can read or approve the run.", "pipeline_run.read", "pipeline_run", "*", objectSchema(map[string]any{"run_id": stringSchema()})),
		toolDef("nopsai.approve_run_approval", "Approve a pending run approval as the current subject. Requires confirm:true.", "approval.approve", "folder", "*", objectSchema(map[string]any{"run_id": stringSchema(), "approval_id": stringSchema(), "comment": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.reject_run_approval", "Reject a pending run approval as the current subject. Requires confirm:true.", "approval.approve", "folder", "*", objectSchema(map[string]any{"run_id": stringSchema(), "approval_id": stringSchema(), "comment": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.rerun_pipeline_run", "Rerun a completed pipeline run. Requires confirm:true.", "pipeline_run.rerun", "pipeline_run", "*", objectSchema(map[string]any{"run_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.cancel_pipeline_run", "Cancel an in-progress pipeline run. Requires confirm:true.", "pipeline_run.cancel", "pipeline_run", "*", objectSchema(map[string]any{"run_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.delete_pipeline_run", "Delete a pipeline run. Requires confirm:true.", "pipeline_run.delete", "pipeline_run", "*", objectSchema(map[string]any{"run_id": stringSchema(), "confirm": booleanSchema()})),

		toolDef("nopsai.propose_schedule_create", "Return a GitOps-ready schedule create file plan without applying changes.", "pipeline_schedule.create", "pipeline_schedule", "*", objectSchema(map[string]any{"schedule": objectSchema(map[string]any{}), "path": stringSchema(), "name": stringSchema(), "pipeline": stringSchema(), "schedule_kind": stringSchema(), "cron_expression": stringSchema(), "run_at": stringSchema(), "timezone": stringSchema(), "enabled": booleanSchema(), "scope": stringSchema(), "run_group_path": stringSchema(), "variables": objectSchema(map[string]any{}), "message": stringSchema()})),
		toolDef("nopsai.propose_schedule_update", "Return a GitOps-ready schedule update file plan without applying changes.", "pipeline_schedule.update", "pipeline_schedule", "*", objectSchema(map[string]any{"schedule_id": stringSchema(), "schedule": objectSchema(map[string]any{}), "path": stringSchema(), "name": stringSchema(), "pipeline": stringSchema(), "schedule_kind": stringSchema(), "cron_expression": stringSchema(), "run_at": stringSchema(), "timezone": stringSchema(), "enabled": booleanSchema(), "scope": stringSchema(), "run_group_path": stringSchema(), "variables": objectSchema(map[string]any{}), "message": stringSchema()})),
		toolDef("nopsai.propose_schedule_delete", "Return a GitOps-ready schedule delete file plan without applying changes.", "pipeline_schedule.delete", "pipeline_schedule", "*", objectSchema(map[string]any{"schedule_id": stringSchema(), "path": stringSchema(), "name": stringSchema(), "message": stringSchema()})),
		toolDef("nopsai.propose_schedule_enable", "Return a GitOps-ready schedule enable file plan without applying changes.", "pipeline_schedule.update", "pipeline_schedule", "*", objectSchema(map[string]any{"schedule_id": stringSchema(), "message": stringSchema()})),
		toolDef("nopsai.propose_schedule_disable", "Return a GitOps-ready schedule disable file plan without applying changes.", "pipeline_schedule.update", "pipeline_schedule", "*", objectSchema(map[string]any{"schedule_id": stringSchema(), "message": stringSchema()})),
		toolDef("nopsai.run_schedule_now", "Run a schedule immediately as the current subject. Requires confirm:true.", "pipeline_schedule.execute", "pipeline_schedule", "*", objectSchema(map[string]any{"schedule_id": stringSchema(), "confirm": booleanSchema()})),

		toolDef("nopsai.propose_knowledge_context_create", "Return a GitOps-ready knowledge context create file plan without applying changes.", "knowledge_context.create", "knowledge_context", "*", objectSchema(map[string]any{"kind": stringSchema(), "group": stringSchema(), "name": stringSchema(), "description": stringSchema(), "content": stringSchema(), "message": stringSchema()})),
		toolDef("nopsai.propose_knowledge_context_update", "Return a GitOps-ready knowledge context update file plan without applying changes.", "knowledge_context.update", "knowledge_context", "*", objectSchema(map[string]any{"id": stringSchema(), "kind": stringSchema(), "group": stringSchema(), "name": stringSchema(), "description": stringSchema(), "content": stringSchema(), "message": stringSchema()})),
		toolDef("nopsai.propose_knowledge_context_delete", "Return a GitOps-ready knowledge context delete file plan without applying changes.", "knowledge_context.delete", "knowledge_context", "*", objectSchema(map[string]any{"id": stringSchema(), "kind": stringSchema(), "group": stringSchema(), "name": stringSchema(), "message": stringSchema()})),

		toolDef("nopsai.list_git_webhook_sources", "List Git webhook sources visible to the current subject.", "git_webhook_source.read", "git_webhook_source", "*", objectSchema(map[string]any{})),
		toolDef("nopsai.get_git_webhook_source", "Read a Git webhook source.", "git_webhook_source.read", "git_webhook_source", "*", objectSchema(map[string]any{"source_id": stringSchema()})),
		toolDef("nopsai.list_git_webhook_deliveries", "List recent deliveries for a Git webhook source.", "git_webhook_source.read", "git_webhook_source", "*", objectSchema(map[string]any{"source_id": stringSchema()})),
		toolDef("nopsai.propose_git_webhook_source_create", "Return a GitOps-ready Git webhook source create file plan without applying changes.", "git_webhook_source.create", "git_webhook_source", "*", objectSchema(map[string]any{"source": objectSchema(map[string]any{}), "id": stringSchema(), "name": stringSchema(), "provider": stringSchema(), "auth_mode": stringSchema(), "credential_ref": stringSchema(), "enabled": booleanSchema(), "repository_allowlist": objectSchema(map[string]any{}), "message": stringSchema()})),
		toolDef("nopsai.propose_git_webhook_source_update", "Return a GitOps-ready Git webhook source update file plan without applying changes.", "git_webhook_source.update", "git_webhook_source", "*", objectSchema(map[string]any{"source_id": stringSchema(), "source": objectSchema(map[string]any{}), "message": stringSchema()})),
		toolDef("nopsai.propose_git_webhook_source_delete", "Return a GitOps-ready Git webhook source delete file plan without applying changes.", "git_webhook_source.delete", "git_webhook_source", "*", objectSchema(map[string]any{"source_id": stringSchema(), "message": stringSchema()})),

		toolDef("nopsai.list_external_triggers", "List external triggers visible to the current subject.", "external_trigger.read", "external_trigger", "*", objectSchema(map[string]any{})),
		toolDef("nopsai.get_external_trigger", "Read an external trigger.", "external_trigger.read", "external_trigger", "*", objectSchema(map[string]any{"trigger_id": stringSchema()})),
		toolDef("nopsai.list_external_trigger_invocations", "List recent invocations for an external trigger.", "external_trigger.read", "external_trigger", "*", objectSchema(map[string]any{"trigger_id": stringSchema()})),
		toolDef("nopsai.propose_external_trigger_create", "Return a GitOps-ready external trigger create file plan without applying changes.", "external_trigger.create", "external_trigger", "*", objectSchema(map[string]any{"trigger": objectSchema(map[string]any{}), "id": stringSchema(), "name": stringSchema(), "pipeline": stringSchema(), "enabled": booleanSchema(), "scope": stringSchema(), "run_group_path": stringSchema(), "message": stringSchema()})),
		toolDef("nopsai.propose_external_trigger_update", "Return a GitOps-ready external trigger update file plan without applying changes.", "external_trigger.update", "external_trigger", "*", objectSchema(map[string]any{"trigger_id": stringSchema(), "trigger": objectSchema(map[string]any{}), "message": stringSchema()})),
		toolDef("nopsai.propose_external_trigger_delete", "Return a GitOps-ready external trigger delete file plan without applying changes.", "external_trigger.delete", "external_trigger", "*", objectSchema(map[string]any{"trigger_id": stringSchema(), "message": stringSchema()})),
		toolDef("nopsai.invoke_external_trigger", "Invoke an external trigger as the current subject. Requires confirm:true.", "external_trigger.read", "external_trigger", "*", objectSchema(map[string]any{"trigger_id": stringSchema(), "event_type": stringSchema(), "idempotency_key": stringSchema(), "variables": objectSchema(map[string]any{}), "payload": objectSchema(map[string]any{}), "confirm": booleanSchema()})),

		toolDef("nopsai.get_config_sync_status", "Read system config sync status.", "system.read", "system", "config-sync", objectSchema(map[string]any{})),
		toolDef("nopsai.sync_system_config", "Run system config sync. Requires confirm:true.", "system.update", "system", "config-sync", objectSchema(map[string]any{"confirm": booleanSchema()})),
		toolDef("nopsai.get_config_repo", "Read the global or folder config repository binding.", "system.read", "system", "config-repos", objectSchema(map[string]any{"folder_id": stringSchema()})),
		toolDef("nopsai.get_config_repo_drift", "Read global or folder config repository drift.", "system.read", "system", "config-repos", objectSchema(map[string]any{"folder_id": stringSchema()})),
		toolDef("nopsai.sync_config_repo", "Sync the global or folder config repository. Requires confirm:true.", "system.update", "system", "config-repos", objectSchema(map[string]any{"folder_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.write_config_repo", "Write files to the global or folder config repository through the existing GitOps workflow. Requires confirm:true.", "system.update", "system", "config-repos", objectSchema(map[string]any{"folder_id": stringSchema(), "body": objectSchema(map[string]any{}), "files": objectSchema(map[string]any{}), "commit_message": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.list_config_repos", "List configured config repositories.", "system.read", "system", "config-repos", objectSchema(map[string]any{})),
		toolDef("nopsai.sync_all_config_repos", "Sync all configured config repositories. Requires confirm:true.", "system.update", "system", "config-repos", objectSchema(map[string]any{"confirm": booleanSchema()})),

		toolDef("nopsai.get_notification_mail_settings", "Read notification mail settings.", "system.read", "system", "notifications", objectSchema(map[string]any{})),
		toolDef("nopsai.propose_notification_mail_settings", "Return a GitOps-ready notification mail settings file plan without applying changes.", "system.update", "system", "notifications", objectSchema(map[string]any{"settings": objectSchema(map[string]any{}), "enabled": booleanSchema(), "from": stringSchema(), "smtp": objectSchema(map[string]any{}), "message": stringSchema()})),
		toolDef("nopsai.test_notification_mail_settings", "Send a test notification email. Requires confirm:true.", "system.update", "system", "notifications", objectSchema(map[string]any{"to": stringSchema(), "subject": stringSchema(), "body": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.get_notification_route", "Read a folder notification route.", "config_repo.read", "folder", "*", objectSchema(map[string]any{"folder_id": stringSchema()})),
		toolDef("nopsai.propose_notification_route_update", "Return a GitOps-ready notification route file plan without applying changes.", "config_repo.manage", "folder", "*", objectSchema(map[string]any{"folder_id": stringSchema(), "group_path": stringSchema(), "definition": objectSchema(map[string]any{}), "message": stringSchema()})),
		toolDef("nopsai.propose_notification_route_delete", "Return a GitOps-ready notification route delete file plan without applying changes.", "config_repo.manage", "folder", "*", objectSchema(map[string]any{"folder_id": stringSchema(), "group_path": stringSchema(), "message": stringSchema()})),

		toolDef("nopsai.list_monitoring_views", "List monitoring saved views visible to the current subject.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{})),
		toolDef("nopsai.create_monitoring_view", "Create a monitoring saved view for the current subject. Requires confirm:true.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{"view": objectSchema(map[string]any{}), "confirm": booleanSchema()})),
		toolDef("nopsai.update_monitoring_view", "Update a monitoring saved view owned by or visible to the current subject. Requires confirm:true.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{"view_id": stringSchema(), "view": objectSchema(map[string]any{}), "confirm": booleanSchema()})),
		toolDef("nopsai.delete_monitoring_view", "Delete a monitoring saved view. Requires confirm:true.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{"view_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.list_monitoring_alert_rules", "List monitoring alert rules visible to the current subject.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{})),
		toolDef("nopsai.create_monitoring_alert_rule", "Create a monitoring alert rule for the current subject. Requires confirm:true.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{"rule": objectSchema(map[string]any{}), "confirm": booleanSchema()})),
		toolDef("nopsai.update_monitoring_alert_rule", "Update a monitoring alert rule. Requires confirm:true.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{"rule_id": stringSchema(), "rule": objectSchema(map[string]any{}), "confirm": booleanSchema()})),
		toolDef("nopsai.delete_monitoring_alert_rule", "Delete a monitoring alert rule. Requires confirm:true.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{"rule_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.evaluate_monitoring_alert_rule", "Evaluate a monitoring alert rule immediately. Requires confirm:true.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{"rule_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.list_monitoring_alert_events", "List monitoring alert events visible to the current subject.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{})),
		toolDef("nopsai.list_monitoring_recommendations", "List monitoring recommendations.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{})),
		toolDef("nopsai.acknowledge_monitoring_recommendation", "Acknowledge a monitoring recommendation. Requires confirm:true.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{"recommendation_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.resolve_monitoring_recommendation", "Resolve a monitoring recommendation. Requires confirm:true.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{"recommendation_id": stringSchema(), "confirm": booleanSchema()})),

		toolDef("nopsai.list_data_backups", "List data backups.", "system.read", "system", "config", objectSchema(map[string]any{})),
		toolDef("nopsai.create_data_backup", "Create a data backup. High-impact operation requiring confirm:true.", "system.update", "system", "config", objectSchema(map[string]any{"backup_type": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.delete_data_backup", "Delete a data backup. High-impact operation requiring confirm:true.", "system.update", "system", "config", objectSchema(map[string]any{"backup_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.preview_data_cleanup", "Preview data cleanup impact without deleting data.", "system.read", "system", "config", objectSchema(map[string]any{"target": stringSchema(), "mode": stringSchema(), "keep_last": numberSchema(), "older_than_days": numberSchema(), "backup_before_cleanup": booleanSchema()})),
		toolDef("nopsai.run_data_cleanup", "Run data cleanup. High-impact operation requiring confirm:true.", "system.update", "system", "config", objectSchema(map[string]any{"target": stringSchema(), "mode": stringSchema(), "keep_last": numberSchema(), "older_than_days": numberSchema(), "backup_before_cleanup": booleanSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.list_data_cleanup_jobs", "List data cleanup jobs.", "system.read", "system", "config", objectSchema(map[string]any{})),
		toolDef("nopsai.list_data_cleanup_schedules", "List data cleanup schedules.", "system.read", "system", "config", objectSchema(map[string]any{})),
		toolDef("nopsai.create_data_cleanup_schedule", "Create a data cleanup schedule. Requires confirm:true.", "system.update", "system", "config", objectSchema(map[string]any{"schedule": objectSchema(map[string]any{}), "confirm": booleanSchema()})),
		toolDef("nopsai.update_data_cleanup_schedule", "Update a data cleanup schedule. Requires confirm:true.", "system.update", "system", "config", objectSchema(map[string]any{"schedule_id": stringSchema(), "schedule": objectSchema(map[string]any{}), "confirm": booleanSchema()})),
		toolDef("nopsai.delete_data_cleanup_schedule", "Delete a data cleanup schedule. High-impact operation requiring confirm:true.", "system.update", "system", "config", objectSchema(map[string]any{"schedule_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.run_data_cleanup_schedule", "Run a data cleanup schedule now. High-impact operation requiring confirm:true.", "system.update", "system", "config", objectSchema(map[string]any{"schedule_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.enable_data_cleanup_schedule", "Enable a data cleanup schedule. Requires confirm:true.", "system.update", "system", "config", objectSchema(map[string]any{"schedule_id": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.disable_data_cleanup_schedule", "Disable a data cleanup schedule. Requires confirm:true.", "system.update", "system", "config", objectSchema(map[string]any{"schedule_id": stringSchema(), "confirm": booleanSchema()})),
	}
}

func (a *App) authorizeHostedMCPDedicatedToolCall(ctx context.Context, subject aaamodel.Subject, tool hostedMCPTool, args map[string]any) (bool, error) {
	permission := hostedMCPToolPermission(tool)
	switch tool.Name {
	case "nopsai.run_pipeline":
		permission.Resource.ID = hostedMCPPipelineIDFromArgs(args)
	case "nopsai.list_run_approvals":
		permission.Resource.ID = stringArg(args, "run_id")
	case "nopsai.approve_run_approval", "nopsai.reject_run_approval":
		return true, nil
	case "nopsai.rerun_pipeline_run", "nopsai.cancel_pipeline_run", "nopsai.delete_pipeline_run":
		permission.Resource.ID = stringArg(args, "run_id")
	case "nopsai.run_schedule_now", "nopsai.propose_schedule_update", "nopsai.propose_schedule_delete", "nopsai.propose_schedule_enable", "nopsai.propose_schedule_disable":
		permission.Resource.ID = a.hostedMCPScheduleArgID(ctx, args)
	case "nopsai.propose_schedule_create":
		permission.Resource.ID = hostedMCPSchedulePlanArgID(args)
	case "nopsai.propose_knowledge_context_create", "nopsai.propose_knowledge_context_update", "nopsai.propose_knowledge_context_delete":
		permission.Resource.ID = a.knowledgeContextArgID(ctx, args)
	case "nopsai.get_git_webhook_source", "nopsai.list_git_webhook_deliveries", "nopsai.propose_git_webhook_source_update", "nopsai.propose_git_webhook_source_delete":
		permission.Resource.ID = stringArg(args, "source_id")
	case "nopsai.propose_git_webhook_source_create":
		permission.Resource.ID = firstNonEmptyString(stringArg(args, "source_id"), stringArg(args, "id"))
	case "nopsai.get_external_trigger", "nopsai.list_external_trigger_invocations", "nopsai.propose_external_trigger_update", "nopsai.propose_external_trigger_delete":
		permission.Resource.ID = firstNonEmptyString(stringArg(args, "trigger_id"), stringArg(args, "id"))
	case "nopsai.propose_external_trigger_create":
		permission.Resource.ID = firstNonEmptyString(stringArg(args, "trigger_id"), stringArg(args, "id"))
	case "nopsai.invoke_external_trigger":
		permission.Resource.ID = firstNonEmptyString(stringArg(args, "trigger_id"), stringArg(args, "id"))
	case "nopsai.get_notification_route", "nopsai.propose_notification_route_update", "nopsai.propose_notification_route_delete":
		permission.Resource.ID = firstNonEmptyString(stringArg(args, "folder_id"), stringArg(args, "group_path"))
	case "nopsai.get_config_repo", "nopsai.get_config_repo_drift", "nopsai.sync_config_repo", "nopsai.write_config_repo":
		if folderID := strings.Trim(strings.TrimSpace(stringArg(args, "folder_id")), "/"); folderID != "" {
			permission.Resource.Type = "folder"
			permission.Resource.ID = folderID
			if tool.Name == "nopsai.sync_config_repo" {
				permission.Action = "config_repo.sync"
			} else if tool.Name == "nopsai.write_config_repo" {
				permission.Action = "config_repo.manage"
			} else {
				permission.Action = "config_repo.read"
			}
		}
	default:
		return false, nil
	}
	if strings.TrimSpace(permission.Resource.ID) == "" {
		permission.Resource.ID = tool.Resource.ID
	}
	if tool.AuthenticatedOnly || a.hostedMCPAllowed(ctx, subject, permission) {
		return true, nil
	}
	return true, fmt.Errorf("tool %s is not allowed for %s:%s", tool.Name, permission.Resource.Type, permission.Resource.ID)
}

func (a *App) executeHostedMCPDedicatedTool(ctx context.Context, subject aaamodel.Subject, name string, args map[string]any) (map[string]any, bool, error) {
	switch name {
	case "nopsai.run_pipeline":
		return a.hostedMCPRunPipeline(ctx, subject, args)
	case "nopsai.list_run_approvals":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, "/v1/runs/"+hostedMCPPathTail(stringArg(args, "run_id"))+"/approvals", nil, false, false, ""), true, nil
	case "nopsai.approve_run_approval":
		return a.hostedMCPRunApprovalDecision(ctx, subject, args, "approve")
	case "nopsai.reject_run_approval":
		return a.hostedMCPRunApprovalDecision(ctx, subject, args, "reject")
	case "nopsai.rerun_pipeline_run":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/runs/"+hostedMCPPathTail(stringArg(args, "run_id"))+"/rerun", nil, boolArg(args, "confirm", false), false, ""), true, nil
	case "nopsai.cancel_pipeline_run":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/runs/"+hostedMCPPathTail(stringArg(args, "run_id"))+"/cancel", nil, boolArg(args, "confirm", false), true, "Cancelling a run can stop running work and child runs."), true, nil
	case "nopsai.delete_pipeline_run":
		return a.hostedMCPAPITool(ctx, subject, http.MethodDelete, "/v1/runs/"+hostedMCPPathTail(stringArg(args, "run_id")), nil, boolArg(args, "confirm", false), true, "Deleting a run removes run history and related records."), true, nil
	case "nopsai.propose_schedule_create":
		result, err := a.hostedMCPProposeScheduleWrite(ctx, args, "create")
		return result, true, err
	case "nopsai.propose_schedule_update":
		result, err := a.hostedMCPProposeScheduleWrite(ctx, args, "update")
		return result, true, err
	case "nopsai.propose_schedule_delete":
		result, err := a.hostedMCPProposeScheduleDelete(ctx, args)
		return result, true, err
	case "nopsai.propose_schedule_enable":
		result, err := a.hostedMCPProposeScheduleEnabled(ctx, args, true)
		return result, true, err
	case "nopsai.propose_schedule_disable":
		result, err := a.hostedMCPProposeScheduleEnabled(ctx, args, false)
		return result, true, err
	case "nopsai.run_schedule_now":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/schedules/"+hostedMCPPathTail(stringArg(args, "schedule_id"))+"/run", nil, boolArg(args, "confirm", false), true, "Running a schedule immediately creates a pipeline run."), true, nil
	case "nopsai.propose_knowledge_context_create":
		result, err := a.hostedMCPProposeKnowledgeContext(ctx, args, "create")
		return result, true, err
	case "nopsai.propose_knowledge_context_update":
		result, err := a.hostedMCPProposeKnowledgeContext(ctx, args, "update")
		return result, true, err
	case "nopsai.propose_knowledge_context_delete":
		result, err := a.hostedMCPProposeKnowledgeContext(ctx, args, "delete")
		return result, true, err
	case "nopsai.list_git_webhook_sources":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, "/v1/git-webhook-sources", nil, false, false, ""), true, nil
	case "nopsai.get_git_webhook_source":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, "/v1/git-webhook-sources/"+hostedMCPPathTail(stringArg(args, "source_id")), nil, false, false, ""), true, nil
	case "nopsai.list_git_webhook_deliveries":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, "/v1/git-webhook-sources/"+hostedMCPPathTail(stringArg(args, "source_id"))+"/deliveries", nil, false, false, ""), true, nil
	case "nopsai.propose_git_webhook_source_create":
		result, err := hostedMCPProposeGitWebhookSource(args, "create")
		return result, true, err
	case "nopsai.propose_git_webhook_source_update":
		result, err := hostedMCPProposeGitWebhookSource(args, "update")
		return result, true, err
	case "nopsai.propose_git_webhook_source_delete":
		result, err := hostedMCPProposeGitWebhookSource(args, "delete")
		return result, true, err
	case "nopsai.list_external_triggers":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, "/v1/external-triggers", nil, false, false, ""), true, nil
	case "nopsai.get_external_trigger":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, "/v1/external-triggers/"+hostedMCPPathTail(firstNonEmptyString(stringArg(args, "trigger_id"), stringArg(args, "id"))), nil, false, false, ""), true, nil
	case "nopsai.list_external_trigger_invocations":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, "/v1/external-triggers/"+hostedMCPPathTail(firstNonEmptyString(stringArg(args, "trigger_id"), stringArg(args, "id")))+"/invocations", nil, false, false, ""), true, nil
	case "nopsai.propose_external_trigger_create":
		result, err := hostedMCPProposeExternalTrigger(args, "create")
		return result, true, err
	case "nopsai.propose_external_trigger_update":
		result, err := hostedMCPProposeExternalTrigger(args, "update")
		return result, true, err
	case "nopsai.propose_external_trigger_delete":
		result, err := hostedMCPProposeExternalTrigger(args, "delete")
		return result, true, err
	case "nopsai.invoke_external_trigger":
		return a.hostedMCPInvokeExternalTrigger(ctx, subject, args)
	case "nopsai.get_config_sync_status":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, "/v1/system/config/sync", nil, false, false, ""), true, nil
	case "nopsai.sync_system_config":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/system/config/sync", nil, boolArg(args, "confirm", false), true, "System config sync changes runtime configuration from configured sources."), true, nil
	case "nopsai.get_config_repo":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, hostedMCPConfigRepoPath(args, ""), nil, false, false, ""), true, nil
	case "nopsai.get_config_repo_drift":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, hostedMCPConfigRepoPath(args, "/drift"), nil, false, false, ""), true, nil
	case "nopsai.sync_config_repo":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, hostedMCPConfigRepoPath(args, "/sync"), nil, boolArg(args, "confirm", false), true, "Config repository sync can create, update, or delete product configuration."), true, nil
	case "nopsai.write_config_repo":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, hostedMCPConfigRepoPath(args, "/write"), hostedMCPConfigRepoWriteBody(args), boolArg(args, "confirm", false), true, "Config repository write pushes files through the configured GitOps workflow."), true, nil
	case "nopsai.list_config_repos":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, "/v1/system/config-repos", nil, false, false, ""), true, nil
	case "nopsai.sync_all_config_repos":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/system/config-repos/sync", nil, boolArg(args, "confirm", false), true, "Syncing all config repositories can update enterprise configuration across scopes."), true, nil
	case "nopsai.get_notification_mail_settings":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, "/v1/system/notifications/mail", nil, false, false, ""), true, nil
	case "nopsai.propose_notification_mail_settings":
		result, err := hostedMCPProposeNotificationMailSettings(args)
		return result, true, err
	case "nopsai.test_notification_mail_settings":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/system/notifications/mail/test", hostedMCPNotificationMailTestBody(args), boolArg(args, "confirm", false), true, "SMTP testing sends an external email."), true, nil
	case "nopsai.get_notification_route":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, hostedMCPGroupPath(args, "/notifications"), nil, false, false, ""), true, nil
	case "nopsai.propose_notification_route_update":
		result, err := hostedMCPProposeNotificationRoute(args, "update")
		return result, true, err
	case "nopsai.propose_notification_route_delete":
		result, err := hostedMCPProposeNotificationRoute(args, "delete")
		return result, true, err
	case "nopsai.list_monitoring_views":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, "/v1/monitoring/views", nil, false, false, ""), true, nil
	case "nopsai.create_monitoring_view":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/monitoring/views", args["view"], boolArg(args, "confirm", false), false, ""), true, nil
	case "nopsai.update_monitoring_view":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPut, "/v1/monitoring/views/"+hostedMCPPathTail(stringArg(args, "view_id")), args["view"], boolArg(args, "confirm", false), false, ""), true, nil
	case "nopsai.delete_monitoring_view":
		return a.hostedMCPAPITool(ctx, subject, http.MethodDelete, "/v1/monitoring/views/"+hostedMCPPathTail(stringArg(args, "view_id")), nil, boolArg(args, "confirm", false), true, "Deleting a monitoring saved view removes the user's saved operational context."), true, nil
	case "nopsai.list_monitoring_alert_rules":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, "/v1/monitoring/alert-rules", nil, false, false, ""), true, nil
	case "nopsai.create_monitoring_alert_rule":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/monitoring/alert-rules", args["rule"], boolArg(args, "confirm", false), false, ""), true, nil
	case "nopsai.update_monitoring_alert_rule":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPut, "/v1/monitoring/alert-rules/"+hostedMCPPathTail(stringArg(args, "rule_id")), args["rule"], boolArg(args, "confirm", false), false, ""), true, nil
	case "nopsai.delete_monitoring_alert_rule":
		return a.hostedMCPAPITool(ctx, subject, http.MethodDelete, "/v1/monitoring/alert-rules/"+hostedMCPPathTail(stringArg(args, "rule_id")), nil, boolArg(args, "confirm", false), true, "Deleting an alert rule stops that rule from producing future events."), true, nil
	case "nopsai.evaluate_monitoring_alert_rule":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/monitoring/alert-rules/"+hostedMCPPathTail(stringArg(args, "rule_id"))+"/evaluate", nil, boolArg(args, "confirm", false), false, ""), true, nil
	case "nopsai.list_monitoring_alert_events":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, "/v1/monitoring/alert-events", nil, false, false, ""), true, nil
	case "nopsai.list_monitoring_recommendations":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, "/v1/monitoring/recommendations", nil, false, false, ""), true, nil
	case "nopsai.acknowledge_monitoring_recommendation":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/monitoring/recommendations/"+hostedMCPPathTail(stringArg(args, "recommendation_id"))+"/acknowledge", nil, boolArg(args, "confirm", false), false, ""), true, nil
	case "nopsai.resolve_monitoring_recommendation":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/monitoring/recommendations/"+hostedMCPPathTail(stringArg(args, "recommendation_id"))+"/resolve", nil, boolArg(args, "confirm", false), false, ""), true, nil
	case "nopsai.list_data_backups":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, "/v1/system/data/backups", nil, false, false, ""), true, nil
	case "nopsai.create_data_backup":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/system/data/backups", map[string]any{"backup_type": stringArg(args, "backup_type"), "type": stringArg(args, "type")}, boolArg(args, "confirm", false), true, "Creating a backup can consume storage and expose export artifacts."), true, nil
	case "nopsai.delete_data_backup":
		return a.hostedMCPAPITool(ctx, subject, http.MethodDelete, "/v1/system/data/backups/"+hostedMCPPathTail(stringArg(args, "backup_id")), nil, boolArg(args, "confirm", false), true, "Deleting a backup permanently removes a recovery artifact."), true, nil
	case "nopsai.preview_data_cleanup":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/system/data/cleanup/preview", hostedMCPDataCleanupBody(args), true, false, ""), true, nil
	case "nopsai.run_data_cleanup":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/system/data/cleanup/run", hostedMCPDataCleanupBody(args), boolArg(args, "confirm", false), true, "Data cleanup permanently deletes matching rows. Preview before confirming."), true, nil
	case "nopsai.list_data_cleanup_jobs":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, "/v1/system/data/cleanup/jobs", nil, false, false, ""), true, nil
	case "nopsai.list_data_cleanup_schedules":
		return a.hostedMCPAPITool(ctx, subject, http.MethodGet, "/v1/system/data/cleanup/schedules", nil, false, false, ""), true, nil
	case "nopsai.create_data_cleanup_schedule":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/system/data/cleanup/schedules", args["schedule"], boolArg(args, "confirm", false), false, ""), true, nil
	case "nopsai.update_data_cleanup_schedule":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPut, "/v1/system/data/cleanup/schedules/"+hostedMCPPathTail(stringArg(args, "schedule_id")), args["schedule"], boolArg(args, "confirm", false), false, ""), true, nil
	case "nopsai.delete_data_cleanup_schedule":
		return a.hostedMCPAPITool(ctx, subject, http.MethodDelete, "/v1/system/data/cleanup/schedules/"+hostedMCPPathTail(stringArg(args, "schedule_id")), nil, boolArg(args, "confirm", false), true, "Deleting a cleanup schedule removes future retention automation."), true, nil
	case "nopsai.run_data_cleanup_schedule":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/system/data/cleanup/schedules/"+hostedMCPPathTail(stringArg(args, "schedule_id"))+"/run", nil, boolArg(args, "confirm", false), true, "Running a cleanup schedule can permanently delete matching data."), true, nil
	case "nopsai.enable_data_cleanup_schedule":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/system/data/cleanup/schedules/"+hostedMCPPathTail(stringArg(args, "schedule_id"))+"/enable", nil, boolArg(args, "confirm", false), false, ""), true, nil
	case "nopsai.disable_data_cleanup_schedule":
		return a.hostedMCPAPITool(ctx, subject, http.MethodPost, "/v1/system/data/cleanup/schedules/"+hostedMCPPathTail(stringArg(args, "schedule_id"))+"/disable", nil, boolArg(args, "confirm", false), false, ""), true, nil
	default:
		return nil, false, nil
	}
}

func (a *App) hostedMCPAPITool(ctx context.Context, subject aaamodel.Subject, method, path string, body any, confirm, highImpact bool, highImpactNote string) map[string]any {
	callArgs := map[string]any{
		"method":  method,
		"path":    path,
		"confirm": confirm,
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

func (a *App) hostedMCPRunPipeline(ctx context.Context, subject aaamodel.Subject, args map[string]any) (map[string]any, bool, error) {
	pipelineID := hostedMCPPipelineIDFromArgs(args)
	path := "/v1/run"
	if pipelineID != "" {
		path += "/" + hostedMCPPathTail(pipelineID)
	}
	body := hostedMCPRunBody(args, pipelineID)
	result := a.hostedMCPAPITool(ctx, subject, http.MethodPost, path, body, boolArg(args, "confirm", false), true, "Starting a pipeline run can dispatch runner work and use configured credentials.")
	return result, true, nil
}

func (a *App) hostedMCPRunApprovalDecision(ctx context.Context, subject aaamodel.Subject, args map[string]any, decision string) (map[string]any, bool, error) {
	runID := stringArg(args, "run_id")
	approvalID := stringArg(args, "approval_id")
	body := map[string]any{}
	if comment := stringArg(args, "comment"); comment != "" {
		body["comment"] = comment
	}
	path := "/v1/runs/" + hostedMCPPathTail(runID) + "/approvals/" + hostedMCPPathTail(approvalID) + "/" + decision
	result := a.hostedMCPAPITool(ctx, subject, http.MethodPost, path, body, boolArg(args, "confirm", false), true, "Approval decisions resume or reject waiting pipeline work.")
	return result, true, nil
}

func (a *App) hostedMCPProposeScheduleWrite(ctx context.Context, args map[string]any, mode string) (map[string]any, error) {
	req, err := a.hostedMCPScheduleRequestFromArgs(ctx, args)
	if err != nil {
		return nil, err
	}
	input, err := normalizeScheduleInput(req)
	if err != nil {
		return map[string]any{"proposal_type": "schedule_" + mode, "applies": false, "valid": false, "error": err.Error()}, nil
	}
	return hostedMCPSchedulePlan(mode, input, stringArg(args, "message")), nil
}

func (a *App) hostedMCPProposeScheduleDelete(ctx context.Context, args map[string]any) (map[string]any, error) {
	input, err := a.hostedMCPScheduleInputForMutation(ctx, args)
	if err != nil {
		return nil, err
	}
	plan := hostedMCPSchedulePlan("delete", input, stringArg(args, "message"))
	plan["valid"] = true
	plan["gitops"].(map[string]any)["files"] = []map[string]any{{"path": hostedMCPScheduleGitOpsPath(input), "content": "", "delete": true}}
	return plan, nil
}

func (a *App) hostedMCPProposeScheduleEnabled(ctx context.Context, args map[string]any, enabled bool) (map[string]any, error) {
	input, err := a.hostedMCPScheduleInputForMutation(ctx, args)
	if err != nil {
		return nil, err
	}
	input.Enabled = enabled
	mode := "disable"
	if enabled {
		mode = "enable"
	}
	return hostedMCPSchedulePlan(mode, input, stringArg(args, "message")), nil
}

func (a *App) hostedMCPScheduleRequestFromArgs(ctx context.Context, args map[string]any) (scheduleRequest, error) {
	var req scheduleRequest
	if err := hostedMCPDecodeObject(firstNonNil(args["schedule"], args["body"]), &req); err != nil {
		return req, err
	}
	if scheduleID := stringArg(args, "schedule_id"); scheduleID != "" && strings.TrimSpace(req.Name) == "" {
		input, err := a.hostedMCPScheduleInputForMutation(ctx, args)
		if err != nil {
			return req, err
		}
		req = hostedMCPScheduleRequestFromInput(input)
	}
	hostedMCPApplyScheduleArgs(&req, args)
	return req, nil
}

func (a *App) hostedMCPScheduleInputForMutation(ctx context.Context, args map[string]any) (scheduleInput, error) {
	scheduleID := stringArg(args, "schedule_id")
	if scheduleID == "" {
		req, err := hostedMCPMinimalScheduleRequest(args)
		if err != nil {
			return scheduleInput{}, err
		}
		return normalizeScheduleInput(req)
	}
	record, err := a.getScheduleRecord(ctx, scheduleID)
	if err != nil {
		return scheduleInput{}, err
	}
	return scheduleInput{
		Path:            record.Path,
		Name:            record.Name,
		Description:     record.Description,
		PipelinePath:    record.PipelinePath,
		PipelineName:    record.PipelineName,
		PipelineVersion: normalizePipelineVersion(record.PipelineVersion),
		ScheduleKind:    normalizeScheduleKindValue(record.ScheduleKind),
		CronExpression:  record.CronExpression,
		RunAt:           record.RunAt,
		Timezone:        record.Timezone,
		Enabled:         record.Enabled,
		Scope:           record.Scope,
		RunGroupPath:    record.RunGroupPath,
		Variables:       record.Variables,
		NextRunAt:       record.NextRunAt,
	}, nil
}

func hostedMCPMinimalScheduleRequest(args map[string]any) (scheduleRequest, error) {
	var req scheduleRequest
	if err := hostedMCPDecodeObject(firstNonNil(args["schedule"], args["body"]), &req); err != nil {
		return req, err
	}
	hostedMCPApplyScheduleArgs(&req, args)
	return req, nil
}

func hostedMCPApplyScheduleArgs(req *scheduleRequest, args map[string]any) {
	if req == nil || args == nil {
		return
	}
	if value := stringArg(args, "path"); value != "" {
		req.Path = value
	}
	if value := stringArg(args, "name"); value != "" {
		req.Name = value
	}
	if value := stringArg(args, "description"); value != "" {
		req.Description = value
	}
	if value := stringArg(args, "pipeline"); value != "" {
		req.Pipeline = value
	}
	if value := stringArg(args, "pipeline_path"); value != "" {
		req.PipelinePath = value
	}
	if value := stringArg(args, "pipeline_name"); value != "" {
		req.PipelineName = value
	}
	if value := stringArg(args, "pipeline_version"); value != "" {
		req.PipelineVersion = value
	}
	if value := stringArg(args, "schedule_kind"); value != "" {
		req.ScheduleKind = value
	}
	if value := stringArg(args, "cron_expression"); value != "" {
		req.CronExpression = value
	}
	if value := stringArg(args, "cron"); value != "" {
		req.Cron = value
	}
	if value := stringArg(args, "run_at"); value != "" {
		req.RunAt = value
	}
	if value := stringArg(args, "timezone"); value != "" {
		req.Timezone = value
	}
	if _, ok := args["enabled"]; ok {
		enabled := boolArg(args, "enabled", true)
		req.Enabled = &enabled
	}
	if value := stringArg(args, "scope"); value != "" {
		req.Scope = value
	}
	if value := stringArg(args, "run_group_path"); value != "" {
		req.RunGroupPath = value
	}
	if variables := hostedMCPStringMapArg(args, "variables"); len(variables) > 0 {
		req.Variables = variables
	}
}

func hostedMCPScheduleRequestFromInput(input scheduleInput) scheduleRequest {
	req := scheduleRequest{
		Path:            input.Path,
		Name:            input.Name,
		Description:     input.Description,
		Pipeline:        configsync.BuildPipelineIdentifier(input.PipelinePath, input.PipelineName),
		PipelineVersion: input.PipelineVersion,
		ScheduleKind:    normalizeScheduleKindValue(input.ScheduleKind),
		CronExpression:  input.CronExpression,
		Timezone:        input.Timezone,
		Enabled:         &input.Enabled,
		Scope:           input.Scope,
		RunGroupPath:    input.RunGroupPath,
		Variables:       input.Variables,
	}
	if input.RunAt != nil {
		req.RunAt = input.RunAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return req
}

func hostedMCPSchedulePlan(mode string, input scheduleInput, message string) map[string]any {
	content, err := hostedMCPScheduleYAML(input)
	valid := err == nil
	if message == "" {
		message = strings.Title(mode) + " NopsAI schedule " + configsync.BuildPipelineIdentifier(input.Path, input.Name)
	}
	plan := map[string]any{
		"proposal_type": "schedule_" + mode,
		"applies":       false,
		"valid":         valid,
		"schedule_id":   configsync.BuildPipelineIdentifier(input.Path, input.Name),
		"gitops": map[string]any{
			"message": message,
			"files": []map[string]any{{
				"path":    hostedMCPScheduleGitOpsPath(input),
				"content": content,
				"delete":  false,
			}},
			"review_note": "Commit this schedule file through the configured config repository, then sync GitOps before relying on the change.",
		},
	}
	if err != nil {
		plan["error"] = err.Error()
	}
	return plan
}

func hostedMCPScheduleYAML(input scheduleInput) (string, error) {
	doc := configRepositoryScheduleDocument{
		Name:         input.Name,
		Description:  strings.TrimSpace(input.Description),
		Pipeline:     configsync.BuildPipelineIdentifier(input.PipelinePath, input.PipelineName),
		Timezone:     input.Timezone,
		Enabled:      input.Enabled,
		Scope:        input.Scope,
		RunGroupPath: input.RunGroupPath,
		Variables:    input.Variables,
	}
	if normalizeScheduleKindValue(input.ScheduleKind) == scheduleKindOnce {
		doc.ScheduleKind = scheduleKindOnce
		if input.RunAt != nil {
			doc.RunAt = input.RunAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
	} else {
		doc.CronExpression = input.CronExpression
	}
	raw, err := marshalConfigRepositoryYAML(doc)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func hostedMCPScheduleGitOpsPath(input scheduleInput) string {
	return filepath.ToSlash(filepath.Join("schedules", configsync.BuildPipelineFilePath(input.Path, input.Name, ".yaml")))
}

func hostedMCPSchedulePlanArgID(args map[string]any) string {
	req, err := hostedMCPMinimalScheduleRequest(args)
	if err != nil {
		return ""
	}
	return configsync.BuildPipelineIdentifier(req.Path, req.Name)
}

func (a *App) hostedMCPScheduleArgID(ctx context.Context, args map[string]any) string {
	if scheduleID := stringArg(args, "schedule_id"); scheduleID != "" && a != nil && a.db != nil {
		record, err := a.getScheduleRecord(ctx, scheduleID)
		if err == nil {
			return record.identifier()
		}
		return scheduleID
	}
	return hostedMCPSchedulePlanArgID(args)
}

func (a *App) hostedMCPProposeKnowledgeContext(ctx context.Context, args map[string]any, mode string) (map[string]any, error) {
	kind, group, name, err := a.hostedMCPKnowledgeContextParts(ctx, args)
	if err != nil {
		return nil, err
	}
	id := buildKnowledgeContextIdentifier(kind, group, name)
	path := hostedMCPKnowledgeContextGitOpsPath(kind, group, name)
	message := stringArg(args, "message")
	if message == "" {
		message = strings.Title(mode) + " NopsAI knowledge context " + id
	}
	deleteFile := mode == "delete"
	content := ""
	if !deleteFile {
		description := stringArg(args, "description")
		body := stringArg(args, "content")
		if body == "" {
			return map[string]any{"proposal_type": "knowledge_context_" + mode, "applies": false, "valid": false, "error": "content is required"}, nil
		}
		content = renderKnowledgeContextGitOpsDocument(kind, name, description, body, id, nil)
	}
	return map[string]any{
		"proposal_type": "knowledge_context_" + mode,
		"applies":       false,
		"valid":         true,
		"id":            id,
		"gitops": map[string]any{
			"message": message,
			"files": []map[string]any{{
				"path":    path,
				"content": content,
				"delete":  deleteFile,
			}},
			"review_note": "Commit this knowledge context file through the configured config repository, then sync GitOps before relying on the change.",
		},
	}, nil
}

func (a *App) hostedMCPKnowledgeContextParts(ctx context.Context, args map[string]any) (string, string, string, error) {
	if id := strings.Trim(strings.TrimSpace(stringArg(args, "id")), "/"); id != "" {
		if strings.Contains(id, "/") {
			return splitKnowledgeContextIdentifier(id)
		}
		if a != nil && a.db != nil {
			var kind, group, name string
			err := a.db.QueryRow(ctx, `SELECT kind, group_path, name FROM knowledge_contexts WHERE id::text = $1`, id).Scan(&kind, &group, &name)
			if err == nil {
				return kind, group, name, nil
			}
		}
	}
	kind, err := normalizeKnowledgeContextKind(stringArg(args, "kind"))
	if err != nil {
		return "", "", "", err
	}
	group, err := normalizeKnowledgeContextGroup(firstNonEmptyString(stringArg(args, "group"), stringArg(args, "group_path")))
	if err != nil {
		return "", "", "", err
	}
	name, err := normalizeKnowledgeContextName(stringArg(args, "name"))
	if err != nil {
		return "", "", "", err
	}
	return kind, group, name, nil
}

func hostedMCPKnowledgeContextGitOpsPath(kind, group, name string) string {
	relID := strings.Trim(strings.Trim(group, "/")+"/"+strings.Trim(name, "/"), "/")
	return filepath.ToSlash(filepath.Join("knowledge", kind, relID+".md"))
}

func hostedMCPProposeGitWebhookSource(args map[string]any, mode string) (map[string]any, error) {
	id := firstNonEmptyString(stringArg(args, "source_id"), stringArg(args, "id"))
	if mode == "delete" {
		if id == "" {
			return nil, fmt.Errorf("source_id is required")
		}
		return hostedMCPDeleteFilePlan("git_webhook_source_delete", id, filepath.ToSlash(filepath.Join(gitWebhookSourcesGitOpsDirectory, externalTriggerGitOpsSlug(id)+".yaml")), stringArg(args, "message")), nil
	}
	input, err := hostedMCPGitWebhookSourceInputFromArgs(args, id)
	if err != nil {
		return nil, err
	}
	source, err := normalizeGitWebhookSourceInput(input, id)
	if err != nil {
		return map[string]any{"proposal_type": "git_webhook_source_" + mode, "applies": false, "valid": false, "error": err.Error()}, nil
	}
	content, err := marshalConfigRepositoryYAML(gitWebhookSourceGitOpsDocument(input))
	if err != nil {
		return nil, err
	}
	path := filepath.ToSlash(filepath.Join(gitWebhookSourcesGitOpsDirectory, externalTriggerGitOpsSlug(source.ID)+".yaml"))
	return hostedMCPFilePlan("git_webhook_source_"+mode, source.ID, path, string(content), false, stringArg(args, "message")), nil
}

func hostedMCPGitWebhookSourceInputFromArgs(args map[string]any, id string) (gitWebhookSourceInput, error) {
	var input gitWebhookSourceInput
	if err := hostedMCPDecodeObject(firstNonNil(args["source"], args["body"]), &input); err != nil {
		return input, err
	}
	if input.ID == "" {
		input.ID = id
	}
	if value := stringArg(args, "name"); value != "" {
		input.Name = value
	}
	if value := stringArg(args, "description"); value != "" {
		input.Description = value
	}
	if value := stringArg(args, "provider"); value != "" {
		input.Provider = value
	}
	if value := stringArg(args, "auth_mode"); value != "" {
		input.AuthMode = value
	}
	if value := stringArg(args, "credential_ref"); value != "" {
		input.CredentialRef = value
	}
	if _, ok := args["enabled"]; ok {
		enabled := boolArg(args, "enabled", true)
		input.Enabled = &enabled
	}
	if allowlist := hostedMCPStringSliceArg(args, "repository_allowlist"); len(allowlist) > 0 {
		input.RepositoryAllowlist = allowlist
	}
	if rateLimit := hostedMCPMapArg(args, "rate_limit"); len(rateLimit) > 0 {
		input.RateLimit = rateLimit
	}
	return input, nil
}

func hostedMCPProposeExternalTrigger(args map[string]any, mode string) (map[string]any, error) {
	id := firstNonEmptyString(stringArg(args, "trigger_id"), stringArg(args, "id"))
	if mode == "delete" {
		if id == "" {
			return nil, fmt.Errorf("trigger_id is required")
		}
		return hostedMCPDeleteFilePlan("external_trigger_delete", id, filepath.ToSlash(filepath.Join(externalTriggersGitOpsDirectory, externalTriggerGitOpsSlug(id)+".yaml")), stringArg(args, "message")), nil
	}
	input, err := hostedMCPExternalTriggerInputFromArgs(args, id)
	if err != nil {
		return nil, err
	}
	trigger, err := normalizeExternalTriggerInput(input, id)
	if err != nil {
		return map[string]any{"proposal_type": "external_trigger_" + mode, "applies": false, "valid": false, "error": err.Error()}, nil
	}
	doc := externalTriggerGitOpsDocument{
		ID:              trigger.ID,
		Name:            trigger.Name,
		Description:     trigger.Description,
		Enabled:         &trigger.Enabled,
		Pipeline:        trigger.Pipeline,
		Scope:           trigger.Scope,
		RunGroupPath:    trigger.RunGroupPath,
		AllowedCallers:  trigger.AllowedCallers,
		VariableMapping: trigger.VariableMapping,
		PayloadSchema:   trigger.PayloadSchema,
		RateLimit:       trigger.RateLimit,
	}
	content, err := marshalConfigRepositoryYAML(doc)
	if err != nil {
		return nil, err
	}
	path := filepath.ToSlash(filepath.Join(externalTriggersGitOpsDirectory, externalTriggerGitOpsSlug(trigger.ID)+".yaml"))
	return hostedMCPFilePlan("external_trigger_"+mode, trigger.ID, path, string(content), false, stringArg(args, "message")), nil
}

func hostedMCPExternalTriggerInputFromArgs(args map[string]any, id string) (externalTriggerInput, error) {
	var input externalTriggerInput
	if err := hostedMCPDecodeObject(firstNonNil(args["trigger"], args["body"]), &input); err != nil {
		return input, err
	}
	if input.ID == "" {
		input.ID = id
	}
	if value := stringArg(args, "name"); value != "" {
		input.Name = value
	}
	if value := stringArg(args, "description"); value != "" {
		input.Description = value
	}
	if value := stringArg(args, "pipeline"); value != "" {
		input.Pipeline = value
	}
	if _, ok := args["enabled"]; ok {
		enabled := boolArg(args, "enabled", true)
		input.Enabled = &enabled
	}
	if value := stringArg(args, "scope"); value != "" {
		input.Scope = value
	}
	if value := stringArg(args, "run_group_path"); value != "" {
		input.RunGroupPath = value
	}
	if value := hostedMCPStringMapArg(args, "variable_mapping"); len(value) > 0 {
		input.VariableMapping = value
	}
	if value := hostedMCPMapArg(args, "payload_schema"); len(value) > 0 {
		input.PayloadSchema = value
	}
	if value := hostedMCPMapArg(args, "rate_limit"); len(value) > 0 {
		input.RateLimit = value
	}
	if raw, ok := args["allowed_callers"]; ok {
		_ = hostedMCPDecodeObject(raw, &input.AllowedCallers)
	}
	return input, nil
}

func (a *App) hostedMCPInvokeExternalTrigger(ctx context.Context, subject aaamodel.Subject, args map[string]any) (map[string]any, bool, error) {
	body := map[string]any{}
	for _, key := range []string{"event_type", "idempotency_key", "variables", "payload"} {
		if value, ok := args[key]; ok {
			body[key] = value
		}
	}
	path := "/v1/external-triggers/" + hostedMCPPathTail(firstNonEmptyString(stringArg(args, "trigger_id"), stringArg(args, "id"))) + "/invoke"
	result := a.hostedMCPAPITool(ctx, subject, http.MethodPost, path, body, boolArg(args, "confirm", false), true, "Invoking an external trigger can create a pipeline run.")
	return result, true, nil
}

func hostedMCPConfigRepoPath(args map[string]any, suffix string) string {
	folderID := strings.Trim(strings.TrimSpace(stringArg(args, "folder_id")), "/")
	if folderID == "" {
		return "/v1/system/config-repo" + suffix
	}
	return "/v1/groups/" + hostedMCPPathTail(folderID) + "/config-repo" + suffix
}

func hostedMCPGroupPath(args map[string]any, suffix string) string {
	folderID := strings.Trim(strings.TrimSpace(firstNonEmptyString(stringArg(args, "folder_id"), stringArg(args, "group_path"))), "/")
	return "/v1/groups/" + hostedMCPPathTail(folderID) + suffix
}

func hostedMCPConfigRepoWriteBody(args map[string]any) any {
	if body := args["body"]; body != nil {
		return body
	}
	body := map[string]any{}
	if files := args["files"]; files != nil {
		body["files"] = files
	}
	if message := stringArg(args, "commit_message"); message != "" {
		body["commit_message"] = message
	}
	if message := stringArg(args, "message"); message != "" && body["commit_message"] == nil {
		body["commit_message"] = message
	}
	return body
}

func hostedMCPProposeNotificationMailSettings(args map[string]any) (map[string]any, error) {
	var settings notificationMailSettingsFile
	if err := hostedMCPDecodeObject(firstNonNil(args["settings"], args["body"]), &settings); err != nil {
		return nil, err
	}
	if _, ok := args["enabled"]; ok {
		settings.Enabled = boolArg(args, "enabled", false)
	}
	if from := stringArg(args, "from"); from != "" {
		settings.From = from
	}
	if smtp := hostedMCPMapArg(args, "smtp"); len(smtp) > 0 {
		if err := hostedMCPDecodeObject(smtp, &settings.SMTP); err != nil {
			return nil, err
		}
	}
	normalized, err := normalizeNotificationMailSettings(settings)
	if err != nil {
		return map[string]any{"proposal_type": "notification_mail_settings", "applies": false, "valid": false, "error": err.Error()}, nil
	}
	content, err := marshalConfigRepositoryYAML(normalized)
	if err != nil {
		return nil, err
	}
	return hostedMCPFilePlan("notification_mail_settings", "system/notifications", configRepositoryMailSettingsPath, string(content), false, stringArg(args, "message")), nil
}

func hostedMCPNotificationMailTestBody(args map[string]any) map[string]any {
	return map[string]any{
		"to":      stringArg(args, "to"),
		"subject": stringArg(args, "subject"),
		"body":    stringArg(args, "body"),
	}
}

func hostedMCPProposeNotificationRoute(args map[string]any, mode string) (map[string]any, error) {
	groupPath := strings.Trim(strings.TrimSpace(firstNonEmptyString(stringArg(args, "group_path"), stringArg(args, "folder_id"))), "/")
	if groupPath == "" {
		return nil, fmt.Errorf("group_path or folder_id is required")
	}
	path := filepath.ToSlash(filepath.Join("config-repositories", "groups", groupPath, "notifications.yaml"))
	if mode == "delete" {
		return hostedMCPDeleteFilePlan("notification_route_delete", groupPath, path, stringArg(args, "message")), nil
	}
	var input notificationRouteDefinitionFile
	if err := hostedMCPDecodeObject(firstNonNil(args["definition"], args["body"]), &input); err != nil {
		return nil, err
	}
	normalized, err := normalizeNotificationRouteDefinition(input)
	if err != nil {
		return map[string]any{"proposal_type": "notification_route_update", "applies": false, "valid": false, "error": err.Error()}, nil
	}
	content, err := marshalConfigRepositoryYAML(notificationRouteDefinitionFileFromDefinition(normalized))
	if err != nil {
		return nil, err
	}
	return hostedMCPFilePlan("notification_route_update", groupPath, path, string(content), false, stringArg(args, "message")), nil
}

func hostedMCPDataCleanupBody(args map[string]any) map[string]any {
	body := map[string]any{
		"target":                stringArg(args, "target"),
		"mode":                  stringArg(args, "mode"),
		"backup_before_cleanup": boolArg(args, "backup_before_cleanup", true),
	}
	if value := intArg(args, "keep_last", 0, 0); value > 0 {
		body["keep_last"] = value
	}
	if value := intArg(args, "older_than_days", 0, 0); value > 0 {
		body["older_than_days"] = value
	}
	return body
}

func hostedMCPRunBody(args map[string]any, pipelineID string) map[string]any {
	body := map[string]any{}
	if pipelineID != "" {
		body["pipeline"] = pipelineID
	}
	if scope := stringArg(args, "scope"); scope != "" {
		body["scope"] = scope
	}
	if definition := stringArg(args, "definition"); definition != "" {
		body["definition"] = definition
	}
	if variables := hostedMCPStringMapArg(args, "variables"); len(variables) > 0 {
		body["variables"] = variables
	}
	return body
}

func hostedMCPPipelineIDFromArgs(args map[string]any) string {
	if pipeline := strings.Trim(strings.TrimSpace(stringArg(args, "pipeline")), "/"); pipeline != "" {
		return pipeline
	}
	pathPart, namePart := splitPipelineArg(args)
	return strings.Trim(configsync.BuildPipelineIdentifier(pathPart, namePart), "/")
}

func hostedMCPFilePlan(proposalType, id, path, content string, deleteFile bool, message string) map[string]any {
	if message == "" {
		message = "Update NopsAI " + id
	}
	return map[string]any{
		"proposal_type": proposalType,
		"applies":       false,
		"valid":         true,
		"id":            id,
		"gitops": map[string]any{
			"message": message,
			"files": []map[string]any{{
				"path":    path,
				"content": content,
				"delete":  deleteFile,
			}},
			"review_note": "Commit this file through the configured config repository, then sync GitOps before relying on the change.",
		},
	}
}

func hostedMCPDeleteFilePlan(proposalType, id, path, message string) map[string]any {
	return hostedMCPFilePlan(proposalType, id, path, "", true, message)
}

func hostedMCPDecodeObject(value any, out any) error {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func hostedMCPMapArg(args map[string]any, key string) map[string]any {
	if args == nil {
		return nil
	}
	value, ok := args[key]
	if !ok || value == nil {
		return nil
	}
	if mapped, ok := value.(map[string]any); ok {
		return mapped
	}
	out := map[string]any{}
	_ = hostedMCPDecodeObject(value, &out)
	return out
}

func hostedMCPStringMapArg(args map[string]any, key string) map[string]string {
	mapped := map[string]string{}
	for k, v := range hostedMCPMapArg(args, key) {
		name := strings.TrimSpace(k)
		if name == "" {
			continue
		}
		mapped[name] = strings.TrimSpace(fmt.Sprint(v))
	}
	return mapped
}

func hostedMCPStringSliceArg(args map[string]any, key string) []string {
	if args == nil {
		return nil
	}
	value := args[key]
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := []string{}
		for _, item := range typed {
			out = append(out, strings.TrimSpace(fmt.Sprint(item)))
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return strings.Split(typed, ",")
	default:
		out := []string{}
		_ = hostedMCPDecodeObject(value, &out)
		return out
	}
}

func hostedMCPPathTail(value string) string {
	value = strings.Trim(strings.TrimSpace(filepath.ToSlash(value)), "/")
	if value == "" {
		return ""
	}
	parts := strings.Split(value, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func intArg(args map[string]any, key string, fallback int, max int) int {
	if args == nil {
		return fallback
	}
	var raw float64
	switch value := args[key].(type) {
	case float64:
		raw = value
	case float32:
		raw = float64(value)
	case int:
		raw = float64(value)
	case int64:
		raw = float64(value)
	case json.Number:
		parsed, _ := value.Float64()
		raw = parsed
	case string:
		parsed := json.Number(strings.TrimSpace(value))
		parsedValue, _ := parsed.Float64()
		raw = parsedValue
	default:
		return fallback
	}
	if raw <= 0 {
		return fallback
	}
	out := int(raw)
	if max > 0 && out > max {
		return max
	}
	return out
}

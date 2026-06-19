package nopsai

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	assistantExplicitToolPattern = regexp.MustCompile(`(?i)\b(nopsai\.[a-z0-9_]+)\b`)
	assistantFeatureValuePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/@-]{0,180}$`)
	assistantFeatureEmailPattern = regexp.MustCompile(`(?i)\b[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}\b`)
)

func assistantFeatureToolPlanFromMessage(content string, plan assistantTurnPlan) (assistantPlanStep, string, bool) {
	lower := strings.ToLower(strings.TrimSpace(content))
	if lower == "" {
		return assistantPlanStep{}, "", false
	}
	confirm := assistantFeatureConfirmed(lower)
	if toolName := assistantExplicitHostedMCPToolName(content); toolName != "" {
		return assistantPlanStep{
			Thought:  "Run the explicitly requested hosted MCP tool with current-subject authorization.",
			ToolName: toolName,
			Args:     assistantFeatureConfirmArgs(map[string]any{}, confirm),
		}, "Execute the explicitly requested hosted MCP tool through current AAA permissions.", true
	}
	if step, criteria, ok := assistantSetupFeatureStep(content, plan, confirm); ok {
		return step, criteria, true
	}
	if step, criteria, ok := assistantRunFeatureStep(content, plan, confirm); ok {
		return step, criteria, true
	}
	if step, criteria, ok := assistantScheduleFeatureStep(content, plan, confirm); ok {
		return step, criteria, true
	}
	if step, criteria, ok := assistantGitWebhookFeatureStep(content, plan); ok {
		return step, criteria, true
	}
	if step, criteria, ok := assistantExternalTriggerFeatureStep(content, plan, confirm); ok {
		return step, criteria, true
	}
	if step, criteria, ok := assistantConfigRepoFeatureStep(content, plan, confirm); ok {
		return step, criteria, true
	}
	if step, criteria, ok := assistantNotificationFeatureStep(content, plan, confirm); ok {
		return step, criteria, true
	}
	if step, criteria, ok := assistantMonitoringFeatureStep(content, plan, confirm); ok {
		return step, criteria, true
	}
	if step, criteria, ok := assistantDataOpsFeatureStep(content, plan, confirm); ok {
		return step, criteria, true
	}
	if step, criteria, ok := assistantCredentialFeatureStep(content, plan, confirm); ok {
		return step, criteria, true
	}
	if step, criteria, ok := assistantSecretFeatureStep(content, plan, confirm); ok {
		return step, criteria, true
	}
	if step, criteria, ok := assistantVariableFeatureStep(content, plan, confirm); ok {
		return step, criteria, true
	}
	if step, criteria, ok := assistantRunnerFeatureStep(content, plan, confirm); ok {
		return step, criteria, true
	}
	if step, criteria, ok := assistantAccessAdminFeatureStep(content, plan, confirm); ok {
		return step, criteria, true
	}
	if step, criteria, ok := assistantKnowledgeFeatureStep(content, plan); ok {
		return step, criteria, true
	}
	if step, criteria, ok := assistantReusableStepFeatureStep(content, plan); ok {
		return step, criteria, true
	}
	if step, criteria, ok := assistantUIContextFeatureStep(content, plan); ok {
		return step, criteria, true
	}
	return assistantPlanStep{}, "", false
}

func assistantSetupFeatureStep(content string, plan assistantTurnPlan, confirm bool) (assistantPlanStep, string, bool) {
	lower := plan.LowerContent
	if !containsAny(lower, "setup", "first install", "first-install", "bootstrap") {
		return assistantPlanStep{}, "", false
	}
	args := map[string]any{}
	if profile := assistantFeatureValueAfterAnyLabel(content, "profile"); profile != "" {
		args["profile"] = profile
	}
	switch {
	case containsAny(lower, "preflight"):
		return assistantFeatureStep("Read first-install preflight checks.", "nopsai.get_setup_preflight", args), "Return first-install preflight checks.", true
	case containsAny(lower, "template", "templates", "starter"):
		return assistantFeatureStep("Generate first-install GitOps starter templates.", "nopsai.get_setup_templates", args), "Return starter GitOps templates without applying changes.", true
	case containsAny(lower, "plan", "guided"):
		return assistantFeatureStep("Build a first-install setup plan without applying it.", "nopsai.plan_first_install_setup", args), "Return a GitOps-compatible first-install plan without applying changes.", true
	case containsAny(lower, "bootstrap") && containsAny(lower, "run", "start", "apply", "execute", "create"):
		args = assistantFeatureConfirmArgs(args, confirm)
		return assistantFeatureStep("Run first-install bootstrap with confirmation enforcement.", "nopsai.bootstrap_first_install_setup", args), "Execute bootstrap only when explicitly confirmed and allowed.", true
	default:
		return assistantFeatureStep("Read first-install setup status.", "nopsai.get_setup_status", args), "Return first-install setup status.", true
	}
}

func assistantRunFeatureStep(content string, plan assistantTurnPlan, confirm bool) (assistantPlanStep, string, bool) {
	lower := plan.LowerContent
	runID := firstNonEmptyString(plan.RunID, assistantFeatureValueAfterAnyLabel(content, "run id", "run"))
	if containsAny(lower, "approval", "approvals") && containsAny(lower, "run") {
		args := map[string]any{"run_id": runID}
		if approvalID := assistantFeatureValueAfterAnyLabel(content, "approval id", "approval"); approvalID != "" {
			args["approval_id"] = approvalID
		}
		if containsAny(lower, "approve") {
			return assistantFeatureStep("Approve a pending run approval with confirmation enforcement.", "nopsai.approve_run_approval", assistantFeatureConfirmArgs(args, confirm)), "Approve the run approval only when explicitly confirmed and allowed.", true
		}
		if containsAny(lower, "reject", "deny") {
			return assistantFeatureStep("Reject a pending run approval with confirmation enforcement.", "nopsai.reject_run_approval", assistantFeatureConfirmArgs(args, confirm)), "Reject the run approval only when explicitly confirmed and allowed.", true
		}
		return assistantFeatureStep("List pending approvals for the run.", "nopsai.list_run_approvals", args), "Return visible run approvals.", true
	}
	if runID != "" {
		args := map[string]any{"run_id": runID}
		switch {
		case containsAny(lower, "rerun", "re-run", "run again"):
			return assistantFeatureStep("Rerun a pipeline run with confirmation enforcement.", "nopsai.rerun_pipeline_run", assistantFeatureConfirmArgs(args, confirm)), "Rerun the pipeline run only when explicitly confirmed and allowed.", true
		case containsAny(lower, "cancel", "stop"):
			return assistantFeatureStep("Cancel a pipeline run with confirmation enforcement.", "nopsai.cancel_pipeline_run", assistantFeatureConfirmArgs(args, confirm)), "Cancel the pipeline run only when explicitly confirmed and allowed.", true
		case containsAny(lower, "delete", "remove"):
			return assistantFeatureStep("Delete a pipeline run with confirmation enforcement.", "nopsai.delete_pipeline_run", assistantFeatureConfirmArgs(args, confirm)), "Delete the pipeline run only when explicitly confirmed and allowed.", true
		}
	}
	if containsAny(lower, "run pipeline", "start pipeline", "execute pipeline") {
		pipelineID := firstNonEmptyString(plan.PipelineID, assistantFeatureValueAfterAnyLabel(content, "pipeline id", "pipeline"))
		args := map[string]any{"pipeline": pipelineID, "scope": plan.Scope}
		return assistantFeatureStep("Start a pipeline run with confirmation enforcement.", "nopsai.run_pipeline", assistantFeatureConfirmArgs(args, confirm)), "Start the pipeline only when explicitly confirmed and allowed.", true
	}
	return assistantPlanStep{}, "", false
}

func assistantScheduleFeatureStep(content string, plan assistantTurnPlan, confirm bool) (assistantPlanStep, string, bool) {
	lower := plan.LowerContent
	if !containsAny(lower, "schedule", "schedules", "cron") {
		return assistantPlanStep{}, "", false
	}
	scheduleID := firstNonEmptyString(plan.ScheduleID, assistantFeatureValueAfterAnyLabel(content, "schedule id", "schedule"))
	args := assistantScheduleArgsFromMessage(content, plan, scheduleID)
	switch {
	case containsAny(lower, "create", "add", "new"):
		return assistantFeatureStep("Draft a GitOps schedule create plan.", "nopsai.propose_schedule_create", args), "Return a GitOps schedule create plan without applying changes.", true
	case containsAny(lower, "delete", "remove"):
		return assistantFeatureStep("Draft a GitOps schedule delete plan.", "nopsai.propose_schedule_delete", args), "Return a GitOps schedule delete plan without applying changes.", true
	case containsAny(lower, "enable"):
		return assistantFeatureStep("Draft a GitOps schedule enable plan.", "nopsai.propose_schedule_enable", args), "Return a GitOps schedule enable plan without applying changes.", true
	case containsAny(lower, "disable", "pause"):
		return assistantFeatureStep("Draft a GitOps schedule disable plan.", "nopsai.propose_schedule_disable", args), "Return a GitOps schedule disable plan without applying changes.", true
	case containsAny(lower, "run now", "run schedule", "trigger schedule", "execute schedule"):
		return assistantFeatureStep("Run a schedule immediately with confirmation enforcement.", "nopsai.run_schedule_now", assistantFeatureConfirmArgs(args, confirm)), "Run the schedule only when explicitly confirmed and allowed.", true
	case containsAny(lower, "update", "change", "modify"):
		return assistantFeatureStep("Draft a GitOps schedule update plan.", "nopsai.propose_schedule_update", args), "Return a GitOps schedule update plan without applying changes.", true
	default:
		return assistantPlanStep{}, "", false
	}
}

func assistantGitWebhookFeatureStep(content string, plan assistantTurnPlan) (assistantPlanStep, string, bool) {
	lower := plan.LowerContent
	if !containsAny(lower, "git webhook", "webhook source", "webhook delivery", "webhook deliveries") {
		return assistantPlanStep{}, "", false
	}
	sourceID := assistantFeatureValueAfterAnyLabel(content, "source id", "webhook source", "source")
	args := map[string]any{"source_id": sourceID, "message": content}
	switch {
	case containsAny(lower, "delivery", "deliveries"):
		return assistantFeatureStep("List recent Git webhook deliveries.", "nopsai.list_git_webhook_deliveries", args), "Return visible Git webhook delivery history.", true
	case containsAny(lower, "create", "add", "new"):
		return assistantFeatureStep("Draft a GitOps Git webhook source create plan.", "nopsai.propose_git_webhook_source_create", args), "Return a GitOps webhook source create plan without applying changes.", true
	case containsAny(lower, "update", "change", "modify"):
		return assistantFeatureStep("Draft a GitOps Git webhook source update plan.", "nopsai.propose_git_webhook_source_update", args), "Return a GitOps webhook source update plan without applying changes.", true
	case containsAny(lower, "delete", "remove"):
		return assistantFeatureStep("Draft a GitOps Git webhook source delete plan.", "nopsai.propose_git_webhook_source_delete", args), "Return a GitOps webhook source delete plan without applying changes.", true
	case sourceID != "":
		return assistantFeatureStep("Read a Git webhook source.", "nopsai.get_git_webhook_source", args), "Return the requested Git webhook source.", true
	default:
		return assistantFeatureStep("List visible Git webhook sources.", "nopsai.list_git_webhook_sources", map[string]any{}), "Return visible Git webhook sources.", true
	}
}

func assistantExternalTriggerFeatureStep(content string, plan assistantTurnPlan, confirm bool) (assistantPlanStep, string, bool) {
	lower := plan.LowerContent
	if !containsAny(lower, "external trigger", "external triggers") {
		return assistantPlanStep{}, "", false
	}
	triggerID := assistantFeatureValueAfterAnyLabel(content, "trigger id", "external trigger", "trigger")
	args := map[string]any{"trigger_id": triggerID, "id": triggerID, "message": content}
	switch {
	case containsAny(lower, "invocation", "invocations", "history"):
		return assistantFeatureStep("List external trigger invocations.", "nopsai.list_external_trigger_invocations", args), "Return visible external trigger invocation history.", true
	case containsAny(lower, "invoke", "fire", "run"):
		return assistantFeatureStep("Invoke an external trigger with confirmation enforcement.", "nopsai.invoke_external_trigger", assistantFeatureConfirmArgs(args, confirm)), "Invoke the external trigger only when explicitly confirmed and allowed.", true
	case containsAny(lower, "create", "add", "new"):
		return assistantFeatureStep("Draft a GitOps external trigger create plan.", "nopsai.propose_external_trigger_create", args), "Return a GitOps external trigger create plan without applying changes.", true
	case containsAny(lower, "update", "change", "modify"):
		return assistantFeatureStep("Draft a GitOps external trigger update plan.", "nopsai.propose_external_trigger_update", args), "Return a GitOps external trigger update plan without applying changes.", true
	case containsAny(lower, "delete", "remove"):
		return assistantFeatureStep("Draft a GitOps external trigger delete plan.", "nopsai.propose_external_trigger_delete", args), "Return a GitOps external trigger delete plan without applying changes.", true
	case triggerID != "":
		return assistantFeatureStep("Read an external trigger.", "nopsai.get_external_trigger", args), "Return the requested external trigger.", true
	default:
		return assistantFeatureStep("List visible external triggers.", "nopsai.list_external_triggers", map[string]any{}), "Return visible external triggers.", true
	}
}

func assistantConfigRepoFeatureStep(content string, plan assistantTurnPlan, confirm bool) (assistantPlanStep, string, bool) {
	lower := plan.LowerContent
	if !containsAny(lower, "config repo", "config repository", "config-repo", "config sync", "gitops repo", "gitops repository", "drift") {
		return assistantPlanStep{}, "", false
	}
	args := map[string]any{"folder_id": assistantFeatureFolderID(content)}
	switch {
	case containsAny(lower, "drift"):
		return assistantFeatureStep("Read config repository drift.", "nopsai.get_config_repo_drift", args), "Return GitOps config repository drift.", true
	case containsAny(lower, "sync all"):
		return assistantFeatureStep("Sync all config repositories with confirmation enforcement.", "nopsai.sync_all_config_repos", assistantFeatureConfirmArgs(map[string]any{}, confirm)), "Sync all config repositories only when explicitly confirmed and allowed.", true
	case containsAny(lower, "sync system config", "system config sync"):
		return assistantFeatureStep("Run system config sync with confirmation enforcement.", "nopsai.sync_system_config", assistantFeatureConfirmArgs(map[string]any{}, confirm)), "Run system config sync only when explicitly confirmed and allowed.", true
	case containsAny(lower, "sync"):
		return assistantFeatureStep("Sync the config repository with confirmation enforcement.", "nopsai.sync_config_repo", assistantFeatureConfirmArgs(args, confirm)), "Sync the config repository only when explicitly confirmed and allowed.", true
	case containsAny(lower, "write", "commit", "push"):
		args["message"] = content
		return assistantFeatureStep("Write config repository files through GitOps confirmation enforcement.", "nopsai.write_config_repo", assistantFeatureConfirmArgs(args, confirm)), "Write config repository files only when explicitly confirmed and allowed.", true
	case containsAny(lower, "list", "all repos", "all repositories"):
		return assistantFeatureStep("List configured config repositories.", "nopsai.list_config_repos", map[string]any{}), "Return configured config repositories.", true
	case containsAny(lower, "status"):
		return assistantFeatureStep("Read system config sync status.", "nopsai.get_config_sync_status", map[string]any{}), "Return system config sync status.", true
	default:
		return assistantFeatureStep("Read the config repository binding.", "nopsai.get_config_repo", args), "Return the config repository binding.", true
	}
}

func assistantNotificationFeatureStep(content string, plan assistantTurnPlan, confirm bool) (assistantPlanStep, string, bool) {
	lower := plan.LowerContent
	if !containsAny(lower, "notification", "notifications", "smtp", "mail settings", "email settings") {
		return assistantPlanStep{}, "", false
	}
	if containsAny(lower, "route") {
		args := map[string]any{"folder_id": assistantFeatureFolderID(content), "group_path": assistantFeatureFolderID(content), "message": content}
		switch {
		case containsAny(lower, "delete", "remove"):
			return assistantFeatureStep("Draft a GitOps notification route delete plan.", "nopsai.propose_notification_route_delete", args), "Return a GitOps notification route delete plan without applying changes.", true
		case containsAny(lower, "update", "change", "modify", "create", "add"):
			return assistantFeatureStep("Draft a GitOps notification route update plan.", "nopsai.propose_notification_route_update", args), "Return a GitOps notification route plan without applying changes.", true
		default:
			return assistantFeatureStep("Read a folder notification route.", "nopsai.get_notification_route", args), "Return the requested notification route.", true
		}
	}
	args := map[string]any{"message": content}
	if email := assistantFeatureEmail(content); email != "" {
		args["to"] = email
	}
	switch {
	case containsAny(lower, "test", "send test"):
		return assistantFeatureStep("Send a notification mail test with confirmation enforcement.", "nopsai.test_notification_mail_settings", assistantFeatureConfirmArgs(args, confirm)), "Send a test email only when explicitly confirmed and allowed.", true
	case containsAny(lower, "update", "change", "modify", "create", "configure", "propose"):
		return assistantFeatureStep("Draft a GitOps notification mail settings plan.", "nopsai.propose_notification_mail_settings", args), "Return a GitOps notification mail settings plan without applying changes.", true
	default:
		return assistantFeatureStep("Read notification mail settings.", "nopsai.get_notification_mail_settings", map[string]any{}), "Return notification mail settings.", true
	}
}

func assistantMonitoringFeatureStep(content string, plan assistantTurnPlan, confirm bool) (assistantPlanStep, string, bool) {
	lower := plan.LowerContent
	if !containsAny(lower, "monitoring", "analytics", "reliability", "efficiency", "security", "alert", "recommendation") {
		return assistantPlanStep{}, "", false
	}
	args := assistantMonitoringArgsFromPlan(plan)
	switch {
	case containsAny(lower, "saved view", "monitoring view", "views"):
		return assistantMonitoringViewStep(content, lower, confirm)
	case containsAny(lower, "alert event", "alert events"):
		return assistantFeatureStep("List monitoring alert events.", "nopsai.list_monitoring_alert_events", map[string]any{}), "Return monitoring alert events.", true
	case containsAny(lower, "alert rule", "alert rules"):
		return assistantMonitoringAlertRuleStep(content, lower, confirm)
	case containsAny(lower, "recommendation", "recommendations"):
		return assistantMonitoringRecommendationStep(content, lower, confirm)
	case containsAny(lower, "pipeline performance"):
		return assistantFeatureStep("Read pipeline performance analytics.", "nopsai.get_monitoring_pipeline_performance", args), "Return monitoring pipeline performance analytics.", true
	case containsAny(lower, "step performance"):
		return assistantFeatureStep("Read step performance analytics.", "nopsai.get_monitoring_step_performance", args), "Return monitoring step performance analytics.", true
	case containsAny(lower, "task performance"):
		return assistantFeatureStep("Read task performance analytics.", "nopsai.get_monitoring_task_performance", args), "Return monitoring task performance analytics.", true
	case containsAny(lower, "external trigger analytics"):
		return assistantFeatureStep("Read external trigger analytics.", "nopsai.get_monitoring_external_trigger_analytics", args), "Return external trigger analytics.", true
	case containsAny(lower, "trigger analytics"):
		return assistantFeatureStep("Read trigger analytics.", "nopsai.get_monitoring_trigger_analytics", args), "Return trigger analytics.", true
	case containsAny(lower, "runner history"):
		return assistantFeatureStep("Read monitoring runner history.", "nopsai.get_monitoring_runner_history", args), "Return monitoring runner history.", true
	case containsAny(lower, "reliability"):
		return assistantFeatureStep("Read reliability analytics.", "nopsai.get_monitoring_reliability", args), "Return monitoring reliability analytics.", true
	case containsAny(lower, "efficiency"):
		return assistantFeatureStep("Read efficiency analytics.", "nopsai.get_monitoring_efficiency", args), "Return monitoring efficiency analytics.", true
	case containsAny(lower, "security"):
		return assistantFeatureStep("Read security analytics.", "nopsai.get_monitoring_security", args), "Return monitoring security analytics.", true
	case containsAny(lower, "run analytics"):
		return assistantFeatureStep("Read run analytics.", "nopsai.get_monitoring_run_analytics", args), "Return monitoring run analytics.", true
	default:
		return assistantPlanStep{}, "", false
	}
}

func assistantDataOpsFeatureStep(content string, plan assistantTurnPlan, confirm bool) (assistantPlanStep, string, bool) {
	lower := plan.LowerContent
	if containsAny(lower, "backup", "backups") {
		args := map[string]any{
			"backup_id":   assistantFeatureValueAfterAnyLabel(content, "backup id", "backup"),
			"backup_type": assistantFeatureValueAfterAnyLabel(content, "backup type", "type"),
		}
		switch {
		case containsAny(lower, "create", "take", "run", "start"):
			return assistantFeatureStep("Create a data backup with confirmation enforcement.", "nopsai.create_data_backup", assistantFeatureConfirmArgs(args, confirm)), "Create the backup only when explicitly confirmed and allowed.", true
		case containsAny(lower, "delete", "remove"):
			return assistantFeatureStep("Delete a data backup with confirmation enforcement.", "nopsai.delete_data_backup", assistantFeatureConfirmArgs(args, confirm)), "Delete the backup only when explicitly confirmed and allowed.", true
		default:
			return assistantFeatureStep("List data backups.", "nopsai.list_data_backups", map[string]any{}), "Return data backups.", true
		}
	}
	if !containsAny(lower, "cleanup", "retention") {
		return assistantPlanStep{}, "", false
	}
	args := assistantDataCleanupArgs(content, plan)
	switch {
	case containsAny(lower, "job", "jobs"):
		return assistantFeatureStep("List data cleanup jobs.", "nopsai.list_data_cleanup_jobs", map[string]any{}), "Return data cleanup jobs.", true
	case containsAny(lower, "schedule", "schedules") && containsAny(lower, "create", "add", "new"):
		return assistantFeatureStep("Create a data cleanup schedule with confirmation enforcement.", "nopsai.create_data_cleanup_schedule", assistantFeatureConfirmArgs(args, confirm)), "Create the cleanup schedule only when explicitly confirmed and allowed.", true
	case containsAny(lower, "schedule", "schedules") && containsAny(lower, "update", "change", "modify"):
		return assistantFeatureStep("Update a data cleanup schedule with confirmation enforcement.", "nopsai.update_data_cleanup_schedule", assistantFeatureConfirmArgs(args, confirm)), "Update the cleanup schedule only when explicitly confirmed and allowed.", true
	case containsAny(lower, "schedule", "schedules") && containsAny(lower, "delete", "remove"):
		return assistantFeatureStep("Delete a data cleanup schedule with confirmation enforcement.", "nopsai.delete_data_cleanup_schedule", assistantFeatureConfirmArgs(args, confirm)), "Delete the cleanup schedule only when explicitly confirmed and allowed.", true
	case containsAny(lower, "schedule", "schedules") && containsAny(lower, "enable"):
		return assistantFeatureStep("Enable a data cleanup schedule with confirmation enforcement.", "nopsai.enable_data_cleanup_schedule", assistantFeatureConfirmArgs(args, confirm)), "Enable the cleanup schedule only when explicitly confirmed and allowed.", true
	case containsAny(lower, "schedule", "schedules") && containsAny(lower, "disable", "pause"):
		return assistantFeatureStep("Disable a data cleanup schedule with confirmation enforcement.", "nopsai.disable_data_cleanup_schedule", assistantFeatureConfirmArgs(args, confirm)), "Disable the cleanup schedule only when explicitly confirmed and allowed.", true
	case containsAny(lower, "schedule", "schedules") && containsAny(lower, "run"):
		return assistantFeatureStep("Run a data cleanup schedule with confirmation enforcement.", "nopsai.run_data_cleanup_schedule", assistantFeatureConfirmArgs(args, confirm)), "Run the cleanup schedule only when explicitly confirmed and allowed.", true
	case containsAny(lower, "schedule", "schedules"):
		return assistantFeatureStep("List data cleanup schedules.", "nopsai.list_data_cleanup_schedules", map[string]any{}), "Return data cleanup schedules.", true
	case containsAny(lower, "run", "execute", "apply"):
		return assistantFeatureStep("Run data cleanup with confirmation enforcement.", "nopsai.run_data_cleanup", assistantFeatureConfirmArgs(args, confirm)), "Run cleanup only when explicitly confirmed and allowed.", true
	default:
		return assistantFeatureStep("Preview data cleanup impact.", "nopsai.preview_data_cleanup", args), "Preview data cleanup impact without deleting data.", true
	}
}

func assistantCredentialFeatureStep(content string, plan assistantTurnPlan, confirm bool) (assistantPlanStep, string, bool) {
	lower := plan.LowerContent
	if !containsAny(lower, "credential", "credentials") {
		return assistantPlanStep{}, "", false
	}
	credentialID := assistantFeatureValueAfterAnyLabel(content, "credential id", "credential")
	args := map[string]any{
		"credential_id": credentialID,
		"reference":     assistantFeatureValueAfterAnyLabel(content, "reference", "credential reference", "ref"),
		"kind":          assistantFeatureValueAfterAnyLabel(content, "kind", "type"),
		"description":   content,
		"message":       content,
	}
	switch {
	case containsAny(lower, "gitops", "git ops", "propose"):
		return assistantFeatureStep("Draft a GitOps credential plan.", "nopsai.propose_credential_gitops", args), "Return a credential GitOps plan without applying changes.", true
	case containsAny(lower, "rotate"):
		return assistantFeatureStep("Rotate credential value with confirmation enforcement.", "nopsai.rotate_credential_value", assistantFeatureConfirmArgs(args, confirm)), "Rotate the credential only when explicitly confirmed and allowed.", true
	case containsAny(lower, "activate"):
		if version := assistantFeatureValueAfterAnyLabel(content, "version"); version != "" {
			args["version"] = version
		}
		return assistantFeatureStep("Activate credential version with confirmation enforcement.", "nopsai.activate_credential_version", assistantFeatureConfirmArgs(args, confirm)), "Activate the credential version only when explicitly confirmed and allowed.", true
	case containsAny(lower, "disable"):
		return assistantFeatureStep("Disable credential with confirmation enforcement.", "nopsai.disable_credential", assistantFeatureConfirmArgs(args, confirm)), "Disable the credential only when explicitly confirmed and allowed.", true
	case containsAny(lower, "enable"):
		return assistantFeatureStep("Enable credential with confirmation enforcement.", "nopsai.enable_credential", assistantFeatureConfirmArgs(args, confirm)), "Enable the credential only when explicitly confirmed and allowed.", true
	case containsAny(lower, "delete", "remove") && containsAny(lower, "version"):
		if version := assistantFeatureValueAfterAnyLabel(content, "version"); version != "" {
			args["version"] = version
		}
		return assistantFeatureStep("Delete credential version with confirmation enforcement.", "nopsai.delete_credential_version", assistantFeatureConfirmArgs(args, confirm)), "Delete the credential version only when explicitly confirmed and allowed.", true
	case containsAny(lower, "delete", "remove"):
		return assistantFeatureStep("Delete credential with confirmation enforcement.", "nopsai.delete_credential", assistantFeatureConfirmArgs(args, confirm)), "Delete the credential only when explicitly confirmed and allowed.", true
	case containsAny(lower, "create", "add", "new"):
		return assistantFeatureStep("Create credential with confirmation enforcement.", "nopsai.create_credential", assistantFeatureConfirmArgs(args, confirm)), "Create the credential only when explicitly confirmed and allowed.", true
	case credentialID != "" && containsAny(lower, "get", "read", "show", "metadata"):
		return assistantFeatureStep("Read credential metadata.", "nopsai.get_credential_metadata", args), "Return credential metadata only.", true
	default:
		return assistantFeatureStep("List credential metadata.", "nopsai.list_credentials_metadata", map[string]any{}), "Return credential metadata without values.", true
	}
}

func assistantSecretFeatureStep(content string, plan assistantTurnPlan, confirm bool) (assistantPlanStep, string, bool) {
	lower := plan.LowerContent
	if !containsAny(lower, "secret", "secrets") {
		return assistantPlanStep{}, "", false
	}
	args := assistantScopedValueArgs(content, plan, "secret")
	switch {
	case containsAny(lower, "scope", "scopes"):
		return assistantFeatureStep("List secret scopes using metadata only.", "nopsai.list_secret_scopes", map[string]any{}), "Return secret scope metadata without plaintext values.", true
	case containsAny(lower, "encrypt"):
		return assistantFeatureStep("Encrypt a secret value for GitOps.", "nopsai.encrypt_secret_for_gitops", assistantFeatureConfirmArgs(args, confirm)), "Encrypt secret material for GitOps without exposing plaintext in audit.", true
	case containsAny(lower, "gitops", "git ops", "propose") && containsAny(lower, "delete", "remove"):
		return assistantFeatureStep("Draft a GitOps secret delete plan.", "nopsai.propose_secret_gitops_delete", args), "Return a secret GitOps delete plan without applying changes.", true
	case containsAny(lower, "gitops", "git ops", "propose"):
		return assistantFeatureStep("Draft a GitOps secret write plan.", "nopsai.propose_secret_gitops_write", args), "Return a secret GitOps write plan without applying changes.", true
	case containsAny(lower, "write", "set", "create", "update"):
		return assistantFeatureStep("Write a secret value with confirmation enforcement.", "nopsai.write_secret_value", assistantFeatureConfirmArgs(args, confirm)), "Write the secret only when explicitly confirmed and allowed.", true
	case containsAny(lower, "delete", "remove"):
		return assistantFeatureStep("Delete a secret value with confirmation enforcement.", "nopsai.delete_secret_value", assistantFeatureConfirmArgs(args, confirm)), "Delete the secret only when explicitly confirmed and allowed.", true
	case containsAny(lower, "list", "show", "metadata"):
		return assistantFeatureStep("List secret metadata only.", "nopsai.list_secrets_metadata", args), "Return secret metadata without plaintext values.", true
	default:
		return assistantPlanStep{}, "", false
	}
}

func assistantVariableFeatureStep(content string, plan assistantTurnPlan, confirm bool) (assistantPlanStep, string, bool) {
	lower := plan.LowerContent
	if !containsAny(lower, "variable", "variables", "env", "envs", "environment variable", "environment variables") {
		return assistantPlanStep{}, "", false
	}
	args := assistantScopedValueArgs(content, plan, "variable")
	switch {
	case containsAny(lower, "scope", "scopes"):
		return assistantFeatureStep("List variable scopes using metadata only.", "nopsai.list_variable_scopes", map[string]any{}), "Return variable scope metadata.", true
	case containsAny(lower, "gitops", "git ops", "propose") && containsAny(lower, "delete", "remove"):
		return assistantFeatureStep("Draft a GitOps variable delete plan.", "nopsai.propose_variable_gitops_delete", args), "Return a variable GitOps delete plan without applying changes.", true
	case containsAny(lower, "gitops", "git ops", "propose"):
		return assistantFeatureStep("Draft a GitOps variable write plan.", "nopsai.propose_variable_gitops_write", args), "Return a variable GitOps write plan without applying changes.", true
	case containsAny(lower, "write", "set", "create", "update"):
		return assistantFeatureStep("Write a variable value with confirmation enforcement.", "nopsai.write_variable_value", assistantFeatureConfirmArgs(args, confirm)), "Write the variable only when explicitly confirmed and allowed.", true
	case containsAny(lower, "delete", "remove"):
		return assistantFeatureStep("Delete a variable value with confirmation enforcement.", "nopsai.delete_variable_value", assistantFeatureConfirmArgs(args, confirm)), "Delete the variable only when explicitly confirmed and allowed.", true
	case containsAny(lower, "value", "read"):
		return assistantFeatureStep("Read a variable value when allowed.", "nopsai.get_variable_value", args), "Return the variable value only when the current subject is allowed.", true
	case containsAny(lower, "list", "show", "metadata"):
		return assistantFeatureStep("List variable metadata.", "nopsai.list_variables_metadata", args), "Return variable metadata.", true
	default:
		return assistantPlanStep{}, "", false
	}
}

func assistantRunnerFeatureStep(content string, plan assistantTurnPlan, confirm bool) (assistantPlanStep, string, bool) {
	lower := plan.LowerContent
	if !containsAny(lower, "runner", "runners") {
		return assistantPlanStep{}, "", false
	}
	args := map[string]any{
		"runner_id":       assistantFeatureValueAfterAnyLabel(content, "runner id", "runner"),
		"runner_scopes":   firstNonEmptyString(plan.Scope, assistantFeatureValueAfterAnyLabel(content, "scope")),
		"namespace":       assistantFeatureValueAfterAnyLabel(content, "namespace"),
		"service_account": assistantFeatureValueAfterAnyLabel(content, "service account"),
	}
	switch {
	case containsAny(lower, "kubernetes", "k8s") && containsAny(lower, "bootstrap command", "bootstrap"):
		return assistantFeatureStep("Generate a Kubernetes runner bootstrap command.", "nopsai.generate_kubernetes_runner_bootstrap_command", args), "Return a Kubernetes runner bootstrap command with sensitive-response controls.", true
	case containsAny(lower, "bootstrap command", "bootstrap"):
		return assistantFeatureStep("Generate a runner bootstrap command.", "nopsai.generate_runner_bootstrap_command", args), "Return a runner bootstrap command with sensitive-response controls.", true
	case containsAny(lower, "kubernetes", "k8s", "manifest"):
		return assistantFeatureStep("Generate Kubernetes runner manifest.", "nopsai.generate_kubernetes_runner_manifest", args), "Return Kubernetes runner installation artifacts.", true
	case containsAny(lower, "compose", "docker compose"):
		return assistantFeatureStep("Generate Docker Compose runner artifacts.", "nopsai.generate_runner_compose", args), "Return Docker Compose runner installation artifacts.", true
	case containsAny(lower, "pause", "disable dispatch", "stop dispatch"):
		args["allow_dispatch"] = false
		return assistantFeatureStep("Pause runner dispatch with confirmation enforcement.", "nopsai.update_runner_dispatch", assistantFeatureConfirmArgs(args, confirm)), "Pause runner dispatch only when explicitly confirmed and allowed.", true
	case containsAny(lower, "resume", "enable dispatch", "allow dispatch"):
		args["allow_dispatch"] = true
		return assistantFeatureStep("Resume runner dispatch with confirmation enforcement.", "nopsai.update_runner_dispatch", assistantFeatureConfirmArgs(args, confirm)), "Resume runner dispatch only when explicitly confirmed and allowed.", true
	default:
		return assistantPlanStep{}, "", false
	}
}

func assistantAccessAdminFeatureStep(content string, plan assistantTurnPlan, confirm bool) (assistantPlanStep, string, bool) {
	lower := plan.LowerContent
	if containsAny(lower, "audit log", "audit logs") {
		return assistantFeatureStep("List AAA audit logs.", "nopsai.list_audit_logs", map[string]any{"limit": 50}), "Return visible audit log entries.", true
	}
	if containsAny(lower, "identity provider", "identity providers", "idp") {
		args := map[string]any{"provider": assistantFeatureValueAfterAnyLabel(content, "provider", "identity provider")}
		if containsAny(lower, "update", "change", "configure") {
			return assistantFeatureStep("Update identity provider settings with confirmation enforcement.", "nopsai.update_admin_identity_provider", assistantFeatureConfirmArgs(args, confirm)), "Update identity provider settings only when explicitly confirmed and allowed.", true
		}
		return assistantFeatureStep("List identity provider configuration.", "nopsai.list_admin_identity_providers", map[string]any{}), "Return identity provider admin configuration.", true
	}
	if containsAny(lower, "service account", "service accounts") {
		id := assistantFeatureValueAfterAnyLabel(content, "service account id", "service account")
		args := map[string]any{"service_account_id": id}
		switch {
		case containsAny(lower, "create", "add", "new"):
			return assistantFeatureStep("Create an admin service account with confirmation enforcement.", "nopsai.create_admin_service_account", assistantFeatureConfirmArgs(args, confirm)), "Create the service account only when explicitly confirmed and allowed.", true
		case containsAny(lower, "update", "change", "modify"):
			return assistantFeatureStep("Update an admin service account with confirmation enforcement.", "nopsai.update_admin_service_account", assistantFeatureConfirmArgs(args, confirm)), "Update the service account only when explicitly confirmed and allowed.", true
		case containsAny(lower, "delete", "remove"):
			return assistantFeatureStep("Delete an admin service account with confirmation enforcement.", "nopsai.delete_admin_service_account", assistantFeatureConfirmArgs(args, confirm)), "Delete the service account only when explicitly confirmed and allowed.", true
		default:
			return assistantFeatureStep("List admin service accounts.", "nopsai.list_admin_service_accounts", map[string]any{}), "Return admin service accounts.", true
		}
	}
	if containsAny(lower, "admin user", "admin users", "users") && containsAny(lower, "admin", "iam") {
		id := assistantFeatureValueAfterAnyLabel(content, "user id", "user")
		args := map[string]any{"user_id": id}
		switch {
		case containsAny(lower, "create", "add", "new"):
			return assistantFeatureStep("Create an admin user with confirmation enforcement.", "nopsai.create_admin_user", assistantFeatureConfirmArgs(args, confirm)), "Create the user only when explicitly confirmed and allowed.", true
		case containsAny(lower, "update", "change", "modify"):
			return assistantFeatureStep("Update an admin user with confirmation enforcement.", "nopsai.update_admin_user", assistantFeatureConfirmArgs(args, confirm)), "Update the user only when explicitly confirmed and allowed.", true
		case containsAny(lower, "delete", "remove"):
			return assistantFeatureStep("Delete an admin user with confirmation enforcement.", "nopsai.delete_admin_user", assistantFeatureConfirmArgs(args, confirm)), "Delete the user only when explicitly confirmed and allowed.", true
		default:
			return assistantFeatureStep("List admin users.", "nopsai.list_admin_users", map[string]any{}), "Return admin users.", true
		}
	}
	if containsAny(lower, "admin role", "admin roles", "roles") && containsAny(lower, "admin", "iam") {
		if containsAny(lower, "create", "add", "new") {
			return assistantFeatureStep("Create an admin role with confirmation enforcement.", "nopsai.create_admin_role", assistantFeatureConfirmArgs(map[string]any{}, confirm)), "Create the role only when explicitly confirmed and allowed.", true
		}
		if containsAny(lower, "delete", "remove") {
			return assistantFeatureStep("Delete an admin role with confirmation enforcement.", "nopsai.delete_admin_role", assistantFeatureConfirmArgs(map[string]any{}, confirm)), "Delete the role only when explicitly confirmed and allowed.", true
		}
		return assistantFeatureStep("List admin roles.", "nopsai.list_admin_roles", map[string]any{}), "Return admin roles.", true
	}
	if containsAny(lower, "effective permission", "effective permissions") {
		args := map[string]any{
			"action":        assistantFeatureValueAfterAnyLabel(content, "action", "permission"),
			"resource_type": assistantFeatureValueAfterAnyLabel(content, "resource type"),
			"resource_id":   assistantFeatureValueAfterAnyLabel(content, "resource id"),
		}
		return assistantFeatureStep("Check effective permissions.", "nopsai.get_effective_permissions", args), "Return effective permission context.", true
	}
	if containsAny(lower, "resource use") {
		args := map[string]any{
			"caller_type":   assistantFeatureValueAfterAnyLabel(content, "caller type"),
			"caller_id":     assistantFeatureValueAfterAnyLabel(content, "caller id"),
			"resource_type": assistantFeatureValueAfterAnyLabel(content, "resource type"),
			"resource_id":   assistantFeatureValueAfterAnyLabel(content, "resource id"),
			"action":        assistantFeatureValueAfterAnyLabel(content, "action"),
		}
		if containsAny(lower, "batch") {
			return assistantFeatureStep("Run batch resource-use authorization checks.", "nopsai.batch_check_resource_use", args), "Return batch resource-use authorization results.", true
		}
		return assistantFeatureStep("Run a resource-use authorization check.", "nopsai.check_resource_use", args), "Return resource-use authorization result.", true
	}
	if containsAny(lower, "resource access") {
		args := map[string]any{
			"resource_type": assistantFeatureValueAfterAnyLabel(content, "resource type"),
			"resource_id":   assistantFeatureValueAfterAnyLabel(content, "resource id"),
			"grant_id":      assistantFeatureValueAfterAnyLabel(content, "grant id", "grant"),
			"subject_type":  assistantFeatureValueAfterAnyLabel(content, "subject type"),
			"subject_id":    assistantFeatureValueAfterAnyLabel(content, "subject id"),
		}
		switch {
		case containsAny(lower, "create", "add", "grant"):
			return assistantFeatureStep("Create a resource-use grant with confirmation enforcement.", "nopsai.create_resource_use_grant", assistantFeatureConfirmArgs(args, confirm)), "Create the resource-use grant only when explicitly confirmed and allowed.", true
		case containsAny(lower, "delete", "remove"):
			return assistantFeatureStep("Delete a resource access grant with confirmation enforcement.", "nopsai.delete_resource_access_grant", assistantFeatureConfirmArgs(args, confirm)), "Delete the resource access grant only when explicitly confirmed and allowed.", true
		case containsAny(lower, "update", "change", "visibility"):
			return assistantFeatureStep("Update resource access with confirmation enforcement.", "nopsai.update_resource_access", assistantFeatureConfirmArgs(args, confirm)), "Update resource access only when explicitly confirmed and allowed.", true
		default:
			return assistantFeatureStep("Read resource access settings.", "nopsai.get_resource_access", args), "Return resource access settings.", true
		}
	}
	if containsAny(lower, "access grant", "access grants") {
		args := map[string]any{
			"grant_id":      assistantFeatureValueAfterAnyLabel(content, "grant id", "grant"),
			"subject_type":  assistantFeatureValueAfterAnyLabel(content, "subject type"),
			"subject_id":    assistantFeatureValueAfterAnyLabel(content, "subject id"),
			"role":          assistantFeatureValueAfterAnyLabel(content, "role"),
			"resource_type": assistantFeatureValueAfterAnyLabel(content, "resource type"),
			"resource_id":   assistantFeatureValueAfterAnyLabel(content, "resource id"),
		}
		switch {
		case containsAny(lower, "create", "add", "grant access"):
			return assistantFeatureStep("Create an access grant with confirmation enforcement.", "nopsai.create_access_grant", assistantFeatureConfirmArgs(args, confirm)), "Create the access grant only when explicitly confirmed and allowed.", true
		case containsAny(lower, "delete", "remove", "revoke"):
			return assistantFeatureStep("Delete an access grant with confirmation enforcement.", "nopsai.delete_access_grant", assistantFeatureConfirmArgs(args, confirm)), "Delete the access grant only when explicitly confirmed and allowed.", true
		default:
			return assistantFeatureStep("List access grants.", "nopsai.list_access_grants", args), "Return access grants visible to the current subject.", true
		}
	}
	return assistantPlanStep{}, "", false
}

func assistantKnowledgeFeatureStep(content string, plan assistantTurnPlan) (assistantPlanStep, string, bool) {
	lower := plan.LowerContent
	if !containsAny(lower, "knowledge context", "knowledge doc", "runbook", "guardrail", "guideline", "adr") {
		return assistantPlanStep{}, "", false
	}
	if !containsAny(lower, "create", "add", "new", "update", "change", "delete", "remove") {
		return assistantPlanStep{}, "", false
	}
	args := map[string]any{
		"id":          assistantFeatureValueAfterAnyLabel(content, "context id", "knowledge context", "id"),
		"kind":        firstNonEmptyString(assistantFeatureValueAfterAnyLabel(content, "kind"), "runbook"),
		"group":       assistantFeatureValueAfterAnyLabel(content, "group", "group path"),
		"name":        assistantFeatureValueAfterAnyLabel(content, "name"),
		"description": content,
		"content":     content,
		"message":     content,
	}
	switch {
	case containsAny(lower, "delete", "remove"):
		return assistantFeatureStep("Draft a GitOps knowledge context delete plan.", "nopsai.propose_knowledge_context_delete", args), "Return a knowledge context delete plan without applying changes.", true
	case containsAny(lower, "update", "change", "modify"):
		return assistantFeatureStep("Draft a GitOps knowledge context update plan.", "nopsai.propose_knowledge_context_update", args), "Return a knowledge context update plan without applying changes.", true
	default:
		return assistantFeatureStep("Draft a GitOps knowledge context create plan.", "nopsai.propose_knowledge_context_create", args), "Return a knowledge context create plan without applying changes.", true
	}
}

func assistantReusableStepFeatureStep(content string, plan assistantTurnPlan) (assistantPlanStep, string, bool) {
	lower := plan.LowerContent
	if !containsAny(lower, "reusable step", "reusable steps") {
		return assistantPlanStep{}, "", false
	}
	if !containsAny(lower, "create", "add", "new", "update", "change", "delete", "remove", "propose") {
		return assistantPlanStep{}, "", false
	}
	args := map[string]any{
		"step":       assistantFeatureValueAfterAnyLabel(content, "step id", "step"),
		"name":       assistantFeatureValueAfterAnyLabel(content, "name"),
		"path":       assistantFeatureValueAfterAnyLabel(content, "path"),
		"definition": firstNonEmptyString(assistantYAMLFromMessage(content), content),
		"message":    content,
	}
	switch {
	case containsAny(lower, "delete", "remove"):
		return assistantFeatureStep("Draft a GitOps reusable step delete plan.", "nopsai.propose_reusable_step_delete", args), "Return a reusable step delete plan without applying changes.", true
	case containsAny(lower, "update", "change", "modify"):
		return assistantFeatureStep("Draft a GitOps reusable step update plan.", "nopsai.propose_reusable_step_update", args), "Return a reusable step update plan without applying changes.", true
	default:
		return assistantFeatureStep("Draft a GitOps reusable step create plan.", "nopsai.propose_reusable_step_create", args), "Return a reusable step create plan without applying changes.", true
	}
}

func assistantUIContextFeatureStep(content string, plan assistantTurnPlan) (assistantPlanStep, string, bool) {
	lower := plan.LowerContent
	if !containsAny(lower, "ui context", "ui ownership", "mcp ui", "ui should", "what should ui", "what should the ui") {
		return assistantPlanStep{}, "", false
	}
	args := map[string]any{"area": assistantFeatureUIArea(lower)}
	return assistantFeatureStep("Read UI ownership context for MCP-backed surfaces.", "nopsai.get_ui_context", args), "Return UI ownership hints for MCP-backed NopsAI surfaces.", true
}

func assistantMonitoringViewStep(content, lower string, confirm bool) (assistantPlanStep, string, bool) {
	args := map[string]any{"view_id": assistantFeatureValueAfterAnyLabel(content, "view id", "view")}
	switch {
	case containsAny(lower, "create", "add", "new"):
		return assistantFeatureStep("Create a monitoring saved view with confirmation enforcement.", "nopsai.create_monitoring_view", assistantFeatureConfirmArgs(args, confirm)), "Create the monitoring view only when explicitly confirmed and allowed.", true
	case containsAny(lower, "update", "change", "modify"):
		return assistantFeatureStep("Update a monitoring saved view with confirmation enforcement.", "nopsai.update_monitoring_view", assistantFeatureConfirmArgs(args, confirm)), "Update the monitoring view only when explicitly confirmed and allowed.", true
	case containsAny(lower, "delete", "remove"):
		return assistantFeatureStep("Delete a monitoring saved view with confirmation enforcement.", "nopsai.delete_monitoring_view", assistantFeatureConfirmArgs(args, confirm)), "Delete the monitoring view only when explicitly confirmed and allowed.", true
	default:
		return assistantFeatureStep("List monitoring saved views.", "nopsai.list_monitoring_views", map[string]any{}), "Return monitoring saved views.", true
	}
}

func assistantMonitoringAlertRuleStep(content, lower string, confirm bool) (assistantPlanStep, string, bool) {
	args := map[string]any{"rule_id": assistantFeatureValueAfterAnyLabel(content, "rule id", "alert rule", "rule")}
	switch {
	case containsAny(lower, "evaluate", "test"):
		return assistantFeatureStep("Evaluate a monitoring alert rule with confirmation enforcement.", "nopsai.evaluate_monitoring_alert_rule", assistantFeatureConfirmArgs(args, confirm)), "Evaluate the alert rule only when explicitly confirmed and allowed.", true
	case containsAny(lower, "create", "add", "new"):
		return assistantFeatureStep("Create a monitoring alert rule with confirmation enforcement.", "nopsai.create_monitoring_alert_rule", assistantFeatureConfirmArgs(args, confirm)), "Create the alert rule only when explicitly confirmed and allowed.", true
	case containsAny(lower, "update", "change", "modify"):
		return assistantFeatureStep("Update a monitoring alert rule with confirmation enforcement.", "nopsai.update_monitoring_alert_rule", assistantFeatureConfirmArgs(args, confirm)), "Update the alert rule only when explicitly confirmed and allowed.", true
	case containsAny(lower, "delete", "remove"):
		return assistantFeatureStep("Delete a monitoring alert rule with confirmation enforcement.", "nopsai.delete_monitoring_alert_rule", assistantFeatureConfirmArgs(args, confirm)), "Delete the alert rule only when explicitly confirmed and allowed.", true
	default:
		return assistantFeatureStep("List monitoring alert rules.", "nopsai.list_monitoring_alert_rules", map[string]any{}), "Return monitoring alert rules.", true
	}
}

func assistantMonitoringRecommendationStep(content, lower string, confirm bool) (assistantPlanStep, string, bool) {
	args := map[string]any{"recommendation_id": assistantFeatureValueAfterAnyLabel(content, "recommendation id", "recommendation")}
	switch {
	case containsAny(lower, "ack", "acknowledge"):
		return assistantFeatureStep("Acknowledge a monitoring recommendation with confirmation enforcement.", "nopsai.acknowledge_monitoring_recommendation", assistantFeatureConfirmArgs(args, confirm)), "Acknowledge the recommendation only when explicitly confirmed and allowed.", true
	case containsAny(lower, "resolve", "close"):
		return assistantFeatureStep("Resolve a monitoring recommendation with confirmation enforcement.", "nopsai.resolve_monitoring_recommendation", assistantFeatureConfirmArgs(args, confirm)), "Resolve the recommendation only when explicitly confirmed and allowed.", true
	default:
		return assistantFeatureStep("List monitoring recommendations.", "nopsai.list_monitoring_recommendations", map[string]any{}), "Return monitoring recommendations.", true
	}
}

func assistantFeatureStep(thought, toolName string, args map[string]any) assistantPlanStep {
	return assistantPlanStep{
		Thought:  strings.TrimSpace(thought),
		ToolName: strings.TrimSpace(toolName),
		Args:     assistantFeatureCleanArgs(args),
	}
}

func assistantFeatureCleanArgs(args map[string]any) map[string]any {
	clean := map[string]any{}
	for key, value := range args {
		switch typed := value.(type) {
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed == "" {
				continue
			}
			clean[key] = trimmed
		case nil:
			continue
		default:
			clean[key] = value
		}
	}
	return clean
}

func assistantFeatureConfirmArgs(args map[string]any, confirm bool) map[string]any {
	if args == nil {
		args = map[string]any{}
	}
	args["confirm"] = confirm
	return args
}

func assistantFeatureConfirmed(lower string) bool {
	return containsAny(lower, "confirm", "confirmed", "with confirmation", "i approve", "approved to execute", "apply it", "execute it")
}

func assistantExplicitHostedMCPToolName(content string) string {
	match := assistantExplicitToolPattern.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	name := strings.ToLower(strings.TrimSpace(match[1]))
	for _, tool := range allHostedMCPTools() {
		if tool.Name == name {
			return name
		}
	}
	return ""
}

func assistantFeatureValueAfterAnyLabel(content string, labels ...string) string {
	for _, label := range labels {
		if value := assistantFeatureValueAfterLabel(content, label); value != "" {
			return value
		}
	}
	return ""
}

func assistantFeatureValueAfterLabel(content, label string) string {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(label)))
	if len(parts) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		quoted = append(quoted, regexp.QuoteMeta(part))
	}
	pattern := regexp.MustCompile(`(?i)\b` + strings.Join(quoted, `\s+`) + `\s+([a-zA-Z0-9][a-zA-Z0-9._:/@-]{0,180})`)
	matches := pattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return ""
	}
	for idx := len(matches) - 1; idx >= 0; idx-- {
		if len(matches[idx]) < 2 {
			continue
		}
		value := strings.Trim(matches[idx][1], ".,;:!?\"'`()[]{}")
		if !assistantFeatureValuePattern.MatchString(value) || assistantFeatureValueCandidateIsGrammar(value) {
			continue
		}
		return value
	}
	return ""
}

func assistantFeatureValueCandidateIsGrammar(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "a", "an", "the", "this", "that", "these", "those", "with", "for", "from", "to", "of", "in", "on", "now", "please", "confirm", "confirmed", "metadata", "settings", "status", "list", "show", "create", "update", "delete", "enable", "disable", "run", "all", "manifest", "invocation", "invocations", "delivery", "deliveries":
		return true
	default:
		return false
	}
}

func assistantScheduleArgsFromMessage(content string, plan assistantTurnPlan, scheduleID string) map[string]any {
	args := map[string]any{
		"schedule_id":     scheduleID,
		"name":            assistantFeatureValueAfterAnyLabel(content, "name"),
		"path":            assistantFeatureValueAfterAnyLabel(content, "path"),
		"pipeline":        firstNonEmptyString(plan.PipelineID, assistantFeatureValueAfterAnyLabel(content, "pipeline id", "pipeline")),
		"cron_expression": assistantFeatureCronExpression(content),
		"timezone":        assistantFeatureValueAfterAnyLabel(content, "timezone", "time zone"),
		"scope":           plan.Scope,
		"message":         content,
	}
	lower := strings.ToLower(content)
	if containsAny(lower, "enabled") {
		args["enabled"] = true
	}
	if containsAny(lower, "disabled") {
		args["enabled"] = false
	}
	return args
}

func assistantFeatureCronExpression(content string) string {
	pattern := regexp.MustCompile(`(?i)\bcron(?:\s+expression)?\s+([0-9*/,\-\s]+)`)
	match := pattern.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	value := strings.TrimSpace(match[1])
	fields := strings.Fields(value)
	if len(fields) > 5 {
		fields = fields[:5]
	}
	if len(fields) < 5 {
		return ""
	}
	return strings.Join(fields, " ")
}

func assistantFeatureFolderID(content string) string {
	return assistantFeatureValueAfterAnyLabel(content, "folder id", "folder", "group path", "group")
}

func assistantFeatureEmail(content string) string {
	match := assistantFeatureEmailPattern.FindString(content)
	return strings.TrimSpace(match)
}

func assistantMonitoringArgsFromPlan(plan assistantTurnPlan) map[string]any {
	args := cloneAssistantArgs(plan.AIUsageFilters)
	if plan.RunID != "" {
		args["run_id"] = plan.RunID
	}
	if plan.PipelineID != "" {
		args["pipeline_path"] = plan.PipelineID
	}
	return args
}

func assistantDataCleanupArgs(content string, plan assistantTurnPlan) map[string]any {
	args := map[string]any{
		"schedule_id":            assistantFeatureValueAfterAnyLabel(content, "schedule id", "schedule"),
		"target":                 assistantFeatureValueAfterAnyLabel(content, "target"),
		"mode":                   assistantFeatureValueAfterAnyLabel(content, "mode"),
		"backup_before_cleanup":  !containsAny(plan.LowerContent, "without backup", "no backup"),
		"backup_before_clean_up": !containsAny(plan.LowerContent, "without backup", "no backup"),
	}
	if value := assistantFeatureNumberAfterAnyLabel(content, "keep last", "keep_last"); value > 0 {
		args["keep_last"] = value
	}
	if value := assistantFeatureNumberAfterAnyLabel(content, "older than", "older_than_days"); value > 0 {
		args["older_than_days"] = value
	}
	return args
}

func assistantFeatureNumberAfterAnyLabel(content string, labels ...string) int {
	for _, label := range labels {
		parts := strings.Fields(strings.ToLower(strings.TrimSpace(label)))
		if len(parts) == 0 {
			continue
		}
		quoted := make([]string, 0, len(parts))
		for _, part := range parts {
			quoted = append(quoted, regexp.QuoteMeta(part))
		}
		pattern := regexp.MustCompile(`(?i)\b` + strings.Join(quoted, `\s+`) + `\s+(\d{1,5})`)
		match := pattern.FindStringSubmatch(content)
		if len(match) < 2 {
			continue
		}
		var value int
		if _, err := fmt.Sscanf(match[1], "%d", &value); err == nil {
			return value
		}
	}
	return 0
}

func assistantScopedValueArgs(content string, plan assistantTurnPlan, kind string) map[string]any {
	nameLabels := []string{"name"}
	valueKey := "name"
	if kind == "secret" {
		nameLabels = append([]string{"secret name", "secret"}, nameLabels...)
		valueKey = "secret_name"
	} else {
		nameLabels = append([]string{"variable name", "variable", "env"}, nameLabels...)
		valueKey = "variable_name"
	}
	name := assistantFeatureValueAfterAnyLabel(content, nameLabels...)
	args := map[string]any{
		"name":       name,
		valueKey:     name,
		"repository": firstNonEmptyString(plan.Repository, assistantFeatureValueAfterAnyLabel(content, "repository", "repo")),
		"scope":      plan.Scope,
		"message":    content,
	}
	if value := assistantFeatureValueAfterAnyLabel(content, "encrypted value"); value != "" {
		args["encrypted_value"] = value
	}
	if value := assistantFeatureValueAfterAnyLabel(content, "value"); value != "" {
		args["value"] = value
	}
	return args
}

func assistantFeatureUIArea(lower string) string {
	for _, area := range []string{"assistant", "pipelines", "monitoring", "setup", "admin"} {
		if containsAny(lower, area) {
			return area
		}
	}
	return ""
}

package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"nopsai/config"
	"nopsai/pkg/models"
	aaamodel "nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
	runquery "nopsai/services/nopsai/internal/runs"
	"nopsai/services/nopsai/internal/systemlogs"
	"nopsai/services/nopsai/pkg/validation"
)

type hostedMCPTool struct {
	Name              string               `json:"name"`
	Description       string               `json:"description"`
	InputSchema       map[string]any       `json:"inputSchema"`
	Action            string               `json:"-"`
	Resource          aaamodel.ResourceRef `json:"-"`
	AuthenticatedOnly bool                 `json:"-"`
}

func allHostedMCPTools() []hostedMCPTool {
	tools := []hostedMCPTool{
		authenticatedToolDef("nopsai.call_api", "Call an allowed NopsAI REST API route as the current authenticated subject. Mutating calls require confirm:true and route/resource AAA checks still apply.", objectSchema(map[string]any{"method": stringSchema(), "path": stringSchema(), "query": objectSchema(map[string]any{}), "body": objectSchema(map[string]any{}), "headers": objectSchema(map[string]any{}), "confirm": booleanSchema(), "include_sensitive_response": booleanSchema()})),
		authenticatedToolDef("nopsai.get_platform_version", "Read the public platform version, compatibility ranges, protocols, capabilities, and release manifest digest.", objectSchema(map[string]any{})),
		toolDef("nopsai.search_docs", "Search Nopsai knowledge and docs.", "knowledge_context.read", "knowledge_context", "*", objectSchema(map[string]any{"query": stringSchema(), "limit": numberSchema()})),
		toolDef("nopsai.read_doc", "Read a Nopsai knowledge document by id.", "knowledge_context.read", "knowledge_context", "*", objectSchema(map[string]any{"id": stringSchema()})),
		toolDef("nopsai.list_knowledge_contexts", "List managed Nopsai knowledge context documents with optional filters.", "knowledge_context.read", "knowledge_context", "*", objectSchema(map[string]any{"kind": stringSchema(), "team": stringSchema(), "team_path": stringSchema(), "query": stringSchema(), "used_by_pipeline": stringSchema(), "limit": numberSchema()})),
		toolDef("nopsai.get_knowledge_context", "Read a managed knowledge context document by id, kind/team/name, or kind plus ref.", "knowledge_context.read", "knowledge_context", "*", objectSchema(map[string]any{"id": stringSchema(), "kind": stringSchema(), "team": stringSchema(), "team_path": stringSchema(), "name": stringSchema(), "ref": stringSchema()})),
		toolDef("nopsai.list_knowledge_connections", "List reusable team-owned Knowledge Context external page connections.", "knowledge_connection.read", "knowledge_connection", "*", objectSchema(map[string]any{"team": stringSchema(), "team_path": stringSchema(), "provider": stringSchema(), "query": stringSchema(), "limit": numberSchema()})),
		toolDef("nopsai.list_pipelines", "List pipelines visible to the current user.", "pipeline.list", "pipeline", "*", objectSchema(map[string]any{"limit": numberSchema()})),
		toolDef("nopsai.search_pipelines", "Search pipeline metadata and readable YAML definitions.", "pipeline.list", "pipeline", "*", objectSchema(map[string]any{"query": stringSchema(), "limit": numberSchema(), "include_snippets": booleanSchema()})),
		toolDef("nopsai.get_pipeline", "Read a pipeline YAML definition.", "pipeline.read", "pipeline", "*", objectSchema(map[string]any{"pipeline": stringSchema(), "path": stringSchema(), "name": stringSchema()})),
		toolDef("nopsai.get_pipeline_knowledge_context", "Resolve knowledge context references used by a stored or supplied pipeline.", "pipeline.read", "pipeline", "*", objectSchema(map[string]any{"pipeline": stringSchema(), "path": stringSchema(), "name": stringSchema(), "yaml": stringSchema(), "include_content": booleanSchema()})),
		toolDef("nopsai.validate_pipeline", "Validate pipeline YAML without saving it.", "pipeline.read", "pipeline", "*", objectSchema(map[string]any{"yaml": stringSchema()})),
		toolDef("nopsai.propose_pipeline_create", "Validate pipeline YAML and return a GitOps-ready create file plan without applying changes.", "pipeline.create", "pipeline", "*", objectSchema(map[string]any{"pipeline": stringSchema(), "path": stringSchema(), "name": stringSchema(), "yaml": stringSchema(), "message": stringSchema()})),
		toolDef("nopsai.propose_pipeline_update", "Validate pipeline YAML and return a GitOps-ready update file plan without applying changes.", "pipeline.update", "pipeline", "*", objectSchema(map[string]any{"pipeline": stringSchema(), "path": stringSchema(), "name": stringSchema(), "yaml": stringSchema(), "message": stringSchema()})),
		toolDef("nopsai.list_pipeline_runs", "List recent pipeline runs visible to the current user.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{"limit": numberSchema()})),
		toolDef("nopsai.get_pipeline_run", "Read run status, scope, trigger subject, Git data, timings, and output summaries.", "pipeline_run.read", "pipeline_run", "*", objectSchema(map[string]any{"run_id": stringSchema()})),
		toolDef("nopsai.get_pipeline_run_output", "Read final output content, dashboard target metadata, timing, and generation/render audit counts.", "pipeline_run.read", "pipeline_run", "*", objectSchema(map[string]any{"run_id": stringSchema(), "output_id": stringSchema(), "name": stringSchema()})),
		toolDef("nopsai.get_pipeline_run_logs", "Read recent pipeline run logs.", "pipeline_run.read_logs", "pipeline_run", "*", objectSchema(map[string]any{"run_id": stringSchema(), "limit": numberSchema()})),
		toolDef("nopsai.analyze_pipeline_run_failure", "Analyze a failed pipeline run from status and log excerpts.", "pipeline_run.read_logs", "pipeline_run", "*", objectSchema(map[string]any{"run_id": stringSchema()})),
		toolDef("nopsai.list_triggers", "List repository triggers.", "trigger.read", "trigger", "*", objectSchema(map[string]any{"limit": numberSchema()})),
		toolDef("nopsai.get_trigger", "Read a trigger definition.", "trigger.read", "trigger", "*", objectSchema(map[string]any{"repository": stringSchema()})),
		toolDef("nopsai.propose_trigger_change", "Draft a trigger change without applying it.", "trigger.update", "trigger", "*", objectSchema(map[string]any{"repository": stringSchema(), "change": stringSchema()})),
		toolDef("nopsai.list_schedules", "List pipeline schedules.", "pipeline_schedule.list", "pipeline_schedule", "*", objectSchema(map[string]any{"limit": numberSchema()})),
		toolDef("nopsai.get_schedule", "Read a schedule definition.", "pipeline_schedule.read", "pipeline_schedule", "*", objectSchema(map[string]any{"schedule_id": stringSchema()})),
		toolDef("nopsai.propose_schedule_change", "Draft a schedule change without applying it.", "pipeline_schedule.update", "pipeline_schedule", "*", objectSchema(map[string]any{"schedule_id": stringSchema(), "change": stringSchema()})),
		toolDef("nopsai.list_dashboards", "List team-owned dashboards visible to the current user.", "dashboard.list", "dashboard", "*", objectSchema(map[string]any{"team": stringSchema(), "team_path": stringSchema(), "query": stringSchema(), "q": stringSchema(), "limit": numberSchema()})),
		toolDef("nopsai.get_dashboard", "Read a dashboard with sections, current publications, source bindings, and provenance.", "dashboard.read", "dashboard", "*", objectSchema(map[string]any{"dashboard_id": stringSchema(), "id": stringSchema(), "ref": stringSchema(), "include_history": booleanSchema()})),
		toolDef("nopsai.list_dashboard_refreshes", "List dashboard refreshes with source, pipeline, and output progress.", "dashboard.read", "dashboard", "*", objectSchema(map[string]any{"dashboard_id": stringSchema(), "id": stringSchema(), "ref": stringSchema(), "limit": numberSchema()})),
		toolDef("nopsai.list_dashboard_refresh_schedules", "List scheduled dashboard refresh definitions.", "dashboard.read", "dashboard", "*", objectSchema(map[string]any{"dashboard_id": stringSchema(), "id": stringSchema(), "ref": stringSchema()})),
		toolDef("nopsai.refresh_dashboard", "Start a confirmed dashboard, section, or source refresh.", "dashboard.refresh", "dashboard", "*", objectSchema(map[string]any{"dashboard_id": stringSchema(), "id": stringSchema(), "ref": stringSchema(), "scope_type": stringSchema(), "section_key": stringSchema(), "source_id": stringSchema(), "mode": stringSchema(), "run_scope": stringSchema(), "timeout": stringSchema(), "max_concurrency": numberSchema(), "idempotency_key": stringSchema(), "variables": objectSchema(map[string]any{}), "confirm": booleanSchema()})),
		toolDef("nopsai.run_dashboard_refresh_schedule", "Run a scheduled dashboard refresh immediately. Requires confirm:true.", "dashboard.refresh", "dashboard", "*", objectSchema(map[string]any{"dashboard_id": stringSchema(), "id": stringSchema(), "ref": stringSchema(), "schedule_id": stringSchema(), "name": stringSchema(), "confirm": booleanSchema()})),
		toolDef("nopsai.list_scopes", "List scopes referenced by pipelines, schedules, secrets, and variables.", "scope.read", "scope", "*", objectSchema(map[string]any{"limit": numberSchema()})),
		toolDef("nopsai.get_scope", "Read high-level scope usage.", "scope.read", "scope", "*", objectSchema(map[string]any{"scope": stringSchema()})),
		toolDef("nopsai.explain_scope_permissions", "Explain what a scope is used for.", "scope.read", "scope", "*", objectSchema(map[string]any{"scope": stringSchema()})),
		toolDef("nopsai.list_lab_items", "List lab-oriented recent runs and experiments.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{"limit": numberSchema()})),
		toolDef("nopsai.get_lab_item", "Read a lab item by run id.", "pipeline_run.read", "pipeline_run", "*", objectSchema(map[string]any{"run_id": stringSchema()})),
		toolDef("nopsai.explain_lab_result", "Explain a lab result using run status and logs.", "pipeline_run.read_logs", "pipeline_run", "*", objectSchema(map[string]any{"run_id": stringSchema()})),
		toolDef("nopsai.get_statistics", "Read platform statistics.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{})),
		toolDef("nopsai.get_cost_summary", "Read cost and usage summary.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{})),
		toolDef("nopsai.suggest_cost_improvements", "Suggest cost improvements from current usage data.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{})),
		toolDef("nopsai.suggest_design_improvements", "Suggest pipeline design improvements from current inventory.", "pipeline.list", "pipeline", "*", objectSchema(map[string]any{})),
		toolDef("nopsai.get_llm_profiles", "List existing LLM profiles the assistant can use.", "system.read", "system", "llm-profiles", objectSchema(map[string]any{})),
		toolDef("nopsai.get_mcp_profiles", "List external MCP profiles configured in Nopsai.", "system.read", "system", "mcp", objectSchema(map[string]any{})),
		toolDef("nopsai.get_feature_capabilities", "List NopsAI feature coverage, MCP surfaces, REST/GitOps backing routes, and current-user AAA availability.", "system.read", "system", "mcp", objectSchema(map[string]any{"area": stringSchema(), "query": stringSchema(), "include_api_routes": booleanSchema()})),
		toolDef("nopsai.get_system_status", "Read basic system setup and dispatcher status.", "system.read", "system", "config", objectSchema(map[string]any{})),
		toolDef("nopsai.get_dispatcher_status", "Read dispatcher and runner health, queued jobs, and runner capacity.", "system.read", "dispatcher", "status", objectSchema(map[string]any{})),
	}
	tools = append(tools, hostedMCPDedicatedTools()...)
	return append(tools, hostedMCPFinalTools()...)
}

func toolDef(name, description, action, resourceType, resourceID string, schema map[string]any) hostedMCPTool {
	return hostedMCPTool{
		Name:        name,
		Description: description,
		InputSchema: schema,
		Action:      action,
		Resource: aaamodel.ResourceRef{
			Type: resourceType,
			ID:   resourceID,
		},
	}
}

func authenticatedToolDef(name, description string, schema map[string]any) hostedMCPTool {
	return hostedMCPTool{
		Name:              name,
		Description:       description,
		InputSchema:       schema,
		AuthenticatedOnly: true,
	}
}

func objectSchema(properties map[string]any) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": true,
	}
}

func stringSchema() map[string]any {
	return map[string]any{"type": "string"}
}

func numberSchema() map[string]any {
	return map[string]any{"type": "number"}
}

func booleanSchema() map[string]any {
	return map[string]any{"type": "boolean"}
}

func (a *App) hostedMCPToolsForSubject(ctx context.Context, subject aaamodel.Subject) []hostedMCPTool {
	all := allHostedMCPTools()
	tools := make([]hostedMCPTool, 0, len(all))
	for _, tool := range all {
		if !a.assistantToolEnabledByConfig(tool) {
			continue
		}
		if tool.AuthenticatedOnly {
			tools = append(tools, tool)
			continue
		}
		if hostedMCPSystemLogTool(tool.Name) && a.hostedMCPAnySystemLogAllowed(ctx, subject) {
			tools = append(tools, tool)
			continue
		}
		if a.hostedMCPAllowed(ctx, subject, hostedMCPToolPermission(tool)) {
			tools = append(tools, tool)
		}
	}
	return tools
}

func hostedMCPSystemLogTool(name string) bool {
	return name == "nopsai.list_system_log_sources" || name == "nopsai.tail_system_logs"
}

func (a *App) hostedMCPAnySystemLogAllowed(ctx context.Context, subject aaamodel.Subject) bool {
	for _, source := range systemlogs.DefaultRegistry().Sources() {
		if a.hostedMCPAllowed(ctx, subject, hostedMCPReadPermission("system_log.read", "system_log", source.ID)) {
			return true
		}
	}
	return false
}

func (a *App) assistantToolEnabledByConfig(tool hostedMCPTool) bool {
	cfg := a.assistantConfig()
	if assistantToolRequiresActionExecution(tool) && !config.AssistantFeatureFlagEnabled(cfg.Features.ActionExecution) {
		return false
	}
	switch assistantFeatureForTool(tool.Name) {
	case "docs":
		return config.AssistantFeatureFlagEnabled(cfg.DocsEnabled) && config.AssistantFeatureFlagEnabled(cfg.Features.Docs)
	case "pipeline_debugging":
		return config.AssistantFeatureFlagEnabled(cfg.Features.PipelineDebugging)
	case "config_generation":
		return config.AssistantFeatureFlagEnabled(cfg.Features.ConfigGeneration)
	case "statistics_insights":
		return config.AssistantFeatureFlagEnabled(cfg.Features.StatisticsInsights)
	case "maintenance_recommendations":
		return config.AssistantFeatureFlagEnabled(cfg.Features.MaintenanceRecommendations)
	case "cost_recommendations":
		return config.AssistantFeatureFlagEnabled(cfg.Features.CostRecommendations)
	default:
		return true
	}
}

func assistantToolRequiresActionExecution(tool hostedMCPTool) bool {
	name := strings.TrimSpace(tool.Name)
	if name == "nopsai.call_api" {
		return true
	}
	if strings.HasPrefix(name, "nopsai.propose_") ||
		strings.HasPrefix(name, "nopsai.plan_") ||
		strings.HasPrefix(name, "nopsai.preview_") {
		return false
	}
	if properties, _ := tool.InputSchema["properties"].(map[string]any); properties != nil {
		if _, ok := properties["confirm"]; ok {
			return true
		}
	}
	action := strings.TrimSpace(tool.Action)
	return strings.HasSuffix(action, ".execute") ||
		strings.HasSuffix(action, ".update") ||
		strings.HasSuffix(action, ".delete") ||
		strings.HasSuffix(action, ".write_value") ||
		strings.HasSuffix(action, ".create") ||
		strings.HasSuffix(action, ".approve") ||
		strings.HasSuffix(action, ".cancel") ||
		strings.HasSuffix(action, ".rerun") ||
		strings.Contains(name, ".sync_") ||
		strings.Contains(name, ".write_") ||
		strings.Contains(name, ".rotate_") ||
		strings.Contains(name, ".activate_") ||
		strings.Contains(name, ".disable_") ||
		strings.Contains(name, ".enable_") ||
		strings.Contains(name, ".bootstrap_") ||
		strings.Contains(name, ".invoke_")
}

func assistantFeatureForTool(name string) string {
	name = strings.TrimSpace(name)
	switch {
	case name == "nopsai.search_docs" ||
		name == "nopsai.read_doc" ||
		strings.Contains(name, "knowledge_connection") ||
		strings.Contains(name, "knowledge_context"):
		return "docs"
	case strings.Contains(name, "cost"):
		return "cost_recommendations"
	case name == "nopsai.get_statistics" ||
		strings.Contains(name, "monitoring_") ||
		strings.Contains(name, "analytics"):
		return "statistics_insights"
	case name == "nopsai.suggest_design_improvements" ||
		strings.Contains(name, "dispatcher") ||
		strings.Contains(name, "runner") ||
		strings.Contains(name, "cleanup") ||
		strings.Contains(name, "backup") ||
		strings.Contains(name, "maintenance"):
		return "maintenance_recommendations"
	case name == "nopsai.validate_pipeline" ||
		strings.HasPrefix(name, "nopsai.propose_pipeline_") ||
		strings.HasPrefix(name, "nopsai.propose_schedule_") ||
		strings.HasPrefix(name, "nopsai.propose_trigger_") ||
		strings.HasPrefix(name, "nopsai.propose_reusable_step_") ||
		strings.HasPrefix(name, "nopsai.propose_external_trigger_") ||
		strings.HasPrefix(name, "nopsai.propose_git_webhook_source_") ||
		strings.HasPrefix(name, "nopsai.propose_secret_") ||
		strings.HasPrefix(name, "nopsai.propose_variable_") ||
		strings.HasPrefix(name, "nopsai.propose_notification_") ||
		strings.HasPrefix(name, "nopsai.propose_credential_"):
		return "config_generation"
	case strings.Contains(name, "pipeline_run") ||
		strings.Contains(name, "pipeline_runs") ||
		strings.Contains(name, "run_approval") ||
		strings.Contains(name, "lab_") ||
		name == "nopsai.list_pipelines" ||
		name == "nopsai.search_pipelines" ||
		name == "nopsai.get_pipeline":
		return "pipeline_debugging"
	default:
		return ""
	}
}

func (a *App) hostedMCPToolByName(ctx context.Context, subject aaamodel.Subject, name string) (hostedMCPTool, bool) {
	name = strings.TrimSpace(name)
	for _, tool := range a.hostedMCPToolsForSubject(ctx, subject) {
		if tool.Name == name {
			return tool, true
		}
	}
	return hostedMCPTool{}, false
}

func (a *App) callHostedMCPTool(ctx context.Context, subject aaamodel.Subject, userID string, tool hostedMCPTool, args map[string]any, conversationID *uuid.UUID) (map[string]any, error) {
	if err := a.authorizeHostedMCPToolCall(ctx, subject, tool, args); err != nil {
		a.recordHostedMCPAudit(ctx, hostedMCPAuditRecord{UserID: userID, ConversationID: conversationID, ToolName: tool.Name, Input: hostedMCPAuditInput(tool.Name, args), Status: "denied"})
		return nil, err
	}
	result, err := a.executeHostedMCPTool(ctx, subject, tool.Name, args)
	status := "success"
	if err != nil {
		status = "error"
	}
	a.recordHostedMCPAudit(ctx, hostedMCPAuditRecord{
		UserID:         userID,
		ConversationID: conversationID,
		ToolName:       tool.Name,
		Input:          hostedMCPAuditInput(tool.Name, args),
		Output:         hostedMCPAuditOutput(tool.Name, result),
		ResourceScope:  tool.Resource.Type + ":" + tool.Resource.ID,
		Status:         status,
	})
	return result, err
}

func (a *App) authorizeHostedMCPToolCall(ctx context.Context, subject aaamodel.Subject, tool hostedMCPTool, args map[string]any) error {
	if tool.Name == "nopsai.call_api" {
		return a.authorizeHostedMCPAPICall(ctx, subject, args)
	}
	if handled, err := a.authorizeHostedMCPFinalToolCall(ctx, subject, tool, args); handled {
		return err
	}
	if handled, err := a.authorizeHostedMCPDedicatedToolCall(ctx, subject, tool, args); handled {
		return err
	}
	if tool.AuthenticatedOnly {
		return nil
	}
	permission := hostedMCPToolPermission(tool)
	switch tool.Name {
	case "nopsai.get_pipeline":
		permission.Resource.ID = pipelineArgID(args)
	case "nopsai.get_pipeline_knowledge_context":
		if strings.TrimSpace(stringArg(args, "yaml")) == "" {
			permission.Resource.ID = pipelineArgID(args)
		}
	case "nopsai.propose_pipeline_create", "nopsai.propose_pipeline_update":
		permission.Resource.ID = pipelineWritePlanResourceID(args)
	case "nopsai.get_knowledge_context":
		permission.Resource.ID = a.knowledgeContextArgID(ctx, args)
	case "nopsai.get_pipeline_run", "nopsai.get_pipeline_run_output", "nopsai.get_pipeline_run_logs", "nopsai.analyze_pipeline_run_failure", "nopsai.get_lab_item", "nopsai.explain_lab_result":
		permission.Resource.ID = stringArg(args, "run_id")
	case "nopsai.get_trigger", "nopsai.propose_trigger_change":
		permission.Resource.ID = stringArg(args, "repository")
	case "nopsai.get_schedule", "nopsai.propose_schedule_change":
		permission.Resource.ID = stringArg(args, "schedule_id")
	case "nopsai.get_dashboard", "nopsai.list_dashboard_refreshes", "nopsai.list_dashboard_refresh_schedules", "nopsai.refresh_dashboard", "nopsai.run_dashboard_refresh_schedule":
		permission.Resource.ID = firstNonEmptyString(stringArg(args, "dashboard_id"), stringArg(args, "id"), stringArg(args, "ref"))
	case "nopsai.get_scope", "nopsai.explain_scope_permissions":
		permission.Resource.ID = stringArg(args, "scope")
	}
	if strings.TrimSpace(permission.Resource.ID) == "" {
		permission.Resource.ID = tool.Resource.ID
	}
	if a.hostedMCPAllowed(ctx, subject, permission) {
		return nil
	}
	return fmt.Errorf("tool %s is not allowed for %s:%s", tool.Name, permission.Resource.Type, permission.Resource.ID)
}

func (a *App) executeHostedMCPTool(ctx context.Context, subject aaamodel.Subject, name string, args map[string]any) (map[string]any, error) {
	if result, handled, err := a.executeHostedMCPFinalTool(ctx, subject, name, args); handled {
		return result, err
	}
	if result, handled, err := a.executeHostedMCPDedicatedTool(ctx, subject, name, args); handled {
		return result, err
	}
	switch name {
	case "nopsai.call_api":
		return a.hostedMCPCallAPI(ctx, subject, args)
	case "nopsai.get_platform_version":
		return versionInfoMap(), nil
	case "nopsai.search_docs":
		return a.hostedMCPSearchDocs(ctx, args)
	case "nopsai.read_doc":
		return a.hostedMCPReadDoc(ctx, args)
	case "nopsai.list_knowledge_contexts":
		return a.hostedMCPListKnowledgeContexts(ctx, subject, args)
	case "nopsai.get_knowledge_context":
		return a.hostedMCPGetKnowledgeContext(ctx, subject, args)
	case "nopsai.list_knowledge_connections":
		return a.hostedMCPListKnowledgeConnections(ctx, subject, args)
	case "nopsai.list_pipelines":
		return a.hostedMCPListPipelines(ctx, args)
	case "nopsai.search_pipelines":
		return a.hostedMCPSearchPipelines(ctx, subject, args)
	case "nopsai.get_pipeline":
		return a.hostedMCPGetPipeline(ctx, args)
	case "nopsai.get_pipeline_knowledge_context":
		return a.hostedMCPGetPipelineKnowledgeContext(ctx, subject, args)
	case "nopsai.validate_pipeline":
		return hostedMCPValidatePipeline(args)
	case "nopsai.propose_pipeline_create":
		return hostedMCPProposePipelineWrite(args, "create")
	case "nopsai.propose_pipeline_update":
		return hostedMCPProposePipelineWrite(args, "update")
	case "nopsai.list_pipeline_runs", "nopsai.list_lab_items":
		return a.hostedMCPListPipelineRuns(ctx, args)
	case "nopsai.get_pipeline_run", "nopsai.get_lab_item":
		return a.hostedMCPGetPipelineRun(ctx, args)
	case "nopsai.get_pipeline_run_output":
		return a.hostedMCPGetPipelineRunOutput(ctx, args)
	case "nopsai.get_pipeline_run_logs":
		return a.hostedMCPGetPipelineRunLogs(ctx, args)
	case "nopsai.analyze_pipeline_run_failure", "nopsai.explain_lab_result":
		return a.hostedMCPAnalyzePipelineRunFailure(ctx, args)
	case "nopsai.list_triggers":
		return a.hostedMCPListTriggers(ctx, args)
	case "nopsai.get_trigger":
		return a.hostedMCPGetTrigger(ctx, args)
	case "nopsai.propose_trigger_change":
		return hostedMCPProposal("trigger", args), nil
	case "nopsai.list_schedules":
		return a.hostedMCPListSchedules(ctx, args)
	case "nopsai.get_schedule":
		return a.hostedMCPGetSchedule(ctx, args)
	case "nopsai.propose_schedule_change":
		return hostedMCPProposal("schedule", args), nil
	case "nopsai.list_dashboards":
		return a.hostedMCPListDashboards(ctx, subject, args)
	case "nopsai.get_dashboard":
		return a.hostedMCPGetDashboard(ctx, args)
	case "nopsai.list_dashboard_refreshes":
		return a.hostedMCPListDashboardRefreshes(ctx, args)
	case "nopsai.list_dashboard_refresh_schedules":
		return a.hostedMCPListDashboardRefreshSchedules(ctx, args)
	case "nopsai.refresh_dashboard":
		return a.hostedMCPRefreshDashboard(ctx, subject, args)
	case "nopsai.run_dashboard_refresh_schedule":
		return a.hostedMCPRunDashboardRefreshSchedule(ctx, args)
	case "nopsai.list_scopes":
		return a.hostedMCPListScopes(ctx, args)
	case "nopsai.get_scope", "nopsai.explain_scope_permissions":
		return a.hostedMCPGetScope(ctx, args)
	case "nopsai.get_statistics":
		return a.hostedMCPStatistics(ctx)
	case "nopsai.get_cost_summary":
		return a.hostedMCPCostSummary(ctx)
	case "nopsai.suggest_cost_improvements":
		return a.hostedMCPSuggestCostImprovements(ctx)
	case "nopsai.suggest_design_improvements":
		return a.hostedMCPSuggestDesignImprovements(ctx)
	case "nopsai.get_llm_profiles":
		return a.hostedMCPGetLLMProfiles(ctx)
	case "nopsai.get_mcp_profiles":
		return a.hostedMCPGetMCPProfiles(ctx)
	case "nopsai.get_feature_capabilities":
		return a.hostedMCPGetFeatureCapabilities(ctx, subject, args)
	case "nopsai.get_system_status":
		return a.hostedMCPGetSystemStatus(ctx)
	case "nopsai.get_dispatcher_status":
		return a.hostedMCPGetDispatcherStatus(ctx)
	default:
		return nil, fmt.Errorf("unknown hosted MCP tool %q", name)
	}
}

func (a *App) hostedMCPListPipelines(ctx context.Context, args map[string]any) (map[string]any, error) {
	rows, err := a.db.Query(ctx, `
		SELECT path, name, version, source, visibility, updated_at
		FROM pipelines
		ORDER BY path ASC, name ASC
		LIMIT $1
	`, limitArg(args, 50, 200))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var path, name, version, source, visibility string
		var updatedAt time.Time
		if err := rows.Scan(&path, &name, &version, &source, &visibility, &updatedAt); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":         aaamodel.BuildPipelineID(path, name),
			"path":       path,
			"name":       name,
			"version":    version,
			"source":     source,
			"visibility": visibility,
			"updated_at": updatedAt,
		})
	}
	return map[string]any{"pipelines": items}, rows.Err()
}

func (a *App) hostedMCPSearchPipelines(ctx context.Context, subject aaamodel.Subject, args map[string]any) (map[string]any, error) {
	query := strings.TrimSpace(stringArg(args, "query"))
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	patterns := hostedMCPSearchPatterns(query)
	rows, err := a.db.Query(ctx, `
		SELECT path, name, version, source, visibility, updated_at, definition
		FROM pipelines
		WHERE LOWER(path || ' ' || name || ' ' || version || ' ' || source || ' ' || visibility || ' ' || definition) LIKE LOWER('%' || $1 || '%')
		   OR ($3::text[] <> '{}'::text[] AND LOWER(path || ' ' || name || ' ' || version || ' ' || source || ' ' || visibility || ' ' || definition) LIKE ALL($3::text[]))
		ORDER BY path ASC, name ASC
		LIMIT $2
	`, query, limitArg(args, 50, 200), patterns)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	includeSnippets := boolArg(args, "include_snippets", true)
	items := []map[string]any{}
	for rows.Next() {
		var path, name, version, source, visibility, definition string
		var updatedAt time.Time
		if err := rows.Scan(&path, &name, &version, &source, &visibility, &updatedAt, &definition); err != nil {
			return nil, err
		}
		pipelineID := aaamodel.BuildPipelineID(path, name)
		matchFields := hostedMCPPipelineMatchFields(query, path, name, version, source, visibility, definition)
		readAllowed := a.hostedMCPAllowed(ctx, subject, hostedMCPReadPermission("pipeline.read", "pipeline", pipelineID))
		if onlyPipelineDefinitionMatched(matchFields) && !readAllowed {
			continue
		}
		item := map[string]any{
			"id":           pipelineID,
			"path":         path,
			"name":         name,
			"version":      version,
			"source":       source,
			"visibility":   visibility,
			"updated_at":   updatedAt,
			"match_fields": matchFields,
			"read_allowed": readAllowed,
		}
		if includeSnippets && readAllowed {
			if snippet := hostedMCPSnippet(definition, query, 180); snippet != "" {
				item["snippet"] = snippet
			}
		}
		items = append(items, item)
	}
	return map[string]any{"query": query, "pipelines": items}, rows.Err()
}

func (a *App) hostedMCPGetPipeline(ctx context.Context, args map[string]any) (map[string]any, error) {
	pathPart, namePart := splitPipelineArg(args)
	var version, definition, source, visibility string
	var updatedAt time.Time
	err := a.db.QueryRow(ctx, `
		SELECT version, definition, source, visibility, updated_at
		FROM pipelines
		WHERE path = $1 AND name = $2
	`, pathPart, namePart).Scan(&version, &definition, &source, &visibility, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			pipelineID := aaamodel.BuildPipelineID(pathPart, namePart)
			if pipelineID == "" {
				pipelineID = "requested pipeline"
			}
			return nil, fmt.Errorf("pipeline %q was not found", pipelineID)
		}
		return nil, err
	}
	return map[string]any{
		"id":         aaamodel.BuildPipelineID(pathPart, namePart),
		"path":       pathPart,
		"name":       namePart,
		"version":    version,
		"definition": definition,
		"source":     source,
		"visibility": visibility,
		"updated_at": updatedAt,
	}, nil
}

func hostedMCPValidatePipeline(args map[string]any) (map[string]any, error) {
	raw := stringArg(args, "yaml")
	if raw == "" {
		return nil, fmt.Errorf("yaml is required")
	}
	return hostedMCPValidatePipelineYAML(raw), nil
}

func hostedMCPValidatePipelineYAML(raw string) map[string]any {
	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(raw), &pipeline); err != nil {
		return map[string]any{"valid": false, "error": err.Error()}
	}
	if err := validation.ValidatePipeline(&pipeline); err != nil {
		return map[string]any{"valid": false, "error": err.Error()}
	}
	return map[string]any{"valid": true, "name": pipeline.Name, "version": pipeline.Version}
}

func hostedMCPParseValidPipelineYAML(raw string) (models.Pipeline, map[string]any, bool) {
	var pipeline models.Pipeline
	validationResult := hostedMCPValidatePipelineYAML(raw)
	if !boolValue(validationResult["valid"]) {
		return pipeline, validationResult, false
	}
	if err := yaml.Unmarshal([]byte(raw), &pipeline); err != nil {
		return pipeline, map[string]any{"valid": false, "error": err.Error()}, false
	}
	pipeline.Version = normalizePipelineVersion(pipeline.Version)
	return pipeline, validationResult, true
}

func hostedMCPProposePipelineWrite(args map[string]any, mode string) (map[string]any, error) {
	raw := stringArg(args, "yaml")
	if raw == "" {
		return nil, fmt.Errorf("yaml is required")
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "create" && mode != "update" {
		return nil, fmt.Errorf("unsupported pipeline write mode %q", mode)
	}
	pathPart, expectedName := splitPipelineArg(args)
	if expectedName != "" {
		raw = hostedMCPPipelineYAMLWithFallbackName(raw, expectedName)
	}
	raw = hostedMCPCompleteGeneratedDockerPipelineYAML(raw)
	pipeline, validationResult, ok := hostedMCPParseValidPipelineYAML(raw)
	if !ok {
		return map[string]any{
			"proposal_type": "pipeline_" + mode,
			"applies":       false,
			"valid":         false,
			"validation":    validationResult,
			"note":          "No write plan was produced because the pipeline YAML did not pass validation.",
		}, nil
	}
	if expectedName != "" && expectedName != pipeline.Name {
		errText := fmt.Sprintf("target pipeline name %q does not match YAML name %q", expectedName, pipeline.Name)
		return map[string]any{
			"proposal_type": "pipeline_" + mode,
			"applies":       false,
			"valid":         false,
			"validation":    map[string]any{"valid": false, "error": errText},
			"note":          "No write plan was produced because the target and YAML disagree.",
		}, nil
	}
	pipelineID := aaamodel.BuildPipelineID(pathPart, pipeline.Name)
	action := "pipeline.create"
	verb := "Create"
	if mode == "update" {
		action = "pipeline.update"
		verb = "Update"
	}
	message := stringArg(args, "message")
	if message == "" {
		message = verb + " Nopsai pipeline " + pipelineID
	}
	relativePath := "pipelines/" + configsync.BuildPipelineFilePath(pathPart, pipeline.Name, ".yaml")
	return map[string]any{
		"proposal_type": "pipeline_" + mode,
		"applies":       false,
		"valid":         true,
		"action":        action,
		"pipeline_id":   pipelineID,
		"path":          pathPart,
		"name":          pipeline.Name,
		"version":       pipeline.Version,
		"yaml":          raw,
		"validation":    validationResult,
		"gitops": map[string]any{
			"message": message,
			"files": []map[string]any{{
				"path":    relativePath,
				"content": raw,
				"delete":  false,
			}},
			"review_note": "Commit this file through the configured config repository review branch, then sync GitOps before relying on the change.",
		},
		"api": map[string]any{
			"method": "PUT",
			"path":   "/v1/pipelines/" + pipelineID,
			"note":   "The direct API can save the pipeline when authorized, but the MCP tool returns a proposal only.",
		},
	}, nil
}

func (a *App) hostedMCPListPipelineRuns(ctx context.Context, args map[string]any) (map[string]any, error) {
	items, err := runquery.List(ctx, a.db, runquery.ListFilter{Limit: limitArg(args, 30, 100)})
	if err != nil {
		return nil, err
	}
	runs := []map[string]any{}
	for _, item := range items {
		run := map[string]any{
			"run_id":         item.RunID,
			"pipeline_id":    aaamodel.BuildPipelineID(item.PipelinePath, item.PipelineName),
			"pipeline_path":  item.PipelinePath,
			"pipeline_name":  item.PipelineName,
			"status":         item.Status,
			"scope":          item.Scope,
			"created_at":     hostedMCPTime(item.CreatedAt),
			"started_at":     hostedMCPTime(item.StartedAt),
			"finished_at":    hostedMCPTime(item.FinishedAt),
			"failure_reason": item.FailureReason,
		}
		if item.FinalOutputStatus != nil {
			run["final_output_status"] = item.FinalOutputStatus
		}
		runs = append(runs, run)
	}
	return map[string]any{"runs": runs}, nil
}

func (a *App) hostedMCPGetPipelineRun(ctx context.Context, args map[string]any) (map[string]any, error) {
	runID := stringArg(args, "run_id")
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	var path, name, version, status, source, scope, failureReason, triggerSource, triggerEventID string
	var gitRepoOwner, gitRepoName, gitCloneURL, gitSSHURL, gitRef, gitTargetRef string
	var gitCommitSHA, gitCommitURL, gitCommitMessage, gitCommitAuthorName, gitCommitAuthorEmail, gitCommitAuthorUsername string
	var gitPusherName, gitPusherEmail string
	var requestedByType, requestedByID, effectiveSubjectType, effectiveSubjectID, runtimeVariableOverridesRaw string
	var definition sql.NullString
	var teamID int
	var gitCheckRunID int64
	var createdAt time.Time
	var startedAt, finishedAt, timeoutAt sql.NullTime
	err := a.db.QueryRow(ctx, `
		SELECT COALESCE(pipeline_path, ''), COALESCE(pipeline_name, ''), pipeline_version, status, COALESCE(pipeline_source, ''),
		       COALESCE(scope, ''), COALESCE(failure_reason, ''), COALESCE(trigger_source, ''), pipeline_definition,
		       COALESCE(trigger_event_id, ''), COALESCE(git_repo_owner, ''), COALESCE(git_repo_name, ''),
		       COALESCE(git_clone_url, ''), COALESCE(git_ssh_url, ''), COALESCE(git_ref, ''),
		       COALESCE(git_target_ref, ''), COALESCE(git_commit_sha, ''), COALESCE(git_commit_url, ''),
		       COALESCE(git_commit_message, ''), COALESCE(git_commit_author_name, ''),
		       COALESCE(git_commit_author_email, ''), COALESCE(git_commit_author_username, ''),
		       COALESCE(git_pusher_name, ''), COALESCE(git_pusher_email, ''),
		       COALESCE(git_check_run_id, 0)::bigint, COALESCE(team_id, 0)::int,
		       COALESCE(requested_by_type, ''), COALESCE(requested_by_id, ''),
		       COALESCE(effective_subject_type, ''), COALESCE(effective_subject_id, ''),
		       COALESCE(runtime_variable_overrides::text, '{}'),
		       created_at, started_at, finished_at, timeout_at
		FROM pipeline_runs
		WHERE run_id::text = $1
	`, runID).Scan(
		&path, &name, &version, &status, &source, &scope, &failureReason, &triggerSource, &definition,
		&triggerEventID, &gitRepoOwner, &gitRepoName, &gitCloneURL, &gitSSHURL, &gitRef, &gitTargetRef,
		&gitCommitSHA, &gitCommitURL, &gitCommitMessage, &gitCommitAuthorName, &gitCommitAuthorEmail,
		&gitCommitAuthorUsername, &gitPusherName, &gitPusherEmail, &gitCheckRunID, &teamID,
		&requestedByType, &requestedByID, &effectiveSubjectType, &effectiveSubjectID, &runtimeVariableOverridesRaw,
		&createdAt, &startedAt, &finishedAt, &timeoutAt,
	)
	if err != nil {
		return nil, err
	}
	outputs, err := a.hostedMCPPipelineRunOutputSummaries(ctx, runID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"run_id":                 runID,
		"pipeline_id":            aaamodel.BuildPipelineID(path, name),
		"pipeline_path":          path,
		"pipeline_name":          name,
		"pipeline_version":       version,
		"pipeline_definition":    definition.String,
		"status":                 status,
		"source":                 source,
		"scope":                  scope,
		"trigger_source":         triggerSource,
		"trigger_event_id":       triggerEventID,
		"requested_by_type":      requestedByType,
		"requested_by_id":        requestedByID,
		"effective_subject_type": effectiveSubjectType,
		"effective_subject_id":   effectiveSubjectID,
		"team_id":                teamID,
		"failure_reason":         failureReason,
		"git": map[string]any{
			"repo_owner":             gitRepoOwner,
			"repo_name":              gitRepoName,
			"clone_url":              gitCloneURL,
			"ssh_url":                gitSSHURL,
			"ref":                    gitRef,
			"target_ref":             gitTargetRef,
			"commit_sha":             gitCommitSHA,
			"commit_url":             gitCommitURL,
			"commit_message":         gitCommitMessage,
			"commit_author_name":     gitCommitAuthorName,
			"commit_author_email":    gitCommitAuthorEmail,
			"commit_author_username": gitCommitAuthorUsername,
			"pusher_name":            gitPusherName,
			"pusher_email":           gitPusherEmail,
			"check_run_id":           gitCheckRunID,
		},
		"runtime_variable_overrides": scanJSONMap(runtimeVariableOverridesRaw),
		"created_at":                 createdAt,
		"started_at":                 hostedMCPNullableTime(startedAt),
		"finished_at":                hostedMCPNullableTime(finishedAt),
		"timeout_at":                 hostedMCPNullableTime(timeoutAt),
		"final_outputs":              outputs,
	}, nil
}

func (a *App) hostedMCPPipelineRunOutputSummaries(ctx context.Context, runID string) ([]map[string]any, error) {
	rows, err := a.db.Query(ctx, `
			SELECT id::text, name, type, status, error, llm_profile,
			       generation_attempts, contract_violations, render_attempts, render_failures, created_at, generation_started_at, updated_at,
			       COALESCE(dashboard_target::text, '{}')
			FROM pipeline_run_outputs
			WHERE run_id::text = $1
			ORDER BY item_index ASC, created_at ASC
		`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	outputs := []map[string]any{}
	for rows.Next() {
		var id, name, outputType, status, errorText, profile string
		var dashboardTargetRaw string
		var generationAttempts, contractViolations, renderAttempts, renderFailures int
		var createdAt, updatedAt time.Time
		var generationStartedAt sql.NullTime
		if err := rows.Scan(
			&id,
			&name,
			&outputType,
			&status,
			&errorText,
			&profile,
			&generationAttempts,
			&contractViolations,
			&renderAttempts,
			&renderFailures,
			&createdAt,
			&generationStartedAt,
			&updatedAt,
			&dashboardTargetRaw,
		); err != nil {
			return nil, err
		}
		startedAt := nullTimePtr(generationStartedAt)
		duration, durationSeconds := pipelineOutputGenerationDuration(startedAt, updatedAt)
		output := hostedMCPPipelineRunOutputMap(models.PipelineRunFinalOutput{
			ID:                  id,
			Name:                name,
			Type:                outputType,
			Status:              status,
			Error:               errorText,
			LLMProfile:          profile,
			DashboardTarget:     hostedMCPFinalOutputDashboardTargetPtr(dashboardTargetRaw),
			GenerationAttempts:  generationAttempts,
			ContractViolations:  contractViolations,
			RenderAttempts:      renderAttempts,
			RenderFailures:      renderFailures,
			CreatedAt:           createdAt,
			GenerationStartedAt: startedAt,
			UpdatedAt:           updatedAt,
			GenerationDuration:  duration,
			GenerationSeconds:   durationSeconds,
		})
		delete(output, "content")
		outputs = append(outputs, output)
	}
	return outputs, rows.Err()
}

func (a *App) hostedMCPGetPipelineRunOutput(ctx context.Context, args map[string]any) (map[string]any, error) {
	runID := stringArg(args, "run_id")
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	outputID := strings.TrimSpace(stringArg(args, "output_id"))
	outputName := strings.TrimSpace(stringArg(args, "name"))
	if outputID == "" && outputName == "" {
		return nil, fmt.Errorf("output_id or name is required")
	}

	query := `
			SELECT id::text, name, type, status, content, error, llm_profile,
			       generation_attempts, contract_violations, render_attempts, render_failures,
			       created_at, generation_started_at, updated_at, COALESCE(dashboard_target::text, '{}')
			FROM pipeline_run_outputs
			WHERE run_id::text = $1 AND `
	argsList := []any{runID}
	if outputID != "" {
		query += `id::text = $2`
		argsList = append(argsList, outputID)
	} else {
		query += `LOWER(name) = LOWER($2)`
		argsList = append(argsList, outputName)
	}
	query += ` ORDER BY item_index ASC LIMIT 1`

	var output models.PipelineRunFinalOutput
	var dashboardTargetRaw string
	var generationStartedAt sql.NullTime
	err := a.db.QueryRow(ctx, query, argsList...).Scan(
		&output.ID,
		&output.Name,
		&output.Type,
		&output.Status,
		&output.Content,
		&output.Error,
		&output.LLMProfile,
		&output.GenerationAttempts,
		&output.ContractViolations,
		&output.RenderAttempts,
		&output.RenderFailures,
		&output.CreatedAt,
		&generationStartedAt,
		&output.UpdatedAt,
		&dashboardTargetRaw,
	)
	if err != nil {
		return nil, err
	}
	output.GenerationStartedAt = nullTimePtr(generationStartedAt)
	output.DashboardTarget = hostedMCPFinalOutputDashboardTargetPtr(dashboardTargetRaw)
	output.GenerationDuration, output.GenerationSeconds = pipelineOutputGenerationDuration(output.GenerationStartedAt, output.UpdatedAt)
	return map[string]any{
		"run_id": runID,
		"output": hostedMCPPipelineRunOutputMap(output),
	}, nil
}

func hostedMCPPipelineRunOutputMap(output models.PipelineRunFinalOutput) map[string]any {
	out := map[string]any{
		"id":                          output.ID,
		"name":                        output.Name,
		"type":                        output.Type,
		"status":                      output.Status,
		"content":                     output.Content,
		"error":                       output.Error,
		"llm_profile":                 output.LLMProfile,
		"generation_attempts":         output.GenerationAttempts,
		"contract_violations":         output.ContractViolations,
		"render_attempts":             output.RenderAttempts,
		"render_failures":             output.RenderFailures,
		"created_at":                  output.CreatedAt,
		"generation_started_at":       output.GenerationStartedAt,
		"updated_at":                  output.UpdatedAt,
		"generation_duration":         output.GenerationDuration,
		"generation_duration_seconds": output.GenerationSeconds,
	}
	if output.DashboardTarget != nil {
		out["dashboard_target"] = output.DashboardTarget
	}
	return out
}

func hostedMCPFinalOutputDashboardTargetPtr(raw string) *models.DashboardOutputTarget {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return nil
	}
	var target models.DashboardOutputTarget
	if err := json.Unmarshal([]byte(raw), &target); err != nil {
		return nil
	}
	if strings.TrimSpace(target.Ref) == "" &&
		strings.TrimSpace(target.Section) == "" &&
		strings.TrimSpace(target.EntryKey) == "" &&
		strings.TrimSpace(target.Mode) == "" &&
		strings.TrimSpace(target.Preset) == "" &&
		strings.TrimSpace(target.TTL) == "" {
		return nil
	}
	return &target
}

func (a *App) hostedMCPGetPipelineRunLogs(ctx context.Context, args map[string]any) (map[string]any, error) {
	runID := stringArg(args, "run_id")
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	maxBytes := a.assistantConfig().MaxInputLogsBytes
	rows, err := a.db.Query(ctx, `
		SELECT id, timestamp, line
		FROM pipeline_run_logs
		WHERE run_id::text = $1
		ORDER BY id DESC
		LIMIT $2
	`, runID, limitArg(args, 80, 500))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs := []map[string]any{}
	usedBytes := 0
	truncated := false
	for rows.Next() {
		var id int64
		var timestamp time.Time
		var line string
		if err := rows.Scan(&id, &timestamp, &line); err != nil {
			return nil, err
		}
		lineBytes := len([]byte(line))
		if maxBytes > 0 && usedBytes+lineBytes > maxBytes {
			truncated = true
			break
		}
		usedBytes += lineBytes
		logs = append(logs, map[string]any{"id": id, "timestamp": timestamp, "line": line})
	}
	reverseMaps(logs)
	return map[string]any{
		"run_id":          runID,
		"logs":            logs,
		"bytes":           usedBytes,
		"max_bytes":       maxBytes,
		"bytes_truncated": truncated,
	}, rows.Err()
}

func (a *App) hostedMCPAnalyzePipelineRunFailure(ctx context.Context, args map[string]any) (map[string]any, error) {
	run, err := a.hostedMCPGetPipelineRun(ctx, args)
	if err != nil {
		return nil, err
	}
	logs, err := a.hostedMCPGetPipelineRunLogs(ctx, map[string]any{"run_id": stringArg(args, "run_id"), "limit": 120})
	if err != nil {
		return nil, err
	}
	failureReason, _ := run["failure_reason"].(string)
	status, _ := run["status"].(string)
	summary := "No terminal failure reason is recorded yet."
	if strings.TrimSpace(failureReason) != "" {
		summary = failureReason
	} else if strings.EqualFold(status, "failure") {
		summary = "Run is marked as failure. Review recent logs for the failing step or task."
	}
	return map[string]any{
		"run":             run,
		"log_excerpt":     logs["logs"],
		"root_cause_hint": summary,
		"suggested_next_steps": []string{
			"Check the first error in the log excerpt.",
			"Compare the failing run pipeline definition with the current pipeline.",
			"Validate any generated YAML before applying it through GitOps.",
		},
	}, nil
}

func (a *App) hostedMCPListTriggers(ctx context.Context, args map[string]any) (map[string]any, error) {
	rows, err := a.db.Query(ctx, `
		SELECT repository_name, source, visibility, provider, COALESCE(team_path, ''), management, COALESCE(webhook_source_id, ''), created_at
		FROM triggers
		ORDER BY repository_name ASC
		LIMIT $1
	`, limitArg(args, 50, 200))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var repository, source, visibility, provider, teamPath, management, webhookSourceID string
		var createdAt time.Time
		if err := rows.Scan(&repository, &source, &visibility, &provider, &teamPath, &management, &webhookSourceID, &createdAt); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"repository":        repository,
			"source":            source,
			"visibility":        visibility,
			"provider":          provider,
			"team_path":         teamPath,
			"management":        management,
			"webhook_source_id": webhookSourceID,
			"created_at":        createdAt,
		})
	}
	return map[string]any{"triggers": items}, rows.Err()
}

func (a *App) hostedMCPGetTrigger(ctx context.Context, args map[string]any) (map[string]any, error) {
	repository := stringArg(args, "repository")
	if repository == "" {
		return nil, fmt.Errorf("repository is required")
	}
	var definition, source, visibility, provider, teamPath, management, webhookSourceID string
	var createdAt time.Time
	err := a.db.QueryRow(ctx, `
		SELECT trigger_definition, source, visibility, provider, COALESCE(team_path, ''), management, COALESCE(webhook_source_id, ''), created_at
		FROM triggers
		WHERE repository_name = $1
	`, repository).Scan(&definition, &source, &visibility, &provider, &teamPath, &management, &webhookSourceID, &createdAt)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"repository":        repository,
		"definition":        definition,
		"source":            source,
		"visibility":        visibility,
		"provider":          provider,
		"team_path":         teamPath,
		"management":        management,
		"webhook_source_id": webhookSourceID,
		"created_at":        createdAt,
	}, nil
}

func (a *App) hostedMCPListSchedules(ctx context.Context, args map[string]any) (map[string]any, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id::text, path, name, pipeline_path, pipeline_name, schedule_kind, cron_expression,
		       run_at, timezone, enabled, scope, next_run_at, last_status
		FROM pipeline_schedules
		ORDER BY path ASC, name ASC
		LIMIT $1
	`, limitArg(args, 50, 200))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, path, name, pipelinePath, pipelineName, kind, cron, timezoneName, scope, lastStatus string
		var enabled bool
		var runAt, nextRunAt sql.NullTime
		if err := rows.Scan(&id, &path, &name, &pipelinePath, &pipelineName, &kind, &cron, &runAt, &timezoneName, &enabled, &scope, &nextRunAt, &lastStatus); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":              id,
			"path":            path,
			"name":            name,
			"pipeline_id":     aaamodel.BuildPipelineID(pipelinePath, pipelineName),
			"schedule_kind":   kind,
			"cron_expression": cron,
			"run_at":          hostedMCPNullableTime(runAt),
			"timezone":        timezoneName,
			"enabled":         enabled,
			"scope":           scope,
			"next_run_at":     hostedMCPNullableTime(nextRunAt),
			"last_status":     lastStatus,
		})
	}
	return map[string]any{"schedules": items}, rows.Err()
}

func (a *App) hostedMCPGetSchedule(ctx context.Context, args map[string]any) (map[string]any, error) {
	id := stringArg(args, "schedule_id")
	if id == "" {
		return nil, fmt.Errorf("schedule_id is required")
	}
	var variablesRaw []byte
	row := a.db.QueryRow(ctx, `
		SELECT id::text, path, name, description, pipeline_path, pipeline_name, pipeline_version,
		       schedule_kind, cron_expression, run_at, timezone, enabled, scope, run_team_path,
		       variables, next_run_at, last_run_at, COALESCE(last_run_id::text, ''), last_status
		FROM pipeline_schedules
		WHERE id::text = $1
	`, id)
	var path, name, description, pipelinePath, pipelineName, version, kind, cron, timezoneName, scope, runTeam, lastRunID, lastStatus string
	var enabled bool
	var runAt, nextRunAt, lastRunAt sql.NullTime
	if err := row.Scan(&id, &path, &name, &description, &pipelinePath, &pipelineName, &version, &kind, &cron, &runAt, &timezoneName, &enabled, &scope, &runTeam, &variablesRaw, &nextRunAt, &lastRunAt, &lastRunID, &lastStatus); err != nil {
		return nil, err
	}
	var variables map[string]any
	_ = json.Unmarshal(variablesRaw, &variables)
	return map[string]any{
		"id":               id,
		"path":             path,
		"name":             name,
		"description":      description,
		"pipeline_id":      aaamodel.BuildPipelineID(pipelinePath, pipelineName),
		"pipeline_version": version,
		"schedule_kind":    kind,
		"cron_expression":  cron,
		"run_at":           hostedMCPNullableTime(runAt),
		"timezone":         timezoneName,
		"enabled":          enabled,
		"scope":            scope,
		"run_team_path":    runTeam,
		"variables":        variables,
		"next_run_at":      hostedMCPNullableTime(nextRunAt),
		"last_run_at":      hostedMCPNullableTime(lastRunAt),
		"last_run_id":      lastRunID,
		"last_status":      lastStatus,
	}, nil
}

func (a *App) hostedMCPListScopes(ctx context.Context, args map[string]any) (map[string]any, error) {
	rows, err := a.db.Query(ctx, `
		SELECT scope FROM (
			SELECT scope FROM pipeline_runs WHERE COALESCE(scope, '') <> ''
			UNION
			SELECT scope FROM pipeline_schedules WHERE COALESCE(scope, '') <> ''
			UNION
			SELECT scope FROM secrets WHERE COALESCE(scope, '') <> ''
			UNION
			SELECT scope FROM variables WHERE COALESCE(scope, '') <> ''
		) scopes
		ORDER BY scope ASC
		LIMIT $1
	`, limitArg(args, 50, 200))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	scopes := []string{}
	for rows.Next() {
		var scope string
		if err := rows.Scan(&scope); err != nil {
			return nil, err
		}
		scopes = append(scopes, scope)
	}
	return map[string]any{"scopes": scopes}, rows.Err()
}

func (a *App) hostedMCPGetScope(ctx context.Context, args map[string]any) (map[string]any, error) {
	scope := strings.Trim(strings.TrimSpace(stringArg(args, "scope")), "/")
	if scope == "" {
		return nil, fmt.Errorf("scope is required")
	}
	counts := map[string]int64{}
	queries := map[string]string{
		"pipeline_runs": "SELECT COUNT(*) FROM pipeline_runs WHERE scope = $1",
		"schedules":     "SELECT COUNT(*) FROM pipeline_schedules WHERE scope = $1",
		"secrets":       "SELECT COUNT(*) FROM secrets WHERE scope = $1",
		"variables":     "SELECT COUNT(*) FROM variables WHERE scope = $1",
	}
	for key, query := range queries {
		var count int64
		if err := a.db.QueryRow(ctx, query, scope).Scan(&count); err != nil {
			return nil, err
		}
		counts[key] = count
	}
	return map[string]any{
		"scope":       scope,
		"usage":       counts,
		"explanation": "Scope permissions are enforced through AAA grants and resource-use checks. Use GitOps access grants for durable enterprise changes.",
	}, nil
}

func (a *App) hostedMCPListKnowledgeContexts(ctx context.Context, subject aaamodel.Subject, args map[string]any) (map[string]any, error) {
	kindFilter := strings.TrimSpace(stringArg(args, "kind"))
	if kindFilter != "" {
		kind, err := normalizeKnowledgeContextKind(kindFilter)
		if err != nil {
			return nil, err
		}
		kindFilter = kind
	}
	teamFilter := strings.Trim(strings.TrimSpace(hostedMCPKnowledgeContextTeamArg(args)), "/")
	if teamFilter != "" {
		team, err := normalizeKnowledgeContextTeam(teamFilter)
		if err != nil {
			return nil, err
		}
		teamFilter = team
	}
	query := strings.ToLower(strings.TrimSpace(stringArg(args, "query")))
	usedByPipeline := strings.Trim(strings.TrimSpace(stringArg(args, "used_by_pipeline")), "/")

	rows, err := a.db.Query(ctx, `
			SELECT id::text, kind, team_path, name, description, source,
			       managed_by_config_repo, config_source_path, config_source_commit_sha, updated_at,
			       sync_mode, sync_interval_minutes, failure_mode, sync_status, sync_error
		FROM knowledge_contexts
		WHERE ($1 = '' OR kind = $1)
		  AND ($2 = '' OR team_path = $2 OR team_path LIKE $2 || '/%')
		  AND ($3 = '' OR LOWER(name || ' ' || description || ' ' || content) LIKE '%' || $3 || '%')
		ORDER BY kind ASC, team_path ASC, name ASC
	`, kindFilter, teamFilter, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	usage := a.knowledgeContextUsage(ctx)
	items := []map[string]any{}
	limit := limitArg(args, 50, 200)
	for rows.Next() {
		var item knowledgeContextListItem
		var managed bool
		if err := rows.Scan(
			&item.UUID, &item.Kind, &item.Team, &item.Name, &item.Description, &item.Source,
			&managed, &item.GitOpsPath, &item.GitOpsCommit, &item.UpdatedAt,
			&item.SyncMode, &item.SyncInterval, &item.FailureMode, &item.SyncStatus, &item.SyncError,
		); err != nil {
			return nil, err
		}
		item.ID = buildKnowledgeContextIdentifier(item.Kind, item.Team, item.Name)
		if usedByPipeline != "" && !hostedMCPContainsString(usage[item.ID], usedByPipeline) {
			continue
		}
		if !a.hostedMCPAllowed(ctx, subject, hostedMCPReadPermission("knowledge_context.read", grantResourceKnowledgeContext, item.ID)) {
			continue
		}
		item.Visibility, err = a.resourceVisibility(ctx, grantResourceKnowledgeContext, item.ID)
		if err != nil {
			return nil, err
		}
		if managed {
			item.Source = knowledgeSourceGitOps
		}
		item.Access = item.Visibility
		item.UsedBy = usage[item.ID]
		item.UsedByCount = len(item.UsedBy)
		items = append(items, hostedMCPKnowledgeContextListItem(item))
		if len(items) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"knowledge_contexts": items}, nil
}

func (a *App) hostedMCPListKnowledgeConnections(ctx context.Context, subject aaamodel.Subject, args map[string]any) (map[string]any, error) {
	teamFilter := strings.Trim(strings.TrimSpace(hostedMCPKnowledgeContextTeamArg(args)), "/")
	if teamFilter != "" {
		team, err := normalizeKnowledgeConnectionTeam(teamFilter)
		if err != nil {
			return nil, err
		}
		teamFilter = team
	}
	providerFilter := strings.TrimSpace(stringArg(args, "provider"))
	if providerFilter != "" {
		provider, err := normalizeKnowledgeConnectionProvider(providerFilter)
		if err != nil {
			return nil, err
		}
		providerFilter = provider
	}
	query := strings.ToLower(strings.TrimSpace(stringArg(args, "query")))
	rows, err := a.db.Query(ctx, `
		SELECT c.id::text, c.team_path, c.name, c.display_name, c.provider, c.status, c.disabled,
		       c.credential_ref, c.base_url, c.scopes, c.config, c.last_checked_at, c.last_error,
		       c.updated_at,
		       COUNT(k.id)::int AS document_count,
		       (COUNT(k.id) FILTER (WHERE COALESCE(k.external_page_id, '') <> ''))::int AS external_document_count
		FROM knowledge_context_connections c
		LEFT JOIN knowledge_contexts k ON k.connection_id = c.id
		WHERE ($1 = '' OR c.team_path = $1 OR c.team_path LIKE $1 || '/%')
		  AND ($2 = '' OR c.provider = $2)
		  AND ($3 = '' OR LOWER(c.team_path || ' ' || c.name || ' ' || c.display_name || ' ' || c.provider) LIKE '%' || $3 || '%')
		GROUP BY c.id, c.team_path, c.name, c.display_name, c.provider, c.status, c.disabled,
		         c.credential_ref, c.base_url, c.scopes, c.config, c.last_checked_at, c.last_error, c.updated_at
		ORDER BY c.team_path ASC, c.name ASC
	`, teamFilter, providerFilter, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []map[string]any{}
	limit := limitArg(args, 50, 200)
	for rows.Next() {
		record, err := scanKnowledgeConnectionRecord(rows)
		if err != nil {
			return nil, err
		}
		if !a.hostedMCPAllowed(ctx, subject, hostedMCPReadPermission("knowledge_connection.read", grantResourceKnowledgeConnection, record.ID)) {
			continue
		}
		items = append(items, hostedMCPKnowledgeConnectionListItem(record.knowledgeConnectionListItem))
		if len(items) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"knowledge_connections": items}, nil
}

func (a *App) hostedMCPGetKnowledgeContext(ctx context.Context, subject aaamodel.Subject, args map[string]any) (map[string]any, error) {
	detail, err := a.loadHostedMCPKnowledgeContextDetail(ctx, args)
	if err != nil {
		return nil, err
	}
	if !a.hostedMCPAllowed(ctx, subject, hostedMCPReadPermission("knowledge_context.read", grantResourceKnowledgeContext, detail.ID)) {
		return nil, fmt.Errorf("knowledge context %s is not allowed", detail.ID)
	}
	return hostedMCPKnowledgeContextDetail(detail), nil
}

func (a *App) hostedMCPGetPipelineKnowledgeContext(ctx context.Context, subject aaamodel.Subject, args map[string]any) (map[string]any, error) {
	raw := strings.TrimSpace(stringArg(args, "yaml"))
	pipelineID := ""
	if raw == "" {
		pipeline, err := a.hostedMCPGetPipeline(ctx, args)
		if err != nil {
			return nil, err
		}
		raw = assistantOutputString(pipeline, "definition")
		pipelineID = assistantOutputString(pipeline, "id")
	}
	if raw == "" {
		return nil, fmt.Errorf("pipeline yaml or pipeline identifier is required")
	}

	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(raw), &pipeline); err != nil {
		return map[string]any{"valid": false, "error": err.Error(), "applies": false}, nil
	}
	refs := collectPipelineKnowledgeContextRefs(pipeline)
	includeContent := boolArg(args, "include_content", true)
	references := make([]map[string]any, 0, len(refs))
	documents := []map[string]any{}
	unresolved := []map[string]any{}
	for _, entry := range refs {
		ref := entry.Ref
		reference := map[string]any{
			"location": entry.Location,
			"kind":     strings.TrimSpace(ref.Kind),
			"ref":      strings.Trim(strings.TrimSpace(ref.Ref), "/"),
			"path":     strings.TrimSpace(ref.Path),
			"required": ref.Required,
		}
		references = append(references, reference)

		kind, err := normalizeKnowledgeContextKind(ref.Kind)
		if err != nil {
			unresolved = append(unresolved, map[string]any{"location": entry.Location, "status": "invalid_kind", "error": err.Error()})
			continue
		}
		if strings.TrimSpace(ref.Ref) == "" {
			if strings.TrimSpace(ref.Path) != "" {
				unresolved = append(unresolved, map[string]any{
					"location": entry.Location,
					"kind":     kind,
					"path":     strings.TrimSpace(ref.Path),
					"status":   "repo_local",
					"note":     "Repo-local knowledge context is resolved at run time from git owner, repo, and commit.",
				})
			}
			continue
		}
		_, team, name, err := knowledgeContextRefToParts(kind, ref.Ref)
		if err != nil {
			unresolved = append(unresolved, map[string]any{"location": entry.Location, "kind": kind, "ref": ref.Ref, "status": "invalid_ref", "error": err.Error()})
			continue
		}
		id := buildKnowledgeContextIdentifier(kind, team, name)
		if !a.hostedMCPAllowed(ctx, subject, hostedMCPReadPermission("knowledge_context.read", grantResourceKnowledgeContext, id)) {
			documents = append(documents, map[string]any{"id": id, "status": "denied", "location": entry.Location})
			continue
		}
		detail, err := a.loadKnowledgeContextDetail(ctx, kind, team, name)
		if err != nil {
			unresolved = append(unresolved, map[string]any{"location": entry.Location, "id": id, "status": "not_found", "error": err.Error()})
			continue
		}
		document := hostedMCPKnowledgeContextDetail(detail)
		document["location"] = entry.Location
		if !includeContent {
			delete(document, "content")
		}
		documents = append(documents, document)
	}
	if pipelineID == "" {
		pipelineID = pipelineWritePlanResourceID(map[string]any{"yaml": raw})
	}
	return map[string]any{
		"valid":              true,
		"pipeline_id":        pipelineID,
		"references":         references,
		"documents":          documents,
		"unresolved":         unresolved,
		"document_count":     len(documents),
		"unresolved_count":   len(unresolved),
		"include_content":    includeContent,
		"gitops_compatible":  true,
		"applies":            false,
		"resolution_surface": "managed knowledge is read from Nopsai; repo-local knowledge is resolved during pipeline runs.",
	}, nil
}

func (a *App) hostedMCPSearchDocs(ctx context.Context, args map[string]any) (map[string]any, error) {
	query := strings.TrimSpace(stringArg(args, "query"))
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	limit := limitArg(args, 10, 50)
	docs := []map[string]any{}
	if a != nil && a.db != nil {
		rows, err := a.db.Query(ctx, `
			SELECT id::text, kind, team_path, name, description
			FROM knowledge_contexts
			WHERE LOWER(name || ' ' || description || ' ' || content) LIKE LOWER('%' || $1 || '%')
			ORDER BY kind ASC, team_path ASC, name ASC
			LIMIT $2
		`, query, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var id, kind, teamPath, name, description string
			if err := rows.Scan(&id, &kind, &teamPath, &name, &description); err != nil {
				return nil, err
			}
			docs = append(docs, map[string]any{"id": id, "kind": kind, "team": teamPath, "team_path": teamPath, "name": name, "description": description, "source": "knowledge_context"})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	if len(docs) < limit {
		docs = append(docs, hostedMCPStaticDocsSearchResults(query, limit-len(docs))...)
	}
	return map[string]any{"docs": docs}, nil
}

func (a *App) hostedMCPReadDoc(ctx context.Context, args map[string]any) (map[string]any, error) {
	id := stringArg(args, "id")
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	if doc, ok := hostedMCPStaticDocByID(id); ok {
		return doc, nil
	}
	var kind, teamPath, name, description, content string
	err := a.db.QueryRow(ctx, `
		SELECT kind, team_path, name, description, content
		FROM knowledge_contexts
		WHERE id::text = $1 OR kind || '/' || CASE WHEN team_path = '' THEN '' ELSE team_path || '/' END || name = $1
		LIMIT 1
	`, id).Scan(&kind, &teamPath, &name, &description, &content)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "kind": kind, "team": teamPath, "team_path": teamPath, "name": name, "description": description, "content": content, "source": "knowledge_context"}, nil
}

type hostedMCPStaticDoc struct {
	ID          string
	Kind        string
	Name        string
	Description string
	Path        string
	Content     string
	Keywords    []string
}

var hostedMCPStaticDocs = []hostedMCPStaticDoc{
	{
		ID:          "doc/dashboards.md",
		Kind:        "product_doc",
		Name:        "Team Dashboards",
		Description: "Dashboard YAML, pipeline dashboard final outputs, DashboardSpec validation, source bindings, refresh, and GitOps examples.",
		Path:        "doc/dashboards.md",
		Keywords: []string{
			"pipeline dashboard data publication example",
			"pipeline sends data to dashboard",
			"working dashboard pipeline definition",
			"dashboard final output",
			"DashboardSpec",
		},
		Content: strings.TrimSpace(`Team dashboards are populated by validated pipeline final outputs. A pipeline publishes dashboard data by declaring an output item with type: dashboard under output.items. NopsAI generates a validated DashboardSpec from the run context and output prompt, then publishes it to the target dashboard when the run subject has dashboard.publish.

Working pipeline example:

` + "```yaml" + `
name: service-health-dashboard
version: "1.0"
description: Publish service health evidence into Engineering Health.
container_image: alpine:3.20
llm_profile: standard

steps:
  - name: collect-evidence
    script: |
      cat > dashboard-evidence.json <<'JSON'
      {
        "service": "payments-api",
        "summary": "Payments API is healthy. Build and deploy passed; latency is within target.",
        "actions": [
          {"label": "Review slow checkout test", "status": "watch", "tone": "warning"},
          {"label": "Keep deployment canary at 25 percent for 30 minutes", "status": "open", "tone": "info"},
          {"label": "Archive successful release notes", "status": "done", "tone": "success"}
        ],
        "stage_results": [
          {"label": "build", "value": 42},
          {"label": "test", "value": 39},
          {"label": "deploy", "value": 37}
        ]
      }
      JSON
      cat dashboard-evidence.json

output:
  items:
    - name: service-health-widgets
      type: dashboard
      when: always
      dashboard:
        ref: platform/engineering-health
        section: service-health
        entry_key: payments-api
        mode: replace
        preset: auto
        ttl: 7d
      prompt: |
        Build a dashboard summary for payments-api from the run evidence.
        Show the current service summary, next actions, and stage throughput.
` + "```" + `

Pipeline authors describe the dashboard they want. NopsAI sends emitted step output evidence first, then run metadata, recent same-pipeline run history, pipeline context, step and task durations, child runs, and dashboard intent to the configured LLM, then validates and repairs the generated DashboardSpec before publication. Emitted step stdout/stderr, including plain echo output and structured JSON/NDJSON, is authoritative for business facts such as artifact names, versions, durations, services, and subjects; configured container images, runner/runtime metadata, image-pull logs, and recent-history values must not replace values present in emitted step output. For example, the prompt can say: "Show how many images were built, which version each image used, how long each image build took, and the most important subject in this pipeline." NopsAI chooses the dashboard structure dynamically from the prompt and evidence. If the prompt does not specify a visualization, NopsAI guides the model to choose by data shape: text or callout for narrative conclusions, status/progress/properties for current state and scalar facts, tables for repeated records, bar charts for categorical counts, durations, and rankings, line or area charts for time series, and pie or donut charts only for bounded part-to-whole data. Dashboard presets are shape hints: report is narrative-first with tables only as supporting evidence, table makes one table primary, status starts with current health/readiness, timeline orders events or series chronologically, comparison presents side-by-side differences, metrics starts with headline numbers and charts, mixed composes complementary operator blocks, and auto chooses the smallest useful layout. Series-mode dashboard outputs must include at least one chart or series block with chart points. Generated dashboard output uses a flat top-level blocks array; common generated wrappers such as top-level widgets, sections[].blocks, and nested blocks/widgets wrappers are normalized before strict validation, and display key aliases are normalized to labels. Authors do not need to know or target the dashboard renderer schema. Emit structured evidence such as JSON to stdout/stderr when the dashboard should use rich or nested data produced by a step. The sample config repo includes five immediately runnable dashboard pipelines: technical-api-readiness, technical-slo-burn-rate, customer-onboarding-pulse, finance-close-snapshot, and people-capacity-plan. They use alpine shell scripts, require no variables, secrets, approvals, or external MCP profiles, and publish to team-1/ops-dashboard.

The dashboard itself can be managed by GitOps under dashboards/platform/engineering-health.yaml with a section whose section_key is service-health and a source binding whose pipeline_id is service-health-dashboard and output_name is service-health-widgets. A source binding may leave entry_key empty to use the output name as the dashboard entry key; this is how operators remove an explicit entry-key binding without breaking output-name fallback publication. Operators can also remove a visible section entry card through the dashboard publication DELETE route; this archives the current publication and writes history without deleting sources or runs. Dashboard refresh source rows keep the refresh rollup status separate from the launched pipeline status and the final output status so a finished pipeline is not confused with a finished output.
`),
	},
	{
		ID:          "doc/final-output-rendering.md",
		Kind:        "product_doc",
		Name:        "Pipeline Final Output Rendering",
		Description: "Final output contracts for Markdown, JSON, PDF, HTML, Excel, and dashboard publications.",
		Path:        "doc/final-output-rendering.md",
		Keywords: []string{
			"dashboard output",
			"pipeline final output",
			"DashboardSpec JSON",
			"output.items dashboard",
		},
		Content: strings.TrimSpace(`Pipeline final outputs are run-owned deliverables configured through output.items in pipeline YAML.

Dashboard outputs use type: dashboard. Authors write the dashboard intent in prompt; NopsAI supplies emitted step output first as authoritative business evidence, then run metadata, recent same-pipeline run history, step and task durations, and child runs to the configured LLM. It guides visualization selection when the prompt does not name one, normalizes common generated wrappers such as sections[].blocks, widgets, or nested blocks/widgets wrappers into flat DashboardSpec blocks, normalizes display key aliases to labels, validates the generated DashboardSpec, and retries invalid generations.

Dashboard outputs are published to team-owned dashboards when their output.items[].dashboard target is valid and the run subject has dashboard.publish. Publication modes are replace, append, snapshot, and series. Run detail and hosted MCP final-output responses include created_at, generation_started_at, updated_at, generation_duration, and generation_duration_seconds so operators can inspect output generation timing separately from queue time and pipeline run duration.
`),
	},
}

func hostedMCPStaticDocsSearchResults(query string, limit int) []map[string]any {
	if limit <= 0 {
		return []map[string]any{}
	}
	type scoredDoc struct {
		doc   hostedMCPStaticDoc
		score int
	}
	scored := []scoredDoc{}
	for _, doc := range hostedMCPStaticDocs {
		score := hostedMCPStaticDocScore(query, doc)
		if score <= 0 {
			continue
		}
		scored = append(scored, scoredDoc{doc: doc, score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].doc.ID < scored[j].doc.ID
	})
	results := make([]map[string]any, 0, min(len(scored), limit))
	for idx, item := range scored {
		if idx >= limit {
			break
		}
		results = append(results, hostedMCPStaticDocSearchResult(item.doc, query))
	}
	return results
}

func hostedMCPStaticDocByID(id string) (map[string]any, bool) {
	id = strings.Trim(strings.TrimSpace(id), "/")
	for _, doc := range hostedMCPStaticDocs {
		if id == doc.ID || id == strings.TrimPrefix(doc.ID, "doc/") || strings.EqualFold(id, doc.Name) {
			return map[string]any{
				"id":          doc.ID,
				"kind":        doc.Kind,
				"team":        "",
				"team_path":   "",
				"name":        doc.Name,
				"description": doc.Description,
				"path":        doc.Path,
				"content":     doc.Content,
				"source":      "product_docs",
			}, true
		}
	}
	return nil, false
}

func hostedMCPStaticDocSearchResult(doc hostedMCPStaticDoc, query string) map[string]any {
	return map[string]any{
		"id":          doc.ID,
		"kind":        doc.Kind,
		"team":        "",
		"team_path":   "",
		"name":        doc.Name,
		"description": doc.Description,
		"path":        doc.Path,
		"snippet":     hostedMCPDocSnippet(doc.Content, query),
		"source":      "product_docs",
	}
}

func hostedMCPStaticDocScore(query string, doc hostedMCPStaticDoc) int {
	lowerQuery := strings.ToLower(strings.TrimSpace(query))
	if lowerQuery == "" {
		return 1
	}
	haystack := strings.ToLower(strings.Join([]string{
		doc.ID,
		doc.Name,
		doc.Description,
		doc.Path,
		strings.Join(doc.Keywords, " "),
		doc.Content,
	}, " "))
	score := 0
	if strings.Contains(haystack, lowerQuery) {
		score += 100
	}
	for _, keyword := range doc.Keywords {
		if strings.Contains(lowerQuery, strings.ToLower(keyword)) || strings.Contains(strings.ToLower(keyword), lowerQuery) {
			score += 80
		}
	}
	for _, token := range hostedMCPDocSearchTokens(lowerQuery) {
		if strings.Contains(haystack, token) {
			score += 10
		}
	}
	if score >= 20 {
		return score
	}
	return 0
}

func hostedMCPDocSearchTokens(query string) []string {
	fields := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
	stop := map[string]bool{
		"and": true, "any": true, "can": true, "data": true, "for": true, "from": true,
		"have": true, "how": true, "implemented": true, "into": true, "just": true,
		"need": true, "see": true, "send": true, "sends": true, "that": true,
		"the": true, "this": true, "to": true, "want": true, "which": true, "with": true,
	}
	tokens := []string{}
	seen := map[string]bool{}
	for _, field := range fields {
		if len(field) < 3 || stop[field] || seen[field] {
			continue
		}
		seen[field] = true
		tokens = append(tokens, field)
	}
	return tokens
}

func hostedMCPDocSnippet(content, query string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	lowerQuery := strings.ToLower(query)
	if strings.Contains(lowerQuery, "pipeline") && strings.Contains(lowerQuery, "dashboard") && strings.Contains(content, "output:\n  items:") {
		if snippet := hostedMCPSnippet(content, "output:", 800); snippet != "" {
			return snippet
		}
	}
	for _, token := range hostedMCPDocSearchTokens(query) {
		if snippet := hostedMCPSnippet(content, token, 480); snippet != "" {
			return snippet
		}
	}
	if len(content) <= 480 {
		return content
	}
	return strings.TrimSpace(content[:480]) + "..."
}

func (a *App) hostedMCPStatistics(ctx context.Context) (map[string]any, error) {
	counts := map[string]int64{}
	queries := map[string]string{
		"pipelines":     "SELECT COUNT(*) FROM pipelines",
		"pipeline_runs": "SELECT COUNT(*) FROM pipeline_runs",
		"triggers":      "SELECT COUNT(*) FROM triggers",
		"schedules":     "SELECT COUNT(*) FROM pipeline_schedules",
		"scopes":        "SELECT COUNT(DISTINCT scope) FROM pipeline_runs WHERE COALESCE(scope, '') <> ''",
		"knowledge":     "SELECT COUNT(*) FROM knowledge_contexts",
	}
	for key, query := range queries {
		var count int64
		if err := a.db.QueryRow(ctx, query).Scan(&count); err != nil {
			return nil, err
		}
		counts[key] = count
	}
	return map[string]any{"counts": counts}, nil
}

func (a *App) hostedMCPCostSummary(ctx context.Context) (map[string]any, error) {
	var runCost, aiCost, totalCost float64
	err := a.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(runner_cost_usd), 0), COALESCE(SUM(ai_cost_usd), 0), COALESCE(SUM(total_cost_usd), 0)
		FROM pipeline_run_usage_summary
	`).Scan(&runCost, &aiCost, &totalCost)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return map[string]any{"runner_cost_usd": 0, "ai_cost_usd": 0, "total_cost_usd": 0}, nil
		}
		return nil, err
	}
	return map[string]any{"runner_cost_usd": runCost, "ai_cost_usd": aiCost, "total_cost_usd": totalCost}, nil
}

func (a *App) hostedMCPSuggestCostImprovements(ctx context.Context) (map[string]any, error) {
	summary, err := a.hostedMCPCostSummary(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"cost_summary": summary,
		"suggestions": []string{
			"Review high-cost runs in Monitoring before changing retention or runner sizing.",
			"Prefer scoped LLM profiles with explicit limits for AI-heavy pipelines.",
			"Use data cleanup previews before deleting historical logs or runs.",
		},
	}, nil
}

func (a *App) hostedMCPSuggestDesignImprovements(ctx context.Context) (map[string]any, error) {
	stats, err := a.hostedMCPStatistics(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"statistics": stats,
		"suggestions": []string{
			"Keep reusable logic in steps and include it from pipelines to reduce drift.",
			"Keep production changes GitOps-managed through config repositories.",
			"Attach knowledge/runbook context to pipelines that rely on human diagnosis.",
		},
	}, nil
}

func (a *App) hostedMCPGetLLMProfiles(ctx context.Context) (map[string]any, error) {
	defaultProfile, profiles, _, err := a.loadLLMProfilesFromDB(ctx)
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		defaultProfile, profiles = a.llmProfilesSnapshot()
	}
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	items := []map[string]any{}
	for _, name := range names {
		profile := config.NormalizeLLMProfile(profiles[name])
		items = append(items, map[string]any{
			"name":           name,
			"provider":       profile.Provider,
			"model":          profile.Model,
			"base_url":       profile.BaseURL,
			"allowed_scopes": profile.AllowedScopes,
			"prompt_cache":   profile.PromptCache,
			"provider_state": profile.ProviderState,
		})
	}
	return map[string]any{"default_profile": defaultProfile, "profiles": items}, nil
}

func (a *App) hostedMCPGetMCPProfiles(ctx context.Context) (map[string]any, error) {
	profiles, err := a.loadMCPProfilesFromDB(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	items := []models.MCPProfile{}
	for _, name := range names {
		items = append(items, models.NormalizeMCPProfile(profiles[name]))
	}
	return map[string]any{"profiles": items}, nil
}

func (a *App) hostedMCPGetSystemStatus(ctx context.Context) (map[string]any, error) {
	stats, err := a.hostedMCPStatistics(ctx)
	if err != nil {
		return nil, err
	}
	cfg := a.getConfigSnapshot()
	return map[string]any{
		"environment": cfg.EffectiveEnvironment(),
		"runtime":     cfg.Runtime,
		"assistant":   cfg.EffectiveAssistantConfig(),
		"statistics":  stats,
	}, nil
}

func (a *App) hostedMCPGetDispatcherStatus(ctx context.Context) (map[string]any, error) {
	status, dispatcherErr := a.fetchDispatcherStatus(ctx)
	if dispatcherErr == nil {
		a.sampleMonitoringRunnerSnapshots(ctx, status)
	}
	runners, summary := monitoringRunnersFromDispatcherStatus(status, nil)
	dispatcher := map[string]any{
		"status": "ok",
	}
	if dispatcherErr != nil {
		dispatcher["status"] = "error"
		dispatcher["error"] = dispatcherErr.Error()
	} else if status == nil {
		dispatcher["status"] = "warning"
		dispatcher["message"] = "Dispatcher status has not been loaded."
	} else {
		dispatcher["queued_jobs"] = status.GetQueuedJobs()
		dispatcher["runner_count"] = len(status.GetRunners())
	}
	return map[string]any{
		"dispatcher":     dispatcher,
		"queued_jobs":    summary.QueuedJobs,
		"runner_summary": summary,
		"runners":        runners,
		"services":       a.buildSystemServiceStatuses(ctx, status, dispatcherErr),
	}, nil
}

func hostedMCPProposal(kind string, args map[string]any) map[string]any {
	return map[string]any{
		"proposal_type": kind + "_change",
		"applies":       false,
		"target":        args,
		"note":          "This hosted MCP foundation returns proposals only. Applying changes must go through the existing API/GitOps approval flow.",
	}
}

func splitPipelineArg(args map[string]any) (string, string) {
	pathPart := strings.Trim(strings.TrimSpace(stringArg(args, "path")), "/")
	namePart := strings.TrimSpace(stringArg(args, "name"))
	if namePart != "" {
		return pathPart, namePart
	}
	id := strings.Trim(strings.TrimSpace(stringArg(args, "pipeline")), "/")
	if id == "" {
		return pathPart, ""
	}
	parts := strings.Split(id, "/")
	if len(parts) == 1 {
		return "", parts[0]
	}
	return strings.Join(parts[:len(parts)-1], "/"), parts[len(parts)-1]
}

func pipelineArgID(args map[string]any) string {
	pathPart, namePart := splitPipelineArg(args)
	return aaamodel.BuildPipelineID(pathPart, namePart)
}

func hostedMCPPipelineYAMLWithFallbackName(raw, fallbackName string) string {
	fallbackName = strings.TrimSpace(fallbackName)
	if fallbackName == "" {
		return raw
	}
	var payload map[string]any
	if err := yaml.Unmarshal([]byte(raw), &payload); err != nil || payload == nil {
		return raw
	}
	if existing := strings.TrimSpace(fmt.Sprint(payload["name"])); existing != "" && existing != "<nil>" {
		return raw
	}
	payload["name"] = fallbackName
	encoded, err := yaml.Marshal(payload)
	if err != nil {
		return raw
	}
	return string(encoded)
}

func hostedMCPPipelineMatchFields(query, path, name, version, source, visibility, definition string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	fields := []struct {
		name  string
		value string
	}{
		{name: "path", value: path},
		{name: "name", value: name},
		{name: "version", value: version},
		{name: "source", value: source},
		{name: "visibility", value: visibility},
		{name: "definition", value: definition},
	}
	matches := []string{}
	for _, field := range fields {
		if hostedMCPTextMatchesSearch(strings.ToLower(field.value), query) {
			matches = append(matches, field.name)
		}
	}
	return matches
}

func onlyPipelineDefinitionMatched(fields []string) bool {
	return len(fields) == 1 && fields[0] == "definition"
}

func hostedMCPSnippet(content, query string, maxLen int) string {
	content = strings.TrimSpace(content)
	query = strings.TrimSpace(query)
	if content == "" || query == "" || maxLen <= 0 {
		return ""
	}
	lowerContent := strings.ToLower(content)
	lowerQuery := strings.ToLower(query)
	index := strings.Index(lowerContent, lowerQuery)
	if index < 0 {
		for _, term := range hostedMCPSearchTerms(query) {
			index = strings.Index(lowerContent, term)
			if index >= 0 {
				break
			}
		}
	}
	if index < 0 {
		return ""
	}
	start := index - maxLen/3
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(content) {
		end = len(content)
		start = end - maxLen
		if start < 0 {
			start = 0
		}
	}
	snippet := strings.TrimSpace(content[start:end])
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(content) {
		snippet += "..."
	}
	return snippet
}

func hostedMCPTextMatchesSearch(value, query string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	query = strings.ToLower(strings.TrimSpace(query))
	if value == "" || query == "" {
		return false
	}
	if strings.Contains(value, query) {
		return true
	}
	terms := hostedMCPSearchTerms(query)
	if len(terms) == 0 {
		return false
	}
	for _, term := range terms {
		if !strings.Contains(value, term) {
			return false
		}
	}
	return true
}

func hostedMCPSearchPatterns(query string) []string {
	terms := hostedMCPSearchTerms(query)
	patterns := make([]string, 0, len(terms))
	for _, term := range terms {
		patterns = append(patterns, "%"+term+"%")
	}
	return patterns
}

func hostedMCPSearchTerms(query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return []string{}
	}
	stopWords := map[string]struct{}{
		"a": {}, "an": {}, "and": {}, "are": {}, "for": {}, "has": {}, "have": {}, "having": {}, "in": {}, "me": {}, "of": {}, "or": {}, "pipeline": {}, "pipelineruns": {}, "pipelines": {}, "please": {}, "run": {}, "runs": {}, "show": {}, "step": {}, "steps": {}, "that": {}, "the": {}, "to": {}, "with": {},
	}
	terms := []string{}
	seen := map[string]struct{}{}
	for _, part := range strings.FieldsFunc(query, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_' && r != '-' && r != '/'
	}) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, skip := stopWords[part]; skip {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		terms = append(terms, part)
	}
	return terms
}

func pipelineWritePlanResourceID(args map[string]any) string {
	pathPart, namePart := splitPipelineArg(args)
	if namePart != "" {
		return aaamodel.BuildPipelineID(pathPart, namePart)
	}
	raw := stringArg(args, "yaml")
	if raw == "" {
		return ""
	}
	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(raw), &pipeline); err != nil {
		return ""
	}
	return aaamodel.BuildPipelineID(pathPart, pipeline.Name)
}

func (a *App) knowledgeContextArgID(ctx context.Context, args map[string]any) string {
	id := strings.Trim(strings.TrimSpace(stringArg(args, "id")), "/")
	if id != "" {
		if strings.Contains(id, "/") {
			kind, team, name, err := splitKnowledgeContextIdentifier(id)
			if err == nil {
				return buildKnowledgeContextIdentifier(kind, team, name)
			}
		}
		if a != nil && a.db != nil {
			var kind, team, name string
			err := a.db.QueryRow(ctx, `SELECT kind, team_path, name FROM knowledge_contexts WHERE id::text = $1`, id).Scan(&kind, &team, &name)
			if err == nil {
				return buildKnowledgeContextIdentifier(kind, team, name)
			}
		}
		return id
	}
	kind := stringArg(args, "kind")
	ref := stringArg(args, "ref")
	if kind != "" && ref != "" {
		kind, team, name, err := knowledgeContextRefToParts(kind, ref)
		if err == nil {
			return buildKnowledgeContextIdentifier(kind, team, name)
		}
	}
	kind = firstNonEmptyString(kind, stringArg(args, "document_kind"))
	team := hostedMCPKnowledgeContextTeamArg(args)
	name := stringArg(args, "name")
	if kind == "" || team == "" || name == "" {
		return ""
	}
	kind, kindErr := normalizeKnowledgeContextKind(kind)
	team, teamErr := normalizeKnowledgeContextTeam(team)
	name, nameErr := normalizeKnowledgeContextName(name)
	if kindErr != nil || teamErr != nil || nameErr != nil {
		return ""
	}
	return buildKnowledgeContextIdentifier(kind, team, name)
}

func (a *App) loadHostedMCPKnowledgeContextDetail(ctx context.Context, args map[string]any) (knowledgeContextDetail, error) {
	id := strings.Trim(strings.TrimSpace(stringArg(args, "id")), "/")
	if id != "" {
		if strings.Contains(id, "/") {
			kind, team, name, err := splitKnowledgeContextIdentifier(id)
			if err != nil {
				return knowledgeContextDetail{}, err
			}
			return a.loadKnowledgeContextDetail(ctx, kind, team, name)
		}
		var kind, team, name string
		err := a.db.QueryRow(ctx, `SELECT kind, team_path, name FROM knowledge_contexts WHERE id::text = $1`, id).Scan(&kind, &team, &name)
		if err != nil {
			return knowledgeContextDetail{}, err
		}
		return a.loadKnowledgeContextDetail(ctx, kind, team, name)
	}
	kind := stringArg(args, "kind")
	ref := stringArg(args, "ref")
	if kind != "" && ref != "" {
		kind, team, name, err := knowledgeContextRefToParts(kind, ref)
		if err != nil {
			return knowledgeContextDetail{}, err
		}
		return a.loadKnowledgeContextDetail(ctx, kind, team, name)
	}
	kind, err := normalizeKnowledgeContextKind(kind)
	if err != nil {
		return knowledgeContextDetail{}, err
	}
	team, err := normalizeKnowledgeContextTeam(hostedMCPKnowledgeContextTeamArg(args))
	if err != nil {
		return knowledgeContextDetail{}, err
	}
	name, err := normalizeKnowledgeContextName(stringArg(args, "name"))
	if err != nil {
		return knowledgeContextDetail{}, err
	}
	return a.loadKnowledgeContextDetail(ctx, kind, team, name)
}

func hostedMCPKnowledgeContextListItem(item knowledgeContextListItem) map[string]any {
	return map[string]any{
		"id":                         item.ID,
		"uuid":                       item.UUID,
		"kind":                       item.Kind,
		"team":                       item.Team,
		"name":                       item.Name,
		"description":                item.Description,
		"visibility":                 item.Visibility,
		"access":                     item.Access,
		"source":                     item.Source,
		"updated_at":                 item.UpdatedAt,
		"used_by_count":              item.UsedByCount,
		"used_by":                    item.UsedBy,
		"config_source_path":         item.GitOpsPath,
		"config_source_commit_sha":   item.GitOpsCommit,
		"connection_id":              item.ConnectionID,
		"connection_ref":             item.ConnectionRef,
		"external_provider":          item.ExternalProvider,
		"external_page_id":           item.ExternalPageID,
		"external_page_url":          item.ExternalPageURL,
		"sync_mode":                  item.SyncMode,
		"sync_interval_minutes":      item.SyncInterval,
		"failure_mode":               item.FailureMode,
		"sync_status":                item.SyncStatus,
		"last_synced_at":             item.LastSyncedAt,
		"sync_error":                 item.SyncError,
		"gitops_managed":             item.Source == knowledgeSourceGitOps,
		"knowledge_context_resource": grantResourceKnowledgeContext + ":" + item.ID,
	}
}

func hostedMCPKnowledgeConnectionListItem(item knowledgeConnectionListItem) map[string]any {
	return map[string]any{
		"id":                            item.ID,
		"uuid":                          item.UUID,
		"team":                          item.Team,
		"name":                          item.Name,
		"display_name":                  item.DisplayName,
		"provider":                      item.Provider,
		"status":                        item.Status,
		"disabled":                      item.Disabled,
		"base_url":                      item.BaseURL,
		"credential_visibility":         item.CredentialVisibility,
		"last_checked_at":               item.LastCheckedAt,
		"last_error":                    item.LastError,
		"updated_at":                    item.UpdatedAt,
		"document_count":                item.DocumentCount,
		"external_document_count":       item.ExternalDocumentCount,
		"knowledge_connection_resource": grantResourceKnowledgeConnection + ":" + item.ID,
	}
}

func hostedMCPKnowledgeContextDetail(detail knowledgeContextDetail) map[string]any {
	item := hostedMCPKnowledgeContextListItem(detail.knowledgeContextListItem)
	item["content"] = detail.Content
	item["assets"] = detail.Assets
	item["managed_by_config_repo"] = detail.ManagedByGit
	item["gitops_compatible"] = true
	return item
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func boolArg(args map[string]any, key string, fallback bool) bool {
	if args == nil {
		return fallback
	}
	value, ok := args[key]
	if !ok || value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		normalized := strings.ToLower(strings.TrimSpace(typed))
		if normalized == "" {
			return fallback
		}
		return normalized == "true" || normalized == "1" || normalized == "yes"
	default:
		return boolValue(typed)
	}
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		normalized := strings.ToLower(strings.TrimSpace(typed))
		return normalized == "true" || normalized == "1" || normalized == "yes"
	default:
		return false
	}
}

func hostedMCPContainsString(values []string, target string) bool {
	target = strings.Trim(strings.TrimSpace(target), "/")
	for _, value := range values {
		if strings.Trim(strings.TrimSpace(value), "/") == target {
			return true
		}
	}
	return false
}

func limitArg(args map[string]any, fallback, max int) int {
	if args == nil {
		return fallback
	}
	var raw float64
	switch value := args["limit"].(type) {
	case float64:
		raw = value
	case int:
		raw = float64(value)
	case json.Number:
		parsed, _ := value.Float64()
		raw = parsed
	default:
		return fallback
	}
	if raw <= 0 {
		return fallback
	}
	if int(raw) > max {
		return max
	}
	return int(raw)
}

func hostedMCPNullableTime(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}

func hostedMCPTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func reverseMaps(values []map[string]any) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

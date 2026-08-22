package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	aaamodel "nopsai/services/aaa/pkg/model"
)

type hostedMCPResource struct {
	URI         string              `json:"uri"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	MimeType    string              `json:"mimeType,omitempty"`
	Permission  hostedMCPPermission `json:"-"`
}

func allHostedMCPResources() []hostedMCPResource {
	return []hostedMCPResource{
		resourceDef("nopsai://docs", "Nopsai docs and knowledge", "Searchable Nopsai knowledge context.", "knowledge_context.read", "knowledge_context", "*"),
		resourceDef("nopsai://knowledge-contexts", "Knowledge contexts", "Managed Nopsai knowledge context inventory.", "knowledge_context.read", "knowledge_context", "*"),
		resourceDef("nopsai://knowledge-connections", "Knowledge connections", "Reusable team-owned Knowledge Context external page connections.", "knowledge_connection.read", "knowledge_connection", "*"),
		resourceDef("nopsai://pipelines", "Pipelines", "Pipeline inventory.", "pipeline.list", "pipeline", "*"),
		resourceDef("nopsai://pipeline-runs", "Pipeline runs", "Recent pipeline runs.", "pipeline_run.list", "pipeline_run", "*"),
		resourceDef("nopsai://dashboards", "Dashboards", "Team dashboard inventory and current publications.", "dashboard.list", "dashboard", "*"),
		resourceDef("nopsai://triggers", "Triggers", "Repository trigger inventory.", "trigger.read", "trigger", "*"),
		resourceDef("nopsai://schedules", "Schedules", "Pipeline schedule inventory.", "pipeline_schedule.list", "pipeline_schedule", "*"),
		resourceDef("nopsai://scopes", "Scopes", "Scope usage inventory.", "scope.read", "scope", "*"),
		resourceDef("nopsai://lab", "Lab", "Lab-oriented recent runs and results.", "pipeline_run.list", "pipeline_run", "*"),
		resourceDef("nopsai://statistics", "Statistics", "Platform statistics.", "pipeline_run.list", "pipeline_run", "*"),
		resourceDef("nopsai://costs", "Costs", "Cost and usage summary.", "pipeline_run.list", "pipeline_run", "*"),
		resourceDef("nopsai://system/models", "LLM profiles", "Existing LLM profile catalog.", "system.read", "system", "models"),
		resourceDef("nopsai://system/mcp-profiles", "MCP profiles", "External MCP profile catalog.", "system.read", "system", "mcp"),
		resourceDef("nopsai://features", "Feature capabilities", "NopsAI feature coverage, hosted MCP surfaces, REST/GitOps backing routes, and current-user AAA availability.", "system.read", "system", "mcp"),
		resourceDef("nopsai://system/status", "System status", "Basic system setup and assistant status.", "system.read", "system", "config"),
		resourceDef("nopsai://system/dispatcher", "Dispatcher and runners", "Dispatcher health, queued jobs, runner capacity, and runner status.", "system.read", "dispatcher", "status"),
	}
}

func resourceDef(uri, name, description, action, resourceType, resourceID string) hostedMCPResource {
	return hostedMCPResource{
		URI:         uri,
		Name:        name,
		Description: description,
		MimeType:    "application/json",
		Permission:  hostedMCPReadPermission(action, resourceType, resourceID),
	}
}

func (a *App) hostedMCPResourcesForSubject(ctx context.Context, subject aaamodel.Subject) []hostedMCPResource {
	all := allHostedMCPResources()
	resources := make([]hostedMCPResource, 0, len(all))
	for _, resource := range all {
		if a.hostedMCPAllowed(ctx, subject, resource.Permission) {
			resources = append(resources, resource)
		}
	}
	return resources
}

func (a *App) readHostedMCPResource(ctx context.Context, subject aaamodel.Subject, uri string) (hostedMCPResource, string, error) {
	uri = strings.TrimSpace(uri)
	var selected hostedMCPResource
	found := false
	for _, resource := range a.hostedMCPResourcesForSubject(ctx, subject) {
		if resource.URI == uri {
			selected = resource
			found = true
			break
		}
	}
	if !found {
		// A URI that matches no fixed resource may still match a template, which
		// is how a client reads one pipeline instead of the whole inventory.
		resource, payload, handled, err := a.hostedMCPTemplatedResource(ctx, subject, uri)
		if !handled {
			return hostedMCPResource{}, "", fmt.Errorf("resource %q is not available", uri)
		}
		if err != nil {
			return hostedMCPResource{}, "", err
		}
		raw, marshalErr := json.MarshalIndent(payload, "", "  ")
		if marshalErr != nil {
			return hostedMCPResource{}, "", marshalErr
		}
		return resource, string(raw), nil
	}

	var (
		payload map[string]any
		err     error
	)
	switch uri {
	case "nopsai://docs":
		payload, err = a.hostedMCPSearchDocs(ctx, map[string]any{"query": "", "limit": 20})
		if err != nil {
			payload = map[string]any{"docs": []any{}, "note": "Pass a query to nopsai.search_docs for targeted docs."}
			err = nil
		}
	case "nopsai://knowledge-contexts":
		payload, err = a.hostedMCPListKnowledgeContexts(ctx, subject, map[string]any{"limit": 100})
	case "nopsai://knowledge-connections":
		payload, err = a.hostedMCPListKnowledgeConnections(ctx, subject, map[string]any{"limit": 100})
	case "nopsai://pipelines":
		payload, err = a.hostedMCPListPipelines(ctx, map[string]any{"limit": 100})
	case "nopsai://pipeline-runs", "nopsai://lab":
		payload, err = a.hostedMCPListPipelineRuns(ctx, map[string]any{"limit": 50})
	case "nopsai://dashboards":
		payload, err = a.hostedMCPListDashboards(ctx, subject, map[string]any{"limit": 100})
	case "nopsai://triggers":
		payload, err = a.hostedMCPListTriggers(ctx, map[string]any{"limit": 100})
	case "nopsai://schedules":
		payload, err = a.hostedMCPListSchedules(ctx, map[string]any{"limit": 100})
	case "nopsai://scopes":
		payload, err = a.hostedMCPListScopes(ctx, map[string]any{"limit": 100})
	case "nopsai://statistics":
		payload, err = a.hostedMCPStatistics(ctx)
	case "nopsai://costs":
		payload, err = a.hostedMCPCostSummary(ctx)
	case "nopsai://system/models":
		payload, err = a.hostedMCPGetLLMProfiles(ctx)
	case "nopsai://system/mcp-profiles":
		payload, err = a.hostedMCPGetMCPProfiles(ctx)
	case "nopsai://features":
		payload, err = a.hostedMCPGetFeatureCapabilities(ctx, subject, map[string]any{"include_api_routes": true})
	case "nopsai://system/status":
		payload, err = a.hostedMCPGetSystemStatus(ctx)
	case "nopsai://system/dispatcher":
		payload, err = a.hostedMCPGetDispatcherStatus(ctx)
	default:
		err = fmt.Errorf("unsupported resource %q", uri)
	}
	if err != nil {
		return hostedMCPResource{}, "", err
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return hostedMCPResource{}, "", err
	}
	return selected, string(raw), nil
}

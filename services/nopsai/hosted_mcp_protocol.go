package nopsai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	aaamodel "nopsai/services/aaa/pkg/model"
)

func errNotAllowedResource(uri string) error {
	return fmt.Errorf("resource %q is not available", uri)
}

func errMissingResourceArgument(name string) error {
	return fmt.Errorf("resource template %q needs an identifier", name)
}

// hostedMCPServerInstructions is returned by initialize. A client shows it to its
// model, so it says what this server is for and the two rules that keep its use
// safe, rather than restating the tool list.
const hostedMCPServerInstructions = `NopsAI exposes one CI/CD and operations platform as tools, scoped to the calling user's permissions: a tool that is not listed is one this user may not call.

Prefer the analysis tools for judgement questions — nopsai.analyze_team, nopsai.analyze_pipeline, and nopsai.analyze_run return ranked findings with evidence and a recommended next call, instead of raw metrics to interpret.
Use nopsai.find_tools to search the catalogue when the tool you need is not in the list you were given.
Mutating tools require confirm:true and are refused without it; tools whose names begin with propose_ return a reviewable file plan and change nothing.`

// hostedMCPNegotiatedProtocolVersion answers with the version the client asked
// for when this server speaks it, and with its own when it does not. Replying
// with a constant, as this used to, tells a client on an older version that its
// request succeeded when the wire format may differ.
func hostedMCPNegotiatedProtocolVersion(params json.RawMessage) string {
	requested := hostedMCPRequestedProtocolVersion(params)
	if hostedMCPProtocolVersionSupported(requested) {
		return requested
	}
	return hostedMCPProtocolVersion
}

func hostedMCPRequestedProtocolVersion(params json.RawMessage) string {
	if len(params) == 0 {
		return ""
	}
	var decoded struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(params, &decoded); err != nil {
		return ""
	}
	return strings.TrimSpace(decoded.ProtocolVersion)
}

func hostedMCPProtocolVersionSupported(version string) bool {
	version = strings.TrimSpace(version)
	if version == "" {
		return false
	}
	for _, supported := range hostedMCPSupportedProtocolVersions {
		if supported == version {
			return true
		}
	}
	return false
}

func hostedMCPListCursor(params json.RawMessage) string {
	if len(params) == 0 {
		return ""
	}
	var decoded struct {
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal(params, &decoded); err != nil {
		return ""
	}
	return strings.TrimSpace(decoded.Cursor)
}

// Cursors carry the last name of the previous page rather than an index, so a
// catalogue that changes between pages cannot silently skip an entry.
func hostedMCPEncodeCursor(name string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(name))
}

func hostedMCPDecodeCursor(cursor string) string {
	if cursor == "" {
		return ""
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return ""
	}
	return string(decoded)
}

func hostedMCPPageTools(tools []hostedMCPTool, cursor string) ([]hostedMCPTool, string) {
	sorted := make([]hostedMCPTool, len(tools))
	copy(sorted, tools)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	after := hostedMCPDecodeCursor(cursor)
	page := make([]hostedMCPTool, 0, hostedMCPListPageSize)
	for _, tool := range sorted {
		if after != "" && tool.Name <= after {
			continue
		}
		if len(page) == hostedMCPListPageSize {
			return page, hostedMCPEncodeCursor(page[len(page)-1].Name)
		}
		page = append(page, tool)
	}
	return page, ""
}

func hostedMCPPageResources(resources []hostedMCPResource, cursor string) ([]hostedMCPResource, string) {
	sorted := make([]hostedMCPResource, len(resources))
	copy(sorted, resources)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].URI < sorted[j].URI })

	after := hostedMCPDecodeCursor(cursor)
	page := make([]hostedMCPResource, 0, hostedMCPListPageSize)
	for _, resource := range sorted {
		if after != "" && resource.URI <= after {
			continue
		}
		if len(page) == hostedMCPListPageSize {
			return page, hostedMCPEncodeCursor(page[len(page)-1].URI)
		}
		page = append(page, resource)
	}
	return page, ""
}

// Resource templates let a client read one thing by identifier rather than
// listing everything and filtering. Each template is gated by the same permission
// as the collection it reads from.
type hostedMCPResourceTemplate struct {
	URITemplate string              `json:"uriTemplate"`
	Name        string              `json:"name"`
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description,omitempty"`
	MimeType    string              `json:"mimeType,omitempty"`
	Permission  hostedMCPPermission `json:"-"`
}

func allHostedMCPResourceTemplates() []hostedMCPResourceTemplate {
	return []hostedMCPResourceTemplate{
		{
			URITemplate: "nopsai://pipelines/{pipeline_id}",
			Name:        "pipeline",
			Title:       "Pipeline definition",
			Description: "One pipeline's stored YAML and metadata, by path/name identifier.",
			MimeType:    "application/json",
			Permission:  hostedMCPReadPermission("pipeline.read", "pipeline", "*"),
		},
		{
			URITemplate: "nopsai://pipeline-runs/{run_id}",
			Name:        "pipeline-run",
			Title:       "Pipeline run",
			Description: "One run's status, timings, git context, and final outputs.",
			MimeType:    "application/json",
			Permission:  hostedMCPReadPermission("pipeline_run.read", "pipeline_run", "*"),
		},
		{
			URITemplate: "nopsai://schedules/{schedule_id}",
			Name:        "schedule",
			Title:       "Schedule",
			Description: "One pipeline schedule definition.",
			MimeType:    "application/json",
			Permission:  hostedMCPReadPermission("pipeline_schedule.read", "pipeline_schedule", "*"),
		},
		{
			URITemplate: "nopsai://triggers/{repository}",
			Name:        "trigger",
			Title:       "Repository trigger",
			Description: "One repository trigger definition.",
			MimeType:    "application/json",
			Permission:  hostedMCPReadPermission("trigger.read", "trigger", "*"),
		},
		{
			URITemplate: "nopsai://teams/{team}",
			Name:        "team",
			Title:       "Team",
			Description: "One team or application, by id or path.",
			MimeType:    "application/json",
			Permission:  hostedMCPReadPermission("team.read", "team", "*"),
		},
		{
			URITemplate: "nopsai://dashboards/{dashboard_id}",
			Name:        "dashboard",
			Title:       "Dashboard",
			Description: "One team dashboard and its current publications.",
			MimeType:    "application/json",
			Permission:  hostedMCPReadPermission("dashboard.read", "dashboard", "*"),
		},
		{
			URITemplate: "nopsai://knowledge-contexts/{knowledge_context_id}",
			Name:        "knowledge-context",
			Title:       "Knowledge context",
			Description: "One managed knowledge context document by id.",
			MimeType:    "application/json",
			Permission:  hostedMCPReadPermission("knowledge_context.read", "knowledge_context", "*"),
		},
		{
			URITemplate: "nopsai://analysis/{subject_type}/{subject_id}",
			Name:        "analysis",
			Title:       "Deterministic analysis",
			Description: "Ranked findings for a team, pipeline, or run: subject_type is team, pipeline, or run.",
			MimeType:    "application/json",
			Permission:  hostedMCPReadPermission("pipeline_run.list", "pipeline_run", "*"),
		},
	}
}

func (a *App) hostedMCPResourceTemplatesForSubject(ctx context.Context, subject aaamodel.Subject) []hostedMCPResourceTemplate {
	all := allHostedMCPResourceTemplates()
	templates := make([]hostedMCPResourceTemplate, 0, len(all))
	for _, template := range all {
		if a.hostedMCPAllowed(ctx, subject, template.Permission) {
			templates = append(templates, template)
		}
	}
	return templates
}

// hostedMCPTemplatedResource resolves a templated URI to the read that answers
// it. Each read goes through the same tool path as a tools/call would, so the
// concrete resource is authorized rather than the template.
func (a *App) hostedMCPTemplatedResource(ctx context.Context, subject aaamodel.Subject, uri string) (hostedMCPResource, map[string]any, bool, error) {
	for _, template := range allHostedMCPResourceTemplates() {
		prefix := strings.SplitN(template.URITemplate, "{", 2)[0]
		// The URI is claimed even with no identifier after the prefix, so the
		// caller is told what is missing instead of "no such resource".
		if !strings.HasPrefix(uri, prefix) {
			continue
		}
		remainder := strings.TrimPrefix(uri, prefix)
		if !a.hostedMCPAllowed(ctx, subject, template.Permission) {
			return hostedMCPResource{}, nil, true, errNotAllowedResource(uri)
		}
		resource := hostedMCPResource{
			URI:         uri,
			Name:        template.Name,
			Description: template.Description,
			MimeType:    template.MimeType,
			Permission:  template.Permission,
		}
		payload, err := a.hostedMCPReadTemplatedPayload(ctx, subject, template.Name, remainder)
		return resource, payload, true, err
	}
	return hostedMCPResource{}, nil, false, nil
}

func (a *App) hostedMCPReadTemplatedPayload(ctx context.Context, subject aaamodel.Subject, name, argument string) (map[string]any, error) {
	argument = strings.Trim(strings.TrimSpace(argument), "/")
	if argument == "" {
		return nil, errMissingResourceArgument(name)
	}
	switch name {
	case "pipeline":
		return a.callHostedMCPToolByName(ctx, subject, "nopsai.get_pipeline", map[string]any{"pipeline": argument})
	case "pipeline-run":
		return a.callHostedMCPToolByName(ctx, subject, "nopsai.get_pipeline_run", map[string]any{"run_id": argument})
	case "schedule":
		return a.callHostedMCPToolByName(ctx, subject, "nopsai.get_schedule", map[string]any{"schedule_id": argument})
	case "trigger":
		return a.callHostedMCPToolByName(ctx, subject, "nopsai.get_trigger", map[string]any{"repository": argument})
	case "team":
		return a.callHostedMCPToolByName(ctx, subject, "nopsai.get_team", map[string]any{"team": argument})
	case "dashboard":
		return a.callHostedMCPToolByName(ctx, subject, "nopsai.get_dashboard", map[string]any{"dashboard_id": argument})
	case "knowledge-context":
		return a.callHostedMCPToolByName(ctx, subject, "nopsai.get_knowledge_context", map[string]any{"id": argument})
	case "analysis":
		subjectType, subjectID, found := strings.Cut(argument, "/")
		if !found || strings.TrimSpace(subjectID) == "" {
			return nil, errMissingResourceArgument("analysis")
		}
		switch subjectType {
		case "team":
			return a.callHostedMCPToolByName(ctx, subject, "nopsai.analyze_team", map[string]any{"team": subjectID})
		case "pipeline":
			return a.callHostedMCPToolByName(ctx, subject, "nopsai.analyze_pipeline", map[string]any{"pipeline": subjectID})
		case "run":
			return a.callHostedMCPToolByName(ctx, subject, "nopsai.analyze_run", map[string]any{"run_id": subjectID})
		}
		return nil, errMissingResourceArgument("analysis subject type must be team, pipeline, or run")
	}
	return nil, errMissingResourceArgument(name)
}

// callHostedMCPToolByName runs a tool the way tools/call does, so a templated
// read is authorized and audited identically to the equivalent tool call.
func (a *App) callHostedMCPToolByName(ctx context.Context, subject aaamodel.Subject, name string, args map[string]any) (map[string]any, error) {
	tool, ok := a.hostedMCPToolByName(ctx, subject, name)
	if !ok {
		return nil, errNotAllowedResource(name)
	}
	return a.callHostedMCPTool(ctx, subject, hostedMCPResourceReaderUserID, tool, args, nil)
}

const hostedMCPResourceReaderUserID = "mcp:resource-read"

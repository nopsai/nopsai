package nopsai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/config"
	"nopsai/services/aaa/pkg/model"
)

func hostedMCPTestApp() *App {
	enabled := true
	return &App{
		cfg: &config.Config{Assistant: config.AssistantConfig{
			Enabled:  true,
			MCP:      config.AssistantMCPConfig{Enabled: &enabled},
			Features: config.AssistantFeaturesConfig{ActionExecution: &enabled},
		}},
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(context.Context, model.Subject, string, model.ResourceRef, map[string]any) (model.Decision, error) {
				return model.Decision{Allowed: true}, nil
			},
		},
	}
}

func hostedMCPTestCall(t *testing.T, app *App, method string, params map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	response := app.processHostedMCPRequest(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		hostedMCPRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: raw},
	)
	if response.Error != nil {
		t.Fatalf("%s returned error %d: %s", method, response.Error.Code, response.Error.Message)
	}
	encoded, err := json.Marshal(response.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return result
}

func TestHostedMCPInitializeNegotiatesTheProtocolVersion(t *testing.T) {
	app := hostedMCPTestApp()

	older := hostedMCPTestCall(t, app, "initialize", map[string]any{"protocolVersion": "2024-11-05"})
	if older["protocolVersion"] != "2024-11-05" {
		t.Fatalf("protocolVersion = %v, want the client's supported version echoed", older["protocolVersion"])
	}

	unknown := hostedMCPTestCall(t, app, "initialize", map[string]any{"protocolVersion": "1999-01-01"})
	if unknown["protocolVersion"] != hostedMCPProtocolVersion {
		t.Fatalf("protocolVersion = %v, want this server's own version for an unknown request", unknown["protocolVersion"])
	}
}

// Advertising listChanged on a request/response transport tells a client to wait
// for a notification that can never arrive.
func TestHostedMCPInitializeDoesNotAdvertiseUndeliverableCapabilities(t *testing.T) {
	result := hostedMCPTestCall(t, hostedMCPTestApp(), "initialize", map[string]any{})

	capabilities, _ := result["capabilities"].(map[string]any)
	tools, _ := capabilities["tools"].(map[string]any)
	resources, _ := capabilities["resources"].(map[string]any)
	if _, present := tools["listChanged"]; present {
		t.Fatalf("tools capability must not claim listChanged: %v", tools)
	}
	if _, present := resources["listChanged"]; present {
		t.Fatalf("resources capability must not claim listChanged: %v", resources)
	}
	if instructions, _ := result["instructions"].(string); !strings.Contains(instructions, "nopsai.analyze_team") {
		t.Fatalf("instructions should point a client at the analysis tools: %q", instructions)
	}
}

func TestHostedMCPPingIsAnswered(t *testing.T) {
	result := hostedMCPTestCall(t, hostedMCPTestApp(), "ping", map[string]any{})
	if len(result) != 0 {
		t.Fatalf("ping result = %v, want an empty object", result)
	}
}

func TestHostedMCPToolsListPaginatesWithoutSkippingOrRepeating(t *testing.T) {
	app := hostedMCPTestApp()
	seen := map[string]bool{}
	cursor := ""
	pages := 0

	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		result := hostedMCPTestCall(t, app, "tools/list", params)
		tools, _ := result["tools"].([]any)
		if len(tools) > hostedMCPListPageSize {
			t.Fatalf("page returned %d tools, want at most %d", len(tools), hostedMCPListPageSize)
		}
		for _, entry := range tools {
			tool, _ := entry.(map[string]any)
			name, _ := tool["name"].(string)
			if seen[name] {
				t.Fatalf("tool %q was returned on more than one page", name)
			}
			seen[name] = true
		}
		next, _ := result["nextCursor"].(string)
		cursor = next
		pages++
		if cursor == "" || pages > 10 {
			break
		}
	}

	if pages < 2 {
		t.Fatalf("expected the catalogue to span more than one page, got %d", pages)
	}
	expected := len(app.hostedMCPToolsForSubject(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"}))
	if len(seen) != expected {
		t.Fatalf("paginated tools = %d, want the full catalogue of %d", len(seen), expected)
	}
}

func TestHostedMCPToolsListPublishesAnnotationsAndSchemas(t *testing.T) {
	app := hostedMCPTestApp()
	described := map[string]map[string]any{}
	cursor := ""
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		result := hostedMCPTestCall(t, app, "tools/list", params)
		for _, entry := range result["tools"].([]any) {
			tool, _ := entry.(map[string]any)
			described[tool["name"].(string)] = tool
		}
		next, _ := result["nextCursor"].(string)
		if cursor = next; cursor == "" {
			break
		}
	}

	read := described["nopsai.get_pipeline"]
	readAnnotations, _ := read["annotations"].(map[string]any)
	if readAnnotations["readOnlyHint"] != true || readAnnotations["destructiveHint"] != false {
		t.Fatalf("read tool annotations = %v, want read-only and non-destructive", readAnnotations)
	}
	if read["title"] != "Get pipeline" {
		t.Fatalf("read tool title = %v, want a readable label", read["title"])
	}

	// The distinction a client needs before it runs something on the user's behalf.
	deletion := described["nopsai.delete_admin_user"]
	deleteAnnotations, _ := deletion["annotations"].(map[string]any)
	if deleteAnnotations["readOnlyHint"] != false || deleteAnnotations["destructiveHint"] != true {
		t.Fatalf("delete tool annotations = %v, want destructive and not read-only", deleteAnnotations)
	}

	// A proposal changes nothing, so it is read-only however alarming its name.
	proposal := described["nopsai.propose_pipeline_update"]
	proposalAnnotations, _ := proposal["annotations"].(map[string]any)
	if proposalAnnotations["readOnlyHint"] != true || proposalAnnotations["destructiveHint"] != false {
		t.Fatalf("proposal annotations = %v, want read-only", proposalAnnotations)
	}

	analysis := described["nopsai.analyze_team"]
	schema, _ := analysis["outputSchema"].(map[string]any)
	if schema == nil {
		t.Fatal("analysis tools should publish an output schema")
	}
	required, _ := schema["required"].([]any)
	if len(required) != 2 {
		t.Fatalf("output schema required = %v, want only the fields every path sets", required)
	}
}

func TestHostedMCPResourceTemplatesAreListedAndResolved(t *testing.T) {
	app := hostedMCPTestApp()

	result := hostedMCPTestCall(t, app, "resources/templates/list", map[string]any{})
	templates, _ := result["resourceTemplates"].([]any)
	if len(templates) == 0 {
		t.Fatal("expected resource templates")
	}
	found := false
	for _, entry := range templates {
		template, _ := entry.(map[string]any)
		if template["uriTemplate"] == "nopsai://pipelines/{pipeline_id}" {
			found = true
		}
	}
	if !found {
		t.Fatalf("pipeline template missing: %v", templates)
	}

	// A template URI with no identifier is a client error, and says so before any
	// tool runs.
	_, _, handled, err := app.hostedMCPTemplatedResource(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"}, "nopsai://analysis/")
	if !handled {
		t.Fatal("analysis template should claim its own URI space")
	}
	if err == nil || !strings.Contains(err.Error(), "identifier") {
		t.Fatalf("error = %v, want a missing-identifier error", err)
	}

	_, _, handled, err = app.hostedMCPTemplatedResource(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"}, "nopsai://analysis/nonsense/42")
	if !handled || err == nil || !strings.Contains(err.Error(), "team, pipeline, or run") {
		t.Fatalf("error = %v, want the accepted subject types named", err)
	}

	if _, _, handled, _ := app.hostedMCPTemplatedResource(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"}, "nopsai://unknown/1"); handled {
		t.Fatal("an unrelated URI must not be claimed by a template")
	}
}

func TestHostedMCPResourceTemplatesRespectPermissions(t *testing.T) {
	app := &App{
		cfg:      hostedMCPTestApp().cfg,
		aaaLocal: allowActionsForAssistantTest("pipeline.read"),
	}

	templates := app.hostedMCPResourceTemplatesForSubject(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"})
	if len(templates) != 1 || templates[0].Name != "pipeline" {
		t.Fatalf("templates = %+v, want only the one this subject may read", templates)
	}
}

func TestHostedMCPRejectsAnUnsupportedProtocolVersionHeader(t *testing.T) {
	app := hostedMCPTestApp()
	request := httptest.NewRequest(http.MethodPost, "/v1/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	request.Header.Set("MCP-Protocol-Version", "1999-01-01")
	request = request.WithContext(withAAASubject(request.Context(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"}))
	recorder := httptest.NewRecorder()

	app.handleHostedMCP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a version this server does not speak", recorder.Code)
	}
}

func TestHostedMCPToolTitlesReadAsLabels(t *testing.T) {
	cases := map[string]string{
		"nopsai.get_pipeline_run_logs":   "Get pipeline run logs",
		"nopsai.analyze_team":            "Analyze team",
		"nopsai.get_monitoring_ai_usage": "Get monitoring AI usage",
		"nopsai.call_api":                "Call API",
	}
	for name, want := range cases {
		if got := hostedMCPToolTitle(name); got != want {
			t.Fatalf("hostedMCPToolTitle(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestHostedMCPPromptsCoverTheFlowsAndFillTheirArguments(t *testing.T) {
	app := hostedMCPTestApp()

	list := hostedMCPTestCall(t, app, "prompts/list", map[string]any{})
	prompts, _ := list["prompts"].([]any)
	names := map[string]bool{}
	for _, entry := range prompts {
		prompt, _ := entry.(map[string]any)
		names[prompt["name"].(string)] = true
	}
	for _, want := range []string{"review-team", "explain-run-failure", "review-pipeline", "platform-spend", "prepare-gitops-change"} {
		if !names[want] {
			t.Fatalf("prompt %q missing from %v", want, names)
		}
	}

	got := hostedMCPTestCall(t, app, "prompts/get", map[string]any{
		"name":      "explain-run-failure",
		"arguments": map[string]any{"run_id": "run-9"},
	})
	messages, _ := got["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("messages = %v, want one user message", messages)
	}
	message, _ := messages[0].(map[string]any)
	content, _ := message["content"].(map[string]any)
	text, _ := content["text"].(string)
	if message["role"] != "user" {
		t.Fatalf("role = %v, want user", message["role"])
	}
	if !strings.Contains(text, "run-9") || !strings.Contains(text, "nopsai.analyze_run") {
		t.Fatalf("prompt text should carry the argument and name the tool:\n%s", text)
	}
}

// A prompt whose required argument is missing is refused with the argument named,
// rather than rendered with an empty subject.
func TestHostedMCPPromptGetRefusesMissingArguments(t *testing.T) {
	app := hostedMCPTestApp()
	response := app.processHostedMCPRequest(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		"user:viewer",
		hostedMCPRequest{JSONRPC: "2.0", ID: 1, Method: "prompts/get", Params: json.RawMessage(`{"name":"review-team"}`)},
	)

	if response.Error == nil || !strings.Contains(response.Error.Message, "team") {
		t.Fatalf("error = %+v, want the missing argument named", response.Error)
	}
}

// Offering a workflow the caller may not run is worse than not offering it.
func TestHostedMCPPromptsAreFilteredByPermission(t *testing.T) {
	app := &App{cfg: hostedMCPTestApp().cfg, aaaLocal: allowActionsForAssistantTest("pipeline_run.read")}

	prompts := app.hostedMCPPromptsForSubject(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"})
	names := make([]string, 0, len(prompts))
	for _, prompt := range prompts {
		names = append(names, prompt.Name)
	}
	if len(names) != 1 || names[0] != "explain-run-failure" {
		t.Fatalf("prompts = %v, want only the one this subject can run", names)
	}
}

func TestHostedMCPCompletionMapsArgumentsToTheRightInventory(t *testing.T) {
	cases := []struct {
		ref      hostedMCPCompletionRef
		argument string
		want     string
	}{
		{hostedMCPCompletionRef{Type: "ref/prompt", Name: "review-pipeline"}, "pipeline", "pipeline"},
		{hostedMCPCompletionRef{Type: "ref/prompt", Name: "explain-run-failure"}, "run_id", "run"},
		{hostedMCPCompletionRef{Type: "ref/prompt", Name: "review-team"}, "team", "team"},
		{hostedMCPCompletionRef{Type: "ref/resource", URI: "nopsai://pipelines/{pipeline_id}"}, "pipeline_id", "pipeline"},
		// The URI names the subject even when the argument name does not.
		{hostedMCPCompletionRef{Type: "ref/resource", URI: "nopsai://pipeline-runs/{id}"}, "id", "run"},
		{hostedMCPCompletionRef{Type: "ref/prompt", Name: "review-team"}, "days", "days"},
		{hostedMCPCompletionRef{Type: "ref/prompt", Name: "unknown"}, "nonsense", ""},
	}
	for _, testCase := range cases {
		if got := hostedMCPCompletionKind(testCase.ref, testCase.argument); got != testCase.want {
			t.Fatalf("kind(%v, %q) = %q, want %q", testCase.ref, testCase.argument, got, testCase.want)
		}
	}
}

func TestHostedMCPCompletionValuesRankPrefixesFirst(t *testing.T) {
	values, hasMore := hostedMCPCompletionValues(
		[]string{"platform/redeploy-api", "platform/deploy-api", "payments/deploy-web", "platform/deploy-api", ""},
		"platform/deploy",
		10,
	)

	if len(values) != 1 || values[0] != "platform/deploy-api" {
		t.Fatalf("values = %v, want the prefix match once", values)
	}
	if hasMore {
		t.Fatal("hasMore should be false when everything fit")
	}

	// A substring still matches, but only after the prefixes.
	mixed, _ := hostedMCPCompletionValues([]string{"platform/redeploy-api", "deploy-api"}, "deploy", 10)
	if len(mixed) != 2 || mixed[0] != "deploy-api" {
		t.Fatalf("values = %v, want the prefix match first", mixed)
	}

	bounded, more := hostedMCPCompletionValues([]string{"a1", "a2", "a3"}, "a", 2)
	if len(bounded) != 2 || !more {
		t.Fatalf("values = %v hasMore = %v, want a bounded page", bounded, more)
	}
}

func TestHostedMCPCompletionAnswersStaticArgumentsWithoutAnInventory(t *testing.T) {
	result := hostedMCPTestCall(t, hostedMCPTestApp(), "completion/complete", map[string]any{
		"ref":      map[string]any{"type": "ref/resource", "uri": "nopsai://analysis/{subject_type}/{subject_id}"},
		"argument": map[string]any{"name": "subject_type", "value": "t"},
	})

	completion, _ := result["completion"].(map[string]any)
	values, _ := completion["values"].([]any)
	if len(values) != 1 || values[0] != "team" {
		t.Fatalf("values = %v, want the matching subject type", values)
	}
}

// A completion is a convenience: with no readable inventory it offers nothing
// rather than failing the keystroke.
func TestHostedMCPCompletionDegradesToEmptyWithoutData(t *testing.T) {
	result := hostedMCPTestCall(t, hostedMCPTestApp(), "completion/complete", map[string]any{
		"ref":      map[string]any{"type": "ref/prompt", "name": "review-pipeline"},
		"argument": map[string]any{"name": "pipeline", "value": "dep"},
	})

	completion, _ := result["completion"].(map[string]any)
	values, _ := completion["values"].([]any)
	if len(values) != 0 || completion["hasMore"] != false {
		t.Fatalf("completion = %v, want an empty, non-failing result", completion)
	}
}

func TestHostedMCPTemplatesCoverDashboardsAndKnowledgeContexts(t *testing.T) {
	templates := map[string]bool{}
	for _, template := range allHostedMCPResourceTemplates() {
		templates[template.URITemplate] = true
	}
	for _, want := range []string{
		"nopsai://dashboards/{dashboard_id}",
		"nopsai://knowledge-contexts/{knowledge_context_id}",
	} {
		if !templates[want] {
			t.Fatalf("template %q missing", want)
		}
	}
}

// The collection URI keeps its own meaning: a template must not swallow it.
func TestHostedMCPCollectionURIsAreNotClaimedByTemplates(t *testing.T) {
	app := hostedMCPTestApp()
	for _, uri := range []string{"nopsai://pipelines", "nopsai://dashboards", "nopsai://knowledge-contexts"} {
		_, _, handled, _ := app.hostedMCPTemplatedResource(context.Background(), model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"}, uri)
		if handled {
			t.Fatalf("collection %q must be served as a fixed resource, not a template", uri)
		}
	}
}

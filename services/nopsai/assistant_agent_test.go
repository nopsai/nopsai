package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"nopsai/config"
	"nopsai/services/aaa/pkg/model"
)

// agentTestServer is a provider that answers with whatever the test scripted,
// recording each request so the test can check what the assistant sent.
type agentTestServer struct {
	*httptest.Server
	requests []map[string]any
}

func newAgentTestServer(t *testing.T, replies ...string) *agentTestServer {
	t.Helper()
	harness := &agentTestServer{}
	harness.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		harness.requests = append(harness.requests, payload)
		index := len(harness.requests) - 1
		if index >= len(replies) {
			index = len(replies) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, replies[index])
	}))
	t.Cleanup(harness.Close)
	return harness
}

func agentTestApp(server *agentTestServer, actions ...string) *App {
	return &App{
		cfg: &config.Config{
			// Action execution on, so a mutating tool reaches the confirmation
			// gate rather than being filtered out of the catalogue first.
			Assistant:         config.AssistantConfig{Features: config.AssistantFeaturesConfig{ActionExecution: boolConfigPtrForTest(true)}},
			LLMDefaultProfile: "standard",
			LLMProfiles: map[string]config.LLMProfile{
				"standard": {
					Provider:      config.LLMProviderOpenAI,
					Model:         "gpt-test",
					BaseURL:       server.URL + "/v1",
					CredentialRef: "credential://system/llm/standard",
				},
			},
		},
		httpClient:         server.Client(),
		credentialResolver: staticCredentialResolver{"credential://system/llm/standard": "secret"},
		aaaLocal:           allowActionsForAssistantTest(actions...),
	}
}

func agentToolCallReply(id, name, arguments string) string {
	return fmt.Sprintf(
		`{"choices":[{"finish_reason":"tool_calls","message":{"content":null,"tool_calls":[{"id":%q,"type":"function","function":{"name":%q,"arguments":%q}}]}}],"usage":{"prompt_tokens":9,"completion_tokens":3,"total_tokens":12}}`,
		id, name, arguments,
	)
}

func agentTextReply(text string) string {
	encoded, _ := json.Marshal(text)
	return fmt.Sprintf(`{"choices":[{"finish_reason":"stop","message":{"content":%s}}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`, encoded)
}

func runAgentTurn(app *App, content string) assistantOrchestrationResult {
	return app.runAssistantConversationTurn(
		context.Background(),
		model.Subject{Type: model.SubjectTypeUser, Sub: "viewer"},
		assistantConversation{ID: uuid.New(), DocsVersion: "auto", SelectedLLMProfile: "standard"},
		content,
		"standard",
	)
}

// The model asks for a tool, reads the result, and answers from it. Nothing in
// NopsAI decided which tool that would be.
func TestAgentRunsTheToolsTheModelAsksForAndAnswersFromThem(t *testing.T) {
	server := newAgentTestServer(t,
		agentToolCallReply("call_1", "nopsai.get_feature_capabilities", `{"query":"assistant"}`),
		agentTextReply("Feature coverage was read through hosted MCP. No changes were applied."),
	)
	app := agentTestApp(server, "system.read")

	result := runAgentTurn(app, "what assistant capabilities can I use?")

	if result.Reply != "Feature coverage was read through hosted MCP. No changes were applied." {
		t.Fatalf("reply = %q", result.Reply)
	}
	call := assistantFirstToolCall(result.ToolCalls, "nopsai.get_feature_capabilities")
	if call.Status != assistantToolStatusSuccess {
		t.Fatalf("the model's tool call should have run: %#v", call)
	}
	if len(server.requests) != 2 {
		t.Fatalf("expected a call turn and an answer turn, got %d", len(server.requests))
	}
	// The tool result must reach the model, or the answer rests on nothing.
	messages, _ := server.requests[1]["messages"].([]any)
	last, _ := messages[len(messages)-1].(map[string]any)
	if last["role"] != "tool" || last["tool_call_id"] != "call_1" {
		t.Fatalf("the tool result was not sent back: %#v", last)
	}
}

// A turn starts with the common reads plus the door to the rest. The working set
// does not depend on what was asked — the same list is offered every turn — so
// nothing here decides which tools a question deserves.
func TestAgentOffersTheCoreWorkingSetAndTheDiscoveryTool(t *testing.T) {
	server := newAgentTestServer(t, agentTextReply("Nothing to do."))
	app := agentTestApp(server, "system.read", "pipeline.read", "pipeline.list", "pipeline_run.read", "pipeline_run.list", "pipeline_run.read_logs")

	runAgentTurn(app, "check current pipeline")

	names := offeredToolNames(t, server.requests[0])
	for _, want := range []string{"nopsai.find_tools", "nopsai.get_pipeline", "nopsai.get_pipeline_run_logs"} {
		if !names[want] {
			t.Fatalf("%s should be in the working set: %v", want, names)
		}
	}
	// The full catalogue is ~23k tokens of schema, paid again on every step of
	// the loop. The working set has to stay small enough to be worth sending.
	if len(names) > 20 {
		t.Fatalf("working set has grown to %d tools; it is meant to be the common reads", len(names))
	}
}

// Anything outside the working set is one model-made call away, and what it finds
// becomes callable rather than merely mentioned.
func TestAgentUnlocksToolsTheModelDiscovers(t *testing.T) {
	server := newAgentTestServer(t,
		agentToolCallReply("call_1", "nopsai.find_tools", `{"query":"list triggers"}`),
		agentTextReply("Found the trigger tools."),
	)
	app := agentTestApp(server, "system.read", "trigger.list", "trigger.read")

	result := runAgentTurn(app, "what triggers exist?")

	find := assistantFirstToolCall(result.ToolCalls, "nopsai.find_tools")
	if find.Status != assistantToolStatusSuccess {
		t.Fatalf("find_tools should have run: %#v", find)
	}
	discovered := assistantUnlockedToolNames(find)
	if len(discovered) == 0 {
		t.Fatal("find_tools returned no tools to unlock")
	}
	// The second turn must offer what the first turn discovered.
	offered := offeredToolNames(t, server.requests[1])
	unlockedSomething := false
	for _, name := range discovered {
		if offered[name] {
			unlockedSomething = true
			break
		}
	}
	if !unlockedSomething {
		t.Fatalf("discovered tools were never offered back: found %v, offered %v", discovered, offered)
	}
}

func offeredToolNames(t *testing.T, request map[string]any) map[string]bool {
	t.Helper()
	tools, _ := request["tools"].([]any)
	names := map[string]bool{}
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		function, _ := tool["function"].(map[string]any)
		names[fmt.Sprint(function["name"])] = true
	}
	return names
}

// The confirmation gate moved from plan validation onto the call itself, so it
// still has to refuse an unconfirmed mutation.
func TestAgentRefusesAnUnconfirmedMutation(t *testing.T) {
	server := newAgentTestServer(t,
		agentToolCallReply("call_1", "nopsai.write_variable_value", `{"scope":"default","name":"API_URL","value":"https://example.test"}`),
		agentTextReply("I can set that variable once you confirm. No changes were applied."),
	)
	app := agentTestApp(server, "system.read", "variable.write_value")

	result := runAgentTurn(app, "set API_URL in the default scope")

	call := assistantFirstToolCall(result.ToolCalls, "nopsai.write_variable_value")
	if call.Status != assistantToolStatusDenied {
		t.Fatalf("an unconfirmed mutation must be refused: %#v", call)
	}
	if reason := assistantOutputString(call.Output, "error"); !strings.Contains(reason, "without confirm:true") {
		t.Fatalf("refusal should name the missing confirmation: %q", reason)
	}
}

// A tool the subject cannot see is refused rather than attempted: failing to
// evaluate the guard is not the same as passing it.
func TestAgentRefusesAToolItCannotSee(t *testing.T) {
	server := newAgentTestServer(t,
		agentToolCallReply("call_1", "nopsai.invented_tool", `{}`),
		agentTextReply("That tool does not exist here."),
	)
	app := agentTestApp(server, "system.read")

	result := runAgentTurn(app, "do something impossible")

	call := assistantFirstToolCall(result.ToolCalls, "nopsai.invented_tool")
	if call.Status == assistantToolStatusSuccess {
		t.Fatalf("an unavailable tool must not run: %#v", call)
	}
}

// The answer-quality checks used to run during synthesis. The agent writes the
// answer itself, so they run on the agent's answer, and a false claim of an
// applied change earns one corrective turn.
func TestAgentCorrectsAnAnswerThatClaimsAnUnappliedChange(t *testing.T) {
	server := newAgentTestServer(t,
		agentTextReply("I applied the change to the pipeline."),
		agentTextReply("This is a proposal only. No changes were applied. Review it, then confirm."),
	)
	app := agentTestApp(server, "system.read")

	result := runAgentTurn(app, "update the pipeline")

	if strings.Contains(strings.ToLower(result.Reply), "i applied the change") {
		t.Fatalf("a false applied claim must not reach the user: %q", result.Reply)
	}
	if len(server.requests) != 2 {
		t.Fatalf("expected one corrective turn, got %d requests", len(server.requests))
	}
	correction, _ := server.requests[1]["messages"].([]any)
	last, _ := correction[len(correction)-1].(map[string]any)
	if !strings.Contains(fmt.Sprint(last["content"]), "no tool applied one") {
		t.Fatalf("the correction should say what was wrong: %#v", last)
	}
}

// A provider failure produces no answer at all rather than an invented one.
func TestAgentFailsClosedWhenTheProviderFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream exploded", http.StatusInternalServerError)
	}))
	defer server.Close()
	harness := &agentTestServer{Server: server}
	app := agentTestApp(harness, "system.read")

	result := runAgentTurn(app, "why did the last run fail?")

	if strings.Contains(strings.ToLower(result.Reply), "the last run failed because") {
		t.Fatalf("a failed provider must not produce an invented answer: %q", result.Reply)
	}
	failure := assistantFirstToolCall(result.ToolCalls, assistantLLMToolName)
	if failure.Status != assistantToolStatusError {
		t.Fatalf("the provider failure should be recorded: %#v", failure)
	}
}

// The execution plan the UI renders is built from the calls that ran, so it can
// only describe what actually happened.
func TestAgentExecutionPlanReportsTheEvidenceItRead(t *testing.T) {
	server := newAgentTestServer(t,
		agentToolCallReply("call_1", "nopsai.get_feature_capabilities", `{"query":"assistant"}`),
		agentTextReply("Read the capability catalogue. No changes were applied."),
	)
	app := agentTestApp(server, "system.read")

	result := runAgentTurn(app, "what can you do?")

	planCall := assistantFirstToolCall(result.ToolCalls, assistantExecutionPlanToolName)
	executionPlan, ok := planCall.Output["execution_plan"].(assistantExecutionPlan)
	if !ok {
		t.Fatalf("execution plan output = %#v", planCall.Output)
	}
	if len(executionPlan.Steps) != 1 || executionPlan.Steps[0].Tool != "nopsai.get_feature_capabilities" {
		t.Fatalf("the plan should list the evidence that was read: %#v", executionPlan.Steps)
	}
	if executionPlan.Steps[0].Status != assistantToolStatusSuccess {
		t.Fatalf("a reported step carries the status it finished with: %#v", executionPlan.Steps[0])
	}
}

// Policy, not routing: the system prompt carries the scope and evidence rules
// and no rule mapping a question type to a tool.
func TestAgentSystemPromptCarriesPolicyAndContext(t *testing.T) {
	prompt := assistantAgentSystemPrompt(
		assistantConversation{ID: uuid.New(), DocsVersion: "auto"},
		assistantBaseTurnPlanWithPageContext(
			"why did this fail",
			assistantConversationMemory{},
			assistantPageContext{ResourceType: "pipeline", ResourceID: "nopsai/nopsai-platform-release"},
		),
	)

	for _, want := range []string{
		"inside the NopsAI world",
		"out of scope",
		"read them rather than reporting that the run failed",
		"nopsai/nopsai-platform-release",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt should carry %q", want)
		}
	}
	for _, unwanted := range []string{"schema_tools", "call nopsai.analyze_team", "Return JSON only"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("system prompt should no longer carry %q", unwanted)
		}
	}
}

func boolConfigPtrForTest(value bool) *bool {
	return &value
}

// A slow turn has to explain itself: the panel needs to say whether the wait was
// the provider thinking or a tool working, without anyone reaching for a profiler.
func TestAgentReportsWhereTheTurnSpentItsTime(t *testing.T) {
	server := newAgentTestServer(t,
		agentToolCallReply("call_1", "nopsai.get_feature_capabilities", `{"query":"assistant"}`),
		agentTextReply("Read the catalogue. No changes were applied."),
	)
	app := agentTestApp(server, "system.read")

	result := runAgentTurn(app, "what can you do?")

	planCall := assistantFirstToolCall(result.ToolCalls, assistantExecutionPlanToolName)
	executionPlan, ok := planCall.Output["execution_plan"].(assistantExecutionPlan)
	if !ok {
		t.Fatalf("execution plan output = %#v", planCall.Output)
	}
	if executionPlan.Timing.ModelTurns != 2 {
		t.Fatalf("two model turns were taken, timing says %d", executionPlan.Timing.ModelTurns)
	}
	if executionPlan.Timing.ToolCalls != 1 {
		t.Fatalf("one tool ran, timing says %d", executionPlan.Timing.ToolCalls)
	}
	// Durations are wall time and can round to zero on a local server, so the
	// test pins that they are recorded and non-negative rather than a magnitude.
	if executionPlan.Timing.ModelMS < 0 || executionPlan.Timing.ToolMS < 0 {
		t.Fatalf("timing must not be negative: %#v", executionPlan.Timing)
	}
	for _, call := range assistantEvidenceToolCalls(result.ToolCalls) {
		if call.DurationMS < 0 {
			t.Fatalf("%s carries a negative duration", call.Name)
		}
	}
	if len(executionPlan.Steps) != 1 || executionPlan.Steps[0].Tool != "nopsai.get_feature_capabilities" {
		t.Fatalf("the step should name the evidence it read: %#v", executionPlan.Steps)
	}
}

// The label a user watches while waiting has to come from the turn, and it has
// to work for a tool nobody has registered in a table anywhere.
func TestToolProgressLabelReadsAnyToolName(t *testing.T) {
	for tool, want := range map[string]string{
		"nopsai.get_pipeline_run_logs":   "Reading pipeline run logs",
		"nopsai.list_pipelines":          "Listing pipelines",
		"nopsai.analyze_run":             "Analysing run",
		"nopsai.find_tools":              "Looking for tools",
		"nopsai.validate_pipeline":       "Validating pipeline",
		"nopsai.propose_pipeline_update": "Preparing a proposal for pipeline update",
		"nopsai.write_variable_value":    "Writing variable value",
		"nopsai.some_future_tool":        "Running some future tool",
	} {
		if got := assistantToolProgressLabel(tool); got != want {
			t.Fatalf("assistantToolProgressLabel(%q) = %q, want %q", tool, got, want)
		}
	}
}

package nopsai

import (
	"testing"
)

func TestHostedMCPToolRoutingDerivesDomainAndCapability(t *testing.T) {
	cases := []struct {
		tool       hostedMCPTool
		domain     string
		capability string
	}{
		{
			tool:       toolDef("nopsai.get_monitoring_step_performance", "Read step performance analytics.", "pipeline_run.list", "pipeline_run", "*", objectSchema(map[string]any{})),
			domain:     "monitoring",
			capability: hostedMCPCapabilityRead,
		},
		{
			tool:       toolDef("nopsai.get_pipeline_run_logs", "Read run logs.", "pipeline_run.read_logs", "pipeline_run", "*", objectSchema(map[string]any{})),
			domain:     "pipeline_run",
			capability: hostedMCPCapabilityRead,
		},
		{
			tool:       toolDef("nopsai.propose_schedule_update", "Return a GitOps-ready schedule update file plan.", "pipeline_schedule.update", "pipeline_schedule", "*", objectSchema(map[string]any{})),
			domain:     "pipeline_schedule",
			capability: hostedMCPCapabilityProposal,
		},
		{
			tool:       toolDef("nopsai.write_secret_value", "Write a scoped secret value. Requires confirm:true.", "secret.write_value", "secret", "*", objectSchema(map[string]any{"confirm": booleanSchema()})),
			domain:     "secret",
			capability: hostedMCPCapabilityMutation,
		},
		{
			tool:       toolDef("nopsai.eject_runner", "Remove a runner. Requires confirm:true.", "system.update", "dispatcher", "*", objectSchema(map[string]any{"confirm": booleanSchema()})),
			domain:     "dispatcher",
			capability: hostedMCPCapabilityMutation,
		},
	}

	for _, testCase := range cases {
		routing := hostedMCPToolRoutingFor(testCase.tool)
		if routing.Domain != testCase.domain {
			t.Fatalf("%s domain = %q, want %q", testCase.tool.Name, routing.Domain, testCase.domain)
		}
		if routing.Capability != testCase.capability {
			t.Fatalf("%s capability = %q, want %q", testCase.tool.Name, routing.Capability, testCase.capability)
		}
	}
}

// The point of deriving routing: a tool registered today is reachable today,
// without anyone remembering to add a branch for it in the planner.

func TestHostedMCPRoutingModePolicyExcludesTheWrongChangeMode(t *testing.T) {
	tools := []hostedMCPTool{
		toolDef("nopsai.write_variable_value", "Write a scoped variable value. Requires confirm:true.", "variable.write_value", "variable", "*", objectSchema(map[string]any{"confirm": booleanSchema()})),
		toolDef("nopsai.propose_variable_gitops_write", "Return a GitOps-ready variable file plan.", "variable.write_value", "variable", "*", objectSchema(map[string]any{})),
		toolDef("nopsai.list_variables_metadata", "List variable metadata without values.", "variable.list_metadata", "variable", "*", objectSchema(map[string]any{})),
	}

	direct := hostedMCPRoutingScores("set TEST_VAR = 1 in scope prod", tools, assistantPlannerModePolicy{AllowMutation: true})
	if _, present := direct["nopsai.propose_variable_gitops_write"]; present {
		t.Fatalf("a direct write must not be offered a GitOps proposal: %#v", direct)
	}
	if _, present := direct["nopsai.write_variable_value"]; !present {
		t.Fatalf("a direct write should reach the runtime write tool: %#v", direct)
	}

	gitops := hostedMCPRoutingScores("propose a gitops change for variable TEST_VAR", tools, assistantPlannerModePolicy{AllowProposal: true})
	if _, present := gitops["nopsai.write_variable_value"]; present {
		t.Fatalf("a GitOps request must not be offered a runtime write: %#v", gitops)
	}

	read := hostedMCPRoutingScores("which variables exist in prod?", tools, assistantPlannerModePolicy{})
	if _, present := read["nopsai.write_variable_value"]; present {
		t.Fatalf("a read must not be offered a write tool: %#v", read)
	}
	if _, present := read["nopsai.list_variables_metadata"]; !present {
		t.Fatalf("a read should reach the metadata list: %#v", read)
	}
}

func TestHostedMCPRoutingKeepsAVagueRequestLean(t *testing.T) {
	scores := hostedMCPTopRoutingScores(
		hostedMCPRoutingScores("Help me plan a rollout", allHostedMCPTools(), assistantPlannerModePolicy{}),
		assistantPlannerMaxRoutedTools,
	)

	if len(scores) > assistantPlannerMaxRoutedTools {
		t.Fatalf("vague request routed %d tools, want at most %d: %#v", len(scores), assistantPlannerMaxRoutedTools, scores)
	}
}

func TestHostedMCPRoutingSeparatesTokenSpendFromCredentialTokens(t *testing.T) {
	scores := hostedMCPTopRoutingScores(
		hostedMCPRoutingScores("how much token did assistant chat use today", allHostedMCPTools(), assistantPlannerModePolicy{}),
		assistantPlannerMaxRoutedTools,
	)

	if _, present := scores["nopsai.get_monitoring_ai_usage"]; !present {
		t.Fatalf("token spend should route to AI usage monitoring: %#v", scores)
	}
	for name := range scores {
		if name == "nopsai.list_credentials_metadata" || name == "nopsai.create_credential" {
			t.Fatalf("a token spend question must not route to credential tools: %#v", scores)
		}
	}
}

func TestHostedMCPFindToolsSearchesTheSubjectsOwnCatalogue(t *testing.T) {
	app := &App{}
	tools := []hostedMCPTool{
		toolDef("nopsai.list_schedules", "List pipeline schedules.", "pipeline_schedule.list", "pipeline_schedule", "*", objectSchema(map[string]any{"limit": numberSchema()})),
		toolDef("nopsai.get_schedule", "Read a schedule definition.", "pipeline_schedule.read", "pipeline_schedule", "*", objectSchema(map[string]any{"schedule_id": stringSchema()})),
		toolDef("nopsai.get_dashboard", "Read a dashboard.", "dashboard.read", "dashboard", "*", objectSchema(map[string]any{"dashboard_id": stringSchema()})),
	}

	result := app.hostedMCPFindTools(tools, map[string]any{"query": "cron schedule", "limit": float64(5)})

	items, _ := result["tools"].([]map[string]any)
	if len(items) == 0 {
		t.Fatalf("find_tools returned nothing: %#v", result)
	}
	if items[0]["name"] != "nopsai.list_schedules" && items[0]["name"] != "nopsai.get_schedule" {
		t.Fatalf("first match = %v, want a schedule tool", items[0]["name"])
	}
	// The schema is the point: it is what stops the planner guessing arguments.
	if _, hasSchema := items[0]["input_schema"]; !hasSchema {
		t.Fatalf("find_tools must return input schemas: %#v", items[0])
	}
	for _, item := range items {
		if item["name"] == "nopsai.get_dashboard" {
			t.Fatalf("unrelated domain leaked into the results: %#v", items)
		}
	}
}

func TestHostedMCPRoutingSingularFoldsPluralsOnly(t *testing.T) {
	cases := map[string]string{
		"pipelines":    "pipeline",
		"policies":     "policy",
		"runs":         "run",
		"status":       "status",
		"access":       "access",
		"credentials":  "credential",
		"monitoring":   "monitoring",
		"dispatchers":  "dispatcher",
		"is":           "is",
		"analysis":     "analysis",
		"addresses":    "address",
		"performances": "performance",
	}
	for input, want := range cases {
		if got := hostedMCPRoutingSingular(input); got != want {
			t.Fatalf("hostedMCPRoutingSingular(%q) = %q, want %q", input, got, want)
		}
	}
}

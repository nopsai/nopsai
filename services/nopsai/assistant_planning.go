package nopsai

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"nopsai/config"
	aaamodel "nopsai/services/aaa/pkg/model"
)

const (
	assistantMaxPlanToolCalls = 8
	assistantMaxPlanArgKeys   = 40
)

type assistantPlanStep struct {
	Thought             string
	ToolName            string
	Args                map[string]any
	StopWhenUsageFound  bool
	StopWhenToolBlocked bool
}

type assistantAnswerQuality struct {
	HasDirectAnswer      bool
	UsedRelevantTools    bool
	EmptyResultExplained bool
	SuggestedNextStep    bool
	NoFakeData           bool
}

type assistantToolRunner func(name string, args map[string]any) assistantToolActivity

var (
	assistantUsageProfileAfterPattern   = regexp.MustCompile(`(?i)\b(?:llm\s+profile|profile)\s+([a-zA-Z0-9][a-zA-Z0-9._:/-]{0,80})`)
	assistantUsageProfileBeforePattern  = regexp.MustCompile(`(?i)\b([a-zA-Z0-9][a-zA-Z0-9._:/-]{1,80})\s+(?:llm\s+profile|profile)\b`)
	assistantUsageModelAfterPattern     = regexp.MustCompile(`(?i)\bmodel\s+([a-zA-Z0-9][a-zA-Z0-9._:/-]{0,80})`)
	assistantUsageModelBeforePattern    = regexp.MustCompile(`(?i)\b([a-zA-Z0-9][a-zA-Z0-9._:/-]{1,80})\s+model\b`)
	assistantUsageProviderAfterPattern  = regexp.MustCompile(`(?i)\bprovider\s+([a-zA-Z0-9][a-zA-Z0-9._:/-]{0,80})`)
	assistantUsageProviderBeforePattern = regexp.MustCompile(`(?i)\b([a-zA-Z0-9][a-zA-Z0-9._:/-]{1,80})\s+provider\b`)
	assistantUsageFeaturePattern        = regexp.MustCompile(`(?i)\bfeature\s+([a-zA-Z0-9][a-zA-Z0-9._:/-]{0,80})`)
	assistantUsageStepPattern           = regexp.MustCompile(`(?i)\bstep\s+([a-zA-Z0-9][a-zA-Z0-9._:/-]{0,80})`)
	assistantUsageTaskPattern           = regexp.MustCompile(`(?i)\btask\s+([a-zA-Z0-9][a-zA-Z0-9._:/-]{0,80})`)
)

func assistantFinalizeTurnPlan(plan assistantTurnPlan, content string) assistantTurnPlan {
	plan.Goal = strings.TrimSpace(content)
	switch plan.Intent {
	case "validate_pipeline":
		plan.Steps = []assistantPlanStep{{
			Thought:  "Validate the supplied pipeline YAML without saving it.",
			ToolName: "nopsai.validate_pipeline",
			Args:     map[string]any{"yaml": plan.YAML},
		}}
		plan.SuccessCriteria = "Return validation status and schema/semantic errors without applying changes."
	case "analyze_run":
		plan.Steps = []assistantPlanStep{
			{Thought: "Read run status and metadata.", ToolName: "nopsai.get_pipeline_run", Args: map[string]any{"run_id": plan.RunID}},
			{Thought: "Read bounded recent logs for evidence.", ToolName: "nopsai.get_pipeline_run_logs", Args: map[string]any{"run_id": plan.RunID, "limit": 160}},
			{Thought: "Analyze the run failure from run and log evidence.", ToolName: "nopsai.analyze_pipeline_run_failure", Args: map[string]any{"run_id": plan.RunID}},
		}
		plan.SuccessCriteria = "Explain the most likely failure using run status and bounded logs."
	case "list_runs":
		plan.Steps = []assistantPlanStep{{
			Thought:  "List recent visible pipeline runs.",
			ToolName: "nopsai.list_pipeline_runs",
			Args:     map[string]any{"limit": 5},
		}}
		plan.SuccessCriteria = "Return recent visible runs or explain empty results."
	case "variable_usage":
		plan.Steps = []assistantPlanStep{{
			Thought:  "Analyze variable metadata for repeated names without reading values.",
			ToolName: "nopsai.analyze_variable_usage",
			Args:     map[string]any{"scope": plan.Scope, "limit": 20},
		}}
		plan.SuccessCriteria = "Summarize visible duplicate variable names without exposing values."
	case "ai_token_usage":
		plan.AIUsageFilters = assistantAIUsageFiltersFromMessage(content)
		plan.Steps = []assistantPlanStep{{
			Thought:             "Check AI usage in the default monitoring window.",
			ToolName:            "nopsai.get_monitoring_ai_usage",
			Args:                cloneAssistantArgs(plan.AIUsageFilters),
			StopWhenUsageFound:  true,
			StopWhenToolBlocked: true,
		}}
		plan.SuccessCriteria = "Return token totals, top pipelines/runs, and explain zero-event results with broader-window evidence."
	case "scope_secret_summary":
		plan.Steps = []assistantPlanStep{
			{Thought: "List visible scopes.", ToolName: "nopsai.list_scopes", Args: map[string]any{"limit": 200}},
			{Thought: "List secret counts by scope using metadata only.", ToolName: "nopsai.list_secret_scopes", Args: map[string]any{}},
		}
		plan.SuccessCriteria = "Count visible scopes and metadata-only secrets per scope without reading plaintext values."
	case "cost":
		plan.Steps = []assistantPlanStep{
			{Thought: "Read cost summary.", ToolName: "nopsai.get_cost_summary", Args: map[string]any{}},
			{Thought: "Read cost improvement suggestions.", ToolName: "nopsai.suggest_cost_improvements", Args: map[string]any{}},
		}
		plan.SuccessCriteria = "Summarize cost signals and safe recommendations without applying changes."
	case "design":
		plan.Steps = []assistantPlanStep{{
			Thought:  "Read design improvement suggestions from visible pipeline inventory.",
			ToolName: "nopsai.suggest_design_improvements",
			Args:     map[string]any{},
		}}
		plan.SuccessCriteria = "Return design recommendations without applying changes."
	case "statistics":
		plan.Steps = []assistantPlanStep{{
			Thought:  "Read platform statistics.",
			ToolName: "nopsai.get_statistics",
			Args:     map[string]any{},
		}}
		plan.SuccessCriteria = "Return visible platform counts."
	case "generate_pipeline":
		plan.SuccessCriteria = "Return a GitOps-safe generated YAML proposal with assumptions, required variables, and required secrets."
	case "trigger":
		if plan.Repository != "" && containsAny(plan.LowerContent, "change", "update", "modify", "propose") {
			plan.Steps = []assistantPlanStep{{Thought: "Draft a trigger change without applying it.", ToolName: "nopsai.propose_trigger_change", Args: map[string]any{"repository": plan.Repository, "change": content}}}
			plan.SuccessCriteria = "Return a trigger proposal only."
		} else if plan.Repository != "" {
			plan.Steps = []assistantPlanStep{{Thought: "Read the requested trigger definition.", ToolName: "nopsai.get_trigger", Args: map[string]any{"repository": plan.Repository}}}
			plan.SuccessCriteria = "Return trigger details visible to the user."
		} else {
			plan.Steps = []assistantPlanStep{{Thought: "List visible trigger definitions.", ToolName: "nopsai.list_triggers", Args: map[string]any{"limit": 20}}}
			plan.SuccessCriteria = "Return visible trigger inventory."
		}
	case "schedule":
		if plan.ScheduleID != "" && containsAny(plan.LowerContent, "change", "update", "modify", "propose") {
			plan.Steps = []assistantPlanStep{{Thought: "Draft a schedule change without applying it.", ToolName: "nopsai.propose_schedule_change", Args: map[string]any{"schedule_id": plan.ScheduleID, "change": content}}}
			plan.SuccessCriteria = "Return a schedule proposal only."
		} else if plan.ScheduleID != "" {
			plan.Steps = []assistantPlanStep{{Thought: "Read the requested schedule definition.", ToolName: "nopsai.get_schedule", Args: map[string]any{"schedule_id": plan.ScheduleID}}}
			plan.SuccessCriteria = "Return schedule details visible to the user."
		} else {
			plan.Steps = []assistantPlanStep{{Thought: "List visible schedules.", ToolName: "nopsai.list_schedules", Args: map[string]any{"limit": 20}}}
			plan.SuccessCriteria = "Return visible schedule inventory."
		}
	case "scope":
		if plan.Scope != "" {
			plan.Steps = []assistantPlanStep{{Thought: "Explain scope permissions and usage.", ToolName: "nopsai.explain_scope_permissions", Args: map[string]any{"scope": plan.Scope}}}
			plan.SuccessCriteria = "Return scope permission context visible to the user."
		} else {
			plan.Steps = []assistantPlanStep{{Thought: "List visible scopes.", ToolName: "nopsai.list_scopes", Args: map[string]any{"limit": 20}}}
			plan.SuccessCriteria = "Return visible scope inventory."
		}
	case "search_pipelines":
		query := plan.SearchQuery
		if query == "" {
			query = content
		}
		plan.Steps = []assistantPlanStep{{Thought: "Search visible pipeline metadata and readable YAML.", ToolName: "nopsai.search_pipelines", Args: map[string]any{"query": query, "limit": 20, "include_snippets": true}}}
		plan.SuccessCriteria = "Return matching visible pipelines and evidence snippets."
	case "pipeline_knowledge_context":
		args := map[string]any{"query": content, "limit": 20}
		toolName := "nopsai.list_knowledge_contexts"
		thought := "Search managed knowledge context."
		if plan.YAML != "" {
			toolName = "nopsai.get_pipeline_knowledge_context"
			args = map[string]any{"yaml": plan.YAML, "include_content": true}
			thought = "Resolve knowledge context references from supplied pipeline YAML."
		} else if plan.PipelineID != "" {
			toolName = "nopsai.get_pipeline_knowledge_context"
			args = map[string]any{"pipeline": plan.PipelineID, "include_content": true}
			thought = "Resolve knowledge context references from the stored pipeline."
		}
		plan.Steps = []assistantPlanStep{{Thought: thought, ToolName: toolName, Args: args}}
		plan.SuccessCriteria = "Return managed and unresolved knowledge context references."
	case "knowledge_context":
		plan.Steps = []assistantPlanStep{{Thought: "Search managed knowledge context.", ToolName: "nopsai.list_knowledge_contexts", Args: map[string]any{"query": content, "limit": 20}}}
		plan.SuccessCriteria = "Return visible matching managed knowledge context."
	case "pipeline":
		if plan.PipelineID != "" {
			plan.Steps = []assistantPlanStep{{Thought: "Read the requested pipeline definition.", ToolName: "nopsai.get_pipeline", Args: map[string]any{"pipeline": plan.PipelineID}}}
			plan.SuccessCriteria = "Return the visible pipeline definition."
		} else {
			plan.Steps = []assistantPlanStep{{Thought: "List visible pipelines.", ToolName: "nopsai.list_pipelines", Args: map[string]any{"limit": 20}}}
			plan.SuccessCriteria = "Return visible pipeline inventory."
		}
	case "profiles":
		plan.Steps = []assistantPlanStep{
			{Thought: "List LLM profiles visible to the assistant.", ToolName: "nopsai.get_llm_profiles", Args: map[string]any{}},
			{Thought: "List MCP profiles visible to the assistant.", ToolName: "nopsai.get_mcp_profiles", Args: map[string]any{}},
		}
		plan.SuccessCriteria = "Return visible LLM and MCP profile names."
	case "feature_capabilities":
		args := map[string]any{"query": plan.SearchQuery, "include_api_routes": false}
		if plan.CapabilityArea != "" {
			args["area"] = plan.CapabilityArea
		}
		plan.Steps = []assistantPlanStep{{Thought: "Read current-user MCP feature coverage and policy notes.", ToolName: "nopsai.get_feature_capabilities", Args: args}}
		plan.SuccessCriteria = "Return current-user feature capabilities and blocked areas."
	case "runtime":
		plan.Steps = []assistantPlanStep{{Thought: "Read dispatcher and runner health.", ToolName: "nopsai.get_dispatcher_status", Args: map[string]any{}}}
		plan.SuccessCriteria = "Return dispatcher/runner health and capacity signals."
	case "system":
		plan.Steps = []assistantPlanStep{{Thought: "Read system status.", ToolName: "nopsai.get_system_status", Args: map[string]any{}}}
		plan.SuccessCriteria = "Return visible system status."
	case "docs":
		plan.Steps = []assistantPlanStep{{Thought: "Search NopsAI docs and knowledge context.", ToolName: "nopsai.search_docs", Args: map[string]any{"query": content, "limit": 5}}}
		plan.SuccessCriteria = "Return relevant visible documentation or explain no matches."
	}
	return plan
}

func (a *App) validateAssistantToolPlan(ctx context.Context, subject aaamodel.Subject, plan assistantTurnPlan) error {
	if len(plan.Steps) == 0 {
		return nil
	}
	if len(plan.Steps) > assistantMaxPlanToolCalls {
		return fmt.Errorf("assistant plan has %d tool calls; max allowed is %d", len(plan.Steps), assistantMaxPlanToolCalls)
	}
	for idx, step := range plan.Steps {
		toolName := strings.TrimSpace(step.ToolName)
		if toolName == "" {
			return fmt.Errorf("assistant plan step %d has no tool", idx+1)
		}
		if len(step.Args) > assistantMaxPlanArgKeys {
			return fmt.Errorf("assistant plan step %d has too many arguments", idx+1)
		}
		tool, ok := a.hostedMCPToolByName(ctx, subject, toolName)
		if !ok {
			return fmt.Errorf("assistant plan step %d requested unavailable tool %q", idx+1, toolName)
		}
		if assistantToolRequiresActionExecution(tool) &&
			configuredAssistantRequiresConfirm(a) &&
			!assistantPlannedToolIsProposal(tool.Name) &&
			!boolArg(step.Args, "confirm", false) {
			return fmt.Errorf("assistant plan step %d requested mutating tool %q without confirm:true", idx+1, toolName)
		}
	}
	return nil
}

func configuredAssistantRequiresConfirm(a *App) bool {
	return a == nil || config.AssistantRequireConfirmation(a.assistantConfig().Actions)
}

func assistantPlannedToolIsProposal(name string) bool {
	name = strings.TrimSpace(name)
	return strings.HasPrefix(name, "nopsai.propose_") ||
		strings.HasPrefix(name, "nopsai.plan_") ||
		strings.HasPrefix(name, "nopsai.preview_") ||
		name == "nopsai.generate_pipeline" ||
		name == "nopsai.validate_pipeline"
}

func assistantAssessAnswerQuality(plan assistantTurnPlan, toolCalls []assistantToolActivity, reply string) assistantAnswerQuality {
	reply = strings.TrimSpace(reply)
	lower := strings.ToLower(reply)
	quality := assistantAnswerQuality{
		HasDirectAnswer:      reply != "",
		UsedRelevantTools:    plan.Intent == "clarify" || len(toolCalls) > 0,
		EmptyResultExplained: true,
		SuggestedNextStep:    true,
		NoFakeData:           !assistantReplyClaimsApplied(lower) || assistantAnyToolApplied(toolCalls),
	}
	if plan.Intent == "ai_token_usage" && !assistantAnyAIUsageCallHasEvents(assistantToolCallsByName(toolCalls, "nopsai.get_monitoring_ai_usage")) {
		quality.EmptyResultExplained = containsAny(lower, "no visible ai usage", "no visible usage", "no visible token", "0 tokens", "zero")
	}
	if assistantPlanNeedsProposalSafetyLanguage(plan) {
		quality.SuggestedNextStep = containsAny(lower, "no changes were applied", "review", "commit", "gitops", "confirm", "proposal")
	}
	return quality
}

func assistantAnswerQualityPasses(quality assistantAnswerQuality) bool {
	return quality.HasDirectAnswer &&
		quality.UsedRelevantTools &&
		quality.EmptyResultExplained &&
		quality.SuggestedNextStep &&
		quality.NoFakeData
}

func assistantPlanNeedsProposalSafetyLanguage(plan assistantTurnPlan) bool {
	switch plan.Intent {
	case "generate_pipeline", "propose_pipeline_create", "propose_pipeline_update":
		return true
	case "feature_tool":
		for _, step := range plan.Steps {
			if assistantFeatureToolNeedsSafetyLanguage(step.ToolName) {
				return true
			}
		}
	case "trigger", "schedule":
		for _, step := range plan.Steps {
			if strings.HasPrefix(strings.TrimSpace(step.ToolName), "nopsai.propose_") {
				return true
			}
		}
	}
	return false
}

func assistantFeatureToolNeedsSafetyLanguage(name string) bool {
	name = strings.TrimSpace(name)
	if assistantPlannedToolIsProposal(name) {
		return true
	}
	withoutPrefix := strings.TrimPrefix(name, "nopsai.")
	for _, prefix := range []string{
		"acknowledge_",
		"activate_",
		"approve_",
		"bootstrap_",
		"cancel_",
		"create_",
		"delete_",
		"disable_",
		"enable_",
		"encrypt_",
		"invoke_",
		"reject_",
		"resolve_",
		"rerun_",
		"rotate_",
		"run_",
		"sync_",
		"test_",
		"update_",
		"write_",
	} {
		if strings.HasPrefix(withoutPrefix, prefix) {
			return true
		}
	}
	return false
}

func assistantReplyClaimsApplied(lowerReply string) bool {
	lowerReply = strings.ToLower(strings.TrimSpace(lowerReply))
	if lowerReply == "" ||
		containsAny(lowerReply, "no changes were applied", "nothing was applied", "not applied", "was not applied", "were not applied") {
		return false
	}
	return containsAny(
		lowerReply,
		"changes were applied",
		"change was applied",
		"i applied",
		"i updated",
		"i deleted",
		"i created",
		"i rotated",
		"i cancelled",
		"i reran",
		"has been applied",
		"have been applied",
	)
}

func assistantAnyToolApplied(toolCalls []assistantToolActivity) bool {
	for _, call := range toolCalls {
		if assistantOutputBool(call.Output, "applied") || assistantOutputBool(call.Output, "applies") {
			return true
		}
	}
	return false
}

func (a *App) runAIUsageInvestigation(plan assistantTurnPlan, runTool assistantToolRunner) {
	now := time.Now().UTC()
	windows := assistantAIUsageInvestigationWindows(plan, now)
	foundUsage := false
	var lastArgs map[string]any
	for _, step := range windows {
		call := runTool(step.ToolName, step.Args)
		lastArgs = cloneAssistantArgs(step.Args)
		if step.StopWhenToolBlocked && call.Status != assistantToolStatusSuccess {
			return
		}
		if step.StopWhenUsageFound && assistantAIUsageCallHasEvents(call) {
			foundUsage = true
			break
		}
	}
	if lastArgs == nil {
		lastArgs = cloneAssistantArgs(plan.AIUsageFilters)
	}
	runTool("nopsai.get_monitoring_efficiency", cloneAssistantArgs(lastArgs))
	runTool("nopsai.get_monitoring_summary", cloneAssistantArgs(lastArgs))
	if foundUsage {
		return
	}
	runTool("nopsai.list_pipeline_runs", map[string]any{"limit": 12})
	runTool("nopsai.get_llm_profiles", map[string]any{})
}

func assistantAIUsageInvestigationWindows(plan assistantTurnPlan, now time.Time) []assistantPlanStep {
	baseArgs := assistantAIUsageBaseArgs(plan, now)
	return []assistantPlanStep{
		{
			Thought:             "Check AI usage in the default monitoring window.",
			ToolName:            "nopsai.get_monitoring_ai_usage",
			Args:                cloneAssistantArgs(baseArgs),
			StopWhenUsageFound:  true,
			StopWhenToolBlocked: true,
		},
		{
			Thought:             "Retry across the last 90 days if the default window is empty.",
			ToolName:            "nopsai.get_monitoring_ai_usage",
			Args:                assistantArgsWithWindow(baseArgs, now.AddDate(0, -3, 0), time.Time{}),
			StopWhenUsageFound:  true,
			StopWhenToolBlocked: true,
		},
		{
			Thought:             "Retry across the last year before concluding no visible usage exists.",
			ToolName:            "nopsai.get_monitoring_ai_usage",
			Args:                assistantArgsWithWindow(baseArgs, now.AddDate(-1, 0, 0), time.Time{}),
			StopWhenUsageFound:  true,
			StopWhenToolBlocked: true,
		},
	}
}

func assistantAIUsageBaseArgs(plan assistantTurnPlan, now time.Time) map[string]any {
	args := cloneAssistantArgs(plan.AIUsageFilters)
	lower := plan.LowerContent
	switch {
	case containsAny(lower, "last week", "past week"):
		args["from"] = now.AddDate(0, 0, -7).Format(time.RFC3339)
	case containsAny(lower, "last month", "past month"):
		args["from"] = now.AddDate(0, -1, 0).Format(time.RFC3339)
	case containsAny(lower, "last quarter", "past quarter"):
		args["from"] = now.AddDate(0, -3, 0).Format(time.RFC3339)
	}
	return args
}

func assistantArgsWithWindow(base map[string]any, from, to time.Time) map[string]any {
	args := cloneAssistantArgs(base)
	if !from.IsZero() {
		args["from"] = from.UTC().Format(time.RFC3339)
	}
	if !to.IsZero() {
		args["to"] = to.UTC().Format(time.RFC3339)
	}
	return args
}

func assistantAIUsageFiltersFromMessage(content string) map[string]any {
	filters := map[string]any{}
	if value := assistantFirstUsagePatternGroup(assistantUsageProfileAfterPattern, content); value != "" {
		filters["llm_profile"] = value
	} else if value := assistantFirstUsagePatternGroup(assistantUsageProfileBeforePattern, content); value != "" {
		filters["llm_profile"] = value
	}
	if value := assistantFirstUsagePatternGroup(assistantUsageModelAfterPattern, content); value != "" {
		filters["model"] = value
	} else if value := assistantFirstUsagePatternGroup(assistantUsageModelBeforePattern, content); value != "" {
		filters["model"] = value
	}
	if value := assistantFirstUsagePatternGroup(assistantUsageProviderAfterPattern, content); value != "" {
		filters["provider"] = value
	} else if value := assistantFirstUsagePatternGroup(assistantUsageProviderBeforePattern, content); value != "" {
		filters["provider"] = value
	}
	if value := assistantFirstUsagePatternGroup(assistantUsageFeaturePattern, content); value != "" {
		filters["feature"] = value
	}
	if value := assistantFirstUsagePatternGroup(assistantUsageStepPattern, content); value != "" {
		filters["step_name"] = value
	}
	if value := assistantFirstUsagePatternGroup(assistantUsageTaskPattern, content); value != "" {
		filters["task_name"] = value
	}
	if len(filters) == 0 {
		return nil
	}
	return filters
}

func assistantFirstUsagePatternGroup(pattern *regexp.Regexp, content string) string {
	value := assistantFirstPatternGroup(pattern, content)
	value = strings.Trim(value, ".,;:!?\"'`")
	if assistantUsageFilterCandidateIsGrammar(value) {
		return ""
	}
	return value
}

func assistantUsageFilterCandidateIsGrammar(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "for", "the", "a", "an", "usage", "usages", "token", "tokens", "llm", "ai", "which", "what", "last", "week", "month", "quarter", "use", "used", "uses", "using", "most", "highest":
		return true
	default:
		return false
	}
}

func cloneAssistantArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(args))
	for key, value := range args {
		cloned[key] = value
	}
	return cloned
}

func assistantAIUsageCallHasEvents(call assistantToolActivity) bool {
	if call.Status != assistantToolStatusSuccess {
		return false
	}
	if assistantOutputFloat(call.Output, "total_tokens") > 0 {
		return true
	}
	if assistantOutputFloat(call.Output, "exact_token_events")+assistantOutputFloat(call.Output, "estimated_token_events") > 0 {
		return true
	}
	return len(assistantMapSlice(call.Output["by_pipeline"])) > 0 || len(assistantMapSlice(call.Output["top_token_runs"])) > 0
}

func assistantAIUsageFilterSummary(args map[string]any) string {
	labels := []string{}
	for _, key := range []string{"provider", "model", "llm_profile", "feature", "step_name", "task_name"} {
		if value := strings.TrimSpace(fmt.Sprint(args[key])); value != "" && value != "<nil>" {
			labels = append(labels, strings.ReplaceAll(key, "_", " ")+"="+value)
		}
	}
	if len(labels) == 0 {
		return ""
	}
	return strings.Join(labels, ", ")
}

func assistantToolCallsByName(toolCalls []assistantToolActivity, name string) []assistantToolActivity {
	matches := []assistantToolActivity{}
	for _, call := range toolCalls {
		if call.Name == name {
			matches = append(matches, call)
		}
	}
	return matches
}

func assistantBestAIUsageCall(calls []assistantToolActivity) assistantToolActivity {
	if len(calls) == 0 {
		return assistantToolActivity{Name: "nopsai.get_monitoring_ai_usage", Output: map[string]any{}}
	}
	for _, call := range calls {
		if assistantAIUsageCallHasEvents(call) {
			return call
		}
	}
	return calls[len(calls)-1]
}

func assistantAnyAIUsageCallHasEvents(calls []assistantToolActivity) bool {
	for _, call := range calls {
		if assistantAIUsageCallHasEvents(call) {
			return true
		}
	}
	return false
}

func assistantAIUsageWindowLabel(call assistantToolActivity, idx int) string {
	from := strings.TrimSpace(fmt.Sprint(call.Input["from"]))
	to := strings.TrimSpace(fmt.Sprint(call.Input["to"]))
	if from == "" || from == "<nil>" {
		return "default monitoring window"
	}
	if to == "" || to == "<nil>" {
		return "window from " + from
	}
	return "window from " + from + " to " + to
}

func assistantAppendTokenGroups(lines []string, call assistantToolActivity) []string {
	lines = assistantAppendTokenGroup(lines, "Highest token steps:", call.Output["by_step"], 5)
	lines = assistantAppendTokenGroup(lines, "Highest token tasks:", call.Output["by_task"], 5)
	lines = assistantAppendTokenGroup(lines, "Usage by model:", call.Output["by_model"], 5)
	lines = assistantAppendTokenGroup(lines, "Usage by LLM profile:", call.Output["by_profile"], 5)
	lines = assistantAppendTokenGroup(lines, "Usage by feature:", call.Output["by_feature"], 5)
	return lines
}

func assistantAppendTokenGroup(lines []string, title string, value any, limit int) []string {
	items := assistantMapSlice(value)
	if len(items) == 0 {
		return lines
	}
	lines = append(lines, "", title)
	for idx, item := range items {
		if idx >= limit {
			break
		}
		label := firstNonEmptyString(assistantOutputString(item, "label"), assistantOutputString(item, "key"))
		if label == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %.0f tokens across %.0f events", label, assistantOutputFloat(item, "tokens"), assistantOutputFloat(item, "count")))
	}
	return lines
}

func assistantVisibleRunCount(toolCalls []assistantToolActivity) float64 {
	for _, call := range toolCalls {
		switch call.Name {
		case "nopsai.get_monitoring_summary":
			if call.Status == assistantToolStatusSuccess {
				if count := assistantOutputFloat(call.Output, "total_runs"); count > 0 {
					return count
				}
			}
		case "nopsai.list_pipeline_runs":
			if call.Status == assistantToolStatusSuccess {
				return float64(len(assistantMapSlice(call.Output["runs"])))
			}
		}
	}
	return 0
}

package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"nopsai/config"
	aaamodel "nopsai/services/aaa/pkg/model"
)

const (
	assistantMaxPlanToolCalls      = 8
	assistantMaxPlanArgKeys        = 40
	assistantMaxPlanArgDepth       = 8
	assistantMaxPlanArgStringBytes = 256 * 1024
	assistantMaxPlanArgListItems   = 200
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

func (a *App) validateAssistantToolPlan(ctx context.Context, subject aaamodel.Subject, plan assistantTurnPlan) error {
	if len(plan.Steps) == 0 {
		return nil
	}
	if err := assistantValidateToolPlanMatchesRequest(plan); err != nil {
		return err
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
		if err := assistantValidatePlanArgs(step.Args, 0); err != nil {
			return fmt.Errorf("assistant plan step %d has unsafe arguments: %w", idx+1, err)
		}
		tool, ok := a.hostedMCPToolByName(ctx, subject, toolName)
		if !ok {
			return fmt.Errorf("assistant plan step %d requested unavailable tool %q", idx+1, toolName)
		}
		userConfirmed := plan.UserConfirmed || assistantFeatureConfirmed(plan.LowerContent)
		if assistantToolRequiresActionExecution(tool) &&
			boolArg(step.Args, "confirm", false) &&
			!assistantPlannedToolIsProposal(tool.Name) &&
			!userConfirmed {
			return fmt.Errorf("assistant plan step %d requested mutating tool %q with confirm:true but the user did not explicitly confirm", idx+1, toolName)
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

func assistantValidatePlanArgs(value any, depth int) error {
	if depth > assistantMaxPlanArgDepth {
		return fmt.Errorf("max depth %d exceeded", assistantMaxPlanArgDepth)
	}
	switch typed := value.(type) {
	case nil, bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return nil
	case string:
		if len(typed) > assistantMaxPlanArgStringBytes {
			return fmt.Errorf("string argument exceeds %d bytes", assistantMaxPlanArgStringBytes)
		}
		return nil
	case map[string]any:
		if len(typed) > assistantMaxPlanArgKeys {
			return fmt.Errorf("object has %d keys; max allowed is %d", len(typed), assistantMaxPlanArgKeys)
		}
		for key, item := range typed {
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("blank argument key")
			}
			if err := assistantValidatePlanArgs(item, depth+1); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
		}
		return nil
	case []any:
		if len(typed) > assistantMaxPlanArgListItems {
			return fmt.Errorf("array has %d items; max allowed is %d", len(typed), assistantMaxPlanArgListItems)
		}
		for idx, item := range typed {
			if err := assistantValidatePlanArgs(item, depth+1); err != nil {
				return fmt.Errorf("[%d]: %w", idx, err)
			}
		}
		return nil
	case []string:
		if len(typed) > assistantMaxPlanArgListItems {
			return fmt.Errorf("array has %d items; max allowed is %d", len(typed), assistantMaxPlanArgListItems)
		}
		for idx, item := range typed {
			if err := assistantValidatePlanArgs(item, depth+1); err != nil {
				return fmt.Errorf("[%d]: %w", idx, err)
			}
		}
		return nil
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Errorf("value is not JSON-serializable")
		}
		if len(encoded) > assistantMaxPlanArgStringBytes {
			return fmt.Errorf("encoded value exceeds %d bytes", assistantMaxPlanArgStringBytes)
		}
		var decoded any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			return fmt.Errorf("value is not valid JSON")
		}
		return assistantValidatePlanArgs(decoded, depth+1)
	}
}

func configuredAssistantRequiresConfirm(a *App) bool {
	return a == nil || config.AssistantRequireConfirmation(a.assistantConfig().Actions)
}

func assistantFeatureConfirmed(lower string) bool {
	return containsAny(lower, "confirm", "confirmed", "with confirmation", "i approve", "approved to execute", "apply it", "execute it")
}

func assistantPlannerRequestRequirements(plan assistantTurnPlan) map[string]any {
	requirements := map[string]any{
		"ai_token_usage_evidence_required": assistantRequestRequiresAIUsageEvidence(plan.LowerContent),
	}
	if assistantRequestRequiresAIUsageEvidence(plan.LowerContent) {
		requirements["required_tool"] = "nopsai.get_monitoring_ai_usage"
		if plan.RunID != "" {
			requirements["required_run_filter"] = map[string]any{
				"run_id": plan.RunID,
				"args":   []string{"run_id", "runId"},
			}
		}
	}
	return requirements
}

func assistantValidateToolPlanMatchesRequest(plan assistantTurnPlan) error {
	if !assistantRequestRequiresAIUsageEvidence(plan.LowerContent) {
		return nil
	}
	if !assistantPlanIncludesTool(plan, "nopsai.get_monitoring_ai_usage") {
		return fmt.Errorf("assistant plan must use nopsai.get_monitoring_ai_usage for token usage requests; pipeline run status, log, and failure-analysis tools do not report token counts")
	}
	if plan.RunID != "" && !assistantPlanIncludesAIUsageRunFilter(plan, plan.RunID) {
		return fmt.Errorf("assistant plan must filter nopsai.get_monitoring_ai_usage by run_id %q for this token usage request", plan.RunID)
	}
	return nil
}

func assistantValidatePlannerFinalAnswer(plan assistantTurnPlan, toolCalls []assistantToolActivity) error {
	if !assistantRequestRequiresAIUsageEvidence(plan.LowerContent) {
		return nil
	}
	if !assistantToolCallsIncludeSuccessfulAIUsage(toolCalls, plan.RunID) {
		return fmt.Errorf("assistant planner cannot answer a token usage request without successful nopsai.get_monitoring_ai_usage evidence")
	}
	return nil
}

func assistantRequestRequiresAIUsageEvidence(lower string) bool {
	lower = strings.ToLower(strings.TrimSpace(lower))
	if lower == "" {
		return false
	}
	if containsAny(lower, "access token", "personal token", "refresh token", "bearer token", "bootstrap token", "jwt", "oauth token", "api token") {
		return false
	}
	if containsAny(lower, "llm usage", "ai usage", "model usage", "llm token", "llm tokens", "ai token", "ai tokens") {
		return true
	}
	if !containsAny(lower, "token", "tokens") {
		return false
	}
	return containsAny(lower, "usage", "used", "use", "uses", "how many", "pipelinerun", "pipeline run", "run ", " run", "pipeline", "schedule", "model", "llm", "ai")
}

func assistantPlanIncludesTool(plan assistantTurnPlan, toolName string) bool {
	for _, step := range plan.Steps {
		if strings.TrimSpace(step.ToolName) == toolName {
			return true
		}
	}
	return false
}

func assistantPlanIncludesAIUsageRunFilter(plan assistantTurnPlan, runID string) bool {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return true
	}
	for _, step := range plan.Steps {
		if strings.TrimSpace(step.ToolName) != "nopsai.get_monitoring_ai_usage" {
			continue
		}
		if assistantPlanArgString(step.Args, "run_id", "runId") == runID {
			return true
		}
	}
	return false
}

func assistantToolCallsIncludeSuccessfulAIUsage(toolCalls []assistantToolActivity, runID string) bool {
	runID = strings.TrimSpace(runID)
	for _, call := range toolCalls {
		if call.Name != "nopsai.get_monitoring_ai_usage" || call.Status != assistantToolStatusSuccess {
			continue
		}
		if runID == "" || assistantPlanArgString(call.Input, "run_id", "runId") == runID {
			return true
		}
	}
	return false
}

func assistantPlanArgString(args map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := args[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		default:
			rendered := strings.TrimSpace(fmt.Sprint(typed))
			if rendered != "" {
				return rendered
			}
		}
	}
	return ""
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
		UsedRelevantTools:    plan.Intent == "clarify" || plan.FinalAnswer != "" || len(assistantEvidenceToolCalls(toolCalls)) > 0 || assistantHasPlanDenial(toolCalls),
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

func assistantHasPlanDenial(toolCalls []assistantToolActivity) bool {
	_, ok := assistantFirstPlanDenial(toolCalls)
	return ok
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

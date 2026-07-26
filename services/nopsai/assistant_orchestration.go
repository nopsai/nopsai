package nopsai

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"

	"nopsai/config"
	"nopsai/services/aaa/pkg/model"
)

const (
	assistantToolStatusSuccess = "success"
	assistantToolStatusError   = "error"
	assistantToolStatusDenied  = "denied"
)

var (
	assistantUUIDPattern           = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	assistantPipelineNamePattern   = regexp.MustCompile(`(?i)(?:called|named|name)\s+([a-zA-Z0-9][a-zA-Z0-9._-]{0,62})`)
	assistantScopePattern          = regexp.MustCompile(`(?i)\bscope\s+([a-zA-Z0-9][a-zA-Z0-9._/-]{0,126})`)
	assistantRepositoryPattern     = regexp.MustCompile(`(?i)\b(?:repository|repo)\s+([a-zA-Z0-9][a-zA-Z0-9._/-]{0,126})`)
	assistantScheduleIDPattern     = regexp.MustCompile(`(?i)\b(?:schedule|schedule_id)\s+([0-9a-fA-F-]{8,64})`)
	assistantPipelineIDPattern     = regexp.MustCompile(`(?i)\b(?:pipeline|pipeline_id)\s+([a-zA-Z0-9][a-zA-Z0-9._/-]{0,126})`)
	assistantPipelineSearchPattern = regexp.MustCompile(`(?i)\b(?:search|find|look(?:\s+for)?|look\s+through)\s+pipelines?(?:\s+(?:for|matching|with))?\s+(.+)`)
	assistantAPICallPattern        = regexp.MustCompile(`(?i)\b(GET|POST|PUT|PATCH|DELETE)\s+((?:/v1/)[^\s` + "`" + `]+)`)
)

type assistantOrchestrationResult struct {
	Reply     string
	ToolCalls []assistantToolActivity
	Memory    assistantConversationMemory
}

func (a *App) runAssistantConversationTurn(
	ctx context.Context,
	subject model.Subject,
	userID string,
	conversation assistantConversation,
	content string,
	selectedProfile string,
) assistantOrchestrationResult {
	return a.runAssistantConversationTurnWithPageContext(ctx, subject, userID, conversation, content, selectedProfile, assistantPageContext{})
}

func (a *App) runAssistantConversationTurnWithPageContext(
	ctx context.Context,
	subject model.Subject,
	userID string,
	conversation assistantConversation,
	content string,
	selectedProfile string,
	pageContext assistantPageContext,
) assistantOrchestrationResult {
	pageContext = normalizeAssistantPageContext(pageContext)
	if result, ok := a.handleAssistantPendingConfirmation(ctx, subject, userID, conversation, content); ok {
		return result
	}
	if result, ok := a.handleAssistantDirectValueConfirmation(ctx, subject, conversation, content); ok {
		return result
	}

	planned := a.runAssistantLLMPlannedTurnWithPageContext(ctx, subject, userID, conversation, content, selectedProfile, pageContext)
	if planned.Handled {
		memory := assistantMemoryForTurn(conversation, planned.Plan)
		memory = assistantMemoryAfterTools(memory, planned.Plan, planned.ToolCalls)
		if pending, ok := a.assistantPendingConfirmationFromDeniedPlan(ctx, subject, planned.Plan, planned.ToolCalls); ok {
			memory = assistantSetPendingConfirmation(memory, pending)
			memory.Summary = "Waiting for explicit confirmation before applying a direct MCP change."
			return assistantOrchestrationResult{
				Reply:     assistantPendingConfirmationPrompt(pending),
				ToolCalls: planned.ToolCalls,
				Memory:    normalizeAssistantMemory(memory),
			}
		}
		reply := composeAssistantReply(planned.Plan, selectedProfile, planned.ToolCalls)
		if assistantTurnReplyIsCompleteWithoutSynthesis(planned) {
			return assistantOrchestrationResult{
				Reply:     reply,
				ToolCalls: planned.ToolCalls,
				Memory:    normalizeAssistantMemory(memory),
			}
		}
		synthesis := a.synthesizeAssistantReplyWithLLM(ctx, conversation, content, selectedProfile, planned.Plan, planned.ToolCalls, reply)
		if synthesis.Activity != nil {
			planned.ToolCalls = append(planned.ToolCalls, *synthesis.Activity)
			reply = synthesis.Reply
		}
		return assistantOrchestrationResult{
			Reply:     reply,
			ToolCalls: planned.ToolCalls,
			Memory:    normalizeAssistantMemory(memory),
		}
	}

	plan := planned.Plan
	if strings.TrimSpace(plan.Goal) == "" {
		plan = assistantBaseTurnPlanWithPageContext(content, conversation.Memory, pageContext)
	}
	memory := assistantMemoryForTurn(conversation, plan)
	reply := a.assistantPlannerFailureReply(ctx, subject, content, plan, planned.ToolCalls)
	return assistantOrchestrationResult{
		Reply:     reply,
		ToolCalls: planned.ToolCalls,
		Memory:    normalizeAssistantMemory(memory),
	}
}

func assistantTurnReplyIsCompleteWithoutSynthesis(planned assistantPlannerResult) bool {
	return planned.Plan.Intent == "clarify" || planned.Plan.FinalAnswer != "" || planned.SkipSynthesis
}

type assistantTurnPlan struct {
	Intent          string
	Goal            string
	LowerContent    string
	PageContext     assistantPageContext
	RunID           string
	YAML            string
	PipelineName    string
	PipelineID      string
	Repository      string
	ScheduleID      string
	Scope           string
	ClarifyQuestion string
	APIMethod       string
	APIPath         string
	Steps           []assistantPlanStep
	SuccessCriteria string
	UserConfirmed   bool
	FinalAnswer     string
}

func (a *App) runAssistantHostedMCPTool(ctx context.Context, subject model.Subject, userID string, conversationID uuid.UUID, name string, args map[string]any) assistantToolActivity {
	if args == nil {
		args = map[string]any{}
	}
	args = hostedMCPMonitoringAnalyticsArgs(name, args)
	if !config.AssistantMCPEnabled(a.assistantConfig().MCP) {
		return assistantToolActivity{
			Name:       name,
			Input:      args,
			Output:     map[string]any{"error": "hosted MCP is disabled by assistant configuration"},
			Status:     assistantToolStatusDenied,
			Source:     "mcp",
			Phase:      "evidence",
			Confidence: "low",
			Purpose:    "Run a hosted MCP tool with current-user authorization.",
		}
	}
	result, err := a.callAssistantHostedMCPTool(ctx, subject, userID, conversationID, name, args)
	status := assistantToolStatusSuccess
	if err != nil {
		status = assistantToolStatusError
		if strings.Contains(strings.ToLower(err.Error()), "not allowed") {
			status = assistantToolStatusDenied
		}
		if result == nil {
			result = map[string]any{"error": err.Error()}
		} else if _, ok := result["error"]; !ok {
			result["error"] = err.Error()
		}
	}
	confidence := "high"
	if status != assistantToolStatusSuccess {
		confidence = "low"
	}
	return assistantToolActivity{
		Name:         name,
		Input:        args,
		Output:       result,
		Status:       status,
		ResourceURIs: assistantResourceURIsForTool(name),
		Source:       "mcp",
		Phase:        "evidence",
		Confidence:   confidence,
		Purpose:      "Run a hosted MCP tool with current-user authorization.",
	}
}

func assistantPlanActivityInput(plan assistantTurnPlan) map[string]any {
	steps := make([]map[string]any, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		steps = append(steps, map[string]any{
			"tool":   strings.TrimSpace(step.ToolName),
			"reason": strings.TrimSpace(step.Thought),
		})
	}
	return map[string]any{
		"goal":             strings.TrimSpace(plan.Goal),
		"intent":           strings.TrimSpace(plan.Intent),
		"success_criteria": strings.TrimSpace(plan.SuccessCriteria),
		"steps":            steps,
	}
}

func assistantMemoryForTurn(conversation assistantConversation, plan assistantTurnPlan) assistantConversationMemory {
	memory := normalizeAssistantMemory(conversation.Memory)
	memory.ConversationID = conversation.ID
	if memory.SelectedDocsVersion == "" {
		memory.SelectedDocsVersion = strings.TrimSpace(conversation.DocsVersion)
	}
	if memory.SelectedScope == "" {
		memory.SelectedScope = strings.Trim(strings.TrimSpace(conversation.Scope), "/")
	}
	if plan.RunID != "" {
		memory.SelectedRun = plan.RunID
	}
	if plan.PipelineID != "" {
		memory.SelectedPipeline = strings.Trim(plan.PipelineID, "/")
	}
	if plan.Scope != "" {
		memory.SelectedScope = strings.Trim(plan.Scope, "/")
	}
	memory.Summary = assistantMemorySummary(plan)
	if memory.Entities == nil {
		memory.Entities = map[string]any{}
	}
	memory.Entities["last_intent"] = plan.Intent
	return memory
}

func assistantMemoryAfterTools(memory assistantConversationMemory, plan assistantTurnPlan, toolCalls []assistantToolActivity) assistantConversationMemory {
	for _, call := range toolCalls {
		switch call.Name {
		case "nopsai.analyze_pipeline_run_failure", "nopsai.get_pipeline_run":
			run, _ := call.Output["run"].(map[string]any)
			if run == nil {
				run = call.Output
			}
			if runID := assistantOutputString(run, "run_id"); runID != "" {
				memory.SelectedRun = runID
			}
			if pipelineID := assistantOutputString(run, "pipeline_id"); pipelineID != "" {
				memory.SelectedPipeline = pipelineID
			}
			if scope := assistantOutputString(run, "scope"); scope != "" {
				memory.SelectedScope = strings.Trim(scope, "/")
			}
		case "nopsai.get_pipeline":
			if id := assistantOutputString(call.Output, "id"); id != "" {
				memory.SelectedPipeline = strings.Trim(id, "/")
			}
		case "nopsai.propose_pipeline_create", "nopsai.propose_pipeline_update", "nopsai.get_pipeline_knowledge_context":
			if id := assistantOutputString(call.Output, "pipeline_id"); id != "" {
				memory.SelectedPipeline = strings.Trim(id, "/")
			}
		case "nopsai.get_scope", "nopsai.explain_scope_permissions":
			if scope := assistantOutputString(call.Output, "scope"); scope != "" {
				memory.SelectedScope = strings.Trim(scope, "/")
			}
		}
	}
	return memory
}

func assistantMemorySummary(plan assistantTurnPlan) string {
	switch plan.Intent {
	case "propose_pipeline_create", "propose_pipeline_update":
		return "Preparing a GitOps-safe pipeline write plan."
	case "validate_pipeline":
		return "Validating pipeline YAML without saving changes."
	case "search_pipelines":
		return "Searching visible pipeline metadata and readable pipeline YAML."
	case "pipeline_knowledge_context", "knowledge_context":
		return "Reviewing managed knowledge context with permission-bound reads."
	case "analyze_run":
		return "Investigating a pipeline run failure."
	case "variable_usage":
		return "Analyzing visible variable names and repeated scope usage without reading values."
	case "ai_token_usage":
		return "Reviewing AI token usage analytics by pipeline and run."
	case "feature_tool":
		return "Routing the request to the relevant NopsAI hosted MCP feature tool with current-user authorization."
	case "clarify":
		return "Waiting for one more detail before choosing a NopsAI tool."
	case "scope_secret_summary":
		return "Counting visible scopes and metadata-only secret counts per scope."
	case "runtime":
		return "Checking dispatcher and runner health."
	case "api_call":
		return "Calling an allowed NopsAI API route through hosted MCP."
	case "feature_capabilities":
		return "Reviewing hosted MCP feature coverage for the current subject."
	case "cost":
		return "Reviewing platform cost and usage signals."
	case "design":
		return "Reviewing pipeline design improvement opportunities."
	default:
		return "Using permission-bound Nopsai context for the latest assistant request."
	}
}

func composeAssistantReply(plan assistantTurnPlan, selectedProfile string, toolCalls []assistantToolActivity) string {
	evidenceCalls := assistantEvidenceToolCalls(toolCalls)
	if denial, ok := assistantFirstPlanDenial(toolCalls); ok {
		return composeAssistantPlanDeniedReply(denial)
	}
	if plan.FinalAnswer != "" {
		return plan.FinalAnswer
	}
	if plan.Intent == "clarify" {
		return composeClarifyingReply(plan)
	}
	if len(toolCalls) == 0 {
		return buildAssistantFoundationReply(selectedProfile)
	}
	if len(evidenceCalls) == 0 {
		return buildAssistantFoundationReply(selectedProfile)
	}
	if assistantAllToolsDenied(evidenceCalls) {
		return "I could not use the required Nopsai tools with your current permissions. No changes were applied."
	}
	switch plan.Intent {
	case "propose_pipeline_create", "propose_pipeline_update":
		return composePipelineWritePlanReply(toolCalls)
	case "validate_pipeline":
		return composePipelineValidationReply(toolCalls)
	case "analyze_run":
		return composeRunAnalysisReply(toolCalls)
	case "list_runs":
		return composeRecentRunsReply(toolCalls)
	case "variable_usage":
		return composeVariableUsageReply(toolCalls)
	case "ai_token_usage":
		return composeAIUsageReply(plan, toolCalls)
	case "feature_tool":
		return composeFeatureToolReply(evidenceCalls)
	case "llm_planned":
		return composeFeatureToolReply(evidenceCalls)
	case "cost":
		return composeCostReply(toolCalls)
	case "design":
		return composeSuggestionsReply("Design improvement suggestions", toolCalls)
	case "statistics":
		return composeStatisticsReply(toolCalls)
	case "trigger", "schedule":
		return composeProposalOrInventoryReply(toolCalls)
	case "scope":
		return composeScopeReply(toolCalls)
	case "scope_secret_summary":
		return composeScopeSecretSummaryReply(toolCalls)
	case "api_call":
		return composeAPICallReply(toolCalls)
	case "search_pipelines":
		return composePipelineSearchReply(toolCalls)
	case "pipeline_knowledge_context":
		return composePipelineKnowledgeContextReply(toolCalls)
	case "knowledge_context":
		return composeKnowledgeContextReply(toolCalls)
	case "pipeline":
		return composePipelineReply(toolCalls)
	case "profiles":
		return composeProfilesReply(toolCalls)
	case "feature_capabilities":
		return composeFeatureCapabilitiesReply(toolCalls)
	case "runtime":
		return composeRuntimeReply(toolCalls)
	case "system":
		return composeSystemReply(toolCalls)
	default:
		if reply := composeFallbackEvidenceReply(plan, evidenceCalls); reply != "" {
			return reply
		}
		return composeDocsReply(toolCalls)
	}
}

func composeFallbackEvidenceReply(plan assistantTurnPlan, toolCalls []assistantToolActivity) string {
	if len(toolCalls) == 0 {
		return ""
	}
	if assistantOnlyDocsToolCalls(toolCalls) {
		return composeDocsReply(toolCalls)
	}
	if assistantHasToolCall(toolCalls, "nopsai.get_monitoring_ai_usage") {
		return composeAIUsageReply(plan, toolCalls)
	}
	return composeFeatureToolReply(toolCalls)
}

func assistantOnlyDocsToolCalls(toolCalls []assistantToolActivity) bool {
	if len(toolCalls) == 0 {
		return false
	}
	for _, call := range toolCalls {
		switch call.Name {
		case "nopsai.search_docs", "nopsai.read_doc", "nopsai.list_knowledge_contexts", "nopsai.get_knowledge_context":
		default:
			return false
		}
	}
	return true
}

func assistantHasToolCall(toolCalls []assistantToolActivity, name string) bool {
	for _, call := range toolCalls {
		if call.Name == name {
			return true
		}
	}
	return false
}

func composePipelineWritePlanReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstPipelineWritePlanToolCall(toolCalls)
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not prepare that pipeline write plan.", call)
	}
	if !assistantOutputBool(call.Output, "valid") {
		validation, _ := call.Output["validation"].(map[string]any)
		errText := assistantOutputString(validation, "error")
		if errText == "" {
			errText = assistantOutputString(call.Output, "note")
		}
		if errText == "" {
			errText = "the pipeline YAML did not pass validation"
		}
		return "I could not prepare a pipeline write plan: " + errText + "\n\nNo changes were applied."
	}
	pipelineID := assistantOutputString(call.Output, "pipeline_id")
	action := assistantOutputString(call.Output, "action")
	lines := []string{"I prepared a GitOps-ready pipeline write plan. No changes were applied."}
	if pipelineID != "" {
		lines = append(lines, "", "Pipeline: "+pipelineID)
	}
	if action != "" {
		lines = append(lines, "Required permission: "+action)
	}
	if gitops, _ := call.Output["gitops"].(map[string]any); len(gitops) > 0 {
		if files := assistantMapSlice(gitops["files"]); len(files) > 0 {
			lines = append(lines, "", "GitOps file plan:")
			for _, file := range files {
				if path := assistantOutputString(file, "path"); path != "" {
					lines = append(lines, "- "+path)
				}
			}
		}
		if message := assistantOutputString(gitops, "message"); message != "" {
			lines = append(lines, "Commit message: "+message)
		}
	}
	lines = append(lines, "", "Review the YAML, commit it to the config repository review branch, and sync GitOps.")
	return strings.Join(lines, "\n")
}

func assistantFirstPipelineWritePlanToolCall(toolCalls []assistantToolActivity) assistantToolActivity {
	for _, call := range toolCalls {
		switch call.Name {
		case "nopsai.propose_pipeline_create", "nopsai.propose_pipeline_update":
			return call
		}
	}
	return assistantFirstNonEmptyToolCall(assistantEvidenceToolCalls(toolCalls))
}

func composePipelineValidationReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstToolCall(toolCalls, "nopsai.validate_pipeline")
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not validate that pipeline YAML.", call)
	}
	if assistantOutputBool(call.Output, "valid") {
		name := assistantOutputString(call.Output, "name")
		version := assistantOutputString(call.Output, "version")
		if name != "" {
			return fmt.Sprintf("Validation passed for pipeline %q version %q. No changes were applied.", name, version)
		}
		return "Validation passed. No changes were applied."
	}
	errText := assistantOutputString(call.Output, "error")
	if errText == "" {
		errText = "the validator returned an invalid result without a detailed error"
	}
	return "Validation failed: " + errText + "\n\nNo changes were applied."
}

func composeRunAnalysisReply(toolCalls []assistantToolActivity) string {
	analysis := assistantFirstToolCall(toolCalls, "nopsai.analyze_pipeline_run_failure")
	runCall := assistantFirstToolCall(toolCalls, "nopsai.get_pipeline_run")
	logsCall := assistantFirstToolCall(toolCalls, "nopsai.get_pipeline_run_logs")
	if analysis.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not analyze that run.", analysis)
	}
	run, _ := runCall.Output["run"].(map[string]any)
	if run == nil {
		run = runCall.Output
	}
	if len(run) == 0 {
		run, _ = analysis.Output["run"].(map[string]any)
	}
	if run == nil {
		run = map[string]any{}
	}
	lines := []string{"I investigated the run through the hosted Nopsai MCP chain: status, recent logs, then failure analysis."}
	if runID := assistantOutputString(run, "run_id"); runID != "" {
		lines = append(lines, "", "Run: "+runID)
	}
	if pipelineID := assistantOutputString(run, "pipeline_id"); pipelineID != "" {
		lines = append(lines, "Pipeline: "+pipelineID)
	}
	if status := assistantOutputString(run, "status"); status != "" {
		lines = append(lines, "Status: "+status)
	}
	if reason := assistantOutputString(run, "failure_reason"); reason != "" {
		lines = append(lines, "Recorded failure: "+reason)
	}
	logs := assistantMapSlice(logsCall.Output["logs"])
	if len(logs) == 0 {
		logs = assistantMapSlice(analysis.Output["log_excerpt"])
	}
	if len(logs) > 0 {
		lines = append(lines, fmt.Sprintf("Log lines reviewed: %d", len(logs)))
		if assistantOutputBool(logsCall.Output, "bytes_truncated") {
			lines = append(lines, fmt.Sprintf(
				"Log excerpt was truncated at %.0f bytes by assistant safety limits.",
				assistantOutputFloat(logsCall.Output, "max_bytes"),
			))
		}
		if excerpt := assistantLastRelevantLogLine(logs); excerpt != "" {
			lines = append(lines, "Most relevant log line: "+excerpt)
		}
	}
	if hint := assistantOutputString(analysis.Output, "root_cause_hint"); hint != "" {
		lines = append(lines, "", "Root-cause hint: "+hint)
	}
	if steps := assistantStringSlice(analysis.Output["suggested_next_steps"]); len(steps) > 0 {
		lines = append(lines, "", "Suggested next steps:")
		for _, step := range steps {
			lines = append(lines, "- "+step)
		}
	}
	lines = append(lines, "", "No changes were applied.")
	return strings.Join(lines, "\n")
}

func composeVariableUsageReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstToolCall(toolCalls, "nopsai.analyze_variable_usage")
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not analyze visible variable usage.", call)
	}
	lines := []string{"Visible variable usage:"}
	lines = append(lines, fmt.Sprintf("- Total visible variable entries: %.0f", assistantOutputFloat(call.Output, "total_visible_variables")))
	lines = append(lines, fmt.Sprintf("- Unique variable names: %.0f", assistantOutputFloat(call.Output, "unique_variable_names")))
	lines = append(lines, fmt.Sprintf("- Repeated variable names: %.0f", assistantOutputFloat(call.Output, "repetitive_variable_names")))
	duplicates := assistantMapSlice(call.Output["duplicates"])
	if len(duplicates) > 0 {
		lines = append(lines, "", "Most repeated variables:")
		for _, duplicate := range duplicates {
			line := fmt.Sprintf("- %s: %.0f entries", assistantOutputString(duplicate, "name"), assistantOutputFloat(duplicate, "occurrences"))
			if scopes := assistantStringSlice(duplicate["scopes"]); len(scopes) > 0 {
				line += " across scopes " + strings.Join(scopes, ", ")
			}
			if repositories := assistantStringSlice(duplicate["repositories"]); len(repositories) > 0 {
				line += " and repositories " + strings.Join(repositories, ", ")
			}
			lines = append(lines, line)
		}
	} else {
		lines = append(lines, "", "No repeated variable names were visible to your account.")
	}
	lines = append(lines, "", "Values were not read. No changes were applied.")
	return strings.Join(lines, "\n")
}

func composeAIUsageReply(plan assistantTurnPlan, toolCalls []assistantToolActivity) string {
	usageCalls := assistantToolCallsByName(toolCalls, "nopsai.get_monitoring_ai_usage")
	call := assistantBestAIUsageCall(usageCalls)
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not load AI token usage analytics.", call)
	}
	output := assistantAIUsageOutput(call)
	lines := []string{"AI token usage investigation:"}
	if filterSummary := assistantAIUsageFilterSummary(call.Input); filterSummary != "" {
		lines = append(lines, "- Filters: "+filterSummary)
	}
	if len(usageCalls) > 1 {
		lines = append(lines, "- Windows checked: "+fmt.Sprint(len(usageCalls)))
	}
	if assistantAnyAIUsageCallHasEvents(usageCalls) {
		if dimensionReply := composeDimensionAIUsageReply(plan, call, output, usageCalls); dimensionReply != "" {
			return dimensionReply
		}
	}
	lines = append(lines, fmt.Sprintf("- Total tokens: %.0f", assistantOutputFloat(output, "total_tokens")))
	lines = append(lines, fmt.Sprintf("- Prompt tokens: %.0f", assistantOutputFloat(output, "total_prompt_tokens")))
	lines = append(lines, fmt.Sprintf("- Completion tokens: %.0f", assistantOutputFloat(output, "total_completion_tokens")))
	lines = append(lines, fmt.Sprintf("- Exact token events: %.0f", assistantOutputFloat(output, "exact_token_events")))
	lines = append(lines, fmt.Sprintf("- Estimated token events: %.0f", assistantOutputFloat(output, "estimated_token_events")))
	lines = assistantAppendAIUsageOverview(lines, output)
	if !assistantAnyAIUsageCallHasEvents(usageCalls) {
		lines = append(lines, "", "Investigation evidence:")
		for idx, usageCall := range usageCalls {
			usageOutput := assistantAIUsageOutput(usageCall)
			lines = append(lines, fmt.Sprintf(
				"- %s: %.0f tokens across %.0f visible events",
				assistantAIUsageWindowLabel(usageCall, idx),
				assistantOutputFloat(usageOutput, "total_tokens"),
				assistantOutputFloat(usageOutput, "exact_token_events")+assistantOutputFloat(usageOutput, "estimated_token_events"),
			))
		}
		lines = append(lines, "", "Diagnosis:")
		lines = append(lines, "- I found no visible AI usage events in the checked monitoring windows.")
		if runs := assistantVisibleRunCount(toolCalls); runs > 0 {
			lines = append(lines, fmt.Sprintf("- I did find %.0f visible pipeline runs, so the likely issue is token usage recording or runs that did not execute LLM-backed tasks.", runs))
		} else {
			lines = append(lines, "- I did not find recent visible runs with the available tools, so this may also be a permissions or data-retention window issue.")
		}
		lines = append(lines, "- Token recording depends on the agent reporting collected usage to /v1/internal/runs/{runID}/ai-usage.")
		lines = append(lines, "- Next action: inspect a recent run that should have used goal, condition, or resolver LLM work and verify the agent AI usage reporter path.")
	}
	lines = append(lines, "", "No changes were applied.")
	return strings.Join(lines, "\n")
}

func composeDimensionAIUsageReply(plan assistantTurnPlan, call assistantToolActivity, output map[string]any, usageCalls []assistantToolActivity) string {
	_, field, header, topLabel, breakdownTitle, ok := assistantAIUsageRequestedDimension(plan, output)
	if !ok {
		return ""
	}
	items := assistantMapSlice(output[field])
	if len(items) == 0 {
		return ""
	}
	lines := []string{header + ":"}
	if filterSummary := assistantAIUsageFilterSummary(call.Input); filterSummary != "" {
		lines = append(lines, "- Filters: "+filterSummary)
	}
	if len(usageCalls) > 1 {
		lines = append(lines, "- Windows checked: "+fmt.Sprint(len(usageCalls)))
	}
	lines = append(lines, fmt.Sprintf("- Total tokens checked: %.0f", assistantOutputFloat(output, "total_tokens")))
	if assistantAIUsageTextHasAnyTerm(assistantAIUsageQuestionText(plan), []string{"cost", "costs", "price", "pricing", "spend"}) {
		lines = append(lines, "- Pricing fields are not included in this monitoring payload, so this ranking uses token volume.")
	}
	first := items[0]
	firstLabel := firstNonEmptyString(assistantOutputString(first, "label"), assistantOutputString(first, "key"))
	topShown := false
	if firstLabel != "" && topLabel != "" {
		lines = append(lines, assistantAIUsageTopLine(topLabel, firstLabel, assistantOutputFloat(first, "tokens"), assistantOutputFloat(first, "count")))
		topShown = true
	}
	start := 0
	if topShown {
		start = 1
	}
	if start >= len(items) {
		lines = append(lines, "", "No changes were applied.")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "", breakdownTitle+":")
	for idx := start; idx < len(items) && idx < start+5; idx++ {
		item := items[idx]
		label := firstNonEmptyString(assistantOutputString(item, "label"), assistantOutputString(item, "key"))
		if label == "" {
			continue
		}
		lines = append(lines, assistantAIUsageItemLine(label, assistantOutputFloat(item, "tokens"), assistantOutputFloat(item, "count")))
	}
	lines = append(lines, "", "No changes were applied.")
	return strings.Join(lines, "\n")
}

func assistantAIUsageQuestionText(plan assistantTurnPlan) string {
	return strings.ToLower(strings.TrimSpace(strings.Join([]string{plan.LowerContent, plan.Goal, plan.SuccessCriteria}, " ")))
}

type assistantAIUsageDimension struct {
	Name           string
	Field          string
	LowField       string
	Header         string
	LowHeader      string
	TopLabel       string
	LowTopLabel    string
	BreakdownTitle string
	LowBreakdown   string
	Terms          []string
	Overview       bool
}

func assistantAIUsageDimensionCatalog() []assistantAIUsageDimension {
	return []assistantAIUsageDimension{
		{Name: "provider", Field: "by_provider", Header: "AI usage by provider", TopLabel: "Highest token provider", BreakdownTitle: "Provider breakdown", Terms: []string{"provider", "providers"}, Overview: true},
		{Name: "model", Field: "by_model", Header: "AI usage by model", TopLabel: "Highest token model", BreakdownTitle: "Model breakdown", Terms: []string{"model", "models"}, Overview: true},
		{Name: "profile", Field: "by_profile", Header: "AI usage by LLM profile", TopLabel: "Highest token LLM profile", BreakdownTitle: "LLM profile breakdown", Terms: []string{"profile", "profiles", "llm profile", "llm profiles"}, Overview: true},
		{Name: "feature", Field: "by_feature", Header: "AI usage by feature", TopLabel: "Highest token feature", BreakdownTitle: "Feature breakdown", Terms: []string{"feature", "features"}, Overview: true},
		{Name: "pipeline", Field: "by_pipeline", Header: "AI usage by pipeline", TopLabel: "Highest token pipeline", BreakdownTitle: "Other high-token pipelines", Terms: []string{"pipeline", "pipelines"}, Overview: true},
		{Name: "step", Field: "by_step", Header: "AI usage by step", TopLabel: "Highest token step", BreakdownTitle: "Other high-token steps", Terms: []string{"step", "steps"}},
		{Name: "task", Field: "by_task", Header: "AI usage by task", TopLabel: "Highest token task", BreakdownTitle: "Other high-token tasks", Terms: []string{"task", "tasks"}},
		{Name: "schedule", Field: "by_schedule", LowField: "lowest_token_schedules", Header: "AI usage by schedule", LowHeader: "Lowest token schedules", TopLabel: "Highest token schedule", LowTopLabel: "Lowest token schedule", BreakdownTitle: "Other high-token schedules", LowBreakdown: "Other low-token schedules", Terms: []string{"schedule", "schedules", "scheduled", "cron"}},
		{Name: "run", Field: "top_token_runs", Header: "AI usage by run", TopLabel: "Highest token run", BreakdownTitle: "Other high-token runs", Terms: []string{"run", "runs", "pipeline run", "pipeline runs"}},
	}
}

func assistantAIUsageRequestedDimension(plan assistantTurnPlan, output map[string]any) (assistantAIUsageDimension, string, string, string, string, bool) {
	text := assistantAIUsageQuestionText(plan)
	lowRequested := assistantAIUsageTextHasAnyTerm(text, []string{"low", "lower", "lowest", "least", "less", "minimal", "minimum", "cheapest"})
	bestScore := 0
	var best assistantAIUsageDimension
	for _, dimension := range assistantAIUsageDimensionCatalog() {
		field := dimension.Field
		if lowRequested && dimension.LowField != "" && len(assistantMapSlice(output[dimension.LowField])) > 0 {
			field = dimension.LowField
		}
		if len(assistantMapSlice(output[field])) == 0 {
			continue
		}
		score := 0
		for _, term := range dimension.Terms {
			if assistantAIUsageTextHasTerm(text, term) {
				score += 10 + strings.Count(term, " ")
			}
		}
		if score > bestScore {
			bestScore = score
			best = dimension
		}
	}
	if bestScore == 0 {
		return assistantAIUsageDimension{}, "", "", "", "", false
	}
	field := best.Field
	header := best.Header
	topLabel := best.TopLabel
	breakdownTitle := best.BreakdownTitle
	if lowRequested && best.LowField != "" && len(assistantMapSlice(output[best.LowField])) > 0 {
		field = best.LowField
		header = firstNonEmptyString(best.LowHeader, header)
		topLabel = firstNonEmptyString(best.LowTopLabel, topLabel)
		breakdownTitle = firstNonEmptyString(best.LowBreakdown, breakdownTitle)
	}
	return best, field, header, topLabel, breakdownTitle, true
}

func assistantAppendAIUsageOverview(lines []string, output map[string]any) []string {
	sections := 0
	for _, dimension := range assistantAIUsageDimensionCatalog() {
		if !dimension.Overview {
			continue
		}
		items := assistantMapSlice(output[dimension.Field])
		if len(items) == 0 {
			continue
		}
		lines = append(lines, "", dimension.Header+":")
		for idx, item := range items {
			if idx >= 3 {
				break
			}
			label := firstNonEmptyString(assistantOutputString(item, "label"), assistantOutputString(item, "key"))
			if label == "" {
				continue
			}
			lines = append(lines, assistantAIUsageItemLine(label, assistantOutputFloat(item, "tokens"), assistantOutputFloat(item, "count")))
		}
		sections++
		if sections >= 5 {
			break
		}
	}
	return lines
}

func assistantAIUsageTextHasAnyTerm(text string, terms []string) bool {
	for _, term := range terms {
		if assistantAIUsageTextHasTerm(text, term) {
			return true
		}
	}
	return false
}

func assistantAIUsageTextHasTerm(text, term string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	term = strings.ToLower(strings.TrimSpace(term))
	if text == "" || term == "" {
		return false
	}
	if strings.Contains(term, " ") {
		return strings.Contains(text, term)
	}
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(term) + `\b`)
	return pattern.MatchString(text)
}

func assistantAIUsageTopLine(topLabel, label string, tokens, count float64) string {
	if count > 0 {
		return fmt.Sprintf("- %s: %s with %.0f tokens across %.0f events", topLabel, label, tokens, count)
	}
	return fmt.Sprintf("- %s: %s with %.0f tokens", topLabel, label, tokens)
}

func assistantAIUsageItemLine(label string, tokens, count float64) string {
	if count > 0 {
		return fmt.Sprintf("- %s: %.0f tokens across %.0f events", label, tokens, count)
	}
	return fmt.Sprintf("- %s: %.0f tokens", label, tokens)
}

func composeClarifyingReply(plan assistantTurnPlan) string {
	question := strings.TrimSpace(plan.ClarifyQuestion)
	if question == "" {
		question = "Which NopsAI area should I check?"
	}
	return "I can help, but I need one more detail first. " + question
}

func composePipelineKnowledgeContextReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstNonEmptyToolCall(toolCalls)
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not load pipeline knowledge context.", call)
	}
	if call.Name == "nopsai.list_knowledge_contexts" {
		return composeKnowledgeContextReply(toolCalls)
	}
	lines := []string{"Pipeline knowledge context:"}
	if pipelineID := assistantOutputString(call.Output, "pipeline_id"); pipelineID != "" {
		lines = append(lines, "- Pipeline: "+pipelineID)
	}
	lines = append(lines, fmt.Sprintf("- Managed documents loaded: %v", call.Output["document_count"]))
	lines = append(lines, fmt.Sprintf("- Unresolved or run-time-only references: %v", call.Output["unresolved_count"]))
	for _, doc := range assistantMapSlice(call.Output["documents"]) {
		id := assistantOutputString(doc, "id")
		if id == "" {
			continue
		}
		line := "- " + id
		if status := assistantOutputString(doc, "status"); status != "" {
			line += " (" + status + ")"
		} else if desc := assistantOutputString(doc, "description"); desc != "" {
			line += ": " + desc
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", "No changes were applied.")
	return strings.Join(lines, "\n")
}

func composeKnowledgeContextReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstNonEmptyToolCall(toolCalls)
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not load knowledge context.", call)
	}
	items := assistantMapSlice(call.Output["knowledge_contexts"])
	if len(items) == 0 {
		return "I did not find managed knowledge context visible to your account. No changes were applied."
	}
	lines := []string{"Visible matching knowledge context:"}
	for _, item := range items {
		line := "- " + assistantOutputString(item, "id")
		if desc := assistantOutputString(item, "description"); desc != "" {
			line += ": " + desc
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", "No changes were applied.")
	return strings.Join(lines, "\n")
}

func composeRecentRunsReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstToolCall(toolCalls, "nopsai.list_pipeline_runs")
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not list recent runs.", call)
	}
	runs := assistantMapSlice(call.Output["runs"])
	if len(runs) == 0 {
		return "I did not find recent pipeline runs visible to your account."
	}
	lines := []string{"Recent visible pipeline runs:"}
	for _, run := range runs {
		line := "- " + assistantOutputString(run, "run_id")
		if pipelineID := assistantOutputString(run, "pipeline_id"); pipelineID != "" {
			line += " (" + pipelineID + ")"
		}
		if status := assistantOutputString(run, "status"); status != "" {
			line += ": " + status
		}
		if reason := assistantOutputString(run, "failure_reason"); reason != "" {
			line += " - " + reason
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", "Share a run ID and I can analyze status and logs.")
	return strings.Join(lines, "\n")
}

func composeCostReply(toolCalls []assistantToolActivity) string {
	summary := assistantFirstToolCall(toolCalls, "nopsai.get_cost_summary")
	suggestions := assistantFirstToolCall(toolCalls, "nopsai.suggest_cost_improvements")
	if summary.Status != assistantToolStatusSuccess && suggestions.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not load cost data.", summary)
	}
	lines := []string{"Cost and usage review:"}
	if summary.Status == assistantToolStatusSuccess {
		lines = append(lines, fmt.Sprintf(
			"- Runner cost: $%.2f",
			assistantOutputFloat(summary.Output, "runner_cost_usd"),
		))
		lines = append(lines, fmt.Sprintf("- AI cost: $%.2f", assistantOutputFloat(summary.Output, "ai_cost_usd")))
		lines = append(lines, fmt.Sprintf("- Total cost: $%.2f", assistantOutputFloat(summary.Output, "total_cost_usd")))
	}
	if suggestions.Status == assistantToolStatusSuccess {
		if values := assistantStringSlice(suggestions.Output["suggestions"]); len(values) > 0 {
			lines = append(lines, "", "Suggestions:")
			for _, value := range values {
				lines = append(lines, "- "+value)
			}
		}
	}
	lines = append(lines, "", "No changes were applied.")
	return strings.Join(lines, "\n")
}

func composeSuggestionsReply(title string, toolCalls []assistantToolActivity) string {
	for _, call := range toolCalls {
		if call.Status == assistantToolStatusSuccess {
			if suggestions := assistantStringSlice(call.Output["suggestions"]); len(suggestions) > 0 {
				lines := []string{title + ":"}
				for _, suggestion := range suggestions {
					lines = append(lines, "- "+suggestion)
				}
				lines = append(lines, "", "No changes were applied.")
				return strings.Join(lines, "\n")
			}
		}
	}
	return assistantToolErrorReply("I could not produce suggestions.", assistantFirstNonEmptyToolCall(toolCalls))
}

func composeStatisticsReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstToolCall(toolCalls, "nopsai.get_statistics")
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not load statistics.", call)
	}
	counts, _ := call.Output["counts"].(map[string]any)
	if len(counts) == 0 {
		return "I could not find platform statistics in the response."
	}
	lines := []string{"Platform statistics:"}
	for _, key := range []string{"pipelines", "pipeline_runs", "triggers", "schedules", "scopes", "knowledge"} {
		if value, ok := counts[key]; ok {
			lines = append(lines, fmt.Sprintf("- %s: %v", key, value))
		}
	}
	return strings.Join(lines, "\n")
}

func composeProposalOrInventoryReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstNonEmptyToolCall(toolCalls)
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not complete that workflow.", call)
	}
	if proposalType := assistantOutputString(call.Output, "proposal_type"); proposalType != "" {
		return fmt.Sprintf("I drafted a %s proposal. No changes were applied; review and apply it through the existing API/GitOps approval flow.", strings.ReplaceAll(proposalType, "_", " "))
	}
	return "I loaded the requested inventory. No changes were applied."
}

func composeScopeReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstNonEmptyToolCall(toolCalls)
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not load scope context.", call)
	}
	if scope := assistantOutputString(call.Output, "scope"); scope != "" {
		lines := []string{"Scope: " + scope}
		if explanation := assistantOutputString(call.Output, "explanation"); explanation != "" {
			lines = append(lines, "", explanation)
		}
		return strings.Join(lines, "\n")
	}
	scopes := assistantStringSlice(call.Output["scopes"])
	if len(scopes) == 0 {
		return "I did not find scopes visible to your account."
	}
	return "Visible scopes:\n- " + strings.Join(scopes, "\n- ")
}

func composeScopeSecretSummaryReply(toolCalls []assistantToolActivity) string {
	scopesCall := assistantFirstToolCall(toolCalls, "nopsai.list_scopes")
	secretsCall := assistantFirstToolCall(toolCalls, "nopsai.list_secret_scopes")
	if scopesCall.Status != assistantToolStatusSuccess && secretsCall.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not load scope and secret metadata.", assistantFirstNonEmptyToolCall(toolCalls))
	}

	scopeSet := map[string]struct{}{}
	scopes := assistantStringSlice(scopesCall.Output["scopes"])
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope != "" {
			scopeSet[scope] = struct{}{}
		}
	}

	secretCounts := map[string]int{}
	for _, row := range assistantSecretScopeRows(secretsCall.Output) {
		scope := strings.TrimSpace(assistantOutputString(row, "scope"))
		if scope == "" {
			continue
		}
		count := int(assistantOutputFloat(row, "secret_count"))
		if count == 0 {
			count = int(assistantOutputFloat(row, "secrets"))
		}
		secretCounts[scope] = count
		scopeSet[scope] = struct{}{}
	}

	orderedScopes := make([]string, 0, len(scopeSet))
	for scope := range scopeSet {
		orderedScopes = append(orderedScopes, scope)
	}
	sort.Slice(orderedScopes, func(i, j int) bool {
		return strings.ToLower(orderedScopes[i]) < strings.ToLower(orderedScopes[j])
	})

	totalSecrets := 0
	for _, scope := range orderedScopes {
		totalSecrets += secretCounts[scope]
	}

	lines := []string{"Visible scopes and secret counts:"}
	if scopesCall.Status == assistantToolStatusSuccess {
		lines = append(lines, fmt.Sprintf("- Total visible scopes: %d", len(orderedScopes)))
	} else {
		lines = append(lines, "- Total visible scopes: unavailable with current permissions")
	}
	if secretsCall.Status == assistantToolStatusSuccess {
		lines = append(lines, fmt.Sprintf("- Total visible secrets: %d", totalSecrets))
	} else {
		lines = append(lines, "- Secret counts: unavailable with current permissions")
	}

	if len(orderedScopes) > 0 {
		lines = append(lines, "", "By scope:")
		for _, scope := range orderedScopes {
			count := secretCounts[scope]
			label := "secrets"
			if count == 1 {
				label = "secret"
			}
			lines = append(lines, fmt.Sprintf("- %s: %d %s", scope, count, label))
		}
	} else if secretsCall.Status == assistantToolStatusSuccess {
		lines = append(lines, "", "No visible scopes with secrets were found.")
	}

	if secretsCall.Status != assistantToolStatusSuccess {
		lines = append(lines, "", "I could list scopes, but secret metadata is not available to your current permissions.")
	}
	lines = append(lines, "", "Only secret metadata was used; plaintext secret values were not read. No changes were applied.")
	return strings.Join(lines, "\n")
}

func composeAPICallReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstToolCall(toolCalls, "nopsai.call_api")
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not call that NopsAI API route.", call)
	}
	lines := []string{"NopsAI API call through hosted MCP:"}
	method := assistantOutputString(call.Output, "method")
	path := assistantOutputString(call.Output, "path")
	if method != "" || path != "" {
		lines = append(lines, "- Route: "+strings.TrimSpace(method+" "+path))
	}
	if assistantOutputBool(call.Output, "requires_confirmation") {
		lines = append(lines, "- Confirmation required: set confirm:true to execute this mutating route.")
		lines = append(lines, "- Applied: false")
		return strings.Join(lines, "\n")
	}
	if status := assistantOutputFloat(call.Output, "status_code"); status > 0 {
		lines = append(lines, fmt.Sprintf("- Status: %.0f", status))
	}
	lines = append(lines, fmt.Sprintf("- Applied: %t", assistantOutputBool(call.Output, "applied")))
	if !assistantOutputBool(call.Output, "ok") {
		if text := assistantOutputString(call.Output, "response_text"); text != "" {
			lines = append(lines, "- Response: "+text)
		}
	}
	return strings.Join(lines, "\n")
}

func composePipelineSearchReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstToolCall(toolCalls, "nopsai.search_pipelines")
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not search pipelines.", call)
	}
	pipelines := assistantMapSlice(call.Output["pipelines"])
	if len(pipelines) == 0 {
		query := assistantOutputString(call.Output, "query")
		if query != "" {
			return "I did not find visible pipelines matching " + query + ". Try a pipeline path, name, step name, or YAML keyword."
		}
		return "I did not find matching pipelines visible to your account."
	}
	lines := []string{"Matching pipelines:"}
	for _, pipeline := range pipelines {
		line := "- " + assistantOutputString(pipeline, "id")
		if fields := assistantStringSlice(pipeline["match_fields"]); len(fields) > 0 {
			line += " (matched " + strings.Join(fields, ", ") + ")"
		}
		lines = append(lines, line)
		if snippet := assistantOutputString(pipeline, "snippet"); snippet != "" {
			lines = append(lines, "  "+snippet)
		}
	}
	lines = append(lines, "", "Use a specific pipeline ID if you want me to load the full YAML.")
	return strings.Join(lines, "\n")
}

func composePipelineReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstNonEmptyToolCall(toolCalls)
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not load pipeline context.", call)
	}
	if definition := assistantOutputString(call.Output, "definition"); definition != "" {
		lines := []string{"I loaded the pipeline definition. No changes were applied."}
		if id := assistantOutputString(call.Output, "id"); id != "" {
			lines = append(lines, "", "Pipeline: "+id)
		}
		lines = append(lines, "", "```yaml", strings.TrimSpace(definition), "```")
		return strings.Join(lines, "\n")
	}
	pipelines := assistantMapSlice(call.Output["pipelines"])
	if len(pipelines) == 0 {
		return "I did not find pipelines visible to your account."
	}
	lines := []string{"Visible pipelines:"}
	for _, pipeline := range pipelines {
		line := "- " + assistantOutputString(pipeline, "id")
		if source := assistantOutputString(pipeline, "source"); source != "" {
			line += " (" + source + ")"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func composeRuntimeReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstToolCall(toolCalls, "nopsai.get_dispatcher_status")
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not check dispatcher and runner status.", call)
	}
	lines := []string{"Dispatcher and runner status:"}
	dispatcher, _ := call.Output["dispatcher"].(map[string]any)
	if status := assistantOutputString(dispatcher, "status"); status != "" {
		lines = append(lines, "- Dispatcher: "+status)
	}
	if errText := assistantOutputString(dispatcher, "error"); errText != "" {
		lines = append(lines, "  "+errText)
	}
	summary, _ := call.Output["runner_summary"].(monitoringRunnerSummary)
	if summary.Total > 0 || call.Output["runner_summary"] != nil {
		lines = append(lines, fmt.Sprintf(
			"- Runners: %d total, %d online, %d stale, %d unreachable, %d disabled, capacity %d, active jobs %d, queued jobs %d",
			summary.Total,
			summary.Online,
			summary.Stale,
			summary.Unreachable,
			summary.Disabled,
			summary.Capacity,
			summary.ActiveJobs,
			summary.QueuedJobs,
		))
	}
	for _, runner := range assistantRuntimeRunnerRows(call.Output["runners"]) {
		line := "- " + runner.RunnerID + ": " + runner.Status
		if runner.Runtime != "" {
			line += " (" + runner.Runtime + ")"
		}
		line += fmt.Sprintf(", active %d/%d", runner.ActiveJobs, runner.Capacity)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func composeProfilesReply(toolCalls []assistantToolActivity) string {
	lines := []string{"Configured assistant profile context:"}
	for _, call := range toolCalls {
		if call.Status != assistantToolStatusSuccess {
			continue
		}
		switch call.Name {
		case "nopsai.get_llm_profiles":
			names := []string{}
			for _, profile := range assistantMapSlice(call.Output["profiles"]) {
				if name := assistantOutputString(profile, "name"); name != "" {
					names = append(names, name)
				}
			}
			lines = append(lines, "- LLM profiles: "+assistantJoinOrNone(names))
		case "nopsai.get_mcp_profiles":
			names := []string{}
			for _, profile := range assistantMapSlice(call.Output["profiles"]) {
				if name := assistantOutputString(profile, "name"); name != "" {
					names = append(names, name)
				}
			}
			lines = append(lines, "- MCP profiles: "+assistantJoinOrNone(names))
		}
	}
	return strings.Join(lines, "\n")
}

func composeFeatureCapabilitiesReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstToolCall(toolCalls, "nopsai.get_feature_capabilities")
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not load MCP feature capabilities for your current permissions.", call)
	}
	lines := []string{"MCP feature coverage for your current permissions:"}
	if areaCount := assistantOutputFloat(call.Output, "area_count"); areaCount > 0 {
		lines = append(lines, fmt.Sprintf("- Areas: %.0f", areaCount))
	}
	if featureCount := assistantOutputFloat(call.Output, "feature_count"); featureCount > 0 {
		lines = append(lines, fmt.Sprintf("- Features tracked: %.0f", featureCount))
	}
	for _, area := range assistantMapSlice(call.Output["areas"]) {
		name := assistantOutputString(area, "area")
		if name == "" {
			continue
		}
		coverage := assistantOutputString(area, "coverage")
		mode := assistantOutputString(area, "mode")
		userAccess, _ := area["user_access"].(map[string]any)
		toolAccess := assistantOutputString(userAccess, "tools")
		permissionAccess := assistantOutputString(userAccess, "permissions")
		line := "- " + name
		if coverage != "" {
			line += ": " + coverage
		}
		if toolAccess != "" && toolAccess != "not_applicable" {
			line += ", tools " + toolAccess
		}
		if permissionAccess != "" && permissionAccess != "not_applicable" {
			line += ", permissions " + permissionAccess
		}
		if mode != "" {
			line += " (" + mode + ")"
		}
		lines = append(lines, line)
	}
	if notes := assistantCapabilityPolicyNotes(call.Output); len(notes) > 0 {
		lines = append(lines, "", "Policy notes:")
		for _, note := range notes {
			lines = append(lines, "- "+note)
		}
	}
	lines = append(lines, "", "The assistant and hosted MCP use the current authenticated AAA subject for the user. No changes were applied.")
	return strings.Join(lines, "\n")
}

func composeSystemReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstToolCall(toolCalls, "nopsai.get_system_status")
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not load system status.", call)
	}
	lines := []string{"System status context loaded."}
	if environment := assistantOutputString(call.Output, "environment"); environment != "" {
		lines = append(lines, "- Environment: "+environment)
	}
	lines = append(lines, "- Assistant settings and runtime statistics were read with your current permissions.")
	return strings.Join(lines, "\n")
}

func composeDocsReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstToolCall(toolCalls, "nopsai.search_docs")
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not search docs.", call)
	}
	docs := assistantMapSlice(call.Output["docs"])
	if len(docs) == 0 {
		return "I searched the knowledge context but did not find a matching document. Try naming a pipeline, run, scope, schedule, trigger, or docs topic."
	}
	lines := []string{"Relevant docs:"}
	for _, doc := range docs {
		line := "- " + assistantOutputString(doc, "name")
		if id := assistantOutputString(doc, "id"); id != "" {
			line += " [" + id + "]"
		}
		team := assistantOutputString(doc, "team_path")
		if team == "" {
			team = assistantOutputString(doc, "team")
		}
		if team != "" {
			line += " (" + team + ")"
		}
		if desc := assistantOutputString(doc, "description"); desc != "" {
			line += ": " + desc
		}
		lines = append(lines, line)
		if snippet := assistantOutputString(doc, "snippet"); snippet != "" {
			lines = append(lines, "  "+snippet)
		}
	}
	return strings.Join(lines, "\n")
}

func assistantToolErrorReply(prefix string, call assistantToolActivity) string {
	errText := assistantOutputString(call.Output, "error")
	if errText == "" {
		errText = strings.TrimSpace(call.Status)
	}
	if errText == "" {
		errText = "the tool did not return a successful result"
	}
	return prefix + " " + errText + "\n\nNo changes were applied."
}

func assistantAllToolsDenied(toolCalls []assistantToolActivity) bool {
	if len(toolCalls) == 0 {
		return false
	}
	for _, call := range toolCalls {
		if call.Status != assistantToolStatusDenied {
			return false
		}
	}
	return true
}

func assistantFirstToolCall(toolCalls []assistantToolActivity, name string) assistantToolActivity {
	for _, call := range toolCalls {
		if call.Name == name {
			return call
		}
	}
	return assistantToolActivity{Name: name, Output: map[string]any{}}
}

func assistantFirstNonEmptyToolCall(toolCalls []assistantToolActivity) assistantToolActivity {
	if len(toolCalls) == 0 {
		return assistantToolActivity{Output: map[string]any{}}
	}
	return toolCalls[0]
}

func assistantResourceURIsForTool(name string) []string {
	switch name {
	case "nopsai.search_docs", "nopsai.read_doc", "nopsai.list_knowledge_contexts", "nopsai.get_knowledge_context", "nopsai.propose_knowledge_context_create", "nopsai.propose_knowledge_context_update", "nopsai.propose_knowledge_context_delete":
		return []string{"nopsai://docs"}
	case "nopsai.list_pipelines", "nopsai.search_pipelines", "nopsai.get_pipeline", "nopsai.validate_pipeline", "nopsai.propose_pipeline_create", "nopsai.propose_pipeline_update":
		return []string{"nopsai://pipelines"}
	case "nopsai.get_pipeline_knowledge_context":
		return []string{"nopsai://pipelines", "nopsai://docs"}
	case "nopsai.run_pipeline", "nopsai.list_pipeline_runs", "nopsai.get_pipeline_run", "nopsai.get_pipeline_run_logs", "nopsai.analyze_pipeline_run_failure", "nopsai.list_run_approvals", "nopsai.approve_run_approval", "nopsai.reject_run_approval", "nopsai.rerun_pipeline_run", "nopsai.cancel_pipeline_run", "nopsai.delete_pipeline_run", "nopsai.list_lab_items", "nopsai.get_lab_item", "nopsai.explain_lab_result":
		return []string{"nopsai://pipeline-runs"}
	case "nopsai.list_triggers", "nopsai.get_trigger", "nopsai.propose_trigger_change", "nopsai.list_git_webhook_sources", "nopsai.get_git_webhook_source", "nopsai.list_git_webhook_deliveries", "nopsai.propose_git_webhook_source_create", "nopsai.propose_git_webhook_source_update", "nopsai.propose_git_webhook_source_delete", "nopsai.list_external_triggers", "nopsai.get_external_trigger", "nopsai.list_external_trigger_invocations", "nopsai.propose_external_trigger_create", "nopsai.propose_external_trigger_update", "nopsai.propose_external_trigger_delete", "nopsai.invoke_external_trigger":
		return []string{"nopsai://triggers"}
	case "nopsai.list_schedules", "nopsai.get_schedule", "nopsai.propose_schedule_change", "nopsai.propose_schedule_create", "nopsai.propose_schedule_update", "nopsai.propose_schedule_delete", "nopsai.propose_schedule_enable", "nopsai.propose_schedule_disable", "nopsai.run_schedule_now":
		return []string{"nopsai://schedules"}
	case "nopsai.list_dashboards", "nopsai.get_dashboard", "nopsai.list_dashboard_refreshes", "nopsai.list_dashboard_refresh_schedules", "nopsai.refresh_dashboard", "nopsai.run_dashboard_refresh_schedule":
		return []string{"nopsai://dashboards"}
	case "nopsai.list_scopes", "nopsai.get_scope", "nopsai.explain_scope_permissions", "nopsai.list_secret_scopes":
		return []string{"nopsai://scopes"}
	case "nopsai.analyze_variable_usage", "nopsai.list_variable_scopes", "nopsai.list_variables_metadata", "nopsai.get_variable_value", "nopsai.write_variable_value", "nopsai.delete_variable_value", "nopsai.propose_variable_gitops_write", "nopsai.propose_variable_gitops_delete":
		return []string{"nopsai://scopes", "nopsai://variables"}
	case "nopsai.list_secrets_metadata", "nopsai.encrypt_secret_for_gitops", "nopsai.write_secret_value", "nopsai.delete_secret_value", "nopsai.propose_secret_gitops_write", "nopsai.propose_secret_gitops_delete":
		return []string{"nopsai://scopes", "nopsai://secrets"}
	case "nopsai.get_cost_summary", "nopsai.suggest_cost_improvements":
		return []string{"nopsai://costs"}
	case "nopsai.get_monitoring_summary", "nopsai.get_monitoring_run_analytics", "nopsai.get_monitoring_pipeline_performance", "nopsai.get_monitoring_step_performance", "nopsai.get_monitoring_task_performance", "nopsai.get_monitoring_trigger_analytics", "nopsai.get_monitoring_external_trigger_analytics", "nopsai.get_monitoring_ai_usage", "nopsai.get_monitoring_reliability", "nopsai.get_monitoring_efficiency", "nopsai.get_monitoring_security", "nopsai.get_monitoring_runner_history", "nopsai.get_monitoring_schedule_ai_usage", "nopsai.get_monitoring_schedule_performance", "nopsai.get_monitoring_trigger_performance", "nopsai.get_pipeline_efficiency", "nopsai.compare_pipelines", "nopsai.compare_schedules", "nopsai.explain_pipeline_health", "nopsai.find_optimization_opportunities", "nopsai.list_monitoring_views", "nopsai.create_monitoring_view", "nopsai.update_monitoring_view", "nopsai.delete_monitoring_view", "nopsai.list_monitoring_alert_rules", "nopsai.create_monitoring_alert_rule", "nopsai.update_monitoring_alert_rule", "nopsai.delete_monitoring_alert_rule", "nopsai.evaluate_monitoring_alert_rule", "nopsai.list_monitoring_alert_events", "nopsai.list_monitoring_recommendations", "nopsai.acknowledge_monitoring_recommendation", "nopsai.resolve_monitoring_recommendation":
		return []string{"nopsai://statistics"}
	case "nopsai.get_llm_profiles":
		return []string{"nopsai://system/llm-profiles"}
	case "nopsai.get_mcp_profiles":
		return []string{"nopsai://system/mcp-profiles"}
	case "nopsai.get_feature_capabilities":
		return []string{"nopsai://features"}
	case "nopsai.call_api":
		return []string{"nopsai://features"}
	case "nopsai.get_setup_status", "nopsai.get_setup_preflight", "nopsai.get_setup_templates", "nopsai.plan_first_install_setup", "nopsai.bootstrap_first_install_setup":
		return []string{"nopsai://system/status"}
	case "nopsai.propose_reusable_step_create", "nopsai.propose_reusable_step_update", "nopsai.propose_reusable_step_delete":
		return []string{"nopsai://pipelines"}
	case "nopsai.get_config_sync_status", "nopsai.sync_system_config", "nopsai.get_config_repo", "nopsai.get_config_repo_drift", "nopsai.sync_config_repo", "nopsai.write_config_repo", "nopsai.list_config_repos", "nopsai.sync_all_config_repos", "nopsai.get_notification_mail_settings", "nopsai.propose_notification_mail_settings", "nopsai.test_notification_mail_settings", "nopsai.get_notification_route", "nopsai.propose_notification_route_update", "nopsai.propose_notification_route_delete", "nopsai.list_data_backups", "nopsai.create_data_backup", "nopsai.delete_data_backup", "nopsai.preview_data_cleanup", "nopsai.run_data_cleanup", "nopsai.list_data_cleanup_jobs", "nopsai.list_data_cleanup_schedules", "nopsai.create_data_cleanup_schedule", "nopsai.update_data_cleanup_schedule", "nopsai.delete_data_cleanup_schedule", "nopsai.run_data_cleanup_schedule", "nopsai.enable_data_cleanup_schedule", "nopsai.disable_data_cleanup_schedule":
		return []string{"nopsai://system/status"}
	case "nopsai.list_credentials_metadata", "nopsai.get_credential_metadata", "nopsai.create_credential", "nopsai.rotate_credential_value", "nopsai.activate_credential_version", "nopsai.disable_credential", "nopsai.enable_credential", "nopsai.delete_credential_version", "nopsai.delete_credential", "nopsai.propose_credential_gitops":
		return []string{"nopsai://system/status"}
	case "nopsai.generate_runner_compose", "nopsai.generate_kubernetes_runner_manifest", "nopsai.generate_runner_bootstrap_command", "nopsai.generate_kubernetes_runner_bootstrap_command", "nopsai.update_runner_dispatch", "nopsai.eject_runner":
		return []string{"nopsai://system/dispatcher"}
	case "nopsai.list_access_grants", "nopsai.create_access_grant", "nopsai.delete_access_grant", "nopsai.get_effective_permissions", "nopsai.check_resource_use", "nopsai.batch_check_resource_use", "nopsai.get_resource_access", "nopsai.update_resource_access", "nopsai.create_resource_use_grant", "nopsai.delete_resource_access_grant", "nopsai.list_audit_logs", "nopsai.list_admin_users", "nopsai.create_admin_user", "nopsai.update_admin_user", "nopsai.delete_admin_user", "nopsai.list_admin_service_accounts", "nopsai.create_admin_service_account", "nopsai.update_admin_service_account", "nopsai.delete_admin_service_account", "nopsai.list_admin_roles", "nopsai.create_admin_role", "nopsai.delete_admin_role", "nopsai.list_admin_identity_providers", "nopsai.update_admin_identity_provider":
		return []string{"nopsai://features"}
	case "nopsai.get_ui_context":
		return []string{"nopsai://features"}
	case "nopsai.get_system_status":
		return []string{"nopsai://system/status"}
	case "nopsai.get_dispatcher_status":
		return []string{"nopsai://system/dispatcher"}
	default:
		return []string{}
	}
}

func assistantFirstPlanDenial(toolCalls []assistantToolActivity) (assistantToolActivity, bool) {
	for _, call := range toolCalls {
		if call.Name == "nopsai.assistant_plan" && call.Status == assistantToolStatusDenied {
			return call, true
		}
	}
	return assistantToolActivity{}, false
}

func assistantPlannerFailureReason(toolCalls []assistantToolActivity) string {
	for _, call := range toolCalls {
		if call.Name != assistantLLMPlannerToolName || call.Status == assistantToolStatusSuccess {
			continue
		}
		reason := strings.TrimSpace(assistantOutputString(call.Output, "fallback_reason"))
		if reason != "" {
			return reason
		}
	}
	return ""
}

func (a *App) assistantPlannerFailureReply(ctx context.Context, subject model.Subject, content string, plan assistantTurnPlan, toolCalls []assistantToolActivity) string {
	reason := assistantPlannerFailureReason(toolCalls)
	if assistantPlannerFailureIsSchemaSubset(reason) {
		if reply := a.assistantSchemaSubsetFailureReply(ctx, subject, content, plan); reply != "" {
			return reply
		}
		return "I did not run that because the requested tool plan did not match an available safe tool for this request. For estimates or calculations, I can use existing same-chat MCP evidence with a clearly labeled LLM-derived estimate, or I can inspect the relevant NopsAI data first. No changes were applied."
	}
	reply := "I could not create a validated NopsAI tool plan for that request because the assistant LLM planner was unavailable or returned an invalid plan. No changes were applied."
	if reason != "" {
		reply = "I could not create a validated NopsAI tool plan for that request because the assistant LLM planner was unavailable or returned an invalid plan: " + reason + ". No changes were applied."
	}
	return reply
}

func assistantPlannerFailureIsSchemaSubset(reason string) bool {
	reason = strings.ToLower(strings.TrimSpace(reason))
	return strings.Contains(reason, "schema_tools") && strings.Contains(reason, "selected")
}

func (a *App) assistantSchemaSubsetFailureReply(ctx context.Context, subject model.Subject, content string, plan assistantTurnPlan) string {
	if strings.TrimSpace(plan.LowerContent) == "" {
		plan = assistantBaseTurnPlan(content, assistantConversationMemory{})
	}
	lower := strings.ToLower(strings.TrimSpace(content))
	if lower == "" {
		lower = plan.LowerContent
	}
	if assistantPlannerWantsExposurePolicySchema(lower) ||
		(assistantTextHasAny(lower, "policy", "policies", "guardrail", "guardrails") &&
			assistantTextHasAny(lower, "knowledge", "docs", "documentation")) {
		return "I could not validate the read plan for that policy question. This should be answered from permission-bound feature capabilities or knowledge context search, using metadata only for variables/secrets and no plaintext values. No changes were applied."
	}
	if assistantPlannerWantsGitOpsProposalSchema(lower) || !assistantPlannerWantsChangeSchema(lower) {
		return ""
	}
	kind := ""
	switch {
	case assistantTextHasAny(lower, "secret", "secrets"):
		kind = "secret"
	case assistantTextHasAny(lower, "env var", "environment variable", "variable", "variables", "var ", "_var"):
		kind = "variable"
	default:
		return ""
	}
	action := "write"
	if assistantPlannerWantsDeleteSchema(lower) {
		action = "delete"
	}
	directTool := "nopsai.write_" + kind + "_value"
	if action == "delete" {
		directTool = "nopsai.delete_" + kind + "_value"
	}
	availableTools := a.hostedMCPToolsForSubject(ctx, subject)
	schemaToolNames := assistantPlannerSchemaToolNames(assistantPlannerSchemaContext(assistantConversation{}, content, plan.PageContext), plan, nil, availableTools)
	if schemaToolNames[directTool] {
		return fmt.Sprintf("I did not use GitOps because you did not ask for a GitOps proposal. This should be a direct MCP %s %s, and NopsAI requires explicit confirmation before applying it. Please confirm the direct %s %s with the name, value, and scope. No changes were applied.", kind, action, kind, action)
	}
	return fmt.Sprintf("I did not use GitOps because you did not ask for a GitOps proposal. The direct MCP %s %s tool is not available in this session, likely because assistant action execution or AAA permission is disabled. Enable direct action execution/permission, or explicitly ask for a GitOps proposal if that is the workflow you want. No changes were applied.", kind, action)
}

func composeAssistantPlanDeniedReply(call assistantToolActivity) string {
	reason := strings.TrimSpace(assistantOutputString(call.Output, "error"))
	if reason == "" {
		reason = "the requested plan did not pass NopsAI safety and permission validation"
	}
	return "I could not safely execute that assistant plan: " + reason + ". No changes were applied."
}

func assistantFirstUUID(content string) string {
	return assistantFirstPatternTeam(assistantUUIDPattern, content)
}

func assistantFirstPatternTeam(pattern *regexp.Regexp, content string) string {
	match := pattern.FindStringSubmatch(content)
	if len(match) == 0 {
		return ""
	}
	if len(match) == 1 {
		return strings.TrimSpace(match[0])
	}
	return strings.TrimSpace(match[1])
}

func assistantYAMLFromMessage(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if strings.Contains(content, "```") {
		parts := strings.Split(content, "```")
		for _, part := range parts {
			candidate := strings.TrimSpace(part)
			if candidate == "" {
				continue
			}
			lines := strings.Split(candidate, "\n")
			if len(lines) > 1 && !strings.Contains(lines[0], ":") {
				candidate = strings.TrimSpace(strings.Join(lines[1:], "\n"))
			}
			if assistantLooksLikePipelineYAML(candidate) {
				return candidate
			}
		}
	}
	if assistantLooksLikePipelineYAML(content) {
		return content
	}
	return ""
}

func assistantLooksLikePipelineYAML(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "name:") && strings.Contains(lower, "steps:")
}

func assistantPipelineIDFromMessage(content string) string {
	id := assistantFirstPatternTeam(assistantPipelineIDPattern, content)
	switch strings.ToLower(strings.Trim(strings.TrimSpace(id), "/")) {
	case "", "a", "an", "the", "that", "which", "who", "where", "has", "have", "having", "with", "through", "via", "yaml", "context", "knowledge", "runs", "run", "logs", "called", "named", "name", "approval", "step", "steps", "use", "uses", "using", "highest", "llm", "tokens", "gonna", "going", "will", "would", "should", "must", "can", "could", "to", "build", "deploy":
		return ""
	default:
		return strings.Trim(id, "/")
	}
}

func assistantScopeFromMessage(content string) string {
	scope := strings.Trim(strings.TrimSpace(assistantFirstPatternTeam(assistantScopePattern, content)), "/")
	scope = strings.Trim(scope, ".,;:!?\"'")
	if assistantScopeCandidateIsGrammar(scope) {
		return ""
	}
	return scope
}

func assistantScopeCandidateIsGrammar(scope string) bool {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", "a", "an", "the", "do", "does", "did", "we", "i", "you", "they", "have", "has", "having", "is", "are", "was", "were", "for", "each", "every", "all", "many", "much", "count", "counts", "total", "secret", "secrets", "variable", "variables", "permission", "permissions", "access":
		return true
	default:
		return false
	}
}

func assistantLastRelevantLogLine(logs []map[string]any) string {
	for idx := len(logs) - 1; idx >= 0; idx-- {
		line := assistantOutputString(logs[idx], "line")
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if containsAny(lower, "error", "failed", "failure", "fatal", "panic", "exception", "denied", "timeout", "invalid") {
			return line
		}
	}
	if len(logs) == 0 {
		return ""
	}
	return assistantOutputString(logs[len(logs)-1], "line")
}

func assistantCapabilityPolicyNotes(output map[string]any) []string {
	notes := []string{}
	seen := map[string]struct{}{}
	for _, area := range assistantMapSlice(output["areas"]) {
		for _, note := range assistantStringSlice(area["notes"]) {
			lower := strings.ToLower(note)
			if !containsAny(lower, "secret", "credential", "sensitive", "plaintext", "redact", "blocked", "policy") {
				continue
			}
			if _, ok := seen[note]; ok {
				continue
			}
			seen[note] = struct{}{}
			notes = append(notes, note)
			if len(notes) >= 3 {
				return notes
			}
		}
	}
	return notes
}

func assistantPipelineNameFromMessage(content string) string {
	name := assistantFirstPatternTeam(assistantPipelineNamePattern, content)
	if name == "" {
		return ""
	}
	return strings.Trim(strings.ToLower(name), "/")
}

func assistantAPICallFromMessage(content string) (string, string) {
	match := assistantAPICallPattern.FindStringSubmatch(content)
	if len(match) < 3 {
		return "", ""
	}
	method := strings.ToUpper(strings.TrimSpace(match[1]))
	path := strings.Trim(strings.TrimSpace(match[2]), "`\"'")
	return method, path
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func assistantOutputString(output map[string]any, key string) string {
	if len(output) == 0 {
		return ""
	}
	value, ok := output[key]
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

func assistantOutputBool(output map[string]any, key string) bool {
	value, ok := output[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func assistantOutputFloat(output map[string]any, key string) float64 {
	value, ok := output[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case string:
		var parsed float64
		_, _ = fmt.Sscanf(typed, "%f", &parsed)
		return parsed
	default:
		return 0
	}
}

func assistantStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return normalizeAssistantStringList(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, strings.TrimSpace(fmt.Sprint(item)))
		}
		return normalizeAssistantStringList(values)
	default:
		return []string{}
	}
}

func assistantMapSlice(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		values := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				values = append(values, mapped)
			}
		}
		return values
	default:
		return []map[string]any{}
	}
}

func assistantSecretScopeRows(output map[string]any) []map[string]any {
	if rows := assistantMapSlice(output["response"]); len(rows) > 0 {
		return rows
	}
	if rows := assistantMapSlice(output["scopes"]); len(rows) > 0 {
		return rows
	}
	return assistantMapSlice(output["secret_scopes"])
}

func assistantRuntimeRunnerRows(value any) []monitoringRunnerStatus {
	switch typed := value.(type) {
	case []monitoringRunnerStatus:
		return typed
	case []any:
		rows := make([]monitoringRunnerStatus, 0, len(typed))
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok {
				continue
			}
			rows = append(rows, monitoringRunnerStatus{
				RunnerID:     assistantOutputString(mapped, "runner_id"),
				Status:       assistantOutputString(mapped, "status"),
				Runtime:      assistantOutputString(mapped, "runtime"),
				Capacity:     int32(assistantOutputFloat(mapped, "capacity")),
				ActiveJobs:   int32(assistantOutputFloat(mapped, "active_jobs")),
				InflightJobs: int32(assistantOutputFloat(mapped, "inflight_jobs")),
			})
		}
		return rows
	default:
		return []monitoringRunnerStatus{}
	}
}

func assistantJoinOrNone(values []string) string {
	values = normalizeAssistantStringList(values)
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

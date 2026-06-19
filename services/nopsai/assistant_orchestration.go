package nopsai

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

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
	plan := assistantPlanFromMessage(content, conversation.Memory)
	memory := assistantMemoryForTurn(conversation, plan)
	toolCalls := []assistantToolActivity{}
	runTool := func(name string, args map[string]any) assistantToolActivity {
		call := a.runAssistantHostedMCPTool(ctx, subject, userID, conversation.ID, name, args)
		toolCalls = append(toolCalls, call)
		return call
	}

	switch plan.Intent {
	case "generate_pipeline":
		generated := runTool("nopsai.generate_pipeline", map[string]any{
			"name":  plan.PipelineName,
			"goal":  content,
			"scope": plan.Scope,
		})
		if yaml := assistantOutputString(generated.Output, "yaml"); yaml != "" {
			runTool("nopsai.validate_pipeline", map[string]any{"yaml": yaml})
			memory.PreviousProposedFixes = append(memory.PreviousProposedFixes, "Generated pipeline YAML draft for "+plan.PipelineName)
			memory.OpenTasks = append(memory.OpenTasks, "Review generated pipeline YAML through GitOps before applying it.")
		}
	case "propose_pipeline_create":
		call := runTool("nopsai.propose_pipeline_create", map[string]any{
			"pipeline": plan.PipelineID,
			"yaml":     plan.YAML,
		})
		if assistantOutputBool(call.Output, "valid") {
			memory.PreviousProposedFixes = append(memory.PreviousProposedFixes, "Prepared GitOps create plan for "+assistantOutputString(call.Output, "pipeline_id"))
			memory.OpenTasks = append(memory.OpenTasks, "Commit the generated pipeline file through the config repository review branch.")
		}
	case "propose_pipeline_update":
		call := runTool("nopsai.propose_pipeline_update", map[string]any{
			"pipeline": plan.PipelineID,
			"yaml":     plan.YAML,
		})
		if assistantOutputBool(call.Output, "valid") {
			memory.PreviousProposedFixes = append(memory.PreviousProposedFixes, "Prepared GitOps update plan for "+assistantOutputString(call.Output, "pipeline_id"))
			memory.OpenTasks = append(memory.OpenTasks, "Commit the generated pipeline file through the config repository review branch.")
		}
	case "validate_pipeline":
		runTool("nopsai.validate_pipeline", map[string]any{"yaml": plan.YAML})
	case "analyze_run":
		runTool("nopsai.analyze_pipeline_run_failure", map[string]any{"run_id": plan.RunID})
		memory.OpenTasks = append(memory.OpenTasks, "Review failed run analysis before applying remediation.")
	case "list_runs":
		runTool("nopsai.list_pipeline_runs", map[string]any{"limit": 5})
	case "cost":
		runTool("nopsai.get_cost_summary", map[string]any{})
		runTool("nopsai.suggest_cost_improvements", map[string]any{})
	case "design":
		runTool("nopsai.suggest_design_improvements", map[string]any{})
	case "statistics":
		runTool("nopsai.get_statistics", map[string]any{})
	case "trigger":
		if plan.Repository != "" && containsAny(plan.LowerContent, "change", "update", "modify", "propose") {
			runTool("nopsai.propose_trigger_change", map[string]any{"repository": plan.Repository, "change": content})
			memory.PreviousProposedFixes = append(memory.PreviousProposedFixes, "Draft trigger change for "+plan.Repository)
		} else if plan.Repository != "" {
			runTool("nopsai.get_trigger", map[string]any{"repository": plan.Repository})
		} else {
			runTool("nopsai.list_triggers", map[string]any{"limit": 20})
		}
	case "schedule":
		if plan.ScheduleID != "" && containsAny(plan.LowerContent, "change", "update", "modify", "propose") {
			runTool("nopsai.propose_schedule_change", map[string]any{"schedule_id": plan.ScheduleID, "change": content})
			memory.PreviousProposedFixes = append(memory.PreviousProposedFixes, "Draft schedule change for "+plan.ScheduleID)
		} else if plan.ScheduleID != "" {
			runTool("nopsai.get_schedule", map[string]any{"schedule_id": plan.ScheduleID})
		} else {
			runTool("nopsai.list_schedules", map[string]any{"limit": 20})
		}
	case "scope":
		if plan.Scope != "" {
			runTool("nopsai.explain_scope_permissions", map[string]any{"scope": plan.Scope})
		} else {
			runTool("nopsai.list_scopes", map[string]any{"limit": 20})
		}
	case "api_call":
		runTool("nopsai.call_api", map[string]any{"method": plan.APIMethod, "path": plan.APIPath, "confirm": containsAny(plan.LowerContent, "confirm", "confirmed", "execute", "apply")})
	case "search_pipelines":
		query := plan.SearchQuery
		if query == "" {
			query = content
		}
		runTool("nopsai.search_pipelines", map[string]any{"query": query, "limit": 20, "include_snippets": true})
	case "pipeline_knowledge_context":
		if plan.YAML != "" {
			runTool("nopsai.get_pipeline_knowledge_context", map[string]any{"yaml": plan.YAML, "include_content": true})
		} else if plan.PipelineID != "" {
			runTool("nopsai.get_pipeline_knowledge_context", map[string]any{"pipeline": plan.PipelineID, "include_content": true})
		} else {
			runTool("nopsai.list_knowledge_contexts", map[string]any{"query": content, "limit": 20})
		}
	case "knowledge_context":
		runTool("nopsai.list_knowledge_contexts", map[string]any{"query": content, "limit": 20})
	case "pipeline":
		if plan.PipelineID != "" {
			runTool("nopsai.get_pipeline", map[string]any{"pipeline": plan.PipelineID})
		} else {
			runTool("nopsai.list_pipelines", map[string]any{"limit": 20})
		}
	case "profiles":
		runTool("nopsai.get_llm_profiles", map[string]any{})
		runTool("nopsai.get_mcp_profiles", map[string]any{})
	case "feature_capabilities":
		runTool("nopsai.get_feature_capabilities", map[string]any{"query": content, "include_api_routes": false})
	case "runtime":
		runTool("nopsai.get_dispatcher_status", map[string]any{})
	case "system":
		runTool("nopsai.get_system_status", map[string]any{})
	default:
		runTool("nopsai.search_docs", map[string]any{"query": content, "limit": 5})
	}

	memory = assistantMemoryAfterTools(memory, plan, toolCalls)
	reply := composeAssistantReply(plan, selectedProfile, toolCalls)
	synthesis := a.synthesizeAssistantReplyWithLLM(ctx, conversation, content, selectedProfile, plan, toolCalls, reply)
	if synthesis.Activity != nil {
		toolCalls = append(toolCalls, *synthesis.Activity)
		reply = synthesis.Reply
	}
	return assistantOrchestrationResult{
		Reply:     reply,
		ToolCalls: toolCalls,
		Memory:    normalizeAssistantMemory(memory),
	}
}

type assistantTurnPlan struct {
	Intent       string
	LowerContent string
	RunID        string
	YAML         string
	PipelineName string
	PipelineID   string
	Repository   string
	ScheduleID   string
	Scope        string
	SearchQuery  string
	APIMethod    string
	APIPath      string
}

func assistantPlanFromMessage(content string, memory assistantConversationMemory) assistantTurnPlan {
	content = strings.TrimSpace(content)
	lower := strings.ToLower(content)
	plan := assistantTurnPlan{
		Intent:       "docs",
		LowerContent: lower,
		RunID:        assistantFirstUUID(content),
		YAML:         assistantYAMLFromMessage(content),
		PipelineName: assistantPipelineNameFromMessage(content),
		PipelineID:   assistantPipelineIDFromMessage(content),
		Repository:   assistantFirstPatternGroup(assistantRepositoryPattern, content),
		ScheduleID:   assistantFirstPatternGroup(assistantScheduleIDPattern, content),
		Scope:        strings.Trim(assistantFirstPatternGroup(assistantScopePattern, content), "/"),
		SearchQuery:  assistantPipelineSearchQueryFromMessage(content),
	}
	plan.APIMethod, plan.APIPath = assistantAPICallFromMessage(content)
	if plan.RunID == "" {
		plan.RunID = strings.TrimSpace(memory.SelectedRun)
	}
	if plan.PipelineID == "" {
		plan.PipelineID = strings.Trim(strings.TrimSpace(memory.SelectedPipeline), "/")
	}
	if plan.Scope == "" {
		plan.Scope = strings.Trim(strings.TrimSpace(memory.SelectedScope), "/")
	}
	if plan.PipelineName == "" {
		plan.PipelineName = "generated-pipeline"
	}

	switch {
	case plan.YAML != "" && containsAny(lower, "update pipeline", "replace pipeline", "modify pipeline", "save pipeline", "write pipeline", "gitops", "review branch", "commit"):
		if containsAny(lower, "update", "replace", "modify", "existing") {
			plan.Intent = "propose_pipeline_update"
		} else {
			plan.Intent = "propose_pipeline_create"
		}
	case plan.YAML != "" && containsAny(lower, "create pipeline", "new pipeline"):
		plan.Intent = "propose_pipeline_create"
	case plan.YAML != "" && (containsAny(lower, "validate", "lint", "check") || !containsAny(lower, "generate", "create", "draft", "new pipeline")):
		plan.Intent = "validate_pipeline"
	case containsAny(lower, "generate pipeline", "create pipeline", "draft pipeline", "pipeline yaml", "new pipeline"):
		plan.Intent = "generate_pipeline"
	case plan.RunID != "" && containsAny(lower, "run", "failed", "failure", "fail", "log", "logs", "analyze", "explain", "why"):
		plan.Intent = "analyze_run"
	case containsAny(lower, "failed run", "failure", "run failed", "recent runs", "pipeline runs", "logs"):
		plan.Intent = "list_runs"
	case containsAny(lower, "cost", "usage", "spend", "budget", "expensive"):
		plan.Intent = "cost"
	case containsAny(lower, "design", "improve", "best practice", "maintenance", "refactor", "architecture"):
		plan.Intent = "design"
	case containsAny(lower, "statistics", "stats", "overview", "dashboard"):
		plan.Intent = "statistics"
	case plan.APIMethod != "" && containsAny(lower, "api", "/v1/"):
		plan.Intent = "api_call"
	case containsAny(lower, "trigger", "webhook"):
		plan.Intent = "trigger"
	case containsAny(lower, "schedule", "cron"):
		plan.Intent = "schedule"
	case containsAny(lower, "mcp coverage", "mcp capability", "mcp capabilities", "mcp support", "hosted mcp", "support all with mcp", "features with mcp", "feature coverage"):
		plan.Intent = "feature_capabilities"
	case containsAny(lower, "scope", "permission", "access grant", "access"):
		plan.Intent = "scope"
	case containsAny(lower, "search pipeline", "search pipelines", "find pipeline", "find pipelines", "look for pipeline", "look through pipeline", "search through pipeline"):
		plan.Intent = "search_pipelines"
	case containsAny(lower, "knowledge context", "knowledge doc", "runbook", "guardrail", "guideline", "adr"):
		if containsAny(lower, "pipeline") || plan.PipelineID != "" || plan.YAML != "" {
			plan.Intent = "pipeline_knowledge_context"
		} else {
			plan.Intent = "knowledge_context"
		}
	case containsAny(lower, "pipeline"):
		plan.Intent = "pipeline"
	case containsAny(lower, "llm profile", "mcp profile", "profiles", "models"):
		plan.Intent = "profiles"
	case containsAny(lower, "dispatcher", "runner", "runners", "runtime"):
		plan.Intent = "runtime"
	case containsAny(lower, "system status", "health"):
		plan.Intent = "system"
	default:
		plan.Intent = "docs"
	}
	return plan
}

func (a *App) runAssistantHostedMCPTool(ctx context.Context, subject model.Subject, userID string, conversationID uuid.UUID, name string, args map[string]any) assistantToolActivity {
	if args == nil {
		args = map[string]any{}
	}
	tool, ok := a.hostedMCPToolByName(ctx, subject, name)
	if !ok {
		return assistantToolActivity{
			Name:   name,
			Input:  args,
			Output: map[string]any{"error": "tool is not available for the current subject"},
			Status: assistantToolStatusDenied,
		}
	}
	result, err := a.callHostedMCPTool(ctx, subject, userID, tool, args, &conversationID)
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
	return assistantToolActivity{
		Name:         name,
		Input:        args,
		Output:       result,
		Status:       status,
		ResourceURIs: assistantResourceURIsForTool(name),
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
		case "nopsai.generate_pipeline":
			if plan.PipelineName != "" {
				memory.SelectedPipeline = plan.PipelineName
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
	case "generate_pipeline":
		return "Drafting a GitOps-safe pipeline proposal."
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
	if len(toolCalls) == 0 {
		return buildAssistantFoundationReply(selectedProfile)
	}
	if assistantAllToolsDenied(toolCalls) {
		return "I could not use the required Nopsai tools with your current permissions. No changes were applied."
	}
	switch plan.Intent {
	case "generate_pipeline":
		return composePipelineGenerationReply(toolCalls)
	case "propose_pipeline_create", "propose_pipeline_update":
		return composePipelineWritePlanReply(toolCalls)
	case "validate_pipeline":
		return composePipelineValidationReply(toolCalls)
	case "analyze_run":
		return composeRunAnalysisReply(toolCalls)
	case "list_runs":
		return composeRecentRunsReply(toolCalls)
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
		return composeDocsReply(toolCalls)
	}
}

func composePipelineGenerationReply(toolCalls []assistantToolActivity) string {
	var yaml string
	validation := map[string]any{}
	for _, call := range toolCalls {
		switch call.Name {
		case "nopsai.generate_pipeline":
			yaml = assistantOutputString(call.Output, "yaml")
		case "nopsai.validate_pipeline":
			validation = call.Output
		}
	}
	lines := []string{"I drafted a GitOps-safe pipeline proposal. No changes were applied."}
	if len(validation) > 0 {
		if assistantOutputBool(validation, "valid") {
			lines = append(lines, "", "Validation: passed.")
		} else {
			lines = append(lines, "", "Validation: failed.")
			if errText := assistantOutputString(validation, "error"); errText != "" {
				lines = append(lines, "Issue: "+errText)
			}
		}
	}
	if yaml != "" {
		lines = append(lines, "", "Draft YAML:", "```yaml", strings.TrimSpace(yaml), "```")
	}
	lines = append(lines, "", "Apply it by committing the reviewed YAML through the GitOps configuration repository.")
	return strings.Join(lines, "\n")
}

func composePipelineWritePlanReply(toolCalls []assistantToolActivity) string {
	call := assistantFirstNonEmptyToolCall(toolCalls)
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
	call := assistantFirstToolCall(toolCalls, "nopsai.analyze_pipeline_run_failure")
	if call.Status != assistantToolStatusSuccess {
		return assistantToolErrorReply("I could not analyze that run.", call)
	}
	run, _ := call.Output["run"].(map[string]any)
	if run == nil {
		run = map[string]any{}
	}
	lines := []string{"I analyzed the run using status and recent logs."}
	if runID := assistantOutputString(run, "run_id"); runID != "" {
		lines = append(lines, "", "Run: "+runID)
	}
	if pipelineID := assistantOutputString(run, "pipeline_id"); pipelineID != "" {
		lines = append(lines, "Pipeline: "+pipelineID)
	}
	if status := assistantOutputString(run, "status"); status != "" {
		lines = append(lines, "Status: "+status)
	}
	if hint := assistantOutputString(call.Output, "root_cause_hint"); hint != "" {
		lines = append(lines, "", "Root-cause hint: "+hint)
	}
	if steps := assistantStringSlice(call.Output["suggested_next_steps"]); len(steps) > 0 {
		lines = append(lines, "", "Suggested next steps:")
		for _, step := range steps {
			lines = append(lines, "- "+step)
		}
	}
	lines = append(lines, "", "No changes were applied.")
	return strings.Join(lines, "\n")
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
		return "I did not find matching pipelines visible to your account."
	}
	lines := []string{"Matching pipelines:"}
	for _, pipeline := range pipelines {
		line := "- " + assistantOutputString(pipeline, "id")
		if fields := assistantStringSlice(pipeline["match_fields"]); len(fields) > 0 {
			line += " (matched " + strings.Join(fields, ", ") + ")"
		}
		lines = append(lines, line)
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
			"- Runners: %d total, %d online, %d stale, %d disabled, capacity %d, active jobs %d, queued jobs %d",
			summary.Total,
			summary.Online,
			summary.Stale,
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
		if group := assistantOutputString(doc, "group_path"); group != "" {
			line += " (" + group + ")"
		}
		if desc := assistantOutputString(doc, "description"); desc != "" {
			line += ": " + desc
		}
		lines = append(lines, line)
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
	case "nopsai.list_pipelines", "nopsai.search_pipelines", "nopsai.get_pipeline", "nopsai.validate_pipeline", "nopsai.generate_pipeline", "nopsai.propose_pipeline_create", "nopsai.propose_pipeline_update":
		return []string{"nopsai://pipelines"}
	case "nopsai.get_pipeline_knowledge_context":
		return []string{"nopsai://pipelines", "nopsai://docs"}
	case "nopsai.run_pipeline", "nopsai.list_pipeline_runs", "nopsai.get_pipeline_run", "nopsai.get_pipeline_run_logs", "nopsai.analyze_pipeline_run_failure", "nopsai.list_run_approvals", "nopsai.approve_run_approval", "nopsai.reject_run_approval", "nopsai.rerun_pipeline_run", "nopsai.cancel_pipeline_run", "nopsai.delete_pipeline_run", "nopsai.list_lab_items", "nopsai.get_lab_item", "nopsai.explain_lab_result":
		return []string{"nopsai://pipeline-runs"}
	case "nopsai.list_triggers", "nopsai.get_trigger", "nopsai.propose_trigger_change", "nopsai.list_git_webhook_sources", "nopsai.get_git_webhook_source", "nopsai.list_git_webhook_deliveries", "nopsai.propose_git_webhook_source_create", "nopsai.propose_git_webhook_source_update", "nopsai.propose_git_webhook_source_delete", "nopsai.list_external_triggers", "nopsai.get_external_trigger", "nopsai.list_external_trigger_invocations", "nopsai.propose_external_trigger_create", "nopsai.propose_external_trigger_update", "nopsai.propose_external_trigger_delete", "nopsai.invoke_external_trigger":
		return []string{"nopsai://triggers"}
	case "nopsai.list_schedules", "nopsai.get_schedule", "nopsai.propose_schedule_change", "nopsai.propose_schedule_create", "nopsai.propose_schedule_update", "nopsai.propose_schedule_delete", "nopsai.propose_schedule_enable", "nopsai.propose_schedule_disable", "nopsai.run_schedule_now":
		return []string{"nopsai://schedules"}
	case "nopsai.list_scopes", "nopsai.get_scope", "nopsai.explain_scope_permissions":
		return []string{"nopsai://scopes"}
	case "nopsai.get_cost_summary", "nopsai.suggest_cost_improvements":
		return []string{"nopsai://costs"}
	case "nopsai.list_monitoring_views", "nopsai.create_monitoring_view", "nopsai.update_monitoring_view", "nopsai.delete_monitoring_view", "nopsai.list_monitoring_alert_rules", "nopsai.create_monitoring_alert_rule", "nopsai.update_monitoring_alert_rule", "nopsai.delete_monitoring_alert_rule", "nopsai.evaluate_monitoring_alert_rule", "nopsai.list_monitoring_alert_events", "nopsai.list_monitoring_recommendations", "nopsai.acknowledge_monitoring_recommendation", "nopsai.resolve_monitoring_recommendation":
		return []string{"nopsai://statistics"}
	case "nopsai.get_llm_profiles":
		return []string{"nopsai://system/llm-profiles"}
	case "nopsai.get_mcp_profiles":
		return []string{"nopsai://system/mcp-profiles"}
	case "nopsai.get_feature_capabilities":
		return []string{"nopsai://features"}
	case "nopsai.call_api":
		return []string{"nopsai://features"}
	case "nopsai.get_config_sync_status", "nopsai.sync_system_config", "nopsai.get_config_repo", "nopsai.get_config_repo_drift", "nopsai.sync_config_repo", "nopsai.write_config_repo", "nopsai.list_config_repos", "nopsai.sync_all_config_repos", "nopsai.get_notification_mail_settings", "nopsai.propose_notification_mail_settings", "nopsai.test_notification_mail_settings", "nopsai.get_notification_route", "nopsai.propose_notification_route_update", "nopsai.propose_notification_route_delete", "nopsai.list_data_backups", "nopsai.create_data_backup", "nopsai.delete_data_backup", "nopsai.preview_data_cleanup", "nopsai.run_data_cleanup", "nopsai.list_data_cleanup_jobs", "nopsai.list_data_cleanup_schedules", "nopsai.create_data_cleanup_schedule", "nopsai.update_data_cleanup_schedule", "nopsai.delete_data_cleanup_schedule", "nopsai.run_data_cleanup_schedule", "nopsai.enable_data_cleanup_schedule", "nopsai.disable_data_cleanup_schedule":
		return []string{"nopsai://system/status"}
	case "nopsai.get_system_status":
		return []string{"nopsai://system/status"}
	case "nopsai.get_dispatcher_status":
		return []string{"nopsai://system/dispatcher"}
	default:
		return []string{}
	}
}

func assistantFirstUUID(content string) string {
	return assistantFirstPatternGroup(assistantUUIDPattern, content)
}

func assistantFirstPatternGroup(pattern *regexp.Regexp, content string) string {
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
	id := assistantFirstPatternGroup(assistantPipelineIDPattern, content)
	switch strings.ToLower(strings.Trim(strings.TrimSpace(id), "/")) {
	case "", "through", "via", "with", "yaml", "context", "knowledge", "runs", "run", "logs", "called", "named", "name":
		return ""
	default:
		return strings.Trim(id, "/")
	}
}

func assistantPipelineSearchQueryFromMessage(content string) string {
	query := assistantFirstPatternGroup(assistantPipelineSearchPattern, content)
	query = strings.TrimSpace(query)
	query = strings.Trim(query, "`\"' ")
	if strings.EqualFold(query, "now") || strings.EqualFold(query, "please") {
		return ""
	}
	return query
}

func assistantPipelineNameFromMessage(content string) string {
	name := assistantFirstPatternGroup(assistantPipelineNamePattern, content)
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

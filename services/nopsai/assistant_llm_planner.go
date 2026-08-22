package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"nopsai/config"
	"nopsai/pkg/llmclient"
	aaamodel "nopsai/services/aaa/pkg/model"
)

const (
	assistantLLMPlannerToolName    = "nopsai.llm.plan"
	assistantMaxPlannerIterations  = 4
	assistantMaxPlannerAttempts    = 2
	assistantMaxPlannerSchemaTools = 18

	// How many tools relevance alone may contribute. Structural context and mode
	// policy add on top of this; the rest of the budget stays for them.
	assistantPlannerMaxRoutedTools = 6

	// Discovery needs the purpose of a tool, not its full argument prose.
	assistantPlannerCatalogDescriptionLimit = 140
)

type assistantPlannerResult struct {
	Plan          assistantTurnPlan
	ToolCalls     []assistantToolActivity
	Handled       bool
	SkipSynthesis bool
}

type assistantPlannerDecision struct {
	Goal               string                 `json:"goal"`
	Intent             string                 `json:"intent"`
	SuccessCriteria    string                 `json:"success_criteria"`
	Steps              []assistantPlannerStep `json:"steps"`
	FinalAnswer        string                 `json:"final_answer"`
	ClarifyingQuestion string                 `json:"clarifying_question"`
	NeedsMoreTools     *bool                  `json:"needs_more_tools"`
}

type assistantPlannerStep struct {
	Tool   string         `json:"tool"`
	Args   map[string]any `json:"args"`
	Reason string         `json:"reason"`
}

func (a *App) runAssistantLLMPlannedTurn(
	ctx context.Context,
	subject aaamodel.Subject,
	userID string,
	conversation assistantConversation,
	content string,
	selectedProfile string,
) assistantPlannerResult {
	return a.runAssistantLLMPlannedTurnWithPageContext(ctx, subject, userID, conversation, content, selectedProfile, assistantPageContext{})
}

func (a *App) runAssistantLLMPlannedTurnWithPageContext(
	ctx context.Context,
	subject aaamodel.Subject,
	userID string,
	conversation assistantConversation,
	content string,
	selectedProfile string,
	pageContext assistantPageContext,
) assistantPlannerResult {
	profileName, profile, client, ok, reason := a.assistantLLMClientForTurn(ctx, conversation, selectedProfile)
	if !ok {
		if reason == "" {
			return assistantPlannerResult{}
		}
		plan := assistantBaseTurnPlanWithPageContext(content, conversation.Memory, pageContext)
		return assistantPlannerResult{
			Plan: plan,
			ToolCalls: []assistantToolActivity{*assistantLLMPlannerActivity(profileName, profile, assistantToolStatusError, map[string]any{
				"fallback_reason": reason,
			})},
			Handled: false,
		}
	}

	plan := assistantBaseTurnPlanWithPageContext(content, conversation.Memory, pageContext)
	toolCalls := []assistantToolActivity{}
	remainingToolCalls := assistantMaxPlanToolCalls
	skipSynthesis := false
	for iteration := 1; iteration <= assistantMaxPlannerIterations; iteration++ {
		decision, activities, ok := a.requestAssistantPlannerDecision(ctx, subject, conversation, content, plan, toolCalls, remainingToolCalls, iteration, profileName, profile, client)
		toolCalls = append(toolCalls, activities...)
		if !ok {
			return assistantPlannerResult{Plan: plan, ToolCalls: toolCalls, Handled: false}
		}
		plan = assistantTurnPlanFromPlannerDecision(plan, decision)
		if plan.ClarifyQuestion != "" && len(plan.Steps) == 0 {
			plan.Intent = "clarify"
			return assistantPlannerResult{Plan: plan, ToolCalls: toolCalls, Handled: true}
		}
		if plan.FinalAnswer != "" && len(plan.Steps) == 0 {
			if err := assistantValidatePlannerFinalAnswer(plan, toolCalls, conversation); err != nil {
				plan.FinalAnswer = ""
				toolCalls = append(toolCalls, assistantPlanDeniedActivity(plan, err))
				return assistantPlannerResult{Plan: plan, ToolCalls: toolCalls, Handled: true}
			}
			toolCalls = append(toolCalls, assistantExecutionPlanActivity(plan, assistantToolStatusSuccess))
			return assistantPlannerResult{Plan: plan, ToolCalls: toolCalls, Handled: true}
		}
		if len(plan.Steps) == 0 {
			plan.FinalAnswer = "I could not identify a safe NopsAI tool plan for that request. No changes were applied."
			return assistantPlannerResult{Plan: plan, ToolCalls: toolCalls, Handled: true}
		}
		if len(plan.Steps) > remainingToolCalls {
			toolCalls = append(toolCalls, assistantPlanDeniedActivity(plan, fmt.Errorf("assistant plan has %d remaining tool calls; max remaining is %d", len(plan.Steps), remainingToolCalls)))
			return assistantPlannerResult{Plan: plan, ToolCalls: toolCalls, Handled: true}
		}
		if err := a.validateAssistantToolPlan(ctx, subject, plan); err != nil {
			toolCalls = append(toolCalls, assistantPlanDeniedActivity(plan, err))
			return assistantPlannerResult{Plan: plan, ToolCalls: toolCalls, Handled: true}
		}
		toolCalls = append(toolCalls, assistantExecutionPlanActivity(plan, assistantToolStatusSuccess))
		for idx, step := range plan.Steps {
			call := a.runAssistantHostedMCPTool(ctx, subject, userID, conversation.ID, step.ToolName, cloneAssistantArgs(step.Args))
			call = assistantAnnotateToolActivityFromPlanStep(call, step, idx+1)
			toolCalls = append(toolCalls, call)
			remainingToolCalls--
			if remainingToolCalls <= 0 {
				break
			}
		}
		if assistantPlanHasTerminalEvidence(plan, toolCalls) {
			skipSynthesis = true
			break
		}
		if remainingToolCalls <= 0 {
			break
		}
		if decision.NeedsMoreTools != nil && !*decision.NeedsMoreTools {
			break
		}
	}
	plan.Steps = nil
	return assistantPlannerResult{Plan: plan, ToolCalls: toolCalls, Handled: true, SkipSynthesis: skipSynthesis}
}

func (a *App) requestAssistantPlannerDecision(
	ctx context.Context,
	subject aaamodel.Subject,
	conversation assistantConversation,
	content string,
	plan assistantTurnPlan,
	toolCalls []assistantToolActivity,
	remainingToolCalls int,
	iteration int,
	profileName string,
	profile config.LLMProfile,
	client *llmclient.Client,
) (assistantPlannerDecision, []assistantToolActivity, bool) {
	availableTools := a.hostedMCPToolsForSubject(ctx, subject)
	schemaToolNames := assistantPlannerSchemaToolNames(assistantPlannerSchemaContext(conversation, content, plan.PageContext), plan, toolCalls, availableTools)
	basePrompt := a.buildAssistantPlannerPromptWithSchemas(conversation, content, plan, toolCalls, remainingToolCalls, iteration, availableTools, schemaToolNames)
	prompt := basePrompt
	activities := []assistantToolActivity{}
	maxAttempts := assistantMaxPlannerAttempts
	schemaRepaired := false
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		completion, err := client.Complete(ctx, prompt)
		output := map[string]any{
			"iteration": iteration,
			"attempt":   attempt,
		}
		if err != nil {
			output["fallback_reason"] = err.Error()
			activities = append(activities, *assistantLLMPlannerActivity(profileName, profile, assistantToolStatusError, output))
			return assistantPlannerDecision{}, activities, false
		}
		output["usage"] = completion.Usage
		decision, err := parseAssistantPlannerDecision(completion.Text)
		if err != nil {
			output["fallback_reason"] = err.Error()
			output["raw_response"] = assistantTruncateForPrompt(completion.Text)
			if attempt < assistantMaxPlannerAttempts && assistantPlannerDecisionRetryable(err) {
				output["will_retry"] = true
				activities = append(activities, *assistantLLMPlannerActivity(profileName, profile, assistantToolStatusError, output))
				prompt = assistantPlannerRepairPrompt(basePrompt, completion.Text, err)
				continue
			}
			activities = append(activities, *assistantLLMPlannerActivity(profileName, profile, assistantToolStatusError, output))
			return assistantPlannerDecision{}, activities, false
		}
		if err := assistantValidatePlannerDecisionUsesSchemaTools(decision, schemaToolNames); err != nil {
			// Naming a real tool whose schema was not shipped is a routing miss on
			// our side, not a bad plan. Hand over the schemas and let the planner
			// finish instead of ending the turn on our own omission.
			missing := assistantPlannerRepairableSchemaTools(decision, schemaToolNames, availableTools)
			if len(missing) > 0 && !schemaRepaired {
				schemaRepaired = true
				maxAttempts++
				for _, name := range missing {
					schemaToolNames[name] = true
				}
				output["fallback_reason"] = err.Error()
				output["schema_repair"] = missing
				output["will_retry"] = true
				activities = append(activities, *assistantLLMPlannerActivity(profileName, profile, assistantToolStatusError, output))
				basePrompt = a.buildAssistantPlannerPromptWithSchemas(conversation, content, plan, toolCalls, remainingToolCalls, iteration, availableTools, schemaToolNames)
				prompt = assistantPlannerSchemaRepairPrompt(basePrompt, missing)
				continue
			}
			output["fallback_reason"] = err.Error()
			output["raw_response"] = assistantTruncateForPrompt(completion.Text)
			activities = append(activities, *assistantLLMPlannerActivity(profileName, profile, assistantToolStatusError, output))
			return assistantPlannerDecision{}, activities, false
		}
		output["goal"] = decision.Goal
		output["intent"] = decision.Intent
		output["success_criteria"] = decision.SuccessCriteria
		output["tool_count"] = len(decision.Steps)
		output["has_final_answer"] = strings.TrimSpace(decision.FinalAnswer) != ""
		if attempt > 1 {
			output["repaired"] = true
		}
		activities = append(activities, *assistantLLMPlannerActivity(profileName, profile, assistantToolStatusSuccess, output))
		return decision, activities, true
	}
	return assistantPlannerDecision{}, activities, false
}

func assistantPlannerDecisionRetryable(err error) bool {
	if err == nil {
		return false
	}
	reason := strings.ToLower(err.Error())
	return strings.Contains(reason, "planner response did not contain a json object") ||
		strings.Contains(reason, "parse planner json")
}

// assistantPlannerRepairableSchemaTools returns the read-only tools a plan asked
// for that exist, are permitted for this subject, and were not given a schema.
//
// Only reads are repaired. A missing read schema is a routing miss on our side,
// and the tool call is still AAA-checked when it runs. A missing write or
// proposal schema is usually the opposite: schema selection withheld it on
// purpose, because plain "add a secret" must not be quietly turned into a GitOps
// proposal, and an unconfirmed mutation must not be handed the tool it needs.
// Repairing those would relax a guardrail rather than fix an omission.
func assistantPlannerRepairableSchemaTools(decision assistantPlannerDecision, schemaToolNames map[string]bool, tools []hostedMCPTool) []string {
	available := map[string]hostedMCPTool{}
	for _, tool := range tools {
		available[tool.Name] = tool
	}
	seen := map[string]bool{}
	missing := []string{}
	for _, step := range decision.Steps {
		name := strings.TrimSpace(step.Tool)
		if name == "" || schemaToolNames[name] || seen[name] {
			continue
		}
		tool, ok := available[name]
		if !ok {
			continue
		}
		if assistantPlannedToolIsProposal(name) || assistantToolRequiresActionExecution(tool) {
			continue
		}
		seen[name] = true
		missing = append(missing, name)
	}
	sort.Strings(missing)
	return missing
}

func assistantPlannerSchemaRepairPrompt(basePrompt string, missing []string) string {
	return basePrompt + "\n\nThe schemas for " + strings.Join(missing, ", ") +
		" are now included in schema_tools because your previous plan selected them. Return the plan again using those exact argument names."
}

func assistantPlannerRepairPrompt(originalPrompt string, rawResponse string, err error) string {
	return strings.TrimSpace(`Your previous NopsAI assistant planner response was invalid or incomplete JSON.

Repair the response by returning exactly one complete JSON object that follows the required planner schema.
Do not include Markdown, prose, code fences, comments, or multiple JSON objects.
Keep the plan compact. Use available tools from the original prompt; do not invent tools or arguments.

Planner parse error:
` + strings.TrimSpace(err.Error()) + `

Previous invalid response:
` + assistantTruncateForPrompt(rawResponse) + `

Original planner instructions and context:
` + originalPrompt)
}

func assistantLLMPlannerActivity(profileName string, profile config.LLMProfile, status string, output map[string]any) *assistantToolActivity {
	if output == nil {
		output = map[string]any{}
	}
	output["profile"] = profileName
	output["provider"] = profile.Provider
	output["model"] = profile.Model
	return &assistantToolActivity{
		Name: assistantLLMPlannerToolName,
		Input: map[string]any{
			"profile":  profileName,
			"provider": profile.Provider,
			"model":    profile.Model,
		},
		Output:       output,
		Status:       status,
		ResourceURIs: []string{"nopsai://features"},
		Source:       "llm",
		Phase:        "planning",
		Confidence:   "medium",
		Purpose:      "Create and validate a safe execution plan before any MCP evidence calls are run.",
	}
}

func assistantBaseTurnPlan(content string) assistantTurnPlan {
	return assistantBaseTurnPlanWithPageContext(content, assistantConversationMemory{}, assistantPageContext{})
}

func assistantBaseTurnPlanWithPageContext(content string, memory assistantConversationMemory, pageContext assistantPageContext) assistantTurnPlan {
	content = strings.TrimSpace(content)
	lower := strings.ToLower(content)
	pageContext = normalizeAssistantPageContext(pageContext)
	plan := assistantTurnPlan{
		Intent:        "llm_planned",
		Goal:          content,
		LowerContent:  lower,
		PageContext:   pageContext,
		RunID:         assistantFirstUUID(content),
		YAML:          assistantYAMLFromMessage(content),
		PipelineName:  assistantPipelineNameFromMessage(content),
		PipelineID:    assistantPipelineIDFromMessage(content),
		Repository:    assistantFirstPatternTeam(assistantRepositoryPattern, content),
		ScheduleID:    assistantFirstPatternTeam(assistantScheduleIDPattern, content),
		Scope:         assistantScopeFromMessage(content),
		UserConfirmed: assistantFeatureConfirmed(lower),
	}
	plan.APIMethod, plan.APIPath = assistantAPICallFromMessage(content)
	if plan.RunID == "" {
		plan.RunID = assistantPageContextRunID(pageContext)
	}
	if plan.RunID == "" {
		plan.RunID = strings.TrimSpace(memory.SelectedRun)
	}
	if plan.PipelineID == "" {
		plan.PipelineID = assistantPageContextPipelineID(pageContext)
	}
	if plan.PipelineID == "" {
		plan.PipelineID = strings.Trim(strings.TrimSpace(memory.SelectedPipeline), "/")
	}
	if plan.Scope == "" {
		plan.Scope = assistantPageContextScope(pageContext)
	}
	if plan.Scope == "" {
		plan.Scope = strings.Trim(strings.TrimSpace(memory.SelectedScope), "/")
	}
	if plan.Repository == "" {
		plan.Repository = assistantPageContextRepository(pageContext)
	}
	if plan.ScheduleID == "" {
		plan.ScheduleID = assistantPageContextScheduleID(pageContext)
	}
	if plan.PipelineName == "" {
		plan.PipelineName = "generated-pipeline"
	}
	return plan
}

func assistantTurnPlanFromPlannerDecision(base assistantTurnPlan, decision assistantPlannerDecision) assistantTurnPlan {
	plan := base
	if value := strings.TrimSpace(decision.Goal); value != "" {
		plan.Goal = value
	}
	if value := strings.TrimSpace(decision.Intent); value != "" {
		plan.Intent = value
	}
	if plan.Intent == "" {
		plan.Intent = "llm_planned"
	}
	plan.SuccessCriteria = strings.TrimSpace(decision.SuccessCriteria)
	plan.FinalAnswer = strings.TrimSpace(decision.FinalAnswer)
	plan.ClarifyQuestion = strings.TrimSpace(decision.ClarifyingQuestion)
	plan.Steps = make([]assistantPlanStep, 0, len(decision.Steps))
	for _, step := range decision.Steps {
		toolName := strings.TrimSpace(step.Tool)
		if toolName == "" {
			continue
		}
		plan.Steps = append(plan.Steps, assistantPlanStep{
			ToolName: toolName,
			Thought:  strings.TrimSpace(step.Reason),
			Args:     cloneAssistantArgs(step.Args),
		})
	}
	if assistantPlanIncludesTool(plan, "nopsai.get_monitoring_ai_usage") {
		plan.Intent = "ai_token_usage"
	} else if assistantPlanIncludesTool(plan, "nopsai.analyze_pipeline_run_failure") {
		plan.Intent = "analyze_run"
	}
	if assistantPlanIncludesTool(plan, "nopsai.propose_pipeline_create") {
		plan.Intent = "propose_pipeline_create"
	} else if assistantPlanIncludesTool(plan, "nopsai.propose_pipeline_update") {
		plan.Intent = "propose_pipeline_update"
	} else if assistantPlanIncludesTool(plan, "nopsai.validate_pipeline") {
		plan.Intent = "validate_pipeline"
	} else if assistantPlanIncludesTool(plan, "nopsai.search_pipelines") {
		plan.Intent = "search_pipelines"
	}
	return plan
}

func assistantPlanHasTerminalEvidence(plan assistantTurnPlan, toolCalls []assistantToolActivity) bool {
	for _, step := range plan.Steps {
		switch strings.TrimSpace(step.ToolName) {
		case "nopsai.analyze_pipeline_run_failure":
			call := assistantFirstToolCall(toolCalls, step.ToolName)
			if call.Status == assistantToolStatusSuccess {
				return true
			}
		case "nopsai.get_monitoring_ai_usage":
			call := assistantFirstToolCall(toolCalls, step.ToolName)
			if assistantAIUsageCallHasEvents(call) {
				return true
			}
		}
	}
	return false
}

func (a *App) buildAssistantPlannerPrompt(ctx context.Context, subject aaamodel.Subject, conversation assistantConversation, content string, plan assistantTurnPlan, toolCalls []assistantToolActivity, remainingToolCalls int, iteration int) string {
	availableTools := a.hostedMCPToolsForSubject(ctx, subject)
	schemaToolNames := assistantPlannerSchemaToolNames(assistantPlannerSchemaContext(conversation, content, plan.PageContext), plan, toolCalls, availableTools)
	return a.buildAssistantPlannerPromptWithSchemas(conversation, content, plan, toolCalls, remainingToolCalls, iteration, availableTools, schemaToolNames)
}

func (a *App) buildAssistantPlannerPromptWithSchemas(
	conversation assistantConversation,
	content string,
	plan assistantTurnPlan,
	toolCalls []assistantToolActivity,
	remainingToolCalls int,
	iteration int,
	availableTools []hostedMCPTool,
	schemaToolNames map[string]bool,
) string {
	payload := map[string]any{
		"user_request":         strings.TrimSpace(content),
		"conversation_memory":  normalizeAssistantMemory(conversation.Memory),
		"conversation_history": assistantPromptConversationHistory(conversation.Messages),
		"extracted_context": map[string]any{
			"run_id":                  plan.RunID,
			"pipeline":                plan.PipelineID,
			"scope":                   plan.Scope,
			"repository":              plan.Repository,
			"schedule_id":             plan.ScheduleID,
			"yaml_present":            plan.YAML != "",
			"api_method":              plan.APIMethod,
			"api_path":                plan.APIPath,
			"user_confirmed_mutation": plan.UserConfirmed,
		},
		"limits": map[string]any{
			"remaining_tool_calls": remainingToolCalls,
			"max_steps_per_plan":   assistantMaxPlanToolCalls,
			"iteration":            iteration,
		},
		"available_tools": assistantPlannerToolCatalog(availableTools, schemaToolNames),
		"schema_tools":    assistantPlannerSchemaToolCatalog(availableTools, schemaToolNames),
	}
	if pageContext := assistantPageContextPromptMap(plan.PageContext); len(pageContext) > 0 {
		payload["page_context"] = pageContext
	}
	if previousEvidence := assistantPromptPreviousEvidence(conversation.Messages); len(previousEvidence) > 0 {
		payload["previous_evidence"] = previousEvidence
	}
	if previousToolCalls := assistantLLMPromptToolCalls(assistantEvidenceToolCalls(toolCalls)); len(previousToolCalls) > 0 {
		payload["previous_tool_calls"] = previousToolCalls
	}
	raw, _ := json.Marshal(payload)
	return strings.TrimSpace(`You are the NopsAI assistant planner.

Create a safe hosted MCP tool plan from user_request, available_tools, schema_tools, and observed tool results.
Use conversation_history, memory, and page_context for follow-ups; explicit user targets win.
Use only tool names from available_tools. Put a tool in steps only when it also appears in schema_tools; the rest of available_tools is discoverability only. Catalogue flags (schema_included, mutating, proposal) appear only when true.
Select tools from their descriptions and use schema_tools for exact argument names. If the tool you need has no schema here, call nopsai.find_tools with a short query to get its schema instead of guessing arguments. Never invent tools or arguments. Never request direct database access.
Step reasons are user-visible; keep them short and operational, not hidden reasoning.
Prefer first-party analytics tools over stitching raw data manually.
For health, review, "how is X doing", "why did this run fail", or "what should we fix first" questions, call nopsai.analyze_team, nopsai.analyze_pipeline, or nopsai.analyze_run first: they return ranked findings with evidence, category scores, a likely failure domain for runs, and the next tool to call. Fall back to individual monitoring tools only when the user asks for one specific metric.
When an analysis result lists next_actions, treat the first one as the recommended next step and offer it to the user.
For reads, choose the smallest evidence set that can answer the question.
If previous_evidence is enough for a follow-up calculation, estimate, comparison, or explanation, return no steps and final_answer with "Data source" and "Confidence". Separate MCP-backed facts from LLM-derived assumptions.
For cost estimates, do not invent exact pricing. If pricing is missing, give a formula or scenario with explicit per-token/per-million-token assumptions.
For pipeline YAML, config, or API validation, use the relevant MCP validation/API/doc tools and schemas instead of relying on memory.
For samples, templates, schema questions, and "what should this definition look like" requests, gather current NopsAI docs or capability evidence before answering. Do not answer from static examples in the prompt.
For pipeline generation or edits, route YAML through nopsai.validate_pipeline or a nopsai.propose_pipeline_* tool before answering.
For changes, choose the tool mode the user asked for: use proposal/GitOps tools for proposed file plans, and use confirmed runtime tools only when the user explicitly confirmed the mutation and the tool accepts confirm:true.
For variables and secrets, plain add/set/update/delete requests should use direct runtime MCP write/delete tools. Do not substitute GitOps/proposal tools unless the user explicitly asks for GitOps, a proposal, or a file plan. "Encrypted secret" means the secret domain stores encrypted material; it is not by itself a GitOps request.
If a mutating tool is needed but the user did not explicitly confirm, return that tool without confirm:true; NopsAI validation will block execution and the answer should explain confirmation is required.
If a needed tool is not listed in schema_tools and it requires arguments you cannot infer safely, ask a clarifying question instead of guessing.
When drafting pipeline YAML, every step must contain exactly one execution method: include, tasks, goal, script, or approval. For explicit operational steps such as clone repository, build docker image, or push image to registry, prefer script steps with concrete shell commands instead of name-only placeholder steps.
If previous_evidence is sufficient, return no steps and final_answer with source/confidence labels.
If the request is too broad or missing a target, return no steps and set clarifying_question.

Return JSON only, with this shape:
{
  "goal": "short user goal",
  "intent": "stable intent label",
  "steps": [
    {"tool": "nopsai.tool_name", "args": {}, "reason": "why this tool is needed"}
  ],
  "success_criteria": "how to decide the answer is complete",
  "needs_more_tools": true,
  "final_answer": "",
  "clarifying_question": ""
}

Context:
` + string(raw))
}

func assistantPlannerToolCatalog(tools []hostedMCPTool, schemaToolNames map[string]bool) []map[string]any {
	items := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		description := tool.Description
		if !schemaToolNames[tool.Name] {
			description = assistantPlannerCatalogDescription(description)
		}
		// Flags are emitted only when true. Across 200+ tools the false ones were
		// most of the catalogue's size and none of its information.
		item := map[string]any{"name": tool.Name, "description": description}
		if schemaToolNames[tool.Name] {
			item["schema_included"] = true
		}
		if assistantToolRequiresActionExecution(tool) {
			item["mutating"] = true
		}
		if assistantPlannedToolIsProposal(tool.Name) {
			item["proposal"] = true
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return fmt.Sprint(items[i]["name"]) < fmt.Sprint(items[j]["name"])
	})
	return items
}

// assistantPlannerCatalogDescription keeps the sentence that says what a tool is
// for and drops the argument and confirmation detail, which only matters once the
// tool's schema is actually included.
func assistantPlannerCatalogDescription(description string) string {
	description = strings.TrimSpace(description)
	if len(description) <= assistantPlannerCatalogDescriptionLimit {
		return description
	}
	if cut := strings.Index(description, ". "); cut > 0 && cut+1 <= assistantPlannerCatalogDescriptionLimit {
		return description[:cut+1]
	}
	trimmed := description[:assistantPlannerCatalogDescriptionLimit]
	if space := strings.LastIndex(trimmed, " "); space > 40 {
		trimmed = trimmed[:space]
	}
	return strings.TrimRight(trimmed, " ,;:") + "..."
}

func assistantPlannerSchemaToolCatalog(tools []hostedMCPTool, schemaToolNames map[string]bool) []map[string]any {
	items := make([]map[string]any, 0, len(schemaToolNames))
	for _, tool := range tools {
		if !schemaToolNames[tool.Name] {
			continue
		}
		items = append(items, map[string]any{
			"name":         tool.Name,
			"input_schema": assistantPlannerCompactInputSchema(tool.InputSchema),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return fmt.Sprint(items[i]["name"]) < fmt.Sprint(items[j]["name"])
	})
	return items
}

func assistantPlannerSchemaToolNames(content string, plan assistantTurnPlan, toolCalls []assistantToolActivity, tools []hostedMCPTool) map[string]bool {
	scores := map[string]int{}
	blocked := map[string]bool{}
	available := map[string]bool{}
	for _, tool := range tools {
		available[tool.Name] = true
	}
	add := func(name string, score int) {
		if available[name] {
			scores[name] += score
		}
	}
	addPrefix := func(prefix string, score int) {
		for name := range available {
			if strings.HasPrefix(name, prefix) {
				add(name, score)
			}
		}
	}
	lower := strings.ToLower(strings.TrimSpace(content))
	modePolicy := assistantPlannerModePolicyFor(lower, plan)
	for _, tool := range tools {
		if strings.Contains(lower, strings.ToLower(tool.Name)) {
			add(tool.Name, 100)
		}
	}
	for _, step := range plan.Steps {
		add(strings.TrimSpace(step.ToolName), 80)
	}
	for _, call := range assistantEvidenceToolCalls(toolCalls) {
		add(call.Name, 60)
	}
	if plan.RunID != "" {
		add("nopsai.analyze_run", 55)
		add("nopsai.get_pipeline_run", 45)
		add("nopsai.get_pipeline_run_logs", 45)
		add("nopsai.analyze_pipeline_run_failure", 45)
		add("nopsai.explain_internal_run_operations", 30)
	}
	if plan.YAML != "" {
		add("nopsai.validate_pipeline", 60)
		if assistantPlannerWantsPipelineProposalSchema(lower, plan) {
			add("nopsai.propose_pipeline_create", 50)
			add("nopsai.propose_pipeline_update", 50)
		}
	}
	if plan.PipelineID != "" || (plan.PipelineName != "" && plan.PipelineName != "generated-pipeline") {
		add("nopsai.get_pipeline", 40)
		add("nopsai.search_pipelines", 35)
		add("nopsai.get_pipeline_knowledge_context", 25)
	}
	if plan.Scope != "" {
		add("nopsai.get_scope", 35)
		add("nopsai.list_variables_metadata", 25)
		add("nopsai.list_secrets_metadata", 25)
	}
	if plan.Repository != "" {
		add("nopsai.get_trigger", 25)
		add("nopsai.list_variables_metadata", 25)
		add("nopsai.list_secrets_metadata", 25)
	}
	if plan.ScheduleID != "" {
		add("nopsai.get_schedule", 45)
		add("nopsai.propose_schedule_change", 35)
	}
	if plan.APIMethod != "" || plan.APIPath != "" {
		add("nopsai.call_api", 60)
	}

	if assistantTextHasAny(lower, "env", "envs", "env var", "environment variable", "environment variables", "variable", "variables", "var ", "_var") {
		addPrefix("nopsai.list_variable", 50)
		add("nopsai.analyze_variable_usage", 45)
		if assistantPlannerWantsExposurePolicySchema(lower) {
			add("nopsai.list_variables_metadata", 50)
		} else if assistantPlannerWantsDeleteSchema(lower) {
			if assistantPlannerWantsGitOpsProposalSchema(lower) {
				add("nopsai.propose_variable_gitops_delete", 55)
			} else {
				add("nopsai.delete_variable_value", 55)
			}
		} else if assistantPlannerWantsWriteSchema(lower) {
			if assistantPlannerWantsGitOpsProposalSchema(lower) {
				add("nopsai.propose_variable_gitops_write", 55)
			} else {
				add("nopsai.write_variable_value", 55)
			}
		} else {
			add("nopsai.get_variable_value", 45)
		}
	}
	if assistantTextHasAny(lower, "secret", "secrets") {
		addPrefix("nopsai.list_secret", 50)
		if assistantPlannerWantsDeleteSchema(lower) {
			if assistantPlannerWantsGitOpsProposalSchema(lower) {
				add("nopsai.propose_secret_gitops_delete", 55)
			} else {
				add("nopsai.delete_secret_value", 55)
			}
		} else if assistantPlannerWantsWriteSchema(lower) {
			if assistantPlannerWantsGitOpsProposalSchema(lower) {
				add("nopsai.encrypt_secret_for_gitops", 55)
				add("nopsai.propose_secret_gitops_write", 55)
			} else {
				add("nopsai.write_secret_value", 55)
			}
		}
	}
	if assistantPlannerWantsExposurePolicySchema(lower) {
		add("nopsai.get_feature_capabilities", 70)
		add("nopsai.search_docs", 60)
		add("nopsai.list_knowledge_contexts", 55)
		add("nopsai.get_knowledge_context", 45)
		add("nopsai.list_variables_metadata", 40)
		add("nopsai.list_secrets_metadata", 40)
		add("nopsai.explain_scope_permissions", 25)
	}

	if assistantTextHasAny(lower, "team", "teams", "squad", "organisation", "organization", "our group", "my group") {
		add("nopsai.list_teams", 45)
		add("nopsai.get_team", 40)
		add("nopsai.analyze_team", 55)
	}
	if assistantTextHasAny(lower, "analyse", "analyze", "analysis", "review", "health", "healthy", "how is", "how are", "how's", "doing", "what should i fix", "what should we fix", "where should i start", "posture", "state of", "assessment", "audit") {
		add("nopsai.analyze_team", 50)
		add("nopsai.analyze_pipeline", 50)
		add("nopsai.analyze_run", 45)
	}
	if assistantTextHasAny(lower, "pipeline", "yaml", "build", "deploy", "approval", "step", "rollout", "release") {
		add("nopsai.analyze_pipeline", 40)
		add("nopsai.list_pipelines", 20)
		add("nopsai.search_pipelines", 35)
		add("nopsai.get_pipeline", 35)
		add("nopsai.validate_pipeline", 45)
		if assistantPlannerWantsPipelineProposalSchema(lower, plan) {
			add("nopsai.propose_pipeline_create", 45)
			add("nopsai.propose_pipeline_update", 40)
		}
		add("nopsai.get_pipeline_knowledge_context", 20)
		add("nopsai.explain_pipeline_health", 20)
	}
	// Everything below domain policy is derived, not enumerated: tools are ranked
	// against the request from their own name, description, action, and resource
	// type, so a newly registered tool is routable without editing this function.
	for name, score := range hostedMCPTopRoutingScores(hostedMCPRoutingScores(content, tools, modePolicy), assistantPlannerMaxRoutedTools) {
		add(name, score)
	}
	// Intent rules. Relevance ranks tools by domain; these few cover the cases
	// where the answer needs a particular shape of evidence, or where the tool a
	// domain match would offer is the wrong one to offer. Each is a statement
	// about answers, not a routing entry per tool.
	if assistantTextHasAny(lower, "analyse", "analyze", "analysis", "review", "health", "healthy", "how is", "how are", "how's", "doing", "what should i fix", "what should we fix", "where should i start", "posture", "state of", "assessment", "audit") {
		add("nopsai.analyze_team", 50)
		add("nopsai.analyze_pipeline", 50)
		add("nopsai.analyze_run", 45)
	}
	if assistantTextHasAny(lower, "policy", "policies", "guardrail", "guardrails", "allowed to", "are we allowed") {
		add("nopsai.get_feature_capabilities", 55)
	}
	// "What should this look like" is answered from current docs and validation,
	// never from an example the model remembers.
	if assistantPlannerWantsExampleSchema(lower) {
		add("nopsai.search_docs", 60)
		add("nopsai.read_doc", 50)
		add("nopsai.list_knowledge_contexts", 40)
		add("nopsai.get_knowledge_context", 40)
		add("nopsai.get_feature_capabilities", 45)
		add("nopsai.validate_pipeline", 45)
	}
	// A "which is slowest" question is answered at step and task granularity; the
	// pipeline-level roll-up alone cannot name the step.
	if assistantTextHasAny(lower, "slowest", "fastest", "longest", "bottleneck", "bottlenecks", "duration", "latency", "p95", "p99") {
		add("nopsai.get_monitoring_step_performance", 65)
		add("nopsai.get_monitoring_task_performance", 60)
		add("nopsai.get_monitoring_pipeline_performance", 55)
	}
	// An exposure question is about what the platform reveals. Answering it by
	// reading a value would be the exposure the question is asking about.
	if assistantPlannerWantsExposurePolicySchema(lower) {
		add("nopsai.list_variables_metadata", 55)
		add("nopsai.list_secrets_metadata", 55)
		add("nopsai.explain_scope_permissions", 40)
		blocked["nopsai.get_variable_value"] = true
		blocked["nopsai.get_secret_value"] = true
	}

	if len(scores) == 0 {
		add("nopsai.get_feature_capabilities", 15)
	}

	type candidate struct {
		name  string
		score int
	}
	candidates := make([]candidate, 0, len(scores))
	for name, score := range scores {
		if score <= 0 || blocked[name] {
			continue
		}
		candidates = append(candidates, candidate{name: name, score: score})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].name < candidates[j].name
	})
	out := map[string]bool{}
	for idx, candidate := range candidates {
		if idx >= assistantMaxPlannerSchemaTools {
			break
		}
		out[candidate.name] = true
	}
	return out
}

// assistantPlannerModePolicyFor reads the requested change mode from the wording
// the user chose. Asking to set a value is not asking for a file plan, and asking
// for a proposal is not asking to apply anything.
func assistantPlannerModePolicyFor(lower string, plan assistantTurnPlan) assistantPlannerModePolicy {
	if assistantPlannerWantsGitOpsProposalSchema(lower) {
		return assistantPlannerModePolicy{AllowProposal: true}
	}
	if assistantPlannerWantsChangeSchema(lower) || plan.UserConfirmed {
		return assistantPlannerModePolicy{AllowMutation: true}
	}
	return assistantPlannerModePolicy{}
}

func assistantPlannerSchemaContext(conversation assistantConversation, content string, pageContext assistantPageContext) string {
	parts := []string{}
	for _, message := range conversation.Messages {
		if message.Role != assistantRoleUser {
			continue
		}
		if text := strings.TrimSpace(message.Content); text != "" {
			parts = append(parts, text)
		}
	}
	if text := strings.TrimSpace(content); text != "" {
		parts = append(parts, text)
	}
	if text := strings.TrimSpace(assistantPageContextSummary(pageContext)); text != "" {
		parts = append(parts, text)
	}
	if len(parts) > assistantPromptHistoryLimit {
		parts = parts[len(parts)-assistantPromptHistoryLimit:]
	}
	return strings.Join(parts, "\n")
}

func assistantValidatePlannerDecisionUsesSchemaTools(decision assistantPlannerDecision, schemaToolNames map[string]bool) error {
	for idx, step := range decision.Steps {
		toolName := strings.TrimSpace(step.Tool)
		if toolName == "" {
			continue
		}
		if !schemaToolNames[toolName] {
			return fmt.Errorf("planner step %d selected %q, but that tool schema was not included in schema_tools for this request", idx+1, toolName)
		}
	}
	return nil
}

func assistantPlannerWantsPipelineProposalSchema(lower string, plan assistantTurnPlan) bool {
	if assistantPlannerWantsGitOpsProposalSchema(lower) {
		return true
	}
	if plan.UserConfirmed {
		return true
	}
	return assistantTextHasAny(lower,
		"create pipeline",
		"new pipeline",
		"save pipeline",
		"update pipeline",
		"edit pipeline",
		"change pipeline",
		"delete pipeline",
		"add step",
		"remove step",
	)
}

// assistantPlannerWantsExampleSchema recognises "show me what this should look
// like", which must be answered from current docs rather than from memory.
func assistantPlannerWantsExampleSchema(lower string) bool {
	return assistantTextHasAny(lower,
		"example", "examples", "sample", "samples", "template", "templates",
		"looks like", "look like", "how it is implemented", "how is it implemented",
		"working example", "starter", "boilerplate", "what should this",
	)
}

func assistantPlannerWantsGitOpsProposalSchema(lower string) bool {
	return assistantTextHasAny(lower,
		"gitops",
		"git ops",
		"proposal",
		"propose",
		"file plan",
		"commit-ready",
		"commit ready",
		"pull request",
		"merge request",
		"without applying",
	)
}

func assistantPlannerWantsExposurePolicySchema(lower string) bool {
	return assistantTextHasAny(lower,
		"policy",
		"policies",
		"guardrail",
		"guardrails",
		"prevent",
		"block",
		"hide",
		"showing",
		"expose",
		"redact",
		"plaintext",
	) && assistantTextHasAny(lower,
		"env",
		"envs",
		"environment variable",
		"environment variables",
		"secret",
		"secrets",
		"credential",
		"credentials",
		"token",
		"password",
		"sensitive",
	)
}

func assistantPlannerWantsChangeSchema(lower string) bool {
	return assistantPlannerWantsWriteSchema(lower) || assistantPlannerWantsDeleteSchema(lower)
}

func assistantPlannerWantsWriteSchema(lower string) bool {
	return strings.Contains(lower, "=") || assistantTextHasAny(lower,
		"add ",
		"change ",
		"create ",
		"enable ",
		"set ",
		"update ",
		"write ",
		// Operational verbs are change requests too. Missing one used to be
		// harmless because a fallback re-admitted the tool anyway; now that mode
		// selection decides what may appear at all, the verb list is the decision.
		"pause ",
		"resume ",
		"drain ",
		"eject ",
		"cancel ",
		"stop ",
		"start ",
		"restart ",
		"rotate ",
		"disable ",
		"approve ",
		"reject ",
		"trigger ",
		"rerun ",
		"retry ",
		"sync ",
		"refresh ",
		"activate ",
		"deactivate ",
	)
}

func assistantPlannerWantsDeleteSchema(lower string) bool {
	return assistantTextHasAny(lower,
		"delete ",
		"disable ",
		"remove ",
	)
}

func assistantTextHasAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func assistantPlannerCompactInputSchema(schema map[string]any) map[string]any {
	compact := map[string]any{"type": "object"}
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) == 0 {
		return compact
	}
	propertyTypes := map[string]string{}
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		property, _ := properties[key].(map[string]any)
		propertyTypes[key] = assistantPlannerSchemaTypeLabel(property)
	}
	compact["properties"] = propertyTypes
	return compact
}

func assistantPlannerSchemaTypeLabel(schema map[string]any) string {
	schemaType, _ := schema["type"].(string)
	schemaType = strings.TrimSpace(schemaType)
	if schemaType == "" {
		return "any"
	}
	return schemaType
}

func parseAssistantPlannerDecision(text string) (assistantPlannerDecision, error) {
	raw := assistantExtractJSONObject(text)
	if raw == "" {
		return assistantPlannerDecision{}, fmt.Errorf("planner response did not contain a JSON object")
	}
	var decision assistantPlannerDecision
	if err := json.Unmarshal([]byte(raw), &decision); err != nil {
		return assistantPlannerDecision{}, fmt.Errorf("parse planner JSON: %w", err)
	}
	for idx := range decision.Steps {
		if decision.Steps[idx].Args == nil {
			decision.Steps[idx].Args = map[string]any{}
		}
	}
	return decision, nil
}

func assistantExtractJSONObject(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "```") {
		parts := strings.Split(text, "```")
		for _, part := range parts {
			candidate := strings.TrimSpace(part)
			if candidate == "" {
				continue
			}
			lines := strings.Split(candidate, "\n")
			if len(lines) > 1 && !strings.HasPrefix(strings.TrimSpace(lines[0]), "{") {
				candidate = strings.TrimSpace(strings.Join(lines[1:], "\n"))
			}
			if strings.HasPrefix(candidate, "{") {
				text = candidate
				break
			}
		}
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return ""
	}
	return strings.TrimSpace(text[start : end+1])
}

func assistantEvidenceToolCalls(toolCalls []assistantToolActivity) []assistantToolActivity {
	filtered := make([]assistantToolActivity, 0, len(toolCalls))
	for _, call := range toolCalls {
		switch call.Name {
		case assistantLLMPlannerToolName, assistantLLMToolName, assistantExecutionPlanToolName, "nopsai.assistant_plan":
			continue
		default:
			filtered = append(filtered, call)
		}
	}
	return filtered
}

func assistantPlanDeniedActivity(plan assistantTurnPlan, err error) assistantToolActivity {
	return assistantToolActivity{
		Name:   "nopsai.assistant_plan",
		Status: assistantToolStatusDenied,
		Input:  assistantPlanActivityInput(plan),
		Output: map[string]any{
			"error":      err.Error(),
			"applied":    false,
			"validated":  false,
			"tool_count": len(plan.Steps),
		},
		Source:     "policy",
		Phase:      "planning",
		Confidence: "high",
		Purpose:    "Reject unsafe or invalid assistant plans before execution.",
	}
}

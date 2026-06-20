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
	assistantMaxPlannerSchemaTools = 18
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
	profileName, profile, client, ok, reason := a.assistantLLMClientForTurn(ctx, conversation, selectedProfile)
	if !ok {
		if reason == "" {
			return assistantPlannerResult{}
		}
		plan := assistantBaseTurnPlan(content, conversation.Memory)
		return assistantPlannerResult{
			Plan: plan,
			ToolCalls: []assistantToolActivity{*assistantLLMPlannerActivity(profileName, profile, assistantToolStatusError, map[string]any{
				"fallback_reason": reason,
			})},
			Handled: false,
		}
	}

	plan := assistantBaseTurnPlan(content, conversation.Memory)
	toolCalls := []assistantToolActivity{}
	remainingToolCalls := assistantMaxPlanToolCalls
	skipSynthesis := false
	for iteration := 1; iteration <= assistantMaxPlannerIterations; iteration++ {
		decision, activity, ok := a.requestAssistantPlannerDecision(ctx, subject, conversation, content, plan, toolCalls, remainingToolCalls, iteration, profileName, profile, client)
		toolCalls = append(toolCalls, activity)
		if !ok {
			return assistantPlannerResult{Plan: plan, ToolCalls: toolCalls, Handled: false}
		}
		plan = assistantTurnPlanFromPlannerDecision(plan, decision)
		if plan.ClarifyQuestion != "" && len(plan.Steps) == 0 {
			plan.Intent = "clarify"
			return assistantPlannerResult{Plan: plan, ToolCalls: toolCalls, Handled: true}
		}
		if plan.FinalAnswer != "" && len(plan.Steps) == 0 {
			if err := assistantValidatePlannerFinalAnswer(plan, toolCalls); err != nil {
				plan.FinalAnswer = ""
				toolCalls = append(toolCalls, assistantPlanDeniedActivity(plan, err))
				return assistantPlannerResult{Plan: plan, ToolCalls: toolCalls, Handled: true}
			}
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
		for _, step := range plan.Steps {
			call := a.runAssistantHostedMCPTool(ctx, subject, userID, conversation.ID, step.ToolName, cloneAssistantArgs(step.Args))
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
) (assistantPlannerDecision, assistantToolActivity, bool) {
	prompt := a.buildAssistantPlannerPrompt(ctx, subject, conversation, content, plan, toolCalls, remainingToolCalls, iteration)
	completion, err := client.Complete(ctx, prompt)
	if err != nil {
		return assistantPlannerDecision{}, *assistantLLMPlannerActivity(profileName, profile, assistantToolStatusError, map[string]any{
			"iteration":       iteration,
			"fallback_reason": err.Error(),
		}), false
	}
	decision, err := parseAssistantPlannerDecision(completion.Text)
	output := map[string]any{
		"iteration": iteration,
		"usage":     completion.Usage,
	}
	if err != nil {
		output["fallback_reason"] = err.Error()
		output["raw_response"] = assistantTruncateForPrompt(completion.Text)
		return assistantPlannerDecision{}, *assistantLLMPlannerActivity(profileName, profile, assistantToolStatusError, output), false
	}
	availableTools := a.hostedMCPToolsForSubject(ctx, subject)
	schemaToolNames := assistantPlannerSchemaToolNames(content, plan, toolCalls, availableTools)
	if err := assistantValidatePlannerDecisionUsesSchemaTools(decision, schemaToolNames); err != nil {
		output["fallback_reason"] = err.Error()
		output["raw_response"] = assistantTruncateForPrompt(completion.Text)
		return assistantPlannerDecision{}, *assistantLLMPlannerActivity(profileName, profile, assistantToolStatusError, output), false
	}
	output["goal"] = decision.Goal
	output["intent"] = decision.Intent
	output["success_criteria"] = decision.SuccessCriteria
	output["tool_count"] = len(decision.Steps)
	output["has_final_answer"] = strings.TrimSpace(decision.FinalAnswer) != ""
	return decision, *assistantLLMPlannerActivity(profileName, profile, assistantToolStatusSuccess, output), true
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
	}
}

func assistantBaseTurnPlan(content string, memory assistantConversationMemory) assistantTurnPlan {
	content = strings.TrimSpace(content)
	lower := strings.ToLower(content)
	plan := assistantTurnPlan{
		Intent:        "llm_planned",
		Goal:          content,
		LowerContent:  lower,
		RunID:         assistantFirstUUID(content),
		YAML:          assistantYAMLFromMessage(content),
		PipelineName:  assistantPipelineNameFromMessage(content),
		PipelineID:    assistantPipelineIDFromMessage(content),
		Repository:    assistantFirstPatternGroup(assistantRepositoryPattern, content),
		ScheduleID:    assistantFirstPatternGroup(assistantScheduleIDPattern, content),
		Scope:         assistantScopeFromMessage(content),
		UserConfirmed: assistantFeatureConfirmed(lower),
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
		}
	}
	return false
}

func (a *App) buildAssistantPlannerPrompt(ctx context.Context, subject aaamodel.Subject, conversation assistantConversation, content string, plan assistantTurnPlan, toolCalls []assistantToolActivity, remainingToolCalls int, iteration int) string {
	availableTools := a.hostedMCPToolsForSubject(ctx, subject)
	schemaToolNames := assistantPlannerSchemaToolNames(content, plan, toolCalls, availableTools)
	payload := map[string]any{
		"user_request":        strings.TrimSpace(content),
		"conversation_memory": normalizeAssistantMemory(conversation.Memory),
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
		"available_tools":     assistantPlannerToolCatalog(availableTools, schemaToolNames),
		"schema_tools":        assistantPlannerSchemaToolCatalog(availableTools, schemaToolNames),
		"previous_tool_calls": assistantLLMPromptToolCalls(assistantEvidenceToolCalls(toolCalls)),
	}
	raw, _ := json.Marshal(payload)
	return strings.TrimSpace(`You are the NopsAI assistant planner for an enterprise CI/CD, GitOps, and operations platform.

Create a safe hosted MCP tool plan from the user's request, available_tools, schema_tools, and observed tool results.
Use only tool names from available_tools, and only put a tool in steps when its available_tools schema_included value is true. Tools with schema_included:false are discoverability context only for this turn.
Select tools from their descriptions and use schema_tools for exact argument names. Never invent tools or arguments. Never request direct database access.
Prefer first-party analytics tools over stitching raw data manually.
For reads, choose the smallest evidence set that can answer the question.
For pipeline YAML, config, or API validation, use the relevant MCP validation/API/doc tools and schemas instead of relying on memory.
For pipeline generation or edits, route YAML through nopsai.validate_pipeline or a nopsai.propose_pipeline_* tool before answering.
For changes, choose the tool mode the user asked for: use proposal/GitOps tools for proposed file plans, and use confirmed runtime tools only when the user explicitly confirmed the mutation and the tool accepts confirm:true.
For variables and secrets, plain add/set/update/delete requests should use direct runtime MCP write/delete tools. Do not substitute GitOps/proposal tools unless the user explicitly asks for GitOps, a proposal, or a file plan. "Encrypted secret" means the secret domain stores encrypted material; it is not by itself a GitOps request.
If a mutating tool is needed but the user did not explicitly confirm, return that tool without confirm:true; NopsAI validation will block execution and the answer should explain confirmation is required.
If a needed tool is not listed in schema_tools and it requires arguments you cannot infer safely, ask a clarifying question instead of guessing.
When drafting pipeline YAML, every step must contain exactly one execution method: include, tasks, goal, script, or approval. For explicit operational steps such as clone repository, build docker image, or push image to registry, prefer script steps with concrete shell commands instead of name-only placeholder steps.
If the previous tool outputs are sufficient, return no steps and write final_answer from the evidence.
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
		items = append(items, map[string]any{
			"name":            tool.Name,
			"description":     tool.Description,
			"schema_included": schemaToolNames[tool.Name],
			"mutating":        assistantToolRequiresActionExecution(tool),
			"proposal":        assistantPlannedToolIsProposal(tool.Name),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return fmt.Sprint(items[i]["name"]) < fmt.Sprint(items[j]["name"])
	})
	return items
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
	addContains := func(fragment string, score int) {
		for name := range available {
			if strings.Contains(name, fragment) {
				add(name, score)
			}
		}
	}

	lower := strings.ToLower(strings.TrimSpace(content))
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

	if assistantTextHasAny(lower, "env var", "environment variable", "variable", "variables", "var ", "_var") {
		addPrefix("nopsai.list_variable", 50)
		add("nopsai.analyze_variable_usage", 45)
		if assistantPlannerWantsDeleteSchema(lower) {
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

	if assistantTextHasAny(lower, "pipeline", "yaml", "build", "deploy", "approval", "step", "rollout", "release") {
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
	if assistantTextHasAny(lower, "run", "failed", "failure", "error", "log", "logs", "why did", "lab") {
		add("nopsai.list_pipeline_runs", 35)
		add("nopsai.get_pipeline_run", 45)
		add("nopsai.get_pipeline_run_logs", 45)
		add("nopsai.analyze_pipeline_run_failure", 55)
		add("nopsai.explain_lab_result", 25)
	}
	if assistantTextHasAny(lower, "monitor", "monitoring", "health", "reliability", "efficiency", "security", "performance", "analytics", "usage", "tokens", "token", "cost", "runner utilization") {
		addPrefix("nopsai.get_monitoring_", 35)
		add("nopsai.get_pipeline_efficiency", 30)
		add("nopsai.compare_pipelines", 25)
		add("nopsai.compare_schedules", 25)
		add("nopsai.explain_pipeline_health", 35)
		add("nopsai.find_optimization_opportunities", 30)
		add("nopsai.get_cost_summary", 25)
		add("nopsai.suggest_cost_improvements", 25)
	}
	if assistantTextHasAny(lower, "trigger", "webhook") {
		add("nopsai.list_triggers", 45)
		add("nopsai.get_trigger", 45)
		if assistantPlannerWantsChangeSchema(lower) || assistantPlannerWantsGitOpsProposalSchema(lower) {
			add("nopsai.propose_trigger_change", 45)
			addContains("webhook_source", 35)
			addContains("external_trigger", 35)
		}
		add("nopsai.explain_webhook_ingress_policy", 35)
	}
	if assistantTextHasAny(lower, "schedule", "cron") {
		add("nopsai.list_schedules", 45)
		add("nopsai.get_schedule", 45)
		if assistantPlannerWantsChangeSchema(lower) || assistantPlannerWantsGitOpsProposalSchema(lower) {
			add("nopsai.propose_schedule_change", 45)
			addContains("schedule", 25)
		}
	}
	if assistantTextHasAny(lower, "credential", "credentials", "api key", "credential token", "api token", "personal token", "service account token", "rotate credential", "rotate credentials") {
		addContains("credential", 45)
	}
	if assistantTextHasAny(lower, "runner", "dispatcher", "kubernetes", "k8s", "docker", "bootstrap command") {
		addContains("runner", 45)
		add("nopsai.get_dispatcher_status", 35)
	}
	if assistantTextHasAny(lower, "setup", "install", "bootstrap", "first install", "template") {
		addContains("setup", 45)
	}
	if assistantTextHasAny(lower, "docs", "documentation", "knowledge", "guideline") {
		add("nopsai.search_docs", 45)
		add("nopsai.read_doc", 35)
		add("nopsai.list_knowledge_contexts", 35)
		add("nopsai.get_knowledge_context", 35)
	}
	if assistantTextHasAny(lower, "profile", "profiles", "llm", "mcp", "capability", "capabilities", "feature coverage", "what can you") {
		add("nopsai.get_feature_capabilities", 55)
		add("nopsai.get_llm_profiles", 40)
		add("nopsai.get_mcp_profiles", 40)
	}
	if assistantTextHasAny(lower, "permission", "permissions", "access", "grant", "aaa", "role", "roles") {
		addContains("access", 40)
		addContains("permission", 40)
		addContains("grant", 40)
		add("nopsai.get_effective_permissions", 50)
	}
	if assistantTextHasAny(lower, "system", "status", "ready", "dispatcher") {
		add("nopsai.get_system_status", 35)
		add("nopsai.get_dispatcher_status", 35)
	}
	if assistantTextHasAny(lower, "suggest", "improve", "optimization", "recommendation", "recommendations") {
		add("nopsai.suggest_design_improvements", 35)
		add("nopsai.suggest_cost_improvements", 35)
		add("nopsai.find_optimization_opportunities", 35)
		addContains("recommendation", 25)
	}

	if len(scores) < 4 {
		addLexicalSchemaToolMatches(lower, tools, scores)
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
		if score <= 0 {
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
	)
}

func assistantPlannerWantsDeleteSchema(lower string) bool {
	return assistantTextHasAny(lower,
		"delete ",
		"disable ",
		"remove ",
	)
}

func addLexicalSchemaToolMatches(lower string, tools []hostedMCPTool, scores map[string]int) {
	requestTokens := assistantPlannerSignificantTokens(lower)
	if len(requestTokens) == 0 {
		return
	}
	for _, tool := range tools {
		if !assistantPlannerAllowsLexicalToolMatch(lower, tool.Name) {
			continue
		}
		haystack := strings.ToLower(tool.Name + " " + tool.Description)
		score := 0
		for token := range requestTokens {
			if strings.Contains(haystack, token) {
				score++
			}
		}
		if score >= 2 {
			scores[tool.Name] += score * 8
		}
	}
}

func assistantPlannerAllowsLexicalToolMatch(lower, toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	isProposalMode := strings.HasPrefix(toolName, "nopsai.propose_") ||
		strings.HasPrefix(toolName, "nopsai.plan_") ||
		strings.HasPrefix(toolName, "nopsai.preview_")
	return !isProposalMode || assistantPlannerWantsGitOpsProposalSchema(lower)
}

func assistantPlannerSignificantTokens(text string) map[string]bool {
	text = strings.NewReplacer("_", " ", "-", " ", "/", " ", ".", " ").Replace(strings.ToLower(text))
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
	stop := map[string]bool{
		"a": true, "an": true, "and": true, "are": true, "about": true, "can": true,
		"create": true, "do": true, "for": true, "from": true, "give": true, "help": true,
		"how": true, "i": true, "in": true, "is": true, "it": true, "me": true,
		"need": true, "of": true, "on": true, "or": true, "plan": true, "please": true,
		"show": true, "set": true, "the": true, "this": true, "to": true, "use": true,
		"want": true, "what": true, "with": true, "you": true,
	}
	out := map[string]bool{}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if len(field) < 3 || stop[field] {
			continue
		}
		out[field] = true
	}
	return out
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
		case assistantLLMPlannerToolName, assistantLLMToolName, "nopsai.assistant_plan":
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
	}
}

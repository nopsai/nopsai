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
	assistantLLMPlannerToolName   = "nopsai.llm.plan"
	assistantMaxPlannerIterations = 4
)

type assistantPlannerResult struct {
	Plan      assistantTurnPlan
	ToolCalls []assistantToolActivity
	Handled   bool
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
		if remainingToolCalls <= 0 {
			break
		}
		if decision.NeedsMoreTools != nil && !*decision.NeedsMoreTools {
			break
		}
	}
	plan.Steps = nil
	return assistantPlannerResult{Plan: plan, ToolCalls: toolCalls, Handled: true}
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

func (a *App) buildAssistantPlannerPrompt(ctx context.Context, subject aaamodel.Subject, conversation assistantConversation, content string, plan assistantTurnPlan, toolCalls []assistantToolActivity, remainingToolCalls int, iteration int) string {
	availableTools := a.hostedMCPToolsForSubject(ctx, subject)
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
		"available_tools":     assistantPlannerToolCatalog(availableTools),
		"previous_tool_calls": assistantLLMPromptToolCalls(assistantEvidenceToolCalls(toolCalls)),
	}
	raw, _ := json.Marshal(payload)
	return strings.TrimSpace(`You are the NopsAI assistant planner for an enterprise CI/CD, GitOps, and operations platform.

Create a safe hosted MCP tool plan from the user's request, available_tools, and observed tool results.
Use only tool names from available_tools. Select tools from their descriptions and input_schema. Never invent tools. Never request direct database access.
Prefer first-party analytics tools over stitching raw data manually.
For reads, choose the smallest evidence set that can answer the question.
For pipeline YAML, config, or API validation, use the relevant MCP validation/API/doc tools and schemas instead of relying on memory.
For changes, prefer nopsai.propose_* or nopsai.plan_* tools. Do not apply changes unless the user explicitly confirmed the mutation and the tool accepts confirm:true.
If a mutating tool is needed but the user did not explicitly confirm, return that tool without confirm:true; NopsAI validation will block execution and the answer should explain confirmation is required.
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

func assistantPlannerToolCatalog(tools []hostedMCPTool) []map[string]any {
	items := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		items = append(items, map[string]any{
			"name":         tool.Name,
			"description":  tool.Description,
			"input_schema": assistantPlannerCompactInputSchema(tool.InputSchema),
			"mutating":     assistantToolRequiresActionExecution(tool),
			"proposal":     assistantPlannedToolIsProposal(tool.Name),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return fmt.Sprint(items[i]["name"]) < fmt.Sprint(items[j]["name"])
	})
	return items
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

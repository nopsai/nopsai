package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	aaamodel "nopsai/services/aaa/pkg/model"

	"nopsai/pkg/llmclient"
)

// The assistant turn.
//
// The model is handed the conversation and the tools this subject may call, and
// it decides what to do: ask for tools, read what comes back, ask for more, then
// answer. Nothing here maps a question to a tool. What used to sit in this place
// — a keyword scorer choosing which schemas the model was allowed to see, a
// prompt asking for a JSON plan, and a parser repairing that JSON — existed only
// because the provider was being driven through a plain text completion. The
// provider does this natively, so the plan, the scorer, and the repair are gone.
//
// What stays is the boundary: every tool call still runs through
// runAssistantHostedMCPTool, so AAA decides what this subject may read or
// change, and an unconfirmed mutation is still refused there rather than here.

const (
	// Model turns allowed in one assistant turn. Enough to gather evidence, read
	// it, and answer; bounded so a loop cannot run away.
	assistantMaxAgentSteps = 6
	// Characters of one tool result handed back to the model. A run's logs are
	// far larger than an answer needs, and an oversized result crowds out the
	// conversation it is supposed to explain.
	assistantMaxToolResultChars = 12000
)

func (a *App) runAssistantAgentTurn(
	ctx context.Context,
	subject aaamodel.Subject,
	userID string,
	conversation assistantConversation,
	content string,
	selectedProfile string,
	pageContext assistantPageContext,
) assistantPlannerResult {
	plan := assistantBaseTurnPlanWithPageContext(content, conversation.Memory, pageContext)
	profileName, profile, client, ok, reason := a.assistantLLMClientForTurn(ctx, conversation, selectedProfile)
	if !ok {
		if reason == "" {
			return assistantPlannerResult{}
		}
		return assistantPlannerResult{
			Plan: plan,
			ToolCalls: []assistantToolActivity{*assistantLLMActivity(profileName, profile, assistantToolStatusError, map[string]any{
				"fallback_reason": reason,
			})},
		}
	}

	unlocked := map[string]bool{}
	messages := assistantChatHistory(conversation.Messages)
	messages = append(messages, llmclient.ChatMessage{Role: llmclient.RoleUser, Content: strings.TrimSpace(content)})

	toolCalls := []assistantToolActivity{}
	remainingToolCalls := assistantMaxPlanToolCalls
	corrected := false

	for step := 1; step <= assistantMaxAgentSteps; step++ {
		// The last step answers from what it already has: offering tools there
		// would invite a call whose result nothing would ever read.
		var offered []llmclient.ToolDefinition
		if step < assistantMaxAgentSteps && remainingToolCalls > 0 {
			offered = a.assistantChatTools(ctx, subject, unlocked)
		}

		if offered == nil {
			a.publishAssistantTurnProgress(ctx, conversation.ID, userID, "Writing the answer")
		} else if step == 1 {
			a.publishAssistantTurnProgress(ctx, conversation.ID, userID, "Deciding what to look at")
		} else {
			a.publishAssistantTurnProgress(ctx, conversation.ID, userID, "Reading what came back")
		}

		startedAt := time.Now()
		response, err := client.Chat(ctx, llmclient.ChatRequest{
			System:   assistantAgentSystemPrompt(conversation, plan),
			Messages: messages,
			Tools:    offered,
		})
		modelDuration := time.Since(startedAt).Milliseconds()
		if err != nil {
			failure := assistantLLMActivity(profileName, profile, assistantToolStatusError, map[string]any{
				"fallback_reason": err.Error(),
				"step":            step,
			})
			failure.DurationMS = modelDuration
			toolCalls = append(toolCalls, *failure)
			return assistantPlannerResult{Plan: plan, ToolCalls: toolCalls}
		}
		modelTurn := assistantLLMActivity(profileName, profile, assistantToolStatusSuccess, map[string]any{
			"usage":         response.Usage,
			"step":          step,
			"tool_calls":    len(response.ToolCalls),
			"offered_tools": len(offered),
		})
		modelTurn.DurationMS = modelDuration
		toolCalls = append(toolCalls, *modelTurn)

		if len(response.ToolCalls) == 0 {
			answer := strings.TrimSpace(response.Text)
			plan.FinalAnswer = answer
			plan.Intent = assistantAgentIntent(plan, toolCalls)

			// The answer still has to survive the same checks a synthesized reply
			// did: no claiming a change was applied that no tool applied, no
			// ungrounded pipeline facts, proposal safety language where it is
			// required. A failure gets one corrective turn rather than a template.
			quality := assistantAssessAnswerQuality(plan, toolCalls, answer)
			if assistantAnswerQualityPasses(quality) || corrected {
				if !assistantAnswerQualityPasses(quality) {
					plan.FinalAnswer = assistantAgentQualityFallback(answer)
				}
				toolCalls = append(toolCalls, assistantAgentExecutionPlanActivity(plan, toolCalls))
				return assistantPlannerResult{Plan: plan, ToolCalls: toolCalls, Handled: true}
			}
			corrected = true
			plan.FinalAnswer = ""
			messages = append(messages,
				llmclient.ChatMessage{Role: llmclient.RoleAssistant, Content: answer},
				llmclient.ChatMessage{Role: llmclient.RoleUser, Content: assistantAgentQualityCorrection(quality)},
			)
			continue
		}

		assistantTurn := llmclient.ChatMessage{Role: llmclient.RoleAssistant, Content: response.Text}
		assistantTurn.ToolCalls = append(assistantTurn.ToolCalls, response.ToolCalls...)
		messages = append(messages, assistantTurn)

		for _, call := range response.ToolCalls {
			if remainingToolCalls <= 0 {
				messages = append(messages, llmclient.ChatMessage{
					Role:       llmclient.RoleTool,
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Content:    "This turn has no tool calls left. Answer from the evidence already gathered.",
				})
				continue
			}
			a.publishAssistantTurnProgress(ctx, conversation.ID, userID, assistantToolProgressLabel(call.Name))
			toolStartedAt := time.Now()
			activity := a.runAssistantAgentToolCall(ctx, subject, userID, conversation, plan, call)
			activity.DurationMS = time.Since(toolStartedAt).Milliseconds()
			remainingToolCalls--
			toolCalls = append(toolCalls, activity)
			for _, name := range assistantUnlockedToolNames(activity) {
				unlocked[name] = true
			}
			messages = append(messages, llmclient.ChatMessage{
				Role:       llmclient.RoleTool,
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Content:    assistantToolResultForModel(activity),
			})
		}
	}

	plan.Intent = assistantAgentIntent(plan, toolCalls)
	toolCalls = append(toolCalls, assistantAgentExecutionPlanActivity(plan, toolCalls))
	return assistantPlannerResult{Plan: plan, ToolCalls: toolCalls, Handled: true}
}

// assistantAgentCoreTools is the working set every turn starts with: the reads
// that answer most questions, and the door to the rest.
//
// The full catalogue is 213 tools and roughly 23,000 tokens of schema. Sending
// all of it on every model turn is what made a single question take minutes
// against a local model, because prefill is paid again on each step of the loop.
//
// This is not the scorer coming back. The scorer looked at the question and
// decided which tools that question deserved. This list does not look at the
// question at all — it is the same for every turn — and anything outside it is
// one nopsai.find_tools call away, made by the model when the model decides it
// needs something else.
var assistantAgentCoreTools = []string{
	"nopsai.find_tools",
	"nopsai.list_pipelines",
	"nopsai.get_pipeline",
	"nopsai.list_pipeline_runs",
	"nopsai.get_pipeline_run",
	"nopsai.get_pipeline_run_logs",
	"nopsai.analyze_run",
	"nopsai.analyze_pipeline",
	"nopsai.analyze_team",
	"nopsai.search_docs",
	"nopsai.get_feature_capabilities",
}

// assistantChatTools is the working set plus whatever the model has unlocked in
// this turn through find_tools. Every entry is still permission-filtered, and
// every call is checked again when it runs.
func (a *App) assistantChatTools(ctx context.Context, subject aaamodel.Subject, unlocked map[string]bool) []llmclient.ToolDefinition {
	offered := map[string]bool{}
	for _, name := range assistantAgentCoreTools {
		offered[name] = true
	}
	for name := range unlocked {
		offered[name] = true
	}
	tools := a.hostedMCPToolsForSubject(ctx, subject)
	definitions := make([]llmclient.ToolDefinition, 0, len(offered))
	for _, tool := range tools {
		if !offered[tool.Name] {
			continue
		}
		definitions = append(definitions, llmclient.ToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	return definitions
}

// assistantUnlockedToolNames reads the tools a find_tools result surfaced, so the
// model can call them natively on its next turn instead of being told they exist
// and then refused.
func assistantUnlockedToolNames(activity assistantToolActivity) []string {
	if activity.Name != "nopsai.find_tools" || activity.Status != assistantToolStatusSuccess {
		return nil
	}
	items, ok := activity.Output["tools"].([]map[string]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		if name, _ := item["name"].(string); strings.TrimSpace(name) != "" {
			names = append(names, name)
		}
	}
	return names
}

// assistantChatHistory replays the stored conversation. Only the text is
// replayed: earlier tool calls are summarized in the messages that reported
// them, and re-sending their raw output would spend the window on evidence the
// model has already used.
func assistantChatHistory(messages []assistantMessage) []llmclient.ChatMessage {
	start := len(messages) - assistantPromptHistoryLimit
	if start < 0 {
		start = 0
	}
	history := make([]llmclient.ChatMessage, 0, len(messages)-start)
	for _, message := range messages[start:] {
		role := strings.TrimSpace(message.Role)
		text := strings.TrimSpace(message.Content)
		if text == "" {
			continue
		}
		switch role {
		case "assistant":
			history = append(history, llmclient.ChatMessage{
				Role:    llmclient.RoleAssistant,
				Content: assistantTruncateHistoryContent(text),
			})
		case "user":
			history = append(history, llmclient.ChatMessage{
				Role:    llmclient.RoleUser,
				Content: assistantTruncateHistoryContent(text),
			})
		}
	}
	return history
}

// assistantToolResultForModel renders one tool result as the model's input. A
// denied or failed call is reported as such rather than dropped, so the model
// can say what it could not read instead of inventing what it might have said.
func assistantToolResultForModel(activity assistantToolActivity) string {
	payload := map[string]any{"status": activity.Status}
	if len(activity.Output) > 0 {
		payload["output"] = activity.Output
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return `{"status":"error","output":{"error":"tool result could not be encoded"}}`
	}
	if len(encoded) <= assistantMaxToolResultChars {
		return string(encoded)
	}
	return string(encoded[:assistantMaxToolResultChars]) + `… (truncated)"}`
}

// assistantAgentIntent labels the turn for memory and for the activity trail.
// It describes what happened, after the fact; it never decides what may happen.
func assistantAgentIntent(plan assistantTurnPlan, toolCalls []assistantToolActivity) string {
	if strings.TrimSpace(plan.Intent) != "" && plan.Intent != "llm_planned" {
		return plan.Intent
	}
	for _, call := range assistantEvidenceToolCalls(toolCalls) {
		if name := strings.TrimSpace(call.Name); name != "" {
			return strings.TrimPrefix(name, "nopsai.")
		}
	}
	return "answer"
}

// assistantAgentSystemPrompt carries policy, not routing. It says who the
// assistant is, what it may claim, when it must stop and ask, and what is in
// scope. It deliberately contains no rule of the form "for question type X call
// tool Y": choosing tools is the model's work, and the tool descriptions are
// what it chooses from.
func assistantAgentSystemPrompt(conversation assistantConversation, plan assistantTurnPlan) string {
	context := map[string]any{
		"conversation_memory": normalizeAssistantMemory(conversation.Memory),
		"docs_version":        conversation.DocsVersion,
	}
	if pageContext := assistantPageContextPromptMap(plan.PageContext); len(pageContext) > 0 {
		context["page_context"] = pageContext
	}
	if extracted := assistantExtractedContext(plan); len(extracted) > 0 {
		context["extracted_context"] = extracted
	}
	raw, _ := json.Marshal(context)

	return strings.TrimSpace(`You are the NopsAI assistant for an enterprise CI/CD and GitOps platform.

Work by gathering evidence and reading it. Call the tools you need, look at what they return, call more if the answer is not there yet, then answer.
The tools listed for you are the common ones. NopsAI has many more — triggers, schedules, scopes, variables, secrets, teams, dashboards, credentials, monitoring, GitOps proposals. When the tool you need is not listed, call nopsai.find_tools with a short description of what you want to do; the tools it returns become callable immediately. The decisive detail usually sits inside the evidence rather than in a summary of it — when a run fails, the reason is a line in its logs, so read them rather than reporting that the run failed.

Answer any question inside the NopsAI world: the platform, its pipelines, runs, steps, triggers, schedules, scopes, variables, secrets, teams, GitOps, CI/CD, and the systems a pipeline touches. When no single tool answers a question, combine the ones that exist. A question outside that world, such as cooking or general trivia, gets one short sentence saying it is out of scope and an offer to help with NopsAI instead.

Use only what the tools returned, the conversation, and the context below. Never invent runs, permissions, approvals, costs, logs, or applied changes. When a tool is denied or fails, say so plainly and answer with what you do have.
Generated pipeline YAML, trigger edits, and schedule edits are proposals. Never say a change was applied unless a tool result says it was applied.
A mutating tool needs the user's explicit confirmation. Without it, explain what you would change and ask; do not call the tool with confirm set.
When you calculate, compare, or estimate, label the data source and your confidence, and separate tool-backed facts from your own assumptions. For costs, never invent pricing: show the formula and its assumptions when prices are absent.

Keep answers concise and operational. When the evidence supports it, use short sections named Summary, Evidence, and Recommended next step.

Context:
` + string(raw))
}

// assistantExtractedContext is what the turn already knows about its target: the
// run or pipeline the user is looking at, the scope they named, whether they
// confirmed a change. It is context for the model, not a routing decision — the
// model still chooses what to do with it.
func assistantExtractedContext(plan assistantTurnPlan) map[string]any {
	extracted := map[string]any{}
	for key, value := range map[string]string{
		"run_id":      plan.RunID,
		"pipeline":    plan.PipelineID,
		"scope":       plan.Scope,
		"repository":  plan.Repository,
		"schedule_id": plan.ScheduleID,
		"api_method":  plan.APIMethod,
		"api_path":    plan.APIPath,
	} {
		if strings.TrimSpace(value) != "" {
			extracted[key] = value
		}
	}
	if plan.YAML != "" {
		extracted["yaml_present"] = true
	}
	if plan.UserConfirmed {
		extracted["user_confirmed_mutation"] = true
	}
	return extracted
}

// runAssistantAgentToolCall is the boundary every model-chosen call crosses.
//
// The confirmation gate used to sit in plan validation, which ran before any
// tool did. There is no plan now, so the gate moved here, onto the call itself:
// a mutating tool that is not a proposal is refused unless the deployment says
// confirmation is not required and the user confirmed in this turn. The refusal
// keeps the wording the confirmation prompt matches on, so a refused mutation
// still becomes "confirm this and I will apply it" rather than a dead end.
func (a *App) runAssistantAgentToolCall(
	ctx context.Context,
	subject aaamodel.Subject,
	userID string,
	conversation assistantConversation,
	plan assistantTurnPlan,
	call llmclient.ToolCall,
) assistantToolActivity {
	args := cloneAssistantArgs(call.Arguments)
	if err := a.assistantAgentMutationRefusal(ctx, subject, plan, call.Name, args); err != nil {
		return assistantToolActivity{
			Name:       call.Name,
			Input:      args,
			Output:     map[string]any{"error": err.Error()},
			Status:     assistantToolStatusDenied,
			Source:     "mcp",
			Phase:      "evidence",
			Confidence: "low",
			Purpose:    "Run a hosted MCP tool with current-user authorization.",
		}
	}
	return a.runAssistantHostedMCPTool(ctx, subject, userID, conversation.ID, call.Name, args)
}

// assistantAgentMutationRefusal returns the reason a mutating call must not run
// yet, or nil when it may. A tool this subject cannot even see is refused too:
// failing to evaluate a guard is never the same as passing it.
func (a *App) assistantAgentMutationRefusal(
	ctx context.Context,
	subject aaamodel.Subject,
	plan assistantTurnPlan,
	toolName string,
	args map[string]any,
) error {
	tool, ok := a.hostedMCPToolByName(ctx, subject, toolName)
	if !ok {
		return fmt.Errorf("assistant requested unavailable tool %q", toolName)
	}
	if !assistantToolRequiresActionExecution(tool) || assistantPlannedToolIsProposal(tool.Name) {
		return nil
	}
	userConfirmed := plan.UserConfirmed || assistantFeatureConfirmed(plan.LowerContent)
	if boolArg(args, "confirm", false) && !userConfirmed {
		return fmt.Errorf("assistant requested mutating tool %q with confirm:true but the user did not explicitly confirm", toolName)
	}
	if configuredAssistantRequiresConfirm(a) && !boolArg(args, "confirm", false) {
		return fmt.Errorf("assistant requested mutating tool %q without confirm:true", toolName)
	}
	return nil
}

// assistantAgentQualityCorrection tells the model what its answer got wrong, in
// the terms the check uses, so the retry fixes the claim rather than rewording it.
func assistantAgentQualityCorrection(quality assistantAnswerQuality) string {
	problems := []string{}
	if !quality.HasDirectAnswer {
		problems = append(problems, "it did not answer the question")
	}
	if !quality.NoFakeData {
		problems = append(problems, "it says a change was applied, but no tool applied one — describe the change as a proposal instead")
	}
	if !quality.PipelineGrounded {
		problems = append(problems, "it states pipeline facts that no successful tool result supports")
	}
	if !quality.EmptyResultExplained {
		problems = append(problems, "the evidence came back empty and the answer does not say so")
	}
	if !quality.SuggestedNextStep {
		problems = append(problems, "a proposal has to say that no changes were applied and what the reviewer should do next")
	}
	if !quality.UsedRelevantTools {
		problems = append(problems, "it rests on no tool evidence")
	}
	return "That answer cannot be sent: " + strings.Join(problems, "; ") + ". Answer again using only what the tool results support."
}

// assistantAgentQualityFallback is the last word when a corrected answer still
// fails. It keeps what the model said and withdraws the claim the check caught,
// rather than inventing a replacement answer.
func assistantAgentQualityFallback(answer string) string {
	trimmed := strings.TrimSpace(answer)
	if trimmed == "" {
		return "I could not produce an answer that the evidence supports. No changes were applied."
	}
	return trimmed + "\n\nTreat this as unverified: it goes beyond what the tool results support. No changes were applied."
}

// assistantAgentExecutionPlanActivity reports what the turn actually did.
//
// The panel this feeds used to be written before any tool ran, so it announced
// steps the turn might never take — a run analysis promising "LLM synthesis"
// that was then skipped. Built from the calls that happened, it can only
// describe the truth.
func assistantAgentExecutionPlanActivity(plan assistantTurnPlan, toolCalls []assistantToolActivity) assistantToolActivity {
	executionPlan := assistantExecutionPlan{
		Goal:    strings.TrimSpace(plan.LowerContent),
		Intent:  strings.TrimSpace(plan.Intent),
		Summary: "Evidence the assistant read to answer, in the order it read it.",
	}
	for _, call := range assistantEvidenceToolCalls(toolCalls) {
		executionPlan.Steps = append(executionPlan.Steps, assistantExecutionPlanStep{
			Index:      len(executionPlan.Steps) + 1,
			Title:      strings.TrimPrefix(call.Name, "nopsai."),
			Tool:       call.Name,
			Source:     "mcp",
			Phase:      "evidence",
			Confidence: call.Confidence,
			Status:     call.Status,
			DurationMS: call.DurationMS,
		})
	}
	if len(executionPlan.Steps) == 0 {
		executionPlan.Summary = "Answered from the conversation without reading new evidence."
	}
	modelMS, toolMS, modelTurns := assistantAgentTiming(toolCalls)
	executionPlan.Timing = assistantExecutionPlanTiming{
		ModelMS:    modelMS,
		ToolMS:     toolMS,
		ModelTurns: modelTurns,
		ToolCalls:  len(executionPlan.Steps),
	}
	return assistantToolActivity{
		Name:       assistantExecutionPlanToolName,
		Status:     assistantToolStatusSuccess,
		DurationMS: modelMS + toolMS,
		Output: map[string]any{
			"execution_plan": executionPlan,
			"applied":        false,
		},
		ResourceURIs: []string{"nopsai://assistant/execution-plan"},
		Source:       "llm",
		Phase:        "planning",
		Confidence:   "high",
		Purpose:      "Show which evidence the answer rests on.",
	}
}

// assistantAgentTiming splits a turn's wall time between the provider and the
// tools, counting the model turns it took to get there.
func assistantAgentTiming(toolCalls []assistantToolActivity) (modelMS int64, toolMS int64, modelTurns int) {
	for _, call := range toolCalls {
		switch call.Name {
		case assistantLLMToolName:
			modelMS += call.DurationMS
			modelTurns++
		case assistantExecutionPlanToolName:
		default:
			toolMS += call.DurationMS
		}
	}
	return modelMS, toolMS, modelTurns
}

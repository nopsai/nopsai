package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"nopsai/services/aaa/pkg/model"
)

const assistantPendingConfirmationEntityKey = "pending_confirmation"

var (
	assistantValueAssignmentPattern = regexp.MustCompile(`(?i)\b([a-zA-Z_][a-zA-Z0-9_.-]{0,127})\s*=\s*([^\s,;]+)`)
	assistantValueScopePattern      = regexp.MustCompile(`(?i)\b(?:scope|secret|secrets|variable|variables|env var|environment variable)\s+([a-zA-Z0-9][a-zA-Z0-9._/-]{0,126})\b`)
)

type assistantPendingConfirmation struct {
	Tool    string         `json:"tool"`
	Args    map[string]any `json:"args"`
	Kind    string         `json:"kind,omitempty"`
	Action  string         `json:"action,omitempty"`
	Summary string         `json:"summary,omitempty"`
}

func (a *App) handleAssistantPendingConfirmation(
	ctx context.Context,
	subject model.Subject,
	userID string,
	conversation assistantConversation,
	content string,
) (assistantOrchestrationResult, bool) {
	memory := normalizeAssistantMemory(conversation.Memory)
	pending, ok := assistantPendingConfirmationFromMemory(memory)
	if !ok {
		return assistantOrchestrationResult{}, false
	}
	if assistantUserCancelledPendingConfirmation(content) {
		memory = assistantClearPendingConfirmation(memory)
		memory.ConversationID = conversation.ID
		memory.Summary = "Cancelled pending assistant confirmation."
		return assistantOrchestrationResult{
			Reply:     "Cancelled the pending action. No changes were applied.",
			ToolCalls: []assistantToolActivity{},
			Memory:    normalizeAssistantMemory(memory),
		}, true
	}
	if assistantUserConfirmedPendingConfirmation(content) {
		args := cloneAssistantArgs(pending.Args)
		args["confirm"] = true
		call := a.runAssistantHostedMCPTool(ctx, subject, userID, conversation.ID, pending.Tool, args)
		memory = assistantClearPendingConfirmation(memory)
		memory = assistantMemoryAfterTools(memory, assistantTurnPlan{Intent: "feature_tool"}, []assistantToolActivity{call})
		memory.ConversationID = conversation.ID
		return assistantOrchestrationResult{
			Reply:     composeFeatureToolReply([]assistantToolActivity{call}),
			ToolCalls: []assistantToolActivity{call},
			Memory:    normalizeAssistantMemory(memory),
		}, true
	}
	if updated, changed := assistantUpdatePendingConfirmationFromContent(pending, content); changed {
		pending = updated
		memory = assistantSetPendingConfirmation(memory, pending)
	}
	memory.ConversationID = conversation.ID
	return assistantOrchestrationResult{
		Reply:     assistantPendingConfirmationPrompt(pending),
		ToolCalls: []assistantToolActivity{},
		Memory:    normalizeAssistantMemory(memory),
	}, true
}

func (a *App) handleAssistantDirectValueConfirmation(
	ctx context.Context,
	subject model.Subject,
	conversation assistantConversation,
	content string,
) (assistantOrchestrationResult, bool) {
	pending, ok := assistantDirectValueConfirmationFromContent(content, conversation.Memory)
	if !ok {
		return assistantOrchestrationResult{}, false
	}
	if _, ok := a.hostedMCPToolByName(ctx, subject, pending.Tool); !ok {
		memory := normalizeAssistantMemory(conversation.Memory)
		memory.ConversationID = conversation.ID
		reply := fmt.Sprintf("I did not use GitOps because you did not ask for a GitOps proposal. The direct MCP %s %s tool is not available in this session, likely because assistant action execution or AAA permission is disabled. Enable direct action execution/permission, or explicitly ask for a GitOps proposal if that is the workflow you want. No changes were applied.", pending.Kind, pending.Action)
		return assistantOrchestrationResult{
			Reply:     reply,
			ToolCalls: []assistantToolActivity{},
			Memory:    normalizeAssistantMemory(memory),
		}, true
	}
	memory := assistantSetPendingConfirmation(normalizeAssistantMemory(conversation.Memory), pending)
	memory.ConversationID = conversation.ID
	memory.Summary = "Waiting for explicit confirmation before applying a direct MCP " + pending.Kind + " " + pending.Action + "."
	return assistantOrchestrationResult{
		Reply:     assistantPendingConfirmationPrompt(pending),
		ToolCalls: []assistantToolActivity{},
		Memory:    normalizeAssistantMemory(memory),
	}, true
}

func (a *App) assistantPendingConfirmationFromDeniedPlan(ctx context.Context, subject model.Subject, plan assistantTurnPlan, toolCalls []assistantToolActivity) (assistantPendingConfirmation, bool) {
	denial, ok := assistantFirstPlanDenial(toolCalls)
	if !ok || !assistantPlanDenialIsMissingConfirm(denial) || len(plan.Steps) != 1 {
		return assistantPendingConfirmation{}, false
	}
	step := plan.Steps[0]
	tool, ok := a.hostedMCPToolByName(ctx, subject, step.ToolName)
	if !ok || !assistantToolRequiresActionExecution(tool) || assistantPlannedToolIsProposal(tool.Name) {
		return assistantPendingConfirmation{}, false
	}
	args := cloneAssistantArgs(step.Args)
	delete(args, "confirm")
	kind, action := assistantPendingKindActionForTool(tool.Name)
	return assistantPendingConfirmation{
		Tool:    tool.Name,
		Args:    args,
		Kind:    kind,
		Action:  action,
		Summary: assistantToolConfirmationSummary(tool.Name, args),
	}, true
}

func assistantDirectValueConfirmationFromContent(content string, memory assistantConversationMemory) (assistantPendingConfirmation, bool) {
	lower := strings.ToLower(strings.TrimSpace(content))
	if assistantPlannerWantsGitOpsProposalSchema(lower) || !assistantPlannerWantsWriteSchema(lower) {
		return assistantPendingConfirmation{}, false
	}
	kind := ""
	switch {
	case assistantTextHasAny(lower, "secret", "secrets"):
		kind = "secret"
	case assistantTextHasAny(lower, "env var", "environment variable", "variable", "variables", "var ", "_var"):
		kind = "variable"
	default:
		return assistantPendingConfirmation{}, false
	}
	name, value := assistantNameValueFromContent(content)
	if name == "" || value == "" {
		return assistantPendingConfirmation{}, false
	}
	scope := assistantScopeFromMessage(content)
	if scope == "" {
		scope = assistantValueScopeFromContent(content)
	}
	if scope == "" {
		scope = strings.Trim(strings.TrimSpace(memory.SelectedScope), "/")
	}
	if scope == "" {
		scope = defaultRuntimeScope
	}
	tool := "nopsai.write_" + kind + "_value"
	args := map[string]any{"value": value, "scope": scope}
	if kind == "secret" {
		args["secret_name"] = name
	} else {
		args["variable_name"] = name
	}
	return assistantPendingConfirmation{
		Tool:    tool,
		Args:    args,
		Kind:    kind,
		Action:  "write",
		Summary: fmt.Sprintf("Set %s %s in scope %s", kind, name, scope),
	}, true
}

func assistantPendingConfirmationFromMemory(memory assistantConversationMemory) (assistantPendingConfirmation, bool) {
	raw, ok := memory.Entities[assistantPendingConfirmationEntityKey]
	if !ok || raw == nil {
		return assistantPendingConfirmation{}, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return assistantPendingConfirmation{}, false
	}
	var pending assistantPendingConfirmation
	if err := json.Unmarshal(encoded, &pending); err != nil {
		return assistantPendingConfirmation{}, false
	}
	pending.Tool = strings.TrimSpace(pending.Tool)
	if pending.Tool == "" {
		return assistantPendingConfirmation{}, false
	}
	if pending.Args == nil {
		pending.Args = map[string]any{}
	}
	return pending, true
}

func assistantSetPendingConfirmation(memory assistantConversationMemory, pending assistantPendingConfirmation) assistantConversationMemory {
	memory = normalizeAssistantMemory(memory)
	if pending.Args == nil {
		pending.Args = map[string]any{}
	}
	memory.Entities[assistantPendingConfirmationEntityKey] = pending
	return memory
}

func assistantClearPendingConfirmation(memory assistantConversationMemory) assistantConversationMemory {
	memory = normalizeAssistantMemory(memory)
	delete(memory.Entities, assistantPendingConfirmationEntityKey)
	return memory
}

func assistantUpdatePendingConfirmationFromContent(pending assistantPendingConfirmation, content string) (assistantPendingConfirmation, bool) {
	if pending.Kind != "secret" && pending.Kind != "variable" {
		return pending, false
	}
	changed := false
	name, value := assistantNameValueFromContent(content)
	if name == "" || value == "" {
		name, value, scope := assistantPositionalValueFields(content)
		if name != "" {
			pending.Args[pending.Kind+"_name"] = name
			changed = true
		}
		if value != "" {
			pending.Args["value"] = value
			changed = true
		}
		if scope != "" {
			pending.Args["scope"] = scope
			changed = true
		}
	} else {
		pending.Args[pending.Kind+"_name"] = name
		pending.Args["value"] = value
		changed = true
	}
	if scope := firstNonEmptyString(assistantScopeFromMessage(content), assistantValueScopeFromContent(content)); scope != "" {
		pending.Args["scope"] = scope
		changed = true
	}
	pending.Summary = assistantPendingConfirmationSummary(pending)
	return pending, changed
}

func assistantPendingConfirmationPrompt(pending assistantPendingConfirmation) string {
	lines := []string{"Please confirm before I apply this direct MCP change:"}
	name := assistantPendingConfirmationName(pending)
	scope := strings.Trim(strings.TrimSpace(stringArg(pending.Args, "scope")), "/")
	if scope == "" && (pending.Kind == "secret" || pending.Kind == "variable") {
		scope = defaultRuntimeScope
	}
	if pending.Kind != "" {
		lines = append(lines, "- Type: "+pending.Kind+" "+pending.Action)
	} else if pending.Tool != "" {
		lines = append(lines, "- Tool: "+pending.Tool)
	}
	if name != "" {
		lines = append(lines, "- Name: "+name)
	}
	if scope != "" {
		lines = append(lines, "- Scope: "+scope)
	}
	if pending.Kind == "secret" && pending.Action == "write" {
		lines = append(lines, "- Value: provided, not shown")
	} else if pending.Action == "write" {
		value := stringArg(pending.Args, "value")
		if value == "" {
			value = stringArg(pending.Args, "body")
		}
		if value != "" {
			lines = append(lines, "- Value: "+value)
		}
	} else if value := stringArg(pending.Args, "value"); value != "" && pending.Kind == "variable" {
		lines = append(lines, "- Value: "+value)
	}
	if pending.Kind == "" && pending.Summary != "" {
		lines = append(lines, "- Change: "+pending.Summary)
	}
	lines = append(lines, "Reply `confirm` to apply, or `cancel` to discard. No changes were applied.")
	return strings.Join(lines, "\n")
}

func assistantPendingConfirmationSummary(pending assistantPendingConfirmation) string {
	name := assistantPendingConfirmationName(pending)
	scope := strings.Trim(strings.TrimSpace(stringArg(pending.Args, "scope")), "/")
	if scope == "" {
		scope = defaultRuntimeScope
	}
	if pending.Kind == "" || name == "" {
		return strings.TrimSpace(pending.Summary)
	}
	return fmt.Sprintf("Set %s %s in scope %s", pending.Kind, name, scope)
}

func assistantPendingConfirmationName(pending assistantPendingConfirmation) string {
	if pending.Kind == "secret" {
		return firstNonEmptyString(stringArg(pending.Args, "secret_name"), stringArg(pending.Args, "name"))
	}
	if pending.Kind == "variable" {
		return firstNonEmptyString(stringArg(pending.Args, "variable_name"), stringArg(pending.Args, "name"))
	}
	return firstNonEmptyString(stringArg(pending.Args, "name"), stringArg(pending.Args, "id"))
}

func assistantPlanDenialIsMissingConfirm(call assistantToolActivity) bool {
	reason := strings.ToLower(strings.TrimSpace(assistantOutputString(call.Output, "error")))
	return strings.Contains(reason, "without confirm:true")
}

func assistantPendingKindActionForTool(toolName string) (string, string) {
	switch toolName {
	case "nopsai.write_secret_value":
		return "secret", "write"
	case "nopsai.delete_secret_value":
		return "secret", "delete"
	case "nopsai.write_variable_value":
		return "variable", "write"
	case "nopsai.delete_variable_value":
		return "variable", "delete"
	default:
		return "", ""
	}
}

func assistantToolConfirmationSummary(toolName string, args map[string]any) string {
	name := strings.TrimPrefix(strings.TrimSpace(toolName), "nopsai.")
	name = strings.ReplaceAll(name, "_", " ")
	switch toolName {
	case "nopsai.write_secret_value", "nopsai.delete_secret_value":
		valueName := firstNonEmptyString(stringArg(args, "secret_name"), stringArg(args, "name"))
		if valueName != "" {
			return "Apply " + name + " for " + valueName
		}
	case "nopsai.write_variable_value", "nopsai.delete_variable_value":
		valueName := firstNonEmptyString(stringArg(args, "variable_name"), stringArg(args, "name"))
		if valueName != "" {
			return "Apply " + name + " for " + valueName
		}
	}
	if name == "" {
		return "Apply the pending MCP action"
	}
	return "Apply " + name
}

func assistantNameValueFromContent(content string) (string, string) {
	match := assistantValueAssignmentPattern.FindStringSubmatch(content)
	if len(match) < 3 {
		return "", ""
	}
	return assistantCleanValueToken(match[1]), assistantCleanValueToken(match[2])
}

func assistantValueScopeFromContent(content string) string {
	matches := assistantValueScopePattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return ""
	}
	for idx := len(matches) - 1; idx >= 0; idx-- {
		if len(matches[idx]) < 2 {
			continue
		}
		scope := strings.Trim(assistantCleanValueToken(matches[idx][1]), "/")
		if !assistantScopeCandidateIsGrammar(scope) {
			return scope
		}
	}
	return ""
}

func assistantPositionalValueFields(content string) (string, string, string) {
	fields := strings.Fields(content)
	cleaned := make([]string, 0, len(fields))
	for _, field := range fields {
		field = assistantCleanValueToken(field)
		if field == "" {
			continue
		}
		lower := strings.ToLower(field)
		switch lower {
		case "confirm", "confirmed", "yes", "apply", "execute", "please":
			continue
		default:
			cleaned = append(cleaned, field)
		}
	}
	if len(cleaned) < 3 {
		return "", "", ""
	}
	return cleaned[0], cleaned[1], cleaned[2]
}

func assistantCleanValueToken(value string) string {
	return strings.Trim(strings.TrimSpace(value), ".,;:!?\"'`")
}

func assistantUserConfirmedPendingConfirmation(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	switch lower {
	case "confirm", "confirmed", "yes confirm", "yes, confirm", "approve", "approved", "apply", "apply it", "execute", "execute it":
		return true
	default:
		return strings.Contains(lower, "i confirm") ||
			strings.Contains(lower, "i approve") ||
			strings.Contains(lower, "confirmed to execute") ||
			strings.Contains(lower, "approved to execute")
	}
}

func assistantUserCancelledPendingConfirmation(content string) bool {
	switch strings.ToLower(strings.TrimSpace(content)) {
	case "cancel", "cancel it", "stop", "discard", "never mind", "nevermind":
		return true
	default:
		return false
	}
}

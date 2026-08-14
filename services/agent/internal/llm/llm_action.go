package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	workspacectx "nopsai/services/agent/internal/workspace"

	"github.com/rs/zerolog/log"
)

func (c *LLMClient) GetAction(ctx context.Context, req *proto.GetActionRequest) (*proto.Action, error) {
	return c.GetActionWithAgentProfile(ctx, req, defaultAgentPromptProfile())
}

func (c *LLMClient) GetActionWithAgentProfile(ctx context.Context, req *proto.GetActionRequest, agentProfile AgentPromptProfile) (*proto.Action, error) {
	actionModel, err := c.getActionModel(ctx, c.buildPromptWithMCP(req, "", "", agentProfile), NewGoalSession(""))
	if err != nil {
		return nil, err
	}
	if err := c.enforceDuringPolicyReview(req, actionModel); err != nil {
		return nil, err
	}
	return actionModelToProto(actionModel)
}

func (c *LLMClient) GetActionWithMCP(ctx context.Context, req *proto.GetActionRequest, mcpRuntime *MCPTaskRuntime) (*proto.Action, error) {
	return c.GetActionWithMCPAndAgentProfile(ctx, req, mcpRuntime, defaultAgentPromptProfile())
}

func (c *LLMClient) GetActionWithMCPAndAgentProfile(ctx context.Context, req *proto.GetActionRequest, mcpRuntime *MCPTaskRuntime, agentProfile AgentPromptProfile) (*proto.Action, error) {
	return c.GetActionWithToolsAndAgentProfile(ctx, req, mcpRuntime, nil, agentProfile)
}

func (c *LLMClient) GetActionWithToolsAndAgentProfile(ctx context.Context, req *proto.GetActionRequest, mcpRuntime *MCPTaskRuntime, workspaceTools *workspacectx.Tools, agentProfile AgentPromptProfile) (*proto.Action, error) {
	goalSession := NewGoalSession("")
	toolTranscript := ""
	if mcpRuntime == nil {
		toolTranscript = ""
	} else {
		toolTranscript = mcpRuntime.ToolTranscript()
	}
	workspaceTranscript := ""
	if workspaceTools != nil {
		workspaceTranscript = workspaceTools.ToolTranscript()
	}
	successfulToolCalls := 0
	if mcpRuntime == nil {
		successfulToolCalls = 0
	} else {
		successfulToolCalls = mcpRuntime.SuccessfulToolCalls()
	}
	mustUseMCP := false
	if mcpRuntime == nil {
		mustUseMCP = false
	} else {
		mustUseMCP = mcpRuntime.RequiresToolCall() || goalRequiresMCPToolCall(req.GetGoal())
	}
	if mustUseMCP {
		logEvent := log.Info().Strs("mcp_profiles", mcpRuntime.Profiles())
		if c.profile != "" {
			logEvent = logEvent.Str("model", c.profile)
		}
		logEvent.Msg("MCP tool call is required before final action")
	}
	for toolCallCount := 0; toolCallCount <= maxMCPToolCallsPerAction; toolCallCount++ {
		actionModel, err := c.getActionModel(ctx, c.buildPromptWithTools(req, toolTranscript, mcpToolPrompt(mcpRuntime), workspaceTranscript, workspaceToolPrompt(workspaceTools), agentProfile), goalSession)
		if err != nil {
			return nil, err
		}
		if err := c.enforceDuringPolicyReview(req, actionModel); err != nil {
			return nil, err
		}
		if actionModel.Type == models.ActionTypeCallWorkspaceTool {
			if workspaceTools == nil || !workspaceTools.Enabled() {
				workspaceTranscript += "\nWorkspace tool call failed: workspace tools are not available\n"
				continue
			}
			if toolCallCount == maxMCPToolCallsPerAction {
				return nil, fmt.Errorf("workspace tool call limit exceeded")
			}
			toolAction := actionModel.WorkspaceToolAction
			if toolAction == nil {
				return nil, fmt.Errorf("CALL_WORKSPACE_TOOL action requires workspace_tool_action")
			}
			toolName := strings.TrimSpace(toolAction.Tool)
			logEvent := log.Info().Str("workspace_tool", toolName)
			if c.profile != "" {
				logEvent = logEvent.Str("model", c.profile)
			}
			logEvent.Msg("Calling workspace tool requested by LLM")
			result, err := workspaceTools.CallTool(ctx, toolName, toolAction.Arguments)
			if err != nil {
				workspaceTranscript += fmt.Sprintf("\nWorkspace tool call failed: tool=%s error=%s\n", toolName, err.Error())
				continue
			}
			resultLog := log.Info().
				Str("workspace_tool", toolName).
				Int("workspace_tool_result_bytes", len(result)).
				Int("workspace_successful_tool_calls", workspaceTools.SuccessfulToolCalls())
			if c.profile != "" {
				resultLog = resultLog.Str("model", c.profile)
			}
			resultLog.Msg("Workspace tool returned result to LLM")
			workspaceTranscript += formatWorkspaceToolResultTranscript(toolName, toolAction.Arguments, result)
			continue
		}
		if actionModel.Type != models.ActionTypeCallMCPTool {
			if mustUseMCP && successfulToolCalls == 0 {
				logEvent := log.Warn().Str("action_type", actionModel.Type)
				if c.profile != "" {
					logEvent = logEvent.Str("model", c.profile)
				}
				logEvent.Msg("LLM selected a final action before the required MCP tool call; retrying")
				toolTranscript += fmt.Sprintf(
					"\nMCP tool call required before final action: you selected %s before a successful MCP call. Choose CALL_MCP_TOOL now using one of the approved tools listed above.\n",
					actionModel.Type,
				)
				continue
			}
			if answer, ok := mcpCommandFallbackAnswer(req.GetGoal(), actionModel, mcpRuntime); ok {
				logEvent := log.Warn().
					Str("action_type", actionModel.Type).
					Str("command", actionModel.CommandAction.Command)
				if c.profile != "" {
					logEvent = logEvent.Str("model", c.profile)
				}
				logEvent.Msg("LLM selected a shell command that appears to bypass missing MCP capability; failing goal resolution")
				return nil, newNonRetryableGoalResolutionError("%s", answer)
			}
			if answer, ok := mcpFinalAnswerFailureReason(actionModel, mcpRuntime); ok {
				logEvent := log.Warn().Str("action_type", actionModel.Type)
				if c.profile != "" {
					logEvent = logEvent.Str("model", c.profile)
				}
				logEvent.Msg("LLM reported missing MCP capability or permission; failing goal resolution")
				return nil, newNonRetryableGoalResolutionError("%s", answer)
			}
			return actionModelToProto(actionModel)
		}
		if mcpRuntime == nil || !mcpRuntime.Enabled() {
			toolTranscript += "\nMCP tool call failed: external MCP tools are not available for this goal\n"
			continue
		}
		if toolCallCount == maxMCPToolCallsPerAction {
			return nil, fmt.Errorf("MCP tool call limit exceeded")
		}
		toolAction := actionModel.MCPToolAction
		if toolAction == nil {
			return nil, fmt.Errorf("CALL_MCP_TOOL action requires mcp_tool_action")
		}
		serverName := strings.TrimSpace(toolAction.Server)
		toolName := strings.TrimSpace(toolAction.Tool)
		logEvent := log.Info().Str("mcp_tool", toolName)
		if serverName != "" {
			logEvent = logEvent.Str("mcp_server", serverName)
		}
		if c.profile != "" {
			logEvent = logEvent.Str("model", c.profile)
		}
		logEvent.Msg("Calling MCP tool requested by LLM")
		result, err := mcpRuntime.CallTool(ctx, serverName, toolName, toolAction.Arguments)
		if err != nil {
			toolTranscript += fmt.Sprintf("\nMCP tool call failed: server=%s tool=%s error=%s\n", serverName, toolName, err.Error())
			continue
		}
		successfulToolCalls = mcpRuntime.SuccessfulToolCalls()
		toolTranscript += formatMCPToolResultTranscript(serverName, toolName, toolAction.Arguments, result)
	}
	return nil, fmt.Errorf("MCP tool call loop ended without a final action")
}

func (c *LLMClient) enforceDuringPolicyReview(req *proto.GetActionRequest, actionModel *models.Action) error {
	if req == nil || actionModel == nil || !policyReviewRequired(req.GetKnowledgeContext()) {
		return nil
	}
	interpretation := interpretPromptPolicyReview(req.GetKnowledgeContext(), actionModel.PolicyReview)
	logEvent := log.Info().
		Str("governance_level", governanceLevelFromPrompt(req.GetKnowledgeContext())).
		Str("policy_decision", interpretation.Decision).
		Str("action_type", actionModel.Type)
	if actionModel.PolicyReview != nil {
		if confidence := strings.TrimSpace(actionModel.PolicyReview.Confidence); confidence != "" {
			logEvent = logEvent.Str("policy_confidence", confidence)
		}
		if len(actionModel.PolicyReview.Refs) > 0 {
			logEvent = logEvent.Strs("policy_refs", actionModel.PolicyReview.Refs)
		}
	}
	if c != nil && c.profile != "" {
		logEvent = logEvent.Str("model", c.profile)
	}
	if !interpretation.Allowed {
		logEvent.Msg("During policy review blocked action")
		return governancePolicyError("during", req.GetKnowledgeContext(), actionModel.PolicyReview)
	}
	if interpretation.Warning {
		logEvent.Msg("During policy review allowed action with warning")
		return nil
	}
	logEvent.Msg("During policy review allowed action")
	return nil
}

func mcpToolPrompt(mcpRuntime *MCPTaskRuntime) string {
	if mcpRuntime == nil {
		return ""
	}
	return mcpRuntime.ToolPrompt()
}

func workspaceToolPrompt(workspaceTools *workspacectx.Tools) string {
	if workspaceTools == nil {
		return ""
	}
	return workspaceTools.ToolPrompt()
}

func formatWorkspaceToolResultTranscript(toolName string, arguments json.RawMessage, result json.RawMessage) string {
	return fmt.Sprintf(
		"\nWorkspace tool result: tool=%s arguments=%s result=%s\n",
		toolName,
		workspacectxJSONString(arguments, 2048),
		workspacectxJSONString(result, 24000),
	)
}

func workspacectxJSONString(raw json.RawMessage, max int) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "{}"
	}
	if max > 0 && len(trimmed) > max {
		return trimmed[:max] + "...[truncated]"
	}
	return trimmed
}

func mcpFinalAnswerFailureReason(actionModel *models.Action, mcpRuntime *MCPTaskRuntime) (string, bool) {
	if mcpRuntime == nil || !mcpRuntime.RequiresToolCall() || mcpRuntime.SuccessfulToolCalls() == 0 {
		return "", false
	}
	if actionModel == nil || actionModel.Type != models.ActionTypeReturnAnswer || actionModel.AnswerAction == nil {
		return "", false
	}
	answer := strings.TrimSpace(actionModel.AnswerAction.Answer)
	if answer == "" {
		return "", false
	}
	normalized := strings.ToLower(answer)
	inability := strings.Contains(normalized, "unable to") ||
		strings.Contains(normalized, "cannot ") ||
		strings.Contains(normalized, "can't ") ||
		strings.Contains(normalized, "could not") ||
		strings.Contains(normalized, "not able to")
	missingCapability := strings.Contains(normalized, "not available") ||
		strings.Contains(normalized, "not approved") ||
		strings.Contains(normalized, "not allowed") ||
		strings.Contains(normalized, "missing") ||
		strings.Contains(normalized, "permission") ||
		strings.Contains(normalized, "allowed mcp") ||
		strings.Contains(normalized, "approved mcp")
	mcpContext := strings.Contains(normalized, "mcp") ||
		strings.Contains(normalized, " tool") ||
		strings.Contains(normalized, "_tool") ||
		strings.Contains(normalized, "list_commits") ||
		strings.Contains(normalized, "get_commit")
	if inability && missingCapability && mcpContext {
		return answer, true
	}
	return "", false
}

func mcpCommandFallbackAnswer(goal string, actionModel *models.Action, mcpRuntime *MCPTaskRuntime) (string, bool) {
	if mcpRuntime == nil || !mcpRuntime.RequiresToolCall() || mcpRuntime.SuccessfulToolCalls() == 0 {
		return "", false
	}
	if actionModel == nil || actionModel.Type != models.ActionTypeExecuteCommand || actionModel.CommandAction == nil {
		return "", false
	}
	command := strings.TrimSpace(actionModel.CommandAction.Command)
	if command == "" {
		return "", false
	}
	capability, suggestion, ok := mcpCommandFallbackCapability(goal, command)
	if !ok {
		return "", false
	}

	profiles := strings.Join(mcpRuntime.Profiles(), ", ")
	if strings.TrimSpace(profiles) == "" {
		profiles = "the selected MCP profile"
	}

	answer := fmt.Sprintf(
		"I used the approved MCP profile (%s), but I cannot complete the requested %s with the MCP tools approved for this run. The model tried to fall back to a shell command (`%s`), but that would bypass the hosted MCP capability/permission boundary and may fail outside the right checkout. Add an approved MCP tool for this capability, such as %s, then rerun.",
		profiles,
		capability,
		command,
		suggestion,
	)
	if availableTools := summarizeMCPToolNames(mcpRuntime, 20); availableTools != "" {
		answer += " Approved MCP tools currently available: " + availableTools + "."
	}
	return answer, true
}

func mcpCommandFallbackCapability(goal, command string) (string, string, bool) {
	normalizedCommand := strings.ToLower(strings.Join(strings.Fields(command), " "))
	normalizedGoal := strings.ToLower(goal)
	if strings.Contains(normalizedCommand, "git log") ||
		strings.Contains(normalizedCommand, "git show") ||
		strings.Contains(normalizedCommand, "git rev-list") ||
		strings.Contains(normalizedGoal, "commit") && strings.Contains(normalizedCommand, "git ") {
		return "Git commit history", "`list_commits` or `get_commit` for GitHub commit history", true
	}
	if strings.Contains(normalizedCommand, "gh ") ||
		strings.Contains(normalizedCommand, "api.github.com") {
		return "GitHub data", "the matching GitHub MCP tool/profile permission", true
	}
	return "", "", false
}

func summarizeMCPToolNames(mcpRuntime *MCPTaskRuntime, limit int) string {
	if mcpRuntime == nil || len(mcpRuntime.tools) == 0 {
		return ""
	}
	names := make([]string, 0, len(mcpRuntime.tools))
	for _, tool := range mcpRuntime.tools {
		server := strings.TrimSpace(tool.Server)
		name := strings.TrimSpace(tool.Name)
		if server == "" {
			names = append(names, name)
			continue
		}
		names = append(names, server+"/"+name)
	}
	sort.Strings(names)
	if limit > 0 && len(names) > limit {
		remaining := len(names) - limit
		names = append(names[:limit], fmt.Sprintf("... and %d more", remaining))
	}
	return strings.Join(names, ", ")
}

func goalRequiresMCPToolCall(goal string) bool {
	normalized := strings.ToLower(strings.TrimSpace(goal))
	if normalized == "" || !strings.Contains(normalized, "mcp") {
		return false
	}
	for _, phrase := range []string{"do not use mcp", "don't use mcp", "without mcp", "no mcp"} {
		if strings.Contains(normalized, phrase) {
			return false
		}
	}
	for _, phrase := range []string{
		"must call",
		"call at least one",
		"before returning",
		"before you return",
		"use mcp",
		"use the approved",
		"use approved",
		"mcp tool",
		"mcp tools",
	} {
		if strings.Contains(normalized, phrase) {
			return true
		}
	}
	return false
}

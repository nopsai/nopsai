package llm

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"

	"github.com/rs/zerolog/log"
)

func (c *LLMClient) getActionModel(ctx context.Context, prompt string) (*models.Action, error) {
	responseText, err := c.providerClient.Complete(ctx, prompt)
	if err != nil {
		logEvent := log.Error().Err(err).Str("provider", c.provider)
		if c.profile != "" {
			logEvent = logEvent.Str("llm_profile", c.profile)
		}
		logEvent.Msg("Error calling LLM provider for GetAction")
		return nil, err
	}

	return decodeActionResponse(responseText)
}

func actionModelToProto(actionModel *models.Action) (*proto.Action, error) {
	if actionModel == nil {
		return nil, fmt.Errorf("LLM returned an empty action")
	}
	protoAction := &proto.Action{Type: string(actionModel.Type)}
	switch actionModel.Type {
	case models.ActionTypeExecuteCommand:
		protoAction.Payload = &proto.Action_CommandAction{CommandAction: &proto.CommandAction{Command: actionModel.CommandAction.Command}}
	case models.ActionTypeReplaceFile:
		protoAction.Payload = &proto.Action_FileAction{FileAction: &proto.FileAction{Path: actionModel.FileAction.Path, Content: actionModel.FileAction.Content}}
	case models.ActionTypeReturnAnswer:
		protoAction.Payload = &proto.Action_AnswerAction{AnswerAction: &proto.AnswerAction{Answer: actionModel.AnswerAction.Answer}}
	default:
		return nil, fmt.Errorf("LLM returned unsupported final action type %q", actionModel.Type)
	}

	return protoAction, nil
}

func (c *LLMClient) EvaluateCondition(ctx context.Context, req *proto.ConditionRequest) (*proto.ConditionResponse, error) {
	return c.EvaluateConditionWithAgentProfile(ctx, req, defaultAgentPromptProfile())
}

func (c *LLMClient) EvaluateConditionWithAgentProfile(ctx context.Context, req *proto.ConditionRequest, agentProfile AgentPromptProfile) (*proto.ConditionResponse, error) {
	prompt := c.buildConditionPrompt(req, agentProfile)

	responseText, err := c.providerClient.Complete(ctx, prompt)
	if err != nil {
		logEvent := log.Error().Err(err).Str("provider", c.provider)
		if c.profile != "" {
			logEvent = logEvent.Str("llm_profile", c.profile)
		}
		logEvent.Msg("Error calling LLM provider for EvaluateCondition")
		return &proto.ConditionResponse{Result: false}, err
	}

	result, err := parseBooleanText(responseText)
	if err != nil {
		return &proto.ConditionResponse{Result: false}, err
	}
	return &proto.ConditionResponse{Result: result}, nil
}

func (c *LLMClient) buildConditionPrompt(req *proto.ConditionRequest, agentProfile AgentPromptProfile) string {
	history := req.GetHistory()
	if history == "" {
		history = "No history yet."
	}

	promptTemplate := `%s

Your task is to answer a YES/NO question based on the provided context.
You must only respond with the word "true" or "false" and nothing else.

---
%s
---
%s
---
**Execution History (Previous Steps):**
%s
---
**Question:**
"%s"
---
Based on the context, is the answer to the question YES or NO? Respond with only "true" or "false".`

	fullPrompt := fmt.Sprintf(promptTemplate, formatAgentPromptProfile(agentProfile), buildVariablesSection(req.GetVariables()), buildKnowledgeContextSection(req.GetKnowledgeContext()), history, req.GetGoal())
	logEvent := log.Debug().Str("provider", c.provider)
	if c.profile != "" {
		logEvent = logEvent.Str("llm_profile", c.profile)
	}
	logEvent.Msgf("Condition prompt:\n%s", fullPrompt)
	return fullPrompt
}

func (c *LLMClient) buildPrompt(req *proto.GetActionRequest) string {
	return c.buildPromptWithMCP(req, "", "", defaultAgentPromptProfile())
}

func (c *LLMClient) buildPromptWithMCP(req *proto.GetActionRequest, mcpTranscript, mcpToolPrompt string, agentProfile AgentPromptProfile) string {
	history := req.GetHistory()
	if history == "" {
		history = "No history yet."
	}
	if strings.TrimSpace(mcpTranscript) != "" {
		history = history + "\n--- MCP Tool Results For Current Goal ---\n" + strings.TrimSpace(mcpTranscript)
	}
	mcpSection := strings.TrimSpace(mcpToolPrompt)
	if mcpSection == "" {
		mcpSection = "**External MCP Tools:**\nNo external MCP tools are available for this goal."
	}

	promptTemplate := `%s

Your task is to achieve a user's goal by choosing the correct action from a toolkit.
You must only respond with a single JSON object. Inside this object, there should be a single key "action" which contains the action to perform.

Here are the available actions:
1. **EXECUTE_COMMAND**: {"action": {"type": "EXECUTE_COMMAND", "command_action": {"command": "your-bash-command-here"}}}
2. **REPLACE_FILE**: {"action": {"type": "REPLACE_FILE", "file_action": {"path": "./path/to/file.txt", "content": "The full new content of the file."}}}
3. **RETURN_ANSWER**: {"action": {"type": "RETURN_ANSWER", "answer_action": {"answer": "The answer to the user's question."}}}
4. **CALL_MCP_TOOL**: {"action": {"type": "CALL_MCP_TOOL", "mcp_tool_action": {"server": "server-name", "tool": "tool_name", "arguments": {}}}}
---
%s
---
%s
---
%s
---
%s
---
**Execution History (Previous Steps):**
%s
---
**Current Goal:**
"%s"
---
Now, choose the single best action from your toolkit and provide the response in the required JSON format.`

	fullPrompt := fmt.Sprintf(
		promptTemplate,
		formatAgentPromptProfile(agentProfile),
		buildVariablesSection(req.GetVariables()),
		buildKnowledgeContextSection(req.GetKnowledgeContext()),
		buildDirectoryListingSection(req.GetDirectoryListing()),
		mcpSection,
		history,
		req.GetGoal(),
	)
	logEvent := log.Debug().Str("provider", c.provider)
	if c.profile != "" {
		logEvent = logEvent.Str("llm_profile", c.profile)
	}
	logEvent.Msgf("Full prompt:\n%s", fullPrompt)
	return fullPrompt
}

func buildKnowledgeContextSection(knowledgeContext string) string {
	var builder strings.Builder
	builder.WriteString("**Knowledge Context:**\n")
	trimmed := strings.TrimSpace(knowledgeContext)
	if trimmed == "" {
		builder.WriteString("No knowledge context provided.\n")
		return builder.String()
	}
	builder.WriteString(trimmed)
	builder.WriteString("\n")
	return builder.String()
}

func buildVariablesSection(variables map[string]string) string {
	var builder strings.Builder
	builder.WriteString("**Variables:**\n")
	if len(variables) == 0 {
		builder.WriteString("No variables provided.\n")
		return builder.String()
	}

	keys := make([]string, 0, len(variables))
	for key := range variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		builder.WriteString(fmt.Sprintf("- %s: %s\n", key, variables[key]))
	}

	return builder.String()
}

func buildDirectoryListingSection(directoryListing map[string]string) string {
	var builder strings.Builder
	builder.WriteString("**Working Directory Contents:**\n")
	if len(directoryListing) == 0 {
		builder.WriteString("Directory is empty.\n")
		return builder.String()
	}

	names := make([]string, 0, len(directoryListing))
	for name := range directoryListing {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		builder.WriteString(fmt.Sprintf("--- File: %s ---\n%s\n", name, directoryListing[name]))
	}

	return builder.String()
}

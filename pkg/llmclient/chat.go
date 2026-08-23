package llmclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"nopsai/config"
)

// Tool-calling chat.
//
// Complete sends one string and reads one string back, which is why everything
// above it had to serialize a tool catalogue into prompt text, ask the model to
// answer in JSON, and then parse and repair that answer. Chat passes the tools
// to the provider instead, so the provider constrains the model to a well-formed
// call and hands it back as data. The three wire formats — OpenAI functions,
// Anthropic tool_use, Gemini functionDeclarations — are encoded here so callers
// work in one vocabulary.

// ToolDefinition is one tool offered to the model. InputSchema is a JSON Schema
// object describing the arguments.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

// ToolCall is the model asking for a tool to run. ID is the provider's handle
// for the call and must come back with the result.
type ToolCall struct {
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// ChatMessage is one turn. An assistant turn may carry ToolCalls; a tool turn
// carries the result of exactly one call, identified by ToolCallID.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolName   string     `json:"tool_name,omitempty"`
}

// Message roles. Tool results use RoleTool regardless of how the provider
// carries them on the wire.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

type ChatRequest struct {
	System   string
	Messages []ChatMessage
	Tools    []ToolDefinition
}

// ChatResponse is one model turn. Text and ToolCalls can both be present: a
// model may explain what it is about to do and call a tool in the same turn.
type ChatResponse struct {
	Text      string
	ToolCalls []ToolCall
	Usage     Usage
	Stop      string
}

// Chat runs one turn of a tool-using conversation.
func (c *Client) Chat(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	if len(request.Messages) == 0 {
		return ChatResponse{}, fmt.Errorf("at least one message is required")
	}
	switch c.options.Provider {
	case config.LLMProviderGemini:
		return c.chatGemini(ctx, request)
	case config.LLMProviderLMStudio:
		return c.chatLMStudio(ctx, request)
	case config.LLMProviderOpenAI,
		config.LLMProviderGroq,
		config.LLMProviderMistral,
		config.LLMProviderOllama,
		config.LLMProviderOpenRouter:
		return c.chatOpenAICompatible(ctx, request)
	case config.LLMProviderAnthropic:
		return c.chatAnthropic(ctx, request)
	case config.LLMProviderAzureOpenAI:
		return c.chatAzureOpenAI(ctx, request)
	default:
		return ChatResponse{}, fmt.Errorf("unsupported llm provider: %s", c.options.Provider)
	}
}

// --- OpenAI function calling -------------------------------------------------

type openAIToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type openAITool struct {
	Type     string             `json:"type"`
	Function openAIToolFunction `json:"function"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name string `json:"name"`
		// Arguments is a JSON document carried as a string.
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIToolMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIToolChatRequest struct {
	Model               string              `json:"model"`
	Messages            []openAIToolMessage `json:"messages"`
	Tools               []openAITool        `json:"tools,omitempty"`
	Temperature         *float64            `json:"temperature,omitempty"`
	MaxTokens           int                 `json:"max_tokens,omitempty"`
	MaxCompletionTokens int                 `json:"max_completion_tokens,omitempty"`
}

type openAIToolChatResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content   json.RawMessage  `json:"content"`
			ToolCalls []openAIToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int64 `json:"prompt_tokens"`
		CompletionTokens    int64 `json:"completion_tokens"`
		TotalTokens         int64 `json:"total_tokens"`
		PromptTokensDetails struct {
			CachedTokens int64 `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

func (c *Client) chatOpenAICompatible(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	baseURL := c.options.BaseURL
	if baseURL == "" {
		baseURL = config.DefaultLLMProviderBaseURL(c.options.Provider)
	}
	return c.chatOpenAI(ctx, buildOpenAIChatCompletionsURL(baseURL), c.openAIHeaders(), c.options.Model, request)
}

func (c *Client) chatAzureOpenAI(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	model := c.options.Model
	if deployment := c.options.Extra["deployment"]; deployment != "" {
		model = deployment
	}
	return c.chatOpenAI(
		ctx,
		buildAzureOpenAIChatCompletionsURL(c.options.BaseURL, c.options.Extra["deployment"], c.options.Extra["api_version"]),
		map[string]string{"api-key": c.options.APIKey},
		model,
		request,
	)
}

func (c *Client) chatLMStudio(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	model, err := c.resolveLMStudioModel(ctx)
	if err != nil {
		return ChatResponse{}, err
	}
	if err := c.ensureLMStudioModelLoaded(ctx, model); err != nil {
		return ChatResponse{}, err
	}
	baseURL := c.options.BaseURL
	if baseURL == "" {
		baseURL = config.DefaultLLMProviderBaseURL(c.options.Provider)
	}
	return c.chatOpenAI(ctx, buildLMStudioOpenAIChatURL(baseURL), c.lmStudioHeaders(), model, request)
}

func (c *Client) openAIHeaders() map[string]string {
	headers := map[string]string{}
	if c.options.APIKey != "" {
		headers["Authorization"] = "Bearer " + c.options.APIKey
	}
	switch c.options.Provider {
	case config.LLMProviderOpenAI:
		if value := c.options.Extra["organization"]; value != "" {
			headers["OpenAI-Organization"] = value
		}
		if value := c.options.Extra["project"]; value != "" {
			headers["OpenAI-Project"] = value
		}
	case config.LLMProviderOpenRouter:
		if value := c.options.Extra["http_referer"]; value != "" {
			headers["HTTP-Referer"] = value
		}
		if value := c.options.Extra["x_title"]; value != "" {
			headers["X-Title"] = value
		}
	}
	return headers
}

func (c *Client) chatOpenAI(ctx context.Context, endpoint string, headers map[string]string, model string, request ChatRequest) (ChatResponse, error) {
	messages := make([]openAIToolMessage, 0, len(request.Messages)+1)
	if system := strings.TrimSpace(request.System); system != "" {
		messages = append(messages, openAIToolMessage{Role: "system", Content: system})
	}
	for _, message := range request.Messages {
		converted := openAIToolMessage{Role: message.Role, Content: message.Content}
		if message.Role == RoleTool {
			converted.ToolCallID = message.ToolCallID
		}
		for _, call := range message.ToolCalls {
			arguments, err := json.Marshal(call.Arguments)
			if err != nil {
				return ChatResponse{}, fmt.Errorf("failed to encode tool arguments for %s: %w", call.Name, err)
			}
			encoded := openAIToolCall{ID: call.ID, Type: "function"}
			encoded.Function.Name = call.Name
			encoded.Function.Arguments = string(arguments)
			converted.ToolCalls = append(converted.ToolCalls, encoded)
		}
		messages = append(messages, converted)
	}

	payload := openAIToolChatRequest{
		Model:       model,
		Messages:    messages,
		Temperature: c.options.Temperature,
	}
	for _, tool := range request.Tools {
		payload.Tools = append(payload.Tools, openAITool{
			Type: "function",
			Function: openAIToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  toolParameters(tool.InputSchema),
			},
		})
	}
	if config.LLMProviderUsesMaxCompletionTokens(c.options.Provider) {
		payload.MaxCompletionTokens = c.maxTokens()
	} else {
		payload.MaxTokens = c.maxTokens()
	}

	body, err := c.postJSON(ctx, endpoint, headers, payload)
	if err != nil {
		return ChatResponse{}, err
	}
	var response openAIToolChatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return ChatResponse{}, fmt.Errorf("failed to unmarshal %s response: %w", c.options.Provider, err)
	}
	if len(response.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("empty response from %s", c.options.Provider)
	}
	choice := response.Choices[0]

	// A tool call without prose is a complete turn, so empty content is only an
	// error when the model returned nothing at all.
	text, textErr := openAIMessageText(choice.Message.Content)
	calls := make([]ToolCall, 0, len(choice.Message.ToolCalls))
	for _, call := range choice.Message.ToolCalls {
		arguments, err := decodeToolArguments(call.Function.Arguments)
		if err != nil {
			return ChatResponse{}, fmt.Errorf("invalid tool arguments from %s for %s: %w", c.options.Provider, call.Function.Name, err)
		}
		calls = append(calls, ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: arguments})
	}
	if textErr != nil && len(calls) == 0 {
		return ChatResponse{}, fmt.Errorf("empty response from %s", c.options.Provider)
	}

	return ChatResponse{
		Text:      strings.TrimSpace(text),
		ToolCalls: calls,
		Stop:      choice.FinishReason,
		Usage: usageFromTokenDetails(
			c.options.Provider,
			model,
			c.options.Profile,
			chatUsagePrompt(request),
			text,
			response.Usage.PromptTokens,
			response.Usage.CompletionTokens,
			response.Usage.TotalTokens,
			response.Usage.PromptTokensDetails.CachedTokens,
		),
	}, nil
}

// --- Anthropic tool use ------------------------------------------------------

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`

	// tool_use
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`

	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

type anthropicToolMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

func (c *Client) chatAnthropic(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	baseURL := c.options.BaseURL
	if baseURL == "" {
		baseURL = config.DefaultLLMProviderBaseURL(c.options.Provider)
	}
	version := c.options.Extra["anthropic_version"]
	if version == "" {
		version = "2023-06-01"
	}

	messages := make([]anthropicToolMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		switch message.Role {
		case RoleTool:
			// Anthropic carries a tool result as a user turn.
			messages = append(messages, anthropicToolMessage{
				Role: RoleUser,
				Content: []anthropicContentBlock{{
					Type:      "tool_result",
					ToolUseID: message.ToolCallID,
					Content:   message.Content,
				}},
			})
		default:
			blocks := []anthropicContentBlock{}
			if strings.TrimSpace(message.Content) != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: message.Content})
			}
			for _, call := range message.ToolCalls {
				blocks = append(blocks, anthropicContentBlock{
					Type:  "tool_use",
					ID:    call.ID,
					Name:  call.Name,
					Input: call.Arguments,
				})
			}
			if len(blocks) == 0 {
				continue
			}
			messages = append(messages, anthropicToolMessage{Role: message.Role, Content: blocks})
		}
	}

	payload := struct {
		Model       string                    `json:"model"`
		MaxTokens   int                       `json:"max_tokens"`
		Temperature *float64                  `json:"temperature,omitempty"`
		System      string                    `json:"system,omitempty"`
		Messages    []anthropicToolMessage    `json:"messages"`
		Tools       []anthropicToolDefinition `json:"tools,omitempty"`
	}{
		Model:       c.options.Model,
		MaxTokens:   c.maxTokens(),
		Temperature: c.options.Temperature,
		System:      strings.TrimSpace(request.System),
		Messages:    messages,
	}
	for _, tool := range request.Tools {
		payload.Tools = append(payload.Tools, anthropicToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: toolParameters(tool.InputSchema),
		})
	}

	body, err := c.postJSON(ctx, buildAnthropicMessagesURL(baseURL), map[string]string{
		"x-api-key":         c.options.APIKey,
		"anthropic-version": version,
	}, payload)
	if err != nil {
		return ChatResponse{}, err
	}
	var response struct {
		StopReason string                  `json:"stop_reason"`
		Content    []anthropicContentBlock `json:"content"`
		Usage      anthropicUsage          `json:"usage"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return ChatResponse{}, fmt.Errorf("failed to unmarshal anthropic response: %w", err)
	}

	texts := []string{}
	calls := []ToolCall{}
	for _, block := range response.Content {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				texts = append(texts, block.Text)
			}
		case "tool_use":
			calls = append(calls, ToolCall{ID: block.ID, Name: block.Name, Arguments: block.Input})
		}
	}
	if len(texts) == 0 && len(calls) == 0 {
		return ChatResponse{}, fmt.Errorf("empty response from anthropic")
	}

	text := strings.Join(texts, "\n")
	usage := usageFromTokenDetails(
		c.options.Provider,
		c.options.Model,
		c.options.Profile,
		chatUsagePrompt(request),
		text,
		response.Usage.PromptTokens(),
		response.Usage.OutputTokens,
		response.Usage.TotalTokens(),
		response.Usage.CacheReadInputTokens,
	)
	usage.CacheWriteTokens = response.Usage.CacheCreationInputTokens
	return ChatResponse{
		Text:      strings.TrimSpace(text),
		ToolCalls: calls,
		Stop:      response.StopReason,
		Usage:     usage,
	}, nil
}

// --- Gemini function calling -------------------------------------------------

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type geminiFunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response,omitempty"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiToolDeclarations struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations"`
}

type geminiFunctionDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

func (c *Client) chatGemini(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	baseURL := strings.TrimRight(c.options.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultGeminiHost
	}
	endpoint := fmt.Sprintf(
		"%s/v1beta/models/%s:generateContent?key=%s",
		baseURL,
		url.PathEscape(c.options.Model),
		url.QueryEscape(c.options.APIKey),
	)

	contents := make([]geminiContent, 0, len(request.Messages))
	for _, message := range request.Messages {
		switch message.Role {
		case RoleTool:
			// Gemini answers a call by name rather than by id.
			contents = append(contents, geminiContent{
				Role: RoleUser,
				Parts: []geminiPart{{FunctionResponse: &geminiFunctionResponse{
					Name:     message.ToolName,
					Response: map[string]any{"result": message.Content},
				}}},
			})
		case RoleAssistant:
			parts := []geminiPart{}
			if strings.TrimSpace(message.Content) != "" {
				parts = append(parts, geminiPart{Text: message.Content})
			}
			for _, call := range message.ToolCalls {
				parts = append(parts, geminiPart{FunctionCall: &geminiFunctionCall{Name: call.Name, Args: call.Arguments}})
			}
			if len(parts) == 0 {
				continue
			}
			contents = append(contents, geminiContent{Role: "model", Parts: parts})
		default:
			contents = append(contents, geminiContent{Role: RoleUser, Parts: []geminiPart{{Text: message.Content}}})
		}
	}

	payload := struct {
		Contents          []geminiContent          `json:"contents"`
		SystemInstruction *geminiContent           `json:"systemInstruction,omitempty"`
		Tools             []geminiToolDeclarations `json:"tools,omitempty"`
		GenerationConfig  map[string]any           `json:"generationConfig,omitempty"`
	}{Contents: contents}
	if system := strings.TrimSpace(request.System); system != "" {
		payload.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: system}}}
	}
	if len(request.Tools) > 0 {
		declarations := make([]geminiFunctionDeclaration, 0, len(request.Tools))
		for _, tool := range request.Tools {
			declarations = append(declarations, geminiFunctionDeclaration{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  toolParameters(tool.InputSchema),
			})
		}
		payload.Tools = []geminiToolDeclarations{{FunctionDeclarations: declarations}}
	}
	generation := map[string]any{}
	if c.options.MaxTokens > 0 {
		generation["maxOutputTokens"] = c.options.MaxTokens
	}
	if c.options.Temperature != nil {
		generation["temperature"] = *c.options.Temperature
	}
	if len(generation) > 0 {
		payload.GenerationConfig = generation
	}

	body, err := c.postJSON(ctx, endpoint, nil, payload)
	if err != nil {
		return ChatResponse{}, err
	}
	var response struct {
		Candidates []struct {
			FinishReason string        `json:"finishReason"`
			Content      geminiContent `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount        int64 `json:"promptTokenCount"`
			CandidatesTokenCount    int64 `json:"candidatesTokenCount"`
			TotalTokenCount         int64 `json:"totalTokenCount"`
			CachedContentTokenCount int64 `json:"cachedContentTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return ChatResponse{}, fmt.Errorf("failed to unmarshal gemini response: %w", err)
	}
	if len(response.Candidates) == 0 {
		return ChatResponse{}, fmt.Errorf("empty response from gemini")
	}
	candidate := response.Candidates[0]

	texts := []string{}
	calls := []ToolCall{}
	for _, part := range candidate.Content.Parts {
		if strings.TrimSpace(part.Text) != "" {
			texts = append(texts, part.Text)
		}
		if part.FunctionCall != nil {
			calls = append(calls, ToolCall{Name: part.FunctionCall.Name, Arguments: part.FunctionCall.Args})
		}
	}
	if len(texts) == 0 && len(calls) == 0 {
		return ChatResponse{}, fmt.Errorf("empty response from gemini")
	}

	text := strings.Join(texts, "\n")
	return ChatResponse{
		Text:      strings.TrimSpace(text),
		ToolCalls: calls,
		Stop:      candidate.FinishReason,
		Usage: usageFromTokenDetails(
			c.options.Provider,
			c.options.Model,
			c.options.Profile,
			chatUsagePrompt(request),
			text,
			response.UsageMetadata.PromptTokenCount,
			response.UsageMetadata.CandidatesTokenCount,
			response.UsageMetadata.TotalTokenCount,
			response.UsageMetadata.CachedContentTokenCount,
		),
	}, nil
}

// --- shared helpers ----------------------------------------------------------

// toolParameters keeps a schema-shaped object on the wire. A provider rejects a
// function whose parameters are absent or not an object, and a tool that takes
// no arguments still has to say so.
func toolParameters(schema map[string]any) map[string]any {
	if len(schema) == 0 {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return schema
}

// decodeToolArguments reads the JSON document OpenAI-style providers send as a
// string. An empty string means a call with no arguments.
func decodeToolArguments(raw string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return map[string]any{}, nil
	}
	arguments := map[string]any{}
	if err := json.Unmarshal([]byte(trimmed), &arguments); err != nil {
		return nil, err
	}
	return arguments, nil
}

// chatUsagePrompt is the text used to estimate prompt tokens when a provider
// reports none. It is the conversation as the model saw it, not the last turn.
func chatUsagePrompt(request ChatRequest) string {
	parts := make([]string, 0, len(request.Messages)+1)
	if system := strings.TrimSpace(request.System); system != "" {
		parts = append(parts, system)
	}
	for _, message := range request.Messages {
		if strings.TrimSpace(message.Content) != "" {
			parts = append(parts, message.Content)
		}
		for _, call := range message.ToolCalls {
			if encoded, err := json.Marshal(call.Arguments); err == nil {
				parts = append(parts, call.Name+" "+string(encoded))
			}
		}
	}
	return strings.Join(parts, "\n")
}

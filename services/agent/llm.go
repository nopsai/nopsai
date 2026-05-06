package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"

	appconfig "nopsai/config"
	"nopsai/pkg/models"
	"nopsai/pkg/proto"

	"github.com/rs/zerolog/log"
)

type LLMClient struct {
	provider   string
	apiKey     string
	model      string
	baseURL    string
	reasoning  string
	httpClient *http.Client

	modelMu sync.Mutex
}

func NewLLMClient(provider, apiKey, model, baseURL, reasoning string) *LLMClient {
	return &LLMClient{
		provider:   appconfig.NormalizeLLMProvider(provider),
		apiKey:     strings.TrimSpace(apiKey),
		model:      strings.TrimSpace(model),
		baseURL:    strings.TrimSpace(baseURL),
		reasoning:  appconfig.NormalizeLMStudioReasoning(reasoning),
		httpClient: &http.Client{},
	}
}

func (c *LLMClient) GetAction(ctx context.Context, req *proto.GetActionRequest) (*proto.Action, error) {
	prompt := c.buildPrompt(req)

	var (
		actionModel *models.Action
		err         error
	)

	switch c.provider {
	case appconfig.LLMProviderGemini:
		actionModel, err = c.callGeminiForAction(ctx, prompt)
	case appconfig.LLMProviderLMStudio:
		actionModel, err = c.callLMStudioForAction(ctx, prompt)
	default:
		err = fmt.Errorf("unsupported llm provider: %s", c.provider)
	}
	if err != nil {
		log.Error().Err(err).Str("provider", c.provider).Msg("Error calling LLM provider for GetAction")
		return nil, err
	}

	protoAction := &proto.Action{Type: string(actionModel.Type)}
	switch actionModel.Type {
	case models.ActionTypeExecuteCommand:
		protoAction.Payload = &proto.Action_CommandAction{CommandAction: &proto.CommandAction{Command: actionModel.CommandAction.Command}}
	case models.ActionTypeReplaceFile:
		protoAction.Payload = &proto.Action_FileAction{FileAction: &proto.FileAction{Path: actionModel.FileAction.Path, Content: actionModel.FileAction.Content}}
	case models.ActionTypeReturnAnswer:
		protoAction.Payload = &proto.Action_AnswerAction{AnswerAction: &proto.AnswerAction{Answer: actionModel.AnswerAction.Answer}}
	}

	return protoAction, nil
}

func (c *LLMClient) EvaluateCondition(ctx context.Context, req *proto.ConditionRequest) (*proto.ConditionResponse, error) {
	prompt := c.buildConditionPrompt(req)

	var (
		result bool
		err    error
	)

	switch c.provider {
	case appconfig.LLMProviderGemini:
		result, err = c.callGeminiForBoolean(ctx, prompt)
	case appconfig.LLMProviderLMStudio:
		result, err = c.callLMStudioForBoolean(ctx, prompt)
	default:
		err = fmt.Errorf("unsupported llm provider: %s", c.provider)
	}
	if err != nil {
		log.Error().Err(err).Str("provider", c.provider).Msg("Error calling LLM provider for EvaluateCondition")
		return &proto.ConditionResponse{Result: false}, err
	}

	return &proto.ConditionResponse{Result: result}, nil
}

func (c *LLMClient) buildConditionPrompt(req *proto.ConditionRequest) string {
	history := req.GetHistory()
	if history == "" {
		history = "No history yet."
	}

	promptTemplate := `You are a CI/CD automation bot. Your task is to answer a YES/NO question based on the provided context.
You must only respond with the word "true" or "false" and nothing else.

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

	fullPrompt := fmt.Sprintf(promptTemplate, buildVariablesSection(req.GetVariables()), history, req.GetGoal())
	log.Debug().Str("provider", c.provider).Msgf("Condition prompt:\n%s", fullPrompt)
	return fullPrompt
}

func (c *LLMClient) buildPrompt(req *proto.GetActionRequest) string {
	history := req.GetHistory()
	if history == "" {
		history = "No history yet."
	}

	promptTemplate := `You are an expert CI/CD automation bot. Your task is to achieve a user's goal by choosing the correct action from a toolkit.
You must only respond with a single JSON object. Inside this object, there should be a single key "action" which contains the action to perform.

Here are the available actions:
1. **EXECUTE_COMMAND**: {"action": {"type": "EXECUTE_COMMAND", "command_action": {"command": "your-bash-command-here"}}}
2. **REPLACE_FILE**: {"action": {"type": "REPLACE_FILE", "file_action": {"path": "./path/to/file.txt", "content": "The full new content of the file."}}}
3. **RETURN_ANSWER**: {"action": {"type": "RETURN_ANSWER", "answer_action": {"answer": "The answer to the user's question."}}}
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
		buildVariablesSection(req.GetVariables()),
		buildDirectoryListingSection(req.GetDirectoryListing()),
		history,
		req.GetGoal(),
	)
	log.Debug().Str("provider", c.provider).Msgf("Full prompt:\n%s", fullPrompt)
	return fullPrompt
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

func (c *LLMClient) callGeminiForBoolean(ctx context.Context, prompt string) (bool, error) {
	log.Debug().Msg("Calling Gemini API for boolean decision")
	geminiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", c.model, c.apiKey)

	reqPayload := models.GeminiRequest{
		Contents: []models.Content{
			{Parts: []models.Part{{Text: prompt}}},
		},
	}

	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return false, fmt.Errorf("failed to marshal gemini request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, geminiURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return false, fmt.Errorf("failed to build gemini request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return false, fmt.Errorf("failed to call gemini api: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("gemini api returned non-200 status: %s, body: %s", resp.Status, string(body))
	}

	var geminiResp models.GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return false, fmt.Errorf("failed to unmarshal gemini response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return false, fmt.Errorf("invalid or empty response from gemini: %s", string(body))
	}

	return parseBooleanText(geminiResp.Candidates[0].Content.Parts[0].Text)
}

func (c *LLMClient) callGeminiForAction(ctx context.Context, prompt string) (*models.Action, error) {
	log.Debug().Msg("Calling Gemini API for action selection")
	geminiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", c.model, c.apiKey)

	reqPayload := models.GeminiRequest{
		Contents: []models.Content{
			{Parts: []models.Part{{Text: prompt}}},
		},
	}

	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal gemini request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, geminiURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to build gemini request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call gemini api: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini api returned non-200 status: %s, body: %s", resp.Status, string(body))
	}

	var geminiResp models.GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal gemini response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("invalid or empty response from gemini: %s", string(body))
	}

	return decodeActionResponse(geminiResp.Candidates[0].Content.Parts[0].Text)
}

func (c *LLMClient) callLMStudioForBoolean(ctx context.Context, prompt string) (bool, error) {
	responseText, err := c.callLMStudio(ctx, prompt)
	if err != nil {
		return false, err
	}
	return parseBooleanText(responseText)
}

func (c *LLMClient) callLMStudioForAction(ctx context.Context, prompt string) (*models.Action, error) {
	responseText, err := c.callLMStudio(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return decodeActionResponse(responseText)
}

func (c *LLMClient) callLMStudio(ctx context.Context, prompt string) (string, error) {
	model, err := c.resolveLMStudioModel(ctx)
	if err != nil {
		return "", err
	}

	reqPayload := struct {
		Model     string `json:"model"`
		Input     string `json:"input"`
		Reasoning string `json:"reasoning,omitempty"`
		Store     bool   `json:"store"`
	}{
		Model:     model,
		Input:     prompt,
		Reasoning: c.reasoning,
		Store:     false,
	}

	logEvent := log.Debug().Str("model", model).Str("endpoint", buildLMStudioChatURL(c.baseURL))
	if c.reasoning != "" {
		logEvent = logEvent.Str("reasoning", c.reasoning)
	}
	logEvent.Msg("Calling LM Studio native chat API")

	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal lm studio request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, buildLMStudioChatURL(c.baseURL), bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to build lm studio request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to call lm studio api: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lm studio api returned non-200 status: %s, body: %s", resp.Status, string(body))
	}

	var lmStudioResp struct {
		Output []struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &lmStudioResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal lm studio response: %w", err)
	}
	if len(lmStudioResp.Output) == 0 {
		return "", fmt.Errorf("invalid or empty response from lm studio: %s", string(body))
	}

	var messages []string
	for _, item := range lmStudioResp.Output {
		if item.Type == "message" && strings.TrimSpace(item.Content) != "" {
			messages = append(messages, item.Content)
		}
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("lm studio response did not contain a message item: %s", string(body))
	}

	return strings.Join(messages, "\n"), nil
}

func (c *LLMClient) resolveLMStudioModel(ctx context.Context) (string, error) {
	c.modelMu.Lock()
	configuredModel := strings.TrimSpace(c.model)
	c.modelMu.Unlock()
	if configuredModel != "" {
		return configuredModel, nil
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, buildLMStudioModelsURL(c.baseURL), nil)
	if err != nil {
		return "", fmt.Errorf("failed to build lm studio model discovery request: %w", err)
	}
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to discover lm studio models: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lm studio model discovery returned non-200 status: %s, body: %s", resp.Status, string(body))
	}

	var modelsResp struct {
		Models []struct {
			Type string `json:"type"`
			Key  string `json:"key"`
		} `json:"models"`
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &modelsResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal lm studio models response: %w", err)
	}

	discoveredModel := ""
	for _, candidate := range modelsResp.Models {
		if candidate.Type == "" || candidate.Type == "llm" {
			if strings.TrimSpace(candidate.Key) != "" {
				discoveredModel = strings.TrimSpace(candidate.Key)
				break
			}
		}
	}
	if discoveredModel == "" {
		for _, candidate := range modelsResp.Data {
			if strings.TrimSpace(candidate.ID) != "" {
				discoveredModel = strings.TrimSpace(candidate.ID)
				break
			}
		}
	}
	if discoveredModel == "" {
		return "", fmt.Errorf("lm studio did not return any usable models")
	}

	c.modelMu.Lock()
	if strings.TrimSpace(c.model) == "" {
		c.model = discoveredModel
	} else {
		discoveredModel = strings.TrimSpace(c.model)
	}
	c.modelMu.Unlock()

	return discoveredModel, nil
}

func buildLMStudioChatURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	lower := strings.ToLower(trimmed)

	switch {
	case strings.HasSuffix(lower, "/api/v1/chat"):
		return trimmed
	case strings.HasSuffix(lower, "/api/v1"):
		return trimmed + "/chat"
	default:
		return trimmed + "/api/v1/chat"
	}
}

func buildLMStudioModelsURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	lower := strings.ToLower(trimmed)

	switch {
	case strings.HasSuffix(lower, "/api/v1/models"):
		return trimmed
	case strings.HasSuffix(lower, "/api/v1/chat"):
		return trimmed[:len(trimmed)-len("/chat")] + "/models"
	case strings.HasSuffix(lower, "/api/v1"):
		return trimmed + "/models"
	case strings.HasSuffix(lower, "/v1/models"):
		return trimmed
	case strings.HasSuffix(lower, "/v1/chat/completions"):
		return trimmed[:len(trimmed)-len("/chat/completions")] + "/models"
	case strings.HasSuffix(lower, "/v1"):
		return trimmed + "/models"
	default:
		return trimmed + "/api/v1/models"
	}
}

func cleanModelTextResponse(raw string) string {
	cleaned := strings.TrimSpace(raw)

	if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
		if len(cleaned) >= 4 && strings.EqualFold(cleaned[:4], "json") {
			cleaned = strings.TrimSpace(cleaned[4:])
		}
		if idx := strings.LastIndex(cleaned, "```"); idx >= 0 {
			cleaned = cleaned[:idx]
		}
	}

	cleaned = strings.TrimSpace(cleaned)
	if len(cleaned) >= 4 && strings.EqualFold(cleaned[:4], "json") {
		cleaned = strings.TrimSpace(cleaned[4:])
	}

	return strings.Trim(cleaned, "` \n\r\t")
}

func decodeActionResponse(raw string) (*models.Action, error) {
	actionJSON := cleanModelTextResponse(raw)

	var actionWrapper struct {
		Action models.Action `json:"action"`
	}
	if err := json.Unmarshal([]byte(actionJSON), &actionWrapper); err != nil {
		return nil, fmt.Errorf("failed to unmarshal action response: %w. Response text: %s", err, actionJSON)
	}

	return &actionWrapper.Action, nil
}

func parseBooleanText(raw string) (bool, error) {
	responseText := strings.ToLower(strings.TrimSpace(cleanModelTextResponse(raw)))
	responseText = strings.Trim(responseText, "\"'` \n\r\t")

	switch {
	case strings.HasPrefix(responseText, "true"):
		return true, nil
	case strings.HasPrefix(responseText, "false"):
		return false, nil
	default:
		return false, fmt.Errorf("unexpected boolean response: %s", raw)
	}
}

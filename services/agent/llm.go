package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"

	"github.com/rs/zerolog/log"
)

type LLMClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewLLMClient(apiKey, model string) *LLMClient {
	return &LLMClient{
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{},
	}
}

func (c *LLMClient) GetAction(ctx context.Context, req *proto.GetActionRequest) (*proto.Action, error) {
	prompt := c.buildPrompt(req)

	actionModel, err := c.callRealGemini(ctx, prompt)
	if err != nil {
		log.Error().Err(err).Msg("Error calling Gemini for GetAction")
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

	result, err := c.callGeminiForBoolean(ctx, prompt)
	if err != nil {
		log.Error().Err(err).Msg("Error calling Gemini for EvaluateCondition")
		return &proto.ConditionResponse{Result: false}, err
	}

	return &proto.ConditionResponse{Result: result}, nil
}

func (c *LLMClient) buildConditionPrompt(req *proto.ConditionRequest) string {
	var envBuilder strings.Builder
	envBuilder.WriteString("**Environment Variables:**\n")
	for key, value := range req.GetEnvironment() {
		envBuilder.WriteString(fmt.Sprintf("- %s: %s\n", key, value))
	}

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

	fullPrompt := fmt.Sprintf(promptTemplate, envBuilder.String(), history, req.GetGoal())
	log.Debug().Msgf("Condition prompt:\n%s", fullPrompt)
	return fullPrompt
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

	responseText := strings.ToLower(strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text))

	return responseText == "true", nil
}

func (c *LLMClient) buildPrompt(req *proto.GetActionRequest) string {
	var envBuilder strings.Builder
	envBuilder.WriteString("**Environment Variables:**\n")
	for key, value := range req.GetEnvironment() {
		envBuilder.WriteString(fmt.Sprintf("- %s: %s\n", key, value))
	}

	var directoryListingBuilder strings.Builder
	directoryListingBuilder.WriteString("**Working Directory Contents:**\n")
	if len(req.GetDirectoryListing()) == 0 {
		directoryListingBuilder.WriteString("Directory is empty.\n")
	} else {
		for name, content := range req.GetDirectoryListing() {
			directoryListingBuilder.WriteString(fmt.Sprintf("--- File: %s ---\n%s\n", name, content))
		}
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

	history := req.GetHistory()
	if history == "" {
		history = "No history yet."
	}

	fullPrompt := fmt.Sprintf(promptTemplate, envBuilder.String(), directoryListingBuilder.String(), history, req.GetGoal())
	log.Debug().Msgf("Full prompt:\n%s", fullPrompt)
	return fullPrompt
}

func (c *LLMClient) callRealGemini(ctx context.Context, prompt string) (*models.Action, error) {
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

	actionJSON := geminiResp.Candidates[0].Content.Parts[0].Text
	actionJSON = strings.Trim(actionJSON, " \n\r\t`")
	actionJSON = strings.TrimPrefix(actionJSON, "json")

	var actionWrapper struct {
		Action models.Action `json:"action"`
	}
	if err := json.Unmarshal([]byte(actionJSON), &actionWrapper); err != nil {
		return nil, fmt.Errorf("failed to unmarshal action from gemini response: %w. Response text: %s", err, actionJSON)
	}

	return &actionWrapper.Action, nil
}

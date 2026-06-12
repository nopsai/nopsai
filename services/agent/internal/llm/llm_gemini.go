package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"nopsai/pkg/models"

	"github.com/rs/zerolog/log"
)

func (c *LLMClient) callGeminiForBoolean(ctx context.Context, prompt string) (bool, error) {
	logEvent := log.Debug().Str("model", c.model)
	if c.profile != "" {
		logEvent = logEvent.Str("llm_profile", c.profile)
	}
	logEvent.Msg("Calling Gemini API for boolean decision")
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
	responseText := geminiResp.Candidates[0].Content.Parts[0].Text
	c.recordGeminiUsage(ctx, geminiResp, prompt, responseText)

	return parseBooleanText(responseText)
}

func (c *LLMClient) callGeminiForAction(ctx context.Context, prompt string) (*models.Action, error) {
	logEvent := log.Debug().Str("model", c.model)
	if c.profile != "" {
		logEvent = logEvent.Str("llm_profile", c.profile)
	}
	logEvent.Msg("Calling Gemini API for action selection")
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
	responseText := geminiResp.Candidates[0].Content.Parts[0].Text
	c.recordGeminiUsage(ctx, geminiResp, prompt, responseText)

	return decodeActionResponse(responseText)
}

func (c *LLMClient) recordGeminiUsage(ctx context.Context, resp models.GeminiResponse, prompt, completion string) {
	recordUsage(ctx, usageFromTokens(
		c.provider,
		c.model,
		c.profile,
		prompt,
		completion,
		resp.UsageMetadata.PromptTokenCount,
		resp.UsageMetadata.CandidatesTokenCount,
		resp.UsageMetadata.TotalTokenCount,
	))
}

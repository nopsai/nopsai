package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"nopsai/pkg/models"

	"github.com/rs/zerolog/log"
)

type geminiClient struct {
	owner       *LLMClient
	apiKey      string
	model       string
	maxTokens   int
	temperature *float64
}

func newGeminiClient(owner *LLMClient, options LLMClientOptions) ProviderClient {
	return &geminiClient{
		owner:       owner,
		apiKey:      options.APIKey,
		model:       options.Model,
		maxTokens:   options.MaxTokens,
		temperature: options.Temperature,
	}
}

func (c *geminiClient) Name() string {
	return c.owner.provider
}

func (c *geminiClient) Complete(ctx context.Context, prompt string) (string, error) {
	logEvent := log.Debug().Str("model", c.model)
	if c.owner.profile != "" {
		logEvent = logEvent.Str("llm_profile", c.owner.profile)
	}
	logEvent.Msg("Calling Gemini API")

	endpoint := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		url.PathEscape(c.model),
		url.QueryEscape(c.apiKey),
	)
	reqPayload := models.GeminiRequest{
		Contents: []models.Content{{Parts: []models.Part{{Text: prompt}}}},
	}
	if c.maxTokens > 0 || c.temperature != nil {
		reqPayload.GenerationConfig = &models.GeminiGenerationConfig{
			MaxOutputTokens: c.maxTokens,
			Temperature:     c.temperature,
		}
	}
	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal gemini request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to build gemini request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.owner.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to call gemini api: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini api returned non-200 status: %s, body: %s", resp.Status, string(body))
	}

	var geminiResp models.GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal gemini response: %w", err)
	}
	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("invalid or empty response from gemini: %s", string(body))
	}

	responseText := geminiResp.Candidates[0].Content.Parts[0].Text
	recordUsage(ctx, usageFromTokens(
		c.owner.provider,
		c.model,
		c.owner.profile,
		prompt,
		responseText,
		geminiResp.UsageMetadata.PromptTokenCount,
		geminiResp.UsageMetadata.CandidatesTokenCount,
		geminiResp.UsageMetadata.TotalTokenCount,
	))
	return responseText, nil
}

func (c *LLMClient) callGeminiForBoolean(ctx context.Context, prompt string) (bool, error) {
	responseText, err := c.providerClient.Complete(ctx, prompt)
	if err != nil {
		return false, err
	}
	return parseBooleanText(responseText)
}

func (c *LLMClient) callGeminiForAction(ctx context.Context, prompt string) (*models.Action, error) {
	responseText, err := c.providerClient.Complete(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return decodeActionResponse(responseText)
}

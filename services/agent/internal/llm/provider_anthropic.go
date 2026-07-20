package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	appconfig "nopsai/config"

	"github.com/rs/zerolog/log"
)

type anthropicClient struct {
	owner       *LLMClient
	apiKey      string
	model       string
	baseURL     string
	version     string
	maxTokens   int
	temperature *float64
}

func newAnthropicClient(owner *LLMClient, options LLMClientOptions) ProviderClient {
	baseURL := options.BaseURL
	if baseURL == "" {
		baseURL = appconfig.DefaultLLMProviderBaseURL(options.Provider)
	}
	version := options.Extra["anthropic_version"]
	if version == "" {
		version = "2023-06-01"
	}
	maxTokens := options.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultLLMMaxTokens
	}
	return &anthropicClient{
		owner:       owner,
		apiKey:      options.APIKey,
		model:       options.Model,
		baseURL:     baseURL,
		version:     version,
		maxTokens:   maxTokens,
		temperature: options.Temperature,
	}
}

func (c *anthropicClient) Name() string {
	return c.owner.provider
}

func (c *anthropicClient) Complete(ctx context.Context, prompt string) (string, error) {
	payload := struct {
		Model       string              `json:"model"`
		MaxTokens   int                 `json:"max_tokens"`
		Temperature *float64            `json:"temperature,omitempty"`
		Messages    []openAIChatMessage `json:"messages"`
	}{
		Model:       c.model,
		MaxTokens:   c.maxTokens,
		Temperature: c.temperature,
		Messages:    []openAIChatMessage{{Role: "user", Content: prompt}},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	endpoint := buildAnthropicMessagesURL(c.baseURL)
	logEvent := log.Debug().Str("model", c.model).Str("endpoint", endpoint)
	if c.owner.profile != "" {
		logEvent = logEvent.Str("llm_profile", c.owner.profile)
	}
	logEvent.Msg("Calling Anthropic Messages API")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to build anthropic request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", c.version)

	resp, err := c.owner.httpClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("anthropic request cancelled: %w", ctxErr)
		}
		return "", fmt.Errorf("failed to call anthropic api: %w", err)
	}
	defer resp.Body.Close()

	body, err := readLLMResponseBody(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read anthropic response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("anthropic api returned non-2xx status: %s, body: %s", resp.Status, string(body))
	}

	var response struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal anthropic response: %w", err)
	}
	var messages []string
	for _, part := range response.Content {
		if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
			messages = append(messages, part.Text)
		}
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("invalid or empty response from anthropic: %s", string(body))
	}
	responseText := strings.Join(messages, "\n")
	recordUsage(ctx, usageFromTokenDetailsForClient(
		c.owner,
		c.model,
		prompt,
		responseText,
		response.Usage.InputTokens,
		response.Usage.OutputTokens,
		response.Usage.InputTokens+response.Usage.OutputTokens,
		0,
	))
	return responseText, nil
}

func buildAnthropicMessagesURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(strings.ToLower(trimmed), "/v1/messages") {
		return trimmed
	}
	if strings.HasSuffix(strings.ToLower(trimmed), "/v1") {
		return trimmed + "/messages"
	}
	return trimmed + "/v1/messages"
}

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

const defaultLLMMaxTokens = 2048

type openAICompatibleClient struct {
	owner       *LLMClient
	apiKey      string
	model       string
	baseURL     string
	maxTokens   int
	temperature *float64
	extra       map[string]string
}

type openAIChatRequest struct {
	Model               string              `json:"model"`
	Messages            []openAIChatMessage `json:"messages"`
	Temperature         *float64            `json:"temperature,omitempty"`
	MaxTokens           int                 `json:"max_tokens,omitempty"`
	MaxCompletionTokens int                 `json:"max_completion_tokens,omitempty"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content json.RawMessage `json:"content"`
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

func newOpenAICompatibleClient(owner *LLMClient, options LLMClientOptions) ProviderClient {
	baseURL := options.BaseURL
	if baseURL == "" {
		baseURL = appconfig.DefaultLLMProviderBaseURL(options.Provider)
	}
	maxTokens := options.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultLLMMaxTokens
	}
	return &openAICompatibleClient{
		owner:       owner,
		apiKey:      options.APIKey,
		model:       options.Model,
		baseURL:     baseURL,
		maxTokens:   maxTokens,
		temperature: options.Temperature,
		extra:       options.Extra,
	}
}

func (c *openAICompatibleClient) Name() string {
	return c.owner.provider
}

func (c *openAICompatibleClient) Complete(ctx context.Context, prompt string) (string, error) {
	headers := map[string]string{}
	if c.apiKey != "" {
		headers["Authorization"] = "Bearer " + c.apiKey
	}
	switch c.owner.provider {
	case appconfig.LLMProviderOpenAI:
		if value := c.extra["organization"]; value != "" {
			headers["OpenAI-Organization"] = value
		}
		if value := c.extra["project"]; value != "" {
			headers["OpenAI-Project"] = value
		}
	case appconfig.LLMProviderOpenRouter:
		if value := c.extra["http_referer"]; value != "" {
			headers["HTTP-Referer"] = value
		}
		if value := c.extra["x_title"]; value != "" {
			headers["X-Title"] = value
		}
	}

	return completeOpenAIChat(
		ctx,
		c.owner,
		buildOpenAIChatCompletionsURL(c.baseURL),
		headers,
		c.model,
		prompt,
		c.maxTokens,
		c.temperature,
	)
}

func completeOpenAIChat(
	ctx context.Context,
	owner *LLMClient,
	endpoint string,
	headers map[string]string,
	model string,
	prompt string,
	maxTokens int,
	temperature *float64,
) (string, error) {
	payload := openAIChatRequest{
		Model:       model,
		Messages:    []openAIChatMessage{{Role: "user", Content: prompt}},
		Temperature: temperature,
	}
	if appconfig.LLMProviderUsesMaxCompletionTokens(owner.provider) {
		payload.MaxCompletionTokens = maxTokens
	} else {
		payload.MaxTokens = maxTokens
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal %s request: %w", owner.provider, err)
	}

	logEvent := log.Debug().
		Str("provider", owner.provider).
		Str("model", model).
		Str("endpoint", endpoint)
	if owner.profile != "" {
		logEvent = logEvent.Str("llm_profile", owner.profile)
	}
	logEvent.Msg("Calling OpenAI-compatible chat completions API")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to build %s request: %w", owner.provider, err)
	}
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		if strings.TrimSpace(value) != "" {
			req.Header.Set(name, value)
		}
	}

	resp, err := owner.httpClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("%s request cancelled: %w", owner.provider, ctxErr)
		}
		return "", fmt.Errorf("failed to call %s api: %w", owner.provider, err)
	}
	defer resp.Body.Close()

	body, err := readLLMResponseBody(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read %s response: %w", owner.provider, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("%s api returned non-2xx status: %s, body: %s", owner.provider, resp.Status, string(body))
	}

	var response openAIChatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal %s response: %w", owner.provider, err)
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("invalid or empty response from %s: %s", owner.provider, string(body))
	}
	responseText, err := openAIMessageText(response.Choices[0].Message.Content)
	if err != nil {
		return "", fmt.Errorf("invalid response from %s: %w", owner.provider, err)
	}

	recordUsage(ctx, usageFromTokenDetailsForClient(
		owner,
		model,
		prompt,
		responseText,
		response.Usage.PromptTokens,
		response.Usage.CompletionTokens,
		response.Usage.TotalTokens,
		response.Usage.PromptTokensDetails.CachedTokens,
	))
	return responseText, nil
}

func openAIMessageText(raw json.RawMessage) (string, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if strings.TrimSpace(text) == "" {
			return "", fmt.Errorf("message content is empty")
		}
		return text, nil
	}

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", fmt.Errorf("message content is neither text nor text parts")
	}
	var messages []string
	for _, part := range parts {
		if (part.Type == "" || part.Type == "text") && strings.TrimSpace(part.Text) != "" {
			messages = append(messages, part.Text)
		}
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("message content has no text parts")
	}
	return strings.Join(messages, "\n"), nil
}

func buildOpenAIChatCompletionsURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasSuffix(lower, "/chat/completions"):
		return trimmed
	case strings.HasSuffix(lower, "/v1"):
		return trimmed + "/chat/completions"
	default:
		return trimmed + "/chat/completions"
	}
}

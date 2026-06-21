package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/models"
)

const (
	defaultMaxTokens  = 2048
	maxResponseBytes  = 2 << 20
	defaultGeminiHost = "https://generativelanguage.googleapis.com"
)

type Options struct {
	Provider       string
	Profile        string
	APIKey         string
	Model          string
	BaseURL        string
	Reasoning      string
	TimeoutSeconds int
	MaxTokens      int
	Temperature    *float64
	Extra          map[string]string
	HTTPClient     *http.Client
}

type Usage struct {
	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
	Profile          string `json:"profile,omitempty"`
	PromptTokens     int64  `json:"prompt_tokens,omitempty"`
	CompletionTokens int64  `json:"completion_tokens,omitempty"`
	TotalTokens      int64  `json:"total_tokens,omitempty"`
	Estimated        bool   `json:"estimated,omitempty"`
}

type Completion struct {
	Text  string `json:"text"`
	Usage Usage  `json:"usage,omitempty"`
}

type Client struct {
	options    Options
	httpClient *http.Client
}

func New(options Options) *Client {
	options.Provider = config.NormalizeLLMProvider(options.Provider)
	options.Profile = strings.TrimSpace(options.Profile)
	options.APIKey = strings.TrimSpace(options.APIKey)
	options.Model = strings.TrimSpace(options.Model)
	options.BaseURL = strings.TrimSpace(options.BaseURL)
	options.Reasoning = config.NormalizeLMStudioReasoning(options.Reasoning)
	options.Extra = normalizeExtra(options.Extra)
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if options.TimeoutSeconds > 0 {
		copied := *httpClient
		copied.Timeout = time.Duration(options.TimeoutSeconds) * time.Second
		httpClient = &copied
	}
	return &Client{options: options, httpClient: httpClient}
}

func (c *Client) Complete(ctx context.Context, prompt string) (Completion, error) {
	return c.CompleteWithSystem(ctx, "", prompt)
}

func (c *Client) CompleteWithSystem(ctx context.Context, systemInstruction, prompt string) (Completion, error) {
	systemInstruction = strings.TrimSpace(systemInstruction)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return Completion{}, fmt.Errorf("prompt is required")
	}
	switch c.options.Provider {
	case config.LLMProviderGemini:
		return c.completeGemini(ctx, systemInstruction, prompt)
	case config.LLMProviderLMStudio:
		return c.completeLMStudio(ctx, systemInstruction, prompt)
	case config.LLMProviderOpenAI,
		config.LLMProviderGroq,
		config.LLMProviderMistral,
		config.LLMProviderOllama,
		config.LLMProviderOpenRouter:
		return c.completeOpenAICompatible(ctx, systemInstruction, prompt)
	case config.LLMProviderAnthropic:
		return c.completeAnthropic(ctx, systemInstruction, prompt)
	case config.LLMProviderAzureOpenAI:
		return c.completeAzureOpenAI(ctx, systemInstruction, prompt)
	default:
		return Completion{}, fmt.Errorf("unsupported llm provider: %s", c.options.Provider)
	}
}

func (c *Client) completeOpenAICompatible(ctx context.Context, systemInstruction, prompt string) (Completion, error) {
	baseURL := c.options.BaseURL
	if baseURL == "" {
		baseURL = config.DefaultLLMProviderBaseURL(c.options.Provider)
	}
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
	return c.completeOpenAIChat(ctx, buildOpenAIChatCompletionsURL(baseURL), headers, c.options.Model, systemInstruction, prompt)
}

func (c *Client) completeAzureOpenAI(ctx context.Context, systemInstruction, prompt string) (Completion, error) {
	model := c.options.Model
	if deployment := c.options.Extra["deployment"]; deployment != "" {
		model = deployment
	}
	return c.completeOpenAIChat(
		ctx,
		buildAzureOpenAIChatCompletionsURL(c.options.BaseURL, c.options.Extra["deployment"], c.options.Extra["api_version"]),
		map[string]string{"api-key": c.options.APIKey},
		model,
		systemInstruction,
		prompt,
	)
}

func (c *Client) completeOpenAIChat(ctx context.Context, endpoint string, headers map[string]string, model, systemInstruction, prompt string) (Completion, error) {
	messages := make([]openAIChatMessage, 0, 2)
	if systemInstruction != "" {
		messages = append(messages, openAIChatMessage{Role: "system", Content: systemInstruction})
	}
	messages = append(messages, openAIChatMessage{Role: "user", Content: prompt})
	payload := openAIChatRequest{
		Model:       model,
		Messages:    messages,
		Temperature: c.options.Temperature,
	}
	maxTokens := c.maxTokens()
	if config.LLMProviderUsesMaxCompletionTokens(c.options.Provider) {
		payload.MaxCompletionTokens = maxTokens
	} else {
		payload.MaxTokens = maxTokens
	}
	body, err := c.postJSON(ctx, endpoint, headers, payload)
	if err != nil {
		return Completion{}, err
	}
	var response openAIChatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return Completion{}, fmt.Errorf("failed to unmarshal %s response: %w", c.options.Provider, err)
	}
	if len(response.Choices) == 0 {
		return Completion{}, fmt.Errorf("empty response from %s", c.options.Provider)
	}
	text, err := openAIMessageText(response.Choices[0].Message.Content)
	if err != nil {
		return Completion{}, fmt.Errorf("invalid response from %s: %w", c.options.Provider, err)
	}
	return Completion{
		Text: strings.TrimSpace(text),
		Usage: usageFromTokens(
			c.options.Provider,
			model,
			c.options.Profile,
			completionPrompt(systemInstruction, prompt),
			text,
			response.Usage.PromptTokens,
			response.Usage.CompletionTokens,
			response.Usage.TotalTokens,
		),
	}, nil
}

func (c *Client) completeAnthropic(ctx context.Context, systemInstruction, prompt string) (Completion, error) {
	baseURL := c.options.BaseURL
	if baseURL == "" {
		baseURL = config.DefaultLLMProviderBaseURL(c.options.Provider)
	}
	version := c.options.Extra["anthropic_version"]
	if version == "" {
		version = "2023-06-01"
	}
	payload := struct {
		Model       string              `json:"model"`
		MaxTokens   int                 `json:"max_tokens"`
		Temperature *float64            `json:"temperature,omitempty"`
		System      string              `json:"system,omitempty"`
		Messages    []openAIChatMessage `json:"messages"`
	}{
		Model:       c.options.Model,
		MaxTokens:   c.maxTokens(),
		Temperature: c.options.Temperature,
		System:      systemInstruction,
		Messages:    []openAIChatMessage{{Role: "user", Content: prompt}},
	}
	body, err := c.postJSON(ctx, buildAnthropicMessagesURL(baseURL), map[string]string{
		"x-api-key":         c.options.APIKey,
		"anthropic-version": version,
	}, payload)
	if err != nil {
		return Completion{}, err
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
		return Completion{}, fmt.Errorf("failed to unmarshal anthropic response: %w", err)
	}
	messages := []string{}
	for _, part := range response.Content {
		if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
			messages = append(messages, part.Text)
		}
	}
	if len(messages) == 0 {
		return Completion{}, fmt.Errorf("empty response from anthropic")
	}
	text := strings.Join(messages, "\n")
	return Completion{
		Text: strings.TrimSpace(text),
		Usage: usageFromTokens(
			c.options.Provider,
			c.options.Model,
			c.options.Profile,
			completionPrompt(systemInstruction, prompt),
			text,
			response.Usage.InputTokens,
			response.Usage.OutputTokens,
			response.Usage.InputTokens+response.Usage.OutputTokens,
		),
	}, nil
}

func (c *Client) completeGemini(ctx context.Context, systemInstruction, prompt string) (Completion, error) {
	baseURL := strings.TrimRight(c.options.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultGeminiHost
	}
	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", baseURL, url.PathEscape(c.options.Model), url.QueryEscape(c.options.APIKey))
	payload := models.GeminiRequest{Contents: []models.Content{{Parts: []models.Part{{Text: prompt}}}}}
	if systemInstruction != "" {
		payload.SystemInstruction = &models.Content{Parts: []models.Part{{Text: systemInstruction}}}
	}
	if c.options.MaxTokens > 0 || c.options.Temperature != nil {
		payload.GenerationConfig = &models.GeminiGenerationConfig{
			MaxOutputTokens: c.options.MaxTokens,
			Temperature:     c.options.Temperature,
		}
	}
	body, err := c.postJSON(ctx, endpoint, nil, payload)
	if err != nil {
		return Completion{}, err
	}
	var response models.GeminiResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return Completion{}, fmt.Errorf("failed to unmarshal gemini response: %w", err)
	}
	if len(response.Candidates) == 0 || len(response.Candidates[0].Content.Parts) == 0 {
		return Completion{}, fmt.Errorf("empty response from gemini")
	}
	text := strings.TrimSpace(response.Candidates[0].Content.Parts[0].Text)
	if text == "" {
		return Completion{}, fmt.Errorf("empty response from gemini")
	}
	return Completion{
		Text: text,
		Usage: usageFromTokens(
			c.options.Provider,
			c.options.Model,
			c.options.Profile,
			completionPrompt(systemInstruction, prompt),
			text,
			response.UsageMetadata.PromptTokenCount,
			response.UsageMetadata.CandidatesTokenCount,
			response.UsageMetadata.TotalTokenCount,
		),
	}, nil
}

func (c *Client) completeLMStudio(ctx context.Context, systemInstruction, prompt string) (Completion, error) {
	model, err := c.resolveLMStudioModel(ctx)
	if err != nil {
		return Completion{}, err
	}
	if err := c.ensureLMStudioModelLoaded(ctx, model); err != nil {
		return Completion{}, err
	}
	payload := struct {
		Model           string   `json:"model"`
		Input           string   `json:"input"`
		SystemPrompt    string   `json:"system_prompt,omitempty"`
		Reasoning       string   `json:"reasoning,omitempty"`
		MaxOutputTokens int      `json:"max_output_tokens,omitempty"`
		Temperature     *float64 `json:"temperature,omitempty"`
		Store           bool     `json:"store"`
	}{
		Model:           model,
		Input:           prompt,
		SystemPrompt:    systemInstruction,
		Reasoning:       c.options.Reasoning,
		MaxOutputTokens: c.options.MaxTokens,
		Temperature:     c.options.Temperature,
		Store:           false,
	}
	headers := map[string]string{}
	if c.options.APIKey != "" {
		headers["Authorization"] = "Bearer " + c.options.APIKey
	}
	body, err := c.postJSON(ctx, buildLMStudioChatURL(c.options.BaseURL), headers, payload)
	if err != nil {
		return Completion{}, err
	}
	var response struct {
		Output []struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"output"`
		Usage struct {
			InputTokens      int64 `json:"input_tokens"`
			OutputTokens     int64 `json:"output_tokens"`
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return Completion{}, fmt.Errorf("failed to unmarshal lm studio response: %w", err)
	}
	messages := []string{}
	for _, item := range response.Output {
		if item.Type == "message" && strings.TrimSpace(item.Content) != "" {
			messages = append(messages, item.Content)
		}
	}
	if len(messages) == 0 {
		return Completion{}, fmt.Errorf("empty response from lm studio")
	}
	text := strings.Join(messages, "\n")
	promptTokens := response.Usage.PromptTokens
	if promptTokens == 0 {
		promptTokens = response.Usage.InputTokens
	}
	completionTokens := response.Usage.CompletionTokens
	if completionTokens == 0 {
		completionTokens = response.Usage.OutputTokens
	}
	return Completion{
		Text: strings.TrimSpace(text),
		Usage: usageFromTokens(
			c.options.Provider,
			model,
			c.options.Profile,
			completionPrompt(systemInstruction, prompt),
			text,
			promptTokens,
			completionTokens,
			response.Usage.TotalTokens,
		),
	}, nil
}

type lmStudioModelsResponse struct {
	Models []lmStudioModelInfo `json:"models"`
	Data   []struct {
		ID string `json:"id"`
	} `json:"data"`
}

type lmStudioModelInfo struct {
	Type            string `json:"type"`
	Key             string `json:"key"`
	SelectedVariant string `json:"selected_variant"`
	LoadedInstances []struct {
		ID string `json:"id"`
	} `json:"loaded_instances"`
	Variants []string `json:"variants"`
}

func (c *Client) resolveLMStudioModel(ctx context.Context) (string, error) {
	if model := strings.TrimSpace(c.options.Model); model != "" {
		return model, nil
	}
	modelsResp, err := c.fetchLMStudioModels(ctx)
	if err != nil {
		return "", err
	}
	for _, candidate := range modelsResp.Models {
		if candidate.Type != "" && candidate.Type != "llm" {
			continue
		}
		if model := strings.TrimSpace(candidate.Key); model != "" {
			c.options.Model = model
			return model, nil
		}
	}
	for _, candidate := range modelsResp.Data {
		if model := strings.TrimSpace(candidate.ID); model != "" {
			c.options.Model = model
			return model, nil
		}
	}
	return "", fmt.Errorf("lm studio did not return any usable models")
}

func (c *Client) ensureLMStudioModelLoaded(ctx context.Context, model string) error {
	available, loaded, err := c.lmStudioModelAvailability(ctx, model)
	if err != nil {
		return err
	}
	if loaded {
		return nil
	}
	if !available {
		return fmt.Errorf("lm studio model %q does not exist", model)
	}
	_, err = c.postJSON(ctx, buildLMStudioModelLoadURL(c.options.BaseURL), c.lmStudioHeaders(), map[string]string{"model": model})
	if err != nil {
		return fmt.Errorf("failed to load lm studio model %q: %w", model, err)
	}
	return nil
}

func (c *Client) lmStudioModelAvailability(ctx context.Context, model string) (bool, bool, error) {
	modelsResp, err := c.fetchLMStudioModels(ctx)
	if err != nil {
		return false, false, err
	}
	for _, candidate := range modelsResp.Models {
		if candidate.Type != "" && candidate.Type != "llm" {
			continue
		}
		available := strings.TrimSpace(candidate.Key) == model || strings.TrimSpace(candidate.SelectedVariant) == model
		if !available {
			for _, variant := range candidate.Variants {
				if strings.TrimSpace(variant) == model {
					available = true
					break
				}
			}
		}
		for _, instance := range candidate.LoadedInstances {
			if strings.TrimSpace(instance.ID) == model {
				return true, true, nil
			}
		}
		if available {
			return true, false, nil
		}
	}
	for _, candidate := range modelsResp.Data {
		if strings.TrimSpace(candidate.ID) == model {
			return true, false, nil
		}
	}
	return false, false, nil
}

func (c *Client) fetchLMStudioModels(ctx context.Context) (lmStudioModelsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildLMStudioModelsURL(c.options.BaseURL), nil)
	if err != nil {
		return lmStudioModelsResponse{}, fmt.Errorf("failed to build lm studio model discovery request: %w", err)
	}
	for name, value := range c.lmStudioHeaders() {
		req.Header.Set(name, value)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return lmStudioModelsResponse{}, fmt.Errorf("lm studio model discovery cancelled: %w", ctxErr)
		}
		return lmStudioModelsResponse{}, fmt.Errorf("failed to discover lm studio models: %w", err)
	}
	defer resp.Body.Close()
	body, err := readResponseBody(resp.Body)
	if err != nil {
		return lmStudioModelsResponse{}, fmt.Errorf("failed to read lm studio model discovery response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return lmStudioModelsResponse{}, fmt.Errorf("lm studio model discovery returned non-2xx status: %s, body: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var modelsResp lmStudioModelsResponse
	if err := json.Unmarshal(body, &modelsResp); err != nil {
		return lmStudioModelsResponse{}, fmt.Errorf("failed to unmarshal lm studio models response: %w", err)
	}
	return modelsResp, nil
}

func (c *Client) lmStudioHeaders() map[string]string {
	headers := map[string]string{}
	if c.options.APIKey != "" {
		headers["Authorization"] = "Bearer " + c.options.APIKey
	}
	return headers
}

func (c *Client) postJSON(ctx context.Context, endpoint string, headers map[string]string, payload any) ([]byte, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %s request: %w", c.options.Provider, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to build %s request: %w", c.options.Provider, err)
	}
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		if strings.TrimSpace(value) != "" {
			req.Header.Set(name, value)
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("%s request cancelled: %w", c.options.Provider, ctxErr)
		}
		return nil, fmt.Errorf("failed to call %s api: %w", c.options.Provider, err)
	}
	defer resp.Body.Close()
	body, err := readResponseBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s response: %w", c.options.Provider, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%s api returned non-2xx status: %s, body: %s", c.options.Provider, resp.Status, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func (c *Client) maxTokens() int {
	if c.options.MaxTokens > 0 {
		return c.options.MaxTokens
	}
	return defaultMaxTokens
}

func completionPrompt(systemInstruction, prompt string) string {
	if strings.TrimSpace(systemInstruction) == "" {
		return prompt
	}
	return systemInstruction + "\n\n" + prompt
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
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
		TotalTokens      int64 `json:"total_tokens"`
	} `json:"usage"`
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
	messages := []string{}
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
	if strings.HasSuffix(strings.ToLower(trimmed), "/chat/completions") {
		return trimmed
	}
	return trimmed + "/chat/completions"
}

func buildAzureOpenAIChatCompletionsURL(baseURL, deployment, apiVersion string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	lower := strings.ToLower(trimmed)
	if strings.TrimSpace(apiVersion) != "" || (strings.TrimSpace(deployment) != "" && !strings.Contains(lower, "/openai/v1")) {
		version := strings.TrimSpace(apiVersion)
		if version == "" {
			version = "2024-10-21"
		}
		return trimmed + "/openai/deployments/" + url.PathEscape(strings.TrimSpace(deployment)) + "/chat/completions?api-version=" + url.QueryEscape(version)
	}
	if strings.HasSuffix(lower, "/chat/completions") {
		return trimmed
	}
	if strings.HasSuffix(lower, "/openai/v1") {
		return trimmed + "/chat/completions"
	}
	return trimmed + "/openai/v1/chat/completions"
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

func buildLMStudioChatURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(strings.ToLower(trimmed), "/api/v1/chat") {
		return trimmed
	}
	if strings.HasSuffix(strings.ToLower(trimmed), "/api/v1") {
		return trimmed + "/chat"
	}
	return trimmed + "/api/v1/chat"
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

func buildLMStudioModelLoadURL(baseURL string) string {
	return strings.TrimRight(buildLMStudioModelsURL(baseURL), "/") + "/load"
}

func readResponseBody(body io.Reader) ([]byte, error) {
	contents, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > maxResponseBytes {
		return nil, fmt.Errorf("LLM response exceeded %d bytes", maxResponseBytes)
	}
	return contents, nil
}

func usageFromTokens(provider, model, profile, prompt, completion string, promptTokens, completionTokens, totalTokens int64) Usage {
	usage := Usage{
		Provider:         strings.TrimSpace(provider),
		Model:            strings.TrimSpace(model),
		Profile:          strings.TrimSpace(profile),
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if usage.TotalTokens > 0 || usage.PromptTokens > 0 || usage.CompletionTokens > 0 {
		return usage
	}
	usage.PromptTokens = estimateTokenCount(prompt)
	usage.CompletionTokens = estimateTokenCount(completion)
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	usage.Estimated = usage.TotalTokens > 0
	return usage
}

func estimateTokenCount(text string) int64 {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	runes := len([]rune(text))
	estimate := int64((runes + 3) / 4)
	if estimate < 1 {
		return 1
	}
	return estimate
}

func normalizeExtra(extra map[string]string) map[string]string {
	if len(extra) == 0 {
		return map[string]string{}
	}
	normalized := make(map[string]string, len(extra))
	for key, value := range extra {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		normalized[key] = strings.TrimSpace(value)
	}
	return normalized
}

package nopsai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"nopsai/config"
)

func testOpenAICompatibleProfile(ctx context.Context, profile config.LLMProfile, apiKey string) (string, error) {
	headers := map[string]string{}
	if strings.TrimSpace(apiKey) != "" {
		headers["Authorization"] = "Bearer " + strings.TrimSpace(apiKey)
	}
	switch profile.Provider {
	case config.LLMProviderOpenAI:
		if value := profile.Extra["organization"]; value != "" {
			headers["OpenAI-Organization"] = value
		}
		if value := profile.Extra["project"]; value != "" {
			headers["OpenAI-Project"] = value
		}
	case config.LLMProviderOpenRouter:
		if value := profile.Extra["http_referer"]; value != "" {
			headers["HTTP-Referer"] = value
		}
		if value := profile.Extra["x_title"]; value != "" {
			headers["X-Title"] = value
		}
	}
	return testOpenAIChatEndpoint(
		ctx,
		profile.Provider,
		buildNopsaiOpenAIChatURL(config.EffectiveLLMProfileBaseURL(profile)),
		headers,
		profile.Model,
		profile.Temperature,
	)
}

func testAzureOpenAIProfile(ctx context.Context, profile config.LLMProfile, apiKey string) (string, error) {
	model := strings.TrimSpace(profile.Model)
	if deployment := strings.TrimSpace(profile.Extra["deployment"]); deployment != "" {
		model = deployment
	}
	return testOpenAIChatEndpoint(
		ctx,
		profile.Provider,
		buildNopsaiAzureOpenAIChatURL(
			profile.BaseURL,
			profile.Extra["deployment"],
			profile.Extra["api_version"],
		),
		map[string]string{"api-key": strings.TrimSpace(apiKey)},
		model,
		profile.Temperature,
	)
}

func testOpenAIChatEndpoint(
	ctx context.Context,
	provider string,
	endpoint string,
	headers map[string]string,
	model string,
	temperature *float64,
) (string, error) {
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{{
			"role":    "user",
			"content": "reply ok",
		}},
	}
	if config.LLMProviderUsesMaxCompletionTokens(provider) {
		payload["max_completion_tokens"] = 16
	} else {
		payload["max_tokens"] = 16
	}
	if temperature != nil {
		payload["temperature"] = *temperature
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		if strings.TrimSpace(value) != "" {
			req.Header.Set(name, value)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("LLM api returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var response struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("LLM provider returned an empty response")
	}
	return nopsaiOpenAIMessageText(response.Choices[0].Message.Content)
}

func nopsaiOpenAIMessageText(raw json.RawMessage) (string, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if text = strings.TrimSpace(text); text != "" {
			return text, nil
		}
		return "", fmt.Errorf("LLM provider returned an empty response")
	}

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", fmt.Errorf("LLM provider returned invalid message content")
	}
	messages := make([]string, 0, len(parts))
	for _, part := range parts {
		if (part.Type == "" || part.Type == "text") && strings.TrimSpace(part.Text) != "" {
			messages = append(messages, strings.TrimSpace(part.Text))
		}
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("LLM provider returned an empty response")
	}
	return strings.Join(messages, "\n"), nil
}

func testAnthropicProfile(ctx context.Context, profile config.LLMProfile, apiKey string) (string, error) {
	baseURL := config.EffectiveLLMProfileBaseURL(profile)
	endpoint := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(strings.ToLower(endpoint), "/v1/messages") {
		if strings.HasSuffix(strings.ToLower(endpoint), "/v1") {
			endpoint += "/messages"
		} else {
			endpoint += "/v1/messages"
		}
	}
	payload := map[string]any{
		"model":      profile.Model,
		"max_tokens": 16,
		"messages": []map[string]string{{
			"role":    "user",
			"content": "reply ok",
		}},
	}
	if profile.Temperature != nil {
		payload["temperature"] = *profile.Temperature
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", strings.TrimSpace(apiKey))
	version := strings.TrimSpace(profile.Extra["anthropic_version"])
	if version == "" {
		version = "2023-06-01"
	}
	req.Header.Set("anthropic-version", version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("anthropic api returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var response struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}
	for _, part := range response.Content {
		if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
			return strings.TrimSpace(part.Text), nil
		}
	}
	return "", fmt.Errorf("anthropic returned an empty response")
}

func buildNopsaiOpenAIChatURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(strings.ToLower(trimmed), "/chat/completions") {
		return trimmed
	}
	return trimmed + "/chat/completions"
}

func buildNopsaiAzureOpenAIChatURL(baseURL, deployment, apiVersion string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	lower := strings.ToLower(trimmed)
	if strings.TrimSpace(apiVersion) != "" || (strings.TrimSpace(deployment) != "" && !strings.Contains(lower, "/openai/v1")) {
		version := strings.TrimSpace(apiVersion)
		if version == "" {
			version = "2024-10-21"
		}
		return trimmed +
			"/openai/deployments/" + url.PathEscape(strings.TrimSpace(deployment)) +
			"/chat/completions?api-version=" + url.QueryEscape(version)
	}
	if strings.HasSuffix(lower, "/chat/completions") {
		return trimmed
	}
	if strings.HasSuffix(lower, "/openai/v1") {
		return trimmed + "/chat/completions"
	}
	return trimmed + "/openai/v1/chat/completions"
}

package llm

import (
	"context"
	"net/url"
	"strings"
)

type azureOpenAIClient struct {
	owner       *LLMClient
	apiKey      string
	model       string
	baseURL     string
	deployment  string
	apiVersion  string
	maxTokens   int
	temperature *float64
}

func newAzureOpenAIClient(owner *LLMClient, options LLMClientOptions) ProviderClient {
	maxTokens := options.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultLLMMaxTokens
	}
	return &azureOpenAIClient{
		owner:       owner,
		apiKey:      options.APIKey,
		model:       options.Model,
		baseURL:     options.BaseURL,
		deployment:  options.Extra["deployment"],
		apiVersion:  options.Extra["api_version"],
		maxTokens:   maxTokens,
		temperature: options.Temperature,
	}
}

func (c *azureOpenAIClient) Name() string {
	return c.owner.provider
}

func (c *azureOpenAIClient) Complete(ctx context.Context, prompt string) (string, error) {
	model := c.model
	if c.deployment != "" {
		model = c.deployment
	}
	return completeOpenAIChat(
		ctx,
		c.owner,
		buildAzureOpenAIChatCompletionsURL(c.baseURL, c.deployment, c.apiVersion),
		map[string]string{"api-key": c.apiKey},
		model,
		prompt,
		c.maxTokens,
		c.temperature,
	)
}

func buildAzureOpenAIChatCompletionsURL(baseURL, deployment, apiVersion string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	lower := strings.ToLower(trimmed)
	useLegacyRoute := strings.TrimSpace(apiVersion) != "" ||
		(strings.TrimSpace(deployment) != "" && !strings.Contains(lower, "/openai/v1"))
	if useLegacyRoute {
		version := strings.TrimSpace(apiVersion)
		if version == "" {
			version = "2024-10-21"
		}
		return trimmed +
			"/openai/deployments/" + url.PathEscape(strings.TrimSpace(deployment)) +
			"/chat/completions?api-version=" + url.QueryEscape(version)
	}

	switch {
	case strings.HasSuffix(lower, "/chat/completions"):
		return trimmed
	case strings.HasSuffix(lower, "/openai/v1"):
		return trimmed + "/chat/completions"
	default:
		return trimmed + "/openai/v1/chat/completions"
	}
}

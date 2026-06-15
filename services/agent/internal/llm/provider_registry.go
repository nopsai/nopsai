package llm

import (
	"context"
	"fmt"

	appconfig "nopsai/config"
)

func newProviderClient(owner *LLMClient, options LLMClientOptions) ProviderClient {
	switch options.Provider {
	case appconfig.LLMProviderGemini:
		return newGeminiClient(owner, options)
	case appconfig.LLMProviderLMStudio:
		return newLMStudioClient(owner, options)
	case appconfig.LLMProviderOpenAI,
		appconfig.LLMProviderGroq,
		appconfig.LLMProviderMistral,
		appconfig.LLMProviderOllama,
		appconfig.LLMProviderOpenRouter:
		return newOpenAICompatibleClient(owner, options)
	case appconfig.LLMProviderAnthropic:
		return newAnthropicClient(owner, options)
	case appconfig.LLMProviderAzureOpenAI:
		return newAzureOpenAIClient(owner, options)
	default:
		return unsupportedProviderClient{name: options.Provider}
	}
}

type unsupportedProviderClient struct {
	name string
}

func (c unsupportedProviderClient) Name() string {
	return c.name
}

func (c unsupportedProviderClient) Complete(context.Context, string) (string, error) {
	return "", fmt.Errorf("unsupported llm provider: %s", c.name)
}

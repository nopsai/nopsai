package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	appconfig "nopsai/config"
)

type ProviderClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
	Name() string
}

type LLMClientOptions struct {
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
}

type LLMClient struct {
	provider       string
	profile        string
	httpClient     *http.Client
	providerClient ProviderClient
}

const maxMCPToolCallsPerAction = 8

type nonRetryableGoalResolutionError struct {
	message string
}

func (e *nonRetryableGoalResolutionError) Error() string {
	return e.message
}

func newNonRetryableGoalResolutionError(format string, args ...any) error {
	return &nonRetryableGoalResolutionError{message: fmt.Sprintf(format, args...)}
}

func IsNonRetryableGoalResolutionError(err error) bool {
	var target *nonRetryableGoalResolutionError
	return errors.As(err, &target)
}

func NewLLMClient(provider, apiKey, model, baseURL, reasoning string, profileName ...string) *LLMClient {
	options := LLMClientOptions{
		Provider:  provider,
		APIKey:    apiKey,
		Model:     model,
		BaseURL:   baseURL,
		Reasoning: reasoning,
	}
	if len(profileName) > 0 {
		options.Profile = profileName[0]
	}
	return NewLLMClientWithOptions(options)
}

func NewLLMClientWithOptions(options LLMClientOptions) *LLMClient {
	timeout := time.Duration(0)
	if options.TimeoutSeconds > 0 {
		timeout = time.Duration(options.TimeoutSeconds) * time.Second
	}
	client := &LLMClient{
		provider:   appconfig.NormalizeLLMProvider(options.Provider),
		profile:    strings.TrimSpace(options.Profile),
		httpClient: &http.Client{Timeout: timeout},
	}
	options.Provider = client.provider
	options.Profile = client.profile
	options.APIKey = strings.TrimSpace(options.APIKey)
	options.Model = strings.TrimSpace(options.Model)
	options.BaseURL = strings.TrimSpace(options.BaseURL)
	options.Reasoning = appconfig.NormalizeLMStudioReasoning(options.Reasoning)
	if len(options.Extra) > 0 {
		normalizedExtra := make(map[string]string, len(options.Extra))
		for key, value := range options.Extra {
			if key = strings.TrimSpace(key); key != "" {
				normalizedExtra[key] = strings.TrimSpace(value)
			}
		}
		options.Extra = normalizedExtra
	}
	client.providerClient = newProviderClient(client, options)
	return client
}

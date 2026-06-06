package llm

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	appconfig "nopsai/config"
)

type LLMClient struct {
	provider   string
	profile    string
	apiKey     string
	model      string
	baseURL    string
	reasoning  string
	httpClient *http.Client

	modelMu     sync.Mutex
	loadedModel string
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

type lmStudioEndpointGate struct {
	sem chan struct{}
}

var lmStudioEndpointLoadGates sync.Map

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

func NewLLMClient(provider, apiKey, model, baseURL, reasoning string, profileName ...string) *LLMClient {
	client := &LLMClient{
		provider:   appconfig.NormalizeLLMProvider(provider),
		apiKey:     strings.TrimSpace(apiKey),
		model:      strings.TrimSpace(model),
		baseURL:    strings.TrimSpace(baseURL),
		reasoning:  appconfig.NormalizeLMStudioReasoning(reasoning),
		httpClient: &http.Client{},
	}
	if len(profileName) > 0 {
		client.profile = strings.TrimSpace(profileName[0])
	}
	return client
}

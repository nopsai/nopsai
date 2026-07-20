package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	appconfig "nopsai/config"
	"nopsai/pkg/models"

	"github.com/rs/zerolog/log"
)

type lmStudioClient struct {
	owner       *LLMClient
	apiKey      string
	model       string
	baseURL     string
	reasoning   string
	maxTokens   int
	temperature *float64
	modelMu     sync.Mutex
	loadedModel string
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

func newLMStudioClient(owner *LLMClient, options LLMClientOptions) ProviderClient {
	return &lmStudioClient{
		owner:       owner,
		apiKey:      options.APIKey,
		model:       options.Model,
		baseURL:     options.BaseURL,
		reasoning:   options.Reasoning,
		maxTokens:   options.MaxTokens,
		temperature: options.Temperature,
	}
}

func (c *lmStudioClient) Name() string {
	return c.owner.provider
}

func (c *lmStudioClient) Complete(ctx context.Context, prompt string) (string, error) {
	return c.callLMStudio(ctx, prompt)
}

func (c *LLMClient) callLMStudioForBoolean(ctx context.Context, prompt string) (bool, error) {
	responseText, err := c.providerClient.Complete(ctx, prompt)
	if err != nil {
		return false, err
	}
	return parseBooleanText(responseText)
}

func (c *LLMClient) callLMStudioForAction(ctx context.Context, prompt string) (*models.Action, error) {
	responseText, err := c.providerClient.Complete(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return decodeActionResponse(responseText)
}

func (c *lmStudioClient) callLMStudio(ctx context.Context, prompt string) (string, error) {
	model, err := c.resolveLMStudioModel(ctx)
	if err != nil {
		return "", err
	}
	if err := c.ensureLMStudioModelLoaded(ctx, model); err != nil {
		return "", err
	}

	reqPayload := struct {
		Model           string   `json:"model"`
		Input           string   `json:"input"`
		Reasoning       string   `json:"reasoning,omitempty"`
		MaxOutputTokens int      `json:"max_output_tokens,omitempty"`
		Temperature     *float64 `json:"temperature,omitempty"`
		Store           bool     `json:"store"`
	}{
		Model:           model,
		Input:           prompt,
		Reasoning:       appconfig.LMStudioReasoningRequestValue(c.reasoning),
		MaxOutputTokens: c.maxTokens,
		Temperature:     c.temperature,
		Store:           false,
	}

	logEvent := log.Debug().Str("model", model).Str("endpoint", buildLMStudioChatURL(c.baseURL))
	if c.owner.profile != "" {
		logEvent = logEvent.Str("llm_profile", c.owner.profile)
	}
	if c.reasoning != "" {
		logEvent = logEvent.Str("reasoning", c.reasoning)
	}
	logEvent.Msg("Calling LM Studio native chat API")

	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal lm studio request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, buildLMStudioChatURL(c.baseURL), bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to build lm studio request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.owner.httpClient.Do(httpReq)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("lm studio chat request cancelled: %w", ctxErr)
		}
		return "", fmt.Errorf("failed to call lm studio api: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lm studio api returned non-200 status: %s, body: %s", resp.Status, string(body))
	}

	var lmStudioResp struct {
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
	if err := json.Unmarshal(body, &lmStudioResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal lm studio response: %w", err)
	}
	if len(lmStudioResp.Output) == 0 {
		return "", fmt.Errorf("invalid or empty response from lm studio: %s", string(body))
	}

	var messages []string
	for _, item := range lmStudioResp.Output {
		if item.Type == "message" && strings.TrimSpace(item.Content) != "" {
			messages = append(messages, item.Content)
		}
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("lm studio response did not contain a message item: %s", string(body))
	}
	responseText := strings.Join(messages, "\n")
	c.recordLMStudioUsage(ctx, model, prompt, responseText, lmStudioResp.Usage.InputTokens, lmStudioResp.Usage.OutputTokens, lmStudioResp.Usage.PromptTokens, lmStudioResp.Usage.CompletionTokens, lmStudioResp.Usage.TotalTokens)

	return responseText, nil
}

func (c *lmStudioClient) recordLMStudioUsage(ctx context.Context, model, prompt, completion string, inputTokens, outputTokens, promptTokens, completionTokens, totalTokens int64) {
	if promptTokens == 0 {
		promptTokens = inputTokens
	}
	if completionTokens == 0 {
		completionTokens = outputTokens
	}
	recordUsage(ctx, usageFromTokenDetailsForClient(c.owner, model, prompt, completion, promptTokens, completionTokens, totalTokens, 0))
}

func lmStudioEndpointLoadGateFor(baseURL string) *lmStudioEndpointGate {
	key := buildLMStudioChatURL(baseURL)
	actual, _ := lmStudioEndpointLoadGates.LoadOrStore(key, &lmStudioEndpointGate{sem: make(chan struct{}, 1)})
	return actual.(*lmStudioEndpointGate)
}

func (g *lmStudioEndpointGate) acquire(ctx context.Context) (func(), error) {
	select {
	case g.sem <- struct{}{}:
		return func() {
			<-g.sem
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *lmStudioClient) ensureLMStudioModelLoaded(ctx context.Context, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("lm studio model is required")
	}
	if c.isLMStudioModelMarkedLoaded(model) {
		return nil
	}

	available, loaded, err := c.lmStudioModelAvailability(ctx, model)
	if err != nil {
		return err
	}
	if loaded {
		c.markLMStudioModelLoaded(model)
		return nil
	}
	if !available {
		return fmt.Errorf("lm studio model %q does not exist", model)
	}

	releaseGate, err := lmStudioEndpointLoadGateFor(c.baseURL).acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed waiting for lm studio model load lock: %w", err)
	}
	defer releaseGate()

	available, loaded, err = c.lmStudioModelAvailability(ctx, model)
	if err != nil {
		return err
	}
	if loaded {
		c.markLMStudioModelLoaded(model)
		return nil
	}
	if !available {
		return fmt.Errorf("lm studio model %q does not exist", model)
	}

	if err := c.loadLMStudioModel(ctx, model); err != nil {
		return err
	}
	c.markLMStudioModelLoaded(model)
	return nil
}

func (c *lmStudioClient) isLMStudioModelMarkedLoaded(model string) bool {
	c.modelMu.Lock()
	defer c.modelMu.Unlock()
	return strings.TrimSpace(c.loadedModel) == strings.TrimSpace(model)
}

func (c *lmStudioClient) markLMStudioModelLoaded(model string) {
	c.modelMu.Lock()
	c.loadedModel = strings.TrimSpace(model)
	c.modelMu.Unlock()
}

func (c *lmStudioClient) lmStudioModelAvailability(ctx context.Context, model string) (bool, bool, error) {
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

func (c *lmStudioClient) loadLMStudioModel(ctx context.Context, model string) error {
	reqPayload := struct {
		Model string `json:"model"`
	}{
		Model: model,
	}
	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal lm studio model load request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, buildLMStudioModelLoadURL(c.baseURL), bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to build lm studio model load request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.owner.httpClient.Do(httpReq)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("lm studio model load cancelled: %w", ctxErr)
		}
		return fmt.Errorf("failed to load lm studio model: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("lm studio model load returned non-2xx status: %s, body: %s", resp.Status, string(body))
	}

	return nil
}

func (c *lmStudioClient) fetchLMStudioModels(ctx context.Context) (lmStudioModelsResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, buildLMStudioModelsURL(c.baseURL), nil)
	if err != nil {
		return lmStudioModelsResponse{}, fmt.Errorf("failed to build lm studio model discovery request: %w", err)
	}
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.owner.httpClient.Do(httpReq)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return lmStudioModelsResponse{}, fmt.Errorf("lm studio model discovery cancelled: %w", ctxErr)
		}
		return lmStudioModelsResponse{}, fmt.Errorf("failed to discover lm studio models: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return lmStudioModelsResponse{}, fmt.Errorf("lm studio model discovery returned non-200 status: %s, body: %s", resp.Status, string(body))
	}

	var modelsResp lmStudioModelsResponse
	if err := json.Unmarshal(body, &modelsResp); err != nil {
		return lmStudioModelsResponse{}, fmt.Errorf("failed to unmarshal lm studio models response: %w", err)
	}

	return modelsResp, nil
}

func (c *lmStudioClient) resolveLMStudioModel(ctx context.Context) (string, error) {
	c.modelMu.Lock()
	configuredModel := strings.TrimSpace(c.model)
	c.modelMu.Unlock()
	if configuredModel != "" {
		return configuredModel, nil
	}

	modelsResp, err := c.fetchLMStudioModels(ctx)
	if err != nil {
		return "", err
	}

	discoveredModel := ""
	for _, candidate := range modelsResp.Models {
		if candidate.Type == "" || candidate.Type == "llm" {
			if strings.TrimSpace(candidate.Key) != "" {
				discoveredModel = strings.TrimSpace(candidate.Key)
				break
			}
		}
	}
	if discoveredModel == "" {
		for _, candidate := range modelsResp.Data {
			if strings.TrimSpace(candidate.ID) != "" {
				discoveredModel = strings.TrimSpace(candidate.ID)
				break
			}
		}
	}
	if discoveredModel == "" {
		return "", fmt.Errorf("lm studio did not return any usable models")
	}

	c.modelMu.Lock()
	if strings.TrimSpace(c.model) == "" {
		c.model = discoveredModel
	} else {
		discoveredModel = strings.TrimSpace(c.model)
	}
	c.modelMu.Unlock()

	return discoveredModel, nil
}

func buildLMStudioChatURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	lower := strings.ToLower(trimmed)

	switch {
	case strings.HasSuffix(lower, "/api/v1/chat"):
		return trimmed
	case strings.HasSuffix(lower, "/api/v1"):
		return trimmed + "/chat"
	default:
		return trimmed + "/api/v1/chat"
	}
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

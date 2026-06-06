package agent

import (
	"errors"
	"strings"

	appconfig "nopsai/config"
	"nopsai/pkg/models"
	agentapp "nopsai/services/agent/internal/app"
	llmruntime "nopsai/services/agent/internal/llm"
	"nopsai/services/agent/internal/resolver"
)

type agentDispatcherConnection interface {
	Close() error
}

type agentRuntimeAdapters struct {
	PipelineLLMEnabled      bool
	LLMRegistry             *llmruntime.LLMProfileRegistry
	MCPRegistry             *llmruntime.MCPProfileRegistry
	ConditionClientResolver resolver.ConditionClientResolver
	ActionSessionResolver   resolver.ActionSessionResolver
}

type agentRuntimeWiringStage string

const (
	agentRuntimeWiringLLMProfiles agentRuntimeWiringStage = "llm_profiles"
	agentRuntimeWiringMCPRegistry agentRuntimeWiringStage = "mcp_registry"
)

type agentRuntimeWiringError struct {
	Stage agentRuntimeWiringStage
	Err   error
}

func (e agentRuntimeWiringError) Error() string {
	if e.Err == nil {
		return string(e.Stage)
	}
	return e.Err.Error()
}

func (e agentRuntimeWiringError) Unwrap() error {
	return e.Err
}

func newAgentRuntimeAdapters(runScope string, pipeline *models.Pipeline) (agentRuntimeAdapters, error) {
	adapters := agentRuntimeAdapters{
		PipelineLLMEnabled: models.PipelineLLMEnabled(pipeline),
	}

	if adapters.PipelineLLMEnabled {
		llmRegistry, err := llmruntime.NewLLMProfileRegistryFromEnv(runScope)
		if err != nil {
			return adapters, agentRuntimeWiringError{Stage: agentRuntimeWiringLLMProfiles, Err: err}
		}
		adapters.LLMRegistry = llmRegistry
		adapters.ConditionClientResolver = func(pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task) (resolver.ConditionClient, string, error) {
			return llmRegistry.ClientFor(pipeline, step, task)
		}
	}

	mcpRegistry, err := llmruntime.NewMCPProfileRegistryFromEnv(runScope)
	if err != nil {
		return adapters, agentRuntimeWiringError{Stage: agentRuntimeWiringMCPRegistry, Err: err}
	}
	adapters.MCPRegistry = mcpRegistry
	adapters.ActionSessionResolver = newAgentActionSessionResolver(adapters.LLMRegistry, adapters.MCPRegistry)
	return adapters, nil
}

func connectAgentDispatcher(lookup agentapp.EnvLookup) (agentDispatcherConnection, error) {
	conn, dispatcher, err := agentapp.NewDispatcherClientFromEnv(lookup)
	if err != nil {
		return nil, err
	}
	dispatcherClient = dispatcher
	return conn, nil
}

func logAgentRuntimeWiringError(runID, pipelineName string, err error) {
	var wiringErr agentRuntimeWiringError
	if errors.As(err, &wiringErr) {
		switch wiringErr.Stage {
		case agentRuntimeWiringLLMProfiles:
			agentLog(runID, pipelineName).Error().Err(wiringErr.Err).Msg("Invalid LLM profile configuration")
			return
		case agentRuntimeWiringMCPRegistry:
			agentLog(runID, pipelineName).Error().Err(wiringErr.Err).Msg("Invalid MCP registry configuration")
			return
		}
	}
	agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to initialize agent runtime adapters")
}

func logAgentRuntimeAdapters(runID, pipelineName string, adapters agentRuntimeAdapters) {
	if adapters.PipelineLLMEnabled {
		defaultLLMProfile, _ := adapters.LLMRegistry.DefaultProfile()
		startupLog := agentLog(runID, pipelineName).Info().
			Str("llm_profile", adapters.LLMRegistry.DefaultProfileName()).
			Str("llm_provider", defaultLLMProfile.Provider)
		switch defaultLLMProfile.Provider {
		case appconfig.LLMProviderGemini:
			startupLog.Str("llm_model", defaultLLMProfile.Model).Msg("Agent starting with embedded LLM profile registry")
		case appconfig.LLMProviderLMStudio:
			logEvent := startupLog.Str("lmstudio_base_url", defaultLLMProfile.BaseURL)
			if strings.TrimSpace(defaultLLMProfile.Model) != "" {
				logEvent = logEvent.Str("llm_model", defaultLLMProfile.Model)
			} else {
				logEvent = logEvent.Str("llm_model", "auto-discover")
			}
			if defaultLLMProfile.Reasoning != "" {
				logEvent = logEvent.Str("lmstudio_reasoning", defaultLLMProfile.Reasoning)
			}
			if defaultLLMProfile.Thinking != nil {
				logEvent = logEvent.Bool("lmstudio_thinking", *defaultLLMProfile.Thinking)
			}
			logEvent.Msg("Agent starting with embedded LLM profile registry")
		default:
			startupLog.Msg("Agent starting with embedded LLM profile registry")
		}
		return
	}
	agentLog(runID, pipelineName).Info().Msg("LLM is disabled for this pipeline; LLM profile registry will not be loaded")
}

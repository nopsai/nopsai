package agent

import (
	"context"
	"fmt"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	llmruntime "nopsai/services/agent/internal/llm"
	"nopsai/services/agent/internal/resolver"
)

type agentActionSession struct {
	client        *llmruntime.LLMClient
	runtime       *llmruntime.MCPTaskRuntime
	llmProfile    string
	agentProfile  string
	promptProfile llmruntime.AgentPromptProfile
}

type agentConditionClient struct {
	client        *llmruntime.LLMClient
	agentProfile  string
	promptProfile llmruntime.AgentPromptProfile
}

func newAgentConditionClientResolver(llmRegistry *llmruntime.LLMProfileRegistry, agentRegistry *llmruntime.AgentProfileRegistry) resolver.ConditionClientResolver {
	return func(pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task) (resolver.ConditionClient, string, error) {
		if llmRegistry == nil {
			return nil, "", fmt.Errorf("LLM profile registry is not initialized")
		}
		if agentRegistry == nil {
			return nil, "", fmt.Errorf("agent profile registry is not initialized")
		}
		conditionClient, llmProfile, err := llmRegistry.ClientFor(pipeline, step, task)
		if err != nil {
			return nil, "", err
		}
		promptProfile, agentProfile, err := agentRegistry.ProfileFor(pipeline, step)
		if err != nil {
			return nil, "", err
		}
		return &agentConditionClient{
			client:        conditionClient,
			agentProfile:  agentProfile,
			promptProfile: promptProfile,
		}, llmProfile, nil
	}
}

func newAgentActionSessionResolver(llmRegistry *llmruntime.LLMProfileRegistry, agentRegistry *llmruntime.AgentProfileRegistry, mcpRegistry *llmruntime.MCPProfileRegistry) resolver.ActionSessionResolver {
	return func(pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task) (resolver.ActionSession, error) {
		if llmRegistry == nil {
			return nil, resolver.NewActionSessionResolutionError(resolver.ActionSessionResolutionLLMProfile, fmt.Errorf("LLM profile registry is not initialized"))
		}
		if agentRegistry == nil {
			return nil, resolver.NewActionSessionResolutionError(resolver.ActionSessionResolutionAgentProfile, fmt.Errorf("agent profile registry is not initialized"))
		}
		actionClient, actionProfile, err := llmRegistry.ClientFor(pipeline, step, task)
		if err != nil {
			return nil, resolver.NewActionSessionResolutionError(resolver.ActionSessionResolutionLLMProfile, err)
		}
		promptProfile, agentProfile, err := agentRegistry.ProfileFor(pipeline, step)
		if err != nil {
			return nil, resolver.NewActionSessionResolutionError(resolver.ActionSessionResolutionAgentProfile, err)
		}
		mcpRuntime, err := mcpRegistry.ResolveFor(pipeline, step, task)
		if err != nil {
			return nil, resolver.NewActionSessionResolutionError(resolver.ActionSessionResolutionMCPProfile, err)
		}
		return &agentActionSession{
			client:        actionClient,
			runtime:       mcpRuntime,
			llmProfile:    actionProfile,
			agentProfile:  agentProfile,
			promptProfile: promptProfile,
		}, nil
	}
}

func (c *agentConditionClient) AgentProfileName() string {
	if c == nil {
		return ""
	}
	return c.agentProfile
}

func (c *agentConditionClient) EvaluateCondition(ctx context.Context, req *proto.ConditionRequest) (*proto.ConditionResponse, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("LLM condition client is not initialized")
	}
	return c.client.EvaluateConditionWithAgentProfile(ctx, req, c.promptProfile)
}

func (s *agentActionSession) ProfileName() string {
	return s.LLMProfileName()
}

func (s *agentActionSession) LLMProfileName() string {
	if s == nil {
		return ""
	}
	return s.llmProfile
}

func (s *agentActionSession) AgentProfileName() string {
	if s == nil {
		return ""
	}
	return s.agentProfile
}

func (s *agentActionSession) MCPEnabled() bool {
	return s != nil && s.runtime.Enabled()
}

func (s *agentActionSession) MCPProfiles() []string {
	if s == nil {
		return nil
	}
	return s.runtime.Profiles()
}

func (s *agentActionSession) MCPToolCount() int {
	if s == nil || s.runtime == nil {
		return 0
	}
	return s.runtime.ToolCount()
}

func (s *agentActionSession) RequiresMCPToolCall() bool {
	return s != nil && s.runtime.RequiresToolCall()
}

func (s *agentActionSession) SuccessfulMCPToolCalls() int {
	if s == nil {
		return 0
	}
	return s.runtime.SuccessfulToolCalls()
}

func (s *agentActionSession) GetAction(ctx context.Context, req *proto.GetActionRequest) (*proto.Action, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("LLM action client is not initialized")
	}
	return s.client.GetActionWithMCPAndAgentProfile(ctx, req, s.runtime, s.promptProfile)
}

package agent

import (
	"context"
	"fmt"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	"nopsai/services/agent/internal/resolver"
)

type agentActionSession struct {
	client  *LLMClient
	runtime *MCPTaskRuntime
	profile string
}

func newAgentActionSessionResolver(llmRegistry *LLMProfileRegistry, mcpRegistry *MCPProfileRegistry) resolver.ActionSessionResolver {
	return func(pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task) (resolver.ActionSession, error) {
		if llmRegistry == nil {
			return nil, resolver.NewActionSessionResolutionError(resolver.ActionSessionResolutionLLMProfile, fmt.Errorf("LLM profile registry is not initialized"))
		}
		actionClient, actionProfile, err := llmRegistry.ClientFor(pipeline, step, task)
		if err != nil {
			return nil, resolver.NewActionSessionResolutionError(resolver.ActionSessionResolutionLLMProfile, err)
		}
		mcpRuntime, err := mcpRegistry.ResolveFor(pipeline, step, task)
		if err != nil {
			return nil, resolver.NewActionSessionResolutionError(resolver.ActionSessionResolutionMCPProfile, err)
		}
		return &agentActionSession{
			client:  actionClient,
			runtime: mcpRuntime,
			profile: actionProfile,
		}, nil
	}
}

func (s *agentActionSession) ProfileName() string {
	if s == nil {
		return ""
	}
	return s.profile
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
	return len(s.runtime.tools)
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
	return s.client.GetActionWithMCP(ctx, req, s.runtime)
}

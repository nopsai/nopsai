package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"nopsai/pkg/mcpclient"
	"nopsai/pkg/models"
	"nopsai/services/nopsai/pkg/validation"
)

const mcpRegistryRuntimeEnv = "NOPSAI_MCP_REGISTRY"

type agentRuntimeMCPServer struct {
	models.MCPServer
	AuthValue string `json:"auth_value,omitempty"`
}

type agentRuntimeMCPRegistryPayload struct {
	Servers  map[string]agentRuntimeMCPServer `json:"servers"`
	Profiles map[string]models.MCPProfile     `json:"profiles"`
	Tools    map[string][]models.MCPTool      `json:"tools"`
}

type MCPProfileRegistry struct {
	servers  map[string]agentRuntimeMCPServer
	profiles map[string]models.MCPProfile
	tools    map[string][]models.MCPTool
	scope    string

	mu      sync.Mutex
	clients map[string]*mcpclient.Client
}

type MCPToolSpec struct {
	Server      string
	Name        string
	Description string
	InputSchema string
}

type MCPTaskRuntime struct {
	registry                 *MCPProfileRegistry
	profiles                 []string
	tools                    []MCPToolSpec
	allowed                  map[string]MCPToolSpec
	requireToolCall          bool
	mu                       sync.Mutex
	successfulToolCalls      int
	successfulToolTranscript string
}

func NewMCPProfileRegistryFromEnv(scope string) (*MCPProfileRegistry, error) {
	raw := strings.TrimSpace(os.Getenv(mcpRegistryRuntimeEnv))
	if raw == "" {
		return &MCPProfileRegistry{
			servers:  map[string]agentRuntimeMCPServer{},
			profiles: map[string]models.MCPProfile{},
			tools:    map[string][]models.MCPTool{},
			scope:    strings.Trim(strings.TrimSpace(scope), "/"),
			clients:  map[string]*mcpclient.Client{},
		}, nil
	}
	payloadBytes, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode MCP registry: %w", err)
	}
	var payload agentRuntimeMCPRegistryPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("parse MCP registry: %w", err)
	}
	if payload.Servers == nil {
		payload.Servers = map[string]agentRuntimeMCPServer{}
	}
	if payload.Profiles == nil {
		payload.Profiles = map[string]models.MCPProfile{}
	}
	if payload.Tools == nil {
		payload.Tools = map[string][]models.MCPTool{}
	}
	return &MCPProfileRegistry{
		servers:  payload.Servers,
		profiles: payload.Profiles,
		tools:    payload.Tools,
		scope:    strings.Trim(strings.TrimSpace(scope), "/"),
		clients:  map[string]*mcpclient.Client{},
	}, nil
}

func (r *MCPProfileRegistry) ResolveFor(pipeline *models.Pipeline, step *models.PipelineStep, task *models.Task) (*MCPTaskRuntime, error) {
	if r == nil {
		return &MCPTaskRuntime{allowed: map[string]MCPToolSpec{}}, nil
	}
	var pipelineProfiles, stepProfiles, taskProfiles []string
	if pipeline != nil {
		pipelineProfiles = pipeline.MCPProfiles
	}
	if step != nil {
		stepProfiles = step.GetMCPProfiles()
	}
	if task != nil {
		taskProfiles = task.MCPProfiles
	}
	profileNames := validation.ResolvePipelineMCPProfiles(pipelineProfiles, stepProfiles, taskProfiles)
	runtime := &MCPTaskRuntime{
		registry:        r,
		profiles:        profileNames,
		allowed:         map[string]MCPToolSpec{},
		requireToolCall: len(profileNames) > 0,
	}
	if len(profileNames) == 0 {
		return runtime, nil
	}

	toolsByServer := map[string]map[string]models.MCPTool{}
	for serverName, tools := range r.tools {
		toolsByServer[serverName] = map[string]models.MCPTool{}
		for _, tool := range tools {
			toolsByServer[serverName][tool.Name] = tool
		}
	}

	for _, profileName := range profileNames {
		profile, ok := r.profiles[profileName]
		if !ok {
			return nil, fmt.Errorf("MCP profile %q is not available to this run", profileName)
		}
		if !profile.Enabled {
			return nil, fmt.Errorf("MCP profile %q is disabled", profileName)
		}
		if !models.MCPAllowedInScope(profile.AllowedScopes, r.scope) {
			return nil, fmt.Errorf("MCP profile %q is not allowed in scope %q", profileName, r.scope)
		}
		for _, ref := range profile.ServerRefs {
			server, ok := r.servers[ref.ServerName]
			if !ok {
				return nil, fmt.Errorf("MCP profile %q references unavailable server %q", profileName, ref.ServerName)
			}
			if !server.Enabled {
				return nil, fmt.Errorf("MCP profile %q references disabled server %q", profileName, ref.ServerName)
			}
			if !models.MCPAllowedInScope(server.AllowedScopes, r.scope) {
				return nil, fmt.Errorf("MCP server %q is not allowed in scope %q", ref.ServerName, r.scope)
			}
			if mcpProfileRefSelectsAllTools(ref) {
				availableTools := make([]models.MCPTool, 0, len(toolsByServer[ref.ServerName]))
				for _, tool := range toolsByServer[ref.ServerName] {
					availableTools = append(availableTools, tool)
				}
				if len(availableTools) == 0 {
					return nil, fmt.Errorf("MCP profile %q wildcard has no discovered tools for server %q", profileName, ref.ServerName)
				}
				sort.SliceStable(availableTools, func(i, j int) bool {
					return availableTools[i].Name < availableTools[j].Name
				})
				for _, tool := range availableTools {
					spec := MCPToolSpec{
						Server:      ref.ServerName,
						Name:        tool.Name,
						Description: tool.Description,
						InputSchema: tool.InputSchema,
					}
					key := mcpToolKey(spec.Server, spec.Name)
					if _, exists := runtime.allowed[key]; !exists {
						runtime.tools = append(runtime.tools, spec)
						runtime.allowed[key] = spec
					}
				}
				continue
			}
			for _, toolName := range ref.Tools {
				tool, ok := toolsByServer[ref.ServerName][toolName]
				if !ok {
					tool = models.MCPTool{
						ServerName:  ref.ServerName,
						Name:        strings.TrimSpace(toolName),
						InputSchema: "{}",
					}
				}
				spec := MCPToolSpec{
					Server:      ref.ServerName,
					Name:        tool.Name,
					Description: tool.Description,
					InputSchema: tool.InputSchema,
				}
				key := mcpToolKey(spec.Server, spec.Name)
				if _, exists := runtime.allowed[key]; !exists {
					runtime.tools = append(runtime.tools, spec)
					runtime.allowed[key] = spec
				}
			}
		}
	}
	sort.SliceStable(runtime.tools, func(i, j int) bool {
		if runtime.tools[i].Server == runtime.tools[j].Server {
			return runtime.tools[i].Name < runtime.tools[j].Name
		}
		return runtime.tools[i].Server < runtime.tools[j].Server
	})
	return runtime, nil
}

func (r *MCPProfileRegistry) clientFor(ctx context.Context, serverName string) (*mcpclient.Client, error) {
	serverName = strings.TrimSpace(serverName)
	if serverName == "" {
		return nil, fmt.Errorf("MCP server is required")
	}
	r.mu.Lock()
	if client := r.clients[serverName]; client != nil {
		r.mu.Unlock()
		return client, nil
	}
	server, ok := r.servers[serverName]
	if !ok {
		r.mu.Unlock()
		return nil, fmt.Errorf("MCP server %q is not available to this run", serverName)
	}
	client, err := mcpclient.New(server.MCPServer, mcpclient.WithAuthValue(server.AuthValue))
	if err != nil {
		r.mu.Unlock()
		return nil, err
	}
	r.clients[serverName] = client
	r.mu.Unlock()

	if _, err := client.Initialize(ctx); err != nil {
		return nil, err
	}
	return client, nil
}

func (t *MCPTaskRuntime) Enabled() bool {
	return t != nil && len(t.tools) > 0
}

func (t *MCPTaskRuntime) Profiles() []string {
	if t == nil {
		return nil
	}
	return append([]string(nil), t.profiles...)
}

func (t *MCPTaskRuntime) ToolCount() int {
	if t == nil {
		return 0
	}
	return len(t.tools)
}

func (t *MCPTaskRuntime) RequiresToolCall() bool {
	return t != nil && t.Enabled() && t.requireToolCall
}

func (t *MCPTaskRuntime) SuccessfulToolCalls() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.successfulToolCalls
}

func (t *MCPTaskRuntime) ToolTranscript() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.successfulToolTranscript
}

func (t *MCPTaskRuntime) ToolPrompt() string {
	if t == nil || len(t.tools) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("**External MCP Tools:**\n")
	builder.WriteString("You may call one of these MCP tools before choosing a final Nopsai action. To call a tool, respond with JSON like ")
	builder.WriteString(`{"action":{"type":"CALL_MCP_TOOL","mcp_tool_action":{"server":"github","tool":"tool_name","arguments":{}}}}`)
	builder.WriteString(". After a tool result is returned in the history, either call another MCP tool or choose EXECUTE_COMMAND, REPLACE_FILE, or RETURN_ANSWER.\n")
	builder.WriteString("Do not use EXECUTE_COMMAND to bypass unavailable hosted-service MCP tools or permissions. If the approved MCP tools cannot satisfy the requested external data, choose RETURN_ANSWER with a clear missing-capability reason; the agent will fail the task with that reason.\n")
	if t.RequiresToolCall() {
		if successfulToolCalls := t.SuccessfulToolCalls(); successfulToolCalls > 0 {
			builder.WriteString(fmt.Sprintf("This goal has already completed %d successful MCP tool call(s), so use the MCP result already present in the history and choose the next best action.\n", successfulToolCalls))
		} else {
			builder.WriteString("This goal has MCP profiles, so your first action must be CALL_MCP_TOOL.\n")
		}
	}
	builder.WriteString("Use the server and tool fields exactly as listed; when a server is set, the tool field should normally contain only the tool name.\n")
	for _, tool := range t.tools {
		builder.WriteString(fmt.Sprintf("- server=%s tool=%s", tool.Server, tool.Name))
		if strings.TrimSpace(tool.Description) != "" {
			builder.WriteString(fmt.Sprintf(": %s", strings.TrimSpace(tool.Description)))
		}
		if strings.TrimSpace(tool.InputSchema) != "" && strings.TrimSpace(tool.InputSchema) != "{}" {
			builder.WriteString(fmt.Sprintf("\n  input_schema: %s", strings.TrimSpace(tool.InputSchema)))
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func (t *MCPTaskRuntime) CallTool(ctx context.Context, serverName, toolName string, arguments json.RawMessage) (json.RawMessage, error) {
	if t == nil || t.registry == nil {
		return nil, fmt.Errorf("MCP tools are not available")
	}
	serverName = strings.TrimSpace(serverName)
	toolName = strings.TrimSpace(toolName)

	candidates := []MCPToolSpec{{Server: serverName, Name: toolName}}
	normalizedServer, normalizedTool := normalizeMCPToolSelection(serverName, toolName)
	if normalizedServer != serverName || normalizedTool != toolName {
		candidates = append(candidates, MCPToolSpec{Server: normalizedServer, Name: normalizedTool})
	}
	for _, candidate := range candidates {
		key := mcpToolKey(candidate.Server, candidate.Name)
		if _, ok := t.allowed[key]; !ok {
			continue
		}
		client, err := t.registry.clientFor(ctx, candidate.Server)
		if err != nil {
			return nil, err
		}
		result, err := client.CallTool(ctx, candidate.Name, arguments)
		if err != nil {
			return nil, err
		}
		t.recordSuccessfulToolCall(candidate.Server, candidate.Name, arguments, result)
		return result, nil
	}

	return nil, fmt.Errorf("MCP tool %q on server %q is not allowed for this task", toolName, serverName)
}

func (t *MCPTaskRuntime) recordSuccessfulToolCall(serverName, toolName string, arguments json.RawMessage, result json.RawMessage) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.successfulToolCalls++
	t.successfulToolTranscript += formatMCPToolResultTranscript(serverName, toolName, arguments, result)
}

func formatMCPToolResultTranscript(serverName, toolName string, arguments json.RawMessage, result json.RawMessage) string {
	return fmt.Sprintf(
		"\nMCP tool result: server=%s tool=%s arguments=%s result=%s\n",
		serverName,
		toolName,
		mcpclient.JSONString(arguments, 4096),
		mcpclient.JSONString(result, 24000),
	)
}

func normalizeMCPToolSelection(serverName, toolName string) (string, string) {
	serverName = strings.TrimSpace(serverName)
	toolName = strings.TrimSpace(toolName)
	for _, sep := range []string{".", "/", ":"} {
		if serverName == "" {
			if before, after, ok := strings.Cut(toolName, sep); ok {
				return strings.TrimSpace(before), strings.TrimSpace(after)
			}
			continue
		}
		prefix := serverName + sep
		if strings.HasPrefix(toolName, prefix) {
			return serverName, strings.TrimSpace(strings.TrimPrefix(toolName, prefix))
		}
	}
	return serverName, toolName
}

func mcpToolKey(serverName, toolName string) string {
	return strings.TrimSpace(serverName) + "/" + strings.TrimSpace(toolName)
}

func isMCPAllToolsSelector(toolName string) bool {
	return strings.TrimSpace(toolName) == "*"
}

func mcpProfileRefSelectsAllTools(ref models.MCPProfileServerRef) bool {
	for _, toolName := range ref.Tools {
		if isMCPAllToolsSelector(toolName) {
			return true
		}
	}
	return false
}

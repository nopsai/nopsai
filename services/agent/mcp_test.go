package agent

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestMCPProfileRegistryAllowsManualToolFallbacks(t *testing.T) {
	payload := agentRuntimeMCPRegistryPayload{
		Servers: map[string]agentRuntimeMCPServer{
			"github": {
				MCPServer: models.MCPServer{
					Name:      "github",
					Enabled:   true,
					Transport: models.MCPTransportStreamableHTTP,
					URL:       "https://provider.example.com/mcp",
					AuthType:  models.MCPAuthNone,
					Timeout:   models.DefaultMCPTimeout,
				},
			},
		},
		Profiles: map[string]models.MCPProfile{
			"github-readonly": {
				Name:    "github-readonly",
				Enabled: true,
				ServerRefs: []models.MCPProfileServerRef{
					{ServerName: "github", Tools: []string{"actions_list"}},
				},
			},
		},
		Tools: map[string][]models.MCPTool{
			"github": {
				{ServerName: "github", Name: "get_file_contents", InputSchema: "{}"},
			},
		},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(mcpRegistryRuntimeEnv, base64.StdEncoding.EncodeToString(payloadBytes))

	registry, err := NewMCPProfileRegistryFromEnv("dev")
	if err != nil {
		t.Fatalf("NewMCPProfileRegistryFromEnv() error = %v", err)
	}
	runtime, err := registry.ResolveFor(&models.Pipeline{MCPProfiles: []string{"github-readonly"}}, nil, nil)
	if err != nil {
		t.Fatalf("ResolveFor() error = %v", err)
	}
	if len(runtime.tools) != 1 {
		t.Fatalf("runtime tools = %#v, want one manual tool", runtime.tools)
	}
	if !runtime.RequiresToolCall() {
		t.Fatalf("pipeline-level MCP profile should force an MCP tool call")
	}
	if got := runtime.tools[0]; got.Server != "github" || got.Name != "actions_list" || got.InputSchema != "{}" {
		t.Fatalf("runtime tool = %#v, want github.actions_list with empty schema", got)
	}
	if prompt := runtime.ToolPrompt(); !strings.Contains(prompt, "server=github tool=actions_list") {
		t.Fatalf("ToolPrompt() = %q, want manual tool name", prompt)
	}
}

func TestMCPProfileRegistryWildcardUsesDiscoveredTools(t *testing.T) {
	payload := agentRuntimeMCPRegistryPayload{
		Servers: map[string]agentRuntimeMCPServer{
			"github": {
				MCPServer: models.MCPServer{
					Name:      "github",
					Enabled:   true,
					Transport: models.MCPTransportStreamableHTTP,
					URL:       "https://api.githubcopilot.com/mcp/x/all/readonly",
					AuthType:  models.MCPAuthNone,
					Timeout:   models.DefaultMCPTimeout,
				},
			},
		},
		Profiles: map[string]models.MCPProfile{
			"github-readonly": {
				Name:    "github-readonly",
				Enabled: true,
				ServerRefs: []models.MCPProfileServerRef{
					{ServerName: "github", Tools: []string{"*"}},
				},
			},
		},
		Tools: map[string][]models.MCPTool{
			"github": {
				{ServerName: "github", Name: "issues_list", Description: "List issues", InputSchema: `{"type":"object"}`},
				{ServerName: "github", Name: "repos_get", Description: "Get repository", InputSchema: `{"type":"object"}`},
			},
		},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(mcpRegistryRuntimeEnv, base64.StdEncoding.EncodeToString(payloadBytes))

	registry, err := NewMCPProfileRegistryFromEnv("dev")
	if err != nil {
		t.Fatalf("NewMCPProfileRegistryFromEnv() error = %v", err)
	}
	runtime, err := registry.ResolveFor(&models.Pipeline{MCPProfiles: []string{"github-readonly"}}, nil, nil)
	if err != nil {
		t.Fatalf("ResolveFor() error = %v", err)
	}
	if len(runtime.tools) != 2 {
		t.Fatalf("runtime tools = %#v, want all discovered tools", runtime.tools)
	}
	if prompt := runtime.ToolPrompt(); !strings.Contains(prompt, "server=github tool=issues_list") || !strings.Contains(prompt, "server=github tool=repos_get") {
		t.Fatalf("ToolPrompt() = %q, want discovered wildcard tools", prompt)
	}
}

func TestMCPProfileRegistryRequiresToolCallForResolvedProfiles(t *testing.T) {
	payload := agentRuntimeMCPRegistryPayload{
		Servers: map[string]agentRuntimeMCPServer{
			"github": {
				MCPServer: models.MCPServer{
					Name:      "github",
					Enabled:   true,
					Transport: models.MCPTransportStreamableHTTP,
					URL:       "https://api.githubcopilot.com/mcp/x/all/readonly",
					AuthType:  models.MCPAuthNone,
					Timeout:   models.DefaultMCPTimeout,
				},
			},
		},
		Profiles: map[string]models.MCPProfile{
			"github-readonly": {
				Name:    "github-readonly",
				Enabled: true,
				ServerRefs: []models.MCPProfileServerRef{
					{ServerName: "github", Tools: []string{"issues_list"}},
				},
			},
		},
		Tools: map[string][]models.MCPTool{
			"github": {
				{ServerName: "github", Name: "issues_list", Description: "List issues", InputSchema: `{"type":"object"}`},
			},
		},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(mcpRegistryRuntimeEnv, base64.StdEncoding.EncodeToString(payloadBytes))

	registry, err := NewMCPProfileRegistryFromEnv("dev")
	if err != nil {
		t.Fatalf("NewMCPProfileRegistryFromEnv() error = %v", err)
	}
	step := &models.PipelineStep{Step: &models.GoalStep{
		BaseStep: models.BaseStep{MCPProfiles: []string{"github-readonly"}},
		Goal:     "Summarize repository metadata",
	}}
	runtime, err := registry.ResolveFor(&models.Pipeline{}, step, nil)
	if err != nil {
		t.Fatalf("ResolveFor() error = %v", err)
	}
	if !runtime.RequiresToolCall() {
		t.Fatalf("step-level MCP profile should force an MCP tool call")
	}
	if prompt := runtime.ToolPrompt(); !strings.Contains(prompt, "your first action must be CALL_MCP_TOOL") {
		t.Fatalf("ToolPrompt() = %q, want required tool call instruction", prompt)
	}
}

func TestMCPToolSelectionNormalizesServerPrefixedToolNames(t *testing.T) {
	tests := []struct {
		name       string
		server     string
		tool       string
		wantServer string
		wantTool   string
	}{
		{name: "dotted tool with server", server: "github", tool: "github.search_repositories", wantServer: "github", wantTool: "search_repositories"},
		{name: "dotted tool without server", tool: "github.search_repositories", wantServer: "github", wantTool: "search_repositories"},
		{name: "slash tool with server", server: "github", tool: "github/search_repositories", wantServer: "github", wantTool: "search_repositories"},
		{name: "plain tool", server: "github", tool: "search_repositories", wantServer: "github", wantTool: "search_repositories"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotServer, gotTool := normalizeMCPToolSelection(tt.server, tt.tool)
			if gotServer != tt.wantServer || gotTool != tt.wantTool {
				t.Fatalf("normalizeMCPToolSelection() = %q, %q; want %q, %q", gotServer, gotTool, tt.wantServer, tt.wantTool)
			}
		})
	}
}

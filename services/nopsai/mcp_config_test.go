package nopsai

import (
	"strings"
	"testing"
	"time"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/mcpregistry"
)

func TestParseGitOpsMCPRegistryPlanFromSettingDirectory(t *testing.T) {
	plan, err := mcpregistry.ParseGitOpsPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		mcpregistry.GitOpsDirectory{
			Root: "setting",
			Files: map[string]string{
				"setting/system/mcp.yaml": `
mcp_servers:
  github:
    display_name: GitHub MCP
    enabled: true
    provider: github
    transport: streamable_http
    url: https://provider.example.com/mcp
    auth_type: bearer_token
    credential_ref: credential://system/mcp/github
    headers:
      X-MCP-Toolsets: default,actions
      X-MCP-Readonly: "true"
    timeout: 30s
    allowed_scopes: ["dev"]

mcp_profiles:
  github-pr-review:
    description: Read-only GitHub PR review tools
    enabled: true
    servers:
      - server: github
        tools:
          - get_file
          - list_pull_request_files
          - search_code
    allowed_scopes: ["dev"]
`,
			},
		},
	)
	if err != nil {
		t.Fatalf("mcpregistry.ParseGitOpsPlan() error = %v", err)
	}
	if plan == nil {
		t.Fatal("expected GitOps MCP registry plan")
	}
	if plan.SourcePath != "setting/system/mcp.yaml" {
		t.Fatalf("sourcePath = %q", plan.SourcePath)
	}
	if got := plan.Servers["github"].AuthType; got != models.MCPAuthBearerToken {
		t.Fatalf("github auth type = %q, want bearer_token", got)
	}
	if got := plan.Servers["github"].Headers["X-MCP-Toolsets"]; got != "default,actions" {
		t.Fatalf("github X-MCP-Toolsets header = %q, want default,actions", got)
	}
	profile := plan.Profiles["github-pr-review"]
	if !profile.Enabled {
		t.Fatal("github-pr-review should be enabled")
	}
	if len(profile.ServerRefs) != 1 || profile.ServerRefs[0].ServerName != "github" {
		t.Fatalf("profile server refs = %#v", profile.ServerRefs)
	}
	if got := strings.Join(profile.ServerRefs[0].Tools, ","); got != "get_file,list_pull_request_files,search_code" {
		t.Fatalf("profile tools = %q", got)
	}
}

func TestParseGitOpsMCPRegistryPlanRejectsTeamScopedRepo(t *testing.T) {
	_, err := mcpregistry.ParseGitOpsPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"},
		mcpregistry.GitOpsDirectory{
			Root: "setting",
			Files: map[string]string{
				"setting/system/mcp.yaml": `
mcp_servers:
  github:
    enabled: true
    provider: github
    transport: streamable_http
    url: https://provider.example.com/mcp
`,
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "system config repository") {
		t.Fatalf("expected system-scope error, got %v", err)
	}
}

func TestParseGitOpsMCPRegistryFileRejectsUnknownServer(t *testing.T) {
	_, err := mcpregistry.ParseGitOpsFile(`
mcp_profiles:
  github-pr-review:
    enabled: true
    servers:
      - server: github
        tools: [get_file]
`, "setting/system/mcp.yaml")
	if err == nil || !strings.Contains(err.Error(), `unknown server "github"`) {
		t.Fatalf("expected unknown server error, got %v", err)
	}
}

func TestParseGitOpsMCPRegistryFileRejectsWriteLikeTools(t *testing.T) {
	_, err := mcpregistry.ParseGitOpsFile(`
mcp_servers:
  github:
    enabled: true
    provider: github
    transport: streamable_http
    url: https://provider.example.com/mcp
mcp_profiles:
  github-writes:
    enabled: true
    servers:
      - server: github
        tools: [create_issue]
`, "setting/system/mcp.yaml")
	if err == nil || !strings.Contains(err.Error(), "write-like tool") {
		t.Fatalf("expected write-like tool error, got %v", err)
	}
}

func TestValidateMCPProfileDefinitionAllowsManuallyConfiguredTools(t *testing.T) {
	err := mcpregistry.ValidateProfileDefinition(
		models.MCPProfile{
			Name:    "github-readonly",
			Enabled: true,
			ServerRefs: []models.MCPProfileServerRef{
				{ServerName: "github", Tools: []string{"actions_list"}},
			},
		},
		map[string]models.MCPServer{
			"github": {
				Name:      "github",
				Enabled:   true,
				Transport: models.MCPTransportStreamableHTTP,
				URL:       "https://provider.example.com/mcp",
				AuthType:  models.MCPAuthNone,
				Timeout:   models.DefaultMCPTimeout,
			},
		},
		map[string][]models.MCPTool{
			"github": {
				{ServerName: "github", Name: "get_file_contents", InputSchema: "{}"},
			},
		},
	)
	if err != nil {
		t.Fatalf("mcpregistry.ValidateProfileDefinition() error = %v", err)
	}
}

func TestValidateMCPProfileDefinitionAllowsWildcardForReadonlyServer(t *testing.T) {
	err := mcpregistry.ValidateProfileDefinition(
		models.MCPProfile{
			Name:    "github-readonly",
			Enabled: true,
			ServerRefs: []models.MCPProfileServerRef{
				{ServerName: "github", Tools: []string{"*"}},
			},
		},
		map[string]models.MCPServer{
			"github": {
				Name:      "github",
				Enabled:   true,
				Transport: models.MCPTransportStreamableHTTP,
				URL:       "https://api.githubcopilot.com/mcp/x/all/readonly",
				AuthType:  models.MCPAuthNone,
				Timeout:   models.DefaultMCPTimeout,
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("mcpregistry.ValidateProfileDefinition() error = %v", err)
	}
}

func TestValidateMCPProfileDefinitionRejectsWildcardForNonReadonlyServer(t *testing.T) {
	err := mcpregistry.ValidateProfileDefinition(
		models.MCPProfile{
			Name:    "github-all",
			Enabled: true,
			ServerRefs: []models.MCPProfileServerRef{
				{ServerName: "github", Tools: []string{"*"}},
			},
		},
		map[string]models.MCPServer{
			"github": {
				Name:      "github",
				Enabled:   true,
				Transport: models.MCPTransportStreamableHTTP,
				URL:       "https://api.githubcopilot.com/mcp",
				AuthType:  models.MCPAuthNone,
				Timeout:   models.DefaultMCPTimeout,
			},
		},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "wildcard tools") {
		t.Fatalf("expected wildcard read-only error, got %v", err)
	}
}

func TestSelectMCPToolsUsesDiscoveredMetadataAndManualFallbacks(t *testing.T) {
	got := mcpregistry.SelectTools("github", []models.MCPTool{
		{
			ServerName:  "github",
			Name:        "get_file_contents",
			Description: "Read a file",
			InputSchema: `{"type":"object"}`,
			SchemaHash:  "hash",
			LastSeenAt:  nowForMCPTest(),
		},
	}, []string{"actions_list", "get_file_contents"})

	if len(got) != 2 {
		t.Fatalf("selected tool count = %d, want 2: %#v", len(got), got)
	}
	if got[0].Name != "actions_list" || got[0].InputSchema != "{}" {
		t.Fatalf("manual fallback = %#v, want actions_list with empty schema", got[0])
	}
	if got[1].Name != "get_file_contents" || got[1].Description != "Read a file" {
		t.Fatalf("discovered tool = %#v, want discovered metadata", got[1])
	}
}

func TestSelectMCPToolsWildcardUsesAllDiscoveredTools(t *testing.T) {
	got := mcpregistry.SelectTools("github", []models.MCPTool{
		{ServerName: "github", Name: "issues_list", Description: "List issues", InputSchema: `{"type":"object"}`, LastSeenAt: nowForMCPTest()},
		{ServerName: "github", Name: "repos_get", Description: "Get repository", InputSchema: `{"type":"object"}`, LastSeenAt: nowForMCPTest()},
	}, []string{"*"})

	if len(got) != 2 {
		t.Fatalf("selected tool count = %d, want 2: %#v", len(got), got)
	}
	if got[0].Name != "issues_list" || got[1].Name != "repos_get" {
		t.Fatalf("selected tools = %#v", got)
	}
}

func nowForMCPTest() time.Time {
	return time.Unix(1700000000, 0).UTC()
}

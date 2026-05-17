package main

import (
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestParseGitOpsMCPRegistryPlanFromSettingDirectory(t *testing.T) {
	plan, err := parseGitOpsMCPRegistryPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		gitOpsMCPDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/mcp.yaml": `
mcp_servers:
  github:
    display_name: GitHub MCP
    enabled: true
    provider: github
    transport: streamable_http
    url: https://provider.example.com/mcp
    auth_type: bearer_token
    auth_secret: GITHUB_MCP_TOKEN
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
		t.Fatalf("parseGitOpsMCPRegistryPlan() error = %v", err)
	}
	if plan == nil {
		t.Fatal("expected GitOps MCP registry plan")
	}
	if plan.sourcePath != "setting/system/mcp.yaml" {
		t.Fatalf("sourcePath = %q", plan.sourcePath)
	}
	if got := plan.servers["github"].AuthType; got != models.MCPAuthBearerToken {
		t.Fatalf("github auth type = %q, want bearer_token", got)
	}
	profile := plan.profiles["github-pr-review"]
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

func TestParseGitOpsMCPRegistryPlanRejectsGroupScopedRepo(t *testing.T) {
	_, err := parseGitOpsMCPRegistryPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"},
		gitOpsMCPDirectory{
			root: "setting",
			files: map[string]string{
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
	_, err := parseGitOpsMCPRegistryFile(`
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
	_, err := parseGitOpsMCPRegistryFile(`
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

package nopsai

import (
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func systemRegistryRepo() models.ConfigRepository {
	return models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
}

func TestParseGitOpsModelsReadsOneFilePerModel(t *testing.T) {
	plan, err := parseGitOpsModels(
		map[string]string{
			"models/gemini-fast.yaml": `
default: true
provider: gemini
model: gemini-2.5-flash
credential_ref: credential://system/llm/gemini
allowed_scopes: ["dev", "prod"]
pricing:
  input_per_million_usd: 0.3
  output_per_million_usd: 2.5
`,

			"models/notes.md": "ignored",
		},
		"models",
		systemRegistryRepo(),
	)
	if err != nil {
		t.Fatalf("parseGitOpsModels() error = %v", err)
	}
	defaultModel, registryModels := plan.registryModels()
	if defaultModel != "gemini-fast" {
		t.Fatalf("default model = %q, want gemini-fast", defaultModel)
	}
	if len(registryModels) != 1 {
		t.Fatalf("global models = %#v, want only gemini-fast", registryModels)
	}
	if registryModels["gemini-fast"].Model != "gemini-2.5-flash" {
		t.Fatalf("model = %#v", registryModels["gemini-fast"])
	}
}

func TestParseGitOpsModelsKeepsTeamQualifiedWorkspaceNames(t *testing.T) {
	plan, err := parseGitOpsModels(
		map[string]string{"models/team-2/shared.yaml": "provider: gemini\nmodel: a\ncredential_ref: credential://system/llm/gemini\n"},
		"models",
		systemRegistryRepo(),
	)
	if err != nil {
		t.Fatalf("parseGitOpsModels() error = %v", err)
	}
	_, registryModels := plan.registryModels()
	if _, ok := registryModels["team-2/shared"]; !ok {
		t.Fatalf("global models = %#v, want the team-qualified registry name", registryModels)
	}
}

func TestParseGitOpsModelsRejectsTwoDefaultsInOneScope(t *testing.T) {
	_, err := parseGitOpsModels(
		map[string]string{
			"models/one.yaml": "default: true\nprovider: gemini\nmodel: a\ncredential_ref: credential://system/llm/gemini\npricing:\n  input_per_million_usd: 0.3\n  output_per_million_usd: 2.5\n",
			"models/two.yaml": "default: true\nprovider: gemini\nmodel: b\ncredential_ref: credential://system/llm/gemini\npricing:\n  input_per_million_usd: 0.3\n  output_per_million_usd: 2.5\n",
		},
		"models",
		systemRegistryRepo(),
	)
	if err == nil || !strings.Contains(err.Error(), "set default: true") {
		t.Fatalf("error = %v, want a single-default error", err)
	}
}

func TestParseGitOpsModelsRejectsTeamRepositoryRegistryFiles(t *testing.T) {
	_, err := parseGitOpsModels(
		map[string]string{"models/team-tuned.yaml": "provider: gemini\nmodel: a\ncredential_ref: credential://system/llm/gemini\npricing:\n  input_per_million_usd: 0.3\n  output_per_million_usd: 2.5\n"},
		"models",
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "platform"},
	)
	if err == nil || !strings.Contains(err.Error(), "system config repository") {
		t.Fatalf("error = %v, want a system-repository-only error", err)
	}
}

func TestParseGitOpsMCPRegistryReadsServersAndProfiles(t *testing.T) {
	plan, err := parseGitOpsMCPRegistry(
		map[string]string{
			"mcp/servers/github.yaml": `
display_name: GitHub MCP
enabled: true
provider: github
transport: streamable_http
url: https://api.githubcopilot.com/mcp/x/all/readonly
auth_type: bearer_token
credential_ref: credential://system/mcp/github-readonly
allowed_scopes: ["dev"]
`,
			"mcp/profiles/github-readonly.yaml": `
description: Read-only GitHub tools.
enabled: true
servers:
  - server: github
    tools: ["*"]
allowed_scopes: ["dev"]
`,
		},
		"mcp/servers",
		"mcp/profiles",
		systemRegistryRepo(),
	)
	if err != nil {
		t.Fatalf("parseGitOpsMCPRegistry() error = %v", err)
	}
	if len(plan.registryServers()) != 1 || len(plan.registryProfiles()) != 1 {
		t.Fatalf("registry = %#v / %#v", plan.registryServers(), plan.registryProfiles())
	}
	if _, ok := plan.registryProfiles()["github-readonly"]; !ok {
		t.Fatalf("profiles = %#v, want the workspace profile", plan.registryProfiles())
	}
}

func TestParseGitOpsMCPRegistryKeepsTeamQualifiedServerNames(t *testing.T) {
	plan, err := parseGitOpsMCPRegistry(
		map[string]string{"mcp/servers/team-1/platform/github.yaml": "enabled: true\ntransport: streamable_http\nurl: https://example.com\nauth_type: none\ntimeout: 30s\n"},
		"mcp/servers",
		"mcp/profiles",
		systemRegistryRepo(),
	)
	if err != nil {
		t.Fatalf("parseGitOpsMCPRegistry() error = %v", err)
	}
	if _, ok := plan.registryServers()["team-1/platform/github"]; !ok {
		t.Fatalf("servers = %#v, want the team-qualified server name", plan.registryServers())
	}
}

func TestParseGitOpsMCPRegistryRejectsProfilesForUnknownServers(t *testing.T) {
	_, err := parseGitOpsMCPRegistry(
		map[string]string{
			"mcp/profiles/team-tools.yaml": "enabled: true\nservers:\n  - server: missing\n    tools: [\"*\"]\n",
		},
		"mcp/servers",
		"mcp/profiles",
		systemRegistryRepo(),
	)
	if err == nil || !strings.Contains(err.Error(), "unknown") || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error = %v, want an unknown-server error naming the missing server", err)
	}
}

func TestRegistryGitOpsExportPathsMatchTheParsedLayout(t *testing.T) {
	repo := systemRegistryRepo()
	for _, tc := range []struct {
		directory string
		name      string
		want      string
	}{
		{directory: modelsGitOpsDirectory, name: "gemini-fast", want: "models/gemini-fast.yaml"},
		{directory: modelsGitOpsDirectory, name: "platform/team-tuned", want: "models/platform/team-tuned.yaml"},
		{directory: agentRolesGitOpsDirectory, name: "release-manager", want: "agent-roles/release-manager.yaml"},
		{directory: mcpServersGitOpsDirectory, name: "github", want: "mcp/servers/github.yaml"},
		{directory: mcpProfilesGitOpsDirectory, name: "platform/team-tools", want: "mcp/profiles/platform/team-tools.yaml"},
	} {
		path, ok := registryGitOpsExportPath(repo, tc.directory, tc.name)
		if !ok || path != tc.want {
			t.Fatalf("export path = %q/%v, want %q", path, ok, tc.want)
		}
		if !isConfigRepositoryDriftPath(path) {
			t.Fatalf("export path %q is not tracked by drift", path)
		}
	}
}

func TestBuildModelGitOpsFileRoundTripsThroughTheParser(t *testing.T) {
	plan, err := parseGitOpsModels(
		map[string]string{"models/gemini-fast.yaml": "default: true\nprovider: gemini\nmodel: gemini-2.5-flash\ncredential_ref: credential://system/llm/gemini\npricing:\n  input_per_million_usd: 0.3\n  output_per_million_usd: 2.5\n"},
		"models",
		systemRegistryRepo(),
	)
	if err != nil {
		t.Fatalf("parseGitOpsModels() error = %v", err)
	}
	_, registryModels := plan.registryModels()
	content, err := marshalConfigRepositoryYAML(buildModelGitOpsFile("gemini-fast", registryModels["gemini-fast"], true))
	if err != nil {
		t.Fatalf("marshalConfigRepositoryYAML() error = %v", err)
	}
	if !strings.Contains(string(content), "default: true") {
		t.Fatalf("exported model should record the default:\n%s", content)
	}
	reparsed, err := parseGitOpsModels(
		map[string]string{"models/gemini-fast.yaml": string(content)},
		"models",
		systemRegistryRepo(),
	)
	if err != nil {
		t.Fatalf("exported model does not round-trip: %v", err)
	}
	if defaultModel, _ := reparsed.registryModels(); defaultModel != "gemini-fast" {
		t.Fatalf("round-tripped default = %q", defaultModel)
	}
}

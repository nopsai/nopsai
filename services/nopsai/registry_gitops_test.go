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
`,
			"models/platform/team-tuned.yaml": `
provider: gemini
model: gemini-2.5-pro
credential_ref: credential://system/llm/gemini
`,
			"models/notes.md": "ignored",
		},
		"models",
		systemRegistryRepo(),
		"",
	)
	if err != nil {
		t.Fatalf("parseGitOpsModels() error = %v", err)
	}
	defaultModel, globalModels := plan.globalModels()
	if defaultModel != "gemini-fast" {
		t.Fatalf("default model = %q, want gemini-fast", defaultModel)
	}
	if len(globalModels) != 1 {
		t.Fatalf("global models = %#v, want only gemini-fast", globalModels)
	}
	if globalModels["gemini-fast"].Model != "gemini-2.5-flash" {
		t.Fatalf("model = %#v", globalModels["gemini-fast"])
	}
	teamModels := plan.teamModels()
	stored, ok := teamModels["platform"]["team-tuned"]
	if !ok {
		t.Fatalf("team models = %#v, want the platform model", teamModels)
	}
	if stored.sourcePath != "models/platform/team-tuned.yaml" {
		t.Fatalf("source path = %q", stored.sourcePath)
	}
}

func TestParseGitOpsModelsRejectsTwoDefaultsInOneScope(t *testing.T) {
	_, err := parseGitOpsModels(
		map[string]string{
			"models/one.yaml": "default: true\nprovider: gemini\nmodel: a\ncredential_ref: credential://system/llm/gemini\n",
			"models/two.yaml": "default: true\nprovider: gemini\nmodel: b\ncredential_ref: credential://system/llm/gemini\n",
		},
		"models",
		systemRegistryRepo(),
		"",
	)
	if err == nil || !strings.Contains(err.Error(), "set default: true") {
		t.Fatalf("error = %v, want a single-default error", err)
	}
}

func TestParseGitOpsModelsAllowsOneDefaultPerScope(t *testing.T) {
	plan, err := parseGitOpsModels(
		map[string]string{
			"models/global-default.yaml":      "default: true\nprovider: gemini\nmodel: a\ncredential_ref: credential://system/llm/gemini\n",
			"models/platform/team-model.yaml": "default: true\nprovider: gemini\nmodel: b\ncredential_ref: credential://system/llm/gemini\n",
		},
		"models",
		systemRegistryRepo(),
		"",
	)
	if err != nil {
		t.Fatalf("parseGitOpsModels() error = %v", err)
	}
	if plan.defaults[""] != "global-default" || plan.defaults["platform"] != "team-model" {
		t.Fatalf("defaults = %#v, want one default per scope", plan.defaults)
	}
}

func TestParseGitOpsModelsNormalizesTeamRepositoryPaths(t *testing.T) {
	plan, err := parseGitOpsModels(
		map[string]string{"models/team-tuned.yaml": "provider: gemini\nmodel: a\ncredential_ref: credential://system/llm/gemini\n"},
		"models",
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "platform"},
		"platform",
	)
	if err != nil {
		t.Fatalf("parseGitOpsModels() error = %v", err)
	}
	if _, ok := plan.teamModels()["platform"]["team-tuned"]; !ok {
		t.Fatalf("team models = %#v, want the bound team prefix", plan.teamModels())
	}
	if _, models := plan.globalModels(); len(models) != 0 {
		t.Fatalf("global models = %#v, want none from a team repository", models)
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
			"mcp/profiles/platform/team-tools.yaml": `
enabled: true
servers:
  - server: github
    tools: ["*"]
`,
		},
		"mcp/servers",
		"mcp/profiles",
		systemRegistryRepo(),
		"",
	)
	if err != nil {
		t.Fatalf("parseGitOpsMCPRegistry() error = %v", err)
	}
	if len(plan.globalServers()) != 1 || len(plan.globalProfiles()) != 1 {
		t.Fatalf("registry = %#v / %#v", plan.globalServers(), plan.globalProfiles())
	}
	if _, ok := plan.teamProfiles()["platform"]["team-tools"]; !ok {
		t.Fatalf("team profiles = %#v, want the platform profile", plan.teamProfiles())
	}
}

func TestParseGitOpsMCPRegistryRejectsTeamScopedServers(t *testing.T) {
	_, err := parseGitOpsMCPRegistry(
		map[string]string{"mcp/servers/platform/github.yaml": "provider: github\ntransport: streamable_http\nurl: https://example.com\n"},
		"mcp/servers",
		"mcp/profiles",
		systemRegistryRepo(),
		"",
	)
	if err == nil || !strings.Contains(err.Error(), "workspace-wide") {
		t.Fatalf("error = %v, want a workspace-only server error", err)
	}
}

func TestParseGitOpsMCPRegistryRejectsProfilesForUnknownServers(t *testing.T) {
	_, err := parseGitOpsMCPRegistry(
		map[string]string{
			"mcp/profiles/platform/team-tools.yaml": "enabled: true\nservers:\n  - server: missing\n    tools: [\"*\"]\n",
		},
		"mcp/servers",
		"mcp/profiles",
		systemRegistryRepo(),
		"",
	)
	if err == nil || !strings.Contains(err.Error(), "unknown MCP server") {
		t.Fatalf("error = %v, want an unknown-server error", err)
	}
}

func TestRegistryGitOpsExportPathsMatchTheParsedLayout(t *testing.T) {
	repo := systemRegistryRepo()
	for _, tc := range []struct {
		directory string
		team      string
		name      string
		want      string
	}{
		{directory: modelsGitOpsDirectory, name: "gemini-fast", want: "models/gemini-fast.yaml"},
		{directory: modelsGitOpsDirectory, team: "platform", name: "team-tuned", want: "models/platform/team-tuned.yaml"},
		{directory: agentRolesGitOpsDirectory, name: "release-manager", want: "agent-roles/release-manager.yaml"},
		{directory: mcpServersGitOpsDirectory, name: "github", want: "mcp/servers/github.yaml"},
		{directory: mcpProfilesGitOpsDirectory, team: "platform", name: "team-tools", want: "mcp/profiles/platform/team-tools.yaml"},
	} {
		path, ok := registryGitOpsExportPath(repo, tc.directory, tc.team, tc.name)
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
		map[string]string{"models/gemini-fast.yaml": "default: true\nprovider: gemini\nmodel: gemini-2.5-flash\ncredential_ref: credential://system/llm/gemini\n"},
		"models",
		systemRegistryRepo(),
		"",
	)
	if err != nil {
		t.Fatalf("parseGitOpsModels() error = %v", err)
	}
	_, globalModels := plan.globalModels()
	content, err := marshalConfigRepositoryYAML(buildModelGitOpsFile("gemini-fast", globalModels["gemini-fast"], true))
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
		"",
	)
	if err != nil {
		t.Fatalf("exported model does not round-trip: %v", err)
	}
	if defaultModel, _ := reparsed.globalModels(); defaultModel != "gemini-fast" {
		t.Fatalf("round-tripped default = %q", defaultModel)
	}
}

func TestRequireSingleTeamDefaultSourceRejectsConflictingDefaults(t *testing.T) {
	teamDefault := "reasoning"
	plan := configSyncPlan{
		modelPlan: &gitOpsModelPlan{
			models:   map[string]storedModel{"platform/team-tuned": {team: "platform", name: "team-tuned"}},
			defaults: map[string]string{"platform": "team-tuned"},
		},
		teamDefaultsPlans: map[string]*gitOpsTeamDefaultsPlan{
			"platform": {
				teamPath:          "platform",
				sourcePath:        "config-repositories/teams/platform/defaults.yaml",
				llmDefaultProfile: &teamDefault,
			},
		},
	}
	err := requireSingleTeamDefaultSource(plan)
	if err == nil || !strings.Contains(err.Error(), "in two places") {
		t.Fatalf("error = %v, want an ambiguous team default error", err)
	}
}

func TestRequireSingleTeamDefaultSourceAllowsAWorkspaceModelAsTeamDefault(t *testing.T) {
	teamDefault := "reasoning"
	plan := configSyncPlan{
		modelPlan: &gitOpsModelPlan{
			models:   map[string]storedModel{"reasoning": {name: "reasoning"}},
			defaults: map[string]string{},
		},
		teamDefaultsPlans: map[string]*gitOpsTeamDefaultsPlan{
			"platform": {
				teamPath:          "platform",
				sourcePath:        "config-repositories/teams/platform/defaults.yaml",
				llmDefaultProfile: &teamDefault,
			},
		},
	}
	if err := requireSingleTeamDefaultSource(plan); err != nil {
		t.Fatalf("naming a workspace model as the team default should be allowed: %v", err)
	}
}

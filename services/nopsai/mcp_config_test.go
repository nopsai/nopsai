package nopsai

import (
	"strings"
	"testing"
	"time"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/mcpregistry"
)

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

func TestValidateMCPDefinitionsAcceptTeamScopedNames(t *testing.T) {
	server := models.MCPServer{
		Name:      "team-1/platform/github",
		Enabled:   true,
		Transport: models.MCPTransportStreamableHTTP,
		URL:       "https://provider.example.com/mcp",
		AuthType:  models.MCPAuthNone,
		Timeout:   models.DefaultMCPTimeout,
	}
	if err := mcpregistry.ValidateServerDefinition(server); err != nil {
		t.Fatalf("mcpregistry.ValidateServerDefinition() error = %v", err)
	}

	err := mcpregistry.ValidateProfileDefinition(
		models.MCPProfile{
			Name:    "team-1/platform/review",
			Enabled: true,
			ServerRefs: []models.MCPProfileServerRef{
				{ServerName: "team-1/platform/github", Tools: []string{"actions_list"}},
			},
		},
		map[string]models.MCPServer{"team-1/platform/github": server},
		map[string][]models.MCPTool{},
	)
	if err != nil {
		t.Fatalf("mcpregistry.ValidateProfileDefinition() error = %v", err)
	}
}

func TestMCPTeamProfileReferenceNameUsesResolvedTeamPath(t *testing.T) {
	records := map[int]teamPathRecord{
		7: {ID: 7, Path: "team-1/dev"},
	}

	got := mcpTeamProfileReferenceName(records, 7, "review")
	if got != "team-1/dev/review" {
		t.Fatalf("mcpTeamProfileReferenceName() = %q, want team-1/dev/review", got)
	}

	got = mcpTeamProfileReferenceName(records, 9, "review")
	if got != "9/review" {
		t.Fatalf("mcpTeamProfileReferenceName() fallback = %q, want 9/review", got)
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

package main

import (
	"context"
	"strings"
	"testing"

	"nopsai/config"
)

func TestNormalizeSetupRepositories(t *testing.T) {
	got := normalizeSetupRepositories([]string{
		" https://github.com/acme/service-api.git ",
		"git@github.com:acme/web.git",
		"acme/service-api",
		"acme/web",
		"invalid",
		"../bad/repo",
	})
	want := []string{"acme/service-api", "acme/web"}
	if len(got) != len(want) {
		t.Fatalf("normalizeSetupRepositories() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalizeSetupRepositories()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNormalizeSetupRepositoryGroups(t *testing.T) {
	groups := normalizeSetupRepositoryGroups([]setupRepositoryGroupInput{
		{Name: " Platform Team ", Repositories: []string{"acme/api", "acme/web"}},
		{Name: "Apps", Repositories: []string{"https://github.com/acme/api.git", "acme/worker"}},
	}, nil)
	if len(groups) != 2 {
		t.Fatalf("groups = %#v, want 2 groups", groups)
	}
	if groups[0].Name != "Platform-Team" || len(groups[0].Repositories) != 2 {
		t.Fatalf("first group = %#v, want Platform-Team with two repos", groups[0])
	}
	if groups[1].Name != "Apps" || len(groups[1].Repositories) != 1 || groups[1].Repositories[0] != "acme/worker" {
		t.Fatalf("second group = %#v, want Apps with deduplicated worker repo", groups[1])
	}
}

func TestSetupStarterTemplatesUseSecretReferencesOnly(t *testing.T) {
	files := setupStarterTemplates(setupProfileProduction, []string{"acme/service-api"})
	if _, ok := files["triggers/acme/service-api.yaml"]; !ok {
		t.Fatal("expected repository trigger template")
	}
	llm := files["setting/system/llm_profile.yaml"]
	if strings.Contains(strings.ToLower(llm), "api_key_value") {
		t.Fatalf("LLM template should not contain secret values:\n%s", llm)
	}
	mcp := files["setting/system/mcp.yaml"]
	if !strings.Contains(mcp, "enabled: false") {
		t.Fatalf("MCP examples should be disabled by default:\n%s", mcp)
	}
}

func TestSetupStarterTemplatesUseSelectedRepositoryGroups(t *testing.T) {
	files := setupStarterTemplatesWithOptions(setupProfileTeam, nil, setupTemplateOptions{
		RepositoryGroups: []setupRepositoryGroupInput{
			{Name: "platform", Repositories: []string{"acme/service-api"}},
			{Name: "apps", Repositories: []string{"acme/web"}},
		},
		IncludeLLM: true,
		IncludeMCP: false,
	})
	structure := files["config-repositories/groups/structure.yaml"]
	for _, want := range []string{"platform:", "- acme/service-api", "apps:", "- acme/web"} {
		if !strings.Contains(structure, want) {
			t.Fatalf("structure missing %q:\n%s", want, structure)
		}
	}
	if _, ok := files["setting/system/mcp.yaml"]; ok {
		t.Fatal("MCP file should be omitted when MCP examples are disabled")
	}
}

func TestSetupStarterTemplatesIncludeSelectedUsersInAccess(t *testing.T) {
	files := setupStarterTemplatesWithOptions(setupProfileTeam, nil, setupTemplateOptions{
		RepositoryGroups: []setupRepositoryGroupInput{
			{Name: "team-1", Repositories: []string{"acme/service-api"}},
		},
		Users: []setupUserInput{
			{Sub: "alice@example.com", Email: "alice@example.com", Role: "developer", Group: "team-1"},
		},
		IncludeLLM: true,
		IncludeMCP: false,
	})
	access := files["access/bootstrap.yaml"]
	for _, want := range []string{`sub: "alice@example.com"`, `email: "alice@example.com"`, `role: developer`, `resource: folder:team-1`} {
		if !strings.Contains(access, want) {
			t.Fatalf("access file missing %q:\n%s", want, access)
		}
	}
	if strings.Contains(access, "password:") {
		t.Fatalf("access file should not write generated passwords:\n%s", access)
	}
}

func TestSetupLLMProfileYAMLGeminiHasProviderSpecificFields(t *testing.T) {
	got := setupLLMProfileYAML(setupLLMProfileInput{
		Provider:     config.LLMProviderGemini,
		Model:        "gemini-2.5-pro",
		APIKeySecret: "MY_GEMINI_KEY",
	})
	for _, want := range []string{"provider: gemini", "model: gemini-2.5-pro", "api_key_secret: MY_GEMINI_KEY"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Gemini template missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "base_url:") {
		t.Fatalf("Gemini template should not contain base_url:\n%s", got)
	}
}

func TestProductionSetupRejectsDirectDatabaseSeed(t *testing.T) {
	app := App{}
	err := app.validateSetupBootstrapRequest(setupBootstrapRequest{
		Profile:                setupProfileProduction,
		ProductionAcknowledged: true,
		SeedStarterDatabase:    true,
		ConfigRepository:       &setupConfigRepositoryInput{RepoURL: "https://github.com/acme/config.git"},
	})
	if err == nil {
		t.Fatal("expected production setup to reject direct database seed")
	}
}

func TestEmptySetupRejectsResourceSeeding(t *testing.T) {
	app := App{}
	err := app.validateSetupBootstrapRequest(setupBootstrapRequest{
		Profile:     setupProfileEmpty,
		MCPExamples: true,
		Users: []setupUserInput{
			{Sub: "alice@example.com", Role: "owner"},
		},
	})
	if err == nil {
		t.Fatal("expected empty setup to reject resource seeding")
	}
}

func TestSetupPreflightBlocksMissingRequiredConfig(t *testing.T) {
	resp := buildSetupPreflightResponse(context.Background(), &config.Config{}, "config.yml", ".env", "preflight_only", nil, nil)
	if resp.Ready || resp.CanLogin {
		t.Fatalf("preflight ready = %v canLogin = %v, want both false", resp.Ready, resp.CanLogin)
	}
	requiredErrors := 0
	for _, check := range resp.Checks {
		if check.Required && check.Status == "error" {
			requiredErrors++
		}
	}
	if requiredErrors < 3 {
		t.Fatalf("required errors = %d, want database, master key, and JWT errors", requiredErrors)
	}
}

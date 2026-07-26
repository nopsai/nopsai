package nopsai

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

func TestNormalizeSetupRepositoryTeams(t *testing.T) {
	teams := normalizeSetupRepositoryTeams([]setupRepositoryTeamInput{
		{Name: " Platform Team ", Repositories: []string{"acme/api", "acme/web"}},
		{Name: "Apps", Repositories: []string{"https://github.com/acme/api.git", "acme/worker"}},
	}, nil)
	if len(teams) != 2 {
		t.Fatalf("teams = %#v, want 2 teams", teams)
	}
	if teams[0].Name != "Platform-Team" || len(teams[0].Repositories) != 2 {
		t.Fatalf("first team = %#v, want Platform-Team with two repos", teams[0])
	}
	if teams[1].Name != "Apps" || len(teams[1].Repositories) != 1 || teams[1].Repositories[0] != "acme/worker" {
		t.Fatalf("second team = %#v, want Apps with deduplicated worker repo", teams[1])
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

func TestSetupStarterTemplatesUseSelectedRepositoryTeams(t *testing.T) {
	files := setupStarterTemplatesWithOptions(setupProfileTeam, nil, setupTemplateOptions{
		RepositoryTeams: []setupRepositoryTeamInput{
			{Name: "platform", Repositories: []string{"acme/service-api"}},
			{Name: "apps", Repositories: []string{"acme/web"}},
		},
		IncludeLLM: true,
		IncludeMCP: false,
	})
	if _, ok := files["config-repositories/teams/structure.yaml"]; ok {
		t.Fatal("starter templates should use scoped team structure files, not the aggregate structure file")
	}
	platformStructure := files["config-repositories/teams/platform/structure.yaml"]
	for _, want := range []string{"description: Repository team", "apps:", "name: service-api", "repo_url: https://github.com/acme/service-api"} {
		if !strings.Contains(platformStructure, want) {
			t.Fatalf("platform structure missing %q:\n%s", want, platformStructure)
		}
	}
	appsStructure := files["config-repositories/teams/apps/structure.yaml"]
	for _, want := range []string{"description: Repository team", "apps:", "name: web", "repo_url: https://github.com/acme/web"} {
		if !strings.Contains(appsStructure, want) {
			t.Fatalf("apps structure missing %q:\n%s", want, appsStructure)
		}
	}
	if _, ok := files["setting/system/mcp.yaml"]; ok {
		t.Fatal("MCP file should be omitted when MCP examples are disabled")
	}
	readme := files["README.md"]
	if strings.Contains(readme, "Workspace: `workspace`") || !strings.Contains(readme, "Starter teams `platform`, `apps`") {
		t.Fatalf("README should describe selected team roots:\n%s", readme)
	}
	scope := files["scopes/dev/scope.yaml"]
	if !strings.Contains(scope, `NOPSAI_SETUP_WORKSPACE: "platform,apps"`) {
		t.Fatalf("scope should use selected team roots:\n%s", scope)
	}
	if _, ok := files["knowledge/guideline/platform/setup-run.md"]; !ok {
		t.Fatal("expected setup knowledge under the first selected team")
	}
}

func TestSetupPipelineRunStructureUsesSelectedRepositoryTeamsAsRoots(t *testing.T) {
	structure := setupPipelineRunStructure([]setupRepositoryTeamInput{
		{Name: "platform", Repositories: []string{"acme/service-api"}},
		{Name: "applications", Repositories: nil},
	}, nil)

	if _, ok := structure["workspace"]; ok {
		t.Fatal("setup structure should not create an implicit workspace root when repository teams were selected")
	}
	platform, ok := structure["platform"]
	if !ok {
		t.Fatalf("setup structure roots = %#v, want platform root", structure)
	}
	if platform.Description != "Repository team platform" {
		t.Fatalf("platform description = %q", platform.Description)
	}
	if len(platform.Apps) != 1 || platform.Apps[0].RepositoryFullName != "acme/service-api" {
		t.Fatalf("platform apps = %#v, want acme/service-api", platform.Apps)
	}
	applications, ok := structure["applications"]
	if !ok {
		t.Fatalf("setup structure roots = %#v, want applications root", structure)
	}
	if len(applications.Apps) != 0 || len(applications.Children) != 0 {
		t.Fatalf("applications structure = %#v, want empty selected team root", applications)
	}
}

func TestSetupDoesNotCreateTeamStructureWithoutSelectedTeams(t *testing.T) {
	structure := setupPipelineRunStructure(nil, nil)
	if len(structure) != 0 {
		t.Fatalf("setup structure = %#v, want no synthetic teams", structure)
	}
	files := setupStarterTemplatesWithOptions(setupProfileTeam, nil, setupTemplateOptions{
		IncludeLLM: false,
		IncludeMCP: false,
	})
	for path, content := range files {
		if strings.Contains(path, "workspace") || strings.Contains(content, "Workspace: `workspace`") {
			t.Fatalf("starter template should not invent workspace in %s:\n%s", path, content)
		}
	}
	if _, ok := files["knowledge/guideline/workspace/setup-run.md"]; ok {
		t.Fatal("starter templates should not create knowledge under an implicit workspace team")
	}
}

func TestSetupStarterTemplatesIncludeSelectedUsersInAccess(t *testing.T) {
	files := setupStarterTemplatesWithOptions(setupProfileTeam, nil, setupTemplateOptions{
		RepositoryTeams: []setupRepositoryTeamInput{
			{Name: "team-1", Repositories: []string{"acme/service-api"}},
		},
		Users: []setupUserInput{
			{Sub: "alice@example.com", Email: "alice@example.com", Role: "developer", Team: "team-1"},
		},
		IncludeLLM: true,
		IncludeMCP: false,
	})
	access := files["access/bootstrap.yaml"]
	for _, want := range []string{`sub: "alice@example.com"`, `email: "alice@example.com"`, `role: developer`, `resource: team:team-1`} {
		if !strings.Contains(access, want) {
			t.Fatalf("access file missing %q:\n%s", want, access)
		}
	}
	if strings.Contains(access, "password:") {
		t.Fatalf("access file should not write generated passwords:\n%s", access)
	}
}

func TestSetupAccessFallsBackToSelectedTeamForUnknownUserTeam(t *testing.T) {
	access := setupAccessYAML([]setupRepositoryTeamInput{
		{Name: "platform", Repositories: nil},
	}, []setupUserInput{
		{Sub: "alice@example.com", Email: "alice@example.com", Role: "developer", Team: "workspace/platform"},
	})
	if !strings.Contains(access, `resource: team:platform`) {
		t.Fatalf("access file should grant selected team root:\n%s", access)
	}
	if strings.Contains(access, "workspace/platform") {
		t.Fatalf("access file should not grant an implicit workspace child:\n%s", access)
	}
}

func TestSetupLLMProfileYAMLGeminiHasProviderSpecificFields(t *testing.T) {
	got := setupLLMProfileYAML(setupLLMProfileInput{
		Provider:      config.LLMProviderGemini,
		Model:         "gemini-2.5-pro",
		CredentialRef: "credential://system/llm/gemini",
	})
	for _, want := range []string{"provider: gemini", "model: gemini-2.5-pro", "credential_ref: credential://system/llm/gemini"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Gemini template missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "base_url:") {
		t.Fatalf("Gemini template should not contain base_url:\n%s", got)
	}
}

func TestSetupLLMProfileYAMLOpenAIUsesProviderDefaults(t *testing.T) {
	got := setupLLMProfileYAML(setupLLMProfileInput{
		Provider: config.LLMProviderOpenAI,
	})
	for _, want := range []string{
		"provider: openai",
		"model: gpt-4.1-mini",
		"base_url: https://api.openai.com/v1",
		"credential_ref: credential://system/llm/standard",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("OpenAI template missing %q:\n%s", want, got)
		}
	}
}

func TestSetupLLMProfileYAMLPreservesAdvancedOptions(t *testing.T) {
	temperature := 0.2
	got := setupLLMProfileYAML(setupLLMProfileInput{
		Provider:       config.LLMProviderOpenRouter,
		TimeoutSeconds: 30,
		MaxTokens:      4096,
		Temperature:    &temperature,
		Extra: map[string]string{
			"x_title":      "NopsAI",
			"http_referer": "https://nopsai.example.com",
		},
	})
	for _, want := range []string{
		"timeout_seconds: 30",
		"max_tokens: 4096",
		"temperature: 0.2",
		"http_referer: https://nopsai.example.com",
		"x_title: NopsAI",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("OpenRouter template missing %q:\n%s", want, got)
		}
	}
}

func TestSetupBootstrapWarningsWhenLLMSkipped(t *testing.T) {
	seedLLM := false
	warnings := setupBootstrapWarnings(setupBootstrapRequest{
		Profile:        setupProfileTeam,
		SeedLLMProfile: &seedLLM,
	})
	if len(warnings) != 1 || !strings.Contains(warnings[0], "AI-enabled") {
		t.Fatalf("warnings = %#v, want AI-enabled pipeline warning", warnings)
	}

	seedLLM = true
	if warnings := setupBootstrapWarnings(setupBootstrapRequest{Profile: setupProfileTeam, SeedLLMProfile: &seedLLM}); len(warnings) != 0 {
		t.Fatalf("warnings with LLM seed enabled = %#v, want none", warnings)
	}
	seedLLM = false
	if warnings := setupBootstrapWarnings(setupBootstrapRequest{Profile: setupProfileProduction, SeedLLMProfile: &seedLLM}); len(warnings) != 0 {
		t.Fatalf("production warnings = %#v, want none", warnings)
	}
}

func TestGenerateSetupSecretsUsesEffectiveDispatcherTLSFallback(t *testing.T) {
	app := App{cfg: &config.Config{
		JWTSigningKey:        "browser-jwt-signing-key-012345678901234567890123",
		ServiceJWTSigningKey: "service-jwt-signing-key-012345678901234567890123",
		AAASharedToken:       "aaa-shared-token-012345678901234567890123",
	}}
	names, restart, err := app.generateSetupSecrets()
	if err != nil {
		t.Fatal(err)
	}
	if restart || len(names) != 0 {
		t.Fatalf("generateSetupSecrets() names=%#v restart=%v, want no env write", names, restart)
	}
}

func TestGenerateSetupSecretsRequiresEnvFilePathWhenSecretsMissing(t *testing.T) {
	app := App{cfg: &config.Config{}}
	_, _, err := app.generateSetupSecrets()
	if err == nil || !strings.Contains(err.Error(), "runtime env file path is not configured") {
		t.Fatalf("generateSetupSecrets() error = %v, want env file path error", err)
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

func TestSetupPreflightBlocksProductionGateFailures(t *testing.T) {
	resp := buildSetupPreflightResponse(context.Background(), &config.Config{
		Environment:          "production",
		DatabaseURL:          "postgres://db",
		MasterKey:            "01234567890123456789012345678901",
		JWTSigningKey:        "abcdefghijklmnopqrstuvwxyz123456",
		ServiceJWTSigningKey: "",
		AAASharedToken:       devDefaultAAAToken,
		DispatcherTLSMode:    "disabled",
	}, "config.yml", ".env", "preflight_only", nil, nil)

	checks := checksByID(resp.Checks)
	for _, id := range []string{
		"service_jwt_isolated",
		"aaa_shared_token_strength",
		"dispatcher_transport_security",
	} {
		if checks[id].Status != "error" || !checks[id].Required {
			t.Fatalf("%s check = %#v, want blocking production error", id, checks[id])
		}
	}
	if resp.Ready || resp.CanLogin {
		t.Fatalf("preflight ready = %v canLogin = %v, want both false", resp.Ready, resp.CanLogin)
	}
}

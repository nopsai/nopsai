package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nopsai/config"
	"nopsai/pkg/models"

	"gopkg.in/yaml.v3"
)

func TestParseGitOpsRuntimeSettingsPlan(t *testing.T) {
	plan, err := parseGitOpsRuntimeSettingsPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		gitOpsRuntimeSettingsDirectory{
			root: "settings",
			files: map[string]string{
				"settings/system/runtime.yaml": `
dispatcher_address: dispatcher:9090
dispatcher_routing:
  prod:
    - runner-prod-1
runner_id: runner-prod-1
runner_scopes: prod
runner_capacity: 2
`,
			},
		},
	)
	if err != nil {
		t.Fatalf("parseGitOpsRuntimeSettingsPlan() error = %v", err)
	}
	if plan == nil {
		t.Fatal("parseGitOpsRuntimeSettingsPlan() = nil, want plan")
	}
	if plan.payload.DispatcherAddress == nil || *plan.payload.DispatcherAddress != "dispatcher:9090" {
		t.Fatalf("dispatcher address = %#v, want dispatcher:9090", plan.payload.DispatcherAddress)
	}
	if got := plan.payload.DispatcherRouting["prod"]; len(got) != 1 || got[0] != "runner-prod-1" {
		t.Fatalf("dispatcher routing = %#v, want prod runner", plan.payload.DispatcherRouting)
	}
	if plan.payload.RunnerCapacity == nil || *plan.payload.RunnerCapacity != 2 {
		t.Fatalf("runner capacity = %#v, want 2", plan.payload.RunnerCapacity)
	}
}

func TestParseGitOpsRuntimeSettingsPlanSupportsSettingAndSettingsPaths(t *testing.T) {
	tests := []struct {
		name       string
		root       string
		path       string
		sourcePath string
	}{
		{name: "setting runtime", root: "setting", path: "setting/system/runtime.yaml", sourcePath: "setting/system/runtime.yaml"},
		{name: "settings runner", root: "settings", path: "settings/system/runner.yml", sourcePath: "settings/system/runner.yml"},
		{name: "empty root dispatcher", root: "", path: "system/dispatcher.yaml", sourcePath: "system/dispatcher.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := parseGitOpsRuntimeSettingsPlan(
				models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
				gitOpsRuntimeSettingsDirectory{
					root: tt.root,
					files: map[string]string{
						tt.path: `
runner_id: runner-a
`,
						"setting/system/not-runtime.yaml": "runner_id: ignored",
					},
				},
			)
			if err != nil {
				t.Fatalf("parseGitOpsRuntimeSettingsPlan() error = %v", err)
			}
			if plan == nil {
				t.Fatal("parseGitOpsRuntimeSettingsPlan() = nil, want plan")
			}
			if plan.sourcePath != tt.sourcePath {
				t.Fatalf("sourcePath = %q, want %q", plan.sourcePath, tt.sourcePath)
			}
			if plan.payload.RunnerID == nil || *plan.payload.RunnerID != "runner-a" {
				t.Fatalf("runner_id = %#v, want runner-a", plan.payload.RunnerID)
			}
		})
	}
}

func TestParseGitOpsRuntimeSettingsPlanReturnsNilWhenNoRuntimeFile(t *testing.T) {
	plan, err := parseGitOpsRuntimeSettingsPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"},
		gitOpsRuntimeSettingsDirectory{
			root: "settings",
			files: map[string]string{
				"settings/system/not-runtime.yaml": "runner_id: ignored",
				"elsewhere/system/runtime.yaml":    "runner_id: ignored",
			},
		},
	)
	if err != nil {
		t.Fatalf("parseGitOpsRuntimeSettingsPlan() error = %v", err)
	}
	if plan != nil {
		t.Fatalf("parseGitOpsRuntimeSettingsPlan() = %#v, want nil", plan)
	}
}

func TestParseGitOpsRuntimeSettingsPlanRejectsNonSystemRepo(t *testing.T) {
	_, err := parseGitOpsRuntimeSettingsPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"},
		gitOpsRuntimeSettingsDirectory{
			root: "settings",
			files: map[string]string{
				"settings/system/runtime.yaml": "runner_id: runner-a",
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "system config repository") {
		t.Fatalf("expected system-scope error, got %v", err)
	}
}

func TestParseGitOpsRuntimeSettingsPlanRejectsMultipleRuntimeFiles(t *testing.T) {
	_, err := parseGitOpsRuntimeSettingsPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		gitOpsRuntimeSettingsDirectory{
			root: "settings",
			files: map[string]string{
				"settings/system/runner.yaml":  "runner_id: runner-a",
				"settings/system/runtime.yaml": "runner_id: runner-b",
			},
		},
	)
	if err == nil {
		t.Fatal("parseGitOpsRuntimeSettingsPlan() error = nil, want duplicate-file error")
	}
	if !strings.Contains(err.Error(), "multiple runtime settings GitOps files found") ||
		!strings.Contains(err.Error(), "settings/system/runner.yaml, settings/system/runtime.yaml") {
		t.Fatalf("duplicate-file error = %q", err.Error())
	}
}

func TestParseGitOpsRuntimeSettingsFileMapsAllFieldsAndRejectsInvalidCapacity(t *testing.T) {
	plan, err := parseGitOpsRuntimeSettingsFile(`
agent_nopsai_api_url: http://nopsai:8080
git_bot_nopsai_api_url: http://git-bot:8081
nopsai_git_bot_api_url: http://git-bot:8081
dispatcher_address: dispatcher:9090
agent_image: nopsai-agent:dev
docker_network_name: nopsai-net
auto_removal_agent_container: false
default_pipeline_timeout: 45m
llm_agent_timeout: 3m
dispatcher_routing:
  prod: [runner-prod]
runner_id: runner-prod
runner_scopes: prod,dev
runner_capacity: 3
`, "settings/system/runtime.yaml")
	if err != nil {
		t.Fatalf("parseGitOpsRuntimeSettingsFile() error = %v", err)
	}
	if plan.payload.AgentNopsaiAPIURL == nil || *plan.payload.AgentNopsaiAPIURL != "http://nopsai:8080" {
		t.Fatalf("agent URL = %#v", plan.payload.AgentNopsaiAPIURL)
	}
	if plan.payload.GitBotNopsaiAPIURL == nil || *plan.payload.GitBotNopsaiAPIURL != "http://git-bot:8081" {
		t.Fatalf("git-bot URL = %#v", plan.payload.GitBotNopsaiAPIURL)
	}
	if plan.payload.NopsaiGitBotAPIURL == nil || *plan.payload.NopsaiGitBotAPIURL != "http://git-bot:8081" {
		t.Fatalf("nopsai git-bot URL = %#v", plan.payload.NopsaiGitBotAPIURL)
	}
	if plan.payload.DispatcherAddress == nil || *plan.payload.DispatcherAddress != "dispatcher:9090" {
		t.Fatalf("dispatcher address = %#v", plan.payload.DispatcherAddress)
	}
	if plan.payload.AgentImage == nil || *plan.payload.AgentImage != "nopsai-agent:dev" {
		t.Fatalf("agent image = %#v", plan.payload.AgentImage)
	}
	if plan.payload.DockerNetworkName == nil || *plan.payload.DockerNetworkName != "nopsai-net" {
		t.Fatalf("docker network = %#v", plan.payload.DockerNetworkName)
	}
	if plan.payload.AutoRemovalAgentContainer == nil || *plan.payload.AutoRemovalAgentContainer {
		t.Fatalf("auto removal = %#v, want explicit false", plan.payload.AutoRemovalAgentContainer)
	}
	if plan.payload.DefaultPipelineTimeout == nil || *plan.payload.DefaultPipelineTimeout != "45m" {
		t.Fatalf("default timeout = %#v", plan.payload.DefaultPipelineTimeout)
	}
	if plan.payload.LLMAgentTimeout == nil || *plan.payload.LLMAgentTimeout != "3m" {
		t.Fatalf("llm timeout = %#v", plan.payload.LLMAgentTimeout)
	}
	if got := plan.payload.DispatcherRouting["prod"]; len(got) != 1 || got[0] != "runner-prod" {
		t.Fatalf("dispatcher routing = %#v", plan.payload.DispatcherRouting)
	}
	if plan.payload.RunnerID == nil || *plan.payload.RunnerID != "runner-prod" {
		t.Fatalf("runner_id = %#v", plan.payload.RunnerID)
	}
	if plan.payload.RunnerScopes == nil || *plan.payload.RunnerScopes != "prod,dev" {
		t.Fatalf("runner_scopes = %#v", plan.payload.RunnerScopes)
	}
	if plan.payload.RunnerCapacity == nil || *plan.payload.RunnerCapacity != 3 {
		t.Fatalf("runner_capacity = %#v", plan.payload.RunnerCapacity)
	}

	_, err = parseGitOpsRuntimeSettingsFile("runner_capacity: 0", "settings/system/runtime.yaml")
	if err == nil || !strings.Contains(err.Error(), "invalid runner_capacity") {
		t.Fatalf("expected invalid capacity error, got %v", err)
	}
}

func TestParseGitOpsRuntimeSettingsFileRejectsInvalidYAML(t *testing.T) {
	_, err := parseGitOpsRuntimeSettingsFile("dispatcher_routing: [", "settings/system/runtime.yaml")
	if err == nil || !strings.Contains(err.Error(), "failed to parse runtime settings GitOps file") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestIsGitOpsRuntimeSettingsRelativePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "system/runtime.yaml", want: true},
		{path: "/system/runtime.yml", want: true},
		{path: "system/runner.yaml", want: true},
		{path: "system/runners.yml", want: true},
		{path: "system/dispatcher.yaml", want: true},
		{path: "system/mcp.yaml", want: false},
		{path: "settings/system/runtime.yaml", want: false},
		{path: "../system/runtime.yaml", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isGitOpsRuntimeSettingsRelativePath(tt.path); got != tt.want {
				t.Fatalf("isGitOpsRuntimeSettingsRelativePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestBuildRuntimeSettingsGitOpsFileUsesValidRunnerCapacity(t *testing.T) {
	doc := buildRuntimeSettingsGitOpsFile(config.Config{RunnerCapacity: 0})
	if doc.RunnerCapacity == nil || *doc.RunnerCapacity != 1 {
		t.Fatalf("runner capacity = %#v, want default 1", doc.RunnerCapacity)
	}
}

func TestBuildRuntimeSettingsGitOpsFileClonesAndNormalizesRouting(t *testing.T) {
	cfg := config.Config{
		DispatcherRouting: map[string][]string{
			" prod ": {" runner-prod ", ""},
			"":       {" runner-default "},
		},
		RunnerCapacity: 4,
	}

	doc := buildRuntimeSettingsGitOpsFile(cfg)
	cfg.DispatcherRouting[" prod "][0] = "mutated"

	if got := doc.DispatcherRouting["prod"]; len(got) != 1 || got[0] != "runner-prod" {
		t.Fatalf("prod routing = %#v, want cloned normalized runner", doc.DispatcherRouting)
	}
	if got := doc.DispatcherRouting["*"]; len(got) != 1 || got[0] != "runner-default" {
		t.Fatalf("default routing = %#v, want normalized wildcard", doc.DispatcherRouting)
	}
}

func TestExportConfigRepositoryRuntimeSettingsUsesCanonicalSettingPath(t *testing.T) {
	app := App{cfg: &config.Config{RunnerCapacity: 2}}
	files := map[string]string{}

	if err := app.exportConfigRepositoryRuntimeSettings(models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem}, files); err != nil {
		t.Fatalf("exportConfigRepositoryRuntimeSettings() error = %v", err)
	}
	if _, ok := files["setting/system/runtime.yaml"]; !ok {
		t.Fatalf("missing canonical runtime settings export path: %#v", files)
	}
	if _, ok := files["settings/system/runtime.yaml"]; ok {
		t.Fatalf("unexpected plural runtime settings export path: %#v", files)
	}
}

func TestApplyRuntimeSettingsGitOpsPlanPersistsConfigAndEnvOverrides(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yml")
	envPath := filepath.Join(tempDir, ".env")
	if err := os.WriteFile(configPath, []byte("database_url: postgres://keep\nrunner_capacity: 9\n"), 0o644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	if err := os.WriteFile(envPath, []byte("# keep this comment\nRUNNER_ID=\"old-runner\"\n"), 0o644); err != nil {
		t.Fatalf("write env fixture: %v", err)
	}

	app := App{
		cfg: &config.Config{
			DatabaseURL:            "postgres://keep",
			RunnerCapacity:         9,
			DispatcherRouting:      map[string][]string{"old": {"old-runner"}},
			AgentNopsaiAPIURL:      "http://old-nopsai",
			DockerNetworkName:      "old-net",
			DefaultPipelineTimeout: "20m",
		},
		configPath:  configPath,
		envFilePath: envPath,
	}
	plan := &gitOpsRuntimeSettingsPlan{
		sourcePath: "settings/system/runtime.yaml",
		payload: systemConfigPayload{
			AgentNopsaiAPIURL:         stringPtr(" http://nopsai.example.com "),
			DispatcherAddress:         stringPtr(" dispatcher:9090 "),
			AutoRemovalAgentContainer: boolPtr(false),
			DefaultPipelineTimeout:    stringPtr(" 45m "),
			DispatcherRouting: map[string][]string{
				" prod ": {" runner-prod ", ""},
				"":       {" runner-default "},
			},
			RunnerID:       stringPtr(" runner-prod "),
			RunnerScopes:   stringPtr(" prod, /dev/ ,prod "),
			RunnerCapacity: intPtr(3),
		},
	}

	if err := app.applyRuntimeSettingsGitOpsPlan(plan); err != nil {
		t.Fatalf("applyRuntimeSettingsGitOpsPlan() error = %v", err)
	}

	cfg := app.getConfigSnapshot()
	if cfg.AgentNopsaiAPIURL != "http://nopsai.example.com" {
		t.Fatalf("AgentNopsaiAPIURL = %q", cfg.AgentNopsaiAPIURL)
	}
	if cfg.DispatcherAddress != "dispatcher:9090" {
		t.Fatalf("DispatcherAddress = %q", cfg.DispatcherAddress)
	}
	if cfg.RunnerID != "runner-prod" || cfg.RunnerScopes != "prod,dev" || cfg.RunnerCapacity != 3 {
		t.Fatalf("runner config = (%q, %q, %d)", cfg.RunnerID, cfg.RunnerScopes, cfg.RunnerCapacity)
	}
	if got := cfg.DispatcherRouting["prod"]; len(got) != 1 || got[0] != "runner-prod" {
		t.Fatalf("dispatcher routing = %#v", cfg.DispatcherRouting)
	}
	if got := cfg.DispatcherRouting["*"]; len(got) != 1 || got[0] != "runner-default" {
		t.Fatalf("dispatcher routing = %#v", cfg.DispatcherRouting)
	}

	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	var persisted struct {
		DatabaseURL               string              `yaml:"database_url"`
		AgentNopsaiAPIURL         string              `yaml:"agent_nopsai_api_url"`
		DispatcherAddress         string              `yaml:"dispatcher_address"`
		AutoRemovalAgentContainer bool                `yaml:"auto_removal_agent_container"`
		DefaultPipelineTimeout    string              `yaml:"default_pipeline_timeout"`
		DispatcherRouting         map[string][]string `yaml:"dispatcher_routing"`
		RunnerID                  string              `yaml:"runner_id"`
		RunnerScopes              string              `yaml:"runner_scopes"`
		RunnerCapacity            int                 `yaml:"runner_capacity"`
	}
	if err := yaml.Unmarshal(configBytes, &persisted); err != nil {
		t.Fatalf("parse persisted config: %v\n%s", err, string(configBytes))
	}
	if persisted.DatabaseURL != "postgres://keep" {
		t.Fatalf("database_url = %q, want existing value preserved", persisted.DatabaseURL)
	}
	if persisted.AgentNopsaiAPIURL != "http://nopsai.example.com" ||
		persisted.DispatcherAddress != "dispatcher:9090" ||
		persisted.AutoRemovalAgentContainer ||
		persisted.DefaultPipelineTimeout != "45m" ||
		persisted.RunnerID != "runner-prod" ||
		persisted.RunnerScopes != "prod,dev" ||
		persisted.RunnerCapacity != 3 {
		t.Fatalf("persisted config = %#v", persisted)
	}
	if got := persisted.DispatcherRouting["prod"]; len(got) != 1 || got[0] != "runner-prod" {
		t.Fatalf("persisted routing = %#v", persisted.DispatcherRouting)
	}

	envBytes, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read persisted env: %v", err)
	}
	env := string(envBytes)
	for _, want := range []string{
		"# keep this comment",
		`RUNNER_ID="runner-prod"`,
		`RUNNER_SCOPES="prod,dev"`,
		`RUNNER_CAPACITY="3"`,
		`AGENT_NOPSAI_API_URL="http://nopsai.example.com"`,
		`DISPATCHER_ADDRESS="dispatcher:9090"`,
		`AUTO_REMOVAL_AGENT_CONTAINER="false"`,
		`DEFAULT_PIPELINE_TIMEOUT="45m"`,
	} {
		if !strings.Contains(env, want) {
			t.Fatalf("env file missing %q:\n%s", want, env)
		}
	}
	if !strings.Contains(env, "DISPATCHER_ROUTING=") ||
		!strings.Contains(env, "runner-prod") ||
		!strings.Contains(env, "runner-default") {
		t.Fatalf("env file missing dispatcher routing JSON:\n%s", env)
	}
}

package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/services/nopsai/pkg/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"gopkg.in/yaml.v3"
)

func TestParseGitOpsRuntimeSettingsPlan(t *testing.T) {
	plan, err := parseGitOpsRuntimeSettingsPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		gitOpsRuntimeSettingsDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/runner.yaml": `
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

func TestParseGitOpsRuntimeSettingsPlanSupportsOnlyRunnerYAML(t *testing.T) {
	tests := []struct {
		name       string
		root       string
		path       string
		sourcePath string
	}{
		{name: "setting runner", root: "setting", path: "setting/system/runner.yaml", sourcePath: "setting/system/runner.yaml"},
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
						"setting/system/runtime.yaml": "runner_id: ignored",
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
			root: "setting",
			files: map[string]string{
				"setting/system/not-runtime.yaml": "runner_id: ignored",
				"setting/system/runtime.yaml":     "runner_id: ignored",
				"elsewhere/system/runner.yaml":    "runner_id: ignored",
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
			root: "setting",
			files: map[string]string{
				"setting/system/runner.yaml": "runner_id: runner-a",
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "system config repository") {
		t.Fatalf("expected system-scope error, got %v", err)
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
github_app_id: "123456"
github_installation_id: "987654"
github_private_key_credential_ref: credential://system/github/app-private-key
github_webhook_credential_ref: credential://system/github/webhook-secret
`, "setting/system/runner.yaml")
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
	if plan.payload.GitHubAppID == nil || *plan.payload.GitHubAppID != "123456" {
		t.Fatalf("github_app_id = %#v", plan.payload.GitHubAppID)
	}
	if plan.payload.GitHubInstallationID == nil || *plan.payload.GitHubInstallationID != "987654" {
		t.Fatalf("github_installation_id = %#v", plan.payload.GitHubInstallationID)
	}
	if plan.payload.GitHubPrivateKeyRef == nil || *plan.payload.GitHubPrivateKeyRef != "credential://system/github/app-private-key" {
		t.Fatalf("github_private_key_credential_ref = %#v", plan.payload.GitHubPrivateKeyRef)
	}
	if plan.payload.GitHubWebhookRef == nil || *plan.payload.GitHubWebhookRef != "credential://system/github/webhook-secret" {
		t.Fatalf("github_webhook_credential_ref = %#v", plan.payload.GitHubWebhookRef)
	}

	_, err = parseGitOpsRuntimeSettingsFile("runner_capacity: 0", "setting/system/runner.yaml")
	if err == nil || !strings.Contains(err.Error(), "invalid runner_capacity") {
		t.Fatalf("expected invalid capacity error, got %v", err)
	}
}

func TestParseGitOpsRuntimeSettingsFileRejectsInvalidYAML(t *testing.T) {
	_, err := parseGitOpsRuntimeSettingsFile("dispatcher_routing: [", "setting/system/runner.yaml")
	if err == nil || !strings.Contains(err.Error(), "failed to parse runtime settings GitOps file") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestIsGitOpsRuntimeSettingsRelativePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "system/runtime.yaml", want: false},
		{path: "/system/runtime.yml", want: false},
		{path: "system/runner.yaml", want: true},
		{path: "system/runner.yml", want: false},
		{path: "system/runners.yaml", want: false},
		{path: "system/runners.yml", want: false},
		{path: "system/dispatcher.yaml", want: false},
		{path: "system/mcp.yaml", want: false},
		{path: "../system/runner.yaml", want: false},
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

func TestHandleInternalDispatcherRoutingRequiresDispatcherClaims(t *testing.T) {
	app := App{cfg: &config.Config{
		DispatcherRouting: map[string][]string{
			"prod": {"runner-prod"},
		},
	}}

	unauthorizedReq := httptest.NewRequest(http.MethodGet, "/v1/internal/dispatcher/routing", nil)
	unauthorizedRec := httptest.NewRecorder()
	app.handleInternalDispatcherRouting(unauthorizedRec, unauthorizedReq)
	if unauthorizedRec.Code != http.StatusForbidden {
		t.Fatalf("unauthorized status = %d, want %d", unauthorizedRec.Code, http.StatusForbidden)
	}

	authorizedReq := httptest.NewRequest(http.MethodGet, "/v1/internal/dispatcher/routing", nil)
	authorizedReq = authorizedReq.WithContext(auth.WithClaims(authorizedReq.Context(), &auth.Claims{
		Sub:      "dispatcher",
		Provider: "internal-service",
	}))
	authorizedRec := httptest.NewRecorder()
	app.handleInternalDispatcherRouting(authorizedRec, authorizedReq)
	if authorizedRec.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, want %d: %s", authorizedRec.Code, http.StatusOK, authorizedRec.Body.String())
	}

	var resp struct {
		DispatcherRouting map[string][]string `json:"dispatcher_routing"`
	}
	if err := json.Unmarshal(authorizedRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := resp.DispatcherRouting["prod"]; len(got) != 1 || got[0] != "runner-prod" {
		t.Fatalf("dispatcher routing = %#v, want prod runner", resp.DispatcherRouting)
	}
}

func TestExportConfigRepositoryRuntimeSettingsUsesCanonicalRunnerPath(t *testing.T) {
	app := App{cfg: &config.Config{RunnerCapacity: 2}}
	files := map[string]string{}

	if err := app.exportConfigRepositoryRuntimeSettings(models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem}, files); err != nil {
		t.Fatalf("exportConfigRepositoryRuntimeSettings() error = %v", err)
	}
	if _, ok := files["setting/system/runner.yaml"]; !ok {
		t.Fatalf("missing canonical runner settings export path: %#v", files)
	}
	for _, unexpected := range []string{"setting/system/runtime.yaml", "settings/system/runtime.yaml", "settings/system/runner.yaml"} {
		if _, ok := files[unexpected]; ok {
			t.Fatalf("unexpected non-canonical runtime settings export path %q: %#v", unexpected, files)
		}
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
		sourcePath: "setting/system/runner.yaml",
		payload: systemConfigPayload{
			AgentNopsaiAPIURL:         stringPtr(" http://nopsai.example.com "),
			DispatcherAddress:         stringPtr(" dispatcher:9090 "),
			AutoRemovalAgentContainer: boolPtr(false),
			DefaultPipelineTimeout:    stringPtr(" 45m "),
			DispatcherRouting: map[string][]string{
				" prod ": {" runner-prod ", ""},
				"":       {" runner-default "},
			},
			RunnerID:             stringPtr(" runner-prod "),
			RunnerScopes:         stringPtr(" prod, /dev/ ,prod "),
			RunnerCapacity:       intPtr(3),
			GitHubAppID:          stringPtr(" 123456 "),
			GitHubInstallationID: stringPtr(" 987654 "),
			GitHubPrivateKeyRef:  stringPtr(" credential://system/github/app-private-key "),
			GitHubWebhookRef:     stringPtr(" credential://system/github/webhook-secret "),
		},
	}

	if err := app.applyRuntimeSettingsGitOpsPlan(context.Background(), models.ConfigRepository{ID: 17}, plan, "commit-a"); err != nil {
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
	if cfg.GitHubAppID != "123456" ||
		cfg.GitHubInstallID != "987654" ||
		cfg.GitHubPrivateKeyCredentialRef != "credential://system/github/app-private-key" ||
		cfg.GitHubWebhookCredentialRef != "credential://system/github/webhook-secret" {
		t.Fatalf("github settings = (%q, %q, %q, %q)", cfg.GitHubAppID, cfg.GitHubInstallID, cfg.GitHubPrivateKeyCredentialRef, cfg.GitHubWebhookCredentialRef)
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
		GitHubAppID               string              `yaml:"github_app_id"`
		GitHubInstallationID      string              `yaml:"github_installation_id"`
		GitHubPrivateKeyRef       string              `yaml:"github_private_key_credential_ref"`
		GitHubWebhookRef          string              `yaml:"github_webhook_credential_ref"`
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
		persisted.RunnerCapacity != 3 ||
		persisted.GitHubAppID != "123456" ||
		persisted.GitHubInstallationID != "987654" ||
		persisted.GitHubPrivateKeyRef != "credential://system/github/app-private-key" ||
		persisted.GitHubWebhookRef != "credential://system/github/webhook-secret" {
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
		`GITHUB_APP_ID="123456"`,
		`GITHUB_INSTALLATION_ID="987654"`,
		`GITHUB_PRIVATE_KEY_CREDENTIAL_REF="credential://system/github/app-private-key"`,
		`GITHUB_WEBHOOK_CREDENTIAL_REF="credential://system/github/webhook-secret"`,
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

func TestPersistRuntimeSettingsSnapshotStoresDurableGitOpsPayload(t *testing.T) {
	repoID := int64(42)
	db := &runtimeSettingsFakeQuerier{}
	cfg := config.Config{
		AgentNopsaiAPIURL:         "http://nopsai.example.com",
		DispatcherAddress:         "dispatcher:9090",
		AutoRemovalAgentContainer: false,
		DefaultPipelineTimeout:    "45m",
		DispatcherRouting: map[string][]string{
			" prod ": {" runner-prod ", ""},
			"":       {" runner-default "},
		},
		RunnerID:                      "runner-prod",
		RunnerScopes:                  "prod,dev",
		RunnerCapacity:                3,
		GitHubAppID:                   "123456",
		GitHubInstallID:               "987654",
		GitHubPrivateKeyCredentialRef: "credential://system/github/app-private-key",
		GitHubWebhookCredentialRef:    "credential://system/github/webhook-secret",
	}

	if err := persistRuntimeSettingsSnapshotToDB(context.Background(), db, cfg, "git", &repoID, " setting/system/runner.yaml ", " commit-a ", true); err != nil {
		t.Fatalf("persistRuntimeSettingsSnapshotToDB() error = %v", err)
	}
	if len(db.execArgs) != 6 {
		t.Fatalf("exec args = %d, want 6", len(db.execArgs))
	}
	if db.execArgs[1] != "git" || db.execArgs[2] != &repoID || db.execArgs[3] != "setting/system/runner.yaml" || db.execArgs[4] != "commit-a" || db.execArgs[5] != true {
		t.Fatalf("metadata args = %#v", db.execArgs[1:])
	}

	var stored runtimeSettingsGitOpsFile
	raw, ok := db.execArgs[0].(string)
	if !ok {
		t.Fatalf("payload arg = %T, want string", db.execArgs[0])
	}
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatalf("decode stored payload: %v\n%s", err, raw)
	}
	if stored.RunnerCapacity == nil || *stored.RunnerCapacity != 3 {
		t.Fatalf("runner capacity = %#v", stored.RunnerCapacity)
	}
	if stored.AutoRemovalAgentContainer == nil || *stored.AutoRemovalAgentContainer {
		t.Fatalf("auto removal = %#v, want explicit false", stored.AutoRemovalAgentContainer)
	}
	if got := stored.DispatcherRouting["prod"]; len(got) != 1 || got[0] != "runner-prod" {
		t.Fatalf("stored routing = %#v", stored.DispatcherRouting)
	}
	if got := stored.DispatcherRouting["*"]; len(got) != 1 || got[0] != "runner-default" {
		t.Fatalf("stored routing = %#v", stored.DispatcherRouting)
	}
	if stored.GitHubAppID == nil || *stored.GitHubAppID != "123456" ||
		stored.GitHubInstallationID == nil || *stored.GitHubInstallationID != "987654" ||
		stored.GitHubPrivateKeyRef == nil || *stored.GitHubPrivateKeyRef != "credential://system/github/app-private-key" ||
		stored.GitHubWebhookRef == nil || *stored.GitHubWebhookRef != "credential://system/github/webhook-secret" {
		t.Fatalf("stored GitHub settings = %#v", stored)
	}
}

func TestLoadRuntimeSettingsRecordAppliesPersistedGitOpsSnapshot(t *testing.T) {
	repoID := int64(42)
	updatedAt := sql.NullTime{Valid: true}
	raw := []byte(`{
		"dispatcher_address": "dispatcher-gitops:9090",
		"auto_removal_agent_container": false,
		"dispatcher_routing": {" prod ": [" runner-prod ", ""]},
		"runner_id": " runner-prod ",
		"runner_scopes": "prod, /dev/, prod",
		"runner_capacity": 5,
		"github_app_id": "123456",
		"github_installation_id": "987654",
		"github_private_key_credential_ref": " credential://system/github/app-private-key ",
		"github_webhook_credential_ref": " credential://system/github/webhook-secret "
	}`)
	db := &runtimeSettingsFakeQuerier{
		row: runtimeSettingsFakeRow{scan: func(dest ...any) error {
			*(dest[0].(*[]byte)) = raw
			*(dest[1].(*string)) = "git"
			*(dest[2].(*sql.NullInt64)) = sql.NullInt64{Int64: repoID, Valid: true}
			*(dest[3].(*string)) = "setting/system/runner.yaml"
			*(dest[4].(*string)) = "commit-a"
			*(dest[5].(*bool)) = true
			*(dest[6].(*sql.NullTime)) = updatedAt
			return nil
		}},
	}

	record, found, err := loadRuntimeSettingsRecord(context.Background(), db)
	if err != nil {
		t.Fatalf("loadRuntimeSettingsRecord() error = %v", err)
	}
	if !found {
		t.Fatal("loadRuntimeSettingsRecord() found = false, want true")
	}
	if record.Source != "git" || record.ConfigRepoID == nil || *record.ConfigRepoID != repoID || !record.ManagedByConfigRepo {
		t.Fatalf("record metadata = %#v", record)
	}

	cfg := config.Config{
		DispatcherAddress:         "old:9090",
		AutoRemovalAgentContainer: true,
		RunnerID:                  "old-runner",
		RunnerScopes:              "old",
		RunnerCapacity:            1,
	}
	next, err := applyRuntimeSettingsRecordToConfig(&cfg, record)
	if err != nil {
		t.Fatalf("applyRuntimeSettingsRecordToConfig() error = %v", err)
	}
	if next.DispatcherAddress != "dispatcher-gitops:9090" {
		t.Fatalf("dispatcher address = %q", next.DispatcherAddress)
	}
	if next.AutoRemovalAgentContainer {
		t.Fatal("auto removal should remain explicit false")
	}
	if next.RunnerID != "runner-prod" || next.RunnerScopes != "prod,dev" || next.RunnerCapacity != 5 {
		t.Fatalf("runner settings = (%q, %q, %d)", next.RunnerID, next.RunnerScopes, next.RunnerCapacity)
	}
	if got := next.DispatcherRouting["prod"]; len(got) != 1 || got[0] != "runner-prod" {
		t.Fatalf("dispatcher routing = %#v", next.DispatcherRouting)
	}
	if next.GitHubAppID != "123456" ||
		next.GitHubInstallID != "987654" ||
		next.GitHubPrivateKeyCredentialRef != "credential://system/github/app-private-key" ||
		next.GitHubWebhookCredentialRef != "credential://system/github/webhook-secret" {
		t.Fatalf("GitHub settings = (%q, %q, %q, %q)", next.GitHubAppID, next.GitHubInstallID, next.GitHubPrivateKeyCredentialRef, next.GitHubWebhookCredentialRef)
	}
}

func TestLoadRuntimeSettingsRecordMissingReturnsNotFound(t *testing.T) {
	db := &runtimeSettingsFakeQuerier{row: runtimeSettingsFakeRow{err: pgx.ErrNoRows}}
	_, found, err := loadRuntimeSettingsRecord(context.Background(), db)
	if err != nil {
		t.Fatalf("loadRuntimeSettingsRecord() error = %v", err)
	}
	if found {
		t.Fatal("loadRuntimeSettingsRecord() found = true, want false")
	}
}

type runtimeSettingsFakeQuerier struct {
	row      pgx.Row
	execArgs []any
	execErr  error
}

func (q *runtimeSettingsFakeQuerier) QueryRow(context.Context, string, ...any) pgx.Row {
	if q.row == nil {
		return runtimeSettingsFakeRow{err: pgx.ErrNoRows}
	}
	return q.row
}

func (q *runtimeSettingsFakeQuerier) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	q.execArgs = append([]any(nil), args...)
	return pgconn.CommandTag{}, q.execErr
}

type runtimeSettingsFakeRow struct {
	scan func(dest ...any) error
	err  error
}

func (r runtimeSettingsFakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if r.scan != nil {
		return r.scan(dest...)
	}
	return nil
}

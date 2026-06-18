package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/services/nopsai/pkg/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
`, "setting/system/runner.yaml")
	if err != nil {
		t.Fatalf("parseGitOpsRuntimeSettingsFile() error = %v", err)
	}
	if plan.payload.AgentNopsaiAPIURL == nil || *plan.payload.AgentNopsaiAPIURL != "http://nopsai:8080" {
		t.Fatalf("agent URL = %#v", plan.payload.AgentNopsaiAPIURL)
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

func TestParseGitOpsRuntimeSettingsFileRejectsGitHubFields(t *testing.T) {
	_, err := parseGitOpsRuntimeSettingsFile(`
runner_id: runner-a
github_app_id: "123456"
git_bot_nopsai_api_url: http://nopsai:8080
`, "setting/system/runner.yaml")
	if err == nil || !strings.Contains(err.Error(), "setting/system/github.yaml") {
		t.Fatalf("expected move-to-github error, got %v", err)
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

func TestHandleInternalRuntimeConfigRequiresMatchingServiceRole(t *testing.T) {
	app := App{cfg: &config.Config{
		GitHubAppID:                   "123456",
		GitHubInstallID:               "987654",
		GitHubPrivateKeyCredentialRef: "credential://system/github/app-private-key",
		GitHubWebhookCredentialRef:    "credential://system/github/webhook-secret",
		GitBotNopsaiAPIURL:            "http://nopsai:8080",
	}}

	req := httptest.NewRequest(http.MethodGet, "/internal/v1/runtime-config/git-bot", nil)
	req.SetPathValue("service", "git-bot")
	rec := httptest.NewRecorder()
	app.handleInternalRuntimeConfig(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unauthorized status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	wrongRoleReq := httptest.NewRequest(http.MethodGet, "/internal/v1/runtime-config/git-bot", nil)
	wrongRoleReq.SetPathValue("service", "git-bot")
	wrongRoleReq = wrongRoleReq.WithContext(auth.WithClaims(wrongRoleReq.Context(), &auth.Claims{
		Sub:      "runner",
		Provider: "internal-service",
		Roles:    []string{"runner"},
	}))
	wrongRoleRec := httptest.NewRecorder()
	app.handleInternalRuntimeConfig(wrongRoleRec, wrongRoleReq)
	if wrongRoleRec.Code != http.StatusForbidden {
		t.Fatalf("wrong-role status = %d, want %d", wrongRoleRec.Code, http.StatusForbidden)
	}

	authorizedReq := httptest.NewRequest(http.MethodGet, "/internal/v1/runtime-config/git-bot", nil)
	authorizedReq.SetPathValue("service", "git-bot")
	authorizedReq = authorizedReq.WithContext(auth.WithClaims(authorizedReq.Context(), &auth.Claims{
		Sub:      "git-bot",
		Provider: "internal-service",
		Roles:    []string{"git-bot"},
	}))
	authorizedRec := httptest.NewRecorder()
	app.handleInternalRuntimeConfig(authorizedRec, authorizedReq)
	if authorizedRec.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, want %d: %s", authorizedRec.Code, http.StatusOK, authorizedRec.Body.String())
	}

	var resp runtimeConfigResponse
	if err := json.Unmarshal(authorizedRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Service != "git-bot" || resp.ReloadMode != config.ConfigScopeRuntimeReload {
		t.Fatalf("runtime config identity = (%q, %q)", resp.Service, resp.ReloadMode)
	}
	if resp.Config["github_private_key"] != nil || resp.Config["github_webhook_secret"] != nil {
		t.Fatalf("runtime config exposed secret material: %#v", resp.Config)
	}
	if resp.Config["github_private_key_ref"] != "credential://system/github/app-private-key" ||
		resp.Config["github_webhook_secret_ref"] != "credential://system/github/webhook-secret" {
		t.Fatalf("runtime config refs = %#v", resp.Config)
	}
	if resp.Metadata["github_app_id"].Apply != "Applies after reconnect" {
		t.Fatalf("metadata = %#v", resp.Metadata["github_app_id"])
	}
}

func TestRuntimeConfigWatchVersionSupportsVersionAliases(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/internal/v1/runtime-config/runner/watch?since_version=7", nil)
	if got := runtimeConfigWatchVersion(req); got != 7 {
		t.Fatalf("since_version = %d, want 7", got)
	}
	req = httptest.NewRequest(http.MethodGet, "/internal/v1/runtime-config/runner/watch?version=9&since_version=7", nil)
	if got := runtimeConfigWatchVersion(req); got != 9 {
		t.Fatalf("version = %d, want 9", got)
	}
}

func TestExportConfigRepositoryRuntimeSettingsUsesCanonicalRunnerPath(t *testing.T) {
	app := App{cfg: &config.Config{
		RunnerCapacity:                2,
		GitHubAppID:                   "123456",
		GitHubPrivateKeyCredentialRef: "credential://system/github/app-private-key",
	}}
	files := map[string]string{}

	if err := app.exportConfigRepositoryRuntimeSettings(models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem}, files); err != nil {
		t.Fatalf("exportConfigRepositoryRuntimeSettings() error = %v", err)
	}
	if _, ok := files["setting/system/runner.yaml"]; !ok {
		t.Fatalf("missing canonical runner settings export path: %#v", files)
	}
	if _, ok := files["setting/system/github.yaml"]; !ok {
		t.Fatalf("missing canonical GitHub settings export path: %#v", files)
	}
	if strings.Contains(files["setting/system/runner.yaml"], "github_") ||
		strings.Contains(files["setting/system/runner.yaml"], "git_bot_") {
		t.Fatalf("runner export contains GitHub-owned settings:\n%s", files["setting/system/runner.yaml"])
	}
	if strings.Contains(files["setting/system/github.yaml"], "runner_") ||
		strings.Contains(files["setting/system/github.yaml"], "dispatcher_") {
		t.Fatalf("GitHub export contains runner-owned settings:\n%s", files["setting/system/github.yaml"])
	}
	for _, unexpected := range []string{"setting/system/runtime.yaml", "settings/system/runtime.yaml", "settings/system/runner.yaml", "settings/system/github.yaml"} {
		if _, ok := files[unexpected]; ok {
			t.Fatalf("unexpected non-canonical runtime settings export path %q: %#v", unexpected, files)
		}
	}
}

func TestApplyRuntimeSettingsGitOpsPlanUsesDatabaseWithoutBootstrapFileMirroring(t *testing.T) {
	app := App{
		cfg: &config.Config{
			DatabaseURL:            "postgres://keep",
			RunnerCapacity:         9,
			DispatcherRouting:      map[string][]string{"old": {"old-runner"}},
			AgentNopsaiAPIURL:      "http://old-nopsai",
			DockerNetworkName:      "old-net",
			DefaultPipelineTimeout: "20m",
		},
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
			RunnerID:       stringPtr(" runner-prod "),
			RunnerScopes:   stringPtr(" prod, /dev/ ,prod "),
			RunnerCapacity: intPtr(3),
		},
	}
	githubPlan := &gitOpsGitHubSettingsPlan{
		sourcePath: "setting/system/github.yaml",
		payload: systemConfigPayload{
			GitBotNopsaiAPIURL:   stringPtr(" http://nopsai:8080 "),
			NopsaiGitBotAPIURL:   stringPtr(" http://git-bot:8081 "),
			GitHubAppID:          stringPtr(" 123456 "),
			GitHubInstallationID: stringPtr(" 987654 "),
			GitHubPrivateKeyRef:  stringPtr(" credential://system/github/app-private-key "),
			GitHubWebhookRef:     stringPtr(" credential://system/github/webhook-secret "),
		},
	}

	if err := app.applySystemSettingsGitOpsPlans(context.Background(), models.ConfigRepository{ID: 17}, plan, githubPlan, "commit-a"); err != nil {
		t.Fatalf("applySystemSettingsGitOpsPlans() error = %v", err)
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
	if cfg.GitBotNopsaiAPIURL != "http://nopsai:8080" || cfg.NopsaiGitBotAPIURL != "http://git-bot:8081" {
		t.Fatalf("git-bot URLs = (%q, %q)", cfg.GitBotNopsaiAPIURL, cfg.NopsaiGitBotAPIURL)
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

	var stored runtimeSettingsSnapshotFile
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
			*(dest[1].(*int64)) = 42
			*(dest[2].(*string)) = "git"
			*(dest[3].(*sql.NullInt64)) = sql.NullInt64{Int64: repoID, Valid: true}
			*(dest[4].(*string)) = "setting/system/runner.yaml"
			*(dest[5].(*string)) = "commit-a"
			*(dest[6].(*bool)) = true
			*(dest[7].(*sql.NullTime)) = updatedAt
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
	if record.Version != 42 || record.Source != "git" || record.ConfigRepoID == nil || *record.ConfigRepoID != repoID || !record.ManagedByConfigRepo {
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

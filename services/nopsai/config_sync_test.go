package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"nopsai/config"
	"nopsai/pkg/models"
)

func TestStartConfigSyncAllowsOnlyOneConcurrentStart(t *testing.T) {
	var (
		app         App
		successes   atomic.Int32
		wg          sync.WaitGroup
		concurrency = 32
	)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			startedAt := time.Unix(int64(offset+1), 0)
			if _, ok := app.startConfigSync(startedAt); ok {
				successes.Add(1)
			}
		}(i)
	}

	wg.Wait()

	if got := successes.Load(); got != 1 {
		t.Fatalf("expected exactly one config sync start to succeed, got %d", got)
	}

	status := app.getConfigSyncStatus()
	if status.Status != "running" {
		t.Fatalf("expected config sync status to be running, got %q", status.Status)
	}
	if status.StartedAt == nil {
		t.Fatal("expected config sync start time to be recorded")
	}
}

func TestNormalizeConfigPathForFolder(t *testing.T) {
	tests := []struct {
		name        string
		boundFolder string
		relPath     string
		want        string
		wantErr     bool
	}{
		{
			name:        "pipeline path joins folder",
			boundFolder: "team-1",
			relPath:     "pipelines/build.yaml",
			want:        "team-1/build",
		},
		{
			name:        "duplicated bound folder is stripped",
			boundFolder: "team-1",
			relPath:     "pipelines/team-1/build.yaml",
			want:        "team-1/build",
		},
		{
			name:        "nested bound folder is stripped",
			boundFolder: "team-1/platform",
			relPath:     "steps/team-1/platform/deploy.yml",
			want:        "team-1/platform/deploy",
		},
		{
			name:        "dot dot path is rejected",
			boundFolder: "team-1",
			relPath:     "pipelines/../build.yaml",
			wantErr:     true,
		},
		{
			name:        "escaping path is rejected",
			boundFolder: "team-1",
			relPath:     "../build.yaml",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeConfigPathForFolder(tt.boundFolder, tt.relPath)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeConfigPathForFolder() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeConfigPathForFolder() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeConfigPathForFolder() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildSystemConfigResponseDoesNotExposeConfigRepoURL(t *testing.T) {
	app := App{}
	response := app.buildSystemConfigResponse(config.Config{})

	for _, key := range []string{"config_repo_url", "config_repo_configured", "config_sync_status"} {
		if _, ok := response[key]; ok {
			t.Fatalf("buildSystemConfigResponse() exposed removed key %q", key)
		}
	}
}

func TestBuildRunnerComposeResponseUsesLiveSecretsAndAdaptsDispatcherAddress(t *testing.T) {
	app := App{}
	req := httptest.NewRequest(http.MethodGet, "http://nopsai.example.com/v1/system/dispatcher/runner-compose?runner_id=runner-cloud-1&runner_scopes=prod&runner_capacity=3", nil)
	resp, err := app.buildRunnerComposeResponse(config.Config{
		AgentNopsaiAPIURL:       "http://nopsai:8080",
		DispatcherAddress:       "dispatcher:9090",
		DispatcherListenAddress: ":9090",
		ServiceJWTSigningKey:    "service-secret",
		ServiceJWTIssuer:        "issuer",
		ServiceJWTAudience:      "audience",
		RunnerServiceID:         "runner-service",
		DispatcherTLSMode:       "mtls",
		DispatcherTLSSecret:     "tls-secret",
		DispatcherTLSServerName: "nopsai-dispatcher.example.com",
		DockerNetworkName:       "nopsai-net",
		RunnerCapacity:          1,
	}, req)
	if err != nil {
		t.Fatalf("buildRunnerComposeResponse() error = %v", err)
	}
	if resp.DispatcherAddress != "nopsai.example.com:9090" {
		t.Fatalf("dispatcher address = %q, want adapted request host", resp.DispatcherAddress)
	}
	if resp.NetworkMode != runnerNetworkModeHost {
		t.Fatalf("network mode = %q, want host for adapted remote runner", resp.NetworkMode)
	}
	if resp.RunnerImage != defaultRunnerImage {
		t.Fatalf("runner image = %q, want default", resp.RunnerImage)
	}
	for _, want := range []string{
		`image: "hoseindocker/nopsai-runner:latest"`,
		`RUNNER_ID: "runner-cloud-1"`,
		`RUNNER_SCOPES: "prod"`,
		`RUNNER_CAPACITY: "3"`,
		`DISPATCHER_ADDRESS: "nopsai.example.com:9090"`,
		`SERVICE_JWT_SIGNING_KEY: "service-secret"`,
		`SERVICE_JWT_ISSUER: "issuer"`,
		`SERVICE_JWT_AUDIENCE: "audience"`,
		`RUNNER_SERVICE_ID: "runner-service"`,
		`DISPATCHER_TLS_SECRET: "tls-secret"`,
		`DISPATCHER_TLS_SERVER_NAME: "nopsai-dispatcher.example.com"`,
		`DOCKER_NETWORK_NAME: ""`,
		`network_mode: "host"`,
	} {
		if !strings.Contains(resp.Compose, want) {
			t.Fatalf("compose missing %q:\n%s", want, resp.Compose)
		}
	}
	if strings.Contains(resp.Compose, "env_file") {
		t.Fatalf("compose should not require env_file:\n%s", resp.Compose)
	}
	if len(resp.Warnings) == 0 {
		t.Fatalf("warnings should explain adapted external runner values")
	}
}

func TestBuildRunnerBootstrapCommandResponseUsesOneTimeToken(t *testing.T) {
	app := App{}
	req := httptest.NewRequest(http.MethodGet, "http://nopsai.example.com/v1/system/dispatcher/runner-bootstrap-command?runner_id=runner-cloud-1&runner_scopes=prod&runner_capacity=3", nil)
	resp, err := app.buildRunnerBootstrapCommandResponse(config.Config{
		AgentNopsaiAPIURL:       "http://nopsai:8080",
		DispatcherAddress:       "dispatcher:9090",
		DispatcherListenAddress: ":9090",
		ServiceJWTSigningKey:    "service-secret",
		ServiceJWTIssuer:        "issuer",
		ServiceJWTAudience:      "audience",
		RunnerServiceID:         "runner-service",
		DispatcherTLSMode:       "mtls",
		DispatcherTLSSecret:     "tls-secret",
		DispatcherTLSServerName: "nopsai-dispatcher.example.com",
	}, req)
	if err != nil {
		t.Fatalf("buildRunnerBootstrapCommandResponse() error = %v", err)
	}
	if strings.Contains(resp.BootstrapCommand, "service-secret") || strings.Contains(resp.BootstrapCommand, "tls-secret") {
		t.Fatalf("bootstrap command should not expose long-lived secrets: %s", resp.BootstrapCommand)
	}
	if resp.NetworkMode != runnerNetworkModeHost {
		t.Fatalf("network mode = %q, want host for adapted remote runner", resp.NetworkMode)
	}
	if resp.RunnerImage != defaultRunnerImage {
		t.Fatalf("runner image = %q, want default", resp.RunnerImage)
	}
	const marker = "token="
	idx := strings.Index(resp.BootstrapCommand, marker)
	if idx < 0 {
		t.Fatalf("bootstrap command missing token: %s", resp.BootstrapCommand)
	}
	rest := resp.BootstrapCommand[idx+len(marker):]
	token := strings.Trim(rest[:strings.Index(rest, "'")], " ")
	script, ok := app.consumeRunnerBootstrapToken(token)
	if !ok {
		t.Fatal("expected bootstrap token to be consumable")
	}
	if !strings.Contains(script, "service-secret") || !strings.Contains(script, "tls-secret") {
		t.Fatalf("bootstrap script should include runner secrets:\n%s", script)
	}
	if !strings.Contains(script, "--network host") {
		t.Fatalf("bootstrap script should use host networking for adapted remote runner:\n%s", script)
	}
	if !strings.Contains(script, "image_arch=$(docker image inspect") {
		t.Fatalf("bootstrap script should check runner image architecture:\n%s", script)
	}
	if _, ok := app.consumeRunnerBootstrapToken(token); ok {
		t.Fatal("bootstrap token should be single-use")
	}
}

func TestBuildConfigRepositoryInputDefaultsAndValidation(t *testing.T) {
	input, err := buildConfigRepositoryInput(upsertConfigRepositoryRequest{
		RepoURL: " https://github.com/acme/configs.git ",
	}, models.ConfigRepositoryScopeFolder, " team-1/ ", "user-1")
	if err != nil {
		t.Fatalf("buildConfigRepositoryInput() error = %v", err)
	}
	if input.RepoURL != "https://github.com/acme/configs.git" {
		t.Fatalf("RepoURL = %q", input.RepoURL)
	}
	if input.Branch != "main" {
		t.Fatalf("Branch = %q, want main", input.Branch)
	}
	if input.ScopeID != "team-1" {
		t.Fatalf("ScopeID = %q, want team-1", input.ScopeID)
	}
	if !input.Enabled {
		t.Fatal("Enabled = false, want true")
	}

	if _, err := buildConfigRepositoryInput(upsertConfigRepositoryRequest{
		RepoURL:  "https://github.com/acme/configs.git",
		BasePath: "../outside",
	}, models.ConfigRepositoryScopeFolder, "team-1", "user-1"); err == nil {
		t.Fatal("buildConfigRepositoryInput() accepted escaping base_path")
	}
}

func TestParseConfigRepositoryBindingPath(t *testing.T) {
	tests := []struct {
		name          string
		relPath       string
		wantScopeType string
		wantScopeID   string
		wantErr       bool
	}{
		{
			name:          "top level group",
			relPath:       "groups/team-1.yaml",
			wantScopeType: models.ConfigRepositoryScopeFolder,
			wantScopeID:   "team-1",
		},
		{
			name:          "nested group",
			relPath:       "groups/team-1/platform.yaml",
			wantScopeType: models.ConfigRepositoryScopeFolder,
			wantScopeID:   "team-1/platform",
		},
		{
			name:    "unsupported scope",
			relPath: "systems/global.yaml",
			wantErr: true,
		},
		{
			name:    "escaping path",
			relPath: "groups/team-1/../prod.yaml",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScopeType, gotScopeID, err := parseConfigRepositoryBindingPath(tt.relPath)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseConfigRepositoryBindingPath() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseConfigRepositoryBindingPath() error = %v", err)
			}
			if gotScopeType != tt.wantScopeType || gotScopeID != tt.wantScopeID {
				t.Fatalf("parseConfigRepositoryBindingPath() = (%q, %q), want (%q, %q)", gotScopeType, gotScopeID, tt.wantScopeType, tt.wantScopeID)
			}
		})
	}
}

func TestNormalizePipelineRunStructureForFolder(t *testing.T) {
	structure := map[string]*pipelineRunStructureNode{
		"dev": {
			Description: "Development",
			Repos:       []string{"acme/app"},
			Children: map[string]*pipelineRunStructureNode{
				"services": {
					Description: "Service workloads",
					Children:    map[string]*pipelineRunStructureNode{},
				},
			},
		},
		"team-1": {
			Description: "Already scoped",
			Children: map[string]*pipelineRunStructureNode{
				"platform": {
					Description: "Platform",
					Children:    map[string]*pipelineRunStructureNode{},
				},
			},
		},
	}

	got, err := normalizePipelineRunStructureForFolder("team-1", structure)
	if err != nil {
		t.Fatalf("normalizePipelineRunStructureForFolder() error = %v", err)
	}
	team, ok := got["team-1"]
	if !ok {
		t.Fatal("expected team-1 root group")
	}
	if _, ok := team.Children["dev"]; !ok {
		t.Fatal("expected dev to be nested under team-1")
	}
	if gotRepos := team.Children["dev"].Repos; len(gotRepos) != 1 || gotRepos[0] != "acme/app" {
		t.Fatalf("dev repos = %#v, want acme/app", gotRepos)
	}
	if _, ok := team.Children["platform"]; !ok {
		t.Fatal("expected already scoped platform group to remain under team-1")
	}
	if _, duplicated := team.Children["team-1"]; duplicated {
		t.Fatal("did not expect duplicated team-1 child")
	}
}

func TestConfigRepositoryPrecedence(t *testing.T) {
	systemRepo := models.ConfigRepository{ID: 1, ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	parentRepo := models.ConfigRepository{ID: 2, ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"}
	childRepo := models.ConfigRepository{ID: 3, ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1/dev"}
	otherRepo := models.ConfigRepository{ID: 4, ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-2"}

	if !canConfigRepositoryWriteOver(parentRepo, systemRepo, "team-1/build") {
		t.Fatal("group repo should be able to take over parent-managed global resources")
	}
	if !canConfigRepositoryWriteOver(childRepo, parentRepo, "team-1/dev/deploy") {
		t.Fatal("child group repo should be able to take over parent group resources in its subtree")
	}
	if canConfigRepositoryWriteOver(parentRepo, childRepo, "team-1/dev/deploy") {
		t.Fatal("parent group repo should not take over child group resources")
	}
	if !configRepositoryShadowsCurrent(childRepo, parentRepo, "team-1/dev/deploy") {
		t.Fatal("child group repo should shadow parent group repo in its subtree")
	}
	if canConfigRepositoryWriteOver(otherRepo, parentRepo, "team-1/build") {
		t.Fatal("unrelated group repo should not take over another group")
	}
}

func TestEffectivePipelineRunStructureForSystemUsesConfigRepositoryGroups(t *testing.T) {
	binding := models.ConfigRepository{ID: 1, ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	configRepositories := map[string]storedConfigRepository{
		"folder/team-1": {
			scopeType: models.ConfigRepositoryScopeFolder,
			scopeID:   "team-1",
			enabled:   true,
		},
		"folder/team-2/platform": {
			scopeType: models.ConfigRepositoryScopeFolder,
			scopeID:   "team-2/platform",
			enabled:   true,
		},
	}
	structure := map[string]*pipelineRunStructureNode{
		"team-1": {
			Description: "Should not be applied from global structure",
			Repos:       []string{"acme/team-1"},
			Children: map[string]*pipelineRunStructureNode{
				"dev": {Description: "Should not be created", Children: map[string]*pipelineRunStructureNode{}},
			},
		},
		"general": {Description: "Should not be created", Children: map[string]*pipelineRunStructureNode{}},
	}

	got, err := effectivePipelineRunStructureForConfigSync(binding, configRepositories, structure, nil, []string{"team-1", "team-2/platform"})
	if err != nil {
		t.Fatalf("effectivePipelineRunStructureForConfigSync() error = %v", err)
	}
	team1, ok := got["team-1"]
	if !ok {
		t.Fatal("expected team-1 shell from config repository binding")
	}
	if team1.Description != "" || len(team1.Repos) != 0 || len(team1.Children) != 0 {
		t.Fatalf("team-1 structure = %#v, want empty shell", team1)
	}
	team2, ok := got["team-2"]
	if !ok {
		t.Fatal("expected team-2 parent shell from nested config repository binding")
	}
	if _, ok := team2.Children["platform"]; !ok {
		t.Fatal("expected team-2/platform shell from nested config repository binding")
	}
	if _, ok := got["general"]; ok {
		t.Fatal("did not expect unbound general group from global structure")
	}
}

func TestEffectivePipelineRunStructureForGroupFiltersNestedConfigRepositoryGroups(t *testing.T) {
	binding := models.ConfigRepository{ID: 2, ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"}
	configRepositories := map[string]storedConfigRepository{
		"folder/team-1/platform": {
			scopeType: models.ConfigRepositoryScopeFolder,
			scopeID:   "team-1/platform",
			enabled:   true,
		},
	}
	structure := map[string]*pipelineRunStructureNode{
		"team-1": {
			Description: "Owned by team-1 repo",
			Children: map[string]*pipelineRunStructureNode{
				"dev":      {Description: "Owned by team-1 repo", Children: map[string]*pipelineRunStructureNode{}},
				"platform": {Description: "Owned by nested repo", Children: map[string]*pipelineRunStructureNode{}},
			},
		},
	}

	got, err := effectivePipelineRunStructureForConfigSync(binding, configRepositories, structure, nil, []string{"team-1/platform"})
	if err != nil {
		t.Fatalf("effectivePipelineRunStructureForConfigSync() error = %v", err)
	}
	team1, ok := got["team-1"]
	if !ok {
		t.Fatal("expected team-1 root group")
	}
	if team1.Description != "Owned by team-1 repo" {
		t.Fatalf("team-1 description = %q", team1.Description)
	}
	if _, ok := team1.Children["dev"]; !ok {
		t.Fatal("expected non-delegated team-1/dev structure to be applied")
	}
	platform, ok := team1.Children["platform"]
	if !ok {
		t.Fatal("expected delegated team-1/platform shell from config repository binding")
	}
	if platform.Description != "" || len(platform.Children) != 0 {
		t.Fatalf("platform structure = %#v, want empty shell", platform)
	}
}

func TestConfigRepositoryGroupStructureAppliesInsideDelegatedGroup(t *testing.T) {
	binding := models.ConfigRepository{ID: 1, ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	configRepositories := map[string]storedConfigRepository{
		"folder/team-1": {
			scopeType: models.ConfigRepositoryScopeFolder,
			scopeID:   "team-1",
			enabled:   true,
		},
	}
	globalStructure := map[string]*pipelineRunStructureNode{
		"team-1": {
			Description: "Ignored from legacy pipelineruns structure",
			Repos:       []string{"acme/ignored"},
			Children:    map[string]*pipelineRunStructureNode{},
		},
	}
	groupStructure, ok, err := parseConfigRepositoryGroupPipelineRunStructure("groups/team-1/structure.yaml", `
description: Team 1 apps
repos:
  - hosein-yousefii/test-app
dev:
  repos:
    - hosein-yousefii/dev-app
`)
	if err != nil {
		t.Fatalf("parseConfigRepositoryGroupPipelineRunStructure() error = %v", err)
	}
	if !ok {
		t.Fatal("expected groups/team-1/structure.yaml to be treated as a group structure file")
	}

	got, err := effectivePipelineRunStructureForConfigSync(binding, configRepositories, globalStructure, groupStructure, []string{"team-1"})
	if err != nil {
		t.Fatalf("effectivePipelineRunStructureForConfigSync() error = %v", err)
	}
	team1, ok := got["team-1"]
	if !ok {
		t.Fatal("expected team-1 root group")
	}
	if team1.Description != "Team 1 apps" {
		t.Fatalf("team-1 description = %q", team1.Description)
	}
	if len(team1.Repos) != 1 || team1.Repos[0] != "hosein-yousefii/test-app" {
		t.Fatalf("team-1 repos = %#v, want hosein-yousefii/test-app", team1.Repos)
	}
	dev, ok := team1.Children["dev"]
	if !ok {
		t.Fatal("expected team-1/dev from config-repositories group structure")
	}
	if len(dev.Repos) != 1 || dev.Repos[0] != "hosein-yousefii/dev-app" {
		t.Fatalf("team-1/dev repos = %#v, want hosein-yousefii/dev-app", dev.Repos)
	}
}

func TestConfigRepositoryGroupStructureCollectsInlineConfig(t *testing.T) {
	structure, ok, err := parseConfigRepositoryGroupPipelineRunStructure("groups/structure.yaml", `
data-team:
  description: Owns data-team scoped configuration
  config:
    repo_url: git@github.com:hosein-yousefii/nopsai-data-team-config.git
    branch: main
    base_path: ""
    enabled: true
`)
	if err != nil {
		t.Fatalf("parseConfigRepositoryGroupPipelineRunStructure() error = %v", err)
	}
	if !ok {
		t.Fatal("expected groups/structure.yaml to be treated as a group structure file")
	}

	bindings, err := configRepositoryBindingsFromPipelineRunStructure(structure, "config-repositories/groups/structure.yaml")
	if err != nil {
		t.Fatalf("configRepositoryBindingsFromPipelineRunStructure() error = %v", err)
	}
	binding, ok := bindings["folder/data-team"]
	if !ok {
		t.Fatal("expected inline data-team config repository binding")
	}
	if binding.scopeType != models.ConfigRepositoryScopeFolder || binding.scopeID != "data-team" {
		t.Fatalf("binding scope = (%q, %q), want (folder, data-team)", binding.scopeType, binding.scopeID)
	}
	if binding.repoURL != "git@github.com:hosein-yousefii/nopsai-data-team-config.git" {
		t.Fatalf("repoURL = %q", binding.repoURL)
	}
	if binding.branch != "main" || binding.basePath != "" || !binding.enabled {
		t.Fatalf("binding defaults = branch %q basePath %q enabled %v", binding.branch, binding.basePath, binding.enabled)
	}
}

func TestFilterDelegatedConfigResourcesFiltersRepoScopeVarsByScope(t *testing.T) {
	binding := models.ConfigRepository{ID: 1, ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	generalScopeVars := map[generalScopeVarKey]storedScopeVar{
		{scopePath: "data-team/dev", name: "API_VERSION"}: {},
		{scopePath: "prod", name: "API_VERSION"}:          {},
	}
	repoScopeVars := map[repoScopeVarKey]storedScopeVar{
		{repo: "hosein-yousefii/test-app", scopePath: "data-team/dev", name: "TEST_SCOPE"}: {},
		{repo: "hosein-yousefii/test-app", scopePath: "prod", name: "TEST_SCOPE"}:          {},
	}
	generalScopeSecrets := map[generalScopeSecretKey]storedScopeSecret{
		{scopePath: "data-team/dev", name: "DEPLOY_TOKEN"}: {},
		{scopePath: "prod", name: "DEPLOY_TOKEN"}:          {},
	}
	repoScopeSecrets := map[repoScopeSecretKey]storedScopeSecret{
		{repo: "hosein-yousefii/test-app", scopePath: "data-team/dev", name: "DEPLOY_TOKEN"}: {},
		{repo: "hosein-yousefii/test-app", scopePath: "prod", name: "DEPLOY_TOKEN"}:          {},
	}

	filterDelegatedConfigResources(
		binding,
		[]string{"data-team"},
		map[string]storedPipeline{},
		map[string]storedStep{},
		map[string]storedKnowledgeContext{},
		generalScopeVars,
		repoScopeVars,
		generalScopeSecrets,
		repoScopeSecrets,
		map[string]storedTrigger{},
	)

	if _, ok := generalScopeVars[generalScopeVarKey{scopePath: "data-team/dev", name: "API_VERSION"}]; ok {
		t.Fatal("expected delegated general scope variable to be filtered")
	}
	if _, ok := repoScopeVars[repoScopeVarKey{repo: "hosein-yousefii/test-app", scopePath: "data-team/dev", name: "TEST_SCOPE"}]; ok {
		t.Fatal("expected delegated repository scope variable to be filtered by scope")
	}
	if _, ok := generalScopeVars[generalScopeVarKey{scopePath: "prod", name: "API_VERSION"}]; !ok {
		t.Fatal("expected unrelated general scope variable to remain")
	}
	if _, ok := repoScopeVars[repoScopeVarKey{repo: "hosein-yousefii/test-app", scopePath: "prod", name: "TEST_SCOPE"}]; !ok {
		t.Fatal("expected unrelated repository scope variable to remain")
	}
	if _, ok := generalScopeSecrets[generalScopeSecretKey{scopePath: "data-team/dev", name: "DEPLOY_TOKEN"}]; ok {
		t.Fatal("expected delegated general scope secret to be filtered")
	}
	if _, ok := repoScopeSecrets[repoScopeSecretKey{repo: "hosein-yousefii/test-app", scopePath: "data-team/dev", name: "DEPLOY_TOKEN"}]; ok {
		t.Fatal("expected delegated repository scope secret to be filtered by scope")
	}
	if _, ok := generalScopeSecrets[generalScopeSecretKey{scopePath: "prod", name: "DEPLOY_TOKEN"}]; !ok {
		t.Fatal("expected unrelated general scope secret to remain")
	}
	if _, ok := repoScopeSecrets[repoScopeSecretKey{repo: "hosein-yousefii/test-app", scopePath: "prod", name: "DEPLOY_TOKEN"}]; !ok {
		t.Fatal("expected unrelated repository scope secret to remain")
	}
}

func TestScopeVariablesSectionCanCoexistWithAccess(t *testing.T) {
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(`
access:
  visibility: restricted
  use_access:
    repositories:
      - hosein-yousefii/test-app
variables:
  API_VERSION: "2026.05"
  hosein-yousefii/test-app/IMAGE_NAME: "ghcr.io/team-1/service-api:dev"
`), &raw); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	generalScopeVars := map[generalScopeVarKey]storedScopeVar{}
	repoScopeVars := map[repoScopeVarKey]storedScopeVar{}
	hasAccess, err := (&App{}).addScopeConfigEntries(
		raw,
		generalScopeVars,
		repoScopeVars,
		map[generalScopeSecretKey]storedScopeSecret{},
		map[repoScopeSecretKey]storedScopeSecret{},
		"team-1/dev",
		"scopes/dev/scope.yaml",
		models.ConfigRepository{
			ScopeType: models.ConfigRepositoryScopeSystem,
			ScopeID:   models.ConfigRepositorySystemGlobalID,
		},
		"",
	)
	if err != nil {
		t.Fatalf("addScopeConfigEntries() error = %v", err)
	}
	if !hasAccess {
		t.Fatal("addScopeConfigEntries() access = false, want true")
	}

	if got := generalScopeVars[generalScopeVarKey{scopePath: "team-1/dev", name: "API_VERSION"}].value; got != "2026.05" {
		t.Fatalf("API_VERSION = %q, want 2026.05", got)
	}
	if got := repoScopeVars[repoScopeVarKey{repo: "hosein-yousefii/test-app", scopePath: "team-1/dev", name: "IMAGE_NAME"}].value; got != "ghcr.io/team-1/service-api:dev" {
		t.Fatalf("repo IMAGE_NAME = %q", got)
	}
}

func TestScopeConfigRejectsTopLevelVariables(t *testing.T) {
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(`
API_VERSION: "2026.05"
variables:
  DEPLOY_TARGET: "production"
`), &raw); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	_, err := (&App{}).addScopeConfigEntries(
		raw,
		map[generalScopeVarKey]storedScopeVar{},
		map[repoScopeVarKey]storedScopeVar{},
		map[generalScopeSecretKey]storedScopeSecret{},
		map[repoScopeSecretKey]storedScopeSecret{},
		"team-1/prod",
		"scopes/prod/scope.yaml",
		models.ConfigRepository{
			ScopeType: models.ConfigRepositoryScopeSystem,
			ScopeID:   models.ConfigRepositorySystemGlobalID,
		},
		"",
	)
	if err == nil {
		t.Fatal("addScopeConfigEntries() error = nil, want unsupported top-level key error")
	}
	if !strings.Contains(err.Error(), "unsupported top-level key 'API_VERSION'") {
		t.Fatalf("addScopeConfigEntries() error = %q, want unsupported top-level key", err)
	}
}

func TestScopeSecretsSectionImportsEncryptedValuesAndNullPlaceholders(t *testing.T) {
	app := &App{encKey: []byte("12345678901234567890123456789012")}
	encrypted, err := app.encrypt("super-secret")
	if err != nil {
		t.Fatalf("encrypt() error = %v", err)
	}

	var raw map[string]any
	if err := yaml.Unmarshal([]byte(`
secrets:
  API_TOKEN: "`+encrypted+`"
  EMPTY_TOKEN:
  BAD_TOKEN: "plain text"
  hosein-yousefii/test-app/DEPLOY_TOKEN: "`+encrypted+`"
`), &raw); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	generalScopeSecrets := map[generalScopeSecretKey]storedScopeSecret{}
	repoScopeSecrets := map[repoScopeSecretKey]storedScopeSecret{}
	secrets, ok := scopeVariablesSection(raw["secrets"])
	if !ok {
		t.Fatal("secrets section was not recognized")
	}
	for secretKey, secretValue := range secrets {
		if err := app.addScopeSecretConfigEntry(generalScopeSecrets, repoScopeSecrets, "team-1/dev", secretKey, secretValue, "scopes/dev/scope.yaml", models.ConfigRepository{
			ScopeType: models.ConfigRepositoryScopeSystem,
			ScopeID:   models.ConfigRepositorySystemGlobalID,
		}, ""); err != nil {
			t.Fatalf("addScopeSecretConfigEntry() error = %v", err)
		}
	}

	apiToken := generalScopeSecrets[generalScopeSecretKey{scopePath: "team-1/dev", name: "API_TOKEN"}]
	if apiToken.encryptedValue == nil || *apiToken.encryptedValue != encrypted {
		t.Fatalf("API_TOKEN encrypted value = %#v, want %q", apiToken.encryptedValue, encrypted)
	}
	if got := generalScopeSecrets[generalScopeSecretKey{scopePath: "team-1/dev", name: "EMPTY_TOKEN"}].encryptedValue; got != nil {
		t.Fatalf("EMPTY_TOKEN encrypted value = %#v, want nil", got)
	}
	if got := generalScopeSecrets[generalScopeSecretKey{scopePath: "team-1/dev", name: "BAD_TOKEN"}].encryptedValue; got != nil {
		t.Fatalf("BAD_TOKEN encrypted value = %#v, want nil for invalid encrypted data", got)
	}
	repoToken := repoScopeSecrets[repoScopeSecretKey{repo: "hosein-yousefii/test-app", scopePath: "team-1/dev", name: "DEPLOY_TOKEN"}]
	if repoToken.encryptedValue == nil || *repoToken.encryptedValue != encrypted {
		t.Fatalf("repo DEPLOY_TOKEN encrypted value = %#v, want %q", repoToken.encryptedValue, encrypted)
	}
}

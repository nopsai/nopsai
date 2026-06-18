package nopsai

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
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/internal/runnerinstall"
	"nopsai/services/nopsai/internal/systemconfig"
)

func boolPointer(value bool) *bool {
	return &value
}

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
			name:        "root prefix is absolute",
			boundFolder: "team-1",
			relPath:     "pipelines/root/platform/build.yaml",
			want:        "platform/build",
		},
		{
			name:        "root only is absolute root",
			boundFolder: "team-1",
			relPath:     "root",
			want:        "",
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
			got, err := configsync.NormalizePathForFolder(tt.boundFolder, tt.relPath)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("configsync.NormalizePathForFolder() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("configsync.NormalizePathForFolder() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("configsync.NormalizePathForFolder() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildSystemConfigResponseDoesNotExposeConfigRepoURL(t *testing.T) {
	response := systemconfig.BuildResponse(config.Config{}, "")

	for _, key := range []string{"config_repo_url", "config_repo_configured", "config_sync_status"} {
		if _, ok := response[key]; ok {
			t.Fatalf("buildSystemConfigResponse() exposed removed key %q", key)
		}
	}
}

func TestBuildRunnerComposeResponseUsesLiveSecretsAndAdaptsDispatcherAddress(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://nopsai.example.com/v1/system/dispatcher/runner-compose?runner_id=runner-cloud-1&runner_scopes=prod&runner_capacity=3", nil)
	resp, err := runnerinstall.BuildComposeResponse(config.Config{
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
	if resp.NetworkMode != runnerinstall.NetworkModeHost {
		t.Fatalf("network mode = %q, want host for adapted remote runner", resp.NetworkMode)
	}
	if resp.RunnerImage != runnerinstall.DefaultRunnerImage {
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
	resp, err := runnerinstall.BuildBootstrapCommandResponse(config.Config{
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
	}, req, app.createRunnerBootstrapToken)
	if err != nil {
		t.Fatalf("buildRunnerBootstrapCommandResponse() error = %v", err)
	}
	if strings.Contains(resp.BootstrapCommand, "service-secret") || strings.Contains(resp.BootstrapCommand, "tls-secret") {
		t.Fatalf("bootstrap command should not expose long-lived secrets: %s", resp.BootstrapCommand)
	}
	if resp.NetworkMode != runnerinstall.NetworkModeHost {
		t.Fatalf("network mode = %q, want host for adapted remote runner", resp.NetworkMode)
	}
	if resp.RunnerImage != runnerinstall.DefaultRunnerImage {
		t.Fatalf("runner image = %q, want default", resp.RunnerImage)
	}
	const marker = "token="
	idx := strings.Index(resp.BootstrapCommand, marker)
	if idx < 0 {
		t.Fatalf("bootstrap command missing token: %s", resp.BootstrapCommand)
	}
	rest := resp.BootstrapCommand[idx+len(marker):]
	token := strings.Trim(rest[:strings.Index(rest, "'")], " ")
	entry, ok := app.consumeRunnerBootstrapToken(token)
	if !ok {
		t.Fatal("expected bootstrap token to be consumable")
	}
	if entry.ContentType != "text/x-shellscript; charset=utf-8" {
		t.Fatalf("content type = %q, want shell script", entry.ContentType)
	}
	script := entry.Content
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

func TestBuildKubernetesRunnerManifestResponseIncludesRuntimeRBACAndPVCSettings(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://nopsai.example.com/v1/system/dispatcher/kubernetes-runner-manifest?runner_id=k8s-runner-ams-1&runner_scopes=production,eu-west&runner_capacity=30&namespace=nopsai-runs&service_account=nopsai-runner&storage_class=fast-rwo", nil)
	affinity := true
	resp, err := runnerinstall.BuildKubernetesManifestResponse(config.Config{
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
		AgentImage:              "nopsai-agent:dev",
		Kubernetes: config.KubernetesConfig{
			DefaultWorkspaceSize:       "10Gi",
			DefaultWorkspaceAccessMode: "ReadWriteOnce",
			AffinityEnabled:            &affinity,
		},
		Limits: config.RunnerLimits{
			MaxConcurrentRuns:        30,
			MaxConcurrentTasks:       200,
			MaxConcurrentTasksPerRun: 20,
			MaxPendingTasks:          1000,
		},
		RuntimePools: map[string]config.RuntimePool{
			"default": {
				NodeSelector: map[string]string{"workload": "nopsai"},
			},
		},
	}, req)
	if err != nil {
		t.Fatalf("buildKubernetesRunnerManifestResponse() error = %v", err)
	}
	if resp.Namespace != "nopsai-runs" || resp.ServiceAccount != "nopsai-runner" {
		t.Fatalf("namespace/service account = %q/%q", resp.Namespace, resp.ServiceAccount)
	}
	for _, want := range []string{
		"kind: Deployment",
		"kind: Role",
		"resources:",
		"- pods/exec",
		"RUNNER_ID: k8s-runner-ams-1",
		"RUNNER_CAPACITY: \"30\"",
		"KUBERNETES_NAMESPACE: nopsai-runs",
		"KUBERNETES_STORAGE_CLASS: fast-rwo",
		"KUBERNETES_AFFINITY_ENABLED: \"true\"",
		"LIMITS_MAX_CONCURRENT_TASKS: \"200\"",
		"SERVICE_JWT_SIGNING_KEY: service-secret",
		"DISPATCHER_TLS_SECRET: tls-secret",
	} {
		if !strings.Contains(resp.Manifest, want) {
			t.Fatalf("manifest missing %q:\n%s", want, resp.Manifest)
		}
	}
	if resp.DispatcherAddress != "nopsai.example.com:9090" {
		t.Fatalf("dispatcher address = %q, want adapted host", resp.DispatcherAddress)
	}
}

func TestBuildKubernetesRunnerManifestResponseUsesDispatcherAddressOverride(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://nopsai-ui.pre-nopsai.orb.local/v1/system/dispatcher/kubernetes-runner-manifest?runner_id=k8s-runner-ams-1&dispatcher_address=nopsai-dispatcher.pre-nopsai.orb.local%3A9090", nil)
	resp, err := runnerinstall.BuildKubernetesManifestResponse(config.Config{
		DispatcherAddress:       "dispatcher:9090",
		DispatcherListenAddress: ":9090",
		ServiceJWTSigningKey:    "service-secret",
		ServiceJWTIssuer:        "issuer",
		ServiceJWTAudience:      "audience",
		RunnerServiceID:         "runner-service",
	}, req)
	if err != nil {
		t.Fatalf("buildKubernetesRunnerManifestResponse() error = %v", err)
	}
	if resp.DispatcherAddress != "nopsai-dispatcher.pre-nopsai.orb.local:9090" {
		t.Fatalf("dispatcher address = %q, want explicit override", resp.DispatcherAddress)
	}
	if !strings.Contains(resp.Manifest, "DISPATCHER_ADDRESS: nopsai-dispatcher.pre-nopsai.orb.local:9090") {
		t.Fatalf("manifest missing explicit dispatcher address:\n%s", resp.Manifest)
	}
	if len(resp.Warnings) == 0 {
		t.Fatal("warnings should explain dispatcher address override")
	}
}

func TestBuildKubernetesRunnerBootstrapCommandResponseUsesOneTimeScriptToken(t *testing.T) {
	app := App{}
	req := httptest.NewRequest(http.MethodGet, "http://nopsai.example.com/v1/system/dispatcher/kubernetes-runner-bootstrap-command?runner_id=k8s-runner-ams-1&runner_scopes=production,eu-west&runner_capacity=30&namespace=nopsai-runs&service_account=nopsai-runner&storage_class=fast-rwo", nil)
	resp, err := runnerinstall.BuildKubernetesBootstrapCommandResponse(config.Config{
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
		AgentImage:              "nopsai-agent:dev",
	}, req, app.createRunnerBootstrapToken)
	if err != nil {
		t.Fatalf("buildKubernetesRunnerBootstrapCommandResponse() error = %v", err)
	}
	if strings.Contains(resp.BootstrapCommand, "service-secret") || strings.Contains(resp.BootstrapCommand, "tls-secret") {
		t.Fatalf("bootstrap command should not expose long-lived secrets: %s", resp.BootstrapCommand)
	}
	if !strings.Contains(resp.BootstrapCommand, "curl -fsSL") || !strings.Contains(resp.BootstrapCommand, "sh \"$tmp\"") {
		t.Fatalf("bootstrap command should download and execute the one-time script: %s", resp.BootstrapCommand)
	}
	if strings.Contains(resp.BootstrapCommand, "kubectl apply -f") || strings.Contains(resp.BootstrapCommand, "rollout status deployment/") {
		t.Fatalf("bootstrap command should keep Kubernetes details inside the downloaded script: %s", resp.BootstrapCommand)
	}
	if resp.Namespace != "nopsai-runs" || resp.ServiceAccount != "nopsai-runner" {
		t.Fatalf("namespace/service account = %q/%q", resp.Namespace, resp.ServiceAccount)
	}
	const marker = "token="
	idx := strings.Index(resp.BootstrapCommand, marker)
	if idx < 0 {
		t.Fatalf("bootstrap command missing token: %s", resp.BootstrapCommand)
	}
	rest := resp.BootstrapCommand[idx+len(marker):]
	end := strings.Index(rest, "'")
	if end < 0 {
		t.Fatalf("bootstrap command token is not single-quoted: %s", resp.BootstrapCommand)
	}
	token := strings.TrimSpace(rest[:end])
	entry, ok := app.consumeRunnerBootstrapToken(token)
	if !ok {
		t.Fatal("expected bootstrap token to be consumable")
	}
	if entry.ContentType != "text/x-shellscript; charset=utf-8" {
		t.Fatalf("content type = %q, want Kubernetes install script", entry.ContentType)
	}
	if !strings.Contains(entry.Content, "kind: Deployment") || !strings.Contains(entry.Content, "SERVICE_JWT_SIGNING_KEY: service-secret") {
		t.Fatalf("bootstrap script should include runner resources and secrets:\n%s", entry.Content)
	}
	if !strings.Contains(entry.Content, "kubectl apply -f \"$tmp\"") || !strings.Contains(entry.Content, "rollout status deployment/") || !strings.Contains(entry.Content, "logs deployment/") {
		t.Fatalf("bootstrap script should apply the manifest and show rollout diagnostics:\n%s", entry.Content)
	}
	if _, ok := app.consumeRunnerBootstrapToken(token); ok {
		t.Fatal("bootstrap token should be single-use")
	}
}

func TestBuildConfigRepositoryInputDefaultsAndValidation(t *testing.T) {
	input, err := configsync.BuildRepositoryInput(configsync.RepositoryInputRequest{
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
	if input.WriteEnabled {
		t.Fatal("WriteEnabled = true, want false")
	}

	if _, err := configsync.BuildRepositoryInput(configsync.RepositoryInputRequest{
		RepoURL:  "https://github.com/acme/configs.git",
		BasePath: "../outside",
	}, models.ConfigRepositoryScopeFolder, "team-1", "user-1"); err == nil {
		t.Fatal("buildConfigRepositoryInput() accepted escaping base_path")
	}

	input, err = configsync.BuildRepositoryInput(configsync.RepositoryInputRequest{
		RepoURL:      "https://github.com/acme/configs.git",
		WriteEnabled: boolPointer(true),
	}, models.ConfigRepositoryScopeFolder, "team-1", "user-1")
	if err != nil {
		t.Fatalf("buildConfigRepositoryInput() with write enabled error = %v", err)
	}
	if !input.WriteEnabled || input.WriteBranch != "nopsai/ui-changes" {
		t.Fatalf("write settings = (%v, %q), want (true, nopsai/ui-changes)", input.WriteEnabled, input.WriteBranch)
	}

	if _, err := configsync.BuildRepositoryInput(configsync.RepositoryInputRequest{
		RepoURL:     "https://github.com/acme/configs.git",
		WriteBranch: "bad branch",
	}, models.ConfigRepositoryScopeFolder, "team-1", "user-1"); err == nil {
		t.Fatal("buildConfigRepositoryInput() accepted invalid write_branch")
	}
}

func TestCleanConfigRepositoryWritePath(t *testing.T) {
	got, err := configsync.CleanRepositoryWritePath("nopsai", "/pipelines/build.yaml")
	if err != nil {
		t.Fatalf("cleanConfigRepositoryWritePath() error = %v", err)
	}
	if got != "nopsai/pipelines/build.yaml" {
		t.Fatalf("path = %q, want nopsai/pipelines/build.yaml", got)
	}
	if _, err := configsync.CleanRepositoryWritePath("", "../outside.yaml"); err == nil {
		t.Fatal("cleanConfigRepositoryWritePath() accepted escaping path")
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
			gotScopeType, gotScopeID, err := configsync.ParseBindingPath(tt.relPath)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("configsync.ParseBindingPath() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("configsync.ParseBindingPath() error = %v", err)
			}
			if gotScopeType != tt.wantScopeType || gotScopeID != tt.wantScopeID {
				t.Fatalf("configsync.ParseBindingPath() = (%q, %q), want (%q, %q)", gotScopeType, gotScopeID, tt.wantScopeType, tt.wantScopeID)
			}
		})
	}
}

func TestNormalizePipelineRunStructureForFolder(t *testing.T) {
	structure := map[string]*configsync.PipelineRunStructureNode{
		"dev": {
			Description: "Development",
			Repos:       []string{"acme/app"},
			Children: map[string]*configsync.PipelineRunStructureNode{
				"services": {
					Description: "Service workloads",
					Children:    map[string]*configsync.PipelineRunStructureNode{},
				},
			},
		},
		"team-1": {
			Description: "Already scoped",
			Children: map[string]*configsync.PipelineRunStructureNode{
				"platform": {
					Description: "Platform",
					Children:    map[string]*configsync.PipelineRunStructureNode{},
				},
			},
		},
	}

	got, err := configsync.NormalizePipelineRunStructureForFolder("team-1", structure)
	if err != nil {
		t.Fatalf("configsync.NormalizePipelineRunStructureForFolder() error = %v", err)
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

	if !configsync.CanRepositoryWriteOver(parentRepo, systemRepo, "team-1/build") {
		t.Fatal("group repo should be able to take over parent-managed global resources")
	}
	if !configsync.CanRepositoryWriteOver(childRepo, parentRepo, "team-1/dev/deploy") {
		t.Fatal("child group repo should be able to take over parent group resources in its subtree")
	}
	if configsync.CanRepositoryWriteOver(parentRepo, childRepo, "team-1/dev/deploy") {
		t.Fatal("parent group repo should not take over child group resources")
	}
	if !configsync.RepositoryShadowsCurrent(childRepo, parentRepo, "team-1/dev/deploy") {
		t.Fatal("child group repo should shadow parent group repo in its subtree")
	}
	if configsync.CanRepositoryWriteOver(otherRepo, parentRepo, "team-1/build") {
		t.Fatal("unrelated group repo should not take over another group")
	}
}

func TestConfigRepositoryAdoptsOnlyInScopeDatabaseResources(t *testing.T) {
	systemRepo := models.ConfigRepository{ID: 1, ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	folderRepo := models.ConfigRepository{ID: 2, ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"}

	if !configsync.CanRepositoryAdoptUnmanagedResource(systemRepo, "platform/deploy") {
		t.Fatal("system config repo should be able to adopt database resources")
	}
	if !configsync.CanRepositoryAdoptUnmanagedResource(folderRepo, "team-1/deploy") {
		t.Fatal("group config repo should be able to adopt database resources inside its group")
	}
	if !configsync.CanRepositoryAdoptUnmanagedResource(folderRepo, "team-1/dev/deploy") {
		t.Fatal("group config repo should be able to adopt database resources inside child groups")
	}
	if configsync.CanRepositoryAdoptUnmanagedResource(folderRepo, "team-2/deploy") {
		t.Fatal("group config repo should not adopt database resources outside its group")
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

	got, err := effectivePipelineRunStructureForConfigSync(binding, configRepositories, nil)
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
}

func TestEffectivePipelineRunStructureForGroupUsesConfigRepositoryStructure(t *testing.T) {
	binding := models.ConfigRepository{ID: 2, ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"}
	configRepositories := map[string]storedConfigRepository{
		"folder/team-1/platform": {
			scopeType: models.ConfigRepositoryScopeFolder,
			scopeID:   "team-1/platform",
			enabled:   true,
		},
	}
	structure := map[string]*configsync.PipelineRunStructureNode{
		"team-1": {
			Description: "Owned by team-1 repo",
			Children: map[string]*configsync.PipelineRunStructureNode{
				"dev":      {Description: "Owned by team-1 repo", Children: map[string]*configsync.PipelineRunStructureNode{}},
				"platform": {Description: "Platform shell", Children: map[string]*configsync.PipelineRunStructureNode{}},
			},
		},
	}

	got, err := effectivePipelineRunStructureForConfigSync(binding, configRepositories, structure)
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
	if platform.Description != "Platform shell" || len(platform.Children) != 0 {
		t.Fatalf("platform structure = %#v, want colocated structure", platform)
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
	groupStructure, ok, err := configsync.ParseConfigRepositoryGroupPipelineRunStructure("groups/team-1/structure.yaml", `
description: Team 1 apps
apps:
  - name: test-app
    repo_url: https://github.com/hosein-yousefii/test-app
dev:
  apps:
    - name: dev-app
      repo_url: https://github.com/hosein-yousefii/dev-app
`)
	if err != nil {
		t.Fatalf("configsync.ParseConfigRepositoryGroupPipelineRunStructure() error = %v", err)
	}
	if !ok {
		t.Fatal("expected groups/team-1/structure.yaml to be treated as a group structure file")
	}

	got, err := effectivePipelineRunStructureForConfigSync(binding, configRepositories, groupStructure)
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
	if len(team1.Apps) != 1 || team1.Apps[0].RepositoryFullName != "hosein-yousefii/test-app" {
		t.Fatalf("team-1 apps = %#v, want hosein-yousefii/test-app", team1.Apps)
	}
	dev, ok := team1.Children["dev"]
	if !ok {
		t.Fatal("expected team-1/dev from config-repositories group structure")
	}
	if len(dev.Apps) != 1 || dev.Apps[0].RepositoryFullName != "hosein-yousefii/dev-app" {
		t.Fatalf("team-1/dev apps = %#v, want hosein-yousefii/dev-app", dev.Apps)
	}
}

func TestConfigRepositoryGroupStructureCollectsInlineConfig(t *testing.T) {
	structure, ok, err := configsync.ParseConfigRepositoryGroupPipelineRunStructure("groups/data-team/structure.yaml", `
description: Owns data-team scoped configuration
config:
  repo_url: git@github.com:hosein-yousefii/nopsai-data-team-config.git
  branch: main
  base_path: ""
  enabled: true
  write_enabled: true
  write_branch: nopsai/data-team-ui
`)
	if err != nil {
		t.Fatalf("configsync.ParseConfigRepositoryGroupPipelineRunStructure() error = %v", err)
	}
	if !ok {
		t.Fatal("expected groups/data-team/structure.yaml to be treated as a group structure file")
	}

	bindings, err := configRepositoryBindingsFromPipelineRunStructure(structure, "config-repositories/groups/data-team/structure.yaml")
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
	if !binding.writeEnabled || binding.writeBranch != "nopsai/data-team-ui" {
		t.Fatalf("binding write settings = (%v, %q), want (true, nopsai/data-team-ui)", binding.writeEnabled, binding.writeBranch)
	}
}

func TestConfigRepositoryGroupStructureParsesAppsWithRepositoryURLs(t *testing.T) {
	structure, ok, err := configsync.ParseConfigRepositoryGroupPipelineRunStructure("groups/team-1/structure.yaml", `
description: Team 1 apps
apps:
  - name: api
    repo_url: https://github.com/acme/service-api.git
  - name: worker
    repo_url: git@github.com:acme/worker.git
`)
	if err != nil {
		t.Fatalf("configsync.ParseConfigRepositoryGroupPipelineRunStructure() error = %v", err)
	}
	if !ok {
		t.Fatal("expected groups/team-1/structure.yaml to be treated as a group structure file")
	}
	team1 := structure["team-1"]
	if team1 == nil {
		t.Fatal("expected team-1 structure")
	}
	if len(team1.Apps) != 2 {
		t.Fatalf("apps = %#v, want 2 apps", team1.Apps)
	}
	if team1.Apps[0].Name != "api" || team1.Apps[0].RepositoryFullName != "acme/service-api" {
		t.Fatalf("first app = %#v, want api -> acme/service-api", team1.Apps[0])
	}
	if team1.Apps[1].Name != "worker" || team1.Apps[1].RepositoryFullName != "acme/worker" {
		t.Fatalf("second app = %#v, want worker -> acme/worker", team1.Apps[1])
	}
}

func TestRepositoryFullNameFromURLUsesGitHubRepoRoot(t *testing.T) {
	got, err := configsync.RepositoryFullNameFromURL("https://github.com/acme/service-api/tree/main")
	if err != nil {
		t.Fatalf("repositoryFullNameFromURL() error = %v", err)
	}
	if got != "acme/service-api" {
		t.Fatalf("repositoryFullNameFromURL() = %q, want acme/service-api", got)
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
	externalTriggers := map[string]storedExternalTrigger{
		"data-team-deploy": {input: externalTriggerRecord{ID: "data-team-deploy", Pipeline: "data-team/deploy", Scope: "data-team/dev", RunGroupPath: "data-team/dev"}},
		"prod-deploy":      {input: externalTriggerRecord{ID: "prod-deploy", Pipeline: "platform/deploy", Scope: "prod", RunGroupPath: "root"}},
	}

	filterDelegatedConfigResources(
		binding,
		[]string{"data-team"},
		map[string]storedPipeline{},
		map[string]storedStep{},
		map[string]storedSchedule{},
		externalTriggers,
		map[string]storedNotificationRoute{},
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
	if _, ok := externalTriggers["data-team-deploy"]; ok {
		t.Fatal("expected delegated external trigger to be filtered")
	}
	if _, ok := externalTriggers["prod-deploy"]; !ok {
		t.Fatal("expected unrelated external trigger to remain")
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

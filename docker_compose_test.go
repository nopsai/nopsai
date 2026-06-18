package nopsai

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type composeDocument struct {
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	ContainerName string            `yaml:"container_name"`
	Hostname      string            `yaml:"hostname"`
	Environment   map[string]string `yaml:"environment"`
	EnvFile       []string          `yaml:"env_file"`
}

func TestDockerComposeDoesNotDependOnTrackedEnvFile(t *testing.T) {
	contents, err := os.ReadFile("docker-compose.yaml")
	if err != nil {
		t.Fatalf("read docker-compose.yaml: %v", err)
	}
	if strings.Contains(string(contents), "env_file:") {
		t.Fatal("docker-compose.yaml should use per-service environment blocks, not env_file")
	}

	var compose composeDocument
	if err := yaml.Unmarshal(contents, &compose); err != nil {
		t.Fatalf("parse docker-compose.yaml: %v", err)
	}
	for name, service := range compose.Services {
		if len(service.EnvFile) > 0 {
			t.Fatalf("service %q still uses env_file: %#v", name, service.EnvFile)
		}
	}
}

func TestDockerComposeNamesDockerRunnerExplicitly(t *testing.T) {
	compose := readCompose(t)

	if _, exists := compose.Services["runner"]; exists {
		t.Fatal("compose service should be named docker-runner, not runner")
	}

	service, exists := compose.Services["docker-runner"]
	if !exists {
		t.Fatal("compose service docker-runner is missing")
	}
	if service.ContainerName != "nopsai-docker-runner" {
		t.Fatalf("docker-runner container_name = %q, want nopsai-docker-runner", service.ContainerName)
	}
	if service.Hostname != "docker-runner" {
		t.Fatalf("docker-runner hostname = %q, want docker-runner", service.Hostname)
	}
}

func TestTrackedEnvFileIsDocumentationOnly(t *testing.T) {
	contents, err := os.ReadFile(".env")
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	for lineNumber, raw := range strings.Split(string(contents), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "=") {
			t.Fatalf(".env line %d contains an active assignment: %s", lineNumber+1, line)
		}
	}
}

func TestProductRuntimeSettingsStayOutOfComposeEnvironment(t *testing.T) {
	compose := readCompose(t)
	runtimeManagedKeys := []string{
		"AAA_ADDR",
		"AGENT_IMAGE",
		"AUTO_REMOVAL_AGENT_CONTAINER",
		"AUTH_PROVIDER_LOCAL_ENABLED",
		"CONFIG_PATH",
		"DEFAULT_PIPELINE_TIMEOUT",
		"DISPATCHER_LISTEN_ADDRESS",
		"DISPATCHER_ROUTING",
		"DISPATCHER_TLS_MODE",
		"DISPATCHER_TLS_SECRET",
		"DISPATCHER_TLS_SERVER_NAME",
		"GITHUB_APP_ID",
		"GITHUB_INSTALLATION_ID",
		"GITHUB_PRIVATE_KEY",
		"GITHUB_PRIVATE_KEY_PATH",
		"GITHUB_WEBHOOK_SECRET",
		"JWT_AUDIENCE",
		"JWT_ISSUER",
		"LLM_AGENT_TIMEOUT",
		"LOG_FORMAT",
		"LOG_LEVEL",
		"NOPSAI_LISTEN_ADDRESS",
		"NOPSAI_PUBLIC_URL",
		"RUNNER_CAPACITY",
		"RUNNER_ID",
		"RUNNER_SCOPES",
		"SERVICE_JWT_AUDIENCE",
		"SERVICE_JWT_ISSUER",
	}

	for serviceName, service := range compose.Services {
		for _, key := range runtimeManagedKeys {
			if _, exists := service.Environment[key]; exists {
				t.Fatalf("service %q should not set runtime-managed environment variable %s", serviceName, key)
			}
		}
	}
}

func TestDockerComposeProvidesLocalBootstrapTopology(t *testing.T) {
	compose := readCompose(t)
	assertEnvValue(t, compose, "nopsai", "AGENT_NOPSAI_API_URL", "http://nopsai:8080")
	assertEnvValue(t, compose, "nopsai", "DISPATCHER_ADDRESS", "dispatcher:9090")
	assertEnvValue(t, compose, "nopsai", "DOCKER_NETWORK_NAME", "nopsai-net")
	assertEnvValue(t, compose, "nopsai", "NOPSAI_GIT_BOT_API_URL", "http://nopsai-git-bot:8081")
	assertEnvValue(t, compose, "dispatcher", "AGENT_NOPSAI_API_URL", "http://nopsai:8080")
	assertEnvValue(t, compose, "git-bot", "GIT_BOT_NOPSAI_API_URL", "http://nopsai:8080")
	assertEnvValue(t, compose, "docker-runner", "DISPATCHER_ADDRESS", "dispatcher:9090")
	assertEnvValue(t, compose, "docker-runner", "DOCKER_NETWORK_NAME", "nopsai-net")
}

func TestProductRuntimeSettingsStayOutOfConfigYAML(t *testing.T) {
	contents, err := os.ReadFile("config.yml")
	if err != nil {
		t.Fatalf("read config.yml: %v", err)
	}
	var cfg map[string]interface{}
	if err := yaml.Unmarshal(contents, &cfg); err != nil {
		t.Fatalf("parse config.yml: %v", err)
	}

	for _, key := range []string{
		"agent_image",
		"agent_nopsai_api_url",
		"auto_removal_agent_container",
		"default_pipeline_timeout",
		"dispatcher_address",
		"dispatcher_routing",
		"docker_network_name",
		"github_app_id",
		"github_installation_id",
		"github_private_key",
		"github_private_key_path",
		"github_webhook_secret",
		"git_bot_nopsai_api_url",
		"llm_agent_timeout",
		"log_format",
		"log_level",
		"nopsai_git_bot_api_url",
		"public_url",
		"runner_capacity",
		"runner_id",
		"runner_scopes",
		"runtime",
		"runtime_pools",
	} {
		if _, exists := cfg[key]; exists {
			t.Fatalf("config.yml should not define product/runtime setting %s", key)
		}
	}
}

func assertEnvValue(t *testing.T, compose composeDocument, serviceName, key, want string) {
	t.Helper()
	service, exists := compose.Services[serviceName]
	if !exists {
		t.Fatalf("compose service %q is missing", serviceName)
	}
	if got := service.Environment[key]; got != want {
		t.Fatalf("service %q environment %s = %q, want %q", serviceName, key, got, want)
	}
}

func readCompose(t *testing.T) composeDocument {
	t.Helper()

	contents, err := os.ReadFile("docker-compose.yaml")
	if err != nil {
		t.Fatalf("read docker-compose.yaml: %v", err)
	}
	var compose composeDocument
	if err := yaml.Unmarshal(contents, &compose); err != nil {
		t.Fatalf("parse docker-compose.yaml: %v", err)
	}
	return compose
}

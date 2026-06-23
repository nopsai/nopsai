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
	Image         string            `yaml:"image"`
	Build         composeBuild      `yaml:"build"`
	Hostname      string            `yaml:"hostname"`
	Environment   map[string]string `yaml:"environment"`
	EnvFile       []string          `yaml:"env_file"`
	Volumes       []string          `yaml:"volumes"`
	ReadOnly      bool              `yaml:"read_only"`
	SecurityOpt   []string          `yaml:"security_opt"`
	CapDrop       []string          `yaml:"cap_drop"`
}

type composeBuild struct {
	AdditionalContexts map[string]string `yaml:"additional_contexts"`
	Args               map[string]string `yaml:"args"`
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
		"AAA_LISTEN_ADDRESS",
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
	assertEnvValue(t, compose, "nopsai", "NOPSAI_API_URL", "http://nopsai:8080")
	assertEnvValue(t, compose, "nopsai", "DISPATCHER_GRPC_ADDRESS", "dispatcher:9090")
	assertEnvValue(t, compose, "nopsai", "DOCKER_NETWORK_NAME", "nopsai-net")
	assertEnvValue(t, compose, "nopsai", "GIT_BOT_API_URL", "http://nopsai-git-bot:8081")
	assertEnvValue(t, compose, "dispatcher", "NOPSAI_API_URL", "http://nopsai:8080")
	assertEnvValue(t, compose, "git-bot", "NOPSAI_API_URL", "http://nopsai:8080")
	assertEnvValue(t, compose, "docker-runner", "DISPATCHER_GRPC_ADDRESS", "dispatcher:9090")
	assertEnvValue(t, compose, "docker-runner", "DOCKER_NETWORK_NAME", "nopsai-net")
	assertEnvValue(t, compose, "nopsai", "SYSTEM_LOGS_DOCKER_HOST", "tcp://docker-socket-proxy:2375")
}

func TestDockerComposeBuildsBaseConsumersFromBaseServiceContext(t *testing.T) {
	compose := readCompose(t)
	for _, serviceName := range []string{"nopsai", "aaa", "dispatcher", "git-bot", "agent", "docker-runner", "k8s-runner"} {
		service, exists := compose.Services[serviceName]
		if !exists {
			t.Fatalf("compose service %q is missing", serviceName)
		}
		if got := service.Build.Args["BASE_IMAGE"]; got != "base" {
			t.Fatalf("service %q BASE_IMAGE = %q, want BuildKit service context", serviceName, got)
		}
		if got := service.Build.AdditionalContexts["base"]; got != "service:base" {
			t.Fatalf("service %q base build context = %q, want service:base", serviceName, got)
		}
	}
}

func TestDockerComposeUsesReadOnlySocketProxyForSystemLogs(t *testing.T) {
	compose := readCompose(t)
	proxy, exists := compose.Services["docker-socket-proxy"]
	if !exists {
		t.Fatal("docker-socket-proxy service is missing")
	}
	if proxy.ContainerName != "nopsai-docker-socket-proxy" || !strings.Contains(proxy.Image, "nopsai-docker-socket-proxy:${NOPSAI_VERSION:-dev}") {
		t.Fatalf("socket proxy identity = %q %q", proxy.ContainerName, proxy.Image)
	}
	if !proxy.ReadOnly || !containsString(proxy.SecurityOpt, "no-new-privileges:true") || !containsString(proxy.CapDrop, "ALL") {
		t.Fatalf("socket proxy hardening is incomplete: %#v", proxy)
	}
	if !containsString(proxy.Volumes, "/var/run/docker.sock:/var/run/docker.sock:ro") {
		t.Fatalf("socket proxy volume = %#v, want read-only Docker socket", proxy.Volumes)
	}
	if proxy.Environment["ALLOWED_CONTAINERS"] != "nopsai,nopsai-aaa,nopsai-dispatcher,nopsai-git-bot,nopsai-ui,nopsai-docker-runner,nopsai-k8s-runner" {
		t.Fatalf("socket proxy allow-list = %q", proxy.Environment["ALLOWED_CONTAINERS"])
	}
	if nopsai := compose.Services["nopsai"]; containsSubstring(nopsai.Volumes, "docker.sock") {
		t.Fatal("nopsai service must not mount the Docker socket")
	}
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
		"auto_removal_agent_container",
		"default_pipeline_timeout",
		"dispatcher_grpc_address",
		"dispatcher_routing",
		"docker_network_name",
		"github_app_id",
		"github_installation_id",
		"github_private_key",
		"github_private_key_path",
		"github_webhook_secret",
		"git_bot_api_url",
		"llm_agent_timeout",
		"log_format",
		"log_level",
		"nopsai_api_url",
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsSubstring(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}

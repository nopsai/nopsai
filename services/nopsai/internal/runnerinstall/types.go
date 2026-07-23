package runnerinstall

import (
	"context"
	"strings"
	"time"

	"nopsai/pkg/buildinfo"
)

const (
	NetworkModeBridge  = "bridge"
	NetworkModeHost    = "host"
	RunnerImageRepo    = "ghcr.io/hosein-yousefii/nopsai-docker-runner"
	K8sRunnerImageRepo = "ghcr.io/hosein-yousefii/nopsai-k8s-runner"
)

func DefaultRunnerImage() string {
	return versionedRunnerImage(RunnerImageRepo)
}

func DefaultK8sImage() string {
	return versionedRunnerImage(K8sRunnerImageRepo)
}

func versionedRunnerImage(repository string) string {
	tag := strings.TrimSpace(buildinfo.Current().Version)
	if tag == "" || strings.EqualFold(tag, "unknown") || strings.EqualFold(tag, "latest") {
		tag = buildinfo.DevelopmentVersion
	}
	return strings.TrimRight(strings.TrimSpace(repository), ":") + ":" + tag
}

type ComposeResponse struct {
	RunnerID          string   `json:"runner_id"`
	RunnerScopes      string   `json:"runner_scopes"`
	RunnerCapacity    int      `json:"runner_capacity"`
	DispatcherAddress string   `json:"dispatcher_grpc_address"`
	NetworkMode       string   `json:"network_mode"`
	RunnerImage       string   `json:"runner_image"`
	Compose           string   `json:"compose"`
	Command           string   `json:"command"`
	Warnings          []string `json:"warnings,omitempty"`
}

type BootstrapCommandResponse struct {
	RunnerID            string    `json:"runner_id"`
	RunnerScopes        string    `json:"runner_scopes"`
	RunnerCapacity      int       `json:"runner_capacity"`
	DispatcherAddress   string    `json:"dispatcher_grpc_address"`
	NetworkMode         string    `json:"network_mode"`
	RunnerImage         string    `json:"runner_image"`
	RegistryCredentials []string  `json:"registry_credential_refs,omitempty"`
	RegistryHosts       []string  `json:"registry_hosts,omitempty"`
	BootstrapCommand    string    `json:"bootstrap_command"`
	ExpiresAt           time.Time `json:"expires_at"`
	Warnings            []string  `json:"warnings,omitempty"`
}

type KubernetesManifestResponse struct {
	RunnerID            string   `json:"runner_id"`
	RunnerScopes        string   `json:"runner_scopes"`
	RunnerCapacity      int      `json:"runner_capacity"`
	Namespace           string   `json:"namespace"`
	ServiceAccount      string   `json:"service_account"`
	DispatcherAddress   string   `json:"dispatcher_grpc_address"`
	RunnerImage         string   `json:"runner_image"`
	RegistryCredentials []string `json:"registry_credential_refs,omitempty"`
	RegistryHosts       []string `json:"registry_hosts,omitempty"`
	Manifest            string   `json:"manifest"`
	Command             string   `json:"command"`
	Warnings            []string `json:"warnings,omitempty"`
}

type KubernetesBootstrapCommandResponse struct {
	RunnerID            string    `json:"runner_id"`
	RunnerScopes        string    `json:"runner_scopes"`
	RunnerCapacity      int       `json:"runner_capacity"`
	Namespace           string    `json:"namespace"`
	ServiceAccount      string    `json:"service_account"`
	DispatcherAddress   string    `json:"dispatcher_grpc_address"`
	RunnerImage         string    `json:"runner_image"`
	RegistryCredentials []string  `json:"registry_credential_refs,omitempty"`
	RegistryHosts       []string  `json:"registry_hosts,omitempty"`
	BootstrapCommand    string    `json:"bootstrap_command"`
	ExpiresAt           time.Time `json:"expires_at"`
	Warnings            []string  `json:"warnings,omitempty"`
}

type BootstrapToken struct {
	Content        string
	ContentType    string
	ExpiresAt      time.Time
	ContentBuilder BootstrapContentBuilder
}

type TokenIssuer func(content string, ttl time.Duration, contentType string) (string, time.Time, error)

type BootstrapContentBuilder func(ctx context.Context) (content string, contentType string, err error)

type BootstrapOptions struct {
	RegistryAuth RegistryAuthBootstrap
}

type RegistryAuthBootstrap struct {
	DockerConfigJSON []byte
	CredentialRefs   []string
	RegistryHosts    []string
}

type installEnv struct {
	key   string
	value string
}

type installSpec struct {
	RunnerID          string
	RunnerScopes      string
	RunnerCapacity    int
	DispatcherAddress string
	ServiceName       string
	DockerNetwork     string
	NetworkMode       string
	RunnerImage       string
	IncludeNetwork    bool
	Env               []installEnv
	RegistryAuth      RegistryAuthBootstrap
	Warnings          []string
}

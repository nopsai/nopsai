package runnerinstall

import "time"

const (
	NetworkModeBridge  = "bridge"
	NetworkModeHost    = "host"
	DefaultRunnerImage = "hoseindocker/nopsai-runner:latest"
	DefaultK8sImage    = "hoseindocker/nopsai-k8s-runner:latest"
)

type ComposeResponse struct {
	RunnerID          string   `json:"runner_id"`
	RunnerScopes      string   `json:"runner_scopes"`
	RunnerCapacity    int      `json:"runner_capacity"`
	DispatcherAddress string   `json:"dispatcher_address"`
	NetworkMode       string   `json:"network_mode"`
	RunnerImage       string   `json:"runner_image"`
	Compose           string   `json:"compose"`
	Command           string   `json:"command"`
	Warnings          []string `json:"warnings,omitempty"`
}

type BootstrapCommandResponse struct {
	RunnerID          string    `json:"runner_id"`
	RunnerScopes      string    `json:"runner_scopes"`
	RunnerCapacity    int       `json:"runner_capacity"`
	DispatcherAddress string    `json:"dispatcher_address"`
	NetworkMode       string    `json:"network_mode"`
	RunnerImage       string    `json:"runner_image"`
	BootstrapCommand  string    `json:"bootstrap_command"`
	ExpiresAt         time.Time `json:"expires_at"`
	Warnings          []string  `json:"warnings,omitempty"`
}

type KubernetesManifestResponse struct {
	RunnerID          string   `json:"runner_id"`
	RunnerScopes      string   `json:"runner_scopes"`
	RunnerCapacity    int      `json:"runner_capacity"`
	Namespace         string   `json:"namespace"`
	ServiceAccount    string   `json:"service_account"`
	DispatcherAddress string   `json:"dispatcher_address"`
	RunnerImage       string   `json:"runner_image"`
	Manifest          string   `json:"manifest"`
	Command           string   `json:"command"`
	Warnings          []string `json:"warnings,omitempty"`
}

type KubernetesBootstrapCommandResponse struct {
	RunnerID          string    `json:"runner_id"`
	RunnerScopes      string    `json:"runner_scopes"`
	RunnerCapacity    int       `json:"runner_capacity"`
	Namespace         string    `json:"namespace"`
	ServiceAccount    string    `json:"service_account"`
	DispatcherAddress string    `json:"dispatcher_address"`
	RunnerImage       string    `json:"runner_image"`
	BootstrapCommand  string    `json:"bootstrap_command"`
	ExpiresAt         time.Time `json:"expires_at"`
	Warnings          []string  `json:"warnings,omitempty"`
}

type BootstrapToken struct {
	Content     string
	ContentType string
	ExpiresAt   time.Time
}

type TokenIssuer func(content string, ttl time.Duration, contentType string) (string, time.Time, error)

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
	Warnings          []string
}

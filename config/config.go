package config

import (
	"os"
	"reflect"
	"strconv"
	"strings"

	"nopsai/pkg/models"

	"gopkg.in/yaml.v3"
)

const (
	LLMProviderGemini   = "gemini"
	LLMProviderLMStudio = "lmstudio"

	DefaultLLMProfileName = "standard"
)

var validLMStudioReasoningLevels = map[string]struct{}{
	"":       {},
	"off":    {},
	"low":    {},
	"medium": {},
	"high":   {},
	"on":     {},
}

type LLMProfile struct {
	Provider      string   `yaml:"provider" json:"provider"`
	Model         string   `yaml:"model,omitempty" json:"model,omitempty"`
	BaseURL       string   `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	APIKeySecret  string   `yaml:"api_key_secret,omitempty" json:"api_key_secret,omitempty"`
	AllowedScopes []string `yaml:"allowed_scopes,omitempty" json:"allowed_scopes,omitempty"`
	Reasoning     string   `yaml:"reasoning,omitempty" json:"reasoning,omitempty"`
	Thinking      *bool    `yaml:"thinking,omitempty" json:"thinking,omitempty"`
}

type KubernetesConfig struct {
	Namespace                  string            `yaml:"namespace" json:"namespace,omitempty"`
	ServiceAccount             string            `yaml:"service_account" json:"service_account,omitempty"`
	DefaultImagePullPolicy     string            `yaml:"default_image_pull_policy" json:"default_image_pull_policy,omitempty"`
	DefaultWorkspaceSize       string            `yaml:"default_workspace_size" json:"default_workspace_size,omitempty"`
	DefaultWorkspaceAccessMode string            `yaml:"default_workspace_access_mode" json:"default_workspace_access_mode,omitempty"`
	DefaultTaskTimeout         string            `yaml:"default_task_timeout" json:"default_task_timeout,omitempty"`
	DefaultRunTimeout          string            `yaml:"default_run_timeout" json:"default_run_timeout,omitempty"`
	WorkspaceVolumeMode        string            `yaml:"workspace_volume_mode" json:"workspace_volume_mode,omitempty"`
	ExistingWorkspacePVC       string            `yaml:"existing_workspace_pvc" json:"existing_workspace_pvc,omitempty"`
	StorageClass               string            `yaml:"storage_class" json:"storage_class,omitempty"`
	AffinityEnabled            *bool             `yaml:"affinity_enabled" json:"affinity_enabled,omitempty"`
	CleanupFinishedPods        *bool             `yaml:"cleanup_finished_pods" json:"cleanup_finished_pods,omitempty"`
	PodLabels                  map[string]string `yaml:"pod_labels" json:"pod_labels,omitempty"`
	PodAnnotations             map[string]string `yaml:"pod_annotations" json:"pod_annotations,omitempty"`
}

type RunnerLimits struct {
	MaxConcurrentRuns        int `yaml:"max_concurrent_runs" json:"max_concurrent_runs,omitempty"`
	MaxConcurrentTasks       int `yaml:"max_concurrent_tasks" json:"max_concurrent_tasks,omitempty"`
	MaxConcurrentTasksPerRun int `yaml:"max_concurrent_tasks_per_run" json:"max_concurrent_tasks_per_run,omitempty"`
	MaxPendingTasks          int `yaml:"max_pending_tasks" json:"max_pending_tasks,omitempty"`
}

type RuntimePool struct {
	NodeSelector map[string]string    `yaml:"node_selector" json:"node_selector,omitempty"`
	Resources    RuntimePoolResources `yaml:"resources" json:"resources,omitempty"`
}

type RuntimePoolResources struct {
	Requests map[string]string `yaml:"requests" json:"requests,omitempty"`
	Limits   map[string]string `yaml:"limits" json:"limits,omitempty"`
}

// Config holds all configuration for the application.
type Config struct {
	DatabaseURL string `yaml:"database_url" env:"DATABASE_URL"`
	LogLevel    string `yaml:"log_level" env:"LOG_LEVEL"`
	LogFormat   string `yaml:"log_format" env:"LOG_FORMAT"`

	MasterKey string `yaml:"master_key" env:"NOPSAI_MASTER_KEY"`

	// Authentication and authorization
	JWTSigningKey            string `yaml:"jwt_signing_key" env:"JWT_SIGNING_KEY"`
	JWTRSAKeyPath            string `yaml:"jwt_rsa_key_path" env:"JWT_RSA_KEY_PATH"`
	JWTIssuer                string `yaml:"jwt_issuer" env:"JWT_ISSUER"`
	JWTAudience              string `yaml:"jwt_audience" env:"JWT_AUDIENCE"`
	JWTExpiryMinutes         int    `yaml:"jwt_expiry_minutes" env:"JWT_EXPIRY_MINUTES"`
	IdleTimeoutMinutes       int    `yaml:"idle_timeout_minutes" env:"IDLE_TIMEOUT_MINUTES"`
	RefreshTokenTTLMinutes   int    `yaml:"refresh_token_ttl_minutes" env:"REFRESH_TOKEN_TTL_MINUTES"`
	AuthProviderLocalEnabled bool   `yaml:"auth_provider_local_enabled" env:"AUTH_PROVIDER_LOCAL_ENABLED"`
	RateLimitLoginPerMinute  int    `yaml:"rate_limit_login_per_minute" env:"RATE_LIMIT_LOGIN_PER_MINUTE"`
	LoginLockoutThreshold    int    `yaml:"login_lockout_threshold" env:"LOGIN_LOCKOUT_THRESHOLD"`
	LoginLockoutWindowMin    int    `yaml:"login_lockout_window_minutes" env:"LOGIN_LOCKOUT_WINDOW_MINUTES"`
	ServiceJWTSigningKey     string `yaml:"service_jwt_signing_key" env:"SERVICE_JWT_SIGNING_KEY"`
	ServiceJWTIssuer         string `yaml:"service_jwt_issuer" env:"SERVICE_JWT_ISSUER"`
	ServiceJWTAudience       string `yaml:"service_jwt_audience" env:"SERVICE_JWT_AUDIENCE"`
	NopsaiServiceID          string `yaml:"nopsai_service_id" env:"NOPSAI_SERVICE_ID"`
	RunnerServiceID          string `yaml:"runner_service_id" env:"RUNNER_SERVICE_ID"`
	AgentServiceID           string `yaml:"agent_service_id" env:"AGENT_SERVICE_ID"`
	DispatcherTLSMode        string `yaml:"dispatcher_tls_mode" env:"DISPATCHER_TLS_MODE"`
	DispatcherTLSSecret      string `yaml:"dispatcher_tls_secret" env:"DISPATCHER_TLS_SECRET"`
	DispatcherTLSServerName  string `yaml:"dispatcher_tls_server_name" env:"DISPATCHER_TLS_SERVER_NAME"`

	LLMDefaultProfile string                       `yaml:"llm_default_profile" env:"LLM_DEFAULT_PROFILE"`
	LLMProfiles       map[string]LLMProfile        `yaml:"llm_profiles" env:"LLM_PROFILES"`
	MCPServers        map[string]models.MCPServer  `yaml:"mcp_servers" env:"MCP_SERVERS"`
	MCPProfiles       map[string]models.MCPProfile `yaml:"mcp_profiles" env:"MCP_PROFILES"`

	// Addresses for services to listen on
	NopsaiListenAddress     string `yaml:"nopsai_listen_address" env:"NOPSAI_LISTEN_ADDRESS"`
	GitBotListenAddress     string `yaml:"git_bot_listen_address" env:"GIT_BOT_LISTEN_ADDRESS"`
	DispatcherListenAddress string `yaml:"dispatcher_listen_address" env:"DISPATCHER_LISTEN_ADDRESS"`
	AAAAddr                 string `yaml:"aaa_addr" env:"AAA_ADDR"`

	// Addresses for services to connect to each other
	AgentLlmAgentAddress string `yaml:"agent_llm_agent_address" env:"AGENT_LLM_AGENT_ADDRESS"`
	AgentNopsaiAPIURL    string `yaml:"agent_nopsai_api_url" env:"AGENT_NOPSAI_API_URL"`
	GitBotNopsaiAPIURL   string `yaml:"git_bot_nopsai_api_url" env:"GIT_BOT_NOPSAI_API_URL"`
	DispatcherAddress    string `yaml:"dispatcher_address" env:"DISPATCHER_ADDRESS"`
	AAAAPIURL            string `yaml:"aaa_api_url" env:"AAA_API_URL"`
	AAASharedToken       string `yaml:"aaa_shared_internal_token" env:"AAA_SHARED_INTERNAL_TOKEN"`

	// Git Bot specific configuration
	GitHubWebhookSecret  string `yaml:"github_webhook_secret" env:"GITHUB_WEBHOOK_SECRET"`
	GitHubAppID          string `yaml:"github_app_id" env:"GITHUB_APP_ID"`
	GitHubInstallID      string `yaml:"github_installation_id" env:"GITHUB_INSTALLATION_ID"`
	GitHubPrivateKeyPath string `yaml:"github_private_key_path" env:"GITHUB_PRIVATE_KEY_PATH"`
	NopsaiGitBotAPIURL   string `yaml:"nopsai_git_bot_api_url" env:"NOPSAI_GIT_BOT_API_URL"`
	GitHubPrivateKey     string `yaml:"github_private_key" env:"GITHUB_PRIVATE_KEY"`

	DockerNetworkName         string `yaml:"docker_network_name" env:"DOCKER_NETWORK_NAME"`
	AutoRemovalAgentContainer bool   `yaml:"auto_removal_agent_container" env:"AUTO_REMOVAL_AGENT_CONTAINER"`
	DefaultPipelineTimeout    string `yaml:"default_pipeline_timeout" env:"DEFAULT_PIPELINE_TIMEOUT"`
	AgentImage                string `yaml:"agent_image" env:"AGENT_IMAGE"`
	LLMAgentTimeout           string `yaml:"llm_agent_timeout" env:"LLM_AGENT_TIMEOUT"`

	Runtime           string                 `yaml:"runtime" env:"RUNTIME"`
	Kubernetes        KubernetesConfig       `yaml:"kubernetes" env:"-"`
	Limits            RunnerLimits           `yaml:"limits" env:"-"`
	RuntimePools      map[string]RuntimePool `yaml:"runtime_pools" env:"RUNTIME_POOLS"`
	DispatcherRouting map[string][]string    `yaml:"dispatcher_routing" env:"DISPATCHER_ROUTING"`
	RunnerID          string                 `yaml:"runner_id" env:"RUNNER_ID"`
	RunnerScopes      string                 `yaml:"runner_scopes" env:"RUNNER_SCOPES"`
	RunnerCapacity    int                    `yaml:"runner_capacity" env:"RUNNER_CAPACITY"`
}

func LoadConfig(path string) (*Config, error) {
	config := &Config{}

	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(file, config)
	if err != nil {
		return nil, err
	}

	// Override with OS variables
	val := reflect.ValueOf(config).Elem()
	for i := 0; i < val.NumField(); i++ {
		field := val.Type().Field(i)
		envTag := field.Tag.Get("env")
		if envTag == "" {
			continue
		}

		envValue := os.Getenv(envTag)
		if envValue != "" {
			switch field.Type.Kind() {
			case reflect.String:
				val.Field(i).SetString(envValue)
			case reflect.Bool:
				// Parse the string to a boolean
				boolVal, err := strconv.ParseBool(envValue)
				if err == nil {
					val.Field(i).SetBool(boolVal)
				}
			case reflect.Int, reflect.Int32, reflect.Int64:
				if intVal, err := strconv.Atoi(envValue); err == nil {
					val.Field(i).SetInt(int64(intVal))
				}
			case reflect.Map:
				// Expect YAML/JSON string for maps (e.g., DISPATCHER_ROUTING='{"prod":["runner-prod-1"]}')
				newVal := reflect.New(field.Type).Interface()
				if err := yaml.Unmarshal([]byte(envValue), newVal); err == nil {
					val.Field(i).Set(reflect.ValueOf(newVal).Elem())
				}
			}
		}
	}

	config.LLMDefaultProfile = NormalizeLLMProfileName(config.LLMDefaultProfile)
	config.LLMProfiles = NormalizeLLMProfiles(config.LLMProfiles)
	config.MCPServers = models.NormalizeMCPServers(config.MCPServers)
	config.MCPProfiles = models.NormalizeMCPProfiles(config.MCPProfiles)
	applyNestedEnvOverrides(config)
	config.Runtime = NormalizeRuntime(config.Runtime)
	config.Kubernetes = NormalizeKubernetesConfig(config.Kubernetes)
	config.RuntimePools = NormalizeRuntimePools(config.RuntimePools)

	return config, nil
}

func applyNestedEnvOverrides(config *Config) {
	if config == nil {
		return
	}
	setStringEnv := func(name string, target *string) {
		if value, ok := os.LookupEnv(name); ok {
			*target = value
		}
	}
	setIntEnv := func(name string, target *int) {
		if value, ok := os.LookupEnv(name); ok {
			if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				*target = parsed
			}
		}
	}
	setBoolPtrEnv := func(name string, target **bool) {
		if value, ok := os.LookupEnv(name); ok {
			if parsed, err := strconv.ParseBool(strings.TrimSpace(value)); err == nil {
				*target = &parsed
			}
		}
	}
	setStringMapEnv := func(name string, target *map[string]string) {
		if value, ok := os.LookupEnv(name); ok {
			next := map[string]string{}
			if err := yaml.Unmarshal([]byte(value), &next); err == nil {
				*target = next
			}
		}
	}

	setStringEnv("KUBERNETES_NAMESPACE", &config.Kubernetes.Namespace)
	setStringEnv("KUBERNETES_SERVICE_ACCOUNT", &config.Kubernetes.ServiceAccount)
	setStringEnv("KUBERNETES_DEFAULT_IMAGE_PULL_POLICY", &config.Kubernetes.DefaultImagePullPolicy)
	setStringEnv("KUBERNETES_DEFAULT_WORKSPACE_SIZE", &config.Kubernetes.DefaultWorkspaceSize)
	setStringEnv("KUBERNETES_DEFAULT_WORKSPACE_ACCESS_MODE", &config.Kubernetes.DefaultWorkspaceAccessMode)
	setStringEnv("KUBERNETES_DEFAULT_TASK_TIMEOUT", &config.Kubernetes.DefaultTaskTimeout)
	setStringEnv("KUBERNETES_DEFAULT_RUN_TIMEOUT", &config.Kubernetes.DefaultRunTimeout)
	setStringEnv("KUBERNETES_WORKSPACE_VOLUME_MODE", &config.Kubernetes.WorkspaceVolumeMode)
	setStringEnv("KUBERNETES_EXISTING_WORKSPACE_PVC", &config.Kubernetes.ExistingWorkspacePVC)
	setStringEnv("KUBERNETES_STORAGE_CLASS", &config.Kubernetes.StorageClass)
	setBoolPtrEnv("KUBERNETES_AFFINITY_ENABLED", &config.Kubernetes.AffinityEnabled)
	setBoolPtrEnv("KUBERNETES_CLEANUP_FINISHED_PODS", &config.Kubernetes.CleanupFinishedPods)
	setStringMapEnv("KUBERNETES_POD_LABELS", &config.Kubernetes.PodLabels)
	setStringMapEnv("KUBERNETES_POD_ANNOTATIONS", &config.Kubernetes.PodAnnotations)

	setIntEnv("LIMITS_MAX_CONCURRENT_RUNS", &config.Limits.MaxConcurrentRuns)
	setIntEnv("LIMITS_MAX_CONCURRENT_TASKS", &config.Limits.MaxConcurrentTasks)
	setIntEnv("LIMITS_MAX_CONCURRENT_TASKS_PER_RUN", &config.Limits.MaxConcurrentTasksPerRun)
	setIntEnv("LIMITS_MAX_PENDING_TASKS", &config.Limits.MaxPendingTasks)
}

func NormalizeRuntime(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "", "docker", "container", "containers":
		return "docker"
	case "k8s", "kubernetes":
		return "kubernetes"
	default:
		return normalized
	}
}

func NormalizeKubernetesConfig(k KubernetesConfig) KubernetesConfig {
	k.Namespace = strings.TrimSpace(k.Namespace)
	k.ServiceAccount = strings.TrimSpace(k.ServiceAccount)
	k.DefaultImagePullPolicy = normalizeImagePullPolicy(k.DefaultImagePullPolicy)
	k.DefaultWorkspaceSize = strings.TrimSpace(k.DefaultWorkspaceSize)
	k.DefaultWorkspaceAccessMode = normalizePVCAccessMode(k.DefaultWorkspaceAccessMode)
	k.DefaultTaskTimeout = strings.TrimSpace(k.DefaultTaskTimeout)
	k.DefaultRunTimeout = strings.TrimSpace(k.DefaultRunTimeout)
	k.WorkspaceVolumeMode = normalizeWorkspaceVolumeMode(k.WorkspaceVolumeMode)
	k.ExistingWorkspacePVC = strings.TrimSpace(k.ExistingWorkspacePVC)
	k.StorageClass = strings.TrimSpace(k.StorageClass)
	k.PodLabels = normalizeStringMap(k.PodLabels)
	k.PodAnnotations = normalizeStringMap(k.PodAnnotations)
	return k
}

func NormalizeRuntimePools(pools map[string]RuntimePool) map[string]RuntimePool {
	if len(pools) == 0 {
		return nil
	}
	normalized := make(map[string]RuntimePool, len(pools))
	for name, pool := range pools {
		poolName := strings.TrimSpace(name)
		if poolName == "" {
			continue
		}
		pool.NodeSelector = normalizeStringMap(pool.NodeSelector)
		pool.Resources.Requests = normalizeStringMap(pool.Resources.Requests)
		pool.Resources.Limits = normalizeStringMap(pool.Resources.Limits)
		normalized[poolName] = pool
	}
	return normalized
}

func normalizeImagePullPolicy(raw string) string {
	normalized := strings.TrimSpace(raw)
	switch strings.ToLower(normalized) {
	case "", "ifnotpresent", "if-not-present":
		return "IfNotPresent"
	case "always":
		return "Always"
	case "never":
		return "Never"
	default:
		return normalized
	}
}

func normalizePVCAccessMode(raw string) string {
	normalized := strings.TrimSpace(raw)
	switch strings.ToLower(normalized) {
	case "", "readwriteonce", "rwo":
		return "ReadWriteOnce"
	case "readwritemany", "rwx":
		return "ReadWriteMany"
	case "readonlymany", "rox":
		return "ReadOnlyMany"
	default:
		return normalized
	}
}

func normalizeWorkspaceVolumeMode(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "", "pvc", "dynamic", "create":
		return "pvc"
	case "existing", "existing_pvc", "existing-pvc":
		return "existing"
	case "emptydir", "empty_dir", "empty-dir":
		return "emptyDir"
	default:
		return normalized
	}
}

func normalizeStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		normalized[key] = strings.TrimSpace(value)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func NormalizeLLMProvider(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))

	switch normalized {
	case "gemini", "google", "google-gemini":
		return LLMProviderGemini
	case "lmstudio", "lm-studio", "openai-compatible", "openai_compatible":
		return LLMProviderLMStudio
	default:
		return normalized
	}
}

func NormalizeLLMProfileName(raw string) string {
	return strings.TrimSpace(raw)
}

func NormalizeLLMProfile(profile LLMProfile) LLMProfile {
	profile.Provider = NormalizeLLMProvider(profile.Provider)
	profile.Model = strings.TrimSpace(profile.Model)
	profile.BaseURL = strings.TrimSpace(profile.BaseURL)
	profile.APIKeySecret = strings.TrimSpace(profile.APIKeySecret)
	profile.Reasoning = NormalizeLMStudioReasoning(profile.Reasoning)

	scopes := make([]string, 0, len(profile.AllowedScopes))
	seen := map[string]bool{}
	for _, scope := range profile.AllowedScopes {
		normalized := strings.Trim(strings.TrimSpace(scope), "/")
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		scopes = append(scopes, normalized)
	}
	profile.AllowedScopes = scopes

	return profile
}

func EffectiveLLMProfileReasoning(profile LLMProfile) string {
	reasoning := NormalizeLMStudioReasoning(profile.Reasoning)
	if reasoning != "" {
		return reasoning
	}
	if profile.Thinking == nil {
		return ""
	}
	if *profile.Thinking {
		return "on"
	}
	return "off"
}

func NormalizeLLMProfiles(raw map[string]LLMProfile) map[string]LLMProfile {
	if len(raw) == 0 {
		return nil
	}

	normalized := make(map[string]LLMProfile, len(raw))
	for name, profile := range raw {
		profileName := NormalizeLLMProfileName(name)
		if profileName == "" {
			continue
		}
		normalized[profileName] = NormalizeLLMProfile(profile)
	}
	return normalized
}

func (c Config) EffectiveLLMDefaultProfile() string {
	if name := NormalizeLLMProfileName(c.LLMDefaultProfile); name != "" {
		return name
	}
	return DefaultLLMProfileName
}

func (c Config) EffectiveLLMProfiles() map[string]LLMProfile {
	return NormalizeLLMProfiles(c.LLMProfiles)
}

func (c Config) EffectiveMCPServers() map[string]models.MCPServer {
	return models.NormalizeMCPServers(c.MCPServers)
}

func (c Config) EffectiveMCPProfiles() map[string]models.MCPProfile {
	return models.NormalizeMCPProfiles(c.MCPProfiles)
}

func LLMProfileAllowedInScope(profile LLMProfile, scope string) bool {
	if len(profile.AllowedScopes) == 0 {
		return true
	}
	scope = strings.Trim(strings.TrimSpace(scope), "/")
	for _, allowed := range profile.AllowedScopes {
		if strings.EqualFold(strings.Trim(strings.TrimSpace(allowed), "/"), scope) {
			return true
		}
	}
	return false
}

func NormalizeLMStudioReasoning(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "true" {
		return "on"
	}
	if normalized == "false" {
		return "off"
	}
	if _, ok := validLMStudioReasoningLevels[normalized]; ok {
		return normalized
	}
	return normalized
}

func IsValidLMStudioReasoning(raw string) bool {
	_, ok := validLMStudioReasoningLevels[NormalizeLMStudioReasoning(raw)]
	return ok
}

func (c Config) EffectiveServiceJWTSigningKey() string {
	if key := strings.TrimSpace(c.ServiceJWTSigningKey); key != "" {
		return key
	}
	return strings.TrimSpace(c.JWTSigningKey)
}

func (c Config) EffectiveServiceJWTIssuer() string {
	if issuer := strings.TrimSpace(c.ServiceJWTIssuer); issuer != "" {
		return issuer
	}
	if issuer := strings.TrimSpace(c.JWTIssuer); issuer != "" {
		return issuer
	}
	return "nopsai.internal"
}

func (c Config) EffectiveServiceJWTAudience() string {
	if audience := strings.TrimSpace(c.ServiceJWTAudience); audience != "" {
		return audience
	}
	return "nopsai-dispatcher"
}

func (c Config) EffectiveNopsaiServiceID() string {
	if id := strings.TrimSpace(c.NopsaiServiceID); id != "" {
		return id
	}
	return "nopsai"
}

func (c Config) EffectiveRunnerServiceID() string {
	if id := strings.TrimSpace(c.RunnerServiceID); id != "" {
		return id
	}
	return "runner"
}

func (c Config) EffectiveAgentServiceID() string {
	if id := strings.TrimSpace(c.AgentServiceID); id != "" {
		return id
	}
	return "agent"
}

func (c Config) EffectiveDispatcherTLSMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.DispatcherTLSMode))
	switch mode {
	case "", "auto":
		return "mtls"
	case "m-tls", "mutual", "mutual-tls":
		return "mtls"
	case "server", "server-tls":
		return "tls"
	case "off", "false", "no", "none", "disable", "insecure", "plaintext":
		return "disabled"
	default:
		return mode
	}
}

func (c Config) EffectiveDispatcherTLSSecret() string {
	if secret := strings.TrimSpace(c.DispatcherTLSSecret); secret != "" {
		return secret
	}
	return c.EffectiveServiceJWTSigningKey()
}

func (c Config) EffectiveDispatcherTLSServerName() string {
	if name := strings.TrimSpace(c.DispatcherTLSServerName); name != "" {
		return name
	}
	return "dispatcher"
}

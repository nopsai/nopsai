package config

import (
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"nopsai/pkg/models"

	"gopkg.in/yaml.v3"
)

const (
	LLMProviderGemini      = "gemini"
	LLMProviderLMStudio    = "lmstudio"
	LLMProviderOpenAI      = "openai"
	LLMProviderAnthropic   = "anthropic"
	LLMProviderGroq        = "groq"
	LLMProviderMistral     = "mistral"
	LLMProviderOllama      = "ollama"
	LLMProviderOpenRouter  = "openrouter"
	LLMProviderAzureOpenAI = "azure-openai"

	DefaultLLMProfileName = "standard"
)

type ConfigScope string

const (
	ConfigScopeBootstrapOnly ConfigScope = "bootstrap_only"
	ConfigScopeRuntimeLive   ConfigScope = "runtime_live"
	ConfigScopeRuntimeReload ConfigScope = "runtime_reload"
	ConfigScopeNextRunOnly   ConfigScope = "next_run_only"
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
	Provider           string            `yaml:"provider" json:"provider"`
	Model              string            `yaml:"model,omitempty" json:"model,omitempty"`
	BaseURL            string            `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	CredentialRef      string            `yaml:"credential_ref,omitempty" json:"credential_ref,omitempty"`
	LegacyAPIKeySecret string            `yaml:"api_key_secret,omitempty" json:"-"`
	AllowedScopes      []string          `yaml:"allowed_scopes,omitempty" json:"allowed_scopes,omitempty"`
	Reasoning          string            `yaml:"reasoning,omitempty" json:"reasoning,omitempty"`
	Thinking           *bool             `yaml:"thinking,omitempty" json:"thinking,omitempty"`
	TimeoutSeconds     int               `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty"`
	MaxTokens          int               `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	Temperature        *float64          `yaml:"temperature,omitempty" json:"temperature,omitempty"`
	PromptCache        LLMFeatureConfig  `yaml:"prompt_cache,omitempty" json:"prompt_cache,omitempty"`
	ProviderState      LLMFeatureConfig  `yaml:"provider_state,omitempty" json:"provider_state,omitempty"`
	Extra              map[string]string `yaml:"extra,omitempty" json:"extra,omitempty"`
}

type LLMFeatureConfig struct {
	Mode      string `yaml:"mode,omitempty" json:"mode,omitempty"`
	Scope     string `yaml:"scope,omitempty" json:"scope,omitempty"`
	Retention string `yaml:"retention,omitempty" json:"retention,omitempty"`
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

type SystemLogsConfig struct {
	Enabled             *bool                      `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Provider            string                     `yaml:"provider,omitempty" json:"provider,omitempty"`
	DockerHost          string                     `yaml:"docker_host,omitempty" json:"docker_host,omitempty"`
	Kubernetes          SystemLogsKubernetesConfig `yaml:"kubernetes,omitempty" json:"kubernetes,omitempty"`
	BufferLines         int                        `yaml:"buffer_lines,omitempty" json:"buffer_lines,omitempty"`
	BufferAgeMinutes    int                        `yaml:"buffer_age_minutes,omitempty" json:"buffer_age_minutes,omitempty"`
	MaxTailLines        int                        `yaml:"max_tail_lines,omitempty" json:"max_tail_lines,omitempty"`
	MaxLineBytes        int                        `yaml:"max_line_bytes,omitempty" json:"max_line_bytes,omitempty"`
	MaxStreams          int                        `yaml:"max_streams,omitempty" json:"max_streams,omitempty"`
	MaxStreamsPerSource int                        `yaml:"max_streams_per_source,omitempty" json:"max_streams_per_source,omitempty"`
}

type SystemLogsKubernetesConfig struct {
	Namespace     string `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	LabelSelector string `yaml:"label_selector,omitempty" json:"label_selector,omitempty"`
	Container     string `yaml:"container,omitempty" json:"container,omitempty"`
}

type BootstrapAdminConfig struct {
	Email                string `yaml:"email,omitempty" json:"email,omitempty"`
	Password             string `yaml:"password,omitempty" json:"-"`
	PasswordFile         string `yaml:"password_file,omitempty" json:"-"`
	AllowDefaultPassword bool   `yaml:"allow_default_password,omitempty" json:"allow_default_password,omitempty"`
	MustChangePassword   *bool  `yaml:"must_change_password,omitempty" json:"must_change_password,omitempty"`
}

type AuthConfig struct {
	LocalEnabled *bool          `yaml:"local_enabled,omitempty" json:"local_enabled,omitempty"`
	OIDC         OIDCAuthConfig `yaml:"oidc,omitempty" json:"oidc,omitempty"`
}

type OIDCAuthConfig struct {
	Enabled           bool                          `yaml:"enabled" json:"enabled"`
	AutoCreateUsers   bool                          `yaml:"auto_create_users" json:"auto_create_users"`
	DefaultRole       string                        `yaml:"default_role,omitempty" json:"default_role,omitempty"`
	AllowEmailLinking bool                          `yaml:"allow_email_linking,omitempty" json:"allow_email_linking,omitempty"`
	DomainMapping     map[string]string             `yaml:"domain_mapping,omitempty" json:"domain_mapping,omitempty"`
	Providers         map[string]OIDCProviderConfig `yaml:"providers,omitempty" json:"providers,omitempty"`
}

type OIDCProviderConfig struct {
	Type                  string                              `yaml:"type,omitempty" json:"type,omitempty"`
	DisplayName           string                              `yaml:"display_name,omitempty" json:"display_name,omitempty"`
	Issuer                string                              `yaml:"issuer,omitempty" json:"issuer,omitempty"`
	AuthorizationEndpoint string                              `yaml:"authorization_endpoint,omitempty" json:"authorization_endpoint,omitempty"`
	TokenEndpoint         string                              `yaml:"token_endpoint,omitempty" json:"token_endpoint,omitempty"`
	JWKSURI               string                              `yaml:"jwks_uri,omitempty" json:"jwks_uri,omitempty"`
	UserInfoEndpoint      string                              `yaml:"userinfo_endpoint,omitempty" json:"userinfo_endpoint,omitempty"`
	ClientID              string                              `yaml:"client_id,omitempty" json:"client_id,omitempty"`
	ClientCredentialRef   string                              `yaml:"client_credential_ref,omitempty" json:"client_credential_ref,omitempty"`
	LegacyClientSecret    string                              `yaml:"client_secret,omitempty" json:"-"`
	Scopes                []string                            `yaml:"scopes,omitempty" json:"scopes,omitempty"`
	AllowedEmailDomains   []string                            `yaml:"allowed_email_domains,omitempty" json:"allowed_email_domains,omitempty"`
	TeamClaim             string                              `yaml:"team_claim,omitempty" json:"team_claim,omitempty"`
	RoleMapping           map[string]string                   `yaml:"role_mapping,omitempty" json:"role_mapping,omitempty"`
	TeamMapping           map[string]string                   `yaml:"team_mapping,omitempty" json:"team_mapping,omitempty"`
	BasicRoleMapping      map[string]OIDCBasicRoleGrantConfig `yaml:"basic_role_mapping,omitempty" json:"basic_role_mapping,omitempty"`
	EntitlementSync       OIDCEntitlementSyncConfig           `yaml:"entitlement_sync,omitempty" json:"entitlement_sync,omitempty"`
	AutoCreateUsers       *bool                               `yaml:"auto_create_users,omitempty" json:"auto_create_users,omitempty"`
	DefaultRole           string                              `yaml:"default_role,omitempty" json:"default_role,omitempty"`
	AllowEmailLinking     *bool                               `yaml:"allow_email_linking,omitempty" json:"allow_email_linking,omitempty"`
	Enabled               *bool                               `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

type OIDCBasicRoleGrantConfig struct {
	Role         string `yaml:"role,omitempty" json:"role,omitempty"`
	Resource     string `yaml:"resource,omitempty" json:"resource,omitempty"`
	ResourceType string `yaml:"resource_type,omitempty" json:"resource_type,omitempty"`
	ResourceID   string `yaml:"resource_id,omitempty" json:"resource_id,omitempty"`
}

type OIDCEntitlementSyncConfig struct {
	Mode                       string `yaml:"mode,omitempty" json:"mode,omitempty"`
	AdminBaseURL               string `yaml:"admin_base_url,omitempty" json:"admin_base_url,omitempty"`
	Realm                      string `yaml:"realm,omitempty" json:"realm,omitempty"`
	AdminRealm                 string `yaml:"admin_realm,omitempty" json:"admin_realm,omitempty"`
	AdminClientID              string `yaml:"admin_client_id,omitempty" json:"admin_client_id,omitempty"`
	AdminClientCredentialRef   string `yaml:"admin_client_credential_ref,omitempty" json:"admin_client_credential_ref,omitempty"`
	LegacyAdminClientSecret    string `yaml:"admin_client_secret,omitempty" json:"-"`
	AdminUsername              string `yaml:"admin_username,omitempty" json:"admin_username,omitempty"`
	AdminPasswordCredentialRef string `yaml:"admin_password_credential_ref,omitempty" json:"admin_password_credential_ref,omitempty"`
	LegacyAdminPassword        string `yaml:"admin_password,omitempty" json:"-"`
	ClientID                   string `yaml:"client_id,omitempty" json:"client_id,omitempty"`
	TargetResourceType         string `yaml:"target_resource_type,omitempty" json:"target_resource_type,omitempty"`
	TeamPathPrefix             string `yaml:"team_path_prefix,omitempty" json:"team_path_prefix,omitempty"`
}

type AssistantMemoryConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Scope   string `yaml:"scope,omitempty" json:"scope,omitempty"`
}

type AssistantMCPConfig struct {
	Enabled   *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	ServerURL string `yaml:"server_url,omitempty" json:"server_url,omitempty"`
}

type AssistantFeaturesConfig struct {
	Docs                       *bool `yaml:"docs,omitempty" json:"docs,omitempty"`
	PipelineDebugging          *bool `yaml:"pipeline_debugging,omitempty" json:"pipeline_debugging,omitempty"`
	ConfigGeneration           *bool `yaml:"config_generation,omitempty" json:"config_generation,omitempty"`
	StatisticsInsights         *bool `yaml:"statistics_insights,omitempty" json:"statistics_insights,omitempty"`
	MaintenanceRecommendations *bool `yaml:"maintenance_recommendations,omitempty" json:"maintenance_recommendations,omitempty"`
	CostRecommendations        *bool `yaml:"cost_recommendations,omitempty" json:"cost_recommendations,omitempty"`
	ActionExecution            *bool `yaml:"action_execution,omitempty" json:"action_execution,omitempty"`
}

type AssistantActionsConfig struct {
	RequireConfirmation *bool `yaml:"require_confirmation,omitempty" json:"require_confirmation,omitempty"`
}

type AssistantConfig struct {
	Enabled                   bool                    `yaml:"enabled" json:"enabled"`
	Provider                  string                  `yaml:"provider,omitempty" json:"provider,omitempty"`
	Model                     string                  `yaml:"model,omitempty" json:"model,omitempty"`
	BaseURL                   string                  `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	CredentialRef             string                  `yaml:"credential_ref,omitempty" json:"credential_ref,omitempty"`
	LegacyAPIKeySecret        string                  `yaml:"api_key_secret,omitempty" json:"-"`
	Timeout                   string                  `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	MaxInputLogsBytes         int                     `yaml:"max_input_logs_bytes,omitempty" json:"max_input_logs_bytes,omitempty"`
	MaxConversationTurns      int                     `yaml:"max_conversation_turns,omitempty" json:"max_conversation_turns,omitempty"`
	DocsEnabled               *bool                   `yaml:"docs_enabled,omitempty" json:"docs_enabled,omitempty"`
	DocsVersionAware          *bool                   `yaml:"docs_version_aware,omitempty" json:"docs_version_aware,omitempty"`
	DefaultDocsVersion        string                  `yaml:"default_docs_version" json:"default_docs_version,omitempty"`
	ConversationRetentionDays int                     `yaml:"conversation_retention_days" json:"conversation_retention_days,omitempty"`
	Memory                    AssistantMemoryConfig   `yaml:"memory" json:"memory"`
	MCP                       AssistantMCPConfig      `yaml:"mcp" json:"mcp"`
	Features                  AssistantFeaturesConfig `yaml:"features" json:"features"`
	Actions                   AssistantActionsConfig  `yaml:"actions" json:"actions"`
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
	Environment string `yaml:"environment" env:"NOPSAI_ENVIRONMENT"`
	PublicURL   string `yaml:"public_url" env:"NOPSAI_PUBLIC_URL"`

	NotificationMailLogoURL       string `yaml:"notification_mail_logo_url" env:"NOPSAI_MAIL_LOGO_URL"`
	NotificationMailWebsiteURL    string `yaml:"notification_mail_website_url" env:"NOPSAI_MAIL_WEBSITE_URL"`
	NotificationMailSupportURL    string `yaml:"notification_mail_support_url" env:"NOPSAI_MAIL_SUPPORT_URL"`
	NotificationMailFooterAddress string `yaml:"notification_mail_footer_address" env:"NOPSAI_MAIL_FOOTER_ADDRESS"`

	RequireProductionGates bool `yaml:"require_production_gates" env:"NOPSAI_REQUIRE_PRODUCTION_GATES"`

	MasterKey      string               `yaml:"master_key" env:"NOPSAI_MASTER_KEY"`
	BootstrapAdmin BootstrapAdminConfig `yaml:"bootstrap_admin" env:"-"`

	// Authentication and authorization
	JWTSigningKey            string     `yaml:"jwt_signing_key" env:"JWT_SIGNING_KEY"`
	JWTRSAKeyPath            string     `yaml:"jwt_rsa_key_path" env:"JWT_RSA_KEY_PATH"`
	JWTIssuer                string     `yaml:"jwt_issuer" env:"JWT_ISSUER"`
	JWTAudience              string     `yaml:"jwt_audience" env:"JWT_AUDIENCE"`
	JWTExpiryMinutes         int        `yaml:"jwt_expiry_minutes" env:"JWT_EXPIRY_MINUTES"`
	IdleTimeoutMinutes       int        `yaml:"idle_timeout_minutes" env:"IDLE_TIMEOUT_MINUTES"`
	RefreshTokenTTLMinutes   int        `yaml:"refresh_token_ttl_minutes" env:"REFRESH_TOKEN_TTL_MINUTES"`
	AuthProviderLocalEnabled bool       `yaml:"auth_provider_local_enabled" env:"AUTH_PROVIDER_LOCAL_ENABLED"`
	RateLimitLoginPerMinute  int        `yaml:"rate_limit_login_per_minute" env:"RATE_LIMIT_LOGIN_PER_MINUTE"`
	LoginLockoutThreshold    int        `yaml:"login_lockout_threshold" env:"LOGIN_LOCKOUT_THRESHOLD"`
	LoginLockoutWindowMin    int        `yaml:"login_lockout_window_minutes" env:"LOGIN_LOCKOUT_WINDOW_MINUTES"`
	Auth                     AuthConfig `yaml:"auth" env:"-"`
	ServiceJWTSigningKey     string     `yaml:"service_jwt_signing_key" env:"SERVICE_JWT_SIGNING_KEY"`
	ServiceJWTIssuer         string     `yaml:"service_jwt_issuer" env:"SERVICE_JWT_ISSUER"`
	ServiceJWTAudience       string     `yaml:"service_jwt_audience" env:"SERVICE_JWT_AUDIENCE"`
	NopsaiServiceID          string     `yaml:"nopsai_service_id" env:"NOPSAI_SERVICE_ID"`
	RunnerServiceID          string     `yaml:"runner_service_id" env:"RUNNER_SERVICE_ID"`
	AgentServiceID           string     `yaml:"agent_service_id" env:"AGENT_SERVICE_ID"`
	GitBotServiceID          string     `yaml:"git_bot_service_id" env:"GIT_BOT_SERVICE_ID"`
	DispatcherTLSMode        string     `yaml:"dispatcher_tls_mode" env:"DISPATCHER_TLS_MODE"`
	DispatcherTLSSecret      string     `yaml:"dispatcher_tls_secret" env:"DISPATCHER_TLS_SECRET"`
	DispatcherTLSServerName  string     `yaml:"dispatcher_tls_server_name" env:"DISPATCHER_TLS_SERVER_NAME"`

	LLMDefaultProfile            string                       `yaml:"llm_default_profile" env:"LLM_DEFAULT_PROFILE"`
	LLMProfiles                  map[string]LLMProfile        `yaml:"llm_profiles" env:"LLM_PROFILES"`
	MCPServers                   map[string]models.MCPServer  `yaml:"mcp_servers" env:"MCP_SERVERS"`
	MCPProfiles                  map[string]models.MCPProfile `yaml:"mcp_profiles" env:"MCP_PROFILES"`
	Assistant                    AssistantConfig              `yaml:"assistant" env:"-"`
	FinalOutputPDFRendererURL    string                       `yaml:"final_output_pdf_renderer_url" env:"FINAL_OUTPUT_PDF_RENDERER_URL"`
	FinalOutputPDFTimeoutSeconds int                          `yaml:"final_output_pdf_timeout_seconds" env:"FINAL_OUTPUT_PDF_TIMEOUT_SECONDS"`
	SystemLogs                   SystemLogsConfig             `yaml:"system_logs" env:"-"`
	SystemLogsDockerHost         string                       `yaml:"-" env:"SYSTEM_LOGS_DOCKER_HOST" json:"-"`

	// Addresses for services to listen on
	NopsaiListenAddress     string `yaml:"nopsai_listen_address" env:"NOPSAI_LISTEN_ADDRESS"`
	GitBotListenAddress     string `yaml:"git_bot_listen_address" env:"GIT_BOT_LISTEN_ADDRESS"`
	DispatcherListenAddress string `yaml:"dispatcher_listen_address" env:"DISPATCHER_LISTEN_ADDRESS"`
	AAAAddr                 string `yaml:"aaa_listen_address" env:"AAA_LISTEN_ADDRESS"`

	// Addresses for services to connect to each other
	AgentLlmAgentAddress string `yaml:"agent_llm_agent_address" env:"AGENT_LLM_AGENT_ADDRESS"`
	NopsaiAPIURL         string `yaml:"nopsai_api_url" env:"NOPSAI_API_URL"`
	AgentNopsaiAPIURL    string `yaml:"-" env:"-" json:"-"`
	GitBotNopsaiAPIURL   string `yaml:"-" env:"-" json:"-"`
	DispatcherAddress    string `yaml:"dispatcher_grpc_address" env:"DISPATCHER_GRPC_ADDRESS"`
	AAAAPIURL            string `yaml:"aaa_api_url" env:"AAA_API_URL"`
	AAASharedToken       string `yaml:"aaa_shared_internal_token" env:"AAA_SHARED_INTERNAL_TOKEN"`

	// Git Bot specific configuration
	GitHubWebhookCredentialRef    string                     `yaml:"github_webhook_credential_ref" env:"GITHUB_WEBHOOK_CREDENTIAL_REF"`
	GitHubPrivateKeyCredentialRef string                     `yaml:"github_private_key_credential_ref" env:"GITHUB_PRIVATE_KEY_CREDENTIAL_REF"`
	GitHubAppID                   string                     `yaml:"github_app_id" env:"GITHUB_APP_ID"`
	GitHubInstallID               string                     `yaml:"github_installation_id" env:"GITHUB_INSTALLATION_ID"`
	GitHubInstallations           []GitHubInstallationConfig `yaml:"github_installations" env:"-"`
	NopsaiGitBotAPIURL            string                     `yaml:"git_bot_api_url" env:"GIT_BOT_API_URL"`
	LegacyGitHubWebhookSecret     string                     `yaml:"github_webhook_secret,omitempty" env:"GITHUB_WEBHOOK_SECRET" json:"-"`
	LegacyGitHubPrivateKeyPath    string                     `yaml:"github_private_key_path,omitempty" env:"GITHUB_PRIVATE_KEY_PATH" json:"-"`
	LegacyGitHubPrivateKey        string                     `yaml:"github_private_key,omitempty" env:"GITHUB_PRIVATE_KEY" json:"-"`

	DockerNetworkName         string `yaml:"docker_network_name" env:"DOCKER_NETWORK_NAME"`
	AutoRemovalAgentContainer bool   `yaml:"auto_removal_agent_container" env:"AUTO_REMOVAL_AGENT_CONTAINER"`
	DefaultPipelineTimeout    string `yaml:"default_pipeline_timeout" env:"DEFAULT_PIPELINE_TIMEOUT"`
	RuntimeOutputMaxBytes     int    `yaml:"runtime_output_max_bytes" env:"RUNTIME_OUTPUT_MAX_BYTES"`
	AgentImage                string `yaml:"agent_image" env:"AGENT_IMAGE"`
	LLMAgentTimeout           string `yaml:"llm_agent_timeout" env:"LLM_AGENT_TIMEOUT"`
	DataBackupDir             string `yaml:"data_backup_dir" env:"DATA_BACKUP_DIR"`

	Runtime           string                 `yaml:"runtime" env:"RUNTIME"`
	Kubernetes        KubernetesConfig       `yaml:"kubernetes" env:"-"`
	Limits            RunnerLimits           `yaml:"limits" env:"-"`
	RuntimePools      map[string]RuntimePool `yaml:"runtime_pools" env:"RUNTIME_POOLS"`
	DispatcherRouting map[string][]string    `yaml:"dispatcher_routing" env:"DISPATCHER_ROUTING"`
	RunnerID          string                 `yaml:"runner_id" env:"RUNNER_ID"`
	RunnerScopes      string                 `yaml:"runner_scopes" env:"RUNNER_SCOPES"`
	RunnerCapacity    int                    `yaml:"runner_capacity" env:"RUNNER_CAPACITY"`
	EjectedRunnerIDs  []string               `yaml:"-" env:"-" json:"-"`
}

type GitHubInstallationConfig struct {
	InstallationID          string `yaml:"installation_id" json:"installation_id"`
	AccountLogin            string `yaml:"account_login,omitempty" json:"account_login,omitempty"`
	AccountType             string `yaml:"account_type,omitempty" json:"account_type,omitempty"`
	Enabled                 *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	AccessibleRepositories  int    `yaml:"accessible_repositories,omitempty" json:"accessible_repositories,omitempty"`
	LastVerifiedAt          string `yaml:"last_verified_at,omitempty" json:"last_verified_at,omitempty"`
	LastRepositoryRefreshAt string `yaml:"last_repository_refresh_at,omitempty" json:"last_repository_refresh_at,omitempty"`
	LastError               string `yaml:"last_error,omitempty" json:"last_error,omitempty"`
}

func GitHubInstallationEnabled(installation GitHubInstallationConfig) bool {
	return installation.Enabled == nil || *installation.Enabled
}

func NormalizeGitHubAccountType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "org", "organization", "organisation":
		return "organization"
	case "user", "personal", "personal_account", "personal account":
		return "user"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func NormalizeGitHubInstallations(installations []GitHubInstallationConfig, legacyInstallationID string) []GitHubInstallationConfig {
	normalized := make([]GitHubInstallationConfig, 0, len(installations)+1)
	seen := map[string]struct{}{}
	for _, installation := range installations {
		id := strings.TrimSpace(installation.InstallationID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, GitHubInstallationConfig{
			InstallationID:          id,
			AccountLogin:            strings.TrimSpace(installation.AccountLogin),
			AccountType:             NormalizeGitHubAccountType(installation.AccountType),
			Enabled:                 installation.Enabled,
			AccessibleRepositories:  max(0, installation.AccessibleRepositories),
			LastVerifiedAt:          strings.TrimSpace(installation.LastVerifiedAt),
			LastRepositoryRefreshAt: strings.TrimSpace(installation.LastRepositoryRefreshAt),
			LastError:               strings.TrimSpace(installation.LastError),
		})
	}
	legacyID := strings.TrimSpace(legacyInstallationID)
	if legacyID != "" {
		if _, exists := seen[legacyID]; !exists {
			enabled := true
			normalized = append(normalized, GitHubInstallationConfig{
				InstallationID: legacyID,
				AccountType:    "",
				Enabled:        &enabled,
			})
		}
	}
	return normalized
}

func (c *Config) EffectiveSystemLogsDockerHost() string {
	if c == nil {
		return ""
	}
	if host := strings.TrimSpace(c.SystemLogsDockerHost); host != "" {
		return host
	}
	return strings.TrimSpace(c.SystemLogs.DockerHost)
}

func (c *Config) EffectiveSystemLogsProvider() string {
	if c == nil {
		return ""
	}
	provider := strings.ToLower(strings.TrimSpace(c.SystemLogs.Provider))
	switch provider {
	case "k8s":
		return "kubernetes"
	case "docker", "kubernetes":
		return provider
	}
	if c.EffectiveSystemLogsDockerHost() != "" {
		return "docker"
	}
	if c.SystemLogs.Kubernetes.Enabled() {
		return "kubernetes"
	}
	return ""
}

func (c *Config) SystemLogsEnabled() bool {
	if c == nil {
		return false
	}
	if c.SystemLogs.Enabled != nil {
		return *c.SystemLogs.Enabled
	}
	return c.EffectiveSystemLogsProvider() != ""
}

func (c SystemLogsKubernetesConfig) Enabled() bool {
	return strings.TrimSpace(c.Namespace) != "" ||
		strings.TrimSpace(c.LabelSelector) != "" ||
		strings.TrimSpace(c.Container) != ""
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
	config.BootstrapAdmin = NormalizeBootstrapAdminConfig(config.BootstrapAdmin)
	config.Auth = NormalizeAuthConfig(config.Auth)
	config.Assistant = NormalizeAssistantConfig(config.Assistant)
	config.Runtime = NormalizeRuntime(config.Runtime)
	if config.RuntimeOutputMaxBytes <= 0 {
		config.RuntimeOutputMaxBytes = 64 * 1024
	}
	config.Kubernetes = NormalizeKubernetesConfig(config.Kubernetes)
	config.SystemLogs = NormalizeSystemLogsConfig(config.SystemLogs)
	config.RuntimePools = NormalizeRuntimePools(config.RuntimePools)
	config.EjectedRunnerIDs = NormalizeRunnerIDs(config.EjectedRunnerIDs)
	config.GitHubInstallations = NormalizeGitHubInstallations(config.GitHubInstallations, config.GitHubInstallID)
	config.NormalizeServiceTopology()

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
	setBoolEnv := func(name string, target *bool) {
		if value, ok := os.LookupEnv(name); ok {
			if parsed, err := strconv.ParseBool(strings.TrimSpace(value)); err == nil {
				*target = parsed
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

	setStringEnv("SYSTEM_LOGS_PROVIDER", &config.SystemLogs.Provider)
	setStringEnv("SYSTEM_LOGS_KUBERNETES_NAMESPACE", &config.SystemLogs.Kubernetes.Namespace)
	setStringEnv("SYSTEM_LOGS_KUBERNETES_LABEL_SELECTOR", &config.SystemLogs.Kubernetes.LabelSelector)
	setStringEnv("SYSTEM_LOGS_KUBERNETES_CONTAINER", &config.SystemLogs.Kubernetes.Container)

	setStringEnv("NOPSAI_BOOTSTRAP_ADMIN_EMAIL", &config.BootstrapAdmin.Email)
	setStringEnv("NOPSAI_BOOTSTRAP_ADMIN_PASSWORD", &config.BootstrapAdmin.Password)
	setStringEnv("NOPSAI_BOOTSTRAP_ADMIN_PASSWORD_FILE", &config.BootstrapAdmin.PasswordFile)
	setBoolEnv("NOPSAI_BOOTSTRAP_ADMIN_ALLOW_DEFAULT_PASSWORD", &config.BootstrapAdmin.AllowDefaultPassword)
	setBoolPtrEnv("NOPSAI_BOOTSTRAP_ADMIN_MUST_CHANGE_PASSWORD", &config.BootstrapAdmin.MustChangePassword)
}

func (c *Config) NormalizeServiceTopology() {
	if c == nil {
		return
	}
	nopsaiAPIURL := c.EffectiveNopsaiAPIURL()
	c.NopsaiAPIURL = nopsaiAPIURL
	c.AgentNopsaiAPIURL = nopsaiAPIURL
	c.GitBotNopsaiAPIURL = nopsaiAPIURL
	c.DispatcherAddress = strings.TrimSpace(c.DispatcherAddress)
	c.AAAAddr = strings.TrimSpace(c.AAAAddr)
	c.NopsaiGitBotAPIURL = strings.TrimSpace(c.NopsaiGitBotAPIURL)
}

func (c Config) EffectiveNopsaiAPIURL() string {
	for _, candidate := range []string{c.NopsaiAPIURL, c.AgentNopsaiAPIURL, c.GitBotNopsaiAPIURL} {
		if value := strings.TrimSpace(candidate); value != "" {
			return value
		}
	}
	return ""
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

func NormalizeAuthConfig(auth AuthConfig) AuthConfig {
	auth.OIDC.DefaultRole = strings.TrimSpace(auth.OIDC.DefaultRole)
	auth.OIDC.DomainMapping = normalizeDomainProviderMap(auth.OIDC.DomainMapping)
	auth.OIDC.Providers = normalizeOIDCProviders(auth.OIDC.Providers)
	return auth
}

func NormalizeBootstrapAdminConfig(admin BootstrapAdminConfig) BootstrapAdminConfig {
	admin.Email = strings.TrimSpace(admin.Email)
	admin.PasswordFile = strings.TrimSpace(admin.PasswordFile)
	return admin
}

func NormalizeAssistantConfig(assistant AssistantConfig) AssistantConfig {
	assistant.Provider = NormalizeLLMProvider(assistant.Provider)
	assistant.Model = strings.TrimSpace(assistant.Model)
	if assistant.Provider != "" && assistant.Model == "" {
		assistant.Model = DefaultLLMProviderModel(assistant.Provider)
	}
	assistant.BaseURL = strings.TrimSpace(assistant.BaseURL)
	assistant.CredentialRef = strings.TrimSpace(assistant.CredentialRef)
	assistant.LegacyAPIKeySecret = strings.TrimSpace(assistant.LegacyAPIKeySecret)
	if assistant.Timeout = strings.TrimSpace(assistant.Timeout); assistant.Timeout == "" {
		assistant.Timeout = "60s"
	} else if _, err := time.ParseDuration(assistant.Timeout); err != nil {
		assistant.Timeout = "60s"
	}
	if assistant.MaxInputLogsBytes <= 0 {
		assistant.MaxInputLogsBytes = 120000
	}
	if assistant.MaxConversationTurns <= 0 {
		assistant.MaxConversationTurns = 30
	}
	if assistant.DocsEnabled == nil {
		assistant.DocsEnabled = boolConfigPtr(true)
	}
	if assistant.DocsVersionAware == nil {
		assistant.DocsVersionAware = boolConfigPtr(true)
	}
	assistant.DefaultDocsVersion = strings.TrimSpace(assistant.DefaultDocsVersion)
	if assistant.DefaultDocsVersion == "" {
		assistant.DefaultDocsVersion = "auto"
	}
	if assistant.ConversationRetentionDays <= 0 {
		assistant.ConversationRetentionDays = 30
	}
	assistant.Memory.Scope = strings.ToLower(strings.TrimSpace(assistant.Memory.Scope))
	if assistant.Memory.Scope == "" {
		assistant.Memory.Scope = "conversation"
	}
	if assistant.Memory.Scope != "conversation" {
		assistant.Memory.Scope = "conversation"
	}
	if assistant.MCP.Enabled == nil {
		assistant.MCP.Enabled = boolConfigPtr(true)
	}
	assistant.MCP.ServerURL = strings.TrimSpace(assistant.MCP.ServerURL)
	assistant.Features = NormalizeAssistantFeaturesConfig(assistant.Features)
	if assistant.Actions.RequireConfirmation == nil {
		assistant.Actions.RequireConfirmation = boolConfigPtr(true)
	}
	return assistant
}

func (c Config) EffectiveAssistantConfig() AssistantConfig {
	return NormalizeAssistantConfig(c.Assistant)
}

func NormalizeAssistantFeaturesConfig(features AssistantFeaturesConfig) AssistantFeaturesConfig {
	if features.Docs == nil {
		features.Docs = boolConfigPtr(true)
	}
	if features.PipelineDebugging == nil {
		features.PipelineDebugging = boolConfigPtr(true)
	}
	if features.ConfigGeneration == nil {
		features.ConfigGeneration = boolConfigPtr(true)
	}
	if features.StatisticsInsights == nil {
		features.StatisticsInsights = boolConfigPtr(true)
	}
	if features.MaintenanceRecommendations == nil {
		features.MaintenanceRecommendations = boolConfigPtr(true)
	}
	if features.CostRecommendations == nil {
		features.CostRecommendations = boolConfigPtr(true)
	}
	if features.ActionExecution == nil {
		features.ActionExecution = boolConfigPtr(false)
	}
	return features
}

func AssistantFeatureFlagEnabled(flag *bool) bool {
	return flag != nil && *flag
}

func AssistantMCPEnabled(mcp AssistantMCPConfig) bool {
	return mcp.Enabled != nil && *mcp.Enabled
}

func AssistantRequireConfirmation(actions AssistantActionsConfig) bool {
	return actions.RequireConfirmation == nil || *actions.RequireConfirmation
}

func boolConfigPtr(value bool) *bool {
	return &value
}

func normalizeOIDCProviders(providers map[string]OIDCProviderConfig) map[string]OIDCProviderConfig {
	if len(providers) == 0 {
		return nil
	}
	normalized := make(map[string]OIDCProviderConfig, len(providers))
	for id, provider := range providers {
		providerID := normalizeProviderID(id)
		if providerID == "" {
			continue
		}
		provider.Type = normalizeOIDCProviderType(provider.Type)
		if provider.Type == "" {
			provider.Type = "oidc"
		}
		provider.DisplayName = strings.TrimSpace(provider.DisplayName)
		if provider.DisplayName == "" {
			provider.DisplayName = providerID
		}
		provider.Issuer = strings.TrimRight(strings.TrimSpace(provider.Issuer), "/")
		provider.AuthorizationEndpoint = strings.TrimSpace(provider.AuthorizationEndpoint)
		provider.TokenEndpoint = strings.TrimSpace(provider.TokenEndpoint)
		provider.JWKSURI = strings.TrimSpace(provider.JWKSURI)
		provider.UserInfoEndpoint = strings.TrimSpace(provider.UserInfoEndpoint)
		provider.ClientID = strings.TrimSpace(provider.ClientID)
		provider.ClientCredentialRef = strings.TrimSpace(provider.ClientCredentialRef)
		provider.LegacyClientSecret = strings.TrimSpace(provider.LegacyClientSecret)
		provider.Scopes = normalizeAuthProviderScopes(provider.Type, provider.Scopes)
		provider.AllowedEmailDomains = normalizeEmailDomains(provider.AllowedEmailDomains)
		provider.TeamClaim = strings.TrimSpace(provider.TeamClaim)
		provider.RoleMapping = normalizeStringMap(provider.RoleMapping)
		provider.TeamMapping = normalizeStringMap(provider.TeamMapping)
		provider.BasicRoleMapping = normalizeOIDCBasicRoleMapping(provider.BasicRoleMapping)
		provider.EntitlementSync = normalizeOIDCEntitlementSync(provider.EntitlementSync)
		provider.DefaultRole = strings.TrimSpace(provider.DefaultRole)
		normalized[providerID] = provider
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizeOIDCBasicRoleMapping(mapping map[string]OIDCBasicRoleGrantConfig) map[string]OIDCBasicRoleGrantConfig {
	if len(mapping) == 0 {
		return nil
	}
	out := make(map[string]OIDCBasicRoleGrantConfig, len(mapping))
	for team, grant := range mapping {
		team = strings.TrimSpace(team)
		grant.Role = strings.ToLower(strings.TrimSpace(grant.Role))
		grant.Resource = strings.TrimSpace(grant.Resource)
		grant.ResourceType = strings.TrimSpace(grant.ResourceType)
		grant.ResourceID = strings.TrimSpace(grant.ResourceID)
		if team == "" || grant.Role == "" {
			continue
		}
		if grant.Resource == "" && (grant.ResourceType == "" || grant.ResourceID == "") {
			continue
		}
		out[team] = grant
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeOIDCEntitlementSync(sync OIDCEntitlementSyncConfig) OIDCEntitlementSyncConfig {
	sync.Mode = strings.ToLower(strings.TrimSpace(sync.Mode))
	sync.AdminBaseURL = strings.TrimRight(strings.TrimSpace(sync.AdminBaseURL), "/")
	sync.Realm = strings.TrimSpace(sync.Realm)
	sync.AdminRealm = strings.TrimSpace(sync.AdminRealm)
	if sync.AdminRealm == "" {
		sync.AdminRealm = "master"
	}
	sync.AdminClientID = strings.TrimSpace(sync.AdminClientID)
	if sync.AdminClientID == "" {
		sync.AdminClientID = "admin-cli"
	}
	sync.AdminClientCredentialRef = strings.TrimSpace(sync.AdminClientCredentialRef)
	sync.LegacyAdminClientSecret = strings.TrimSpace(sync.LegacyAdminClientSecret)
	sync.AdminUsername = strings.TrimSpace(sync.AdminUsername)
	sync.AdminPasswordCredentialRef = strings.TrimSpace(sync.AdminPasswordCredentialRef)
	sync.LegacyAdminPassword = strings.TrimSpace(sync.LegacyAdminPassword)
	sync.ClientID = strings.TrimSpace(sync.ClientID)
	sync.TargetResourceType = strings.TrimSpace(sync.TargetResourceType)
	if sync.TargetResourceType == "" {
		sync.TargetResourceType = "team"
	}
	sync.TeamPathPrefix = strings.Trim(strings.TrimSpace(sync.TeamPathPrefix), "/")
	if sync.Mode == "" && sync.AdminBaseURL == "" {
		return OIDCEntitlementSyncConfig{}
	}
	return sync
}

func normalizeProviderID(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	normalized = strings.ReplaceAll(normalized, " ", "-")
	return normalized
}

func normalizeOIDCProviderType(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "okta":
		return "okta"
	case "keycloak":
		return "keycloak"
	case "github", "github-oauth", "oauth2-github":
		return "github"
	case "entra", "entra-id", "azure", "azure-ad", "microsoft-entra":
		return "microsoft"
	case "google-workspace":
		return "google"
	case "generic":
		return "oidc"
	default:
		return normalized
	}
}

func normalizeAuthProviderScopes(providerType string, scopes []string) []string {
	if normalizeOIDCProviderType(providerType) == "github" {
		seen := map[string]bool{}
		normalized := make([]string, 0, len(scopes)+3)
		add := func(scope string) {
			scope = strings.TrimSpace(scope)
			if scope == "" || seen[scope] {
				return
			}
			seen[scope] = true
			normalized = append(normalized, scope)
		}
		for _, scope := range scopes {
			add(scope)
		}
		if len(normalized) == 0 {
			add("read:user")
			add("user:email")
			add("read:org")
		}
		return normalized
	}
	return normalizeOIDCScopes(scopes)
}

func normalizeOIDCScopes(scopes []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(scopes)+3)
	add := func(scope string) {
		scope = strings.TrimSpace(scope)
		if scope == "" || seen[scope] {
			return
		}
		seen[scope] = true
		normalized = append(normalized, scope)
	}
	add("openid")
	for _, scope := range scopes {
		add(scope)
	}
	if len(normalized) == 1 {
		add("email")
		add("profile")
	}
	return normalized
}

func normalizeEmailDomains(domains []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(domains))
	for _, domain := range domains {
		domain = normalizeEmailDomain(domain)
		if domain == "" || seen[domain] {
			continue
		}
		seen[domain] = true
		normalized = append(normalized, domain)
	}
	return normalized
}

func normalizeDomainProviderMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(values))
	for domain, providerID := range values {
		domain = normalizeEmailDomain(domain)
		providerID = normalizeProviderID(providerID)
		if domain == "" || providerID == "" {
			continue
		}
		normalized[domain] = providerID
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizeEmailDomain(raw string) string {
	domain := strings.ToLower(strings.TrimSpace(raw))
	domain = strings.TrimPrefix(domain, "@")
	return domain
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

func NormalizeSystemLogsConfig(cfg SystemLogsConfig) SystemLogsConfig {
	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	if cfg.Provider == "k8s" {
		cfg.Provider = "kubernetes"
	}
	cfg.DockerHost = strings.TrimSpace(cfg.DockerHost)
	cfg.Kubernetes.Namespace = strings.TrimSpace(cfg.Kubernetes.Namespace)
	cfg.Kubernetes.LabelSelector = strings.TrimSpace(cfg.Kubernetes.LabelSelector)
	cfg.Kubernetes.Container = strings.TrimSpace(cfg.Kubernetes.Container)
	return cfg
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

func NormalizeRunnerIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	normalized := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	sort.Strings(normalized)
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
	case "openai", "chatgpt", "gpt":
		return LLMProviderOpenAI
	case "anthropic", "claude":
		return LLMProviderAnthropic
	case "groq":
		return LLMProviderGroq
	case "mistral":
		return LLMProviderMistral
	case "ollama":
		return LLMProviderOllama
	case "openrouter", "open-router":
		return LLMProviderOpenRouter
	case "azure-openai", "azure_openai", "azure":
		return LLMProviderAzureOpenAI
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
	profile.CredentialRef = strings.TrimSpace(profile.CredentialRef)
	profile.LegacyAPIKeySecret = strings.TrimSpace(profile.LegacyAPIKeySecret)
	profile.Reasoning = NormalizeLMStudioReasoning(profile.Reasoning)
	profile.PromptCache = NormalizeLLMFeatureConfig(profile.PromptCache)
	profile.ProviderState = NormalizeLLMFeatureConfig(profile.ProviderState)
	profile.Extra = normalizeStringMap(profile.Extra)

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

func NormalizeLLMFeatureConfig(value LLMFeatureConfig) LLMFeatureConfig {
	value.Mode = NormalizeLLMFeatureMode(value.Mode)
	value.Scope = strings.Trim(strings.TrimSpace(value.Scope), "/")
	value.Retention = strings.TrimSpace(value.Retention)
	return value
}

func NormalizeLLMFeatureMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return "auto"
	case "required":
		return "required"
	case "disabled":
		return "disabled"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func SupportedLLMFeatureMode(value string) bool {
	switch NormalizeLLMFeatureMode(value) {
	case "auto", "required", "disabled":
		return true
	default:
		return false
	}
}

func DefaultLLMProviderBaseURL(provider string) string {
	switch NormalizeLLMProvider(provider) {
	case LLMProviderLMStudio:
		return "http://lmstudio:1234"
	case LLMProviderOpenAI:
		return "https://api.openai.com/v1"
	case LLMProviderAnthropic:
		return "https://api.anthropic.com"
	case LLMProviderGroq:
		return "https://api.groq.com/openai/v1"
	case LLMProviderMistral:
		return "https://api.mistral.ai/v1"
	case LLMProviderOllama:
		return "http://ollama:11434/v1"
	case LLMProviderOpenRouter:
		return "https://openrouter.ai/api/v1"
	default:
		return ""
	}
}

func DefaultLLMProviderModel(provider string) string {
	switch NormalizeLLMProvider(provider) {
	case LLMProviderGemini:
		return "gemini-2.5-flash"
	case LLMProviderLMStudio:
		return "qwen3-coder"
	case LLMProviderOpenAI:
		return "gpt-4.1-mini"
	case LLMProviderAnthropic:
		return "claude-sonnet-4-6"
	case LLMProviderGroq:
		return "llama-3.3-70b-versatile"
	case LLMProviderMistral:
		return "mistral-large-latest"
	case LLMProviderOllama:
		return "qwen2.5-coder:14b"
	case LLMProviderOpenRouter:
		return "openai/gpt-4.1-mini"
	case LLMProviderAzureOpenAI:
		return "gpt-4.1-mini"
	default:
		return ""
	}
}

func DefaultLLMProviderAPIKeySecret(provider string) string {
	switch NormalizeLLMProvider(provider) {
	case LLMProviderGemini:
		return "GEMINI_API_KEY"
	case LLMProviderOpenAI:
		return "OPENAI_API_KEY"
	case LLMProviderAnthropic:
		return "ANTHROPIC_API_KEY"
	case LLMProviderGroq:
		return "GROQ_API_KEY"
	case LLMProviderMistral:
		return "MISTRAL_API_KEY"
	case LLMProviderOpenRouter:
		return "OPENROUTER_API_KEY"
	case LLMProviderAzureOpenAI:
		return "AZURE_OPENAI_API_KEY"
	case LLMProviderOllama:
		return "OLLAMA_API_KEY"
	case LLMProviderLMStudio:
		return "LLM_API_KEY"
	default:
		return ""
	}
}

func EffectiveLLMProfileBaseURL(profile LLMProfile) string {
	if baseURL := strings.TrimSpace(profile.BaseURL); baseURL != "" {
		return baseURL
	}
	return DefaultLLMProviderBaseURL(profile.Provider)
}

func LLMProviderRequiresAPIKey(provider string) bool {
	switch NormalizeLLMProvider(provider) {
	case LLMProviderGemini,
		LLMProviderOpenAI,
		LLMProviderAnthropic,
		LLMProviderGroq,
		LLMProviderMistral,
		LLMProviderOpenRouter,
		LLMProviderAzureOpenAI:
		return true
	default:
		return false
	}
}

func LLMProviderSupportsGenericReasoning(provider string) bool {
	return NormalizeLLMProvider(provider) == LLMProviderLMStudio
}

func LLMProviderSupportsMaxTokens(provider string) bool {
	switch NormalizeLLMProvider(provider) {
	case LLMProviderGemini,
		LLMProviderLMStudio,
		LLMProviderOpenAI,
		LLMProviderAnthropic,
		LLMProviderGroq,
		LLMProviderMistral,
		LLMProviderOllama,
		LLMProviderOpenRouter,
		LLMProviderAzureOpenAI:
		return true
	default:
		return false
	}
}

func LLMProviderTemperatureRange(provider string) (float64, float64, bool) {
	switch NormalizeLLMProvider(provider) {
	case LLMProviderLMStudio, LLMProviderAnthropic:
		return 0, 1, true
	case LLMProviderGemini,
		LLMProviderOpenAI,
		LLMProviderGroq,
		LLMProviderMistral,
		LLMProviderOllama,
		LLMProviderOpenRouter,
		LLMProviderAzureOpenAI:
		return 0, 2, true
	default:
		return 0, 0, false
	}
}

func LLMProviderUsesMaxCompletionTokens(provider string) bool {
	switch NormalizeLLMProvider(provider) {
	case LLMProviderOpenAI,
		LLMProviderGroq,
		LLMProviderOpenRouter,
		LLMProviderAzureOpenAI:
		return true
	default:
		return false
	}
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

// LMStudioReasoningRequestValue returns the reasoning value that is safe to send
// to LM Studio's native API. Disabled reasoning is omitted because some local
// models reject any reasoning configuration, including "off".
func LMStudioReasoningRequestValue(raw string) string {
	reasoning := NormalizeLMStudioReasoning(raw)
	if reasoning == "off" {
		return ""
	}
	return reasoning
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

func (c Config) EffectiveAuthProviderLocalEnabled() bool {
	return true
}

func (c Config) EffectiveOIDCAuth() OIDCAuthConfig {
	return NormalizeAuthConfig(c.Auth).OIDC
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

func (c Config) EffectiveEnvironment() string {
	env := strings.ToLower(strings.TrimSpace(c.Environment))
	switch env {
	case "", "dev", "local":
		return "development"
	case "prod":
		return "production"
	default:
		return env
	}
}

func (c Config) RequiresProductionGates() bool {
	if c.RequireProductionGates {
		return true
	}
	return c.EffectiveEnvironment() == "production"
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

func (c Config) EffectiveGitBotServiceID() string {
	if id := strings.TrimSpace(c.GitBotServiceID); id != "" {
		return id
	}
	return "git-bot"
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

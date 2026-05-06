package config

import (
	"os"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	LLMProviderGemini   = "gemini"
	LLMProviderLMStudio = "lmstudio"
)

var validLMStudioReasoningLevels = map[string]struct{}{
	"":       {},
	"off":    {},
	"low":    {},
	"medium": {},
	"high":   {},
	"on":     {},
}

// Config holds all configuration for the application.
type Config struct {
	DatabaseURL string `yaml:"database_url" env:"DATABASE_URL"`
	LogLevel    string `yaml:"log_level" env:"LOG_LEVEL"`
	LogFormat   string `yaml:"log_format" env:"LOG_FORMAT"`

	MasterKey string `yaml:"master_key" env:"NOPSAI_MASTER_KEY"`

	// Authentication and authorization
	OIDCIssuer               string `yaml:"oidc_issuer" env:"OIDC_ISSUER"`
	OIDCAudience             string `yaml:"oidc_audience" env:"OIDC_AUDIENCE"`
	OIDCJwksURL              string `yaml:"oidc_jwks_url" env:"OIDC_JWKS_URL"`
	OIDCClientID             string `yaml:"oidc_client_id" env:"OIDC_CLIENT_ID"`
	OIDCClientSecret         string `yaml:"oidc_client_secret" env:"OIDC_CLIENT_SECRET"`
	JWTSigningKey            string `yaml:"jwt_signing_key" env:"JWT_SIGNING_KEY"`
	JWTRSAKeyPath            string `yaml:"jwt_rsa_key_path" env:"JWT_RSA_KEY_PATH"`
	JWTIssuer                string `yaml:"jwt_issuer" env:"JWT_ISSUER"`
	JWTAudience              string `yaml:"jwt_audience" env:"JWT_AUDIENCE"`
	JWTExpiryMinutes         int    `yaml:"jwt_expiry_minutes" env:"JWT_EXPIRY_MINUTES"`
	IdleTimeoutMinutes       int    `yaml:"idle_timeout_minutes" env:"IDLE_TIMEOUT_MINUTES"`
	RefreshTokenTTLMinutes   int    `yaml:"refresh_token_ttl_minutes" env:"REFRESH_TOKEN_TTL_MINUTES"`
	AuthProviderLocalEnabled bool   `yaml:"auth_provider_local_enabled" env:"AUTH_PROVIDER_LOCAL_ENABLED"`
	AuthProviderOIDCEnabled  bool   `yaml:"auth_provider_oidc_enabled" env:"AUTH_PROVIDER_OIDC_ENABLED"`
	DefaultTenant            string `yaml:"default_tenant" env:"DEFAULT_TENANT"`
	RateLimitLoginPerMinute  int    `yaml:"rate_limit_login_per_minute" env:"RATE_LIMIT_LOGIN_PER_MINUTE"`
	LoginLockoutThreshold    int    `yaml:"login_lockout_threshold" env:"LOGIN_LOCKOUT_THRESHOLD"`
	LoginLockoutWindowMin    int    `yaml:"login_lockout_window_minutes" env:"LOGIN_LOCKOUT_WINDOW_MINUTES"`

	LLMProvider            string `yaml:"llm_provider" env:"LLM_PROVIDER"`
	GeminiAPIKey           string `yaml:"gemini_api_key" env:"GEMINI_API_KEY"`
	GeminiModel            string `yaml:"gemini_model" env:"GEMINI_MODEL"`
	LMStudioBaseURL        string `yaml:"lmstudio_base_url" env:"LMSTUDIO_BASE_URL"`
	LMStudioAPIKey         string `yaml:"lmstudio_api_key" env:"LMSTUDIO_API_KEY"`
	LMStudioModel          string `yaml:"lmstudio_model" env:"LMSTUDIO_MODEL"`
	LMStudioReasoning      string `yaml:"lmstudio_reasoning" env:"LMSTUDIO_REASONING"`
	LMStudioEnableThinking bool   `yaml:"lmstudio_enable_thinking" env:"LMSTUDIO_ENABLE_THINKING"`

	ConfigRepoURL string `yaml:"config_repo_url" env:"CONFIG_REPO_URL"`

	// Addresses for services to listen on
	NopsaiListenAddress     string `yaml:"nopsai_listen_address" env:"NOPSAI_LISTEN_ADDRESS"`
	GitBotListenAddress     string `yaml:"git_bot_listen_address" env:"GIT_BOT_LISTEN_ADDRESS"`
	DispatcherListenAddress string `yaml:"dispatcher_listen_address" env:"DISPATCHER_LISTEN_ADDRESS"`

	// Addresses for services to connect to each other
	AgentLlmAgentAddress string `yaml:"agent_llm_agent_address" env:"AGENT_LLM_AGENT_ADDRESS"`
	AgentNopsaiAPIURL    string `yaml:"agent_nopsai_api_url" env:"AGENT_NOPSAI_API_URL"`
	GitBotNopsaiAPIURL   string `yaml:"git_bot_nopsai_api_url" env:"GIT_BOT_NOPSAI_API_URL"`
	DispatcherAddress    string `yaml:"dispatcher_address" env:"DISPATCHER_ADDRESS"`

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

	DispatcherRouting map[string][]string `yaml:"dispatcher_routing" env:"DISPATCHER_ROUTING"`
	RunnerID          string              `yaml:"runner_id" env:"RUNNER_ID"`
	RunnerScopes      string              `yaml:"runner_scopes" env:"RUNNER_SCOPES"`
	RunnerCapacity    int                 `yaml:"runner_capacity" env:"RUNNER_CAPACITY"`
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

	// Override with environment variables
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

	if config.LMStudioAPIKey == "" {
		config.LMStudioAPIKey = os.Getenv("LM_API_TOKEN")
	}
	config.LLMProvider = NormalizeLLMProvider(config.LLMProvider)
	config.LMStudioReasoning = NormalizeLMStudioReasoning(config.LMStudioReasoning)

	return config, nil
}

func NormalizeLLMProvider(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))

	switch normalized {
	case "", "gemini", "google", "google-gemini":
		return LLMProviderGemini
	case "lmstudio", "lm-studio", "openai-compatible", "openai_compatible":
		return LLMProviderLMStudio
	default:
		return normalized
	}
}

func (c Config) GetLLMProvider() string {
	return NormalizeLLMProvider(c.LLMProvider)
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

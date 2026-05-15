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

	LLMProvider       string                `yaml:"llm_provider" env:"LLM_PROVIDER"`
	GeminiAPIKey      string                `yaml:"gemini_api_key" env:"GEMINI_API_KEY"`
	GeminiModel       string                `yaml:"gemini_model" env:"GEMINI_MODEL"`
	LMStudioBaseURL   string                `yaml:"lmstudio_base_url" env:"LMSTUDIO_BASE_URL"`
	LMStudioAPIKey    string                `yaml:"lmstudio_api_key" env:"LMSTUDIO_API_KEY"`
	LMStudioModel     string                `yaml:"lmstudio_model" env:"LMSTUDIO_MODEL"`
	LMStudioReasoning string                `yaml:"lmstudio_reasoning" env:"LMSTUDIO_REASONING"`
	LLMDefaultProfile string                `yaml:"llm_default_profile" env:"LLM_DEFAULT_PROFILE"`
	LLMProfiles       map[string]LLMProfile `yaml:"llm_profiles" env:"LLM_PROFILES"`

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

	if config.LMStudioAPIKey == "" {
		config.LMStudioAPIKey = os.Getenv("LM_API_TOKEN")
	}
	config.LLMProvider = NormalizeLLMProvider(config.LLMProvider)
	config.LMStudioReasoning = NormalizeLMStudioReasoning(config.LMStudioReasoning)
	config.LLMDefaultProfile = NormalizeLLMProfileName(config.LLMDefaultProfile)
	config.LLMProfiles = NormalizeLLMProfiles(config.LLMProfiles)

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

func (c Config) LegacyLLMProfile() LLMProfile {
	provider := c.GetLLMProvider()
	profile := LLMProfile{Provider: provider}
	switch provider {
	case LLMProviderLMStudio:
		profile.Model = strings.TrimSpace(c.LMStudioModel)
		profile.BaseURL = strings.TrimSpace(c.LMStudioBaseURL)
		profile.APIKeySecret = "LMSTUDIO_API_KEY"
		profile.Reasoning = NormalizeLMStudioReasoning(c.LMStudioReasoning)
	default:
		profile.Provider = LLMProviderGemini
		profile.Model = strings.TrimSpace(c.GeminiModel)
		profile.APIKeySecret = "GEMINI_API_KEY"
	}
	return NormalizeLLMProfile(profile)
}

func (c Config) EffectiveLLMProfiles() map[string]LLMProfile {
	profiles := NormalizeLLMProfiles(c.LLMProfiles)
	if len(profiles) == 0 {
		return map[string]LLMProfile{
			c.EffectiveLLMDefaultProfile(): c.LegacyLLMProfile(),
		}
	}

	return profiles
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

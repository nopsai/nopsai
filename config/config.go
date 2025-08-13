package config

import (
	"os"
	"reflect"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the application.
type Config struct {
	DatabaseURL string `yaml:"database_url" env:"DATABASE_URL"`
	LogLevel    string `yaml:"log_level" env:"LOG_LEVEL"`
	LogFormat   string `yaml:"log_format" env:"LOG_FORMAT"` // "console" or "json"

	GeminiAPIKey string `yaml:"gemini_api_key" env:"GEMINI_API_KEY"`
	GeminiModel  string `yaml:"gemini_model" env:"GEMINI_MODEL"`

	// Addresses for services to listen on
	NopsaiListenAddress   string `yaml:"nopsai_listen_address" env:"NOPSAI_LISTEN_ADDRESS"`
	LlmAgentListenAddress string `yaml:"llm_agent_listen_address" env:"LLM_AGENT_LISTEN_ADDRESS"`
	GitBotListenAddress   string `yaml:"git_bot_listen_address" env:"GIT_BOT_LISTEN_ADDRESS"`

	// Addresses for services to connect to each other
	AgentLlmAgentAddress string `yaml:"agent_llm_agent_address" env:"AGENT_LLM_AGENT_ADDRESS"`
	AgentNopsaiAPIURL    string `yaml:"agent_nopsai_api_url" env:"AGENT_NOPSAI_API_URL"`
	GitBotNopsaiAPIURL   string `yaml:"git_bot_nopsai_api_url" env:"GIT_BOT_NOPSAI_API_URL"`

	// Git Bot specific configuration
	GitHubWebhookSecret  string `yaml:"github_webhook_secret" env:"GITHUB_WEBHOOK_SECRET"`
	GitHubAppID          string `yaml:"github_app_id" env:"GITHUB_APP_ID"`
	GitHubInstallID      string `yaml:"github_installation_id" env:"GITHUB_INSTALLATION_ID"`
	GitHubPrivateKeyPath string `yaml:"github_private_key_path" env:"GITHUB_PRIVATE_KEY_PATH"`
	NopsaiGitBotAPIURL   string `yaml:"nopsai_git_bot_api_url" env:"NOPSAI_GIT_BOT_API_URL"` // New key

	DockerNetworkName         string `yaml:"docker_network_name" env:"DOCKER_NETWORK_NAME"`
	AutoRemovalAgentContainer bool   `yaml:"auto_removal_agent_container" env:"AUTO_REMOVAL_AGENT_CONTAINER"`
	DefaultPipelineTimeout    string `yaml:"default_pipeline_timeout" env:"DEFAULT_PIPELINE_TIMEOUT"`
	AgentImage                string `yaml:"agent_image" env:"AGENT_IMAGE"`
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
			val.Field(i).SetString(envValue)
		}
	}

	return config, nil
}

package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the application.
type Config struct {
	DatabaseURL string `yaml:"database_url"`
	LogLevel    string `yaml:"log_level"`
	LogFormat   string `yaml:"log_format"` // "console" or "json"

	GeminiAPIKey string `yaml:"gemini_api_key"`
	GeminiModel  string `yaml:"gemini_model"`

	// Addresses for services to listen on
	NopsaiListenAddress   string `yaml:"nopsai_listen_address"`
	LlmAgentListenAddress string `yaml:"llm_agent_listen_address"`

	// Addresses for services to connect to each other (used by nopsai to inject into agents)
	AgentLlmAgentAddress string `yaml:"agent_llm_agent_address"`
	AgentNopsaiAPIURL    string `yaml:"agent_nopsai_api_url"`

	DockerNetworkName         string `yaml:"docker_network_name"`
	AutoRemovalAgentContainer bool   `yaml:"auto_removal_agent_container"`
	DefaultPipelineTimeout    string `yaml:"default_pipeline_timeout"`
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

	return config, nil
}

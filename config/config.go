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

	NopsaiListenAddress   string `yaml:"nopsai_listen_address"`
	ExecutorListenAddress string `yaml:"executor_listen_address"`
	ExecutorAddress       string `yaml:"executor_address"`
	LlmAgentListenAddress string `yaml:"llm_agent_listen_address"`
	AgentLlmAgentAddress  string `yaml:"agent_llm_agent_address"`

	DockerNetworkName string `yaml:"docker_network_name"`
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

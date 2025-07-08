package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	LLMProvider        string  `mapstructure:"llm_provider"`
	GeminiAPIKey       string  `mapstructure:"gemini_api_key"`
	DefaultTimeout     int     `mapstructure:"default_timeout_seconds"`
	LLMModelName       string  `mapstructure:"llm_model_name"`
	LLMMaxOutputTokens int     `mapstructure:"llm_max_output_tokens"`
	LLMTemperature     float64 `mapstructure:"llm_temperature"`
	Verbose            bool    `mapstructure:"verbose"`
	ExecutorRuntime    string  `mapstructure:"executor_runtime"`
}

func LoadConfig(path string) (*Config, error) {
	if path != "" {
		viper.SetConfigFile(path)
	} else {
		viper.AddConfigPath(".")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetDefault("llm_model_name", "gemini-1.5-flash")
	viper.SetDefault("llm_max_output_tokens", 1024)
	viper.SetDefault("llm_temperature", 0.3)
	viper.SetDefault("verbose", false)
	viper.SetDefault("executor_runtime", "docker")
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		} else {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}
	var cfg Config
	err := viper.Unmarshal(&cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to decode config into struct: %w", err)
	}
	return &cfg, nil
}

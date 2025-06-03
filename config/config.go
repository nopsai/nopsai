// config/config.go
package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	LLMProvider         string `mapstructure:"llm_provider"`
	GeminiAPIKey        string `mapstructure:"gemini_api_key"`
	DefaultTimeout      int    `mapstructure:"default_timeout_seconds"`
	LLMModelName        string `mapstructure:"llm_model_name"`
	DefaultExecutionDir string `mapstructure:"default_execution_dir"`
	Verbose             bool   `mapstructure:"verbose"`
}

func LoadConfig(path string) (*Config, error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetDefault("llm_model_name", "gemini-1.5-flash")
	viper.SetDefault("default_execution_dir", "")
	viper.SetDefault("verbose", false)
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

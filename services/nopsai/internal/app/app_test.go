package app

import (
	"testing"

	"nopsai/config"

	"github.com/rs/zerolog"
)

func TestRuntimeEnvFilePathFromEnvDefaultsOff(t *testing.T) {
	got := runtimeEnvFilePathFromEnv(func(string) string { return "" })
	if got != "" {
		t.Fatalf("runtimeEnvFilePathFromEnv() = %q, want empty path", got)
	}
}

func TestRuntimeEnvFilePathFromEnvUsesExplicitPath(t *testing.T) {
	got := runtimeEnvFilePathFromEnv(func(key string) string {
		if key == "ENV_FILE_PATH" {
			return " /app/.env.runtime "
		}
		return ""
	})
	if got != "/app/.env.runtime" {
		t.Fatalf("runtimeEnvFilePathFromEnv() = %q, want explicit path", got)
	}
}

func TestConfigureLoggingDefaultsBlankLevelToInfo(t *testing.T) {
	previousLevel := zerolog.GlobalLevel()
	t.Cleanup(func() { zerolog.SetGlobalLevel(previousLevel) })
	zerolog.SetGlobalLevel(zerolog.Disabled)

	configureLogging(&config.Config{})

	if got := zerolog.GlobalLevel(); got != zerolog.InfoLevel {
		t.Fatalf("GlobalLevel() = %s, want %s", got, zerolog.InfoLevel)
	}
}

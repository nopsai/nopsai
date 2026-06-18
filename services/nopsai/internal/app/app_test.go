package app

import "testing"

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

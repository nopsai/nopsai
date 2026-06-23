package startupgates

import (
	"strings"
	"testing"

	"nopsai/config"
)

func TestDispatcherGatesDoNotBlockDevelopmentDefaults(t *testing.T) {
	if err := ValidateDispatcher(&config.Config{}); err != nil {
		t.Fatalf("ValidateDispatcher() error = %v, want nil", err)
	}
}

func TestDispatcherGatesBlockProductionDefaults(t *testing.T) {
	err := ValidateDispatcher(&config.Config{
		Environment:          "production",
		JWTSigningKey:        "browser-token-secret-012345678901234",
		ServiceJWTSigningKey: "browser-token-secret-012345678901234",
		DispatcherTLSMode:    "disabled",
	})
	if err == nil {
		t.Fatal("ValidateDispatcher() error = nil, want startup gate failure")
	}
	if !strings.Contains(err.Error(), "SERVICE_JWT_SIGNING_KEY must be separate") {
		t.Fatalf("error = %q, want service JWT isolation failure", err.Error())
	}
	if !strings.Contains(err.Error(), "NOPSAI_API_URL") {
		t.Fatalf("error = %q, want NopsAI callback URL failure", err.Error())
	}
}

func TestAgentEnvGatesBlockProductionDefaults(t *testing.T) {
	env := map[string]string{
		"NOPSAI_ENVIRONMENT":       "production",
		"SERVICE_JWT_SIGNING_KEY":  "short",
		"DISPATCHER_TLS_MODE":      "disabled",
		"DISPATCHER_TLS_SECRET":    "short",
		"DISPATCHER_GRPC_ADDRESS":  "",
		"NOPSAI_REQUIRE_GATES_BAD": "ignored",
	}
	err := ValidateAgentEnv(func(key string) string { return env[key] })
	if err == nil {
		t.Fatal("ValidateAgentEnv() error = nil, want startup gate failure")
	}
	for _, want := range []string{"DISPATCHER_GRPC_ADDRESS", "SERVICE_ID", "SERVICE_JWT_SIGNING_KEY", "DISPATCHER_TLS_MODE"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	}
}

func TestAgentEnvGatesAcceptCanonicalDispatcherAddressAlias(t *testing.T) {
	env := map[string]string{
		"NOPSAI_ENVIRONMENT":      "production",
		"SERVICE_ID":              "agent",
		"SERVICE_JWT_SIGNING_KEY": "service-token-secret-012345678901234",
		"DISPATCHER_TLS_MODE":     "mtls",
		"DISPATCHER_TLS_SECRET":   "dispatcher-tls-secret-012345678901",
		"DISPATCHER_GRPC_ADDRESS": "dispatcher.pre-nopsai:9090",
	}
	if err := ValidateAgentEnv(func(key string) string { return env[key] }); err != nil {
		t.Fatalf("ValidateAgentEnv() error = %v, want nil", err)
	}
}

func TestRunnerGatesAcceptProductionConfig(t *testing.T) {
	err := ValidateRunner(&config.Config{
		Environment:          "production",
		JWTSigningKey:        "browser-token-secret-012345678901234",
		ServiceJWTSigningKey: "service-token-secret-012345678901234",
		DispatcherTLSMode:    "mtls",
		DispatcherTLSSecret:  "dispatcher-tls-secret-012345678901",
		DispatcherAddress:    "dispatcher:9090",
	})
	if err != nil {
		t.Fatalf("ValidateRunner() error = %v, want nil", err)
	}
}

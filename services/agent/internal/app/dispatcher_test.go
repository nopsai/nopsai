package app

import (
	"strings"
	"testing"

	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicetls"
)

func TestLoadDispatcherClientConfigUsesEnterpriseFallbacks(t *testing.T) {
	env := map[string]string{
		"DISPATCHER_GRPC_ADDRESS": " dispatcher:9090 ",
		"AGENT_SERVICE_ID":        "agent-fallback",
		serviceauth.EnvSigningKey: "jwt-secret",
		serviceauth.EnvIssuer:     "issuer",
		serviceauth.EnvAudience:   "audience",
		servicetls.EnvMode:        "shared_secret",
		servicetls.EnvServerName:  "dispatcher.internal",
	}

	cfg, err := LoadDispatcherClientConfig(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("LoadDispatcherClientConfig() error = %v", err)
	}
	if cfg.Address != "dispatcher:9090" {
		t.Fatalf("address = %q, want dispatcher:9090", cfg.Address)
	}
	if cfg.ServiceID != "agent-fallback" {
		t.Fatalf("service id = %q, want fallback service id", cfg.ServiceID)
	}
	if cfg.TLSSecret != "jwt-secret" {
		t.Fatalf("tls secret = %q, want signing-key fallback", cfg.TLSSecret)
	}
	if cfg.TLSServerName != "dispatcher.internal" {
		t.Fatalf("server name = %q, want dispatcher.internal", cfg.TLSServerName)
	}
}

func TestLoadDispatcherClientConfigAcceptsCanonicalAddress(t *testing.T) {
	env := map[string]string{
		"DISPATCHER_GRPC_ADDRESS": " dispatcher.nopsai:9090 ",
		serviceauth.EnvSigningKey: "jwt-secret",
	}

	cfg, err := LoadDispatcherClientConfig(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("LoadDispatcherClientConfig() error = %v", err)
	}
	if cfg.Address != "dispatcher.nopsai:9090" {
		t.Fatalf("address = %q, want canonical address", cfg.Address)
	}
}

func TestLoadDispatcherClientConfigRequiresAddress(t *testing.T) {
	_, err := LoadDispatcherClientConfig(func(key string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "DISPATCHER_GRPC_ADDRESS") {
		t.Fatalf("error = %v, want missing dispatcher address", err)
	}
}

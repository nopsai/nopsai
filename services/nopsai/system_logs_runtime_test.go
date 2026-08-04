package nopsai

import (
	"context"
	"testing"

	"nopsai/config"
	"nopsai/services/nopsai/internal/systemlogs"
)

func TestNewSystemLogBrokerRejectsDirectUnixSocket(t *testing.T) {
	enabled := true
	_, err := newSystemLogBroker(&config.Config{
		MasterKey:  "test-key",
		SystemLogs: config.SystemLogsConfig{Enabled: &enabled, DockerHost: "unix:///var/run/docker.sock"},
	}, nil)
	if err == nil {
		t.Fatal("newSystemLogBroker() error = nil, want direct Unix socket rejected")
	}
}

func TestNewSystemLogBrokerUsesUnavailableProviderWhenDisabled(t *testing.T) {
	broker, err := newSystemLogBroker(&config.Config{MasterKey: "test-key"}, nil)
	if err != nil {
		t.Fatalf("newSystemLogBroker() error = %v", err)
	}
	sources, err := broker.ListSources(context.Background())
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if len(sources) != len(systemlogs.DefaultRegistry().Sources()) || sources[0].Available || sources[0].State != "unavailable" {
		t.Fatalf("ListSources() = %#v", sources)
	}
}

func TestNewSystemLogBrokerUsesUnavailableProviderWhenEnabledWithoutProvider(t *testing.T) {
	enabled := true
	broker, err := newSystemLogBroker(&config.Config{MasterKey: "test-key", SystemLogs: config.SystemLogsConfig{Enabled: &enabled}}, nil)
	if err != nil {
		t.Fatalf("newSystemLogBroker() error = %v", err)
	}
	sources, err := broker.ListSources(context.Background())
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if len(sources) != len(systemlogs.DefaultRegistry().Sources()) || sources[0].Available || sources[0].Status == "" {
		t.Fatalf("ListSources() = %#v", sources)
	}
}

func TestEffectiveSystemLogsKubernetesNamespace(t *testing.T) {
	if got := effectiveSystemLogsKubernetesNamespace(" nopsai "); got != "nopsai" {
		t.Fatalf("effective namespace = %q, want configured namespace", got)
	}
}

func TestSystemLogProviderNamesSplitsAndDeduplicates(t *testing.T) {
	got := systemLogProviderNames("kubernetes,docker,kubernetes")
	if len(got) != 2 || got[0] != "kubernetes" || got[1] != "docker" {
		t.Fatalf("provider names = %#v, want kubernetes,docker", got)
	}
}

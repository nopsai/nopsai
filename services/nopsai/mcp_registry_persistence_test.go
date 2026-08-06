package nopsai

import (
	"os"
	"strings"
	"testing"

	"nopsai/config"
	"nopsai/pkg/models"
)

func TestPersistMCPRegistryBootstrapConfigIsOptionalAfterDBPersistence(t *testing.T) {
	app := &App{configPath: t.TempDir()}
	cfg := config.Config{
		MCPServers: map[string]models.MCPServer{
			"gitea-local": {
				Name:      "gitea-local",
				Enabled:   true,
				Provider:  "custom",
				Transport: models.MCPTransportStreamableHTTP,
				URL:       "http://192.168.1.143:8102/mcp",
				AuthType:  models.MCPAuthNone,
				Timeout:   models.DefaultMCPTimeout,
			},
		},
	}

	if err := app.persistMCPRegistryBootstrapConfig(cfg, false); err != nil {
		t.Fatalf("optional bootstrap persistence error = %v, want nil", err)
	}
	if err := app.persistMCPRegistryBootstrapConfig(cfg, true); err == nil {
		t.Fatal("required bootstrap persistence unexpectedly succeeded")
	}
}

func TestPersistMCPRegistryBootstrapConfigWritesRegistry(t *testing.T) {
	path := t.TempDir() + "/config.yml"
	app := &App{configPath: path}
	cfg := config.Config{
		MCPServers: map[string]models.MCPServer{
			"prometheus-local": {
				Name:      "prometheus-local",
				Enabled:   true,
				Provider:  "prometheus",
				Transport: models.MCPTransportStreamableHTTP,
				URL:       "http://192.168.1.143:8101/mcp",
				AuthType:  models.MCPAuthNone,
				Timeout:   models.DefaultMCPTimeout,
				Headers:   map[string]string{"X-MCP-Readonly": "true"},
			},
		},
		MCPProfiles: map[string]models.MCPProfile{
			"prometheus-local-readonly": {
				Name:        "prometheus-local-readonly",
				Description: "Read-only Prometheus tools",
				Enabled:     true,
				ServerRefs: []models.MCPProfileServerRef{{
					ServerName: "prometheus-local",
					Tools:      []string{"*"},
				}},
			},
		},
	}

	if err := app.persistMCPRegistryBootstrapConfig(cfg, true); err != nil {
		t.Fatalf("persistMCPRegistryBootstrapConfig() error = %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, want := range []string{
		"mcp_servers:",
		"prometheus-local:",
		"mcp_profiles:",
		"prometheus-local-readonly:",
	} {
		if !strings.Contains(string(contents), want) {
			t.Fatalf("bootstrap config missing %q:\n%s", want, string(contents))
		}
	}
}

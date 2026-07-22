package nopsai

import (
	"os"
	"strings"
	"testing"
)

func readLicenseContractFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func requireLicenseContractText(t *testing.T, contents, required, source string) {
	t.Helper()
	if !strings.Contains(contents, required) {
		t.Errorf("%s is missing %q", source, required)
	}
}

func TestLicenseComplianceDocumentationCoversCommercialSurfaces(t *testing.T) {
	doc := readLicenseContractFile(t, "doc/license-compliance.md")
	for _, required := range []string{
		"scripts/license-check.sh",
		"GPL/AGPL/LGPL/SSPL-style",
		"MPL-2.0",
		"CC-BY-4.0",
		"gotenberg/gotenberg",
		"ngrok/ngrok",
		"External MCP servers",
		"Customer-provided data",
	} {
		requireLicenseContractText(t, doc, required, "doc/license-compliance.md")
	}

	docsMap := readLicenseContractFile(t, "doc/README.md")
	requireLicenseContractText(t, docsMap, "license-compliance.md", "doc/README.md")

	readme := readLicenseContractFile(t, "Readme.md")
	requireLicenseContractText(t, readme, "scripts/license-check.sh", "Readme.md")

	mcpDoc := readLicenseContractFile(t, "doc/mcp-pipeline-integration.md")
	requireLicenseContractText(t, mcpDoc, "license-compliance.md", "doc/mcp-pipeline-integration.md")
	requireLicenseContractText(t, mcpDoc, "enterprise redistribution", "doc/mcp-pipeline-integration.md")
}

func TestUnusedStarterAssetsAreNotTracked(t *testing.T) {
	for _, path := range []string{
		"services/ui/public/vite.svg",
		"services/ui/src/assets/react.svg",
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s should not be tracked in the commercial UI asset surface", path)
		}
	}
}

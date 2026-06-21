package release

import (
	"os"
	"strings"
	"testing"

	"nopsai/pkg/compatibility"
)

func TestCompatibilityContract(t *testing.T) {
	file, err := os.Open("compatibility.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	contract, err := compatibility.DecodeCompatibility(file)
	if err != nil {
		t.Fatal(err)
	}
	if contract.CLIVersion != "2.7.0" || contract.RunnerProtocolVersion != 1 || !compatibility.HasCapability(contract.Capabilities, compatibility.CapabilityPlatformHelm) {
		t.Fatalf("contract = %#v", contract)
	}
}

func TestManifestTemplateDeclaresEveryPinnedPlatformArtifact(t *testing.T) {
	contents, err := os.ReadFile("manifest.tmpl.json")
	if err != nil {
		t.Fatal(err)
	}
	template := string(contents)
	for _, image := range compatibility.RequiredPlatformImages {
		if !strings.Contains(template, `"`+image+`"`) {
			t.Errorf("manifest template is missing %q", image)
		}
	}
	for _, forbidden := range []string{"@latest", `:latest"`, `:edge"`} {
		if strings.Contains(template, forbidden) {
			t.Errorf("manifest template contains floating reference %q", forbidden)
		}
	}
	if !strings.Contains(template, `"rollbackPolicy": "forward-only"`) {
		t.Fatal("manifest template does not declare migration rollback policy")
	}
}

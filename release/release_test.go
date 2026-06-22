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

func TestCommitCountVersionSeriesMatchesCompatibilityBaseline(t *testing.T) {
	contents, err := os.ReadFile("version.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(contents)); got != "2.7" {
		t.Fatalf("release version series = %q, want 2.7", got)
	}
}

func TestDeploymentTemplatesOwnEveryReleasedImage(t *testing.T) {
	composeBytes, err := os.ReadFile("../deploy/docker-compose.release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	valuesBytes, err := os.ReadFile("../deploy/kubernetes/values.images.yaml.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	compose, values := string(composeBytes), string(valuesBytes)
	for _, required := range []string{"API", "AAA", "AGENT", "DISPATCHER", "GIT_BOT", "RUNNER", "DOCKER_SOCKET_PROXY", "UI"} {
		if !strings.Contains(compose, "NOPSAI_"+required+"_IMAGE") {
			t.Errorf("release Compose template is missing %s image", required)
		}
	}
	for _, required := range []string{"API", "AAA", "AGENT", "DISPATCHER", "GIT_BOT", "RUNNER", "K8S_RUNNER", "UI"} {
		if !strings.Contains(values, "{{"+required+"_DIGEST}}") {
			t.Errorf("Kubernetes values template is missing %s digest", required)
		}
	}
	for _, template := range []string{compose, values} {
		if strings.Contains(template, "nopsai-api:latest") || strings.Contains(template, "nopsai-agent:latest") {
			t.Fatal("release deployment template contains a floating NopsAI image")
		}
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

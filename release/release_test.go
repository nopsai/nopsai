package release

import (
	"os"
	"path/filepath"
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

func TestOnlyPlatformReleasePublishesImagesAndCLIFromMain(t *testing.T) {
	workflowPaths, err := filepath.Glob("../.github/workflows/*.yml")
	if err != nil {
		t.Fatal(err)
	}
	publishers := make([]string, 0, 1)
	var platformWorkflow string
	for _, workflowPath := range workflowPaths {
		contents, readErr := os.ReadFile(workflowPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		workflow := string(contents)
		if strings.Contains(workflow, "packages: write") {
			publishers = append(publishers, filepath.Base(workflowPath))
		}
		if filepath.Base(workflowPath) == "platform-release.yml" {
			platformWorkflow = workflow
		}
	}
	if len(publishers) != 1 || publishers[0] != "platform-release.yml" {
		t.Fatalf("package-publishing workflows = %v, want only platform-release.yml", publishers)
	}
	for _, required := range []string{"branches: [main]", "./cmd/nopsai-cli", "docker/build-push-action"} {
		if !strings.Contains(platformWorkflow, required) {
			t.Errorf("platform release workflow is missing %q", required)
		}
	}
	for _, forbidden := range []string{"\n  pull_request:", "\n    tags:"} {
		if strings.Contains(platformWorkflow, forbidden) {
			t.Errorf("platform release workflow contains non-main trigger %q", forbidden)
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

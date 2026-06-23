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
	valuesBytes, err := os.ReadFile("../deploy/helm/release-images.yaml.tmpl")
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
			t.Errorf("Helm release values template is missing %s digest", required)
		}
	}
	for _, template := range []string{compose, values} {
		if strings.Contains(template, "nopsai-api:latest") || strings.Contains(template, "nopsai-agent:latest") {
			t.Fatal("release deployment template contains a floating NopsAI image")
		}
	}
}

func TestHelmChartOwnsControlPlaneAndRunnerResources(t *testing.T) {
	for _, chartFile := range []string{
		"../deploy/helm/nopsai/Chart.yaml",
		"../deploy/helm/nopsai/values.yaml",
		"../deploy/helm/nopsai/templates/api.yaml",
		"../deploy/helm/nopsai/templates/aaa.yaml",
		"../deploy/helm/nopsai/templates/dispatcher.yaml",
		"../deploy/helm/nopsai/templates/git-bot.yaml",
		"../deploy/helm/nopsai/templates/ui.yaml",
		"../deploy/helm/nopsai/templates/k8s-runner.yaml",
	} {
		if info, err := os.Stat(chartFile); err != nil || info.Size() == 0 {
			t.Errorf("required Helm chart file %s is missing or empty", chartFile)
		}
	}
	workflowBytes, err := os.ReadFile("../.github/workflows/platform-release.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(workflowBytes)
	for _, required := range []string{"helm push", "nopsai-$VERSION.tgz", "oci://ghcr.io/"} {
		if !strings.Contains(workflow, required) {
			t.Errorf("platform release workflow is missing Helm publication contract %q", required)
		}
	}
}

func TestHelmChartConfiguresKubernetesSystemLogs(t *testing.T) {
	valuesBytes, err := os.ReadFile("../deploy/helm/nopsai/values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	apiTemplateBytes, err := os.ReadFile("../deploy/helm/nopsai/templates/api.yaml")
	if err != nil {
		t.Fatal(err)
	}
	helpersBytes, err := os.ReadFile("../deploy/helm/nopsai/templates/_helpers.tpl")
	if err != nil {
		t.Fatal(err)
	}
	values := string(valuesBytes)
	apiTemplate := string(apiTemplateBytes)
	helpers := string(helpersBytes)
	for _, required := range []string{
		"systemLogs:",
		"provider: kubernetes",
		"serviceAccount:",
		"name: nopsai-api",
	} {
		if !strings.Contains(values, required) {
			t.Errorf("values.yaml is missing Kubernetes system logs contract %q", required)
		}
	}
	for _, required := range []string{
		"kind: ServiceAccount",
		"serviceAccountName: {{ include \"nopsai.apiServiceAccountName\" . }}",
		"resources: [pods/log]",
		"verbs: [get]",
		"SYSTEM_LOGS_PROVIDER",
		"SYSTEM_LOGS_KUBERNETES_LABEL_SELECTOR",
	} {
		if !strings.Contains(apiTemplate, required) {
			t.Errorf("api.yaml is missing Kubernetes system logs contract %q", required)
		}
	}
	for _, required := range []string{"nopsai.apiServiceAccountName", "nopsai.systemLogsKubernetesEnabled", "nopsai.systemLogsKubernetesLabelSelector"} {
		if !strings.Contains(helpers, required) {
			t.Errorf("_helpers.tpl is missing %q", required)
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

func TestDarwinCLIReleaseIsSignedAndNotarizedOnMacOS(t *testing.T) {
	workflowBytes, err := os.ReadFile("../.github/workflows/platform-release.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(workflowBytes)
	for _, required := range []string{
		"runs-on: ${{ matrix.runner }}",
		"runner: macos-14",
		"scripts/sign-notarize-macos-cli.sh",
		"APPLE_DEVELOPER_ID_P12_BASE64",
		"APPLE_NOTARY_KEY_P8_BASE64",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("platform release workflow is missing macOS release contract %q", required)
		}
	}

	scriptPath := "../scripts/sign-notarize-macos-cli.sh"
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	for _, required := range []string{
		"codesign",
		"--options runtime",
		"--timestamp",
		"xcrun notarytool submit",
		"spctl --assess --type execute",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("macOS signing script is missing %q", required)
		}
	}
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatal("macOS signing script is not executable")
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

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
	if contract.CLIVersion != "2.7.0" || contract.RunnerProtocolVersion != 1 || !compatibility.HasCapability(contract.Capabilities, compatibility.CapabilityPlatformHelm) || !compatibility.HasCapability(contract.Capabilities, compatibility.CapabilityPlatformCompose) {
		t.Fatalf("contract = %#v", contract)
	}
}

func TestCommitCountVersionSeriesMatchesCompatibilityBaseline(t *testing.T) {
	contents, err := os.ReadFile("version.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(contents)); got != "2.10" {
		t.Fatalf("release version series = %q, want 2.10", got)
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
	for _, required := range []string{"API", "AAA", "AGENT", "DISPATCHER", "GIT_BOT", "RUNNER", "K8S_RUNNER", "DOCKER_SOCKET_PROXY", "UI"} {
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
	pipeline := readNopsAIReleasePipeline(t)
	for _, required := range []string{"helm push", "nopsai-$VERSION.tgz", "oci://ghcr.io/"} {
		if !strings.Contains(pipeline, required) {
			t.Errorf("NopsAI platform release pipeline is missing Helm publication contract %q", required)
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

func TestNopsAIPlatformReleasePublishesImagesAndCLIFromMain(t *testing.T) {
	workflowPaths, err := filepath.Glob("../.github/workflows/*.yml")
	if err != nil {
		t.Fatal(err)
	}
	for _, workflowPath := range workflowPaths {
		contents, readErr := os.ReadFile(workflowPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		workflow := string(contents)
		for _, forbidden := range []string{"packages: write", "docker/build-push-action", "gh release upload"} {
			if strings.Contains(workflow, forbidden) {
				t.Fatalf("GitHub Actions workflow %s still contains release publisher contract %q", filepath.Base(workflowPath), forbidden)
			}
		}
	}

	triggerBytes, err := os.ReadFile("../doc/sample-config-repo/global-repo/triggers/hosein-yousefii/pre-nopsai.yaml")
	if err != nil {
		t.Fatal(err)
	}
	trigger := string(triggerBytes)
	for _, required := range []string{
		"provider: github",
		"team_path: platform/prod",
		"on: push",
		"- main",
		"scope: prod",
		"platform/prod/nopsai-platform-release",
	} {
		if !strings.Contains(trigger, required) {
			t.Errorf("NopsAI release trigger is missing %q", required)
		}
	}
	for _, forbidden := range []string{"pull_request", "tags:", "workflow_run:", "webhook_source:"} {
		if strings.Contains(trigger, forbidden) {
			t.Errorf("NopsAI release trigger contains non-GitHub-App/main-only contract %q", forbidden)
		}
	}

	pipeline := readNopsAIReleasePipeline(t)
	for _, required := range []string{
		"NOPSAI_RELEASE_SOURCE_REF",
		"NOPSAI_RELEASE_ALLOW_EXISTING",
		"git fetch --force --tags origin main:refs/remotes/origin/main",
		"Release source $source_commit is not current origin/main $main_commit",
		`docker compose --env-file "$validation_env" -f dist/release/docker-compose.yaml config --quiet`,
		"scripts/release-tooling-test.sh",
		"./cmd/nopsai-cli",
		"docker buildx build",
		`--build-arg "BASE_IMAGE=$REGISTRY/nopsai-base@$base_digest"`,
		`gh release upload "v$VERSION" dist/assets/* --repo "$GITHUB_REPOSITORY" --clobber`,
	} {
		if !strings.Contains(pipeline, required) {
			t.Errorf("NopsAI platform release pipeline is missing %q", required)
		}
	}
}

func TestPlatformReleasePublishesCLIArtifactsAndParsesHelmDigest(t *testing.T) {
	pipeline := readNopsAIReleasePipeline(t)
	for _, required := range []string{
		"for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64",
		"./cmd/nopsai-cli",
		"rm -rf dist/cli",
		"shopt -s nullglob",
		"cli_assets=(dist/cli/*)",
		"No CLI release artifacts were built into dist/cli",
		`helm push "dist/release/nopsai-$VERSION.tgz" "$chart_repository" 2>&1`,
		`grep -Eo 'sha256:[a-f0-9]{64}'`,
		`tail -1 || true`,
		"apk add --no-cache bash coreutils perl-utils tar gzip",
		"checksum_files=(.env db/init.sql docker-compose.yaml",
		"checksum_files+=(release-manifest.json)",
		"release_manifest=missing packaging release assets without release-manifest.json",
		`cp "${cli_assets[@]}" dist/assets/`,
		"if [[ -f dist/release/release-manifest.json ]]",
	} {
		if !strings.Contains(pipeline, required) {
			t.Errorf("NopsAI platform release pipeline is missing %q", required)
		}
	}
}

func TestPlatformReleasePublishesEveryContainerPackage(t *testing.T) {
	workflowBytes, err := os.ReadFile("../.github/workflows/platform-release.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(workflowBytes)
	for _, image := range []string{
		"nopsai-base",
		"nopsai-api",
		"nopsai-aaa",
		"nopsai-agent",
		"nopsai-dispatcher",
		"nopsai-git-bot",
		"nopsai-runner",
		"nopsai-k8s-runner",
		"nopsai-docker-socket-proxy",
		"nopsai-ui",
		"pipeline-image",
	} {
		if !strings.Contains(workflow, image) {
			t.Errorf("platform release workflow is missing published image %q", image)
		}
	}
	if !strings.Contains(workflow, "image-digest-nopsai-base") || !strings.Contains(workflow, "image-digest-${{ matrix.name }}") {
		t.Fatal("platform release workflow is missing image digest artifacts")
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

func readNopsAIReleasePipeline(t *testing.T) string {
	t.Helper()
	contents, err := os.ReadFile("../doc/sample-config-repo/global-repo/pipelines/platform/prod/nopsai-platform-release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

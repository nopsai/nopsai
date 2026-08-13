package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nopsai/pkg/models"

	"gopkg.in/yaml.v3"
)

func TestNopsAIGitOpsPlatformReleasePipelineValidates(t *testing.T) {
	path := filepath.Join(
		"..", "..", "..", "..",
		".nopsai", "nopsai-platform-release.yaml",
	)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", path, err)
	}

	var pipeline models.Pipeline
	if err := yaml.Unmarshal(raw, &pipeline); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	if err := ValidatePipeline(&pipeline); err != nil {
		t.Fatalf("ValidatePipeline() error = %v", err)
	}
	if models.PipelineLLMEnabled(&pipeline) {
		t.Fatal("GitOps NopsAI release pipeline must keep LLM disabled")
	}

	requireVariables(t, pipeline.Variables,
		"NOPSAI_RELEASE_REPOSITORY_URL",
		"NOPSAI_RELEASE_SOURCE_REF",
		"NOPSAI_RELEASE_REQUIRE_MAIN",
		"NOPSAI_RELEASE_ALLOW_EXISTING",
		"NOPSAI_RELEASE_REGISTRY",
		"NOPSAI_RELEASE_GITHUB_REPOSITORY",
		"NOPSAI_RELEASE_GHCR_USERNAME",
		"NOPSAI_RELEASE_PLATFORMS",
		"NOPSAI_RELEASE_ENABLE_QEMU",
		"NOPSAI_RELEASE_BUILDX_NAME",
		"NOPSAI_RELEASE_HELM_VERSION",
		"NOPSAI_RELEASE_GH_VERSION",
		"DOCKER_HOST",
	)

	steps := stepsByName(pipeline.Steps)
	imageStepNames := []string{
		"publish-image-nopsai-api",
		"publish-image-nopsai-aaa",
		"publish-image-nopsai-agent",
		"publish-image-nopsai-dispatcher",
		"publish-image-nopsai-git-bot",
		"publish-image-nopsai-docker-runner",
		"publish-image-nopsai-k8s-runner",
		"publish-image-nopsai-docker-socket-proxy",
		"publish-image-nopsai-ui",
		"publish-image-pipeline-image",
	}
	for _, name := range []string{
		"checkout-repository",
		"release-metadata",
		"quality-gates",
		"ui-gates",
		"build-cli-archives",
		"publish-base-image",
		"publish-images",
		"package-helm-chart",
		"publish-helm-chart",
		"package-release-assets",
		"publish-release",
	} {
		if _, ok := steps[name]; !ok {
			t.Fatalf("missing release pipeline step %q", name)
		}
	}
	for _, name := range imageStepNames {
		if _, ok := steps[name]; !ok {
			t.Fatalf("missing release pipeline image step %q", name)
		}
	}

	requireSecrets(t, steps["checkout-repository"].GetSecrets(), "NOPSAI_RELEASE_GITHUB_TOKEN")
	requireSecrets(t, steps["release-metadata"].GetSecrets(), "NOPSAI_RELEASE_GITHUB_TOKEN")
	requireSecrets(t, steps["publish-base-image"].GetSecrets(), "NOPSAI_RELEASE_GHCR_TOKEN")
	for _, name := range imageStepNames {
		requireSecrets(t, steps[name].GetSecrets(), "NOPSAI_RELEASE_GHCR_TOKEN")
		requireDependsOn(t, steps[name].GetDependsOn(), "publish-base-image")
	}
	requireSecrets(t, steps["publish-helm-chart"].GetSecrets(), "NOPSAI_RELEASE_GHCR_TOKEN")
	requireSecrets(t, steps["publish-release"].GetSecrets(), "NOPSAI_RELEASE_GITHUB_TOKEN")
	requireDependsOn(t, steps["publish-images"].GetDependsOn(), imageStepNames...)
	requireDependsOn(t, steps["package-helm-chart"].GetDependsOn(), "publish-images")
	requireDependsOn(t, steps["publish-helm-chart"].GetDependsOn(), "package-helm-chart")
	requireDependsOn(t, steps["build-cli-archives"].GetDependsOn(), "publish-helm-chart")
	requireDependsOn(t, steps["package-release-assets"].GetDependsOn(), "build-cli-archives", "publish-helm-chart")
	requireDependsOn(t, steps["publish-release"].GetDependsOn(), "package-release-assets")
	requireContains(t, steps["release-metadata"].GetScript(), "GIT_ASKPASS")
	requireContains(t, steps["checkout-repository"].GetScript(), "git rev-parse --absolute-git-dir")
	requireContains(t, steps["release-metadata"].GetScript(), "git rev-parse --absolute-git-dir")
	requireContains(t, steps["publish-release"].GetScript(), "git rev-parse --absolute-git-dir")
	requireContains(t, steps["checkout-repository"].GetScript(), "git fetch --force --tags origin")
	requireContains(t, steps["quality-gates"].GetScript(), "apk add --no-cache bash build-base")
	requireContains(t, steps["quality-gates"].GetScript(), "go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2")
	requireContains(t, steps["quality-gates"].GetScript(), "go install github.com/securego/gosec/v2/cmd/gosec@v2.27.1")
	requireContains(t, steps["quality-gates"].GetScript(), "go install golang.org/x/vuln/cmd/govulncheck@v1.6.0")
	requireContains(t, steps["quality-gates"].GetScript(), "SKIP_DOCKER_BUILDS=1 scripts/enterprise-gates.sh")
	requireContains(t, steps["ui-gates"].GetScript(), "NPM_CONFIG_FOREGROUND_SCRIPTS=true")
	requireContains(t, steps["ui-gates"].GetScript(), "node --version")
	requireContains(t, steps["ui-gates"].GetScript(), "npm ci --foreground-scripts --no-audit --no-fund --loglevel=notice")
	requireContains(t, steps["build-cli-archives"].GetScript(), "asset=\"nopsai-cli_${VERSION}_${goos}_${goarch}\"")
	requireContains(t, steps["build-cli-archives"].GetScript(), "nopsai/pkg/buildinfo.APIVersion=${API_VERSION}")
	requireContains(t, steps["build-cli-archives"].GetScript(), "nopsai/pkg/buildinfo.PlatformCompatibility=${PLATFORM_COMPATIBILITY}")
	requireContains(t, steps["release-metadata"].GetScript(), "release/compatibility.yaml")
	requireContains(t, steps["release-metadata"].GetScript(), "CLI_COMPATIBILITY")
	requireContains(t, steps["publish-base-image"].GetScript(), "--build-arg \"PLATFORM_COMPATIBILITY=$PLATFORM_COMPATIBILITY\"")
	requireContains(t, steps["publish-base-image"].GetScript(), "--build-arg \"CAPABILITIES=$CAPABILITIES\"")
	requireContains(t, steps["release-metadata"].GetScript(), "source_url=\"https://github.com/$repo_owner_lower/$repo_name_lower\"")
	requireContains(t, steps["release-metadata"].GetScript(), "printf 'SOURCE_URL=%q\\n' \"$source_url\"")
	requireContains(t, steps["release-metadata"].GetScript(), "scripts/release-tags.sh \"$version\"")
	requireContains(t, steps["release-metadata"].GetScript(), "printf 'RELEASE_TAGS=%q\\n' \"$release_tags_csv\"")
	requireContains(t, steps["release-metadata"].GetScript(), "cp scripts/install-release-tools.sh dist/release/install-release-tools.sh")
	if strings.Contains(steps["release-metadata"].GetScript(), "cat >dist/release/install-release-tools.sh") {
		t.Fatal("release-metadata must copy the checked-in release tool installer instead of embedding a generated copy")
	}
	requireContains(t, steps["publish-base-image"].GetScript(), "publish-release-image.sh")
	requireContains(t, steps["publish-base-image"].GetScript(), "nopsai-base")
	requireContains(t, steps["publish-base-image"].GetScript(), `release_tag_args+=(--tag "$REGISTRY/nopsai-base:$release_tag")`)
	requireContains(t, steps["publish-base-image"].GetScript(), `release_tag_args+=(--tag "$REGISTRY/$image_name:$release_tag")`)
	requireContains(t, steps["publish-base-image"].GetScript(), `done < <(scripts/release-tags.sh "$VERSION")`)
	requireContains(t, steps["publish-base-image"].GetScript(), `--annotation "index,manifest:org.opencontainers.image.source=$SOURCE_URL"`)
	requireContains(t, steps["publish-base-image"].GetScript(), `--annotation "index,manifest:org.opencontainers.image.title=$image_name"`)
	requireContains(t, steps["publish-base-image"].GetScript(), `--annotation "index,manifest:org.opencontainers.image.title=nopsai-base"`)
	requireContains(t, steps["publish-base-image"].GetScript(), `"${oci_annotation_args[@]}"`)
	requireContains(t, steps["publish-base-image"].GetScript(), "--build-arg \"SOURCE_URL=$SOURCE_URL\"")
	requireContains(t, steps["publish-image-pipeline-image"].GetScript(), "publish-release-image.sh pipeline-image . container/Dockerfile.pipeline")
	requireContains(t, steps["publish-images"].GetScript(), "dist/digests/${image_name}.digest")
	requireContains(t, steps["publish-images"].GetScript(), "verified_image=$REGISTRY/$image_name@$digest")
	requireContains(t, steps["package-helm-chart"].GetScript(), "helm lint dist/release/chart")
	requireContains(t, steps["package-helm-chart"].GetScript(), "helm package dist/release/chart --destination dist/release")
	requireContains(t, steps["package-helm-chart"].GetScript(), "scripts/generate-changelog.sh")
	requireContains(t, steps["package-helm-chart"].GetScript(), "rm -rf dist/assets")
	requireContains(t, steps["package-helm-chart"].GetScript(), `version = ARGV.fetch(0)`)
	requireContains(t, steps["package-helm-chart"].GetScript(), `' "$VERSION"`)
	if strings.Contains(steps["package-helm-chart"].GetScript(), `ENV.fetch("VERSION")`) {
		t.Fatal("package-helm-chart must pass VERSION to Ruby explicitly instead of requiring an exported shell variable")
	}
	requireContains(t, steps["publish-helm-chart"].GetScript(), "NOPSAI_RELEASE_GHCR_TOKEN is required to publish the Helm chart to GHCR")
	requireContains(t, steps["publish-helm-chart"].GetScript(), "printf '%s' \"$NOPSAI_RELEASE_GHCR_TOKEN\" | helm registry login ghcr.io")
	requireContains(t, steps["publish-helm-chart"].GetScript(), "printf '%s' \"$NOPSAI_RELEASE_GHCR_TOKEN\" | oras login ghcr.io")
	requireContains(t, steps["publish-helm-chart"].GetScript(), "chart_repository=\"oci://$REGISTRY/charts\"")
	requireContains(t, steps["publish-helm-chart"].GetScript(), "helm push \"dist/release/nopsai-$VERSION.tgz\" \"$chart_repository\" 2>&1")
	requireContains(t, steps["publish-helm-chart"].GetScript(), `oras copy "$oras_chart_reference:$VERSION" "$oras_chart_reference:$release_tag"`)
	requireContains(t, steps["publish-helm-chart"].GetScript(), "tail -1 || true")
	requireContains(t, steps["publish-helm-chart"].GetScript(), "release_phase=publish-helm-chart")
	requireContains(t, steps["package-release-assets"].GetScript(), "apk add --no-cache bash coreutils perl-utils tar gzip")
	requireContains(t, steps["package-release-assets"].GetScript(), "helm_chart_asset=\"nopsai-helm-chart-$VERSION.tgz\"")
	requireContains(t, steps["package-release-assets"].GetScript(), "changelog_asset=\"nopsai-changelog-$VERSION.md\"")
	requireContains(t, steps["package-release-assets"].GetScript(), "cp \"dist/release/nopsai-$VERSION.tgz\" \"dist/assets/$helm_chart_asset\"")
	requireContains(t, steps["package-release-assets"].GetScript(), "release_phase=package-release-assets")
	requireContains(t, steps["publish-release"].GetScript(), "release_phase=publish-github-release")
	requireContains(t, steps["publish-release"].GetScript(), "failed line=$LINENO command=$BASH_COMMAND exit_code=$status")
	requireContains(t, steps["publish-release"].GetScript(), "apk add --no-cache bash curl git tar gzip")
	requireContains(t, steps["publish-release"].GetScript(), "configure_git_auth")
	requireContains(t, steps["publish-release"].GetScript(), "publish_alias_release()")
	requireContains(t, steps["publish-release"].GetScript(), `publish_alias_release "$release_tag" "v$release_tag"`)
	requireContains(t, steps["publish-release"].GetScript(), `done < <(scripts/release-tags.sh "$VERSION")`)
	requireContains(t, steps["publish-release"].GetScript(), "github_release_action=create")
	requireContains(t, steps["publish-release"].GetScript(), "github_release_action=update")
	requireContains(t, steps["publish-release"].GetScript(), "legacy_assets=(")
	requireContains(t, steps["publish-release"].GetScript(), "gh release delete-asset \"v$VERSION\" \"$asset\"")
	requireContains(t, steps["publish-release"].GetScript(), "gh release upload \"v$VERSION\" dist/assets/* --repo \"$GITHUB_REPOSITORY\" --clobber")
	forbidden := []string{"release-manifest.json", "release-index.json", "deployment_bundle_asset=", "compose_asset=", "render-release-bundle", "validate-release-compose", "/tmp/nopsai-git-askpass"}
	for _, step := range pipeline.Steps {
		script := step.GetScript()
		for _, value := range forbidden {
			if strings.Contains(script, value) {
				t.Fatalf("GitOps release pipeline should not contain %q in step %s", value, step.GetName())
			}
		}
	}
}

func TestQuickstartSampleResourcesValidate(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..", "..")
	teamRepo := filepath.Join(repoRoot, "examples", "gitops-quickstart", "team-repo")

	pipelines := map[string]models.Pipeline{}
	pipelineDir := filepath.Join(teamRepo, "pipelines", "platform")
	entries, err := os.ReadDir(pipelineDir)
	if err != nil {
		t.Fatalf("os.ReadDir(%q) error = %v", pipelineDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		pipeline := readSamplePipeline(t, filepath.Join(pipelineDir, entry.Name()))
		if err := ValidatePipeline(&pipeline); err != nil {
			t.Fatalf("ValidatePipeline(%s) error = %v", entry.Name(), err)
		}
		if pipeline.Name != strings.TrimSuffix(entry.Name(), ".yaml") {
			t.Fatalf("%s pipeline name = %q, want the file name", entry.Name(), pipeline.Name)
		}
		if pipeline.Description == "" {
			t.Fatalf("%s should describe what the sample demonstrates", entry.Name())
		}
		pipelines[pipeline.Name] = pipeline
	}

	for _, name := range []string{
		"hello-world",
		"build-and-test",
		"deploy-service",
		"release-notes",
		"service-health-dashboard",
	} {
		if _, ok := pipelines[name]; !ok {
			t.Fatalf("quickstart sample is missing pipeline %q", name)
		}
	}

	buildSteps := stepsByName(pipelines["build-and-test"].Steps)
	requireDependsOn(t, buildSteps["build-image"].GetDependsOn(), "test")
	requireVariables(t, pipelines["build-and-test"].Variables, "API_VERSION", "IMAGE_NAME")

	deploySteps := stepsByName(pipelines["deploy-service"].Steps)
	approval, ok := deploySteps["approve-deployment"].AsApprovalStep()
	if !ok {
		t.Fatal("deploy-service should demonstrate a human approval step")
	}
	if len(approval.Approval.Teams) != 1 || approval.Approval.Teams[0] != "platform" {
		t.Fatalf("approval teams = %#v, want the platform team", approval.Approval.Teams)
	}
	requireSecrets(t, deploySteps["deploy"].GetSecrets(), "DEPLOY_TOKEN")

	releaseNotes := pipelines["release-notes"]
	if !models.PipelineLLMEnabled(&releaseNotes) {
		t.Fatal("release-notes should demonstrate an LLM-backed goal task")
	}
	if len(releaseNotes.KnowledgeContext) != 2 {
		t.Fatalf("release-notes knowledge context = %#v, want architecture and guardrail documents", releaseNotes.KnowledgeContext)
	}

	stepDir := filepath.Join(teamRepo, "steps", "platform", "shared")
	stepEntries, err := os.ReadDir(stepDir)
	if err != nil {
		t.Fatalf("os.ReadDir(%q) error = %v", stepDir, err)
	}
	if len(stepEntries) == 0 {
		t.Fatal("quickstart sample should ship reusable steps")
	}
	for _, entry := range stepEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		step := readSampleStep(t, filepath.Join(stepDir, entry.Name()))
		if err := ValidateReusableStep(&step); err != nil {
			t.Fatalf("ValidateReusableStep(%s) error = %v", entry.Name(), err)
		}
	}
}

func readSamplePipeline(t *testing.T, path string) models.Pipeline {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", path, err)
	}
	var pipeline models.Pipeline
	if err := yaml.Unmarshal(raw, &pipeline); err != nil {
		t.Fatalf("yaml.Unmarshal(%q) error = %v", path, err)
	}
	return pipeline
}

func readSampleStep(t *testing.T, path string) models.PipelineStep {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", path, err)
	}
	var step models.PipelineStep
	if err := yaml.Unmarshal(raw, &step); err != nil {
		t.Fatalf("yaml.Unmarshal(%q) error = %v", path, err)
	}
	return step
}

func stepsByName(steps []models.PipelineStep) map[string]models.PipelineStep {
	byName := make(map[string]models.PipelineStep, len(steps))
	for _, step := range steps {
		byName[step.GetName()] = step
	}
	return byName
}

func requireVariables(t *testing.T, got []string, want ...string) {
	t.Helper()
	present := make(map[string]bool, len(got))
	for _, value := range got {
		present[value] = true
	}
	for _, value := range want {
		if !present[value] {
			t.Fatalf("missing pipeline variable %q", value)
		}
	}
}

func requireSecrets(t *testing.T, got []string, want ...string) {
	t.Helper()
	present := make(map[string]bool, len(got))
	for _, value := range got {
		present[value] = true
	}
	for _, value := range want {
		if !present[value] {
			t.Fatalf("missing step secret %q", value)
		}
	}
}

func requireDependsOn(t *testing.T, got []string, want ...string) {
	t.Helper()
	present := make(map[string]bool, len(got))
	for _, value := range got {
		present[value] = true
	}
	for _, value := range want {
		if !present[value] {
			t.Fatalf("missing dependency %q", value)
		}
	}
}

func requireTaskOutputs(t *testing.T, got []models.TaskOutput, want ...string) {
	t.Helper()
	present := make(map[string]bool, len(got))
	for _, output := range got {
		present[output.Name] = true
	}
	for _, value := range want {
		if !present[value] {
			t.Fatalf("missing task output %q", value)
		}
	}
}

func taskOutputSensitive(outputs []models.TaskOutput, name string) bool {
	for _, output := range outputs {
		if output.Name == name {
			return output.Sensitive
		}
	}
	return false
}

func requireContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("script does not contain %q", needle)
	}
}

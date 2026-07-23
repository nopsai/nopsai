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
	requireContains(t, steps["checkout-repository"].GetScript(), "git fetch --force --tags origin")
	requireContains(t, steps["quality-gates"].GetScript(), "apk add --no-cache bash build-base")
	requireContains(t, steps["quality-gates"].GetScript(), "scripts/test-backend.sh")
	requireContains(t, steps["quality-gates"].GetScript(), "scripts/test-backend.sh -race")
	requireContains(t, steps["quality-gates"].GetScript(), "scripts/release-tooling-test.sh")
	requireContains(t, steps["build-cli-archives"].GetScript(), "asset=\"nopsai-cli_${VERSION}_${goos}_${goarch}\"")
	requireContains(t, steps["build-cli-archives"].GetScript(), "nopsai/pkg/buildinfo.APIVersion=v1")
	requireContains(t, steps["publish-base-image"].GetScript(), "publish-release-image.sh")
	requireContains(t, steps["publish-base-image"].GetScript(), "nopsai-base")
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
	requireContains(t, steps["publish-helm-chart"].GetScript(), "helm push \"dist/release/nopsai-$VERSION.tgz\" \"$chart_repository\" 2>&1")
	requireContains(t, steps["publish-helm-chart"].GetScript(), "tail -1 || true")
	requireContains(t, steps["publish-helm-chart"].GetScript(), "release_phase=publish-helm-chart")
	requireContains(t, steps["package-release-assets"].GetScript(), "apk add --no-cache bash coreutils perl-utils tar gzip")
	requireContains(t, steps["package-release-assets"].GetScript(), "helm_chart_asset=\"nopsai-helm-chart-$VERSION.tgz\"")
	requireContains(t, steps["package-release-assets"].GetScript(), "changelog_asset=\"nopsai-changelog-$VERSION.md\"")
	requireContains(t, steps["package-release-assets"].GetScript(), "cp \"dist/release/nopsai-$VERSION.tgz\" \"dist/assets/$helm_chart_asset\"")
	requireContains(t, steps["package-release-assets"].GetScript(), "release_phase=package-release-assets")
	requireContains(t, steps["publish-release"].GetScript(), "release_phase=publish-github-release")
	requireContains(t, steps["publish-release"].GetScript(), "failed line=$LINENO command=$BASH_COMMAND exit_code=$status")
	requireContains(t, steps["publish-release"].GetScript(), "github_release_action=create")
	requireContains(t, steps["publish-release"].GetScript(), "github_release_action=update")
	requireContains(t, steps["publish-release"].GetScript(), "legacy_assets=(")
	requireContains(t, steps["publish-release"].GetScript(), "gh release delete-asset \"v$VERSION\" \"$asset\"")
	requireContains(t, steps["publish-release"].GetScript(), "gh release upload \"v$VERSION\" dist/assets/* --repo \"$GITHUB_REPOSITORY\" --clobber")
	forbidden := []string{"release-manifest.json", "release-index.json", "deployment_bundle_asset=", "compose_asset=", "render-release-bundle", "validate-release-compose"}
	for _, step := range pipeline.Steps {
		script := step.GetScript()
		for _, value := range forbidden {
			if strings.Contains(script, value) {
				t.Fatalf("GitOps release pipeline should not contain %q in step %s", value, step.GetName())
			}
		}
	}
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

func requireContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("script does not contain %q", needle)
	}
}

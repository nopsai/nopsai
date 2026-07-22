package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nopsai/pkg/models"

	"gopkg.in/yaml.v3"
)

func TestSampleNopsAIPlatformReleasePipelineValidates(t *testing.T) {
	path := filepath.Join(
		"..", "..", "..", "..",
		"doc", "sample-config-repo", "global-repo", "pipelines", "platform", "prod", "nopsai-platform-release.yaml",
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
		t.Fatal("sample NopsAI release pipeline must keep LLM disabled")
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
		"publish-image-nopsai-runner",
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
	requireSecrets(t, steps["publish-release"].GetSecrets(), "NOPSAI_RELEASE_GITHUB_TOKEN")
	requireSecrets(t, steps["publish-release"].GetSecrets(), "NOPSAI_RELEASE_GHCR_TOKEN")
	requireDependsOn(t, steps["publish-images"].GetDependsOn(), imageStepNames...)
	requireContains(t, steps["release-metadata"].GetScript(), "GIT_ASKPASS")
	requireContains(t, steps["checkout-repository"].GetScript(), "git fetch --force --tags origin")
	requireContains(t, steps["quality-gates"].GetScript(), "apk add --no-cache bash build-base")
	requireContains(t, steps["quality-gates"].GetScript(), "scripts/test-backend.sh")
	requireContains(t, steps["quality-gates"].GetScript(), "scripts/test-backend.sh -race")
	requireContains(t, steps["quality-gates"].GetScript(), "scripts/release-tooling-test.sh")
	requireContains(t, steps["publish-base-image"].GetScript(), "publish-release-image.sh")
	requireContains(t, steps["publish-base-image"].GetScript(), "nopsai-base")
	requireContains(t, steps["publish-image-pipeline-image"].GetScript(), "publish-release-image.sh pipeline-image . container/Dockerfile.pipeline")
	requireContains(t, steps["publish-images"].GetScript(), "dist/digests/${image_name}.digest")
	requireContains(t, steps["publish-images"].GetScript(), "verified_image=$REGISTRY/$image_name@$digest")
	requireContains(t, steps["publish-release"].GetScript(), "scripts/render-release-bundle.sh")
	requireContains(t, steps["publish-release"].GetScript(), "NOPSAI_RELEASE_GHCR_TOKEN is required to publish the Helm chart to GHCR")
	requireContains(t, steps["publish-release"].GetScript(), "printf '%s' \"$NOPSAI_RELEASE_GHCR_TOKEN\" | helm registry login ghcr.io")
	requireContains(t, steps["publish-release"].GetScript(), "cp dist/release/release-manifest.json dist/assets/")
	requireContains(t, steps["publish-release"].GetScript(), "gh release upload \"v$VERSION\" dist/assets/* --repo \"$GITHUB_REPOSITORY\" --clobber")
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

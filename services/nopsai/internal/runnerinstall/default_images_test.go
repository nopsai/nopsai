package runnerinstall

import (
	"strings"
	"testing"

	"nopsai/pkg/buildinfo"
)

func TestDefaultRunnerImagesUseBuildVersion(t *testing.T) {
	oldVersion := buildinfo.Version
	defer func() { buildinfo.Version = oldVersion }()

	testVersion := testRunnerImageVersion()
	buildinfo.Version = testVersion
	if got := DefaultRunnerImage(); got != "ghcr.io/nopsai/nopsai-docker-runner:"+testVersion {
		t.Fatalf("DefaultRunnerImage() = %q", got)
	}
	if got := DefaultK8sImage(); got != "ghcr.io/nopsai/nopsai-k8s-runner:"+testVersion {
		t.Fatalf("DefaultK8sImage() = %q", got)
	}
	if strings.Contains(DefaultRunnerImage(), ":latest") || strings.Contains(DefaultK8sImage(), ":latest") {
		t.Fatal("runner image defaults must not use latest")
	}
}

func testRunnerImageVersion() string {
	for _, field := range strings.Fields(strings.ReplaceAll(buildinfo.DefaultPlatformCompatibility, ",", " ")) {
		if strings.HasPrefix(field, ">=") {
			return strings.TrimPrefix(field, ">=")
		}
		if strings.HasPrefix(field, "=") {
			return strings.TrimPrefix(field, "=")
		}
	}
	panic("default platform compatibility does not declare a lower bound")
}

func TestDefaultRunnerImagesUseDevForDevelopmentBuilds(t *testing.T) {
	oldVersion := buildinfo.Version
	defer func() { buildinfo.Version = oldVersion }()

	buildinfo.Version = "unknown"
	if got := DefaultRunnerImage(); got != "ghcr.io/nopsai/nopsai-docker-runner:dev" {
		t.Fatalf("DefaultRunnerImage() = %q", got)
	}
}

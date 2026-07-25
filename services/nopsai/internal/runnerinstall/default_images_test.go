package runnerinstall

import (
	"strings"
	"testing"

	"nopsai/pkg/buildinfo"
)

func TestDefaultRunnerImagesUseBuildVersion(t *testing.T) {
	oldVersion := buildinfo.Version
	defer func() { buildinfo.Version = oldVersion }()

	buildinfo.Version = "2.10.648"
	if got := DefaultRunnerImage(); got != "ghcr.io/nopsai/nopsai-docker-runner:2.10.648" {
		t.Fatalf("DefaultRunnerImage() = %q", got)
	}
	if got := DefaultK8sImage(); got != "ghcr.io/nopsai/nopsai-k8s-runner:2.10.648" {
		t.Fatalf("DefaultK8sImage() = %q", got)
	}
	if strings.Contains(DefaultRunnerImage(), ":latest") || strings.Contains(DefaultK8sImage(), ":latest") {
		t.Fatal("runner image defaults must not use latest")
	}
}

func TestDefaultRunnerImagesUseDevForDevelopmentBuilds(t *testing.T) {
	oldVersion := buildinfo.Version
	defer func() { buildinfo.Version = oldVersion }()

	buildinfo.Version = "unknown"
	if got := DefaultRunnerImage(); got != "ghcr.io/nopsai/nopsai-docker-runner:dev" {
		t.Fatalf("DefaultRunnerImage() = %q", got)
	}
}

package dockerexec

import "testing"

func TestBuildStepContainerNameIncludesRepository(t *testing.T) {
	got := BuildStepContainerName("payments api", "deploy/prod", "ship now", "1234567890abcdef")
	want := "payments-api-deployprod-ship-now-12345678"
	if got != want {
		t.Fatalf("BuildStepContainerName() = %q, want %q", got, want)
	}
}

func TestBuildStepContainerNameWithoutRepository(t *testing.T) {
	got := BuildStepContainerName("", "deploy prod", "ship_now", "1234567890abcdef")
	want := "deploy-prod-ship_now-12345678"
	if got != want {
		t.Fatalf("BuildStepContainerName() = %q, want %q", got, want)
	}
}

func TestBuildStepContainerNameKeepsShortRunID(t *testing.T) {
	got := BuildStepContainerName("", "pipeline", "step", "abc")
	want := "pipeline-step-abc"
	if got != want {
		t.Fatalf("BuildStepContainerName() = %q, want %q", got, want)
	}
}

func TestParseDockerStepVolumeSpecRequiresAbsoluteMount(t *testing.T) {
	volumeName, mountPath, err := parseDockerStepVolumeSpec("nopsai-cache:/cache")
	if err != nil {
		t.Fatalf("parseDockerStepVolumeSpec() error = %v", err)
	}
	if volumeName != "nopsai-cache" || mountPath != "/cache" {
		t.Fatalf("parseDockerStepVolumeSpec() = %q/%q, want nopsai-cache//cache", volumeName, mountPath)
	}

	if _, _, err := parseDockerStepVolumeSpec("host-cache:cache"); err == nil {
		t.Fatal("parseDockerStepVolumeSpec() accepted a relative mount path")
	}
	if _, _, err := parseDockerStepVolumeSpec("/var/lib/cache:/cache"); err == nil {
		t.Fatal("parseDockerStepVolumeSpec() accepted a host-path-shaped volume name")
	}
}

func TestDockerStepVolumeOwnershipLabelsAreRunScoped(t *testing.T) {
	labels := dockerStepVolumeLabels("vol-run-123")
	if !dockerStepVolumeOwnedBy(labels, "vol-run-123") {
		t.Fatalf("dockerStepVolumeOwnedBy() = false for matching labels %#v", labels)
	}
	if dockerStepVolumeOwnedBy(labels, "vol-run-456") {
		t.Fatal("dockerStepVolumeOwnedBy() accepted labels from another run")
	}
	if dockerStepVolumeOwnedBy(map[string]string{dockerStepVolumeManagedLabel: "true"}, "vol-run-123") {
		t.Fatal("dockerStepVolumeOwnedBy() accepted incomplete labels")
	}
}

package dockerexec

import (
	"testing"

	"nopsai/pkg/models"
)

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

func TestDockerStepHostConfigUsesSandboxDefaults(t *testing.T) {
	hostConfig := dockerStepHostConfig([]string{"workspace:/workspace"}, dockerStepTmpfs(true), "nopsai-net")

	if len(hostConfig.CapDrop) != 1 || hostConfig.CapDrop[0] != "ALL" {
		t.Fatalf("CapDrop = %#v, want drop ALL", hostConfig.CapDrop)
	}
	if len(hostConfig.SecurityOpt) != 1 || hostConfig.SecurityOpt[0] != "no-new-privileges:true" {
		t.Fatalf("SecurityOpt = %#v, want no-new-privileges", hostConfig.SecurityOpt)
	}
	if !hostConfig.ReadonlyRootfs {
		t.Fatal("ReadonlyRootfs = false, want true")
	}
	if hostConfig.PidsLimit == nil || *hostConfig.PidsLimit != defaultDockerStepPidsLimit {
		t.Fatalf("PidsLimit = %#v, want %d", hostConfig.PidsLimit, defaultDockerStepPidsLimit)
	}
	if hostConfig.NetworkMode != "none" {
		t.Fatalf("NetworkMode = %q, want none for the control-plane network", hostConfig.NetworkMode)
	}
	if hostConfig.Init == nil || !*hostConfig.Init {
		t.Fatalf("Init = %#v, want true", hostConfig.Init)
	}
	if hostConfig.Tmpfs["/tmp"] == "" || hostConfig.Tmpfs["/var/tmp"] == "" {
		t.Fatalf("tmpfs = %#v, want writable temp mounts", hostConfig.Tmpfs)
	}
	if hostConfig.Tmpfs[models.RuntimeOutputsMountPath] != dockerStepOutputsTmpfs {
		t.Fatalf("outputs tmpfs = %q, want %q", hostConfig.Tmpfs[models.RuntimeOutputsMountPath], dockerStepOutputsTmpfs)
	}
}

func TestDockerStepNetworkModeAllowsDedicatedWorkloadNetwork(t *testing.T) {
	if got := dockerStepNetworkMode("pipeline-egress"); got != "pipeline-egress" {
		t.Fatalf("dockerStepNetworkMode() = %q, want dedicated network", got)
	}
	if got := dockerStepNetworkMode(""); got != "none" {
		t.Fatalf("dockerStepNetworkMode(empty) = %q, want none", got)
	}
}

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

func TestDockerStepVolumeLabelsDescribePipelineVolume(t *testing.T) {
	labels := dockerStepVolumeLabels("cache")
	if labels[dockerStepVolumeManagedLabel] != "true" {
		t.Fatalf("managed label = %q, want true", labels[dockerStepVolumeManagedLabel])
	}
	if labels[dockerStepVolumePurposeLabel] != dockerStepVolumePurpose {
		t.Fatalf("purpose label = %q, want %q", labels[dockerStepVolumePurposeLabel], dockerStepVolumePurpose)
	}
	if labels[dockerStepVolumeLogicalLabel] != "cache" {
		t.Fatalf("logical label = %q, want cache", labels[dockerStepVolumeLogicalLabel])
	}
}

func TestDockerStepHostConfigUsesSandboxDefaults(t *testing.T) {
	hostConfig := dockerStepHostConfig([]string{"workspace:/workspace"}, dockerStepTmpfs(true), "nopsai-net")

	if len(hostConfig.CapDrop) != 0 {
		t.Fatalf("CapDrop = %#v, want Docker/image default capabilities", hostConfig.CapDrop)
	}
	if len(hostConfig.SecurityOpt) != 1 || hostConfig.SecurityOpt[0] != "no-new-privileges:true" {
		t.Fatalf("SecurityOpt = %#v, want no-new-privileges", hostConfig.SecurityOpt)
	}
	if hostConfig.ReadonlyRootfs {
		t.Fatal("ReadonlyRootfs = true, want writable root filesystem for package-installing step images")
	}
	if hostConfig.PidsLimit == nil || *hostConfig.PidsLimit != defaultDockerStepPidsLimit {
		t.Fatalf("PidsLimit = %#v, want %d", hostConfig.PidsLimit, defaultDockerStepPidsLimit)
	}
	if hostConfig.NetworkMode != "bridge" {
		t.Fatalf("NetworkMode = %q, want bridge for default step egress", hostConfig.NetworkMode)
	}
	if hostConfig.Init == nil || !*hostConfig.Init {
		t.Fatalf("Init = %#v, want true", hostConfig.Init)
	}
	if _, ok := hostConfig.Tmpfs["/tmp"]; ok {
		t.Fatalf("tmpfs = %#v, want image-provided /tmp behavior", hostConfig.Tmpfs)
	}
	if _, ok := hostConfig.Tmpfs["/var/tmp"]; ok {
		t.Fatalf("tmpfs = %#v, want image-provided /var/tmp behavior", hostConfig.Tmpfs)
	}
	if hostConfig.Tmpfs[models.RuntimeOutputsMountPath] != dockerStepOutputsTmpfs {
		t.Fatalf("outputs tmpfs = %q, want %q", hostConfig.Tmpfs[models.RuntimeOutputsMountPath], dockerStepOutputsTmpfs)
	}
}

func TestDockerStepTmpfsLeavesImageTempDirectoriesAlone(t *testing.T) {
	tmpfs := dockerStepTmpfs(false)
	if len(tmpfs) != 0 {
		t.Fatalf("dockerStepTmpfs(false) = %#v, want no implicit tmpfs mounts", tmpfs)
	}
}

func TestDockerStepNetworkModeAllowsDedicatedWorkloadNetwork(t *testing.T) {
	if got := dockerStepNetworkMode("pipeline-egress"); got != "pipeline-egress" {
		t.Fatalf("dockerStepNetworkMode() = %q, want dedicated network", got)
	}
	if got := dockerStepNetworkMode(""); got != "bridge" {
		t.Fatalf("dockerStepNetworkMode(empty) = %q, want bridge", got)
	}
	if got := dockerStepNetworkMode("nopsai-net"); got != "bridge" {
		t.Fatalf("dockerStepNetworkMode(nopsai-net) = %q, want bridge for default step egress", got)
	}
	if got := dockerStepNetworkMode("none"); got != "none" {
		t.Fatalf("dockerStepNetworkMode(none) = %q, want none", got)
	}
}

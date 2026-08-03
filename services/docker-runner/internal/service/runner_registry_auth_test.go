package service

import "testing"

func TestNewDockerRunnerKeepsRegistryAuthConfigEnv(t *testing.T) {
	runner, ok := NewDockerRunner(RunnerOptions{
		RunnerID:                 "runner-1",
		RegistryAuthConfigBase64: " eyJhdXRocyI6e319 ",
	}).(*dockerRunner)
	if !ok {
		t.Fatal("NewDockerRunner() did not return dockerRunner")
	}
	if runner.registryAuthConfigBase64 != "eyJhdXRocyI6e319" {
		t.Fatalf("registry auth config env = %q", runner.registryAuthConfigBase64)
	}
}

func TestDockerRunVolumeOwnershipLabelsAreRunScoped(t *testing.T) {
	labels := dockerRunVolumeLabels("run-123")
	if !dockerRunVolumeOwnedBy(labels, "run-123") {
		t.Fatalf("dockerRunVolumeOwnedBy() = false for matching labels %#v", labels)
	}
	if dockerRunVolumeOwnedBy(labels, "run-456") {
		t.Fatal("dockerRunVolumeOwnedBy() accepted labels from another run")
	}
	if dockerRunVolumeOwnedBy(map[string]string{dockerRunVolumeManagedLabel: "true"}, "run-123") {
		t.Fatal("dockerRunVolumeOwnedBy() accepted incomplete labels")
	}
}

package service

import (
	"testing"
)

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

package service

import (
	"context"
	"errors"
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

func TestDockerImagePullOptionsFailsClosedOnResolverError(t *testing.T) {
	wantErr := errors.New("resolver down")
	_, _, err := dockerImagePullOptions(context.Background(), "registry.local/app:latest", failingRegistryAuthResolver{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("dockerImagePullOptions() error = %v, want wrapped resolver error", err)
	}
}

func TestDockerImagePullOptionsUsesResolvedAuth(t *testing.T) {
	options, authenticated, err := dockerImagePullOptions(context.Background(), "registry.local/app:latest", staticRegistryAuthResolver{auth: " encoded-auth "})
	if err != nil {
		t.Fatalf("dockerImagePullOptions() error = %v", err)
	}
	if !authenticated || options.RegistryAuth != "encoded-auth" {
		t.Fatalf("dockerImagePullOptions() = auth %q authenticated %v, want trimmed auth", options.RegistryAuth, authenticated)
	}
}

type failingRegistryAuthResolver struct {
	err error
}

func (r failingRegistryAuthResolver) Resolve(context.Context, string) (string, error) {
	return "", r.err
}

type staticRegistryAuthResolver struct {
	auth string
}

func (r staticRegistryAuthResolver) Resolve(context.Context, string) (string, error) {
	return r.auth, nil
}

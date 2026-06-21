package buildinfo

import (
	"bytes"
	"strings"
	"testing"
)

func TestCurrentNormalizesBuildInputs(t *testing.T) {
	setBuildVariablesForTest(t)
	Version = " 2.7.0 "
	Commit = "abc123"
	BuildDate = "2026-06-21T12:00:00Z"
	ReleaseManifestDigest = "sha256:abc"
	APIVersion = "v1"
	RunnerProtocolVersion = "2"
	Capabilities = "monitoring.v1, api.v1,monitoring.v1"
	info := Current()
	if info.Version != "2.7.0" || info.RunnerProtocolVersion != 2 || len(info.Capabilities) != 2 || info.Capabilities[0] != "api.v1" {
		t.Fatalf("Current = %#v", info)
	}
	public := info.Public()
	info.Capabilities[0] = "changed"
	if public.Capabilities[0] != "api.v1" || public.ProductVersion != "2.7.0" {
		t.Fatalf("Public = %#v", public)
	}
}

func TestCurrentUsesSafeDevelopmentDefaults(t *testing.T) {
	setBuildVariablesForTest(t)
	Version, Commit, BuildDate, APIVersion = "", "", "", ""
	RunnerProtocolVersion = "invalid"
	CLICompatibility, RunnerCompatibility, PlatformCompatibility = "", "", ""
	Capabilities = ""
	info := Current()
	if !info.IsDevelopment() || info.RunnerProtocolVersion != 1 || info.APIVersion != "v1" || info.CLICompatibility == "" {
		t.Fatalf("defaults = %#v", info)
	}
}

func TestVersionRequestAndOutput(t *testing.T) {
	setBuildVariablesForTest(t)
	Version, Commit, BuildDate = "2.7.0", "abc123", "today"
	if !Requested([]string{"--version"}) || !Requested([]string{"version"}) || Requested(nil) || Requested([]string{"--version", "extra"}) {
		t.Fatal("Requested returned unexpected result")
	}
	var output bytes.Buffer
	if err := WriteVersion(&output, "nopsai-api"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "nopsai-api 2.7.0") || !strings.Contains(output.String(), "abc123") {
		t.Fatalf("output = %q", output.String())
	}
}

func setBuildVariablesForTest(t *testing.T) {
	t.Helper()
	values := []string{Version, Commit, BuildDate, ReleaseManifestDigest, APIVersion, RunnerProtocolVersion, CLICompatibility, RunnerCompatibility, PlatformCompatibility, Capabilities}
	t.Cleanup(func() {
		Version, Commit, BuildDate, ReleaseManifestDigest, APIVersion, RunnerProtocolVersion, CLICompatibility, RunnerCompatibility, PlatformCompatibility, Capabilities = values[0], values[1], values[2], values[3], values[4], values[5], values[6], values[7], values[8], values[9]
	})
}

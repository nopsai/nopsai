package compatibility

import (
	"strings"
	"testing"

	"nopsai/pkg/buildinfo"
)

func TestSemverParsingComparisonAndRanges(t *testing.T) {
	tests := []struct {
		left, right string
		comparison  int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0-rc.2", "1.0.0-rc.10", -1},
		{"1.0.0-alpha", "1.0.0-1", 1},
	}
	for _, test := range tests {
		left, err := ParseVersion(test.left)
		if err != nil {
			t.Fatal(err)
		}
		right, err := ParseVersion(test.right)
		if err != nil {
			t.Fatal(err)
		}
		if got := left.Compare(right); got != test.comparison {
			t.Errorf("%s.Compare(%s) = %d", test.left, test.right, got)
		}
	}
	rangeValue, err := ParseRange(">=1.3.0, <2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	inside, _ := ParseVersion("1.9.2")
	outside, _ := ParseVersion("2.0.0")
	if !rangeValue.Contains(inside) || rangeValue.Contains(outside) {
		t.Fatal("range containment is incorrect")
	}
	for _, invalid := range []string{"", "1.2", "01.2.3", "1.2.3-01", "1.2.3+bad!", ">=dev"} {
		if strings.HasPrefix(invalid, ">") {
			if _, err := ParseRange(invalid); err == nil {
				t.Errorf("ParseRange(%q) succeeded", invalid)
			}
		} else if _, err := ParseVersion(invalid); err == nil {
			t.Errorf("ParseVersion(%q) succeeded", invalid)
		}
	}
}

func TestManifestValidation(t *testing.T) {
	manifest := validManifest()
	encoded, err := CanonicalJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeManifest(strings.NewReader(string(encoded)))
	if err != nil || decoded.Version != "2.7.0" {
		t.Fatalf("DecodeManifest = %#v, %v", decoded, err)
	}
	if !strings.HasPrefix(DigestBytes(encoded), "sha256:") {
		t.Fatal("digest is not sha256")
	}
	repository, digest, err := SplitImageReference(manifest.Images["api"])
	if err != nil || repository != "ghcr.io/example/nopsai-api" || digest != testDigest("a") {
		t.Fatalf("SplitImageReference = %q %q %v", repository, digest, err)
	}

	tests := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"schema", func(value *Manifest) { value.SchemaVersion = "v2" }},
		{"chart version", func(value *Manifest) { value.Chart.Version = "2.6.0" }},
		{"chart reference", func(value *Manifest) { value.Chart.Reference = "https://example/chart.tgz" }},
		{"chart digest", func(value *Manifest) { value.Chart.Digest = "latest" }},
		{"missing image", func(value *Manifest) { delete(value.Images, "aaa") }},
		{"floating image", func(value *Manifest) { value.Images["api"] = "ghcr.io/example/nopsai-api:latest" }},
		{"cli range", func(value *Manifest) { value.Compatibility.CLI = "nope" }},
		{"api", func(value *Manifest) { value.Compatibility.API = "" }},
		{"runner", func(value *Manifest) { value.Compatibility.RunnerProtocol = 0 }},
		{"migration", func(value *Manifest) { value.Database.MigrationVersion = -1 }},
		{"rollback policy", func(value *Manifest) { value.Database.RollbackPolicy = "maybe" }},
		{"rollback mismatch", func(value *Manifest) { value.Database.RollbackSafe = true }},
		{"capability", func(value *Manifest) { value.Capabilities = []string{"bad"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := validManifest()
			test.mutate(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("validation succeeded")
			}
		})
	}
}

func TestCLICompatibilityValidation(t *testing.T) {
	cli := buildinfo.Info{Version: "2.7.0", APIVersion: "v1", RunnerProtocolVersion: 1, PlatformCompatibility: ">=2.0.0 <3.0.0"}
	platform := PlatformInfo{ProductVersion: "2.6.0", APIVersion: "v1", CLICompatibility: ">=2.5.0 <3.0.0"}
	if err := ValidatePlatformForCLI(platform, cli); err != nil {
		t.Fatal(err)
	}
	platform.ProductVersion = "3.0.0"
	if err := ValidatePlatformForCLI(platform, cli); err == nil {
		t.Fatal("incompatible platform succeeded")
	}
	manifest := validManifest()
	if err := ValidateManifestForCLI(manifest, cli); err != nil {
		t.Fatal(err)
	}
	manifest.Compatibility.API = "v2"
	if err := ValidateManifestForCLI(manifest, cli); err == nil {
		t.Fatal("incompatible manifest API succeeded")
	}
	if err := RequireCapabilities([]string{CapabilityPlatformHelm}, CapabilityPlatformHelm); err != nil {
		t.Fatal(err)
	}
	if err := RequireCapabilities(nil, CapabilityPlatformHelm); err == nil {
		t.Fatal("missing capability succeeded")
	}
}

func TestDecodeCompatibility(t *testing.T) {
	contract, err := DecodeCompatibility(strings.NewReader(`
cliVersion: 2.7.0
platformCompatibility: ">=2.0.0 <3.0.0"
apiCompatibility: [v1]
runnerCompatibility: ">=2.0.0 <3.0.0"
runnerProtocolVersion: 1
capabilities: [platform.helm, api.v1]
`))
	if err != nil || contract.CLIVersion != "2.7.0" || contract.Capabilities[0] != "api.v1" {
		t.Fatalf("DecodeCompatibility = %#v, %v", contract, err)
	}
	if _, err := DecodeCompatibility(strings.NewReader("cliVersion: nope\nunknown: true\n")); err == nil {
		t.Fatal("invalid compatibility contract succeeded")
	}
}

func validManifest() Manifest {
	images := make(map[string]string, len(RequiredPlatformImages))
	for _, name := range RequiredPlatformImages {
		images[name] = "ghcr.io/example/nopsai-" + strings.ToLower(name) + "@" + testDigest("a")
	}
	return Manifest{
		SchemaVersion: "v1",
		Version:       "2.7.0",
		Chart:         ChartArtifact{Reference: "oci://ghcr.io/example/charts/nopsai", Version: "2.7.0", Digest: testDigest("b")},
		Images:        images,
		Compatibility: ManifestCompatibility{CLI: ">=2.0.0 <3.0.0", API: "v1", RunnerProtocol: 1},
		Database:      DatabaseContract{MigrationVersion: 1, RollbackSafe: false, RollbackPolicy: "forward-only"},
		Capabilities:  []string{CapabilityAPIV1, CapabilityPlatformHelm},
	}
}

func testDigest(character string) string {
	return "sha256:" + strings.Repeat(character, 64)
}

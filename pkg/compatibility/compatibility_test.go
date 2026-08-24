package compatibility

import (
	"strings"
	"testing"

	"nopsai/pkg/buildinfo"
)

var (
	testManifestVersion      = testCompatibilityLowerBound()
	testOtherManifestVersion = testCompatibilityVersionWithPatchOffset(testManifestVersion, 1)
	testUnsupportedVersion   = testCompatibilityUpperBound()
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
	if err != nil || decoded.Version != testManifestVersion {
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
		{"chart version", func(value *Manifest) { value.Chart.Version = testOtherManifestVersion }},
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
	cli := buildinfo.Info{Version: testManifestVersion, APIVersion: "v1", RunnerProtocolVersion: 1, PlatformCompatibility: buildinfo.DefaultPlatformCompatibility}
	platform := PlatformInfo{ProductVersion: testOtherManifestVersion, APIVersion: "v1", CLICompatibility: buildinfo.DefaultCLICompatibility}
	if err := ValidatePlatformForCLI(platform, cli); err != nil {
		t.Fatal(err)
	}
	platform.ProductVersion = testUnsupportedVersion
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
apiCompatibility: [v1]
runnerProtocolVersion: 1
capabilities: [platform.helm, api.v1]
`))
	if err != nil || contract.RunnerProtocolVersion != 1 || contract.Capabilities[0] != "api.v1" {
		t.Fatalf("DecodeCompatibility = %#v, %v", contract, err)
	}

	// The version ranges are derived from the release series, so a contract
	// that tries to declare one is a stale file, not a valid override.
	if _, err := DecodeCompatibility(strings.NewReader("apiCompatibility: [v1]\nrunnerProtocolVersion: 1\ncapabilities: [api.v1]\nplatformCompatibility: \">=1.0.0 <2.0.0\"\n")); err == nil {
		t.Fatal("a contract declaring a derived compatibility range must be rejected")
	}
	if _, err := DecodeCompatibility(strings.NewReader("unknown: true\n")); err == nil {
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
		Version:       testManifestVersion,
		Chart:         ChartArtifact{Reference: "oci://ghcr.io/example/charts/nopsai", Version: testManifestVersion, Digest: testDigest("b")},
		Images:        images,
		Compatibility: ManifestCompatibility{CLI: buildinfo.DefaultCLICompatibility, API: "v1", RunnerProtocol: 1},
		Database:      DatabaseContract{MigrationVersion: 1, RollbackSafe: false, RollbackPolicy: "forward-only"},
		Capabilities:  []string{CapabilityAPIV1, CapabilityPlatformHelm},
	}
}

func testCompatibilityLowerBound() string {
	rangeValue, err := ParseRange(buildinfo.DefaultPlatformCompatibility)
	if err != nil {
		panic(err)
	}
	for _, comparator := range rangeValue.Comparators {
		if comparator.Operator == ">=" || comparator.Operator == "=" {
			return comparator.Version.String()
		}
	}
	panic("default platform compatibility does not declare a lower bound")
}

func testCompatibilityUpperBound() string {
	rangeValue, err := ParseRange(buildinfo.DefaultPlatformCompatibility)
	if err != nil {
		panic(err)
	}
	for _, comparator := range rangeValue.Comparators {
		if comparator.Operator == "<" || comparator.Operator == "<=" {
			return comparator.Version.String()
		}
	}
	panic("default platform compatibility does not declare an upper bound")
}

func testCompatibilityVersionWithPatchOffset(raw string, offset int) string {
	version, err := ParseVersion(raw)
	if err != nil {
		panic(err)
	}
	version.Patch += offset
	return version.String()
}

func testDigest(character string) string {
	return "sha256:" + strings.Repeat(character, 64)
}

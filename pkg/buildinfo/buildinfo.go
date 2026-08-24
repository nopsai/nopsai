package buildinfo

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"nopsai"
)

const (
	DevelopmentVersion           = "dev"
	DefaultAPIVersion            = "v1"
	DefaultRunnerProtocolVersion = 1
	DefaultCapabilities          = "api.v1,cli.api-catalog.v1,config-sync.v1,dashboard-refresh.v1,dashboards.v1,mcp.v1,monitoring.v1,platform.docker-compose,platform.helm,runner.docker,runner.kubernetes,runner.local-registry-auth.v1,runner.registry-auth.v1"
)

// The compatibility defaults are derived from the release series in the
// repository root, never written out here. A release overrides them through
// the linker anyway; these are what a plain `go build` falls back to, and a
// hand-copied range would be one more thing to forget when the series moves.
//
// They are vars rather than consts only because a derived value cannot be a
// const. Nothing assigns to them.
var (
	DefaultCLICompatibility      = nopsai.CompatibilityRange()
	DefaultRunnerCompatibility   = nopsai.CompatibilityRange()
	DefaultPlatformCompatibility = nopsai.CompatibilityRange()
)

// These variables are release-linker inputs. Keep names stable for GoReleaser
// and Docker builds.
var (
	Version               = DevelopmentVersion
	Commit                = "unknown"
	BuildDate             = "unknown"
	ReleaseManifestDigest = ""
	APIVersion            = DefaultAPIVersion
	RunnerProtocolVersion = "1"
	CLICompatibility      = DefaultCLICompatibility
	RunnerCompatibility   = DefaultRunnerCompatibility
	PlatformCompatibility = DefaultPlatformCompatibility
	Capabilities          = DefaultCapabilities

	// LicensePublicKey is the base64 Ed25519 key that licence keys are verified
	// against, set at build time with -ldflags. It is a public key: shipping it
	// in every binary is the point, and it is what lets verification work with
	// no network access. A build without it verifies nothing, so every
	// installation of that build enforces nothing and runs under the
	// non-commercial licence.
	LicensePublicKey = ""
)

type Info struct {
	Version               string
	Commit                string
	BuildDate             string
	ReleaseManifestDigest string
	APIVersion            string
	RunnerProtocolVersion int
	CLICompatibility      string
	RunnerCompatibility   string
	PlatformCompatibility string
	Capabilities          []string
}

type PublicInfo struct {
	ProductVersion        string   `json:"productVersion" yaml:"productVersion"`
	Commit                string   `json:"commit" yaml:"commit"`
	BuildDate             string   `json:"buildDate" yaml:"buildDate"`
	APIVersion            string   `json:"apiVersion" yaml:"apiVersion"`
	CLICompatibility      string   `json:"cliCompatibility" yaml:"cliCompatibility"`
	RunnerCompatibility   string   `json:"runnerCompatibility" yaml:"runnerCompatibility"`
	RunnerProtocolVersion int      `json:"runnerProtocolVersion" yaml:"runnerProtocolVersion"`
	Capabilities          []string `json:"capabilities" yaml:"capabilities"`
	ReleaseManifestDigest string   `json:"releaseManifestDigest" yaml:"releaseManifestDigest"`
}

func Current() Info {
	runnerProtocol, err := strconv.Atoi(strings.TrimSpace(RunnerProtocolVersion))
	if err != nil || runnerProtocol < 1 {
		runnerProtocol = DefaultRunnerProtocolVersion
	}
	return Info{
		Version:               valueOrDefault(Version, DevelopmentVersion),
		Commit:                valueOrDefault(Commit, "unknown"),
		BuildDate:             valueOrDefault(BuildDate, "unknown"),
		ReleaseManifestDigest: strings.TrimSpace(ReleaseManifestDigest),
		APIVersion:            valueOrDefault(APIVersion, DefaultAPIVersion),
		RunnerProtocolVersion: runnerProtocol,
		CLICompatibility:      valueOrDefault(CLICompatibility, DefaultCLICompatibility),
		RunnerCompatibility:   valueOrDefault(RunnerCompatibility, DefaultRunnerCompatibility),
		PlatformCompatibility: valueOrDefault(PlatformCompatibility, DefaultPlatformCompatibility),
		Capabilities:          parseCapabilities(valueOrDefault(Capabilities, DefaultCapabilities)),
	}
}

func (i Info) Public() PublicInfo {
	return PublicInfo{
		ProductVersion:        i.Version,
		Commit:                i.Commit,
		BuildDate:             i.BuildDate,
		APIVersion:            i.APIVersion,
		CLICompatibility:      i.CLICompatibility,
		RunnerCompatibility:   i.RunnerCompatibility,
		RunnerProtocolVersion: i.RunnerProtocolVersion,
		Capabilities:          append([]string(nil), i.Capabilities...),
		ReleaseManifestDigest: i.ReleaseManifestDigest,
	}
}

func (i Info) IsDevelopment() bool {
	version := strings.ToLower(strings.TrimSpace(i.Version))
	return version == "" || version == DevelopmentVersion || version == "unknown"
}

func WriteVersion(writer io.Writer, binary string) error {
	info := Current()
	_, err := fmt.Fprintf(writer, "%s %s (commit %s, built %s, api %s, runner protocol %d)\n",
		strings.TrimSpace(binary), info.Version, info.Commit, info.BuildDate, info.APIVersion, info.RunnerProtocolVersion)
	return err
}

func Requested(args []string) bool {
	if len(args) != 1 {
		return false
	}
	return args[0] == "--version" || args[0] == "version"
}

func parseCapabilities(raw string) []string {
	seen := make(map[string]struct{})
	for _, capability := range strings.Split(raw, ",") {
		capability = strings.TrimSpace(capability)
		if capability != "" {
			seen[capability] = struct{}{}
		}
	}
	capabilities := make([]string, 0, len(seen))
	for capability := range seen {
		capabilities = append(capabilities, capability)
	}
	sort.Strings(capabilities)
	return capabilities
}

func valueOrDefault(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

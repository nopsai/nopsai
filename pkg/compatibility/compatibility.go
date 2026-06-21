package compatibility

import (
	"errors"
	"fmt"
	"strings"

	"nopsai/pkg/buildinfo"
)

type PlatformInfo struct {
	ProductVersion        string   `json:"productVersion" yaml:"productVersion"`
	APIVersion            string   `json:"apiVersion" yaml:"apiVersion"`
	CLICompatibility      string   `json:"cliCompatibility" yaml:"cliCompatibility"`
	RunnerCompatibility   string   `json:"runnerCompatibility" yaml:"runnerCompatibility"`
	RunnerProtocolVersion int      `json:"runnerProtocolVersion" yaml:"runnerProtocolVersion"`
	Capabilities          []string `json:"capabilities" yaml:"capabilities"`
	ReleaseManifestDigest string   `json:"releaseManifestDigest" yaml:"releaseManifestDigest"`
}

func ValidatePlatformForCLI(platform PlatformInfo, cli buildinfo.Info) error {
	if strings.TrimSpace(platform.APIVersion) != strings.TrimSpace(cli.APIVersion) {
		return fmt.Errorf("CLI API %s does not support platform API %s", cli.APIVersion, platform.APIVersion)
	}
	if cli.IsDevelopment() || developmentVersion(platform.ProductVersion) {
		return nil
	}
	cliVersion, err := ParseVersion(cli.Version)
	if err != nil {
		return fmt.Errorf("invalid CLI build version: %w", err)
	}
	platformVersion, err := ParseVersion(platform.ProductVersion)
	if err != nil {
		return fmt.Errorf("invalid platform version: %w", err)
	}
	platformRange, err := ParseRange(cli.PlatformCompatibility)
	if err != nil {
		return fmt.Errorf("invalid CLI platform compatibility: %w", err)
	}
	if !platformRange.Contains(platformVersion) {
		return fmt.Errorf("CLI %s does not support platform %s; supported range is %s", cliVersion.String(), platformVersion.String(), cli.PlatformCompatibility)
	}
	cliRange, err := ParseRange(platform.CLICompatibility)
	if err != nil {
		return fmt.Errorf("invalid platform CLI compatibility: %w", err)
	}
	if !cliRange.Contains(cliVersion) {
		return fmt.Errorf("platform %s does not support CLI %s; supported range is %s", platformVersion.String(), cliVersion.String(), platform.CLICompatibility)
	}
	return nil
}

func ValidateManifestForCLI(manifest Manifest, cli buildinfo.Info) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	if manifest.Compatibility.API != cli.APIVersion {
		return fmt.Errorf("release API %s is incompatible with CLI API %s", manifest.Compatibility.API, cli.APIVersion)
	}
	if manifest.Compatibility.RunnerProtocol != cli.RunnerProtocolVersion {
		return fmt.Errorf("release runner protocol %d is incompatible with CLI runner protocol %d", manifest.Compatibility.RunnerProtocol, cli.RunnerProtocolVersion)
	}
	if cli.IsDevelopment() {
		return nil
	}
	cliVersion, err := ParseVersion(cli.Version)
	if err != nil {
		return err
	}
	platformVersion, err := ParseVersion(manifest.Version)
	if err != nil {
		return err
	}
	platformRange, err := ParseRange(cli.PlatformCompatibility)
	if err != nil {
		return err
	}
	if !platformRange.Contains(platformVersion) {
		return fmt.Errorf("CLI %s does not support platform %s; supported range is %s", cliVersion.String(), platformVersion.String(), cli.PlatformCompatibility)
	}
	cliRange, err := ParseRange(manifest.Compatibility.CLI)
	if err != nil {
		return err
	}
	if !cliRange.Contains(cliVersion) {
		return fmt.Errorf("release %s does not support CLI %s; supported range is %s", platformVersion.String(), cliVersion.String(), manifest.Compatibility.CLI)
	}
	return nil
}

func RequireCapabilities(values []string, required ...string) error {
	for _, capability := range required {
		if !HasCapability(values, capability) {
			return fmt.Errorf("required capability %q is unavailable", capability)
		}
	}
	return nil
}

func developmentVersion(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "dev", "unknown":
		return true
	default:
		return false
	}
}

var ErrManifestDigestMismatch = errors.New("release manifest digest does not match expected digest")

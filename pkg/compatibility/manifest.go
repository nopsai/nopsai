package compatibility

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const ManifestSchemaVersion = "v1"

var RequiredPlatformImages = []string{"aaa", "agent", "api", "dispatcher", "dockerSocketProxy", "gitBot", "k8sRunner", "runner", "ui"}

var sha256DigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

type Manifest struct {
	SchemaVersion string                `json:"schemaVersion" yaml:"schemaVersion"`
	Version       string                `json:"version" yaml:"version"`
	Chart         ChartArtifact         `json:"chart" yaml:"chart"`
	Images        map[string]string     `json:"images" yaml:"images"`
	Compatibility ManifestCompatibility `json:"compatibility" yaml:"compatibility"`
	Database      DatabaseContract      `json:"database" yaml:"database"`
	Capabilities  []string              `json:"capabilities" yaml:"capabilities"`
}

type ChartArtifact struct {
	Reference string `json:"reference" yaml:"reference"`
	Version   string `json:"version" yaml:"version"`
	Digest    string `json:"digest" yaml:"digest"`
}

type ManifestCompatibility struct {
	CLI            string `json:"cli" yaml:"cli"`
	API            string `json:"api" yaml:"api"`
	RunnerProtocol int    `json:"runnerProtocol" yaml:"runnerProtocol"`
}

type DatabaseContract struct {
	MigrationVersion int    `json:"migrationVersion" yaml:"migrationVersion"`
	RollbackSafe     bool   `json:"rollbackSafe" yaml:"rollbackSafe"`
	RollbackPolicy   string `json:"rollbackPolicy" yaml:"rollbackPolicy"`
}

type CompatibilityFile struct {
	CLIVersion            string   `yaml:"cliVersion" json:"cliVersion"`
	PlatformCompatibility string   `yaml:"platformCompatibility" json:"platformCompatibility"`
	APICompatibility      []string `yaml:"apiCompatibility" json:"apiCompatibility"`
	RunnerCompatibility   string   `yaml:"runnerCompatibility" json:"runnerCompatibility"`
	RunnerProtocolVersion int      `yaml:"runnerProtocolVersion" json:"runnerProtocolVersion"`
	Capabilities          []string `yaml:"capabilities" json:"capabilities"`
}

func DecodeCompatibility(reader io.Reader) (CompatibilityFile, error) {
	var contract CompatibilityFile
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)
	if err := decoder.Decode(&contract); err != nil {
		return CompatibilityFile{}, fmt.Errorf("decode compatibility contract: %w", err)
	}
	if _, err := ParseVersion(contract.CLIVersion); err != nil {
		return CompatibilityFile{}, fmt.Errorf("invalid CLI version: %w", err)
	}
	if _, err := ParseRange(contract.PlatformCompatibility); err != nil {
		return CompatibilityFile{}, fmt.Errorf("invalid platform compatibility: %w", err)
	}
	if _, err := ParseRange(contract.RunnerCompatibility); err != nil {
		return CompatibilityFile{}, fmt.Errorf("invalid runner compatibility: %w", err)
	}
	if len(contract.APICompatibility) == 0 {
		return CompatibilityFile{}, errors.New("API compatibility must not be empty")
	}
	for _, apiVersion := range contract.APICompatibility {
		if strings.TrimSpace(apiVersion) == "" {
			return CompatibilityFile{}, errors.New("API compatibility contains an empty version")
		}
	}
	if contract.RunnerProtocolVersion < 1 {
		return CompatibilityFile{}, errors.New("runner protocol version must be at least 1")
	}
	capabilities, err := ValidateCapabilities(contract.Capabilities)
	if err != nil {
		return CompatibilityFile{}, err
	}
	contract.Capabilities = capabilities
	return contract, nil
}

func DecodeManifest(reader io.Reader) (Manifest, error) {
	var manifest Manifest
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode release manifest: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Manifest{}, errors.New("decode release manifest: trailing JSON content")
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (m *Manifest) Validate() error {
	if m.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("unsupported manifest schema %q", m.SchemaVersion)
	}
	version, err := ParseVersion(m.Version)
	if err != nil {
		return fmt.Errorf("invalid product version: %w", err)
	}
	m.Version = version.String()
	chartVersion, err := ParseVersion(m.Chart.Version)
	if err != nil {
		return fmt.Errorf("invalid chart version: %w", err)
	}
	if chartVersion.Compare(version) != 0 {
		return fmt.Errorf("chart version %s does not match product version %s", chartVersion.String(), version.String())
	}
	if !strings.HasPrefix(strings.TrimSpace(m.Chart.Reference), "oci://") {
		return errors.New("chart reference must use oci://")
	}
	if err := ValidateDigest(m.Chart.Digest); err != nil {
		return fmt.Errorf("invalid chart digest: %w", err)
	}
	for _, name := range RequiredPlatformImages {
		reference, ok := m.Images[name]
		if !ok {
			return fmt.Errorf("release manifest is missing image %q", name)
		}
		if err := ValidateImageReference(reference); err != nil {
			return fmt.Errorf("image %q: %w", name, err)
		}
	}
	for name, reference := range m.Images {
		if strings.TrimSpace(name) == "" {
			return errors.New("image name cannot be empty")
		}
		if err := ValidateImageReference(reference); err != nil {
			return fmt.Errorf("image %q: %w", name, err)
		}
	}
	if _, err := ParseRange(m.Compatibility.CLI); err != nil {
		return fmt.Errorf("invalid CLI compatibility: %w", err)
	}
	if strings.TrimSpace(m.Compatibility.API) == "" {
		return errors.New("API compatibility is required")
	}
	if m.Compatibility.RunnerProtocol < 1 {
		return errors.New("runner protocol must be at least 1")
	}
	if m.Database.MigrationVersion < 0 {
		return errors.New("database migration version cannot be negative")
	}
	policy := strings.TrimSpace(m.Database.RollbackPolicy)
	if policy != "rollback-safe" && policy != "forward-only" {
		return errors.New("database rollbackPolicy must be rollback-safe or forward-only")
	}
	if m.Database.RollbackSafe != (policy == "rollback-safe") {
		return errors.New("database rollbackSafe does not match rollbackPolicy")
	}
	capabilities, err := ValidateCapabilities(m.Capabilities)
	if err != nil {
		return err
	}
	if !HasCapability(capabilities, CapabilityPlatformHelm) {
		return fmt.Errorf("release manifest requires capability %q", CapabilityPlatformHelm)
	}
	m.Capabilities = capabilities
	return nil
}

func ValidateDigest(value string) error {
	if !sha256DigestPattern.MatchString(strings.TrimSpace(value)) {
		return errors.New("expected sha256 followed by 64 lowercase hexadecimal characters")
	}
	return nil
}

func ValidateImageReference(value string) error {
	value = strings.TrimSpace(value)
	separator := strings.LastIndex(value, "@")
	if separator <= 0 || separator == len(value)-1 || strings.ContainsAny(value, " \t\r\n") {
		return errors.New("must be an image name pinned with @sha256 digest")
	}
	if err := ValidateDigest(value[separator+1:]); err != nil {
		return fmt.Errorf("must be digest-pinned: %w", err)
	}
	return nil
}

func SplitImageReference(value string) (string, string, error) {
	if err := ValidateImageReference(value); err != nil {
		return "", "", err
	}
	separator := strings.LastIndex(value, "@")
	return value[:separator], value[separator+1:], nil
}

func DigestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func CanonicalJSON(manifest Manifest) ([]byte, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

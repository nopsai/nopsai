package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nopsai/pkg/buildinfo"
	"nopsai/pkg/compatibility"
)

const (
	EmbeddedManifestSource = "embedded:release-manifest.json"
	DefaultReleaseName     = "nopsai"
	DefaultNamespace       = "nopsai"
	DefaultLockFile        = ".nopsai/release.lock"
	maxManifestBytes       = 2 << 20
	maxValuesFileBytes     = 10 << 20
	maxReleaseLockBytes    = 1 << 20
)

type ProcessRunner = func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error

// EmbeddedReleaseManifestBase64 is a release-linker input for standalone CLI
// archives. It carries the digest-pinned manifest that matches the CLI version.
var EmbeddedReleaseManifestBase64 = ""

type ManifestResolver struct {
	HTTPClient            *http.Client
	URLTemplate           string
	AuthToken             string
	AuthTokenHostSuffixes []string
	EmbeddedManifest      []byte
}

type ResolvedManifest struct {
	Manifest compatibility.Manifest
	Source   string
	Digest   string
	Raw      []byte
}

type ManifestHTTPError struct {
	Source     string
	StatusCode int
}

func (e *ManifestHTTPError) Error() string {
	status := http.StatusText(e.StatusCode)
	if status == "" {
		return fmt.Sprintf("download release manifest from %s: HTTP %d", e.Source, e.StatusCode)
	}
	return fmt.Sprintf("download release manifest from %s: HTTP %d %s", e.Source, e.StatusCode, status)
}

type KubernetesOptions struct {
	Version                string
	ManifestSource         string
	ExpectedManifestDigest string
	ValuesFiles            []string
	ReleaseName            string
	Namespace              string
	Wait                   bool
	LockFile               string
}

type DeploymentPlan struct {
	Version              string                              `json:"version" yaml:"version"`
	CLI                  string                              `json:"cliVersion" yaml:"cliVersion"`
	ManifestSource       string                              `json:"manifestSource" yaml:"manifestSource"`
	ManifestDigest       string                              `json:"manifestDigest" yaml:"manifestDigest"`
	ReleaseName          string                              `json:"releaseName" yaml:"releaseName"`
	Namespace            string                              `json:"namespace" yaml:"namespace"`
	Chart                compatibility.ChartArtifact         `json:"chart" yaml:"chart"`
	Images               map[string]string                   `json:"images" yaml:"images"`
	Compatibility        compatibility.ManifestCompatibility `json:"compatibility" yaml:"compatibility"`
	Database             compatibility.DatabaseContract      `json:"database" yaml:"database"`
	Capabilities         []string                            `json:"capabilities" yaml:"capabilities"`
	ValuesFiles          []string                            `json:"valuesFiles" yaml:"valuesFiles"`
	ValuesHash           string                              `json:"valuesHash" yaml:"valuesHash"`
	RenderedManifestYAML string                              `json:"renderedManifestYaml" yaml:"renderedManifestYaml"`
}

type ReleaseLock struct {
	SchemaVersion    string            `json:"schemaVersion" yaml:"schemaVersion"`
	Version          string            `json:"version" yaml:"version"`
	ReleaseName      string            `json:"releaseName" yaml:"releaseName"`
	Namespace        string            `json:"namespace" yaml:"namespace"`
	ManifestSource   string            `json:"manifestSource" yaml:"manifestSource"`
	ManifestDigest   string            `json:"manifestDigest" yaml:"manifestDigest"`
	ChartDigest      string            `json:"chartDigest" yaml:"chartDigest"`
	Images           map[string]string `json:"images" yaml:"images"`
	ValuesHash       string            `json:"valuesHash" yaml:"valuesHash"`
	MigrationVersion int               `json:"migrationVersion" yaml:"migrationVersion"`
	RollbackPolicy   string            `json:"rollbackPolicy" yaml:"rollbackPolicy"`
	DeployedAt       time.Time         `json:"deployedAt" yaml:"deployedAt"`
}

type KubernetesDeployer struct {
	Resolver ManifestResolver
	Runner   ProcessRunner
	CLI      buildinfo.Info
	Stderr   io.Writer
	Now      func() time.Time
}

type preparedRelease struct {
	plan      DeploymentPlan
	manifest  compatibility.Manifest
	chartPath string
	common    []string
	cleanup   func()
}

func (r ManifestResolver) Resolve(ctx context.Context, version, source, expectedDigest string) (ResolvedManifest, error) {
	parsedVersion, err := compatibility.ParseVersion(version)
	if err != nil {
		return ResolvedManifest{}, fmt.Errorf("invalid requested release version: %w", err)
	}
	version = parsedVersion.String()
	source = strings.TrimSpace(source)
	var raw []byte
	if source == "" {
		template := strings.TrimSpace(r.URLTemplate)
		if template != "" {
			if strings.Count(template, "%s") != 1 {
				return ResolvedManifest{}, errors.New("release manifest URL template must contain exactly one %s placeholder")
			}
			source = fmt.Sprintf(template, version)
		} else {
			embedded, ok, err := r.embeddedManifest()
			if err != nil {
				return ResolvedManifest{}, err
			}
			if !ok {
				return ResolvedManifest{}, errors.New("release manifest is required when no embedded manifest or URL template is configured")
			}
			source = EmbeddedManifestSource
			raw = embedded
		}
	}
	if raw == nil {
		var err error
		raw, err = r.read(ctx, source)
		if err != nil {
			return ResolvedManifest{}, err
		}
	}
	digest := compatibility.DigestBytes(raw)
	if expected := strings.TrimSpace(expectedDigest); expected != "" {
		if err := compatibility.ValidateDigest(expected); err != nil {
			return ResolvedManifest{}, fmt.Errorf("invalid expected manifest digest: %w", err)
		}
		if digest != expected {
			return ResolvedManifest{}, fmt.Errorf("%w: got %s, expected %s", compatibility.ErrManifestDigestMismatch, digest, expected)
		}
	}
	manifest, err := compatibility.DecodeManifest(bytes.NewReader(raw))
	if err != nil {
		return ResolvedManifest{}, err
	}
	if manifest.Version != version {
		return ResolvedManifest{}, fmt.Errorf("manifest version %s does not match requested version %s", manifest.Version, version)
	}
	return ResolvedManifest{Manifest: manifest, Source: source, Digest: digest, Raw: raw}, nil
}

func (r ManifestResolver) embeddedManifest() ([]byte, bool, error) {
	if len(bytes.TrimSpace(r.EmbeddedManifest)) > 0 {
		if len(r.EmbeddedManifest) > maxManifestBytes {
			return nil, false, errors.New("embedded release manifest exceeds 2 MiB")
		}
		return append([]byte(nil), r.EmbeddedManifest...), true, nil
	}
	encoded := strings.TrimSpace(EmbeddedReleaseManifestBase64)
	if encoded == "" {
		return nil, false, nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, false, fmt.Errorf("decode embedded release manifest: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, false, nil
	}
	if len(raw) > maxManifestBytes {
		return nil, false, errors.New("embedded release manifest exceeds 2 MiB")
	}
	return raw, true, nil
}

func (r ManifestResolver) read(ctx context.Context, source string) ([]byte, error) {
	parsed, err := url.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse release manifest source: %w", err)
	}
	if parsed.Scheme == "" {
		contents, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("read release manifest: %w", err)
		}
		if len(contents) > maxManifestBytes {
			return nil, errors.New("release manifest exceeds 2 MiB")
		}
		return contents, nil
	}
	if parsed.Scheme != "https" {
		return nil, errors.New("remote release manifest source must use https")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build release manifest request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if token := r.authTokenForHost(parsed.Hostname()); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	client := r.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	client = httpsOnlyClient(client)
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download release manifest: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, &ManifestHTTPError{Source: parsed.String(), StatusCode: response.StatusCode}
	}
	contents, err := io.ReadAll(io.LimitReader(response.Body, maxManifestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read release manifest response: %w", err)
	}
	if len(contents) > maxManifestBytes {
		return nil, errors.New("release manifest exceeds 2 MiB")
	}
	return contents, nil
}

func (r ManifestResolver) authTokenForHost(host string) string {
	token := strings.TrimSpace(r.AuthToken)
	if token == "" {
		return ""
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	if len(r.AuthTokenHostSuffixes) == 0 {
		return token
	}
	for _, suffix := range r.AuthTokenHostSuffixes {
		suffix = strings.ToLower(strings.TrimSpace(suffix))
		if suffix == "" {
			continue
		}
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return token
		}
	}
	return ""
}

func httpsOnlyClient(client *http.Client) *http.Client {
	clone := *client
	upstreamRedirect := clone.CheckRedirect
	clone.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if request.URL.Scheme != "https" {
			return errors.New("release manifest redirect must use https")
		}
		if upstreamRedirect != nil {
			return upstreamRedirect(request, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &clone
}

func (d KubernetesDeployer) Plan(ctx context.Context, options KubernetesOptions) (DeploymentPlan, error) {
	prepared, err := d.prepare(ctx, options)
	if err != nil {
		return DeploymentPlan{}, err
	}
	defer prepared.cleanup()
	if err := d.render(ctx, prepared); err != nil {
		return DeploymentPlan{}, err
	}
	return prepared.plan, nil
}

func (d KubernetesDeployer) Deploy(ctx context.Context, options KubernetesOptions) (DeploymentPlan, ReleaseLock, error) {
	plan, lock, _, err := d.PlanAndDeploy(ctx, options, nil)
	return plan, lock, err
}

func (d KubernetesDeployer) PlanAndDeploy(ctx context.Context, options KubernetesOptions, approve func(DeploymentPlan) (bool, error)) (DeploymentPlan, ReleaseLock, bool, error) {
	prepared, err := d.prepare(ctx, options)
	if err != nil {
		return DeploymentPlan{}, ReleaseLock{}, false, err
	}
	defer prepared.cleanup()
	if err := d.render(ctx, prepared); err != nil {
		return DeploymentPlan{}, ReleaseLock{}, false, err
	}
	if approve != nil {
		approved, err := approve(prepared.plan)
		if err != nil {
			return DeploymentPlan{}, ReleaseLock{}, false, err
		}
		if !approved {
			return prepared.plan, ReleaseLock{}, false, nil
		}
	}
	lock, err := d.deployPrepared(ctx, prepared, options)
	if err != nil {
		return DeploymentPlan{}, ReleaseLock{}, true, err
	}
	return prepared.plan, lock, true, nil
}

func (d KubernetesDeployer) deployPrepared(ctx context.Context, prepared *preparedRelease, options KubernetesOptions) (ReleaseLock, error) {
	lockPath := strings.TrimSpace(options.LockFile)
	if lockPath == "" {
		lockPath = DefaultLockFile
	}
	if err := validateDeploymentTransition(lockPath, prepared.plan); err != nil {
		return ReleaseLock{}, err
	}
	args := []string{"upgrade", "--install", prepared.plan.ReleaseName, prepared.chartPath}
	args = append(args, prepared.common...)
	args = append(args, "--create-namespace")
	if options.Wait {
		args = append(args, "--wait")
	}
	if err := d.run(ctx, "helm", args, io.Discard); err != nil {
		return ReleaseLock{}, fmt.Errorf("deploy Helm release: %w", err)
	}
	now := time.Now().UTC()
	if d.Now != nil {
		now = d.Now().UTC()
	}
	lock := ReleaseLock{
		SchemaVersion:    "v1",
		Version:          prepared.plan.Version,
		ReleaseName:      prepared.plan.ReleaseName,
		Namespace:        prepared.plan.Namespace,
		ManifestSource:   prepared.plan.ManifestSource,
		ManifestDigest:   prepared.plan.ManifestDigest,
		ChartDigest:      prepared.plan.Chart.Digest,
		Images:           cloneStrings(prepared.plan.Images),
		ValuesHash:       prepared.plan.ValuesHash,
		MigrationVersion: prepared.plan.Database.MigrationVersion,
		RollbackPolicy:   prepared.plan.Database.RollbackPolicy,
		DeployedAt:       now,
	}
	if err := WriteReleaseLock(lockPath, lock); err != nil {
		return ReleaseLock{}, err
	}
	return lock, nil
}

func ReadReleaseLock(path string) (ReleaseLock, bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return ReleaseLock{}, false, nil
	}
	if err != nil {
		return ReleaseLock{}, false, fmt.Errorf("open release lock: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return ReleaseLock{}, false, fmt.Errorf("inspect release lock: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxReleaseLockBytes {
		return ReleaseLock{}, false, errors.New("release lock must be a regular file no larger than 1 MiB")
	}
	var lock ReleaseLock
	decoder := json.NewDecoder(io.LimitReader(file, maxReleaseLockBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&lock); err != nil {
		return ReleaseLock{}, false, fmt.Errorf("decode release lock: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return ReleaseLock{}, false, errors.New("decode release lock: trailing JSON content")
	}
	if lock.SchemaVersion != "v1" {
		return ReleaseLock{}, false, fmt.Errorf("unsupported release lock schema %q", lock.SchemaVersion)
	}
	if _, err := compatibility.ParseVersion(lock.Version); err != nil {
		return ReleaseLock{}, false, fmt.Errorf("invalid release lock version: %w", err)
	}
	if lock.MigrationVersion < 0 {
		return ReleaseLock{}, false, errors.New("release lock migration version cannot be negative")
	}
	lock.RollbackPolicy = strings.TrimSpace(lock.RollbackPolicy)
	if lock.RollbackPolicy == "" {
		// Locks created before this field existed remain conservative.
		lock.RollbackPolicy = "forward-only"
	}
	if lock.RollbackPolicy != "forward-only" && lock.RollbackPolicy != "rollback-safe" {
		return ReleaseLock{}, false, fmt.Errorf("invalid release lock rollback policy %q", lock.RollbackPolicy)
	}
	return lock, true, nil
}

func validateDeploymentTransition(lockPath string, next DeploymentPlan) error {
	current, found, err := ReadReleaseLock(lockPath)
	if err != nil {
		return fmt.Errorf("validate deployment transition: %w", err)
	}
	if !found {
		return nil
	}
	if current.ReleaseName != next.ReleaseName || current.Namespace != next.Namespace {
		return fmt.Errorf("release lock belongs to %s in namespace %s, not %s in namespace %s", current.ReleaseName, current.Namespace, next.ReleaseName, next.Namespace)
	}
	currentVersion, _ := compatibility.ParseVersion(current.Version)
	nextVersion, err := compatibility.ParseVersion(next.Version)
	if err != nil {
		return fmt.Errorf("invalid deployment version: %w", err)
	}
	if next.Database.MigrationVersion < current.MigrationVersion {
		return fmt.Errorf("database migration regression from %d to %d is not allowed", current.MigrationVersion, next.Database.MigrationVersion)
	}
	if nextVersion.Compare(currentVersion) < 0 && (current.RollbackPolicy != "rollback-safe" || next.Database.RollbackPolicy != "rollback-safe") {
		return fmt.Errorf("downgrade from %s to %s is blocked by forward-only rollback policy", current.Version, next.Version)
	}
	return nil
}

func (d KubernetesDeployer) prepare(ctx context.Context, options KubernetesOptions) (*preparedRelease, error) {
	cli := d.CLI
	if strings.TrimSpace(cli.Version) == "" {
		cli = buildinfo.Current()
	}
	resolved, err := d.Resolver.Resolve(ctx, options.Version, options.ManifestSource, defaultReleaseManifestDigest(options.ManifestSource, options.ExpectedManifestDigest, cli))
	if err != nil {
		return nil, err
	}
	if err := compatibility.ValidateManifestForCLI(resolved.Manifest, cli); err != nil {
		return nil, fmt.Errorf("release compatibility check failed: %w", err)
	}
	if err := compatibility.RequireCapabilities(resolved.Manifest.Capabilities, compatibility.CapabilityPlatformHelm); err != nil {
		return nil, err
	}
	releaseName := strings.TrimSpace(options.ReleaseName)
	if releaseName == "" {
		releaseName = DefaultReleaseName
	}
	namespace := strings.TrimSpace(options.Namespace)
	if namespace == "" {
		namespace = DefaultNamespace
	}
	if err := validateHelmName("release", releaseName); err != nil {
		return nil, err
	}
	if err := validateHelmName("namespace", namespace); err != nil {
		return nil, err
	}
	valuesHash, err := hashDeploymentValues(options.ValuesFiles, resolved)
	if err != nil {
		return nil, err
	}
	tempDir, err := os.MkdirTemp("", "nopsai-release-*")
	if err != nil {
		return nil, fmt.Errorf("create release workspace: %w", err)
	}
	prepared := &preparedRelease{cleanup: func() { _ = os.RemoveAll(tempDir) }}
	pullArgs := []string{"pull", resolved.Manifest.Chart.Reference, "--version", resolved.Manifest.Chart.Version, "--destination", tempDir}
	if err := d.run(ctx, "helm", pullArgs, io.Discard); err != nil {
		prepared.cleanup()
		return nil, fmt.Errorf("pull Helm chart: %w", err)
	}
	chartPath, err := findChartArchive(tempDir)
	if err != nil {
		prepared.cleanup()
		return nil, err
	}
	chartDigest, err := digestFile(chartPath)
	if err != nil {
		prepared.cleanup()
		return nil, err
	}
	if chartDigest != resolved.Manifest.Chart.Digest {
		prepared.cleanup()
		return nil, fmt.Errorf("Helm chart digest mismatch: got %s, expected %s", chartDigest, resolved.Manifest.Chart.Digest)
	}
	common := []string{"--namespace", namespace}
	for _, valuesFile := range options.ValuesFiles {
		common = append(common, "--values", valuesFile)
	}
	for _, assignment := range releaseValueAssignments(resolved) {
		common = append(common, "--set-string", assignment)
	}
	prepared.chartPath = chartPath
	prepared.common = common
	prepared.manifest = resolved.Manifest
	prepared.plan = DeploymentPlan{
		Version:              resolved.Manifest.Version,
		CLI:                  cli.Version,
		ManifestSource:       resolved.Source,
		ManifestDigest:       resolved.Digest,
		ReleaseName:          releaseName,
		Namespace:            namespace,
		Chart:                resolved.Manifest.Chart,
		Images:               cloneStrings(resolved.Manifest.Images),
		Compatibility:        resolved.Manifest.Compatibility,
		Database:             resolved.Manifest.Database,
		Capabilities:         append([]string(nil), resolved.Manifest.Capabilities...),
		ValuesFiles:          append([]string(nil), options.ValuesFiles...),
		ValuesHash:           valuesHash,
		RenderedManifestYAML: "",
	}
	return prepared, nil
}

func (d KubernetesDeployer) render(ctx context.Context, prepared *preparedRelease) error {
	var rendered bytes.Buffer
	args := []string{"template", prepared.plan.ReleaseName, prepared.chartPath}
	args = append(args, prepared.common...)
	if err := d.run(ctx, "helm", args, &rendered); err != nil {
		return fmt.Errorf("render Helm release: %w", err)
	}
	prepared.plan.RenderedManifestYAML = rendered.String()
	return nil
}

func (d KubernetesDeployer) run(ctx context.Context, name string, args []string, stdout io.Writer) error {
	if d.Runner == nil {
		return errors.New("process runner is not configured")
	}
	stderr := d.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	return d.Runner(ctx, name, args, stdout, stderr)
}

func WriteReleaseLock(path string, lock ReleaseLock) error {
	contents, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("encode release lock: %w", err)
	}
	contents = append(contents, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create release lock directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".release-lock-*.tmp")
	if err != nil {
		return fmt.Errorf("create release lock: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o644); err != nil { // #nosec G302 -- release locks contain no secrets and are GitOps-readable.
		temp.Close()
		return fmt.Errorf("set release lock permissions: %w", err)
	}
	if _, err := temp.Write(contents); err != nil {
		temp.Close()
		return fmt.Errorf("write release lock: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync release lock: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close release lock: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace release lock: %w", err)
	}
	return nil
}

func releaseValueAssignments(resolved ResolvedManifest) []string {
	assignments := []string{
		"global.releaseVersion=" + resolved.Manifest.Version,
		"global.releaseManifestDigest=" + resolved.Digest,
	}
	names := make([]string, 0, len(resolved.Manifest.Images))
	for name := range resolved.Manifest.Images {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		repository, digest, _ := compatibility.SplitImageReference(resolved.Manifest.Images[name])
		assignments = append(assignments, name+".image.repository="+repository, name+".image.digest="+digest)
	}
	return assignments
}

func hashDeploymentValues(files []string, resolved ResolvedManifest) (string, error) {
	hash := sha256.New()
	for index, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("inspect values file %s: %w", path, err)
		}
		if !info.Mode().IsRegular() || info.Size() > maxValuesFileBytes {
			return "", fmt.Errorf("values file %s must be a regular file no larger than 10 MiB", path)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read values file %s: %w", path, err)
		}
		_, _ = hash.Write(fmt.Appendf(nil, "values[%d]", index))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(contents)
		_, _ = hash.Write([]byte{0})
	}
	for _, assignment := range releaseValueAssignments(resolved) {
		_, _ = hash.Write([]byte(assignment))
		_, _ = hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func findChartArchive(dir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.tgz"))
	if err != nil {
		return "", fmt.Errorf("find Helm chart archive: %w", err)
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("expected one Helm chart archive, found %d", len(matches))
	}
	return matches[0], nil
}

func digestFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open Helm chart archive: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash Helm chart archive: %w", err)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func validateHelmName(label, value string) error {
	if value == "" || len(value) > 63 {
		return fmt.Errorf("%s name must contain 1-63 characters", label)
	}
	for index, character := range value {
		valid := character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-'
		if !valid || (character == '-' && (index == 0 || index == len(value)-1)) {
			return fmt.Errorf("%s name must use lowercase letters, numbers, and interior hyphens", label)
		}
	}
	return nil
}

func cloneStrings(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

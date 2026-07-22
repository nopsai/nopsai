package platform

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"nopsai/pkg/buildinfo"
	"nopsai/pkg/compatibility"
)

func TestManifestResolverLoadsAndVerifiesLocalManifest(t *testing.T) {
	manifestPath, raw, _ := writeReleaseFixture(t, []byte("chart"))
	resolver := ManifestResolver{}
	resolved, err := resolver.Resolve(context.Background(), "v2.7.0", manifestPath, compatibility.DigestBytes(raw))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Manifest.Version != "2.7.0" || resolved.Source != manifestPath || resolved.Digest != compatibility.DigestBytes(raw) {
		t.Fatalf("resolved = %#v", resolved)
	}
	if _, err := resolver.Resolve(context.Background(), "2.7.0", manifestPath, testSHA("f")); !errors.Is(err, compatibility.ErrManifestDigestMismatch) {
		t.Fatalf("digest mismatch error = %v", err)
	}
	if _, err := resolver.Resolve(context.Background(), "2.6.0", manifestPath, ""); err == nil {
		t.Fatal("manifest version mismatch succeeded")
	}
	if _, err := resolver.Resolve(context.Background(), "2.7.0", "http://example.com/manifest.json", ""); err == nil {
		t.Fatal("insecure remote manifest succeeded")
	}
}

func TestManifestResolverLoadsEmbeddedManifestByDefault(t *testing.T) {
	_, raw, _ := writeReleaseFixture(t, []byte("chart"))
	resolver := ManifestResolver{EmbeddedManifest: raw}
	resolved, err := resolver.Resolve(context.Background(), "2.7.0", "", compatibility.DigestBytes(raw))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Source != EmbeddedManifestSource || !bytes.Equal(resolved.Raw, raw) || resolved.Digest != compatibility.DigestBytes(raw) {
		t.Fatalf("resolved embedded manifest = %#v", resolved)
	}
}

func TestManifestResolverLoadsReleaseLinkedEmbeddedManifest(t *testing.T) {
	_, raw, _ := writeReleaseFixture(t, []byte("chart"))
	previous := EmbeddedReleaseManifestBase64
	EmbeddedReleaseManifestBase64 = base64.StdEncoding.EncodeToString(raw)
	defer func() { EmbeddedReleaseManifestBase64 = previous }()

	resolved, err := (ManifestResolver{}).Resolve(context.Background(), "2.7.0", "", compatibility.DigestBytes(raw))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Source != EmbeddedManifestSource || !bytes.Equal(resolved.Raw, raw) {
		t.Fatalf("resolved linked manifest = %#v", resolved)
	}
}

func TestManifestResolverRequiresExplicitManifestWhenNoDefaultExists(t *testing.T) {
	_, err := (ManifestResolver{}).Resolve(context.Background(), "2.7.0", "", "")
	if err == nil || !strings.Contains(err.Error(), "release manifest is required") {
		t.Fatalf("missing manifest error = %v", err)
	}
}

func TestManifestResolverUsesConfiguredURLTemplate(t *testing.T) {
	_, raw, _ := writeReleaseFixture(t, []byte("chart"))
	secure := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v2.7.0/release-manifest.json" {
			t.Fatalf("request path = %s", request.URL.Path)
		}
		_, _ = writer.Write(raw)
	}))
	defer secure.Close()

	resolver := ManifestResolver{HTTPClient: secure.Client(), URLTemplate: secure.URL + "/v%s/release-manifest.json"}
	resolved, err := resolver.Resolve(context.Background(), "2.7.0", "", compatibility.DigestBytes(raw))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Source != secure.URL+"/v2.7.0/release-manifest.json" {
		t.Fatalf("source = %q", resolved.Source)
	}
}

func TestManifestResolverSendsScopedManifestToken(t *testing.T) {
	_, raw, _ := writeReleaseFixture(t, []byte("chart"))
	var sawToken bool
	secure := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") == "Bearer manifest-token" {
			sawToken = true
		}
		_, _ = writer.Write(raw)
	}))
	defer secure.Close()
	parsedURL, err := url.Parse(secure.URL)
	if err != nil {
		t.Fatal(err)
	}

	resolver := ManifestResolver{
		HTTPClient:            secure.Client(),
		AuthToken:             "manifest-token",
		AuthTokenHostSuffixes: []string{parsedURL.Hostname()},
	}
	if _, err := resolver.Resolve(context.Background(), "2.7.0", secure.URL, compatibility.DigestBytes(raw)); err != nil {
		t.Fatal(err)
	}
	if !sawToken {
		t.Fatal("remote manifest request did not include scoped token")
	}
}

func TestManifestResolverReportsRemoteHTTPStatus(t *testing.T) {
	secure := httptest.NewTLSServer(http.NotFoundHandler())
	defer secure.Close()

	_, err := (ManifestResolver{HTTPClient: secure.Client()}).Resolve(context.Background(), "2.7.0", secure.URL, "")
	var httpErr *ManifestHTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound || httpErr.Source != secure.URL {
		t.Fatalf("HTTP status error = %#v, %v", httpErr, err)
	}
}

func TestManifestResolverRejectsHTTPSRedirectToHTTP(t *testing.T) {
	insecure := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer insecure.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, insecure.URL, http.StatusFound)
	}))
	defer secure.Close()

	resolver := ManifestResolver{HTTPClient: secure.Client()}
	_, err := resolver.Resolve(context.Background(), "2.7.0", secure.URL, "")
	if err == nil || !strings.Contains(err.Error(), "redirect must use https") {
		t.Fatalf("redirect downgrade error = %v", err)
	}
}

func TestKubernetesPlanAndDeployUseVerifiedBundleAndWriteLock(t *testing.T) {
	chart := []byte("signed chart archive")
	manifestPath, _, manifest := writeReleaseFixture(t, chart)
	valuesPath := filepath.Join(t.TempDir(), "production.yaml")
	if err := os.WriteFile(valuesPath, []byte("ingress:\n  enabled: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeHelmRunner{t: t, chart: chart}
	deployer := KubernetesDeployer{
		Resolver: ManifestResolver{},
		Runner:   runner.Run,
		CLI:      releaseCLIInfo("2.7.0"),
		Now:      func() time.Time { return time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC) },
	}
	options := KubernetesOptions{
		Version:        "2.7.0",
		ManifestSource: manifestPath,
		ValuesFiles:    []string{valuesPath},
		ReleaseName:    "nopsai-prod",
		Namespace:      "nopsai-system",
		Wait:           true,
		LockFile:       filepath.Join(t.TempDir(), ".nopsai", "release.lock"),
	}
	plan, err := deployer.Plan(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Version != "2.7.0" || plan.Chart.Digest != manifest.Chart.Digest || !strings.Contains(plan.RenderedManifestYAML, "kind: Deployment") || !strings.HasPrefix(plan.ValuesHash, "sha256:") {
		t.Fatalf("plan = %#v", plan)
	}
	plan, lock, err := deployer.Deploy(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if lock.Version != plan.Version || lock.ManifestDigest != plan.ManifestDigest || lock.MigrationVersion != 1 || lock.RollbackPolicy != "forward-only" || !lock.DeployedAt.Equal(time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("lock = %#v", lock)
	}
	lockContents, err := os.ReadFile(options.LockFile)
	if err != nil {
		t.Fatal(err)
	}
	var persisted ReleaseLock
	if err := json.Unmarshal(lockContents, &persisted); err != nil || persisted.ChartDigest != manifest.Chart.Digest {
		t.Fatalf("persisted lock = %#v, %v", persisted, err)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(options.LockFile)
		if info.Mode().Perm() != 0o644 {
			t.Fatalf("lock permissions = %o", info.Mode().Perm())
		}
	}
	if !runner.sawUpgradeWait || !runner.sawDigestAssignment {
		t.Fatalf("helm calls = %#v", runner.calls)
	}
}

func TestKubernetesPlanAndDeployCanBeDeclinedAfterRendering(t *testing.T) {
	chart := []byte("signed chart archive")
	manifestPath, _, _ := writeReleaseFixture(t, chart)
	runner := &fakeHelmRunner{t: t, chart: chart}
	deployer := KubernetesDeployer{
		Resolver: ManifestResolver{},
		Runner:   runner.Run,
		CLI:      releaseCLIInfo("2.7.0"),
	}
	plan, lock, deployed, err := deployer.PlanAndDeploy(context.Background(), KubernetesOptions{
		Version:        "2.7.0",
		ManifestSource: manifestPath,
	}, func(plan DeploymentPlan) (bool, error) {
		if !strings.Contains(plan.RenderedManifestYAML, "kind: Deployment") {
			t.Fatalf("approval did not receive rendered plan: %#v", plan)
		}
		return false, nil
	})
	if err != nil || deployed || lock.Version != "" || plan.Version != "2.7.0" {
		t.Fatalf("declined deployment = plan %#v lock %#v deployed %v err %v", plan, lock, deployed, err)
	}
	if runner.sawUpgrade {
		t.Fatalf("declined deployment reached helm upgrade: %#v", runner.calls)
	}
}

func TestKubernetesDeployBlocksForwardOnlyDowngradeBeforeUpgrade(t *testing.T) {
	chart := []byte("chart")
	manifestPath, _, _ := writeReleaseFixture(t, chart)
	lockPath := filepath.Join(t.TempDir(), "release.lock")
	if err := WriteReleaseLock(lockPath, ReleaseLock{
		SchemaVersion:    "v1",
		Version:          "2.8.0",
		ReleaseName:      DefaultReleaseName,
		Namespace:        DefaultNamespace,
		MigrationVersion: 1,
		RollbackPolicy:   "forward-only",
	}); err != nil {
		t.Fatal(err)
	}
	runner := &fakeHelmRunner{t: t, chart: chart}
	deployer := KubernetesDeployer{Resolver: ManifestResolver{}, Runner: runner.Run, CLI: releaseCLIInfo("2.7.0")}
	_, _, err := deployer.Deploy(context.Background(), KubernetesOptions{Version: "2.7.0", ManifestSource: manifestPath, LockFile: lockPath})
	if err == nil || !strings.Contains(err.Error(), "forward-only") {
		t.Fatalf("downgrade error = %v", err)
	}
	if runner.sawUpgrade {
		t.Fatalf("unsafe transition reached helm upgrade: %#v", runner.calls)
	}
}

func TestDeploymentTransitionValidatesLockIdentityPolicyAndMigration(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "release.lock")
	plan := DeploymentPlan{
		Version:     "2.7.0",
		ReleaseName: DefaultReleaseName,
		Namespace:   DefaultNamespace,
		Database:    compatibility.DatabaseContract{MigrationVersion: 2, RollbackSafe: true, RollbackPolicy: "rollback-safe"},
	}
	if err := validateDeploymentTransition(lockPath, plan); err != nil {
		t.Fatalf("missing lock = %v", err)
	}
	legacy := ReleaseLock{SchemaVersion: "v1", Version: "2.8.0", ReleaseName: DefaultReleaseName, Namespace: DefaultNamespace, MigrationVersion: 2}
	if err := WriteReleaseLock(lockPath, legacy); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := ReadReleaseLock(lockPath)
	if err != nil || !found || loaded.RollbackPolicy != "forward-only" {
		t.Fatalf("legacy lock = %#v, %v, %v", loaded, found, err)
	}
	if err := validateDeploymentTransition(lockPath, plan); err == nil || !strings.Contains(err.Error(), "forward-only") {
		t.Fatalf("legacy downgrade error = %v", err)
	}

	legacy.RollbackPolicy = "rollback-safe"
	if err := WriteReleaseLock(lockPath, legacy); err != nil {
		t.Fatal(err)
	}
	if err := validateDeploymentTransition(lockPath, plan); err != nil {
		t.Fatalf("rollback-safe downgrade = %v", err)
	}
	regression := plan
	regression.Version = "2.9.0"
	regression.Database.MigrationVersion = 1
	if err := validateDeploymentTransition(lockPath, regression); err == nil || !strings.Contains(err.Error(), "migration regression") {
		t.Fatalf("migration regression error = %v", err)
	}
	wrongRelease := plan
	wrongRelease.ReleaseName = "other"
	if err := validateDeploymentTransition(lockPath, wrongRelease); err == nil || !strings.Contains(err.Error(), "release lock belongs") {
		t.Fatalf("lock identity error = %v", err)
	}
	invalidVersion := plan
	invalidVersion.Version = "invalid"
	if err := validateDeploymentTransition(lockPath, invalidVersion); err == nil || !strings.Contains(err.Error(), "invalid deployment version") {
		t.Fatalf("target version error = %v", err)
	}
}

func TestReadReleaseLockRejectsMalformedContracts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "release.lock")
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"invalid JSON", `{`, "decode release lock"},
		{"unknown field", `{"schemaVersion":"v1","version":"2.7.0","unknown":true}`, "unknown field"},
		{"trailing content", `{"schemaVersion":"v1","version":"2.7.0"} {}`, "trailing JSON"},
		{"unsupported schema", `{"schemaVersion":"v2","version":"2.7.0"}`, "unsupported release lock schema"},
		{"invalid version", `{"schemaVersion":"v1","version":"bad"}`, "invalid release lock version"},
		{"negative migration", `{"schemaVersion":"v1","version":"2.7.0","migrationVersion":-1}`, "cannot be negative"},
		{"invalid policy", `{"schemaVersion":"v1","version":"2.7.0","rollbackPolicy":"sometimes"}`, "invalid release lock rollback policy"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := ReadReleaseLock(path); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ReadReleaseLock error = %v, want %q", err, test.want)
			}
		})
	}
	directoryPath := filepath.Join(dir, "lock-directory")
	if err := os.Mkdir(directoryPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadReleaseLock(directoryPath); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("directory lock error = %v", err)
	}
}

func TestKubernetesPlanRejectsIncompatibleCLIAndChartDigest(t *testing.T) {
	chart := []byte("chart")
	manifestPath, _, _ := writeReleaseFixture(t, chart)
	runner := &fakeHelmRunner{t: t, chart: chart}
	deployer := KubernetesDeployer{Resolver: ManifestResolver{}, Runner: runner.Run, CLI: releaseCLIInfo("3.0.0")}
	if _, err := deployer.Plan(context.Background(), KubernetesOptions{Version: "2.7.0", ManifestSource: manifestPath}); err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("compatibility error = %v", err)
	}

	deployer.CLI = releaseCLIInfo("2.7.0")
	runner.chart = []byte("tampered")
	if _, err := deployer.Plan(context.Background(), KubernetesOptions{Version: "2.7.0", ManifestSource: manifestPath}); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("chart digest error = %v", err)
	}
}

func TestDeploymentInputValidation(t *testing.T) {
	chart := []byte("chart")
	manifestPath, _, _ := writeReleaseFixture(t, chart)
	deployer := KubernetesDeployer{Resolver: ManifestResolver{}, Runner: (&fakeHelmRunner{t: t, chart: chart}).Run, CLI: releaseCLIInfo("2.7.0")}
	for _, options := range []KubernetesOptions{
		{Version: "2.7.0", ManifestSource: manifestPath, ReleaseName: "Bad_Name"},
		{Version: "2.7.0", ManifestSource: manifestPath, Namespace: "-bad"},
		{Version: "2.7.0", ManifestSource: manifestPath, ValuesFiles: []string{filepath.Join(t.TempDir(), "missing.yaml")}},
	} {
		if _, err := deployer.Plan(context.Background(), options); err == nil {
			t.Fatalf("invalid options succeeded: %#v", options)
		}
	}
}

type fakeHelmRunner struct {
	t                   *testing.T
	chart               []byte
	calls               [][]string
	sawUpgrade          bool
	sawUpgradeWait      bool
	sawDigestAssignment bool
}

func (r *fakeHelmRunner) Run(_ context.Context, name string, args []string, stdout, _ io.Writer) error {
	r.t.Helper()
	if name != "helm" {
		return errors.New("unexpected process")
	}
	r.calls = append(r.calls, append([]string(nil), args...))
	switch args[0] {
	case "pull":
		destination := argumentValue(args, "--destination")
		if destination == "" {
			return errors.New("missing destination")
		}
		return os.WriteFile(filepath.Join(destination, "nopsai-2.7.0.tgz"), r.chart, 0o600)
	case "template":
		_, err := io.WriteString(stdout, "apiVersion: apps/v1\nkind: Deployment\n")
		return err
	case "upgrade":
		r.sawUpgrade = true
		r.sawUpgradeWait = containsArgument(args, "--wait")
		for _, value := range args {
			if strings.Contains(value, ".image.digest=sha256:") {
				r.sawDigestAssignment = true
			}
		}
		return nil
	default:
		return errors.New("unexpected helm command")
	}
}

func writeReleaseFixture(t *testing.T, chart []byte) (string, []byte, compatibility.Manifest) {
	t.Helper()
	images := make(map[string]string, len(compatibility.RequiredPlatformImages))
	for _, name := range compatibility.RequiredPlatformImages {
		images[name] = "ghcr.io/example/nopsai-" + strings.ToLower(name) + "@" + testSHA("a")
	}
	manifest := compatibility.Manifest{
		SchemaVersion: "v1",
		Version:       "2.7.0",
		Chart: compatibility.ChartArtifact{
			Reference: "oci://ghcr.io/example/charts/nopsai",
			Version:   "2.7.0",
			Digest:    compatibility.DigestBytes(chart),
		},
		Images:        images,
		Compatibility: compatibility.ManifestCompatibility{CLI: ">=2.0.0 <3.0.0", API: "v1", RunnerProtocol: 1},
		Database:      compatibility.DatabaseContract{MigrationVersion: 1, RollbackSafe: false, RollbackPolicy: "forward-only"},
		Capabilities:  []string{compatibility.CapabilityAPIV1, compatibility.CapabilityPlatformHelm},
	}
	raw, err := compatibility.CanonicalJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "release-manifest.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, raw, manifest
}

func releaseCLIInfo(version string) buildinfo.Info {
	return buildinfo.Info{
		Version:               version,
		APIVersion:            "v1",
		RunnerProtocolVersion: 1,
		PlatformCompatibility: ">=2.0.0 <3.0.0",
	}
}

func testSHA(character string) string {
	return "sha256:" + strings.Repeat(character, 64)
}

func argumentValue(args []string, name string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name {
			return args[index+1]
		}
	}
	return ""
}

func containsArgument(args []string, wanted string) bool {
	for _, value := range args {
		if value == wanted {
			return true
		}
	}
	return false
}

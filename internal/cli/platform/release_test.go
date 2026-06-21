package platform

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
	if lock.Version != plan.Version || lock.ManifestDigest != plan.ManifestDigest || lock.MigrationVersion != 1 || !lock.DeployedAt.Equal(time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)) {
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

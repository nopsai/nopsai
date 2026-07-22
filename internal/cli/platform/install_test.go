package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"nopsai/pkg/buildinfo"
	"nopsai/pkg/compatibility"
)

func TestDockerComposeInstallPlanWritesVersionedArtifacts(t *testing.T) {
	manifestPath, manifest := writeInstallManifestFixture(t, []string{
		compatibility.CapabilityAPIV1,
		compatibility.CapabilityPlatformCompose,
		compatibility.CapabilityPlatformHelm,
		compatibility.CapabilityRunnerDocker,
		compatibility.CapabilityRunnerK8s,
	})
	outputDir := filepath.Join(t.TempDir(), "install")
	installer := Installer{
		Resolver:     ManifestResolver{},
		CLI:          installCLIInfo("2.7.0"),
		RandomReader: bytes.NewReader(bytes.Repeat([]byte{7}, 256)),
		Now:          func() time.Time { return time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC) },
	}
	plan, err := installer.PlanDockerCompose(context.Background(), DockerComposeInstallOptions{
		Version:        "2.7.0",
		ManifestSource: manifestPath,
		OutputDir:      outputDir,
		APIPort:        "18080",
		UIPort:         "18000",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Target != "docker-compose" || !strings.Contains(plan.Command, "docker compose") || !strings.Contains(plan.Command, "cd "+outputDir) {
		t.Fatalf("plan = %#v", plan)
	}
	if err := WriteInstallPlan(plan, false); err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(filepath.Join(outputDir, "docker-compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	composeText := string(compose)
	for _, required := range []string{
		"image: ${NOPSAI_API_IMAGE",
		"NOPSAI_SERVICE_NAME: nopsai",
		"SYSTEM_LOGS_PROVIDER: docker",
		"nopsai-docker-socket-proxy",
	} {
		if !strings.Contains(composeText, required) {
			t.Fatalf("compose is missing %q in:\n%s", required, composeText)
		}
	}
	if strings.Contains(composeText, "build:") || strings.Contains(composeText, ":latest") {
		t.Fatalf("compose should be deployment-only and not floating:\n%s", composeText)
	}
	env, err := os.ReadFile(filepath.Join(outputDir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	envText := string(env)
	if !strings.Contains(envText, "NOPSAI_VERSION=2.7.0") || !strings.Contains(envText, "NOPSAI_API_PORT=18080") || !strings.Contains(envText, manifest.Images["dockerSocketProxy"]) {
		t.Fatalf(".env = %s", envText)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(filepath.Join(outputDir, ".env"))
		if info.Mode().Perm() != 0o600 {
			t.Fatalf(".env permissions = %o", info.Mode().Perm())
		}
	}
	rawLock, err := os.ReadFile(filepath.Join(outputDir, ".nopsai", "install.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawLock), "BwcHBwcH") {
		t.Fatalf("install lock leaked generated secret material: %s", rawLock)
	}
	var lock InstallLock
	if err := json.Unmarshal(rawLock, &lock); err != nil {
		t.Fatal(err)
	}
	if lock.Target != "docker-compose" || lock.Version != "2.7.0" || lock.Images["api"] != manifest.Images["api"] || lock.FileHashes[".env"] != "" {
		t.Fatalf("lock = %#v", lock)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "db", "init.sql")); err != nil {
		t.Fatalf("embedded db init was not written: %v", err)
	}
}

func TestDockerComposeInstallRequiresComposeCapabilityAndSocketProxyImage(t *testing.T) {
	manifestPath, _ := writeInstallManifestFixture(t, []string{
		compatibility.CapabilityAPIV1,
		compatibility.CapabilityPlatformHelm,
		compatibility.CapabilityRunnerDocker,
	})
	installer := Installer{Resolver: ManifestResolver{}, CLI: installCLIInfo("2.7.0"), RandomReader: bytes.NewReader(bytes.Repeat([]byte{1}, 256))}
	_, err := installer.PlanDockerCompose(context.Background(), DockerComposeInstallOptions{Version: "2.7.0", ManifestSource: manifestPath})
	if err == nil || !strings.Contains(err.Error(), compatibility.CapabilityPlatformCompose) {
		t.Fatalf("capability error = %v", err)
	}
}

func TestKubernetesValuesInstallPlanRendersEditableValues(t *testing.T) {
	manifestPath, manifest := writeInstallManifestFixture(t, []string{
		compatibility.CapabilityAPIV1,
		compatibility.CapabilityPlatformCompose,
		compatibility.CapabilityPlatformHelm,
		compatibility.CapabilityRunnerDocker,
		compatibility.CapabilityRunnerK8s,
	})
	outputDir := filepath.Join(t.TempDir(), "k8s")
	installer := Installer{Resolver: ManifestResolver{}, CLI: installCLIInfo("2.7.0")}
	plan, err := installer.PlanKubernetesValues(context.Background(), KubernetesValuesOptions{
		Version:        "2.7.0",
		ManifestSource: manifestPath,
		OutputDir:      outputDir,
		ValuesFile:     "prod/values.yaml",
		ReleaseName:    "nopsai-prod",
		Namespace:      "nopsai-system",
		ExistingSecret: "nopsai-prod-secrets",
		IngressHost:    "nopsai.example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Command, "nopsai install kubernetes") ||
		!strings.Contains(plan.Command, "--output-dir .") ||
		!strings.Contains(plan.Command, "--values-file prod/values.yaml") ||
		!strings.Contains(plan.Command, "--deploy") {
		t.Fatalf("command = %q", plan.Command)
	}
	if err := WriteInstallPlan(plan, false); err != nil {
		t.Fatal(err)
	}
	k8sRunnerRepository, k8sRunnerDigest, err := compatibility.SplitImageReference(manifest.Images["k8sRunner"])
	if err != nil {
		t.Fatal(err)
	}
	values, err := os.ReadFile(filepath.Join(outputDir, "prod", "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	valuesText := string(values)
	for _, required := range []string{
		`existingSecret: "nopsai-prod-secrets"`,
		`host: "nopsai.example.com"`,
		`provider: kubernetes`,
		`repository: "` + k8sRunnerRepository + `"`,
		`digest: "` + k8sRunnerDigest + `"`,
	} {
		if !strings.Contains(valuesText, required) {
			t.Fatalf("values missing %q in:\n%s", required, valuesText)
		}
	}
}

func TestWriteInstallPlanRefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	plan := InstallPlan{
		OutputDir: dir,
		Files: []InstallFile{{
			RelativePath: "docker-compose.yaml",
			Mode:         0o644,
			Contents:     []byte("services: {}\n"),
		}},
	}
	if err := WriteInstallPlan(plan, false); err != nil {
		t.Fatal(err)
	}
	if err := WriteInstallPlan(plan, false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("overwrite error = %v", err)
	}
	plan.Files[0].Contents = []byte("name: replaced\n")
	if err := WriteInstallPlan(plan, true); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(dir, "docker-compose.yaml"))
	if err != nil || string(contents) != "name: replaced\n" {
		t.Fatalf("forced contents = %q, %v", contents, err)
	}
	if err := WriteInstallPlan(InstallPlan{OutputDir: dir, Files: []InstallFile{{RelativePath: "../outside", Contents: []byte("bad")}}}, true); err == nil {
		t.Fatal("path traversal was accepted")
	}
}

func writeInstallManifestFixture(t *testing.T, capabilities []string) (string, compatibility.Manifest) {
	t.Helper()
	images := make(map[string]string, len(compatibility.RequiredPlatformImages))
	for _, name := range compatibility.RequiredPlatformImages {
		images[name] = "ghcr.io/example/nopsai-" + strings.ToLower(name) + "@" + testInstallSHA("a")
	}
	manifest := compatibility.Manifest{
		SchemaVersion: "v1",
		Version:       "2.7.0",
		Chart: compatibility.ChartArtifact{
			Reference: "oci://ghcr.io/example/charts/nopsai",
			Version:   "2.7.0",
			Digest:    testInstallSHA("b"),
		},
		Images:        images,
		Compatibility: compatibility.ManifestCompatibility{CLI: ">=2.0.0 <3.0.0", API: "v1", RunnerProtocol: 1},
		Database:      compatibility.DatabaseContract{MigrationVersion: 1, RollbackSafe: false, RollbackPolicy: "forward-only"},
		Capabilities:  capabilities,
	}
	contents, err := compatibility.CanonicalJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "release-manifest.json")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	decoded, err := compatibility.DecodeManifest(bytes.NewReader(contents))
	if err != nil {
		t.Fatal(err)
	}
	return path, decoded
}

func testInstallSHA(character string) string {
	return "sha256:" + strings.Repeat(character, 64)
}

func installCLIInfo(version string) buildinfo.Info {
	return buildinfo.Info{
		Version:               version,
		APIVersion:            "v1",
		RunnerProtocolVersion: 1,
		PlatformCompatibility: ">=2.0.0 <3.0.0",
	}
}

package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	outputDir := filepath.Join(t.TempDir(), "install")
	installer := Installer{
		CLI:          installCLIInfo("2.7.0"),
		RandomReader: bytes.NewReader(bytes.Repeat([]byte{7}, 256)),
		Now:          func() time.Time { return time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC) },
	}
	plan, err := installer.PlanDockerCompose(context.Background(), DockerComposeInstallOptions{
		Version:                "2.7.0",
		OutputDir:              outputDir,
		APIPort:                "18080",
		UIPort:                 "18000",
		NopsaiAPIURL:           "http://nopsai-api.internal:8080",
		DispatcherAddress:      "dispatcher.internal:9090",
		AAAAPIURL:              "http://aaa.internal:8082",
		GitBotAPIURL:           "http://git-bot.internal:8081",
		GotenbergURL:           "http://gotenberg.internal:3000",
		DockerNetworkName:      "nopsai-prod-net",
		BootstrapAdminEmail:    "platform-admin@example.com",
		BootstrapAdminPassword: "custom-admin-password",
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
		"NOPSAI_BOOTSTRAP_ADMIN_EMAIL: ${NOPSAI_BOOTSTRAP_ADMIN_EMAIL",
		"NOPSAI_BOOTSTRAP_ADMIN_PASSWORD: ${NOPSAI_BOOTSTRAP_ADMIN_PASSWORD",
		"SYSTEM_LOGS_PROVIDER: docker",
		"nopsai-docker-socket-proxy",
		"${DISPATCHER_GRPC_ADDRESS:-dispatcher:9090}",
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
	for _, required := range []string{
		"NOPSAI_VERSION=2.7.0",
		"NOPSAI_API_PORT=18080",
		"NOPSAI_INTERNAL_API_URL=http://nopsai-api.internal:8080",
		"DISPATCHER_GRPC_ADDRESS=dispatcher.internal:9090",
		"AAA_API_URL=http://aaa.internal:8082",
		"GIT_BOT_API_URL=http://git-bot.internal:8081",
		"FINAL_OUTPUT_PDF_RENDERER_URL=http://gotenberg.internal:3000",
		"DOCKER_NETWORK_NAME=nopsai-prod-net",
		"NOPSAI_BOOTSTRAP_ADMIN_EMAIL=platform-admin@example.com",
		"NOPSAI_BOOTSTRAP_ADMIN_PASSWORD=custom-admin-password",
		"NOPSAI_DOCKER_SOCKET_PROXY_IMAGE=ghcr.io/hosein-yousefii/nopsai-docker-socket-proxy:2.7.0",
	} {
		if !strings.Contains(envText, required) {
			t.Fatalf(".env missing %q in:\n%s", required, envText)
		}
	}
	if !strings.Contains(envText, "NOPSAI_UI_PORT=18000") {
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
	if strings.Contains(string(rawLock), "BwcHBwcH") || strings.Contains(string(rawLock), "custom-admin-password") {
		t.Fatalf("install lock leaked generated secret material: %s", rawLock)
	}
	var lock InstallLock
	if err := json.Unmarshal(rawLock, &lock); err != nil {
		t.Fatal(err)
	}
	if lock.Target != "docker-compose" || lock.Version != "2.7.0" || lock.Images["api"] != "ghcr.io/hosein-yousefii/nopsai-api:2.7.0" || lock.FileHashes[".env"] != "" || strings.Contains(string(rawLock), "manifestDigest") {
		t.Fatalf("lock = %#v", lock)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "db", "init.sql")); err != nil {
		t.Fatalf("embedded db init was not written: %v", err)
	}
}

func TestDockerComposeInstallRejectsDefaultBootstrapAdminPassword(t *testing.T) {
	installer := Installer{CLI: installCLIInfo("2.7.0")}
	_, err := installer.PlanDockerCompose(context.Background(), DockerComposeInstallOptions{
		Version:                "2.7.0",
		BootstrapAdminPassword: "admin",
	})
	if err == nil || !strings.Contains(err.Error(), "built-in development password") {
		t.Fatalf("bootstrap admin password error = %v", err)
	}
}

func TestDockerComposeInstallRequiresComposeCapability(t *testing.T) {
	cli := installCLIInfo("2.7.0")
	cli.Capabilities = []string{compatibility.CapabilityAPIV1, compatibility.CapabilityPlatformHelm, compatibility.CapabilityRunnerDocker}
	installer := Installer{CLI: cli, RandomReader: bytes.NewReader(bytes.Repeat([]byte{1}, 256))}
	_, err := installer.PlanDockerCompose(context.Background(), DockerComposeInstallOptions{Version: "2.7.0"})
	if err == nil || !strings.Contains(err.Error(), compatibility.CapabilityPlatformCompose) {
		t.Fatalf("capability error = %v", err)
	}
}

func TestKubernetesValuesInstallPlanRendersEditableValues(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "k8s")
	installer := Installer{CLI: installCLIInfo("2.7.0")}
	plan, err := installer.PlanKubernetesValues(context.Background(), KubernetesValuesOptions{
		Version:                         "2.7.0",
		OutputDir:                       outputDir,
		ValuesFile:                      "prod/values.yaml",
		ReleaseName:                     "nopsai-prod",
		Namespace:                       "nopsai-system",
		ExistingSecret:                  "nopsai-prod-secrets",
		IngressHost:                     "nopsai.example.com",
		NopsaiAPIURL:                    "http://nopsai-api.prod.svc:8080",
		DispatcherAddress:               "dispatcher.prod.svc:9090",
		AAAAPIURL:                       "http://aaa.prod.svc:8082",
		GitBotAPIURL:                    "http://git-bot.prod.svc:8081",
		GotenbergURL:                    "http://gotenberg.prod.svc:3000",
		BootstrapAdminEmail:             "platform-admin@example.com",
		BootstrapAdminPasswordSecretKey: "initial-admin-password",
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
	values, err := os.ReadFile(filepath.Join(outputDir, "prod", "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	valuesText := string(values)
	for _, required := range []string{
		`existingSecret: "nopsai-prod-secrets"`,
		`host: "nopsai.example.com"`,
		`nopsaiAPIURL: "http://nopsai-api.prod.svc:8080"`,
		`dispatcherGRPCAddress: "dispatcher.prod.svc:9090"`,
		`aaaAPIURL: "http://aaa.prod.svc:8082"`,
		`gitBotAPIURL: "http://git-bot.prod.svc:8081"`,
		`gotenbergURL: "http://gotenberg.prod.svc:3000"`,
		`email: "platform-admin@example.com"`,
		`bootstrapAdminPassword: "initial-admin-password"`,
		`provider: kubernetes`,
		`repository: "ghcr.io/hosein-yousefii/nopsai-k8s-runner"`,
		`tag: "2.7.0"`,
		`digest: ""`,
	} {
		if !strings.Contains(valuesText, required) {
			t.Fatalf("values missing %q in:\n%s", required, valuesText)
		}
	}
}

func TestKubernetesInstallDeploysVersionedOCIChartAndWritesLock(t *testing.T) {
	valuesPath := filepath.Join(t.TempDir(), "values.yaml")
	if err := os.WriteFile(valuesPath, []byte("global:\n  releaseVersion: \"2.7.0\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(t.TempDir(), ".nopsai", "release.lock")
	var sawUpgrade bool
	installer := Installer{
		CLI: installCLIInfo("2.7.0"),
		Runner: func(_ context.Context, name string, args []string, _, _ io.Writer) error {
			if name != "helm" {
				t.Fatalf("process = %s", name)
			}
			sawUpgrade = true
			for _, required := range []string{"upgrade", "--install", "nopsai-prod", DefaultInstallChartReference, "--version", "2.7.0", "--namespace", "nopsai-system", "--values", valuesPath, "--create-namespace", "--wait"} {
				if !containsInstallArgument(args, required) {
					t.Fatalf("helm args missing %q in %#v", required, args)
				}
			}
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC) },
	}
	plan, err := installer.DeployKubernetesValues(context.Background(), KubernetesInstallDeployOptions{
		Version:     "2.7.0",
		ValuesFiles: []string{valuesPath},
		ReleaseName: "nopsai-prod",
		Namespace:   "nopsai-system",
		Wait:        true,
		LockFile:    lockPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawUpgrade || plan.ChartReference != DefaultInstallChartReference || plan.ChartVersion != "2.7.0" || !strings.HasPrefix(plan.ValuesHash, "sha256:") {
		t.Fatalf("plan = %#v, sawUpgrade=%v", plan, sawUpgrade)
	}
	rawLock, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	var lock InstallDeploymentLock
	if err := json.Unmarshal(rawLock, &lock); err != nil {
		t.Fatal(err)
	}
	if lock.Target != "kubernetes" || lock.ChartReference != DefaultInstallChartReference || lock.Images["api"] != "ghcr.io/hosein-yousefii/nopsai-api:2.7.0" {
		t.Fatalf("lock = %#v", lock)
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

func installCLIInfo(version string) buildinfo.Info {
	return buildinfo.Info{
		Version:               version,
		APIVersion:            "v1",
		RunnerProtocolVersion: 1,
		PlatformCompatibility: ">=2.0.0 <3.0.0",
		Capabilities:          buildinfo.Current().Capabilities,
	}
}

func containsInstallArgument(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

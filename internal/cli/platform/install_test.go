package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

var (
	testPlatformVersion            = platformTestCompatibilityLowerBound()
	testOtherPlatformVersion       = platformTestVersionWithPatchOffset(testPlatformVersion, 1)
	testUnsupportedPlatformVersion = platformTestCompatibilityUpperBound()
)

func TestDockerComposeInstallPlanWritesVersionedArtifacts(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "install")
	installer := Installer{
		CLI:          installCLIInfo(testPlatformVersion),
		RandomReader: bytes.NewReader(bytes.Repeat([]byte{7}, 256)),
		Now:          func() time.Time { return time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC) },
	}
	plan, err := installer.PlanDockerCompose(context.Background(), DockerComposeInstallOptions{
		Version:                testPlatformVersion,
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
		"DISPATCHER_TLS_SECRET: ${DISPATCHER_TLS_SECRET:-}",
		"NOPSAI_PLATFORM_ID: ${NOPSAI_PLATFORM_ID:?NOPSAI_PLATFORM_ID is required}",
		"SYSTEM_LOGS_PROVIDER: ${SYSTEM_LOGS_PROVIDER:-docker,kubernetes}",
		"nopsai-docker-socket-proxy",
		"nopsai.io/platform-id: ${NOPSAI_PLATFORM_ID:?NOPSAI_PLATFORM_ID is required}",
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
		"NOPSAI_VERSION=" + testPlatformVersion,
		"NOPSAI_API_PORT=18080",
		"NOPSAI_INTERNAL_API_URL=http://nopsai-api.internal:8080",
		"DISPATCHER_GRPC_ADDRESS=dispatcher.internal:9090",
		"AAA_API_URL=http://aaa.internal:8082",
		"GIT_BOT_API_URL=http://git-bot.internal:8081",
		"FINAL_OUTPUT_PDF_RENDERER_URL=http://gotenberg.internal:3000",
		"DOCKER_NETWORK_NAME=nopsai-prod-net",
		"DISPATCHER_TLS_SECRET=",
		"NOPSAI_BOOTSTRAP_ADMIN_EMAIL=platform-admin@example.com",
		"NOPSAI_BOOTSTRAP_ADMIN_PASSWORD=custom-admin-password",
		"NOPSAI_DOCKER_SOCKET_PROXY_IMAGE=ghcr.io/nopsai/nopsai-docker-socket-proxy:" + testPlatformVersion,
	} {
		if !strings.Contains(envText, required) {
			t.Fatalf(".env missing %q in:\n%s", required, envText)
		}
	}
	if !strings.Contains(envText, "NOPSAI_UI_PORT=18000") {
		t.Fatalf(".env = %s", envText)
	}
	masterKey := envValue(envText, "NOPSAI_MASTER_KEY")
	if masterKey == "" {
		t.Fatalf(".env missing NOPSAI_MASTER_KEY in:\n%s", envText)
	}
	if want := "NOPSAI_PLATFORM_ID=" + installPlatformID(masterKey); !strings.Contains(envText, want) {
		t.Fatalf(".env missing %q in:\n%s", want, envText)
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
	if lock.Target != "docker-compose" || lock.Version != testPlatformVersion || lock.Images["api"] != "ghcr.io/nopsai/nopsai-api:"+testPlatformVersion || lock.FileHashes[".env"] != "" || strings.Contains(string(rawLock), "manifestDigest") {
		t.Fatalf("lock = %#v", lock)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "db", "init.sql")); err != nil {
		t.Fatalf("embedded db init was not written: %v", err)
	}
}

func envValue(envText, key string) string {
	prefix := key + "="
	for _, line := range strings.Split(envText, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	return ""
}

func TestDockerComposeInstallRejectsDefaultBootstrapAdminPassword(t *testing.T) {
	installer := Installer{CLI: installCLIInfo(testPlatformVersion)}
	_, err := installer.PlanDockerCompose(context.Background(), DockerComposeInstallOptions{
		Version:                testPlatformVersion,
		BootstrapAdminPassword: "admin",
	})
	if err == nil || !strings.Contains(err.Error(), "built-in development password") {
		t.Fatalf("bootstrap admin password error = %v", err)
	}
}

func TestDockerComposeInstallRequiresComposeCapability(t *testing.T) {
	cli := installCLIInfo(testPlatformVersion)
	cli.Capabilities = []string{compatibility.CapabilityAPIV1, compatibility.CapabilityPlatformHelm, compatibility.CapabilityRunnerDocker}
	installer := Installer{CLI: cli, RandomReader: bytes.NewReader(bytes.Repeat([]byte{1}, 512))}
	_, err := installer.PlanDockerCompose(context.Background(), DockerComposeInstallOptions{Version: testPlatformVersion})
	if err == nil || !strings.Contains(err.Error(), compatibility.CapabilityPlatformCompose) {
		t.Fatalf("capability error = %v", err)
	}
}

func TestDockerComposeInstallRepairsStaleLinkedPlatformCompatibility(t *testing.T) {
	version := platformTestVersionWithPatchOffset(testPlatformVersion, 745)
	cli := installCLIInfo(version)
	cli.PlatformCompatibility = ">=2.0.0,<3.0.0"
	installer := Installer{CLI: cli, RandomReader: bytes.NewReader(bytes.Repeat([]byte{1}, 512))}
	plan, err := installer.PlanDockerCompose(context.Background(), DockerComposeInstallOptions{Version: version})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Version != version || plan.CLI != version {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestDockerComposeInstallStillRejectsVersionsOutsideCLIReleaseSeries(t *testing.T) {
	cli := installCLIInfo(platformTestVersionWithPatchOffset(testPlatformVersion, 745))
	cli.PlatformCompatibility = ">=2.0.0,<3.0.0"
	installer := Installer{CLI: cli, RandomReader: bytes.NewReader(bytes.Repeat([]byte{1}, 256))}
	_, err := installer.PlanDockerCompose(context.Background(), DockerComposeInstallOptions{Version: testUnsupportedPlatformVersion})
	if err == nil || !strings.Contains(err.Error(), "supported range is") {
		t.Fatalf("unsupported version error = %v", err)
	}
}

func TestKubernetesInstallRepairsStaleLinkedPlatformCompatibility(t *testing.T) {
	version := platformTestVersionWithPatchOffset(testPlatformVersion, 745)
	cli := installCLIInfo(version)
	cli.PlatformCompatibility = ">=2.0.0,<3.0.0"
	installer := Installer{CLI: cli}
	plan, err := installer.PlanKubernetesValues(context.Background(), KubernetesValuesOptions{Version: version})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Target != "kubernetes" || plan.Version != version || plan.CLI != version {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestKubernetesValuesInstallPlanRendersEditableValues(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "k8s")
	installer := Installer{CLI: installCLIInfo(testPlatformVersion), RandomReader: bytes.NewReader(bytes.Repeat([]byte{8}, 512))}
	plan, err := installer.PlanKubernetesValues(context.Background(), KubernetesValuesOptions{
		Version:                         testPlatformVersion,
		OutputDir:                       outputDir,
		ValuesFile:                      "prod/values.yaml",
		SecretFile:                      "prod/nopsai-secrets.yaml",
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
		BootstrapAdminPassword:          "custom-admin-password",
		BootstrapAdminPasswordSecretKey: "initial-admin-password",
		PostgresDatabase:                "nopsai_prod",
		PostgresUser:                    "nopsai_prod_user",
		PostgresPassword:                "custom-postgres-password",
		DatabaseURL:                     "postgres://nopsai_prod_user:custom-postgres-password@postgres:5432/nopsai_prod?sslmode=disable",
		MasterKey:                       "custom-master-key-with-32-characters",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Command, "nopsai install kubernetes") ||
		!strings.Contains(plan.Command, "--output-dir .") ||
		!strings.Contains(plan.Command, "--values-file prod/values.yaml") ||
		!strings.Contains(plan.Command, "kubectl apply -f prod/nopsai-secrets.yaml") ||
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
		`dispatcherTLSSecret: dispatcher-tls-secret`,
		`postgresPassword: postgres-password`,
		`email: "platform-admin@example.com"`,
		`bootstrapAdminPassword: "initial-admin-password"`,
		`database: "nopsai_prod"`,
		`username: "nopsai_prod_user"`,
		`metricsRequireAuth: true`,
		`postgres:`,
		`enabled: true`,
		`repository: postgres`,
		`passwordKey: postgres-password`,
		`storageClass: ""`,
		`provider: kubernetes,docker`,
		`dockerHost: ""`,
		`repository: "ghcr.io/nopsai/nopsai-k8s-runner"`,
		`tag: ""`,
		`digest: ""`,
	} {
		if !strings.Contains(valuesText, required) {
			t.Fatalf("values missing %q in:\n%s", required, valuesText)
		}
	}
	if strings.Contains(valuesText, "dockerSocketProxy") || strings.Contains(valuesText, "nopsai-docker-socket-proxy") {
		t.Fatalf("kubernetes values should not include Docker socket proxy settings:\n%s", valuesText)
	}
	secret, err := os.ReadFile(filepath.Join(outputDir, "prod", "nopsai-secrets.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	secretText := string(secret)
	for _, required := range []string{
		`kind: Secret`,
		`name: "nopsai-prod-secrets"`,
		`namespace: "nopsai-system"`,
		`database-url: "postgres://nopsai_prod_user:custom-postgres-password@postgres:5432/nopsai_prod?sslmode=disable"`,
		`postgres-password: "custom-postgres-password"`,
		`master-key: "custom-master-key-with-32-characters"`,
		`initial-admin-password: "custom-admin-password"`,
	} {
		if !strings.Contains(secretText, required) {
			t.Fatalf("Secret manifest missing %q in:\n%s", required, secretText)
		}
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(filepath.Join(outputDir, "prod", "nopsai-secrets.yaml"))
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("Secret manifest permissions = %o", info.Mode().Perm())
		}
	}
	readme, err := os.ReadFile(filepath.Join(outputDir, "installation.md"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "README.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy README.md should not be generated: %v", err)
	}
	readmeText := string(readme)
	for _, required := range []string{
		"# NopsAI Kubernetes Installation",
		"Generated by `nopsai install kubernetes` for NopsAI " + testPlatformVersion + ".",
		"## Requirements",
		"## Registry Pull Secrets",
		"## Image Pull Secrets",
		"## Service Accounts",
		"## Apply Secrets",
		"`prod/nopsai-secrets.yaml`",
		"| PostgreSQL password for bundled PostgreSQL | `postgres-password` |",
		"| Bootstrap admin password | `initial-admin-password` |",
		"kubectl apply -f prod/nopsai-secrets.yaml",
		"## Review Values",
		"Bundled PostgreSQL defaults to database `nopsai_prod` and user `nopsai_prod_user`",
		"## Deploy NopsAI Helm",
		"nopsai install kubernetes --output-dir . --values-file prod/values.yaml --release nopsai-prod --namespace nopsai-system --deploy",
		"--values overrides/prod.yaml",
		"helm upgrade --install nopsai-prod oci://ghcr.io/nopsai/charts/nopsai --version " + testPlatformVersion + " --namespace nopsai-system --create-namespace --values prod/values.yaml",
		"## Generated Secrets",
	} {
		if !strings.Contains(readmeText, required) {
			t.Fatalf("installation guide missing %q in:\n%s", required, readmeText)
		}
	}
	if strings.Contains(readmeText, "openssl rand") || strings.Contains(readmeText, "--from-literal") {
		t.Fatalf("installation guide still contains manual secret generation commands:\n%s", readmeText)
	}
	rawLock, err := os.ReadFile(filepath.Join(outputDir, ".nopsai", "install.lock"))
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"custom-admin-password", "custom-postgres-password", "custom-master-key-with-32-characters"} {
		if strings.Contains(string(rawLock), leaked) {
			t.Fatalf("install lock leaked secret %q: %s", leaked, rawLock)
		}
	}
	var lock InstallLock
	if err := json.Unmarshal(rawLock, &lock); err != nil {
		t.Fatal(err)
	}
	if lock.FileHashes["installation.md"] == "" || lock.FileHashes["prod/values.yaml"] == "" {
		t.Fatalf("lock does not hash generated guide and values: %#v", lock.FileHashes)
	}
	if _, ok := lock.FileHashes["prod/nopsai-secrets.yaml"]; ok {
		t.Fatalf("lock should not hash generated Secret manifest: %#v", lock.FileHashes)
	}
	if _, ok := lock.Images["dockerSocketProxy"]; ok {
		t.Fatalf("kubernetes install lock should not include docker socket proxy image: %#v", lock.Images)
	}
}

func TestKubernetesInstallDeploysVersionedOCIChartAndWritesLock(t *testing.T) {
	valuesPath := filepath.Join(t.TempDir(), "values.yaml")
	if err := os.WriteFile(valuesPath, []byte("global:\n  releaseVersion: \""+testPlatformVersion+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(t.TempDir(), ".nopsai", "release.lock")
	var sawUpgrade bool
	installer := Installer{
		CLI: installCLIInfo(testPlatformVersion),
		Runner: func(_ context.Context, name string, args []string, _, _ io.Writer) error {
			if name != "helm" {
				t.Fatalf("process = %s", name)
			}
			sawUpgrade = true
			for _, required := range []string{"upgrade", "--install", "nopsai-prod", DefaultInstallChartReference, "--version", testPlatformVersion, "--namespace", "nopsai-system", "--values", valuesPath, "--create-namespace", "--wait"} {
				if !containsInstallArgument(args, required) {
					t.Fatalf("helm args missing %q in %#v", required, args)
				}
			}
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC) },
	}
	plan, err := installer.DeployKubernetesValues(context.Background(), KubernetesInstallDeployOptions{
		Version:     testPlatformVersion,
		ValuesFiles: []string{valuesPath},
		ReleaseName: "nopsai-prod",
		Namespace:   "nopsai-system",
		Wait:        true,
		LockFile:    lockPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawUpgrade || plan.ChartReference != DefaultInstallChartReference || plan.ChartVersion != testPlatformVersion || !strings.HasPrefix(plan.ValuesHash, "sha256:") {
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
	if lock.Target != "kubernetes" || lock.ChartReference != DefaultInstallChartReference || lock.Images["api"] != "ghcr.io/nopsai/nopsai-api:"+testPlatformVersion {
		t.Fatalf("lock = %#v", lock)
	}
	if _, ok := lock.Images["dockerSocketProxy"]; ok {
		t.Fatalf("kubernetes install lock should not include docker socket proxy image: %#v", lock.Images)
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
		PlatformCompatibility: buildinfo.DefaultPlatformCompatibility,
		Capabilities:          buildinfo.Current().Capabilities,
	}
}

func platformTestCompatibilityLowerBound() string {
	rangeValue, err := compatibility.ParseRange(buildinfo.DefaultPlatformCompatibility)
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

func platformTestCompatibilityUpperBound() string {
	rangeValue, err := compatibility.ParseRange(buildinfo.DefaultPlatformCompatibility)
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

func platformTestVersionWithPatchOffset(raw string, offset int) string {
	version, err := compatibility.ParseVersion(raw)
	if err != nil {
		panic(err)
	}
	version.Patch += offset
	return version.String()
}

func containsInstallArgument(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"nopsai/pkg/compatibility"
)

func TestDockerComposeUpgradeKeepsSecretsAndMovesImagePins(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "install")
	installer := newUpgradeTestInstaller(nil)
	installPlan, err := installer.PlanDockerCompose(context.Background(), DockerComposeInstallOptions{
		Version:   testPlatformVersion,
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteInstallPlan(installPlan, false); err != nil {
		t.Fatal(err)
	}
	installedEnv := readUpgradeTestEnv(t, filepath.Join(outputDir, ".env"))

	upgrader := Upgrader{Installer: installer}
	plan, err := upgrader.PlanDockerCompose(context.Background(), DockerComposeUpgradeOptions{
		Version:   testOtherPlatformVersion,
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.CurrentVersion != testPlatformVersion || plan.Version != testOtherPlatformVersion {
		t.Fatalf("plan versions = %s -> %s", plan.CurrentVersion, plan.Version)
	}
	if plan.SeriesUpgrade {
		t.Fatal("a patch upgrade must not be reported as a series upgrade")
	}
	if err := upgrader.ApplyDockerCompose(context.Background(), plan, false); err != nil {
		t.Fatal(err)
	}

	upgradedEnv := readUpgradeTestEnv(t, filepath.Join(outputDir, ".env"))
	for _, key := range []string{
		"NOPSAI_MASTER_KEY",
		"POSTGRES_PASSWORD",
		"DATABASE_URL",
		"JWT_SIGNING_KEY",
		"SERVICE_JWT_SIGNING_KEY",
		"AAA_SHARED_INTERNAL_TOKEN",
		"NOPSAI_PLATFORM_ID",
		"NOPSAI_BOOTSTRAP_ADMIN_PASSWORD",
	} {
		if installedEnv[key] == "" {
			t.Fatalf("install did not generate %s", key)
		}
		if upgradedEnv[key] != installedEnv[key] {
			t.Fatalf("upgrade rewrote %s; secrets generated at install time must survive an upgrade", key)
		}
	}
	if upgradedEnv["NOPSAI_VERSION"] != testOtherPlatformVersion {
		t.Fatalf("NOPSAI_VERSION = %q, want %q", upgradedEnv["NOPSAI_VERSION"], testOtherPlatformVersion)
	}
	for _, image := range composeImageEnvs {
		if !strings.HasSuffix(upgradedEnv[image.Env], ":"+testOtherPlatformVersion) {
			t.Fatalf("%s = %q, want the %s tag", image.Env, upgradedEnv[image.Env], testOtherPlatformVersion)
		}
	}

	lock, err := ReadInstallLock(filepath.Join(outputDir, installLockFile))
	if err != nil {
		t.Fatal(err)
	}
	if lock.Version != testOtherPlatformVersion || lock.Target != UpgradeTargetDockerCompose {
		t.Fatalf("install lock = %+v", lock)
	}
}

func TestDockerComposeUpgradeRejectsNonForwardVersions(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "install")
	installer := newUpgradeTestInstaller(nil)
	plan, err := installer.PlanDockerCompose(context.Background(), DockerComposeInstallOptions{
		Version:   testOtherPlatformVersion,
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteInstallPlan(plan, false); err != nil {
		t.Fatal(err)
	}

	upgrader := Upgrader{Installer: installer}
	_, err = upgrader.PlanDockerCompose(context.Background(), DockerComposeUpgradeOptions{
		Version:   testPlatformVersion,
		OutputDir: outputDir,
	})
	if err == nil || !strings.Contains(err.Error(), "not newer than the installed version") {
		t.Fatalf("error = %v, want a forward-only upgrade error", err)
	}
}

func TestDockerComposeUpgradeWithoutInstallExplainsNextStep(t *testing.T) {
	upgrader := Upgrader{Installer: newUpgradeTestInstaller(nil)}
	_, err := upgrader.PlanDockerCompose(context.Background(), DockerComposeUpgradeOptions{
		Version:   testPlatformVersion,
		OutputDir: filepath.Join(t.TempDir(), "missing"),
	})
	if err == nil || !strings.Contains(err.Error(), "nopsai install") {
		t.Fatalf("error = %v, want guidance to install first", err)
	}
}

func TestSeriesUpgradeCarriesChangelogAndRequiredActions(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "install")
	installer := newUpgradeTestInstaller(nil)
	installPlan, err := installer.PlanDockerCompose(context.Background(), DockerComposeInstallOptions{
		Version:   testPlatformVersion,
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteInstallPlan(installPlan, false); err != nil {
		t.Fatal(err)
	}

	seriesVersion := nextSeriesVersion(t, testPlatformVersion)
	changelog := strings.Join([]string{
		"# NopsAI " + seriesVersion,
		"",
		"## Breaking",
		"",
		"- move assistant settings to setting/system/assistant.yaml (`abc123`)",
		"",
		"## Added",
		"",
		"- platform upgrade command (`def456`)",
	}, "\n")
	upgrader := Upgrader{
		Installer: newUpgradeTestInstallerForVersion(seriesVersion),
		Changelog: func(_ context.Context, version string) (string, string, error) {
			if version != seriesVersion {
				t.Fatalf("changelog requested for %q, want %q", version, seriesVersion)
			}
			return changelog, "https://example.com/" + ChangelogAssetName(version), nil
		},
	}
	plan, err := upgrader.PlanDockerCompose(context.Background(), DockerComposeUpgradeOptions{
		Version:   seriesVersion,
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.SeriesUpgrade {
		t.Fatalf("upgrade from %s to %s should be a series upgrade", testPlatformVersion, seriesVersion)
	}
	if !strings.Contains(plan.Changelog, "## Breaking") {
		t.Fatalf("plan changelog = %q, want the published changelog body", plan.Changelog)
	}
	if plan.ChangelogSource == "" {
		t.Fatal("plan should record where the changelog came from")
	}
	wantActions := []string{
		"Breaking change: move assistant settings to setting/system/assistant.yaml (`abc123`)",
		"Back up the database before applying this series upgrade.",
	}
	for _, want := range wantActions {
		if !containsString(plan.RequiredActions, want) {
			t.Fatalf("required actions = %#v, missing %q", plan.RequiredActions, want)
		}
	}
}

func TestUpgradePlanWarnsWhenChangelogIsUnavailable(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "install")
	installer := newUpgradeTestInstaller(nil)
	installPlan, err := installer.PlanDockerCompose(context.Background(), DockerComposeInstallOptions{
		Version:   testPlatformVersion,
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteInstallPlan(installPlan, false); err != nil {
		t.Fatal(err)
	}

	upgrader := Upgrader{Installer: newUpgradeTestInstaller(nil)}
	plan, err := upgrader.PlanDockerCompose(context.Background(), DockerComposeUpgradeOptions{
		Version:   testOtherPlatformVersion,
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Changelog != "" {
		t.Fatalf("plan changelog = %q, want empty without a fetcher", plan.Changelog)
	}
}

func TestKubernetesUpgradeReusesDeploymentLock(t *testing.T) {
	workDir := t.TempDir()
	valuesFile := filepath.Join(workDir, "values.yaml")
	if err := os.WriteFile(valuesFile, []byte("ingress:\n  host: nopsai.example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lockFile := filepath.Join(workDir, "release.lock")
	writeUpgradeTestDeploymentLock(t, lockFile, InstallDeploymentLock{
		SchemaVersion:  installSchemaVersion,
		Target:         UpgradeTargetKubernetes,
		Version:        testPlatformVersion,
		CLI:            testPlatformVersion,
		ReleaseName:    "nopsai",
		Namespace:      "nopsai-prod",
		ChartReference: DefaultInstallChartReference,
		ValuesFiles:    []string{valuesFile},
		DeployedAt:     time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC),
	})

	var helmArgs []string
	installer := newUpgradeTestInstaller(func(_ context.Context, name string, args []string, _, _ io.Writer) error {
		if name == "helm" {
			helmArgs = args
		}
		return nil
	})
	upgrader := Upgrader{Installer: installer}
	plan, err := upgrader.PlanKubernetes(context.Background(), KubernetesUpgradeOptions{
		Version:  testOtherPlatformVersion,
		LockFile: lockFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ReleaseName != "nopsai" || plan.Namespace != "nopsai-prod" {
		t.Fatalf("plan = %+v, want the release and namespace from the lock", plan)
	}
	if len(plan.ValuesFiles) != 1 || plan.ValuesFiles[0] != valuesFile {
		t.Fatalf("plan values files = %#v, want the recorded values file", plan.ValuesFiles)
	}

	deployment, err := upgrader.DeployKubernetes(context.Background(), plan, false)
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Version != testOtherPlatformVersion {
		t.Fatalf("deployment version = %q, want %q", deployment.Version, testOtherPlatformVersion)
	}
	for _, want := range []string{"upgrade", "--install", "nopsai", "--namespace", "nopsai-prod", testOtherPlatformVersion} {
		if !containsInstallArgument(helmArgs, want) {
			t.Fatalf("helm args = %#v, missing %q", helmArgs, want)
		}
	}
	if !containsInstallArgument(helmArgs, "global.releaseVersion="+testOtherPlatformVersion) {
		t.Fatalf("helm args = %#v, want the chart version and image tags pinned together", helmArgs)
	}
	if !strings.Contains(plan.Command, "global.releaseVersion="+testOtherPlatformVersion) {
		t.Fatalf("printed command = %q, want the release version pin", plan.Command)
	}
}

func TestKubernetesUpgradeRecordsDeployedVersionInValues(t *testing.T) {
	workDir := t.TempDir()
	valuesFile := filepath.Join(workDir, "values.yaml")
	values := strings.Join([]string{
		"global:",
		"  # Keep this comment.",
		"  releaseVersion: " + strconv.Quote(testPlatformVersion),
		"  logLevel: info",
		"",
		"api:",
		"  replicaCount: 1",
		"",
	}, "\n")
	if err := os.WriteFile(valuesFile, []byte(values), 0o600); err != nil {
		t.Fatal(err)
	}
	lockFile := filepath.Join(workDir, "release.lock")
	writeUpgradeTestDeploymentLock(t, lockFile, InstallDeploymentLock{
		SchemaVersion:  installSchemaVersion,
		Target:         UpgradeTargetKubernetes,
		Version:        testPlatformVersion,
		CLI:            testPlatformVersion,
		ReleaseName:    "nopsai",
		Namespace:      "nopsai",
		ChartReference: DefaultInstallChartReference,
		ValuesFiles:    []string{valuesFile},
		DeployedAt:     time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC),
	})

	upgrader := Upgrader{Installer: newUpgradeTestInstaller(func(context.Context, string, []string, io.Writer, io.Writer) error { return nil })}
	plan, err := upgrader.PlanKubernetes(context.Background(), KubernetesUpgradeOptions{
		Version:  testOtherPlatformVersion,
		LockFile: lockFile,
	})
	if plan.LockFile != lockFile {
		t.Fatalf("plan lock file = %q, want the lock the plan was read from", plan.LockFile)
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, err := upgrader.DeployKubernetes(context.Background(), plan, false); err != nil {
		t.Fatal(err)
	}

	updated, err := os.ReadFile(valuesFile) // #nosec G304 -- test fixture path.
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "releaseVersion: "+strconv.Quote(testOtherPlatformVersion)) {
		t.Fatalf("values file = %s, want the deployed release version", updated)
	}
	for _, want := range []string{"# Keep this comment.", "logLevel: info", "replicaCount: 1"} {
		if !strings.Contains(string(updated), want) {
			t.Fatalf("values file lost %q:\n%s", want, updated)
		}
	}
}

func TestReplaceValuesReleaseVersionOnlyTouchesTheGlobalKey(t *testing.T) {
	contents := strings.Join([]string{
		"api:",
		"  releaseVersion: \"1.0.0\"",
		"global:",
		"  releaseVersion: \"1.0.0\"",
	}, "\n")
	updated, changed := replaceValuesReleaseVersion(contents, "2.0.0")
	if !changed {
		t.Fatal("expected the global release version to change")
	}
	if !strings.Contains(updated, "api:\n  releaseVersion: \"1.0.0\"") {
		t.Fatalf("unrelated key was rewritten:\n%s", updated)
	}
	if !strings.Contains(updated, "global:\n  releaseVersion: \"2.0.0\"") {
		t.Fatalf("global release version was not rewritten:\n%s", updated)
	}
	if _, changed := replaceValuesReleaseVersion(updated, "2.0.0"); changed {
		t.Fatal("rewriting an already-current values file should be a no-op")
	}
}

func TestIsSeriesUpgrade(t *testing.T) {
	for _, tc := range []struct {
		current string
		target  string
		want    bool
	}{
		{current: "0.22.5", target: "0.22.9", want: false},
		{current: "0.22.5", target: "0.23.0", want: true},
		{current: "1.2.0", target: "1.9.0", want: false},
		{current: "1.2.0", target: "2.0.0", want: true},
		{current: "not-a-version", target: "1.0.0", want: false},
	} {
		if got := IsSeriesUpgrade(tc.current, tc.target); got != tc.want {
			t.Fatalf("IsSeriesUpgrade(%q, %q) = %v, want %v", tc.current, tc.target, got, tc.want)
		}
	}
}

func TestParseChangelogCollectsBreakingEntries(t *testing.T) {
	changelog := ParseChangelog("1.0.0", "https://example.com/changelog.md", strings.Join([]string{
		"# NopsAI 1.0.0",
		"",
		"## Breaking",
		"",
		"- rename runner settings (`aaa111`)",
		"- drop legacy github file (`bbb222`)",
		"",
		"## Fixed",
		"",
		"- fix drift scrolling (`ccc333`)",
	}, "\n"))
	if len(changelog.Breaking) != 2 {
		t.Fatalf("breaking entries = %#v, want 2", changelog.Breaking)
	}
	actions := changelog.RequiredActions()
	if len(actions) != 2 || !strings.HasPrefix(actions[0], "Breaking change: rename runner settings") {
		t.Fatalf("required actions = %#v", actions)
	}
}

func newUpgradeTestInstaller(runner ProcessRunner) Installer {
	return newUpgradeTestInstallerForVersion(testPlatformVersion, runner)
}

func newUpgradeTestInstallerForVersion(version string, runner ...ProcessRunner) Installer {
	installer := Installer{
		CLI:          installCLIInfo(version),
		RandomReader: bytes.NewReader(bytes.Repeat([]byte{9}, 512)),
		Now:          func() time.Time { return time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC) },
	}
	if len(runner) > 0 && runner[0] != nil {
		installer.Runner = runner[0]
	}
	return installer
}

func readUpgradeTestEnv(t *testing.T, path string) map[string]string {
	t.Helper()
	contents, err := os.ReadFile(path) // #nosec G304 -- test fixture path.
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(contents), "\n") {
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		values[strings.TrimSpace(key)] = value
	}
	return values
}

func writeUpgradeTestDeploymentLock(t *testing.T, path string, lock InstallDeploymentLock) {
	t.Helper()
	contents, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}

func nextSeriesVersion(t *testing.T, raw string) string {
	t.Helper()
	version, err := compatibility.ParseVersion(raw)
	if err != nil {
		t.Fatal(err)
	}
	if version.Major == 0 {
		version.Minor++
	} else {
		version.Major++
	}
	version.Patch = 0
	return version.String()
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestUpgradeRejectsVersionsThisCLICannotGenerate(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "install")
	installer := newUpgradeTestInstaller(nil)
	plan, err := installer.PlanDockerCompose(context.Background(), DockerComposeInstallOptions{
		Version:   testPlatformVersion,
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteInstallPlan(plan, false); err != nil {
		t.Fatal(err)
	}

	upgrader := Upgrader{Installer: installer}
	_, err = upgrader.PlanDockerCompose(context.Background(), DockerComposeUpgradeOptions{
		Version:   testUnsupportedPlatformVersion,
		OutputDir: outputDir,
	})
	if err == nil || !strings.Contains(err.Error(), "nopsai update --version "+testUnsupportedPlatformVersion) {
		t.Fatalf("error = %v, want guidance to update the CLI first", err)
	}
}

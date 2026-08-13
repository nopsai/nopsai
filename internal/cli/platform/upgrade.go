package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	dbassets "nopsai/db"
	"nopsai/pkg/compatibility"
)

const (
	// UpgradeTargetDockerCompose upgrades a generated Docker Compose install.
	UpgradeTargetDockerCompose = "docker-compose"
	// UpgradeTargetKubernetes upgrades a Helm-deployed install.
	UpgradeTargetKubernetes = "kubernetes"

	maxInstallLockBytes = 1 << 20
	maxComposeEnvBytes  = 1 << 20
)

// Upgrader moves an existing install from its current version to a newer
// release. It reuses the install renderers so an upgraded install matches what
// the same CLI version would generate, and it never regenerates the secrets an
// install already owns.
type Upgrader struct {
	Installer Installer
	Changelog ChangelogFetcher
}

type DockerComposeUpgradeOptions struct {
	Version   string
	OutputDir string
}

type KubernetesUpgradeOptions struct {
	Version        string
	ChartReference string
	ValuesFiles    []string
	ReleaseName    string
	Namespace      string
	Wait           bool
	LockFile       string
}

// UpgradePlan is the reviewable result of planning an upgrade. It is printed
// before anything is written or deployed.
type UpgradePlan struct {
	Target          string            `json:"target" yaml:"target"`
	CurrentVersion  string            `json:"currentVersion" yaml:"currentVersion"`
	Version         string            `json:"version" yaml:"version"`
	CLI             string            `json:"cliVersion" yaml:"cliVersion"`
	SeriesUpgrade   bool              `json:"seriesUpgrade" yaml:"seriesUpgrade"`
	OutputDir       string            `json:"outputDir,omitempty" yaml:"outputDir,omitempty"`
	ReleaseName     string            `json:"releaseName,omitempty" yaml:"releaseName,omitempty"`
	Namespace       string            `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	ChartReference  string            `json:"chartReference,omitempty" yaml:"chartReference,omitempty"`
	ValuesFiles     []string          `json:"valuesFiles,omitempty" yaml:"valuesFiles,omitempty"`
	Images          map[string]string `json:"images" yaml:"images"`
	ChangelogSource string            `json:"changelogSource,omitempty" yaml:"changelogSource,omitempty"`
	Changelog       string            `json:"changelog,omitempty" yaml:"changelog,omitempty"`
	RequiredActions []string          `json:"requiredActions,omitempty" yaml:"requiredActions,omitempty"`
	Warnings        []string          `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	Files           []InstallFile     `json:"-" yaml:"-"`
	Command         string            `json:"command,omitempty" yaml:"command,omitempty"`
}

// PlanDockerCompose reads the install lock and environment file of an existing
// Docker Compose install and produces the files for the target version. The
// generated secrets in .env are preserved; only the version and image pins move.
func (u Upgrader) PlanDockerCompose(ctx context.Context, options DockerComposeUpgradeOptions) (UpgradePlan, error) {
	outputDir := installOutputDir(options.OutputDir)
	lock, err := ReadInstallLock(filepath.Join(outputDir, installLockFile))
	if err != nil {
		return UpgradePlan{}, err
	}
	if lock.Target != UpgradeTargetDockerCompose {
		return UpgradePlan{}, fmt.Errorf("install lock in %s targets %q; run the matching upgrade target", outputDir, lock.Target)
	}
	version, cli, err := u.Installer.resolveInstallVersion(options.Version, compatibility.CapabilityPlatformCompose, compatibility.CapabilityRunnerDocker)
	if err != nil {
		return UpgradePlan{}, upgradeVersionError(options.Version, err)
	}
	if err := requireForwardUpgrade(lock.Version, version); err != nil {
		return UpgradePlan{}, err
	}

	envPath := filepath.Join(outputDir, installEnvFile)
	existingEnv, err := readComposeEnvFile(envPath)
	if err != nil {
		return UpgradePlan{}, err
	}
	images := versionedInstallImages(version)
	for _, image := range composeImageEnvs {
		if _, err := requiredInstallImage(images, image.Key); err != nil {
			return UpgradePlan{}, err
		}
	}
	projectName, err := composeProjectName(filepath.Join(outputDir, installComposeFile))
	if err != nil {
		return UpgradePlan{}, err
	}
	compose, err := renderComposeTemplate(projectName)
	if err != nil {
		return UpgradePlan{}, err
	}
	env := upgradeComposeEnv(existingEnv, version, images)
	baseFiles := []InstallFile{
		{RelativePath: installComposeFile, Mode: 0o644, Contents: compose},
		{RelativePath: installEnvFile, Mode: 0o600, Sensitive: true, Contents: env},
		{RelativePath: installDatabaseBootstrapSQLFile, Mode: 0o644, Contents: dbassets.InitSQL()},
	}
	files, err := appendInstallLock(baseFiles, installLock(UpgradeTargetDockerCompose, version, cli, images, baseFiles, u.Installer.now(), "", ""))
	if err != nil {
		return UpgradePlan{}, err
	}

	plan := UpgradePlan{
		Target:         UpgradeTargetDockerCompose,
		CurrentVersion: lock.Version,
		Version:        version,
		CLI:            cli.Version,
		SeriesUpgrade:  IsSeriesUpgrade(lock.Version, version),
		OutputDir:      outputDir,
		Images:         cloneStrings(images),
		Files:          files,
		Command:        composeCommandText(outputDir),
	}
	plan.Warnings = append(plan.Warnings,
		installEnvFile+" keeps the secrets generated at install time; only the version and image pins are rewritten.",
		installDatabaseBootstrapSQLFile+" is refreshed for this release; it only runs when a database is created for the first time.",
		"Existing containers keep running until the compose command is applied.",
	)
	for _, key := range missingComposeEnvKeys(existingEnv, version, images) {
		plan.RequiredActions = append(plan.RequiredActions, fmt.Sprintf("Add %s to %s; this release expects it and the upgrade cannot invent a value.", key, envPath))
	}
	u.addUpgradeGuidance(ctx, &plan)
	return plan, nil
}

// ApplyDockerCompose writes the planned files and optionally restarts the stack.
func (u Upgrader) ApplyDockerCompose(ctx context.Context, plan UpgradePlan, run bool) error {
	if plan.Target != UpgradeTargetDockerCompose {
		return fmt.Errorf("upgrade plan target %q cannot be applied with Docker Compose", plan.Target)
	}
	installPlan := InstallPlan{
		Target:    plan.Target,
		Version:   plan.Version,
		CLI:       plan.CLI,
		OutputDir: plan.OutputDir,
		Files:     plan.Files,
		Command:   plan.Command,
	}
	if err := WriteInstallPlan(installPlan, true); err != nil {
		return err
	}
	if !run {
		return nil
	}
	return u.Installer.RunDockerCompose(ctx, installPlan)
}

// PlanKubernetes reads the deployment lock written by the last install or
// upgrade and prepares the Helm upgrade for the target version.
func (u Upgrader) PlanKubernetes(ctx context.Context, options KubernetesUpgradeOptions) (UpgradePlan, error) {
	lockFile := strings.TrimSpace(options.LockFile)
	if lockFile == "" {
		lockFile = DefaultLockFile
	}
	lock, err := ReadInstallDeploymentLock(lockFile)
	if err != nil {
		return UpgradePlan{}, err
	}
	if lock.Target != UpgradeTargetKubernetes {
		return UpgradePlan{}, fmt.Errorf("deployment lock %s targets %q; run the matching upgrade target", lockFile, lock.Target)
	}
	version, cli, err := u.Installer.resolveInstallVersion(options.Version, compatibility.CapabilityPlatformHelm, compatibility.CapabilityRunnerK8s)
	if err != nil {
		return UpgradePlan{}, upgradeVersionError(options.Version, err)
	}
	if err := requireForwardUpgrade(lock.Version, version); err != nil {
		return UpgradePlan{}, err
	}

	valuesFiles := append([]string(nil), options.ValuesFiles...)
	if len(valuesFiles) == 0 {
		valuesFiles = append(valuesFiles, lock.ValuesFiles...)
	}
	if len(valuesFiles) == 0 {
		return UpgradePlan{}, fmt.Errorf("no Helm values files recorded in %s; pass --values with the files used for this release", lockFile)
	}
	for _, valuesFile := range valuesFiles {
		if _, err := os.Stat(valuesFile); err != nil {
			return UpgradePlan{}, fmt.Errorf("read Helm values file %s: %w", valuesFile, err)
		}
	}

	plan := UpgradePlan{
		Target:         UpgradeTargetKubernetes,
		CurrentVersion: lock.Version,
		Version:        version,
		CLI:            cli.Version,
		SeriesUpgrade:  IsSeriesUpgrade(lock.Version, version),
		ReleaseName:    valueOrFallback(options.ReleaseName, lock.ReleaseName, DefaultReleaseName),
		Namespace:      valueOrFallback(options.Namespace, lock.Namespace, DefaultNamespace),
		ChartReference: valueOrFallback(options.ChartReference, lock.ChartReference, DefaultInstallChartReference),
		ValuesFiles:    valuesFiles,
		Images:         cloneStrings(kubernetesInstallImages(versionedInstallImages(version))),
	}
	plan.Command = shellJoin(append([]string{"helm"}, kubernetesUpgradeArgs(plan, options.Wait)...))
	plan.Warnings = append(plan.Warnings,
		"Helm upgrades apply database migrations on start; take a database backup before applying.",
		"The recorded values files are reused; review them for settings added by this release.",
	)
	u.addUpgradeGuidance(ctx, &plan)
	return plan, nil
}

// DeployKubernetes applies a planned Helm upgrade and rewrites the deployment lock.
func (u Upgrader) DeployKubernetes(ctx context.Context, plan UpgradePlan, wait bool) (KubernetesInstallDeploymentPlan, error) {
	if plan.Target != UpgradeTargetKubernetes {
		return KubernetesInstallDeploymentPlan{}, fmt.Errorf("upgrade plan target %q cannot be deployed with Helm", plan.Target)
	}
	deployment, err := u.Installer.DeployKubernetesValues(ctx, KubernetesInstallDeployOptions{
		Version:        plan.Version,
		ChartReference: plan.ChartReference,
		ValuesFiles:    plan.ValuesFiles,
		ReleaseName:    plan.ReleaseName,
		Namespace:      plan.Namespace,
		Wait:           wait,
		LockFile:       DefaultLockFile,
	})
	if err != nil {
		return KubernetesInstallDeploymentPlan{}, err
	}
	if _, err := updateValuesReleaseVersion(plan.ValuesFiles, plan.Version); err != nil {
		return deployment, fmt.Errorf("record the deployed release version in the Helm values: %w", err)
	}
	return deployment, nil
}

// addUpgradeGuidance attaches the release changelog and operator checklist. A
// series upgrade always carries the changelog; smaller upgrades only carry it
// when the release publishes breaking entries.
func (u Upgrader) addUpgradeGuidance(ctx context.Context, plan *UpgradePlan) {
	if u.Changelog == nil {
		if plan.SeriesUpgrade {
			plan.Warnings = append(plan.Warnings, "Release changelog is unavailable in this environment; review the release notes for "+plan.Version+" before applying.")
		}
		return
	}
	body, source, err := u.Changelog(ctx, plan.Version)
	if err != nil {
		plan.Warnings = append(plan.Warnings, "Release changelog could not be read: "+err.Error())
		return
	}
	changelog := ParseChangelog(plan.Version, source, body)
	plan.ChangelogSource = changelog.Source
	if plan.SeriesUpgrade || len(changelog.Breaking) > 0 {
		plan.Changelog = changelog.Body
	}
	plan.RequiredActions = append(plan.RequiredActions, changelog.RequiredActions()...)
	if plan.SeriesUpgrade {
		plan.RequiredActions = append(plan.RequiredActions,
			"Back up the database before applying this series upgrade.",
			"Update runners to "+plan.Version+" after the platform is upgraded.",
		)
	}
}

// IsSeriesUpgrade reports whether the target release changes the compatibility
// series. Releases below 1.0.0 use the minor number as their series, matching
// the compatibility ranges published in release/compatibility.yaml.
func IsSeriesUpgrade(current, target string) bool {
	currentVersion, currentErr := compatibility.ParseVersion(current)
	targetVersion, targetErr := compatibility.ParseVersion(target)
	if currentErr != nil || targetErr != nil {
		return false
	}
	if currentVersion.Major != targetVersion.Major {
		return true
	}
	return currentVersion.Major == 0 && currentVersion.Minor != targetVersion.Minor
}

// ReadInstallLock reads the lock written next to generated install files.
func ReadInstallLock(path string) (InstallLock, error) {
	var lock InstallLock
	contents, err := readBoundedFile(path, maxInstallLockBytes, "install lock")
	if err != nil {
		return InstallLock{}, err
	}
	if err := json.Unmarshal(contents, &lock); err != nil {
		return InstallLock{}, fmt.Errorf("decode install lock %s: %w", path, err)
	}
	if strings.TrimSpace(lock.Version) == "" {
		return InstallLock{}, fmt.Errorf("install lock %s does not record an installed version", path)
	}
	return lock, nil
}

// ReadInstallDeploymentLock reads the lock written after a Helm deployment.
func ReadInstallDeploymentLock(path string) (InstallDeploymentLock, error) {
	var lock InstallDeploymentLock
	contents, err := readBoundedFile(path, maxInstallLockBytes, "deployment lock")
	if err != nil {
		return InstallDeploymentLock{}, err
	}
	if err := json.Unmarshal(contents, &lock); err != nil {
		return InstallDeploymentLock{}, fmt.Errorf("decode deployment lock %s: %w", path, err)
	}
	if strings.TrimSpace(lock.Version) == "" {
		return InstallDeploymentLock{}, fmt.Errorf("deployment lock %s does not record a deployed version", path)
	}
	return lock, nil
}

func readBoundedFile(path string, limit int64, label string) ([]byte, error) {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if cleaned == "" || cleaned == "." {
		return nil, fmt.Errorf("%s path is required", label)
	}
	file, err := os.Open(cleaned) // #nosec G304 -- the operator selects the install directory.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read %s %s: no install was generated here; run nopsai install first", label, cleaned)
		}
		return nil, fmt.Errorf("read %s %s: %w", label, cleaned, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect %s %s: %w", label, cleaned, err)
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("%s %s must be a regular file no larger than %s", label, cleaned, formatByteSize(limit))
	}
	return io.ReadAll(io.LimitReader(file, limit+1))
}

// upgradeVersionError keeps the CLI-first upgrade order actionable: a CLI can
// only generate installs for the release series it was built for.
func upgradeVersionError(version string, err error) error {
	if err == nil || !strings.Contains(err.Error(), "does not support platform") {
		return err
	}
	return fmt.Errorf("%w; update this CLI first with: nopsai update --version %s", err, strings.TrimSpace(version))
}

func requireForwardUpgrade(current, target string) error {
	currentVersion, err := compatibility.ParseVersion(current)
	if err != nil {
		return fmt.Errorf("installed version %q is not a valid release version: %w", current, err)
	}
	targetVersion, err := compatibility.ParseVersion(target)
	if err != nil {
		return fmt.Errorf("target version %q is not a valid release version: %w", target, err)
	}
	if targetVersion.Compare(currentVersion) <= 0 {
		return fmt.Errorf("target version %s is not newer than the installed version %s; upgrades only move forward", targetVersion.String(), currentVersion.String())
	}
	return nil
}

type composeEnvEntry struct {
	key   string
	line  string
	isKey bool
}

func readComposeEnvFile(path string) ([]composeEnvEntry, error) {
	contents, err := readBoundedFile(path, maxComposeEnvBytes, "compose environment file")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(contents), "\n"), "\n")
	entries := make([]composeEnvEntry, 0, len(lines))
	for _, line := range lines {
		key, _, found := strings.Cut(line, "=")
		trimmedKey := strings.TrimSpace(key)
		if !found || trimmedKey == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			entries = append(entries, composeEnvEntry{line: line})
			continue
		}
		entries = append(entries, composeEnvEntry{key: trimmedKey, line: line, isKey: true})
	}
	return entries, nil
}

func upgradeComposeEnv(entries []composeEnvEntry, version string, images map[string]string) []byte {
	replacements := map[string]string{"NOPSAI_VERSION": version}
	for _, image := range composeImageEnvs {
		replacements[image.Env] = images[image.Key]
	}
	var builder strings.Builder
	for _, entry := range entries {
		if entry.isKey {
			if value, ok := replacements[entry.key]; ok {
				builder.WriteString(entry.key)
				builder.WriteString("=")
				builder.WriteString(value)
				builder.WriteString("\n")
				continue
			}
		}
		builder.WriteString(entry.line)
		builder.WriteString("\n")
	}
	return []byte(builder.String())
}

// missingComposeEnvKeys reports keys this release renders that an older install
// does not have, so the operator can supply values the upgrade must not invent.
func missingComposeEnvKeys(entries []composeEnvEntry, version string, images map[string]string) []string {
	present := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.isKey {
			present[entry.key] = struct{}{}
		}
	}
	rendered := renderComposeEnv(version, images, composeSecrets{}, DefaultInstallAPIPort, DefaultInstallUIPort, installTopology{
		NopsaiAPIURL:      DefaultInstallNopsaiAPIURL,
		DispatcherAddress: DefaultInstallDispatcherAddress,
		AAAAPIURL:         DefaultInstallAAAAPIURL,
		GitBotAPIURL:      DefaultInstallGitBotAPIURL,
		GotenbergURL:      DefaultInstallGotenbergURL,
		DockerNetworkName: DefaultInstallDockerNetworkName,
	}, DefaultInstallBootstrapAdminEmail)
	missing := []string{}
	for _, line := range strings.Split(string(rendered), "\n") {
		key, _, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			continue
		}
		if _, ok := present[key]; !ok {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	return missing
}

func composeProjectName(path string) (string, error) {
	contents, err := readBoundedFile(path, maxComposeEnvBytes, "compose file")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(contents), "\n") {
		value, found := strings.CutPrefix(line, "name:")
		if !found {
			continue
		}
		name := strings.TrimSpace(value)
		if name == "" {
			break
		}
		if err := validateHelmName("project", name); err != nil {
			return "", err
		}
		return name, nil
	}
	return DefaultInstallProjectName, nil
}

func kubernetesUpgradeArgs(plan UpgradePlan, wait bool) []string {
	args := []string{"upgrade", "--install", plan.ReleaseName, plan.ChartReference, "--version", plan.Version, "--namespace", plan.Namespace}
	for _, valuesFile := range plan.ValuesFiles {
		args = append(args, "--values", valuesFile)
	}
	args = append(args, "--set-string", "global.releaseVersion="+plan.Version, "--create-namespace")
	if wait {
		args = append(args, "--wait")
	}
	return args
}

// updateValuesReleaseVersion rewrites the pinned release version in the values
// files that already record one, so the GitOps values keep describing the
// deployed release instead of the version the install started from. It edits the
// single scalar line to preserve comments and formatting.
func updateValuesReleaseVersion(paths []string, version string) ([]string, error) {
	updated := []string{}
	for _, path := range paths {
		contents, err := readBoundedFile(path, maxValuesFileBytes, "Helm values file")
		if err != nil {
			return nil, err
		}
		next, changed := replaceValuesReleaseVersion(string(contents), version)
		if !changed {
			continue
		}
		if err := writeFileAtomic(filepath.Clean(path), []byte(next), 0o644); err != nil {
			return nil, err
		}
		updated = append(updated, path)
	}
	return updated, nil
}

func replaceValuesReleaseVersion(contents, version string) (string, bool) {
	lines := strings.Split(contents, "\n")
	inGlobal := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inGlobal = strings.HasPrefix(trimmed, "global:")
			continue
		}
		if !inGlobal {
			continue
		}
		key, value, found := strings.Cut(trimmed, ":")
		if !found || strings.TrimSpace(key) != "releaseVersion" {
			continue
		}
		if strings.Trim(strings.TrimSpace(value), `"'`) == version {
			return contents, false
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		lines[index] = indent + "releaseVersion: " + strconv.Quote(version)
		return strings.Join(lines, "\n"), true
	}
	return contents, false
}

func valueOrFallback(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func formatByteSize(limit int64) string {
	return fmt.Sprintf("%d bytes", limit)
}

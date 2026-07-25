package platform

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	dbassets "nopsai/db"
	"nopsai/pkg/buildinfo"
	"nopsai/pkg/compatibility"
)

const (
	DefaultInstallOutputDir                          = "nopsai-install"
	DefaultInstallProjectName                        = "nopsai"
	DefaultInstallUIPort                             = "80"
	DefaultInstallAPIPort                            = "8080"
	DefaultInstallPostgresDB                         = "nopsai_db"
	DefaultInstallPostgresUser                       = "nopsai_user"
	DefaultInstallNopsaiAPIURL                       = "http://nopsai:8080"
	DefaultInstallDispatcherAddress                  = "dispatcher:9090"
	DefaultInstallAAAAPIURL                          = "http://aaa:8082"
	DefaultInstallGitBotAPIURL                       = "http://git-bot:8081"
	DefaultInstallGotenbergURL                       = "http://gotenberg:3000"
	DefaultInstallDockerNetworkName                  = "nopsai-net"
	DefaultInstallBootstrapAdminEmail                = "admin@example.com"
	DefaultInstallChartReference                     = "oci://ghcr.io/nopsai/charts/nopsai"
	DefaultKubernetesValuesFile                      = "values.yaml"
	DefaultKubernetesExistingSecret                  = "nopsai-secrets"
	DefaultKubernetesBootstrapAdminPasswordSecretKey = "bootstrap-admin-password"
	installSchemaVersion                             = "v1"
	installComposeFile                               = "docker-compose.yaml"
	installEnvFile                                   = ".env"
	installLockFile                                  = ".nopsai/install.lock"
	installDatabaseBootstrapSQLFile                  = "db/init.sql"
	installBootstrapAdminMinPasswordLength           = 12
)

type Installer struct {
	Resolver     ManifestResolver
	Runner       ProcessRunner
	CLI          buildinfo.Info
	RandomReader io.Reader
	Now          func() time.Time
	Stderr       io.Writer
}

type DockerComposeInstallOptions struct {
	Version                string
	OutputDir              string
	ProjectName            string
	UIPort                 string
	APIPort                string
	NopsaiAPIURL           string
	DispatcherAddress      string
	AAAAPIURL              string
	GitBotAPIURL           string
	GotenbergURL           string
	DockerNetworkName      string
	BootstrapAdminEmail    string
	BootstrapAdminPassword string
}

type KubernetesValuesOptions struct {
	Version                         string
	OutputDir                       string
	ValuesFile                      string
	ReleaseName                     string
	Namespace                       string
	ExistingSecret                  string
	IngressHost                     string
	NopsaiAPIURL                    string
	DispatcherAddress               string
	AAAAPIURL                       string
	GitBotAPIURL                    string
	GotenbergURL                    string
	BootstrapAdminEmail             string
	BootstrapAdminPasswordSecretKey string
	Wait                            bool
}

type KubernetesInstallDeployOptions struct {
	Version        string
	ChartReference string
	ValuesFiles    []string
	ReleaseName    string
	Namespace      string
	Wait           bool
	LockFile       string
}

type InstallPlan struct {
	Target    string
	Version   string
	CLI       string
	OutputDir string
	Files     []InstallFile
	Command   string
	Warnings  []string
}

type InstallFile struct {
	RelativePath string
	Mode         fs.FileMode
	Sensitive    bool
	Contents     []byte
}

type InstallLock struct {
	SchemaVersion  string            `json:"schemaVersion" yaml:"schemaVersion"`
	Target         string            `json:"target" yaml:"target"`
	Version        string            `json:"version" yaml:"version"`
	CLI            string            `json:"cliVersion" yaml:"cliVersion"`
	ChartReference string            `json:"chartReference,omitempty" yaml:"chartReference,omitempty"`
	ChartVersion   string            `json:"chartVersion,omitempty" yaml:"chartVersion,omitempty"`
	Images         map[string]string `json:"images" yaml:"images"`
	FileHashes     map[string]string `json:"fileHashes" yaml:"fileHashes"`
	GeneratedAt    time.Time         `json:"generatedAt" yaml:"generatedAt"`
}

type KubernetesInstallDeploymentPlan struct {
	Version        string            `json:"version" yaml:"version"`
	CLI            string            `json:"cliVersion" yaml:"cliVersion"`
	ReleaseName    string            `json:"releaseName" yaml:"releaseName"`
	Namespace      string            `json:"namespace" yaml:"namespace"`
	ChartReference string            `json:"chartReference" yaml:"chartReference"`
	ChartVersion   string            `json:"chartVersion" yaml:"chartVersion"`
	Images         map[string]string `json:"images" yaml:"images"`
	ValuesFiles    []string          `json:"valuesFiles" yaml:"valuesFiles"`
	ValuesHash     string            `json:"valuesHash" yaml:"valuesHash"`
	LockFile       string            `json:"lockFile" yaml:"lockFile"`
}

type InstallDeploymentLock struct {
	SchemaVersion  string            `json:"schemaVersion" yaml:"schemaVersion"`
	Target         string            `json:"target" yaml:"target"`
	Version        string            `json:"version" yaml:"version"`
	CLI            string            `json:"cliVersion" yaml:"cliVersion"`
	ReleaseName    string            `json:"releaseName" yaml:"releaseName"`
	Namespace      string            `json:"namespace" yaml:"namespace"`
	ChartReference string            `json:"chartReference" yaml:"chartReference"`
	ChartVersion   string            `json:"chartVersion" yaml:"chartVersion"`
	Images         map[string]string `json:"images" yaml:"images"`
	ValuesFiles    []string          `json:"valuesFiles" yaml:"valuesFiles"`
	ValuesHash     string            `json:"valuesHash" yaml:"valuesHash"`
	DeployedAt     time.Time         `json:"deployedAt" yaml:"deployedAt"`
}

type composeTemplateData struct {
	ProjectName string
}

type composeSecrets struct {
	PostgresPassword       string
	JWTSigningKey          string
	ServiceJWTSigningKey   string
	AAASharedInternalToken string
	DispatcherTLSSecret    string
	MasterKey              string
	BootstrapAdminPassword string
}

type installTopology struct {
	NopsaiAPIURL      string
	DispatcherAddress string
	AAAAPIURL         string
	GitBotAPIURL      string
	GotenbergURL      string
	DockerNetworkName string
}

type imageEnv struct {
	Key string
	Env string
}

var composeImageEnvs = []imageEnv{
	{Key: "api", Env: "NOPSAI_API_IMAGE"},
	{Key: "aaa", Env: "NOPSAI_AAA_IMAGE"},
	{Key: "agent", Env: "NOPSAI_AGENT_IMAGE"},
	{Key: "dispatcher", Env: "NOPSAI_DISPATCHER_IMAGE"},
	{Key: "dockerSocketProxy", Env: "NOPSAI_DOCKER_SOCKET_PROXY_IMAGE"},
	{Key: "gitBot", Env: "NOPSAI_GIT_BOT_IMAGE"},
	{Key: "runner", Env: "NOPSAI_DOCKER_RUNNER_IMAGE"},
	{Key: "ui", Env: "NOPSAI_UI_IMAGE"},
}

func (i Installer) PlanDockerCompose(_ context.Context, options DockerComposeInstallOptions) (InstallPlan, error) {
	version, cli, err := i.resolveInstallVersion(options.Version, compatibility.CapabilityPlatformCompose, compatibility.CapabilityRunnerDocker)
	if err != nil {
		return InstallPlan{}, err
	}
	images := versionedInstallImages(version)
	outputDir := installOutputDir(options.OutputDir)
	projectName := strings.TrimSpace(options.ProjectName)
	if projectName == "" {
		projectName = DefaultInstallProjectName
	}
	if err := validateHelmName("project", projectName); err != nil {
		return InstallPlan{}, err
	}
	apiPort := strings.TrimSpace(options.APIPort)
	if apiPort == "" {
		apiPort = DefaultInstallAPIPort
	}
	uiPort := strings.TrimSpace(options.UIPort)
	if uiPort == "" {
		uiPort = DefaultInstallUIPort
	}
	if err := validateTCPPort("api port", apiPort); err != nil {
		return InstallPlan{}, err
	}
	if err := validateTCPPort("ui port", uiPort); err != nil {
		return InstallPlan{}, err
	}
	topology, err := normalizeInstallTopology(installTopology{
		NopsaiAPIURL:      options.NopsaiAPIURL,
		DispatcherAddress: options.DispatcherAddress,
		AAAAPIURL:         options.AAAAPIURL,
		GitBotAPIURL:      options.GitBotAPIURL,
		GotenbergURL:      options.GotenbergURL,
		DockerNetworkName: options.DockerNetworkName,
	}, true)
	if err != nil {
		return InstallPlan{}, err
	}
	bootstrapAdminEmail, err := normalizeInstallBootstrapAdminEmail(options.BootstrapAdminEmail)
	if err != nil {
		return InstallPlan{}, err
	}
	if options.BootstrapAdminPassword != "" {
		if err := validateInstallBootstrapAdminPassword(options.BootstrapAdminPassword); err != nil {
			return InstallPlan{}, err
		}
	}
	for _, image := range composeImageEnvs {
		if _, err := requiredInstallImage(images, image.Key); err != nil {
			return InstallPlan{}, err
		}
	}
	secrets, err := i.generateComposeSecrets(options.BootstrapAdminPassword)
	if err != nil {
		return InstallPlan{}, err
	}
	compose, err := renderComposeTemplate(projectName)
	if err != nil {
		return InstallPlan{}, err
	}
	env := renderComposeEnv(version, images, secrets, apiPort, uiPort, topology, bootstrapAdminEmail)
	baseFiles := []InstallFile{
		{RelativePath: installComposeFile, Mode: 0o644, Contents: compose},
		{RelativePath: installEnvFile, Mode: 0o600, Sensitive: true, Contents: env},
		{RelativePath: installDatabaseBootstrapSQLFile, Mode: 0o644, Contents: dbassets.InitSQL()},
	}
	files, err := appendInstallLock(baseFiles, installLock("docker-compose", version, cli, images, baseFiles, i.now(), "", ""))
	if err != nil {
		return InstallPlan{}, err
	}
	return InstallPlan{
		Target:    "docker-compose",
		Version:   version,
		CLI:       cli.Version,
		OutputDir: outputDir,
		Files:     files,
		Command:   composeCommandText(outputDir),
		Warnings: []string{
			installEnvFile + " contains generated secrets, including the bootstrap admin password, and should stay out of Git.",
			"Rotate generated secrets through your production secret manager before promoting this install beyond evaluation.",
		},
	}, nil
}

func (i Installer) PlanKubernetesValues(_ context.Context, options KubernetesValuesOptions) (InstallPlan, error) {
	version, cli, err := i.resolveInstallVersion(options.Version, compatibility.CapabilityPlatformHelm, compatibility.CapabilityRunnerK8s)
	if err != nil {
		return InstallPlan{}, err
	}
	images := kubernetesInstallImages(versionedInstallImages(version))
	outputDir := installOutputDir(options.OutputDir)
	valuesFile := strings.TrimSpace(options.ValuesFile)
	if valuesFile == "" {
		valuesFile = DefaultKubernetesValuesFile
	}
	valuesFile = filepath.ToSlash(filepath.Clean(valuesFile))
	if err := validateInstallRelativePath(valuesFile); err != nil {
		return InstallPlan{}, fmt.Errorf("values file: %w", err)
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
		return InstallPlan{}, err
	}
	if err := validateHelmName("namespace", namespace); err != nil {
		return InstallPlan{}, err
	}
	existingSecret := strings.TrimSpace(options.ExistingSecret)
	if existingSecret == "" {
		existingSecret = DefaultKubernetesExistingSecret
	}
	if err := validateHelmName("secret", existingSecret); err != nil {
		return InstallPlan{}, err
	}
	topology, err := normalizeInstallTopology(installTopology{
		NopsaiAPIURL:      options.NopsaiAPIURL,
		DispatcherAddress: options.DispatcherAddress,
		AAAAPIURL:         options.AAAAPIURL,
		GitBotAPIURL:      options.GitBotAPIURL,
		GotenbergURL:      options.GotenbergURL,
	}, false)
	if err != nil {
		return InstallPlan{}, err
	}
	bootstrapAdminEmail, err := normalizeInstallBootstrapAdminEmail(options.BootstrapAdminEmail)
	if err != nil {
		return InstallPlan{}, err
	}
	bootstrapAdminPasswordSecretKey := strings.TrimSpace(options.BootstrapAdminPasswordSecretKey)
	if bootstrapAdminPasswordSecretKey == "" {
		bootstrapAdminPasswordSecretKey = DefaultKubernetesBootstrapAdminPasswordSecretKey
	}
	if err := validateKubernetesSecretKey("bootstrap admin password secret key", bootstrapAdminPasswordSecretKey); err != nil {
		return InstallPlan{}, err
	}
	values, err := renderKubernetesValues(version, images, existingSecret, options.IngressHost, topology, bootstrapAdminEmail, bootstrapAdminPasswordSecretKey)
	if err != nil {
		return InstallPlan{}, err
	}
	readme := renderKubernetesInstallReadme(version, releaseName, namespace, valuesFile, existingSecret, bootstrapAdminEmail, bootstrapAdminPasswordSecretKey, options.Wait)
	baseFiles := []InstallFile{
		{RelativePath: valuesFile, Mode: 0o644, Contents: values},
		{RelativePath: installKubernetesReadmeFile, Mode: 0o644, Contents: readme},
	}
	files, err := appendInstallLock(baseFiles, installLock("kubernetes", version, cli, images, baseFiles, i.now(), DefaultInstallChartReference, version))
	if err != nil {
		return InstallPlan{}, err
	}
	return InstallPlan{
		Target:    "kubernetes",
		Version:   version,
		CLI:       cli.Version,
		OutputDir: outputDir,
		Files:     files,
		Command:   kubernetesCommandText(outputDir, releaseName, namespace, valuesFile, options.Wait),
		Warnings: []string{
			"values.yaml references an existing Kubernetes Secret; create it with your cluster secret manager before deploying, including the bootstrap admin password key.",
			"Do not commit raw database URLs, signing keys, or master keys to GitOps repositories.",
		},
	}, nil
}

func (i Installer) RunDockerCompose(ctx context.Context, plan InstallPlan) error {
	if plan.Target != "docker-compose" {
		return fmt.Errorf("install plan target %q cannot be run with Docker Compose", plan.Target)
	}
	if i.Runner == nil {
		return errors.New("process runner is not configured")
	}
	args := []string{
		"compose",
		"--env-file", filepath.Join(plan.OutputDir, installEnvFile),
		"-f", filepath.Join(plan.OutputDir, installComposeFile),
		"up", "-d",
	}
	stderr := i.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	return i.Runner(ctx, "docker", args, io.Discard, stderr)
}

func (i Installer) DeployKubernetesValues(ctx context.Context, options KubernetesInstallDeployOptions) (KubernetesInstallDeploymentPlan, error) {
	version, cli, err := i.resolveInstallVersion(options.Version, compatibility.CapabilityPlatformHelm, compatibility.CapabilityRunnerK8s)
	if err != nil {
		return KubernetesInstallDeploymentPlan{}, err
	}
	images := kubernetesInstallImages(versionedInstallImages(version))
	chartReference := strings.TrimSpace(options.ChartReference)
	if chartReference == "" {
		chartReference = DefaultInstallChartReference
	}
	if !strings.HasPrefix(chartReference, "oci://") {
		return KubernetesInstallDeploymentPlan{}, errors.New("install chart reference must use oci://")
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
		return KubernetesInstallDeploymentPlan{}, err
	}
	if err := validateHelmName("namespace", namespace); err != nil {
		return KubernetesInstallDeploymentPlan{}, err
	}
	valuesHash, err := hashInstallValues(options.ValuesFiles, images)
	if err != nil {
		return KubernetesInstallDeploymentPlan{}, err
	}
	if i.Runner == nil {
		return KubernetesInstallDeploymentPlan{}, errors.New("process runner is not configured")
	}
	args := []string{"upgrade", "--install", releaseName, chartReference, "--version", version, "--namespace", namespace}
	for _, valuesFile := range options.ValuesFiles {
		args = append(args, "--values", valuesFile)
	}
	args = append(args, "--create-namespace")
	if options.Wait {
		args = append(args, "--wait")
	}
	stderr := i.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	if err := i.Runner(ctx, "helm", args, io.Discard, stderr); err != nil {
		return KubernetesInstallDeploymentPlan{}, fmt.Errorf("deploy Helm release: %w", err)
	}
	lockFile := strings.TrimSpace(options.LockFile)
	if lockFile == "" {
		lockFile = DefaultLockFile
	}
	plan := KubernetesInstallDeploymentPlan{
		Version:        version,
		CLI:            cli.Version,
		ReleaseName:    releaseName,
		Namespace:      namespace,
		ChartReference: chartReference,
		ChartVersion:   version,
		Images:         cloneStrings(images),
		ValuesFiles:    append([]string(nil), options.ValuesFiles...),
		ValuesHash:     valuesHash,
		LockFile:       lockFile,
	}
	lock := InstallDeploymentLock{
		SchemaVersion:  installSchemaVersion,
		Target:         "kubernetes",
		Version:        version,
		CLI:            cli.Version,
		ReleaseName:    releaseName,
		Namespace:      namespace,
		ChartReference: chartReference,
		ChartVersion:   version,
		Images:         cloneStrings(images),
		ValuesFiles:    append([]string(nil), options.ValuesFiles...),
		ValuesHash:     valuesHash,
		DeployedAt:     i.now(),
	}
	if err := WriteInstallDeploymentLock(lockFile, lock); err != nil {
		return KubernetesInstallDeploymentPlan{}, err
	}
	return plan, nil
}

func WriteInstallPlan(plan InstallPlan, overwrite bool) error {
	if strings.TrimSpace(plan.OutputDir) == "" {
		return errors.New("install output directory is required")
	}
	if len(plan.Files) == 0 {
		return errors.New("install plan has no files")
	}
	for _, file := range plan.Files {
		path, err := installFilePath(plan.OutputDir, file.RelativePath)
		if err != nil {
			return err
		}
		info, statErr := os.Stat(path)
		switch {
		case statErr == nil:
			if info.IsDir() {
				return fmt.Errorf("%s is a directory", path)
			}
			if !overwrite {
				return fmt.Errorf("%s already exists; pass --force to replace generated install files", path)
			}
		case !errors.Is(statErr, os.ErrNotExist):
			return fmt.Errorf("inspect %s: %w", path, statErr)
		}
	}
	for _, file := range plan.Files {
		path, err := installFilePath(plan.OutputDir, file.RelativePath)
		if err != nil {
			return err
		}
		if err := writeFileAtomic(path, file.Contents, file.Mode); err != nil {
			return err
		}
	}
	return nil
}

func (i Installer) resolveInstallVersion(version string, requiredCapabilities ...string) (string, buildinfo.Info, error) {
	cli := i.CLI
	if strings.TrimSpace(cli.Version) == "" {
		cli = buildinfo.Current()
	}
	cli = normalizeInstallCLIInfo(cli)
	parsedVersion, err := compatibility.ParseVersion(version)
	if err != nil {
		return "", buildinfo.Info{}, fmt.Errorf("invalid requested install version: %w", err)
	}
	version = parsedVersion.String()
	if strings.Contains(version, "+") {
		return "", buildinfo.Info{}, errors.New("install version must not contain build metadata because container image tags do not support '+'")
	}
	if !cli.IsDevelopment() {
		platformRange, err := compatibility.ParseRange(cli.PlatformCompatibility)
		if err != nil {
			return "", buildinfo.Info{}, fmt.Errorf("invalid CLI platform compatibility: %w", err)
		}
		if !platformRange.Contains(parsedVersion) {
			return "", buildinfo.Info{}, fmt.Errorf("CLI %s does not support platform %s; supported range is %s", cli.Version, version, cli.PlatformCompatibility)
		}
	}
	if err := compatibility.RequireCapabilities(cli.Capabilities, requiredCapabilities...); err != nil {
		return "", buildinfo.Info{}, err
	}
	return version, cli, nil
}

func defaultReleaseManifestDigest(source, digest string, cli buildinfo.Info) string {
	if strings.TrimSpace(source) != "" || strings.TrimSpace(digest) != "" {
		return digest
	}
	return strings.TrimSpace(cli.ReleaseManifestDigest)
}

func (i Installer) generateComposeSecrets(bootstrapAdminPassword string) (composeSecrets, error) {
	reader := i.randomReader()
	postgresPassword, err := generateInstallSecret(reader, 32)
	if err != nil {
		return composeSecrets{}, err
	}
	jwtSigningKey, err := generateInstallSecret(reader, 48)
	if err != nil {
		return composeSecrets{}, err
	}
	serviceJWTSigningKey, err := generateInstallSecret(reader, 48)
	if err != nil {
		return composeSecrets{}, err
	}
	aaaSharedInternalToken, err := generateInstallSecret(reader, 32)
	if err != nil {
		return composeSecrets{}, err
	}
	dispatcherTLSSecret, err := generateInstallSecret(reader, 48)
	if err != nil {
		return composeSecrets{}, err
	}
	masterKey, err := generateInstallSecret(reader, 32)
	if err != nil {
		return composeSecrets{}, err
	}
	bootstrapAdminPassword = strings.TrimSpace(bootstrapAdminPassword)
	if bootstrapAdminPassword == "" {
		bootstrapAdminPassword, err = generateInstallSecret(reader, 24)
		if err != nil {
			return composeSecrets{}, err
		}
	}
	return composeSecrets{
		PostgresPassword:       postgresPassword,
		JWTSigningKey:          jwtSigningKey,
		ServiceJWTSigningKey:   serviceJWTSigningKey,
		AAASharedInternalToken: aaaSharedInternalToken,
		DispatcherTLSSecret:    dispatcherTLSSecret,
		MasterKey:              masterKey,
		BootstrapAdminPassword: bootstrapAdminPassword,
	}, nil
}

func generateInstallSecret(reader io.Reader, size int) (string, error) {
	buf := make([]byte, size)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return "", fmt.Errorf("generate install secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (i Installer) randomReader() io.Reader {
	if i.RandomReader != nil {
		return i.RandomReader
	}
	return rand.Reader
}

func (i Installer) now() time.Time {
	if i.Now != nil {
		return i.Now().UTC()
	}
	return time.Now().UTC()
}

func installOutputDir(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultInstallOutputDir
	}
	return value
}

func normalizeInstallCLIInfo(cli buildinfo.Info) buildinfo.Info {
	current := buildinfo.Current()
	if strings.TrimSpace(cli.Version) == "" {
		cli.Version = current.Version
	}
	if strings.TrimSpace(cli.APIVersion) == "" {
		cli.APIVersion = current.APIVersion
	}
	if cli.RunnerProtocolVersion < 1 {
		cli.RunnerProtocolVersion = current.RunnerProtocolVersion
	}
	if strings.TrimSpace(cli.CLICompatibility) == "" {
		cli.CLICompatibility = current.CLICompatibility
	}
	if strings.TrimSpace(cli.RunnerCompatibility) == "" {
		cli.RunnerCompatibility = current.RunnerCompatibility
	}
	if strings.TrimSpace(cli.PlatformCompatibility) == "" {
		cli.PlatformCompatibility = current.PlatformCompatibility
	}
	if len(cli.Capabilities) == 0 {
		cli.Capabilities = append([]string(nil), current.Capabilities...)
	}
	return cli
}

func versionedInstallImages(version string) map[string]string {
	repositories := map[string]string{
		"aaa":               "ghcr.io/nopsai/nopsai-aaa",
		"agent":             "ghcr.io/nopsai/nopsai-agent",
		"api":               "ghcr.io/nopsai/nopsai-api",
		"dispatcher":        "ghcr.io/nopsai/nopsai-dispatcher",
		"dockerSocketProxy": "ghcr.io/nopsai/nopsai-docker-socket-proxy",
		"gitBot":            "ghcr.io/nopsai/nopsai-git-bot",
		"k8sRunner":         "ghcr.io/nopsai/nopsai-k8s-runner",
		"runner":            "ghcr.io/nopsai/nopsai-docker-runner",
		"ui":                "ghcr.io/nopsai/nopsai-ui",
	}
	images := make(map[string]string, len(repositories))
	for key, repository := range repositories {
		images[key] = repository + ":" + version
	}
	return images
}

func kubernetesInstallImages(images map[string]string) map[string]string {
	out := cloneStrings(images)
	delete(out, "dockerSocketProxy")
	return out
}

func requiredInstallImage(images map[string]string, key string) (string, error) {
	value := strings.TrimSpace(images[key])
	if value == "" {
		return "", fmt.Errorf("install image set is missing image %q", key)
	}
	if _, _, _, err := splitInstallImageReference(value); err != nil {
		return "", fmt.Errorf("image %q: %w", key, err)
	}
	return value, nil
}

func renderComposeTemplate(projectName string) ([]byte, error) {
	var buffer bytes.Buffer
	tmpl, err := template.New("compose").Parse(dockerComposeTemplate)
	if err != nil {
		return nil, err
	}
	if err := tmpl.Execute(&buffer, composeTemplateData{ProjectName: projectName}); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func renderComposeEnv(version string, images map[string]string, secrets composeSecrets, apiPort, uiPort string, topology installTopology, bootstrapAdminEmail string) []byte {
	var builder strings.Builder
	builder.WriteString("NOPSAI_VERSION=")
	builder.WriteString(version)
	builder.WriteString("\nNOPSAI_ENVIRONMENT=production\nLOG_FORMAT=json\nLOG_LEVEL=info\n")
	builder.WriteString("NOPSAI_API_PORT=")
	builder.WriteString(apiPort)
	builder.WriteString("\nNOPSAI_UI_PORT=")
	builder.WriteString(uiPort)
	builder.WriteString("\nNOPSAI_INTERNAL_API_URL=")
	builder.WriteString(topology.NopsaiAPIURL)
	builder.WriteString("\nDISPATCHER_GRPC_ADDRESS=")
	builder.WriteString(topology.DispatcherAddress)
	builder.WriteString("\nAAA_API_URL=")
	builder.WriteString(topology.AAAAPIURL)
	builder.WriteString("\nGIT_BOT_API_URL=")
	builder.WriteString(topology.GitBotAPIURL)
	builder.WriteString("\nFINAL_OUTPUT_PDF_RENDERER_URL=")
	builder.WriteString(topology.GotenbergURL)
	builder.WriteString("\nDOCKER_NETWORK_NAME=")
	builder.WriteString(topology.DockerNetworkName)
	builder.WriteString("\nPOSTGRES_DB=")
	builder.WriteString(DefaultInstallPostgresDB)
	builder.WriteString("\nPOSTGRES_USER=")
	builder.WriteString(DefaultInstallPostgresUser)
	builder.WriteString("\nPOSTGRES_PASSWORD=")
	builder.WriteString(secrets.PostgresPassword)
	builder.WriteString("\nDATABASE_URL=postgres://")
	builder.WriteString(DefaultInstallPostgresUser)
	builder.WriteString(":")
	builder.WriteString(secrets.PostgresPassword)
	builder.WriteString("@db:5432/")
	builder.WriteString(DefaultInstallPostgresDB)
	builder.WriteString("\nSERVICE_JWT_SIGNING_KEY=")
	builder.WriteString(secrets.ServiceJWTSigningKey)
	builder.WriteString("\nJWT_SIGNING_KEY=")
	builder.WriteString(secrets.JWTSigningKey)
	builder.WriteString("\nAAA_SHARED_INTERNAL_TOKEN=")
	builder.WriteString(secrets.AAASharedInternalToken)
	builder.WriteString("\nDISPATCHER_TLS_SECRET=")
	builder.WriteString(secrets.DispatcherTLSSecret)
	builder.WriteString("\nNOPSAI_MASTER_KEY=")
	builder.WriteString(secrets.MasterKey)
	builder.WriteString("\nNOPSAI_BOOTSTRAP_ADMIN_EMAIL=")
	builder.WriteString(bootstrapAdminEmail)
	builder.WriteString("\nNOPSAI_BOOTSTRAP_ADMIN_PASSWORD=")
	builder.WriteString(secrets.BootstrapAdminPassword)
	builder.WriteString("\n")
	for _, image := range composeImageEnvs {
		builder.WriteString(image.Env)
		builder.WriteString("=")
		builder.WriteString(images[image.Key])
		builder.WriteString("\n")
	}
	return []byte(builder.String())
}

func renderKubernetesValues(version string, images map[string]string, existingSecret, ingressHost string, topology installTopology, bootstrapAdminEmail, bootstrapAdminPasswordSecretKey string) ([]byte, error) {
	var builder strings.Builder
	builder.WriteString("# Generated by nopsai install kubernetes. Edit non-secret values, then deploy with the command printed by the CLI.\n")
	builder.WriteString("global:\n")
	builder.WriteString("  releaseVersion: ")
	builder.WriteString(strconv.Quote(version))
	builder.WriteString("\n  sourceCommit: \"\"\n  environment: production\n  logLevel: info\n  logFormat: json\n  imagePullSecrets: []\n\n")
	builder.WriteString("topology:\n")
	builder.WriteString("  nopsaiAPIURL: ")
	builder.WriteString(strconv.Quote(topology.NopsaiAPIURL))
	builder.WriteString("\n  dispatcherGRPCAddress: ")
	builder.WriteString(strconv.Quote(topology.DispatcherAddress))
	builder.WriteString("\n  aaaAPIURL: ")
	builder.WriteString(strconv.Quote(topology.AAAAPIURL))
	builder.WriteString("\n  gitBotAPIURL: ")
	builder.WriteString(strconv.Quote(topology.GitBotAPIURL))
	builder.WriteString("\n  gotenbergURL: ")
	builder.WriteString(strconv.Quote(topology.GotenbergURL))
	builder.WriteString("\n\nbootstrapAdmin:\n  email: ")
	builder.WriteString(strconv.Quote(bootstrapAdminEmail))
	builder.WriteString("\n\n")
	builder.WriteString("secrets:\n")
	builder.WriteString("  existingSecret: ")
	builder.WriteString(strconv.Quote(existingSecret))
	builder.WriteString("\n  keys:\n")
	builder.WriteString("    databaseURL: database-url\n")
	builder.WriteString("    postgresPassword: postgres-password\n")
	builder.WriteString("    masterKey: master-key\n")
	builder.WriteString("    jwtSigningKey: jwt-signing-key\n")
	builder.WriteString("    serviceJWTSigningKey: service-jwt-signing-key\n")
	builder.WriteString("    aaaSharedInternalToken: aaa-shared-internal-token\n")
	builder.WriteString("    dispatcherTLSSecret: dispatcher-tls-secret\n")
	builder.WriteString("    bootstrapAdminPassword: ")
	builder.WriteString(strconv.Quote(bootstrapAdminPasswordSecretKey))
	builder.WriteString("\n\n")
	builder.WriteString("postgres:\n  enabled: true\n  database: nopsai_db\n  username: nopsai_user\n  image:\n    repository: postgres\n    tag: \"15\"\n    digest: \"\"\n  auth:\n    passwordKey: postgres-password\n  service:\n    name: postgres\n    port: 5432\n  persistence:\n    enabled: true\n    storageClass: \"\"\n    size: 20Gi\n\n")
	builder.WriteString("api:\n  replicaCount: 1\n")
	if err := writeKubernetesImage(&builder, images, "api"); err != nil {
		return nil, err
	}
	builder.WriteString("  service:\n    type: ClusterIP\n    port: 8080\n\n")
	builder.WriteString("aaa:\n  replicaCount: 1\n")
	if err := writeKubernetesImage(&builder, images, "aaa"); err != nil {
		return nil, err
	}
	builder.WriteString("\nagent:\n")
	if err := writeKubernetesImage(&builder, images, "agent"); err != nil {
		return nil, err
	}
	builder.WriteString("\ndispatcher:\n  replicaCount: 1\n")
	if err := writeKubernetesImage(&builder, images, "dispatcher"); err != nil {
		return nil, err
	}
	builder.WriteString("\ngitBot:\n  replicaCount: 1\n")
	if err := writeKubernetesImage(&builder, images, "gitBot"); err != nil {
		return nil, err
	}
	builder.WriteString("\nrunner:\n")
	if err := writeKubernetesImage(&builder, images, "runner"); err != nil {
		return nil, err
	}
	builder.WriteString("\nk8sRunner:\n  enabled: true\n  replicaCount: 1\n  runnerID: k8s-runner-1\n  scopes: \"\"\n  capacity: 10\n  serviceAccount:\n    create: true\n    name: nopsai-runner\n  workspace:\n    size: 10Gi\n    accessMode: ReadWriteOnce\n    volumeMode: pvc\n    storageClass: \"\"\n")
	if err := writeKubernetesImage(&builder, images, "k8sRunner"); err != nil {
		return nil, err
	}
	builder.WriteString("\nsystemLogs:\n  enabled: true\n  provider: kubernetes\n  kubernetes:\n    labelSelector: \"\"\n    container: \"\"\n    rbac:\n      create: true\n\n")
	builder.WriteString("ui:\n  replicaCount: 1\n")
	if err := writeKubernetesImage(&builder, images, "ui"); err != nil {
		return nil, err
	}
	builder.WriteString("  service:\n    type: ClusterIP\n    port: 80\n\n")
	builder.WriteString("ingress:\n")
	if strings.TrimSpace(ingressHost) == "" {
		builder.WriteString("  enabled: false\n  className: \"\"\n  annotations: {}\n  hosts:\n    - host: nopsai.example.com\n      paths:\n        - path: /\n          pathType: Prefix\n  tls: []\n")
	} else {
		builder.WriteString("  enabled: true\n  className: \"\"\n  annotations: {}\n  hosts:\n    - host: ")
		builder.WriteString(strconv.Quote(strings.TrimSpace(ingressHost)))
		builder.WriteString("\n      paths:\n        - path: /\n          pathType: Prefix\n  tls: []\n")
	}
	return []byte(builder.String()), nil
}

func writeKubernetesImage(builder *strings.Builder, images map[string]string, imageKey string) error {
	image, err := requiredInstallImage(images, imageKey)
	if err != nil {
		return err
	}
	repository, tag, digest, err := splitInstallImageReference(image)
	if err != nil {
		return err
	}
	builder.WriteString("  image:\n    repository: ")
	builder.WriteString(strconv.Quote(repository))
	builder.WriteString("\n    tag: ")
	builder.WriteString(strconv.Quote(tag))
	builder.WriteString("\n    digest: ")
	builder.WriteString(strconv.Quote(digest))
	builder.WriteString("\n  imagePullPolicy: IfNotPresent\n")
	return nil
}

func splitInstallImageReference(value string) (repository, tag, digest string, err error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return "", "", "", errors.New("must be a container image reference without whitespace")
	}
	if separator := strings.LastIndex(value, "@"); separator >= 0 {
		if separator == 0 || separator == len(value)-1 {
			return "", "", "", errors.New("digest-pinned image reference is incomplete")
		}
		digest = value[separator+1:]
		if err := compatibility.ValidateDigest(digest); err != nil {
			return "", "", "", fmt.Errorf("invalid image digest: %w", err)
		}
		return value[:separator], "", digest, nil
	}
	lastSlash := strings.LastIndex(value, "/")
	lastColon := strings.LastIndex(value, ":")
	if lastColon <= lastSlash || lastColon == len(value)-1 {
		return "", "", "", errors.New("must include an explicit tag or @sha256 digest")
	}
	tag = value[lastColon+1:]
	if err := validateInstallImageTag(tag); err != nil {
		return "", "", "", err
	}
	return value[:lastColon], tag, "", nil
}

func validateInstallImageTag(tag string) error {
	if tag == "" || len(tag) > 128 {
		return errors.New("image tag must contain 1-128 characters")
	}
	for index, character := range tag {
		valid := character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '_' || character == '.' || character == '-'
		if !valid || (index == 0 && (character == '.' || character == '-')) {
			return errors.New("image tag must use letters, numbers, underscores, periods, or dashes")
		}
	}
	return nil
}

func appendInstallLock(files []InstallFile, lock InstallLock) ([]InstallFile, error) {
	contents, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return nil, err
	}
	contents = append(contents, '\n')
	out := append([]InstallFile(nil), files...)
	out = append(out, InstallFile{RelativePath: installLockFile, Mode: 0o644, Contents: contents})
	return out, nil
}

func installLock(target, version string, cli buildinfo.Info, images map[string]string, files []InstallFile, generatedAt time.Time, chartReference, chartVersion string) InstallLock {
	return InstallLock{
		SchemaVersion:  installSchemaVersion,
		Target:         target,
		Version:        version,
		CLI:            cli.Version,
		ChartReference: strings.TrimSpace(chartReference),
		ChartVersion:   strings.TrimSpace(chartVersion),
		Images:         cloneStrings(images),
		FileHashes:     installFileHashes(files),
		GeneratedAt:    generatedAt.UTC(),
	}
}

func WriteInstallDeploymentLock(path string, lock InstallDeploymentLock) error {
	contents, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("encode install deployment lock: %w", err)
	}
	contents = append(contents, '\n')
	return writeFileAtomic(path, contents, 0o644)
}

func hashInstallValues(files []string, images map[string]string) (string, error) {
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
		_, _ = hash.Write([]byte(fmt.Sprintf("values[%d]", index)))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(contents)
		_, _ = hash.Write([]byte{0})
	}
	names := make([]string, 0, len(images))
	for name := range images {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		_, _ = hash.Write([]byte(name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(images[name]))
		_, _ = hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func installFileHashes(files []InstallFile) map[string]string {
	hashes := map[string]string{}
	for _, file := range files {
		if file.Sensitive {
			continue
		}
		sum := sha256.Sum256(file.Contents)
		hashes[file.RelativePath] = "sha256:" + hex.EncodeToString(sum[:])
	}
	return hashes
}

func installFilePath(outputDir, relativePath string) (string, error) {
	if err := validateInstallRelativePath(relativePath); err != nil {
		return "", err
	}
	return filepath.Join(outputDir, filepath.FromSlash(relativePath)), nil
}

func validateInstallRelativePath(relativePath string) error {
	if strings.TrimSpace(relativePath) == "" {
		return errors.New("install file path is required")
	}
	cleaned := filepath.ToSlash(filepath.Clean(relativePath))
	if filepath.IsAbs(relativePath) || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("install file path %q must stay inside the output directory", relativePath)
	}
	return nil
}

func writeFileAtomic(path string, contents []byte, mode fs.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary %s: %w", path, err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return fmt.Errorf("set permissions on %s: %w", tempPath, err)
	}
	if _, err := temp.Write(contents); err != nil {
		temp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync %s: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("replace %s: %w", path, err)
		}
		if retryErr := os.Rename(tempPath, path); retryErr != nil {
			return fmt.Errorf("replace %s: %w", path, retryErr)
		}
	}
	return nil
}

func validateTCPPort(label, raw string) error {
	port, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%s must be a TCP port from 1 to 65535", label)
	}
	return nil
}

func normalizeInstallTopology(input installTopology, includeDockerNetwork bool) (installTopology, error) {
	topology := installTopology{
		NopsaiAPIURL:      strings.TrimSpace(input.NopsaiAPIURL),
		DispatcherAddress: strings.TrimSpace(input.DispatcherAddress),
		AAAAPIURL:         strings.TrimSpace(input.AAAAPIURL),
		GitBotAPIURL:      strings.TrimSpace(input.GitBotAPIURL),
		GotenbergURL:      strings.TrimSpace(input.GotenbergURL),
		DockerNetworkName: strings.TrimSpace(input.DockerNetworkName),
	}
	if topology.NopsaiAPIURL == "" {
		topology.NopsaiAPIURL = DefaultInstallNopsaiAPIURL
	}
	if topology.DispatcherAddress == "" {
		topology.DispatcherAddress = DefaultInstallDispatcherAddress
	}
	if topology.AAAAPIURL == "" {
		topology.AAAAPIURL = DefaultInstallAAAAPIURL
	}
	if topology.GitBotAPIURL == "" {
		topology.GitBotAPIURL = DefaultInstallGitBotAPIURL
	}
	if topology.GotenbergURL == "" {
		topology.GotenbergURL = DefaultInstallGotenbergURL
	}
	if topology.DockerNetworkName == "" {
		topology.DockerNetworkName = DefaultInstallDockerNetworkName
	}
	for label, value := range map[string]string{
		"nopsai API URL":  topology.NopsaiAPIURL,
		"AAA API URL":     topology.AAAAPIURL,
		"git-bot API URL": topology.GitBotAPIURL,
		"Gotenberg URL":   topology.GotenbergURL,
	} {
		if err := validateInstallHTTPURL(label, value); err != nil {
			return installTopology{}, err
		}
	}
	if err := validateInstallAddress("dispatcher address", topology.DispatcherAddress); err != nil {
		return installTopology{}, err
	}
	if includeDockerNetwork {
		if err := validateInstallToken("Docker network name", topology.DockerNetworkName); err != nil {
			return installTopology{}, err
		}
	}
	return topology, nil
}

func validateInstallHTTPURL(label, raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", label)
	}
	if parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("%s must be an absolute URL without credentials or a fragment", label)
	}
	return nil
}

func validateInstallAddress(label, raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if strings.ContainsAny(value, "\r\n") || strings.Contains(value, "://") {
		return fmt.Errorf("%s must be a host:port service address, not a URL", label)
	}
	return nil
}

func validateInstallToken(label, raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if strings.ContainsAny(value, "\r\n\t ") {
		return fmt.Errorf("%s cannot contain whitespace", label)
	}
	return nil
}

func validateKubernetesSecretKey(label, raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if len(value) > 253 {
		return fmt.Errorf("%s must be no longer than 253 characters", label)
	}
	for _, character := range value {
		valid := character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '.' || character == '_' || character == '-'
		if !valid {
			return fmt.Errorf("%s must use letters, numbers, periods, underscores, or dashes", label)
		}
	}
	return nil
}

func normalizeInstallBootstrapAdminEmail(raw string) (string, error) {
	email := strings.TrimSpace(raw)
	if email == "" {
		email = DefaultInstallBootstrapAdminEmail
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil {
		return "", fmt.Errorf("bootstrap admin email must be valid")
	}
	return strings.TrimSpace(parsed.Address), nil
}

func validateInstallBootstrapAdminPassword(raw string) error {
	if raw != strings.TrimSpace(raw) {
		return errors.New("bootstrap admin password cannot start or end with whitespace")
	}
	if err := validateInstallToken("bootstrap admin password", raw); err != nil {
		return err
	}
	if raw == "admin" {
		return errors.New("bootstrap admin password cannot be the built-in development password; leave it blank to generate a unique first-login password")
	}
	if len([]rune(raw)) < installBootstrapAdminMinPasswordLength {
		return fmt.Errorf("bootstrap admin password must be at least %d characters", installBootstrapAdminMinPasswordLength)
	}
	return nil
}

func composeCommandText(outputDir string) string {
	return "cd " + shellQuote(outputDir) + " && docker compose --env-file .env -f docker-compose.yaml up -d"
}

func kubernetesCommandText(outputDir, releaseName, namespace, valuesFile string, wait bool) string {
	return "cd " + shellQuote(outputDir) + " && " + shellJoin(kubernetesDeployArgs(releaseName, namespace, valuesFile, wait))
}

func kubernetesDeployArgs(releaseName, namespace, valuesFile string, wait bool) []string {
	args := []string{
		"nopsai", "install", "kubernetes",
		"--output-dir", ".",
		"--values-file", valuesFile,
		"--release", releaseName,
		"--namespace", namespace,
		"--deploy",
	}
	if wait {
		args = append(args, "--wait")
	}
	return args
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-:=@", r))
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

const dockerComposeTemplate = `name: {{.ProjectName}}

networks:
  nopsai-net:
    name: ${DOCKER_NETWORK_NAME:-nopsai-net}

volumes:
  db:
    name: nopsai-db

x-service-auth-env: &service-auth-env
  SERVICE_JWT_SIGNING_KEY: ${SERVICE_JWT_SIGNING_KEY:?SERVICE_JWT_SIGNING_KEY is required}
  DISPATCHER_TLS_SECRET: ${DISPATCHER_TLS_SECRET:-}

x-local-topology-env: &local-topology-env
  NOPSAI_API_URL: ${NOPSAI_INTERNAL_API_URL:-http://nopsai:8080}
  DISPATCHER_GRPC_ADDRESS: ${DISPATCHER_GRPC_ADDRESS:-dispatcher:9090}
  DOCKER_NETWORK_NAME: ${DOCKER_NETWORK_NAME:-nopsai-net}

x-observability-env: &observability-env
  NOPSAI_ENV: ${NOPSAI_ENVIRONMENT:-production}
  NOPSAI_ENVIRONMENT: ${NOPSAI_ENVIRONMENT:-production}
  LOG_FORMAT: ${LOG_FORMAT:-json}
  LOG_LEVEL: ${LOG_LEVEL:-info}

services:
  db:
    container_name: nopsai-db
    image: postgres:15
    restart: unless-stopped
    environment:
      POSTGRES_DB: ${POSTGRES_DB:-nopsai_db}
      POSTGRES_USER: ${POSTGRES_USER:-nopsai_user}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?POSTGRES_PASSWORD is required}
    volumes:
      - ./db/init.sql:/docker-entrypoint-initdb.d/init.sql:ro
      - db:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER:-nopsai_user} -d ${POSTGRES_DB:-nopsai_db}"]
      interval: 5s
      timeout: 5s
      retries: 10
    networks: [nopsai-net]

  aaa:
    container_name: nopsai-aaa
    image: ${NOPSAI_AAA_IMAGE:?NOPSAI_AAA_IMAGE is required}
    restart: unless-stopped
    depends_on:
      db:
        condition: service_healthy
    environment:
      <<: *observability-env
      NOPSAI_SERVICE_NAME: aaa
      DATABASE_URL: ${DATABASE_URL:?DATABASE_URL is required}
      AAA_SHARED_INTERNAL_TOKEN: ${AAA_SHARED_INTERNAL_TOKEN:?AAA_SHARED_INTERNAL_TOKEN is required}
    networks: [nopsai-net]

  docker-socket-proxy:
    container_name: nopsai-docker-socket-proxy
    image: ${NOPSAI_DOCKER_SOCKET_PROXY_IMAGE:?NOPSAI_DOCKER_SOCKET_PROXY_IMAGE is required}
    restart: unless-stopped
    environment:
      <<: *observability-env
      NOPSAI_SERVICE_NAME: docker-socket-proxy
      ALLOWED_CONTAINERS: nopsai,nopsai-aaa,nopsai-dispatcher,nopsai-git-bot,nopsai-ui,nopsai-docker-runner,nopsai-k8s-runner,nopsai-docker-socket-proxy,nopsai-gotenberg,nopsai-db
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    read_only: true
    security_opt: [no-new-privileges:true]
    cap_drop: [ALL]
    networks: [nopsai-net]

  gotenberg:
    container_name: nopsai-gotenberg
    image: gotenberg/gotenberg:8.32.0
    restart: unless-stopped
    read_only: true
    tmpfs:
      - /tmp:size=512m,mode=1777
    security_opt: [no-new-privileges:true]
    cap_drop: [ALL]
    healthcheck:
      test: ["CMD", "curl", "--fail", "--silent", "http://localhost:3000/health"]
      interval: 10s
      timeout: 5s
      retries: 10
    networks: [nopsai-net]

  nopsai:
    container_name: nopsai
    image: ${NOPSAI_API_IMAGE:?NOPSAI_API_IMAGE is required}
    restart: unless-stopped
    depends_on:
      db:
        condition: service_healthy
      aaa:
        condition: service_started
      gotenberg:
        condition: service_healthy
      docker-socket-proxy:
        condition: service_started
    ports:
      - "${NOPSAI_API_PORT:-8080}:8080"
    environment:
      <<: [*service-auth-env, *local-topology-env, *observability-env]
      NOPSAI_SERVICE_NAME: nopsai
      DATABASE_URL: ${DATABASE_URL:?DATABASE_URL is required}
      NOPSAI_MASTER_KEY: ${NOPSAI_MASTER_KEY:?NOPSAI_MASTER_KEY is required}
      JWT_SIGNING_KEY: ${JWT_SIGNING_KEY:?JWT_SIGNING_KEY is required}
      NOPSAI_BOOTSTRAP_ADMIN_EMAIL: ${NOPSAI_BOOTSTRAP_ADMIN_EMAIL:?NOPSAI_BOOTSTRAP_ADMIN_EMAIL is required}
      NOPSAI_BOOTSTRAP_ADMIN_PASSWORD: ${NOPSAI_BOOTSTRAP_ADMIN_PASSWORD:?NOPSAI_BOOTSTRAP_ADMIN_PASSWORD is required}
      AAA_API_URL: ${AAA_API_URL:-http://aaa:8082}
      AAA_SHARED_INTERNAL_TOKEN: ${AAA_SHARED_INTERNAL_TOKEN:?AAA_SHARED_INTERNAL_TOKEN is required}
      GIT_BOT_API_URL: ${GIT_BOT_API_URL:-http://git-bot:8081}
      FINAL_OUTPUT_PDF_RENDERER_URL: ${FINAL_OUTPUT_PDF_RENDERER_URL:-http://gotenberg:3000}
      SYSTEM_LOGS_PROVIDER: docker
      SYSTEM_LOGS_DOCKER_HOST: tcp://docker-socket-proxy:2375
      AGENT_IMAGE: ${NOPSAI_AGENT_IMAGE:?NOPSAI_AGENT_IMAGE is required}
    networks: [nopsai-net]

  dispatcher:
    container_name: nopsai-dispatcher
    image: ${NOPSAI_DISPATCHER_IMAGE:?NOPSAI_DISPATCHER_IMAGE is required}
    restart: unless-stopped
    depends_on:
      nopsai:
        condition: service_started
    environment:
      <<: [*service-auth-env, *observability-env]
      NOPSAI_SERVICE_NAME: dispatcher
      NOPSAI_API_URL: ${NOPSAI_INTERNAL_API_URL:-http://nopsai:8080}
    networks: [nopsai-net]

  git-bot:
    container_name: nopsai-git-bot
    image: ${NOPSAI_GIT_BOT_IMAGE:?NOPSAI_GIT_BOT_IMAGE is required}
    restart: unless-stopped
    depends_on:
      nopsai:
        condition: service_started
    environment:
      <<: [*service-auth-env, *observability-env]
      NOPSAI_SERVICE_NAME: git-bot
      GIT_BOT_LISTEN_ADDRESS: 0.0.0.0:8081
      NOPSAI_API_URL: ${NOPSAI_INTERNAL_API_URL:-http://nopsai:8080}
    networks: [nopsai-net]

  ui:
    container_name: nopsai-ui
    image: ${NOPSAI_UI_IMAGE:?NOPSAI_UI_IMAGE is required}
    restart: unless-stopped
    depends_on:
      nopsai:
        condition: service_started
    ports:
      - "${NOPSAI_UI_PORT:-80}:80"
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1/"]
      interval: 30s
      timeout: 5s
      retries: 5
    networks: [nopsai-net]

  docker-runner:
    container_name: nopsai-docker-runner
    hostname: docker-runner
    image: ${NOPSAI_DOCKER_RUNNER_IMAGE:?NOPSAI_DOCKER_RUNNER_IMAGE is required}
    restart: unless-stopped
    depends_on:
      dispatcher:
        condition: service_started
    environment:
      <<: [*service-auth-env, *observability-env]
      NOPSAI_SERVICE_NAME: docker-runner
      DISPATCHER_GRPC_ADDRESS: ${DISPATCHER_GRPC_ADDRESS:-dispatcher:9090}
      DOCKER_NETWORK_NAME: ${DOCKER_NETWORK_NAME:-nopsai-net}
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    networks: [nopsai-net]
`

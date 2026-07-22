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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	dbassets "nopsai/db"
	"nopsai/pkg/buildinfo"
	"nopsai/pkg/compatibility"
)

const (
	DefaultInstallOutputDir         = "nopsai-install"
	DefaultInstallProjectName       = "nopsai"
	DefaultInstallUIPort            = "80"
	DefaultInstallAPIPort           = "8080"
	DefaultInstallPostgresDB        = "nopsai_db"
	DefaultInstallPostgresUser      = "nopsai_user"
	DefaultKubernetesValuesFile     = "values.yaml"
	DefaultKubernetesExistingSecret = "nopsai-secrets"
	installSchemaVersion            = "v1"
	installComposeFile              = "docker-compose.yaml"
	installEnvFile                  = ".env"
	installManifestFile             = "release-manifest.json"
	installLockFile                 = ".nopsai/install.lock"
	installDatabaseBootstrapSQLFile = "db/init.sql"
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
	ManifestSource         string
	ExpectedManifestDigest string
	OutputDir              string
	ProjectName            string
	UIPort                 string
	APIPort                string
}

type KubernetesValuesOptions struct {
	Version                string
	ManifestSource         string
	ExpectedManifestDigest string
	OutputDir              string
	ValuesFile             string
	ReleaseName            string
	Namespace              string
	ExistingSecret         string
	IngressHost            string
	Wait                   bool
}

type InstallPlan struct {
	Target         string
	Version        string
	CLI            string
	OutputDir      string
	ManifestSource string
	ManifestDigest string
	Files          []InstallFile
	Command        string
	Warnings       []string
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
	ManifestSource string            `json:"manifestSource" yaml:"manifestSource"`
	ManifestDigest string            `json:"manifestDigest" yaml:"manifestDigest"`
	Images         map[string]string `json:"images" yaml:"images"`
	FileHashes     map[string]string `json:"fileHashes" yaml:"fileHashes"`
	GeneratedAt    time.Time         `json:"generatedAt" yaml:"generatedAt"`
}

type composeTemplateData struct {
	ProjectName string
}

type composeSecrets struct {
	PostgresPassword       string
	JWTSigningKey          string
	ServiceJWTSigningKey   string
	AAASharedInternalToken string
	MasterKey              string
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
	{Key: "runner", Env: "NOPSAI_RUNNER_IMAGE"},
	{Key: "ui", Env: "NOPSAI_UI_IMAGE"},
}

func (i Installer) PlanDockerCompose(ctx context.Context, options DockerComposeInstallOptions) (InstallPlan, error) {
	resolved, cli, err := i.resolveInstallManifest(ctx, options.Version, options.ManifestSource, options.ExpectedManifestDigest, compatibility.CapabilityPlatformCompose, compatibility.CapabilityRunnerDocker)
	if err != nil {
		return InstallPlan{}, err
	}
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
	for _, image := range composeImageEnvs {
		if _, err := requiredInstallImage(resolved.Manifest, image.Key); err != nil {
			return InstallPlan{}, err
		}
	}
	secrets, err := i.generateComposeSecrets()
	if err != nil {
		return InstallPlan{}, err
	}
	compose, err := renderComposeTemplate(projectName)
	if err != nil {
		return InstallPlan{}, err
	}
	env := renderComposeEnv(resolved.Manifest, secrets, apiPort, uiPort)
	baseFiles := []InstallFile{
		{RelativePath: installComposeFile, Mode: 0o644, Contents: compose},
		{RelativePath: installEnvFile, Mode: 0o600, Sensitive: true, Contents: env},
		{RelativePath: installDatabaseBootstrapSQLFile, Mode: 0o644, Contents: dbassets.InitSQL()},
		{RelativePath: installManifestFile, Mode: 0o644, Contents: resolved.Raw},
	}
	files, err := appendInstallLock(baseFiles, installLock("docker-compose", resolved, cli, baseFiles, i.now()))
	if err != nil {
		return InstallPlan{}, err
	}
	return InstallPlan{
		Target:         "docker-compose",
		Version:        resolved.Manifest.Version,
		CLI:            cli.Version,
		OutputDir:      outputDir,
		ManifestSource: resolved.Source,
		ManifestDigest: resolved.Digest,
		Files:          files,
		Command:        composeCommandText(outputDir),
		Warnings: []string{
			installEnvFile + " contains generated secrets and should stay out of Git.",
			"Rotate generated secrets through your production secret manager before promoting this install beyond evaluation.",
		},
	}, nil
}

func (i Installer) PlanKubernetesValues(ctx context.Context, options KubernetesValuesOptions) (InstallPlan, error) {
	resolved, cli, err := i.resolveInstallManifest(ctx, options.Version, options.ManifestSource, options.ExpectedManifestDigest, compatibility.CapabilityPlatformHelm, compatibility.CapabilityRunnerK8s)
	if err != nil {
		return InstallPlan{}, err
	}
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
	values, err := renderKubernetesValues(resolved.Manifest, existingSecret, options.IngressHost)
	if err != nil {
		return InstallPlan{}, err
	}
	baseFiles := []InstallFile{
		{RelativePath: valuesFile, Mode: 0o644, Contents: values},
		{RelativePath: installManifestFile, Mode: 0o644, Contents: resolved.Raw},
	}
	files, err := appendInstallLock(baseFiles, installLock("kubernetes", resolved, cli, baseFiles, i.now()))
	if err != nil {
		return InstallPlan{}, err
	}
	return InstallPlan{
		Target:         "kubernetes",
		Version:        resolved.Manifest.Version,
		CLI:            cli.Version,
		OutputDir:      outputDir,
		ManifestSource: resolved.Source,
		ManifestDigest: resolved.Digest,
		Files:          files,
		Command:        kubernetesCommandText(outputDir, releaseName, namespace, valuesFile, options.Wait),
		Warnings: []string{
			"values.yaml references an existing Kubernetes Secret; create it with your cluster secret manager before deploying.",
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

func (i Installer) resolveInstallManifest(ctx context.Context, version, source, digest string, requiredCapabilities ...string) (ResolvedManifest, buildinfo.Info, error) {
	cli := i.CLI
	if strings.TrimSpace(cli.Version) == "" {
		cli = buildinfo.Current()
	}
	resolved, err := i.Resolver.Resolve(ctx, version, source, defaultReleaseManifestDigest(source, digest, cli))
	if err != nil {
		return ResolvedManifest{}, buildinfo.Info{}, err
	}
	if err := compatibility.ValidateManifestForCLI(resolved.Manifest, cli); err != nil {
		return ResolvedManifest{}, buildinfo.Info{}, fmt.Errorf("release compatibility check failed: %w", err)
	}
	if err := compatibility.RequireCapabilities(resolved.Manifest.Capabilities, requiredCapabilities...); err != nil {
		return ResolvedManifest{}, buildinfo.Info{}, err
	}
	return resolved, cli, nil
}

func defaultReleaseManifestDigest(source, digest string, cli buildinfo.Info) string {
	if strings.TrimSpace(source) != "" || strings.TrimSpace(digest) != "" {
		return digest
	}
	return strings.TrimSpace(cli.ReleaseManifestDigest)
}

func (i Installer) generateComposeSecrets() (composeSecrets, error) {
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
	masterKey, err := generateInstallSecret(reader, 32)
	if err != nil {
		return composeSecrets{}, err
	}
	return composeSecrets{
		PostgresPassword:       postgresPassword,
		JWTSigningKey:          jwtSigningKey,
		ServiceJWTSigningKey:   serviceJWTSigningKey,
		AAASharedInternalToken: aaaSharedInternalToken,
		MasterKey:              masterKey,
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

func requiredInstallImage(manifest compatibility.Manifest, key string) (string, error) {
	value := strings.TrimSpace(manifest.Images[key])
	if value == "" {
		return "", fmt.Errorf("release manifest is missing image %q", key)
	}
	if err := compatibility.ValidateImageReference(value); err != nil {
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

func renderComposeEnv(manifest compatibility.Manifest, secrets composeSecrets, apiPort, uiPort string) []byte {
	var builder strings.Builder
	builder.WriteString("NOPSAI_VERSION=")
	builder.WriteString(manifest.Version)
	builder.WriteString("\nNOPSAI_ENVIRONMENT=production\nLOG_FORMAT=json\nLOG_LEVEL=info\n")
	builder.WriteString("NOPSAI_API_PORT=")
	builder.WriteString(apiPort)
	builder.WriteString("\nNOPSAI_UI_PORT=")
	builder.WriteString(uiPort)
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
	builder.WriteString("\nNOPSAI_MASTER_KEY=")
	builder.WriteString(secrets.MasterKey)
	builder.WriteString("\n")
	for _, image := range composeImageEnvs {
		builder.WriteString(image.Env)
		builder.WriteString("=")
		builder.WriteString(manifest.Images[image.Key])
		builder.WriteString("\n")
	}
	return []byte(builder.String())
}

func renderKubernetesValues(manifest compatibility.Manifest, existingSecret, ingressHost string) ([]byte, error) {
	var builder strings.Builder
	builder.WriteString("# Generated by nopsai install kubernetes. Edit non-secret values, then deploy with the command printed by the CLI.\n")
	builder.WriteString("global:\n")
	builder.WriteString("  releaseVersion: ")
	builder.WriteString(strconv.Quote(manifest.Version))
	builder.WriteString("\n  sourceCommit: \"\"\n  environment: production\n  logLevel: info\n  logFormat: json\n  imagePullSecrets: []\n\n")
	builder.WriteString("secrets:\n")
	builder.WriteString("  existingSecret: ")
	builder.WriteString(strconv.Quote(existingSecret))
	builder.WriteString("\n  keys:\n")
	builder.WriteString("    databaseURL: database-url\n")
	builder.WriteString("    masterKey: master-key\n")
	builder.WriteString("    jwtSigningKey: jwt-signing-key\n")
	builder.WriteString("    serviceJWTSigningKey: service-jwt-signing-key\n")
	builder.WriteString("    aaaSharedInternalToken: aaa-shared-internal-token\n\n")
	builder.WriteString("api:\n  replicaCount: 1\n")
	if err := writeKubernetesImage(&builder, manifest, "api"); err != nil {
		return nil, err
	}
	builder.WriteString("  service:\n    type: ClusterIP\n    port: 8080\n\n")
	builder.WriteString("aaa:\n  replicaCount: 1\n")
	if err := writeKubernetesImage(&builder, manifest, "aaa"); err != nil {
		return nil, err
	}
	builder.WriteString("\nagent:\n")
	if err := writeKubernetesImage(&builder, manifest, "agent"); err != nil {
		return nil, err
	}
	builder.WriteString("\ndispatcher:\n  replicaCount: 1\n")
	if err := writeKubernetesImage(&builder, manifest, "dispatcher"); err != nil {
		return nil, err
	}
	builder.WriteString("\ngitBot:\n  replicaCount: 1\n")
	if err := writeKubernetesImage(&builder, manifest, "gitBot"); err != nil {
		return nil, err
	}
	builder.WriteString("\nrunner:\n")
	if err := writeKubernetesImage(&builder, manifest, "runner"); err != nil {
		return nil, err
	}
	builder.WriteString("\nk8sRunner:\n  enabled: true\n  replicaCount: 1\n  runnerID: k8s-runner-1\n  scopes: \"\"\n  capacity: 10\n  serviceAccount:\n    create: true\n    name: nopsai-runner\n  workspace:\n    size: 10Gi\n    accessMode: ReadWriteOnce\n    volumeMode: pvc\n    storageClass: \"\"\n")
	if err := writeKubernetesImage(&builder, manifest, "k8sRunner"); err != nil {
		return nil, err
	}
	builder.WriteString("\ndockerSocketProxy:\n")
	if err := writeKubernetesImage(&builder, manifest, "dockerSocketProxy"); err != nil {
		return nil, err
	}
	builder.WriteString("\nsystemLogs:\n  enabled: true\n  provider: kubernetes\n  kubernetes:\n    labelSelector: \"\"\n    container: \"\"\n    rbac:\n      create: true\n\n")
	builder.WriteString("ui:\n  replicaCount: 1\n")
	if err := writeKubernetesImage(&builder, manifest, "ui"); err != nil {
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

func writeKubernetesImage(builder *strings.Builder, manifest compatibility.Manifest, imageKey string) error {
	image, err := requiredInstallImage(manifest, imageKey)
	if err != nil {
		return err
	}
	repository, digest, err := compatibility.SplitImageReference(image)
	if err != nil {
		return err
	}
	builder.WriteString("  image:\n    repository: ")
	builder.WriteString(strconv.Quote(repository))
	builder.WriteString("\n    tag: \"\"\n    digest: ")
	builder.WriteString(strconv.Quote(digest))
	builder.WriteString("\n  imagePullPolicy: IfNotPresent\n")
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

func installLock(target string, resolved ResolvedManifest, cli buildinfo.Info, files []InstallFile, generatedAt time.Time) InstallLock {
	return InstallLock{
		SchemaVersion:  installSchemaVersion,
		Target:         target,
		Version:        resolved.Manifest.Version,
		CLI:            cli.Version,
		ManifestSource: resolved.Source,
		ManifestDigest: resolved.Digest,
		Images:         cloneStrings(resolved.Manifest.Images),
		FileHashes:     installFileHashes(files),
		GeneratedAt:    generatedAt.UTC(),
	}
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

func composeCommandText(outputDir string) string {
	return "cd " + shellQuote(outputDir) + " && docker compose --env-file .env -f docker-compose.yaml up -d"
}

func kubernetesCommandText(outputDir, releaseName, namespace, valuesFile string, wait bool) string {
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
	return "cd " + shellQuote(outputDir) + " && " + shellJoin(args)
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
    name: nopsai-net

volumes:
  db:
    name: nopsai-db

x-service-auth-env: &service-auth-env
  SERVICE_JWT_SIGNING_KEY: ${SERVICE_JWT_SIGNING_KEY:?SERVICE_JWT_SIGNING_KEY is required}

x-local-topology-env: &local-topology-env
  NOPSAI_API_URL: http://nopsai:8080
  DISPATCHER_GRPC_ADDRESS: dispatcher:9090
  DOCKER_NETWORK_NAME: nopsai-net

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
      AAA_API_URL: http://aaa:8082
      AAA_SHARED_INTERNAL_TOKEN: ${AAA_SHARED_INTERNAL_TOKEN:?AAA_SHARED_INTERNAL_TOKEN is required}
      GIT_BOT_API_URL: http://git-bot:8081
      FINAL_OUTPUT_PDF_RENDERER_URL: http://gotenberg:3000
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
      NOPSAI_API_URL: http://nopsai:8080
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
      NOPSAI_API_URL: http://nopsai:8080
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
    image: ${NOPSAI_RUNNER_IMAGE:?NOPSAI_RUNNER_IMAGE is required}
    restart: unless-stopped
    depends_on:
      dispatcher:
        condition: service_started
    environment:
      <<: [*service-auth-env, *observability-env]
      NOPSAI_SERVICE_NAME: docker-runner
      DISPATCHER_GRPC_ADDRESS: dispatcher:9090
      DOCKER_NETWORK_NAME: nopsai-net
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    networks: [nopsai-net]
`

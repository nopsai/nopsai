package command

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"nopsai/internal/cli/interactive"
	"nopsai/internal/cli/platform"
	"nopsai/pkg/compatibility"

	"github.com/spf13/cobra"
)

type installDockerComposeOptions struct {
	version        string
	manifest       string
	manifestDigest string
	outputDir      string
	projectName    string
	apiPort        string
	uiPort         string
	force          bool
	run            bool
	interactive    bool
}

type installKubernetesOptions struct {
	version        string
	manifest       string
	manifestDigest string
	outputDir      string
	valuesFile     string
	values         []string
	releaseName    string
	namespace      string
	existingSecret string
	ingressHost    string
	lockFile       string
	force          bool
	deploy         bool
	wait           bool
	interactive    bool
}

const installReleaseManifestFile = "release-manifest.json"

func newInstallCommand(root *rootOptions) *cobra.Command {
	command := &cobra.Command{
		Use:   "install",
		Short: "Start the NopsAI installation wizard",
		Long:  "Start an interactive installation wizard. The wizard selects Docker Compose or Kubernetes, resolves the matching release from the CLI version, generates the required files, and can run the install command for the selected target.",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runInstallWizard(command, root)
		},
	}
	command.AddCommand(newInstallDockerComposeCommand(root))
	command.AddCommand(newInstallKubernetesCommand(root))
	return command
}

func runInstallWizard(command *cobra.Command, root *rootOptions) error {
	prompter := interactive.NewPrompter(command.InOrStdin(), command.OutOrStdout())
	selected, err := prompter.Choose("Install target", []interactive.Choice{
		{Label: "docker-compose", Description: "Generate Docker Compose files and optionally start the stack", SearchText: "docker compose local single host"},
		{Label: "kubernetes", Description: "Generate editable Helm values and optionally deploy", SearchText: "kubernetes k8s helm cluster gitops"},
	})
	if err != nil {
		return err
	}
	switch selected {
	case 0:
		options := &installDockerComposeOptions{
			version:     defaultPlatformVersion(root),
			outputDir:   platform.DefaultInstallOutputDir,
			projectName: platform.DefaultInstallProjectName,
			apiPort:     platform.DefaultInstallAPIPort,
			uiPort:      platform.DefaultInstallUIPort,
		}
		if err := resolveInteractiveDockerComposeInstall(prompter, options, defaultPlatformVersion(root)); err != nil {
			return err
		}
		return executeInstallDockerCompose(command, root, options)
	case 1:
		options := &installKubernetesOptions{
			version:        defaultPlatformVersion(root),
			outputDir:      platform.DefaultInstallOutputDir,
			valuesFile:     platform.DefaultKubernetesValuesFile,
			releaseName:    platform.DefaultReleaseName,
			namespace:      platform.DefaultNamespace,
			existingSecret: platform.DefaultKubernetesExistingSecret,
		}
		if err := resolveInteractiveKubernetesInstall(prompter, options, defaultPlatformVersion(root)); err != nil {
			return err
		}
		return executeInstallKubernetes(command, root, options, false)
	default:
		return fmt.Errorf("unsupported install target")
	}
}

func newInstallDockerComposeCommand(root *rootOptions) *cobra.Command {
	options := &installDockerComposeOptions{
		version:     defaultPlatformVersion(root),
		outputDir:   platform.DefaultInstallOutputDir,
		projectName: platform.DefaultInstallProjectName,
		apiPort:     platform.DefaultInstallAPIPort,
		uiPort:      platform.DefaultInstallUIPort,
	}
	command := &cobra.Command{
		Use:     "docker-compose",
		Aliases: []string{"compose", "docker"},
		Short:   "Generate Docker Compose install files and optionally start them",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			var prompter *interactive.Prompter
			if options.interactive || strings.TrimSpace(options.version) == "" {
				prompter = interactive.NewPrompter(command.InOrStdin(), command.OutOrStdout())
				if err := resolveInteractiveDockerComposeInstall(prompter, options, defaultPlatformVersion(root)); err != nil {
					return err
				}
			}
			return executeInstallDockerCompose(command, root, options)
		},
	}
	command.Flags().StringVar(&options.version, "version", options.version, "exact semantic NopsAI version to install; defaults to this CLI build version")
	command.Flags().StringVar(&options.manifest, "manifest", "", "release manifest source as a local file path or trusted HTTPS URL")
	command.Flags().StringVar(&options.manifestDigest, "manifest-digest", "", "expected SHA-256 digest for the release manifest bytes")
	command.Flags().StringVar(&options.outputDir, "output-dir", options.outputDir, "directory where generated install files are stored")
	command.Flags().StringVar(&options.projectName, "project", options.projectName, "Docker Compose project name")
	command.Flags().StringVar(&options.apiPort, "api-port", options.apiPort, "host TCP port for the NopsAI API")
	command.Flags().StringVar(&options.uiPort, "ui-port", options.uiPort, "host TCP port for the NopsAI UI")
	command.Flags().BoolVar(&options.force, "force", false, "replace previously generated install files in the output directory")
	command.Flags().BoolVar(&options.run, "run", false, "run docker compose up -d after writing generated files")
	command.Flags().BoolVar(&options.interactive, "interactive", false, "prompt for version, output directory, ports, overwrite, and run")
	return command
}

func newInstallKubernetesCommand(root *rootOptions) *cobra.Command {
	options := &installKubernetesOptions{
		version:        defaultPlatformVersion(root),
		outputDir:      platform.DefaultInstallOutputDir,
		valuesFile:     platform.DefaultKubernetesValuesFile,
		releaseName:    platform.DefaultReleaseName,
		namespace:      platform.DefaultNamespace,
		existingSecret: platform.DefaultKubernetesExistingSecret,
	}
	command := &cobra.Command{
		Use:     "kubernetes",
		Aliases: []string{"k8s"},
		Short:   "Generate Kubernetes values and optionally deploy with Helm",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			var prompter *interactive.Prompter
			if options.interactive || strings.TrimSpace(options.version) == "" {
				prompter = interactive.NewPrompter(command.InOrStdin(), command.OutOrStdout())
				if err := resolveInteractiveKubernetesInstall(prompter, options, defaultPlatformVersion(root)); err != nil {
					return err
				}
			}
			return executeInstallKubernetes(command, root, options, command.Flags().Changed("version"))
		},
	}
	command.Flags().StringVar(&options.version, "version", options.version, "exact semantic NopsAI version to install; defaults to this CLI build version")
	command.Flags().StringVar(&options.manifest, "manifest", "", "release manifest source as a local file path or trusted HTTPS URL")
	command.Flags().StringVar(&options.manifestDigest, "manifest-digest", "", "expected SHA-256 digest for the release manifest bytes")
	command.Flags().StringVar(&options.outputDir, "output-dir", options.outputDir, "directory where generated values and release metadata are stored")
	command.Flags().StringVar(&options.valuesFile, "values-file", options.valuesFile, "generated Kubernetes values file path relative to output-dir")
	command.Flags().StringArrayVarP(&options.values, "values", "f", nil, "additional Helm values file to merge after the generated sample; repeat in GitOps order")
	command.Flags().StringVar(&options.releaseName, "release", options.releaseName, "Helm release name to install or upgrade")
	command.Flags().StringVar(&options.namespace, "namespace", options.namespace, "Kubernetes namespace for all rendered and deployed resources")
	command.Flags().StringVar(&options.existingSecret, "existing-secret", options.existingSecret, "Kubernetes Secret name referenced by generated values")
	command.Flags().StringVar(&options.ingressHost, "ingress-host", "", "optional ingress host to enable in generated values")
	command.Flags().StringVar(&options.lockFile, "lock-file", "", "GitOps-tracked release lock path written after successful deployment (default: output-dir/.nopsai/release.lock)")
	command.Flags().BoolVar(&options.force, "force", false, "replace previously generated install files in the output directory")
	command.Flags().BoolVar(&options.deploy, "deploy", false, "run Helm upgrade --install after writing generated values")
	command.Flags().BoolVar(&options.wait, "wait", false, "wait for Kubernetes resources to become ready before writing the release lock")
	command.Flags().BoolVar(&options.interactive, "interactive", false, "prompt for version, values, namespace, secret, overwrite, and deployment")
	return command
}

func executeInstallDockerCompose(command *cobra.Command, root *rootOptions, options *installDockerComposeOptions) error {
	if strings.TrimSpace(options.version) == "" {
		return fmt.Errorf("--version is required when this CLI build does not embed a release version")
	}
	installer := installPlanner(root, command)
	plan, err := installer.PlanDockerCompose(command.Context(), platform.DockerComposeInstallOptions{
		Version:                options.version,
		ManifestSource:         options.manifest,
		ExpectedManifestDigest: options.manifestDigest,
		OutputDir:              options.outputDir,
		ProjectName:            options.projectName,
		APIPort:                options.apiPort,
		UIPort:                 options.uiPort,
	})
	if err != nil {
		return err
	}
	if err := platform.WriteInstallPlan(plan, options.force); err != nil {
		return err
	}
	if options.run {
		if err := installer.RunDockerCompose(command.Context(), plan); err != nil {
			return err
		}
	}
	return renderInstallPlan(command, plan, options.run)
}

func executeInstallKubernetes(command *cobra.Command, root *rootOptions, options *installKubernetesOptions, versionExplicit bool) error {
	if options.deploy && !options.force && strings.TrimSpace(options.manifest) == "" && strings.TrimSpace(options.manifestDigest) == "" {
		deployed, err := deployExistingKubernetesInstall(command, root, options, versionExplicit)
		if deployed || err != nil {
			return err
		}
	}
	if strings.TrimSpace(options.version) == "" {
		return fmt.Errorf("--version is required when this CLI build does not embed a release version")
	}
	installer := installPlanner(root, command)
	plan, err := installer.PlanKubernetesValues(command.Context(), platform.KubernetesValuesOptions{
		Version:                options.version,
		ManifestSource:         options.manifest,
		ExpectedManifestDigest: options.manifestDigest,
		OutputDir:              options.outputDir,
		ValuesFile:             options.valuesFile,
		ReleaseName:            options.releaseName,
		Namespace:              options.namespace,
		ExistingSecret:         options.existingSecret,
		IngressHost:            options.ingressHost,
		Wait:                   options.wait,
	})
	if err != nil {
		return err
	}
	if err := platform.WriteInstallPlan(plan, options.force); err != nil {
		return err
	}
	if err := renderInstallPlan(command, plan, false); err != nil {
		return err
	}
	if !options.deploy {
		return nil
	}
	valuesFiles := []string{filepath.Join(plan.OutputDir, installPlanValuesFile(plan, options.valuesFile))}
	valuesFiles = append(valuesFiles, options.values...)
	lockFile := strings.TrimSpace(options.lockFile)
	if lockFile == "" {
		lockFile = filepath.Join(plan.OutputDir, platform.DefaultLockFile)
	}
	deployer := kubernetesDeployer(root, command)
	deploymentPlan, _, _, err := deployer.PlanAndDeploy(command.Context(), platform.KubernetesOptions{
		Version:                options.version,
		ManifestSource:         filepath.Join(plan.OutputDir, installReleaseManifestFile),
		ExpectedManifestDigest: plan.ManifestDigest,
		ValuesFiles:            valuesFiles,
		ReleaseName:            options.releaseName,
		Namespace:              options.namespace,
		Wait:                   options.wait,
		LockFile:               lockFile,
	}, nil)
	if err != nil {
		return err
	}
	return renderDeploymentPlan(command, deploymentPlan, "text", true, lockFile)
}

func deployExistingKubernetesInstall(command *cobra.Command, root *rootOptions, options *installKubernetesOptions, versionExplicit bool) (bool, error) {
	outputDir := installCommandOutputDir(options.outputDir)
	valuesFile, err := cleanInstallValuesFile(options.valuesFile)
	if err != nil {
		return true, err
	}
	manifestPath := filepath.Join(outputDir, installReleaseManifestFile)
	valuesPath := filepath.Join(outputDir, valuesFile)
	manifestExists, err := regularFileExists(manifestPath)
	if err != nil {
		return true, err
	}
	valuesExists, err := regularFileExists(valuesPath)
	if err != nil {
		return true, err
	}
	if !manifestExists && !valuesExists {
		return false, nil
	}
	if !manifestExists || !valuesExists {
		return true, fmt.Errorf("stored Kubernetes install in %s is incomplete; expected %s and %s", outputDir, installReleaseManifestFile, valuesFile)
	}
	rawManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return true, fmt.Errorf("read stored release manifest: %w", err)
	}
	manifest, err := compatibility.DecodeManifest(bytes.NewReader(rawManifest))
	if err != nil {
		return true, err
	}
	version := manifest.Version
	if versionExplicit {
		requested, err := compatibility.ParseVersion(options.version)
		if err != nil {
			return true, fmt.Errorf("invalid requested release version: %w", err)
		}
		if requested.String() != manifest.Version {
			return true, fmt.Errorf("stored Kubernetes install is for NopsAI %s, not requested version %s", manifest.Version, requested.String())
		}
		version = requested.String()
	}
	lockFile := strings.TrimSpace(options.lockFile)
	if lockFile == "" {
		lockFile = filepath.Join(outputDir, platform.DefaultLockFile)
	}
	valuesFiles := []string{valuesPath}
	valuesFiles = append(valuesFiles, options.values...)
	deployer := kubernetesDeployer(root, command)
	deploymentPlan, _, _, err := deployer.PlanAndDeploy(command.Context(), platform.KubernetesOptions{
		Version:                version,
		ManifestSource:         manifestPath,
		ExpectedManifestDigest: compatibility.DigestBytes(rawManifest),
		ValuesFiles:            valuesFiles,
		ReleaseName:            options.releaseName,
		Namespace:              options.namespace,
		Wait:                   options.wait,
		LockFile:               lockFile,
	}, nil)
	if err != nil {
		return true, err
	}
	if _, err := fmt.Fprintf(command.OutOrStdout(), "Using generated NopsAI %s kubernetes install in %s\n", manifest.Version, outputDir); err != nil {
		return true, err
	}
	return true, renderDeploymentPlan(command, deploymentPlan, "text", true, lockFile)
}

func installCommandOutputDir(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return platform.DefaultInstallOutputDir
	}
	return value
}

func cleanInstallValuesFile(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = platform.DefaultKubernetesValuesFile
	}
	cleaned := filepath.ToSlash(filepath.Clean(value))
	if filepath.IsAbs(value) || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("values file %q must stay inside the output directory", value)
	}
	return filepath.FromSlash(cleaned), nil
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return false, fmt.Errorf("%s is a directory", path)
		}
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("inspect %s: %w", path, err)
}

func resolveInteractiveDockerComposeInstall(prompter *interactive.Prompter, options *installDockerComposeOptions, defaultVersion string) error {
	if strings.TrimSpace(options.version) == "" {
		version, err := prompter.AskRequired("NopsAI version", strings.TrimSpace(defaultVersion))
		if err != nil {
			return err
		}
		options.version = strings.TrimSpace(version)
	}
	outputDir, err := prompter.AskRequired("Install output directory", valueOrDefault(options.outputDir, platform.DefaultInstallOutputDir))
	if err != nil {
		return err
	}
	options.outputDir = strings.TrimSpace(outputDir)
	project, err := prompter.AskRequired("Docker Compose project", valueOrDefault(options.projectName, platform.DefaultInstallProjectName))
	if err != nil {
		return err
	}
	options.projectName = strings.TrimSpace(project)
	apiPort, err := prompter.AskRequired("API host port", valueOrDefault(options.apiPort, platform.DefaultInstallAPIPort))
	if err != nil {
		return err
	}
	options.apiPort = strings.TrimSpace(apiPort)
	uiPort, err := prompter.AskRequired("UI host port", valueOrDefault(options.uiPort, platform.DefaultInstallUIPort))
	if err != nil {
		return err
	}
	options.uiPort = strings.TrimSpace(uiPort)
	force, err := prompter.Confirm("Replace existing generated files", options.force)
	if err != nil {
		return err
	}
	options.force = force
	run, err := prompter.Confirm("Run Docker Compose after generating files", options.run)
	if err != nil {
		return err
	}
	options.run = run
	return nil
}

func resolveInteractiveKubernetesInstall(prompter *interactive.Prompter, options *installKubernetesOptions, defaultVersion string) error {
	if strings.TrimSpace(options.version) == "" {
		version, err := prompter.AskRequired("NopsAI version", strings.TrimSpace(defaultVersion))
		if err != nil {
			return err
		}
		options.version = strings.TrimSpace(version)
	}
	outputDir, err := prompter.AskRequired("Install output directory", valueOrDefault(options.outputDir, platform.DefaultInstallOutputDir))
	if err != nil {
		return err
	}
	options.outputDir = strings.TrimSpace(outputDir)
	valuesFile, err := prompter.AskRequired("Generated values file", valueOrDefault(options.valuesFile, platform.DefaultKubernetesValuesFile))
	if err != nil {
		return err
	}
	options.valuesFile = strings.TrimSpace(valuesFile)
	releaseName, err := prompter.AskRequired("Helm release name", valueOrDefault(options.releaseName, platform.DefaultReleaseName))
	if err != nil {
		return err
	}
	options.releaseName = strings.TrimSpace(releaseName)
	namespace, err := prompter.AskRequired("Kubernetes namespace", valueOrDefault(options.namespace, platform.DefaultNamespace))
	if err != nil {
		return err
	}
	options.namespace = strings.TrimSpace(namespace)
	secret, err := prompter.AskRequired("Existing Secret name", valueOrDefault(options.existingSecret, platform.DefaultKubernetesExistingSecret))
	if err != nil {
		return err
	}
	options.existingSecret = strings.TrimSpace(secret)
	ingressHost, err := prompter.Ask("Ingress host (blank disables ingress)", options.ingressHost)
	if err != nil {
		return err
	}
	options.ingressHost = strings.TrimSpace(ingressHost)
	force, err := prompter.Confirm("Replace existing generated files", options.force)
	if err != nil {
		return err
	}
	options.force = force
	deploy, err := prompter.Confirm("Deploy with Helm after generating values", options.deploy)
	if err != nil {
		return err
	}
	options.deploy = deploy
	if deploy {
		wait, err := prompter.Confirm("Wait for resources to become ready", options.wait)
		if err != nil {
			return err
		}
		options.wait = wait
		lockFile, err := prompter.Ask("Release lock file", options.lockFile)
		if err != nil {
			return err
		}
		options.lockFile = strings.TrimSpace(lockFile)
	}
	return nil
}

func installPlanner(root *rootOptions, command *cobra.Command) platform.Installer {
	httpClient := root.dependencies.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: root.timeout}
	}
	return platform.Installer{
		Resolver:     releaseManifestResolver(root, httpClient),
		Runner:       root.dependencies.RunProcess,
		CLI:          root.dependencies.BuildInfo,
		RandomReader: root.dependencies.Random,
		Stderr:       command.ErrOrStderr(),
	}
}

func renderInstallPlan(command *cobra.Command, plan platform.InstallPlan, executed bool) error {
	if _, err := fmt.Fprintf(command.OutOrStdout(), "Generated NopsAI %s %s install in %s\n", plan.Version, plan.Target, plan.OutputDir); err != nil {
		return err
	}
	files := append([]platform.InstallFile(nil), plan.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].RelativePath < files[j].RelativePath })
	for _, file := range files {
		label := file.RelativePath
		if file.Sensitive {
			label += " (sensitive, 0600)"
		}
		if _, err := fmt.Fprintf(command.OutOrStdout(), "  - %s\n", label); err != nil {
			return err
		}
	}
	if plan.Command != "" {
		if executed {
			if _, err := fmt.Fprintf(command.OutOrStdout(), "Executed: %s\n", plan.Command); err != nil {
				return err
			}
		} else if _, err := fmt.Fprintf(command.OutOrStdout(), "Next: %s\n", plan.Command); err != nil {
			return err
		}
	}
	for _, warning := range plan.Warnings {
		if _, err := fmt.Fprintf(command.OutOrStdout(), "Warning: %s\n", warning); err != nil {
			return err
		}
	}
	return nil
}

func installPlanValuesFile(plan platform.InstallPlan, configured string) string {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		return filepath.FromSlash(filepath.Clean(configured))
	}
	for _, file := range plan.Files {
		if strings.HasSuffix(file.RelativePath, ".yaml") || strings.HasSuffix(file.RelativePath, ".yml") {
			return filepath.FromSlash(file.RelativePath)
		}
	}
	return platform.DefaultKubernetesValuesFile
}

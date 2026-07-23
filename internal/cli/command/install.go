package command

import (
	"errors"
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
	"gopkg.in/yaml.v3"
)

type installDockerComposeOptions struct {
	version                string
	outputDir              string
	projectName            string
	apiPort                string
	uiPort                 string
	nopsaiAPIURL           string
	dispatcherAddr         string
	aaaAPIURL              string
	gitBotAPIURL           string
	gotenbergURL           string
	dockerNetwork          string
	bootstrapAdminEmail    string
	bootstrapAdminPassword string
	force                  bool
	run                    bool
	interactive            bool
}

type installKubernetesOptions struct {
	version                         string
	outputDir                       string
	valuesFile                      string
	values                          []string
	releaseName                     string
	namespace                       string
	existingSecret                  string
	ingressHost                     string
	nopsaiAPIURL                    string
	dispatcherAddr                  string
	aaaAPIURL                       string
	gitBotAPIURL                    string
	gotenbergURL                    string
	bootstrapAdminEmail             string
	bootstrapAdminPasswordSecretKey string
	lockFile                        string
	force                           bool
	deploy                          bool
	wait                            bool
	interactive                     bool
}

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
	choices := []interactive.Choice{
		{Label: "docker-compose", Description: "Generate Docker Compose files and optionally start the stack", SearchText: "docker compose local single host"},
		{Label: "kubernetes", Description: "Generate editable Helm values and optionally deploy", SearchText: "kubernetes k8s helm cluster gitops"},
	}
	for {
		var (
			selected int
			err      error
		)
		if prompter.CanUseLiveSelector() {
			selected, err = prompter.ChooseScreen("Install target", choices, installTargetScreenOptions(root))
		} else {
			selected, err = prompter.Choose("Install target", choices)
		}
		if errors.Is(err, interactive.ErrBack) {
			return nil
		}
		if err != nil {
			return err
		}
		switch selected {
		case 0:
			options := defaultDockerComposeInstallOptions(root)
			if err := resolveInteractiveDockerComposeInstall(prompter, options, defaultPlatformVersion(root)); err != nil {
				if errors.Is(err, interactive.ErrBack) {
					continue
				}
				return err
			}
			if err := previewInteractiveDockerComposeInstall(prompter, options); err != nil {
				if errors.Is(err, interactive.ErrBack) {
					continue
				}
				return err
			}
			if prompter.CanUseLiveSelector() {
				stdout, stderr, runErr := captureCommandOutput(command, func() error {
					return executeInstallDockerCompose(command, root, options)
				})
				resultErr := showInteractiveOutput(prompter, "Docker Compose install", stdout, stderr, runErr, installResultScreenOptions("Docker Compose", root))
				if errors.Is(resultErr, interactive.ErrBack) {
					continue
				}
				return resultErr
			}
			return executeInstallDockerCompose(command, root, options)
		case 1:
			options := defaultKubernetesInstallOptions(root)
			if err := resolveInteractiveKubernetesInstall(prompter, options, defaultPlatformVersion(root)); err != nil {
				if errors.Is(err, interactive.ErrBack) {
					continue
				}
				return err
			}
			if err := previewInteractiveKubernetesInstall(prompter, options); err != nil {
				if errors.Is(err, interactive.ErrBack) {
					continue
				}
				return err
			}
			if prompter.CanUseLiveSelector() {
				stdout, stderr, runErr := captureCommandOutput(command, func() error {
					return executeInstallKubernetes(command, root, options, false)
				})
				resultErr := showInteractiveOutput(prompter, "Kubernetes install", stdout, stderr, runErr, installResultScreenOptions("Kubernetes", root))
				if errors.Is(resultErr, interactive.ErrBack) {
					continue
				}
				return resultErr
			}
			return executeInstallKubernetes(command, root, options, false)
		default:
			return fmt.Errorf("unsupported install target")
		}
	}
}

func newInstallDockerComposeCommand(root *rootOptions) *cobra.Command {
	options := defaultDockerComposeInstallOptions(root)
	command := &cobra.Command{
		Use:     "docker-compose",
		Aliases: []string{"compose", "docker"},
		Short:   "Generate Docker Compose install files and optionally start them",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			var prompter *interactive.Prompter
			if options.interactive {
				prompter = interactive.NewPrompter(command.InOrStdin(), command.OutOrStdout())
				if err := resolveInteractiveDockerComposeInstall(prompter, options, defaultPlatformVersion(root)); err != nil {
					return err
				}
				if err := previewInteractiveDockerComposeInstall(prompter, options); err != nil {
					if errors.Is(err, interactive.ErrBack) {
						return renderInstallPreviewCancelled(command)
					}
					return err
				}
				if prompter.CanUseLiveSelector() {
					stdout, stderr, runErr := captureCommandOutput(command, func() error {
						return executeInstallDockerCompose(command, root, options)
					})
					resultErr := showInteractiveOutput(prompter, "Docker Compose install", stdout, stderr, runErr, installResultScreenOptions("Docker Compose", root))
					if errors.Is(resultErr, interactive.ErrBack) {
						return nil
					}
					return resultErr
				}
			}
			return executeInstallDockerCompose(command, root, options)
		},
	}
	command.Flags().StringVar(&options.version, "version", options.version, "exact semantic NopsAI version to install; defaults to this CLI build version")
	command.Flags().StringVar(&options.outputDir, "output-dir", options.outputDir, "directory where generated install files are stored")
	command.Flags().StringVar(&options.projectName, "project", options.projectName, "Docker Compose project name")
	command.Flags().StringVar(&options.apiPort, "api-port", options.apiPort, "host TCP port for the NopsAI API")
	command.Flags().StringVar(&options.uiPort, "ui-port", options.uiPort, "host TCP port for the NopsAI UI")
	command.Flags().StringVar(&options.nopsaiAPIURL, "nopsai-api-url", options.nopsaiAPIURL, "internal NopsAI API URL used by dispatcher, git-bot, and runners")
	command.Flags().StringVar(&options.dispatcherAddr, "dispatcher-address", options.dispatcherAddr, "dispatcher gRPC host:port used by API and runners")
	command.Flags().StringVar(&options.aaaAPIURL, "aaa-api-url", options.aaaAPIURL, "internal AAA API URL used by the NopsAI API")
	command.Flags().StringVar(&options.gitBotAPIURL, "git-bot-api-url", options.gitBotAPIURL, "internal git-bot API URL used by the NopsAI API")
	command.Flags().StringVar(&options.gotenbergURL, "gotenberg-url", options.gotenbergURL, "internal Gotenberg URL used for final-output PDF rendering")
	command.Flags().StringVar(&options.dockerNetwork, "docker-network", options.dockerNetwork, "Docker network name shared by generated services and Docker runner tasks")
	command.Flags().StringVar(&options.bootstrapAdminEmail, "bootstrap-admin-email", options.bootstrapAdminEmail, "initial local administrator email")
	command.Flags().StringVar(&options.bootstrapAdminPassword, "bootstrap-admin-password", "", "initial local administrator password; omitted generates one in .env")
	command.Flags().BoolVar(&options.force, "force", false, "replace previously generated install files in the output directory")
	command.Flags().BoolVar(&options.run, "run", false, "run docker compose up -d after writing generated files")
	command.Flags().BoolVar(&options.interactive, "interactive", false, "prompt for version, output directory, bootstrap admin, ports, overwrite, and run")
	return command
}

func defaultDockerComposeInstallOptions(root *rootOptions) *installDockerComposeOptions {
	return &installDockerComposeOptions{
		version:             defaultPlatformVersion(root),
		outputDir:           platform.DefaultInstallOutputDir,
		projectName:         platform.DefaultInstallProjectName,
		apiPort:             platform.DefaultInstallAPIPort,
		uiPort:              platform.DefaultInstallUIPort,
		nopsaiAPIURL:        platform.DefaultInstallNopsaiAPIURL,
		dispatcherAddr:      platform.DefaultInstallDispatcherAddress,
		aaaAPIURL:           platform.DefaultInstallAAAAPIURL,
		gitBotAPIURL:        platform.DefaultInstallGitBotAPIURL,
		gotenbergURL:        platform.DefaultInstallGotenbergURL,
		dockerNetwork:       platform.DefaultInstallDockerNetworkName,
		bootstrapAdminEmail: platform.DefaultInstallBootstrapAdminEmail,
	}
}

func newInstallKubernetesCommand(root *rootOptions) *cobra.Command {
	options := defaultKubernetesInstallOptions(root)
	command := &cobra.Command{
		Use:     "kubernetes",
		Aliases: []string{"k8s"},
		Short:   "Generate Kubernetes values and optionally deploy with Helm",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			var prompter *interactive.Prompter
			if options.interactive {
				prompter = interactive.NewPrompter(command.InOrStdin(), command.OutOrStdout())
				if err := resolveInteractiveKubernetesInstall(prompter, options, defaultPlatformVersion(root)); err != nil {
					return err
				}
				if err := previewInteractiveKubernetesInstall(prompter, options); err != nil {
					if errors.Is(err, interactive.ErrBack) {
						return renderInstallPreviewCancelled(command)
					}
					return err
				}
				if prompter.CanUseLiveSelector() {
					stdout, stderr, runErr := captureCommandOutput(command, func() error {
						return executeInstallKubernetes(command, root, options, command.Flags().Changed("version"))
					})
					resultErr := showInteractiveOutput(prompter, "Kubernetes install", stdout, stderr, runErr, installResultScreenOptions("Kubernetes", root))
					if errors.Is(resultErr, interactive.ErrBack) {
						return nil
					}
					return resultErr
				}
			}
			return executeInstallKubernetes(command, root, options, command.Flags().Changed("version"))
		},
	}
	command.Flags().StringVar(&options.version, "version", options.version, "exact semantic NopsAI version to install; defaults to this CLI build version")
	command.Flags().StringVar(&options.outputDir, "output-dir", options.outputDir, "directory where generated values and install metadata are stored")
	command.Flags().StringVar(&options.valuesFile, "values-file", options.valuesFile, "generated Kubernetes values file path relative to output-dir")
	command.Flags().StringArrayVarP(&options.values, "values", "f", nil, "additional Helm values file to merge after the generated sample; repeat in GitOps order")
	command.Flags().StringVar(&options.releaseName, "release", options.releaseName, "Helm release name to install or upgrade")
	command.Flags().StringVar(&options.namespace, "namespace", options.namespace, "Kubernetes namespace for all rendered and deployed resources")
	command.Flags().StringVar(&options.existingSecret, "existing-secret", options.existingSecret, "Kubernetes Secret name referenced by generated values")
	command.Flags().StringVar(&options.ingressHost, "ingress-host", "", "optional ingress host to enable in generated values")
	command.Flags().StringVar(&options.nopsaiAPIURL, "nopsai-api-url", options.nopsaiAPIURL, "internal NopsAI API URL used by dispatcher, git-bot, and runners")
	command.Flags().StringVar(&options.dispatcherAddr, "dispatcher-address", options.dispatcherAddr, "dispatcher gRPC host:port used by API and runners")
	command.Flags().StringVar(&options.aaaAPIURL, "aaa-api-url", options.aaaAPIURL, "internal AAA API URL used by the NopsAI API")
	command.Flags().StringVar(&options.gitBotAPIURL, "git-bot-api-url", options.gitBotAPIURL, "internal git-bot API URL used by the NopsAI API")
	command.Flags().StringVar(&options.gotenbergURL, "gotenberg-url", options.gotenbergURL, "internal Gotenberg URL used for final-output PDF rendering")
	command.Flags().StringVar(&options.bootstrapAdminEmail, "bootstrap-admin-email", options.bootstrapAdminEmail, "initial local administrator email")
	command.Flags().StringVar(&options.bootstrapAdminPasswordSecretKey, "bootstrap-admin-password-secret-key", options.bootstrapAdminPasswordSecretKey, "Kubernetes Secret key containing the initial local administrator password")
	command.Flags().StringVar(&options.lockFile, "lock-file", "", "GitOps-tracked release lock path written after successful deployment (default: output-dir/.nopsai/release.lock)")
	command.Flags().BoolVar(&options.force, "force", false, "replace previously generated install files in the output directory")
	command.Flags().BoolVar(&options.deploy, "deploy", false, "run Helm upgrade --install after writing generated values")
	command.Flags().BoolVar(&options.wait, "wait", false, "wait for Kubernetes resources to become ready before writing the release lock")
	command.Flags().BoolVar(&options.interactive, "interactive", false, "prompt for version, values, namespace, bootstrap admin, secrets, overwrite, and deployment")
	return command
}

func defaultKubernetesInstallOptions(root *rootOptions) *installKubernetesOptions {
	return &installKubernetesOptions{
		version:                         defaultPlatformVersion(root),
		outputDir:                       platform.DefaultInstallOutputDir,
		valuesFile:                      platform.DefaultKubernetesValuesFile,
		releaseName:                     platform.DefaultReleaseName,
		namespace:                       platform.DefaultNamespace,
		existingSecret:                  platform.DefaultKubernetesExistingSecret,
		nopsaiAPIURL:                    platform.DefaultInstallNopsaiAPIURL,
		dispatcherAddr:                  platform.DefaultInstallDispatcherAddress,
		aaaAPIURL:                       platform.DefaultInstallAAAAPIURL,
		gitBotAPIURL:                    platform.DefaultInstallGitBotAPIURL,
		gotenbergURL:                    platform.DefaultInstallGotenbergURL,
		bootstrapAdminEmail:             platform.DefaultInstallBootstrapAdminEmail,
		bootstrapAdminPasswordSecretKey: platform.DefaultKubernetesBootstrapAdminPasswordSecretKey,
	}
}

func installTargetScreenOptions(root *rootOptions) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "Install"},
		Title:      "Install",
		Header:     []string{"Version: " + valueOrDefault(defaultPlatformVersion(root), "not embedded; version is required")},
		LeftTitle:  "Targets",
		RightTitle: "Install Detail",
		LeftWidth:  38,
		Footer: []string{
			"Keys: type filter | Up/Down move | Enter select | Esc home | Ctrl+C quit",
			"Tip: generated Docker Compose and Helm values are versioned and GitOps friendly.",
		},
		Detail: func(index int, choice interactive.Choice) []string {
			switch index {
			case 0:
				return []string{
					choice.Description,
					"",
					"Generates",
					"  - docker-compose.yaml",
					"  - .env with local generated secrets",
					"  - db/init.sql",
					"  - .nopsai/install.lock",
					"",
					"Configurable",
					"  - host ports",
					"  - service URLs and gRPC addresses",
					"  - Docker network name",
					"  - bootstrap admin email and password",
					"",
					"Noninteractive example",
					"  nopsai install docker-compose --version 2.10.648 --output-dir ./nopsai-prod",
				}
			case 1:
				return []string{
					choice.Description,
					"",
					"Generates",
					"  - values.yaml",
					"  - .nopsai/install.lock",
					"",
					"Configurable",
					"  - Helm release and namespace",
					"  - existing Secret name",
					"  - bootstrap admin email and Secret key",
					"  - service URLs and gRPC addresses",
					"  - optional ingress host",
					"  - GitOps release lock path",
					"",
					"Noninteractive example",
					"  nopsai install kubernetes --version 2.10.648 --output-dir ./nopsai-prod --values-file values.yaml",
				}
			default:
				return []string{choice.Description}
			}
		},
	}
}

func installFormScreenOptions(target, version string) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb:  []string{"Home", "Install", target},
		Title:       target + " Install",
		Header:      []string{"Version: " + valueOrDefault(version, "not embedded; version is required")},
		LeftTitle:   "Install Steps",
		RightTitle:  "Values & Details",
		LeftWidth:   64,
		ActionLabel: "Generate install files",
		Footer: []string{
			"Keys: type to edit | Enter next/generate | Up/Down move | Ctrl+S generate | Esc targets | Ctrl+C quit",
			"Defaults are editable. Start typing on a field to replace its value.",
		},
	}
}

func installResultScreenOptions(target string, root *rootOptions) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "Install", target + " Result"},
		Title:      target + " Install Result",
		Header: []string{
			"Version: " + valueOrDefault(defaultPlatformVersion(root), "not embedded"),
			"Generated files stay local/GitOps friendly; no release manifest is required for install generation.",
		},
		Footer: []string{
			"Keys: Up/Down scroll | PgUp/PgDn jump | Home/End | Enter home | Esc targets | Ctrl+C quit",
		},
	}
}

func renderInstallPreviewCancelled(command *cobra.Command) error {
	_, err := fmt.Fprintln(command.OutOrStdout(), "Install cancelled before execution.")
	return err
}

func executeInstallDockerCompose(command *cobra.Command, root *rootOptions, options *installDockerComposeOptions) error {
	if strings.TrimSpace(options.version) == "" {
		return fmt.Errorf("--version is required when this CLI build does not embed a release version")
	}
	installer := installPlanner(root, command)
	plan, err := installer.PlanDockerCompose(command.Context(), platform.DockerComposeInstallOptions{
		Version:                options.version,
		OutputDir:              options.outputDir,
		ProjectName:            options.projectName,
		APIPort:                options.apiPort,
		UIPort:                 options.uiPort,
		NopsaiAPIURL:           options.nopsaiAPIURL,
		DispatcherAddress:      options.dispatcherAddr,
		AAAAPIURL:              options.aaaAPIURL,
		GitBotAPIURL:           options.gitBotAPIURL,
		GotenbergURL:           options.gotenbergURL,
		DockerNetworkName:      options.dockerNetwork,
		BootstrapAdminEmail:    options.bootstrapAdminEmail,
		BootstrapAdminPassword: options.bootstrapAdminPassword,
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
	if options.deploy && !options.force {
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
		Version:                         options.version,
		OutputDir:                       options.outputDir,
		ValuesFile:                      options.valuesFile,
		ReleaseName:                     options.releaseName,
		Namespace:                       options.namespace,
		ExistingSecret:                  options.existingSecret,
		IngressHost:                     options.ingressHost,
		NopsaiAPIURL:                    options.nopsaiAPIURL,
		DispatcherAddress:               options.dispatcherAddr,
		AAAAPIURL:                       options.aaaAPIURL,
		GitBotAPIURL:                    options.gitBotAPIURL,
		GotenbergURL:                    options.gotenbergURL,
		BootstrapAdminEmail:             options.bootstrapAdminEmail,
		BootstrapAdminPasswordSecretKey: options.bootstrapAdminPasswordSecretKey,
		Wait:                            options.wait,
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
	deploymentPlan, err := installer.DeployKubernetesValues(command.Context(), platform.KubernetesInstallDeployOptions{
		Version:     plan.Version,
		ValuesFiles: valuesFiles,
		ReleaseName: options.releaseName,
		Namespace:   options.namespace,
		Wait:        options.wait,
		LockFile:    lockFile,
	})
	if err != nil {
		return err
	}
	return renderInstallDeploymentPlan(command, deploymentPlan)
}

func deployExistingKubernetesInstall(command *cobra.Command, root *rootOptions, options *installKubernetesOptions, versionExplicit bool) (bool, error) {
	outputDir := installCommandOutputDir(options.outputDir)
	valuesFile, err := cleanInstallValuesFile(options.valuesFile)
	if err != nil {
		return true, err
	}
	valuesPath := filepath.Join(outputDir, valuesFile)
	valuesExists, err := regularFileExists(valuesPath)
	if err != nil {
		return true, err
	}
	if !valuesExists {
		return false, nil
	}
	storedVersion, err := readInstallValuesVersion(valuesPath)
	if err != nil {
		return true, err
	}
	version := storedVersion
	if versionExplicit {
		requested, err := compatibility.ParseVersion(options.version)
		if err != nil {
			return true, fmt.Errorf("invalid requested install version: %w", err)
		}
		if storedVersion != "" && requested.String() != storedVersion {
			return true, fmt.Errorf("stored Kubernetes install is for NopsAI %s, not requested version %s", storedVersion, requested.String())
		}
		version = requested.String()
	}
	if strings.TrimSpace(version) == "" {
		version = strings.TrimSpace(options.version)
	}
	if strings.TrimSpace(version) == "" {
		return true, fmt.Errorf("--version is required when stored values do not set global.releaseVersion")
	}
	lockFile := strings.TrimSpace(options.lockFile)
	if lockFile == "" {
		lockFile = filepath.Join(outputDir, platform.DefaultLockFile)
	}
	valuesFiles := []string{valuesPath}
	valuesFiles = append(valuesFiles, options.values...)
	installer := installPlanner(root, command)
	deploymentPlan, err := installer.DeployKubernetesValues(command.Context(), platform.KubernetesInstallDeployOptions{
		Version:     version,
		ValuesFiles: valuesFiles,
		ReleaseName: options.releaseName,
		Namespace:   options.namespace,
		Wait:        options.wait,
		LockFile:    lockFile,
	})
	if err != nil {
		return true, err
	}
	if _, err := fmt.Fprintf(command.OutOrStdout(), "Using generated NopsAI %s kubernetes install in %s\n", version, outputDir); err != nil {
		return true, err
	}
	return true, renderInstallDeploymentPlan(command, deploymentPlan)
}

func readInstallValuesVersion(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read stored values: %w", err)
	}
	var values struct {
		Global struct {
			ReleaseVersion string `yaml:"releaseVersion"`
		} `yaml:"global"`
	}
	if err := yaml.Unmarshal(raw, &values); err != nil {
		return "", fmt.Errorf("decode stored values: %w", err)
	}
	if strings.TrimSpace(values.Global.ReleaseVersion) == "" {
		return "", nil
	}
	version, err := compatibility.ParseVersion(values.Global.ReleaseVersion)
	if err != nil {
		return "", fmt.Errorf("invalid stored global.releaseVersion: %w", err)
	}
	return version.String(), nil
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
	if prompter.CanUseLiveSelector() {
		return resolveLiveDockerComposeInstall(prompter, options, defaultVersion)
	}
	version, err := prompter.AskRequired("NopsAI version", valueOrDefault(options.version, strings.TrimSpace(defaultVersion)))
	if err != nil {
		return err
	}
	options.version = strings.TrimSpace(version)
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
	apiURL, err := prompter.AskRequired("Internal NopsAI API URL", valueOrDefault(options.nopsaiAPIURL, platform.DefaultInstallNopsaiAPIURL))
	if err != nil {
		return err
	}
	options.nopsaiAPIURL = strings.TrimSpace(apiURL)
	dispatcher, err := prompter.AskRequired("Dispatcher gRPC address", valueOrDefault(options.dispatcherAddr, platform.DefaultInstallDispatcherAddress))
	if err != nil {
		return err
	}
	options.dispatcherAddr = strings.TrimSpace(dispatcher)
	aaaURL, err := prompter.AskRequired("Internal AAA API URL", valueOrDefault(options.aaaAPIURL, platform.DefaultInstallAAAAPIURL))
	if err != nil {
		return err
	}
	options.aaaAPIURL = strings.TrimSpace(aaaURL)
	gitBotURL, err := prompter.AskRequired("Internal git-bot API URL", valueOrDefault(options.gitBotAPIURL, platform.DefaultInstallGitBotAPIURL))
	if err != nil {
		return err
	}
	options.gitBotAPIURL = strings.TrimSpace(gitBotURL)
	gotenbergURL, err := prompter.AskRequired("Internal Gotenberg URL", valueOrDefault(options.gotenbergURL, platform.DefaultInstallGotenbergURL))
	if err != nil {
		return err
	}
	options.gotenbergURL = strings.TrimSpace(gotenbergURL)
	dockerNetwork, err := prompter.AskRequired("Docker network name", valueOrDefault(options.dockerNetwork, platform.DefaultInstallDockerNetworkName))
	if err != nil {
		return err
	}
	options.dockerNetwork = strings.TrimSpace(dockerNetwork)
	adminEmail, err := prompter.AskRequired("Bootstrap admin email", valueOrDefault(options.bootstrapAdminEmail, platform.DefaultInstallBootstrapAdminEmail))
	if err != nil {
		return err
	}
	options.bootstrapAdminEmail = strings.TrimSpace(adminEmail)
	adminPassword, err := prompter.Ask("Bootstrap admin password (blank generates)", options.bootstrapAdminPassword)
	if err != nil {
		return err
	}
	options.bootstrapAdminPassword = strings.TrimSpace(adminPassword)
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
	if prompter.CanUseLiveSelector() {
		return resolveLiveKubernetesInstall(prompter, options, defaultVersion)
	}
	version, err := prompter.AskRequired("NopsAI version", valueOrDefault(options.version, strings.TrimSpace(defaultVersion)))
	if err != nil {
		return err
	}
	options.version = strings.TrimSpace(version)
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
	adminEmail, err := prompter.AskRequired("Bootstrap admin email", valueOrDefault(options.bootstrapAdminEmail, platform.DefaultInstallBootstrapAdminEmail))
	if err != nil {
		return err
	}
	options.bootstrapAdminEmail = strings.TrimSpace(adminEmail)
	adminPasswordKey, err := prompter.AskRequired("Bootstrap admin password Secret key", valueOrDefault(options.bootstrapAdminPasswordSecretKey, platform.DefaultKubernetesBootstrapAdminPasswordSecretKey))
	if err != nil {
		return err
	}
	options.bootstrapAdminPasswordSecretKey = strings.TrimSpace(adminPasswordKey)
	ingressHost, err := prompter.Ask("Ingress host (blank disables ingress)", options.ingressHost)
	if err != nil {
		return err
	}
	options.ingressHost = strings.TrimSpace(ingressHost)
	apiURL, err := prompter.AskRequired("Internal NopsAI API URL", valueOrDefault(options.nopsaiAPIURL, platform.DefaultInstallNopsaiAPIURL))
	if err != nil {
		return err
	}
	options.nopsaiAPIURL = strings.TrimSpace(apiURL)
	dispatcher, err := prompter.AskRequired("Dispatcher gRPC address", valueOrDefault(options.dispatcherAddr, platform.DefaultInstallDispatcherAddress))
	if err != nil {
		return err
	}
	options.dispatcherAddr = strings.TrimSpace(dispatcher)
	aaaURL, err := prompter.AskRequired("Internal AAA API URL", valueOrDefault(options.aaaAPIURL, platform.DefaultInstallAAAAPIURL))
	if err != nil {
		return err
	}
	options.aaaAPIURL = strings.TrimSpace(aaaURL)
	gitBotURL, err := prompter.AskRequired("Internal git-bot API URL", valueOrDefault(options.gitBotAPIURL, platform.DefaultInstallGitBotAPIURL))
	if err != nil {
		return err
	}
	options.gitBotAPIURL = strings.TrimSpace(gitBotURL)
	gotenbergURL, err := prompter.AskRequired("Internal Gotenberg URL", valueOrDefault(options.gotenbergURL, platform.DefaultInstallGotenbergURL))
	if err != nil {
		return err
	}
	options.gotenbergURL = strings.TrimSpace(gotenbergURL)
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

func resolveLiveDockerComposeInstall(prompter *interactive.Prompter, options *installDockerComposeOptions, defaultVersion string) error {
	fields := []interactive.Field{
		{Name: "version", Label: "NopsAI version", Value: options.version, Default: strings.TrimSpace(defaultVersion), Required: true, Description: "Exact semantic NopsAI version used to generate image tags and install metadata.", Example: "2.10.648"},
		{Name: "outputDir", Label: "Output directory", Value: options.outputDir, Default: platform.DefaultInstallOutputDir, Required: true, Description: "Directory where docker-compose.yaml, .env, database init files, and install lock metadata are written.", Example: "./nopsai-prod"},
		{Name: "projectName", Label: "Compose project", Value: options.projectName, Default: platform.DefaultInstallProjectName, Required: true, Description: "Docker Compose project name. Use different names when multiple NopsAI stacks share one host.", Example: "nopsai-prod"},
		{Name: "apiPort", Label: "API host port", Value: options.apiPort, Default: platform.DefaultInstallAPIPort, Required: true, Description: "Host TCP port published for the NopsAI API.", Example: "8080"},
		{Name: "uiPort", Label: "UI host port", Value: options.uiPort, Default: platform.DefaultInstallUIPort, Required: true, Description: "Host TCP port published for the NopsAI UI.", Example: "80"},
		{Name: "nopsaiAPIURL", Label: "NopsAI API URL", Value: options.nopsaiAPIURL, Default: platform.DefaultInstallNopsaiAPIURL, Required: true, Description: "Internal NopsAI API URL used by dispatcher, git-bot, and runners. Change this when services live on a custom network or host.", Example: "http://nopsai:8080"},
		{Name: "dispatcherAddr", Label: "Dispatcher gRPC", Value: options.dispatcherAddr, Default: platform.DefaultInstallDispatcherAddress, Required: true, Description: "Internal dispatcher gRPC host:port used by the API and runners.", Example: "dispatcher:9090"},
		{Name: "aaaAPIURL", Label: "AAA API URL", Value: options.aaaAPIURL, Default: platform.DefaultInstallAAAAPIURL, Required: true, Description: "Internal AAA API URL used by the NopsAI API for authentication and authorization.", Example: "http://aaa:8082"},
		{Name: "gitBotAPIURL", Label: "git-bot API URL", Value: options.gitBotAPIURL, Default: platform.DefaultInstallGitBotAPIURL, Required: true, Description: "Internal git-bot API URL used by NopsAI for repository automation.", Example: "http://git-bot:8081"},
		{Name: "gotenbergURL", Label: "Gotenberg URL", Value: options.gotenbergURL, Default: platform.DefaultInstallGotenbergURL, Required: true, Description: "Internal Gotenberg URL used for final-output PDF rendering.", Example: "http://gotenberg:3000"},
		{Name: "dockerNetwork", Label: "Docker network", Value: options.dockerNetwork, Default: platform.DefaultInstallDockerNetworkName, Required: true, Description: "Docker network shared by generated services and Docker runner tasks.", Example: "nopsai-net"},
		{Name: "bootstrapAdminEmail", Label: "Admin email", Value: options.bootstrapAdminEmail, Default: platform.DefaultInstallBootstrapAdminEmail, Required: true, Description: "Initial local administrator email created on first startup.", Example: "platform-admin@example.com"},
		{Name: "bootstrapAdminPassword", Label: "Admin password", Value: options.bootstrapAdminPassword, Description: "Initial local administrator password. Leave blank to generate one into .env.", Example: "use-a-unique-secret"},
		{Name: "force", Label: "Replace files", Value: formatYesNo(options.force), Kind: interactive.FieldBoolean, Description: "Replace existing generated install files in the output directory."},
		{Name: "run", Label: "Start stack", Value: formatYesNo(options.run), Kind: interactive.FieldBoolean, Description: "Run docker compose up -d after writing generated files."},
	}
	edited, err := prompter.EditFieldsScreen("Docker Compose install", fields, installFormScreenOptions("Docker Compose", rootVersionForScreen(defaultVersion)))
	if err != nil {
		return err
	}
	values := fieldValueMap(edited)
	options.version = strings.TrimSpace(values["version"])
	options.outputDir = strings.TrimSpace(values["outputDir"])
	options.projectName = strings.TrimSpace(values["projectName"])
	options.apiPort = strings.TrimSpace(values["apiPort"])
	options.uiPort = strings.TrimSpace(values["uiPort"])
	options.nopsaiAPIURL = strings.TrimSpace(values["nopsaiAPIURL"])
	options.dispatcherAddr = strings.TrimSpace(values["dispatcherAddr"])
	options.aaaAPIURL = strings.TrimSpace(values["aaaAPIURL"])
	options.gitBotAPIURL = strings.TrimSpace(values["gitBotAPIURL"])
	options.gotenbergURL = strings.TrimSpace(values["gotenbergURL"])
	options.dockerNetwork = strings.TrimSpace(values["dockerNetwork"])
	options.bootstrapAdminEmail = strings.TrimSpace(values["bootstrapAdminEmail"])
	options.bootstrapAdminPassword = strings.TrimSpace(values["bootstrapAdminPassword"])
	options.force = parseYesNo(values["force"])
	options.run = parseYesNo(values["run"])
	return nil
}

func resolveLiveKubernetesInstall(prompter *interactive.Prompter, options *installKubernetesOptions, defaultVersion string) error {
	fields := []interactive.Field{
		{Name: "version", Label: "NopsAI version", Value: options.version, Default: strings.TrimSpace(defaultVersion), Required: true, Description: "Exact semantic NopsAI version used to generate image tags, Helm chart version, values, and lock metadata.", Example: "2.10.648"},
		{Name: "outputDir", Label: "Output directory", Value: options.outputDir, Default: platform.DefaultInstallOutputDir, Required: true, Description: "Directory where generated Helm values and install metadata are written.", Example: "./nopsai-prod"},
		{Name: "valuesFile", Label: "Values file", Value: options.valuesFile, Default: platform.DefaultKubernetesValuesFile, Required: true, Description: "Generated values file path relative to the output directory. Keep this GitOps-tracked.", Example: "values.yaml"},
		{Name: "releaseName", Label: "Helm release", Value: options.releaseName, Default: platform.DefaultReleaseName, Required: true, Description: "Helm release name to install or upgrade.", Example: "nopsai"},
		{Name: "namespace", Label: "Namespace", Value: options.namespace, Default: platform.DefaultNamespace, Required: true, Description: "Kubernetes namespace for rendered and deployed resources.", Example: "nopsai"},
		{Name: "existingSecret", Label: "Existing Secret", Value: options.existingSecret, Default: platform.DefaultKubernetesExistingSecret, Required: true, Description: "Kubernetes Secret referenced by generated values. It should contain database URL, master key, JWT keys, service JWT key, AAA shared internal token, and bootstrap admin password.", Example: "nopsai-secrets"},
		{Name: "bootstrapAdminEmail", Label: "Admin email", Value: options.bootstrapAdminEmail, Default: platform.DefaultInstallBootstrapAdminEmail, Required: true, Description: "Initial local administrator email created on first startup.", Example: "platform-admin@example.com"},
		{Name: "bootstrapAdminPasswordSecretKey", Label: "Admin password key", Value: options.bootstrapAdminPasswordSecretKey, Default: platform.DefaultKubernetesBootstrapAdminPasswordSecretKey, Required: true, Description: "Secret key in the existing Kubernetes Secret that contains the initial local administrator password.", Example: "bootstrap-admin-password"},
		{Name: "ingressHost", Label: "Ingress host", Value: options.ingressHost, Description: "Optional ingress host. Leave blank to keep ingress disabled in generated values.", Example: "nopsai.example.com"},
		{Name: "nopsaiAPIURL", Label: "NopsAI API URL", Value: options.nopsaiAPIURL, Default: platform.DefaultInstallNopsaiAPIURL, Required: true, Description: "Internal NopsAI API URL used by dispatcher, git-bot, and runners. Change this for custom service DNS or mesh addresses.", Example: "http://nopsai:8080"},
		{Name: "dispatcherAddr", Label: "Dispatcher gRPC", Value: options.dispatcherAddr, Default: platform.DefaultInstallDispatcherAddress, Required: true, Description: "Internal dispatcher gRPC host:port used by the API and runners.", Example: "dispatcher:9090"},
		{Name: "aaaAPIURL", Label: "AAA API URL", Value: options.aaaAPIURL, Default: platform.DefaultInstallAAAAPIURL, Required: true, Description: "Internal AAA API URL used by the NopsAI API for authentication and authorization.", Example: "http://aaa:8082"},
		{Name: "gitBotAPIURL", Label: "git-bot API URL", Value: options.gitBotAPIURL, Default: platform.DefaultInstallGitBotAPIURL, Required: true, Description: "Internal git-bot API URL used by NopsAI for repository automation.", Example: "http://git-bot:8081"},
		{Name: "gotenbergURL", Label: "Gotenberg URL", Value: options.gotenbergURL, Default: platform.DefaultInstallGotenbergURL, Required: true, Description: "Internal Gotenberg URL used for final-output PDF rendering.", Example: "http://gotenberg:3000"},
		{Name: "force", Label: "Replace files", Value: formatYesNo(options.force), Kind: interactive.FieldBoolean, Description: "Replace existing generated install files in the output directory."},
		{Name: "deploy", Label: "Deploy with Helm", Value: formatYesNo(options.deploy), Kind: interactive.FieldBoolean, Description: "Run Helm upgrade --install after writing generated values. Leave off for GitOps-only generation."},
		{Name: "wait", Label: "Wait for rollout", Value: formatYesNo(options.wait), Kind: interactive.FieldBoolean, Description: "Wait for Kubernetes resources to become ready before writing the release lock."},
		{Name: "lockFile", Label: "Release lock", Value: options.lockFile, Description: "GitOps-tracked release lock path. Blank defaults to output-dir/.nopsai/release.lock.", Example: "clusters/prod/nopsai/.nopsai/release.lock"},
	}
	edited, err := prompter.EditFieldsScreen("Kubernetes install", fields, installFormScreenOptions("Kubernetes", rootVersionForScreen(defaultVersion)))
	if err != nil {
		return err
	}
	values := fieldValueMap(edited)
	options.version = strings.TrimSpace(values["version"])
	options.outputDir = strings.TrimSpace(values["outputDir"])
	options.valuesFile = strings.TrimSpace(values["valuesFile"])
	options.releaseName = strings.TrimSpace(values["releaseName"])
	options.namespace = strings.TrimSpace(values["namespace"])
	options.existingSecret = strings.TrimSpace(values["existingSecret"])
	options.bootstrapAdminEmail = strings.TrimSpace(values["bootstrapAdminEmail"])
	options.bootstrapAdminPasswordSecretKey = strings.TrimSpace(values["bootstrapAdminPasswordSecretKey"])
	options.ingressHost = strings.TrimSpace(values["ingressHost"])
	options.nopsaiAPIURL = strings.TrimSpace(values["nopsaiAPIURL"])
	options.dispatcherAddr = strings.TrimSpace(values["dispatcherAddr"])
	options.aaaAPIURL = strings.TrimSpace(values["aaaAPIURL"])
	options.gitBotAPIURL = strings.TrimSpace(values["gitBotAPIURL"])
	options.gotenbergURL = strings.TrimSpace(values["gotenbergURL"])
	options.force = parseYesNo(values["force"])
	options.deploy = parseYesNo(values["deploy"])
	options.wait = parseYesNo(values["wait"])
	options.lockFile = strings.TrimSpace(values["lockFile"])
	return nil
}

func fieldValueMap(fields []interactive.Field) map[string]string {
	values := make(map[string]string, len(fields))
	for _, field := range fields {
		values[field.Name] = field.Value
	}
	return values
}

func rootVersionForScreen(defaultVersion string) string {
	return valueOrDefault(strings.TrimSpace(defaultVersion), "dev")
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

func renderInstallDeploymentPlan(command *cobra.Command, plan platform.KubernetesInstallDeploymentPlan) error {
	if _, err := fmt.Fprintf(command.OutOrStdout(), "Deployed NopsAI %s as %s in namespace %s\n", plan.Version, plan.ReleaseName, plan.Namespace); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(command.OutOrStdout(), "Chart: %s --version %s\nValues hash: %s\nRelease lock: %s\n", plan.ChartReference, plan.ChartVersion, plan.ValuesHash, plan.LockFile); err != nil {
		return err
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

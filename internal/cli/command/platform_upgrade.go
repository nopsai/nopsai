package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"nopsai/internal/cli/interactive"
	"nopsai/internal/cli/platform"
	"nopsai/internal/cli/selfupdate"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type platformUpgradeOptions struct {
	version           string
	outputDir         string
	releaseName       string
	namespace         string
	chartReference    string
	values            []string
	lockFile          string
	changelogRepo     string
	changelogBaseURL  string
	output            string
	run               bool
	deploy            bool
	wait              bool
	acknowledgeSeries bool
	planOnly          bool
}

func newPlatformUpgradeCommand(root *rootOptions) *cobra.Command {
	command := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade an installed NopsAI platform to a newer release",
		Long: "Upgrade an existing NopsAI install to a newer release version. The upgrade reads the install or deployment lock written by " +
			"nopsai install, keeps the secrets that install generated, and prints the release changelog and the required operator actions " +
			"before anything is applied. Update this CLI first with nopsai update; the CLI owns the templates a target release expects.",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runPlatformUpgradeWizard(command, root)
		},
	}
	command.AddCommand(newPlatformUpgradeDockerComposeCommand(root))
	command.AddCommand(newPlatformUpgradeKubernetesCommand(root))
	return command
}

// runPlatformUpgradeWizard shares the interactive console's upgrade flow so
// `nopsai platform upgrade` and Home -> Platform -> Upgrade behave identically.
// It defaults to a plan so a bare invocation reviews the upgrade rather than
// applying one.
func runPlatformUpgradeWizard(command *cobra.Command, root *rootOptions) error {
	prompter := interactive.NewPrompter(command.InOrStdin(), command.OutOrStdout())
	options := defaultPlatformUpgradeOptions(root)
	options.planOnly = true
	err := runInteractivePlatformUpgrade(command, root, prompter, options)
	if errors.Is(err, interactive.ErrBack) || errors.Is(err, interactive.ErrCancelled) {
		return nil
	}
	return err
}

func defaultPlatformUpgradeOptions(root *rootOptions) *platformUpgradeOptions {
	return &platformUpgradeOptions{version: defaultPlatformVersion(root), output: "text"}
}

func newPlatformUpgradeDockerComposeCommand(root *rootOptions) *cobra.Command {
	options := defaultPlatformUpgradeOptions(root)
	command := &cobra.Command{
		Use:     "docker-compose",
		Aliases: []string{"compose", "docker"},
		Short:   "Upgrade a Docker Compose install to a newer release",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return executePlatformUpgradeDockerCompose(command, root, options)
		},
	}
	addPlatformUpgradeCommonFlags(command, options, defaultPlatformVersion(root))
	command.Flags().StringVar(&options.outputDir, "output-dir", platform.DefaultInstallOutputDir, "directory that holds the generated install files")
	command.Flags().BoolVar(&options.run, "run", false, "after writing the upgraded files, run docker compose up -d")
	return command
}

func newPlatformUpgradeKubernetesCommand(root *rootOptions) *cobra.Command {
	options := defaultPlatformUpgradeOptions(root)
	command := &cobra.Command{
		Use:     "kubernetes",
		Aliases: []string{"k8s"},
		Short:   "Upgrade a Helm-deployed install to a newer release",
		Args:    cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return executePlatformUpgradeKubernetes(command, root, options)
		},
	}
	addPlatformUpgradeCommonFlags(command, options, defaultPlatformVersion(root))
	command.Flags().StringVar(&options.releaseName, "release", "", "Helm release name (default: the name in the deployment lock)")
	command.Flags().StringVar(&options.namespace, "namespace", "", "Kubernetes namespace (default: the namespace in the deployment lock)")
	command.Flags().StringVar(&options.chartReference, "chart", "", "OCI chart reference (default: the chart in the deployment lock)")
	command.Flags().StringArrayVarP(&options.values, "values", "f", nil, "Helm values file to use instead of the files recorded in the deployment lock")
	command.Flags().StringVar(&options.lockFile, "lock-file", platform.DefaultLockFile, "deployment lock written by the last install or upgrade")
	command.Flags().BoolVar(&options.deploy, "deploy", false, "after a successful plan, run helm upgrade --install and rewrite the deployment lock")
	command.Flags().BoolVar(&options.wait, "wait", false, "wait for Kubernetes resources to become ready before writing the lock")
	return command
}

func addPlatformUpgradeCommonFlags(command *cobra.Command, options *platformUpgradeOptions, defaultVersion string) {
	versionHelp := "exact semantic release version to upgrade to"
	if strings.TrimSpace(defaultVersion) != "" {
		versionHelp += " (defaults to this CLI build version)"
	}
	command.Flags().StringVar(&options.version, "version", defaultVersion, versionHelp)
	command.Flags().StringVarP(&options.output, "output", "o", "text", "output format for the upgrade plan: text, json, or yaml")
	command.Flags().BoolVar(&options.planOnly, "plan", false, "print the upgrade plan and required actions without changing anything")
	command.Flags().BoolVar(&options.acknowledgeSeries, "accept-series-upgrade", false, "confirm the reviewed changelog for an upgrade that crosses a compatibility series")
	command.Flags().StringVar(&options.changelogRepo, "changelog-repository", "", "GitHub owner/repository that publishes the release changelog (default: the public NopsAI repository)")
	command.Flags().StringVar(&options.changelogBaseURL, "changelog-base-url", "", "HTTPS base URL that hosts the release changelog and SHA256SUMS")
}

func executePlatformUpgradeDockerCompose(command *cobra.Command, root *rootOptions, options *platformUpgradeOptions) error {
	if strings.TrimSpace(options.version) == "" {
		return fmt.Errorf("--version is required when this CLI build does not embed a release version")
	}
	upgrader := platformUpgrader(root, command, options)
	plan, err := upgrader.PlanDockerCompose(command.Context(), platform.DockerComposeUpgradeOptions{
		Version:   options.version,
		OutputDir: options.outputDir,
	})
	if err != nil {
		return err
	}
	if err := renderPlatformUpgradePlan(command, plan, options.output, false); err != nil {
		return err
	}
	if options.planOnly {
		return nil
	}
	if err := requireSeriesUpgradeAcknowledgement(plan, options); err != nil {
		return err
	}
	if err := upgrader.ApplyDockerCompose(command.Context(), plan, options.run); err != nil {
		return err
	}
	return renderPlatformUpgradeResult(command, plan, options.output, options.run)
}

func executePlatformUpgradeKubernetes(command *cobra.Command, root *rootOptions, options *platformUpgradeOptions) error {
	if strings.TrimSpace(options.version) == "" {
		return fmt.Errorf("--version is required when this CLI build does not embed a release version")
	}
	upgrader := platformUpgrader(root, command, options)
	plan, err := upgrader.PlanKubernetes(command.Context(), platform.KubernetesUpgradeOptions{
		Version:        options.version,
		ChartReference: options.chartReference,
		ValuesFiles:    options.values,
		ReleaseName:    options.releaseName,
		Namespace:      options.namespace,
		Wait:           options.wait,
		LockFile:       options.lockFile,
	})
	if err != nil {
		return err
	}
	if err := renderPlatformUpgradePlan(command, plan, options.output, false); err != nil {
		return err
	}
	if options.planOnly || !options.deploy {
		return nil
	}
	if err := requireSeriesUpgradeAcknowledgement(plan, options); err != nil {
		return err
	}
	deployment, err := upgrader.DeployKubernetes(command.Context(), plan, options.wait)
	if err != nil {
		return err
	}
	return renderInstallDeploymentPlan(command, deployment)
}

// requireSeriesUpgradeAcknowledgement keeps a compatibility-series upgrade from
// being applied before the operator has seen the changelog and the actions it
// requires.
func requireSeriesUpgradeAcknowledgement(plan platform.UpgradePlan, options *platformUpgradeOptions) error {
	if !plan.SeriesUpgrade || options.acknowledgeSeries {
		return nil
	}
	return fmt.Errorf(
		"upgrading from %s to %s crosses a compatibility series; review the changelog and required actions above, then re-run with --accept-series-upgrade",
		plan.CurrentVersion, plan.Version,
	)
}

func platformUpgrader(root *rootOptions, command *cobra.Command, options *platformUpgradeOptions) platform.Upgrader {
	return platform.Upgrader{
		Installer: installPlanner(root, command),
		Changelog: platformChangelogFetcher(root, options),
	}
}

func platformChangelogFetcher(root *rootOptions, options *platformUpgradeOptions) platform.ChangelogFetcher {
	httpClient := root.dependencies.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultUpdateTimeout}
	}
	updater := selfupdate.Updater{
		HTTPClient: httpClient,
		Token:      root.dependencies.Getenv("NOPSAI_UPDATE_TOKEN"),
	}
	repository := updateRepository(root, options.changelogRepo)
	baseURL := strings.TrimSpace(options.changelogBaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(root.dependencies.Getenv("NOPSAI_CHANGELOG_BASE_URL"))
	}
	return platform.ReleaseChangelogFetcher(func(ctx context.Context, assetName string) ([]byte, string, error) {
		return updater.ReleaseAsset(ctx, selfupdate.AssetOptions{
			Version:      options.version,
			AssetName:    assetName,
			Repository:   repository,
			AssetBaseURL: baseURL,
		})
	})
}

func renderPlatformUpgradePlan(command *cobra.Command, plan platform.UpgradePlan, output string, applied bool) error {
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "text":
		return renderPlatformUpgradePlanText(command, plan, applied)
	case "json":
		encoder := json.NewEncoder(command.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(plan)
	case "yaml":
		encoder := yaml.NewEncoder(command.OutOrStdout())
		defer encoder.Close()
		encoder.SetIndent(2)
		return encoder.Encode(plan)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func renderPlatformUpgradePlanText(command *cobra.Command, plan platform.UpgradePlan, applied bool) error {
	out := command.OutOrStdout()
	verb := "Planned"
	if applied {
		verb = "Upgraded"
	}
	if _, err := fmt.Fprintf(out, "%s NopsAI %s upgrade from %s to %s (CLI %s)\n", verb, plan.Target, plan.CurrentVersion, plan.Version, plan.CLI); err != nil {
		return err
	}
	if plan.OutputDir != "" {
		if _, err := fmt.Fprintf(out, "Install directory: %s\n", plan.OutputDir); err != nil {
			return err
		}
	}
	if plan.ReleaseName != "" {
		if _, err := fmt.Fprintf(out, "Helm release: %s in namespace %s\nChart: %s --version %s\nValues files: %s\n",
			plan.ReleaseName, plan.Namespace, plan.ChartReference, plan.Version, strings.Join(plan.ValuesFiles, ", ")); err != nil {
			return err
		}
	}
	if plan.SeriesUpgrade {
		if _, err := fmt.Fprintf(out, "Series upgrade: yes. This release changes the compatibility series.\n"); err != nil {
			return err
		}
	}
	if plan.Changelog != "" {
		if _, err := fmt.Fprintf(out, "\n--- Changelog for %s (%s) ---\n%s\n", plan.Version, plan.ChangelogSource, plan.Changelog); err != nil {
			return err
		}
	}
	if len(plan.RequiredActions) > 0 {
		if _, err := fmt.Fprintf(out, "\nRequired actions:\n"); err != nil {
			return err
		}
		for _, action := range plan.RequiredActions {
			if _, err := fmt.Fprintf(out, "  - %s\n", action); err != nil {
				return err
			}
		}
	}
	for _, warning := range plan.Warnings {
		if _, err := fmt.Fprintf(out, "Warning: %s\n", warning); err != nil {
			return err
		}
	}
	if !applied && plan.Command != "" {
		if _, err := fmt.Fprintf(out, "Next: %s\n", plan.Command); err != nil {
			return err
		}
	}
	return nil
}

func renderPlatformUpgradeResult(command *cobra.Command, plan platform.UpgradePlan, output string, executed bool) error {
	if strings.ToLower(strings.TrimSpace(output)) != "text" {
		return nil
	}
	out := command.OutOrStdout()
	if _, err := fmt.Fprintf(out, "Upgraded NopsAI %s install in %s to %s\n", plan.Target, plan.OutputDir, plan.Version); err != nil {
		return err
	}
	if plan.Command == "" {
		return nil
	}
	if executed {
		_, err := fmt.Fprintf(out, "Executed: %s\n", plan.Command)
		return err
	}
	_, err := fmt.Fprintf(out, "Next: %s\n", plan.Command)
	return err
}

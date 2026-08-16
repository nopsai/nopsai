package command

import (
	"errors"
	"fmt"
	"strings"

	"nopsai/internal/cli/interactive"
	"nopsai/internal/cli/platform"

	"github.com/spf13/cobra"
)

// runInteractivePlatformUpgrade drives `platform upgrade` from the interactive
// console. It is the same flow the bare `nopsai platform upgrade` command uses,
// so both entry points pick a target, review a form, confirm the equivalent
// noninteractive command, and then read the plan or the applied result.
func runInteractivePlatformUpgrade(command *cobra.Command, root *rootOptions, prompter *interactive.Prompter, options *platformUpgradeOptions) error {
	target, err := chooseInteractivePlatformUpgradeTarget(prompter, root)
	if err != nil {
		return err
	}
	return runInteractivePlatformUpgradeTarget(command, root, prompter, options, target)
}

func runInteractivePlatformUpgradeTarget(command *cobra.Command, root *rootOptions, prompter *interactive.Prompter, options *platformUpgradeOptions, target string) error {
	switch target {
	case "docker-compose":
		return runInteractivePlatformUpgradeDockerCompose(command, root, prompter, options)
	case "kubernetes":
		return runInteractivePlatformUpgradeKubernetes(command, root, prompter, options)
	default:
		return fmt.Errorf("unsupported upgrade target %q; expected docker-compose or kubernetes", target)
	}
}

func runInteractivePlatformUpgradeDockerCompose(command *cobra.Command, root *rootOptions, prompter *interactive.Prompter, options *platformUpgradeOptions) error {
	if err := resolveInteractiveUpgradeDockerComposeOptions(prompter, options, defaultPlatformVersion(root)); err != nil {
		return err
	}
	if strings.TrimSpace(options.version) == "" {
		return fmt.Errorf("--version is required when this CLI build does not embed a release version")
	}
	if err := showInteractiveCommandPreview(prompter, "Platform upgrade command preview", platformUpgradeDockerComposePreviewArgs(options), upgradePreviewEffects(options.planOnly, []string{
		"Rewrite the generated Compose files for the target release, keeping the secrets the install created.",
		"Restart the stack with docker compose up -d only when Run compose after apply is yes.",
	}), commandPreviewScreenOptions([]string{"Home", "Platform", "Upgrade", "Docker Compose", "Preview"}, "Platform Upgrade Preview", upgradeHeaderLines(root, options))); err != nil {
		return err
	}
	return executeInteractivePlatformUpgrade(command, prompter, options, "docker-compose", func() error {
		return executePlatformUpgradeDockerCompose(command, root, options)
	})
}

func runInteractivePlatformUpgradeKubernetes(command *cobra.Command, root *rootOptions, prompter *interactive.Prompter, options *platformUpgradeOptions) error {
	if err := resolveInteractiveUpgradeKubernetesOptions(prompter, options, defaultPlatformVersion(root)); err != nil {
		return err
	}
	if strings.TrimSpace(options.version) == "" {
		return fmt.Errorf("--version is required when this CLI build does not embed a release version")
	}
	if err := showInteractiveCommandPreview(prompter, "Platform upgrade command preview", platformUpgradeKubernetesPreviewArgs(options), upgradePreviewEffects(options.planOnly || !options.deploy, []string{
		"Read the deployment lock, resolve the target chart, and verify the release before anything changes.",
		"Run helm upgrade --install and rewrite the deployment lock only when Deploy after plan is yes.",
	}), commandPreviewScreenOptions([]string{"Home", "Platform", "Upgrade", "Kubernetes", "Preview"}, "Platform Upgrade Preview", upgradeHeaderLines(root, options))); err != nil {
		return err
	}
	return executeInteractivePlatformUpgrade(command, prompter, options, "kubernetes", func() error {
		return executePlatformUpgradeKubernetes(command, root, options)
	})
}

// executeInteractivePlatformUpgrade runs an upgrade and shows its output on a
// result screen. A refused series upgrade surfaces there like any other failure,
// so the operator reads the changelog and required actions before retrying with
// the acknowledgement set.
func executeInteractivePlatformUpgrade(command *cobra.Command, prompter *interactive.Prompter, options *platformUpgradeOptions, target string, run func() error) error {
	if !prompter.CanUseLiveSelector() {
		return run()
	}
	stdout, stderr, upgradeErr := captureCommandOutput(command, run)
	resultErr := showInteractiveOutput(prompter, "Platform upgrade", stdout, stderr, upgradeErr, platformUpgradeResultScreenOptions(*options, target))
	if errors.Is(resultErr, interactive.ErrBack) {
		return interactive.ErrBack
	}
	return resultErr
}

func chooseInteractivePlatformUpgradeTarget(prompter *interactive.Prompter, root *rootOptions) (string, error) {
	choices := []interactive.Choice{
		{Label: "docker-compose", Description: "Upgrade a generated Docker Compose install", SearchText: "docker compose local single host"},
		{Label: "kubernetes", Description: "Upgrade a Helm-deployed install", SearchText: "kubernetes k8s helm cluster"},
	}
	if prompter.CanUseLiveSelector() {
		selected, err := prompter.ChooseScreen("Upgrade target", choices, platformUpgradeTargetScreenOptions(root))
		if err != nil {
			return "", err
		}
		return choices[selected].Label, nil
	}
	selected, err := prompter.Choose("Upgrade target", choices)
	if err != nil {
		return "", err
	}
	return choices[selected].Label, nil
}

func resolveInteractiveUpgradeDockerComposeOptions(prompter *interactive.Prompter, options *platformUpgradeOptions, defaultVersion string) error {
	versionDefault := upgradeVersionDefault(options, defaultVersion)
	fields := []interactive.Field{
		{Name: "version", Label: "Target version", Value: options.version, Default: versionDefault, Required: true, Description: "Exact semantic release version to upgrade to. Update this CLI first; the CLI owns the templates a release expects.", Example: cliExampleVersion(versionDefault)},
		{Name: "outputDir", Label: "Install directory", Value: options.outputDir, Default: platform.DefaultInstallOutputDir, Required: true, Description: "Directory holding the generated install files and the install lock written by nopsai install.", Example: platform.DefaultInstallOutputDir},
		{Name: "output", Label: "Output format", Value: valueOrDefault(options.output, "text"), Default: "text", Required: true, Description: "Output format for the upgrade plan. Supported values: text, json, yaml.", Example: "text"},
		{Name: "plan", Label: "Plan only", Value: formatYesNo(options.planOnly), Default: "yes", Kind: interactive.FieldBoolean, Description: "Print the plan, changelog, and required actions without changing anything. Leave yes to review first.", Example: "yes"},
		{Name: "run", Label: "Run compose after apply", Value: formatYesNo(options.run), Default: "no", Kind: interactive.FieldBoolean, Description: "After the upgraded files are written, run docker compose up -d. Ignored while Plan only is yes.", Example: "no"},
		{Name: "acceptSeries", Label: "Accept series upgrade", Value: formatYesNo(options.acknowledgeSeries), Default: "no", Kind: interactive.FieldBoolean, Description: "Confirm a reviewed upgrade that crosses a compatibility series. Required before such an upgrade applies.", Example: "no"},
	}
	edited, err := prompter.EditFieldsScreen("Docker Compose upgrade", fields, platformUpgradeFormScreenOptions("Docker Compose", defaultVersion))
	if err != nil {
		return err
	}
	values := fieldValueMap(edited)
	output, err := requireChoiceValue(values["output"], "Output format", "text", "json", "yaml")
	if err != nil {
		return err
	}
	options.version = strings.TrimSpace(values["version"])
	options.outputDir = strings.TrimSpace(values["outputDir"])
	options.output = output
	options.planOnly = parseYesNo(values["plan"])
	options.run = parseYesNo(values["run"])
	options.acknowledgeSeries = parseYesNo(values["acceptSeries"])
	return nil
}

func resolveInteractiveUpgradeKubernetesOptions(prompter *interactive.Prompter, options *platformUpgradeOptions, defaultVersion string) error {
	versionDefault := upgradeVersionDefault(options, defaultVersion)
	fields := []interactive.Field{
		{Name: "version", Label: "Target version", Value: options.version, Default: versionDefault, Required: true, Description: "Exact semantic release version to upgrade to. Update this CLI first; the CLI owns the templates a release expects.", Example: cliExampleVersion(versionDefault)},
		{Name: "lockFile", Label: "Deployment lock", Value: options.lockFile, Default: platform.DefaultLockFile, Required: true, Description: "Deployment lock written by the last install or upgrade. It supplies the release, namespace, chart, and values files.", Example: platform.DefaultLockFile},
		{Name: "releaseName", Label: "Helm release", Value: options.releaseName, Description: "Helm release name. Blank uses the name recorded in the deployment lock.", Example: platform.DefaultReleaseName},
		{Name: "namespace", Label: "Namespace", Value: options.namespace, Description: "Kubernetes namespace. Blank uses the namespace recorded in the deployment lock.", Example: platform.DefaultNamespace},
		{Name: "chart", Label: "Chart reference", Value: options.chartReference, Description: "OCI chart reference. Blank uses the chart recorded in the deployment lock.", Example: "oci://ghcr.io/nopsai/charts/nopsai"},
		{Name: "values", Label: "Values files", Value: strings.Join(options.values, "\n"), Multiline: true, Description: "Helm values files to use instead of the files recorded in the deployment lock. One path per line; blank keeps the recorded files.", Example: "deploy/base.yaml\ndeploy/prod.yaml"},
		{Name: "output", Label: "Output format", Value: valueOrDefault(options.output, "text"), Default: "text", Required: true, Description: "Output format for the upgrade plan. Supported values: text, json, yaml.", Example: "text"},
		{Name: "plan", Label: "Plan only", Value: formatYesNo(options.planOnly), Default: "yes", Kind: interactive.FieldBoolean, Description: "Print the plan, changelog, and required actions without changing anything. Leave yes to review first.", Example: "yes"},
		{Name: "deploy", Label: "Deploy after plan", Value: formatYesNo(options.deploy), Default: "no", Kind: interactive.FieldBoolean, Description: "Run helm upgrade --install after the verified plan and rewrite the deployment lock. Ignored while Plan only is yes.", Example: "no"},
		{Name: "wait", Label: "Wait for rollout", Value: formatYesNo(options.wait), Default: "no", Kind: interactive.FieldBoolean, Description: "Wait for Kubernetes resources to become ready before writing the deployment lock.", Example: "yes"},
		{Name: "acceptSeries", Label: "Accept series upgrade", Value: formatYesNo(options.acknowledgeSeries), Default: "no", Kind: interactive.FieldBoolean, Description: "Confirm a reviewed upgrade that crosses a compatibility series. Required before such an upgrade applies.", Example: "no"},
	}
	edited, err := prompter.EditFieldsScreen("Kubernetes upgrade", fields, platformUpgradeFormScreenOptions("Kubernetes", defaultVersion))
	if err != nil {
		return err
	}
	values := fieldValueMap(edited)
	output, err := requireChoiceValue(values["output"], "Output format", "text", "json", "yaml")
	if err != nil {
		return err
	}
	options.version = strings.TrimSpace(values["version"])
	options.lockFile = strings.TrimSpace(values["lockFile"])
	options.releaseName = strings.TrimSpace(values["releaseName"])
	options.namespace = strings.TrimSpace(values["namespace"])
	options.chartReference = strings.TrimSpace(values["chart"])
	options.values = splitPromptList(values["values"])
	options.output = output
	options.planOnly = parseYesNo(values["plan"])
	options.deploy = parseYesNo(values["deploy"])
	options.wait = parseYesNo(values["wait"])
	options.acknowledgeSeries = parseYesNo(values["acceptSeries"])
	return nil
}

func upgradeVersionDefault(options *platformUpgradeOptions, defaultVersion string) string {
	if version := strings.TrimSpace(options.version); version != "" {
		return version
	}
	return strings.TrimSpace(defaultVersion)
}

func upgradeHeaderLines(root *rootOptions, options *platformUpgradeOptions) []string {
	return []string{
		"CLI: " + valueOrDefault(defaultPlatformVersion(root), "not embedded"),
		"Target version: " + valueOrDefault(strings.TrimSpace(options.version), "not set; version is required"),
	}
}

// upgradePreviewEffects keeps the plan-only case honest: a review run states
// that nothing changes instead of listing the effects of applying.
func upgradePreviewEffects(planOnly bool, effects []string) []string {
	if planOnly {
		return []string{
			"Print the plan, changelog, and required actions. Nothing is changed.",
		}
	}
	return effects
}

func platformUpgradeTargetScreenOptions(root *rootOptions) interactive.ScreenOptions {
	exampleVersion := cliExampleVersion(defaultPlatformVersion(root))
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "Platform", "Upgrade"},
		Title:      "Platform Upgrade",
		Header:     []string{"CLI: " + valueOrDefault(defaultPlatformVersion(root), "not embedded; version is required")},
		LeftTitle:  "Targets",
		RightTitle: "Upgrade Detail",
		LeftWidth:  38,
		Footer: []string{
			"Keys: type filter | Up/Down move | Enter select | Esc platform | Ctrl+C quit",
			"Tip: run nopsai update first. The CLI owns the templates a target release expects.",
		},
		Detail: func(index int, choice interactive.Choice) []string {
			lines := []string{
				choice.Description,
				"",
				"Upgrade reads the lock written by the last install or upgrade, keeps the secrets install generated, and prints the changelog and required actions before anything is applied.",
				"",
				"Noninteractive example",
			}
			switch index {
			case 0:
				lines = append(lines,
					fmt.Sprintf("  nopsai platform upgrade docker-compose --version %s --plan", exampleVersion),
					fmt.Sprintf("  nopsai platform upgrade docker-compose --version %s --run", exampleVersion),
				)
			case 1:
				lines = append(lines,
					fmt.Sprintf("  nopsai platform upgrade kubernetes --version %s --plan", exampleVersion),
					fmt.Sprintf("  nopsai platform upgrade kubernetes --version %s --deploy --wait", exampleVersion),
				)
			}
			return lines
		},
	}
}

func platformUpgradeFormScreenOptions(target, defaultVersion string) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb:  []string{"Home", "Platform", "Upgrade", target},
		Title:       target + " Upgrade",
		Header:      []string{"CLI: " + valueOrDefault(strings.TrimSpace(defaultVersion), "not embedded; version is required")},
		LeftTitle:   "Upgrade Steps",
		RightTitle:  "Values & Details",
		LeftWidth:   58,
		ActionLabel: "Plan upgrade",
		Footer: []string{
			"Edit: type/backspace | Next: Enter or Tab | Submit: Ctrl+S | Back: Esc target | Quit: Ctrl+C",
			"Plan only: yes reviews the upgrade without changing anything. A series upgrade needs Accept series upgrade.",
		},
	}
}

func platformUpgradeResultScreenOptions(options platformUpgradeOptions, target string) interactive.ScreenOptions {
	mode := "apply"
	if options.planOnly || (target == "kubernetes" && !options.deploy) {
		mode = "plan"
	}
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "Platform", "Upgrade", "Result"},
		Title:      "Platform Upgrade Result",
		Header: []string{
			fmt.Sprintf("Target: %s | Version: %s", target, valueOrDefault(strings.TrimSpace(options.version), "not set")),
			fmt.Sprintf("Mode: %s | Output: %s", mode, valueOrDefault(options.output, "text")),
		},
		Footer: []string{"Keys: Up/Down scroll | PgUp/PgDn jump | Home/End | Enter platform | Esc platform | Ctrl+C quit"},
	}
}

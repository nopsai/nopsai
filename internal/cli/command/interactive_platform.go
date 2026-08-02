package command

import (
	"errors"
	"fmt"
	"strings"

	"nopsai/internal/cli/interactive"
	"nopsai/internal/cli/platform"

	"github.com/spf13/cobra"
)

func runInteractivePlatformMenu(command *cobra.Command, options *rootOptions, prompter *interactive.Prompter) error {
	choices := []interactive.Choice{
		{Label: "doctor", Description: "Run local tooling, API readiness, AAA, monitoring, dispatcher, and runner checks", SearchText: "doctor health monitoring aaa metrics dispatcher runner helm kubectl docker"},
		{Label: "release", Description: "Plan or deploy a digest-pinned Kubernetes platform bundle", SearchText: "release kubernetes helm manifest digest values gitops lock deploy"},
		{Label: "back", Description: "Return to the home menu", SearchText: "back home"},
	}
	for {
		state := collectHomeState(command.Context(), options)
		var (
			selected int
			err      error
		)
		if prompter.CanUseLiveSelector() {
			selected, err = prompter.ChooseScreen("Platform", choices, platformMenuScreenOptions(state))
		} else {
			selected, err = prompter.Choose("Platform", choices)
		}
		if errors.Is(err, interactive.ErrBack) {
			return nil
		}
		if err != nil {
			return err
		}
		switch selected {
		case 0:
			if err := runInteractiveDoctor(command, options); err != nil {
				if errors.Is(err, interactive.ErrBack) {
					continue
				}
				return err
			}
		case 1:
			releaseOptions := &platformReleaseOptions{
				version:     defaultPlatformVersion(options),
				releaseName: platform.DefaultReleaseName,
				namespace:   platform.DefaultNamespace,
				output:      "text",
				lockFile:    platform.DefaultLockFile,
				interactive: true,
			}
			if err := runInteractivePlatformRelease(command, options, prompter, releaseOptions); err != nil {
				if errors.Is(err, interactive.ErrBack) {
					continue
				}
				return err
			}
		case 2:
			return nil
		}
	}
}

func platformMenuScreenOptions(state homeState) interactive.ScreenOptions {
	exampleVersion := cliExampleVersion(state.Version)
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "Platform"},
		Title:      "Platform",
		Header:     sessionHeaderLines(state),
		LeftTitle:  "Workflows",
		RightTitle: "Guide",
		LeftWidth:  44,
		Footer:     sessionFooterLines(),
		Detail: func(index int, choice interactive.Choice) []string {
			lines := []string{choice.Description, ""}
			switch index {
			case 0:
				lines = append(lines,
					"Guide: Doctor checks local tools, API setup preflight, metrics, token acceptance, dispatcher monitoring, and runner visibility.",
					"",
					"Example: nopsai platform doctor --output json",
				)
			case 1:
				lines = append(lines,
					"Guide: Release is the advanced GitOps primitive for digest-pinned Helm bundle planning and deployment. Install remains the first-install path.",
					"",
					fmt.Sprintf("Example: nopsai platform release kubernetes --version %s --manifest release-manifest.json --deploy --wait", exampleVersion),
				)
			}
			return lines
		},
	}
}

func runInteractivePlatformRelease(command *cobra.Command, root *rootOptions, prompter *interactive.Prompter, options *platformReleaseOptions) error {
	target, err := chooseInteractiveReleaseTarget(prompter, root)
	if err != nil {
		return err
	}
	return runInteractivePlatformReleaseTarget(command, root, prompter, options, target)
}

func runInteractivePlatformReleaseTarget(command *cobra.Command, root *rootOptions, prompter *interactive.Prompter, options *platformReleaseOptions, target string) error {
	if target != "kubernetes" {
		return fmt.Errorf("unsupported deployment target %q; expected kubernetes", target)
	}
	if prompter.CanUseLiveSelector() {
		if err := resolveLiveKubernetesReleaseOptions(prompter, options, defaultPlatformVersion(root)); err != nil {
			return err
		}
		if strings.TrimSpace(options.version) == "" {
			return fmt.Errorf("--version is required when this CLI build does not embed a release version")
		}
		if err := showInteractiveCommandPreview(prompter, "Platform release command preview", platformReleasePreviewArgs(options), []string{
			"Resolve and verify the release manifest, chart, rendered values, and image pins.",
			"Deploy with Helm only when Deploy after plan is yes.",
		}, commandPreviewScreenOptions([]string{"Home", "Platform", "Release", "Kubernetes", "Preview"}, "Platform Release Preview", []string{"Version: " + valueOrDefault(defaultPlatformVersion(root), "not embedded")})); err != nil {
			return err
		}
		execOptions := *options
		execOptions.interactive = false
		stdout, stderr, releaseErr := captureCommandOutput(command, func() error {
			return executePlatformRelease(command, root, &execOptions, nil)
		})
		resultErr := showInteractiveOutput(prompter, "Platform release", stdout, stderr, releaseErr, platformReleaseResultScreenOptions(root, execOptions))
		if errors.Is(resultErr, interactive.ErrBack) {
			return interactive.ErrBack
		}
		return resultErr
	}
	if err := resolveInteractiveKubernetesOptions(prompter, options, defaultPlatformVersion(root)); err != nil {
		return err
	}
	if strings.TrimSpace(options.version) == "" {
		return fmt.Errorf("--version is required when this CLI build does not embed a release version")
	}
	if err := showInteractiveCommandPreview(prompter, "Platform release command preview", platformReleasePreviewArgs(options), []string{
		"Resolve and verify the release manifest, chart, rendered values, and image pins.",
		"Deploy with Helm only when deployment is confirmed.",
	}, commandPreviewScreenOptions([]string{"Home", "Platform", "Release", "Kubernetes", "Preview"}, "Platform Release Preview", []string{"Version: " + valueOrDefault(defaultPlatformVersion(root), "not embedded")})); err != nil {
		return err
	}
	return executePlatformRelease(command, root, options, prompter)
}

func chooseInteractiveReleaseTarget(prompter *interactive.Prompter, root *rootOptions) (string, error) {
	choices := []interactive.Choice{
		{Label: "kubernetes", Description: "Helm-based NopsAI platform deployment", SearchText: "kubernetes k8s helm cluster gitops"},
	}
	if prompter.CanUseLiveSelector() {
		selected, err := prompter.ChooseScreen("Release target", choices, platformReleaseTargetScreenOptions(root))
		if err != nil {
			return "", err
		}
		return choices[selected].Label, nil
	}
	selected, err := prompter.Choose("Deployment target", choices)
	if err != nil {
		return "", err
	}
	return choices[selected].Label, nil
}

func platformReleaseTargetScreenOptions(root *rootOptions) interactive.ScreenOptions {
	exampleVersion := cliExampleVersion(defaultPlatformVersion(root))
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "Platform", "Release"},
		Title:      "Platform Release",
		Header:     []string{"Version: " + valueOrDefault(defaultPlatformVersion(root), "not embedded; version is required")},
		LeftTitle:  "Targets",
		RightTitle: "Release Detail",
		LeftWidth:  38,
		Footer:     []string{"Keys: type filter | Up/Down move | Enter select | Esc platform | Ctrl+C quit"},
		Detail: func(_ int, choice interactive.Choice) []string {
			return []string{
				choice.Description,
				"",
				"Release plans verify the manifest, CLI compatibility, chart digest, rendered image pins, and GitOps release lock behavior before deploy.",
				"",
				"Noninteractive example",
				fmt.Sprintf("  nopsai platform release kubernetes --version %s --manifest release-manifest.json --values deploy/prod.yaml", exampleVersion),
			}
		},
	}
}

func resolveLiveKubernetesReleaseOptions(prompter *interactive.Prompter, options *platformReleaseOptions, defaultVersion string) error {
	versionDefault := strings.TrimSpace(options.version)
	if versionDefault == "" {
		versionDefault = strings.TrimSpace(defaultVersion)
	}
	fields := []interactive.Field{
		{Name: "version", Label: "Platform version", Value: options.version, Default: versionDefault, Required: true, Description: "Exact semantic platform bundle version to resolve and verify.", Example: cliExampleVersion(versionDefault)},
		{Name: "manifest", Label: "Release manifest", Value: options.manifest, Description: "Release manifest source as a local file path or trusted HTTPS URL. Blank uses embedded/default resolver behavior when available.", Example: "release-manifest.json"},
		{Name: "manifestDigest", Label: "Manifest digest", Value: options.manifestDigest, Description: "Expected SHA-256 digest for the manifest bytes before decoding. Blank trusts the selected source.", Example: "sha256:..."},
		{Name: "values", Label: "Values files", Value: strings.Join(options.values, "\n"), Multiline: true, Description: "Additional Helm values files to merge in GitOps order. Use one path per line.", Example: "deploy/base.yaml\ndeploy/prod.yaml"},
		{Name: "releaseName", Label: "Helm release", Value: options.releaseName, Default: platform.DefaultReleaseName, Required: true, Description: "Helm release name to install or upgrade.", Example: "nopsai"},
		{Name: "namespace", Label: "Namespace", Value: options.namespace, Default: platform.DefaultNamespace, Required: true, Description: "Kubernetes namespace for all rendered and deployed resources.", Example: "nopsai"},
		{Name: "output", Label: "Output format", Value: valueOrDefault(options.output, "text"), Default: "text", Required: true, Description: "Output format for plans and deployment summaries. Supported values: text, json, yaml.", Example: "text"},
		{Name: "wait", Label: "Wait for rollout", Value: formatYesNo(options.wait), Default: "no", Kind: interactive.FieldBoolean, Description: "Wait for Kubernetes resources to become ready before writing the release lock.", Example: "yes"},
		{Name: "lockFile", Label: "Release lock", Value: options.lockFile, Default: platform.DefaultLockFile, Required: true, Description: "GitOps-tracked release lock path written after successful deployment.", Example: ".nopsai/release.lock"},
		{Name: "deploy", Label: "Deploy after plan", Value: formatYesNo(options.deploy), Default: "no", Kind: interactive.FieldBoolean, Description: "Run Helm upgrade --install after the verified plan. Leave no for plan-only output.", Example: "no"},
	}
	edited, err := prompter.EditFieldsScreen("Kubernetes release", fields, platformReleaseFormScreenOptions(defaultVersion))
	if err != nil {
		return err
	}
	values := fieldValueMap(edited)
	output, err := requireChoiceValue(values["output"], "Output format", "text", "json", "yaml")
	if err != nil {
		return err
	}
	options.version = strings.TrimSpace(values["version"])
	options.manifest = strings.TrimSpace(values["manifest"])
	options.manifestDigest = strings.TrimSpace(values["manifestDigest"])
	options.values = splitPromptList(values["values"])
	options.releaseName = strings.TrimSpace(values["releaseName"])
	options.namespace = strings.TrimSpace(values["namespace"])
	options.output = output
	options.wait = parseYesNo(values["wait"])
	options.lockFile = strings.TrimSpace(values["lockFile"])
	options.deploy = parseYesNo(values["deploy"])
	return nil
}

func platformReleaseFormScreenOptions(defaultVersion string) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb:  []string{"Home", "Platform", "Release", "Kubernetes"},
		Title:       "Kubernetes Release",
		Header:      []string{"Version: " + valueOrDefault(strings.TrimSpace(defaultVersion), "not embedded; version is required")},
		LeftTitle:   "Release Steps",
		RightTitle:  "Values & Details",
		LeftWidth:   58,
		ActionLabel: "Plan release",
		Footer: []string{
			"Edit: type/backspace | Next: Enter or Tab | Submit: Ctrl+S | Back: Esc target | Quit: Ctrl+C",
			"Deploy after plan: yes performs Helm upgrade and writes the release lock after success.",
		},
	}
}

func platformReleaseResultScreenOptions(root *rootOptions, options platformReleaseOptions) interactive.ScreenOptions {
	mode := "plan"
	if options.deploy {
		mode = "deploy"
	}
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "Platform", "Release", "Result"},
		Title:      "Platform Release Result",
		Header: []string{
			"Version: " + valueOrDefault(defaultPlatformVersion(root), "not embedded"),
			fmt.Sprintf("Mode: %s | Output: %s | Lock: %s", mode, valueOrDefault(options.output, "text"), releaseLockPath(options)),
		},
		Footer: []string{"Keys: Up/Down scroll | PgUp/PgDn jump | Home/End | Enter platform | Esc platform | Ctrl+C quit"},
	}
}

package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"nopsai/internal/cli/interactive"
	"nopsai/internal/cli/platform"
	"nopsai/pkg/compatibility"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type platformReleaseOptions struct {
	version        string
	manifest       string
	manifestDigest string
	values         []string
	releaseName    string
	namespace      string
	output         string
	wait           bool
	lockFile       string
	deploy         bool
	interactive    bool
}

func newPlatformReleaseCommand(root *rootOptions) *cobra.Command {
	options := &platformReleaseOptions{}
	command := &cobra.Command{
		Use:   "release [kubernetes]",
		Short: "Plan and optionally deploy a versioned platform bundle",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			var prompter *interactive.Prompter
			if options.interactive {
				prompter = interactive.NewPrompter(command.InOrStdin(), command.OutOrStdout())
			}
			target := "kubernetes"
			if len(args) == 1 {
				target = strings.ToLower(strings.TrimSpace(args[0]))
			}
			if options.interactive && len(args) == 0 {
				var err error
				target, err = chooseInteractiveReleaseTarget(prompter, root)
				if err != nil {
					return err
				}
			}
			if target != "kubernetes" {
				return fmt.Errorf("unsupported deployment target %q; expected kubernetes", target)
			}
			if options.interactive {
				if prompter.CanUseLiveSelector() {
					err := runInteractivePlatformReleaseTarget(command, root, prompter, options, target)
					if errors.Is(err, interactive.ErrBack) || errors.Is(err, interactive.ErrCancelled) {
						return nil
					}
					return err
				}
				if err := resolveInteractiveKubernetesOptions(prompter, options, defaultPlatformVersion(root)); err != nil {
					return err
				}
			}
			if strings.TrimSpace(options.version) == "" {
				return fmt.Errorf("--version is required when this CLI build does not embed a release version")
			}
			return executePlatformRelease(command, root, options, prompter)
		},
	}
	addPlatformReleaseFlags(command, options, defaultPlatformVersion(root))
	command.Flags().BoolVar(&options.deploy, "deploy", false, "after a successful plan, run Helm upgrade --install and write the release lock")
	command.Flags().BoolVar(&options.interactive, "interactive", false, "prompt for target, version, manifest, values, namespace, wait, lock, and deployment confirmation")
	return command
}

// defaultPlatformVersion returns the generated version embedded in a released
// CLI. Development builds keep --version mandatory because values such as dev
// are deliberately not deployable semantic versions.
func defaultPlatformVersion(root *rootOptions) string {
	if root == nil {
		return ""
	}
	for _, version := range []string{root.dependencies.BuildInfo.Version, root.dependencies.Version} {
		version = strings.TrimSpace(version)
		if _, err := compatibility.ParseVersion(version); err == nil {
			return version
		}
	}
	return ""
}

func addPlatformReleaseFlags(command *cobra.Command, options *platformReleaseOptions, defaultVersion string) {
	defaultVersion = strings.TrimSpace(defaultVersion)
	versionHelp := "exact semantic platform bundle version to resolve and verify"
	if defaultVersion != "" {
		versionHelp += " (defaults to this CLI build version)"
	}
	command.Flags().StringVar(&options.version, "version", defaultVersion, versionHelp)
	command.Flags().StringVar(&options.manifest, "manifest", "", "release manifest source as a local file path or trusted HTTPS URL")
	command.Flags().StringVar(&options.manifestDigest, "manifest-digest", "", "expected SHA-256 digest for the release manifest bytes")
	command.Flags().StringArrayVarP(&options.values, "values", "f", nil, "Helm values file to merge; repeat in the same order used by GitOps")
	command.Flags().StringVar(&options.releaseName, "release", platform.DefaultReleaseName, "Helm release name to install or upgrade")
	command.Flags().StringVar(&options.namespace, "namespace", platform.DefaultNamespace, "Kubernetes namespace for all rendered and deployed resources")
	command.Flags().StringVarP(&options.output, "output", "o", "text", "output format for plans and deployment summaries: text, json, or yaml")
	command.Flags().BoolVar(&options.wait, "wait", false, "wait for Kubernetes resources to become ready before writing the release lock")
	command.Flags().StringVar(&options.lockFile, "lock-file", platform.DefaultLockFile, "GitOps-tracked release lock path written after successful deployment")
}

func (o platformReleaseOptions) kubernetesOptions() platform.KubernetesOptions {
	return platform.KubernetesOptions{
		Version:                o.version,
		ManifestSource:         o.manifest,
		ExpectedManifestDigest: o.manifestDigest,
		ValuesFiles:            append([]string(nil), o.values...),
		ReleaseName:            o.releaseName,
		Namespace:              o.namespace,
		Wait:                   o.wait,
		LockFile:               o.lockFile,
	}
}

func executePlatformRelease(command *cobra.Command, root *rootOptions, options *platformReleaseOptions, prompter *interactive.Prompter) error {
	output := strings.ToLower(strings.TrimSpace(options.output))
	if options.interactive && output != "" && output != "text" {
		return fmt.Errorf("--interactive supports text output only")
	}
	deployer := kubernetesDeployer(root, command)
	if options.deploy || options.interactive {
		if prompter == nil {
			prompter = interactive.NewPrompter(command.InOrStdin(), command.OutOrStdout())
		}
		plan, _, deployed, err := deployer.PlanAndDeploy(command.Context(), options.kubernetesOptions(), func(plan platform.DeploymentPlan) (bool, error) {
			if !options.interactive {
				return true, nil
			}
			if err := renderDeploymentPlan(command, plan, options.output, false, ""); err != nil {
				return false, err
			}
			if options.deploy {
				return true, nil
			}
			return prompter.Confirm("Deploy this plan now", false)
		})
		if err != nil {
			return err
		}
		if !deployed {
			return nil
		}
		return renderDeploymentPlan(command, plan, options.output, true, releaseLockPath(*options))
	}
	plan, err := deployer.Plan(command.Context(), options.kubernetesOptions())
	if err != nil {
		return err
	}
	return renderDeploymentPlan(command, plan, options.output, false, "")
}

func resolveInteractiveKubernetesOptions(prompter *interactive.Prompter, options *platformReleaseOptions, defaultVersion string) error {
	versionDefault := strings.TrimSpace(options.version)
	if versionDefault == "" {
		versionDefault = strings.TrimSpace(defaultVersion)
	}
	version, err := prompter.AskRequired("Platform version", versionDefault)
	if err != nil {
		return err
	}
	options.version = strings.TrimSpace(version)
	manifest, err := prompter.Ask("Release manifest path or HTTPS URL", options.manifest)
	if err != nil {
		return err
	}
	options.manifest = strings.TrimSpace(manifest)
	manifestDigest, err := prompter.Ask("Expected manifest digest (sha256:, blank to trust source)", options.manifestDigest)
	if err != nil {
		return err
	}
	options.manifestDigest = strings.TrimSpace(manifestDigest)
	values, err := prompter.Ask("Helm values files (comma-separated, blank for none)", strings.Join(options.values, ","))
	if err != nil {
		return err
	}
	options.values = splitPromptList(values)
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
	wait, err := prompter.Confirm("Wait for resources to become ready", options.wait)
	if err != nil {
		return err
	}
	options.wait = wait
	lockFile, err := prompter.Ask("Release lock file", valueOrDefault(options.lockFile, platform.DefaultLockFile))
	if err != nil {
		return err
	}
	options.lockFile = strings.TrimSpace(lockFile)
	return nil
}

func releaseLockPath(options platformReleaseOptions) string {
	lockPath := strings.TrimSpace(options.lockFile)
	if lockPath == "" {
		return platform.DefaultLockFile
	}
	return lockPath
}

func valueOrDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func kubernetesDeployer(root *rootOptions, command *cobra.Command) platform.KubernetesDeployer {
	httpClient := root.dependencies.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: root.timeout}
	}
	return platform.KubernetesDeployer{
		Resolver: releaseManifestResolver(root, httpClient),
		Runner:   root.dependencies.RunProcess,
		CLI:      root.dependencies.BuildInfo,
		Stderr:   command.ErrOrStderr(),
	}
}

func renderDeploymentPlan(command *cobra.Command, plan platform.DeploymentPlan, output string, deployed bool, lockPath string) error {
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "text":
		verb := "Plan"
		if deployed {
			verb = "Deployed"
		}
		if _, err := fmt.Fprintf(command.OutOrStdout(), "%s NopsAI %s as %s in namespace %s\n", verb, plan.Version, plan.ReleaseName, plan.Namespace); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(command.OutOrStdout(), "Manifest: %s\nChart: %s@%s\nValues hash: %s\nDatabase migration: %d (%s)\n",
			plan.ManifestDigest, plan.Chart.Reference, plan.Chart.Digest, plan.ValuesHash, plan.Database.MigrationVersion, plan.Database.RollbackPolicy); err != nil {
			return err
		}
		if deployed {
			_, err := fmt.Fprintf(command.OutOrStdout(), "Release lock: %s\n", lockPath)
			return err
		}
		_, err := fmt.Fprintf(command.OutOrStdout(), "\n--- Rendered Kubernetes manifests ---\n%s", plan.RenderedManifestYAML)
		return err
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

package command

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"nopsai/internal/cli/platform"

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
}

func newPlatformPlanCommand(root *rootOptions) *cobra.Command {
	command := &cobra.Command{Use: "plan", Short: "Preview a versioned platform deployment"}
	command.AddCommand(newPlatformPlanKubernetesCommand(root))
	return command
}

func newPlatformPlanKubernetesCommand(root *rootOptions) *cobra.Command {
	options := &platformReleaseOptions{}
	command := &cobra.Command{
		Use:   "kubernetes",
		Short: "Resolve, verify, and render a Kubernetes platform bundle",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			plan, err := kubernetesDeployer(root, command).Plan(command.Context(), options.kubernetesOptions())
			if err != nil {
				return err
			}
			return renderDeploymentPlan(command, plan, options.output, false, "")
		},
	}
	addPlatformReleaseFlags(command, options, false)
	return command
}

func newPlatformDeployCommand(root *rootOptions) *cobra.Command {
	command := &cobra.Command{Use: "deploy", Short: "Deploy a versioned platform bundle"}
	command.AddCommand(newPlatformDeployKubernetesCommand(root))
	return command
}

func newPlatformDeployKubernetesCommand(root *rootOptions) *cobra.Command {
	options := &platformReleaseOptions{}
	command := &cobra.Command{
		Use:   "kubernetes",
		Short: "Deploy an exact digest-pinned Kubernetes platform bundle",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			plan, _, err := kubernetesDeployer(root, command).Deploy(command.Context(), options.kubernetesOptions())
			if err != nil {
				return err
			}
			lockPath := strings.TrimSpace(options.lockFile)
			if lockPath == "" {
				lockPath = platform.DefaultLockFile
			}
			return renderDeploymentPlan(command, plan, options.output, true, lockPath)
		},
	}
	addPlatformReleaseFlags(command, options, true)
	return command
}

func addPlatformReleaseFlags(command *cobra.Command, options *platformReleaseOptions, deploy bool) {
	command.Flags().StringVar(&options.version, "version", "", "platform bundle version")
	command.Flags().StringVar(&options.manifest, "manifest", "", "local path or HTTPS release manifest override")
	command.Flags().StringVar(&options.manifestDigest, "manifest-digest", "", "expected release manifest SHA-256")
	command.Flags().StringArrayVarP(&options.values, "values", "f", nil, "Helm values file (repeatable)")
	command.Flags().StringVar(&options.releaseName, "release", platform.DefaultReleaseName, "Helm release name")
	command.Flags().StringVar(&options.namespace, "namespace", platform.DefaultNamespace, "Kubernetes namespace")
	command.Flags().StringVarP(&options.output, "output", "o", "text", "output format: text, json, or yaml")
	if deploy {
		command.Flags().BoolVar(&options.wait, "wait", false, "wait for Kubernetes resources to become ready")
		command.Flags().StringVar(&options.lockFile, "lock-file", platform.DefaultLockFile, "deployment release lock path")
	}
	_ = command.MarkFlagRequired("version")
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

func kubernetesDeployer(root *rootOptions, command *cobra.Command) platform.KubernetesDeployer {
	httpClient := root.dependencies.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: root.timeout}
	}
	return platform.KubernetesDeployer{
		Resolver: platform.ManifestResolver{
			HTTPClient:  httpClient,
			URLTemplate: strings.TrimSpace(root.dependencies.Getenv("NOPSAI_RELEASE_MANIFEST_URL_TEMPLATE")),
		},
		Runner: root.dependencies.RunProcess,
		CLI:    root.dependencies.BuildInfo,
		Stderr: command.ErrOrStderr(),
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

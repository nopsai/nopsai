package command

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"nopsai/internal/cli/selfupdate"

	"github.com/spf13/cobra"
)

const defaultUpdateTimeout = 5 * time.Minute

type updateOptions struct {
	version      string
	repository   string
	packageRef   string
	assetBaseURL string
	installPath  string
	dryRun       bool
}

func newUpdateCommand(root *rootOptions) *cobra.Command {
	options := &updateOptions{}
	command := &cobra.Command{
		Use:   "update",
		Short: "Update this CLI to an exact release version",
		Long:  "Update this CLI to an exact release version. Release downloads use a longer timeout than normal API calls; pass --timeout to override it.",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return executeUpdate(command, root, options)
		},
	}
	command.Flags().StringVar(&options.version, "version", "", "exact semantic NopsAI CLI release version to install")
	command.Flags().StringVar(&options.packageRef, "package", "", "OCI package containing public CLI release assets (default: ghcr.io/nopsai/nopsai-cli or $NOPSAI_UPDATE_PACKAGE)")
	command.Flags().StringVar(&options.repository, "repository", "", "legacy GitHub owner/repository for release assets; used only when --package and --asset-base-url are empty")
	command.Flags().StringVar(&options.assetBaseURL, "asset-base-url", "", "HTTPS base URL containing release assets and SHA256SUMS; overrides package and repository sources")
	command.Flags().StringVar(&options.installPath, "install-path", "", "path to replace (default: current nopsai executable)")
	command.Flags().BoolVar(&options.dryRun, "dry-run", false, "print the planned update without downloading or replacing the binary")
	return command
}

func executeUpdate(command *cobra.Command, root *rootOptions, options *updateOptions) error {
	if strings.TrimSpace(options.version) == "" {
		return fmt.Errorf("--version is required")
	}
	timeout, err := updateTimeout(root, command)
	if err != nil {
		return err
	}
	httpClient := root.dependencies.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	} else {
		clone := *httpClient
		clone.Timeout = timeout
		httpClient = &clone
	}
	result, err := (selfupdate.Updater{
		HTTPClient: httpClient,
		Token:      root.dependencies.Getenv("NOPSAI_UPDATE_TOKEN"),
	}).Update(command.Context(), selfupdate.Options{
		Version:      options.version,
		Repository:   updateRepository(root, options.repository),
		PackageRef:   updatePackageRef(root, options.packageRef),
		AssetBaseURL: updateAssetBaseURL(root, options.assetBaseURL),
		InstallPath:  options.installPath,
		DryRun:       options.dryRun,
	})
	if err != nil {
		return err
	}
	return renderUpdateResult(command, result)
}

func updateRepository(root *rootOptions, value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	if root != nil {
		if env := strings.TrimSpace(root.dependencies.Getenv("NOPSAI_UPDATE_GITHUB_REPOSITORY")); env != "" {
			return env
		}
	}
	return ""
}

func updatePackageRef(root *rootOptions, value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	if root != nil {
		return strings.TrimSpace(root.dependencies.Getenv("NOPSAI_UPDATE_PACKAGE"))
	}
	return ""
}

func updateTimeout(root *rootOptions, command *cobra.Command) (time.Duration, error) {
	if root != nil && root.timeout < 0 {
		return 0, fmt.Errorf("timeout cannot be negative")
	}
	if rootTimeoutFlagChanged(command) {
		return root.timeout, nil
	}
	return defaultUpdateTimeout, nil
}

func rootTimeoutFlagChanged(command *cobra.Command) bool {
	if command == nil {
		return false
	}
	root := command.Root()
	if root == nil {
		return false
	}
	flag := root.PersistentFlags().Lookup("timeout")
	return flag != nil && flag.Changed
}

func updateAssetBaseURL(root *rootOptions, value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	if root != nil {
		return strings.TrimSpace(root.dependencies.Getenv("NOPSAI_UPDATE_ASSET_BASE_URL"))
	}
	return ""
}

func renderUpdateResult(command *cobra.Command, result selfupdate.Result) error {
	if !result.Updated {
		_, err := fmt.Fprintf(command.OutOrStdout(), "Would update nopsai to %s\nAsset: %s\nChecksums: %s\nInstall path: %s\n",
			result.Plan.Version, result.Plan.AssetURL, result.Plan.ChecksumURL, result.Plan.InstallPath)
		return err
	}
	_, err := fmt.Fprintf(command.OutOrStdout(), "Updated nopsai to %s\nAsset: %s\nSHA256: %s\nInstalled: %s\n",
		result.Plan.Version, result.Plan.AssetURL, result.Digest, result.Plan.InstallPath)
	return err
}

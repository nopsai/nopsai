package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"nopsai/internal/cli/interactive"

	"github.com/spf13/cobra"
)

// runInteractiveUpdate exposes CLI self-update in the interactive console. It
// defaults to a dry run because an update replaces the running binary, so a
// bare pass through the form reports the plan instead of performing it.
func runInteractiveUpdate(command *cobra.Command, root *rootOptions, prompter *interactive.Prompter) error {
	options := &updateOptions{version: defaultPlatformVersion(root), dryRun: true}
	state := collectHomeState(command.Context(), root)
	if prompter.CanUseLiveSelector() {
		if err := resolveInteractiveUpdateOptions(prompter, options, state); err != nil {
			return err
		}
	} else {
		version, err := prompter.AskRequired("CLI release version", options.version)
		if err != nil {
			return err
		}
		options.version = strings.TrimSpace(version)
		dryRun, err := prompter.Confirm("Dry run only", true)
		if err != nil {
			return err
		}
		options.dryRun = dryRun
	}
	if err := showInteractiveCommandPreview(prompter, "Update command preview", updatePreviewArgs(options), updatePreviewEffects(options.dryRun),
		commandPreviewScreenOptions([]string{"Home", "Update", "Preview"}, "Update Preview", sessionHeaderLines(state))); err != nil {
		return err
	}
	if !prompter.CanUseLiveSelector() {
		return executeUpdate(command, root, options)
	}
	stdout, stderr, updateErr := captureCommandOutput(command, func() error {
		return executeUpdate(command, root, options)
	})
	resultErr := showInteractiveOutput(prompter, "Update", stdout, stderr, updateErr, updateResultScreenOptions(options, state))
	if errors.Is(resultErr, interactive.ErrBack) {
		return nil
	}
	return resultErr
}

func resolveInteractiveUpdateOptions(prompter *interactive.Prompter, options *updateOptions, state homeState) error {
	versionDefault := strings.TrimSpace(options.version)
	fields := []interactive.Field{
		{Name: "version", Label: "CLI version", Value: options.version, Default: versionDefault, Required: true, Description: "Exact semantic NopsAI CLI release version to install.", Example: cliExampleVersion(versionDefault)},
		{Name: "dryRun", Label: "Dry run", Value: formatYesNo(options.dryRun), Default: "yes", Kind: interactive.FieldBoolean, Description: "Print the planned update without downloading or replacing the binary. Leave yes to review first.", Example: "yes"},
		{Name: "packageRef", Label: "Release package", Value: options.packageRef, Description: "OCI package containing public CLI release assets. Blank uses the default package or $NOPSAI_UPDATE_PACKAGE.", Example: "ghcr.io/nopsai/nopsai-cli"},
		{Name: "assetBaseURL", Label: "Asset base URL", Value: options.assetBaseURL, Description: "HTTPS base URL hosting release assets and SHA256SUMS. Overrides the package and repository sources.", Example: "https://releases.example.com/nopsai"},
		{Name: "repository", Label: "Release repository", Value: options.repository, Description: "Legacy GitHub owner/repository for release assets. Used only when package and asset base URL are blank.", Example: "nopsai/nopsai"},
		{Name: "installPath", Label: "Install path", Value: options.installPath, Description: "Path to replace. Blank replaces the current nopsai executable.", Example: "/usr/local/bin/nopsai"},
	}
	edited, err := prompter.EditFieldsScreen("Update CLI", fields, updateFormScreenOptions(state))
	if err != nil {
		return err
	}
	values := fieldValueMap(edited)
	options.version = strings.TrimSpace(values["version"])
	options.dryRun = parseYesNo(values["dryRun"])
	options.packageRef = strings.TrimSpace(values["packageRef"])
	options.assetBaseURL = strings.TrimSpace(values["assetBaseURL"])
	options.repository = strings.TrimSpace(values["repository"])
	options.installPath = strings.TrimSpace(values["installPath"])
	return nil
}

func updatePreviewArgs(options *updateOptions) []string {
	args := []string{"nopsai", "update", "--version", strings.TrimSpace(options.version)}
	if strings.TrimSpace(options.packageRef) != "" {
		args = append(args, "--package", strings.TrimSpace(options.packageRef))
	}
	if strings.TrimSpace(options.assetBaseURL) != "" {
		args = append(args, "--asset-base-url", strings.TrimSpace(options.assetBaseURL))
	}
	if strings.TrimSpace(options.repository) != "" {
		args = append(args, "--repository", strings.TrimSpace(options.repository))
	}
	if strings.TrimSpace(options.installPath) != "" {
		args = append(args, "--install-path", strings.TrimSpace(options.installPath))
	}
	if options.dryRun {
		args = append(args, "--dry-run")
	}
	return args
}

func updatePreviewEffects(dryRun bool) []string {
	if dryRun {
		return []string{"Resolve and verify the release assets and print the planned update. The binary is not replaced."}
	}
	return []string{
		"Download the release assets, verify their checksums, and replace the nopsai executable in place.",
		"Update the CLI before upgrading a platform: the CLI owns the templates a release expects.",
	}
}

func updateFormScreenOptions(state homeState) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb:  []string{"Home", "Update"},
		Title:       "Update CLI",
		Header:      append([]string{"Installed CLI: " + valueOrDefault(state.Version, "dev")}, sessionHeaderLines(state)...),
		LeftTitle:   "Update Steps",
		RightTitle:  "Values & Details",
		LeftWidth:   58,
		ActionLabel: "Plan update",
		Footer: []string{
			"Edit: type/backspace | Next: Enter or Tab | Submit: Ctrl+S | Back: Esc home | Quit: Ctrl+C",
			"Dry run: yes reports the planned update without replacing the binary.",
		},
	}
}

func updateResultScreenOptions(options *updateOptions, state homeState) interactive.ScreenOptions {
	mode := "update"
	if options.dryRun {
		mode = "dry run"
	}
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "Update", "Result"},
		Title:      "Update Result",
		Header: []string{
			"Installed CLI: " + valueOrDefault(state.Version, "dev"),
			"Mode: " + mode + " | Target: " + valueOrDefault(strings.TrimSpace(options.version), "not set"),
		},
		Footer: []string{"Keys: Up/Down scroll | PgUp/PgDn jump | Home/End | Enter home | Esc home | Ctrl+C quit"},
	}
}

// runInteractiveLicense shows the proprietary notice on a scrollable screen so
// the interactive console covers the same ground as `nopsai license`.
func runInteractiveLicense(command *cobra.Command, prompter *interactive.Prompter, state homeState) error {
	if err := showInteractiveCommandPreview(prompter, "License command preview", []string{"nopsai", "license"}, []string{
		"Print the NopsAI proprietary software notice.",
		"Show what this installation is entitled to run, the same as `nopsai license status`.",
	}, commandPreviewScreenOptions([]string{"Home", "License", "Preview"}, "License Preview", sessionHeaderLines(state))); err != nil {
		return err
	}
	if !prompter.CanUseLiveSelector() {
		return renderLicenseNotice(command)
	}

	// The console must cover the same ground as the CLI, so the entitlement is
	// shown here rather than only under `nopsai license status`. It is
	// best-effort: an unreachable or unauthenticated API leaves the notice
	// readable instead of failing the screen.
	lines := interactiveLicenseEntitlementLines(command, state)
	lines = append(lines, strings.Split(proprietaryLicenseNotice, "\n")...)

	err := prompter.ShowTextScreen("License", lines, interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "License"},
		Title:      "License",
		Header:     sessionHeaderLines(state),
		Footer:     []string{"Keys: Up/Down scroll | PgUp/PgDn jump | Home/End | Enter home | Esc home | Ctrl+C quit"},
	})
	if errors.Is(err, interactive.ErrBack) {
		return nil
	}
	return err
}

// interactiveLicenseEntitlementLines renders the entitlement summary shown above
// the notice in the interactive console. It never returns an error: a console
// that cannot reach the API must still be able to display the notice.
func interactiveLicenseEntitlementLines(command *cobra.Command, state homeState) []string {
	if state.Session.Client == nil {
		return []string{"Entitlement: not available without an API context.", ""}
	}
	request, err := state.Session.Client.NewRequest(http.MethodGet, "/v1/system/license", nil)
	if err != nil {
		return []string{"Entitlement: could not be requested.", ""}
	}
	response, err := state.Session.Client.Do(request.WithContext(command.Context()))
	if err != nil {
		return []string{"Entitlement: API unreachable.", ""}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || response.StatusCode != http.StatusOK {
		return []string{"Entitlement: not readable with the current credentials.", ""}
	}
	var status licenseStatusResponse
	if err := json.Unmarshal(body, &status); err != nil {
		return []string{"Entitlement: response could not be decoded.", ""}
	}

	lines := []string{}
	if status.Licensed {
		lines = append(lines, fmt.Sprintf("Entitlement: licensed to %s (%s tier)", status.Licensee, status.Tier))
		if status.ExpiresAt != "" {
			lines = append(lines, "Expires:     "+status.ExpiresAt)
		}
	} else {
		lines = append(lines, fmt.Sprintf("Entitlement: not licensed (%s tier)", status.Tier))
		if status.Reason != "" {
			lines = append(lines, "Reason:      "+status.Reason)
		}
	}
	lines = append(lines,
		"Users:       "+limitLine(status.Usage.Users, status.Limits.MaxUsers),
		"Teams:       "+limitLine(status.Usage.Teams, status.Limits.MaxTeams),
		"Runs:        "+ceilingLine(status.Limits.MaxConcurrentRuns),
		"",
	)
	return lines
}

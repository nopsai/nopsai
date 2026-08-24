package command

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

const licenseNotice = `NopsAI Licence

Copyright (c) 2026 Hossein Yousefi. All rights reserved.

NopsAI is licensed under the PolyForm Noncommercial License 1.0.0. It is free
for any noncommercial purpose: personal use, study, research, experimentation,
hobby projects, and use by charitable organizations, educational institutions,
public research organizations, public safety or health organizations,
environmental protection organizations and government institutions.

Commercial use is not granted by this licence. Using NopsAI in or for a
business, or for any other commercial purpose, requires a separate
written agreement.

Third-party components remain subject to their applicable licence terms. See the
LICENSE and THIRD_PARTY_NOTICES.md files supplied with the NopsAI release.

Commercial licensing enquiries: contact@nopsai.com`

func newLicenseCommand(options *rootOptions) *cobra.Command {
	command := &cobra.Command{
		Use:   "license",
		Short: "Show the NopsAI licence notice",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return renderLicenseNotice(command)
		},
	}
	command.AddCommand(newLicenseStatusCommand(options))
	return command
}

type licenseStatusResponse struct {
	Licensed  bool     `json:"licensed"`
	Tier      string   `json:"tier"`
	Licensee  string   `json:"licensee"`
	LicenseID string   `json:"license_id"`
	ExpiresAt string   `json:"expires_at"`
	Reason    string   `json:"reason"`
	Features  []string `json:"features"`
	Limits    struct {
		MaxUsers          int `json:"max_users"`
		MaxTeams          int `json:"max_teams"`
		MaxConcurrentRuns int `json:"max_concurrent_runs"`
	} `json:"limits"`
	Usage struct {
		Users int `json:"users"`
		Teams int `json:"teams"`
	} `json:"usage"`
}

func newLicenseStatusCommand(options *rootOptions) *cobra.Command {
	var output string
	command := &cobra.Command{
		Use:   "status",
		Short: "Show what this installation is entitled to run",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			session, err := options.resolveSession(true)
			if err != nil {
				return err
			}
			request, err := session.Client.NewRequest(http.MethodGet, "/v1/system/license", nil)
			if err != nil {
				return err
			}
			response, err := session.Client.Do(request.WithContext(command.Context()))
			if err != nil {
				return fmt.Errorf("request failed: %w", err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
			if err != nil {
				return fmt.Errorf("read response: %w", err)
			}
			if response.StatusCode != http.StatusOK {
				return fmt.Errorf("licence status request failed (%d): %s", response.StatusCode, strings.TrimSpace(string(body)))
			}
			var status licenseStatusResponse
			if err := json.Unmarshal(body, &status); err != nil {
				return fmt.Errorf("decode licence status: %w", err)
			}
			return renderLicenseStatus(command, status, output)
		},
	}
	command.Flags().StringVarP(&output, "output", "o", "text", "output format: text or json")
	return command
}

func renderLicenseStatus(command *cobra.Command, status licenseStatusResponse, output string) error {
	if strings.EqualFold(strings.TrimSpace(output), "json") {
		encoder := json.NewEncoder(command.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(status)
	}

	writer := command.OutOrStdout()
	if status.Licensed {
		if _, err := fmt.Fprintf(writer, "Commercially licensed to %s (%s tier)\n", status.Licensee, status.Tier); err != nil {
			return err
		}
		if status.LicenseID != "" {
			fmt.Fprintf(writer, "Licence ID:  %s\n", status.LicenseID)
		}
		if status.ExpiresAt != "" {
			fmt.Fprintf(writer, "Expires:     %s\n", status.ExpiresAt)
		}
	} else {
		// Not a fault state: the non-commercial licence is a complete grant, so
		// this reads as a position rather than as something to fix.
		if _, err := fmt.Fprintf(writer, "Non-commercial use (%s)\n", status.Tier); err != nil {
			return err
		}
		if status.Reason != "" {
			fmt.Fprintf(writer, "Reason:      %s\n", status.Reason)
		}
	}
	if len(status.Features) > 0 {
		fmt.Fprintf(writer, "Features:    %s\n", strings.Join(status.Features, ", "))
	}
	fmt.Fprintf(writer, "Users:       %s\n", limitLine(status.Usage.Users, status.Limits.MaxUsers))
	fmt.Fprintf(writer, "Teams:       %s\n", limitLine(status.Usage.Teams, status.Limits.MaxTeams))
	fmt.Fprintf(writer, "Runs:        %s\n", ceilingLine(status.Limits.MaxConcurrentRuns))
	return nil
}

// A limit of zero means unlimited, so it must never be printed as a ceiling of
// nothing.
func limitLine(current, limit int) string {
	if limit <= 0 {
		return fmt.Sprintf("%d of unlimited", current)
	}
	return fmt.Sprintf("%d of %d", current, limit)
}

func ceilingLine(limit int) string {
	if limit <= 0 {
		return "unlimited"
	}
	return fmt.Sprintf("up to %d concurrent", limit)
}

func renderLicenseNotice(command *cobra.Command) error {
	_, err := fmt.Fprintln(command.OutOrStdout(), licenseNotice)
	return err
}

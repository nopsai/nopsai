package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"nopsai/internal/cli/platform"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newPlatformCommand(options *rootOptions) *cobra.Command {
	command := &cobra.Command{Use: "platform", Short: "Operate NopsAI deployments"}
	command.AddCommand(newPlatformDoctorCommand(options))
	command.AddCommand(newPlatformPlanCommand(options))
	command.AddCommand(newPlatformDeployCommand(options))
	return command
}

func newPlatformDoctorCommand(options *rootOptions) *cobra.Command {
	var output string
	command := &cobra.Command{
		Use:   "doctor",
		Short: "Check local tooling, API readiness, AAA, and monitoring",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			session, err := options.resolveSession(false)
			if err != nil {
				return err
			}
			checks := (platform.Doctor{
				Client:          session.Client,
				TokenConfigured: strings.TrimSpace(session.Token) != "",
				LookPath:        options.dependencies.LookPath,
				RunCommand:      options.dependencies.RunCommand,
			}).Run(command.Context())
			if err := renderDoctor(command, checks, output); err != nil {
				return err
			}
			if platform.HasErrors(checks) {
				return errors.New("one or more doctor checks failed")
			}
			return nil
		},
	}
	command.Flags().StringVarP(&output, "output", "o", "text", "output format: text, json, or yaml")
	return command
}

func renderDoctor(command *cobra.Command, checks []platform.Check, output string) error {
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "text":
		for _, check := range checks {
			if _, err := fmt.Fprintf(command.OutOrStdout(), "%-7s %-24s %s\n", strings.ToUpper(string(check.Severity)), check.Name, check.Message); err != nil {
				return err
			}
		}
		return nil
	case "json":
		encoder := json.NewEncoder(command.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(checks)
	case "yaml":
		encoder := yaml.NewEncoder(command.OutOrStdout())
		defer encoder.Close()
		encoder.SetIndent(2)
		return encoder.Encode(checks)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

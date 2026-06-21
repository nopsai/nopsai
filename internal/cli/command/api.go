package command

import "github.com/spf13/cobra"

func newAPICommand(options *rootOptions) *cobra.Command {
	command := &cobra.Command{Use: "api", Short: "Access the NopsAI REST API"}
	command.AddCommand(newAPIRequestCommand(options))
	command.AddCommand(newAPICallCommand(options))
	command.AddCommand(newAPIRoutesCommand())
	command.AddCommand(newAPIDescribeCommand())
	return command
}

package command

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

func newContextCommand(options *rootOptions) *cobra.Command {
	command := &cobra.Command{Use: "context", Short: "Manage API contexts"}
	command.AddCommand(newContextAddCommand(options))
	command.AddCommand(newContextUseCommand(options))
	command.AddCommand(newContextListCommand(options))
	command.AddCommand(newContextCurrentCommand(options))
	command.AddCommand(newContextDeleteCommand(options))
	return command
}

func newContextAddCommand(options *rootOptions) *cobra.Command {
	var apiURL string
	command := &cobra.Command{
		Use:   "add NAME",
		Short: "Add or update an API context",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			store, err := options.store()
			if err != nil {
				return err
			}
			ctx, err := store.AddContext(args[0], apiURL)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "Context %q configured for %s\n", args[0], ctx.API)
			return err
		},
	}
	command.Flags().StringVar(&apiURL, "api", "", "NopsAI API URL")
	_ = command.MarkFlagRequired("api")
	return command
}

func newContextUseCommand(options *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "use NAME",
		Short: "Select the current context",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			store, err := options.store()
			if err != nil {
				return err
			}
			if err := store.UseContext(args[0]); err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "Current context is now %q\n", args[0])
			return err
		},
	}
}

func newContextListCommand(options *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured contexts",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			store, err := options.store()
			if err != nil {
				return err
			}
			cfg, err := store.Load()
			if err != nil {
				return err
			}
			names := make([]string, 0, len(cfg.Contexts))
			for name := range cfg.Contexts {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				current := " "
				if name == cfg.CurrentContext {
					current = "*"
				}
				if _, err := fmt.Fprintf(command.OutOrStdout(), "%s %s\t%s\n", current, name, cfg.Contexts[name].API); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newContextCurrentCommand(options *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Print the current context",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			store, err := options.store()
			if err != nil {
				return err
			}
			name, ctx, err := store.ResolveContext("")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "%s\t%s\n", name, ctx.API)
			return err
		},
	}
}

func newContextDeleteCommand(options *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "delete NAME",
		Short: "Delete a context and its locally stored credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			store, err := options.store()
			if err != nil {
				return err
			}
			if err := store.DeleteContext(args[0]); err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "Context %q deleted\n", args[0])
			return err
		},
	}
}

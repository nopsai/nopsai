package command

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"nopsai/internal/cli/interactive"

	"github.com/spf13/cobra"
)

var supportedCompletionShells = []string{"bash", "zsh", "fish", "powershell"}

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	var outputDir string
	var stdout bool
	command := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate a shell completion file with install instructions",
		Long: "Generate a shell completion file for nopsai. By default the file is written to the current directory " +
			"and the command prints copy instructions for the selected shell. Use --stdout when packaging completion scripts.",
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: supportedCompletionShells,
		RunE: func(command *cobra.Command, args []string) error {
			shell := ""
			if len(args) == 1 {
				shell = args[0]
			} else {
				selected, err := interactive.NewPrompter(command.InOrStdin(), command.OutOrStdout()).Choose("Shell", completionShellChoices())
				if err != nil {
					return err
				}
				shell = supportedCompletionShells[selected]
			}
			shell = strings.ToLower(strings.TrimSpace(shell))
			if !completionShellSupported(shell) {
				return fmt.Errorf("unsupported shell %q; expected bash, zsh, fish, or powershell", shell)
			}
			var contents bytes.Buffer
			if err := generateCompletion(root, shell, &contents); err != nil {
				return err
			}
			if stdout {
				_, err := command.OutOrStdout().Write(contents.Bytes())
				return err
			}
			path, err := writeCompletionFile(outputDir, shell, contents.Bytes())
			if err != nil {
				return err
			}
			return renderCompletionInstructions(command, shell, path)
		},
	}
	command.Flags().StringVar(&outputDir, "output-dir", ".", "directory where the generated completion file is written")
	command.Flags().BoolVar(&stdout, "stdout", false, "write the completion script to stdout instead of creating a file")
	return command
}

func completionShellChoices() []interactive.Choice {
	choices := make([]interactive.Choice, 0, len(supportedCompletionShells))
	for _, shell := range supportedCompletionShells {
		choices = append(choices, interactive.Choice{Label: shell, SearchText: shell})
	}
	return choices
}

func completionShellSupported(shell string) bool {
	for _, supported := range supportedCompletionShells {
		if shell == supported {
			return true
		}
	}
	return false
}

func generateCompletion(root *cobra.Command, shell string, output *bytes.Buffer) error {
	switch shell {
	case "bash":
		return root.GenBashCompletionV2(output, true)
	case "zsh":
		return root.GenZshCompletion(output)
	case "fish":
		return root.GenFishCompletion(output, true)
	case "powershell":
		return root.GenPowerShellCompletionWithDesc(output)
	default:
		return fmt.Errorf("unsupported shell %q", shell)
	}
}

func writeCompletionFile(outputDir, shell string, contents []byte) (string, error) {
	outputDir = strings.TrimSpace(outputDir)
	if outputDir == "" {
		outputDir = "."
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create completion output directory: %w", err)
	}
	path := filepath.Join(outputDir, completionFilename(shell))
	if err := os.WriteFile(path, contents, 0o644); err != nil { // #nosec G306 -- shell completion files contain no secrets.
		return "", fmt.Errorf("write completion file: %w", err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path, nil
	}
	return absolute, nil
}

func completionFilename(shell string) string {
	switch shell {
	case "bash":
		return "nopsai.bash"
	case "zsh":
		return "_nopsai"
	case "fish":
		return "nopsai.fish"
	case "powershell":
		return "nopsai.ps1"
	default:
		return "nopsai." + shell
	}
}

func renderCompletionInstructions(command *cobra.Command, shell, path string) error {
	if _, err := fmt.Fprintf(command.OutOrStdout(), "Wrote %s completion: %s\n\n", shell, path); err != nil {
		return err
	}
	lines := completionInstallInstructions(shell, path)
	for _, line := range lines {
		if _, err := fmt.Fprintln(command.OutOrStdout(), line); err != nil {
			return err
		}
	}
	return nil
}

func completionInstallInstructions(shell, path string) []string {
	switch shell {
	case "bash":
		return []string{
			"Install for Linux bash:",
			"  mkdir -p ~/.local/share/bash-completion/completions",
			fmt.Sprintf("  cp %s ~/.local/share/bash-completion/completions/nopsai", shellQuote(path)),
			"",
			"Install for macOS bash with Homebrew bash-completion:",
			"  mkdir -p \"$(brew --prefix)/etc/bash_completion.d\"",
			fmt.Sprintf("  cp %s \"$(brew --prefix)/etc/bash_completion.d/nopsai\"", shellQuote(path)),
		}
	case "zsh":
		return []string{
			"Install for zsh:",
			"  mkdir -p ~/.zsh/completions",
			fmt.Sprintf("  cp %s ~/.zsh/completions/_nopsai", shellQuote(path)),
			"  echo 'fpath=(~/.zsh/completions $fpath)' >> ~/.zshrc",
			"  echo 'autoload -Uz compinit && compinit' >> ~/.zshrc",
		}
	case "fish":
		return []string{
			"Install for fish:",
			"  mkdir -p ~/.config/fish/completions",
			fmt.Sprintf("  cp %s ~/.config/fish/completions/nopsai.fish", shellQuote(path)),
		}
	case "powershell":
		return []string{
			"Install for PowerShell:",
			"  New-Item -ItemType Directory -Force (Split-Path $PROFILE)",
			fmt.Sprintf("  Copy-Item %s (Join-Path (Split-Path $PROFILE) 'nopsai.ps1')", powershellQuote(path)),
			"  Add-Content $PROFILE '. (Join-Path (Split-Path $PROFILE) ''nopsai.ps1'')'",
		}
	default:
		return []string{"Copy the generated file to your shell completion directory."}
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func powershellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

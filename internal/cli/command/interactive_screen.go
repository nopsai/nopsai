package command

import (
	"bytes"
	"fmt"
	"strings"

	"nopsai/internal/cli/interactive"

	"github.com/spf13/cobra"
)

func captureCommandOutput(command *cobra.Command, run func() error) (string, string, error) {
	oldOut := command.OutOrStdout()
	oldErr := command.ErrOrStderr()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	err := run()
	command.SetOut(oldOut)
	command.SetErr(oldErr)
	return stdout.String(), stderr.String(), err
}

func outputScreenLines(title, stdout, stderr string, err error) []string {
	lines := []string{title}
	if err != nil {
		lines = append(lines, "Status: error", "Error: "+err.Error())
	} else {
		lines = append(lines, "Status: ok")
	}
	if strings.TrimSpace(stderr) != "" {
		lines = append(lines, "", "Diagnostics")
		lines = append(lines, splitOutputLines(stderr)...)
	}
	if strings.TrimSpace(stdout) != "" {
		lines = append(lines, "", "Output")
		lines = append(lines, splitOutputLines(stdout)...)
	} else if err == nil {
		lines = append(lines, "", "Output", "(no response body)")
	}
	return lines
}

func splitOutputLines(value string) []string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.TrimRight(value, "\n")
	if value == "" {
		return nil
	}
	return strings.Split(value, "\n")
}

func showInteractiveOutput(prompter *interactive.Prompter, title string, stdout string, stderr string, err error, options interactive.ScreenOptions) error {
	showErr := prompter.ShowTextScreen(title, outputScreenLines(title, stdout, stderr, err), options)
	if showErr != nil {
		return showErr
	}
	return nil
}

func helpScreenLines(command *cobra.Command) []string {
	usage := strings.TrimRight(command.Root().UsageString(), "\n")
	if usage == "" {
		usage = strings.TrimRight(command.UsageString(), "\n")
	}
	lines := []string{"NopsAI command help"}
	if usage != "" {
		lines = append(lines, "")
		lines = append(lines, splitOutputLines(usage)...)
	} else {
		lines = append(lines, "", fmt.Sprintf("Use `%s --help` for command help.", command.CommandPath()))
	}
	return lines
}

func parseYesNo(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}

func formatYesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

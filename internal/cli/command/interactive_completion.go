package command

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"nopsai/internal/cli/interactive"

	"github.com/spf13/cobra"
)

func runInteractiveCompletion(command *cobra.Command, prompter *interactive.Prompter) error {
	shell, err := chooseInteractiveCompletionShell(prompter)
	if err != nil {
		return err
	}
	fields := []interactive.Field{
		{Name: "outputDir", Label: "Output directory", Value: ".", Default: ".", Required: true, Description: "Directory where the generated completion file is written when Stdout is no.", Example: "./completion"},
		{Name: "stdout", Label: "Write to stdout", Value: "no", Default: "no", Kind: interactive.FieldBoolean, Description: "Write the completion script to stdout instead of creating a file. Useful for packaging scripts.", Example: "no"},
		{Name: "generate", Label: "Generate completion", Value: "yes", Default: "yes", Kind: interactive.FieldBoolean, Description: "Final review gate. Change to no or press Esc to return home without generating.", Example: "yes"},
	}
	edited, err := prompter.EditFieldsScreen("Completion: "+shell, fields, completionFormScreenOptions(shell))
	if err != nil {
		return err
	}
	values := fieldValueMap(edited)
	if !parseYesNo(values["generate"]) {
		return interactive.ErrBack
	}
	var contents bytes.Buffer
	if err := generateCompletion(command.Root(), shell, &contents); err != nil {
		return err
	}
	var (
		stdout string
		stderr string
	)
	if parseYesNo(values["stdout"]) {
		stdout = contents.String()
	} else {
		path, err := writeCompletionFile(values["outputDir"], shell, contents.Bytes())
		if err != nil {
			return err
		}
		var output bytes.Buffer
		if err := renderCompletionInstructionsTo(&output, shell, path); err != nil {
			return err
		}
		stdout = output.String()
	}
	resultErr := prompter.ShowTextScreen("Completion", outputScreenLines("Completion: "+shell, stdout, stderr, nil), completionResultScreenOptions(shell))
	if errors.Is(resultErr, interactive.ErrBack) {
		return interactive.ErrBack
	}
	return resultErr
}

func chooseInteractiveCompletionShell(prompter *interactive.Prompter) (string, error) {
	choices := completionShellChoices()
	var (
		selected int
		err      error
	)
	if prompter.CanUseLiveSelector() {
		selected, err = prompter.ChooseScreen("Completion shell", choices, completionShellScreenOptions())
	} else {
		selected, err = prompter.Choose("Shell", choices)
	}
	if err != nil {
		return "", err
	}
	return supportedCompletionShells[selected], nil
}

func completionShellScreenOptions() interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Title:      "Completion",
		Header:     []string{"Generate shell completion files without modifying shell startup files automatically."},
		LeftTitle:  "Shells",
		RightTitle: "Install Guide",
		LeftWidth:  36,
		Footer:     []string{"Keys: type filter | Up/Down move | Enter select | Esc home | Ctrl+C quit"},
		Detail: func(_ int, choice interactive.Choice) []string {
			shell := strings.TrimSpace(choice.Label)
			lines := []string{
				choice.Description,
				"",
				fmt.Sprintf("File: %s", completionFilename(shell)),
				"",
				"Instructions are printed after generation. Use stdout only for packaging workflows that need raw script bytes.",
			}
			return lines
		},
	}
}

func completionFormScreenOptions(shell string) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Title:       "Completion",
		Header:      []string{"Shell: " + shell + " | File: " + completionFilename(shell)},
		LeftTitle:   "Completion Steps",
		RightTitle:  "Values & Details",
		LeftWidth:   46,
		ActionLabel: "Generate completion",
		Footer:      []string{"Edit: type/backspace | Next: Enter or Tab | Submit: Ctrl+S | Back: Esc shells | Quit: Ctrl+C"},
	}
}

func completionResultScreenOptions(shell string) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Title:  "Completion Result",
		Header: []string{"Shell: " + shell + " | Startup files are not modified automatically."},
		Footer: []string{"Keys: Up/Down scroll | PgUp/PgDn jump | Home/End | Enter home | Esc home | Ctrl+C quit"},
	}
}

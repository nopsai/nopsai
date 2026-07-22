package command

import (
	"errors"
	"fmt"

	"nopsai/internal/cli/interactive"

	"github.com/spf13/cobra"
)

func runInteractiveAuthMenu(command *cobra.Command, options *rootOptions, prompter *interactive.Prompter) error {
	choices := []interactive.Choice{
		{Label: "login token", Description: "Verify and store a token for the selected context", SearchText: "login token nopat nopsat jwt authenticate aaa"},
		{Label: "logout", Description: "Remove the locally stored credential for the selected context", SearchText: "logout remove credential token context"},
		{Label: "back", Description: "Return to the home menu", SearchText: "back home"},
	}
	for {
		state := collectHomeState(command.Context(), options)
		var (
			selected int
			err      error
		)
		if prompter.CanUseLiveSelector() {
			selected, err = prompter.ChooseScreen("Authentication", choices, authMenuScreenOptions(state))
		} else {
			selected, err = prompter.Choose("Authentication", choices)
		}
		if errors.Is(err, interactive.ErrBack) {
			return nil
		}
		if err != nil {
			return err
		}
		switch selected {
		case 0:
			if err := runInteractiveLogin(command, options, prompter); err != nil {
				if errors.Is(err, interactive.ErrBack) {
					continue
				}
				return err
			}
		case 1:
			if err := runInteractiveLogout(command, options, prompter); err != nil {
				if errors.Is(err, interactive.ErrBack) {
					continue
				}
				return err
			}
		case 2:
			return nil
		}
	}
}

func authMenuScreenOptions(state homeState) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "Authentication"},
		Title:      "Authentication",
		Header:     sessionHeaderLines(state),
		LeftTitle:  "Actions",
		RightTitle: "Guide",
		LeftWidth:  42,
		Footer:     sessionFooterLines(),
		Detail: func(index int, choice interactive.Choice) []string {
			lines := []string{choice.Description, ""}
			switch index {
			case 0:
				lines = append(lines,
					"Guide: Reads NOPSAI_TOKEN first; otherwise reads stdin and hides terminal input. The token is verified with GET /v1/auth/me before local storage.",
					"",
					"Example: NOPSAI_TOKEN=nopat_<secret> nopsai login --token",
				)
			case 1:
				lines = append(lines,
					"Guide: Removes only the local credential for the selected context. Server-side personal or service-account tokens are not revoked.",
					"",
					"Example: nopsai logout",
				)
			}
			return lines
		},
	}
}

func runInteractiveLogin(command *cobra.Command, options *rootOptions, prompter *interactive.Prompter) error {
	contextName, err := authenticateContextWithToken(command, options)
	if err != nil {
		return err
	}
	if prompter.CanUseLiveSelector() {
		return prompter.ShowTextScreen("Authenticated", []string{
			"Authenticated",
			"",
			fmt.Sprintf("Context: %s", contextName),
		}, commandOutputScreenOptions("Authentication", collectHomeState(command.Context(), options), nil))
	}
	_, err = fmt.Fprintf(command.OutOrStdout(), "Authenticated context %q\n", contextName)
	return err
}

func runInteractiveLogout(command *cobra.Command, options *rootOptions, prompter *interactive.Prompter) error {
	contextName, err := removeStoredContextToken(options)
	if err != nil {
		return err
	}
	if prompter.CanUseLiveSelector() {
		return prompter.ShowTextScreen("Logged out", []string{
			"Logged out",
			"",
			fmt.Sprintf("Removed the local credential for context %q", contextName),
		}, commandOutputScreenOptions("Authentication", collectHomeState(command.Context(), options), nil))
	}
	_, err = fmt.Fprintf(command.OutOrStdout(), "Removed the local credential for context %q\n", contextName)
	return err
}

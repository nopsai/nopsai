package command

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"nopsai/internal/cli/client"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newLoginCommand(options *rootOptions) *cobra.Command {
	var tokenMode bool
	command := &cobra.Command{
		Use:   "login",
		Short: "Authenticate the current context",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if !tokenMode {
				return errors.New("only token login is available in this release; use --token")
			}
			contextName, err := authenticateContextWithToken(command, options)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "Authenticated context %q\n", contextName)
			return err
		},
	}
	command.Flags().BoolVar(&tokenMode, "token", false, "read an API, personal-access, or service-account token from NOPSAI_TOKEN or stdin")
	return command
}

func newLogoutCommand(options *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the locally stored credential for a context",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			name, err := removeStoredContextToken(options)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "Removed the local credential for context %q\n", name)
			return err
		},
	}
}

func authenticateContextWithToken(command *cobra.Command, options *rootOptions) (string, error) {
	session, err := options.resolveSession(true)
	if err != nil {
		return "", err
	}
	token, err := readLoginToken(command, options.dependencies)
	if err != nil {
		return "", err
	}
	verifiedClient, err := clientWithToken(options, session.API, token)
	if err != nil {
		return "", err
	}
	request, err := verifiedClient.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	if err != nil {
		return "", err
	}
	response, err := verifiedClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("verify token: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("token verification returned HTTP %d", response.StatusCode)
	}
	store, err := options.store()
	if err != nil {
		return "", err
	}
	if err := store.SaveToken(session.ContextName, token); err != nil {
		return "", err
	}
	return session.ContextName, nil
}

func removeStoredContextToken(options *rootOptions) (string, error) {
	store, err := options.store()
	if err != nil {
		return "", err
	}
	name, _, err := store.ResolveContext(options.contextName)
	if err != nil {
		return "", err
	}
	if err := store.DeleteToken(name); err != nil {
		return "", err
	}
	return name, nil
}

func readLoginToken(command *cobra.Command, dependencies Dependencies) (string, error) {
	if token := strings.TrimSpace(dependencies.Getenv("NOPSAI_TOKEN")); token != "" {
		return validateTokenInput(token)
	}
	if inputFile, ok := command.InOrStdin().(*os.File); ok && term.IsTerminal(int(inputFile.Fd())) {
		if _, err := fmt.Fprint(command.ErrOrStderr(), "Token: "); err != nil {
			return "", err
		}
		value, err := term.ReadPassword(int(inputFile.Fd()))
		_, _ = fmt.Fprintln(command.ErrOrStderr())
		if err != nil {
			return "", fmt.Errorf("read token: %w", err)
		}
		return validateTokenInput(string(value))
	}
	value, err := io.ReadAll(io.LimitReader(command.InOrStdin(), 1<<20))
	if err != nil {
		return "", fmt.Errorf("read token: %w", err)
	}
	return validateTokenInput(string(value))
}

func validateTokenInput(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("token cannot be empty")
	}
	if strings.ContainsAny(token, "\r\n") {
		return "", errors.New("token cannot contain a newline")
	}
	return token, nil
}

func clientWithToken(options *rootOptions, apiURL, token string) (*client.Client, error) {
	httpClient := options.dependencies.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: options.timeout}
	} else {
		clone := *httpClient
		clone.Timeout = options.timeout
		httpClient = &clone
	}
	return client.New(client.Options{BaseURL: apiURL, Token: token, HTTPClient: httpClient, UserAgent: "nopsai-cli/" + options.dependencies.Version})
}

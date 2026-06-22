package command

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"nopsai/internal/cli/client"
	clconfig "nopsai/internal/cli/config"
	"nopsai/pkg/buildinfo"

	"github.com/spf13/cobra"
)

type Dependencies struct {
	In         io.Reader
	Out        io.Writer
	Err        io.Writer
	HTTPClient *http.Client
	LookPath   func(string) (string, error)
	RunCommand func(context.Context, string, ...string) error
	RunProcess func(context.Context, string, []string, io.Writer, io.Writer) error
	Getenv     func(string) string
	Version    string
	BuildInfo  buildinfo.Info
}

type rootOptions struct {
	dependencies Dependencies
	configDir    string
	contextName  string
	apiURL       string
	timeout      time.Duration
}

type session struct {
	ContextName string
	API         string
	Token       string
	Client      *client.Client
}

func NewRootCommand(dependencies Dependencies) *cobra.Command {
	dependencies = withDependencyDefaults(dependencies)
	version := strings.TrimSpace(dependencies.Version)
	if version == "" {
		version = strings.TrimSpace(dependencies.BuildInfo.Version)
	}
	dependencies.Version = version
	options := &rootOptions{dependencies: dependencies, timeout: 30 * time.Second}
	root := &cobra.Command{
		Use:           "nopsai",
		Short:         "Operate the NopsAI platform and API",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetIn(dependencies.In)
	root.SetOut(dependencies.Out)
	root.SetErr(dependencies.Err)
	root.PersistentFlags().StringVar(&options.configDir, "config-dir", "", "configuration directory (default: $NOPSAI_CONFIG_DIR or user config directory)")
	root.PersistentFlags().StringVar(&options.contextName, "context", "", "context to use instead of the current context")
	root.PersistentFlags().StringVar(&options.apiURL, "api", "", "API URL override")
	root.PersistentFlags().DurationVar(&options.timeout, "timeout", options.timeout, "request timeout")

	root.AddCommand(newContextCommand(options))
	root.AddCommand(newLoginCommand(options))
	root.AddCommand(newLogoutCommand(options))
	root.AddCommand(newAPICommand(options))
	root.AddCommand(newPlatformCommand(options))
	return root
}

func Execute(version string) error {
	return NewRootCommand(Dependencies{Version: version}).Execute()
}

func (o *rootOptions) store() (*clconfig.Store, error) {
	return clconfig.NewStore(o.configDir)
}

func (o *rootOptions) resolveSession(requireContext bool) (session, error) {
	return o.resolveSessionWithToken(requireContext, true)
}

func (o *rootOptions) resolveSessionWithToken(requireContext, loadToken bool) (session, error) {
	if o.timeout < 0 {
		return session{}, fmt.Errorf("timeout cannot be negative")
	}
	store, err := o.store()
	if err != nil {
		return session{}, err
	}
	contextName, contextAPI, apiURL := "", "", strings.TrimSpace(o.apiURL)
	if contextName = strings.TrimSpace(o.contextName); contextName != "" || apiURL == "" || requireContext {
		resolvedName, ctx, resolveErr := store.ResolveContext(contextName)
		if resolveErr != nil {
			return session{}, resolveErr
		}
		contextName = resolvedName
		contextAPI = ctx.API
		if apiURL == "" {
			apiURL = ctx.API
		}
	}
	if apiURL == "" {
		return session{}, fmt.Errorf("API URL is required")
	}
	normalizedAPI, err := clconfig.NormalizeAPIURL(apiURL)
	if err != nil {
		return session{}, err
	}
	token := ""
	if loadToken {
		token = strings.TrimSpace(o.dependencies.Getenv("NOPSAI_TOKEN"))
		if token == "" && contextName != "" && sameAPIOrigin(normalizedAPI, contextAPI) {
			token, err = store.Token(contextName)
			if err != nil {
				return session{}, err
			}
		}
	}
	if strings.ContainsAny(token, "\r\n") {
		return session{}, fmt.Errorf("token cannot contain a newline")
	}
	httpClient := o.dependencies.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: o.timeout}
	} else {
		clone := *httpClient
		clone.Timeout = o.timeout
		httpClient = &clone
	}
	apiClient, err := client.New(client.Options{
		BaseURL:    normalizedAPI,
		Token:      token,
		HTTPClient: httpClient,
		UserAgent:  "nopsai-cli/" + strings.TrimSpace(o.dependencies.Version),
	})
	if err != nil {
		return session{}, err
	}
	return session{ContextName: contextName, API: normalizedAPI, Token: token, Client: apiClient}, nil
}

func sameAPIOrigin(left, right string) bool {
	leftURL, leftErr := url.Parse(strings.TrimSpace(left))
	rightURL, rightErr := url.Parse(strings.TrimSpace(right))
	if leftErr != nil || rightErr != nil {
		return false
	}
	return strings.EqualFold(leftURL.Scheme, rightURL.Scheme) && strings.EqualFold(leftURL.Host, rightURL.Host)
}

func withDependencyDefaults(dependencies Dependencies) Dependencies {
	if dependencies.In == nil {
		dependencies.In = os.Stdin
	}
	if dependencies.Out == nil {
		dependencies.Out = os.Stdout
	}
	if dependencies.Err == nil {
		dependencies.Err = os.Stderr
	}
	if dependencies.LookPath == nil {
		dependencies.LookPath = exec.LookPath
	}
	if dependencies.RunCommand == nil {
		dependencies.RunCommand = func(ctx context.Context, command string, args ...string) error {
			return exec.CommandContext(ctx, command, args...).Run() // #nosec G204 -- platform owns the fixed command set.
		}
	}
	if dependencies.RunProcess == nil {
		dependencies.RunProcess = func(ctx context.Context, command string, args []string, stdout, stderr io.Writer) error {
			process := exec.CommandContext(ctx, command, args...) // #nosec G204 -- platform commands are fixed and arguments are validated.
			process.Stdout = stdout
			process.Stderr = stderr
			return process.Run()
		}
	}
	if dependencies.Getenv == nil {
		dependencies.Getenv = os.Getenv
	}
	if strings.TrimSpace(dependencies.BuildInfo.Version) == "" {
		dependencies.BuildInfo = buildinfo.Current()
	}
	return dependencies
}

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"nopsai/internal/cli/client"
	clconfig "nopsai/internal/cli/config"
	"nopsai/internal/cli/interactive"
	"nopsai/internal/cli/platform"

	"github.com/spf13/cobra"
)

const homeHealthTimeout = 3 * time.Second

type homeState struct {
	Version      string
	ConfigDir    string
	ContextName  string
	ContextCount int
	API          string
	Token        string
	User         string
	Session      session
	Checks       []platform.Check
	Warning      string
}

func executeInteractiveHome(command *cobra.Command, options *rootOptions) error {
	prompter := interactive.NewPrompter(command.InOrStdin(), command.OutOrStdout())
	for {
		state := collectHomeState(command.Context(), options)
		choices := homeChoices()
		var selected int
		var err error
		if prompter.CanUseLiveSelector() {
			selected, err = prompter.ChooseScreen("NopsAI home", choices, homeScreenOptions(options, state))
		} else {
			if err := renderInteractiveHome(command, options, state); err != nil {
				return err
			}
			selected, err = prompter.Choose("NopsAI home", choices)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if errors.Is(err, interactive.ErrBack) || errors.Is(err, interactive.ErrCancelled) {
			return nil
		}
		if err != nil {
			return err
		}
		var actionErr error
		switch selected {
		case 0:
			actionErr = runInteractiveAPIMenu(command, options, prompter)
		case 1:
			actionErr = runInteractiveContextMenu(command, options, prompter)
		case 2:
			actionErr = runInteractiveAuthMenu(command, options, prompter)
		case 3:
			actionErr = runInstallWizard(command, options)
		case 4:
			actionErr = runInteractivePlatformMenu(command, options, prompter)
		case 5:
			actionErr = runInteractiveUpdate(command, options, prompter)
		case 6:
			actionErr = runInteractiveCompletion(command, prompter)
		case 7:
			actionErr = runInteractiveGuideMenu(command, prompter)
		case 8:
			actionErr = runInteractiveLicense(command, prompter, state)
		case 9:
			actionErr = showInteractiveCommandPreview(prompter, "Help command preview", []string{"nopsai", "--help"}, []string{
				"Render top-level CLI help.",
			}, commandPreviewScreenOptions([]string{"Home", "Help", "Preview"}, "Help Preview", sessionHeaderLines(state)))
			if actionErr != nil {
				break
			}
			if prompter.CanUseLiveSelector() {
				actionErr = prompter.ShowTextScreen("Help", helpScreenLines(command), homeTextScreenOptions("Help", state))
			} else {
				actionErr = command.Help()
			}
		case 10:
			return nil
		}
		if errors.Is(actionErr, io.EOF) {
			return nil
		}
		if errors.Is(actionErr, interactive.ErrBack) {
			continue
		}
		if errors.Is(actionErr, interactive.ErrCancelled) {
			return nil
		}
		if actionErr != nil {
			if prompter.CanUseLiveSelector() {
				_ = prompter.ShowTextScreen("Error", []string{"Error", "", actionErr.Error()}, homeTextScreenOptions("Error", state))
			} else {
				_, _ = fmt.Fprintf(command.ErrOrStderr(), "Error: %v\n", actionErr)
			}
		}
	}
}

func homeChoices() []interactive.Choice {
	return []interactive.Choice{
		{Label: "api", Description: "Routes, descriptions, catalog calls, and concrete requests", SearchText: "api request call routes describe catalog endpoint payload parameter header"},
		{Label: "contexts", Description: "List, add, use, or delete API contexts", SearchText: "context config token api address"},
		{Label: "authentication", Description: "Login with a token or remove a local credential", SearchText: "auth login logout token credential aaa nopat nopsat"},
		{Label: "install", Description: "Open the first-install wizard for Docker Compose or Kubernetes", SearchText: "install setup docker compose kubernetes helm environment topology gitops"},
		{Label: "platform", Description: "Run doctor checks, plan/deploy releases, or upgrade an install", SearchText: "platform health monitoring doctor release upgrade helm manifest lock aaa metrics dispatcher"},
		{Label: "update", Description: "Update this CLI to an exact release version", SearchText: "update selfupdate cli binary version release download upgrade"},
		{Label: "completion", Description: "Generate shell completion files or raw scripts", SearchText: "completion bash zsh fish powershell shell output stdout"},
		{Label: "guide", Description: "Read CLI examples for common operator topics", SearchText: "help sample example config gitops mcp"},
		{Label: "license", Description: "Show the NopsAI proprietary software notice", SearchText: "license notice legal copyright proprietary terms"},
		{Label: "help", Description: "Show command help", SearchText: "help commands flags"},
		{Label: "exit", Description: "Close the interactive session", SearchText: "quit back exit"},
	}
}

func homeScreenOptions(options *rootOptions, state homeState) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home"},
		Title:      "Home",
		LeftTitle:  "Menu",
		RightTitle: "Overview",
		LeftWidth:  44,
		Header:     sessionHeaderLines(state),
		Footer:     sessionFooterLines(),
		Detail: func(index int, choice interactive.Choice) []string {
			return homeChoiceDetail(index, choice, state)
		},
	}
}

func homeTextScreenOptions(title string, state homeState) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Title:      title,
		Breadcrumb: []string{"Home", title},
		Header:     sessionHeaderLines(state),
		Footer:     sessionFooterLines(),
	}
}

func contextMenuScreenOptions(state homeState) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "Contexts"},
		Title:      "Contexts",
		Header:     sessionHeaderLines(state),
		LeftTitle:  "Actions",
		RightTitle: "Context Detail",
		LeftWidth:  38,
		Footer:     sessionFooterLines(),
		Detail: func(_ int, choice interactive.Choice) []string {
			return []string{
				choice.Description,
				"",
				"Contexts store API URLs and local tokens by environment.",
				"",
				"Examples",
				"  nopsai context add prod --api https://nopsai.example.com",
				"  nopsai context use prod",
				"  nopsai login --token",
			}
		},
	}
}

func contextChoiceScreenOptions(state homeState, title string) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "Contexts", title},
		Title:      title,
		Header:     sessionHeaderLines(state),
		LeftTitle:  "Contexts",
		RightTitle: "Context Detail",
		LeftWidth:  48,
		Footer:     sessionFooterLines(),
	}
}

func contextFormScreenOptions(state homeState, title string) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb:  []string{"Home", "Contexts", title},
		Title:       title,
		Header:      sessionHeaderLines(state),
		LeftTitle:   "Context Steps",
		RightTitle:  "Values & Details",
		LeftWidth:   48,
		ActionLabel: "Submit context change",
		Footer:      sessionFooterLines(),
	}
}

func contextTextScreenOptions(state homeState, title string) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "Contexts", title},
		Title:      title,
		Header:     sessionHeaderLines(state),
		Footer:     sessionFooterLines(),
	}
}

func homeHealthSummary(checks []platform.Check) string {
	if len(checks) == 0 {
		return "unknown"
	}
	counts := map[platform.Severity]int{}
	for _, check := range checks {
		counts[check.Severity]++
	}
	parts := make([]string, 0, 3)
	for _, severity := range []platform.Severity{platform.SeverityOK, platform.SeverityWarning, platform.SeverityError} {
		if count := counts[severity]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", strings.ToUpper(string(severity)), count))
		}
	}
	return strings.Join(parts, ", ")
}

func homeChoiceDetail(index int, choice interactive.Choice, state homeState) []string {
	lines := []string{
		choice.Description,
		"",
		"Session",
		"  User: " + valueOrDefault(state.User, "not authenticated"),
		"  Token: " + state.Token,
		"  API: " + valueOrDefault(state.API, "not configured"),
		"",
		"Health",
	}
	for _, check := range state.Checks {
		lines = append(lines, fmt.Sprintf("  %-7s %-24s %s", strings.ToUpper(string(check.Severity)), check.Name, check.Message))
	}
	switch index {
	case 0:
		lines = append(lines, "", "API opens the registered route catalog, raw request transport, route listing filters, and route descriptions.")
	case 1:
		lines = append(lines, "", "Contexts store API URLs and local tokens. Use them to switch between environments.")
	case 2:
		lines = append(lines, "", "Authentication verifies tokens through AAA before storing them locally and removes only local credentials on logout.")
	case 3:
		lines = append(lines, "", "Install generates Docker Compose files or Helm values from the selected NopsAI version.")
	case 4:
		lines = append(lines, "", "Platform includes doctor diagnostics, advanced digest-pinned Kubernetes release planning and deployment, and upgrades of an existing install.")
	case 5:
		lines = append(lines, "", "Update replaces this CLI binary with an exact release version. Update the CLI before a platform upgrade: the CLI owns the templates a release expects.")
	case 6:
		lines = append(lines, "", "Completion generates bash, zsh, fish, or PowerShell scripts with copy instructions or raw stdout output.")
	case 7:
		lines = append(lines, "", "Guides show copyable examples for API calls, GitOps, installs, AAA, MCP, monitoring, and config.")
	case 8:
		lines = append(lines, "", "License prints the NopsAI proprietary software notice and where third-party terms are recorded.")
	}
	return lines
}

func collectHomeState(ctx context.Context, options *rootOptions) homeState {
	state := homeState{Token: "not configured", Version: valueOrDefault(options.dependencies.Version, "dev")}
	store, err := options.store()
	if err != nil {
		state.Warning = err.Error()
		return state
	}
	state.ConfigDir = store.Dir()
	cfg, err := store.Load()
	if err != nil {
		state.Warning = err.Error()
		return state
	}
	state.ContextCount = len(cfg.Contexts)
	state.ContextName = strings.TrimSpace(options.contextName)
	if state.ContextName == "" {
		state.ContextName = cfg.CurrentContext
	}
	session, err := resolveOptionalHomeSession(options, store, cfg)
	if err != nil {
		state.Warning = err.Error()
		return state
	}
	state.Session = session
	state.API = session.API
	if session.Token != "" {
		switch session.TokenSource {
		case "environment":
			state.Token = "configured from NOPSAI_TOKEN"
		case "context":
			state.Token = "configured for context"
		default:
			state.Token = "configured"
		}
	}
	if session.Client == nil {
		state.Checks = []platform.Check{{
			Name:     "api/context",
			Severity: platform.SeverityWarning,
			Message:  "no API configured; run `nopsai context add NAME --api URL` or pass --api",
		}}
		return state
	}
	state.Checks, state.User = runHomeHealthChecks(ctx, session)
	return state
}

func resolveOptionalHomeSession(options *rootOptions, store *clconfig.Store, cfg clconfig.File) (session, error) {
	contextName := strings.TrimSpace(options.contextName)
	if contextName == "" {
		contextName = cfg.CurrentContext
	}
	contextAPI := ""
	if contextName != "" {
		ctx, ok := cfg.Contexts[contextName]
		if !ok {
			return session{}, fmt.Errorf("context %q does not exist", contextName)
		}
		contextAPI = ctx.API
	}
	apiURL := strings.TrimSpace(options.apiURL)
	if apiURL == "" {
		apiURL = contextAPI
	}
	if apiURL == "" {
		return session{ContextName: contextName}, nil
	}
	normalizedAPI, err := clconfig.NormalizeAPIURL(apiURL)
	if err != nil {
		return session{}, err
	}
	token := strings.TrimSpace(options.dependencies.Getenv("NOPSAI_TOKEN"))
	tokenSource := ""
	if token != "" {
		tokenSource = "environment"
	}
	if token == "" && contextName != "" && sameAPIOrigin(normalizedAPI, contextAPI) {
		token, err = store.Token(contextName)
		if err != nil {
			return session{}, err
		}
		if token != "" {
			tokenSource = "context"
		}
	}
	if strings.ContainsAny(token, "\r\n") {
		return session{}, fmt.Errorf("token cannot contain a newline")
	}
	httpClient := options.dependencies.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: options.timeout}
	} else {
		clone := *httpClient
		clone.Timeout = options.timeout
		httpClient = &clone
	}
	apiClient, err := client.New(client.Options{
		BaseURL:    normalizedAPI,
		Token:      token,
		HTTPClient: httpClient,
		UserAgent:  "nopsai-cli/" + strings.TrimSpace(options.dependencies.Version),
	})
	if err != nil {
		return session{}, err
	}
	return session{ContextName: contextName, API: normalizedAPI, Token: token, TokenSource: tokenSource, Client: apiClient}, nil
}

func renderInteractiveHome(command *cobra.Command, options *rootOptions, state homeState) error {
	out := command.OutOrStdout()
	if _, err := fmt.Fprintln(out, "\nNopsAI CLI"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "Mode: interactive operator console"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Version: %s\n", valueOrDefault(options.dependencies.Version, "dev")); err != nil {
		return err
	}
	if state.ConfigDir != "" {
		if _, err := fmt.Fprintf(out, "Config: %s\n", state.ConfigDir); err != nil {
			return err
		}
	}
	contextLabel := valueOrDefault(state.ContextName, "not selected")
	if state.ContextCount > 0 {
		contextLabel = fmt.Sprintf("%s (%d configured)", contextLabel, state.ContextCount)
	}
	if _, err := fmt.Fprintf(out, "Context: %s\n", contextLabel); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "API: %s\n", valueOrDefault(state.API, "not configured")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Token: %s\n", state.Token); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Session: %s\n", valueOrDefault(state.User, "not authenticated")); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "Navigation: type to filter | Enter select | Esc back | Ctrl+C quit"); err != nil {
		return err
	}
	if strings.TrimSpace(state.Warning) != "" {
		if _, err := fmt.Fprintf(out, "Warning: %s\n", state.Warning); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(out, "Health:"); err != nil {
		return err
	}
	for _, check := range state.Checks {
		if _, err := fmt.Fprintf(out, "  %-7s %-24s %s\n", strings.ToUpper(string(check.Severity)), check.Name, check.Message); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(out, "Quick commands: nopsai api call --interactive | nopsai api request GET /v1/auth/me | nopsai platform release --interactive"); err != nil {
		return err
	}
	return nil
}

func runHomeHealthChecks(parent context.Context, session session) ([]platform.Check, string) {
	ctx, cancel := context.WithTimeout(parent, homeHealthTimeout)
	defer cancel()
	checks := make([]platform.Check, 0, 4)
	checks = append(checks, homeStatusCheck(ctx, session.Client, "nopsai/healthz", "/healthz", false))
	versionCheck := homeVersionCheck(ctx, session.Client)
	checks = append(checks, versionCheck)
	checks = append(checks, homePreflightCheck(ctx, session.Client))
	user := ""
	if session.Token == "" {
		checks = append(checks, platform.Check{Name: "aaa/session", Severity: platform.SeverityWarning, Message: "no token configured"})
	} else {
		check, display := homeSessionCheck(ctx, session.Client)
		checks = append(checks, check)
		user = display
	}
	return checks, user
}

func homeStatusCheck(ctx context.Context, apiClient *client.Client, name, path string, authenticated bool) platform.Check {
	status, _, err := homeGet(ctx, apiClient, path, authenticated)
	if err != nil {
		return platform.Check{Name: name, Severity: platform.SeverityWarning, Message: err.Error()}
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return platform.Check{Name: name, Severity: platform.SeverityWarning, Message: fmt.Sprintf("HTTP %d", status)}
	}
	return platform.Check{Name: name, Severity: platform.SeverityOK, Message: "reachable"}
}

func homeVersionCheck(ctx context.Context, apiClient *client.Client) platform.Check {
	status, body, err := homeGet(ctx, apiClient, "/version", false)
	if err != nil {
		return platform.Check{Name: "nopsai/version", Severity: platform.SeverityWarning, Message: err.Error()}
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return platform.Check{Name: "nopsai/version", Severity: platform.SeverityWarning, Message: fmt.Sprintf("HTTP %d", status)}
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return platform.Check{Name: "nopsai/version", Severity: platform.SeverityWarning, Message: "invalid version response"}
	}
	version := firstHomeString(payload, "product_version", "version")
	apiVersion := firstHomeString(payload, "api_version")
	message := valueOrDefault(version, "unknown version")
	if apiVersion != "" {
		message += " (API " + apiVersion + ")"
	}
	return platform.Check{Name: "nopsai/version", Severity: platform.SeverityOK, Message: message}
}

func homePreflightCheck(ctx context.Context, apiClient *client.Client) platform.Check {
	status, body, err := homeGet(ctx, apiClient, "/v1/setup/preflight", false)
	if err != nil {
		return platform.Check{Name: "setup/preflight", Severity: platform.SeverityWarning, Message: err.Error()}
	}
	var payload struct {
		Ready bool   `json:"ready"`
		Mode  string `json:"mode"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return platform.Check{Name: "setup/preflight", Severity: platform.SeverityWarning, Message: "invalid preflight response"}
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices || !payload.Ready {
		return platform.Check{Name: "setup/preflight", Severity: platform.SeverityWarning, Message: fmt.Sprintf("not ready (HTTP %d, mode %s)", status, homeValueOrUnknown(payload.Mode))}
	}
	return platform.Check{Name: "setup/preflight", Severity: platform.SeverityOK, Message: "ready (mode " + homeValueOrUnknown(payload.Mode) + ")"}
}

func homeSessionCheck(ctx context.Context, apiClient *client.Client) (platform.Check, string) {
	status, body, err := homeGet(ctx, apiClient, "/v1/auth/me", true)
	if err != nil {
		return platform.Check{Name: "aaa/session", Severity: platform.SeverityWarning, Message: err.Error()}, ""
	}
	if status == http.StatusUnauthorized {
		return platform.Check{Name: "aaa/session", Severity: platform.SeverityError, Message: "token rejected"}, ""
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return platform.Check{Name: "aaa/session", Severity: platform.SeverityWarning, Message: fmt.Sprintf("HTTP %d", status)}, ""
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return platform.Check{Name: "aaa/session", Severity: platform.SeverityWarning, Message: "invalid session response"}, ""
	}
	display := firstHomeString(payload, "display_name", "email", "sub", "id")
	if display == "" {
		display = "authenticated"
	}
	return platform.Check{Name: "aaa/session", Severity: platform.SeverityOK, Message: display}, display
}

func homeGet(ctx context.Context, apiClient *client.Client, path string, authenticated bool) (int, []byte, error) {
	request, err := apiClient.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		return 0, nil, err
	}
	request = request.WithContext(ctx)
	var response *http.Response
	if authenticated {
		response, err = apiClient.Do(request)
	} else {
		response, err = apiClient.DoUnauthenticated(request)
	}
	if err != nil {
		return 0, nil, fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return 0, nil, fmt.Errorf("read response: %w", err)
	}
	return response.StatusCode, body, nil
}

func firstHomeString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func homeValueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return strings.TrimSpace(value)
}

func runInteractiveContextMenu(command *cobra.Command, options *rootOptions, prompter *interactive.Prompter) error {
	choices := []interactive.Choice{
		{Label: "list", Description: "Show configured contexts and API URLs", SearchText: "list current context"},
		{Label: "add", Description: "Add or update a context API URL", SearchText: "add api url hostname address"},
		{Label: "use", Description: "Switch current context", SearchText: "switch use current"},
		{Label: "delete", Description: "Delete a context and local credential", SearchText: "remove delete logout credential"},
		{Label: "back", Description: "Return to the home menu", SearchText: "back home"},
	}
	for {
		state := collectHomeState(command.Context(), options)
		var (
			selected int
			err      error
		)
		if prompter.CanUseLiveSelector() {
			selected, err = prompter.ChooseScreen("Contexts", choices, contextMenuScreenOptions(state))
		} else {
			selected, err = prompter.Choose("Contexts", choices)
		}
		if errors.Is(err, interactive.ErrBack) {
			return nil
		}
		if err != nil {
			return err
		}
		switch selected {
		case 0:
			if err := printInteractiveContexts(command, options); err != nil {
				if errors.Is(err, interactive.ErrBack) {
					continue
				}
				return err
			}
		case 1:
			if err := addInteractiveContext(command, options, prompter); err != nil {
				if errors.Is(err, interactive.ErrBack) {
					continue
				}
				return err
			}
		case 2:
			if err := useInteractiveContext(command, options, prompter); err != nil {
				if errors.Is(err, interactive.ErrBack) {
					continue
				}
				return err
			}
		case 3:
			if err := deleteInteractiveContext(command, options, prompter); err != nil {
				if errors.Is(err, interactive.ErrBack) {
					continue
				}
				return err
			}
		case 4:
			return nil
		}
	}
}

func printInteractiveContexts(command *cobra.Command, options *rootOptions) error {
	store, err := options.store()
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	names := sortedContextNames(cfg)
	prompter := interactive.NewPrompter(command.InOrStdin(), command.OutOrStdout())
	state := collectHomeState(command.Context(), options)
	if err := showInteractiveCommandPreview(prompter, "Context list command preview", []string{"nopsai", "context", "list"}, []string{
		"Render configured local API contexts.",
	}, commandPreviewScreenOptions([]string{"Home", "Contexts", "List", "Preview"}, "Context List Preview", sessionHeaderLines(state))); err != nil {
		return err
	}
	if len(names) == 0 {
		if prompter.CanUseLiveSelector() {
			return prompter.ShowTextScreen("Contexts", []string{"No contexts configured."}, contextTextScreenOptions(state, "Contexts"))
		}
		_, err = fmt.Fprintln(command.OutOrStdout(), "No contexts configured.")
		return err
	}
	if prompter.CanUseLiveSelector() {
		lines := []string{"Configured contexts", ""}
		for _, name := range names {
			current := " "
			if name == cfg.CurrentContext {
				current = "*"
			}
			lines = append(lines, fmt.Sprintf("%s %s  %s", current, name, cfg.Contexts[name].API))
		}
		return prompter.ShowTextScreen("Contexts", lines, contextTextScreenOptions(state, "Contexts"))
	}
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
}

func addInteractiveContext(command *cobra.Command, options *rootOptions, prompter *interactive.Prompter) error {
	if prompter.CanUseLiveSelector() {
		fields := []interactive.Field{
			{Name: "name", Label: "Context name", Required: true, Description: "Local name for this API environment.", Example: "prod"},
			{Name: "api", Label: "NopsAI API URL", Value: valueOrDefault(options.apiURL, "http://localhost:8080"), Required: true, Description: "Absolute NopsAI API URL for this context.", Example: "https://nopsai.example.com"},
		}
		edited, err := prompter.EditFieldsScreen("Add context", fields, contextFormScreenOptions(collectHomeState(command.Context(), options), "Add Context"))
		if err != nil {
			return err
		}
		values := fieldValueMap(edited)
		state := collectHomeState(command.Context(), options)
		if err := showInteractiveCommandPreview(prompter, "Context add command preview", []string{"nopsai", "context", "add", strings.TrimSpace(values["name"]), "--api", strings.TrimSpace(values["api"])}, []string{
			"Write the local API context configuration.",
		}, commandPreviewScreenOptions([]string{"Home", "Contexts", "Add", "Preview"}, "Context Add Preview", sessionHeaderLines(state))); err != nil {
			return err
		}
		store, err := options.store()
		if err != nil {
			return err
		}
		ctx, err := store.AddContext(values["name"], values["api"])
		if err != nil {
			return err
		}
		return prompter.ShowTextScreen("Context configured", []string{
			"Context configured",
			"",
			fmt.Sprintf("%s  %s", strings.TrimSpace(values["name"]), ctx.API),
		}, contextTextScreenOptions(collectHomeState(command.Context(), options), "Context Configured"))
	}
	name, err := prompter.AskRequired("Context name", "")
	if err != nil {
		return err
	}
	api, err := prompter.AskRequired("NopsAI API URL", valueOrDefault(options.apiURL, "http://localhost:8080"))
	if err != nil {
		return err
	}
	state := collectHomeState(command.Context(), options)
	if err := showInteractiveCommandPreview(prompter, "Context add command preview", []string{"nopsai", "context", "add", strings.TrimSpace(name), "--api", strings.TrimSpace(api)}, []string{
		"Write the local API context configuration.",
	}, commandPreviewScreenOptions([]string{"Home", "Contexts", "Add", "Preview"}, "Context Add Preview", sessionHeaderLines(state))); err != nil {
		return err
	}
	store, err := options.store()
	if err != nil {
		return err
	}
	ctx, err := store.AddContext(name, api)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(command.OutOrStdout(), "Context %q configured for %s\n", strings.TrimSpace(name), ctx.API)
	return err
}

func useInteractiveContext(command *cobra.Command, options *rootOptions, prompter *interactive.Prompter) error {
	store, cfg, names, err := contextChoices(options)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		_, err = fmt.Fprintln(command.OutOrStdout(), "No contexts configured.")
		return err
	}
	choices := make([]interactive.Choice, 0, len(names))
	for _, name := range names {
		description := cfg.Contexts[name].API
		if name == cfg.CurrentContext {
			description += " (current)"
		}
		choices = append(choices, interactive.Choice{Label: name, Description: description, SearchText: name + " " + description})
	}
	var selected int
	if prompter.CanUseLiveSelector() {
		selected, err = prompter.ChooseScreen("Use context", choices, contextChoiceScreenOptions(collectHomeState(command.Context(), options), "Use Context"))
	} else {
		selected, err = prompter.Choose("Use context", choices)
	}
	if err != nil {
		return err
	}
	state := collectHomeState(command.Context(), options)
	if err := showInteractiveCommandPreview(prompter, "Context use command preview", []string{"nopsai", "context", "use", names[selected]}, []string{
		"Set the selected context as the current local API context.",
	}, commandPreviewScreenOptions([]string{"Home", "Contexts", "Use", "Preview"}, "Context Use Preview", sessionHeaderLines(state))); err != nil {
		return err
	}
	if err := store.UseContext(names[selected]); err != nil {
		return err
	}
	if prompter.CanUseLiveSelector() {
		return prompter.ShowTextScreen("Context selected", []string{"Current context", "", names[selected]}, contextTextScreenOptions(collectHomeState(command.Context(), options), "Context Selected"))
	}
	_, err = fmt.Fprintf(command.OutOrStdout(), "Current context is now %q\n", names[selected])
	return err
}

func deleteInteractiveContext(command *cobra.Command, options *rootOptions, prompter *interactive.Prompter) error {
	store, cfg, names, err := contextChoices(options)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		_, err = fmt.Fprintln(command.OutOrStdout(), "No contexts configured.")
		return err
	}
	choices := make([]interactive.Choice, 0, len(names))
	for _, name := range names {
		description := cfg.Contexts[name].API
		if name == cfg.CurrentContext {
			description += " (current)"
		}
		choices = append(choices, interactive.Choice{Label: name, Description: description, SearchText: name + " " + description})
	}
	var selected int
	if prompter.CanUseLiveSelector() {
		selected, err = prompter.ChooseScreen("Delete context", choices, contextChoiceScreenOptions(collectHomeState(command.Context(), options), "Delete Context"))
	} else {
		selected, err = prompter.Choose("Delete context", choices)
	}
	if err != nil {
		return err
	}
	var confirmed bool
	if prompter.CanUseLiveSelector() {
		edited, err := prompter.EditFieldsScreen("Confirm delete", []interactive.Field{{
			Name:        "confirm",
			Label:       "Delete context",
			Value:       "no",
			Kind:        interactive.FieldBoolean,
			Description: "Delete local context configuration and its stored credential.",
			Example:     names[selected],
		}}, contextFormScreenOptions(collectHomeState(command.Context(), options), "Delete Context"))
		if err != nil {
			return err
		}
		confirmed = parseYesNo(fieldValueMap(edited)["confirm"])
	} else {
		confirmed, err = prompter.Confirm("Delete context "+names[selected], false)
	}
	if err != nil {
		return err
	}
	if !confirmed {
		if prompter.CanUseLiveSelector() {
			return prompter.ShowTextScreen("Delete cancelled", []string{"Delete cancelled", "", names[selected]}, contextTextScreenOptions(collectHomeState(command.Context(), options), "Delete Cancelled"))
		}
		_, err = fmt.Fprintln(command.OutOrStdout(), "Delete cancelled.")
		return err
	}
	state := collectHomeState(command.Context(), options)
	if err := showInteractiveCommandPreview(prompter, "Context delete command preview", []string{"nopsai", "context", "delete", names[selected]}, []string{
		"Delete the local context and its stored credential.",
	}, commandPreviewScreenOptions([]string{"Home", "Contexts", "Delete", "Preview"}, "Context Delete Preview", sessionHeaderLines(state))); err != nil {
		return err
	}
	if err := store.DeleteContext(names[selected]); err != nil {
		return err
	}
	if prompter.CanUseLiveSelector() {
		return prompter.ShowTextScreen("Context deleted", []string{"Context deleted", "", names[selected]}, contextTextScreenOptions(collectHomeState(command.Context(), options), "Context Deleted"))
	}
	_, err = fmt.Fprintf(command.OutOrStdout(), "Context %q deleted\n", names[selected])
	return err
}

func contextChoices(options *rootOptions) (*clconfig.Store, clconfig.File, []string, error) {
	store, err := options.store()
	if err != nil {
		return nil, clconfig.File{}, nil, err
	}
	cfg, err := store.Load()
	if err != nil {
		return nil, clconfig.File{}, nil, err
	}
	return store, cfg, sortedContextNames(cfg), nil
}

func sortedContextNames(cfg clconfig.File) []string {
	names := make([]string, 0, len(cfg.Contexts))
	for name := range cfg.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func runInteractiveDoctor(command *cobra.Command, options *rootOptions) error {
	prompter := interactive.NewPrompter(command.InOrStdin(), command.OutOrStdout())
	state := collectHomeState(command.Context(), options)
	if err := showInteractiveCommandPreview(prompter, "Doctor command preview", []string{"nopsai", "platform", "doctor"}, []string{
		"Run local tooling, API readiness, metrics, token, dispatcher, and runner checks.",
	}, commandPreviewScreenOptions([]string{"Home", "Platform", "Doctor", "Preview"}, "Doctor Preview", sessionHeaderLines(state))); err != nil {
		return err
	}
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
	if prompter.CanUseLiveSelector() {
		stdout, stderr, renderErr := captureCommandOutput(command, func() error {
			return renderDoctor(command, checks, "text")
		})
		if renderErr == nil && platform.HasErrors(checks) {
			renderErr = errors.New("one or more doctor checks failed")
		}
		resultErr := showInteractiveOutput(prompter, "Doctor", stdout, stderr, renderErr, doctorScreenOptions(options))
		if errors.Is(resultErr, interactive.ErrBack) {
			return nil
		}
		return resultErr
	}
	if err := renderDoctor(command, checks, "text"); err != nil {
		return err
	}
	if platform.HasErrors(checks) {
		return errors.New("one or more doctor checks failed")
	}
	return nil
}

func doctorScreenOptions(options *rootOptions) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "Platform", "Doctor"},
		Title:      "Doctor",
		Header:     []string{"API: " + valueOrDefault(options.apiURL, "context API")},
		Footer:     []string{"Keys: Up/Down scroll | PgUp/PgDn jump | Home/End | Enter home | Esc home | Ctrl+C quit"},
	}
}

func runInteractiveGuideMenu(command *cobra.Command, prompter *interactive.Prompter) error {
	topics := guideTopics()
	names := make([]string, 0, len(topics))
	for name := range topics {
		names = append(names, name)
	}
	sort.Strings(names)
	choices := make([]interactive.Choice, 0, len(names)+1)
	for _, name := range names {
		topic := topics[name]
		choices = append(choices, interactive.Choice{Label: topic.Name, Description: topic.Summary, SearchText: topic.Name + " " + topic.Summary})
	}
	choices = append(choices, interactive.Choice{Label: "back", Description: "Return to the home menu", SearchText: "back home"})
	for {
		var (
			selected int
			err      error
		)
		if prompter.CanUseLiveSelector() {
			selected, err = prompter.ChooseScreen("Guide topic", choices, guideTopicScreenOptions())
		} else {
			selected, err = prompter.Choose("Guide topic", choices)
		}
		if errors.Is(err, interactive.ErrBack) {
			return nil
		}
		if err != nil {
			return err
		}
		if selected == len(choices)-1 {
			return nil
		}
		if err := showInteractiveCommandPreview(prompter, "Guide command preview", []string{"nopsai", "guide", choices[selected].Label}, []string{
			"Render the selected CLI guide topic.",
		}, commandPreviewScreenOptions([]string{"Home", "Guide", choices[selected].Label, "Preview"}, "Guide Preview", []string{"Topic: " + choices[selected].Label})); err != nil {
			if errors.Is(err, interactive.ErrBack) {
				continue
			}
			return err
		}
		if prompter.CanUseLiveSelector() {
			stdout, stderr, renderErr := captureCommandOutput(command, func() error {
				return renderGuideTopic(command, choices[selected].Label)
			})
			resultErr := showInteractiveOutput(prompter, "Guide: "+choices[selected].Label, stdout, stderr, renderErr, guideTextScreenOptions(choices[selected].Label))
			if errors.Is(resultErr, interactive.ErrBack) {
				continue
			}
			if resultErr != nil {
				return resultErr
			}
			continue
		}
		if err := renderGuideTopic(command, choices[selected].Label); err != nil {
			return err
		}
	}
}

func guideTopicScreenOptions() interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "Guide"},
		Title:      "Guide",
		Header:     []string{"Operator examples and command guidance"},
		LeftTitle:  "Topics",
		RightTitle: "Topic Detail",
		LeftWidth:  42,
		Footer:     []string{"Keys: type filter | Up/Down move | Enter open | Esc home | Ctrl+C quit"},
		Detail: func(_ int, choice interactive.Choice) []string {
			return []string{
				choice.Description,
				"",
				"Guides include copyable noninteractive commands for repeatable operator workflows.",
			}
		},
	}
}

func guideTextScreenOptions(topic string) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "Guide", topic},
		Title:      "Guide: " + topic,
		Header:     []string{"Topic: " + topic},
		Footer:     []string{"Keys: Up/Down scroll | PgUp/PgDn jump | Home/End | Enter topics | Esc topics | Ctrl+C quit"},
	}
}

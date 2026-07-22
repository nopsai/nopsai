package command

import (
	"errors"
	"fmt"
	"strings"

	"nopsai/internal/cli/apicatalog"
	"nopsai/internal/cli/interactive"

	"github.com/spf13/cobra"
)

func runInteractiveAPIMenu(command *cobra.Command, options *rootOptions, prompter *interactive.Prompter) error {
	choices := []interactive.Choice{
		{Label: "catalog call", Description: "Search registered routes, fill route parameters, and send", SearchText: "api call route catalog template parameters body query"},
		{Label: "raw request", Description: "Send a concrete METHOD/PATH request with all transport options", SearchText: "api request method path header data raw accept output file no auth"},
		{Label: "routes", Description: "List registered routes with domain, method, audience, and output filters", SearchText: "api routes list domain method audience output"},
		{Label: "describe", Description: "Describe a registered route as text, JSON, or YAML", SearchText: "api describe route detail text json yaml"},
		{Label: "back", Description: "Return to the home menu", SearchText: "back home"},
	}
	for {
		state := collectHomeState(command.Context(), options)
		var (
			selected int
			err      error
		)
		if prompter.CanUseLiveSelector() {
			selected, err = prompter.ChooseScreen("API", choices, apiMenuScreenOptions(state))
		} else {
			selected, err = prompter.Choose("API", choices)
		}
		if errors.Is(err, interactive.ErrBack) {
			return nil
		}
		if err != nil {
			return err
		}
		switch selected {
		case 0:
			if err := executeInteractiveAPICall(command, options, &apiRequestOptions{}); err != nil {
				if errors.Is(err, interactive.ErrBack) {
					continue
				}
				return err
			}
		case 1:
			if err := runInteractiveRawAPIRequest(command, options, prompter); err != nil {
				if errors.Is(err, interactive.ErrBack) {
					continue
				}
				return err
			}
		case 2:
			if err := runInteractiveAPIRoutes(command, prompter, state); err != nil {
				if errors.Is(err, interactive.ErrBack) {
					continue
				}
				return err
			}
		case 3:
			if err := runInteractiveAPIDescribe(command, prompter, state); err != nil {
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

func apiMenuScreenOptions(state homeState) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "API"},
		Title:      "API",
		Header:     sessionHeaderLines(state),
		LeftTitle:  "Workflows",
		RightTitle: "Guide",
		LeftWidth:  44,
		Footer:     sessionFooterLines(),
		Detail: func(index int, choice interactive.Choice) []string {
			lines := []string{choice.Description, ""}
			switch index {
			case 0:
				lines = append(lines,
					"Guide: Use this for registered API routes. Path templates, query values, request bodies, auth default, and compatibility checks remain validated before the request is sent.",
					"",
					"Example: nopsai api call GET '/v1/pipelines/{pipelineName...}' --path pipelineName=delivery/release",
				)
			case 1:
				lines = append(lines,
					"Guide: Use this when the path is already concrete or when you need byte-exact transport flags such as headers, downloads, streams, or stdin body input.",
					"",
					"Example: nopsai api request POST /v1/mcp --data - --content-type application/json",
				)
			case 2:
				lines = append(lines,
					"Guide: Filter the compiled route catalog locally. No API connection or credentials are required.",
					"",
					"Example: nopsai api routes --domain monitoring --method GET --output json",
				)
			case 3:
				lines = append(lines,
					"Guide: Inspect one route's audience, path/query/body requirements, samples, and noninteractive command shape.",
					"",
					"Example: nopsai api describe POST /v1/run --output text",
				)
			}
			return lines
		},
	}
}

func runInteractiveRawAPIRequest(command *cobra.Command, options *rootOptions, prompter *interactive.Prompter) error {
	state := collectHomeState(command.Context(), options)
	fields := []interactive.Field{
		{Name: "method", Label: "HTTP method", Value: "GET", Default: "GET", Required: true, Description: "HTTP method to send to a concrete API path.", Example: "GET"},
		{Name: "path", Label: "Concrete path", Value: "/", Default: "/", Required: true, Description: "Host-free absolute API path. Include a query string only when you need an exact concrete request.", Example: "/v1/monitoring/summary"},
		{Name: "data", Label: "Body file", Description: "Request body file path. Use - to stream stdin. Mutually exclusive with Literal body.", Example: "payload.json"},
		{Name: "dataRaw", Label: "Literal body", Multiline: true, Description: "Literal request body bytes. Leave blank when using Body file or when the method has no body.", Example: `{"key":"value"}`},
		{Name: "headers", Label: "Headers", Multiline: true, Description: "Additional HTTP headers, one Name: value pair per line. Header names and values reject newlines.", Example: "X-Trace: request-1"},
		{Name: "contentType", Label: "Content-Type", Description: "Overrides request Content-Type. Blank defaults to application/json when a body is present.", Example: "application/json"},
		{Name: "accept", Label: "Accept", Description: "Optional response media type for streams, downloads, YAML, or other non-JSON responses.", Example: "text/event-stream"},
		{Name: "outputFile", Label: "Output file", Description: "Write only a 2xx response body atomically to this file. Error bodies stay on stdout.", Example: "artifact.zip"},
		{Name: "auth", Label: "Attach bearer token", Value: "yes", Default: "yes", Kind: interactive.FieldBoolean, Description: "Attach the configured context or environment token. Set no for public endpoints.", Example: "yes"},
		{Name: "showHeaders", Label: "Show response headers", Value: "no", Default: "no", Kind: interactive.FieldBoolean, Description: "Write response status and headers before the body.", Example: "no"},
		{Name: "send", Label: "Send request", Value: "yes", Default: "yes", Kind: interactive.FieldBoolean, Description: "Final review gate. Change to no or press Esc to return to the API menu without sending.", Example: "yes"},
	}
	var (
		method         string
		path           string
		requestOptions apiRequestOptions
		validation     string
	)
	for {
		edited, err := prompter.EditFieldsScreen("Raw API request", fields, rawAPIRequestScreenOptions(state, validation))
		if err != nil {
			return err
		}
		fields = edited
		method, path, requestOptions, err = applyRawAPIRequestFields(edited)
		if err == nil {
			break
		}
		if errors.Is(err, interactive.ErrBack) {
			return err
		}
		if !prompter.CanUseLiveSelector() {
			return err
		}
		validation = err.Error()
	}
	session, err := options.resolveSessionWithToken(false, !requestOptions.noAuth)
	if err != nil {
		return err
	}
	stdout, stderr, callErr := captureCommandOutput(command, func() error {
		return executeAPIRequest(command, session, method, path, requestOptions, options.dependencies.BuildInfo)
	})
	resultErr := prompter.ShowTextScreen("API request", outputScreenLines(method+" "+path, stdout, stderr, callErr), commandOutputScreenOptions("API Request", state, nil))
	if errors.Is(resultErr, interactive.ErrBack) {
		return interactive.ErrBack
	}
	return resultErr
}

func rawAPIRequestScreenOptions(state homeState, validation string) interactive.ScreenOptions {
	_ = validation
	return interactive.ScreenOptions{
		Breadcrumb:  []string{"Home", "API", "Raw request"},
		Title:       "Raw API Request",
		Header:      sessionHeaderLines(state),
		LeftTitle:   "Request Steps",
		RightTitle:  "Values & Details",
		LeftWidth:   58,
		ActionLabel: "Send request",
		Footer:      sessionFooterLines(),
	}
}

func applyRawAPIRequestFields(fields []interactive.Field) (string, string, apiRequestOptions, error) {
	values := fieldValueMap(fields)
	if !parseYesNo(values["send"]) {
		return "", "", apiRequestOptions{}, interactive.ErrBack
	}
	method := strings.ToUpper(strings.TrimSpace(values["method"]))
	if method == "" {
		return "", "", apiRequestOptions{}, fmt.Errorf("HTTP method is required")
	}
	path := strings.TrimSpace(values["path"])
	if path == "" {
		return "", "", apiRequestOptions{}, fmt.Errorf("concrete path is required")
	}
	if strings.TrimSpace(values["data"]) != "" && strings.TrimSpace(values["dataRaw"]) != "" {
		return "", "", apiRequestOptions{}, fmt.Errorf("Body file and Literal body are mutually exclusive")
	}
	return method, path, apiRequestOptions{
		dataPath:    strings.TrimSpace(values["data"]),
		dataRaw:     values["dataRaw"],
		headers:     splitPromptLines(values["headers"]),
		contentType: strings.TrimSpace(values["contentType"]),
		accept:      strings.TrimSpace(values["accept"]),
		outputFile:  strings.TrimSpace(values["outputFile"]),
		noAuth:      !parseYesNo(values["auth"]),
		showHeaders: parseYesNo(values["showHeaders"]),
	}, nil
}

func runInteractiveAPIRoutes(command *cobra.Command, prompter *interactive.Prompter, state homeState) error {
	fields := []interactive.Field{
		{Name: "domain", Label: "Domain filter", Description: "Optional API domain filter, such as monitoring, auth, mcp, or pipelines.", Example: "monitoring"},
		{Name: "method", Label: "Method filter", Description: "Optional HTTP method filter.", Example: "GET"},
		{Name: "audience", Label: "Audience", Value: "all", Default: "all", Required: true, Description: "Route audience filter. Supported values: all, operator, public, internal.", Example: "public"},
		{Name: "output", Label: "Output format", Value: "text", Default: "text", Required: true, Description: "Output format for route results. Supported values: text, json, yaml.", Example: "json"},
	}
	edited, err := prompter.EditFieldsScreen("API routes", fields, apiRoutesScreenOptions(state))
	if err != nil {
		return err
	}
	values := fieldValueMap(edited)
	output, err := requireChoiceValue(values["output"], "Output format", "text", "json", "yaml")
	if err != nil {
		return err
	}
	routes, err := filterRoutes(apicatalog.Routes(), values["domain"], values["method"], values["audience"])
	if err != nil {
		return err
	}
	stdout, stderr, renderErr := captureCommandOutput(command, func() error {
		return renderRoutes(command, routes, output)
	})
	resultErr := prompter.ShowTextScreen("API routes", outputScreenLines("API routes", stdout, stderr, renderErr), commandOutputScreenOptions("API Routes", state, nil))
	if errors.Is(resultErr, interactive.ErrBack) {
		return interactive.ErrBack
	}
	return resultErr
}

func apiRoutesScreenOptions(state homeState) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb:  []string{"Home", "API", "Routes"},
		Title:       "API Routes",
		Header:      sessionHeaderLines(state),
		LeftTitle:   "Route Filters",
		RightTitle:  "Values & Details",
		LeftWidth:   48,
		ActionLabel: "List routes",
		Footer:      sessionFooterLines(),
	}
}

func runInteractiveAPIDescribe(command *cobra.Command, prompter *interactive.Prompter, state homeState) error {
	routes := apicatalog.Routes()
	choices := apiRouteChoices(routes)
	var (
		selected int
		err      error
	)
	if prompter.CanUseLiveSelector() {
		selected, err = prompter.ChooseScreen("Describe route", choices, apiRouteScreenOptions(routes, state))
	} else {
		selected, err = prompter.Choose("Describe route", choices)
	}
	if err != nil {
		return err
	}
	route := routes[selected]
	fields := []interactive.Field{{
		Name:        "output",
		Label:       "Output format",
		Value:       "text",
		Default:     "text",
		Required:    true,
		Description: "Output format for route detail. Supported values: text, json, yaml.",
		Example:     "text",
	}}
	edited, err := prompter.EditFieldsScreen(route.Method+" "+route.Path, fields, apiDescribeScreenOptions(route, state))
	if err != nil {
		return err
	}
	output, err := requireChoiceValue(fieldValueMap(edited)["output"], "Output format", "text", "json", "yaml")
	if err != nil {
		return err
	}
	stdout, stderr, renderErr := captureCommandOutput(command, func() error {
		if output == "text" {
			return renderRouteDetail(command, route)
		}
		return renderRoutes(command, []apicatalog.Route{route}, output)
	})
	resultErr := prompter.ShowTextScreen("API describe", outputScreenLines(route.Method+" "+route.Path, stdout, stderr, renderErr), commandOutputScreenOptions("API Describe", state, nil))
	if errors.Is(resultErr, interactive.ErrBack) {
		return interactive.ErrBack
	}
	return resultErr
}

func apiDescribeScreenOptions(route apicatalog.Route, state homeState) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb:  []string{"Home", "API", "Describe"},
		Title:       "API Describe",
		Header:      sessionHeaderLines(state),
		LeftTitle:   "Describe Steps",
		RightTitle:  "Values & Details",
		LeftWidth:   42,
		ActionLabel: "Render route detail",
		Footer:      sessionFooterLines(),
	}
}

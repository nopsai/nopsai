package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"nopsai/internal/cli/apicatalog"
	"nopsai/internal/cli/interactive"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newAPICallCommand(options *rootOptions) *cobra.Command {
	requestOptions := &apiRequestOptions{}
	var pathValues []string
	var queryValues []string
	var interactiveMode bool
	command := &cobra.Command{
		Use:   "call [METHOD ROUTE_TEMPLATE]",
		Short: "Invoke a registered route with safe path-template expansion",
		Args: func(command *cobra.Command, args []string) error {
			if interactiveMode || len(args) == 0 {
				if len(args) != 0 {
					return fmt.Errorf("--interactive does not accept METHOD or ROUTE_TEMPLATE arguments")
				}
				return nil
			}
			return cobra.ExactArgs(2)(command, args)
		},
		RunE: func(command *cobra.Command, args []string) error {
			if interactiveMode || len(args) == 0 {
				return executeInteractiveAPICall(command, options, requestOptions)
			}
			return executeCatalogAPICall(command, options, strings.ToUpper(args[0]), args[1], pathValues, queryValues, *requestOptions)
		},
	}
	command.Flags().BoolVar(&interactiveMode, "interactive", false, "search the API catalog, select a route, and prompt for parameters before calling it")
	command.Flags().StringArrayVarP(&pathValues, "path", "p", nil, "path template value as NAME=VALUE; repeat for every route parameter")
	command.Flags().StringArrayVarP(&queryValues, "query", "q", nil, "query string value as NAME=VALUE; repeat to preserve duplicate names")
	addAPIRequestFlags(command, requestOptions)
	return command
}

func executeCatalogAPICall(command *cobra.Command, options *rootOptions, method, pathTemplate string, pathValues, queryValues []string, requestOptions apiRequestOptions) error {
	route, ok := apicatalog.Find(method, pathTemplate)
	if !ok {
		return fmt.Errorf("route %s %s is not registered; use `nopsai api routes`", method, pathTemplate)
	}
	parameters, err := parseUniqueAssignments(pathValues, "path parameter")
	if err != nil {
		return err
	}
	if err := validateCatalogPathParameters(route, parameters); err != nil {
		return err
	}
	path, err := route.Expand(parameters)
	if err != nil {
		return err
	}
	query, err := parseQueryAssignments(queryValues)
	if err != nil {
		return err
	}
	if err := validateCatalogQueryParameters(route, query); err != nil {
		return err
	}
	if route.Body != nil && route.Body.Required && !apiRequestBodyConfigured(requestOptions) {
		return missingCatalogBodyError(route)
	}
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	session, err := options.resolveSessionWithToken(false, !requestOptions.noAuth)
	if err != nil {
		return err
	}
	return executeAPIRequest(command, session, route.Method, path, requestOptions, options.dependencies.BuildInfo)
}

func executeInteractiveAPICall(command *cobra.Command, options *rootOptions, requestOptions *apiRequestOptions) error {
	prompter := interactive.NewPrompter(command.InOrStdin(), command.OutOrStdout())
	routes := apicatalog.Routes()
	choices := apiRouteChoices(routes)
	for {
		var selected int
		var err error
		live := prompter.CanUseLiveSelector()
		if live {
			state := collectHomeState(command.Context(), options)
			selected, err = prompter.ChooseScreen("API route", choices, apiRouteScreenOptions(routes, state))
		} else {
			selected, err = prompter.Choose("API route", choices)
		}
		if errors.Is(err, interactive.ErrBack) {
			return nil
		}
		if err != nil {
			return err
		}
		route := routes[selected]
		if live {
			err := executeInteractiveAPIRoute(command, options, prompter, route, requestOptions)
			if errors.Is(err, interactive.ErrBack) {
				continue
			}
			return err
		}
		if err := renderRouteDetail(command, route); err != nil {
			return err
		}
		return executeInteractiveAPIRouteLine(command, options, prompter, route, requestOptions)
	}
}

func executeInteractiveAPIRouteLine(command *cobra.Command, options *rootOptions, prompter *interactive.Prompter, route apicatalog.Route, requestOptions *apiRequestOptions) error {
	parameters := make([]string, 0, len(route.PathParameters))
	for _, parameter := range route.PathParameters {
		label := "Path parameter " + parameter.Name
		if parameter.CatchAll {
			label += " (may include /)"
		}
		value, err := prompter.AskRequired(label, "")
		if err != nil {
			return err
		}
		parameters = append(parameters, parameter.Name+"="+value)
	}
	queryValues := make([]string, 0, len(route.QueryParameters))
	for _, parameter := range route.QueryParameters {
		if !parameter.Required {
			continue
		}
		label := "Required query parameter " + parameter.Name
		if strings.TrimSpace(parameter.Description) != "" {
			label += " (" + strings.TrimSpace(parameter.Description) + ")"
		}
		value, err := prompter.AskRequired(label, "")
		if err != nil {
			return err
		}
		queryValues = append(queryValues, parameter.Name+"="+value)
	}
	queryPrompt := "Query parameters (comma-separated NAME=VALUE)"
	if hint := queryPromptHint(route.QueryParameters); hint != "" {
		queryPrompt += "; known: " + hint
	}
	queryRaw, err := prompter.Ask(queryPrompt, "")
	if err != nil {
		return err
	}
	queryValues = append(queryValues, splitPromptList(queryRaw)...)
	if routeAllowsRequestBody(route.Method) {
		bodyPrompt := "Request body file (- for stdin, blank for none)"
		if route.Body != nil && route.Body.Required {
			bodyPrompt = "Request body file (- for stdin, blank to paste literal body)"
		}
		dataPath, err := prompter.Ask(bodyPrompt, requestOptions.dataPath)
		if err != nil {
			return err
		}
		requestOptions.dataPath = strings.TrimSpace(dataPath)
		if requestOptions.dataPath == "" {
			bodyPrompt = "Literal request body (blank for none)"
			if route.Body != nil && route.Body.Required {
				bodyPrompt = "Literal request body (required when no body file is provided)"
			}
			var dataRaw string
			var err error
			if route.Body != nil && route.Body.Required {
				dataRaw, err = prompter.AskRequired(bodyPrompt, requestOptions.dataRaw)
			} else {
				dataRaw, err = prompter.Ask(bodyPrompt, requestOptions.dataRaw)
			}
			if err != nil {
				return err
			}
			requestOptions.dataRaw = dataRaw
		}
	}
	accept, err := prompter.Ask("Response format / HTTP Accept (blank for default application/json)", requestOptions.accept)
	if err != nil {
		return err
	}
	requestOptions.accept = strings.TrimSpace(accept)
	attachTokenDefault := !route.Public
	attachToken, err := prompter.Confirm("Attach bearer token", attachTokenDefault)
	if err != nil {
		return err
	}
	requestOptions.noAuth = !attachToken
	return executeCatalogAPICall(command, options, route.Method, route.Path, parameters, queryValues, *requestOptions)
}

func executeInteractiveAPIRoute(command *cobra.Command, options *rootOptions, prompter *interactive.Prompter, route apicatalog.Route, requestOptions *apiRequestOptions) error {
	state := collectHomeState(command.Context(), options)
	fields := apiRequestFields(route, *requestOptions)
	var (
		parameters  []string
		queryValues []string
		formOptions = apiRequestFormScreenOptions(route, state, "")
	)
	for {
		edited, err := prompter.EditFieldsScreen(route.Method+" "+route.Path, fields, formOptions)
		if err != nil {
			return err
		}
		fields = edited
		var applyErr error
		parameters, queryValues, applyErr = applyAPIRequestFields(route, fields, requestOptions)
		if applyErr == nil {
			break
		}
		if errors.Is(applyErr, interactive.ErrBack) {
			return interactive.ErrBack
		}
		formOptions = apiRequestFormScreenOptions(route, state, applyErr.Error())
	}
	stdout, stderr, callErr := captureCommandOutput(command, func() error {
		return executeCatalogAPICall(command, options, route.Method, route.Path, parameters, queryValues, *requestOptions)
	})
	resultErr := prompter.ShowTextScreen(route.Method+" "+route.Path, apiResponseScreenLines(route, stdout, stderr, callErr), apiResultScreenOptions(route, state))
	if errors.Is(resultErr, interactive.ErrBack) {
		return interactive.ErrBack
	}
	return resultErr
}

func apiResponseScreenLines(route apicatalog.Route, stdout, stderr string, err error) []string {
	lines := []string{"Request", "Method: " + route.Method, "Route: " + route.Path, "", "Result"}
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
		lines = append(lines, "", "Response body")
		lines = append(lines, prettyResponseBodyLines(stdout)...)
	} else if err == nil {
		lines = append(lines, "", "Response body", "(empty)")
	}
	return lines
}

func prettyResponseBodyLines(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var pretty bytes.Buffer
	if json.Indent(&pretty, []byte(raw), "", "  ") == nil {
		return splitOutputLines(pretty.String())
	}
	return splitOutputLines(raw)
}

func apiRouteScreenOptions(routes []apicatalog.Route, state homeState) interactive.ScreenOptions {
	header := []string{
		fmt.Sprintf("Context: %s | API: %s", valueOrDefault(state.ContextName, "not selected"), valueOrDefault(state.API, "not configured")),
		fmt.Sprintf("User: %s | Token: %s | Health: %s", valueOrDefault(state.User, "not authenticated"), state.Token, homeHealthSummary(state.Checks)),
	}
	return interactive.ScreenOptions{
		Title:      "API Catalog",
		LeftTitle:  "Routes",
		RightTitle: "Route Detail",
		LeftWidth:  76,
		Header:     header,
		Footer: []string{
			"Keys: type filter | Up/Down move | PgUp/PgDn jump | Enter select | Esc home | Ctrl+C quit",
			"Tip: required path, query, and body guidance is shown before the request is sent.",
		},
		DetailTitle: func(index int, _ interactive.Choice) string {
			if index < 0 || index >= len(routes) {
				return ""
			}
			route := routes[index]
			return route.Method + " " + route.Path
		},
		Detail: func(index int, _ interactive.Choice) []string {
			if index < 0 || index >= len(routes) {
				return nil
			}
			return routeDetailLines(routes[index])
		},
	}
}

func apiRequestFormScreenOptions(route apicatalog.Route, state homeState, validation string) interactive.ScreenOptions {
	header := []string{
		fmt.Sprintf("Route: %s %s | Workflow: request inputs", route.Method, route.Path),
		fmt.Sprintf("Context: %s | API: %s | User: %s", valueOrDefault(state.ContextName, "not selected"), valueOrDefault(state.API, "not configured"), valueOrDefault(state.User, "not authenticated")),
		fmt.Sprintf("Auth default: %s | Response: JSON unless response format is set", apiAuthDefaultLabel(route)),
	}
	if strings.TrimSpace(validation) != "" {
		header = append(header, "Validation: "+strings.TrimSpace(validation))
	}
	return interactive.ScreenOptions{
		Title:  "API Request Wizard",
		Header: header,
		Sidebar: []string{
			"Workflow",
			"Home > API Catalog",
			"API Request",
			"",
			"Route",
			route.Method + " " + route.Path,
			"",
			"Session",
			"Context: " + valueOrDefault(state.ContextName, "not selected"),
			"API: " + valueOrDefault(state.API, "not configured"),
			"User: " + valueOrDefault(state.User, "not authenticated"),
			"Token: " + state.Token,
		},
		LeftTitle:   "Steps & Parameters",
		RightTitle:  "Values & Details",
		LeftWidth:   56,
		ActionLabel: "Send request",
		Footer: []string{
			"Edit: type/backspace | Next: Enter or Tab | Send: Ctrl+S | Back: Esc routes | Quit: Ctrl+C",
			"Multiline: Enter new line | Next step: Tab | Result: pretty JSON when possible",
		},
	}
}

func apiResultScreenOptions(route apicatalog.Route, state homeState) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Title: "API Response",
		Header: []string{
			fmt.Sprintf("%s %s", route.Method, route.Path),
			fmt.Sprintf("API: %s | User: %s | Token: %s", valueOrDefault(state.API, "not configured"), valueOrDefault(state.User, "not authenticated"), state.Token),
		},
		Footer: []string{
			"Keys: Up/Down scroll | PgUp/PgDn jump | Home/End | Enter home | Esc routes | Ctrl+C quit",
		},
	}
}

func apiRequestFields(route apicatalog.Route, requestOptions apiRequestOptions) []interactive.Field {
	fields := make([]interactive.Field, 0, len(route.PathParameters)+len(route.QueryParameters)+6)
	for _, parameter := range route.PathParameters {
		description := "Required path value used to expand the registered route template."
		if parameter.CatchAll {
			description += " This parameter may include slash-separated path segments."
		}
		fields = append(fields, interactive.Field{
			Name:        "path." + parameter.Name,
			Label:       "Path parameter: " + parameter.Name,
			Required:    true,
			Description: description,
			Example:     parameter.Example,
		})
	}
	for _, parameter := range route.QueryParameters {
		description := strings.TrimSpace(parameter.Description)
		if description == "" {
			description = "Catalogued query parameter for this route."
		}
		if parameter.Repeatable {
			description += " Repeat with the optional query assignments field when multiple values are needed."
		}
		fields = append(fields, interactive.Field{
			Name:        "query." + parameter.Name,
			Label:       queryStepLabel(parameter),
			Required:    parameter.Required,
			Description: description,
			Example:     parameter.Example,
		})
	}
	if hasOptionalQueryParameters(route.QueryParameters) {
		fields = append(fields, interactive.Field{
			Name:        "query.extra",
			Label:       "Additional query values",
			Multiline:   true,
			Description: "Optional or repeatable query assignments. Use one NAME=VALUE per line, or comma-separate short values. Leave blank to skip.",
			Example:     queryPromptHint(route.QueryParameters),
		})
	}
	if routeExpectsRequestBody(route, requestOptions) {
		bodyDescription := "Optional request body file path. Leave blank to paste literal content in the next step."
		if route.Body != nil && route.Body.Required {
			bodyDescription = "Request content is required. Provide a file path here or paste literal content in the next field."
		}
		fields = append(fields,
			interactive.Field{
				Name:        "body.file",
				Label:       "Payload source: file",
				Value:       requestOptions.dataPath,
				Description: bodyDescription,
				Example:     "payload.json",
			},
			interactive.Field{
				Name:        "body.raw",
				Label:       "Payload editor",
				Value:       requestOptions.dataRaw,
				Multiline:   true,
				Description: apiBodyDescription(route),
				Example:     apiBodyExample(route),
			},
			interactive.Field{
				Name:        "contentType",
				Label:       "Payload media type",
				Value:       requestOptions.contentType,
				Description: "Overrides the request Content-Type header. Blank defaults to application/json when a body is present.",
				Example:     strings.Join(apiBodyContentTypes(route), ", "),
			},
		)
	}
	if route.Streaming || route.Download || strings.TrimSpace(requestOptions.accept) != "" {
		fields = append(fields, interactive.Field{
			Name:        "accept",
			Label:       "Response format (HTTP Accept)",
			Value:       requestOptions.accept,
			Description: "Optional HTTP Accept header. Leave blank for JSON defaults; set this for streams, downloads, YAML, or other formats.",
			Example:     "application/json",
		})
	}
	fields = append(fields, interactive.Field{
		Name:        "auth",
		Label:       "Attach bearer token",
		Value:       formatYesNo(!requestOptions.noAuth && !route.Public),
		Default:     formatYesNo(!route.Public),
		Kind:        interactive.FieldBoolean,
		Description: "Attach the configured bearer token for this context. Public routes default to no; operator/internal routes default to yes.",
		Example:     "yes",
	})
	fields = append(fields, interactive.Field{
		Name:        "send",
		Label:       "Send request",
		Value:       "yes",
		Default:     "yes",
		Kind:        interactive.FieldBoolean,
		Description: "Final review gate. Leave yes and press Enter or Ctrl+S to send the request. Change to no, or press Esc, to return to the route catalog without sending.",
		Example:     "yes",
	})
	return fields
}

func applyAPIRequestFields(route apicatalog.Route, fields []interactive.Field, requestOptions *apiRequestOptions) ([]string, []string, error) {
	values := map[string]string{}
	for _, field := range fields {
		values[field.Name] = field.Value
	}
	parameters := make([]string, 0, len(route.PathParameters))
	for _, parameter := range route.PathParameters {
		parameters = append(parameters, parameter.Name+"="+strings.TrimSpace(values["path."+parameter.Name]))
	}
	queryValues := make([]string, 0, len(route.QueryParameters))
	for _, parameter := range route.QueryParameters {
		value := strings.TrimSpace(values["query."+parameter.Name])
		if value != "" {
			queryValues = append(queryValues, parameter.Name+"="+value)
		}
	}
	extraQuery := strings.TrimSpace(values["query.extra"])
	if extraQuery != "" {
		for _, assignment := range splitPromptList(extraQuery) {
			if !strings.Contains(assignment, "=") {
				return nil, nil, fmt.Errorf("query assignment %q must use NAME=VALUE", assignment)
			}
			queryValues = append(queryValues, assignment)
		}
	}
	if routeAllowsRequestBody(route.Method) {
		requestOptions.dataPath = strings.TrimSpace(values["body.file"])
		requestOptions.dataRaw = values["body.raw"]
		requestOptions.contentType = strings.TrimSpace(values["contentType"])
		if route.Body != nil && route.Body.Required && strings.TrimSpace(requestOptions.dataPath) == "" && strings.TrimSpace(requestOptions.dataRaw) == "" {
			return nil, nil, fmt.Errorf("request body is required; provide Payload source: file or Payload editor")
		}
	}
	requestOptions.accept = strings.TrimSpace(values["accept"])
	requestOptions.noAuth = !parseYesNo(values["auth"])
	if !parseYesNo(values["send"]) {
		return nil, nil, interactive.ErrBack
	}
	return parameters, queryValues, nil
}

func queryStepLabel(parameter apicatalog.QueryParameter) string {
	if parameter.Required {
		return "Required query: " + parameter.Name
	}
	return "Optional query: " + parameter.Name
}

func hasOptionalQueryParameters(parameters []apicatalog.QueryParameter) bool {
	for _, parameter := range parameters {
		if !parameter.Required {
			return true
		}
	}
	return false
}

func routeExpectsRequestBody(route apicatalog.Route, requestOptions apiRequestOptions) bool {
	return route.Body != nil ||
		strings.TrimSpace(requestOptions.dataPath) != "" ||
		strings.TrimSpace(requestOptions.dataRaw) != ""
}

func apiAuthDefaultLabel(route apicatalog.Route) string {
	if route.Public {
		return "no token"
	}
	return "configured token"
}

func apiBodyDescription(route apicatalog.Route) string {
	description := "Literal request body bytes."
	if route.Body != nil {
		if strings.TrimSpace(route.Body.Description) != "" {
			description = strings.TrimSpace(route.Body.Description)
		}
		if len(route.Body.ContentTypes) > 0 {
			description += " Expected content type: " + strings.Join(route.Body.ContentTypes, ", ") + "."
		}
		if route.Body.Required {
			description += " Required when no body file is provided."
		}
	}
	return description
}

func apiBodyExample(route apicatalog.Route) string {
	if route.Body != nil && strings.TrimSpace(route.Body.Example) != "" {
		return strings.TrimSpace(route.Body.Example)
	}
	return `{"key":"value"}`
}

func apiBodyContentTypes(route apicatalog.Route) []string {
	if route.Body != nil && len(route.Body.ContentTypes) > 0 {
		return route.Body.ContentTypes
	}
	return []string{"application/json"}
}

func apiRouteChoices(routes []apicatalog.Route) []interactive.Choice {
	choices := make([]interactive.Choice, 0, len(routes))
	for _, route := range routes {
		flags := routeFlags(route)
		description := "domain=" + route.Domain
		if flags != "" {
			description += ", " + flags
		}
		choices = append(choices, interactive.Choice{
			Label:       fmt.Sprintf("%-7s %s", route.Method, route.Path),
			Description: description,
			SearchText:  strings.Join([]string{route.Method, route.Path, route.Domain, flags, parameterSearchText(route.PathParameters)}, " "),
		})
	}
	return choices
}

func parameterSearchText(parameters []apicatalog.Parameter) string {
	names := make([]string, 0, len(parameters))
	for _, parameter := range parameters {
		names = append(names, parameter.Name)
	}
	return strings.Join(names, " ")
}

func newAPIRoutesCommand() *cobra.Command {
	var domain string
	var method string
	var audience string
	var output string
	command := &cobra.Command{
		Use:   "routes",
		Short: "List every API route compiled into this CLI",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			routes, err := filterRoutes(apicatalog.Routes(), domain, method, audience)
			if err != nil {
				return err
			}
			return renderRoutes(command, routes, output)
		},
	}
	command.Flags().StringVar(&domain, "domain", "", "filter by API domain")
	command.Flags().StringVar(&method, "method", "", "filter by HTTP method")
	command.Flags().StringVar(&audience, "audience", "all", "filter by audience: all, operator, public, or internal")
	command.Flags().StringVarP(&output, "output", "o", "text", "output format: text, json, or yaml")
	return command
}

func newAPIDescribeCommand() *cobra.Command {
	var output string
	command := &cobra.Command{
		Use:   "describe METHOD ROUTE_TEMPLATE",
		Short: "Describe a registered API route",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			route, ok := apicatalog.Find(args[0], args[1])
			if !ok {
				return fmt.Errorf("route %s %s is not registered", strings.ToUpper(args[0]), args[1])
			}
			if strings.EqualFold(strings.TrimSpace(output), "text") {
				return renderRouteDetail(command, route)
			}
			return renderRoutes(command, []apicatalog.Route{route}, output)
		},
	}
	command.Flags().StringVarP(&output, "output", "o", "yaml", "output format: text, json, or yaml")
	return command
}

func filterRoutes(routes []apicatalog.Route, domain, method, audience string) ([]apicatalog.Route, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	method = strings.ToUpper(strings.TrimSpace(method))
	audience = strings.ToLower(strings.TrimSpace(audience))
	if audience == "" {
		audience = "all"
	}
	if audience != "all" && audience != "operator" && audience != "public" && audience != "internal" {
		return nil, fmt.Errorf("unsupported audience %q", audience)
	}
	filtered := make([]apicatalog.Route, 0, len(routes))
	for _, route := range routes {
		if domain != "" && route.Domain != domain {
			continue
		}
		if method != "" && route.Method != method {
			continue
		}
		switch audience {
		case "operator":
			if route.Internal || route.Public {
				continue
			}
		case "public":
			if !route.Public {
				continue
			}
		case "internal":
			if !route.Internal {
				continue
			}
		}
		filtered = append(filtered, route)
	}
	return filtered, nil
}

func renderRoutes(command *cobra.Command, routes []apicatalog.Route, output string) error {
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "text":
		for _, route := range routes {
			flags := routeFlags(route)
			if flags != "" {
				flags = " [" + flags + "]"
			}
			if _, err := fmt.Fprintf(command.OutOrStdout(), "%-7s %s%s\n", route.Method, route.Path, flags); err != nil {
				return err
			}
		}
		return nil
	case "json":
		encoder := json.NewEncoder(command.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(routes)
	case "yaml":
		encoder := yaml.NewEncoder(command.OutOrStdout())
		defer encoder.Close()
		encoder.SetIndent(2)
		return encoder.Encode(routes)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func routeFlags(route apicatalog.Route) string {
	flags := make([]string, 0, 4)
	if route.Public {
		flags = append(flags, "public")
	}
	if route.Internal {
		flags = append(flags, "internal")
	}
	if route.Streaming {
		flags = append(flags, "stream")
	}
	if route.Download {
		flags = append(flags, "download")
	}
	if len(route.PathParameters) > 0 {
		flags = append(flags, "path:"+strings.Join(pathParameterNames(route.PathParameters), ","))
	}
	if len(route.QueryParameters) > 0 {
		flags = append(flags, "query")
	}
	if route.Body != nil {
		if route.Body.Required {
			flags = append(flags, "body:required")
		} else {
			flags = append(flags, "body")
		}
	}
	return strings.Join(flags, ",")
}

func renderRouteDetail(command *cobra.Command, route apicatalog.Route) error {
	out := command.OutOrStdout()
	for _, line := range routeDetailLines(route) {
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

func routeDetailLines(route apicatalog.Route) []string {
	audience := "operator"
	switch {
	case route.Public:
		audience = "public"
	case route.Internal:
		audience = "internal"
	}
	lines := []string{
		route.Method + " " + route.Path,
		"",
		"Overview",
		routeDetailRow("Domain", route.Domain),
		routeDetailRow("Audience", audience),
	}
	if route.Streaming || route.Download {
		parts := make([]string, 0, 2)
		if route.Streaming {
			parts = append(parts, "streaming")
		}
		if route.Download {
			parts = append(parts, "download")
		}
		lines = append(lines, routeDetailRow("Transport", strings.Join(parts, ", ")))
	}
	lines = append(lines, "")
	if len(route.PathParameters) == 0 {
		lines = append(lines, "Path parameters: none")
	} else {
		lines = append(lines, "Path parameters:")
		for _, parameter := range route.PathParameters {
			detail := "required"
			if parameter.CatchAll {
				detail += ", may include /"
			}
			if parameter.Example != "" {
				detail += ", example " + parameter.Example
			}
			lines = append(lines, fmt.Sprintf("  - %-24s %s", parameter.Name, "("+detail+")"))
		}
	}
	lines = append(lines, "")
	if len(route.QueryParameters) == 0 {
		lines = append(lines, "Query parameters: none catalogued")
	} else {
		lines = append(lines, "Query parameters:")
		for _, parameter := range route.QueryParameters {
			detail := "optional"
			if parameter.Required {
				detail = "required"
			}
			if parameter.Repeatable {
				detail += ", repeatable"
			}
			if parameter.Example != "" {
				detail += ", example " + parameter.Example
			}
			if strings.TrimSpace(parameter.Description) != "" {
				detail += "; " + strings.TrimSpace(parameter.Description)
			}
			lines = append(lines, fmt.Sprintf("  - %-24s %s", parameter.Name, "("+detail+")"))
		}
	}
	lines = append(lines, "")
	if route.Body == nil {
		lines = append(lines, "Body: none")
	} else {
		requirement := "optional"
		if route.Body.Required {
			requirement = "required"
		}
		body := "Body: " + requirement
		if len(route.Body.ContentTypes) > 0 {
			body += " (" + strings.Join(route.Body.ContentTypes, ", ") + ")"
		}
		lines = append(lines, body)
		if strings.TrimSpace(route.Body.Description) != "" {
			lines = append(lines, routeDetailRow("Description", strings.TrimSpace(route.Body.Description)))
		}
		if strings.TrimSpace(route.Body.Example) != "" {
			lines = append(lines, "", "  Example:")
			for _, line := range strings.Split(strings.TrimRight(route.Body.Example, "\n"), "\n") {
				lines = append(lines, "    "+line)
			}
		}
	}
	if len(route.Examples) > 0 {
		lines = append(lines, "", "Examples:")
		for index, example := range route.Examples {
			lines = append(lines, fmt.Sprintf("  %d. %s", index+1, example.Description), "     "+example.Command)
		}
	}
	lines = append(lines, "")
	return lines
}

func routeDetailRow(label, value string) string {
	return fmt.Sprintf("  %-14s %s", label+":", strings.TrimSpace(value))
}

func indentBlock(value, prefix string) string {
	lines := strings.Split(strings.TrimRight(value, "\n"), "\n")
	for index := range lines {
		lines[index] = prefix + lines[index]
	}
	return strings.Join(lines, "\n")
}

func pathParameterNames(parameters []apicatalog.Parameter) []string {
	names := make([]string, 0, len(parameters))
	for _, parameter := range parameters {
		names = append(names, parameter.Name)
	}
	return names
}

func queryPromptHint(parameters []apicatalog.QueryParameter) string {
	hints := make([]string, 0, len(parameters))
	for _, parameter := range parameters {
		if parameter.Required {
			continue
		}
		value := parameter.Name
		if parameter.Example != "" {
			value += "=" + parameter.Example
		}
		hints = append(hints, value)
		if len(hints) == 4 {
			break
		}
	}
	return strings.Join(hints, ", ")
}

func parseUniqueAssignments(values []string, label string) (map[string]string, error) {
	result := make(map[string]string, len(values))
	for _, assignment := range values {
		name, value, ok := strings.Cut(assignment, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("invalid %s %q; expected NAME=VALUE", label, assignment)
		}
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("duplicate %s %q", label, name)
		}
		result[name] = value
	}
	return result, nil
}

func validateCatalogPathParameters(route apicatalog.Route, values map[string]string) error {
	allowed := make(map[string]apicatalog.Parameter, len(route.PathParameters))
	for _, parameter := range route.PathParameters {
		allowed[parameter.Name] = parameter
	}
	for name := range values {
		if _, ok := allowed[name]; !ok {
			allowedNames := strings.Join(pathParameterNames(route.PathParameters), ", ")
			if allowedNames == "" {
				allowedNames = "none"
			}
			return fmt.Errorf("route %s %s does not accept path parameter %q; allowed: %s. Use `nopsai api describe %s %s` for guidance",
				route.Method, route.Path, name, allowedNames, route.Method, shellQuoteForHelp(route.Path))
		}
	}
	missing := make([]apicatalog.Parameter, 0)
	for _, parameter := range route.PathParameters {
		if strings.TrimSpace(values[parameter.Name]) == "" {
			missing = append(missing, parameter)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	names := make([]string, 0, len(missing))
	flags := make([]string, 0, len(missing))
	for _, parameter := range missing {
		names = append(names, parameter.Name)
		example := parameter.Example
		if example == "" {
			example = "VALUE"
		}
		flags = append(flags, "--path "+parameter.Name+"="+example)
	}
	return fmt.Errorf("route %s %s requires path parameter(s): %s. Provide %s. Use `nopsai api describe %s %s` for examples",
		route.Method, route.Path, strings.Join(names, ", "), strings.Join(flags, " "), route.Method, shellQuoteForHelp(route.Path))
}

func validateCatalogQueryParameters(route apicatalog.Route, query url.Values) error {
	missing := make([]apicatalog.QueryParameter, 0)
	for _, parameter := range route.QueryParameters {
		if parameter.Required && strings.TrimSpace(query.Get(parameter.Name)) == "" {
			missing = append(missing, parameter)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	names := make([]string, 0, len(missing))
	flags := make([]string, 0, len(missing))
	for _, parameter := range missing {
		names = append(names, parameter.Name)
		example := parameter.Example
		if example == "" {
			example = "VALUE"
		}
		flags = append(flags, "--query "+parameter.Name+"="+example)
	}
	return fmt.Errorf("route %s %s requires query parameter(s): %s. Provide %s. Use `nopsai api describe %s %s` for examples",
		route.Method, route.Path, strings.Join(names, ", "), strings.Join(flags, " "), route.Method, shellQuoteForHelp(route.Path))
}

func apiRequestBodyConfigured(options apiRequestOptions) bool {
	return strings.TrimSpace(options.dataPath) != "" || options.dataRaw != ""
}

func missingCatalogBodyError(route apicatalog.Route) error {
	return fmt.Errorf("route %s %s expects request content. Provide --data payload.json or --data-raw content. Use `nopsai api describe %s %s` to see the expected shape",
		route.Method, route.Path, route.Method, shellQuoteForHelp(route.Path))
}

func shellQuoteForHelp(value string) string {
	if strings.ContainsAny(value, " \t'{}") {
		return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
	}
	return value
}

func parseQueryAssignments(values []string) (url.Values, error) {
	query := make(url.Values)
	for _, assignment := range values {
		name, value, ok := strings.Cut(assignment, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("invalid query parameter %q; expected NAME=VALUE", assignment)
		}
		query.Add(name, value)
	}
	return query, nil
}

func splitPromptList(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func routeAllowsRequestBody(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}

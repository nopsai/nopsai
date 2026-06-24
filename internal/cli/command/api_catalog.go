package command

import (
	"encoding/json"
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
	path, err := route.Expand(parameters)
	if err != nil {
		return err
	}
	query, err := parseQueryAssignments(queryValues)
	if err != nil {
		return err
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
	selected, err := prompter.Choose("API route", apiRouteChoices(routes))
	if err != nil {
		return err
	}
	route := routes[selected]
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
	queryRaw, err := prompter.Ask("Query parameters (comma-separated NAME=VALUE)", "")
	if err != nil {
		return err
	}
	queryValues := splitPromptList(queryRaw)
	if routeAllowsRequestBody(route.Method) {
		dataPath, err := prompter.Ask("Request body file (- for stdin, blank for none)", requestOptions.dataPath)
		if err != nil {
			return err
		}
		requestOptions.dataPath = strings.TrimSpace(dataPath)
		if requestOptions.dataPath == "" {
			dataRaw, err := prompter.Ask("Literal request body (blank for none)", requestOptions.dataRaw)
			if err != nil {
				return err
			}
			requestOptions.dataRaw = dataRaw
		}
	}
	accept, err := prompter.Ask("Accept header (blank for default application/json)", requestOptions.accept)
	if err != nil {
		return err
	}
	requestOptions.accept = strings.TrimSpace(accept)
	attachTokenDefault := !route.Public
	attachToken, err := prompter.Confirm("Attach configured bearer token", attachTokenDefault)
	if err != nil {
		return err
	}
	requestOptions.noAuth = !attachToken
	return executeCatalogAPICall(command, options, route.Method, route.Path, parameters, queryValues, *requestOptions)
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
	return strings.Join(flags, ",")
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
	parts := strings.Split(raw, ",")
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

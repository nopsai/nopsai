// Package apicatalog describes every HTTP route registered by the NopsAI API.
package apicatalog

//go:generate go run ./cmd/generate -root ../../.. -output catalog_generated.go

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

type Parameter struct {
	Name     string `json:"name" yaml:"name"`
	CatchAll bool   `json:"catch_all" yaml:"catch_all"`
}

type Route struct {
	Method         string      `json:"method" yaml:"method"`
	Path           string      `json:"path" yaml:"path"`
	Domain         string      `json:"domain" yaml:"domain"`
	Internal       bool        `json:"internal" yaml:"internal"`
	Public         bool        `json:"public" yaml:"public"`
	Streaming      bool        `json:"streaming" yaml:"streaming"`
	Download       bool        `json:"download" yaml:"download"`
	PathParameters []Parameter `json:"path_parameters,omitempty" yaml:"path_parameters,omitempty"`
}

func Routes() []Route {
	routes := make([]Route, len(generatedRoutes))
	for index, route := range generatedRoutes {
		routes[index] = route
		routes[index].PathParameters = append([]Parameter(nil), route.PathParameters...)
	}
	return routes
}

func Domains() []string {
	seen := make(map[string]struct{})
	for _, route := range generatedRoutes {
		seen[route.Domain] = struct{}{}
	}
	domains := make([]string, 0, len(seen))
	for domain := range seen {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	return domains
}

func Find(method, pathTemplate string) (Route, bool) {
	method = strings.ToUpper(strings.TrimSpace(method))
	pathTemplate = strings.TrimSpace(pathTemplate)
	for _, route := range generatedRoutes {
		if route.Method == method && route.Path == pathTemplate {
			copy := route
			copy.PathParameters = append([]Parameter(nil), route.PathParameters...)
			return copy, true
		}
	}
	return Route{}, false
}

func (r Route) Expand(values map[string]string) (string, error) {
	if strings.TrimSpace(r.Path) == "" {
		return "", errors.New("route path is empty")
	}
	allowed := make(map[string]Parameter, len(r.PathParameters))
	for _, parameter := range r.PathParameters {
		allowed[parameter.Name] = parameter
	}
	for name := range values {
		if _, ok := allowed[name]; !ok {
			return "", fmt.Errorf("unknown path parameter %q", name)
		}
	}
	path := r.Path
	for _, parameter := range r.PathParameters {
		value, ok := values[parameter.Name]
		if !ok || strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("missing path parameter %q", parameter.Name)
		}
		if parameter.CatchAll {
			for _, segment := range strings.Split(strings.Trim(value, "/"), "/") {
				if segment == "" {
					return "", fmt.Errorf("catch-all path parameter %q contains an empty segment", parameter.Name)
				}
			}
		}
		if !parameter.CatchAll && strings.Contains(value, "/") {
			return "", fmt.Errorf("path parameter %q cannot contain /", parameter.Name)
		}
		encoded := encodePathValue(value, parameter.CatchAll)
		placeholder := "{" + parameter.Name + "}"
		if parameter.CatchAll {
			placeholder = "{" + parameter.Name + "...}"
		}
		path = strings.Replace(path, placeholder, encoded, 1)
	}
	return path, nil
}

func newRoute(method, path string) Route {
	return Route{
		Method:         method,
		Path:           path,
		Domain:         domainForPath(path),
		Internal:       strings.HasPrefix(path, "/internal/") || strings.HasPrefix(path, "/v1/internal/"),
		Public:         publicPath(path),
		Streaming:      strings.HasSuffix(path, "/stream") || strings.HasSuffix(path, "/watch"),
		Download:       strings.HasSuffix(path, "/download") || strings.HasSuffix(path, ".zip"),
		PathParameters: pathParameters(path),
	}
}

func domainForPath(path string) string {
	if path == "/healthz" || path == "/metrics" || path == "/version" {
		return "platform"
	}
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) == 0 {
		return "platform"
	}
	if segments[0] == "internal" {
		return "internal"
	}
	if segments[0] == "v1" && len(segments) > 1 {
		if segments[1] == "internal" {
			return "internal"
		}
		return segments[1]
	}
	return segments[0]
}

func publicPath(path string) bool {
	switch path {
	case "/healthz", "/metrics", "/version", "/v1/auth/providers", "/v1/auth/discover", "/v1/auth/session/exchange", "/v1/auth/login", "/v1/auth/refresh", "/v1/auth/logout", "/v1/setup/preflight", "/v1/system/dispatcher/runner-bootstrap":
		return true
	default:
		return strings.HasPrefix(path, "/v1/auth/oidc/") || strings.HasPrefix(path, "/v1/git/webhooks/")
	}
}

func pathParameters(path string) []Parameter {
	parameters := make([]Parameter, 0)
	for start := strings.IndexByte(path, '{'); start >= 0; {
		remaining := path[start+1:]
		end := strings.IndexByte(remaining, '}')
		if end < 0 {
			break
		}
		name := remaining[:end]
		catchAll := strings.HasSuffix(name, "...")
		name = strings.TrimSuffix(name, "...")
		if name != "" {
			parameters = append(parameters, Parameter{Name: name, CatchAll: catchAll})
		}
		nextOffset := start + end + 2
		next := strings.IndexByte(path[nextOffset:], '{')
		if next < 0 {
			break
		}
		start = nextOffset + next
	}
	return parameters
}

func encodePathValue(value string, catchAll bool) string {
	if !catchAll {
		return url.PathEscape(value)
	}
	segments := strings.Split(strings.Trim(value, "/"), "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return strings.Join(segments, "/")
}

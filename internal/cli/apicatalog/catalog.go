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
	Name        string `json:"name" yaml:"name"`
	CatchAll    bool   `json:"catch_all" yaml:"catch_all"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Example     string `json:"example,omitempty" yaml:"example,omitempty"`
}

type QueryParameter struct {
	Name        string `json:"name" yaml:"name"`
	Required    bool   `json:"required" yaml:"required"`
	Repeatable  bool   `json:"repeatable,omitempty" yaml:"repeatable,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Example     string `json:"example,omitempty" yaml:"example,omitempty"`
}

type BodySpec struct {
	Required     bool     `json:"required" yaml:"required"`
	ContentTypes []string `json:"content_types,omitempty" yaml:"content_types,omitempty"`
	Description  string   `json:"description,omitempty" yaml:"description,omitempty"`
	Example      string   `json:"example,omitempty" yaml:"example,omitempty"`
}

type Example struct {
	Description string `json:"description" yaml:"description"`
	Command     string `json:"command" yaml:"command"`
}

type Route struct {
	Method          string           `json:"method" yaml:"method"`
	Path            string           `json:"path" yaml:"path"`
	Domain          string           `json:"domain" yaml:"domain"`
	Internal        bool             `json:"internal" yaml:"internal"`
	Public          bool             `json:"public" yaml:"public"`
	Streaming       bool             `json:"streaming" yaml:"streaming"`
	Download        bool             `json:"download" yaml:"download"`
	PathParameters  []Parameter      `json:"path_parameters,omitempty" yaml:"path_parameters,omitempty"`
	QueryParameters []QueryParameter `json:"query_parameters,omitempty" yaml:"query_parameters,omitempty"`
	Body            *BodySpec        `json:"body,omitempty" yaml:"body,omitempty"`
	Examples        []Example        `json:"examples,omitempty" yaml:"examples,omitempty"`
}

func Routes() []Route {
	routes := make([]Route, len(generatedRoutes))
	for index, route := range generatedRoutes {
		routes[index] = cloneRoute(route)
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
			return cloneRoute(route), true
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
	route := Route{
		Method:          method,
		Path:            path,
		Domain:          domainForPath(path),
		Internal:        strings.HasPrefix(path, "/internal/") || strings.HasPrefix(path, "/v1/internal/"),
		Public:          publicPath(path),
		Streaming:       strings.HasSuffix(path, "/stream") || strings.HasSuffix(path, "/watch"),
		Download:        strings.HasSuffix(path, "/download") || strings.HasSuffix(path, ".zip"),
		PathParameters:  pathParameters(path),
		QueryParameters: queryParameters(path),
		Body:            bodySpec(method, path),
	}
	route.Examples = routeExamples(route)
	return route
}

func cloneRoute(route Route) Route {
	copy := route
	copy.PathParameters = append([]Parameter(nil), route.PathParameters...)
	copy.QueryParameters = append([]QueryParameter(nil), route.QueryParameters...)
	copy.Examples = append([]Example(nil), route.Examples...)
	if route.Body != nil {
		body := *route.Body
		body.ContentTypes = append([]string(nil), route.Body.ContentTypes...)
		copy.Body = &body
	}
	return copy
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
			parameters = append(parameters, Parameter{
				Name:        name,
				CatchAll:    catchAll,
				Description: pathParameterDescription(name, catchAll),
				Example:     pathParameterExample(name, catchAll),
			})
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

func pathParameterDescription(name string, catchAll bool) string {
	if catchAll {
		return "Required path value; may contain slash-separated segments."
	}
	return "Required single path segment."
}

func pathParameterExample(name string, catchAll bool) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "pipeline", "pipelinename":
		return "delivery/release"
	case "step", "stepname":
		return "shared/checkout"
	case "knowledgeid":
		return "policy/team-1/release-evidence"
	case "connectionid":
		return "docs/confluence"
	case "repoowner":
		return "acme"
	case "reponame":
		return "payments-api"
	case "branch":
		return "main"
	case "teamid":
		return "team-1"
	case "runid":
		return "run-id"
	case "approvalid":
		return "approval-id"
	case "taskname":
		return "build"
	case "sourceid":
		return "dispatcher"
	case "profileid", "profilename":
		return "default"
	case "servername":
		return "github"
	case "secretname":
		return "DATABASE_URL"
	case "variablename":
		return "IMAGE_TAG"
	case "credentialid":
		return "credential-id"
	case "version":
		return "1"
	case "userid":
		return "user-id"
	case "serviceaccountid":
		return "service-account-id"
	case "tokenid":
		return "token-id"
	case "provider":
		return "nopsai"
	case "dashboardid":
		return "dashboard-id"
	case "scheduleid":
		return "schedule-id"
	case "ruleid":
		return "rule-id"
	case "recommendationid":
		return "recommendation-id"
	case "outputid":
		return "output-id"
	case "refreshid":
		return "refresh-id"
	case "publicationid":
		return "publication-id"
	case "sectionid":
		return "section-id"
	case "backupid":
		return "backup-id"
	case "grantid":
		return "grant-id"
	case "checkrunid":
		return "123456"
	case "service":
		return "dispatcher"
	}
	if catchAll {
		return "path/to/" + strings.TrimSpace(name)
	}
	return strings.TrimSpace(name) + "-value"
}

func queryParameters(path string) []QueryParameter {
	var parameters []QueryParameter
	add := func(name, description, example string) {
		for _, existing := range parameters {
			if existing.Name == name {
				return
			}
		}
		parameters = append(parameters, QueryParameter{Name: name, Description: description, Example: example})
	}
	addRequired := func(name, description, example string) {
		for index := range parameters {
			if parameters[index].Name == name {
				parameters[index].Required = true
				return
			}
		}
		parameters = append(parameters, QueryParameter{Name: name, Required: true, Description: description, Example: example})
	}
	addRepeatable := func(name, description, example string) {
		for index := range parameters {
			if parameters[index].Name == name {
				parameters[index].Repeatable = true
				return
			}
		}
		parameters = append(parameters, QueryParameter{Name: name, Repeatable: true, Description: description, Example: example})
	}

	switch path {
	case "/v1/runs":
		add("limit", "Maximum runs to return.", "100")
		add("offset", "Number of runs to skip.", "0")
		add("teamId", "Team numeric ID or root team marker.", "1")
		add("branch", "Filter runs by Git branch.", "main")
	case "/v1/audit", "/v1/auth/personal-tokens", "/v1/admin/users", "/v1/admin/service-accounts", "/v1/system/data/cleanup/jobs", "/v1/system/data/backups":
		add("limit", "Maximum records to return.", "100")
	case "/v1/access/grants":
		add("resource_type", "Filter grants by resource type.", "pipeline")
		add("resource_id", "Filter grants by resource ID.", "delivery/release")
		add("role", "Filter grants by role.", "operator")
	case "/v1/access/effective-permissions":
		addRequired("action", "Action to evaluate.", "pipeline.run")
		addRequired("resource_type", "Resource type to evaluate.", "pipeline")
		addRequired("resource_id", "Resource ID to evaluate.", "delivery/release")
	case "/v1/auth/oidc/{provider}/start":
		add("return_to", "Relative UI path to return to after login.", "/")
		add("prompt", "Optional OIDC prompt value.", "login")
	case "/v1/auth/oidc/{provider}/callback":
		addRequired("code", "OIDC authorization code returned by the provider.", "provider-code")
		addRequired("state", "Opaque OIDC state generated by the start endpoint.", "state")
	case "/v1/setup/templates", "/v1/setup/templates.zip":
		add("profile", "Starter profile to render.", "production")
	case "/v1/system/logs/sources/{sourceID}/stream":
		add("cursor", "Resume cursor from a previous log stream.", "cursor-token")
		add("lines", "Number of tail lines to read first.", "200")
		add("tail", "Alias for lines.", "200")
	case "/v1/system/dispatcher/runner-compose", "/v1/system/dispatcher/runner-bootstrap-command", "/v1/system/dispatcher/kubernetes-runner-manifest", "/v1/system/dispatcher/kubernetes-runner-bootstrap-command":
		add("dispatcher_grpc_address", "Dispatcher address visible from the runner host or cluster.", "dispatcher:9090")
	case "/internal/v1/runtime-config/{service}/watch":
		add("version", "Last seen runtime-config version.", "42")
		add("since_version", "Alias for version.", "42")
	case "/v1/knowledge-connections/{connectionID}/pages", "/v1/knowledge-context-connections/{connectionID}/pages/search":
		add("query", "Search text.", "release")
		add("q", "Alias for query.", "release")
		add("cursor", "Provider pagination cursor.", "cursor-token")
	case "/v1/knowledge-connections":
		add("team", "Filter by team path.", "team-1")
	case "/v1/dashboards":
		add("team", "Filter by team path.", "team-1")
		add("q", "Search dashboards by name.", "operations")
	}

	if strings.Contains(path, "/secrets") || strings.Contains(path, "/variables") {
		add("scope", "Runtime scope for the value.", "prod")
		add("include_source", "Include GitOps or database source metadata.", "true")
	}
	if strings.Contains(path, "/pipelines") || strings.Contains(path, "/steps") {
		add("include_source", "Include GitOps or database source metadata.", "true")
	}
	if strings.HasSuffix(path, "/logs") {
		add("since_line", "Only return log lines after this line number.", "100")
	}
	if strings.Contains(path, "/invocations") || strings.Contains(path, "/deliveries") || strings.Contains(path, "/history") || strings.Contains(path, "/refreshes") {
		add("limit", "Maximum records to return.", "100")
	}
	if strings.Contains(path, "/branches") {
		add("repoOwner", "Repository owner for provider-backed branch lookup.", "acme")
		add("repoName", "Repository name for provider-backed branch lookup.", "payments-api")
	}
	if strings.HasSuffix(path, "/config-repo/sync") || strings.HasSuffix(path, "/config/sync") || strings.HasSuffix(path, "/config-repository/sync") {
		add("dry_run", "Preview config sync without applying changes.", "true")
	}
	if strings.Contains(path, "/mcp/") && strings.HasSuffix(path, "/test") {
		add("scope", "Team or runtime scope to test.", "team-1")
	}
	if strings.Contains(path, "/llm-profiles/") && strings.HasSuffix(path, "/test") {
		add("scope", "Team or runtime scope to test.", "team-1")
	}
	if strings.Contains(path, "/agent-profiles/") && strings.HasSuffix(path, "/usage") {
		add("force", "Force delete/update when references exist.", "true")
	}
	if strings.Contains(path, "/knowledge") {
		addRepeatable("tag", "Optional repeated tag filter where supported.", "release")
	}
	return parameters
}

func bodySpec(method, path string) *BodySpec {
	if !bodyCapableMethod(method) {
		return nil
	}
	spec := &BodySpec{
		Required:     false,
		ContentTypes: []string{"application/json"},
		Description:  "This route can receive request content. Omit the body for action endpoints that do not need content.",
	}
	switch path {
	case "/v1/auth/login":
		spec.Required = true
		spec.Description = "Local username/email login credentials."
		spec.Example = `{"identifier":"admin","password":"temporary-password"}`
	case "/v1/auth/refresh":
		spec.Required = true
		spec.Description = "Refresh an existing UI/session refresh token."
		spec.Example = `{"refresh_token":"refresh-token"}`
	case "/v1/auth/logout":
		spec.Description = "Optional refresh-token revocation and post-logout return path."
		spec.Example = `{"refresh_token":"refresh-token","return_to":"/"}`
	case "/v1/auth/discover", "/v1/auth/email":
		spec.Required = true
		spec.Description = "Email address used to discover or update an identity provider."
		spec.Example = `{"email":"operator@example.com"}`
	case "/v1/auth/password":
		spec.Required = true
		spec.Description = "Change the current user's password."
		spec.Example = `{"current_password":"old-password","new_password":"new-password"}`
	case "/v1/auth/personal-tokens", "/v1/admin/service-accounts/{serviceAccountID}/tokens":
		spec.Required = true
		spec.Description = "Create a named token with an expiry policy."
		spec.Example = `{"name":"automation","expires_in_days":30}`
	case "/v1/admin/users":
		spec.Required = true
		spec.Description = "Create a local user and assign an initial product role."
		spec.Example = `{"sub":"operator","email":"operator@example.com","password":"temporary-password","role":"admin"}`
	case "/v1/admin/users/{userID}":
		spec.Required = true
		spec.Description = "Update local user fields; omit fields that should not change."
		spec.Example = `{"email":"operator@example.com","status":"active"}`
	case "/v1/admin/service-accounts":
		spec.Required = true
		spec.Description = "Create a service account and optionally return an initial token."
		spec.Example = `{"sub":"ci-bot","email":"ci-bot@example.com","role":"operator","token_name":"gitops","expires_in_days":90}`
	case "/v1/admin/service-accounts/{serviceAccountID}":
		spec.Required = true
		spec.Description = "Update service-account metadata or status."
		spec.Example = `{"email":"ci-bot@example.com","status":"active"}`
	case "/v1/admin/user-roles":
		spec.Required = true
		spec.Description = "Assign a product role to a user."
		spec.Example = `{"user_id":"user-id","role":"operator"}`
	case "/v1/admin/service-account-roles":
		spec.Required = true
		spec.Description = "Assign a product role to a service account."
		spec.Example = `{"service_account_id":"service-account-id","role":"operator"}`
	case "/v1/admin/roles":
		spec.Required = true
		spec.Description = "Create a custom authorization role rule."
		spec.Example = `{"role":"release-operator","name":"run-prod","resource_type":"pipeline","resource_id":"delivery/release","act":"allow:pipeline.run","effect":"allow"}`
	case "/v1/access/grants":
		spec.Required = true
		spec.Description = "Create an access grant for a subject and resource."
		spec.Example = `{"subject_type":"user","subject_id":"user-id","role":"operator","resource_type":"pipeline","resource_id":"delivery/release","inherit":true}`
	case "/v1/authz/resource-use/check":
		spec.Required = true
		spec.Description = "Evaluate one resource-use authorization decision."
		spec.Example = `{"caller_type":"user","caller_id":"user-id","action":"pipeline.run","resource_type":"pipeline","resource_id":"delivery/release"}`
	case "/v1/authz/resource-use/batch-check":
		spec.Required = true
		spec.Description = "Evaluate multiple resource-use checks for one caller."
		spec.Example = `{"caller_type":"user","caller_id":"user-id","checks":[{"action":"pipeline.run","resource_type":"pipeline","resource_id":"delivery/release"}]}`
	case "/v1/run":
		spec.Required = true
		spec.Description = "Run a named pipeline or inline pipeline definition."
		spec.Example = `{"pipeline":"delivery/release","scope":"prod","variables":{"IMAGE_TAG":"v1.2.3"}}`
	case "/v1/run/{pipelineName...}":
		spec.Description = "Optional run scope and variable overrides for the selected pipeline."
		spec.Example = `{"scope":"prod","variables":{"IMAGE_TAG":"v1.2.3"}}`
	case "/v1/mcp":
		spec.Description = "JSON-RPC request for the hosted NopsAI MCP endpoint. Empty body lists tools."
		spec.Example = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	case "/v1/analysis/evaluate":
		spec.Required = true
		spec.Description = "Evaluate a client-provided redacted analysis snapshot with the selected/default LLM profile."
		spec.Example = `{"subject_type":"run","subject_id":"run-id","scope":"prod","selected_llm_profile":"standard","prompt":"Return structured JSON for this redacted reviewer snapshot."}`
	case "/v1/setup/bootstrap":
		spec.Required = true
		spec.Description = "First-install setup payload for users, GitOps repositories, starter teams, LLM, and MCP examples."
		spec.Example = `{"profile":"production","generate_secrets":true,"production_acknowledged":true,"mcp_examples":true,"users":[{"sub":"admin","email":"admin@example.com","role":"admin","password":"temporary-password"}]}`
	case "/v1/system/data/backups":
		spec.Required = true
		spec.Description = "Create a platform data backup."
		spec.Example = `{"backup_type":"full"}`
	case "/v1/system/data/cleanup/preview", "/v1/system/data/cleanup/run":
		spec.Required = true
		spec.Description = "Preview or run a data cleanup plan."
		spec.Example = `{"target":"runs","mode":"older_than_days","older_than_days":90,"backup_before_cleanup":true}`
	case "/v1/system/data/cleanup/schedules", "/v1/system/data/cleanup/schedules/{scheduleID}":
		spec.Required = true
		spec.Description = "Create or replace a recurring cleanup schedule."
		spec.Example = `{"name":"weekly-run-cleanup","enabled":true,"target":"runs","mode":"older_than_days","older_than_days":90,"cron":"0 2 * * 0"}`
	case "/v1/system/credentials", "/v1/system/credentials/{credentialID}/value":
		spec.Required = true
		spec.Description = "Create or rotate an encrypted system credential value."
		spec.Example = `{"name":"llm-openai","purpose":"llm_api_key","value":"secret-value"}`
	case "/v1/system/config-repo/write", "/v1/teams/{teamID}/config-repository/write":
		spec.Required = true
		spec.Description = "Write GitOps files through the configured repository workflow."
		spec.Example = `{"message":"Update NopsAI config","files":[{"path":"pipelines/delivery/release.yaml","content":"name: release\n"}]}`
	case "/v1/system/notifications/mail/test":
		spec.Required = true
		spec.Description = "Send a test notification email."
		spec.Example = `{"recipient":"operator@example.com"}`
	case "/v1/system/dispatcher/runners/{runnerID}/dispatch":
		spec.Required = true
		spec.Description = "Pause or resume dispatcher work assignment for a runner."
		spec.Example = `{"allow_dispatch":false}`
	case "/v1/secrets/encrypt":
		spec.Required = true
		spec.Description = "Encrypt a secret value for GitOps-safe storage."
		spec.Example = `{"value":"secret-value"}`
	}
	if path == "/v1/pipelines/{pipelineName...}" || path == "/v1/steps/{stepName...}" {
		spec.Required = true
		spec.ContentTypes = []string{"application/x-yaml", "application/yaml", "text/yaml", "application/json"}
		spec.Description = "Pipeline or reusable-step definition content."
		spec.Example = "name: delivery/release\nsteps:\n  - name: build\n    script: go test ./...\n"
	}
	return spec
}

func bodyCapableMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "POST", "PUT", "PATCH":
		return true
	default:
		return false
	}
}

func routeExamples(route Route) []Example {
	command := "nopsai api call " + route.Method + " " + shellSingleQuote(route.Path)
	for _, parameter := range route.PathParameters {
		command += " --path " + shellSingleQuote(parameter.Name+"="+parameter.Example)
	}
	if len(route.QueryParameters) > 0 {
		query := route.QueryParameters[0]
		if query.Example != "" {
			command += " --query " + shellSingleQuote(query.Name+"="+query.Example)
		}
	}
	if route.Body != nil && route.Body.Required {
		command += " --data payload.json"
	}
	if route.Public {
		command += " --no-auth"
	}
	if route.Streaming {
		command = "nopsai --timeout 0 api call " + route.Method + " " + shellSingleQuote(route.Path)
		for _, parameter := range route.PathParameters {
			command += " --path " + shellSingleQuote(parameter.Name+"="+parameter.Example)
		}
		command += " --accept text/event-stream"
	}
	return []Example{{Description: "Call this registered route without entering the interactive selector.", Command: command}}
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
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

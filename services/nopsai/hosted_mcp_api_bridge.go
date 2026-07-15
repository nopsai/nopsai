package nopsai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"

	"nopsai/services/nopsai/pkg/auth"
	"nopsai/services/nopsai/pkg/routeauthz"

	aaamodel "nopsai/services/aaa/pkg/model"
)

type hostedMCPAPICall struct {
	Method                   string
	Path                     string
	Body                     []byte
	Headers                  http.Header
	Confirm                  bool
	IncludeSensitiveResponse bool
}

func (a *App) hostedMCPCallAPI(ctx context.Context, subject aaamodel.Subject, args map[string]any) (map[string]any, error) {
	call, err := hostedMCPAPICallFromArgs(args)
	if err != nil {
		return nil, err
	}
	if err := hostedMCPAPIRouteAllowed(call.Method, call.Path); err != nil {
		return nil, err
	}
	if hostedMCPAPISensitiveRead(call.Method, call.Path) && !call.IncludeSensitiveResponse {
		return nil, fmt.Errorf("sensitive response blocked for %s %s; use a metadata/encryption flow or explicitly set include_sensitive_response", call.Method, call.Path)
	}

	req := httptest.NewRequest(call.Method, "http://nopsai.internal"+call.Path, bytes.NewReader(call.Body))
	req = req.WithContext(hostedMCPContextWithSubject(ctx, subject))
	req.Header.Set("Accept", "application/json")
	if len(call.Body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, values := range call.Headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	action, resource, requiresFilter, err := routeauthz.MapRequest(req)
	if err != nil {
		return nil, fmt.Errorf("map route authorization: %w", err)
	}
	if action == "" && !requiresFilter && !hostedMCPDeferredAPIRoute(call.Method, call.Path, resource) {
		return nil, fmt.Errorf("route %s %s is not exposed through hosted MCP", call.Method, call.Path)
	}
	if action != "" && !a.hostedMCPAllowed(ctx, subject, hostedMCPReadPermission(action, resource.Type, resource.ID)) {
		return nil, fmt.Errorf("API route %s %s is not allowed for %s:%s with action %s", call.Method, call.Path, resource.Type, resource.ID, action)
	}

	mutating := hostedMCPAPIMutates(call.Method)
	if mutating && !call.Confirm {
		return map[string]any{
			"method":                call.Method,
			"path":                  call.Path,
			"requires_confirmation": true,
			"applied":               false,
			"permission":            hostedMCPAPIPermissionSummary(action, resource, requiresFilter),
			"note":                  "Set confirm:true to execute this mutating API call as the current authenticated subject.",
		}, nil
	}

	rec := httptest.NewRecorder()
	a.hostedMCPAPIHandler().ServeHTTP(rec, req)
	responseBody := strings.TrimSpace(rec.Body.String())
	payload := map[string]any{
		"method":                call.Method,
		"path":                  call.Path,
		"status_code":           rec.Code,
		"ok":                    rec.Code >= 200 && rec.Code < 300,
		"applied":               mutating && call.Confirm && rec.Code >= 200 && rec.Code < 300,
		"requires_confirmation": false,
		"permission":            hostedMCPAPIPermissionSummary(action, resource, requiresFilter),
	}
	if responseBody != "" {
		contentType := rec.Header().Get("Content-Type")
		if hostedMCPResponseLooksJSON(contentType, responseBody) {
			var parsed any
			if err := json.Unmarshal([]byte(responseBody), &parsed); err == nil {
				payload["response"] = parsed
			} else {
				payload["response_text"] = responseBody
			}
		} else {
			payload["response_text"] = responseBody
		}
	}
	return payload, nil
}

func (a *App) authorizeHostedMCPAPICall(ctx context.Context, subject aaamodel.Subject, args map[string]any) error {
	call, err := hostedMCPAPICallFromArgs(args)
	if err != nil {
		return err
	}
	if err := hostedMCPAPIRouteAllowed(call.Method, call.Path); err != nil {
		return err
	}
	if hostedMCPAPISensitiveRead(call.Method, call.Path) && !call.IncludeSensitiveResponse {
		return fmt.Errorf("sensitive response blocked for %s %s; use a metadata/encryption flow or explicitly set include_sensitive_response", call.Method, call.Path)
	}

	req := httptest.NewRequest(call.Method, "http://nopsai.internal"+call.Path, nil)
	req = req.WithContext(hostedMCPContextWithSubject(ctx, subject))
	action, resource, requiresFilter, err := routeauthz.MapRequest(req)
	if err != nil {
		return fmt.Errorf("map route authorization: %w", err)
	}
	if action == "" && !requiresFilter && !hostedMCPDeferredAPIRoute(call.Method, call.Path, resource) {
		return fmt.Errorf("route %s %s is not exposed through hosted MCP", call.Method, call.Path)
	}
	if action != "" && !a.hostedMCPAllowed(ctx, subject, hostedMCPReadPermission(action, resource.Type, resource.ID)) {
		return fmt.Errorf("API route %s %s is not allowed for %s:%s with action %s", call.Method, call.Path, resource.Type, resource.ID, action)
	}
	return nil
}

func (a *App) hostedMCPAPIHandler() http.Handler {
	mux := http.NewServeMux()
	a.registerAuthRoutes(mux)
	a.registerAccessRoutes(mux)
	a.registerGitHubRoutes(mux)
	a.registerTeamRoutes(mux)
	a.registerSystemRoutes(mux)
	a.registerMonitoringRoutes(mux)
	a.registerPipelineRoutes(mux)
	a.registerScheduleRoutes(mux)
	a.registerExternalTriggerRoutes(mux)
	a.registerGitWebhookSourceRoutes(mux)
	a.registerKnowledgeContextRoutes(mux)
	a.registerSecretVariableRoutes(mux)
	a.registerRunRoutes(mux)
	a.registerSetupRoutes(mux)
	return recoveryMiddleware(mux)
}

func hostedMCPAPICallFromArgs(args map[string]any) (hostedMCPAPICall, error) {
	method := strings.ToUpper(strings.TrimSpace(stringArg(args, "method")))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead:
	default:
		return hostedMCPAPICall{}, fmt.Errorf("unsupported method %q", method)
	}
	path, err := hostedMCPAPIPathFromArgs(args)
	if err != nil {
		return hostedMCPAPICall{}, err
	}
	body, err := hostedMCPAPIBodyFromArgs(args)
	if err != nil {
		return hostedMCPAPICall{}, err
	}
	headers, err := hostedMCPAPIHeadersFromArgs(args)
	if err != nil {
		return hostedMCPAPICall{}, err
	}
	return hostedMCPAPICall{
		Method:                   method,
		Path:                     path,
		Body:                     body,
		Headers:                  headers,
		Confirm:                  boolArg(args, "confirm", false),
		IncludeSensitiveResponse: boolArg(args, "include_sensitive_response", false),
	}, nil
}

func hostedMCPAPIPathFromArgs(args map[string]any) (string, error) {
	raw := strings.TrimSpace(stringArg(args, "path"))
	if raw == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.Contains(raw, "://") || strings.HasPrefix(raw, "//") {
		return "", fmt.Errorf("path must be relative to the NopsAI API")
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	values := parsed.Query()
	if args != nil {
		for key, vals := range hostedMCPAPIQueryValues(args["query"]) {
			for _, value := range vals {
				values.Add(key, value)
			}
		}
	}
	parsed.RawQuery = values.Encode()
	return parsed.RequestURI(), nil
}

func hostedMCPAPIQueryValues(raw any) url.Values {
	values := url.Values{}
	mapped, ok := raw.(map[string]any)
	if !ok {
		return values
	}
	keys := make([]string, 0, len(mapped))
	for key := range mapped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		name := strings.TrimSpace(key)
		if name == "" {
			continue
		}
		switch typed := mapped[key].(type) {
		case []any:
			for _, item := range typed {
				values.Add(name, strings.TrimSpace(fmt.Sprint(item)))
			}
		case []string:
			for _, item := range typed {
				values.Add(name, strings.TrimSpace(item))
			}
		default:
			values.Add(name, strings.TrimSpace(fmt.Sprint(typed)))
		}
	}
	return values
}

func hostedMCPAPIBodyFromArgs(args map[string]any) ([]byte, error) {
	if args == nil {
		return nil, nil
	}
	value, ok := args["body"]
	if !ok || value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil, nil
		}
		return []byte(text), nil
	case json.RawMessage:
		return []byte(typed), nil
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return nil, fmt.Errorf("invalid body: %w", err)
		}
		return raw, nil
	}
}

func hostedMCPAPIHeadersFromArgs(args map[string]any) (http.Header, error) {
	headers := http.Header{}
	if args == nil {
		return headers, nil
	}
	mapped, ok := args["headers"].(map[string]any)
	if !ok {
		return headers, nil
	}
	for key, value := range mapped {
		name := http.CanonicalHeaderKey(strings.TrimSpace(key))
		if name == "" {
			continue
		}
		if hostedMCPAPIHeaderForbidden(name) {
			return nil, fmt.Errorf("header %q is not allowed through hosted MCP", name)
		}
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				headers.Add(name, strings.TrimSpace(fmt.Sprint(item)))
			}
		case []string:
			for _, item := range typed {
				headers.Add(name, strings.TrimSpace(item))
			}
		default:
			headers.Set(name, strings.TrimSpace(fmt.Sprint(typed)))
		}
	}
	return headers, nil
}

func hostedMCPAPIHeaderForbidden(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "authorization", "cookie", "set-cookie", "x-forwarded-for", "x-real-ip":
		return true
	default:
		return false
	}
}

func hostedMCPAPIRouteAllowed(method, rawPath string) error {
	pathOnly := hostedMCPAPIPathOnly(rawPath)
	switch {
	case !strings.HasPrefix(pathOnly, "/v1/"):
		return fmt.Errorf("only /v1 API routes are exposed through hosted MCP")
	case pathOnly == "/v1/mcp" || strings.HasPrefix(pathOnly, "/v1/assistant/"):
		return fmt.Errorf("assistant and MCP control routes are not exposed through hosted MCP API calls")
	case strings.HasPrefix(pathOnly, "/v1/internal/") || strings.HasPrefix(pathOnly, "/internal/"):
		return fmt.Errorf("internal service routes are not exposed through hosted MCP")
	case strings.HasPrefix(pathOnly, "/v1/git/webhooks/") || pathOnly == "/v1/git/events":
		return fmt.Errorf("provider webhook ingress routes are not exposed through hosted MCP")
	case strings.HasPrefix(pathOnly, "/v1/auth/oidc/") ||
		pathOnly == "/v1/auth/providers" ||
		pathOnly == "/v1/auth/discover" ||
		pathOnly == "/v1/auth/session/exchange" ||
		pathOnly == "/v1/auth/login" ||
		pathOnly == "/v1/auth/refresh" ||
		pathOnly == "/v1/auth/logout":
		return fmt.Errorf("authentication bootstrap/session routes are not exposed through hosted MCP")
	case method == http.MethodGet && strings.HasPrefix(pathOnly, "/v1/system/data/backups/") && strings.HasSuffix(pathOnly, "/download"):
		return fmt.Errorf("backup downloads are not exposed through hosted MCP")
	case method == http.MethodGet && strings.HasPrefix(pathOnly, "/v1/system/logs/sources/") && strings.HasSuffix(pathOnly, "/stream"):
		return fmt.Errorf("long-lived system log streams are not exposed through hosted MCP; use the bounded tail tool")
	default:
		return nil
	}
}

func hostedMCPDeferredAPIRoute(method, rawPath string, resource aaamodel.ResourceRef) bool {
	pathOnly := hostedMCPAPIPathOnly(rawPath)
	switch {
	case pathOnly == "/v1/auth/me":
		return method == http.MethodGet
	case pathOnly == "/v1/auth/password" || pathOnly == "/v1/auth/email":
		return method == http.MethodPost
	case pathOnly == "/v1/auth/personal-tokens" || strings.HasPrefix(pathOnly, "/v1/auth/personal-tokens/"):
		return method == http.MethodGet || method == http.MethodPost || method == http.MethodDelete
	case strings.HasPrefix(pathOnly, "/v1/access/") || strings.HasPrefix(pathOnly, "/v1/resources/"):
		return true
	case strings.HasPrefix(pathOnly, "/v1/monitoring/"):
		return true
	case strings.HasPrefix(pathOnly, "/v1/teams/") || pathOnly == "/v1/teams":
		return true
	case strings.HasPrefix(pathOnly, "/v1/schedules/") || pathOnly == "/v1/schedules":
		return true
	case strings.HasPrefix(pathOnly, "/v1/external-triggers/") || pathOnly == "/v1/external-triggers":
		return true
	case strings.HasPrefix(pathOnly, "/v1/pipelines/") || pathOnly == "/v1/run":
		return true
	case strings.HasPrefix(pathOnly, "/v1/runs/") || strings.HasPrefix(pathOnly, "/v1/runs-by-check/"):
		return true
	case strings.HasPrefix(pathOnly, "/v1/steps/"):
		return true
	case pathOnly == "/v1/knowledge-contexts" || strings.HasPrefix(pathOnly, "/v1/knowledge-contexts/"):
		return true
	case pathOnly == "/v1/knowledge-context-connections" || strings.HasPrefix(pathOnly, "/v1/knowledge-context-connections/"):
		return true
	case pathOnly == "/v1/knowledge-connections" || strings.HasPrefix(pathOnly, "/v1/knowledge-connections/"):
		return true
	case pathOnly == "/v1/secrets/encrypt":
		return method == http.MethodPost
	default:
		return strings.TrimSpace(resource.Type) != "" || strings.TrimSpace(resource.ID) != ""
	}
}

func hostedMCPAPISensitiveRead(method, rawPath string) bool {
	if method != http.MethodGet {
		return false
	}
	pathOnly := hostedMCPAPIPathOnly(rawPath)
	switch {
	case pathOnly == "/v1/secrets" || pathOnly == "/v1/secrets/scopes":
		return false
	case strings.HasPrefix(pathOnly, "/v1/secrets/") || strings.Contains(pathOnly, "/secrets/"):
		return true
	case strings.HasSuffix(pathOnly, "/runner-bootstrap-command") ||
		strings.HasSuffix(pathOnly, "/kubernetes-runner-bootstrap-command") ||
		strings.HasSuffix(pathOnly, "/runner-bootstrap"):
		return true
	default:
		return false
	}
}

func hostedMCPAPIMutates(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func hostedMCPAPIPathOnly(rawPath string) string {
	parsed, err := url.ParseRequestURI(rawPath)
	if err != nil {
		return rawPath
	}
	return strings.TrimSpace(parsed.Path)
}

func hostedMCPResponseLooksJSON(contentType, body string) bool {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	return strings.Contains(mediaType, "json") || strings.HasPrefix(strings.TrimSpace(body), "{") || strings.HasPrefix(strings.TrimSpace(body), "[")
}

func hostedMCPAPIPermissionSummary(action string, resource aaamodel.ResourceRef, requiresFilter bool) map[string]any {
	return map[string]any{
		"action":          strings.TrimSpace(action),
		"resource_type":   strings.TrimSpace(resource.Type),
		"resource_id":     strings.TrimSpace(resource.ID),
		"requires_filter": requiresFilter,
	}
}

func hostedMCPContextWithSubject(ctx context.Context, subject aaamodel.Subject) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := auth.ClaimsFromContext(ctx); !ok {
		ctx = auth.WithClaims(ctx, hostedMCPClaimsForSubject(subject))
	}
	return withAAASubject(ctx, subject)
}

func hostedMCPClaimsForSubject(subject aaamodel.Subject) *auth.Claims {
	provider := "hosted-mcp"
	if subject.Type == aaamodel.SubjectTypeServiceAccount {
		provider = auth.ProviderServiceAccount
	}
	return &auth.Claims{
		Sub:      firstNonEmptyString(subject.Sub, subject.ID, subject.Email),
		Email:    strings.TrimSpace(subject.Email),
		Provider: provider,
	}
}

func hostedMCPAuditInput(toolName string, args map[string]any) map[string]any {
	if toolName != "nopsai.call_api" {
		if hostedMCPSensitiveAuditTool(toolName) {
			return hostedMCPRedactSensitiveMap(args)
		}
		return args
	}
	return map[string]any{
		"method":                     stringArg(args, "method"),
		"path":                       stringArg(args, "path"),
		"query":                      args["query"],
		"confirm":                    boolArg(args, "confirm", false),
		"include_sensitive_response": boolArg(args, "include_sensitive_response", false),
		"body":                       "[redacted]",
		"headers":                    "[redacted]",
	}
}

func hostedMCPAuditOutput(toolName string, output map[string]any) map[string]any {
	if len(output) == 0 {
		return output
	}
	if toolName != "nopsai.call_api" {
		if hostedMCPSensitiveAuditTool(toolName) {
			return hostedMCPRedactSensitiveMap(output)
		}
		return output
	}
	summary := map[string]any{}
	for _, key := range []string{"method", "path", "status_code", "ok", "applied", "requires_confirmation", "permission", "note", "error"} {
		if value, ok := output[key]; ok {
			summary[key] = value
		}
	}
	if _, ok := output["response"]; ok {
		summary["response"] = "[redacted]"
	}
	if _, ok := output["response_text"]; ok {
		summary["response_text"] = "[redacted]"
	}
	return summary
}

func hostedMCPSensitiveAuditTool(toolName string) bool {
	name := strings.ToLower(strings.TrimSpace(toolName))
	switch {
	case strings.Contains(name, "secret"):
		return true
	case strings.Contains(name, "credential"):
		return true
	case strings.Contains(name, "bootstrap_first_install"):
		return true
	case strings.Contains(name, "bootstrap_command"):
		return true
	case strings.Contains(name, "admin_user"):
		return true
	case strings.Contains(name, "service_account"):
		return true
	case strings.Contains(name, "variable") && (strings.Contains(name, "write") || strings.Contains(name, "gitops")):
		return true
	default:
		return false
	}
}

func hostedMCPRedactSensitiveMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = hostedMCPRedactSensitiveValue(key, value)
	}
	return out
}

func hostedMCPRedactSensitiveValue(key string, value any) any {
	if hostedMCPRedactSensitiveKey(key) {
		return "[redacted]"
	}
	switch typed := value.(type) {
	case map[string]any:
		return hostedMCPRedactSensitiveMap(typed)
	case []map[string]any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, hostedMCPRedactSensitiveMap(item))
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, hostedMCPRedactSensitiveValue("", item))
		}
		return out
	default:
		return value
	}
}

func hostedMCPRedactSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "" {
		return false
	}
	for _, marker := range []string{
		"value",
		"password",
		"secret",
		"token",
		"private_key",
		"api_key",
		"credential",
		"encrypted_value",
		"ciphertext",
		"wrapped_data_key",
		"response",
		"response_text",
		"content",
		"files",
		"body",
		"headers",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

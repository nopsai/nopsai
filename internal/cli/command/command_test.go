package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nopsai/internal/cli/apicatalog"
	clconfig "nopsai/internal/cli/config"
	"nopsai/internal/cli/interactive"
	cliplatform "nopsai/internal/cli/platform"
	"nopsai/pkg/buildinfo"
	"nopsai/pkg/compatibility"
)

func TestContextCommands(t *testing.T) {
	dir := t.TempDir()
	dependencies := testDependencies(nil, nil)
	output, err := executeCommand(dependencies, "--config-dir", dir, "context", "add", "prod", "--api", "https://api.example.com/")
	if err != nil || !strings.Contains(output, `Context "prod" configured`) {
		t.Fatalf("context add = %q, %v", output, err)
	}
	_, err = executeCommand(dependencies, "--config-dir", dir, "context", "add", "dev", "--api", "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	output, err = executeCommand(dependencies, "--config-dir", dir, "context", "use", "prod")
	if err != nil || !strings.Contains(output, "Current context") {
		t.Fatalf("context use = %q, %v", output, err)
	}
	output, err = executeCommand(dependencies, "--config-dir", dir, "context", "list")
	if err != nil || !strings.Contains(output, "  dev") || !strings.Contains(output, "* prod") {
		t.Fatalf("context list = %q, %v", output, err)
	}
	output, err = executeCommand(dependencies, "--config-dir", dir, "context", "current")
	if err != nil || output != "prod\thttps://api.example.com\n" {
		t.Fatalf("context current = %q, %v", output, err)
	}
	output, err = executeCommand(dependencies, "--config-dir", dir, "context", "delete", "dev")
	if err != nil || !strings.Contains(output, "deleted") {
		t.Fatalf("context delete = %q, %v", output, err)
	}
	if _, err := executeCommand(dependencies, "--config-dir", dir, "context", "use", "missing"); err == nil {
		t.Fatal("missing context unexpectedly selected")
	}
}

func TestAPIRequestUsesContextTokenDataAndHeaders(t *testing.T) {
	var gotAuthorization, gotHeader, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		gotHeader = r.Header.Get("X-Trace")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		if r.Method != http.MethodPost || r.URL.RequestURI() != "/v1/system/config/sync?dry_run=true" {
			t.Errorf("request = %s %s", r.Method, r.URL.RequestURI())
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	dir := t.TempDir()
	store, _ := clconfig.NewStore(dir)
	_, _ = store.AddContext("prod", server.URL)
	_ = store.SaveToken("prod", "stored-token")
	dataPath := filepath.Join(t.TempDir(), "sync.json")
	if err := os.WriteFile(dataPath, []byte(`{"sync":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dependencies := testDependencies(server.Client(), map[string]string{"NOPSAI_TOKEN": "environment-token"})
	output, err := executeCommand(dependencies,
		"--config-dir", dir, "api", "request", "post", "/v1/system/config/sync?dry_run=true",
		"--data", dataPath, "-H", "X-Trace: request-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	if output != "{\"status\":\"ok\"}" || gotAuthorization != "Bearer environment-token" || gotHeader != "request-1" || gotBody != `{"sync":true}` {
		t.Fatalf("output/auth/header/body = %q / %q / %q / %q", output, gotAuthorization, gotHeader, gotBody)
	}
}

func TestAPIRequestSupportsAPIOverrideAndReportsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer direct-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden\n"))
	}))
	defer server.Close()
	dependencies := testDependencies(server.Client(), map[string]string{"NOPSAI_TOKEN": "direct-token"})
	outputPath := filepath.Join(t.TempDir(), "response.bin")
	if err := os.WriteFile(outputPath, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := executeCommand(dependencies, "--api", server.URL, "api", "request", "GET", "/v1/runs", "--output-file", outputPath)
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") || output != "forbidden\n" {
		t.Fatalf("request failure = %q, %v", output, err)
	}
	contents, readErr := os.ReadFile(outputPath)
	if readErr != nil || string(contents) != "original" {
		t.Fatalf("error response replaced output file: %q, %v", contents, readErr)
	}
}

func TestAPIOverrideDoesNotForwardStoredTokenAcrossOrigins(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	dir := t.TempDir()
	store, _ := clconfig.NewStore(dir)
	_, _ = store.AddContext("prod", "https://api.example.invalid")
	_ = store.SaveToken("prod", "stored-token")

	output, err := executeCommand(testDependencies(server.Client(), nil),
		"--config-dir", dir, "--context", "prod", "--api", server.URL,
		"api", "request", "GET", "/v1/runs",
	)
	if err != nil || output != "ok" {
		t.Fatalf("cross-origin override = %q, %v", output, err)
	}
	if authorization != "" {
		t.Fatalf("cross-origin override forwarded stored credential: %q", authorization)
	}
}

func TestAPIOverrideKeepsStoredTokenOnSameOrigin(t *testing.T) {
	var authorization, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		path = r.URL.Path
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	dir := t.TempDir()
	store, _ := clconfig.NewStore(dir)
	_, _ = store.AddContext("prod", server.URL+"/primary")
	_ = store.SaveToken("prod", "stored-token")

	output, err := executeCommand(testDependencies(server.Client(), nil),
		"--config-dir", dir, "--context", "prod", "--api", server.URL+"/alternate",
		"api", "request", "GET", "/v1/runs",
	)
	if err != nil || output != "ok" {
		t.Fatalf("same-origin override = %q, %v", output, err)
	}
	if authorization != "Bearer stored-token" || path != "/alternate/v1/runs" {
		t.Fatalf("same-origin auth/path = %q / %q", authorization, path)
	}
	if sameAPIOrigin("://invalid", server.URL) {
		t.Fatal("invalid API URL matched a valid origin")
	}
}

func TestAPIRequestReadsStdinAndValidatesHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_, _ = w.Write(body)
	}))
	defer server.Close()
	dependencies := testDependencies(server.Client(), nil)
	dependencies.In = strings.NewReader(`{"input":true}`)
	output, err := executeCommand(dependencies, "--api", server.URL, "api", "request", "POST", "/echo", "--data", "-")
	if err != nil || output != "{\"input\":true}" {
		t.Fatalf("stdin request = %q, %v", output, err)
	}
	if _, err := executeCommand(testDependencies(server.Client(), nil), "--api", server.URL, "api", "request", "GET", "/", "-H", "broken"); err == nil {
		t.Fatal("invalid header succeeded")
	}
}

func TestAPICallExpandsRegisteredRouteAndPreservesQueryValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/pipelines/delivery/release" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query()["label"]; len(got) != 2 || got[0] != "one" || got[1] != "two" {
			t.Errorf("labels = %#v", got)
		}
		_, _ = w.Write([]byte("pipeline: release"))
	}))
	defer server.Close()
	output, err := executeCommand(testDependencies(server.Client(), nil),
		"--api", server.URL,
		"api", "call", "GET", "/v1/pipelines/{pipelineName...}",
		"--path", "pipelineName=delivery/release",
		"--query", "label=one", "--query", "label=two",
	)
	if err != nil || output != "pipeline: release" {
		t.Fatalf("api call = %q, %v", output, err)
	}
	if _, err := executeCommand(testDependencies(server.Client(), nil), "--api", server.URL, "api", "call", "GET", "/not-registered"); err == nil {
		t.Fatal("unregistered route succeeded")
	}
}

func TestAPICallGuidesMissingRequiredInputs(t *testing.T) {
	_, err := executeCommand(testDependencies(nil, nil), "api", "call", "GET", "/v1/pipelines/{pipelineName...}")
	if err == nil || !strings.Contains(err.Error(), "requires path parameter(s): pipelineName") || !strings.Contains(err.Error(), "--path pipelineName=delivery/release") {
		t.Fatalf("missing path parameter guidance = %v", err)
	}
	_, err = executeCommand(testDependencies(nil, nil), "api", "call", "GET", "/v1/access/effective-permissions")
	if err == nil || !strings.Contains(err.Error(), "requires query parameter(s): action, resource_type, resource_id") || !strings.Contains(err.Error(), "--query action=pipeline.run") {
		t.Fatalf("missing query parameter guidance = %v", err)
	}
	_, err = executeCommand(testDependencies(nil, nil), "api", "call", "POST", "/v1/auth/login", "--no-auth")
	if err == nil || !strings.Contains(err.Error(), "expects request content") || !strings.Contains(err.Error(), "api describe POST /v1/auth/login") {
		t.Fatalf("missing body guidance = %v", err)
	}
}

func TestAPIDescribeTextShowsSamples(t *testing.T) {
	output, err := executeCommand(testDependencies(nil, nil), "api", "describe", "POST", "/v1/auth/login", "--output", "text")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Body: required",
		`{"identifier":"admin","password":"temporary-password"}`,
		"Examples:",
		"--data payload.json",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("describe login missing %q in:\n%s", want, output)
		}
	}
	output, err = executeCommand(testDependencies(nil, nil), "api", "describe", "GET", "/v1/access/effective-permissions", "--output", "text")
	if err != nil || !strings.Contains(output, "Query parameters:\n  - action") || !strings.Contains(output, "(required, example pipeline.run") || !strings.Contains(output, "action=pipeline.run") {
		t.Fatalf("describe query guidance = %q, %v", output, err)
	}
}

func TestAPICallInteractiveSelectsPublicRoute(t *testing.T) {
	var authorization, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"providers":[]}`))
	}))
	defer server.Close()
	dependencies := testDependencies(server.Client(), map[string]string{"NOPSAI_TOKEN": "secret"})
	dependencies.In = strings.NewReader("auth providers\n1\n\n\n\n")

	output, err := executeCommand(dependencies, "--api", server.URL, "api", "call", "--interactive")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/auth/providers" || authorization != "" || !strings.Contains(output, `{"providers":[]}`) {
		t.Fatalf("interactive call path/auth/output = %q / %q / %q", path, authorization, output)
	}
}

func TestAPIInteractiveFieldsOnlyShowRelevantSteps(t *testing.T) {
	route, ok := apicatalog.Find(http.MethodGet, "/v1/access/teams")
	if !ok {
		t.Fatal("route not found")
	}
	fields := apiRequestFields(route, apiRequestOptions{})
	labels := make([]string, 0, len(fields))
	for _, field := range fields {
		labels = append(labels, field.Label)
	}
	if strings.Join(labels, ",") != "Attach bearer token,Send request" {
		t.Fatalf("GET /v1/access/teams fields = %#v", labels)
	}

	route, ok = apicatalog.Find(http.MethodGet, "/v1/pipelines/{pipelineName...}")
	if !ok {
		t.Fatal("pipeline route not found")
	}
	fields = apiRequestFields(route, apiRequestOptions{})
	labels = labels[:0]
	for _, field := range fields {
		labels = append(labels, field.Label)
	}
	if !containsString(labels, "Path parameter: pipelineName") || !containsString(labels, "Attach bearer token") || containsString(labels, "Response format (HTTP Accept)") {
		t.Fatalf("pipeline fields = %#v", labels)
	}

	route, ok = apicatalog.Find(http.MethodPost, "/v1/admin/service-account-roles")
	if !ok {
		t.Fatal("service-account role route not found")
	}
	fields = apiRequestFields(route, apiRequestOptions{})
	if field, ok := fieldByName(fields, "body.raw"); !ok || !field.Multiline || field.Label != "Payload editor" {
		t.Fatalf("payload paste field = %#v, found=%v", field, ok)
	}
}

func TestAPIRequestParameterMapFollowsWizardOrder(t *testing.T) {
	route, ok := apicatalog.Find(http.MethodGet, "/internal/v1/runtime-config/{service}/watch")
	if !ok {
		t.Fatal("runtime-config watch route not found")
	}
	fields := apiRequestFields(route, apiRequestOptions{})
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.Name)
	}
	want := []string{
		"path.service",
		"query.version",
		"query.since_version",
		"query.extra",
		"accept",
		"auth",
		"send",
	}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("request fields = %#v; want %#v", names, want)
	}
}

func TestAPIResponseScreenPrettyPrintsJSON(t *testing.T) {
	route, ok := apicatalog.Find(http.MethodGet, "/v1/access/teams")
	if !ok {
		t.Fatal("route not found")
	}
	lines := apiResponseScreenLines(route, `{"teams":[{"id":"team-1","name":"Team One"}]}`, "", nil)
	text := strings.Join(lines, "\n")
	if !strings.Contains(text, "Request\nMethod: GET\nRoute: /v1/access/teams") || !strings.Contains(text, `"teams": [`) || !strings.Contains(text, `  "id": "team-1"`) || strings.Contains(text, `{"teams":[`) {
		t.Fatalf("pretty response lines = %#v", lines)
	}
}

func TestSplitPromptListAcceptsMultilineAssignments(t *testing.T) {
	values := splitPromptList("role=admin\nresource_type=pipeline, values=read")
	if strings.Join(values, "|") != "role=admin|resource_type=pipeline|values=read" {
		t.Fatalf("split prompt values = %#v", values)
	}
}

func TestAPIRequestSupportsRawContentPublicCallsHeadersAndDownloads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/upload":
			if got := r.Header.Get("Authorization"); got != "" {
				t.Errorf("Authorization = %q", got)
			}
			if got := r.Header.Get("Content-Type"); got != "application/x-yaml" {
				t.Errorf("Content-Type = %q", got)
			}
			if got := r.Header.Get("Accept"); got != "application/x-yaml" {
				t.Errorf("Accept = %q", got)
			}
			body, _ := io.ReadAll(r.Body)
			_, _ = w.Write(body)
		case "/download":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("X-Artifact", "result")
			_, _ = w.Write([]byte{0, 1, 2, 3})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	dependencies := testDependencies(server.Client(), map[string]string{"NOPSAI_TOKEN": "secret"})
	output, err := executeCommand(dependencies,
		"--api", server.URL, "--timeout", "0",
		"api", "request", "PUT", "/upload", "--data-raw", "name: release", "--content-type", "application/x-yaml", "--accept", "application/x-yaml", "--no-auth",
	)
	if err != nil || output != "name: release" {
		t.Fatalf("raw request = %q, %v", output, err)
	}
	outputPath := filepath.Join(t.TempDir(), "artifact.bin")
	output, err = executeCommand(dependencies, "--api", server.URL, "api", "request", "GET", "/download", "--output-file", outputPath, "--show-headers")
	if err != nil || output != "" {
		t.Fatalf("download = %q, %v", output, err)
	}
	contents, err := os.ReadFile(outputPath)
	if err != nil || !bytes.Equal(contents, []byte{0, 1, 2, 3}) {
		t.Fatalf("downloaded contents = %v, %v", contents, err)
	}
}

func TestAPIRouteCatalogCommandsExposeAllRegisteredAPIs(t *testing.T) {
	output, err := executeCommand(testDependencies(nil, nil), "api", "routes", "--domain", "pipelines", "--output", "json")
	if err != nil {
		t.Fatal(err)
	}
	var routes []apicatalog.Route
	if err := json.Unmarshal([]byte(output), &routes); err != nil {
		t.Fatal(err)
	}
	if len(routes) != 4 {
		t.Fatalf("pipeline routes = %d", len(routes))
	}
	output, err = executeCommand(testDependencies(nil, nil), "api", "describe", "GET", "/v1/system/logs/sources/{sourceID}/stream")
	if err != nil || !strings.Contains(output, "streaming: true") || !strings.Contains(output, "sourceID") {
		t.Fatalf("describe = %q, %v", output, err)
	}
	output, err = executeCommand(testDependencies(nil, nil), "api", "routes", "--audience", "internal")
	if err != nil || !strings.Contains(output, "/v1/internal/config/sync") || strings.Contains(output, "/v1/auth/login") {
		t.Fatalf("internal routes = %q, %v", output, err)
	}
	if _, err := executeCommand(testDependencies(nil, nil), "api", "routes", "--audience", "unknown"); err == nil {
		t.Fatal("unknown audience succeeded")
	}
}

func TestReleasedCLIValidatesVersionBeforeMutation(t *testing.T) {
	versionChecks := 0
	mutations := 0
	platformVersion := "2.6.0"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version" {
			versionChecks++
			if r.Header.Get("Authorization") != "" {
				t.Errorf("version request carried authorization")
			}
			_ = json.NewEncoder(w).Encode(compatibility.PlatformInfo{
				ProductVersion:   platformVersion,
				APIVersion:       "v1",
				CLICompatibility: ">=2.0.0 <3.0.0",
			})
			return
		}
		mutations++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	dependencies := testDependencies(server.Client(), nil)
	dependencies.BuildInfo = commandBuildInfo("2.7.0")
	if _, err := executeCommand(dependencies, "--api", server.URL, "api", "request", "POST", "/v1/system/config/sync"); err != nil {
		t.Fatal(err)
	}
	if versionChecks != 1 || mutations != 1 {
		t.Fatalf("checks/mutations = %d/%d", versionChecks, mutations)
	}
	platformVersion = "3.0.0"
	if _, err := executeCommand(dependencies, "--api", server.URL, "api", "request", "DELETE", "/v1/runs/id"); err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("compatibility error = %v", err)
	}
	if mutations != 1 {
		t.Fatal("incompatible mutation reached server")
	}
}

func TestPlatformReleasePlansWithJSONOutput(t *testing.T) {
	chart := []byte("chart archive")
	manifestPath := writeCommandReleaseManifest(t, chart)
	valuesPath := filepath.Join(t.TempDir(), "values.yaml")
	if err := os.WriteFile(valuesPath, []byte("ingress:\n  enabled: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dependencies := testDependencies(nil, nil)
	dependencies.BuildInfo = commandBuildInfo("2.7.0")
	dependencies.RunProcess = func(_ context.Context, name string, args []string, stdout, _ io.Writer) error {
		if name != "helm" {
			return errors.New("unexpected process")
		}
		switch args[0] {
		case "pull":
			destination := commandArgumentValue(args, "--destination")
			return os.WriteFile(filepath.Join(destination, "nopsai-2.7.0.tgz"), chart, 0o600)
		case "template":
			_, err := io.WriteString(stdout, "kind: Deployment\n")
			return err
		case "upgrade":
			return nil
		default:
			return errors.New("unexpected helm command")
		}
	}
	output, err := executeCommand(dependencies,
		"platform", "release", "kubernetes", "--version", "2.7.0", "--manifest", manifestPath, "--values", valuesPath, "--output", "json",
	)
	if err != nil {
		t.Fatal(err)
	}
	var plan cliplatform.DeploymentPlan
	if err := json.Unmarshal([]byte(output), &plan); err != nil || plan.Version != "2.7.0" || !strings.Contains(plan.RenderedManifestYAML, "Deployment") {
		t.Fatalf("plan = %#v, %v", plan, err)
	}
	if _, err := executeCommand(dependencies, "platform", "plan"); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("legacy platform plan command still available: %v", err)
	}
}

func TestPlatformReleaseCommandDeploysWithSingleFlag(t *testing.T) {
	chart := []byte("chart archive")
	manifestPath := writeCommandReleaseManifest(t, chart)
	lockPath := filepath.Join(t.TempDir(), ".nopsai", "release.lock")
	var sawUpgrade bool
	dependencies := testDependencies(nil, nil)
	dependencies.BuildInfo = commandBuildInfo("2.7.0")
	dependencies.RunProcess = func(_ context.Context, name string, args []string, stdout, _ io.Writer) error {
		if name != "helm" {
			return errors.New("unexpected process")
		}
		switch args[0] {
		case "pull":
			destination := commandArgumentValue(args, "--destination")
			return os.WriteFile(filepath.Join(destination, "nopsai-2.7.0.tgz"), chart, 0o600)
		case "template":
			_, err := io.WriteString(stdout, "kind: Deployment\n")
			return err
		case "upgrade":
			sawUpgrade = true
			if !containsCommandArgument(args, "--wait") {
				t.Fatal("release --deploy did not pass --wait to helm upgrade")
			}
			return nil
		default:
			return errors.New("unexpected helm command")
		}
	}
	output, err := executeCommand(dependencies,
		"platform", "release", "kubernetes", "--version", "2.7.0", "--manifest", manifestPath, "--deploy", "--wait", "--lock-file", lockPath,
	)
	if err != nil || !strings.Contains(output, "Deployed NopsAI 2.7.0") || !sawUpgrade {
		t.Fatalf("platform release deploy = %q, %v, sawUpgrade=%v", output, err, sawUpgrade)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("release lock: %v", err)
	}
}

func TestPlatformReleaseInteractiveCanPlanWithoutDeploying(t *testing.T) {
	chart := []byte("chart archive")
	manifestPath := writeCommandReleaseManifest(t, chart)
	lockPath := filepath.Join(t.TempDir(), ".nopsai", "release.lock")
	var sawUpgrade bool
	dependencies := testDependencies(nil, nil)
	dependencies.BuildInfo = commandBuildInfo("2.7.0")
	dependencies.In = strings.NewReader("kub\n\n\n" + manifestPath + "\n\n\n\n\n\n" + lockPath + "\nn\n")
	dependencies.RunProcess = func(_ context.Context, name string, args []string, stdout, _ io.Writer) error {
		if name != "helm" {
			return errors.New("unexpected process")
		}
		switch args[0] {
		case "pull":
			destination := commandArgumentValue(args, "--destination")
			return os.WriteFile(filepath.Join(destination, "nopsai-2.7.0.tgz"), chart, 0o600)
		case "template":
			_, err := io.WriteString(stdout, "kind: Deployment\n")
			return err
		case "upgrade":
			sawUpgrade = true
			return nil
		default:
			return errors.New("unexpected helm command")
		}
	}
	output, err := executeCommand(dependencies, "platform", "release", "--interactive")
	if err != nil || !strings.Contains(output, "Plan NopsAI 2.7.0") {
		t.Fatalf("interactive platform release = %q, %v", output, err)
	}
	if sawUpgrade {
		t.Fatal("interactive plan deployed after the operator declined confirmation")
	}
	if _, err := os.Stat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("declined deployment wrote lock: %v", err)
	}
}

func TestInstallDockerComposeGeneratesFromVersionWithoutReleaseManifest(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "install")
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("unexpected manifest HTTP request to %s", request.URL.String())
	})}
	dependencies := testDependencies(client, nil)
	dependencies.BuildInfo = commandBuildInfo("2.7.0")
	dependencies.Random = bytes.NewReader(bytes.Repeat([]byte{9}, 256))

	output, err := executeCommand(dependencies, "install", "docker-compose", "--output-dir", outputDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "Generated NopsAI 2.7.0 docker-compose install") {
		t.Fatalf("install output = %q", output)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "release-manifest.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("installer should not write release-manifest.json: %v", err)
	}
	lockContents, err := os.ReadFile(filepath.Join(outputDir, ".nopsai", "install.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(lockContents), "manifestSource") || !strings.Contains(string(lockContents), "ghcr.io/hosein-yousefii/nopsai-api:2.7.0") {
		t.Fatalf("install lock did not record versioned generated images: %s", lockContents)
	}
}

func TestInstallDockerComposeRequiresVersionWhenDefaultIsMissing(t *testing.T) {
	dependencies := testDependencies(nil, nil)
	dependencies.BuildInfo = commandBuildInfo("dev")
	_, err := executeCommand(dependencies, "install", "docker-compose", "--output-dir", filepath.Join(t.TempDir(), "install"))
	if err == nil || !strings.Contains(err.Error(), "--version is required") {
		t.Fatalf("missing version error = %v", err)
	}
}

func TestRootInteractiveHomeShowsContextSessionAndHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_, _ = w.Write([]byte("ok"))
		case "/version":
			_ = json.NewEncoder(w).Encode(map[string]string{"product_version": "2.7.0", "api_version": "v1"})
		case "/v1/setup/preflight":
			_, _ = w.Write([]byte(`{"ready":true,"mode":"ready"}`))
		case "/v1/auth/me":
			if r.Header.Get("Authorization") != "Bearer stored-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"email":"operator@example.com"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	store, _ := clconfig.NewStore(dir)
	_, _ = store.AddContext("prod", server.URL)
	_ = store.SaveToken("prod", "stored-token")
	dependencies := testDependencies(server.Client(), nil)
	dependencies.In = strings.NewReader("exit\n1\n")
	output, err := executeCommand(dependencies, "--config-dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"NopsAI CLI",
		"Context: prod",
		"Token: configured for context",
		"Session: operator@example.com",
		"nopsai/healthz",
		"2.7.0 (API v1)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("interactive home missing %q in:\n%s", want, output)
		}
	}
}

func TestRootInteractiveHomeExposesCurrentCLISurface(t *testing.T) {
	labels := make([]string, 0, len(homeChoices()))
	for _, choice := range homeChoices() {
		labels = append(labels, choice.Label)
	}
	want := []string{"api", "contexts", "authentication", "install", "platform", "completion", "guide", "help", "exit"}
	if strings.Join(labels, ",") != strings.Join(want, ",") {
		t.Fatalf("home choices = %#v", labels)
	}
}

func TestRootInteractiveRawAPIRequestUsesTransportOptions(t *testing.T) {
	var gotAuthorization, gotTrace, gotContentType, gotAccept, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/echo" {
			switch r.URL.Path {
			case "/healthz":
				_, _ = w.Write([]byte("ok"))
			case "/version":
				_ = json.NewEncoder(w).Encode(map[string]string{"product_version": "2.7.0", "api_version": "v1"})
			case "/v1/setup/preflight":
				_, _ = w.Write([]byte(`{"ready":true,"mode":"ready"}`))
			case "/v1/auth/me":
				_, _ = w.Write([]byte(`{"email":"operator@example.com"}`))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
			return
		}
		gotAuthorization = r.Header.Get("Authorization")
		gotTrace = r.Header.Get("X-Trace")
		gotContentType = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		if r.Method != http.MethodGet {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("X-Result", "ok")
		_, _ = w.Write([]byte("pong"))
	}))
	defer server.Close()

	dependencies := testDependencies(server.Client(), map[string]string{"NOPSAI_TOKEN": "should-not-send"})
	dependencies.In = strings.NewReader(strings.Join([]string{
		"payload", "1", // home -> api
		"header", "1", // api -> raw request
		"", // method default GET
		"/echo",
		"", // body file
		"hello",
		"X-Trace: request-1",
		"text/plain",
		"text/plain",
		"",  // output file
		"n", // do not attach bearer token
		"y", // show headers
		"y", // send
		"back", "1",
		"exit", "1",
	}, "\n") + "\n")
	output, err := executeCommand(dependencies, "--api", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuthorization != "" || gotTrace != "request-1" || gotContentType != "text/plain" || gotAccept != "text/plain" || gotBody != "hello" {
		t.Fatalf("transport = auth %q trace %q content-type %q accept %q body %q", gotAuthorization, gotTrace, gotContentType, gotAccept, gotBody)
	}
	for _, want := range []string{"Status: ok", "HTTP/1.1 200 OK", "X-Result: ok", "pong"} {
		if !strings.Contains(output, want) {
			t.Fatalf("interactive raw request missing %q in:\n%s", want, output)
		}
	}
}

func TestGuideCommandListsAndRendersTopics(t *testing.T) {
	output, err := executeCommand(testDependencies(nil, nil), "guide", "--list")
	if err != nil || !strings.Contains(output, "config") || !strings.Contains(output, "mcp") {
		t.Fatalf("guide list = %q, %v", output, err)
	}
	output, err = executeCommand(testDependencies(nil, nil), "guide", "api")
	if err != nil || !strings.Contains(output, "nopsai api describe POST /v1/run --output text") {
		t.Fatalf("guide api = %q, %v", output, err)
	}
	if _, err := executeCommand(testDependencies(nil, nil), "guide", "missing"); err == nil {
		t.Fatal("unknown guide topic succeeded")
	}
}

func TestCompletionWritesShellFileAndSupportsStdout(t *testing.T) {
	outputDir := t.TempDir()
	output, err := executeCommand(testDependencies(nil, nil), "completion", "bash", "--output-dir", outputDir)
	if err != nil || !strings.Contains(output, "Wrote bash completion") || !strings.Contains(output, "cp ") {
		t.Fatalf("completion file output = %q, %v", output, err)
	}
	contents, err := os.ReadFile(filepath.Join(outputDir, "nopsai.bash"))
	if err != nil || !strings.Contains(string(contents), "bash completion") {
		t.Fatalf("completion file = %q, %v", contents, err)
	}
	output, err = executeCommand(testDependencies(nil, nil), "completion", "fish", "--stdout")
	if err != nil || !strings.Contains(output, "fish completion") {
		t.Fatalf("completion stdout = %q, %v", output, err)
	}
}

func TestLoginVerifiesAndStoresTokenThenLogoutRemovesIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/me" || r.Header.Get("Authorization") != "Bearer nopat_valid" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"sub":"operator"}`))
	}))
	defer server.Close()
	dir := t.TempDir()
	store, _ := clconfig.NewStore(dir)
	_, _ = store.AddContext("prod", server.URL)
	dependencies := testDependencies(server.Client(), map[string]string{"NOPSAI_TOKEN": "nopat_valid"})
	output, err := executeCommand(dependencies, "--config-dir", dir, "login", "--token")
	if err != nil || !strings.Contains(output, "Authenticated") {
		t.Fatalf("login = %q, %v", output, err)
	}
	if token, _ := store.Token("prod"); token != "nopat_valid" {
		t.Fatalf("stored token = %q", token)
	}
	output, err = executeCommand(testDependencies(server.Client(), nil), "--config-dir", dir, "logout")
	if err != nil || !strings.Contains(output, "Removed") {
		t.Fatalf("logout = %q, %v", output, err)
	}
	if token, _ := store.Token("prod"); token != "" {
		t.Fatalf("token remains after logout: %q", token)
	}
}

func TestLoginReadsNonTerminalStdinAndRejectsBadToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer stdin-token" {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	store, _ := clconfig.NewStore(dir)
	_, _ = store.AddContext("prod", server.URL)
	dependencies := testDependencies(server.Client(), nil)
	dependencies.In = strings.NewReader("stdin-token\n")
	if _, err := executeCommand(dependencies, "--config-dir", dir, "login", "--token"); err != nil {
		t.Fatal(err)
	}
	if _, err := executeCommand(testDependencies(server.Client(), nil), "--config-dir", dir, "login"); err == nil {
		t.Fatal("login without token mode succeeded")
	}
	empty := testDependencies(server.Client(), nil)
	empty.In = strings.NewReader("\n")
	if _, err := executeCommand(empty, "--config-dir", dir, "login", "--token"); err == nil {
		t.Fatal("empty token succeeded")
	}
}

func TestPlatformDoctorJSONAndFailedCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/setup/preflight":
			_, _ = w.Write([]byte(`{"ready":true,"mode":"ready"}`))
		case "/metrics":
			_, _ = w.Write([]byte("ok"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	dependencies := testDependencies(server.Client(), nil)
	dependencies.LookPath = func(tool string) (string, error) { return "/bin/" + tool, nil }
	output, err := executeCommand(dependencies, "--api", server.URL, "platform", "doctor", "-o", "json")
	if err != nil || !strings.Contains(output, `"name": "api/preflight"`) || !strings.Contains(output, `"severity": "ok"`) {
		t.Fatalf("doctor = %q, %v", output, err)
	}
	if _, err := executeCommand(dependencies, "--api", server.URL, "platform", "doctor", "-o", "toml"); err == nil {
		t.Fatal("unsupported doctor format succeeded")
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"ready":false}`))
	}))
	defer failing.Close()
	if _, err := executeCommand(testDependencies(failing.Client(), nil), "--api", failing.URL, "platform", "doctor"); err == nil {
		t.Fatal("failed doctor returned success")
	}
}

func TestRootVersionAndInputHelpers(t *testing.T) {
	output, err := executeCommand(testDependencies(nil, nil), "version")
	if err == nil || output != "" || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("version command error = %v", err)
	}
	output, err = executeCommand(testDependencies(nil, nil), "--version")
	if err != nil || !strings.Contains(output, "test") {
		t.Fatalf("--version = %q, %v", output, err)
	}
	if _, _, err := openRequestBody(nil, "-", ""); err == nil {
		t.Fatal("nil stdin accepted")
	}
	if _, _, err := openRequestBody(nil, filepath.Join(t.TempDir(), "missing"), ""); err == nil {
		t.Fatal("missing data file accepted")
	}
	if _, err := validateTokenInput("one\ntwo"); err == nil {
		t.Fatal("multiline token accepted")
	}
	if _, err := executeCommand(testDependencies(nil, map[string]string{"NOPSAI_TOKEN": "one\ntwo"}), "--api", "https://example.com", "api", "request", "GET", "/"); err == nil {
		t.Fatal("multiline environment token accepted")
	}
	if _, err := executeCommand(testDependencies(nil, nil), "--timeout", "-1s", "--api", "https://example.com", "api", "request", "GET", "/"); err == nil {
		t.Fatal("negative timeout accepted")
	}
}

func testDependencies(client *http.Client, environment map[string]string) Dependencies {
	return Dependencies{
		In:         strings.NewReader(""),
		HTTPClient: client,
		Getenv: func(key string) string {
			return environment[key]
		},
		LookPath:   func(string) (string, error) { return "", errors.New("missing") },
		RunCommand: func(context.Context, string, ...string) error { return nil },
		Version:    "test",
	}
}

func commandBuildInfo(version string) buildinfo.Info {
	return buildinfo.Info{
		Version:               version,
		APIVersion:            "v1",
		RunnerProtocolVersion: 1,
		PlatformCompatibility: ">=2.0.0 <3.0.0",
	}
}

func writeCommandInstallManifest(t *testing.T) string {
	t.Helper()
	images := make(map[string]string, len(compatibility.RequiredPlatformImages))
	for _, name := range compatibility.RequiredPlatformImages {
		images[name] = "ghcr.io/example/nopsai-" + strings.ToLower(name) + "@sha256:" + strings.Repeat("a", 64)
	}
	manifest := compatibility.Manifest{
		SchemaVersion: "v1",
		Version:       "2.7.0",
		Chart: compatibility.ChartArtifact{
			Reference: "oci://ghcr.io/example/charts/nopsai",
			Version:   "2.7.0",
			Digest:    "sha256:" + strings.Repeat("b", 64),
		},
		Images:        images,
		Compatibility: compatibility.ManifestCompatibility{CLI: ">=2.0.0 <3.0.0", API: "v1", RunnerProtocol: 1},
		Database:      compatibility.DatabaseContract{MigrationVersion: 1, RollbackSafe: false, RollbackPolicy: "forward-only"},
		Capabilities: []string{
			compatibility.CapabilityAPIV1,
			compatibility.CapabilityPlatformCompose,
			compatibility.CapabilityPlatformHelm,
			compatibility.CapabilityRunnerDocker,
			compatibility.CapabilityRunnerK8s,
		},
	}
	contents, err := compatibility.CanonicalJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "release-manifest.json")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeCommandReleaseManifest(t *testing.T, chart []byte) string {
	t.Helper()
	images := make(map[string]string, len(compatibility.RequiredPlatformImages))
	for _, name := range compatibility.RequiredPlatformImages {
		images[name] = "ghcr.io/example/nopsai-" + strings.ToLower(name) + "@sha256:" + strings.Repeat("a", 64)
	}
	manifest := compatibility.Manifest{
		SchemaVersion: "v1",
		Version:       "2.7.0",
		Chart: compatibility.ChartArtifact{
			Reference: "oci://ghcr.io/example/charts/nopsai",
			Version:   "2.7.0",
			Digest:    compatibility.DigestBytes(chart),
		},
		Images:        images,
		Compatibility: compatibility.ManifestCompatibility{CLI: ">=2.0.0 <3.0.0", API: "v1", RunnerProtocol: 1},
		Database:      compatibility.DatabaseContract{MigrationVersion: 1, RollbackSafe: false, RollbackPolicy: "forward-only"},
		Capabilities:  []string{compatibility.CapabilityAPIV1, compatibility.CapabilityPlatformHelm},
	}
	contents, err := compatibility.CanonicalJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "release-manifest.json")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func commandArgumentValue(args []string, name string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name {
			return args[index+1]
		}
	}
	return ""
}

func containsCommandArgument(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func fieldByName(fields []interactive.Field, name string) (interactive.Field, bool) {
	for _, field := range fields {
		if field.Name == name {
			return field, true
		}
	}
	return interactive.Field{}, false
}

func executeCommand(dependencies Dependencies, args ...string) (string, error) {
	var output bytes.Buffer
	var errorsOutput bytes.Buffer
	dependencies.Out = &output
	dependencies.Err = &errorsOutput
	command := NewRootCommand(dependencies)
	command.SetArgs(args)
	err := command.Execute()
	return output.String(), err
}

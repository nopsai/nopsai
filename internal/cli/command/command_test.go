package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nopsai/internal/cli/apicatalog"
	clconfig "nopsai/internal/cli/config"
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

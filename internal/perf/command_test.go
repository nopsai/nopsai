package perf

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestIntsToString(t *testing.T) {
	if got := intsToString([]int{1, 5, 10}); got != "1,5,10" {
		t.Fatalf("intsToString = %q", got)
	}
	if got := intsToString(nil); got != "" {
		t.Fatalf("intsToString(nil) = %q, want an empty string", got)
	}
}

func TestNewCommandDeclaresEverySuiteInItsHelp(t *testing.T) {
	cmd := NewCommand("test")
	help := cmd.Long + cmd.Flags().FlagUsages()
	for _, suite := range SuiteNames() {
		if !strings.Contains(help, suite) {
			t.Errorf("help text does not document the %q suite", suite)
		}
	}
}

func TestNewCommandRejectsMalformedConcurrency(t *testing.T) {
	cmd := NewCommand("test")
	cmd.SetArgs([]string{"--concurrency", "1,not-a-number"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("the command accepted a malformed concurrency ramp")
	}
	if !strings.Contains(err.Error(), "--concurrency") {
		t.Fatalf("error = %q, want it to name the offending flag", err)
	}
}

func TestNewCommandRejectsMalformedPipelineConcurrency(t *testing.T) {
	cmd := NewCommand("test")
	cmd.SetArgs([]string{"--pipeline-concurrency", "x"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err := cmd.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "--pipeline-concurrency") {
		t.Fatalf("error = %v, want it to name the offending flag", err)
	}
}

// commandStack is the smallest stack the CLI needs to complete a run.
func commandStack(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"version": "test-1.0"})
	})
	mux.HandleFunc("/v1/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-token",
			"expires_at":   time.Now().Add(time.Hour),
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`[]`)) })

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func commandArgs(serverURL, outputDir string, extra ...string) []string {
	args := []string{
		"--api-url", serverURL,
		"--suites", SuiteAPIRead,
		"--concurrency", "1",
		"--stage-duration", "120ms",
		"--warmup", "20ms",
		"--no-resources",
		"--output-dir", outputDir,
	}
	return append(args, extra...)
}

func TestCommandRunPrintsReportAndWritesArtifacts(t *testing.T) {
	server := commandStack(t)
	dir := t.TempDir()

	var stdout, stderr bytes.Buffer
	cmd := NewCommand("test")
	cmd.SetArgs(commandArgs(server.URL, dir))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("command returned %v", err)
	}
	if !strings.Contains(stdout.String(), "VERDICT: EFFICIENT OPERATING NUMBERS") {
		t.Fatalf("the report was not printed to stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "reports written to") {
		t.Errorf("the artifact paths were not reported:\n%s", stderr.String())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir returned %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d artifacts, want a text and a JSON report", len(entries))
	}
}

// TestCommandQuietSuppressesProgressButKeepsTheReport separates the two output
// streams: progress is noise, the report is the deliverable.
func TestCommandQuietSuppressesProgressButKeepsTheReport(t *testing.T) {
	server := commandStack(t)

	var stdout, stderr bytes.Buffer
	cmd := NewCommand("test")
	cmd.SetArgs(commandArgs(server.URL, "", "--quiet"))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("command returned %v", err)
	}
	if strings.Contains(stderr.String(), "stage: concurrency") {
		t.Errorf("--quiet still emitted stage progress:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "LOAD RAMP") {
		t.Error("--quiet suppressed the report itself")
	}
}

// TestCommandFailOnBreachExitsNonZero is what makes the harness usable as a
// regression gate rather than only as a human-read report.
func TestCommandFailOnBreachExitsNonZero(t *testing.T) {
	server := commandStack(t)

	var stdout, stderr bytes.Buffer
	cmd := NewCommand("test")
	// A 1ns SLO cannot be met, so every stage breaches.
	cmd.SetArgs(commandArgs(server.URL, "", "--fail-on-breach", "--latency-slo", "1ns"))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("--fail-on-breach did not fail a breaching run")
	}
	if !strings.Contains(err.Error(), "no concurrency level met") {
		t.Fatalf("error = %q", err)
	}
	// The report must still be printed so the operator can see why it failed.
	if !strings.Contains(stdout.String(), "LOAD RAMP") {
		t.Error("the report was withheld on failure")
	}
}

func TestCommandSucceedsWithoutFailOnBreachDespiteBreaches(t *testing.T) {
	server := commandStack(t)

	cmd := NewCommand("test")
	cmd.SetArgs(commandArgs(server.URL, "", "--latency-slo", "1ns"))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("a breaching run without --fail-on-breach returned %v", err)
	}
}

func TestCommandPropagatesRunFailure(t *testing.T) {
	cmd := NewCommand("test")
	cmd.SetArgs(commandArgs("http://127.0.0.1:1", ""))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("the command succeeded against an unreachable target")
	}
}

package perf

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeStack serves the endpoints the harness needs for a full run: health,
// version, login and the read surface.
type fakeStack struct {
	loginStatus  int
	healthStatus int
	logins       atomic.Int64
	requests     atomic.Int64
}

func (f *fakeStack) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		status := f.healthStatus
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
	})
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"version": "test-1.0"})
	})
	mux.HandleFunc("/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		f.logins.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		status := f.loginStatus
		if status == 0 {
			status = http.StatusOK
		}
		if status != http.StatusOK {
			http.Error(w, "invalid credentials", status)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-token",
			"expires_at":   time.Now().Add(time.Hour),
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func harnessConfig(serverURL, outputDir string) Config {
	cfg := DefaultConfig()
	cfg.APIURL = serverURL
	cfg.Suites = []string{SuiteAPIRead}
	cfg.Concurrency = []int{1, 2}
	cfg.StageDuration = 120 * time.Millisecond
	cfg.WarmupDuration = 20 * time.Millisecond
	cfg.RequestTimeout = 2 * time.Second
	// Resource sampling needs Docker, which a unit test must not depend on.
	cfg.Containers = nil
	cfg.OutputDir = outputDir
	return cfg
}

func TestRunProducesACompleteReport(t *testing.T) {
	stack := &fakeStack{}
	server := stack.server(t)

	report, err := Run(context.Background(), harnessConfig(server.URL, ""), io.Discard)
	if err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if len(report.Stages) != 2 {
		t.Fatalf("got %d stages, want 2", len(report.Stages))
	}
	if report.Version != "test-1.0" {
		t.Errorf("Version = %q, want the version reported by the stack", report.Version)
	}
	if report.Target != server.URL {
		t.Errorf("Target = %q, want %q", report.Target, server.URL)
	}
	if report.Duration <= 0 {
		t.Error("run duration was not recorded")
	}
	if report.Analysis.PeakThroughput <= 0 {
		t.Error("the analysis produced no peak throughput")
	}
	if stack.requests.Load() == 0 {
		t.Error("no load reached the stack")
	}
	// Every scenario in the api-read suite must have been exercised.
	if len(report.Stages[0].Scenarios) < 5 {
		t.Errorf("only %d scenarios ran, want the full api-read mix", len(report.Stages[0].Scenarios))
	}
}

// TestRunAuthenticatesOnceForTheWholeRun protects the read-path measurements
// from being polluted by repeated login cost.
func TestRunAuthenticatesOnceForTheWholeRun(t *testing.T) {
	stack := &fakeStack{}
	server := stack.server(t)

	if _, err := Run(context.Background(), harnessConfig(server.URL, ""), io.Discard); err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if got := stack.logins.Load(); got != 1 {
		t.Fatalf("the harness logged in %d times, want exactly 1 for an api-read run", got)
	}
}

func TestRunFailsFastWhenTheStackIsDown(t *testing.T) {
	server := (&fakeStack{}).server(t)
	url := server.URL
	server.Close()

	_, err := Run(context.Background(), harnessConfig(url, ""), io.Discard)
	if err == nil {
		t.Fatal("Run succeeded against a stopped stack")
	}
	if !strings.Contains(err.Error(), "docker compose up") {
		t.Fatalf("error = %q, want it to tell the operator how to start the stack", err)
	}
}

func TestRunFailsFastWhenHealthzIsUnhealthy(t *testing.T) {
	stack := &fakeStack{healthStatus: http.StatusServiceUnavailable}
	server := stack.server(t)

	_, err := Run(context.Background(), harnessConfig(server.URL, ""), io.Discard)
	if err == nil {
		t.Fatal("Run succeeded against an unhealthy stack")
	}
	if !strings.Contains(err.Error(), "not ready for a load test") {
		t.Fatalf("error = %q", err)
	}
}

// TestRunFailsWithActionableAuthError keeps a credential problem from being
// reported as a performance result.
func TestRunFailsWithActionableAuthError(t *testing.T) {
	stack := &fakeStack{loginStatus: http.StatusUnauthorized}
	server := stack.server(t)

	_, err := Run(context.Background(), harnessConfig(server.URL, ""), io.Discard)
	if err == nil {
		t.Fatal("Run succeeded with rejected credentials")
	}
	if !strings.Contains(err.Error(), "NOPSAI_PERF_PASSWORD") {
		t.Fatalf("error = %q, want it to name the variables to set", err)
	}
}

func TestRunRejectsInvalidConfiguration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Suites = []string{"nonsense"}
	if _, err := Run(context.Background(), cfg, io.Discard); err == nil {
		t.Fatal("Run accepted an invalid configuration")
	}
}

func TestRunReportsMissingPayloadFile(t *testing.T) {
	stack := &fakeStack{}
	server := stack.server(t)

	cfg := harnessConfig(server.URL, "")
	cfg.Suites = []string{SuiteWebhook}
	cfg.WebhookSecret = "secret"
	cfg.PayloadFile = filepath.Join(t.TempDir(), "missing.json")

	_, err := Run(context.Background(), cfg, io.Discard)
	if err == nil {
		t.Fatal("Run succeeded with a missing payload file")
	}
	if !strings.Contains(err.Error(), "read webhook payload") {
		t.Fatalf("error = %q", err)
	}
}

func TestRunWritesArtifactsToTheOutputDirectory(t *testing.T) {
	stack := &fakeStack{}
	server := stack.server(t)
	dir := t.TempDir()

	report, err := Run(context.Background(), harnessConfig(server.URL, dir), io.Discard)
	if err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if _, _, err := WriteArtifacts(report, dir); err != nil {
		t.Fatalf("WriteArtifacts returned %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir returned %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d artifacts, want a text and a JSON report", len(entries))
	}
}

func TestPeakConcurrencyCoversBothRamps(t *testing.T) {
	cfg := Config{Concurrency: []int{1, 25}, PipelineConcurrency: []int{3, 40}}
	if got := peakConcurrency(cfg); got != 40 {
		t.Fatalf("peakConcurrency = %d, want 40", got)
	}
	if got := peakConcurrency(Config{}); got != 1 {
		t.Fatalf("peakConcurrency of an empty config = %d, want 1", got)
	}
}

func TestRunWritesProgressOutput(t *testing.T) {
	stack := &fakeStack{}
	server := stack.server(t)

	var progress strings.Builder
	if _, err := Run(context.Background(), harnessConfig(server.URL, ""), &progress); err != nil {
		t.Fatalf("Run returned %v", err)
	}
	text := progress.String()
	if !strings.Contains(text, "preflight") {
		t.Error("progress output did not report preflight")
	}
	if !strings.Contains(text, "stage: concurrency=1") {
		t.Errorf("progress output did not report stages:\n%s", text)
	}
}

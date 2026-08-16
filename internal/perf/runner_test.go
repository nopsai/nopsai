package perf

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// newRunnerHarness returns a Runner wired to the given handler, along with the
// stub server, using stage settings short enough for a unit test.
func newRunnerHarness(t *testing.T, handler http.Handler, mutate func(*Config)) (*Runner, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	cfg := DefaultConfig()
	cfg.APIURL = server.URL
	cfg.StageDuration = 150 * time.Millisecond
	cfg.WarmupDuration = 20 * time.Millisecond
	cfg.RequestTimeout = 2 * time.Second
	cfg.Concurrency = []int{1, 2}
	cfg.LatencySLO = time.Second
	cfg.ErrorBudget = 0.01
	if mutate != nil {
		mutate(&cfg)
	}

	request := &RequestContext{
		APIURL:      server.URL,
		TokenSource: func() (string, error) { return "test-token", nil },
	}
	mix := NewMix([]Scenario{authenticatedGet("probe", SuiteAPIRead, ServiceAPI, 1, "/v1/runs")})
	return NewRunner(cfg, NewHTTPClient(cfg.RequestTimeout, 4), mix, request, nil, nil), server
}

func TestRunStageMeasuresThroughputAndLatency(t *testing.T) {
	runner, _ := newRunnerHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}), nil)

	report, err := runner.RunStage(context.Background(), 2)
	if err != nil {
		t.Fatalf("RunStage returned %v", err)
	}
	if report.Concurrency != 2 {
		t.Errorf("Concurrency = %d, want 2", report.Concurrency)
	}
	if report.Total.Requests == 0 {
		t.Fatal("no requests were recorded")
	}
	if report.Total.Throughput <= 0 {
		t.Error("throughput was not computed")
	}
	if report.Total.ErrorRate != 0 {
		t.Errorf("ErrorRate = %v, want 0 against a healthy server", report.Total.ErrorRate)
	}
	if report.Total.Latency.P95 <= 0 {
		t.Error("p95 was not computed")
	}
	if report.Saturated {
		t.Error("a healthy stage was marked saturated")
	}
	if len(report.Scenarios) != 1 || report.Scenarios[0].Name != "probe" {
		t.Fatalf("scenario breakdown = %+v", report.Scenarios)
	}
}

// TestRunStageExcludesWarmupFromMeasurements is the guard for measurement
// correctness: requests issued during warmup must not reach the recorder.
func TestRunStageExcludesWarmupFromMeasurements(t *testing.T) {
	var served atomic.Int64
	runner, _ := newRunnerHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served.Add(1)
		w.WriteHeader(http.StatusOK)
	}), func(c *Config) {
		// Spend most of the stage in warmup so the gap is unambiguous.
		c.StageDuration = 200 * time.Millisecond
		c.WarmupDuration = 150 * time.Millisecond
	})

	report, err := runner.RunStage(context.Background(), 1)
	if err != nil {
		t.Fatalf("RunStage returned %v", err)
	}
	if report.Total.Requests == 0 {
		t.Fatal("no requests were recorded after warmup")
	}
	if report.Total.Requests >= served.Load() {
		t.Fatalf("recorded %d of %d served requests; warmup traffic was not excluded",
			report.Total.Requests, served.Load())
	}
	// Throughput must be derived from the measured window, not the whole stage.
	if report.Measured > 100*time.Millisecond {
		t.Errorf("Measured = %v, want roughly the 50ms post-warmup window", report.Measured)
	}
}

func TestRunStageRecordsServerErrors(t *testing.T) {
	runner, _ := newRunnerHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "overloaded", http.StatusServiceUnavailable)
	}), nil)

	report, err := runner.RunStage(context.Background(), 1)
	if err != nil {
		t.Fatalf("RunStage returned %v", err)
	}
	if report.Total.ErrorRate != 1 {
		t.Fatalf("ErrorRate = %v, want 1", report.Total.ErrorRate)
	}
	if report.Total.StatusCodes[http.StatusServiceUnavailable] == 0 {
		t.Error("503 responses were not counted")
	}
	if !report.Saturated {
		t.Error("a stage failing every request should be marked saturated")
	}
}

// TestRunStageMarksSaturationOnLatencyBreach separates the two saturation
// signals: this server is healthy but too slow for the SLO.
func TestRunStageMarksSaturationOnLatencyBreach(t *testing.T) {
	runner, _ := newRunnerHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(40 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}), func(c *Config) {
		c.LatencySLO = 5 * time.Millisecond
	})

	report, err := runner.RunStage(context.Background(), 1)
	if err != nil {
		t.Fatalf("RunStage returned %v", err)
	}
	if report.Total.ErrorRate != 0 {
		t.Fatalf("ErrorRate = %v, want 0: the server answered every request", report.Total.ErrorRate)
	}
	if !report.Saturated {
		t.Fatal("a stage breaching the latency SLO should be marked saturated")
	}
}

func TestRunExecutesEveryConcurrencyLevelInOrder(t *testing.T) {
	runner, _ := newRunnerHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), func(c *Config) {
		c.Concurrency = []int{4, 1, 2}
	})

	reports, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if len(reports) != 3 {
		t.Fatalf("got %d stages, want 3", len(reports))
	}
	for i, want := range []int{1, 2, 4} {
		if reports[i].Concurrency != want {
			t.Errorf("stage %d had concurrency %d, want %d", i, reports[i].Concurrency, want)
		}
	}
}

// TestRunAbortsRampOnCollapse verifies the early stop: once the system fails
// outright there is nothing to learn from pushing harder.
func TestRunAbortsRampOnCollapse(t *testing.T) {
	runner, _ := newRunnerHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}), func(c *Config) {
		c.Concurrency = []int{1, 2, 4, 8}
		c.AbortErrorRate = 0.5
	})

	reports, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("got %d stages, want the ramp to stop after the first collapse", len(reports))
	}
}

func TestRunContinuesPastCollapseWhenAbortDisabled(t *testing.T) {
	runner, _ := newRunnerHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}), func(c *Config) {
		c.Concurrency = []int{1, 2}
		c.AbortErrorRate = 0
	})

	reports, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("got %d stages, want both levels when the abort check is disabled", len(reports))
	}
}

func TestRunStopsWhenContextIsCancelled(t *testing.T) {
	runner, _ := newRunnerHarness(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), func(c *Config) {
		c.Concurrency = []int{1, 2, 4}
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reports, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if len(reports) != 0 {
		t.Fatalf("got %d stages, want none from an already-cancelled context", len(reports))
	}
}

func TestRunSkipsAnEmptyMix(t *testing.T) {
	cfg := DefaultConfig()
	runner := NewRunner(cfg, NewHTTPClient(time.Second, 1), NewMix(nil), &RequestContext{}, nil, nil)
	reports, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if reports != nil {
		t.Fatalf("got %d stages from an empty mix, want none", len(reports))
	}
}

// TestRunStageRecordsBuildFailures makes sure a credential problem shows up in
// the report rather than spinning silently.
func TestRunStageRecordsBuildFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	cfg := DefaultConfig()
	cfg.StageDuration = 80 * time.Millisecond
	cfg.WarmupDuration = 0
	request := &RequestContext{
		APIURL:      server.URL,
		TokenSource: func() (string, error) { return "", context.DeadlineExceeded },
	}
	mix := NewMix([]Scenario{authenticatedGet("probe", SuiteAPIRead, ServiceAPI, 1, "/v1/runs")})
	runner := NewRunner(cfg, NewHTTPClient(time.Second, 1), mix, request, nil, nil)

	report, err := runner.RunStage(context.Background(), 1)
	if err != nil {
		t.Fatalf("RunStage returned %v", err)
	}
	if report.Total.Errors["request build failed"] == 0 {
		t.Fatalf("build failures were not recorded: %+v", report.Total.Errors)
	}
}

func TestExecuteRequestClassifiesTransportFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close() // Close first so the connection is refused.

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest returned %v", err)
	}
	result := ExecuteRequest(NewHTTPClient(time.Second, 1), Scenario{Name: "probe"}, req)
	if result.Err == "" {
		t.Fatal("a refused connection produced no error label")
	}
	if !result.Failed() {
		t.Error("a refused connection must count as a failure")
	}
}

func TestClassifyError(t *testing.T) {
	for _, testCase := range []struct {
		message string
		want    string
	}{
		{"dial tcp: connection refused", "connection refused"},
		{"read: connection reset by peer", "connection reset"},
		{"context deadline exceeded", "timeout"},
		{"context canceled", "canceled"},
		{"unexpected EOF", "unexpected EOF"},
		{"lookup nope: no such host", "dns failure"},
		{"dial tcp: cannot assign requested address", "ephemeral port exhaustion"},
		{"socket: too many open files", "file descriptor exhaustion"},
		{"something else entirely", "transport error"},
	} {
		if got := classifyError(errorString(testCase.message)); got != testCase.want {
			t.Errorf("classifyError(%q) = %q, want %q", testCase.message, got, testCase.want)
		}
	}
	if got := classifyError(nil); got != "" {
		t.Errorf("classifyError(nil) = %q, want an empty label", got)
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }

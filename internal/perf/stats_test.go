package perf

import (
	"sync"
	"testing"
	"time"
)

func TestResultFailedClassifiesTransportAndStatusFailures(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		result Result
		want   bool
	}{
		{"2xx succeeds", Result{Status: 200}, false},
		{"3xx succeeds", Result{Status: 302}, false},
		{"4xx fails", Result{Status: 401}, true},
		{"5xx fails", Result{Status: 503}, true},
		{"transport error fails even with a status", Result{Status: 200, Err: "timeout"}, true},
		{"missing status fails", Result{}, true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.result.Failed(); got != testCase.want {
				t.Fatalf("Failed() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestPercentileUsesNearestRank(t *testing.T) {
	sorted := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
		60 * time.Millisecond,
		70 * time.Millisecond,
		80 * time.Millisecond,
		90 * time.Millisecond,
		100 * time.Millisecond,
	}
	for _, testCase := range []struct {
		quantile float64
		want     time.Duration
	}{
		{0.50, 50 * time.Millisecond},
		{0.90, 90 * time.Millisecond},
		{0.95, 100 * time.Millisecond},
		{0.99, 100 * time.Millisecond},
		{0.0, 10 * time.Millisecond},
		{1.0, 100 * time.Millisecond},
	} {
		if got := percentile(sorted, testCase.quantile); got != testCase.want {
			t.Errorf("percentile(%v) = %v, want %v", testCase.quantile, got, testCase.want)
		}
	}
}

func TestPercentileHandlesEmptyInput(t *testing.T) {
	if got := percentile(nil, 0.95); got != 0 {
		t.Fatalf("percentile(nil) = %v, want 0", got)
	}
}

func TestSummarizeComputesDistribution(t *testing.T) {
	latencies := []time.Duration{
		30 * time.Millisecond,
		10 * time.Millisecond,
		20 * time.Millisecond,
		40 * time.Millisecond,
	}
	stats := summarize(latencies)

	if stats.Min != 10*time.Millisecond {
		t.Errorf("Min = %v, want 10ms", stats.Min)
	}
	if stats.Max != 40*time.Millisecond {
		t.Errorf("Max = %v, want 40ms", stats.Max)
	}
	if stats.Mean != 25*time.Millisecond {
		t.Errorf("Mean = %v, want 25ms", stats.Mean)
	}
	if stats.P50 != 20*time.Millisecond {
		t.Errorf("P50 = %v, want 20ms", stats.P50)
	}
	if stats.StdDev == 0 {
		t.Error("StdDev = 0, want a non-zero spread for a varied sample")
	}
}

func TestSummarizeHandlesEmptyInput(t *testing.T) {
	if stats := summarize(nil); stats != (LatencyStats{}) {
		t.Fatalf("summarize(nil) = %+v, want the zero value", stats)
	}
}

func TestRecorderSnapshotSeparatesScenarios(t *testing.T) {
	recorder := NewRecorder()
	recorder.Record(Result{Scenario: "a", Latency: 10 * time.Millisecond, Status: 200, Bytes: 100})
	recorder.Record(Result{Scenario: "a", Latency: 30 * time.Millisecond, Status: 200, Bytes: 100})
	recorder.Record(Result{Scenario: "b", Latency: 50 * time.Millisecond, Status: 500})
	recorder.Record(Result{Scenario: "b", Latency: 70 * time.Millisecond, Err: "timeout"})

	scenarios := recorder.Snapshot(2 * time.Second)
	if len(scenarios) != 2 {
		t.Fatalf("got %d scenarios, want 2", len(scenarios))
	}
	// Snapshot sorts by name, so index 0 is "a".
	if scenarios[0].Name != "a" || scenarios[1].Name != "b" {
		t.Fatalf("scenarios are not sorted by name: %q, %q", scenarios[0].Name, scenarios[1].Name)
	}
	if scenarios[0].Failures != 0 {
		t.Errorf("scenario a failures = %d, want 0", scenarios[0].Failures)
	}
	if scenarios[0].BytesTotal != 200 {
		t.Errorf("scenario a bytes = %d, want 200", scenarios[0].BytesTotal)
	}
	if scenarios[0].Throughput != 1 {
		t.Errorf("scenario a throughput = %v, want 1 req/s over a 2s window", scenarios[0].Throughput)
	}
	if scenarios[1].Failures != 2 {
		t.Errorf("scenario b failures = %d, want 2", scenarios[1].Failures)
	}
	if scenarios[1].ErrorRate != 1 {
		t.Errorf("scenario b error rate = %v, want 1", scenarios[1].ErrorRate)
	}
	if scenarios[1].Errors["timeout"] != 1 {
		t.Errorf("scenario b timeout count = %d, want 1", scenarios[1].Errors["timeout"])
	}
	if scenarios[1].StatusCodes[500] != 1 {
		t.Errorf("scenario b 500 count = %d, want 1", scenarios[1].StatusCodes[500])
	}
}

// TestRecorderOverallIsExactAcrossScenarios is the guard for the property that
// motivated the combined sample set: a stage percentile must come from the real
// merged distribution, not from combining per-scenario percentiles.
func TestRecorderOverallIsExactAcrossScenarios(t *testing.T) {
	recorder := NewRecorder()
	// Scenario "fast" is uniformly 10ms; scenario "slow" is uniformly 100ms.
	// Nine fast requests and one slow one put the true p95 at 100ms, while the
	// mean of the two per-scenario p95 values would be 55ms.
	for i := 0; i < 9; i++ {
		recorder.Record(Result{Scenario: "fast", Latency: 10 * time.Millisecond, Status: 200})
	}
	recorder.Record(Result{Scenario: "slow", Latency: 100 * time.Millisecond, Status: 200})

	overall := recorder.Overall("all", time.Second)
	if overall.Requests != 10 {
		t.Fatalf("Requests = %d, want 10", overall.Requests)
	}
	if overall.Latency.P95 != 100*time.Millisecond {
		t.Errorf("P95 = %v, want 100ms from the true merged distribution", overall.Latency.P95)
	}
	if overall.Latency.P50 != 10*time.Millisecond {
		t.Errorf("P50 = %v, want 10ms", overall.Latency.P50)
	}
	if overall.Latency.Max != 100*time.Millisecond {
		t.Errorf("Max = %v, want 100ms", overall.Latency.Max)
	}
	if overall.Throughput != 10 {
		t.Errorf("Throughput = %v, want 10 req/s", overall.Throughput)
	}
}

func TestRecorderIsSafeForConcurrentUse(t *testing.T) {
	recorder := NewRecorder()
	const workers, perWorker = 8, 200

	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				recorder.Record(Result{Scenario: "shared", Latency: time.Millisecond, Status: 200})
			}
		}()
	}
	wg.Wait()

	overall := recorder.Overall("all", time.Second)
	if overall.Requests != workers*perWorker {
		t.Fatalf("Requests = %d, want %d", overall.Requests, workers*perWorker)
	}
}

func TestRecorderOverallHandlesNoObservations(t *testing.T) {
	overall := NewRecorder().Overall("all", time.Second)
	if overall.Requests != 0 || overall.ErrorRate != 0 || overall.Throughput != 0 {
		t.Fatalf("empty recorder produced %+v, want zeroes", overall)
	}
}

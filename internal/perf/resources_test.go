package perf

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestParseByteSize(t *testing.T) {
	for _, testCase := range []struct {
		raw  string
		want float64
	}{
		{"1B", 1},
		{"1KiB", 1024},
		{"1MiB", 1 << 20},
		{"2.5GiB", 2.5 * (1 << 30)},
		{" 12.3MiB ", 12.3 * (1 << 20)},
		{"1GB", 1e9},
		{"garbage", 0},
		{"", 0},
	} {
		if got := parseByteSize(testCase.raw); got != testCase.want {
			t.Errorf("parseByteSize(%q) = %v, want %v", testCase.raw, got, testCase.want)
		}
	}
}

func TestParsePercent(t *testing.T) {
	for _, testCase := range []struct {
		raw  string
		want float64
	}{
		{"12.34%", 12.34},
		{" 0.00% ", 0},
		{"250%", 250},
		{"n/a", 0},
	} {
		if got := parsePercent(testCase.raw); got != testCase.want {
			t.Errorf("parsePercent(%q) = %v, want %v", testCase.raw, got, testCase.want)
		}
	}
}

func TestParseMemUsageSplitsUsedAndLimit(t *testing.T) {
	used, limit := parseMemUsage("12.3MiB / 7.66GiB")
	if used != 12.3*(1<<20) {
		t.Errorf("used = %v", used)
	}
	if limit != 7.66*(1<<30) {
		t.Errorf("limit = %v", limit)
	}
}

func TestParseDockerStatsReadsEveryContainer(t *testing.T) {
	at := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	output := []byte(`{"Name":"nopsai","CPUPerc":"142.50%","MemUsage":"256MiB / 2GiB","MemPerc":"12.50%","PIDs":"23"}
{"Name":"nopsai-db","CPUPerc":"12.00%","MemUsage":"512MiB / 2GiB","MemPerc":"25.00%","PIDs":"9"}

`)
	samples, err := ParseDockerStats(at, output)
	if err != nil {
		t.Fatalf("ParseDockerStats returned %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("got %d samples, want 2 (blank lines must be skipped)", len(samples))
	}
	first := samples[0]
	if first.Container != "nopsai" {
		t.Errorf("Container = %q", first.Container)
	}
	if first.CPUPercent != 142.5 {
		t.Errorf("CPUPercent = %v, want 142.5 (Docker reports above 100%% for multi-core)", first.CPUPercent)
	}
	if first.MemBytes != 256*(1<<20) {
		t.Errorf("MemBytes = %v", first.MemBytes)
	}
	if first.MemLimitBytes != 2*(1<<30) {
		t.Errorf("MemLimitBytes = %v", first.MemLimitBytes)
	}
	if first.PIDs != 23 {
		t.Errorf("PIDs = %d", first.PIDs)
	}
	if !first.At.Equal(at) {
		t.Errorf("At = %v, want %v", first.At, at)
	}
}

func TestParseDockerStatsRejectsMalformedJSON(t *testing.T) {
	if _, err := ParseDockerStats(time.Now(), []byte("{not json}")); err == nil {
		t.Fatal("ParseDockerStats accepted malformed input")
	}
}

func TestUsageBetweenAggregatesWithinWindow(t *testing.T) {
	base := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	samples := []ResourceSample{
		// Before the window: must be excluded.
		{At: base.Add(-time.Minute), Container: "nopsai", CPUPercent: 999, MemBytes: 999},
		{At: base, Container: "nopsai", CPUPercent: 100, MemBytes: 100, MemPercent: 10, MemLimitBytes: 1000},
		{At: base.Add(time.Second), Container: "nopsai", CPUPercent: 200, MemBytes: 300, MemPercent: 30, MemLimitBytes: 1000},
		{At: base.Add(time.Second), Container: "nopsai-db", CPUPercent: 50, MemBytes: 50, MemPercent: 5, MemLimitBytes: 1000},
		// After the window: must be excluded.
		{At: base.Add(time.Hour), Container: "nopsai", CPUPercent: 999, MemBytes: 999},
	}

	usage := UsageBetween(samples, base, base.Add(time.Minute))
	if len(usage) != 2 {
		t.Fatalf("got %d containers, want 2", len(usage))
	}
	// Sorted by descending average CPU, so the API container comes first.
	api := usage[0]
	if api.Container != "nopsai" {
		t.Fatalf("busiest container = %q, want nopsai", api.Container)
	}
	if api.Samples != 2 {
		t.Errorf("Samples = %d, want 2 (out-of-window samples must be dropped)", api.Samples)
	}
	if api.CPUAvg != 150 {
		t.Errorf("CPUAvg = %v, want 150", api.CPUAvg)
	}
	if api.CPUPeak != 200 {
		t.Errorf("CPUPeak = %v, want 200", api.CPUPeak)
	}
	if api.MemAvgBytes != 200 {
		t.Errorf("MemAvgBytes = %v, want 200", api.MemAvgBytes)
	}
	if api.MemPeakBytes != 300 {
		t.Errorf("MemPeakBytes = %v, want 300", api.MemPeakBytes)
	}
	if api.MemPeakPct != 30 {
		t.Errorf("MemPeakPct = %v, want 30", api.MemPeakPct)
	}
}

func TestUsageBetweenHandlesNoSamples(t *testing.T) {
	if usage := UsageBetween(nil, time.Now(), time.Now()); len(usage) != 0 {
		t.Fatalf("got %d entries, want none", len(usage))
	}
}

func TestSamplerCollectsUntilStopped(t *testing.T) {
	sampler := NewSampler(5*time.Millisecond, []string{"nopsai"}, func(context.Context, []string) ([]ResourceSample, error) {
		return []ResourceSample{{At: time.Now(), Container: "nopsai", CPUPercent: 10}}, nil
	})

	sampler.Start(context.Background())
	time.Sleep(40 * time.Millisecond)
	sampler.Stop()

	samples := sampler.Samples()
	if len(samples) == 0 {
		t.Fatal("sampler collected nothing")
	}
	if len(sampler.Errors()) != 0 {
		t.Fatalf("sampler reported errors: %v", sampler.Errors())
	}

	// Stopping must actually end collection.
	countAfterStop := len(sampler.Samples())
	time.Sleep(20 * time.Millisecond)
	if len(sampler.Samples()) != countAfterStop {
		t.Fatal("sampler kept collecting after Stop")
	}
}

// TestSamplerSurvivesCollectionFailures guards the rule that a resource
// sampling problem must degrade the report, never abort the load test.
func TestSamplerSurvivesCollectionFailures(t *testing.T) {
	sampler := NewSampler(5*time.Millisecond, []string{"nopsai"}, func(context.Context, []string) ([]ResourceSample, error) {
		return nil, errors.New("docker daemon unavailable")
	})
	sampler.Start(context.Background())
	time.Sleep(30 * time.Millisecond)
	sampler.Stop()

	if len(sampler.Samples()) != 0 {
		t.Fatal("a failing collector should produce no samples")
	}
	errs := sampler.Errors()
	if len(errs) != 1 {
		t.Fatalf("got %d distinct errors, want 1 deduplicated entry: %v", len(errs), errs)
	}
	if errs[0] != "docker daemon unavailable" {
		t.Fatalf("error = %q", errs[0])
	}
}

func TestSamplerIsNoOpWithoutContainers(t *testing.T) {
	sampler := NewSampler(time.Millisecond, nil, func(context.Context, []string) ([]ResourceSample, error) {
		t.Fatal("collector must not run when no containers are configured")
		return nil, nil
	})
	sampler.Start(context.Background())
	sampler.Stop()
	if len(sampler.Samples()) != 0 {
		t.Fatal("expected no samples")
	}
}

func TestFormatBytes(t *testing.T) {
	for _, testCase := range []struct {
		value float64
		want  string
	}{
		{512, "512B"},
		{2048, "2.0KiB"},
		{5 * (1 << 20), "5.0MiB"},
		{2.5 * (1 << 30), "2.50GiB"},
	} {
		if got := FormatBytes(testCase.value); got != testCase.want {
			t.Errorf("FormatBytes(%v) = %q, want %q", testCase.value, got, testCase.want)
		}
	}
}

// TestDockerStatsAgainstRealDaemon exercises the real collector. It is skipped
// wherever Docker is unavailable, so the suite stays runnable in environments
// without a daemon while still covering the code path locally and in CI runners
// that do have one.
func TestDockerStatsAgainstRealDaemon(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	names, err := runningContainers(ctx, DefaultContainers())
	if err != nil {
		t.Skipf("docker is not usable: %v", err)
	}
	if len(names) == 0 {
		t.Skip("no nopsai containers are running")
	}

	samples, err := DockerStats(ctx, names)
	if err != nil {
		t.Fatalf("DockerStats returned %v", err)
	}
	if len(samples) == 0 {
		t.Fatal("DockerStats returned no samples for running containers")
	}
	for _, sample := range samples {
		if sample.Container == "" {
			t.Error("a sample has no container name")
		}
		if sample.MemLimitBytes <= 0 {
			t.Errorf("%s reported no memory limit", sample.Container)
		}
		if sample.At.IsZero() {
			t.Errorf("%s has no timestamp", sample.Container)
		}
	}
}

// TestDockerStatsSkipsContainersThatAreNotRunning covers the retry path: asking
// for a stopped container must not abort the whole sampling round.
func TestDockerStatsSkipsContainersThatAreNotRunning(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	running, err := runningContainers(ctx, DefaultContainers())
	if err != nil {
		t.Skipf("docker is not usable: %v", err)
	}
	if len(running) == 0 {
		t.Skip("no nopsai containers are running")
	}

	// Mix a real container with one that certainly does not exist.
	samples, err := DockerStats(ctx, append([]string{"nopsai-perf-no-such-container"}, running...))
	if err != nil {
		t.Fatalf("DockerStats aborted instead of skipping the missing container: %v", err)
	}
	if len(samples) == 0 {
		t.Fatal("the retry path returned no samples")
	}
}

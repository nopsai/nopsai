package perf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sampleReport() *Report {
	started := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	stages := []StageReport{
		{
			Concurrency: 1,
			Measured:    25 * time.Second,
			Total: ScenarioStats{
				Name: "all", Requests: 1200, Throughput: 48,
				Latency: LatencyStats{P50: 15 * time.Millisecond, P95: 30 * time.Millisecond},
			},
			Scenarios: []ScenarioStats{
				{Name: "runs.list", Requests: 800, Throughput: 32, Latency: LatencyStats{P95: 35 * time.Millisecond}},
				{Name: "health.livez", Requests: 400, Throughput: 16, Latency: LatencyStats{P95: 2 * time.Millisecond}},
			},
			Resources: []ContainerUsage{
				{Container: "nopsai", CPUAvg: 45, CPUPeak: 80, MemAvgBytes: 200 << 20, MemPeakBytes: 250 << 20, MemLimitBytes: 2 << 30, MemPeakPct: 12},
			},
		},
		{
			Concurrency: 50,
			Measured:    25 * time.Second,
			Saturated:   true,
			Total: ScenarioStats{
				Name: "all", Requests: 20000, Throughput: 800, ErrorRate: 0.05, Failures: 1000,
				Latency:     LatencyStats{P50: 40 * time.Millisecond, P95: 1500 * time.Millisecond},
				StatusCodes: map[int]int64{200: 19000, 503: 1000},
				Errors:      map[string]int64{"timeout": 12},
			},
			Scenarios: []ScenarioStats{
				{Name: "runs.list", Requests: 20000, Throughput: 800, Latency: LatencyStats{P95: 1500 * time.Millisecond}},
			},
			Resources: []ContainerUsage{
				{Container: "nopsai", CPUAvg: 320, CPUPeak: 390, MemAvgBytes: 900 << 20, MemPeakBytes: 1 << 30, MemLimitBytes: 2 << 30, MemPeakPct: 50},
			},
		},
	}
	cfg := DefaultConfig()
	return &Report{
		StartedAt:  started,
		EndedAt:    started.Add(2 * time.Minute),
		Duration:   2 * time.Minute,
		Target:     "http://127.0.0.1:8080",
		Suites:     []string{SuiteAPIRead},
		Version:    "1.2.3",
		Thresholds: Thresholds{LatencySLO: "1s", ErrorBudget: 0.01},
		Stages:     stages,
		Analysis:   Analyze(stages, cfg),
	}
}

func TestWriteTextRendersEverySection(t *testing.T) {
	var out strings.Builder
	if err := sampleReport().WriteText(&out); err != nil {
		t.Fatalf("WriteText returned %v", err)
	}
	text := out.String()

	for _, want := range []string{
		"NOPSAI BACKEND PERFORMANCE REPORT",
		"LOAD RAMP (per concurrency level)",
		"PER-ENDPOINT BREAKDOWN",
		"SERVICE RESOURCE USAGE",
		"VERDICT: EFFICIENT OPERATING NUMBERS",
		"http://127.0.0.1:8080",
		"1.2.3",
		// The ramp rows and their saturation marker.
		"SATURATED",
		// Per-service numbers must be present, not just the aggregate.
		"nopsai",
		// The verdict must name the operating point.
		"Safe operating point",
		"Peak throughput",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("report is missing %q", want)
		}
	}
}

// TestWriteTextReportsSafeOperatingPoint checks the number an operator actually
// acts on is present and correct, not merely that a section header rendered.
func TestWriteTextReportsSafeOperatingPoint(t *testing.T) {
	var out strings.Builder
	if err := sampleReport().WriteText(&out); err != nil {
		t.Fatalf("WriteText returned %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "1 concurrent clients, 48.0 req/s") {
		t.Fatalf("report did not state the safe operating point:\n%s", text)
	}
}

func TestWriteTextOmitsResourceSectionWhenNoSamples(t *testing.T) {
	report := sampleReport()
	for i := range report.Stages {
		report.Stages[i].Resources = nil
	}
	var out strings.Builder
	if err := report.WriteText(&out); err != nil {
		t.Fatalf("WriteText returned %v", err)
	}
	if strings.Contains(out.String(), "SERVICE RESOURCE USAGE") {
		t.Fatal("the resource section rendered with no samples to show")
	}
}

func TestWriteTextRendersPipelineSection(t *testing.T) {
	report := sampleReport()
	report.Stages = nil
	report.PipelineStages = []PipelineStageReport{{
		Concurrency: 3, Families: 3, Succeeded: 2, Failed: 1, TotalRuns: 6, RunsPerMinute: 1.5,
		IngestLatency:  LatencyStats{P95: 40 * time.Millisecond},
		VisibleLatency: LatencyStats{P95: 2 * time.Second},
		FamilyDuration: LatencyStats{P50: 3 * time.Minute, P95: 4 * time.Minute},
		FamilyResults: []PipelineFamilyResult{
			{Err: "timed out after 10m0s with 1/2 runs complete"},
		},
	}}

	var out strings.Builder
	if err := report.WriteText(&out); err != nil {
		t.Fatalf("WriteText returned %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "END-TO-END PIPELINE EXECUTION") {
		t.Error("the pipeline section is missing")
	}
	if !strings.Contains(text, "Pipeline failure reasons") {
		t.Error("pipeline failure reasons were not reported")
	}
	if !strings.Contains(text, "timed out after 10m0s") {
		t.Error("the specific failure reason is missing")
	}
}

func TestWriteTextReportsSamplingWarnings(t *testing.T) {
	report := sampleReport()
	report.SamplingErrors = []string{"docker daemon unavailable"}

	var out strings.Builder
	if err := report.WriteText(&out); err != nil {
		t.Fatalf("WriteText returned %v", err)
	}
	if !strings.Contains(out.String(), "docker daemon unavailable") {
		t.Fatal("sampling warnings were not surfaced")
	}
}

func TestWriteJSONRoundTrips(t *testing.T) {
	var out strings.Builder
	if err := sampleReport().WriteJSON(&out); err != nil {
		t.Fatalf("WriteJSON returned %v", err)
	}
	var decoded Report
	if err := json.Unmarshal([]byte(out.String()), &decoded); err != nil {
		t.Fatalf("the JSON report does not round-trip: %v", err)
	}
	if len(decoded.Stages) != 2 {
		t.Fatalf("decoded %d stages, want 2", len(decoded.Stages))
	}
	if decoded.Analysis.PeakThroughput != 800 {
		t.Errorf("decoded PeakThroughput = %v, want 800", decoded.Analysis.PeakThroughput)
	}
	if decoded.Stages[1].Total.StatusCodes[503] != 1000 {
		t.Error("status code counts did not survive serialization")
	}
}

func TestWriteArtifactsProducesBothFormats(t *testing.T) {
	dir := t.TempDir()
	textPath, jsonPath, err := WriteArtifacts(sampleReport(), dir)
	if err != nil {
		t.Fatalf("WriteArtifacts returned %v", err)
	}
	for _, path := range []string{textPath, jsonPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Size() == 0 {
			t.Fatalf("%s is empty", path)
		}
	}
	if filepath.Dir(textPath) != dir {
		t.Errorf("text report was written to %s, want %s", filepath.Dir(textPath), dir)
	}
	if !strings.HasSuffix(textPath, ".txt") || !strings.HasSuffix(jsonPath, ".json") {
		t.Errorf("unexpected artifact names: %s, %s", textPath, jsonPath)
	}
}

func TestWriteArtifactsSkippedWhenNoDirectory(t *testing.T) {
	textPath, jsonPath, err := WriteArtifacts(sampleReport(), "  ")
	if err != nil {
		t.Fatalf("WriteArtifacts returned %v", err)
	}
	if textPath != "" || jsonPath != "" {
		t.Fatal("artifacts were written despite no output directory")
	}
}

func TestShortFormatsAcrossMagnitudes(t *testing.T) {
	for _, testCase := range []struct {
		value time.Duration
		want  string
	}{
		{0, "-"},
		{500 * time.Microsecond, "0.50ms"},
		{25 * time.Millisecond, "25.0ms"},
		{2500 * time.Millisecond, "2.50s"},
		{90 * time.Second, "1m30s"},
	} {
		if got := short(testCase.value); got != testCase.want {
			t.Errorf("short(%v) = %q, want %q", testCase.value, got, testCase.want)
		}
	}
}

// TestWriteTextReportsBrokenScenarios covers the section that keeps a
// misconfigured endpoint from being read as a capacity limit.
func TestWriteTextReportsBrokenScenarios(t *testing.T) {
	report := sampleReport()
	report.Analysis.Broken = []BrokenScenario{{
		Name:           "auth.effective_permissions",
		Requests:       512,
		ErrorRate:      1,
		DominantStatus: 400,
		Hint:           "the request shape is wrong for this endpoint (missing or invalid parameters)",
	}}

	var out strings.Builder
	if err := report.WriteText(&out); err != nil {
		t.Fatalf("WriteText returned %v", err)
	}
	text := out.String()
	for _, want := range []string{
		"BROKEN SCENARIOS (excluded from the verdict)",
		"auth.effective_permissions",
		"HTTP 400",
		"request shape is wrong",
		// The honesty caveat about latency must be present.
		"latency percentiles do not",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("report is missing %q", want)
		}
	}
	// The section must precede the verdict it qualifies.
	if strings.Index(text, "BROKEN SCENARIOS") > strings.Index(text, "VERDICT:") {
		t.Error("broken scenarios were reported after the verdict they qualify")
	}
}

func TestWriteTextOmitsBrokenSectionWhenNoneDetected(t *testing.T) {
	var out strings.Builder
	if err := sampleReport().WriteText(&out); err != nil {
		t.Fatalf("WriteText returned %v", err)
	}
	if strings.Contains(out.String(), "BROKEN SCENARIOS") {
		t.Fatal("the broken-scenario section rendered with nothing to report")
	}
}

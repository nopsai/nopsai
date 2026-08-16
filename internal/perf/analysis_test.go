package perf

import (
	"strings"
	"testing"
	"time"
)

// stage builds a StageReport with the numbers the analysis actually reads,
// keeping the tests focused on the interpretation rather than on plumbing.
func stage(concurrency int, rps float64, p95 time.Duration, errorRate float64) StageReport {
	return StageReport{
		Concurrency: concurrency,
		Total: ScenarioStats{
			Name:       "all",
			Requests:   int64(rps * 30),
			Throughput: rps,
			ErrorRate:  errorRate,
			Latency:    LatencyStats{P50: p95 / 2, P95: p95, P99: p95 * 2, Max: p95 * 3},
		},
	}
}

func analysisConfig() Config {
	cfg := DefaultConfig()
	cfg.LatencySLO = time.Second
	cfg.ErrorBudget = 0.01
	return cfg
}

func TestAnalyzeHandlesEmptyRamp(t *testing.T) {
	analysis := Analyze(nil, analysisConfig())
	if analysis.RecommendedFound || analysis.KneeFound || analysis.FirstBreachFound {
		t.Fatalf("empty ramp produced conclusions: %+v", analysis)
	}
}

func TestAnalyzeFindsPeakThroughput(t *testing.T) {
	stages := []StageReport{
		stage(1, 50, 20*time.Millisecond, 0),
		stage(10, 400, 25*time.Millisecond, 0),
		// Throughput falls off past the peak as the system starts thrashing.
		stage(50, 380, 300*time.Millisecond, 0),
	}
	analysis := Analyze(stages, analysisConfig())
	if analysis.PeakConcurrency != 10 {
		t.Errorf("PeakConcurrency = %d, want 10", analysis.PeakConcurrency)
	}
	if analysis.PeakThroughput != 400 {
		t.Errorf("PeakThroughput = %v, want 400", analysis.PeakThroughput)
	}
}

// TestAnalyzeRecommendsHighestPassingLevel is the core "efficient number":
// the largest concurrency that still met both thresholds.
func TestAnalyzeRecommendsHighestPassingLevel(t *testing.T) {
	stages := []StageReport{
		stage(1, 50, 20*time.Millisecond, 0),
		stage(10, 400, 200*time.Millisecond, 0),
		stage(25, 450, 900*time.Millisecond, 0),
		// Breaches the p95 SLO.
		stage(50, 460, 1500*time.Millisecond, 0),
	}
	analysis := Analyze(stages, analysisConfig())
	if !analysis.RecommendedFound {
		t.Fatal("no recommendation was produced")
	}
	if analysis.Recommended != 25 {
		t.Errorf("Recommended = %d, want 25", analysis.Recommended)
	}
	if analysis.RecommendedRPS != 450 {
		t.Errorf("RecommendedRPS = %v, want 450", analysis.RecommendedRPS)
	}
}

// TestAnalyzeStopsRecommendingAfterFirstBreach guards against a later stage
// that happens to pass being recommended even though everything between it and
// the safe range failed.
func TestAnalyzeStopsRecommendingAfterFirstBreach(t *testing.T) {
	stages := []StageReport{
		stage(1, 50, 20*time.Millisecond, 0),
		// Breach.
		stage(10, 100, 2*time.Second, 0),
		// A "lucky" stage that meets the thresholds again.
		stage(25, 120, 100*time.Millisecond, 0),
	}
	analysis := Analyze(stages, analysisConfig())
	if analysis.Recommended != 1 {
		t.Fatalf("Recommended = %d, want 1: levels past the first breach must not be recommended", analysis.Recommended)
	}
}

func TestAnalyzeReportsNoSafePointWhenEvenTheLightestStageFails(t *testing.T) {
	stages := []StageReport{stage(1, 5, 3*time.Second, 0)}
	analysis := Analyze(stages, analysisConfig())
	if analysis.RecommendedFound {
		t.Fatal("a failing first stage must not yield a recommendation")
	}
	if !containsFinding(analysis.Findings, "No concurrency level met the thresholds") {
		t.Fatalf("findings did not explain the failure: %v", analysis.Findings)
	}
}

// TestAnalyzeDetectsKnee covers the saturation signature: throughput flattens
// while latency keeps climbing.
func TestAnalyzeDetectsKnee(t *testing.T) {
	stages := []StageReport{
		stage(1, 100, 10*time.Millisecond, 0),
		stage(10, 500, 20*time.Millisecond, 0),
		// Throughput gains 2% while p95 triples: the queue is absorbing load.
		stage(25, 510, 60*time.Millisecond, 0),
	}
	analysis := Analyze(stages, analysisConfig())
	if !analysis.KneeFound {
		t.Fatal("the knee was not detected")
	}
	if analysis.Knee != 10 {
		t.Errorf("Knee = %d, want 10 (the last level that still scaled)", analysis.Knee)
	}
}

func TestAnalyzeReportsNoKneeWhenThroughputKeepsScaling(t *testing.T) {
	stages := []StageReport{
		stage(1, 100, 10*time.Millisecond, 0),
		stage(10, 900, 12*time.Millisecond, 0),
		stage(25, 2000, 14*time.Millisecond, 0),
	}
	if analysis := Analyze(stages, analysisConfig()); analysis.KneeFound {
		t.Fatalf("a linearly scaling ramp should have no knee, got %d", analysis.Knee)
	}
}

func TestAnalyzeIdentifiesFirstBreachReason(t *testing.T) {
	t.Run("error budget", func(t *testing.T) {
		stages := []StageReport{
			stage(1, 50, 10*time.Millisecond, 0),
			stage(10, 100, 10*time.Millisecond, 0.25),
		}
		analysis := Analyze(stages, analysisConfig())
		if analysis.FirstBreach != 10 {
			t.Fatalf("FirstBreach = %d, want 10", analysis.FirstBreach)
		}
		if !strings.Contains(analysis.FirstBreachWhy, "error rate") {
			t.Fatalf("FirstBreachWhy = %q, want it to cite the error rate", analysis.FirstBreachWhy)
		}
	})

	t.Run("latency slo", func(t *testing.T) {
		stages := []StageReport{
			stage(1, 50, 10*time.Millisecond, 0),
			stage(10, 100, 5*time.Second, 0),
		}
		analysis := Analyze(stages, analysisConfig())
		if !strings.Contains(analysis.FirstBreachWhy, "p95") {
			t.Fatalf("FirstBreachWhy = %q, want it to cite p95", analysis.FirstBreachWhy)
		}
	})
}

func TestAnalyzeNamesBusiestServiceAndConstraintType(t *testing.T) {
	t.Run("cpu bound", func(t *testing.T) {
		stages := []StageReport{stage(10, 100, 10*time.Millisecond, 0)}
		stages[0].Resources = []ContainerUsage{
			{Container: "nopsai", CPUAvg: 190, CPUPeak: 240, MemPeakBytes: 300 << 20, MemPeakPct: 20},
			{Container: "nopsai-db", CPUAvg: 40, CPUPeak: 60, MemPeakBytes: 100 << 20, MemPeakPct: 10},
		}
		analysis := Analyze(stages, analysisConfig())
		if !strings.Contains(analysis.Bottleneck, "nopsai") {
			t.Fatalf("Bottleneck = %q, want it to name the busiest container", analysis.Bottleneck)
		}
		if !strings.Contains(analysis.Bottleneck, "CPU constraint") {
			t.Fatalf("Bottleneck = %q, want it to identify a CPU constraint", analysis.Bottleneck)
		}
	})

	t.Run("memory pressure takes priority", func(t *testing.T) {
		stages := []StageReport{stage(10, 100, 10*time.Millisecond, 0)}
		stages[0].Resources = []ContainerUsage{
			{Container: "nopsai", CPUAvg: 190, MemPeakPct: 20},
			{Container: "nopsai-db", CPUAvg: 40, MemPeakPct: 95},
		}
		analysis := Analyze(stages, analysisConfig())
		if !strings.Contains(analysis.Bottleneck, "OOM-killed") {
			t.Fatalf("Bottleneck = %q, want memory pressure to be called out first", analysis.Bottleneck)
		}
	})

	t.Run("neither cpu nor memory bound", func(t *testing.T) {
		stages := []StageReport{stage(10, 100, 10*time.Millisecond, 0)}
		stages[0].Resources = []ContainerUsage{{Container: "nopsai", CPUAvg: 15, MemPeakPct: 20}}
		analysis := Analyze(stages, analysisConfig())
		if !strings.Contains(analysis.Bottleneck, "I/O, database or lock contention") {
			t.Fatalf("Bottleneck = %q, want it to point away from CPU", analysis.Bottleneck)
		}
	})
}

func TestAnalyzeNamesSlowestEndpoint(t *testing.T) {
	stages := []StageReport{stage(10, 100, 10*time.Millisecond, 0)}
	stages[0].Scenarios = []ScenarioStats{
		{Name: "health.livez", Requests: 100, Latency: LatencyStats{P95: 2 * time.Millisecond}},
		{Name: "monitoring.summary", Requests: 100, Latency: LatencyStats{P95: 800 * time.Millisecond}},
	}
	analysis := Analyze(stages, analysisConfig())
	if !strings.Contains(analysis.SlowestPath, "monitoring.summary") {
		t.Fatalf("SlowestPath = %q, want the slowest endpoint", analysis.SlowestPath)
	}
}

// brokenRamp builds a ramp where one scenario always fails and one always
// succeeds, which is the shape a misconfigured endpoint produces.
func brokenRamp() []StageReport {
	stages := []StageReport{
		stage(1, 100, 20*time.Millisecond, 0),
		stage(10, 400, 30*time.Millisecond, 0),
	}
	for i := range stages {
		healthy := stages[i].Total.Requests * 8 / 10
		broken := stages[i].Total.Requests - healthy
		stages[i].Measured = 30 * time.Second
		stages[i].Scenarios = []ScenarioStats{
			{
				Name: "runs.list", Requests: healthy, Failures: 0, ErrorRate: 0,
				Latency: LatencyStats{P95: 30 * time.Millisecond},
			},
			{
				Name: "auth.broken", Requests: broken, Failures: broken, ErrorRate: 1,
				StatusCodes: map[int]int64{400: broken},
				Latency:     LatencyStats{P95: 3 * time.Millisecond},
			},
		}
		// The stage total carries the constant error floor the broken scenario
		// contributes.
		stages[i].Total.Failures = broken
		stages[i].Total.ErrorRate = float64(broken) / float64(stages[i].Total.Requests)
		stages[i].Total.StatusCodes = map[int]int64{200: healthy, 400: broken}
	}
	return stages
}

// TestDetectBrokenScenariosFindsLoadIndependentFailures is the guard for the
// central rule: a scenario that fails identically at every level is a
// configuration defect and must not be read as saturation.
func TestDetectBrokenScenariosFindsLoadIndependentFailures(t *testing.T) {
	broken := DetectBrokenScenarios(brokenRamp())
	if len(broken) != 1 {
		t.Fatalf("detected %d broken scenarios, want 1: %+v", len(broken), broken)
	}
	if broken[0].Name != "auth.broken" {
		t.Errorf("Name = %q", broken[0].Name)
	}
	if broken[0].DominantStatus != 400 {
		t.Errorf("DominantStatus = %d, want 400", broken[0].DominantStatus)
	}
	if !strings.Contains(broken[0].Hint, "request shape is wrong") {
		t.Errorf("Hint = %q, want it to name the cause", broken[0].Hint)
	}
}

// TestDetectBrokenScenariosIgnoresLoadInducedFailures is the counterpart: a
// scenario that only collapses under load is exactly what the ramp exists to
// find, so it must never be excused as broken.
func TestDetectBrokenScenariosIgnoresLoadInducedFailures(t *testing.T) {
	stages := []StageReport{
		stage(1, 100, 20*time.Millisecond, 0),
		stage(50, 400, 30*time.Millisecond, 0),
	}
	stages[0].Scenarios = []ScenarioStats{
		{Name: "runs.list", Requests: 100, Failures: 0, ErrorRate: 0},
	}
	stages[1].Scenarios = []ScenarioStats{
		{Name: "runs.list", Requests: 100, Failures: 100, ErrorRate: 1,
			StatusCodes: map[int]int64{503: 100}},
	}
	if broken := DetectBrokenScenarios(stages); len(broken) != 0 {
		t.Fatalf("a scenario that only fails under load was marked broken: %+v", broken)
	}
}

// TestAnalyzeExcludesBrokenScenariosFromTheVerdict is the regression guard for
// the real-world run where two misconfigured scenarios put an 11.8% error floor
// under every stage and made a healthy backend report as saturated at
// concurrency 1.
func TestAnalyzeExcludesBrokenScenariosFromTheVerdict(t *testing.T) {
	stages := brokenRamp()
	analysis := Analyze(stages, analysisConfig())

	if len(analysis.Broken) != 1 {
		t.Fatalf("the analysis did not record the broken scenario: %+v", analysis.Broken)
	}
	// The raw totals breach the 1% budget; the healthy traffic does not.
	if stages[0].Total.ErrorRate <= analysisConfig().ErrorBudget {
		t.Fatal("the fixture no longer reproduces a constant error floor")
	}
	if stages[0].EffectiveErrorRate != 0 {
		t.Errorf("EffectiveErrorRate = %v, want 0 once the broken scenario is excluded", stages[0].EffectiveErrorRate)
	}
	if stages[0].Saturated {
		t.Error("a stage was marked saturated purely because of a broken scenario")
	}
	if !analysis.RecommendedFound {
		t.Fatal("no safe operating point was found despite healthy traffic meeting the thresholds")
	}
	if analysis.Recommended != 10 {
		t.Errorf("Recommended = %d, want 10", analysis.Recommended)
	}
	if !containsFinding(analysis.Findings, "auth.broken failed every request") {
		t.Fatalf("findings did not lead with the broken scenario: %v", analysis.Findings)
	}
}

// TestAnnotateStagesFallsBackWithoutScenarioDetail keeps stage totals honest
// when no per-scenario breakdown is available.
func TestAnnotateStagesFallsBackWithoutScenarioDetail(t *testing.T) {
	stages := []StageReport{stage(1, 50, 10*time.Millisecond, 0.25)}
	AnnotateStages(stages, nil, analysisConfig())
	if stages[0].EffectiveErrorRate != 0.25 {
		t.Fatalf("EffectiveErrorRate = %v, want the raw rate as a fallback", stages[0].EffectiveErrorRate)
	}
}

func TestBrokenScenarioHintCoversCommonFailures(t *testing.T) {
	for _, testCase := range []struct {
		status int
		want   string
	}{
		{400, "request shape is wrong"},
		{401, "credentials or request signature were rejected"},
		{403, "lacks permission"},
		{404, "does not exist in the build"},
		{405, "does not accept this HTTP method"},
	} {
		if got := brokenScenarioHint(testCase.status, ""); !strings.Contains(got, testCase.want) {
			t.Errorf("brokenScenarioHint(%d) = %q, want it to mention %q", testCase.status, got, testCase.want)
		}
	}
	if got := brokenScenarioHint(0, "timeout"); !strings.Contains(got, "timeout") {
		t.Errorf("brokenScenarioHint with a transport error = %q", got)
	}
}

func TestAnalyzeReportsLatencyAmplification(t *testing.T) {
	stages := []StageReport{
		stage(1, 50, 10*time.Millisecond, 0),
		stage(100, 500, 200*time.Millisecond, 0),
	}
	analysis := Analyze(stages, analysisConfig())
	if !containsFinding(analysis.Findings, "multiplied p95 by 20.0x") {
		t.Fatalf("findings did not quantify the degradation: %v", analysis.Findings)
	}
}

// TestAnalyzeSuggestsHigherRangeWhenNoLimitFound tells the operator the test
// was too gentle rather than implying the system is unbounded.
func TestAnalyzeSuggestsHigherRangeWhenNoLimitFound(t *testing.T) {
	stages := []StageReport{
		stage(1, 50, 10*time.Millisecond, 0),
		stage(10, 500, 15*time.Millisecond, 0),
	}
	analysis := Analyze(stages, analysisConfig())
	if !containsFinding(analysis.Findings, "never found a limit") {
		t.Fatalf("findings did not suggest a wider ramp: %v", analysis.Findings)
	}
}

func TestAnalyzeSortsUnorderedStages(t *testing.T) {
	stages := []StageReport{
		stage(50, 460, 1500*time.Millisecond, 0),
		stage(1, 50, 20*time.Millisecond, 0),
		stage(10, 400, 200*time.Millisecond, 0),
	}
	analysis := Analyze(stages, analysisConfig())
	if analysis.Recommended != 10 {
		t.Fatalf("Recommended = %d, want 10: stages must be analysed in ascending order", analysis.Recommended)
	}
}

func containsFinding(findings []string, substring string) bool {
	for _, finding := range findings {
		if strings.Contains(finding, substring) {
			return true
		}
	}
	return false
}

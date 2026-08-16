package perf

import (
	"fmt"
	"sort"
	"time"
)

// Knee detection tuning. A stage is the saturation point when pushing more
// concurrency at the system stops buying throughput but keeps costing latency.
const (
	// kneeThroughputGain is the minimum relative throughput improvement a stage
	// must deliver over the previous one to count as still scaling.
	kneeThroughputGain = 0.10
	// kneeLatencyGrowth is the relative p95 increase that, combined with a
	// throughput plateau, marks the queue as the thing absorbing the load.
	kneeLatencyGrowth = 0.50
	// bottleneckCPUPercent is the per-container CPU level (100 == one full
	// core) above which a service is called out as the constraint.
	bottleneckCPUPercent = 80.0
	// bottleneckMemPercent is the share of a container's memory limit above
	// which memory is called out as the constraint.
	bottleneckMemPercent = 85.0
	// brokenScenarioErrorRate is the per-stage failure rate above which a
	// scenario is a candidate for being broken rather than overloaded.
	brokenScenarioErrorRate = 0.99
)

// BrokenScenario is a scenario that failed at essentially every request in
// every stage. A load-independent failure like that is a configuration or
// request-shape defect, not a capacity limit, and it must be reported as such
// instead of being folded into the saturation verdict.
type BrokenScenario struct {
	Name           string  `json:"name"`
	Requests       int64   `json:"requests"`
	ErrorRate      float64 `json:"error_rate"`
	DominantStatus int     `json:"dominant_status,omitempty"`
	DominantError  string  `json:"dominant_error,omitempty"`
	Hint           string  `json:"hint,omitempty"`
}

// DetectBrokenScenarios returns the scenarios that failed at every concurrency
// level. The test is deliberately strict: a scenario has to fail almost every
// request in every stage it ran in. A scenario that only fails under load is
// exactly what the ramp exists to find, and must not be excused this way.
func DetectBrokenScenarios(stages []StageReport) []BrokenScenario {
	type accumulator struct {
		requests  int64
		failures  int64
		statuses  map[int]int64
		errors    map[string]int64
		stages    int
		badStages int
	}
	totals := make(map[string]*accumulator)

	for _, stage := range stages {
		for _, scenario := range stage.Scenarios {
			if scenario.Requests == 0 {
				continue
			}
			entry, ok := totals[scenario.Name]
			if !ok {
				entry = &accumulator{statuses: make(map[int]int64), errors: make(map[string]int64)}
				totals[scenario.Name] = entry
			}
			entry.requests += scenario.Requests
			entry.failures += scenario.Failures
			entry.stages++
			if scenario.ErrorRate >= brokenScenarioErrorRate {
				entry.badStages++
			}
			for code, count := range scenario.StatusCodes {
				entry.statuses[code] += count
			}
			for message, count := range scenario.Errors {
				entry.errors[message] += count
			}
		}
	}

	broken := make([]BrokenScenario, 0)
	for name, entry := range totals {
		if entry.stages == 0 || entry.badStages != entry.stages {
			continue
		}
		scenario := BrokenScenario{
			Name:      name,
			Requests:  entry.requests,
			ErrorRate: float64(entry.failures) / float64(entry.requests),
		}
		scenario.DominantStatus = dominantStatus(entry.statuses)
		scenario.DominantError = dominantError(entry.errors)
		scenario.Hint = brokenScenarioHint(scenario.DominantStatus, scenario.DominantError)
		broken = append(broken, scenario)
	}
	sort.Slice(broken, func(i, j int) bool { return broken[i].Name < broken[j].Name })
	return broken
}

// dominantStatus returns the most frequent failing HTTP status, ignoring
// successful responses so a mostly-broken scenario is described by its failure.
func dominantStatus(statuses map[int]int64) int {
	var best int
	var bestCount int64
	for code, count := range statuses {
		if code < 400 {
			continue
		}
		if count > bestCount || (count == bestCount && code < best) {
			best, bestCount = code, count
		}
	}
	return best
}

func dominantError(errors map[string]int64) string {
	var best string
	var bestCount int64
	for message, count := range errors {
		if count > bestCount || (count == bestCount && message < best) {
			best, bestCount = message, count
		}
	}
	return best
}

// brokenScenarioHint translates a failure mode into the thing an operator has
// to go and fix.
func brokenScenarioHint(status int, transportError string) string {
	switch status {
	case 400:
		return "the request shape is wrong for this endpoint (missing or invalid parameters)"
	case 401:
		return "the credentials or request signature were rejected"
	case 403:
		return "the account running the test lacks permission for this endpoint"
	case 404:
		return "this endpoint does not exist in the build under test"
	case 405:
		return "the endpoint does not accept this HTTP method"
	case 501:
		return "the endpoint is not implemented in this build"
	}
	if transportError != "" {
		return "every request failed with " + transportError
	}
	return "this scenario never succeeded, so it measures nothing"
}

// AnnotateStages fills in each stage's load-induced error rate and saturation
// verdict, excluding scenarios that were broken for the whole run. Without this
// step a single misconfigured endpoint would put a constant error floor under
// every stage and make a healthy system look saturated from the first level.
func AnnotateStages(stages []StageReport, broken []BrokenScenario, cfg Config) {
	excluded := make(map[string]struct{}, len(broken))
	for _, scenario := range broken {
		excluded[scenario.Name] = struct{}{}
	}

	for i := range stages {
		stage := &stages[i]
		stage.BrokenRequests = 0
		var requests, failures int64
		for _, scenario := range stage.Scenarios {
			if _, skip := excluded[scenario.Name]; skip {
				stage.BrokenRequests += scenario.Requests
				continue
			}
			requests += scenario.Requests
			failures += scenario.Failures
		}
		// With no scenario breakdown available, or with every scenario
		// excluded, fall back to the raw rate rather than reporting a
		// misleading zero.
		if requests == 0 {
			stage.EffectiveErrorRate = stage.Total.ErrorRate
			stage.EffectiveThroughput = stage.Total.Throughput
		} else {
			stage.EffectiveErrorRate = float64(failures) / float64(requests)
			if stage.Measured > 0 {
				stage.EffectiveThroughput = float64(requests) / stage.Measured.Seconds()
			}
		}
		stage.Saturated = stage.EffectiveErrorRate > cfg.ErrorBudget || stage.Total.Latency.P95 > cfg.LatencySLO
	}
}

// Analysis is the interpreted verdict over a ramp: the numbers an operator
// needs in order to size the deployment.
type Analysis struct {
	// PeakThroughput is the highest sustained request rate observed and the
	// concurrency that produced it.
	PeakThroughput    float64 `json:"peak_throughput_rps"`
	PeakConcurrency   int     `json:"peak_concurrency"`
	PeakThroughputP95 string  `json:"peak_throughput_p95"`
	// Recommended is the highest concurrency that still met both the latency
	// SLO and the error budget. This is the safe operating point.
	Recommended      int     `json:"recommended_concurrency"`
	RecommendedRPS   float64 `json:"recommended_throughput_rps"`
	RecommendedP95   string  `json:"recommended_p95"`
	RecommendedFound bool    `json:"recommended_found"`
	// Knee is the concurrency at which added load stopped producing added
	// throughput. Beyond it, requests queue rather than complete faster.
	Knee      int  `json:"knee_concurrency"`
	KneeFound bool `json:"knee_found"`
	// FirstBreach is the first concurrency that violated the SLO or budget.
	FirstBreach      int    `json:"first_breach_concurrency"`
	FirstBreachFound bool   `json:"first_breach_found"`
	FirstBreachWhy   string `json:"first_breach_reason,omitempty"`

	Bottleneck  string   `json:"bottleneck,omitempty"`
	SlowestPath string   `json:"slowest_endpoint,omitempty"`
	Findings    []string `json:"findings,omitempty"`

	// Broken lists scenarios that failed for the entire run. They are excluded
	// from the saturation verdict and reported separately, because they
	// describe a configuration defect rather than a capacity limit.
	Broken []BrokenScenario `json:"broken_scenarios,omitempty"`
}

// Analyze interprets a completed ramp against the configured thresholds.
// Analyze annotates the stages in place with their load-induced error rates and
// saturation verdicts, then interprets the ramp against the configured
// thresholds.
func Analyze(stages []StageReport, cfg Config) Analysis {
	analysis := Analysis{}
	if len(stages) == 0 {
		return analysis
	}
	analysis.Broken = DetectBrokenScenarios(stages)
	AnnotateStages(stages, analysis.Broken, cfg)

	ordered := make([]StageReport, len(stages))
	copy(ordered, stages)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Concurrency < ordered[j].Concurrency })

	analysis.findPeak(ordered)
	analysis.findRecommended(ordered, cfg)
	analysis.findKnee(ordered)
	analysis.findFirstBreach(ordered, cfg)
	analysis.Bottleneck = describeBottleneck(ordered)
	analysis.SlowestPath = describeSlowestScenario(ordered)
	analysis.Findings = buildFindings(ordered, cfg, analysis)
	return analysis
}

func (a *Analysis) findPeak(stages []StageReport) {
	for _, stage := range stages {
		if stage.EffectiveThroughput > a.PeakThroughput {
			a.PeakThroughput = stage.EffectiveThroughput
			a.PeakConcurrency = stage.Concurrency
			a.PeakThroughputP95 = stage.Total.Latency.P95.Round(time.Millisecond).String()
		}
	}
}

// findRecommended picks the highest concurrency that satisfied both thresholds.
// Higher levels are only considered if every level up to them also passed, so a
// single lucky stage past the breach point cannot be recommended.
func (a *Analysis) findRecommended(stages []StageReport, cfg Config) {
	for _, stage := range stages {
		if stage.Total.Requests == 0 {
			continue
		}
		if stage.EffectiveErrorRate > cfg.ErrorBudget || stage.Total.Latency.P95 > cfg.LatencySLO {
			break
		}
		a.Recommended = stage.Concurrency
		a.RecommendedRPS = stage.EffectiveThroughput
		a.RecommendedP95 = stage.Total.Latency.P95.Round(time.Millisecond).String()
		a.RecommendedFound = true
	}
}

// findKnee locates the first stage where throughput plateaued while latency
// kept climbing, which is the point the system stops converting concurrency
// into work.
func (a *Analysis) findKnee(stages []StageReport) {
	for i := 1; i < len(stages); i++ {
		previous, current := stages[i-1], stages[i]
		if previous.EffectiveThroughput <= 0 || previous.Total.Latency.P95 <= 0 {
			continue
		}
		throughputGain := (current.EffectiveThroughput - previous.EffectiveThroughput) / previous.EffectiveThroughput
		latencyGrowth := float64(current.Total.Latency.P95-previous.Total.Latency.P95) / float64(previous.Total.Latency.P95)
		if throughputGain < kneeThroughputGain && latencyGrowth > kneeLatencyGrowth {
			a.Knee = previous.Concurrency
			a.KneeFound = true
			return
		}
	}
}

func (a *Analysis) findFirstBreach(stages []StageReport, cfg Config) {
	for _, stage := range stages {
		if stage.Total.Requests == 0 {
			continue
		}
		switch {
		case stage.EffectiveErrorRate > cfg.ErrorBudget:
			a.FirstBreach = stage.Concurrency
			a.FirstBreachFound = true
			a.FirstBreachWhy = fmt.Sprintf("error rate %.2f%% exceeded the %.2f%% budget",
				stage.EffectiveErrorRate*100, cfg.ErrorBudget*100)
			return
		case stage.Total.Latency.P95 > cfg.LatencySLO:
			a.FirstBreach = stage.Concurrency
			a.FirstBreachFound = true
			a.FirstBreachWhy = fmt.Sprintf("p95 %s exceeded the %s SLO",
				stage.Total.Latency.P95.Round(time.Millisecond), cfg.LatencySLO)
			return
		}
	}
}

// describeBottleneck names the service that was working hardest at the highest
// concurrency reached, which is the first place to look when raising capacity.
func describeBottleneck(stages []StageReport) string {
	last := lastStageWithResources(stages)
	if last == nil || len(last.Resources) == 0 {
		return ""
	}
	// Resources are already sorted by descending average CPU.
	top := last.Resources[0]
	description := fmt.Sprintf("%s at concurrency %d: CPU avg %.0f%% (peak %.0f%%), memory %s",
		top.Container, last.Concurrency, top.CPUAvg, top.CPUPeak, FormatBytes(top.MemPeakBytes))

	// Memory pressure is called out ahead of CPU because a container near its
	// limit is at risk of being killed, which is a harder failure than being
	// merely slow.
	for _, usage := range last.Resources {
		if usage.MemPeakPct >= bottleneckMemPercent {
			return description + fmt.Sprintf("; %s reached %.0f%% of its memory limit and is at risk of being OOM-killed before it is CPU-bound",
				usage.Container, usage.MemPeakPct)
		}
	}
	if top.CPUAvg >= bottleneckCPUPercent {
		return description + "; this service is the CPU constraint, so give it more cores or replicas first"
	}
	return description + "; no service was CPU-bound, so the limit is more likely I/O, database or lock contention"
}

func lastStageWithResources(stages []StageReport) *StageReport {
	for i := len(stages) - 1; i >= 0; i-- {
		if len(stages[i].Resources) > 0 {
			return &stages[i]
		}
	}
	return nil
}

// describeSlowestScenario names the endpoint with the worst p95 at the highest
// concurrency reached.
func describeSlowestScenario(stages []StageReport) string {
	if len(stages) == 0 {
		return ""
	}
	last := stages[len(stages)-1]
	var worst ScenarioStats
	for _, scenario := range last.Scenarios {
		if scenario.Requests > 0 && scenario.Latency.P95 > worst.Latency.P95 {
			worst = scenario
		}
	}
	if worst.Name == "" {
		return ""
	}
	return fmt.Sprintf("%s (p95 %s at concurrency %d)",
		worst.Name, worst.Latency.P95.Round(time.Millisecond), last.Concurrency)
}

// buildFindings turns the ramp into short operator-facing statements. Each one
// is phrased as an observation with its supporting number so the report can be
// read without re-deriving anything.
func buildFindings(stages []StageReport, cfg Config, analysis Analysis) []string {
	var findings []string

	// Broken scenarios come first: until they are fixed, every other number in
	// the report is computed over a smaller workload than was requested.
	for _, scenario := range analysis.Broken {
		detail := scenario.DominantError
		if scenario.DominantStatus != 0 {
			detail = fmt.Sprintf("HTTP %d", scenario.DominantStatus)
		}
		findings = append(findings, fmt.Sprintf(
			"%s failed every request at every concurrency level (%d requests, %s): %s. It is excluded from the saturation verdict.",
			scenario.Name, scenario.Requests, detail, scenario.Hint))
	}

	if !analysis.RecommendedFound {
		findings = append(findings, fmt.Sprintf(
			"No concurrency level met the thresholds: even %d concurrent clients breached the %s p95 SLO or the %.2f%% error budget.",
			stages[0].Concurrency, cfg.LatencySLO, cfg.ErrorBudget*100))
	} else if analysis.Recommended == stages[len(stages)-1].Concurrency {
		findings = append(findings, fmt.Sprintf(
			"The ramp never found a limit: the highest level tested (%d concurrent clients, %.1f req/s) still met the SLO. Re-run with higher concurrency to locate the ceiling.",
			analysis.Recommended, analysis.RecommendedRPS))
	}

	if analysis.KneeFound {
		findings = append(findings, fmt.Sprintf(
			"Throughput stops scaling past %d concurrent clients; beyond that, extra load turns into queue time rather than completed work.",
			analysis.Knee))
	}

	// Latency amplification between the lightest and heaviest stage tells the
	// operator how gracefully the system degrades.
	first, last := stages[0], stages[len(stages)-1]
	if first.Total.Latency.P95 > 0 && last.Concurrency > first.Concurrency {
		amplification := float64(last.Total.Latency.P95) / float64(first.Total.Latency.P95)
		concurrencyRatio := float64(last.Concurrency) / float64(first.Concurrency)
		findings = append(findings, fmt.Sprintf(
			"Going from %d to %d concurrent clients (%.0fx load) multiplied p95 by %.1fx (%s to %s).",
			first.Concurrency, last.Concurrency, concurrencyRatio, amplification,
			first.Total.Latency.P95.Round(time.Millisecond), last.Total.Latency.P95.Round(time.Millisecond)))
	}

	// Surface any transport error class that actually occurred, since a
	// saturation number means something different when the failures are
	// timeouts versus refusals.
	for _, stage := range stages {
		if len(stage.Total.Errors) == 0 || stage.EffectiveErrorRate == 0 {
			continue
		}
		for message, count := range stage.Total.Errors {
			findings = append(findings, fmt.Sprintf(
				"At concurrency %d, %d request(s) failed with %q.", stage.Concurrency, count, message))
		}
		break
	}

	return findings
}

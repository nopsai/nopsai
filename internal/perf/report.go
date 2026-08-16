package perf

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
)

// Report is the complete result of a performance test run.
type Report struct {
	StartedAt time.Time     `json:"started_at"`
	EndedAt   time.Time     `json:"ended_at"`
	Duration  time.Duration `json:"duration"`
	Target    string        `json:"target"`
	Suites    []string      `json:"suites"`
	Version   string        `json:"platform_version,omitempty"`

	Thresholds Thresholds `json:"thresholds"`

	Stages          []StageReport         `json:"stages,omitempty"`
	ServiceCapacity []ServiceCapacity     `json:"service_capacity,omitempty"`
	PipelineStages  []PipelineStageReport `json:"pipeline_stages,omitempty"`
	Analysis        Analysis              `json:"analysis"`

	SamplingErrors []string `json:"sampling_errors,omitempty"`
}

// Thresholds records the pass criteria a run was judged against so a stored
// report stays interpretable later.
type Thresholds struct {
	LatencySLO  string  `json:"latency_slo"`
	ErrorBudget float64 `json:"error_budget"`
}

// WriteJSON serializes the report.
func (r *Report) WriteJSON(w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}

// WriteText renders the human-readable report: the ramp table first, then the
// per-service resource cost, then the interpreted verdict.
func (r *Report) WriteText(w io.Writer) error {
	var out strings.Builder

	section(&out, "NOPSAI BACKEND PERFORMANCE REPORT")
	fmt.Fprintf(&out, "Target:     %s\n", r.Target)
	fmt.Fprintf(&out, "Suites:     %s\n", strings.Join(r.Suites, ", "))
	if r.Version != "" {
		fmt.Fprintf(&out, "Version:    %s\n", r.Version)
	}
	fmt.Fprintf(&out, "Started:    %s\n", r.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(&out, "Duration:   %s\n", r.Duration.Round(time.Second))
	fmt.Fprintf(&out, "Thresholds: p95 <= %s, errors <= %.2f%%\n", r.Thresholds.LatencySLO, r.Thresholds.ErrorBudget*100)

	if len(r.Stages) > 0 {
		r.writeRampTable(&out)
		r.writeScenarioTable(&out)
		r.writeServiceCapacity(&out)
		r.writeResourceTable(&out)
	}
	if len(r.PipelineStages) > 0 {
		r.writePipelineTable(&out)
	}
	r.writeBrokenScenarios(&out)
	r.writeVerdict(&out)

	if len(r.SamplingErrors) > 0 {
		section(&out, "RESOURCE SAMPLING WARNINGS")
		for _, message := range r.SamplingErrors {
			fmt.Fprintf(&out, "  - %s\n", message)
		}
	}

	_, err := io.WriteString(w, out.String())
	return err
}

// writeRampTable is the core output: one row per concurrency level showing how
// throughput and latency moved together.
func (r *Report) writeRampTable(out *strings.Builder) {
	section(out, "LOAD RAMP (per concurrency level)")
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "CONC\tREQS\tRPS\tMEAN\tP50\tP90\tP95\tP99\tMAX\tERR%\tSTATUS")
	for _, stage := range r.Stages {
		status := "ok"
		if stage.Saturated {
			status = "SATURATED"
		}
		requests := stage.Total.Requests - stage.BrokenRequests
		fmt.Fprintf(table, "%d\t%d\t%.1f\t%s\t%s\t%s\t%s\t%s\t%s\t%.2f\t%s\n",
			stage.Concurrency,
			requests,
			stage.EffectiveThroughput,
			short(stage.Total.Latency.Mean),
			short(stage.Total.Latency.P50),
			short(stage.Total.Latency.P90),
			short(stage.Total.Latency.P95),
			short(stage.Total.Latency.P99),
			short(stage.Total.Latency.Max),
			stage.EffectiveErrorRate*100,
			status,
		)
	}
	_ = table.Flush()
	fmt.Fprintln(out, "\nRPS is achieved throughput, not offered load: workers issue requests back to back,")
	fmt.Fprintln(out, "so the rate shown is what the system was able to complete at that concurrency.")
	if r.hasBrokenScenarios() {
		fmt.Fprintln(out, "REQS, RPS and ERR% exclude the broken scenarios listed below, which failed")
		fmt.Fprintln(out, "identically at every level and therefore say nothing about capacity.")
	}
}

// writeScenarioTable breaks the heaviest stage down per endpoint, which is what
// identifies the specific query that degrades first.
func (r *Report) writeScenarioTable(out *strings.Builder) {
	last := r.Stages[len(r.Stages)-1]
	section(out, fmt.Sprintf("PER-ENDPOINT BREAKDOWN (at concurrency %d)", last.Concurrency))
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ENDPOINT\tREQS\tRPS\tP50\tP95\tP99\tMAX\tERR%")
	for _, scenario := range last.Scenarios {
		fmt.Fprintf(table, "%s\t%d\t%.1f\t%s\t%s\t%s\t%s\t%.2f\n",
			scenario.Name,
			scenario.Requests,
			scenario.Throughput,
			short(scenario.Latency.P50),
			short(scenario.Latency.P95),
			short(scenario.Latency.P99),
			short(scenario.Latency.Max),
			scenario.ErrorRate*100,
		)
	}
	_ = table.Flush()
}

// writeServiceCapacity answers the comparison question directly: under the same
// pressure, which service carried the most work, which degraded first, and how
// much each one cost to run.
func (r *Report) writeServiceCapacity(out *strings.Builder) {
	if len(r.ServiceCapacity) == 0 {
		return
	}
	section(out, "SERVICE CAPACITY (per service, across the ramp)")
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "SERVICE\tREQS\tPEAK RPS\tAT CONC\tBASE P95\tPEAK P95\tP95 GROWTH\tWORST ERR%\tCPU@PEAK\tVERDICT")
	for _, capacity := range r.ServiceCapacity {
		verdict := "held"
		if capacity.Breached {
			verdict = fmt.Sprintf("broke at %d", capacity.BreachConcurrency)
		}
		cpu := "-"
		if capacity.CPUAvgAtPeak > 0 {
			cpu = fmt.Sprintf("%.0f%%", capacity.CPUAvgAtPeak)
		}
		fmt.Fprintf(table, "%s\t%d\t%.1f\t%d\t%s\t%s\t%.1fx\t%.2f\t%s\t%s\n",
			capacity.Service,
			capacity.TotalRequests,
			capacity.PeakThroughput,
			capacity.PeakConcurrency,
			short(capacity.BaselineP95),
			short(capacity.PeakP95),
			capacity.LatencyGrowth,
			capacity.WorstErrorRate*100,
			cpu,
			verdict,
		)
	}
	_ = table.Flush()

	fmt.Fprintln(out, "\nP95 GROWTH is peak p95 over the lightest stage's p95: a service that carries load")
	fmt.Fprintln(out, "well keeps this near 1x. Per-service p95 is the worst of that service's endpoints,")
	fmt.Fprintln(out, "so it never claims a service was faster than one of its calls actually was.")

	if weakest, ok := WeakestService(r.ServiceCapacity); ok {
		fmt.Fprintf(out, "\n  Gave out first : %s at %d concurrent clients (%.1f req/s, p95 %s). Scale this first.\n",
			weakest.Service, weakest.BreachConcurrency, weakest.PeakThroughput, short(weakest.PeakP95))
	}
	if busiest, ok := BusiestService(r.ServiceCapacity); ok {
		fmt.Fprintf(out, "  Carried most   : %s at %.1f req/s peak (p95 %s, growth %.1fx).\n",
			busiest.Service, busiest.PeakThroughput, short(busiest.PeakP95), busiest.LatencyGrowth)
	}
	if resilient, ok := MostResilientService(r.ServiceCapacity); ok {
		fmt.Fprintf(out, "  Degraded least : %s held p95 at %s across the ramp (growth %.1fx, %.1f req/s peak).\n",
			resilient.Service, short(resilient.PeakP95), resilient.LatencyGrowth, resilient.PeakThroughput)
	}
	fmt.Fprintln(out, "\nPostgres is not listed as a request target because every call reaches it; its")
	fmt.Fprintln(out, "cost shows up as nopsai-db in the resource table below.")
}

// writeResourceTable shows what each service cost at each level, so the report
// answers "which container do I need to give more CPU" and not just "it is slow".
func (r *Report) writeResourceTable(out *strings.Builder) {
	if !r.hasResources() {
		return
	}
	section(out, "SERVICE RESOURCE USAGE (per concurrency level)")
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "CONC\tSERVICE\tCPU AVG%\tCPU PEAK%\tMEM AVG\tMEM PEAK\tMEM LIMIT\tMEM PEAK%")
	for _, stage := range r.Stages {
		for _, usage := range stage.Resources {
			fmt.Fprintf(table, "%d\t%s\t%.1f\t%.1f\t%s\t%s\t%s\t%.1f\n",
				stage.Concurrency,
				usage.Container,
				usage.CPUAvg,
				usage.CPUPeak,
				FormatBytes(usage.MemAvgBytes),
				FormatBytes(usage.MemPeakBytes),
				FormatBytes(usage.MemLimitBytes),
				usage.MemPeakPct,
			)
		}
	}
	_ = table.Flush()
	fmt.Fprintln(out, "\nCPU% is Docker's scale where 100% is one fully saturated core, so a service on a")
	fmt.Fprintln(out, "4-core host can legitimately reach 400%.")
}

// writeBrokenScenarios is deliberately placed before the verdict: an operator
// has to know part of the workload never ran before reading capacity numbers
// derived from the rest of it.
func (r *Report) writeBrokenScenarios(out *strings.Builder) {
	if !r.hasBrokenScenarios() {
		return
	}
	section(out, "BROKEN SCENARIOS (excluded from the verdict)")
	fmt.Fprintln(out, "These failed at every concurrency level, so they measure configuration, not capacity.")
	fmt.Fprintln(out)
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "SCENARIO\tREQS\tFAILURE\tLIKELY CAUSE")
	for _, scenario := range r.Analysis.Broken {
		failure := scenario.DominantError
		if scenario.DominantStatus != 0 {
			failure = fmt.Sprintf("HTTP %d", scenario.DominantStatus)
		}
		fmt.Fprintf(table, "%s\t%d\t%s\t%s\n", scenario.Name, scenario.Requests, failure, scenario.Hint)
	}
	_ = table.Flush()
	fmt.Fprintln(out, "\nRequest counts, throughput and error rates above exclude these scenarios, but the")
	fmt.Fprintln(out, "latency percentiles do not: a rejected request usually returns in well under a")
	fmt.Fprintln(out, "millisecond, so it pulls the reported percentiles DOWN. Treat the latency numbers")
	fmt.Fprintln(out, "as optimistic and re-run once these scenarios are fixed.")
}

func (r *Report) hasBrokenScenarios() bool {
	return len(r.Analysis.Broken) > 0
}

func (r *Report) hasResources() bool {
	for _, stage := range r.Stages {
		if len(stage.Resources) > 0 {
			return true
		}
	}
	return false
}

// writePipelineTable reports the end-to-end suite, whose units are runs and
// minutes rather than requests and milliseconds.
func (r *Report) writePipelineTable(out *strings.Builder) {
	section(out, "END-TO-END PIPELINE EXECUTION")
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "CONC\tFAMILIES\tOK\tFAIL\tRUNS\tRUNS/MIN\tINGEST P95\tQUEUE P95\tFAMILY P50\tFAMILY P95")
	for _, stage := range r.PipelineStages {
		fmt.Fprintf(table, "%d\t%d\t%d\t%d\t%d\t%.2f\t%s\t%s\t%s\t%s\n",
			stage.Concurrency,
			stage.Families,
			stage.Succeeded,
			stage.Failed,
			stage.TotalRuns,
			stage.RunsPerMinute,
			short(stage.IngestLatency.P95),
			short(stage.VisibleLatency.P95),
			short(stage.FamilyDuration.P50),
			short(stage.FamilyDuration.P95),
		)
	}
	_ = table.Flush()
	fmt.Fprintln(out, "\nINGEST is the webhook POST round trip. QUEUE is the delay from an accepted webhook")
	fmt.Fprintln(out, "to the first run being visible through the API. FAMILY spans trigger to last run done.")

	r.writePipelineFailures(out)
}

// writePipelineFailures lists why families failed, because a failed end-to-end
// run is far more often a configuration gap than a capacity limit.
func (r *Report) writePipelineFailures(out *strings.Builder) {
	reasons := make(map[string]int)
	for _, stage := range r.PipelineStages {
		for _, family := range stage.FamilyResults {
			if family.Err != "" {
				reasons[family.Err]++
			}
		}
	}
	if len(reasons) == 0 {
		return
	}
	fmt.Fprintln(out, "\nPipeline failure reasons:")
	for reason, count := range reasons {
		fmt.Fprintf(out, "  %3d x %s\n", count, reason)
	}
}

// writeVerdict is the section that answers "what are the efficient numbers".
func (r *Report) writeVerdict(out *strings.Builder) {
	analysis := r.Analysis
	section(out, "VERDICT: EFFICIENT OPERATING NUMBERS")

	if analysis.RecommendedFound {
		fmt.Fprintf(out, "  Safe operating point   : %d concurrent clients, %.1f req/s sustained, p95 %s\n",
			analysis.Recommended, analysis.RecommendedRPS, analysis.RecommendedP95)
	} else {
		fmt.Fprintln(out, "  Safe operating point   : none found - the lightest level tested already breached the thresholds")
	}
	fmt.Fprintf(out, "  Peak throughput        : %.1f req/s at %d concurrent clients (p95 %s)\n",
		analysis.PeakThroughput, analysis.PeakConcurrency, analysis.PeakThroughputP95)
	if analysis.KneeFound {
		fmt.Fprintf(out, "  Saturation knee        : %d concurrent clients\n", analysis.Knee)
	} else {
		fmt.Fprintln(out, "  Saturation knee        : not reached in the tested range")
	}
	if analysis.FirstBreachFound {
		fmt.Fprintf(out, "  First threshold breach : %d concurrent clients (%s)\n",
			analysis.FirstBreach, analysis.FirstBreachWhy)
	} else {
		fmt.Fprintln(out, "  First threshold breach : none in the tested range")
	}
	if analysis.Bottleneck != "" {
		fmt.Fprintf(out, "  Busiest service        : %s\n", analysis.Bottleneck)
	}
	if analysis.SlowestPath != "" {
		fmt.Fprintf(out, "  Slowest endpoint       : %s\n", analysis.SlowestPath)
	}

	if len(analysis.Findings) > 0 {
		fmt.Fprintln(out, "\n  Findings:")
		for _, finding := range analysis.Findings {
			fmt.Fprintf(out, "    - %s\n", finding)
		}
	}
}

func section(out *strings.Builder, title string) {
	fmt.Fprintf(out, "\n%s\n%s\n", title, strings.Repeat("=", len(title)))
}

// short renders a duration at a precision that stays readable across the six
// orders of magnitude a load test spans, from sub-millisecond to minutes.
func short(d time.Duration) string {
	switch {
	case d == 0:
		return "-"
	case d < time.Millisecond:
		return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond))
	case d < time.Second:
		return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond))
	case d < time.Minute:
		return fmt.Sprintf("%.2fs", d.Seconds())
	default:
		return d.Round(time.Second).String()
	}
}

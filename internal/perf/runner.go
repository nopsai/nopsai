package perf

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// StageReport is everything measured during one concurrency level.
type StageReport struct {
	Concurrency int              `json:"concurrency"`
	StartedAt   time.Time        `json:"started_at"`
	EndedAt     time.Time        `json:"ended_at"`
	Measured    time.Duration    `json:"measured_duration"`
	Total       ScenarioStats    `json:"total"`
	Scenarios   []ScenarioStats  `json:"scenarios"`
	Resources   []ContainerUsage `json:"resources,omitempty"`

	// EffectiveErrorRate is the failure rate over the scenarios that were not
	// broken for the whole run. It is the number the saturation verdict uses,
	// because a permanently misconfigured endpoint says nothing about capacity.
	EffectiveErrorRate float64 `json:"effective_error_rate"`
	// EffectiveThroughput is the request rate over those same scenarios.
	EffectiveThroughput float64 `json:"effective_throughput_rps"`
	// BrokenRequests counts requests belonging to broken scenarios.
	BrokenRequests int64 `json:"broken_requests,omitempty"`

	// Saturated marks a stage that breached the error budget or latency SLO.
	Saturated bool `json:"saturated"`
}

// Logf is the harness progress sink. It exists so the runner stays free of a
// logging dependency and tests can capture output.
type Logf func(format string, args ...any)

// Runner executes the concurrency ramp for the short-request suites.
type Runner struct {
	cfg     Config
	client  *http.Client
	mix     *Mix
	request *RequestContext
	sampler *Sampler
	logf    Logf
}

// NewRunner wires a Runner. The sampler may be nil when resource collection is
// disabled.
func NewRunner(cfg Config, client *http.Client, mix *Mix, request *RequestContext, sampler *Sampler, logf Logf) *Runner {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Runner{cfg: cfg, client: client, mix: mix, request: request, sampler: sampler, logf: logf}
}

// Run executes every configured concurrency level in ascending order and
// returns one report per stage. The ramp stops early once a stage collapses,
// because measuring levels beyond total failure adds time but no information.
func (r *Runner) Run(ctx context.Context) ([]StageReport, error) {
	if r.mix.Empty() {
		return nil, nil
	}
	levels := NormalizedConcurrency(r.cfg.Concurrency)
	reports := make([]StageReport, 0, len(levels))

	for _, concurrency := range levels {
		if ctx.Err() != nil {
			break
		}
		r.logf("stage: concurrency=%d duration=%s (warmup %s)", concurrency, r.cfg.StageDuration, r.cfg.WarmupDuration)
		report, err := r.RunStage(ctx, concurrency)
		if err != nil {
			return reports, err
		}
		reports = append(reports, report)
		r.logf("  -> %.1f req/s, p95 %s, errors %.2f%%",
			report.Total.Throughput, report.Total.Latency.P95.Round(time.Millisecond), report.Total.ErrorRate*100)

		if report.Total.ErrorRate >= r.cfg.AbortErrorRate && r.cfg.AbortErrorRate > 0 {
			r.logf("  !! error rate %.1f%% reached the abort threshold; stopping the ramp", report.Total.ErrorRate*100)
			break
		}
	}
	return reports, nil
}

// RunStage drives one concurrency level. Workers issue requests back to back
// for the whole stage, but only requests that start after the warmup window are
// recorded, so connection setup and cache warming stay out of the numbers.
func (r *Runner) RunStage(ctx context.Context, concurrency int) (StageReport, error) {
	stageCtx, cancel := context.WithTimeout(ctx, r.cfg.StageDuration)
	defer cancel()

	recorder := NewRecorder()
	startedAt := time.Now()
	measureStart := startedAt.Add(r.cfg.WarmupDuration)

	var sequence atomic.Uint64
	var wg sync.WaitGroup

	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for stageCtx.Err() == nil {
				scenario := r.mix.Pick(sequence.Add(1) - 1)
				req, err := scenario.Build(stageCtx, r.request)
				if err != nil {
					// A build failure is a harness or credential problem rather
					// than a server-side latency observation, but it still has
					// to surface in the report instead of being swallowed.
					if stageCtx.Err() == nil {
						recorder.Record(Result{Scenario: scenario.Name, Service: scenario.Service, Err: "request build failed"})
					}
					continue
				}
				issuedAt := time.Now()
				result := ExecuteRequest(r.client, scenario, req)
				if issuedAt.Before(measureStart) || stageCtx.Err() != nil {
					continue
				}
				recorder.Record(result)
			}
		}()
	}
	wg.Wait()

	endedAt := time.Now()
	measured := endedAt.Sub(measureStart)
	if measured <= 0 {
		return StageReport{}, fmt.Errorf("stage at concurrency %d produced no measured window", concurrency)
	}

	report := StageReport{
		Concurrency: concurrency,
		StartedAt:   startedAt,
		EndedAt:     endedAt,
		Measured:    measured,
		Scenarios:   recorder.Snapshot(measured),
		Total:       recorder.Overall("all", measured),
	}
	if r.sampler != nil {
		report.Resources = UsageBetween(r.sampler.Samples(), measureStart, endedAt)
	}
	report.Saturated = report.Total.ErrorRate > r.cfg.ErrorBudget || report.Total.Latency.P95 > r.cfg.LatencySLO
	return report, nil
}

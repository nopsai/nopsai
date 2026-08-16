package perf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// runListItem is the subset of the /v1/runs response the harness needs. It is
// declared locally rather than importing pkg/models so that the load harness
// stays coupled to the wire contract it exercises rather than to server
// internals; a field disappearing from the API should fail here loudly.
type runListItem struct {
	RunID          string    `json:"run_id"`
	PipelineName   string    `json:"pipeline_name"`
	Status         string    `json:"status"`
	StartedAt      time.Time `json:"started_at"`
	FinishedAt     time.Time `json:"finished_at"`
	IsComplete     bool      `json:"is_complete"`
	ParentRunID    *string   `json:"parent_run_id"`
	TriggerEventID string    `json:"trigger_event_id,omitempty"`
}

// PipelineRunObservation records one pipeline run inside a triggered family.
type PipelineRunObservation struct {
	RunID        string        `json:"run_id"`
	PipelineName string        `json:"pipeline_name"`
	Status       string        `json:"status"`
	Relationship string        `json:"relationship"`
	Duration     time.Duration `json:"duration"`
}

// PipelineFamilyResult is the end-to-end outcome of a single webhook trigger and
// every run it produced.
type PipelineFamilyResult struct {
	DeliveryID string `json:"delivery_id"`
	// IngestLatency is how long the webhook POST itself took to be accepted.
	IngestLatency time.Duration `json:"ingest_latency"`
	// VisibleLatency is the queue delay between the accepted webhook and the
	// first run becoming visible through the API.
	VisibleLatency time.Duration `json:"visible_latency"`
	// TotalDuration spans the trigger through the last run in the family
	// reaching a terminal state.
	TotalDuration time.Duration            `json:"total_duration"`
	Runs          []PipelineRunObservation `json:"runs"`
	Succeeded     bool                     `json:"succeeded"`
	Err           string                   `json:"error,omitempty"`
}

// PipelineStageReport summarizes one pipeline concurrency level.
type PipelineStageReport struct {
	Concurrency   int           `json:"concurrency"`
	StartedAt     time.Time     `json:"started_at"`
	EndedAt       time.Time     `json:"ended_at"`
	WallClock     time.Duration `json:"wall_clock"`
	Families      int           `json:"families"`
	Succeeded     int           `json:"succeeded"`
	Failed        int           `json:"failed"`
	TotalRuns     int           `json:"total_runs"`
	RunsPerMinute float64       `json:"runs_per_minute"`
	// Distributions across the families in this stage.
	IngestLatency  LatencyStats `json:"ingest_latency"`
	VisibleLatency LatencyStats `json:"visible_latency"`
	FamilyDuration LatencyStats `json:"family_duration"`
	RunDuration    LatencyStats `json:"run_duration"`

	Resources     []ContainerUsage       `json:"resources,omitempty"`
	FamilyResults []PipelineFamilyResult `json:"family_results,omitempty"`
}

// pipelineHeartbeat is how often a waiting family reports progress, so a slow
// run reads as slow rather than as a hang.
const pipelineHeartbeat = 15 * time.Second

// PipelineRunner drives whole pipelines end to end. Runs take minutes, so the
// stage shape is "how many families are in flight" rather than "how many
// requests per second".
type PipelineRunner struct {
	cfg     Config
	client  *http.Client
	request *RequestContext
	sampler *Sampler
	logf    Logf
}

// NewPipelineRunner wires a PipelineRunner.
func NewPipelineRunner(cfg Config, client *http.Client, request *RequestContext, sampler *Sampler, logf Logf) *PipelineRunner {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &PipelineRunner{cfg: cfg, client: client, request: request, sampler: sampler, logf: logf}
}

// Run executes each configured pipeline concurrency level in ascending order.
func (p *PipelineRunner) Run(ctx context.Context) ([]PipelineStageReport, error) {
	levels := NormalizedConcurrency(p.cfg.PipelineConcurrency)
	reports := make([]PipelineStageReport, 0, len(levels))

	for _, concurrency := range levels {
		if ctx.Err() != nil {
			break
		}
		p.logf("pipeline stage: %d concurrent families x %d iteration(s)", concurrency, p.cfg.PipelineIterations)
		report := p.RunStage(ctx, concurrency)
		reports = append(reports, report)
		p.logf("  -> %d/%d families succeeded, median family %s, %d runs total",
			report.Succeeded, report.Families,
			report.FamilyDuration.P50.Round(time.Second), report.TotalRuns)
	}
	return reports, nil
}

// RunStage triggers Concurrency families simultaneously, repeated for the
// configured number of iterations, and waits for every run to finish.
func (p *PipelineRunner) RunStage(ctx context.Context, concurrency int) PipelineStageReport {
	startedAt := time.Now()
	var mu sync.Mutex
	var results []PipelineFamilyResult

	for iteration := 0; iteration < p.cfg.PipelineIterations; iteration++ {
		if ctx.Err() != nil {
			break
		}
		var wg sync.WaitGroup
		for family := 0; family < concurrency; family++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				result := p.runFamily(ctx)
				mu.Lock()
				results = append(results, result)
				mu.Unlock()
			}()
		}
		wg.Wait()
	}

	endedAt := time.Now()
	report := PipelineStageReport{
		Concurrency:   concurrency,
		StartedAt:     startedAt,
		EndedAt:       endedAt,
		WallClock:     endedAt.Sub(startedAt),
		Families:      len(results),
		FamilyResults: results,
	}

	var ingest, visible, family, runs []time.Duration
	for _, result := range results {
		if result.Succeeded {
			report.Succeeded++
		} else {
			report.Failed++
		}
		if result.IngestLatency > 0 {
			ingest = append(ingest, result.IngestLatency)
		}
		if result.VisibleLatency > 0 {
			visible = append(visible, result.VisibleLatency)
		}
		if result.TotalDuration > 0 {
			family = append(family, result.TotalDuration)
		}
		report.TotalRuns += len(result.Runs)
		for _, run := range result.Runs {
			if run.Duration > 0 {
				runs = append(runs, run.Duration)
			}
		}
	}
	report.IngestLatency = summarize(ingest)
	report.VisibleLatency = summarize(visible)
	report.FamilyDuration = summarize(family)
	report.RunDuration = summarize(runs)
	if report.WallClock > 0 {
		report.RunsPerMinute = float64(report.TotalRuns) / report.WallClock.Minutes()
	}
	if p.sampler != nil {
		report.Resources = UsageBetween(p.sampler.Samples(), startedAt, endedAt)
	}
	return report
}

// runFamily fires one signed webhook and follows every run it spawns, including
// child runs, until the whole family reaches a terminal state.
func (p *PipelineRunner) runFamily(ctx context.Context) PipelineFamilyResult {
	deadline := time.Now().Add(p.cfg.PipelineTimeout)
	familyCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	label := fmt.Sprintf("perf-%d", time.Now().UnixNano())
	triggerAt := time.Now()

	var (
		trigger triggerResult
		err     error
	)
	switch p.cfg.PipelineTrigger {
	case TriggerWebhook:
		trigger, err = p.triggerWebhook(familyCtx)
	default:
		trigger, err = p.invokeExternalTrigger(familyCtx, label)
	}

	result := PipelineFamilyResult{DeliveryID: trigger.TriggerEventID}
	if err != nil {
		result.Err = err.Error()
		result.IngestLatency = time.Since(triggerAt)
		return result
	}
	result.IngestLatency = time.Since(triggerAt)

	watched := make(map[string]struct{})
	// An external trigger names the run it created, so the family is anchored
	// on that run rather than discovered by correlation id. This removes the
	// guesswork entirely and makes the queue-latency measurement exact.
	if trigger.RunID != "" {
		watched[trigger.RunID] = struct{}{}
	}
	completed := make(map[string]runListItem)
	var firstVisibleAt time.Time
	lastReport := time.Now()

	ticker := time.NewTicker(p.cfg.PipelinePollEvery)
	defer ticker.Stop()

	for {
		runs, err := p.listRuns(familyCtx)
		if err == nil {
			p.discoverFamily(runs, trigger.TriggerEventID, watched)
			if len(watched) > 0 && firstVisibleAt.IsZero() {
				firstVisibleAt = time.Now()
				result.VisibleLatency = firstVisibleAt.Sub(triggerAt)
				p.logf("  run visible after %s (%s)",
					result.VisibleLatency.Round(time.Millisecond), label)
			}
			for _, run := range runs {
				if _, tracked := watched[run.RunID]; !tracked {
					continue
				}
				if run.IsComplete {
					completed[run.RunID] = run
				}
			}
			if len(watched) > 0 && len(completed) == len(watched) {
				result.TotalDuration = time.Since(triggerAt)
				result.Runs = observations(completed)
				result.Succeeded = allSucceeded(completed)
				return result
			}
		}

		// Long waits are normal here, but silence is not: without a heartbeat a
		// multi-minute pipeline is indistinguishable from a hang.
		if time.Since(lastReport) >= pipelineHeartbeat {
			lastReport = time.Now()
			p.logf("  waiting %s: %d run(s) discovered, %d complete (%s)",
				time.Since(triggerAt).Round(time.Second), len(watched), len(completed), label)
		}

		// A run that never becomes visible is a trigger that produced no work.
		// That answer is available in seconds, so it is not worth holding the
		// whole pipeline timeout open for it.
		if len(watched) == 0 && time.Since(triggerAt) > p.cfg.PipelineFirstRunTimeout {
			result.TotalDuration = time.Since(triggerAt)
			if p.cfg.PipelineTrigger == TriggerWebhook {
				result.Err = fmt.Sprintf(
					"webhook accepted but no run appeared within %s. "+
						"The platform resolves the pipeline definition from the repository at the payload's commit SHA, "+
						"so a synthetic payload produces no run: the commit must exist and the repository must contain a "+
						"pipeline the trigger matches. Use the default --pipeline-trigger external to avoid this entirely",
					p.cfg.PipelineFirstRunTimeout)
			} else {
				result.Err = fmt.Sprintf(
					"the trigger was accepted but no run appeared within %s; check that the pipeline it references exists and is enabled",
					p.cfg.PipelineFirstRunTimeout)
			}
			return result
		}

		select {
		case <-familyCtx.Done():
			result.TotalDuration = time.Since(triggerAt)
			result.Runs = observations(completed)
			switch {
			case len(watched) == 0:
				result.Err = fmt.Sprintf("no run appeared within %s", p.cfg.PipelineTimeout)
			default:
				result.Err = fmt.Sprintf("timed out after %s with %d/%d runs complete",
					p.cfg.PipelineTimeout, len(completed), len(watched))
			}
			return result
		case <-ticker.C:
		}
	}
}

// discoverFamily adds root runs matching the delivery ID and, transitively, any
// child run whose parent is already watched.
func (p *PipelineRunner) discoverFamily(runs []runListItem, deliveryID string, watched map[string]struct{}) {
	for _, run := range runs {
		if run.TriggerEventID == deliveryID {
			watched[run.RunID] = struct{}{}
		}
	}
	// Children can appear after their parents, and grandchildren after that, so
	// keep expanding until a pass adds nothing new.
	for {
		added := false
		for _, run := range runs {
			if run.ParentRunID == nil {
				continue
			}
			if _, tracked := watched[run.RunID]; tracked {
				continue
			}
			if _, parentWatched := watched[*run.ParentRunID]; parentWatched {
				watched[run.RunID] = struct{}{}
				added = true
			}
		}
		if !added {
			return
		}
	}
}

func (p *PipelineRunner) fireWebhook(ctx context.Context, deliveryID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.request.WebhookURL, bytes.NewReader(p.request.Payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", deliveryID)
	req.Header.Set("X-Hub-Signature-256", "sha256="+p.request.Signature)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook rejected with status %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}
	return nil
}

func (p *PipelineRunner) listRuns(ctx context.Context) ([]runListItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(p.request.APIURL, "/v1/runs?limit=1000"), nil)
	if err != nil {
		return nil, err
	}
	if err := authorize(req, p.request); err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("list runs returned status %d", resp.StatusCode)
	}
	var runs []runListItem
	if err := json.NewDecoder(resp.Body).Decode(&runs); err != nil {
		return nil, fmt.Errorf("decode runs: %w", err)
	}
	return runs, nil
}

func observations(completed map[string]runListItem) []PipelineRunObservation {
	out := make([]PipelineRunObservation, 0, len(completed))
	for _, run := range completed {
		observation := PipelineRunObservation{
			RunID:        run.RunID,
			PipelineName: run.PipelineName,
			Status:       run.Status,
			Relationship: "root",
		}
		if run.ParentRunID != nil {
			observation.Relationship = "child"
		}
		if !run.StartedAt.IsZero() && run.FinishedAt.After(run.StartedAt) {
			observation.Duration = run.FinishedAt.Sub(run.StartedAt)
		}
		out = append(out, observation)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RunID < out[j].RunID })
	return out
}

// successfulRunStatuses are the terminal statuses the platform treats as a
// non-failure. They mirror the vocabulary in
// services/nopsai/internal/runs.IsTerminalRunStatus: "warning" and an ignored
// failure both complete the run without marking it failed.
var successfulRunStatuses = map[string]struct{}{
	"success":           {},
	"warning":           {},
	"failure (ignored)": {},
}

// allSucceeded reports whether every run in the family finished without a
// failure status.
func allSucceeded(completed map[string]runListItem) bool {
	if len(completed) == 0 {
		return false
	}
	for _, run := range completed {
		normalized := strings.ToLower(strings.TrimSpace(run.Status))
		if _, ok := successfulRunStatuses[normalized]; !ok {
			return false
		}
	}
	return true
}

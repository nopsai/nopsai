package perf

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakePlatform is a stand-in for the webhook receiver plus the runs API. It
// accepts signed deliveries and, after a configurable number of polls, reports
// the runs that the delivery produced.
type fakePlatform struct {
	secret string
	// pollsBeforeVisible controls how long a run stays invisible after the
	// webhook is accepted, which is what the queue-latency measurement reads.
	pollsBeforeVisible int
	// pollsBeforeComplete controls how long a visible run stays in flight, which
	// separates "the run appeared" from "the run finished".
	pollsBeforeComplete int
	// childRuns is how many child runs each root run spawns.
	childRuns int
	// finalStatus is the terminal status every run reaches.
	finalStatus string
	// rejectSignature makes the webhook endpoint refuse the delivery.
	rejectSignature bool
	// invokeStatus, when set, makes the external trigger endpoint fail.
	invokeStatus int

	mu        sync.Mutex
	polls     map[string]int
	delivered []string
}

func newFakePlatform(secret string) *fakePlatform {
	return &fakePlatform{
		secret:      secret,
		childRuns:   1,
		finalStatus: "success",
		polls:       make(map[string]int),
	}
}

func (f *fakePlatform) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", f.handleWebhook)
	mux.HandleFunc("/v1/external-triggers/{id}/invoke", f.handleInvoke)
	mux.HandleFunc("/v1/runs", f.handleRuns)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func (f *fakePlatform) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	mac := hmac.New(sha256.New, []byte(f.secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if f.rejectSignature || r.Header.Get("X-Hub-Signature-256") != expected {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	delivery := r.Header.Get("X-GitHub-Delivery")
	if delivery == "" {
		http.Error(w, "missing delivery id", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.delivered = append(f.delivered, delivery)
	f.mu.Unlock()
	w.WriteHeader(http.StatusAccepted)
}

// handleInvoke models the external trigger API: it names the run it created, so
// the harness never has to guess which run belongs to it.
func (f *fakePlatform) handleInvoke(w http.ResponseWriter, r *http.Request) {
	if f.invokeStatus != 0 {
		http.Error(w, "trigger rejected", f.invokeStatus)
		return
	}
	f.mu.Lock()
	id := fmt.Sprintf("invocation-%d", len(f.delivered)+1)
	f.delivered = append(f.delivered, id)
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"run_id":           "root-" + id,
		"trigger_event_id": id,
		"status":           "queued",
	})
}

// handleRuns reports each delivery's runs once it has been polled often enough,
// modelling the delay between accepting a webhook and the run being queryable.
func (f *fakePlatform) handleRuns(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	runs := make([]runListItem, 0)
	for _, delivery := range f.delivered {
		f.polls[delivery]++
		if f.polls[delivery] <= f.pollsBeforeVisible {
			continue
		}
		rootID := "root-" + delivery
		complete := f.polls[delivery] > f.pollsBeforeVisible+f.pollsBeforeComplete
		status := f.finalStatus
		if !complete {
			status = "running"
		}
		runs = append(runs, runListItem{
			RunID:          rootID,
			PipelineName:   "perf-pipeline",
			Status:         status,
			StartedAt:      time.Now().Add(-30 * time.Second),
			FinishedAt:     time.Now(),
			IsComplete:     complete,
			TriggerEventID: delivery,
		})
		if !complete {
			// Children are reported only once the root has finished, matching
			// the order the platform makes them visible.
			continue
		}
		for child := 0; child < f.childRuns; child++ {
			parent := rootID
			runs = append(runs, runListItem{
				RunID:        fmt.Sprintf("%s-child-%d", rootID, child),
				PipelineName: "perf-child",
				Status:       f.finalStatus,
				StartedAt:    time.Now().Add(-10 * time.Second),
				FinishedAt:   time.Now(),
				IsComplete:   true,
				ParentRunID:  &parent,
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(runs)
}

func newPipelineHarness(t *testing.T, platform *fakePlatform, mutate func(*Config)) *PipelineRunner {
	t.Helper()
	server := platform.server(t)
	payload := []byte(`{"ref":"refs/heads/main"}`)

	cfg := DefaultConfig()
	cfg.Suites = []string{SuitePipeline}
	cfg.PipelineConcurrency = []int{1}
	cfg.PipelineIterations = 1
	cfg.PipelineTimeout = 3 * time.Second
	cfg.PipelinePollEvery = 10 * time.Millisecond
	cfg.PipelineTrigger = TriggerWebhook
	if mutate != nil {
		mutate(&cfg)
	}
	request := &RequestContext{
		APIURL:      server.URL,
		WebhookURL:  server.URL + "/webhook",
		Payload:     payload,
		Signature:   SignPayload(platform.secret, payload),
		TokenSource: func() (string, error) { return "test-token", nil },
	}
	return NewPipelineRunner(cfg, NewHTTPClient(5*time.Second, 4), request, nil, nil)
}

func TestPipelineStageTracksFamilyThroughCompletion(t *testing.T) {
	platform := newFakePlatform("secret")
	platform.pollsBeforeVisible = 2
	runner := newPipelineHarness(t, platform, nil)

	report := runner.RunStage(context.Background(), 1)

	if report.Families != 1 {
		t.Fatalf("Families = %d, want 1", report.Families)
	}
	if report.Succeeded != 1 {
		t.Fatalf("Succeeded = %d, want 1 (failures: %+v)", report.Succeeded, report.FamilyResults)
	}
	// One root plus one child must both be tracked.
	if report.TotalRuns != 2 {
		t.Errorf("TotalRuns = %d, want 2 (the root run and its child)", report.TotalRuns)
	}
	if report.IngestLatency.P95 <= 0 {
		t.Error("ingest latency was not measured")
	}
	if report.VisibleLatency.P95 <= 0 {
		t.Error("queue latency was not measured")
	}
	if report.FamilyDuration.P50 <= 0 {
		t.Error("family duration was not measured")
	}
	if report.RunsPerMinute <= 0 {
		t.Error("run throughput was not computed")
	}
}

// TestPipelineDiscoversChildRuns is the guard for family tracking: a child run
// that appears only after its parent must still be waited for and counted.
func TestPipelineDiscoversChildRuns(t *testing.T) {
	platform := newFakePlatform("secret")
	platform.childRuns = 3
	runner := newPipelineHarness(t, platform, nil)

	report := runner.RunStage(context.Background(), 1)
	if report.TotalRuns != 4 {
		t.Fatalf("TotalRuns = %d, want 4 (one root and three children)", report.TotalRuns)
	}
	var children int
	for _, family := range report.FamilyResults {
		for _, run := range family.Runs {
			if run.Relationship == "child" {
				children++
			}
		}
	}
	if children != 3 {
		t.Fatalf("counted %d child runs, want 3", children)
	}
}

func TestPipelineReportsFailedRunStatus(t *testing.T) {
	platform := newFakePlatform("secret")
	platform.finalStatus = "failure"
	runner := newPipelineHarness(t, platform, nil)

	report := runner.RunStage(context.Background(), 1)
	if report.Succeeded != 0 || report.Failed != 1 {
		t.Fatalf("Succeeded=%d Failed=%d, want 0/1", report.Succeeded, report.Failed)
	}
}

// TestPipelineTreatsWarningAsSuccess pins the harness to the platform's own
// vocabulary, where a warning is a completed run rather than a failure.
func TestPipelineTreatsWarningAsSuccess(t *testing.T) {
	platform := newFakePlatform("secret")
	platform.finalStatus = "warning"
	runner := newPipelineHarness(t, platform, nil)

	if report := runner.RunStage(context.Background(), 1); report.Succeeded != 1 {
		t.Fatalf("Succeeded = %d, want a warning run to count as a success", report.Succeeded)
	}
}

func TestPipelineReportsRejectedWebhook(t *testing.T) {
	platform := newFakePlatform("secret")
	platform.rejectSignature = true
	runner := newPipelineHarness(t, platform, nil)

	report := runner.RunStage(context.Background(), 1)
	if report.Failed != 1 {
		t.Fatalf("Failed = %d, want 1", report.Failed)
	}
	if len(report.FamilyResults) != 1 || !strings.Contains(report.FamilyResults[0].Err, "401") {
		t.Fatalf("family error = %+v, want the rejection to be reported", report.FamilyResults)
	}
}

// TestPipelineTimesOutWhenNoRunAppears keeps a misconfigured trigger from
// looking like an infinite hang.
func TestPipelineTimesOutWhenNoRunAppears(t *testing.T) {
	platform := newFakePlatform("secret")
	// The run never becomes visible within the timeout.
	platform.pollsBeforeVisible = 1_000_000
	runner := newPipelineHarness(t, platform, func(c *Config) {
		c.PipelineTimeout = 200 * time.Millisecond
	})

	report := runner.RunStage(context.Background(), 1)
	if report.Failed != 1 {
		t.Fatalf("Failed = %d, want 1", report.Failed)
	}
	if !strings.Contains(report.FamilyResults[0].Err, "no run appeared") {
		t.Fatalf("family error = %q, want a clear timeout reason", report.FamilyResults[0].Err)
	}
}

func TestPipelineRunsEveryConcurrencyLevel(t *testing.T) {
	platform := newFakePlatform("secret")
	runner := newPipelineHarness(t, platform, func(c *Config) {
		c.PipelineConcurrency = []int{2, 1}
	})

	reports, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("got %d stages, want 2", len(reports))
	}
	if reports[0].Concurrency != 1 || reports[1].Concurrency != 2 {
		t.Fatalf("stages ran at %d then %d, want ascending order", reports[0].Concurrency, reports[1].Concurrency)
	}
	if reports[1].Families != 2 {
		t.Errorf("the concurrency-2 stage ran %d families, want 2", reports[1].Families)
	}
}

func TestPipelineIterationsMultiplyFamilies(t *testing.T) {
	platform := newFakePlatform("secret")
	runner := newPipelineHarness(t, platform, func(c *Config) {
		c.PipelineIterations = 3
	})

	report := runner.RunStage(context.Background(), 2)
	if report.Families != 6 {
		t.Fatalf("Families = %d, want 6 (2 concurrent x 3 iterations)", report.Families)
	}
}

func TestDiscoverFamilyFollowsGrandchildren(t *testing.T) {
	runner := &PipelineRunner{}
	root := "root-1"
	child := "child-1"
	runs := []runListItem{
		{RunID: root, TriggerEventID: "delivery-1"},
		{RunID: child, ParentRunID: &root},
		{RunID: "grandchild-1", ParentRunID: &child},
		{RunID: "unrelated", TriggerEventID: "other-delivery"},
	}

	watched := make(map[string]struct{})
	runner.discoverFamily(runs, "delivery-1", watched)

	for _, want := range []string{root, child, "grandchild-1"} {
		if _, ok := watched[want]; !ok {
			t.Errorf("%s was not discovered", want)
		}
	}
	if _, ok := watched["unrelated"]; ok {
		t.Error("a run from another delivery was pulled into the family")
	}
}

func TestAllSucceededRequiresAtLeastOneRun(t *testing.T) {
	if allSucceeded(map[string]runListItem{}) {
		t.Fatal("an empty family must not count as a success")
	}
}

// TestPipelineFailsFastWhenNoRunAppears is the regression guard for the run
// that looked hung: a trigger producing no work must be reported in seconds,
// not hold the full pipeline timeout open in silence.
func TestPipelineFailsFastWhenNoRunAppears(t *testing.T) {
	platform := newFakePlatform("secret")
	platform.pollsBeforeVisible = 1_000_000 // never becomes visible
	runner := newPipelineHarness(t, platform, func(c *Config) {
		c.PipelineFirstRunTimeout = 150 * time.Millisecond
		// An overall timeout far larger than the first-run bound, so a family
		// that waits on the wrong one is unmistakable.
		c.PipelineTimeout = 30 * time.Second
	})

	started := time.Now()
	report := runner.RunStage(context.Background(), 1)
	elapsed := time.Since(started)

	if report.Failed != 1 {
		t.Fatalf("Failed = %d, want 1", report.Failed)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("the family took %s to give up, want it bounded by the first-run timeout", elapsed)
	}
	if !strings.Contains(report.FamilyResults[0].Err, "no run appeared") {
		t.Fatalf("family error = %q, want it to say no run appeared", report.FamilyResults[0].Err)
	}
	// The message must name the actual cause, not just the symptom.
	if !strings.Contains(report.FamilyResults[0].Err, "commit") {
		t.Fatalf("family error = %q, want it to explain that the commit must exist", report.FamilyResults[0].Err)
	}
}

// TestPipelineWaitsBeyondFirstRunTimeoutOnceARunExists is the counterpart: the
// short bound only governs whether a run ever appeared. Once one has, execution
// is allowed to take far longer, because that is what pipelines legitimately do.
func TestPipelineWaitsBeyondFirstRunTimeoutOnceARunExists(t *testing.T) {
	platform := newFakePlatform("secret")
	platform.pollsBeforeVisible = 1
	// The run stays in flight well past the first-run bound before finishing.
	platform.pollsBeforeComplete = 20
	runner := newPipelineHarness(t, platform, func(c *Config) {
		c.PipelinePollEvery = 10 * time.Millisecond
		c.PipelineFirstRunTimeout = 50 * time.Millisecond
		c.PipelineTimeout = 10 * time.Second
	})

	started := time.Now()
	report := runner.RunStage(context.Background(), 1)

	if report.Succeeded != 1 {
		t.Fatalf("Succeeded = %d, want 1; the family gave up on a run that was already visible: %+v",
			report.Succeeded, report.FamilyResults)
	}
	if elapsed := time.Since(started); elapsed < 50*time.Millisecond {
		t.Fatalf("the family finished in %s, so it did not actually wait past the first-run bound", elapsed)
	}
}

// externalHarness wires a PipelineRunner that drives runs through the external
// trigger, which is the default and self-contained path.
func externalHarness(t *testing.T, platform *fakePlatform, mutate func(*Config)) *PipelineRunner {
	t.Helper()
	runner := newPipelineHarness(t, platform, func(c *Config) {
		c.PipelineTrigger = TriggerExternal
		c.ExternalTriggerID = DefaultExternalTriggerID
		c.PipelineWorkSeconds = 0
		if mutate != nil {
			mutate(c)
		}
	})
	return runner
}

// TestPipelineExternalTriggerAnchorsOnTheReturnedRun is the point of the
// external trigger: the invoke response names the run, so the family is exact
// rather than discovered by correlation.
func TestPipelineExternalTriggerAnchorsOnTheReturnedRun(t *testing.T) {
	platform := newFakePlatform("secret")
	platform.childRuns = 2
	runner := externalHarness(t, platform, nil)

	report := runner.RunStage(context.Background(), 1)
	if report.Succeeded != 1 {
		t.Fatalf("Succeeded = %d, want 1: %+v", report.Succeeded, report.FamilyResults)
	}
	if report.TotalRuns != 3 {
		t.Errorf("TotalRuns = %d, want 3 (one root and two children)", report.TotalRuns)
	}
	if report.VisibleLatency.P95 <= 0 {
		t.Error("queue latency was not measured")
	}
}

// TestPipelineExternalTriggerNeedsNoWebhookCredentials is the property that
// makes this the default: it works with no signing secret and no payload.
func TestPipelineExternalTriggerNeedsNoWebhookCredentials(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Suites = []string{SuitePipeline}
	cfg.WebhookSecret = ""
	cfg.PayloadFile = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the default pipeline suite should need no webhook credentials, got %v", err)
	}
	if cfg.NeedsWebhookPayload() {
		t.Error("the external trigger must not require a git payload")
	}

	cfg.PipelineTrigger = TriggerWebhook
	if !cfg.NeedsWebhookPayload() {
		t.Error("the webhook trigger must require a git payload")
	}
	if err := cfg.Validate(); err == nil {
		t.Error("the webhook trigger should require a signing secret")
	}
}

func TestPipelineExternalTriggerExplainsAMissingFixture(t *testing.T) {
	platform := newFakePlatform("secret")
	platform.invokeStatus = http.StatusNotFound
	runner := externalHarness(t, platform, nil)

	report := runner.RunStage(context.Background(), 1)
	if report.Failed != 1 {
		t.Fatalf("Failed = %d, want 1", report.Failed)
	}
	message := report.FamilyResults[0].Err
	// The error has to tell the operator how to install the fixture.
	for _, want := range []string{"test/perf/fixtures", "perf-load-probe"} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q is missing %q", message, want)
		}
	}
}

func TestPipelineExternalTriggerExplainsARejectedCaller(t *testing.T) {
	platform := newFakePlatform("secret")
	platform.invokeStatus = http.StatusForbidden
	runner := externalHarness(t, platform, nil)

	report := runner.RunStage(context.Background(), 1)
	if !strings.Contains(report.FamilyResults[0].Err, "allowed_callers") {
		t.Fatalf("error = %q, want it to name allowed_callers", report.FamilyResults[0].Err)
	}
}

func TestValidateRejectsUnknownTriggerMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Suites = []string{SuitePipeline}
	cfg.PipelineTrigger = "carrier-pigeon"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "unknown pipeline trigger") {
		t.Fatalf("Validate() = %v, want it to reject the mode", err)
	}
}

package perf

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestRuntimeTargetsRotate(t *testing.T) {
	targets := NewRuntimeTargets([]string{"a", "b", "c"})
	if targets.Empty() || targets.Len() != 3 {
		t.Fatalf("Len = %d, Empty = %v", targets.Len(), targets.Empty())
	}
	// Load must spread across runs rather than concentrating on one row.
	seen := map[string]int{}
	for i := 0; i < 9; i++ {
		seen[targets.Next()]++
	}
	for _, id := range []string{"a", "b", "c"} {
		if seen[id] != 3 {
			t.Errorf("run %q was targeted %d times, want an even 3", id, seen[id])
		}
	}
}

func TestRuntimeTargetsEmpty(t *testing.T) {
	targets := NewRuntimeTargets(nil)
	if !targets.Empty() {
		t.Error("nil targets should be empty")
	}
	if targets.Next() != "" {
		t.Error("an empty target set must yield no run id")
	}
}

func TestRuntimeTargetsAreSafeForConcurrentUse(t *testing.T) {
	targets := NewRuntimeTargets([]string{"a", "b"})
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				if targets.Next() == "" {
					t.Error("Next returned an empty id")
					return
				}
			}
		}()
	}
	wg.Wait()
}

// TestRuntimeLogIngestBuildsABatch pins the shape of the highest-volume call a
// running pipeline makes. The handler queues one INSERT per line, so the batch
// size is the main lever on database write pressure per request.
func TestRuntimeLogIngestBuildsABatch(t *testing.T) {
	request := &RequestContext{
		APIURL:      "http://api.test",
		Runtime:     NewRuntimeTargets([]string{"run-1"}),
		TokenSource: func() (string, error) { return "token", nil },
	}
	var ingest Scenario
	for _, scenario := range runtimeScenarios() {
		if scenario.Name == "runtime.logs_ingest" {
			ingest = scenario
		}
	}
	if ingest.Name == "" {
		t.Fatal("the runtime suite has no log ingest scenario")
	}
	if ingest.Service != ServiceAPI {
		t.Errorf("Service = %q, want the API", ingest.Service)
	}

	req, err := ingest.Build(context.Background(), request)
	if err != nil {
		t.Fatalf("Build returned %v", err)
	}
	if req.URL.String() != "http://api.test/v1/runs/run-1/logs/ingest" {
		t.Fatalf("URL = %q", req.URL.String())
	}
	if req.Header.Get("Authorization") == "" {
		t.Error("log ingest must be authorized, since it is in production too")
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var payload struct {
		Lines  []string `json:"lines"`
		Source string   `json:"source"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if len(payload.Lines) != runtimeLogBatchLines {
		t.Errorf("got %d lines, want %d", len(payload.Lines), runtimeLogBatchLines)
	}
	// Everything this harness writes has to be identifiable afterwards.
	if payload.Source != "nopsai-perf" {
		t.Errorf("source = %q, want the harness to mark its own writes", payload.Source)
	}
	for _, line := range payload.Lines {
		if !strings.Contains(line, "nopsai-perf") {
			t.Fatalf("log line %q is not marked as synthetic", line)
		}
	}
}

func TestRuntimeScenariosFailClearlyWithoutTargets(t *testing.T) {
	request := &RequestContext{
		APIURL:      "http://api.test",
		Runtime:     NewRuntimeTargets(nil),
		TokenSource: func() (string, error) { return "token", nil },
	}
	for _, scenario := range runtimeScenarios() {
		if _, err := scenario.Build(context.Background(), request); err == nil {
			t.Errorf("%s built a request with no target run", scenario.Name)
		}
	}
}

// TestRuntimeSuiteCoversTheTelemetryPath documents what the suite stands in for:
// the calls a run makes against the platform while it executes.
func TestRuntimeSuiteCoversTheTelemetryPath(t *testing.T) {
	mix := BuildMix([]string{SuiteRuntime})
	names := mix.Names()

	for _, want := range []string{
		"runtime.logs_ingest",
		"runtime.run_status",
		"runtime.run_logs",
		"runtime.run_detail",
	} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("the runtime suite is missing %q", want)
		}
	}
}

func TestUISuiteTargetsTheUIService(t *testing.T) {
	scenarios := uiScenarios()
	if len(scenarios) == 0 {
		t.Fatal("the ui suite has no scenarios")
	}
	if scenarios[0].Service != ServiceUI {
		t.Errorf("Service = %q, want the UI", scenarios[0].Service)
	}

	request := &RequestContext{UIURL: "http://ui.test/"}
	req, err := scenarios[0].Build(context.Background(), request)
	if err != nil {
		t.Fatalf("Build returned %v", err)
	}
	if req.URL.String() != "http://ui.test/" {
		t.Errorf("URL = %q", req.URL.String())
	}
	// The UI is served by nginx and needs no platform credentials.
	if req.Header.Get("Authorization") != "" {
		t.Error("the UI scenario must not send credentials")
	}
}

// TestScenariosDeclareAService guards the report: an untagged scenario silently
// disappears from the per-service comparison.
func TestScenariosDeclareAService(t *testing.T) {
	mix := BuildMix([]string{SuiteAPIRead, SuiteAuth, SuiteRuntime, SuiteUI, SuiteWebhook})
	for _, scenario := range mix.scenarios {
		if scenario.Service == "" {
			t.Errorf("scenario %q declares no service", scenario.Name)
		}
	}
}

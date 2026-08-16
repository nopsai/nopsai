package perf

import (
	"testing"
	"time"
)

// serviceStage builds a stage whose scenarios are spread across services, which
// is the shape the capacity comparison consumes.
func serviceStage(concurrency int, entries ...ScenarioStats) StageReport {
	stage := StageReport{Concurrency: concurrency, Measured: 30 * time.Second}
	var requests int64
	for _, entry := range entries {
		requests += entry.Requests
	}
	stage.Scenarios = entries
	stage.Total = ScenarioStats{Name: "all", Requests: requests}
	return stage
}

func scenarioFor(name, service string, requests int64, rps float64, p95 time.Duration, errorRate float64) ScenarioStats {
	return ScenarioStats{
		Name:       name,
		Service:    service,
		Requests:   requests,
		Failures:   int64(float64(requests) * errorRate),
		ErrorRate:  errorRate,
		Throughput: rps,
		Latency:    LatencyStats{P50: p95 / 2, P95: p95, P99: p95 * 2, Max: p95 * 3},
	}
}

func TestServiceStagesGroupsByService(t *testing.T) {
	stage := serviceStage(10,
		scenarioFor("runs.list", ServiceAPI, 100, 10, 50*time.Millisecond, 0),
		scenarioFor("monitoring.summary", ServiceAPI, 100, 10, 500*time.Millisecond, 0),
		scenarioFor("auth.me", ServiceAuth, 200, 20, 20*time.Millisecond, 0),
	)

	services := ServiceStages(stage)
	if len(services) != 2 {
		t.Fatalf("got %d services, want 2", len(services))
	}
	// Sorted by name, so aaa comes first.
	auth, api := services[0], services[1]
	if auth.Service != ServiceAuth || api.Service != ServiceAPI {
		t.Fatalf("unexpected grouping: %q, %q", auth.Service, api.Service)
	}
	if api.Requests != 200 {
		t.Errorf("api requests = %d, want the sum of its scenarios", api.Requests)
	}
	if api.Throughput != 20 {
		t.Errorf("api throughput = %v, want the sum of its scenarios", api.Throughput)
	}
	// The service's p95 must be its worst endpoint, never an average that would
	// understate how slow the service actually got.
	if api.Latency.P95 != 500*time.Millisecond {
		t.Errorf("api p95 = %v, want the worst endpoint's 500ms", api.Latency.P95)
	}
}

func TestServiceStagesIgnoresUntaggedScenarios(t *testing.T) {
	stage := serviceStage(10, ScenarioStats{Name: "untagged", Requests: 10})
	if services := ServiceStages(stage); len(services) != 0 {
		t.Fatalf("got %d services from untagged scenarios, want 0", len(services))
	}
}

// TestCompareServicesRanksByCarriedLoad is the headline question: under the same
// pressure, which service carried the most work.
func TestCompareServicesRanksByCarriedLoad(t *testing.T) {
	cfg := analysisConfig()
	stages := []StageReport{
		serviceStage(1,
			scenarioFor("runs.list", ServiceAPI, 100, 10, 20*time.Millisecond, 0),
			scenarioFor("auth.me", ServiceAuth, 100, 10, 10*time.Millisecond, 0),
			scenarioFor("ui.index", ServiceUI, 100, 10, 2*time.Millisecond, 0),
		),
		serviceStage(50,
			// The API slows sharply and starts failing.
			scenarioFor("runs.list", ServiceAPI, 900, 30, 1500*time.Millisecond, 0.05),
			// aaa carries more requests and stays fast.
			scenarioFor("auth.me", ServiceAuth, 3000, 100, 30*time.Millisecond, 0),
			scenarioFor("ui.index", ServiceUI, 1500, 50, 3*time.Millisecond, 0),
		),
	}

	capacities := CompareServices(stages, cfg)
	if len(capacities) != 3 {
		t.Fatalf("got %d services, want 3", len(capacities))
	}
	if capacities[0].Service != ServiceAuth {
		t.Errorf("highest-throughput service = %q, want aaa", capacities[0].Service)
	}

	byName := map[string]ServiceCapacity{}
	for _, capacity := range capacities {
		byName[capacity.Service] = capacity
	}

	api := byName[ServiceAPI]
	if !api.Breached || api.BreachConcurrency != 50 {
		t.Errorf("api should have breached at 50, got breached=%v at %d", api.Breached, api.BreachConcurrency)
	}
	if api.LatencyGrowth < 70 {
		t.Errorf("api latency growth = %.1fx, want ~75x (20ms to 1.5s)", api.LatencyGrowth)
	}

	auth := byName[ServiceAuth]
	if auth.Breached {
		t.Error("aaa met the thresholds throughout and must not be marked as breached")
	}
	if auth.PeakThroughput != 100 {
		t.Errorf("aaa peak throughput = %v, want 100", auth.PeakThroughput)
	}
}

func TestWeakestServiceIsTheFirstToBreak(t *testing.T) {
	capacities := []ServiceCapacity{
		{Service: ServiceAPI, Breached: true, BreachConcurrency: 50},
		{Service: ServiceDispatcher, Breached: true, BreachConcurrency: 10},
		{Service: ServiceAuth, Breached: false},
	}
	weakest, ok := WeakestService(capacities)
	if !ok {
		t.Fatal("no weakest service was identified")
	}
	if weakest.Service != ServiceDispatcher {
		t.Fatalf("weakest = %q, want the one that broke at the lowest level", weakest.Service)
	}
}

func TestWeakestServiceAbsentWhenNothingBroke(t *testing.T) {
	capacities := []ServiceCapacity{{Service: ServiceAPI}, {Service: ServiceAuth}}
	if _, ok := WeakestService(capacities); ok {
		t.Fatal("a weakest service was reported although nothing breached")
	}
}

func TestBusiestServiceIsTheOneCompletingMostWork(t *testing.T) {
	capacities := []ServiceCapacity{
		{Service: ServiceAPI, PeakThroughput: 900, TotalRequests: 9000, LatencyGrowth: 8},
		{Service: ServiceAuth, PeakThroughput: 400, TotalRequests: 4000, LatencyGrowth: 1.1},
	}
	busiest, ok := BusiestService(capacities)
	if !ok {
		t.Fatal("no busiest service was identified")
	}
	if busiest.Service != ServiceAPI {
		t.Fatalf("busiest = %q, want the highest-throughput service", busiest.Service)
	}
}

// TestMostResilientServiceIgnoresRawThroughput is the distinction the report
// depends on: the service receiving the most traffic is not automatically the
// one with capacity to spare. Absorbing load without turning it into latency is.
func TestMostResilientServiceIgnoresRawThroughput(t *testing.T) {
	capacities := []ServiceCapacity{
		// Busiest, but its latency grew eightfold.
		{Service: ServiceAPI, PeakThroughput: 900, TotalRequests: 9000, LatencyGrowth: 8},
		// Less traffic, but nearly flat latency.
		{Service: ServiceAuth, PeakThroughput: 400, TotalRequests: 4000, LatencyGrowth: 1.1},
		// Flattest of all, but it broke, so it cannot be resilient.
		{Service: ServiceUI, PeakThroughput: 200, TotalRequests: 2000, LatencyGrowth: 1.0,
			Breached: true, BreachConcurrency: 10},
	}
	resilient, ok := MostResilientService(capacities)
	if !ok {
		t.Fatal("no resilient service was identified")
	}
	if resilient.Service != ServiceAuth {
		t.Fatalf("most resilient = %q, want the service that absorbed load with least latency growth", resilient.Service)
	}
}

func TestMostResilientServiceAbsentWhenEverythingBroke(t *testing.T) {
	capacities := []ServiceCapacity{
		{Service: ServiceAPI, TotalRequests: 10, LatencyGrowth: 2, Breached: true},
	}
	if _, ok := MostResilientService(capacities); ok {
		t.Fatal("a resilient service was reported although every service breached")
	}
}

// TestCompareServicesAttributesContainerCPU ties the request-side numbers to
// what the container actually cost, which is what turns the comparison into a
// scaling decision.
func TestCompareServicesAttributesContainerCPU(t *testing.T) {
	stage := serviceStage(25, scenarioFor("runs.list", ServiceAPI, 500, 50, 40*time.Millisecond, 0))
	stage.Resources = []ContainerUsage{
		{Container: "nopsai", CPUAvg: 210},
		{Container: "nopsai-db", CPUAvg: 300},
	}
	capacities := CompareServices([]StageReport{stage}, analysisConfig())
	if len(capacities) != 1 {
		t.Fatalf("got %d services, want 1", len(capacities))
	}
	if capacities[0].CPUAvgAtPeak != 210 {
		t.Fatalf("CPUAvgAtPeak = %v, want the nopsai container's 210%%", capacities[0].CPUAvgAtPeak)
	}
}

func TestServiceContainerMapping(t *testing.T) {
	for service, want := range map[string]string{
		ServiceAPI:        "nopsai",
		ServiceAuth:       "nopsai-aaa",
		ServiceDispatcher: "nopsai-dispatcher",
		ServiceUI:         "nopsai-ui",
		"unknown":         "",
	} {
		if got := serviceContainer(service); got != want {
			t.Errorf("serviceContainer(%q) = %q, want %q", service, got, want)
		}
	}
}

func TestCompareServicesHandlesEmptyRamp(t *testing.T) {
	if capacities := CompareServices(nil, analysisConfig()); capacities != nil {
		t.Fatalf("got %d capacities from an empty ramp, want none", len(capacities))
	}
}

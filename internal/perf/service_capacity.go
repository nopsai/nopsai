package perf

import (
	"sort"
	"time"
)

// ServiceStageStats is one service's behaviour at one concurrency level.
type ServiceStageStats struct {
	Service     string       `json:"service"`
	Concurrency int          `json:"concurrency"`
	Requests    int64        `json:"requests"`
	Failures    int64        `json:"failures"`
	ErrorRate   float64      `json:"error_rate"`
	Throughput  float64      `json:"throughput_rps"`
	Latency     LatencyStats `json:"latency"`
}

// ServiceCapacity is the comparison verdict for one service across the ramp:
// how much load it carried, how hard it was working, and where it gave out.
type ServiceCapacity struct {
	Service string `json:"service"`
	// PeakThroughput is the most work this service completed at any level.
	PeakThroughput  float64 `json:"peak_throughput_rps"`
	PeakConcurrency int     `json:"peak_concurrency"`
	// BaselineP95 and PeakP95 bracket how much latency grew across the ramp.
	BaselineP95 time.Duration `json:"baseline_p95"`
	PeakP95     time.Duration `json:"peak_p95"`
	// LatencyGrowth is PeakP95 relative to BaselineP95. A service that carries
	// load well keeps this low even as concurrency rises.
	LatencyGrowth float64 `json:"latency_growth"`
	// WorstErrorRate is the highest failure rate the service reached.
	WorstErrorRate float64 `json:"worst_error_rate"`
	// BreachConcurrency is the first level where this service alone breached the
	// thresholds. Zero means it never did.
	BreachConcurrency int  `json:"breach_concurrency"`
	Breached          bool `json:"breached"`
	// CPUAvgAtPeak is the container's average CPU at the busiest level, when
	// resource sampling was available.
	CPUAvgAtPeak  float64 `json:"cpu_avg_at_peak,omitempty"`
	TotalRequests int64   `json:"total_requests"`
}

// serviceContainer maps a measured service to the container that runs it, so
// request-side numbers can be lined up with resource cost.
func serviceContainer(service string) string {
	switch service {
	case ServiceAPI:
		return "nopsai"
	case ServiceAuth:
		return "nopsai-aaa"
	case ServiceDispatcher:
		return "nopsai-dispatcher"
	case ServiceUI:
		return "nopsai-ui"
	default:
		return ""
	}
}

// ServiceStages groups a stage's scenarios by the service they load, producing
// exact per-service latency by re-deriving it from the scenario summaries that
// belong to each service.
//
// Percentiles here are the maximum across the service's scenarios rather than a
// merged distribution: the harness keeps merged samples only at stage level, and
// taking the worst scenario is the honest conservative reading, never claiming
// the service was faster than one of its endpoints actually was.
func ServiceStages(stage StageReport) []ServiceStageStats {
	grouped := make(map[string]*ServiceStageStats)

	for _, scenario := range stage.Scenarios {
		service := scenario.Service
		if service == "" || scenario.Requests == 0 {
			continue
		}
		entry, ok := grouped[service]
		if !ok {
			entry = &ServiceStageStats{Service: service, Concurrency: stage.Concurrency}
			grouped[service] = entry
		}
		entry.Requests += scenario.Requests
		entry.Failures += scenario.Failures
		entry.Throughput += scenario.Throughput

		// Keep the worst observation per quantile across the service's endpoints.
		entry.Latency.P50 = maxDuration(entry.Latency.P50, scenario.Latency.P50)
		entry.Latency.P90 = maxDuration(entry.Latency.P90, scenario.Latency.P90)
		entry.Latency.P95 = maxDuration(entry.Latency.P95, scenario.Latency.P95)
		entry.Latency.P99 = maxDuration(entry.Latency.P99, scenario.Latency.P99)
		entry.Latency.Max = maxDuration(entry.Latency.Max, scenario.Latency.Max)
		if entry.Latency.Min == 0 || (scenario.Latency.Min > 0 && scenario.Latency.Min < entry.Latency.Min) {
			entry.Latency.Min = scenario.Latency.Min
		}
	}

	out := make([]ServiceStageStats, 0, len(grouped))
	for _, entry := range grouped {
		if entry.Requests > 0 {
			entry.ErrorRate = float64(entry.Failures) / float64(entry.Requests)
		}
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Service < out[j].Service })
	return out
}

// CompareServices builds the per-service capacity verdict across the whole ramp.
func CompareServices(stages []StageReport, cfg Config) []ServiceCapacity {
	if len(stages) == 0 {
		return nil
	}
	ordered := make([]StageReport, len(stages))
	copy(ordered, stages)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Concurrency < ordered[j].Concurrency })

	capacities := make(map[string]*ServiceCapacity)
	peakStage := make(map[string]StageReport)

	for _, stage := range ordered {
		for _, service := range ServiceStages(stage) {
			entry, ok := capacities[service.Service]
			if !ok {
				entry = &ServiceCapacity{
					Service:     service.Service,
					BaselineP95: service.Latency.P95,
				}
				capacities[service.Service] = entry
			}
			entry.TotalRequests += service.Requests
			if service.Throughput > entry.PeakThroughput {
				entry.PeakThroughput = service.Throughput
				entry.PeakConcurrency = service.Concurrency
				peakStage[service.Service] = stage
			}
			entry.PeakP95 = maxDuration(entry.PeakP95, service.Latency.P95)
			if service.ErrorRate > entry.WorstErrorRate {
				entry.WorstErrorRate = service.ErrorRate
			}
			if !entry.Breached &&
				(service.ErrorRate > cfg.ErrorBudget || service.Latency.P95 > cfg.LatencySLO) {
				entry.Breached = true
				entry.BreachConcurrency = service.Concurrency
			}
		}
	}

	out := make([]ServiceCapacity, 0, len(capacities))
	for name, entry := range capacities {
		if entry.BaselineP95 > 0 {
			entry.LatencyGrowth = float64(entry.PeakP95) / float64(entry.BaselineP95)
		}
		if stage, ok := peakStage[name]; ok {
			container := serviceContainer(name)
			for _, usage := range stage.Resources {
				if usage.Container == container {
					entry.CPUAvgAtPeak = usage.CPUAvg
					break
				}
			}
		}
		out = append(out, *entry)
	}
	// Rank by the thing being asked: how much load the service carried.
	sort.Slice(out, func(i, j int) bool {
		if out[i].PeakThroughput != out[j].PeakThroughput {
			return out[i].PeakThroughput > out[j].PeakThroughput
		}
		return out[i].Service < out[j].Service
	})
	return out
}

// WeakestService returns the service that gave out first, which is the one to
// scale before any other. A service that never breached is never returned.
func WeakestService(capacities []ServiceCapacity) (ServiceCapacity, bool) {
	var weakest ServiceCapacity
	found := false
	for _, capacity := range capacities {
		if !capacity.Breached {
			continue
		}
		if !found || capacity.BreachConcurrency < weakest.BreachConcurrency {
			weakest = capacity
			found = true
		}
	}
	return weakest, found
}

// BusiestService returns the service that completed the most work. Carrying the
// most load is not the same as carrying it well, which is why this is reported
// separately from resilience.
func BusiestService(capacities []ServiceCapacity) (ServiceCapacity, bool) {
	var busiest ServiceCapacity
	found := false
	for _, capacity := range capacities {
		if capacity.TotalRequests == 0 {
			continue
		}
		if !found || capacity.PeakThroughput > busiest.PeakThroughput {
			busiest = capacity
			found = true
		}
	}
	return busiest, found
}

// MostResilientService returns the service whose latency grew least across the
// ramp without breaching. This is the "better capacity" answer: absorbing more
// concurrency without turning it into latency is what separates a service with
// headroom from one that merely happens to receive the most traffic.
func MostResilientService(capacities []ServiceCapacity) (ServiceCapacity, bool) {
	var best ServiceCapacity
	found := false
	for _, capacity := range capacities {
		if capacity.Breached || capacity.TotalRequests == 0 || capacity.LatencyGrowth <= 0 {
			continue
		}
		if !found || capacity.LatencyGrowth < best.LatencyGrowth {
			best = capacity
			found = true
		}
	}
	return best, found
}

// sharedConstraintGrowthSpread is how close per-service latency growth has to be
// before the degradation is read as shared rather than service-specific.
const sharedConstraintGrowthSpread = 2.0

// SharedConstraint reports whether every service degraded by a similar factor.
// When they do, no single service is the weak one: they are all queueing behind
// something they have in common, which in this topology is Postgres or the host.
// Naming a "most resilient" service in that situation would be noise.
func SharedConstraint(capacities []ServiceCapacity) bool {
	lowest, highest := 0.0, 0.0
	counted := 0
	for _, capacity := range capacities {
		if capacity.LatencyGrowth <= 0 || capacity.TotalRequests == 0 {
			continue
		}
		counted++
		if lowest == 0 || capacity.LatencyGrowth < lowest {
			lowest = capacity.LatencyGrowth
		}
		if capacity.LatencyGrowth > highest {
			highest = capacity.LatencyGrowth
		}
	}
	// A single service cannot demonstrate a shared constraint, and neither can
	// a set that barely moved.
	if counted < 2 || lowest <= 0 || highest < 1.5 {
		return false
	}
	return highest/lowest < sharedConstraintGrowthSpread
}

// EarliestServiceBreach returns the lowest concurrency at which any single
// service breached. The stage-level p95 blends fast and slow endpoints together,
// so it can still pass while a service is already over the SLO.
func EarliestServiceBreach(capacities []ServiceCapacity) (ServiceCapacity, bool) {
	return WeakestService(capacities)
}

func maxDuration(a, b time.Duration) time.Duration {
	if b > a {
		return b
	}
	return a
}

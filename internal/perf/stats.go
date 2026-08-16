package perf

import (
	"math"
	"sort"
	"sync"
	"time"
)

// Result is a single completed request observation fed into a Recorder.
type Result struct {
	Scenario string
	Service  string
	Latency  time.Duration
	Status   int
	Bytes    int64
	// Err is non-empty when the request never produced a response, for example
	// on a timeout or a refused connection.
	Err string
}

// Failed reports whether the observation counts against the error budget. A
// response is a failure when the transport failed or the server returned any
// status outside 2xx/3xx.
func (r Result) Failed() bool {
	if r.Err != "" {
		return true
	}
	return r.Status < 200 || r.Status >= 400
}

// Recorder accumulates request observations from every worker in a stage. It is
// safe for concurrent use.
//
// Every observation is stored twice: once under its scenario and once in a
// combined set. Keeping the combined samples is what makes the stage-level
// percentiles exact, because a percentile of a mixed workload cannot be derived
// from the per-scenario percentiles.
type Recorder struct {
	mu        sync.Mutex
	scenarios map[string]*scenarioSamples
	overall   *scenarioSamples
}

type scenarioSamples struct {
	service   string
	latencies []time.Duration
	statuses  map[int]int64
	errors    map[string]int64
	bytes     int64
	requests  int64
	failures  int64
}

func newScenarioSamples() *scenarioSamples {
	return &scenarioSamples{
		statuses: make(map[int]int64),
		errors:   make(map[string]int64),
	}
}

func (s *scenarioSamples) add(result Result) {
	if s.service == "" {
		s.service = result.Service
	}
	s.requests++
	s.bytes += result.Bytes
	s.latencies = append(s.latencies, result.Latency)
	if result.Err != "" {
		s.errors[result.Err]++
	} else {
		s.statuses[result.Status]++
	}
	if result.Failed() {
		s.failures++
	}
}

func (s *scenarioSamples) summary(name string, elapsed time.Duration) ScenarioStats {
	stats := ScenarioStats{
		Name:       name,
		Service:    s.service,
		Requests:   s.requests,
		Failures:   s.failures,
		BytesTotal: s.bytes,
		Latency:    summarize(s.latencies),
	}
	if s.requests > 0 {
		stats.ErrorRate = float64(s.failures) / float64(s.requests)
	}
	if elapsed > 0 {
		stats.Throughput = float64(s.requests) / elapsed.Seconds()
	}
	if len(s.statuses) > 0 {
		stats.StatusCodes = make(map[int]int64, len(s.statuses))
		for code, count := range s.statuses {
			stats.StatusCodes[code] = count
		}
	}
	if len(s.errors) > 0 {
		stats.Errors = make(map[string]int64, len(s.errors))
		for message, count := range s.errors {
			stats.Errors[message] = count
		}
	}
	return stats
}

// NewRecorder returns an empty Recorder.
func NewRecorder() *Recorder {
	return &Recorder{
		scenarios: make(map[string]*scenarioSamples),
		overall:   newScenarioSamples(),
	}
}

// Record stores one observation.
func (r *Recorder) Record(result Result) {
	r.mu.Lock()
	defer r.mu.Unlock()
	samples, ok := r.scenarios[result.Scenario]
	if !ok {
		samples = newScenarioSamples()
		r.scenarios[result.Scenario] = samples
	}
	samples.add(result)
	r.overall.add(result)
}

// LatencyStats holds the distribution numbers reported for a scenario or an
// aggregate of scenarios.
type LatencyStats struct {
	Min    time.Duration `json:"min"`
	Mean   time.Duration `json:"mean"`
	P50    time.Duration `json:"p50"`
	P90    time.Duration `json:"p90"`
	P95    time.Duration `json:"p95"`
	P99    time.Duration `json:"p99"`
	Max    time.Duration `json:"max"`
	StdDev time.Duration `json:"stddev"`
}

// ScenarioStats is the per-scenario summary for one stage.
type ScenarioStats struct {
	Name        string           `json:"name"`
	Service     string           `json:"service,omitempty"`
	Requests    int64            `json:"requests"`
	Failures    int64            `json:"failures"`
	ErrorRate   float64          `json:"error_rate"`
	Throughput  float64          `json:"throughput_rps"`
	BytesTotal  int64            `json:"bytes_total"`
	Latency     LatencyStats     `json:"latency"`
	StatusCodes map[int]int64    `json:"status_codes,omitempty"`
	Errors      map[string]int64 `json:"errors,omitempty"`
}

// Snapshot converts the accumulated observations into per-scenario summaries.
// The elapsed duration is the measured window used to derive throughput.
func (r *Recorder) Snapshot(elapsed time.Duration) []ScenarioStats {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]ScenarioStats, 0, len(r.scenarios))
	for name, samples := range r.scenarios {
		out = append(out, samples.summary(name, elapsed))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Overall returns the exact stage-level summary across every scenario,
// computed from the combined sample set rather than from per-scenario
// summaries.
func (r *Recorder) Overall(name string, elapsed time.Duration) ScenarioStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.overall.summary(name, elapsed)
}

// summarize computes the distribution numbers for a latency sample set. The
// input slice is sorted in place, which is safe because the Recorder owns it.
func summarize(latencies []time.Duration) LatencyStats {
	if len(latencies) == 0 {
		return LatencyStats{}
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	var sum float64
	for _, latency := range latencies {
		sum += float64(latency)
	}
	mean := sum / float64(len(latencies))

	var variance float64
	for _, latency := range latencies {
		delta := float64(latency) - mean
		variance += delta * delta
	}
	variance /= float64(len(latencies))

	return LatencyStats{
		Min:    latencies[0],
		Mean:   time.Duration(mean),
		P50:    percentile(latencies, 0.50),
		P90:    percentile(latencies, 0.90),
		P95:    percentile(latencies, 0.95),
		P99:    percentile(latencies, 0.99),
		Max:    latencies[len(latencies)-1],
		StdDev: time.Duration(math.Sqrt(variance)),
	}
}

// percentile returns the nearest-rank percentile of an already sorted sample
// set. Nearest-rank is used rather than interpolation so that every reported
// number is a latency the system actually produced.
func percentile(sorted []time.Duration, quantile float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if quantile <= 0 {
		return sorted[0]
	}
	if quantile >= 1 {
		return sorted[len(sorted)-1]
	}
	rank := int(math.Ceil(quantile * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

package perf

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ResourceSample is one observation of a container's resource usage.
type ResourceSample struct {
	At            time.Time `json:"at"`
	Container     string    `json:"container"`
	CPUPercent    float64   `json:"cpu_percent"`
	MemBytes      float64   `json:"mem_bytes"`
	MemLimitBytes float64   `json:"mem_limit_bytes"`
	MemPercent    float64   `json:"mem_percent"`
	PIDs          int       `json:"pids"`
}

// ContainerUsage summarizes one container's resource usage over a time window,
// normally a single load stage.
type ContainerUsage struct {
	Container     string  `json:"container"`
	Samples       int     `json:"samples"`
	CPUAvg        float64 `json:"cpu_avg_percent"`
	CPUPeak       float64 `json:"cpu_peak_percent"`
	MemAvgBytes   float64 `json:"mem_avg_bytes"`
	MemPeakBytes  float64 `json:"mem_peak_bytes"`
	MemLimitBytes float64 `json:"mem_limit_bytes"`
	MemPeakPct    float64 `json:"mem_peak_percent"`
}

// StatsCollector returns one round of container statistics. It is an interface
// point so the sampler can be tested without Docker present.
type StatsCollector func(ctx context.Context, containers []string) ([]ResourceSample, error)

// Sampler periodically records container resource usage for the duration of a
// test so that each load stage can be annotated with what the services were
// actually doing.
type Sampler struct {
	interval   time.Duration
	containers []string
	collect    StatsCollector

	mu      sync.Mutex
	samples []ResourceSample
	errs    []string

	cancel context.CancelFunc
	done   chan struct{}
}

// NewSampler returns a Sampler that shells out to `docker stats`. Passing a nil
// collector selects the Docker implementation.
func NewSampler(interval time.Duration, containers []string, collect StatsCollector) *Sampler {
	if collect == nil {
		collect = DockerStats
	}
	return &Sampler{interval: interval, containers: containers, collect: collect}
}

// Start begins sampling in the background until Stop is called or ctx ends. It
// is a no-op when no containers were configured.
func (s *Sampler) Start(ctx context.Context) {
	if s == nil || len(s.containers) == 0 || s.done != nil {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})

	go func() {
		defer close(s.done)
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		s.sampleOnce(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.sampleOnce(ctx)
			}
		}
	}()
}

func (s *Sampler) sampleOnce(ctx context.Context) {
	samples, err := s.collect(ctx, s.containers)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		// Sampling failures must never abort a load test; they are reported
		// alongside the results so a missing resource section is explainable.
		if ctx.Err() == nil {
			s.errs = append(s.errs, err.Error())
		}
		return
	}
	s.samples = append(s.samples, samples...)
}

// Stop ends sampling and waits for the background goroutine to finish.
func (s *Sampler) Stop() {
	if s == nil || s.done == nil {
		return
	}
	s.cancel()
	<-s.done
	s.done = nil
}

// Samples returns a copy of everything recorded so far.
func (s *Sampler) Samples() []ResourceSample {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ResourceSample, len(s.samples))
	copy(out, s.samples)
	return out
}

// Errors returns the distinct sampling failures observed during the run.
func (s *Sampler) Errors() []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]struct{}, len(s.errs))
	out := make([]string, 0, len(s.errs))
	for _, message := range s.errs {
		if _, ok := seen[message]; ok {
			continue
		}
		seen[message] = struct{}{}
		out = append(out, message)
	}
	return out
}

// UsageBetween summarizes per-container usage for samples taken within
// [start, end]. Results are sorted by descending average CPU so the busiest
// service appears first.
func UsageBetween(samples []ResourceSample, start, end time.Time) []ContainerUsage {
	grouped := make(map[string]*ContainerUsage)
	sums := make(map[string]*struct{ cpu, mem float64 })

	for _, sample := range samples {
		if sample.At.Before(start) || sample.At.After(end) {
			continue
		}
		usage, ok := grouped[sample.Container]
		if !ok {
			usage = &ContainerUsage{Container: sample.Container}
			grouped[sample.Container] = usage
			sums[sample.Container] = &struct{ cpu, mem float64 }{}
		}
		usage.Samples++
		sums[sample.Container].cpu += sample.CPUPercent
		sums[sample.Container].mem += sample.MemBytes
		usage.CPUPeak = math.Max(usage.CPUPeak, sample.CPUPercent)
		usage.MemPeakBytes = math.Max(usage.MemPeakBytes, sample.MemBytes)
		usage.MemPeakPct = math.Max(usage.MemPeakPct, sample.MemPercent)
		if sample.MemLimitBytes > 0 {
			usage.MemLimitBytes = sample.MemLimitBytes
		}
	}

	out := make([]ContainerUsage, 0, len(grouped))
	for name, usage := range grouped {
		if usage.Samples > 0 {
			usage.CPUAvg = sums[name].cpu / float64(usage.Samples)
			usage.MemAvgBytes = sums[name].mem / float64(usage.Samples)
		}
		out = append(out, *usage)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CPUAvg != out[j].CPUAvg {
			return out[i].CPUAvg > out[j].CPUAvg
		}
		return out[i].Container < out[j].Container
	})
	return out
}

// dockerStatsLine matches the JSON document `docker stats --format {{json .}}`
// emits per container.
type dockerStatsLine struct {
	Name     string `json:"Name"`
	CPUPerc  string `json:"CPUPerc"`
	MemUsage string `json:"MemUsage"`
	MemPerc  string `json:"MemPerc"`
	PIDs     string `json:"PIDs"`
}

// containerNamePattern is Docker's own container name grammar. Names are
// rejected rather than escaped because a name is either a valid Docker
// reference or it is not, and anything outside this grammar could reach the
// docker CLI as an option instead of an operand.
var containerNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// validateContainerNames rejects names that Docker itself would not accept. The
// leading-character rule is what keeps a configured name such as "--format"
// from being parsed as a flag by the docker CLI.
func validateContainerNames(containers []string) error {
	for _, container := range containers {
		if !containerNamePattern.MatchString(container) {
			return fmt.Errorf("invalid container name %q", container)
		}
	}
	return nil
}

// dockerStatsOutput runs one `docker stats` invocation for the given containers.
// Every name is validated first, so the variable arguments below are known to be
// plain operands.
func dockerStatsOutput(ctx context.Context, containers []string) ([]byte, error) {
	if err := validateContainerNames(containers); err != nil {
		return nil, err
	}
	args := append([]string{"stats", "--no-stream", "--format", "{{json .}}"}, containers...)
	// #nosec G204 -- fixed binary and subcommand; every operand is validated
	// against containerNamePattern above and no shell is involved.
	return exec.CommandContext(ctx, "docker", args...).Output()
}

// DockerStats collects one round of statistics from the Docker daemon. Only the
// requested containers are queried, and containers that are not running are
// skipped rather than failing the round.
func DockerStats(ctx context.Context, containers []string) ([]ResourceSample, error) {
	output, err := dockerStatsOutput(ctx, containers)
	if err != nil {
		if invalidErr := validateContainerNames(containers); invalidErr != nil {
			return nil, fmt.Errorf("docker stats: %w", invalidErr)
		}
		// A container that is not running makes the whole invocation fail, so
		// retry against the set that Docker currently knows about.
		running, listErr := runningContainers(ctx, containers)
		if listErr != nil || len(running) == 0 {
			return nil, fmt.Errorf("docker stats: %w", err)
		}
		output, err = dockerStatsOutput(ctx, running)
		if err != nil {
			return nil, fmt.Errorf("docker stats: %w", err)
		}
	}
	return ParseDockerStats(time.Now(), output)
}

// runningContainers filters the requested names down to those Docker reports as
// currently running.
func runningContainers(ctx context.Context, containers []string) ([]string, error) {
	output, err := exec.CommandContext(ctx, "docker", "ps", "--format", "{{.Names}}").Output()
	if err != nil {
		return nil, err
	}
	live := make(map[string]struct{})
	for _, line := range strings.Split(string(output), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			live[name] = struct{}{}
		}
	}
	out := make([]string, 0, len(containers))
	for _, name := range containers {
		if _, ok := live[name]; ok {
			out = append(out, name)
		}
	}
	return out, nil
}

// ParseDockerStats converts newline-delimited `docker stats` JSON into samples.
func ParseDockerStats(at time.Time, output []byte) ([]ResourceSample, error) {
	var samples []ResourceSample
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var parsed dockerStatsLine
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return nil, fmt.Errorf("parse docker stats line %q: %w", trimmed, err)
		}
		used, limit := parseMemUsage(parsed.MemUsage)
		pids, _ := strconv.Atoi(strings.TrimSpace(parsed.PIDs))
		samples = append(samples, ResourceSample{
			At:            at,
			Container:     parsed.Name,
			CPUPercent:    parsePercent(parsed.CPUPerc),
			MemBytes:      used,
			MemLimitBytes: limit,
			MemPercent:    parsePercent(parsed.MemPerc),
			PIDs:          pids,
		})
	}
	return samples, nil
}

// parsePercent converts a Docker percentage string such as "12.34%" to a float.
func parsePercent(raw string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), "%")), 64)
	if err != nil {
		return 0
	}
	return value
}

// parseMemUsage converts a Docker memory string such as "12.3MiB / 7.66GiB"
// into used and limit byte counts.
func parseMemUsage(raw string) (used, limit float64) {
	parts := strings.Split(raw, "/")
	if len(parts) > 0 {
		used = parseByteSize(parts[0])
	}
	if len(parts) > 1 {
		limit = parseByteSize(parts[1])
	}
	return used, limit
}

var byteUnits = []struct {
	suffix string
	factor float64
}{
	{"PiB", 1 << 50}, {"TiB", 1 << 40}, {"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10},
	{"PB", 1e15}, {"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"KB", 1e3},
	{"B", 1},
}

// parseByteSize converts a Docker human-readable size to bytes.
func parseByteSize(raw string) float64 {
	trimmed := strings.TrimSpace(raw)
	for _, unit := range byteUnits {
		if !strings.HasSuffix(trimmed, unit.suffix) {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(trimmed, unit.suffix)), 64)
		if err != nil {
			return 0
		}
		return value * unit.factor
	}
	return 0
}

// FormatBytes renders a byte count using binary units for report output.
func FormatBytes(value float64) string {
	switch {
	case value >= 1<<30:
		return fmt.Sprintf("%.2fGiB", value/(1<<30))
	case value >= 1<<20:
		return fmt.Sprintf("%.1fMiB", value/(1<<20))
	case value >= 1<<10:
		return fmt.Sprintf("%.1fKiB", value/(1<<10))
	default:
		return fmt.Sprintf("%.0fB", value)
	}
}

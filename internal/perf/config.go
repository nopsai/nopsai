// Package perf implements the nopsai backend performance and load-test
// harness. It drives configurable request mixes against the platform HTTP
// surfaces at increasing concurrency levels, samples per-service container
// resource usage while the load runs, and reports the latency, throughput and
// saturation numbers that describe how the system behaves under load.
package perf

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Default endpoints match the docker-compose topology shipped in
// docker-compose.yaml, where the API is published on 8080 and git-bot on 8081.
const (
	DefaultAPIURL        = "http://127.0.0.1:8080"
	DefaultWebhookURL    = "http://127.0.0.1:8081/webhook"
	DefaultUIURL         = "http://127.0.0.1:80/"
	DefaultAdminEmail    = "admin@example.com"
	DefaultAdminPassword = "admin"
	// DefaultExternalTriggerID matches the fixture in
	// test/perf/fixtures/external-triggers/perf-load-probe.yaml.
	DefaultExternalTriggerID = "perf-load-probe"
)

// Config is the fully resolved input to a performance test. Every field is
// validated by Validate before a run starts so that a misconfigured harness
// fails immediately instead of producing meaningless numbers.
type Config struct {
	// Targets.
	APIURL     string
	WebhookURL string
	UIURL      string

	// RuntimeRunIDs pins the runs the runtime suite writes against. When empty
	// the harness discovers recent runs from the API.
	RuntimeRunIDs []string
	// RuntimeRunCount is how many runs to spread runtime load across when
	// discovering them. Spreading avoids concentrating every write on one row.
	RuntimeRunCount int

	// Credentials used to obtain an access token and to sign webhook bodies.
	Identifier    string
	Password      string
	WebhookSecret string
	PayloadFile   string
	// WebhookInstallationID overrides installation.id in the payload. git-bot
	// rejects a delivery whose installation is not registered, and the shipped
	// sample payload carries a placeholder, so a real environment needs its own.
	WebhookInstallationID string

	// Suites selects which scenario suites take part in the run.
	Suites []string

	// Load shape. The harness is closed-loop: each stage runs Concurrency
	// workers that issue requests back to back, so the achieved throughput is
	// the system's answer rather than an offered rate.
	Concurrency    []int
	StageDuration  time.Duration
	WarmupDuration time.Duration
	RequestTimeout time.Duration

	// End-to-end pipeline suite settings. Pipeline runs take minutes rather
	// than milliseconds, so they get their own in-flight limits and timeouts.
	PipelineConcurrency []int
	PipelineIterations  int
	PipelineTimeout     time.Duration
	PipelinePollEvery   time.Duration
	// PipelineFirstRunTimeout bounds the wait for a run to become visible after
	// a webhook is accepted. It is deliberately separate from PipelineTimeout:
	// a run that never appears means the trigger did not produce work, which is
	// a configuration answer available in seconds, while a run that appears and
	// then executes legitimately takes minutes.
	PipelineFirstRunTimeout time.Duration
	// PipelineTrigger selects how runs are started: through the purpose-built
	// external trigger (self-contained) or by replaying a git webhook (measures
	// the real ingestion path, but needs a payload for a real commit).
	PipelineTrigger string
	// ExternalTriggerID is the trigger the pipeline suite invokes.
	ExternalTriggerID string
	// PipelineWorkSeconds is the synthetic work the probe pipeline performs.
	// Zero measures pure orchestration overhead.
	PipelineWorkSeconds int

	// Resource sampling.
	SampleInterval time.Duration
	Containers     []string

	// Pass/fail thresholds used to derive the recommended operating point.
	LatencySLO  time.Duration
	ErrorBudget float64
	// AbortErrorRate stops the ramp once a stage fails this badly. Levels past
	// total collapse cost time and add no information. Zero disables the check.
	AbortErrorRate float64

	// Output.
	OutputDir string
	JSONPath  string
}

// DefaultConfig returns a configuration that produces a meaningful run against
// a local docker-compose stack without any flags being supplied.
func DefaultConfig() Config {
	return Config{
		APIURL:                  DefaultAPIURL,
		WebhookURL:              DefaultWebhookURL,
		UIURL:                   DefaultUIURL,
		RuntimeRunCount:         5,
		Identifier:              DefaultAdminEmail,
		Password:                DefaultAdminPassword,
		PayloadFile:             "doc/sample-git-event.json",
		Suites:                  []string{SuiteAPIRead, SuiteAuth, SuiteRuntime, SuiteUI},
		Concurrency:             []int{1, 2, 5, 10, 25, 50, 100},
		StageDuration:           30 * time.Second,
		WarmupDuration:          5 * time.Second,
		RequestTimeout:          30 * time.Second,
		PipelineConcurrency:     []int{1, 3, 5},
		PipelineIterations:      1,
		PipelineTimeout:         10 * time.Minute,
		PipelinePollEvery:       2 * time.Second,
		PipelineFirstRunTimeout: 60 * time.Second,
		PipelineTrigger:         TriggerExternal,
		ExternalTriggerID:       DefaultExternalTriggerID,
		PipelineWorkSeconds:     2,
		SampleInterval:          2 * time.Second,
		Containers:              DefaultContainers(),
		LatencySLO:              time.Second,
		ErrorBudget:             0.01,
		AbortErrorRate:          0.5,
		OutputDir:               "test/perf/results",
	}
}

// DefaultContainers lists the compose container names whose resource usage is
// attributed to each load stage. Names mirror the container_name entries in
// docker-compose.yaml.
func DefaultContainers() []string {
	return []string{
		"nopsai",
		"nopsai-db",
		"nopsai-aaa",
		"nopsai-dispatcher",
		"nopsai-git-bot",
		"nopsai-ui",
	}
}

// ApplyEnv fills unset credential fields from the same environment variables
// the compose stack uses, so the harness works with an already-configured shell
// without repeating secrets on the command line.
func (c *Config) ApplyEnv(lookup func(string) (string, bool)) {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	if v, ok := lookup("NOPSAI_BOOTSTRAP_ADMIN_PASSWORD"); ok && strings.TrimSpace(v) != "" {
		c.Password = v
	}
	if v, ok := lookup("NOPSAI_PERF_PASSWORD"); ok && strings.TrimSpace(v) != "" {
		c.Password = v
	}
	if v, ok := lookup("NOPSAI_PERF_IDENTIFIER"); ok && strings.TrimSpace(v) != "" {
		c.Identifier = v
	}
	if v, ok := lookup("GITHUB_WEBHOOK_SECRET"); ok && strings.TrimSpace(v) != "" {
		c.WebhookSecret = v
	}
	if v, ok := lookup("NOPSAI_PERF_WEBHOOK_SECRET"); ok && strings.TrimSpace(v) != "" {
		c.WebhookSecret = v
	}
	if v, ok := lookup("NOPSAI_API_URL"); ok && strings.TrimSpace(v) != "" {
		c.APIURL = v
	}
}

// Validate reports the first configuration problem that would make the run
// produce misleading results.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.APIURL) == "" {
		return fmt.Errorf("api url is required")
	}
	if len(c.Suites) == 0 {
		return fmt.Errorf("at least one suite is required")
	}
	for _, suite := range c.Suites {
		if !KnownSuite(suite) {
			return fmt.Errorf("unknown suite %q (known suites: %s)", suite, strings.Join(SuiteNames(), ", "))
		}
	}
	if c.SelectsHTTPSuite() {
		if len(c.Concurrency) == 0 {
			return fmt.Errorf("at least one concurrency level is required")
		}
		for _, level := range c.Concurrency {
			if level <= 0 {
				return fmt.Errorf("concurrency levels must be positive, got %d", level)
			}
		}
		if c.StageDuration <= 0 {
			return fmt.Errorf("stage duration must be positive")
		}
		if c.WarmupDuration < 0 {
			return fmt.Errorf("warmup duration cannot be negative")
		}
		if c.WarmupDuration >= c.StageDuration {
			return fmt.Errorf("warmup duration (%s) must be shorter than stage duration (%s)", c.WarmupDuration, c.StageDuration)
		}
	}
	if c.NeedsWebhookPayload() {
		if strings.TrimSpace(c.WebhookSecret) == "" {
			return fmt.Errorf("webhook secret is required for the %q suite and for --pipeline-trigger %s", SuiteWebhook, TriggerWebhook)
		}
		if strings.TrimSpace(c.PayloadFile) == "" {
			return fmt.Errorf("payload file is required for the %q suite and for --pipeline-trigger %s", SuiteWebhook, TriggerWebhook)
		}
	}
	if c.Contains(SuiteRuntime) && c.RuntimeRunCount <= 0 && len(c.RuntimeRunIDs) == 0 {
		return fmt.Errorf("the %q suite needs at least one target run", SuiteRuntime)
	}
	if c.Contains(SuiteUI) && strings.TrimSpace(c.UIURL) == "" {
		return fmt.Errorf("ui url is required for the %q suite", SuiteUI)
	}
	if c.Contains(SuitePipeline) {
		if len(c.PipelineConcurrency) == 0 {
			return fmt.Errorf("at least one pipeline concurrency level is required")
		}
		for _, level := range c.PipelineConcurrency {
			if level <= 0 {
				return fmt.Errorf("pipeline concurrency levels must be positive, got %d", level)
			}
		}
		if c.PipelineIterations <= 0 {
			return fmt.Errorf("pipeline iterations must be positive")
		}
		if c.PipelineTimeout <= 0 {
			return fmt.Errorf("pipeline timeout must be positive")
		}
		if c.PipelinePollEvery <= 0 {
			return fmt.Errorf("pipeline poll interval must be positive")
		}
		if c.PipelineFirstRunTimeout <= 0 {
			return fmt.Errorf("pipeline first-run timeout must be positive")
		}
		if !KnownTriggerMode(c.PipelineTrigger) {
			return fmt.Errorf("unknown pipeline trigger %q (known modes: %s)", c.PipelineTrigger, strings.Join(TriggerModes(), ", "))
		}
		if c.PipelineTrigger == TriggerExternal && strings.TrimSpace(c.ExternalTriggerID) == "" {
			return fmt.Errorf("external trigger id is required for --pipeline-trigger %s", TriggerExternal)
		}
		if c.PipelineWorkSeconds < 0 {
			return fmt.Errorf("pipeline work seconds cannot be negative")
		}
		if c.PipelineFirstRunTimeout > c.PipelineTimeout {
			return fmt.Errorf("pipeline first-run timeout (%s) cannot exceed the overall pipeline timeout (%s)",
				c.PipelineFirstRunTimeout, c.PipelineTimeout)
		}
	}
	if c.RequestTimeout <= 0 {
		return fmt.Errorf("request timeout must be positive")
	}
	if c.SampleInterval <= 0 {
		return fmt.Errorf("resource sample interval must be positive")
	}
	if c.LatencySLO <= 0 {
		return fmt.Errorf("latency SLO must be positive")
	}
	if c.ErrorBudget < 0 || c.ErrorBudget > 1 {
		return fmt.Errorf("error budget must be a ratio between 0 and 1, got %v", c.ErrorBudget)
	}
	if c.AbortErrorRate < 0 || c.AbortErrorRate > 1 {
		return fmt.Errorf("abort error rate must be a ratio between 0 and 1, got %v", c.AbortErrorRate)
	}
	return nil
}

// Contains reports whether the named suite takes part in the run.
func (c *Config) Contains(suite string) bool {
	for _, candidate := range c.Suites {
		if candidate == suite {
			return true
		}
	}
	return false
}

// NeedsWebhookPayload reports whether the run has to load and sign a git event
// payload. The pipeline suite only needs one when it is driving runs through the
// webhook trigger; its default external trigger needs nothing.
func (c *Config) NeedsWebhookPayload() bool {
	if c.Contains(SuiteWebhook) {
		return true
	}
	return c.Contains(SuitePipeline) && c.PipelineTrigger == TriggerWebhook
}

// SelectsHTTPSuite reports whether any short-request suite is selected. The
// pipeline suite is excluded because it uses its own stage shape.
func (c *Config) SelectsHTTPSuite() bool {
	for _, suite := range c.Suites {
		if suite != SuitePipeline {
			return true
		}
	}
	return false
}

// NormalizedConcurrency returns the configured levels sorted ascending with
// duplicates removed, so a ramp always moves in one direction.
func NormalizedConcurrency(levels []int) []int {
	seen := make(map[int]struct{}, len(levels))
	out := make([]int, 0, len(levels))
	for _, level := range levels {
		if _, ok := seen[level]; ok {
			continue
		}
		seen[level] = struct{}{}
		out = append(out, level)
	}
	sort.Ints(out)
	return out
}

// ParseIntList parses a comma-separated list of positive integers such as
// "1,5,10,25" into a concurrency ramp.
func ParseIntList(raw string) ([]int, error) {
	fields := strings.Split(raw, ",")
	out := make([]int, 0, len(fields))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			continue
		}
		value, err := strconv.Atoi(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q: %w", trimmed, err)
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no values parsed from %q", raw)
	}
	return out, nil
}

// ParseStringList parses a comma-separated list, trimming blanks.
func ParseStringList(raw string) []string {
	fields := strings.Split(raw, ",")
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

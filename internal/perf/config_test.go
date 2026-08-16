package perf

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfigIsValid(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WebhookSecret = "secret"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("DefaultConfig is not valid: %v", err)
	}
}

func TestValidateRejectsMisconfiguration(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"missing api url", func(c *Config) { c.APIURL = "" }, "api url is required"},
		{"no suites", func(c *Config) { c.Suites = nil }, "at least one suite is required"},
		{"unknown suite", func(c *Config) { c.Suites = []string{"nope"} }, `unknown suite "nope"`},
		{"zero concurrency", func(c *Config) { c.Concurrency = []int{0} }, "concurrency levels must be positive"},
		{"empty concurrency", func(c *Config) { c.Concurrency = nil }, "at least one concurrency level is required"},
		{"zero stage duration", func(c *Config) { c.StageDuration = 0 }, "stage duration must be positive"},
		{"negative warmup", func(c *Config) { c.WarmupDuration = -time.Second }, "warmup duration cannot be negative"},
		{"warmup swallows stage", func(c *Config) { c.WarmupDuration = c.StageDuration }, "must be shorter than stage duration"},
		{"zero request timeout", func(c *Config) { c.RequestTimeout = 0 }, "request timeout must be positive"},
		{"zero sample interval", func(c *Config) { c.SampleInterval = 0 }, "resource sample interval must be positive"},
		{"zero latency slo", func(c *Config) { c.LatencySLO = 0 }, "latency SLO must be positive"},
		{"error budget above one", func(c *Config) { c.ErrorBudget = 1.5 }, "error budget must be a ratio"},
		{"abort rate below zero", func(c *Config) { c.AbortErrorRate = -1 }, "abort error rate must be a ratio"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.WebhookSecret = "secret"
			testCase.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("Validate() succeeded, want an error containing %q", testCase.want)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("Validate() = %q, want it to contain %q", err, testCase.want)
			}
		})
	}
}

// TestValidateRequiresWebhookSecretOnlyWhenNeeded guards the property that a
// pure read-path run must not demand webhook credentials it will never use.
func TestValidateRequiresWebhookSecretOnlyWhenNeeded(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Suites = []string{SuiteAPIRead, SuiteAuth}
	cfg.WebhookSecret = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("read-only suites should not require a webhook secret, got %v", err)
	}

	cfg.Suites = []string{SuiteWebhook}
	if err := cfg.Validate(); err == nil {
		t.Fatal("the webhook suite should require a webhook secret")
	}
}

func TestValidatePipelineSettings(t *testing.T) {
	base := func() Config {
		cfg := DefaultConfig()
		cfg.Suites = []string{SuitePipeline}
		cfg.WebhookSecret = "secret"
		return cfg
	}
	for _, testCase := range []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"no levels", func(c *Config) { c.PipelineConcurrency = nil }, "at least one pipeline concurrency level"},
		{"zero level", func(c *Config) { c.PipelineConcurrency = []int{0} }, "pipeline concurrency levels must be positive"},
		{"zero iterations", func(c *Config) { c.PipelineIterations = 0 }, "pipeline iterations must be positive"},
		{"zero timeout", func(c *Config) { c.PipelineTimeout = 0 }, "pipeline timeout must be positive"},
		{"zero poll", func(c *Config) { c.PipelinePollEvery = 0 }, "pipeline poll interval must be positive"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := base()
			testCase.mutate(&cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("Validate() = %v, want an error containing %q", err, testCase.want)
			}
		})
	}
}

// TestValidateSkipsRequestRampChecksForPipelineOnlyRuns covers the case where
// the pipeline suite runs alone: it has its own stage shape, so the HTTP ramp
// settings must not be enforced.
func TestValidateSkipsRequestRampChecksForPipelineOnlyRuns(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Suites = []string{SuitePipeline}
	cfg.WebhookSecret = "secret"
	cfg.Concurrency = nil
	cfg.StageDuration = 0
	if err := cfg.Validate(); err != nil {
		t.Fatalf("pipeline-only run should not require request ramp settings, got %v", err)
	}
}

func TestApplyEnvOverridesCredentials(t *testing.T) {
	env := map[string]string{
		"NOPSAI_BOOTSTRAP_ADMIN_PASSWORD": "bootstrap-secret",
		"NOPSAI_PERF_IDENTIFIER":          "perf@example.com",
		"GITHUB_WEBHOOK_SECRET":           "hook",
		"NOPSAI_API_URL":                  "http://api.internal:8080",
	}
	cfg := DefaultConfig()
	cfg.ApplyEnv(func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	})

	if cfg.Password != "bootstrap-secret" {
		t.Errorf("Password = %q, want the bootstrap password", cfg.Password)
	}
	if cfg.Identifier != "perf@example.com" {
		t.Errorf("Identifier = %q, want the env identifier", cfg.Identifier)
	}
	if cfg.WebhookSecret != "hook" {
		t.Errorf("WebhookSecret = %q, want the env secret", cfg.WebhookSecret)
	}
	if cfg.APIURL != "http://api.internal:8080" {
		t.Errorf("APIURL = %q, want the env API URL", cfg.APIURL)
	}
}

// TestApplyEnvPrefersPerfSpecificPassword documents the precedence: the
// harness-specific variable wins over the shared bootstrap one, so a perf run
// can target a dedicated account without changing the stack's own settings.
func TestApplyEnvPrefersPerfSpecificPassword(t *testing.T) {
	env := map[string]string{
		"NOPSAI_BOOTSTRAP_ADMIN_PASSWORD": "bootstrap-secret",
		"NOPSAI_PERF_PASSWORD":            "perf-secret",
	}
	cfg := DefaultConfig()
	cfg.ApplyEnv(func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	})
	if cfg.Password != "perf-secret" {
		t.Fatalf("Password = %q, want the perf-specific password to win", cfg.Password)
	}
}

func TestApplyEnvIgnoresBlankValues(t *testing.T) {
	cfg := DefaultConfig()
	original := cfg.Password
	cfg.ApplyEnv(func(string) (string, bool) { return "   ", true })
	if cfg.Password != original {
		t.Fatalf("Password = %q, want blank env values to be ignored", cfg.Password)
	}
}

func TestNormalizedConcurrencySortsAndDeduplicates(t *testing.T) {
	got := NormalizedConcurrency([]int{10, 1, 10, 5, 1})
	want := []int{1, 5, 10}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizedConcurrency = %v, want %v", got, want)
	}
}

func TestParseIntList(t *testing.T) {
	got, err := ParseIntList(" 1, 5 ,10, ")
	if err != nil {
		t.Fatalf("ParseIntList returned %v", err)
	}
	if want := []int{1, 5, 10}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseIntList = %v, want %v", got, want)
	}
	if _, err := ParseIntList("1,x"); err == nil {
		t.Error("ParseIntList should reject non-numeric entries")
	}
	if _, err := ParseIntList("  ,  "); err == nil {
		t.Error("ParseIntList should reject an all-blank list")
	}
}

func TestParseStringListTrimsBlanks(t *testing.T) {
	got := ParseStringList(" a , ,b ")
	if want := []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseStringList = %v, want %v", got, want)
	}
}

func TestSelectsHTTPSuite(t *testing.T) {
	pipelineOnly := Config{Suites: []string{SuitePipeline}}
	if pipelineOnly.SelectsHTTPSuite() {
		t.Error("a pipeline-only run should not select an HTTP suite")
	}
	mixed := Config{Suites: []string{SuitePipeline, SuiteAPIRead}}
	if !mixed.SelectsHTTPSuite() {
		t.Error("a run including api-read should select an HTTP suite")
	}
}

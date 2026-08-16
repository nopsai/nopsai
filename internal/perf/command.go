package perf

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

// NewCommand builds the nopsai-perf command tree.
func NewCommand(version string) *cobra.Command {
	cfg := DefaultConfig()
	var (
		suites        string
		concurrency   string
		pipelineConc  string
		containers    string
		runtimeRunIDs string
		failOnBreach  bool
		skipResources bool
		quiet         bool
	)

	cmd := &cobra.Command{
		Use:     "nopsai-perf",
		Short:   "Load-test the nopsai backend and report its performance envelope",
		Version: version,
		Long: strings.TrimSpace(`
Drives the nopsai backend at increasing concurrency levels and reports how
throughput, latency and per-service resource usage move together, then names the
safe operating point, the saturation knee and the busiest service.

The harness is closed-loop: each stage runs N workers that issue requests back
to back, so the reported request rate is what the system completed rather than a
rate that was forced onto it.

The load-bearing services are nopsai (API), aaa (auth), the dispatcher, Postgres
and the UI. Results are reported per service, so a run answers which one carried
the most load, which degraded least, and which gave out first.

Suites:
  api-read   run listings, monitoring aggregates, pipeline and team queries
  auth       login, token-bearing calls and authorization checks through aaa
  runtime    the telemetry a running pipeline emits: log ingest, status, log reads
  ui         the UI container serving static assets
  webhook    signed git webhook ingestion and dispatch enqueue
  pipeline   whole pipelines end to end, measured in runs and minutes

Requires a running stack: docker compose up --build -d`),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg.ApplyEnv(os.LookupEnv)

			cfg.Suites = ParseStringList(suites)
			levels, err := ParseIntList(concurrency)
			if err != nil {
				return fmt.Errorf("--concurrency: %w", err)
			}
			cfg.Concurrency = levels

			pipelineLevels, err := ParseIntList(pipelineConc)
			if err != nil {
				return fmt.Errorf("--pipeline-concurrency: %w", err)
			}
			cfg.PipelineConcurrency = pipelineLevels

			if strings.TrimSpace(runtimeRunIDs) != "" {
				cfg.RuntimeRunIDs = ParseStringList(runtimeRunIDs)
			}
			cfg.Containers = ParseStringList(containers)
			if skipResources {
				cfg.Containers = nil
			}

			progress := cmd.ErrOrStderr()
			if quiet {
				progress = nil
			}

			report, err := Run(cmd.Context(), cfg, progress)
			if report == nil {
				return err
			}
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "run ended early: %v\n", err)
			}

			if writeErr := report.WriteText(cmd.OutOrStdout()); writeErr != nil {
				return writeErr
			}
			textPath, jsonPath, writeErr := WriteArtifacts(report, cfg.OutputDir)
			if writeErr != nil {
				return writeErr
			}
			if textPath != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "\nreports written to %s and %s\n", textPath, jsonPath)
			}
			if err != nil {
				return err
			}
			if failOnBreach && !report.Analysis.RecommendedFound && len(report.Stages) > 0 {
				return fmt.Errorf("no concurrency level met the p95 %s / %.2f%% error thresholds",
					cfg.LatencySLO, cfg.ErrorBudget*100)
			}
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&cfg.APIURL, "api-url", cfg.APIURL, "base URL of the nopsai API")
	flags.StringVar(&cfg.UIURL, "ui-url", cfg.UIURL, "URL of the UI container, loaded by the ui suite")
	flags.IntVar(&cfg.RuntimeRunCount, "runtime-runs", cfg.RuntimeRunCount, "how many existing runs the runtime suite spreads its writes across")
	flags.StringVar(&runtimeRunIDs, "runtime-run-ids", "", "comma-separated run ids for the runtime suite (default: discover recent runs)")
	flags.StringVar(&cfg.WebhookURL, "webhook-url", cfg.WebhookURL, "git-bot webhook URL used by the webhook and pipeline suites")
	flags.StringVar(&suites, "suites", strings.Join(cfg.Suites, ","), "comma-separated suites to run ("+strings.Join(SuiteNames(), ", ")+")")
	flags.StringVar(&concurrency, "concurrency", intsToString(cfg.Concurrency), "comma-separated concurrency ramp for the request suites")
	flags.DurationVar(&cfg.StageDuration, "stage-duration", cfg.StageDuration, "wall-clock duration of each concurrency stage")
	flags.DurationVar(&cfg.WarmupDuration, "warmup", cfg.WarmupDuration, "leading part of each stage excluded from the measurements")
	flags.DurationVar(&cfg.RequestTimeout, "request-timeout", cfg.RequestTimeout, "per-request timeout")

	flags.StringVar(&cfg.Identifier, "identifier", cfg.Identifier, "login identifier (env: NOPSAI_PERF_IDENTIFIER)")
	flags.StringVar(&cfg.Password, "password", cfg.Password, "login password (env: NOPSAI_PERF_PASSWORD or NOPSAI_BOOTSTRAP_ADMIN_PASSWORD)")
	flags.StringVar(&cfg.WebhookSecret, "webhook-secret", cfg.WebhookSecret, "HMAC secret for signed webhooks (env: GITHUB_WEBHOOK_SECRET)")
	flags.StringVar(&cfg.PayloadFile, "payload", cfg.PayloadFile, "git event payload sent by the webhook and pipeline suites")
	flags.StringVar(&cfg.WebhookInstallationID, "webhook-installation-id", cfg.WebhookInstallationID, "override installation.id in the payload; must be an installation registered in the platform")

	flags.StringVar(&pipelineConc, "pipeline-concurrency", intsToString(cfg.PipelineConcurrency), "comma-separated concurrent-family levels for the pipeline suite")
	flags.IntVar(&cfg.PipelineIterations, "pipeline-iterations", cfg.PipelineIterations, "how many times each pipeline concurrency level repeats")
	flags.DurationVar(&cfg.PipelineTimeout, "pipeline-timeout", cfg.PipelineTimeout, "maximum time to wait for one pipeline family to finish")
	flags.DurationVar(&cfg.PipelinePollEvery, "pipeline-poll", cfg.PipelinePollEvery, "how often run status is polled during the pipeline suite")
	flags.StringVar(&cfg.PipelineTrigger, "pipeline-trigger", cfg.PipelineTrigger, "how the pipeline suite starts runs ("+strings.Join(TriggerModes(), ", ")+")")
	flags.StringVar(&cfg.ExternalTriggerID, "external-trigger-id", cfg.ExternalTriggerID, "external trigger the pipeline suite invokes")
	flags.IntVar(&cfg.PipelineWorkSeconds, "pipeline-work-seconds", cfg.PipelineWorkSeconds, "synthetic work the probe pipeline performs per run; 0 measures pure orchestration overhead")
	flags.DurationVar(&cfg.PipelineFirstRunTimeout, "pipeline-first-run-timeout", cfg.PipelineFirstRunTimeout, "how long to wait for a triggered run to appear before giving up on it")

	flags.StringVar(&containers, "containers", strings.Join(cfg.Containers, ","), "container names sampled for CPU and memory")
	flags.DurationVar(&cfg.SampleInterval, "sample-interval", cfg.SampleInterval, "how often container resource usage is sampled")
	flags.BoolVar(&skipResources, "no-resources", false, "skip container resource sampling")

	flags.DurationVar(&cfg.LatencySLO, "latency-slo", cfg.LatencySLO, "p95 latency a stage must stay under to count as passing")
	flags.Float64Var(&cfg.ErrorBudget, "error-budget", cfg.ErrorBudget, "error ratio a stage must stay under to count as passing")
	flags.Float64Var(&cfg.AbortErrorRate, "abort-error-rate", cfg.AbortErrorRate, "error ratio at which the ramp stops early (0 disables)")

	flags.StringVar(&cfg.OutputDir, "output-dir", cfg.OutputDir, "directory that receives the text and JSON reports (empty to skip)")
	flags.BoolVar(&failOnBreach, "fail-on-breach", false, "exit non-zero when no concurrency level meets the thresholds")
	flags.BoolVar(&quiet, "quiet", false, "suppress per-stage progress output")

	return cmd
}

func intsToString(values []int) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%d", value))
	}
	return strings.Join(parts, ",")
}

// Execute runs the command with a context that cancels on interrupt, so a
// long ramp can be stopped with Ctrl-C and still report the stages that were
// already measured instead of discarding them.
func Execute(version string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return NewCommand(version).ExecuteContext(ctx)
}

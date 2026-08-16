package perf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Run executes a complete performance test and returns the report. Progress is
// written to progress as the run proceeds; the report itself is returned rather
// than printed so the caller decides where it goes.
func Run(ctx context.Context, cfg Config, progress io.Writer) (*Report, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	logf := func(format string, args ...any) {
		if progress == nil {
			return
		}
		fmt.Fprintf(progress, format+"\n", args...)
	}

	request := &RequestContext{
		APIURL:     strings.TrimSuffix(cfg.APIURL, "/"),
		WebhookURL: cfg.WebhookURL,
		UIURL:      cfg.UIURL,
		Identifier: cfg.Identifier,
		Password:   cfg.Password,
	}
	if cfg.NeedsWebhookPayload() {
		payload, err := os.ReadFile(cfg.PayloadFile)
		if err != nil {
			return nil, fmt.Errorf("read webhook payload %s: %w", cfg.PayloadFile, err)
		}
		payload, err = ApplyInstallationID(payload, cfg.WebhookInstallationID)
		if err != nil {
			return nil, err
		}
		request.Payload = payload
		request.Signature = SignPayload(cfg.WebhookSecret, payload)
	}

	peak := peakConcurrency(cfg)
	client := NewHTTPClient(cfg.RequestTimeout, peak)

	if err := preflight(ctx, client, request.APIURL); err != nil {
		return nil, err
	}
	logf("preflight: %s is reachable", request.APIURL)

	tokens := NewTokenManager(client, request.APIURL, cfg.Identifier, cfg.Password)
	if _, err := tokens.Login(ctx); err != nil {
		return nil, fmt.Errorf("authenticate as %s: %w\n"+
			"Set NOPSAI_PERF_IDENTIFIER and NOPSAI_PERF_PASSWORD (or NOPSAI_BOOTSTRAP_ADMIN_PASSWORD) to valid credentials.",
			cfg.Identifier, err)
	}
	request.TokenSource = tokens.Token
	logf("preflight: authenticated as %s", cfg.Identifier)

	if len(request.Payload) > 0 {
		if err := preflightWebhook(ctx, client, request); err != nil {
			return nil, err
		}
		logf("preflight: webhook signature accepted by %s", request.WebhookURL)
	}

	if cfg.Contains(SuiteRuntime) {
		targets, err := resolveRuntimeTargets(ctx, client, request, cfg)
		if err != nil {
			return nil, err
		}
		request.Runtime = targets
		logf("preflight: runtime suite will write against %d run(s)", targets.Len())
	}

	sampler := NewSampler(cfg.SampleInterval, cfg.Containers, nil)
	sampler.Start(ctx)
	defer sampler.Stop()

	report := &Report{
		StartedAt: time.Now(),
		Target:    request.APIURL,
		Suites:    cfg.Suites,
		Version:   platformVersion(ctx, client, request.APIURL),
		Thresholds: Thresholds{
			LatencySLO:  cfg.LatencySLO.String(),
			ErrorBudget: cfg.ErrorBudget,
		},
	}

	mix := BuildMix(cfg.Suites)
	if !mix.Empty() {
		logf("running %d scenario(s) across %d concurrency level(s)", mix.Len(), len(NormalizedConcurrency(cfg.Concurrency)))
		stages, err := NewRunner(cfg, client, mix, request, sampler, logf).Run(ctx)
		report.Stages = stages
		if err != nil {
			return report, err
		}
	}

	if cfg.Contains(SuitePipeline) {
		logf("running end-to-end pipeline suite")
		stages, err := NewPipelineRunner(cfg, client, request, sampler, logf).Run(ctx)
		report.PipelineStages = stages
		if err != nil {
			return report, err
		}
	}

	sampler.Stop()
	report.SamplingErrors = sampler.Errors()
	report.EndedAt = time.Now()
	report.Duration = report.EndedAt.Sub(report.StartedAt)
	report.Analysis = Analyze(report.Stages, cfg)
	report.ServiceCapacity = CompareServices(report.Stages, cfg)
	return report, nil
}

// peakConcurrency returns the largest number of workers any stage will use, so
// the connection pool can be sized once up front.
func peakConcurrency(cfg Config) int {
	peak := 1
	for _, level := range cfg.Concurrency {
		if level > peak {
			peak = level
		}
	}
	for _, level := range cfg.PipelineConcurrency {
		if level > peak {
			peak = level
		}
	}
	return peak
}

// preflight fails fast when the stack is not up, so an operator gets a clear
// message instead of a report full of connection refusals.
func preflight(ctx context.Context, client *http.Client, apiURL string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(apiURL, "/healthz"), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s is not reachable: %w\nStart the stack with: docker compose up --build -d", apiURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s/healthz returned status %d; the stack is not ready for a load test", apiURL, resp.StatusCode)
	}
	return nil
}

// preflightWebhook sends a single signed delivery before the ramp starts. A
// rejected signature would otherwise fail every webhook request for the whole
// run, so it is worth one request up front to turn a wasted ramp into an
// immediate, specific error.
func preflightWebhook(ctx context.Context, client *http.Client, request *RequestContext) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	deliveryID, err := newDeliveryID()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, request.WebhookURL, bytes.NewReader(request.Payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", deliveryID)
	req.Header.Set("X-Hub-Signature-256", "sha256="+request.Signature)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook endpoint %s is not reachable: %w", request.WebhookURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf(`webhook signature rejected by %s (HTTP 401: %s).

The secret must match the GitHub App webhook secret configured in the platform,
which git-bot loads from the credential broker. It is NOT a free-form value and
NOT read from the compose environment.

Find the configured secret in either:
  - the UI, under the GitHub App settings, or
  - the global config repository, at setting/git-apps/github.yaml, where the
    webhook_secret credential reference is declared.

Then export it as GITHUB_WEBHOOK_SECRET before running the webhook or pipeline
suites, or drop those suites with --suites api-read,auth`,
			request.WebhookURL, strings.TrimSpace(string(body)))
	}
	// git-bot resolves the installation before forwarding, so a delivery whose
	// installation is unknown is rejected even with a valid signature. The
	// shipped sample payload carries a placeholder id, which makes this the
	// second thing to go wrong once the secret is right.
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf(`webhook rejected by %s (HTTP %d: %s).

The signature was accepted, but the GitHub App installation in the payload is
not registered in this platform. The shipped sample payload carries a
placeholder installation id.

List the registered installations:
  curl -s -H "Authorization: Bearer $TOKEN" \
    %s/v1/git-apps/github/installations

Then re-run with one of them:
  --webhook-installation-id <installation_id>`,
			request.WebhookURL, resp.StatusCode, strings.TrimSpace(string(body)), request.APIURL)
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		return fmt.Errorf(`webhook unavailable at %s (HTTP 503: %s).

git-bot is running without GitHub App credentials, so it cannot verify or
forward deliveries. The webhook and pipeline suites cannot run against this
stack. Measure the request paths instead:

  test/perf/run-perf-test.sh --suites api-read,auth`,
			request.WebhookURL, strings.TrimSpace(string(body)))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook preflight to %s returned HTTP %d: %s",
			request.WebhookURL, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// resolveRuntimeTargets picks the runs the runtime suite writes against. The
// suite reproduces the telemetry a running pipeline emits, and every one of
// those calls is scoped to an existing run, so the targets have to be resolved
// before the ramp starts rather than invented per request.
func resolveRuntimeTargets(ctx context.Context, client *http.Client, request *RequestContext, cfg Config) (*RuntimeTargets, error) {
	if len(cfg.RuntimeRunIDs) > 0 {
		return NewRuntimeTargets(cfg.RuntimeRunIDs), nil
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		joinURL(request.APIURL, fmt.Sprintf("/v1/runs?limit=%d", cfg.RuntimeRunCount)), nil)
	if err != nil {
		return nil, err
	}
	if err := authorize(req, request); err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover runtime target runs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("discover runtime target runs: /v1/runs returned HTTP %d", resp.StatusCode)
	}
	var runs []runListItem
	if err := json.NewDecoder(resp.Body).Decode(&runs); err != nil {
		return nil, fmt.Errorf("decode runs: %w", err)
	}

	ids := make([]string, 0, len(runs))
	for _, run := range runs {
		if strings.TrimSpace(run.RunID) != "" {
			ids = append(ids, run.RunID)
		}
		if len(ids) >= cfg.RuntimeRunCount {
			break
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf(`the %q suite needs at least one existing run to write against, and this platform has none.

It reproduces the telemetry a pipeline emits while it executes - log batches,
status polling, log reads - all of which are scoped to a run that already
exists. Create one first, for example with the pipeline suite fixture in
test/perf/fixtures, or pass ids explicitly:

  --runtime-run-ids <run-id>[,<run-id>...]`, SuiteRuntime)
	}
	return NewRuntimeTargets(ids), nil
}

// platformVersion records which build was measured. A missing version is not an
// error: the report is still valid, it is just less traceable.
func platformVersion(ctx context.Context, client *http.Client, apiURL string) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(apiURL, "/version"), nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return ""
	}
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&payload); err != nil {
		return ""
	}
	return payload.Version
}

// WriteArtifacts persists the report next to any previous runs. Both a text and
// a JSON copy are written: the text one is what an operator reads, the JSON one
// is what a trend chart or a regression gate consumes.
func WriteArtifacts(report *Report, outputDir string) (textPath, jsonPath string, err error) {
	if strings.TrimSpace(outputDir) == "" {
		return "", "", nil
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create output directory: %w", err)
	}
	stamp := report.StartedAt.UTC().Format("20060102-150405")
	textPath = filepath.Join(outputDir, fmt.Sprintf("perf-%s.txt", stamp))
	jsonPath = filepath.Join(outputDir, fmt.Sprintf("perf-%s.json", stamp))

	textFile, err := os.Create(textPath)
	if err != nil {
		return "", "", fmt.Errorf("create text report: %w", err)
	}
	defer func() { _ = textFile.Close() }()
	if err := report.WriteText(textFile); err != nil {
		return "", "", fmt.Errorf("write text report: %w", err)
	}

	jsonFile, err := os.Create(jsonPath)
	if err != nil {
		return "", "", fmt.Errorf("create json report: %w", err)
	}
	defer func() { _ = jsonFile.Close() }()
	if err := report.WriteJSON(jsonFile); err != nil {
		return "", "", fmt.Errorf("write json report: %w", err)
	}
	return textPath, jsonPath, nil
}

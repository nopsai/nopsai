package perf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Pipeline trigger modes.
const (
	// TriggerExternal invokes an external trigger. This is the default because
	// it is self-contained: the platform owns the pipeline definition, nothing
	// is resolved from a third-party repository, and the response names the run
	// that was created.
	TriggerExternal = "external"
	// TriggerWebhook replays a signed git event. It measures the real ingestion
	// path, but only works when the payload describes a commit that exists in a
	// repository the platform can read.
	TriggerWebhook = "webhook"
)

// TriggerModes returns the selectable pipeline trigger modes.
func TriggerModes() []string { return []string{TriggerExternal, TriggerWebhook} }

// KnownTriggerMode reports whether the mode is selectable.
func KnownTriggerMode(mode string) bool {
	for _, candidate := range TriggerModes() {
		if candidate == mode {
			return true
		}
	}
	return false
}

// triggerResult is what a trigger returns to the family watcher. RunID is set
// when the trigger names the run it created, which lets the watcher follow that
// run directly instead of matching on a correlation id.
type triggerResult struct {
	RunID          string
	TriggerEventID string
}

// externalTriggerInvokeResponse mirrors the invoke endpoint's response body.
type externalTriggerInvokeResponse struct {
	RunID          string `json:"run_id"`
	TriggerEventID string `json:"trigger_event_id"`
	Status         string `json:"status"`
}

// invokeExternalTrigger starts one run through the external trigger API.
func (p *PipelineRunner) invokeExternalTrigger(ctx context.Context, label string) (triggerResult, error) {
	body, err := json.Marshal(map[string]any{
		"payload": map[string]any{
			"work_seconds": p.cfg.PipelineWorkSeconds,
			"label":        label,
		},
	})
	if err != nil {
		return triggerResult{}, err
	}
	url := joinURL(p.request.APIURL, "/v1/external-triggers/"+p.cfg.ExternalTriggerID+"/invoke")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return triggerResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := authorize(req, p.request); err != nil {
		return triggerResult{}, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return triggerResult{}, fmt.Errorf("invoke external trigger: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	if resp.StatusCode == http.StatusNotFound {
		return triggerResult{}, fmt.Errorf(`external trigger %q does not exist (HTTP 404).

The pipeline suite drives runs through a purpose-built trigger that has to be
installed first. Both fixtures live in test/perf/fixtures:

  pipelines/perf-load-probe.yaml          the deterministic no-LLM pipeline
  external-triggers/perf-load-probe.yaml  the trigger that starts it

Copy them into your GitOps config repository (editing pipeline, scope,
run_team_path and allowed_callers for your environment), let config sync apply
them, then re-run. See doc/performance-testing.md.`, p.cfg.ExternalTriggerID)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return triggerResult{}, fmt.Errorf(
			"external trigger %q rejected the caller (HTTP %d: %s); add the identity running the test to its allowed_callers",
			p.cfg.ExternalTriggerID, resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return triggerResult{}, fmt.Errorf("external trigger %q returned HTTP %d: %s",
			p.cfg.ExternalTriggerID, resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var decoded externalTriggerInvokeResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return triggerResult{}, fmt.Errorf("decode invoke response: %w", err)
	}
	if strings.TrimSpace(decoded.RunID) == "" && strings.TrimSpace(decoded.TriggerEventID) == "" {
		return triggerResult{}, fmt.Errorf("external trigger %q accepted the invocation but named no run", p.cfg.ExternalTriggerID)
	}
	return triggerResult{RunID: decoded.RunID, TriggerEventID: decoded.TriggerEventID}, nil
}

// triggerWebhook fires one signed git delivery. The delivery id doubles as the
// correlation key, because the platform records it as the run's trigger event.
func (p *PipelineRunner) triggerWebhook(ctx context.Context) (triggerResult, error) {
	deliveryID, err := newDeliveryID()
	if err != nil {
		return triggerResult{}, err
	}
	if err := p.fireWebhook(ctx, deliveryID); err != nil {
		return triggerResult{}, err
	}
	return triggerResult{TriggerEventID: deliveryID}, nil
}

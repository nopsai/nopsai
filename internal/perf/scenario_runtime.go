package perf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
)

// runtimeLogBatchLines is how many log lines each ingest carries. Runners ship
// logs in batches rather than one line per request, and the batch size changes
// what is being measured: the handler queues one INSERT per line, so this is the
// main lever on database write pressure per request.
const runtimeLogBatchLines = 20

// RuntimeTargets holds the run identifiers the runtime suite writes against.
// It is populated before the ramp starts, because every scenario needs a run
// that already exists.
type RuntimeTargets struct {
	runIDs []string
	cursor atomic.Uint64
}

// NewRuntimeTargets returns targets over the given run identifiers.
func NewRuntimeTargets(runIDs []string) *RuntimeTargets {
	return &RuntimeTargets{runIDs: runIDs}
}

// Empty reports whether there is nothing to write against.
func (t *RuntimeTargets) Empty() bool { return t == nil || len(t.runIDs) == 0 }

// Len returns how many runs the load is spread across.
func (t *RuntimeTargets) Len() int {
	if t == nil {
		return 0
	}
	return len(t.runIDs)
}

// Next returns the next run identifier, cycling through the set so load is
// spread evenly rather than concentrated on one row.
func (t *RuntimeTargets) Next() string {
	if t.Empty() {
		return ""
	}
	index := t.cursor.Add(1) - 1
	return t.runIDs[index%uint64(len(t.runIDs))]
}

// runtimeScenarios models the traffic a pipeline generates against the platform
// while it executes: log batches streaming in, status being polled, and the UI
// reading it all back. This is the load the platform actually has to absorb
// during a run. It is measured directly rather than by executing pipelines,
// because the execution itself belongs to the runner and the agent, not to the
// services under test.
//
// Every one of these requests is authorized, so the auth path is exercised at
// the same rate. Log ingest is weighted highest because it is by far the
// highest-volume call a real run makes.
func runtimeScenarios() []Scenario {
	return []Scenario{
		{
			Name:    "runtime.logs_ingest",
			Suite:   SuiteRuntime,
			Service: ServiceAPI,
			Weight:  6,
			Build: func(ctx context.Context, rc *RequestContext) (*http.Request, error) {
				runID := rc.Runtime.Next()
				if runID == "" {
					return nil, fmt.Errorf("no runtime target runs available")
				}
				body, err := runtimeLogBody(runID)
				if err != nil {
					return nil, err
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodPost,
					joinURL(rc.APIURL, "/v1/runs/"+runID+"/logs/ingest"), strings.NewReader(body))
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", "application/json")
				return req, authorize(req, rc)
			},
		},
		runtimeGet("runtime.run_status", 3, "/status"),
		runtimeGet("runtime.run_logs", 2, "/logs"),
		runtimeGet("runtime.run_detail", 2, ""),
	}
}

// runtimeGet builds a read against a rotating target run.
func runtimeGet(name string, weight int, suffix string) Scenario {
	return Scenario{
		Name:    name,
		Suite:   SuiteRuntime,
		Service: ServiceAPI,
		Weight:  weight,
		Build: func(ctx context.Context, rc *RequestContext) (*http.Request, error) {
			runID := rc.Runtime.Next()
			if runID == "" {
				return nil, fmt.Errorf("no runtime target runs available")
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet,
				joinURL(rc.APIURL, "/v1/runs/"+runID+suffix), nil)
			if err != nil {
				return nil, err
			}
			return req, authorize(req, rc)
		},
	}
}

// runtimeLogBody builds one log batch. Lines are marked so that anything this
// harness wrote is identifiable and removable afterwards.
func runtimeLogBody(runID string) (string, error) {
	lines := make([]string, 0, runtimeLogBatchLines)
	for i := 0; i < runtimeLogBatchLines; i++ {
		lines = append(lines, fmt.Sprintf("[nopsai-perf] synthetic log line %d for run %s", i, runID))
	}
	payload := map[string]any{
		"lines":     lines,
		"source":    "nopsai-perf",
		"stream":    "stdout",
		"level":     "info",
		"step_name": "perf-probe",
		"task_name": "perf-probe",
		"runner_id": "nopsai-perf",
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// uiScenarios load the UI container. It serves static assets through nginx, so
// it is included as the cheap control in the comparison: if the UI degrades at
// the same point as the API, the constraint is the host rather than either
// service.
func uiScenarios() []Scenario {
	return []Scenario{
		{
			Name:    "ui.index",
			Suite:   SuiteUI,
			Service: ServiceUI,
			Weight:  1,
			Build: func(ctx context.Context, rc *RequestContext) (*http.Request, error) {
				return http.NewRequestWithContext(ctx, http.MethodGet, rc.UIURL, nil)
			},
		},
	}
}

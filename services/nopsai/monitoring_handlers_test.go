package nopsai

import (
	"testing"
	"time"

	"nopsai/pkg/proto"
	"nopsai/services/nopsai/pkg/routeauthz"
)

func TestMonitoringRunnersExposeOnlyAuthorizedActiveRuns(t *testing.T) {
	status := &proto.DispatcherStatus{
		QueuedJobs: 3,
		Runners: []*proto.RunnerInfo{
			{
				RunnerId:          "runner-a",
				Capacity:          2,
				ActiveJobs:        2,
				InflightJobs:      2,
				LastHeartbeatUnix: time.Now().Unix(),
				AllowDispatch:     true,
				Metadata: map[string]string{
					"active_runs": `[
						{"run_id":"run-visible","pipeline":"Deploy","trigger_event_id":"trigger-1"},
						{"run_id":"run-hidden","pipeline":"Secret Deploy","trigger_event_id":"trigger-2"}
					]`,
				},
			},
		},
	}

	allowed := map[string]struct{}{
		resourceKey(routeauthz.RunResource("run-visible")): {},
	}

	runners, summary := monitoringRunnersFromDispatcherStatus(status, allowed)
	if summary.QueuedJobs != 3 || summary.ActiveJobs != 2 || summary.InflightJobs != 2 {
		t.Fatalf("summary = %#v, want queued=3 active=2 inflight=2", summary)
	}
	if len(runners) != 1 {
		t.Fatalf("runners len = %d, want 1", len(runners))
	}
	if len(runners[0].ActiveRuns) != 1 {
		t.Fatalf("active runs = %#v, want only authorized run", runners[0].ActiveRuns)
	}
	if got := runners[0].ActiveRuns[0]; got.RunID != "run-visible" || got.Pipeline != "Deploy" || got.TriggerID != "trigger-1" {
		t.Fatalf("active run = %#v, want visible deploy run", got)
	}
}

func TestMonitoringActiveRunResourcesDeduplicatesRuns(t *testing.T) {
	status := &proto.DispatcherStatus{
		Runners: []*proto.RunnerInfo{
			{Metadata: map[string]string{"active_runs": `[{"run_id":"run-1"},{"run_id":"run-2"}]`}},
			{Metadata: map[string]string{"active_runs": `[{"run_id":"run-1"},{"run_id":""},{"pipeline":"missing"}]`}},
		},
	}

	resources := monitoringActiveRunResources(status)
	if len(resources) != 2 {
		t.Fatalf("resources = %#v, want 2 unique run resources", resources)
	}
	if resources[0].ID != "run-1" || resources[1].ID != "run-2" {
		t.Fatalf("resources = %#v, want run-1 and run-2", resources)
	}
}

package nopsai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"nopsai/pkg/proto"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/routeauthz"
)

const monitoringRunnerStaleAfter = 30 * time.Second

type monitoringActiveRun struct {
	RunID      string `json:"run_id"`
	Pipeline   string `json:"pipeline,omitempty"`
	ParentStep string `json:"parent_step,omitempty"`
	TriggerID  string `json:"trigger_event_id,omitempty"`
}

type monitoringRunnerStatus struct {
	RunnerID          string                `json:"runner_id"`
	Label             string                `json:"label"`
	Status            string                `json:"status"`
	Runtime           string                `json:"runtime,omitempty"`
	Namespace         string                `json:"namespace,omitempty"`
	Node              string                `json:"node,omitempty"`
	Capacity          int32                 `json:"capacity"`
	ActiveJobs        int32                 `json:"active_jobs"`
	InflightJobs      int32                 `json:"inflight_jobs"`
	LastHeartbeatUnix int64                 `json:"last_heartbeat_unix,omitempty"`
	AllowDispatch     bool                  `json:"allow_dispatch"`
	ActiveRuns        []monitoringActiveRun `json:"active_runs,omitempty"`
}

type monitoringRunnerSummary struct {
	Total        int   `json:"total"`
	Online       int   `json:"online"`
	Stale        int   `json:"stale"`
	Disabled     int   `json:"disabled"`
	Unknown      int   `json:"unknown"`
	Docker       int   `json:"docker"`
	Kubernetes   int   `json:"kubernetes"`
	Capacity     int32 `json:"capacity"`
	ActiveJobs   int32 `json:"active_jobs"`
	InflightJobs int32 `json:"inflight_jobs"`
	QueuedJobs   int32 `json:"queued_jobs"`
}

func (a *App) handleMonitoringDispatcherStatus(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)

	ctx := r.Context()
	status, dispatcherErr := a.fetchDispatcherStatus(ctx)
	if dispatcherErr != nil {
		log.Error().Err(dispatcherErr).Msg("Failed to fetch dispatcher status for monitoring")
	}

	allowedRunSet := a.allowedMonitoringActiveRunSet(r, status)
	runners, summary := monitoringRunnersFromDispatcherStatus(status, allowedRunSet)
	resp := map[string]interface{}{
		"queued_jobs":    summary.QueuedJobs,
		"runner_summary": summary,
		"runners":        runners,
		"services":       a.buildSystemServiceStatuses(ctx, status, dispatcherErr),
	}
	if dispatcherErr != nil {
		resp["dispatcher_error"] = "Failed to fetch dispatcher status"
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Warn().Err(err).Msg("Failed to encode monitoring dispatcher status")
	}
}

func (a *App) fetchDispatcherStatus(ctx context.Context) (*proto.DispatcherStatus, error) {
	if a.dispatcher == nil {
		return nil, errors.New("dispatcher client is unavailable")
	}
	statusCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return a.dispatcher.GetStatus(statusCtx)
}

func (a *App) allowedMonitoringActiveRunSet(r *http.Request, status *proto.DispatcherStatus) map[string]struct{} {
	resources := monitoringActiveRunResources(status)
	if len(resources) == 0 {
		return nil
	}

	allowedSet, err := a.allowedResourceSet(r, "pipeline_run.list", resources)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to filter monitoring active runs by authorization")
		return map[string]struct{}{}
	}
	return allowedSet
}

func monitoringActiveRunResources(status *proto.DispatcherStatus) []model.ResourceRef {
	if status == nil {
		return nil
	}

	seen := map[string]struct{}{}
	resources := make([]model.ResourceRef, 0)
	for _, runner := range status.GetRunners() {
		for _, activeRun := range parseMonitoringActiveRuns(runner.GetMetadata()) {
			if _, ok := seen[activeRun.RunID]; ok {
				continue
			}
			seen[activeRun.RunID] = struct{}{}
			resources = append(resources, routeauthz.RunResource(activeRun.RunID))
		}
	}
	return resources
}

func monitoringRunnersFromDispatcherStatus(status *proto.DispatcherStatus, allowedRunSet map[string]struct{}) ([]monitoringRunnerStatus, monitoringRunnerSummary) {
	summary := monitoringRunnerSummary{}
	if status == nil {
		return nil, summary
	}

	summary.QueuedJobs = status.GetQueuedJobs()
	runners := append([]*proto.RunnerInfo(nil), status.GetRunners()...)
	sort.Slice(runners, func(i, j int) bool {
		return runners[i].GetRunnerId() < runners[j].GetRunnerId()
	})

	now := time.Now()
	items := make([]monitoringRunnerStatus, 0, len(runners))
	for index, runner := range runners {
		metadata := runner.GetMetadata()
		runtime := runnerRuntime(metadata)
		item := monitoringRunnerStatus{
			RunnerID:          runner.GetRunnerId(),
			Label:             firstMonitoringText(runner.GetRunnerId(), "Runner "+strconv.Itoa(index+1)),
			Status:            monitoringRunnerState(runner, now),
			Runtime:           runtime,
			Namespace:         firstMonitoringText(metadata["kubernetes_namespace"], metadata["namespace"]),
			Node:              firstMonitoringText(metadata["kubernetes_node"], metadata["node"], metadata["hostname"]),
			Capacity:          runner.GetCapacity(),
			ActiveJobs:        runner.GetActiveJobs(),
			InflightJobs:      runner.GetInflightJobs(),
			LastHeartbeatUnix: runner.GetLastHeartbeatUnix(),
			AllowDispatch:     runner.GetAllowDispatch(),
			ActiveRuns:        authorizedMonitoringActiveRuns(metadata, allowedRunSet),
		}
		items = append(items, item)

		summary.Total++
		summary.Capacity += item.Capacity
		summary.ActiveJobs += item.ActiveJobs
		summary.InflightJobs += item.InflightJobs
		if runtime == "kubernetes" {
			summary.Kubernetes++
		} else {
			summary.Docker++
		}
		switch item.Status {
		case "online":
			summary.Online++
		case "stale":
			summary.Stale++
		case "disabled":
			summary.Disabled++
		default:
			summary.Unknown++
		}
	}
	return items, summary
}

func runnerRuntime(metadata map[string]string) string {
	runtime := strings.ToLower(strings.TrimSpace(metadata["runtime"]))
	switch runtime {
	case "k8s", "kubernetes":
		return "kubernetes"
	case "docker", "":
		return "docker"
	default:
		return runtime
	}
}

func authorizedMonitoringActiveRuns(metadata map[string]string, allowedRunSet map[string]struct{}) []monitoringActiveRun {
	activeRuns := parseMonitoringActiveRuns(metadata)
	if len(activeRuns) == 0 || len(allowedRunSet) == 0 {
		return nil
	}

	visible := make([]monitoringActiveRun, 0, len(activeRuns))
	for _, activeRun := range activeRuns {
		if _, ok := allowedRunSet[resourceKey(routeauthz.RunResource(activeRun.RunID))]; !ok {
			continue
		}
		visible = append(visible, activeRun)
	}
	return visible
}

func parseMonitoringActiveRuns(metadata map[string]string) []monitoringActiveRun {
	raw := strings.TrimSpace(metadata["active_runs"])
	if raw == "" {
		return nil
	}

	var payload []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		log.Debug().Err(err).Msg("Failed to parse runner active_runs metadata")
		return nil
	}

	seen := map[string]struct{}{}
	activeRuns := make([]monitoringActiveRun, 0, len(payload))
	for _, item := range payload {
		runID := monitoringJSONText(item["run_id"])
		if runID == "" {
			continue
		}
		if _, ok := seen[runID]; ok {
			continue
		}
		seen[runID] = struct{}{}
		activeRuns = append(activeRuns, monitoringActiveRun{
			RunID:      runID,
			Pipeline:   monitoringJSONText(item["pipeline"]),
			ParentStep: monitoringJSONText(item["parent_step"]),
			TriggerID:  firstMonitoringText(monitoringJSONText(item["trigger_event_id"]), monitoringJSONText(item["trigger_id"])),
		})
	}
	return activeRuns
}

func monitoringJSONText(value interface{}) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func firstMonitoringText(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func monitoringRunnerState(runner *proto.RunnerInfo, now time.Time) string {
	if runner == nil {
		return "unknown"
	}
	if !runner.GetAllowDispatch() {
		return "disabled"
	}
	lastHeartbeat := runner.GetLastHeartbeatUnix()
	if lastHeartbeat <= 0 {
		return "unknown"
	}
	if now.Sub(time.Unix(lastHeartbeat, 0)) > monitoringRunnerStaleAfter {
		return "stale"
	}
	return "online"
}

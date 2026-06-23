package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nopsai/pkg/proto"

	"github.com/rs/zerolog/log"
	gproto "google.golang.org/protobuf/proto"
)

func (d *dispatcherServer) enqueue(job *proto.JobRequest) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queue = append(d.queue, job)
	log.Info().Str("run_id", job.RunId).Str("scope", job.Scope).Msg("job queued")
}

func (d *dispatcherServer) dispatch(job *proto.JobRequest) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	runner := d.pickRunnerForJobLocked(job)
	if runner == nil {
		return ""
	}

	jobForRunner := d.prepareJobForRunner(job, runner)
	affinityKey := affinityKeyForJob(job)
	preferredRunnerID := strings.TrimSpace(job.PreferredRunnerId)

	select {
	case runner.sendCh <- &proto.DispatcherMessage{Message: &proto.DispatcherMessage_Job{Job: jobForRunner}}:
		runner.inflight[job.RunId] = jobForRunner
		runner.active++
		log.Info().
			Str("run_id", job.RunId).
			Str("runner_id", runner.id).
			Str("pipeline", job.PipelineName).
			Str("scope", job.Scope).
			Str("trigger_event_id", job.TriggerEventId).
			Str("runner_affinity_key", affinityKey).
			Str("preferred_runner_id", preferredRunnerID).
			Msg("job dispatched to runner")
		if affinityKey != "" {
			d.triggerAssignments[affinityKey] = runner.id
		}
		d.recordAssignment(job.RunId, job.PipelineName, runner.id, job.Scope, job.TriggerEventId)
		return runner.id
	default:
		log.Warn().Str("runner_id", runner.id).Msg("runner send channel is full; queueing job")
		return ""
	}
}

func (d *dispatcherServer) pumpQueue() {
	d.mu.Lock()
	if len(d.queue) == 0 {
		d.mu.Unlock()
		return
	}
	queuedJobs := append([]*proto.JobRequest(nil), d.queue...)
	d.queue = nil
	d.mu.Unlock()

	var assignments []struct {
		runID        string
		pipelineName string
		runnerID     string
		scope        string
		triggerID    string
	}
	for _, job := range queuedJobs {
		dispatchCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		runStatus, err := d.fetchRunStatusValue(dispatchCtx, job.RunId)
		cancel()
		if err != nil {
			log.Warn().Err(err).Str("run_id", job.RunId).Msg("Failed to verify queued run status before dispatch")
		} else if !runStatusAllowsDispatch(runStatus) {
			log.Info().Str("run_id", job.RunId).Str("status", runStatus).Msg("Dropping queued job for non-dispatchable run")
			continue
		}

		d.mu.Lock()
		runner := d.pickRunnerForJobLocked(job)
		if runner == nil {
			d.queue = append(d.queue, job)
			d.mu.Unlock()
			continue
		}
		jobForRunner := d.prepareJobForRunner(job, runner)
		if jobForRunner == nil {
			log.Error().Str("run_id", job.RunId).Str("runner_id", runner.id).Msg("failed to prepare queued job for runner")
			d.queue = append(d.queue, job)
			d.mu.Unlock()
			continue
		}
		affinityKey := affinityKeyForJob(job)
		preferredRunnerID := strings.TrimSpace(job.PreferredRunnerId)
		select {
		case runner.sendCh <- &proto.DispatcherMessage{Message: &proto.DispatcherMessage_Job{Job: jobForRunner}}:
			runner.inflight[job.RunId] = jobForRunner
			runner.active++
			log.Info().
				Str("run_id", job.RunId).
				Str("runner_id", runner.id).
				Str("pipeline", job.PipelineName).
				Str("scope", job.Scope).
				Str("trigger_event_id", job.TriggerEventId).
				Str("runner_affinity_key", affinityKey).
				Str("preferred_runner_id", preferredRunnerID).
				Msg("job dispatched from queue")
			if affinityKey != "" {
				d.triggerAssignments[affinityKey] = runner.id
			}
			assignments = append(assignments, struct {
				runID        string
				pipelineName string
				runnerID     string
				scope        string
				triggerID    string
			}{
				runID:        job.RunId,
				pipelineName: job.PipelineName,
				runnerID:     runner.id,
				scope:        job.Scope,
				triggerID:    job.TriggerEventId,
			})
			d.mu.Unlock()
		default:
			log.Warn().Str("runner_id", runner.id).Msg("runner send channel is full during queue dispatch")
			d.queue = append(d.queue, job)
			d.mu.Unlock()
		}
	}

	// Send assignment log entries without holding the dispatcher lock.
	for _, a := range assignments {
		d.recordAssignment(a.runID, a.pipelineName, a.runnerID, a.scope, a.triggerID)
	}
}

func runStatusAllowsDispatch(statusText string) bool {
	switch strings.ToLower(strings.TrimSpace(statusText)) {
	case "", "pending", "running":
		return true
	default:
		return false
	}
}

func (d *dispatcherServer) prepareJobForRunner(job *proto.JobRequest, runner *runnerConn) *proto.JobRequest {
	if job == nil {
		return nil
	}
	copyJob, ok := gproto.Clone(job).(*proto.JobRequest)
	if !ok || copyJob == nil {
		return nil
	}
	runtimeVars := append([]string(nil), copyJob.Env...)
	if runner != nil {
		if override, ok := runner.metadata["docker_network"]; ok {
			copyJob.DockerNetwork = strings.TrimSpace(override)
			runtimeVars = upsertRuntimeVar(runtimeVars, "DOCKER_NETWORK_NAME", copyJob.DockerNetwork)
		}
		if addr, ok := runner.metadata["dispatcher_addr"]; ok {
			runtimeVars = upsertRuntimeVar(runtimeVars, "DISPATCHER_GRPC_ADDRESS", strings.TrimSpace(addr))
		}
		runtimeVars = upsertRuntimeVar(runtimeVars, "RUNNER_ID", runner.id)
	}
	copyJob.Env = runtimeVars
	return copyJob
}

func upsertRuntimeVar(runtimeVars []string, key, val string) []string {
	prefix := key + "="
	for i, e := range runtimeVars {
		if strings.HasPrefix(e, prefix) {
			runtimeVars[i] = prefix + val
			return runtimeVars
		}
	}
	return append(runtimeVars, prefix+val)
}

func pipelinePath(identifier string) string {
	trimmed := strings.TrimSpace(identifier)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if strings.HasSuffix(lower, ".yaml") {
		trimmed = trimmed[:len(trimmed)-len(".yaml")]
	} else if strings.HasSuffix(lower, ".yml") {
		trimmed = trimmed[:len(trimmed)-len(".yml")]
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) <= 1 {
		return ""
	}
	return strings.Join(parts[:len(parts)-1], "/")
}

func gitHeaderKey(envKey string) string {
	key := strings.TrimSpace(envKey)
	key = strings.TrimPrefix(key, "X-")
	lowerKey := strings.ToLower(key)

	// Preserve trigger event ID with the header name expected by nopsai
	if lowerKey == "git_trigger_event_id" || lowerKey == "trigger_event_id" {
		return "X-Nopsai-Trigger-Event-ID"
	}

	parts := strings.Split(lowerKey, "_")
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return "X-" + strings.Join(parts, "-")
}

func (d *dispatcherServer) recordAssignment(runID, pipelineName, runnerID, scope, triggerID string) {
	runID = strings.TrimSpace(runID)
	runnerID = strings.TrimSpace(runnerID)
	if runID == "" || runnerID == "" {
		return
	}

	scope = strings.TrimSpace(scope)
	pipelineName = strings.TrimSpace(pipelineName)
	triggerID = strings.TrimSpace(triggerID)

	msg := fmt.Sprintf("Assigned run %s to runner %s", runID, runnerID)
	if pipelineName != "" {
		msg = fmt.Sprintf("Assigned pipeline %s (run %s) to runner %s", pipelineName, runID, runnerID)
	}
	if scope != "" {
		msg = fmt.Sprintf("%s (scope %s)", msg, scope)
	}
	if triggerID != "" {
		msg = fmt.Sprintf("%s [trigger %s]", msg, triggerID)
	}

	go func(runID, line string) {
		if _, err := d.IngestLogs(context.Background(), &proto.LogBatch{
			RunId: runID,
			Lines: []string{line},
		}); err != nil {
			log.Warn().Err(err).Str("run_id", runID).Msg("failed to record runner assignment")
		}
	}(runID, msg)
}

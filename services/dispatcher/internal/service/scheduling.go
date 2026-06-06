package service

import (
	"strings"

	"nopsai/pkg/proto"

	"github.com/rs/zerolog/log"
)

func affinityKeyForJob(job *proto.JobRequest) string {
	if job == nil {
		return ""
	}
	if key := strings.TrimSpace(job.RunnerAffinityKey); key != "" {
		return key
	}
	if key := strings.TrimSpace(job.TriggerEventId); key != "" {
		return key
	}
	return strings.TrimSpace(job.RunId)
}

func (d *dispatcherServer) pickRunnerForJobLocked(job *proto.JobRequest) *runnerConn {
	if job == nil {
		return nil
	}

	allowed := d.allowedRunnerIDs(job.Scope)
	preferredRunnerID := strings.TrimSpace(job.PreferredRunnerId)
	affinityKey := affinityKeyForJob(job)

	if preferredRunnerID != "" {
		if r := d.runnerByIDLocked(preferredRunnerID); r != nil {
			if d.runnerEligibleForJob(r, job.Scope, allowed) {
				return r
			}
			if d.runnerMatchesScope(r, job.Scope, allowed) && runnerLoad(r) >= r.capacity {
				log.Info().
					Str("run_id", job.RunId).
					Str("preferred_runner_id", preferredRunnerID).
					Msg("preferred runner at capacity; falling back to affinity/selection")
			} else {
				log.Info().
					Str("run_id", job.RunId).
					Str("preferred_runner_id", preferredRunnerID).
					Msg("preferred runner not eligible; falling back to affinity/selection")
			}
		} else {
			log.Info().
				Str("run_id", job.RunId).
				Str("preferred_runner_id", preferredRunnerID).
				Msg("preferred runner not connected; falling back to affinity/selection")
		}
	}

	if affinityKey != "" {
		if runnerID, ok := d.triggerAssignments[affinityKey]; ok {
			if r := d.runnerByIDLocked(runnerID); r != nil {
				if d.runnerEligibleForJob(r, job.Scope, allowed) {
					return r
				}
				if d.runnerMatchesScope(r, job.Scope, allowed) && runnerLoad(r) >= r.capacity {
					log.Info().
						Str("run_id", job.RunId).
						Str("runner_affinity_key", affinityKey).
						Str("runner_id", runnerID).
						Msg("affinity runner at capacity; falling back to selection")
				} else {
					delete(d.triggerAssignments, affinityKey)
				}
			} else {
				delete(d.triggerAssignments, affinityKey)
			}
		}
	}

	var candidates []*runnerConn
	var busyChoice *runnerConn
	for _, r := range d.runners {
		load := runnerLoad(r)
		if !d.runnerMatchesScope(r, job.Scope, allowed) {
			continue
		}
		if r.allowDispatch && load < r.capacity {
			candidates = append(candidates, r)
		} else if r.allowDispatch {
			if busyChoice == nil || load < runnerLoad(busyChoice) {
				busyChoice = r
			}
		}
	}

	if len(candidates) == 0 {
		if affinityKey != "" && busyChoice != nil {
			d.triggerAssignments[affinityKey] = busyChoice.id
		}
		return nil
	}

	selected := candidates[0]
	for _, r := range candidates[1:] {
		if runnerLoad(r) < runnerLoad(selected) {
			selected = r
		}
	}
	if affinityKey != "" {
		d.triggerAssignments[affinityKey] = selected.id
	}
	return selected
}

func (d *dispatcherServer) runnerMatchesScope(r *runnerConn, scope string, allowed []string) bool {
	if r == nil {
		return false
	}
	if len(allowed) > 0 && !contains(allowed, r.id) {
		return false
	}
	if scope != "" && !r.hasScope(scope) {
		return false
	}
	return r.allowDispatch
}

func (d *dispatcherServer) runnerEligibleForJob(r *runnerConn, scope string, allowed []string) bool {
	if !d.runnerMatchesScope(r, scope, allowed) {
		return false
	}
	return runnerLoad(r) < r.capacity
}

// runnerByIDLocked returns the runner connection matching the given runner ID.
// Caller must hold d.mu.
func (d *dispatcherServer) runnerByIDLocked(runnerID string) *runnerConn {
	if runnerID == "" {
		return nil
	}
	for _, r := range d.runners {
		if r.id == runnerID {
			return r
		}
	}
	return nil
}

func (d *dispatcherServer) dropTriggerAssignmentsForRunner(runnerID string) {
	if runnerID == "" {
		return
	}
	for triggerID, assigned := range d.triggerAssignments {
		if assigned == runnerID {
			delete(d.triggerAssignments, triggerID)
		}
	}
}

func (r *runnerConn) hasScope(scope string) bool {
	if scope == "" {
		return true
	}
	if len(r.scopes) == 0 {
		return true
	}
	_, ok := r.scopes[strings.ToLower(scope)]
	return ok
}

func runnerLoad(r *runnerConn) int32 {
	if r == nil {
		return 1 << 30
	}
	load := r.active
	if inflight := int32(len(r.inflight)); inflight > load {
		load = inflight
	}
	return load
}

func contains(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

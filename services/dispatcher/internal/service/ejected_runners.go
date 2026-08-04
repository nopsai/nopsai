package service

import (
	"sort"
	"strings"

	"github.com/rs/zerolog/log"
)

const runnerBlockedByEjectedRunnerIDs = "runner ID is blocked by ejected_runner_ids"

func normalizeEjectedRunnerIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	normalized := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	sort.Strings(normalized)
	return normalized
}

func (d *dispatcherServer) runnerEjectedLocked(runnerID string) bool {
	runnerID = strings.TrimSpace(runnerID)
	if runnerID == "" || len(d.ejectedRunners) == 0 {
		return false
	}
	_, ok := d.ejectedRunners[runnerID]
	return ok
}

func runnerIDSetEqual(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for id := range left {
		if _, ok := right[id]; !ok {
			return false
		}
	}
	return true
}

func runnerIDSetFromList(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

func (d *dispatcherServer) applyEjectedRunners(ids []string) ([]*runnerConn, int, bool) {
	normalized := normalizeEjectedRunnerIDs(ids)
	next := runnerIDSetFromList(normalized)

	d.mu.Lock()
	changed := !runnerIDSetEqual(d.ejectedRunners, next)
	d.ejectedRunners = next
	var targets []*runnerConn
	requeuedJobs := 0
	for _, runnerID := range normalized {
		removedTargets, removedJobs, removed := d.removeRunnerRegistrationLocked(runnerID, "ejected_runner_sync")
		targets = append(targets, removedTargets...)
		requeuedJobs += removedJobs
		if removed {
			changed = true
		}
	}
	d.mu.Unlock()

	return targets, requeuedJobs, changed
}

func (d *dispatcherServer) updateEjectedRunners(ids []string) bool {
	targets, requeuedJobs, changed := d.applyEjectedRunners(ids)
	for _, target := range targets {
		if target.cancel != nil {
			target.cancel()
		}
	}
	if !changed {
		return false
	}

	log.Warn().
		Int("ejected_runner_ids", len(normalizeEjectedRunnerIDs(ids))).
		Int("disconnected_runners", len(targets)).
		Int("requeued_jobs", requeuedJobs).
		Msg("dispatcher ejected runner blocklist updated")
	if requeuedJobs > 0 {
		go d.pumpQueue()
	}
	return true
}

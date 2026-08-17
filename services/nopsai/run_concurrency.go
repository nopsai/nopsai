package nopsai

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// Runs that target the same pipeline on the same branch are serialized instead
// of racing each other. A second push while the first run is still executing
// queues behind it rather than cancelling it, so every triggered run reports a
// result and no run is ever killed part-way through work it cannot undo.
//
// The queue is expressed entirely through run status: a queued run stays
// `pending` and is simply not dispatched. The pending-run recovery worker
// already retries pending runs, so a queued run is picked up even if the
// in-process release below never fires.
const runConcurrencyGroupUnset = ""

// buildRunConcurrencyGroup keys the queue on repository, ref, and pipeline.
// Two different pipelines, or the same pipeline on two branches, are unrelated
// work and run in parallel. Runs without a repository and ref — manual runs,
// schedules, and external triggers — are not serialized.
func buildRunConcurrencyGroup(gitContext map[string]string, pipelineName string) string {
	owner := strings.TrimSpace(gitContext["repo_owner"])
	repo := strings.TrimSpace(gitContext["repo_name"])
	ref := strings.TrimSpace(gitContext["ref"])
	pipeline := strings.TrimSpace(pipelineName)
	if owner == "" || repo == "" || ref == "" || pipeline == "" {
		return runConcurrencyGroupUnset
	}
	return fmt.Sprintf("%s/%s@%s#%s", owner, repo, ref, pipeline)
}

// runConcurrencyGroupForRequest leaves child runs out of the queue entirely:
// their parent already holds the group and is blocked waiting for them, so
// queueing a child behind its own parent would deadlock the pipeline.
func runConcurrencyGroupForRequest(req createPendingRunRequest) string {
	if strings.TrimSpace(req.ParentRunID) != "" {
		return runConcurrencyGroupUnset
	}
	return buildRunConcurrencyGroup(req.GitContext, req.Pipeline.Name)
}

// runHoldsConcurrencyGroup reports whether the run may start now. A run may
// start when it is the oldest unfinished top-level run in its group, which
// makes the queue first-in-first-out and mutually exclusive with one query.
// Child runs never participate: their parent already holds the group, so
// queueing them would deadlock the parent that is waiting for them.
func (a *App) runHoldsConcurrencyGroup(ctx context.Context, runID, group string) (string, bool) {
	group = strings.TrimSpace(group)
	if a == nil || a.db == nil || group == runConcurrencyGroupUnset {
		return "", true
	}
	var holderRunID string
	err := a.db.QueryRow(ctx, `
		SELECT run_id::text
		FROM pipeline_runs
		WHERE concurrency_group = $1
		  AND parent_run_id IS NULL
		  AND status NOT IN ('success', 'warning', 'failure', 'cancelled', 'timed_out', 'failure (ignored)', 'rejected')
		ORDER BY created_at ASC, run_id ASC
		LIMIT 1
	`, group).Scan(&holderRunID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", true
		}
		// Being unable to evaluate the queue is not permission to skip it: a
		// failed read leaves the run pending for the recovery worker to retry.
		log.Warn().Err(err).Str("run_id", runID).Str("concurrency_group", group).Msg("Failed to evaluate run concurrency group")
		return "", false
	}
	return holderRunID, holderRunID == runID
}

func (a *App) runConcurrencyGroup(ctx context.Context, runID string) string {
	if a == nil || a.db == nil {
		return runConcurrencyGroupUnset
	}
	var group string
	if err := a.db.QueryRow(ctx, `SELECT COALESCE(concurrency_group, '') FROM pipeline_runs WHERE run_id = $1`, runID).Scan(&group); err != nil {
		return runConcurrencyGroupUnset
	}
	return strings.TrimSpace(group)
}

// releaseRunConcurrencyGroup starts the next queued run as soon as the run that
// held the group finishes, so a queue advances immediately instead of waiting
// for the next recovery poll.
func (a *App) releaseRunConcurrencyGroup(ctx context.Context, group string) {
	group = strings.TrimSpace(group)
	if a == nil || a.db == nil || group == runConcurrencyGroupUnset {
		return
	}
	var nextRunID string
	err := a.db.QueryRow(ctx, `
		SELECT run_id::text
		FROM pipeline_runs
		WHERE concurrency_group = $1
		  AND parent_run_id IS NULL
		  AND status = 'pending'
		ORDER BY created_at ASC, run_id ASC
		LIMIT 1
	`, group).Scan(&nextRunID)
	if err != nil {
		if err != pgx.ErrNoRows {
			log.Warn().Err(err).Str("concurrency_group", group).Msg("Failed to find the next queued run")
		}
		return
	}
	record, err := a.loadPendingRunForRecovery(ctx, nextRunID)
	if err != nil {
		log.Warn().Err(err).Str("run_id", nextRunID).Msg("Failed to load the next queued run for dispatch")
		return
	}
	log.Info().Str("run_id", nextRunID).Str("concurrency_group", group).Msg("Starting the next queued run")
	if err := a.recoverPendingPipelineRun(ctx, record); err != nil {
		log.Warn().Err(err).Str("run_id", nextRunID).Msg("Failed to start the next queued run")
	}
}

// releaseRunConcurrencyGroupForRun looks the group up from the run that just
// reached a terminal status.
func (a *App) releaseRunConcurrencyGroupForRun(ctx context.Context, runID string) {
	if group := a.runConcurrencyGroup(ctx, runID); group != runConcurrencyGroupUnset {
		a.releaseRunConcurrencyGroup(ctx, group)
	}
}

func queuedBehindRunMessage(holderRunID, group string) string {
	return fmt.Sprintf("Queued behind run %s in concurrency group %s", holderRunID, group)
}

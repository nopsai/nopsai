package nopsai

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

func (a *App) runDataCleanupScheduleWorker(ctx context.Context) {
	ticker := time.NewTicker(dataCleanupWorkerPollInterval)
	defer ticker.Stop()

	a.dispatchDueDataCleanupSchedules(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.dispatchDueDataCleanupSchedules(ctx)
		}
	}
}

func (a *App) dispatchDueDataCleanupSchedules(ctx context.Context) {
	if a == nil || a.db == nil {
		return
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to begin data cleanup schedule dispatch")
		return
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, baseDataCleanupScheduleSelect()+`
		WHERE enabled = TRUE
		  AND next_run_at IS NOT NULL
		  AND next_run_at <= NOW()
		ORDER BY next_run_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, dataCleanupWorkerBatchSize)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to claim due data cleanup schedules")
		return
	}
	var due []dataCleanupScheduleRecord
	for rows.Next() {
		record, err := scanDataCleanupSchedule(rows)
		if err != nil {
			rows.Close()
			log.Warn().Err(err).Msg("Failed to scan due data cleanup schedule")
			return
		}
		due = append(due, record)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		log.Warn().Err(err).Msg("Failed to read due data cleanup schedules")
		return
	}
	rows.Close()

	now := time.Now()
	for _, record := range due {
		nextRunAt, err := nextScheduleRunAt(record.CronExpression, record.Timezone, now)
		if err != nil {
			log.Warn().Err(err).Str("schedule_id", record.ID).Msg("Failed to calculate next data cleanup schedule time")
			_, _ = tx.Exec(ctx, `
				UPDATE data_cleanup_schedules
				SET next_run_at = NULL,
					last_status = 'failure',
					last_error = $2,
					updated_at = NOW()
				WHERE id::text = $1
			`, record.ID, err.Error())
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE data_cleanup_schedules
			SET next_run_at = $2,
				updated_at = NOW()
			WHERE id::text = $1
		`, record.ID, nextRunAt); err != nil {
			log.Warn().Err(err).Str("schedule_id", record.ID).Msg("Failed to advance data cleanup schedule")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to commit data cleanup schedule claims")
		return
	}

	for _, record := range due {
		job, err := a.runDataCleanupJob(ctx, dataCleanupPlanFromSchedule(record), dataCleanupTriggerScheduled, record.ID, "cleanup-schedule:"+record.ID)
		if err != nil {
			a.auditDataManagementAction(ctx, nil, "data.cleanup.executed", "data_cleanup_job:"+job.ID, "failure", map[string]any{
				"schedule_id": record.ID,
				"target":      record.Target,
				"mode":        record.Mode,
				"error":       err.Error(),
			})
			log.Warn().Err(err).Str("schedule_id", record.ID).Msg("Scheduled data cleanup failed")
			continue
		}
		a.auditDataManagementAction(ctx, nil, "data.cleanup.executed", "data_cleanup_job:"+job.ID, "success", map[string]any{
			"schedule_id":  record.ID,
			"target":       job.Target,
			"mode":         job.Mode,
			"deleted_rows": totalCount(job.DeletedCounts),
		})
	}
}

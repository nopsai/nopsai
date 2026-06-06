package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (a *App) previewDataCleanup(ctx context.Context, plan dataCleanupPlan) (map[string]int64, error) {
	switch plan.Target {
	case dataCleanupTargetRuns:
		return a.previewRunCleanup(ctx, plan)
	case dataCleanupTargetLogs:
		return a.previewLogCleanup(ctx, plan)
	default:
		return nil, fmt.Errorf("unsupported cleanup target")
	}
}

func (a *App) executeDataCleanup(ctx context.Context, plan dataCleanupPlan) (map[string]int64, error) {
	switch plan.Target {
	case dataCleanupTargetRuns:
		return a.executeRunCleanup(ctx, plan)
	case dataCleanupTargetLogs:
		return a.executeLogCleanup(ctx, plan)
	default:
		return nil, fmt.Errorf("unsupported cleanup target")
	}
}

func (a *App) previewRunCleanup(ctx context.Context, plan dataCleanupPlan) (map[string]int64, error) {
	switch plan.Mode {
	case dataCleanupModeKeepLast:
		return scanRunCleanupCounts(a.db.QueryRow(ctx, runCleanupCountSQL(`
			WITH protected_runs AS (
				SELECT run_id
				FROM pipeline_runs
				ORDER BY COALESCE(finished_at, started_at, created_at) DESC, created_at DESC, run_id DESC
				LIMIT $1
			),
			candidate_runs AS (
				SELECT pr.run_id
				FROM pipeline_runs pr
				WHERE pr.status = ANY($2::text[])
				  AND NOT EXISTS (SELECT 1 FROM protected_runs keep WHERE keep.run_id = pr.run_id)
			)
		`), plan.KeepLast, terminalRunStatusesForCleanup))
	case dataCleanupModeOlderThanDays:
		return scanRunCleanupCounts(a.db.QueryRow(ctx, runCleanupCountSQL(`
			WITH candidate_runs AS (
				SELECT pr.run_id
				FROM pipeline_runs pr
				WHERE pr.status = ANY($1::text[])
				  AND COALESCE(pr.finished_at, pr.started_at, pr.created_at) < NOW() - ($2::int * INTERVAL '1 day')
			)
		`), terminalRunStatusesForCleanup, plan.OlderThanDays))
	case dataCleanupModeAllTerminalRuns:
		return scanRunCleanupCounts(a.db.QueryRow(ctx, runCleanupCountSQL(`
			WITH candidate_runs AS (
				SELECT pr.run_id
				FROM pipeline_runs pr
				WHERE pr.status = ANY($1::text[])
			)
		`), terminalRunStatusesForCleanup))
	default:
		return nil, fmt.Errorf("unsupported runs cleanup mode")
	}
}

func (a *App) executeRunCleanup(ctx context.Context, plan dataCleanupPlan) (map[string]int64, error) {
	counts, err := a.previewRunCleanup(ctx, plan)
	if err != nil {
		return nil, err
	}
	if counts["pipeline_runs"] == 0 {
		return counts, nil
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var deleteSQL string
	var args []any
	switch plan.Mode {
	case dataCleanupModeKeepLast:
		deleteSQL = runCleanupDeleteSQL(`
			WITH protected_runs AS (
				SELECT run_id
				FROM pipeline_runs
				ORDER BY COALESCE(finished_at, started_at, created_at) DESC, created_at DESC, run_id DESC
				LIMIT $1
			),
			candidate_runs AS (
				SELECT pr.run_id
				FROM pipeline_runs pr
				WHERE pr.status = ANY($2::text[])
				  AND NOT EXISTS (SELECT 1 FROM protected_runs keep WHERE keep.run_id = pr.run_id)
			)
		`)
		args = []any{plan.KeepLast, terminalRunStatusesForCleanup}
	case dataCleanupModeOlderThanDays:
		deleteSQL = runCleanupDeleteSQL(`
			WITH candidate_runs AS (
				SELECT pr.run_id
				FROM pipeline_runs pr
				WHERE pr.status = ANY($1::text[])
				  AND COALESCE(pr.finished_at, pr.started_at, pr.created_at) < NOW() - ($2::int * INTERVAL '1 day')
			)
		`)
		args = []any{terminalRunStatusesForCleanup, plan.OlderThanDays}
	case dataCleanupModeAllTerminalRuns:
		deleteSQL = runCleanupDeleteSQL(`
			WITH candidate_runs AS (
				SELECT pr.run_id
				FROM pipeline_runs pr
				WHERE pr.status = ANY($1::text[])
			)
		`)
		args = []any{terminalRunStatusesForCleanup}
	default:
		return nil, fmt.Errorf("unsupported runs cleanup mode")
	}
	var deletedRuns int64
	if err := tx.QueryRow(ctx, deleteSQL, args...).Scan(&deletedRuns); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	counts["pipeline_runs"] = deletedRuns
	return counts, nil
}

func (a *App) previewLogCleanup(ctx context.Context, plan dataCleanupPlan) (map[string]int64, error) {
	var count int64
	switch plan.Mode {
	case dataCleanupModeOlderThanDays:
		err := a.db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM pipeline_run_logs
			WHERE timestamp < NOW() - ($1::int * INTERVAL '1 day')
		`, plan.OlderThanDays).Scan(&count)
		if err != nil {
			return nil, err
		}
	case dataCleanupModeAllLogs:
		if err := a.db.QueryRow(ctx, `SELECT COUNT(*) FROM pipeline_run_logs`).Scan(&count); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported logs cleanup mode")
	}
	return map[string]int64{"pipeline_run_logs": count}, nil
}

func (a *App) executeLogCleanup(ctx context.Context, plan dataCleanupPlan) (map[string]int64, error) {
	var deleted int64
	switch plan.Mode {
	case dataCleanupModeOlderThanDays:
		tag, err := a.db.Exec(ctx, `
			DELETE FROM pipeline_run_logs
			WHERE timestamp < NOW() - ($1::int * INTERVAL '1 day')
		`, plan.OlderThanDays)
		if err != nil {
			return nil, err
		}
		deleted = tag.RowsAffected()
	case dataCleanupModeAllLogs:
		tag, err := a.db.Exec(ctx, `DELETE FROM pipeline_run_logs`)
		if err != nil {
			return nil, err
		}
		deleted = tag.RowsAffected()
	default:
		return nil, fmt.Errorf("unsupported logs cleanup mode")
	}
	return map[string]int64{"pipeline_run_logs": deleted}, nil
}

func (a *App) runDataCleanupJob(ctx context.Context, plan dataCleanupPlan, triggerType, scheduleID, requestedBy string) (dataCleanupJobRecord, error) {
	previewCounts, err := a.previewDataCleanup(ctx, plan)
	if err != nil {
		return dataCleanupJobRecord{}, err
	}
	previewJSON, err := json.Marshal(previewCounts)
	if err != nil {
		return dataCleanupJobRecord{}, err
	}
	jobID := uuid.NewString()
	requestedBy = strings.TrimSpace(requestedBy)
	if requestedBy == "" && strings.TrimSpace(scheduleID) != "" {
		requestedBy = "cleanup-schedule:" + strings.TrimSpace(scheduleID)
	}
	if requestedBy == "" {
		requestedBy = "system"
	}
	_, err = a.db.Exec(ctx, `
		INSERT INTO data_cleanup_jobs (
			id, schedule_id, trigger_type, status, target, mode, keep_last, older_than_days,
			backup_before_cleanup, requested_by, preview_counts
		) VALUES (
			$1, NULLIF($2, '')::uuid, $3, 'running', $4, $5, $6, $7,
			$8, $9, $10::jsonb
		)
	`, jobID, strings.TrimSpace(scheduleID), normalizeCleanupTrigger(triggerType), plan.Target, plan.Mode, plan.KeepLast, plan.OlderThanDays,
		plan.BackupBeforeCleanup, requestedBy, string(previewJSON))
	if err != nil {
		return dataCleanupJobRecord{}, err
	}

	var backupID string
	if plan.BackupBeforeCleanup {
		backup, backupErr := a.createDataBackup(ctx, dataBackupTypeFull, requestedBy)
		if backupErr != nil {
			job, _ := a.failDataCleanupJob(ctx, jobID, backupErr.Error())
			if strings.TrimSpace(scheduleID) != "" {
				_ = a.updateDataCleanupScheduleAfterJob(ctx, scheduleID, job)
			}
			return job, backupErr
		}
		backupID = backup.ID
		_, _ = a.db.Exec(ctx, `UPDATE data_cleanup_jobs SET backup_id = $2 WHERE id::text = $1`, jobID, backupID)
	}

	deletedCounts, err := a.executeDataCleanup(ctx, plan)
	if err != nil {
		job, _ := a.failDataCleanupJob(ctx, jobID, err.Error())
		if strings.TrimSpace(scheduleID) != "" {
			_ = a.updateDataCleanupScheduleAfterJob(ctx, scheduleID, job)
		}
		return job, err
	}
	deletedJSON, err := json.Marshal(deletedCounts)
	if err != nil {
		return dataCleanupJobRecord{}, err
	}
	if _, err := a.db.Exec(ctx, `
		UPDATE data_cleanup_jobs
		SET status = 'success',
			backup_id = COALESCE(NULLIF($2, '')::uuid, backup_id),
			deleted_counts = $3::jsonb,
			completed_at = NOW()
		WHERE id::text = $1
	`, jobID, backupID, string(deletedJSON)); err != nil {
		return dataCleanupJobRecord{}, err
	}
	job, err := a.getDataCleanupJob(ctx, jobID)
	if err != nil {
		return dataCleanupJobRecord{}, err
	}
	if strings.TrimSpace(scheduleID) != "" {
		_ = a.updateDataCleanupScheduleAfterJob(ctx, scheduleID, job)
	}
	return job, nil
}

func (a *App) failDataCleanupJob(ctx context.Context, jobID, message string) (dataCleanupJobRecord, error) {
	_, err := a.db.Exec(ctx, `
		UPDATE data_cleanup_jobs
		SET status = 'failure',
			error = $2,
			completed_at = NOW()
		WHERE id::text = $1
	`, jobID, strings.TrimSpace(message))
	if err != nil {
		return dataCleanupJobRecord{}, err
	}
	return a.getDataCleanupJob(ctx, jobID)
}

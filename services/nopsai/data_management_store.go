package nopsai

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (a *App) listDataBackups(ctx context.Context, limit int) ([]dataBackupRecord, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id::text, backup_type, status, file_path, file_name, content_type,
			size_bytes, checksum_sha256, requested_by, error, created_at, completed_at
		FROM data_backups
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []dataBackupRecord
	for rows.Next() {
		record, err := scanDataBackup(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (a *App) getDataBackup(ctx context.Context, backupID string) (dataBackupRecord, error) {
	return scanDataBackup(a.db.QueryRow(ctx, `
		SELECT id::text, backup_type, status, file_path, file_name, content_type,
			size_bytes, checksum_sha256, requested_by, error, created_at, completed_at
		FROM data_backups
		WHERE id::text = $1
	`, strings.TrimSpace(backupID)))
}

func (a *App) listDataCleanupJobs(ctx context.Context, limit int) ([]dataCleanupJobRecord, error) {
	rows, err := a.db.Query(ctx, baseDataCleanupJobSelect()+`
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []dataCleanupJobRecord
	for rows.Next() {
		record, err := scanDataCleanupJob(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (a *App) getDataCleanupJob(ctx context.Context, jobID string) (dataCleanupJobRecord, error) {
	return scanDataCleanupJob(a.db.QueryRow(ctx, baseDataCleanupJobSelect()+`
		WHERE id::text = $1
	`, strings.TrimSpace(jobID)))
}

func (a *App) listDataCleanupSchedules(ctx context.Context) ([]dataCleanupScheduleRecord, error) {
	rows, err := a.db.Query(ctx, baseDataCleanupScheduleSelect()+`
		ORDER BY name ASC, created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []dataCleanupScheduleRecord
	for rows.Next() {
		record, err := scanDataCleanupSchedule(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (a *App) getDataCleanupSchedule(ctx context.Context, scheduleID string) (dataCleanupScheduleRecord, error) {
	return scanDataCleanupSchedule(a.db.QueryRow(ctx, baseDataCleanupScheduleSelect()+`
		WHERE id::text = $1
	`, strings.TrimSpace(scheduleID)))
}

func (a *App) createDataCleanupSchedule(ctx context.Context, input dataCleanupScheduleInput, actor string) (dataCleanupScheduleRecord, error) {
	var scheduleID string
	if err := a.db.QueryRow(ctx, `
		INSERT INTO data_cleanup_schedules (
			name, description, enabled, target, mode, keep_last, older_than_days,
			backup_before_cleanup, cron_expression, timezone, next_run_at, created_by, updated_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $12
		)
		RETURNING id::text
	`, input.Name, input.Description, input.Enabled, input.Plan.Target, input.Plan.Mode, input.Plan.KeepLast, input.Plan.OlderThanDays,
		input.Plan.BackupBeforeCleanup, input.CronExpression, input.Timezone, input.NextRunAt, strings.TrimSpace(actor)).Scan(&scheduleID); err != nil {
		return dataCleanupScheduleRecord{}, err
	}
	return a.getDataCleanupSchedule(ctx, scheduleID)
}

func (a *App) updateDataCleanupSchedule(ctx context.Context, scheduleID string, input dataCleanupScheduleInput, actor string) (dataCleanupScheduleRecord, error) {
	tag, err := a.db.Exec(ctx, `
		UPDATE data_cleanup_schedules
		SET name = $2,
			description = $3,
			enabled = $4,
			target = $5,
			mode = $6,
			keep_last = $7,
			older_than_days = $8,
			backup_before_cleanup = $9,
			cron_expression = $10,
			timezone = $11,
			next_run_at = $12,
			updated_by = $13,
			updated_at = NOW()
		WHERE id::text = $1
	`, strings.TrimSpace(scheduleID), input.Name, input.Description, input.Enabled, input.Plan.Target, input.Plan.Mode, input.Plan.KeepLast,
		input.Plan.OlderThanDays, input.Plan.BackupBeforeCleanup, input.CronExpression, input.Timezone, input.NextRunAt, strings.TrimSpace(actor))
	if err != nil {
		return dataCleanupScheduleRecord{}, err
	}
	if tag.RowsAffected() == 0 {
		return dataCleanupScheduleRecord{}, pgx.ErrNoRows
	}
	return a.getDataCleanupSchedule(ctx, scheduleID)
}

func (a *App) setDataCleanupScheduleEnabled(ctx context.Context, scheduleID string, enabled bool, nextRunAt *time.Time, actor string) (dataCleanupScheduleRecord, error) {
	tag, err := a.db.Exec(ctx, `
		UPDATE data_cleanup_schedules
		SET enabled = $2,
			next_run_at = $3,
			updated_by = $4,
			updated_at = NOW()
		WHERE id::text = $1
	`, strings.TrimSpace(scheduleID), enabled, nextRunAt, strings.TrimSpace(actor))
	if err != nil {
		return dataCleanupScheduleRecord{}, err
	}
	if tag.RowsAffected() == 0 {
		return dataCleanupScheduleRecord{}, pgx.ErrNoRows
	}
	return a.getDataCleanupSchedule(ctx, scheduleID)
}

func (a *App) updateDataCleanupScheduleAfterJob(ctx context.Context, scheduleID string, job dataCleanupJobRecord) error {
	countsJSON, _ := json.Marshal(job.DeletedCounts)
	_, err := a.db.Exec(ctx, `
		UPDATE data_cleanup_schedules
		SET last_run_at = COALESCE($2, NOW()),
			last_job_id = $3::uuid,
			last_status = $4,
			last_deleted_counts = $5::jsonb,
			last_error = $6,
			updated_at = NOW()
		WHERE id::text = $1
	`, strings.TrimSpace(scheduleID), job.CompletedAt, job.ID, job.Status, string(countsJSON), job.Error)
	return err
}

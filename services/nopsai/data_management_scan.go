package nopsai

import (
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
)

func dataCleanupPlanFromSchedule(record dataCleanupScheduleRecord) dataCleanupPlan {
	return dataCleanupPlan{
		Target:              record.Target,
		Mode:                record.Mode,
		KeepLast:            record.KeepLast,
		OlderThanDays:       record.OlderThanDays,
		BackupBeforeCleanup: record.BackupBeforeCleanup,
	}
}

func baseDataCleanupJobSelect() string {
	return `
		SELECT id::text, COALESCE(schedule_id::text, ''), trigger_type, status,
			target, mode, keep_last, older_than_days, backup_before_cleanup,
			COALESCE(backup_id::text, ''), requested_by,
			preview_counts::text, deleted_counts::text, error,
			started_at, completed_at, created_at
		FROM data_cleanup_jobs
	`
}

func baseDataCleanupScheduleSelect() string {
	return `
		SELECT id::text, name, description, enabled, target, mode, keep_last, older_than_days,
			backup_before_cleanup, cron_expression, timezone, next_run_at, last_run_at,
			COALESCE(last_job_id::text, ''), last_status, last_deleted_counts::text, last_error,
			COALESCE(source, 'database'), config_repo_id, COALESCE(config_source_path, ''),
			COALESCE(config_source_commit_sha, ''), managed_by_config_repo,
			created_by, updated_by, created_at, updated_at
		FROM data_cleanup_schedules
	`
}

type scanner interface {
	Scan(dest ...any) error
}

func scanDataBackup(scanner scanner) (dataBackupRecord, error) {
	var record dataBackupRecord
	var completedAt sql.NullTime
	if err := scanner.Scan(
		&record.ID, &record.BackupType, &record.Status, &record.FilePath, &record.FileName, &record.ContentType,
		&record.SizeBytes, &record.ChecksumSHA256, &record.RequestedBy, &record.Error, &record.CreatedAt, &completedAt,
	); err != nil {
		return record, err
	}
	if completedAt.Valid {
		t := completedAt.Time
		record.CompletedAt = &t
	}
	return record, nil
}

func scanDataCleanupJob(scanner scanner) (dataCleanupJobRecord, error) {
	var record dataCleanupJobRecord
	var previewCountsRaw, deletedCountsRaw string
	var completedAt sql.NullTime
	if err := scanner.Scan(
		&record.ID, &record.ScheduleID, &record.TriggerType, &record.Status,
		&record.Target, &record.Mode, &record.KeepLast, &record.OlderThanDays, &record.BackupBeforeCleanup,
		&record.BackupID, &record.RequestedBy,
		&previewCountsRaw, &deletedCountsRaw, &record.Error,
		&record.StartedAt, &completedAt, &record.CreatedAt,
	); err != nil {
		return record, err
	}
	record.PreviewCounts = decodeCounts(previewCountsRaw)
	record.DeletedCounts = decodeCounts(deletedCountsRaw)
	if completedAt.Valid {
		t := completedAt.Time
		record.CompletedAt = &t
	}
	return record, nil
}

func scanDataCleanupSchedule(scanner scanner) (dataCleanupScheduleRecord, error) {
	var record dataCleanupScheduleRecord
	var nextRunAt, lastRunAt sql.NullTime
	var configRepoID sql.NullInt64
	var lastCountsRaw string
	if err := scanner.Scan(
		&record.ID, &record.Name, &record.Description, &record.Enabled, &record.Target, &record.Mode, &record.KeepLast, &record.OlderThanDays,
		&record.BackupBeforeCleanup, &record.CronExpression, &record.Timezone, &nextRunAt, &lastRunAt,
		&record.LastJobID, &record.LastStatus, &lastCountsRaw, &record.LastError,
		&record.Source, &configRepoID, &record.ConfigSourcePath,
		&record.ConfigSourceCommitSHA, &record.ManagedByConfigRepo,
		&record.CreatedBy, &record.UpdatedBy, &record.CreatedAt, &record.UpdatedAt,
	); err != nil {
		return record, err
	}
	record.LastDeletedCounts = decodeCounts(lastCountsRaw)
	if configRepoID.Valid {
		id := configRepoID.Int64
		record.ConfigRepoID = &id
	}
	if nextRunAt.Valid {
		t := nextRunAt.Time
		record.NextRunAt = &t
	}
	if lastRunAt.Valid {
		t := lastRunAt.Time
		record.LastRunAt = &t
	}
	return record, nil
}

func runCleanupCountSQL(candidateCTE string) string {
	return candidateCTE + `
		SELECT
			(SELECT COUNT(*) FROM candidate_runs),
			(SELECT COUNT(*) FROM task_runs WHERE run_id IN (SELECT run_id FROM candidate_runs)),
			(SELECT COUNT(*) FROM step_runs WHERE run_id IN (SELECT run_id FROM candidate_runs)),
			(SELECT COUNT(*) FROM pipeline_run_logs WHERE run_id IN (SELECT run_id FROM candidate_runs)),
			(SELECT COUNT(*) FROM pipeline_run_checkpoints WHERE run_id IN (SELECT run_id FROM candidate_runs)),
			(SELECT COUNT(*) FROM pipeline_approvals WHERE run_id IN (SELECT run_id FROM candidate_runs)),
			(SELECT COUNT(*) FROM pipeline_run_knowledge_contexts WHERE run_id IN (SELECT run_id FROM candidate_runs)),
			(SELECT COUNT(*) FROM pipeline_run_outputs WHERE run_id IN (SELECT run_id FROM candidate_runs))
	`
}

func runCleanupDeleteSQL(candidateCTE string) string {
	return candidateCTE + `
		, deleted_runs AS (
			DELETE FROM pipeline_runs
			WHERE run_id IN (SELECT run_id FROM candidate_runs)
			RETURNING run_id
		)
		SELECT COUNT(*) FROM deleted_runs
	`
}

func scanRunCleanupCounts(row pgx.Row) (map[string]int64, error) {
	var pipelineRuns, taskRuns, stepRuns, logs, checkpoints, approvals, knowledgeContexts, outputs int64
	err := row.Scan(
		&pipelineRuns,
		&taskRuns,
		&stepRuns,
		&logs,
		&checkpoints,
		&approvals,
		&knowledgeContexts,
		&outputs,
	)
	if err != nil {
		return nil, err
	}
	return map[string]int64{
		"pipeline_runs":                   pipelineRuns,
		"task_runs":                       taskRuns,
		"step_runs":                       stepRuns,
		"pipeline_run_logs":               logs,
		"pipeline_run_checkpoints":        checkpoints,
		"pipeline_approvals":              approvals,
		"pipeline_run_knowledge_contexts": knowledgeContexts,
		"pipeline_run_outputs":            outputs,
	}, nil
}

func decodeCounts(raw string) map[string]int64 {
	counts := map[string]int64{}
	if strings.TrimSpace(raw) == "" {
		return counts
	}
	_ = json.Unmarshal([]byte(raw), &counts)
	if counts == nil {
		return map[string]int64{}
	}
	return counts
}

func totalCount(counts map[string]int64) int64 {
	var total int64
	for _, count := range counts {
		total += count
	}
	return total
}

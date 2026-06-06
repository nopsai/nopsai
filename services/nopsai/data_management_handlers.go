package nopsai

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"nopsai/pkg/httpapi"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

func (a *App) dataBackupDirectory() string {
	cfg := a.getConfigSnapshot()
	if strings.TrimSpace(cfg.DataBackupDir) != "" {
		return strings.TrimSpace(cfg.DataBackupDir)
	}
	return defaultDataBackupDir
}

func (a *App) handleListDataBackups(w http.ResponseWriter, r *http.Request) {
	records, err := a.listDataBackups(r.Context(), queryLimit(r, 100, 500))
	if err != nil {
		log.Error().Err(err).Msg("Failed to list data backups")
		http.Error(w, "Failed to list backups", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (a *App) handleCreateDataBackup(w http.ResponseWriter, r *http.Request) {
	var req dataBackupRequest
	if err := httpapi.DecodeOptionalJSON(r, &req); err != nil {
		http.Error(w, "Invalid backup payload", http.StatusBadRequest)
		return
	}
	backupType, err := normalizeDataBackupType(firstNonEmptyString(req.BackupType, req.Type))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	record, err := a.createDataBackup(r.Context(), backupType, actorIDFromRequest(r))
	if err != nil {
		log.Error().Err(err).Str("backup_type", backupType).Msg("Failed to create data backup")
		a.auditDataManagementAction(r.Context(), r, "data.backup.created", "data_backup:"+record.ID, "failure", map[string]any{
			"backup_type": backupType,
			"error":       err.Error(),
		})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.auditDataManagementAction(r.Context(), r, "data.backup.created", "data_backup:"+record.ID, "success", map[string]any{
		"backup_type": record.BackupType,
		"size_bytes":  record.SizeBytes,
	})
	writeJSON(w, http.StatusCreated, record)
}

func (a *App) handleDownloadDataBackup(w http.ResponseWriter, r *http.Request) {
	record, err := a.getDataBackup(r.Context(), r.PathValue("backupID"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "backup not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to load backup", http.StatusInternalServerError)
		return
	}
	if record.Status != dataBackupStatusSuccess {
		http.Error(w, "backup is not available for download", http.StatusConflict)
		return
	}
	if !a.backupFilePathAllowed(record.FilePath) {
		http.Error(w, "backup file path is invalid", http.StatusInternalServerError)
		return
	}
	file, err := os.Open(record.FilePath)
	if err != nil {
		http.Error(w, "backup file not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	a.auditDataManagementAction(r.Context(), r, "data.backup.downloaded", "data_backup:"+record.ID, "success", map[string]any{
		"backup_type": record.BackupType,
		"size_bytes":  record.SizeBytes,
	})
	w.Header().Set("Content-Type", firstNonEmptyString(record.ContentType, "application/gzip"))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeDownloadFileName(record.FileName)))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, record.FileName, record.CreatedAt, file)
}

func (a *App) handleDeleteDataBackup(w http.ResponseWriter, r *http.Request) {
	record, err := a.getDataBackup(r.Context(), r.PathValue("backupID"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "backup not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to load backup", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(record.FilePath) != "" && a.backupFilePathAllowed(record.FilePath) {
		if err := os.Remove(record.FilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			http.Error(w, "Failed to delete backup file", http.StatusInternalServerError)
			return
		}
	}
	if _, err := a.db.Exec(r.Context(), `DELETE FROM data_backups WHERE id::text = $1`, record.ID); err != nil {
		http.Error(w, "Failed to delete backup", http.StatusInternalServerError)
		return
	}
	a.auditDataManagementAction(r.Context(), r, "data.backup.deleted", "data_backup:"+record.ID, "success", map[string]any{
		"backup_type": record.BackupType,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handlePreviewDataCleanup(w http.ResponseWriter, r *http.Request) {
	var req dataCleanupRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid cleanup payload", http.StatusBadRequest)
		return
	}
	plan, err := normalizeDataCleanupPlan(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	counts, err := a.previewDataCleanup(r.Context(), plan)
	if err != nil {
		log.Error().Err(err).Msg("Failed to preview data cleanup")
		http.Error(w, "Failed to preview cleanup", http.StatusInternalServerError)
		return
	}
	a.auditDataManagementAction(r.Context(), r, "data.cleanup.previewed", "data_cleanup:"+plan.Target, "success", map[string]any{
		"target":     plan.Target,
		"mode":       plan.Mode,
		"total_rows": totalCount(counts),
	})
	writeJSON(w, http.StatusOK, dataCleanupPreviewResponse{Plan: plan, Counts: counts, TotalRows: totalCount(counts)})
}

func (a *App) handleRunDataCleanup(w http.ResponseWriter, r *http.Request) {
	var req dataCleanupRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid cleanup payload", http.StatusBadRequest)
		return
	}
	plan, err := normalizeDataCleanupPlan(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	job, err := a.runDataCleanupJob(r.Context(), plan, dataCleanupTriggerManual, "", actorIDFromRequest(r))
	if err != nil {
		log.Error().Err(err).Msg("Failed to run data cleanup")
		a.auditDataManagementAction(r.Context(), r, "data.cleanup.executed", "data_cleanup_job:"+job.ID, "failure", map[string]any{
			"target": plan.Target,
			"mode":   plan.Mode,
			"error":  err.Error(),
		})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.auditDataManagementAction(r.Context(), r, "data.cleanup.executed", "data_cleanup_job:"+job.ID, "success", map[string]any{
		"target":       job.Target,
		"mode":         job.Mode,
		"deleted_rows": totalCount(job.DeletedCounts),
	})
	writeJSON(w, http.StatusCreated, job)
}

func (a *App) handleListDataCleanupJobs(w http.ResponseWriter, r *http.Request) {
	records, err := a.listDataCleanupJobs(r.Context(), queryLimit(r, 100, 500))
	if err != nil {
		log.Error().Err(err).Msg("Failed to list data cleanup jobs")
		http.Error(w, "Failed to list cleanup jobs", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (a *App) handleListDataCleanupSchedules(w http.ResponseWriter, r *http.Request) {
	records, err := a.listDataCleanupSchedules(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("Failed to list data cleanup schedules")
		http.Error(w, "Failed to list cleanup schedules", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (a *App) handleCreateDataCleanupSchedule(w http.ResponseWriter, r *http.Request) {
	var req dataCleanupScheduleRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid cleanup schedule payload", http.StatusBadRequest)
		return
	}
	input, err := normalizeDataCleanupScheduleInput(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	record, err := a.createDataCleanupSchedule(r.Context(), input, actorIDFromRequest(r))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create data cleanup schedule")
		http.Error(w, "Failed to create cleanup schedule", http.StatusInternalServerError)
		return
	}
	a.auditDataManagementAction(r.Context(), r, "data.cleanup_schedule.created", "data_cleanup_schedule:"+record.ID, "success", map[string]any{
		"target": record.Target,
		"mode":   record.Mode,
	})
	writeJSON(w, http.StatusCreated, record)
}

func (a *App) handleUpdateDataCleanupSchedule(w http.ResponseWriter, r *http.Request) {
	var req dataCleanupScheduleRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid cleanup schedule payload", http.StatusBadRequest)
		return
	}
	input, err := normalizeDataCleanupScheduleInput(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	record, err := a.updateDataCleanupSchedule(r.Context(), r.PathValue("scheduleID"), input, actorIDFromRequest(r))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "cleanup schedule not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Msg("Failed to update data cleanup schedule")
		http.Error(w, "Failed to update cleanup schedule", http.StatusInternalServerError)
		return
	}
	a.auditDataManagementAction(r.Context(), r, "data.cleanup_schedule.updated", "data_cleanup_schedule:"+record.ID, "success", map[string]any{
		"target": record.Target,
		"mode":   record.Mode,
	})
	writeJSON(w, http.StatusOK, record)
}

func (a *App) handleDeleteDataCleanupSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := strings.TrimSpace(r.PathValue("scheduleID"))
	if scheduleID == "" {
		http.Error(w, "schedule id is required", http.StatusBadRequest)
		return
	}
	tag, err := a.db.Exec(r.Context(), `DELETE FROM data_cleanup_schedules WHERE id::text = $1`, scheduleID)
	if err != nil {
		http.Error(w, "Failed to delete cleanup schedule", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "cleanup schedule not found", http.StatusNotFound)
		return
	}
	a.auditDataManagementAction(r.Context(), r, "data.cleanup_schedule.deleted", "data_cleanup_schedule:"+scheduleID, "success", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleRunDataCleanupScheduleNow(w http.ResponseWriter, r *http.Request) {
	record, err := a.getDataCleanupSchedule(r.Context(), r.PathValue("scheduleID"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "cleanup schedule not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to load cleanup schedule", http.StatusInternalServerError)
		return
	}
	job, err := a.runDataCleanupJob(r.Context(), dataCleanupPlanFromSchedule(record), dataCleanupTriggerScheduled, record.ID, actorIDFromRequest(r))
	if err != nil {
		log.Error().Err(err).Str("schedule_id", record.ID).Msg("Failed to run cleanup schedule")
		a.auditDataManagementAction(r.Context(), r, "data.cleanup.executed", "data_cleanup_job:"+job.ID, "failure", map[string]any{
			"schedule_id": record.ID,
			"target":      record.Target,
			"mode":        record.Mode,
			"error":       err.Error(),
		})
		a.auditDataManagementAction(r.Context(), r, "data.cleanup_schedule.run_manually", "data_cleanup_schedule:"+record.ID, "failure", map[string]any{
			"job_id": job.ID,
			"error":  err.Error(),
		})
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.auditDataManagementAction(r.Context(), r, "data.cleanup.executed", "data_cleanup_job:"+job.ID, "success", map[string]any{
		"schedule_id":  record.ID,
		"target":       job.Target,
		"mode":         job.Mode,
		"deleted_rows": totalCount(job.DeletedCounts),
	})
	a.auditDataManagementAction(r.Context(), r, "data.cleanup_schedule.run_manually", "data_cleanup_schedule:"+record.ID, "success", map[string]any{
		"job_id":       job.ID,
		"deleted_rows": totalCount(job.DeletedCounts),
	})
	writeJSON(w, http.StatusAccepted, job)
}

func (a *App) handleEnableDataCleanupSchedule(w http.ResponseWriter, r *http.Request) {
	a.handleSetDataCleanupScheduleEnabled(w, r, true)
}

func (a *App) handleDisableDataCleanupSchedule(w http.ResponseWriter, r *http.Request) {
	a.handleSetDataCleanupScheduleEnabled(w, r, false)
}

func (a *App) handleSetDataCleanupScheduleEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	record, err := a.getDataCleanupSchedule(r.Context(), r.PathValue("scheduleID"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "cleanup schedule not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to load cleanup schedule", http.StatusInternalServerError)
		return
	}
	var nextRunAt *time.Time
	if enabled {
		next, err := nextScheduleRunAt(record.CronExpression, record.Timezone, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		nextRunAt = &next
	}
	updated, err := a.setDataCleanupScheduleEnabled(r.Context(), record.ID, enabled, nextRunAt, actorIDFromRequest(r))
	if err != nil {
		http.Error(w, "Failed to update cleanup schedule", http.StatusInternalServerError)
		return
	}
	action := "data.cleanup_schedule.disabled"
	if enabled {
		action = "data.cleanup_schedule.enabled"
	}
	a.auditDataManagementAction(r.Context(), r, action, "data_cleanup_schedule:"+record.ID, "success", nil)
	writeJSON(w, http.StatusOK, updated)
}

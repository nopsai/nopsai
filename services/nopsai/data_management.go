package nopsai

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nopsai/pkg/httpapi"
	"nopsai/services/nopsai/pkg/audit"
	"nopsai/services/nopsai/pkg/auth"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

const (
	dataBackupTypeFull = "full"
	dataBackupTypeRuns = "runs"
	dataBackupTypeLogs = "logs"

	dataBackupStatusRunning = "running"
	dataBackupStatusSuccess = "success"
	dataBackupStatusFailure = "failure"

	dataCleanupTargetRuns = "runs"
	dataCleanupTargetLogs = "logs"

	dataCleanupModeKeepLast        = "keep_last"
	dataCleanupModeOlderThanDays   = "older_than_days"
	dataCleanupModeAllTerminalRuns = "all_terminal_runs"
	dataCleanupModeAllLogs         = "all_logs"

	dataCleanupTriggerManual    = "manual"
	dataCleanupTriggerScheduled = "scheduled"

	dataCleanupWorkerPollInterval = time.Minute
	dataCleanupWorkerBatchSize    = 10

	defaultDataBackupDir = "/data/backups"
	defaultCleanupCron   = "0 2 * * 0"
)

var terminalRunStatusesForCleanup = []string{"success", "failure", "cancelled", "timed_out", "failure (ignored)", "rejected", "skipped"}

var runBackupTables = []string{
	"pipeline_runs",
	"step_runs",
	"task_runs",
	"pipeline_run_checkpoints",
	"pipeline_approvals",
	"pipeline_run_logs",
	"pipeline_run_knowledge_contexts",
	"external_trigger_invocations",
}

type dataBackupRecord struct {
	ID             string     `json:"id"`
	BackupType     string     `json:"backup_type"`
	Status         string     `json:"status"`
	FilePath       string     `json:"file_path,omitempty"`
	FileName       string     `json:"file_name,omitempty"`
	ContentType    string     `json:"content_type"`
	SizeBytes      int64      `json:"size_bytes"`
	ChecksumSHA256 string     `json:"checksum_sha256,omitempty"`
	RequestedBy    string     `json:"requested_by,omitempty"`
	Error          string     `json:"error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

type dataBackupRequest struct {
	BackupType string `json:"backup_type"`
	Type       string `json:"type"`
}

type dataCleanupRequest struct {
	Target              string `json:"target"`
	Mode                string `json:"mode"`
	KeepLast            int    `json:"keep_last"`
	OlderThanDays       int    `json:"older_than_days"`
	BackupBeforeCleanup bool   `json:"backup_before_cleanup"`
}

type dataCleanupPlan struct {
	Target              string `json:"target"`
	Mode                string `json:"mode"`
	KeepLast            int    `json:"keep_last,omitempty"`
	OlderThanDays       int    `json:"older_than_days,omitempty"`
	BackupBeforeCleanup bool   `json:"backup_before_cleanup"`
}

type dataCleanupPreviewResponse struct {
	Plan      dataCleanupPlan  `json:"plan"`
	Counts    map[string]int64 `json:"counts"`
	TotalRows int64            `json:"total_rows"`
}

type dataCleanupJobRecord struct {
	ID                  string           `json:"id"`
	ScheduleID          string           `json:"schedule_id,omitempty"`
	TriggerType         string           `json:"trigger_type"`
	Status              string           `json:"status"`
	Target              string           `json:"target"`
	Mode                string           `json:"mode"`
	KeepLast            int              `json:"keep_last,omitempty"`
	OlderThanDays       int              `json:"older_than_days,omitempty"`
	BackupBeforeCleanup bool             `json:"backup_before_cleanup"`
	BackupID            string           `json:"backup_id,omitempty"`
	RequestedBy         string           `json:"requested_by,omitempty"`
	PreviewCounts       map[string]int64 `json:"preview_counts"`
	DeletedCounts       map[string]int64 `json:"deleted_counts"`
	Error               string           `json:"error,omitempty"`
	StartedAt           time.Time        `json:"started_at"`
	CompletedAt         *time.Time       `json:"completed_at,omitempty"`
	CreatedAt           time.Time        `json:"created_at"`
}

type dataCleanupScheduleRequest struct {
	Name                string `json:"name"`
	Description         string `json:"description"`
	Enabled             *bool  `json:"enabled"`
	Target              string `json:"target"`
	Mode                string `json:"mode"`
	KeepLast            int    `json:"keep_last"`
	OlderThanDays       int    `json:"older_than_days"`
	BackupBeforeCleanup *bool  `json:"backup_before_cleanup"`
	CronExpression      string `json:"cron_expression"`
	Cron                string `json:"cron"`
	Timezone            string `json:"timezone"`
}

type dataCleanupScheduleInput struct {
	Name           string
	Description    string
	Enabled        bool
	Plan           dataCleanupPlan
	CronExpression string
	Timezone       string
	NextRunAt      *time.Time
}

type dataCleanupScheduleRecord struct {
	ID                  string           `json:"id"`
	Name                string           `json:"name"`
	Description         string           `json:"description,omitempty"`
	Enabled             bool             `json:"enabled"`
	Target              string           `json:"target"`
	Mode                string           `json:"mode"`
	KeepLast            int              `json:"keep_last,omitempty"`
	OlderThanDays       int              `json:"older_than_days,omitempty"`
	BackupBeforeCleanup bool             `json:"backup_before_cleanup"`
	CronExpression      string           `json:"cron_expression"`
	Timezone            string           `json:"timezone"`
	NextRunAt           *time.Time       `json:"next_run_at,omitempty"`
	LastRunAt           *time.Time       `json:"last_run_at,omitempty"`
	LastJobID           string           `json:"last_job_id,omitempty"`
	LastStatus          string           `json:"last_status,omitempty"`
	LastDeletedCounts   map[string]int64 `json:"last_deleted_counts"`
	LastError           string           `json:"last_error,omitempty"`
	CreatedBy           string           `json:"created_by,omitempty"`
	UpdatedBy           string           `json:"updated_by,omitempty"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
}

type backupJSONLine struct {
	Kind        string          `json:"kind"`
	BackupID    string          `json:"backup_id,omitempty"`
	BackupType  string          `json:"backup_type,omitempty"`
	GeneratedAt time.Time       `json:"generated_at,omitempty"`
	Table       string          `json:"table,omitempty"`
	Tables      []string        `json:"tables,omitempty"`
	Row         json.RawMessage `json:"row,omitempty"`
}

type countingWriter struct {
	count int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.count += int64(len(p))
	return len(p), nil
}

func normalizeDataBackupType(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", dataBackupTypeFull, "db", "database":
		return dataBackupTypeFull, nil
	case dataBackupTypeRuns, "run":
		return dataBackupTypeRuns, nil
	case dataBackupTypeLogs, "log":
		return dataBackupTypeLogs, nil
	default:
		return "", fmt.Errorf("backup_type must be one of full, runs, or logs")
	}
}

func normalizeDataCleanupPlan(req dataCleanupRequest) (dataCleanupPlan, error) {
	target := strings.ToLower(strings.TrimSpace(req.Target))
	if target == "" {
		target = dataCleanupTargetRuns
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	mode = strings.ReplaceAll(mode, "-", "_")
	mode = strings.ReplaceAll(mode, " ", "_")

	switch target {
	case dataCleanupTargetRuns:
		if mode == "" {
			mode = dataCleanupModeKeepLast
		}
		switch mode {
		case dataCleanupModeKeepLast, "keep":
			if req.KeepLast <= 0 {
				return dataCleanupPlan{}, fmt.Errorf("keep_last must be greater than zero")
			}
			mode = dataCleanupModeKeepLast
		case dataCleanupModeOlderThanDays, "older_than":
			if req.OlderThanDays <= 0 {
				return dataCleanupPlan{}, fmt.Errorf("older_than_days must be greater than zero")
			}
			mode = dataCleanupModeOlderThanDays
		case dataCleanupModeAllTerminalRuns, "all_runs", "all_terminal":
			mode = dataCleanupModeAllTerminalRuns
		default:
			return dataCleanupPlan{}, fmt.Errorf("runs cleanup mode must be keep_last, older_than_days, or all_terminal_runs")
		}
	case dataCleanupTargetLogs:
		if mode == "" {
			mode = dataCleanupModeOlderThanDays
		}
		switch mode {
		case dataCleanupModeOlderThanDays, "older_than":
			if req.OlderThanDays <= 0 {
				return dataCleanupPlan{}, fmt.Errorf("older_than_days must be greater than zero")
			}
			mode = dataCleanupModeOlderThanDays
		case dataCleanupModeAllLogs, "all":
			mode = dataCleanupModeAllLogs
		default:
			return dataCleanupPlan{}, fmt.Errorf("logs cleanup mode must be older_than_days or all_logs")
		}
	default:
		return dataCleanupPlan{}, fmt.Errorf("target must be runs or logs")
	}

	plan := dataCleanupPlan{
		Target:              target,
		Mode:                mode,
		BackupBeforeCleanup: req.BackupBeforeCleanup,
	}
	if mode == dataCleanupModeKeepLast {
		plan.KeepLast = req.KeepLast
	}
	if mode == dataCleanupModeOlderThanDays {
		plan.OlderThanDays = req.OlderThanDays
	}
	return plan, nil
}

func defaultDataCleanupScheduleRequest() dataCleanupScheduleRequest {
	enabled := true
	backup := true
	return dataCleanupScheduleRequest{
		Name:                "Weekly cleanup",
		Enabled:             &enabled,
		Target:              dataCleanupTargetRuns,
		Mode:                dataCleanupModeKeepLast,
		KeepLast:            30,
		BackupBeforeCleanup: &backup,
		CronExpression:      defaultCleanupCron,
		Timezone:            "UTC",
	}
}

func normalizeDataCleanupScheduleInput(req dataCleanupScheduleRequest) (dataCleanupScheduleInput, error) {
	defaults := defaultDataCleanupScheduleRequest()
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = defaults.Name
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	backupBeforeCleanup := true
	if req.BackupBeforeCleanup != nil {
		backupBeforeCleanup = *req.BackupBeforeCleanup
	}
	target := firstNonEmptyString(req.Target, defaults.Target)
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		if strings.EqualFold(target, dataCleanupTargetLogs) {
			mode = dataCleanupModeOlderThanDays
		} else {
			mode = defaults.Mode
		}
	}
	keepLast := req.KeepLast
	if keepLast == 0 && strings.EqualFold(mode, dataCleanupModeKeepLast) {
		keepLast = defaults.KeepLast
	}
	olderThanDays := req.OlderThanDays
	if olderThanDays == 0 && strings.EqualFold(mode, dataCleanupModeOlderThanDays) {
		olderThanDays = 30
	}
	plan, err := normalizeDataCleanupPlan(dataCleanupRequest{
		Target:              target,
		Mode:                mode,
		KeepLast:            keepLast,
		OlderThanDays:       olderThanDays,
		BackupBeforeCleanup: backupBeforeCleanup,
	})
	if err != nil {
		return dataCleanupScheduleInput{}, err
	}
	cronExpression := strings.TrimSpace(firstNonEmptyString(req.CronExpression, req.Cron, defaults.CronExpression))
	timezone := firstNonEmptyString(req.Timezone, defaults.Timezone)
	if _, err := time.LoadLocation(timezone); err != nil {
		return dataCleanupScheduleInput{}, fmt.Errorf("invalid timezone %q: %w", timezone, err)
	}
	nextRunAt, err := nextScheduleRunAt(cronExpression, timezone, time.Now())
	if err != nil {
		return dataCleanupScheduleInput{}, err
	}
	return dataCleanupScheduleInput{
		Name:           name,
		Description:    strings.TrimSpace(req.Description),
		Enabled:        enabled,
		Plan:           plan,
		CronExpression: cronExpression,
		Timezone:       timezone,
		NextRunAt:      &nextRunAt,
	}, nil
}

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

func (a *App) createDataBackup(ctx context.Context, backupType, requestedBy string) (dataBackupRecord, error) {
	backupType, err := normalizeDataBackupType(backupType)
	if err != nil {
		return dataBackupRecord{}, err
	}
	backupID := uuid.NewString()
	now := time.Now().UTC()
	fileName := fmt.Sprintf("nopsai-%s-%s-%s.jsonl.gz", backupType, now.Format("20060102T150405Z"), backupID[:8])
	dir := a.dataBackupDirectory()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return dataBackupRecord{}, err
	}
	filePath := filepath.Join(dir, fileName)

	if _, err := a.db.Exec(ctx, `
		INSERT INTO data_backups (id, backup_type, status, file_path, file_name, requested_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, backupID, backupType, dataBackupStatusRunning, filePath, fileName, strings.TrimSpace(requestedBy), now); err != nil {
		return dataBackupRecord{}, err
	}

	sizeBytes, checksum, err := a.writeDataBackupFile(ctx, backupID, backupType, filePath)
	if err != nil {
		_ = os.Remove(filePath)
		_, _ = a.db.Exec(ctx, `
			UPDATE data_backups
			SET status = $2, error = $3, completed_at = NOW()
			WHERE id::text = $1
		`, backupID, dataBackupStatusFailure, err.Error())
		record, _ := a.getDataBackup(ctx, backupID)
		return record, err
	}
	if _, err := a.db.Exec(ctx, `
		UPDATE data_backups
		SET status = $2,
			size_bytes = $3,
			checksum_sha256 = $4,
			completed_at = NOW()
		WHERE id::text = $1
	`, backupID, dataBackupStatusSuccess, sizeBytes, checksum); err != nil {
		return dataBackupRecord{}, err
	}
	return a.getDataBackup(ctx, backupID)
}

func (a *App) writeDataBackupFile(ctx context.Context, backupID, backupType, filePath string) (int64, string, error) {
	tables, err := a.dataBackupTables(ctx, backupType)
	if err != nil {
		return 0, "", err
	}
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()

	hasher := sha256.New()
	counter := &countingWriter{}
	gzipWriter := gzip.NewWriter(io.MultiWriter(file, hasher, counter))
	gzipWriter.Name = filepath.Base(filePath)
	gzipWriter.ModTime = time.Now().UTC()
	encoder := json.NewEncoder(gzipWriter)

	if err := encoder.Encode(backupJSONLine{
		Kind:        "manifest",
		BackupID:    backupID,
		BackupType:  backupType,
		GeneratedAt: time.Now().UTC(),
		Tables:      tables,
	}); err != nil {
		gzipWriter.Close()
		return 0, "", err
	}
	for _, table := range tables {
		if err := encoder.Encode(backupJSONLine{Kind: "table", Table: table}); err != nil {
			gzipWriter.Close()
			return 0, "", err
		}
		if err := a.writeDataBackupTable(ctx, encoder, table); err != nil {
			gzipWriter.Close()
			return 0, "", err
		}
	}
	if err := gzipWriter.Close(); err != nil {
		return 0, "", err
	}
	return counter.count, hex.EncodeToString(hasher.Sum(nil)), nil
}

func (a *App) writeDataBackupTable(ctx context.Context, encoder *json.Encoder, table string) error {
	query := fmt.Sprintf(`SELECT row_to_json(t)::text FROM (SELECT * FROM %s) t`, quoteSQLIdentifier(table))
	rows, err := a.db.Query(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var rowJSON string
		if err := rows.Scan(&rowJSON); err != nil {
			return err
		}
		if err := encoder.Encode(backupJSONLine{Kind: "row", Table: table, Row: json.RawMessage(rowJSON)}); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (a *App) dataBackupTables(ctx context.Context, backupType string) ([]string, error) {
	switch backupType {
	case dataBackupTypeRuns:
		return runBackupTables, nil
	case dataBackupTypeLogs:
		return []string{"pipeline_run_logs"}, nil
	case dataBackupTypeFull:
		rows, err := a.db.Query(ctx, `
			SELECT table_name
			FROM information_schema.tables
			WHERE table_schema = 'public'
			  AND table_type = 'BASE TABLE'
			ORDER BY table_name ASC
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var tables []string
		for rows.Next() {
			var table string
			if err := rows.Scan(&table); err != nil {
				return nil, err
			}
			tables = append(tables, table)
		}
		return tables, rows.Err()
	default:
		return nil, fmt.Errorf("unsupported backup type %q", backupType)
	}
}

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
	var lastCountsRaw string
	if err := scanner.Scan(
		&record.ID, &record.Name, &record.Description, &record.Enabled, &record.Target, &record.Mode, &record.KeepLast, &record.OlderThanDays,
		&record.BackupBeforeCleanup, &record.CronExpression, &record.Timezone, &nextRunAt, &lastRunAt,
		&record.LastJobID, &record.LastStatus, &lastCountsRaw, &record.LastError,
		&record.CreatedBy, &record.UpdatedBy, &record.CreatedAt, &record.UpdatedAt,
	); err != nil {
		return record, err
	}
	record.LastDeletedCounts = decodeCounts(lastCountsRaw)
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
			(SELECT COUNT(*) FROM pipeline_run_knowledge_contexts WHERE run_id IN (SELECT run_id FROM candidate_runs))
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
	var pipelineRuns, taskRuns, stepRuns, logs, checkpoints, approvals, knowledgeContexts int64
	err := row.Scan(
		&pipelineRuns,
		&taskRuns,
		&stepRuns,
		&logs,
		&checkpoints,
		&approvals,
		&knowledgeContexts,
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

func queryLimit(r *http.Request, defaultLimit, maxLimit int) int {
	if r == nil {
		return defaultLimit
	}
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return defaultLimit
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return defaultLimit
	}
	if parsed > maxLimit {
		return maxLimit
	}
	return parsed
}

func normalizeCleanupTrigger(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), dataCleanupTriggerScheduled) {
		return dataCleanupTriggerScheduled
	}
	return dataCleanupTriggerManual
}

func quoteSQLIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func sanitizeDownloadFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "nopsai-backup.jsonl.gz"
	}
	return strings.ReplaceAll(name, `"`, "")
}

func (a *App) backupFilePathAllowed(filePath string) bool {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return false
	}
	baseDir, err := filepath.Abs(a.dataBackupDirectory())
	if err != nil {
		return false
	}
	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(baseDir, absFile)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

func (a *App) auditDataManagementAction(ctx context.Context, r *http.Request, action, resource, result string, metadata map[string]any) {
	if a == nil || a.auditLogger == nil {
		return
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	entry := audit.Entry{
		ActorSub:   "system",
		ActorEmail: "",
		Provider:   "system",
		Action:     action,
		Resource:   resource,
		Result:     result,
		Metadata:   metadata,
	}
	if r != nil {
		if claims, _ := auth.ClaimsFromContext(r.Context()); claims != nil {
			entry.ActorSub = claims.Sub
			entry.ActorEmail = claims.Email
			entry.Provider = claims.Provider
		}
		if requestID, _ := r.Context().Value(ctxKeyRequestID).(string); strings.TrimSpace(requestID) != "" {
			entry.Metadata["request_id"] = requestID
		}
	}
	_ = a.auditLogger.Write(ctx, entry)
}

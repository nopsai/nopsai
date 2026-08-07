package nopsai

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
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

var terminalRunStatusesForCleanup = []string{"success", "warning", "failure", "cancelled", "timed_out", "failure (ignored)", "rejected", "skipped"}

var runBackupTables = []string{
	"pipeline_runs",
	"step_runs",
	"task_runs",
	"pipeline_run_checkpoints",
	"pipeline_approvals",
	"pipeline_run_logs",
	"pipeline_run_knowledge_contexts",
	"pipeline_run_outputs",
	"external_trigger_invocations",
	"git_webhook_deliveries",
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
	ID                    string           `json:"id"`
	Name                  string           `json:"name"`
	Description           string           `json:"description,omitempty"`
	Enabled               bool             `json:"enabled"`
	Target                string           `json:"target"`
	Mode                  string           `json:"mode"`
	KeepLast              int              `json:"keep_last,omitempty"`
	OlderThanDays         int              `json:"older_than_days,omitempty"`
	BackupBeforeCleanup   bool             `json:"backup_before_cleanup"`
	CronExpression        string           `json:"cron_expression"`
	Timezone              string           `json:"timezone"`
	NextRunAt             *time.Time       `json:"next_run_at,omitempty"`
	LastRunAt             *time.Time       `json:"last_run_at,omitempty"`
	LastJobID             string           `json:"last_job_id,omitempty"`
	LastStatus            string           `json:"last_status,omitempty"`
	LastDeletedCounts     map[string]int64 `json:"last_deleted_counts"`
	LastError             string           `json:"last_error,omitempty"`
	Source                string           `json:"source"`
	ConfigRepoID          *int64           `json:"config_repo_id,omitempty"`
	ConfigSourcePath      string           `json:"config_source_path,omitempty"`
	ConfigSourceCommitSHA string           `json:"config_source_commit_sha,omitempty"`
	ManagedByConfigRepo   bool             `json:"managed_by_config_repo"`
	CreatedBy             string           `json:"created_by,omitempty"`
	UpdatedBy             string           `json:"updated_by,omitempty"`
	CreatedAt             time.Time        `json:"created_at"`
	UpdatedAt             time.Time        `json:"updated_at"`
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

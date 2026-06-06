package nopsai

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	aaamodel "nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/pkg/routeauthz"
)

const (
	scheduleWorkerPollInterval = time.Minute
	scheduleWorkerBatchSize    = 10
	scheduleTriggerSource      = "schedule"
	scheduleServiceAccountPref = "schedule:"
	scheduleKindCron           = "cron"
	scheduleKindOnce           = "once"
)

type scheduleRequest struct {
	Path            string            `json:"path" yaml:"path"`
	Name            string            `json:"name" yaml:"name"`
	Description     string            `json:"description" yaml:"description"`
	Pipeline        string            `json:"pipeline" yaml:"pipeline"`
	PipelinePath    string            `json:"pipeline_path" yaml:"pipeline_path"`
	PipelineName    string            `json:"pipeline_name" yaml:"pipeline_name"`
	PipelineVersion string            `json:"pipeline_version" yaml:"pipeline_version"`
	ScheduleKind    string            `json:"schedule_kind" yaml:"schedule_kind"`
	Cron            string            `json:"cron" yaml:"cron"`
	CronExpression  string            `json:"cron_expression" yaml:"cron_expression"`
	RunAt           string            `json:"run_at" yaml:"run_at"`
	Timezone        string            `json:"timezone" yaml:"timezone"`
	Enabled         *bool             `json:"enabled" yaml:"enabled"`
	Scope           string            `json:"scope" yaml:"scope"`
	RunGroupPath    string            `json:"run_group_path" yaml:"run_group_path"`
	Variables       map[string]string `json:"variables" yaml:"variables"`
}

type scheduleInput struct {
	Path            string
	Name            string
	Description     string
	PipelinePath    string
	PipelineName    string
	PipelineVersion string
	ScheduleKind    string
	CronExpression  string
	RunAt           *time.Time
	Timezone        string
	Enabled         bool
	Scope           string
	RunGroupPath    string
	Variables       map[string]string
	NextRunAt       *time.Time
}

type storedSchedule struct {
	input      scheduleInput
	sourcePath string
}

type scheduleRecord struct {
	ID                    string
	Path                  string
	Name                  string
	Description           string
	PipelinePath          string
	PipelineName          string
	PipelineVersion       string
	ScheduleKind          string
	CronExpression        string
	RunAt                 *time.Time
	Timezone              string
	Enabled               bool
	Scope                 string
	RunGroupPath          string
	Variables             map[string]string
	NextRunAt             *time.Time
	LastRunAt             *time.Time
	LastRunID             string
	LastStatus            string
	Source                string
	Visibility            string
	ConfigRepoID          sql.NullInt64
	ConfigSourcePath      string
	ConfigSourceCommitSHA string
	ManagedByConfigRepo   bool
	CreatedBy             string
	UpdatedBy             string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	LatestRun             *scheduleRunSummary
}

type scheduleRunSummary struct {
	RunID      string     `json:"run_id"`
	Status     string     `json:"status"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Duration   string     `json:"duration,omitempty"`
}

type scheduleResponse struct {
	ID                    string              `json:"id"`
	Path                  string              `json:"path"`
	Name                  string              `json:"name"`
	Identifier            string              `json:"identifier"`
	Description           string              `json:"description,omitempty"`
	Pipeline              string              `json:"pipeline"`
	PipelinePath          string              `json:"pipeline_path"`
	PipelineName          string              `json:"pipeline_name"`
	PipelineVersion       string              `json:"pipeline_version"`
	ScheduleKind          string              `json:"schedule_kind"`
	Cron                  string              `json:"cron"`
	CronExpression        string              `json:"cron_expression,omitempty"`
	RunAt                 *time.Time          `json:"run_at,omitempty"`
	Timezone              string              `json:"timezone"`
	Enabled               bool                `json:"enabled"`
	Scope                 string              `json:"scope,omitempty"`
	RunGroupPath          string              `json:"run_group_path,omitempty"`
	Variables             map[string]string   `json:"variables,omitempty"`
	NextRunAt             *time.Time          `json:"next_run_at,omitempty"`
	LastRunAt             *time.Time          `json:"last_run_at,omitempty"`
	LastRunID             string              `json:"last_run_id,omitempty"`
	LastStatus            string              `json:"last_status,omitempty"`
	LatestRun             *scheduleRunSummary `json:"latest_run,omitempty"`
	Source                string              `json:"source"`
	Visibility            string              `json:"visibility"`
	ConfigSourcePath      string              `json:"config_source_path,omitempty"`
	ConfigSourceCommitSHA string              `json:"config_source_commit_sha,omitempty"`
	ManagedByConfigRepo   bool                `json:"managed_by_config_repo"`
	CreatedAt             time.Time           `json:"created_at"`
	UpdatedAt             time.Time           `json:"updated_at"`
}

func (r scheduleRecord) identifier() string {
	return configsync.BuildPipelineIdentifier(r.Path, r.Name)
}

func (r scheduleRecord) pipelineIdentifier() string {
	return configsync.BuildPipelineIdentifier(r.PipelinePath, r.PipelineName)
}

func (r scheduleRecord) resourceRef() aaamodel.ResourceRef {
	return aaamodel.ResourceRef{Type: grantResourceSchedule, ID: r.identifier()}
}

func scheduleServiceAccountID(scheduleID string) string {
	return scheduleServiceAccountPref + strings.TrimSpace(scheduleID)
}

func scheduleResponseFromRecord(record scheduleRecord) scheduleResponse {
	lastStatus := strings.TrimSpace(record.LastStatus)
	if record.LatestRun != nil && strings.TrimSpace(record.LatestRun.Status) != "" {
		lastStatus = record.LatestRun.Status
	}
	return scheduleResponse{
		ID:                    record.ID,
		Path:                  record.Path,
		Name:                  record.Name,
		Identifier:            record.identifier(),
		Description:           record.Description,
		Pipeline:              record.pipelineIdentifier(),
		PipelinePath:          record.PipelinePath,
		PipelineName:          record.PipelineName,
		PipelineVersion:       normalizePipelineVersion(record.PipelineVersion),
		ScheduleKind:          normalizeScheduleKindValue(record.ScheduleKind),
		Cron:                  record.CronExpression,
		CronExpression:        record.CronExpression,
		RunAt:                 record.RunAt,
		Timezone:              record.Timezone,
		Enabled:               record.Enabled,
		Scope:                 record.Scope,
		RunGroupPath:          record.RunGroupPath,
		Variables:             record.Variables,
		NextRunAt:             record.NextRunAt,
		LastRunAt:             record.LastRunAt,
		LastRunID:             record.LastRunID,
		LastStatus:            lastStatus,
		LatestRun:             record.LatestRun,
		Source:                record.Source,
		Visibility:            record.Visibility,
		ConfigSourcePath:      record.ConfigSourcePath,
		ConfigSourceCommitSHA: record.ConfigSourceCommitSHA,
		ManagedByConfigRepo:   record.ManagedByConfigRepo,
		CreatedAt:             record.CreatedAt,
		UpdatedAt:             record.UpdatedAt,
	}
}

func normalizeScheduleInput(req scheduleRequest) (scheduleInput, error) {
	name := sanitizeInput(strings.TrimSpace(req.Name))
	if name == "" {
		return scheduleInput{}, fmt.Errorf("name is required")
	}
	path, err := normalizeSchedulePath(req.Path)
	if err != nil {
		return scheduleInput{}, err
	}

	pipelineID := strings.TrimSpace(req.Pipeline)
	pipelinePath := strings.Trim(strings.TrimSpace(req.PipelinePath), "/")
	pipelineName := strings.TrimSpace(req.PipelineName)
	if pipelineID != "" {
		parsedPath, parsedName, _, err := configsync.SplitPipelineIdentifier(pipelineID)
		if err != nil {
			return scheduleInput{}, fmt.Errorf("invalid pipeline: %w", err)
		}
		pipelinePath = parsedPath
		pipelineName = parsedName
	}
	pipelineName = sanitizeInput(pipelineName)
	if pipelineName == "" {
		return scheduleInput{}, fmt.Errorf("pipeline is required")
	}
	if pipelinePath != "" {
		normalizedPath, err := normalizeSchedulePath(pipelinePath)
		if err != nil {
			return scheduleInput{}, fmt.Errorf("invalid pipeline path: %w", err)
		}
		pipelinePath = normalizedPath
	}
	if path == "" {
		path = pipelinePath
	}

	timezone := strings.TrimSpace(req.Timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return scheduleInput{}, fmt.Errorf("invalid timezone %q: %w", timezone, err)
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	cronExpression := strings.TrimSpace(firstNonEmptyString(req.CronExpression, req.Cron))
	scheduleKind, err := normalizeScheduleKind(req.ScheduleKind, req.RunAt)
	if err != nil {
		return scheduleInput{}, err
	}
	now := time.Now()
	var runAt *time.Time
	var nextRunAt *time.Time
	switch scheduleKind {
	case scheduleKindOnce:
		parsedRunAt, err := parseScheduleRunAt(req.RunAt, timezone)
		if err != nil {
			return scheduleInput{}, err
		}
		if enabled && !parsedRunAt.After(now) {
			return scheduleInput{}, fmt.Errorf("run_at must be in the future for enabled one-time schedules")
		}
		runAt = &parsedRunAt
		if parsedRunAt.After(now) {
			nextRunAt = &parsedRunAt
		}
		cronExpression = strings.TrimSpace(cronExpression)
	default:
		if cronExpression == "" {
			return scheduleInput{}, fmt.Errorf("cron_expression is required")
		}
		next, err := nextScheduleRunAt(cronExpression, timezone, now)
		if err != nil {
			return scheduleInput{}, err
		}
		nextRunAt = &next
	}
	scope := normalizeScheduleScope(req.Scope)
	runGroupPath, err := normalizeRunGroupPath(req.RunGroupPath)
	if err != nil {
		return scheduleInput{}, fmt.Errorf("invalid run_group_path: %w", err)
	}
	variables, err := normalizeScheduleVariables(req.Variables)
	if err != nil {
		return scheduleInput{}, err
	}
	version := normalizePipelineVersion(req.PipelineVersion)

	return scheduleInput{
		Path:            path,
		Name:            name,
		Description:     strings.TrimSpace(req.Description),
		PipelinePath:    pipelinePath,
		PipelineName:    pipelineName,
		PipelineVersion: version,
		ScheduleKind:    scheduleKind,
		CronExpression:  cronExpression,
		RunAt:           runAt,
		Timezone:        timezone,
		Enabled:         enabled,
		Scope:           scope,
		RunGroupPath:    runGroupPath,
		Variables:       variables,
		NextRunAt:       nextRunAt,
	}, nil
}

func parseGitOpsSchedules(files map[string]string, scheduleDir string, binding models.ConfigRepository, boundFolder string) (map[string]storedSchedule, error) {
	schedules := make(map[string]storedSchedule)
	for path, content := range files {
		normalized := filepath.ToSlash(path)
		rel, ok := configsync.RelativePath(normalized, scheduleDir)
		if !ok {
			continue
		}
		if rel == "" || strings.HasSuffix(rel, "/") || !isYAMLFile(rel) {
			continue
		}

		schedulePath, fileBase, _, err := configsync.SplitPipelineIdentifier(rel)
		if err != nil {
			return nil, fmt.Errorf("invalid schedule path '%s': %w", normalized, err)
		}
		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			targetID, err := configsync.NormalizePathForFolder(boundFolder, rel)
			if err != nil {
				return nil, fmt.Errorf("invalid group-scoped schedule path '%s': %w", normalized, err)
			}
			schedulePath, fileBase, _, err = configsync.SplitPipelineIdentifier(targetID)
			if err != nil {
				return nil, fmt.Errorf("invalid normalized schedule path '%s': %w", targetID, err)
			}
		}

		var req scheduleRequest
		if err := yaml.Unmarshal([]byte(content), &req); err != nil {
			return nil, fmt.Errorf("failed to parse schedule '%s': %w", normalized, err)
		}
		if strings.TrimSpace(req.Name) != "" && sanitizeInput(req.Name) != sanitizeInput(fileBase) {
			return nil, fmt.Errorf("schedule '%s' name '%s' must match file name '%s'", normalized, req.Name, fileBase)
		}
		req.Path = schedulePath
		req.Name = fileBase
		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			if err := normalizeScheduleRuntimeRefsForFolder(boundFolder, &req); err != nil {
				return nil, fmt.Errorf("invalid group-scoped schedule '%s': %w", normalized, err)
			}
		}
		input, err := normalizeScheduleInput(req)
		if err != nil {
			return nil, fmt.Errorf("invalid schedule '%s': %w", normalized, err)
		}
		key := configsync.BuildPipelineIdentifier(input.Path, input.Name)
		if _, exists := schedules[key]; exists {
			return nil, fmt.Errorf("duplicate schedule '%s' detected in config repository", key)
		}
		schedules[key] = storedSchedule{input: input, sourcePath: normalized}
	}
	return schedules, nil
}

func normalizeScheduleRuntimeRefsForFolder(boundFolder string, req *scheduleRequest) error {
	if req == nil {
		return nil
	}
	if strings.TrimSpace(req.Pipeline) != "" {
		pipeline, rootQualified, err := configsync.NormalizePipelineIdentifierReference(configsync.StripResourcePrefix(req.Pipeline))
		if err != nil {
			pipeline, err = configsync.NormalizePathForFolder(boundFolder, req.Pipeline)
			if err != nil {
				return err
			}
		} else if !rootQualified {
			pipeline, err = configsync.NormalizePathForFolder(boundFolder, req.Pipeline)
			if err != nil {
				return err
			}
		}
		req.Pipeline = pipeline
	} else if strings.TrimSpace(req.PipelinePath) != "" || strings.TrimSpace(req.PipelineName) != "" {
		pipelineID := configsync.BuildPipelineIdentifier(req.PipelinePath, req.PipelineName)
		pipeline, rootQualified, err := configsync.NormalizePipelineIdentifierReference(pipelineID)
		if err != nil {
			pipeline, err = configsync.NormalizePathForFolder(boundFolder, pipelineID)
			if err != nil {
				return err
			}
		} else if !rootQualified {
			pipeline, err = configsync.NormalizePathForFolder(boundFolder, pipelineID)
			if err != nil {
				return err
			}
		}
		pipelinePath, pipelineName, _, err := configsync.SplitPipelineIdentifier(pipeline)
		if err != nil {
			return err
		}
		req.PipelinePath = pipelinePath
		req.PipelineName = pipelineName
	}
	if scope := strings.Trim(strings.TrimSpace(req.Scope), "/"); scope != "" && !strings.EqualFold(scope, "default") {
		if _, rootOnly := stripRootPathPrefix(scope); rootOnly {
			req.Scope = ""
		} else {
			normalized, err := configsync.NormalizePathForFolder(boundFolder, scope)
			if err != nil {
				return err
			}
			req.Scope = normalized
		}
	}
	if groupPath := strings.Trim(strings.TrimSpace(req.RunGroupPath), "/"); groupPath != "" {
		if _, rootOnly := stripRootPathPrefix(groupPath); rootOnly {
			req.RunGroupPath = rootGrantID
			return nil
		}
		normalized, err := configsync.NormalizePathForFolder(boundFolder, groupPath)
		if err != nil {
			return err
		}
		req.RunGroupPath = normalized
	}
	return nil
}

func normalizeSchedulePath(raw string) (string, error) {
	path := strings.Trim(strings.TrimSpace(raw), "/")
	path, rootOnly := stripRootPathPrefix(path)
	if rootOnly {
		return "", nil
	}
	if path == "." {
		return "", nil
	}
	if path == "" {
		return "", nil
	}
	if strings.Contains(path, "..") {
		return "", fmt.Errorf("path contains invalid segments")
	}
	parts := strings.Split(path, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = sanitizeInput(part)
		if part == "" {
			return "", fmt.Errorf("path contains invalid segments")
		}
		cleaned = append(cleaned, part)
	}
	return strings.Join(cleaned, "/"), nil
}

func normalizeScheduleScope(raw string) string {
	scope := strings.Trim(strings.TrimSpace(raw), "/")
	scope, rootOnly := stripRootPathPrefix(scope)
	if rootOnly {
		return ""
	}
	if strings.EqualFold(scope, "default") {
		return ""
	}
	return scope
}

func normalizeRunGroupPath(raw string) (string, error) {
	path := strings.Trim(strings.TrimSpace(raw), "/")
	if path == "" {
		return rootGrantID, nil
	}
	path, rootOnly := stripRootPathPrefix(path)
	if rootOnly {
		return rootGrantID, nil
	}
	normalized, err := normalizeSchedulePath(path)
	if err != nil {
		return "", err
	}
	if normalized == "" {
		return rootGrantID, nil
	}
	return normalized, nil
}

func normalizeScheduleKind(raw, runAt string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		if strings.TrimSpace(runAt) != "" {
			return scheduleKindOnce, nil
		}
		return scheduleKindCron, nil
	}
	kind := normalizeScheduleKindValue(raw)
	switch kind {
	case scheduleKindCron:
		return scheduleKindCron, nil
	case scheduleKindOnce:
		return scheduleKindOnce, nil
	default:
		return "", fmt.Errorf("schedule_kind must be one of cron or once")
	}
}

func normalizeScheduleKindValue(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", scheduleKindCron, "recurring":
		return scheduleKindCron
	case scheduleKindOnce, "one_time", "one-time", "specific", "date":
		return scheduleKindOnce
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func parseScheduleRunAt(raw, timezone string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, fmt.Errorf("run_at is required for one-time schedules")
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC(), nil
	}
	location, err := time.LoadLocation(firstNonEmptyString(timezone, "UTC"))
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timezone %q: %w", timezone, err)
	}
	for _, layout := range []string{
		"2006-01-02T15:04",
		"2006-01-02 15:04",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("run_at must be RFC3339 or YYYY-MM-DDTHH:MM")
}

func normalizeScheduleVariables(raw map[string]string) (map[string]string, error) {
	if len(raw) == 0 {
		return map[string]string{}, nil
	}
	var invalid []string
	out := make(map[string]string, len(raw))
	for key, value := range raw {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if !envKeyPattern.MatchString(key) {
			invalid = append(invalid, key)
			continue
		}
		out[key] = value
	}
	if len(invalid) > 0 {
		return nil, fmt.Errorf("invalid variable override name(s): %s", strings.Join(invalid, ", "))
	}
	return out, nil
}

func scheduleSpec(cronExpression, timezone string) (cron.Schedule, error) {
	cronExpression = strings.TrimSpace(cronExpression)
	if cronExpression == "" {
		return nil, fmt.Errorf("cron_expression is required")
	}
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return nil, fmt.Errorf("invalid timezone %q: %w", timezone, err)
	}
	lower := strings.ToLower(cronExpression)
	if !strings.HasPrefix(lower, "cron_tz=") && !strings.HasPrefix(lower, "tz=") {
		cronExpression = "CRON_TZ=" + timezone + " " + cronExpression
	}
	spec, err := cron.ParseStandard(cronExpression)
	if err != nil {
		return nil, fmt.Errorf("invalid cron_expression: %w", err)
	}
	return spec, nil
}

func nextScheduleRunAt(cronExpression, timezone string, from time.Time) (time.Time, error) {
	spec, err := scheduleSpec(cronExpression, timezone)
	if err != nil {
		return time.Time{}, err
	}
	next := spec.Next(from)
	if next.IsZero() {
		return time.Time{}, fmt.Errorf("cron_expression does not produce a future run time")
	}
	return next.UTC(), nil
}

func (a *App) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	pipelineFilter := strings.TrimSpace(r.URL.Query().Get("pipeline"))
	pathFilter, nameFilter, _, filterErr := configsync.SplitPipelineIdentifier(pipelineFilter)
	if pipelineFilter != "" && filterErr != nil {
		http.Error(w, filterErr.Error(), http.StatusBadRequest)
		return
	}

	records, err := a.listScheduleRecords(r.Context(), pathFilter, nameFilter)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list schedules")
		http.Error(w, "Failed to retrieve schedules", http.StatusInternalServerError)
		return
	}
	resources := make([]aaamodel.ResourceRef, 0, len(records))
	for _, record := range records {
		resources = append(resources, record.resourceRef())
	}
	allowedSet, err := a.allowedResourceSet(r, "pipeline_schedule.list", resources)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	responses := make([]scheduleResponse, 0, len(records))
	for _, record := range records {
		if _, ok := allowedSet[resourceKey(record.resourceRef())]; !ok {
			continue
		}
		responses = append(responses, scheduleResponseFromRecord(record))
	}
	writeJSON(w, http.StatusOK, responses)
}

func (a *App) handleGetSchedule(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireScheduleDecision(w, r, "pipeline_schedule.read")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, scheduleResponseFromRecord(record))
}

func (a *App) handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	var req scheduleRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid schedule payload", http.StatusBadRequest)
		return
	}
	input, err := normalizeScheduleInput(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	folderID := input.Path
	if folderID == "" {
		folderID = generalGrantID
	}
	if !a.requireAAADecision(w, r, "pipeline_schedule.create", aaamodel.ResourceRef{Type: grantResourceFolder, ID: folderID}) {
		return
	}
	pipeline, _, err := a.validateScheduleRuntimeAccess(r.Context(), r, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	record, err := a.createSchedule(r.Context(), input, pipeline, actorIDFromRequest(r), "database", "", "")
	if err != nil {
		if isUniqueViolation(err) {
			http.Error(w, "schedule already exists", http.StatusConflict)
			return
		}
		log.Error().Err(err).Msg("Failed to create schedule")
		http.Error(w, "Failed to create schedule", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, scheduleResponseFromRecord(record))
}

func (a *App) handleUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	existing, ok := a.requireScheduleDecision(w, r, "pipeline_schedule.update")
	if !ok {
		return
	}
	if existing.ManagedByConfigRepo {
		http.Error(w, "GitOps-managed schedules must be changed in the config repository", http.StatusConflict)
		return
	}
	var req scheduleRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid schedule payload", http.StatusBadRequest)
		return
	}
	input, err := normalizeScheduleInput(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	pipeline, _, err := a.validateScheduleRuntimeAccess(r.Context(), r, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	record, err := a.updateSchedule(r.Context(), existing.ID, input, pipeline, actorIDFromRequest(r))
	if err != nil {
		if isUniqueViolation(err) {
			http.Error(w, "schedule already exists", http.StatusConflict)
			return
		}
		log.Error().Err(err).Str("schedule_id", existing.ID).Msg("Failed to update schedule")
		http.Error(w, "Failed to update schedule", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, scheduleResponseFromRecord(record))
}

func (a *App) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireScheduleDecision(w, r, "pipeline_schedule.delete")
	if !ok {
		return
	}
	if record.ManagedByConfigRepo {
		http.Error(w, "GitOps-managed schedules must be changed in the config repository", http.StatusConflict)
		return
	}
	if err := a.deleteSchedule(r.Context(), record.ID); err != nil {
		log.Error().Err(err).Str("schedule_id", record.ID).Msg("Failed to delete schedule")
		http.Error(w, "Failed to delete schedule", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleEnableSchedule(w http.ResponseWriter, r *http.Request) {
	a.handleSetScheduleEnabled(w, r, true)
}

func (a *App) handleDisableSchedule(w http.ResponseWriter, r *http.Request) {
	a.handleSetScheduleEnabled(w, r, false)
}

func (a *App) handleSetScheduleEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	record, ok := a.requireScheduleDecision(w, r, "pipeline_schedule.update")
	if !ok {
		return
	}
	if record.ManagedByConfigRepo {
		http.Error(w, "GitOps-managed schedules must be changed in the config repository", http.StatusConflict)
		return
	}
	nextRunAt := record.NextRunAt
	if enabled {
		switch normalizeScheduleKindValue(record.ScheduleKind) {
		case scheduleKindOnce:
			if record.RunAt == nil {
				http.Error(w, "run_at is required for one-time schedules", http.StatusBadRequest)
				return
			}
			if !record.RunAt.After(time.Now()) {
				http.Error(w, "run_at must be in the future before enabling this one-time schedule", http.StatusBadRequest)
				return
			}
			nextRunAt = record.RunAt
		default:
			next, err := nextScheduleRunAt(record.CronExpression, record.Timezone, time.Now())
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			nextRunAt = &next
		}
	}
	updated, err := a.setScheduleEnabled(r.Context(), record.ID, enabled, nextRunAt, actorIDFromRequest(r))
	if err != nil {
		log.Error().Err(err).Str("schedule_id", record.ID).Msg("Failed to update schedule enabled state")
		http.Error(w, "Failed to update schedule", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, scheduleResponseFromRecord(updated))
}

func (a *App) handleRunScheduleNow(w http.ResponseWriter, r *http.Request) {
	record, ok := a.requireScheduleDecision(w, r, "pipeline_schedule.execute")
	if !ok {
		return
	}
	runID, err := a.executeSchedule(r.Context(), record)
	if err != nil {
		log.Error().Err(err).Str("schedule_id", record.ID).Msg("Failed to execute schedule")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"run_id": runID})
}

func (a *App) requireScheduleDecision(w http.ResponseWriter, r *http.Request, action string) (scheduleRecord, bool) {
	record, err := a.getScheduleRecord(r.Context(), r.PathValue("scheduleID"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "schedule not found", http.StatusNotFound)
			return scheduleRecord{}, false
		}
		log.Error().Err(err).Msg("Failed to load schedule")
		http.Error(w, "Failed to load schedule", http.StatusInternalServerError)
		return scheduleRecord{}, false
	}
	if !a.requireAAADecision(w, r, action, record.resourceRef()) {
		return scheduleRecord{}, false
	}
	return record, true
}

func (a *App) validateScheduleRuntimeAccess(ctx context.Context, r *http.Request, input scheduleInput) (models.Pipeline, []byte, error) {
	if !a.requireAAADecision(noopResponseWriter{}, r, "pipeline.execute", routeauthz.PipelineResource(input.PipelinePath, input.PipelineName)) {
		return models.Pipeline{}, nil, fmt.Errorf("forbidden")
	}
	pipeline, definition, err := a.loadSchedulePipeline(ctx, input.PipelinePath, input.PipelineName)
	if err != nil {
		return models.Pipeline{}, nil, err
	}
	subject, ok := a.currentAAASubject(r)
	if !ok {
		return models.Pipeline{}, nil, fmt.Errorf("unauthorized")
	}
	callerID := firstNonEmptyString(subject.ID, subject.Sub, subject.Email)
	if _, err := a.authorizeRunResourceUses(ctx, subject.Type, callerID, scheduleTriggerSource, map[string]string{}, input.PipelinePath, scheduleTriggerSource, pipeline, input.Scope); err != nil {
		return models.Pipeline{}, nil, err
	}
	return pipeline, definition, nil
}

type noopResponseWriter struct{}

func (noopResponseWriter) Header() http.Header       { return http.Header{} }
func (noopResponseWriter) Write([]byte) (int, error) { return 0, nil }
func (noopResponseWriter) WriteHeader(int)           {}

func (a *App) loadSchedulePipeline(ctx context.Context, pipelinePath, pipelineName string) (models.Pipeline, []byte, error) {
	var definition string
	if err := a.db.QueryRow(ctx, `SELECT definition FROM pipelines WHERE path = $1 AND name = $2`, pipelinePath, pipelineName).Scan(&definition); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return models.Pipeline{}, nil, fmt.Errorf("pipeline not found")
		}
		return models.Pipeline{}, nil, fmt.Errorf("failed to load pipeline: %w", err)
	}
	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(definition), &pipeline); err != nil {
		return models.Pipeline{}, nil, fmt.Errorf("pipeline YAML is malformed: %w", err)
	}
	pipeline.Name = sanitizeInput(pipeline.Name)
	pipeline.Version = normalizePipelineVersion(pipeline.Version)
	if pipeline.Name == "" {
		pipeline.Name = pipelineName
	}
	return pipeline, []byte(definition), nil
}

func loadSchedulePipelineFromSync(ctx context.Context, runner queryRunner, input scheduleInput, pipelines map[string]storedPipeline) (models.Pipeline, error) {
	pipelineID := configsync.BuildPipelineIdentifier(input.PipelinePath, input.PipelineName)
	if stored, ok := pipelines[pipelineID]; ok {
		var pipeline models.Pipeline
		if err := yaml.Unmarshal([]byte(stored.definition), &pipeline); err != nil {
			return models.Pipeline{}, err
		}
		pipeline.Name = sanitizeInput(pipeline.Name)
		pipeline.Version = normalizePipelineVersion(pipeline.Version)
		if pipeline.Name == "" {
			pipeline.Name = input.PipelineName
		}
		return pipeline, nil
	}
	var definition string
	if err := runner.QueryRow(ctx, `SELECT definition FROM pipelines WHERE path = $1 AND name = $2`, input.PipelinePath, input.PipelineName).Scan(&definition); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return models.Pipeline{}, fmt.Errorf("pipeline %q not found", pipelineID)
		}
		return models.Pipeline{}, err
	}
	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(definition), &pipeline); err != nil {
		return models.Pipeline{}, err
	}
	pipeline.Name = sanitizeInput(pipeline.Name)
	pipeline.Version = normalizePipelineVersion(pipeline.Version)
	if pipeline.Name == "" {
		pipeline.Name = input.PipelineName
	}
	return pipeline, nil
}

func (a *App) createSchedule(ctx context.Context, input scheduleInput, pipeline models.Pipeline, actor, source, sourcePath, commitSHA string) (scheduleRecord, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return scheduleRecord{}, err
	}
	defer tx.Rollback(ctx)

	variablesJSON, err := json.Marshal(input.Variables)
	if err != nil {
		return scheduleRecord{}, err
	}
	var scheduleID string
	err = tx.QueryRow(ctx, `
		INSERT INTO pipeline_schedules (
			path, name, description, pipeline_path, pipeline_name, pipeline_version,
			schedule_kind, cron_expression, run_at, timezone, enabled, scope, variables, next_run_at,
			run_group_path, source, config_source_path, config_source_commit_sha, managed_by_config_repo,
			created_by, updated_by
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12, $13::jsonb, $14,
			$15, $16, $17, $18, $19,
			$20, $20
		)
		RETURNING id::text
	`, input.Path, input.Name, input.Description, input.PipelinePath, input.PipelineName, input.PipelineVersion,
		input.ScheduleKind, input.CronExpression, input.RunAt, input.Timezone, input.Enabled, input.Scope, string(variablesJSON), input.NextRunAt,
		input.RunGroupPath, source, sourcePath, commitSHA, strings.EqualFold(source, "git"), actor).Scan(&scheduleID)
	if err != nil {
		return scheduleRecord{}, err
	}
	if err := ensureScheduleExecutionACLs(ctx, tx, scheduleID, input, pipeline); err != nil {
		return scheduleRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return scheduleRecord{}, err
	}
	return a.getScheduleRecord(ctx, scheduleID)
}

func (a *App) updateSchedule(ctx context.Context, scheduleID string, input scheduleInput, pipeline models.Pipeline, actor string) (scheduleRecord, error) {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return scheduleRecord{}, err
	}
	defer tx.Rollback(ctx)

	variablesJSON, err := json.Marshal(input.Variables)
	if err != nil {
		return scheduleRecord{}, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE pipeline_schedules
		SET path = $2,
			name = $3,
			description = $4,
			pipeline_path = $5,
			pipeline_name = $6,
			pipeline_version = $7,
			schedule_kind = $8,
			cron_expression = $9,
			run_at = $10,
			timezone = $11,
			enabled = $12,
			scope = $13,
			variables = $14::jsonb,
			next_run_at = $15,
			run_group_path = $16,
			updated_by = $17,
			updated_at = NOW()
		WHERE id::text = $1
	`, scheduleID, input.Path, input.Name, input.Description, input.PipelinePath, input.PipelineName, input.PipelineVersion,
		input.ScheduleKind, input.CronExpression, input.RunAt, input.Timezone, input.Enabled, input.Scope, string(variablesJSON), input.NextRunAt, input.RunGroupPath, actor)
	if err != nil {
		return scheduleRecord{}, err
	}
	if tag.RowsAffected() == 0 {
		return scheduleRecord{}, pgx.ErrNoRows
	}
	if err := ensureScheduleExecutionACLs(ctx, tx, scheduleID, input, pipeline); err != nil {
		return scheduleRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return scheduleRecord{}, err
	}
	return a.getScheduleRecord(ctx, scheduleID)
}

func (a *App) setScheduleEnabled(ctx context.Context, scheduleID string, enabled bool, nextRunAt *time.Time, actor string) (scheduleRecord, error) {
	tag, err := a.db.Exec(ctx, `
		UPDATE pipeline_schedules
		SET enabled = $2,
			next_run_at = $3,
			updated_by = $4,
			updated_at = NOW()
		WHERE id::text = $1
	`, scheduleID, enabled, nextRunAt, actor)
	if err != nil {
		return scheduleRecord{}, err
	}
	if tag.RowsAffected() == 0 {
		return scheduleRecord{}, pgx.ErrNoRows
	}
	return a.getScheduleRecord(ctx, scheduleID)
}

func (a *App) deleteSchedule(ctx context.Context, scheduleID string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, _ = tx.Exec(ctx, `DELETE FROM resource_acl WHERE subject_type = $1 AND subject_id = $2`, aaamodel.SubjectTypeServiceAccount, scheduleServiceAccountID(scheduleID))
	tag, err := tx.Exec(ctx, `DELETE FROM pipeline_schedules WHERE id::text = $1`, scheduleID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return tx.Commit(ctx)
}

func ensureScheduleExecutionACLs(ctx context.Context, runner queryRunner, scheduleID string, input scheduleInput, pipeline models.Pipeline) error {
	serviceAccountID := scheduleServiceAccountID(scheduleID)
	if _, err := runner.Exec(ctx, `DELETE FROM resource_acl WHERE subject_type = $1 AND subject_id = $2`, aaamodel.SubjectTypeServiceAccount, serviceAccountID); err != nil {
		return err
	}
	type acl struct {
		resourceType string
		resourceID   string
		action       string
	}
	var grants []acl
	pipelineID := aaamodel.BuildPipelineID(input.PipelinePath, input.PipelineName)
	grants = append(grants,
		acl{resourceType: grantResourcePipeline, resourceID: pipelineID, action: "pipeline.execute"},
		acl{resourceType: grantResourcePipeline, resourceID: pipelineID, action: "pipeline.use"},
	)
	scopeID := strings.Trim(strings.TrimSpace(input.Scope), "/")
	if scopeID == "" {
		scopeID = "default"
	}
	normalizedScopeID, _, _ := normalizeScopeGrantResourceID(scopeID)
	grants = append(grants, acl{resourceType: grantResourceScope, resourceID: normalizedScopeID, action: "scope.use"})
	for _, stepID := range collectReferencedStepIdentifiers(&pipeline) {
		grants = append(grants, acl{resourceType: grantResourceStep, resourceID: stepID, action: "step.use"})
	}
	for _, childPipelineID := range collectReferencedPipelineIdentifiers(&pipeline) {
		grants = append(grants, acl{resourceType: grantResourcePipeline, resourceID: childPipelineID, action: "pipeline.use"})
	}

	seen := map[string]struct{}{}
	for _, grant := range grants {
		key := grant.resourceType + "\x00" + grant.resourceID + "\x00" + grant.action
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if _, err := runner.Exec(ctx, `
			INSERT INTO resource_acl (resource_type, resource_id, subject_type, subject_id, action, effect)
			VALUES ($1, $2, $3, $4, $5, 'allow')
			ON CONFLICT (resource_type, resource_id, subject_type, subject_id, action, effect)
			DO UPDATE SET access_grant_id = NULL
		`, grant.resourceType, grant.resourceID, aaamodel.SubjectTypeServiceAccount, serviceAccountID, grant.action); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) listScheduleRecords(ctx context.Context, pipelinePath, pipelineName string) ([]scheduleRecord, error) {
	query := baseScheduleSelect()
	args := []any{}
	if strings.TrimSpace(pipelineName) != "" {
		query += " WHERE s.pipeline_path = $1 AND s.pipeline_name = $2"
		args = append(args, pipelinePath, pipelineName)
	}
	query += " ORDER BY s.path ASC, s.name ASC"
	rows, err := a.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []scheduleRecord
	for rows.Next() {
		record, err := scanScheduleRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (a *App) getScheduleRecord(ctx context.Context, scheduleID string) (scheduleRecord, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if scheduleID == "" {
		return scheduleRecord{}, pgx.ErrNoRows
	}
	return scanScheduleRecord(a.db.QueryRow(ctx, baseScheduleSelect()+" WHERE s.id::text = $1 OR "+scheduleIdentifierSQL("s")+" = $1 LIMIT 1", scheduleID))
}

func baseScheduleSelect() string {
	return `
		SELECT
			s.id::text, s.path, s.name, s.description,
			s.pipeline_path, s.pipeline_name, s.pipeline_version,
			COALESCE(s.schedule_kind, 'cron'), s.cron_expression, s.run_at,
			s.timezone, s.enabled, s.scope, COALESCE(s.run_group_path, ''), s.variables::text,
			s.next_run_at, s.last_run_at, COALESCE(s.last_run_id::text, ''),
			COALESCE(s.last_status, ''), COALESCE(s.source, 'database'), COALESCE(s.visibility, 'group'),
			s.config_repo_id, COALESCE(s.config_source_path, ''), COALESCE(s.config_source_commit_sha, ''),
			s.managed_by_config_repo, COALESCE(s.created_by, ''), COALESCE(s.updated_by, ''),
			s.created_at, s.updated_at,
			COALESCE(r.run_id::text, ''), COALESCE(r.status, ''), r.started_at, r.finished_at
		FROM pipeline_schedules s
		LEFT JOIN pipeline_runs r ON r.run_id = s.last_run_id
	`
}

func scheduleIdentifierSQL(alias string) string {
	alias = strings.TrimSpace(alias)
	if alias != "" {
		alias += "."
	}
	return "CASE WHEN " + alias + "path = '' THEN " + alias + "name ELSE " + alias + "path || '/' || " + alias + "name END"
}

type scheduleScanner interface {
	Scan(dest ...any) error
}

func scanScheduleRecord(scanner scheduleScanner) (scheduleRecord, error) {
	var record scheduleRecord
	var variablesRaw string
	var runAt, nextRunAt, lastRunAt, latestStartedAt, latestFinishedAt sql.NullTime
	var latestRunID, latestStatus sql.NullString
	if err := scanner.Scan(
		&record.ID, &record.Path, &record.Name, &record.Description,
		&record.PipelinePath, &record.PipelineName, &record.PipelineVersion,
		&record.ScheduleKind, &record.CronExpression, &runAt,
		&record.Timezone, &record.Enabled, &record.Scope, &record.RunGroupPath, &variablesRaw,
		&nextRunAt, &lastRunAt, &record.LastRunID, &record.LastStatus, &record.Source, &record.Visibility,
		&record.ConfigRepoID, &record.ConfigSourcePath, &record.ConfigSourceCommitSHA,
		&record.ManagedByConfigRepo, &record.CreatedBy, &record.UpdatedBy, &record.CreatedAt, &record.UpdatedAt,
		&latestRunID, &latestStatus, &latestStartedAt, &latestFinishedAt,
	); err != nil {
		return record, err
	}
	record.ScheduleKind = normalizeScheduleKindValue(record.ScheduleKind)
	if strings.TrimSpace(variablesRaw) != "" {
		_ = json.Unmarshal([]byte(variablesRaw), &record.Variables)
	}
	if record.Variables == nil {
		record.Variables = map[string]string{}
	}
	if runAt.Valid {
		t := runAt.Time
		record.RunAt = &t
	}
	if nextRunAt.Valid {
		t := nextRunAt.Time
		record.NextRunAt = &t
	}
	if lastRunAt.Valid {
		t := lastRunAt.Time
		record.LastRunAt = &t
	}
	if latestRunID.Valid && strings.TrimSpace(latestRunID.String) != "" {
		summary := &scheduleRunSummary{
			RunID:  latestRunID.String,
			Status: latestStatus.String,
		}
		if latestStartedAt.Valid {
			t := latestStartedAt.Time
			summary.StartedAt = &t
		}
		if latestFinishedAt.Valid {
			t := latestFinishedAt.Time
			summary.FinishedAt = &t
		}
		if latestStartedAt.Valid {
			if latestFinishedAt.Valid {
				summary.Duration = latestFinishedAt.Time.Sub(latestStartedAt.Time).Round(time.Second).String()
			} else {
				summary.Duration = time.Since(latestStartedAt.Time).Round(time.Second).String()
			}
		}
		record.LatestRun = summary
	}
	return record, nil
}

func resolveScheduleGrantResource(ctx context.Context, runner queryRunner, rawID string, requireExists bool) (accessGrantResource, error) {
	rawID = strings.Trim(strings.TrimSpace(rawID), "/")
	if rawID == "" {
		return accessGrantResource{}, fmt.Errorf("resource_id is required")
	}
	if requireExists || looksLikeUUID(rawID) {
		var path, name string
		err := runner.QueryRow(ctx, `
			SELECT path, name
			FROM pipeline_schedules
			WHERE id::text = $1 OR `+scheduleIdentifierSQL("")+` = $1
			LIMIT 1
		`, rawID).Scan(&path, &name)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
				return accessGrantResource{}, fmt.Errorf("resource not found")
			}
			return accessGrantResource{}, err
		}
		identifier := configsync.BuildPipelineIdentifier(path, name)
		return accessGrantResource{Type: grantResourceSchedule, ID: identifier, Display: identifier}, nil
	}
	path, name, _, err := configsync.SplitPipelineIdentifier(rawID)
	if err != nil {
		return accessGrantResource{}, err
	}
	identifier := configsync.BuildPipelineIdentifier(path, name)
	return accessGrantResource{Type: grantResourceSchedule, ID: identifier, Display: identifier}, nil
}

func looksLikeUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}

func (a *App) resolveGroupIDForPath(ctx context.Context, groupPath string) (sql.NullInt32, error) {
	var out sql.NullInt32
	groupPath = strings.Trim(strings.TrimSpace(groupPath), "/")
	groupPath, rootOnly := stripRootPathPrefix(groupPath)
	if rootOnly || groupPath == "" {
		return out, nil
	}
	records, err := loadGroupPathRecords(ctx, a.db)
	if err != nil {
		return out, err
	}
	for _, record := range records {
		if record.Path == groupPath {
			out.Int32 = int32(record.ID)
			out.Valid = true
			return out, nil
		}
	}
	return out, nil
}

func (a *App) executeSchedule(ctx context.Context, record scheduleRecord) (string, error) {
	runGroupPath := effectiveScheduleRunGroupPath(record)
	payload := runRequestPayload{
		Pipeline:  record.pipelineIdentifier(),
		Scope:     record.Scope,
		Variables: record.Variables,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/run", bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Nopsai-Caller-Type", aaamodel.SubjectTypeServiceAccount)
	req.Header.Set("X-Nopsai-Caller-ID", scheduleServiceAccountID(record.ID))
	req.Header.Set("X-Nopsai-Trigger-Source", scheduleTriggerSource)
	req.Header.Set("X-Nopsai-Pipeline-Source", scheduleTriggerSource)
	req.Header.Set("X-Nopsai-Trigger-Event-ID", fmt.Sprintf("schedule:%s:%d", record.ID, time.Now().Unix()))
	if strings.TrimSpace(record.Scope) != "" {
		req.Header.Set("X-Nopsai-Scope", record.Scope)
	}
	if strings.TrimSpace(runGroupPath) != "" {
		req.Header.Set("X-Nopsai-Group-Path", runGroupPath)
	}
	req = req.WithContext(withAAASubject(req.Context(), aaamodel.Subject{
		Type: aaamodel.SubjectTypeServiceAccount,
		ID:   scheduleServiceAccountID(record.ID),
	}))

	recorder := httptest.NewRecorder()
	a.handleRunPipeline(recorder, req)
	if recorder.Code < 200 || recorder.Code >= 300 {
		message := strings.TrimSpace(recorder.Body.String())
		if message == "" {
			message = fmt.Sprintf("schedule execution failed with status %d", recorder.Code)
		}
		_, _ = a.db.Exec(ctx, `
			UPDATE pipeline_schedules
			SET last_run_at = NOW(),
				last_status = $2,
				updated_at = NOW()
			WHERE id::text = $1
		`, record.ID, "failure")
		return "", fmt.Errorf("%s", message)
	}

	var response struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || strings.TrimSpace(response.RunID) == "" {
		response.RunID = parseRunIDFromCreatedMessage(recorder.Body.String())
	}
	if strings.TrimSpace(response.RunID) == "" {
		return "", fmt.Errorf("schedule execution did not return a run id")
	}

	groupID, groupErr := a.resolveGroupIDForPath(ctx, runGroupPath)
	if groupErr != nil {
		log.Warn().Err(groupErr).Str("schedule_id", record.ID).Str("group_path", runGroupPath).Msg("Failed to resolve schedule run group")
	}
	if _, err := a.db.Exec(ctx, `
		UPDATE pipeline_runs
		SET schedule_id = $2,
			group_id = CASE WHEN $3::boolean THEN $4::integer ELSE group_id END
		WHERE run_id::text = $1
	`, response.RunID, record.ID, groupID.Valid, groupID); err != nil {
		return "", err
	}
	if _, err := a.db.Exec(ctx, `
		UPDATE pipeline_schedules
		SET last_run_id = $2,
			last_run_at = NOW(),
			last_status = 'pending',
			updated_at = NOW()
		WHERE id::text = $1
	`, record.ID, response.RunID); err != nil {
		return "", err
	}
	return response.RunID, nil
}

func effectiveScheduleRunGroupPath(record scheduleRecord) string {
	if groupPath := strings.Trim(strings.TrimSpace(record.RunGroupPath), "/"); groupPath != "" {
		return groupPath
	}
	return rootGrantID
}

func parseRunIDFromCreatedMessage(message string) string {
	message = strings.TrimSpace(message)
	const marker = "ID:"
	idx := strings.LastIndex(message, marker)
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(message[idx+len(marker):])
}

func (a *App) runScheduleWorker(ctx context.Context) {
	ticker := time.NewTicker(scheduleWorkerPollInterval)
	defer ticker.Stop()

	a.dispatchDueSchedules(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.dispatchDueSchedules(ctx)
		}
	}
}

func (a *App) dispatchDueSchedules(ctx context.Context) {
	if a == nil || a.db == nil {
		return
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to begin scheduled pipeline dispatch")
		return
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, baseScheduleSelect()+`
		WHERE s.enabled = TRUE
		  AND s.next_run_at IS NOT NULL
		  AND s.next_run_at <= NOW()
		ORDER BY s.next_run_at ASC
		LIMIT $1
		FOR UPDATE OF s SKIP LOCKED
	`, scheduleWorkerBatchSize)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to claim due schedules")
		return
	}
	var due []scheduleRecord
	for rows.Next() {
		record, err := scanScheduleRecord(rows)
		if err != nil {
			rows.Close()
			log.Warn().Err(err).Msg("Failed to scan due schedule")
			return
		}
		due = append(due, record)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		log.Warn().Err(err).Msg("Failed to read due schedules")
		return
	}
	rows.Close()

	now := time.Now()
	for _, record := range due {
		if normalizeScheduleKindValue(record.ScheduleKind) == scheduleKindOnce {
			if _, err := tx.Exec(ctx, `
				UPDATE pipeline_schedules
				SET enabled = FALSE,
					next_run_at = NULL,
					updated_at = NOW()
				WHERE id::text = $1
			`, record.ID); err != nil {
				log.Warn().Err(err).Str("schedule_id", record.ID).Msg("Failed to complete one-time schedule claim")
				return
			}
			continue
		}

		nextRunAt, err := nextScheduleRunAt(record.CronExpression, record.Timezone, now)
		if err != nil {
			log.Warn().Err(err).Str("schedule_id", record.ID).Msg("Failed to calculate next schedule time")
			_, _ = tx.Exec(ctx, `
				UPDATE pipeline_schedules
				SET next_run_at = NULL,
					last_status = 'failure',
					updated_at = NOW()
				WHERE id::text = $1
			`, record.ID)
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE pipeline_schedules
			SET next_run_at = $2,
				updated_at = NOW()
			WHERE id::text = $1
		`, record.ID, nextRunAt); err != nil {
			log.Warn().Err(err).Str("schedule_id", record.ID).Msg("Failed to advance schedule")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to commit due schedule claims")
		return
	}

	for _, record := range due {
		if _, err := a.executeSchedule(ctx, record); err != nil {
			log.Warn().Err(err).Str("schedule_id", record.ID).Msg("Scheduled pipeline execution failed")
		}
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

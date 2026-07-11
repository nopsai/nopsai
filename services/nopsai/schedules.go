package nopsai

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
	aaamodel "nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
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
	RunTeamPath     string            `json:"run_team_path" yaml:"run_team_path"`
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
	RunTeamPath     string
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
	RunTeamPath           string
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
	RunTeamPath           string              `json:"run_team_path,omitempty"`
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
		RunTeamPath:           record.RunTeamPath,
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
	runTeamPath, err := normalizeRunTeamPath(req.RunTeamPath)
	if err != nil {
		return scheduleInput{}, fmt.Errorf("invalid run_team_path: %w", err)
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
		RunTeamPath:     runTeamPath,
		Variables:       variables,
		NextRunAt:       nextRunAt,
	}, nil
}

func parseGitOpsSchedules(files map[string]string, scheduleDir string, binding models.ConfigRepository, boundTeam string) (map[string]storedSchedule, error) {
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
		if binding.ScopeType == models.ConfigRepositoryScopeTeam {
			targetID, err := configsync.NormalizePathForTeam(boundTeam, rel)
			if err != nil {
				return nil, fmt.Errorf("invalid team-scoped schedule path '%s': %w", normalized, err)
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
		if binding.ScopeType == models.ConfigRepositoryScopeTeam {
			if err := normalizeScheduleRuntimeRefsForTeam(boundTeam, &req); err != nil {
				return nil, fmt.Errorf("invalid team-scoped schedule '%s': %w", normalized, err)
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

func normalizeScheduleRuntimeRefsForTeam(boundTeam string, req *scheduleRequest) error {
	if req == nil {
		return nil
	}
	if strings.TrimSpace(req.Pipeline) != "" {
		pipeline, rootQualified, err := configsync.NormalizePipelineIdentifierReference(configsync.StripResourcePrefix(req.Pipeline))
		if err != nil {
			pipeline, err = configsync.NormalizePathForTeam(boundTeam, req.Pipeline)
			if err != nil {
				return err
			}
		} else if !rootQualified {
			pipeline, err = configsync.NormalizePathForTeam(boundTeam, req.Pipeline)
			if err != nil {
				return err
			}
		}
		req.Pipeline = pipeline
	} else if strings.TrimSpace(req.PipelinePath) != "" || strings.TrimSpace(req.PipelineName) != "" {
		pipelineID := configsync.BuildPipelineIdentifier(req.PipelinePath, req.PipelineName)
		pipeline, rootQualified, err := configsync.NormalizePipelineIdentifierReference(pipelineID)
		if err != nil {
			pipeline, err = configsync.NormalizePathForTeam(boundTeam, pipelineID)
			if err != nil {
				return err
			}
		} else if !rootQualified {
			pipeline, err = configsync.NormalizePathForTeam(boundTeam, pipelineID)
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
			normalized, err := configsync.NormalizePathForTeam(boundTeam, scope)
			if err != nil {
				return err
			}
			req.Scope = normalized
		}
	}
	if teamPath := strings.Trim(strings.TrimSpace(req.RunTeamPath), "/"); teamPath != "" {
		if _, rootOnly := stripRootPathPrefix(teamPath); rootOnly {
			req.RunTeamPath = rootGrantID
			return nil
		}
		normalized, err := configsync.NormalizePathForTeam(boundTeam, teamPath)
		if err != nil {
			return err
		}
		req.RunTeamPath = normalized
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

func normalizeRunTeamPath(raw string) (string, error) {
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

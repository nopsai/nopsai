package nopsai

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	aaamodel "nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
)

const (
	dashboardDefaultVisibility   = resourceVisibilityTeam
	dashboardPublishModeReplace  = "replace"
	dashboardPublishModeAppend   = "append"
	dashboardPublishModeSnapshot = "snapshot"
	dashboardPublishModeSeries   = "series"
)

const (
	dashboardRefreshModeStrict     = "strict"
	dashboardRefreshModeBestEffort = "best_effort"

	dashboardRefreshScopeDashboard = "dashboard"
	dashboardRefreshScopeSection   = "section"
	dashboardRefreshScopeSource    = "source"

	dashboardRefreshTriggerManual    = "manual"
	dashboardRefreshTriggerScheduled = "scheduled"
	dashboardRefreshTriggerAPI       = "api"
	dashboardRefreshTriggerAssistant = "assistant"

	dashboardRefreshStatusRunning   = "running"
	dashboardRefreshStatusComplete  = "complete"
	dashboardRefreshStatusPartial   = "partial"
	dashboardRefreshStatusFailed    = "failed"
	dashboardRefreshStatusCancelled = "cancelled"
	dashboardRefreshStatusTimedOut  = "timed_out"

	dashboardRefreshRunStatusQueued    = "queued"
	dashboardRefreshRunStatusRunning   = "running"
	dashboardRefreshRunStatusSuccess   = "success"
	dashboardRefreshRunStatusFailed    = "failed"
	dashboardRefreshRunStatusSkipped   = "skipped"
	dashboardRefreshRunStatusCancelled = "cancelled"
	dashboardRefreshRunStatusTimedOut  = "timed_out"

	dashboardRefreshDefaultConcurrency = 4
	dashboardRefreshDefaultTimeout     = 45 * time.Minute
	dashboardRefreshMaxConcurrency     = 16
	dashboardRefreshMaxSources         = 100
	dashboardRefreshMaxTimeout         = 12 * time.Hour
	dashboardPublicationMaxTTL         = 180 * 24 * time.Hour
	dashboardSeriesRetentionPoints     = 1000
)

var (
	dashboardSlugPattern       = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	dashboardSectionKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	dashboardEntryKeyPattern   = regexp.MustCompile(`^[a-zA-Z0-9_.:/-]+$`)
)

type dashboardRequest struct {
	TeamID        int                       `json:"team_id,omitempty"`
	TeamPath      string                    `json:"team_path,omitempty"`
	Slug          string                    `json:"slug"`
	Title         string                    `json:"title"`
	Description   string                    `json:"description,omitempty"`
	Visibility    string                    `json:"visibility,omitempty"`
	RefreshPolicy map[string]any            `json:"refresh_policy,omitempty"`
	Sections      []dashboardSectionRequest `json:"sections,omitempty"`
}

type dashboardInput struct {
	TeamID        int
	TeamPath      string
	Slug          string
	Title         string
	Description   string
	Visibility    string
	RefreshPolicy map[string]any
	Sections      []dashboardSectionInput
}

type dashboardSectionRequest struct {
	SectionKey   string         `json:"section_key"`
	Title        string         `json:"title"`
	Description  string         `json:"description,omitempty"`
	Layout       map[string]any `json:"layout,omitempty"`
	DisplayOrder int            `json:"display_order,omitempty"`
}

type dashboardSectionInput struct {
	SectionKey   string
	Title        string
	Description  string
	Layout       map[string]any
	DisplayOrder int
}

type dashboardSourceRequest struct {
	SectionKey         string `json:"section_key"`
	PipelineID         string `json:"pipeline_id"`
	OutputName         string `json:"output_name"`
	EntryKey           string `json:"entry_key,omitempty"`
	RunScope           string `json:"run_scope,omitempty"`
	Enabled            *bool  `json:"enabled,omitempty"`
	RequiredForRefresh *bool  `json:"required_for_refresh,omitempty"`
	RefreshOrder       int    `json:"refresh_order,omitempty"`
}

type dashboardSourceInput struct {
	SectionKey         string
	PipelineID         string
	OutputName         string
	EntryKey           string
	RunScope           string
	Enabled            bool
	RequiredForRefresh bool
	RefreshOrder       int
}

type dashboardRefreshScopeRequest struct {
	Type        string   `json:"type,omitempty" yaml:"type,omitempty"`
	SectionKey  string   `json:"section_key,omitempty" yaml:"section_key,omitempty"`
	SectionKeys []string `json:"section_keys,omitempty" yaml:"section_keys,omitempty"`
	SourceID    string   `json:"source_id,omitempty" yaml:"source_id,omitempty"`
	SourceIDs   []string `json:"source_ids,omitempty" yaml:"source_ids,omitempty"`
}

type dashboardRefreshRequest struct {
	Scope          dashboardRefreshScopeRequest `json:"scope,omitempty"`
	SectionKey     string                       `json:"section_key,omitempty"`
	SourceID       string                       `json:"source_id,omitempty"`
	SourceIDs      []string                     `json:"source_ids,omitempty"`
	TriggerType    string                       `json:"trigger_type,omitempty"`
	Mode           string                       `json:"mode,omitempty"`
	RunScope       string                       `json:"run_scope,omitempty"`
	Variables      map[string]string            `json:"variables,omitempty"`
	MaxConcurrency int                          `json:"max_concurrency,omitempty"`
	Timeout        string                       `json:"timeout,omitempty"`
	IdempotencyKey string                       `json:"idempotency_key,omitempty"`
}

type dashboardRefreshInput struct {
	ScopeType      string
	Scope          map[string]any
	SectionKeys    []string
	SourceIDs      []string
	TriggerType    string
	Mode           string
	RunScope       string
	Variables      map[string]string
	MaxConcurrency int
	Timeout        time.Duration
	IdempotencyKey string
}

type dashboardRefreshScheduleRequest struct {
	Name           string                       `json:"name"`
	Description    string                       `json:"description,omitempty"`
	Cron           string                       `json:"cron,omitempty"`
	CronExpression string                       `json:"cron_expression,omitempty"`
	Timezone       string                       `json:"timezone,omitempty"`
	Enabled        *bool                        `json:"enabled,omitempty"`
	Scope          dashboardRefreshScopeRequest `json:"scope,omitempty"`
	Mode           string                       `json:"mode,omitempty"`
	RunScope       string                       `json:"run_scope,omitempty"`
	Variables      map[string]string            `json:"variables,omitempty"`
	MaxConcurrency int                          `json:"max_concurrency,omitempty"`
	Timeout        string                       `json:"timeout,omitempty"`
}

type dashboardRefreshScheduleInput struct {
	Name           string
	Description    string
	CronExpression string
	Timezone       string
	Enabled        bool
	Refresh        dashboardRefreshInput
	NextRunAt      *time.Time
}

type dashboardRecord struct {
	ID                      string
	TeamID                  int
	TeamPath                string
	Slug                    string
	Title                   string
	Description             string
	Visibility              string
	RefreshPolicy           map[string]any
	CurrentPublicationCount int
	LastPublishedAt         *time.Time
	Source                  string
	ConfigRepoID            sql.NullInt64
	ConfigSourcePath        string
	ConfigSourceCommitSHA   string
	ManagedByConfigRepo     bool
	CreatedBy               string
	UpdatedBy               string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type dashboardSectionRecord struct {
	ID           string
	DashboardID  string
	SectionKey   string
	Title        string
	Description  string
	Layout       map[string]any
	DisplayOrder int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type dashboardSourceRecord struct {
	ID                 string
	DashboardID        string
	SectionKey         string
	PipelineID         string
	OutputName         string
	EntryKey           string
	RunScope           string
	Enabled            bool
	RequiredForRefresh bool
	RefreshOrder       int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type dashboardPublicationRecord struct {
	ID               string
	DashboardID      string
	SectionKey       string
	EntryKey         string
	Mode             string
	Content          json.RawMessage
	Revision         int
	RunID            string
	RunOutputID      string
	PipelineID       string
	OutputName       string
	RunScope         string
	RefreshID        string
	SourceFinishedAt *time.Time
	PublishedAt      time.Time
	ExpiresAt        *time.Time
	Status           string
	Stale            bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type dashboardEventRecord struct {
	ID            string
	DashboardID   string
	SectionKey    string
	EntryKey      string
	PublicationID string
	Revision      int
	EventType     string
	Content       json.RawMessage
	RunID         string
	RefreshID     string
	CreatedAt     time.Time
}

type dashboardRefreshRecord struct {
	ID                string
	DashboardID       string
	DashboardRef      string
	RequestedByType   string
	RequestedByID     string
	TriggerType       string
	ScopeType         string
	Scope             map[string]any
	Mode              string
	Status            string
	TotalSources      int
	RequiredSources   int
	QueuedSources     int
	RunningSources    int
	SuccessfulSources int
	FailedSources     int
	SkippedSources    int
	MaxConcurrency    int
	TimeoutSeconds    int
	IdempotencyKey    string
	Error             string
	StartedAt         time.Time
	FinishedAt        *time.Time
	TimeoutAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type dashboardRefreshRunRecord struct {
	ID              string
	RefreshID       string
	DashboardID     string
	SourceBindingID string
	PipelineID      string
	OutputName      string
	SectionKey      string
	EntryKey        string
	RunScope        string
	RunID           string
	Required        bool
	Status          string
	Error           string
	StartedAt       *time.Time
	FinishedAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type dashboardRefreshScheduleRecord struct {
	ID                    string
	DashboardID           string
	DashboardRef          string
	Name                  string
	Description           string
	CronExpression        string
	Timezone              string
	Enabled               bool
	ScopeType             string
	Scope                 map[string]any
	Mode                  string
	RunScope              string
	Variables             map[string]string
	MaxConcurrency        int
	TimeoutSeconds        int
	NextRunAt             *time.Time
	LastRefreshID         string
	LastStatus            string
	ServiceAccountID      string
	Source                string
	ConfigRepoID          sql.NullInt64
	ConfigSourcePath      string
	ConfigSourceCommitSHA string
	ManagedByConfigRepo   bool
	CreatedBy             string
	UpdatedBy             string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type dashboardResponse struct {
	ID                      string         `json:"id"`
	TeamID                  int            `json:"team_id"`
	TeamPath                string         `json:"team_path"`
	Ref                     string         `json:"ref"`
	Slug                    string         `json:"slug"`
	Title                   string         `json:"title"`
	Description             string         `json:"description,omitempty"`
	Visibility              string         `json:"visibility"`
	RefreshPolicy           map[string]any `json:"refresh_policy,omitempty"`
	CurrentPublicationCount int            `json:"current_publication_count"`
	LastPublishedAt         *time.Time     `json:"last_published_at,omitempty"`
	Source                  string         `json:"source"`
	ConfigSourcePath        string         `json:"config_source_path,omitempty"`
	ConfigSourceCommitSHA   string         `json:"config_source_commit_sha,omitempty"`
	ManagedByConfigRepo     bool           `json:"managed_by_config_repo"`
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`
}

type dashboardViewResponse struct {
	Dashboard    dashboardResponse              `json:"dashboard"`
	Sections     []dashboardSectionResponse     `json:"sections"`
	Publications []dashboardPublicationResponse `json:"publications"`
	Sources      []dashboardSourceResponse      `json:"sources"`
}

type dashboardSectionResponse struct {
	ID           string         `json:"id"`
	SectionKey   string         `json:"section_key"`
	Title        string         `json:"title"`
	Description  string         `json:"description,omitempty"`
	Layout       map[string]any `json:"layout,omitempty"`
	DisplayOrder int            `json:"display_order"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type dashboardSourceResponse struct {
	ID                 string    `json:"id"`
	SectionKey         string    `json:"section_key"`
	PipelineID         string    `json:"pipeline_id"`
	OutputName         string    `json:"output_name"`
	EntryKey           string    `json:"entry_key,omitempty"`
	RunScope           string    `json:"run_scope,omitempty"`
	Enabled            bool      `json:"enabled"`
	RequiredForRefresh bool      `json:"required_for_refresh"`
	RefreshOrder       int       `json:"refresh_order"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type dashboardPublicationResponse struct {
	ID               string          `json:"id"`
	SectionKey       string          `json:"section_key"`
	EntryKey         string          `json:"entry_key"`
	Mode             string          `json:"mode"`
	Content          json.RawMessage `json:"content"`
	Revision         int             `json:"revision"`
	RunID            string          `json:"run_id,omitempty"`
	RunOutputID      string          `json:"run_output_id,omitempty"`
	PipelineID       string          `json:"pipeline_id,omitempty"`
	OutputName       string          `json:"output_name,omitempty"`
	RunScope         string          `json:"run_scope,omitempty"`
	RefreshID        string          `json:"refresh_id,omitempty"`
	SourceFinishedAt *time.Time      `json:"source_finished_at,omitempty"`
	PublishedAt      time.Time       `json:"published_at"`
	ExpiresAt        *time.Time      `json:"expires_at,omitempty"`
	Status           string          `json:"status"`
	Stale            bool            `json:"stale"`
}

type dashboardEventResponse struct {
	ID            string          `json:"id"`
	SectionKey    string          `json:"section_key"`
	EntryKey      string          `json:"entry_key"`
	PublicationID string          `json:"publication_id,omitempty"`
	Revision      int             `json:"revision"`
	EventType     string          `json:"event_type"`
	Content       json.RawMessage `json:"content"`
	RunID         string          `json:"run_id,omitempty"`
	RefreshID     string          `json:"refresh_id,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

type dashboardRefreshResponse struct {
	ID                string                        `json:"id"`
	DashboardID       string                        `json:"dashboard_id"`
	DashboardRef      string                        `json:"dashboard_ref"`
	RequestedByType   string                        `json:"requested_by_type,omitempty"`
	RequestedByID     string                        `json:"requested_by_id,omitempty"`
	TriggerType       string                        `json:"trigger_type"`
	ScopeType         string                        `json:"scope_type"`
	Scope             map[string]any                `json:"scope,omitempty"`
	Mode              string                        `json:"mode"`
	Status            string                        `json:"status"`
	TotalSources      int                           `json:"total_sources"`
	RequiredSources   int                           `json:"required_sources"`
	QueuedSources     int                           `json:"queued_sources"`
	RunningSources    int                           `json:"running_sources"`
	SuccessfulSources int                           `json:"successful_sources"`
	FailedSources     int                           `json:"failed_sources"`
	SkippedSources    int                           `json:"skipped_sources"`
	MaxConcurrency    int                           `json:"max_concurrency"`
	TimeoutSeconds    int                           `json:"timeout_seconds"`
	IdempotencyKey    string                        `json:"idempotency_key,omitempty"`
	Error             string                        `json:"error,omitempty"`
	StartedAt         time.Time                     `json:"started_at"`
	FinishedAt        *time.Time                    `json:"finished_at,omitempty"`
	TimeoutAt         *time.Time                    `json:"timeout_at,omitempty"`
	CreatedAt         time.Time                     `json:"created_at"`
	UpdatedAt         time.Time                     `json:"updated_at"`
	Sources           []dashboardRefreshRunResponse `json:"sources,omitempty"`
}

type dashboardRefreshRunResponse struct {
	ID              string     `json:"id"`
	RefreshID       string     `json:"refresh_id"`
	SourceBindingID string     `json:"source_binding_id,omitempty"`
	PipelineID      string     `json:"pipeline_id"`
	OutputName      string     `json:"output_name"`
	SectionKey      string     `json:"section_key"`
	EntryKey        string     `json:"entry_key,omitempty"`
	RunScope        string     `json:"run_scope,omitempty"`
	RunID           string     `json:"run_id,omitempty"`
	Required        bool       `json:"required"`
	Status          string     `json:"status"`
	Error           string     `json:"error,omitempty"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type dashboardRefreshScheduleResponse struct {
	ID                    string            `json:"id"`
	DashboardID           string            `json:"dashboard_id"`
	DashboardRef          string            `json:"dashboard_ref"`
	Name                  string            `json:"name"`
	Description           string            `json:"description,omitempty"`
	Cron                  string            `json:"cron"`
	CronExpression        string            `json:"cron_expression"`
	Timezone              string            `json:"timezone"`
	Enabled               bool              `json:"enabled"`
	ScopeType             string            `json:"scope_type"`
	Scope                 map[string]any    `json:"scope,omitempty"`
	Mode                  string            `json:"mode"`
	RunScope              string            `json:"run_scope,omitempty"`
	Variables             map[string]string `json:"variables,omitempty"`
	MaxConcurrency        int               `json:"max_concurrency"`
	TimeoutSeconds        int               `json:"timeout_seconds"`
	NextRunAt             *time.Time        `json:"next_run_at,omitempty"`
	LastRefreshID         string            `json:"last_refresh_id,omitempty"`
	LastStatus            string            `json:"last_status,omitempty"`
	ServiceAccountID      string            `json:"service_account_id"`
	Source                string            `json:"source"`
	ConfigSourcePath      string            `json:"config_source_path,omitempty"`
	ConfigSourceCommitSHA string            `json:"config_source_commit_sha,omitempty"`
	ManagedByConfigRepo   bool              `json:"managed_by_config_repo"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
}

func (r dashboardRecord) ref() string {
	return dashboardResourceID(r.TeamPath, r.Slug)
}

func (r dashboardRecord) resourceRef() aaamodel.ResourceRef {
	return aaamodel.ResourceRef{Type: grantResourceDashboard, ID: r.ref()}
}

func dashboardResourceID(teamPath, slug string) string {
	teamPath = strings.Trim(strings.TrimSpace(teamPath), "/")
	slug = strings.Trim(strings.TrimSpace(slug), "/")
	if teamPath == "" {
		return slug
	}
	return teamPath + "/" + slug
}

func splitDashboardRef(ref string) (teamPath, slug string, err error) {
	ref = strings.Trim(strings.TrimSpace(ref), "/")
	if ref == "" {
		return "", "", fmt.Errorf("dashboard ref is required")
	}
	parts := strings.Split(ref, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("dashboard ref must use team/dashboard format")
	}
	slug = strings.TrimSpace(parts[len(parts)-1])
	teamPath = strings.Trim(strings.Join(parts[:len(parts)-1], "/"), "/")
	if teamPath == "" || slug == "" {
		return "", "", fmt.Errorf("dashboard ref must use team/dashboard format")
	}
	return teamPath, slug, nil
}

func normalizeDashboardInput(req dashboardRequest, teamID int, teamPath string) (dashboardInput, error) {
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		return dashboardInput{}, fmt.Errorf("slug is required")
	}
	if !dashboardSlugPattern.MatchString(slug) {
		return dashboardInput{}, fmt.Errorf("slug can only contain alphanumeric characters, underscores, dots, and hyphens")
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return dashboardInput{}, fmt.Errorf("title is required")
	}
	visibility := normalizeResourceVisibility(req.Visibility)
	refreshPolicy := req.RefreshPolicy
	if refreshPolicy == nil {
		refreshPolicy = map[string]any{}
	}
	sections := make([]dashboardSectionInput, 0, len(req.Sections))
	for _, sectionReq := range req.Sections {
		section, err := normalizeDashboardSectionInput(sectionReq)
		if err != nil {
			return dashboardInput{}, err
		}
		sections = append(sections, section)
	}
	return dashboardInput{
		TeamID:        teamID,
		TeamPath:      strings.Trim(strings.TrimSpace(teamPath), "/"),
		Slug:          slug,
		Title:         title,
		Description:   strings.TrimSpace(req.Description),
		Visibility:    visibility,
		RefreshPolicy: refreshPolicy,
		Sections:      sections,
	}, nil
}

func normalizeDashboardSectionInput(req dashboardSectionRequest) (dashboardSectionInput, error) {
	key := strings.TrimSpace(req.SectionKey)
	if key == "" {
		return dashboardSectionInput{}, fmt.Errorf("section_key is required")
	}
	if !dashboardSectionKeyPattern.MatchString(key) {
		return dashboardSectionInput{}, fmt.Errorf("section_key can only contain alphanumeric characters, underscores, dots, and hyphens")
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = titleFromKey(key)
	}
	layout := req.Layout
	if layout == nil {
		layout = map[string]any{}
	}
	return dashboardSectionInput{
		SectionKey:   key,
		Title:        title,
		Description:  strings.TrimSpace(req.Description),
		Layout:       layout,
		DisplayOrder: req.DisplayOrder,
	}, nil
}

func normalizeDashboardSourceInput(req dashboardSourceRequest) (dashboardSourceInput, error) {
	section, err := normalizeDashboardSectionInput(dashboardSectionRequest{SectionKey: req.SectionKey})
	if err != nil {
		return dashboardSourceInput{}, err
	}
	pipelineID := strings.Trim(strings.TrimSpace(req.PipelineID), "/")
	if pipelineID == "" {
		return dashboardSourceInput{}, fmt.Errorf("pipeline_id is required")
	}
	outputName := strings.TrimSpace(req.OutputName)
	if outputName == "" {
		return dashboardSourceInput{}, fmt.Errorf("output_name is required")
	}
	entryKey := strings.TrimSpace(req.EntryKey)
	if entryKey == "" {
		entryKey = outputName
	}
	if entryKey != "" && !dashboardEntryKeyPattern.MatchString(entryKey) {
		return dashboardSourceInput{}, fmt.Errorf("entry_key can only contain alphanumeric characters, underscores, dots, colons, slashes, and hyphens")
	}
	runScope, err := normalizeDashboardRunScope(req.RunScope)
	if err != nil {
		return dashboardSourceInput{}, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	required := true
	if req.RequiredForRefresh != nil {
		required = *req.RequiredForRefresh
	}
	return dashboardSourceInput{
		SectionKey:         section.SectionKey,
		PipelineID:         pipelineID,
		OutputName:         outputName,
		EntryKey:           entryKey,
		RunScope:           runScope,
		Enabled:            enabled,
		RequiredForRefresh: required,
		RefreshOrder:       req.RefreshOrder,
	}, nil
}

func normalizeDashboardRunScope(raw string) (string, error) {
	scope := normalizeScheduleScope(raw)
	if scope == "" {
		return "", nil
	}
	if _, err := configsync.CleanPathSegments(scope, false); err != nil {
		return "", fmt.Errorf("run_scope is invalid: %w", err)
	}
	return scope, nil
}

func dashboardResponseFromRecord(record dashboardRecord) dashboardResponse {
	return dashboardResponse{
		ID:                      record.ID,
		TeamID:                  record.TeamID,
		TeamPath:                record.TeamPath,
		Ref:                     record.ref(),
		Slug:                    record.Slug,
		Title:                   record.Title,
		Description:             record.Description,
		Visibility:              normalizeResourceVisibility(record.Visibility),
		RefreshPolicy:           record.RefreshPolicy,
		CurrentPublicationCount: record.CurrentPublicationCount,
		LastPublishedAt:         record.LastPublishedAt,
		Source:                  record.Source,
		ConfigSourcePath:        record.ConfigSourcePath,
		ConfigSourceCommitSHA:   record.ConfigSourceCommitSHA,
		ManagedByConfigRepo:     record.ManagedByConfigRepo,
		CreatedAt:               record.CreatedAt,
		UpdatedAt:               record.UpdatedAt,
	}
}

func dashboardSectionResponseFromRecord(record dashboardSectionRecord) dashboardSectionResponse {
	return dashboardSectionResponse{
		ID:           record.ID,
		SectionKey:   record.SectionKey,
		Title:        record.Title,
		Description:  record.Description,
		Layout:       record.Layout,
		DisplayOrder: record.DisplayOrder,
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
	}
}

func dashboardSourceResponseFromRecord(record dashboardSourceRecord) dashboardSourceResponse {
	return dashboardSourceResponse{
		ID:                 record.ID,
		SectionKey:         record.SectionKey,
		PipelineID:         record.PipelineID,
		OutputName:         record.OutputName,
		EntryKey:           record.EntryKey,
		RunScope:           record.RunScope,
		Enabled:            record.Enabled,
		RequiredForRefresh: record.RequiredForRefresh,
		RefreshOrder:       record.RefreshOrder,
		CreatedAt:          record.CreatedAt,
		UpdatedAt:          record.UpdatedAt,
	}
}

func dashboardPublicationResponseFromRecord(record dashboardPublicationRecord) dashboardPublicationResponse {
	return dashboardPublicationResponse{
		ID:               record.ID,
		SectionKey:       record.SectionKey,
		EntryKey:         record.EntryKey,
		Mode:             record.Mode,
		Content:          record.Content,
		Revision:         record.Revision,
		RunID:            record.RunID,
		RunOutputID:      record.RunOutputID,
		PipelineID:       record.PipelineID,
		OutputName:       record.OutputName,
		RunScope:         record.RunScope,
		RefreshID:        record.RefreshID,
		SourceFinishedAt: record.SourceFinishedAt,
		PublishedAt:      record.PublishedAt,
		ExpiresAt:        record.ExpiresAt,
		Status:           record.Status,
		Stale:            record.Stale,
	}
}

func dashboardEventResponseFromRecord(record dashboardEventRecord) dashboardEventResponse {
	return dashboardEventResponse{
		ID:            record.ID,
		SectionKey:    record.SectionKey,
		EntryKey:      record.EntryKey,
		PublicationID: record.PublicationID,
		Revision:      record.Revision,
		EventType:     record.EventType,
		Content:       record.Content,
		RunID:         record.RunID,
		RefreshID:     record.RefreshID,
		CreatedAt:     record.CreatedAt,
	}
}

func dashboardRefreshResponseFromRecord(record dashboardRefreshRecord, runs []dashboardRefreshRunRecord) dashboardRefreshResponse {
	response := dashboardRefreshResponse{
		ID:                record.ID,
		DashboardID:       record.DashboardID,
		DashboardRef:      record.DashboardRef,
		RequestedByType:   record.RequestedByType,
		RequestedByID:     record.RequestedByID,
		TriggerType:       record.TriggerType,
		ScopeType:         record.ScopeType,
		Scope:             record.Scope,
		Mode:              record.Mode,
		Status:            record.Status,
		TotalSources:      record.TotalSources,
		RequiredSources:   record.RequiredSources,
		QueuedSources:     record.QueuedSources,
		RunningSources:    record.RunningSources,
		SuccessfulSources: record.SuccessfulSources,
		FailedSources:     record.FailedSources,
		SkippedSources:    record.SkippedSources,
		MaxConcurrency:    record.MaxConcurrency,
		TimeoutSeconds:    record.TimeoutSeconds,
		IdempotencyKey:    record.IdempotencyKey,
		Error:             record.Error,
		StartedAt:         record.StartedAt,
		FinishedAt:        record.FinishedAt,
		TimeoutAt:         record.TimeoutAt,
		CreatedAt:         record.CreatedAt,
		UpdatedAt:         record.UpdatedAt,
		Sources:           make([]dashboardRefreshRunResponse, 0, len(runs)),
	}
	for _, run := range runs {
		response.Sources = append(response.Sources, dashboardRefreshRunResponseFromRecord(run))
	}
	return response
}

func dashboardRefreshRunResponseFromRecord(record dashboardRefreshRunRecord) dashboardRefreshRunResponse {
	return dashboardRefreshRunResponse{
		ID:              record.ID,
		RefreshID:       record.RefreshID,
		SourceBindingID: record.SourceBindingID,
		PipelineID:      record.PipelineID,
		OutputName:      record.OutputName,
		SectionKey:      record.SectionKey,
		EntryKey:        record.EntryKey,
		RunScope:        record.RunScope,
		RunID:           record.RunID,
		Required:        record.Required,
		Status:          record.Status,
		Error:           record.Error,
		StartedAt:       record.StartedAt,
		FinishedAt:      record.FinishedAt,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
	}
}

func dashboardRefreshScheduleResponseFromRecord(record dashboardRefreshScheduleRecord) dashboardRefreshScheduleResponse {
	return dashboardRefreshScheduleResponse{
		ID:                    record.ID,
		DashboardID:           record.DashboardID,
		DashboardRef:          record.DashboardRef,
		Name:                  record.Name,
		Description:           record.Description,
		Cron:                  record.CronExpression,
		CronExpression:        record.CronExpression,
		Timezone:              record.Timezone,
		Enabled:               record.Enabled,
		ScopeType:             record.ScopeType,
		Scope:                 record.Scope,
		Mode:                  record.Mode,
		RunScope:              record.RunScope,
		Variables:             record.Variables,
		MaxConcurrency:        record.MaxConcurrency,
		TimeoutSeconds:        record.TimeoutSeconds,
		NextRunAt:             record.NextRunAt,
		LastRefreshID:         record.LastRefreshID,
		LastStatus:            record.LastStatus,
		ServiceAccountID:      record.ServiceAccountID,
		Source:                record.Source,
		ConfigSourcePath:      record.ConfigSourcePath,
		ConfigSourceCommitSHA: record.ConfigSourceCommitSHA,
		ManagedByConfigRepo:   record.ManagedByConfigRepo,
		CreatedAt:             record.CreatedAt,
		UpdatedAt:             record.UpdatedAt,
	}
}

func normalizeDashboardRefreshRequest(req dashboardRefreshRequest, policy map[string]any) (dashboardRefreshInput, error) {
	scopeReq := req.Scope
	if strings.TrimSpace(scopeReq.SectionKey) == "" {
		scopeReq.SectionKey = req.SectionKey
	}
	if strings.TrimSpace(scopeReq.SourceID) == "" {
		scopeReq.SourceID = req.SourceID
	}
	if len(scopeReq.SourceIDs) == 0 {
		scopeReq.SourceIDs = req.SourceIDs
	}
	scopeType := strings.ToLower(strings.TrimSpace(scopeReq.Type))
	sectionKeys := normalizeDashboardRefreshSections(scopeReq)
	sourceIDs := normalizeDashboardRefreshSourceIDs(scopeReq)
	switch {
	case scopeType == "" && len(sourceIDs) > 0:
		scopeType = dashboardRefreshScopeSource
	case scopeType == "" && len(sectionKeys) > 0:
		scopeType = dashboardRefreshScopeSection
	case scopeType == "":
		scopeType = dashboardRefreshScopeDashboard
	}
	switch scopeType {
	case dashboardRefreshScopeDashboard:
		sectionKeys = nil
		sourceIDs = nil
	case dashboardRefreshScopeSection:
		if len(sectionKeys) == 0 {
			return dashboardRefreshInput{}, fmt.Errorf("section refresh requires section_key")
		}
	case dashboardRefreshScopeSource:
		if len(sourceIDs) == 0 {
			return dashboardRefreshInput{}, fmt.Errorf("source refresh requires source_id")
		}
	default:
		return dashboardRefreshInput{}, fmt.Errorf("refresh scope.type must be dashboard, section, or source")
	}
	for _, section := range sectionKeys {
		if !dashboardSectionKeyPattern.MatchString(section) {
			return dashboardRefreshInput{}, fmt.Errorf("refresh section_key is invalid")
		}
	}
	mode := normalizeDashboardRefreshMode(req.Mode)
	concurrency := req.MaxConcurrency
	if concurrency <= 0 {
		concurrency = intFromPolicy(policy, "max_concurrency", dashboardRefreshDefaultConcurrency)
	}
	if concurrency <= 0 {
		concurrency = dashboardRefreshDefaultConcurrency
	}
	if concurrency > dashboardRefreshMaxConcurrency {
		return dashboardRefreshInput{}, fmt.Errorf("refresh max_concurrency cannot exceed %d", dashboardRefreshMaxConcurrency)
	}
	timeout := dashboardRefreshDefaultTimeout
	if policyTimeout := stringFromPolicy(policy, "timeout"); policyTimeout != "" {
		parsed, err := time.ParseDuration(policyTimeout)
		if err != nil || parsed < 0 {
			return dashboardRefreshInput{}, fmt.Errorf("dashboard refresh_policy.timeout is invalid")
		}
		if parsed > dashboardRefreshMaxTimeout {
			return dashboardRefreshInput{}, fmt.Errorf("dashboard refresh_policy.timeout cannot exceed %s", dashboardRefreshMaxTimeout)
		}
		timeout = parsed
	}
	if strings.TrimSpace(req.Timeout) != "" {
		parsed, err := time.ParseDuration(strings.TrimSpace(req.Timeout))
		if err != nil || parsed < 0 {
			return dashboardRefreshInput{}, fmt.Errorf("refresh timeout is invalid")
		}
		if parsed > dashboardRefreshMaxTimeout {
			return dashboardRefreshInput{}, fmt.Errorf("refresh timeout cannot exceed %s", dashboardRefreshMaxTimeout)
		}
		timeout = parsed
	}
	scope := map[string]any{"type": scopeType}
	if len(sectionKeys) > 0 {
		scope["section_keys"] = sectionKeys
	}
	if len(sourceIDs) > 0 {
		scope["source_ids"] = sourceIDs
	}
	variables := make(map[string]string, len(req.Variables))
	for key, value := range req.Variables {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		variables[key] = value
	}
	runScope, err := normalizeDashboardRunScope(req.RunScope)
	if err != nil {
		return dashboardRefreshInput{}, err
	}
	return dashboardRefreshInput{
		ScopeType:      scopeType,
		Scope:          scope,
		SectionKeys:    sectionKeys,
		SourceIDs:      sourceIDs,
		TriggerType:    normalizeDashboardRefreshTrigger(req.TriggerType),
		Mode:           mode,
		RunScope:       runScope,
		Variables:      variables,
		MaxConcurrency: concurrency,
		Timeout:        timeout,
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
	}, nil
}

func normalizeDashboardRefreshScheduleInput(req dashboardRefreshScheduleRequest, policy map[string]any) (dashboardRefreshScheduleInput, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return dashboardRefreshScheduleInput{}, fmt.Errorf("name is required")
	}
	if !dashboardSlugPattern.MatchString(name) {
		return dashboardRefreshScheduleInput{}, fmt.Errorf("name can only contain alphanumeric characters, underscores, dots, and hyphens")
	}
	timezone := strings.TrimSpace(req.Timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return dashboardRefreshScheduleInput{}, fmt.Errorf("invalid timezone %q: %w", timezone, err)
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	refreshReq := dashboardRefreshRequest{
		Scope:          req.Scope,
		TriggerType:    dashboardRefreshTriggerScheduled,
		Mode:           req.Mode,
		RunScope:       req.RunScope,
		Variables:      req.Variables,
		MaxConcurrency: req.MaxConcurrency,
		Timeout:        req.Timeout,
	}
	refresh, err := normalizeDashboardRefreshRequest(refreshReq, policy)
	if err != nil {
		return dashboardRefreshScheduleInput{}, err
	}
	refresh.TriggerType = dashboardRefreshTriggerScheduled
	cronExpression := strings.TrimSpace(firstNonEmptyString(req.CronExpression, req.Cron))
	if cronExpression == "" {
		return dashboardRefreshScheduleInput{}, fmt.Errorf("cron_expression is required")
	}
	nextRunAt, err := nextScheduleRunAt(cronExpression, timezone, time.Now())
	if err != nil {
		return dashboardRefreshScheduleInput{}, err
	}
	var next *time.Time
	if enabled {
		next = &nextRunAt
	}
	return dashboardRefreshScheduleInput{
		Name:           name,
		Description:    strings.TrimSpace(req.Description),
		CronExpression: cronExpression,
		Timezone:       timezone,
		Enabled:        enabled,
		Refresh:        refresh,
		NextRunAt:      next,
	}, nil
}

func normalizeDashboardRefreshTrigger(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case dashboardRefreshTriggerScheduled:
		return dashboardRefreshTriggerScheduled
	case dashboardRefreshTriggerAPI:
		return dashboardRefreshTriggerAPI
	case dashboardRefreshTriggerAssistant:
		return dashboardRefreshTriggerAssistant
	default:
		return dashboardRefreshTriggerManual
	}
}

func normalizeDashboardRefreshMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case dashboardRefreshModeBestEffort, "best-effort", "best effort":
		return dashboardRefreshModeBestEffort
	default:
		return dashboardRefreshModeStrict
	}
}

func normalizeDashboardRefreshSections(scope dashboardRefreshScopeRequest) []string {
	values := make([]string, 0, len(scope.SectionKeys)+1)
	if strings.TrimSpace(scope.SectionKey) != "" {
		values = append(values, strings.TrimSpace(scope.SectionKey))
	}
	for _, value := range scope.SectionKeys {
		value = strings.TrimSpace(value)
		if value != "" {
			values = append(values, value)
		}
	}
	return dedupeStrings(values)
}

func normalizeDashboardRefreshSourceIDs(scope dashboardRefreshScopeRequest) []string {
	values := make([]string, 0, len(scope.SourceIDs)+1)
	if strings.TrimSpace(scope.SourceID) != "" {
		values = append(values, strings.TrimSpace(scope.SourceID))
	}
	for _, value := range scope.SourceIDs {
		value = strings.TrimSpace(value)
		if value != "" {
			values = append(values, value)
		}
	}
	return dedupeStrings(values)
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func intFromPolicy(policy map[string]any, key string, fallback int) int {
	if policy == nil {
		return fallback
	}
	switch value := policy[key].(type) {
	case int:
		if value > 0 {
			return value
		}
	case int64:
		if value > 0 {
			return int(value)
		}
	case float64:
		if value > 0 {
			return int(value)
		}
	case json.Number:
		parsed, err := value.Int64()
		if err == nil && parsed > 0 {
			return int(parsed)
		}
	}
	return fallback
}

func stringFromPolicy(policy map[string]any, key string) string {
	if policy == nil {
		return ""
	}
	if value, ok := policy[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func scanJSONMap(raw string) map[string]any {
	out := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		return map[string]any{}
	}
	return out
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func titleFromKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.ReplaceAll(key, "_", " ")
	key = strings.ReplaceAll(key, "-", " ")
	if key == "" {
		return "Section"
	}
	parts := strings.Fields(key)
	for idx, part := range parts {
		if len(part) == 0 {
			continue
		}
		parts[idx] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

package nopsai

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"nopsai/pkg/httpapi"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/routeauthz"
)

const (
	monitoringDefaultWindow = 30 * 24 * time.Hour
	monitoringMaxTableRows  = 50
)

type monitoringAnalyticsFilters struct {
	From                  time.Time
	To                    time.Time
	ComparePreviousPeriod bool
	GroupID               *int
	RootGroup             bool
	PipelinePath          string
	PipelineName          string
	Repo                  string
	RunID                 string
	Ref                   string
	CommitSHA             string
	TriggerSource         string
	Status                string
	RequestedByType       string
	RequestedByID         string
	EffectiveSubjectType  string
	EffectiveSubjectID    string
	ExternalTriggerID     string
	ScheduleID            string
	MinDurationSeconds    *float64
	MaxDurationSeconds    *float64
}

type monitoringWindowResponse struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type monitoringDurationStats struct {
	AverageSeconds float64 `json:"average_seconds"`
	MedianSeconds  float64 `json:"median_seconds"`
	P95Seconds     float64 `json:"p95_seconds"`
	P99Seconds     float64 `json:"p99_seconds"`
	MaxSeconds     float64 `json:"max_seconds"`
	TotalSeconds   float64 `json:"total_seconds"`
}

type monitoringRunRef struct {
	RunID           string  `json:"run_id"`
	PipelinePath    string  `json:"pipeline_path"`
	PipelineName    string  `json:"pipeline_name"`
	Status          string  `json:"status"`
	DurationSeconds float64 `json:"duration_seconds"`
}

type monitoringRunRow struct {
	RunID             string     `json:"run_id"`
	PipelinePath      string     `json:"pipeline_path"`
	PipelineName      string     `json:"pipeline_name"`
	Status            string     `json:"status"`
	GroupName         string     `json:"group_name,omitempty"`
	Repo              string     `json:"repo,omitempty"`
	Ref               string     `json:"ref,omitempty"`
	CommitSHA         string     `json:"commit_sha,omitempty"`
	TriggerSource     string     `json:"trigger_source,omitempty"`
	ExternalTriggerID string     `json:"external_trigger_id,omitempty"`
	ScheduleID        string     `json:"schedule_id,omitempty"`
	FailureReason     string     `json:"failure_reason,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	QueueSeconds      float64    `json:"queue_seconds"`
	DurationSeconds   float64    `json:"duration_seconds"`
	EndToEndSeconds   float64    `json:"end_to_end_seconds"`
}

type monitoringNamedCount struct {
	Key     string  `json:"key"`
	Label   string  `json:"label"`
	Count   int64   `json:"count"`
	Failed  int64   `json:"failed,omitempty"`
	Tokens  int64   `json:"tokens,omitempty"`
	Rate    float64 `json:"rate,omitempty"`
	Seconds float64 `json:"seconds,omitempty"`
	CostUSD float64 `json:"-"`
}

type monitoringTimeBucket struct {
	Key                    string  `json:"key"`
	Label                  string  `json:"label"`
	Runs                   int64   `json:"runs"`
	Failures               int64   `json:"failures"`
	AverageDurationSeconds float64 `json:"average_duration_seconds"`
	TotalDurationSeconds   float64 `json:"total_duration_seconds"`
}

type monitoringHeatmapCell struct {
	DayOfWeek int   `json:"day_of_week"`
	Hour      int   `json:"hour"`
	Runs      int64 `json:"runs"`
	Failures  int64 `json:"failures"`
}

type monitoringSummaryResponse struct {
	Window                       monitoringWindowResponse `json:"window"`
	TotalRuns                    int64                    `json:"total_runs"`
	SuccessfulRuns               int64                    `json:"successful_runs"`
	FailedRuns                   int64                    `json:"failed_runs"`
	CancelledRuns                int64                    `json:"cancelled_runs"`
	RunningRuns                  int64                    `json:"running_runs"`
	PendingRuns                  int64                    `json:"pending_runs"`
	WaitingApprovalRuns          int64                    `json:"waiting_approval_runs"`
	SkippedRuns                  int64                    `json:"skipped_runs"`
	SuccessRate                  float64                  `json:"success_rate"`
	FailureRate                  float64                  `json:"failure_rate"`
	AverageDurationSeconds       float64                  `json:"average_duration_seconds"`
	MedianDurationSeconds        float64                  `json:"median_duration_seconds"`
	P95DurationSeconds           float64                  `json:"p95_duration_seconds"`
	P99DurationSeconds           float64                  `json:"p99_duration_seconds"`
	LongestRun                   *monitoringRunRef        `json:"longest_run,omitempty"`
	TotalRuntimeSeconds          float64                  `json:"total_runtime_seconds"`
	TotalStepsExecuted           int64                    `json:"total_steps_executed"`
	TotalTasksExecuted           int64                    `json:"total_tasks_executed"`
	ActiveRunners                int                      `json:"active_runners"`
	QueuedJobs                   int32                    `json:"queued_jobs"`
	RunnerUtilization            float64                  `json:"runner_utilization"`
	ExternalTriggerInvocations   int64                    `json:"external_trigger_invocations"`
	NotificationFailures         int64                    `json:"notification_failures"`
	EstimatedAITokens            int64                    `json:"estimated_ai_tokens"`
	RunnerSummary                monitoringRunnerSummary  `json:"runner_summary"`
	DispatcherError              string                   `json:"dispatcher_error,omitempty"`
	ComparePreviousPeriodEnabled bool                     `json:"compare_previous_period_enabled"`
}

type monitoringRunAnalyticsResponse struct {
	Window             monitoringWindowResponse `json:"window"`
	RunsOverTime       []monitoringTimeBucket   `json:"runs_over_time"`
	StatusSplit        []monitoringNamedCount   `json:"status_split"`
	TriggerSourceSplit []monitoringNamedCount   `json:"trigger_source_split"`
	FailureReasons     []monitoringNamedCount   `json:"failure_reasons"`
	Duration           monitoringDurationStats  `json:"duration"`
	QueueTime          monitoringDurationStats  `json:"queue_time"`
	EndToEndTime       monitoringDurationStats  `json:"end_to_end_time"`
	LongestRuns        []monitoringRunRow       `json:"longest_runs"`
	RunHeatmap         []monitoringHeatmapCell  `json:"run_heatmap"`
	RerunCount         int64                    `json:"rerun_count"`
	TimeoutCount       int64                    `json:"timeout_count"`
	RecentRuns         []monitoringRunRow       `json:"recent_runs"`
}

type monitoringPerformanceRow struct {
	Key                    string  `json:"key"`
	PipelinePath           string  `json:"pipeline_path,omitempty"`
	PipelineName           string  `json:"pipeline_name,omitempty"`
	StepName               string  `json:"step_name,omitempty"`
	TaskName               string  `json:"task_name,omitempty"`
	TotalRuns              int64   `json:"total_runs"`
	SuccessfulRuns         int64   `json:"successful_runs"`
	FailedRuns             int64   `json:"failed_runs"`
	CancelledRuns          int64   `json:"cancelled_runs"`
	TimeoutRuns            int64   `json:"timeout_runs"`
	SuccessRate            float64 `json:"success_rate"`
	FailureRate            float64 `json:"failure_rate"`
	AverageDurationSeconds float64 `json:"average_duration_seconds"`
	MedianDurationSeconds  float64 `json:"median_duration_seconds"`
	P95DurationSeconds     float64 `json:"p95_duration_seconds"`
	P99DurationSeconds     float64 `json:"p99_duration_seconds"`
	MaxDurationSeconds     float64 `json:"max_duration_seconds"`
	TotalDurationSeconds   float64 `json:"total_duration_seconds"`
	AverageQueueSeconds    float64 `json:"average_queue_seconds,omitempty"`
}

type monitoringTriggerAnalyticsResponse struct {
	Window                   monitoringWindowResponse `json:"window"`
	TriggerSources           []monitoringNamedCount   `json:"trigger_sources"`
	TriggerSourceTrend       []monitoringTimeBucket   `json:"trigger_source_trend"`
	FailuresByTriggerSource  []monitoringNamedCount   `json:"failures_by_trigger_source"`
	DurationByTriggerSource  []monitoringNamedCount   `json:"duration_by_trigger_source"`
	TokenByTriggerSource     []monitoringNamedCount   `json:"token_by_trigger_source"`
	TriggerSourceReliability []monitoringNamedCount   `json:"trigger_source_reliability"`
}

type monitoringExternalTriggerLastFired struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Enabled    bool       `json:"enabled"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RateLimit  string     `json:"rate_limit,omitempty"`
}

type monitoringExternalTriggerAnalyticsResponse struct {
	Window                     monitoringWindowResponse             `json:"window"`
	TotalExternalTriggers      int64                                `json:"total_external_triggers"`
	EnabledExternalTriggers    int64                                `json:"enabled_external_triggers"`
	DisabledExternalTriggers   int64                                `json:"disabled_external_triggers"`
	InvocationCount            int64                                `json:"invocation_count"`
	SuccessfulInvocations      int64                                `json:"successful_invocations"`
	FailedInvocations          int64                                `json:"failed_invocations"`
	PendingInvocations         int64                                `json:"pending_invocations"`
	InvocationToRunRate        float64                              `json:"invocation_to_run_rate"`
	MostFiredTriggers          []monitoringNamedCount               `json:"most_fired_triggers"`
	TopCallers                 []monitoringNamedCount               `json:"top_callers"`
	ErrorReasons               []monitoringNamedCount               `json:"error_reasons"`
	IdempotencyConflicts       int64                                `json:"idempotency_conflicts"`
	LastFiredTriggers          []monitoringExternalTriggerLastFired `json:"last_fired_triggers"`
	RateLimitViolations        int64                                `json:"rate_limit_violations"`
	RateLimitViolationTriggers []monitoringNamedCount               `json:"rate_limit_violation_triggers"`
}

type monitoringAIUsageResponse struct {
	Window                monitoringWindowResponse `json:"window"`
	TotalPromptTokens     int64                    `json:"total_prompt_tokens"`
	TotalCompletionTokens int64                    `json:"total_completion_tokens"`
	TotalTokens           int64                    `json:"total_tokens"`
	ExactTokens           int64                    `json:"exact_tokens"`
	EstimatedTokens       int64                    `json:"estimated_tokens"`
	ExactTokenEvents      int64                    `json:"exact_token_events"`
	EstimatedTokenEvents  int64                    `json:"estimated_token_events"`
	ByPipeline            []monitoringNamedCount   `json:"by_pipeline"`
	ByStep                []monitoringNamedCount   `json:"by_step"`
	ByTask                []monitoringNamedCount   `json:"by_task"`
	ByFeature             []monitoringNamedCount   `json:"by_feature"`
	ByProfile             []monitoringNamedCount   `json:"by_profile"`
	ByModel               []monitoringNamedCount   `json:"by_model"`
	BySubject             []monitoringNamedCount   `json:"by_subject"`
	Trend                 []monitoringTimeBucket   `json:"trend"`
	TopTokenRuns          []monitoringNamedCount   `json:"top_token_runs"`
}

type monitoringReliabilityResponse struct {
	Window                    monitoringWindowResponse   `json:"window"`
	RecentFailures            []monitoringRunRow         `json:"recent_failures"`
	FailureReasons            []monitoringNamedCount     `json:"failure_reasons"`
	RepeatedFailurePipelines  []monitoringPerformanceRow `json:"repeated_failure_pipelines"`
	FlakyPipelines            []monitoringPerformanceRow `json:"flaky_pipelines"`
	StuckRuns                 []monitoringRunRow         `json:"stuck_runs"`
	ApprovalsWaitingTooLong   []monitoringNamedCount     `json:"approvals_waiting_too_long"`
	NotificationFailures      []monitoringNamedCount     `json:"notification_failures"`
	FailedExternalInvocations []monitoringNamedCount     `json:"failed_external_invocations"`
}

type monitoringEfficiencyResponse struct {
	Window                        monitoringWindowResponse   `json:"window"`
	TotalRuntimeSeconds           float64                    `json:"total_runtime_seconds"`
	TotalRunnerMinutes            float64                    `json:"total_runner_minutes"`
	TotalAITokens                 int64                      `json:"total_ai_tokens"`
	TokenByPipeline               []monitoringNamedCount     `json:"token_by_pipeline"`
	TokenByGroup                  []monitoringNamedCount     `json:"token_by_group"`
	TokenByStep                   []monitoringNamedCount     `json:"token_by_step"`
	TokenHeavyLowSuccessPipelines []monitoringPerformanceRow `json:"token_heavy_low_success_pipelines"`
	FrequentReruns                []monitoringPerformanceRow `json:"frequent_reruns"`
	HighQueueGroups               []monitoringNamedCount     `json:"high_queue_groups"`
	Recommendations               []string                   `json:"recommendations"`
}

type monitoringSecurityResponse struct {
	Window                  monitoringWindowResponse   `json:"window"`
	RunsByRequester         []monitoringNamedCount     `json:"runs_by_requester"`
	RunsByEffectiveSubject  []monitoringNamedCount     `json:"runs_by_effective_subject"`
	ExternalTriggerCallers  []monitoringNamedCount     `json:"external_trigger_callers"`
	ServiceAccountRuns      []monitoringNamedCount     `json:"service_account_runs"`
	HighRiskFailedPipelines []monitoringPerformanceRow `json:"high_risk_failed_pipelines"`
}

func (a *App) handleMonitoringSummary(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	filters, runIDs, ok := a.prepareMonitoringAnalytics(w, r)
	if !ok {
		return
	}
	resp, err := a.loadMonitoringSummary(r.Context(), filters, runIDs)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load monitoring summary")
		http.Error(w, "failed to load monitoring summary", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleMonitoringRunAnalytics(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	filters, runIDs, ok := a.prepareMonitoringAnalytics(w, r)
	if !ok {
		return
	}
	resp, err := a.loadMonitoringRunAnalytics(r.Context(), filters, runIDs)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load monitoring run analytics")
		http.Error(w, "failed to load monitoring run analytics", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleMonitoringPipelinePerformance(w http.ResponseWriter, r *http.Request) {
	a.handleMonitoringPerformance(w, r, "pipeline")
}

func (a *App) handleMonitoringStepPerformance(w http.ResponseWriter, r *http.Request) {
	a.handleMonitoringPerformance(w, r, "step")
}

func (a *App) handleMonitoringTaskPerformance(w http.ResponseWriter, r *http.Request) {
	a.handleMonitoringPerformance(w, r, "task")
}

func (a *App) handleMonitoringPerformance(w http.ResponseWriter, r *http.Request, kind string) {
	setNoStoreHeaders(w)
	filters, runIDs, ok := a.prepareMonitoringAnalytics(w, r)
	if !ok {
		return
	}
	rows, err := a.loadMonitoringPerformance(r.Context(), runIDs, kind)
	if err != nil {
		log.Error().Err(err).Str("kind", kind).Msg("Failed to load monitoring performance")
		http.Error(w, "failed to load monitoring performance", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, map[string]any{
		"window": windowResponse(filters),
		"items":  rows,
	})
}

func (a *App) handleMonitoringTriggerAnalytics(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	filters, runIDs, ok := a.prepareMonitoringAnalytics(w, r)
	if !ok {
		return
	}
	resp, err := a.loadMonitoringTriggerAnalytics(r.Context(), filters, runIDs)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load monitoring trigger analytics")
		http.Error(w, "failed to load monitoring trigger analytics", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleMonitoringExternalTriggerAnalytics(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	filters, runIDs, ok := a.prepareMonitoringAnalytics(w, r)
	if !ok {
		return
	}
	triggerIDs, err := a.visibleMonitoringExternalTriggerIDs(r, filters)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	resp, err := a.loadMonitoringExternalTriggerAnalytics(r.Context(), filters, runIDs, triggerIDs)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load monitoring external trigger analytics")
		http.Error(w, "failed to load monitoring external trigger analytics", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleMonitoringAIUsage(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	filters, runIDs, ok := a.prepareMonitoringAnalytics(w, r)
	if !ok {
		return
	}
	resp, err := a.loadMonitoringAIUsage(r.Context(), filters, runIDs)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load monitoring AI usage")
		http.Error(w, "failed to load monitoring AI usage", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleMonitoringReliability(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	filters, runIDs, ok := a.prepareMonitoringAnalytics(w, r)
	if !ok {
		return
	}
	resp, err := a.loadMonitoringReliability(r.Context(), filters, runIDs)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load monitoring reliability")
		http.Error(w, "failed to load monitoring reliability", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleMonitoringEfficiency(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	filters, runIDs, ok := a.prepareMonitoringAnalytics(w, r)
	if !ok {
		return
	}
	resp, err := a.loadMonitoringEfficiency(r.Context(), filters, runIDs)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load monitoring efficiency")
		http.Error(w, "failed to load monitoring efficiency", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleMonitoringSecurity(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	filters, runIDs, ok := a.prepareMonitoringAnalytics(w, r)
	if !ok {
		return
	}
	triggerIDs, err := a.visibleMonitoringExternalTriggerIDs(r, filters)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	resp, err := a.loadMonitoringSecurity(r.Context(), filters, runIDs, triggerIDs)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load monitoring security")
		http.Error(w, "failed to load monitoring security", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) prepareMonitoringAnalytics(w http.ResponseWriter, r *http.Request) (monitoringAnalyticsFilters, []string, bool) {
	filters, err := parseMonitoringAnalyticsFilters(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return filters, nil, false
	}
	runIDs, err := a.visibleMonitoringRunIDs(r, filters)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to filter monitoring analytics runs by authorization")
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return filters, nil, false
	}
	return filters, runIDs, true
}

func parseMonitoringAnalyticsFilters(r *http.Request) (monitoringAnalyticsFilters, error) {
	now := time.Now().UTC()
	filters := monitoringAnalyticsFilters{
		From: now.Add(-monitoringDefaultWindow),
		To:   now,
	}
	values := r.URL.Query()
	if raw := strings.TrimSpace(values.Get("from")); raw != "" {
		parsed, err := parseMonitoringTime(raw, false)
		if err != nil {
			return filters, fmt.Errorf("invalid from")
		}
		filters.From = parsed
	}
	if raw := strings.TrimSpace(values.Get("to")); raw != "" {
		parsed, err := parseMonitoringTime(raw, true)
		if err != nil {
			return filters, fmt.Errorf("invalid to")
		}
		filters.To = parsed
	}
	if !filters.To.After(filters.From) {
		return filters, fmt.Errorf("to must be after from")
	}

	if raw := strings.TrimSpace(values.Get("groupId")); raw != "" {
		if strings.EqualFold(raw, rootGrantID) {
			filters.RootGroup = true
		} else {
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				return filters, fmt.Errorf("invalid groupId")
			}
			filters.GroupID = &parsed
		}
	}

	filters.ComparePreviousPeriod = strings.EqualFold(strings.TrimSpace(values.Get("compare")), "previous_period")
	filters.PipelinePath = strings.TrimSpace(values.Get("pipelinePath"))
	filters.PipelineName = strings.TrimSpace(values.Get("pipelineName"))
	filters.Repo = strings.TrimSpace(values.Get("repo"))
	filters.RunID = strings.TrimSpace(firstMonitoringText(values.Get("runId"), values.Get("runID"), values.Get("run_id")))
	filters.Ref = strings.TrimSpace(firstMonitoringText(values.Get("ref"), values.Get("branch")))
	if rawBranch := strings.TrimSpace(values.Get("branch")); rawBranch != "" && !strings.HasPrefix(filters.Ref, "refs/") {
		filters.Ref = "refs/heads/" + rawBranch
	}
	filters.CommitSHA = strings.TrimSpace(firstMonitoringText(values.Get("commitSHA"), values.Get("commitSha"), values.Get("commit")))
	filters.TriggerSource = strings.TrimSpace(values.Get("triggerSource"))
	filters.Status = strings.TrimSpace(values.Get("status"))
	filters.RequestedByType = strings.TrimSpace(values.Get("requestedByType"))
	filters.RequestedByID = strings.TrimSpace(values.Get("requestedById"))
	filters.EffectiveSubjectType = strings.TrimSpace(values.Get("effectiveSubjectType"))
	filters.EffectiveSubjectID = strings.TrimSpace(values.Get("effectiveSubjectId"))
	filters.ExternalTriggerID = strings.TrimSpace(values.Get("externalTriggerId"))
	filters.ScheduleID = strings.TrimSpace(values.Get("scheduleId"))
	if raw := strings.TrimSpace(values.Get("minDurationSeconds")); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil || parsed < 0 {
			return filters, fmt.Errorf("invalid minDurationSeconds")
		}
		filters.MinDurationSeconds = &parsed
	}
	if raw := strings.TrimSpace(values.Get("maxDurationSeconds")); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil || parsed < 0 {
			return filters, fmt.Errorf("invalid maxDurationSeconds")
		}
		filters.MaxDurationSeconds = &parsed
	}
	return filters, nil
}

func parseMonitoringTime(raw string, endOfDay bool) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.UTC(), nil
	}
	if parsed, err := time.Parse("2006-01-02", raw); err == nil {
		if endOfDay {
			return parsed.Add(24*time.Hour - time.Nanosecond).UTC(), nil
		}
		return parsed.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid time")
}

func windowResponse(filters monitoringAnalyticsFilters) monitoringWindowResponse {
	return monitoringWindowResponse{
		From: filters.From.UTC().Format(time.RFC3339),
		To:   filters.To.UTC().Format(time.RFC3339),
	}
}

func (a *App) visibleMonitoringRunIDs(r *http.Request, filters monitoringAnalyticsFilters) ([]string, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("database unavailable")
	}
	query, args := buildMonitoringCandidateRunIDsQuery(filters)
	rows, err := a.db.Query(r.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candidates := []string{}
	resources := []model.ResourceRef{}
	for rows.Next() {
		var runID string
		if err := rows.Scan(&runID); err != nil {
			return nil, err
		}
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		candidates = append(candidates, runID)
		resources = append(resources, routeauthz.RunResource(runID))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return []string{}, nil
	}

	allowedSet, err := a.allowedResourceSet(r, "pipeline_run.list", resources)
	if err != nil {
		return nil, err
	}
	visible := make([]string, 0, len(candidates))
	for _, runID := range candidates {
		if _, ok := allowedSet[resourceKey(routeauthz.RunResource(runID))]; ok {
			visible = append(visible, runID)
		}
	}
	return visible, nil
}

func buildMonitoringCandidateRunIDsQuery(filters monitoringAnalyticsFilters) (string, []any) {
	args := []any{filters.From, filters.To}
	conditions := []string{"pr.created_at >= $1", "pr.created_at <= $2"}
	withClause := ""

	if filters.GroupID != nil {
		args = append(args, *filters.GroupID)
		withClause = fmt.Sprintf(`
			WITH RECURSIVE selected_groups AS (
				SELECT id FROM groups WHERE id = $%d
				UNION ALL
				SELECT g.id
				FROM groups g
				JOIN selected_groups sg ON g.parent_id = sg.id
			)
		`, len(args))
		conditions = append(conditions, "pr.group_id IN (SELECT id FROM selected_groups)")
	} else if filters.RootGroup {
		conditions = append(conditions, "pr.group_id IS NULL")
	}

	addTextCondition := func(column, value string) {
		if value == "" {
			return
		}
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf("LOWER(COALESCE(%s, '')) = LOWER($%d)", column, len(args)))
	}
	addTextCondition("pr.pipeline_path", filters.PipelinePath)
	addTextCondition("pr.pipeline_name", filters.PipelineName)
	addTextCondition("pr.run_id::text", filters.RunID)
	addTextCondition("pr.git_ref", filters.Ref)
	addTextCondition("pr.git_commit_sha", filters.CommitSHA)
	addTextCondition("pr.trigger_source", filters.TriggerSource)
	addTextCondition("pr.status", filters.Status)
	addTextCondition("pr.requested_by_type", filters.RequestedByType)
	addTextCondition("pr.requested_by_id", filters.RequestedByID)
	addTextCondition("pr.effective_subject_type", filters.EffectiveSubjectType)
	addTextCondition("pr.effective_subject_id", filters.EffectiveSubjectID)
	if filters.Repo != "" {
		args = append(args, filters.Repo)
		idx := len(args)
		conditions = append(conditions, fmt.Sprintf(`(
			LOWER(COALESCE(pr.git_repo_owner, '') || '/' || COALESCE(pr.git_repo_name, '')) = LOWER($%d)
			OR LOWER(COALESCE(pr.git_repo_name, '')) = LOWER($%d)
		)`, idx, idx))
	}
	if filters.ExternalTriggerID != "" {
		args = append(args, filters.ExternalTriggerID)
		conditions = append(conditions, fmt.Sprintf("LOWER(COALESCE(eti.trigger_id, '')) = LOWER($%d)", len(args)))
	}
	if filters.ScheduleID != "" {
		args = append(args, filters.ScheduleID)
		conditions = append(conditions, fmt.Sprintf("pr.schedule_id::text = $%d", len(args)))
	}
	if filters.MinDurationSeconds != nil {
		args = append(args, *filters.MinDurationSeconds)
		conditions = append(conditions, fmt.Sprintf("pr.started_at IS NOT NULL AND pr.finished_at IS NOT NULL AND EXTRACT(EPOCH FROM pr.finished_at - pr.started_at) >= $%d", len(args)))
	}
	if filters.MaxDurationSeconds != nil {
		args = append(args, *filters.MaxDurationSeconds)
		conditions = append(conditions, fmt.Sprintf("pr.started_at IS NOT NULL AND pr.finished_at IS NOT NULL AND EXTRACT(EPOCH FROM pr.finished_at - pr.started_at) <= $%d", len(args)))
	}

	return withClause + `
		SELECT DISTINCT pr.run_id::text
		FROM pipeline_runs pr
		LEFT JOIN external_trigger_invocations eti ON eti.id::text = pr.trigger_event_id OR eti.run_id = pr.run_id
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY pr.run_id::text`, args
}

func (a *App) visibleMonitoringExternalTriggerIDs(r *http.Request, filters monitoringAnalyticsFilters) ([]string, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("database unavailable")
	}
	args := []any{}
	where := ""
	if filters.ExternalTriggerID != "" {
		args = append(args, filters.ExternalTriggerID)
		where = " WHERE LOWER(id) = LOWER($1)"
	}
	rows, err := a.db.Query(r.Context(), `SELECT id FROM external_triggers`+where+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candidates := []string{}
	resources := []model.ResourceRef{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		candidates = append(candidates, id)
		resources = append(resources, routeauthz.ExternalTriggerResource(id))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return []string{}, nil
	}
	allowedSet, err := a.allowedResourceSet(r, "external_trigger.read", resources)
	if err != nil {
		return nil, err
	}
	visible := make([]string, 0, len(candidates))
	for _, id := range candidates {
		if _, ok := allowedSet[resourceKey(routeauthz.ExternalTriggerResource(id))]; ok {
			visible = append(visible, id)
		}
	}
	return visible, nil
}

func (a *App) loadMonitoringSummary(ctx context.Context, filters monitoringAnalyticsFilters, runIDs []string) (monitoringSummaryResponse, error) {
	resp := monitoringSummaryResponse{Window: windowResponse(filters), ComparePreviousPeriodEnabled: filters.ComparePreviousPeriod}
	if len(runIDs) > 0 {
		row := a.db.QueryRow(ctx, `
			WITH filtered AS (
				SELECT *
				FROM pipeline_runs
				WHERE run_id::text = ANY($1)
			),
			durations AS (
				SELECT EXTRACT(EPOCH FROM finished_at - started_at)::float8 AS duration_seconds
				FROM filtered
				WHERE started_at IS NOT NULL
				  AND finished_at IS NOT NULL
				  AND finished_at >= started_at
			)
			SELECT
				(SELECT COUNT(*) FROM filtered),
				(SELECT COUNT(*) FROM filtered WHERE LOWER(status) = 'success'),
				(SELECT COUNT(*) FROM filtered WHERE LOWER(status) IN ('failure', 'failed')),
				(SELECT COUNT(*) FROM filtered WHERE LOWER(status) = 'cancelled'),
				(SELECT COUNT(*) FROM filtered WHERE LOWER(status) = 'running'),
				(SELECT COUNT(*) FROM filtered WHERE LOWER(status) = 'pending'),
				(SELECT COUNT(*) FROM filtered WHERE LOWER(status) IN ('waiting_approval', 'waiting approval')),
				(SELECT COUNT(*) FROM filtered WHERE LOWER(status) = 'skipped'),
				COALESCE((SELECT AVG(duration_seconds) FROM durations), 0)::float8,
				COALESCE((SELECT PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_seconds) FROM durations), 0)::float8,
				COALESCE((SELECT PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_seconds) FROM durations), 0)::float8,
				COALESCE((SELECT PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY duration_seconds) FROM durations), 0)::float8,
				COALESCE((SELECT SUM(duration_seconds) FROM durations), 0)::float8,
				(SELECT COUNT(*) FROM step_runs sr WHERE sr.run_id::text = ANY($1)) +
				COALESCE((
					SELECT COUNT(*)
					FROM (
						SELECT DISTINCT tr.run_id, tr.step_name
						FROM task_runs tr
						WHERE tr.run_id::text = ANY($1)
						  AND NOT EXISTS (
						      SELECT 1
						      FROM step_runs sr
						      WHERE sr.run_id = tr.run_id
						  )
					) inferred_steps
				), 0),
				(SELECT COUNT(*) FROM task_runs tr WHERE tr.run_id::text = ANY($1)),
				(SELECT COUNT(*) FROM external_trigger_invocations eti WHERE eti.run_id::text = ANY($1)),
				(SELECT COUNT(*) FROM notification_deliveries nd WHERE nd.run_id::text = ANY($1) AND LOWER(nd.status) = 'failed'),
				COALESCE((SELECT SUM(total_tokens) FROM ai_usage_events au WHERE au.run_id::text = ANY($1)), 0)::bigint
			`, runIDs)
		if err := row.Scan(&resp.TotalRuns, &resp.SuccessfulRuns, &resp.FailedRuns, &resp.CancelledRuns, &resp.RunningRuns, &resp.PendingRuns,
			&resp.WaitingApprovalRuns, &resp.SkippedRuns, &resp.AverageDurationSeconds, &resp.MedianDurationSeconds, &resp.P95DurationSeconds,
			&resp.P99DurationSeconds, &resp.TotalRuntimeSeconds, &resp.TotalStepsExecuted, &resp.TotalTasksExecuted, &resp.ExternalTriggerInvocations,
			&resp.NotificationFailures, &resp.EstimatedAITokens); err != nil {
			return resp, err
		}
		longest, err := a.loadMonitoringLongestRun(ctx, runIDs)
		if err != nil {
			return resp, err
		}
		resp.LongestRun = longest
	}
	completed := resp.SuccessfulRuns + resp.FailedRuns + resp.CancelledRuns
	if completed > 0 {
		resp.SuccessRate = float64(resp.SuccessfulRuns) / float64(completed)
		resp.FailureRate = float64(resp.FailedRuns) / float64(completed)
	}

	status, dispatcherErr := a.fetchDispatcherStatus(ctx)
	if dispatcherErr != nil {
		resp.DispatcherError = "Failed to fetch dispatcher status"
	} else {
		a.sampleMonitoringRunnerSnapshots(ctx, status)
	}
	_, runnerSummary := monitoringRunnersFromDispatcherStatus(status, nil)
	resp.RunnerSummary = runnerSummary
	resp.ActiveRunners = runnerSummary.Online
	resp.QueuedJobs = runnerSummary.QueuedJobs
	if runnerSummary.Capacity > 0 {
		resp.RunnerUtilization = float64(runnerSummary.ActiveJobs) / float64(runnerSummary.Capacity)
	}
	return resp, nil
}

func (a *App) loadMonitoringLongestRun(ctx context.Context, runIDs []string) (*monitoringRunRef, error) {
	if len(runIDs) == 0 {
		return nil, nil
	}
	var item monitoringRunRef
	err := a.db.QueryRow(ctx, `
		SELECT run_id::text, COALESCE(pipeline_path, ''), COALESCE(pipeline_name, ''), COALESCE(status, ''),
		       EXTRACT(EPOCH FROM finished_at - started_at)::float8
		FROM pipeline_runs
		WHERE run_id::text = ANY($1)
		  AND started_at IS NOT NULL
		  AND finished_at IS NOT NULL
		  AND finished_at >= started_at
		ORDER BY EXTRACT(EPOCH FROM finished_at - started_at) DESC
		LIMIT 1
	`, runIDs).Scan(&item.RunID, &item.PipelinePath, &item.PipelineName, &item.Status, &item.DurationSeconds)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (a *App) loadMonitoringRunAnalytics(ctx context.Context, filters monitoringAnalyticsFilters, runIDs []string) (monitoringRunAnalyticsResponse, error) {
	resp := monitoringRunAnalyticsResponse{Window: windowResponse(filters)}
	if len(runIDs) == 0 {
		return resp, nil
	}
	var err error
	resp.RunsOverTime, err = a.loadMonitoringTimeBuckets(ctx, runIDs, "")
	if err != nil {
		return resp, err
	}
	resp.StatusSplit, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT COALESCE(NULLIF(LOWER(status), ''), 'unknown'), COALESCE(NULLIF(status, ''), 'Unknown'), COUNT(*), 0::bigint, 0::float8, 0::float8
		FROM pipeline_runs
		WHERE run_id::text = ANY($1)
		GROUP BY 1,2
		ORDER BY COUNT(*) DESC, 2
	`, runIDs)
	if err != nil {
		return resp, err
	}
	resp.TriggerSourceSplit, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT COALESCE(NULLIF(trigger_source, ''), 'unknown'), COALESCE(NULLIF(trigger_source, ''), 'Unknown'), COUNT(*),
		       COUNT(*) FILTER (WHERE LOWER(status) IN ('failure', 'failed')),
		       CASE WHEN COUNT(*) = 0 THEN 0 ELSE COUNT(*) FILTER (WHERE LOWER(status) IN ('failure', 'failed'))::float8 / COUNT(*)::float8 END,
		       0::float8
		FROM pipeline_runs
		WHERE run_id::text = ANY($1)
		GROUP BY 1,2
		ORDER BY COUNT(*) DESC, 2
	`, runIDs)
	if err != nil {
		return resp, err
	}
	resp.FailureReasons, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT LEFT(COALESCE(NULLIF(failure_reason, ''), 'unknown'), 180), LEFT(COALESCE(NULLIF(failure_reason, ''), 'Unknown'), 180),
		       COUNT(*), 0::bigint, 0::float8, 0::float8
		FROM pipeline_runs
		WHERE run_id::text = ANY($1)
		  AND LOWER(status) IN ('failure', 'failed')
		GROUP BY 1,2
		ORDER BY COUNT(*) DESC, 2
		LIMIT 20
	`, runIDs)
	if err != nil {
		return resp, err
	}
	resp.Duration, err = a.loadMonitoringDurationStats(ctx, runIDs, "finished_at - started_at", "started_at IS NOT NULL AND finished_at IS NOT NULL AND finished_at >= started_at")
	if err != nil {
		return resp, err
	}
	resp.QueueTime, err = a.loadMonitoringDurationStats(ctx, runIDs, "started_at - created_at", "started_at IS NOT NULL AND started_at >= created_at")
	if err != nil {
		return resp, err
	}
	resp.EndToEndTime, err = a.loadMonitoringDurationStats(ctx, runIDs, "finished_at - created_at", "finished_at IS NOT NULL AND finished_at >= created_at")
	if err != nil {
		return resp, err
	}
	resp.LongestRuns, err = a.loadMonitoringRunRows(ctx, runIDs, `
		AND pr.started_at IS NOT NULL AND pr.finished_at IS NOT NULL AND pr.finished_at >= pr.started_at
		ORDER BY EXTRACT(EPOCH FROM pr.finished_at - pr.started_at) DESC
		LIMIT 10`)
	if err != nil {
		return resp, err
	}
	resp.RunHeatmap, err = a.loadMonitoringHeatmap(ctx, runIDs)
	if err != nil {
		return resp, err
	}
	if err := a.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE parent_run_id IS NOT NULL),
			COUNT(*) FILTER (WHERE timeout_at IS NOT NULL AND (finished_at IS NULL OR finished_at >= timeout_at OR failure_reason ILIKE '%timeout%'))
		FROM pipeline_runs
		WHERE run_id::text = ANY($1)
	`, runIDs).Scan(&resp.RerunCount, &resp.TimeoutCount); err != nil {
		return resp, err
	}
	resp.RecentRuns, err = a.loadMonitoringRunRows(ctx, runIDs, `ORDER BY pr.created_at DESC LIMIT 50`)
	return resp, err
}

func (a *App) loadMonitoringPerformance(ctx context.Context, runIDs []string, kind string) ([]monitoringPerformanceRow, error) {
	if len(runIDs) == 0 {
		return []monitoringPerformanceRow{}, nil
	}
	query := ""
	switch kind {
	case "pipeline":
		query = `
			WITH rows AS (
				SELECT COALESCE(pr.pipeline_path, '') AS pipeline_path,
				       COALESCE(pr.pipeline_name, '') AS pipeline_name,
				       COALESCE(pr.status, '') AS status,
				       COALESCE(pr.failure_reason, '') AS failure_reason,
				       CASE WHEN pr.started_at IS NOT NULL AND pr.finished_at IS NOT NULL AND pr.finished_at >= pr.started_at
				            THEN EXTRACT(EPOCH FROM pr.finished_at - pr.started_at)::float8 END AS duration_seconds,
				       CASE WHEN pr.started_at IS NOT NULL AND pr.started_at >= pr.created_at
				            THEN EXTRACT(EPOCH FROM pr.started_at - pr.created_at)::float8 END AS queue_seconds
				FROM pipeline_runs pr
				WHERE pr.run_id::text = ANY($1)
			)
			SELECT pipeline_path || '/' || pipeline_name, pipeline_path, pipeline_name, '', '',
			       COUNT(*),
			       COUNT(*) FILTER (WHERE LOWER(status) = 'success'),
			       COUNT(*) FILTER (WHERE LOWER(status) IN ('failure', 'failed')),
			       COUNT(*) FILTER (WHERE LOWER(status) = 'cancelled'),
			       COUNT(*) FILTER (WHERE failure_reason ILIKE '%timeout%'),
			       COALESCE(AVG(duration_seconds), 0)::float8,
			       COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_seconds) FILTER (WHERE duration_seconds IS NOT NULL), 0)::float8,
			       COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_seconds) FILTER (WHERE duration_seconds IS NOT NULL), 0)::float8,
			       COALESCE(PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY duration_seconds) FILTER (WHERE duration_seconds IS NOT NULL), 0)::float8,
			       COALESCE(MAX(duration_seconds), 0)::float8,
			       COALESCE(SUM(duration_seconds), 0)::float8,
			       COALESCE(AVG(queue_seconds), 0)::float8
			FROM rows
			GROUP BY pipeline_path, pipeline_name
			ORDER BY COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_seconds) FILTER (WHERE duration_seconds IS NOT NULL), 0) DESC,
			         COUNT(*) DESC
			LIMIT 50`
	case "step":
		query = `
			WITH step_rows AS (
				SELECT sr.run_id,
				       COALESCE(sr.name, '') AS step_name,
				       COALESCE(pr.pipeline_path, '') AS pipeline_path,
				       COALESCE(pr.pipeline_name, '') AS pipeline_name,
				       COALESCE(sr.status, '') AS status,
				       CASE WHEN sr.started_at IS NOT NULL AND sr.finished_at IS NOT NULL AND sr.finished_at >= sr.started_at
				            THEN EXTRACT(EPOCH FROM sr.finished_at - sr.started_at)::float8 END AS duration_seconds
				FROM step_runs sr
				JOIN pipeline_runs pr ON pr.run_id = sr.run_id
				WHERE sr.run_id::text = ANY($1)
			),
			task_step_rows AS (
				SELECT tr.run_id,
				       COALESCE(tr.step_name, '') AS step_name,
				       COALESCE(pr.pipeline_path, '') AS pipeline_path,
				       COALESCE(pr.pipeline_name, '') AS pipeline_name,
				       CASE
				         WHEN COUNT(*) FILTER (WHERE LOWER(tr.status) IN ('failure', 'failed')) > 0 THEN 'failure'
				         WHEN COUNT(*) FILTER (WHERE LOWER(tr.status) = 'running') > 0 THEN 'running'
				         WHEN COUNT(*) FILTER (WHERE LOWER(tr.status) = 'pending') > 0 THEN 'pending'
				         WHEN COUNT(*) FILTER (WHERE LOWER(tr.status) = 'cancelled') > 0 THEN 'cancelled'
				         WHEN COUNT(*) FILTER (WHERE LOWER(tr.status) = 'success') = COUNT(*) THEN 'success'
				         ELSE COALESCE(MAX(tr.status), '')
				       END AS status,
				       CASE WHEN MIN(tr.started_at) IS NOT NULL AND MAX(tr.finished_at) IS NOT NULL AND MAX(tr.finished_at) >= MIN(tr.started_at)
				            THEN EXTRACT(EPOCH FROM MAX(tr.finished_at) - MIN(tr.started_at))::float8 END AS duration_seconds
				FROM task_runs tr
				JOIN pipeline_runs pr ON pr.run_id = tr.run_id
				WHERE tr.run_id::text = ANY($1)
				GROUP BY tr.run_id, tr.step_name, pr.pipeline_path, pr.pipeline_name
			),
			rows AS (
				SELECT * FROM step_rows
				UNION ALL
				SELECT * FROM task_step_rows
				WHERE NOT EXISTS (
					SELECT 1
					FROM step_rows
					WHERE step_rows.run_id = task_step_rows.run_id
				)
			)
			SELECT pipeline_path || '/' || pipeline_name || '/' || step_name, pipeline_path, pipeline_name, step_name, '',
			       COUNT(*),
			       COUNT(*) FILTER (WHERE LOWER(status) = 'success'),
			       COUNT(*) FILTER (WHERE LOWER(status) IN ('failure', 'failed')),
			       COUNT(*) FILTER (WHERE LOWER(status) = 'cancelled'),
			       0::bigint,
			       COALESCE(AVG(duration_seconds), 0)::float8,
			       COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_seconds) FILTER (WHERE duration_seconds IS NOT NULL), 0)::float8,
			       COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_seconds) FILTER (WHERE duration_seconds IS NOT NULL), 0)::float8,
			       COALESCE(PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY duration_seconds) FILTER (WHERE duration_seconds IS NOT NULL), 0)::float8,
			       COALESCE(MAX(duration_seconds), 0)::float8,
			       COALESCE(SUM(duration_seconds), 0)::float8,
			       0::float8
			FROM rows
			GROUP BY pipeline_path, pipeline_name, step_name
			ORDER BY COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_seconds) FILTER (WHERE duration_seconds IS NOT NULL), 0) DESC,
			         COUNT(*) DESC
			LIMIT 50`
	case "task":
		query = `
			WITH rows AS (
				SELECT COALESCE(tr.step_name, '') AS step_name,
				       COALESCE(tr.task_name, '') AS task_name,
				       COALESCE(pr.pipeline_path, '') AS pipeline_path,
				       COALESCE(pr.pipeline_name, '') AS pipeline_name,
				       COALESCE(tr.status, '') AS status,
				       CASE WHEN tr.started_at IS NOT NULL AND tr.finished_at IS NOT NULL AND tr.finished_at >= tr.started_at
				            THEN EXTRACT(EPOCH FROM tr.finished_at - tr.started_at)::float8 END AS duration_seconds
				FROM task_runs tr
				JOIN pipeline_runs pr ON pr.run_id = tr.run_id
				WHERE tr.run_id::text = ANY($1)
			)
			SELECT pipeline_path || '/' || pipeline_name || '/' || step_name || '/' || task_name, pipeline_path, pipeline_name, step_name, task_name,
			       COUNT(*),
			       COUNT(*) FILTER (WHERE LOWER(status) = 'success'),
			       COUNT(*) FILTER (WHERE LOWER(status) IN ('failure', 'failed')),
			       COUNT(*) FILTER (WHERE LOWER(status) = 'cancelled'),
			       0::bigint,
			       COALESCE(AVG(duration_seconds), 0)::float8,
			       COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_seconds) FILTER (WHERE duration_seconds IS NOT NULL), 0)::float8,
			       COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_seconds) FILTER (WHERE duration_seconds IS NOT NULL), 0)::float8,
			       COALESCE(PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY duration_seconds) FILTER (WHERE duration_seconds IS NOT NULL), 0)::float8,
			       COALESCE(MAX(duration_seconds), 0)::float8,
			       COALESCE(SUM(duration_seconds), 0)::float8,
			       0::float8
			FROM rows
			GROUP BY pipeline_path, pipeline_name, step_name, task_name
			ORDER BY COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_seconds) FILTER (WHERE duration_seconds IS NOT NULL), 0) DESC,
			         COUNT(*) DESC
			LIMIT 50`
	default:
		return nil, fmt.Errorf("unknown performance kind")
	}
	rows, err := a.db.Query(ctx, query, runIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []monitoringPerformanceRow{}
	for rows.Next() {
		var item monitoringPerformanceRow
		if err := rows.Scan(&item.Key, &item.PipelinePath, &item.PipelineName, &item.StepName, &item.TaskName, &item.TotalRuns,
			&item.SuccessfulRuns, &item.FailedRuns, &item.CancelledRuns, &item.TimeoutRuns, &item.AverageDurationSeconds,
			&item.MedianDurationSeconds, &item.P95DurationSeconds, &item.P99DurationSeconds, &item.MaxDurationSeconds,
			&item.TotalDurationSeconds, &item.AverageQueueSeconds); err != nil {
			return nil, err
		}
		completed := item.SuccessfulRuns + item.FailedRuns + item.CancelledRuns
		if completed > 0 {
			item.SuccessRate = float64(item.SuccessfulRuns) / float64(completed)
			item.FailureRate = float64(item.FailedRuns) / float64(completed)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) loadMonitoringTriggerAnalytics(ctx context.Context, filters monitoringAnalyticsFilters, runIDs []string) (monitoringTriggerAnalyticsResponse, error) {
	resp := monitoringTriggerAnalyticsResponse{Window: windowResponse(filters)}
	if len(runIDs) == 0 {
		return resp, nil
	}
	var err error
	resp.TriggerSources, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT COALESCE(NULLIF(trigger_source, ''), 'unknown'), COALESCE(NULLIF(trigger_source, ''), 'Unknown'),
		       COUNT(*),
		       COUNT(*) FILTER (WHERE LOWER(status) IN ('failure', 'failed')),
		       CASE WHEN COUNT(*) = 0 THEN 0 ELSE COUNT(*) FILTER (WHERE LOWER(status) IN ('failure', 'failed'))::float8 / COUNT(*)::float8 END,
		       0::float8
		FROM pipeline_runs
		WHERE run_id::text = ANY($1)
		GROUP BY 1,2
		ORDER BY COUNT(*) DESC, 2
	`, runIDs)
	if err != nil {
		return resp, err
	}
	resp.TriggerSourceTrend, err = a.loadMonitoringTimeBuckets(ctx, runIDs, "trigger_source")
	if err != nil {
		return resp, err
	}
	resp.FailuresByTriggerSource, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT COALESCE(NULLIF(trigger_source, ''), 'unknown'), COALESCE(NULLIF(trigger_source, ''), 'Unknown'),
		       COUNT(*) FILTER (WHERE LOWER(status) IN ('failure', 'failed')), 0::bigint, 0::float8, 0::float8
		FROM pipeline_runs
		WHERE run_id::text = ANY($1)
		GROUP BY 1,2
		ORDER BY 3 DESC, 2
		LIMIT 20
	`, runIDs)
	if err != nil {
		return resp, err
	}
	resp.DurationByTriggerSource, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT COALESCE(NULLIF(trigger_source, ''), 'unknown'), COALESCE(NULLIF(trigger_source, ''), 'Unknown'), COUNT(*), 0::bigint,
		       COALESCE(AVG(EXTRACT(EPOCH FROM finished_at - started_at)) FILTER (WHERE started_at IS NOT NULL AND finished_at IS NOT NULL AND finished_at >= started_at), 0)::float8,
		       0::float8
		FROM pipeline_runs
		WHERE run_id::text = ANY($1)
		GROUP BY 1,2
		ORDER BY 5 DESC, 2
		LIMIT 20
	`, runIDs)
	if err != nil {
		return resp, err
	}
	resp.TokenByTriggerSource, err = a.loadMonitoringTokenCounts(ctx, `
		SELECT COALESCE(NULLIF(pr.trigger_source, ''), 'unknown'), COALESCE(NULLIF(pr.trigger_source, ''), 'Unknown'), COUNT(DISTINCT pr.run_id),
		       COALESCE(SUM(au.total_tokens), 0)::bigint, 0::float8
		FROM pipeline_runs pr
		JOIN ai_usage_events au ON au.run_id = pr.run_id
			AND au.created_at >= $2 AND au.created_at <= $3
		WHERE pr.run_id::text = ANY($1)
		GROUP BY 1,2
		ORDER BY 4 DESC, 2
		LIMIT 20
	`, runIDs, filters.From, filters.To)
	if err != nil {
		return resp, err
	}
	resp.TriggerSourceReliability = resp.TriggerSources
	return resp, nil
}

func (a *App) loadMonitoringExternalTriggerAnalytics(ctx context.Context, filters monitoringAnalyticsFilters, runIDs []string, triggerIDs []string) (monitoringExternalTriggerAnalyticsResponse, error) {
	resp := monitoringExternalTriggerAnalyticsResponse{Window: windowResponse(filters)}
	if len(triggerIDs) == 0 {
		return resp, nil
	}
	if err := a.db.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE enabled), COUNT(*) FILTER (WHERE NOT enabled)
		FROM external_triggers
		WHERE id = ANY($1)
	`, triggerIDs).Scan(&resp.TotalExternalTriggers, &resp.EnabledExternalTriggers, &resp.DisabledExternalTriggers); err != nil {
		return resp, err
	}
	var runBound int64
	if err := a.db.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE LOWER(status) IN ('queued', 'success', 'succeeded')),
			COUNT(*) FILTER (WHERE LOWER(status) IN ('failed', 'failure')),
			COUNT(*) FILTER (WHERE LOWER(status) IN ('pending', 'queued')),
			COUNT(*) FILTER (WHERE run_id IS NOT NULL)
		FROM external_trigger_invocations
		WHERE trigger_id = ANY($1)
		  AND created_at >= $2 AND created_at <= $3
	`, triggerIDs, filters.From, filters.To).Scan(&resp.InvocationCount, &resp.SuccessfulInvocations, &resp.FailedInvocations, &resp.PendingInvocations, &runBound); err != nil {
		return resp, err
	}
	if resp.InvocationCount > 0 {
		resp.InvocationToRunRate = float64(runBound) / float64(resp.InvocationCount)
	}
	var err error
	resp.MostFiredTriggers, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT eti.trigger_id, COALESCE(NULLIF(et.name, ''), eti.trigger_id), COUNT(*),
		       COUNT(*) FILTER (WHERE LOWER(eti.status) IN ('failed', 'failure')),
		       CASE WHEN COUNT(*) = 0 THEN 0 ELSE COUNT(*) FILTER (WHERE LOWER(eti.status) IN ('failed', 'failure'))::float8 / COUNT(*)::float8 END,
		       0::float8
		FROM external_trigger_invocations eti
		LEFT JOIN external_triggers et ON et.id = eti.trigger_id
		WHERE eti.trigger_id = ANY($1)
		  AND eti.created_at >= $2 AND eti.created_at <= $3
		GROUP BY eti.trigger_id, et.name
		ORDER BY COUNT(*) DESC, 2
		LIMIT 20
	`, triggerIDs, filters.From, filters.To)
	if err != nil {
		return resp, err
	}
	resp.TopCallers, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT caller_type || ':' || caller_id, caller_type || ':' || caller_id, COUNT(*), 0::bigint, 0::float8, 0::float8
		FROM external_trigger_invocations
		WHERE trigger_id = ANY($1)
		  AND created_at >= $2 AND created_at <= $3
		GROUP BY 1,2
		ORDER BY COUNT(*) DESC, 2
		LIMIT 20
	`, triggerIDs, filters.From, filters.To)
	if err != nil {
		return resp, err
	}
	resp.ErrorReasons, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT LEFT(COALESCE(NULLIF(error, ''), 'unknown'), 180), LEFT(COALESCE(NULLIF(error, ''), 'Unknown'), 180), COUNT(*), 0::bigint, 0::float8, 0::float8
		FROM external_trigger_invocations
		WHERE trigger_id = ANY($1)
		  AND created_at >= $2 AND created_at <= $3
		  AND LOWER(status) IN ('failed', 'failure')
		GROUP BY 1,2
		ORDER BY COUNT(*) DESC, 2
		LIMIT 20
	`, triggerIDs, filters.From, filters.To)
	if err != nil {
		return resp, err
	}
	if err := a.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM (
			SELECT trigger_id, caller_type, caller_id, idempotency_key, COUNT(*)
			FROM external_trigger_invocations
			WHERE trigger_id = ANY($1)
			  AND created_at >= $2 AND created_at <= $3
			  AND idempotency_key <> ''
			GROUP BY trigger_id, caller_type, caller_id, idempotency_key
			HAVING COUNT(*) > 1
		) conflicts
	`, triggerIDs, filters.From, filters.To).Scan(&resp.IdempotencyConflicts); err != nil {
		return resp, err
	}
	resp.LastFiredTriggers, err = a.loadMonitoringExternalTriggerLastFired(ctx, triggerIDs)
	if err != nil {
		return resp, err
	}
	if err := a.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM external_trigger_invocations
		WHERE trigger_id = ANY($1)
		  AND created_at >= $2 AND created_at <= $3
		  AND (
		      LOWER(COALESCE(error, '')) LIKE '%rate limit%'
		      OR LOWER(COALESCE(error, '')) LIKE '%limit exceeded%'
		  )
	`, triggerIDs, filters.From, filters.To).Scan(&resp.RateLimitViolations); err != nil {
		return resp, err
	}
	resp.RateLimitViolationTriggers, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT eti.trigger_id, COALESCE(NULLIF(et.name, ''), eti.trigger_id), COUNT(*), 0::bigint, 0::float8, 0::float8
		FROM external_trigger_invocations eti
		LEFT JOIN external_triggers et ON et.id = eti.trigger_id
		WHERE eti.trigger_id = ANY($1)
		  AND eti.created_at >= $2 AND eti.created_at <= $3
		  AND (
		      LOWER(COALESCE(eti.error, '')) LIKE '%rate limit%'
		      OR LOWER(COALESCE(eti.error, '')) LIKE '%limit exceeded%'
		  )
		GROUP BY eti.trigger_id, et.name
		ORDER BY COUNT(*) DESC, 2
		LIMIT 20
	`, triggerIDs, filters.From, filters.To)
	if err != nil {
		return resp, err
	}
	_ = runIDs
	return resp, nil
}

func (a *App) loadMonitoringExternalTriggerLastFired(ctx context.Context, triggerIDs []string) ([]monitoringExternalTriggerLastFired, error) {
	if len(triggerIDs) == 0 {
		return nil, nil
	}
	rows, err := a.db.Query(ctx, `
		SELECT id,
		       COALESCE(NULLIF(name, ''), id),
		       enabled,
		       last_used_at,
		       COALESCE(CONCAT_WS(', ',
		           CASE WHEN rate_limit ? 'per_minute' THEN (rate_limit->>'per_minute') || '/min' END,
		           CASE WHEN rate_limit ? 'requests_per_minute' THEN (rate_limit->>'requests_per_minute') || '/min' END,
		           CASE WHEN rate_limit ? 'invocations_per_minute' THEN (rate_limit->>'invocations_per_minute') || '/min' END,
		           CASE WHEN rate_limit ? 'per_hour' THEN (rate_limit->>'per_hour') || '/hour' END
		       ), '')
		FROM external_triggers
		WHERE id = ANY($1)
		ORDER BY last_used_at DESC NULLS LAST, name ASC
		LIMIT 20
	`, triggerIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []monitoringExternalTriggerLastFired{}
	for rows.Next() {
		var item monitoringExternalTriggerLastFired
		var lastUsedAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.Name, &item.Enabled, &lastUsedAt, &item.RateLimit); err != nil {
			return nil, err
		}
		if lastUsedAt.Valid {
			item.LastUsedAt = &lastUsedAt.Time
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) loadMonitoringAIUsage(ctx context.Context, filters monitoringAnalyticsFilters, runIDs []string) (monitoringAIUsageResponse, error) {
	resp := monitoringAIUsageResponse{Window: windowResponse(filters)}
	if len(runIDs) == 0 {
		return resp, nil
	}
	if err := a.db.QueryRow(ctx, monitoringAIUsageTotalsQuery(), runIDs, filters.From, filters.To).Scan(
		&resp.TotalPromptTokens,
		&resp.TotalCompletionTokens,
		&resp.TotalTokens,
		&resp.ExactTokens,
		&resp.EstimatedTokens,
		&resp.ExactTokenEvents,
		&resp.EstimatedTokenEvents,
	); err != nil {
		return resp, err
	}
	var err error
	resp.ByPipeline, err = a.loadMonitoringTokenCounts(ctx, monitoringAIUsageByPipelineQuery(), runIDs, filters.From, filters.To)
	if err != nil {
		return resp, err
	}
	resp.ByStep, err = a.loadAIUsageGroup(ctx, runIDs, filters, "step_name")
	if err != nil {
		return resp, err
	}
	resp.ByTask, err = a.loadMonitoringTokenCounts(ctx, monitoringAIUsageByTaskQuery(), runIDs, filters.From, filters.To)
	if err != nil {
		return resp, err
	}
	resp.ByFeature, err = a.loadAIUsageGroup(ctx, runIDs, filters, "feature")
	if err != nil {
		return resp, err
	}
	resp.ByProfile, err = a.loadAIUsageGroup(ctx, runIDs, filters, "llm_profile")
	if err != nil {
		return resp, err
	}
	resp.ByModel, err = a.loadAIUsageGroup(ctx, runIDs, filters, "provider || '/' || model")
	if err != nil {
		return resp, err
	}
	resp.BySubject, err = a.loadAIUsageGroup(ctx, runIDs, filters, "effective_subject_type || ':' || effective_subject_id")
	if err != nil {
		return resp, err
	}
	resp.Trend, err = a.loadAIUsageTrend(ctx, runIDs, filters)
	if err != nil {
		return resp, err
	}
	resp.TopTokenRuns, err = a.loadMonitoringTokenCounts(ctx, monitoringAITopTokenRunsQuery(), runIDs, filters.From, filters.To)
	return resp, err
}

func (a *App) loadMonitoringReliability(ctx context.Context, filters monitoringAnalyticsFilters, runIDs []string) (monitoringReliabilityResponse, error) {
	resp := monitoringReliabilityResponse{Window: windowResponse(filters)}
	if len(runIDs) == 0 {
		return resp, nil
	}
	var err error
	resp.RecentFailures, err = a.loadMonitoringRunRows(ctx, runIDs, `
		AND LOWER(pr.status) IN ('failure', 'failed')
		ORDER BY pr.created_at DESC
		LIMIT 20`)
	if err != nil {
		return resp, err
	}
	resp.FailureReasons, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT LEFT(COALESCE(NULLIF(failure_reason, ''), 'unknown'), 180), LEFT(COALESCE(NULLIF(failure_reason, ''), 'Unknown'), 180),
		       COUNT(*), 0::bigint, 0::float8, 0::float8
		FROM pipeline_runs
		WHERE run_id::text = ANY($1)
		  AND LOWER(status) IN ('failure', 'failed')
		GROUP BY 1,2
		ORDER BY COUNT(*) DESC
		LIMIT 20
	`, runIDs)
	if err != nil {
		return resp, err
	}
	perf, err := a.loadMonitoringPerformance(ctx, runIDs, "pipeline")
	if err != nil {
		return resp, err
	}
	for _, item := range perf {
		if item.FailedRuns >= 2 {
			resp.RepeatedFailurePipelines = append(resp.RepeatedFailurePipelines, item)
		}
		if item.FailedRuns > 0 && item.SuccessfulRuns > 0 {
			resp.FlakyPipelines = append(resp.FlakyPipelines, item)
		}
	}
	resp.StuckRuns, err = a.loadMonitoringRunRows(ctx, runIDs, `
		AND LOWER(pr.status) IN ('pending', 'running', 'waiting_approval')
		AND pr.created_at < NOW() - INTERVAL '1 hour'
		ORDER BY pr.created_at ASC
		LIMIT 20`)
	if err != nil {
		return resp, err
	}
	resp.ApprovalsWaitingTooLong, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT pa.run_id::text, COALESCE(pr.pipeline_name, pa.run_id::text), COUNT(*), 0::bigint,
		       COALESCE(MAX(EXTRACT(EPOCH FROM NOW() - pa.requested_at)), 0)::float8, 0::float8
		FROM pipeline_approvals pa
		JOIN pipeline_runs pr ON pr.run_id = pa.run_id
		WHERE pa.run_id::text = ANY($1)
		  AND pa.status = 'pending'
		  AND pa.requested_at < NOW() - INTERVAL '1 hour'
		GROUP BY pa.run_id, pr.pipeline_name
		ORDER BY 5 DESC
		LIMIT 20
	`, runIDs)
	if err != nil {
		return resp, err
	}
	resp.NotificationFailures, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT channel || ':' || event_type, channel || ':' || event_type, COUNT(*), 0::bigint, 0::float8, 0::float8
		FROM notification_deliveries
		WHERE run_id::text = ANY($1)
		  AND LOWER(status) = 'failed'
		GROUP BY 1,2
		ORDER BY COUNT(*) DESC
		LIMIT 20
	`, runIDs)
	if err != nil {
		return resp, err
	}
	resp.FailedExternalInvocations, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT trigger_id, trigger_id, COUNT(*), 0::bigint, 0::float8, 0::float8
		FROM external_trigger_invocations
		WHERE run_id::text = ANY($1)
		  AND LOWER(status) IN ('failed', 'failure')
		GROUP BY trigger_id
		ORDER BY COUNT(*) DESC
		LIMIT 20
	`, runIDs)
	return resp, err
}

func (a *App) loadMonitoringEfficiency(ctx context.Context, filters monitoringAnalyticsFilters, runIDs []string) (monitoringEfficiencyResponse, error) {
	resp := monitoringEfficiencyResponse{Window: windowResponse(filters)}
	if len(runIDs) == 0 {
		return resp, nil
	}
	if err := a.db.QueryRow(ctx, `
		WITH durations AS (
			SELECT EXTRACT(EPOCH FROM finished_at - started_at)::float8 AS duration_seconds
			FROM pipeline_runs
			WHERE run_id::text = ANY($1)
			  AND started_at IS NOT NULL AND finished_at IS NOT NULL AND finished_at >= started_at
		)
		SELECT COALESCE(SUM(duration_seconds), 0)::float8
		FROM durations
	`, runIDs).Scan(&resp.TotalRuntimeSeconds); err != nil {
		return resp, err
	}
	resp.TotalRunnerMinutes = resp.TotalRuntimeSeconds / 60
	if err := a.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(total_tokens), 0)::bigint
		FROM ai_usage_events
		WHERE run_id::text = ANY($1)
		  AND created_at >= $2 AND created_at <= $3
	`, runIDs, filters.From, filters.To).Scan(&resp.TotalAITokens); err != nil {
		return resp, err
	}
	var err error
	resp.TokenByPipeline, err = a.loadMonitoringTokenCounts(ctx, `
		SELECT COALESCE(pr.pipeline_path, '') || '/' || COALESCE(pr.pipeline_name, ''),
		       COALESCE(NULLIF(pr.pipeline_name, ''), COALESCE(pr.pipeline_path, '') || '/'),
		       COUNT(DISTINCT pr.run_id), COALESCE(SUM(au.total_tokens), 0)::bigint, 0::float8
		FROM pipeline_runs pr
		JOIN ai_usage_events au ON au.run_id = pr.run_id
			AND au.created_at >= $2 AND au.created_at <= $3
		WHERE pr.run_id::text = ANY($1)
		GROUP BY pr.pipeline_path, pr.pipeline_name
		ORDER BY 4 DESC, 2
		LIMIT 20
	`, runIDs, filters.From, filters.To)
	if err != nil {
		return resp, err
	}
	resp.TokenByGroup, err = a.loadMonitoringTokenCounts(ctx, `
		SELECT COALESCE(g.id::text, 'root'), COALESCE(g.name, 'Root'), COUNT(DISTINCT pr.run_id),
		       COALESCE(SUM(au.total_tokens), 0)::bigint, 0::float8
		FROM pipeline_runs pr
		LEFT JOIN groups g ON g.id = pr.group_id
		JOIN ai_usage_events au ON au.run_id = pr.run_id
			AND au.created_at >= $2 AND au.created_at <= $3
		WHERE pr.run_id::text = ANY($1)
		GROUP BY g.id, g.name
		ORDER BY 4 DESC, 2
		LIMIT 20
	`, runIDs, filters.From, filters.To)
	if err != nil {
		return resp, err
	}
	resp.TokenByStep, err = a.loadMonitoringTokenCounts(ctx, `
		SELECT COALESCE(NULLIF(step_name, ''), 'unknown'), COALESCE(NULLIF(step_name, ''), 'Unknown'),
		       COUNT(*), COALESCE(SUM(total_tokens), 0)::bigint, 0::float8
		FROM ai_usage_events
		WHERE run_id::text = ANY($1)
		  AND created_at >= $2 AND created_at <= $3
		GROUP BY 1,2
		ORDER BY 4 DESC, 2
		LIMIT 20
	`, runIDs, filters.From, filters.To)
	if err != nil {
		return resp, err
	}
	perf, err := a.loadMonitoringPerformance(ctx, runIDs, "pipeline")
	if err != nil {
		return resp, err
	}
	tokenByPipeline := make(map[string]int64, len(resp.TokenByPipeline))
	for _, item := range resp.TokenByPipeline {
		tokenByPipeline[item.Key] = item.Tokens
	}
	for _, item := range perf {
		if item.TotalRuns >= 3 && item.SuccessRate < 0.6 && tokenByPipeline[item.Key] > 0 {
			resp.TokenHeavyLowSuccessPipelines = append(resp.TokenHeavyLowSuccessPipelines, item)
		}
	}
	resp.FrequentReruns, err = a.loadMonitoringPipelineReruns(ctx, runIDs)
	if err != nil {
		return resp, err
	}
	resp.HighQueueGroups, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT COALESCE(g.id::text, 'root'), COALESCE(g.name, 'Root'), COUNT(*), 0::bigint,
		       COALESCE(AVG(EXTRACT(EPOCH FROM pr.started_at - pr.created_at)) FILTER (WHERE pr.started_at IS NOT NULL AND pr.started_at >= pr.created_at), 0)::float8,
		       0::float8
		FROM pipeline_runs pr
		LEFT JOIN groups g ON g.id = pr.group_id
		WHERE pr.run_id::text = ANY($1)
		GROUP BY g.id, g.name
		ORDER BY 5 DESC
		LIMIT 20
	`, runIDs)
	if err != nil {
		return resp, err
	}
	resp.Recommendations = monitoringEfficiencyRecommendations(resp)
	if err := a.persistMonitoringRecommendations(ctx, resp.Recommendations); err != nil {
		log.Debug().Err(err).Msg("Failed to persist monitoring recommendations")
	}
	return resp, nil
}

func (a *App) loadMonitoringSecurity(ctx context.Context, filters monitoringAnalyticsFilters, runIDs []string, triggerIDs []string) (monitoringSecurityResponse, error) {
	resp := monitoringSecurityResponse{Window: windowResponse(filters)}
	if len(runIDs) == 0 {
		return resp, nil
	}
	var err error
	resp.RunsByRequester, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT requested_by_type || ':' || requested_by_id, requested_by_type || ':' || requested_by_id, COUNT(*), 0::bigint, 0::float8, 0::float8
		FROM pipeline_runs
		WHERE run_id::text = ANY($1)
		GROUP BY 1,2
		ORDER BY COUNT(*) DESC
		LIMIT 20
	`, runIDs)
	if err != nil {
		return resp, err
	}
	resp.RunsByEffectiveSubject, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT effective_subject_type || ':' || effective_subject_id, effective_subject_type || ':' || effective_subject_id, COUNT(*), 0::bigint, 0::float8, 0::float8
		FROM pipeline_runs
		WHERE run_id::text = ANY($1)
		GROUP BY 1,2
		ORDER BY COUNT(*) DESC
		LIMIT 20
	`, runIDs)
	if err != nil {
		return resp, err
	}
	resp.ServiceAccountRuns, err = a.loadMonitoringNamedCounts(ctx, `
		SELECT effective_subject_id, effective_subject_id, COUNT(*), 0::bigint, 0::float8, 0::float8
		FROM pipeline_runs
		WHERE run_id::text = ANY($1)
		  AND effective_subject_type = 'service_account'
		GROUP BY 1,2
		ORDER BY COUNT(*) DESC
		LIMIT 20
	`, runIDs)
	if err != nil {
		return resp, err
	}
	if len(triggerIDs) > 0 {
		resp.ExternalTriggerCallers, err = a.loadMonitoringNamedCounts(ctx, `
			SELECT caller_type || ':' || caller_id, caller_type || ':' || caller_id, COUNT(*), 0::bigint, 0::float8, 0::float8
			FROM external_trigger_invocations
			WHERE trigger_id = ANY($1)
			  AND created_at >= $2 AND created_at <= $3
			GROUP BY 1,2
			ORDER BY COUNT(*) DESC
			LIMIT 20
		`, triggerIDs, filters.From, filters.To)
		if err != nil {
			return resp, err
		}
	}
	perf, err := a.loadMonitoringPerformance(ctx, runIDs, "pipeline")
	if err != nil {
		return resp, err
	}
	for _, item := range perf {
		if item.FailedRuns > 0 && (strings.Contains(strings.ToLower(item.PipelineName), "prod") || strings.Contains(strings.ToLower(item.PipelinePath), "prod")) {
			resp.HighRiskFailedPipelines = append(resp.HighRiskFailedPipelines, item)
		}
	}
	return resp, nil
}

func (a *App) loadMonitoringTimeBuckets(ctx context.Context, runIDs []string, splitColumn string) ([]monitoringTimeBucket, error) {
	if len(runIDs) == 0 {
		return []monitoringTimeBucket{}, nil
	}
	labelExpr := "TO_CHAR(DATE_TRUNC('day', created_at), 'YYYY-MM-DD')"
	if splitColumn != "" {
		labelExpr = "TO_CHAR(DATE_TRUNC('day', created_at), 'YYYY-MM-DD') || ' ' || COALESCE(NULLIF(" + splitColumn + ", ''), 'unknown')"
	}
	rows, err := a.db.Query(ctx, `
		SELECT `+labelExpr+`,
		       `+labelExpr+`,
		       COUNT(*),
		       COUNT(*) FILTER (WHERE LOWER(status) IN ('failure', 'failed')),
		       COALESCE(AVG(EXTRACT(EPOCH FROM finished_at - started_at)) FILTER (WHERE started_at IS NOT NULL AND finished_at IS NOT NULL AND finished_at >= started_at), 0)::float8,
		       COALESCE(SUM(EXTRACT(EPOCH FROM finished_at - started_at)) FILTER (WHERE started_at IS NOT NULL AND finished_at IS NOT NULL AND finished_at >= started_at), 0)::float8
		FROM pipeline_runs
		WHERE run_id::text = ANY($1)
		GROUP BY 1,2
		ORDER BY 1
	`, runIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []monitoringTimeBucket{}
	for rows.Next() {
		var item monitoringTimeBucket
		if err := rows.Scan(&item.Key, &item.Label, &item.Runs, &item.Failures, &item.AverageDurationSeconds, &item.TotalDurationSeconds); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) loadMonitoringNamedCounts(ctx context.Context, query string, args ...any) ([]monitoringNamedCount, error) {
	rows, err := a.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []monitoringNamedCount{}
	for rows.Next() {
		var item monitoringNamedCount
		if err := rows.Scan(&item.Key, &item.Label, &item.Count, &item.Failed, &item.Seconds, &item.CostUSD); err != nil {
			return nil, err
		}
		if item.Count > 0 && item.Failed > 0 && item.Failed <= item.Count {
			item.Rate = float64(item.Failed) / float64(item.Count)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) loadMonitoringTokenCounts(ctx context.Context, query string, args ...any) ([]monitoringNamedCount, error) {
	rows, err := a.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []monitoringNamedCount{}
	for rows.Next() {
		var item monitoringNamedCount
		if err := rows.Scan(&item.Key, &item.Label, &item.Count, &item.Tokens, &item.CostUSD); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) loadMonitoringDurationStats(ctx context.Context, runIDs []string, expression, predicate string) (monitoringDurationStats, error) {
	var stats monitoringDurationStats
	if len(runIDs) == 0 {
		return stats, nil
	}
	query := fmt.Sprintf(`
		WITH durations AS (
			SELECT EXTRACT(EPOCH FROM %s)::float8 AS duration_seconds
			FROM pipeline_runs
			WHERE run_id::text = ANY($1)
			  AND %s
		)
		SELECT
			COALESCE(AVG(duration_seconds), 0)::float8,
			COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_seconds), 0)::float8,
			COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_seconds), 0)::float8,
			COALESCE(PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY duration_seconds), 0)::float8,
			COALESCE(MAX(duration_seconds), 0)::float8,
			COALESCE(SUM(duration_seconds), 0)::float8
		FROM durations
	`, expression, predicate)
	err := a.db.QueryRow(ctx, query, runIDs).Scan(&stats.AverageSeconds, &stats.MedianSeconds, &stats.P95Seconds, &stats.P99Seconds, &stats.MaxSeconds, &stats.TotalSeconds)
	return stats, err
}

func (a *App) loadMonitoringRunRows(ctx context.Context, runIDs []string, suffix string) ([]monitoringRunRow, error) {
	if len(runIDs) == 0 {
		return []monitoringRunRow{}, nil
	}
	query := `
		SELECT pr.run_id::text, COALESCE(pr.pipeline_path, ''), COALESCE(pr.pipeline_name, ''), COALESCE(pr.status, ''),
		       COALESCE(g.name, ''),
		       CASE WHEN COALESCE(pr.git_repo_owner, '') <> '' AND COALESCE(pr.git_repo_name, '') <> ''
		            THEN pr.git_repo_owner || '/' || pr.git_repo_name
		            ELSE COALESCE(pr.git_repo_name, '')
		       END,
		       COALESCE(pr.git_ref, ''), COALESCE(pr.git_commit_sha, ''), COALESCE(pr.trigger_source, ''),
		       COALESCE(eti.trigger_id, ''), COALESCE(pr.schedule_id::text, ''), COALESCE(pr.failure_reason, ''),
		       pr.created_at, pr.started_at, pr.finished_at,
		       CASE WHEN pr.started_at IS NOT NULL AND pr.started_at >= pr.created_at
		            THEN EXTRACT(EPOCH FROM pr.started_at - pr.created_at)::float8 ELSE 0 END,
		       CASE WHEN pr.started_at IS NOT NULL AND pr.finished_at IS NOT NULL AND pr.finished_at >= pr.started_at
		            THEN EXTRACT(EPOCH FROM pr.finished_at - pr.started_at)::float8 ELSE 0 END,
		       CASE WHEN pr.finished_at IS NOT NULL AND pr.finished_at >= pr.created_at
		            THEN EXTRACT(EPOCH FROM pr.finished_at - pr.created_at)::float8 ELSE 0 END
		FROM pipeline_runs pr
		LEFT JOIN groups g ON g.id = pr.group_id
		LEFT JOIN external_trigger_invocations eti ON eti.id::text = pr.trigger_event_id OR eti.run_id = pr.run_id
		WHERE pr.run_id::text = ANY($1) ` + suffix
	rows, err := a.db.Query(ctx, query, runIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []monitoringRunRow{}
	for rows.Next() {
		item, err := scanMonitoringRunRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanMonitoringRunRow(scanner interface{ Scan(dest ...any) error }) (monitoringRunRow, error) {
	var item monitoringRunRow
	var startedAt, finishedAt sql.NullTime
	if err := scanner.Scan(&item.RunID, &item.PipelinePath, &item.PipelineName, &item.Status, &item.GroupName, &item.Repo, &item.Ref,
		&item.CommitSHA, &item.TriggerSource, &item.ExternalTriggerID, &item.ScheduleID, &item.FailureReason, &item.CreatedAt,
		&startedAt, &finishedAt, &item.QueueSeconds, &item.DurationSeconds, &item.EndToEndSeconds); err != nil {
		return item, err
	}
	if startedAt.Valid {
		item.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		item.FinishedAt = &finishedAt.Time
	}
	return item, nil
}

func (a *App) loadMonitoringHeatmap(ctx context.Context, runIDs []string) ([]monitoringHeatmapCell, error) {
	rows, err := a.db.Query(ctx, `
		SELECT EXTRACT(ISODOW FROM created_at)::int,
		       EXTRACT(HOUR FROM created_at)::int,
		       COUNT(*),
		       COUNT(*) FILTER (WHERE LOWER(status) IN ('failure', 'failed'))
		FROM pipeline_runs
		WHERE run_id::text = ANY($1)
		GROUP BY 1,2
		ORDER BY 1,2
	`, runIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []monitoringHeatmapCell{}
	for rows.Next() {
		var item monitoringHeatmapCell
		if err := rows.Scan(&item.DayOfWeek, &item.Hour, &item.Runs, &item.Failures); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) loadAIUsageGroup(ctx context.Context, runIDs []string, filters monitoringAnalyticsFilters, expression string) ([]monitoringNamedCount, error) {
	return a.loadMonitoringTokenCounts(ctx, monitoringAIUsageGroupQuery(expression), runIDs, filters.From, filters.To)
}

func (a *App) loadAIUsageTrend(ctx context.Context, runIDs []string, filters monitoringAnalyticsFilters) ([]monitoringTimeBucket, error) {
	rows, err := a.db.Query(ctx, monitoringAIUsageTrendQuery(), runIDs, filters.From, filters.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []monitoringTimeBucket{}
	for rows.Next() {
		var item monitoringTimeBucket
		if err := rows.Scan(&item.Key, &item.Label, &item.Runs, &item.Failures, &item.AverageDurationSeconds, &item.TotalDurationSeconds); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func monitoringAIUsageTotalsQuery() string {
	estimatedPredicate := monitoringEstimatedTokenPredicate()
	return `
		SELECT COALESCE(SUM(prompt_tokens), 0)::bigint, COALESCE(SUM(completion_tokens), 0)::bigint,
		       COALESCE(SUM(total_tokens), 0)::bigint,
		       COALESCE(SUM(total_tokens) FILTER (WHERE NOT (` + estimatedPredicate + `)), 0)::bigint,
		       COALESCE(SUM(total_tokens) FILTER (WHERE ` + estimatedPredicate + `), 0)::bigint,
		       (COUNT(*) FILTER (WHERE NOT (` + estimatedPredicate + `)))::bigint,
		       (COUNT(*) FILTER (WHERE ` + estimatedPredicate + `))::bigint
		FROM ai_usage_events
		WHERE run_id::text = ANY($1)
		  AND created_at >= $2 AND created_at <= $3`
}

func monitoringEstimatedTokenPredicate() string {
	return `LOWER(COALESCE(metadata->>'estimated_tokens', 'false')) IN ('true', '1', 'yes')`
}

func monitoringAIUsageByPipelineQuery() string {
	return `
		SELECT pipeline_path || '/' || pipeline_name, COALESCE(NULLIF(pipeline_name, ''), pipeline_path || '/'), COUNT(*),
		       COALESCE(SUM(total_tokens), 0)::bigint, 0::float8
		FROM ai_usage_events
		WHERE run_id::text = ANY($1)
		  AND created_at >= $2 AND created_at <= $3
		GROUP BY pipeline_path, pipeline_name
		ORDER BY 4 DESC, 2
		LIMIT 20`
}

func monitoringAITopTokenRunsQuery() string {
	return `
		SELECT run_id::text, run_id::text, COUNT(*), COALESCE(SUM(total_tokens), 0)::bigint, 0::float8
		FROM ai_usage_events
		WHERE run_id::text = ANY($1)
		  AND created_at >= $2 AND created_at <= $3
		GROUP BY run_id
		ORDER BY 4 DESC, 3 DESC
		LIMIT 20`
}

func monitoringAIUsageByTaskQuery() string {
	return `
		SELECT COALESCE(NULLIF(CONCAT_WS('/', NULLIF(step_name, ''), NULLIF(task_name, '')), ''), 'unknown'),
		       COALESCE(NULLIF(CONCAT_WS('/', NULLIF(step_name, ''), NULLIF(task_name, '')), ''), 'Unknown'),
		       COUNT(*), COALESCE(SUM(total_tokens), 0)::bigint, 0::float8
		FROM ai_usage_events
		WHERE run_id::text = ANY($1)
		  AND created_at >= $2 AND created_at <= $3
		  AND COALESCE(task_name, '') <> ''
		GROUP BY 1,2
		ORDER BY 4 DESC, 2
		LIMIT 20`
}

func monitoringAIUsageGroupQuery(expression string) string {
	return `
		SELECT COALESCE(NULLIF(` + expression + `, ''), 'unknown'), COALESCE(NULLIF(` + expression + `, ''), 'Unknown'),
		       COUNT(*), COALESCE(SUM(total_tokens), 0)::bigint, 0::float8
		FROM ai_usage_events
		WHERE run_id::text = ANY($1)
		  AND created_at >= $2 AND created_at <= $3
		GROUP BY 1,2
		ORDER BY 4 DESC, 2
		LIMIT 20`
}

func monitoringAIUsageTrendQuery() string {
	return `
		SELECT TO_CHAR(DATE_TRUNC('day', created_at), 'YYYY-MM-DD'),
		       TO_CHAR(DATE_TRUNC('day', created_at), 'YYYY-MM-DD'),
		       COALESCE(SUM(total_tokens), 0)::bigint,
		       0::bigint,
		       0::float8,
		       0::float8
		FROM ai_usage_events
		WHERE run_id::text = ANY($1)
		  AND created_at >= $2 AND created_at <= $3
		GROUP BY 1,2
		ORDER BY 1
	`
}

func (a *App) loadMonitoringPipelineReruns(ctx context.Context, runIDs []string) ([]monitoringPerformanceRow, error) {
	rows, err := a.db.Query(ctx, `
		SELECT COALESCE(pipeline_path, '') || '/' || COALESCE(pipeline_name, ''),
		       COALESCE(pipeline_path, ''), COALESCE(pipeline_name, ''), '', '',
		       COUNT(*), 0::bigint, COUNT(*) FILTER (WHERE LOWER(status) IN ('failure', 'failed')),
		       0::bigint, 0::bigint, 0::float8, 0::float8, 0::float8, 0::float8, 0::float8, 0::float8, 0::float8
		FROM pipeline_runs
		WHERE run_id::text = ANY($1)
		  AND parent_run_id IS NOT NULL
		GROUP BY pipeline_path, pipeline_name
		ORDER BY COUNT(*) DESC
		LIMIT 20
	`, runIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []monitoringPerformanceRow{}
	for rows.Next() {
		var item monitoringPerformanceRow
		if err := rows.Scan(&item.Key, &item.PipelinePath, &item.PipelineName, &item.StepName, &item.TaskName, &item.TotalRuns,
			&item.SuccessfulRuns, &item.FailedRuns, &item.CancelledRuns, &item.TimeoutRuns, &item.AverageDurationSeconds,
			&item.MedianDurationSeconds, &item.P95DurationSeconds, &item.P99DurationSeconds, &item.MaxDurationSeconds,
			&item.TotalDurationSeconds, &item.AverageQueueSeconds); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func monitoringEfficiencyRecommendations(resp monitoringEfficiencyResponse) []string {
	recommendations := []string{}
	if len(resp.TokenHeavyLowSuccessPipelines) > 0 {
		item := resp.TokenHeavyLowSuccessPipelines[0]
		recommendations = append(recommendations, fmt.Sprintf("Pipeline %s has a %.0f%% success rate across %d runs.", item.Key, item.SuccessRate*100, item.TotalRuns))
	}
	if len(resp.HighQueueGroups) > 0 && resp.HighQueueGroups[0].Seconds > 300 {
		item := resp.HighQueueGroups[0]
		recommendations = append(recommendations, fmt.Sprintf("Group %s has average queue time above five minutes.", item.Label))
	}
	if resp.TotalAITokens > 0 && len(resp.TokenByPipeline) > 0 {
		item := resp.TokenByPipeline[0]
		recommendations = append(recommendations, fmt.Sprintf("Pipeline %s is the highest AI token consumer in this window.", item.Label))
	}
	return recommendations
}

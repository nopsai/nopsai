package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/pkg/serviceauth"
	aaamodel "nopsai/services/aaa/pkg/model"
)

func (a *App) handleRecordAIUsage(w http.ResponseWriter, r *http.Request) {
	if !requireInternalServiceRole(w, r, serviceauth.RoleAgent) {
		return
	}
	runID := strings.TrimSpace(r.PathValue("runID"))
	if _, err := uuid.Parse(runID); err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}
	var report models.AIUsageReport
	if err := httpapi.DecodeJSON(r, &report); err != nil {
		http.Error(w, "Invalid AI usage report", http.StatusBadRequest)
		return
	}
	report, ok := normalizeAIUsageReport(report)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := a.recordAIUsage(r.Context(), runID, report); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to record AI usage")
		http.Error(w, "Failed to record AI usage", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func normalizeAIUsageReport(report models.AIUsageReport) (models.AIUsageReport, bool) {
	report.StepName = strings.TrimSpace(report.StepName)
	report.TaskName = strings.TrimSpace(report.TaskName)
	report.Feature = strings.TrimSpace(report.Feature)
	if report.Feature == "" {
		report.Feature = "unknown"
	}
	report.Provider = strings.TrimSpace(report.Provider)
	if report.Provider == "" {
		report.Provider = "unknown"
	}
	report.ProviderModel = strings.TrimSpace(report.ProviderModel)
	report.LLMProfile = strings.TrimSpace(report.LLMProfile)
	report.PromptTokens = nonNegativeInt64(report.PromptTokens)
	report.CompletionTokens = nonNegativeInt64(report.CompletionTokens)
	report.TotalTokens = nonNegativeInt64(report.TotalTokens)
	report.CachedInputTokens = nonNegativeInt64(report.CachedInputTokens)
	report.CacheWriteTokens = nonNegativeInt64(report.CacheWriteTokens)
	if report.TotalTokens == 0 {
		report.TotalTokens = report.PromptTokens + report.CompletionTokens
	}
	// Cost is never taken from the caller. The rate card belongs to the model
	// definition, which only this service can see, so a reported cost would be a
	// number the reporter had no way to compute.
	report.InputCostUSD = 0
	report.OutputCostUSD = 0
	report.TotalCostUSD = 0
	if report.Metadata == nil {
		report.Metadata = map[string]any{}
	}
	return report, report.PromptTokens > 0 || report.CompletionTokens > 0 || report.TotalTokens > 0
}

// aiUsageCost is the money side of a usage event. Priced is false when the call
// could not be turned into a dollar figure, which stores NULL rather than zero
// so that an unpriced call is visibly missing instead of silently free.
type aiUsageCost struct {
	Priced bool
	Input  float64
	Output float64
	Total  float64
}

func (c aiUsageCost) column(value float64) any {
	if !c.Priced {
		return nil
	}
	return value
}

// priceAIUsage turns a token report into money using the rate card declared on
// the model that produced it.
//
// Two cases refuse to produce a figure. An estimated token count is a guess from
// character length, and multiplying a guess by a rate produces a number that
// looks authoritative and is not. A model with no rate card cannot be priced at
// all — most often because the event names a model that has since been removed
// from the configuration repository.
func (a *App) priceAIUsage(report models.AIUsageReport) aiUsageCost {
	if report.Estimated {
		return aiUsageCost{}
	}
	_, profiles := a.llmProfilesSnapshot()
	profile, ok := profiles[config.NormalizeLLMProfileName(report.LLMProfile)]
	if !ok || profile.Pricing == nil {
		return aiUsageCost{}
	}
	input, output := profile.Pricing.CostUSD(
		report.PromptTokens,
		report.CompletionTokens,
		report.CachedInputTokens,
		report.CacheWriteTokens,
	)
	return aiUsageCost{Priced: true, Input: input, Output: output, Total: input + output}
}

func nonNegativeInt64(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

// aiUsageAttribution is the run context an event is filed under. A usage event
// that has no run — an assistant analysis of a pipeline, for instance — still
// counts towards spend and is recorded with an empty attribution rather than
// dropped.
type aiUsageAttribution struct {
	RunID                string
	PipelinePath         string
	PipelineName         string
	TeamID               sql.NullInt64
	RequestedByType      string
	RequestedByID        string
	EffectiveSubjectType string
	EffectiveSubjectID   string
}

func (a *App) recordAIUsage(ctx context.Context, runID string, report models.AIUsageReport) error {
	attribution, err := a.aiUsageAttributionForRun(ctx, runID)
	if err != nil {
		return err
	}
	return a.insertAIUsage(ctx, attribution, report)
}

// recordStandaloneAIUsage records spend that no pipeline run produced, such as
// an assistant analysis of a pipeline rather than of one of its runs. Without
// this path those calls cost real money and appear nowhere.
func (a *App) recordStandaloneAIUsage(ctx context.Context, subject aaamodel.Subject, report models.AIUsageReport) error {
	attribution := aiUsageAttribution{
		RequestedByType:      strings.TrimSpace(subject.Type),
		RequestedByID:        strings.TrimSpace(subject.ID),
		EffectiveSubjectType: strings.TrimSpace(subject.Type),
		EffectiveSubjectID:   strings.TrimSpace(subject.ID),
	}
	return a.insertAIUsage(ctx, attribution, report)
}

func (a *App) aiUsageAttributionForRun(ctx context.Context, runID string) (aiUsageAttribution, error) {
	if a == nil || a.db == nil {
		return aiUsageAttribution{}, fmt.Errorf("database is unavailable")
	}
	attribution := aiUsageAttribution{RunID: runID}
	var (
		requestedByType      sql.NullString
		requestedByID        sql.NullString
		effectiveSubjectType sql.NullString
		effectiveSubjectID   sql.NullString
	)
	if err := a.db.QueryRow(ctx, `
		SELECT COALESCE(pipeline_path, ''), COALESCE(pipeline_name, ''), team_id,
		       COALESCE(requested_by_type, ''), COALESCE(requested_by_id, ''),
		       COALESCE(effective_subject_type, ''), COALESCE(effective_subject_id, '')
		FROM pipeline_runs
		WHERE run_id = $1
	`, runID).Scan(
		&attribution.PipelinePath,
		&attribution.PipelineName,
		&attribution.TeamID,
		&requestedByType,
		&requestedByID,
		&effectiveSubjectType,
		&effectiveSubjectID,
	); err != nil {
		return aiUsageAttribution{}, fmt.Errorf("load run metadata: %w", err)
	}
	attribution.RequestedByType = requestedByType.String
	attribution.RequestedByID = requestedByID.String
	attribution.EffectiveSubjectType = effectiveSubjectType.String
	attribution.EffectiveSubjectID = effectiveSubjectID.String
	return attribution, nil
}

func (a *App) insertAIUsage(ctx context.Context, attribution aiUsageAttribution, report models.AIUsageReport) error {
	if a == nil || a.db == nil {
		return fmt.Errorf("database is unavailable")
	}
	cost := a.priceAIUsage(report)
	if !cost.Priced {
		report.Metadata["unpriced"] = true
	}
	metadataJSON, err := json.Marshal(report.Metadata)
	if err != nil {
		return fmt.Errorf("marshal AI usage metadata: %w", err)
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_usage_events (
			run_id, step_name, task_name, pipeline_path, pipeline_name, team_id,
			feature, provider, model, llm_profile,
			prompt_tokens, completion_tokens, total_tokens, cached_input_tokens, cache_write_tokens,
			input_cost_usd, output_cost_usd, total_cost_usd,
			requested_by_type, requested_by_id, effective_subject_type, effective_subject_id, metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23::jsonb)
	`, nullableRunID(attribution.RunID), report.StepName, report.TaskName, attribution.PipelinePath, attribution.PipelineName, nullableTeamID(attribution.TeamID),
		report.Feature, report.Provider, report.ProviderModel, report.LLMProfile,
		report.PromptTokens, report.CompletionTokens, report.TotalTokens, report.CachedInputTokens, report.CacheWriteTokens,
		cost.column(cost.Input), cost.column(cost.Output), cost.column(cost.Total),
		attribution.RequestedByType, attribution.RequestedByID, attribution.EffectiveSubjectType, attribution.EffectiveSubjectID, string(metadataJSON)); err != nil {
		return fmt.Errorf("insert AI usage event: %w", err)
	}
	if attribution.RunID == "" {
		return tx.Commit(ctx)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO pipeline_run_usage_summary (
			run_id, ai_prompt_tokens, ai_completion_tokens, ai_total_tokens, ai_cost_usd, ai_unpriced_calls, total_cost_usd, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $5, NOW())
		ON CONFLICT (run_id) DO UPDATE SET
			ai_prompt_tokens = pipeline_run_usage_summary.ai_prompt_tokens + EXCLUDED.ai_prompt_tokens,
			ai_completion_tokens = pipeline_run_usage_summary.ai_completion_tokens + EXCLUDED.ai_completion_tokens,
			ai_total_tokens = pipeline_run_usage_summary.ai_total_tokens + EXCLUDED.ai_total_tokens,
			ai_cost_usd = pipeline_run_usage_summary.ai_cost_usd + EXCLUDED.ai_cost_usd,
			ai_unpriced_calls = pipeline_run_usage_summary.ai_unpriced_calls + EXCLUDED.ai_unpriced_calls,
			total_cost_usd = pipeline_run_usage_summary.total_cost_usd + EXCLUDED.ai_cost_usd,
			updated_at = NOW()
	`, attribution.RunID, report.PromptTokens, report.CompletionTokens, report.TotalTokens, cost.Total, unpricedCallCount(cost)); err != nil {
		return fmt.Errorf("update AI usage summary: %w", err)
	}
	return tx.Commit(ctx)
}

func unpricedCallCount(cost aiUsageCost) int64 {
	if cost.Priced {
		return 0
	}
	return 1
}

func nullableRunID(runID string) any {
	if strings.TrimSpace(runID) == "" {
		return nil
	}
	return runID
}

func nullableTeamID(teamID sql.NullInt64) any {
	if !teamID.Valid {
		return nil
	}
	return teamID.Int64
}

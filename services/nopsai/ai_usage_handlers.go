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

	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/pkg/serviceauth"
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
	report.Model = strings.TrimSpace(report.Model)
	report.LLMProfile = strings.TrimSpace(report.LLMProfile)
	report.PromptTokens = nonNegativeInt64(report.PromptTokens)
	report.CompletionTokens = nonNegativeInt64(report.CompletionTokens)
	report.TotalTokens = nonNegativeInt64(report.TotalTokens)
	report.InputCostUSD = nonNegativeFloat64(report.InputCostUSD)
	report.OutputCostUSD = nonNegativeFloat64(report.OutputCostUSD)
	report.TotalCostUSD = nonNegativeFloat64(report.TotalCostUSD)
	if report.TotalTokens == 0 {
		report.TotalTokens = report.PromptTokens + report.CompletionTokens
	}
	if report.TotalCostUSD == 0 {
		report.TotalCostUSD = report.InputCostUSD + report.OutputCostUSD
	}
	if report.Metadata == nil {
		report.Metadata = map[string]any{}
	}
	return report, report.PromptTokens > 0 || report.CompletionTokens > 0 || report.TotalTokens > 0 || report.TotalCostUSD > 0
}

func nonNegativeInt64(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func nonNegativeFloat64(value float64) float64 {
	if value < 0 {
		return 0
	}
	return value
}

func (a *App) recordAIUsage(ctx context.Context, runID string, report models.AIUsageReport) error {
	var (
		pipelinePath         string
		pipelineName         string
		teamID               sql.NullInt64
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
	`, runID).Scan(&pipelinePath, &pipelineName, &teamID, &requestedByType, &requestedByID, &effectiveSubjectType, &effectiveSubjectID); err != nil {
		return fmt.Errorf("load run metadata: %w", err)
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
			prompt_tokens, completion_tokens, total_tokens,
			input_cost_usd, output_cost_usd, total_cost_usd,
			requested_by_type, requested_by_id, effective_subject_type, effective_subject_id, metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21::jsonb)
	`, runID, report.StepName, report.TaskName, pipelinePath, pipelineName, nullableTeamID(teamID),
		report.Feature, report.Provider, report.Model, report.LLMProfile,
		report.PromptTokens, report.CompletionTokens, report.TotalTokens,
		report.InputCostUSD, report.OutputCostUSD, report.TotalCostUSD,
		requestedByType.String, requestedByID.String, effectiveSubjectType.String, effectiveSubjectID.String, string(metadataJSON)); err != nil {
		return fmt.Errorf("insert AI usage event: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO pipeline_run_usage_summary (
			run_id, ai_prompt_tokens, ai_completion_tokens, ai_total_tokens, ai_cost_usd, total_cost_usd, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $5, NOW())
		ON CONFLICT (run_id) DO UPDATE SET
			ai_prompt_tokens = pipeline_run_usage_summary.ai_prompt_tokens + EXCLUDED.ai_prompt_tokens,
			ai_completion_tokens = pipeline_run_usage_summary.ai_completion_tokens + EXCLUDED.ai_completion_tokens,
			ai_total_tokens = pipeline_run_usage_summary.ai_total_tokens + EXCLUDED.ai_total_tokens,
			ai_cost_usd = pipeline_run_usage_summary.ai_cost_usd + EXCLUDED.ai_cost_usd,
			total_cost_usd = pipeline_run_usage_summary.total_cost_usd + EXCLUDED.ai_cost_usd,
			updated_at = NOW()
	`, runID, report.PromptTokens, report.CompletionTokens, report.TotalTokens, report.TotalCostUSD); err != nil {
		return fmt.Errorf("update AI usage summary: %w", err)
	}
	return tx.Commit(ctx)
}

func nullableTeamID(teamID sql.NullInt64) any {
	if !teamID.Valid {
		return nil
	}
	return teamID.Int64
}
